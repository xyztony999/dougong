package fileman

import (
	"context"
	"fmt"
	"time"
)

// TrashRepo 实现 trash_entry 表访问（SQL 清单 §5.2 #4~#7，全参数化）。
// 入站 Insert 与还原 UpdateRestored、清空 DeleteAllInTrash 均在调用方
// 事务视图内执行（I-01 同事务边界，D-15）。
type TrashRepo struct{ x Executor }

// NewTrashRepo 构造绑定给定执行器的回收站仓储。
func NewTrashRepo(x Executor) *TrashRepo { return &TrashRepo{x: x} }

const trashCols = `id, original_path, trash_name, entry_type, size, operator_id, operator_name, status, deleted_at, resolved_at`

// Insert 入站登记（SQL #4：与文件系统 rename 配对同一事务，I-01/D-15）。
// id 自增回填 e.ID。
func (r *TrashRepo) Insert(ctx context.Context, e *TrashEntry, at time.Time) error {
	var operatorID any
	if e.OperatorID != nil {
		operatorID = *e.OperatorID
	}
	res, err := r.x.ExecContext(ctx,
		`INSERT INTO trash_entry
		   (original_path, trash_name, entry_type, size, operator_id, operator_name, status, deleted_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.OriginalPath, e.TrashName, e.EntryType, e.Size, operatorID,
		e.OperatorName, TrashStatusInTrash, formatTime(at))
	if err != nil {
		return wrapErr("trash insert", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("store/fileman: trash insert last id: %w", err)
	}
	e.ID = id
	e.Status = TrashStatusInTrash
	e.DeletedAt = at
	return nil
}

// GetByID 按主键读取（还原/清空前置校验；行不存在返回包装后的 sql.ErrNoRows）。
func (r *TrashRepo) GetByID(ctx context.Context, id int64) (TrashEntry, error) {
	row := r.x.QueryRowContext(ctx,
		`SELECT `+trashCols+` FROM trash_entry WHERE id = ?`, id)
	return scanTrash(row)
}

// ListInTrash 在站条目清单（SQL #5：status=in_trash，deleted_at 倒序，D-03）。
func (r *TrashRepo) ListInTrash(ctx context.Context) ([]TrashEntry, error) {
	rows, err := r.x.QueryContext(ctx,
		`SELECT `+trashCols+` FROM trash_entry WHERE status = ? ORDER BY deleted_at DESC, id DESC`,
		TrashStatusInTrash)
	if err != nil {
		return nil, wrapErr("trash list in_trash", err)
	}
	defer func() { _ = rows.Close() }()
	var out []TrashEntry
	for rows.Next() {
		e, err := scanTrash(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, wrapErr("trash list in_trash rows", rows.Err())
}

// UpdateRestored 还原（SQL #6：status in_trash→restored + resolved_at，
// 条件带 status='in_trash' 防终态逆转，I-02；返回是否实际更新）。
func (r *TrashRepo) UpdateRestored(ctx context.Context, id int64, at time.Time) (bool, error) {
	res, err := r.x.ExecContext(ctx,
		`UPDATE trash_entry SET status = ?, resolved_at = ? WHERE id = ? AND status = ?`,
		TrashStatusRestored, formatTime(at), id, TrashStatusInTrash)
	if err != nil {
		return false, wrapErr("trash update restored", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store/fileman: trash update restored rows: %w", err)
	}
	return n > 0, nil
}

// DeleteAllInTrash 清空全部在站记录（SQL #7 DELETE 子句；与内容删除同事务，I-01）。
func (r *TrashRepo) DeleteAllInTrash(ctx context.Context) (int64, error) {
	res, err := r.x.ExecContext(ctx,
		`DELETE FROM trash_entry WHERE status = ?`, TrashStatusInTrash)
	if err != nil {
		return 0, wrapErr("trash delete all in_trash", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("store/fileman: trash delete all rows: %w", err)
	}
	return n, nil
}

// Delete 按主键删除单条（清空逐条路径：内容删除与记录删除同事务，I-01）。
func (r *TrashRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.x.ExecContext(ctx, `DELETE FROM trash_entry WHERE id = ?`, id)
	return wrapErr("trash delete", err)
}

func scanTrash(row interface{ Scan(...any) error }) (TrashEntry, error) {
	var e TrashEntry
	var operatorID, deletedAt, resolvedAt any
	if err := row.Scan(&e.ID, &e.OriginalPath, &e.TrashName, &e.EntryType, &e.Size,
		&operatorID, &e.OperatorName, &e.Status, &deletedAt, &resolvedAt); err != nil {
		return TrashEntry{}, wrapErr("trash scan", err)
	}
	if operatorID != nil {
		if v, ok := operatorID.(int64); ok {
			e.OperatorID = &v
		} else {
			return TrashEntry{}, fmt.Errorf("store/fileman: trash operator_id type %T", operatorID)
		}
	}
	t, err := scanTime(deletedAt)
	if err != nil {
		return TrashEntry{}, fmt.Errorf("store/fileman: trash scan deleted_at: %w", err)
	}
	e.DeletedAt = t
	if resolvedAt != nil {
		rt, err := scanTimePtr(resolvedAt)
		if err != nil {
			return TrashEntry{}, fmt.Errorf("store/fileman: trash scan resolved_at: %w", err)
		}
		e.ResolvedAt = rt
	}
	return e, nil
}
