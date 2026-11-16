package terminal

import (
	"encoding/json"
	"time"
)

// UserOptionItem 是候选系统用户条目（GET /terminal/user/options items[]）。
type UserOptionItem struct {
	Name  string `json:"name"`
	UID   uint32 `json:"uid"`
	Shell string `json:"shell"`
}

// UserOptionsData 是 GET /terminal/user/options 响应 data（按 name 字典序
// 升序；全量不分页——单机数十至数百条量级，D-02/service-mgmt D-02 同口径）。
type UserOptionsData struct {
	Items       []UserOptionItem `json:"items"`
	GeneratedAt string           `json:"generatedAt"`
}

// wsClientFrame 是客户端帧解码载体（input/resize 白名单外均协议违规 I-06）。
type wsClientFrame struct {
	Type string `json:"type"`
	Data string `json:"data"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// wsServerFrame 是服务端帧编码载体（ready/output/closed，D-01）。
type wsServerFrame struct {
	Type      string `json:"type"`
	Data      any    `json:"data,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

// IdentityPayload 是 ready 帧 identity 载荷。
type IdentityPayload struct {
	Source  string `json:"source"`
	RunUser string `json:"runUser"`
}

// LimitsPayload 是 ready 帧 limits 载荷（前端权威值）。
type LimitsPayload struct {
	MaxConcurrent      int `json:"maxConcurrent"`
	IdleTimeoutSeconds int `json:"idleTimeoutSeconds"`
	HardLimitSeconds   int `json:"hardLimitSeconds"`
}

// ReadyPayload 是 ready 帧 data（连接后服务端首帧）。
type ReadyPayload struct {
	SessionID string          `json:"sessionId"`
	Identity  IdentityPayload `json:"identity"`
	Limits    LimitsPayload   `json:"limits"`
}

// ClosedPayload 是 closed 帧 data（断开通知一次性完整反馈，D-05）。
type ClosedPayload struct {
	Reason          string `json:"reason"`
	ClosedAt        string `json:"closedAt"`
	DurationSeconds int64  `json:"durationSeconds"`
}

// formatTS 统一 ISO-8601 UTC（公共约定）。
func formatTS(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// marshalFrame 编码服务端帧（ready/output/closed）。
func marshalFrame(f wsServerFrame) ([]byte, error) { return json.Marshal(f) }
