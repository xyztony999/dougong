// logs.go 承载日志双通道（D-03）：尾部读取（GetContainerLogs，REST 一次性）
// 与实时跟随（FollowContainerLogs，WS 流订阅）+ 容器状态映射（BA 附录B.1）。
// 跟随流为 Follow+Tail 单流承载（A-D16：daemon 原生回放尾部再跟随，无间隙
// 丢行）；多路复用流经 stdcopy 解复用，TTY 容器（无 mux 头）经 peek 头判定
// 后直读。
package dockerx

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/errdefs"
	"github.com/docker/docker/pkg/stdcopy"
)

// WS end 帧终止原因（D-03 帧载荷常量，非枚举池条目，不入审计）。
const (
	ReasonContainerRemoved = "container_removed"
	ReasonContainerStopped = "container_stopped"
	ReasonProjectGone      = "project_gone"
	ReasonLocateFailed     = "locate_failed"
	ReasonDaemonLost       = "daemon_lost"
)

// LogStream 是日志跟随订阅句柄（D-03 流协议数据源）：Lines 承载回放+跟随
// 行（服务端逐帧 {"type":"log","line"}）；End 承载终止原因（缓冲 1 值，
// 泵收束时投递）；Cancel 取消订阅（客户端断开即调——泵 goroutine 由此
// 收敛退出，生命周期归属=本句柄持有者，A-T7 连接作用域同判）。
type LogStream struct {
	Lines  <-chan string
	End    <-chan string
	Cancel context.CancelFunc
}

// MapState 将 daemon 原生状态映射为 container_status（BA 附录B.1）：
// created/exited/dead→stopped 归并；running/paused 原值；restarting/
// removing 过渡态透传原始值（supportedActions 为空数组，D-02）。
func MapState(raw string) string {
	switch raw {
	case "running", "paused", "restarting", "removing":
		return raw
	default:
		// created / exited / dead / 未知值 → stopped（BA 附录B.1 归并族）。
		return StatusStopped
	}
}

// tailStr 组装 LogsOptions.Tail（tail<0 视为 all；tail=0 抑制回放——项目
// 级跟随重订阅场景只取新增行；REST/容器级跟随 tail 恒 ≥1 由服务层校验）。
func tailStr(tail int) string {
	if tail < 0 {
		return "all"
	}
	return strconv.Itoa(tail)
}

// readLogLines 读取整个日志流并按行拆分（stdout/stderr 解复用合并收集——
// 契约 lines 为聚合行数组不分流）。
func readLogLines(rc io.Reader) ([]string, error) {
	var lines []string
	w := newCollector(func(line string) { lines = append(lines, line) })
	err := copyLogStream(w, rc)
	w.flushTail()
	if err != nil && !isStreamEnd(err) {
		return nil, &ExecError{Verb: "container logs", Reason: err.Error()}
	}
	return lines, nil
}

// copyLogStream 将日志流按 mux 头判定复制到 w（多路复用经 stdcopy，TTY
// 直读；peek 判定不消费字节——bufio.Reader 转交后续读取）。
func copyLogStream(w io.Writer, rc io.Reader) error {
	br := bufio.NewReader(rc)
	if isMultiplexed(br) {
		_, err := stdcopy.StdCopy(w, w, br)
		return err
	}
	_, err := io.Copy(w, br)
	return err
}

// isMultiplexed peek 8 字节判定多路复用头（STREAM_TYPE ∈ {1,2} 且 size
// 前三字节为 0；TTY 流内容首字节几乎必非 1/2 或后续字节非 0）。
func isMultiplexed(br *bufio.Reader) bool {
	head, err := br.Peek(8)
	if err != nil && len(head) < 8 {
		return false // 短于头的流必为直读
	}
	return (head[0] == 1 || head[0] == 2) && head[1] == 0 && head[2] == 0 && head[3] == 0
}

// isStreamEnd 判定流正常收束（EOF 家族；hijacked 连接关闭常以 closed
// 语义呈现——跟随流被 daemon 收束/取消属正常终态）。
func isStreamEnd(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "closed") || strings.Contains(msg, "EOF")
}

// lineCollector 按行拆分的 io.Writer（\r\n 与 \n 均收；行经 emitTo 投递）。
type lineCollector struct {
	emitTo func(line string)
	buf    []byte
}

// newCollector 构造（REST 场景 emitTo 追加切片；跟随场景投递 channel）。
func newCollector(emitTo func(string)) *lineCollector {
	return &lineCollector{emitTo: emitTo}
}

// Write 实现 io.Writer（积累+拆行投递）。
func (w *lineCollector) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		w.emit(strings.TrimSuffix(string(w.buf[:i]), "\r"))
		w.buf = w.buf[i+1:]
	}
	return len(p), nil
}

// flushTail 收束尾行（流结束时无换行符的残留片段）。
func (w *lineCollector) flushTail() {
	if len(w.buf) > 0 {
		w.emit(strings.TrimSuffix(string(w.buf), "\r"))
		w.buf = nil
	}
}

// emit 投递一行（空行如实投递——daemon 实况，不虚构不丢行）。
func (w *lineCollector) emit(line string) {
	if w.emitTo != nil {
		w.emitTo(line)
	}
}

// FollowContainerLogs 建立跟随订阅（D-03/A-D16：Follow+Tail 单流，先回放
// 尾部再跟随）。终止归因（A-D12）：流结束后 inspect 复核——NotFound→
// container_removed；!Running→container_stopped；其余→daemon_lost。
func (g *Gateway) FollowContainerLogs(ctx context.Context, id string, tail int) (*LogStream, error) {
	sctx, cancel := context.WithCancel(ctx)
	rc, err := g.cli.ContainerLogs(sctx, id, container.LogsOptions{
		ShowStdout: true, ShowStderr: true, Follow: true, Tail: tailStr(tail),
	})
	if err != nil {
		cancel()
		return nil, sdkError("container logs follow", err)
	}
	lines := make(chan string, 256)
	end := make(chan string, 1)
	go func() {
		defer rc.Close()
		w := newCollector(func(line string) {
			select {
			case lines <- line:
			case <-sctx.Done(): // 客户端已断开，行无处投递即弃
			}
		})
		copyErr := copyLogStream(w, rc)
		w.flushTail()
		if sctx.Err() != nil {
			return // 订阅取消：静默收敛，不发 End
		}
		if copyErr != nil && !isStreamEnd(copyErr) {
			end <- ReasonDaemonLost
			close(lines)
			return
		}
		end <- g.resolveEndReason(sctx, id)
		close(lines)
	}()
	return &LogStream{Lines: lines, End: end, Cancel: cancel}, nil
}

// resolveEndReason 跟随流结束归因（A-D12：inspect 复核实况）。
func (g *Gateway) resolveEndReason(ctx context.Context, id string) string {
	cctx, cancel := withTimeout(ctx, SDKTimeout)
	defer cancel()
	insp, err := g.cli.ContainerInspect(cctx, id)
	switch {
	case err != nil && errdefs.IsNotFound(err):
		return ReasonContainerRemoved
	case err != nil:
		return ReasonDaemonLost
	case !insp.State.Running:
		return ReasonContainerStopped
	default:
		// 容器仍在运行而流已断：daemon 侧中断（重启/失联语义）。
		return ReasonDaemonLost
	}
}
