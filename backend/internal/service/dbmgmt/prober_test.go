package dbmgmt

// 探测策略 v2.3 测试（D-21/Q1 二次拍板）：TCP 优先两阶段登记 + socket 兜底
// + rescan 保守清理（无凭据冗余 socket 行 DELETE+审计同事务，已配凭据保留）
// + instance/list redundantSocket 派生 + rescan merged 统计。reachable 全
// 注入（零 runner 实况依赖——Windows/Linux 行为一致）。

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/dougong/dougong/internal/adapter/dbconn"
	storeauth "github.com/dougong/dougong/internal/store/auth"
	storedb "github.com/dougong/dougong/internal/store/dbmgmt"
)

// reachBy 构造 reachable 注入（按 connMode+端口/路径判定）。
func reachTCP(ports ...int) func(dbconn.Target) bool {
	set := map[int]bool{}
	for _, p := range ports {
		set[p] = true
	}
	return func(t dbconn.Target) bool { return t.ConnMode == storedb.ConnModeTCP && set[t.Port] }
}

func reachAll(dbconn.Target) bool { return true }

func reachSocketOnly(t dbconn.Target) bool { return t.ConnMode == storedb.ConnModeSocket }

// seedSocketRow 直接落一条 socket 档案（可选凭据）。
func (e *testEnv) seedSocketRow(t *testing.T, engine, path string, withCreds bool) *storedb.Instance {
	t.Helper()
	ctx := context.Background()
	inst := storedb.Instance{
		IdentityKey: BuildIdentityKey(engine, storedb.ConnModeSocket, path, "", 0),
		Engine:      engine, ConnMode: storedb.ConnModeSocket, SocketPath: &path,
	}
	tx, err := e.svc.store.BeginTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Instance().UpsertDiscovered(ctx, inst, storedb.ConnStateReachable, nil); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	row, err := e.svc.store.Instance().GetByIdentity(ctx, inst.IdentityKey)
	if err != nil || row == nil {
		t.Fatalf("seed socket row: %v", err)
	}
	if withCreds {
		cipher, err := e.vault.Encrypt("secret-old")
		if err != nil {
			t.Fatal(err)
		}
		tx2, err := e.svc.store.BeginTx(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx2.Instance().UpdateCredentials(ctx, row.ID, "root", cipher, nil); err != nil {
			t.Fatal(err)
		}
		if err := tx2.Commit(); err != nil {
			t.Fatal(err)
		}
		row, _ = e.svc.store.Instance().GetByID(ctx, row.ID)
	}
	return row
}

// TCP 优先：mysql TCP 可达时 socket 即使可达也跳过登记（仅一行 TCP）。
func TestProbeTCPFirstSkipsSocketRegistration(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	prober := NewProber(e.svc.store, e.mysql, e.redis, e.mongo, e.vault, reachAll)
	res, err := prober.ProbeAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := e.svc.store.Instance().List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	// 三引擎各一行 TCP（mysql 3306/redis 6379/mongodb 27017），零 socket 行。
	if len(rows) != 3 {
		t.Fatalf("TCP-first must register 3 tcp rows only, got %d: %+v", len(rows), rows)
	}
	for _, r := range rows {
		if r.ConnMode != storedb.ConnModeTCP {
			t.Fatalf("socket row must be skipped when engine TCP reachable: %+v", r)
		}
	}
	if res.Discovered != 3 || !res.TCPReachable["mysql"] || !res.TCPReachable["redis"] || !res.TCPReachable["mongodb"] {
		t.Fatalf("probe result: %+v", res)
	}
}

// socket 兜底：engine 无任何本机 TCP 可达时 socket 登记（skip-networking）。
func TestProbeSocketFallbackWhenNoTCP(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	prober := NewProber(e.svc.store, e.mysql, e.redis, e.mongo, e.vault, reachSocketOnly)
	res, err := prober.ProbeAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := e.svc.store.Instance().List(ctx, "")
	if len(rows) == 0 {
		t.Fatal("socket fallback must register when no local TCP reachable")
	}
	for _, r := range rows {
		if r.ConnMode != storedb.ConnModeSocket {
			t.Fatalf("only socket rows expected: %+v", r)
		}
	}
	// mysql 4 个 socket 候选 + mongodb 1 个（redis 探测集无 socket 目标）。
	if res.Discovered != 5 {
		t.Fatalf("socket fallback discovered = %d, want 5", res.Discovered)
	}
	for engine, reachable := range res.TCPReachable {
		if reachable {
			t.Fatalf("engine %q must not be TCP reachable", engine)
		}
	}
}

// 既有 socket 档案在 TCP 可达轮：conn_state 仍整值刷新，行保留不 INSERT。
func TestProbeExistingSocketRefreshedNotInserted(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	sock := e.seedSocketRow(t, storedb.EngineMySQL, "/tmp/mysql.sock", false)
	// TCP 3306 与该 socket 路径均可达（跳过登记但既有档案整值刷新 reachable）。
	reach := func(t dbconn.Target) bool {
		return (t.ConnMode == storedb.ConnModeTCP && t.Port == 3306) ||
			(t.ConnMode == storedb.ConnModeSocket && t.SocketPath == "/tmp/mysql.sock")
	}
	prober := NewProber(e.svc.store, e.mysql, e.redis, e.mongo, e.vault, reach)
	res, err := prober.ProbeAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// TCP 行新登记 + 既有 socket 行刷新（updated 计数）。
	if res.Discovered != 1 || res.Updated != 1 {
		t.Fatalf("discovered=%d updated=%d, want 1/1", res.Discovered, res.Updated)
	}
	row, _ := e.svc.store.Instance().GetByID(ctx, sock.ID)
	if row == nil || row.ConnState != storedb.ConnStateReachable {
		t.Fatalf("existing socket row must be refreshed in place: %+v", row)
	}
	// socket 不可达轮：整值覆盖 unreachable（I-03）。
	p2 := NewProber(e.svc.store, e.mysql, e.redis, e.mongo, e.vault, reachTCP(3306))
	// reachTCP 对 socket 返回 false → socket 档案刷新 unreachable。
	if _, err := p2.ProbeAll(ctx); err != nil {
		t.Fatal(err)
	}
	row, _ = e.svc.store.Instance().GetByID(ctx, sock.ID)
	if row == nil || row.ConnState != storedb.ConnStateUnreachable {
		t.Fatalf("socket unreachable overwrite missing: %+v", row)
	}
}

// rescan 保守清理：无凭据冗余 socket 行 DELETE+审计 instance_merged 同事务；
// 已配凭据 socket 行保留；TCP 不可达 engine 的 socket 行保留。
func TestMergeRedundantSocketsConservative(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	bare := e.seedSocketRow(t, storedb.EngineMySQL, "/tmp/mysql.sock", false)
	cred := e.seedSocketRow(t, storedb.EngineMySQL, "/var/run/mysqld/mysqld.sock", true)
	// redis TCP 不可达 → 其 socket 行不清理（探测集 redis 无 socket，手工造行）。
	redisSock := e.seedSocketRow(t, storedb.EngineRedis, "/tmp/redis.sock", false)

	prober := NewProber(e.svc.store, e.mysql, e.redis, e.mongo, e.vault, reachTCP(3306))
	merged, err := prober.MergeRedundantSockets(ctx, map[string]bool{"mysql": true}, testActor())
	if err != nil || merged != 1 {
		t.Fatalf("merged = %d err = %v, want 1", merged, err)
	}
	if row, _ := e.svc.store.Instance().GetByID(ctx, bare.ID); row != nil {
		t.Fatal("credential-less redundant socket row must be deleted")
	}
	if row, _ := e.svc.store.Instance().GetByID(ctx, cred.ID); row == nil {
		t.Fatal("credentialed socket row must be preserved（避免凭据丢失）")
	}
	if row, _ := e.svc.store.Instance().GetByID(ctx, redisSock.ID); row == nil {
		t.Fatal("socket row of engine without TCP reachability must be preserved")
	}
	// 审计 instance_merged 同事务落库（detail 含 reason，R-03）。
	rows, total, err := e.auditQ.Search(ctx, storeauth.AuditCondition{Action: "instance_merged"}, 0, 10)
	if err != nil || total != 1 {
		t.Fatalf("instance_merged audit: %v total=%d", err, total)
	}
	if rows[0].Detail == nil || !strings.Contains(*rows[0].Detail, "redundant_socket_tcp_reachable") ||
		!strings.Contains(*rows[0].Detail, bare.IdentityKey) {
		t.Fatalf("instance_merged detail wrong: %+v", rows[0].Detail)
	}
	// 再清理：无剩余目标 → 0。
	merged, err = prober.MergeRedundantSockets(ctx, map[string]bool{"mysql": true}, testActor())
	if err != nil || merged != 0 {
		t.Fatalf("second merge = %d err = %v, want 0", merged, err)
	}
}

// Rescan 编排：merged 统计入响应与 instance_rescanned detail。
func TestRescanMergedStat(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	e.seedSocketRow(t, storedb.EngineMySQL, "/tmp/mysql.sock", false)
	e.svc.prober = NewProber(e.svc.store, e.mysql, e.redis, e.mongo, e.vault, reachTCP(3306))

	data, err := e.svc.Rescan(ctx, testActor())
	if err != nil {
		t.Fatal(err)
	}
	if data.Merged != 1 || data.Discovered != 1 {
		t.Fatalf("rescan data: %+v, want merged=1 discovered=1", data)
	}
	rows, _, err := e.auditQ.Search(ctx, storeauth.AuditCondition{Action: "instance_rescanned"}, 0, 10)
	if err != nil || len(rows) == 0 || rows[0].Detail == nil {
		t.Fatalf("rescan audit: %v", err)
	}
	if !strings.Contains(*rows[0].Detail, `"merged":1`) {
		t.Fatalf("rescan detail must carry merged stat: %s", *rows[0].Detail)
	}
}

// 自动探测（首次 list）不清理不写审计（D-07 系统机制口径不变）。
func TestAutoProbeDoesNotMerge(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	sock := e.seedSocketRow(t, storedb.EngineMySQL, "/tmp/mysql.sock", false)
	e.svc.prober = NewProber(e.svc.store, e.mysql, e.redis, e.mongo, e.vault, reachTCP(3306))
	e.svc.discoverOnce = sync.Once{}

	if _, err := e.svc.ListInstances(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if row, _ := e.svc.store.Instance().GetByID(ctx, sock.ID); row == nil {
		t.Fatal("auto discovery must not merge（仅 rescan 保守清理）")
	}
	if e.hasAudit(ctx, "instance_merged") {
		t.Fatal("auto discovery must not write instance_merged audit")
	}
}

// redundantSocket 派生：本机 TCP 可达档案存在 → socket 行 true；远程 TCP
// 档案不触发；TCP unreachable 不触发。
func TestListRedundantSocketDerivation(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	sock := e.seedSocketRow(t, storedb.EngineMySQL, "/tmp/mysql.sock", true)
	e.seedInstance(t, storedb.EngineMySQL) // mysql|tcp:localhost:3306 reachable

	data, err := e.svc.ListInstances(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	var sockItem, tcpItem *InstanceItem
	for i := range data.Items {
		if data.Items[i].ID == sock.ID {
			sockItem = &data.Items[i]
		} else {
			tcpItem = &data.Items[i]
		}
	}
	if sockItem == nil || !sockItem.RedundantSocket {
		t.Fatalf("socket row must be flagged redundant: %+v", sockItem)
	}
	if tcpItem == nil || tcpItem.RedundantSocket {
		t.Fatalf("tcp row must not be flagged: %+v", tcpItem)
	}
	// TCP 档案 unreachable → 不标注。
	e.setConnState(t, tcpItem.ID, storedb.ConnStateUnreachable)
	data, _ = e.svc.ListInstances(ctx, "")
	for _, item := range data.Items {
		if item.ID == sock.ID && item.RedundantSocket {
			t.Fatal("unreachable TCP must not flag redundant")
		}
	}
	// 远程 TCP 档案（host 非本机）reachable → 不标注（D-21 远程不受影响）。
	e.setConnState(t, tcpItem.ID, storedb.ConnStateReachable)
	remote := storedb.Instance{
		IdentityKey: BuildIdentityKey(storedb.EngineMySQL, storedb.ConnModeTCP, "", "10.0.0.8", 3306),
		Engine:      storedb.EngineMySQL, ConnMode: storedb.ConnModeTCP,
		Host: strPtrOf("10.0.0.8"), Port: intPtrOf(3306),
	}
	tx, err := e.svc.store.BeginTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Instance().UpsertDiscovered(ctx, remote, storedb.ConnStateReachable, nil); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	// 将本机 TCP 档案改远程形态不可行（identity 唯一）——改判据：删除本机
	// TCP 行后仅远程 TCP 行存在，socket 不应被标注。
	tx2, _ := e.svc.store.BeginTx(ctx)
	if err := tx2.Instance().Delete(ctx, tcpItem.ID); err != nil {
		t.Fatal(err)
	}
	if err := tx2.Commit(); err != nil {
		t.Fatal(err)
	}
	data, _ = e.svc.ListInstances(ctx, "")
	for _, item := range data.Items {
		if item.ID == sock.ID && item.RedundantSocket {
			t.Fatal("remote TCP archive must not flag local socket redundant")
		}
	}
}
