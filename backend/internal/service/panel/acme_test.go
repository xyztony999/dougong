package panel

// v2.0 增量（ADR-028/029）表驱动测试：域名绑定校验、面板证书状态推算、
// ACME 编排（fake 网关路径：注入/撤除/落库/审计/失败形态）与每日调度单轮。

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/adapter/acme"
	"github.com/dougong/dougong/internal/adapter/nginxx"
	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	"github.com/dougong/dougong/internal/store"
	storepanel "github.com/dougong/dougong/internal/store/panel"
)

// ---- fake 网关 ----

// fakeAcmeIssuer AcmeIssuer 桩：可注入产物/错误，记录调用实参。
type fakeAcmeIssuer struct {
	result  *acme.IssueResult
	err     error
	calls   int
	domains []string
	webroot string
	// onIssue 在 Issue 调用时回调（捕获注入期配置实况）。
	onIssue func(f *fakeAcmeIssuer)
}

func (f *fakeAcmeIssuer) Issue(ctx context.Context, domains []string, challenge, webroot string) (*acme.IssueResult, error) {
	f.calls++
	f.domains = domains
	f.webroot = webroot
	if f.onIssue != nil {
		f.onIssue(f)
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

// fakeNginxChannel NginxChannel 桩：文件系统原语落在真实临时目录（注入/
// 撤除链路真实可验），TestConfig/Reload 结果可注入。
type fakeNginxChannel struct {
	confDirs  []string // 惯例目录绝对路径（扫描序即优先级序）
	unavail   error    // Available 错误（PANEL_008 注入）
	testErr   error    // TestConfig 通道错误
	testOK    bool     // TestConfig 退出码语义
	reloadErr error
	reloads   int
}

func (f *fakeNginxChannel) Available() error { return f.unavail }

func (f *fakeNginxChannel) ScanConfDirs() ([]nginxx.FileSnapshot, error) {
	var out []nginxx.FileSnapshot
	for _, dir := range f.confDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".conf") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				return nil, err
			}
			out = append(out, nginxx.FileSnapshot{
				Path: filepath.Join(dir, e.Name()), Dir: dir, Name: e.Name(), Content: string(b),
			})
		}
	}
	return out, nil
}

func (f *fakeNginxChannel) ReadFile(path string) (string, time.Time, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", time.Time{}, err
	}
	return string(b), time.Time{}, nil
}

func (f *fakeNginxChannel) WriteFileAtomic(path string, content []byte) error {
	return os.WriteFile(path, content, 0o644)
}

func (f *fakeNginxChannel) CopyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

func (f *fakeNginxChannel) MkdirAll(dir string) error { return os.MkdirAll(dir, 0o755) }

func (f *fakeNginxChannel) TestConfig(ctx context.Context) (nginxx.TestResult, error) {
	if f.testErr != nil {
		return nginxx.TestResult{}, f.testErr
	}
	return nginxx.TestResult{OK: f.testOK}, nil
}

func (f *fakeNginxChannel) Reload(ctx context.Context) (string, error) {
	f.reloads++
	if f.reloadErr != nil {
		return "reload output", f.reloadErr
	}
	return "", nil
}

func (f *fakeNginxChannel) RemoveFile(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// acmeTestEnv 编排测试环境：真实 :memory: 库 + fake 双网关 + 临时目录。
type acmeTestEnv struct {
	svc   *Service
	db    *sql.DB
	acme  *fakeAcmeIssuer
	nginx *fakeNginxChannel
	conf  string // 主配置文件路径
	Dir   string // 面板数据目录（webroot/备份派生基点）
}

// writeConf 写主配置文件内容。
func (e *acmeTestEnv) writeConf(t *testing.T, content string) {
	t.Helper()
	if err := os.WriteFile(e.conf, []byte(content), 0o644); err != nil {
		t.Fatalf("write conf: %v", err)
	}
}

// readConf 读主配置文件内容。
func (e *acmeTestEnv) readConf(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(e.conf)
	if err != nil {
		t.Fatalf("read conf: %v", err)
	}
	return string(b)
}

// newAcmeTestEnv 构造环境；confContent 为初始 nginx 配置（80 通路载体）。
func newAcmeTestEnv(t *testing.T, confContent string) *acmeTestEnv {
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
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	dir := t.TempDir()
	confDir := filepath.Join(dir, "sites-enabled")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatalf("mkdir conf: %v", err)
	}
	conf := filepath.Join(confDir, "panel.conf")
	if err := os.WriteFile(conf, []byte(confContent), 0o644); err != nil {
		t.Fatalf("write conf: %v", err)
	}
	fa := &fakeAcmeIssuer{}
	fn := &fakeNginxChannel{confDirs: []string{confDir}, testOK: true}
	svc := New(storepanel.New(db), WithAcmeChannel(fa, fn, dir))
	if _, err := svc.Bootstrap(context.Background()); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	return &acmeTestEnv{svc: svc, db: db, acme: fa, nginx: fn, conf: conf, Dir: dir}
}

// issueResultOf 构造 fake 签发产物（有效期自 now 起）。
func issueResultOf(now time.Time) *acme.IssueResult {
	return &acme.IssueResult{
		CertPEM: []byte("ACME-CERT-PEM"), KeyPEM: []byte("ACME-KEY-PEM"),
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(90 * 24 * time.Hour),
		Issuer: "Fake CA", AccountURI: "https://acme.example/acct/1",
	}
}

// bindDomain 绑定域名（直接落库，模拟 config/update 已生效）。
func (e *acmeTestEnv) bindDomain(t *testing.T, domain string) {
	t.Helper()
	err := e.svc.withTx(context.Background(), func(tx *storepanel.TxStore) error {
		return tx.Config().Update(context.Background(),
			storepanel.ConfigChanges{BindDomain: &domain}, time.Now())
	})
	if err != nil {
		t.Fatalf("bind domain: %v", err)
	}
}

// seedAcmeCert 直插证书行（status 注入用：lastError 非 nil → 经 UpdateAcmeResult
// 落 renew_failed——Upsert 恒清 last_error，D-P13）。
func (e *acmeTestEnv) seedAcmeCert(t *testing.T, notAfter time.Time, lastError *string) {
	t.Helper()
	ctx := context.Background()
	err := e.svc.st.Cert().Upsert(ctx, storepanel.Cert{
		CertPEM: "OLD-PEM", KeyPEM: "OLD-KEY", Source: storepanel.CertSourceAcme,
		Issuer: ptr("Old CA"), NotBefore: ptr(notAfter.Add(-24 * time.Hour)), NotAfter: ptr(notAfter),
	}, time.Now())
	if err != nil {
		t.Fatalf("seed cert: %v", err)
	}
	if lastError != nil {
		if err := e.svc.st.Cert().UpdateAcmeResult(ctx, nil, nil, nil, lastError, time.Now()); err != nil {
			t.Fatalf("seed last error: %v", err)
		}
	}
}

// ---- PanelCertStatusCalculator（D-P13 读时推算）----

func TestCalcCertStatus(t *testing.T) {
	now := time.Now().UTC()
	inHour := now.Add(time.Hour)
	in10d := now.Add(10 * 24 * time.Hour)
	in60d := now.Add(60 * 24 * time.Hour)
	cases := []struct {
		name string
		cert storepanel.Cert
		want string
	}{
		{"非 acme 来源无状态", storepanel.Cert{Source: storepanel.CertSourceImported, NotAfter: ptr(in60d)}, ""},
		{"last_error 非空即 renew_failed", storepanel.Cert{Source: storepanel.CertSourceAcme, NotAfter: ptr(in60d), LastError: ptr("x")}, storepanel.CertStatusRenewFailed},
		{"已过期", storepanel.Cert{Source: storepanel.CertSourceAcme, NotAfter: ptr(now.Add(-time.Hour))}, storepanel.CertStatusExpired},
		{"not_after 缺失按 expired 如实呈现", storepanel.Cert{Source: storepanel.CertSourceAcme}, storepanel.CertStatusExpired},
		{"30 天阈值内 expiring_soon", storepanel.Cert{Source: storepanel.CertSourceAcme, NotAfter: ptr(in10d)}, storepanel.CertStatusExpiringSoon},
		{"阈值边界当日仍 expiring_soon", storepanel.Cert{Source: storepanel.CertSourceAcme, NotAfter: ptr(inHour)}, storepanel.CertStatusExpiringSoon},
		{"30 天外 valid", storepanel.Cert{Source: storepanel.CertSourceAcme, NotAfter: ptr(in60d)}, storepanel.CertStatusValid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CalcCertStatus(tc.cert, now); got != tc.want {
				t.Fatalf("CalcCertStatus = %q, want %q", got, tc.want)
			}
		})
	}
}

// ---- 域名绑定校验（D-P12）----

func TestValidDomain(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"panel.example.com", true},
		{"a-b.example.co", true},
		{"192.168.1.5", true}, // RFC 1123 主机语法（IPv4 通过标签规则）
		{strings.Repeat("a", 63) + ".example.com", true},
		{"", false},
		{"panel.example.com:8443", false},    // 含端口
		{"https://panel.example.com", false}, // 含 scheme
		{"panel.example.com/path", false},    // 含路径
		{"a@b.example.com", false},
		{"-lead.example.com", false},
		{"trail-.example.com", false},
		{"panel..example.com", false},
		{"pa nel.example.com", false},
		{strings.Repeat("a", 254) + ".com", false}, // >253
	}
	for _, tc := range cases {
		if got := validDomain(tc.in); got != tc.want {
			t.Fatalf("validDomain(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestParseBindDomain(t *testing.T) {
	raw := func(s string) json.RawMessage { return json.RawMessage(s) }
	if v, fe := parseBindDomain(raw(`"panel.example.com"`)); fe != nil || v != "panel.example.com" {
		t.Fatalf("string payload = %q, %v", v, fe)
	}
	if v, fe := parseBindDomain(raw(`null`)); fe != nil || v != "" {
		t.Fatalf("null payload = %q, %v", v, fe)
	}
	if v, fe := parseBindDomain(raw(`""`)); fe != nil || v != "" {
		t.Fatalf("empty payload = %q, %v", v, fe)
	}
	if v, fe := parseBindDomain(raw(`" "`)); fe != nil || v != "" {
		t.Fatalf("blank payload = %q, %v", v, fe)
	}
	if _, fe := parseBindDomain(raw(`123`)); fe == nil {
		t.Fatalf("number payload must be field error")
	}
	if _, fe := parseBindDomain(raw(`"bad:80"`)); fe == nil {
		t.Fatalf("invalid domain must be field error")
	}
}

// TestUpdateConfigBindDomain config/update.bindDomain 三态 + 审计明文 + 响应视图。
func TestUpdateConfigBindDomain(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()
	actor := adminActor()

	// 绑定。
	env := mustUpdate(t, svc, ctx, actor, UpdateConfigRequest{BindDomain: rawJSON(`"panel.example.com"`)})
	if env.BindDomain.Value == nil || *env.BindDomain.Value != "panel.example.com" {
		t.Fatalf("bindDomain value = %v", env.BindDomain.Value)
	}
	if env.BindDomain.EffectMode != EffectImmediate {
		t.Fatalf("bindDomain effectMode = %q", env.BindDomain.EffectMode)
	}
	detail := lastAuditDetail(t, db, ActionPanelConfigChanged)
	if !strings.Contains(detail, `"field":"bindDomain"`) ||
		!strings.Contains(detail, `"after":"panel.example.com"`) {
		t.Fatalf("bindDomain delta missing or not plaintext: %s", detail)
	}
	// 未携带=不变（幂等空变更 → 仍成功，无新 delta）。
	before := countAudit(t, db, ActionPanelConfigChanged)
	env = mustUpdate(t, svc, ctx, actor, UpdateConfigRequest{UpdateCheck: ptr(true)})
	if env.BindDomain.Value == nil || *env.BindDomain.Value != "panel.example.com" {
		t.Fatalf("bindDomain must persist when absent: %v", env.BindDomain.Value)
	}
	if after := countAudit(t, db, ActionPanelConfigChanged); after != before+1 {
		t.Fatalf("absent bindDomain must not produce extra delta rows (%d→%d)", before, after)
	}
	if strings.Contains(lastAuditDetail(t, db, ActionPanelConfigChanged), "bindDomain") {
		t.Fatalf("absent bindDomain leaked into delta")
	}
	// null=解绑。
	env = mustUpdate(t, svc, ctx, actor, UpdateConfigRequest{BindDomain: rawJSON(`null`)})
	if env.BindDomain.Value != nil {
		t.Fatalf("unbind failed: %v", env.BindDomain.Value)
	}
	detail = lastAuditDetail(t, db, ActionPanelConfigChanged)
	if !strings.Contains(detail, `"before":"panel.example.com"`) {
		t.Fatalf("unbind delta missing before: %s", detail)
	}
	// 非法域名 → AUTH_012 fields.bindDomain，零写入。
	_, err := svc.UpdateConfig(ctx, actor, UpdateConfigRequest{BindDomain: rawJSON(`"bad:80"`)})
	wantSymbol(t, err, "AUTH_012")
	var ec *errcode.Error
	if errors.As(err, &ec) {
		d, _ := json.Marshal(ec.Data())
		if !strings.Contains(string(d), `"bindDomain"`) {
			t.Fatalf("fields must name bindDomain: %s", d)
		}
	}
	// httpsMode=acme 前置（I-P04/D-P14）：无证书行 → PANEL_003。
	_, err = svc.UpdateConfig(ctx, actor, UpdateConfigRequest{HTTPSMode: ptr("acme")})
	wantSymbol(t, err, "PANEL_003")
}

// rawJSON 速记。
func rawJSON(s string) json.RawMessage {
	return json.RawMessage(s)
}

// mustUpdate 执行更新并返回视图。
func mustUpdate(t *testing.T, svc *Service, ctx context.Context, actor *authsvc.Identity, req UpdateConfigRequest) *DetailView {
	t.Helper()
	view, err := svc.UpdateConfig(ctx, actor, req)
	if err != nil {
		t.Fatalf("update config: %v", err)
	}
	return view
}

func countAudit(t *testing.T, db *sql.DB, action string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = ?`, action).Scan(&n); err != nil {
		t.Fatalf("count audit: %v", err)
	}
	return n
}

// ---- ACME 编排：RequestCert ----

const testConf = `server {
    listen 80;
    server_name panel.example.com;
    root /var/www/html;
}
server {
    listen 443 ssl;
    server_name other.example.com;
}
`

func TestRequestCertGuards(t *testing.T) {
	env := newAcmeTestEnv(t, testConf)
	ctx := context.Background()

	// PANEL_007：未绑定域名。
	_, err := env.svc.RequestCert(ctx, adminActor())
	wantSymbol(t, err, "PANEL_007")
	// PANEL_008：nginx 通道不可用（二进制未检出语义）。
	env.bindDomain(t, "panel.example.com")
	env.nginx.unavail = errors.New("nginx not found")
	_, err = env.svc.RequestCert(ctx, adminActor())
	wantSymbol(t, err, "PANEL_008")
	var ec *errcode.Error
	if errors.As(err, &ec) {
		d, _ := json.Marshal(ec.Data())
		if !strings.Contains(string(d), "nginx not found") {
			t.Fatalf("PANEL_008 data.reason missing: %s", d)
		}
	}
	env.nginx.unavail = nil
	// PANEL_011：已有 valid acme 证书。
	now := time.Now()
	env.seedAcmeCert(t, now.Add(60*24*time.Hour), nil)
	_, err = env.svc.RequestCert(ctx, adminActor())
	wantSymbol(t, err, "PANEL_011")
	if errors.As(err, &ec) {
		d, _ := json.Marshal(ec.Data())
		if !strings.Contains(string(d), "notAfter") {
			t.Fatalf("PANEL_011 data.notAfter missing: %s", d)
		}
	}
	// 防重不阻塞：renew_failed 行重新申请放行到通道层（此处通道正常 → 成功）。
	env.seedAcmeCert(t, now.Add(60*24*time.Hour), ptr("上次失败"))
	env.acme.result = issueResultOf(now)
	if _, err := env.svc.RequestCert(ctx, adminActor()); err != nil {
		t.Fatalf("renew_failed re-request must pass: %v", err)
	}
	// PANEL_008：80 通路缺失（配置只有 443）。
	env2 := newAcmeTestEnv(t, "server {\n    listen 443 ssl;\n    server_name panel.example.com;\n}\n")
	env2.bindDomain(t, "panel.example.com")
	_, err = env2.svc.RequestCert(ctx, adminActor())
	wantSymbol(t, err, "PANEL_008")
}

func TestRequestCertSuccessFlow(t *testing.T) {
	env := newAcmeTestEnv(t, testConf)
	ctx := context.Background()
	now := time.Now()
	env.bindDomain(t, "panel.example.com")
	env.acme.result = issueResultOf(now)
	// 捕获注入期配置实况：Issue 回调时读配置文件。
	var liveConf string
	env.acme.onIssue = func(f *fakeAcmeIssuer) {
		b, _ := os.ReadFile(env.conf)
		liveConf = string(b)
	}

	res, err := env.svc.RequestCert(ctx, adminActor())
	if err != nil {
		t.Fatalf("request cert: %v", err)
	}
	// 响应形态（D-P14 联动）。
	if res.Cert.Source != storepanel.CertSourceAcme || res.Cert.Status != storepanel.CertStatusValid ||
		res.HTTPSMode != storepanel.HTTPSModeAcme || res.Cert.Issuer == nil || *res.Cert.Issuer != "Fake CA" {
		t.Fatalf("result = %+v", res)
	}
	// 注入期：挑战 location 已写入目标 server 块（alias 指向面板挑战目录）。
	if !strings.Contains(liveConf, "location ^~ /.well-known/acme-challenge/") {
		t.Fatalf("challenge location not injected:\n%s", liveConf)
	}
	if !strings.Contains(liveConf, "alias ") {
		t.Fatalf("challenge location must use alias (D-P15):\n%s", liveConf)
	}
	// 签发域名与 webroot。
	if len(env.acme.domains) != 1 || env.acme.domains[0] != "panel.example.com" {
		t.Fatalf("issue domains = %v", env.acme.domains)
	}
	if !strings.Contains(strings.ToLower(env.acme.webroot), "acme-webroot") {
		t.Fatalf("webroot = %q", env.acme.webroot)
	}
	// 撤除：配置恢复原状（I-P10 成败均恢复）。
	if got := env.readConf(t); got != testConf {
		t.Fatalf("conf not restored:\n%s", got)
	}
	if env.nginx.reloads < 2 { // 注入生效 + 撤除生效
		t.Fatalf("reloads = %d, want ≥2", env.nginx.reloads)
	}
	// 落库：source=acme、last_error=NULL、材料替换、https_mode 联动。
	cert, err := env.svc.st.Cert().Get(ctx)
	if err != nil {
		t.Fatalf("get cert: %v", err)
	}
	if cert.CertPEM != "ACME-CERT-PEM" || cert.KeyPEM != "ACME-KEY-PEM" ||
		cert.Source != storepanel.CertSourceAcme || cert.LastError != nil {
		t.Fatalf("cert row = %+v", cert)
	}
	cfg, err := env.svc.st.Config().Get(ctx)
	if err != nil || cfg.HTTPSMode != storepanel.HTTPSModeAcme {
		t.Fatalf("https_mode linkage = %v (%v)", cfg.HTTPSMode, err)
	}
	// 审计：panel_cert_requested 成功一条（detail={domain,result}）。
	detail := lastAuditDetail(t, env.db, ActionPanelCertRequested)
	if !strings.Contains(detail, "panel.example.com") || !strings.Contains(detail, "success") {
		t.Fatalf("audit detail = %s", detail)
	}
	// 备份已落（R-06 第一步）。
	bucket := filepath.Join(env.Dir, "conf.backup", "panel-acme", "panel.conf")
	entries, rerr := os.ReadDir(bucket)
	if rerr != nil || len(entries) == 0 {
		t.Fatalf("backup missing: %v (%d)", rerr, len(entries))
	}
}

func TestRequestCertIssueFailureNoRowNoResidue(t *testing.T) {
	env := newAcmeTestEnv(t, testConf)
	ctx := context.Background()
	env.bindDomain(t, "panel.example.com")
	env.acme.err = errors.New("acme: connection refused")

	_, err := env.svc.RequestCert(ctx, adminActor())
	wantSymbol(t, err, "PANEL_009")
	var ec *errcode.Error
	if errors.As(err, &ec) {
		d, _ := json.Marshal(ec.Data())
		if !strings.Contains(string(d), "connection refused") {
			t.Fatalf("PANEL_009 data.reason missing: %s", d)
		}
	}
	// D-P13：首次申请失败不落库。
	if _, gerr := env.svc.st.Cert().Get(ctx); !errors.Is(gerr, sql.ErrNoRows) {
		t.Fatalf("cert row must not exist after first failure, got %v", gerr)
	}
	// I-P10：挑战 location 已撤除（配置恢复原状）。
	if got := env.readConf(t); got != testConf {
		t.Fatalf("conf not restored after failure:\n%s", got)
	}
	// 审计 failure 一条（detail 含 reason）。
	detail := lastAuditDetail(t, env.db, ActionPanelCertRequested)
	if !strings.Contains(detail, "failure") || !strings.Contains(detail, "connection refused") {
		t.Fatalf("audit detail = %s", detail)
	}
}

func TestRequestCertInjectTestFailRollsBack(t *testing.T) {
	env := newAcmeTestEnv(t, testConf)
	ctx := context.Background()
	env.bindDomain(t, "panel.example.com")
	env.nginx.testOK = false // `nginx -t` 退出码非 0 → 恢复原状并拒绝

	_, err := env.svc.RequestCert(ctx, adminActor())
	wantSymbol(t, err, "PANEL_008")
	if got := env.readConf(t); got != testConf {
		t.Fatalf("inject must roll back on nginx -t failure:\n%s", got)
	}
	if env.acme.calls != 0 {
		t.Fatalf("issue must not run when inject failed")
	}
}

// ---- ACME 编排：RetryRenewal ----

func TestRetryRenewalGuards(t *testing.T) {
	env := newAcmeTestEnv(t, testConf)
	ctx := context.Background()
	now := time.Now()

	// 无行 → PANEL_010 data.currentSource=null。
	_, err := env.svc.RetryRenewal(ctx, adminActor())
	wantSymbol(t, err, "PANEL_010")
	var ec *errcode.Error
	if errors.As(err, &ec) {
		d, _ := json.Marshal(ec.Data())
		if !strings.Contains(string(d), `"currentSource":null`) {
			t.Fatalf("PANEL_010 data = %s", d)
		}
	}
	// 非 acme 来源 → PANEL_010 data.currentSource。
	if uerr := env.svc.st.Cert().Upsert(ctx, storepanel.Cert{
		CertPEM: "P", KeyPEM: "K", Source: storepanel.CertSourceImported,
	}, now); uerr != nil {
		t.Fatalf("upsert: %v", uerr)
	}
	_, err = env.svc.RetryRenewal(ctx, adminActor())
	wantSymbol(t, err, "PANEL_010")
	if errors.As(err, &ec) {
		d, _ := json.Marshal(ec.Data())
		if !strings.Contains(string(d), `"currentSource":"imported"`) {
			t.Fatalf("PANEL_010 data = %s", d)
		}
	}
	// 状态非 renew_failed → PANEL_010 data.currentStatus。
	env.seedAcmeCert(t, now.Add(60*24*time.Hour), nil) // valid
	_, err = env.svc.RetryRenewal(ctx, adminActor())
	wantSymbol(t, err, "PANEL_010")
	if errors.As(err, &ec) {
		d, _ := json.Marshal(ec.Data())
		if !strings.Contains(string(d), `"currentStatus":"valid"`) {
			t.Fatalf("PANEL_010 data = %s", d)
		}
	}
	// renew_failed 但未绑定域名 → PANEL_007。
	env.seedAcmeCert(t, now.Add(60*24*time.Hour), ptr("上次失败"))
	_, err = env.svc.RetryRenewal(ctx, adminActor())
	wantSymbol(t, err, "PANEL_007")
}

func TestRetryRenewalSuccessAndFailure(t *testing.T) {
	env := newAcmeTestEnv(t, testConf)
	ctx := context.Background()
	now := time.Now()
	env.bindDomain(t, "panel.example.com")
	// 域名变更场景：旧证书行为旧域名签发，续期按新 bind_domain。
	env.seedAcmeCert(t, now.Add(60*24*time.Hour), ptr("上次失败"))

	env.acme.result = issueResultOf(now)
	res, err := env.svc.RetryRenewal(ctx, adminActor())
	if err != nil {
		t.Fatalf("retry renewal: %v", err)
	}
	if res.HTTPSMode != storepanel.HTTPSModeAcme || res.Cert.Status != storepanel.CertStatusValid {
		t.Fatalf("result = %+v", res)
	}
	if len(env.acme.domains) != 1 || env.acme.domains[0] != "panel.example.com" {
		t.Fatalf("renewal domain = %v", env.acme.domains)
	}
	cert, err := env.svc.st.Cert().Get(ctx)
	if err != nil {
		t.Fatalf("get cert: %v", err)
	}
	if cert.CertPEM != "ACME-CERT-PEM" || cert.LastError != nil { // 材料替换 + last_error 清空
		t.Fatalf("cert after renewal = %+v", cert)
	}
	detail := lastAuditDetail(t, env.db, ActionPanelCertRenewalRetried)
	if !strings.Contains(detail, "success") {
		t.Fatalf("audit detail = %s", detail)
	}

	// 失败：last_error 落库（renew_failed 呈现）+ PANEL_009。
	env.nginx.testOK = true
	env.acme.err = errors.New("challenge timeout")
	env.seedAcmeCert(t, now.Add(60*24*time.Hour), ptr("上次失败"))
	_, err = env.svc.RetryRenewal(ctx, adminActor())
	wantSymbol(t, err, "PANEL_009")
	cert, err = env.svc.st.Cert().Get(ctx)
	if err != nil {
		t.Fatalf("get cert: %v", err)
	}
	if cert.LastError == nil || !strings.Contains(*cert.LastError, "challenge timeout") {
		t.Fatalf("last_error after failure = %v", cert.LastError)
	}
	detail = lastAuditDetail(t, env.db, ActionPanelCertRenewalRetried)
	if !strings.Contains(detail, "failure") || !strings.Contains(detail, "challenge timeout") {
		t.Fatalf("audit detail = %s", detail)
	}
}

// ---- ACME 编排：D-P15 落点选择与复用 ----

func TestChallengeLocationTargetSelection(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name   string
		conf   string
		domain string
	}{
		{"server_name 精确匹配优先", `server {
    listen 80 default_server;
    server_name fallback.example;
}
server {
    listen 80;
    server_name panel.example.com;
}
`, "panel.example.com"},
		{"无名字匹配回退 default_server", `server {
    listen 80 default_server;
    server_name catchall;
}
server {
    listen 80;
    server_name other.example;
}
`, "panel.example.com"},
		{"再无默认回退首个 80 块", `server {
    listen 80;
    server_name first.example;
}
server {
    listen 80;
    server_name second.example;
}
`, "panel.example.com"},
		{"IPv6 80 监听计入", `server {
    listen [::]:80;
    server_name v6.example;
}
`, "panel.example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newAcmeTestEnv(t, tc.conf)
			env.bindDomain(t, tc.domain)
			env.acme.result = issueResultOf(now)
			if _, err := env.svc.RequestCert(context.Background(), adminActor()); err != nil {
				t.Fatalf("request cert: %v", err)
			}
			if got := env.readConf(t); got != tc.conf {
				t.Fatalf("conf not restored (target selection path):\n%s", got)
			}
		})
	}
}

func TestChallengeLocationReuseExisting(t *testing.T) {
	// 既有 acme-challenge location（root 形态）→ 零写回复用其 root 为 webroot。
	conf := `server {
    listen 80;
    server_name panel.example.com;
    location /.well-known/acme-challenge/ {
        root /srv/shared-webroot;
    }
}
`
	env := newAcmeTestEnv(t, conf)
	env.bindDomain(t, "panel.example.com")
	now := time.Now()
	env.acme.result = issueResultOf(now)
	if _, err := env.svc.RequestCert(context.Background(), adminActor()); err != nil {
		t.Fatalf("request cert: %v", err)
	}
	if env.acme.webroot != "/srv/shared-webroot" {
		t.Fatalf("webroot = %q, want reuse existing root", env.acme.webroot)
	}
	if env.nginx.reloads != 0 {
		t.Fatalf("reuse must not write/reload, reloads = %d", env.nginx.reloads)
	}
	if got := env.readConf(t); got != conf {
		t.Fatalf("reuse must not modify conf:\n%s", got)
	}
}

// ---- 4.1.3 每日调度单轮 ----

func TestPanelRenewalPassExpiringSoon(t *testing.T) {
	env := newAcmeTestEnv(t, testConf)
	ctx := context.Background()
	now := time.Now()
	env.bindDomain(t, "panel.example.com")
	env.seedAcmeCert(t, now.Add(10*24*time.Hour), nil) // expiring_soon
	env.acme.result = issueResultOf(now)

	env.svc.runPanelRenewalPass(ctx)
	cert, err := env.svc.st.Cert().Get(ctx)
	if err != nil {
		t.Fatalf("get cert: %v", err)
	}
	if cert.CertPEM != "ACME-CERT-PEM" {
		t.Fatalf("cert not renewed: %+v", cert)
	}
	// 审计：panel_cert_renewed source=system 成功一条。
	var source string
	if err := env.db.QueryRow(`SELECT source FROM audit_log WHERE action = ? ORDER BY id DESC LIMIT 1`,
		ActionPanelCertRenewed).Scan(&source); err != nil || source != "system" {
		t.Fatalf("renewal audit source = %q (%v)", source, err)
	}
	if got := env.readConf(t); got != testConf {
		t.Fatalf("conf not restored after scheduled renewal:\n%s", got)
	}
}

func TestPanelRenewalPassSkipsAndFails(t *testing.T) {
	env := newAcmeTestEnv(t, testConf)
	ctx := context.Background()
	now := time.Now()
	env.bindDomain(t, "panel.example.com")

	// 非到期窗口（valid）与失败态：不发起签发。
	env.seedAcmeCert(t, now.Add(60*24*time.Hour), nil)
	env.svc.runPanelRenewalPass(ctx)
	if env.acme.calls != 0 {
		t.Fatalf("valid cert must not trigger renewal")
	}
	// expiring_soon + 续期失败 → last_error 落库（renew_failed 呈现）。
	env.seedAcmeCert(t, now.Add(10*24*time.Hour), nil)
	env.acme.err = errors.New("ca rejected")
	env.svc.runPanelRenewalPass(ctx)
	cert, err := env.svc.st.Cert().Get(ctx)
	if err != nil {
		t.Fatalf("get cert: %v", err)
	}
	if cert.LastError == nil || !strings.Contains(*cert.LastError, "ca rejected") {
		t.Fatalf("scheduled failure must persist last_error, got %v", cert.LastError)
	}
	if env.acme.calls != 1 {
		t.Fatalf("exactly one attempt per pass, calls = %d", env.acme.calls)
	}
	// bind_domain 未绑定 → 无法续期，last_error 如实落库。
	env2 := newAcmeTestEnv(t, testConf)
	env2.seedAcmeCert(t, now.Add(10*24*time.Hour), nil)
	env2.svc.runPanelRenewalPass(ctx)
	cert, err = env2.svc.st.Cert().Get(ctx)
	if err != nil {
		t.Fatalf("get cert: %v", err)
	}
	if cert.LastError == nil || !strings.Contains(*cert.LastError, "bind_domain") {
		t.Fatalf("unbound domain must persist reason, got %v", cert.LastError)
	}
}

// ---- cert/status 视图 ----

func TestCertStatusView(t *testing.T) {
	env := newAcmeTestEnv(t, testConf)
	ctx := context.Background()
	now := time.Now()

	// 未配置证书：cert=null，acme 概要完整。
	res, err := env.svc.CertStatus(ctx)
	if err != nil {
		t.Fatalf("cert status: %v", err)
	}
	if res.Cert != nil {
		t.Fatalf("cert must be null when unconfigured")
	}
	if res.Acme.Domain != nil || res.Acme.Challenge != ChallengePanelCert || !res.Acme.AutoRenew {
		t.Fatalf("acme info = %+v", res.Acme)
	}
	// 绑定后 domain=bind_domain 当前值。
	env.bindDomain(t, "panel.example.com")
	res, _ = env.svc.CertStatus(ctx)
	if res.Acme.Domain == nil || *res.Acme.Domain != "panel.example.com" {
		t.Fatalf("acme.domain = %v", res.Acme.Domain)
	}
	// 非 acme 来源：status=null（D-P13）。
	if err := env.svc.st.Cert().Upsert(ctx, storepanel.Cert{
		CertPEM: "P", KeyPEM: "K", Source: storepanel.CertSourceSelfSigned,
		NotAfter: ptr(now.Add(90 * 24 * time.Hour)),
	}, now); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	res, _ = env.svc.CertStatus(ctx)
	if res.Cert == nil || res.Cert.Status != nil || res.Cert.Source != storepanel.CertSourceSelfSigned {
		t.Fatalf("self_signed status view = %+v", res.Cert)
	}
	// acme 来源：status 推算 + lastError 透出。
	env.seedAcmeCert(t, now.Add(10*24*time.Hour), ptr("上次失败"))
	res, _ = env.svc.CertStatus(ctx)
	if res.Cert == nil || res.Cert.Status == nil || *res.Cert.Status != storepanel.CertStatusRenewFailed {
		t.Fatalf("renew_failed view = %+v", res.Cert)
	}
	if res.Cert.LastError == nil || *res.Cert.LastError != "上次失败" {
		t.Fatalf("lastError = %v", res.Cert.LastError)
	}
}
