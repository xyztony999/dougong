package dbmgmt

// Gate 2 W1 修复轮（2026-11-16）补充用例：覆盖 gate2 报告 §7 W1 指出的
// service/dbmgmt 分支余量——网关错误翻译全集（translateGatewayErr）、读路径
// （ListDatabases/ListCharsets/OHS 三入口）、引擎实况探测（engineProbe）、
// 导出流中途收尾（Broken/CountingWriter）、审计降级（recordAudit fail-open）、
// 凭据解密失败脱敏（decryptCreds→INTERNAL）、各写路径网关错误分支与入参校验
// 分支。adapter/dbmysql/dbredis 两包仍为引擎二进制环境绑定（W1 维持部分，归
// Linux 目标环境冒烟）。

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/dougong/dougong/internal/adapter/dbconn"
	"github.com/dougong/dougong/internal/adapter/dbmysql"
	"github.com/dougong/dougong/internal/adapter/dbredis"
	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	"github.com/dougong/dougong/internal/store"
	storedb "github.com/dougong/dougong/internal/store/dbmgmt"
)

// ---- 网关错误翻译（service.go translateGatewayErr 全哨兵表）----

func TestTranslateGatewayErrTable(t *testing.T) {
	wrapped := func(err error, reason string) error {
		return fmt.Errorf("%w: %s", err, reason)
	}
	cases := []struct {
		name string
		in   error
		want string // 期望错误码符号；""=nil，"@"=原样透传（非 errcode）
	}{
		{"nil", nil, ""},
		{"errcode passthrough", errcode.ErrDBMInstanceBusy, "DBM_016"},
		{"mysql auth rejected", dbmysql.ErrAuthRejected, "DBM_005"},
		{"redis auth rejected wrapped", wrapped(dbredis.ErrAuthRejected, "WRONGPASS"), "DBM_005"},
		{"mysql conn failed", dbmysql.ErrConnFailed, "DBM_006"},
		{"redis conn failed wrapped", wrapped(dbredis.ErrConnFailed, "dial refused"), "DBM_006"},
		{"database exists", dbmysql.ErrDatabaseExists, "DBM_008"},
		{"database not found", dbmysql.ErrDatabaseNotFound, "DBM_007"},
		{"user exists", dbmysql.ErrUserExists, "DBM_010"},
		{"user not found", dbmysql.ErrUserNotFound, "DBM_009"},
		{"grant missing", dbmysql.ErrGrantMissing, "DBM_011"},
		{"redis key not found", dbredis.ErrKeyNotFound, "DBM_015"},
		{"dump failed", dbmysql.ErrDumpFailed, "DBM_012"},
		{"import failed wrapped", wrapped(dbmysql.ErrImportFailed, "syntax"), "DBM_013"},
		{"unknown passthrough", errors.New("boom"), "@"},
	}
	for _, c := range cases {
		got := translateGatewayErr(c.in)
		if c.want == "" {
			if got != nil {
				t.Errorf("%s: want nil, got %v", c.name, got)
			}
			continue
		}
		if c.want == "@" {
			if !errors.Is(got, c.in) {
				t.Errorf("%s: unknown error must pass through, got %v", c.name, got)
			}
			continue
		}
		var ec *errcode.Error
		if !errors.As(got, &ec) || ec.Symbol() != c.want {
			t.Errorf("%s: want %s, got %v", c.name, c.want, got)
		}
	}
	// reasonOf 剥首个冒号前的包前缀（哨兵本体文本随 reason 段保留，生产口径）。
	if got := reasonOf(wrapped(dbmysql.ErrDumpFailed, "exit 5")); got != "mysqldump failed: exit 5" {
		t.Errorf("reasonOf = %q", got)
	}
	if got := reasonOf(errors.New("no-colon")); got != "no-colon" {
		t.Errorf("reasonOf no colon = %q", got)
	}
}

// ---- MySQL 观测读路径（mysql.go ListDatabases/ListCharsets/Status/ProcessList）----

func TestMySQLReadPathsAndErrors(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	inst := e.seedInstance(t, storedb.EngineMySQL)

	e.mysql.dbs = []dbmysql.DatabaseInfo{{Name: "appdb", Charset: "utf8mb4"}, {Name: "mysql", Charset: "utf8mb4"}}
	dbs, err := e.svc.ListDatabases(ctx, itoa(inst.ID))
	if err != nil || len(dbs.Items) != 2 || dbs.Items[0].Name != "appdb" {
		t.Fatalf("list databases: %+v %v", dbs, err)
	}
	charsets, err := e.svc.ListCharsets(ctx, itoa(inst.ID))
	if err != nil || len(charsets.Items) != 2 {
		t.Fatalf("list charsets: %+v %v", charsets, err)
	}
	// nil 候选 → 空数组（契约 items 恒数组形态）。
	e.mysql.charset = nil
	charsets, err = e.svc.ListCharsets(ctx, itoa(inst.ID))
	if err != nil || charsets.Items == nil || len(charsets.Items) != 0 {
		t.Fatalf("nil charsets should normalize to empty slice: %+v %v", charsets, err)
	}

	// 网关错误分支（ErrConnFailed 哨兵 → DBM_006）。
	connErr := fmt.Errorf("%w: dial tcp refused", dbmysql.ErrConnFailed)
	e.mysql.queryErr = connErr
	if _, err := e.svc.ListDatabases(ctx, itoa(inst.ID)); errCode(t, err) != "DBM_006" {
		t.Fatalf("list databases want DBM_006, got %v", err)
	}
	e.mysql.queryErr = nil
	e.mysql.charsetErr = connErr
	if _, err := e.svc.ListCharsets(ctx, itoa(inst.ID)); errCode(t, err) != "DBM_006" {
		t.Fatalf("list charsets want DBM_006, got %v", err)
	}
	e.mysql.charsetErr = nil

	// 进程列表行映射（含可空字段）+ 错误分支。
	e.mysql.process = []dbmysql.ProcessRow{
		{ID: 1, User: "root", Host: "localhost", DB: nil, Command: "Query", TimeSec: 3, State: strPtrOf("executing"), Info: nil},
		{ID: 2, User: "app", Host: "10.0.0.2:5", DB: strPtrOf("appdb"), Command: "Sleep", TimeSec: 0, Info: strPtrOf("NULL")},
	}
	pl, err := e.svc.MySQLProcessList(ctx, itoa(inst.ID))
	if err != nil || len(pl.Items) != 2 || pl.Items[0].DB != nil || *pl.Items[1].DB != "appdb" {
		t.Fatalf("processlist mapping: %+v %v", pl, err)
	}
	e.mysql.processErr = connErr
	if _, err := e.svc.MySQLProcessList(ctx, itoa(inst.ID)); errCode(t, err) != "DBM_006" {
		t.Fatalf("processlist want DBM_006, got %v", err)
	}
	e.mysql.processErr = nil
	e.mysql.statusErr = connErr
	if _, err := e.svc.MySQLStatus(ctx, itoa(inst.ID)); errCode(t, err) != "DBM_006" {
		t.Fatalf("status want DBM_006, got %v", err)
	}
	e.mysql.statusErr = nil
}

// ---- OHS 三入口（ohs.go DumpStream/DatabaseNames/InstanceArchive）----

func TestOHSDumpStreamDatabaseNamesArchive(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	inst := e.seedInstance(t, storedb.EngineMySQL)
	e.mysql.dbs = []dbmysql.DatabaseInfo{
		{Name: "mysql", Charset: "utf8mb4"},
		{Name: "information_schema", Charset: "utf8mb3"},
		{Name: "performance_schema", Charset: "utf8mb4"},
		{Name: "sys", Charset: "utf8mb4"},
		{Name: "zzz", Charset: "latin1"},
		{Name: "appdb", Charset: "utf8mb4"},
	}
	e.mysql.dumpBody = "-- cron-backup dump"

	// DatabaseNames：滤系统库 + 字典序稳定产出（A-C6）。
	names, err := e.svc.DatabaseNames(ctx, inst.ID)
	if err != nil || strings.Join(names, ",") != "appdb,mysql,zzz" {
		t.Fatalf("database names: %v %v", names, err)
	}
	// 错误分支。
	e.mysql.queryErr = fmt.Errorf("%w: dial refused", dbmysql.ErrConnFailed)
	if _, err := e.svc.DatabaseNames(ctx, inst.ID); errCode(t, err) != "DBM_006" {
		t.Fatalf("database names want DBM_006, got %v", err)
	}

	// DumpStream：charset 尽力解析 + 流透传（失败不阻塞，charset 按库实况命中）。
	stream, err := e.svc.DumpStream(ctx, inst.ID, "appdb")
	if err != nil {
		t.Fatal(err)
	}
	body, rerr := io.ReadAll(stream)
	if rerr != nil || string(body) != "-- cron-backup dump" {
		t.Fatalf("dump stream: %q %v", body, rerr)
	}
	_ = stream.Close()
	// ListDatabases 失败不阻塞（charset 空串走默认字符集）。
	e.mysql.dumpBody = "-- default charset"
	stream2, err := e.svc.DumpStream(ctx, inst.ID, "appdb")
	if err != nil {
		t.Fatal(err)
	}
	_ = stream2.Close()
	// 前置链：redis 档案对 mysql 族 OHS → DBM_018。
	redisInst := e.seedInstance(t, storedb.EngineRedis)
	if _, err := e.svc.DumpStream(ctx, redisInst.ID, "appdb"); errCode(t, err) != "DBM_018" {
		t.Fatalf("dump stream on redis want DBM_018, got %v", err)
	}
	e.mysql.queryErr = nil

	// InstanceArchive：命中（engine + hostPortLabel）与未命中（found=false 不报错）。
	found, engine, label, err := e.svc.InstanceArchive(ctx, inst.ID)
	if err != nil || !found || engine != storedb.EngineMySQL || label != "localhost-3306" {
		t.Fatalf("archive hit: %v %s %s %v", found, engine, label, err)
	}
	found, _, _, err = e.svc.InstanceArchive(ctx, 9999)
	if err != nil || found {
		t.Fatalf("archive miss must be (false, nil err), got %v %v", found, err)
	}
}

// ---- 探测：engineProbe 实况刷新（prober.go 131）与 ProbeAll 容错 ----

func TestProbeEngineRefreshBranches(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	inst := e.seedInstance(t, storedb.EngineMySQL)  // mysql|tcp:localhost:3306 有凭据
	rinst := e.seedInstance(t, storedb.EngineRedis) // redis|tcp:localhost:6379 有凭据
	reach := func(t dbconn.Target) bool { return t.ConnMode == "tcp" && (t.Port == 3306 || t.Port == 6379) }
	prober := NewProber(e.svc.store, e.mysql, e.redis, e.vault, reach)

	// Ping 成功 → reachable + version 实况刷新（双引擎）。
	if _, updated, err := prober.ProbeAll(ctx); err != nil || updated < 2 {
		t.Fatalf("probe refresh: updated=%d err=%v", updated, err)
	}
	row, _ := e.svc.store.Instance().GetByID(ctx, inst.ID)
	if row.Version == nil || *row.Version != "8.0.36" {
		t.Fatalf("mysql version not refreshed: %+v", row)
	}
	rrow, _ := e.svc.store.Instance().GetByID(ctx, rinst.ID)
	if rrow.Version == nil || *rrow.Version != "7.2.4" {
		t.Fatalf("redis version not refreshed: %+v", rrow)
	}

	// 网络层失败 → unreachable（I-03 整值覆盖）。
	e.mysql.pingErr = dbmysql.ErrConnFailed
	if _, _, err := prober.ProbeAll(ctx); err != nil {
		t.Fatal(err)
	}
	row, _ = e.svc.store.Instance().GetByID(ctx, inst.ID)
	if row.ConnState != storedb.ConnStateUnreachable {
		t.Fatalf("conn failed should overwrite unreachable: %+v", row)
	}

	// 认证失败 → reachable 保持（实例活着，凭据问题归治理链 A-DB4）。
	e.mysql.pingErr = dbmysql.ErrAuthRejected
	if _, _, err := prober.ProbeAll(ctx); err != nil {
		t.Fatal(err)
	}
	row, _ = e.svc.store.Instance().GetByID(ctx, inst.ID)
	if row.ConnState != storedb.ConnStateReachable {
		t.Fatalf("auth rejected must stay reachable: %+v", row)
	}

	// 解密失败 → reachable 保持、version 不动（凭据问题归治理链）。
	e.mysql.pingErr = nil
	tx, err := e.svc.store.BeginTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Instance().UpdateCredentials(ctx, inst.ID, "root", "not-base64!!", nil); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := prober.ProbeAll(ctx); err != nil {
		t.Fatal(err)
	}
	row, _ = e.svc.store.Instance().GetByID(ctx, inst.ID)
	if row.ConnState != storedb.ConnStateReachable {
		t.Fatalf("decrypt failure must stay reachable: %+v", row)
	}

	// 网关缺席（nil）防御分支：传输可达即 reachable、无 version。
	nilProber := NewProber(e.svc.store, nil, nil, e.vault, reach)
	if _, _, err := nilProber.ProbeAll(ctx); err != nil {
		t.Fatal(err)
	}
	row, _ = e.svc.store.Instance().GetByID(ctx, inst.ID)
	if row.ConnState != storedb.ConnStateReachable {
		t.Fatalf("nil gateway defense: %+v", row)
	}
}

// ProbeAll 单目标失败不中断整体（store 读写异常 → 记日志继续，D-07 尽力而为）。
func TestProbeAllContinuesOnStoreError(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	st := storedb.New(db)
	if err := db.Close(); err != nil { // 关库后一切读写报错（错误注入）
		t.Fatal(err)
	}
	p := NewProber(AdaptStore(st), &fakeMySQL{}, &fakeRedis{}, nil, func(dbconn.Target) bool { return true })
	discovered, updated, err := p.ProbeAll(context.Background())
	if err != nil || discovered != 0 || updated != 0 {
		t.Fatalf("probe must degrade to 0/0 nil on store failure: %d %d %v", discovered, updated, err)
	}
	// ports.go BeginTx 错误分支（关库后 BeginTx 如实报错）。
	if _, err := AdaptStore(st).BeginTx(context.Background()); err == nil {
		t.Fatal("BeginTx on closed store should fail")
	}
}

// 首次 list 自动探测失败 → slogWarn 降级不阻塞清单（instance.go:22-26）。
func TestListInstancesFirstProbeDegrades(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	e.seedInstance(t, storedb.EngineMySQL)

	// 重置 discoverOnce 并以已取消 ctx 触发首探失败（ProbeAll 首 target 即返回）。
	e.svc.discoverOnce = sync.Once{}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	data, err := e.svc.ListInstances(cctx, "")
	if err == nil && data == nil {
		t.Fatal("degraded list must still resolve (data or error), not neither")
	}
	// mariadb 合法筛选分支（空集如实）。
	data, err = e.svc.ListInstances(ctx, "mariadb")
	if err != nil || len(data.Items) != 0 {
		t.Fatalf("mariadb filter: %+v %v", data, err)
	}
}

// ---- 写路径网关错误与校验分支（mysql.go database/dbuser）----

func TestDatabaseMutationErrorBranches(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	inst := e.seedInstance(t, storedb.EngineMySQL)
	actor := testActor()
	e.mysql.dbs = []dbmysql.DatabaseInfo{{Name: "appdb", Charset: "utf8mb4"}}
	connErr := fmt.Errorf("%w: dial refused", dbmysql.ErrConnFailed)

	// CreateDatabase：ShowCharsets 失败 / DatabaseExists 失败 / 建库执行失败。
	e.mysql.charsetErr = connErr
	if _, err := e.svc.CreateDatabase(ctx, actor, DatabaseCreateRequest{InstanceID: inst.ID, Name: "dbx"}); errCode(t, err) != "DBM_006" {
		t.Fatalf("charset err want DBM_006, got %v", err)
	}
	e.mysql.charsetErr = nil
	e.mysql.queryErr = connErr // DatabaseExists 走 ListDatabases → queryErr
	if _, err := e.svc.CreateDatabase(ctx, actor, DatabaseCreateRequest{InstanceID: inst.ID, Name: "dbx"}); errCode(t, err) != "DBM_006" {
		t.Fatalf("exists err want DBM_006, got %v", err)
	}
	// 建库执行失败（audit 失败分支）。
	e.mysql.queryErr = nil
	e.mysql.createExecErr = connErr
	if _, err := e.svc.CreateDatabase(ctx, actor, DatabaseCreateRequest{InstanceID: inst.ID, Name: "dbx"}); errCode(t, err) != "DBM_006" {
		t.Fatalf("create exec err want DBM_006, got %v", err)
	}
	e.mysql.createExecErr = nil

	// DeleteDatabase：库名非法 / exists 失败 / 删除执行失败。
	if _, err := e.svc.DeleteDatabase(ctx, actor, DatabaseDeleteRequest{InstanceID: inst.ID, Name: "bad;name"}); errCode(t, err) != "AUTH_012" {
		t.Fatalf("bad name want AUTH_012, got %v", err)
	}
	e.mysql.queryErr = connErr
	if _, err := e.svc.DeleteDatabase(ctx, actor, DatabaseDeleteRequest{InstanceID: inst.ID, Name: "appdb"}); errCode(t, err) != "DBM_006" {
		t.Fatalf("delete exists err want DBM_006, got %v", err)
	}
	e.mysql.queryErr = nil
	e.mysql.dropExecErr = connErr
	if _, err := e.svc.DeleteDatabase(ctx, actor, DatabaseDeleteRequest{InstanceID: inst.ID, Name: "appdb"}); errCode(t, err) != "DBM_006" {
		t.Fatalf("delete exec err want DBM_006, got %v", err)
	}
	e.mysql.dropExecErr = nil
}

func TestDBUserGrantErrorBranches(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	inst := e.seedInstance(t, storedb.EngineMySQL)
	actor := testActor()
	e.mysql.dbs = []dbmysql.DatabaseInfo{{Name: "appdb", Charset: "utf8mb4"}}
	connErr := fmt.Errorf("%w: dial refused", dbmysql.ErrConnFailed)

	// ListDBUsers：全局行/未识别档位过滤（D-10）+ 清单失败。
	e.mysql.users = []dbmysql.UserGrants{{
		Username: "app", Host: "%",
		Grants: []dbmysql.GrantLine{
			{DB: "*", Raw: "GRANT ALL PRIVILEGES ON *.* TO 'app'@'%'"},
			{DB: "db_sub", Raw: "GRANT SELECT, DROP ON `db_sub`.* TO 'app'@'%'"},
			{DB: "zzz", Raw: grantRawOf(LevelReadonly, "zzz", "app", "%")},
			{DB: "appdb", Raw: grantRawOf(LevelWrite, "appdb", "app", "%")},
			{DB: "aaa", Raw: grantRawOf(LevelAll, "aaa", "app", "%")},
		},
	}}
	list, err := e.svc.ListDBUsers(ctx, itoa(inst.ID))
	if err != nil || len(list.Items) != 1 {
		t.Fatalf("list users: %+v %v", list, err)
	}
	got := list.Items[0].Grants
	if len(got) != 3 { // 全局行与子集行不呈现；三档各一条
		t.Fatalf("grant filter: %+v", got)
	}
	// 呈现顺序沿用网关行序（sortGrantItems 未接线，见该函数直接单测）。
	if got[0].Database != "zzz" || got[0].Level != LevelReadonly ||
		got[1].Database != "appdb" || got[1].Level != LevelWrite ||
		got[2].Database != "aaa" || got[2].Level != LevelAll {
		t.Fatalf("grant projection wrong: %+v", got)
	}
	e.mysql.listUsersErr = connErr
	if _, err := e.svc.ListDBUsers(ctx, itoa(inst.ID)); errCode(t, err) != "DBM_006" {
		t.Fatalf("list users err want DBM_006, got %v", err)
	}
	e.mysql.listUsersErr = nil

	// CreateDBUser：三段校验 + 建用户执行失败。
	if _, err := e.svc.CreateDBUser(ctx, actor, DBUserCreateRequest{InstanceID: inst.ID, Username: "a b", Host: "%", Password: "pw"}); errCode(t, err) != "AUTH_012" {
		t.Fatalf("bad username want AUTH_012, got %v", err)
	}
	if _, err := e.svc.CreateDBUser(ctx, actor, DBUserCreateRequest{InstanceID: inst.ID, Username: "app", Host: "bad host", Password: "pw"}); errCode(t, err) != "AUTH_012" {
		t.Fatalf("bad host want AUTH_012, got %v", err)
	}
	if _, err := e.svc.CreateDBUser(ctx, actor, DBUserCreateRequest{InstanceID: inst.ID, Username: "app", Host: "%", Password: ""}); errCode(t, err) != "AUTH_012" {
		t.Fatalf("empty password want AUTH_012, got %v", err)
	}
	e.mysql.createUserEr = fmt.Errorf("%w: WRONGPASS", dbmysql.ErrAuthRejected)
	if _, err := e.svc.CreateDBUser(ctx, actor, DBUserCreateRequest{InstanceID: inst.ID, Username: "svc", Host: "10.0.0.%", Password: "pw"}); errCode(t, err) != "DBM_005" {
		t.Fatalf("create exec err want DBM_005, got %v", err)
	}
	e.mysql.createUserEr = nil

	// DeleteDBUser：校验 / 不存在 DBM_009 / 删除执行失败。
	if _, err := e.svc.DeleteDBUser(ctx, actor, DBUserRevokeRequest{InstanceID: inst.ID, Username: "", Host: "%"}); errCode(t, err) != "AUTH_012" {
		t.Fatalf("empty username want AUTH_012, got %v", err)
	}
	if _, err := e.svc.DeleteDBUser(ctx, actor, DBUserRevokeRequest{InstanceID: inst.ID, Username: "ghost", Host: "%"}); errCode(t, err) != "DBM_009" {
		t.Fatalf("ghost want DBM_009, got %v", err)
	}
	e.mysql.dropUserErr = connErr
	if _, err := e.svc.DeleteDBUser(ctx, actor, DBUserRevokeRequest{InstanceID: inst.ID, Username: "app", Host: "%"}); errCode(t, err) != "DBM_006" {
		t.Fatalf("drop exec err want DBM_006, got %v", err)
	}
	e.mysql.dropUserErr = nil

	// GrantDBUser：level 枚举 / 用户不存在 / 库不存在 / exists 失败 / 授权执行失败。
	if _, err := e.svc.GrantDBUser(ctx, actor, DBUserGrantRequest{InstanceID: inst.ID, Username: "app", Host: "%", Database: "appdb", Level: "admin"}); errCode(t, err) != "AUTH_012" {
		t.Fatalf("bad level want AUTH_012, got %v", err)
	}
	if _, err := e.svc.GrantDBUser(ctx, actor, DBUserGrantRequest{InstanceID: inst.ID, Username: "ghost", Host: "%", Database: "appdb", Level: LevelReadonly}); errCode(t, err) != "DBM_009" {
		t.Fatalf("ghost grant want DBM_009, got %v", err)
	}
	if _, err := e.svc.GrantDBUser(ctx, actor, DBUserGrantRequest{InstanceID: inst.ID, Username: "app", Host: "%", Database: "nodb", Level: LevelReadonly}); errCode(t, err) != "DBM_007" {
		t.Fatalf("nodb grant want DBM_007, got %v", err)
	}
	e.mysql.queryErr = connErr
	if _, err := e.svc.GrantDBUser(ctx, actor, DBUserGrantRequest{InstanceID: inst.ID, Username: "app", Host: "%", Database: "appdb", Level: LevelReadonly}); errCode(t, err) != "DBM_006" {
		t.Fatalf("grant exists err want DBM_006, got %v", err)
	}
	e.mysql.queryErr = nil
	e.mysql.grantErr = connErr
	if _, err := e.svc.GrantDBUser(ctx, actor, DBUserGrantRequest{InstanceID: inst.ID, Username: "app", Host: "%", Database: "appdb", Level: LevelReadonly}); errCode(t, err) != "DBM_006" {
		t.Fatalf("grant exec err want DBM_006, got %v", err)
	}
	e.mysql.grantErr = nil

	// RevokeDBUser：用户不存在 / 回收执行失败。
	if _, err := e.svc.RevokeDBUser(ctx, actor, DBUserRevokeRequest{InstanceID: inst.ID, Username: "ghost", Host: "%", Database: "appdb"}); errCode(t, err) != "DBM_009" {
		t.Fatalf("ghost revoke want DBM_009, got %v", err)
	}
	// 先授一条可回收授权，再注入回收失败。
	if _, err := e.svc.GrantDBUser(ctx, actor, DBUserGrantRequest{InstanceID: inst.ID, Username: "app", Host: "%", Database: "appdb", Level: LevelReadonly}); err != nil {
		t.Fatal(err)
	}
	e.mysql.revokeErr = connErr
	if _, err := e.svc.RevokeDBUser(ctx, actor, DBUserRevokeRequest{InstanceID: inst.ID, Username: "app", Host: "%", Database: "appdb"}); errCode(t, err) != "DBM_006" {
		t.Fatalf("revoke exec err want DBM_006, got %v", err)
	}
	e.mysql.revokeErr = nil
}

// ---- 改密编排错误分支 + 审计降级 fail-open（service.go recordAudit）----

// failingRecorder 审计持续失败（recordAudit 重试一次后降级日志，不影响操作）。
type failingRecorder struct{}

func (failingRecorder) Record(ctx context.Context, e audit.Entry) error {
	return errors.New("audit backend down")
}

func TestPasswordChangeErrorBranches(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	inst := e.seedInstance(t, storedb.EngineMySQL)
	rinst := e.seedInstance(t, storedb.EngineRedis)
	actor := testActor()

	// 空新密码 → AUTH_012（mysql/redis 双入口）。
	if _, err := e.svc.ChangeRootPassword(ctx, actor, PasswordRequest{InstanceID: inst.ID, NewPassword: ""}); errCode(t, err) != "AUTH_012" {
		t.Fatalf("mysql empty password want AUTH_012, got %v", err)
	}
	if _, err := e.svc.SetRedisPassword(ctx, actor, PasswordRequest{InstanceID: rinst.ID, NewPassword: ""}); errCode(t, err) != "AUTH_012" {
		t.Fatalf("redis empty password want AUTH_012, got %v", err)
	}
	// 引擎侧失败：认证被拒不落库（D-03/D-05）。
	e.mysql.queryErr = fmt.Errorf("%w: WRONGPASS", dbmysql.ErrAuthRejected)
	_, err := e.svc.ChangeRootPassword(ctx, actor, PasswordRequest{InstanceID: inst.ID, NewPassword: "new-pw"})
	if errCode(t, err) != "DBM_005" {
		t.Fatalf("mysql engine err want DBM_005, got %v", err)
	}
	after, _ := e.svc.store.Instance().GetByID(ctx, inst.ID)
	plain, _ := e.vault.Decrypt(*after.CredCipher)
	if plain != "secret-old" {
		t.Fatalf("failed engine change must not touch cipher: %q", plain)
	}
	e.mysql.queryErr = nil
	e.redis.setPassErr = fmt.Errorf("%w: dial refused", dbredis.ErrConnFailed)
	if _, err := e.svc.SetRedisPassword(ctx, actor, PasswordRequest{InstanceID: rinst.ID, NewPassword: "redis-pw"}); errCode(t, err) != "DBM_006" {
		t.Fatalf("redis engine err want DBM_006, got %v", err)
	}
	e.redis.setPassErr = nil

	// 审计持续失败 → 重试后降级运行日志，操作仍成功（fail-open 语义）。
	origAudit := e.svc.audit
	e.svc.audit = failingRecorder{}
	data, err := e.svc.ChangeRootPassword(ctx, actor, PasswordRequest{InstanceID: inst.ID, NewPassword: "audit-down-pw"})
	e.svc.audit = origAudit
	if err != nil || !data.Synced {
		t.Fatalf("audit outage must not fail operation (fail-open): %+v %v", data, err)
	}
	after, _ = e.svc.store.Instance().GetByID(ctx, inst.ID)
	plain, _ = e.vault.Decrypt(*after.CredCipher)
	if plain != "audit-down-pw" {
		t.Fatalf("cipher should sync despite audit outage: %q", plain)
	}
}

// ---- 导出流中途收尾（backup.go Broken/CountingWriter）----

func TestExportBrokenAndCountingWriter(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	inst := e.seedInstance(t, storedb.EngineMySQL)
	e.mysql.dbs = []dbmysql.DatabaseInfo{{Name: "appdb", Charset: "utf8mb4"}}
	e.mysql.dumpBody = "-- dump body"

	stream, err := e.svc.ExportDatabase(ctx, testActor(), inst.ID, "appdb")
	if err != nil {
		t.Fatal(err)
	}
	stream.Broken(42) // 中途断流：busy 解除 + 失败审计（transferredBytes）
	if e.svc.busy.Busy(inst.ID) {
		t.Fatal("busy not released by Broken")
	}
	// busy 解除后可再次导出（收尾用 Abort 归位）。
	rel, err := e.svc.ExportDatabase(ctx, testActor(), inst.ID, "appdb")
	if err != nil {
		t.Fatalf("re-export after Broken: %v", err)
	}
	if _, aerr := PeekExport(rel); aerr != nil {
		t.Fatal(aerr)
	}
	_ = rel.Abort(errors.New("done"))

	// CountingWriter 计数透传（A-DB7 导出审计 sizeBytes 载体）。
	var buf strings.Builder
	cw := &CountingWriter{W: &buf}
	if n, err := cw.Write([]byte("12345")); n != 5 || err != nil {
		t.Fatalf("counting write: %d %v", n, err)
	}
	if _, err := cw.Write([]byte("67890")); err != nil {
		t.Fatal(err)
	}
	if cw.N != 10 || buf.String() != "1234567890" {
		t.Fatalf("counting accum: N=%d buf=%q", cw.N, buf.String())
	}
}

// ---- 凭据链失败分支（decryptCreds 脱敏 / vault 加解密错误）----

func TestCredentialFailureBranches(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	inst := e.seedInstance(t, storedb.EngineMySQL)

	// decryptCreds 守卫分支（vault nil / 密文缺失）。
	bare := &storedb.Instance{ID: 1, CredUsername: strPtrOf("root")}
	if _, err := decryptCreds(nil, bare); err == nil {
		t.Fatal("nil vault should fail")
	}
	v := e.vault
	if _, err := decryptCreds(v, &storedb.Instance{ID: 1, CredUsername: strPtrOf("root")}); err == nil {
		t.Fatal("nil cipher should fail")
	}

	// 端到端：密文损坏 → 治理链 INTERNAL（细节脱敏，I-02）。
	tx, err := e.svc.store.BeginTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Instance().UpdateCredentials(ctx, inst.ID, "root", "not-base64!!", nil); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.ListDatabases(ctx, itoa(inst.ID)); errCode(t, err) != "INTERNAL" {
		t.Fatalf("corrupt cipher want INTERNAL, got %v", err)
	}

	// vault 短密钥：Encrypt/Decrypt 走 gcm 构造失败分支。
	bad := &CredentialVault{key: []byte("01234567")}
	if _, err := bad.Encrypt("x"); err == nil {
		t.Fatal("encrypt with short key should fail")
	}
	if _, err := bad.Decrypt("AAAAAAAAAAAAAAAAAAAAAA=="); err == nil {
		t.Fatal("decrypt with short key should fail")
	}
}

// ---- vault 密钥文件失败分支（OpenVault/writeSecretFile）----

func TestVaultOpenFailureBranches(t *testing.T) {
	dir := t.TempDir()
	// 长度错误密钥文件 → 如实报错（不静默重生成——防误覆盖既有凭据）。
	if err := os.WriteFile(filepath.Join(dir, CredKeyFile), make([]byte, 31), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenVault(dir); err == nil {
		t.Fatal("31-byte key file should fail")
	}
	// dataDir 为普通文件（join 后路径非法）→ 读/写失败如实报错。
	notDir := filepath.Join(t.TempDir(), "plainfile")
	if err := os.WriteFile(notDir, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenVault(notDir); err == nil {
		t.Fatal("file-as-datadir should fail")
	}
	// 临时文件创建失败（父目录不存在）。
	if err := writeSecretFile(filepath.Join(t.TempDir(), "missing-dir", "k"), []byte("x")); err == nil {
		t.Fatal("write into missing dir should fail")
	}
}

// ---- SetCredentials / RemoveInstance 校验与 redis 链分支 ----

func TestSetCredentialsValidationBranches(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	inst := e.seedInstance(t, storedb.EngineMySQL)
	rinst := e.seedInstance(t, storedb.EngineRedis)
	actor := testActor()

	if _, err := e.svc.SetCredentials(ctx, actor, CredentialsRequest{InstanceID: 0, Username: "root", Password: "p"}); errCode(t, err) != "DBM_001" {
		t.Fatalf("zero id want DBM_001, got %v", err)
	}
	if _, err := e.svc.SetCredentials(ctx, actor, CredentialsRequest{InstanceID: inst.ID, Username: "", Password: "p"}); errCode(t, err) != "AUTH_012" {
		t.Fatalf("empty username want AUTH_012, got %v", err)
	}
	if _, err := e.svc.SetCredentials(ctx, actor, CredentialsRequest{InstanceID: inst.ID, Username: "root", Password: ""}); errCode(t, err) != "AUTH_012" {
		t.Fatalf("empty password want AUTH_012, got %v", err)
	}
	// I-07 互斥：socket 模式携带 host → AUTH_012。
	socketMode := "socket"
	if _, err := e.svc.SetCredentials(ctx, actor, CredentialsRequest{
		InstanceID: inst.ID, Username: "root", Password: "p",
		ConnMode: &socketMode, Host: strPtrOf("localhost")}); errCode(t, err) != "AUTH_012" {
		t.Fatalf("socket+host want AUTH_012, got %v", err)
	}

	// redis 认证被拒不落库（DBM_005）。
	e.redis.pingErr = dbredis.ErrAuthRejected
	if _, err := e.svc.SetCredentials(ctx, actor, CredentialsRequest{InstanceID: rinst.ID, Username: "default", Password: "bad"}); errCode(t, err) != "DBM_005" {
		t.Fatalf("redis auth want DBM_005, got %v", err)
	}
	e.redis.pingErr = nil
	// redis 成功链：username 固定 default（A-DB6）+ version 实况。
	data, err := e.svc.SetCredentials(ctx, actor, CredentialsRequest{InstanceID: rinst.ID, Username: "whatever", Password: "redis-new"})
	if err != nil || data.Version != "7.2.4" || data.ConnState != "reachable" {
		t.Fatalf("redis credentials: %+v %v", data, err)
	}
	rrow, _ := e.svc.store.Instance().GetByID(ctx, rinst.ID)
	if *rrow.CredUsername != RedisDefaultUsername {
		t.Fatalf("redis username must be forced default: %q", *rrow.CredUsername)
	}

	// 加密失败分支（短密钥 vault）→ 如实报错不落库。
	origVault := e.svc.vault
	e.svc.vault = &CredentialVault{key: []byte("01234567")}
	_, err = e.svc.SetCredentials(ctx, actor, CredentialsRequest{InstanceID: inst.ID, Username: "root", Password: "p"})
	e.svc.vault = origVault
	if err == nil {
		t.Fatal("encrypt failure should fail SetCredentials")
	}

	// RemoveInstance 零值 → DBM_001。
	if _, err := e.svc.RemoveInstance(ctx, actor, InstanceIDRequest{InstanceID: 0}); errCode(t, err) != "DBM_001" {
		t.Fatalf("remove zero id want DBM_001, got %v", err)
	}
}

// ---- Redis 观测/浏览错误分支（redis.go）----

func TestRedisBrowseErrorBranches(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	inst := e.seedInstance(t, storedb.EngineRedis)
	connErr := fmt.Errorf("%w: dial refused", dbredis.ErrConnFailed)

	// KeyTTL：key 空与超长 → AUTH_012。
	if _, err := e.svc.KeyTTL(ctx, itoa(inst.ID), ""); errCode(t, err) != "AUTH_012" {
		t.Fatalf("empty key want AUTH_012, got %v", err)
	}
	if _, err := e.svc.KeyTTL(ctx, itoa(inst.ID), strings.Repeat("k", 513)); errCode(t, err) != "AUTH_012" {
		t.Fatalf("long key want AUTH_012, got %v", err)
	}
	// ListKeys：pattern 超长 → DBM_017；SCAN 失败 → DBM_006。
	if _, err := e.svc.ListKeys(ctx, itoa(inst.ID), "0", strings.Repeat("k", 513)); errCode(t, err) != "DBM_017" {
		t.Fatalf("long pattern want DBM_017, got %v", err)
	}
	e.redis.scanErr = connErr
	if _, err := e.svc.ListKeys(ctx, itoa(inst.ID), "0", "*"); errCode(t, err) != "DBM_006" {
		t.Fatalf("scan err want DBM_006, got %v", err)
	}
	e.redis.scanErr = nil
	// RedisInfo / Slowlog 错误与 limit 校验。
	e.redis.infoErr = connErr
	if _, err := e.svc.RedisInfo(ctx, itoa(inst.ID)); errCode(t, err) != "DBM_006" {
		t.Fatalf("info err want DBM_006, got %v", err)
	}
	e.redis.infoErr = nil
	e.redis.slowErr = connErr
	if _, err := e.svc.RedisSlowlog(ctx, itoa(inst.ID), ""); errCode(t, err) != "DBM_006" {
		t.Fatalf("slowlog err want DBM_006, got %v", err)
	}
	e.redis.slowErr = nil
	if _, err := e.svc.RedisSlowlog(ctx, itoa(inst.ID), "0"); errCode(t, err) != "AUTH_012" {
		t.Fatalf("limit 0 want AUTH_012, got %v", err)
	}
	if _, err := e.svc.RedisSlowlog(ctx, itoa(inst.ID), "abc"); errCode(t, err) != "AUTH_012" {
		t.Fatalf("limit abc want AUTH_012, got %v", err)
	}
}

// ---- 连接目标求值表（identity.go resolve/hostPortLabel）----

func TestConnTargetSpecResolveTable(t *testing.T) {
	socketInst := &storedb.Instance{
		Engine: "mysql", ConnMode: storedb.ConnModeSocket,
		SocketPath:  strPtrOf("/var/lib/mysql/mysql.sock"),
		IdentityKey: BuildIdentityKey("mysql", storedb.ConnModeSocket, "/var/lib/mysql/mysql.sock", "", 0),
	}
	tcpInst := &storedb.Instance{
		Engine: "mysql", ConnMode: storedb.ConnModeTCP,
		Host: strPtrOf("localhost"), Port: intPtrOf(3306),
		IdentityKey: BuildIdentityKey("mysql", storedb.ConnModeTCP, "", "localhost", 3306),
	}

	// 全未提供 → 沿用档案。
	t0, k0, err := (ConnTargetSpec{}).resolve(tcpInst)
	if err != nil || t0.Host != "localhost" || t0.Port != 3306 || k0 != tcpInst.IdentityKey {
		t.Fatalf("empty spec: %+v %s %v", t0, k0, err)
	}
	// 仅 Port：host 沿用档案、端口纠正。
	newPort := 3307
	t1, k1, err := (ConnTargetSpec{Port: &newPort}).resolve(tcpInst)
	if err != nil || t1.Host != "localhost" || t1.Port != 3307 || k1 != "mysql|tcp:localhost:3307" {
		t.Fatalf("port only: %+v %s %v", t1, k1, err)
	}
	// 仅 Host。
	newHost := "db.internal"
	t2, _, err := (ConnTargetSpec{Host: &newHost}).resolve(tcpInst)
	if err != nil || t2.Host != "db.internal" || t2.Port != 3306 {
		t.Fatalf("host only: %+v %v", t2, err)
	}
	// tcp 档案显式纠正为 socket：整体切换目标形态（socketPath 必给）。
	socketMode := storedb.ConnModeSocket
	newPath := "/tmp/alt.sock"
	t3, k3, err := (ConnTargetSpec{ConnMode: &socketMode, SocketPath: &newPath}).resolve(tcpInst)
	if err != nil || t3.ConnMode != storedb.ConnModeSocket || t3.SocketPath != "/tmp/alt.sock" || k3 != "mysql|socket:/tmp/alt.sock" {
		t.Fatalf("tcp→socket: %+v %s %v", t3, k3, err)
	}
	// 仅纠正 ConnMode=socket（socketPath 沿用 socket 档案既有值）。
	t4, k4, err := (ConnTargetSpec{ConnMode: &socketMode}).resolve(socketInst)
	if err != nil || t4.SocketPath != "/var/lib/mysql/mysql.sock" || k4 != socketInst.IdentityKey {
		t.Fatalf("mode only on socket inst: %+v %s %v", t4, k4, err)
	}
	// tcp 模式携带 socketPath → AUTH_012（I-07）。
	badPath := "/tmp/x.sock"
	if _, _, err := (ConnTargetSpec{SocketPath: &badPath}).resolve(tcpInst); errCode(t, err) != "AUTH_012" {
		t.Fatalf("tcp with socketPath want AUTH_012, got %v", err)
	}
	// hostPortLabel 双形态（D-06 socket=local 裁定）。
	if got := hostPortLabel(dbconn.Target{ConnMode: storedb.ConnModeSocket}); got != "local" {
		t.Fatalf("socket label = %q", got)
	}
	if got := hostPortLabel(dbconn.Target{ConnMode: storedb.ConnModeTCP, Host: "localhost", Port: 3306}); got != "localhost-3306" {
		t.Fatalf("tcp label = %q", got)
	}
}

// ---- sortGrantItems 稳定排序（grantmap.go）----

func TestSortGrantItemsStable(t *testing.T) {
	items := []GrantItem{
		{Database: "zzz", Level: LevelReadonly},
		{Database: "appdb", Level: LevelWrite},
		{Database: "mid", Level: LevelAll},
		{Database: "aaa", Level: LevelReadonly},
	}
	sortGrantItems(items)
	want := []string{"aaa", "appdb", "mid", "zzz"}
	for i, w := range want {
		if items[i].Database != w {
			t.Fatalf("sort order: %+v", items)
		}
	}
	// 空切片/单元素不 panic。
	sortGrantItems(nil)
	sortGrantItems([]GrantItem{{Database: "x", Level: LevelAll}})
}
