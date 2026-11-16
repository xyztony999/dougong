// Package sitemgmt 是网站管理（site-mgmt，核心卖点）应用服务（api-site-mgmt.md）：
// SiteScanner 实时扫描投影（D-01）、SiteProjector 组装、SiteEditService/
// SiteCreateService/SiteToggleService 结构化写回（D-16）、SourceEditService
// 乐观锁源码编辑（D-06）、LogTailService 日志尾部（D-07）、WritePipeline 安全
// 写入管线（R-06 四步 + R-16 隔离，D-04）、CertStatusCalculator 读时推算
// （D-08）、CertReferencedResolver 引用推导（D-11）、证书六接口编排与
// RenewalScheduler 每日续期调度（4.1.3）。
//
// site 为配置文件实时投影零持久化（I-07）；certificate 为唯一持久化聚合根
// （store/sitemgmt）。文件系统侧动作审计走 AuditService.Record（D-12 常规），
// certificate 表写与审计同事务（D-12 例外）。
package sitemgmt

import "time"

// 业务常量（api-site-mgmt.md §3.4 操作参数与阈值，编译期常量非配置键）。
const (
	// CertExpiringDays expiring_soon 推算阈值（ADR-024 拍板固定不可配，D-08）。
	CertExpiringDays = 30
	// LogTailDefault / LogTailMax 日志尾部默认行数/上限（D-07）。
	LogTailDefault = 100
	LogTailMax     = 1000
	// ACMETimeout 签发/续期同步等待上限，超时转 renew_failed（D-09）。
	ACMETimeout = 120 * time.Second
	// ConfBackupKeep 每来源文件 conf.backup 快照保留份数（D-04）。
	ConfBackupKeep = 20
	// SourceSizeLimit 源码编辑大小上限（SITE_014；file-manager D-05 同值先例）。
	SourceSizeLimit = 1 << 20
	// NewSiteFilePrefix 新建站点配置文件名前缀（D-03）。
	NewSiteFilePrefix = "dougong-"
	// BackupDirName 快照根目录名（R-06：conf.backup/时间戳/，挂面板数据目录下）。
	BackupDirName = "conf.backup"
)

// ScanDirNames 惯例目录优先级（D-03 SCAN_DIR_PRIORITY：sites-enabled >
// conf.d > vhost.d；多目录消重与新建落点同序）。
var ScanDirNames = []string{"sites-enabled", "conf.d", "vhost.d"}

// nginx 默认日志路径回退（⚠️A15：server 块无 access_log/error_log 指令时）。
const (
	NginxDefaultAccessLog = "/var/log/nginx/access.log"
	NginxDefaultErrorLog  = "/var/log/nginx/error.log"
)

// site_type / site_enabled / cert_source / cert_status / acme_challenge /
// site_log_type 枚举取值（引用池定稿，api §3.4；列值同 store/sitemgmt 常量）。
const (
	SiteTypeStatic      = "static"
	SiteTypePHPFPM      = "php_fpm"
	SiteTypeReverseProx = "reverse_proxy"

	SiteEnabled  = "enabled"
	SiteDisabled = "disabled"

	LogTypeAccess = "access"
	LogTypeError  = "error"
)

// 审计动作码（api §3.4 业务常量：action 列存此码；category=change）。
const (
	AuditActionSiteCreated        = "site_created"
	AuditActionSiteUpdated        = "site_updated"
	AuditActionProxyUpdated       = "proxy_updated"
	AuditActionSiteSourceSaved    = "site_source_saved"
	AuditActionSiteEnabled        = "site_enabled"
	AuditActionSiteDisabled       = "site_disabled"
	AuditActionNginxReloaded      = "nginx_reloaded"
	AuditActionCertRequested      = "cert_requested"
	AuditActionCertDeployed       = "cert_deployed"
	AuditActionCertUndeployed     = "cert_undeployed"
	AuditActionCertRenewalRetried = "cert_renewal_retried"
	AuditActionCertDeleted        = "cert_deleted"
	AuditActionCertRenewed        = "cert_renewed"
)

// 审计对象类型（object_type 取值，api §3.4）。
const (
	ObjectTypeSite        = "site"
	ObjectTypeCertificate = "certificate"
)

// defaultHostHeader 反代 Host 头默认值（模板内置默认，api 1.3 D-03）。
const defaultHostHeader = "$host"
