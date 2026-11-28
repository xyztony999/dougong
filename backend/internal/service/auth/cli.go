package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	storeauth "github.com/dougong/dougong/internal/store/auth"
)

// 本文件是 CLI 管理通道（api-panel-core.md v2.1 4.1.4「CLI 入口」，ADR-030/
// D-P17；5.1 account-auth 消费预告的进程内 OHS 方法落点）的 auth 侧服务方法：
// reset-password 与 HTTP /auth/user/reset-password 同源（A10 长度校验、bcrypt
// 哈希、成功吊销目标用户全部会话、审计 password_reset security），仅
// source=cli、目标用户按用户名解析。

// ErrUserNotFound CLI reset-password 目标用户不存在（按用户名 COLLATE NOCASE
// 解析失败）。HTTP 侧同场景为 AUTH_012 fields 语义，CLI 契约钉死为执行失败
// （退出码 1，api-panel-core.md 4.1.4），故以哨兵区分于参数校验错误。
var ErrUserNotFound = errors.New("auth: user not found")

// ResetPasswordCLIResult 是 ResetPasswordViaCLI 的结果投影：目标用户当前锁定
// 态如实透出（重置本身不解锁——与 HTTP 同源语义；锁定期满由登录路径惰性
// 解锁 I-08，cmd 层据此输出提示，「按 auth 服务能力如实」）。
type ResetPasswordCLIResult struct {
	Username    string
	Locked      bool
	LockedUntil *time.Time
}

// ResetPasswordViaCLI `dougong reset-password -username <name>`（写）：新密码
// 经环境变量注入（DOUGONG_INIT_PASSWORD，cmd 层校验缺失=PANEL_012 退出码 2，
// 不经 argv）；A10 长度校验失败返回 AUTH_012（cmd 层退出码 2）；目标用户不
// 存在返回 ErrUserNotFound（退出码 1）。单事务：更新密码哈希 → 级联吊销目标
// 用户全部会话（end_reason=cascade）→ 审计 password_reset（security，
// source=cli，actor=终端操作者标签）。
func (s *Service) ResetPasswordViaCLI(ctx context.Context, operator, username, newPassword string) (*ResetPasswordCLIResult, error) {
	now := s.now()
	if fe := validatePassword(newPassword); fe != nil {
		return nil, invalid(*fe)
	}
	if username == "" {
		return nil, invalid(errcode.FieldError{Field: "username", Message: "用户名不能为空"})
	}
	target, err := s.st.Users().GetByUsername(ctx, username)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("auth: cli reset password get user: %w", err)
	}
	passwordHash, err := s.hasher.Hash(newPassword)
	if err != nil {
		return nil, fmt.Errorf("auth: cli reset password hash: %w", err)
	}
	err = s.withTx(ctx, func(tx *storeauth.TxStore) error {
		if err := tx.Users().UpdatePassword(ctx, target.ID, passwordHash, now); err != nil {
			return fmt.Errorf("update password: %w", err)
		}
		if err := tx.Sessions().RevokeByUser(ctx, target.ID, now, storeauth.EndReasonCascade, ""); err != nil {
			return fmt.Errorf("cascade revoke sessions: %w", err)
		}
		entry := audit.Entry{
			ActorName:  operator,
			Category:   audit.CategorySecurity,
			Action:     audit.ActionPasswordReset,
			ObjectType: audit.ObjectTypeUser,
			ObjectID:   strconv.FormatInt(target.ID, 10),
			Detail:     map[string]any{"username": target.Username},
			Result:     audit.ResultSuccess,
			Source:     audit.SourceCLI,
		}
		return tx.Audit().Append(ctx, entry, now)
	})
	if err != nil {
		return nil, err
	}
	return &ResetPasswordCLIResult{
		Username:    target.Username,
		Locked:      target.Status == storeauth.UserStatusLocked,
		LockedUntil: target.LockedUntil,
	}, nil
}

// RenameUserCLIResult 是 RenameUserViaCLI 的结果投影（改名前后用户名）。
type RenameUserCLIResult struct {
	OldUsername string
	NewUsername string
	Changed     bool
}

// RenameUserViaCLI `dougong rename-user -username <old> -new <new>`（写，紧急救援）：
// 终端强制改面板用户名（被锁/遗忘用户名时救回访问）。新用户名 A10 校验失败返回
// AUTH_012（cmd 退出码 2）；原用户不存在返回 ErrUserNotFound（退出码 1）；新名已被
// 占用返回 errcode.ErrUsernameTaken。单事务：更新用户名 → 级联吊销目标用户全部会话
// （end_reason=cascade，改名后旧会话失效）→ 审计 username_changed（security，source=cli）。
// 同名（NOCASE）输入零写入零审计（Changed=false）。
func (s *Service) RenameUserViaCLI(ctx context.Context, operator, oldName, newName string) (*RenameUserCLIResult, error) {
	now := s.now()
	if strings.TrimSpace(oldName) == "" {
		return nil, invalid(errcode.FieldError{Field: "username", Message: "原用户名不能为空"})
	}
	if fe := validateUsername(newName); fe != nil {
		return nil, invalid(*fe)
	}
	target, err := s.st.Users().GetByUsername(ctx, oldName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("auth: cli rename get user: %w", err)
	}
	if strings.EqualFold(target.Username, newName) {
		return &RenameUserCLIResult{OldUsername: target.Username, NewUsername: target.Username, Changed: false}, nil
	}
	if _, err := s.st.Users().GetByUsername(ctx, newName); err == nil {
		return nil, errcode.ErrUsernameTaken
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("auth: cli rename check taken: %w", err)
	}
	updated := target
	updated.Username = newName
	err = s.withTx(ctx, func(tx *storeauth.TxStore) error {
		if err := tx.Users().Update(ctx, updated, now); err != nil {
			return fmt.Errorf("update username: %w", err)
		}
		if err := tx.Sessions().RevokeByUser(ctx, target.ID, now, storeauth.EndReasonCascade, ""); err != nil {
			return fmt.Errorf("cascade revoke sessions: %w", err)
		}
		entry := audit.Entry{
			ActorName:  operator,
			Category:   audit.CategorySecurity,
			Action:     audit.ActionUsernameChanged,
			ObjectType: audit.ObjectTypeUser,
			ObjectID:   strconv.FormatInt(target.ID, 10),
			Detail:     map[string]any{"oldUsername": target.Username, "newUsername": newName},
			Result:     audit.ResultSuccess,
			Source:     audit.SourceCLI,
		}
		return tx.Audit().Append(ctx, entry, now)
	})
	if err != nil {
		return nil, err
	}
	return &RenameUserCLIResult{OldUsername: target.Username, NewUsername: newName, Changed: true}, nil
}

// DisableSecondFactorsCLIResult 是 DisableSecondFactorsViaCLI 的结果投影：
// 各因子清除明细（cmd 层据此输出；audit detail 同口径）。
type DisableSecondFactorsCLIResult struct {
	Username        string
	TotpRemoved     bool
	PasskeysRemoved int
	RecoveryUnused  int
}

// DisableSecondFactorsViaCLI `dougong disable-2fa -username <name> -yes`（写，
// 紧急救援）：终端紧急清除目标用户全部第二登录因子——TOTP 绑定、全部 passkey、
// 全部恢复码（含已用行），登录恢复为仅用户名+密码。两步验证丢失（换机后无法
// 出断言、动态码不可用、恢复码用尽）时的最后通道；语义=全部清除（2026-11-28
// 用户拍板）。单事务：解绑 TOTP → 删除全部 passkey → 删除全部恢复码 → 审计
// second_factor_disabled（security，source=cli，detail 带清除明细）。会话不吊销
// （清因子不改变既有会话有效性）。目标用户不存在返回 ErrUserNotFound；本无任何
// 因子时零写入零审计（result 各项为 0/false）。
func (s *Service) DisableSecondFactorsViaCLI(ctx context.Context, operator, username string) (*DisableSecondFactorsCLIResult, error) {
	now := s.now()
	if strings.TrimSpace(username) == "" {
		return nil, invalid(errcode.FieldError{Field: "username", Message: "用户名不能为空"})
	}
	target, err := s.st.Users().GetByUsername(ctx, username)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("auth: cli disable 2fa get user: %w", err)
	}
	_, totpErr := s.st.Totp().GetByUser(ctx, target.ID)
	hasTotp := totpErr == nil
	if totpErr != nil && !errors.Is(totpErr, sql.ErrNoRows) {
		return nil, fmt.Errorf("auth: cli disable 2fa totp: %w", totpErr)
	}
	passkeys, err := s.st.Webauthn().CountByUser(ctx, target.ID)
	if err != nil {
		return nil, fmt.Errorf("auth: cli disable 2fa passkey count: %w", err)
	}
	recoveryUnused, err := s.st.Recovery().CountUnused(ctx, target.ID)
	if err != nil {
		return nil, fmt.Errorf("auth: cli disable 2fa recovery count: %w", err)
	}
	if !hasTotp && passkeys == 0 {
		// 本无第二因子：零写入零审计，如实返回（恢复码行数与 passkey 一致以 0 记）。
		return &DisableSecondFactorsCLIResult{Username: target.Username}, nil
	}
	err = s.withTx(ctx, func(tx *storeauth.TxStore) error {
		if hasTotp {
			if err := tx.Totp().Delete(ctx, target.ID); err != nil {
				return fmt.Errorf("delete totp: %w", err)
			}
		}
		if passkeys > 0 {
			if _, err := tx.Webauthn().DeleteAllByUser(ctx, target.ID); err != nil {
				return fmt.Errorf("delete passkeys: %w", err)
			}
		}
		if _, err := tx.Recovery().DeleteAllByUser(ctx, target.ID); err != nil {
			return fmt.Errorf("delete recovery codes: %w", err)
		}
		entry := audit.Entry{
			ActorName:  operator,
			Category:   audit.CategorySecurity,
			Action:     audit.ActionSecondFactorDisabled,
			ObjectType: audit.ObjectTypeUser,
			ObjectID:   strconv.FormatInt(target.ID, 10),
			Detail:     map[string]any{"username": target.Username, "totp": hasTotp, "passkeys": passkeys, "recoveryUnused": recoveryUnused},
			Result:     audit.ResultSuccess,
			Source:     audit.SourceCLI,
		}
		return tx.Audit().Append(ctx, entry, now)
	})
	if err != nil {
		return nil, err
	}
	return &DisableSecondFactorsCLIResult{
		Username:        target.Username,
		TotpRemoved:     hasTotp,
		PasskeysRemoved: passkeys,
		RecoveryUnused:  recoveryUnused,
	}, nil
}
