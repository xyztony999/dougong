package auth

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dougong/dougong/internal/errcode"
	storeauth "github.com/dougong/dougong/internal/store/auth"
)

// fakeWA 是 WebAuthn 端口替身（协议层真校验归浏览器虚拟认证器验收；此处测服务编排）。
type fakeWA struct{}

func (fakeWA) BeginRegistration(_ context.Context, _ int64, _ string, _ []storeauth.WebauthnCredential) ([]byte, string, error) {
	return []byte(`{"publicKey":{}}`), "regtok", nil
}
func (fakeWA) FinishRegistration(_ context.Context, _ string, _ []byte) (storeauth.WebauthnCredential, error) {
	return storeauth.WebauthnCredential{CredentialID: []byte("cred-1"), PublicKey: []byte("pk"), SignCount: 0}, nil
}
func (fakeWA) BeginLogin(_ context.Context, _ int64, _ string, _ []storeauth.WebauthnCredential) ([]byte, string, error) {
	return []byte(`{"publicKey":{}}`), "logtok", nil
}
func (fakeWA) FinishLogin(_ context.Context, _ string, _ []byte, _ []storeauth.WebauthnCredential) ([]byte, uint32, byte, error) {
	return []byte("cred-1"), 1, 0, nil
}

func withFakeWA(svc *Service) {
	svc.wa = func(rpID string, origins []string) WebAuthnProvider { return fakeWA{} }
}

// flagsWA 让注册/断言返回指定 flags 原始字节（校验落库与回写）。
type flagsWA struct {
	fakeWA
	regFlags    byte
	assertFlags byte
	signCount   uint32
}

func (f flagsWA) FinishRegistration(_ context.Context, _ string, _ []byte) (storeauth.WebauthnCredential, error) {
	return storeauth.WebauthnCredential{
		CredentialID: []byte("cred-flags"), PublicKey: []byte("pk"), SignCount: 0, Flags: f.regFlags,
	}, nil
}

func (f flagsWA) FinishLogin(_ context.Context, _ string, _ []byte, _ []storeauth.WebauthnCredential) ([]byte, uint32, byte, error) {
	return []byte("cred-flags"), f.signCount, f.assertFlags, nil
}

// flags 原始字节须注册即落库，并在断言成功后随 sign_count 回写（旧行 flags=0 自愈即靠此持久化）。
// v0.6.4 前未持久化 → 可同步 passkey（BE=1）登录被判 BackupEligible 不一致而拒。
func TestWebauthnFlagsPersistedAndWrittenBack(t *testing.T) {
	svc, st, _ := newTestService(t)
	ctx := context.Background()
	_, _ = bootstrapAdmin(t, svc, "admin", "admin12345")
	id := &Identity{UserID: 1, Username: "admin", Role: "admin"}

	const regFlags = byte(0x4D)  // UP|UV|BE|AT：注册时认证器上报的原始字节
	const liveFlags = byte(0x0D) // UP|UV|BE：断言时无 AT 位

	svc.wa = func(string, []string) WebAuthnProvider {
		return flagsWA{regFlags: regFlags, assertFlags: liveFlags, signCount: 7}
	}
	registerPasskey(t, svc, id)

	creds, err := st.Webauthn().ListByUser(ctx, 1)
	if err != nil || len(creds) != 1 {
		t.Fatalf("list: %v %d", err, len(creds))
	}
	if creds[0].Flags != regFlags {
		t.Fatalf("persisted flags = %#x, want %#x", creds[0].Flags, regFlags)
	}

	// 密码 → 二因素 passkey 登录，成功后须把断言侧 flags 与 sign_count 写回。
	out, err := svc.Login(ctx, LoginRequest{Username: "admin", Password: "admin12345"}, waMeta())
	if err != nil || !out.RequiresPasskey {
		t.Fatalf("login: %v %+v", err, out)
	}
	if _, err := svc.PasskeyVerify(ctx, WebAuthnLoginFinishRequest{
		LoginTicket: out.LoginTicket, Token: "logtok", Credential: json.RawMessage(`{}`),
	}, waMeta()); err != nil {
		t.Fatalf("passkey verify: %v", err)
	}

	got, err := st.Webauthn().GetByID(ctx, 1, creds[0].ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Flags != liveFlags {
		t.Fatalf("flags after assert = %#x, want %#x", got.Flags, liveFlags)
	}
	if got.SignCount != 7 {
		t.Fatalf("signCount = %d, want 7", got.SignCount)
	}
	if got.LastUsedAt == nil {
		t.Fatal("lastUsedAt not set")
	}
}

func waMeta() RequestMeta {
	return RequestMeta{Host: "localhost:18697", Origin: "http://localhost:18697", UserAgent: "test-UA"}
}

func TestRecoveryCodesLifecycle(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	_, _ = bootstrapAdmin(t, svc, "admin", "admin12345")
	id := &Identity{UserID: 1, Username: "admin", Role: "admin"}

	res, err := svc.GenerateRecoveryCodes(ctx, id)
	if err != nil || len(res.Codes) != 10 {
		t.Fatalf("generate: %v codes=%d", err, len(res.Codes))
	}
	cnt, _ := svc.RecoveryCount(ctx, id)
	if cnt.Remaining != 10 {
		t.Fatalf("remaining=%d want 10", cnt.Remaining)
	}
	// 恢复码登录消耗一码。
	issued, err := svc.RecoveryLogin(ctx, RecoveryLoginRequest{Username: "admin", Code: res.Codes[0]}, RequestMeta{})
	if err != nil || issued.RawToken == "" {
		t.Fatalf("recovery login: %v", err)
	}
	cnt, _ = svc.RecoveryCount(ctx, id)
	if cnt.Remaining != 9 {
		t.Fatalf("remaining after use=%d want 9", cnt.Remaining)
	}
	// 一次性：重用同码失败。
	if _, err := svc.RecoveryLogin(ctx, RecoveryLoginRequest{Username: "admin", Code: res.Codes[0]}, RequestMeta{}); err == nil {
		t.Fatal("reused recovery code must fail")
	}
	// 错误码失败。
	if _, err := svc.RecoveryLogin(ctx, RecoveryLoginRequest{Username: "admin", Code: "AAAA-BBBB-CCCC-DDDD"}, RequestMeta{}); err == nil {
		t.Fatal("wrong recovery code must fail")
	}
	// 重新生成作废旧码。
	if _, err := svc.GenerateRecoveryCodes(ctx, id); err != nil {
		t.Fatalf("regen: %v", err)
	}
	if _, err := svc.RecoveryLogin(ctx, RecoveryLoginRequest{Username: "admin", Code: res.Codes[1]}, RequestMeta{}); err == nil {
		t.Fatal("old code after regen must fail")
	}
}

func TestNewDeviceDetection(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	_, _ = bootstrapAdmin(t, svc, "admin", "admin12345")
	meta := RequestMeta{UserAgent: "UA", DeviceCookie: "dev-1"}
	out, err := svc.Login(ctx, LoginRequest{Username: "admin", Password: "admin12345"}, meta)
	if err != nil || !out.Session.NewDevice {
		t.Fatalf("first login newDevice=true: %v %+v", err, out.Session)
	}
	out, err = svc.Login(ctx, LoginRequest{Username: "admin", Password: "admin12345"}, meta)
	if err != nil || out.Session.NewDevice {
		t.Fatalf("second login newDevice=false: %v %+v", err, out.Session)
	}
}

func registerPasskey(t *testing.T, svc *Service, id *Identity) {
	t.Helper()
	ctx := context.Background()
	if _, err := svc.RegisterBegin(ctx, id, waMeta()); err != nil {
		t.Fatalf("register begin: %v", err)
	}
	if _, err := svc.RegisterFinish(ctx, id, WebAuthnRegisterFinishRequest{Token: "regtok", Credential: json.RawMessage(`{}`), Name: "key"}, waMeta()); err != nil {
		t.Fatalf("register finish: %v", err)
	}
}

func TestPasskeyTwoFactor(t *testing.T) {
	svc, _, _ := newTestService(t)
	withFakeWA(svc)
	ctx := context.Background()
	_, _ = bootstrapAdmin(t, svc, "admin", "admin12345")
	id := &Identity{UserID: 1, Username: "admin", Role: "admin"}
	registerPasskey(t, svc, id)

	// 密码后（无 TOTP 有 passkey）→ requiresPasskey + ticket。
	out, err := svc.Login(ctx, LoginRequest{Username: "admin", Password: "admin12345"}, waMeta())
	if err != nil || !out.RequiresPasskey || out.LoginTicket == "" {
		t.Fatalf("requiresPasskey: %v %+v", err, out)
	}
	issued, err := svc.PasskeyVerify(ctx, WebAuthnLoginFinishRequest{LoginTicket: out.LoginTicket, Token: "logtok", Credential: json.RawMessage(`{}`)}, waMeta())
	if err != nil || issued.RawToken == "" {
		t.Fatalf("passkey verify: %v", err)
	}
	// ticket 一次性。
	if _, err := svc.PasskeyVerify(ctx, WebAuthnLoginFinishRequest{LoginTicket: out.LoginTicket, Token: "logtok", Credential: json.RawMessage(`{}`)}, waMeta()); err == nil {
		t.Fatal("reused ticket must fail")
	}
}

func TestPasswordlessLogin(t *testing.T) {
	svc, _, _ := newTestService(t)
	withFakeWA(svc)
	ctx := context.Background()
	_, _ = bootstrapAdmin(t, svc, "admin", "admin12345")
	id := &Identity{UserID: 1, Username: "admin", Role: "admin"}
	registerPasskey(t, svc, id)

	begin, err := svc.LoginBegin(ctx, WebAuthnLoginBeginRequest{Username: "admin"}, waMeta())
	if err != nil || begin.Token == "" {
		t.Fatalf("login begin: %v", err)
	}
	issued, err := svc.LoginFinish(ctx, WebAuthnLoginFinishRequest{Username: "admin", Token: begin.Token, Credential: json.RawMessage(`{}`)}, waMeta())
	if err != nil || issued.RawToken == "" {
		t.Fatalf("passwordless login: %v", err)
	}
	// 无 passkey 用户 login-begin → 凭据不存在（独立实例）。
	svc2, _, _ := newTestService(t)
	withFakeWA(svc2)
	_, _ = bootstrapAdmin(t, svc2, "bob", "bob1234567")
	if _, err := svc2.LoginBegin(ctx, WebAuthnLoginBeginRequest{Username: "bob"}, waMeta()); err == nil {
		t.Fatal("user without passkey login-begin must fail")
	}
}

func TestWebAuthnListDelete(t *testing.T) {
	svc, _, _ := newTestService(t)
	withFakeWA(svc)
	ctx := context.Background()
	_, _ = bootstrapAdmin(t, svc, "admin", "admin12345")
	id := &Identity{UserID: 1, Username: "admin", Role: "admin"}
	registerPasskey(t, svc, id)

	items, err := svc.ListCredentials(ctx, id)
	if err != nil || len(items) != 1 {
		t.Fatalf("list: %v %d", err, len(items))
	}
	if err := svc.DeleteCredential(ctx, id, WebAuthnDeleteRequest{ID: items[0].ID}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	items, _ = svc.ListCredentials(ctx, id)
	if len(items) != 0 {
		t.Fatalf("after delete len=%d", len(items))
	}
	if err := svc.DeleteCredential(ctx, id, WebAuthnDeleteRequest{ID: 999}); err == nil {
		t.Fatal("delete missing must fail")
	}
}

// provider 的允许 origin 须含同主机多形态（CDN/反代可能归一化或剥离 Origin 端口）。
func TestWebAuthnProviderTolerantOrigins(t *testing.T) {
	svc, _, _ := newTestService(t)
	var gotRP string
	var gotOrigins []string
	svc.wa = func(rpID string, origins []string) WebAuthnProvider {
		gotRP, gotOrigins = rpID, origins
		return fakeWA{}
	}
	meta := RequestMeta{Host: "panel.example.com:2083", Origin: "https://panel.example.com:2083"}
	if _, err := svc.provider(meta); err != nil {
		t.Fatalf("provider: %v", err)
	}
	if gotRP != "panel.example.com" {
		t.Fatalf("rpID = %q", gotRP)
	}
	want := []string{
		"https://panel.example.com:2083",
		"https://panel.example.com",
		"http://panel.example.com:2083",
		"http://panel.example.com",
	}
	for _, w := range want {
		found := false
		for _, o := range gotOrigins {
			if o == w {
				found = true
			}
		}
		if !found {
			t.Fatalf("origins %v missing %q", gotOrigins, w)
		}
	}
}

var _ = errcode.ErrWebAuthnInvalid
