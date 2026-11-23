package dockermgmt

import (
	"context"
	"strconv"
	"strings"

	authsvc "github.com/dougong/dougong/internal/service/auth"
	sitesvc "github.com/dougong/dougong/internal/service/sitemgmt"
)

// siteLinkAdapter 是 site-mgmt 建站能力的适配（SiteLinker 实现；main 装配注入
// ——消费 api-docker-mgmt v1.2 应用建站，与 app-mgmt 建站联动走同一通道）。经
// sitesvc.CreateSite 走安全写入管线 R-06 落盘（reverse_proxy 上游
// 127.0.0.1:{port}）；域名合法性由 site-mgmt 裁定，拒绝原因原样透传。
type siteLinkAdapter struct{ svc *sitesvc.Service }

// AdaptSiteLinker 构造建站适配器。
func AdaptSiteLinker(svc *sitesvc.Service) SiteLinker { return &siteLinkAdapter{svc: svc} }

// CreateReverseProxySite 创建反向代理站点：ServerNames=domains（空=通配 "_"，
// 由 site-mgmt 校验语义裁定）；Listen=80；Upstream=127.0.0.1:{port}；WebSocket
// 透传默认开（应用常见 SSE/WS 场景）。
func (a *siteLinkAdapter) CreateReverseProxySite(ctx context.Context, actor *authsvc.Identity,
	name string, port int, domains []string) (AppSiteRef, error) {
	names := make([]string, 0, len(domains))
	for _, d := range domains {
		if d = strings.TrimSpace(d); d != "" {
			names = append(names, d)
		}
	}
	if len(names) == 0 {
		names = []string{"_"}
	}
	ws := true
	data, err := a.svc.CreateSite(ctx, actor, sitesvc.CreateSiteRequest{
		Type:        sitesvc.SiteTypeReverseProx,
		ServerNames: names,
		Listen:      "80",
		Upstream:    "127.0.0.1:" + strconv.Itoa(port),
		Websocket:   &ws,
	})
	if err != nil {
		return AppSiteRef{}, err
	}
	return AppSiteRef{SiteID: data.SiteID, SourceFile: data.SourceFile}, nil
}

// HasUpstreamSite 站点清单联动查询（上游含 127.0.0.1:{port} 即视为已建站；
// 只读值快照呈现，本模块不持久化站点引用）。
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
	}
	return false, nil
}

// 编译期断言：适配器满足消费契约。
var _ SiteLinker = (*siteLinkAdapter)(nil)
