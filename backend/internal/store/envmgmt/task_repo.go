package envmgmt

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	storeauth "github.com/dougong/dougong/internal/store/auth"
)

// TaskRepo 实现 env_install_task 表访问（api §5.2 SQL #1~#9 全参数化）。
type TaskRepo struct{ x Executor }

const taskCols = `id, env_type, action, status, output_tail, error, requested_by,
	created_at, updated_at, started_at, finished_at, bootstrap_status, bootstrap_note`

// Create 插入 pending 任务（SQL #1；与发起审计同事务由 TxStore 保证 I-03）。
func (r *TaskRepo) Create(ctx context.Context, t Task) (int64, error) {
	now := formatTime(time.Now().UTC())
	res, err := r.x.ExecContext(ctx,
		`INSERT INTO env_install_task (env_type, action, status, output_tail, error,
		 requested_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		t.EnvType, t.Action, TaskPending, "", strPtrArg(t.Error), t.RequestedBy, now, now)
	if err != nil {
		return 0, wrapErr("task create", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store/envmgmt: task create last id: %w", err)
	}
	return id, nil
}

// InFlight 同环境在途任务（SQL #2：pending/running；I-01 互斥判定——创建事务
// 内调用，SQLite 单连接写串行保证原子）。
func (r *TaskRepo) InFlight(ctx context.Context, envType string) (*Task, error) {
	row := r.x.QueryRowContext(ctx,
		`SELECT `+taskCols+` FROM env_install_task
		 WHERE env_type = ? AND status IN ('pending','running') ORDER BY id DESC LIMIT 1`, envType)
	t, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, wrapErr("task in-flight", err)
	}
	return &t, nil
}

// InFlightAll 全部在途任务（SQL #9：环境列表在途标记 + 重启恢复扫描共用）。
func (r *TaskRepo) InFlightAll(ctx context.Context) ([]Task, error) {
	rows, err := r.x.QueryContext(ctx,
		`SELECT `+taskCols+` FROM env_install_task
		 WHERE status IN ('pending','running') ORDER BY id ASC`)
	if err != nil {
		return nil, wrapErr("task in-flight all", err)
	}
	defer rows.Close()
	var out []Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("store/envmgmt: in-flight all scan: %w", err)
		}
		out = append(out, t)
	}
	return out, wrapErr("task in-flight all rows", rows.Err())
}

// GetByID 按 id 取单行（SQL #3 通道）；不存在返回 ErrTaskNotFound。
func (r *TaskRepo) GetByID(ctx context.Context, id int64) (Task, error) {
	row := r.x.QueryRowContext(ctx,
		`SELECT `+taskCols+` FROM env_install_task WHERE id = ?`, id)
	t, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrTaskNotFound
	}
	return t, wrapErr("task get by id", err)
}

// List 筛选分页清单（SQL #4：envType?/status?，created_at 倒序）。
func (r *TaskRepo) List(ctx context.Context, envType, status string, limit, offset int) ([]Task, int, error) {
	where := " WHERE 1=1"
	args := []any{}
	if envType != "" {
		where += " AND env_type = ?"
		args = append(args, envType)
	}
	if status != "" {
		where += " AND status = ?"
		args = append(args, status)
	}
	var total int
	if err := r.x.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM env_install_task`+where, args...).Scan(&total); err != nil {
		return nil, 0, wrapErr("task count", err)
	}
	q := `SELECT ` + taskCols + ` FROM env_install_task` + where +
		` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`
	rows, err := r.x.QueryContext(ctx, q, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, wrapErr("task list", err)
	}
	defer rows.Close()
	var out []Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("store/envmgmt: task list scan: %w", err)
		}
		out = append(out, t)
	}
	return out, total, wrapErr("task list rows", rows.Err())
}

// MarkRunning 执行开始（SQL #5）。
func (r *TaskRepo) MarkRunning(ctx context.Context, id int64) error {
	_, err := r.x.ExecContext(ctx,
		`UPDATE env_install_task SET status = 'running', started_at = ?, updated_at = ?
		 WHERE id = ?`, formatTime(time.Now().UTC()), formatTime(time.Now().UTC()), id)
	return wrapErr("task mark running", err)
}

// AppendOutput 输出尾部追加/定格（SQL #6；service 层截尾 ≤200 行后整列覆写）。
func (r *TaskRepo) AppendOutput(ctx context.Context, id int64, tail string) error {
	_, err := r.x.ExecContext(ctx,
		`UPDATE env_install_task SET output_tail = ?, updated_at = ? WHERE id = ?`,
		tail, formatTime(time.Now().UTC()), id)
	return wrapErr("task append output", err)
}

// Finish 终态回写（SQL #7：succeeded/failed + error + finished_at + 引导结果
// bootstrap_status/note 同事务定格，v1.2 D-18/I-09；与终态审计同事务）。
// bootstrapStatus/bootstrapNote 为 nil 时无引导步骤（两列落 NULL）。
func (r *TaskRepo) Finish(ctx context.Context, id int64, status string, taskErr, bootstrapStatus, bootstrapNote *string) error {
	_, err := r.x.ExecContext(ctx,
		`UPDATE env_install_task SET status = ?, error = ?, bootstrap_status = ?, bootstrap_note = ?,
		 finished_at = ?, updated_at = ? WHERE id = ?`,
		status, strPtrArg(taskErr), strPtrArg(bootstrapStatus), strPtrArg(bootstrapNote),
		formatTime(time.Now().UTC()), formatTime(time.Now().UTC()), id)
	return wrapErr("task finish", err)
}

// FailRunningAll 重启恢复（SQL #8：running→failed「面板重启中断」，I-04）。
func (r *TaskRepo) FailRunningAll(ctx context.Context, reason string) (int64, error) {
	now := formatTime(time.Now().UTC())
	res, err := r.x.ExecContext(ctx,
		`UPDATE env_install_task SET status = 'failed', error = ?, finished_at = ?, updated_at = ?
		 WHERE status = 'running'`, reason, now, now)
	if err != nil {
		return 0, wrapErr("task fail running all", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// Audit 返回 audit_log 仓储（跨模块复用 store/auth，同事务 I-03）。
func (s *Store) Audit() *storeauth.AuditRepo { return storeauth.NewAuditRepo(s.db) }

// Audit 返回事务内 audit_log 仓储。
func (t *TxStore) Audit() *storeauth.AuditRepo { return storeauth.NewAuditRepo(t.tx) }

func scanTask(row interface{ Scan(...any) error }) (Task, error) {
	var t Task
	var taskErr, bootstrapStatus, bootstrapNote sql.NullString
	var createdAt, updatedAt any
	var startedAt, finishedAt any
	if err := row.Scan(&t.ID, &t.EnvType, &t.Action, &t.Status, &t.OutputTail, &taskErr,
		&t.RequestedBy, &createdAt, &updatedAt, &startedAt, &finishedAt,
		&bootstrapStatus, &bootstrapNote); err != nil {
		return Task{}, err
	}
	if taskErr.Valid {
		v := taskErr.String
		t.Error = &v
	}
	if bootstrapStatus.Valid {
		v := bootstrapStatus.String
		t.BootstrapStatus = &v
	}
	if bootstrapNote.Valid {
		v := bootstrapNote.String
		t.BootstrapNote = &v
	}
	var err error
	if t.CreatedAt, err = scanTime(createdAt); err != nil {
		return Task{}, fmt.Errorf("scan created_at: %w", err)
	}
	if t.UpdatedAt, err = scanTime(updatedAt); err != nil {
		return Task{}, fmt.Errorf("scan updated_at: %w", err)
	}
	if t.StartedAt, err = scanTimePtr(startedAt); err != nil {
		return Task{}, fmt.Errorf("scan started_at: %w", err)
	}
	if t.FinishedAt, err = scanTimePtr(finishedAt); err != nil {
		return Task{}, fmt.Errorf("scan finished_at: %w", err)
	}
	return t, nil
}

func strPtrArg(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}
