package auth

import "time"

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

// LoginOutcome 登录结果：RequiresTOTP=true 时前端凭 LoginTicket 走二阶段。
type LoginOutcome struct {
	RequiresTOTP bool
	LoginTicket  string
	Session      *SessionIssued
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
