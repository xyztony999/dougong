package middleware

import (
	"context"
	"errors"
	"log/slog"

	"github.com/dougong/dougong/internal/api/response"
	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	"github.com/gin-gonic/gin"
)

const wsIdentityKey = "dougong.terminal.ws.identity"

// WsIdentityData 是 WS 握手身份（TerminalHandshakeGuard 产出、终端模块
// handler 消费；RawToken=panelSessionRef 存续期复核凭据，D-07）。
type WsIdentityData struct {
	UserID   int64
	Role     string
	RawToken string
}

// WsVerifier 是 WS 握手会话验证契约（SessionAuthService.VerifyWs OHS——
// R-18 只读不续期；实现为 authsvc.Service）。
type WsVerifier interface {
	VerifyWs(ctx context.Context, rawToken string) (userID int64, role string, err error)
}

// TerminalHandshakeGuard 是终端 WS 握手守卫（api-terminal.md §3.1/§4.1.4
// I-01 ①~③，顺序固定）：① Origin 同源（D-09：浏览器 WS 必带 Origin，
// 缺失/非同源=AUTH_003）→ ② VerifyWs 面板会话鉴权（AUTH_001/002）→
// ③ 角色=admin（AUTH_008——本模块 readonly 整域禁用，ADR-018）。
// 拒绝以 HTTP 状态+包络 JSON 上线（升级前仍是 HTTP 语义，§3.1）；
// ④~⑧（参数/并发/身份/spawn/审计）在 service.Admit 编排。
func TerminalHandshakeGuard(v WsVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ① Origin 同源（复用 CsrfGuard 同源判定语义；帧层不设 CSRF——D-09）。
		if !sameOrigin(c) {
			response.FailWithStatus(c, response.HTTPStatus(errcode.ErrCSRF), errcode.ErrCSRF)
			c.Abort()
			return
		}
		// ② 面板会话鉴权（HttpOnly Cookie dg_session，R-18/R-02）。
		raw, err := c.Cookie(authsvc.SessionCookieName)
		if err != nil || raw == "" {
			response.FailWithStatus(c, 401, errcode.ErrUnauthenticated)
			c.Abort()
			return
		}
		uid, role, err := v.VerifyWs(c.Request.Context(), raw)
		if err != nil {
			var ec *errcode.Error
			if errors.As(err, &ec) {
				response.FailWithStatus(c, response.HTTPStatus(ec), ec)
			} else {
				// 存储异常等非业务错误：脱敏 500，细节进日志。
				slog.ErrorContext(c.Request.Context(), "terminal handshake: verify ws",
					"err", err, "path", c.Request.URL.Path)
				response.FailWithStatus(c, 500, errcode.ErrInternal)
			}
			c.Abort()
			return
		}
		// ③ 角色判定（admin 专属；readonly 无界面入口 + 边界拒绝，ADR-018）。
		if role != authsvc.RoleAdmin {
			response.FailWithStatus(c, 403, errcode.ErrForbidden)
			c.Abort()
			return
		}
		c.Set(wsIdentityKey, WsIdentityData{UserID: uid, Role: role, RawToken: raw})
		c.Next()
	}
}

// WsIdentity 读取 WS 握手身份（ok=false 表示守卫未生效）。
func WsIdentity(c *gin.Context) (WsIdentityData, bool) {
	v, ok := c.Get(wsIdentityKey)
	if !ok {
		return WsIdentityData{}, false
	}
	id, ok := v.(WsIdentityData)
	return id, ok
}
