package cronback

import (
	"context"
	"time"

	"github.com/dougong/dougong/internal/adapter/backupx"
	"github.com/dougong/dougong/internal/adapter/cronx"
	"github.com/dougong/dougong/internal/service/audit"
	storeauth "github.com/dougong/dougong/internal/store/auth"
	storecron "github.com/dougong/dougong/internal/store/cronback"
)

// ports.go 定义本模块全部进程内契约（接口定义在消费方，backend-01 §1）。

// CronTabGateway 是 crontab 读写投影契约（§3.2，cronx 承载；R-08 参数数组）。
type CronTabGateway interface {
	List(ctx context.Context) (*cronx.TabProjection, error)
	ReadRaw(ctx context.Context) (string, error)
	Upsert(ctx context.Context, oldJobKey string, job cronx.JobInput) (string, error)
	Delete(ctx context.Context, jobKey string) error
}

// CronLogSource 是 cron 执行日志尾部契约（D-04：journalctl → syslog → cron
// 适配归适配器）。
type CronLogSource interface {
	Tail(ctx context.Context, lines int) (string, error)
}

// JobRunner 是面板代执行契约（D-05：shell 语义、120s 进程组终止；命令原文
// 由调用方取自 crontab——I-08）。
type JobRunner interface {
	Run(ctx context.Context, command string, timeout time.Duration) cronx.TriggerResult
}

// BackupExecutor 是备份执行契约（§3.2，backupx 承载；tar/导出/rsync 全参数
// 数组 R-08；manifest 写入 D-10）。
type BackupExecutor interface {
	Execute(ctx context.Context, spec backupx.JobSpec, startedAt time.Time) (backupx.ExecResult, error)
	// PullFile 产物单文件拉回本地（download 通道；local 直读/rsync 拉流）。
	PullFile(ctx context.Context, artifactPath, name string, port int) (string, func(), error)
	// ReadManifestPath 读产物 manifest（detail.sources 数据源，A-C5）。
	ReadManifestPath(ctx context.Context, artifactPath string, port int) (*backupx.Manifest, error)
	// RemoveArtifactPath 产物清理（轮转事务后 best-effort，A-C4）。
	RemoveArtifactPath(ctx context.Context, artifactPath string, port int) error
}

// JobRepoAPI 是 backup_job 仓储契约（§4.1.5；实现=store/cronback）。
type JobRepoAPI interface {
	List(ctx context.Context) ([]storecron.Job, error)
	GetByID(ctx context.Context, id int64) (*storecron.Job, error)
	GetByName(ctx context.Context, name string) (*storecron.Job, error)
	Create(ctx context.Context, j *storecron.Job, now time.Time) (int64, error)
	Update(ctx context.Context, j *storecron.Job, now time.Time) error
	Delete(ctx context.Context, id int64) error
}

// RunRepoAPI 是 backup_run 仓储契约（§4.2.5；实现=store/cronback）。
type RunRepoAPI interface {
	Create(ctx context.Context, jobID int64, jobName, trigger string, startedAt time.Time) (int64, error)
	FinishSuccess(ctx context.Context, id int64, finishedAt time.Time,
		artifactPath string, artifactSize int64, files []storecron.FileEntry) (int64, error)
	FinishFailure(ctx context.Context, id int64, finishedAt time.Time, errorMessage string) (int64, error)
	Rotate(ctx context.Context, jobID int64, retention int) ([]storecron.RotateVictim, error)
	Search(ctx context.Context, f storecron.RunFilter, offset, limit int) ([]storecron.Run, int64, error)
	GetByID(ctx context.Context, id int64) (*storecron.Run, error)
	LatestByJob(ctx context.Context, jobID int64) (*storecron.Run, error)
	MarkOrphanFailed(ctx context.Context, now time.Time, reason string) (int64, error)
}

// TxAPI 是事务绑定仓储视图（终态回写+轮转+审计同事务 D-09；手动触发 INSERT
// run+审计同事务 R-03/A-C3；任务 CRUD 与审计同事务 R-03）。
type TxAPI interface {
	Job() JobRepoAPI
	Run() RunRepoAPI
	Audit() *storeauth.AuditRepo
	Commit() error
	Rollback() error
}

// StoreAPI 是仓储聚合契约（BeginTx 供同事务编排）。
type StoreAPI interface {
	Job() JobRepoAPI
	Run() RunRepoAPI
	BeginTx(ctx context.Context) (TxAPI, error)
}

// AuditRecorder 是审计 OHS 契约（R-03；service/audit.Service 实现）。
type AuditRecorder interface {
	Record(ctx context.Context, e audit.Entry) error
}

// InstanceArchiveAPI 是实例档案只读校验契约（InstanceRefChecker 数据源，
// §4.1.1）。
type InstanceArchiveAPI interface {
	InstanceArchive(ctx context.Context, instanceID int64) (found bool, engine, instanceName string, err error)
}
