// Package panel 是面板基座（panel-core）模块的数据访问层（SQL 只允许出现
// 在本包；行结构体即领域持久化模型，供 service 层接口引用——依赖方向
// api → service → store，本包不感知 HTTP 与业务编排）。
//
// 两表均为单行表（id=1 CHECK，D-P01/D-P04）：panel_config 配置单例 +
// panel_cert 证书单行（自签/导入/acme 共用一行，替换式更新；v2.0 增量
// bind_domain/https_mode+acme/source+acme/last_error，迁移 007）。
package panel

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/dougong/dougong/internal/store"
)

// Executor 是 store.Executor 的别名（*sql.DB 与 *sql.Tx 共同满足），
// 仓储以它绑定连接池或事务。
type Executor = store.Executor

// 监听范围（listen_scope）与 HTTPS 模式（https_mode）、证书来源
// （panel_cert_source）取值域，由 DDL CHECK 约束兜底（api-panel-core.md §3.4）。
const (
	ListenScopeAll       = "all"
	ListenScopeLocalhost = "localhost"
	ListenScopeCustom    = "custom"

	HTTPSModeOff        = "off"
	HTTPSModeSelfSigned = "self_signed"
	HTTPSModeImported   = "imported"
	HTTPSModeAcme       = "acme" // v2.0 扩展（D-P14，迁移 007）

	CertSourceSelfSigned = "self_signed"
	CertSourceImported   = "imported"
	CertSourceAcme       = "acme" // v2.0 扩展（ADR-029，迁移 007）
)

// 面板证书状态（cert_status 引用的 4 值子集，D-P13 读时推算，不落库；
// pending 不呈现——同步终态无停留）。
const (
	CertStatusValid        = "valid"
	CertStatusExpiringSoon = "expiring_soon"
	CertStatusExpired      = "expired"
	CertStatusRenewFailed  = "renew_failed" // 落库态：last_error 非空即呈现
)

// Config 对应 panel_config 单行（id=1，I-P01 单例保证）。
type Config struct {
	ID               int64
	ListenScope      string
	ListenAddress    *string // 仅 listen_scope=custom 时非空（I-P02）
	ListenPort       int     // 1024..65535；修改重启生效（D-P02）
	EntryPathEnabled bool
	EntryPath        *string // ^[A-Za-z0-9_-]{8,32}$（I-P03）
	BindDomain       *string // 绑定域名（v2.0/ADR-028）：NULL=未绑定放行全部 Host；唯一写入口=config/update.bindDomain（D-P12）
	HTTPSMode        string
	UpdateCheck      bool
	Lang             string
	UpdatedAt        time.Time
}

// Cert 对应 panel_cert 单行（id=1，D-P04）。KeyPEM 永不经任何接口下发
// （I-P06）——仅 TLS 装配（GetCertificate）与服务端配对校验内部使用。
type Cert struct {
	ID        int64
	CertPEM   string
	KeyPEM    string
	Source    string
	Issuer    *string
	NotBefore *time.Time
	NotAfter  *time.Time
	LastError *string // ACME 签发/续期失败原因（v2.0；非空即 renew_failed 呈现，D-P13）
	UpdatedAt time.Time
}

// ConfigChanges 是 panel_config 部分更新载荷（SQL 清单 #3）：nil 字段不更新；
// ListenAddress 空串表示置 NULL（清空，I-P02 ⇔ 语义）。
// EntryPath 仅服务端内部路径使用（regenerate 与开启时自动生成，api 4.1.4：
// 值不可经 config/update 接口直接设置）。
type ConfigChanges struct {
	ListenScope      *string
	ListenAddress    *string
	ListenPort       *int
	EntryPathEnabled *bool
	EntryPath        *string
	BindDomain       *string // v2.0：空串=解绑（置 NULL），非空=绑定（D-P12）
	HTTPSMode        *string
	UpdateCheck      *bool
	Lang             *string
}

// Empty 报告是否不含任何列变更。
func (c ConfigChanges) Empty() bool {
	return c.ListenScope == nil && c.ListenAddress == nil && c.ListenPort == nil &&
		c.EntryPathEnabled == nil && c.EntryPath == nil && c.BindDomain == nil &&
		c.HTTPSMode == nil && c.UpdateCheck == nil && c.Lang == nil
}

// ---- 时间映射：与 001_account_auth 同法，DATETIME 列以 TEXT(RFC3339, UTC)
// 存储，字典序即时间序。 ----

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func scanTime(src any) (time.Time, error) {
	switch v := src.(type) {
	case string:
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return time.Time{}, fmt.Errorf("store/panel: parse time %q: %w", v, err)
		}
		return t, nil
	case time.Time:
		// modernc.org/sqlite 对声明类型 DATETIME 的 TEXT 列会返回 time.Time。
		return v, nil
	default:
		return time.Time{}, fmt.Errorf("store/panel: unexpected time column type %T", src)
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

// boolInt 布尔按公共约定落 INTEGER 0/1。
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// wrapErr 统一包装存储错误（保持 %w 解链）。
func wrapErr(op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("store/panel: %s: %w", op, err)
}
