package errcode

// docker-mgmt 模块错误码（api-docker-mgmt.md §3.4，DK_001~018 共 18 个；
// 鉴权/CSRF/越界/绑定复用 AUTH 码族）。symbol 即契约 code 列原值，经响应
// 包络 code 字段上线符号串。data 载荷：DK_004={currentState,action,
// allowedActions}、DK_013/016/017={reason}、DK_015={workingDir,configFile,
// missing}、DK_018={busy:true}。
var (
	// ErrDkInvalidParam DK_001 参数错误（containerId/imageId/volumeName/networkId/project/tail 非空与取值；R-14/D-11）。
	ErrDkInvalidParam = New("DK_001", "参数错误")
	// ErrDkDaemonUnavailable DK_002 daemon 接管判定 unavailable 时访问业务面（不返回空清单——未装≠无容器，I-01/D-01）。
	ErrDkDaemonUnavailable = New("DK_002", "Docker 未就绪（未安装或不可达）")
	// ErrDkContainerNotFound DK_003 容器不存在（面板外已删，不静默成功）。
	ErrDkContainerNotFound = New("DK_003", "容器不存在（可能已在面板外被删除）")
	// ErrDkActionStateMismatch DK_004 动作与当前状态不匹配（BA 附录B.2 矩阵；data={currentState,action,allowedActions}，I-04/D-04）。
	ErrDkActionStateMismatch = New("DK_004", "操作与容器当前状态不匹配")
	// ErrDkImageNotFound DK_005 镜像不存在（删除目标）。
	ErrDkImageNotFound = New("DK_005", "镜像不存在")
	// ErrDkImageInUse DK_006 镜像被容器引用（daemon 拒绝透传，D-08）。
	ErrDkImageInUse = New("DK_006", "镜像正被容器引用，无法删除")
	// ErrDkVolumeNotFound DK_007 卷不存在。
	ErrDkVolumeNotFound = New("DK_007", "卷不存在")
	// ErrDkVolumeInUse DK_008 卷被容器挂载（daemon 拒绝透传）。
	ErrDkVolumeInUse = New("DK_008", "卷正被容器挂载，无法删除")
	// ErrDkNetworkNotFound DK_009 网络不存在。
	ErrDkNetworkNotFound = New("DK_009", "网络不存在")
	// ErrDkNetworkInUse DK_010 网络被容器接入（daemon 拒绝透传）。
	ErrDkNetworkInUse = New("DK_010", "网络正被容器接入，无法删除")
	// ErrDkNetworkPredefined DK_011 预定义网络不可删除（bridge/host/none 等，daemon 拒绝语义归类，D-08）。
	ErrDkNetworkPredefined = New("DK_011", "预定义网络不可删除")
	// ErrDkImageRefInvalid DK_012 镜像引用格式非法（名称[:tag] 服务端格式校验，R-14）。
	ErrDkImageRefInvalid = New("DK_012", "镜像引用格式非法（应为 名称[:tag]）")
	// ErrDkPullFailed DK_013 拉取失败（data.reason 透传 daemon 错误：tag 不存在/网络不通/限流等，D-05/D-08）。
	ErrDkPullFailed = New("DK_013", "镜像拉取失败")
	// ErrDkComposeProjectNotFound DK_014 项目不存在（label 聚合无此项目名）。
	ErrDkComposeProjectNotFound = New("DK_014", "Compose 项目不存在")
	// ErrDkComposeLocateFailed DK_015 定位失败（工作目录已移除/配置文件缺失；data={workingDir,configFile,missing}，D-06）。
	ErrDkComposeLocateFailed = New("DK_015", "Compose 项目定位失败（工作目录或配置文件缺失）")
	// ErrDkComposeExecFailed DK_016 compose CLI 执行失败（data.reason 透传 stderr；含超时，I-03）。
	ErrDkComposeExecFailed = New("DK_016", "Compose 命令执行失败")
	// ErrDkDaemonExecFailed DK_017 SDK 操作失败（daemon 内部错误，data.reason 原文透传不虚构，D-08）。
	ErrDkDaemonExecFailed = New("DK_017", "Docker 操作失败")
	// ErrDkComposeBusy DK_018 同项目 up/down/restart 进行中（进程内 per-project 串行互斥，data.busy=true，D-06/I-08）。
	ErrDkComposeBusy = New("DK_018", "该项目已有 Compose 操作进行中")
)
