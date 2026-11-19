// store/dbtools 仓储测试（内存 SQLite + 全量迁移；api §5.2 SQL 清单 round-trip
// 与互斥/恢复/同事务审计断言）。
package dbtools

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/store"
	storeauth "github.com/dougong/dougong/internal/store/auth"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate（含 013 两表）: %v", err)
	}
	return New(db)
}

// TestToolRoundTrip 档案 CRUD（SQL #10~#15）+ UNIQUE 一工具一档案（I-01）。
func TestToolRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	unit := "dougong-mongo-express.service"
	inst := int64(7)
	id, err := s.Tool().Create(ctx, Tool{
		ToolType: ToolMongoExpress, InstalledVersion: "1.0.0",
		DeployPath: "/usr/lib/node_modules/mongo-express", SiteRef: `{"file":"/etc/nginx/conf.d/panel.conf","index":0}`,
		UnitName: &unit, InstanceID: &inst, CredCipher: "cipher-text",
	})
	if err != nil || id == 0 {
		t.Fatalf("create: id=%d err=%v", id, err)
	}
	got, err := s.Tool().GetByType(ctx, ToolMongoExpress)
	if err != nil || got == nil {
		t.Fatalf("get: %v %v", got, err)
	}
	if got.InstalledVersion != "1.0.0" || got.UnitName == nil || *got.UnitName != unit ||
		got.InstanceID == nil || *got.InstanceID != 7 || got.CredCipher != "cipher-text" {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	// UNIQUE 承载一工具一档案。
	if _, err := s.Tool().Create(ctx, Tool{ToolType: ToolMongoExpress, InstalledVersion: "x",
		DeployPath: "x", SiteRef: "x", CredCipher: "x"}); err == nil {
		t.Fatal("duplicate tool_type should violate UNIQUE (I-01)")
	}
	// phpmyadmin 档案可空字段（unit/instance NULL）。
	if _, err := s.Tool().Create(ctx, Tool{ToolType: ToolPHPMyAdmin, InstalledVersion: "5.2.2",
		DeployPath: "/data/tools/phpmyadmin", SiteRef: "ref", CredCipher: "c"}); err != nil {
		t.Fatal(err)
	}
	pma, err := s.Tool().GetByType(ctx, ToolPHPMyAdmin)
	if err != nil || pma == nil || pma.UnitName != nil || pma.InstanceID != nil {
		t.Fatalf("phpmyadmin nullable fields: %+v err=%v", pma, err)
	}
	// 升级换版 / 轮换密文。
	if err := s.Tool().UpdateVersion(ctx, ToolMongoExpress, "1.0.1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Tool().UpdateCredCipher(ctx, ToolMongoExpress, "cipher-2"); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Tool().GetByType(ctx, ToolMongoExpress)
	if got.InstalledVersion != "1.0.1" || got.CredCipher != "cipher-2" {
		t.Fatalf("update mismatch: %+v", got)
	}
	list, err := s.Tool().List(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %d err=%v", len(list), err)
	}
	// 卸载除档。
	if err := s.Tool().Delete(ctx, ToolMongoExpress); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Tool().GetByType(ctx, ToolMongoExpress); err != nil || got != nil {
		t.Fatalf("after delete: %+v err=%v", got, err)
	}
	// 不存在类型 → nil（档案存在性投影）。
	if got, err := s.Tool().GetByType(ctx, "nope"); err != nil || got != nil {
		t.Fatalf("unknown type: %+v err=%v", got, err)
	}
}

// TestTaskLifecycle 任务四态机 + 互斥查询 + 筛选分页 + 重启恢复（I-06~I-09）。
func TestTaskLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	inst := int64(3)
	id, err := s.Task().Create(ctx, Task{ToolType: ToolMongoExpress, Action: ActionInstall,
		InstanceID: &inst, RequestedBy: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	// 互斥查询命中 pending。
	inflight, err := s.Task().InFlight(ctx, ToolMongoExpress)
	if err != nil || inflight == nil || inflight.ID != id {
		t.Fatalf("in-flight = %+v err=%v", inflight, err)
	}
	// 异工具不互斥。
	if other, err := s.Task().InFlight(ctx, ToolPHPMyAdmin); err != nil || other != nil {
		t.Fatalf("other tool in-flight = %+v err=%v", other, err)
	}
	if err := s.Task().MarkRunning(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := s.Task().AppendOutput(ctx, id, "line1\nline2"); err != nil {
		t.Fatal(err)
	}
	task, err := s.Task().GetByID(ctx, id)
	if err != nil || task.Status != TaskRunning || task.StartedAt == nil || task.OutputTail != "line1\nline2" {
		t.Fatalf("running state: %+v err=%v", task, err)
	}
	errMsg := "sha256_mismatch"
	if err := s.Task().Finish(ctx, id, TaskFailed, &errMsg); err != nil {
		t.Fatal(err)
	}
	task, _ = s.Task().GetByID(ctx, id)
	if task.Status != TaskFailed || task.Error == nil || *task.Error != errMsg || task.FinishedAt == nil {
		t.Fatalf("failed state: %+v", task)
	}
	// 终态后互斥解除。
	if inflight, err := s.Task().InFlight(ctx, ToolMongoExpress); err != nil || inflight != nil {
		t.Fatalf("in-flight after finish = %+v err=%v", inflight, err)
	}
	// 不存在任务 → ErrTaskNotFound（DBT_005）。
	if _, err := s.Task().GetByID(ctx, 999); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("want ErrTaskNotFound, got %v", err)
	}
	// 筛选分页（created_at 倒序）。
	id2, _ := s.Task().Create(ctx, Task{ToolType: ToolPHPMyAdmin, Action: ActionInstall, RequestedBy: "admin"})
	rows, total, err := s.Task().List(ctx, "", "", 50, 0)
	if err != nil || total != 2 || len(rows) != 2 || rows[0].ID != id2 {
		t.Fatalf("list: total=%d rows=%d err=%v", total, len(rows), err)
	}
	rows, total, err = s.Task().List(ctx, ToolMongoExpress, TaskFailed, 50, 0)
	if err != nil || total != 1 || len(rows) != 1 || rows[0].ID != id {
		t.Fatalf("filtered list: total=%d err=%v", total, err)
	}
	// 重启恢复：running/pending 一并 failed（I-09）。先终态化分页用例任务。
	if err := s.Task().Finish(ctx, id2, TaskSucceeded, nil); err != nil {
		t.Fatal(err)
	}
	id3, _ := s.Task().Create(ctx, Task{ToolType: ToolMongoExpress, Action: ActionUpgrade, RequestedBy: "admin"})
	_ = s.Task().MarkRunning(ctx, id3)
	id4, _ := s.Task().Create(ctx, Task{ToolType: ToolPHPMyAdmin, Action: ActionUninstall, RequestedBy: "admin"})
	n, err := s.Task().FailRunningAll(ctx, "面板重启中断")
	if err != nil || n != 2 {
		t.Fatalf("recover n=%d err=%v", n, err)
	}
	for _, tid := range []int64{id3, id4} {
		task, _ := s.Task().GetByID(ctx, tid)
		if task.Status != TaskFailed || task.Error == nil || *task.Error != "面板重启中断" {
			t.Fatalf("recovered task %d: %+v", tid, task)
		}
	}
}

// TestTxTaskAuditAtomic 任务创建+发起审计同事务（I-08）：回滚则两者皆无。
func TestTxTaskAuditAtomic(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	tx, err := s.BeginTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id, err := tx.Task().Create(ctx, Task{ToolType: ToolPHPMyAdmin, Action: ActionInstall, RequestedBy: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Audit().Append(ctx, storeauth.AuditEntry{
		ActorName: "admin", Category: "change", Action: "dbt_install_started",
		ObjectType: "tool_deploy_task", ObjectID: "1", Result: "success", Source: "web",
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Task().GetByID(ctx, id); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("rollback should discard task, got %v", err)
	}
	recs, _, err := s.Audit().Search(ctx, storeauth.AuditCondition{Action: "dbt_install_started"}, 0, 10)
	if err != nil || len(recs) != 0 {
		t.Fatalf("rollback should discard audit: %d err=%v", len(recs), err)
	}
	// 提交路径：两者同在。
	tx, _ = s.BeginTx(ctx)
	id, _ = tx.Task().Create(ctx, Task{ToolType: ToolPHPMyAdmin, Action: ActionInstall, RequestedBy: "admin"})
	_ = tx.Audit().Append(ctx, storeauth.AuditEntry{
		ActorName: "admin", Category: "change", Action: "dbt_install_started",
		ObjectType: "tool_deploy_task", ObjectID: "1", Result: "success", Source: "web",
	}, time.Now())
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Task().GetByID(ctx, id); err != nil {
		t.Fatalf("committed task missing: %v", err)
	}
	recs, _, _ = s.Audit().Search(ctx, storeauth.AuditCondition{Action: "dbt_install_started"}, 0, 10)
	if len(recs) != 1 {
		t.Fatalf("committed audit missing: %d", len(recs))
	}
}
