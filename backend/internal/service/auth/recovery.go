package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	storeauth "github.com/dougong/dougong/internal/store/auth"
)

// recoveryCodeCount 每次生成的恢复码数量。
const recoveryCodeCount = 10

// GenerateRecoveryCodes POST /auth/recovery-codes/generate（会话）：作废旧未用码，
// 生成 10 个一次性码，明文仅此一次返回，落库 sha256 哈希。
func (s *Service) GenerateRecoveryCodes(ctx context.Context, id *Identity) (*RecoveryCodesResponse, error) {
	now := s.now()
	codes := make([]string, 0, recoveryCodeCount)
	hashes := make([]string, 0, recoveryCodeCount)
	for i := 0; i < recoveryCodeCount; i++ {
		b := make([]byte, 8)
		if _, err := s.rand.Read(b); err != nil {
			return nil, fmt.Errorf("auth: recovery rand: %w", err)
		}
		raw := strings.ToUpper(hex.EncodeToString(b))
		code := raw[0:4] + "-" + raw[4:8] + "-" + raw[8:12] + "-" + raw[12:16]
		codes = append(codes, code)
		hashes = append(hashes, recoveryHash(code))
	}
	err := s.withTx(ctx, func(tx *storeauth.TxStore) error {
		if err := tx.Recovery().ReplaceUnused(ctx, id.UserID, hashes, formatTimeNow(now)); err != nil {
			return err
		}
		return tx.Audit().Append(ctx, actorEntry(id, audit.CategorySecurity, "recovery_codes_generated", audit.ObjectTypeUser, fmt.Sprintf("%d", id.UserID)), now)
	})
	if err != nil {
		return nil, err
	}
	return &RecoveryCodesResponse{Codes: codes}, nil
}

// RecoveryCount GET /auth/recovery-codes（会话）。
func (s *Service) RecoveryCount(ctx context.Context, id *Identity) (*RecoveryCountResponse, error) {
	n, err := s.st.Recovery().CountUnused(ctx, id.UserID)
	if err != nil {
		return nil, fmt.Errorf("auth: recovery count: %w", err)
	}
	return &RecoveryCountResponse{Remaining: n}, nil
}

// RecoveryLogin POST /auth/session/recovery-login（公开）：消耗一码签发会话。
func (s *Service) RecoveryLogin(ctx context.Context, req RecoveryLoginRequest, meta RequestMeta) (*SessionIssued, error) {
	now := s.now()
	u, err := s.st.Users().GetByUsername(ctx, req.Username)
	if errors.Is(err, sql.ErrNoRows) {
		s.auditLoginFailedStandalone(ctx, req.Username, nil, meta, "recovery")
		return nil, errcode.ErrBadCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("auth: recovery get user: %w", err)
	}
	u, locked, err := s.resolveLockState(ctx, u, now)
	if err != nil {
		return nil, err
	}
	if locked != nil {
		return nil, locked
	}
	code, err := s.st.Recovery().GetUnusedByHash(ctx, u.ID, recoveryHash(req.Code))
	if errors.Is(err, sql.ErrNoRows) {
		if rerr := s.recordLoginFailure(ctx, u, now, meta, "recovery"); rerr != nil {
			return nil, rerr
		}
		return nil, errcode.ErrRecoveryInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("auth: recovery get: %w", err)
	}
	var issued *SessionIssued
	err = s.withTx(ctx, func(tx *storeauth.TxStore) error {
		if err := tx.Recovery().MarkUsed(ctx, code.ID, formatTimeNow(now)); err != nil {
			return err
		}
		if u.FailCount > 0 {
			if err := tx.Users().UpdateLockState(ctx, u.ID, 0, nil, storeauth.UserStatusActive, now); err != nil {
				return err
			}
		}
		var err error
		issued, err = s.createSession(ctx, tx, u, meta, now)
		if err != nil {
			return err
		}
		return tx.Audit().Append(ctx, loginAuditEntry(u, audit.ActionLoginSuccess, audit.ResultSuccess, map[string]any{"sourceIp": meta.SourceIP, "recovery": true}), now)
	})
	if err != nil {
		return nil, err
	}
	return issued, nil
}

// recoveryHash 规范化（去分隔符/空白、大写）后 sha256。
func recoveryHash(code string) string {
	norm := strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(code), "-", ""), " ", ""))
	sum := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(sum[:])
}

// deviceHash 设备指纹=sha256(UA|设备cookie)。
func deviceHash(userAgent, deviceCookie string) string {
	sum := sha256.Sum256([]byte(userAgent + "|" + deviceCookie))
	return hex.EncodeToString(sum[:])
}
