// Package webauthnx 封装 go-webauthn/webauthn（D-P 登录安全增量，纯 Go 无 CGO）。
// 实现 service/auth.WebAuthnProvider；begin/finish 之间的 session data 以一次性
// token 存服务端内存（5min TTL），客户端不持有 challenge。RP ID/Origin 按请求构造
// （绑定域名或访问 Host；CF 橙云回源 TLS 满足安全上下文）。
package webauthnx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	storeauth "github.com/dougong/dougong/internal/store/auth"
)

const sessionTTL = 5 * time.Minute

// 全局 session 存储：provider 按请求构造（RP ID/Origin 随主机变），但 begin/finish
// 是两次请求，token→sessionData 必须跨实例共享，故放包级。
var globalSessions = struct {
	sync.Mutex
	m map[string]sessionEntry
}{m: map[string]sessionEntry{}}

// Provider 实现 service/auth.WebAuthnProvider。
type Provider struct {
	w *webauthn.WebAuthn
}

type sessionEntry struct {
	data   *webauthn.SessionData
	userID int64
	exp    time.Time
}

// New 按 RP ID 与允许 Origin 构造 provider。
func New(rpID string, origins []string) (*Provider, error) {
	w, err := webauthn.New(&webauthn.Config{
		RPDisplayName: "斗拱面板",
		RPID:          rpID,
		RPOrigins:     origins,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementPreferred,
			UserVerification: protocol.VerificationPreferred,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("webauthnx: config: %w", err)
	}
	return &Provider{w: w}, nil
}

type waUser struct {
	id    int64
	name  string
	creds []webauthn.Credential
}

func (u waUser) WebAuthnID() []byte                         { return []byte(fmt.Sprintf("%d", u.id)) }
func (u waUser) WebAuthnName() string                       { return u.name }
func (u waUser) WebAuthnDisplayName() string                { return u.name }
func (u waUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

func toCreds(list []storeauth.WebauthnCredential) []webauthn.Credential {
	out := make([]webauthn.Credential, 0, len(list))
	for _, c := range list {
		out = append(out, webauthn.Credential{
			ID:              c.CredentialID,
			PublicKey:       c.PublicKey,
			AttestationType: c.AttestationType,
			Flags:           webauthn.CredentialFlagsFromMsgpByte(c.Flags),
			Transport:       toTransports(c.Transports),
			Authenticator: webauthn.Authenticator{
				AAGUID:    c.AAGUID,
				SignCount: c.SignCount,
			},
		})
	}
	return out
}

func toTransports(ss []string) []protocol.AuthenticatorTransport {
	out := make([]protocol.AuthenticatorTransport, 0, len(ss))
	for _, s := range ss {
		out = append(out, protocol.AuthenticatorTransport(s))
	}
	return out
}

func (p *Provider) put(userID int64, data *webauthn.SessionData) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	globalSessions.Lock()
	defer globalSessions.Unlock()
	gcSessions()
	globalSessions.m[token] = sessionEntry{data: data, userID: userID, exp: time.Now().Add(sessionTTL)}
	return token, nil
}

func (p *Provider) take(token string) (sessionEntry, error) {
	globalSessions.Lock()
	defer globalSessions.Unlock()
	e, ok := globalSessions.m[token]
	if !ok || time.Now().After(e.exp) {
		delete(globalSessions.m, token)
		return sessionEntry{}, fmt.Errorf("webauthnx: session invalid or expired")
	}
	delete(globalSessions.m, token)
	return e, nil
}

func gcSessions() {
	now := time.Now()
	for k, e := range globalSessions.m {
		if now.After(e.exp) {
			delete(globalSessions.m, k)
		}
	}
}

// BeginRegistration 返回 creation options JSON + token。
func (p *Provider) BeginRegistration(_ context.Context, userID int64, username string, existing []storeauth.WebauthnCredential) ([]byte, string, error) {
	u := waUser{id: userID, name: username, creds: toCreds(existing)}
	opt, data, err := p.w.BeginRegistration(u)
	if err != nil {
		return nil, "", fmt.Errorf("webauthnx: begin registration: %w", err)
	}
	token, err := p.put(userID, data)
	if err != nil {
		return nil, "", err
	}
	b, err := json.Marshal(opt)
	return b, token, err
}

// FinishRegistration 校验 attestation 并返回落库凭据材料。
func (p *Provider) FinishRegistration(_ context.Context, token string, response []byte) (storeauth.WebauthnCredential, error) {
	e, err := p.take(token)
	if err != nil {
		return storeauth.WebauthnCredential{}, err
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(response)
	if err != nil {
		return storeauth.WebauthnCredential{}, fmt.Errorf("webauthnx: parse creation: %w", err)
	}
	u := waUser{id: e.userID, name: ""}
	cred, err := p.w.CreateCredential(u, *e.data, parsed)
	if err != nil {
		return storeauth.WebauthnCredential{}, fmt.Errorf("webauthnx: create credential: %w", err)
	}
	return storeauth.WebauthnCredential{
		CredentialID:    cred.ID,
		PublicKey:       cred.PublicKey,
		AttestationType: cred.AttestationType,
		AAGUID:          cred.Authenticator.AAGUID,
		SignCount:       cred.Authenticator.SignCount,
		Flags:           cred.Flags.MsgpByte(),
		Transports:      fromTransports(cred.Transport),
	}, nil
}

func fromTransports(ts []protocol.AuthenticatorTransport) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, string(t))
	}
	return out
}

// BeginLogin 返回 assertion options JSON + token。
func (p *Provider) BeginLogin(_ context.Context, userID int64, username string, creds []storeauth.WebauthnCredential) ([]byte, string, error) {
	u := waUser{id: userID, name: username, creds: toCreds(creds)}
	opt, data, err := p.w.BeginLogin(u)
	if err != nil {
		return nil, "", fmt.Errorf("webauthnx: begin login: %w", err)
	}
	token, err := p.put(userID, data)
	if err != nil {
		return nil, "", err
	}
	b, err := json.Marshal(opt)
	return b, token, err
}

// FinishLogin 校验 assertion（凭据集由 service 层加载传入），返回凭据标识、新
// sign_count 与应回写的 flags 原始字节。对 v0.6.3 遗留 flags=0 的凭据做一次性自愈：
// 若因 BackupEligible 不一致失败且存储 flags 全零，按本次断言的 authenticator flags
// 修补后重试一次（BE 标志本应注册时固化，旧行缺失故以首次断言为准回写）。
func (p *Provider) FinishLogin(_ context.Context, token string, response []byte, creds []storeauth.WebauthnCredential) ([]byte, uint32, byte, error) {
	e, err := p.take(token)
	if err != nil {
		return nil, 0, 0, err
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(response)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("webauthnx: parse assertion: %w", err)
	}
	u := waUser{id: e.userID, name: "", creds: toCreds(creds)}
	cred, verr := p.w.ValidateLogin(u, *e.data, parsed)
	if verr != nil && isBackupEligibleInconsistency(verr) && allFlagsZero(creds) {
		flagsByte := byte(parsed.Response.AuthenticatorData.Flags)
		patched := make([]storeauth.WebauthnCredential, len(creds))
		copy(patched, creds)
		for i := range patched {
			patched[i].Flags = flagsByte
		}
		u2 := waUser{id: e.userID, name: "", creds: toCreds(patched)}
		if cred2, err2 := p.w.ValidateLogin(u2, *e.data, parsed); err2 == nil {
			return cred2.ID, cred2.Authenticator.SignCount, flagsByte, nil
		}
		return nil, 0, 0, fmt.Errorf("webauthnx: validate login: %w", verr)
	}
	if verr != nil {
		return nil, 0, 0, fmt.Errorf("webauthnx: validate login: %w", verr)
	}
	return cred.ID, cred.Authenticator.SignCount, cred.Flags.MsgpByte(), nil
}

func isBackupEligibleInconsistency(err error) bool {
	return err != nil && strings.Contains(err.Error(), "Backup Eligible flag inconsistency")
}

func allFlagsZero(creds []storeauth.WebauthnCredential) bool {
	for _, c := range creds {
		if c.Flags != 0 {
			return false
		}
	}
	return true
}
