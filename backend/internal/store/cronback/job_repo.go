package cronback

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"modernc.org/sqlite"
)

// JobRepo 是 backup_job 仓储（§4.1.5 BackupJobRepository：List/GetByID/Create/
// Update/Delete；SQL 清单 #1~#3）。同一实现绑定 *sql.DB（连接池）或 *sql.Tx
// （事务视图）——store.Executor 最小接口（I-05 同事务组合）。
type JobRepo struct {
	x Executor
}

const jobColumns = `id, name, source_items, target_type, target_config,
	retention_count, schedule_cron, created_at, updated_at`

// List 全量任务清单（id 升序；任务个位数~十余，无分页，§4.1.4）。
func (r *JobRepo) List(ctx context.Context) ([]Job, error) {
	rows, err := r.x.QueryContext(ctx,
		`SELECT `+jobColumns+` FROM backup_job ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("cronback: list jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var jobs []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, *j)
	}
	return jobs, rows.Err()
}

// GetByID 按 id 取任务；不存在返回 nil, nil（CB_004 由 service 判定）。
func (r *JobRepo) GetByID(ctx context.Context, id int64) (*Job, error) {
	row := r.x.QueryRowContext(ctx,
		`SELECT `+jobColumns+` FROM backup_job WHERE id = ?`, id)
	j, err := scanJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return j, nil
}

// GetByName 按名取任务（I-06 唯一性预检与编辑排除自身）；不存在 nil, nil。
func (r *JobRepo) GetByName(ctx context.Context, name string) (*Job, error) {
	row := r.x.QueryRowContext(ctx,
		`SELECT `+jobColumns+` FROM backup_job WHERE name = ?`, name)
	j, err := scanJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return j, nil
}

// Create 创建任务（SQL #1）；name 唯一冲突返回 ErrJobNameTaken。
func (r *JobRepo) Create(ctx context.Context, j *Job, now time.Time) (int64, error) {
	sources, err := marshalSourceItems(j.SourceItems)
	if err != nil {
		return 0, err
	}
	target, err := marshalTargetConfig(j.TargetConfig)
	if err != nil {
		return 0, err
	}
	res, err := r.x.ExecContext(ctx,
		`INSERT INTO backup_job
			(name, source_items, target_type, target_config, retention_count, schedule_cron, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		j.Name, sources, j.TargetType, target, j.RetentionCount, j.ScheduleCron,
		formatTime(now), formatTime(now))
	if err != nil {
		if isConstraintErr(err) {
			return 0, ErrJobNameTaken
		}
		return 0, fmt.Errorf("cronback: create job: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("cronback: create job last id: %w", err)
	}
	j.ID, j.CreatedAt, j.UpdatedAt = id, now, now
	return id, nil
}

// Update 整体替换定义列（SQL #2，值对象替换语义）；name 撞他人返回
// ErrJobNameTaken。
func (r *JobRepo) Update(ctx context.Context, j *Job, now time.Time) error {
	sources, err := marshalSourceItems(j.SourceItems)
	if err != nil {
		return err
	}
	target, err := marshalTargetConfig(j.TargetConfig)
	if err != nil {
		return err
	}
	res, err := r.x.ExecContext(ctx,
		`UPDATE backup_job SET
			name = ?, source_items = ?, target_type = ?, target_config = ?,
			retention_count = ?, schedule_cron = ?, updated_at = ?
		 WHERE id = ?`,
		j.Name, sources, j.TargetType, target,
		j.RetentionCount, j.ScheduleCron, formatTime(now), j.ID)
	if err != nil {
		if isConstraintErr(err) {
			return ErrJobNameTaken
		}
		return fmt.Errorf("cronback: update job %d: %w", j.ID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cronback: update job %d rows: %w", j.ID, err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	j.UpdatedAt = now
	return nil
}

// Delete 删除任务行（SQL #3；run/产物不级联——FR-09/D-12）。
func (r *JobRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.x.ExecContext(ctx,
		`DELETE FROM backup_job WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("cronback: delete job %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cronback: delete job %d rows: %w", id, err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// scanner 是 *sql.Row 与 *sql.Rows 共同满足的扫描接口。
type scanner interface{ Scan(dest ...any) error }

// scanJob 逐列扫描 backup_job 行并反序列化 JSON 值列。
func scanJob(row scanner) (*Job, error) {
	var (
		j          Job
		sourcesRaw string
		targetRaw  string
		createdRaw string
		updatedRaw string
	)
	if err := row.Scan(&j.ID, &j.Name, &sourcesRaw, &j.TargetType, &targetRaw,
		&j.RetentionCount, &j.ScheduleCron, &createdRaw, &updatedRaw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		return nil, fmt.Errorf("cronback: scan job: %w", err)
	}
	items, err := unmarshalSourceItems(sourcesRaw)
	if err != nil {
		return nil, err
	}
	target, err := unmarshalTargetConfig(targetRaw)
	if err != nil {
		return nil, err
	}
	createdAt, err := time.Parse(time.RFC3339, createdRaw)
	if err != nil {
		return nil, fmt.Errorf("cronback: parse job created_at %q: %w", createdRaw, err)
	}
	updatedAt, err := time.Parse(time.RFC3339, updatedRaw)
	if err != nil {
		return nil, fmt.Errorf("cronback: parse job updated_at %q: %w", updatedRaw, err)
	}
	j.SourceItems, j.TargetConfig, j.CreatedAt, j.UpdatedAt =
		items, target, createdAt, updatedAt
	return &j, nil
}

// isConstraintErr 判定 SQLite 约束冲突（主码 SQLITE_CONSTRAINT=19；扩展码
// 如 2067=CONSTRAINT_UNIQUE 取低 8 位主码判定——本模块仅 name 唯一与
// idx_run_running 部分唯一两处可触发，哨兵区分由调用方语境决定）。
func isConstraintErr(err error) bool {
	var se *sqlite.Error
	if errors.As(err, &se) {
		return se.Code()&0xFF == 19 // SQLITE_CONSTRAINT 主码
	}
	return false
}
