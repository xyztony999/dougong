-- 001_account_auth.sql — 账户与安全基座（account-auth）5 表 DDL
-- 来源：api-account-auth.md v1.1 §5.3（唯一事实源，逐字落库，禁止自行增删列）
-- 说明：时间列一律 TEXT(RFC3339, UTC)，字典序即时间序；布尔为 INTEGER 0/1。

CREATE TABLE IF NOT EXISTS panel_user (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  username      TEXT    NOT NULL UNIQUE COLLATE NOCASE,
  password_hash TEXT    NOT NULL,
  role          TEXT    NOT NULL CHECK (role IN ('admin','readonly')),
  status        TEXT    NOT NULL DEFAULT 'active' CHECK (status IN ('active','locked')),
  fail_count    INTEGER NOT NULL DEFAULT 0,
  locked_until  DATETIME,
  created_at    DATETIME NOT NULL,
  updated_at    DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS panel_user_totp (
  user_id      INTEGER PRIMARY KEY REFERENCES panel_user(id) ON DELETE CASCADE,
  secret       TEXT    NOT NULL,
  enabled      INTEGER NOT NULL DEFAULT 0,
  confirmed_at DATETIME
);

CREATE TABLE IF NOT EXISTS panel_session (
  token_hash      TEXT    PRIMARY KEY,
  user_id         INTEGER NOT NULL REFERENCES panel_user(id) ON DELETE CASCADE,
  csrf_token      TEXT    NOT NULL,
  created_at      DATETIME NOT NULL,
  last_active_at  DATETIME NOT NULL,
  expires_at      DATETIME NOT NULL,
  idle_expires_at DATETIME NOT NULL,
  revoked_at      DATETIME,
  end_reason      TEXT CHECK (end_reason IN ('self_logout','forced_logout','cascade')),
  source_ip       TEXT,
  user_agent      TEXT
);
CREATE INDEX IF NOT EXISTS idx_session_user ON panel_session(user_id);
CREATE INDEX IF NOT EXISTS idx_session_active ON panel_session(idle_expires_at);

CREATE TABLE IF NOT EXISTS audit_log (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  actor_id    INTEGER,
  actor_name  TEXT    NOT NULL,
  category    TEXT    NOT NULL CHECK (category IN ('change','security')),
  action      TEXT    NOT NULL,
  object_type TEXT    NOT NULL,
  object_id   TEXT,
  detail      TEXT,
  result      TEXT    NOT NULL CHECK (result IN ('success','failure')),
  source      TEXT    NOT NULL CHECK (source IN ('web','cli','system')),
  created_at  DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_log(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_actor ON audit_log(actor_id);
CREATE INDEX IF NOT EXISTS idx_audit_action ON audit_log(action);

CREATE TABLE IF NOT EXISTS init_state (
  id             INTEGER PRIMARY KEY CHECK (id = 1),
  initialized    INTEGER NOT NULL DEFAULT 0,
  method         TEXT CHECK (method IN ('web','cli')),
  initialized_at DATETIME
);
