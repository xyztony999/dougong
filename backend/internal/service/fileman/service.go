package fileman

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"time"

	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	storefileman "github.com/dougong/dougong/internal/store/fileman"
)

// AuditRecorder 是审计 OHS 契约（*audit.Service 实现；R-03，D-09 非同事务
// 降级语义 + 两处同事务例外经 TxStore.Audit 直写）。
type AuditRecorder interface {
	Record(ctx context.Context, e audit.Entry) error
}

// Service 是文件管理器门面（22 接口的业务承载；handler 薄壳，不感知 HTTP）。
// 各领域服务方法按文件分置：projector（entry/list）、scope（path-scope）、
// transfer（上传/分块/下载）、edit（在线编辑）、entry（整理动作）、meta
// （chmod/chown/候选）、archive（压缩/解压）、trash（回收站）。
type Service struct {
	gw     FSGateway
	store  *storefileman.Store
	audit  AuditRecorder
	guard  *PathScopeGuard
	chunks *ChunkStore
	clock  func() time.Time
}

// New 构造服务（clock 为 nil 用 time.Now；rec 为审计 OHS，R-03）。
// guard 由 main 启动时自 fs_path_scope 加载注入（D-02 内存副本）。
func New(gw FSGateway, st *storefileman.Store, rec AuditRecorder, guard *PathScopeGuard) *Service {
	if guard == nil {
		guard = NewGuard(ScopeConfig{RootPath: "/"}) // 防御默认（正常装配必传）
	}
	return &Service{
		gw:     gw,
		store:  st,
		audit:  rec,
		guard:  guard,
		chunks: NewChunkStore(time.Now),
		clock:  time.Now,
	}
}

// Guard 暴露守卫（main 启动加载/装配与 api 层测试使用）。
func (s *Service) Guard() *PathScopeGuard { return s.guard }

// ChunkSweeper 暴露分块任务清理循环（main 以 goroutine 编排，G-06 唯一
// 退出路径=ctx 取消；D-10 1h 无活动作废）。
func (s *Service) ChunkSweeper(ctx context.Context, interval time.Duration) error {
	return s.chunks.RunSweeper(ctx, s.gw, interval)
}

// ---- 审计（D-09：文件系统操作审计非同事务，失败重试一次、仍失败记面板
// 运行日志兜底；下载 category=security，D-08；其余变更类 change） ----

// auditAction 记录一条操作审计（恰好一条原则；A-F10：FILE_002 越界访问
// 拒绝零审计——由调用方在记审计前判定）。
func (s *Service) auditAction(ctx context.Context, actor *authsvc.Identity,
	category, action, objectType, objectID, result string, detail any) {
	entry := buildAuditEntry(actor, category, action, objectType, objectID, result, detail)
	if err := s.audit.Record(ctx, entry); err != nil {
		if err := s.audit.Record(ctx, entry); err != nil { // 重试一次（D-09）
			slog.WarnContext(ctx, "file-manager: audit record degraded to runtime log",
				"action", action, "object", objectID, "result", result, "err", err)
		}
	}
}

// buildAuditEntry 组装审计四要素载荷（操作者快照；web 来源）。
func buildAuditEntry(actor *authsvc.Identity, category, action, objectType, objectID, result string, detail any) audit.Entry {
	e := audit.Entry{
		Category:   category,
		Action:     action,
		ObjectType: objectType,
		ObjectID:   objectID,
		Detail:     detail,
		Result:     result,
		Source:     audit.SourceWeb,
	}
	if actor != nil {
		uid := actor.UserID
		e.ActorID = &uid
		e.ActorName = actor.Username
	} else { // 防御：AuthRequired 链路下不可达
		e.ActorName = "unknown"
	}
	return e
}

// detailError 构造审计 detail 的 {error: FILE 码} 载荷（失败留痕，A-S5 先例）。
func detailError(err error) map[string]any {
	var ec *errcode.Error
	if errors.As(err, &ec) {
		return map[string]any{"error": ec.Symbol()}
	}
	return map[string]any{"error": err.Error()}
}

// isScopeReject 判定是否路径越界拒绝（D-01：越界拒绝不记审计，A-F10）。
func isScopeReject(err error) bool {
	var ec *errcode.Error
	return errors.As(err, &ec) && ec.Symbol() == errcode.ErrFilePathOutOfScope.Symbol()
}

// ---- 错误映射 ----

// mapFSErr fs 通道错误 → FILE 错误码（FILE_013 语义归一，data.reason 粗分
// permission_denied/io_error/unsupported_platform；全量细节进结构化日志）。
func (s *Service) mapFSErr(err error) error {
	switch {
	case errors.Is(err, ErrEntryNotExist):
		return errcode.ErrFileEntryNotFound
	case errors.Is(err, ErrNotADirectory):
		return errcode.ErrFileNotADirectory
	case errors.Is(err, ErrIsDirectory):
		return errcode.ErrFileNotADirectory
	case errors.Is(err, ErrUnsupportedPlatform):
		return errcode.ErrFilesystem.WithData(ReasonData{Reason: "unsupported_platform"})
	case errors.Is(err, fs.ErrPermission):
		return errcode.ErrFilesystem.WithData(ReasonData{Reason: "permission_denied"})
	default:
		return errcode.ErrFilesystem.WithData(ReasonData{Reason: "io_error"})
	}
}

// mapResolveErr guard 解析错误映射：fs.ErrNotExist → notExistCode（各接口
// 语义不同：单点动作 FILE_003 / 目录类 FILE_012，A-F16）；errcode（FILE_001
// 形态 / FILE_002 越界）原样透传；其余按 fs 错误归一。
func (s *Service) mapResolveErr(err error, notExistCode *errcode.Error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return notExistCode
	}
	var ec *errcode.Error
	if errors.As(err, &ec) {
		return ec // FILE_001 / FILE_002（guard 产出的契约错误）
	}
	return s.mapFSErr(fmt.Errorf("resolve: %w", err))
}
