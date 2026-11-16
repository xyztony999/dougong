// Package auth 是账户与安全基座应用服务（api-account-auth.md §4）：
// 初始化双入口、登录（密码+TOTP 两步式）、会话生命周期、RBAC 守卫、
// 用户管理与审计查询编排。不感知 HTTP（DTO 仅带 JSON tag）。
package auth

import (
	"time"

	storeauth "github.com/dougong/dougong/internal/store/auth"
)

// 角色常量（panel_role 固定枚举，透出供 api 层路由/守卫使用）。
const (
	RoleAdmin    = storeauth.RoleAdmin
	RoleReadonly = storeauth.RoleReadonly
)

// 业务常量（编译期固定，非配置键——api-account-auth.md §3.4 配置表）。
const (
	// PasswordMinLength 密码最小长度（A10 从宽固定 8）。
	PasswordMinLength = 8
	// PasswordMaxLength bcrypt 输入上限 72 字节。
	PasswordMaxLength = 72
	// UsernameMaxLength 用户名最大长度（I-10）。
	UsernameMaxLength = 32
	// MaxFailAttempts 连续失败锁定阈值（R-01：5 次）。
	MaxFailAttempts = 5
	// LockoutDuration 锁定时长（R-01：15 分钟）。
	LockoutDuration = 15 * time.Minute
	// LoginTicketTTL 两步式 login_ticket 有效期（D-02：5 分钟一次性）。
	LoginTicketTTL = 5 * time.Minute
	// SessionCookieName 会话 Cookie 名（§3.1：HttpOnly dg_session）。
	SessionCookieName = "dg_session"
	// CSRFHeader CSRF 请求头（§3.1）。
	CSRFHeader = "X-CSRF-Token"
)

// SessionPolicy 会话有效期策略（D-10：载体=面板配置文件，重启生效 MVP）。
type SessionPolicy struct {
	// AbsoluteTimeout 绝对有效期（默认 12h，自签发起算）。
	AbsoluteTimeout time.Duration
	// IdleTimeout 空闲有效期（默认 2h，活跃滑动顺延）。
	IdleTimeout time.Duration
}

// DefaultSessionPolicy 默认会话策略（§3.4 配置表默认值）。
func DefaultSessionPolicy() SessionPolicy {
	return SessionPolicy{AbsoluteTimeout: 12 * time.Hour, IdleTimeout: 2 * time.Hour}
}
