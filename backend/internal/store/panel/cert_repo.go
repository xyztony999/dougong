package panel

import (
	"context"
	"database/sql"
	"time"
)

// CertRepo 实现 panel_cert 单行表访问（D-P04：自签/导入共用一行，替换式
// 更新；仓储契约 Get/Upsert/Delete，api-panel-core.md §4.1.5）。
type CertRepo struct{ x Executor }

// Upsert 替换式落库证书行（SQL 清单 #4；id 恒 1——自签生成与导入共用）。
func (r *CertRepo) Upsert(ctx context.Context, c Cert, at time.Time) error {
	var issuer any
	if c.Issuer != nil {
		issuer = *c.Issuer
	}
	_, err := r.x.ExecContext(ctx,
		`INSERT OR REPLACE INTO panel_cert
		   (id, cert_pem, key_pem, source, issuer, not_before, not_after, updated_at)
		 VALUES (1, ?, ?, ?, ?, ?, ?, ?)`,
		c.CertPEM, c.KeyPEM, c.Source, issuer,
		timeArg(c.NotBefore), timeArg(c.NotAfter), formatTime(at))
	return wrapErr("cert upsert", err)
}

// Get 读取证书行（SQL 清单 #5：TLS 启动/回调读取 + detail 摘要数据源）。
// 行不存在返回包装后的 sql.ErrNoRows（https_mode=off 且从未生成/导入时常态）。
func (r *CertRepo) Get(ctx context.Context) (Cert, error) {
	var c Cert
	var issuer sql.NullString
	var notBefore, notAfter, updatedAt any
	err := r.x.QueryRowContext(ctx,
		`SELECT id, cert_pem, key_pem, source, issuer, not_before, not_after, updated_at
		   FROM panel_cert WHERE id = 1`).
		Scan(&c.ID, &c.CertPEM, &c.KeyPEM, &c.Source, &issuer,
			&notBefore, &notAfter, &updatedAt)
	if err != nil {
		return Cert{}, wrapErr("cert get", err)
	}
	c.Issuer = scanStringPtr(issuer)
	var scanErr error
	if c.NotBefore, scanErr = scanTimePtr(notBefore); scanErr != nil {
		return Cert{}, wrapErr("cert scan not_before", scanErr)
	}
	if c.NotAfter, scanErr = scanTimePtr(notAfter); scanErr != nil {
		return Cert{}, wrapErr("cert scan not_after", scanErr)
	}
	updatedAtT, scanErr := scanTime(updatedAt)
	if scanErr != nil {
		return Cert{}, wrapErr("cert scan updated_at", scanErr)
	}
	c.UpdatedAt = updatedAtT
	return c, nil
}

// Delete 删除证书行（仓储契约补全；当前无接口消费——https_mode 切 off
// 保留证书行，api 4.1.4「to off 直接生效」不触碰 panel_cert）。
func (r *CertRepo) Delete(ctx context.Context) error {
	_, err := r.x.ExecContext(ctx, `DELETE FROM panel_cert WHERE id = 1`)
	return wrapErr("cert delete", err)
}

// timeArg 可空时间转 SQL 参数。
func timeArg(t *time.Time) any {
	if t == nil {
		return nil
	}
	return formatTime(*t)
}
