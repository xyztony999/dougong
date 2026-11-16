package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	storeauth "github.com/dougong/dougong/internal/store/auth"
)

// Login POST /auth/session/login（§4.2.4 登录编排）：
// 已初始化(I-01) → 用户存在且 active(I-04) → 未锁定(I-08，惰性解锁) →
// 密码比对（失败计数 R-01）→ [启用 TOTP：签发 login_ticket（D-02）] →
// 签发会话 + login_success 审计。错误统一 AUTH_00x 语义，防枚举不区分提示。
func (s *Service) Login(ctx context.Context, req LoginRequest, meta RequestMeta) (*LoginOutcome, error) {
	now := s.now()

	// I-01：系统初始化前禁止会话签发。
	st, err := s.st.Init().Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("auth: login read init state: %w", err)
	}
	if !st.Initialized {
		return nil, errcode.ErrAlreadyInitialized
	}

	u, err := s.st.Users().GetByUsername(ctx, req.Username)
	if errors.Is(err, sql.ErrNoRows) {
		// 用户不存在：与密码错误同提示（防枚举），并记 login_failed 审计。
		s.auditLoginFailedStandalone(ctx, req.Username, nil, meta, "password")
		return nil, errcode.ErrBadCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("auth: login get user: %w", err)
	}

	u, locked, err := s.resolveLockState(ctx, u, now)
	if err != nil {
		return nil, err
	}
	if locked != nil {
		return nil, locked
	}

	// 密码比对（常量时间；错误：fail_count+1，达 5 次锁定并审计 account_locked）。
	if !s.hasher.Verify(u.PasswordHash, req.Password) {
		if err := s.recordLoginFailure(ctx, u, now, meta, "password"); err != nil {
			return nil, err
		}
		return nil, errcode.ErrBadCredentials
	}

	// 两步式：启用 TOTP → 签发一次性 login_ticket（AUTH_007 由 handler 表达）。
	cred, err := s.st.Totp().GetByUser(ctx, u.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("auth: login get totp: %w", err)
	}
	if err == nil && cred.Enabled {
		ticket, err := newTicketToken(s.rand)
		if err != nil {
			return nil, fmt.Errorf("auth: login ticket: %w", err)
		}
		// 密码已通过：清零失败计数再签发 ticket（R-01 连续失败语义）。
		if u.FailCount > 0 {
			if err := s.resetFailCount(ctx, u.ID, now); err != nil {
				return nil, err
			}
		}
		s.tickets.issue(ticket, u.ID, u.Username, now.Add(LoginTicketTTL))
		return &LoginOutcome{RequiresTOTP: true, LoginTicket: ticket}, nil
	}

	issued, err := s.issueSessionWithLoginAudit(ctx, u, meta, now, false)
	if err != nil {
		return nil, err
	}
	return &LoginOutcome{Session: issued}, nil
}

// TOTPVerify POST /auth/session/totp-verify（D-02 二阶段）：
// ticket 存在未过期（AUTH_013）→ 用户/锁定复核（I-04/I-08）→ 动态码验证
// （错误 AUTH_006 并计入 R-01 计数）→ 吊销 ticket（一次性）→ 签发会话。
func (s *Service) TOTPVerify(ctx context.Context, req TOTPVerifyRequest, meta RequestMeta) (*SessionIssued, error) {
	now := s.now()
	tk, ok := s.tickets.lookup(req.LoginTicket)
	if !ok {
		return nil, errcode.ErrTicketInvalid
	}
	u, err := s.st.Users().GetByID(ctx, tk.userID)
	if errors.Is(err, sql.ErrNoRows) {
		// 用户在两阶段间被删除：ticket 语义失效。
		s.tickets.consume(req.LoginTicket)
		return nil, errcode.ErrTicketInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("auth: totp verify get user: %w", err)
	}
	u, locked, err := s.resolveLockState(ctx, u, now)
	if err != nil {
		return nil, err
	}
	if locked != nil {
		return nil, locked
	}

	cred, err := s.st.Totp().GetByUser(ctx, u.ID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !cred.Enabled) {
		return nil, errcode.ErrTotpInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("auth: totp verify get cred: %w", err)
	}
	if !s.totp.Verify(cred.Secret, req.Code, now) {
		// 动态码错误计一次失败入 R-01 计数（§4.2.4）。
		if err := s.recordLoginFailure(ctx, u, now, meta, "totp"); err != nil {
			return nil, err
		}
		return nil, errcode.ErrTotpInvalid
	}
	if !s.tickets.consume(req.LoginTicket) {
		return nil, errcode.ErrTicketInvalid
	}
	return s.issueSessionWithLoginAudit(ctx, u, meta, now, true)
}

// resolveLockState 处理惰性解锁（I-08：锁定期过自动归零恢复 active）并返回
// 锁定错误（含 remainingSeconds）；未锁定返回 (u, nil, nil)。
func (s *Service) resolveLockState(ctx context.Context, u storeauth.User, now time.Time) (storeauth.User, *errcode.Error, error) {
	if u.Status != storeauth.UserStatusLocked {
		return u, nil, nil
	}
	// locked 状态但锁期已过：惰性解锁并落库归零。
	if u.LockedUntil == nil || !now.Before(*u.LockedUntil) {
		if err := s.withTx(ctx, func(tx *storeauth.TxStore) error {
			if err := tx.Users().UpdateLockState(ctx, u.ID, 0, nil, storeauth.UserStatusActive, now); err != nil {
				return fmt.Errorf("lazy unlock: %w", err)
			}
			// 解锁事件审计（动作码 account_unlocked，§3.4 常量表）。
			return tx.Audit().Append(ctx, audit.Entry{
				ActorID:    &u.ID,
				ActorName:  u.Username,
				Category:   audit.CategorySecurity,
				Action:     audit.ActionAccountUnlocked,
				ObjectType: audit.ObjectTypeUser,
				ObjectID:   strconv.FormatInt(u.ID, 10),
				Result:     audit.ResultSuccess,
				Source:     audit.SourceSystem,
			}, now)
		}); err != nil {
			return u, nil, fmt.Errorf("auth: lazy unlock: %w", err)
		}
		u.Status, u.FailCount, u.LockedUntil = storeauth.UserStatusActive, 0, nil
		return u, nil, nil
	}
	remaining := int64(math.Ceil(u.LockedUntil.Sub(now).Seconds()))
	if remaining < 0 {
		remaining = 0
	}
	return u, errcode.ErrAccountLocked.WithData(map[string]int64{"remainingSeconds": remaining}), nil
}

// recordLoginFailure 密码/动态码失败处理：fail_count+1；达 MaxFailAttempts 落
// locked（15 分钟）并在同一事务写 account_locked 审计（I-05）；未达阈值时
// 独立写 login_failed 审计。
func (s *Service) recordLoginFailure(ctx context.Context, u storeauth.User, now time.Time, meta RequestMeta, stage string) error {
	fail := u.FailCount + 1
	willLock := fail >= MaxFailAttempts
	var lockedUntil *time.Time
	status := storeauth.UserStatusActive
	if willLock {
		t := now.Add(LockoutDuration)
		lockedUntil = &t
		status = storeauth.UserStatusLocked
	}
	err := s.withTx(ctx, func(tx *storeauth.TxStore) error {
		if err := tx.Users().UpdateLockState(ctx, u.ID, fail, lockedUntil, status, now); err != nil {
			return fmt.Errorf("update fail count: %w", err)
		}
		if willLock {
			return tx.Audit().Append(ctx, audit.Entry{
				ActorID:    &u.ID,
				ActorName:  u.Username,
				Category:   audit.CategorySecurity,
				Action:     audit.ActionAccountLocked,
				ObjectType: audit.ObjectTypeUser,
				ObjectID:   strconv.FormatInt(u.ID, 10),
				Detail:     map[string]any{"failCount": fail, "lockedMinutes": int(LockoutDuration.Minutes())},
				Result:     audit.ResultSuccess,
				Source:     audit.SourceWeb,
			}, now)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("auth: record login failure: %w", err)
	}
	if !willLock {
		s.auditLoginFailedStandalone(ctx, u.Username, &u.ID, meta, stage)
	}
	return nil
}

// resetFailCount 独立事务清零失败计数（成功路径）。
func (s *Service) resetFailCount(ctx context.Context, userID int64, now time.Time) error {
	return s.withTx(ctx, func(tx *storeauth.TxStore) error {
		return tx.Users().UpdateLockState(ctx, userID, 0, nil, storeauth.UserStatusActive, now)
	})
}
