// Package monitoring 是系统监控模块 HTTP 传输层（4 只读 GET 接口，薄壳：
// 绑定 → service → 包络响应；业务逻辑一律在 service，api-monitoring.md §3.1/§4）。
package monitoring

import (
	"github.com/dougong/dougong/internal/api/response"
	monsvc "github.com/dougong/dougong/internal/service/monitoring"
	"github.com/gin-gonic/gin"
)

// Handler 聚合监控接口的 service 依赖。
type Handler struct {
	svc *monsvc.Service
}

// NewHandler 构造 handler。
func NewHandler(svc *monsvc.Service) *Handler { return &Handler{svc: svc} }

// Snapshot GET /monitoring/metrics/snapshot（admin+readonly）：实时快照——
// 最新采样全量（processes 同源同响应，§4.1.4）。
func (h *Handler) Snapshot(c *gin.Context) {
	data, err := h.svc.Snapshot(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// History GET /monitoring/metrics/history（admin+readonly）：query
// {window, dimension, scope?}——枚举校验与序列投影均在 service（ParamsGuard）。
func (h *Handler) History(c *gin.Context) {
	data, err := h.svc.History(c.Request.Context(),
		c.Query("window"), c.Query("dimension"), c.Query("scope"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// TopProcesses GET /monitoring/processes/top（admin+readonly）：TOP 进程清单
// （最新轮，≤10 降序，I-06）。
func (h *Handler) TopProcesses(c *gin.Context) {
	data, err := h.svc.TopProcesses(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// SystemInfo GET /monitoring/system/info（admin+readonly）：系统信息实时投影
// （静态+运行信息单响应，§4.2.4）。
func (h *Handler) SystemInfo(c *gin.Context) {
	data, err := h.svc.SystemInfo(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}
