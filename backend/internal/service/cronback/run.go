package cronback

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/dougong/dougong/internal/adapter/backupx"
	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	storecron "github.com/dougong/dougong/internal/store/cronback"
)

// run.go 是 backup_run 编排：立即执行（D-07 异步提交）/执行 worker（A-C9）/
// 历史检索（D-11/D-12）/详情/产物下载（D-10/D-16）/启动恢复（D-08）。

// ListBackupJobs GET /cron/backup-job/list（含最近执行结论读时派生 D-11 与
// nextFireAt 调度器推算）。
func (s *Service) ListBackupJobs(ctx context.Context) (*BackupJobListData, error) {
	jobs, err := s.store.Job().List(ctx)
	if err != nil {
		return nil, fmt.Errorf("cronback: list jobs: %w", err)
	}
	now := s.clock()
	items := make([]BackupJobItem, 0, len(jobs))
	for _, j := range jobs {
		item := BackupJobItem{
			ID: j.ID, Name: j.Name, SourceSummary: s.toSourceDTOs(ctx, j.SourceItems),
			TargetType: j.TargetType, TargetBrief: targetBrief(j),
			RetentionCount: j.RetentionCount, ScheduleCron: j.ScheduleCron,
		}
		if next := NextFireStr(j.ScheduleCron, now); next != "" {
			item.NextFireAt = &next
		}
		last, err := s.store.Run().LatestByJob(ctx, j.ID)
		if err != nil {
			return nil, fmt.Errorf("cronback: latest run of job %d: %w", j.ID, err)
		}
		if last != nil {
			item.LastRun = &LastRunSummary{
				ID: last.ID, Status: last.Status, Trigger: last.Trigger,
				StartedAt: fmtTime(last.StartedAt), FinishedAt: fmtTimePtr(last.FinishedAt),
			}
		}
		items = append(items, item)
	}
	return &BackupJobListData{Items: items}, nil
}

// RunBackupJob POST /cron/backup-job/run：互斥（I-01 由部分唯一索引承载——
// 竞态不可能）→ INSERT run(running, manual) + audit 同事务（A-C3）→ 入队
// 异步执行 → 立即返回 runId（D-07）。
func (s *Service) RunBackupJob(ctx context.Context, actor *authsvc.Identity, jobID int64) (*BackupRunIDData, error) {
	job, err := s.store.Job().GetByID(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("cronback: load job: %w", err)
	}
	if job == nil {
		return nil, errcode.ErrBackupJobNotFound
	}
	now := s.clock()
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("cronback: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	runID, err := tx.Run().Create(ctx, job.ID, job.Name, storecron.TriggerManual, now)
	if err != nil {
		if errors.Is(err, storecron.ErrRunMutex) {
			return nil, errcode.ErrBackupJobRunning
		}
		return nil, fmt.Errorf("cronback: create run: %w", err)
	}
	if err := tx.Audit().Append(ctx, auditEntry(actor, ActionBackupRunTriggered,
		ObjectTypeBackupRun, fmt.Sprintf("%d", runID),
		map[string]any{"jobId": job.ID, "jobName": job.Name, "trigger": storecron.TriggerManual},
		true), now); err != nil {
		return nil, fmt.Errorf("cronback: audit backup_run_triggered: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cronback: commit: %w", err)
	}
	if !s.enqueue(workItem{runID: runID, startedAt: now, spec: s.jobSpecOf(job)}) {
		// 队列满（A-C9）：如实置失败——run 是唯一事实载体。
		s.failRunDirect(ctx, runID, now, "执行队列已满，请稍后重试")
	}
	return &BackupRunIDData{RunID: runID}, nil
}

// enqueue 非阻塞入队（队列满返回 false）。
func (s *Service) enqueue(it workItem) bool {
	select {
	case s.queue <- it:
		return true
	default:
		return false
	}
}

// failRunDirect 直接置 run 失败（入队失败的补偿路径；孤儿恢复同语义兜底）。
func (s *Service) failRunDirect(ctx context.Context, runID int64, now time.Time, msg string) {
	n, err := s.store.Run().FinishFailure(ctx, runID, now, msg)
	if err != nil {
		slog.ErrorContext(ctx, "cronback: fail run direct", "runId", runID, "err", err)
		return
	}
	if n == 0 {
		slog.WarnContext(ctx, "cronback: run already terminal on direct fail", "runId", runID)
	}
}

// ListBackupRuns GET /cron/backup-run/list（筛选+公共分页，id 倒序 D-11）。
func (s *Service) ListBackupRuns(ctx context.Context, q RunQuery) (*BackupRunListData, error) {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = 20
	}
	if q.PageSize > 100 {
		q.PageSize = 100
	}
	filter := storecron.RunFilter{Status: q.Status, Trigger: q.Trigger}
	if q.JobID > 0 {
		filter.JobID = &q.JobID
	}
	if q.From != "" {
		t, err := time.Parse(time.RFC3339, q.From)
		if err != nil {
			return nil, errcode.ErrCBValidation.WithFields(
				errcode.FieldError{Field: "from", Message: "from 须为 RFC3339 时间"})
		}
		filter.From = &t
	}
	if q.To != "" {
		t, err := time.Parse(time.RFC3339, q.To)
		if err != nil {
			return nil, errcode.ErrCBValidation.WithFields(
				errcode.FieldError{Field: "to", Message: "to 须为 RFC3339 时间"})
		}
		filter.To = &t
	}
	if filter.Status != "" && filter.Status != storecron.StatusRunning &&
		filter.Status != storecron.StatusSuccess && filter.Status != storecron.StatusFailure {
		return nil, errcode.ErrCBValidation.WithFields(
			errcode.FieldError{Field: "status", Message: "status 非法（running|success|failure）"})
	}
	if filter.Trigger != "" && filter.Trigger != storecron.TriggerCron &&
		filter.Trigger != storecron.TriggerManual {
		return nil, errcode.ErrCBValidation.WithFields(
			errcode.FieldError{Field: "trigger", Message: "trigger 非法（cron|manual）"})
	}
	runs, total, err := s.store.Run().Search(ctx, filter, (q.Page-1)*q.PageSize, q.PageSize)
	if err != nil {
		return nil, fmt.Errorf("cronback: search runs: %w", err)
	}
	items := make([]BackupRunItem, 0, len(runs))
	for _, r := range runs {
		items = append(items, BackupRunItem{
			ID: r.ID, JobID: r.JobID, JobName: r.JobName, Trigger: r.Trigger,
			Status: r.Status, StartedAt: fmtTime(r.StartedAt), FinishedAt: fmtTimePtr(r.FinishedAt),
			DurationMs:   durationMs(r.StartedAt, r.FinishedAt),
			ArtifactSize: r.ArtifactSize, ErrorBrief: briefPtr(r.ErrorMessage, ErrorBriefLength),
		})
	}
	return &BackupRunListData{Items: items, Total: total}, nil
}

// BackupRunDetail GET /cron/backup-run/detail（CB_007；sources 来自 manifest
// 语义 A-C5——success+local 实读，running/failure/rsync 为 null）。
func (s *Service) BackupRunDetail(ctx context.Context, runID int64) (*BackupRunDetail, error) {
	run, err := s.store.Run().GetByID(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("cronback: load run: %w", err)
	}
	if run == nil {
		return nil, errcode.ErrBackupRunNotFound
	}
	detail := &BackupRunDetail{
		ID: run.ID, JobID: run.JobID, JobName: run.JobName, Trigger: run.Trigger,
		Status: run.Status, StartedAt: fmtTime(run.StartedAt), FinishedAt: fmtTimePtr(run.FinishedAt),
		DurationMs:   durationMs(run.StartedAt, run.FinishedAt),
		ArtifactPath: run.ArtifactPath, ArtifactSize: run.ArtifactSize,
		Files: run.Files, ErrorMessage: run.ErrorMessage,
	}
	if run.Status == storecron.StatusSuccess && run.ArtifactPath != nil {
		port := s.jobPortHint(ctx, run.JobID)
		m, merr := s.exec.ReadManifestPath(ctx, *run.ArtifactPath, port)
		switch {
		case merr == nil && m != nil:
			detail.Sources = m.Sources
		case errors.Is(merr, backupx.ErrRemoteManifest):
			// rsync 目标 MVP 不拉流（A-C5）：files 清单已完整呈现。
		case merr == nil:
			// manifest 为空（不可达防御）：如实置 null。
		default:
			slog.WarnContext(ctx, "cronback: read manifest", "runId", run.ID, "err", merr)
		}
	}
	return detail, nil
}

// DownloadArtifact GET /cron/backup-run/download 校验链（CB_007 → CB_008 →
// file ∈ files（否则 CB_001）→ 文件存在 CB_009）；全过才开流（D-16 流式响应
// 账同判）。审计 backup_artifact_downloaded（security，数据取出口径）。
func (s *Service) DownloadArtifact(ctx context.Context, actor *authsvc.Identity, runID int64, file string) (*DownloadTarget, error) {
	run, err := s.store.Run().GetByID(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("cronback: load run: %w", err)
	}
	if run == nil {
		return nil, errcode.ErrBackupRunNotFound
	}
	if run.Status != storecron.StatusSuccess || run.ArtifactPath == nil {
		return nil, errcode.ErrRunNotDownloadable
	}
	if file == "" || !inFiles(run.Files, file) {
		return nil, errcode.ErrCBValidation.WithFields(
			errcode.FieldError{Field: "file", Message: "file 必须为产物清单内的文件名"})
	}
	port := s.jobPortHint(ctx, run.JobID)
	local, cleanup, err := s.exec.PullFile(ctx, *run.ArtifactPath, file, port)
	if err != nil {
		if errors.Is(err, backupx.ErrArtifactMissing) {
			return nil, errcode.ErrArtifactMissing
		}
		return nil, fmt.Errorf("cronback: pull artifact: %w", err)
	}
	if cleanup == nil {
		cleanup = func() {}
	}
	st, err := statSize(local)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("cronback: stat artifact: %w", err)
	}
	s.recordAudit(ctx, downloadAuditEntry(actor, run.ID, file, st))
	return &DownloadTarget{Name: file, Size: st, Path: local, Cleanup: cleanup}, nil
}

// jobPortHint 已删/未知任务的 rsync 产物按默认端口 22 拉回（A-C14：非默认
// 端口任务的 job 删除后下载将以连接错误如实呈现）。
func (s *Service) jobPortHint(ctx context.Context, jobID int64) int {
	job, err := s.store.Job().GetByID(ctx, jobID)
	if err != nil || job == nil {
		return 0
	}
	if job.TargetType == storecron.TargetRsync {
		return job.TargetConfig.Port
	}
	return 0
}

// downloadAuditEntry 下载审计（category=security——数据取出口径对齐
// file-manager file_downloaded 先例，§4.2.4）。
func downloadAuditEntry(actor *authsvc.Identity, runID int64, file string, size int64) audit.Entry {
	e := auditEntry(actor, ActionBackupArtifactDownloaded, ObjectTypeBackupRun,
		fmt.Sprintf("%d", runID),
		map[string]any{"runId": runID, "file": file, "sizeBytes": size}, true)
	e.Category = audit.CategorySecurity
	return e
}

// RunExecutor 是执行 worker 主循环（main 编排启动，A-C9；ctx 取消退出——
// G-06；单条执行 panic 由 worker 隔离）。
func (s *Service) RunExecutor(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case it := <-s.queue:
			s.runOneSafely(ctx, it)
		}
	}
}

// runOneSafely 单条执行 panic 隔离（backend-03 §5）。
func (s *Service) runOneSafely(ctx context.Context, it workItem) {
	defer func() {
		if r := recover(); r != nil {
			slog.ErrorContext(ctx, "cronback: executor panic", "runId", it.runID, "panic", r)
			s.failRunDirect(context.Background(), it.runID, s.clock(), "执行器内部错误（panic 已隔离）")
		}
	}()
	if err := s.executeRun(ctx, it); err != nil {
		slog.ErrorContext(ctx, "cronback: execute run", "runId", it.runID, "err", err)
	}
}

// executeRun 执行编排落库段（2.3）：BackupExecutor 逐源执行 → 同事务（终态
// 回写 + 轮转 DELETE + audit backup_rotated〔成功且确有删除时〕，D-09）→
// 事务后产物清理（best-effort，A-C4）。
func (s *Service) executeRun(ctx context.Context, it workItem) error {
	res, execErr := s.exec.Execute(ctx, it.spec, it.startedAt)
	finishedAt := s.clock()
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if execErr != nil {
		if _, ferr := tx.Run().FinishFailure(ctx, it.runID, finishedAt, execErr.Error()); ferr != nil {
			return fmt.Errorf("finish failure: %w", ferr)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit failure: %w", err)
		}
		slog.WarnContext(ctx, "cronback: backup run failed",
			"runId", it.runID, "jobId", it.spec.JobID, "err", execErr.Error())
		return nil
	}
	if _, err := tx.Run().FinishSuccess(ctx, it.runID, finishedAt,
		res.ArtifactPath, res.ArtifactSize, toStoreFiles(res.Files)); err != nil {
		return fmt.Errorf("finish success: %w", err)
	}
	victims, err := tx.Run().Rotate(ctx, it.spec.JobID, it.spec.RetentionCount)
	if err != nil {
		return fmt.Errorf("rotate: %w", err)
	}
	if len(victims) > 0 { // 删除真实数据须留痕（D-09：system 审计 backup_rotated）
		if err := tx.Audit().Append(ctx, systemAuditEntry(ActionBackupRotated,
			ObjectTypeBackupRun, fmt.Sprintf("%d", it.spec.JobID),
			map[string]any{
				"jobId": it.spec.JobID, "jobName": it.spec.Name,
				"deletedRuns": len(victims), "deletedIds": victimIDs(victims),
			}, finishedAt), finishedAt); err != nil {
			return fmt.Errorf("audit backup_rotated: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit success: %w", err)
	}
	s.cleanupArtifacts(ctx, victims)
	return nil
}

// cleanupArtifacts 事务后产物清理（D-09/A-C4：失败 slog 记录、孤儿留存，
// 不阻塞新执行）。
func (s *Service) cleanupArtifacts(ctx context.Context, victims []storecron.RotateVictim) {
	for _, v := range victims {
		if v.ArtifactPath == "" {
			continue
		}
		if err := s.exec.RemoveArtifactPath(ctx, v.ArtifactPath, 0); err != nil {
			slog.WarnContext(ctx, "cronback: artifact cleanup deferred (orphan kept)",
				"path", v.ArtifactPath, "err", err)
		}
	}
}

// RecoverOrphans 启动恢复编排（2.3/D-08）：进程启动时全部 running 必为孤儿
// （执行器随进程消亡）→ 置 failure + system 审计（A-C13 backup_run_interrupted）。
func (s *Service) RecoverOrphans(ctx context.Context) (int64, error) {
	now := s.clock()
	n, err := s.store.Run().MarkOrphanFailed(ctx, now, OrphanRunMessage)
	if err != nil {
		return 0, fmt.Errorf("cronback: mark orphan failed: %w", err)
	}
	if n > 0 {
		s.recordAudit(ctx, systemAuditEntry(ActionBackupRunInterrupted,
			ObjectTypeBackupRun, "all",
			map[string]any{"count": n, "message": OrphanRunMessage}, now))
	}
	return n, nil
}

// systemAuditEntry system 来源审计（调度侧/恢复侧）。
func systemAuditEntry(action, objectType, objectID string, detail any, at time.Time) audit.Entry {
	return audit.Entry{
		ActorName: "system", Category: audit.CategoryChange,
		Action: action, ObjectType: objectType, ObjectID: objectID,
		Detail: detail, Result: audit.ResultSuccess, Source: audit.SourceSystem,
	}
}

// ---- 装配辅助 ----

// jobSpecOf job 定义 → 执行快照（site 项原样；database 项补实例显示名）。
func (s *Service) jobSpecOf(j *storecron.Job) backupx.JobSpec {
	spec := backupx.JobSpec{
		JobID: j.ID, Name: j.Name,
		Target: backupx.TargetSpec{
			Type: j.TargetType, Path: j.TargetConfig.Path,
			Host: j.TargetConfig.Host, User: j.TargetConfig.User, Port: j.TargetConfig.Port,
		},
		RetentionCount: j.RetentionCount,
	}
	ctx := context.Background()
	for _, it := range j.SourceItems {
		ss := backupx.SourceSpec{
			Kind: it.Type, Path: it.Path,
			InstanceID: it.InstanceID, Database: it.Database,
		}
		switch it.Type {
		case storecron.SourceSiteDir:
			ss.Ref = backupx.SourceRef(it.Path)
		case storecron.SourceDatabase:
			name := "#" + strconv.FormatInt(it.InstanceID, 10)
			if s.instArchive != nil {
				if found, _, n, err := s.instArchive.InstanceArchive(ctx, it.InstanceID); err == nil && found {
					name = n
				}
			}
			ss.InstanceName, ss.Ref = name, backupx.DBRef(name, it.Database)
		}
		spec.Sources = append(spec.Sources, ss)
	}
	return spec
}

// toSourceDTOs 源清单值集合 → 传输形态（含契约 ref 投影，E1）：site_dir→
// 目录路径；database→「实例名·库名」——实例名经档案 OHS 解析（hostPortLabel
// 显示名）；悬空（已删实例，D-12 弱引用合法态）→「#<id>」前缀如实可辨。
func (s *Service) toSourceDTOs(ctx context.Context, items []storecron.SourceItem) []SourceItemDTO {
	out := make([]SourceItemDTO, 0, len(items))
	for _, it := range items {
		dto := SourceItemDTO{
			Type: it.Type, Path: it.Path,
			InstanceID: it.InstanceID, Database: it.Database,
		}
		switch it.Type {
		case storecron.SourceSiteDir:
			dto.Ref = backupx.SourceRef(it.Path)
		case storecron.SourceDatabase:
			dto.Ref = backupx.DBRef(s.instanceLabel(ctx, it.InstanceID), it.Database)
		}
		out = append(out, dto)
	}
	return out
}

// instanceLabel 解析实例显示名（档案 OHS；悬空/异常→#<id>，E1）。
func (s *Service) instanceLabel(ctx context.Context, instanceID int64) string {
	if s.instArchive != nil {
		if found, _, name, err := s.instArchive.InstanceArchive(ctx, instanceID); err == nil && found {
			return name
		}
	}
	return "#" + strconv.FormatInt(instanceID, 10)
}

// targetBrief 目标简述（local=路径；rsync=[user@]host:path）。
func targetBrief(j storecron.Job) string {
	if j.TargetType == storecron.TargetLocal {
		return j.TargetConfig.Path
	}
	host := j.TargetConfig.Host
	if j.TargetConfig.User != "" {
		host = j.TargetConfig.User + "@" + host
	}
	return host + ":" + j.TargetConfig.Path
}

// toStoreFiles 执行结果清单 → 存储形态。
func toStoreFiles(files []backupx.FileEntry) []storecron.FileEntry {
	out := make([]storecron.FileEntry, 0, len(files))
	for _, f := range files {
		out = append(out, storecron.FileEntry{Name: f.Name, Size: f.Size})
	}
	return out
}

// inFiles 判定 file 是否在产物清单（裸名精确匹配——manifest 名即枚举依据）。
func inFiles(files []storecron.FileEntry, name string) bool {
	for _, f := range files {
		if f.Name == name {
			return true
		}
	}
	return false
}

// briefPtr 可空摘要截断（rune 级）。
func briefPtr(s *string, n int) *string {
	if s == nil {
		return nil
	}
	runes := []rune(*s)
	if len(runes) <= n {
		return s
	}
	brief := string(runes[:n]) + "…"
	return &brief
}

// victimIds 轮转删除 id 清单（审计 detail）。
func victimIDs(victims []storecron.RotateVictim) []int64 {
	ids := make([]int64, 0, len(victims))
	for _, v := range victims {
		ids = append(ids, v.ID)
	}
	return ids
}
