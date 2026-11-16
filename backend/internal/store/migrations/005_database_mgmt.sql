-- database-mgmt（数据库管理）模块 DDL（api-database-mgmt.md §5.3 逐字落库）。
-- db_instance：合一实例档案表（D-01，engine 列区分 mysql/mariadb/redis；凭据
-- 密文内嵌值对象列组 D-02；I-07 由服务端校验承载——SQLite CHECK 无法跨列
-- 表达互斥空语义）。
CREATE TABLE IF NOT EXISTS db_instance (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  identity_key    TEXT    NOT NULL UNIQUE,
  engine          TEXT    NOT NULL CHECK (engine IN ('mysql','mariadb','redis')),
  conn_mode       TEXT    NOT NULL CHECK (conn_mode IN ('socket','tcp')),
  socket_path     TEXT,
  host            TEXT,
  port            INTEGER CHECK (port IS NULL OR (port BETWEEN 1 AND 65535)),
  version         TEXT,
  conn_state      TEXT    NOT NULL DEFAULT 'unreachable' CHECK (conn_state IN ('reachable','unreachable')),
  cred_username   TEXT,
  cred_cipher     TEXT,
  cred_updated_at DATETIME,
  discovered_at   DATETIME NOT NULL,
  updated_at      DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_dbinst_engine ON db_instance(engine);
