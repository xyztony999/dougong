package dbmgmt

import (
	"context"
	"io"
	"sort"
)

// 本文件是 database-mgmt 对 cron-backup 的进程内 OHS（api-database-mgmt.md
// §4.1.5/§5.1 预告的 DbGateway.DumpDatabase 消费契约兑现——api-cron-backup.md
// §3.2 正式登记消费）。凭据解密在本域内完成（governTargetByID → vault），
// 明文不出域（⚠️A18/D-02）；不占 busy 互斥（其为交互导出/导入 UX 互斥 D-12；
// 备份线执行互斥由 backup_run.idx_run_running 承载，api-cron-backup I-01）。

// systemSchemas 整实例全库展开时剔除的系统库（mysqldump --all-databases 同
// 语义排除；api-cron-backup A-C6）。
var systemSchemas = map[string]bool{
	"information_schema": true,
	"performance_schema": true,
	"sys":                true,
}

// DumpStream 单库导出流（cron-backup 执行编排消费）：I-05 前置链（档案/引擎/
// 可达/凭据——错误携带 DBM 语义，调用方按执行失败呈现）→ charset 按库实况
// 尽力解析（查不到不阻塞，mysqldump 按默认字符集导出）→ mysqldump 流。
// 流由调用方 Close（进程生命周期见 DumpReader）。
func (s *Service) DumpStream(ctx context.Context, instanceID int64, database string) (io.ReadCloser, error) {
	_, t, c, err := s.governTargetByID(ctx, instanceID, true)
	if err != nil {
		return nil, err
	}
	charset := ""
	if dbs, lerr := s.mysql.ListDatabases(ctx, t, c); lerr == nil {
		for _, d := range dbs {
			if d.Name == database {
				charset = d.Charset
				break
			}
		}
	}
	return s.mysql.DumpDatabase(ctx, t, c, database, charset)
}

// DatabaseNames 实况库清单（整实例全库备份项的执行期展开，已滤系统库；按名
// 字典序稳定产出——产物文件序确定性）。
func (s *Service) DatabaseNames(ctx context.Context, instanceID int64) ([]string, error) {
	_, t, c, err := s.governTargetByID(ctx, instanceID, true)
	if err != nil {
		return nil, err
	}
	dbs, err := s.mysql.ListDatabases(ctx, t, c)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	names := make([]string, 0, len(dbs))
	for _, d := range dbs {
		if systemSchemas[d.Name] {
			continue
		}
		names = append(names, d.Name)
	}
	sort.Strings(names)
	return names, nil
}

// InstanceArchive db_instance 档案存在性与显示名（cron-backup
// InstanceRefChecker 数据源：创建/编辑时悬空拒绝 CB_001——只读档案校验，
// 不要求可达；执行期悬空→run.failure 不级联，api-cron-backup §4.1.1）。
// instanceName 为产物命名标签（socket→"local"、tcp→"host-port"，hostPortLabel
// 既有口径，D-10 db-{实例名}-{库名}.sql.gz）。
func (s *Service) InstanceArchive(ctx context.Context, instanceID int64) (found bool, engine, instanceName string, err error) {
	inst, err := s.store.Instance().GetByID(ctx, instanceID)
	if err != nil {
		return false, "", "", err
	}
	if inst == nil {
		return false, "", "", nil
	}
	return true, inst.Engine, hostPortLabel(targetOf(inst)), nil
}
