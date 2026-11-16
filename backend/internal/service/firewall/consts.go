package firewall

import "github.com/dougong/dougong/internal/adapter/firewallx"

// 枚举取值（workspace/01-solution/04-business-enum.md，D2 定稿引用无新增）。
const (
	// firewall_rule_action：allow | deny | reject | limit。
	ActionAllow  = "allow"
	ActionDeny   = "deny"
	ActionReject = "reject"
	ActionLimit  = "limit"

	// firewall_rule_protocol：tcp | udp。
	ProtocolTCP = "tcp"
	ProtocolUDP = "udp"

	// listen_port_state（D-04 两态对照）。
	OpenAllowedByRule = "allowed_by_rule"
	OpenNoAllowRule   = "no_allow_rule"

	// 方向固定入站（MVP 单值维度，非枚举池条目；响应呈现用，D-05）。
	DirectionIngress = "ingress"
)

// 审计业务常量（api §3.4：操作审计细粒度动作码与对象类型，非枚举池条目；
// object_id=ruleKey）。
const (
	AuditActionRuleCreated = "firewall_rule_created"
	AuditActionRuleDeleted = "firewall_rule_deleted"
	ObjectTypeFirewallRule = "firewall_rule"
)

// DangerHint FW_005 data.hint 固定文案（两段式确认引导，I-05/D-06）。
const DangerHint = "该操作命中面板自身监听端口，可能使面板失去访问入口；如确认继续，请重新提交并携带 confirmDanger=true"

// 双层状态固定值（FW_007 data；D-08 不回滚 permanent——写入是用户意图）。
const (
	LayerWritten = "written"
	LayerFailed  = "failed"
)

// validActions / validProtocols 枚举值域（R-14 服务端校验依据，D-05）。
var (
	validActions = map[string]bool{
		ActionAllow: true, ActionDeny: true, ActionReject: true, ActionLimit: true,
	}
	validProtocols = map[string]bool{
		ProtocolTCP: true, ProtocolUDP: true,
	}
)

// SupportedActions 返回后端动作支持面（D-01 服务端权威计算：ufw→全四值 /
// firewalld→仅 allow / none→空；前端添加入口照此渲染，不自行推断）。
func SupportedActions(backendType string) []string {
	switch backendType {
	case firewallx.BackendUFW:
		return []string{ActionAllow, ActionDeny, ActionReject, ActionLimit}
	case firewallx.BackendFirewalld:
		return []string{ActionAllow}
	default:
		return []string{}
	}
}

// isAllowFamily allow 族判定（allow/limit——D-04 对照命中族与 I-05 删除涉险族）。
func isAllowFamily(action string) bool {
	return action == ActionAllow || action == ActionLimit
}
