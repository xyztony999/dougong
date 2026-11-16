// Package terminal 是 Web 终端模块 HTTP/WS 传输层（api-terminal.md §3.1/§4.1.4：
// 1 HTTP 薄壳 + 1 WS 升级端点；握手准入 ①~③在 TerminalHandshakeGuard、
// ④~⑧在 service.Admit、⑨在 handler 升级后 Attach）。
package terminal

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// writeWait 单帧写截止（输出/closed 帧；防慢消费者永久占住写锁）。
const writeWait = 10 * time.Second

// gorillaConn 将 gorilla/websocket 连接适配为 terminal.WSConn。
// WriteFrame 经互斥串行化（输出/ready/closed 帧序不乱）；Ping 走
// WriteControl 控制帧通道（gorilla 允许与数据帧并发，A-T8）。
type gorillaConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func wrapConn(c *websocket.Conn) *gorillaConn {
	// 抑制 gorilla 默认 close 自动回执：closed 帧是本协议的应用级关闭反馈
	//（D-05 一次性完整反馈），须在 close 握手回执之前送达客户端——默认
	// handler 收到客户端 close 即回写 close 帧，会使后续数据帧写失败
	//（"websocket: close sent"）。
	c.SetCloseHandler(func(int, string) error { return nil })
	return &gorillaConn{conn: c}
}

// ReadFrame 读一条文本帧（返回原始 JSON 字节；连接层错误原样上抛）。
func (g *gorillaConn) ReadFrame() ([]byte, error) {
	_, data, err := g.conn.ReadMessage()
	return data, err
}

// WriteFrame 串行写一条文本帧。
func (g *gorillaConn) WriteFrame(payload []byte) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	_ = g.conn.SetWriteDeadline(time.Now().Add(writeWait))
	return g.conn.WriteMessage(websocket.TextMessage, payload)
}

// Ping 发送协议层 ping 控制帧（心跳，D-03）。
func (g *gorillaConn) Ping() error {
	return g.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait))
}

// SetPongHandler 注册 pong 回调（读截止续期）。
func (g *gorillaConn) SetPongHandler(h func()) {
	g.conn.SetPongHandler(func(string) error {
		h()
		return nil
	})
}

// SetReadDeadline 读截止（失联判定，A-T8）。
func (g *gorillaConn) SetReadDeadline(t time.Time) error {
	return g.conn.SetReadDeadline(t)
}

// Close 关闭底层连接。
func (g *gorillaConn) Close() error { return g.conn.Close() }
