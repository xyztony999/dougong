package cronback

import (
	"context"
	"os"

	storeauth "github.com/dougong/dougong/internal/store/auth"
	storecron "github.com/dougong/dougong/internal/store/cronback"
)

// storeapi.go 将 *storecron.Store / *storecron.TxStore 适配为 StoreAPI/TxAPI
//（BeginTx 返回具体类型与接口签名的桥接；行为纯转发——dbmgmt.AdaptStore
// 同法）。

// storeAdapter 适配 *storecron.Store。
type storeAdapter struct{ s *storecron.Store }

// AdaptStore 包装 *storecron.Store 为 StoreAPI（main 装配与测试共用）。
func AdaptStore(s *storecron.Store) StoreAPI { return storeAdapter{s: s} }

// Job 返回任务仓储。
func (a storeAdapter) Job() JobRepoAPI { return a.s.Job() }

// Run 返回执行历史仓储。
func (a storeAdapter) Run() RunRepoAPI { return a.s.Run() }

// BeginTx 开启事务并返回接口视图。
func (a storeAdapter) BeginTx(ctx context.Context) (TxAPI, error) {
	tx, err := a.s.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	return txAdapter{t: tx}, nil
}

// txAdapter 适配 *storecron.TxStore。
type txAdapter struct{ t *storecron.TxStore }

// Job 返回事务内任务仓储。
func (a txAdapter) Job() JobRepoAPI { return a.t.Job() }

// Run 返回事务内执行历史仓储。
func (a txAdapter) Run() RunRepoAPI { return a.t.Run() }

// Audit 返回事务内审计仓储（同事务写入，R-03）。
func (a txAdapter) Audit() *storeauth.AuditRepo { return a.t.Audit() }

// Commit 提交事务。
func (a txAdapter) Commit() error { return a.t.Commit() }

// Rollback 回滚事务。
func (a txAdapter) Rollback() error { return a.t.Rollback() }

// statSize 产物文件字节数。
func statSize(path string) (int64, error) {
	st, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return st.Size(), nil
}
