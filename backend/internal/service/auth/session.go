package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	storeauth "github.com/dougong/dougong/internal/store/auth"
)

// VerifyToken 验证会话（AuthRequired 中间件消费）：
// I-11 计算态判定（revoked/expired）+ 滑动续期（Touch last_active_at/
// idle_expires_at，ADR-019）。无效→AUTH_001；过期→AUTH_002。
func (s *Service) VerifyToken(ctx context.Context, rawToken string) (*Identity, error) {
	now := s.now()
	hash := sha256Hex(rawToken)
	sess, err := s.st.Sessions().GetByTokenHash(ctx, hash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errcode.ErrUnauthenticated
	}
	if err != nil {
		return nil, fmt.Errorf("auth: verify get session: %w", err)
	}
	if sess.RevokedAt != nil {
		return nil, errcode.ErrUnauthenticated // I-13 吊销立即生效
	}
	if !now.Before(sess.ExpiresAt) || !now.Before(sess.IdleExpiresAt) {
		return nil, errcode.ErrSessionExpired // D-03 计算态过期
	}
	u, err := s.st.Users().GetByID(ctx, sess.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errcode.ErrUnauthenticated
	}
	if err != nil {
		return nil, fmt.Errorf("auth: verify get user: %w", err)
	}
	// 滑动续期：写失败不影响本次认证（记日志），下次请求重试。
	idle := now.Add(s.policy.IdleTimeout)
	if err := s.st.Sessions().Touch(ctx, hash, now, idle); err != nil {
		slog.WarnContext(ctx, "auth: session touch failed", "digest", sess.Digest(), "err", err)
	}
	return &Identity{
		UserID:        u.ID,
		Username:      u.Username,
		Role:          u.Role,
		TokenHash:     hash,
		SessionDigest: sess.Digest(),
		CsrfToken:     sess.CsrfToken,
		ExpiresAt:     sess.ExpiresAt,
	}, nil
}

// VerifyWs 是 SessionAuthService.VerifyWs OHS（R-18，terminal WS 握手复用）：
// 与 VerifyToken 同一 I-11 判定，只读不续期。
func (s *Service) VerifyWs(ctx context.Context, rawToken string) (userID int64, role string, err error) {
	now := s.now()
	sess, err := s.st.Sessions().GetByTokenHash(ctx, sha256Hex(rawToken))
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", errcode.ErrUnauthenticated
	}
	if err != nil {
		return 0, "", fmt.Errorf("auth: verify ws get session: %w", err)
	}
	if sess.RevokedAt != nil {
		return 0, "", errcode.ErrUnauthenticated
	}
	if !now.Before(sess.ExpiresAt) || !now.Before(sess.IdleExpiresAt) {
		return 0, "", errcode.ErrSessionExpired
	}
	u, err := s.st.Users().GetByID(ctx, sess.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", errcode.ErrUnauthenticated
	}
	if err != nil {
		return 0, "", fmt.Errorf("auth: verify ws get user: %w", err)
	}
	return u.ID, u.Role, nil
}

// Logout POST /auth/session/logout：吊销当前会话（self_logout）+ 审计 logout。
func (s *Service) Logout(ctx context.Context, id *Identity, rawToken string) error {
	now := s.now()
	return s.withTx(ctx, func(tx *storeauth.TxStore) error {
		if err := tx.Sessions().Revoke(ctx, sha256Hex(rawToken), now, storeauth.EndReasonSelfLogout); err != nil {
			return fmt.Errorf("logout revoke: %w", err)
		}
		entry := actorEntry(id, audit.CategorySecurity, audit.ActionLogout, audit.ObjectTypeSession, id.SessionDigest)
		return tx.Audit().Append(ctx, entry, now)
	})
}

// ListSessions GET /auth/session/list：有效会话清单（I-11），lastActiveAt 倒序，
// current 标记当前会话；脱敏不下发 token 哈希与 csrf_token。
func (s *Service) ListSessions(ctx context.Context, id *Identity) ([]SessionItem, error) {
	now := s.now()
	sessions, err := s.st.Sessions().ListActive(ctx, now)
	if err != nil {
		return nil, fmt.Errorf("auth: list sessions: %w", err)
	}
	items := make([]SessionItem, 0, len(sessions))
	for _, sess := range sessions {
		ip := ""
		if sess.SourceIP != nil {
			ip = *sess.SourceIP
		}
		items = append(items, SessionItem{
			ID:           sess.Digest(),
			Username:     sess.Username,
			CreatedAt:    sess.CreatedAt,
			LastActiveAt: sess.LastActiveAt,
			SourceIP:     ip,
			Current:      sess.Digest() == id.SessionDigest,
		})
	}
	return items, nil
}

// RevokeSession POST /auth/session/revoke（admin）：强制下线目标有效会话
// （end_reason=forced_logout）+ 审计 force_logout；不得吊销当前会话自身
// （登出走 logout，设计文档 §4.2.4）。
func (s *Service) RevokeSession(ctx context.Context, actor *Identity, sessionID string) error {
	now := s.now()
	target, err := s.st.Sessions().FindActiveByDigest(ctx, sessionID, now)
	if errors.Is(err, sql.ErrNoRows) {
		return invalid(errcode.FieldError{Field: "sessionId", Message: "会话不存在或已失效"})
	}
	if err != nil {
		return fmt.Errorf("auth: revoke find session: %w", err)
	}
	if target.Digest() == actor.SessionDigest {
		return invalid(errcode.FieldError{Field: "sessionId", Message: "不能吊销当前会话，请使用登出"})
	}
	return s.withTx(ctx, func(tx *storeauth.TxStore) error {
		if err := tx.Sessions().Revoke(ctx, target.TokenHash, now, storeauth.EndReasonForcedLogout); err != nil {
			return fmt.Errorf("revoke session: %w", err)
		}
		entry := actorEntry(actor, audit.CategoryChange, audit.ActionForceLogout, audit.ObjectTypeSession, target.Digest())
		entry.Detail = map[string]any{"username": target.Username, "sourceIp": deref(target.SourceIP)}
		return tx.Audit().Append(ctx, entry, now)
	})
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
