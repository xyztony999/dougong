// app.go —— docker-mgmt v1.2 的 HTTP 传输层（ADR-033：面板应用
// /docker/app/{list,create,service,logs,delete,site} 与 Compose 项目删除
// /docker/compose/delete）。薄壳：绑定 → 身份 → service → 包络；业务逻辑一律
// 在 service。
package dockermgmt

import (
	"github.com/dougong/dougong/internal/api/middleware"
	"github.com/dougong/dougong/internal/api/response"
	"github.com/dougong/dougong/internal/errcode"
	dockersvc "github.com/dougong/dougong/internal/service/dockermgmt"
	"github.com/gin-gonic/gin"
)

// AppList GET /docker/app/list（admin+readonly）。
func (h *Handler) AppList(c *gin.Context) {
	data, err := h.svc.AppList(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// AppLogs GET /docker/app/logs（admin+readonly）：query name、tail?。
func (h *Handler) AppLogs(c *gin.Context) {
	data, err := h.svc.AppLogs(c.Request.Context(), c.Query("name"), c.Query("tail"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// AppCreate POST /docker/app/create（admin）。
func (h *Handler) AppCreate(c *gin.Context) {
	var req dockersvc.AppCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	actor, ok := actorOf(c)
	if !ok {
		return
	}
	data, err := h.svc.AppCreate(c.Request.Context(), actor, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// AppService POST /docker/app/service（admin）：{name, op}。
func (h *Handler) AppService(c *gin.Context) {
	var req dockersvc.AppServiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	actor, ok := actorOf(c)
	if !ok {
		return
	}
	data, err := h.svc.AppService(c.Request.Context(), actor, req.Name, req.Op)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// AppDelete POST /docker/app/delete（admin）：{name}。
func (h *Handler) AppDelete(c *gin.Context) {
	var req dockersvc.AppDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	actor, ok := actorOf(c)
	if !ok {
		return
	}
	data, err := h.svc.AppDelete(c.Request.Context(), actor, req.Name)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// AppSite POST /docker/app/site（admin）：{name, domains}。建站需要完整调用者
// 身份（site-mgmt 侧审计），故此处取 Identity 而非仅用户名。
func (h *Handler) AppSite(c *gin.Context) {
	var req dockersvc.AppSiteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	id, ok := middleware.Identity(c)
	if !ok {
		response.FailWith(c, errcode.ErrUnauthenticated)
		return
	}
	data, err := h.svc.AppSite(c.Request.Context(), id, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ComposeDelete POST /docker/compose/delete（admin）：{project}。
func (h *Handler) ComposeDelete(c *gin.Context) {
	var req composeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	actor, ok := actorOf(c)
	if !ok {
		return
	}
	data, err := h.svc.ComposeDelete(c.Request.Context(), actor, req.Project)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}
