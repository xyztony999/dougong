package monitoring

import (
	"fmt"
	"sort"
	"time"
)

// Series 是缓冲访问契约的窗口投影（api §4.1.5）：只读，窗口 × 维度 × 口径
// （D-03/D-04）。返回（series, startTime, partial）：
//   - series：无数据时为空切片（D-07，code=0 空序列非错误流）
//   - startTime：全序列最早非 null 点时间；无有效点为 nil
//   - partial：任一请求序列点存在 v=null（含窗口尾部未聚合/前部未覆盖，A-M10）
//
// 每个窗口分钟槽位无论有无数据均出点（点数恒 = 窗口分钟数，A-M4）。
func (b *MetricsBuffer) Series(win Window, dim Dimension, scope Scope) ([]Series, *time.Time, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	slots, ok := b.seriesSlotsLocked(win)
	if !ok {
		return []Series{}, nil, false
	}
	entries := make([]*MinuteEntry, len(slots))
	for i, s := range slots {
		entries[i] = b.entryAtLocked(s)
	}

	var series []Series
	switch dim {
	case DimCPU:
		series = cpuSeries(entries, slots, scope)
	case DimMemory:
		series = []Series{scalarSeries(entries, slots, keyMemoryUsed, "内存使用率", UnitPercent,
			func(e *MinuteEntry) *float64 { return e.MemUsedPercent })}
	case DimDisk:
		series = diskSeries(entries, slots)
	case DimNetwork:
		series = []Series{
			intSeries(entries, slots, keyNetUp, "上行速率", UnitBps, func(e *MinuteEntry) *int64 { return e.NetUpBps }),
			intSeries(entries, slots, keyNetDown, "下行速率", UnitBps, func(e *MinuteEntry) *int64 { return e.NetDownBps }),
		}
	case DimLoad:
		series = []Series{
			scalarSeries(entries, slots, keyLoad1m, "负载 1 分钟", UnitValue, func(e *MinuteEntry) *float64 { return e.Load1 }),
			scalarSeries(entries, slots, keyLoad5m, "负载 5 分钟", UnitValue, func(e *MinuteEntry) *float64 { return e.Load5 }),
			scalarSeries(entries, slots, keyLoad15m, "负载 15 分钟", UnitValue, func(e *MinuteEntry) *float64 { return e.Load15 }),
		}
	}

	var start *time.Time
	partial := false
	for si := range series {
		for _, p := range series[si].Points {
			if p.V == nil {
				partial = true
				continue
			}
			if start == nil || p.TS.Before(*start) {
				t := p.TS
				start = &t
			}
		}
	}
	return series, start, partial
}

// cpuSeries 构建 cpu 维度序列：total → 1 条（D-04）；per_core → 每核 1 条
// （序列数 = 窗口内条目分核长度最大值，A-M9）。
func cpuSeries(entries []*MinuteEntry, slots []time.Time, scope Scope) []Series {
	if scope == ScopePerCore {
		cores := 0
		for _, e := range entries {
			if e != nil && len(e.CPUPerCore) > cores {
				cores = len(e.CPUPerCore)
			}
		}
		if cores == 0 {
			return []Series{}
		}
		out := make([]Series, cores)
		for i := 0; i < cores; i++ {
			idx := i
			out[i] = scalarSeries(entries, slots,
				fmt.Sprintf(keyCPUCoreFmt, idx), fmt.Sprintf("核心 %d", idx), UnitPercent,
				func(e *MinuteEntry) *float64 {
					if len(e.CPUPerCore) > idx {
						v := e.CPUPerCore[idx]
						return &v
					}
					return nil
				})
		}
		return out
	}
	return []Series{scalarSeries(entries, slots, keyCPUTotal, "CPU 使用率", UnitPercent,
		func(e *MinuteEntry) *float64 { return e.CPUTotal })}
}

// diskSeries 构建磁盘维度序列：每挂载点 1 条容量序列（key=disk_capacity:{mount}，
// v=usedBytes）+ 2 条全设备合计 IO 速率序列（api 4.1.4）。挂载集 = 窗口内条目
// 并集、按 mount 字典序稳定排序（A-M9，如实记录卸载历史）。
func diskSeries(entries []*MinuteEntry, slots []time.Time) []Series {
	mounts := map[string]bool{}
	for _, e := range entries {
		if e == nil {
			continue
		}
		for _, c := range e.DiskCapacity {
			mounts[c.Mount] = true
		}
	}
	sorted := make([]string, 0, len(mounts))
	for m := range mounts {
		sorted = append(sorted, m)
	}
	sort.Strings(sorted)

	out := make([]Series, 0, len(sorted)+2)
	for _, m := range sorted {
		mount := m
		out = append(out, scalarSeries(entries, slots,
			fmt.Sprintf(keyDiskCapFmt, mount), "磁盘 "+mount, UnitBytes,
			func(e *MinuteEntry) *float64 {
				for _, c := range e.DiskCapacity {
					if c.Mount == mount {
						v := float64(c.UsedBytes)
						return &v
					}
				}
				return nil
			}))
	}
	out = append(out,
		intSeries(entries, slots, keyDiskIORead, "磁盘读取速率", UnitBps, func(e *MinuteEntry) *int64 { return e.DiskIOReadBps }),
		intSeries(entries, slots, keyDiskIOWrite, "磁盘写入速率", UnitBps, func(e *MinuteEntry) *int64 { return e.DiskIOWriteBps }))
	return out
}

// scalarSeries 由 *float64 提取器构建单条序列（缺失槽位 v=null）。
func scalarSeries(entries []*MinuteEntry, slots []time.Time, key, label, unit string, get func(*MinuteEntry) *float64) Series {
	pts := make([]Point, len(slots))
	for i, e := range entries {
		pts[i] = Point{TS: slots[i]}
		if e != nil {
			pts[i].V = get(e)
		}
	}
	return Series{Key: key, Label: label, Unit: unit, Points: pts}
}

// intSeries 由 *int64 提取器构建单条序列（bps 等，转 float64 承载——容量上限
// 远低于 2^53 精度边界）。
func intSeries(entries []*MinuteEntry, slots []time.Time, key, label, unit string, get func(*MinuteEntry) *int64) Series {
	return scalarSeries(entries, slots, key, label, unit, func(e *MinuteEntry) *float64 {
		if v := get(e); v != nil {
			f := float64(*v)
			return &f
		}
		return nil
	})
}
