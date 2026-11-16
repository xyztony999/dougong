// Package monitoring 是系统监控只读观测服务（api-monitoring.md）：后台采集
// 链路（3s 采样 → 1min 聚合 → 进程内环形缓冲 1440 条 ≈ 24h）+ 四类只读查询。
// 零持久化（ADR-014：无表/无 DDL/无 SQL），不感知 HTTP（DTO 仅带 JSON tag）。
package monitoring

import "time"

// 业务常量（api-monitoring.md §3.4：⚠️A12 编译期定值，非配置键、不入库）。
const (
	// SampleInterval 采样周期（3s，采集协程节拍）。
	SampleInterval = 3 * time.Second
	// BufferCapacity 环形缓冲容量（1440 条 1min 粒度 ≈ 24h，满后滚动淘汰）。
	BufferCapacity = 1440
	// TopN TOP 进程条数（按 cpuPercent 降序，I-06）。
	TopN = 10
)

// Window 是趋势时间窗口枚举（metric_window）。
type Window string

// 窗口取值（01-solution/04-business-enum.md 定稿）。
const (
	WindowLast1h  Window = "last_1h"
	WindowLast24h Window = "last_24h"
)

// Minutes 返回窗口覆盖的分钟数（= 序列点数上限，api 4.1.4）。
func (w Window) Minutes() int {
	if w == WindowLast24h {
		return BufferCapacity
	}
	return 60
}

// validWindow 校验 window ∈ metric_window（MON_001 映射依据）。
func validWindow(s string) bool {
	return s == string(WindowLast1h) || s == string(WindowLast24h)
}

// Dimension 是趋势指标维度枚举（metric_dimension）。
type Dimension string

// 维度取值。
const (
	DimCPU     Dimension = "cpu"
	DimMemory  Dimension = "memory"
	DimDisk    Dimension = "disk"
	DimNetwork Dimension = "network"
	DimLoad    Dimension = "load"
)

// validDimension 校验 dimension ∈ metric_dimension（MON_002 映射依据）。
func validDimension(s string) bool {
	switch Dimension(s) {
	case DimCPU, DimMemory, DimDisk, DimNetwork, DimLoad:
		return true
	}
	return false
}

// Scope 是 cpu 维度统计口径枚举（metric_scope，D-04：仅 dimension=cpu 合法）。
type Scope string

// 口径取值。
const (
	ScopeTotal   Scope = "total"
	ScopePerCore Scope = "per_core"
)

// validScope 校验 scope ∈ metric_scope（MON_003 映射依据；空串由调用方先归一为 total）。
func validScope(s string) bool {
	return s == string(ScopeTotal) || s == string(ScopePerCore)
}

// series key 常量（api-monitoring.md §3.3 series 命名前缀，D-03 统一形状）。
const (
	keyCPUTotal    = "cpu_total"
	keyCPUCoreFmt  = "cpu_core_%d"
	keyMemoryUsed  = "memory_used"
	keyDiskCapFmt  = "disk_capacity:%s"
	keyDiskIORead  = "disk_io_read"
	keyDiskIOWrite = "disk_io_write"
	keyNetUp       = "network_up"
	keyNetDown     = "network_down"
	keyLoad1m      = "load_1m"
	keyLoad5m      = "load_5m"
	keyLoad15m     = "load_15m"
)

// 序列单位（D-02：SI 基本单位，人类可读格式化归前端）。
const (
	UnitPercent = "percent"
	UnitBytes   = "bytes"
	UnitBps     = "bps"
	UnitValue   = "value"
)
