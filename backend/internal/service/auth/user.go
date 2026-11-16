package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	storeauth "github.com/dougong/dougong/internal/store/auth"
)

// ListUsers GET /auth/user/list：全量用户（脱敏，§4.1.4）。
func (s *Service) ListUsers(ctx context.Context) ([]UserInfo, error) {
	users, err := s.st.Users().List(ctx)
	if err != nil {
		return nil, fmt.Errorf("auth: list users: %w", err)
	}
	items := make([]UserInfo, 0, len(users))
	for _, u := range users {
		cred, err := s.st.Totp().GetByUser(ctx, u.ID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("auth: list users totp: %w", err)
		}
		items = append(items, UserInfo{
			ID:          u.ID,
			Username:    u.Username,
			Role:        u.Role,
			Status:      u.Status,
			TotpEnabled: err == nil && cred.Enabled,
			LockedUntil: u.LockedUntil,
			CreatedAt:   u.CreatedAt,
		})
	}
	return items, nil
}

// CreateUser POST /auth/user/create（admin）：校验 I-10/A10/角色枚举 + I-06
// 唯一性（大小写不敏感）→ 建用户 → user_created 审计（同事务）。
func (s *Service) CreateUser(ctx context.Context, actor *Identity, req CreateUserRequest) (*CreatedUser, error) {
	now := s.now()
	var fields []errcode.FieldError
	if fe := validateUsername(req.Username); fe != nil {
		fields = append(fields, *fe)
	}
	if fe := validatePassword(req.Password); fe != nil {
		fields = append(fields, *fe)
	}
	if fe := validateRole(req.Role); fe != nil {
		fields = append(fields, *fe)
	}
	if len(fields) > 0 {
		return nil, invalid(fields...)
	}
	if _, err := s.st.Users().GetByUsername(ctx, req.Username); err == nil {
		return nil, errcode.ErrUsernameTaken // I-06
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("auth: create user check: %w", err)
	}
	passwordHash, err := s.hasher.Hash(req.Password)
	if err != nil {
		return nil, fmt.Errorf("auth: create user hash: %w", err)
	}
	var created *CreatedUser
	err = s.withTx(ctx, func(tx *storeauth.TxStore) error {
		id, err := tx.Users().Create(ctx, storeauth.User{
			Username: req.Username, PasswordHash: passwordHash, Role: req.Role,
		}, now)
		if err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		entry := actorEntry(actor, audit.CategoryChange, audit.ActionUserCreated,
			audit.ObjectTypeUser, strconv.FormatInt(id, 10))
		entry.Detail = map[string]any{"username": req.Username, "role": req.Role}
		if err := tx.Audit().Append(ctx, entry, now); err != nil {
			return fmt.Errorf("audit user_created: %w", err)
		}
		created = &CreatedUser{ID: id, Username: req.Username, Role: req.Role, CreatedAt: now}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// DeleteUser POST /auth/user/delete（admin）：D-09 禁删自身、I-03 唯一管理员保护；
// 级联吊销其全部会话（end_reason=cascade）后删除用户，user_deleted 审计同事务。
func (s *Service) DeleteUser(ctx context.Context, actor *Identity, userID int64) error {
	now := s.now()
	target, err := s.st.Users().GetByID(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return invalid(errcode.FieldError{Field: "userId", Message: "用户不存在"})
	}
	if err != nil {
		return fmt.Errorf("auth: delete user get: %w", err)
	}
	if target.ID == actor.UserID {
		return errcode.ErrSelfDelete // D-09
	}
	if target.Role == storeauth.RoleAdmin {
		admins, err := s.st.Users().CountAdmins(ctx)
		if err != nil {
			return fmt.Errorf("auth: delete user count admins: %w", err)
		}
		if admins <= 1 {
			return errcode.ErrLastAdmin // I-03
		}
	}
	return s.withTx(ctx, func(tx *storeauth.TxStore) error {
		// 级联吊销（I-04）：显式写 end_reason=cascade 后删用户
		//（panel_session 行由 FK ON DELETE CASCADE 清理，吊销语义先落库）。
		if err := tx.Sessions().RevokeByUser(ctx, target.ID, now, storeauth.EndReasonCascade, ""); err != nil {
			return fmt.Errorf("cascade revoke sessions: %w", err)
		}
		if err := tx.Users().Delete(ctx, target.ID); err != nil {
			return fmt.Errorf("delete user: %w", err)
		}
		entry := actorEntry(actor, audit.CategoryChange, audit.ActionUserDeleted,
			audit.ObjectTypeUser, strconv.FormatInt(target.ID, 10))
		entry.Detail = map[string]any{"username": target.Username, "role": target.Role}
		return tx.Audit().Append(ctx, entry, now)
	})
}

// ResetPassword POST /auth/user/reset-password（admin）：重置密码并吊销该用户
// 全部会话（cascade），password_reset（security）审计同事务。
func (s *Service) ResetPassword(ctx context.Context, actor *Identity, userID int64, newPassword string) error {
	now := s.now()
	if fe := validatePassword(newPassword); fe != nil {
		return invalid(*fe)
	}
	target, err := s.st.Users().GetByID(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return invalid(errcode.FieldError{Field: "userId", Message: "用户不存在"})
	}
	if err != nil {
		return fmt.Errorf("auth: reset password get: %w", err)
	}
	passwordHash, err := s.hasher.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("auth: reset password hash: %w", err)
	}
	return s.withTx(ctx, func(tx *storeauth.TxStore) error {
		if err := tx.Users().UpdatePassword(ctx, target.ID, passwordHash, now); err != nil {
			return fmt.Errorf("update password: %w", err)
		}
		if err := tx.Sessions().RevokeByUser(ctx, target.ID, now, storeauth.EndReasonCascade, ""); err != nil {
			return fmt.Errorf("cascade revoke sessions: %w", err)
		}
		entry := actorEntry(actor, audit.CategorySecurity, audit.ActionPasswordReset,
			audit.ObjectTypeUser, strconv.FormatInt(target.ID, 10))
		entry.Detail = map[string]any{"username": target.Username}
		return tx.Audit().Append(ctx, entry, now)
	})
}

// ChangePassword POST /auth/user/password-change（本人）：旧密码常量时间比对、
// 新密码 ≥8；成功吊销本人其他会话（保留当前），password_changed 审计同事务。
func (s *Service) ChangePassword(ctx context.Context, id *Identity, oldPassword, newPassword string) error {
	now := s.now()
	if fe := validatePassword(newPassword); fe != nil {
		return invalid(*fe)
	}
	u, err := s.st.Users().GetByID(ctx, id.UserID)
	if err != nil {
		return fmt.Errorf("auth: change password get: %w", err)
	}
	if !s.hasher.Verify(u.PasswordHash, oldPassword) {
		return errcode.ErrBadCredentials
	}
	passwordHash, err := s.hasher.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("auth: change password hash: %w", err)
	}
	return s.withTx(ctx, func(tx *storeauth.TxStore) error {
		if err := tx.Users().UpdatePassword(ctx, u.ID, passwordHash, now); err != nil {
			return fmt.Errorf("update password: %w", err)
		}
		// 保留当前会话，其余全部级联吊销（§4.1.4）。
		if err := tx.Sessions().RevokeByUser(ctx, u.ID, now, storeauth.EndReasonCascade, id.TokenHash); err != nil {
			return fmt.Errorf("cascade revoke other sessions: %w", err)
		}
		entry := actorEntry(id, audit.CategorySecurity, audit.ActionPasswordChanged,
			audit.ObjectTypeUser, strconv.FormatInt(u.ID, 10))
		entry.Detail = map[string]any{"username": u.Username}
		return tx.Audit().Append(ctx, entry, now)
	})
}

// TOTPSetup POST /auth/user/totp-setup：生成待确认密钥（覆盖未激活旧值），
// otpauth URI + secret 仅此一次下发（I-09）；未生效不审计。已启用者须先解绑。
func (s *Service) TOTPSetup(ctx context.Context, id *Identity) (*TotpSetupResponse, error) {
	if cred, err := s.st.Totp().GetByUser(ctx, id.UserID); err == nil && cred.Enabled {
		return nil, invalid(errcode.FieldError{Field: "totp", Message: "已绑定动态码，请先解绑后再重新生成"})
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("auth: totp setup get: %w", err)
	}
	secret, err := s.totp.GenerateSecret()
	if err != nil {
		return nil, fmt.Errorf("auth: totp setup secret: %w", err)
	}
	if err := s.st.Totp().Upsert(ctx, storeauth.TotpCredential{
		UserID: id.UserID, Secret: secret, Enabled: false,
	}); err != nil {
		return nil, fmt.Errorf("auth: totp setup upsert: %w", err)
	}
	return &TotpSetupResponse{
		OtpauthURI: s.totp.OtpauthURI(secret, id.Username),
		Secret:     secret,
	}, nil
}

// TOTPEnable POST /auth/user/totp-enable：验证待确认密钥的动态码后激活
// （enabled=1、confirmed_at=now），totp_enabled（security）审计同事务。
func (s *Service) TOTPEnable(ctx context.Context, id *Identity, code string) error {
	now := s.now()
	cred, err := s.st.Totp().GetByUser(ctx, id.UserID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && cred.Enabled) {
		return errcode.ErrTotpInvalid
	}
	if err != nil {
		return fmt.Errorf("auth: totp enable get: %w", err)
	}
	if !s.totp.Verify(cred.Secret, code, now) {
		return errcode.ErrTotpInvalid
	}
	return s.withTx(ctx, func(tx *storeauth.TxStore) error {
		if err := tx.Totp().Upsert(ctx, storeauth.TotpCredential{
			UserID: id.UserID, Secret: cred.Secret, Enabled: true, ConfirmedAt: &now,
		}); err != nil {
			return fmt.Errorf("totp enable upsert: %w", err)
		}
		entry := actorEntry(id, audit.CategorySecurity, audit.ActionTotpEnabled,
			audit.ObjectTypeUser, strconv.FormatInt(id.UserID, 10))
		return tx.Audit().Append(ctx, entry, now)
	})
}

// TOTPDisable POST /auth/user/totp-disable：密码确认后解绑（删除行），
// totp_disabled（security）审计同事务。
func (s *Service) TOTPDisable(ctx context.Context, id *Identity, password string) error {
	now := s.now()
	u, err := s.st.Users().GetByID(ctx, id.UserID)
	if err != nil {
		return fmt.Errorf("auth: totp disable get user: %w", err)
	}
	if !s.hasher.Verify(u.PasswordHash, password) {
		return errcode.ErrBadCredentials
	}
	if _, err := s.st.Totp().GetByUser(ctx, id.UserID); errors.Is(err, sql.ErrNoRows) {
		return invalid(errcode.FieldError{Field: "totp", Message: "未绑定动态码"})
	} else if err != nil {
		return fmt.Errorf("auth: totp disable get: %w", err)
	}
	return s.withTx(ctx, func(tx *storeauth.TxStore) error {
		if err := tx.Totp().Delete(ctx, id.UserID); err != nil {
			return fmt.Errorf("totp disable delete: %w", err)
		}
		entry := actorEntry(id, audit.CategorySecurity, audit.ActionTotpDisabled,
			audit.ObjectTypeUser, strconv.FormatInt(id.UserID, 10))
		return tx.Audit().Append(ctx, entry, now)
	})
}

// CurrentUserInfo GET /auth/session/current 的 data 组装（authenticated/user/csrfToken）。
func (s *Service) CurrentUserInfo(id *Identity) map[string]any {
	return map[string]any{
		"authenticated": true,
		"user":          UserBrief{ID: id.UserID, Username: id.Username, Role: id.Role},
		"csrfToken":     id.CsrfToken,
	}
}
