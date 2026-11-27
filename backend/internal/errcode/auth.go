package errcode

// account-auth 模块错误码（api-account-auth.md §3.4，AUTH_001~AUTH_014）。
// symbol 即契约 code 列原值，直接经响应包络 code 字段上线（数值码不使用）。
var (
	// ErrUnauthenticated AUTH_001 未登录（无/无效会话）。
	ErrUnauthenticated = New("AUTH_001", "未登录或会话无效")
	// ErrSessionExpired AUTH_002 会话过期（绝对或空闲到期，前端转登录）。
	ErrSessionExpired = New("AUTH_002", "会话已过期，请重新登录")
	// ErrCSRF AUTH_003 CSRF 校验失败（token 缺失/不匹配或非同源）。
	ErrCSRF = New("AUTH_003", "请求被拒绝：CSRF 校验失败")
	// ErrAccountLocked AUTH_004 登录锁定中；data.remainingSeconds。
	ErrAccountLocked = New("AUTH_004", "账户已锁定，请稍后重试")
	// ErrBadCredentials AUTH_005 用户名或密码错误（不区分提示，防枚举）。
	ErrBadCredentials = New("AUTH_005", "用户名或密码错误")
	// ErrTotpInvalid AUTH_006 动态码错误（计一次失败）。
	ErrTotpInvalid = New("AUTH_006", "动态码错误")
	// ErrTotpRequired AUTH_007 需要二次验证；data.loginTicket。
	ErrTotpRequired = New("AUTH_007", "需要二次验证")
	// ErrForbidden AUTH_008 权限不足（readonly 越界/非 admin）。
	ErrForbidden = New("AUTH_008", "权限不足")
	// ErrAlreadyInitialized AUTH_009 重复初始化/初始化前拦截（I-01/I-02）。
	ErrAlreadyInitialized = New("AUTH_009", "初始化状态不允许该操作")
	// ErrLastAdmin AUTH_010 唯一管理员保护（I-03）。
	ErrLastAdmin = New("AUTH_010", "不能删除最后一个管理员")
	// ErrUsernameTaken AUTH_011 用户名已存在（I-06）。
	ErrUsernameTaken = New("AUTH_011", "用户名已存在")
	// ErrValidation AUTH_012 参数校验失败；data.fields[]。
	ErrValidation = New("AUTH_012", "参数错误")
	// ErrTicketInvalid AUTH_013 login_ticket 无效/过期/已使用。
	ErrTicketInvalid = New("AUTH_013", "二次验证凭证无效或已过期")
	// ErrSelfDelete AUTH_014 不能删除当前登录账户（D-09）。
	ErrSelfDelete = New("AUTH_014", "不能删除当前登录账户")
	// ErrWebAuthnInvalid AUTH_015 passkey 校验失败（注册/断言）。
	ErrWebAuthnInvalid = New("AUTH_015", "passkey 校验失败，请重试")
	// ErrRecoveryInvalid AUTH_016 恢复码无效或已使用（计一次失败）。
	ErrRecoveryInvalid = New("AUTH_016", "恢复码无效或已使用")
	// ErrCredentialNotFound AUTH_017 passkey 凭据不存在。
	ErrCredentialNotFound = New("AUTH_017", "passkey 不存在")
	// ErrWebAuthnUnavailable AUTH_018 当前访问地址不支持 passkey（需 HTTPS/有效域名）。
	ErrWebAuthnUnavailable = New("AUTH_018", "当前访问地址不支持 passkey，请改用 HTTPS 域名访问")
)

// 跨模块公共错误标识（前端 http.ts 对 NOT_FOUND 有既有处理分支）。
var (
	// ErrNotFound 通用资源不存在（NoRoute）。
	ErrNotFound = New("NOT_FOUND", "资源或接口不存在")
	// ErrInternal 未分类错误的对外脱敏表达（细节只进日志）。
	ErrInternal = New("INTERNAL", "内部错误")
)
