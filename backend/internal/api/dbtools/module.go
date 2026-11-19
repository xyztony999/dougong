// Package dbtools 是数据库工具集成（db-tools）模块 HTTP 传输层（api-db-tools
// v1.2：面板面 9 接口薄壳 + 内部 2 端点防护链；业务逻辑一律在 service）。
// 内部端点（gate/verify·signon/verify）不挂会话/CSRF/InitGuard 中间件——
// loopback 源校验 + 内部秘密头双保险，服务层自行校验（D-11）。
package dbtools

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/dougong/dougong/internal/api"
	"github.com/dougong/dougong/internal/api/middleware"
	"github.com/dougong/dougong/internal/api/response"
	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	dbtoolssvc "github.com/dougong/dougong/internal/service/dbtools"
	"github.com/gin-gonic/gin"
)

// Module 是 db-tools 路由模块（挂载 /dbtool 前缀）。
type Module struct {
	auth         *authsvc.Service
	svc          *dbtoolssvc.Service
	cookieSecure bool
}

// NewModule 构造模块（cookieSecure 供 dg_tool_instance cookie Secure 位）。
func NewModule(auth *authsvc.Service, svc *dbtoolssvc.Service, cookieSecure bool) *Module {
	return &Module{auth: auth, svc: svc, cookieSecure: cookieSecure}
}

// Prefix 挂载前缀 /dbtool。
func (m *Module) Prefix() string { return "/dbtool" }

// Register 注册 11 端点（§3.3 权限矩阵：tool/list·task/list·task/detail
// 两角色；install/upgrade/uninstall/open/rotate admin+CSRF；reopen GET
// admin 无 CSRF（302 重定向载体，handler 内会话 gating）；内部 2 端点
// loopback+秘密头防护链）。
func (m *Module) Register(rg *gin.RouterGroup) {
	readBoth := rg.Group("", middleware.InitGuard(m.auth),
		middleware.AuthRequired(m.auth),
		middleware.RequireRole(authsvc.RoleAdmin, authsvc.RoleReadonly))
	readBoth.GET("/tool/list", m.toolList)
	readBoth.GET("/task/list", m.taskList)
	readBoth.GET("/task/detail", m.taskDetail)

	write := rg.Group("", middleware.InitGuard(m.auth),
		middleware.AuthRequired(m.auth), middleware.CsrfGuard(),
		middleware.RequireRole(authsvc.RoleAdmin))
	write.POST("/tool/install", m.install)
	write.POST("/tool/upgrade", m.upgrade)
	write.POST("/tool/uninstall", m.uninstall)
	write.POST("/access/open", m.open)
	write.POST("/access/rotate", m.rotate)

	// reopen：GET 重定向落地载体——handler 内会话 gating（失败 302 回面板
	// 页面而非 JSON 包络，浏览器顶层导航形态，4.1.4）；admin 专属 ⚠️A10。
	rg.GET("/access/reopen", middleware.InitGuard(m.auth), m.reopen)

	// 内部端点：非用户面（nginx/PHP-FPM 服务端调用），不挂会话/CSRF/InitGuard。
	internal := rg.Group("", m.loopbackGuard(), m.internalSecretGuard())
	internal.GET("/gate/verify", m.gateVerify)
	internal.POST("/signon/verify", m.signonVerify)
}

// ---- 面板面 handler（薄壳：绑定 → service → 包络）----

func (m *Module) toolList(c *gin.Context) {
	data, err := m.svc.ToolList(c.Request.Context())
	respond(c, data, err)
}

type toolRequest struct {
	ToolType string `json:"toolType"`
}

type installRequest struct {
	ToolType   string `json:"toolType"`
	InstanceID *int64 `json:"instanceId"`
}

type openRequest struct {
	ToolType   string `json:"toolType"`
	InstanceID *int64 `json:"instanceId"`
}

func (m *Module) install(c *gin.Context) {
	var req installRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := m.svc.Install(c.Request.Context(), identity(c), req.ToolType, req.InstanceID)
	respond(c, data, err)
}

func (m *Module) upgrade(c *gin.Context) {
	var req toolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := m.svc.Upgrade(c.Request.Context(), identity(c), req.ToolType)
	respond(c, data, err)
}

func (m *Module) uninstall(c *gin.Context) {
	var req toolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := m.svc.Uninstall(c.Request.Context(), identity(c), req.ToolType)
	respond(c, data, err)
}

func (m *Module) open(c *gin.Context) {
	var req openRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := m.svc.Open(c.Request.Context(), identity(c), req.ToolType, req.InstanceID, c.Request.Host)
	if err != nil {
		respond(c, nil, err)
		return
	}
	// phpmyadmin：下发非敏感实例标识 cookie（HttpOnly，Path=工具路径，
	// SameSite=Lax——SignonURL 经 PHP 动态拼接 instanceId 承载重认证，D-06）。
	if req.ToolType == "phpmyadmin" && req.InstanceID != nil {
		c.SetSameSite(http.SameSiteLaxMode)
		c.SetCookie(dbtoolssvc.ToolInstanceCookie, strconv.FormatInt(*req.InstanceID, 10),
			0, dbtoolssvc.ToolPathPrefixPHPMyAdmin, "", m.cookieSecure, true)
	}
	response.OK(c, data)
}

func (m *Module) rotate(c *gin.Context) {
	var req toolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := m.svc.Rotate(c.Request.Context(), identity(c), req.ToolType)
	respond(c, data, err)
}

// reopen GET /dbtool/access/reopen（v1.2 W3）：admin 会话 gating + 重签令牌
// 302 回工具 URL。失败呈现（浏览器导航形态，不返回 JSON）：无会话/过期/
// 吊销→302 回面板登录入口；readonly 越界/未装/实例未就绪/运行态未就绪→
// 302 回面板工具管理页。entryPath 由请求路径反推（浏览器经安全入口到达）。
func (m *Module) reopen(c *gin.Context) {
	prefix := strings.TrimSuffix(c.Request.URL.Path, "/dbtool/access/reopen")
	loginURL := prefix + "/login"
	toolsURL := prefix + "/db-tools"
	raw, err := c.Cookie(authsvc.SessionCookieName)
	if err != nil || raw == "" {
		c.Redirect(http.StatusFound, loginURL)
		return
	}
	id, err := m.auth.VerifyToken(c.Request.Context(), raw)
	if err != nil {
		c.Redirect(http.StatusFound, loginURL)
		return
	}
	if id.Role != authsvc.RoleAdmin {
		c.Redirect(http.StatusFound, toolsURL) // ⚠️A10 工具访问 admin 专属
		return
	}
	instanceID, perr := strconv.ParseInt(c.Query("instanceId"), 10, 64)
	if perr != nil || instanceID <= 0 {
		c.Redirect(http.StatusFound, toolsURL)
		return
	}
	url, serr := m.svc.Reopen(c.Request.Context(), id, instanceID, c.Request.Host)
	if serr != nil {
		c.Redirect(http.StatusFound, toolsURL)
		return
	}
	c.Redirect(http.StatusFound, url)
}

func (m *Module) taskList(c *gin.Context) {
	toolType := c.Query("toolType")
	status := c.Query("status")
	page := queryInt(c, "page", 1)
	pageSize := queryInt(c, "pageSize", dbtoolssvc.TaskListPageSize)
	data, err := m.svc.TaskList(c.Request.Context(), toolType, status, page, pageSize)
	respond(c, data, err)
}

func (m *Module) taskDetail(c *gin.Context) {
	taskID := queryInt(c, "taskId", 0)
	if taskID <= 0 {
		response.Fail(c, errcode.ErrValidation.WithFields(errcode.FieldError{
			Field: "taskId", Message: "taskId 必填"}))
		return
	}
	data, err := m.svc.TaskDetail(c.Request.Context(), int64(taskID))
	respond(c, data, err)
}

// ---- 内部端点防护链（D-11 三重防护①②；③服务层自行校验）----

// loopbackGuard loopback 源校验：RemoteAddr 必须 127.0.0.1/::1，否则裸
// HTTP 404 无包络（防探测，沿 HostGuard 标准 404 语义）。
func (m *Module) loopbackGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
		if err != nil {
			host = c.Request.RemoteAddr
		}
		if host != "127.0.0.1" && host != "::1" {
			c.Status(http.StatusNotFound)
			c.Abort()
			return
		}
		c.Next()
	}
}

// internalSecretGuard 内部秘密头校验：X-Dbtool-Internal 与 {dataDir}/tools/
// internal-secret 常量时间比对；缺失/不符/秘密不可用→裸 404（防探测）。
func (m *Module) internalSecretGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		expected, err := m.svc.InternalSecret()
		got := c.GetHeader(dbtoolssvc.InternalSecretHeader)
		if err != nil || got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
			c.Status(http.StatusNotFound)
			c.Abort()
			return
		}
		c.Next()
	}
}

// gateVerify GET /dbtool/gate/verify（nginx auth_request 子请求专用）：
// 裸 HTTP 状态——会话有效+admin→200 放行；无会话/过期/吊销/readonly→401
// 拒绝（auth_request 语义，无包络）。
func (m *Module) gateVerify(c *gin.Context) {
	raw, _ := c.Cookie(authsvc.SessionCookieName)
	if m.svc.GateVerify(c.Request.Context(), raw) {
		c.Status(http.StatusOK)
		return
	}
	c.Status(http.StatusUnauthorized)
}

// signonVerifyReq signon/verify 请求（W4：session=SignonScript 转发的
// dg_session cookie 供会话绑定校验；instanceCookie=dg_tool_instance 辅助
// 实例绑定校验）。
type signonVerifyReq struct {
	Token          string `json:"token"`
	Session        string `json:"session"`
	InstanceCookie string `json:"instanceCookie"`
}

// signonVerify POST /dbtool/signon/verify（PHP-FPM SignonScript 回调专用）：
// 令牌四重校验；成功包络 data={user,password,host,port}（仅 loopback 服务端
// 通道，凭据永不进浏览器）；失败包络 DBT_007/008/010（HTTP 200）。
func (m *Module) signonVerify(c *gin.Context) {
	var req signonVerifyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := m.svc.SignonVerify(c.Request.Context(), req.Token, req.Session, req.InstanceCookie)
	if err != nil {
		response.Fail(c, err) // 失败 data=null（D-11：DBT_007/008/010 包络）
		return
	}
	response.OK(c, data)
}

// 编译期断言：Module 实现 api.Module 契约。
var _ api.Module = (*Module)(nil)
