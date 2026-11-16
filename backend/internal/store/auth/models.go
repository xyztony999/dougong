// Package auth 是 account-auth 模块的数据访问层（SQL 只允许出现在本包及
// internal/store 基座）。行结构体即领域持久化模型，供 service 层接口引用
// （依赖方向：api → service → store，本包不感知 HTTP 与业务编排）。
package auth

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/dougong/dougong/internal/store"
)

// Executor 是 store.Executor 的别名（*sql.DB 与 *sql.Tx 共同满足），
// 仓储以它绑定连接池或事务。
type Executor = store.Executor

// 角色（panel_role）与用户状态（panel_user_status），值域由 DDL CHECK 约束兜底。
const (
	RoleAdmin    = "admin"
	RoleReadonly = "readonly"

	UserStatusActive = "active"
	UserStatusLocked = "locked"
)

// 会话吊销原因（session_end_reason）。
const (
	EndReasonSelfLogout   = "self_logout"
	EndReasonForcedLogout = "forced_logout"
	EndReasonCascade      = "cascade"
)

// User 对应 panel_user 行（含内嵌登录锁定字段组，D-04）。
type User struct {
	ID           int64
	Username     string
	PasswordHash string
	Role         string
	Status       string
	FailCount    int
	LockedUntil  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// TotpCredential 对应 panel_user_totp 行（1:1，enabled=false 为待确认态）。
type TotpCredential struct {
	UserID      int64
	Secret      string
	Enabled     bool
	ConfirmedAt *time.Time
}

// Session 对应 panel_session 行；Username 由 JOIN panel_user 填充（清单展示用）。
// 有效期为计算态（D-03）：RevokedAt==nil 且当前时刻早于 ExpiresAt 与 IdleExpiresAt。
type Session struct {
	TokenHash     string
	UserID        int64
	Username      string
	CsrfToken     string
	CreatedAt     time.Time
	LastActiveAt  time.Time
	ExpiresAt     time.Time
	IdleExpiresAt time.Time
	RevokedAt     *time.Time
	EndReason     *string
	SourceIP      *string
	UserAgent     *string
}

// Digest 返回会话对外标识（token_hash 前 8 位摘要，设计文档 §4.2.4）。
func (s Session) Digest() string {
	if len(s.TokenHash) >= 8 {
		return s.TokenHash[:8]
	}
	return s.TokenHash
}

// InitState 对应 init_state 单行（id=1）。
type InitState struct {
	Initialized   bool
	Method        *string
	InitializedAt *time.Time
}

// AuditEntry 是审计写入侧载荷（四要素 R-03：操作者/时间戳由 Append 落库、对象、结果）。
type AuditEntry struct {
	ActorID    *int64
	ActorName  string
	Category   string // change | security
	Action     string // 细粒度动作码（api-account-auth.md §3.4 业务常量）
	ObjectType string
	ObjectID   string
	Detail     any    // 序列化为 JSON 落库；nil → NULL
	Result     string // success | failure
	Source     string // web | cli | system
}

// AuditRecord 是审计查询侧行（Detail 为原始 JSON 字符串，透传前端/导出）。
// JSON 键为 camelCase（契约 §4.3.4 items 与前端 types/auth.ts AuditLogItem
// 对齐——Gate 2 E2 修复）；actorId 不在契约 items 中，仅服务端内部使用。
type AuditRecord struct {
	ID         int64     `json:"id"`
	ActorID    *int64    `json:"-"`
	ActorName  string    `json:"actorName"`
	Category   string    `json:"category"`
	Action     string    `json:"action"`
	ObjectType string    `json:"objectType"`
	ObjectID   *string   `json:"objectId"`
	Detail     *string   `json:"detail"`
	Result     string    `json:"result"`
	Source     string    `json:"source"`
	CreatedAt  time.Time `json:"createdAt"`
}

// AuditCondition 是审计条件查询过滤（全部字段可选，空为零条件）。
type AuditCondition struct {
	From       *time.Time
	To         *time.Time
	ActorName  string
	Category   string
	Action     string
	ObjectType string
	ObjectID   string
}

// ---- 时间映射：DDL 中 DATETIME 列以 TEXT(RFC3339, UTC) 存储，
// 字典序即时间序（年份恒 4 位、UTC 后缀恒 Z），供 WHERE 比较使用。 ----

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
			return time.Time{}, fmt.Errorf("store/auth: parse time %q: %w", v, err)
		}
		return t, nil
	case time.Time:
		// modernc.org/sqlite 对声明类型 DATETIME 的 TEXT 列会返回 time.Time。
		return v, nil
	default:
		return time.Time{}, fmt.Errorf("store/auth: unexpected time column type %T", src)
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

func scanStringPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	v := ns.String
	return &v
}
