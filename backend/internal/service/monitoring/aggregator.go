package monitoring

import (
	"math"
	"time"
)

// MinuteAggregator 是 3s→1min 聚合器（api §4.1.1 领域服务）：跨分钟首个采样
// 到达时终结算上一分钟并 AppendMinute（A-M3：只追加、条目不可变 I-01；当前
// 未完成分钟仅驻留实时层）。聚合口径 A-M2：速率/使用率类 = 算术平均，容量类
// 与 TOP = 该分钟最后一次成功值；count=0 → nil（I-05）。
//
// 仅由采集链路（Collector）单线程调用（I-03），无锁。
type MinuteAggregator struct {
	buf *MetricsBuffer
	acc *minuteAccumulator
}

// NewMinuteAggregator 构造聚合器（写入目标缓冲）。
func NewMinuteAggregator(buf *MetricsBuffer) *MinuteAggregator {
	return &MinuteAggregator{buf: buf}
}

// Offer 提交一轮采样：归属分钟未变则累计；跨分钟则先终结算上一分钟入缓冲。
// 采样时间戳视为单调（采集链路同 goroutine 顺序提交；偶发时钟回拨样本并入
// 当前累计桶，不影响后续分钟对齐）。
func (a *MinuteAggregator) Offer(s Sample) {
	minute := s.TS.Truncate(time.Minute)
	if a.acc == nil {
		a.acc = newMinuteAccumulator(minute)
	} else if !a.acc.minute.Equal(minute) {
		a.buf.AppendMinute(a.acc.entry())
		a.acc = newMinuteAccumulator(minute)
	}
	a.acc.offer(s)
}

// minuteAccumulator 是单个分钟窗口的聚合累加器。
type minuteAccumulator struct {
	minute time.Time

	cpuSum       float64
	cpuCount     int
	perCoreSum   []float64
	perCoreCount []int

	memUsedSum  float64
	memTotalSum float64
	memPctSum   float64
	memCount    int

	diskReadSum  float64
	diskWriteSum float64
	diskIOCount  int

	netUpSum   float64
	netDownSum float64
	netCount   int

	load1Sum  float64
	load5Sum  float64
	load15Sum float64
	loadCount int

	capacity []DiskCapacityItem // 最后一次成功值
	top      []ProcessInfo      // 最后一次成功值
	degraded bool
}

func newMinuteAccumulator(minute time.Time) *minuteAccumulator {
	return &minuteAccumulator{minute: minute}
}

// offer 累加单轮采样（nil 指针 = 该指标本轮缺失，不计数）。
func (a *minuteAccumulator) offer(s Sample) {
	if s.CPUTotal != nil {
		a.cpuSum += *s.CPUTotal
		a.cpuCount++
	}
	for i, v := range s.CPUPerCore {
		for len(a.perCoreSum) <= i {
			a.perCoreSum = append(a.perCoreSum, 0)
			a.perCoreCount = append(a.perCoreCount, 0)
		}
		a.perCoreSum[i] += v
		a.perCoreCount[i]++
	}
	if s.MemUsedBytes != nil {
		a.memUsedSum += float64(*s.MemUsedBytes)
		a.memTotalSum += float64(*s.MemTotalBytes)
		a.memPctSum += *s.MemUsedPercent
		a.memCount++
	}
	if s.DiskIOReadBps != nil {
		a.diskReadSum += float64(*s.DiskIOReadBps)
		a.diskWriteSum += float64(*s.DiskIOWriteBps)
		a.diskIOCount++
	}
	if s.NetUpBps != nil {
		a.netUpSum += float64(*s.NetUpBps)
		a.netDownSum += float64(*s.NetDownBps)
		a.netCount++
	}
	if s.Load1 != nil {
		a.load1Sum += *s.Load1
		a.load5Sum += *s.Load5
		a.load15Sum += *s.Load15
		a.loadCount++
	}
	if s.DiskCapacity != nil {
		a.capacity = s.DiskCapacity
	}
	if s.ProcessesOK {
		a.top = s.Processes
	}
	if s.Degraded {
		a.degraded = true
	}
}

// entry 终结算产出分钟条目（count=0 指标保持 nil → 趋势 v=null，I-05）。
func (a *minuteAccumulator) entry() MinuteEntry {
	e := MinuteEntry{
		TS:           a.minute,
		DiskCapacity: a.capacity,
		TopProcesses: a.top,
		Degraded:     a.degraded,
	}
	if a.cpuCount > 0 {
		v := round1(a.cpuSum / float64(a.cpuCount))
		e.CPUTotal = &v
	}
	if len(a.perCoreCount) > 0 {
		cores := make([]float64, len(a.perCoreCount))
		has := false
		for i, c := range a.perCoreCount {
			if c > 0 {
				cores[i] = round1(a.perCoreSum[i] / float64(c))
				has = true
			}
		}
		if has {
			e.CPUPerCore = cores
		}
	}
	if a.memCount > 0 {
		n := float64(a.memCount)
		used := uint64(math.Round(a.memUsedSum / n))
		total := uint64(math.Round(a.memTotalSum / n))
		pct := round1(a.memPctSum / n)
		e.MemUsedBytes, e.MemTotalBytes, e.MemUsedPercent = &used, &total, &pct
	}
	if a.diskIOCount > 0 {
		n := float64(a.diskIOCount)
		read := int64(math.Round(a.diskReadSum / n))
		write := int64(math.Round(a.diskWriteSum / n))
		e.DiskIOReadBps, e.DiskIOWriteBps = &read, &write
	}
	if a.netCount > 0 {
		n := float64(a.netCount)
		up := int64(math.Round(a.netUpSum / n))
		down := int64(math.Round(a.netDownSum / n))
		e.NetUpBps, e.NetDownBps = &up, &down
	}
	if a.loadCount > 0 {
		n := float64(a.loadCount)
		l1 := round2(a.load1Sum / n)
		l5 := round2(a.load5Sum / n)
		l15 := round2(a.load15Sum / n)
		e.Load1, e.Load5, e.Load15 = &l1, &l5, &l15
	}
	return e
}
