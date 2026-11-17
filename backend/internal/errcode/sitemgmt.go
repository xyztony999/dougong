package errcode

// site-mgmt（网站管理）模块错误码（api-site-mgmt.md §3.4，SITE_001~014 +
// CERT_001~007 共 21 个新增；鉴权/CSRF/越界/参数绑定复用 AUTH 码族）。
// symbol 即契约 code 列原值，经响应包络 code 字段上线符号串。
var (
	// ErrSiteInvalidID SITE_001 siteId 形态非法（非「文件路径#序号」结构，R-14/D-02）。
	ErrSiteInvalidID = New("SITE_001", "siteId 形态非法（须为 文件路径#序号）")
	// ErrSiteNotFound SITE_002 站点不存在（扫描实况校验：文件已删/server 块已移除/序号越界，D-01）。
	ErrSiteNotFound = New("SITE_002", "站点不存在")
	// ErrNginxTestFailed SITE_003 `nginx -t` 校验失败且写回已自动回滚（R-06）；data.errors[]+data.diff。
	ErrNginxTestFailed = New("SITE_003", "nginx -t 校验失败，写回已自动回滚")
	// ErrReloadFailed SITE_004 reload 失败：配置保留（D-05，nginx -t 已通过）；data.output。
	ErrReloadFailed = New("SITE_004", "reload 失败（配置已保留，请检查环境后手动重试）")
	// ErrSiteExists SITE_005 同主域名+监听端口的 server 块已存在（D-15）；data.existingSiteId。
	ErrSiteExists = New("SITE_005", "同主域名与监听端口的站点已存在")
	// ErrSiteInvalidListen SITE_006 监听端口非法（非 1~65535/与地址组合形态非法，R-14）。
	ErrSiteInvalidListen = New("SITE_006", "监听声明非法（端口须为 1~65535）")
	// ErrSiteInvalidDomain SITE_007 server_name 形态非法（空/含空格/通配符越界，R-14）。
	ErrSiteInvalidDomain = New("SITE_007", "域名形态非法")
	// ErrSiteInvalidUpstream SITE_008 反代上游地址非法（非 http(s)://host[:port] 形态，R-14）。
	ErrSiteInvalidUpstream = New("SITE_008", "反代上游地址非法（须为 http(s)://host[:port]）")
	// ErrPoolNotFound SITE_009 指定 php-fpm pool 不存在（实况校验，D-14）。
	ErrPoolNotFound = New("SITE_009", "指定的 php-fpm pool 不存在")
	// ErrSourceStale SITE_010 源码编辑基线过期（D-06）；data.currentMtime，重新加载后编辑。
	ErrSourceStale = New("SITE_010", "源码基线已过期（文件已被外部修改，请重新加载）")
	// ErrLogNotFound SITE_011 日志文件不存在/不可读（⚠️A15 回退链穷尽，D-07）。
	ErrLogNotFound = New("SITE_011", "日志文件不存在或不可读")
	// ErrNginxUnavailable SITE_012 nginx 未检出或 `nginx -t` 通道不可用（D-15 空态）；data.reason。
	ErrNginxUnavailable = New("SITE_012", "nginx 未检出或通道不可用")
	// ErrSiteBackupFailed SITE_013 写前快照失败：本次写入整体不执行（FR-01，配置未动）；data.reason。
	ErrSiteBackupFailed = New("SITE_013", "写前备份失败，本次写入未执行")
	// ErrSiteInvalidSource SITE_014 源码内容非法：超 1MiB 上限或非 UTF-8（R-14）。
	ErrSiteInvalidSource = New("SITE_014", "源码内容非法（超 1MiB 或非 UTF-8）")
	// ErrCertNotFound CERT_001 证书不存在（certificate 表实况）。
	ErrCertNotFound = New("CERT_001", "证书不存在")
	// ErrCertInvalidState CERT_002 证书状态不允许该操作；data.currentStatus。
	ErrCertInvalidState = New("CERT_002", "证书当前状态不允许该操作")
	// ErrCertDuplicate CERT_003 同域名集合已有 acme 源 active 或任意来源 pending
	// 证书（D-10 v2.1 接管语义：imported 源放行=ACME 接管入口）；data.existingCertId。
	ErrCertDuplicate = New("CERT_003", "同域名集合已有生效中的 ACME 证书或进行中申请")
	// ErrAcmeFailed CERT_004 ACME 签发/续期失败（挑战不通/CA 拒绝/超时）；data.reason。
	ErrAcmeFailed = New("CERT_004", "ACME 签发/续期失败")
	// ErrCertReferenced CERT_005 证书仍被站点引用，先解除部署（D-11）；data.sites[]。
	ErrCertReferenced = New("CERT_005", "证书仍被站点引用，请先解除部署")
	// ErrUndeployNotDeployed CERT_006 该站点当前未部署此证书（I-05 引用一致性校验）。
	ErrUndeployNotDeployed = New("CERT_006", "该站点未部署此证书")
	// ErrInvalidChallenge CERT_007 challenge 不在 acme_challenge 取值内（R-14；v2.0 扩 dns_01）。
	ErrInvalidChallenge = New("CERT_007", "challenge 取值非法（http_01|tls_alpn|dns_01）")
	// ErrCertInvalidPEM CERT_008 certPem 非法：x509 PEM 解析失败/超 1MiB（R-14，v1.2，cert/replace）。
	ErrCertInvalidPEM = New("CERT_008", "证书 PEM 非法（解析失败或超 1MiB）")
	// ErrDnsCredentialNotFound CERT_009 DNS 凭证不存在或已删除（v2.0 ⚠️A20；request dns_01 前置与凭证接口实况）。
	ErrDnsCredentialNotFound = New("CERT_009", "DNS 凭证不存在")
	// ErrCertInvalidDomainInput CERT_010 域名输入非法（v2.0）：siteId/domains 二选一违反；domains 形态非法（泛域名仅 *.domain 前缀形态）。
	ErrCertInvalidDomainInput = New("CERT_010", "域名输入非法（siteId 与 domains 二选一；泛域名须为 *.domain 形态）")
)
