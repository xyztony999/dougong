package panel

import (
	"context"
	"strings"
	"testing"
)

// 单行输入框粘贴多行 PEM 时，浏览器把换行换成空格，得到标记与正文挤在一行、base64 夹空格的
// 坏格式（HttpsCard 私钥在 v0.5.0 前是 password 单行框）。normalizePEM 须救回这种格式，
// ValidateCertPair 仍配对成功；但容错只修空白，不放宽配对校验。
func TestValidateCertPairToleratesWhitespaceMangledPEM(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(t)
	if _, err := svc.UpdateConfig(ctx, adminActor(), UpdateConfigRequest{HTTPSMode: ptr("self_signed")}); err != nil {
		t.Fatalf("self signed: %v", err)
	}
	certPEM, keyPEM, err := svc.CertMaterial(ctx)
	if err != nil {
		t.Fatalf("cert material: %v", err)
	}

	// 模拟单行框粘贴：所有换行变空格、挤成一行
	mangle := func(p string) string {
		return strings.ReplaceAll(strings.ReplaceAll(p, "\r\n", " "), "\n", " ")
	}

	if _, err := ValidateCertPair(certPEM, mangle(keyPEM)); err != nil {
		t.Fatalf("mangled key must still parse and match: %v", err)
	}
	if _, err := ValidateCertPair(mangle(certPEM), keyPEM); err != nil {
		t.Fatalf("mangled cert must still parse: %v", err)
	}
	if _, err := ValidateCertPair(mangle(certPEM), mangle(keyPEM)); err != nil {
		t.Fatalf("both mangled must still parse and match: %v", err)
	}
	// 正常 PEM 不受影响
	if _, err := ValidateCertPair(certPEM, keyPEM); err != nil {
		t.Fatalf("well-formed pair must parse: %v", err)
	}

	// 容错不能放宽配对校验：另一把私钥即便格式正常也必须被拒
	svc2, _ := newTestService(t)
	if _, err := svc2.UpdateConfig(ctx, adminActor(), UpdateConfigRequest{HTTPSMode: ptr("self_signed")}); err != nil {
		t.Fatalf("self signed 2: %v", err)
	}
	_, key2, err := svc2.CertMaterial(ctx)
	if err != nil {
		t.Fatalf("cert material 2: %v", err)
	}
	if _, err := ValidateCertPair(certPEM, key2); err == nil {
		t.Fatal("mismatched pair must still be rejected")
	}
	if _, err := ValidateCertPair(mangle(certPEM), mangle(key2)); err == nil {
		t.Fatal("mangled mismatched pair must still be rejected")
	}
}

func TestNormalizePEM(t *testing.T) {
	const good = "-----BEGIN PRIVATE KEY-----\nMIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgEer+Clx7x1sJtc4q\n-----END PRIVATE KEY-----\n"
	// 坏格式：标记后跟空格、正文夹空格、单行
	bad := "-----BEGIN PRIVATE KEY----- MIGHAgEAMBMGByqGSM49AgEGCCqGSM49 AwEHBG0wawIBAQQgEer+Clx7x1sJtc4q -----END PRIVATE KEY-----"
	got := normalizePEM(bad)
	if !strings.HasPrefix(got, "-----BEGIN PRIVATE KEY-----\n") || !strings.HasSuffix(got, "-----END PRIVATE KEY-----\n") {
		t.Fatalf("markers must be on their own lines:\n%s", got)
	}
	lines := strings.Split(strings.TrimSpace(got), "\n")
	body := strings.Join(lines[1:len(lines)-1], "")
	if strings.Contains(body, " ") {
		t.Fatalf("base64 body must not contain spaces:\n%s", got)
	}
	// 正常输入重排后 base64 内容不变
	if normalizePEM(good) != good {
		t.Fatalf("well-formed PEM must be unchanged:\n%s", normalizePEM(good))
	}
	// 无标记原样返回
	if normalizePEM("not a pem") != "not a pem" {
		t.Fatal("text without PEM markers must be returned unchanged")
	}
}
