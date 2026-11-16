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
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/errcode"
	storesitemgmt "github.com/dougong/dougong/internal/store/sitemgmt"
)

// v1.2/v2.0 增量测试：聚合投影（D-17）、DNS 凭证（D-20）、replace（D-18）。

// newEnvWithVault 构造带 DNS 凭据保险库的测试环境（D-20：vault 注入）。
func newEnvWithVault(t *testing.T) *testEnv {
	t.Helper()
	env := newEnv(t)
	vault, err := OpenDnsCredentialVault(filepath.Join(env.dataDir, "dns"))
	if err != nil {
		t.Fatalf("open dns vault: %v", err)
	}
	env.svc.dnsVault = vault
	return env
}

func errSymbol(err error) string {
	var ec *errcode.Error
	if errors.As(err, &ec) {
		return ec.Symbol()
	}
	return ""
}

// selfSignedPEM 生成自签证书 PEM（replace 校验通道用）。
func selfSignedPEM(t *testing.T, cn string, days int) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: cn},
		DNSNames:  []string{cn},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(0, 0, days),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

// TestListSitesAggregate 同源文件同域名多 server 块聚合（D-17：一行条目、
// blocks 子清单、监听合并、跨文件不聚合）。
func TestListSitesAggregate(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	env.writeConf(t, "sites-enabled", "agg.conf", "server {\n    listen 80;\n    server_name proxy2.example.com;\n}\n"+
		"server {\n    listen 443 ssl;\n    server_name proxy2.example.com;\n"+
		"    ssl_certificate /etc/ssl/agg.pem;\n"+
		"    location / { proxy_pass http://127.0.0.1:35039; }\n}\n")
	env.writeConf(t, "sites-enabled", "other.conf", "server {\n    listen 8080;\n    server_name proxy2.example.com;\n}\n")
	list, err := env.svc.ListSites(ctx, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list.Items) != 2 {
		t.Fatalf("items = %d, want 2（同文件聚合 1 + 跨文件 1）", len(list.Items))
	}
	agg := list.Items[0]
	if len(agg.Blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(agg.Blocks))
	}
	if b := agg.Blocks[0]; b.SiteID == "" || b.Listen[0] != "80" {
		t.Fatalf("block0 寻址键/监听异常: %+v", b)
	}
	joined := agg.Listen[0] + " " + agg.Listen[1]
	if joined != "80 443 ssl" {
		t.Fatalf("listen 合并 = %q, want \"80 443 ssl\"", joined)
	}
	if agg.Enabled != SiteEnabled {
		t.Fatalf("聚合 enabled = %q, want enabled", agg.Enabled)
	}
	if agg.CertRef == nil || *agg.CertRef != "/etc/ssl/agg.pem" {
		t.Fatalf("聚合证书引用（ssl 块优先）异常: %v", agg.CertRef)
	}
	if agg.Proxy == nil || agg.Proxy.Upstream != "http://127.0.0.1:35039" {
		t.Fatalf("聚合反代段异常: %+v", agg.Proxy)
	}
}

// TestDnsCredentialLifecycle 凭证 CRUD（D-20：密文落库零回显、删除引用警示
// I-C01、CERT_009 幂等、凭证缺失续期失败落库）。
func TestDnsCredentialLifecycle(t *testing.T) {
	env := newEnvWithVault(t)
	ctx := context.Background()
	actor := adminActor()

	created, err := env.svc.DnsCredentialCreate(ctx, actor, DnsCredentialCreateRequest{
		Name: "cf-main", Provider: storesitemgmt.ProviderCloudflare,
		Credential: DnsCredentialPayload{APIToken: "tok-123"}})
	if err != nil || created.ID == 0 {
		t.Fatalf("create: %+v err=%v", created, err)
	}
	row, err := env.store.DnsCred().GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(row.CredentialEnc) == "tok-123" || len(row.CredentialEnc) < 16 {
		t.Fatalf("credential_enc 非密文形态: %q", string(row.CredentialEnc))
	}
	if _, err := env.svc.DnsCredentialCreate(ctx, actor, DnsCredentialCreateRequest{
		Name: "bad", Provider: storesitemgmt.ProviderAliyun,
		Credential: DnsCredentialPayload{AccessKeyID: "id"}}); err == nil {
		t.Fatal("aliyun 缺 secret 应拒绝")
	}
	// dns_01 证书引用 → 删除警示不硬阻断。
	cred2, _ := env.svc.DnsCredentialCreate(ctx, actor, DnsCredentialCreateRequest{
		Name: "ali", Provider: storesitemgmt.ProviderAliyun,
		Credential: DnsCredentialPayload{AccessKeyID: "k", AccessKeySecret: "s"}})
	ch := storesitemgmt.ChallengeDNS01
	certID, err := env.store.Cert().Create(ctx, storesitemgmt.Certificate{
		PrimaryDomain: "w.example.com", Domains: []string{"w.example.com"},
		Source: storesitemgmt.SourceAcme, Status: storesitemgmt.StatusValid,
		Challenge: &ch, DnsCredentialID: &cred2.ID,
	})
	if err != nil {
		t.Fatalf("cert create: %v", err)
	}
	del, err := env.svc.DnsCredentialDelete(ctx, actor, cred2.ID)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(del.ReferencedCerts) != 1 || del.ReferencedCerts[0].CertID != certID {
		t.Fatalf("referencedCerts = %+v, want 1 项指向 cert %d", del.ReferencedCerts, certID)
	}
	if _, err := env.svc.DnsCredentialDelete(ctx, actor, cred2.ID); errSymbol(err) != "CERT_009" {
		t.Fatalf("重复删除 = %v, want CERT_009", err)
	}
	if _, err := env.svc.CertRenew(ctx, actor, certID); errSymbol(err) != "CERT_009" {
		t.Fatalf("凭证缺失续期 = %v, want CERT_009", err)
	}
	// D-18 分流：valid 行续期失败保持 valid（旧证书仍在用）+ last_error 落库。
	after, _ := env.store.Cert().GetByID(ctx, certID)
	if after.Status != storesitemgmt.StatusValid || after.LastError == nil || *after.LastError != "DNS 凭证已删除" {
		t.Fatalf("凭证缺失终态 = %s/%v, want valid+last_error=凭证缺失", after.Status, after.LastError)
	}
}

// TestCertReplaceImported imported 文件替换（D-18：PEM 校验+备份+写回+响应
// 元数据；非法 PEM → CERT_008）。
func TestCertReplaceImported(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	origDir := filepath.Join(env.gw.root, "imported")
	if err := os.MkdirAll(origDir, 0o755); err != nil {
		t.Fatal(err)
	}
	origPath := filepath.Join(origDir, "old.pem")
	if err := os.WriteFile(origPath, []byte("old-cert-pem"), 0o644); err != nil {
		t.Fatal(err)
	}
	id, err := env.store.Cert().Create(ctx, storesitemgmt.Certificate{
		PrimaryDomain: "i.example.com", Domains: []string{"i.example.com"},
		Source: storesitemgmt.SourceImported, Status: storesitemgmt.StatusValid,
		OriginPath: &origPath,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	pemText := selfSignedPEM(t, "i.example.com", 90)
	data, err := env.svc.CertReplace(ctx, adminActor(), CertReplaceRequest{CertID: id, CertPEM: pemText})
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	if data.Issuer == "" || data.NotAfter == nil || len(data.Domains) == 0 {
		t.Fatalf("replace 响应元数据缺失: %+v", data)
	}
	got, _ := os.ReadFile(origPath)
	if string(got) != pemText {
		t.Fatalf("origin_path 未被替换（原样保留或写入失败）")
	}
	if _, err := env.svc.CertReplace(ctx, adminActor(), CertReplaceRequest{CertID: id, CertPEM: "not-a-pem"}); errSymbol(err) != "CERT_008" {
		t.Fatalf("非法 PEM = %v, want CERT_008", err)
	}
}
