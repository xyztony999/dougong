// Package fileman 是文件管理器模块 HTTP 传输层（22 接口薄壳：绑定 → service
// → 包络响应；业务逻辑一律在 service，api-file-manager.md §3.1/§3.3/§4）。
package fileman

import (
	"github.com/dougong/dougong/internal/api"
	"github.com/dougong/dougong/internal/api/middleware"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	filemansvc "github.com/dougong/dougong/internal/service/fileman"
	"github.com/gin-gonic/gin"
)

// Module 是文件管理器路由模块（实现 api.Module，挂载 /files 前缀）。
// 中间件链（api §3.1）：InitGuard → AuthRequired → CsrfGuard(POST) →
// RoleRequired（entry/list 与 trash/list 两角色「全可看」ADR-018；白名单
// 配置/全部写/下载/编辑读取 admin 专属）。
type Module struct {
	auth *authsvc.Service // account-auth 中间件契约实现（InitGuard/AuthRequired 消费）
	svc  *filemansvc.Service
	h    *Handler
}

// NewModule 构造模块。
func NewModule(auth *authsvc.Service, svc *filemansvc.Service) *Module {
	return &Module{auth: auth, svc: svc, h: NewHandler(svc)}
}

// Prefix 挂载前缀 /files（api §3.1 域段）。
func (m *Module) Prefix() string { return "/files" }

// Register 注册 22 接口（§3.3 权限矩阵：读组两角色 2 + admin 只读 4；
// 写组 admin 专属 + CSRF 双重校验 16；readonly 越界由 RoleRequired 拦截
// 返回 AUTH_008，接口层不自建权限分支）。
func (m *Module) Register(rg *gin.RouterGroup) {
	rg.Use(middleware.InitGuard(m.auth))

	// 读组·两角色（GET 豁免 CSRF；ADR-018 全可看）。
	both := rg.Group("", middleware.AuthRequired(m.auth),
		middleware.RequireRole(authsvc.RoleAdmin, authsvc.RoleReadonly))
	both.GET("/entry/list", m.h.List)
	both.GET("/trash/list", m.h.TrashList)

	// 读组·admin 专属（安全配置/数据取出/编辑前置/chown 候选）。
	adminRead := rg.Group("", middleware.AuthRequired(m.auth),
		middleware.RequireRole(authsvc.RoleAdmin))
	adminRead.GET("/path-scope/get", m.h.ScopeGet)
	adminRead.GET("/entry/download", m.h.Download)
	adminRead.GET("/entry/content", m.h.Content)
	adminRead.GET("/system-accounts/list", m.h.SystemAccounts)

	// 写组·admin 专属 + CSRF（R-02）。
	write := rg.Group("", middleware.AuthRequired(m.auth),
		middleware.CsrfGuard(), middleware.RequireRole(authsvc.RoleAdmin))
	write.POST("/path-scope/update", m.h.ScopeUpdate)
	write.POST("/entry/upload", m.h.Upload)
	write.POST("/upload/chunk-init", m.h.ChunkInit)
	write.POST("/upload/chunk", m.h.ChunkUpload)
	write.POST("/upload/chunk-complete", m.h.ChunkComplete)
	write.POST("/entry/content-save", m.h.ContentSave)
	write.POST("/entry/create", m.h.Create)
	write.POST("/entry/rename", m.h.Rename)
	write.POST("/entry/move", m.h.Move)
	write.POST("/entry/delete", m.h.Delete)
	write.POST("/entry/chmod", m.h.Chmod)
	write.POST("/entry/chown", m.h.Chown)
	write.POST("/entry/archive", m.h.Archive)
	write.POST("/entry/unarchive", m.h.Unarchive)
	write.POST("/trash/restore", m.h.TrashRestore)
	write.POST("/trash/purge", m.h.TrashPurge)
}

// 编译期断言：Module 实现 api.Module 契约。
var _ api.Module = (*Module)(nil)
