package monitoring

import (
	"context"
	"log/slog"
	"time"
)

// Collector 是后台采集链路（api §3.2：3s 节拍 ⚠️A12，容错 PRD N2）：采样 →
// 覆写实时层槽位 → 提交分钟聚合器。由 main 统一编排（goroutine 退出路径 G-06：
// ctx 取消即返回）；单轮 panic 隔离记日志，不击穿进程（backend-03 §5）。
type Collector struct {
	sampler *MetricsSampler
	agg     *MinuteAggregator
	buf     *MetricsBuffer
}

// NewCollector 构造采集链路（写入目标服务的缓冲）。
func NewCollector(svc *Service) *Collector {
	return &Collector{
		sampler: NewMetricsSampler(),
		agg:     NewMinuteAggregator(svc.buf),
		buf:     svc.buf,
	}
}

// Run 阻塞运行采集循环：启动即执行首轮采样（快照就绪性尽早满足，MON_004 仅
// 存在于启动后首轮完成前），此后按 3s ticker 节拍；ctx 取消返回 nil（优雅退出）。
func (c *Collector) Run(ctx context.Context) error {
	c.tick(ctx)
	ticker := time.NewTicker(SampleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			c.tick(ctx)
		}
	}
}

// tick 执行单轮采集（panic 隔离：单轮失败只损失该轮数据，D-08）。
func (c *Collector) tick(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			slog.ErrorContext(ctx, "monitoring collector tick panic", "panic", r)
		}
	}()
	s := c.sampler.Sample(ctx)
	c.buf.SetLatest(s)
	c.agg.Offer(s)
}
