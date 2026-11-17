package envmgmt

import (
	"context"
	"testing"

	"github.com/dougong/dougong/internal/store"
)

func openTestDB(t *testing.T) *Store {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(db)
}

// TestTaskLifecycle 任务四态生命周期（创建 pending → running → succeeded；
// SQL #1/#2/#5/#7）。
func TestTaskLifecycle(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id, err := s.Task().Create(ctx, Task{EnvType: EnvNginx, Action: ActionInstall, RequestedBy: "admin"})
	if err != nil || id == 0 {
		t.Fatalf("create: id=%d err=%v", id, err)
	}
	got, err := s.Task().GetByID(ctx, id)
	if err != nil || got.Status != TaskPending || got.OutputTail != "" {
		t.Fatalf("get: %+v err=%v（want pending/空输出）", got, err)
	}
	if err := s.Task().MarkRunning(ctx, id); err != nil {
		t.Fatalf("mark running: %v", err)
	}
	if err := s.Task().AppendOutput(ctx, id, "[channel] apt\nHit:1 ..."); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := s.Task().Finish(ctx, id, TaskSucceeded, nil, nil, nil); err != nil {
		t.Fatalf("finish: %v", err)
	}
	got, _ = s.Task().GetByID(ctx, id)
	if got.Status != TaskSucceeded || got.OutputTail == "" || got.StartedAt == nil || got.FinishedAt == nil {
		t.Fatalf("终态行异常: %+v", got)
	}
	// 终态不可逆（I-02）：MarkRunning 对终态行仍可执行 SQL 但流程层不调用——
	// 互斥查询不命中终态行即可证。
	if inflight, err := s.Task().InFlight(ctx, EnvNginx); err != nil || inflight != nil {
		t.Fatalf("终态后 in-flight = %+v err=%v, want nil", inflight, err)
	}
}

// TestInFlightMutex 同环境在途互斥判定（I-01：pending/running 命中、其他环境不命中）。
func TestInFlightMutex(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id, _ := s.Task().Create(ctx, Task{EnvType: EnvDocker, Action: ActionUpgrade, RequestedBy: "admin"})
	if inflight, err := s.Task().InFlight(ctx, EnvDocker); err != nil || inflight == nil || inflight.ID != id {
		t.Fatalf("in-flight = %+v err=%v, want task %d", inflight, err, id)
	}
	if inflight, err := s.Task().InFlight(ctx, EnvRedis); err != nil || inflight != nil {
		t.Fatalf("他环境 in-flight = %+v err=%v, want nil", inflight, err)
	}
}

// TestFailRunningAll 重启恢复（I-04：running→failed「面板重启中断」；pending
// 不受影响——恢复语义由调用方决定，repo 层只按 running 终态化）。
func TestFailRunningAll(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id1, _ := s.Task().Create(ctx, Task{EnvType: EnvNginx, Action: ActionInstall, RequestedBy: "admin"})
	id2, _ := s.Task().Create(ctx, Task{EnvType: EnvRedis, Action: ActionInstall, RequestedBy: "admin"})
	_ = s.Task().MarkRunning(ctx, id1)
	n, err := s.Task().FailRunningAll(ctx, "面板重启中断")
	if err != nil || n != 1 {
		t.Fatalf("fail running all: n=%d err=%v, want 1（仅 running）", n, err)
	}
	g1, _ := s.Task().GetByID(ctx, id1)
	g2, _ := s.Task().GetByID(ctx, id2)
	if g1.Status != TaskFailed || g1.Error == nil || *g1.Error != "面板重启中断" {
		t.Fatalf("running 恢复行 = %+v, want failed+原因", g1)
	}
	if g2.Status != TaskPending {
		t.Fatalf("pending 行被误恢复: %+v", g2)
	}
}

// TestListFilterAndPage 筛选分页（SQL #4）。
func TestListFilterAndPage(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		id, _ := s.Task().Create(ctx, Task{EnvType: EnvMySQL, Action: ActionUninstall, RequestedBy: "admin"})
		_ = s.Task().Finish(ctx, id, TaskFailed, strp("boom"), nil, nil)
	}
	id, _ := s.Task().Create(ctx, Task{EnvType: EnvNginx, Action: ActionInstall, RequestedBy: "admin"})
	_ = id
	rows, total, err := s.Task().List(ctx, EnvMySQL, TaskFailed, 10, 0)
	if err != nil || total != 3 || len(rows) != 3 {
		t.Fatalf("filtered list: rows=%d total=%d err=%v, want 3/3", len(rows), total, err)
	}
	rows, total, err = s.Task().List(ctx, "", "", 2, 0)
	if err != nil || total != 4 || len(rows) != 2 {
		t.Fatalf("paged list: rows=%d total=%d err=%v, want 2/4", len(rows), total, err)
	}
}

func strp(s string) *string { return &s }

// TestEnvTypeSevenValues 迁移 010（env_type CHECK 扩七值）回归：mongodb 可
// 写入、非法值仍被 CHECK 拒绝、重建后既有行可读（I-04 台账保留）。
func TestEnvTypeSevenValues(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id, err := s.Task().Create(ctx, Task{EnvType: EnvMongoDB, Action: ActionInstall, RequestedBy: "admin"})
	if err != nil || id == 0 {
		t.Fatalf("mongodb create: id=%d err=%v（迁移 010 七值 CHECK 未生效？）", id, err)
	}
	row, err := s.Task().GetByID(ctx, id)
	if err != nil || row.EnvType != EnvMongoDB {
		t.Fatalf("get: %+v err=%v", row, err)
	}
	if _, err := s.Task().Create(ctx, Task{EnvType: "postgresql", Action: ActionInstall, RequestedBy: "admin"}); err == nil {
		t.Fatal("非法 env_type 应被 CHECK 拒绝")
	}
}

// TestBootstrapResultRoundTrip 迁移 012（bootstrap_status/note 两列）回归：
// 引导结果随终态定格可读、无引导步骤两列 NULL、非法 bootstrap_status 被 CHECK
// 拒绝（D-18/I-09）。
func TestBootstrapResultRoundTrip(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	// 数据库类安装成功 + 引导成功：status=succeeded + bootstrap_status=succeeded。
	id, err := s.Task().Create(ctx, Task{EnvType: EnvMySQL, Action: ActionInstall, RequestedBy: "admin"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.Task().Finish(ctx, id, TaskSucceeded, nil, strp(BootstrapSucceeded), strp("已创建管理账户并注册")); err != nil {
		t.Fatalf("finish with bootstrap: %v", err)
	}
	row, err := s.Task().GetByID(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if row.Status != TaskSucceeded || row.BootstrapStatus == nil || *row.BootstrapStatus != BootstrapSucceeded {
		t.Fatalf("引导结果未定格: %+v", row)
	}
	if row.BootstrapNote == nil || *row.BootstrapNote == "" {
		t.Fatalf("引导摘要缺失: %+v", row)
	}
	// 安装成功但引导失败：status=succeeded + bootstrap_status=failed（独立维度）。
	id2, _ := s.Task().Create(ctx, Task{EnvType: EnvRedis, Action: ActionInstall, RequestedBy: "admin"})
	if err := s.Task().Finish(ctx, id2, TaskSucceeded, nil, strp(BootstrapFailed), strp("引导通道建连失败")); err != nil {
		t.Fatalf("finish bootstrap failed: %v", err)
	}
	row2, _ := s.Task().GetByID(ctx, id2)
	if row2.Status != TaskSucceeded || row2.BootstrapStatus == nil || *row2.BootstrapStatus != BootstrapFailed {
		t.Fatalf("引导失败应不改任务 succeeded: %+v", row2)
	}
	// 非数据库环境：无引导步骤，两列 NULL。
	id3, _ := s.Task().Create(ctx, Task{EnvType: EnvNginx, Action: ActionInstall, RequestedBy: "admin"})
	if err := s.Task().Finish(ctx, id3, TaskSucceeded, nil, nil, nil); err != nil {
		t.Fatalf("finish no bootstrap: %v", err)
	}
	row3, _ := s.Task().GetByID(ctx, id3)
	if row3.BootstrapStatus != nil || row3.BootstrapNote != nil {
		t.Fatalf("无引导步骤两列应为 NULL: %+v", row3)
	}
	// 非法 bootstrap_status 被 CHECK 拒绝。
	id4, _ := s.Task().Create(ctx, Task{EnvType: EnvMySQL, Action: ActionInstall, RequestedBy: "admin"})
	if err := s.Task().Finish(ctx, id4, TaskSucceeded, nil, strp("bogus"), nil); err == nil {
		t.Fatal("非法 bootstrap_status 应被 CHECK 拒绝")
	}
}
