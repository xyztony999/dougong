// Package firewall 是防火墙模块 HTTP 传输层（5 接口薄壳：绑定 → service →
// 包络响应；业务逻辑一律在 service，api-firewall.md §3.1/§4.1.4）。
package firewall

import (
	"github.com/dougong/dougong/internal/api"
	"github.com/dougong/dougong/internal/api/middleware"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	fwsvc "github.com/dougong/dougong/internal/service/firewall"
	"github.com/gin-gonic/gin"
)

// Module 是防火墙路由模块（实现 api.Module，挂载 /firewall 前缀）。
// 中间件链（api §3.1）：InitGuard → AuthRequired → CsrfGuard(POST) →
// RoleRequired（读=admin+readonly「全可看」ADR-018；写=admin）。
type Module struct {
	auth *authsvc.Service // account-auth 中间件契约实现（InitGuard/AuthRequired 消费）
	svc  *fwsvc.Service
	h    *Handler
}

// NewModule 构造模块。
func NewModule(auth *authsvc.Service, svc *fwsvc.Service) *Module {
	return &Module{auth: auth, svc: svc, h: NewHandler(svc)}
}

// Prefix 挂载前缀 /firewall（api §3.1 域段）。
func (m *Module) Prefix() string { return "/firewall" }

// Register 注册 5 接口（§3.3 权限矩阵：读三接口两角色同等；写两接口
// admin 专属 + CSRF 双重校验 R-02；readonly 越界由 RoleRequired 拦截返回
// AUTH_008，接口层不自建权限分支）。
func (m *Module) Register(rg *gin.RouterGroup) {
	rg.Use(middleware.InitGuard(m.auth))

	// 读组（GET 豁免 CSRF；两角色全可看，ADR-018）。
	read := rg.Group("", middleware.AuthRequired(m.auth),
		middleware.RequireRole(authsvc.RoleAdmin, authsvc.RoleReadonly))
	read.GET("/backend/status", m.h.BackendStatus)
	read.GET("/rule/list", m.h.RuleList)
	read.GET("/port/list", m.h.PortList)

	// 写组（POST 全部 admin 专属 + CSRF 双重校验，R-02）。
	write := rg.Group("", middleware.AuthRequired(m.auth),
		middleware.CsrfGuard(), middleware.RequireRole(authsvc.RoleAdmin))
	write.POST("/rule/create", m.h.CreateRule)
	write.POST("/rule/delete", m.h.DeleteRule)
}

// 编译期断言：Module 实现 api.Module 契约。
var _ api.Module = (*Module)(nil)
