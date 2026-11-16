// Package sysd 是 systemd dbus 网关适配器（api-service-mgmt.md §4.1.5
// SysdGateway 契约的实现；R-08 唯一 systemd 交互出口——全部经 dbus 参数化
// 方法调用，无 shell、无命令拼接）。
//
// 契约类型与哨兵错误定义于本包（底层共享内核，依赖方向 service → adapter，
// backend-01 §1），消费方 internal/service/servicemgmt 以 Gateway 接口引用
// （接口随消费者），main 装配注入。
package sysd

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"
)

// UnitSnapshot 是 list 投影条目（api §4.1.1 字段清单；ActiveState 已归一
// 面板三态，EnabledState/SubState 为 systemd 实际值透传）。
type UnitSnapshot struct {
	Name         string // unit 全名（投影标识与动作寻址，D-09）
	UnitType     string // name 后缀归一（官方 11 类；未知后缀为 ""）
	ActiveState  string // active|inactive|failed（A-S3 瞬时态归一）
	SubState     string // systemd 实际值透传（running/exited/waiting…，D-04）
	EnabledState string // enabled/disabled/static… 透传（无 unit 文件为 ""，D-05）
	Description  string // detail 用（list 载荷考虑不下发）
	LoadState    string // detail 用（loaded/not-found/errored… 透传）
}

// UnitDetail 是单 unit 属性快照（含 description/loadState，§4.1.5 GetUnit）。
type UnitDetail struct {
	Name         string
	UnitType     string
	ActiveState  string
	SubState     string
	EnabledState string
	Description  string
	LoadState    string
}

// JobResult 是动作 Job 终态（§4.1.5 契约：Succeeded/Reason/终态运行态）。
type JobResult struct {
	Succeeded   bool
	Reason      string // systemd JobRemoved result（done/failed/timeout/…）
	ActiveState string // Job 终态后的实际运行态（尽力读取，读取失败为零值）
	SubState    string
}

// 哨兵错误（消费方据此映射 SVC_003/006/007/008，A-S8 归一口径）。
var (
	// ErrUnavailable dbus 通道/systemd 不可达（SVC_008 语义；不返回过期数据 I-01）。
	ErrUnavailable = errors.New("sysd: systemd channel unavailable")
	// ErrUnitNotFound 目标 unit 不存在（SVC_003 语义，I-03 实况校验）。
	ErrUnitNotFound = errors.New("sysd: unit not found")
	// ErrJobTimeout 等待 Job 终态超上限（SVC_006 语义；Job 仍在 systemd 侧执行）。
	ErrJobTimeout = errors.New("sysd: job wait timeout")
	// ErrRejected systemd 拒绝操作调用（SVC_007 语义，如破坏性事务/配置只读）。
	ErrRejected = errors.New("sysd: operation rejected by systemd")
)

// unitTypeSuffixes 官方 unit 类型后缀全集（unit_type 枚举，D-09 取值域）。
var unitTypeSuffixes = map[string]bool{
	"service": true, "socket": true, "timer": true, "mount": true,
	"path": true, "target": true, "automount": true, "slice": true,
	"swap": true, "scope": true, "device": true,
}

// UnitTypeOf 由 unit 全名后缀归一类型（§4.1.1 unitType 字段口径；
// 未知后缀返回 ""）。
func UnitTypeOf(name string) string {
	i := strings.LastIndex(name, ".")
	if i < 0 || i == len(name)-1 {
		return ""
	}
	sfx := name[i+1:]
	if unitTypeSuffixes[sfx] {
		return sfx
	}
	return ""
}

// NormalizeActiveState 将 systemd active state 归一面板三态（A-S3，辨析①：
// 瞬时执行态不进面板状态模型，方向归一——activating/reloading→active、
// deactivating→inactive、maintenance→failed；未知值透传兜底（防御，systemd
// 为封闭集））。
func NormalizeActiveState(raw string) string {
	switch raw {
	case "active", "reloading", "activating":
		return "active"
	case "inactive", "deactivating":
		return "inactive"
	case "failed", "maintenance":
		return "failed"
	default:
		return raw
	}
}

// LoadedUnit 是 dbus ListUnits 单条的纯数据视图（gateway 层转换后进入合并）。
type LoadedUnit struct {
	Name, Description, LoadState, ActiveState, SubState string
}

// UnitFile 是 dbus ListUnitFiles 单条（路径已归一为 unit 名）。
type UnitFile struct {
	Name  string
	State string // enabled/disabled/static/indirect/masked/alias/…（透传）
}

// FileBaseName unit 文件绝对路径 → unit 名（basename；服务端数据，非用户输入）。
func FileBaseName(path string) string { return filepath.Base(path) }

// MergeUnitProjection 合并已加载（ListUnits）与已装（ListUnitFiles）清单 →
// 已装 unit 全集投影（I-03/A-S2）：file-only 条目（已装未加载）取加载后规范值
// inactive/dead；无 unit 文件的 transient/scope/device 条目 EnabledState=""。
// 输出按名称字典序升序（D-02）。
func MergeUnitProjection(units []LoadedUnit, files []UnitFile) []UnitSnapshot {
	fileState := make(map[string]string, len(files))
	for _, f := range files {
		if f.Name != "" {
			fileState[f.Name] = f.State
		}
	}
	seen := make(map[string]bool, len(units))
	out := make([]UnitSnapshot, 0, len(units)+len(files)/2)
	for _, u := range units {
		if u.Name == "" {
			continue
		}
		seen[u.Name] = true
		out = append(out, UnitSnapshot{
			Name:         u.Name,
			UnitType:     UnitTypeOf(u.Name),
			ActiveState:  NormalizeActiveState(u.ActiveState),
			SubState:     u.SubState,
			EnabledState: fileState[u.Name],
			Description:  u.Description,
			LoadState:    u.LoadState,
		})
	}
	for name, st := range fileState {
		if seen[name] {
			continue
		}
		out = append(out, UnitSnapshot{
			Name:         name,
			UnitType:     UnitTypeOf(name),
			ActiveState:  "inactive",
			SubState:     "dead",
			EnabledState: st,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
