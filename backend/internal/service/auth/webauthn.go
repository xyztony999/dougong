package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	storeauth "github.com/dougong/dougong/internal/store/auth"
)

// provider 按请求构造 WebAuthn provider（RP ID=Host 去端口；Origin=请求 Origin）。
// 未装配/缺 Host/Origin → AUTH_018（当前地址不支持 passkey）。
func (s *Service) provider(meta RequestMeta) (WebAuthnProvider, error) {
	if s.wa == nil || meta.Host == "" || meta.Origin == "" {
		return nil, errcode.ErrWebAuthnUnavailable
	}
	rpID := meta.Host
	if i := strings.LastIndex(rpID, ":"); i > 0 {
		rpID = rpID[:i]
	}
	// 允许的 origin 放宽为同主机的多种形态：反向代理/CDN（如 Cloudflare 橙云）可能
	// 归一化或剥离 Origin 头的端口，而浏览器 clientData 的 origin 带非默认端口，
	// 单一 origin 会校验失败（AUTH_015）。同主机不同 scheme/端口视为等价。
	seen := map[string]bool{}
	var origins []string
	add := func(o string) {
		if o != "" && !seen[o] {
			seen[o] = true
			origins = append(origins, o)
		}
	}
	add(meta.Origin)
	for _, scheme := range []string{"https", "http"} {
		add(scheme + "://" + meta.Host)
		add(scheme + "://" + rpID)
	}
	p := s.wa(rpID, origins)
	if p == nil {
		return nil, errcode.ErrWebAuthnUnavailable
	}
	return p, nil
}

// RegisterBegin POST /auth/webauthn/register-begin（会话）。
func (s *Service) RegisterBegin(ctx context.Context, id *Identity, meta RequestMeta) (*WebAuthnBeginResponse, error) {
	p, err := s.provider(meta)
	if err != nil {
		return nil, err
	}
	creds, err := s.st.Webauthn().ListByUser(ctx, id.UserID)
	if err != nil {
		return nil, fmt.Errorf("auth: webauthn list: %w", err)
	}
	opt, token, err := p.BeginRegistration(ctx, id.UserID, id.Username, creds)
	if err != nil {
		return nil, errcode.ErrWebAuthnInvalid
	}
	return &WebAuthnBeginResponse{Options: opt, Token: token}, nil
}

// RegisterFinish POST /auth/webauthn/register-finish（会话）：存凭据 + 审计。
func (s *Service) RegisterFinish(ctx context.Context, id *Identity, req WebAuthnRegisterFinishRequest, meta RequestMeta) (*WebAuthnCredentialItem, error) {
	p, err := s.provider(meta)
	if err != nil {
		return nil, err
	}
	cred, err := p.FinishRegistration(ctx, req.Token, req.Credential)
	if err != nil {
		slog.WarnContext(ctx, "auth: webauthn register finish", "err", err)
		return nil, errcode.ErrWebAuthnInvalid
	}
	now := s.now()
	cred.UserID = id.UserID
	cred.Name = strings.TrimSpace(req.Name)
	if cred.Name == "" {
		cred.Name = "passkey"
	}
	cred.CreatedAt = now
	var item WebAuthnCredentialItem
	err = s.withTx(ctx, func(tx *storeauth.TxStore) error {
		cid, err := tx.Webauthn().Create(ctx, cred)
		if err != nil {
			return err
		}
		item = WebAuthnCredentialItem{ID: cid, Name: cred.Name, CreatedAt: now}
		return tx.Audit().Append(ctx, actorEntry(id, audit.CategorySecurity, "webauthn_registered", audit.ObjectTypeUser, fmt.Sprintf("%d", id.UserID)), now)
	})
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// ListCredentials GET /auth/webauthn/list（会话，脱敏）。
func (s *Service) ListCredentials(ctx context.Context, id *Identity) ([]WebAuthnCredentialItem, error) {
	creds, err := s.st.Webauthn().ListByUser(ctx, id.UserID)
	if err != nil {
		return nil, fmt.Errorf("auth: webauthn list: %w", err)
	}
	out := make([]WebAuthnCredentialItem, 0, len(creds))
	for _, c := range creds {
		out = append(out, WebAuthnCredentialItem{ID: c.ID, Name: c.Name, CreatedAt: c.CreatedAt, LastUsedAt: c.LastUsedAt})
	}
	return out, nil
}

// DeleteCredential POST /auth/webauthn/delete（会话，本人凭据）。
func (s *Service) DeleteCredential(ctx context.Context, id *Identity, req WebAuthnDeleteRequest) error {
	if _, err := s.st.Webauthn().GetByID(ctx, id.UserID, req.ID); errors.Is(err, sql.ErrNoRows) {
		return errcode.ErrCredentialNotFound
	} else if err != nil {
		return fmt.Errorf("auth: webauthn get: %w", err)
	}
	now := s.now()
	return s.withTx(ctx, func(tx *storeauth.TxStore) error {
		if err := tx.Webauthn().Delete(ctx, id.UserID, req.ID); err != nil {
			return err
		}
		return tx.Audit().Append(ctx, actorEntry(id, audit.CategorySecurity, "webauthn_deleted", audit.ObjectTypeUser, fmt.Sprintf("%d", id.UserID)), now)
	})
}

// LoginBegin POST /auth/webauthn/login-begin（公开，无密码流）。
func (s *Service) LoginBegin(ctx context.Context, req WebAuthnLoginBeginRequest, meta RequestMeta) (*WebAuthnBeginResponse, error) {
	p, err := s.provider(meta)
	if err != nil {
		return nil, err
	}
	u, err := s.st.Users().GetByUsername(ctx, req.Username)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errcode.ErrBadCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("auth: webauthn login get user: %w", err)
	}
	creds, err := s.st.Webauthn().ListByUser(ctx, u.ID)
	if err != nil {
		return nil, fmt.Errorf("auth: webauthn login list: %w", err)
	}
	if len(creds) == 0 {
		return nil, errcode.ErrCredentialNotFound
	}
	opt, token, err := p.BeginLogin(ctx, u.ID, u.Username, creds)
	if err != nil {
		return nil, errcode.ErrWebAuthnInvalid
	}
	return &WebAuthnBeginResponse{Options: opt, Token: token}, nil
}

// LoginFinish POST /auth/webauthn/login-finish（公开，无密码登录）。
func (s *Service) LoginFinish(ctx context.Context, req WebAuthnLoginFinishRequest, meta RequestMeta) (*SessionIssued, error) {
	now := s.now()
	u, err := s.st.Users().GetByUsername(ctx, req.Username)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errcode.ErrBadCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("auth: webauthn finish get user: %w", err)
	}
	u, locked, err := s.resolveLockState(ctx, u, now)
	if err != nil {
		return nil, err
	}
	if locked != nil {
		return nil, locked
	}
	if _, err := s.assertPasskey(ctx, u, req.Token, req.Credential, meta); err != nil {
		return nil, err
	}
	return s.issueSessionWithLoginAudit(ctx, u, meta, now, false)
}

// PasskeyVerify POST /auth/session/passkey-verify（公开凭 ticket，密码后二因素）。
func (s *Service) PasskeyVerify(ctx context.Context, req WebAuthnLoginFinishRequest, meta RequestMeta) (*SessionIssued, error) {
	now := s.now()
	tk, ok := s.tickets.lookup(req.LoginTicket)
	if !ok {
		return nil, errcode.ErrTicketInvalid
	}
	u, err := s.st.Users().GetByID(ctx, tk.userID)
	if errors.Is(err, sql.ErrNoRows) {
		s.tickets.consume(req.LoginTicket)
		return nil, errcode.ErrTicketInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("auth: passkey verify get user: %w", err)
	}
	u, locked, err := s.resolveLockState(ctx, u, now)
	if err != nil {
		return nil, err
	}
	if locked != nil {
		return nil, locked
	}
	if _, err := s.assertPasskey(ctx, u, req.Token, req.Credential, meta); err != nil {
		return nil, err
	}
	if !s.tickets.consume(req.LoginTicket) {
		return nil, errcode.ErrTicketInvalid
	}
	return s.issueSessionWithLoginAudit(ctx, u, meta, now, false)
}

// assertPasskey 校验断言并更新 sign_count/last_used；失败计 R-01。
func (s *Service) assertPasskey(ctx context.Context, u storeauth.User, token string, response []byte, meta RequestMeta) ([]byte, error) {
	p, err := s.provider(meta)
	if err != nil {
		return nil, err
	}
	creds, err := s.st.Webauthn().ListByUser(ctx, u.ID)
	if err != nil {
		return nil, fmt.Errorf("auth: assert list: %w", err)
	}
	credID, signCount, flags, err := p.FinishLogin(ctx, token, response, creds)
	if err != nil {
		slog.WarnContext(ctx, "auth: webauthn assert", "err", err, "user", u.Username)
		if rerr := s.recordLoginFailure(ctx, u, s.now(), meta, "passkey"); rerr != nil {
			return nil, rerr
		}
		return nil, errcode.ErrWebAuthnInvalid
	}
	// 更新匹配凭据的 sign_count/flags/last_used（flags 回写使 v0.6.3 旧行自愈持久化）。
	for _, c := range creds {
		if string(c.CredentialID) == string(credID) {
			if err := s.st.Webauthn().Touch(ctx, c.ID, signCount, flags, formatTimeNow(s.now())); err != nil {
				return nil, fmt.Errorf("auth: webauthn touch: %w", err)
			}
			break
		}
	}
	return credID, nil
}

func formatTimeNow(t time.Time) string { return t.UTC().Format(time.RFC3339) }
