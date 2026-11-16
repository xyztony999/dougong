-- cron-backup（计划任务与备份，M8）——api-cron-backup.md §5.3 DDL 逐字落库。
-- backup_job：备份任务主数据（源清单/目标为 JSON 值列内嵌，domain-model v1.14 D2）；
-- backup_run：执行历史（append+终态一次转移 I-02；job_id 弱引用+job_name 快照 D-12；
-- idx_run_running 部分唯一索引承载执行互斥 I-01/D-08）。

CREATE TABLE IF NOT EXISTS backup_job (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  name            TEXT    NOT NULL UNIQUE,
  source_items    TEXT    NOT NULL,              -- JSON 值集合：[{type:'site_dir',path}|{type:'database',instanceId,database?}]
  target_type     TEXT    NOT NULL CHECK (target_type IN ('local','rsync')),
  target_config   TEXT    NOT NULL,              -- JSON：{path} | {host,user,port,path}（无凭据字段，D-06）
  retention_count INTEGER NOT NULL CHECK (retention_count BETWEEN 1 AND 100),
  schedule_cron   TEXT    NOT NULL,              -- 五段表达式（宏已归一化，D-03）
  created_at      DATETIME NOT NULL,
  updated_at      DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS backup_run (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  job_id        INTEGER NOT NULL,                -- 弱引用（无 FK，D-12：任务删除后历史留存）
  job_name      TEXT    NOT NULL,                -- 冗余快照（I-03）
  trigger       TEXT    NOT NULL CHECK (trigger IN ('cron','manual')),
  status        TEXT    NOT NULL DEFAULT 'running' CHECK (status IN ('running','success','failure')),
  started_at    DATETIME NOT NULL,
  finished_at   DATETIME,
  artifact_path TEXT,
  artifact_size INTEGER,
  error_message TEXT,
  files         TEXT                             -- JSON：[{name,size}]（success 时写，I-04）
);
CREATE INDEX IF NOT EXISTS idx_run_job    ON backup_run(job_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_run_status ON backup_run(status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_run_running ON backup_run(job_id) WHERE status='running';  -- I-01 执行互斥（D-08）
