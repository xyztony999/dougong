package sitemgmt

import (
	"context"
	"log/slog"
	"time"

	"github.com/dougong/dougong/internal/service/audit"
	storesitemgmt "github.com/dougong/dougong/internal/store/sitemgmt"
)

// RenewalScheduler（§4.1.3 自动续期任务，D-09/A-SM15）：每日后台调度对读时
// 推算为 expiring_soon 的 acme 来源证书逐张发起单次尝试；失败转 renew_failed
// 等待次日调度或手动 cert/retry-renewal（不自动无限重试）；单证书失败不影响
// 其余。goroutine 由 main 统一编排（ctx 取消退出，G-06）。

// RunRenewalScheduler 阻塞运行调度循环：启动 1 分钟后首轮 + 24h ticker
// （A-SM15；重启覆盖到期窗口）。
func (s *Service) RunRenewalScheduler(ctx context.Context) error {
	select {
	case <-time.After(time.Minute):
	case <-ctx.Done():
		return nil
	}
	for {
		s.runRenewalPass(ctx)
		select {
		case <-time.After(24 * time.Hour):
		case <-ctx.Done():
			return nil
		}
	}
}

// runRenewalPass 单轮调度：expiring_soon 的 acme 证书逐张续期（每张失败仅
// 记日志——不影响其余证书，4.1.3 失败处理）。
func (s *Service) runRenewalPass(ctx context.Context) {
	rows, err := s.store.Cert().List(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "site-mgmt: renewal pass: list certs failed", "err", err)
		return
	}
	now := s.clock()
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return
		}
		if row.Source != storesitemgmt.SourceAcme || row.Challenge == nil || row.StoragePath == nil {
			continue
		}
		if s.computeStatus(row, now) != storesitemgmt.StatusExpiringSoon {
			continue
		}
		if _, rerr := s.renewCert(ctx, nil, row, AuditActionCertRenewed, audit.SourceSystem); rerr != nil {
			// 单证书失败不影响其余（4.1.3）；状态已转 renew_failed + last_error。
			slog.WarnContext(ctx, "site-mgmt: auto renewal failed",
				"cert", row.ID, "domain", row.PrimaryDomain, "err", rerr)
			continue
		}
		slog.InfoContext(ctx, "site-mgmt: certificate auto renewed",
			"cert", row.ID, "domain", row.PrimaryDomain)
	}
}
