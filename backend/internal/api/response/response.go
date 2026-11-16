// Package response 是统一响应包络（api-account-auth.md §3.1 / D-07 + §3.4）：
// HTTP 一律 200；成功 {"code":0,"message":"ok","data":{...}}，失败 code 字段
// 上线契约错误标识符号串（AUTH_001 等，数值码不使用——Gate 2 E1 修复，
// 前端 http.ts 以 String(env.code) 比对 'AUTH_xxx'）。
// api 层与 middleware 共同消费，不依赖任何 service/store（避免环依赖）。
package response

import (
	"errors"
	"log/slog"

	"github.com/dougong/dougong/internal/errcode"
	"github.com/gin-gonic/gin"
)

// Envelope 是统一响应包络。Code：成功=数值 0；失败=错误标识符（string）。
// JSON 编码自然产生 number/string 两种形态，与前端 Envelope<unknown> 兼容。
type Envelope struct {
	Code    any    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// OK 输出成功包络（code=0）。
func OK(c *gin.Context, data any) {
	c.JSON(200, Envelope{Code: 0, Message: "ok", Data: data})
}

// Fail 输出失败包络：errcode.Error 携带 symbol/message/data 原样透出；
// 未知错误不外泄内部细节（全量细节进结构化日志，响应给脱敏 INTERNAL）。
func Fail(c *gin.Context, err error) {
	var ec *errcode.Error
	if errors.As(err, &ec) {
		c.JSON(200, Envelope{Code: ec.Symbol(), Message: ec.Message(), Data: ec.Data()})
		return
	}
	slog.ErrorContext(c.Request.Context(), "unhandled error",
		"path", c.FullPath(), "method", c.Request.Method, "err", err)
	c.JSON(200, Envelope{Code: errcode.ErrInternal.Symbol(), Message: errcode.ErrInternal.Message(), Data: nil})
}

// FailWith 直接以错误定义输出失败包络（middleware 快速路径）。
func FailWith(c *gin.Context, ec *errcode.Error) {
	c.JSON(200, Envelope{Code: ec.Symbol(), Message: ec.Message(), Data: ec.Data()})
}

// FailWithStatus 以指定 HTTP 状态输出失败包络（WS 握手拒绝通道：
// 升级前仍是 HTTP 语义——api-terminal.md §3.1；常规 HTTP 接口仍用
// 200 包络通道，不经过本函数）。
func FailWithStatus(c *gin.Context, status int, ec *errcode.Error) {
	c.JSON(status, Envelope{Code: ec.Symbol(), Message: ec.Message(), Data: ec.Data()})
}

// HTTPStatus 映射错误标识的 HTTP 状态档（api-terminal.md §3.4 语义档位 →
// 数值：401 未认证 / 403 越界与 Origin / 400 拒绝与参数 / 500 失败；
// 状态仅用于 WS 握手拒绝，包络 code 仍是 symbol——全局通道不变）。
func HTTPStatus(ec *errcode.Error) int {
	switch ec.Symbol() {
	case "AUTH_001", "AUTH_002":
		return 401
	case "AUTH_003", "AUTH_008":
		return 403
	case "TERM_004", "TERM_007", "INTERNAL":
		return 500
	default:
		return 400
	}
}
