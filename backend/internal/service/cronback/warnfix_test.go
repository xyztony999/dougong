package cronback

// warnfix_test.go 是 Gate 2 WARN 修复轮（2026-11-16）W1 补测：service/cronback
// 覆盖率 72.8% → 80% 目标。补齐复检轮 0% 函数（ListBackupRuns/DeleteBackupJob/
// TailCronLog/RunExecutor/runOneSafely/failRunDirect/briefPtr/RunScheduler）与
// 低覆盖分支（targetBrief rsync 形态、jobPortHint rsync/已删任务、instanceLabel
// 悬空/档案异常、recordAudit 重试降级+actor 装配、executeRun/CRUD 各错误分支、
// scheduleTick 防御路径、fireCronRun 队列满与仓储错误、NextFireStr 无命中、
// statSize 失败、detail manifest 三分支、错误翻译 wrap 分支）。

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/adapter/backupx"
	"github.com/dougong/dougong/internal/adapter/cronx"
	auditsvc "github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	storeauth "github.com/dougong/dougong/internal/store/auth"
	storecron "github.com/dougong/dougong/internal/store/cronback"
)

// ---- 修复轮补充替身 ----

// failLogs 可预置错误的日志源（TailCronLog 通道三分支）。
type failLogs struct{ err error }

func (f failLogs) Tail(context.Context, int) (string, error) { return "", f.err }

// panicExec Execute 必 panic（runOneSafely panic 隔离路径）；其余通道转发内层。
type panicExec struct{ inner BackupExecutor }

func (p *panicExec) Execute(context.Context, backupx.JobSpec, time.Time) (backupx.ExecResult, error) {
	panic("executor boom")
}
func (p *panicExec) PullFile(ctx context.Context, path, name string, port int) (string, func(), error) {
	return p.inner.PullFile(ctx, path, name, port)
}
func (p *panicExec) ReadManifestPath(ctx context.Context, path string, port int) (*backupx.Manifest, error) {
	return p.inner.ReadManifestPath(ctx, path, port)
}
func (p *panicExec) RemoveArtifactPath(ctx context.Context, path string, port int) error {
	return p.inner.RemoveArtifactPath(ctx, path, port)
}

// failAudit 恒失败审计替身（recordAudit 重试一次+降级日志路径）。
type failAudit struct{ calls int }

func (f *failAudit) Record(context.Context, auditsvc.Entry) error {
	f.calls++
	return errors.New("audit down")
}

// errArchive 恒异常实例档案（instanceLabel 异常分支）。
type errArchive struct{}

func (errArchive) InstanceArchive(context.Context, int64) (bool, string, string, error) {
	return false, "", "", errors.New("archive down")
}

// failJobRepo 注入故障的任务仓储（默认全部转发内层）。
type failJobRepo struct {
	JobRepoAPI
	listErr      error
	getByIDErr   error
	getByNameErr error
	createErr    error
	updateErr    error
	deleteErr    error
}

func (r *failJobRepo) List(ctx context.Context) ([]storecron.Job, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	return r.JobRepoAPI.List(ctx)
}
func (r *failJobRepo) GetByID(ctx context.Context, id int64) (*storecron.Job, error) {
	if r.getByIDErr != nil {
		return nil, r.getByIDErr
	}
	return r.JobRepoAPI.GetByID(ctx, id)
}
func (r *failJobRepo) GetByName(ctx context.Context, name string) (*storecron.Job, error) {
	if r.getByNameErr != nil {
		return nil, r.getByNameErr
	}
	return r.JobRepoAPI.GetByName(ctx, name)
}
func (r *failJobRepo) Create(ctx context.Context, j *storecron.Job, now time.Time) (int64, error) {
	if r.createErr != nil {
		return 0, r.createErr
	}
	return r.JobRepoAPI.Create(ctx, j, now)
}
func (r *failJobRepo) Update(ctx context.Context, j *storecron.Job, now time.Time) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	return r.JobRepoAPI.Update(ctx, j, now)
}
func (r *failJobRepo) Delete(ctx context.Context, id int64) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	return r.JobRepoAPI.Delete(ctx, id)
}

// failRunRepo 注入故障的执行历史仓储（默认全部转发内层）。
type failRunRepo struct {
	RunRepoAPI
	createErr  error
	finishSErr error
	finishFErr error
	rotateErr  error
	searchErr  error
	getByIDErr error
	latestErr  error
	orphanErr  error
}

func (r *failRunRepo) Create(ctx context.Context, jobID int64, name, trigger string, at time.Time) (int64, error) {
	if r.createErr != nil {
		return 0, r.createErr
	}
	return r.RunRepoAPI.Create(ctx, jobID, name, trigger, at)
}
func (r *failRunRepo) FinishSuccess(ctx context.Context, id int64, at time.Time, p string, sz int64, f []storecron.FileEntry) (int64, error) {
	if r.finishSErr != nil {
		return 0, r.finishSErr
	}
	return r.RunRepoAPI.FinishSuccess(ctx, id, at, p, sz, f)
}
func (r *failRunRepo) FinishFailure(ctx context.Context, id int64, at time.Time, msg string) (int64, error) {
	if r.finishFErr != nil {
		return 0, r.finishFErr
	}
	return r.RunRepoAPI.FinishFailure(ctx, id, at, msg)
}
func (r *failRunRepo) Rotate(ctx context.Context, jobID int64, retention int) ([]storecron.RotateVictim, error) {
	if r.rotateErr != nil {
		return nil, r.rotateErr
	}
	return r.RunRepoAPI.Rotate(ctx, jobID, retention)
}
func (r *failRunRepo) Search(ctx context.Context, f storecron.RunFilter, offset, limit int) ([]storecron.Run, int64, error) {
	if r.searchErr != nil {
		return nil, 0, r.searchErr
	}
	return r.RunRepoAPI.Search(ctx, f, offset, limit)
}
func (r *failRunRepo) GetByID(ctx context.Context, id int64) (*storecron.Run, error) {
	if r.getByIDErr != nil {
		return nil, r.getByIDErr
	}
	return r.RunRepoAPI.GetByID(ctx, id)
}
func (r *failRunRepo) LatestByJob(ctx context.Context, jobID int64) (*storecron.Run, error) {
	if r.latestErr != nil {
		return nil, r.latestErr
	}
	return r.RunRepoAPI.LatestByJob(ctx, jobID)
}
func (r *failRunRepo) MarkOrphanFailed(ctx context.Context, now time.Time, reason string) (int64, error) {
	if r.orphanErr != nil {
		return 0, r.orphanErr
	}
	return r.RunRepoAPI.MarkOrphanFailed(ctx, now, reason)
}

// failTx 注入故障的事务视图（Audit 为具体类型不可注入——该分支不在本装置面）。
type failTx struct {
	TxAPI
	job       *failJobRepo
	run       *failRunRepo
	commitErr error
}

func (t *failTx) Job() JobRepoAPI { return t.job }
func (t *failTx) Run() RunRepoAPI { return t.run }
func (t *failTx) Commit() error {
	if t.commitErr != nil {
		return t.commitErr
	}
	return t.TxAPI.Commit()
}

// stubTx 无真实连接的事务替身（池 MaxOpenConns(1) 下，服务在 BeginTx 前还需
// 走连接池查询的路径——如 RunBackupJob 事务前 GetByID——不能预占连接，用
// 纯 stub 注入事务内错误）。
type stubTx struct {
	run       RunRepoAPI
	job       JobRepoAPI
	commitErr error
}

func (s *stubTx) Job() JobRepoAPI             { return s.job }
func (s *stubTx) Run() RunRepoAPI             { return s.run }
func (s *stubTx) Audit() *storeauth.AuditRepo { return nil } // 注入路径不触 Audit
func (s *stubTx) Commit() error               { return s.commitErr }
func (s *stubTx) Rollback() error             { return nil }

// failStore 注入故障的 StoreAPI 聚合（BeginTx 级或仓储级）。
type failStore struct {
	StoreAPI
	beginErr   error
	tx         *failTx
	txOverride TxAPI // 原始事务替身直通（stubTx 场景）
	job        JobRepoAPI
	run        RunRepoAPI
}

func (f *failStore) BeginTx(ctx context.Context) (TxAPI, error) {
	if f.beginErr != nil {
		return nil, f.beginErr
	}
	if f.txOverride != nil {
		return f.txOverride, nil
	}
	if f.tx != nil {
		return f.tx, nil
	}
	return f.StoreAPI.BeginTx(ctx)
}
func (f *failStore) Job() JobRepoAPI {
	if f.job != nil {
		return f.job
	}
	return f.StoreAPI.Job()
}
func (f *failStore) Run() RunRepoAPI {
	if f.run != nil {
		return f.run
	}
	return f.StoreAPI.Run()
}

// withFailStore 用可注入故障的包装替换 env.svc.store（内层为真实仓储）。
func withFailStore(env *testEnv) *failStore {
	fs := &failStore{StoreAPI: AdaptStore(env.st)}
	env.svc.store = fs
	return fs
}

// failingTx 基于真实事务构造 failTx（单次使用：service 出错路径 defer 回滚）。
func failingTx(env *testEnv, t *testing.T) *failTx {
	t.Helper()
	real, err := env.st.BeginTx(context.Background())
	if err != nil {
		t.Fatalf("begin real tx: %v", err)
	}
	return &failTx{
		TxAPI: txAdapter{t: real},
		job:   &failJobRepo{JobRepoAPI: real.Job()},
		run:   &failRunRepo{RunRepoAPI: real.Run()},
	}
}

// waitForRunStatus 轮询等待 run 达到期望终态（worker 异步消费）。
func waitForRunStatus(t *testing.T, env *testEnv, runID int64, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r, err := env.st.Run().GetByID(context.Background(), runID)
		if err == nil && r != nil && r.Status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run %d 未在期限内达到 %s", runID, want)
}

// ---- W1 用例 ----

func TestListBackupRunsQueryAndBrief(t *testing.T) {
	env := newEnv(t, "/var/lib/dougong")
	ctx := context.Background()
	data, err := env.svc.CreateBackupJob(ctx, nil, validReq())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	now := time.Now().UTC()
	// 建序受 idx_run_running 互斥约束（I-01）：先建终态行，最后建 running。
	// failure（超长错误消息→briefPtr 截断，startedAt +1min）。
	rFail, _ := env.st.Run().Create(ctx, data.ID, "nightly", storecron.TriggerCron, now.Add(time.Minute))
	long := strings.Repeat("败", 150) // > ErrorBriefLength(120) rune
	if _, err := env.st.Run().FinishFailure(ctx, rFail, now.Add(2*time.Minute), long); err != nil {
		t.Fatalf("seed failure: %v", err)
	}
	// success（startedAt +3min）。
	rSucc, _ := env.st.Run().Create(ctx, data.ID, "nightly", storecron.TriggerManual, now.Add(3*time.Minute))
	if _, err := env.st.Run().FinishSuccess(ctx, rSucc, now.Add(4*time.Minute), "/bk/3", 5,
		[]storecron.FileEntry{{Name: "a.gz", Size: 5}}); err != nil {
		t.Fatalf("seed success: %v", err)
	}
	// running（无终态，startedAt +5min；id 最大→倒序首位）。
	rRun, err := env.st.Run().Create(ctx, data.ID, "nightly", storecron.TriggerManual, now.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("seed running: %v", err)
	}

	// 缺省兜底（page<1→1、pageSize<1→20、>100→100）+ id 倒序（D-11）。
	list, err := env.svc.ListBackupRuns(ctx, RunQuery{})
	if err != nil || len(list.Items) != 3 || list.Total != 3 {
		t.Fatalf("list: %v %+v", err, list)
	}
	if list.Items[0].ID != rRun || list.Items[2].ID != rFail {
		t.Fatalf("id 倒序不对: %+v", list.Items)
	}
	// running 行：finishedAt/durationMs/errorBrief 全 null（briefPtr nil 分支）。
	running := list.Items[0]
	if running.FinishedAt != nil || running.DurationMs != nil || running.ErrorBrief != nil {
		t.Fatalf("running 行可空字段须 null: %+v", running)
	}
	// failure 行：errorBrief 截断 120 rune + 省略号。
	failure := list.Items[2]
	if failure.ErrorBrief == nil || *failure.ErrorBrief != string([]rune(long)[:120])+"…" {
		t.Fatalf("errorBrief 截断不对: %v", failure.ErrorBrief)
	}
	// pageSize 越界钳制（>100→100，不报错）。
	if _, err := env.svc.ListBackupRuns(ctx, RunQuery{PageSize: 500}); err != nil {
		t.Fatalf("pageSize clamp: %v", err)
	}

	// 筛选：status/trigger/jobId/from-to。
	lf, _ := env.svc.ListBackupRuns(ctx, RunQuery{Status: storecron.StatusFailure})
	if len(lf.Items) != 1 || lf.Items[0].ID != rFail {
		t.Fatalf("status filter: %+v", lf.Items)
	}
	lm, _ := env.svc.ListBackupRuns(ctx, RunQuery{Trigger: storecron.TriggerManual})
	if len(lm.Items) != 2 {
		t.Fatalf("trigger filter: %+v", lm.Items)
	}
	lj, _ := env.svc.ListBackupRuns(ctx, RunQuery{JobID: 999})
	if len(lj.Items) != 0 || lj.Total != 0 {
		t.Fatalf("jobId filter empty: %+v", lj)
	}
	lw, err := env.svc.ListBackupRuns(ctx, RunQuery{
		From: now.Add(90 * time.Second).Format(time.RFC3339),
		To:   now.Add(6 * time.Minute).Format(time.RFC3339)})
	if err != nil || len(lw.Items) != 2 { // rSucc(+3min)、rRun(+5min) 在窗内，rFail(+1min) 窗外
		t.Fatalf("from/to filter: %v %+v", err, lw.Items)
	}

	// 校验失败分支：from/to 非 RFC3339、status/trigger 非法 → CB_001。
	if _, err := env.svc.ListBackupRuns(ctx, RunQuery{From: "yesterday"}); mustEC(t, err).Symbol() != "CB_001" {
		t.Fatal("bad from must be CB_001")
	}
	if _, err := env.svc.ListBackupRuns(ctx, RunQuery{To: "soon"}); mustEC(t, err).Symbol() != "CB_001" {
		t.Fatal("bad to must be CB_001")
	}
	if _, err := env.svc.ListBackupRuns(ctx, RunQuery{Status: "paused"}); mustEC(t, err).Symbol() != "CB_001" {
		t.Fatal("bad status must be CB_001")
	}
	if _, err := env.svc.ListBackupRuns(ctx, RunQuery{Trigger: "api"}); mustEC(t, err).Symbol() != "CB_001" {
		t.Fatal("bad trigger must be CB_001")
	}

	// Search 仓储错误 → wrap（非 errcode）。
	fs := withFailStore(env)
	fs.run = &failRunRepo{RunRepoAPI: env.st.Run(), searchErr: errors.New("search down")}
	if _, err := env.svc.ListBackupRuns(ctx, RunQuery{}); err == nil || !strings.Contains(err.Error(), "search down") {
		t.Fatalf("search err wrap = %v", err)
	}
}

func TestDeleteBackupJobLifecycleAndErrors(t *testing.T) {
	env := newEnv(t, "/var/lib/dougong")
	ctx := context.Background()
	data, err := env.svc.CreateBackupJob(ctx, nil, validReq())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	now := time.Now().UTC()
	runID, _ := env.st.Run().Create(ctx, data.ID, "nightly", storecron.TriggerManual, now)

	// CB_004。
	if err := env.svc.DeleteBackupJob(ctx, nil, &BackupJobIDReq{JobID: 404}); mustEC(t, err).Symbol() != "CB_004" {
		t.Fatal("missing job must be CB_004")
	}
	// 成功删除：审计留痕 + job 消失 + run 不级联（FR-09）。
	if err := env.svc.DeleteBackupJob(ctx, nil, &BackupJobIDReq{JobID: data.ID}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if rows := auditRows(t, env.audit, ActionBackupJobDeleted); len(rows) != 1 {
		t.Fatalf("backup_job_deleted rows = %d", len(rows))
	}
	if job, _ := env.st.Job().GetByID(ctx, data.ID); job != nil {
		t.Fatal("job must be gone")
	}
	if run, _ := env.st.Run().GetByID(ctx, runID); run == nil {
		t.Fatal("run must survive job deletion (FR-09)")
	}

	// 错误注入：事务内 GetByID / Delete / Commit 失败 → wrap。delete/commit 分支
	// 需真实存在的行（先走真实仓储判定）——补种一行。
	seed := &storecron.Job{
		Name: "seed-for-err", SourceItems: []storecron.SourceItem{
			{Type: storecron.SourceSiteDir, Path: "/var/www/site"}},
		TargetType: storecron.TargetLocal, TargetConfig: storecron.TargetConfig{Path: "/bk"},
		RetentionCount: 3, ScheduleCron: "0 3 * * *",
	}
	seedID, err := env.st.Job().Create(ctx, seed, now)
	if err != nil {
		t.Fatalf("seed job: %v", err)
	}
	fs := withFailStore(env)
	fs.tx = failingTx(env, t)
	fs.tx.job.getByIDErr = errors.New("load down")
	if err := env.svc.DeleteBackupJob(ctx, nil, &BackupJobIDReq{JobID: seedID}); !strings.Contains(err.Error(), "load job") {
		t.Fatalf("getByID err wrap = %v", err)
	}
	fs.tx = failingTx(env, t)
	fs.tx.job.deleteErr = errors.New("del down")
	if err := env.svc.DeleteBackupJob(ctx, nil, &BackupJobIDReq{JobID: seedID}); !strings.Contains(err.Error(), "delete job") {
		t.Fatalf("delete err wrap = %v", err)
	}
	fs.tx = failingTx(env, t)
	fs.tx.commitErr = errors.New("commit down")
	if err := env.svc.DeleteBackupJob(ctx, nil, &BackupJobIDReq{JobID: seedID}); !strings.Contains(err.Error(), "commit") {
		t.Fatalf("commit err wrap = %v", err)
	}
	fs.tx = nil
}

func TestTailCronLogChannels(t *testing.T) {
	env := newEnv(t, "/var/lib/dougong")
	ctx := context.Background()
	// 快乐路径（fakeLogs 固定文本；越档 lines 归 200）。
	text, err := env.svc.TailCronLog(ctx, "9999")
	if err != nil || text != "cron log line" {
		t.Fatalf("tail: %v %q", err, text)
	}
	// ErrNoLogSource（三级来源均缺）→ INTERNAL 脱敏。
	env.svc.logs = failLogs{err: cronx.ErrNoLogSource}
	if _, err := env.svc.TailCronLog(ctx, "200"); mustEC(t, err).Symbol() != "INTERNAL" {
		t.Fatal("no log source must be INTERNAL")
	}
	// 其余错误 → wrap 原样（非 errcode）。
	env.svc.logs = failLogs{err: errors.New("journalctl boom")}
	if _, err := env.svc.TailCronLog(ctx, "200"); err == nil || !strings.Contains(err.Error(), "journalctl boom") {
		t.Fatalf("generic wrap = %v", err)
	}
}

func TestRunExecutorLoopAndPanicIsolation(t *testing.T) {
	env := newEnv(t, "/var/lib/dougong")
	ctx := context.Background()
	data, err := env.svc.CreateBackupJob(ctx, nil, validReq())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	wctx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- env.svc.RunExecutor(wctx) }()

	// worker 消费：立即执行 → 异步转 success（RunExecutor+runOneSafely+executeRun）。
	env.exec.execRes = backupx.ExecResult{
		ArtifactPath: "/var/backups/nightly/w1",
		ArtifactSize: 7,
		Files:        []backupx.FileEntry{{Name: "site-1.tar.gz", Size: 7}},
	}
	res, err := env.svc.RunBackupJob(ctx, nil, data.ID)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	waitForRunStatus(t, env, res.RunID, storecron.StatusSuccess)
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("RunExecutor ctx 取消须返回 nil: %v", err)
	}

	// panic 隔离：Execute panic → runOneSafely recover + failRunDirect 补偿。
	job, _ := env.st.Job().GetByID(ctx, data.ID)
	now := time.Now().UTC()
	r2, _ := env.st.Run().Create(ctx, data.ID, "nightly", storecron.TriggerManual, now)
	env.svc.exec = &panicExec{inner: env.exec}
	env.svc.runOneSafely(ctx, workItem{runID: r2, startedAt: now, spec: env.svc.jobSpecOf(job)})
	run2, _ := env.st.Run().GetByID(ctx, r2)
	if run2.Status != storecron.StatusFailure || run2.ErrorMessage == nil ||
		!strings.Contains(*run2.ErrorMessage, "panic 已隔离") {
		t.Fatalf("panic 隔离后 run 状态: %+v", run2)
	}

	// failRunDirect：仓储错误分支（日志吞掉不抛）与 n==0（已终态）分支。
	fs := withFailStore(env)
	fs.run = &failRunRepo{RunRepoAPI: env.st.Run(), finishFErr: errors.New("finish down")}
	env.svc.failRunDirect(ctx, 999, now, "x") // 不 panic 即过（错误分支）
	fs.run = nil
	env.svc.failRunDirect(ctx, res.RunID, now, "x") // 已 success → n==0 分支
}

func TestRunSchedulerExitsOnCancel(t *testing.T) {
	env := newEnv(t, "/var/lib/dougong")
	wctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- env.svc.RunScheduler(wctx) }()
	cancel() // G-06：ctx 取消退出
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunScheduler 取消须返回 nil: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunScheduler 未在期限内退出")
	}
}

func TestRunBackupJobQueueFullCompensation(t *testing.T) {
	env := newEnv(t, "/var/lib/dougong")
	ctx := context.Background()
	data, err := env.svc.CreateBackupJob(ctx, nil, validReq())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// 填满执行队列（A-C9 容量）→ enqueue false → failRunDirect 如实置失败。
	for i := 0; i < QueueCapacity; i++ {
		env.svc.queue <- workItem{}
	}
	res, err := env.svc.RunBackupJob(ctx, nil, data.ID)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	run, _ := env.st.Run().GetByID(ctx, res.RunID)
	if run.Status != storecron.StatusFailure || run.ErrorMessage == nil ||
		!strings.Contains(*run.ErrorMessage, "队列已满") {
		t.Fatalf("队列满补偿: %+v", run)
	}
	// 手动触发审计仍先于入队落库（A-C3 事务在入队前提交）。
	if rows := auditRows(t, env.audit, ActionBackupRunTriggered); len(rows) != 1 {
		t.Fatalf("backup_run_triggered rows = %d", len(rows))
	}
}

func TestTargetBriefRsyncAndPortHint(t *testing.T) {
	env := newEnv(t, "/var/lib/dougong")
	ctx := context.Background()
	// rsync 带 user/port + 不带 user 两形态（targetBrief 全分支）。
	req := validReq()
	req.TargetType = "rsync"
	req.TargetConfig = TargetConfigDTO{Host: "bk.example.com", User: "root", Path: "/srv/bk", Port: 2022}
	d1, err := env.svc.CreateBackupJob(ctx, nil, req)
	if err != nil {
		t.Fatalf("create rsync: %v", err)
	}
	// 无 user 形态：rsync 校验强制非空 user（validate.go unixUserRe），历史行
	// 防御分支（User==""）绕过 service 校验直插存储覆盖。
	req2 := validReq()
	req2.Name = "mirror2"
	req2.TargetType = "rsync"
	req2.TargetConfig = TargetConfigDTO{Host: "172.16.0.9", User: "sync", Path: "/srv/bk2"}
	d2, err := env.svc.CreateBackupJob(ctx, nil, req2)
	if err != nil {
		t.Fatalf("create rsync2: %v", err)
	}
	noUser := &storecron.Job{
		Name: "legacy-nouser", SourceItems: []storecron.SourceItem{
			{Type: storecron.SourceSiteDir, Path: "/var/www/site"}},
		TargetType:     storecron.TargetRsync,
		TargetConfig:   storecron.TargetConfig{Host: "172.16.0.9", Path: "/srv/bk3"},
		RetentionCount: 3, ScheduleCron: "0 3 * * *",
	}
	noUserID, err := env.st.Job().Create(ctx, noUser, time.Now().UTC())
	if err != nil {
		t.Fatalf("seed no-user job: %v", err)
	}
	list, err := env.svc.ListBackupJobs(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	briefs := map[int64]string{}
	for _, it := range list.Items {
		briefs[it.ID] = it.TargetBrief
	}
	if briefs[d1.ID] != "root@bk.example.com:/srv/bk" {
		t.Fatalf("rsync brief = %q", briefs[d1.ID])
	}
	if briefs[noUserID] != "172.16.0.9:/srv/bk3" {
		t.Fatalf("rsync brief (no user) = %q", briefs[noUserID])
	}

	// jobPortHint：rsync 任务 → 目标端口；已删任务 → 0（A-C14）。
	now := time.Now().UTC()
	r1, _ := env.st.Run().Create(ctx, d1.ID, "nightly", storecron.TriggerCron, now)
	env.st.Run().FinishSuccess(ctx, r1, now.Add(time.Minute), "/remote/nightly/1", 1,
		[]storecron.FileEntry{{Name: "a.tar.gz", Size: 1}})
	env.exec.manifest = &backupx.Manifest{JobID: d1.ID, Sources: []backupx.SourceSpec{
		{Kind: "site_dir", Ref: "/var/www/site"}}}
	detail, err := env.svc.BackupRunDetail(ctx, r1)
	if err != nil || len(detail.Sources) != 1 || detail.Sources[0].Ref != "/var/www/site" {
		t.Fatalf("rsync detail: %v %+v", err, detail)
	}
	r2, _ := env.st.Run().Create(ctx, d2.ID, "mirror2", storecron.TriggerCron, now)
	env.st.Run().FinishSuccess(ctx, r2, now.Add(time.Minute), "/remote/mirror2/1", 1,
		[]storecron.FileEntry{{Name: "b.tar.gz", Size: 1}})
	env.svc.DeleteBackupJob(ctx, nil, &BackupJobIDReq{JobID: d2.ID})
	if _, err := env.svc.BackupRunDetail(ctx, r2); err != nil { // 已删任务端口兜底 0
		t.Fatalf("deleted job detail: %v", err)
	}
}

func TestInstanceLabelDanglingAndArchiveError(t *testing.T) {
	env := newEnv(t, "/var/lib/dougong")
	ctx := context.Background()
	now := time.Now().UTC()
	// 悬空实例（D-12 弱引用合法态）：绕过 service 校验直插，list 投影 → #id 前缀。
	job := &storecron.Job{
		Name: "dangling", SourceItems: []storecron.SourceItem{
			{Type: storecron.SourceDatabase, InstanceID: 404, Database: "shop"}},
		TargetType: storecron.TargetLocal, TargetConfig: storecron.TargetConfig{Path: "/bk"},
		RetentionCount: 3, ScheduleCron: "0 3 * * *",
	}
	if _, err := env.st.Job().Create(ctx, job, now); err != nil {
		t.Fatalf("seed dangling job: %v", err)
	}
	list, err := env.svc.ListBackupJobs(ctx)
	if err != nil || len(list.Items) != 1 {
		t.Fatalf("list: %v %+v", err, list)
	}
	if got := list.Items[0].SourceSummary[0].Ref; got != "#404·shop" {
		t.Fatalf("悬空 ref = %q, want #404·shop", got)
	}
	// 档案 OHS 异常 → 同样 #id 前缀如实可辨。
	env.svc.instArchive = errArchive{}
	list, err = env.svc.ListBackupJobs(ctx)
	if err != nil || list.Items[0].SourceSummary[0].Ref != "#404·shop" {
		t.Fatalf("档案异常 ref: %v %+v", err, list)
	}
}

func TestAuditActorBranchAndDegradation(t *testing.T) {
	env := newEnv(t, "/var/lib/dougong")
	ctx := context.Background()
	// 未正常退出（ExitCode nil）+ 超长输出 → exitCodeOf nil / briefOf 截断分支。
	env.svc.runner = &fakeRunner{res: cronx.TriggerResult{
		ExitCode: nil, Output: strings.Repeat("x", 130), DurationMs: 5}}
	fa := &failAudit{}
	env.svc.audit = fa // recordAudit 重试一次仍失败 → 降级日志
	key := cronx.MakeJobKey("0 2 * * *", "/legacy/nightly.sh")
	res, err := env.svc.TriggerCronJob(ctx, &authsvc.Identity{UserID: 1, Username: "admin"},
		&CronJobTriggerReq{JobKey: key})
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if res.ExitCode != nil {
		t.Fatalf("exitCode = %v, want nil", res.ExitCode)
	}
	if fa.calls != 2 { // 首次失败 + 重试一次
		t.Fatalf("audit record calls = %d, want 2", fa.calls)
	}
}

func TestBackupJobCRUDErrorBranches(t *testing.T) {
	env := newEnv(t, "/var/lib/dougong")
	ctx := context.Background()
	// update 的 commit 失败分支需真实存在行（Update 先走真实仓储行匹配）。
	seed := &storecron.Job{
		Name: "seed", SourceItems: []storecron.SourceItem{
			{Type: storecron.SourceSiteDir, Path: "/var/www/site"}},
		TargetType: storecron.TargetLocal, TargetConfig: storecron.TargetConfig{Path: "/bk"},
		RetentionCount: 3, ScheduleCron: "0 3 * * *",
	}
	seedID, err := env.st.Job().Create(ctx, seed, time.Now().UTC())
	if err != nil {
		t.Fatalf("seed job: %v", err)
	}
	fs := withFailStore(env)

	// CreateBackupJob：BeginTx / tx Create / Commit 失败。
	fs.beginErr = errors.New("bt down")
	if _, err := env.svc.CreateBackupJob(ctx, nil, validReq()); !strings.Contains(err.Error(), "begin tx") {
		t.Fatalf("begin wrap = %v", err)
	}
	fs.beginErr = nil
	fs.tx = failingTx(env, t)
	fs.tx.job.createErr = errors.New("insert down")
	if _, err := env.svc.CreateBackupJob(ctx, nil, validReq()); !strings.Contains(err.Error(), "job repo") {
		t.Fatalf("create wrap（translateJobErr default）= %v", err)
	}
	fs.tx = failingTx(env, t)
	fs.tx.commitErr = errors.New("commit down")
	if _, err := env.svc.CreateBackupJob(ctx, nil, validReq()); !strings.Contains(err.Error(), "commit") {
		t.Fatalf("commit wrap = %v", err)
	}
	fs.tx = nil

	// UpdateBackupJob：tx Update / Commit 失败。
	fs.tx = failingTx(env, t)
	fs.tx.job.updateErr = errors.New("update down")
	req := validReq()
	req.JobID = seedID
	if err := env.svc.UpdateBackupJob(ctx, nil, req); !strings.Contains(err.Error(), "job repo") {
		t.Fatalf("update wrap = %v", err)
	}
	fs.tx = failingTx(env, t)
	fs.tx.commitErr = errors.New("commit down")
	if err := env.svc.UpdateBackupJob(ctx, nil, req); !strings.Contains(err.Error(), "commit") {
		t.Fatalf("update commit wrap = %v", err)
	}
	fs.tx = nil

	// RunBackupJob：load job 失败（非事务级）/ tx Create 非 mutex 错误。
	fs.job = &failJobRepo{JobRepoAPI: env.st.Job(), getByIDErr: errors.New("load down")}
	if _, err := env.svc.RunBackupJob(ctx, nil, 1); !strings.Contains(err.Error(), "load job") {
		t.Fatalf("load wrap = %v", err)
	}
	fs.job = nil
	// RunBackupJob 事务前走连接池 GetByID（db.go SetMaxOpenConns(1)），不能预占
	// 真实连接——以 stub 事务注入 Create 错误。
	st := &stubTx{run: &failRunRepo{createErr: errors.New("create down")}}
	fs.tx = nil
	fs.beginErr = nil
	fs.txOverride = st
	if _, err := env.svc.RunBackupJob(ctx, nil, seedID); !strings.Contains(err.Error(), "create run") {
		t.Fatalf("create run wrap = %v", err)
	}
	fs.txOverride = nil

	// ListBackupJobs：清单/最近执行派生失败。
	fs.job = &failJobRepo{JobRepoAPI: env.st.Job(), listErr: errors.New("list down")}
	if _, err := env.svc.ListBackupJobs(ctx); !strings.Contains(err.Error(), "list jobs") {
		t.Fatalf("list jobs wrap = %v", err)
	}
	fs.job = nil
	fs.run = &failRunRepo{RunRepoAPI: env.st.Run(), latestErr: errors.New("latest down")}
	if _, err := env.svc.ListBackupJobs(ctx); !strings.Contains(err.Error(), "latest run") {
		t.Fatalf("latest wrap = %v", err)
	}
	fs.run = nil
}

func TestExecuteRunErrorBranches(t *testing.T) {
	env := newEnv(t, "/var/lib/dougong")
	ctx := context.Background()
	data, err := env.svc.CreateBackupJob(ctx, nil, validReq())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	fs := withFailStore(env)
	spec := backupx.JobSpec{JobID: data.ID, Name: "nightly", RetentionCount: 3}
	item := workItem{runID: 1, startedAt: time.Now().UTC(), spec: spec}

	// BeginTx 失败。
	fs.beginErr = errors.New("bt down")
	if err := env.svc.executeRun(ctx, item); !strings.Contains(err.Error(), "begin tx") {
		t.Fatalf("begin wrap = %v", err)
	}
	fs.beginErr = nil

	// 执行失败 + FinishFailure 失败 / commit 失败。
	env.exec.execErr = errors.New("exec down")
	fs.tx = failingTx(env, t)
	fs.tx.run.finishFErr = errors.New("ff down")
	if err := env.svc.executeRun(ctx, item); !strings.Contains(err.Error(), "finish failure") {
		t.Fatalf("finish failure wrap = %v", err)
	}
	fs.tx = failingTx(env, t)
	fs.tx.commitErr = errors.New("commit down")
	if err := env.svc.executeRun(ctx, item); !strings.Contains(err.Error(), "commit failure") {
		t.Fatalf("commit failure wrap = %v", err)
	}
	fs.tx = nil
	env.exec.execErr = nil

	// 执行成功 + FinishSuccess / Rotate 失败 / commit 失败。
	fs.tx = failingTx(env, t)
	fs.tx.run.finishSErr = errors.New("fs down")
	if err := env.svc.executeRun(ctx, item); !strings.Contains(err.Error(), "finish success") {
		t.Fatalf("finish success wrap = %v", err)
	}
	fs.tx = failingTx(env, t)
	fs.tx.run.rotateErr = errors.New("rotate down")
	if err := env.svc.executeRun(ctx, item); !strings.Contains(err.Error(), "rotate") {
		t.Fatalf("rotate wrap = %v", err)
	}
	fs.tx = failingTx(env, t)
	fs.tx.commitErr = errors.New("commit down")
	if err := env.svc.executeRun(ctx, item); !strings.Contains(err.Error(), "commit success") {
		t.Fatalf("commit success wrap = %v", err)
	}
	fs.tx = nil
}

func TestScheduleTickDefensivePaths(t *testing.T) {
	env := newEnv(t, "/var/lib/dougong")
	ctx := context.Background()
	now := time.Now().UTC()
	state := &schedulerState{next: map[int64]time.Time{}, clock: env.svc.clock}

	// Job().List 失败 → 错误日志提前返回（不 panic）。
	fs := withFailStore(env)
	fs.job = &failJobRepo{JobRepoAPI: env.st.Job(), listErr: errors.New("list down")}
	env.svc.scheduleTick(ctx, state)

	// 非法表达式任务（I-07 落库前已校验——防御性跳过分支）。
	fs.job = nil
	bad := &storecron.Job{
		Name: "badexpr", SourceItems: []storecron.SourceItem{
			{Type: storecron.SourceSiteDir, Path: "/var/www/site"}},
		TargetType: storecron.TargetLocal, TargetConfig: storecron.TargetConfig{Path: "/bk"},
		RetentionCount: 3, ScheduleCron: "not cron",
	}
	id, err := env.st.Job().Create(ctx, bad, now)
	if err != nil {
		t.Fatalf("seed bad job: %v", err)
	}
	env.svc.scheduleTick(ctx, state)
	if _, ok := state.next[id]; ok {
		t.Fatalf("非法表达式任务不应进入 next 表: %+v", state.next)
	}

	// fireCronRun：仓储 Create 泛化错误（非 mutex）→ 错误日志分支。
	fs.run = &failRunRepo{RunRepoAPI: env.st.Run(), createErr: errors.New("create down")}
	env.svc.fireCronRun(ctx, *bad, now)

	// fireCronRun：队列满 → failRunDirect 补偿（run 置失败）。
	fs.run = nil
	for i := 0; i < QueueCapacity; i++ {
		env.svc.queue <- workItem{}
	}
	env.svc.fireCronRun(ctx, *bad, now)
	runs, total, _ := env.st.Run().Search(ctx, storecron.RunFilter{JobID: &id}, 0, 10)
	if total != 1 || runs[0].Status != storecron.StatusFailure {
		t.Fatalf("队列满 fireCronRun 补偿: total=%d %+v", total, runs)
	}
}

func TestDownloadStatErrorAndDetailManifestBranches(t *testing.T) {
	env := newEnv(t, "/var/lib/dougong")
	ctx := context.Background()
	data, err := env.svc.CreateBackupJob(ctx, nil, validReq())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	now := time.Now().UTC()
	runID, _ := env.st.Run().Create(ctx, data.ID, "nightly", storecron.TriggerManual, now)
	env.st.Run().FinishSuccess(ctx, runID, now.Add(time.Minute), "/bk/x", 9,
		[]storecron.FileEntry{{Name: "site-1.tar.gz", Size: 9}})

	// stat 失败：拉回定位指向不存在文件 → statSize 错误分支（cleanup 后 wrap）。
	env.exec.pullLocal = filepath.Join(t.TempDir(), "ghost.gz")
	if _, err := env.svc.DownloadArtifact(ctx, nil, runID, "site-1.tar.gz"); !strings.Contains(err.Error(), "stat artifact") {
		t.Fatalf("stat wrap = %v", err)
	}
	env.exec.pullLocal = ""

	// detail manifest 三分支：rsync 不拉流 / 读取错误 / 空 manifest → sources nil。
	env.exec.manifestErr = backupx.ErrRemoteManifest
	if d, err := env.svc.BackupRunDetail(ctx, runID); err != nil || d.Sources != nil {
		t.Fatalf("remote manifest → sources nil: %v %+v", err, d)
	}
	env.exec.manifestErr = errors.New("read boom")
	if d, err := env.svc.BackupRunDetail(ctx, runID); err != nil || d.Sources != nil {
		t.Fatalf("manifest err → sources nil: %v %+v", err, d)
	}
	env.exec.manifestErr = nil
	env.exec.manifest = nil
	if d, err := env.svc.BackupRunDetail(ctx, runID); err != nil || d.Sources != nil {
		t.Fatalf("empty manifest → sources nil: %v %+v", err, d)
	}

	// 仓储错误：detail/download 的 GetByID 失败 → wrap。
	fs := withFailStore(env)
	fs.run = &failRunRepo{RunRepoAPI: env.st.Run(), getByIDErr: errors.New("get down")}
	if _, err := env.svc.BackupRunDetail(ctx, runID); !strings.Contains(err.Error(), "load run") {
		t.Fatalf("detail load wrap = %v", err)
	}
	if _, err := env.svc.DownloadArtifact(ctx, nil, runID, "site-1.tar.gz"); !strings.Contains(err.Error(), "load run") {
		t.Fatalf("download load wrap = %v", err)
	}
	fs.run = nil
}

func TestTranslateErrWrapBranchesDirect(t *testing.T) {
	// translateCronTabErr wrap 分支（非 ErrJobNotFound 原样包装）。
	if err := translateCronTabErr(errors.New("gateway boom")); err == nil ||
		!strings.Contains(err.Error(), "gateway boom") {
		t.Fatalf("crontab wrap = %v", err)
	}
	// translateJobErr default 分支（非唯一冲突/非行缺失）。
	if err := translateJobErr(errors.New("repo boom")); err == nil ||
		!strings.Contains(err.Error(), "job repo") {
		t.Fatalf("job wrap = %v", err)
	}
}

func TestNextFireStrNoHitBranches(t *testing.T) {
	from := time.Date(2026, 11, 15, 3, 0, 30, 0, time.UTC)
	// 非法表达式 → 空串。
	if NextFireStr("bad expr", from) != "" {
		t.Fatal("bad expr must be empty")
	}
	// 无命中表达式（2 月 30 日，扫描上限如实）→ 空串。
	if NextFireStr("0 0 30 2 *", from) != "" {
		t.Fatal("no-hit expr must be empty")
	}
	// 正常命中 → RFC3339 UTC。
	if got := NextFireStr("0 3 * * *", from); got != "2026-11-16T03:00:00Z" {
		t.Fatalf("next fire = %q", got)
	}
}
