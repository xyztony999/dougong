package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/api"
	"github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	"github.com/dougong/dougong/internal/store"
	storeauth "github.com/dougong/dougong/internal/store/auth"
)

// 测试口令占位（程序内生成语义的固定串，非真实凭据；仅用于 :memory: 测试库，
// 满足 ≥8 位服务端校验）。真实口令一律经环境/配置注入。
const (
	testCred      = "dougong-test-only" // 初始口令
	testCredAlt   = "dougong-test-alt2" // 改密后口令
	testCredReset = "dougong-test-rst3" // 重置口令
)

// envelope 是统一响应包络（code/message/data）。code：成功=数值 0（JSON 解码
// 为 float64）；失败=符号串（"AUTH_001" 等，契约 §3.4——Gate 2 E1 口径）。
type envelope struct {
	Code    any             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// codeStr 以字符串视角取 code（模拟前端 http.ts 的 String(env.code) 通道）。
func codeStr(env envelope) string { return fmt.Sprint(env.Code) }

func wantCode(t *testing.T, env envelope, want string) {
	t.Helper()
	if got := codeStr(env); got != want {
		t.Fatalf("code = %s, want %s (message=%s)", got, want, env.Message)
	}
}

// newTestServer 以真实 :memory: SQLite 装配完整 HTTP 栈（含中间件链）。
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if cerr := db.Close(); cerr != nil {
			t.Errorf("close db: %v", cerr)
		}
	})
	ctx := context.Background()
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := storeauth.New(db)
	if err := repo.Init().EnsureRow(ctx); err != nil {
		t.Fatalf("ensure init row: %v", err)
	}
	hasher, err := authsvc.NewBcryptHasher(10)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	svc := authsvc.New(repo, hasher, authsvc.NewTotpService(nil), authsvc.DefaultSessionPolicy())
	auditSvc := audit.New(repo.Audit(), time.Now)
	server := api.NewServer()
	server.RegisterModules(NewModule(svc, auditSvc, false))
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	return ts
}

// do 执行 JSON 请求并解析包络。
func do(t *testing.T, ts *httptest.Server, method, path string, body any, headers map[string]string) (envelope, *http.Response) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, ts.URL+path, rd)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode envelope %s %s: %v (body=%s)", method, path, err, raw)
	}
	return env, resp
}

// sessionHeaders 携带 Cookie/CSRF/同源 Origin 的请求头集合。
func sessionHeaders(ts *httptest.Server, cookie, csrf string) map[string]string {
	return map[string]string{
		"Cookie":           authsvc.SessionCookieName + "=" + cookie,
		authsvc.CSRFHeader: csrf,
		"Origin":           ts.URL,
	}
}

func cookieOnly(cookie string) map[string]string {
	return map[string]string{"Cookie": authsvc.SessionCookieName + "=" + cookie}
}

func loginBody(username string) map[string]string {
	return map[string]string{"username": username, "password": testCred}
}

func TestFullFlowOverHTTP(t *testing.T) {
	ts := newTestServer(t)

	// 1. 初始化前探测：init/check 可达（InitGuard 豁免）。
	env, _ := do(t, ts, "GET", "/auth/init/check", nil, nil)
	wantCode(t, env, "0")
	var st struct {
		Initialized bool `json:"initialized"`
		AllowWizard bool `json:"allowWizard"`
	}
	if err := json.Unmarshal(env.Data, &st); err != nil || st.Initialized || !st.AllowWizard {
		t.Fatalf("init/check data=%s", env.Data)
	}

	// 2. I-01：初始化前登录被拦（AUTH_009）。
	env, _ = do(t, ts, "POST", "/auth/session/login", loginBody("admin"), nil)
	wantCode(t, env, "AUTH_009")

	// 3. 向导初始化：200 + code 0 + Set-Cookie + csrfToken。
	env, resp := do(t, ts, "POST", "/auth/init/wizard", loginBody("admin"), nil)
	wantCode(t, env, "0")
	var wiz struct {
		CsrfToken string `json:"csrfToken"`
		User      struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
			Role     string `json:"role"`
		} `json:"user"`
	}
	if err := json.Unmarshal(env.Data, &wiz); err != nil || wiz.CsrfToken == "" || wiz.User.Role != "admin" {
		t.Fatalf("wizard data=%s", env.Data)
	}
	adminCookie := extractCookie(t, resp, authsvc.SessionCookieName)

	// 4. I-02：重复向导 AUTH_009。
	env, _ = do(t, ts, "POST", "/auth/init/wizard", loginBody("hacker"), nil)
	wantCode(t, env, "AUTH_009")

	// 5. 当前会话（Cookie）。
	env, _ = do(t, ts, "GET", "/auth/session/current", nil, cookieOnly(adminCookie))
	wantCode(t, env, "0")
	if !strings.Contains(string(env.Data), `"authenticated":true`) {
		t.Fatalf("current data=%s", env.Data)
	}

	// 6. 无会话访问业务接口 → AUTH_001。
	env, _ = do(t, ts, "GET", "/auth/user/list", nil, nil)
	wantCode(t, env, "AUTH_001")

	// 7. CSRF：写操作缺头 → AUTH_003。
	env, _ = do(t, ts, "POST", "/auth/user/create",
		map[string]string{"username": "viewer", "password": testCred, "role": "readonly"},
		map[string]string{"Cookie": authsvc.SessionCookieName + "=" + adminCookie, "Origin": ts.URL})
	wantCode(t, env, "AUTH_003")

	// 8. 正常创建 readonly（CSRF+同源通过）。
	env, _ = do(t, ts, "POST", "/auth/user/create",
		map[string]string{"username": "viewer", "password": testCred, "role": "readonly"},
		sessionHeaders(ts, adminCookie, wiz.CsrfToken))
	wantCode(t, env, "0")

	// 9. viewer 登录并尝试越界写 → AUTH_008（ADR-018）。
	env, vresp := do(t, ts, "POST", "/auth/session/login", loginBody("viewer"), nil)
	wantCode(t, env, "0")
	var vlogin struct {
		CsrfToken string `json:"csrfToken"`
	}
	if err := json.Unmarshal(env.Data, &vlogin); err != nil {
		t.Fatalf("viewer login data: %v", err)
	}
	viewerCookie := extractCookie(t, vresp, authsvc.SessionCookieName)
	env, _ = do(t, ts, "POST", "/auth/user/create",
		map[string]string{"username": "x2", "password": testCred, "role": "admin"},
		sessionHeaders(ts, viewerCookie, vlogin.CsrfToken))
	wantCode(t, env, "AUTH_008")
	// readonly 可看审计清单（ADR-018 含审计），键名须为 camelCase（契约 §4.3.4）。
	env, _ = do(t, ts, "GET", "/auth/audit-log/list", nil, cookieOnly(viewerCookie))
	wantCode(t, env, "0")
	assertAuditItemsCamelCase(t, env.Data)
	// readonly 不可导出（AUTH_008）。
	env, _ = do(t, ts, "GET", "/auth/audit-log/export", nil, cookieOnly(viewerCookie))
	wantCode(t, env, "AUTH_008")

	// 10. admin 导出 CSV：BOM + 表头 + attachment。
	req, err := http.NewRequest("GET", ts.URL+"/auth/audit-log/export", nil)
	if err != nil {
		t.Fatalf("new export req: %v", err)
	}
	req.Header.Set("Cookie", authsvc.SessionCookieName+"="+adminCookie)
	cresp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	defer cresp.Body.Close()
	body, _ := io.ReadAll(cresp.Body)
	if cresp.StatusCode != 200 || !strings.HasPrefix(string(body), "\xEF\xBB\xBF") {
		t.Fatalf("export status=%d BOM missing", cresp.StatusCode)
	}
	if !strings.Contains(string(body), "id,createdAt,actorName") {
		t.Fatal("export header row missing")
	}
	if !strings.Contains(cresp.Header.Get("Content-Disposition"), "attachment") {
		t.Fatal("export Content-Disposition missing")
	}

	// 11. 登出：Cookie 清除 + 旧会话立即失效（I-13）。
	env, _ = do(t, ts, "POST", "/auth/session/logout", nil, sessionHeaders(ts, adminCookie, wiz.CsrfToken))
	wantCode(t, env, "0")
	env, _ = do(t, ts, "GET", "/auth/session/current", nil, cookieOnly(adminCookie))
	wantCode(t, env, "AUTH_001")

	// 12. 会话清单（admin 重新登录）。
	env, resp = do(t, ts, "POST", "/auth/session/login", loginBody("admin"), nil)
	wantCode(t, env, "0")
	c2 := extractCookie(t, resp, authsvc.SessionCookieName)
	env, _ = do(t, ts, "GET", "/auth/session/list", nil, cookieOnly(c2))
	wantCode(t, env, "0")
	var sessList struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &sessList); err != nil || len(sessList.Items) == 0 {
		t.Fatalf("session list data=%s", env.Data)
	}
	first := sessList.Items[0]
	for _, key := range []string{"id", "username", "createdAt", "lastActiveAt", "sourceIp", "current"} {
		if _, ok := first[key]; !ok {
			t.Fatalf("session item missing key %q: %v", key, first)
		}
	}
}

// assertAuditItemsCamelCase 按前端 types/auth.ts AuditLogItem 契约逐键断言，
// 并确保无 PascalCase 泄漏（Gate 2 E2 回归防线）。
func assertAuditItemsCamelCase(t *testing.T, raw json.RawMessage) {
	t.Helper()
	var page struct {
		Items []map[string]json.RawMessage `json:"items"`
		Total int64                        `json:"total"`
	}
	if err := json.Unmarshal(raw, &page); err != nil {
		t.Fatalf("audit page decode: %v (data=%s)", err, raw)
	}
	if len(page.Items) == 0 {
		t.Fatal("audit items empty (向导/登录已产生审计，应非空)")
	}
	wantKeys := []string{"id", "actorName", "category", "action", "objectType",
		"objectId", "detail", "result", "source", "createdAt"}
	banKeys := []string{"ID", "ActorName", "ActorID", "Category", "Action",
		"ObjectType", "ObjectID", "Detail", "Result", "Source", "CreatedAt", "actorId"}
	for i, item := range page.Items {
		for _, k := range wantKeys {
			if _, ok := item[k]; !ok {
				t.Fatalf("audit item[%d] missing camelCase key %q: %v", i, k, item)
			}
		}
		for _, k := range banKeys {
			if _, ok := item[k]; ok {
				t.Fatalf("audit item[%d] has non-contract key %q", i, k)
			}
		}
	}
}

// TestAdminWriteFlowOverHTTP 覆盖 admin 写接口与 TOTP 全流程（HTTP 层）：
// user/list、totp-setup/enable/disable、password-change、reset-password、
// session/revoke（含自吊销守卫）、user/delete（含 D-09 守卫）、AUTH_007 二步登录。
func TestAdminWriteFlowOverHTTP(t *testing.T) {
	ts := newTestServer(t)

	// 向导建 admin 并登录。
	env, resp := do(t, ts, "POST", "/auth/init/wizard", loginBody("admin"), nil)
	wantCode(t, env, "0")
	var wiz struct {
		CsrfToken string `json:"csrfToken"`
	}
	if err := json.Unmarshal(env.Data, &wiz); err != nil {
		t.Fatalf("wizard data: %v", err)
	}
	cookie := extractCookie(t, resp, authsvc.SessionCookieName)
	h := sessionHeaders(ts, cookie, wiz.CsrfToken)

	// 用户清单（脱敏键 camelCase）。
	env, _ = do(t, ts, "GET", "/auth/user/list", nil, cookieOnly(cookie))
	wantCode(t, env, "0")
	var ul struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &ul); err != nil || len(ul.Items) != 1 {
		t.Fatalf("user list data=%s", env.Data)
	}
	for _, k := range []string{"id", "username", "role", "status", "totpEnabled", "lockedUntil", "createdAt"} {
		if _, ok := ul.Items[0][k]; !ok {
			t.Fatalf("user item missing key %q: %v", k, ul.Items[0])
		}
	}

	// TOTP 绑定：setup →（错码 enable → AUTH_006）→ 正码 enable。
	env, _ = do(t, ts, "POST", "/auth/user/totp-setup", nil, h)
	wantCode(t, env, "0")
	var setup struct {
		OtpauthURI string `json:"otpauthUri"`
		Secret     string `json:"secret"`
	}
	if err := json.Unmarshal(env.Data, &setup); err != nil || setup.Secret == "" ||
		!strings.HasPrefix(setup.OtpauthURI, "otpauth://totp/") {
		t.Fatalf("totp setup data=%s", env.Data)
	}
	env, _ = do(t, ts, "POST", "/auth/user/totp-enable",
		map[string]string{"code": "000000"}, h)
	wantCode(t, env, "AUTH_006")
	code, err := authsvc.GenerateCode(setup.Secret, time.Now())
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}
	env, _ = do(t, ts, "POST", "/auth/user/totp-enable",
		map[string]string{"code": code}, h)
	wantCode(t, env, "0")

	// AUTH_007 二步登录：login → AUTH_007+loginTicket → 错码 AUTH_006 → 正码换会话。
	env, _ = do(t, ts, "POST", "/auth/session/login", loginBody("admin"), nil)
	wantCode(t, env, "AUTH_007")
	var twoStep struct {
		LoginTicket string `json:"loginTicket"`
	}
	if err := json.Unmarshal(env.Data, &twoStep); err != nil || twoStep.LoginTicket == "" {
		t.Fatalf("AUTH_007 data=%s (want loginTicket)", env.Data)
	}
	env, _ = do(t, ts, "POST", "/auth/session/totp-verify",
		map[string]string{"loginTicket": twoStep.LoginTicket, "code": "000000"}, nil)
	wantCode(t, env, "AUTH_006")
	code2, err := authsvc.GenerateCode(setup.Secret, time.Now())
	if err != nil {
		t.Fatalf("generate code 2: %v", err)
	}
	env, tres := do(t, ts, "POST", "/auth/session/totp-verify",
		map[string]string{"loginTicket": twoStep.LoginTicket, "code": code2}, nil)
	wantCode(t, env, "0")
	secondCookie := extractCookie(t, tres, authsvc.SessionCookieName)

	// 会话吊销守卫与强制下线：以向导会话（h）为 actor——
	// otherID（secondCookie 视角非 current）= 向导会话自身 → AUTH_012；
	// selfID（secondCookie 视角 current）= TOTP 二步会话 → 吊销成功。
	env, _ = do(t, ts, "GET", "/auth/session/list", nil, cookieOnly(secondCookie))
	wantCode(t, env, "0")
	var sessList struct {
		Items []struct {
			ID      string `json:"id"`
			Current bool   `json:"current"`
		} `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &sessList); err != nil || len(sessList.Items) < 2 {
		t.Fatalf("session list data=%s", env.Data)
	}
	var totpSessID, wizardSessID string
	for _, it := range sessList.Items {
		if it.Current {
			totpSessID = it.ID
		} else {
			wizardSessID = it.ID
		}
	}
	env, _ = do(t, ts, "POST", "/auth/session/revoke",
		map[string]string{"sessionId": wizardSessID}, h)
	wantCode(t, env, "AUTH_012")
	env, _ = do(t, ts, "POST", "/auth/session/revoke",
		map[string]string{"sessionId": totpSessID}, h)
	wantCode(t, env, "0")
	// 被强制的会话立即失效（I-13）。
	env, _ = do(t, ts, "GET", "/auth/session/current", nil, cookieOnly(secondCookie))
	wantCode(t, env, "AUTH_001")

	// 自助改密（成功后当前会话保留——继续用同头请求验证）。
	env, _ = do(t, ts, "POST", "/auth/user/password-change",
		map[string]string{"oldPassword": testCred, "newPassword": testCredAlt}, h)
	wantCode(t, env, "0")
	env, _ = do(t, ts, "GET", "/auth/session/current", nil, cookieOnly(cookie))
	wantCode(t, env, "0")

	// TOTP 解绑（改密后旧口令应 AUTH_005，新口令成功）。
	env, _ = do(t, ts, "POST", "/auth/user/totp-disable",
		map[string]string{"password": testCred}, h)
	wantCode(t, env, "AUTH_005")
	env, _ = do(t, ts, "POST", "/auth/user/totp-disable",
		map[string]string{"password": testCredAlt}, h)
	wantCode(t, env, "0")

	// 建 readonly → 重置其密码 → 删除（级联）→ 删自身 AUTH_014。
	env, _ = do(t, ts, "POST", "/auth/user/create",
		map[string]string{"username": "temp", "password": testCred, "role": "readonly"}, h)
	wantCode(t, env, "0")
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatalf("create data: %v", err)
	}
	env, _ = do(t, ts, "POST", "/auth/user/reset-password",
		map[string]any{"userId": created.ID, "newPassword": testCredReset}, h)
	wantCode(t, env, "0")
	env, _ = do(t, ts, "POST", "/auth/user/delete",
		map[string]any{"userId": created.ID}, h)
	wantCode(t, env, "0")
	env, _ = do(t, ts, "POST", "/auth/user/delete",
		map[string]any{"userId": 1}, h)
	wantCode(t, env, "AUTH_014")

	// 审计清单过滤：action=user_created 应命中 ≥1。
	env, _ = do(t, ts, "GET", "/auth/audit-log/list?action=user_created&page=1&pageSize=10", nil, cookieOnly(cookie))
	wantCode(t, env, "0")
	var filtered struct {
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal(env.Data, &filtered); err != nil || filtered.Total < 1 {
		t.Fatalf("audit filter data=%s", env.Data)
	}
}

func extractCookie(t *testing.T, resp *http.Response, name string) string {
	t.Helper()
	for _, c := range resp.Cookies() {
		if c.Name == name {
			return c.Value
		}
	}
	t.Fatalf("cookie %s not set", name)
	return ""
}

func TestBindValidationOverHTTP(t *testing.T) {
	ts := newTestServer(t)
	// 畸形 JSON → AUTH_012（wizard 为 InitGuard 豁免路径，可达绑定层）。
	req, _ := http.NewRequest("POST", ts.URL+"/auth/init/wizard", strings.NewReader("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	wantCode(t, env, "AUTH_012")
	// 初始化前缺字段登录：InitGuard 先拦（AUTH_009，中间件链序先于绑定）。
	env, _ = do(t, ts, "POST", "/auth/session/login", map[string]string{"username": "a"}, nil)
	wantCode(t, env, "AUTH_009")
}
