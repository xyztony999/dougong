package cronback

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// RunRepo 是 backup_run 仓储（§4.2.5 BackupRunRepository；SQL 清单 #4~#9）。
type RunRepo struct {
	x Executor
}

const runColumns = `id, job_id, job_name, trigger, status, started_at,
	finished_at, artifact_path, artifact_size, error_message, files`

// Create 插入 running run（SQL #4；trigger 时刻即 started_at）。idx_run_running
// 部分唯一索引冲突（I-01 并发提交后到者）返回 ErrRunMutex。
func (r *RunRepo) Create(ctx context.Context, jobID int64, jobName, trigger string, startedAt time.Time) (int64, error) {
	res, err := r.x.ExecContext(ctx,
		`INSERT INTO backup_run (job_id, job_name, trigger, status, started_at)
		 VALUES (?, ?, ?, ?, ?)`,
		jobID, jobName, trigger, StatusRunning, formatTime(startedAt))
	if err != nil {
		if isConstraintErr(err) {
			return 0, ErrRunMutex
		}
		return 0, fmt.Errorf("cronback: create run: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("cronback: create run last id: %w", err)
	}
	return id, nil
}

// FinishSuccess 终态回写 success（SQL #5，I-02：仅 running 可转移；I-04 终态
// 产物字段一并写入）。返回受影响行数（0=已被孤儿恢复等先行转移）。
func (r *RunRepo) FinishSuccess(ctx context.Context, id int64, finishedAt time.Time,
	artifactPath string, artifactSize int64, files []FileEntry) (int64, error) {
	filesJSON, err := marshalFiles(files)
	if err != nil {
		return 0, err
	}
	res, err := r.x.ExecContext(ctx,
		`UPDATE backup_run SET status = ?, finished_at = ?,
			artifact_path = ?, artifact_size = ?, files = ?, error_message = NULL
		 WHERE id = ? AND status = ?`,
		StatusSuccess, formatTime(finishedAt), artifactPath, artifactSize,
		filesJSON, id, StatusRunning)
	if err != nil {
		return 0, fmt.Errorf("cronback: finish success run %d: %w", id, err)
	}
	return rowsOf(res, id)
}

// FinishFailure 终态回写 failure（SQL #5；error_message 列明失败源；产物字段
// 置 NULL——I-04 failure 不可下载）。
func (r *RunRepo) FinishFailure(ctx context.Context, id int64, finishedAt time.Time, errorMessage string) (int64, error) {
	res, err := r.x.ExecContext(ctx,
		`UPDATE backup_run SET status = ?, finished_at = ?,
			artifact_path = NULL, artifact_size = NULL, files = NULL, error_message = ?
		 WHERE id = ? AND status = ?`,
		StatusFailure, formatTime(finishedAt), errorMessage, id, StatusRunning)
	if err != nil {
		return 0, fmt.Errorf("cronback: finish failure run %d: %w", id, err)
	}
	return rowsOf(res, id)
}

// Rotate 轮转清理（SQL #6，D-09：保留最近 retention 条 success，删除更早
// success 与更早 failure 全部记录）。事务内先收集被删记录的产物定位（事务外
// 产物清理用，A-C4），再 DELETE；返回被删 id 与产物定位集。
func (r *RunRepo) Rotate(ctx context.Context, jobID int64, retention int) (deleted []RotateVictim, err error) {
	rows, err := r.x.QueryContext(ctx,
		`SELECT id, artifact_path FROM backup_run
		 WHERE job_id = ? AND id < (
			SELECT MIN(id) FROM (
				SELECT id FROM backup_run
				WHERE job_id = ? AND status = ?
				ORDER BY id DESC LIMIT ?))`,
		jobID, jobID, StatusSuccess, retention)
	if err != nil {
		return nil, fmt.Errorf("cronback: rotate select job %d: %w", jobID, err)
	}
	defer func() { _ = rows.Close() }()
	victims := make([]RotateVictim, 0, 8)
	ids := make([]string, 0, len(victims))
	for rows.Next() {
		var v RotateVictim
		var path any
		if err := rows.Scan(&v.ID, &path); err != nil {
			return nil, fmt.Errorf("cronback: rotate scan job %d: %w", jobID, err)
		}
		if s := stringPtr(path); s != nil {
			v.ArtifactPath = *s
		}
		victims = append(victims, v)
		ids = append(ids, fmt.Sprintf("%d", v.ID))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cronback: rotate rows job %d: %w", jobID, err)
	}
	if len(victims) == 0 {
		return nil, nil
	}
	// id 集由本事务上一 SELECT 收集（纯数字），IN 列表以 strings.Join 占位符
	// 数组拼接——值全部经 ? 参数化绑定（G-02：无任何用户输入拼入 SQL 文本）。
	placeholders := strings.Repeat("?, ", len(ids))
	placeholders = strings.TrimSuffix(placeholders, ", ")
	args := make([]any, 0, len(ids)+1)
	for _, id := range ids {
		args = append(args, id)
	}
	args = append(args, jobID)
	res, err := r.x.ExecContext(ctx,
		`DELETE FROM backup_run WHERE id IN (`+placeholders+`) AND job_id = ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("cronback: rotate delete job %d: %w", jobID, err)
	}
	if _, err := res.RowsAffected(); err != nil {
		return nil, fmt.Errorf("cronback: rotate rows job %d: %w", jobID, err)
	}
	return victims, nil
}

// Search 条件分页检索（SQL #7：jobId?/status?/trigger?/时间窗，id 倒序）。
// 动态 WHERE 为静态条件串拼接 + ? 参数数组（audit 先例同法，G-02）。
func (r *RunRepo) Search(ctx context.Context, f RunFilter, offset, limit int) ([]Run, int64, error) {
	where, args := runWhere(f)
	totalRow := r.x.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM backup_run`+where, args...)
	var total int64
	if err := totalRow.Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("cronback: count runs: %w", err)
	}
	rows, err := r.x.QueryContext(ctx,
		`SELECT `+runColumns+` FROM backup_run`+where+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(append([]any{}, args...), limit, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("cronback: search runs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	runs := make([]Run, 0, limit)
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, 0, err
		}
		runs = append(runs, *run)
	}
	return runs, total, rows.Err()
}

// GetByID 按	id 取 run；不存在返回 nil, nil（CB_007 由 service 判定）。
func (r *RunRepo) GetByID(ctx context.Context, id int64) (*Run, error) {
	row := r.x.QueryRowContext(ctx,
		`SELECT `+runColumns+` FROM backup_run WHERE id = ?`, id)
	run, err := scanRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return run, nil
}

// LatestByJob 取 job 最近一条 run 摘要（SQL #8 读时派生，D-11）；无记录
// 返回 nil, nil。
func (r *RunRepo) LatestByJob(ctx context.Context, jobID int64) (*Run, error) {
	row := r.x.QueryRowContext(ctx,
		`SELECT `+runColumns+` FROM backup_run
		 WHERE job_id = ? ORDER BY id DESC LIMIT 1`, jobID)
	run, err := scanRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return run, nil
}

// MarkOrphanFailed 启动恢复编排（SQL #9：进程启动时将全部 running 孤儿置
// failure，D-08——否则 I-01 互斥永久堵塞该任务）。返回受影响行数。
func (r *RunRepo) MarkOrphanFailed(ctx context.Context, now time.Time, reason string) (int64, error) {
	res, err := r.x.ExecContext(ctx,
		`UPDATE backup_run SET status = ?, finished_at = ?, error_message = ?
		 WHERE status = ?`,
		StatusFailure, formatTime(now), reason, StatusRunning)
	if err != nil {
		return 0, fmt.Errorf("cronback: mark orphan failed: %w", err)
	}
	return rowsOf(res, 0)
}

// rowsOf 读取受影响行数（id 仅用于错误上下文）。
func rowsOf(res sql.Result, id int64) (int64, error) {
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("cronback: rows affected (run %d): %w", id, err)
	}
	return n, nil
}

// runWhere 组装检索条件（静态条件串 + ? 参数数组）。
func runWhere(f RunFilter) (string, []any) {
	conds := make([]string, 0, 5)
	args := make([]any, 0, 5)
	if f.JobID != nil {
		conds = append(conds, "job_id = ?")
		args = append(args, *f.JobID)
	}
	if f.Status != "" {
		conds = append(conds, "status = ?")
		args = append(args, f.Status)
	}
	if f.Trigger != "" {
		conds = append(conds, "trigger = ?")
		args = append(args, f.Trigger)
	}
	if f.From != nil {
		conds = append(conds, "started_at >= ?")
		args = append(args, formatTime(*f.From))
	}
	if f.To != nil {
		conds = append(conds, "started_at <= ?")
		args = append(args, formatTime(*f.To))
	}
	if len(conds) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// scanRun 逐列扫描 backup_run 行并反序列化 files JSON。
func scanRun(row scanner) (*Run, error) {
	var (
		run          Run
		filesRaw     *string
		startedRaw   string
		finishedRaw  any
		artifactPath any
		artifactSize any
		errorMessage any
	)
	if err := row.Scan(&run.ID, &run.JobID, &run.JobName, &run.Trigger, &run.Status,
		&startedRaw, &finishedRaw, &artifactPath, &artifactSize, &errorMessage, &filesRaw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		return nil, fmt.Errorf("cronback: scan run: %w", err)
	}
	startedAt, err := time.Parse(time.RFC3339, startedRaw)
	if err != nil {
		return nil, fmt.Errorf("cronback: parse run started_at %q: %w", startedRaw, err)
	}
	run.StartedAt = startedAt
	if run.FinishedAt, err = scanTimePtr(finishedRaw); err != nil {
		return nil, err
	}
	run.ArtifactPath = stringPtr(artifactPath)
	run.ArtifactSize = int64Ptr(artifactSize)
	run.ErrorMessage = stringPtr(errorMessage)
	if run.Files, err = unmarshalFiles(filesRaw); err != nil {
		return nil, err
	}
	return &run, nil
}
