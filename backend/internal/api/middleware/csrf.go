package middleware

import (
	"crypto/subtle"
	"net/url"

	"github.com/dougong/dougong/internal/api/response"
	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	"github.com/gin-gonic/gin"
)

// CsrfGuard 变更类请求（POST）CSRF 双重校验（R-02/§3.1）：
//  1. X-CSRF-Token 头须等于会话绑定 csrf_token（subtle 常量时间比较，I-12）；
//  2. Origin/Referer 同源校验（二者全缺或不同源均拒绝）。
//
// 须挂在 AuthRequired 之后（依赖 Identity）。GET 与无会话接口不经本中间件。
func CsrfGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := Identity(c)
		if !ok {
			response.FailWith(c, errcode.ErrCSRF)
			c.Abort()
			return
		}
		got := c.GetHeader(authsvc.CSRFHeader)
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(id.CsrfToken)) != 1 {
			response.FailWith(c, errcode.ErrCSRF)
			c.Abort()
			return
		}
		if !sameOrigin(c) {
			response.FailWith(c, errcode.ErrCSRF)
			c.Abort()
			return
		}
		c.Next()
	}
}

// sameOrigin 校验 Origin（缺失时退回 Referer）与请求 Host 同源；
// 两者均缺失视为非浏览器/可疑请求，拒绝（Strict 语义）。
func sameOrigin(c *gin.Context) bool {
	host := c.Request.Host
	check := func(raw string) bool {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return false
		}
		return u.Host == host
	}
	if org := c.GetHeader("Origin"); org != "" {
		return check(org)
	}
	if ref := c.GetHeader("Referer"); ref != "" {
		return check(ref)
	}
	return false
}

// SameOrigin 导出同源判定（WS 握手守卫复用同一语义——docker-mgmt 日志跟随
// D-03/Cookie+Origin；与 CsrfGuard 同源判定单一实现）。
func SameOrigin(c *gin.Context) bool { return sameOrigin(c) }
