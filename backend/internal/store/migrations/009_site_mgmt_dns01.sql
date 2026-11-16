-- 009_site_mgmt_dns01.sql — site-mgmt DNS-01 增量（api-site-mgmt.md v2.0 §5.3 增量
-- 迁移，⚠️A20/Gate 1 改判 P2）
-- 1) certificate.challenge CHECK 扩展 +dns_01 + 新列 dns_credential_id（仅 dns_01 行
--    非空，I-01/I-C01）——SQLite 不支持 ALTER 修改约束，经事务内表重建承载
--    （007 同法）：CREATE 新表（§5.3 DDL 形态，唯一事实源）→ INSERT SELECT →
--    DROP → RENAME。语义以 §5.3 DDL 为准，禁止自行增删列。
-- 2) dns_credential 表：DNS 服务商 API 凭证（凭据材料 AES-256-GCM 密文单列 +
--    独立密钥文件 dns-cred.key 0600，沿 database-mgmt CredentialVault 先例 D-20；
--    明文永不回显 I-D01）。

CREATE TABLE certificate_v2 (
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  primary_domain   TEXT    NOT NULL,
  domains          TEXT    NOT NULL,            -- SAN 域名列表 JSON 数组
  issuer           TEXT    NOT NULL DEFAULT '',
  source           TEXT    NOT NULL CHECK (source IN ('acme','imported')),
  status           TEXT    NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','valid','expiring_soon','expired','renew_failed')),
  challenge        TEXT    CHECK (challenge IN ('http_01','tls_alpn','dns_01')),  -- acme 必填（I-01 应用层校验）
  storage_path     TEXT,                                          -- acme 托管路径（⚠️A16）
  origin_path      TEXT,                                          -- imported 源站原路径（⚠️A16 不迁移）
  not_before       DATETIME,                                      -- acme 落库；imported 读时解析呈现（D-08）
  not_after        DATETIME,
  deploy_prev_refs TEXT,                                          -- 部署前引用快照 JSON map siteId→原值（D-11）
  last_error       TEXT,                                          -- renew_failed 失败原因（FR-24）
  acme_account_ref TEXT,                                          -- ACME 账户标识（D-09；密钥在文件不入库）
  dns_credential_id INTEGER,                                      -- v2.0：DNS 凭证引用（仅 dns_01 行非空，I-C01）
  created_at       DATETIME NOT NULL,
  updated_at       DATETIME NOT NULL
);

INSERT INTO certificate_v2
  (id, primary_domain, domains, issuer, source, status, challenge,
   storage_path, origin_path, not_before, not_after, deploy_prev_refs,
   last_error, acme_account_ref, dns_credential_id, created_at, updated_at)
SELECT id, primary_domain, domains, issuer, source, status, challenge,
       storage_path, origin_path, not_before, not_after, deploy_prev_refs,
       last_error, acme_account_ref, NULL, created_at, updated_at
  FROM certificate;

DROP TABLE certificate;
ALTER TABLE certificate_v2 RENAME TO certificate;

CREATE INDEX IF NOT EXISTS idx_cert_not_after ON certificate(not_after);
CREATE INDEX IF NOT EXISTS idx_cert_source_status ON certificate(source, status);

CREATE TABLE IF NOT EXISTS dns_credential (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  name           TEXT    NOT NULL,                 -- 显示名（≤64，不唯一，id 寻址）
  provider       TEXT    NOT NULL CHECK (provider IN ('cloudflare','aliyun','dnspod')),
  credential_enc BLOB    NOT NULL,                 -- 凭据材料密文（JSON→AES-256-GCM；D-20）
  created_at     DATETIME NOT NULL,
  updated_at     DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_dns_cred_provider ON dns_credential(provider);
