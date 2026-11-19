package panel

import (
	"context"
	"log/slog"
)

// 本文件是 panel-core 配置变更通知钩子提供方（api-db-tools v1.3 D-20③⑤
// 消费契约兑现，S-11-01 机制侧变更）：config/update bindDomain 变更成功
// （D-P12）后进程内同步通知已注册观察者——db-tools LandingService 判定
// 自动落地块存在则重渲染 server_name 经 ConfigApply 刷新（域名联动主通道；
// install/open 惰性校正为兜底）。

// ConfigChangeHook 配置变更通知钩子契约（消费方期望签名，api-db-tools
// §4.1.5）：bindDomain=""=解绑；调用为进程内同步。调用失败（panic）不阻断
// 配置变更——panel-core 侧 recover+slog（panel_config 权威已成功）；观察者
// 侧刷新失败自行记日志，惰性校正兜底自愈。
type ConfigChangeHook interface {
	OnBindDomainChanged(ctx context.Context, bindDomain string)
}

// RegisterConfigChangeHook 注册配置变更观察者（main 装配阶段调用，沿
// BootstrapAfterInstall OHS 先例；运行期只读——HTTP 服务启动前完成注册）。
func (s *Service) RegisterConfigChangeHook(h ConfigChangeHook) {
	if h != nil {
		s.configHooks = append(s.configHooks, h)
	}
}

// notifyBindDomainChanged bindDomain 变更成功后通知观察者（D-20③：钩子
// 失败不阻断配置变更——逐观察者 recover 隔离+slog）。
func (s *Service) notifyBindDomainChanged(ctx context.Context, bindDomain string) {
	for _, h := range s.configHooks {
		s.invokeConfigHook(ctx, h, bindDomain)
	}
}

// invokeConfigHook 单观察者调用（panic 隔离——配置变更权威已成功，观察者
// 异常不得回滚/阻断）。
func (s *Service) invokeConfigHook(ctx context.Context, h ConfigChangeHook, bindDomain string) {
	defer func() {
		if r := recover(); r != nil {
			slog.ErrorContext(ctx, "panel: config change hook panic (config change not blocked)", "panic", r)
		}
	}()
	h.OnBindDomainChanged(ctx, bindDomain)
}
