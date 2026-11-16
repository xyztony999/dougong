package firewall

import (
	"github.com/dougong/dougong/internal/api/middleware"
	"github.com/dougong/dougong/internal/api/response"
	"github.com/dougong/dougong/internal/errcode"
	fwsvc "github.com/dougong/dougong/internal/service/firewall"
	"github.com/gin-gonic/gin"
)

// Handler 聚合防火墙接口的 service 依赖。
type Handler struct {
	svc *fwsvc.Service
}

// NewHandler 构造 handler。
func NewHandler(svc *fwsvc.Service) *Handler { return &Handler{svc: svc} }

// createRequest 是 rule/create 请求体（§4.1.4：{port, protocol, source?,
// action, confirmDanger?}；值域校验唯一权威在 service 层——R-14 单一校验
// 通道，handler 不做 required 绑定）。
type createRequest struct {
	Port          string  `json:"port"`
	Protocol      string  `json:"protocol"`
	Source        *string `json:"source"`
	Action        string  `json:"action"`
	ConfirmDanger bool    `json:"confirmDanger"`
}

// deleteRequest 是 rule/delete 请求体（§4.1.4：{ruleKey, confirmDanger?}）。
type deleteRequest struct {
	RuleKey       string `json:"ruleKey"`
	ConfirmDanger bool   `json:"confirmDanger"`
}

// BackendStatus GET /firewall/backend/status（admin+readonly）：即时探测结论
// + 权威动作面（D-01）。
func (h *Handler) BackendStatus(c *gin.Context) {
	data, err := h.svc.BackendStatus(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// RuleList GET /firewall/rule/list（admin+readonly）：query {action?,
// protocol?}——多值逗号分隔；筛选/排序/标注均在 service（D-02/D-06）。
func (h *Handler) RuleList(c *gin.Context) {
	data, err := h.svc.RuleList(c.Request.Context(), c.Query("action"), c.Query("protocol"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// PortList GET /firewall/port/list（admin+readonly）：监听×规则对照（D-04）。
func (h *Handler) PortList(c *gin.Context) {
	data, err := h.svc.PortList(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// CreateRule POST /firewall/rule/create（admin+CSRF）：绑定 → 身份 →
// service 编排（校验→动作面→涉险两段式→执行→审计）→ 包络。
func (h *Handler) CreateRule(c *gin.Context) {
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	id, ok := middleware.Identity(c)
	if !ok { // 防御双保险：AuthRequired 链路下不可达
		response.FailWith(c, errcode.ErrUnauthenticated)
		return
	}
	var source string
	if req.Source != nil {
		source = *req.Source
	}
	data, err := h.svc.CreateRule(c.Request.Context(), id, fwsvc.CreateParams{
		Port: req.Port, Protocol: req.Protocol, Source: source,
		Action: req.Action, ConfirmDanger: req.ConfirmDanger,
	})
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// DeleteRule POST /firewall/rule/delete（admin+CSRF）：ruleKey 寻址（D-03）。
func (h *Handler) DeleteRule(c *gin.Context) {
	var req deleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	id, ok := middleware.Identity(c)
	if !ok { // 防御双保险：AuthRequired 链路下不可达
		response.FailWith(c, errcode.ErrUnauthenticated)
		return
	}
	data, err := h.svc.DeleteRule(c.Request.Context(), id, req.RuleKey, req.ConfirmDanger)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// bindError 将 gin 绑定错误转 AUTH_012（body 字段级提示；复用 account-auth
// 参数码族，panel/servicemgmt bindError 同法——A-S9）。
func bindError(err error) *errcode.Error {
	return errcode.ErrValidation.WithFields(errcode.FieldError{
		Field: "body", Message: err.Error(),
	})
}
