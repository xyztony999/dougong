package servicemgmt

import "time"

// DTO（camelCase JSON；时间 RFC3339 UTC——api §3.1 公共约定）。结构与
// api-service-mgmt.md §4.1.4 接口详情逐字段对齐，禁止自行增删；nil 指针
// 序列化为 null（description/loadState 可空属性）。

// UnitListItem 是 list 单条（不含 description——单点 detail 承载，载荷考虑）。
type UnitListItem struct {
	Name               string `json:"name"`
	UnitType           string `json:"unitType"`
	ActiveState        string `json:"activeState"`
	SubState           string `json:"subState"`
	EnabledState       string `json:"enabledState"`
	CanOperate         bool   `json:"canOperate"`
	CanToggleAutostart bool   `json:"canToggleAutostart"`
}

// ListData 是 GET /services/unit/list 的 data（全量不分页，D-02；total=过滤后
// 条数；generatedAt=投影生成时刻）。
type ListData struct {
	Items       []UnitListItem `json:"items"`
	Total       int            `json:"total"`
	GeneratedAt string         `json:"generatedAt"`
}

// DetailData 是 GET /services/unit/detail 的 data（description/loadState 为
// 可空属性，缺失为 null）。
type DetailData struct {
	Name               string  `json:"name"`
	UnitType           string  `json:"unitType"`
	ActiveState        string  `json:"activeState"`
	SubState           string  `json:"subState"`
	EnabledState       string  `json:"enabledState"`
	CanOperate         bool    `json:"canOperate"`
	CanToggleAutostart bool    `json:"canToggleAutostart"`
	Description        *string `json:"description"`
	LoadState          *string `json:"loadState"`
}

// ActionData 是 start/stop/restart 成功 data（操作后投影快照，免二次查询）。
type ActionData struct {
	Name         string `json:"name"`
	ActiveState  string `json:"activeState"`
	SubState     string `json:"subState"`
	EnabledState string `json:"enabledState"`
}

// ActionFailureData 是 SVC_007 data（失败原因 + 实际终态；调用期拒绝无终态
// 时 activeState=null）。
type ActionFailureData struct {
	Name        string  `json:"name"`
	Reason      string  `json:"reason"`
	ActiveState *string `json:"activeState"`
}

// TimeoutData 是 SVC_006 data（任务仍在执行，终态经列表刷新确认）。
type TimeoutData struct {
	Name string `json:"name"`
	Note string `json:"note"`
}

// AutostartData 是 enable/disable 成功 data（enabledState=刷新值）。
type AutostartData struct {
	Name         string `json:"name"`
	EnabledState string `json:"enabledState"`
}

// formatTS 输出 ISO-8601 UTC（RFC3339 秒级精度，api §3.1 时间格式）。
func formatTS(t time.Time) string { return t.UTC().Format(time.RFC3339) }
