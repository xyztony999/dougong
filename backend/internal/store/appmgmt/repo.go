package appmgmt

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	storeauth "github.com/dougong/dougong/internal/store/auth"
)

// ---- app（应用档案仓储，api §5.2 SQL #8/#9 + 查询通道）----

const appCols = `id, name, dir, entry, start_mode, start_spec, listen_port,
	unit_name, created_by, created_at, updated_at`

// AppRepo 实现 app 表访问（全参数化；SQL 仅本包）。
type AppRepo struct{ x Executor }

// GetByID 按 id 取档案（不存在 ErrAppNotFound）。
func (r *AppRepo) GetByID(ctx context.Context, id int64) (App, error) {
	row := r.x.QueryRowContext(ctx, `SELECT `+appCols+` FROM app WHERE id = ?`, id)
	return scanApp(row)
}

// GetByName 按应用名取档案（创建唯一性预查/在途归组；不存在 ErrAppNotFound）。
func (r *AppRepo) GetByName(ctx context.Context, name string) (App, error) {
	row := r.x.QueryRowContext(ctx, `SELECT `+appCols+` FROM app WHERE name = ?`, name)
	a, err := scanApp(row)
	if errors.Is(err, sql.ErrNoRows) {
		return App{}, ErrAppNotFound
	}
	return a, wrapErr("app get by name", err)
}

// List 全量清单（应用数 Typically < 20，api prd N1——无分页参数，D-01 投影）。
func (r *AppRepo) List(ctx context.Context) ([]App, error) {
	rows, err := r.x.QueryContext(ctx, `SELECT `+appCols+` FROM app ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, wrapErr("app list", err)
	}
	defer rows.Close()
	var out []App
	for rows.Next() {
		a, err := scanApp(rows)
		if err != nil {
			return nil, fmt.Errorf("store/appmgmt: app list scan: %w", err)
		}
		out = append(out, a)
	}
	return out, wrapErr("app list rows", rows.Err())
}

// Create 插入档案（create 任务 succeeded 终态同事务 I-C01；唯一性由 UNIQUE
// name 约束兜底 I-02——预查已在创建校验链 APP_002 承载）。
func (r *AppRepo) Create(ctx context.Context, a App) (int64, error) {
	fmt.Println("[dbg] AppRepo.Create enter", a.Name, a.UnitName)
	now := formatTime(time.Now().UTC())
	res, err := r.x.ExecContext(ctx,
		`INSERT INTO app (name, dir, entry, start_mode, start_spec, listen_port,
		  unit_name, created_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.Name, a.Dir, a.Entry, a.StartMode, a.StartSpec, a.ListenPort,
		a.UnitName, a.CreatedBy, now, now)
	fmt.Println("[dbg] AppRepo.Create done", err)
	if err != nil {
		return 0, wrapErr("app create", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store/appmgmt: app create last id: %w", err)
	}
	return id, nil
}

// Delete 撤除档案（delete 任务 succeeded 终态同事务 I-C01）。
func (r *AppRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.x.ExecContext(ctx, `DELETE FROM app WHERE id = ?`, id)
	return wrapErr("app delete", err)
}

// UpdateStartSpec 启动参数变更（SQL #13；params 保存与审计同事务）。
func (r *AppRepo) UpdateStartSpec(ctx context.Context, id int64, spec string) error {
	_, err := r.x.ExecContext(ctx,
		`UPDATE app SET start_spec = ?, updated_at = ? WHERE id = ?`,
		spec, formatTime(time.Now().UTC()), id)
	return wrapErr("app update start spec", err)
}

func scanApp(row interface{ Scan(...any) error }) (App, error) {
	var a App
	var createdAt, updatedAt any
	if err := row.Scan(&a.ID, &a.Name, &a.Dir, &a.Entry, &a.StartMode, &a.StartSpec,
		&a.ListenPort, &a.UnitName, &a.CreatedBy, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return App{}, ErrAppNotFound
		}
		return App{}, err
	}
	var err error
	if a.CreatedAt, err = scanTime(createdAt); err != nil {
		return App{}, fmt.Errorf("scan created_at: %w", err)
	}
	if a.UpdatedAt, err = scanTime(updatedAt); err != nil {
		return App{}, fmt.Errorf("scan updated_at: %w", err)
	}
	return a, nil
}

// ---- app_task（任务仓储，api §5.2 SQL #1~#7/#11/#12）----

// TaskRepo 实现 app_task 表访问（全参数化；SQL 仅本包）。
type TaskRepo struct{ x Executor }

const taskCols = `id, app_id, app_name, action, status, purge_dir, output_tail,
	error, requested_by, created_at, updated_at, started_at, finished_at`

// Create 插入 pending 任务（SQL #1；与发起审计同事务 I-07）。
func (r *TaskRepo) Create(ctx context.Context, t AppTask) (int64, error) {
	now := formatTime(time.Now().UTC())
	purge := 0
	if t.PurgeDir {
		purge = 1
	}
	res, err := r.x.ExecContext(ctx,
		`INSERT INTO app_task (app_id, app_name, action, status, purge_dir, output_tail,
		 error, requested_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.AppID, t.AppName, t.Action, TaskPending, purge, "", strPtrArg(t.Error),
		t.RequestedBy, now, now)
	if err != nil {
		return 0, wrapErr("task create", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store/appmgmt: task create last id: %w", err)
	}
	return id, nil
}

// InFlightByName 按应用名查在途任务（create 场景 I-05——档案未登记前以名字互斥）。
func (r *TaskRepo) InFlightByName(ctx context.Context, name string) (*AppTask, error) {
	row := r.x.QueryRowContext(ctx,
		`SELECT `+taskCols+` FROM app_task
		 WHERE app_name = ? AND status IN ('pending','running') ORDER BY id DESC LIMIT 1`, name)
	return scanTaskOpt(row, "task in-flight by name")
}

// InFlightByApp 按档案 id 查在途任务（deps/delete 场景 I-05）。
func (r *TaskRepo) InFlightByApp(ctx context.Context, appID int64) (*AppTask, error) {
	row := r.x.QueryRowContext(ctx,
		`SELECT `+taskCols+` FROM app_task
		 WHERE app_id = ? AND status IN ('pending','running') ORDER BY id DESC LIMIT 1`, appID)
	return scanTaskOpt(row, "task in-flight by app")
}

// GetByID 按 id 取单行；不存在 ErrTaskNotFound。
func (r *TaskRepo) GetByID(ctx context.Context, id int64) (AppTask, error) {
	row := r.x.QueryRowContext(ctx, `SELECT `+taskCols+` FROM app_task WHERE id = ?`, id)
	t, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AppTask{}, ErrTaskNotFound
	}
	return t, wrapErr("task get by id", err)
}

// List 筛选分页清单（SQL #4：appID?/action?/status?，created_at 倒序）。
func (r *TaskRepo) List(ctx context.Context, appID int64, action, status string, limit, offset int) ([]AppTask, int, error) {
	where := " WHERE 1=1"
	args := []any{}
	if appID > 0 {
		where += " AND app_id = ?"
		args = append(args, appID)
	}
	if action != "" {
		where += " AND action = ?"
		args = append(args, action)
	}
	if status != "" {
		where += " AND status = ?"
		args = append(args, status)
	}
	var total int
	if err := r.x.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM app_task`+where, args...).Scan(&total); err != nil {
		return nil, 0, wrapErr("task count", err)
	}
	q := `SELECT ` + taskCols + ` FROM app_task` + where +
		` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`
	rows, err := r.x.QueryContext(ctx, q, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, wrapErr("task list", err)
	}
	defer rows.Close()
	var out []AppTask
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("store/appmgmt: task list scan: %w", err)
		}
		out = append(out, t)
	}
	return out, total, wrapErr("task list rows", rows.Err())
}

// MarkRunning 执行开始（SQL #5）。
func (r *TaskRepo) MarkRunning(ctx context.Context, id int64) error {
	now := formatTime(time.Now().UTC())
	_, err := r.x.ExecContext(ctx,
		`UPDATE app_task SET status = 'running', started_at = ?, updated_at = ? WHERE id = ?`,
		now, now, id)
	return wrapErr("task mark running", err)
}

// Retry 失败任务重试回 running（SQL #7；D-13/I-06 例外——仅
// create/deps_install 由 service 层前置判定；重试复用原任务行）。
func (r *TaskRepo) Retry(ctx context.Context, id int64) error {
	now := formatTime(time.Now().UTC())
	_, err := r.x.ExecContext(ctx,
		`UPDATE app_task SET status = 'running', started_at = ?, finished_at = NULL, updated_at = ?
		 WHERE id = ? AND status = 'failed'`, now, now, id)
	return wrapErr("task retry", err)
}

// AppendOutput 输出尾部追加/定格（service 层截尾 ≤200 行后整列覆写）。
func (r *TaskRepo) AppendOutput(ctx context.Context, id int64, tail string) error {
	_, err := r.x.ExecContext(ctx,
		`UPDATE app_task SET output_tail = ?, updated_at = ? WHERE id = ?`,
		tail, formatTime(time.Now().UTC()), id)
	return wrapErr("task append output", err)
}

// Finish 终态回写（SQL #10；档案协作由 service 层同事务经 App() 完成 I-C01）。
func (r *TaskRepo) Finish(ctx context.Context, id int64, status string, taskErr *string) error {
	now := formatTime(time.Now().UTC())
	_, err := r.x.ExecContext(ctx,
		`UPDATE app_task SET status = ?, error = ?, finished_at = ?, updated_at = ? WHERE id = ?`,
		status, strPtrArg(taskErr), now, now, id)
	return wrapErr("task finish", err)
}

// FailRunningAll 重启恢复（SQL #11：running→failed「面板重启中断」，I-08）。
func (r *TaskRepo) FailRunningAll(ctx context.Context, reason string) (int64, error) {
	now := formatTime(time.Now().UTC())
	res, err := r.x.ExecContext(ctx,
		`UPDATE app_task SET status = 'failed', error = ?, finished_at = ?, updated_at = ?
		 WHERE status = 'running'`, reason, now, now)
	if err != nil {
		return 0, wrapErr("task fail running all", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// InFlightIDs 全部在途任务（SQL #12：应用列表在途标记 + 重启恢复扫描共用）。
func (r *TaskRepo) InFlightIDs(ctx context.Context) ([]AppTask, error) {
	rows, err := r.x.QueryContext(ctx,
		`SELECT `+taskCols+` FROM app_task WHERE status IN ('pending','running') ORDER BY id ASC`)
	if err != nil {
		return nil, wrapErr("task in-flight ids", err)
	}
	defer rows.Close()
	var out []AppTask
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("store/appmgmt: in-flight scan: %w", err)
		}
		out = append(out, t)
	}
	return out, wrapErr("task in-flight rows", rows.Err())
}

// Audit 返回 audit_log 仓储（跨模块复用 store/auth，同事务 I-07）。
func (s *Store) Audit() *storeauth.AuditRepo { return storeauth.NewAuditRepo(s.db) }

// Audit 返回事务内 audit_log 仓储。
func (t *TxStore) Audit() *storeauth.AuditRepo { return storeauth.NewAuditRepo(t.tx) }

func scanTaskOpt(row interface{ Scan(...any) error }, op string) (*AppTask, error) {
	t, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, wrapErr(op, err)
	}
	return &t, nil
}

func scanTask(row interface{ Scan(...any) error }) (AppTask, error) {
	var t AppTask
	var taskErr sql.NullString
	var purge int
	var createdAt, updatedAt any
	var startedAt, finishedAt any
	if err := row.Scan(&t.ID, &t.AppID, &t.AppName, &t.Action, &t.Status, &purge,
		&t.OutputTail, &taskErr, &t.RequestedBy, &createdAt, &updatedAt,
		&startedAt, &finishedAt); err != nil {
		return AppTask{}, err
	}
	t.PurgeDir = purge != 0
	if taskErr.Valid {
		v := taskErr.String
		t.Error = &v
	}
	var err error
	if t.CreatedAt, err = scanTime(createdAt); err != nil {
		return AppTask{}, fmt.Errorf("scan created_at: %w", err)
	}
	if t.UpdatedAt, err = scanTime(updatedAt); err != nil {
		return AppTask{}, fmt.Errorf("scan updated_at: %w", err)
	}
	if t.StartedAt, err = scanTimePtr(startedAt); err != nil {
		return AppTask{}, fmt.Errorf("scan started_at: %w", err)
	}
	if t.FinishedAt, err = scanTimePtr(finishedAt); err != nil {
		return AppTask{}, fmt.Errorf("scan finished_at: %w", err)
	}
	return t, nil
}
