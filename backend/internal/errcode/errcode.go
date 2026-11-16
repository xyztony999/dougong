// Package errcode 是业务错误码集中定义与注册机制（共享内核：api 与 service
// 共同依赖，避免 service→api 反向依赖；backend-rules §4「或同层」条款）。
//
// 错误由 symbol（契约错误标识，如 AUTH_001）+ message（人读提示）构成，
// 可选携带 data 载荷（如 AUTH_004 的 remainingSeconds、AUTH_012 的 fields）。
//
// 响应通道（api-account-auth.md §3.4，Gate 2 E1 修复 2026-11-14）：
// 包络 code 字段直接上线 symbol 符号串（成功恒为数值 0）；数值错误码不使用
// ——前端 http.ts 以 String(env.code) 比对 'AUTH_xxx' 符号，契约 §3.4 的
// code 列即 AUTH_001~014 符号本身。
package errcode

import "fmt"

// Error 实现标准 error 接口的业务错误；经 %w 包装后仍可被 errors.As 提取。
type Error struct {
	symbol  string
	message string
	data    any
}

// New 定义一个业务错误（各模块错误集中在自己的文件中定义，禁止 handler 裸写码）。
func New(symbol, message string) *Error {
	return &Error{symbol: symbol, message: message}
}

// Error 实现 error 接口（含 symbol，便于日志检索）。
func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.symbol, e.message)
}

// Symbol 返回契约错误标识（如 AUTH_001；响应包络 code 字段取此值）。
func (e *Error) Symbol() string { return e.symbol }

// Message 返回人读提示（响应包络 message 字段）。
func (e *Error) Message() string { return e.message }

// Data 返回附加载荷（响应包络 data 字段；无则为 nil）。
func (e *Error) Data() any { return e.data }

// WithData 返回携带 data 载荷的副本（sentinel 本体不可变，避免共享污染）。
func (e *Error) WithData(data any) *Error {
	ne := *e
	ne.data = data
	return &ne
}

// FieldError 是 AUTH_012 类逐字段校验错误的单项（R-14：data.fields[]）。
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationData 是校验失败错误的 data 结构：{"fields":[{field,message}]}。
type ValidationData struct {
	Fields []FieldError `json:"fields"`
}

// WithFields 返回附带逐字段错误列表的校验失败错误副本。
func (e *Error) WithFields(fields ...FieldError) *Error {
	return e.WithData(ValidationData{Fields: fields})
}
