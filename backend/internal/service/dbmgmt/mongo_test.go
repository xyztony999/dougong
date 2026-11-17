package dbmgmt

// MongoDB 治理链测试（v0.1.4 反馈第 3 项增量）：fakeMongo 替身承载全部链路
//（CI 环境耦合预防第 6 起：mongo-driver 连接类测试不依赖 runner 实况——
// 适配器单测仅测纯函数分类/URI 组装，service 层全走替身，探测经注入）。

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/dougong/dougong/internal/adapter/dbconn"
	"github.com/dougong/dougong/internal/adapter/dbmongo"
	"github.com/dougong/dougong/internal/errcode"
	storedb "github.com/dougong/dougong/internal/store/dbmgmt"
)

// fakeMongo 是 MongoGateway 同构替身（记忆实况+行为注入；零值=成功路径）。
type fakeMongo struct {
	pingErr error
	version string

	dbs         []dbmongo.DatabaseInfo
	listDBErr   error
	status      *dbmongo.StatusSummary
	statusErr   error
	users       []dbmongo.UserItem
	listUserErr error
	createErr   error
	created     []string // username:db:role
	dropErr     error
	dropped     []string
	grantErr    error
	granted     []string
	revokeErr   error
	revoked     []string
	lastCreds   dbconn.Creds
}

func (f *fakeMongo) Available() error { return nil }

func (f *fakeMongo) Ping(ctx context.Context, t dbconn.Target, c dbconn.Creds) error {
	f.lastCreds = c
	return f.pingErr
}

func (f *fakeMongo) ServerVersion(ctx context.Context, t dbconn.Target, c dbconn.Creds) (string, error) {
	return f.version, nil
}

func (f *fakeMongo) ListDatabases(ctx context.Context, t dbconn.Target, c dbconn.Creds) ([]dbmongo.DatabaseInfo, error) {
	if f.listDBErr != nil {
		return nil, f.listDBErr
	}
	return f.dbs, nil
}

func (f *fakeMongo) StatusSummary(ctx context.Context, t dbconn.Target, c dbconn.Creds) (*dbmongo.StatusSummary, error) {
	if f.statusErr != nil {
		return nil, f.statusErr
	}
	return f.status, nil
}

func (f *fakeMongo) ListUsers(ctx context.Context, t dbconn.Target, c dbconn.Creds) ([]dbmongo.UserItem, error) {
	if f.listUserErr != nil {
		return nil, f.listUserErr
	}
	return f.users, nil
}

func (f *fakeMongo) CreateUser(ctx context.Context, t dbconn.Target, c dbconn.Creds, username, password, db, role string) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = append(f.created, username+":"+db+":"+role)
	return nil
}

func (f *fakeMongo) DropUser(ctx context.Context, t dbconn.Target, c dbconn.Creds, username string) error {
	if f.dropErr != nil {
		return f.dropErr
	}
	f.dropped = append(f.dropped, username)
	return nil
}

func (f *fakeMongo) GrantRole(ctx context.Context, t dbconn.Target, c dbconn.Creds, username, db, role string) error {
	if f.grantErr != nil {
		return f.grantErr
	}
	f.granted = append(f.granted, username+":"+db+":"+role)
	return nil
}

func (f *fakeMongo) RevokeRole(ctx context.Context, t dbconn.Target, c dbconn.Creds, username, db, role string) error {
	if f.revokeErr != nil {
		return f.revokeErr
	}
	f.revoked = append(f.revoked, username+":"+db+":"+role)
	return nil
}

// ---- 观测读路径 ----

func TestMongoObservationPaths(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	inst := e.seedInstance(t, storedb.EngineMongoDB)

	// 引擎不匹配：mysql 族接口对 mongodb 实例 → DBM_018。
	if _, err := e.svc.ListDatabases(ctx, itoa(inst.ID)); errCode(t, err) != "DBM_018" {
		t.Fatalf("mysql iface on mongo want DBM_018, got %v", err)
	}

	e.mongo.status = &dbmongo.StatusSummary{Version: "7.0.14", UptimeSec: 100, ConnectionsCurrent: 2,
		ConnectionsAvailable: 838858, MemResidentMb: 64, OpInsert: 3, OpQuery: 5, OpUpdate: 1, OpDelete: 0}
	st, err := e.svc.MongoStatus(ctx, itoa(inst.ID))
	if err != nil || st.Version != "7.0.14" || st.UptimeSec != 100 || st.OpQuery != 5 {
		t.Fatalf("mongo status: %+v %v", st, err)
	}
	// 权限不足（Unauthorized code 13）→ DBM_022（非裸 INTERNAL）。
	e.mongo.statusErr = fmt.Errorf("%w: not authorized on admin to execute command", dbmongo.ErrUnauthorized)
	if _, err := e.svc.MongoStatus(ctx, itoa(inst.ID)); errCode(t, err) != "DBM_022" {
		t.Fatalf("unauthorized want DBM_022, got %v", err)
	}
	e.mongo.statusErr = nil

	e.mongo.dbs = []dbmongo.DatabaseInfo{{Name: "admin", SizeBytes: 1, Collections: 2}, {Name: "appdb", SizeBytes: 4096, Collections: 5}}
	dbs, err := e.svc.MongoListDatabases(ctx, itoa(inst.ID))
	if err != nil || len(dbs.Items) != 2 || dbs.Items[1].Collections != 5 {
		t.Fatalf("mongo dbs: %+v %v", dbs, err)
	}

	e.mongo.users = []dbmongo.UserItem{{
		Username: "app", Mechanisms: []string{"SCRAM-SHA-256"},
		Roles: []dbmongo.UserRole{{DB: "appdb", Role: "readWrite"}},
	}}
	users, err := e.svc.MongoListUsers(ctx, itoa(inst.ID))
	if err != nil || len(users.Items) != 1 || users.Items[0].Roles[0].Role != "readWrite" {
		t.Fatalf("mongo users: %+v %v", users, err)
	}

	// redis 实例对 mongo 族接口 → DBM_018（反向）。
	rinst := e.seedInstance(t, storedb.EngineRedis)
	if _, err := e.svc.MongoStatus(ctx, itoa(rinst.ID)); errCode(t, err) != "DBM_018" {
		t.Fatalf("mongo iface on redis want DBM_018, got %v", err)
	}
}

// ---- 用户治理写路径 ----

func TestMongoUserGovernance(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	inst := e.seedInstance(t, storedb.EngineMongoDB)
	actor := testActor()

	// 创建：白名单/角色域校验。
	if _, err := e.svc.MongoCreateUser(ctx, actor, MongoUserCreateRequest{
		InstanceID: inst.ID, Username: "bad name", Password: "p", DB: "d", Role: "read"}); errCode(t, err) != "AUTH_012" {
		t.Fatalf("bad username want AUTH_012, got %v", err)
	}
	if _, err := e.svc.MongoCreateUser(ctx, actor, MongoUserCreateRequest{
		InstanceID: inst.ID, Username: "app", Password: "p", DB: "d", Role: "root"}); errCode(t, err) != "AUTH_012" {
		t.Fatalf("bad role want AUTH_012, got %v", err)
	}
	// 重名（引擎 51003）→ DBM_010。
	e.mongo.createErr = fmt.Errorf("%w: User app@admin already exists", dbmongo.ErrUserExists)
	if _, err := e.svc.MongoCreateUser(ctx, actor, MongoUserCreateRequest{
		InstanceID: inst.ID, Username: "app", Password: "secret", DB: "appdb", Role: "readWrite"}); errCode(t, err) != "DBM_010" {
		t.Fatalf("dup user want DBM_010, got %v", err)
	}
	e.mongo.createErr = nil
	if _, err := e.svc.MongoCreateUser(ctx, actor, MongoUserCreateRequest{
		InstanceID: inst.ID, Username: "app", Password: "secret", DB: "appdb", Role: "readWrite"}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if len(e.mongo.created) != 1 || e.mongo.created[0] != "app:appdb:readWrite" {
		t.Fatalf("created = %v", e.mongo.created)
	}

	// 授权：用户不存在 → DBM_009（service 前置实况校验）。
	if _, err := e.svc.MongoGrantRole(ctx, actor, MongoUserRoleRequest{
		InstanceID: inst.ID, Username: "ghost", DB: "appdb", Role: "read"}); errCode(t, err) != "DBM_009" {
		t.Fatalf("grant missing user want DBM_009, got %v", err)
	}
	e.mongo.users = []dbmongo.UserItem{{Username: "app", Roles: []dbmongo.UserRole{{DB: "appdb", Role: "readWrite"}}}}
	if _, err := e.svc.MongoGrantRole(ctx, actor, MongoUserRoleRequest{
		InstanceID: inst.ID, Username: "app", DB: "appdb", Role: "dbAdmin"}); err != nil {
		t.Fatalf("grant: %v", err)
	}

	// 回收：授权关系不存在（引擎不必然报错——service 前置判据）→ DBM_011。
	e.mongo.users = []dbmongo.UserItem{{Username: "app", Roles: []dbmongo.UserRole{{DB: "appdb", Role: "readWrite"}}}}
	if _, err := e.svc.MongoRevokeRole(ctx, actor, MongoUserRoleRequest{
		InstanceID: inst.ID, Username: "app", DB: "appdb", Role: "read"}); errCode(t, err) != "DBM_011" {
		t.Fatalf("revoke unheld role want DBM_011, got %v", err)
	}
	if _, err := e.svc.MongoRevokeRole(ctx, actor, MongoUserRoleRequest{
		InstanceID: inst.ID, Username: "app", DB: "appdb", Role: "readWrite"}); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if len(e.mongo.revoked) != 1 {
		t.Fatalf("revoked = %v", e.mongo.revoked)
	}

	// 删除：不存在（引擎 code 11）→ DBM_009。
	e.mongo.dropErr = fmt.Errorf("%w: User 'ghost@admin' not found", dbmongo.ErrUserNotFound)
	if _, err := e.svc.MongoDeleteUser(ctx, actor, MongoUserRequest{
		InstanceID: inst.ID, Username: "ghost"}); errCode(t, err) != "DBM_009" {
		t.Fatalf("drop missing want DBM_009, got %v", err)
	}
	e.mongo.dropErr = nil
	if _, err := e.svc.MongoDeleteUser(ctx, actor, MongoUserRequest{
		InstanceID: inst.ID, Username: "app"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

// ---- SetCredentials mongodb 分支与探测分支 ----

func TestMongoCredentialsAndProbe(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	inst := e.seedInstance(t, storedb.EngineMongoDB)
	actor := testActor()

	// 凭据验证：认证被拒 DBM_005；连接失败 DBM_006；未分类 DBM_006（translatePingErr）；成功实名落库。
	e.mongo.pingErr = fmt.Errorf("%w: (AuthenticationFailed) Authentication failed", dbmongo.ErrAuthRejected)
	if _, err := e.svc.SetCredentials(ctx, actor, CredentialsRequest{
		InstanceID: inst.ID, Username: "paneladmin", Password: "P@ss w0rd!"}); errCode(t, err) != "DBM_005" {
		t.Fatalf("mongo auth want DBM_005, got %v", err)
	}
	e.mongo.pingErr = fmt.Errorf("%w: server selection error: dial tcp 127.0.0.1:27017", dbmongo.ErrConnFailed)
	if _, err := e.svc.SetCredentials(ctx, actor, CredentialsRequest{
		InstanceID: inst.ID, Username: "paneladmin", Password: "x"}); errCode(t, err) != "DBM_006" {
		t.Fatalf("mongo conn want DBM_006, got %v", err)
	}
	e.mongo.pingErr = fmt.Errorf("%w: weird failure", dbmongo.ErrQueryFailed)
	if _, err := e.svc.SetCredentials(ctx, actor, CredentialsRequest{
		InstanceID: inst.ID, Username: "paneladmin", Password: "x"}); errCode(t, err) != "DBM_006" {
		t.Fatalf("mongo unclassified want DBM_006 (translatePingErr), got %v", err)
	}
	e.mongo.pingErr = nil
	e.mongo.version = "7.0.14"
	data, err := e.svc.SetCredentials(ctx, actor, CredentialsRequest{
		InstanceID: inst.ID, Username: "paneladmin", Password: "P@ss w0rd!"})
	if err != nil || data.Version != "7.0.14" || data.ConnState != "reachable" {
		t.Fatalf("mongo credentials: %+v %v", data, err)
	}
	if e.mongo.lastCreds.Password != "P@ss w0rd!" {
		t.Fatalf("password must reach gateway verbatim (structured credential, no URI escaping): %+v", e.mongo.lastCreds)
	}
	// mongodb 凭据用户名按请求实名（非 redis default 强改）。
	row, _ := e.svc.store.Instance().GetByID(ctx, inst.ID)
	if *row.CredUsername != "paneladmin" {
		t.Fatalf("mongo cred username = %q", *row.CredUsername)
	}

	// 探测：mongo 分支（注入 reachable；Ping 失败=连接层 → unreachable；成功 → reachable+version）。
	reach := func(dbconn.Target) bool { return true }
	prober := NewProber(e.svc.store, e.mysql, e.redis, e.mongo, e.vault, reach)
	if _, _, err := prober.ProbeAll(ctx); err != nil {
		t.Fatal(err)
	}
	mongoRows := []storedb.Instance{}
	rows, _ := e.svc.store.Instance().List(ctx, storedb.EngineMongoDB)
	mongoRows = append(mongoRows, rows...)
	if len(mongoRows) == 0 {
		t.Fatal("probe set must discover mongodb targets (tcp+socket)")
	}
	for _, r := range mongoRows {
		if r.ConnState != "reachable" {
			t.Fatalf("mongo probe state = %s", r.ConnState)
		}
	}
	// 连接层失败 → unreachable 整值更新。
	e.mongo.pingErr = fmt.Errorf("%w: connection refused", dbmongo.ErrConnFailed)
	if _, _, err := prober.ProbeAll(ctx); err != nil {
		t.Fatal(err)
	}
}

// ---- 翻译表 mongo 哨兵 ----

func TestTranslateMongoSentinels(t *testing.T) {
	cases := []struct {
		in   error
		want string
	}{
		{dbmongo.ErrAuthRejected, "DBM_005"},
		{dbmongo.ErrConnFailed, "DBM_006"},
		{dbmongo.ErrUnauthorized, "DBM_022"},
		{dbmongo.ErrUserExists, "DBM_010"},
		{dbmongo.ErrUserNotFound, "DBM_009"},
		{errors.New("boom"), "@"},
	}
	for _, c := range cases {
		got := translateGatewayErr(c.in)
		if c.want == "@" {
			if !errors.Is(got, c.in) {
				t.Errorf("%v: unknown must pass through, got %v", c.in, got)
			}
			continue
		}
		var ec *errcode.Error
		if !errors.As(got, &ec) || ec.Symbol() != c.want {
			t.Errorf("%v: want %s, got %v", c.in, c.want, got)
		}
	}
	if got := translatePingErr(fmt.Errorf("%w: x", dbmongo.ErrQueryFailed)); errCode(t, got) != "DBM_006" {
		t.Fatalf("ping unclassified want DBM_006, got %v", got)
	}
}
