package terminal

import (
	"context"
	"errors"
	"net/http"

	"github.com/dougong/dougong/internal/api/middleware"
	"github.com/dougong/dougong/internal/api/response"
	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	terminalsvc "github.com/dougong/dougong/internal/service/terminal"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// Handler 聚合终端模块的 service 依赖。
type Handler struct {
	svc      *terminalsvc.Service
	upgrader websocket.Upgrader
}

// NewHandler 构造 handler。upgrader.CheckOrigin 恒真——Origin 同源校验在
// TerminalHandshakeGuard ①（D-09），此处不重复判定。
func NewHandler(svc *terminalsvc.Service) *Handler {
	return &Handler{
		svc: svc,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(*http.Request) bool { return true },
		},
	}
}

// Options GET /terminal/user/options（admin）：候选系统用户清单（D-02）。
// 常规 HTTP 通道（200 包络；GET 豁免 CSRF）。
func (h *Handler) Options(c *gin.Context) {
	data, err := h.svc.ListUserOptions(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// Connect WS /terminal/session/connect（admin）：握手准入 ④~⑧（Admit，
// 升级前 HTTP 语义拒绝）→ Upgrade（⑨）→ Attach（登记+ready 帧+泵+读循环，
// 当前 goroutine 阻塞至会话终结）。
func (h *Handler) Connect(c *gin.Context) {
	ident, ok := middleware.WsIdentity(c)
	if !ok { // 防御双保险：守卫链路下不可达
		response.FailWithStatus(c, 401, errcode.ErrUnauthenticated)
		return
	}
	pending, err := h.svc.Admit(c.Request.Context(), terminalsvc.WsActor{
		UserID: ident.UserID, Role: ident.Role, RawToken: ident.RawToken,
	}, c.Query("identitySource"), c.Query("username"), c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		wsFail(c, err)
		return
	}
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		// 升级失败：gorilla 已写 HTTP 错误响应；丢弃已准入会话（Kill PTY +
		// closed 审计，A-T5/I-07）。
		h.svc.Abort(pending, terminalsvc.ReasonNetworkError)
		return
	}
	if err := h.svc.Attach(pending, wrapConn(conn)); err != nil {
		// 配额竞态拒绝（升级后帧层反馈已由 Attach 内 closed 帧给出）。
		wsFail(c, err)
		return
	}
}

// wsFail 以 HTTP 状态 + 包络输出 WS 握手拒绝（升级前仍是 HTTP 语义，§3.1；
// 未知错误脱敏 500）。
func wsFail(c *gin.Context, err error) {
	var ec *errcode.Error
	if errors.As(err, &ec) {
		response.FailWithStatus(c, response.HTTPStatus(ec), ec)
		return
	}
	response.FailWithStatus(c, 500, errcode.ErrInternal)
}

// panelDirectory 适配 account-auth 用户清单为 PanelDirectory 端口
// （A-T6：WS 路径 VerifyWs 只返回 uid/role，审计 ActorName 经此解析；
// 进程内小用户量全量查一次）。
type panelDirectory struct {
	auth *authsvc.Service
}

// NewPanelDirectory 构造 PanelDirectory 适配器（main 装配注入 Service）。
func NewPanelDirectory(auth *authsvc.Service) terminalsvc.PanelDirectory {
	return panelDirectory{auth: auth}
}

// UsernameByID 按面板用户 ID 解析用户名。
func (p panelDirectory) UsernameByID(ctx context.Context, userID int64) (string, bool) {
	users, err := p.auth.ListUsers(ctx)
	if err != nil {
		return "", false
	}
	for _, u := range users {
		if u.ID == userID {
			return u.Username, true
		}
	}
	return "", false
}
