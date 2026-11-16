package middleware

import (
	"github.com/dougong/dougong/internal/api/response"
	"github.com/dougong/dougong/internal/errcode"
	"github.com/gin-gonic/gin"
)

// RequireRole 角色边界守卫（ADR-018 RBAC）：当前身份角色不在允许集合内
// 返回 AUTH_008（readonly 越界/非 admin）。roles 为空表示拒绝全部（防误配）。
func RequireRole(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}
	return func(c *gin.Context) {
		id, ok := Identity(c)
		if !ok {
			response.FailWith(c, errcode.ErrUnauthenticated)
			c.Abort()
			return
		}
		if !allowed[id.Role] {
			response.FailWith(c, errcode.ErrForbidden)
			c.Abort()
			return
		}
		c.Next()
	}
}
