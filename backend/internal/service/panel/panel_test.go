package panel

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	"github.com/dougong/dougong/internal/store"
	storeauth "github.com/dougong/dougong/internal/store/auth"
	storepanel "github.com/dougong/dougong/internal/store/panel"
)

// ---- 测试基座 ----

func newTestService(t *testing.T) (*Service, *sql.DB) {
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
	svc := New(storepanel.New(db))
	if _, err := svc.Bootstrap(context.Background()); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	return svc, db
}

func adminActor() *authsvc.Identity {
	return &authsvc.Identity{UserID: 1, Username: "admin", Role: storeauth.RoleAdmin}
}

func readonlyActor() *authsvc.Identity {
	return &authsvc.Identity{UserID: 2, Username: "watcher", Role: storeauth.RoleReadonly}
}

// wantSymbol 断言错误为指定契约符号。
func wantSymbol(t *testing.T, err error, symbol string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want %s, got nil", symbol)
	}
	var ec *errcode.Error
	if !errors.As(err, &ec) {
		t.Fatalf("want errcode %s, got %v", symbol, err)
	}
	if ec.Symbol() != symbol {
		t.Fatalf("symbol = %s, want %s (message=%s)", ec.Symbol(), symbol, ec.Message())
	}
}

func ptr[T any](v T) *T { return &v }

// lastAuditDetail 取最新一条指定动作审计的 detail JSON。
func lastAuditDetail(t *testing.T, db *sql.DB, action string) string {
	t.Helper()
	var detail sql.NullString
	err := db.QueryRow(`SELECT detail FROM audit_log WHERE action = ? ORDER BY id DESC LIMIT 1`, action).Scan(&detail)
	if err != nil {
		t.Fatalf("read audit %s: %v", action, err)
	}
	if !detail.Valid {
		return ""
	}
	return detail.String
}

// ---- EntryPathGenerator ----

func TestGenerateEntryPath(t *testing.T) {
	for i := 0; i < 64; i++ {
		p, err := GenerateEntryPath(defaultTestRand{})
		if err != nil {
			t.Fatalf("generate: %v", err)
		}
		if !EntryPathPattern.MatchString(p) {
			t.Fatalf("path %q violates I-P03 pattern", p)
		}
		if len(p) != entryPathLen {
			t.Fatalf("length = %d", len(p))
		}
	}
	// 确定性：递增熵源 0..15 映射字符集前 16 项（无偏模映射）。
	p, err := GenerateEntryPath(seqReader{})
	if err != nil {
		t.Fatalf("generate deterministic: %v", err)
	}
	if p != "ABCDEFGHIJKLMNOP" {
		t.Fatalf("deterministic mapping = %q", p)
	}
}

type seqReader struct{}

func (seqReader) Read(b []byte) (int, error) {
	for i := range b {
		b[i] = byte(i)
	}
	return len(b), nil
}

type defaultTestRand struct{}

func (defaultTestRand) Read(b []byte) (int, error) {
	// 简单伪随机（测试熵源，仅喂生成器；不用于生产语义）。
	seed := time.Now().UnixNano()
	for i := range b {
		seed = seed*6364136223846793005 + 1442695040888963407
		b[i] = byte(seed >> 56)
	}
	return len(b), nil
}

// randomListenPort 覆盖（Bootstrap 已隐式验证；此处直测范围）。
func TestRandomListenPortRange(t *testing.T) {
	for i := 0; i < 64; i++ {
		p, err := randomListenPort(defaultTestRand{})
		if err != nil {
			t.Fatalf("random port: %v", err)
		}
		if p < ListenPortMin || p > ListenPortMax {
			t.Fatalf("port %d out of range", p)
		}
	}
}

// ---- UpdateConfig 校验矩阵（api 4.1.4 校验顺序）----

func TestUpdateConfigValidationMatrix(t *testing.T) {
	tests := []struct {
		name    string
		req     UpdateConfigRequest
		wantSym string
	}{
		{"session 字段被拒（D-P08）", UpdateConfigRequest{Session: ptr(json.RawMessage(`{"absoluteTimeout":1}`))}, "AUTH_012"},
		{"空字段集", UpdateConfigRequest{}, "AUTH_012"},
		{"listenScope 非法枚举", UpdateConfigRequest{ListenScope: ptr("anywhere")}, "AUTH_012"},
		{"端口低于 1024", UpdateConfigRequest{ListenPort: ptr(80)}, "AUTH_012"},
		{"端口高于 65535", UpdateConfigRequest{ListenPort: ptr(70000)}, "AUTH_012"},
		{"地址非法", UpdateConfigRequest{ListenAddress: ptr("bad host!")}, "AUTH_012"},
		{"custom 缺地址（I-P02→PANEL_001）", UpdateConfigRequest{ListenScope: ptr("custom")}, "PANEL_001"},
		{"custom 地址格式非法（R-14 字段级）", UpdateConfigRequest{ListenScope: ptr("custom"), ListenAddress: ptr("not a host!!")}, "AUTH_012"},
		{"非 custom 携带地址", UpdateConfigRequest{ListenScope: ptr("all"), ListenAddress: ptr("127.0.0.1")}, "AUTH_012"},
		{"语言不受支持（I-P08）", UpdateConfigRequest{Lang: ptr("en-US")}, "PANEL_006"},
		{"httpsMode 非法枚举", UpdateConfigRequest{HTTPSMode: ptr("on")}, "AUTH_012"},
		{"切 imported 无证书（I-P04）", UpdateConfigRequest{HTTPSMode: ptr("imported")}, "PANEL_003"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, db := newTestService(t)
			before, err := svc.Detail(context.Background())
			if err != nil {
				t.Fatalf("detail before: %v", err)
			}
			_, err = svc.UpdateConfig(context.Background(), adminActor(), tt.req)
			wantSymbol(t, err, tt.wantSym)
			// 失败保持原值（I-P07：不允许半写）。
			after, err := svc.Detail(context.Background())
			if err != nil {
				t.Fatalf("detail after: %v", err)
			}
			if after.PendingRestart != before.PendingRestart || after.Listen.Port != before.Listen.Port ||
				after.HTTPS.Mode != before.HTTPS.Mode || after.Lang.Value != before.Lang.Value {
				t.Fatalf("failed update mutated config: before=%+v after=%+v", before, after)
			}
			var n int
			if err := db.QueryRow(`SELECT COUNT(*) FROM audit_log`).Scan(&n); err != nil || n != 0 {
				t.Fatalf("failed update wrote audit: n=%d err=%v", n, err)
			}
		})
	}
}

func TestUpdateConfigHappyPaths(t *testing.T) {
	ctx := context.Background()
	svc, db := newTestService(t)
	actor := adminActor()

	detail, err := svc.Detail(ctx)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	// effectMode 分类（RestartClassifier：仅 listen.* restart_required）。
	if detail.Listen.EffectMode != EffectRestartRequired || detail.EntryPath.EffectMode != EffectImmediate ||
		detail.HTTPS.EffectMode != EffectImmediate || detail.UpdateCheck.EffectMode != EffectImmediate ||
		detail.Lang.EffectMode != EffectImmediate {
		t.Fatalf("effect mode classification wrong: %+v", detail)
	}
	if detail.PendingRestart {
		t.Fatal("fresh boot must not be pending restart")
	}
	if len(detail.Lang.Supported) != 1 || detail.Lang.Supported[0] != LangZhCN {
		t.Fatalf("supported langs = %v", detail.Lang.Supported)
	}
	startupPort := detail.Listen.Port

	// 1) 端口变更 → pendingRestart 派生为 true；审计 panel_config_changed。
	updated, err := svc.UpdateConfig(ctx, actor, UpdateConfigRequest{ListenPort: ptr(startupPort + 1)})
	if err != nil {
		t.Fatalf("update port: %v", err)
	}
	if !updated.PendingRestart || updated.Listen.Port != startupPort+1 {
		t.Fatalf("pendingRestart derivation failed: %+v", updated.Listen)
	}
	d := lastAuditDetail(t, db, ActionPanelConfigChanged)
	if !strings.Contains(d, `"listenPort"`) {
		t.Fatalf("audit detail lacks listenPort delta: %s", d)
	}

	// 2) 端口改回 → pendingRestart 消失（基线对比语义）。
	updated, err = svc.UpdateConfig(ctx, actor, UpdateConfigRequest{ListenPort: ptr(startupPort)})
	if err != nil {
		t.Fatalf("update port back: %v", err)
	}
	if updated.PendingRestart {
		t.Fatal("pendingRestart should clear after revert")
	}

	// 3) scope custom + 合法地址 → 保存；再切 all（不带地址）→ 自动清空。
	updated, err = svc.UpdateConfig(ctx, actor, UpdateConfigRequest{
		ListenScope: ptr("custom"), ListenAddress: ptr("192.168.1.10"),
	})
	if err != nil {
		t.Fatalf("update custom scope: %v", err)
	}
	if updated.Listen.Address == nil || *updated.Listen.Address != "192.168.1.10" {
		t.Fatalf("custom address not saved: %+v", updated.Listen)
	}
	updated, err = svc.UpdateConfig(ctx, actor, UpdateConfigRequest{ListenScope: ptr("all")})
	if err != nil {
		t.Fatalf("update back to all: %v", err)
	}
	if updated.Listen.Address != nil {
		t.Fatalf("address not auto-cleared: %v", *updated.Listen.Address)
	}
	d = lastAuditDetail(t, db, ActionPanelConfigChanged)
	if !strings.Contains(d, `"listenAddress"`) {
		t.Fatalf("auto-clear not audited: %s", d)
	}

	// 4) httpsMode=self_signed → 即时生成证书（D-P06）。
	updated, err = svc.UpdateConfig(ctx, actor, UpdateConfigRequest{HTTPSMode: ptr("self_signed")})
	if err != nil {
		t.Fatalf("update self_signed: %v", err)
	}
	if updated.HTTPS.Mode != storepanel.HTTPSModeSelfSigned || updated.HTTPS.Cert == nil ||
		updated.HTTPS.Cert.Source != storepanel.CertSourceSelfSigned {
		t.Fatalf("self-signed cert not materialized: %+v", updated.HTTPS)
	}
	if updated.HTTPS.Cert.Issuer == nil || *updated.HTTPS.Cert.Issuer == "" || updated.HTTPS.Cert.NotAfter == nil {
		t.Fatalf("self-signed summary incomplete: %+v", updated.HTTPS.Cert)
	}

	// 5) 有证书后切 imported 成功（I-P04 前置满足）。
	updated, err = svc.UpdateConfig(ctx, actor, UpdateConfigRequest{HTTPSMode: ptr("imported")})
	if err != nil {
		t.Fatalf("update imported: %v", err)
	}
	if updated.HTTPS.Mode != storepanel.HTTPSModeImported {
		t.Fatalf("imported mode not set: %s", updated.HTTPS.Mode)
	}

	// 6) updateCheck 开关 → 审计含 updateCheck delta。
	updated, err = svc.UpdateConfig(ctx, actor, UpdateConfigRequest{UpdateCheck: ptr(true)})
	if err != nil {
		t.Fatalf("update updateCheck: %v", err)
	}
	if !updated.UpdateCheck.Enabled {
		t.Fatal("updateCheck not enabled")
	}
	d = lastAuditDetail(t, db, ActionPanelConfigChanged)
	if !strings.Contains(d, `"updateCheck"`) || !strings.Contains(d, `"after":true`) {
		t.Fatalf("updateCheck delta missing: %s", d)
	}
}

// TestUpdateConfigEntryPathEnableAutoGenerate：0→1 无路径时自动生成
// （等价 regenerate 语义，A-P9：两笔审计同事务、entryPath 脱敏）。
func TestUpdateConfigEntryPathEnableAutoGenerate(t *testing.T) {
	ctx := context.Background()
	svc, db := newTestService(t)

	updated, err := svc.UpdateConfig(ctx, adminActor(), UpdateConfigRequest{EntryPathEnabled: ptr(true)})
	if err != nil {
		t.Fatalf("enable entry path: %v", err)
	}
	if !updated.EntryPath.Enabled || updated.EntryPath.Path == nil || !EntryPathPattern.MatchString(*updated.EntryPath.Path) {
		t.Fatalf("entry path not generated: %+v", updated.EntryPath)
	}
	enabled, path, err := svc.EntryPathState(ctx)
	if err != nil || !enabled || path != *updated.EntryPath.Path {
		t.Fatalf("entry state mismatch: enabled=%v path=%q err=%v", enabled, path, err)
	}
	// 审计脱敏：明文路径不得入审计（readonly 可见审计页）。
	for _, action := range []string{ActionPanelConfigChanged, ActionPanelEntryPathRegenerated} {
		d := lastAuditDetail(t, db, action)
		if strings.Contains(d, *updated.EntryPath.Path) {
			t.Fatalf("audit %s leaks plaintext entry path: %s", action, d)
		}
		if !strings.Contains(d, "****(len=") {
			t.Fatalf("audit %s lacks masked entry path: %s", action, d)
		}
	}
	// 再次提交 enabled=true（已有路径）：路径不变，不重复生成。
	again, err := svc.UpdateConfig(ctx, adminActor(), UpdateConfigRequest{EntryPathEnabled: ptr(true)})
	if err != nil {
		t.Fatalf("idempotent enable: %v", err)
	}
	if *again.EntryPath.Path != *updated.EntryPath.Path {
		t.Fatalf("existing path must be kept: %q -> %q", *updated.EntryPath.Path, *again.EntryPath.Path)
	}
}

func TestRegenerateEntryPath(t *testing.T) {
	ctx := context.Background()
	svc, db := newTestService(t)

	// 默认关闭状态：regenerate 仍可用（保持 enabled 状态不变）。
	res, err := svc.RegenerateEntryPath(ctx, adminActor())
	if err != nil {
		t.Fatalf("regenerate: %v", err)
	}
	if !EntryPathPattern.MatchString(res.EntryPath) {
		t.Fatalf("regenerated path invalid: %q", res.EntryPath)
	}
	d := lastAuditDetail(t, db, ActionPanelEntryPathRegenerated)
	if strings.Contains(d, res.EntryPath) {
		t.Fatalf("regenerate audit leaks path: %s", d)
	}
	enabled, path, err := svc.EntryPathState(ctx)
	if err != nil || enabled || path != res.EntryPath {
		t.Fatalf("enabled state must be preserved: enabled=%v path=%q", enabled, path)
	}

	res2, err := svc.RegenerateEntryPath(ctx, adminActor())
	if err != nil {
		t.Fatalf("regenerate 2: %v", err)
	}
	if res2.EntryPath == res.EntryPath {
		t.Fatal("regenerate must produce a new path")
	}
	// masked before/after 都在审计中。
	d = lastAuditDetail(t, db, ActionPanelEntryPathRegenerated)
	if !strings.Contains(d, `"before"`) || !strings.Contains(d, `"after"`) {
		t.Fatalf("regenerate audit detail incomplete: %s", d)
	}
}

// ---- 证书 ----

func TestUploadCertificate(t *testing.T) {
	ctx := context.Background()
	svc, db := newTestService(t)
	now := time.Now().UTC().Truncate(time.Second)

	certPEM, keyPEM, fields, err := IssueSelfSigned(defaultTestRand{}, now)
	if err != nil {
		t.Fatalf("issue pair: %v", err)
	}

	// 垃圾 PEM → PANEL_002（R-14）。
	_, err = svc.UploadCertificate(ctx, adminActor(), UploadCertificateRequest{CertPEM: "not a pem", KeyPEM: keyPEM})
	wantSymbol(t, err, "PANEL_002")
	// 空内容 → AUTH_012。
	_, err = svc.UploadCertificate(ctx, adminActor(), UploadCertificateRequest{})
	wantSymbol(t, err, "AUTH_012")

	// 正常导入。
	res, err := svc.UploadCertificate(ctx, adminActor(), UploadCertificateRequest{CertPEM: certPEM, KeyPEM: keyPEM})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if res.Cert.Source != storepanel.CertSourceImported || res.Cert.Issuer == nil || *res.Cert.Issuer != fields.Issuer {
		t.Fatalf("upload summary mismatch: %+v", res.Cert)
	}
	// 导入即启用（v0.5.2 拍板）：https_mode 自动切到 imported，模式标签与实际
	// serve 的证书一致（此前导入不改模式，自签标签谎报）。
	detail, err := svc.Detail(ctx)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.HTTPS.Mode != storepanel.HTTPSModeImported {
		t.Fatalf("upload must activate imported mode: %s", detail.HTTPS.Mode)
	}

	// I-P06：响应零私钥材料。
	blob, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(blob), "PRIVATE KEY") || strings.Contains(string(blob), "keyPem") ||
		strings.Contains(string(blob), keyPEM[:32]) {
		t.Fatalf("upload response leaks key material: %s", blob)
	}
	// 审计 panel_cert_updated：detail 无 PEM 材料。
	d := lastAuditDetail(t, db, ActionPanelCertUpdated)
	if strings.Contains(d, "PRIVATE KEY") || !strings.Contains(d, `"imported"`) {
		t.Fatalf("cert audit detail wrong: %s", d)
	}

	// 错配对：证书 A + 私钥 B → PANEL_002。
	cert2PEM, key2PEM, _, err := IssueSelfSigned(defaultTestRand{}, now)
	if err != nil {
		t.Fatalf("issue pair 2: %v", err)
	}
	_ = cert2PEM
	_, err = svc.UploadCertificate(ctx, adminActor(), UploadCertificateRequest{CertPEM: certPEM, KeyPEM: key2PEM})
	wantSymbol(t, err, "PANEL_002")

	// 导入后切 imported 成功（I-P04）。
	if _, err := svc.UpdateConfig(ctx, adminActor(), UpdateConfigRequest{HTTPSMode: ptr("imported")}); err != nil {
		t.Fatalf("switch imported after upload: %v", err)
	}
}

// ---- 更新检查（I-P05 双重门控）----

type stubVersionSource struct {
	version string
	err     error
}

func (s stubVersionSource) LatestVersion(context.Context) (string, error) {
	return s.version, s.err
}

func TestUpdateCheckGating(t *testing.T) {
	ctx := context.Background()

	// 1) 默认关：外呼禁止 → PANEL_004（I-P05）。
	svc, _ := newTestService(t)
	_, err := svc.UpdateCheck(ctx)
	wantSymbol(t, err, "PANEL_004")

	// 2) 开启但未装配版本源 → PANEL_005（A-P8）。
	if _, err := svc.UpdateConfig(ctx, adminActor(), UpdateConfigRequest{UpdateCheck: ptr(true)}); err != nil {
		t.Fatalf("enable update check: %v", err)
	}
	_, err = svc.UpdateCheck(ctx)
	wantSymbol(t, err, "PANEL_005")

	// 3) 装配 stub：成功比对（dev 构建视作 0.0.0 → 有更新）。
	svc2, _ := newTestService(t)
	svc2.source = stubVersionSource{version: "v1.2.3"}
	if _, err := svc2.UpdateConfig(ctx, adminActor(), UpdateConfigRequest{UpdateCheck: ptr(true)}); err != nil {
		t.Fatalf("enable: %v", err)
	}
	res, err := svc2.UpdateCheck(ctx)
	if err != nil {
		t.Fatalf("update check: %v", err)
	}
	if res.CurrentVersion != "dev" || res.LatestVersion == nil || *res.LatestVersion != "v1.2.3" || !res.UpdateAvailable {
		t.Fatalf("update check result mismatch: %+v", res)
	}

	// 4) 源失败 → PANEL_005（不自动重试，D-P09）。
	svc2.source = stubVersionSource{err: fmt.Errorf("upstream unreachable")}
	_, err = svc2.UpdateCheck(ctx)
	wantSymbol(t, err, "PANEL_005")
}

func TestHTTPVersionSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"latestVersion":"2.0.0"}`))
	}))
	defer srv.Close()
	ctx := context.Background()

	src := &HTTPVersionSource{Endpoint: srv.URL}
	v, err := src.LatestVersion(ctx)
	if err != nil || v != "2.0.0" {
		t.Fatalf("http source: v=%q err=%v", v, err)
	}
	if _, err := (&HTTPVersionSource{Endpoint: srv.URL + "/bad"}).LatestVersion(ctx); err == nil {
		t.Fatal("500 must fail")
	}
	if _, err := (&HTTPVersionSource{}).LatestVersion(ctx); err == nil {
		t.Fatal("empty endpoint must fail")
	}
}

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		a, b string
		want int
		fail bool
	}{
		{"1.2.3", "1.2.3", 0, false},
		{"v1.2.4", "1.2.3", 1, false},
		{"1.2", "1.2.0", 0, false},
		{"1.10.0", "1.9.9", 1, false},
		{"1.0.0", "1.0.0-rc.1", 1, false},           // 无先行版 > 有先行版
		{"1.0.0-rc.1", "1.0.0-rc.2", -1, false},     // 先行版数值比较
		{"1.0.0-rc.2", "1.0.0-rc.10", -1, false},    // 数值非字典序
		{"1.0.0-alpha", "1.0.0-alpha.1", -1, false}, // 标识符数量
		{"1.0.0-rc", "1.0.0-1", 1, false},           // 数字标识 < 字母数字标识
		{"1.2.3+build.5", "1.2.3", 0, false},        // build metadata 忽略
		{"x.y.z", "1.0.0", 0, true},
		{"1..2", "1.0.0", 0, true},
		{"", "1.0.0", 0, true},
	}
	for _, tt := range tests {
		got, err := CompareSemver(tt.a, tt.b)
		if tt.fail {
			if err == nil {
				t.Fatalf("CompareSemver(%q,%q) want error", tt.a, tt.b)
			}
			continue
		}
		if err != nil {
			t.Fatalf("CompareSemver(%q,%q): %v", tt.a, tt.b, err)
		}
		if got != tt.want {
			t.Fatalf("CompareSemver(%q,%q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

// ---- 菜单（D-P07）----

func TestMenuRegistryRoleFilter(t *testing.T) {
	svc, _ := newTestService(t)
	svc.Menus().Register(
		MenuItem{Key: "sites", Title: "网站", Path: "/sites", Icon: "globe", Order: 100,
			Visibility: []string{storeauth.RoleAdmin, storeauth.RoleReadonly}},
		MenuItem{Key: "admin-zone", Title: "管理", Path: "/admin", Icon: "shield", Order: 200,
			Visibility: []string{storeauth.RoleAdmin}},
		MenuItem{Key: "secret", Title: "无可见性声明", Path: "/x", Order: 50}, // 空集视作 admin 专属
	)

	admin := svc.MenuList(storeauth.RoleAdmin)
	if len(admin) != 3 {
		t.Fatalf("admin must see all: %d", len(admin))
	}
	// 排序：Order 升序（secret 50 < sites 100 < admin-zone 200）。
	if admin[0].Key != "secret" || admin[1].Key != "sites" || admin[2].Key != "admin-zone" {
		t.Fatalf("order wrong: %v,%v,%v", admin[0].Key, admin[1].Key, admin[2].Key)
	}

	ro := svc.MenuList(storeauth.RoleReadonly)
	if len(ro) != 1 || ro[0].Key != "sites" {
		t.Fatalf("readonly filter wrong: %+v", ro)
	}
	// 下发节点不含 visibility（服务端判定，ADR-018）。
	blob, _ := json.Marshal(ro)
	if strings.Contains(string(blob), "visibility") {
		t.Fatalf("menu payload leaks visibility: %s", blob)
	}
}

func TestMenuRegistryChildrenRecursion(t *testing.T) {
	svc, _ := newTestService(t)
	svc.Menus().Register(MenuItem{Key: "grp", Title: "组", Path: "/grp", Order: 10,
		Visibility: []string{storeauth.RoleAdmin, storeauth.RoleReadonly},
		Children: []MenuItem{
			{Key: "child-ro", Title: "可看", Path: "/grp/ro", Order: 1, Visibility: []string{storeauth.RoleReadonly}},
			{Key: "child-admin", Title: "专属", Path: "/grp/adm", Order: 2, Visibility: []string{storeauth.RoleAdmin}},
		}})

	ro := svc.MenuList(storeauth.RoleReadonly)
	if len(ro) != 1 || len(ro[0].Children) != 1 || ro[0].Children[0].Key != "child-ro" {
		t.Fatalf("children recursion filter wrong: %+v", ro)
	}
	adm := svc.MenuList(storeauth.RoleAdmin)
	if len(adm[0].Children) != 2 {
		t.Fatalf("admin children = %d", len(adm[0].Children))
	}
}

// ---- Detail 视图杂项 ----

func TestDetailSessionPassthrough(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := New(storepanel.New(db), WithSessionPolicy(SessionPolicy{
		AbsoluteTimeout: 3 * time.Hour, IdleTimeout: 30 * time.Minute,
	}))
	if _, err := svc.Bootstrap(context.Background()); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	d, err := svc.Detail(context.Background())
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if d.Session.AbsoluteTimeout != 3*3600 || d.Session.IdleTimeout != 30*60 {
		t.Fatalf("session passthrough wrong: %+v", d.Session)
	}
}

func TestDetailCertNullWhenAbsent(t *testing.T) {
	svc, _ := newTestService(t)
	d, err := svc.Detail(context.Background())
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if d.HTTPS.Cert != nil {
		t.Fatalf("cert must be null when absent: %+v", d.HTTPS.Cert)
	}
}

// I-P06：TLS 材料读取仅进程内消费（CertMaterial 可用性验证）。
func TestCertMaterialAfterSelfSigned(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(t)
	if _, _, err := svc.CertMaterial(ctx); err == nil {
		t.Fatal("cert material must be absent before first issue")
	}
	if _, err := svc.UpdateConfig(ctx, adminActor(), UpdateConfigRequest{HTTPSMode: ptr("self_signed")}); err != nil {
		t.Fatalf("self signed: %v", err)
	}
	certPEM, keyPEM, err := svc.CertMaterial(ctx)
	if err != nil {
		t.Fatalf("cert material: %v", err)
	}
	if !strings.Contains(certPEM, "BEGIN CERTIFICATE") || !strings.Contains(keyPEM, "BEGIN PRIVATE KEY") {
		t.Fatal("cert material PEM malformed")
	}
}

// 同值提交（无实际变更列）：幂等成功、零审计行（delta 仅记录真实变更）。
// 服务层不做角色判定（RequireRole 职责），identity 仅作审计 actor 传递。
func TestUpdateConfigNoOpSameValue(t *testing.T) {
	ctx := context.Background()
	svc, db := newTestService(t)
	view, err := svc.UpdateConfig(ctx, readonlyActor(), UpdateConfigRequest{Lang: ptr("zh-CN")})
	if err != nil {
		t.Fatalf("no-op same value: %v", err)
	}
	if view.Lang.Value != LangZhCN {
		t.Fatalf("view lang = %s", view.Lang.Value)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_log`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("no-op update must not audit: n=%d err=%v", n, err)
	}
}
