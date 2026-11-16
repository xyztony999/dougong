// Package fileman 是 file-manager 模块的数据访问层（SQL 只允许出现在本包及
// internal/store 基座）。行结构体即领域持久化模型，供 service 层引用
// （依赖方向：api → service → store，本包不感知 HTTP 与业务编排）。
package fileman

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/dougong/dougong/internal/store"
)

// Executor 是 store.Executor 的别名（*sql.DB 与 *sql.Tx 共同满足），
// 仓储以它绑定连接池或事务。
type Executor = store.Executor

// trash_entry.status 取值（trash_entry_status 枚举，api-file-manager §3.4）。
const (
	TrashStatusInTrash  = "in_trash"
	TrashStatusRestored = "restored"
	TrashStatusPurged   = "purged"
)

// trash_entry.entry_type 取值（仅两值；符号链接按 file 归类，§4.1.1）。
const (
	EntryTypeFile      = "file"
	EntryTypeDirectory = "directory"
)

// TrashEntry 对应 trash_entry 行（聚合根，I-01~I-05 不变量见 §4.1.1）。
type TrashEntry struct {
	ID           int64
	OriginalPath string
	TrashName    string
	EntryType    string // file | directory
	Size         int64
	OperatorID   *int64
	OperatorName string // 冗余快照（D-12：落库后不随用户变化）
	Status       string // in_trash | restored | purged
	DeletedAt    time.Time
	ResolvedAt   *time.Time
}

// PathScope 对应 fs_path_scope 单行（id=1，D-02）。
type PathScope struct {
	RootPath     string
	ExcludedDirs []string // 逗号分隔存储，加载时拆分
	UpdatedAt    time.Time
}

// ---- 时间映射：DDL 中 DATETIME 列以 TEXT(RFC3339, UTC) 存储，
// 字典序即时间序（与 store/auth 同口径）。 ----

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func formatTimePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return formatTime(*t)
}

func scanTime(src any) (time.Time, error) {
	switch v := src.(type) {
	case string:
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return time.Time{}, fmt.Errorf("store/fileman: parse time %q: %w", v, err)
		}
		return t, nil
	case time.Time:
		// modernc.org/sqlite 对声明类型 DATETIME 的 TEXT 列会返回 time.Time。
		return v, nil
	default:
		return time.Time{}, fmt.Errorf("store/fileman: unexpected time column type %T", src)
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

// wrapErr 统一包装仓储错误（保持 %w 解链；sql.ErrNoRows 由调用方判语义）。
func wrapErr(op string, err error) error {
	if err == nil {
		return nil
	}
	if err == sql.ErrNoRows {
		return fmt.Errorf("store/fileman: %s: %w", op, err)
	}
	return fmt.Errorf("store/fileman: %s: %w", op, err)
}
