package sitemgmt

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/dougong/dougong/internal/adapter/nginxx"
	"github.com/dougong/dougong/internal/errcode"
)

// WritePipeline（D-04，I-11 管线强制）：全部配置写入的唯一放行口径——
// 备份（SITE_013 失败即整体不执行，配置未动）→ 修改（原子写）→ `nginx -t`
// （配置错误 → 自动恢复写前备份并报错 diff，SITE_003）→ 尾部清理最旧快照
// （CONF_BACKUP_KEEP）。reload 独立于管线（D-05 语义由调用方呈现）；
// 通道不可用（exec 失败/超时，A-SM18）→ 回滚 + SITE_012。
func (s *Service) applyPipeline(ctx context.Context, file, newContent string, allowNew bool) error {
	orig := ""
	exists := s.gw.Exists(file)
	if exists {
		b, _, err := s.gw.ReadFile(file)
		if err != nil {
			return fmt.Errorf("site-mgmt pipeline: read original %s: %w", file, err)
		}
		orig = b
	} else if !allowNew {
		return fmt.Errorf("site-mgmt pipeline: target %s not found", file)
	}

	// 第一步：写前快照（新文件无可备份原样——回滚语义=删除新文件）。
	if exists {
		bak, err := s.backupPathFor(file)
		if err != nil {
			return errcode.ErrSiteBackupFailed.WithData(ReasonData{Reason: err.Error()})
		}
		if err := s.gw.CopyFile(file, bak); err != nil {
			return errcode.ErrSiteBackupFailed.WithData(ReasonData{Reason: err.Error()})
		}
	}

	// 第二步：修改（临时文件 + rename 原子替换；span 模型保证未编辑字节
	// 恒不变——R-16 其余站点零影响由构造保证，A-SM2）。
	if err := s.gw.WriteFileAtomic(file, []byte(newContent)); err != nil {
		return fmt.Errorf("site-mgmt pipeline: write %s: %w", file, err)
	}

	// 第三步：`nginx -t` 校验；失败自动恢复写前内容（R-06）。
	tr, terr := s.gw.TestConfig(ctx)
	if terr != nil {
		s.rollback(ctx, file, exists, orig)
		return errcode.ErrNginxUnavailable.WithData(ReasonData{Reason: terr.Error()})
	}
	if !tr.OK {
		diff := unifiedDiff(orig, newContent)
		if rerr := s.rollback(ctx, file, exists, orig); rerr != nil {
			slog.ErrorContext(ctx, "site-mgmt pipeline: rollback failed after nginx -t failure",
				"file", file, "err", rerr)
		}
		return errcode.ErrNginxTestFailed.WithData(SiteErrorData{Errors: tr.Output, Diff: diff})
	}

	// 尾部：快照保留清理（每来源文件最近 CONF_BACKUP_KEEP 份，D-04）。
	if exists {
		s.pruneBackups(ctx, file)
	}
	return nil
}

// rollback 恢复写前内容（新文件回滚=删除；删除失败记日志不掩盖主错误）。
func (s *Service) rollback(ctx context.Context, file string, existed bool, orig string) error {
	if !existed {
		if err := s.gw.RemoveFile(file); err != nil {
			return fmt.Errorf("rollback remove new file: %w", err)
		}
		return nil
	}
	if err := s.gw.WriteFileAtomic(file, []byte(orig)); err != nil {
		return fmt.Errorf("rollback restore: %w", err)
	}
	return nil
}

// backupPathFor 生成快照路径：{backupRoot}/{文件基名}/{unixnano}.conf
// （R-06：conf.backup/时间戳/；基名分桶隔离同域文件）。
func (s *Service) backupPathFor(file string) (string, error) {
	bucket := filepath.Join(s.backupRoot, filepath.Base(file))
	if err := s.gw.MkdirAll(bucket); err != nil {
		return "", fmt.Errorf("backup dir %s: %w", bucket, err)
	}
	return filepath.Join(bucket, strconv.FormatInt(s.clock().UnixNano(), 10)+".conf"), nil
}

// pruneBackups 清理最旧快照至 CONF_BACKUP_KEEP 份（文件名=定宽时间戳，
// 字典序即时间序）。
func (s *Service) pruneBackups(ctx context.Context, file string) {
	bucket := filepath.Join(s.backupRoot, filepath.Base(file))
	entries, err := os.ReadDir(bucket)
	if err != nil {
		return // 清理失败不影响写回主流程（下次写回再清）
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".conf" {
			names = append(names, e.Name())
		}
	}
	if len(names) <= ConfBackupKeep {
		return
	}
	sort.Strings(names) // 升序=旧在前
	for _, n := range names[:len(names)-ConfBackupKeep] {
		if rerr := s.gw.RemoveFile(filepath.Join(bucket, n)); rerr != nil {
			slog.WarnContext(ctx, "site-mgmt: prune backup failed", "file", n, "err", rerr)
		}
	}
}

// requireNginx 写接口/nginx 可用性前置（I-12：SITE_012 data.reason）。
func (s *Service) requireNginx() error {
	if err := s.gw.Available(); err != nil {
		return errcode.ErrNginxUnavailable.WithData(ReasonData{Reason: err.Error()})
	}
	return nil
}

// reloadAfterWrite 写回成功后的平滑重载（D-05：reload 失败=配置保留 +
// SITE_004 data.output，不自动回滚）。
func (s *Service) reloadAfterWrite(ctx context.Context) error {
	out, err := s.gw.Reload(ctx)
	if err != nil {
		if isChannelErr(err) {
			return errcode.ErrNginxUnavailable.WithData(ReasonData{Reason: err.Error()})
		}
		return errcode.ErrReloadFailed.WithData(SiteErrorData{Output: out})
	}
	return nil
}

// isChannelErr 判定 reload 错误是否为通道不可用（ErrUnavailable 哨兵链）。
func isChannelErr(err error) bool {
	return nginxx.IsUnavailable(err)
}
