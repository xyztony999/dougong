package panel

import (
	"context"
	"database/sql"
	"time"
)

// CertRepo 实现 panel_cert 单行表访问（D-P04：自签/导入共用一行，替换式
// 更新；仓储契约 Get/Upsert/Delete，api-panel-core.md §4.1.5）。
type CertRepo struct{ x Executor }

// Upsert 替换式落库证书行（SQL 清单 #4；id 恒 1——自签生成/导入/ACME 申请
// 成功共用；last_error 恒置 NULL——acme 申请成功即清失败原因，D-P13）。
func (r *CertRepo) Upsert(ctx context.Context, c Cert, at time.Time) error {
	var issuer any
	if c.Issuer != nil {
		issuer = *c.Issuer
	}
	_, err := r.x.ExecContext(ctx,
		`INSERT OR REPLACE INTO panel_cert
		   (id, cert_pem, key_pem, source, issuer, not_before, not_after, last_error, updated_at)
		 VALUES (1, ?, ?, ?, ?, ?, ?, NULL, ?)`,
		c.CertPEM, c.KeyPEM, c.Source, issuer,
		timeArg(c.NotBefore), timeArg(c.NotAfter), formatTime(at))
	return wrapErr("cert upsert", err)
}

// Get 读取证书行（SQL 清单 #5：TLS 启动/回调读取 + detail/cert-status 摘要
// 与推算输入）。行不存在返回包装后的 sql.ErrNoRows（https_mode=off 且从未
// 生成/导入时常态）。
func (r *CertRepo) Get(ctx context.Context) (Cert, error) {
	var c Cert
	var issuer, lastError sql.NullString
	var notBefore, notAfter, updatedAt any
	err := r.x.QueryRowContext(ctx,
		`SELECT id, cert_pem, key_pem, source, issuer, not_before, not_after, last_error, updated_at
		   FROM panel_cert WHERE id = 1`).
		Scan(&c.ID, &c.CertPEM, &c.KeyPEM, &c.Source, &issuer,
			&notBefore, &notAfter, &lastError, &updatedAt)
	if err != nil {
		return Cert{}, wrapErr("cert get", err)
	}
	c.Issuer = scanStringPtr(issuer)
	c.LastError = scanStringPtr(lastError)
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

// UpdateAcmeResult ACME 结果回写（SQL 清单 #6/#7，4.1.3 与 retry-renewal 共用）：
// lastError=nil（成功）→ #6：回写 not_before/not_after/issuer 且 last_error=NULL
// （推算回 valid）；lastError 非 nil（失败）→ #7：仅落 last_error（renew_failed
// 呈现，D-P13）。
func (r *CertRepo) UpdateAcmeResult(ctx context.Context, notBefore, notAfter *time.Time,
	issuer *string, lastError *string, at time.Time) error {
	if lastError == nil {
		var iss any
		if issuer != nil {
			iss = *issuer
		}
		_, err := r.x.ExecContext(ctx,
			`UPDATE panel_cert SET not_before = ?, not_after = ?, issuer = ?, last_error = NULL, updated_at = ?
			   WHERE id = 1`,
			timeArg(notBefore), timeArg(notAfter), iss, formatTime(at))
		return wrapErr("cert update acme result", err)
	}
	_, err := r.x.ExecContext(ctx,
		`UPDATE panel_cert SET last_error = ?, updated_at = ? WHERE id = 1`,
		*lastError, formatTime(at))
	return wrapErr("cert update acme error", err)
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
