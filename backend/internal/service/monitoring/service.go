package monitoring

import (
	"context"

	"github.com/dougong/dougong/internal/errcode"
)

// Service 是系统监控只读查询门面（api §4.1.4/§4.2.4 四接口的业务承载）：
// 快照/TOP 同源于实时层最新槽位（一次内存读），history 只读环形缓冲投影；
// 参数枚举校验统一收敛于此（ParamsGuard 职责，MON_001~003 映射，R-14）。
type Service struct {
	buf *MetricsBuffer
}

// NewService 构造监控服务（缓冲随服务创建；采集链路由 main 经 NewCollector
// 单独编排启动）。
func NewService() *Service { return &Service{buf: NewMetricsBuffer()} }

// Buffer 暴露进程内缓冲（api §4.1.5 MetricsBuffer 契约）：采集链路单写者写入，
// 查询侧只读；测试经此注入确定性数据。
func (s *Service) Buffer() *MetricsBuffer { return s.buf }

// Snapshot 返回实时快照（GET /monitoring/metrics/snapshot）：启动后首轮采样
// 未完成 → MON_004；单指标失败字段 null + degraded=true（D-08）。
func (s *Service) Snapshot(ctx context.Context) (*SnapshotData, error) {
	smp, ok := s.buf.Latest()
	if !ok {
		return nil, errcode.ErrMonitoringNoData
	}
	return snapshotFromSample(&smp), nil
}

// History 返回趋势序列（GET /monitoring/metrics/history）：窗口 × 维度 × 口径
// 校验（MON_001~003）后读缓冲投影；窗口内无任何数据返回空 series + startTime
// null + code=0（D-07，非错误流）。
func (s *Service) History(ctx context.Context, window, dimension, scope string) (*HistoryData, error) {
	if !validWindow(window) {
		return nil, errcode.ErrMonitoringInvalidWindow
	}
	if !validDimension(dimension) {
		return nil, errcode.ErrMonitoringInvalidDimension
	}
	sc := scope
	if sc == "" {
		sc = string(ScopeTotal) // 省略默认 total（D-04）
	} else if !validScope(sc) || Dimension(dimension) != DimCPU {
		// scope 非法，或 dimension≠cpu 时携带 scope（D-04）→ MON_003。
		return nil, errcode.ErrMonitoringInvalidScope
	}

	series, start, partial := s.buf.Series(Window(window), Dimension(dimension), Scope(sc))
	data := &HistoryData{
		Window:    window,
		Dimension: dimension,
		Scope:     sc,
		Partial:   partial,
		Series:    make([]SeriesData, 0, len(series)), // 空数据序列化为 []（非 null）
	}
	if start != nil {
		ts := formatTS(*start)
		data.StartTime = &ts
	}
	for _, sr := range series {
		sd := SeriesData{Key: sr.Key, Label: sr.Label, Unit: sr.Unit, Points: make([]PointData, len(sr.Points))}
		for i, p := range sr.Points {
			sd.Points[i] = PointData{TS: formatTS(p.TS), V: p.V}
		}
		data.Series = append(data.Series, sd)
	}
	return data, nil
}

// TopProcesses 返回 TOP 进程清单（GET /monitoring/processes/top）：无采样或本轮
// TOP 采集失败 → MON_004（进程维度无部分成功语义，区别于指标降级）。
func (s *Service) TopProcesses(ctx context.Context) (*TopData, error) {
	smp, ok := s.buf.Latest()
	if !ok || !smp.ProcessesOK {
		return nil, errcode.ErrMonitoringNoData
	}
	return &TopData{TS: formatTS(smp.TS), Processes: smp.Processes}, nil
}

// SystemInfo 返回系统信息（GET /monitoring/system/info）：每次实时读取，全部
// 字段失败仍 code=0 + errors 全量清单（§4.2.4，读取行为不视为服务错误）。
func (s *Service) SystemInfo(ctx context.Context) (*SystemInfoData, error) {
	data := readSystemInfo(ctx)
	return &data, nil
}

// snapshotFromSample 由实时层采样组装快照 DTO（字段一一映射，无增删）。
// processes 采集失败（null）本身即降级态——degraded 不变式在此自持有
// （不依赖采集侧置位约定，D-08）。
func snapshotFromSample(s *Sample) *SnapshotData {
	d := &SnapshotData{
		TS:       formatTS(s.TS),
		Degraded: s.Degraded || !s.ProcessesOK,
		CPU: CPUSnapshot{
			TotalPercent: s.CPUTotal,
			PerCore:      s.CPUPerCore,
		},
		Memory: MemorySnapshot{
			UsedBytes:   s.MemUsedBytes,
			TotalBytes:  s.MemTotalBytes,
			UsedPercent: s.MemUsedPercent,
		},
		Disk: DiskSnapshot{
			Capacity:   s.DiskCapacity,
			IOReadBps:  s.DiskIOReadBps,
			IOWriteBps: s.DiskIOWriteBps,
		},
		Net: NetSnapshot{
			UpBps:   s.NetUpBps,
			DownBps: s.NetDownBps,
		},
		Load: LoadSnapshot{
			L1:  s.Load1,
			L5:  s.Load5,
			L15: s.Load15,
		},
	}
	if s.ProcessesOK {
		d.Processes = s.Processes
	} // 失败保持 nil → JSON null（D-08）
	return d
}
