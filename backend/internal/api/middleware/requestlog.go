package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestLog 是 HTTP 访问审计日志（R-03 的传输层补充；业务审计走
// AuditService.Record）：method/path/status/latency/ip/uid 结构化记录。
// 敏感字段（Cookie/token/密码载荷）绝不落日志。
func RequestLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		attrs := []any{
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency_ms", time.Since(start).Milliseconds(),
			"ip", c.ClientIP(),
		}
		if id, ok := Identity(c); ok {
			attrs = append(attrs, "uid", id.UserID, "username", id.Username)
		}
		slog.InfoContext(c.Request.Context(), "http request", attrs...)
	}
}
