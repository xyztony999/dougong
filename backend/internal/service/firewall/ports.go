// Package firewall 是防火墙应用服务（api-firewall.md §4.1）：BackendDetector
// 结论组装（D-01 权威动作面）、RuleProjector（投影+筛选排序+isPanelPort
// 标注）、PortComparisonService（监听×规则对照 D-04）、RuleMutationService
// （写编排：校验→涉险保护→命令执行→审计，I-05/I-07/D-09）。零持久化
// （D-10）：无表无 SQL，投影恒为当次后端读取快照（I-01 无缓存层）。
package firewall

import (
	"context"

	"github.com/dougong/dougong/internal/adapter/firewallx"
	"github.com/dougong/dougong/internal/adapter/netlisten"
	"github.com/dougong/dougong/internal/service/audit"
)

// Gateway 是 ufw/firewalld 命令网关契约（api §4.1.5 FirewallGateway；接口
// 定义在消费方，实现为 internal/adapter/firewallx.Gateway，main 装配注入
// ——backend-01 §1）。全部方法 ctx 首参（backend-rules §1.1）；错误以包级
// 哨兵表达（ErrNoBackend/ErrNotActive/ErrAlreadyExists/ErrRuleNotFound，
// 其余如实包装原文）；测试以 fake 实现注入（CLI 交互不出本模块单测边界）。
type Gateway interface {
	// Detect 即时探测（D-01：ufw 优先→firewalld→none；只读）。
	Detect(ctx context.Context) (firewallx.BackendInfo, error)
	// ListRules 端口级规则实况投影（I-01/I-02；含原文语义）。
	ListRules(ctx context.Context) ([]firewallx.BackendRule, error)
	// AddRule 添加（ufw 四动作；firewalld permanent+runtime 双写 D-08）。
	AddRule(ctx context.Context, spec firewallx.RuleSpec) (firewallx.AddResult, error)
	// DeleteRule 删除（ruleKey=后端原生定位串 D-03）。
	DeleteRule(ctx context.Context, ruleKey string) (firewallx.DelResult, error)
}

// ListenPortSource 是监听端口实况取数契约（api §4.1.5；实现为
// internal/adapter/netlisten.Source——与系统监控采集统一 gopsutil 来源，
// D1 口径-3 单一只读通道）。
type ListenPortSource interface {
	List(ctx context.Context) ([]netlisten.ListenPort, error)
}

// PanelListenConfig 是面板监听端口读取契约（api §4.1.5；D-06 涉险判定
// 基准。设计文假设 internal/config 直读；实际监听值在 panel_config 落库且
// DOUGONG_ADDR 可覆盖——A-W9 裁定 main 装配期解析进程实际绑定端口注入
// 静态实现，保持「进程内直读、非 HTTP 自调用」语义）。
type PanelListenConfig interface {
	GetListenPort() (uint16, error)
}

// staticPanelPort 是 PanelListenConfig 的静态值实现（main 装配注入；纯内存
// 读无 IO，故无 ctx 参数——backend-rules §1.1 IO 语境）。
type staticPanelPort uint16

// GetListenPort 返回装配期解析的面板实际监听端口。
func (p staticPanelPort) GetListenPort() (uint16, error) { return uint16(p), nil }

// StaticPanelListen 以固定端口构造 PanelListenConfig（main 装配用）。
func StaticPanelListen(port uint16) PanelListenConfig { return staticPanelPort(port) }

// AuditRecorder 是审计写入契约（account-auth OHS：AuditService.Record，R-03；
// D-09 降级语义：重试一次仍失败记面板运行日志，不影响操作结果返回）。
type AuditRecorder interface {
	Record(ctx context.Context, e audit.Entry) error
}
