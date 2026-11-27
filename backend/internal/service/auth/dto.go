package auth

import (
	"encoding/json"
	"time"
)

// DTO（camelCase JSON；时间 RFC3339 UTC 由 time.Time 默认编码满足）。

// UserBrief 登录成功/初始化响应中的用户摘要。
type UserBrief struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

// SessionIssued 签发会话结果：RawToken 仅用于写 Cookie（D-01），不进响应体。
type SessionIssued struct {
	RawToken  string    `json:"-"`
	CsrfToken string    `json:"csrfToken"`
	MaxAgeSec int       `json:"-"`
	ExpiresAt time.Time `json:"-"`
	User      UserBrief `json:"user"`
	NewDevice bool      `json:"newDevice"`
}

// LoginRequest POST /auth/session/login 入参（code 字段已废弃，D-02 两步式）。
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// TOTPVerifyRequest POST /auth/session/totp-verify 入参。
type TOTPVerifyRequest struct {
	LoginTicket string `json:"loginTicket" binding:"required"`
	Code        string `json:"code" binding:"required"`
}

// LoginOutcome 登录结果：RequiresTOTP=true 时前端凭 LoginTicket 走 TOTP 二阶段；
// RequiresPasskey=true 时凭 LoginTicket 走 passkey 二阶段（无 TOTP 但有 passkey）。
type LoginOutcome struct {
	RequiresTOTP    bool
	RequiresPasskey bool
	LoginTicket     string
	Session         *SessionIssued
}

// SessionItem 会话清单行（脱敏：不含 token 哈希与 csrf_token）。
type SessionItem struct {
	ID           string    `json:"id"` // token_hash 前 8 位摘要
	Username     string    `json:"username"`
	CreatedAt    time.Time `json:"createdAt"`
	LastActiveAt time.Time `json:"lastActiveAt"`
	SourceIP     string    `json:"sourceIp"`
	Current      bool      `json:"current"`
}

// UserInfo 用户清单行（脱敏：不含 password_hash/TOTP secret）。
type UserInfo struct {
	ID          int64      `json:"id"`
	Username    string     `json:"username"`
	Role        string     `json:"role"`
	Status      string     `json:"status"`
	TotpEnabled bool       `json:"totpEnabled"`
	LockedUntil *time.Time `json:"lockedUntil"`
	CreatedAt   time.Time  `json:"createdAt"`
}

// CreateUserRequest POST /auth/user/create 入参。
type CreateUserRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Role     string `json:"role" binding:"required"`
}

// CreatedUser 创建用户响应。
type CreatedUser struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

// InitWizardRequest POST /auth/init/wizard 入参（TOTP 可选一步完成）。
type InitWizardRequest struct {
	Username   string `json:"username" binding:"required"`
	Password   string `json:"password" binding:"required"`
	TotpSecret string `json:"totpSecret"`
	TotpCode   string `json:"totpCode"`
}

// InitStatus GET /auth/init/check 响应。
type InitStatus struct {
	Initialized bool `json:"initialized"`
	AllowWizard bool `json:"allowWizard"`
}

// TotpSetupResponse totp-setup 响应（secret 仅此一次下发，I-09）。
type TotpSetupResponse struct {
	OtpauthURI string `json:"otpauthUri"`
	Secret     string `json:"secret"`
}

// ---- 登录安全增量（passkey/恢复码，v0.7.0）----

// WebAuthnBeginResponse register-begin/login-begin 响应：options 为协议 JSON
// （publicKey），token 为一次性会话凭证（finish 携带）。
type WebAuthnBeginResponse struct {
	Options json.RawMessage `json:"options"`
	Token   string          `json:"token"`
}

// WebAuthnRegisterFinishRequest register-finish 入参。
type WebAuthnRegisterFinishRequest struct {
	Token      string          `json:"token" binding:"required"`
	Credential json.RawMessage `json:"credential" binding:"required"`
	Name       string          `json:"name"`
}

// WebAuthnLoginBeginRequest login-begin 入参（用户名定位凭据）。
type WebAuthnLoginBeginRequest struct {
	Username string `json:"username" binding:"required"`
}

// WebAuthnLoginFinishRequest login-finish / passkey-verify 入参。
// 无密码流用 Username+Token；二因素流用 LoginTicket+Token。
type WebAuthnLoginFinishRequest struct {
	Username    string          `json:"username"`
	Token       string          `json:"token"`
	LoginTicket string          `json:"loginTicket"`
	Credential  json.RawMessage `json:"credential" binding:"required"`
}

// WebAuthnCredentialItem 凭据清单行（脱敏：不含公钥）。
type WebAuthnCredentialItem struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
}

// WebAuthnDeleteRequest delete 入参。
type WebAuthnDeleteRequest struct {
	ID int64 `json:"id" binding:"required"`
}

// RecoveryCodesResponse generate 响应（明文仅此一次下发）。
type RecoveryCodesResponse struct {
	Codes []string `json:"codes"`
}

// RecoveryCountResponse 剩余未用码数。
type RecoveryCountResponse struct {
	Remaining int `json:"remaining"`
}

// RecoveryLoginRequest recovery-login 入参。
type RecoveryLoginRequest struct {
	Username string `json:"username" binding:"required"`
	Code     string `json:"code" binding:"required"`
}
