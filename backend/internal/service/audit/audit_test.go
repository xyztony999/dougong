package audit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/store"
	storeauth "github.com/dougong/dougong/internal/store/auth"
)

func newTestAudit(t *testing.T) (*Service, *storeauth.Store, *clockStub) {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if cerr := db.Close(); cerr != nil {
			t.Errorf("close db: %v", cerr)
		}
	})
	ctx := context.Background()
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := storeauth.New(db)
	clk := &clockStub{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	return New(repo.Audit(), clk.Now), repo, clk
}

type clockStub struct{ t time.Time }

func (c *clockStub) Now() time.Time { return c.t }

func TestRecordAndQuery(t *testing.T) {
	svc, _, clk := newTestAudit(t)
	ctx := context.Background()
	uid := int64(1)

	// I-15 四要素校验：缺 actor 拒绝。
	if err := svc.Record(ctx, Entry{Category: CategorySecurity, Action: ActionLoginSuccess, ObjectType: ObjectTypeUser, Result: ResultSuccess, Source: SourceWeb}); err == nil {
		t.Fatal("empty actor accepted")
	}
	// 非法枚举拒绝。
	if err := svc.Record(ctx, Entry{ActorName: "a", Category: "bogus", Action: "x", ObjectType: "y", Result: ResultSuccess, Source: SourceWeb}); err == nil {
		t.Fatal("bogus category accepted")
	}

	entries := []Entry{
		{ActorID: &uid, ActorName: "admin", Category: CategorySecurity, Action: ActionLoginSuccess, ObjectType: ObjectTypeUser, ObjectID: "1", Detail: map[string]any{"ip": "127.0.0.1"}, Result: ResultSuccess, Source: SourceWeb},
		{ActorName: "admin", Category: CategoryChange, Action: ActionUserCreated, ObjectType: ObjectTypeUser, ObjectID: "2", Detail: map[string]any{"role": "readonly"}, Result: ResultSuccess, Source: SourceWeb},
		{ActorName: "ghost", Category: CategorySecurity, Action: ActionLoginFailed, ObjectType: ObjectTypeUser, Result: ResultFailure, Source: SourceWeb},
	}
	base := clk.t
	for i, e := range entries {
		// 绝对时刻基准：第 i 条落在 base+(i+1) 分钟（避免累加漂移）。
		clk.t = base.Add(time.Duration(i+1) * time.Minute)
		if err := svc.Record(ctx, e); err != nil {
			t.Fatalf("record[%d]: %v", i, err)
		}
	}

	// 全量：3 条。
	items, total, err := svc.Query(ctx, Condition{}, 1, 20)
	if err != nil || total != 3 || len(items) != 3 {
		t.Fatalf("query all = (%d items, total %d, err %v)", len(items), total, err)
	}
	// created_at 倒序。
	if !items[0].CreatedAt.After(items[2].CreatedAt) {
		t.Fatal("not ordered by created_at desc")
	}

	// 条件：category=security → 2；action 等值 → 1；actorName 模糊 ghost → 1；objectId 等值 → 1。
	tests := []struct {
		name string
		cond Condition
		want int64
	}{
		{"category security", Condition{Category: CategorySecurity}, 2},
		{"action equal", Condition{Action: ActionUserCreated}, 1},
		{"actor like", Condition{ActorName: "hos"}, 1},
		{"objectId equal", Condition{ObjectID: "2"}, 1},
		{"from filter", Condition{From: ptrTime(clk.t.Add(-time.Minute))}, 2},
		{"to filter", Condition{To: ptrTime(clk.t.Add(-2 * time.Minute))}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, total, err := svc.Query(ctx, tt.cond, 1, 20)
			if err != nil {
				t.Fatalf("query: %v", err)
			}
			if total != tt.want {
				t.Fatalf("total = %d, want %d", total, tt.want)
			}
		})
	}

	// 分页：page=2 pageSize=2 → 1 条。
	items, total, err = svc.Query(ctx, Condition{}, 2, 2)
	if err != nil || total != 3 || len(items) != 1 {
		t.Fatalf("paged query = (%d items, total %d, err %v)", len(items), total, err)
	}
	// StreamBatch：批大小 2 → 2 批共 3 条。
	var got int
	if err := svc.StreamBatch(ctx, Condition{}, 2, func(b []Record) error { got += len(b); return nil }); err != nil || got != 3 {
		t.Fatalf("stream = %d rows err %v, want 3", got, err)
	}
	// LIKE 通配符转义：% 不应匹配全部。
	_, total, _ = svc.Query(ctx, Condition{ActorName: "%"}, 1, 20)
	if total != 0 {
		t.Fatalf("unescaped LIKE wildcard matched %d rows", total)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

func TestPurgeRetention(t *testing.T) {
	svc, _, clk := newTestAudit(t)
	ctx := context.Background()
	if RetentionDays != 90 {
		t.Fatalf("retention = %d, want 90", RetentionDays)
	}
	// 两条“90+ 天前”与两条“今天”的记录。
	old := Entry{ActorName: "system", Category: CategoryChange, Action: "x_old", ObjectType: ObjectTypeAuditLog, Result: ResultSuccess, Source: SourceSystem}
	fresh := Entry{ActorName: "system", Category: CategoryChange, Action: "x_new", ObjectType: ObjectTypeAuditLog, Result: ResultSuccess, Source: SourceSystem}
	clk.t = clk.t.AddDate(0, 0, -100)
	if err := svc.Record(ctx, old); err != nil {
		t.Fatalf("record old: %v", err)
	}
	if err := svc.Record(ctx, old); err != nil {
		t.Fatalf("record old 2: %v", err)
	}
	clk.t = clk.t.AddDate(0, 0, 100)
	if err := svc.Record(ctx, fresh); err != nil {
		t.Fatalf("record fresh: %v", err)
	}

	deleted, err := svc.Purge(ctx)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted = %d, want 2", deleted)
	}
	// 剩 fresh + audit_purged 审计记录共 2 条。
	_, total, err := svc.Query(ctx, Condition{}, 1, 20)
	if err != nil || total != 2 {
		t.Fatalf("after purge total = %d err %v, want 2 (fresh + audit_purged)", total, err)
	}
	purged, _, err := svc.Query(ctx, Condition{Action: ActionAuditPurged}, 1, 20)
	if err != nil || len(purged) != 1 || purged[0].Source != SourceSystem {
		t.Fatalf("audit_purged record missing: %v", purged)
	}
}

// TestQueryDefaultsAndStreamStop 覆盖分页默认/上限与流遍历提前停止。
func TestQueryDefaultsAndStreamStop(t *testing.T) {
	svc, _, clk := newTestAudit(t)
	ctx := context.Background()
	base := clk.t
	for i := 0; i < 3; i++ {
		clk.t = base.Add(time.Duration(i) * time.Minute)
		if err := svc.Record(ctx, Entry{
			ActorName: "admin", Category: CategorySecurity, Action: ActionLoginSuccess,
			ObjectType: ObjectTypeUser, Result: ResultSuccess, Source: SourceWeb,
		}); err != nil {
			t.Fatalf("record[%d]: %v", i, err)
		}
	}
	// page<1 / pageSize 越界 → 归一为 1/20（不额外报错）。
	items, total, err := svc.Query(ctx, Condition{}, -5, 0)
	if err != nil || total != 3 || len(items) != 3 {
		t.Fatalf("normalized query = (%d items, total %d, err %v)", len(items), total, err)
	}
	// StreamBatch：回调返回错误即中止并透传。
	sentinelErr := errors.New("stop streaming")
	err = svc.StreamBatch(ctx, Condition{}, 0, func(b []Record) error { return sentinelErr })
	if !errors.Is(err, sentinelErr) {
		t.Fatalf("stream abort err = %v, want %v", err, sentinelErr)
	}
}

// TestWorkerRunExitsOnCancel 覆盖每日清理 Worker：启动即补跑一轮 + ctx 取消退出
// （G-06 退出路径验证）。
func TestWorkerRunExitsOnCancel(t *testing.T) {
	svc, _, _ := newTestAudit(t)
	ctx, cancel := context.WithCancel(context.Background())
	w := NewWorker(svc, time.Hour)
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	cancel() // 启动补跑（空库零删除）后应随 ctx 取消返回 nil
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("worker exit err = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not exit after ctx cancel")
	}
}
