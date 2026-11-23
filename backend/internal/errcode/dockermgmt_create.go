package errcode

// docker-mgmt v1.1 创建类错误码（api-docker-mgmt.md v1.1 §3.4 DK_019~027；
// 编号全序唯一，daemon 拒绝语义归一沿 D-08 既有映射）。
var (
	// ErrDKContainerNameInvalid DK_019 容器名格式非法（D-09 白名单，R-14）。
	ErrDKContainerNameInvalid = New("DK_019", "容器名格式非法")
	// ErrDKPortMappingInvalid DK_020 端口映射格式非法（R-14）。
	ErrDKPortMappingInvalid = New("DK_020", "端口映射格式非法")
	// ErrDKEnvFormatInvalid DK_021 环境变量格式非法（须 KEY=VALUE）。
	ErrDKEnvFormatInvalid = New("DK_021", "环境变量格式非法")
	// ErrDKMountFormatInvalid DK_022 挂载格式非法（source:target[:mode]）。
	ErrDKMountFormatInvalid = New("DK_022", "挂载格式非法")
	// ErrDKContainerCreateFailed DK_023 容器创建/启动失败（data.reason 透传）。
	ErrDKContainerCreateFailed = New("DK_023", "容器创建失败")
	// ErrDKVolumeCreateFailed DK_024 卷创建失败（data.reason 透传）。
	ErrDKVolumeCreateFailed = New("DK_024", "卷创建失败")
	// ErrDKNetworkCreateFailed DK_025 网络创建失败（data.reason 透传）。
	ErrDKNetworkCreateFailed = New("DK_025", "网络创建失败")
	// ErrDKComposeYAMLInvalid DK_026 compose.yml 语法校验失败（data.reason 带行号）。
	ErrDKComposeYAMLInvalid = New("DK_026", "compose.yml 校验失败")
	// ErrDKComposeProjectExists DK_027 项目已存在（落盘目录已存在）。
	ErrDKComposeProjectExists = New("DK_027", "Compose 项目已存在")
)

// docker-mgmt v1.2 面板应用与 Compose 生命周期错误码（ADR-033，api-docker-mgmt
// v1.2 §C；应用重名复用 DK_027）。
var (
	// ErrDKAppNotFound DK_028 应用不存在（可能已在面板外删除）。
	ErrDKAppNotFound = New("DK_028", "应用不存在")
	// ErrDKAppNameInvalid DK_029 应用名格式非法（小写字母/数字/连字符）。
	ErrDKAppNameInvalid = New("DK_029", "应用名格式非法")
	// ErrDKAppOpFailed DK_030 应用操作失败（创建/启停/删除；data.reason 透传）。
	ErrDKAppOpFailed = New("DK_030", "应用操作失败")
	// ErrDKAppPortInUse DK_031 主机端口已被占用。
	ErrDKAppPortInUse = New("DK_031", "主机端口已被占用")
	// ErrDKComposeDeleteFailed DK_032 Compose 项目删除失败（data.reason 透传）。
	ErrDKComposeDeleteFailed = New("DK_032", "Compose 项目删除失败")
)
