package servicemgmt

import (
	"context"

	"github.com/dougong/dougong/internal/api/middleware"
	"github.com/dougong/dougong/internal/api/response"
	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	svcsvc "github.com/dougong/dougong/internal/service/servicemgmt"
	"github.com/gin-gonic/gin"
)

// Handler 聚合服务管理接口的 service 依赖。
type Handler struct {
	svc *svcsvc.Service
}

// NewHandler 构造 handler。
func NewHandler(svc *svcsvc.Service) *Handler { return &Handler{svc: svc} }

// unitRequest 是五动作接口的请求体（name=unit 全名寻址，D-09；形态校验唯一
// 权威在 service 层 SVC_001——handler 不做 required 绑定以保单一校验通道）。
type unitRequest struct {
	Name string `json:"name"`
}

// actionOp 是动作接口的 service 操作签名（job 与 autostart 两族统一经 any
// 承载 data——handler 薄壳不做业务分派）。
type actionOp func(ctx context.Context, actor *authsvc.Identity, name string) (any, error)

// List GET /services/unit/list（admin+readonly）：query {state?, search?}——
// 过滤/搜索/排序均在 service（SVC_002/D-02）。
func (h *Handler) List(c *gin.Context) {
	data, err := h.svc.List(c.Request.Context(), c.Query("state"), c.Query("search"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// Detail GET /services/unit/detail（admin+readonly）：query {name}——形态与
// 存在性校验在 service（SVC_001/SVC_003）。
func (h *Handler) Detail(c *gin.Context) {
	data, err := h.svc.Detail(c.Request.Context(), c.Query("name"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// Start POST /services/unit/start（admin+CSRF）：等待终态 ≤90s（D-01）。
func (h *Handler) Start(c *gin.Context) { h.action(c, adaptJob(h.svc.Start)) }

// Stop POST /services/unit/stop（admin+CSRF）：等待终态。
func (h *Handler) Stop(c *gin.Context) { h.action(c, adaptJob(h.svc.Stop)) }

// Restart POST /services/unit/restart（admin+CSRF）：等待终态（failed 服务
// 首要恢复动作）。
func (h *Handler) Restart(c *gin.Context) { h.action(c, adaptJob(h.svc.Restart)) }

// Enable POST /services/unit/enable（admin+CSRF）：开机自启启用（轻量同步）。
func (h *Handler) Enable(c *gin.Context) { h.action(c, adaptAutostart(h.svc.Enable)) }

// Disable POST /services/unit/disable（admin+CSRF）：开机自启禁用。
func (h *Handler) Disable(c *gin.Context) { h.action(c, adaptAutostart(h.svc.Disable)) }

// action 是五动作接口共用薄壳：绑定 {name} → 身份 → service → 包络。
func (h *Handler) action(c *gin.Context, op actionOp) {
	var req unitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	id, ok := middleware.Identity(c)
	if !ok { // 防御双保险：AuthRequired 链路下不可达
		response.FailWith(c, errcode.ErrUnauthenticated)
		return
	}
	data, err := op(c.Request.Context(), id, req.Name)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// adaptJob 将 job 族 service 方法适配为 actionOp 签名。
func adaptJob(op func(context.Context, *authsvc.Identity, string) (*svcsvc.ActionData, error)) actionOp {
	return func(ctx context.Context, actor *authsvc.Identity, name string) (any, error) {
		return op(ctx, actor, name)
	}
}

// adaptAutostart 将自启族 service 方法适配为 actionOp 签名。
func adaptAutostart(op func(context.Context, *authsvc.Identity, string) (*svcsvc.AutostartData, error)) actionOp {
	return func(ctx context.Context, actor *authsvc.Identity, name string) (any, error) {
		return op(ctx, actor, name)
	}
}

// bindError 将 gin 绑定错误转 AUTH_012（body 字段级提示；复用 account-auth
// 参数码族，panel-core bindError 同法——A-S9）。
func bindError(err error) *errcode.Error {
	return errcode.ErrValidation.WithFields(errcode.FieldError{
		Field: "body", Message: err.Error(),
	})
}
