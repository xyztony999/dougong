-- 013 · db-tools（数据库工具集成，api-db-tools.md §5.3 DDL 逐字；D0 Q1/Q2
-- 拍板 2026-11-19）。db_tool 部署档案+凭据密文（一工具一档案 UNIQUE tool_type
-- I-01；cred_cipher AES-256-GCM I-02 零回显）；tool_deploy_task 四态任务台账
-- （D-04：同工具在途互斥 I-06 经 tool_type+status 组合索引承载查询、SQLite
-- 单连接写串行保证创建原子；终态与档案登记同事务 I-C01）。
CREATE TABLE IF NOT EXISTS db_tool (
  id                INTEGER PRIMARY KEY AUTOINCREMENT,
  tool_type         TEXT    NOT NULL UNIQUE CHECK (tool_type IN ('phpmyadmin','mongo_express')),
  installed_version TEXT    NOT NULL,
  deploy_path       TEXT    NOT NULL,
  site_ref          TEXT    NOT NULL,                  -- 注入落点标识（来源文件+server 块定位，D-03）
  unit_name         TEXT,                              -- mongo_express=dougong-mongo-express.service；phpmyadmin=NULL
  instance_id       INTEGER,                           -- mongo_express 目标实例引用（db_instance.id）；phpmyadmin=NULL
  cred_cipher       TEXT    NOT NULL,                  -- AES-256-GCM 密文（I-02 零回显；dbtool-cred.key 0600）
  created_at        DATETIME NOT NULL,
  updated_at        DATETIME NOT NULL
);
CREATE TABLE IF NOT EXISTS tool_deploy_task (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  tool_type    TEXT    NOT NULL CHECK (tool_type IN ('phpmyadmin','mongo_express')),
  action       TEXT    NOT NULL CHECK (action IN ('install','upgrade','uninstall')),
  status       TEXT    NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','succeeded','failed')),
  instance_id  INTEGER,                               -- 安装时目标实例（溯源）
  output_tail  TEXT    NOT NULL DEFAULT '',           -- ≤200 行定格
  error        TEXT,
  requested_by TEXT    NOT NULL,
  created_at   DATETIME NOT NULL,
  updated_at   DATETIME NOT NULL,
  started_at   DATETIME,
  finished_at  DATETIME
);
CREATE INDEX IF NOT EXISTS idx_dbtool_task_type_status ON tool_deploy_task(tool_type, status);
CREATE INDEX IF NOT EXISTS idx_dbtool_task_created ON tool_deploy_task(created_at);