package dbtools

// ---- DTO（api §4.1.4/§4.2.4；可空纪律：JSON null ≠ TS undefined，可空字段
// 显式 *T 无 omitempty——沿 database-mgmt v2.3 AL-003 先例）----

// DepMissing 依赖缺失项（DBT_001 data.missing / tool/list depsMissing）。
type DepMissing struct {
	Kind string `json:"kind"` // env | extension
	Name string `json:"name"`
}

// ToolItem 工具全景条目（目录+实况+依赖就绪度；探测失败字段 null，D-02/I-11）。
type ToolItem struct {
	ToolType         string       `json:"toolType"`
	DisplayName      string       `json:"displayName"`
	Description      string       `json:"description"`
	TargetEngines    []string     `json:"targetEngines"`
	CatalogVersion   string       `json:"catalogVersion"`
	Installed        bool         `json:"installed"`
	InstalledVersion *string      `json:"installedVersion"`
	BoundInstanceID  *int64       `json:"boundInstanceId"`
	Running          *bool        `json:"running"`
	DepsReady        bool         `json:"depsReady"`
	DepsMissing      []DepMissing `json:"depsMissing"`
	RunningTaskID    *int64       `json:"runningTaskId"`
	// v1.3 additive 可空（D-19/I-14）：siteRef.port/autoCreated 投影——
	// 未装（无档案）=null/null；已装=落点端口（自动块=18090，既有 80/443
	// 块=0）与是否自动创建（旧格式档案缺省 0/false，不虚构非零端口）。
	LandingPort *int  `json:"landingPort"`
	AutoCreated *bool `json:"autoCreated"`
}

// ToolListData GET /dbtool/tool/list 响应。
type ToolListData struct {
	Items       []ToolItem `json:"items"`
	GeneratedAt string     `json:"generatedAt"`
}

// TaskCreatedData install/upgrade/uninstall 响应。
type TaskCreatedData struct {
	TaskID int64  `json:"taskId"`
	Action string `json:"action"`
}

// TaskItem 任务清单项。
type TaskItem struct {
	ID          int64   `json:"id"`
	ToolType    string  `json:"toolType"`
	Action      string  `json:"action"`
	Status      string  `json:"status"`
	InstanceID  *int64  `json:"instanceId"`
	RequestedBy string  `json:"requestedBy"`
	CreatedAt   string  `json:"createdAt"`
	StartedAt   *string `json:"startedAt"`
	FinishedAt  *string `json:"finishedAt"`
	Error       *string `json:"error"`
}

// TaskListData GET /dbtool/task/list 响应。
type TaskListData struct {
	Items []TaskItem `json:"items"`
	Total int        `json:"total"`
}

// TaskDetailData GET /dbtool/task/detail 响应（+输出尾部）。
type TaskDetailData struct {
	TaskItem
	OutputTail string `json:"outputTail"`
}

// OpenData POST /dbtool/access/open 响应（令牌明文仅此次响应下发 admin
// 浏览器用于 signon 重定向；凭据永不下发 I-02）。
type OpenData struct {
	ToolType       string  `json:"toolType"`
	URL            string  `json:"url"`
	TokenExpiresAt *string `json:"tokenExpiresAt"`
}

// RotateData POST /dbtool/access/rotate 响应（无口令字段，零回显）。
type RotateData struct {
	ToolType  string `json:"toolType"`
	RotatedAt string `json:"rotatedAt"`
}

// ToolStatusInfo 进程内 OHS ToolStatusForEngine 返回（5.1：database-mgmt
// 高级管理入口状态源）。
type ToolStatusInfo struct {
	ToolType        string `json:"toolType"`
	Installed       bool   `json:"installed"`
	Running         bool   `json:"running"`
	Openable        bool   `json:"openable"`
	BoundInstanceID *int64 `json:"boundInstanceId"`
}

// SignonVerifyData POST /dbtool/signon/verify 成功响应（仅 loopback 服务端
// 通道，永不进浏览器；+host/port 为 phpMyAdmin 建连必需，tasks.md A-DBT4）。
type SignonVerifyData struct {
	User     string `json:"user"`
	Password string `json:"password"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
}
