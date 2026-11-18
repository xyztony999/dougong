package errcode

// database-mgmt（数据库管理）模块错误码（api-database-mgmt.md §3.4 v2.3，
// DBM_001~028 契约码 + DBM_029 引擎操作被拒〔A-DB13 编号裁定〕；鉴权/CSRF/
// 越界/参数校验复用 AUTH 码族）。symbol 即契约 code 列原值，经响应包络
// code 字段上线符号串。
var (
	// ErrDBMInvalidInstanceID DBM_001 instanceId 非法（R-14）。
	ErrDBMInvalidInstanceID = New("DBM_001", "instanceId 非法")
	// ErrDBMInstanceNotFound DBM_002 实例档案不存在（已移除/从未纳管）。
	ErrDBMInstanceNotFound = New("DBM_002", "实例不存在")
	// ErrDBMInstanceUnreachable DBM_003 连接状态 unreachable（治理/观测前置拒绝；
	// data.reason；启停实例归 service-mgmt）。
	ErrDBMInstanceUnreachable = New("DBM_003", "实例连接不可达")
	// ErrDBMCredentialsNotSet DBM_004 实例未配置凭据（data.hint=credentials）。
	ErrDBMCredentialsNotSet = New("DBM_004", "实例未配置凭据")
	// ErrDBMCredentialsInvalid DBM_005 凭据验证失败=认证被拒（data.reason；不落库，D-03）。
	ErrDBMCredentialsInvalid = New("DBM_005", "凭据验证失败：认证被拒")
	// ErrDBMConnTestFailed DBM_006 凭据验证失败=网络/socket 层（data.reason；不落库）。
	ErrDBMConnTestFailed = New("DBM_006", "凭据验证失败：连接失败")
	// ErrDBMDatabaseNotFound DBM_007 目标库不存在（实况校验）。
	ErrDBMDatabaseNotFound = New("DBM_007", "目标数据库不存在")
	// ErrDBMDatabaseExists DBM_008 同名库已存在（实况）。
	ErrDBMDatabaseExists = New("DBM_008", "同名数据库已存在")
	// ErrDBMDBUserNotFound DBM_009 目标用户不存在（实况）。
	ErrDBMDBUserNotFound = New("DBM_009", "目标数据库用户不存在")
	// ErrDBMDBUserExists DBM_010 同名同 host 用户已存在（实况）。
	ErrDBMDBUserExists = New("DBM_010", "同名同 host 用户已存在")
	// ErrDBMGrantNotFound DBM_011 revoke 目标授权关系不存在（实况，D-10）。
	ErrDBMGrantNotFound = New("DBM_011", "授权关系不存在")
	// ErrDBMDumpFailed DBM_012 mysqldump 执行失败（data.reason；不虚构成功）。
	ErrDBMDumpFailed = New("DBM_012", "数据库导出失败")
	// ErrDBMImportFailed DBM_013 导入执行失败（data.reason；不自动回滚）。
	ErrDBMImportFailed = New("DBM_013", "数据库导入失败")
	// ErrDBMDumpSizeExceeded DBM_014 导入文件超过 512MiB（D-06）。
	ErrDBMDumpSizeExceeded = New("DBM_014", "导入文件超过大小上限（512MiB）")
	// ErrDBMKeyNotFound DBM_015 TTL 查询的 key 不存在（实况；过期即消失）。
	ErrDBMKeyNotFound = New("DBM_015", "key 不存在")
	// ErrDBMInstanceBusy DBM_016 实例有导出/导入在途（D-12 内存任务表）。
	ErrDBMInstanceBusy = New("DBM_016", "实例有导出/导入任务在途，请稍后重试")
	// ErrDBMInvalidPattern DBM_017 key 匹配模式非法（R-14；禁 KEYS 全量语义）。
	ErrDBMInvalidPattern = New("DBM_017", "key 匹配模式非法")
	// ErrDBMEngineMismatch DBM_018 接口与实例引擎不符（I-05 前置链）。
	ErrDBMEngineMismatch = New("DBM_018", "接口与实例引擎不符")
	// ErrDBMCredSyncFailed DBM_019 引擎侧改密成功、面板密文同步失败（D-05；
	// data.recoverHint=credentials；不回滚引擎侧）。
	ErrDBMCredSyncFailed = New("DBM_019", "密码已修改，但面板凭据同步失败")
	// ErrDBMIdentityConflict DBM_020 连接目标纠正后 identity_key 冲突（I-01/D-03）。
	ErrDBMIdentityConflict = New("DBM_020", "连接目标与既有实例档案冲突")
	// ErrDBMChannelUnavailable DBM_021 引擎客户端通道不可用（mysql/redis-cli
	// 二进制未检出；data.reason。区别于 DBM_006 网络不可达——实例可能正常，
	// 是宿主机缺客户端，v0.1.4 凭据 INTERNAL bug 修复轮补齐）。
	ErrDBMChannelUnavailable = New("DBM_021", "引擎客户端通道不可用（宿主机未安装 mysql/redis-cli）")
	// ErrDBMTableNotFound DBM_022 目标表不存在（实况校验；table/structure、
	// table/drop、row/* 前置，api v2.3 §3.4）。
	ErrDBMTableNotFound = New("DBM_022", "目标表不存在")
	// ErrDBMNoSinglePK DBM_023 表无单列主键，行写禁用（I-10/D-22；data.reason
	// 如实提示——无主键/复合主键；服务端为最终守卫）。
	ErrDBMNoSinglePK = New("DBM_023", "该表无单列主键，不支持行级编辑/删除")
	// ErrDBMRowNotFound DBM_024 行定位失败（WHERE 主键=旧值匹配 0 行——行已
	// 删除或主键值已被面板外修改；不静默成功）。
	ErrDBMRowNotFound = New("DBM_024", "行已不存在或主键值已被修改")
	// ErrDBMCollectionNotFound DBM_025 目标集合不存在（实况校验；mcoll/drop、
	// mdoc/* 前置）。
	ErrDBMCollectionNotFound = New("DBM_025", "目标集合不存在")
	// ErrDBMInvalidDocumentJSON DBM_026 文档 JSON 非法（服务端校验 R-14；
	// mdoc/insert、mdoc/update）。
	ErrDBMInvalidDocumentJSON = New("DBM_026", "文档 JSON 非法")
	// ErrDBMDocumentNotFound DBM_027 目标文档不存在（_id 定位匹配 0——已删除/
	// 从未存在；mdoc/update、mdoc/delete；不静默成功）。
	ErrDBMDocumentNotFound = New("DBM_027", "文档已不存在")
	// ErrDBMSystemDatabaseProtected DBM_028 Mongo 系统库（admin/config/local）
	// 删除保护（I-12/D-25；mdb/dropdb 服务端硬校验）。
	ErrDBMSystemDatabaseProtected = New("DBM_028", "系统库（admin/config/local）不可删除")
	// ErrDBMEngineUnauthorized DBM_029 引擎操作被拒（凭据账户权限不足——
	// MongoDB Unauthorized（code 13）类；data.reason。区别于 DBM_005 认证被拒
	// （身份错）——这是身份对但无权执行该命令）。
	// 编号沿革（A-DB13）：v0.1.4 修复轮原占 DBM_022；api v2.3 §3.4 定稿
	// DBM_022=TABLE_NOT_FOUND，本码续接空号改 DBM_029（语义不变，前端映射
	// 须同步——见 backend/tasks.md 增量轮裁定表）。
	ErrDBMEngineUnauthorized = New("DBM_029", "引擎操作被拒（凭据账户权限不足）")
)
