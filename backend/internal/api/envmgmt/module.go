// Package envmgmt 是环境管理（env-mgmt）模块 HTTP 传输层（10 接口薄壳：
// 绑定 → service → 包络；业务逻辑一律在 service，api-env-mgmt.md §3.1/§4.1.4）。
package envmgmt

import (
	"github.com/dougong/dougong/internal/api"
	"github.com/dougong/dougong/internal/api/middleware"
	"github.com/dougong/dougong/internal/api/response"
	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	envsvc "github.com/dougong/dougong/internal/service/envmgmt"
	"github.com/gin-gonic/gin"
)

// Module 是环境管理路由模块（挂载 /env 前缀）。
type Module struct {
	auth *authsvc.Service
	svc  *envsvc.Service
}

// NewModule 构造模块。
func NewModule(auth *authsvc.Service, svc *envsvc.Service) *Module {
	return &Module{auth: auth, svc: svc}
}

// Prefix 挂载前缀 /env。
func (m *Module) Prefix() string { return "/env" }

// Register 注册 10 接口（§3.3 权限矩阵：env/list·task/list·task/detail 两角色
// （ADR-018 全可看）；install/uninstall/service 全家 admin+CSRF）。
func (m *Module) Register(rg *gin.RouterGroup) {
	rg.Use(middleware.InitGuard(m.auth))

	readBoth := rg.Group("", middleware.AuthRequired(m.auth),
		middleware.RequireRole(authsvc.RoleAdmin, authsvc.RoleReadonly))
	readBoth.GET("/env/list", m.list)
	readBoth.GET("/env/profile", m.profile)
	readBoth.GET("/env/config-file", m.configFileGet)
	readBoth.GET("/env/versions", m.versions)

	write := rg.Group("", middleware.AuthRequired(m.auth),
		middleware.CsrfGuard(), middleware.RequireRole(authsvc.RoleAdmin))
	write.POST("/env/install", m.install)
	write.POST("/env/uninstall", m.uninstall)
	write.POST("/env/save-config-file", m.configFileSave)
	write.POST("/env/save-settings", m.settingsSave)
	write.POST("/service/start", m.serviceOp("start"))
	write.POST("/service/stop", m.serviceOp("stop"))
	write.POST("/service/restart", m.serviceOp("restart"))
	write.POST("/service/enable", m.serviceOp("enable"))
	write.POST("/service/disable", m.serviceOp("disable"))

	readBoth.GET("/task/list", m.taskList)
	readBoth.GET("/task/detail", m.taskDetail)
}

type envRequest struct {
	EnvType string `json:"envType"`
	Version string `json:"version"`
}

func (m *Module) list(c *gin.Context) {
	data, err := m.svc.EnvList(c.Request.Context())
	respond(c, data, err)
}

func (m *Module) install(c *gin.Context) {
	var req envRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := m.svc.Install(c.Request.Context(), identity(c), req.EnvType, req.Version)
	respond(c, data, err)
}

func (m *Module) uninstall(c *gin.Context) {
	var req envRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := m.svc.Uninstall(c.Request.Context(), identity(c), req.EnvType)
	respond(c, data, err)
}

func (m *Module) serviceOp(op string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req envRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Fail(c, bindError(err))
			return
		}
		data, err := m.svc.ServiceOp(c.Request.Context(), identity(c), req.EnvType, op)
		respond(c, data, err)
	}
}

func (m *Module) taskList(c *gin.Context) {
	envType := c.Query("envType")
	status := c.Query("status")
	page := queryInt(c, "page", 1)
	pageSize := queryInt(c, "pageSize", envsvc.TaskListPageSize)
	data, err := m.svc.TaskList(c.Request.Context(), envType, status, page, pageSize)
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

func (m *Module) profile(c *gin.Context) {
	data, err := m.svc.EnvProfile(c.Request.Context(), c.Query("envType"))
	respond(c, data, err)
}

func (m *Module) configFileGet(c *gin.Context) {
	data, err := m.svc.ConfigFileGet(c.Request.Context(), c.Query("envType"), c.Query("fileId"))
	respond(c, data, err)
}

func (m *Module) versions(c *gin.Context) {
	data, err := m.svc.ListVersions(c.Request.Context(), c.Query("envType"))
	respond(c, data, err)
}

type configFileSaveRequest struct {
	EnvType string `json:"envType"`
	FileID  string `json:"fileId"`
	Content string `json:"content"`
}

func (m *Module) configFileSave(c *gin.Context) {
	var req configFileSaveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := m.svc.ConfigFileSave(c.Request.Context(), identity(c), req.EnvType, req.FileID, req.Content)
	respond(c, data, err)
}

type settingsSaveRequest struct {
	EnvType string            `json:"envType"`
	Values  map[string]string `json:"values"`
}

func (m *Module) settingsSave(c *gin.Context) {
	var req settingsSaveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := m.svc.SettingsSave(c.Request.Context(), identity(c), req.EnvType, req.Values)
	respond(c, data, err)
}

// 编译期断言：Module 实现 api.Module 契约。
var _ api.Module = (*Module)(nil)
