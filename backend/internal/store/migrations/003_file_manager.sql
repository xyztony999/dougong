-- 003_file_manager.sql — 文件管理器（file-manager）2 表 DDL
-- 来源：api-file-manager.md v1.0 §5.3（唯一事实源，逐字落库，禁止自行增删列）
-- 说明：trash_entry 为本模块唯一持久化业务对象（ADR-021，主数据类子形态）；
-- fs_path_scope 为受管根单行配置表（D-02：修改与审计同事务，提交后刷新进程内副本）。
-- 时间列 TEXT(RFC3339, UTC)，字典序即时间序（与 001/002 迁移同口径）。

CREATE TABLE IF NOT EXISTS trash_entry (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  original_path TEXT    NOT NULL,
  trash_name    TEXT    NOT NULL UNIQUE,
  entry_type    TEXT    NOT NULL CHECK (entry_type IN ('file','directory')),
  size          INTEGER NOT NULL DEFAULT 0,
  operator_id   INTEGER,
  operator_name TEXT    NOT NULL,
  status        TEXT    NOT NULL DEFAULT 'in_trash' CHECK (status IN ('in_trash','restored','purged')),
  deleted_at    DATETIME NOT NULL,
  resolved_at   DATETIME
);
CREATE INDEX IF NOT EXISTS idx_trash_status ON trash_entry(status, deleted_at DESC);

CREATE TABLE IF NOT EXISTS fs_path_scope (
  id            INTEGER PRIMARY KEY CHECK (id = 1),
  root_path     TEXT    NOT NULL DEFAULT '/',
  excluded_dirs TEXT    NOT NULL DEFAULT '/proc,/sys,/dev',
  updated_at    DATETIME NOT NULL
);
