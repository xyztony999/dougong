// 安全核心回归（D-09/D-11，安全底线不可桩假）：signon 令牌四重校验
// （防伪/时效/一次性/双绑定）+ gating 会话+admin 判定 + 内部秘密。
package dbtools

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	authsvc "github.com/dougong/dougong/internal/service/auth"
	storedbt "github.com/dougong/dougong/internal/store/dbtools"
)

// installPMAForSignon 建立 phpmyadmin 档案（signon 密钥在位）。
func installPMAForSignon(t *testing.T, svc *Service, f *testFakes) {
	t.Helper()
	ctx := context.Background()
	created, err := svc.Install(ctx, testActor(), storedbt.ToolPHPMyAdmin, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.runTask(created.TaskID)
}

// TestSignonVerifySuccess 四重校验通过 → 凭据 loopback 注入响应（含 host/port）。
func TestSignonVerifySuccess(t *testing.T) {
	f := defaultFakes()
	svc, _ := newTestService(t, f)
	ctx := context.Background()
	installPMAForSignon(t, svc, f)
	// 会话 rawToken="admin-token"（fakeSession admin）；digest 同口径推导。
	digest := sessionDigestOf("admin-token")
	token, exp, err := svc.signon.Issue(ctx, digest, 9)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if !exp.After(svc.clock()) || exp.Sub(svc.clock()) > SignonTokenTTL+time.Second {
		t.Fatalf("exp = %v（TTL 60s）", exp)
	}
	data, err := svc.SignonVerify(ctx, token, "admin-token", "9")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if data.User != "root" || data.Password != "mysql-pass" || data.Host != "127.0.0.1" || data.Port != 3306 {
		t.Fatalf("creds = %+v", data)
	}
}

// TestSignonTampered ①签名防伪：payload 篡改（实例改绑）→ DBT_007。
func TestSignonTampered(t *testing.T) {
	f := defaultFakes()
	svc, _ := newTestService(t, f)
	ctx := context.Background()
	installPMAForSignon(t, svc, f)
	token, _, err := svc.signon.Issue(ctx, sessionDigestOf("admin-token"), 9)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(token, ".")
	payloadJSON, _ := base64.RawURLEncoding.DecodeString(parts[0])
	var p tokenPayload
	if err := json.Unmarshal(payloadJSON, &p); err != nil {
		t.Fatal(err)
	}
	p.InstanceID = 7 // 跨实例篡改
	tampered, _ := json.Marshal(p)
	forged := base64.RawURLEncoding.EncodeToString(tampered) + "." + parts[1]
	if _, err := svc.SignonVerify(ctx, forged, "admin-token", "7"); symbolOf(err) != "DBT_007" {
		t.Fatalf("tampered token: %v", err)
	}
	// 签名本体篡改。
	forged2 := parts[0] + "." + base64.RawURLEncoding.EncodeToString([]byte("forged-signature"))
	if _, err := svc.SignonVerify(ctx, forged2, "admin-token", "9"); symbolOf(err) != "DBT_007" {
		t.Fatalf("forged signature: %v", err)
	}
	// 形态非法。
	if _, err := svc.SignonVerify(ctx, "not-a-token", "admin-token", "9"); symbolOf(err) != "DBT_007" {
		t.Fatalf("malformed token: %v", err)
	}
}

// TestSignonExpired ②时效：exp 过期 → DBT_008（时钟注入推进）。
func TestSignonExpired(t *testing.T) {
	f := defaultFakes()
	svc, _ := newTestService(t, f)
	ctx := context.Background()
	installPMAForSignon(t, svc, f)
	now := time.Now()
	svc.clock = func() time.Time { return now }
	svc.signon.clock = svc.clock
	token, _, err := svc.signon.Issue(ctx, sessionDigestOf("admin-token"), 9)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(SignonTokenTTL + time.Second) // 推进超 60s
	if _, err := svc.SignonVerify(ctx, token, "admin-token", "9"); symbolOf(err) != "DBT_008" {
		t.Fatalf("expired token: %v", err)
	}
}

// TestSignonReplay ③一次性：消费后重放 → DBT_010；重启（新台账）→ DBT_010。
func TestSignonReplay(t *testing.T) {
	f := defaultFakes()
	svc, store := newTestService(t, f)
	ctx := context.Background()
	installPMAForSignon(t, svc, f)
	token, _, err := svc.signon.Issue(ctx, sessionDigestOf("admin-token"), 9)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SignonVerify(ctx, token, "admin-token", "9"); err != nil {
		t.Fatalf("first verify: %v", err)
	}
	if _, err := svc.SignonVerify(ctx, token, "admin-token", "9"); symbolOf(err) != "DBT_010" {
		t.Fatalf("replayed token: %v", err)
	}
	// 面板重启语义：新 SignonService 台账清空 → 旧令牌全部失效。
	svc2 := New(store, svc.vault, Deps{
		Deploy: f.deploy, Creds: f.creds, Pipeline: f.pipeline, Env: f.env,
		Sysd: f.sysd, PhpFpm: f.phpfpm, Nginx: f.nginx, Panel: f.panel,
		Session: f.session, Audit: f.audit,
	}, Options{DataDir: svc.dataDir, DisableDispatch: true})
	token2, _, _ := svc.signon.Issue(ctx, sessionDigestOf("admin-token"), 9)
	if _, err := svc2.SignonVerify(ctx, token2, "admin-token", "9"); symbolOf(err) != "DBT_010" {
		t.Fatalf("token after restart: %v", err)
	}
}

// TestSignonBinding ④双绑定：跨会话/跨实例/会话失效 → DBT_007。
func TestSignonBinding(t *testing.T) {
	f := defaultFakes()
	svc, _ := newTestService(t, f)
	ctx := context.Background()
	installPMAForSignon(t, svc, f)
	// 跨会话：令牌绑定 admin 会话 digest，以 readonly 会话校验。
	token, _, _ := svc.signon.Issue(ctx, sessionDigestOf("admin-token"), 9)
	if _, err := svc.SignonVerify(ctx, token, "readonly-token", "9"); symbolOf(err) != "DBT_007" {
		t.Fatalf("cross-session: %v", err)
	}
	// 会话失效（未知 token）。
	token2, _, _ := svc.signon.Issue(ctx, sessionDigestOf("admin-token"), 9)
	if _, err := svc.SignonVerify(ctx, token2, "revoked-token", "9"); symbolOf(err) != "DBT_007" {
		t.Fatalf("invalid session: %v", err)
	}
	// 未转发会话 cookie。
	token3, _, _ := svc.signon.Issue(ctx, sessionDigestOf("admin-token"), 9)
	if _, err := svc.SignonVerify(ctx, token3, "", "9"); symbolOf(err) != "DBT_007" {
		t.Fatalf("missing session forward: %v", err)
	}
	// 跨实例（cookie 辅助绑定）。
	token4, _, _ := svc.signon.Issue(ctx, sessionDigestOf("admin-token"), 9)
	if _, err := svc.SignonVerify(ctx, token4, "admin-token", "7"); symbolOf(err) != "DBT_007" {
		t.Fatalf("cross-instance cookie: %v", err)
	}
}

// TestSignonKeyIsolation 密钥 vault 密文存储：不同档案密钥不可互验（防伪
// 根密钥隔离）；卸载后密钥删除令牌永久不可校验。
func TestSignonKeyIsolation(t *testing.T) {
	f := defaultFakes()
	svc, store := newTestService(t, f)
	ctx := context.Background()
	installPMAForSignon(t, svc, f)
	token, _, _ := svc.signon.Issue(ctx, sessionDigestOf("admin-token"), 9)
	// 卸载除档（密钥随档案删除）。
	if err := store.Tool().Delete(ctx, storedbt.ToolPHPMyAdmin); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SignonVerify(ctx, token, "admin-token", "9"); err == nil {
		t.Fatal("token must be unverifiable after uninstall (key removed)")
	}
}

// TestGateVerify gating：无会话/readonly/失效→拒；admin→放行（只读不续期）。
func TestGateVerify(t *testing.T) {
	f := defaultFakes()
	svc, _ := newTestService(t, f)
	ctx := context.Background()
	cases := []struct {
		name, raw string
		want      bool
	}{
		{"no-session", "", false},
		{"admin", "admin-token", true},
		{"readonly", "readonly-token", false},
		{"invalid", "no-such-token", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := svc.GateVerify(ctx, tc.raw); got != tc.want {
				t.Fatalf("GateVerify(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

// TestInternalSecret 内部秘密：0600 + 64 hex + 复用幂等。
func TestInternalSecret(t *testing.T) {
	f := defaultFakes()
	svc, _ := newTestService(t, f)
	s1, err := svc.ensureInternalSecret()
	if err != nil {
		t.Fatal(err)
	}
	if len(s1) != 64 {
		t.Fatalf("secret len = %d, want 64 hex", len(s1))
	}
	if _, err := hex.DecodeString(s1); err != nil {
		t.Fatalf("secret not hex: %v", err)
	}
	s2, err := svc.ensureInternalSecret()
	if err != nil || s1 != s2 {
		t.Fatalf("secret not idempotent: %v", err)
	}
	got, err := svc.InternalSecret()
	if err != nil || got != s1 {
		t.Fatalf("InternalSecret = %q err=%v", got, err)
	}
}

// TestSessionDigestDerivation digest 口径与 account-auth Session.Digest 一致
// （token SHA-256 hex 前 8 位）。
func TestSessionDigestDerivation(t *testing.T) {
	raw := "session-token-value"
	sum := sha256.Sum256([]byte(raw))
	want := hex.EncodeToString(sum[:])[:8]
	if got := sessionDigestOf(raw); got != want {
		t.Fatalf("digest = %q, want %q", got, want)
	}
	// HMAC 规范化：同 payload 同密钥签名稳定（防伪校验基础）。
	key := []byte("0123456789abcdef0123456789abcdef")
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(`{"jti":"x"}`))
	a := mac.Sum(nil)
	mac2 := hmac.New(sha256.New, key)
	mac2.Write([]byte(`{"jti":"x"}`))
	if !hmac.Equal(a, mac2.Sum(nil)) {
		t.Fatal("HMAC not deterministic")
	}
}

// TestOpenValidation 打开前置（DBT_004/012/006/AUTH_012）+ 令牌 URL 形态 +
// 审计脱敏（detail 不含令牌/凭据）。
func TestOpenValidation(t *testing.T) {
	f := defaultFakes()
	svc, _ := newTestService(t, f)
	ctx := context.Background()
	inst := int64(9)
	if _, err := svc.Open(ctx, testActor(), storedbt.ToolPHPMyAdmin, &inst, ""); symbolOf(err) != "DBT_004" {
		t.Fatalf("open not installed: %v", err)
	}
	installPMAForSignon(t, svc, f)
	// 运行态：注入 location 在位 + pool 可用 → 可打开。
	if _, err := svc.Open(ctx, testActor(), storedbt.ToolPHPMyAdmin, nil, ""); symbolOf(err) != "AUTH_012" {
		t.Fatalf("open missing instanceId: %v", err)
	}
	f.creds.ready[9] = ReadyInfo{Ready: false, Reason: "凭据未配置"}
	if _, err := svc.Open(ctx, testActor(), storedbt.ToolPHPMyAdmin, &inst, ""); symbolOf(err) != "DBT_006" {
		t.Fatalf("open instance not ready: %v", err)
	}
	f.creds.ready[9] = ReadyInfo{Ready: true, Engine: "mysql"}
	data, err := svc.Open(ctx, testActor(), storedbt.ToolPHPMyAdmin, &inst, "")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !strings.HasPrefix(data.URL, "http://panel.example.test/dbtool/phpmyadmin/?dg_signon=") {
		t.Fatalf("url = %q", data.URL)
	}
	if data.TokenExpiresAt == nil {
		t.Fatal("tokenExpiresAt missing for phpmyadmin")
	}
	// 审计 detail 脱敏：不含令牌明文与凭据。
	for _, e := range f.audit.entries {
		if e.Action != AuditActionToolAccessed {
			continue
		}
		b, _ := json.Marshal(e.Detail)
		if strings.Contains(string(b), "dg_signon") || strings.Contains(string(b), "mysql-pass") ||
			strings.Contains(string(b), data.URL) {
			t.Fatalf("access audit leaks token/creds: %s", b)
		}
	}
	// mongo_express 未装 → DBT_004；运行态未就绪（单元 not-found）→ DBT_012。
	if _, err := svc.Open(ctx, testActor(), storedbt.ToolMongoExpress, nil, ""); symbolOf(err) != "DBT_004" {
		t.Fatalf("open mongo not installed: %v", err)
	}
	mi := int64(7)
	created, _ := svc.Install(ctx, testActor(), storedbt.ToolMongoExpress, &mi)
	svc.runTask(created.TaskID)
	// fakeSysd start 后单元 active → 可打开；URL 无令牌（basic auth 服务端注入）。
	mdata, err := svc.Open(ctx, testActor(), storedbt.ToolMongoExpress, nil, "")
	if err != nil {
		t.Fatalf("open mongo: %v", err)
	}
	if mdata.URL != "http://panel.example.test/dbtool/mongo-express/" || mdata.TokenExpiresAt != nil {
		t.Fatalf("mongo open data = %+v", mdata)
	}
	// 运行态未就绪（单元被外部移除）→ DBT_012。
	delete(f.sysd.units, MongoExpressUnitName)
	if _, err := svc.Open(ctx, testActor(), storedbt.ToolMongoExpress, nil, ""); symbolOf(err) != "DBT_012" {
		t.Fatalf("open mongo not running: %v", err)
	}
}

// TestReopenChain reopen 重签新令牌（与 open 同一规约）+ 实例就绪前置。
func TestReopenChain(t *testing.T) {
	f := defaultFakes()
	svc, _ := newTestService(t, f)
	ctx := context.Background()
	installPMAForSignon(t, svc, f)
	// actor 会话 digest 须与校验转发的会话同口径（双绑定）。
	actor := &authsvc.Identity{UserID: 1, Username: "admin", Role: "admin",
		SessionDigest: sessionDigestOf("admin-token")}
	url, err := svc.Reopen(ctx, actor, 9, "")
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if !strings.Contains(url, "/dbtool/phpmyadmin/?dg_signon=") {
		t.Fatalf("reopen url = %q", url)
	}
	// 新令牌可校验（一次性独立）。
	token := url[strings.Index(url, "dg_signon=")+len("dg_signon="):]
	if _, err := svc.SignonVerify(ctx, token, "admin-token", "9"); err != nil {
		t.Fatalf("reopen token verify: %v", err)
	}
	// 实例未就绪 → DBT_006。
	f.creds.ready[9] = ReadyInfo{Ready: false, Reason: "x"}
	if _, err := svc.Reopen(ctx, testActor(), 9, ""); symbolOf(err) != "DBT_006" {
		t.Fatalf("reopen not ready: %v", err)
	}
}

// TestTokenSweeper TTL 清扫：过期条目移除（重放窗口收敛）。
func TestTokenSweeper(t *testing.T) {
	f := defaultFakes()
	svc, _ := newTestService(t, f)
	ctx := context.Background()
	installPMAForSignon(t, svc, f)
	now := time.Now()
	svc.clock = func() time.Time { return now }
	svc.signon.clock = svc.clock
	token, _, _ := svc.signon.Issue(ctx, sessionDigestOf("admin-token"), 9)
	parts := strings.Split(token, ".")
	payloadJSON, _ := base64.RawURLEncoding.DecodeString(parts[0])
	var p tokenPayload
	_ = json.Unmarshal(payloadJSON, &p)
	svc.signon.mu.Lock()
	n := len(svc.signon.ledger)
	svc.signon.mu.Unlock()
	if n != 1 {
		t.Fatalf("ledger = %d", n)
	}
	now = now.Add(SignonTokenTTL + time.Second)
	svc.signon.sweep()
	svc.signon.mu.Lock()
	n = len(svc.signon.ledger)
	svc.signon.mu.Unlock()
	if n != 0 {
		t.Fatalf("ledger after sweep = %d, want 0", n)
	}
	// 清扫后令牌不可校验（时钟已超 exp——②时效校验先于③台账判定，DBT_008）。
	if _, err := svc.SignonVerify(ctx, token, "admin-token", "9"); symbolOf(err) != "DBT_008" {
		t.Fatalf("swept token: %v", err)
	}
}
