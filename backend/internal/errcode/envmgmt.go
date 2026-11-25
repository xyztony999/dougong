package errcode

// env-mgmt 错误码（api-env-mgmt.md §3.4，ENV_001~006；鉴权/CSRF/越界复用
// account-auth 码族——AUTH_001/002/003/008，见 auth.go）。
var (
	// ErrEnvChannelUnavailable ENV_001 包管理器通道不可用（非 Linux / apt 与
	// dnf·yum 皆未检出，D-03）；data.reason；变更接口前置拒绝，列表不报错。
	ErrEnvChannelUnavailable = New("ENV_001", "包管理器通道不可用")
	// ErrEnvInvalidType ENV_002 envType 不在 env_type 六值目录内（R-14）。
	ErrEnvInvalidType = New("ENV_002", "环境类型非法")
	// ErrEnvTaskInFlight ENV_003 同环境在途任务互斥（D-04/I-01）；data.taskId。
	ErrEnvTaskInFlight = New("ENV_003", "该环境已有任务进行中")
	// ErrEnvNotInstalled ENV_004 环境未安装（uninstall/service 前置）；data.installed。
	ErrEnvNotInstalled = New("ENV_004", "环境未安装")
	// ErrEnvTaskNotFound ENV_005 任务不存在（台账实况）。
	ErrEnvTaskNotFound = New("ENV_005", "任务不存在")
	// ErrEnvUnitOpFailed ENV_006 systemd 服务操作失败（D-09）；data.reason。
	ErrEnvUnitOpFailed = New("ENV_006", "服务操作失败")
	// ErrEnvConfigUnavailable ENV_007 配置文件不存在或环境未安装，无法读写。
	ErrEnvConfigUnavailable = New("ENV_007", "配置文件不可用")
	// ErrEnvConfigInvalid ENV_008 配置写入后校验失败（如 nginx -t）或取值不合法。
	ErrEnvConfigInvalid = New("ENV_008", "配置未能保存")
)
