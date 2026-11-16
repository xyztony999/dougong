// Package cronback 是计划任务与备份（cron-backup）模块的存储层：backup_job
// 主数据 + backup_run 执行历史两表（api-cron-backup.md §5.3；cron_job 零台账
// 不落库——D2 口径-1）。SQL 只出现在本包，全部参数化（G-02）。
//
// 事务组合：BackupExecutor 终态回写 + 轮转 DELETE + audit backup_rotated 同
// 事务原子提交（D-09）；手动触发 INSERT run 与 audit backup_run_triggered 同
// 事务（R-03/A-C3）——事务视图经 BeginTx 提供。
package cronback

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	store "github.com/dougong/dougong/internal/store"
)

// Executor 是 *sql.DB 与 *sql.Tx 共同满足的最小执行接口（store 基座别名；
// 同一仓储即可绑定连接池、也可绑定事务，I-05 同事务组合）。
type Executor = store.Executor

// 备份源类型（backup_source_type 枚举，§3.4）。
const (
	SourceSiteDir  = "site_dir"
	SourceDatabase = "database"
)

// 备份目标类型（backup_target_type 枚举）。
const (
	TargetLocal = "local"
	TargetRsync = "rsync"
)

// run 触发方式（backup_trigger 枚举）。
const (
	TriggerCron   = "cron"
	TriggerManual = "manual"
)

// run 状态机（backup_run_status 枚举，ADR-016）。
const (
	StatusRunning = "running"
	StatusSuccess = "success"
	StatusFailure = "failure"
)

// 哨兵错误（service 层翻译为 CB_* 错误码）。
var (
	// ErrJobNameTaken name 唯一索引冲突（I-06 → CB_005）。
	ErrJobNameTaken = errors.New("cronback: backup job name taken")
	// ErrRunMutex idx_run_running 部分唯一索引冲突（I-01 → CB_006 手动侧/调度侧跳过）。
	ErrRunMutex = errors.New("cronback: backup job already running")
)

// SourceItem 是备份源清单项（source_items JSON 值集合元素，§4.1.1）：
// site_dir 项只填 Path；database 项填 InstanceID，Database 可缺省=整实例全库
// （执行期逐库展开，A-C6）。
type SourceItem struct {
	Type       string `json:"type"`
	Path       string `json:"path,omitempty"`
	InstanceID int64  `json:"instanceId,omitempty"`
	Database   string `json:"database,omitempty"`
}

// TargetConfig 是备份目标值对象（target_config JSON，D-06：无凭据字段）：
// local 填 Path；rsync 填 Host/User/Port?/Path（ssh 免密，凭据由运行环境提供）。
type TargetConfig struct {
	Path string `json:"path,omitempty"`
	Host string `json:"host,omitempty"`
	User string `json:"user,omitempty"`
	Port int    `json:"port,omitempty"`
}

// FileEntry 是产物清单项（files JSON：[{name,size}]，D-10 manifest files 快照）。
type FileEntry struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// Job 是 backup_job 行（源清单/目标在读写边界与 JSON 文本列互转）。
type Job struct {
	ID             int64
	Name           string
	SourceItems    []SourceItem
	TargetType     string
	TargetConfig   TargetConfig
	RetentionCount int
	ScheduleCron   string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Run 是 backup_run 行（files 在读写边界与 JSON 文本列互转；终态字段可空）。
type Run struct {
	ID           int64
	JobID        int64
	JobName      string
	Trigger      string
	Status       string
	StartedAt    time.Time
	FinishedAt   *time.Time
	ArtifactPath *string
	ArtifactSize *int64
	ErrorMessage *string
	Files        []FileEntry
}

// RunFilter 是历史检索条件（§4.2.4：jobId?/status?/trigger?/from?/to?；jobId
// 对已删任务仍有效——弱引用归组，D-12）。
type RunFilter struct {
	JobID   *int64
	Status  string
	Trigger string
	From    *time.Time
	To      *time.Time
}

// RotateVictim 是轮转将删除的记录（事务内先 SELECT 收集产物定位，DELETE 后
// 供事务外产物清理使用，D-09/A-C4）。
type RotateVictim struct {
	ID           int64
	ArtifactPath string
}

// marshalSourceItems 序列化源清单值集合。
func marshalSourceItems(items []SourceItem) (string, error) {
	b, err := json.Marshal(items)
	if err != nil {
		return "", fmt.Errorf("cronback: marshal source items: %w", err)
	}
	return string(b), nil
}

// unmarshalSourceItems 反序列化源清单值集合。
func unmarshalSourceItems(raw string) ([]SourceItem, error) {
	var items []SourceItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Errorf("cronback: unmarshal source items: %w", err)
	}
	return items, nil
}

// marshalTargetConfig 序列化目标值对象。
func marshalTargetConfig(t TargetConfig) (string, error) {
	b, err := json.Marshal(t)
	if err != nil {
		return "", fmt.Errorf("cronback: marshal target config: %w", err)
	}
	return string(b), nil
}

// unmarshalTargetConfig 反序列化目标值对象。
func unmarshalTargetConfig(raw string) (TargetConfig, error) {
	var t TargetConfig
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return TargetConfig{}, fmt.Errorf("cronback: unmarshal target config: %w", err)
	}
	return t, nil
}

// marshalFiles 序列化产物清单。
func marshalFiles(files []FileEntry) (string, error) {
	b, err := json.Marshal(files)
	if err != nil {
		return "", fmt.Errorf("cronback: marshal files: %w", err)
	}
	return string(b), nil
}

// unmarshalFiles 反序列化产物清单（空串/NULL → nil）。
func unmarshalFiles(raw *string) ([]FileEntry, error) {
	if raw == nil || *raw == "" {
		return nil, nil
	}
	var files []FileEntry
	if err := json.Unmarshal([]byte(*raw), &files); err != nil {
		return nil, fmt.Errorf("cronback: unmarshal files: %w", err)
	}
	return files, nil
}

// formatTime/scanTime 等时间辅助与 store/auth、store/dbmgmt 同法（RFC3339
// UTC 文本列；DATETIME 列经 any 扫描兼容 string/[]byte/time.Time 三形态）。
func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func scanTime(src any) (time.Time, error) {
	switch v := src.(type) {
	case time.Time:
		return v.UTC(), nil
	case string:
		return time.Parse(time.RFC3339, v)
	case []byte:
		return time.Parse(time.RFC3339, string(v))
	default:
		return time.Time{}, fmt.Errorf("cronback: unsupported time column %T", src)
	}
}

func scanTimePtr(src any) (*time.Time, error) {
	if src == nil {
		return nil, nil
	}
	t, err := scanTime(src)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func stringPtr(src any) *string {
	if src == nil {
		return nil
	}
	switch v := src.(type) {
	case string:
		s := v
		return &s
	case []byte:
		s := string(v)
		return &s
	default:
		return nil
	}
}

func int64Ptr(src any) *int64 {
	if src == nil {
		return nil
	}
	switch v := src.(type) {
	case int64:
		n := v
		return &n
	default:
		return nil
	}
}
