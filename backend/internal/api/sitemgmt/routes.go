// Package sitemgmt 是网站管理（site-mgmt，核心卖点）模块 HTTP 传输层
// （18 接口薄壳：绑定 → service → 包络响应；业务逻辑一律在 service，
// api-site-mgmt.md §3.1/§4.2.4/§4.1.4）。
package sitemgmt

import (
	"github.com/dougong/dougong/internal/api"
	"github.com/dougong/dougong/internal/api/middleware"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	sitesvc "github.com/dougong/dougong/internal/service/sitemgmt"
	"github.com/gin-gonic/gin"
)

// Module 是网站管理路由模块（实现 api.Module，挂载 /sites 前缀）。
// 中间件链（api §3.1）：InitGuard → AuthRequired → [GET 两角色 / GET admin
// 专属] / [POST → CsrfGuard → RoleRequired(admin)]；WritePipeline 与 nginx
// 可用性预检为服务层编排（§3.1 明文「非中间件」）。
type Module struct {
	auth *authsvc.Service
	svc  *sitesvc.Service
	h    *Handler
}

// NewModule 构造模块。
func NewModule(auth *authsvc.Service, svc *sitesvc.Service) *Module {
	return &Module{auth: auth, svc: svc, h: NewHandler(svc)}
}

// Prefix 挂载前缀 /sites（api §3.1 域段）。
func (m *Module) Prefix() string { return "/sites" }

// Register 注册 18 接口（§3.3 权限矩阵：site/list·site/detail·cert/list·
// log/tail 两角色（D0 口径+ADR-018 全可看）；编辑前置读取 site/source 与
// 建站候选 php-pools/list 随写接口 admin；全部写 admin+CSRF）。
func (m *Module) Register(rg *gin.RouterGroup) {
	rg.Use(middleware.InitGuard(m.auth))

	// 读组 A（GET；两角色同等——查看类，D0 口径 + ADR-018）。
	readBoth := rg.Group("", middleware.AuthRequired(m.auth),
		middleware.RequireRole(authsvc.RoleAdmin, authsvc.RoleReadonly))
	readBoth.GET("/site/list", m.h.List)
	readBoth.GET("/site/detail", m.h.Detail)
	readBoth.GET("/log/tail", m.h.TailLog)
	readBoth.GET("/cert/list", m.h.ListCerts)

	// 读组 B（GET；admin 专属——编辑前置读取与建站候选，§3.3 矩阵）。
	readAdmin := rg.Group("", middleware.AuthRequired(m.auth),
		middleware.RequireRole(authsvc.RoleAdmin))
	readAdmin.GET("/site/source", m.h.ReadSource)
	readAdmin.GET("/php-pools/list", m.h.ListPhpPools)
	readAdmin.GET("/dns-credential/list", m.h.DnsCredentialList) // v2.0 D-20（敏感配置最小暴露）

	// 写组（POST 全部 admin 专属 + CSRF，R-02）。
	write := rg.Group("", middleware.AuthRequired(m.auth),
		middleware.CsrfGuard(), middleware.RequireRole(authsvc.RoleAdmin))
	write.POST("/site/create", m.h.Create)
	write.POST("/site/update", m.h.Update)
	write.POST("/proxy/update", m.h.UpdateProxy)
	write.POST("/site/source-save", m.h.SaveSource)
	write.POST("/site/enable", m.h.Enable)
	write.POST("/site/disable", m.h.Disable)
	write.POST("/nginx/reload", m.h.Reload)
	write.POST("/cert/request", m.h.CertRequest)
	write.POST("/cert/deploy", m.h.CertDeploy)
	write.POST("/cert/undeploy", m.h.CertUndeploy)
	write.POST("/cert/retry-renewal", m.h.CertRetryRenewal)
	write.POST("/cert/renew", m.h.CertRenew)                      // v1.2 D-18
	write.POST("/cert/replace", m.h.CertReplace)                  // v1.2 D-18
	write.POST("/dns-credential/create", m.h.DnsCredentialCreate) // v2.0 D-20
	write.POST("/dns-credential/delete", m.h.DnsCredentialDelete) // v2.0 D-20
	write.POST("/cert/delete", m.h.CertDelete)
}

// 编译期断言：Module 实现 api.Module 契约。
var _ api.Module = (*Module)(nil)
