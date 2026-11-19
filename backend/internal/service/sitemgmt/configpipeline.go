package sitemgmt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// 本文件是 site-mgmt 对 db-tools 的 ConfigWritePipeline OHS（api-site-mgmt
// v2.8 D-21/§5.1 登记 / api-db-tools.md D-03 消费契约兑现）：R-06 四步安全
// 写入管线泛化提供——工具 location 注入/撤除经此通道（备份→写入→nginx -t
// →失败回滚），不复制管线实现。

// ConfigApply 四步管线 + 写后平滑重载（db-tools 注入/撤除即时生效；reload
// 失败=配置保留+SITE_004 语义错误，不自动回滚，D-05 同口径）。
func (s *Service) ConfigApply(ctx context.Context, file, newContent string, allowNew bool) error {
	if err := s.applyPipeline(ctx, file, newContent, allowNew); err != nil {
		return err
	}
	return s.reloadAfterWrite(ctx)
}

// ConfigRemove 撤除恢复原字节（最近写前快照经管线恢复——R-06 备份回滚
// 同构；无快照=不可恢复如实报错）。db-tools 卸载主路径采用标记 span 精确
// 移除+ConfigApply（不吞没注入后其他合法编辑，tasks.md A-DBT8）。
// v1.3（api-db-tools D-19⑥）：原态=不存在（管线 allowNew 创建的新文件，
// origin-absent 标记判定）→恢复原字节=删除文件+reload（自动落地块清块；
// 文件已不存在=幂等成功）。
func (s *Service) ConfigRemove(ctx context.Context, file string) error {
	if s.originAbsent(file) {
		if !s.gw.Exists(file) {
			return nil // 块已不存在——清块成功（幂等）
		}
		if err := s.gw.RemoveFile(file); err != nil {
			return fmt.Errorf("site-mgmt: config remove %s: %w", file, err)
		}
		return s.reloadAfterWrite(ctx)
	}
	bucket := filepath.Join(s.backupRoot, filepath.Base(file))
	entries, err := os.ReadDir(bucket)
	if err != nil {
		return fmt.Errorf("site-mgmt: config remove read backups %s: %w", bucket, err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".conf" {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return fmt.Errorf("site-mgmt: config remove %s: no backup snapshot to restore", file)
	}
	sort.Strings(names) // 定宽时间戳字典序=时间序，末位=最近
	latest := filepath.Join(bucket, names[len(names)-1])
	orig, _, err := s.gw.ReadFile(latest)
	if err != nil {
		return fmt.Errorf("site-mgmt: config remove read snapshot %s: %w", latest, err)
	}
	return s.ConfigApply(ctx, file, orig, false)
}

// originAbsentMarker 「原态=不存在」标记文件名（备份桶内，非 .conf 扩展名
// ——不参与快照保留清理与快照恢复寻址；记录管线对该文件的首次写入为
// allowNew 新建，v1.3）。
const originAbsentMarker = ".origin-absent"

// markOriginAbsent 记录文件原态=不存在（applyPipeline 新文件创建成功后
// 调用；标记持久于备份桶，先于任何后续写前快照）。
func (s *Service) markOriginAbsent(file string) error {
	bucket := filepath.Join(s.backupRoot, filepath.Base(file))
	if err := s.gw.MkdirAll(bucket); err != nil {
		return fmt.Errorf("site-mgmt: origin absent mkdir %s: %w", bucket, err)
	}
	if err := s.gw.WriteFileAtomic(filepath.Join(bucket, originAbsentMarker), []byte("absent\n")); err != nil {
		return fmt.Errorf("site-mgmt: origin absent write: %w", err)
	}
	return nil
}

// originAbsent 报告文件原态是否为不存在（管线创建的新文件）。
func (s *Service) originAbsent(file string) bool {
	return s.gw.Exists(filepath.Join(s.backupRoot, filepath.Base(file), originAbsentMarker))
}
