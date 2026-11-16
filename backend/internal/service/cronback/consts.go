// Package cronback 是计划任务与备份（cron-backup）应用服务（M8）：cron_job
// 通道投影编排（crontab 读写经 cronx 网关，零台账）+ backup_job/backup_run
// 编排（BackupScheduler 分钟 tick D-13 + BackupExecutor 异步执行 D-07 + 轮转
// R-11/D-09 + 启动恢复 D-08）。不感知 HTTP（分层纪律）。
package cronback

import (
	"time"

	"github.com/dougong/dougong/internal/service/audit"
)

// ---- 审计动作码（§3.4 业务常量：10 契约动作码 + 1 启动恢复系统码 A-C13）----

// cron_job 侧（object_type=cron_job）。
const (
	ActionCronJobCreated   = "cron_job_created"
	ActionCronJobUpdated   = "cron_job_updated"
	ActionCronJobDeleted   = "cron_job_deleted"
	ActionCronJobTriggered = "cron_job_triggered"
)

// backup_job 侧（object_type=backup_job）。
const (
	ActionBackupJobCreated = "backup_job_created"
	ActionBackupJobUpdated = "backup_job_updated"
	ActionBackupJobDeleted = "backup_job_deleted"
)

// backup_run 侧（object_type=backup_run）。
const (
	ActionBackupRunTriggered       = "backup_run_triggered"       // 手动触发（A-C3：与 INSERT run 同事务）
	ActionBackupArtifactDownloaded = "backup_artifact_downloaded" // security（数据取出口径）
	ActionBackupRotated            = "backup_rotated"             // system（删除真实数据须留痕，D-09）
	ActionBackupRunInterrupted     = "backup_run_interrupted"     // system（启动恢复，2.3 编排；A-C13）
)

// 审计对象类型。
const (
	ObjectTypeCronJob   = "cron_job"
	ObjectTypeBackupJob = "backup_job"
	ObjectTypeBackupRun = "backup_run"
)

// 审计便捷别名（audit 包枚举）。
const (
	resultSuccess = audit.ResultSuccess
	resultFailure = audit.ResultFailure
	sourceWeb     = audit.SourceWeb
	sourceSystem  = audit.SourceSystem
)

// ---- 业务常量（§3.4 档位常量：编译期，不入库不配置）----

const (
	// TriggerTimeout 手动触发同步等待上限（D-05）。
	TriggerTimeout = 120 * time.Second
	// PreviewCount 表达式预览未来执行次数（D-03）。
	PreviewCount = 5
	// LogLinesDefault 日志尾部默认档位（D-04）。
	LogLinesDefault = 200
	// SchedulerTick 调度器 tick 周期（D-13：分钟级=cron 语义精度上限）。
	SchedulerTick = time.Minute
	// ExecutorWorkers 执行 worker 数（A-C9：与立即执行共用同一执行通道）。
	ExecutorWorkers = 2
	// QueueCapacity 执行队列容量（A-C9；个位数任务量级下不会触顶）。
	QueueCapacity = 128
	// OrphanRunMessage 启动恢复错误语（D-08/§5.2 SQL #9）。
	OrphanRunMessage = "面板重启中断"
	// MaxNameLength 任务名长度上限（R-14，≤64 字符）。
	MaxNameLength = 64
	// MaxRetention / MinRetention 保留份数区间（§4.1.1 CHECK 1..100）。
	MaxRetention = 100
	MinRetention = 1
	// ErrorBriefLength 历史列表失败原因摘要截断长度（rune）。
	ErrorBriefLength = 120
)

// LogLinesTiers 日志尾部档位（D-04：越档归默认 200）。
var LogLinesTiers = []int{50, 100, 200, 500, 1000}

// NormalizeLogLines 归一化 lines 查询参数：缺省/非法/越档 → 200。
func NormalizeLogLines(raw string) int {
	n := 0
	for _, r := range raw {
		if r < '0' || r > '9' {
			return LogLinesDefault
		}
		n = n*10 + int(r-'0')
		if n > 10000 {
			return LogLinesDefault
		}
	}
	for _, tier := range LogLinesTiers {
		if n == tier {
			return n
		}
	}
	return LogLinesDefault
}
