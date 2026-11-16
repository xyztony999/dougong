/**
 * firewall api 层 —— 5 Action 与 api-firewall.md 4.1.4 一一对应（S7，无虚构接口）。
 * 读 3 GET（admin+readonly，ADR-018 全可看）；写 2 POST（admin 专属 + CSRF——
 * http.ts 统一注入 X-CSRF-Token）。函数名 = PageSpec 第 2 节 Action 名。
 */
import type {
  FirewallBackendStatusData,
  FirewallPortListData,
  FirewallRuleCreateReq,
  FirewallRuleCreateResult,
  FirewallRuleDeleteReq,
  FirewallRuleDeleteResult,
  FirewallRuleListData
} from '@/types/firewall'
import { request } from './http'

export interface RuleListQuery {
  /** firewall_rule_action 多值逗号拼接（如 allow,limit）；空=全部（api D-02） */
  action?: string
  /** 单值；空=全部 */
  protocol?: string
}

/** backendStatus：后端识别结论（即时探测 D-01；supportedActions 服务端权威动作面） */
export function backendStatus(): Promise<FirewallBackendStatusData> {
  return request<FirewallBackendStatusData>('/firewall/backend/status')
}

/** ruleList：规则投影清单（全量不分页 D-02；服务端筛选+端口数值升序+isPanelPort 标注） */
export function ruleList(q: RuleListQuery = {}): Promise<FirewallRuleListData> {
  return request<FirewallRuleListData>('/firewall/rule/list', {
    query: { action: q.action, protocol: q.protocol }
  })
}

/** portList：监听端口对照清单（无筛选参数，全量视图；external/openState 服务端计算 D-04） */
export function portList(): Promise<FirewallPortListData> {
  return request<FirewallPortListData>('/firewall/port/list')
}

/** ruleCreate：添加规则（面板端口保护两段式——confirmDanger 由 FW_005 流程追加，I-05/D-06） */
export function ruleCreate(req: FirewallRuleCreateReq): Promise<FirewallRuleCreateResult> {
  return request<FirewallRuleCreateResult>('/firewall/rule/create', {
    method: 'POST',
    body: req
  })
}

/** ruleDelete：删除规则（ruleKey 不透明引用寻址，D-03；涉险两段式同上） */
export function ruleDelete(req: FirewallRuleDeleteReq): Promise<FirewallRuleDeleteResult> {
  return request<FirewallRuleDeleteResult>('/firewall/rule/delete', {
    method: 'POST',
    body: req
  })
}
