package sitemgmt

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrCertNotFound 证书不存在（CERT_001 语义；sql.ErrNoRows 的模块化别名）。
var ErrCertNotFound = errors.New("store/sitemgmt: certificate not found")

// CertRepo 实现 certificate 表访问（api §4.1.5 仓储契约：Create/GetByID/List/
// UpdateStatus/UpdateRenewalMeta/SetDeployPrevRef/ClearDeployPrevRef/Delete；
// §5.2 SQL 清单 #1~#6 全参数化——标识符全部静态字面量，G-02）。
type CertRepo struct{ x Executor }

// NewCertRepo 构造绑定给定执行器的证书仓储（连接池或事务视图）。
func NewCertRepo(x Executor) *CertRepo { return &CertRepo{x: x} }

const certCols = `id, primary_domain, domains, issuer, source, status, challenge,
	storage_path, origin_path, not_before, not_after, deploy_prev_refs, last_error,
	acme_account_ref, dns_credential_id, created_at, updated_at`

// Create 插入一行证书（SQL #1；pending 签发落库/导入纳管共用），返回自增 id。
func (r *CertRepo) Create(ctx context.Context, c Certificate) (int64, error) {
	domains, err := marshalDomains(c.Domains)
	if err != nil {
		return 0, err
	}
	now := formatTime(time.Now().UTC())
	refs, err := marshalRefs(c.DeployPrevRefs)
	if err != nil {
		return 0, err
	}
	res, err := r.x.ExecContext(ctx,
		`INSERT INTO certificate (primary_domain, domains, issuer, source, status, challenge,
			storage_path, origin_path, not_before, not_after, deploy_prev_refs, last_error,
			acme_account_ref, dns_credential_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.PrimaryDomain, domains, c.Issuer, c.Source, c.Status, strPtrArg(c.Challenge),
		strPtrArg(c.StoragePath), strPtrArg(c.OriginPath), timePtrArg(c.NotBefore), timePtrArg(c.NotAfter),
		refs, strPtrArg(c.LastError), strPtrArg(c.AcmeAccountRef), int64PtrArg(c.DnsCredentialID), now, now)
	if err != nil {
		return 0, wrapErr("cert create", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store/sitemgmt: cert create last id: %w", err)
	}
	return id, nil
}

// GetByID 按 id 取单行（SQL #2）；不存在返回 ErrCertNotFound。
func (r *CertRepo) GetByID(ctx context.Context, id int64) (Certificate, error) {
	row := r.x.QueryRowContext(ctx,
		`SELECT `+certCols+` FROM certificate WHERE id = ?`, id)
	c, err := scanCert(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Certificate{}, ErrCertNotFound
	}
	return c, wrapErr("cert get by id", err)
}

// List 全表清单（SQL #3 临期在前：not_after 升序；NULL 有效期行——pending/
// 读时不可刷新的 imported——排尾，id 升序稳定）。
func (r *CertRepo) List(ctx context.Context) ([]Certificate, error) {
	rows, err := r.x.QueryContext(ctx,
		`SELECT `+certCols+` FROM certificate
		 ORDER BY (not_after IS NULL), not_after ASC, id ASC`)
	if err != nil {
		return nil, wrapErr("cert list", err)
	}
	defer rows.Close()
	var out []Certificate
	for rows.Next() {
		c, err := scanCert(rows)
		if err != nil {
			return nil, fmt.Errorf("store/sitemgmt: cert list scan: %w", err)
		}
		out = append(out, c)
	}
	return out, wrapErr("cert list rows", rows.Err())
}

// ListActive 返回 active 态行（pending/valid/expiring_soon——I-03 同域名防重
// 与扫描匹配的数据面；集合等值比较在 service 层完成）。
func (r *CertRepo) ListActive(ctx context.Context) ([]Certificate, error) {
	rows, err := r.x.QueryContext(ctx,
		`SELECT `+certCols+` FROM certificate
		 WHERE status IN ('pending','valid','expiring_soon') ORDER BY id ASC`)
	if err != nil {
		return nil, wrapErr("cert list active", err)
	}
	defer rows.Close()
	var out []Certificate
	for rows.Next() {
		c, err := scanCert(rows)
		if err != nil {
			return nil, fmt.Errorf("store/sitemgmt: cert list active scan: %w", err)
		}
		out = append(out, c)
	}
	return out, wrapErr("cert list active rows", rows.Err())
}

// UpdateStatus 更新状态与失败原因（renew_failed 落库态迁移；SQL #4 简化形）。
func (r *CertRepo) UpdateStatus(ctx context.Context, id int64, status string, lastError *string) error {
	_, err := r.x.ExecContext(ctx,
		`UPDATE certificate SET status = ?, last_error = ?, updated_at = ? WHERE id = ?`,
		status, strPtrArg(lastError), formatTime(time.Now().UTC()), id)
	return wrapErr("cert update status", err)
}

// UpdateRenewalMeta 签发/续期终态落库（SQL #4 全形：status/last_error/有效期/
// issuer 一并更新；lastError=nil 表示清空失败原因）。
func (r *CertRepo) UpdateRenewalMeta(ctx context.Context, id int64, status string,
	lastError *string, notBefore, notAfter *time.Time, issuer string) error {
	_, err := r.x.ExecContext(ctx,
		`UPDATE certificate SET status = ?, last_error = ?, not_before = ?, not_after = ?,
		 issuer = ?, updated_at = ? WHERE id = ?`,
		status, strPtrArg(lastError), timePtrArg(notBefore), timePtrArg(notAfter),
		issuer, formatTime(time.Now().UTC()), id)
	return wrapErr("cert update renewal meta", err)
}

// SetDeployPrevRef 写入/覆盖单站点部署前引用快照（SQL #5；read-modify-write
// 两条语句——单连接写串行与调用方事务视图保证原子，D-11/I-05）。
func (r *CertRepo) SetDeployPrevRef(ctx context.Context, id int64, siteID string, ref DeployRef) error {
	c, err := r.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if c.DeployPrevRefs == nil {
		c.DeployPrevRefs = map[string]DeployRef{}
	}
	c.DeployPrevRefs[siteID] = ref
	refs, err := marshalRefs(c.DeployPrevRefs)
	if err != nil {
		return err
	}
	_, err = r.x.ExecContext(ctx,
		`UPDATE certificate SET deploy_prev_refs = ?, updated_at = ? WHERE id = ?`,
		refs, formatTime(time.Now().UTC()), id)
	return wrapErr("cert set deploy ref", err)
}

// ClearDeployPrevRef 清除单站点键（SQL #5 对偶；键不存在时为 no-op，幂等）。
func (r *CertRepo) ClearDeployPrevRef(ctx context.Context, id int64, siteID string) error {
	c, err := r.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if _, ok := c.DeployPrevRefs[siteID]; !ok {
		return nil
	}
	delete(c.DeployPrevRefs, siteID)
	refs, err := marshalRefs(c.DeployPrevRefs)
	if err != nil {
		return err
	}
	_, err = r.x.ExecContext(ctx,
		`UPDATE certificate SET deploy_prev_refs = ?, updated_at = ? WHERE id = ?`,
		refs, formatTime(time.Now().UTC()), id)
	return wrapErr("cert clear deploy ref", err)
}

// Delete 删除行（SQL #6；I-02 引用守卫在 service 层完成后调用）。
func (r *CertRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.x.ExecContext(ctx, `DELETE FROM certificate WHERE id = ?`, id)
	return wrapErr("cert delete", err)
}

// SetAccountRef 写入 ACME 账户标识（签发成功后随终态落库，D-09；密钥本体
// 在文件不入库——本列仅注册资源 URI 引用）。
func (r *CertRepo) SetAccountRef(ctx context.Context, id int64, ref string) error {
	_, err := r.x.ExecContext(ctx,
		`UPDATE certificate SET acme_account_ref = ?, updated_at = ? WHERE id = ?`,
		ref, formatTime(time.Now().UTC()), id)
	return wrapErr("cert set account ref", err)
}

// scanCert 统一行扫描（*sql.Row 与 *sql.Rows 共同满足 Scan 接口）。
func scanCert(row interface{ Scan(...any) error }) (Certificate, error) {
	var c Certificate
	var domains, refs sql.NullString
	var challenge, storagePath, originPath, lastError, accountRef sql.NullString
	var dnsCredID sql.NullInt64
	var notBefore, notAfter, createdAt, updatedAt any
	if err := row.Scan(&c.ID, &c.PrimaryDomain, &domains, &c.Issuer, &c.Source, &c.Status,
		&challenge, &storagePath, &originPath, &notBefore, &notAfter, &refs, &lastError,
		&accountRef, &dnsCredID, &createdAt, &updatedAt); err != nil {
		return Certificate{}, err
	}
	if dnsCredID.Valid {
		v := dnsCredID.Int64
		c.DnsCredentialID = &v
	}
	if domains.Valid {
		if err := json.Unmarshal([]byte(domains.String), &c.Domains); err != nil {
			return Certificate{}, fmt.Errorf("store/sitemgmt: parse domains %q: %w", domains.String, err)
		}
	}
	if refs.Valid && refs.String != "" {
		if err := json.Unmarshal([]byte(refs.String), &c.DeployPrevRefs); err != nil {
			return Certificate{}, fmt.Errorf("store/sitemgmt: parse deploy refs %q: %w", refs.String, err)
		}
	}
	c.Challenge = nullStr(challenge)
	c.StoragePath = nullStr(storagePath)
	c.OriginPath = nullStr(originPath)
	c.LastError = nullStr(lastError)
	c.AcmeAccountRef = nullStr(accountRef)
	var err error
	if c.NotBefore, err = scanTimePtr(notBefore); err != nil {
		return Certificate{}, fmt.Errorf("scan not_before: %w", err)
	}
	if c.NotAfter, err = scanTimePtr(notAfter); err != nil {
		return Certificate{}, fmt.Errorf("scan not_after: %w", err)
	}
	if c.CreatedAt, err = scanTime(createdAt); err != nil {
		return Certificate{}, fmt.Errorf("scan created_at: %w", err)
	}
	if c.UpdatedAt, err = scanTime(updatedAt); err != nil {
		return Certificate{}, fmt.Errorf("scan updated_at: %w", err)
	}
	return c, nil
}

func nullStr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	v := ns.String
	return &v
}

func strPtrArg(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func timePtrArg(t *time.Time) any { return formatTimePtr(t) }

func int64PtrArg(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func marshalDomains(domains []string) (any, error) {
	if domains == nil {
		domains = []string{}
	}
	b, err := json.Marshal(domains)
	if err != nil {
		return nil, fmt.Errorf("store/sitemgmt: marshal domains: %w", err)
	}
	return string(b), nil
}

func marshalRefs(refs map[string]DeployRef) (any, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(refs)
	if err != nil {
		return nil, fmt.Errorf("store/sitemgmt: marshal deploy refs: %w", err)
	}
	return string(b), nil
}
