-- 019_account_auth_login_security.sql
-- 登录安全适配（v0.7.0，d0-clarification-account-auth-login-security-2026-11-27）：
-- passkey(WebAuthn) 凭据 + 一次性恢复码 + 新设备识别。私钥永不落库；恢复码哈希存储。

CREATE TABLE IF NOT EXISTS panel_user_webauthn (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id         INTEGER NOT NULL REFERENCES panel_user(id) ON DELETE CASCADE,
  credential_id   BLOB    NOT NULL UNIQUE,
  public_key      BLOB    NOT NULL,
  attestation_type TEXT   NOT NULL DEFAULT '',
  aaguid          BLOB,
  sign_count      INTEGER NOT NULL DEFAULT 0,
  transports      TEXT    NOT NULL DEFAULT '[]',
  name            TEXT    NOT NULL DEFAULT '',
  created_at      TEXT    NOT NULL,
  last_used_at    TEXT
);
CREATE INDEX IF NOT EXISTS idx_webauthn_user ON panel_user_webauthn(user_id);

CREATE TABLE IF NOT EXISTS panel_user_recovery_codes (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES panel_user(id) ON DELETE CASCADE,
  code_hash  TEXT    NOT NULL,
  used_at    TEXT,
  created_at TEXT    NOT NULL,
  UNIQUE(user_id, code_hash)
);
CREATE INDEX IF NOT EXISTS idx_recovery_user ON panel_user_recovery_codes(user_id);

CREATE TABLE IF NOT EXISTS panel_user_devices (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES panel_user(id) ON DELETE CASCADE,
  device_hash  TEXT    NOT NULL,
  user_agent   TEXT    NOT NULL DEFAULT '',
  first_seen_at TEXT   NOT NULL,
  last_seen_at TEXT    NOT NULL,
  UNIQUE(user_id, device_hash)
);
