package fileman

import (
	"context"
	"path/filepath"

	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
)

// resolveDir 解析目录入参：形态（FILE_001）→ 守卫（FILE_002）→ 实况为目录
// （FILE_012；不存在合并 FILE_012，A-F16）。
func (s *Service) resolveDir(ctx context.Context, dir string) (string, error) {
	real, err := s.guard.Resolve(dir)
	if err != nil {
		return "", s.mapResolveErr(err, errcode.ErrFileNotADirectory)
	}
	snap, err := s.gw.Stat(ctx, real)
	if err != nil {
		return "", s.mapFSErr(err)
	}
	if snap.EntryType != TypeDirectory {
		return "", errcode.ErrFileNotADirectory
	}
	return real, nil
}

// Create POST /files/entry/create（admin+CSRF）：新建空文件/空目录（FR-09）；
// 同名已存在 FILE_004 直接错误（D-13 不覆盖）；幂等语义=重复提交 FILE_004。
func (s *Service) Create(ctx context.Context, actor *authsvc.Identity,
	dir, name, entryType string) (*CreateData, error) {
	action := ActionFileCreated
	fail := func(err error) error {
		if isScopeReject(err) {
			return err
		}
		s.auditAction(ctx, actor, audit.CategoryChange, action, ObjectTypeFileEntry,
			filepath.Join(dir, name), audit.ResultFailure, detailError(err))
		return err
	}
	if err := ValidateName(name); err != nil { // FILE_007
		return nil, fail(err)
	}
	if entryType != TypeFile && entryType != TypeDirectory { // A-F9
		return nil, fail(errcode.ErrFileInvalidPath.WithFields(errcode.FieldError{
			Field: "type", Message: "取值域 file|directory",
		}))
	}
	dirReal, err := s.resolveDir(ctx, dir)
	if err != nil {
		return nil, fail(err)
	}
	target := filepath.Join(dirReal, name)
	exists, err := s.gw.Exists(ctx, target)
	if err != nil {
		return nil, fail(s.mapFSErr(err))
	}
	if exists {
		return nil, fail(errcode.ErrFileEntryExists) // D-13
	}
	if entryType == TypeDirectory {
		err = s.gw.Mkdir(ctx, target)
	} else {
		err = s.gw.CreateFile(ctx, target)
	}
	if err != nil {
		return nil, fail(s.mapFSErr(err))
	}
	s.auditAction(ctx, actor, audit.CategoryChange, action, ObjectTypeFileEntry,
		target, audit.ResultSuccess, map[string]any{"type": entryType})
	return &CreateData{Path: target, EntryType: entryType}, nil
}

// Rename POST /files/entry/rename（admin+CSRF）：原目录内重命名；同目录同名
// 已存在 FILE_004（D-13）。
func (s *Service) Rename(ctx context.Context, actor *authsvc.Identity,
	path, newName string) (*RenameData, error) {
	action := ActionFileRenamed
	fail := func(objectID string, err error) error {
		if isScopeReject(err) {
			return err
		}
		s.auditAction(ctx, actor, audit.CategoryChange, action, ObjectTypeFileEntry,
			objectID, audit.ResultFailure, detailError(err))
		return err
	}
	if err := ValidateName(newName); err != nil { // FILE_007
		return nil, fail(path, err)
	}
	real, err := s.guard.Resolve(path)
	if err != nil {
		return nil, fail(path, s.mapResolveErr(err, errcode.ErrFileEntryNotFound))
	}
	dir := filepath.Dir(real)
	target := filepath.Join(dir, newName)
	exists, err := s.gw.Exists(ctx, target)
	if err != nil {
		return nil, fail(real, s.mapFSErr(err))
	}
	if exists {
		return nil, fail(real, errcode.ErrFileEntryExists) // D-13
	}
	if err := s.gw.Rename(ctx, real, target); err != nil {
		return nil, fail(real, s.mapFSErr(err)) // 同目录 rename 无 EXDEV 场景
	}
	s.auditAction(ctx, actor, audit.CategoryChange, action, ObjectTypeFileEntry,
		target, audit.ResultSuccess, map[string]any{"oldPath": real})
	return &RenameData{Path: target}, nil
}

// Move POST /files/entry/move（admin+CSRF）：跨目录移动（含子树）；目标同名
// 预检（D-07 ask→FILE_005 零动作 / skip→跳过 / overwrite→覆盖）；EXDEV 降级
// copy+delete（detail.degraded=true，D-15 同口径）。
func (s *Service) Move(ctx context.Context, actor *authsvc.Identity,
	sourcePath, targetDir, onConflict string) (*MoveData, error) {
	action := ActionFileMoved
	if onConflict == "" {
		onConflict = ConflictAsk
	}
	if onConflict != ConflictAsk && onConflict != ConflictOverwrite && onConflict != ConflictSkip {
		return nil, errcode.ErrFileInvalidPath.WithFields(errcode.FieldError{
			Field: "onConflict", Message: "取值域 ask|overwrite|skip",
		})
	}
	fail := func(objectID string, err error) error {
		if isScopeReject(err) {
			return err
		}
		s.auditAction(ctx, actor, audit.CategoryChange, action, ObjectTypeFileEntry,
			objectID, audit.ResultFailure, detailError(err))
		return err
	}
	srcReal, err := s.guard.Resolve(sourcePath)
	if err != nil {
		return nil, fail(sourcePath, s.mapResolveErr(err, errcode.ErrFileEntryNotFound))
	}
	dstDirReal, err := s.resolveDir(ctx, targetDir)
	if err != nil {
		return nil, fail(sourcePath, err)
	}
	name := filepath.Base(srcReal)
	dst := filepath.Join(dstDirReal, name)
	if dst == srcReal {
		return &MoveData{Moved: []MoveItem{}, SkippedCount: 0}, nil // 同位置移动=无操作成功
	}
	occupied, err := s.gw.Exists(ctx, dst)
	if err != nil {
		return nil, fail(srcReal, s.mapFSErr(err))
	}
	if occupied && onConflict == ConflictAsk {
		return nil, fail(srcReal, errcode.ErrFileConflict.WithData(ConflictData{
			Conflicts: []ConflictItem{{Path: dst}}, // 零动作（D-07）
		}))
	}
	if occupied && onConflict == ConflictSkip {
		s.auditAction(ctx, actor, audit.CategoryChange, action, ObjectTypeFileEntry,
			srcReal, audit.ResultSuccess, map[string]any{"skippedCount": 1})
		return &MoveData{Moved: []MoveItem{}, SkippedCount: 1}, nil
	}
	if occupied { // overwrite：先清占用（覆盖语义，D-07）
		if err := s.gw.RemoveTree(ctx, dst); err != nil {
			return nil, fail(srcReal, s.mapFSErr(err))
		}
	}
	degraded, err := s.moveEntry(ctx, srcReal, dst)
	if err != nil {
		return nil, fail(srcReal, err)
	}
	detail := map[string]any{}
	if degraded {
		detail["degraded"] = true
	}
	s.auditAction(ctx, actor, audit.CategoryChange, action, ObjectTypeFileEntry,
		dst, audit.ResultSuccess, detail)
	return &MoveData{
		Moved:        []MoveItem{{From: srcReal, To: dst}},
		SkippedCount: 0,
	}, nil
}
