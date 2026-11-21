-- 017 · app-mgmt（v1.1 容器化应用增量：app.start_mode CHECK 扩 +container，
-- prd-app-mgmt v1.1 FR-03 双形态/api-app-mgmt v1.1）。SQLite CHECK 约束不可
-- ALTER，迁移内表重建（单事务原子，沿 010/014/016 先例：建新表〔container 值
-- CHECK〕→ 全量拷贝 → 弃旧表 → 改名 → 重建唯一索引；AUTOINCREMENT 序列随
-- 显式 id 拷贝保持，既有档案零丢失）。
CREATE TABLE app_rebuild (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  name         TEXT    NOT NULL UNIQUE,
  dir          TEXT    NOT NULL,
  entry        TEXT    NOT NULL,
  start_mode   TEXT    NOT NULL CHECK (start_mode IN ('preset_fastapi','custom','container')),
  start_spec   TEXT    NOT NULL DEFAULT '{}',
  listen_port  INTEGER NOT NULL CHECK (listen_port BETWEEN 1024 AND 65535),
  unit_name    TEXT    NOT NULL,
  created_by   TEXT    NOT NULL,
  created_at   DATETIME NOT NULL,
  updated_at   DATETIME NOT NULL
);
INSERT INTO app_rebuild (id, name, dir, entry, start_mode, start_spec, listen_port,
  unit_name, created_by, created_at, updated_at)
  SELECT id, name, dir, entry, start_mode, start_spec, listen_port,
         unit_name, created_by, created_at, updated_at FROM app;
DROP TABLE app;
ALTER TABLE app_rebuild RENAME TO app;
CREATE UNIQUE INDEX IF NOT EXISTS idx_app_unit ON app(unit_name);
