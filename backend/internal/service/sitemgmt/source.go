package sitemgmt

import (
	"context"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/dougong/dougong/internal/adapter/nginxx"
	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
)

// SourceEditService（§4.2.1，D-06 乐观锁）：源码读取带 baseMtime 基线，
// 保存回传基线与当前文件 mtime 比对——不一致 SITE_010（面板外修改检测，
// 打开→保存窗口冲突保护）；保存前 nginxx.ValidateSyntax 自检（A-SM8：
// 站点片段无法独立 `nginx -t`，语法级拒收零落盘；真实 -t 由管线第三步执行）。

// ReadSource GET /sites/site/source：来源文件可读 + 大小上限（SITE_014）。
func (s *Service) ReadSource(ctx context.Context, siteID string) (*SourceData, error) {
	file, _, err := parseSiteID(siteID)
	if err != nil {
		return nil, err
	}
	e, err := s.resolveSite(ctx, siteID)
	if err != nil {
		return nil, err
	}
	_ = file
	content, mtime, rerr := s.gw.ReadFile(e.File)
	if rerr != nil {
		return nil, fmt.Errorf("site-mgmt: read source %s: %w", e.File, rerr)
	}
	if int64(len(content)) > SourceSizeLimit {
		return nil, errcode.ErrSiteInvalidSource.WithData(ReasonData{
			Reason: fmt.Sprintf("文件 %d 字节超编辑上限 %d 字节", len(content), SourceSizeLimit),
		})
	}
	return &SourceData{
		SiteID: siteID, SourceFile: e.File, Content: content,
		BaseMtime: mtime.UTC().Format(time.RFC3339Nano), Size: int64(len(content)),
	}, nil
}

// SaveSource POST /sites/site/source-save：content UTF-8 且 ≤1MiB（SITE_014）
// → baseMtime 比对（SITE_010 data.currentMtime）→ 语法自检（SITE_003 语义
// 拒收零落盘，A-SM8）→ WritePipeline → reload → 审计 site_source_saved。
func (s *Service) SaveSource(ctx context.Context, actor *authsvc.Identity, req SourceSaveRequest) (*SourceSaveData, error) {
	if !utf8.ValidString(req.Content) {
		return nil, errcode.ErrSiteInvalidSource.WithData(ReasonData{Reason: "content 非 UTF-8"})
	}
	if int64(len(req.Content)) > SourceSizeLimit {
		return nil, errcode.ErrSiteInvalidSource.WithData(ReasonData{
			Reason: fmt.Sprintf("内容 %d 字节超编辑上限 %d 字节", len(req.Content), SourceSizeLimit),
		})
	}
	if err := s.requireNginx(); err != nil {
		return nil, err
	}
	e, err := s.resolveSite(ctx, req.SiteID)
	if err != nil {
		return nil, err
	}
	_, cur, rerr := s.gw.ReadFile(e.File)
	if rerr != nil {
		return nil, fmt.Errorf("site-mgmt: read source %s: %w", e.File, rerr)
	}
	base, perr := time.Parse(time.RFC3339Nano, req.BaseMtime)
	if perr != nil {
		return nil, fieldError("baseMtime", "baseMtime 须为 RFC3339 时间")
	}
	if !cur.Equal(base) { // D-06：窗口内外部修改 → 基线过期
		return nil, errcode.ErrSourceStale.WithData(ReasonData{
			CurrentMtime: cur.UTC().Format(time.RFC3339Nano),
		})
	}
	if issues := nginxx.ValidateSyntax(req.Content); len(issues) > 0 { // A-SM8
		return nil, errcode.ErrNginxTestFailed.WithData(SiteErrorData{Errors: issues})
	}
	if err := s.applyPipeline(ctx, e.File, req.Content, false); err != nil {
		return nil, err
	}
	s.auditAction(ctx, actor, AuditActionSiteSourceSaved, ObjectTypeSite, req.SiteID,
		map[string]any{"size": len(req.Content)}, true)
	_, after, aerr := s.gw.ReadFile(e.File) // 新基线=写回后 mtime
	if aerr != nil {
		return nil, fmt.Errorf("site-mgmt: re-read source %s: %w", e.File, aerr)
	}
	newBase := after.UTC().Format(time.RFC3339Nano)
	if rerr := s.reloadAfterWrite(ctx); rerr != nil {
		return &SourceSaveData{SiteID: req.SiteID, BaseMtime: newBase}, rerr
	}
	return &SourceSaveData{SiteID: req.SiteID, BaseMtime: newBase}, nil
}
