package panel

import (
	"context"

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
