// Package dockermgmt 是容器管理模块 HTTP/WS 传输层（22 HTTP + 2 WS 薄壳：
// 绑定 → service → 包络响应；业务逻辑一律在 service，api-docker-mgmt.md
// §3.1/§4.1.4/§4.2.4/§4.3.4）。
//
// 中间件链（api §3.1）：InitGuard → AuthRequired → CsrfGuard(POST) →
// RoleRequired（读/WS=admin+readonly「全可看」ADR-018；写=admin）→
// DaemonGuard（业务面前置复核 daemon 可用性 D-01，unavailable → DK_002——
// daemon/status 豁免：本接口即探测面；WS 路径的 DK_002 在握手 handler 内
// 以 HTTP 状态+包络拒绝，A-D13）。
package dockermgmt

import (
	"context"
	"errors"

	"github.com/dougong/dougong/internal/adapter/dockerx"
	"github.com/dougong/dougong/internal/api"
	"github.com/dougong/dougong/internal/api/middleware"
	"github.com/dougong/dougong/internal/api/response"
	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	dockersvc "github.com/dougong/dougong/internal/service/dockermgmt"
	"github.com/gin-gonic/gin"
)

// daemonInfoKey 是 DaemonGuard 写入的当次探测结论键（container/list 页头
// daemon 摘要消费——单次探测单请求，A-D14）。
const daemonInfoKey = "dougong.docker.daemon"

// Module 是容器管理路由模块（实现 api.Module，挂载 /docker 前缀）。
type Module struct {
	auth *authsvc.Service // account-auth 中间件契约实现（InitGuard/AuthRequired/VerifyWs 消费）
	svc  *dockersvc.Service
	h    *Handler
}

// NewModule 构造模块。
func NewModule(auth *authsvc.Service, svc *dockersvc.Service) *Module {
	return &Module{auth: auth, svc: svc, h: NewHandler(svc)}
}

// Prefix 挂载前缀 /docker（api §3.1 域段）。
func (m *Module) Prefix() string { return "/docker" }

// Register 注册 24 端点（§3.3 权限矩阵：9 查询+2 WS 两角色同等；13 写
// admin 专属 + CSRF 双重校验 R-02；readonly 越界由 RequireRole 拦截返回
// AUTH_008，接口层不自建权限分支）。
func (m *Module) Register(rg *gin.RouterGroup) {
	rg.Use(middleware.InitGuard(m.auth))

	// 探测面（无 DaemonGuard——本接口即探测结论本身，D-01）。
	probe := rg.Group("", middleware.AuthRequired(m.auth),
		middleware.RequireRole(authsvc.RoleAdmin, authsvc.RoleReadonly))
	probe.GET("/daemon/status", m.h.DaemonStatus)

	// 读组（GET 豁免 CSRF；两角色全可看，ADR-018；DaemonGuard 业务面前置）。
	read := rg.Group("", middleware.AuthRequired(m.auth),
		middleware.RequireRole(authsvc.RoleAdmin, authsvc.RoleReadonly),
		DaemonGuard(m.svc))
	read.GET("/container/list", m.h.ContainerList)
	read.GET("/container/logs", m.h.ContainerLogs)
	read.GET("/container/stats", m.h.ContainerStats)
	read.GET("/image/list", m.h.ImageList)
	read.GET("/volume/list", m.h.VolumeList)
	read.GET("/network/list", m.h.NetworkList)
	read.GET("/compose/list", m.h.ComposeList)
	read.GET("/compose/logs", m.h.ComposeLogs)

	// WS 组（Cookie 会话握手鉴权 admin+readonly + Origin 同源，D-03；
	// daemon 前置与存在性在握手 handler 内以 HTTP 状态拒绝——A-D13）。
	ws := rg.Group("", dockerWsGuard(m.auth))
	ws.GET("/container/logs-follow", m.h.ContainerLogsFollow)
	ws.GET("/compose/logs-follow", m.h.ComposeLogsFollow)

	// 写组（POST 全部 admin 专属 + CSRF 双重校验，R-02；DaemonGuard 前置）。
	write := rg.Group("", middleware.AuthRequired(m.auth),
		middleware.CsrfGuard(), middleware.RequireRole(authsvc.RoleAdmin),
		DaemonGuard(m.svc))
	write.POST("/container/start", m.h.ContainerStart)
	write.POST("/container/stop", m.h.ContainerStop)
	write.POST("/container/restart", m.h.ContainerRestart)
	write.POST("/container/pause", m.h.ContainerPause)
	write.POST("/container/unpause", m.h.ContainerUnpause)
	write.POST("/container/delete", m.h.ContainerDelete)
	write.POST("/image/pull", m.h.ImagePull)
	write.POST("/image/delete", m.h.ImageDelete)
	write.POST("/volume/delete", m.h.VolumeDelete)
	write.POST("/network/delete", m.h.NetworkDelete)
	write.POST("/compose/up", m.h.ComposeUp)
	write.POST("/compose/down", m.h.ComposeDown)
	write.POST("/compose/restart", m.h.ComposeRestart)
	write.POST("/container/create", m.h.ContainerCreate)
	write.POST("/volume/create", m.h.VolumeCreate)
	write.POST("/network/create", m.h.NetworkCreate)
	write.POST("/compose/create", m.h.ComposeCreate)
}

// 编译期断言：Module 实现 api.Module 契约。
var _ api.Module = (*Module)(nil)

// daemonProber 是 daemon 探测契约（DaemonGuard 消费，dockersvc.Service 实现）。
type daemonProber interface {
	ProbeDaemon(ctx context.Context) dockerx.DaemonInfo
}

// DaemonGuard 业务面前置守卫（D-01/I-01：每次请求即时探测，unavailable →
// DK_002 拒绝——不返回空清单，未装≠无容器）；available 时探测结论写入
// context 供 handler 消费（container/list daemon 摘要）。
func DaemonGuard(p daemonProber) gin.HandlerFunc {
	return func(c *gin.Context) {
		info := p.ProbeDaemon(c.Request.Context())
		if info.State != dockerx.StateAvailable {
			response.FailWith(c, errcode.ErrDkDaemonUnavailable)
			c.Abort()
			return
		}
		c.Set(daemonInfoKey, info)
		c.Next()
	}
}

// daemonInfoOf 读取 DaemonGuard 写入的当次探测结论（container/list 消费）。
func daemonInfoOf(c *gin.Context) dockerx.DaemonInfo {
	if v, ok := c.Get(daemonInfoKey); ok {
		if info, ok := v.(dockerx.DaemonInfo); ok {
			return info
		}
	}
	return dockerx.DaemonInfo{State: dockerx.StateAvailable}
}

// dockerWsGuard 日志跟随 WS 握手守卫（D-03/terminal D-09 同判）：① Origin
// 同源（AUTH_003）→ ② Cookie 会话 VerifyWs 非续期复核（AUTH_001/002）→
// ③ 角色 admin+readonly（查看类流，ADR-018；越界 AUTH_008）。拒绝以 HTTP
// 状态+包络上线（升级前仍是 HTTP 语义）。
func dockerWsGuard(tv middleware.WsVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !middleware.SameOrigin(c) {
			response.FailWithStatus(c, response.HTTPStatus(errcode.ErrCSRF), errcode.ErrCSRF)
			c.Abort()
			return
		}
		raw, err := c.Cookie(authsvc.SessionCookieName)
		if err != nil || raw == "" {
			response.FailWithStatus(c, 401, errcode.ErrUnauthenticated)
			c.Abort()
			return
		}
		_, role, err := tv.VerifyWs(c.Request.Context(), raw)
		if err != nil {
			var ec *errcode.Error
			if errors.As(err, &ec) {
				response.FailWithStatus(c, response.HTTPStatus(ec), ec)
			} else {
				response.FailWithStatus(c, 500, errcode.ErrInternal)
			}
			c.Abort()
			return
		}
		if role != authsvc.RoleAdmin && role != authsvc.RoleReadonly {
			response.FailWithStatus(c, 403, errcode.ErrForbidden)
			c.Abort()
			return
		}
		c.Next()
	}
}
