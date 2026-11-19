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

// internalExemptPrefixes 内部端点豁免路径前缀（api-db-tools v1.3 D-21/
// 4.4.5，INTERNAL_EXEMPT_PREFIXES 编译期常量最小化集合——仅 db-tools 内部
// 2 端点路径前缀，前缀匹配含子路径）。豁免的是 Host/入口路径校验而非鉴权：
// 内部端点 LoopbackGuard+InternalSecretGuard+会话/令牌防护链全量不变
// （非 loopback/无秘密头仍裸 404）；非内部路径强校验不变（fail-closed）。
// 豁免理由：内部调用方为 nginx auth_request 子请求/PHP-FPM SignonScript
// loopback 回调，Host 头由 proxy_pass 目标决定而非用户浏览器——Host 校验
// 对其为误伤非安全增益（绑域名后子请求 Host≠域名→gating/signon 失效）。
var internalExemptPrefixes = []string{"/dbtool/gate/", "/dbtool/signon/"}

// isInternalExemptPath 报告规范路径（EntryPath 剥前缀后）是否命中内部端点
// 豁免前缀（D-21：豁免判定在各包装器内部前置，命中即直通 next）。
func isInternalExemptPath(path string) bool {
	for _, p := range internalExemptPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
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
//
// v1.3（api-db-tools D-21）：内部端点路径前缀（/dbtool/gate/、/dbtool/signon/）
// 前置直通豁免——不做 Host 校验；非内部路径强校验不变。
func HostGuard(src HostGuardSource, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isInternalExemptPath(r.URL.Path) {
			next.ServeHTTP(w, r) // 内部端点豁免（D-21：鉴权防护链不放宽）
			return
		}
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
