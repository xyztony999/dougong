package dbmgmt

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// InstanceRepo 是 db_instance 档案表仓储（api-database-mgmt.md §4.1.5 仓储
// 契约 InstanceRepository；SQL 清单 §5.2 #1~#6 全参数绑定 D-13）。事务绑定
// 形态（*sql.Tx）供凭据/移除/探测与审计同事务编排（R-03，3.2）。
type InstanceRepo struct {
	x Executor
}

// NewInstanceRepo 构造仓储（x 为 *sql.DB 或事务绑定 *sql.Tx）。
func NewInstanceRepo(x Executor) *InstanceRepo { return &InstanceRepo{x: x} }

const instanceColumns = `id, identity_key, engine, conn_mode, socket_path, host, port,
	version, conn_state, cred_username, cred_cipher, cred_updated_at,
	discovered_at, updated_at`

// List 列出全部实例档案，按 discovered_at 升序；engine 非空时按引擎筛选
// （SQL 清单 #1；GET /db/instance/list 数据源，D-04 读路径零写副作用）。
func (r *InstanceRepo) List(ctx context.Context, engine string) ([]Instance, error) {
	q := `SELECT ` + instanceColumns + ` FROM db_instance`
	var args []any
	if engine != "" {
		q += ` WHERE engine = ?`
		args = append(args, engine)
	}
	q += ` ORDER BY discovered_at, id`
	rows, err := r.x.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, wrapErr("instance list", err)
	}
	defer rows.Close()
	var out []Instance
	for rows.Next() {
		inst, serr := scanInstance(rows)
		if serr != nil {
			return nil, fmt.Errorf("store/dbmgmt: scan instance row: %w", serr)
		}
		out = append(out, inst)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapErr("instance list rows", err)
	}
	return out, nil
}

// GetByID 按 id 取单条（nil, nil = 不存在；I-05 档案存在性前置 DBM_002 判据）。
func (r *InstanceRepo) GetByID(ctx context.Context, id int64) (*Instance, error) {
	row := r.x.QueryRowContext(ctx,
		`SELECT `+instanceColumns+` FROM db_instance WHERE id = ?`, id)
	inst, err := scanInstance(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store/dbmgmt: get instance %d: %w", id, err)
	}
	return &inst, nil
}

// GetByIdentity 按 identity_key 取单条（SQL 清单 #6；连接目标纠正冲突校验
// DBM_020 与探测归并 I-01 数据源）。
func (r *InstanceRepo) GetByIdentity(ctx context.Context, key string) (*Instance, error) {
	row := r.x.QueryRowContext(ctx,
		`SELECT `+instanceColumns+` FROM db_instance WHERE identity_key = ?`, key)
	inst, err := scanInstance(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store/dbmgmt: get instance by identity %q: %w", key, err)
	}
	return &inst, nil
}

// UpsertDiscovered 探测登记与状态整值更新（SQL 清单 #2；D-07/I-01/I-03）：
// identity_key 已存在 → conn_state/version 整值覆盖（返回 discovered=false）；
// 不存在 → INSERT 无凭据档案（返回 discovered=true）。version 可为 nil（无凭据
// 探测仅达传输层，version 维持既有值——D-04 仅可达连接实况刷新）。
// 本方法须在事务绑定仓储上调用（与审计同事务，rescan 场景 R-03）。
func (r *InstanceRepo) UpsertDiscovered(ctx context.Context, inst Instance, connState string, version *string) (discovered bool, err error) {
	existing, err := r.GetByIdentity(ctx, inst.IdentityKey)
	if err != nil {
		return false, err
	}
	now := formatTime(time.Now().UTC())
	if existing != nil {
		newVersion := existing.Version
		if version != nil {
			newVersion = version
		}
		if _, err := r.x.ExecContext(ctx,
			`UPDATE db_instance SET conn_state = ?, version = ?, updated_at = ? WHERE id = ?`,
			connState, strPtrArg(newVersion), now, existing.ID,
		); err != nil {
			return false, wrapErr("instance upsert update", err)
		}
		return false, nil
	}
	if _, err := r.x.ExecContext(ctx,
		`INSERT INTO db_instance (identity_key, engine, conn_mode, socket_path, host, port,
			version, conn_state, cred_username, cred_cipher, cred_updated_at,
			discovered_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL, ?, ?)`,
		inst.IdentityKey, inst.Engine, inst.ConnMode,
		strPtrArg(inst.SocketPath), strPtrArg(inst.Host), intPtrArg(inst.Port),
		strPtrArg(inst.Version), connState, now, now,
	); err != nil {
		return false, wrapErr("instance upsert insert", err)
	}
	return true, nil
}

// UpdateCredentials 凭据验证通过后落库（SQL 清单 #3；D-03 先验证后写：
// conn_state 固定 reachable + version 实况刷新；cred_username/cred_cipher
// 同进 I-09）。本方法须在事务绑定仓储上调用（与审计同事务，R-03）。
func (r *InstanceRepo) UpdateCredentials(ctx context.Context, id int64, username, cipher string, version *string) error {
	now := formatTime(time.Now().UTC())
	if _, err := r.x.ExecContext(ctx,
		`UPDATE db_instance SET cred_username = ?, cred_cipher = ?, cred_updated_at = ?,
			conn_state = 'reachable', version = ?, updated_at = ? WHERE id = ?`,
		username, cipher, now, strPtrArg(version), now, id,
	); err != nil {
		return wrapErr("instance update credentials", err)
	}
	return nil
}

// UpdateTargetAndCredentials 连接目标纠正与凭据一并落库（SQL 清单 #7——
// A-DB9 裁定新增：D-03 连接目标纠正字段触发 identity_key 重算，与凭据
// UPSERT 同事务原子提交；conn_state 固定 reachable=验证通过语义 D-03）。
func (r *InstanceRepo) UpdateTargetAndCredentials(ctx context.Context, id int64,
	identityKey, connMode string, socketPath, host *string, port *int,
	username, cipher string, version *string) error {
	now := formatTime(time.Now().UTC())
	if _, err := r.x.ExecContext(ctx,
		`UPDATE db_instance SET identity_key = ?, conn_mode = ?, socket_path = ?, host = ?, port = ?,
			cred_username = ?, cred_cipher = ?, cred_updated_at = ?,
			conn_state = 'reachable', version = ?, updated_at = ? WHERE id = ?`,
		identityKey, connMode, strPtrArg(socketPath), strPtrArg(host), intPtrArg(port),
		username, cipher, now, strPtrArg(version), now, id,
	); err != nil {
		return wrapErr("instance update target and credentials", err)
	}
	return nil
}

// UpdateConnState 探测状态整值刷新（SQL 清单 #4；I-03：仅探测动作调用）。
func (r *InstanceRepo) UpdateConnState(ctx context.Context, id int64, connState string, version *string) error {
	now := formatTime(time.Now().UTC())
	if _, err := r.x.ExecContext(ctx,
		`UPDATE db_instance SET conn_state = ?, version = ?, updated_at = ? WHERE id = ?`,
		connState, strPtrArg(version), now, id,
	); err != nil {
		return wrapErr("instance update conn state", err)
	}
	return nil
}

// Delete 移除纳管（SQL 清单 #5；⚠️A20：仅删档案行连带凭据密文，不卸载不删数据）。
func (r *InstanceRepo) Delete(ctx context.Context, id int64) error {
	if _, err := r.x.ExecContext(ctx, `DELETE FROM db_instance WHERE id = ?`, id); err != nil {
		return wrapErr("instance delete", err)
	}
	return nil
}

// scanInstance 统一行扫描（*sql.Row 与 *sql.Rows 共同满足 Scan 接口）。
func scanInstance(row interface{ Scan(...any) error }) (Instance, error) {
	var inst Instance
	var socketPath, host, version, credUser, credCipher sql.NullString
	var port sql.NullInt64
	var credUpdatedAt, discoveredAt, updatedAt any
	if err := row.Scan(&inst.ID, &inst.IdentityKey, &inst.Engine, &inst.ConnMode,
		&socketPath, &host, &port, &version, &inst.ConnState,
		&credUser, &credCipher, &credUpdatedAt, &discoveredAt, &updatedAt); err != nil {
		return Instance{}, err
	}
	inst.SocketPath = nullStr(socketPath)
	inst.Host = nullStr(host)
	if port.Valid {
		v := int(port.Int64)
		inst.Port = &v
	}
	inst.Version = nullStr(version)
	inst.CredUsername = nullStr(credUser)
	inst.CredCipher = nullStr(credCipher)
	var err error
	if inst.CredUpdatedAt, err = scanTimePtr(credUpdatedAt); err != nil {
		return Instance{}, fmt.Errorf("scan cred_updated_at: %w", err)
	}
	if inst.DiscoveredAt, err = scanTime(discoveredAt); err != nil {
		return Instance{}, fmt.Errorf("scan discovered_at: %w", err)
	}
	if inst.UpdatedAt, err = scanTime(updatedAt); err != nil {
		return Instance{}, fmt.Errorf("scan updated_at: %w", err)
	}
	return inst, nil
}

func nullStr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	v := ns.String
	return &v
}

func strPtrArg(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func intPtrArg(i *int) any {
	if i == nil {
		return nil
	}
	return *i
}

// wrapErr 包装存储错误（SQLite 约束冲突等保持 %w 链路供上层语义分派）。
func wrapErr(op string, err error) error {
	return fmt.Errorf("store/dbmgmt: %s: %w", op, err)
}
