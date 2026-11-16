package firewall

// DTO 是接口契约载荷（api-firewall.md §4.1.4；字段与设计文档逐一对齐，
// handler 薄壳直接包络透出）。时间字段 ISO-8601 UTC（公共约定），
// generatedAt 为投影生成时刻。

// BackendStatusData GET /firewall/backend/status 响应（D-01：即时探测结论
// + 服务端权威动作面；backend=none 时 active=false/version=null/
// supportedActions=[]——前端据此渲染引导页，不调规则面）。
type BackendStatusData struct {
	Backend          string   `json:"backend"`
	Active           bool     `json:"active"`
	Version          *string  `json:"version"`
	SupportedActions []string `json:"supportedActions"`
	GeneratedAt      string   `json:"generatedAt"`
}

// RuleItem 规则投影条目（api §4.1.1 投影字段清单；isPanelPort 服务端标注）。
type RuleItem struct {
	RuleKey     string  `json:"ruleKey"`
	Port        string  `json:"port"`
	Protocol    string  `json:"protocol"`
	Direction   string  `json:"direction"`
	Source      *string `json:"source"`
	Action      string  `json:"action"`
	RawSpec     string  `json:"rawSpec"`
	IsPanelPort bool    `json:"isPanelPort"`
}

// BackendSummary 规则清单随行后端摘要（宿主 S-42-01 页头结论）。
type BackendSummary struct {
	Backend string `json:"backend"`
	Active  bool   `json:"active"`
}

// RuleListData GET /firewall/rule/list 响应（D-02：全量返回不分页，服务端
// 筛选+排序：端口数值升序、同端口按 ruleKey 字典序）。
type RuleListData struct {
	Items       []RuleItem     `json:"items"`
	Total       int            `json:"total"`
	Backend     BackendSummary `json:"backend"`
	GeneratedAt string         `json:"generatedAt"`
}

// ProcessInfo 监听进程（取不到时 process 整体 null；pid/name 字段各自
// 可 null——不虚构，monitoring system_info 同口径）。
type ProcessInfo struct {
	Pid  *int    `json:"pid"`
	Name *string `json:"name"`
}

// PortItem 监听端口对照条目（D-04：openState 读取时派生两态；external 服务
// 端计算）。
type PortItem struct {
	Proto     string       `json:"proto"`
	Addr      string       `json:"addr"`
	Port      int          `json:"port"`
	Process   *ProcessInfo `json:"process"`
	External  bool         `json:"external"`
	OpenState string       `json:"openState"`
}

// PortListData GET /firewall/port/list 响应（对照仅入站方向，出站 OUT-3）。
type PortListData struct {
	Items       []PortItem `json:"items"`
	Total       int        `json:"total"`
	GeneratedAt string     `json:"generatedAt"`
}

// CreateData POST /firewall/rule/create 成功响应（写入后定位引用）。
type CreateData struct {
	RuleKey  string `json:"ruleKey"`
	Port     string `json:"port"`
	Protocol string `json:"protocol"`
	Action   string `json:"action"`
}

// DeleteData POST /firewall/rule/delete 成功响应（delete 非幂等：不存在
// FW_003 不静默成功，D-07）。
type DeleteData struct {
	Deleted bool   `json:"deleted"`
	RuleKey string `json:"ruleKey"`
}

// DangerData FW_005 data 载荷（I-05/D-06：命中详情驱动前端警示与二次确认；
// confirmDanger=true 后重提交放行——保护不阻止操作本身）。
type DangerData struct {
	PanelPort int    `json:"panelPort"`
	RulePort  string `json:"rulePort"`
	Action    string `json:"action"`
	Hint      string `json:"hint"`
}

// DualLayerData FW_007 data 载荷（D-08：两层状态明示；不回滚 permanent）。
type DualLayerData struct {
	Permanent string `json:"permanent"`
	Runtime   string `json:"runtime"`
	Reason    string `json:"reason,omitempty"`
}

// ReasonData FW_008 data 载荷（后端命令失败原因如实携带）。
type ReasonData struct {
	Reason string `json:"reason"`
}
