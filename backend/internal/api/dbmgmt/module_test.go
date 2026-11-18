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
	"github.com/dougong/dougong/internal/adapter/dbmongo"
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
	// v2.3 表级治理记忆实况。
	tables     []dbmysql.TableInfo
	structures map[string][]dbmysql.ColumnInfo // key "db.table"
	rows       map[string][]map[string]any     // key "db.table"
	affected   int64                           // 行写受影响行数（0 → DBM_024）
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

// ---- v2.3 表级治理替身方法 ----

func (s *stubMySQL) ListTables(context.Context, dbconn.Target, dbconn.Creds, string) ([]dbmysql.TableInfo, error) {
	return s.tables, nil
}

func (s *stubMySQL) TableStructure(_ context.Context, _ dbconn.Target, _ dbconn.Creds, db, table string) ([]dbmysql.ColumnInfo, error) {
	cols := s.structures[db+"."+table]
	if len(cols) == 0 {
		return nil, dbmysql.ErrTableNotFound
	}
	return cols, nil
}

func (s *stubMySQL) TableRows(_ context.Context, _ dbconn.Target, _ dbconn.Creds, db, table string, offset, limit int) ([]map[string]any, int64, error) {
	all := s.rows[db+"."+table]
	total := int64(len(all))
	if offset >= len(all) {
		return []map[string]any{}, total, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], total, nil
}

func (s *stubMySQL) DropTable(_ context.Context, _ dbconn.Target, _ dbconn.Creds, db, table string) error {
	if _, ok := s.structures[db+"."+table]; !ok {
		return dbmysql.ErrTableNotFound
	}
	delete(s.structures, db+"."+table)
	delete(s.rows, db+"."+table)
	return nil
}

func (s *stubMySQL) UpdateRow(_ context.Context, _ dbconn.Target, _ dbconn.Creds, _, _, _, _ string, _ map[string]any) (int64, error) {
	return s.affected, nil
}

func (s *stubMySQL) DeleteRow(_ context.Context, _ dbconn.Target, _ dbconn.Creds, _, _, _, _ string) (int64, error) {
	return s.affected, nil
}

// stubMongo 是 MongoGateway 测试替身（零值=成功路径；v0.1.4 反馈第 3 项；
// v2.3 集合/文档治理记忆实况）。
type stubMongo struct {
	pingErr error
	dbs     []dbmongo.DatabaseInfo
	colls   map[string][]dbmongo.CollectionInfo // key db
	docs    map[string][]map[string]any         // key "db.coll"
}

func (s *stubMongo) Available() error { return nil }

func (s *stubMongo) Ping(ctx context.Context, t dbconn.Target, c dbconn.Creds) error {
	return s.pingErr
}

func (s *stubMongo) ServerVersion(ctx context.Context, t dbconn.Target, c dbconn.Creds) (string, error) {
	return "7.0.14", nil
}

func (s *stubMongo) ListDatabases(ctx context.Context, t dbconn.Target, c dbconn.Creds) ([]dbmongo.DatabaseInfo, error) {
	if s.dbs != nil {
		return s.dbs, nil
	}
	return []dbmongo.DatabaseInfo{{Name: "admin", SizeBytes: 1, Collections: 2}, {Name: "appdb", SizeBytes: 4096, Collections: 5}}, nil
}

func (s *stubMongo) StatusSummary(ctx context.Context, t dbconn.Target, c dbconn.Creds) (*dbmongo.StatusSummary, error) {
	return &dbmongo.StatusSummary{Version: "7.0.14", UptimeSec: 100, ConnectionsCurrent: 2,
		ConnectionsAvailable: 838858, MemResidentMb: 64, OpInsert: 3, OpQuery: 5, OpUpdate: 1, OpDelete: 0}, nil
}

func (s *stubMongo) ListUsers(ctx context.Context, t dbconn.Target, c dbconn.Creds) ([]dbmongo.UserItem, error) {
	return []dbmongo.UserItem{{Username: "app", Mechanisms: []string{"SCRAM-SHA-256"},
		Roles: []dbmongo.UserRole{{DB: "appdb", Role: "readWrite"}}}}, nil
}

func (s *stubMongo) CreateUser(ctx context.Context, t dbconn.Target, c dbconn.Creds, username, password, db, role string) error {
	return nil
}

func (s *stubMongo) DropUser(ctx context.Context, t dbconn.Target, c dbconn.Creds, username string) error {
	return nil
}

func (s *stubMongo) GrantRole(ctx context.Context, t dbconn.Target, c dbconn.Creds, username, db, role string) error {
	return nil
}

func (s *stubMongo) RevokeRole(ctx context.Context, t dbconn.Target, c dbconn.Creds, username, db, role string) error {
	return nil
}

// ---- v2.3 集合/文档治理替身方法（记忆实况，引擎语义对齐）----

func (s *stubMongo) DropDatabase(_ context.Context, _ dbconn.Target, _ dbconn.Creds, db string) error {
	for i, d := range s.dbs {
		if d.Name == db {
			s.dbs = append(s.dbs[:i], s.dbs[i+1:]...)
			break
		}
	}
	delete(s.colls, db)
	return nil
}

func (s *stubMongo) ListCollections(_ context.Context, _ dbconn.Target, _ dbconn.Creds, db string) ([]dbmongo.CollectionInfo, error) {
	return s.colls[db], nil
}

func (s *stubMongo) CreateCollection(_ context.Context, _ dbconn.Target, _ dbconn.Creds, db, name string) error {
	for _, coll := range s.colls[db] {
		if coll.Name == name {
			return dbmongo.ErrCollectionExists
		}
	}
	s.colls[db] = append(s.colls[db], dbmongo.CollectionInfo{Name: name})
	found := false
	for _, d := range s.dbs {
		if d.Name == db {
			found = true
		}
	}
	if !found {
		s.dbs = append(s.dbs, dbmongo.DatabaseInfo{Name: db})
	}
	return nil
}

func (s *stubMongo) DropCollection(_ context.Context, _ dbconn.Target, _ dbconn.Creds, db, name string) error {
	for i, coll := range s.colls[db] {
		if coll.Name == name {
			s.colls[db] = append(s.colls[db][:i], s.colls[db][i+1:]...)
			delete(s.docs, db+"."+name)
			return nil
		}
	}
	return dbmongo.ErrCollectionNotFound
}

func (s *stubMongo) ListDocuments(_ context.Context, _ dbconn.Target, _ dbconn.Creds, db, coll string, skip, limit int) ([]map[string]any, int64, error) {
	all := s.docs[db+"."+coll]
	total := int64(len(all))
	if skip >= len(all) {
		return []map[string]any{}, total, nil
	}
	end := skip + limit
	if end > len(all) {
		end = len(all)
	}
	return all[skip:end], total, nil
}

func (s *stubMongo) InsertDocument(_ context.Context, _ dbconn.Target, _ dbconn.Creds, db, coll string, doc map[string]any) (any, error) {
	key := db + "." + coll
	id := doc["_id"]
	if id == nil {
		id = map[string]any{"$oid": "665f000000000000000000aa"}
		doc["_id"] = id
	}
	for _, d := range s.docs[key] {
		if stubSameID(d["_id"], id) {
			return nil, dbmongo.ErrDocumentExists
		}
	}
	s.docs[key] = append(s.docs[key], doc)
	return id, nil
}

func (s *stubMongo) ReplaceDocument(_ context.Context, _ dbconn.Target, _ dbconn.Creds, db, coll string, id any, doc map[string]any) (int64, error) {
	key := db + "." + coll
	for i, d := range s.docs[key] {
		if stubSameID(d["_id"], id) {
			s.docs[key][i] = doc
			return 1, nil
		}
	}
	return 0, nil
}

func (s *stubMongo) DeleteDocument(_ context.Context, _ dbconn.Target, _ dbconn.Creds, db, coll string, id any) (int64, error) {
	key := db + "." + coll
	for i, d := range s.docs[key] {
		if stubSameID(d["_id"], id) {
			s.docs[key] = append(s.docs[key][:i], s.docs[key][i+1:]...)
			return 1, nil
		}
	}
	return 0, nil
}

func stubSameID(a, b any) bool {
	ab, err := json.Marshal(a)
	if err != nil {
		return false
	}
	bb, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(ab) == string(bb)
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
	mg      *stubMongo
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
	my := &stubMySQL{dumpBody: "-- dump\nINSERT 1;\n",
		structures: map[string][]dbmysql.ColumnInfo{},
		rows:       map[string][]map[string]any{},
		affected:   1}
	rd := &stubRedis{ttlMap: map[string]int64{"k1": -1}}
	mg := &stubMongo{
		dbs: []dbmongo.DatabaseInfo{{Name: "admin", SizeBytes: 1, Collections: 2}, {Name: "appdb", SizeBytes: 4096, Collections: 1}},
		colls: map[string][]dbmongo.CollectionInfo{
			"appdb": {{Name: "orders", DocCount: 1, SizeBytes: 1024}},
		},
		docs: map[string][]map[string]any{
			"appdb.orders": {{"_id": map[string]any{"$oid": "665f00000000000000000001"}, "sku": "A"}},
		},
	}
	svc := dbmgmtsvc.New(dbmgmtsvc.AdaptStore(st), my, rd, mg, vault, auditSvc)

	server := api.NewServer()
	server.RegisterModules(
		authapi.NewModule(authSvcObj, auditSvc, false),
		NewModule(authSvcObj, svc),
	)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	return &testApp{ts: ts, my: my, rd: rd, mg: mg, vault: vault, dbStore: st}
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
	if engine == storedb.EngineMongoDB {
		port = 27017
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

// restoreReachable 探测整值覆盖后恢复档案 reachable（治理链用例前置）。
func restoreReachable(t *testing.T, app *testApp, ids ...int64) {
	t.Helper()
	ctx := context.Background()
	tx, err := app.dbStore.BeginTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if err := tx.Instance().UpdateConnState(ctx, id, storedb.ConnStateReachable, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

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
	// readonly：mongo 族 3 个查看接口全通（v0.1.4 反馈第 3 项）。
	mongoID := app.seedInstance(t, storedb.EngineMongoDB)
	for _, p := range []string{
		"/db/mongo/status?instanceId=" + fmt.Sprint(mongoID),
		"/db/mdb/list?instanceId=" + fmt.Sprint(mongoID),
		"/db/muser/list?instanceId=" + fmt.Sprint(mongoID),
	} {
		if env := get(t, app, ro, p); fmt.Sprint(env.Code) != "0" {
			t.Fatalf("readonly mongo GET %s → %s (message=%s)", p, fmt.Sprint(env.Code), env.Message)
		}
	}
	// readonly：mongo 写接口携合法 CSRF → AUTH_008。
	env := postJSON(t, app, ro, "/db/muser/create", map[string]any{
		"instanceId": mongoID, "username": "x", "password": "p", "db": "d", "role": "read"}, true)
	wantCode(t, env, "AUTH_008")
	// readonly：写接口携合法 CSRF 仍 AUTH_008。
	env = postJSON(t, app, ro, "/db/instance/rescan", map[string]string{}, true)
	wantCode(t, env, "AUTH_008")
	// admin 写接口缺 CSRF → AUTH_003。
	env = postJSON(t, app, admin, "/db/instance/rescan", map[string]string{}, false)
	wantCode(t, env, "AUTH_003")
	// admin + CSRF → 成功（rescan 探测在 Windows 开发机可达集为空，0/0 如实）。
	env = postJSON(t, app, admin, "/db/instance/rescan", map[string]string{}, true)
	wantCode(t, env, "0")

	// ---- v2.3 新增 14 接口权限矩阵（D-24/I-11：元数据两角色 / 数据内容
	// GET admin 专属 / POST admin+CSRF）----
	// 前置：上方 rescan 在 Windows 开发机将种子档案整值覆盖 unreachable
	// （I-03 探测态如实）——恢复 reachable 供治理链用例。
	restoreReachable(t, app, mysqlID, mongoID)
	// admin 建 appdb + 种 users 表（元数据 GET 成功路径需实况对象）。
	env = postJSON(t, app, admin, "/db/database/create", map[string]any{
		"instanceId": mysqlID, "name": "appdb"}, true)
	wantCode(t, env, "0")
	app.my.structures["appdb.users"] = []dbmysql.ColumnInfo{
		{Column: "id", Type: "int(11)", Key: "PRI"}, {Column: "name", Type: "varchar(64)", Nullable: true}}
	app.my.rows["appdb.users"] = []map[string]any{{"id": float64(1), "name": "alpha"}}
	app.my.tables = []dbmysql.TableInfo{{Name: "users", Engine: "InnoDB"}}

	// readonly：元数据 GET 两角色全通（table/list、table/structure、mcoll/list）。
	for _, p := range []string{
		"/db/table/list?instanceId=" + fmt.Sprint(mysqlID) + "&database=appdb",
		"/db/table/structure?instanceId=" + fmt.Sprint(mysqlID) + "&database=appdb&table=users",
		"/db/mcoll/list?instanceId=" + fmt.Sprint(mongoID) + "&database=appdb",
	} {
		if env := get(t, app, ro, p); fmt.Sprint(env.Code) != "0" {
			t.Fatalf("readonly metadata GET %s → %s (message=%s)", p, fmt.Sprint(env.Code), env.Message)
		}
	}
	// readonly：数据内容 GET（row/list、mdoc/list）→ AUTH_008（I-11 沿 key 先例）。
	for _, p := range []string{
		"/db/row/list?instanceId=" + fmt.Sprint(mysqlID) + "&database=appdb&table=users",
		"/db/mdoc/list?instanceId=" + fmt.Sprint(mongoID) + "&database=appdb&collection=orders",
	} {
		if env := get(t, app, ro, p); fmt.Sprint(env.Code) != "AUTH_008" {
			t.Fatalf("readonly data-content GET %s → %s, want AUTH_008", p, fmt.Sprint(env.Code))
		}
	}
	// admin：数据内容 GET 全通。
	for _, p := range []string{
		"/db/row/list?instanceId=" + fmt.Sprint(mysqlID) + "&database=appdb&table=users",
		"/db/mdoc/list?instanceId=" + fmt.Sprint(mongoID) + "&database=appdb&collection=orders",
	} {
		if env := get(t, app, admin, p); fmt.Sprint(env.Code) != "0" {
			t.Fatalf("admin data-content GET %s → %s (message=%s)", p, fmt.Sprint(env.Code), env.Message)
		}
	}
	// readonly：v2.3 全部写接口携合法 CSRF → AUTH_008。
	roWrites := []struct {
		path string
		body map[string]any
	}{
		{"/db/table/drop", map[string]any{"instanceId": mysqlID, "database": "appdb", "table": "users"}},
		{"/db/row/update", map[string]any{"instanceId": mysqlID, "database": "appdb", "table": "users", "pkValue": "1", "values": map[string]any{"name": "x"}}},
		{"/db/row/delete", map[string]any{"instanceId": mysqlID, "database": "appdb", "table": "users", "pkValue": "1"}},
		{"/db/mcoll/create", map[string]any{"instanceId": mongoID, "database": "appdb", "name": "c1"}},
		{"/db/mcoll/drop", map[string]any{"instanceId": mongoID, "database": "appdb", "name": "orders"}},
		{"/db/mdb/dropdb", map[string]any{"instanceId": mongoID, "database": "appdb"}},
		{"/db/mdoc/insert", map[string]any{"instanceId": mongoID, "database": "appdb", "collection": "orders", "document": map[string]any{"sku": "X"}}},
		{"/db/mdoc/update", map[string]any{"instanceId": mongoID, "database": "appdb", "collection": "orders", "id": map[string]any{"$oid": "665f00000000000000000001"}, "document": map[string]any{"sku": "Y"}}},
		{"/db/mdoc/delete", map[string]any{"instanceId": mongoID, "database": "appdb", "collection": "orders", "id": map[string]any{"$oid": "665f00000000000000000001"}}},
	}
	for _, w := range roWrites {
		if env := postJSON(t, app, ro, w.path, w.body, true); fmt.Sprint(env.Code) != "AUTH_008" {
			t.Fatalf("readonly POST %s → %s, want AUTH_008", w.path, fmt.Sprint(env.Code))
		}
	}
	// admin 写接口缺 CSRF → AUTH_003（抽 table/drop 与 mdoc/insert 两族）。
	env = postJSON(t, app, admin, "/db/table/drop", roWrites[0].body, false)
	wantCode(t, env, "AUTH_003")
	env = postJSON(t, app, admin, "/db/mdoc/insert", roWrites[6].body, false)
	wantCode(t, env, "AUTH_003")
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

// TestMongoGovernanceFlow mongo 族治理 HTTP 全流程（v0.1.4 反馈第 3 项）。
func TestMongoGovernanceFlow(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	_ = get(t, app, admin, "/db/instance/list") // 耗掉首次自动探测 once
	mongoID := app.seedInstance(t, storedb.EngineMongoDB)

	// 状态/库清单/用户清单（stub 实况）。
	env := get(t, app, admin, "/db/mongo/status?instanceId="+fmt.Sprint(mongoID))
	wantCode(t, env, "0")
	env = get(t, app, admin, "/db/mdb/list?instanceId="+fmt.Sprint(mongoID))
	wantCode(t, env, "0")
	env = get(t, app, admin, "/db/muser/list?instanceId="+fmt.Sprint(mongoID))
	wantCode(t, env, "0")

	// 建用户（缺 CSRF → AUTH_003；合法 → 0；非法角色 → AUTH_012）。
	env = postJSON(t, app, admin, "/db/muser/create", map[string]any{
		"instanceId": mongoID, "username": "app", "password": "secret", "db": "appdb", "role": "read"}, false)
	wantCode(t, env, "AUTH_003")
	env = postJSON(t, app, admin, "/db/muser/create", map[string]any{
		"instanceId": mongoID, "username": "app", "password": "secret", "db": "appdb", "role": "root"}, true)
	wantCode(t, env, "AUTH_012")
	env = postJSON(t, app, admin, "/db/muser/create", map[string]any{
		"instanceId": mongoID, "username": "app", "password": "secret", "db": "appdb", "role": "readWrite"}, true)
	wantCode(t, env, "0")

	// 引擎不匹配：mysql 族接口对 mongodb 实例 → DBM_018（HTTP 链）。
	env = get(t, app, admin, "/db/database/list?instanceId="+fmt.Sprint(mongoID))
	wantCode(t, env, "DBM_018")
	env = get(t, app, admin, "/db/redis/info?instanceId="+fmt.Sprint(mongoID))
	wantCode(t, env, "DBM_018")
}

// TestTableGovernanceFlow v2.3 MySQL 表级治理 HTTP 全流程（§4.7）。
func TestTableGovernanceFlow(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	_ = get(t, app, admin, "/db/instance/list") // 耗掉首次自动探测 once
	mysqlID := app.seedInstance(t, storedb.EngineMySQL)
	idStr := fmt.Sprint(mysqlID)

	env := postJSON(t, app, admin, "/db/database/create", map[string]any{
		"instanceId": mysqlID, "name": "appdb"}, true)
	wantCode(t, env, "0")
	app.my.structures["appdb.users"] = []dbmysql.ColumnInfo{
		{Column: "id", Type: "int(11)", Key: "PRI", Extra: "auto_increment"},
		{Column: "name", Type: "varchar(64)", Nullable: true}}
	app.my.rows["appdb.users"] = []map[string]any{
		{"id": float64(1), "name": "alpha"}, {"id": float64(2), "name": nil}}
	app.my.tables = []dbmysql.TableInfo{{Name: "users", Engine: "InnoDB"}}

	// 表清单/结构（singlePk 派生）。
	env = get(t, app, admin, "/db/table/list?instanceId="+idStr+"&database=appdb")
	wantCode(t, env, "0")
	if !bytes.Contains(env.Data, []byte(`"name":"users"`)) {
		t.Fatalf("table list: %s", env.Data)
	}
	env = get(t, app, admin, "/db/table/structure?instanceId="+idStr+"&database=appdb&table=users")
	wantCode(t, env, "0")
	if !bytes.Contains(env.Data, []byte(`"singlePk":true`)) || !bytes.Contains(env.Data, []byte(`"pkColumn":"id"`)) {
		t.Fatalf("structure singlePk: %s", env.Data)
	}
	// 表不存在 DBM_022。
	env = get(t, app, admin, "/db/table/structure?instanceId="+idStr+"&database=appdb&table=gone")
	wantCode(t, env, "DBM_022")

	// 行数据分页（columns 列序 + NULL 语义 + total）。
	env = get(t, app, admin, "/db/row/list?instanceId="+idStr+"&database=appdb&table=users&page=1&pageSize=1")
	wantCode(t, env, "0")
	if !bytes.Contains(env.Data, []byte(`"columns":["id","name"]`)) ||
		!bytes.Contains(env.Data, []byte(`"total":2`)) || !bytes.Contains(env.Data, []byte(`"pageSize":1`)) {
		t.Fatalf("row list: %s", env.Data)
	}
	env = get(t, app, admin, "/db/row/list?instanceId="+idStr+"&database=appdb&table=users&page=2&pageSize=1")
	if !bytes.Contains(env.Data, []byte(`"name":null`)) {
		t.Fatalf("NULL cell must serialize as null: %s", env.Data)
	}

	// 行编辑（null→NULL）+ 行删除。
	env = postJSON(t, app, admin, "/db/row/update", map[string]any{
		"instanceId": mysqlID, "database": "appdb", "table": "users",
		"pkValue": "1", "values": map[string]any{"name": nil}}, true)
	wantCode(t, env, "0")
	if !bytes.Contains(env.Data, []byte(`"affected":1`)) {
		t.Fatalf("row update: %s", env.Data)
	}
	env = postJSON(t, app, admin, "/db/row/delete", map[string]any{
		"instanceId": mysqlID, "database": "appdb", "table": "users", "pkValue": "2"}, true)
	wantCode(t, env, "0")
	// affected=0 → DBM_024（不静默成功）。
	app.my.affected = 0
	env = postJSON(t, app, admin, "/db/row/delete", map[string]any{
		"instanceId": mysqlID, "database": "appdb", "table": "users", "pkValue": "99"}, true)
	wantCode(t, env, "DBM_024")
	app.my.affected = 1
	// 主键列不可编辑 AUTH_012。
	env = postJSON(t, app, admin, "/db/row/update", map[string]any{
		"instanceId": mysqlID, "database": "appdb", "table": "users",
		"pkValue": "1", "values": map[string]any{"id": "5"}}, true)
	wantCode(t, env, "AUTH_012")
	// 无单列主键表 → DBM_023。
	app.my.structures["appdb.nopk"] = []dbmysql.ColumnInfo{{Column: "a", Key: ""}}
	env = postJSON(t, app, admin, "/db/row/delete", map[string]any{
		"instanceId": mysqlID, "database": "appdb", "table": "nopk", "pkValue": "1"}, true)
	wantCode(t, env, "DBM_023")

	// 删表 + 再删 DBM_022。
	env = postJSON(t, app, admin, "/db/table/drop", map[string]any{
		"instanceId": mysqlID, "database": "appdb", "table": "users"}, true)
	wantCode(t, env, "0")
	env = postJSON(t, app, admin, "/db/table/drop", map[string]any{
		"instanceId": mysqlID, "database": "appdb", "table": "users"}, true)
	wantCode(t, env, "DBM_022")
	// 引擎不匹配：table 接口对 mongodb 实例 DBM_018。
	mongoID := app.seedInstance(t, storedb.EngineMongoDB)
	env = get(t, app, admin, "/db/table/list?instanceId="+fmt.Sprint(mongoID)+"&database=appdb")
	wantCode(t, env, "DBM_018")
}

// TestMongoCollDocFlow v2.3 Mongo 集合/文档治理 HTTP 全流程（§4.8）。
func TestMongoCollDocFlow(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	_ = get(t, app, admin, "/db/instance/list")
	mongoID := app.seedInstance(t, storedb.EngineMongoDB)
	idStr := fmt.Sprint(mongoID)
	oid1 := map[string]any{"$oid": "665f00000000000000000001"}

	// 集合清单 + 建集合（隐式建库）+ 重名 DBM_008 + 形态 AUTH_012。
	env := get(t, app, admin, "/db/mcoll/list?instanceId="+idStr+"&database=appdb")
	wantCode(t, env, "0")
	if !bytes.Contains(env.Data, []byte(`"name":"orders"`)) {
		t.Fatalf("mcoll list: %s", env.Data)
	}
	env = postJSON(t, app, admin, "/db/mcoll/create", map[string]any{
		"instanceId": mongoID, "database": "newdb", "name": "first"}, true)
	wantCode(t, env, "0")
	env = postJSON(t, app, admin, "/db/mcoll/create", map[string]any{
		"instanceId": mongoID, "database": "appdb", "name": "orders"}, true)
	wantCode(t, env, "DBM_008")
	env = postJSON(t, app, admin, "/db/mcoll/create", map[string]any{
		"instanceId": mongoID, "database": "appdb", "name": "$bad"}, true)
	wantCode(t, env, "AUTH_012")

	// 文档分页（扩展 JSON $oid 透传）。
	env = get(t, app, admin, "/db/mdoc/list?instanceId="+idStr+"&database=appdb&collection=orders")
	wantCode(t, env, "0")
	if !bytes.Contains(env.Data, []byte(`"$oid":"665f00000000000000000001"`)) ||
		!bytes.Contains(env.Data, []byte(`"total":1`)) {
		t.Fatalf("mdoc list: %s", env.Data)
	}
	// 集合不存在 DBM_025。
	env = get(t, app, admin, "/db/mdoc/list?instanceId="+idStr+"&database=appdb&collection=gone")
	wantCode(t, env, "DBM_025")

	// 插入（缺省 _id）+ JSON 非法 DBM_026（数组形态）。
	env = postJSON(t, app, admin, "/db/mdoc/insert", map[string]any{
		"instanceId": mongoID, "database": "appdb", "collection": "orders",
		"document": map[string]any{"sku": "B"}}, true)
	wantCode(t, env, "0")
	if !bytes.Contains(env.Data, []byte(`"insertedId"`)) {
		t.Fatalf("mdoc insert: %s", env.Data)
	}
	env = postJSON(t, app, admin, "/db/mdoc/insert", map[string]any{
		"instanceId": mongoID, "database": "appdb", "collection": "orders",
		"document": []any{1, 2}}, true)
	wantCode(t, env, "DBM_026")

	// 编辑（_id 原值透传）+ _id 不一致 AUTH_012 + 匹配 0 DBM_027。
	env = postJSON(t, app, admin, "/db/mdoc/update", map[string]any{
		"instanceId": mongoID, "database": "appdb", "collection": "orders",
		"id": oid1, "document": map[string]any{"sku": "A2"}}, true)
	wantCode(t, env, "0")
	if !bytes.Contains(env.Data, []byte(`"matched":1`)) {
		t.Fatalf("mdoc update: %s", env.Data)
	}
	env = postJSON(t, app, admin, "/db/mdoc/update", map[string]any{
		"instanceId": mongoID, "database": "appdb", "collection": "orders",
		"id": oid1, "document": map[string]any{"_id": map[string]any{"$oid": "665f00000000000000000002"}, "sku": "x"}}, true)
	wantCode(t, env, "AUTH_012")
	env = postJSON(t, app, admin, "/db/mdoc/update", map[string]any{
		"instanceId": mongoID, "database": "appdb", "collection": "orders",
		"id": map[string]any{"$oid": "665f000000000000000000ff"}, "document": map[string]any{"sku": "x"}}, true)
	wantCode(t, env, "DBM_027")

	// 删除 + 再删 DBM_027。
	env = postJSON(t, app, admin, "/db/mdoc/delete", map[string]any{
		"instanceId": mongoID, "database": "appdb", "collection": "orders", "id": oid1}, true)
	wantCode(t, env, "0")
	env = postJSON(t, app, admin, "/db/mdoc/delete", map[string]any{
		"instanceId": mongoID, "database": "appdb", "collection": "orders", "id": oid1}, true)
	wantCode(t, env, "DBM_027")

	// 删集合 + 再删 DBM_025。
	env = postJSON(t, app, admin, "/db/mcoll/drop", map[string]any{
		"instanceId": mongoID, "database": "appdb", "name": "orders"}, true)
	wantCode(t, env, "0")
	env = postJSON(t, app, admin, "/db/mcoll/drop", map[string]any{
		"instanceId": mongoID, "database": "appdb", "name": "orders"}, true)
	wantCode(t, env, "DBM_025")

	// 系统库保护 DBM_028 + 删库 + 再删 DBM_007。
	env = postJSON(t, app, admin, "/db/mdb/dropdb", map[string]any{
		"instanceId": mongoID, "database": "admin"}, true)
	wantCode(t, env, "DBM_028")
	env = postJSON(t, app, admin, "/db/mdb/dropdb", map[string]any{
		"instanceId": mongoID, "database": "appdb"}, true)
	wantCode(t, env, "0")
	env = postJSON(t, app, admin, "/db/mdb/dropdb", map[string]any{
		"instanceId": mongoID, "database": "appdb"}, true)
	wantCode(t, env, "DBM_007")
}
