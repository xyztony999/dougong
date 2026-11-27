package auth

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"time"

	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	storeauth "github.com/dougong/dougong/internal/store/auth"
)

// Service 是 account-auth 应用服务（聚合初始化/登录/会话/用户/审计查询编排）。
type Service struct {
	st      Store
	hasher  PasswordHasher
	totp    TotpVerifier
	policy  SessionPolicy
	tickets *ticketStore
	clock   func() time.Time
	rand    io.Reader
	wa      WebAuthnFactory
}

// Option 是 Service 可选注入（测试用时钟/随机源）。
type Option func(*Service)

// WithClock 注入时钟（默认 time.Now）。
func WithClock(clock func() time.Time) Option {
	return func(s *Service) {
		if clock != nil {
			s.clock = clock
		}
	}
}

// WithRand 注入随机源（默认 crypto/rand）。
func WithRand(r io.Reader) Option {
	return func(s *Service) {
		if r != nil {
			s.rand = r
		}
	}
}

// WithWebAuthnFactory 注入 WebAuthn provider 工厂（main 装配 webauthnx；测试注入 fake）。
func WithWebAuthnFactory(f WebAuthnFactory) Option {
	return func(s *Service) {
		if f != nil {
			s.wa = f
		}
	}
}

// New 构造服务。policy 零值时取默认 12h/2h。
func New(st Store, hasher PasswordHasher, totp TotpVerifier, policy SessionPolicy, opts ...Option) *Service {
	if policy.AbsoluteTimeout <= 0 || policy.IdleTimeout <= 0 {
		policy = DefaultSessionPolicy()
	}
	s := &Service{
		st:     st,
		hasher: hasher,
		totp:   totp,
		policy: policy,
		clock:  time.Now,
		rand:   rand.Reader,
	}
	// 闭包引用 s.clock：WithClock 注入后 ticketStore 同步生效。
	s.tickets = newTicketStore(func() time.Time { return s.clock() })
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// now 取当前时刻（统一入口，测试可控）。
func (s *Service) now() time.Time { return s.clock().UTC() }

// withTx 事务编排助手：fn 返回错误即回滚，成功提交（I-05 同事务审计）。
func (s *Service) withTx(ctx context.Context, fn func(tx *storeauth.TxStore) error) error {
	tx, err := s.st.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("auth: begin tx: %w", err)
	}
	if err := fn(tx); err != nil {
		if rerr := tx.Rollback(); rerr != nil {
			return fmt.Errorf("auth: rollback: %w (tx error: %v)", rerr, err)
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("auth: commit: %w", err)
	}
	return nil
}

// createSession 在给定事务内创建会话行（不发审计）；调用方负责事务内
// 业务审计，保证原子性。返回 RawToken（Cookie 值）与摘要信息。
func (s *Service) createSession(ctx context.Context, tx *storeauth.TxStore, u storeauth.User, meta RequestMeta, now time.Time) (*SessionIssued, error) {
	raw, hash, err := newTokenPair(s.rand)
	if err != nil {
		return nil, fmt.Errorf("auth: generate session token: %w", err)
	}
	csrf, err := newCSRFToken(s.rand)
	if err != nil {
		return nil, fmt.Errorf("auth: generate csrf token: %w", err)
	}
	sess := storeauth.Session{
		TokenHash:     hash,
		UserID:        u.ID,
		CsrfToken:     csrf,
		CreatedAt:     now,
		LastActiveAt:  now,
		ExpiresAt:     now.Add(s.policy.AbsoluteTimeout),
		IdleExpiresAt: now.Add(s.policy.IdleTimeout),
		SourceIP:      strPtr(meta.SourceIP),
		UserAgent:     strPtr(meta.UserAgent),
	}
	if err := tx.Sessions().Create(ctx, sess); err != nil {
		return nil, fmt.Errorf("auth: create session: %w", err)
	}
	return &SessionIssued{
		RawToken:  raw,
		CsrfToken: csrf,
		MaxAgeSec: int(s.policy.AbsoluteTimeout.Seconds()),
		ExpiresAt: sess.ExpiresAt,
		User:      UserBrief{ID: u.ID, Username: u.Username, Role: u.Role},
	}, nil
}

// issueSessionWithLoginAudit 独立事务：签发会话 + login_success 审计（§2.3 登录编排）。
func (s *Service) issueSessionWithLoginAudit(ctx context.Context, u storeauth.User, meta RequestMeta, now time.Time, totpUsed bool) (*SessionIssued, error) {
	var issued *SessionIssued
	err := s.withTx(ctx, func(tx *storeauth.TxStore) error {
		// 登录成功即清零失败计数（R-01 连续失败语义）。
		if u.FailCount > 0 {
			if err := tx.Users().UpdateLockState(ctx, u.ID, 0, nil, storeauth.UserStatusActive, now); err != nil {
				return fmt.Errorf("reset fail count: %w", err)
			}
		}
		var err error
		issued, err = s.createSession(ctx, tx, u, meta, now)
		if err != nil {
			return err
		}
		detail := map[string]any{"sourceIp": meta.SourceIP}
		if totpUsed {
			detail["totp"] = true
		}
		// 新设备识别（登录保护强化）：device_hash=sha256(UA|设备cookie)，首次→标记+审计字段。
		if meta.DeviceCookie != "" {
			newDev, derr := tx.Devices().Touch(ctx, u.ID, deviceHash(meta.UserAgent, meta.DeviceCookie), meta.UserAgent, formatTimeNow(now))
			if derr != nil {
				return derr
			}
			issued.NewDevice = newDev
			if newDev {
				detail["new_device"] = true
			}
		}
		return tx.Audit().Append(ctx, loginAuditEntry(u, audit.ActionLoginSuccess, audit.ResultSuccess, detail), now)
	})
	if err != nil {
		return nil, err
	}
	return issued, nil
}

// loginAuditEntry 构造登录类审计（security/web，对象=user）。
func loginAuditEntry(u storeauth.User, action, result string, detail map[string]any) audit.Entry {
	id := u.ID
	return audit.Entry{
		ActorID:    &id,
		ActorName:  u.Username,
		Category:   audit.CategorySecurity,
		Action:     action,
		ObjectType: audit.ObjectTypeUser,
		ObjectID:   strconv.FormatInt(u.ID, 10),
		Detail:     detail,
		Result:     result,
		Source:     audit.SourceWeb,
	}
}

// auditLoginFailedStandalone 独立事务写 login_failed（无业务变更伴随时；
// 用户不存在/未达锁定阈值场景），失败仅告警不阻断响应。
func (s *Service) auditLoginFailedStandalone(ctx context.Context, username string, userID *int64, meta RequestMeta, stage string) {
	detail := map[string]any{"stage": stage, "sourceIp": meta.SourceIP}
	e := audit.Entry{
		ActorID:    userID,
		ActorName:  username,
		Category:   audit.CategorySecurity,
		Action:     audit.ActionLoginFailed,
		ObjectType: audit.ObjectTypeUser,
		Detail:     detail,
		Result:     audit.ResultFailure,
		Source:     audit.SourceWeb,
	}
	if err := s.st.Audit().Append(ctx, e, s.now()); err != nil {
		slog.WarnContext(ctx, "auth: audit login_failed append", "err", err)
	}
}

// actorEntry 构造以当前登录者为 actor 的审计四要素基底。
func actorEntry(id *Identity, category, action, objectType, objectID string) audit.Entry {
	return audit.Entry{
		ActorID:    &id.UserID,
		ActorName:  id.Username,
		Category:   category,
		Action:     action,
		ObjectType: objectType,
		ObjectID:   objectID,
		Result:     audit.ResultSuccess,
		Source:     audit.SourceWeb,
	}
}

// invalid 构造 AUTH_012 逐字段校验错误。
func invalid(fields ...errcode.FieldError) *errcode.Error {
	return errcode.ErrValidation.WithFields(fields...)
}

func strPtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
