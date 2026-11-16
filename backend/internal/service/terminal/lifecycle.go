package terminal

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/dougong/dougong/internal/errcode"
)

// RunLifecycle 是 TerminalLifecycleService 后台扫描（I-03/D-03/D-07）：
// tick 粒度遍历存续会话——空闲判定（lastActiveAt+IdleTimeout，仅输入刷新）/
// 硬上限判定（createdAt+HardLimit，与活跃无关）/ 面板会话 30s 复核
// （VerifyWs 失效即联动断开）/ 心跳 30s ping。goroutine 由 main 统一编排
// （ctx 取消退出——G-06）；tick 生产取 ScanTick（10s），测试注入小粒度。
func (s *Service) RunLifecycle(ctx context.Context, tick time.Duration) error {
	if tick <= 0 {
		tick = ScanTick
	}
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			s.scanOnce(ctx)
		}
	}
}

// scanOnce 单轮扫描：任一阈值先到即断（I-03 双阈值独立生效）；断开后
// registry.Remove 已由统一清理完成，本轮后续判定对该会话自然跳过
// （close 幂等，I-02）。
func (s *Service) scanOnce(ctx context.Context) {
	now := s.clock()
	s.registry.Scan(func(sess *Session) {
		// I-03 空闲：仅输入刷新的 lastActiveAt 判定（D1 从严口径）。
		if now.Sub(sess.activityAt()) >= s.limits.IdleTimeout {
			sess.close(ReasonIdleTimeout, "lifecycle idle timeout")
			return
		}
		// I-03 硬上限：createdAt+8h，与活跃无关。
		if !now.Before(sess.HardDeadlineAt) {
			sess.close(ReasonHardLimit, "lifecycle hard limit")
			return
		}
		// D-07 面板会话复核：VerifyWs 只读；失效（AUTH_001/002）即联动断开；
		// 存储异常非会话失效——不联动（如实日志，等下轮复核）。
		if sess.dueRecheck(now, s.limits.RecheckInterval) {
			if _, _, err := s.verifier.VerifyWs(ctx, sess.PanelSessionRef); err != nil {
				var ec *errcode.Error
				if errors.As(err, &ec) &&
					(ec.Symbol() == "AUTH_001" || ec.Symbol() == "AUTH_002") {
					sess.close(ReasonPanelSessionInvalid, "panel session invalid: "+ec.Symbol())
					return
				}
				slog.WarnContext(ctx, "terminal lifecycle: verify panel session",
					"session", sess.ID, "err", err)
			}
		}
		// D-03 心跳：30s ping（pong 续期读截止在 serve/PongHandler；失联经
		// 读截止超时判 network_error，A-T8）。ping 写失败=连接已死。
		if sess.duePing(now, s.limits.PingInterval) {
			if err := sess.Conn.Ping(); err != nil {
				sess.close(ReasonNetworkError, "lifecycle ping: "+err.Error())
			}
		}
	})
}
