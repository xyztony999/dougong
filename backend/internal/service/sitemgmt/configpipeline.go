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
// 移除+ConfigApply（不吞没注入后其他合法编辑，tasks.md A-DBT8）；本方法
// 为契约完备提供（整文件级恢复语义）。
func (s *Service) ConfigRemove(ctx context.Context, file string) error {
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
