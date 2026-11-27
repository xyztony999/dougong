package webauthnx

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	storeauth "github.com/dougong/dougong/internal/store/auth"
)

const (
	testRPID   = "panel.example.com"
	testOrigin = "https://panel.example.com"

	// authenticatorData flags 位（WebAuthn L3 §6.1）。
	flagUP = 0x01
	flagUV = 0x04
	flagBE = 0x08
	flagAT = 0x40 // attested credential data：注册 authData 必置
)

// softAuthenticator 是测试用软认证器：自持 P-256 私钥，手工产出 attestation/assertion。
// 浏览器（含 CDP 虚拟认证器）无法置 BackupEligible 位，而 BE 不一致正是可同步 passkey
// 在真实环境被拒的根因，只能在协议层构造断言复现。
type softAuthenticator struct {
	priv   *ecdsa.PrivateKey
	credID []byte
}

func newSoftAuthenticator(t *testing.T) *softAuthenticator {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	credID := make([]byte, 32)
	if _, err := rand.Read(credID); err != nil {
		t.Fatalf("credential id: %v", err)
	}
	return &softAuthenticator{priv: priv, credID: credID}
}

func newTestProvider(t *testing.T) *Provider {
	t.Helper()
	p, err := New(testRPID, []string{testOrigin})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	return p
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func pad32(b []byte) []byte {
	if len(b) == 32 {
		return b
	}
	out := make([]byte, 32)
	copy(out[32-len(b):], b)
	return out
}

// coseKey 手工 CBOR 编码 EC2/P-256 公钥：{1:2, 3:-7, -1:1, -2:x, -3:y}。
func (a *softAuthenticator) coseKey() []byte {
	out := []byte{0xA5, 0x01, 0x02, 0x03, 0x26, 0x20, 0x01, 0x21, 0x58, 0x20}
	out = append(out, pad32(a.priv.PublicKey.X.Bytes())...)
	out = append(out, 0x22, 0x58, 0x20)
	out = append(out, pad32(a.priv.PublicKey.Y.Bytes())...)
	return out
}

// cborByteString / cborTextString 只覆盖本测试所需的短长度头。
func cborByteString(s []byte) []byte {
	switch {
	case len(s) < 24:
		return append([]byte{0x40 | byte(len(s))}, s...)
	case len(s) < 256:
		return append([]byte{0x58, byte(len(s))}, s...)
	default:
		h := []byte{0x59, 0, 0}
		binary.BigEndian.PutUint16(h[1:], uint16(len(s)))
		return append(h, s...)
	}
}

func cborTextString(s string) []byte {
	b := []byte(s)
	if len(b) < 24 {
		return append([]byte{0x60 | byte(len(b))}, b...)
	}
	return append([]byte{0x78, byte(len(b))}, b...)
}

// authData 组装 rpIdHash ‖ flags ‖ counter（attested 时再追加 attestedCredentialData）。
func (a *softAuthenticator) authData(flags byte, counter uint32, attested bool) []byte {
	rpHash := sha256.Sum256([]byte(testRPID))
	out := append([]byte{}, rpHash[:]...)
	out = append(out, flags)
	ctr := make([]byte, 4)
	binary.BigEndian.PutUint32(ctr, counter)
	out = append(out, ctr...)
	if attested {
		out = append(out, make([]byte, 16)...) // AAGUID
		n := make([]byte, 2)
		binary.BigEndian.PutUint16(n, uint16(len(a.credID)))
		out = append(out, n...)
		out = append(out, a.credID...)
		out = append(out, a.coseKey()...)
	}
	return out
}

func clientData(t *testing.T, ceremony, challenge string) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"type":        ceremony,
		"challenge":   challenge,
		"origin":      testOrigin,
		"crossOrigin": false,
	})
	if err != nil {
		t.Fatalf("marshal clientDataJSON: %v", err)
	}
	return b
}

func optionsChallenge(t *testing.T, opt []byte) string {
	t.Helper()
	var o struct {
		PublicKey struct {
			Challenge string `json:"challenge"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(opt, &o); err != nil {
		t.Fatalf("parse options: %v", err)
	}
	if o.PublicKey.Challenge == "" {
		t.Fatalf("empty challenge in options: %s", opt)
	}
	return o.PublicKey.Challenge
}

// register 走真实 BeginRegistration/FinishRegistration，产出带 flags 的落库凭据。
func (a *softAuthenticator) register(t *testing.T, p *Provider, userID int64, username string, flags byte) storeauth.WebauthnCredential {
	t.Helper()
	ctx := context.Background()
	opt, token, err := p.BeginRegistration(ctx, userID, username, nil)
	if err != nil {
		t.Fatalf("begin registration: %v", err)
	}
	cdj := clientData(t, "webauthn.create", optionsChallenge(t, opt))
	attObj := []byte{0xA3}
	attObj = append(attObj, cborTextString("fmt")...)
	attObj = append(attObj, cborTextString("none")...)
	attObj = append(attObj, cborTextString("attStmt")...)
	attObj = append(attObj, 0xA0)
	attObj = append(attObj, cborTextString("authData")...)
	attObj = append(attObj, cborByteString(a.authData(flags, 1, true))...)
	resp, err := json.Marshal(map[string]any{
		"id":       b64(a.credID),
		"rawId":    b64(a.credID),
		"type":     "public-key",
		"response": map[string]any{"clientDataJSON": b64(cdj), "attestationObject": b64(attObj)},
	})
	if err != nil {
		t.Fatalf("marshal attestation: %v", err)
	}
	cred, err := p.FinishRegistration(ctx, token, resp)
	if err != nil {
		t.Fatalf("finish registration: %v", err)
	}
	return cred
}

// assert 用指定 flags 手工签一次断言并调 FinishLogin。
func (a *softAuthenticator) assert(t *testing.T, p *Provider, userID int64, username string, creds []storeauth.WebauthnCredential, flags byte, counter uint32) ([]byte, uint32, byte, error) {
	t.Helper()
	ctx := context.Background()
	opt, token, err := p.BeginLogin(ctx, userID, username, creds)
	if err != nil {
		t.Fatalf("begin login: %v", err)
	}
	cdj := clientData(t, "webauthn.get", optionsChallenge(t, opt))
	ad := a.authData(flags, counter, false)
	clientHash := sha256.Sum256(cdj)
	sigBase := append(append([]byte{}, ad...), clientHash[:]...)
	digest := sha256.Sum256(sigBase)
	sig, err := ecdsa.SignASN1(rand.Reader, a.priv, digest[:])
	if err != nil {
		t.Fatalf("sign assertion: %v", err)
	}
	resp, err := json.Marshal(map[string]any{
		"id":    b64(a.credID),
		"rawId": b64(a.credID),
		"type":  "public-key",
		"response": map[string]any{
			"clientDataJSON":    b64(cdj),
			"authenticatorData": b64(ad),
			"signature":         b64(sig),
			"userHandle":        b64([]byte(fmt.Sprintf("%d", userID))),
		},
	})
	if err != nil {
		t.Fatalf("marshal assertion: %v", err)
	}
	return p.FinishLogin(ctx, token, resp, creds)
}

// 注册须把认证器 flags 原始字节带回落库结构（v0.6.4 前丢失，导致 BE 校验假不一致）。
func TestFinishRegistrationReturnsAuthenticatorFlags(t *testing.T) {
	p := newTestProvider(t)
	a := newSoftAuthenticator(t)
	cred := a.register(t, p, 7, "tony", flagUP|flagUV|flagBE|flagAT)
	if cred.Flags != flagUP|flagUV|flagBE|flagAT {
		t.Fatalf("flags = %#x, want %#x", cred.Flags, flagUP|flagUV|flagBE|flagAT)
	}
	if len(cred.PublicKey) == 0 || string(cred.CredentialID) != string(a.credID) {
		t.Fatalf("credential material not returned: id=%d pk=%d", len(cred.CredentialID), len(cred.PublicKey))
	}
}

// v0.6.3/v0.6.4 注册的旧行 flags=0（迁移默认值），而可同步 passkey 断言 BE=1 →
// go-webauthn 判「Backup Eligible flag inconsistency」拒登（用户服务器实测）。
// 旧行须按本次断言自愈并回写 flags，避免用户重新注册。
func TestFinishLoginSelfHealsLegacyZeroFlags(t *testing.T) {
	p := newTestProvider(t)
	a := newSoftAuthenticator(t)
	cred := a.register(t, p, 7, "tony", flagUP|flagUV|flagBE|flagAT)

	legacy := cred
	legacy.Flags = 0 // 模拟升级前落库的旧行

	credID, signCount, flags, err := a.assert(t, p, 7, "tony", []storeauth.WebauthnCredential{legacy}, flagUP|flagUV|flagBE, 2)
	if err != nil {
		t.Fatalf("legacy flags=0 must self-heal, got: %v", err)
	}
	if string(credID) != string(a.credID) {
		t.Fatalf("credential id mismatch")
	}
	if flags != flagUP|flagUV|flagBE {
		t.Fatalf("flags written back = %#x, want %#x", flags, flagUP|flagUV|flagBE)
	}
	if signCount != 2 {
		t.Fatalf("signCount = %d, want 2", signCount)
	}
}

// 自愈只放行「flags 全零」的旧行；已存 flags 而 BE 不一致是真实异常（换认证器/克隆），必须拒。
func TestFinishLoginRejectsBackupEligibleMismatchWhenFlagsStored(t *testing.T) {
	p := newTestProvider(t)
	a := newSoftAuthenticator(t)
	cred := a.register(t, p, 7, "tony", flagUP|flagUV|flagAT) // 非同步认证器，BE=0

	if cred.Flags != flagUP|flagUV|flagAT {
		t.Fatalf("stored flags = %#x", cred.Flags)
	}
	_, _, _, err := a.assert(t, p, 7, "tony", []storeauth.WebauthnCredential{cred}, flagUP|flagUV|flagBE, 2)
	if err == nil {
		t.Fatal("BE mismatch on a non-legacy credential must be rejected")
	}
	if !strings.Contains(err.Error(), "Backup Eligible flag inconsistency") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// 正常路径（flags 已正确持久化）不得受自愈逻辑影响。
func TestFinishLoginNormalPathWithPersistedFlags(t *testing.T) {
	p := newTestProvider(t)
	a := newSoftAuthenticator(t)
	cred := a.register(t, p, 7, "tony", flagUP|flagUV|flagBE|flagAT)

	credID, signCount, flags, err := a.assert(t, p, 7, "tony", []storeauth.WebauthnCredential{cred}, flagUP|flagUV|flagBE, 2)
	if err != nil {
		t.Fatalf("normal login: %v", err)
	}
	if string(credID) != string(a.credID) || signCount != 2 || flags != flagUP|flagUV|flagBE {
		t.Fatalf("unexpected result: id=%x count=%d flags=%#x", credID, signCount, flags)
	}
	// 签名错误仍须拒。
	bad := *a
	bad.priv = mustOtherKey(t)
	if _, _, _, err := bad.assert(t, p, 7, "tony", []storeauth.WebauthnCredential{cred}, flagUP|flagUV|flagBE, 3); err == nil {
		t.Fatal("assertion signed by another key must be rejected")
	}
}

func mustOtherKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return k
}
