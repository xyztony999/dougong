package panel

import (
	"time"

	storepanel "github.com/dougong/dougong/internal/store/panel"
)

// CalcCertStatus 是 PanelCertStatusCalculator 领域服务（D-P13 读时惰性推算，
// api §4.1.1/4.1.4）：cert/status 与 detail/前置校验共用；无独立状态列。
//
// 推算口径：source≠acme → ""（自签/导入无续期语义，呈现 null）；acme 且
// last_error 非空 → renew_failed（落库态）；否则由 not_after 推算——已过期
// → expired、剩余 ≤CERT_EXPIRING_DAYS 天 → expiring_soon、否则 valid；
// not_after 缺失（异常行）按 expired 如实呈现（不虚构有效）。
func CalcCertStatus(c storepanel.Cert, now time.Time) string {
	if c.Source != storepanel.CertSourceAcme {
		return ""
	}
	if c.LastError != nil && *c.LastError != "" {
		return storepanel.CertStatusRenewFailed
	}
	if c.NotAfter == nil {
		return storepanel.CertStatusExpired
	}
	notAfter := (*c.NotAfter).UTC()
	if !now.Before(notAfter) {
		return storepanel.CertStatusExpired
	}
	if notAfter.Sub(now) <= CertExpiringDays*24*time.Hour {
		return storepanel.CertStatusExpiringSoon
	}
	return storepanel.CertStatusValid
}

// acmeCertUsable 报告推算状态是否可承载 https_mode=acme（I-P04：未过期；
// renew_failed 但 not_after 未过的存量证书材料仍可服务 TLS——D-P14「手动
// 切至其他态后 acme 证书行保留（可再切回）」，仅 expired 拒绝）。
func acmeCertUsable(status string) bool {
	return status != storepanel.CertStatusExpired
}
