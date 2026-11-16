-- 002_panel_core.sql — 面板基座（panel-core）2 表 DDL
-- 来源：api-panel-core.md v1.0 §5.3（唯一事实源，逐字落库，禁止自行增删列）
-- 说明：两表均为单行表（CHECK(id=1)，D-P01/D-P04）；时间列 TEXT(RFC3339, UTC)，
-- 字典序即时间序；布尔为 INTEGER 0/1；panel_cert.key_pem 永不经接口下发（I-P06）。

CREATE TABLE IF NOT EXISTS panel_config (
  id                 INTEGER PRIMARY KEY CHECK (id = 1),
  listen_scope       TEXT    NOT NULL DEFAULT 'all' CHECK (listen_scope IN ('all','localhost','custom')),
  listen_address     TEXT,
  listen_port        INTEGER NOT NULL CHECK (listen_port BETWEEN 1024 AND 65535),
  entry_path_enabled INTEGER NOT NULL DEFAULT 0,
  entry_path         TEXT,
  https_mode         TEXT    NOT NULL DEFAULT 'off' CHECK (https_mode IN ('off','self_signed','imported')),
  update_check       INTEGER NOT NULL DEFAULT 0,
  lang               TEXT    NOT NULL DEFAULT 'zh-CN',
  updated_at         DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS panel_cert (
  id         INTEGER PRIMARY KEY CHECK (id = 1),
  cert_pem   TEXT    NOT NULL,
  key_pem    TEXT    NOT NULL,
  source     TEXT    NOT NULL CHECK (source IN ('self_signed','imported')),
  issuer     TEXT,
  not_before DATETIME,
  not_after  DATETIME,
  updated_at DATETIME NOT NULL
);
