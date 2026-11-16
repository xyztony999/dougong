package panel

import (
	"github.com/dougong/dougong/internal/api"
	"github.com/dougong/dougong/internal/api/middleware"
	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	panelsvc "github.com/dougong/dougong/internal/service/panel"
	"github.com/gin-gonic/gin"
)

// Module 是 panel-core 路由模块（实现 api.Module，挂载 /panel 前缀）。
// 中间件链（api §3.1 顺序固定）：InitGuard → AuthRequired → CsrfGuard(POST)
// → RoleRequired；全局第一环 EntryPathGuard 由 main 经 Server.UseGlobal
// 挂载（D-P03，本模块 Register 之前的整链前置）。
type Module struct {
	auth *authsvc.Service // account-auth 中间件契约实现（InitGuard/AuthRequired 消费）
	svc  *panelsvc.Service
	h    *Handler
}

// NewModule 构造模块。
func NewModule(auth *authsvc.Service, svc *panelsvc.Service) *Module {
	return &Module{auth: auth, svc: svc, h: NewHandler(svc)}
}

// Prefix 挂载前缀 /panel。
func (m *Module) Prefix() string { return "/panel" }

// Register 注册 7 接口（api §3.3 权限矩阵：GET 3 项两角色，POST 4 项
// admin+CSRF；D-P11 全部要求会话、无公开端点）。
func (m *Module) Register(rg *gin.RouterGroup) {
	rg.Use(middleware.InitGuard(m.auth))

	// 会话组（D-P11：未登录一律 AUTH_001）。
	sess := rg.Group("", middleware.AuthRequired(m.auth))
	sess.GET("/config/detail", m.h.ConfigDetail)
	sess.GET("/menu/list", m.h.MenuList)
	sess.GET("/system/version", m.h.Version)

	// 变更组（CsrfGuard → RoleRequired(admin)，§3.1 链序）。
	write := sess.Group("", middleware.CsrfGuard(), middleware.RequireRole(authsvc.RoleAdmin))
	write.POST("/config/update", m.h.ConfigUpdate)
	write.POST("/config/regenerate-entry-path", m.h.RegenerateEntryPath)
	write.POST("/config/upload-certificate", m.h.UploadCertificate)
	write.POST("/system/update-check", m.h.UpdateCheck)
}

// 编译期断言：Module 实现 api.Module 契约。
var _ api.Module = (*Module)(nil)

// bindError 将 gin 绑定错误转 AUTH_012（body 字段级提示）。
func bindError(err error) *errcode.Error {
	return errcode.ErrValidation.WithFields(errcode.FieldError{
		Field: "body", Message: err.Error(),
	})
}

// errcodeUnauthenticated 返回 AUTH_001（身份缺失兜底；正常链路下
// AuthRequired 已拦截，防御性双保险）。
func errcodeUnauthenticated() *errcode.Error { return errcode.ErrUnauthenticated }
