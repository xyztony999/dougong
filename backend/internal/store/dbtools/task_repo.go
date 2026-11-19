package dbtools

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// TaskRepo 实现 tool_deploy_task 表访问（api §5.2 SQL #1~#9 全参数化）。
type TaskRepo struct{ x Executor }

const taskCols = `id, tool_type, action, status, instance_id, output_tail, error,
	requested_by, created_at, updated_at, started_at, finished_at`

// Create 插入 pending 任务（SQL #1；与发起审计同事务由 TxStore 保证 I-08）。
func (r *TaskRepo) Create(ctx context.Context, t Task) (int64, error) {
	now := formatTime(time.Now().UTC())
	res, err := r.x.ExecContext(ctx,
		`INSERT INTO tool_deploy_task (tool_type, action, status, instance_id,
		 output_tail, error, requested_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ToolType, t.Action, TaskPending, int64PtrArg(t.InstanceID), "",
		strPtrArg(t.Error), t.RequestedBy, now, now)
	if err != nil {
		return 0, wrapErr("task create", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store/dbtools: task create last id: %w", err)
	}
	return id, nil
}

// InFlight 同工具在途任务（SQL #2：pending/running；I-06 互斥判定——创建
// 事务内调用，SQLite 单连接写串行保证原子）。
func (r *TaskRepo) InFlight(ctx context.Context, toolType string) (*Task, error) {
	row := r.x.QueryRowContext(ctx,
		`SELECT `+taskCols+` FROM tool_deploy_task
		 WHERE tool_type = ? AND status IN ('pending','running') ORDER BY id DESC LIMIT 1`, toolType)
	t, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, wrapErr("task in-flight", err)
	}
	return &t, nil
}

// InFlightAll 全部在途任务（SQL #9：工具列表在途标记 + 重启恢复扫描共用）。
func (r *TaskRepo) InFlightAll(ctx context.Context) ([]Task, error) {
	rows, err := r.x.QueryContext(ctx,
		`SELECT `+taskCols+` FROM tool_deploy_task
		 WHERE status IN ('pending','running') ORDER BY id ASC`)
	if err != nil {
		return nil, wrapErr("task in-flight all", err)
	}
	defer rows.Close()
	out := []Task{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("store/dbtools: in-flight all scan: %w", err)
		}
		out = append(out, t)
	}
	return out, wrapErr("task in-flight all rows", rows.Err())
}

// GetByID 按 id 取单行（SQL #3）；不存在返回 ErrTaskNotFound（DBT_005）。
func (r *TaskRepo) GetByID(ctx context.Context, id int64) (Task, error) {
	row := r.x.QueryRowContext(ctx,
		`SELECT `+taskCols+` FROM tool_deploy_task WHERE id = ?`, id)
	t, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrTaskNotFound
	}
	return t, wrapErr("task get by id", err)
}

// List 筛选分页清单（SQL #4：toolType?/status?，created_at 倒序）。
func (r *TaskRepo) List(ctx context.Context, toolType, status string, limit, offset int) ([]Task, int, error) {
	where := " WHERE 1=1"
	args := []any{}
	if toolType != "" {
		where += " AND tool_type = ?"
		args = append(args, toolType)
	}
	if status != "" {
		where += " AND status = ?"
		args = append(args, status)
	}
	var total int
	if err := r.x.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tool_deploy_task`+where, args...).Scan(&total); err != nil {
		return nil, 0, wrapErr("task count", err)
	}
	q := `SELECT ` + taskCols + ` FROM tool_deploy_task` + where +
		` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`
	rows, err := r.x.QueryContext(ctx, q, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, wrapErr("task list", err)
	}
	defer rows.Close()
	out := []Task{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("store/dbtools: task list scan: %w", err)
		}
		out = append(out, t)
	}
	return out, total, wrapErr("task list rows", rows.Err())
}

// MarkRunning 执行开始（SQL #5）。
func (r *TaskRepo) MarkRunning(ctx context.Context, id int64) error {
	now := formatTime(time.Now().UTC())
	_, err := r.x.ExecContext(ctx,
		`UPDATE tool_deploy_task SET status = 'running', started_at = ?, updated_at = ? WHERE id = ?`,
		now, now, id)
	return wrapErr("task mark running", err)
}

// AppendOutput 输出尾部追加/定格（SQL #6；≤200 行由采集侧保证 I-10）。
func (r *TaskRepo) AppendOutput(ctx context.Context, id int64, tail string) error {
	_, err := r.x.ExecContext(ctx,
		`UPDATE tool_deploy_task SET output_tail = ?, updated_at = ? WHERE id = ?`,
		tail, formatTime(time.Now().UTC()), id)
	return wrapErr("task append output", err)
}

// Finish 终态回写（SQL #7；同事务档案编排 I-C01 由 TxStore 保证）。
func (r *TaskRepo) Finish(ctx context.Context, id int64, status string, taskErr *string) error {
	now := formatTime(time.Now().UTC())
	_, err := r.x.ExecContext(ctx,
		`UPDATE tool_deploy_task SET status = ?, error = ?, finished_at = ?, updated_at = ? WHERE id = ?`,
		status, strPtrArg(taskErr), now, now, id)
	return wrapErr("task finish", err)
}

// FailRunningAll 重启恢复（SQL #8：running→failed「面板重启中断」，I-09；
// pending 一并终态化——下载/npm 中断态不安全，worker 不续跑）。
func (r *TaskRepo) FailRunningAll(ctx context.Context, reason string) (int64, error) {
	now := formatTime(time.Now().UTC())
	res, err := r.x.ExecContext(ctx,
		`UPDATE tool_deploy_task SET status = 'failed', error = ?, finished_at = ?, updated_at = ?
		 WHERE status IN ('running','pending')`, reason, now, now)
	if err != nil {
		return 0, wrapErr("task fail running all", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("store/dbtools: fail running all rows: %w", err)
	}
	return n, nil
}

func scanTask(row rowScanner) (Task, error) {
	var t Task
	var instanceID sql.NullInt64
	var taskErr sql.NullString
	var createdAt, updatedAt any
	var startedAt, finishedAt any
	err := row.Scan(&t.ID, &t.ToolType, &t.Action, &t.Status, &instanceID, &t.OutputTail,
		&taskErr, &t.RequestedBy, &createdAt, &updatedAt, &startedAt, &finishedAt)
	if err != nil {
		return Task{}, err
	}
	if instanceID.Valid {
		t.InstanceID = &instanceID.Int64
	}
	if taskErr.Valid {
		t.Error = &taskErr.String
	}
	if t.CreatedAt, err = scanTime(createdAt); err != nil {
		return Task{}, fmt.Errorf("store/dbtools: task created_at: %w", err)
	}
	if t.UpdatedAt, err = scanTime(updatedAt); err != nil {
		return Task{}, fmt.Errorf("store/dbtools: task updated_at: %w", err)
	}
	if t.StartedAt, err = scanTimePtr(startedAt); err != nil {
		return Task{}, fmt.Errorf("store/dbtools: task started_at: %w", err)
	}
	if t.FinishedAt, err = scanTimePtr(finishedAt); err != nil {
		return Task{}, fmt.Errorf("store/dbtools: task finished_at: %w", err)
	}
	return t, nil
}
