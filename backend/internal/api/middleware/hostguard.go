package middleware

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"
)

// HostGuardSource 是域名绑定状态契约（HostGuard 消费，panelsvc.Service 实现：
// 每请求读取 panel_config.bind_domain 当前值——绑定/解绑即时生效，下一请求
// 即用新值，D-P16/ADR-028）。
type HostGuardSource interface {
	BindDomainState(ctx context.Context) (bindDomain string, err error)
}

// HostGuard 是域名绑定强校验网关（panel-core v2.0 D-P16，ADR-028；api §5.1
// 被依赖契约）：net/http 包装器形态，与 EntryPathHandler 同层、紧随其后
// （Entry → Host → 引擎中间件链），作用于全部路由（含 /auth/* 与未认证访问）。
//
// 规则（I-P09）：bind_domain 空=未绑定，直通放行全部 Host；非空时请求 Host
// 头去除可选 `:port` 后与 bind_domain 全等比较（大小写不敏感）方放行，
// 不匹配（含 IP 直连）一律返回标准 404（无业务错误码包裹——与 EntryPathGuard
// 同防探测语义 D-P03，两网关失败响应形态一致不泄露先后顺序）；读态失败
// fail-closed 标准 404（沿 A-P7 先例，细节进结构化日志）。
func HostGuard(src HostGuardSource, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bindDomain, err := src.BindDomainState(r.Context())
		if err != nil {
			slog.ErrorContext(r.Context(), "host guard: read state",
				"err", err, "path", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if bindDomain == "" {
			next.ServeHTTP(w, r) // 未绑定（I-P09 ⇔ 左侧）：放行全部 Host
			return
		}
		if !strings.EqualFold(stripPort(r.Host), bindDomain) {
			// 不匹配（含 IP 直连）→ 标准 404 无业务码（I-P09/D-P16）。
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// stripPort 去除 Host 头的可选 `:port`（IPv6 字面量形如 [::1]:8080；
// 无端口时原样返回——net.SplitHostPort 失败即视为不含端口）。
func stripPort(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}
