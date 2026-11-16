package fileman

import (
	"context"
	"unicode/utf8"

	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
)

// Content GET /files/entry/content（admin）：编辑打开（D-05）——大小 ≤1MiB
// （FILE_011）→ 前 8KB 空字节+UTF-8 探测（FILE_006 二进制拒绝）。打开未变更
// 不审计。
func (s *Service) Content(ctx context.Context, path string) (*ContentData, error) {
	real, err := s.guard.Resolve(path)
	if err != nil {
		return nil, s.mapResolveErr(err, errcode.ErrFileEntryNotFound)
	}
	snap, err := s.gw.Stat(ctx, real)
	if err != nil {
		return nil, s.mapFSErr(err)
	}
	if snap.EntryType == TypeDirectory {
		return nil, errcode.ErrFileNotADirectory
	}
	if snap.Size > EditSizeLimit {
		return nil, errcode.ErrFileTooLarge.WithData(ReasonData{
			Reason: "编辑上限 1MiB，请下载后本地处理",
		})
	}
	data, err := s.gw.ReadFile(ctx, real, EditSizeLimit)
	if err != nil {
		return nil, s.mapFSErr(err)
	}
	if !editableProbe(data) { // 二进制探测（D-05：前 8KB 窗口）
		return nil, errcode.ErrFileNotEditable
	}
	return &ContentData{
		Path:    real,
		Size:    snap.Size,
		Mtime:   formatTS(snap.Mtime),
		Content: string(data),
	}, nil
}

// editableProbe 前 8KB 空字节（\0）检测 + UTF-8 有效性探测（D-05/EDIT_BINARY_PROBE）。
func editableProbe(data []byte) bool {
	probe := data
	if len(probe) > EditBinaryProbe {
		probe = probe[:EditBinaryProbe]
	}
	for _, b := range probe {
		if b == 0 {
			return false
		}
	}
	return utf8.Valid(probe)
}

// ContentSave POST /files/entry/content-save（admin+CSRF）：UTF-8 + ≤1MiB
// （FILE_011）→ 临时文件+rename 全量覆盖（I-09 最后写者胜，无乐观锁 D-05）。
func (s *Service) ContentSave(ctx context.Context, actor *authsvc.Identity,
	path, content string) (*SaveResultData, error) {
	action := ActionFileContentSaved
	fail := func(err error) error {
		if isScopeReject(err) {
			return err
		}
		s.auditAction(ctx, actor, audit.CategoryChange, action, ObjectTypeFileEntry,
			path, audit.ResultFailure, detailError(err))
		return err
	}
	raw := []byte(content)
	if int64(len(raw)) > EditSizeLimit {
		return nil, fail(errcode.ErrFileTooLarge.WithData(ReasonData{
			Reason: "编辑内容上限 1MiB",
		}))
	}
	if !utf8.Valid(raw) {
		return nil, fail(errcode.ErrFileNotEditable)
	}
	// 写路径统一解析（A-F2：目标已存在且为符号链接时全链解析防逃逸写出）。
	target, existed, err := s.guard.ResolveForWrite(path)
	if err != nil {
		return nil, fail(s.mapResolveErr(err, errcode.ErrFileEntryNotFound))
	}
	_ = existed
	if err := s.gw.WriteFileAtomic(ctx, target, raw); err != nil {
		return nil, fail(s.mapFSErr(err))
	}
	snap, err := s.gw.Stat(ctx, target)
	if err != nil {
		return nil, fail(s.mapFSErr(err))
	}
	s.auditAction(ctx, actor, audit.CategoryChange, action, ObjectTypeFileEntry,
		target, audit.ResultSuccess, nil)
	return &SaveResultData{Path: target, Size: snap.Size, Mtime: formatTS(snap.Mtime)}, nil
}
