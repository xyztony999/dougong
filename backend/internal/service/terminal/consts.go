// Package terminal 是 Web 终端应用服务（api-terminal.md §4.1）：进程内
// terminal_session 聚合（零持久化 I-04——无表无 SQL，面板重启即消亡）、
// 握手准入编排（I-01 三重准入 + 建立审计 D-06）、帧协议（D-01 JSON+Base64）、
// 生命周期管控（I-03 空闲/硬上限 + D-03 心跳 + D-07 面板会话复核）与
// 统一清理（I-02/I-07）。不感知 HTTP/WS 传输（帧通道经 WSConn 端口注入）。
package terminal

import (
	"path"
	"strings"
	"time"
)

// 终端会话运行身份来源（terminal_identity_source 枚举）。
const (
	IdentitySourcePanel     = "panel_run_user"
	IdentitySourceSpecified = "specified_system_user"
)

// 断开原因（terminal_end_reason 枚举，D3 增补 protocol_error）。
const (
	ReasonSelfClose           = "self_close"
	ReasonIdleTimeout         = "idle_timeout"
	ReasonHardLimit           = "hard_limit"
	ReasonPanelSessionInvalid = "panel_session_invalid"
	ReasonNetworkError        = "network_error"
	ReasonProtocolError       = "protocol_error"
)

// WS 帧类型（D-01 帧协议常量，非枚举池条目——纯传输协议）。
const (
	FrameReady  = "ready"
	FrameInput  = "input"
	FrameResize = "resize"
	FrameOutput = "output"
	FrameClosed = "closed"
)

// 审计动作码（api-terminal.md §3.4 业务常量；category=security，
// object_type=terminal_session，object_id=sessionId）。
const (
	ActionSessionOpened = "terminal_session_opened"
	ActionSessionClosed = "terminal_session_closed"
)

// ObjectTypeTerminalSession 审计对象类型。
const ObjectTypeTerminalSession = "terminal_session"

// 业务常量（ADR-023 固定标准档，编译期常量非配置键——PRD OUT-6）。
const (
	// MaxConcurrentPerUser 每用户并发终端上限（TERM_001）。
	MaxConcurrentPerUser = 5
	// IdleTimeout 空闲超时（仅 input/resize 刷新 lastActiveAt，D1 从严口径）。
	IdleTimeout = 30 * time.Minute
	// HardLimit 单连接硬上限（createdAt+8h，与活跃无关）。
	HardLimit = 8 * time.Hour
	// ScanTick 生命周期扫描间隔（空闲/硬上限判定精度）。
	ScanTick = 10 * time.Second
	// PanelRecheckInterval 面板会话复核间隔（D-07 联动断开最大延迟）。
	PanelRecheckInterval = 30 * time.Second
	// PingInterval WS 心跳发送间隔（D-03：30s ping，连续 2 次未答判失联）。
	PingInterval = 30 * time.Second
	// PongWait 读截止窗口（2×PingInterval + 余量；超时=network_error）。
	PongWait = 75 * time.Second
	// FrameDataLimit 单帧 data 解码后上限（I-06：超限=协议违规断开）。
	FrameDataLimit = 16 * 1024
	// OutputRingSize 每会话输出环形缓冲（D-04 背压：溢出丢弃最旧+truncated 标记）。
	OutputRingSize = 256 * 1024
	// ResizeMax cols/rows 上限（A-T4：越界=协议违规）。
	ResizeMax = 500
	// DefaultCols / DefaultRows 初始 PTY 尺寸（前端 ready 后首个 resize 帧校正）。
	DefaultCols = 80
	DefaultRows = 24
)

// nonInteractiveShells 非交互 shell 排除集（§3.4 常量：nologin、false、halt、
// shutdown 及空 shell；按 basename 匹配，如 /sbin/nologin→nologin）。
var nonInteractiveShells = map[string]bool{
	"nologin":  true,
	"false":    true,
	"halt":     true,
	"shutdown": true,
	"":         true,
}

// IsInteractiveShell 判定 shell 是否交互登录 shell（候选清单服务端过滤与
// TERM_003 判定共用，D1 落定边界）。
func IsInteractiveShell(shell string) bool {
	trimmed := strings.TrimSpace(shell)
	if trimmed == "" {
		return false // 空 shell 排除（§3.4 排除集含空 shell）
	}
	base := path.Base(trimmed)
	if i := strings.LastIndex(base, "\\"); i >= 0 { // Windows 风格路径防御（目标环境不出现）
		base = base[i+1:]
	}
	return !nonInteractiveShells[strings.ToLower(base)]
}
