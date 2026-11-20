package appmgmt

import (
	"context"
	"strconv"
	"strings"

	authsvc "github.com/dougong/dougong/internal/service/auth"
	sitesvc "github.com/dougong/dougong/internal/service/sitemgmt"
)

// siteLinkAdapter 是 site-mgmt 建站 OHS 的适配器（SiteLinkGateway 实现；
// main 装配注入——消费 api-app-mgmt D-12，提供方登记归 site-mgmt 联动轮：
// api-site-mgmt.md §5.1 跨模块待同步项）。经 sitesvc.CreateSite 走安全写入
// 管线 R-06 落盘（reverse_proxy 上游 127.0.0.1:{port}）；失败透传 site-mgmt
// 错误语义（APP_011 reason）。
type siteLinkAdapter struct{ svc *sitesvc.Service }

// AdaptSiteLinker 构造建站联动适配器。
func AdaptSiteLinker(svc *sitesvc.Service) SiteLinkGateway { return &siteLinkAdapter{svc: svc} }

// CreateReverseProxySite 创建反向代理站点：ServerNames={domain}（空=通配 "_"，
// 由 site-mgmt 域名校验语义裁定，拒绝则 APP_011 引导站点管理）；Listen=80；
// Upstream=127.0.0.1:{port}；WebSocket 透传默认开（应用常见 SSE/WS 场景，
// A-SM 模板口径）。
func (a *siteLinkAdapter) CreateReverseProxySite(ctx context.Context, actor *authsvc.Identity,
	name string, port int, domain string) (SiteRef, error) {
	host := strings.TrimSpace(domain)
	if host == "" {
		host = "_"
	}
	ws := true
	data, err := a.svc.CreateSite(ctx, actor, sitesvc.CreateSiteRequest{
		Type:        sitesvc.SiteTypeReverseProx,
		ServerNames: []string{host},
		Listen:      "80",
		Upstream:    "127.0.0.1:" + strconv.Itoa(port),
		Websocket:   &ws,
	})
	if err != nil {
		return SiteRef{}, err
	}
	return SiteRef{SiteID: data.SiteID, SourceFile: data.SourceFile}, nil
}

// HasUpstreamSite 站点清单联动查询（上游含 127.0.0.1:{port} 即视为已建站；
// 只读值快照呈现，本模块不持久化站点引用——D-12）。清单读取失败=错误如实
// （调用方保持 null）。
func (a *siteLinkAdapter) HasUpstreamSite(ctx context.Context, port int) (bool, error) {
	list, err := a.svc.ListSites(ctx, "")
	if err != nil {
		return false, err
	}
	want := "127.0.0.1:" + strconv.Itoa(port)
	for _, it := range list.Items {
		if it.Proxy != nil && strings.Contains(it.Proxy.Upstream, want) {
			return true, nil
		}
		for _, b := range it.Blocks {
			_ = b // 聚合子块无反代参数投影（D-17 列表聚合仅呈现层），主段判定足够
		}
	}
	return false, nil
}

// 编译期断言：适配器满足消费契约。
var _ SiteLinkGateway = (*siteLinkAdapter)(nil)
