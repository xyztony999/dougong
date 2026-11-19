package panel

import (
	"context"
	"fmt"

	storepanel "github.com/dougong/dougong/internal/store/panel"
)

// 本文件是 panel-core 对 db-tools 的进程内读契约（api-db-tools.md D-03/
// 1.5：panel_config.bind_domain/监听地址/入口路径为落点选择与内部端点 URL
// 构造输入）。只读投影，零写副作用。

// ToolAccessInfo 面板访问形态（db-tools 消费）。EntryPath 为生效安全入口
// 前缀（含前导 /；关闭=""）；ListenHost 为绑定主机（all=0.0.0.0 枚举表明文
// 映射，D-P02 同口径）。
type ToolAccessInfo struct {
	BindDomain   string
	ListenHost   string
	ListenPort   int
	EntryPath    string
	HTTPSEnabled bool
}

// PanelAccess 读取面板访问形态（每请求实时读 panel_config——绑定/入口
// 变更即时可见；注入内容安装时烘焙的时效声明见 db-tools tasks.md A-DBT6）。
func (s *Service) PanelAccess(ctx context.Context) (ToolAccessInfo, error) {
	cfg, err := s.st.Config().Get(ctx)
	if err != nil {
		return ToolAccessInfo{}, fmt.Errorf("panel: tool access get config: %w", err)
	}
	info := ToolAccessInfo{
		ListenPort:   cfg.ListenPort,
		HTTPSEnabled: cfg.HTTPSMode != storepanel.HTTPSModeOff,
	}
	if cfg.BindDomain != nil {
		info.BindDomain = *cfg.BindDomain
	}
	switch cfg.ListenScope {
	case storepanel.ListenScopeLocalhost:
		info.ListenHost = "127.0.0.1"
	case storepanel.ListenScopeCustom:
		if cfg.ListenAddress != nil && *cfg.ListenAddress != "" {
			info.ListenHost = *cfg.ListenAddress
		} else {
			info.ListenHost = "0.0.0.0"
		}
	default:
		info.ListenHost = "0.0.0.0"
	}
	if cfg.EntryPathEnabled && cfg.EntryPath != nil && *cfg.EntryPath != "" {
		info.EntryPath = "/" + *cfg.EntryPath
	}
	return info, nil
}
