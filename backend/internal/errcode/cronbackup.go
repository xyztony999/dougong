package errcode

// cron-backup（计划任务与备份）模块错误码（api-cron-backup.md §3.4，
// CB_001~009 共 9 个新增；鉴权/CSRF/越界/绑定复用 AUTH 码族）。
// symbol 即契约 code 列原值，经响应包络 code 字段上线符号串。
var (
	// ErrCBValidation CB_001 参数错误；data.fields[] 逐字段错误（R-14）。
	ErrCBValidation = New("CB_001", "参数错误")
	// ErrCronExprInvalid CB_002 表达式解析失败；data.errors[] 段位级错误（D-03）。
	ErrCronExprInvalid = New("CB_002", "cron 表达式非法")
	// ErrCronJobNotFound CB_003 计划任务不存在（面板外已删——如实报错不静默，D-01）。
	ErrCronJobNotFound = New("CB_003", "计划任务不存在")
	// ErrBackupJobNotFound CB_004 备份任务不存在。
	ErrBackupJobNotFound = New("CB_004", "备份任务不存在")
	// ErrBackupJobNameTaken CB_005 任务名称已存在（I-06）。
	ErrBackupJobNameTaken = New("CB_005", "任务名称已存在")
	// ErrBackupJobRunning CB_006 同任务已有执行进行中（I-01 手动侧，D-08）。
	ErrBackupJobRunning = New("CB_006", "该任务已有备份执行进行中")
	// ErrBackupRunNotFound CB_007 执行记录不存在。
	ErrBackupRunNotFound = New("CB_007", "执行记录不存在")
	// ErrRunNotDownloadable CB_008 非 success 状态（failure/running 不可下载，I-04）。
	ErrRunNotDownloadable = New("CB_008", "仅成功的执行可下载产物")
	// ErrArtifactMissing CB_009 产物文件缺失（面板外被删——记录为历史事实不回改，PRD N2）。
	ErrArtifactMissing = New("CB_009", "产物文件缺失")
)
