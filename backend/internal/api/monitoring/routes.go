package monitoring

import (
	"github.com/dougong/dougong/internal/api"
	"github.com/dougong/dougong/internal/api/middleware"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	monsvc "github.com/dougong/dougong/internal/service/monitoring"
	"github.com/gin-gonic/gin"
)

// Module 是系统监控路由模块（实现 api.Module，挂载 /monitoring 前缀）。
// 中间件链（api-monitoring.md §3.1）：InitGuard → AuthRequired → RoleRequired
// （全部接口 admin+readonly，无差异化角色分支；全 GET 只读无 CSRF——§3.1）。
type Module struct {
	auth *authsvc.Service // account-auth 中间件契约实现（InitGuard/AuthRequired 消费）
	svc  *monsvc.Service
	h    *Handler
}

// NewModule 构造模块。
func NewModule(auth *authsvc.Service, svc *monsvc.Service) *Module {
	return &Module{auth: auth, svc: svc, h: NewHandler(svc)}
}

// Prefix 挂载前缀 /monitoring。
func (m *Module) Prefix() string { return "/monitoring" }

// Register 注册 4 接口（§3.3 权限矩阵：全部两角色同等可用，ADR-018 全可看）。
func (m *Module) Register(rg *gin.RouterGroup) {
	rg.Use(middleware.InitGuard(m.auth))

	// 会话组（未登录一律 AUTH_001；两角色同权，无访客可达面）。
	sess := rg.Group("", middleware.AuthRequired(m.auth),
		middleware.RequireRole(authsvc.RoleAdmin, authsvc.RoleReadonly))
	sess.GET("/metrics/snapshot", m.h.Snapshot)
	sess.GET("/metrics/history", m.h.History)
	sess.GET("/processes/top", m.h.TopProcesses)
	sess.GET("/system/info", m.h.SystemInfo)
}

// 编译期断言：Module 实现 api.Module 契约。
var _ api.Module = (*Module)(nil)
