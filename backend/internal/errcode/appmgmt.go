package errcode

// app-mgmt 错误码（api-app-mgmt.md §3.4，APP_001~012；鉴权/CSRF/越界复用
// account-auth 码族——AUTH_001/002/003/008，见 auth.go）。
var (
	// ErrAppInvalidName APP_001 应用名非法（格式约束 ^[a-z0-9][a-z0-9-]{0,31}$，D-03）。
	ErrAppInvalidName = New("APP_001", "应用名非法")
	// ErrAppNameTaken APP_002 应用名已存在（I-02 唯一性）；data.appId。
	ErrAppNameTaken = New("APP_002", "应用名已存在")
	// ErrAppDirInvalid APP_003 代码目录或入口文件不存在（D-03；data.reason）。
	ErrAppDirInvalid = New("APP_003", "代码目录或入口文件不存在")
	// ErrAppEnvMissing APP_004 Python 环境就绪度不足（D-08）；data.missing 分层。
	ErrAppEnvMissing = New("APP_004", "Python 环境未就绪")
	// ErrAppPortOccupied APP_005 监听端口被占用（D-03）；data.port。
	ErrAppPortOccupied = New("APP_005", "端口已被占用")
	// ErrAppTaskInFlight APP_006 同应用在途任务互斥（D-04/I-05）；data.taskId。
	ErrAppTaskInFlight = New("APP_006", "该应用已有任务进行中")
	// ErrAppNotFound APP_007 应用档案不存在（deps/delete/params/service/log/site 前置）。
	ErrAppNotFound = New("APP_007", "应用不存在")
	// ErrAppTaskNotFound APP_008 任务不存在（台账实况）。
	ErrAppTaskNotFound = New("APP_008", "任务不存在")
	// ErrAppVenvMissing APP_009 venv 不存在（deps 前置 D-05）。
	ErrAppVenvMissing = New("APP_009", "运行环境（venv）缺失")
	// ErrAppUnitOpFailed APP_010 systemd 服务操作失败（D-10）；data.reason。
	ErrAppUnitOpFailed = New("APP_010", "服务操作失败")
	// ErrAppSiteLinkFailed APP_011 建站联动失败（D-12）；data.reason（site-mgmt 语义透传）。
	ErrAppSiteLinkFailed = New("APP_011", "建站联动失败")
	// ErrAppParamsInvalid APP_012 参数校验失败（基线不一致/行格式/白名单，D-06/D-09）；data.reason。
	ErrAppParamsInvalid = New("APP_012", "参数校验失败")
)

// ErrAppImageInvalid APP_013 镜像引用格式非法（容器化形态校验，R-14；
// 复用 DK_012 同规则——名称[:tag] 服务端格式校验）。
var ErrAppImageInvalid = New("APP_013", "镜像引用格式非法")
