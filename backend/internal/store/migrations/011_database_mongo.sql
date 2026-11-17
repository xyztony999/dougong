-- 011 · database-mgmt（v0.1.4 用户反馈第 3 项：db_instance.engine CHECK 扩值
-- +mongodb）。SQLite CHECK 不可 ALTER，迁移内表重建（010 先例：建新表〔四值
-- CHECK〕→ 全量拷贝 → 弃旧表 → 改名 → 重建索引；单事务原子，AUTOINCREMENT
-- 序列随显式 id 拷贝保持，既有实例档案与凭据密文零丢失）。
CREATE TABLE db_instance_rebuild (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  identity_key    TEXT    NOT NULL UNIQUE,
  engine          TEXT    NOT NULL CHECK (engine IN ('mysql','mariadb','redis','mongodb')),
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
INSERT INTO db_instance_rebuild (id, identity_key, engine, conn_mode, socket_path, host,
  port, version, conn_state, cred_username, cred_cipher, cred_updated_at, discovered_at, updated_at)
  SELECT id, identity_key, engine, conn_mode, socket_path, host,
         port, version, conn_state, cred_username, cred_cipher, cred_updated_at, discovered_at, updated_at
  FROM db_instance;
DROP TABLE db_instance;
ALTER TABLE db_instance_rebuild RENAME TO db_instance;
CREATE INDEX IF NOT EXISTS idx_dbinst_engine ON db_instance(engine);
