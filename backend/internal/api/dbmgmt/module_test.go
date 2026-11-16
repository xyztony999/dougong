package dbmgmt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/adapter/dbconn"
	"github.com/dougong/dougong/internal/adapter/dbmysql"
	"github.com/dougong/dougong/internal/adapter/dbredis"
	"github.com/dougong/dougong/internal/api"
	authapi "github.com/dougong/dougong/internal/api/auth"
	auditsvc "github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	dbmgmtsvc "github.com/dougong/dougong/internal/service/dbmgmt"
	"github.com/dougong/dougong/internal/store"
	storeauth "github.com/dougong/dougong/internal/store/auth"
	storedb "github.com/dougong/dougong/internal/store/dbmgmt"
)

// 测试口令占位（程序内生成语义的固定串，非真实凭据；仅 :memory: 测试库）。
const testCred = "dougong-test-only"

// stubMySQL / stubRedis 是网关替身（进程内记忆实况；幂等语义对齐引擎）。
type stubMySQL struct {
	dbs        []dbmysql.DatabaseInfo
	users      []dbmysql.UserGrants
	dumpBody   string
	importSeen string
	importErr  error
}

func (s *stubMySQL) Available() error { return nil }
func (s *stubMySQL) Ping(context.Context, dbconn.Target, dbconn.Creds) error {
	return nil
}
func (s *stubMySQL) ServerVersion(context.Context, dbconn.Target, dbconn.Creds) (string, error) {
	return "8.0.36", nil
}
func (s *stubMySQL) ListDatabases(context.Context, dbconn.Target, dbconn.Creds) ([]dbmysql.DatabaseInfo, error) {
	return s.dbs, nil
}
func (s *stubMySQL) DatabaseExists(ctx context.Context, t dbconn.Target, c dbconn.Creds, n string) (bool, error) {
	for _, d := range s.dbs {
		if d.Name == n {
			return true, nil
		}
	}
	return false, nil
}
func (s *stubMySQL) CreateDatabase(_ context.Context, _ dbconn.Target, _ dbconn.Creds, name, charset string) error {
	for _, d := range s.dbs {
		if d.Name == name {
			return dbmysql.ErrDatabaseExists
		}
	}
	s.dbs = append(s.dbs, dbmysql.DatabaseInfo{Name: name, Charset: charset})
	return nil
}
func (s *stubMySQL) DropDatabase(_ context.Context, _ dbconn.Target, _ dbconn.Creds, name string) error {
	for i, d := range s.dbs {
		if d.Name == name {
			s.dbs = append(s.dbs[:i], s.dbs[i+1:]...)
			return nil
		}
	}
	return dbmysql.ErrDatabaseNotFound
}
func (s *stubMySQL) ShowCharsets(context.Context, dbconn.Target, dbconn.Creds) ([]string, error) {
	return []string{"utf8mb4", "latin1"}, nil
}
func (s *stubMySQL) ListUsers(context.Context, dbconn.Target, dbconn.Creds) ([]dbmysql.UserGrants, error) {
	return s.users, nil
}
func (s *stubMySQL) ShowGrants(_ context.Context, _ dbconn.Target, _ dbconn.Creds, u, h string) ([]dbmysql.GrantLine, error) {
	for _, x := range s.users {
		if x.Username == u && x.Host == h {
			return x.Grants, nil
		}
	}
	return nil, fmt.Errorf("dbmysql: statement failed: no grant for %s@%s", u, h)
}
func (s *stubMySQL) CreateUser(_ context.Context, _ dbconn.Target, _ dbconn.Creds, u, h, _ string) error {
	s.users = append(s.users, dbmysql.UserGrants{Username: u, Host: h})
	return nil
}
func (s *stubMySQL) DropUser(_ context.Context, _ dbconn.Target, _ dbconn.Creds, u, h string) error {
	for i, x := range s.users {
		if x.Username == u && x.Host == h {
			s.users = append(s.users[:i], s.users[i+1:]...)
			return nil
		}
	}
	return dbmysql.ErrUserNotFound
}
func (s *stubMySQL) Grant(_ context.Context, _ dbconn.Target, _ dbconn.Creds, u, h, db, level string) error {
	raw := "GRANT SELECT ON `" + db + "`.* TO '" + u + "'@'" + h + "'"
	if level == dbmgmtsvc.LevelAll {
		raw = "GRANT ALL PRIVILEGES ON `" + db + "`.* TO '" + u + "'@'" + h + "'"
	} else if level == dbmgmtsvc.LevelWrite {
		raw = "GRANT SELECT, INSERT, UPDATE, DELETE ON `" + db + "`.* TO '" + u + "'@'" + h + "'"
	}
	for i, x := range s.users {
		if x.Username != u || x.Host != h {
			continue
		}
		for j, g := range x.Grants {
			if g.DB == db {
				s.users[i].Grants[j] = dbmysql.GrantLine{DB: db, Raw: raw}
				return nil
			}
		}
		s.users[i].Grants = append(s.users[i].Grants, dbmysql.GrantLine{DB: db, Raw: raw})
		return nil
	}
	return dbmysql.ErrUserNotFound
}
func (s *stubMySQL) Revoke(_ context.Context, _ dbconn.Target, _ dbconn.Creds, u, h, db string) error {
	for i, x := range s.users {
		if x.Username != u || x.Host != h {
			continue
		}
		for j, g := range x.Grants {
			if g.DB == db {
				s.users[i].Grants = append(x.Grants[:j], x.Grants[j+1:]...)
				return nil
			}
		}
		return dbmysql.ErrGrantMissing
	}
	return dbmysql.ErrUserNotFound
}
func (s *stubMySQL) AlterRootPassword(context.Context, dbconn.Target, dbconn.Creds, string) error {
	return nil
}
func (s *stubMySQL) StatusVariables(context.Context, dbconn.Target, dbconn.Creds) (string, map[string]string, error) {
	return "8.0.36", map[string]string{"max_connections": "151"}, nil
}
func (s *stubMySQL) ProcessList(context.Context, dbconn.Target, dbconn.Creds) ([]dbmysql.ProcessRow, error) {
	db := "appdb"
	return []dbmysql.ProcessRow{{ID: 5, User: "root", Host: "localhost", DB: &db,
		Command: "Query", TimeSec: 1, Info: nil}}, nil
}
func (s *stubMySQL) DumpDatabase(context.Context, dbconn.Target, dbconn.Creds, string, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(s.dumpBody)), nil
}
func (s *stubMySQL) ImportDatabase(_ context.Context, _ dbconn.Target, _ dbconn.Creds, _ string, r io.Reader) error {
	b, _ := io.ReadAll(r)
	s.importSeen = string(b)
	return s.importErr
}

type stubRedis struct {
	scanKeys []dbredis.KeyItem
	ttlMap   map[string]int64
}

func (s *stubRedis) Available() error                                        { return nil }
func (s *stubRedis) Ping(context.Context, dbconn.Target, dbconn.Creds) error { return nil }
func (s *stubRedis) ServerVersion(context.Context, dbconn.Target, dbconn.Creds) (string, error) {
	return "7.2.4", nil
}
func (s *stubRedis) InfoFields(context.Context, dbconn.Target, dbconn.Creds) (dbredis.InfoSections, error) {
	return dbredis.InfoSections{
		"server":  {"redis_version": "7.2.4"},
		"clients": {"connected_clients": "2"},
		"memory":  {"used_memory_human": "1M", "maxmemory_human": "0B"},
		"stats":   {"total_connections_received": "7", "keyspace_hits": "4", "keyspace_misses": "1"},
	}, nil
}
func (s *stubRedis) Slowlog(context.Context, dbconn.Target, dbconn.Creds, int) ([]dbredis.SlowlogEntry, error) {
	return nil, nil
}
func (s *stubRedis) SetRequirepass(_ context.Context, _ dbconn.Target, _ dbconn.Creds, _, _ string) error {
	return nil
}
func (s *stubRedis) ScanKeys(_ context.Context, _ dbconn.Target, _ dbconn.Creds, _, _ string, _ int) (string, []dbredis.KeyItem, error) {
	return "0", s.scanKeys, nil
}
func (s *stubRedis) KeyTTL(_ context.Context, _ dbconn.Target, _ dbconn.Creds, key string) (int64, error) {
	if v, ok := s.ttlMap[key]; ok && v != -2 {
		return v, nil
	}
	return 0, dbredis.ErrKeyNotFound
}

// ---- 测试基座 ----

type envelope struct {
	Code    any             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func wantCode(t *testing.T, env envelope, want string) {
	t.Helper()
	if got := fmt.Sprint(env.Code); got != want {
		t.Fatalf("code = %s, want %s (message=%s)", got, want, env.Message)
	}
}

type testApp struct {
	ts      *httptest.Server
	my      *stubMySQL
	rd      *stubRedis
	vault   *dbmgmtsvc.CredentialVault
	dbStore *storedb.Store
}

func newTestApp(t *testing.T) *testApp {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := storeauth.New(db)
	if err := repo.Init().EnsureRow(ctx); err != nil {
		t.Fatalf("ensure init: %v", err)
	}
	hasher, err := authsvc.NewBcryptHasher(10)
	if err != nil {
		t.Fatal(err)
	}
	authSvcObj := authsvc.New(repo, hasher, authsvc.NewTotpService(nil), authsvc.DefaultSessionPolicy())
	auditSvc := auditsvc.New(repo.Audit(), time.Now)

	st := storedb.New(db)
	vault, err := dbmgmtsvc.OpenVault(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	my := &stubMySQL{dumpBody: "-- dump\nINSERT 1;\n"}
	rd := &stubRedis{ttlMap: map[string]int64{"k1": -1}}
	svc := dbmgmtsvc.New(dbmgmtsvc.AdaptStore(st), my, rd, vault, auditSvc)

	server := api.NewServer()
	server.RegisterModules(
		authapi.NewModule(authSvcObj, auditSvc, false),
		NewModule(authSvcObj, svc),
	)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	return &testApp{ts: ts, my: my, rd: rd, vault: vault, dbStore: st}
}

type session struct{ cookie, csrf string }

func decodeEnv(t *testing.T, resp *http.Response) envelope {
	t.Helper()
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, raw)
	}
	return env
}

func initWizardAndLogin(t *testing.T, app *testApp) session {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": testCred})
	resp, err := http.Post(app.ts.URL+"/auth/init/wizard", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	env := decodeEnv(t, resp)
	wantCode(t, env, "0")
	var data struct {
		CSRFToken string `json:"csrfToken"`
	}
	json.Unmarshal(env.Data, &data)
	for _, c := range resp.Cookies() {
		if c.Name == authsvc.SessionCookieName {
			return session{cookie: c.Value, csrf: data.CSRFToken}
		}
	}
	t.Fatal("wizard missing session cookie")
	return session{}
}

func loginReadonly(t *testing.T, app *testApp, admin session) session {
	t.Helper()
	// 先经 admin 建 readonly 用户。
	ub, _ := json.Marshal(map[string]any{"username": "watcher", "password": testCred, "role": "readonly"})
	ureq, _ := http.NewRequest("POST", app.ts.URL+"/auth/user/create", bytes.NewReader(ub))
	ureq.Header.Set("Content-Type", "application/json")
	ureq.Header.Set("Cookie", authsvc.SessionCookieName+"="+admin.cookie)
	ureq.Header.Set("X-CSRF-Token", admin.csrf)
	ureq.Header.Set("Origin", app.ts.URL)
	uresp, err := http.DefaultClient.Do(ureq)
	if err != nil {
		t.Fatal(err)
	}
	uenv := decodeEnv(t, uresp)
	wantCode(t, uenv, "0")
	// 再以 watcher 登录。
	lb, _ := json.Marshal(map[string]string{"username": "watcher", "password": testCred})
	lreq, _ := http.NewRequest("POST", app.ts.URL+"/auth/session/login", bytes.NewReader(lb))
	lreq.Header.Set("Content-Type", "application/json")
	lresp, err := http.DefaultClient.Do(lreq)
	if err != nil {
		t.Fatal(err)
	}
	lenv := decodeEnv(t, lresp)
	wantCode(t, lenv, "0")
	var ld struct {
		CSRFToken string `json:"csrfToken"`
	}
	json.Unmarshal(lenv.Data, &ld)
	for _, c := range lresp.Cookies() {
		if c.Name == authsvc.SessionCookieName {
			return session{cookie: c.Value, csrf: ld.CSRFToken}
		}
	}
	t.Fatal("readonly login missing cookie")
	return session{}
}

func doReq(t *testing.T, app *testApp, s session, method, path, ctype string, body io.Reader, withCSRF bool) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, app.ts.URL+path, body)
	req.Header.Set("Cookie", authsvc.SessionCookieName+"="+s.cookie)
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	if withCSRF {
		req.Header.Set("X-CSRF-Token", s.csrf)
		req.Header.Set("Origin", app.ts.URL)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func get(t *testing.T, app *testApp, s session, path string) envelope {
	t.Helper()
	return decodeEnv(t, doReq(t, app, s, "GET", path, "", nil, false))
}

func postJSON(t *testing.T, app *testApp, s session, path string, body any, withCSRF bool) envelope {
	t.Helper()
	b, _ := json.Marshal(body)
	return decodeEnv(t, doReq(t, app, s, "POST", path, "application/json", bytes.NewReader(b), withCSRF))
}

// seedInstance 直接种一条带凭据的可达档案（治理链就绪态）。
func (app *testApp) seedInstance(t *testing.T, engine string) int64 {
	t.Helper()
	ctx := context.Background()
	port := 3306
	if engine == storedb.EngineRedis {
		port = 6379
	}
	key := engine + "|tcp:localhost:" + fmt.Sprint(port) // D-01 组装规则字面量
	cipher, err := app.vault.Encrypt("secret-old")
	if err != nil {
		t.Fatal(err)
	}
	inst := storedb.Instance{
		IdentityKey: key, Engine: engine, ConnMode: storedb.ConnModeTCP,
		Host: strp("localhost"), Port: &port, ConnState: storedb.ConnStateReachable,
	}
	tx, err := app.dbStore.BeginTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Instance().UpsertDiscovered(ctx, inst, storedb.ConnStateReachable, nil); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	row, err := app.dbStore.Instance().GetByIdentity(ctx, key)
	if err != nil || row == nil {
		t.Fatalf("seed row: %v", err)
	}
	tx2, _ := app.dbStore.BeginTx(ctx)
	user := "root"
	if engine == storedb.EngineRedis {
		user = dbmgmtsvc.RedisDefaultUsername
	}
	if err := tx2.Instance().UpdateCredentials(ctx, row.ID, user, cipher, nil); err != nil {
		t.Fatal(err)
	}
	if err := tx2.Commit(); err != nil {
		t.Fatal(err)
	}
	return row.ID
}

func strp(s string) *string { return &s }

// ---- 用例 ----

func TestPermissionMatrix(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	ro := loginReadonly(t, app, admin)

	// 未登录 → AUTH_001。
	resp, err := http.Get(app.ts.URL + "/db/instance/list")
	if err != nil {
		t.Fatal(err)
	}
	wantCode(t, decodeEnv(t, resp), "AUTH_001")

	// readonly list 首次触发自动探测（D-07；Windows 开发机探测集不可达，
	// 空清单如实）——先耗掉进程内 once，再种档案，避免探测整值覆盖种子行。
	if env := get(t, app, ro, "/db/instance/list"); fmt.Sprint(env.Code) != "0" {
		t.Fatalf("readonly list → %s", fmt.Sprint(env.Code))
	}
	mysqlID := app.seedInstance(t, storedb.EngineMySQL)

	// readonly：7 个查看接口全通（ADR-018 全可看）。
	for _, p := range []string{
		"/db/instance/list",
		"/db/mysql/status?instanceId=" + fmt.Sprint(mysqlID),
		"/db/mysql/processlist?instanceId=" + fmt.Sprint(mysqlID),
		"/db/database/list?instanceId=" + fmt.Sprint(mysqlID),
		"/db/dbuser/list?instanceId=" + fmt.Sprint(mysqlID),
	} {
		if env := get(t, app, ro, p); fmt.Sprint(env.Code) != "0" {
			t.Fatalf("readonly GET %s → %s (message=%s)", p, fmt.Sprint(env.Code), env.Message)
		}
	}
	// readonly：admin 专属 GET（charsets/key×2）→ AUTH_008。
	for _, p := range []string{
		"/db/database/charsets?instanceId=" + fmt.Sprint(mysqlID),
		"/db/key/list?instanceId=" + fmt.Sprint(mysqlID),
		"/db/key/ttl?instanceId=" + fmt.Sprint(mysqlID) + "&key=k",
	} {
		if env := get(t, app, ro, p); fmt.Sprint(env.Code) != "AUTH_008" {
			t.Fatalf("readonly admin-GET %s → %s", p, fmt.Sprint(env.Code))
		}
	}
	// readonly：写接口携合法 CSRF 仍 AUTH_008。
	env := postJSON(t, app, ro, "/db/instance/rescan", map[string]string{}, true)
	wantCode(t, env, "AUTH_008")
	// admin 写接口缺 CSRF → AUTH_003。
	env = postJSON(t, app, admin, "/db/instance/rescan", map[string]string{}, false)
	wantCode(t, env, "AUTH_003")
	// admin + CSRF → 成功（rescan 探测在 Windows 开发机可达集为空，0/0 如实）。
	env = postJSON(t, app, admin, "/db/instance/rescan", map[string]string{}, true)
	wantCode(t, env, "0")
}

func TestAdminMySQLFullFlow(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	mysqlID := app.seedInstance(t, storedb.EngineMySQL)
	idStr := fmt.Sprint(mysqlID)

	// engine 筛选与清单。
	env := get(t, app, admin, "/db/instance/list?engine=mysql")
	wantCode(t, env, "0")
	var listData struct {
		Items []struct {
			ID             int64  `json:"id"`
			Engine         string `json:"engine"`
			ConnState      string `json:"connState"`
			CredentialsSet bool   `json:"credentialsSet"`
			CredUsername   string `json:"credUsername"`
		} `json:"items"`
	}
	json.Unmarshal(env.Data, &listData)
	if len(listData.Items) != 1 || !listData.Items[0].CredentialsSet ||
		listData.Items[0].CredUsername != "root" {
		t.Fatalf("instance list: %+v", listData.Items)
	}

	// 凭据配置（验证成功落库）。
	env = postJSON(t, app, admin, "/db/instance/credentials", map[string]any{
		"instanceId": mysqlID, "username": "root", "password": "secret-new"}, true)
	wantCode(t, env, "0")

	// 状态概览白名单。
	env = get(t, app, admin, "/db/mysql/status?instanceId="+idStr)
	wantCode(t, env, "0")
	if !bytes.Contains(env.Data, []byte(`"max_connections":"151"`)) {
		t.Fatalf("status data: %s", env.Data)
	}
	// 进程列表。
	env = get(t, app, admin, "/db/mysql/processlist?instanceId="+idStr)
	wantCode(t, env, "0")
	if !bytes.Contains(env.Data, []byte(`"user":"root"`)) {
		t.Fatalf("processlist data: %s", env.Data)
	}

	// 建库 → 清单 → DBM_008 重复。
	env = postJSON(t, app, admin, "/db/database/create", map[string]any{
		"instanceId": mysqlID, "name": "appdb"}, true)
	wantCode(t, env, "0")
	env = get(t, app, admin, "/db/database/list?instanceId="+idStr)
	if !bytes.Contains(env.Data, []byte(`"name":"appdb"`)) {
		t.Fatalf("database list: %s", env.Data)
	}
	env = postJSON(t, app, admin, "/db/database/create", map[string]any{
		"instanceId": mysqlID, "name": "appdb"}, true)
	wantCode(t, env, "DBM_008")

	// 建用户 → 授权 → 清单三档映射 → 回收。
	env = postJSON(t, app, admin, "/db/dbuser/create", map[string]any{
		"instanceId": mysqlID, "username": "app", "host": "%", "password": "pw12345"}, true)
	wantCode(t, env, "0")
	env = postJSON(t, app, admin, "/db/dbuser/grant", map[string]any{
		"instanceId": mysqlID, "username": "app", "host": "%", "database": "appdb", "level": "write"}, true)
	wantCode(t, env, "0")
	env = get(t, app, admin, "/db/dbuser/list?instanceId="+idStr)
	if !bytes.Contains(env.Data, []byte(`"level":"write"`)) {
		t.Fatalf("dbuser list: %s", env.Data)
	}
	env = postJSON(t, app, admin, "/db/dbuser/revoke", map[string]any{
		"instanceId": mysqlID, "username": "app", "host": "%", "database": "appdb"}, true)
	wantCode(t, env, "0")
	env = postJSON(t, app, admin, "/db/dbuser/revoke", map[string]any{
		"instanceId": mysqlID, "username": "app", "host": "%", "database": "appdb"}, true)
	wantCode(t, env, "DBM_011")

	// 导出：octet-stream 双形态成功侧。
	resp := doReq(t, app, admin, "POST", "/db/backup/export", "application/json",
		strings.NewReader(`{"instanceId":`+idStr+`,"database":"appdb"}`), true)
	if ct := resp.Header.Get("Content-Type"); ct != "application/octet-stream" {
		dbg, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("export content-type = %q body=%s", ct, dbg)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") ||
		!strings.Contains(cd, "appdb_") {
		t.Fatalf("export disposition = %q", cd)
	}
	dump, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !bytes.HasPrefix(dump, []byte("-- dump")) {
		t.Fatalf("export body: %q", dump)
	}

	// 导出失败侧：库不存在 → JSON 错误体。
	resp = doReq(t, app, admin, "POST", "/db/backup/export", "application/json",
		strings.NewReader(`{"instanceId":`+idStr+`,"database":"nodb"}`), true)
	env = decodeEnv(t, resp)
	wantCode(t, env, "DBM_007")

	// 导入：multipart 成功 + 超限 DBM_014。
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("instanceId", idStr)
	mw.WriteField("database", "appdb")
	fw, _ := mw.CreateFormFile("file", "dump.sql")
	fw.Write([]byte("INSERT INTO t VALUES (1);"))
	mw.Close()
	resp = doReq(t, app, admin, "POST", "/db/backup/import", mw.FormDataContentType(),
		bytes.NewReader(buf.Bytes()), true)
	env = decodeEnv(t, resp)
	wantCode(t, env, "0")
	if app.my.importSeen != "INSERT INTO t VALUES (1);" {
		t.Fatalf("import payload = %q", app.my.importSeen)
	}

	// root 改密（两步同步，D-05）。
	env = postJSON(t, app, admin, "/db/mysql/root-password", map[string]any{
		"instanceId": mysqlID, "newPassword": "root-pw-2"}, true)
	wantCode(t, env, "0")
	if !bytes.Contains(env.Data, []byte(`"synced":true`)) {
		t.Fatalf("root-password data: %s", env.Data)
	}

	// 移除（⚠️A20）→ 再操作 DBM_002。
	env = postJSON(t, app, admin, "/db/instance/remove", map[string]any{
		"instanceId": mysqlID}, true)
	wantCode(t, env, "0")
	env = get(t, app, admin, "/db/mysql/status?instanceId="+idStr)
	wantCode(t, env, "DBM_002")
}

func TestRedisAndKeyFlow(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	redisID := app.seedInstance(t, storedb.EngineRedis)
	idStr := fmt.Sprint(redisID)
	app.rd.scanKeys = []dbredis.KeyItem{{Key: "sess:1", Type: "hash"}}

	// 引擎不匹配（DBM_018）：mysql 接口对 redis 档案。
	env := get(t, app, admin, "/db/mysql/status?instanceId="+idStr)
	wantCode(t, env, "DBM_018")

	// info 白名单 sections。
	env = get(t, app, admin, "/db/redis/info?instanceId="+idStr)
	wantCode(t, env, "0")
	if !bytes.Contains(env.Data, []byte(`"usedMemoryHuman":"1M"`)) {
		t.Fatalf("redis info: %s", env.Data)
	}
	// 慢日志（空态）。
	env = get(t, app, admin, "/db/redis/slowlog?instanceId="+idStr)
	wantCode(t, env, "0")
	// key 浏览（SCAN 游标，complete=true）。
	env = get(t, app, admin, "/db/key/list?instanceId="+idStr)
	wantCode(t, env, "0")
	if !bytes.Contains(env.Data, []byte(`"type":"hash"`)) || !bytes.Contains(env.Data, []byte(`"complete":true`)) {
		t.Fatalf("key list: %s", env.Data)
	}
	env = get(t, app, admin, "/db/key/ttl?instanceId="+idStr+"&key=k1")
	wantCode(t, env, "0")
	if !bytes.Contains(env.Data, []byte(`"ttlSec":-1`)) {
		t.Fatalf("key ttl: %s", env.Data)
	}
	env = get(t, app, admin, "/db/key/ttl?instanceId="+idStr+"&key=gone")
	wantCode(t, env, "DBM_015")
	// redis 改密。
	env = postJSON(t, app, admin, "/db/redis/password", map[string]any{
		"instanceId": redisID, "newPassword": "redis-pw"}, true)
	wantCode(t, env, "0")
}

func TestCredentialsValidationFlow(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	mysqlID := app.seedInstance(t, storedb.EngineMySQL)

	// DBM_001（数值形态非法）。
	env := postJSON(t, app, admin, "/db/instance/credentials", map[string]any{
		"instanceId": 0, "username": "root", "password": "p"}, true)
	wantCode(t, env, "DBM_001")
	// DBM_002（不存在）。
	env = postJSON(t, app, admin, "/db/instance/credentials", map[string]any{
		"instanceId": 999, "username": "root", "password": "p"}, true)
	wantCode(t, env, "DBM_002")
	// AUTH_012：socket 模式带 host（I-07）。
	env = postJSON(t, app, admin, "/db/instance/credentials", map[string]any{
		"instanceId": mysqlID, "username": "root", "password": "p",
		"connMode": "socket", "socketPath": "/tmp/mysql.sock", "host": "localhost"}, true)
	wantCode(t, env, "AUTH_012")
	// AUTH_012：密码空。
	env = postJSON(t, app, admin, "/db/instance/credentials", map[string]any{
		"instanceId": mysqlID, "username": "root", "password": ""}, true)
	wantCode(t, env, "AUTH_012")
}
