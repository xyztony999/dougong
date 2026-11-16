package monitoring

import "time"

// DiskCapacityItem 是按挂载点分设备的磁盘容量（⚠️A13；单位口径 D-02：bytes 整数）。
type DiskCapacityItem struct {
	Mount      string `json:"mount"`
	TotalBytes uint64 `json:"totalBytes"`
	UsedBytes  uint64 `json:"usedBytes"`
}

// ProcessInfo 是 TOP 进程清单单项（I-06：≤10、cpuPercent 降序；cpuPercent/memBytes
// 为 null 表该进程本轮读取失败或无基线——D-08 不虚构默认值）。
type ProcessInfo struct {
	PID        int32    `json:"pid"`
	Name       string   `json:"name"`
	CPUPercent *float64 `json:"cpuPercent"`
	MemBytes   *uint64  `json:"memBytes"`
}

// Sample 是一轮 3s 采样的完整条目（实时层槽位数据源；指针/slice nil = 该指标
// 本轮采集失败，D-08）。TS 为 UTC 秒精度。
type Sample struct {
	TS time.Time

	CPUTotal       *float64 // CPU 总使用率 %（1 位小数，A-M5 分核均值）
	CPUPerCore     []float64
	MemUsedBytes   *uint64 // 内存占用（三值同源于一次读取，同生同缺）
	MemTotalBytes  *uint64
	MemUsedPercent *float64

	DiskCapacity   []DiskCapacityItem // 按挂载点（nil = 分区读取整体失败）
	DiskIOReadBps  *int64             // 全设备合计 IO 速率（⚠️A13；首轮无基线 → nil）
	DiskIOWriteBps *int64

	NetUpBps   *int64 // 全设备合计上下行速率（⚠️A13）
	NetDownBps *int64

	Load1  *float64 // 系统负载（2 位小数）
	Load5  *float64
	Load15 *float64

	Processes   []ProcessInfo // TOP 清单（I-06；nil = 本轮采集失败）
	ProcessesOK bool          // 本轮进程采集是否成功（MON_004 判据，区别于指标降级）
	Degraded    bool          // 本轮存在采集失败指标（部分字段缺省）
}

// MinuteEntry 是环形缓冲的分钟聚合条目（缓冲条目 = 快照/趋势/TOP 共同数据源；
// I-01 写入后不可变；nil 指针 = 该分钟该指标全程缺失，趋势点 v=null，I-05）。
type MinuteEntry struct {
	TS time.Time // 分钟聚合锚点（UTC，对齐分钟边界）

	CPUTotal       *float64  // 窗口内算术平均（A-M2；下同）
	CPUPerCore     []float64 // 分核均值（nil = 全程缺失）
	MemUsedBytes   *uint64
	MemTotalBytes  *uint64
	MemUsedPercent *float64

	DiskCapacity   []DiskCapacityItem // 该分钟最后一次成功值
	DiskIOReadBps  *int64
	DiskIOWriteBps *int64

	NetUpBps   *int64
	NetDownBps *int64

	Load1  *float64
	Load5  *float64
	Load15 *float64

	TopProcesses []ProcessInfo // 该分钟最后一次成功值
	Degraded     bool          // 该分钟任一样本存在采集失败
}

// Point 是趋势序列点（D-03：v=null 表该分钟缺失）。
type Point struct {
	TS time.Time
	V  *float64
}

// Series 是一条趋势序列的进程内形态（D-03 统一形状；只读投影，调用方不持有
// 内部切片引用——api §4.1.5 缓冲访问契约约束）。
type Series struct {
	Key    string
	Label  string
	Unit   string
	Points []Point
}
