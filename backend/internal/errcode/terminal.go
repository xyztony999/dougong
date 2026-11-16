package errcode

// terminal 模块错误码（api-terminal.md §3.4，TERM_001~TERM_007）。
// symbol 即契约 code 列原值；WS 握手拒绝经响应包络 + HTTP 状态上线
// （api-terminal.md §3.1「升级前仍是 HTTP 语义」）。
var (
	// ErrConcurrentLimit TERM_001 同操作者存续终端连接已达上限 5；
	// data={limit, current}。
	ErrConcurrentLimit = New("TERM_001", "终端连接数已达上限，请先关闭既有会话")
	// ErrSystemUserNotFound TERM_002 指定的系统用户不存在（R-14 兜底校验）。
	ErrSystemUserNotFound = New("TERM_002", "指定的系统用户不存在")
	// ErrNoLoginShell TERM_003 指定用户的 shell 非交互（nologin/false 等）。
	ErrNoLoginShell = New("TERM_003", "指定用户的 shell 不允许登录")
	// ErrPtySpawnFailed TERM_004 PTY 进程创建失败；不产生会话对象、不占并发配额。
	ErrPtySpawnFailed = New("TERM_004", "终端进程创建失败")
	// ErrValidation TERM_005 参数校验失败；data.fields[]（R-14）。
	ErrTerminalValidation = New("TERM_005", "参数错误")
	// ErrAuditUnavailable TERM_006 建立审计写入失败→从严拒建（D-06）。
	ErrAuditUnavailable = New("TERM_006", "审计服务不可用，已拒绝建立终端会话")
	// ErrSystemUserDirUnavailable TERM_007 系统用户实况读取失败（不返回虚构数据）。
	ErrSystemUserDirUnavailable = New("TERM_007", "系统用户实况读取失败")
)
