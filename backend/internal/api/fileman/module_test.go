package fileman

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/adapter/fslocal"
	"github.com/dougong/dougong/internal/api"
	authapi "github.com/dougong/dougong/internal/api/auth"
	auditsvc "github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	filemansvc "github.com/dougong/dougong/internal/service/fileman"
	"github.com/dougong/dougong/internal/store"
	storeauth "github.com/dougong/dougong/internal/store/auth"
	storefileman "github.com/dougong/dougong/internal/store/fileman"
)

const testCred = "dougong-test-only"

type envelope struct {
	Code    any             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func codeStr(env envelope) string { return fmt.Sprint(env.Code) }

func wantCode(t *testing.T, env envelope, want string) {
	t.Helper()
	if got := codeStr(env); got != want {
		t.Fatalf("code = %s, want %s (message=%s)", got, want, env.Message)
	}
}

// testApp 聚合全栈测试依赖（真实 :memory: SQLite + 真实临时目录文件系统，
// 不 mock；镜像 main 装配）。
type testApp struct {
	ts   *httptest.Server
	root string
}

func newTestApp(t *testing.T) *testApp {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := storeauth.New(db)
	if err := repo.Init().EnsureRow(ctx); err != nil {
		t.Fatalf("ensure init row: %v", err)
	}
	st := storefileman.New(db)
	if err := st.Scope().EnsureRow(ctx, time.Now()); err != nil {
		t.Fatalf("ensure scope row: %v", err)
	}
	root := t.TempDir()
	guard := filemansvc.NewGuard(filemansvc.ScopeConfig{RootPath: root})
	hasher, err := authsvc.NewBcryptHasher(10)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	authSvcObj := authsvc.New(repo, hasher, authsvc.NewTotpService(nil), authsvc.DefaultSessionPolicy())
	auditSvc := auditsvc.New(repo.Audit(), time.Now)
	fileSvc := filemansvc.New(fslocal.New(), st, auditSvc, guard)
	server := api.NewServer()
	server.RegisterModules(
		authapi.NewModule(authSvcObj, auditSvc, false),
		NewModule(authSvcObj, fileSvc),
	)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	return &testApp{ts: ts, root: root}
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
	body := map[string]string{"username": "admin", "password": testCred}
	b, _ := json.Marshal(body)
	resp, err := http.Post(app.ts.URL+"/auth/init/wizard", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("wizard: %v", err)
	}
	env := decodeEnv(t, resp)
	wantCode(t, env, "0")
	var data struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("wizard data: %v", err)
	}
	for _, c := range resp.Cookies() {
		if c.Name == authsvc.SessionCookieName {
			return session{cookie: c.Value, csrf: data.CSRFToken}
		}
	}
	t.Fatal("wizard response missing session cookie")
	return session{}
}

// createReadonlyAndLogin 由 admin 创建 readonly 用户并登录（ADR-018 边界用）。
func createReadonlyAndLogin(t *testing.T, app *testApp, admin session) session {
	t.Helper()
	body := map[string]any{"username": "watcher", "password": testCred, "role": "readonly"}
	b, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", app.ts.URL+"/auth/user/create", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", authsvc.SessionCookieName+"="+admin.cookie)
	req.Header.Set(authsvc.CSRFHeader, admin.csrf)
	req.Header.Set("Origin", app.ts.URL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create readonly: %v", err)
	}
	wantCode(t, decodeEnv(t, resp), "0")

	lb, _ := json.Marshal(map[string]string{"username": "watcher", "password": testCred})
	lresp, err := http.Post(app.ts.URL+"/auth/session/login", "application/json", bytes.NewReader(lb))
	if err != nil {
		t.Fatalf("login readonly: %v", err)
	}
	lenv := decodeEnv(t, lresp)
	wantCode(t, lenv, "0")
	var data struct {
		CSRFToken string `json:"csrfToken"`
	}
	_ = json.Unmarshal(lenv.Data, &data)
	for _, c := range lresp.Cookies() {
		if c.Name == authsvc.SessionCookieName {
			return session{cookie: c.Value, csrf: data.CSRFToken}
		}
	}
	t.Fatal("readonly login missing cookie")
	return session{}
}

// doJSON 发 JSON 请求（带会话与 CSRF/Origin）。
func (a *testApp) doJSON(t *testing.T, s session, method, path string, body any) envelope {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, a.ts.URL+path, rd)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", authsvc.SessionCookieName+"="+s.cookie)
	if method != "GET" {
		req.Header.Set(authsvc.CSRFHeader, s.csrf)
		req.Header.Set("Origin", a.ts.URL)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, path, err)
	}
	return decodeEnv(t, resp)
}

func TestNoSessionRejected(t *testing.T) {
	app := newTestApp(t)
	initWizardAndLogin(t, app) // 先过 InitGuard（I-01：未初始化时拦截一切）
	resp, err := http.Get(app.ts.URL + "/files/entry/list?path=" + app.root)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	wantCode(t, decodeEnv(t, resp), "AUTH_001")
}

func TestRoleBoundaries(t *testing.T) { // ADR-018：查看两角色 / 配置与写 admin
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	ro := createReadonlyAndLogin(t, app, admin)

	// 两角色可看目录与回收站。
	env := app.doJSON(t, ro, "GET", "/files/entry/list?path="+app.root, nil)
	wantCode(t, env, "0")
	env = app.doJSON(t, ro, "GET", "/files/trash/list", nil)
	wantCode(t, env, "0")

	// Gate 2 E1 回归（A-F20）：首屏不带 path（缺省=受管根）两角色均可用——
	// readonly 无 path-scope/get 权，受管根只能由服务端缺省解析。
	env = app.doJSON(t, ro, "GET", "/files/entry/list", nil)
	wantCode(t, env, "0")
	env = app.doJSON(t, admin, "GET", "/files/entry/list", nil)
	wantCode(t, env, "0")

	// readonly：白名单配置/写/下载 → AUTH_008；缺 CSRF（admin）→ AUTH_003。
	env = app.doJSON(t, ro, "GET", "/files/path-scope/get", nil)
	wantCode(t, env, "AUTH_008")
	env = app.doJSON(t, ro, "POST", "/files/entry/create",
		map[string]string{"dir": app.root, "name": "x.txt", "type": "file"})
	wantCode(t, env, "AUTH_008")
	env = app.doJSON(t, ro, "GET", "/files/entry/download?path="+app.root, nil)
	wantCode(t, env, "AUTH_008")

	req, _ := http.NewRequest("POST", app.ts.URL+"/files/entry/create",
		bytes.NewReader([]byte(`{"dir":"`+app.root+`","name":"x.txt","type":"file"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", authsvc.SessionCookieName+"="+admin.cookie)
	req.Header.Set("Origin", app.ts.URL) // 有 Origin 无 CSRF 头
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	wantCode(t, decodeEnv(t, resp), "AUTH_003")
}

func TestFileLifecycleOverHTTP(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)

	// create → list 可见。
	env := app.doJSON(t, admin, "POST", "/files/entry/create",
		map[string]string{"dir": app.root, "name": "hello.txt", "type": "file"})
	wantCode(t, env, "0")
	env = app.doJSON(t, admin, "GET", "/files/entry/list?path="+app.root, nil)
	wantCode(t, env, "0")
	var listData struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(env.Data, &listData); err != nil {
		t.Fatalf("list data: %v", err)
	}
	if listData.Total != 1 || listData.Items[0].Name != "hello.txt" {
		t.Fatalf("list = %+v", listData)
	}

	// 越界路径 → FILE_002（越界原因在 data.reason）。
	env = app.doJSON(t, admin, "GET", "/files/entry/list?path="+filepath.Join(app.root, "..", "elsewhere"), nil)
	wantCode(t, env, "FILE_002")
	if !strings.Contains(string(env.Data), "not_under_root") &&
		!strings.Contains(string(env.Data), "excluded") {
		t.Fatalf("reason data = %s", env.Data)
	}

	// content-save → content 回读。
	p := filepath.Join(app.root, "hello.txt")
	env = app.doJSON(t, admin, "POST", "/files/entry/content-save",
		map[string]string{"path": p, "content": "panel-content"})
	wantCode(t, env, "0")
	env = app.doJSON(t, admin, "GET", "/files/entry/content?path="+p, nil)
	wantCode(t, env, "0")
	if !strings.Contains(string(env.Data), "panel-content") {
		t.Fatalf("content = %s", env.Data)
	}

	// download：200 + attachment 头 + 原文（D-16 校验先行开流）。
	req, _ := http.NewRequest("GET", app.ts.URL+"/files/entry/download?path="+p, nil)
	req.Header.Set("Cookie", authsvc.SessionCookieName+"="+admin.cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 || string(body) != "panel-content" {
		t.Fatalf("download = %d %q", resp.StatusCode, body)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") ||
		!strings.Contains(cd, "hello.txt") {
		t.Fatalf("content-disposition = %q", cd)
	}

	// delete 入站 → trash/list 可见 → purge。
	env = app.doJSON(t, admin, "POST", "/files/entry/delete", map[string]string{"path": p})
	wantCode(t, env, "0")
	env = app.doJSON(t, admin, "GET", "/files/trash/list", nil)
	wantCode(t, env, "0")
	if !strings.Contains(string(env.Data), "hello.txt") {
		t.Fatalf("trash list = %s", env.Data)
	}
	env = app.doJSON(t, admin, "POST", "/files/trash/purge", nil)
	wantCode(t, env, "0")
	if !strings.Contains(string(env.Data), `"purgedCount":1`) {
		t.Fatalf("purge data = %s", env.Data)
	}
}

func TestUploadMultipartOverHTTP(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("dir", app.root)
	_ = mw.WriteField("onConflict", "ask")
	fw, _ := mw.CreateFormFile("files", "m1.txt")
	_, _ = fw.Write([]byte("multipart-one"))
	_ = mw.Close()

	req, _ := http.NewRequest("POST", app.ts.URL+"/files/entry/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Cookie", authsvc.SessionCookieName+"="+admin.cookie)
	req.Header.Set(authsvc.CSRFHeader, admin.csrf)
	req.Header.Set("Origin", app.ts.URL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	env := decodeEnv(t, resp)
	wantCode(t, env, "0")
	if !strings.Contains(string(env.Data), `"uploadedCount":1`) {
		t.Fatalf("upload data = %s", env.Data)
	}
	if b, _ := os.ReadFile(filepath.Join(app.root, "m1.txt")); string(b) != "multipart-one" {
		t.Fatalf("uploaded = %q", b)
	}
}

func TestChunkedUploadOverHTTP(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	total := filemansvc.UploadChunkSize + 5

	env := app.doJSON(t, admin, "POST", "/files/upload/chunk-init", map[string]any{
		"dir": app.root, "fileName": "chunked.bin",
		"totalSize": total, "totalChunks": 2,
	})
	wantCode(t, env, "0")
	var init struct {
		UploadID string `json:"uploadId"`
	}
	if err := json.Unmarshal(env.Data, &init); err != nil || init.UploadID == "" {
		t.Fatalf("init data = %s (%v)", env.Data, err)
	}

	send := func(index int, data []byte) envelope {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		_ = mw.WriteField("uploadId", init.UploadID)
		_ = mw.WriteField("index", fmt.Sprint(index))
		fw, _ := mw.CreateFormFile("chunk", "part")
		_, _ = fw.Write(data)
		_ = mw.Close()
		req, _ := http.NewRequest("POST", app.ts.URL+"/files/upload/chunk", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("Cookie", authsvc.SessionCookieName+"="+admin.cookie)
		req.Header.Set(authsvc.CSRFHeader, admin.csrf)
		req.Header.Set("Origin", app.ts.URL)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("chunk %d: %v", index, err)
		}
		return decodeEnv(t, resp)
	}
	wantCode(t, send(0, bytesOf('a', filemansvc.UploadChunkSize)), "0")
	wantCode(t, send(1, bytesOf('b', 5)), "0")

	env = app.doJSON(t, admin, "POST", "/files/upload/chunk-complete",
		map[string]string{"uploadId": init.UploadID})
	wantCode(t, env, "0")
	got, _ := os.ReadFile(filepath.Join(app.root, "chunked.bin"))
	if len(got) != total || got[0] != 'a' || got[filemansvc.UploadChunkSize] != 'b' {
		t.Fatalf("merged = %d bytes", len(got))
	}
}

func bytesOf(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

func TestPathScopeUpdateTakesEffect(t *testing.T) { // D-02：修改即予生效
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)

	env := app.doJSON(t, admin, "GET", "/files/path-scope/get", nil)
	wantCode(t, env, "0")
	if !strings.Contains(string(env.Data), `"rootPath":"`) {
		t.Fatalf("scope get = %s", env.Data)
	}

	sub := filepath.Join(app.root, "scoped")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	env = app.doJSON(t, admin, "POST", "/files/path-scope/update", map[string]any{
		"rootPath": sub, "excludedDirs": []string{},
	})
	wantCode(t, env, "0")

	// 新根下可用；旧根内（新根外）条目立即越界（D-02 副本生效）。
	writeFileT(t, filepath.Join(sub, "inside.txt"), "ok")
	writeFileT(t, filepath.Join(app.root, "legacy.txt"), "old")
	env = app.doJSON(t, admin, "GET", "/files/entry/list?path="+sub, nil)
	wantCode(t, env, "0")
	env = app.doJSON(t, admin, "GET", "/files/entry/download?path="+filepath.Join(app.root, "legacy.txt"), nil)
	wantCode(t, env, "FILE_002")
	if !strings.Contains(string(env.Data), "not_under_root") {
		t.Fatalf("rejection reason = %s", env.Data)
	}
	// trash 目录跟随新根（还原/删除链路不受影响——行为级由 service 测试覆盖）。
	env = app.doJSON(t, admin, "POST", "/files/entry/delete",
		map[string]string{"path": filepath.Join(sub, "inside.txt")})
	wantCode(t, env, "0")
}

func writeFileT(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
