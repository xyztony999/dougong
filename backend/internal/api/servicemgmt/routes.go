// Package servicemgmt 是 systemd 服务管理模块 HTTP 传输层（7 接口薄壳：
// 绑定 → service → 包络响应；业务逻辑一律在 service，api-service-mgmt.md
// §3.1/§4.1.4）。
package servicemgmt

import (
	"github.com/dougong/dougong/internal/api"
	"github.com/dougong/dougong/internal/api/middleware"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	svcsvc "github.com/dougong/dougong/internal/service/servicemgmt"
	"github.com/gin-gonic/gin"
)

// Module 是服务管理路由模块（实现 api.Module，挂载 /services 前缀）。
// 中间件链（api §3.1）：InitGuard → AuthRequired → CsrfGuard(POST) →
// RoleRequired（读=admin+readonly「全可看」ADR-018；写=admin）。
type Module struct {
	auth *authsvc.Service // account-auth 中间件契约实现（InitGuard/AuthRequired 消费）
	svc  *svcsvc.Service
	h    *Handler
}

// NewModule 构造模块。
func NewModule(auth *authsvc.Service, svc *svcsvc.Service) *Module {
	return &Module{auth: auth, svc: svc, h: NewHandler(svc)}
}

// Prefix 挂载前缀 /services（api §3.1 域段）。
func (m *Module) Prefix() string { return "/services" }

// Register 注册 7 接口（§3.3 权限矩阵：读两角色同等；写 admin 专属 + CSRF；
// readonly 越界由 RoleRequired 拦截返回 AUTH_008，接口层不自建权限分支）。
func (m *Module) Register(rg *gin.RouterGroup) {
	rg.Use(middleware.InitGuard(m.auth))

	// 读组（GET 豁免 CSRF；两角色全可看，ADR-018）。
	read := rg.Group("", middleware.AuthRequired(m.auth),
		middleware.RequireRole(authsvc.RoleAdmin, authsvc.RoleReadonly))
	read.GET("/unit/list", m.h.List)
	read.GET("/unit/detail", m.h.Detail)

	// 写组（POST 全部 admin 专属 + CSRF 双重校验，R-02）。
	write := rg.Group("", middleware.AuthRequired(m.auth),
		middleware.CsrfGuard(), middleware.RequireRole(authsvc.RoleAdmin))
	write.POST("/unit/start", m.h.Start)
	write.POST("/unit/stop", m.h.Stop)
	write.POST("/unit/restart", m.h.Restart)
	write.POST("/unit/enable", m.h.Enable)
	write.POST("/unit/disable", m.h.Disable)
}

// 编译期断言：Module 实现 api.Module 契约。
var _ api.Module = (*Module)(nil)
