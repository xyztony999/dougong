-- site-mgmt（网站管理，核心卖点）：certificate 单表——本模块唯一持久化对象
-- （api-site-mgmt.md §5.3 逐字落库；site 为配置文件实时投影零持久化，D2 判定）。
-- deploy_prev_refs 承载部署前引用快照 JSON map（D-11）；引用关系本身实时推导不落冗余列。
CREATE TABLE IF NOT EXISTS certificate (
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  primary_domain   TEXT    NOT NULL,
  domains          TEXT    NOT NULL,            -- SAN 域名列表 JSON 数组
  issuer           TEXT    NOT NULL DEFAULT '',
  source           TEXT    NOT NULL CHECK (source IN ('acme','imported')),
  status           TEXT    NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','valid','expiring_soon','expired','renew_failed')),
  challenge        TEXT    CHECK (challenge IN ('http_01','tls_alpn')),          -- acme 来源必填（I-01，应用层校验）
  storage_path     TEXT,                                          -- acme 托管路径（⚠️A16）
  origin_path      TEXT,                                          -- imported 源站原路径（⚠️A16 不迁移）
  not_before       DATETIME,                                      -- acme 落库；imported 读时解析呈现（D-08）
  not_after        DATETIME,
  deploy_prev_refs TEXT,                                          -- 部署前引用快照 JSON map siteId→原 ssl_cert 值（D-11）
  last_error       TEXT,                                          -- renew_failed 失败原因（FR-24）
  acme_account_ref TEXT,                                          -- ACME 账户标识（D-09；密钥在文件不入库）
  created_at       DATETIME NOT NULL,
  updated_at       DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_cert_not_after ON certificate(not_after);
CREATE INDEX IF NOT EXISTS idx_cert_source_status ON certificate(source, status);
