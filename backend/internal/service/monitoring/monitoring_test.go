package monitoring

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/errcode"
)

// fixedClock 返回固定时刻（缓冲锚点确定性）。
func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

func f64(v float64) *float64 { return &v }
func i64(v int64) *int64     { return &v }
func u64(v uint64) *uint64   { return &v }

func minuteAt(base time.Time, delta int) time.Time {
	return base.Truncate(time.Minute).Add(time.Duration(delta) * time.Minute)
}

// ---- 环形缓冲不变量（I-01/I-02/I-04）----

func TestBufferRingEviction(t *testing.T) {
	buf := NewMetricsBuffer()
	base := time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC)
	// 写入 1445 条（>容量）：最旧 5 条被滚动淘汰（I-02）。
	for i := 0; i < BufferCapacity+5; i++ {
		v := float64(i)
		buf.AppendMinute(MinuteEntry{TS: base.Add(time.Duration(i) * time.Minute), CPUTotal: &v})
	}
	for i := 0; i < 5; i++ {
		if e := buf.ring[minuteIndex(base.Add(time.Duration(i)*time.Minute))]; e == nil || e.TS != base.Add(time.Duration(i+BufferCapacity)*time.Minute) {
			t.Fatalf("slot %d should hold evicted-by newer entry (i=%d)", minuteIndex(base.Add(time.Duration(i)*time.Minute)), i)
		}
	}
	// 保留区间 [5, 1445)：任意抽查仍可按分钟索引命中。
	for _, i := range []int{5, 700, 1444} {
		want := base.Add(time.Duration(i) * time.Minute)
		if e := buf.ring[minuteIndex(want)]; e == nil || !e.TS.Equal(want) {
			t.Fatalf("entry at minute %d missing", i)
		}
	}
}

func TestBufferLatestSlot(t *testing.T) {
	buf := NewMetricsBuffer()
	if _, ok := buf.Latest(); ok {
		t.Fatal("empty buffer Latest should be !ok (MON_004 判据)")
	}
	s := Sample{TS: time.Now(), CPUTotal: f64(12.5), ProcessesOK: true}
	buf.SetLatest(s)
	got, ok := buf.Latest()
	if !ok || got.CPUTotal == nil || *got.CPUTotal != 12.5 {
		t.Fatalf("Latest = %+v ok=%v", got, ok)
	}
}

// ---- 分钟聚合（A-M2/A-M3，I-05/I-06）----

func TestMinuteAggregatorAveragingAndRollover(t *testing.T) {
	buf := NewMetricsBuffer()
	agg := NewMinuteAggregator(buf)
	m0 := time.Date(2026, 11, 14, 8, 30, 0, 0, time.UTC)

	proc := func(pid int32, pct float64) ProcessInfo {
		return ProcessInfo{PID: pid, Name: "p", CPUPercent: f64(pct), MemBytes: u64(100)}
	}
	// 第 1 分钟两轮采样（cpu 10/20 → 均值 15；容量与 TOP 取末值；无 load → nil）。
	agg.Offer(Sample{TS: m0.Add(3 * time.Second), CPUTotal: f64(10), CPUPerCore: []float64{10, 10},
		MemUsedBytes: u64(100), MemTotalBytes: u64(400), MemUsedPercent: f64(25),
		DiskIOReadBps: i64(100), DiskIOWriteBps: i64(50), NetUpBps: i64(10), NetDownBps: i64(20),
		DiskCapacity: []DiskCapacityItem{{Mount: "/", TotalBytes: 1000, UsedBytes: 300}},
		ProcessesOK:  true, Processes: []ProcessInfo{proc(1, 5)}})
	agg.Offer(Sample{TS: m0.Add(6 * time.Second), CPUTotal: f64(20), CPUPerCore: []float64{20, 20},
		MemUsedBytes: u64(300), MemTotalBytes: u64(400), MemUsedPercent: f64(75),
		DiskIOReadBps: i64(300), DiskIOWriteBps: i64(150), NetUpBps: i64(30), NetDownBps: i64(60),
		DiskCapacity: []DiskCapacityItem{{Mount: "/", TotalBytes: 1000, UsedBytes: 500}},
		ProcessesOK:  true, Processes: []ProcessInfo{proc(2, 9)}, Degraded: true})
	// 跨分钟首个采样 → 终结算第 1 分钟入缓冲（A-M3）。
	agg.Offer(Sample{TS: m0.Add(time.Minute + 2*time.Second), CPUTotal: f64(99)})

	if buf.lastMinute != m0 {
		t.Fatalf("lastMinute = %v want %v", buf.lastMinute, m0)
	}
	e := buf.ring[minuteIndex(m0)]
	if e == nil {
		t.Fatal("minute entry not appended on rollover")
	}
	if e.CPUTotal == nil || *e.CPUTotal != 15 {
		t.Fatalf("cpu avg = %v want 15", e.CPUTotal)
	}
	if len(e.CPUPerCore) != 2 || e.CPUPerCore[0] != 15 || e.CPUPerCore[1] != 15 {
		t.Fatalf("per-core avg = %v", e.CPUPerCore)
	}
	if e.MemUsedPercent == nil || *e.MemUsedPercent != 50 {
		t.Fatalf("mem pct avg = %v want 50", e.MemUsedPercent)
	}
	if e.MemUsedBytes == nil || *e.MemUsedBytes != 200 || *e.MemTotalBytes != 400 {
		t.Fatalf("mem bytes avg = %v/%v want 200/400", e.MemUsedBytes, e.MemTotalBytes)
	}
	if e.DiskIOReadBps == nil || *e.DiskIOReadBps != 200 || *e.DiskIOWriteBps != 100 {
		t.Fatalf("disk io avg = %v/%v want 200/100", e.DiskIOReadBps, e.DiskIOWriteBps)
	}
	if e.NetUpBps == nil || *e.NetUpBps != 20 || *e.NetDownBps != 40 {
		t.Fatalf("net avg = %v/%v want 20/40", e.NetUpBps, e.NetDownBps)
	}
	if e.Load1 != nil || e.Load5 != nil || e.Load15 != nil {
		t.Fatalf("load absent all minute should stay nil (I-05): %v/%v/%v", e.Load1, e.Load5, e.Load15)
	}
	if len(e.DiskCapacity) != 1 || e.DiskCapacity[0].UsedBytes != 500 {
		t.Fatalf("capacity should take last value: %+v", e.DiskCapacity)
	}
	if len(e.TopProcesses) != 1 || e.TopProcesses[0].PID != 2 {
		t.Fatalf("top should take last value: %+v", e.TopProcesses)
	}
	if !e.Degraded {
		t.Fatal("degraded should propagate from any sample in minute")
	}
}

func TestMinuteAggregatorFullyDegradedMinute(t *testing.T) {
	buf := NewMetricsBuffer()
	agg := NewMinuteAggregator(buf)
	m0 := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	agg.Offer(Sample{TS: m0.Add(3 * time.Second), Degraded: true}) // 全指标缺失
	agg.Offer(Sample{TS: m0.Add(time.Minute + 3*time.Second)})
	e := buf.ring[minuteIndex(m0)]
	if e == nil {
		t.Fatal("fully degraded minute still appends (I-05: null 值条目)")
	}
	if e.CPUTotal != nil || e.MemUsedPercent != nil || e.TopProcesses != nil {
		t.Fatalf("all metrics should be nil: %+v", e)
	}
	if !e.Degraded {
		t.Fatal("Degraded must be true")
	}
}

// ---- 序列投影（D-03/D-04，A-M4/A-M9/A-M10）----

func newProjectionBuffer(t *testing.T) (*MetricsBuffer, time.Time) {
	t.Helper()
	buf := NewMetricsBuffer()
	// 固定时钟：锚点=12:59（墙上 13:00 的上一分钟）。
	now := time.Date(2026, 11, 14, 13, 0, 5, 0, time.UTC)
	buf.now = fixedClock(now)
	return buf, now
}

func TestSeriesEmptyBuffer(t *testing.T) {
	buf, _ := newProjectionBuffer(t)
	series, start, partial := buf.Series(WindowLast1h, DimCPU, ScopeTotal)
	if len(series) != 0 || start != nil || partial {
		t.Fatalf("empty buffer: series=%d start=%v partial=%v（D-07 空序列）", len(series), start, partial)
	}
}

func TestSeriesCPUTotal(t *testing.T) {
	buf, now := newProjectionBuffer(t)
	// 窗口内最后 3 个完成分钟有数据（12:57/12:58/12:59），12:59 缺 cpu。
	buf.AppendMinute(MinuteEntry{TS: minuteAt(now, -3), CPUTotal: f64(10)})
	buf.AppendMinute(MinuteEntry{TS: minuteAt(now, -2), CPUTotal: f64(20)})
	buf.AppendMinute(MinuteEntry{TS: minuteAt(now, -1)}) // cpu 缺失 → v=null
	series, start, partial := buf.Series(WindowLast1h, DimCPU, ScopeTotal)
	if len(series) != 1 {
		t.Fatalf("cpu total series count = %d want 1", len(series))
	}
	s := series[0]
	if s.Key != keyCPUTotal || s.Unit != UnitPercent || s.Label == "" {
		t.Fatalf("series meta: %+v", s)
	}
	if len(s.Points) != 60 {
		t.Fatalf("points = %d want 60（窗口分钟数）", len(s.Points))
	}
	if !s.Points[0].TS.Equal(minuteAt(now, -60)) || !s.Points[59].TS.Equal(minuteAt(now, -1)) {
		t.Fatalf("slot range = [%v..%v]", s.Points[0].TS, s.Points[59].TS)
	}
	if s.Points[57].V == nil || *s.Points[57].V != 10 || *s.Points[58].V != 20 {
		t.Fatalf("values at -3/-2 min wrong: %+v", s.Points[57:60])
	}
	if s.Points[59].V != nil {
		t.Fatal("minute with cpu missing must be v=null (I-05)")
	}
	if s.Points[0].V != nil {
		t.Fatal("uncovered minute must be v=null")
	}
	if !partial {
		t.Fatal("partial must be true when any null point (A-M10)")
	}
	if start == nil || !start.Equal(minuteAt(now, -3)) {
		t.Fatalf("startTime = %v want earliest valid point %v", start, minuteAt(now, -3))
	}
}

func TestSeriesFullCoverageNoPartial(t *testing.T) {
	buf, now := newProjectionBuffer(t)
	for i := -60; i <= -1; i++ {
		buf.AppendMinute(MinuteEntry{TS: minuteAt(now, i), CPUTotal: f64(float64(i))})
	}
	_, _, partial := buf.Series(WindowLast1h, DimCPU, ScopeTotal)
	if partial {
		t.Fatal("fully covered window should not be partial")
	}
}

func TestSeriesCPUPerCore(t *testing.T) {
	buf, now := newProjectionBuffer(t)
	buf.AppendMinute(MinuteEntry{TS: minuteAt(now, -2), CPUPerCore: []float64{11, 22, 33}})
	buf.AppendMinute(MinuteEntry{TS: minuteAt(now, -1), CPUPerCore: []float64{44, 55, 66}})
	series, _, _ := buf.Series(WindowLast1h, DimCPU, ScopePerCore)
	if len(series) != 3 {
		t.Fatalf("per-core series = %d want 3", len(series))
	}
	for i, s := range series {
		if s.Key != fmt.Sprintf("cpu_core_%d", i) {
			t.Fatalf("key[%d] = %s", i, s.Key)
		}
		last := s.Points[59].V
		if last == nil || *last != float64(44+11*i) {
			t.Fatalf("core %d last value = %v", i, last)
		}
	}
	// 分核全缺失窗口 → 空序列（A-M10 边界）。
	buf2, now2 := newProjectionBuffer(t)
	buf2.AppendMinute(MinuteEntry{TS: minuteAt(now2, -1), CPUTotal: f64(5)})
	series2, _, _ := buf2.Series(WindowLast1h, DimCPU, ScopePerCore)
	if len(series2) != 0 {
		t.Fatalf("per-core with no data = %d want 0", len(series2))
	}
}

func TestSeriesMemoryNetworkLoad(t *testing.T) {
	buf, now := newProjectionBuffer(t)
	e := MinuteEntry{TS: minuteAt(now, -1),
		MemUsedPercent: f64(66.6), NetUpBps: i64(1000), NetDownBps: i64(2000),
		Load1: f64(0.51), Load5: f64(0.52), Load15: f64(0.53)}
	buf.AppendMinute(e)

	mem, _, _ := buf.Series(WindowLast1h, DimMemory, ScopeTotal)
	if len(mem) != 1 || mem[0].Key != keyMemoryUsed || mem[0].Unit != UnitPercent ||
		mem[0].Points[59].V == nil || *mem[0].Points[59].V != 66.6 {
		t.Fatalf("memory series wrong: %+v", mem)
	}
	net, _, _ := buf.Series(WindowLast1h, DimNetwork, ScopeTotal)
	if len(net) != 2 || net[0].Key != keyNetUp || net[1].Key != keyNetDown || net[0].Unit != UnitBps {
		t.Fatalf("network series wrong: %+v", net)
	}
	if net[0].Points[59].V == nil || *net[0].Points[59].V != 1000 || *net[1].Points[59].V != 2000 {
		t.Fatalf("network values wrong")
	}
	load, _, _ := buf.Series(WindowLast1h, DimLoad, ScopeTotal)
	if len(load) != 3 || load[0].Key != keyLoad1m || load[2].Key != keyLoad15m || load[0].Unit != UnitValue {
		t.Fatalf("load series wrong: %+v", load)
	}
	if load[0].Points[59].V == nil || *load[2].Points[59].V != 0.53 {
		t.Fatalf("load values wrong")
	}
}

func TestSeriesDiskMountUnion(t *testing.T) {
	buf, now := newProjectionBuffer(t)
	// -2 分钟挂 / 与 /data；-1 分钟 /data 已卸载（如实记录卸载历史，A-M9）。
	buf.AppendMinute(MinuteEntry{TS: minuteAt(now, -2),
		DiskCapacity:  []DiskCapacityItem{{Mount: "/data", TotalBytes: 200, UsedBytes: 20}, {Mount: "/", TotalBytes: 100, UsedBytes: 10}},
		DiskIOReadBps: i64(500), DiskIOWriteBps: i64(600)})
	buf.AppendMinute(MinuteEntry{TS: minuteAt(now, -1),
		DiskCapacity:  []DiskCapacityItem{{Mount: "/", TotalBytes: 100, UsedBytes: 30}},
		DiskIOReadBps: i64(700), DiskIOWriteBps: i64(800)})

	series, _, partial := buf.Series(WindowLast1h, DimDisk, ScopeTotal)
	if len(series) != 4 { // 2 挂载点 + 双 IO
		t.Fatalf("disk series = %d want 4", len(series))
	}
	if series[0].Key != "disk_capacity:/" || series[1].Key != "disk_capacity:/data" {
		t.Fatalf("capacity keys/order wrong: %s %s（mount 字典序）", series[0].Key, series[1].Key)
	}
	if series[0].Unit != UnitBytes || series[2].Key != keyDiskIORead || series[3].Key != keyDiskIOWrite ||
		series[2].Unit != UnitBps {
		t.Fatalf("io series meta wrong: %+v", series[2:])
	}
	if v := series[0].Points[59].V; v == nil || *v != 30 {
		t.Fatalf("/ last used = %v want 30", v)
	}
	if v := series[1].Points[59].V; v != nil {
		t.Fatalf("/data unmounted minute must be null, got %v", v)
	}
	if v := series[1].Points[58].V; v == nil || *v != 20 {
		t.Fatalf("/data historical value = %v want 20", v)
	}
	if v := series[2].Points[59].V; v == nil || *v != 700 {
		t.Fatalf("io read last = %v want 700", v)
	}
	if !partial {
		t.Fatal("partial expected（/data 缺失分钟）")
	}
}

func TestSeriesLast24hWindowLength(t *testing.T) {
	buf, now := newProjectionBuffer(t)
	buf.AppendMinute(MinuteEntry{TS: minuteAt(now, -1400), CPUTotal: f64(7)})
	series, start, _ := buf.Series(WindowLast24h, DimCPU, ScopeTotal)
	if len(series) != 1 || len(series[0].Points) != 1440 {
		t.Fatalf("24h points = %d want 1440", len(series[0].Points))
	}
	if start == nil || !start.Equal(minuteAt(now, -1400)) {
		t.Fatalf("startTime = %v", start)
	}
}

// ---- Service 门面（MON_001~004 与 DTO 组装）----

func wantSymbol(t *testing.T, err error, symbol string) {
	t.Helper()
	var ec *errcode.Error
	if !errors.As(err, &ec) || ec.Symbol() != symbol {
		t.Fatalf("err = %v want symbol %s", err, symbol)
	}
}

func TestHistoryValidation(t *testing.T) {
	svc := NewService()
	ctx := context.Background()
	cases := []struct {
		name, window, dim, scope, want string
	}{
		{"bad window", "last_1d", "cpu", "", "MON_001"},
		{"empty window", "", "cpu", "", "MON_001"},
		{"bad dimension", "last_1h", "gpu", "", "MON_002"},
		{"empty dimension", "last_1h", "", "", "MON_002"},
		{"bad scope value", "last_1h", "cpu", "all", "MON_003"},
		{"scope on non-cpu", "last_1h", "memory", "total", "MON_003"},
		{"scope per_core on disk", "last_24h", "disk", "per_core", "MON_003"},
		{"ok default scope", "last_1h", "cpu", "", ""},
		{"ok explicit total", "last_1h", "cpu", "total", ""},
		{"ok per_core", "last_1h", "cpu", "per_core", ""},
		{"ok no scope memory", "last_24h", "memory", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := svc.History(ctx, tc.window, tc.dim, tc.scope)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("unexpected err: %v", err)
				}
				if data.Scope != string(ScopeTotal) && tc.scope == "" {
					t.Fatalf("default scope = %s want total", data.Scope)
				}
				if len(data.Series) != 0 || data.StartTime != nil {
					t.Fatalf("empty buffer should yield empty series (D-07): %+v", data)
				}
				return
			}
			wantSymbol(t, err, tc.want)
		})
	}
}

func TestSnapshotAndTopSemantics(t *testing.T) {
	svc := NewService()
	ctx := context.Background()

	// 空缓冲：快照/TOP 均 MON_004；history 空 series + code 0（D-07）。
	_, err := svc.Snapshot(ctx)
	wantSymbol(t, err, "MON_004")
	_, err = svc.TopProcesses(ctx)
	wantSymbol(t, err, "MON_004")

	// 最新轮进程采集失败：快照 processes null + degraded；TOP 仍 MON_004。
	ts := time.Date(2026, 11, 14, 10, 0, 3, 0, time.UTC)
	svc.Buffer().SetLatest(Sample{TS: ts, CPUTotal: f64(50), Degraded: true})
	snap, err := svc.Snapshot(ctx)
	if err != nil {
		t.Fatalf("snapshot err: %v", err)
	}
	if snap.Processes != nil || !snap.Degraded {
		t.Fatalf("processes should be null + degraded=true: %+v", snap)
	}
	if snap.TS != "2026-11-14T10:00:03Z" {
		t.Fatalf("ts = %s", snap.TS)
	}
	if snap.CPU.TotalPercent == nil || *snap.CPU.TotalPercent != 50 {
		t.Fatalf("cpu total lost: %+v", snap.CPU)
	}
	_, err = svc.TopProcesses(ctx)
	wantSymbol(t, err, "MON_004")

	// 成功轮：TOP 同源直读（跨接口复用规则）。
	svc.Buffer().SetLatest(Sample{TS: ts, ProcessesOK: true,
		Processes: []ProcessInfo{{PID: 7, Name: "nginx", CPUPercent: f64(3.5), MemBytes: u64(2048)}}})
	top, err := svc.TopProcesses(ctx)
	if err != nil || top.TS != "2026-11-14T10:00:03Z" || len(top.Processes) != 1 || top.Processes[0].Name != "nginx" {
		t.Fatalf("top = %+v err=%v", top, err)
	}
}

func TestHistoryDTOShape(t *testing.T) {
	svc := NewService()
	buf := svc.Buffer()
	now := time.Now()
	buf.AppendMinute(MinuteEntry{TS: minuteAt(now, -2), CPUTotal: f64(10)})
	buf.AppendMinute(MinuteEntry{TS: minuteAt(now, -1), CPUTotal: f64(20)})
	data, err := svc.History(context.Background(), "last_1h", "cpu", "")
	if err != nil {
		t.Fatalf("history err: %v", err)
	}
	if data.Window != "last_1h" || data.Dimension != "cpu" || data.Scope != "total" || !data.Partial {
		t.Fatalf("history meta wrong: %+v", data)
	}
	if data.StartTime == nil || len(data.Series) != 1 || len(data.Series[0].Points) != 60 {
		t.Fatalf("history shape wrong: %+v", data)
	}
	if data.Series[0].Key != keyCPUTotal || data.Series[0].Points[0].V != nil {
		t.Fatalf("series[0] = %+v", data.Series[0])
	}
}

// ---- 采集与系统信息（本机实时冒烟，容错断言：失败路径即降级字段）----

func TestSamplerLiveOneShot(t *testing.T) {
	s := NewMetricsSampler()
	ctx := context.Background()
	first := s.Sample(ctx)
	if first.TS.IsZero() {
		t.Fatal("sample TS must be set")
	}
	time.Sleep(60 * time.Millisecond) // 保证差分 elapsed > 0
	second := s.Sample(ctx)
	// 结构不变量（机器差异容忍：字段可能失败降级，但结构必须自洽）。
	if second.CPUPerCore != nil && second.CPUTotal == nil {
		t.Fatal("per-core present but total missing")
	}
	if second.ProcessesOK {
		if len(second.Processes) > TopN {
			t.Fatalf("processes = %d want ≤%d (I-06)", len(second.Processes), TopN)
		}
		for i := 1; i < len(second.Processes); i++ {
			prev, cur := second.Processes[i-1].CPUPercent, second.Processes[i].CPUPercent
			if prev != nil && cur != nil && *prev < *cur {
				t.Fatal("processes not sorted desc by cpuPercent (I-06)")
			}
		}
	}
	if second.NetUpBps != nil && *second.NetUpBps < 0 {
		t.Fatal("negative rate")
	}
}

func TestSystemInfoLiveRead(t *testing.T) {
	data := readSystemInfo(context.Background())
	// I-08：失败字段必须 null 且登记 errors；成功字段非空。两者互斥一致。
	if data.Distro == nil && data.Errors == nil {
		t.Fatal("distro null must come with errors entry")
	}
	if data.Hostname == nil && data.Errors == nil {
		t.Fatal("hostname null must come with errors entry")
	}
	if data.UptimeSeconds != nil && *data.UptimeSeconds == 0 && data.Distro == nil {
		t.Fatalf("suspicious uptime zero with distro missing: %+v", data)
	}
}

func TestCollectorRunExitsOnCancel(t *testing.T) {
	svc := NewService()
	col := NewCollector(svc)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- col.Run(ctx) }()
	// 首轮采样同步先行——短暂等待后实时槽位应有数据。
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, ok := svc.Buffer().Latest(); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("first sample not ready in 5s")
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("collector exit err: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("collector did not exit on cancel (G-06)")
	}
}

// ---- 工具函数 ----

func TestRoundHelpers(t *testing.T) {
	if round1(1.25) != 1.3 || round1(1.24) != 1.2 {
		t.Fatalf("round1 wrong: %v %v", round1(1.25), round1(1.24))
	}
	if round2(0.125) != 0.13 {
		t.Fatalf("round2 wrong: %v", round2(0.125))
	}
	if mean(nil) != 0 {
		t.Fatal("mean(nil) must be 0")
	}
	if v := diffRate(100, 100, true, 0); v != nil {
		t.Fatalf("elapsed<=0 must be nil, got %v", *v)
	}
	if v := diffRate(100, 200, false, 3); v != nil {
		t.Fatalf("no baseline must be nil, got %v", *v)
	}
	if v := diffRate(300, 100, true, 3); v != nil {
		t.Fatalf("counter rewind must be nil, got %v", *v)
	}
	if v := diffRate(100, 400, true, 3); v == nil || *v != 100 {
		t.Fatalf("diffRate = %v want 100", v)
	}
}

func TestSnapshotFromSampleNullShapes(t *testing.T) {
	// 全指标失败的样本：所有字段 nil、degraded 透传（D-08 不虚构）。
	s := &Sample{TS: time.Date(2026, 11, 14, 10, 0, 3, 0, time.UTC), Degraded: true}
	d := snapshotFromSample(s)
	if d.CPU.TotalPercent != nil || d.CPU.PerCore != nil || d.Memory.UsedBytes != nil ||
		d.Disk.Capacity != nil || d.Disk.IOReadBps != nil || d.Net.UpBps != nil ||
		d.Load.L1 != nil || d.Processes != nil {
		t.Fatalf("all-null snapshot expected: %+v", d)
	}
	if !d.Degraded {
		t.Fatal("degraded lost")
	}
}
