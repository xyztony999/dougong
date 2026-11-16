package cronback

import (
	"time"

	"github.com/dougong/dougong/internal/adapter/backupx"
	"github.com/dougong/dougong/internal/adapter/cronx"
	storecron "github.com/dougong/dougong/internal/store/cronback"
)

// dto.go 是传输契约 DTO（§4.3.4/§4.1.4/§4.2.4；时间 ISO-8601 UTC，§3.1）。
// 由 api 层直接编码（handler 薄壳不复制字段）。

// ---- cron_job（§4.3.4）----

// CronJobItem 是 /cron/job/list items 元素（jobKey 不透明透传，D-01）。
type CronJobItem struct {
	JobKey  string  `json:"jobKey"`
	Expr    string  `json:"expr"`
	Command string  `json:"command"`
	Remark  *string `json:"remark"`
	Source  string  `json:"source"`
}

// CronJobListData 是 GET /cron/job/list data。
type CronJobListData struct {
	Items         []CronJobItem       `json:"items"`
	SystemEntries cronx.SystemEntries `json:"systemEntries"`
}

// CronJobCreateReq POST /cron/job/create。
type CronJobCreateReq struct {
	Expr    string  `json:"expr" binding:"required"`
	Command string  `json:"command" binding:"required"`
	Remark  *string `json:"remark"`
}

// CronJobUpdateReq POST /cron/job/update。
type CronJobUpdateReq struct {
	JobKey  string  `json:"jobKey" binding:"required"`
	Expr    string  `json:"expr" binding:"required"`
	Command string  `json:"command" binding:"required"`
	Remark  *string `json:"remark"`
}

// CronJobDeleteReq / CronJobTriggerReq POST delete / trigger。
type CronJobDeleteReq struct {
	JobKey string `json:"jobKey" binding:"required"`
}

type CronJobTriggerReq struct {
	JobKey string `json:"jobKey" binding:"required"`
}

// CronExprValidateReq POST /cron/job/validate。
type CronExprValidateReq struct {
	Expr string `json:"expr" binding:"required"`
}

// CronExprValidateData validate 成功 data（nextFires[5]，D-03）。
type CronExprValidateData struct {
	NextFires []string `json:"nextFires"`
}

// CronTriggerData trigger 成功 data（cronx.TriggerResult 同构：exitCode 可空
// =未正常退出、output 已尾部截断、timedOut、durationMs，D-05）。
type CronTriggerData = cronx.TriggerResult

// JobKeyData create/update 成功 data（update 返回新 jobKey——随内容变更，D-01）。
type JobKeyData struct {
	JobKey string `json:"jobKey"`
}

// ---- backup_job（§4.1.4）----

// SourceItemDTO 是源清单项传输形态（I-09；database 缺省=整实例全库）。
// Ref 是契约 §4.1.4 sourceSummary[{type, ref}] 的引用字段（Gate 2 E1 修复）：
// site_dir→目录路径 / database→「实例名·库名」（库名缺省=*，整实例语义）；
// path/instanceId/database 保留供编辑回填（契约无 detail 接口，list 为唯一
// 回填源，超集字段前端宽容消费）。
type SourceItemDTO struct {
	Type       string `json:"type"`
	Ref        string `json:"ref"`
	Path       string `json:"path,omitempty"`
	InstanceID int64  `json:"instanceId,omitempty"`
	Database   string `json:"database,omitempty"`
}

// TargetConfigDTO 是目标值对象传输形态（D-06：无凭据字段）。
type TargetConfigDTO struct {
	Path string `json:"path,omitempty"`
	Host string `json:"host,omitempty"`
	User string `json:"user,omitempty"`
	Port int    `json:"port,omitempty"`
}

// BackupJobReq 是 create/update 请求体（update 多 jobId）。
type BackupJobReq struct {
	JobID          int64           `json:"jobId"`
	Name           string          `json:"name" binding:"required"`
	SourceItems    []SourceItemDTO `json:"sourceItems" binding:"required"`
	TargetType     string          `json:"targetType" binding:"required"`
	TargetConfig   TargetConfigDTO `json:"targetConfig"`
	RetentionCount int             `json:"retentionCount" binding:"required"`
	ScheduleCron   string          `json:"scheduleCron" binding:"required"`
}

// BackupJobIDReq 是 delete / run 请求体。
type BackupJobIDReq struct {
	JobID int64 `json:"jobId" binding:"required"`
}

// LastRunSummary 是任务清单最近执行结论（读时派生，D-11；null=从未执行）。
type LastRunSummary struct {
	ID         int64   `json:"id"`
	Status     string  `json:"status"`
	Trigger    string  `json:"trigger"`
	StartedAt  string  `json:"startedAt"`
	FinishedAt *string `json:"finishedAt"`
}

// BackupJobItem 是 /cron/backup-job/list items 元素。
type BackupJobItem struct {
	ID             int64           `json:"id"`
	Name           string          `json:"name"`
	SourceSummary  []SourceItemDTO `json:"sourceSummary"`
	TargetType     string          `json:"targetType"`
	TargetBrief    string          `json:"targetBrief"`
	RetentionCount int             `json:"retentionCount"`
	ScheduleCron   string          `json:"scheduleCron"`
	NextFireAt     *string         `json:"nextFireAt"`
	LastRun        *LastRunSummary `json:"lastRun"`
}

// BackupJobListData 是 GET /cron/backup-job/list data（无分页，id 升序）。
type BackupJobListData struct {
	Items []BackupJobItem `json:"items"`
}

// BackupRunIDData 是 create/run 等成功 data。
type BackupRunIDData struct {
	RunID int64 `json:"runId"`
}

// BackupJobIDData 是 create 成功 data。
type BackupJobIDData struct {
	ID int64 `json:"id"`
}

// ---- backup_run（§4.2.4）----

// RunQuery 是历史检索 query 绑定形态（筛选 {jobId?, status?, trigger?, from?,
// to?} + 公共分页 {page, pageSize}，D-11）。
type RunQuery struct {
	JobID    int64  `form:"jobId"`
	Status   string `form:"status"`
	Trigger  string `form:"trigger"`
	From     string `form:"from"`
	To       string `form:"to"`
	Page     int    `form:"page"`
	PageSize int    `form:"pageSize"`
}

// BackupRunItem 是历史列表 items 元素（durationMs 派生，D-11）。
type BackupRunItem struct {
	ID           int64   `json:"id"`
	JobID        int64   `json:"jobId"`
	JobName      string  `json:"jobName"`
	Trigger      string  `json:"trigger"`
	Status       string  `json:"status"`
	StartedAt    string  `json:"startedAt"`
	FinishedAt   *string `json:"finishedAt"`
	DurationMs   *int64  `json:"durationMs"`
	ArtifactSize *int64  `json:"artifactSize"`
	ErrorBrief   *string `json:"errorBrief"`
}

// BackupRunListData 是 GET /cron/backup-run/list data（公共分页 {items,total}）。
type BackupRunListData struct {
	Items []BackupRunItem `json:"items"`
	Total int64           `json:"total"`
}

// BackupRunDetail 是 GET /cron/backup-run/detail data（running 态产物字段为
// null；sources 来自 manifest 语义，A-C5）。
type BackupRunDetail struct {
	ID           int64                 `json:"id"`
	JobID        int64                 `json:"jobId"`
	JobName      string                `json:"jobName"`
	Trigger      string                `json:"trigger"`
	Status       string                `json:"status"`
	StartedAt    string                `json:"startedAt"`
	FinishedAt   *string               `json:"finishedAt"`
	DurationMs   *int64                `json:"durationMs"`
	ArtifactPath *string               `json:"artifactPath"`
	ArtifactSize *int64                `json:"artifactSize"`
	Files        []storecron.FileEntry `json:"files"`
	ErrorMessage *string               `json:"errorMessage"`
	Sources      []backupx.SourceSpec  `json:"sources"`
}

// DownloadTarget 是产物下载定位（handler 开流载荷；Name=manifest 内原名 D-10）。
type DownloadTarget struct {
	Name    string
	Size    int64
	Path    string // 本地文件绝对路径（local 直读/rsync 已拉回临时文件）
	Cleanup func() // rsync 拉回流后清理（local 为 no-op）
}

// ---- 内部 DTO 辅助 ----

// fmtTime 统一 RFC3339 UTC 串。
func fmtTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// fmtTimePtr 可空时间。
func fmtTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := fmtTime(*t)
	return &s
}

// durationMs 派生耗时（running→nil，D-11 不存耗时列）。
func durationMs(started time.Time, finished *time.Time) *int64 {
	if finished == nil {
		return nil
	}
	ms := finished.Sub(started).Milliseconds()
	return &ms
}
