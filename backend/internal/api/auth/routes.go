package auth

import (
	"github.com/dougong/dougong/internal/api"
	"github.com/dougong/dougong/internal/api/middleware"
	auditsvc "github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	"github.com/gin-gonic/gin"
)

// Module 是 account-auth 路由模块（实现 api.Module，挂载 /auth 前缀）。
// 中间件链（§3.1）：InitGuard → AuthRequired → CsrfGuard(POST) → RoleRequired。
type Module struct {
	svc    *authsvc.Service
	audits *auditsvc.Service
	h      *Handler
}

// NewModule 构造模块（cookieSecure：HTTPS 部署置 true）。
func NewModule(svc *authsvc.Service, audits *auditsvc.Service, cookieSecure bool) *Module {
	return &Module{svc: svc, audits: audits, h: NewHandler(svc, audits, cookieSecure)}
}

// Prefix 挂载前缀 /auth。
func (m *Module) Prefix() string { return "/auth" }

// Register 注册 18 接口（§3.3 权限矩阵）：
// 公开 4（init/check、init/wizard、session/login、session/totp-verify）；
// 会话 5 GET（current、session/list、user/list、audit-log/list、audit-log/export[admin]）；
// 会话+CSRF 9 POST（logout、revoke[admin]、password-change、totp-setup/enable/disable、
// user/create|delete|reset-password[admin]）。
func (m *Module) Register(rg *gin.RouterGroup) {
	rg.Use(middleware.InitGuard(m.svc))

	// 公开组（无会话；login/totp-verify 仅已初始化可达——InitGuard 保证 I-01）。
	pub := rg.Group("")
	pub.GET("/init/check", m.h.InitCheck)
	pub.POST("/init/wizard", m.h.InitWizard)
	pub.POST("/session/login", m.h.Login)
	pub.POST("/session/totp-verify", m.h.TOTPVerify)

	// 会话组（AuthRequired：I-11 校验 + 滑动续期）。
	sess := rg.Group("", middleware.AuthRequired(m.svc))
	sess.GET("/session/current", m.h.CurrentSession)
	sess.GET("/session/list", m.h.ListSessions)
	sess.GET("/user/list", m.h.ListUsers)
	sess.GET("/audit-log/list", m.h.AuditList)
	// admin 专属 GET：审计导出（解释性决策，ADR-018）。
	sess.GET("/audit-log/export", middleware.RequireRole(authsvc.RoleAdmin), m.h.AuditExport)

	// 变更组（CsrfGuard：POST 一律 CSRF 校验，R-02）。
	write := sess.Group("", middleware.CsrfGuard())
	write.POST("/session/logout", m.h.Logout)
	write.POST("/session/revoke", middleware.RequireRole(authsvc.RoleAdmin), m.h.RevokeSession)
	write.POST("/user/password-change", m.h.ChangePassword)
	write.POST("/user/totp-setup", m.h.TOTPSetup)
	write.POST("/user/totp-enable", m.h.TOTPEnable)
	write.POST("/user/totp-disable", m.h.TOTPDisable)
	write.POST("/user/create", middleware.RequireRole(authsvc.RoleAdmin), m.h.CreateUser)
	write.POST("/user/delete", middleware.RequireRole(authsvc.RoleAdmin), m.h.DeleteUser)
	write.POST("/user/reset-password", middleware.RequireRole(authsvc.RoleAdmin), m.h.ResetPassword)
}

// 编译期断言：Module 实现 api.Module 契约（后续模块同构挂载）。
var _ api.Module = (*Module)(nil)
