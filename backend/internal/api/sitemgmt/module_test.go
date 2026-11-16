package sitemgmt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/adapter/acme"
	"github.com/dougong/dougong/internal/adapter/nginxx"
	"github.com/dougong/dougong/internal/adapter/phpfpm"
	"github.com/dougong/dougong/internal/api"
	authapi "github.com/dougong/dougong/internal/api/auth"
	auditsvc "github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	sitesvc "github.com/dougong/dougong/internal/service/sitemgmt"
	"github.com/dougong/dougong/internal/store"
	storeauth "github.com/dougong/dougong/internal/store/auth"
	storesitemgmt "github.com/dougong/dougong/internal/store/sitemgmt"
)

const testCred = "dougong-test-only"

// ---- 替身与工具（镜像 service 层测试替身语义，HTTP 全栈不 mock 数据库）----

type fakeNginx struct {
	mu          sync.Mutex
	root        string
	testOK      bool
	testErr     error
	reloadErr   error
	reloadCalls int
	availErr    error
}

func newFakeNginx(t *testing.T) *fakeNginx {
	t.Helper()
	root := t.TempDir()
	for _, d := range sitesvc.ScanDirNames {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return &fakeNginx{root: root, testOK: true}
}

func (f *fakeNginx) Available() error { return f.availErr }

func (f *fakeNginx) PriorityDirs() []string {
	out := make([]string, len(sitesvc.ScanDirNames))
	for i, d := range sitesvc.ScanDirNames {
		out[i] = filepath.Join(f.root, d)
	}
	return out
}

func (f *fakeNginx) ScanConfDirs() ([]nginxx.FileSnapshot, error) {
	var out []nginxx.FileSnapshot
	for _, dir := range f.PriorityDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		var names []string
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".conf") {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		for _, n := range names {
			p := filepath.Join(dir, n)
			b, err := os.ReadFile(p)
			if err != nil {
				return nil, err
			}
			st, _ := os.Stat(p)
			out = append(out, nginxx.FileSnapshot{Path: p, Dir: dir, Name: n,
				Content: string(b), ModTime: st.ModTime()})
		}
	}
	return out, nil
}

func (f *fakeNginx) ReadFile(path string) (string, time.Time, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", time.Time{}, err
	}
	st, _ := os.Stat(path)
	return string(b), st.ModTime(), nil
}

func (f *fakeNginx) WriteFileAtomic(p string, c []byte) error       { return os.WriteFile(p, c, 0o644) }
func (f *fakeNginx) WriteSecretFileAtomic(p string, c []byte) error { return os.WriteFile(p, c, 0o600) }

func (f *fakeNginx) CopyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

func (f *fakeNginx) MkdirAll(d string) error { return os.MkdirAll(d, 0o755) }
func (f *fakeNginx) Exists(p string) bool    { _, err := os.Stat(p); return err == nil }
func (f *fakeNginx) RemoveFile(p string) error {
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (f *fakeNginx) FirstWritableDir() (string, error) { return f.PriorityDirs()[0], nil }

func (f *fakeNginx) TestConfig(context.Context) (nginxx.TestResult, error) {
	if f.testErr != nil {
		return nginxx.TestResult{}, f.testErr
	}
	return nginxx.TestResult{OK: f.testOK}, nil
}

func (f *fakeNginx) Reload(context.Context) (string, error) {
	f.mu.Lock()
	f.reloadCalls++
	f.mu.Unlock()
	return "", f.reloadErr
}

type fakeAcme struct {
	res *acme.IssueResult
	err error
}

func (f *fakeAcme) Issue(context.Context, []string, string, string) (*acme.IssueResult, error) {
	return f.res, f.err
}

type fakePhp struct{ pools []phpfpm.Pool }

func (f *fakePhp) ListPools(context.Context) ([]phpfpm.Pool, error) { return f.pools, nil }

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
	ts     *httptest.Server
	gw     *fakeNginx
	acmeGw *fakeAcme
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

	gw := newFakeNginx(t)
	dataDir := t.TempDir()
	acmeGw := &fakeAcme{res: &acme.IssueResult{
		CertPEM: []byte("PEM"), KeyPEM: []byte("KEY"),
		NotBefore: time.Now(), NotAfter: time.Now().Add(90 * 24 * time.Hour),
		Issuer: "R11", AccountURI: "acct:1",
	}}
	svc := sitesvc.New(gw, &fakePhp{pools: []phpfpm.Pool{{Name: "www", Socket: "unix:/run/php/www.sock"}}},
		acmeGw, storesitemgmt.New(db), auditSvc, sitesvc.Options{DataDir: dataDir})
	server := api.NewServer()
	server.RegisterModules(
		authapi.NewModule(authSvcObj, auditSvc, false),
		NewModule(authSvcObj, svc),
	)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	return &testApp{ts: ts, gw: gw, acmeGw: acmeGw}
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

func loginAndCreateReadonly(t *testing.T, app *testApp, admin session) session {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"username": "watcher", "password": testCred, "role": "readonly"})
	req, _ := http.NewRequest("POST", app.ts.URL+"/auth/user/create", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", authsvc.SessionCookieName+"="+admin.cookie)
	req.Header.Set("X-CSRF-Token", admin.csrf)
	req.Header.Set("Origin", app.ts.URL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	decodeEnv(t, resp) // 200 envelope

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

func get(t *testing.T, app *testApp, s session, path string) envelope {
	t.Helper()
	req, _ := http.NewRequest("GET", app.ts.URL+path, nil)
	req.Header.Set("Cookie", authsvc.SessionCookieName+"="+s.cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return decodeEnv(t, resp)
}

func post(t *testing.T, app *testApp, s session, path string, body any) envelope {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", app.ts.URL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", authsvc.SessionCookieName+"="+s.cookie)
	if s.csrf != "" {
		req.Header.Set("X-CSRF-Token", s.csrf)
		req.Header.Set("Origin", app.ts.URL)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return decodeEnv(t, resp)
}

// writeConf 写入受管配置文件。
func (app *testApp) writeConf(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(app.gw.root, "sites-enabled", name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// ---- 用例 ----

// 权限矩阵：未登录 AUTH_001；readonly 查看 4 接口可看 + 越界 AUTH_008；
// 编辑前置读取与建站候选 admin 专属（§3.3）。
func TestPermissionMatrix(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	ro := loginAndCreateReadonly(t, app, admin)
	missingLog := filepath.Join(t.TempDir(), "missing.access.log") // 确定性缺失（默认路径回退链穷尽 D-07）
	conf := app.writeConf(t, "a.conf",
		"server {\n    listen 80;\n    server_name a.example.com;\n    access_log "+missingLog+";\n}\n")

	// 未登录。
	for _, p := range []string{"/sites/site/list", "/sites/site/detail?siteId=x%230", "/sites/cert/list"} {
		env := get(t, app, session{}, p)
		wantCode(t, env, "AUTH_001")
	}

	// readonly：查看类两角色同等（log/tail 权限通过后按 D-07 如实报 SITE_011）。
	env := get(t, app, ro, "/sites/site/list")
	wantCode(t, env, "0")
	env = get(t, app, ro, "/sites/cert/list")
	wantCode(t, env, "0")
	env = get(t, app, ro, "/sites/log/tail?siteId="+confEscape(conf+"#0")+"&type=access")
	wantCode(t, env, "SITE_011")

	// readonly：编辑前置读取 + 建站候选 → AUTH_008（ADR-018）。
	env = get(t, app, ro, "/sites/site/source?siteId="+confEscape(conf+"#0"))
	wantCode(t, env, "AUTH_008")
	env = get(t, app, ro, "/sites/php-pools/list")
	wantCode(t, env, "AUTH_008")
	// readonly：写接口（携带合法 CSRF 仍越界拒绝）。
	env = post(t, app, ro, "/sites/site/disable", map[string]string{"siteId": conf + "#0"})
	wantCode(t, env, "AUTH_008")

	// admin：PHP pools 与详情可用。
	env = get(t, app, admin, "/sites/php-pools/list")
	wantCode(t, env, "0")
	env = get(t, app, admin, "/sites/site/source?siteId="+confEscape(conf+"#0"))
	wantCode(t, env, "0")
}

// CSRF：POST 缺头/错 token → AUTH_003（R-02）。
func TestCSRFRequired(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	conf := app.writeConf(t, "a.conf", "server {\n    listen 80;\n}\n")
	siteID := conf + "#0"

	// 缺 CSRF 头。
	b, _ := json.Marshal(map[string]string{"siteId": siteID})
	req, _ := http.NewRequest("POST", app.ts.URL+"/sites/site/disable", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", authsvc.SessionCookieName+"="+admin.cookie)
	resp, _ := http.DefaultClient.Do(req)
	wantCode(t, decodeEnv(t, resp), "AUTH_003")

	// 错 token。
	req2, _ := http.NewRequest("POST", app.ts.URL+"/sites/site/disable", bytes.NewReader(b))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Cookie", authsvc.SessionCookieName+"="+admin.cookie)
	req2.Header.Set("X-CSRF-Token", "wrong")
	req2.Header.Set("Origin", app.ts.URL)
	resp2, _ := http.DefaultClient.Do(req2)
	wantCode(t, decodeEnv(t, resp2), "AUTH_003")

	// 合法 CSRF → 通过（SITE_002：空 server 块无碍 disable 寻址成功）。
	env := post(t, app, admin, "/sites/site/disable", map[string]string{"siteId": siteID})
	wantCode(t, env, "0")
}

// admin 全链路：扫描→建站→编辑→反代→源码→启停→reload→证书→审计。
func TestAdminFullFlow(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	ro := loginAndCreateReadonly(t, app, admin)
	ctx := context.Background()

	// 列表（空态：nginxAvailable=true）。
	env := get(t, app, admin, "/sites/site/list")
	wantCode(t, env, "0")
	var listData struct {
		NginxAvailable bool  `json:"nginxAvailable"`
		Items          []any `json:"items"`
	}
	json.Unmarshal(env.Data, &listData)
	if !listData.NginxAvailable || listData.Items == nil {
		t.Fatalf("empty list: %+v", listData)
	}

	// 建站（反代模板）。
	env = post(t, app, admin, "/sites/site/create", map[string]any{
		"type": "reverse_proxy", "serverNames": []string{"flow.example.com"},
		"listen": "80", "upstream": "http://127.0.0.1:6000",
	})
	wantCode(t, env, "0")
	var created struct {
		SiteID string `json:"siteId"`
	}
	json.Unmarshal(env.Data, &created)
	if created.SiteID == "" {
		t.Fatalf("create data: %s", env.Data)
	}

	// php_fpm 无 root 建站（Gate 2 F-1 回归锁：契约 §4.2.4 root?(type=static
	// 必填)——php_fpm 不带 root 经完整 HTTP 链路必须成功）。
	env = post(t, app, admin, "/sites/site/create", map[string]any{
		"type": "php_fpm", "serverNames": []string{"phpflow.example.com"},
		"listen": "8080", "poolName": "www",
	})
	wantCode(t, env, "0")

	// 详情（locations 投影）。
	env = get(t, app, admin, "/sites/site/detail?siteId="+confEscape(created.SiteID))
	wantCode(t, env, "0")
	var detail struct {
		Proxy     *struct{ Upstream string } `json:"proxy"`
		Locations []map[string]any           `json:"locations"`
		RawBlocks []map[string]any           `json:"rawBlocks"`
	}
	json.Unmarshal(env.Data, &detail)
	if detail.Proxy == nil || detail.Proxy.Upstream != "http://127.0.0.1:6000" || len(detail.Locations) == 0 {
		t.Fatalf("detail: %s", env.Data)
	}

	// 结构化编辑（M3 主场景：反代地址改写）。
	env = post(t, app, admin, "/sites/proxy/update", map[string]any{
		"siteId": created.SiteID, "upstream": "http://10.1.1.1:9000", "websocket": true,
	})
	wantCode(t, env, "0")

	// 源码读取 + 保存（乐观锁）。
	env = get(t, app, admin, "/sites/site/source?siteId="+confEscape(created.SiteID))
	wantCode(t, env, "0")
	var src struct {
		Content   string `json:"content"`
		BaseMtime string `json:"baseMtime"`
	}
	json.Unmarshal(env.Data, &src)
	if !strings.Contains(src.Content, "proxy_pass http://10.1.1.1:9000;") {
		t.Fatalf("proxy edit not visible in source: %s", src.Content)
	}
	env = post(t, app, admin, "/sites/site/source-save", map[string]string{
		"siteId": created.SiteID, "content": src.Content, "baseMtime": src.BaseMtime,
	})
	wantCode(t, env, "0")

	// 启停。
	env = post(t, app, admin, "/sites/site/disable", map[string]string{"siteId": created.SiteID})
	wantCode(t, env, "0")
	env = post(t, app, admin, "/sites/site/enable", map[string]string{"siteId": created.SiteID})
	wantCode(t, env, "0")

	// reload。
	env = post(t, app, admin, "/sites/nginx/reload", map[string]string{})
	wantCode(t, env, "0")

	// 证书：request（同步终态，fake ACME）→ list → deploy → undeploy → delete。
	env = post(t, app, admin, "/sites/cert/request", map[string]string{
		"siteId": created.SiteID, "challenge": "http_01",
	})
	wantCode(t, env, "0")
	var cert struct {
		CertID int64 `json:"certId"`
	}
	json.Unmarshal(env.Data, &cert)
	if cert.CertID == 0 {
		t.Fatalf("cert request data: %s", env.Data)
	}

	env = get(t, app, ro, "/sites/cert/list") // readonly 亦可看（ADR-018）
	wantCode(t, env, "0")
	var certList struct {
		Items []map[string]any `json:"items"`
	}
	json.Unmarshal(env.Data, &certList)
	if len(certList.Items) == 0 || certList.Items[0]["status"] != "valid" {
		t.Fatalf("cert list: %s", env.Data)
	}

	env = post(t, app, admin, "/sites/cert/deploy", map[string]any{"certId": cert.CertID, "siteId": created.SiteID})
	wantCode(t, env, "0")
	env = post(t, app, admin, "/sites/cert/undeploy", map[string]any{"certId": cert.CertID, "siteId": created.SiteID})
	wantCode(t, env, "0")
	env = post(t, app, admin, "/sites/cert/delete", map[string]any{"certId": cert.CertID})
	wantCode(t, env, "0")

	// 校验链抽样：SITE_005 防重 / SITE_001 形态 / CERT_007。
	env = post(t, app, admin, "/sites/site/create", map[string]any{
		"type": "reverse_proxy", "serverNames": []string{"flow.example.com"},
		"listen": "80", "upstream": "http://127.0.0.1:6000"})
	wantCode(t, env, "SITE_005")
	env = get(t, app, admin, "/sites/site/detail?siteId=malformed")
	wantCode(t, env, "SITE_001")
	env = post(t, app, admin, "/sites/cert/request", map[string]string{
		"siteId": created.SiteID, "challenge": "dns_01"})
	wantCode(t, env, "CERT_007")

	// SITE_003 管线回滚：注入失败校验 → 配置自动回滚（R-06）。
	confPath := filepath.Join(app.gw.root, "sites-enabled", "dougong-flow.example.com.conf")
	before, _ := os.ReadFile(confPath)
	app.gw.testOK = false
	env = post(t, app, admin, "/sites/site/update", map[string]any{
		"siteId": created.SiteID, "serverNames": []string{"changed.example.com"}, "listen": []string{"80"}})
	wantCode(t, env, "SITE_003")
	after, _ := os.ReadFile(confPath)
	if !bytes.Equal(before, after) {
		t.Fatal("config must be rolled back after nginx -t failure (R-06)")
	}
	app.gw.testOK = true

	// php-pools（admin）。
	env = get(t, app, admin, "/sites/php-pools/list")
	wantCode(t, env, "0")
	var pools struct {
		Pools []map[string]string `json:"pools"`
	}
	json.Unmarshal(env.Data, &pools)
	if len(pools.Pools) != 1 || pools.Pools[0]["socket"] != "unix:/run/php/www.sock" {
		t.Fatalf("pools: %s", env.Data)
	}
	_ = ctx
}

// confEscape 对 siteId 做 query 安全编码（仅传输层：# 与反斜杠与空格百分号
// 编码，其余原样——解码后与扫描实况逐字节一致，D-02）。
func confEscape(siteID string) string {
	var sb strings.Builder
	for _, r := range siteID {
		switch r {
		case '#':
			sb.WriteString("%23")
		case '\\':
			sb.WriteString("%5C")
		case ' ':
			sb.WriteString("%20")
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}
