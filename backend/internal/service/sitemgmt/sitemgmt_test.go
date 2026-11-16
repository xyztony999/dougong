package sitemgmt

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
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
	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	"github.com/dougong/dougong/internal/store"
	storeauth "github.com/dougong/dougong/internal/store/auth"
	storesitemgmt "github.com/dougong/dougong/internal/store/sitemgmt"
)

// ---- 测试替身 ----

// fakeNginx 实现NginxGateway：文件系统原语落在真实临时目录（真实 fs 语义），
// TestConfig/Reload/Available 注入。
type fakeNginx struct {
	mu          sync.Mutex
	root        string // conf root
	testOK      bool
	testOutput  []string
	testErr     error
	reloadOut   string
	reloadErr   error
	reloadCalls int
	availErr    error
}

func newFakeNginx(t *testing.T) *fakeNginx {
	t.Helper()
	root := t.TempDir()
	for _, d := range ScanDirNames {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return &fakeNginx{root: root, testOK: true}
}

func (f *fakeNginx) Available() error { return f.availErr }

func (f *fakeNginx) PriorityDirs() []string {
	out := make([]string, len(ScanDirNames))
	for i, d := range ScanDirNames {
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

func (f *fakeNginx) WriteFileAtomic(path string, content []byte) error {
	return os.WriteFile(path, content, 0o644)
}

func (f *fakeNginx) WriteSecretFileAtomic(path string, content []byte) error {
	return os.WriteFile(path, content, 0o600)
}

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

func (f *fakeNginx) MkdirAll(dir string) error { return os.MkdirAll(dir, 0o755) }
func (f *fakeNginx) Exists(path string) bool   { _, err := os.Stat(path); return err == nil }
func (f *fakeNginx) RemoveFile(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (f *fakeNginx) FirstWritableDir() (string, error) {
	for _, d := range f.PriorityDirs() {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			return d, nil
		}
	}
	return "", fmt.Errorf("no writable dir")
}

func (f *fakeNginx) TestConfig(context.Context) (nginxx.TestResult, error) {
	if f.testErr != nil {
		return nginxx.TestResult{}, f.testErr
	}
	return nginxx.TestResult{OK: f.testOK, Output: f.testOutput}, nil
}

func (f *fakeNginx) Reload(context.Context) (string, error) {
	f.mu.Lock()
	f.reloadCalls++
	f.mu.Unlock()
	if f.reloadErr != nil {
		return f.reloadOut, f.reloadErr
	}
	return f.reloadOut, nil
}

// fakeAcme 注入签发结果。
type fakeAcme struct {
	mu        sync.Mutex
	res       *acme.IssueResult
	err       error
	calls     int
	domains   []string
	challenge string
	webroot   string
}

func (f *fakeAcme) IssueDNS(ctx context.Context, domains []string, provider string, cred acme.DnsCredential) (*acme.IssueResult, error) {
	return f.Issue(ctx, domains, "dns_01", "")
}

func (f *fakeAcme) Issue(_ context.Context, domains []string, challenge, webroot string) (*acme.IssueResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.domains, f.challenge, f.webroot = domains, challenge, webroot
	return f.res, f.err
}

// fakePhp 注入 pool 清单。
type fakePhp struct{ pools []phpfpm.Pool }

func (f *fakePhp) ListPools(context.Context) ([]phpfpm.Pool, error) { return f.pools, nil }

// recordingAudit 包装审计服务（计数 + 真库写入）。
type recordingAudit struct {
	inner   *audit.Service
	mu      sync.Mutex
	entries []audit.Entry
}

func (r *recordingAudit) Record(ctx context.Context, e audit.Entry) error {
	if err := r.inner.Record(ctx, e); err != nil {
		return err
	}
	r.mu.Lock()
	r.entries = append(r.entries, e)
	r.mu.Unlock()
	return nil
}

func (r *recordingAudit) snapshot() []audit.Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]audit.Entry(nil), r.entries...)
}

// testEnv 聚合 service 测试依赖。
type testEnv struct {
	svc     *Service
	gw      *fakeNginx
	acmeGw  *fakeAcme
	php     *fakePhp
	rec     *recordingAudit
	store   *storesitemgmt.Store
	dataDir string
	now     time.Time
}

func newEnv(t *testing.T) *testEnv {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	st := storesitemgmt.New(db)
	gw := newFakeNginx(t)
	dataDir := t.TempDir()
	env := &testEnv{
		gw: gw, store: st, dataDir: dataDir,
		acmeGw: &fakeAcme{}, php: &fakePhp{},
		now: time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC),
	}
	repo := storeauth.New(db)
	env.rec = &recordingAudit{inner: audit.New(repo.Audit(), func() time.Time { return env.now })}
	env.svc = New(gw, env.php, env.acmeGw, st, env.rec, Options{
		DataDir: dataDir,
		// 环境无关性：CI runner 可能装有 nginx（/var/log/nginx/* 真实存在），
		// 「回退链穷尽 → SITE_011」用例的默认回退路径指向必然不存在的测试路径。
		DefaultAccessLog: filepath.Join(dataDir, "no-such-access.log"),
		DefaultErrorLog:  filepath.Join(dataDir, "no-such-error.log"),
	})
	env.svc.clock = func() time.Time { return env.now }
	return env
}

// writeConf 写入配置文件（落 sites-enabled 或指定目录）。
func (e *testEnv) writeConf(t *testing.T, dir, name, content string) string {
	t.Helper()
	d := filepath.Join(e.gw.root, dir)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(d, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func readConf(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const adminActorName = "admin"

func adminActor() *authsvc.Identity {
	return &authsvc.Identity{UserID: 1, Username: adminActorName, Role: "admin"}
}

func wantSymbol(t *testing.T, err error, symbol string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error %s, got nil", symbol)
	}
	var ec *errcode.Error
	if !errors.As(err, &ec) || ec.Symbol() != symbol {
		t.Fatalf("err = %v, want symbol %s", err, symbol)
	}
}

// genCertPEM 生成自签证书 PEM（有效期注入；imported 纳管与 D-08 刷新测试用）。
func genCertPEM(t *testing.T, cn string, notBefore, notAfter time.Time) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: cn},
		NotBefore: notBefore, NotAfter: notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

// ---- 扫描 · 消重 · 自动纳管（D-01/D-03/A-SM5/A-SM17）----

func TestScanDedupeAndImport(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	env.writeConf(t, "sites-enabled", "a.conf",
		"server {\n    listen 80;\n    server_name app.example.com;\n}\n")
	// 同签名重复定义在低优先级目录（D-03：items 只保留 sites-enabled 一处）。
	env.writeConf(t, "conf.d", "b.conf",
		"server {\n    listen 80;\n    server_name app.example.com;\n}\n")
	env.writeConf(t, "conf.d", "c.conf",
		"server {\n    listen 8080;\n    server_name other.example.com;\n}\n")

	data, err := env.svc.ListSites(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if !data.NginxAvailable {
		t.Fatal("nginxAvailable should be true")
	}
	if len(data.Items) != 2 {
		t.Fatalf("items=%d want 2 (跨目录重复仅保留一处)", len(data.Items))
	}
	kept := data.Items[0]
	if kept.Blocks[0].SiteID == "" || !strings.HasSuffix(kept.SourceFile, filepath.Join("sites-enabled", "a.conf")) {
		t.Fatalf("kept item should anchor at sites-enabled: %+v", kept)
	}
	if len(data.Duplicates) != 1 || len(data.Duplicates[0].Files) != 2 {
		t.Fatalf("duplicates=%+v", data.Duplicates)
	}

	// enabled 筛选。
	all, _ := env.svc.ListSites(ctx, SiteEnabled)
	if len(all.Items) != 2 {
		t.Fatalf("enabled filter: %d", len(all.Items))
	}

	// 自动纳管（A-SM5）：ssl_cert 指向可解析 PEM → imported 行。
	certPEM := genCertPEM(t, "imported.example.com",
		env.now.Add(-24*time.Hour), env.now.Add(60*24*time.Hour))
	certPath := filepath.Join(env.dataDir, "legacy.pem")
	os.WriteFile(certPath, []byte(certPEM), 0o644)
	env.writeConf(t, "sites-enabled", "tls.conf",
		fmt.Sprintf("server {\n    listen 443 ssl;\n    server_name tls.example.com;\n    ssl_certificate %s;\n}\n", certPath))
	env.writeConf(t, "sites-enabled", "bad.conf",
		"server {\n    listen 443 ssl;\n    server_name bad.example.com;\n    ssl_certificate /nonexistent.pem;\n}\n")

	detail := func() []storesitemgmt.Certificate {
		rows, err := env.store.Cert().List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	env.svc.ListSites(ctx, "") // 触发扫描（含尾部纳管）
	rows := detail()
	if len(rows) != 1 {
		t.Fatalf("imported rows=%d want 1", len(rows))
	}
	if rows[0].Source != storesitemgmt.SourceImported || rows[0].PrimaryDomain != "tls.example.com" ||
		rows[0].OriginPath == nil || *rows[0].OriginPath != certPath {
		t.Fatalf("imported row: %+v", rows[0])
	}
	// 幂等：再次扫描不重复纳管；不可解析引用不纳管（certId null 如实）。
	env.svc.ListSites(ctx, "")
	if rows = detail(); len(rows) != 1 {
		t.Fatalf("re-scan rows=%d (must be idempotent)", len(rows))
	}

	// 列表项证书匹配（D-11）：tls.conf 站点 certId/certStatus 推算 valid。
	data, _ = env.svc.ListSites(ctx, "")
	var tlsItem *SiteItem
	for i := range data.Items {
		if len(data.Items[i].ServerNames) > 0 && data.Items[i].ServerNames[0] == "tls.example.com" {
			tlsItem = &data.Items[i]
		}
	}
	if tlsItem == nil {
		t.Fatal("tls site missing")
	}
	if tlsItem.CertRef == nil || *tlsItem.CertRef != certPath {
		t.Fatalf("certRef: %+v", tlsItem.CertRef)
	}
	if tlsItem.CertID == nil || *tlsItem.CertID != rows[0].ID {
		t.Fatalf("certId: %+v", tlsItem.CertID)
	}
	if tlsItem.CertStatus == nil || *tlsItem.CertStatus != storesitemgmt.StatusValid {
		t.Fatalf("certStatus: %+v", tlsItem.CertStatus)
	}
}

// nginx 缺席：空列表 + nginxAvailable=false（I-06 空态非错误）。
func TestListNginxUnavailableEmptyState(t *testing.T) {
	env := newEnv(t)
	env.gw.availErr = errors.New("nginx not found")
	env.writeConf(t, "sites-enabled", "a.conf", "server {\n    listen 80;\n}\n")
	data, err := env.svc.ListSites(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if data.NginxAvailable || len(data.Items) != 0 || len(data.Duplicates) != 0 {
		t.Fatalf("unavailable state: %+v", data)
	}
	// 写接口前置拒绝 SITE_012（I-12/D-15）。
	_, err = env.svc.DisableSite(context.Background(), adminActor(), "x#0")
	wantSymbol(t, err, "SITE_012")
}

// ---- WritePipeline：回滚 + diff + 备份保留（R-06/D-04）----

func TestPipelineRollbackAndBackup(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	p := env.writeConf(t, "sites-enabled", "a.conf",
		"server {\n    listen 80;\n    server_name a.example.com;\n    root /var/www/a;\n}\n")
	siteID := p + "#0"

	// nginx -t 失败 → 自动回滚 + SITE_003（errors+diff），reload 不执行。
	env.gw.testOK = false
	env.gw.testOutput = []string{"nginx: [emerg] unknown directive"}
	_, err := env.svc.UpdateSite(ctx, adminActor(), UpdateSiteRequest{
		SiteID: siteID, ServerNames: []string{"new.example.com"}, Listen: []string{"80"}, Root: "/var/www/a",
	})
	wantSymbol(t, err, "SITE_003")
	var ec *errcode.Error
	errors.As(err, &ec)
	data, _ := ec.Data().(SiteErrorData)
	if len(data.Errors) == 0 || !strings.Contains(data.Diff, "-") {
		t.Fatalf("SITE_003 data: %+v", ec.Data())
	}
	if got := readConf(t, p); !strings.Contains(got, "a.example.com") || strings.Contains(got, "new.example.com") {
		t.Fatalf("rollback failed, file=%s", got)
	}
	if env.gw.reloadCalls != 0 {
		t.Fatalf("reload after failed test: %d", env.gw.reloadCalls)
	}

	// 备份保留：成功写回 N 次 → 快照 ≤ CONF_BACKUP_KEEP。
	env.gw.testOK = true
	for i := 0; i < ConfBackupKeep+6; i++ {
		if _, err := env.svc.UpdateSite(ctx, adminActor(), UpdateSiteRequest{
			SiteID: siteID, ServerNames: []string{fmt.Sprintf("v%d.example.com", i)},
			Listen: []string{"80"}, Root: "/var/www/a",
		}); err != nil {
			t.Fatalf("update %d: %v", i, err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(env.dataDir, BackupDirName, "a.conf"))
	if err != nil {
		t.Fatal(err)
	}
	var backups int
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".conf") {
			backups++
		}
	}
	if backups > ConfBackupKeep {
		t.Fatalf("backups=%d > %d", backups, ConfBackupKeep)
	}
	if env.gw.reloadCalls != ConfBackupKeep+6 {
		t.Fatalf("reload calls=%d", env.gw.reloadCalls)
	}
}

// SITE_013：快照失败写入整体不执行（FR-01）。
func TestPipelineBackupFailureAborts(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	p := env.writeConf(t, "sites-enabled", "a.conf",
		"server {\n    listen 80;\n    server_name a.example.com;\n}\n")
	blockFile := filepath.Join(env.dataDir, "blocker") // 占位文件使 backupRoot MkdirAll 失败
	os.WriteFile(blockFile, []byte("x"), 0o644)
	env.svc.backupRoot = blockFile
	_, err := env.svc.UpdateSite(ctx, adminActor(), UpdateSiteRequest{
		SiteID: p + "#0", ServerNames: []string{"new.example.com"}, Listen: []string{"80"},
	})
	wantSymbol(t, err, "SITE_013")
	if got := readConf(t, p); !strings.Contains(got, "a.example.com") {
		t.Fatalf("file changed despite backup failure: %s", got)
	}
}

// reload 失败：配置保留 + SITE_004（D-05）。
func TestReloadFailureKeepsConfig(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	p := env.writeConf(t, "sites-enabled", "a.conf",
		"server {\n    listen 80;\n    server_name a.example.com;\n}\n")
	env.gw.reloadErr = errors.New("reload: exit status 1")
	env.gw.reloadOut = "nginx: [error] open() failed"
	req := UpdateSiteRequest{SiteID: p + "#0", ServerNames: []string{"new.example.com"}, Listen: []string{"80"}}
	data, err := env.svc.UpdateSite(ctx, adminActor(), req)
	if err == nil || data == nil {
		t.Fatalf("expected SITE_004 with data, got data=%v err=%v", data, err)
	}
	wantSymbol(t, err, "SITE_004")
	if got := readConf(t, p); !strings.Contains(got, "new.example.com") {
		t.Fatalf("config should be kept after reload failure: %s", got)
	}
	// 审计仍记 success（写入成功事实，A-SM7）。
	found := false
	for _, e := range env.rec.snapshot() {
		if e.Action == AuditActionSiteUpdated && e.Result == audit.ResultSuccess {
			found = true
		}
	}
	if !found {
		t.Fatal("site_updated success audit missing after reload failure")
	}
}

// 通道不可用（A-SM18）：TestConfig exec 错误 → 回滚 + SITE_012。
func TestPipelineChannelUnavailable(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	p := env.writeConf(t, "sites-enabled", "a.conf",
		"server {\n    listen 80;\n    server_name a.example.com;\n}\n")
	env.gw.testErr = nginxx.ErrUnavailable
	_, err := env.svc.UpdateSite(ctx, adminActor(), UpdateSiteRequest{
		SiteID: p + "#0", ServerNames: []string{"new.example.com"}, Listen: []string{"80"},
	})
	wantSymbol(t, err, "SITE_012")
	if got := readConf(t, p); !strings.Contains(got, "a.example.com") {
		t.Fatalf("rollback missing on channel error: %s", got)
	}
}

// ---- 结构化编辑：多站点隔离 + Raw 保全（I-08/I-09/I-10/R-16）----

func TestUpdateSiteIsolationAndRawPreservation(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	other := "# other site: untouched\nserver {\n    listen 80;\n    server_name keep.example.com;\n    if ($arg_x) {\n        return 404;\n    }\n}\n"
	p := env.writeConf(t, "sites-enabled", "a.conf", other+
		"server {\n    listen 80;\n    server_name a.example.com;\n    root /old;\n    add_header X-Custom 1;\n}\n")
	siteID := p + "#1"

	if _, err := env.svc.UpdateSite(ctx, adminActor(), UpdateSiteRequest{
		SiteID:      siteID,
		ServerNames: []string{"new.example.com", "alias.example.com"},
		Listen:      []string{"80", "443 ssl"},
		Root:        "/var/www/new",
	}); err != nil {
		t.Fatal(err)
	}
	got := readConf(t, p)
	if !strings.Contains(got, "server_name new.example.com alias.example.com;") {
		t.Fatalf("server_name edit missing:\n%s", got)
	}
	if !strings.Contains(got, "listen 443 ssl;") {
		t.Fatalf("listen edit missing:\n%s", got)
	}
	if !strings.Contains(got, "root /var/www/new;") {
		t.Fatalf("root edit missing:\n%s", got)
	}
	if !strings.Contains(got, "add_header X-Custom 1;") { // Raw 保全（I-09）
		t.Fatalf("raw directive lost:\n%s", got)
	}
	if !strings.Contains(got, "if ($arg_x) {") || !strings.Contains(got, "keep.example.com") {
		t.Fatalf("other site affected (R-16):\n%s", got)
	}
	// 其余站点 byte 级不变（R-16）：前缀逐字节比对。
	if !strings.HasPrefix(got, other) {
		t.Fatalf("other-site prefix changed (byte-level isolation broken)")
	}
	// 审计 site_updated fields。
	found := false
	for _, e := range env.rec.snapshot() {
		if e.Action == AuditActionSiteUpdated {
			found = true
		}
	}
	if !found {
		t.Fatal("site_updated audit missing")
	}

	// 校验：SITE_007/SITE_006/SITE_001/SITE_002。
	_, err := env.svc.UpdateSite(ctx, adminActor(), UpdateSiteRequest{
		SiteID: siteID, ServerNames: []string{"bad name"}, Listen: []string{"80"}})
	wantSymbol(t, err, "SITE_007")
	_, err = env.svc.UpdateSite(ctx, adminActor(), UpdateSiteRequest{
		SiteID: siteID, ServerNames: []string{"ok.example.com"}, Listen: []string{"99999"}})
	wantSymbol(t, err, "SITE_006")
	_, err = env.svc.UpdateSite(ctx, adminActor(), UpdateSiteRequest{
		SiteID: "bad-site-id", ServerNames: []string{"ok.example.com"}, Listen: []string{"80"}})
	wantSymbol(t, err, "SITE_001")
	_, err = env.svc.UpdateSite(ctx, adminActor(), UpdateSiteRequest{
		SiteID: p + "#9", ServerNames: []string{"ok.example.com"}, Listen: []string{"80"}})
	wantSymbol(t, err, "SITE_002")
}

// 反代参数写回（M3 验收主场景）+ 无反代段拒绝。
func TestUpdateProxy(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	p := env.writeConf(t, "sites-enabled", "proxy.conf",
		"server {\n    listen 80;\n    server_name p.example.com;\n    location / {\n"+
			"        proxy_pass http://127.0.0.1:9000;\n"+
			"        proxy_set_header Host $host;\n"+
			"    }\n}\n")
	noProxy := env.writeConf(t, "sites-enabled", "static.conf",
		"server {\n    listen 80;\n    server_name s.example.com;\n    root /var/www;\n}\n")

	ws := false
	_, err := env.svc.UpdateProxy(ctx, adminActor(), UpdateProxyRequest{
		SiteID: p + "#0", Upstream: "http://10.0.0.5:8080", HostHeader: "api.example.com", Websocket: &ws,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := readConf(t, p)
	if !strings.Contains(got, "proxy_pass http://10.0.0.5:8080;") ||
		!strings.Contains(got, "proxy_set_header Host api.example.com;") {
		t.Fatalf("proxy edit missing:\n%s", got)
	}

	// 开启 WS 透传 → 注入 Upgrade/Connection。
	ws = true
	if _, err := env.svc.UpdateProxy(ctx, adminActor(), UpdateProxyRequest{
		SiteID: p + "#0", Upstream: "http://10.0.0.5:8080"}); err != nil {
		t.Fatal(err)
	}
	got = readConf(t, p)
	if !strings.Contains(got, "proxy_set_header Upgrade $http_upgrade;") ||
		!strings.Contains(got, `proxy_set_header Connection "upgrade";`) {
		t.Fatalf("websocket headers missing:\n%s", got)
	}
	// 关闭 WS → 移除。
	ws = false
	if _, err := env.svc.UpdateProxy(ctx, adminActor(), UpdateProxyRequest{
		SiteID: p + "#0", Upstream: "http://10.0.0.5:8080", Websocket: &ws}); err != nil {
		t.Fatal(err)
	}
	if got = readConf(t, p); strings.Contains(got, "Upgrade") {
		t.Fatalf("websocket headers should be removed:\n%s", got)
	}
	// 上游形态 + 无反代段。
	_, err = env.svc.UpdateProxy(ctx, adminActor(), UpdateProxyRequest{
		SiteID: p + "#0", Upstream: "ftp://bad"})
	wantSymbol(t, err, "SITE_008")
	_, err = env.svc.UpdateProxy(ctx, adminActor(), UpdateProxyRequest{
		SiteID: noProxy + "#0", Upstream: "http://x:1"})
	wantSymbol(t, err, "SITE_002")
}

// ---- 启停：D-16 注释/还原 + siteId 稳定 + 幂等 ----

func TestToggleDisableEnable(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	orig := "server {\n    listen 80;\n    server_name t.example.com;\n}\n"
	p := env.writeConf(t, "sites-enabled", "t.conf", orig)
	siteID := p + "#0"

	data, err := env.svc.DisableSite(ctx, adminActor(), siteID)
	if err != nil || data.Enabled != SiteDisabled {
		t.Fatalf("disable: %+v err=%v", data, err)
	}
	got := readConf(t, p)
	if !strings.Contains(got, "# server {") || !strings.Contains(got, "#     listen 80;") {
		t.Fatalf("disable should comment out block:\n%s", got)
	}
	// 投影：dormant 站点仍可见，enabled=disabled，siteId 稳定（A-SM3）。
	list, _ := env.svc.ListSites(ctx, SiteDisabled)
	if len(list.Items) != 1 || list.Items[0].Blocks[0].SiteID != siteID {
		t.Fatalf("dormant projection: %+v", list.Items)
	}
	// 幂等：重复 disable 成功零写回。
	before := env.gw.reloadCalls
	if data, err = env.svc.DisableSite(ctx, adminActor(), siteID); err != nil || data.Enabled != SiteDisabled {
		t.Fatalf("idempotent disable: %+v err=%v", data, err)
	}
	if env.gw.reloadCalls != before {
		t.Fatal("idempotent disable should not write/reload")
	}
	// enable：byte 级还原（round-trip）。
	if data, err = env.svc.EnableSite(ctx, adminActor(), siteID); err != nil || data.Enabled != SiteEnabled {
		t.Fatalf("enable: %+v err=%v", data, err)
	}
	if got = readConf(t, p); got != orig {
		t.Fatalf("enable restore mismatch:\n got=%q\nwant=%q", got, orig)
	}
	list, _ = env.svc.ListSites(ctx, SiteEnabled)
	if len(list.Items) != 1 || list.Items[0].Blocks[0].SiteID != siteID {
		t.Fatalf("enabled projection: %+v", list.Items)
	}
}

// ---- 建站三模板 + D-15 防重 + 校验链 ----

// php_fpm 无 root 建站（Gate 2 F-1 回归锁）：契约 §4.2.4 root?(type=static
// 必填)——php_fpm root 可选，空则模板省略 root 指令；static 仍必填。
func TestCreatePHPSiteWithoutRoot(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	env.php.pools = []phpfpm.Pool{{Name: "www", Socket: "unix:/run/php/www.sock"}}

	data, err := env.svc.CreateSite(ctx, adminActor(), CreateSiteRequest{
		Type: SiteTypePHPFPM, ServerNames: []string{"noroot.example.com"},
		Listen: "80", PoolName: "www", // 不带 root
	})
	if err != nil {
		t.Fatalf("php_fpm create without root must pass (contract root?=static only): %v", err)
	}
	got := readConf(t, data.SourceFile)
	if strings.Contains(got, "root ") {
		t.Fatalf("empty root must omit directive:\n%s", got)
	}
	for _, want := range []string{"fastcgi_pass unix:/run/php/www.sock;", "try_files"} {
		if !strings.Contains(got, want) {
			t.Fatalf("php template missing %q:\n%s", want, got)
		}
	}
	// static 无 root 仍拒绝（契约必填项不变）。
	_, err = env.svc.CreateSite(ctx, adminActor(), CreateSiteRequest{
		Type: SiteTypeStatic, ServerNames: []string{"s2.example.com"}, Listen: "80"})
	wantSymbol(t, err, "AUTH_012")
}

func TestCreateSite(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	env.php.pools = []phpfpm.Pool{{Name: "www", Socket: "unix:/run/php/www.sock"}}
	www := filepath.Join(env.dataDir, "wwwroot")
	if err := os.MkdirAll(www, 0o755); err != nil {
		t.Fatal(err)
	}

	data, err := env.svc.CreateSite(ctx, adminActor(), CreateSiteRequest{
		Type: SiteTypePHPFPM, ServerNames: []string{"php.example.com"},
		Listen: "80", Root: www, PoolName: "www",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(data.SourceFile, "dougong-php.example.com.conf") {
		t.Fatalf("source file naming (D-03): %s", data.SourceFile)
	}
	got := readConf(t, data.SourceFile)
	for _, want := range []string{
		"server_name php.example.com;",
		"root " + www + ";",
		"try_files $uri $uri/ /index.php?$query_string;",
		"fastcgi_pass unix:/run/php/www.sock;",                              // pool socket 实况绑定（D-14）
		"fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;", // ⚠️A14 模板内置
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("php template missing %q:\n%s", want, got)
		}
	}
	// detail 投影：fastcgi location 呈现。
	d, err := env.svc.SiteDetail(ctx, data.SiteID)
	if err != nil || len(d.Locations) != 2 {
		t.Fatalf("detail locations: %+v err=%v", d, err)
	}

	// 反代模板（WS 透传默认值）。
	rp, err := env.svc.CreateSite(ctx, adminActor(), CreateSiteRequest{
		Type: SiteTypeReverseProx, ServerNames: []string{"rp.example.com"},
		Listen: "80", Upstream: "http://127.0.0.1:7000",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got = readConf(t, rp.SourceFile); !strings.Contains(got, "proxy_pass http://127.0.0.1:7000;") ||
		!strings.Contains(got, "proxy_set_header Upgrade $http_upgrade;") {
		t.Fatalf("reverse_proxy template:\n%s", got)
	}

	// 静态模板 root 必填 + 根目录不存在。
	_, err = env.svc.CreateSite(ctx, adminActor(), CreateSiteRequest{
		Type: SiteTypeStatic, ServerNames: []string{"s.example.com"}, Listen: "80"})
	wantSymbol(t, err, "AUTH_012")
	_, err = env.svc.CreateSite(ctx, adminActor(), CreateSiteRequest{
		Type: SiteTypeStatic, ServerNames: []string{"s.example.com"}, Listen: "80",
		Root: filepath.Join(env.dataDir, "missing-root")})
	wantSymbol(t, err, "AUTH_012")
	// pool 实况（SITE_009）。
	_, err = env.svc.CreateSite(ctx, adminActor(), CreateSiteRequest{
		Type: SiteTypePHPFPM, ServerNames: []string{"p2.example.com"}, Listen: "80",
		Root: www, PoolName: "ghost"})
	wantSymbol(t, err, "SITE_009")
	// type 非法。
	_, err = env.svc.CreateSite(ctx, adminActor(), CreateSiteRequest{
		Type: "magic", ServerNames: []string{"x.example.com"}, Listen: "80"})
	wantSymbol(t, err, "AUTH_012")
	// D-15 防重：同主域名+端口（含停用站点——停用≠删除）。
	if _, err := env.svc.DisableSite(ctx, adminActor(), data.SiteID); err != nil {
		t.Fatal(err)
	}
	_, err = env.svc.CreateSite(ctx, adminActor(), CreateSiteRequest{
		Type: SiteTypeStatic, ServerNames: []string{"php.example.com"}, Listen: "80",
		Root: www})
	wantSymbol(t, err, "SITE_005")
	// 审计 site_created。
	found := false
	for _, e := range env.rec.snapshot() {
		if e.Action == AuditActionSiteCreated && e.Detail.(map[string]any)["type"] == SiteTypePHPFPM {
			found = true
		}
	}
	if !found {
		t.Fatal("site_created audit missing")
	}
}

// ---- 源码编辑乐观锁（D-06/A-SM8）----

func TestSourceEditOptimisticLock(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	p := env.writeConf(t, "sites-enabled", "src.conf",
		"server {\n    listen 80;\n    server_name src.example.com;\n}\n")
	siteID := p + "#0"

	src, err := env.svc.ReadSource(ctx, siteID)
	if err != nil || src.Content == "" || src.BaseMtime == "" {
		t.Fatalf("read source: %+v err=%v", src, err)
	}
	// 外部修改 → 基线过期（SITE_010 + data.currentMtime）。
	time.Sleep(10 * time.Millisecond) // 确保 mtime 变化
	os.WriteFile(p, []byte(src.Content+"\n"), 0o644)
	_, err = env.svc.SaveSource(ctx, adminActor(), SourceSaveRequest{
		SiteID: siteID, Content: "server {\n    listen 80;\n}\n", BaseMtime: src.BaseMtime})
	wantSymbol(t, err, "SITE_010")

	// 非法语法拒收零落盘（A-SM8，SITE_003 语义）。
	fresh, _ := env.svc.ReadSource(ctx, siteID)
	_, err = env.svc.SaveSource(ctx, adminActor(), SourceSaveRequest{
		SiteID: siteID, Content: "server {\n  listen 80;\n", BaseMtime: fresh.BaseMtime})
	wantSymbol(t, err, "SITE_003")
	if readConf(t, p) != src.Content+"\n" {
		t.Fatal("invalid source must not be written")
	}
	// 超 1MiB（SITE_014）。
	fresh2, _ := env.svc.ReadSource(ctx, siteID)
	_, err = env.svc.SaveSource(ctx, adminActor(), SourceSaveRequest{
		SiteID: siteID, Content: strings.Repeat("#", SourceSizeLimit+1), BaseMtime: fresh2.BaseMtime})
	wantSymbol(t, err, "SITE_014")
	// 正常保存：内容生效 + 新基线返回。
	fresh3, _ := env.svc.ReadSource(ctx, siteID)
	newContent := "server {\n    listen 8080;\n    server_name src.example.com;\n}\n"
	saved, err := env.svc.SaveSource(ctx, adminActor(), SourceSaveRequest{
		SiteID: siteID, Content: newContent, BaseMtime: fresh3.BaseMtime})
	if err != nil {
		t.Fatal(err)
	}
	if readConf(t, p) != newContent {
		t.Fatalf("saved content mismatch: %s", readConf(t, p))
	}
	// 保存后的新基线可用（二次保存零冲突）。
	if _, err := env.svc.SaveSource(ctx, adminActor(), SourceSaveRequest{
		SiteID: siteID, Content: newContent, BaseMtime: saved.BaseMtime}); err != nil {
		t.Fatalf("second save with new baseline: %v", err)
	}
}

// ---- 日志尾部：路径解析链（⚠️A15/D-07）----

func TestLogTail(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	logPath := filepath.Join(env.dataDir, "access.log")
	var sb strings.Builder
	for i := 1; i <= 250; i++ {
		fmt.Fprintf(&sb, "2026-11-15T10:00:%02dZ line %d\n", i%60, i)
	}
	os.WriteFile(logPath, []byte(sb.String()), 0o644)
	errLog := filepath.Join(env.dataDir, "error.log")
	os.WriteFile(errLog, []byte("2026/11/15 10:00:00 [error] 1#1 boom\nplain line\n"), 0o644)

	p := env.writeConf(t, "sites-enabled", "log.conf",
		fmt.Sprintf("server {\n    listen 80;\n    server_name log.example.com;\n"+
			"    access_log %s;\n    error_log %s;\n}\n", logPath, errLog))

	// 默认 100 行 + truncated。
	data, err := env.svc.TailLog(ctx, p+"#0", LogTypeAccess, "")
	if err != nil {
		t.Fatal(err)
	}
	if data.LogPath != logPath || len(data.Lines) != LogTailDefault || !data.Truncated {
		t.Fatalf("tail default: lines=%d truncated=%v", len(data.Lines), data.Truncated)
	}
	if !strings.HasSuffix(data.Lines[len(data.Lines)-1].Text, "line 250") {
		t.Fatalf("tail should return last N lines, got %q", data.Lines[len(data.Lines)-1].Text)
	}
	// 上限钳制：请求 5000 → 钳至 1000，但文件仅 250 行 → 全量返回且非截断；
	// 请求 200 < 文件行数 → 截断为 200。
	data, _ = env.svc.TailLog(ctx, p+"#0", LogTypeAccess, "5000")
	if len(data.Lines) != 250 || data.Truncated {
		t.Fatalf("cap beyond file: n=%d truncated=%v", len(data.Lines), data.Truncated)
	}
	data, _ = env.svc.TailLog(ctx, p+"#0", LogTypeAccess, "200")
	if len(data.Lines) != 200 || !data.Truncated {
		t.Fatalf("partial tail: n=%d truncated=%v", len(data.Lines), data.Truncated)
	}
	// error log 时间戳拆解 + 无时间戳行原样。
	data, _ = env.svc.TailLog(ctx, p+"#0", LogTypeError, "10")
	if len(data.Lines) != 2 {
		t.Fatalf("error lines: %d", len(data.Lines))
	}
	if data.Lines[0].TS == nil || data.Lines[1].TS != nil {
		t.Fatalf("ts extraction: %+v", data.Lines)
	}
	if !strings.Contains(data.Lines[0].Text, "[error]") {
		t.Fatalf("error text: %+v", data.Lines[0])
	}
	// 回退链穷尽 → SITE_011（无 access_log 指令且默认路径不存在）。
	q := env.writeConf(t, "sites-enabled", "nolog.conf",
		"server {\n    listen 80;\n    server_name nolog.example.com;\n}\n")
	_, err = env.svc.TailLog(ctx, q+"#0", LogTypeAccess, "")
	wantSymbol(t, err, "SITE_011")
	// 参数校验。
	_, err = env.svc.TailLog(ctx, p+"#0", "bogus", "")
	wantSymbol(t, err, "AUTH_012")
	_, err = env.svc.TailLog(ctx, p+"#0", LogTypeAccess, "x")
	wantSymbol(t, err, "AUTH_012")
}

// ---- 证书：状态推算（D-08）/ 防重（D-10）/ 引用（D-11）/ 六接口 ----

func issueResult(notAfter time.Time) *acme.IssueResult {
	return &acme.IssueResult{
		CertPEM: []byte("CERT-PEM"), KeyPEM: []byte("KEY-PEM"),
		NotBefore: notAfter.Add(-90 * 24 * time.Hour), NotAfter: notAfter,
		Issuer: "R11", AccountURI: "https://acme.example/acct/1",
	}
}

func TestCertRequestFlowAndDup(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	p := env.writeConf(t, "sites-enabled", "acme.conf",
		"server {\n    listen 80;\n    server_name acme.example.com www.acme.example.com;\n}\n")
	siteID := p + "#0"

	// 签发成功：pending → valid + 托管文件 + 同事务审计。
	env.acmeGw.res = issueResult(env.now.Add(90 * 24 * time.Hour))
	data, err := env.svc.CertRequest(ctx, adminActor(), CertRequestRequest{
		SiteID: siteID, Challenge: storesitemgmt.ChallengeHTTP01})
	if err != nil {
		t.Fatal(err)
	}
	if data.Status != storesitemgmt.StatusValid {
		t.Fatalf("status: %+v", data)
	}
	row, _ := env.store.Cert().GetByID(ctx, data.CertID)
	if row.Status != storesitemgmt.StatusValid || row.Issuer != "R11" ||
		row.AcmeAccountRef == nil || *row.AcmeAccountRef != "https://acme.example/acct/1" {
		t.Fatalf("row after issue: %+v", row)
	}
	full, key := env.svc.certStoragePaths("acme.example.com")
	if b, _ := os.ReadFile(full); string(b) != "CERT-PEM" {
		t.Fatalf("managed cert file: %q", b)
	}
	if _, serr := os.Stat(key); serr != nil {
		t.Fatalf("key file must exist (D-09): %v", serr) // 0600 权限位归 Linux 目标环境验证（Windows 无 POSIX perm，A-F7 同判）
	}
	// http-01 临时 location 已移除（A-SM10：恢复原样）。
	if got := readConf(t, p); strings.Contains(got, "acme-challenge") {
		t.Fatalf("challenge location not removed:\n%s", got)
	}
	// 终态落库与审计同事务（D-12 例外）——经审计查询通道断言。
	qrows, _, qerr := env.rec.inner.Query(ctx, storeauth.AuditCondition{Action: AuditActionCertRequested}, 1, 20)
	if qerr != nil {
		t.Fatal(qerr)
	}
	found := false
	for _, e := range qrows {
		if e.Result == audit.ResultSuccess {
			found = true
		}
	}
	if !found {
		t.Fatal("cert_requested success audit missing")
	}

	// I-03 同域名防重（CERT_003 data.existingCertId）。
	_, err = env.svc.CertRequest(ctx, adminActor(), CertRequestRequest{
		SiteID: siteID, Challenge: storesitemgmt.ChallengeTLSALPN})
	wantSymbol(t, err, "CERT_003")
	// challenge 非法（CERT_007）。
	q := env.writeConf(t, "sites-enabled", "acme2.conf",
		"server {\n    listen 80;\n    server_name other.example.com;\n}\n")
	// v2.0：dns_01 合法但缺凭证 → CERT_009；非法枚举 → CERT_007。
	_, err = env.svc.CertRequest(ctx, adminActor(), CertRequestRequest{
		SiteID: q + "#0", Challenge: "dns_01"})
	wantSymbol(t, err, "CERT_009")
	_, err = env.svc.CertRequest(ctx, adminActor(), CertRequestRequest{
		SiteID: q + "#0", Challenge: "bogus"})
	wantSymbol(t, err, "CERT_007")

	// 签发失败：renew_failed + last_error + CERT_004 + failure 审计（同事务）。
	env.acmeGw.err = errors.New("challenge connection refused")
	_, err = env.svc.CertRequest(ctx, adminActor(), CertRequestRequest{
		SiteID: q + "#0", Challenge: storesitemgmt.ChallengeHTTP01})
	wantSymbol(t, err, "CERT_004")
	rows, _ := env.store.Cert().List(ctx)
	var failed *storesitemgmt.Certificate
	for i := range rows {
		if rows[i].PrimaryDomain == "other.example.com" {
			failed = &rows[i]
		}
	}
	if failed == nil || failed.Status != storesitemgmt.StatusRenewFailed ||
		failed.LastError == nil || !strings.Contains(*failed.LastError, "connection refused") {
		t.Fatalf("failed row: %+v", failed)
	}
	qrows2, _, qerr2 := env.rec.inner.Query(ctx, storeauth.AuditCondition{Action: AuditActionCertRequested}, 1, 20)
	if qerr2 != nil {
		t.Fatal(qerr2)
	}
	failedAudit := false
	for _, e := range qrows2 {
		if e.Result == audit.ResultFailure {
			failedAudit = true
		}
	}
	if !failedAudit {
		t.Fatal("cert_requested failure audit missing")
	}

	// retry-renewal：非 renew_failed → CERT_002；renew_failed → 成功转 valid。
	_, err = env.svc.CertRetryRenewal(ctx, adminActor(), data.CertID)
	wantSymbol(t, err, "CERT_002")
	env.acmeGw.err = nil
	env.acmeGw.res = issueResult(env.now.Add(120 * 24 * time.Hour))
	rd, err := env.svc.CertRetryRenewal(ctx, adminActor(), failed.ID)
	if err != nil || rd.Status != storesitemgmt.StatusValid {
		t.Fatalf("retry: %+v err=%v", rd, err)
	}
	row, _ = env.store.Cert().GetByID(ctx, failed.ID)
	if row.Status != storesitemgmt.StatusValid || row.LastError != nil {
		t.Fatalf("row after retry: %+v", row)
	}
	// 不存在（CERT_001）。
	_, err = env.svc.CertRetryRenewal(ctx, adminActor(), 999)
	wantSymbol(t, err, "CERT_001")
}

func TestCertDeployUndeployDelete(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	// 部署目标站点：已有旧证书引用（undeploy 恢复原值语义，D-11）。
	p := env.writeConf(t, "sites-enabled", "dep.conf",
		"server {\n    listen 443 ssl;\n    server_name dep.example.com;\n"+
			"    ssl_certificate /old/fullchain.pem;\n    ssl_certificate_key /old/key.pem;\n}\n")
	siteID := p + "#0"

	env.acmeGw.res = issueResult(env.now.Add(90 * 24 * time.Hour))
	// 经由 request 建证书行（同步终态）。
	reqSite := env.writeConf(t, "sites-enabled", "dep-acme.conf",
		"server {\n    listen 80;\n    server_name dep.example.com;\n}\n")
	// 先防重：同域名只能一张——目标站点与申请站点同域名会撞 I-03。申请用别名域。
	_ = reqSite
	acmeSite := env.writeConf(t, "sites-enabled", "acme-src.conf",
		"server {\n    listen 80;\n    server_name cert.example.com;\n}\n")
	data, err := env.svc.CertRequest(ctx, adminActor(), CertRequestRequest{
		SiteID: acmeSite + "#0", Challenge: storesitemgmt.ChallengeHTTP01})
	if err != nil {
		t.Fatal(err)
	}
	certID := data.CertID
	row, _ := env.store.Cert().GetByID(ctx, certID)

	// deploy：写回 ssl_cert 引用 + 快照原值 + reload。
	dep, err := env.svc.CertDeploy(ctx, adminActor(), CertRefRequest{CertID: certID, SiteID: siteID})
	if err != nil {
		t.Fatal(err)
	}
	if dep.CertRef != *row.StoragePath {
		t.Fatalf("deploy ref: %+v", dep)
	}
	got := readConf(t, p)
	if !strings.Contains(got, "ssl_certificate "+*row.StoragePath+";") ||
		!strings.Contains(got, "ssl_certificate_key "+certKeyPathOf(*row.StoragePath)+";") {
		t.Fatalf("deploy write-back missing:\n%s", got)
	}
	after, _ := env.store.Cert().GetByID(ctx, certID)
	if ref, ok := after.DeployPrevRefs[siteID]; !ok || ref.Cert != "/old/fullchain.pem" || ref.Key != "/old/key.pem" {
		t.Fatalf("deploy_prev_refs: %+v", after.DeployPrevRefs)
	}
	// 幂等：重复 deploy 直接成功。
	if _, err := env.svc.CertDeploy(ctx, adminActor(), CertRefRequest{CertID: certID, SiteID: siteID}); err != nil {
		t.Fatalf("idempotent deploy: %v", err)
	}

	// 引用守卫（CERT_005 data.sites）。
	_, err = env.svc.CertDelete(ctx, adminActor(), certID)
	wantSymbol(t, err, "CERT_005")
	var ec *errcode.Error
	errors.As(err, &ec)
	if d, ok := ec.Data().(CertRefSitesData); !ok || len(d.Sites) != 1 || d.Sites[0].SiteID != siteID {
		t.Fatalf("CERT_005 data: %+v", ec.Data())
	}

	// undeploy：恢复原引用值 + 清键。
	und, err := env.svc.CertUndeploy(ctx, adminActor(), CertRefRequest{CertID: certID, SiteID: siteID})
	if err != nil || und.CertRef != "/old/fullchain.pem" {
		t.Fatalf("undeploy: %+v err=%v", und, err)
	}
	if got = readConf(t, p); !strings.Contains(got, "ssl_certificate /old/fullchain.pem;") {
		t.Fatalf("undeploy restore missing:\n%s", got)
	}
	after, _ = env.store.Cert().GetByID(ctx, certID)
	if len(after.DeployPrevRefs) != 0 {
		t.Fatalf("refs after undeploy: %+v", after.DeployPrevRefs)
	}
	// 第二次 undeploy → CERT_006（如实）。
	_, err = env.svc.CertUndeploy(ctx, adminActor(), CertRefRequest{CertID: certID, SiteID: siteID})
	wantSymbol(t, err, "CERT_006")

	// 删除：acme → 删行 + 托管文件移除（I-04）。
	if _, err := env.svc.CertDelete(ctx, adminActor(), certID); err != nil {
		t.Fatal(err)
	}
	if _, err := env.store.Cert().GetByID(ctx, certID); err == nil {
		t.Fatal("row should be deleted")
	}
	full, _ := env.svc.certStoragePaths("cert.example.com")
	if _, serr := os.Stat(full); !os.IsNotExist(serr) {
		t.Fatal("managed cert file should be removed")
	}
	// 重复删除 → CERT_001（如实）。
	_, err = env.svc.CertDelete(ctx, adminActor(), certID)
	wantSymbol(t, err, "CERT_001")

	// imported 引用同样受守卫与推导约束（D-11：origin_path 匹配）。
	certPEM := genCertPEM(t, "legacy.example.com", env.now.Add(-time.Hour), env.now.Add(30*24*time.Hour))
	legacy := filepath.Join(env.dataDir, "legacy.pem")
	os.WriteFile(legacy, []byte(certPEM), 0o644)
	l := env.writeConf(t, "sites-enabled", "legacy.conf",
		fmt.Sprintf("server {\n    listen 443 ssl;\n    server_name legacy.example.com;\n    ssl_certificate %s;\n}\n", legacy))
	env.svc.ListSites(ctx, "") // 自动纳管
	rows, _ := env.store.Cert().List(ctx)
	var importedID int64
	for _, r := range rows {
		if r.Source == storesitemgmt.SourceImported {
			importedID = r.ID
		}
	}
	if importedID == 0 {
		t.Fatal("imported row missing")
	}
	_, err = env.svc.CertDelete(ctx, adminActor(), importedID)
	wantSymbol(t, err, "CERT_005")
	// 改掉引用后可删（imported 仅删行，源文件不动 ⚠️A16）。
	os.WriteFile(l, []byte("server {\n    listen 443 ssl;\n    server_name legacy.example.com;\n}\n"), 0o644)
	if _, err := env.svc.CertDelete(ctx, adminActor(), importedID); err != nil {
		t.Fatalf("imported delete: %v", err)
	}
	if _, serr := os.Stat(legacy); serr != nil {
		t.Fatalf("imported origin file must stay (⚠️A16): %v", serr)
	}
}

// 状态读时推算（D-08）：valid/expiring_soon/expired 由 not_after 即时推算；
// imported 面板外替换后元数据刷新。
func TestCertStatusInference(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	mk := func(daysToExpiry int) int64 {
		id, err := env.store.Cert().Create(ctx, storesitemgmt.Certificate{
			PrimaryDomain: fmt.Sprintf("d%d.example.com", daysToExpiry),
			Domains:       []string{fmt.Sprintf("d%d.example.com", daysToExpiry)},
			Source:        storesitemgmt.SourceAcme, Status: storesitemgmt.StatusValid,
			Challenge: strpLocal(storesitemgmt.ChallengeHTTP01),
			NotAfter:  tmPtr(env.now.Add(time.Duration(daysToExpiry) * 24 * time.Hour)),
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	check := func(days int, want string) {
		t.Helper()
		row, _ := env.store.Cert().GetByID(ctx, mk(days))
		if got := env.svc.computeStatus(row, env.now); got != want {
			t.Fatalf("days=%d status=%s want=%s", days, got, want)
		}
	}
	check(90, storesitemgmt.StatusValid)
	check(30, storesitemgmt.StatusExpiringSoon)
	check(10, storesitemgmt.StatusExpiringSoon)
	check(-1, storesitemgmt.StatusExpired)
	check(0, storesitemgmt.StatusExpired) // 边界：now >= notAfter → expired

	// imported 读时刷新：文件替换为临期证书后呈现随之变化（不落库）。
	certPEM := genCertPEM(t, "imp.example.com",
		env.now.Add(-time.Hour), env.now.Add(10*24*time.Hour))
	impPath := filepath.Join(env.dataDir, "imp.pem")
	os.WriteFile(impPath, []byte(certPEM), 0o644)
	id, _ := env.store.Cert().Create(ctx, storesitemgmt.Certificate{
		PrimaryDomain: "imp.example.com", Domains: []string{"imp.example.com"},
		Source: storesitemgmt.SourceImported, Status: storesitemgmt.StatusValid,
		OriginPath: &impPath,
	})
	row, _ := env.store.Cert().GetByID(ctx, id)
	c := env.svc.computeCert(row, env.now)
	if c.Status != storesitemgmt.StatusExpiringSoon || c.NotAfter == nil ||
		!c.NotAfter.Equal(env.now.Add(10*24*time.Hour).Truncate(time.Second)) {
		t.Fatalf("imported refresh: %+v", c)
	}
	if again, _ := env.store.Cert().GetByID(ctx, id); again.NotAfter != nil {
		t.Fatalf("imported refresh must not persist (D-08): %+v", again.NotAfter)
	}

	// cert/list 筛选 + 排序（临期在前，D-08/#3）。
	data, err := env.svc.CertList(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Items) < 4 {
		t.Fatalf("items=%d", len(data.Items))
	}
	for i := 1; i < len(data.Items); i++ {
		a, b := data.Items[i-1].NotAfter, data.Items[i].NotAfter
		if a != nil && b != nil && *a > *b {
			t.Fatalf("sort broken at %d: %s > %s", i, *a, *b)
		}
	}
	expiring, _ := env.svc.CertList(ctx, storesitemgmt.SourceAcme, storesitemgmt.StatusExpiringSoon)
	if len(expiring.Items) != 2 { // d10 + d30（30 天边界含于 expiring_soon）
		t.Fatalf("status filter: %+v", expiring.Items)
	}
}

// ---- 自动续期调度（4.1.3/A-SM15）----

func TestRenewalScheduler(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	// expiring_soon 证书（acme，含部署引用站点）。
	expiring := storesitemgmt.Certificate{
		PrimaryDomain: "renew.example.com", Domains: []string{"renew.example.com"},
		Source: storesitemgmt.SourceAcme, Status: storesitemgmt.StatusValid,
		Challenge: strpLocal(storesitemgmt.ChallengeHTTP01),
		NotAfter:  tmPtr(env.now.Add(5 * 24 * time.Hour)),
	}
	full, _ := env.svc.certStoragePaths("renew.example.com")
	expiring.StoragePath = &full
	id, err := env.store.Cert().Create(ctx, expiring)
	if err != nil {
		t.Fatal(err)
	}
	// 非 expiring_soon 证书（不应被调度触碰）。
	healthy, _ := env.store.Cert().Create(ctx, storesitemgmt.Certificate{
		PrimaryDomain: "healthy.example.com", Domains: []string{"healthy.example.com"},
		Source: storesitemgmt.SourceAcme, Status: storesitemgmt.StatusValid,
		Challenge: strpLocal(storesitemgmt.ChallengeHTTP01),
		NotAfter:  tmPtr(env.now.Add(200 * 24 * time.Hour)),
	})

	// 单轮成功：status → valid + 新有效期 + cert_renewed 审计（system）。
	env.acmeGw.res = issueResult(env.now.Add(150 * 24 * time.Hour))
	env.svc.runRenewalPass(ctx)
	row, _ := env.store.Cert().GetByID(ctx, id)
	if row.Status != storesitemgmt.StatusValid || row.NotAfter == nil ||
		!row.NotAfter.Equal(env.now.Add(150*24*time.Hour)) {
		t.Fatalf("renewed row: %+v", row)
	}
	hrow, _ := env.store.Cert().GetByID(ctx, healthy)
	if hrow.NotAfter == nil || !hrow.NotAfter.Equal(env.now.Add(200*24*time.Hour)) {
		t.Fatalf("healthy cert must be untouched: %+v", hrow)
	}
	// 同事务审计不经 recorder 包装——经审计查询通道断言（D-12 例外）。
	qrows, _, qerr := env.rec.inner.Query(ctx, storeauth.AuditCondition{Action: AuditActionCertRenewed}, 1, 20)
	if qerr != nil {
		t.Fatal(qerr)
	}
	found := false
	for _, e := range qrows {
		if e.Source == audit.SourceSystem && e.ActorName == "system" {
			found = true
		}
	}
	if !found {
		t.Fatal("cert_renewed system audit missing")
	}

	// 失败轮：转 renew_failed + last_error（单次尝试不重试）。
	row.NotAfter = tmPtr(env.now.Add(3 * 24 * time.Hour))
	env.store.Cert().UpdateRenewalMeta(ctx, id, storesitemgmt.StatusExpiringSoon, nil, nil, row.NotAfter, "")
	env.acmeGw.err = errors.New("ca unreachable")
	env.svc.runRenewalPass(ctx)
	row, _ = env.store.Cert().GetByID(ctx, id)
	if row.Status != storesitemgmt.StatusRenewFailed || row.LastError == nil {
		t.Fatalf("failed renewal row: %+v", row)
	}

	// 调度循环退出路径（G-06）：已取消 ctx 立即返回。
	cctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	go func() { done <- env.svc.RunRenewalScheduler(cctx) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("scheduler exit err: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler did not exit on canceled ctx")
	}
}

// ---- 纯函数表驱动 ----

func TestValidateHelpers(t *testing.T) {
	domains := map[string]bool{
		"a.example.com": true, "*.example.com": true, "localhost": true,
		"": false, "a b.com": false, "a**b.com": false, "*x.example.com": false,
		"a.*.com": false, strings.Repeat("a", 254) + ".com": false,
	}
	for d, want := range domains {
		if got := validDomain(d); got != want {
			t.Errorf("validDomain(%q)=%v want=%v", d, got, want)
		}
	}
	if validateDomainList(nil) == nil {
		t.Error("empty domain list must fail")
	}
	listens := map[string]bool{
		"80": true, "443 ssl": true, "127.0.0.1:8080": true, "[::]:80 default_server": true,
		"0": false, "70000": false, "abc": false, "": false, "80 x;y": false, "unix:/s": false,
	}
	for l, want := range listens {
		got := validateListenDecl(l) == nil
		if got != want {
			t.Errorf("validateListen(%q)=%v want=%v", l, got, want)
		}
	}
	upstreams := map[string]bool{
		"http://127.0.0.1:8080": true, "https://up.example.com": true,
		"http://up/path": true, "ftp://x": false, "http://": false,
		"http://a b": false, "": false, "http://x:99999": false,
	}
	for u, want := range upstreams {
		got := validateUpstream(u) == nil
		if got != want {
			t.Errorf("validateUpstream(%q)=%v want=%v", u, got, want)
		}
	}
	if got := normalizeDomain("*.My Site.Example.COM"); got != "my-site.example.com" {
		t.Fatalf("normalizeDomain=%q", got)
	}
	if !domainSetEqual([]string{"a.com", "B.com"}, []string{"b.COM", "A.com"}) {
		t.Fatal("domainSetEqual should be order/case insensitive")
	}
	if domainSetEqual([]string{"a.com"}, []string{"a.com", "b.com"}) {
		t.Fatal("domainSetEqual size mismatch")
	}
	// siteId 解析（D-02/SITE_001）。
	if _, _, err := parseSiteID("/etc/nginx/a.conf#0"); err != nil {
		t.Fatalf("valid id: %v", err)
	}
	for _, bad := range []string{"nohash", "#0", "a.conf#", "a.conf#x", "a.conf#-1"} {
		if _, _, err := parseSiteID(bad); err == nil {
			t.Errorf("parseSiteID(%q) should fail", bad)
		}
	}
}

// reload 独立接口（D-05：失败亦记 failure 审计）。
func TestNginxReloadEndpoint(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	data, err := env.svc.NginxReload(ctx, adminActor())
	if err != nil || !data.Reloaded {
		t.Fatalf("reload: %+v err=%v", data, err)
	}
	env.gw.reloadErr = errors.New("exit 1")
	env.gw.reloadOut = "nginx: [error] reload failed"
	_, err = env.svc.NginxReload(ctx, adminActor())
	wantSymbol(t, err, "SITE_004")
	fail := false
	for _, e := range env.rec.snapshot() {
		if e.Action == AuditActionNginxReloaded && e.Result == audit.ResultFailure {
			fail = true
		}
	}
	if !fail {
		t.Fatal("nginx_reloaded failure audit missing")
	}
}

// php-pools 实时读取（D-14）。
func TestListPhpPools(t *testing.T) {
	env := newEnv(t)
	env.php.pools = []phpfpm.Pool{{Name: "www", Socket: "unix:/run/php/www.sock"}}
	data, err := env.svc.ListPhpPools(context.Background())
	if err != nil || len(data.Pools) != 1 || data.Pools[0].Socket != "unix:/run/php/www.sock" {
		t.Fatalf("pools: %+v err=%v", data, err)
	}
	env.php.pools = nil
	data, _ = env.svc.ListPhpPools(context.Background())
	if data.Pools == nil || len(data.Pools) != 0 {
		t.Fatalf("empty pools must be [] not nil (FR-15)")
	}
}

// 工具函数。
func strpLocal(s string) *string { return &s }

func tmPtr(t time.Time) *time.Time { return &t }

// ---- Gate 2 第 1 轮补测（W-2 覆盖率收敛 + F-1 相邻分支）----

// dormant 块结构化编辑：编辑在剥离文本子树进行、写回整体重新注释，
// enabled 投影不变（D-16 + A-SM2）。
func TestDormantStructuredEdit(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	p := env.writeConf(t, "sites-enabled", "d.conf",
		"server {\n    listen 80;\n    server_name d.example.com;\n    location / {\n"+
			"        proxy_pass http://127.0.0.1:1234;\n    }\n}\n")
	siteID := p + "#0"
	if _, err := env.svc.DisableSite(ctx, adminActor(), siteID); err != nil {
		t.Fatal(err)
	}

	// dormant 上结构化编辑字段。
	if _, err := env.svc.UpdateSite(ctx, adminActor(), UpdateSiteRequest{
		SiteID: siteID, ServerNames: []string{"d2.example.com"}, Listen: []string{"80"},
	}); err != nil {
		t.Fatalf("dormant update: %v", err)
	}
	got := readConf(t, p)
	if !strings.Contains(got, "# server {") {
		t.Fatalf("site must stay dormant after edit (D-16):\n%s", got)
	}
	if !strings.Contains(got, "#     server_name d2.example.com;") {
		t.Fatalf("edited directive missing in dormant block:\n%s", got)
	}
	// dormant 上反代参数编辑。
	if _, err := env.svc.UpdateProxy(ctx, adminActor(), UpdateProxyRequest{
		SiteID: siteID, Upstream: "http://10.9.9.9:9"}); err != nil {
		t.Fatalf("dormant proxy update: %v", err)
	}
	if got = readConf(t, p); !strings.Contains(got, "#         proxy_pass http://10.9.9.9:9;") {
		t.Fatalf("dormant proxy edit missing:\n%s", got)
	}
	// enable 后为活动态且编辑生效。
	if _, err := env.svc.EnableSite(ctx, adminActor(), siteID); err != nil {
		t.Fatal(err)
	}
	got = readConf(t, p)
	if strings.Contains(got, "# server") || !strings.Contains(got, "proxy_pass http://10.9.9.9:9;") {
		t.Fatalf("enable should activate edited block:\n%s", got)
	}
}

// 建站管线失败：新文件回滚=删除（rollback !existed 分支）。
func TestCreateRollbackRemovesNewFile(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	env.php.pools = []phpfpm.Pool{{Name: "www", Socket: "unix:/run/php/www.sock"}}
	env.gw.testOK = false
	_, err := env.svc.CreateSite(ctx, adminActor(), CreateSiteRequest{
		Type: SiteTypePHPFPM, ServerNames: []string{"rb.example.com"},
		Listen: "80", PoolName: "www",
	})
	wantSymbol(t, err, "SITE_003")
	matches, _ := filepath.Glob(filepath.Join(env.gw.root, "sites-enabled", "dougong-rb.example.com.conf"))
	if len(matches) != 0 {
		t.Fatalf("new file must be removed on rollback: %v", matches)
	}
}

// 空 server 块插入路径（insertPos 无子分支）：全部指令走块体首部插入。
func TestUpdateSiteOnEmptyServerBlock(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	p := env.writeConf(t, "sites-enabled", "empty.conf", "server {\n}\n")
	if _, err := env.svc.UpdateSite(ctx, adminActor(), UpdateSiteRequest{
		SiteID: p + "#0", ServerNames: []string{"e.example.com"}, Listen: []string{"8080"},
		Root: "/var/www/e",
	}); err != nil {
		t.Fatal(err)
	}
	got := readConf(t, p)
	for _, want := range []string{"server_name e.example.com;", "listen 8080;", "root /var/www/e;"} {
		if !strings.Contains(got, want) {
			t.Fatalf("insert missing %q:\n%s", want, got)
		}
	}
}

// http-01 挑战复用既有 location（A-SM10 ①）：零注入，webroot=其 root 值。
func TestCertRequestReuseChallengeLocation(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	webroot := filepath.Join(env.dataDir, "site-webroot")
	os.MkdirAll(webroot, 0o755)
	p := env.writeConf(t, "sites-enabled", "reuse.conf",
		"server {\n    listen 80;\n    server_name reuse.example.com;\n"+
			"    location ^~ /.well-known/acme-challenge/ {\n        root "+webroot+";\n    }\n}\n")
	before := readConf(t, p)
	env.acmeGw.res = issueResult(env.now.Add(90 * 24 * time.Hour))
	if _, err := env.svc.CertRequest(ctx, adminActor(), CertRequestRequest{
		SiteID: p + "#0", Challenge: storesitemgmt.ChallengeHTTP01}); err != nil {
		t.Fatal(err)
	}
	if got := readConf(t, p); got != before {
		t.Fatalf("reuse branch must not modify site config:\n%s", got)
	}
	env.acmeGw.mu.Lock()
	defer env.acmeGw.mu.Unlock()
	if env.acmeGw.webroot != webroot {
		t.Fatalf("webroot = %q, want reuse of location root %q", env.acmeGw.webroot, webroot)
	}
}

// deploy 管线失败补偿：快照键清除 + 配置回滚原样（I-05 一致性）。
func TestCertDeployPipelineFailureCompensation(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	p := env.writeConf(t, "sites-enabled", "dep2.conf",
		"server {\n    listen 443 ssl;\n    server_name dep2.example.com;\n}\n")
	before := readConf(t, p)
	src := env.writeConf(t, "sites-enabled", "dep2-src.conf",
		"server {\n    listen 80;\n    server_name cert2.example.com;\n}\n")
	env.acmeGw.res = issueResult(env.now.Add(90 * 24 * time.Hour))
	data, err := env.svc.CertRequest(ctx, adminActor(), CertRequestRequest{
		SiteID: src + "#0", Challenge: storesitemgmt.ChallengeTLSALPN})
	if err != nil {
		t.Fatal(err)
	}
	env.gw.testOK = false
	_, err = env.svc.CertDeploy(ctx, adminActor(), CertRefRequest{CertID: data.CertID, SiteID: p + "#0"})
	wantSymbol(t, err, "SITE_003")
	if got := readConf(t, p); got != before {
		t.Fatalf("config must be rolled back:\n%s", got)
	}
	row, _ := env.store.Cert().GetByID(ctx, data.CertID)
	if len(row.DeployPrevRefs) != 0 {
		t.Fatalf("prev refs must be compensated (I-05): %+v", row.DeployPrevRefs)
	}
}

// 续期后部署引用纠偏（redeployRef）：漂移站点经管线写回 storage_path。
func TestRetryRenewalRedeploysDriftedSite(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	// 直接构造 renew_failed 行 + 部署引用键（4.1.3 数据面）。
	full, _ := env.svc.certStoragePaths("renew2.example.com")
	os.MkdirAll(env.svc.certsDir, 0o755)
	os.WriteFile(full, []byte("OLD"), 0o600)
	reason := "old failure"
	site := env.writeConf(t, "sites-enabled", "drift.conf",
		"server {\n    listen 443 ssl;\n    server_name renew2.example.com;\n"+
			"    ssl_certificate /drifted/fullchain.pem;\n}\n")
	row := storesitemgmt.Certificate{
		PrimaryDomain: "renew2.example.com", Domains: []string{"renew2.example.com"},
		Source: storesitemgmt.SourceAcme, Status: storesitemgmt.StatusRenewFailed,
		Challenge: strpLocal(storesitemgmt.ChallengeHTTP01), StoragePath: &full,
		LastError: &reason,
		DeployPrevRefs: map[string]storesitemgmt.DeployRef{
			site + "#0": {Cert: "/old/full.pem", Key: "/old/key.pem"},
		},
		NotAfter: tmPtr(env.now.Add(2 * 24 * time.Hour)),
	}
	id, err := env.store.Cert().Create(ctx, row)
	if err != nil {
		t.Fatal(err)
	}
	env.acmeGw.res = issueResult(env.now.Add(150 * 24 * time.Hour))
	if _, err := env.svc.CertRetryRenewal(ctx, adminActor(), id); err != nil {
		t.Fatal(err)
	}
	got := readConf(t, site)
	if !strings.Contains(got, "ssl_certificate "+full+";") { // 漂移纠偏回托管路径
		t.Fatalf("redeploy ref missing:\n%s", got)
	}
	if b, _ := os.ReadFile(full); string(b) != "CERT-PEM" { // 托管文件原路径替换（I-04）
		t.Fatalf("managed file not replaced: %q", b)
	}
	after, _ := env.store.Cert().GetByID(ctx, id)
	if after.Status != storesitemgmt.StatusValid || after.LastError != nil {
		t.Fatalf("row after retry: %+v", after)
	}
}

// 边界杂项：enabled 筛选非法码 / 证书路径索引择优 / NULL 有效期排序 / 含空格 root 引号 / 超限源码。
func TestMiscEdges(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()

	// bindFieldError：enabled 筛选值非法。
	_, err := env.svc.ListSites(ctx, "bogus")
	wantSymbol(t, err, "AUTH_012")

	// 证书路径索引择优（betterCert）：同路径两行取 not_after 更晚者。
	certPEM := genCertPEM(t, "x.example.com", env.now.Add(-time.Hour), env.now.Add(50*24*time.Hour))
	shared := filepath.Join(env.dataDir, "shared.pem")
	os.WriteFile(shared, []byte(certPEM), 0o644)
	id1, _ := env.store.Cert().Create(ctx, storesitemgmt.Certificate{
		PrimaryDomain: "x1.example.com", Domains: []string{"x1.example.com"},
		Source: storesitemgmt.SourceImported, Status: storesitemgmt.StatusValid, OriginPath: &shared,
	})
	id2, _ := env.store.Cert().Create(ctx, storesitemgmt.Certificate{
		PrimaryDomain: "x2.example.com", Domains: []string{"x2.example.com"},
		Source: storesitemgmt.SourceImported, Status: storesitemgmt.StatusValid, OriginPath: &shared,
		NotAfter: tmPtr(env.now.Add(80 * 24 * time.Hour)), // 落库更晚（呈现取更晚者）
	})
	idx := env.svc.buildCertIndex(ctx)
	best, ok := idx.matchCert(shared)
	if !ok || best.ID != id2 {
		t.Fatalf("index best = %+v ok=%v (want id=%d)", best, ok, id2)
	}
	_ = id1

	// NULL 有效期行（pending）参与清单排序（sortCertItems nil 分支）。
	env.store.Cert().Create(ctx, storesitemgmt.Certificate{
		PrimaryDomain: "p.example.com", Domains: []string{"p.example.com"},
		Source: storesitemgmt.SourceAcme, Status: storesitemgmt.StatusPending,
		Challenge: strpLocal(storesitemgmt.ChallengeHTTP01),
	})
	if _, err := env.svc.CertList(ctx, "", ""); err != nil {
		t.Fatal(err)
	}

	// 含空格 root 写回加引号（quoteIfNeeded）。
	spaceRoot := filepath.Join(env.dataDir, "root with space")
	os.MkdirAll(spaceRoot, 0o755)
	p := env.writeConf(t, "sites-enabled", "sp.conf",
		"server {\n    listen 80;\n    server_name sp.example.com;\n}\n")
	if _, err := env.svc.UpdateSite(ctx, adminActor(), UpdateSiteRequest{
		SiteID: p + "#0", ServerNames: []string{"sp.example.com"}, Listen: []string{"80"},
		Root: spaceRoot,
	}); err != nil {
		t.Fatal(err)
	}
	if got := readConf(t, p); !strings.Contains(got, `root "`+spaceRoot+`";`) {
		t.Fatalf("spaced root must be quoted:\n%s", got)
	}

	// 超限源码读取（ReadSource SITE_014 分支）。
	big := env.writeConf(t, "sites-enabled", "big.conf",
		"server {\n    listen 80;\n    server_name big.example.com;\n"+
			"    # "+strings.Repeat("x", SourceSizeLimit)+"\n}\n")
	_, err = env.svc.ReadSource(ctx, big+"#0")
	wantSymbol(t, err, "SITE_014")
}
