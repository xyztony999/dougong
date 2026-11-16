package sitemgmt

import (
	"context"
	"time"

	"github.com/dougong/dougong/internal/adapter/acme"
	"github.com/dougong/dougong/internal/adapter/nginxx"
	"github.com/dougong/dougong/internal/adapter/phpfpm"
	"github.com/dougong/dougong/internal/service/audit"
)

// NginxGateway 是 nginx 通道与受管配置域文件系统原语的网关契约（api §4.2.5；
// 消费方接口定义，adapter/nginxx.Gateway 实现，main 装配注入）。
type NginxGateway interface {
	// Available 返回 nginx 二进制通道可用性（SITE_012 判据；错误携带 reason）。
	Available() error
	// PriorityDirs 惯例目录绝对路径（优先级降序，D-03）。
	PriorityDirs() []string
	// ScanConfDirs 读取全部惯例目录 .conf 文件（D-01；纯文件读取不经二进制）。
	ScanConfDirs() ([]nginxx.FileSnapshot, error)
	// ReadFile 读文件内容与 mtime（D-06 乐观锁基线）。
	ReadFile(path string) (string, time.Time, error)
	// WriteFileAtomic 原子写回（临时文件+rename）。
	WriteFileAtomic(path string, content []byte) error
	// WriteSecretFileAtomic 原子写敏感文件（0600，D-09 密钥纪律）。
	WriteSecretFileAtomic(path string, content []byte) error
	// CopyFile 复制文件（快照备份，R-06 第一步）。
	CopyFile(src, dst string) error
	// MkdirAll 递归建目录。
	MkdirAll(dir string) error
	// Exists 路径存在性。
	Exists(path string) bool
	// FirstWritableDir 首个存在且可写的惯例目录（D-03 新建落点）。
	FirstWritableDir() (string, error)
	// TestConfig `nginx -t`（R-06 第三步；ok=false=配置错误，err=通道不可用）。
	TestConfig(ctx context.Context) (nginxx.TestResult, error)
	// Reload 平滑重载（D-05；输出与错误分离呈现）。
	Reload(ctx context.Context) (string, error)
	// RemoveFile 删除文件（幂等；acme 托管文件清理 I-04）。
	RemoveFile(path string) error
}

// PhpFpmGateway 是 php-fpm pool 实况读取契约（D-14；adapter/phpfpm 实现）。
type PhpFpmGateway interface {
	ListPools(ctx context.Context) ([]phpfpm.Pool, error)
}

// AcmeGateway 是 ACME 防腐层出站契约（api §4.1.5；R-09 唯一外呼通道，
// adapter/acme 实现——外部 ACME 概念止步于适配器，出参仅证书产物与元数据）。
// webroot 为 http-01 挑战落点（空=面板默认 webroot；复用站点既有
// acme-challenge location 时传其 root，A-SM10）。
type AcmeGateway interface {
	Issue(ctx context.Context, domains []string, challenge, webroot string) (*acme.IssueResult, error)
}

// AuditRecorder 是审计 OHS 契约（R-03；service/audit.Service 实现）。
type AuditRecorder interface {
	Record(ctx context.Context, e audit.Entry) error
}
