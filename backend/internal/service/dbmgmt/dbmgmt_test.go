package dbmgmt

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/adapter/dbconn"
	"github.com/dougong/dougong/internal/adapter/dbmysql"
	"github.com/dougong/dougong/internal/adapter/dbredis"
	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	"github.com/dougong/dougong/internal/store"
	storeauth "github.com/dougong/dougong/internal/store/auth"
	storedb "github.com/dougong/dougong/internal/store/dbmgmt"
)

// ---- fake 网关（MySQLGateway/RedisGateway 进程内替身）----

type fakeMySQL struct {
	mu sync.Mutex
	// 记忆实况（库/用户）。
	dbs     []dbmysql.DatabaseInfo
	users   []dbmysql.UserGrants
	charset []string
	// 行为注入。
	pingErr     error
	queryErr    error // 除 Ping 外统一失败（建库/删库等）
	createCalls []string
	dumpBody    string
	dumpErr     error
	importSeen  string
	importErr   error
	alterCalls  int
	lastCreds   dbconn.Creds
	version     string
	vars        map[string]string
	process     []dbmysql.ProcessRow
	// 错误注入（W1 修复轮补充；零值=成功路径，不影响既有用例）。
	statusErr     error
	processErr    error
	listUsersErr  error
	charsetErr    error
	createUserEr  error
	dropUserErr   error
	grantErr      error
	revokeErr     error
	createExecErr error // CreateDatabase 执行体（exists 前置已过后）
	dropExecErr   error // DropDatabase 执行体
	// v2.3 表级治理（记忆实况+行为注入；零值=成功路径）。
	tables       []dbmysql.TableInfo
	structures   map[string][]dbmysql.ColumnInfo // key "db.table"
	rows         map[string][]map[string]any     // key "db.table"
	tableErr     error                           // 表级读统一失败注入
	dropTableErr error
	updateRowErr error
	deleteRowErr error
	affected     int64    // 行写受影响行数（0 → DBM_024）
	updatedRows  []string // 行写定位记录 db.table:pk
	deletedRows  []string
	lastValues   map[string]any
}

func (f *fakeMySQL) Available() error { return nil }

func (f *fakeMySQL) Ping(ctx context.Context, t dbconn.Target, c dbconn.Creds) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastCreds = c
	return f.pingErr
}

func (f *fakeMySQL) ServerVersion(ctx context.Context, t dbconn.Target, c dbconn.Creds) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.version, nil
}

func (f *fakeMySQL) ListDatabases(ctx context.Context, t dbconn.Target, c dbconn.Creds) ([]dbmysql.DatabaseInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.queryErr != nil {
		return nil, f.queryErr
	}
	out := make([]dbmysql.DatabaseInfo, len(f.dbs))
	copy(out, f.dbs)
	return out, nil
}

func (f *fakeMySQL) DatabaseExists(ctx context.Context, t dbconn.Target, c dbconn.Creds, name string) (bool, error) {
	dbs, err := f.ListDatabases(ctx, t, c)
	if err != nil {
		return false, err
	}
	for _, d := range dbs {
		if d.Name == name {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeMySQL) CreateDatabase(ctx context.Context, t dbconn.Target, c dbconn.Creds, name, charset string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createExecErr != nil {
		return f.createExecErr
	}
	if f.queryErr != nil {
		return f.queryErr
	}
	for _, d := range f.dbs {
		if d.Name == name {
			return dbmysql.ErrDatabaseExists
		}
	}
	f.dbs = append(f.dbs, dbmysql.DatabaseInfo{Name: name, Charset: charset})
	f.createCalls = append(f.createCalls, name+":"+charset)
	return nil
}

func (f *fakeMySQL) DropDatabase(ctx context.Context, t dbconn.Target, c dbconn.Creds, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.dropExecErr != nil {
		return f.dropExecErr
	}
	if f.queryErr != nil {
		return f.queryErr
	}
	for i, d := range f.dbs {
		if d.Name == name {
			f.dbs = append(f.dbs[:i], f.dbs[i+1:]...)
			return nil
		}
	}
	return dbmysql.ErrDatabaseNotFound
}

func (f *fakeMySQL) ShowCharsets(ctx context.Context, t dbconn.Target, c dbconn.Creds) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.charsetErr != nil {
		return nil, f.charsetErr
	}
	return f.charset, nil
}

func (f *fakeMySQL) ListUsers(ctx context.Context, t dbconn.Target, c dbconn.Creds) ([]dbmysql.UserGrants, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listUsersErr != nil {
		return nil, f.listUsersErr
	}
	out := make([]dbmysql.UserGrants, len(f.users))
	copy(out, f.users)
	return out, nil
}

func (f *fakeMySQL) ShowGrants(ctx context.Context, t dbconn.Target, c dbconn.Creds, username, host string) ([]dbmysql.GrantLine, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.users {
		if u.Username == username && u.Host == host {
			return u.Grants, nil
		}
	}
	// 契约对齐（ports.go MySQLGateway.ShowGrants 注释）：用户不存在 → ErrUserNotFound
	// 哨兵（服务层译 DBM_009）。
	return nil, dbmysql.ErrUserNotFound
}

func (f *fakeMySQL) CreateUser(ctx context.Context, t dbconn.Target, c dbconn.Creds, username, host, password string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createUserEr != nil {
		return f.createUserEr
	}
	for _, u := range f.users {
		if u.Username == username && u.Host == host {
			return dbmysql.ErrUserExists
		}
	}
	f.users = append(f.users, dbmysql.UserGrants{Username: username, Host: host})
	return nil
}

func (f *fakeMySQL) DropUser(ctx context.Context, t dbconn.Target, c dbconn.Creds, username, host string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.dropUserErr != nil {
		return f.dropUserErr
	}
	for i, u := range f.users {
		if u.Username == username && u.Host == host {
			f.users = append(f.users[:i], f.users[i+1:]...)
			return nil
		}
	}
	return dbmysql.ErrUserNotFound
}

func (f *fakeMySQL) Grant(ctx context.Context, t dbconn.Target, c dbconn.Creds, username, host, db, level string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.grantErr != nil {
		return f.grantErr
	}
	for i, u := range f.users {
		if u.Username == username && u.Host == host {
			// 引擎幂等语义：同库重复 GRANT 合并为一条（SHOW GRANTS 呈现口径）。
			for j, g := range u.Grants {
				if g.DB == db {
					f.users[i].Grants[j] = dbmysql.GrantLine{DB: db, Raw: grantRawOf(level, db, username, host)}
					return nil
				}
			}
			f.users[i].Grants = append(f.users[i].Grants, dbmysql.GrantLine{
				DB: db, Raw: grantRawOf(level, db, username, host)})
			return nil
		}
	}
	return dbmysql.ErrUserNotFound
}

func grantRawOf(level, db, user, host string) string {
	switch level {
	case LevelAll:
		return "GRANT ALL PRIVILEGES ON `" + db + "`.* TO '" + user + "'@'" + host + "'"
	case LevelWrite:
		return "GRANT SELECT, INSERT, UPDATE, DELETE ON `" + db + "`.* TO '" + user + "'@'" + host + "'"
	default:
		return "GRANT SELECT ON `" + db + "`.* TO '" + user + "'@'" + host + "'"
	}
}

func (f *fakeMySQL) Revoke(ctx context.Context, t dbconn.Target, c dbconn.Creds, username, host, db string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.revokeErr != nil {
		return f.revokeErr
	}
	for i, u := range f.users {
		if u.Username != username || u.Host != host {
			continue
		}
		for j, g := range u.Grants {
			if g.DB == db {
				f.users[i].Grants = append(u.Grants[:j], u.Grants[j+1:]...)
				return nil
			}
		}
		return dbmysql.ErrGrantMissing
	}
	return dbmysql.ErrUserNotFound
}

func (f *fakeMySQL) AlterRootPassword(ctx context.Context, t dbconn.Target, c dbconn.Creds, newPassword string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.alterCalls++
	if f.queryErr != nil {
		return f.queryErr
	}
	return nil
}

func (f *fakeMySQL) StatusVariables(ctx context.Context, t dbconn.Target, c dbconn.Creds) (string, map[string]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.statusErr != nil {
		return "", nil, f.statusErr
	}
	return f.version, f.vars, nil
}

func (f *fakeMySQL) ProcessList(ctx context.Context, t dbconn.Target, c dbconn.Creds) ([]dbmysql.ProcessRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.processErr != nil {
		return nil, f.processErr
	}
	return f.process, nil
}

func (f *fakeMySQL) DumpDatabase(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, charset string) (io.ReadCloser, error) {
	f.mu.Lock()
	body, dumpErr := f.dumpBody, f.dumpErr
	f.mu.Unlock()
	if dumpErr != nil {
		return nil, dumpErr
	}
	return io.NopCloser(strings.NewReader(body)), nil
}

func (f *fakeMySQL) ImportDatabase(ctx context.Context, t dbconn.Target, c dbconn.Creds, db string, r io.Reader) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, _ := io.ReadAll(r)
	f.importSeen = string(b)
	return f.importErr
}

// fakeRedis 同构替身。
type fakeRedis struct {
	mu         sync.Mutex
	pingErr    error
	info       dbredis.InfoSections
	slow       []dbredis.SlowlogEntry
	setPassErr error
	oldPass    string
	newPass    string
	scanNext   string
	scanKeys   []dbredis.KeyItem
	ttlMap     map[string]int64
	// 错误注入（W1 修复轮补充；零值=成功路径）。
	infoErr error
	slowErr error
	scanErr error
}

func (f *fakeRedis) Available() error { return nil }

func (f *fakeRedis) Ping(ctx context.Context, t dbconn.Target, c dbconn.Creds) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pingErr
}

func (f *fakeRedis) ServerVersion(ctx context.Context, t dbconn.Target, c dbconn.Creds) (string, error) {
	return "7.2.4", nil
}

func (f *fakeRedis) InfoFields(ctx context.Context, t dbconn.Target, c dbconn.Creds) (dbredis.InfoSections, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.infoErr != nil {
		return nil, f.infoErr
	}
	return f.info, nil
}

func (f *fakeRedis) Slowlog(ctx context.Context, t dbconn.Target, c dbconn.Creds, n int) ([]dbredis.SlowlogEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.slowErr != nil {
		return nil, f.slowErr
	}
	if n < len(f.slow) {
		return f.slow[:n], nil
	}
	return f.slow, nil
}

func (f *fakeRedis) SetRequirepass(ctx context.Context, t dbconn.Target, c dbconn.Creds, currentPassword, newPassword string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.oldPass, f.newPass = currentPassword, newPassword
	return f.setPassErr
}

func (f *fakeRedis) ScanKeys(ctx context.Context, t dbconn.Target, c dbconn.Creds, cursor, pattern string, count int) (string, []dbredis.KeyItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.scanErr != nil {
		return "", nil, f.scanErr
	}
	return f.scanNext, f.scanKeys, nil
}

func (f *fakeRedis) KeyTTL(ctx context.Context, t dbconn.Target, c dbconn.Creds, key string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if v, ok := f.ttlMap[key]; ok {
		if v == -2 {
			return 0, dbredis.ErrKeyNotFound
		}
		return v, nil
	}
	return 0, dbredis.ErrKeyNotFound
}

// ---- 测试基座 ----

type testEnv struct {
	svc    *Service
	mysql  *fakeMySQL
	redis  *fakeRedis
	mongo  *fakeMongo
	vault  *CredentialVault
	auditQ *storeauth.AuditRepo
	db     interface{ Close() error }
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	st := storedb.New(db)
	vault, err := OpenVault(t.TempDir())
	if err != nil {
		t.Fatalf("open vault: %v", err)
	}
	my := &fakeMySQL{version: "8.0.36", charset: []string{"utf8mb4", "latin1"}}
	rd := &fakeRedis{}
	auditSvc := audit.New(st.Audit(), time.Now)
	mg := &fakeMongo{version: "7.0.14"}
	svc := New(AdaptStore(st), my, rd, mg, vault, auditSvc)
	// 跳过首次自动探测（单测直接造档案行）。
	svc.discoverOnce.Do(func() {})
	return &testEnv{svc: svc, mysql: my, redis: rd, mongo: mg, vault: vault, auditQ: st.Audit(), db: db}
}

// seedInstance 直接落一条可达且带凭据的档案行（治理链就绪态）。
func (e *testEnv) seedInstance(t *testing.T, engine string) *storedb.Instance {
	t.Helper()
	cipher, err := e.vault.Encrypt("secret-old")
	if err != nil {
		t.Fatal(err)
	}
	inst := storedb.Instance{
		IdentityKey: BuildIdentityKey(engine, storedb.ConnModeTCP, "", "localhost", 3306),
		Engine:      engine, ConnMode: storedb.ConnModeTCP,
		Host: strPtrOf("localhost"), Port: intPtrOf(3306),
		ConnState: storedb.ConnStateReachable,
	}
	if engine == storedb.EngineRedis {
		inst.IdentityKey = BuildIdentityKey(engine, storedb.ConnModeTCP, "", "localhost", 6379)
		inst.Port = intPtrOf(6379)
	}
	discovered, err := e.svc.store.BeginTx(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := discovered.Instance().UpsertDiscovered(context.Background(), inst, storedb.ConnStateReachable, nil); err != nil {
		t.Fatal(err)
	}
	if err := discovered.Commit(); err != nil {
		t.Fatal(err)
	}
	rows, err := e.svc.store.Instance().GetByIdentity(context.Background(), inst.IdentityKey)
	if err != nil || rows == nil {
		t.Fatalf("seed: %v %v", rows, err)
	}
	tx, err := e.svc.store.BeginTx(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	user := "root"
	if engine == storedb.EngineRedis {
		user = RedisDefaultUsername
	}
	if err := tx.Instance().UpdateCredentials(context.Background(), rows.ID, user, cipher, nil); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	got, _ := e.svc.store.Instance().GetByID(context.Background(), rows.ID)
	return got
}

func strPtrOf(s string) *string { return &s }
func intPtrOf(i int) *int       { return &i }

func errCode(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		return ""
	}
	var ec *errcode.Error
	if errors.As(err, &ec) {
		return ec.Symbol()
	}
	t.Fatalf("non-errcode error: %v", err)
	return ""
}

// ---- 用例 ----

func TestVaultRoundTrip(t *testing.T) {
	dir := t.TempDir()
	v, err := OpenVault(dir)
	if err != nil {
		t.Fatal(err)
	}
	// D-02：密钥文件落数据目录独立文件（不入 SQLite）+ 重开复用同一密钥。
	if _, serr := os.Stat(filepath.Join(dir, CredKeyFile)); serr != nil {
		t.Fatalf("cred key file missing: %v", serr)
	}
	v2, verr := OpenVault(dir)
	if verr != nil {
		t.Fatal(verr)
	}
	c0, _ := v.Encrypt("stable")
	if got, derr := v2.Decrypt(c0); derr != nil || got != "stable" {
		t.Fatalf("reopen vault decrypt: %q %v", got, derr)
	}
	c1, err := v.Encrypt("p@ss'word\\x")
	if err != nil {
		t.Fatal(err)
	}
	c2, _ := v.Encrypt("p@ss'word\\x")
	if c1 == c2 {
		t.Fatal("nonce should differ per encryption")
	}
	for _, c := range []string{c1, c2} {
		got, err := v.Decrypt(c)
		if err != nil || got != "p@ss'word\\x" {
			t.Fatalf("roundtrip: %q %v", got, err)
		}
	}
	if _, err := v.Decrypt("not-base64!!"); err == nil {
		t.Fatal("bad base64 should fail")
	}
	if _, err := v.Decrypt("YWJj"); err == nil { // 3 字节 < nonce
		t.Fatal("short cipher should fail")
	}
}

func TestIdentityKeyAndConnTargetValidation(t *testing.T) {
	if got := BuildIdentityKey("mysql", "socket", "/tmp/mysql.sock", "", 0); got != "mysql|socket:/tmp/mysql.sock" {
		t.Fatalf("socket key = %q", got)
	}
	if got := BuildIdentityKey("redis", "tcp", "", "localhost", 6379); got != "redis|tcp:localhost:6379" {
		t.Fatalf("tcp key = %q", got)
	}
	// I-07：socket 模式带 host 拒绝；tcp 缺 host 拒绝；端口越界拒绝（I-08）。
	if err := validateConnTarget("socket", "/tmp/s.sock", "localhost", 3306); err == nil {
		t.Fatal("socket with host should fail")
	}
	if err := validateConnTarget("tcp", "", "", 0); err == nil {
		t.Fatal("tcp without host should fail")
	}
	if err := validateConnTarget("tcp", "", "localhost", 70000); err == nil {
		t.Fatal("port out of range should fail")
	}
	if err := validateConnTarget("socket", "relative/path", "", 0); err == nil {
		t.Fatal("relative socket path should fail")
	}
}

func TestGrantLevelMapper(t *testing.T) {
	cases := []struct {
		raw, want string
	}{
		{grantRawOf(LevelAll, "d", "u", "h"), LevelAll},
		{grantRawOf(LevelWrite, "d", "u", "h"), LevelWrite},
		{grantRawOf(LevelReadonly, "d", "u", "h"), LevelReadonly},
		{"GRANT SELECT, INSERT ON `d`.* TO 'u'@'h'", ""},     // 子集：未识别
		{"GRANT SELECT, DROP ON `d`.* TO 'u'@'h'", ""},       // 交集外
		{"GRANT ALL PRIVILEGES ON *.* TO 'u'@'h'", LevelAll}, // 全局行本身可映射（呈现侧由 DB!="*" 过滤）
		{"garbage", ""},
	}
	for _, c := range cases {
		if got := GrantLevelMapper(c.raw); got != c.want {
			t.Errorf("GrantLevelMapper(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
}

func TestGoverTargetChain(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	inst := e.seedInstance(t, storedb.EngineMySQL)

	// DBM_001 / DBM_002。
	if _, _, _, err := e.svc.governTarget(ctx, "abc", familyMySQL); errCode(t, err) != "DBM_001" {
		t.Fatalf("want DBM_001, got %v", err)
	}
	if _, _, _, err := e.svc.governTarget(ctx, "999", familyMySQL); errCode(t, err) != "DBM_002" {
		t.Fatalf("want DBM_002, got %v", err)
	}
	// DBM_018：mysql 接口对 redis 档案。
	redisInst := e.seedInstance(t, storedb.EngineRedis)
	if _, _, _, err := e.svc.governTarget(ctx, itoa(redisInst.ID), familyMySQL); errCode(t, err) != "DBM_018" {
		t.Fatalf("want DBM_018, got %v", err)
	}
	// DBM_003：unreachable。
	e.setConnState(t, inst.ID, storedb.ConnStateUnreachable)
	if _, _, _, err := e.svc.governTarget(ctx, itoa(inst.ID), familyMySQL); errCode(t, err) != "DBM_003" {
		t.Fatalf("want DBM_003, got %v", err)
	}
	e.setConnState(t, inst.ID, storedb.ConnStateReachable)
	// DBM_004：未配置凭据（新纳管档案）。
	fresh := e.seedBareInstance(t, storedb.EngineMySQL)
	if _, _, _, err := e.svc.governTarget(ctx, itoa(fresh.ID), familyMySQL); errCode(t, err) != "DBM_004" {
		t.Fatalf("want DBM_004, got %v", err)
	}
	// 全链通过：解密凭据可用。
	_, _, creds, err := e.svc.governTarget(ctx, itoa(inst.ID), familyMySQL)
	if err != nil || creds.Username != "root" || creds.Password != "secret-old" {
		t.Fatalf("govern ok path: %v %+v", err, creds)
	}
}

func (e *testEnv) setConnState(t *testing.T, id int64, state string) {
	t.Helper()
	tx, err := e.svc.store.BeginTx(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Instance().UpdateConnState(context.Background(), id, state, nil); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

// seedBareInstance 落一条无凭据档案（探测即纳管形态）。
func (e *testEnv) seedBareInstance(t *testing.T, engine string) *storedb.Instance {
	t.Helper()
	inst := storedb.Instance{
		IdentityKey: BuildIdentityKey(engine, storedb.ConnModeTCP, "", "localhost", 3307),
		Engine:      engine, ConnMode: storedb.ConnModeTCP,
		Host: strPtrOf("localhost"), Port: intPtrOf(3307),
	}
	tx, err := e.svc.store.BeginTx(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Instance().UpsertDiscovered(context.Background(), inst, storedb.ConnStateReachable, nil); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	got, _ := e.svc.store.Instance().GetByIdentity(context.Background(), inst.IdentityKey)
	return got
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func TestSetCredentialsFlow(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	inst := e.seedInstance(t, storedb.EngineMySQL)
	actor := testActor()

	// DBM_005：认证被拒不落库、不覆盖既有凭据。
	e.mysql.pingErr = dbmysql.ErrAuthRejected
	_, err := e.svc.SetCredentials(ctx, actor, CredentialsRequest{
		InstanceID: inst.ID, Username: "root", Password: "bad"})
	if errCode(t, err) != "DBM_005" {
		t.Fatalf("want DBM_005, got %v", err)
	}
	after, _ := e.svc.store.Instance().GetByID(ctx, inst.ID)
	if !after.CredentialsSet() || *after.CredCipher == "" {
		t.Fatalf("failed validation must not touch credentials (D-03)")
	}

	// 成功：密文覆盖 + version 刷新 + 审计（同事务）。
	e.mysql.pingErr = nil
	e.mysql.version = "8.0.99"
	data, err := e.svc.SetCredentials(ctx, actor, CredentialsRequest{
		InstanceID: inst.ID, Username: "root", Password: "secret-new"})
	if err != nil {
		t.Fatal(err)
	}
	if data.ConnState != "reachable" || data.Version != "8.0.99" {
		t.Fatalf("credentials data: %+v", data)
	}
	after, _ = e.svc.store.Instance().GetByID(ctx, inst.ID)
	plain, _ := e.vault.Decrypt(*after.CredCipher)
	if plain != "secret-new" || after.ConnState != storedb.ConnStateReachable {
		t.Fatalf("credentials persisted wrong: %+v", after)
	}
	if !e.hasAudit(ctx, "instance_credentials_set") {
		t.Fatal("credentials audit missing")
	}

	// DBM_020：连接目标纠正撞既有档案 identity_key（同引擎同目标，I-01）。
	second := e.seedBareInstance(t, storedb.EngineMySQL)
	second.IdentityKey = BuildIdentityKey(storedb.EngineMySQL, storedb.ConnModeTCP, "", "localhost", 3310)
	second.Port = intPtrOf(3310)
	tx10, err := e.svc.store.BeginTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx10.Instance().UpsertDiscovered(ctx, *second, storedb.ConnStateReachable, nil); err != nil {
		t.Fatal(err)
	}
	if err := tx10.Commit(); err != nil {
		t.Fatal(err)
	}
	secondRow, _ := e.svc.store.Instance().GetByIdentity(ctx, second.IdentityKey)
	_, err = e.svc.SetCredentials(ctx, actor, CredentialsRequest{
		InstanceID: secondRow.ID, Username: "root", Password: "p",
		Host: strPtrOf("localhost"), Port: intPtrOf(3306)})
	if errCode(t, err) != "DBM_020" {
		t.Fatalf("want DBM_020, got %v", err)
	}
	// 纠正成功路径：mysql 档案切 socket。
	socket := "/var/lib/mysql/mysql.sock"
	_, err = e.svc.SetCredentials(ctx, actor, CredentialsRequest{
		InstanceID: inst.ID, Username: "root", Password: "secret-new",
		ConnMode: strPtrOf("socket"), SocketPath: &socket})
	if err != nil {
		t.Fatal(err)
	}
	after, _ = e.svc.store.Instance().GetByID(ctx, inst.ID)
	if after.ConnMode != storedb.ConnModeSocket || after.IdentityKey != "mysql|socket:"+socket {
		t.Fatalf("target correction not persisted: %+v", after)
	}
}

func TestRemoveInstanceBusyGuard(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	inst := e.seedInstance(t, storedb.EngineMySQL)
	e.svc.busy.Acquire(inst.ID)
	_, err := e.svc.RemoveInstance(ctx, testActor(), InstanceIDRequest{InstanceID: inst.ID})
	if errCode(t, err) != "DBM_016" {
		t.Fatalf("want DBM_016, got %v", err)
	}
	e.svc.busy.Release(inst.ID)
	data, err := e.svc.RemoveInstance(ctx, testActor(), InstanceIDRequest{InstanceID: inst.ID})
	if err != nil || data.InstanceID != inst.ID {
		t.Fatalf("remove: %v %+v", err, data)
	}
	gone, _ := e.svc.store.Instance().GetByID(ctx, inst.ID)
	if gone != nil {
		t.Fatal("remove left row")
	}
	if !e.hasAudit(ctx, "instance_removed") {
		t.Fatal("remove audit missing")
	}
	// 重复提交如实报 DBM_002。
	_, err = e.svc.RemoveInstance(ctx, testActor(), InstanceIDRequest{InstanceID: inst.ID})
	if errCode(t, err) != "DBM_002" {
		t.Fatalf("repeat remove want DBM_002, got %v", err)
	}
}

func TestDatabaseCRUD(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	inst := e.seedInstance(t, storedb.EngineMySQL)
	actor := testActor()

	// 建库：默认 utf8mb4（D-11）+ 审计。
	data, err := e.svc.CreateDatabase(ctx, actor, DatabaseCreateRequest{
		InstanceID: inst.ID, Name: "appdb"})
	if err != nil || data.Charset != "utf8mb4" {
		t.Fatalf("create default charset: %+v %v", data, err)
	}
	// DBM_008 重复建。
	_, err = e.svc.CreateDatabase(ctx, actor, DatabaseCreateRequest{
		InstanceID: inst.ID, Name: "appdb"})
	if errCode(t, err) != "DBM_008" {
		t.Fatalf("want DBM_008, got %v", err)
	}
	// AUTH_012：charset 非实况候选 / 库名非法。
	_, err = e.svc.CreateDatabase(ctx, actor, DatabaseCreateRequest{
		InstanceID: inst.ID, Name: "db2", Charset: "gbk-big5"})
	if errCode(t, err) != "AUTH_012" {
		t.Fatalf("charset candidate want AUTH_012, got %v", err)
	}
	_, err = e.svc.CreateDatabase(ctx, actor, DatabaseCreateRequest{
		InstanceID: inst.ID, Name: "bad;name"})
	if errCode(t, err) != "AUTH_012" {
		t.Fatalf("bad name want AUTH_012, got %v", err)
	}
	// 删库 DBM_007 / 成功审计。
	_, err = e.svc.DeleteDatabase(ctx, actor, DatabaseDeleteRequest{
		InstanceID: inst.ID, Name: "nodb"})
	if errCode(t, err) != "DBM_007" {
		t.Fatalf("want DBM_007, got %v", err)
	}
	if _, err = e.svc.DeleteDatabase(ctx, actor, DatabaseDeleteRequest{
		InstanceID: inst.ID, Name: "appdb"}); err != nil {
		t.Fatal(err)
	}
	if !e.hasAudit(ctx, "database_created") || !e.hasAudit(ctx, "database_dropped") {
		t.Fatal("database audit missing")
	}
}

func TestDBUserGrantLifecycle(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	inst := e.seedInstance(t, storedb.EngineMySQL)
	actor := testActor()
	e.mysql.dbs = []dbmysql.DatabaseInfo{{Name: "mysql", Charset: "utf8mb4"}}

	if _, err := e.svc.CreateDBUser(ctx, actor, DBUserCreateRequest{
		InstanceID: inst.ID, Username: "app", Host: "192.168.%.%", Password: "pw-123"}); err != nil {
		t.Fatal(err)
	}
	// DBM_010 重复建。
	if _, err := e.svc.CreateDBUser(ctx, actor, DBUserCreateRequest{
		InstanceID: inst.ID, Username: "app", Host: "192.168.%.%", Password: "pw"}); errCode(t, err) != "DBM_010" {
		t.Fatalf("want DBM_010, got %v", err)
	}
	// 授权（幂等：重复授权结果一致）。
	grantReq := DBUserGrantRequest{InstanceID: inst.ID, Username: "app",
		Host: "192.168.%.%", Database: "mysql", Level: LevelWrite}
	if _, err := e.svc.GrantDBUser(ctx, actor, grantReq); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.GrantDBUser(ctx, actor, grantReq); err != nil {
		t.Fatalf("grant idempotency: %v", err)
	}
	// 清单映射三档呈现（D-10）。
	list, err := e.svc.ListDBUsers(ctx, itoa(inst.ID))
	if err != nil || len(list.Items) != 1 {
		t.Fatalf("list users: %v %+v", err, list)
	}
	if len(list.Items[0].Grants) != 1 || list.Items[0].Grants[0].Level != LevelWrite {
		t.Fatalf("grants mapping: %+v", list.Items[0].Grants)
	}
	// DBM_011：revoke 不存在的库授权。
	_, err = e.svc.RevokeDBUser(ctx, actor, DBUserRevokeRequest{
		InstanceID: inst.ID, Username: "app", Host: "192.168.%.%", Database: "nogrant"})
	if errCode(t, err) != "DBM_011" {
		t.Fatalf("want DBM_011, got %v", err)
	}
	// revoke 成功后清单不再呈现。
	if _, err = e.svc.RevokeDBUser(ctx, actor, DBUserRevokeRequest{
		InstanceID: inst.ID, Username: "app", Host: "192.168.%.%", Database: "mysql"}); err != nil {
		t.Fatal(err)
	}
	list, _ = e.svc.ListDBUsers(ctx, itoa(inst.ID))
	if len(list.Items[0].Grants) != 0 {
		t.Fatalf("revoke not reflected: %+v", list.Items[0].Grants)
	}
	// 删用户 DBM_009 / 成功。
	if _, err := e.svc.DeleteDBUser(ctx, actor, DBUserRevokeRequest{
		InstanceID: inst.ID, Username: "ghost", Host: "%"}); errCode(t, err) != "DBM_009" {
		t.Fatalf("want DBM_009, got %v", err)
	}
	if _, err := e.svc.DeleteDBUser(ctx, actor, DBUserRevokeRequest{
		InstanceID: inst.ID, Username: "app", Host: "192.168.%.%"}); err != nil {
		t.Fatal(err)
	}
}

func TestPasswordSyncTwoStep(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	inst := e.seedInstance(t, storedb.EngineMySQL)
	actor := testActor()

	// 成功路径：引擎改密 + 密文 UPSERT 同请求。
	data, err := e.svc.ChangeRootPassword(ctx, actor, PasswordRequest{
		InstanceID: inst.ID, NewPassword: "new-pw-1"})
	if err != nil || !data.Synced {
		t.Fatalf("change password: %+v %v", data, err)
	}
	after, _ := e.svc.store.Instance().GetByID(ctx, inst.ID)
	plain, _ := e.vault.Decrypt(*after.CredCipher)
	if plain != "new-pw-1" {
		t.Fatalf("cipher not synced: %q", plain)
	}
	if e.mysql.alterCalls != 1 {
		t.Fatalf("engine alter calls = %d", e.mysql.alterCalls)
	}

	// DBM_019：引擎成功但落库失败（BeginTx 注入失败）——不回滚引擎侧。
	failing := &failingTxStore{inner: e.svc.store}
	e.svc.store = failing
	_, err = e.svc.ChangeRootPassword(ctx, actor, PasswordRequest{
		InstanceID: inst.ID, NewPassword: "new-pw-2"})
	if errCode(t, err) != "DBM_019" {
		t.Fatalf("want DBM_019, got %v", err)
	}
	if e.mysql.alterCalls != 2 {
		t.Fatalf("engine side must have run twice (no rollback), got %d", e.mysql.alterCalls)
	}
	e.svc.store = failing.inner // 还原
}

// failingTxStore 仅 BeginTx 失败（DBM_019 注入）。
type failingTxStore struct{ inner StoreAPI }

func (f *failingTxStore) Instance() InstanceRepoAPI { return f.inner.Instance() }
func (f *failingTxStore) BeginTx(ctx context.Context) (TxAPI, error) {
	return nil, errors.New("injected begin failure")
}

func TestRedisPasswordSync(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	inst := e.seedInstance(t, storedb.EngineRedis)

	data, err := e.svc.SetRedisPassword(ctx, testActor(), PasswordRequest{
		InstanceID: inst.ID, NewPassword: "redis-pw"})
	if err != nil || !data.Synced {
		t.Fatalf("redis password: %+v %v", data, err)
	}
	if e.redis.oldPass != "secret-old" || e.redis.newPass != "redis-pw" {
		t.Fatalf("requirepass args wrong: %+v", e.redis)
	}
	if !e.hasAudit(ctx, "redis_password_set") {
		t.Fatal("redis password audit missing")
	}
}

func TestKeyBrowseAndPatternGuard(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	inst := e.seedInstance(t, storedb.EngineRedis)

	e.redis.scanNext = "42"
	e.redis.scanKeys = []dbredis.KeyItem{{Key: "session:1", Type: "string"}}
	data, err := e.svc.ListKeys(ctx, itoa(inst.ID), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if data.NextCursor != "42" || data.Complete || len(data.Items) != 1 || data.Items[0].Type != "string" {
		t.Fatalf("key list: %+v", data)
	}
	// 默认 pattern=*，显式空 pattern → DBM_017（禁 KEYS 全量语义）。
	if err := validateKeyPattern(""); errCode(t, err) != "DBM_017" {
		t.Fatalf("empty pattern want DBM_017, got %v", err)
	}
	if err := validateKeyPattern("has space*"); errCode(t, err) != "DBM_017" {
		t.Fatalf("space pattern want DBM_017, got %v", err)
	}
	if _, err := e.svc.ListKeys(ctx, itoa(inst.ID), "bad-cursor", "*"); errCode(t, err) != "AUTH_012" {
		t.Fatalf("bad cursor want AUTH_012, got %v", err)
	}
	// TTL：-1 透传 / DBM_015。
	e.redis.ttlMap = map[string]int64{"k1": -1, "k2": -2}
	ttl, err := e.svc.KeyTTL(ctx, itoa(inst.ID), "k1")
	if err != nil || ttl.TTLSec != -1 {
		t.Fatalf("ttl -1: %+v %v", ttl, err)
	}
	if _, err := e.svc.KeyTTL(ctx, itoa(inst.ID), "k2"); errCode(t, err) != "DBM_015" {
		t.Fatalf("want DBM_015, got %v", err)
	}
}

func TestBackupExportFlow(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	inst := e.seedInstance(t, storedb.EngineMySQL)
	e.mysql.dbs = []dbmysql.DatabaseInfo{{Name: "appdb", Charset: "utf8mb4"}}

	// DBM_007：库不存在。
	_, err := e.svc.ExportDatabase(ctx, testActor(), inst.ID, "nodb")
	if errCode(t, err) != "DBM_007" {
		t.Fatalf("want DBM_007, got %v", err)
	}
	// 成功：首字节探测 + 流读取 + Finish 审计 sizeBytes。
	e.mysql.dumpBody = "-- fake dump\nINSERT 1;\n"
	stream, err := e.svc.ExportDatabase(ctx, testActor(), inst.ID, "appdb")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stream.Filename, "appdb_") || !strings.HasSuffix(stream.Filename, ".sql") {
		t.Fatalf("filename = %q", stream.Filename)
	}
	br, err := PeekExport(stream)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	n, err := io.Copy(&buf, br)
	if err != nil || !strings.HasPrefix(buf.String(), "-- fake dump") {
		t.Fatalf("stream copy: %v %q", err, buf.String())
	}
	stream.Finish(n)
	if !e.hasAudit(ctx, "database_exported") {
		t.Fatal("export audit missing")
	}
	// busy 已解除：可再次导出（随即收尾释放，供后续断言复用）。
	rel, err := e.svc.ExportDatabase(ctx, testActor(), inst.ID, "appdb")
	if err != nil {
		t.Fatalf("busy not released: %v", err)
	}
	_ = rel.Abort(errors.New("test done"))
	// 启动即失败：Abort → DBM_012 JSON 语义。
	e.mysql.dumpBody = ""
	e.mysql.dumpErr = nil
	stream2, _ := e.svc.ExportDatabase(ctx, testActor(), inst.ID, "appdb")
	_, err = PeekExport(stream2) // 空输出 → EOF → Abort
	if errCode(t, err) != "DBM_012" {
		t.Fatalf("want DBM_012, got %v", err)
	}
}

func TestBackupImportFlow(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	inst := e.seedInstance(t, storedb.EngineMySQL)
	e.mysql.dbs = []dbmysql.DatabaseInfo{{Name: "appdb", Charset: "utf8mb4"}}

	// DBM_014：超限。
	_, err := e.svc.ImportDatabase(ctx, testActor(), inst.ID, "appdb",
		DumpSizeLimit+1, strings.NewReader("x"))
	if errCode(t, err) != "DBM_014" {
		t.Fatalf("want DBM_014, got %v", err)
	}
	// DBM_007：目标库不存在。
	_, err = e.svc.ImportDatabase(ctx, testActor(), inst.ID, "nodb",
		10, strings.NewReader("INSERT 1;"))
	if errCode(t, err) != "DBM_007" {
		t.Fatalf("want DBM_007, got %v", err)
	}
	// 成功：内容透传 mysql 客户端 + 审计 sizeBytes + busy 解除。
	data, err := e.svc.ImportDatabase(ctx, testActor(), inst.ID, "appdb",
		10, strings.NewReader("INSERT 1;"))
	if err != nil || data.SizeBytes != 10 {
		t.Fatalf("import: %+v %v", data, err)
	}
	if e.mysql.importSeen != "INSERT 1;" {
		t.Fatalf("import payload = %q", e.mysql.importSeen)
	}
	if !e.hasAudit(ctx, "database_imported") {
		t.Fatal("import audit missing")
	}
	if e.svc.busy.Busy(inst.ID) {
		t.Fatal("busy not released after import")
	}
	// DBM_013：执行失败。
	e.mysql.importErr = dbmysql.ErrImportFailed
	_, err = e.svc.ImportDatabase(ctx, testActor(), inst.ID, "appdb",
		10, strings.NewReader("bad"))
	if errCode(t, err) != "DBM_013" {
		t.Fatalf("want DBM_013, got %v", err)
	}
}

func TestProberDiscovery(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	// reachable 注入：仅 localhost:3306 可达。
	reach := func(t dbconn.Target) bool {
		return t.ConnMode == "tcp" && t.Port == 3306
	}
	prober := NewProber(e.svc.store, e.mysql, e.redis, e.mongo, e.vault, reach)
	res, err := prober.ProbeAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Discovered != 1 {
		t.Fatalf("discovered = %d, want 1", res.Discovered)
	}
	if !res.TCPReachable["mysql"] || res.TCPReachable["redis"] || res.TCPReachable["mongodb"] {
		t.Fatalf("tcp reachability map wrong: %+v", res.TCPReachable)
	}
	got, _ := e.svc.store.Instance().GetByIdentity(ctx, "mysql|tcp:localhost:3306")
	if got == nil || got.ConnState != storedb.ConnStateReachable || got.CredentialsSet() {
		t.Fatalf("probe row wrong: %+v", got)
	}
	// 再跑一轮：既有档案 → updated=1。
	res, _ = prober.ProbeAll(ctx)
	if res.Discovered != 0 || res.Updated != 1 {
		t.Fatalf("second pass: discovered=%d updated=%d", res.Discovered, res.Updated)
	}
	// 不可达轮：既有档案整值覆盖 unreachable（I-03）。
	none := func(dbconn.Target) bool { return false }
	p2 := NewProber(e.svc.store, e.mysql, e.redis, e.mongo, e.vault, none)
	_, _ = p2.ProbeAll(ctx)
	got, _ = e.svc.store.Instance().GetByIdentity(ctx, "mysql|tcp:localhost:3306")
	if got == nil || got.ConnState != storedb.ConnStateUnreachable {
		t.Fatalf("unreachable overwrite missing: %+v", got)
	}
}

func TestMySQLStatusWhitelist(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	inst := e.seedInstance(t, storedb.EngineMySQL)
	e.mysql.vars = map[string]string{
		"version_comment":         "MySQL Community Server",
		"max_connections":         "151",
		"innodb_buffer_pool_size": "134217728",
		"secret_internal":         "must-not-leak", // 白名单外：不呈现（D-08）
	}
	data, err := e.svc.MySQLStatus(ctx, itoa(inst.ID))
	if err != nil {
		t.Fatal(err)
	}
	if data.Version != "8.0.36" || data.Variables["version_comment"] != "MySQL Community Server" {
		t.Fatalf("status: %+v", data)
	}
	if len(data.Variables) != 3 {
		t.Fatalf("whitelist leak: %+v", data.Variables)
	}
	pl, err := e.svc.MySQLProcessList(ctx, itoa(inst.ID))
	if err != nil || len(pl.Items) != 0 {
		t.Fatalf("processlist: %+v %v", pl, err)
	}
}

func TestRedisInfoWhitelist(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	inst := e.seedInstance(t, storedb.EngineRedis)
	e.redis.info = dbredis.InfoSections{
		"server":  {"redis_version": "7.2.4", "os": "Linux"},
		"clients": {"connected_clients": "3"},
		"memory":  {"used_memory_human": "2M", "maxmemory_human": "0B"},
		"stats":   {"total_connections_received": "10", "keyspace_hits": "5", "keyspace_misses": "2"},
	}
	data, err := e.svc.RedisInfo(ctx, itoa(inst.ID))
	if err != nil {
		t.Fatal(err)
	}
	if data.Sections["server"]["version"] != "7.2.4" ||
		data.Sections["memory"]["usedMemoryHuman"] != "2M" ||
		data.Sections["stats"]["keyspaceMisses"] != "2" {
		t.Fatalf("redis info mapping: %+v", data.Sections)
	}
	if _, leak := data.Sections["server"]["os"]; leak {
		t.Fatal("non-whitelist field leaked")
	}
	// 慢日志 limit 钳制。
	e.redis.slow = []dbredis.SlowlogEntry{{ID: 1, Timestamp: 1700000000, DurationUs: 5000, Command: "get k"}}
	sl, err := e.svc.RedisSlowlog(ctx, itoa(inst.ID), "")
	if err != nil || len(sl.Items) != 1 || sl.Items[0].Command != "get k" {
		t.Fatalf("slowlog: %+v %v", sl, err)
	}
	if _, err := e.svc.RedisSlowlog(ctx, itoa(inst.ID), "999"); errCode(t, err) != "AUTH_012" {
		t.Fatalf("slowlog limit want AUTH_012, got %v", err)
	}
}

// ---- 辅助 ----

func testActor() *authsvc.Identity {
	return &authsvc.Identity{UserID: 7, Username: "admin", Role: "admin"}
}

// hasAudit 检查动作码是否已写审计（成功或失败）。
func (e *testEnv) hasAudit(ctx context.Context, action string) bool {
	rows, total, err := e.auditQ.Search(ctx, storeauth.AuditCondition{Action: action}, 0, 50)
	if err != nil {
		return false
	}
	_ = rows
	return total > 0
}

func TestListInstancesFilterAndFirstProbe(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	e.seedInstance(t, storedb.EngineMySQL)

	// engine 非法 → AUTH_012。
	if _, err := e.svc.ListInstances(ctx, "postgres"); errCode(t, err) != "AUTH_012" {
		t.Fatalf("engine filter want AUTH_012, got %v", err)
	}
	// 合法筛选 + 投影字段（credentialsSet/credUsername 可见、密文零出口）。
	data, err := e.svc.ListInstances(ctx, "mysql")
	if err != nil || len(data.Items) != 1 {
		t.Fatalf("list: %+v %v", data, err)
	}
	item := data.Items[0]
	if !item.CredentialsSet || *item.CredUsername != "root" || item.ConnState != "reachable" {
		t.Fatalf("item projection: %+v", item)
	}
	// 首次 list 触发探测（discoverOnce 已在基座耗掉——此处验证再次调用零副作用）。
	if _, err := e.svc.ListInstances(ctx, ""); err != nil {
		t.Fatal(err)
	}
}

func TestRescanRecordsAudit(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	data, err := e.svc.Rescan(ctx, testActor())
	if err != nil {
		t.Fatal(err)
	}
	if data.Discovered != 0 { // Windows 测试机探测集全部不可达：0/0 如实
		t.Fatalf("discovered = %d, want 0", data.Discovered)
	}
	if !e.hasAudit(ctx, "instance_rescanned") {
		t.Fatal("rescan audit missing（admin 显式动作写审计，D-07）")
	}
}

func TestIdentifierWhitelists(t *testing.T) {
	// R-14/A-DB3 白名单表驱动。
	bad := []struct {
		fn   func(string) error
		val  string
		name string
	}{
		{validateDatabaseName, "has-dash", "database dash"},
		{validateDatabaseName, "a`b", "database backtick"},
		{validateDatabaseName, "", "database empty"},
		{validateUsername, "u ser", "username space"},
		{validateUserHost, "bad host", "host space"},
		{validateRedisKey, strings.Repeat("k", 513), "key too long"},
	}
	for _, c := range bad {
		if errCode(t, c.fn(c.val)) != "AUTH_012" {
			t.Errorf("%s should fail AUTH_012", c.name)
		}
	}
	// 密码长度上限（双参签名单独验证）。
	if errCode(t, validatePassword("newPassword", strings.Repeat("x", 129))) != "AUTH_012" {
		t.Error("password too long should fail AUTH_012")
	}
	// 合法形态。
	if err := validateUserHost("192.168.%.%"); err != nil {
		t.Fatalf("host wildcard should pass: %v", err)
	}
	if err := validateDatabaseName("db_01"); err != nil {
		t.Fatalf("valid db name: %v", err)
	}
}
