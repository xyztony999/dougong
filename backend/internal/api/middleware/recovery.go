// Package middleware 是可复用 HTTP 中间件（account-auth 沉淀、后续模块共享）：
// recovery / 请求访问审计日志 / InitGuard / AuthRequired / CsrfGuard / RoleRequired。
// 链序（设计文档 §3.1）：InitGuard → AuthRequired → CsrfGuard(POST) → RoleRequired。
package middleware

import (
	"log/slog"
	"runtime/debug"

	"github.com/dougong/dougong/internal/api/response"
	"github.com/dougong/dougong/internal/errcode"
	"github.com/gin-gonic/gin"
)

// Recovery 捕获 handler panic：堆栈进结构化日志，响应脱敏为 INTERNAL 符号码（不外泄细节）。
func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, rec any) {
		slog.ErrorContext(c.Request.Context(), "panic recovered",
			"panic", rec, "stack", string(debug.Stack()),
			"path", c.Request.URL.Path, "method", c.Request.Method)
		response.FailWith(c, errcode.ErrInternal)
	})
}
