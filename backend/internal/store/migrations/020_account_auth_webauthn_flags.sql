-- 020_account_auth_webauthn_flags.sql
-- v0.6.3 注册 passkey 时未持久化 authenticator flags（BackupEligible 等），登录时
-- go-webauthn 比对 BE 标志不一致报 AUTH_015。补 flags 列存原始字节（CredentialFlags.MsgpByte）。
-- 旧行 flags=0 视为「未知」，登录时按断言自愈一次（service/adapter 处理）。

ALTER TABLE panel_user_webauthn ADD COLUMN flags INTEGER NOT NULL DEFAULT 0;
