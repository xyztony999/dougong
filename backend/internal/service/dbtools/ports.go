package dbtools

import (
	"context"
	"io/fs"
	"time"

	"github.com/dougong/dougong/internal/adapter/nginxx"
	"github.com/dougong/dougong/internal/adapter/phpfpm"
	"github.com/dougong/dougong/internal/adapter/sysd"
	"github.com/dougong/dougong/internal/service/audit"
	dbmgmtsvc "github.com/dougong/dougong/internal/service/dbmgmt"
	panelsvc "github.com/dougong/dougong/internal/service/panel"
)

// DeployGateway 工具部署网关契约（api §4.1.5 ToolDeployGateway；adapter/toolx
// 实现，main 装配注入；测试注入替身零 runner 实况依赖）。全部命令字面量
// 参数数组（R-08）；凭据不经 argv（Mongo 凭据经 systemd Environment，D-07）。
type DeployGateway interface {
	DownloadTarball(ctx context.Context, url, mirror string, timeout time.Duration) (string, error)
	VerifySha256(localPath, expected string) (bool, error)
	ExtractTar(localPath, destDir string) error
	NpmInstall(ctx context.Context, pkg, version, registry string, timeout time.Duration) (string, error)
	NpmRemove(ctx context.Context, pkg string, timeout time.Duration) (string, error)
	NpmGlobalRoot(ctx context.Context) (string, error)
	PhpExtensions(ctx context.Context) ([]string, error)
	WriteSecretFile(path string, content []byte) error
	WriteFile(path string, content []byte, mode fs.FileMode) error
	ReadFile(path string) ([]byte, error)
	MkdirAll(path string, mode fs.FileMode) error
	Remove(path string) error
	RemoveAll(path string) error
	Rename(old, new string) error
	// ProbePortAvailable 端口占用探针（v1.3，D-19③：net.Listen 即探即放；
	// 占用→false；探针通过不保证 nginx 绑定成功，nginx -t/reload 兜底）。
	ProbePortAvailable(port int) (bool, error)
}

// ToolCreds 实例凭据（D-10：明文仅服务端内存态，调用方用毕即弃，不落库
// 不入日志不入审计 I-02）。契约类型定义在提供方 dbmgmt（沿 envmgmt 消费
// BootstrapOutcome 先例——消费方别名引用，单向依赖 db-tools→db-mgmt）。
type ToolCreds = dbmgmtsvc.ToolCreds

// ReadyInfo 实例就绪结论（D-10 InstanceReadyForTool）。
type ReadyInfo = dbmgmtsvc.ReadyInfo

// CredentialGateway 实例凭据 OHS 消费契约（D-10，进程内单向 db-tools→
// db-mgmt；db-mgmt Service 实现，vault 解密仅服务端；契约权威定义见
// api-database-mgmt.md §4.9/§5.1）。
type CredentialGateway interface {
	InstanceCredentialsForTool(ctx context.Context, instanceID int64) (ToolCreds, error)
	InstanceReadyForTool(ctx context.Context, instanceID int64) (ReadyInfo, error)
}

// ConfigPipeline site-mgmt R-06 四步安全写入管线 OHS 消费契约（D-03；
// api-site-mgmt.md v2.8 D-21 登记；sitemgmt Service 实现）。
type ConfigPipeline interface {
	ConfigApply(ctx context.Context, file, newContent string, allowNew bool) error
	ConfigRemove(ctx context.Context, file string) error
}

// EnvReadiness env-mgmt 环境实况读契约消费（D-18/FR-08⑤：PHP-FPM/Node/nginx
// 已装态与版本；probeOK=false=就绪不可确认，保守按缺失处理）。
type EnvReadiness interface {
	EnvStatus(ctx context.Context, envType string) (installed bool, version string, probeOK bool)
}

// SysdGateway systemd 网关契约（service-mgmt 适配层共享，D-07；含新增
// DaemonReload 扩展——单元文件经文件通道写入后重载）。
type SysdGateway interface {
	GetUnit(ctx context.Context, name string) (sysd.UnitDetail, error)
	StartUnit(ctx context.Context, name string) (sysd.JobResult, error)
	StopUnit(ctx context.Context, name string) (sysd.JobResult, error)
	RestartUnit(ctx context.Context, name string) (sysd.JobResult, error)
	EnableUnit(ctx context.Context, name string) error
	DisableUnit(ctx context.Context, name string) error
	DaemonReload(ctx context.Context) error
}

// PhpFpmGateway pool 实况读取契约（site-mgmt 适配层共享，D-06 默认 pool 复用）。
type PhpFpmGateway interface {
	ListPools(ctx context.Context) ([]phpfpm.Pool, error)
}

// NginxConfReader nginx 受管配置域读取契约（nginxx 适配层共享：落点识别
// D-03 ExtractServers/Proxy()；写入经 ConfigPipeline，本契约只读）。
// v1.3 +FirstWritableDir（D-19①：自动落地块目录=PriorityDirs 首个实存
// 可写目录；全部不可用→错误承载原因）。
type NginxConfReader interface {
	ScanConfDirs() ([]nginxx.FileSnapshot, error)
	ReadFile(path string) (string, time.Time, error)
	FirstWritableDir() (string, error)
}

// PanelAccess 面板访问形态（panel-core 进程内读：D-03 落点选择与内部端点
// URL 构造输入）。契约类型定义在提供方 panelsvc（消费方别名引用）。
type PanelAccess = panelsvc.ToolAccessInfo

// PanelInfo 面板配置读契约（panelsvc.Service 实现，main 装配注入）。
type PanelInfo interface {
	PanelAccess(ctx context.Context) (PanelAccess, error)
}

// SessionVerifier 只读会话校验 OHS 消费契约（D-11：account-auth VerifyWs
// 语义 session.go:59——I-11 判定 revoked/expired，只读不续期，避免工具静态
// 资源逐请求滑动续期写放大；account-auth 无新增 HTTP 接口）。
type SessionVerifier interface {
	VerifyWs(ctx context.Context, rawToken string) (userID int64, role string, err error)
}

// AuditRecorder 审计 OHS 契约（R-03；service/audit.Service 实现）。
type AuditRecorder interface {
	Record(ctx context.Context, e audit.Entry) error
}
