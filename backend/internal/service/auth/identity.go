package auth

import "time"

// Identity 是一次已认证请求的身份快照（middleware 注入 gin context；
// handler/service 共用）。CsrfToken 与会话一一绑定（I-12），不得下发到
// 响应体以外的日志（敏感字段脱敏纪律）。
type Identity struct {
	UserID        int64
	Username      string
	Role          string
	TokenHash     string // 会话 token SHA-256（服务端持有，绝不下发）
	SessionDigest string // token_hash 前 8 位，对外会话标识
	CsrfToken     string
	ExpiresAt     time.Time
}

// RequestMeta 是登录/会话签发时的请求元数据（会话清单展示用）。
type RequestMeta struct {
	SourceIP  string
	UserAgent string
}
