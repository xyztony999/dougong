package monitoring

import (
	"sync"
	"time"
)

// MetricsBuffer 是监控指标进程内承载（api §4.1.5 缓冲访问契约，零持久化）：
// 双层组织——实时层最新 3s 采样单槽位（Latest，快照/TOP 共源一次内存读）+
// 历史层 1440 条环形缓冲（AppendMinute 滚动淘汰）。
//
// 并发策略（I-03）：单写者——SetLatest/AppendMinute 仅由采集链路（Collector
// goroutine）调用；读取并发不限，RWMutex 保护。条目写入后不可变（I-01），
// 重启即空（I-04，进程生命周期 = 数据生命周期）。
type MetricsBuffer struct {
	mu         sync.RWMutex
	ring       [BufferCapacity]*MinuteEntry
	latest     *Sample
	lastMinute time.Time        // 最后落缓冲分钟；零值 = 尚无任何数据
	now        func() time.Time // 时钟注入（测试确定性）
}

// NewMetricsBuffer 构造空缓冲。
func NewMetricsBuffer() *MetricsBuffer { return &MetricsBuffer{now: time.Now} }

// SetLatest 覆写实时层最新采样槽位（仅采集链路调用，单写者）。
func (b *MetricsBuffer) SetLatest(s Sample) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.latest = &s
}

// Latest 读取实时层最新采样（ok=false：启动后首轮采样未完成，MON_004 判据）。
func (b *MetricsBuffer) Latest() (Sample, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.latest == nil {
		return Sample{}, false
	}
	return *b.latest, true
}

// AppendMinute 追加分钟聚合条目（单写者；I-02 淘汰语义由环形下标覆盖最旧槽位
// 保证——满后写入必伴随最旧条目淘汰）。
func (b *MetricsBuffer) AppendMinute(e MinuteEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	e.DiskCapacity = cloneCapacity(e.DiskCapacity)
	e.TopProcesses = cloneProcesses(e.TopProcesses)
	e.CPUPerCore = cloneFloats(e.CPUPerCore)
	b.ring[minuteIndex(e.TS)] = &e
	if e.TS.After(b.lastMinute) {
		b.lastMinute = e.TS
	}
}

// minuteIndex 由分钟锚点推导环形下标：连续 1440 个分钟互不碰撞（窗口上限即
// 1440，无同槽共存冲突；更旧残留由 entryAt 的 TS 校验淘汰）。
func minuteIndex(minute time.Time) int {
	return int((minute.Unix() / 60) % BufferCapacity)
}

// entryAtLocked 读取指定分钟的条目；槽位残留（>24h 前旧条目）以 TS 不等判无数据。
// 调用方须持有读锁。
func (b *MetricsBuffer) entryAtLocked(minute time.Time) *MinuteEntry {
	e := b.ring[minuteIndex(minute)]
	if e == nil || !e.TS.Equal(minute) {
		return nil
	}
	return e
}

// seriesSlotsLocked 计算窗口分钟槽位序列（升序）。锚点 = 墙上时钟上一分钟
// （已完成分钟——当前未完成分钟仅存在于实时层，A-M3）；窗口尾部尚未聚合的
// 分钟以 v=null 表达（D-03），partial 如实置位。ok=false：缓冲无任何数据（D-07）。
// 调用方须持有读锁。
func (b *MetricsBuffer) seriesSlotsLocked(win Window) ([]time.Time, bool) {
	if b.lastMinute.IsZero() {
		return nil, false
	}
	anchor := b.now().UTC().Truncate(time.Minute).Add(-time.Minute)
	n := win.Minutes()
	slots := make([]time.Time, n)
	for i := range slots {
		slots[i] = anchor.Add(-time.Duration(n-1-i) * time.Minute)
	}
	return slots, true
}

// cloneCapacity 深拷贝挂载点切片（写入侧隔离，读取方不持有内部引用）。
func cloneCapacity(in []DiskCapacityItem) []DiskCapacityItem {
	if in == nil {
		return nil
	}
	out := make([]DiskCapacityItem, len(in))
	copy(out, in)
	return out
}

// cloneProcesses 深拷贝进程清单（同上）。
func cloneProcesses(in []ProcessInfo) []ProcessInfo {
	if in == nil {
		return nil
	}
	out := make([]ProcessInfo, len(in))
	copy(out, in)
	return out
}

// cloneFloats 深拷贝浮点切片（同上）。
func cloneFloats(in []float64) []float64 {
	if in == nil {
		return nil
	}
	out := make([]float64, len(in))
	copy(out, in)
	return out
}
