package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
)

// EntryPathSource 是安全入口状态契约（EntryPathGuard 消费，
// panelsvc.Service 实现：每请求读取 panel_config 当前值——修改即时生效，
// 下一请求即用新值，D-P03）。
type EntryPathSource interface {
	EntryPathState(ctx context.Context) (enabled bool, entryPath string, err error)
}

// EntryPathHandler 是全局安全入口网关（panel-core D-P03，api §5.1 被依赖
// 契约）：net/http 包装器形态，包裹在整个 gin engine 之外——gin 的路由
// 解析先于其任何中间件执行，前缀改写必须发生在 gin 之前，故不能以 gin
// middleware 实现；语义上仍是「全局第一环」，位于 account-auth 中间件链
// （InitGuard 等）之前，作用于全部路由（含 /auth/* 与未认证访问）。
//
// 规则：开启后实际访问路径 = /{entryPath} + 接口路径——携带正确前缀的
// 请求剥前缀改写后交给 gin（下游按规范路径路由，接口文档一律省略前缀）；
// 未携带/错携带一律返回标准 404（无业务错误码包裹——不泄露「这是一个
// 面板」的存在性，防探测 PRD N2）；开关关闭直通；读态失败 fail-closed
// 标准 404（A-P7 裁定，细节进结构化日志）。
//
// v1.3（api-db-tools D-21）：内部端点不受入口路径限制——未携前缀但路径
// 直接命中豁免前缀（/dbtool/gate/、/dbtool/signon/）者前置直通（防御性
// 兜底：内部调用方 URL 含 entryPath 烘焙，入口路径变更窗口/烘焙时效内
// 回调仍可达）；携入口前缀者沿既有剥前缀语义放行；非内部路径未携/错携
// 前缀仍标准 404（防探测语义不变）。
func EntryPathHandler(src EntryPathSource, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isInternalExemptPath(r.URL.Path) {
			next.ServeHTTP(w, r) // 内部端点豁免（D-21：鉴权防护链不放宽）
			return
		}
		enabled, entryPath, err := src.EntryPathState(r.Context())
		if err != nil {
			slog.ErrorContext(r.Context(), "entry path guard: read state",
				"err", err, "path", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if !enabled || entryPath == "" {
			next.ServeHTTP(w, r) // 开关关闭直通（R-10 可选开关）
			return
		}
		prefix := "/" + entryPath
		switch p := r.URL.Path; {
		case p == prefix:
			r.URL.Path = "/" // 入口根（SPA 挂载点）
		case strings.HasPrefix(p, prefix+"/"):
			r.URL.Path = p[len(prefix):] // 剥前缀改写，gin 按规范路径路由
		default:
			// 未携带/错携带 → 标准 404（D-P03：无业务码包裹）。
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
