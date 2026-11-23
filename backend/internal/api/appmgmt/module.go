// Package appmgmt 是应用管理（app-mgmt）模块 HTTP 传输层（16 接口薄壳：
// 绑定 → service → 包络；业务逻辑一律在 service，api-app-mgmt.md §3.1/§4.3）。
package appmgmt

import (
	"strings"

	"github.com/dougong/dougong/internal/api"
	"github.com/dougong/dougong/internal/api/middleware"
	"github.com/dougong/dougong/internal/api/response"
	"github.com/dougong/dougong/internal/errcode"
	appsvc "github.com/dougong/dougong/internal/service/appmgmt"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	"github.com/gin-gonic/gin"
)

// Module 是应用管理路由模块（挂载 /app 前缀）。
type Module struct {
	auth *authsvc.Service
	svc  *appsvc.Service
}

// NewModule 构造模块。
func NewModule(auth *authsvc.Service, svc *appsvc.Service) *Module {
	return &Module{auth: auth, svc: svc}
}

// Prefix 挂载前缀 /app。
func (m *Module) Prefix() string { return "/app" }

// Register 注册 16 接口（§3.3 权限矩阵：app/list·task/list·task/detail 两角色
// （ADR-018 全可看）；app/env（参数面）与 app/log（日志 ⚠️A8）admin 专属读取；
// 全部写接口 admin+CSRF）。
func (m *Module) Register(rg *gin.RouterGroup) {
	rg.Use(middleware.InitGuard(m.auth))

	readBoth := rg.Group("", middleware.AuthRequired(m.auth),
		middleware.RequireRole(authsvc.RoleAdmin, authsvc.RoleReadonly))
	readBoth.GET("/app/list", m.list)
	readBoth.GET("/task/list", m.taskList)
	readBoth.GET("/task/detail", m.taskDetail)

	readAdmin := rg.Group("", middleware.AuthRequired(m.auth),
		middleware.RequireRole(authsvc.RoleAdmin))
	readAdmin.GET("/app/env", m.envGet)
	readAdmin.GET("/log", m.logTail)

	write := rg.Group("", middleware.AuthRequired(m.auth),
		middleware.CsrfGuard(), middleware.RequireRole(authsvc.RoleAdmin))
	write.POST("/app/create", m.create)
	write.POST("/app/delete", m.delete)
	write.POST("/app/params", m.paramsSave)
	write.POST("/app/deps", m.deps)
	write.POST("/task/retry", m.taskRetry)
	write.POST("/service/start", m.serviceOp("start"))
	write.POST("/service/stop", m.serviceOp("stop"))
	write.POST("/service/restart", m.serviceOp("restart"))
	write.POST("/service/enable", m.serviceOp("enable"))
	write.POST("/service/disable", m.serviceOp("disable"))
	write.POST("/site/link", m.siteLink)
}

func (m *Module) list(c *gin.Context) {
	// runtime 过滤（ADR-033：网站管理页 Python / Node 分栏各取其一；缺省返回全部）。
	runtime := strings.TrimSpace(c.Query("runtime"))
	if runtime != "" && runtime != "python" && runtime != "node" {
		response.Fail(c, errcode.ErrAppParamsInvalid.WithData(map[string]any{
			"reason": "runtime 取值须为 python 或 node"}))
		return
	}
	data, err := m.svc.AppList(c.Request.Context(), runtime)
	respond(c, data, err)
}

func (m *Module) create(c *gin.Context) {
	var req appsvc.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := m.svc.Create(c.Request.Context(), identity(c), req)
	respond(c, data, err)
}

func (m *Module) delete(c *gin.Context) {
	var req appsvc.DeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := m.svc.Delete(c.Request.Context(), identity(c), req)
	respond(c, data, err)
}

func (m *Module) envGet(c *gin.Context) {
	appID := queryInt(c, "appId", 0)
	if appID <= 0 {
		response.Fail(c, errcode.ErrValidation.WithFields(errcode.FieldError{
			Field: "appId", Message: "appId 必填"}))
		return
	}
	data, err := m.svc.EnvGet(c.Request.Context(), int64(appID))
	respond(c, data, err)
}

func (m *Module) paramsSave(c *gin.Context) {
	var req appsvc.ParamsSaveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := m.svc.ParamsSave(c.Request.Context(), identity(c), req)
	respond(c, data, err)
}

func (m *Module) deps(c *gin.Context) {
	var req appsvc.DeleteRequest // 仅取 appId 字段（同形）
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := m.svc.Deps(c.Request.Context(), identity(c), req.AppID)
	respond(c, data, err)
}

func (m *Module) serviceOp(op string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			AppID int64 `json:"appId"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Fail(c, bindError(err))
			return
		}
		data, err := m.svc.ServiceOp(c.Request.Context(), identity(c), req.AppID, op)
		respond(c, data, err)
	}
}

func (m *Module) taskList(c *gin.Context) {
	appID := int64(queryInt(c, "appId", 0))
	action := c.Query("action")
	status := c.Query("status")
	page := queryInt(c, "page", 1)
	pageSize := queryInt(c, "pageSize", appsvc.TaskListPageSize)
	data, err := m.svc.TaskList(c.Request.Context(), appID, action, status, page, pageSize)
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

func (m *Module) taskRetry(c *gin.Context) {
	var req struct {
		TaskID int64 `json:"taskId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := m.svc.TaskRetry(c.Request.Context(), identity(c), req.TaskID)
	respond(c, data, err)
}

func (m *Module) logTail(c *gin.Context) {
	appID := queryInt(c, "appId", 0)
	if appID <= 0 {
		response.Fail(c, errcode.ErrValidation.WithFields(errcode.FieldError{
			Field: "appId", Message: "appId 必填"}))
		return
	}
	lines := queryInt(c, "lines", 200)
	data, err := m.svc.LogTail(c.Request.Context(), int64(appID), lines)
	respond(c, data, err)
}

func (m *Module) siteLink(c *gin.Context) {
	var req appsvc.SiteLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := m.svc.SiteLink(c.Request.Context(), identity(c), req)
	respond(c, data, err)
}

// 编译期断言：Module 实现 api.Module 契约。
var _ api.Module = (*Module)(nil)
