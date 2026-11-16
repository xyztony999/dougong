package fileman

import (
	"context"
	"log/slog"

	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
)

// GetScope GET /files/path-scope/get（admin）：fs_path_scope 当前值（内存
// 副本数据源，D-02）。查看类不审计（A-F10）。
func (s *Service) GetScope() *ScopeData {
	snap := s.guard.Snapshot()
	return &ScopeData{RootPath: snap.RootPath, ExcludedDirs: snap.ExcludedDirs}
}

// UpdateScope POST /files/path-scope/update（admin+CSRF）：R-14 校验 →
// UPDATE fs_path_scope + 审计同事务（D-02/D-09 例外）→ 提交后刷新进程内
// 副本（后续请求即予生效）。
func (s *Service) UpdateScope(ctx context.Context, actor *authsvc.Identity,
	rootPath string, excludedDirs []string) (*ScopeData, error) {
	action := ActionPathScopeUpdated
	fail := func(err error) error {
		// update 自身校验失败属变更尝试留痕（A-F10：D-01 越界豁免仅指携带
		// 路径接口的访问越界拒绝，不含本接口参数校验）。
		s.auditAction(ctx, actor, audit.CategoryChange, action, ObjectTypeFsPathScope,
			rootPath, audit.ResultFailure, detailError(err))
		return err
	}
	cfg, err := ValidateScopeUpdate(rootPath, excludedDirs)
	if err != nil {
		return nil, fail(err)
	}
	now := s.clock()
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return nil, fail(errcode.ErrFilesystem.WithData(ReasonData{Reason: "db_error"}))
	}
	defer func() {
		if rerr := tx.Rollback(); rerr != nil {
			slog.WarnContext(ctx, "file-manager: scope tx rollback", "err", rerr)
		}
	}()
	if err := tx.Scope().Update(ctx, cfg.RootPath, cfg.ExcludedDirs, now); err != nil {
		return nil, fail(errcode.ErrFilesystem.WithData(ReasonData{Reason: "db_error"}))
	}
	if err := tx.Audit().Append(ctx, buildAuditEntry(actor, audit.CategoryChange, action,
		ObjectTypeFsPathScope, cfg.RootPath, audit.ResultSuccess, nil), now); err != nil {
		return nil, fail(errcode.ErrFilesystem.WithData(ReasonData{Reason: "db_error"}))
	}
	if err := tx.Commit(); err != nil {
		return nil, fail(errcode.ErrFilesystem.WithData(ReasonData{Reason: "db_error"}))
	}
	s.guard.Refresh(cfg) // 事务提交后刷新副本（D-02：后续请求即予生效）
	return &ScopeData{RootPath: cfg.RootPath, ExcludedDirs: cfg.ExcludedDirs}, nil
}
