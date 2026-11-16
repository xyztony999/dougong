-- 008 · env-mgmt（环境管理，v0.1.2 用户反馈第 4 项 /pipeline 增量轮）
-- env_install_task：四态异步变更任务台账（api-env-mgmt.md §5.3 DDL 逐字；
-- ⚠️A3 状态机 pending→running→succeeded/failed，同环境在途互斥 I-01 经
-- env_type+status 组合索引承载查询、SQLite 单连接写串行保证创建原子）。
CREATE TABLE IF NOT EXISTS env_install_task (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  env_type     TEXT    NOT NULL CHECK (env_type IN ('nginx','apache','docker','mysql','redis','php_fpm')),
  action       TEXT    NOT NULL CHECK (action IN ('install','upgrade','uninstall')),
  status       TEXT    NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','succeeded','failed')),
  output_tail  TEXT    NOT NULL DEFAULT '',           -- 命令输出尾部（D-08，≤200 行定格）
  error        TEXT,                                   -- 失败原因（failed；timeout/面板重启中断等）
  requested_by TEXT    NOT NULL,                       -- 发起者用户名（admin）
  created_at   DATETIME NOT NULL,
  updated_at   DATETIME NOT NULL,
  started_at   DATETIME,
  finished_at  DATETIME
);
CREATE INDEX IF NOT EXISTS idx_env_task_env_status ON env_install_task(env_type, status);
CREATE INDEX IF NOT EXISTS idx_env_task_created ON env_install_task(created_at);
