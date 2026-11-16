// Package panel 是面板基座（panel-core）模块 HTTP 传输层（7 接口，薄壳：
// 绑定 → service → 包络响应；业务逻辑一律在 service，api-panel-core.md §4.1.4）。
package panel

import (
	"github.com/dougong/dougong/internal/api/middleware"
	"github.com/dougong/dougong/internal/api/response"
	panelsvc "github.com/dougong/dougong/internal/service/panel"
	"github.com/gin-gonic/gin"
)

// Handler 聚合 panel-core 接口的 service 依赖。
type Handler struct {
	svc *panelsvc.Service
}

// NewHandler 构造 handler。
func NewHandler(svc *panelsvc.Service) *Handler { return &Handler{svc: svc} }

// ---- 配置（4） ----

// ConfigDetail GET /panel/config/detail（admin+readonly）：配置单例全量读取
// （含生效状态与会话透传）。
func (h *Handler) ConfigDetail(c *gin.Context) {
	view, err := h.svc.Detail(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// ConfigUpdate POST /panel/config/update（admin）：配置项部分更新
// （生效分类判定；响应为更新后配置视图）。
func (h *Handler) ConfigUpdate(c *gin.Context) {
	var req panelsvc.UpdateConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	id, ok := middleware.Identity(c)
	if !ok {
		response.FailWith(c, errcodeUnauthenticated())
		return
	}
	view, err := h.svc.UpdateConfig(c.Request.Context(), id, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// RegenerateEntryPath POST /panel/config/regenerate-entry-path（admin）：
// 重新生成随机安全入口路径。
func (h *Handler) RegenerateEntryPath(c *gin.Context) {
	id, ok := middleware.Identity(c)
	if !ok {
		response.FailWith(c, errcodeUnauthenticated())
		return
	}
	res, err := h.svc.RegenerateEntryPath(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// UploadCertificate POST /panel/config/upload-certificate（admin）：
// 导入证书与私钥（PEM 文本 JSON 字段，D-P05）。
func (h *Handler) UploadCertificate(c *gin.Context) {
	var req panelsvc.UploadCertificateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	id, ok := middleware.Identity(c)
	if !ok {
		response.FailWith(c, errcodeUnauthenticated())
		return
	}
	res, err := h.svc.UploadCertificate(c.Request.Context(), id, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// ---- 菜单与系统（3） ----

// MenuList GET /panel/menu/list（admin+readonly）：菜单树（按操作者角色
// 服务端过滤，D-P07）。
func (h *Handler) MenuList(c *gin.Context) {
	id, ok := middleware.Identity(c)
	if !ok {
		response.FailWith(c, errcodeUnauthenticated())
		return
	}
	response.OK(c, gin.H{"items": h.svc.MenuList(id.Role)})
}

// Version GET /panel/system/version（admin+readonly）：版本与构建信息
// （构建期 ldflags 注入，FR-03）。
func (h *Handler) Version(c *gin.Context) {
	response.OK(c, panelsvc.GetVersionInfo())
}

// UpdateCheck POST /panel/system/update-check（admin）：手动触发一次版本
// 检查（双重门控，I-P05；失败不自动重试，D-P09）。
func (h *Handler) UpdateCheck(c *gin.Context) {
	res, err := h.svc.UpdateCheck(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}
