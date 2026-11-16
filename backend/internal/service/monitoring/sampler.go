package monitoring

import (
	"context"
	"log/slog"
	"math"
	"sort"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

// MetricsSampler 是本机指标采样器（api §4.1.1 领域服务；idea.md §5.3 gopsutil
// 选型，材料给定事实）。逐指标容错（PRD N2）：单指标失败该值 nil + degraded，
// 不中断采集进程；速率类指标维护累计计数基线，差分 / 实际流逝时间（A-M6/A-M7）。
type MetricsSampler struct {
	lastWall     time.Time // 上一轮采样时刻（速率差分基线时钟；零值 = 无基线）
	lastDiskRead uint64    // 磁盘 IO 累计字节基线
	lastDiskWr   uint64
	lastNetUp    uint64 // 网络累计字节基线
	lastNetDown  uint64
	lastProcCPU  map[int32]float64 // pid → 累计 CPU 秒基线（瞬时占比差分依据）
}

// NewMetricsSampler 构造采样器（首轮仅建立基线，速率类指标 nil，A-M7）。
func NewMetricsSampler() *MetricsSampler {
	return &MetricsSampler{lastProcCPU: make(map[int32]float64)}
}

// Sample 执行一轮全量采样（ctx 首参贯穿全部 WithContext 读取）。
func (s *MetricsSampler) Sample(ctx context.Context) Sample {
	nowRaw := time.Now()
	now := nowRaw.UTC().Truncate(time.Second)
	hasBaseline := !s.lastWall.IsZero()
	elapsed := nowRaw.Sub(s.lastWall).Seconds()

	smp := Sample{TS: now}
	degraded := false

	// CPU（A-M5：单次 percpu 调用派生 total，规避库内 lastCPUTimes 状态串扰）。
	if per, ok := safeRead(ctx, "cpu", func() ([]float64, error) {
		return cpu.PercentWithContext(ctx, 0, true)
	}); ok {
		for i := range per {
			per[i] = round1(per[i])
		}
		smp.CPUPerCore = per
		if len(per) > 0 {
			total := round1(mean(per))
			smp.CPUTotal = &total
		}
	} else {
		degraded = true
	}

	// 内存（一次读取派生三值，同生同缺）。
	if m, ok := safeRead(ctx, "memory", func() (*mem.VirtualMemoryStat, error) {
		return mem.VirtualMemoryWithContext(ctx)
	}); ok {
		used, total := m.Used, m.Total
		pct := round1(m.UsedPercent)
		smp.MemUsedBytes, smp.MemTotalBytes, smp.MemUsedPercent = &used, &total, &pct
	} else {
		degraded = true
	}

	// 磁盘容量（A-M8：伪文件系统过滤；单挂载失败该挂载缺席 + degraded）。
	caps, capDegraded := s.sampleDiskCapacity(ctx)
	if caps != nil {
		smp.DiskCapacity = caps
	}
	degraded = degraded || capDegraded

	// 磁盘 IO / 网络速率（全设备合计 ⚠️A13；首轮无基线 → nil 不计 degraded）。
	diskRead, diskWrite, derr := s.sampleDiskIO(ctx, hasBaseline, elapsed)
	smp.DiskIOReadBps, smp.DiskIOWriteBps = diskRead, diskWrite
	degraded = degraded || derr
	netUp, netDown, derr := s.sampleNetIO(ctx, hasBaseline, elapsed)
	smp.NetUpBps, smp.NetDownBps = netUp, netDown
	degraded = degraded || derr

	// 负载。
	if avg, ok := safeRead(ctx, "load", func() (*load.AvgStat, error) {
		return load.AvgWithContext(ctx)
	}); ok {
		l1, l5, l15 := round2(avg.Load1), round2(avg.Load5), round2(avg.Load15)
		smp.Load1, smp.Load5, smp.Load15 = &l1, &l5, &l15
	} else {
		degraded = true
	}

	// TOP 进程（进程维度无部分成功语义：Pids 失败 → ProcessesOK=false → MON_004）。
	procs, procOK, procDegraded := s.sampleProcesses(ctx, hasBaseline, elapsed)
	smp.ProcessesOK = procOK
	if procOK {
		smp.Processes = procs
	}
	degraded = degraded || procDegraded

	smp.Degraded = degraded
	s.lastWall = nowRaw
	return smp
}

// sampleDiskCapacity 读取按挂载点容量清单；返回（清单，是否存在失败）。
func (s *MetricsSampler) sampleDiskCapacity(ctx context.Context) ([]DiskCapacityItem, bool) {
	parts, ok := safeRead(ctx, "disk.partitions", func() ([]disk.PartitionStat, error) {
		return disk.PartitionsWithContext(ctx, false)
	})
	if !ok {
		return nil, true
	}
	items := make([]DiskCapacityItem, 0, len(parts))
	degraded := false
	for _, p := range parts {
		if pseudoFilesystems[p.Fstype] {
			continue
		}
		u, uok := safeRead(ctx, "disk.usage", func() (*disk.UsageStat, error) {
			return disk.UsageWithContext(ctx, p.Mountpoint)
		})
		if !uok {
			degraded = true
			continue
		}
		items = append(items, DiskCapacityItem{Mount: p.Mountpoint, TotalBytes: u.Total, UsedBytes: u.Used})
	}
	return items, degraded
}

// sampleDiskIO 读取全设备合计磁盘 IO 速率（差分基线；计数器回绕 → 本轮 nil 并
// 重同步基线）。
func (s *MetricsSampler) sampleDiskIO(ctx context.Context, hasBaseline bool, elapsed float64) (*int64, *int64, bool) {
	stats, ok := safeRead(ctx, "disk.io", func() (map[string]disk.IOCountersStat, error) {
		return disk.IOCountersWithContext(ctx)
	})
	if !ok {
		return nil, nil, true
	}
	var totalRead, totalWrite uint64
	for _, st := range stats {
		totalRead += st.ReadBytes
		totalWrite += st.WriteBytes
	}
	read := diffRate(s.lastDiskRead, totalRead, hasBaseline, elapsed)
	write := diffRate(s.lastDiskWr, totalWrite, hasBaseline, elapsed)
	s.lastDiskRead, s.lastDiskWr = totalRead, totalWrite
	return read, write, false
}

// sampleNetIO 读取全设备合计网络上下行速率。
func (s *MetricsSampler) sampleNetIO(ctx context.Context, hasBaseline bool, elapsed float64) (*int64, *int64, bool) {
	counters, ok := safeRead(ctx, "net.io", func() ([]gnet.IOCountersStat, error) {
		return gnet.IOCountersWithContext(ctx, false)
	})
	if !ok {
		return nil, nil, true
	}
	if len(counters) == 0 {
		return nil, nil, false
	}
	up, down := counters[0].BytesSent, counters[0].BytesRecv
	upBps := diffRate(s.lastNetUp, up, hasBaseline, elapsed)
	downBps := diffRate(s.lastNetDown, down, hasBaseline, elapsed)
	s.lastNetUp, s.lastNetDown = up, down
	return upBps, downBps, false
}

// sampleProcesses 枚举进程并计算瞬时 CPU 占比（A-M6：pid→累计 CPU 秒基线差分
// / 流逝时间；新进程首轮无基线 → cpuPercent=nil）。返回（清单≤TopN 降序，
// Pids 是否成功，是否存在单项失败）。
func (s *MetricsSampler) sampleProcesses(ctx context.Context, hasBaseline bool, elapsed float64) ([]ProcessInfo, bool, bool) {
	pids, ok := safeRead(ctx, "process.pids", func() ([]int32, error) {
		return process.PidsWithContext(ctx)
	})
	if !ok {
		return nil, false, true
	}
	next := make(map[int32]float64, len(pids))
	list := make([]ProcessInfo, 0, len(pids))
	degraded := false
	for _, pid := range pids {
		p, perr := process.NewProcessWithContext(ctx, pid)
		if perr != nil {
			degraded = true
			continue
		}
		item := ProcessInfo{PID: pid}
		if name, nok := safeRead(ctx, "process.name", func() (string, error) {
			return p.NameWithContext(ctx)
		}); nok {
			item.Name = name
		} else {
			degraded = true
		}
		if times, tok := safeRead(ctx, "process.times", func() (*cpu.TimesStat, error) {
			return p.TimesWithContext(ctx)
		}); tok {
			total := times.Total()
			if hasBaseline && elapsed > 0 {
				if prev, seen := s.lastProcCPU[pid]; seen && total >= prev {
					pct := round1((total - prev) / elapsed * 100)
					item.CPUPercent = &pct
				}
			}
			next[pid] = total
		} else {
			degraded = true
		}
		if mi, mok := safeRead(ctx, "process.mem", func() (*process.MemoryInfoStat, error) {
			return p.MemoryInfoWithContext(ctx)
		}); mok {
			rss := mi.RSS
			item.MemBytes = &rss
		} else {
			degraded = true
		}
		list = append(list, item)
	}
	s.lastProcCPU = next

	// I-06：cpuPercent 降序（nil 排尾——新进程无基线），同值按 pid 升序稳定。
	sort.SliceStable(list, func(i, j int) bool {
		vi, vj := list[i].CPUPercent, list[j].CPUPercent
		switch {
		case vi != nil && vj != nil:
			if *vi != *vj {
				return *vi > *vj
			}
		case vi != nil:
			return true
		case vj != nil:
			return false
		}
		return list[i].PID < list[j].PID
	})
	if len(list) > TopN {
		list = list[:TopN]
	}
	return list, true, degraded
}

// pseudoFilesystems 是容量统计剔除的伪/虚拟文件系统集合（A-M8：正确性过滤
// 非设备数截断——D-05「不虚构截断」针对真实设备）。
var pseudoFilesystems = map[string]bool{
	"proc": true, "procfs": true, "sysfs": true, "tmpfs": true, "devtmpfs": true,
	"devfs": true, "mqueue": true, "cgroup": true, "cgroup2": true, "squashfs": true,
	"iso9660": true, "overlay": true, "aufs": true, "nsfs": true, "rpc_pipefs": true,
	"hugetlbfs": true, "fusectl": true, "tracefs": true, "debugfs": true, "pstore": true,
	"securityfs": true, "configfs": true, "bpf": true, "autofs": true, "binfmt_misc": true,
}

// safeRead 执行单指标读取并兜底 panic（采集容错，PRD N2）；失败返回 ok=false
// 并记结构化日志（含指标名），不中断调用方。
func safeRead[T any](ctx context.Context, metric string, fn func() (T, error)) (v T, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			slog.ErrorContext(ctx, "monitoring sampler read panic", "metric", metric, "panic", r)
			ok = false
		}
	}()
	val, err := fn()
	if err != nil {
		slog.WarnContext(ctx, "monitoring sampler read failed", "metric", metric, "err", err)
		var zero T
		return zero, false
	}
	return val, true
}

// diffRate 由累计计数差分速率（bytes/s 整数，D-02）；无基线/时间无效/计数器
// 回绕 → nil（不虚构，D-08）。
func diffRate(prev, cur uint64, hasBaseline bool, elapsed float64) *int64 {
	if !hasBaseline || elapsed <= 0 || cur < prev {
		return nil
	}
	v := int64(math.Round(float64(cur-prev) / elapsed))
	return &v
}

func mean(vs []float64) float64 {
	if len(vs) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vs {
		sum += v
	}
	return sum / float64(len(vs))
}

// round1 百分比 1 位小数（D-02）。
func round1(v float64) float64 { return math.Round(v*10) / 10 }

// round2 负载 2 位小数（D-02）。
func round2(v float64) float64 { return math.Round(v*100) / 100 }
