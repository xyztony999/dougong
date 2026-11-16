package cronback

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/dougong/dougong/internal/adapter/backupx"
	"github.com/dougong/dougong/internal/adapter/cronx"
	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	storecron "github.com/dougong/dougong/internal/store/cronback"
)

// Service 是 cron-backup 应用服务门面（cron 线 + 备份线 + 执行编排）。
type Service struct {
	tab         CronTabGateway
	logs        CronLogSource
	runner      JobRunner
	exec        BackupExecutor
	store       StoreAPI
	audit       AuditRecorder
	instArchive InstanceArchiveAPI
	dataDir     string
	clock       func() time.Time

	queue chan workItem // 执行队列（A-C9；worker 由 main 启动消费）
}

// workItem 是执行队列项（job 定义快照——运行中删除不中断执行，A-C10）。
type workItem struct {
	runID     int64
	startedAt time.Time
	spec      backupx.JobSpec
}

// New 构造服务（装配：cronx 三网关 + backupx 执行器 + store + audit + 实例
// 档案 OHS；dataDir=面板数据目录，本地目标防递归判定基准 D-06）。
func New(tab CronTabGateway, logs CronLogSource, runner JobRunner, exec BackupExecutor,
	store StoreAPI, auditRec AuditRecorder, instArchive InstanceArchiveAPI, dataDir string) *Service {
	return &Service{
		tab: tab, logs: logs, runner: runner, exec: exec,
		store: store, audit: auditRec, instArchive: instArchive,
		dataDir: dataDir, clock: time.Now,
		queue: make(chan workItem, QueueCapacity),
	}
}

// ---- cron_job 线（§4.3.4；零台账——crontab 实况为唯一事实源）----

// ListCronJobs GET /cron/job/list（行序如实呈现 D-02）。
func (s *Service) ListCronJobs(ctx context.Context) (*CronJobListData, error) {
	proj, err := s.tab.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("cronback: list crontab: %w", err)
	}
	items := make([]CronJobItem, 0, len(proj.Entries))
	for _, e := range proj.Entries {
		items = append(items, CronJobItem{
			JobKey: e.JobKey, Expr: e.Expr, Command: e.Command,
			Remark: e.Remark, Source: e.Source,
		})
	}
	return &CronJobListData{Items: items, SystemEntries: proj.SystemEntries}, nil
}

// CreateCronJob POST /cron/job/create：宏归一化（D-03）+ 标记注释写入（D-02）。
func (s *Service) CreateCronJob(ctx context.Context, actor *authsvc.Identity, req *CronJobCreateReq) (*JobKeyData, error) {
	normExpr, ec := validateCronJobInput(req.Expr, req.Command)
	if ec != nil {
		return nil, ec
	}
	jobKey, err := s.tab.Upsert(ctx, "", cronx.JobInput{Expr: normExpr, Command: req.Command, Remark: req.Remark})
	if err != nil {
		return nil, translateCronTabErr(err)
	}
	s.recordAudit(ctx, auditEntry(actor, ActionCronJobCreated, ObjectTypeCronJob, jobKey,
		map[string]string{"expr": normExpr, "command": briefOf(req.Command)}, true))
	return &JobKeyData{JobKey: jobKey}, nil
}

// UpdateCronJob POST /cron/job/update：删旧写新（jobKey 随内容变更，D-01）。
func (s *Service) UpdateCronJob(ctx context.Context, actor *authsvc.Identity, req *CronJobUpdateReq) (*JobKeyData, error) {
	normExpr, ec := validateCronJobInput(req.Expr, req.Command)
	if ec != nil {
		return nil, ec
	}
	newKey, err := s.tab.Upsert(ctx, req.JobKey, cronx.JobInput{Expr: normExpr, Command: req.Command, Remark: req.Remark})
	if err != nil {
		return nil, translateCronTabErr(err)
	}
	s.recordAudit(ctx, auditEntry(actor, ActionCronJobUpdated, ObjectTypeCronJob, newKey,
		map[string]string{"expr": normExpr, "command": briefOf(req.Command)}, true))
	return &JobKeyData{JobKey: newKey}, nil
}

// DeleteCronJob POST /cron/job/delete：删任务行+紧邻标记注释。
func (s *Service) DeleteCronJob(ctx context.Context, actor *authsvc.Identity, req *CronJobDeleteReq) error {
	if err := s.tab.Delete(ctx, req.JobKey); err != nil {
		return translateCronTabErr(err)
	}
	s.recordAudit(ctx, auditEntry(actor, ActionCronJobDeleted, ObjectTypeCronJob, req.JobKey,
		map[string]string{"jobKey": briefOf(req.JobKey)}, true))
	return nil
}

// ValidateCronExpr POST /cron/job/validate：纯计算无副作用（D-03）。
func (s *Service) ValidateCronExpr(ctx context.Context, req *CronExprValidateReq) (*CronExprValidateData, error) {
	_, errs := ParseCronExpr(req.Expr)
	if len(errs) > 0 {
		return nil, errcode.ErrCronExprInvalid.WithData(map[string]any{"errors": errs})
	}
	return &CronExprValidateData{NextFires: PreviewFireStrs(req.Expr, s.clock(), PreviewCount)}, nil
}

// TriggerCronJob POST /cron/job/trigger：同步等待代执行（D-05；I-08 命令原文
// 取自 crontab 不经请求）；失败/超时为业务结果非请求错误（200+code=0）。
func (s *Service) TriggerCronJob(ctx context.Context, actor *authsvc.Identity, req *CronJobTriggerReq) (*CronTriggerData, error) {
	proj, err := s.tab.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("cronback: list crontab: %w", err)
	}
	var command string
	found := false
	for _, e := range proj.Entries {
		if e.JobKey == req.JobKey {
			command, found = e.Command, true
			break
		}
	}
	if !found {
		return nil, errcode.ErrCronJobNotFound
	}
	res := s.runner.Run(ctx, command, TriggerTimeout)
	success := res.ExitCode != nil && *res.ExitCode == 0 && !res.TimedOut
	s.recordAudit(ctx, auditEntry(actor, ActionCronJobTriggered, ObjectTypeCronJob, req.JobKey,
		map[string]any{
			"exitCode": exitCodeOf(res.ExitCode), "timedOut": res.TimedOut,
			"durationMs": res.DurationMs, "outputBrief": briefOf(res.Output),
		}, success))
	return &res, nil
}

// TailCronLog GET /cron/log/tail：纯文本尾部（D-04）。
func (s *Service) TailCronLog(ctx context.Context, linesParam string) (string, error) {
	text, err := s.logs.Tail(ctx, NormalizeLogLines(linesParam))
	if err != nil {
		if errors.Is(err, cronx.ErrNoLogSource) {
			return "", errcode.ErrInternal.WithData(map[string]string{
				"reason": "cron 日志来源不可用（journalctl / /var/log/syslog / /var/log/cron 均缺失）"})
		}
		return "", fmt.Errorf("cronback: tail cron log: %w", err)
	}
	return text, nil
}

// ---- 备份任务 CRUD（§4.1.4；SQLite 域内变更与审计同事务 R-03）----

// CreateBackupJob POST /cron/backup-job/create（四要素校验 → I-06 唯一 → 落库+审计同事务）。
func (s *Service) CreateBackupJob(ctx context.Context, actor *authsvc.Identity, req *BackupJobReq) (*BackupJobIDData, error) {
	job, ec := s.validateJobDef(ctx, req)
	if ec != nil {
		return nil, ec
	}
	now := s.clock()
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("cronback: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	id, err := tx.Job().Create(ctx, job, now)
	if err != nil {
		return nil, translateJobErr(err)
	}
	if err := tx.Audit().Append(ctx, auditEntry(actor, ActionBackupJobCreated,
		ObjectTypeBackupJob, fmt.Sprintf("%d", id),
		map[string]any{"name": job.Name, "targetType": job.TargetType}, true), now); err != nil {
		return nil, fmt.Errorf("cronback: audit backup_job_created: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cronback: commit: %w", err)
	}
	return &BackupJobIDData{ID: id}, nil
}

// UpdateBackupJob POST /cron/backup-job/update：整体替换（值对象替换语义）。
func (s *Service) UpdateBackupJob(ctx context.Context, actor *authsvc.Identity, req *BackupJobReq) error {
	job, ec := s.validateJobDef(ctx, req)
	if ec != nil {
		return ec
	}
	job.ID = req.JobID
	now := s.clock()
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("cronback: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := tx.Job().Update(ctx, job, now); err != nil {
		return translateJobErr(err)
	}
	if err := tx.Audit().Append(ctx, auditEntry(actor, ActionBackupJobUpdated,
		ObjectTypeBackupJob, fmt.Sprintf("%d", job.ID),
		map[string]any{"name": job.Name}, true), now); err != nil {
		return fmt.Errorf("cronback: audit backup_job_updated: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cronback: commit: %w", err)
	}
	return nil
}

// DeleteBackupJob POST /cron/backup-job/delete：仅删 job 行（run/产物不级联
// FR-09；运行中删除→执行继续完成 A-C10）。
func (s *Service) DeleteBackupJob(ctx context.Context, actor *authsvc.Identity, req *BackupJobIDReq) error {
	now := s.clock()
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("cronback: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	job, err := tx.Job().GetByID(ctx, req.JobID)
	if err != nil {
		return fmt.Errorf("cronback: load job: %w", err)
	}
	if job == nil {
		return errcode.ErrBackupJobNotFound
	}
	if err := tx.Job().Delete(ctx, req.JobID); err != nil {
		return fmt.Errorf("cronback: delete job: %w", err)
	}
	if err := tx.Audit().Append(ctx, auditEntry(actor, ActionBackupJobDeleted,
		ObjectTypeBackupJob, fmt.Sprintf("%d", req.JobID),
		map[string]string{"name": job.Name}, true), now); err != nil {
		return fmt.Errorf("cronback: audit backup_job_deleted: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cronback: commit: %w", err)
	}
	return nil
}

// translateCronTabErr 网关错误翻译（CB_003 面板外已删；其余 wrap 原样——
// response 兜底 INTERNAL）。
func translateCronTabErr(err error) error {
	if errors.Is(err, cronx.ErrJobNotFound) {
		return errcode.ErrCronJobNotFound
	}
	return fmt.Errorf("cronback: crontab gateway: %w", err)
}

// translateJobErr 任务仓储错误翻译（唯一冲突 CB_005；行缺失 CB_004）。
func translateJobErr(err error) error {
	switch {
	case errors.Is(err, storecron.ErrJobNameTaken):
		return errcode.ErrBackupJobNameTaken
	case errors.Is(err, sql.ErrNoRows):
		return errcode.ErrBackupJobNotFound
	default:
		return fmt.Errorf("cronback: job repo: %w", err)
	}
}

// recordAudit 独立事务审计写入（crontab/命令/传输类动作紧随写入，R-03；
// 失败重试一次仍失败降级运行日志——dbmgmt/sitemgmt 同法 D-13）。
func (s *Service) recordAudit(ctx context.Context, e audit.Entry) {
	if err := s.audit.Record(ctx, e); err != nil {
		if err := s.audit.Record(ctx, e); err != nil {
			slog.WarnContext(ctx, "cronback: audit record degraded to runtime log",
				"action", e.Action, "object", e.ObjectID, "result", e.Result, "err", err)
		}
	}
}

// auditEntry 组装四要素载荷（R-03；dbmgmt auditEntry 同构）。
func auditEntry(actor *authsvc.Identity, action, objectType, objectID string, detail any, success bool) audit.Entry {
	result := audit.ResultSuccess
	if !success {
		result = audit.ResultFailure
	}
	e := audit.Entry{
		Category: audit.CategoryChange,
		Action:   action, ObjectType: objectType, ObjectID: objectID,
		Detail: detail, Result: result, Source: audit.SourceWeb,
	}
	if actor != nil {
		uid := actor.UserID
		e.ActorID = &uid
		e.ActorName = actor.Username
	} else {
		e.ActorName = "unknown"
	}
	return e
}

// briefOf 审计输出/命令摘要（截断防审计行膨胀）。
func briefOf(s string) string {
	runes := []rune(s)
	if len(runes) <= 120 {
		return s
	}
	return string(runes[:120]) + "…"
}

// exitCodeOf 审计 detail 的退出码可空形态。
func exitCodeOf(code *int) any {
	if code == nil {
		return nil
	}
	return *code
}
