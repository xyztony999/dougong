package terminal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"
)

// serve 是会话交互主循环（握手守卫通过 → Attach 后在连接 goroutine 上运行；
// 返回=连接终结，统一清理已由 readLoop/写泵/生命周期侧的 close 触发完成）。
func (s *Session) serve() {
	s.Conn.SetPongHandler(func() {
		// pong 即续期读截止（A-T8：读截止到点=2 次未答等价实现）。
		_ = s.Conn.SetReadDeadline(s.svc.clock().Add(s.svc.limits.PongWait))
	})
	_ = s.Conn.SetReadDeadline(s.svc.clock().Add(s.svc.limits.PongWait))
	s.readLoop()
	// 读循环退出：客户端 close/读错误已触发 close；兜底再触发一次（幂等）。
	s.close(ReasonSelfClose, "read loop exited")
}

// readLoop 逐帧解码与分派（I-06 帧协议完整性守卫）。
func (s *Session) readLoop() {
	for {
		raw, err := s.Conn.ReadFrame()
		if err != nil {
			if wsReadTimeout(err) {
				s.close(ReasonNetworkError, "read deadline exceeded")
			} else {
				// 客户端 close 帧 / 连接层错误：按 self_close 走统一清理。
				s.close(ReasonSelfClose, fmt.Sprintf("read: %v", err))
			}
			return
		}
		if done := s.handleFrame(raw); done {
			return
		}
	}
}

// handleFrame 处理一条客户端帧；返回 true=读循环终止（协议违规断开）。
func (s *Session) handleFrame(raw []byte) bool {
	var f wsClientFrame
	if err := json.Unmarshal(raw, &f); err != nil {
		s.close(ReasonProtocolError, "frame json decode: "+err.Error())
		return true
	}
	switch f.Type {
	case FrameInput:
		data, err := base64.StdEncoding.DecodeString(f.Data)
		if err != nil {
			s.close(ReasonProtocolError, "input base64 decode: "+err.Error())
			return true
		}
		if len(data) > FrameDataLimit { // I-06：单帧解码后 >16KB
			s.close(ReasonProtocolError, "input frame exceeds 16KB limit")
			return true
		}
		if err := s.PTY.Write(data); err != nil {
			// PTY 已死（shell 退出）= self_close 语义。
			s.close(ReasonSelfClose, "pty write: "+err.Error())
			return true
		}
		s.touch(s.svc.clock())
		return false
	case FrameResize:
		if f.Cols == 0 || f.Rows == 0 || f.Cols > ResizeMax || f.Rows > ResizeMax {
			s.close(ReasonProtocolError, "resize out of range") // A-T4
			return true
		}
		if err := s.PTY.Resize(f.Cols, f.Rows); err != nil {
			slog.WarnContext(context.Background(), "terminal resize", "session", s.ID, "err", err)
		}
		s.touch(s.svc.clock())
		return false
	default: // 未知帧类型（含客户端误发服务端帧）=协议违规，I-06
		s.close(ReasonProtocolError, "unknown frame type "+f.Type)
		return true
	}
}

// ptyPump 读 PTY 输出写入环形缓冲（永不阻塞：溢出丢最旧，D-04）。
// 退出路径：Kill() 关闭 master → Read 返回错误。
func (s *Session) ptyPump() {
	buf := make([]byte, 8192)
	for {
		n, err := s.PTY.Read(buf)
		if n > 0 {
			s.ring.write(buf[:n])
		}
		if err != nil {
			// 子进程退出（exit）或 master 关闭：触发统一清理（shell 内 exit
			// 视作操作者主动结束，self_close 语义）。
			s.close(ReasonSelfClose, fmt.Sprintf("pty read: %v", err))
			return
		}
	}
}

// wsPump 排空环形缓冲输出到 WS（D-04：truncated 标记随恢复批次上线）。
// 退出路径：ring.stop()（清理第一步）或写失败（连接侧终结）。
func (s *Session) wsPump() {
	for {
		chunk, truncated := s.ring.take()
		if chunk == nil {
			return // stopped
		}
		frame := wsServerFrame{
			Type:      FrameOutput,
			Data:      base64.StdEncoding.EncodeToString(chunk),
			Truncated: truncated,
		}
		payload, err := json.Marshal(frame)
		if err != nil {
			s.close(ReasonNetworkError, "output encode: "+err.Error())
			return
		}
		if err := s.Conn.WriteFrame(payload); err != nil {
			s.close(ReasonNetworkError, "output write: "+err.Error())
			return
		}
	}
}

// close 是统一清理单点（I-02/I-07；幂等——closeOnce 保证恰好一次完整清理）：
// closed 帧 → Kill PTY 进程组 → 注销会话（释放配额）→ closed 审计。
func (s *Session) close(reason, cause string) {
	s.closeOnce.Do(func() {
		now := s.svc.clock()
		svc := s.svc
		slog.Info("terminal session closed", "session", s.ID, "reason", reason,
			"userId", s.UserID, "duration", now.Sub(s.CreatedAt).String(), "cause", cause)

		// 1) 停输出泵（环形缓冲置停，写泵即刻退出，避免与 closed 帧竞争写序）。
		s.ring.stop()

		// 2) closed 帧（一次性完整反馈：reason/closedAt/durationSeconds，D-05）。
		if s.Conn != nil {
			payload, err := json.Marshal(wsServerFrame{Type: FrameClosed, Data: ClosedPayload{
				Reason:          reason,
				ClosedAt:        formatTS(now),
				DurationSeconds: int64(now.Sub(s.CreatedAt).Seconds()),
			}})
			if err == nil {
				if werr := s.Conn.WriteFrame(payload); werr != nil {
					slog.Warn("terminal closed frame write", "session", s.ID, "err", werr)
				}
			}
			// 3) 关闭 WS 通道。
			if cerr := s.Conn.Close(); cerr != nil {
				slog.Debug("terminal ws close", "session", s.ID, "err", cerr)
			}
		}

		// 4) Kill PTY 进程组 + Wait 收割（孤儿进程回收随清理即时，D-03/I-02）；
		//    会话作用域 ctx 一并取消（exec.CommandContext 双保险）。
		if s.cancelCtx != nil {
			s.cancelCtx()
		}
		if err := s.PTY.Kill(); err != nil && !errors.Is(err, os.ErrClosed) && !errors.Is(err, ErrUnsupportedPlatform) {
			slog.Warn("terminal pty kill", "session", s.ID, "err", err)
		}
		if err := s.PTY.Wait(); err != nil {
			slog.Debug("terminal pty wait", "session", s.ID, "err", err)
		}

		// 5) 注销会话（释放并发配额，I-05；幂等）。
		svc.registry.Remove(s.ID)

		// 6) closed 审计（恰好一条，I-07；失败=既成事实不可回滚 → 运行日志
		//    兜底不阻断清理，D-06）。source 映射：self_close=web、其余=system。
		source := SourceWeb
		if reason != ReasonSelfClose {
			source = SourceSystem
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := svc.audit.Record(ctx, AuditEntry{
			ActorID:    &s.UserID,
			ActorName:  s.Username,
			Category:   CategorySecurity,
			Action:     ActionSessionClosed,
			ObjectType: ObjectTypeTerminalSession,
			ObjectID:   s.ID,
			Detail: map[string]any{
				"reason": reason, "durationSeconds": int64(now.Sub(s.CreatedAt).Seconds()),
			},
			Result: ResultSuccess,
			Source: source,
		}); err != nil {
			slog.Error("terminal closed audit fallback (session terminal_session_closed)",
				"session", s.ID, "reason", reason, "userId", s.UserID, "err", err)
		}
	})
}
