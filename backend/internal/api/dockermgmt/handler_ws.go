// handler_ws.go 是 2 个日志跟随 WS 端点（D-03）：握手（wsGuard 已过
// Origin+Cookie+角色；此处 daemon 前置 DK_002 → 订阅建立 → 升级，失败以
// HTTP 状态+包络拒绝——A-D13）与帧泵（回放+跟随行→{"type":"log","line"}；
// 终止→{"type":"end","reason"} 后关闭；客户端无上行帧，断开即取消订阅）。
package dockermgmt

import (
	"errors"
	"net/http"
	"sync"

	"github.com/dougong/dougong/internal/adapter/dockerx"
	"github.com/dougong/dougong/internal/api/response"
	"github.com/dougong/dougong/internal/errcode"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// wsFrame 是服务端→客户端帧（D-03 流协议：log/end 两类）。
type wsFrame struct {
	Type   string `json:"type"`
	Line   string `json:"line,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// 帧类型常量（D-03）。
const (
	frameLog = "log"
	frameEnd = "end"
)

// ContainerLogsFollow WS /docker/container/logs-follow（admin+readonly）：
// 握手 query {containerId, tail?}。
func (h *Handler) ContainerLogsFollow(c *gin.Context) {
	info := h.svc.ProbeDaemon(c.Request.Context())
	if info.State != dockerx.StateAvailable {
		wsFail(c, errcode.ErrDkDaemonUnavailable)
		return
	}
	stream, err := h.svc.ContainerLogFollow(c.Request.Context(),
		c.Query("containerId"), c.Query("tail"))
	if err != nil {
		wsFail(c, err)
		return
	}
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		// gorilla 已写 HTTP 错误响应；订阅即弃（A-T5 同判：连接未建立）。
		stream.Cancel()
		return
	}
	pumpLogFrames(conn, stream)
}

// ComposeLogsFollow WS /docker/compose/logs-follow（admin+readonly）：
// 握手 query {project, tail?}（项目聚合流）。
func (h *Handler) ComposeLogsFollow(c *gin.Context) {
	info := h.svc.ProbeDaemon(c.Request.Context())
	if info.State != dockerx.StateAvailable {
		wsFail(c, errcode.ErrDkDaemonUnavailable)
		return
	}
	stream, err := h.svc.ComposeLogFollow(c.Request.Context(),
		c.Query("project"), c.Query("tail"))
	if err != nil {
		wsFail(c, err)
		return
	}
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		stream.Cancel()
		return
	}
	pumpLogFrames(conn, stream)
}

// pumpLogFrames 帧泵（当前 goroutine 阻塞至连接终结）：行帧→end 帧→关闭；
// 读泵独立 goroutine 仅做断连检测（协议无上行帧，ReadMessage 阻塞至断开/
// 连接关闭即退出——退出路径明确）。行通道优先排空后取 End（End 先于
// close(lines) 投递，避免尾行丢失）。
func pumpLogFrames(conn *websocket.Conn, stream *dockerx.LogStream) {
	defer conn.Close()
	defer stream.Cancel()

	disconnected := make(chan struct{})
	go func() { // 读泵：客户端断连检测（gorilla 连接关闭即退出）
		defer close(disconnected)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
			// 协议无上行帧：任何客户端消息忽略（防御性丢弃）
		}
	}()

	var writeMu sync.Mutex // 串行化帧写（防交错）
	writeFrame := func(f wsFrame) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteJSON(f)
	}
	writeEnd := func(reason string) {
		_ = writeFrame(wsFrame{Type: frameEnd, Reason: reason})
	}

	for {
		// 第一阶段：非阻塞优先排空行通道（End 就绪也不越过未投递行）。
		select {
		case line, ok := <-stream.Lines:
			if !ok {
				writeEnd(awaitEnd(stream))
				return
			}
			if err := writeFrame(wsFrame{Type: frameLog, Line: line}); err != nil {
				return
			}
			continue
		default:
		}
		// 第二阶段：阻塞等待行/终止/断连。
		select {
		case line, ok := <-stream.Lines:
			if !ok {
				writeEnd(awaitEnd(stream))
				return
			}
			if err := writeFrame(wsFrame{Type: frameLog, Line: line}); err != nil {
				return
			}
		case reason := <-stream.End:
			writeEnd(reason)
			return
		case <-disconnected:
			return
		}
	}
}

// awaitEnd 行通道关闭后取终止原因（泵协议保证 End 已投递；防御兜底
// daemon_lost 不虚构缺失值——通道契约 A-D12 下不可达分支）。
func awaitEnd(stream *dockerx.LogStream) string {
	select {
	case reason := <-stream.End:
		return reason
	default:
		return dockerx.ReasonDaemonLost
	}
}

// wsFail 以 HTTP 状态 + 包络输出 WS 握手拒绝（升级前仍是 HTTP 语义；
// 状态映射 A-D13：DK_002→503、AUTH_001/002→401、AUTH_003/008→403、
// 其余 DK_→400；未知错误脱敏 500）。
func wsFail(c *gin.Context, err error) {
	var ec *errcode.Error
	if !errors.As(err, &ec) {
		response.FailWithStatus(c, 500, errcode.ErrInternal)
		return
	}
	response.FailWithStatus(c, wsStatus(ec), ec)
}

// wsStatus WS 握手拒绝状态档（A-D13；response.HTTPStatus 为 terminal 基线，
// 本模块仅覆写 DK_002→503 依赖不可用语义）。
func wsStatus(ec *errcode.Error) int {
	if ec.Symbol() == "DK_002" {
		return http.StatusServiceUnavailable
	}
	return response.HTTPStatus(ec)
}
