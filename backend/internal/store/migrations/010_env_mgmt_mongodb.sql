-- 010 · env-mgmt（W1 实机冒烟修复轮：env_type CHECK 扩七值 +mongodb）
-- ⚠️A1 六→七环境目录（用户点名补 MongoDB）；SQLite CHECK 约束不可 ALTER，
-- 迁移内表重建（单事务原子：建新表〔七值 CHECK〕→ 全量拷贝 → 弃旧表 → 改名
-- → 重建两索引；AUTOINCREMENT 序列随显式 id 拷贝写入 sqlite_sequence 保持，
-- 既有任务台账行零丢失）。008 初建、009 已被 site-mgmt DNS-01 占用。
CREATE TABLE env_install_task_rebuild (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  env_type     TEXT    NOT NULL CHECK (env_type IN ('nginx','apache','docker','mysql','redis','php_fpm','mongodb')),
  action       TEXT    NOT NULL CHECK (action IN ('install','upgrade','uninstall')),
  status       TEXT    NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','succeeded','failed')),
  output_tail  TEXT    NOT NULL DEFAULT '',
  error        TEXT,
  requested_by TEXT    NOT NULL,
  created_at   DATETIME NOT NULL,
  updated_at   DATETIME NOT NULL,
  started_at   DATETIME,
  finished_at  DATETIME
);
INSERT INTO env_install_task_rebuild (id, env_type, action, status, output_tail, error,
  requested_by, created_at, updated_at, started_at, finished_at)
  SELECT id, env_type, action, status, output_tail, error,
         requested_by, created_at, updated_at, started_at, finished_at FROM env_install_task;
DROP TABLE env_install_task;
ALTER TABLE env_install_task_rebuild RENAME TO env_install_task;
CREATE INDEX IF NOT EXISTS idx_env_task_env_status ON env_install_task(env_type, status);
CREATE INDEX IF NOT EXISTS idx_env_task_created ON env_install_task(created_at);
