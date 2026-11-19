-- 014 · env-mgmt（v1.3 Node 增量：env_type CHECK 扩八值 +node，prd-env-mgmt
-- FR-08/⚠️A16；api-db-tools D-18 跨模块待同步项本轮落定）。SQLite CHECK 约束
-- 不可 ALTER，迁移内表重建（单事务原子，沿 010 先例：建新表〔八值 CHECK〕→
-- 全量拷贝〔含 012 bootstrap_status/bootstrap_note 引导结果两列〕→ 弃旧表 →
-- 改名 → 重建两索引；AUTOINCREMENT 序列随显式 id 拷贝保持，既有台账零丢失）。
-- 013 已被 db-tools 两表占用（api-db-tools §5.3 钉 013）。
CREATE TABLE env_install_task_rebuild (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  env_type     TEXT    NOT NULL CHECK (env_type IN ('nginx','apache','docker','mysql','redis','php_fpm','mongodb','node')),
  action       TEXT    NOT NULL CHECK (action IN ('install','upgrade','uninstall')),
  status       TEXT    NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','succeeded','failed')),
  output_tail  TEXT    NOT NULL DEFAULT '',
  error        TEXT,
  requested_by TEXT    NOT NULL,
  created_at   DATETIME NOT NULL,
  updated_at   DATETIME NOT NULL,
  started_at   DATETIME,
  finished_at  DATETIME,
  bootstrap_status TEXT CHECK (bootstrap_status IN ('succeeded','failed','skipped')),
  bootstrap_note   TEXT
);
INSERT INTO env_install_task_rebuild (id, env_type, action, status, output_tail, error,
  requested_by, created_at, updated_at, started_at, finished_at, bootstrap_status, bootstrap_note)
  SELECT id, env_type, action, status, output_tail, error,
         requested_by, created_at, updated_at, started_at, finished_at, bootstrap_status, bootstrap_note
  FROM env_install_task;
DROP TABLE env_install_task;
ALTER TABLE env_install_task_rebuild RENAME TO env_install_task;
CREATE INDEX IF NOT EXISTS idx_env_task_env_status ON env_install_task(env_type, status);
CREATE INDEX IF NOT EXISTS idx_env_task_created ON env_install_task(created_at);