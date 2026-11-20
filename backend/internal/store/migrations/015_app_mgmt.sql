-- 015_app_mgmt.sql · 应用管理（app-mgmt）双聚合根初建（api-app-mgmt.md §5.3 逐字）。
-- app：应用档案（UNIQUE name 承载 I-02；unit_name 唯一索引承载 I-01 派生一致性）。
-- app_task：应用变更任务台账（四态状态机 ⚠️A3，与 env_install_task 同构；
-- app_id+app_name 快照沿 backup_job 先例；purge_dir 承载删除范围显式性 I-10）。

CREATE TABLE IF NOT EXISTS app (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  name         TEXT    NOT NULL UNIQUE,
  dir          TEXT    NOT NULL,
  entry        TEXT    NOT NULL,
  start_mode   TEXT    NOT NULL CHECK (start_mode IN ('preset_fastapi','custom')),
  start_spec   TEXT    NOT NULL DEFAULT '{}',
  listen_port  INTEGER NOT NULL CHECK (listen_port BETWEEN 1024 AND 65535),
  unit_name    TEXT    NOT NULL,
  created_by   TEXT    NOT NULL,
  created_at   DATETIME NOT NULL,
  updated_at   DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_app_unit ON app(unit_name);

CREATE TABLE IF NOT EXISTS app_task (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  app_id       INTEGER NOT NULL DEFAULT 0,
  app_name     TEXT    NOT NULL,
  action       TEXT    NOT NULL CHECK (action IN ('create','deps_install','delete')),
  status       TEXT    NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','succeeded','failed')),
  purge_dir    INTEGER NOT NULL DEFAULT 0,
  output_tail  TEXT    NOT NULL DEFAULT '',
  error        TEXT,
  requested_by TEXT    NOT NULL,
  created_at   DATETIME NOT NULL,
  updated_at   DATETIME NOT NULL,
  started_at   DATETIME,
  finished_at  DATETIME
);
CREATE INDEX IF NOT EXISTS idx_app_task_app_status ON app_task(app_id, status);
CREATE INDEX IF NOT EXISTS idx_app_task_created ON app_task(created_at);
