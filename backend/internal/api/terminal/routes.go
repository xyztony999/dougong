package terminal

import (
	"github.com/dougong/dougong/internal/api"
	"github.com/dougong/dougong/internal/api/middleware"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	terminalsvc "github.com/dougong/dougong/internal/service/terminal"
	"github.com/gin-gonic/gin"
)

// Module 是终端路由模块（实现 api.Module，挂载 /terminal 前缀）。
// 中间件链（api §3.1）：
//   - HTTP：InitGuard → AuthRequired → RoleRequired(admin)（GET 豁免 CSRF）；
//   - WS：TerminalHandshakeGuard（①Origin 同源→②VerifyWs→③admin，
//     D-09/I-01；帧层无 CSRF 面）。
//
// 本模块全端点 admin 专属——readonly 整域禁用（ADR-018，全项目唯一
// readonly 完全无功能模块）。
type Module struct {
	auth *authsvc.Service
	svc  *terminalsvc.Service
	h    *Handler
}

// NewModule 构造模块。
func NewModule(auth *authsvc.Service, svc *terminalsvc.Service) *Module {
	return &Module{auth: auth, svc: svc, h: NewHandler(svc)}
}

// Prefix 挂载前缀 /terminal（api §3.1 域段）。
func (m *Module) Prefix() string { return "/terminal" }

// Register 注册 2 端点（§3.3 权限矩阵：两列 admin 专属，CSRF 列—/豁免）。
func (m *Module) Register(rg *gin.RouterGroup) {
	rg.Use(middleware.InitGuard(m.auth))

	// HTTP 读组：GET /user/options（候选系统用户清单，D-02）。
	read := rg.Group("", middleware.AuthRequired(m.auth),
		middleware.RequireRole(authsvc.RoleAdmin))
	read.GET("/user/options", m.h.Options)

	// WS 组：GET /session/connect（升级端点；握手准入守卫 D-09）。
	ws := rg.Group("", middleware.TerminalHandshakeGuard(m.auth))
	ws.GET("/session/connect", m.h.Connect)
}

// 编译期断言：Module 实现 api.Module 契约。
var _ api.Module = (*Module)(nil)
