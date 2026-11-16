package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	storeauth "github.com/dougong/dougong/internal/store/auth"
)

// IsInitialized 报告系统是否已完成初始化（InitGuard I-01 消费）。
func (s *Service) IsInitialized(ctx context.Context) (bool, error) {
	st, err := s.st.Init().Get(ctx)
	if err != nil {
		return false, fmt.Errorf("auth: read init state: %w", err)
	}
	return st.Initialized, nil
}

// InitCheck GET /auth/init/check：前端路由判断依据（§4.4.4）。
func (s *Service) InitCheck(ctx context.Context) (InitStatus, error) {
	st, err := s.st.Init().Get(ctx)
	if err != nil {
		return InitStatus{}, fmt.Errorf("auth: init check: %w", err)
	}
	return InitStatus{Initialized: st.Initialized, AllowWizard: !st.Initialized}, nil
}

// InitWizard POST /auth/init/wizard：Web 向导初始化（创建首个 admin 并直接登录）。
// 单事务：init_state 仲裁抢占（I-02）→ 创建 admin →（可选 TOTP 激活）→
// init_completed 审计 → 签发会话（§2.3 初始化编排）。已初始化返回 AUTH_009。
func (s *Service) InitWizard(ctx context.Context, req InitWizardRequest, meta RequestMeta) (*SessionIssued, error) {
	now := s.now()
	secret, err := s.prepareInit(req, now)
	if err != nil {
		return nil, err
	}
	passwordHash, err := s.hasher.Hash(req.Password)
	if err != nil {
		return nil, fmt.Errorf("auth: wizard hash password: %w", err)
	}

	var issued *SessionIssued
	err = s.withTx(ctx, func(tx *storeauth.TxStore) error {
		ok, err := tx.Init().TryClaim(ctx, "web", now)
		if err != nil {
			return fmt.Errorf("init claim: %w", err)
		}
		if !ok {
			return errcode.ErrAlreadyInitialized // I-02 后到者拒绝
		}
		uid, err := tx.Users().Create(ctx, storeauth.User{
			Username: req.Username, PasswordHash: passwordHash, Role: storeauth.RoleAdmin,
		}, now)
		if err != nil {
			return fmt.Errorf("init create admin: %w", err)
		}
		if secret != "" {
			if err := tx.Totp().Upsert(ctx, storeauth.TotpCredential{
				UserID: uid, Secret: secret, Enabled: true, ConfirmedAt: &now,
			}); err != nil {
				return fmt.Errorf("init upsert totp: %w", err)
			}
		}
		actorID := uid
		if err := tx.Audit().Append(ctx, audit.Entry{
			ActorID:    &actorID,
			ActorName:  req.Username,
			Category:   audit.CategorySecurity,
			Action:     audit.ActionInitCompleted,
			ObjectType: audit.ObjectTypeInit,
			ObjectID:   "1",
			Detail:     map[string]any{"method": "web", "username": req.Username},
			Result:     audit.ResultSuccess,
			Source:     audit.SourceWeb,
		}, now); err != nil {
			return fmt.Errorf("init audit: %w", err)
		}
		issued, err = s.createSession(ctx, tx, storeauth.User{
			ID: uid, Username: req.Username, Role: storeauth.RoleAdmin,
		}, meta, now)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return issued, nil
}

// InitViaCLI CLI 子命令 `dougong init`（§4.4.4 CLI 入口）：与向导共用同一
// init_state 事务仲裁；审计 source=cli；已初始化返回 AUTH_009（cmd 层转
// 人类可读输出与非 0 退出码）。
func (s *Service) InitViaCLI(ctx context.Context, username, password, totpSecret, totpCode string) error {
	now := s.now()
	secret, err := s.prepareInit(InitWizardRequest{
		Username: username, Password: password, TotpSecret: totpSecret, TotpCode: totpCode,
	}, now)
	if err != nil {
		return err
	}
	passwordHash, err := s.hasher.Hash(password)
	if err != nil {
		return fmt.Errorf("auth: cli hash password: %w", err)
	}
	return s.withTx(ctx, func(tx *storeauth.TxStore) error {
		ok, err := tx.Init().TryClaim(ctx, "cli", now)
		if err != nil {
			return fmt.Errorf("init claim: %w", err)
		}
		if !ok {
			return errcode.ErrAlreadyInitialized
		}
		uid, err := tx.Users().Create(ctx, storeauth.User{
			Username: username, PasswordHash: passwordHash, Role: storeauth.RoleAdmin,
		}, now)
		if err != nil {
			return fmt.Errorf("init create admin: %w", err)
		}
		if secret != "" {
			if err := tx.Totp().Upsert(ctx, storeauth.TotpCredential{
				UserID: uid, Secret: secret, Enabled: true, ConfirmedAt: &now,
			}); err != nil {
				return fmt.Errorf("init upsert totp: %w", err)
			}
		}
		return tx.Audit().Append(ctx, audit.Entry{
			ActorID:    &uid,
			ActorName:  username,
			Category:   audit.CategorySecurity,
			Action:     audit.ActionInitCompleted,
			ObjectType: audit.ObjectTypeInit,
			ObjectID:   "1",
			Detail:     map[string]any{"method": "cli", "username": username},
			Result:     audit.ResultSuccess,
			Source:     audit.SourceCLI,
		}, now)
	})
}

// prepareInit 向导/CLI 共入参校验：I-10/A10 + TOTP 可选对（secret+code 须成对，
// 提供时先验证动态码——AUTH_006）。返回待落库 secret（空=未启用）。
func (s *Service) prepareInit(req InitWizardRequest, now time.Time) (string, error) {
	var fields []errcode.FieldError
	if fe := validateUsername(req.Username); fe != nil {
		fields = append(fields, *fe)
	}
	if fe := validatePassword(req.Password); fe != nil {
		fields = append(fields, *fe)
	}
	if (req.TotpSecret == "") != (req.TotpCode == "") {
		fields = append(fields, errcode.FieldError{Field: "totpSecret", Message: "totpSecret 与 totpCode 必须成对提供"})
	}
	if len(fields) > 0 {
		return "", invalid(fields...)
	}
	if req.TotpSecret == "" {
		return "", nil
	}
	if !s.totp.Verify(req.TotpSecret, req.TotpCode, now) {
		return "", errcode.ErrTotpInvalid
	}
	return req.TotpSecret, nil
}
