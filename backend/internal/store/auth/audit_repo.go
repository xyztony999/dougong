package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// AuditRepo 实现 audit_log 表访问（只追加 I-14：本仓储不提供 UPDATE；
// DELETE 仅限按保留策略的分批清理 DeleteBefore）。
type AuditRepo struct{ x Executor }

// NewAuditRepo 构造绑定给定执行器的审计仓储：供其他业务模块（panel-core 等）
// 在自身事务视图内写入同事务审计（I-05 语义，R-03 全变更记审计）。
func NewAuditRepo(x Executor) *AuditRepo { return &AuditRepo{x: x} }

// Append 追加一条审计记录；at 为发生时间（UTC 落库），Detail 序列化为 JSON（nil → NULL）。
func (r *AuditRepo) Append(ctx context.Context, e AuditEntry, at time.Time) error {
	var detail any
	if e.Detail != nil {
		b, err := json.Marshal(e.Detail)
		if err != nil {
			return fmt.Errorf("store/auth: audit marshal detail: %w", err)
		}
		detail = string(b)
	}
	var actorID any
	if e.ActorID != nil {
		actorID = *e.ActorID
	}
	_, err := r.x.ExecContext(ctx,
		`INSERT INTO audit_log (actor_id, actor_name, category, action, object_type, object_id, detail, result, source, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		actorID, e.ActorName, e.Category, e.Action, e.ObjectType, e.ObjectID, detail, e.Result, e.Source, formatTime(at))
	return wrapErr("audit append", err)
}

// auditWhere 依据条件构造 WHERE 片段与参数（条件串全部为静态字面量拼接，
// 值一律 ? 参数绑定——G-02 合规；ActorName 做 LIKE 模糊匹配并转义通配符）。
func auditWhere(c AuditCondition) (string, []any) {
	conds := []string{"1 = 1"}
	var args []any
	if c.From != nil {
		conds = append(conds, "created_at >= ?")
		args = append(args, formatTime(*c.From))
	}
	if c.To != nil {
		conds = append(conds, "created_at <= ?")
		args = append(args, formatTime(*c.To))
	}
	if c.ActorName != "" {
		conds = append(conds, "actor_name LIKE ? ESCAPE '\\'")
		args = append(args, "%"+escapeLike(c.ActorName)+"%")
	}
	if c.Category != "" {
		conds = append(conds, "category = ?")
		args = append(args, c.Category)
	}
	if c.Action != "" {
		conds = append(conds, "action = ?")
		args = append(args, c.Action)
	}
	if c.ObjectType != "" {
		conds = append(conds, "object_type = ?")
		args = append(args, c.ObjectType)
	}
	if c.ObjectID != "" {
		conds = append(conds, "object_id = ?")
		args = append(args, c.ObjectID)
	}
	return strings.Join(conds, " AND "), args
}

func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

const auditCols = `id, actor_id, actor_name, category, action, object_type, object_id, detail, result, source, created_at`

func scanAudit(row interface{ Scan(...any) error }) (AuditRecord, error) {
	var a AuditRecord
	var actorID, createdAt any
	var objectID, detail sql.NullString
	if err := row.Scan(&a.ID, &actorID, &a.ActorName, &a.Category, &a.Action,
		&a.ObjectType, &objectID, &detail, &a.Result, &a.Source, &createdAt); err != nil {
		return AuditRecord{}, err
	}
	if actorID != nil {
		if v, ok := actorID.(int64); ok {
			a.ActorID = &v
		} else {
			return AuditRecord{}, fmt.Errorf("store/auth: audit actor_id type %T", actorID)
		}
	}
	a.ObjectID = nullStringPtr(objectID) // NULL → JSON null（契约 §4.3.4 可空字段）
	a.Detail = nullStringPtr(detail)
	t, err := scanTime(createdAt)
	if err != nil {
		return AuditRecord{}, fmt.Errorf("scan audit created_at: %w", err)
	}
	a.CreatedAt = t
	return a, nil
}

// nullStringPtr 将 sql.NullString 转为 *string（NULL → nil → JSON null）。
func nullStringPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	v := ns.String
	return &v
}

// Search 条件分页查询（created_at 倒序），返回当页记录与满足条件的总数。
// offset/limit 由 service 层按公共分页约定换算。
func (r *AuditRepo) Search(ctx context.Context, cond AuditCondition, offset, limit int) ([]AuditRecord, int64, error) {
	where, args := auditWhere(cond)
	var total int64
	if err := r.x.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audit_log WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, wrapErr("audit count", err)
	}
	q := `SELECT ` + auditCols + ` FROM audit_log WHERE ` + where +
		` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`
	rows, err := r.x.QueryContext(ctx, q, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, wrapErr("audit search", err)
	}
	defer closeRows(rows)
	var out []AuditRecord
	for rows.Next() {
		a, err := scanAudit(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("audit scan: %w", err)
		}
		out = append(out, a)
	}
	return out, total, wrapErr("audit search rows", rows.Err())
}

// DeleteBefore 分批物理删除 created_at 早于 cutoff 的记录，返回本批删除行数
// （审计清理任务循环调用至不足一批；I-14 允许的唯一删除路径）。
func (r *AuditRepo) DeleteBefore(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	res, err := r.x.ExecContext(ctx,
		`DELETE FROM audit_log WHERE id IN (
			SELECT id FROM audit_log WHERE created_at < ? ORDER BY id LIMIT ?)`,
		formatTime(cutoff), limit)
	if err != nil {
		return 0, wrapErr("audit delete before", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("store/auth: audit delete before rows: %w", err)
	}
	return n, nil
}
