-- 007_panel_core_acme.sql — panel-core 增量（api-panel-core.md v2.0 §5.3 增量迁移，ADR-028/029）
-- 1) panel_config 新增 bind_domain（可空无约束列，直接加列，ADR-028 HostGuard 强校验输入）；
-- 2) panel_config.https_mode CHECK 扩展 +acme 与 panel_cert.source CHECK 扩展 +acme、
--    panel_cert 新增 last_error 列——SQLite 不支持 ALTER 修改约束，经事务内表重建承载：
--    CREATE 新表（v2.0 §5.3 DDL 形态，唯一事实源）→ INSERT SELECT 迁移既有单行 →
--    DROP 旧表 → RENAME。语义以 §5.3 DDL 为准，禁止自行增删列。

ALTER TABLE panel_config ADD COLUMN bind_domain TEXT;

CREATE TABLE panel_config_v2 (
  id                 INTEGER PRIMARY KEY CHECK (id = 1),
  listen_scope       TEXT    NOT NULL DEFAULT 'all' CHECK (listen_scope IN ('all','localhost','custom')),
  listen_address     TEXT,
  listen_port        INTEGER NOT NULL CHECK (listen_port BETWEEN 1024 AND 65535),
  entry_path_enabled INTEGER NOT NULL DEFAULT 0,
  entry_path         TEXT,
  bind_domain        TEXT,
  https_mode         TEXT    NOT NULL DEFAULT 'off' CHECK (https_mode IN ('off','self_signed','imported','acme')),
  update_check       INTEGER NOT NULL DEFAULT 0,
  lang               TEXT    NOT NULL DEFAULT 'zh-CN',
  updated_at         DATETIME NOT NULL
);

INSERT INTO panel_config_v2
  (id, listen_scope, listen_address, listen_port, entry_path_enabled,
   entry_path, bind_domain, https_mode, update_check, lang, updated_at)
SELECT id, listen_scope, listen_address, listen_port, entry_path_enabled,
       entry_path, bind_domain, https_mode, update_check, lang, updated_at
  FROM panel_config;

DROP TABLE panel_config;
ALTER TABLE panel_config_v2 RENAME TO panel_config;

CREATE TABLE panel_cert_v2 (
  id         INTEGER PRIMARY KEY CHECK (id = 1),
  cert_pem   TEXT    NOT NULL,
  key_pem    TEXT    NOT NULL,
  source     TEXT    NOT NULL CHECK (source IN ('self_signed','imported','acme')),
  issuer     TEXT,
  not_before DATETIME,
  not_after  DATETIME,
  last_error TEXT,
  updated_at DATETIME NOT NULL
);

INSERT INTO panel_cert_v2
  (id, cert_pem, key_pem, source, issuer, not_before, not_after, last_error, updated_at)
SELECT id, cert_pem, key_pem, source, issuer, not_before, not_after, NULL, updated_at
  FROM panel_cert;

DROP TABLE panel_cert;
ALTER TABLE panel_cert_v2 RENAME TO panel_cert;
