/**
 * firewall API 类型 —— 与 api-firewall.md v1.0 4.1.4 对齐（唯一事实源）。
 * 零持久化投影（D-10）：rule/port 响应为当次后端/系统实况快照（I-01 无缓存，
 * backend=none 不返回空规则列表而返回 FW_009——前端引导态先行判定）。
 * - ruleKey 为后端原生命令定位串（D-03），前端作不透明引用透传回 delete；
 * - isPanelPort 为服务端标注（D-06 从严口径），前端仅作预警线索（权威判定 FW_005 服务端兜底）；
 * - supportedActions 为服务端权威动作支持面（D-01/I-04：ufw 全四值 / firewalld [allow] / none []）。
 * 无分页（D-02 全量返回）；generatedAt=投影生成时刻。
 */
import type { FirewallBackend, FirewallRuleAction, FirewallRuleProtocol, ListenPortState } from './enums'

/** GET /firewall/backend/status 响应 data（backend=none 时 active=false/version=null/supportedActions=[]） */
export interface FirewallBackendStatusData {
  backend: FirewallBackend
  active: boolean
  version: string | null
  /** 服务端权威计算的动作支持面（D-01/I-04：添加入口照此渲染，前端不自行推断） */
  supportedActions: FirewallRuleAction[]
  generatedAt: string
}

/** 规则投影条目（api 4.1.1 字段清单） */
export interface FirewallRuleItem {
  /** 后端原生命令定位串（D-03），删除寻址的不透明引用 */
  ruleKey: string
  /** 端口显示形："80" 或 "8080-8090"（D-05） */
  port: string
  protocol: FirewallRuleProtocol
  /** 方向固定入站（MVP 单值维度常量，api 4.1.1） */
  direction: 'ingress'
  /** 来源限制 CIDR；null=任意来源 */
  source: string | null
  action: FirewallRuleAction
  /** 后端原始语义标识（ufw 规则原文 / firewalld 端口串），与后端自身工具输出互认 */
  rawSpec: string
  /** 服务端计算：规则端口数值直接命中面板监听端口（D-06） */
  isPanelPort: boolean
}

/** rule/list 随行携带的后端摘要（宿主 S-42-01 页头呈现） */
export interface FirewallBackendSummary {
  backend: FirewallBackend
  active: boolean
}

/** GET /firewall/rule/list 响应 data（全量不分页 D-02；服务端筛选+端口数值升序） */
export interface FirewallRuleListData {
  items: FirewallRuleItem[]
  total: number
  backend: FirewallBackendSummary
  generatedAt: string
}

/** 监听进程（取不到时整体 null 或字段 null，不虚构——monitoring 同口径） */
export interface FirewallPortProcess {
  pid: number | null
  name: string | null
}

/** 监听端口对照条目（过程数据，读取响应即消亡；external/openState 为服务端计算 D-04） */
export interface FirewallPortItem {
  /** 透传 tcp/tcp6/udp/udp6（开放集合不枚举化，api D-04） */
  proto: string
  /** 监听地址原样 */
  addr: string
  port: number
  process: FirewallPortProcess | null
  /** 监听地址非 loopback（服务端计算，D-04） */
  external: boolean
  /** 对照派生值：allow 族命中→allowed_by_rule，否则 no_allow_rule（D-04） */
  openState: ListenPortState
}

/** GET /firewall/port/list 响应 data */
export interface FirewallPortListData {
  items: FirewallPortItem[]
  total: number
  generatedAt: string
}

/** POST /firewall/rule/create 请求（direction 不入表单/不入请求——D-05 单值维度） */
export interface FirewallRuleCreateReq {
  /** "N" 或 "N-M"（1-65535 且 N≤M，FW_001） */
  port: string
  protocol: FirewallRuleProtocol
  /** 缺省=任意来源；非缺省须为合法 IPv4/IPv6 CIDR（FW_002） */
  source?: string
  action: FirewallRuleAction
  /** FW_005 两段式确认载体（api 3.4；每次涉险操作独立要求、不记忆） */
  confirmDanger?: boolean
}

/** POST /firewall/rule/create 成功 data：写入后定位引用 */
export interface FirewallRuleCreateResult {
  ruleKey: string
  port: string
  protocol: FirewallRuleProtocol
  action: FirewallRuleAction
}

/** POST /firewall/rule/delete 请求（ruleKey 寻址 D-03） */
export interface FirewallRuleDeleteReq {
  ruleKey: string
  confirmDanger?: boolean
}

/** POST /firewall/rule/delete 成功 data */
export interface FirewallRuleDeleteResult {
  deleted: boolean
  ruleKey: string
}

/** FW_005 data（api 4.1.4：{panelPort, rulePort, action, hint}——驱动前端危险确认 Modal） */
export interface FirewallDangerInfo {
  panelPort: number
  rulePort: string
  action: string
  hint: string
}

/**
 * FW_001~010 错误文案（PageSpec 第 1 节错误处理，逐条对应 api 3.4）：
 * 001/004=「参数非法」兜底（正常 UI 由控件与 supportedActions 权威面约束不可达）；
 * 002=Dialog source 字段错误（正常 UI 预校验不可达）；003/006/008 为行为分支文案；
 * 005/007 走专属 Modal；009 兜底跳引导态；010 页面级 NAlert。
 */
export const FW_ERROR_TEXT: Readonly<Record<string, string>> = {
  FW_001: '参数非法',
  FW_002: '来源限制须为合法 IPv4/IPv6 CIDR',
  FW_003: '该规则已不存在（可能已在面板外删除）',
  FW_004: '参数非法',
  FW_005: '面板端口涉险，需二次确认',
  FW_006: '等价规则已存在，未重复添加',
  FW_007: '永久配置已写入，运行态应用失败',
  FW_008: '后端命令执行失败',
  FW_009: '未检测到受支持的防火墙后端',
  FW_010: '防火墙后端不可用（如 firewalld 服务停止）'
}
