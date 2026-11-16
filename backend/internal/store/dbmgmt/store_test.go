package dbmgmt

import (
	"context"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/store"
	storeauth "github.com/dougong/dougong/internal/store/auth"
)

// auditEntryForTest 同事务审计测试载荷（四要素齐备）。
func auditEntryForTest() storeauth.AuditEntry {
	return storeauth.AuditEntry{
		ActorName: "tester", Category: "change", Action: "instance_credentials_set",
		ObjectType: "db_instance", ObjectID: "1", Result: "success", Source: "web",
	}
}

// auditConditionAll 零条件（全量）。
func auditConditionAll() storeauth.AuditCondition { return storeauth.AuditCondition{} }

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(db)
}

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

func sampleInstance(key string) Instance {
	return Instance{
		IdentityKey: key, Engine: EngineMySQL, ConnMode: ConnModeTCP,
		Host: strPtr("localhost"), Port: intPtr(3306),
	}
}

func TestInstanceLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := s.Instance()

	// 探测 upsert：首次 INSERT（discovered=true）。
	inst := sampleInstance("mysql|tcp:localhost:3306")
	discovered, err := repo.UpsertDiscovered(ctx, inst, ConnStateReachable, nil)
	if err != nil || !discovered {
		t.Fatalf("first upsert: discovered=%v err=%v", discovered, err)
	}
	got, err := repo.GetByIdentity(ctx, "mysql|tcp:localhost:3306")
	if err != nil || got == nil {
		t.Fatalf("get by identity: %v %v", got, err)
	}
	if got.ConnState != ConnStateReachable || got.CredentialsSet() {
		t.Fatalf("unexpected row: %+v", got)
	}

	// 二次 upsert：整值更新（discovered=false）+ version 刷新。
	v := "8.0.36"
	discovered, err = repo.UpsertDiscovered(ctx, inst, ConnStateUnreachable, &v)
	if err != nil || discovered {
		t.Fatalf("second upsert: discovered=%v err=%v", discovered, err)
	}
	got, _ = repo.GetByIdentity(ctx, "mysql|tcp:localhost:3306")
	if got.ConnState != ConnStateUnreachable || got.Version == nil || *got.Version != "8.0.36" {
		t.Fatalf("updated row wrong: %+v", got)
	}

	// 凭据落库（I-09 同进；conn_state=reachable 语义）。
	if err := repo.UpdateCredentials(ctx, got.ID, "root", "cipher==", &v); err != nil {
		t.Fatalf("update credentials: %v", err)
	}
	got, _ = repo.GetByID(ctx, got.ID)
	if !got.CredentialsSet() || *got.CredUsername != "root" || *got.CredCipher != "cipher==" ||
		got.ConnState != ConnStateReachable || got.CredUpdatedAt == nil {
		t.Fatalf("credentials row wrong: %+v", got)
	}

	// 连接目标纠正一并落库（SQL #7，A-DB9）。
	if err := repo.UpdateTargetAndCredentials(ctx, got.ID,
		"mysql|socket:/tmp/mysql.sock", ConnModeSocket, strPtr("/tmp/mysql.sock"), nil, nil,
		"root", "cipher2==", &v); err != nil {
		t.Fatalf("update target: %v", err)
	}
	got, _ = repo.GetByID(ctx, got.ID)
	if got.IdentityKey != "mysql|socket:/tmp/mysql.sock" || got.ConnMode != ConnModeSocket ||
		got.Host != nil || got.Port != nil || *got.SocketPath != "/tmp/mysql.sock" {
		t.Fatalf("target row wrong: %+v", got)
	}

	// 引擎筛选 + 排序（SQL #1）。
	inst2 := sampleInstance("redis|tcp:localhost:6379")
	inst2.Engine = EngineRedis
	inst2.Port = intPtr(6379)
	if _, err := repo.UpsertDiscovered(ctx, inst2, ConnStateReachable, nil); err != nil {
		t.Fatal(err)
	}
	all, err := repo.List(ctx, "")
	if err != nil || len(all) != 2 {
		t.Fatalf("list all: %v %d", err, len(all))
	}
	onlyRedis, _ := repo.List(ctx, EngineRedis)
	if len(onlyRedis) != 1 || onlyRedis[0].Engine != EngineRedis {
		t.Fatalf("list filter: %+v", onlyRedis)
	}

	// 删除（SQL #5）。
	if err := repo.Delete(ctx, got.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	missing, _ := repo.GetByID(ctx, got.ID)
	if missing != nil {
		t.Fatalf("deleted row still present: %+v", missing)
	}
}

func TestTxCommitAndRollback(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// 回滚：审计写入后主动回滚，档案与审计均不落。
	tx, err := s.BeginTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Instance().UpsertDiscovered(ctx,
		sampleInstance("mysql|tcp:10.0.0.1:3306"), ConnStateReachable, nil); err != nil {
		t.Fatal(err)
	}
	if err := tx.Audit().Append(ctx, auditEntryForTest(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if inst, _ := s.Instance().GetByIdentity(ctx, "mysql|tcp:10.0.0.1:3306"); inst != nil {
		t.Fatal("rollback left instance row")
	}

	// 提交：档案与审计原子落库（R-03 同事务）。
	tx, err = s.BeginTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Instance().UpsertDiscovered(ctx,
		sampleInstance("mysql|tcp:10.0.0.1:3306"), ConnStateReachable, nil); err != nil {
		t.Fatal(err)
	}
	if err := tx.Audit().Append(ctx, auditEntryForTest(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if inst, _ := s.Instance().GetByIdentity(ctx, "mysql|tcp:10.0.0.1:3306"); inst == nil {
		t.Fatal("commit lost instance row")
	}
	n, total, err := s.Audit().Search(ctx, auditConditionAll(), 0, 10)
	if err != nil || total != 1 || len(n) != 1 {
		t.Fatalf("audit search: %v total=%d rows=%d", err, total, len(n))
	}
}
