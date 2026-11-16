package audit

import (
	"context"
	"log/slog"
	"time"
)

// Worker 是审计保留期清理任务（每日调度，§4.3.3）：启动立即补跑一次，
// 之后按 interval 周期执行。Run 阻塞直至 ctx 取消——由 main 以 goroutine
// 编排（G-06：唯一退出路径 = ctx 取消），本类型不自起 goroutine。
type Worker struct {
	svc      *Service
	interval time.Duration
}

// NewWorker 构造清理任务；interval 通常为 24h。
func NewWorker(svc *Service, interval time.Duration) *Worker {
	return &Worker{svc: svc, interval: interval}
}

// Run 阻塞执行清理循环；单轮失败记日志并等下轮重试（次日重试语义）。
func (w *Worker) Run(ctx context.Context) error {
	w.purgeOnce(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			w.purgeOnce(ctx)
		}
	}
}

func (w *Worker) purgeOnce(ctx context.Context) {
	// 单轮清理挂上可取消 ctx，避免进程退出时卡在长批次上。
	runCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	deleted, err := w.svc.Purge(runCtx)
	if err != nil {
		slog.ErrorContext(runCtx, "audit purge failed (will retry next cycle)", "deleted", deleted, "err", err)
		return
	}
	if deleted > 0 {
		slog.InfoContext(runCtx, "audit purged", "deleted", deleted)
	}
}
