package sitemgmt

import (
	"errors"
	"reflect"

	"github.com/dougong/dougong/internal/api/middleware"
	"github.com/dougong/dougong/internal/api/response"
	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	sitesvc "github.com/dougong/dougong/internal/service/sitemgmt"
	"github.com/gin-gonic/gin"
)

// Handler 聚合网站管理接口的 service 依赖（薄壳：绑定 → service → 包络）。
type Handler struct {
	svc *sitesvc.Service
}

// NewHandler 构造 handler。
func NewHandler(svc *sitesvc.Service) *Handler { return &Handler{svc: svc} }

// identity 取认证身份（AuthRequired 链路下必有；防御双保险）。
func identity(c *gin.Context) *authsvc.Identity {
	id, ok := middleware.Identity(c)
	if !ok {
		return nil
	}
	return id
}

// bindError 绑定错误 → AUTH_012（A-S9/panel-core 同法）。
func bindError(err error) *errcode.Error {
	return errcode.ErrValidation.WithFields(errcode.FieldError{
		Field: "body", Message: err.Error(),
	})
}

// ---- 读组（GET；两角色经路由组链保证）----

// List GET /sites/site/list：query {enabled?}。
func (h *Handler) List(c *gin.Context) {
	data, err := h.svc.ListSites(c.Request.Context(), c.Query("enabled"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// Detail GET /sites/site/detail：query {siteId}。
func (h *Handler) Detail(c *gin.Context) {
	data, err := h.svc.SiteDetail(c.Request.Context(), c.Query("siteId"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ReadSource GET /sites/site/source（admin）：query {siteId}。
func (h *Handler) ReadSource(c *gin.Context) {
	data, err := h.svc.ReadSource(c.Request.Context(), c.Query("siteId"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// TailLog GET /sites/log/tail：query {siteId, type, lines?}。
func (h *Handler) TailLog(c *gin.Context) {
	data, err := h.svc.TailLog(c.Request.Context(),
		c.Query("siteId"), c.Query("type"), c.Query("lines"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ListPhpPools GET /sites/php-pools/list（admin）。
func (h *Handler) ListPhpPools(c *gin.Context) {
	data, err := h.svc.ListPhpPools(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ListCerts GET /sites/cert/list：query {source?, status?}。
func (h *Handler) ListCerts(c *gin.Context) {
	data, err := h.svc.CertList(c.Request.Context(), c.Query("source"), c.Query("status"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ---- 写组（POST；admin+CSRF 经路由组链保证）----

// Create POST /sites/site/create。
func (h *Handler) Create(c *gin.Context) {
	var req sitesvc.CreateSiteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := h.svc.CreateSite(c.Request.Context(), identity(c), req)
	h.respond(c, data, err)
}

// Update POST /sites/site/update。
func (h *Handler) Update(c *gin.Context) {
	var req sitesvc.UpdateSiteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := h.svc.UpdateSite(c.Request.Context(), identity(c), req)
	h.respond(c, data, err)
}

// UpdateProxy POST /sites/proxy/update。
func (h *Handler) UpdateProxy(c *gin.Context) {
	var req sitesvc.UpdateProxyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := h.svc.UpdateProxy(c.Request.Context(), identity(c), req)
	h.respond(c, data, err)
}

// SaveSource POST /sites/site/source-save。
func (h *Handler) SaveSource(c *gin.Context) {
	var req sitesvc.SourceSaveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := h.svc.SaveSource(c.Request.Context(), identity(c), req)
	h.respond(c, data, err)
}

// siteIDRequest 启停共用请求体。
type siteIDRequest struct {
	SiteID string `json:"siteId"`
}

// Enable POST /sites/site/enable。
func (h *Handler) Enable(c *gin.Context) {
	h.toggle(c, true)
}

// Disable POST /sites/site/disable。
func (h *Handler) Disable(c *gin.Context) {
	h.toggle(c, false)
}

func (h *Handler) toggle(c *gin.Context, enable bool) {
	var req siteIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	var data any
	var err error
	if enable {
		data, err = h.svc.EnableSite(c.Request.Context(), identity(c), req.SiteID)
	} else {
		data, err = h.svc.DisableSite(c.Request.Context(), identity(c), req.SiteID)
	}
	h.respond(c, data, err)
}

// Reload POST /sites/nginx/reload（无请求体）。
func (h *Handler) Reload(c *gin.Context) {
	data, err := h.svc.NginxReload(c.Request.Context(), identity(c))
	h.respond(c, data, err)
}

// CertRequest POST /sites/cert/request。
func (h *Handler) CertRequest(c *gin.Context) {
	var req sitesvc.CertRequestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := h.svc.CertRequest(c.Request.Context(), identity(c), req)
	h.respond(c, data, err)
}

// CertDeploy POST /sites/cert/deploy。
func (h *Handler) CertDeploy(c *gin.Context) {
	var req sitesvc.CertRefRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := h.svc.CertDeploy(c.Request.Context(), identity(c), req)
	h.respond(c, data, err)
}

// CertUndeploy POST /sites/cert/undeploy。
func (h *Handler) CertUndeploy(c *gin.Context) {
	var req sitesvc.CertRefRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := h.svc.CertUndeploy(c.Request.Context(), identity(c), req)
	h.respond(c, data, err)
}

// CertRetryRenewal POST /sites/cert/retry-renewal。
func (h *Handler) CertRetryRenewal(c *gin.Context) {
	var req sitesvc.CertIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := h.svc.CertRetryRenewal(c.Request.Context(), identity(c), req.CertID)
	h.respond(c, data, err)
}

// CertRenew POST /sites/cert/renew（v1.2 D-18 手动立即续期）。
func (h *Handler) CertRenew(c *gin.Context) {
	var req sitesvc.CertIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := h.svc.CertRenew(c.Request.Context(), identity(c), req.CertID)
	h.respond(c, data, err)
}

// CertReplace POST /sites/cert/replace（v1.2 D-18 imported 文件替换）。
func (h *Handler) CertReplace(c *gin.Context) {
	var req sitesvc.CertReplaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := h.svc.CertReplace(c.Request.Context(), identity(c), req)
	h.respond(c, data, err)
}

// DnsCredentialList GET /sites/dns-credential/list（v2.0 D-20）。
func (h *Handler) DnsCredentialList(c *gin.Context) {
	data, err := h.svc.DnsCredentialList(c.Request.Context())
	h.respond(c, data, err)
}

// DnsCredentialCreate POST /sites/dns-credential/create（v2.0 D-20）。
func (h *Handler) DnsCredentialCreate(c *gin.Context) {
	var req sitesvc.DnsCredentialCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := h.svc.DnsCredentialCreate(c.Request.Context(), identity(c), req)
	h.respond(c, data, err)
}

// DnsCredentialDelete POST /sites/dns-credential/delete（v2.0 D-20）。
func (h *Handler) DnsCredentialDelete(c *gin.Context) {
	var req sitesvc.CertIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := h.svc.DnsCredentialDelete(c.Request.Context(), identity(c), req.CertID)
	h.respond(c, data, err)
}

// CertDelete POST /sites/cert/delete。
func (h *Handler) CertDelete(c *gin.Context) {
	var req sitesvc.CertIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := h.svc.CertDelete(c.Request.Context(), identity(c), req.CertID)
	h.respond(c, data, err)
}

// respond 统一收尾：写回成功但 reload 失败（SITE_004 类）时非空 data 随错误
// 一并上浮（写回已生效、终态呈现由前端组合）；其余错误走 Fail（保留错误
// 自带 data 载荷，如 SITE_012 reason——服务层 typed-nil 指针不误判为有数据）。
func (h *Handler) respond(c *gin.Context, data any, err error) {
	if err != nil {
		if hasValue(data) {
			var ec *errcode.Error
			if errors.As(err, &ec) {
				c.JSON(200, response.Envelope{Code: ec.Symbol(), Message: ec.Message(), Data: data})
				return
			}
		}
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// hasValue 报告 data 是否携带真实值（typed-nil 指针视为无值）。
func hasValue(data any) bool {
	if data == nil {
		return false
	}
	rv := reflect.ValueOf(data)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Interface:
		return !rv.IsNil()
	}
	return true
}
