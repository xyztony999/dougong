package panel

import (
	"context"
	"time"

	"github.com/dougong/dougong/internal/adapter/acme"
	"github.com/dougong/dougong/internal/adapter/nginxx"
	storeauth "github.com/dougong/dougong/internal/store/auth"
	storepanel "github.com/dougong/dougong/internal/store/panel"
)

// Store 是仓储聚合契约：连接池视图 + 事务开启（I-P07 原子保存与 I-05
// 同事务审计）。仓储行为契约见 internal/store/panel 各 Repo 文档注释
// （测试采用真实 :memory: SQLite，不 mock 数据库，backend-03 §6）。
type Store interface {
	Config() *storepanel.ConfigRepo
	Cert() *storepanel.CertRepo
	Audit() *storeauth.AuditRepo
	BeginTx(ctx context.Context) (*storepanel.TxStore, error)
}

// VersionSource 是版本源适配器契约（D-P09：ACL 适配器隔离版本源；源标识
// 与清单格式未锁定——prd 待对齐-1，main 暂不装配实现，触发即 PANEL_005；
// 待源定稿后仅新增 adapter 实现，零契约变更）。
type VersionSource interface {
	LatestVersion(ctx context.Context) (string, error)
}

// AcmeIssuer 是 site-mgmt AcmeGateway 的 OHS 消费契约（v2.0 跨模块消费，
// ADR-029/D-P13：面板证书 ACME 签发/续期唯一通道，main 注入同一
// adapter/acme.Gateway 实例——ACME 账户密钥与 CA 目录复用 site-mgmt 载体）。
type AcmeIssuer interface {
	Issue(ctx context.Context, domains []string, challenge, webroot string) (*acme.IssueResult, error)
}

// NginxChannel 是 site-mgmt nginx 安全写回的 OHS 消费契约面（v2.0 跨模块
// 消费，ADR-029/D-P15：HTTP-01 挑战 location 临时注入/撤除走 R-06 四步；
// main 注入同一 adapter/nginxx.Gateway 实例——与 site-mgmt 共用通道与配置域）。
type NginxChannel interface {
	// Available 返回 nginx 二进制通道可用性（PANEL_008 判据；错误携带 reason）。
	Available() error
	// ScanConfDirs 读取全部惯例目录 .conf 文件（D-P15 落点选择输入）。
	ScanConfDirs() ([]nginxx.FileSnapshot, error)
	// ReadFile 读文件内容与 mtime（注入前原文快照）。
	ReadFile(path string) (string, time.Time, error)
	// WriteFileAtomic 原子写回（临时文件+rename，注入与撤除共用）。
	WriteFileAtomic(path string, content []byte) error
	// CopyFile 复制文件（R-06 写前备份）。
	CopyFile(src, dst string) error
	// MkdirAll 递归建目录（挑战目录）。
	MkdirAll(dir string) error
	// TestConfig `nginx -t`（R-06 第三步；ok=false=配置错误，err=通道不可用）。
	TestConfig(ctx context.Context) (nginxx.TestResult, error)
	// Reload 平滑重载（挑战 location 生效/撤除即时可见）。
	Reload(ctx context.Context) (string, error)
	// RemoveFile 删除文件（幂等；备份清理）。
	RemoveFile(path string) error
}
