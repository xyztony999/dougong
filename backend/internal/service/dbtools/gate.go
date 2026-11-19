package dbtools

import (
	"context"
	"log/slog"

	authsvc "github.com/dougong/dougong/internal/service/auth"
)

// SignonVerify signon/verify 内部端点业务入口（api 层防护链后调用；
// 四重校验+凭据 loopback 注入响应，D-09/D-11）。
func (s *Service) SignonVerify(ctx context.Context, token, forwardedSession, instanceCookie string) (SignonVerifyData, error) {
	return s.signon.Verify(ctx, token, forwardedSession, instanceCookie)
}

// GateVerify nginx auth_request 子请求会话 gating（D-11/4.4.1）：消费
// account-auth 只读会话校验 OHS（VerifyWs 语义——I-11 判定 revoked/expired，
// 只读不续期，避免工具静态资源逐请求滑动续期写放大）→ 角色判定 role==admin
// （服务端权威，不信任客户端声明）。纯读校验无状态变更（CSRF 无攻击面）；
// 逐请求不落业务审计（⚠️A11 防写放大）；拒绝事件 slog.Debug 安全日志。
// 返回 true=放行（HTTP 200）；false=拒绝（HTTP 401，裸状态无包络）。
func (s *Service) GateVerify(ctx context.Context, rawSession string) bool {
	if rawSession == "" {
		slog.DebugContext(ctx, "dbtools: gate rejected (no session)")
		return false
	}
	_, role, err := s.session.VerifyWs(ctx, rawSession)
	if err != nil {
		slog.DebugContext(ctx, "dbtools: gate rejected (session invalid)", "err", err)
		return false
	}
	if role != authsvc.RoleAdmin {
		slog.DebugContext(ctx, "dbtools: gate rejected (non-admin)", "role", role)
		return false
	}
	return true
}
