package fileman

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	storefileman "github.com/dougong/dougong/internal/store/fileman"
)

// generateTrashName 生成 trash 内唯一标识子目录名（D-15：{deletedAt 时间戳}-
// {8 字节随机串}；A-F6：Unix 秒 + 16 位 hex；I-03 概率冲突可忽略）。
func generateTrashName(now time.Time) (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("trash name rand: %w", err)
	}
	return fmt.Sprintf("%d-%s", now.Unix(), hex.EncodeToString(buf)), nil
}

// trashPathOf 组装 trash 内条目完整路径。
func trashPathOf(trashDir, trashName string) string {
	return filepath.Join(trashDir, trashName)
}

// Delete POST /files/entry/delete（admin+CSRF）：TrashService 入站登记
// （FR-16）：生成 trash_name（D-15）→ rename 至 .dougong-trash/{标识}
// （EXDEV 降级 copy+delete，detail.degraded=true）+ trash_entry INSERT 与
// file_deleted 审计**同一事务**（I-01/D-12/D-15）。
func (s *Service) Delete(ctx context.Context, actor *authsvc.Identity, path string) (*DeleteData, error) {
	action := ActionFileDeleted
	fail := func(err error) error {
		if isScopeReject(err) { // D-01：越界拒绝零审计（A-F10）
			return err
		}
		s.auditAction(ctx, actor, audit.CategoryChange, action, ObjectTypeFileEntry,
			path, audit.ResultFailure, detailError(err))
		return err
	}
	real, err := s.guard.Resolve(path)
	if err != nil {
		return nil, fail(s.mapResolveErr(err, errcode.ErrFileEntryNotFound))
	}
	snap, err := s.gw.Stat(ctx, real)
	if err != nil {
		return nil, fail(s.mapFSErr(err))
	}
	entryType := storefileman.EntryTypeFile // 符号链接按 file 归类（§4.1.1）
	if snap.EntryType == TypeDirectory {
		entryType = storefileman.EntryTypeDirectory
	}
	size, err := s.gw.SizeOf(ctx, real)
	if err != nil {
		return nil, fail(s.mapFSErr(err))
	}
	now := s.clock()
	trashName, err := generateTrashName(now)
	if err != nil {
		return nil, fail(errcode.ErrFilesystem.WithData(ReasonData{Reason: "io_error"}))
	}
	trashDir, err := s.guard.TrashDir()
	if err != nil {
		return nil, fail(s.mapResolveErr(err, errcode.ErrFilesystem))
	}
	if err := s.gw.EnsureDir(ctx, trashDir); err != nil {
		return nil, fail(s.mapFSErr(err))
	}
	trashPath := trashPathOf(trashDir, trashName)
	degraded, err := s.moveEntry(ctx, real, trashPath)
	if err != nil {
		return nil, fail(err)
	}
	// tx：INSERT + 审计同事务（D-12/D-15）；失败补偿 rename 回原位（A-F13）。
	detail := map[string]any{}
	if degraded {
		detail["degraded"] = true
	}
	entry := storefileman.TrashEntry{
		OriginalPath: real,
		TrashName:    trashName,
		EntryType:    entryType,
		Size:         size,
		OperatorName: "unknown",
	}
	if actor != nil {
		uid := actor.UserID
		entry.OperatorID = &uid
		entry.OperatorName = actor.Username
	}
	tx, txErr := s.store.BeginTx(ctx)
	if txErr == nil {
		txErr = tx.Trash().Insert(ctx, &entry, now)
	}
	if txErr == nil {
		txErr = tx.Audit().Append(ctx, buildAuditEntry(actor, audit.CategoryChange,
			action, ObjectTypeFileEntry, real, audit.ResultSuccess, detail), now)
	}
	if txErr == nil {
		txErr = tx.Commit()
	}
	if txErr != nil {
		if rerr := tx.Rollback(); rerr != nil {
			slog.WarnContext(ctx, "file-manager: delete tx rollback", "err", rerr)
		}
		// 补偿：内容移回原位（I-01 一致性尽力恢复；补偿失败留孤儿由对账剔除）。
		if cerr := s.compensateMove(ctx, trashPath, real, degraded); cerr != nil {
			slog.ErrorContext(ctx, "file-manager: delete compensate failed (orphan left in trash)",
				"trashPath", trashPath, "original", real, "err", cerr)
		}
		return nil, fail(errcode.ErrFilesystem.WithData(ReasonData{Reason: "db_error"}))
	}
	return &DeleteData{TrashID: entry.ID, TrashName: trashName}, nil
}

// moveEntry 统一移动通道：rename 直移；EXDEV 降级 copy+delete（D-15）返回
// degraded=true，降级失败整次失败、原条目不动。
func (s *Service) moveEntry(ctx context.Context, from, to string) (degraded bool, err error) {
	if err := s.gw.Rename(ctx, from, to); err == nil {
		return false, nil
	} else if !errors.Is(err, ErrCrossDeviceRename) {
		return false, s.mapFSErr(err)
	}
	if cerr := s.gw.CopyThenRemove(ctx, from, to); cerr != nil {
		return false, s.mapFSErr(cerr)
	}
	return true, nil
}

// compensateMove 事务失败后逆向移动（A-F13）。
func (s *Service) compensateMove(ctx context.Context, from, to string, wasDegraded bool) error {
	if !wasDegraded {
		return s.gw.Rename(ctx, from, to)
	}
	return s.gw.CopyThenRemove(ctx, from, to)
}

// TrashList GET /files/trash/list（admin+readonly）：status=in_trash 全量
// （D-03 deleted_at 倒序）→ trash 实况对账（孤儿剔除+warning 标注，PRD N2/
// A-F11）。查看类不审计。
func (s *Service) TrashList(ctx context.Context) (*TrashListData, error) {
	rows, err := s.store.Trash().ListInTrash(ctx)
	if err != nil {
		return nil, s.mapFSErr(fmt.Errorf("trash list: %w", err))
	}
	trashDir, terr := s.guard.TrashDir()
	items := make([]TrashItem, 0, len(rows))
	orphans := 0
	for _, r := range rows {
		if terr != nil {
			orphans++
			continue
		}
		ok, err := s.gw.Exists(ctx, trashPathOf(trashDir, r.TrashName))
		if err != nil {
			return nil, s.mapFSErr(err)
		}
		if !ok {
			orphans++ // 面板外被清（PRD N2）：剔除并标注
			continue
		}
		items = append(items, TrashItem{
			ID:           r.ID,
			OriginalPath: r.OriginalPath,
			EntryType:    r.EntryType,
			Size:         r.Size,
			OperatorName: r.OperatorName,
			DeletedAt:    formatTS(r.DeletedAt),
		})
	}
	data := &TrashListData{Items: items, Total: len(items)}
	if orphans > 0 {
		data.Warning = fmt.Sprintf("%d 条记录因回收站内容缺失已剔除（可能被面板外改动）", orphans)
	}
	return data, nil
}

// TrashRestore POST /files/trash/restore（admin+CSRF）：校验链（I-02 在站
// FILE_014 → I-04 原路径 PathScopeGuard FILE_002 → 预检占用 D-07）→
// rename 回原路径 + status=restored 同事务（I-01）→ 审计 trash_restored。
func (s *Service) TrashRestore(ctx context.Context, actor *authsvc.Identity,
	trashID int64, onConflict string) (*RestoreData, error) {
	action := ActionTrashRestored
	if onConflict == "" {
		onConflict = ConflictAsk // 默认 ask（D-07）
	}
	if onConflict != ConflictAsk && onConflict != ConflictOverwrite {
		return nil, errcode.ErrFileInvalidPath.WithFields(errcode.FieldError{
			Field: "onConflict", Message: "还原取值域 ask|overwrite",
		})
	}
	fail := func(objectID string, err error) error {
		if isScopeReject(err) { // I-04 越界拒绝零审计（D-01/A-F10）
			return err
		}
		s.auditAction(ctx, actor, audit.CategoryChange, action, ObjectTypeTrashEntry,
			objectID, audit.ResultFailure, detailError(err))
		return err
	}
	row, err := s.store.Trash().GetByID(ctx, trashID)
	if err != nil || row.Status != storefileman.TrashStatusInTrash {
		return nil, fail(fmt.Sprint(trashID), errcode.ErrFileTrashNotFound) // I-02：非在站如实拒绝
	}
	trashDir, err := s.guard.TrashDir()
	if err != nil {
		return nil, fail(fmt.Sprint(trashID), s.mapResolveErr(err, errcode.ErrFilesystem))
	}
	origReal, _, err := s.guard.ResolveForWrite(row.OriginalPath) // I-04：原路径过守卫（父目录须在受管根内）
	if err != nil {
		return nil, fail(fmt.Sprint(trashID), s.mapResolveErr(err, errcode.ErrFileNotADirectory))
	}
	occupied, err := s.gw.Exists(ctx, origReal)
	if err != nil {
		return nil, fail(fmt.Sprint(trashID), s.mapFSErr(err))
	}
	if occupied && onConflict == ConflictAsk {
		return nil, fail(fmt.Sprint(trashID), errcode.ErrFileConflict.WithData(ConflictData{
			Conflicts: []ConflictItem{{Path: row.OriginalPath}}, // 零动作（D-07）
		}))
	}
	if occupied { // overwrite：先清占用（覆盖语义，D-07）
		if err := s.gw.RemoveTree(ctx, origReal); err != nil {
			return nil, fail(fmt.Sprint(trashID), s.mapFSErr(err))
		}
	}
	trashPath := trashPathOf(trashDir, row.TrashName)
	if _, err := s.moveEntry(ctx, trashPath, origReal); err != nil {
		return nil, fail(fmt.Sprint(trashID), err)
	}
	now := s.clock()
	tx, txErr := s.store.BeginTx(ctx)
	restored := false
	if txErr == nil {
		restored, txErr = tx.Trash().UpdateRestored(ctx, trashID, now)
		if !restored && txErr == nil {
			txErr = errcode.ErrFileTrashNotFound // 并发终态逆转防护（I-02）
		}
	}
	if txErr == nil {
		txErr = tx.Commit()
	}
	if txErr != nil {
		if tx != nil {
			if rerr := tx.Rollback(); rerr != nil {
				slog.WarnContext(ctx, "file-manager: restore tx rollback", "err", rerr)
			}
		}
		// 补偿：条目移回 trash（A-F13；失败留孤儿由对账剔除）。
		if cerr := s.gw.Rename(ctx, origReal, trashPath); cerr != nil {
			slog.ErrorContext(ctx, "file-manager: restore compensate failed (content left at original)",
				"original", origReal, "err", cerr)
		}
		return nil, fail(fmt.Sprint(trashID), errcode.ErrFilesystem.WithData(ReasonData{Reason: "db_error"}))
	}
	detail := map[string]any{}
	if occupied {
		detail["overwritten"] = true // 被覆盖事实留痕（§4.1.4）
	}
	s.auditAction(ctx, actor, audit.CategoryChange, action, ObjectTypeTrashEntry,
		fmt.Sprint(trashID), audit.ResultSuccess, detail) // D-09：restore 审计非同事务
	return &RestoreData{ID: row.ID, OriginalPath: row.OriginalPath, Status: storefileman.TrashStatusRestored}, nil
}

// TrashPurge POST /files/trash/purge（admin+CSRF）：逐条删除内容 + DELETE
// 记录同事务（I-01；purged 物理删除语义 I-02）；空站幂等成功（PRD N3）。
func (s *Service) TrashPurge(ctx context.Context, actor *authsvc.Identity) (*PurgeData, error) {
	action := ActionTrashPurged
	fail := func(err error) error {
		s.auditAction(ctx, actor, audit.CategoryChange, action, ObjectTypeTrashEntry,
			"", audit.ResultFailure, detailError(err))
		return err
	}
	rows, err := s.store.Trash().ListInTrash(ctx)
	if err != nil {
		return nil, fail(s.mapFSErr(fmt.Errorf("trash purge list: %w", err)))
	}
	if len(rows) == 0 { // 空站幂等成功（PRD N3）
		s.auditAction(ctx, actor, audit.CategoryChange, action, ObjectTypeTrashEntry,
			"", audit.ResultSuccess, map[string]any{"purgedCount": 0})
		return &PurgeData{PurgedCount: 0}, nil
	}
	trashDir, err := s.guard.TrashDir()
	if err != nil {
		return nil, fail(s.mapResolveErr(err, errcode.ErrFilesystem))
	}
	// 同事务逐条：内容删除 + 记录 DELETE（A-F12：中途 fs 失败整体回滚，
	// 已删内容成孤儿由 list 对账剔除）。
	var txErr error
	reason := "db_error"
	tx, txErr := s.store.BeginTx(ctx)
	if txErr == nil {
		for _, r := range rows {
			if err := s.gw.RemoveTree(ctx, trashPathOf(trashDir, r.TrashName)); err != nil {
				txErr, reason = err, "io_error"
				break
			}
			if err := tx.Trash().Delete(ctx, r.ID); err != nil {
				txErr, reason = err, "db_error"
				break
			}
		}
	}
	if txErr == nil {
		txErr = tx.Commit()
	}
	if txErr != nil {
		if tx != nil {
			if rerr := tx.Rollback(); rerr != nil {
				slog.WarnContext(ctx, "file-manager: purge tx rollback", "err", rerr)
			}
		}
		return nil, fail(errcode.ErrFilesystem.WithData(ReasonData{Reason: reason}))
	}
	s.auditAction(ctx, actor, audit.CategoryChange, action, ObjectTypeTrashEntry,
		"", audit.ResultSuccess, map[string]any{"purgedCount": len(rows)})
	return &PurgeData{PurgedCount: len(rows)}, nil
}
