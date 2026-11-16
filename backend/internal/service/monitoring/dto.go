package monitoring

import "time"

// DTO（camelCase JSON；时间 RFC3339 UTC，api-monitoring.md §3.1）。结构与
// api-monitoring.md §4.1.4/§4.2.4 接口详情逐字段对齐，禁止自行增删；nil
// 指针/切片序列化为 null（D-08：单指标失败对应字段 null + degraded=true）。

// SnapshotData 是 GET /monitoring/metrics/snapshot 的 data 结构（最新采样全量，
// processes 与 TOP 同源同响应——快照页一次取全）。
type SnapshotData struct {
	TS        string         `json:"ts"`
	Degraded  bool           `json:"degraded"`
	CPU       CPUSnapshot    `json:"cpu"`
	Memory    MemorySnapshot `json:"memory"`
	Disk      DiskSnapshot   `json:"disk"`
	Net       NetSnapshot    `json:"net"`
	Load      LoadSnapshot   `json:"load"`
	Processes []ProcessInfo  `json:"processes"`
}

// CPUSnapshot CPU 组（totalPercent + perCore 两层，D-04 快照单响应无需参数）。
type CPUSnapshot struct {
	TotalPercent *float64  `json:"totalPercent"`
	PerCore      []float64 `json:"perCore"`
}

// MemorySnapshot 内存组（bytes 整数 + 百分比 1 位小数，D-02）。
type MemorySnapshot struct {
	UsedBytes   *uint64  `json:"usedBytes"`
	TotalBytes  *uint64  `json:"totalBytes"`
	UsedPercent *float64 `json:"usedPercent"`
}

// DiskSnapshot 磁盘组（按挂载点容量 + 全设备合计 IO 速率，⚠️A13）。
type DiskSnapshot struct {
	Capacity   []DiskCapacityItem `json:"capacity"`
	IOReadBps  *int64             `json:"ioReadBps"`
	IOWriteBps *int64             `json:"ioWriteBps"`
}

// NetSnapshot 网络组（全设备合计上下行速率）。
type NetSnapshot struct {
	UpBps   *int64 `json:"upBps"`
	DownBps *int64 `json:"downBps"`
}

// LoadSnapshot 负载组（原值 2 位小数）。
type LoadSnapshot struct {
	L1  *float64 `json:"l1"`
	L5  *float64 `json:"l5"`
	L15 *float64 `json:"l15"`
}

// HistoryData 是 GET /monitoring/metrics/history 的 data 结构（D-03 统一形状：
// 窗口 × 维度 × 口径 → series[]；startTime 表达重启清零边界，v=null 表缺失）。
type HistoryData struct {
	Window    string       `json:"window"`
	Dimension string       `json:"dimension"`
	Scope     string       `json:"scope"`
	StartTime *string      `json:"startTime"`
	Partial   bool         `json:"partial"`
	Series    []SeriesData `json:"series"`
}

// SeriesData 单条序列（key 规范见 §3.3 series 命名前缀常量表）。
type SeriesData struct {
	Key    string      `json:"key"`
	Label  string      `json:"label"`
	Unit   string      `json:"unit"`
	Points []PointData `json:"points"`
}

// PointData 单个分钟点（v=null 表该分钟缺失：采样失败/未覆盖，D-03）。
type PointData struct {
	TS string   `json:"ts"`
	V  *float64 `json:"v"`
}

// TopData 是 GET /monitoring/processes/top 的 data 结构（≤10、降序，I-06）。
type TopData struct {
	TS        string        `json:"ts"`
	Processes []ProcessInfo `json:"processes"`
}

// SystemInfoData 是 GET /monitoring/system/info 的 data 结构（静态+运行信息单
// 响应；失败字段 null 并入 errors，不返回占位文本，I-08/D-08）。
type SystemInfoData struct {
	Distro        *string          `json:"distro"`
	Kernel        *string          `json:"kernel"`
	Hostname      *string          `json:"hostname"`
	UptimeSeconds *uint64          `json:"uptimeSeconds"`
	Errors        []InfoFieldError `json:"errors"`
}

// InfoFieldError 是系统信息部分失败清单单项（{field, reason}）。
type InfoFieldError struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

// formatTS 输出 ISO-8601 UTC（RFC3339，秒级精度，api §3.1 时间格式）。
func formatTS(t time.Time) string { return t.UTC().Format(time.RFC3339) }
