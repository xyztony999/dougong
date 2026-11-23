-- 018 · app-mgmt（ADR-033 宝塔式重组：app 扩 runtime 列〔python|node〕，
-- start_mode CHECK 扩 node_file/npm_script；api-app-mgmt v1.2 / prd v1.2）。
-- SQLite CHECK 约束不可 ALTER，迁移内表重建（单事务原子，沿 010/014/016/017
-- 先例：建新表〔runtime 列 + 五值 CHECK〕→ 全量拷贝〔既有行 runtime 缺省
-- python〕→ 弃旧表 → 改名 → 重建唯一索引；AUTOINCREMENT 序列随显式 id 拷贝
-- 保持，既有档案零丢失）。container 为历史值保留于 CHECK（该形态已下线，
-- 服务层拒绝 APP_014；容器化部署归 docker-mgmt「应用」页签）。
CREATE TABLE app_rebuild (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  name         TEXT    NOT NULL UNIQUE,
  dir          TEXT    NOT NULL,
  entry        TEXT    NOT NULL,
  runtime      TEXT    NOT NULL DEFAULT 'python' CHECK (runtime IN ('python','node')),
  start_mode   TEXT    NOT NULL CHECK (start_mode IN ('preset_fastapi','custom','container','node_file','npm_script')),
  start_spec   TEXT    NOT NULL DEFAULT '{}',
  listen_port  INTEGER NOT NULL CHECK (listen_port BETWEEN 1024 AND 65535),
  unit_name    TEXT    NOT NULL,
  created_by   TEXT    NOT NULL,
  created_at   DATETIME NOT NULL,
  updated_at   DATETIME NOT NULL
);
INSERT INTO app_rebuild (id, name, dir, entry, runtime, start_mode, start_spec, listen_port,
  unit_name, created_by, created_at, updated_at)
  SELECT id, name, dir, entry, 'python', start_mode, start_spec, listen_port,
         unit_name, created_by, created_at, updated_at FROM app;
DROP TABLE app;
ALTER TABLE app_rebuild RENAME TO app;
CREATE UNIQUE INDEX IF NOT EXISTS idx_app_unit ON app(unit_name);
