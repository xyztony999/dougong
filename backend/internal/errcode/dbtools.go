package errcode

// db-tools 错误码（api-db-tools.md §3.4，DBT_001~012，D-12 新码族裁定；
// 鉴权/CSRF/越界复用 account-auth 码族 AUTH_001/002/003/008，见 auth.go）。
// 内部端点拒绝语义：gate/verify 失败=裸 HTTP 401（无包络，D-11）；
// signon/verify 失败=包络 DBT_007/008/010（HTTP 200）；loopback/秘密头校验
// 失败=裸 HTTP 404（防探测）。任务执行链失败落 task.error 文本分类（非接口码）。
var (
	// ErrDBTDependencyMissing DBT_001 依赖前置未满足（PHP-FPM/Node/nginx 未装
	// 或 PHP 扩展缺失，D-06）；data.missing=[{kind: env|extension, name}]。
	ErrDBTDependencyMissing = New("DBT_001", "依赖前置未满足")
	// ErrDBTInvalidToolType DBT_002 toolType 不在 db_tool_type 两值内（R-14）。
	ErrDBTInvalidToolType = New("DBT_002", "工具类型非法")
	// ErrDBTTaskInFlight DBT_003 同工具在途任务互斥（D-04/I-06）；data.taskId。
	ErrDBTTaskInFlight = New("DBT_003", "该工具已有任务进行中")
	// ErrDBTNotInstalled DBT_004 工具未安装（upgrade/uninstall/open/rotate
	// 前置；install 已装亦此码语义分流升级 D-14）；data.installed。
	ErrDBTNotInstalled = New("DBT_004", "工具未安装")
	// ErrDBTTaskNotFound DBT_005 任务不存在（台账实况）。
	ErrDBTTaskNotFound = New("DBT_005", "任务不存在")
	// ErrDBTInstanceNotReady DBT_006 目标实例未纳管/凭据未配置/引擎不匹配
	// （D-10 InstanceReadyForTool）；data.instanceId?；引导到 database-mgmt。
	ErrDBTInstanceNotReady = New("DBT_006", "目标实例未就绪")
	// ErrDBTTokenInvalid DBT_007 signon 令牌签名篡改/实例绑定不符/会话绑定
	// 不符（D-09 校验①④）。
	ErrDBTTokenInvalid = New("DBT_007", "令牌无效")
	// ErrDBTTokenExpired DBT_008 signon 令牌过期（D-09 校验②，exp 超 60s）。
	ErrDBTTokenExpired = New("DBT_008", "令牌已过期")
	// ErrDBTRotateNotSupported DBT_009 轮换仅 mongo_express 支持（phpmyadmin
	// signon 密钥无轮换入口，D-08）。
	ErrDBTRotateNotSupported = New("DBT_009", "该工具不支持凭据轮换")
	// ErrDBTTokenReplayed DBT_010 signon 令牌已消费（D-09 校验③，一次性防重放）。
	ErrDBTTokenReplayed = New("DBT_010", "令牌已被使用")
	// ErrDBTAlreadyCurrentVersion DBT_011 升级时目录版本=已装版本（D-14，
	// 不虚构升级）。
	ErrDBTAlreadyCurrentVersion = New("DBT_011", "已是目录钉死版本，无需升级")
	// ErrDBTDeployOpFailed DBT_012 部署/轮换操作失败（落点冲突/注入失败/
	// 服务创建失败/文件回滚等）；data.reason。
	ErrDBTDeployOpFailed = New("DBT_012", "部署操作失败")
)
