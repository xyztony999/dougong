// handler.go 是 22 个 HTTP 接口的薄壳（绑定 → service → 包络响应；值域
// 校验唯一权威在 service 层——R-14 单一校验通道，handler 不做 required 绑定）。
package dockermgmt

import (
	"net/http"

	"github.com/dougong/dougong/internal/api/middleware"
	"github.com/dougong/dougong/internal/api/response"
	"github.com/dougong/dougong/internal/errcode"
	dockersvc "github.com/dougong/dougong/internal/service/dockermgmt"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// Handler 聚合容器管理接口的 service 依赖。
type Handler struct {
	svc      *dockersvc.Service
	upgrader websocket.Upgrader
}

// NewHandler 构造 handler。upgrader.CheckOrigin 恒真——Origin 同源校验在
// dockerWsGuard（D-03），此处不重复判定（terminal 同判）。
func NewHandler(svc *dockersvc.Service) *Handler {
	return &Handler{
		svc: svc,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(*http.Request) bool { return true },
		},
	}
}

// 请求体形状（§4.1.4/§4.2.4/§4.3.4；delete 均无 force 字段，I-04/I-05）。
type (
	containerRequest struct {
		ContainerID string `json:"containerId"`
	}
	imagePullRequest struct {
		Ref string `json:"ref"`
	}
	imageDeleteRequest struct {
		ImageID string `json:"imageId"`
	}
	volumeDeleteRequest struct {
		VolumeName string `json:"volumeName"`
	}
	networkDeleteRequest struct {
		NetworkID string `json:"networkId"`
	}
	composeRequest struct {
		Project string `json:"project"`
	}
)

// DaemonStatus GET /docker/daemon/status（admin+readonly）：即时探测结论
// （D-01；无 DaemonGuard——本接口即探测面）。
func (h *Handler) DaemonStatus(c *gin.Context) {
	data, err := h.svc.DaemonStatus(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ContainerList GET /docker/container/list（admin+readonly）：query
// {state?, composeProject?}——筛选/排序/动作面均在 service（D-02）；daemon
// 摘要取 DaemonGuard 当次探测结论（A-D14）。
func (h *Handler) ContainerList(c *gin.Context) {
	data, err := h.svc.ContainerList(c.Request.Context(), daemonInfoOf(c),
		c.Query("state"), c.Query("composeProject"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ContainerLogs GET /docker/container/logs（admin+readonly）：query
// {containerId, tail?}（D-03 尾部通道）。
func (h *Handler) ContainerLogs(c *gin.Context) {
	data, err := h.svc.ContainerLogs(c.Request.Context(),
		c.Query("containerId"), c.Query("tail"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ContainerStats GET /docker/container/stats（admin+readonly）：query
// {containerId}（D-07 单次快照）。
func (h *Handler) ContainerStats(c *gin.Context) {
	data, err := h.svc.ContainerStats(c.Request.Context(), c.Query("containerId"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ContainerStart/Stop/Restart/Pause/Unpause POST 生命周期动作（admin+CSRF；
// 矩阵守卫在 service，D-04）。
func (h *Handler) ContainerStart(c *gin.Context)   { h.containerAction(c, dockersvc.ActionStart) }
func (h *Handler) ContainerStop(c *gin.Context)    { h.containerAction(c, dockersvc.ActionStop) }
func (h *Handler) ContainerRestart(c *gin.Context) { h.containerAction(c, dockersvc.ActionRestart) }
func (h *Handler) ContainerPause(c *gin.Context)   { h.containerAction(c, dockersvc.ActionPause) }
func (h *Handler) ContainerUnpause(c *gin.Context) {
	h.containerAction(c, dockersvc.ActionUnpause)
}

// containerAction 生命周期动作公共薄壳：绑定 {containerId} → 身份 → service。
func (h *Handler) containerAction(c *gin.Context, action string) {
	var req containerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	actor, ok := actorOf(c)
	if !ok {
		return // 已响应 AUTH_001（防御双保险：链路下不可达）
	}
	data, err := h.svc.ContainerAction(c.Request.Context(), actor, action, req.ContainerID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ContainerDelete POST /docker/container/delete（admin+CSRF；无 force 字段）。
func (h *Handler) ContainerDelete(c *gin.Context) {
	var req containerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	actor, ok := actorOf(c)
	if !ok {
		return
	}
	data, err := h.svc.ContainerDelete(c.Request.Context(), actor, req.ContainerID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ImageList GET /docker/image/list（admin+readonly）。
func (h *Handler) ImageList(c *gin.Context) {
	data, err := h.svc.ImageList(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ImagePull POST /docker/image/pull（admin+CSRF；同步终态 D-05）。
func (h *Handler) ImagePull(c *gin.Context) {
	var req imagePullRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	actor, ok := actorOf(c)
	if !ok {
		return
	}
	data, err := h.svc.ImagePull(c.Request.Context(), actor, req.Ref)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ImageDelete POST /docker/image/delete（admin+CSRF）。
func (h *Handler) ImageDelete(c *gin.Context) {
	var req imageDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	actor, ok := actorOf(c)
	if !ok {
		return
	}
	data, err := h.svc.ImageDelete(c.Request.Context(), actor, req.ImageID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// VolumeList GET /docker/volume/list（admin+readonly）。
func (h *Handler) VolumeList(c *gin.Context) {
	data, err := h.svc.VolumeList(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// VolumeDelete POST /docker/volume/delete（admin+CSRF）。
func (h *Handler) VolumeDelete(c *gin.Context) {
	var req volumeDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	actor, ok := actorOf(c)
	if !ok {
		return
	}
	data, err := h.svc.VolumeDelete(c.Request.Context(), actor, req.VolumeName)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// NetworkList GET /docker/network/list（admin+readonly）。
func (h *Handler) NetworkList(c *gin.Context) {
	data, err := h.svc.NetworkList(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// NetworkDelete POST /docker/network/delete（admin+CSRF）。
func (h *Handler) NetworkDelete(c *gin.Context) {
	var req networkDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	actor, ok := actorOf(c)
	if !ok {
		return
	}
	data, err := h.svc.NetworkDelete(c.Request.Context(), actor, req.NetworkID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ComposeList GET /docker/compose/list（admin+readonly；label 聚合+locatable）。
func (h *Handler) ComposeList(c *gin.Context) {
	data, err := h.svc.ComposeList(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ComposeUp/Down/Restart POST 动词端点（admin+CSRF；D-06）。
func (h *Handler) ComposeUp(c *gin.Context)      { h.composeAction(c, dockersvc.VerbUp) }
func (h *Handler) ComposeDown(c *gin.Context)    { h.composeAction(c, dockersvc.VerbDown) }
func (h *Handler) ComposeRestart(c *gin.Context) { h.composeAction(c, dockersvc.VerbRestart) }

// composeAction Compose 动词公共薄壳：绑定 {project} → 身份 → service。
func (h *Handler) composeAction(c *gin.Context, verb string) {
	var req composeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	actor, ok := actorOf(c)
	if !ok {
		return
	}
	data, err := h.svc.ComposeAction(c.Request.Context(), actor, req.Project, verb)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ComposeLogs GET /docker/compose/logs（admin+readonly）：query {project, tail?}。
func (h *Handler) ComposeLogs(c *gin.Context) {
	data, err := h.svc.ComposeLogs(c.Request.Context(),
		c.Query("project"), c.Query("tail"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// actorOf 读取当前身份并返回审计操作者用户名（防御双保险：AuthRequired 链路
// 下不可达；firewall/servicemgmt 同法）。
func actorOf(c *gin.Context) (string, bool) {
	id, ok := middleware.Identity(c)
	if !ok {
		response.FailWith(c, errcode.ErrUnauthenticated)
		return "", false
	}
	return id.Username, true
}

// bindError 将 gin 绑定错误转 AUTH_012（body 字段级提示；A-S9 先例）。
func bindError(err error) *errcode.Error {
	return errcode.ErrValidation.WithFields(errcode.FieldError{
		Field: "body", Message: err.Error(),
	})
}
