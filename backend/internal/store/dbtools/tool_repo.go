package dbtools

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ToolRepo 实现 db_tool 表访问（api §5.2 SQL #10~#15 全参数化）。
type ToolRepo struct{ x Executor }

const toolCols = `id, tool_type, installed_version, deploy_path, site_ref,
	unit_name, instance_id, cred_cipher, created_at, updated_at`

// Create 安装落档（SQL #10；install succeeded 同事务 I-C01 由 TxStore 保证）。
func (r *ToolRepo) Create(ctx context.Context, t Tool) (int64, error) {
	now := formatTime(time.Now().UTC())
	res, err := r.x.ExecContext(ctx,
		`INSERT INTO db_tool (tool_type, installed_version, deploy_path, site_ref,
		 unit_name, instance_id, cred_cipher, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ToolType, t.InstalledVersion, t.DeployPath, t.SiteRef,
		strPtrArg(t.UnitName), int64PtrArg(t.InstanceID), t.CredCipher, now, now)
	if err != nil {
		return 0, wrapErr("tool create", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store/dbtools: tool create last id: %w", err)
	}
	return id, nil
}

// GetByType 按工具类型取档案（SQL #11）；不存在返回 nil（部署状态=档案
// 存在性投影，D-02）。
func (r *ToolRepo) GetByType(ctx context.Context, toolType string) (*Tool, error) {
	row := r.x.QueryRowContext(ctx,
		`SELECT `+toolCols+` FROM db_tool WHERE tool_type = ?`, toolType)
	t, err := scanTool(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, wrapErr("tool get by type", err)
	}
	return &t, nil
}

// UpdateVersion 升级换版（SQL #12；upgrade succeeded 同事务）。
func (r *ToolRepo) UpdateVersion(ctx context.Context, toolType, version string) error {
	_, err := r.x.ExecContext(ctx,
		`UPDATE db_tool SET installed_version = ?, updated_at = ? WHERE tool_type = ?`,
		version, formatTime(time.Now().UTC()), toolType)
	return wrapErr("tool update version", err)
}

// UpdateCredCipher 凭据轮换（SQL #13；rotate 成功后同事务审计 I-C02）。
func (r *ToolRepo) UpdateCredCipher(ctx context.Context, toolType, cipher string) error {
	_, err := r.x.ExecContext(ctx,
		`UPDATE db_tool SET cred_cipher = ?, updated_at = ? WHERE tool_type = ?`,
		cipher, formatTime(time.Now().UTC()), toolType)
	return wrapErr("tool update cred cipher", err)
}

// Delete 卸载除档（SQL #14；uninstall succeeded 同事务——绝不触及引擎数据
// 目录与 db_instance 档案，I-C03 跨域硬约束）。
func (r *ToolRepo) Delete(ctx context.Context, toolType string) error {
	_, err := r.x.ExecContext(ctx, `DELETE FROM db_tool WHERE tool_type = ?`, toolType)
	return wrapErr("tool delete", err)
}

// List 全部档案（SQL #15；工具列表档案存在性投影）。
func (r *ToolRepo) List(ctx context.Context) ([]Tool, error) {
	rows, err := r.x.QueryContext(ctx, `SELECT `+toolCols+` FROM db_tool ORDER BY id ASC`)
	if err != nil {
		return nil, wrapErr("tool list", err)
	}
	defer rows.Close()
	out := []Tool{}
	for rows.Next() {
		t, err := scanTool(rows)
		if err != nil {
			return nil, fmt.Errorf("store/dbtools: tool list scan: %w", err)
		}
		out = append(out, t)
	}
	return out, wrapErr("tool list rows", rows.Err())
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanTool(row rowScanner) (Tool, error) {
	var t Tool
	var unitName sql.NullString
	var instanceID sql.NullInt64
	var createdAt, updatedAt any
	err := row.Scan(&t.ID, &t.ToolType, &t.InstalledVersion, &t.DeployPath, &t.SiteRef,
		&unitName, &instanceID, &t.CredCipher, &createdAt, &updatedAt)
	if err != nil {
		return Tool{}, err
	}
	if unitName.Valid {
		t.UnitName = &unitName.String
	}
	if instanceID.Valid {
		t.InstanceID = &instanceID.Int64
	}
	if t.CreatedAt, err = scanTime(createdAt); err != nil {
		return Tool{}, fmt.Errorf("store/dbtools: tool created_at: %w", err)
	}
	if t.UpdatedAt, err = scanTime(updatedAt); err != nil {
		return Tool{}, fmt.Errorf("store/dbtools: tool updated_at: %w", err)
	}
	return t, nil
}
