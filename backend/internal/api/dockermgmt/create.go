// create.go —— docker-mgmt v1.1 创建类 HTTP 传输层（api-docker-mgmt.md v1.1
// §4.3 FR-19~22 四端点薄壳：绑定 → service → 包络；actor 由 actorOf 取认证身份）。
package dockermgmt

import (
	"github.com/dougong/dougong/internal/api/response"
	"github.com/dougong/dougong/internal/service/dockermgmt"
	"github.com/gin-gonic/gin"
)

// containerCreateRequest 容器创建请求体（api §4.3 FR-19 详情逐字段）。
type containerCreateRequest struct {
	Name          string                      `json:"name"`
	Image         string                      `json:"image"`
	PortMappings  []dockermgmt.PortMappingDTO `json:"ports,omitempty"`
	Env           []dockermgmt.EnvKVDTO       `json:"env,omitempty"`
	Mounts        []dockermgmt.MountDTO       `json:"mounts,omitempty"`
	RestartPolicy string                      `json:"restartPolicy,omitempty"`
	Network       string                      `json:"network,omitempty"`
}

// CreateContainer POST /docker/container/create（admin）：绑定 → service → 包络。
func (h *Handler) ContainerCreate(c *gin.Context) {
	var req containerCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	actor, ok := actorOf(c)
	if !ok {
		return
	}
	data, err := h.svc.CreateContainer(c.Request.Context(), actor, dockermgmt.ContainerCreateRequest{
		Name:          req.Name,
		Image:         req.Image,
		PortMappings:  req.PortMappings,
		Env:           req.Env,
		Mounts:        req.Mounts,
		RestartPolicy: req.RestartPolicy,
		Network:       req.Network,
	})
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// VolumeCreate POST /docker/volume/create（admin）。
func (h *Handler) VolumeCreate(c *gin.Context) {
	var req dockermgmt.VolumeCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	actor, ok := actorOf(c)
	if !ok {
		return
	}
	data, err := h.svc.CreateVolume(c.Request.Context(), actor, req.Name, req.Driver)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// NetworkCreate POST /docker/network/create（admin）。
func (h *Handler) NetworkCreate(c *gin.Context) {
	var req dockermgmt.NetworkCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	actor, ok := actorOf(c)
	if !ok {
		return
	}
	data, err := h.svc.CreateNetwork(c.Request.Context(), actor, req.Name, req.Driver, req.Subnet, req.Gateway)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ComposeCreate POST /docker/compose/create（admin）。
func (h *Handler) ComposeCreate(c *gin.Context) {
	var req dockermgmt.ComposeCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	actor, ok := actorOf(c)
	if !ok {
		return
	}
	data, err := h.svc.CreateComposeProject(c.Request.Context(), actor, req.ProjectName, req.ComposeYaml, req.AutoUp)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}
