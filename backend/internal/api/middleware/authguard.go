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

const identityKey = "dougong.auth.identity"

// SetIdentity 由 AuthRequired 写入身份（仅供本包与测试使用）。
func SetIdentity(c *gin.Context, id *authsvc.Identity) {
	c.Set(identityKey, id)
}

// Identity 读取当前认证身份（ok=false 表示未认证/中间件未生效）。
func Identity(c *gin.Context) (*authsvc.Identity, bool) {
	v, ok := c.Get(identityKey)
	if !ok {
		return nil, false
	}
	id, ok := v.(*authsvc.Identity)
	return id, ok && id != nil
}

// Initializer 是初始化状态查询契约（InitGuard 消费，authsvc.Service 实现）。
type Initializer interface {
	IsInitialized(ctx context.Context) (bool, error)
}

// InitGuard 是 I-01 守卫：系统初始化前拦截一切业务接口与会话签发
// （AUTH_009 语义）；豁免路径：/auth/init/check 与 /auth/init/wizard
// （向导本身的重复初始化由服务层单行仲裁拒绝）。
func InitGuard(init Initializer) gin.HandlerFunc {
	exempt := map[string]bool{
		"/auth/init/check":  true,
		"/auth/init/wizard": true,
	}
	return func(c *gin.Context) {
		if exempt[c.Request.URL.Path] {
			c.Next()
			return
		}
		initialized, err := init.IsInitialized(c.Request.Context())
		if err != nil {
			slog.ErrorContext(c.Request.Context(), "init guard: read init state",
				"err", err, "path", c.Request.URL.Path)
			response.FailWith(c, errcode.ErrInternal)
			c.Abort()
			return
		}
		if initialized {
			c.Next()
			return
		}
		response.FailWith(c, errcode.ErrAlreadyInitialized)
		c.Abort()
	}
}

// TokenVerifier 是会话验证契约（AuthRequired 消费，authsvc.Service 实现：
// 含 I-11 判定与滑动续期）。
type TokenVerifier interface {
	VerifyToken(ctx context.Context, rawToken string) (*authsvc.Identity, error)
}

// AuthRequired 要求有效会话：读 dg_session Cookie → VerifyToken。
// 失败统一 200 包络：AUTH_001（无/无效/已吊销）或 AUTH_002（已过期），
// 前端据此转登录页（I-13 立即生效）。
func AuthRequired(tv TokenVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, err := c.Cookie(authsvc.SessionCookieName)
		if err != nil || raw == "" {
			response.FailWith(c, errcode.ErrUnauthenticated)
			c.Abort()
			return
		}
		id, err := tv.VerifyToken(c.Request.Context(), raw)
		if err != nil {
			var ec *errcode.Error
			if errors.As(err, &ec) {
				response.FailWith(c, ec)
			} else {
				// 非 errcode 错误（存储异常等）：脱敏为 INTERNAL 符号码（ErrInternal），细节进日志。
				slog.ErrorContext(c.Request.Context(), "auth required: verify token",
					"err", err, "path", c.Request.URL.Path)
				response.FailWith(c, errcode.ErrInternal)
			}
			c.Abort()
			return
		}
		SetIdentity(c, id)
		c.Next()
	}
}
