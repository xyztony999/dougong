// dto.go 承载契约响应载荷（api §4.1.4/§4.2.4/§4.3.4 各接口 data 形状；
// 字段名/可空性与设计文档逐形状对齐，禁止自行增删）。
package dockermgmt

// DaemonStatusData 是 GET /docker/daemon/status 载荷（D-01：即时探测结论）。
type DaemonStatusData struct {
	State       string  `json:"state"`
	Version     *string `json:"version"`
	Detail      *string `json:"detail"`
	GeneratedAt string  `json:"generatedAt"`
}

// ContainerItem 是容器投影条目（§4.1.1 字段清单；composeProject 无项目为
// null；supportedActions 权威动作面）。
type ContainerItem struct {
	ContainerID      string   `json:"containerId"`
	Name             string   `json:"name"`
	Image            string   `json:"image"`
	State            string   `json:"state"`
	ComposeProject   *string  `json:"composeProject"`
	SupportedActions []string `json:"supportedActions"`
}

// DaemonSummary 是 container/list 随行 daemon 摘要（页头呈现，宿主 S-45-01）。
type DaemonSummary struct {
	State   string  `json:"state"`
	Version *string `json:"version"`
}

// ContainerListData 是 GET /docker/container/list 载荷（全量+筛选+排序 D-02）。
type ContainerListData struct {
	Items       []ContainerItem `json:"items"`
	Total       int             `json:"total"`
	Daemon      DaemonSummary   `json:"daemon"`
	GeneratedAt string          `json:"generatedAt"`
}

// LogsData 是 logs 载荷（D-03：JSON 行数组）。
type LogsData struct {
	Lines       []string `json:"lines"`
	GeneratedAt string   `json:"generatedAt"`
}

// StatsData 是 stats 载荷（D-07 单次快照）。
type StatsData struct {
	CPUPercent   float64 `json:"cpuPercent"`
	MemUsageByte uint64  `json:"memUsageBytes"`
	MemLimitByte uint64  `json:"memLimitBytes"`
	MemPercent   float64 `json:"memPercent"`
	GeneratedAt  string  `json:"generatedAt"`
}

// ActionStateData 是 DK_004 data 载荷（I-04/D-04： currentState+action+
// allowedActions=该容器权威动作面）。
type ActionStateData struct {
	CurrentState   string   `json:"currentState"`
	Action         string   `json:"action"`
	AllowedActions []string `json:"allowedActions"`
}

// LocateFailedData 是 DK_015 data 载荷（D1 口径-2/D-06：label 记录路径透传）。
type LocateFailedData struct {
	WorkingDir string `json:"workingDir"`
	ConfigFile string `json:"configFile"`
	Missing    string `json:"missing"`
}

// ContainerActionData 是生命周期动作成功载荷（操作后即读实况，§4.1.4）。
type ContainerActionData struct {
	ContainerID  string `json:"containerId"`
	CurrentState string `json:"currentState"`
}

// ContainerDeleteData 是 delete 成功载荷（无 force，I-04/I-05）。
type ContainerDeleteData struct {
	Deleted     bool   `json:"deleted"`
	ContainerID string `json:"containerId"`
}

// ImageItem 是镜像投影条目（§4.2.1）。
type ImageItem struct {
	ImageID   string `json:"imageId"`
	Ref       string `json:"ref"`
	SizeBytes int64  `json:"sizeBytes"`
}

// ImageListData 是 GET /docker/image/list 载荷。
type ImageListData struct {
	Items       []ImageItem `json:"items"`
	Total       int         `json:"total"`
	GeneratedAt string      `json:"generatedAt"`
}

// ImagePullData 是 pull 成功载荷（D-05；ref 回显请求原值 A-D7）。
type ImagePullData struct {
	ImageID string `json:"imageId"`
	Ref     string `json:"ref"`
}

// ImageDeleteData 是镜像删除成功载荷。
type ImageDeleteData struct {
	Deleted bool   `json:"deleted"`
	ImageID string `json:"imageId"`
}

// VolumeItem 是卷投影条目（§4.2.1）。
type VolumeItem struct {
	VolumeName string `json:"volumeName"`
	Mountpoint string `json:"mountpoint"`
	SizeBytes  int64  `json:"sizeBytes"`
}

// VolumeListData 是 GET /docker/volume/list 载荷。
type VolumeListData struct {
	Items       []VolumeItem `json:"items"`
	Total       int          `json:"total"`
	GeneratedAt string       `json:"generatedAt"`
}

// VolumeDeleteData 是卷删除成功载荷。
type VolumeDeleteData struct {
	Deleted    bool   `json:"deleted"`
	VolumeName string `json:"volumeName"`
}

// NetworkItem 是网络投影条目（§4.2.1；预定义网络如实呈现）。
type NetworkItem struct {
	NetworkID string `json:"networkId"`
	Name      string `json:"name"`
	Driver    string `json:"driver"`
	Subnet    string `json:"subnet"`
}

// NetworkListData 是 GET /docker/network/list 载荷。
type NetworkListData struct {
	Items       []NetworkItem `json:"items"`
	Total       int           `json:"total"`
	GeneratedAt string        `json:"generatedAt"`
}

// NetworkDeleteData 是网络删除成功载荷。
type NetworkDeleteData struct {
	Deleted   bool   `json:"deleted"`
	NetworkID string `json:"networkId"`
}

// ComposeContainerItem 是项目容器清单条目（§4.3.1 containers）。
type ComposeContainerItem struct {
	ContainerID string `json:"containerId"`
	Name        string `json:"name"`
	State       string `json:"state"`
}

// ComposeItem 是项目投影条目（§4.3.1；workingDir/configFile label 透传，
// 空为 null；locatable 服务端定位判定）。
type ComposeItem struct {
	Project    string                 `json:"project"`
	WorkingDir *string                `json:"workingDir"`
	ConfigFile *string                `json:"configFile"`
	Containers []ComposeContainerItem `json:"containers"`
	Locatable  bool                   `json:"locatable"`
}

// ComposeListData 是 GET /docker/compose/list 载荷。
type ComposeListData struct {
	Items       []ComposeItem `json:"items"`
	Total       int           `json:"total"`
	GeneratedAt string        `json:"generatedAt"`
}

// ComposeActionData 是 up/down/restart 成功载荷（D-06）。
type ComposeActionData struct {
	Project string `json:"project"`
	Verb    string `json:"verb"`
	Done    bool   `json:"done"`
}
