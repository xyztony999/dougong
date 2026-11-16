package cronback

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/adapter/backupx"
	"github.com/dougong/dougong/internal/adapter/cronx"
	"github.com/dougong/dougong/internal/errcode"
	auditsvc "github.com/dougong/dougong/internal/service/audit"
	"github.com/dougong/dougong/internal/store"
	storeauth "github.com/dougong/dougong/internal/store/auth"
	storecron "github.com/dougong/dougong/internal/store/cronback"
)

// ---- 测试替身 ----

// fakeTab 内存 crontab（Upsert/Delete 走纯函数，等价网关语义）。
type fakeTab struct {
	mu      sync.Mutex
	content string
	sys     cronx.SystemEntries
}

func (f *fakeTab) List(ctx context.Context) (*cronx.TabProjection, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &cronx.TabProjection{
		Entries:       cronx.ParseTabLines(splitLinesOf(f.content)),
		SystemEntries: f.sys,
	}, nil
}
func (f *fakeTab) ReadRaw(context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.content, nil
}
func (f *fakeTab) Upsert(_ context.Context, oldKey string, job cronx.JobInput) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	lines, key, err := cronx.RenderTabJob(splitLinesOf(f.content), oldKey, job)
	if err != nil {
		return "", err
	}
	f.content = joinLines(lines)
	return key, nil
}
func (f *fakeTab) Delete(_ context.Context, jobKey string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	lines, err := cronx.RemoveTabJob(splitLinesOf(f.content), jobKey)
	if err != nil {
		return err
	}
	f.content = joinLines(lines)
	return nil
}

func splitLinesOf(raw string) []string {
	if raw == "" {
		return nil
	}
	out := []string{}
	start := 0
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\n' {
			out = append(out, raw[start:i])
			start = i + 1
		}
	}
	out = append(out, raw[start:])
	return out
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n"
		}
		out += l
	}
	return out + "\n"
}

// fakeLogs 固定文本尾部。
type fakeLogs struct{ text string }

func (f fakeLogs) Tail(context.Context, int) (string, error) { return f.text, nil }

// fakeRunner 可预置代执行结果。
type fakeRunner struct {
	res   cronx.TriggerResult
	calls []string
}

func (f *fakeRunner) Run(_ context.Context, command string, _ time.Duration) cronx.TriggerResult {
	f.calls = append(f.calls, command)
	return f.res
}

// fakeExec 可预置执行结果/错误与产物通道行为。
type fakeExec struct {
	mu sync.Mutex

	execRes     backupx.ExecResult
	execErr     error
	pullLocal   string
	pullErr     error
	pullErrIs   error
	manifest    *backupx.Manifest
	manifestErr error    // ReadManifestPath 注入错误（W1：detail 三分支）
	removed     []string // RemoveArtifactPath 调用记录
}

func (f *fakeExec) Execute(context.Context, backupx.JobSpec, time.Time) (backupx.ExecResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.execRes, f.execErr
}
func (f *fakeExec) PullFile(_ context.Context, _, name string, _ int) (string, func(), error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.pullErr != nil {
		if f.pullErrIs != nil && errors.Is(f.pullErr, f.pullErrIs) {
			return "", nil, f.pullErr
		}
		if f.pullErrIs == nil {
			return "", nil, f.pullErr
		}
	}
	return f.pullLocal, func() {}, nil
}
func (f *fakeExec) ReadManifestPath(context.Context, string, int) (*backupx.Manifest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.manifestErr != nil {
		return nil, f.manifestErr
	}
	return f.manifest, nil
}
func (f *fakeExec) RemoveArtifactPath(_ context.Context, path string, _ int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, path)
	return nil
}

// fakeInstArchive 实例档案替身。
type fakeInstArchive struct {
	exists map[int64]string // id → instanceName
}

func (f fakeInstArchive) InstanceArchive(_ context.Context, id int64) (bool, string, string, error) {
	name, ok := f.exists[id]
	return ok, "mysql", name, nil
}

// ---- 测试装置 ----

type testEnv struct {
	svc   *Service
	st    *storecron.Store
	db    interface{ Close() error }
	exec  *fakeExec
	tab   *fakeTab
	audit *storeauth.AuditRepo
}

func newEnv(t *testing.T, dataDir string) *testEnv {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	st := storecron.New(db)
	auditRepo := storeauth.NewAuditRepo(db)
	tab := &fakeTab{content: "# 每日\n0 2 * * * /legacy/nightly.sh\n"}
	exec := &fakeExec{}
	inst := fakeInstArchive{exists: map[int64]string{7: "local", 9: "192.168.1.10-3306"}}
	svc := New(tab, fakeLogs{text: "cron log line"}, &fakeRunner{}, exec,
		AdaptStore(st), auditsvc.New(auditRepo, time.Now), inst, dataDir)
	return &testEnv{svc: svc, st: st, db: db, exec: exec, tab: tab, audit: auditRepo}
}

func mustEC(t *testing.T, err error) *errcode.Error {
	t.Helper()
	var ec *errcode.Error
	if !errors.As(err, &ec) {
		t.Fatalf("err = %v (%T), want errcode.Error", err, err)
	}
	return ec
}

func validReq() *BackupJobReq {
	return &BackupJobReq{
		Name: "nightly",
		SourceItems: []SourceItemDTO{
			{Type: "site_dir", Path: "/var/www/site"},
			{Type: "database", InstanceID: 7, Database: "shop"},
		},
		TargetType:     "local",
		TargetConfig:   TargetConfigDTO{Path: "/var/backups"},
		RetentionCount: 3,
		ScheduleCron:   "0 3 * * *",
	}
}

func auditRows(t *testing.T, repo *storeauth.AuditRepo, action string) []storeauth.AuditRecord {
	t.Helper()
	rows, _, err := repo.Search(context.Background(), storeauth.AuditCondition{Action: action}, 0, 50)
	if err != nil {
		t.Fatalf("audit search %s: %v", action, err)
	}
	return rows
}

// ---- 用例 ----

func TestCreateBackupJobValidation(t *testing.T) {
	env := newEnv(t, "/var/lib/dougong")
	ctx := context.Background()

	cases := []struct {
		name string
		mut  func(*BackupJobReq)
		code string
	}{
		{"空名", func(r *BackupJobReq) { r.Name = "  " }, "CB_001"},
		{"超长名", func(r *BackupJobReq) { r.Name = string(make([]byte, 65)) }, "CB_001"},
		{"空源清单", func(r *BackupJobReq) { r.SourceItems = nil }, "CB_001"},
		{"源类型非法", func(r *BackupJobReq) { r.SourceItems[0].Type = "ftp" }, "CB_001"},
		{"路径非绝对", func(r *BackupJobReq) { r.SourceItems[0].Path = "relative/x" }, "CB_001"},
		{"实例悬空", func(r *BackupJobReq) { r.SourceItems[1].InstanceID = 404 }, "CB_001"},
		{"库名非法", func(r *BackupJobReq) { r.SourceItems[1].Database = "bad db!" }, "CB_001"},
		{"目标类型非法", func(r *BackupJobReq) { r.TargetType = "s3" }, "CB_001"},
		{"本地路径非绝对", func(r *BackupJobReq) { r.TargetConfig.Path = "backups" }, "CB_001"},
		{"目标=数据目录", func(r *BackupJobReq) { r.TargetConfig.Path = "/var/lib/dougong" }, "CB_001"},
		{"目标含数据目录", func(r *BackupJobReq) { r.TargetConfig.Path = "/var" }, "CB_001"},
		{"目标在数据目录内", func(r *BackupJobReq) { r.TargetConfig.Path = "/var/lib/dougong/bk" }, "CB_001"},
		{"rsync 缺 host", func(r *BackupJobReq) {
			r.TargetType = "rsync"
			r.TargetConfig = TargetConfigDTO{User: "root", Path: "/srv/bk"}
		}, "CB_001"},
		{"rsync 非法端口", func(r *BackupJobReq) {
			r.TargetType = "rsync"
			r.TargetConfig = TargetConfigDTO{Host: "bk.example.com", User: "root", Path: "/srv/bk", Port: 70000}
		}, "CB_001"},
		{"保留份数越界", func(r *BackupJobReq) { r.RetentionCount = 101 }, "CB_001"},
		{"表达式非法", func(r *BackupJobReq) { r.ScheduleCron = "99 * * * *" }, "CB_002"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := validReq()
			tc.mut(req)
			_, err := env.svc.CreateBackupJob(ctx, nil, req)
			ec := mustEC(t, err)
			if ec.Symbol() != tc.code {
				t.Fatalf("code = %s, want %s (%s)", ec.Symbol(), tc.code, ec.Message())
			}
		})
	}
	// 校验失败零写入零审计（panel-core I-P07 同判）。
	jobs, _ := env.st.Job().List(ctx)
	if len(jobs) != 0 {
		t.Fatalf("jobs leaked on validation failure: %d", len(jobs))
	}
	if rows := auditRows(t, env.audit, ActionBackupJobCreated); len(rows) != 0 {
		t.Fatalf("audit leaked on validation failure: %d", len(rows))
	}
}

func TestCreateJobMacroNormalizationAndDuplicate(t *testing.T) {
	env := newEnv(t, "/var/lib/dougong")
	ctx := context.Background()
	req := validReq()
	req.ScheduleCron = "@daily" // 宏归一化写入（D-03）
	data, err := env.svc.CreateBackupJob(ctx, nil, req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	job, _ := env.st.Job().GetByID(ctx, data.ID)
	if job.ScheduleCron != "0 0 * * *" {
		t.Fatalf("macro not normalized: %q", job.ScheduleCron)
	}
	if rows := auditRows(t, env.audit, ActionBackupJobCreated); len(rows) != 1 {
		t.Fatalf("backup_job_created audit rows = %d, want 1", len(rows))
	}
	// I-06 重名 → CB_005。
	req2 := validReq()
	req2.ScheduleCron = "0 4 * * *"
	_, err = env.svc.CreateBackupJob(ctx, nil, req2)
	if ec := mustEC(t, err); ec.Symbol() != "CB_005" {
		t.Fatalf("dup name code = %s, want CB_005", ec.Symbol())
	}
	// update 改名撞他人 → CB_005；排除自身不误伤。
	req3 := validReq()
	req3.JobID = data.ID // 未改名（name 仍 nightly）
	if err := env.svc.UpdateBackupJob(ctx, nil, req3); err != nil {
		t.Fatalf("self-name update must pass: %v", err)
	}
	// update 不存在的任务 → CB_004。
	req4 := validReq()
	req4.JobID = 999
	if ec := mustEC(t, env.svc.UpdateBackupJob(ctx, nil, req4).(*errcode.Error)); ec.Symbol() != "CB_004" {
		t.Fatalf("update missing code = %s", ec.Symbol())
	}
}

func TestRunBackupJobFullOrchestration(t *testing.T) {
	env := newEnv(t, "/var/lib/dougong")
	ctx := context.Background()
	data, err := env.svc.CreateBackupJob(ctx, nil, validReq())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// 预置 3 条 success 历史（retention=3 → 本次成功后轮转删除最旧 1 条）。
	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		rid, cerr := env.st.Run().Create(ctx, data.ID, "nightly", storecron.TriggerCron, now.Add(-time.Duration(48-i)*time.Hour))
		if cerr != nil {
			t.Fatalf("seed run: %v", cerr)
		}
		if _, ferr := env.st.Run().FinishSuccess(ctx, rid,
			now.Add(-time.Duration(48-i)*time.Hour+time.Minute), fmt.Sprintf("/bk/old-%d", rid), 1,
			[]storecron.FileEntry{{Name: "x", Size: 1}}); ferr != nil {
			t.Fatalf("seed finish: %v", ferr)
		}
	}

	// 立即执行（D-07：立即返回 runId）。
	res, err := env.svc.RunBackupJob(ctx, nil, data.ID)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// 手动触发审计与 INSERT 同事务（A-C3）。
	if rows := auditRows(t, env.audit, ActionBackupRunTriggered); len(rows) != 1 {
		t.Fatalf("backup_run_triggered rows = %d, want 1", len(rows))
	}
	// CB_004。
	if _, err := env.svc.RunBackupJob(ctx, nil, 404); mustEC(t, err).Symbol() != "CB_004" {
		t.Fatal("missing job must be CB_004")
	}
	// CB_006：running 在途（worker 未消费——队列暂存）。
	if _, err := env.svc.RunBackupJob(ctx, nil, data.ID); mustEC(t, err).Symbol() != "CB_006" {
		t.Fatal("in-flight job must be CB_006")
	}

	// 消费队列：执行成功路径。
	env.exec.execRes = backupx.ExecResult{
		ArtifactPath: "/var/backups/nightly/new",
		ArtifactSize: 42,
		Files:        []backupx.FileEntry{{Name: "site-1.tar.gz", Size: 42}},
	}
	item := <-env.svc.queue
	if err := env.svc.executeRun(ctx, item); err != nil {
		t.Fatalf("executeRun: %v", err)
	}
	run, _ := env.st.Run().GetByID(ctx, res.RunID)
	if run.Status != storecron.StatusSuccess || run.ArtifactPath == nil ||
		*run.ArtifactPath != "/var/backups/nightly/new" || len(run.Files) != 1 {
		t.Fatalf("run state wrong: %+v", run)
	}
	// 轮转：4 条 success + retention 3 → 删除最旧 1 条（/bk/old-4 即 rid=4）。
	if rows := auditRows(t, env.audit, ActionBackupRotated); len(rows) != 1 {
		t.Fatalf("backup_rotated rows = %d, want 1", len(rows))
	}
	_, total, _ := env.st.Run().Search(ctx, storecron.RunFilter{JobID: &data.ID}, 0, 100)
	if total != 3 { // seed 3 + new 1 = 4 条 success，retention=3 删最旧 1 条 → 3
		t.Fatalf("rotation retention broken: total = %d, want 3", total)
	}
	// 事务后产物清理（A-C4：被删记录〔id=1，最旧 success〕的产物定位已收集并清理）。
	if len(env.exec.removed) != 1 || env.exec.removed[0] != "/bk/old-1" {
		t.Fatalf("artifact cleanup calls = %v", env.exec.removed)
	}

	// 失败路径：error_message 含失败源（I-04）。
	failRes, err := env.svc.RunBackupJob(ctx, nil, data.ID)
	if err != nil {
		t.Fatalf("run2: %v", err)
	}
	env.exec.execErr = errors.New("database local/shop: dump open: DBM_003: 连接不可达")
	item2 := <-env.svc.queue
	if err := env.svc.executeRun(ctx, item2); err != nil {
		t.Fatalf("executeRun failure path: %v", err)
	}
	run2, _ := env.st.Run().GetByID(ctx, failRes.RunID)
	if run2.Status != storecron.StatusFailure || run2.ErrorMessage == nil ||
		!containsStr(*run2.ErrorMessage, "shop") {
		t.Fatalf("failure state wrong: %+v", run2)
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestCronJobCRUDOverGateway(t *testing.T) {
	env := newEnv(t, "/var/lib/dougong")
	ctx := context.Background()
	remark := "站点日志切割"
	created, err := env.svc.CreateCronJob(ctx, nil, &CronJobCreateReq{
		Expr: "@daily", Command: "/scripts/logrotate.sh  --verbose", Remark: &remark})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	list, err := env.svc.ListCronJobs(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var found *CronJobItem
	for i := range list.Items {
		if list.Items[i].JobKey == created.JobKey {
			found = &list.Items[i]
		}
	}
	if found == nil {
		t.Fatalf("created job not listed: %+v", list.Items)
	}
	if found.Source != cronx.SourcePanel || found.Remark == nil || *found.Remark != remark {
		t.Fatalf("marker projection wrong: %+v", found)
	}
	if found.Expr != "0 0 * * *" { // 宏归一化（D-03）
		t.Fatalf("expr = %q", found.Expr)
	}
	if found.Command != "/scripts/logrotate.sh --verbose" {
		t.Fatalf("command normalization = %q", found.Command)
	}
	// CB_002：面板写入必须是五段（宏在写入侧归一，非法直接拒）。
	if _, err := env.svc.CreateCronJob(ctx, nil, &CronJobCreateReq{Expr: "* * * *", Command: "x"}); mustEC(t, err).Symbol() != "CB_002" {
		t.Fatal("bad expr must be CB_002")
	}
	// update：jobKey 随内容变更（D-01）。
	updated, err := env.svc.UpdateCronJob(ctx, nil, &CronJobUpdateReq{
		JobKey: created.JobKey, Expr: "0 5 * * *", Command: "/scripts/logrotate.sh"})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.JobKey == created.JobKey {
		t.Fatal("jobKey must change with content")
	}
	// 旧 key 不存在 → CB_003。
	if err := env.svc.DeleteCronJob(ctx, nil, &CronJobDeleteReq{JobKey: created.JobKey}); mustEC(t, err).Symbol() != "CB_003" {
		t.Fatal("stale jobKey must be CB_003")
	}
	if err := env.svc.DeleteCronJob(ctx, nil, &CronJobDeleteReq{JobKey: updated.JobKey}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if rows := auditRows(t, env.audit, ActionCronJobCreated); len(rows) != 1 {
		t.Fatalf("cron_job_created rows = %d", len(rows))
	}
	if rows := auditRows(t, env.audit, ActionCronJobDeleted); len(rows) != 1 {
		t.Fatalf("cron_job_deleted rows = %d", len(rows))
	}
}

func TestTriggerCronJob(t *testing.T) {
	env := newEnv(t, "/var/lib/dougong")
	ctx := context.Background()
	zeroExit := 0
	runner := &fakeRunner{res: cronx.TriggerResult{ExitCode: &zeroExit, Output: "done"}}
	env.svc.runner = runner
	// 环境内置 legacy 任务 0 2 * * * /legacy/nightly.sh。
	key := cronx.MakeJobKey("0 2 * * *", "/legacy/nightly.sh")
	res, err := env.svc.TriggerCronJob(ctx, nil, &CronJobTriggerReq{JobKey: key})
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if len(runner.calls) != 1 || runner.calls[0] != "/legacy/nightly.sh" {
		t.Fatalf("runner calls = %v（I-08：命令原文取自 crontab）", runner.calls)
	}
	_ = res
	// CB_003。
	if _, err := env.svc.TriggerCronJob(ctx, nil, &CronJobTriggerReq{JobKey: "ghost\tcmd"}); mustEC(t, err).Symbol() != "CB_003" {
		t.Fatal("ghost key must be CB_003")
	}
	// 失败/超时=业务结果（D-05）+ failure 审计。
	env.svc.runner = &fakeRunner{res: cronx.TriggerResult{ExitCode: &zeroExit, Output: "x", TimedOut: true}}
	if _, err := env.svc.TriggerCronJob(ctx, nil, &CronJobTriggerReq{JobKey: key}); err != nil {
		t.Fatalf("timeout is business result, not error: %v", err)
	}
	rows := auditRows(t, env.audit, ActionCronJobTriggered)
	if len(rows) != 2 || rows[0].Result != "failure" || rows[1].Result != "success" {
		t.Fatalf("trigger audit rows（倒序，最新在前）= %+v", rows)
	}
}

func TestValidateCronExprService(t *testing.T) {
	env := newEnv(t, "/var/lib/dougong")
	ctx := context.Background()
	data, err := env.svc.ValidateCronExpr(ctx, &CronExprValidateReq{Expr: "@hourly"})
	if err != nil || len(data.NextFires) != PreviewCount {
		t.Fatalf("validate: %v %+v", err, data)
	}
	_, err = env.svc.ValidateCronExpr(ctx, &CronExprValidateReq{Expr: "61 * * * *"})
	ec := mustEC(t, err)
	if ec.Symbol() != "CB_002" {
		t.Fatalf("code = %s", ec.Symbol())
	}
}

func TestSchedulerTickFireAndSkip(t *testing.T) {
	env := newEnv(t, "/var/lib/dougong")
	ctx := context.Background()
	data, err := env.svc.CreateBackupJob(ctx, nil, validReq())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// fake clock：固定当前时刻，使调度判定确定。
	fixed := time.Date(2026, 11, 15, 3, 0, 30, 0, time.UTC)
	env.svc.clock = func() time.Time { return fixed }
	state := &schedulerState{next: map[int64]time.Time{}, clock: env.svc.clock}

	// 首轮：next 从当前时刻重算（03:00:30 → 次日 03:00）→ 不触发（错过即跳过，D-13）。
	env.svc.scheduleTick(ctx, state)
	if got := state.next[data.ID]; !got.Equal(time.Date(2026, 11, 16, 3, 0, 0, 0, time.UTC)) {
		t.Fatalf("first tick next = %s, want 2026-11-16 03:00", got)
	}
	if l := len(env.svc.queue); l != 0 {
		t.Fatalf("queue = %d, want 0 (no catch-up fire)", l)
	}

	// 推进时钟越过次日触发点（表达式 0 3 * * *：daily 03:00）→ 触发一次并重算。
	fixed = fixed.Add(25 * time.Hour) // 2026-11-16 04:00:30（已越过 11-16 03:00）
	env.svc.scheduleTick(ctx, state)
	if l := len(env.svc.queue); l != 1 {
		t.Fatalf("queue = %d, want 1", l)
	}
	item := <-env.svc.queue
	if item.spec.JobID != data.ID || item.runID == 0 {
		t.Fatalf("fired item wrong: %+v", item)
	}

	// 互斥跳过：上一触发 run 尚处 running（未消费执行）→ 次日到期 tick 静默
	// 跳过（I-01 调度侧，D-08）。
	fixed = fixed.Add(24 * time.Hour) // 2026-11-17 04:00:30（再次越过触发点）
	env.svc.scheduleTick(ctx, state)
	if l := len(env.svc.queue); l != 0 {
		t.Fatalf("queue = %d, want 0 (mutex skip)", l)
	}
	_, total, _ := env.st.Run().Search(ctx, storecron.RunFilter{JobID: &data.ID}, 0, 10)
	if total != 1 { // 仅首次触发的 run，第二次被互斥跳过
		t.Fatalf("runs = %d, want 1", total)
	}
}

func TestDownloadAndDetailChain(t *testing.T) {
	env := newEnv(t, "/var/lib/dougong")
	ctx := context.Background()
	data, _ := env.svc.CreateBackupJob(ctx, nil, validReq())
	now := time.Now().UTC()
	runID, _ := env.st.Run().Create(ctx, data.ID, "nightly", storecron.TriggerManual, now)
	env.st.Run().FinishSuccess(ctx, runID, now.Add(time.Minute), "/bk/x", 9,
		[]storecron.FileEntry{{Name: "site-1.tar.gz", Size: 9}})

	// running 态 → CB_008。
	runID2, _ := env.st.Run().Create(ctx, data.ID, "nightly", storecron.TriggerCron, now.Add(2*time.Minute))
	if _, err := env.svc.DownloadArtifact(ctx, nil, runID2, "site-1.tar.gz"); mustEC(t, err).Symbol() != "CB_008" {
		t.Fatal("running run must be CB_008")
	}
	// 非法文件名 → CB_001。
	if _, err := env.svc.DownloadArtifact(ctx, nil, runID, "../manifest.json"); mustEC(t, err).Symbol() != "CB_001" {
		t.Fatal("file outside manifest must be CB_001")
	}
	if _, err := env.svc.DownloadArtifact(ctx, nil, runID, ""); mustEC(t, err).Symbol() != "CB_001" {
		t.Fatal("empty file must be CB_001")
	}
	// CB_009：清单内但磁盘缺失。
	env.exec.pullErr = backupx.ErrArtifactMissing
	env.exec.pullErrIs = backupx.ErrArtifactMissing
	if _, err := env.svc.DownloadArtifact(ctx, nil, runID, "site-1.tar.gz"); mustEC(t, err).Symbol() != "CB_009" {
		t.Fatal("missing artifact must be CB_009")
	}
	// 成功：security 审计（数据取出口径）；拉回产物为真实临时文件（size 校验用）。
	env.exec.pullErr = nil
	env.exec.pullErrIs = nil
	pulled := filepath.Join(t.TempDir(), "pulled.gz")
	if werr := os.WriteFile(pulled, []byte("012345678"), 0o644); werr != nil {
		t.Fatalf("seed pulled file: %v", werr)
	}
	env.exec.pullLocal = pulled
	target, err := env.svc.DownloadArtifact(ctx, nil, runID, "site-1.tar.gz")
	if err != nil || target.Name != "site-1.tar.gz" || target.Path != pulled || target.Size != 9 {
		t.Fatalf("download: %v %+v", err, target)
	}
	if rows := auditRows(t, env.audit, ActionBackupArtifactDownloaded); len(rows) != 1 || rows[0].Category != "security" {
		t.Fatalf("download audit rows = %+v", rows)
	}
	// CB_007。
	if _, err := env.svc.DownloadArtifact(ctx, nil, 999, "x"); mustEC(t, err).Symbol() != "CB_007" {
		t.Fatal("missing run must be CB_007")
	}

	// detail：success+manifest 可读 → sources 呈现（A-C5）。
	env.exec.manifest = &backupx.Manifest{
		JobID: data.ID, JobName: "nightly",
		Sources: []backupx.SourceSpec{{Kind: "database", InstanceID: 7, Database: "shop", Ref: "local·shop"}},
	}
	detail, err := env.svc.BackupRunDetail(ctx, runID)
	if err != nil || len(detail.Sources) != 1 || detail.Sources[0].Database != "shop" {
		t.Fatalf("detail sources: %v %+v", err, detail)
	}
	if detail.Sources[0].Ref != "local·shop" { // detail sources[] {type, ref}（E1）
		t.Fatalf("detail source ref = %q, want local·shop", detail.Sources[0].Ref)
	}
	if detail.DurationMs == nil || *detail.DurationMs != int64(time.Minute/time.Millisecond) {
		t.Fatalf("detail durationMs = %v", detail.DurationMs)
	}
	// running 态 detail：产物字段 null。
	d2, err := env.svc.BackupRunDetail(ctx, runID2)
	if err != nil || d2.ArtifactPath != nil || d2.Files != nil || d2.FinishedAt != nil {
		t.Fatalf("running detail must be null-ish: %+v", d2)
	}
}

func TestRecoverOrphansAndListDerivation(t *testing.T) {
	env := newEnv(t, "/var/lib/dougong")
	ctx := context.Background()
	data, _ := env.svc.CreateBackupJob(ctx, nil, validReq())
	now := time.Now().UTC()
	runID, _ := env.st.Run().Create(ctx, data.ID, "nightly", storecron.TriggerManual, now)
	env.st.Run().FinishSuccess(ctx, runID, now.Add(time.Minute), "/bk/1", 5,
		[]storecron.FileEntry{{Name: "a.gz", Size: 5}})
	env.st.Run().Create(ctx, data.ID, "nightly", storecron.TriggerCron, now.Add(2*time.Minute)) // 孤儿 running

	n, err := env.svc.RecoverOrphans(ctx)
	if err != nil || n != 1 {
		t.Fatalf("recover: n=%d err=%v", n, err)
	}
	if rows := auditRows(t, env.audit, ActionBackupRunInterrupted); len(rows) != 1 || rows[0].Source != "system" {
		t.Fatalf("interrupted audit rows = %+v", rows)
	}
	// 互斥解除。
	if _, err := env.st.Run().Create(ctx, data.ID, "nightly", storecron.TriggerManual, now.Add(3*time.Minute)); err != nil {
		t.Fatalf("mutex not released: %v", err)
	}

	// job/list：lastRun 派生（D-11）+ nextFireAt 推算。
	list, err := env.svc.ListBackupJobs(ctx)
	if err != nil || len(list.Items) != 1 {
		t.Fatalf("list: %v %+v", err, list)
	}
	item := list.Items[0]
	if item.LastRun == nil || item.LastRun.ID != 3 || item.LastRun.Status != "running" {
		t.Fatalf("lastRun = %+v（最新一条=id3）", item.LastRun)
	}
	if item.NextFireAt == nil || *item.NextFireAt == "" {
		t.Fatal("nextFireAt missing")
	}
	if item.TargetBrief != "/var/backups" || item.SourceSummary[0].Type != "site_dir" {
		t.Fatalf("item projection wrong: %+v", item)
	}
	// ref 形状断言（Gate 2 E1：sourceSummary[{type, ref}]）——site_dir→路径；
	// database→实例名·库名（档案显示名，悬空→#id 前缀）。
	if item.SourceSummary[0].Ref != "/var/www/site" {
		t.Fatalf("site ref = %q, want /var/www/site", item.SourceSummary[0].Ref)
	}
	if item.SourceSummary[1].Ref != "local·shop" {
		t.Fatalf("db ref = %q, want local·shop", item.SourceSummary[1].Ref)
	}
}

// compile-time：接口实现断言（fakeExec 满足 BackupExecutor 契约）。
var _ BackupExecutor = (*fakeExec)(nil)
