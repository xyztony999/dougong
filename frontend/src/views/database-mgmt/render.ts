/**
 * database-mgmt 模块内共享渲染助手（清单+治理+浏览页共用：引擎/连接状态/凭据/授权档位 Tag、
 * 实例标识文本组装）。Tag 语义色 = PageSpec 第 3 节颜色方案；redis 引擎紫色无 NTag
 * 语义型 → rgba 语义色（site-mgmt 先例，frontend-rules §4.1 禁大面积硬编码 hex 界内）。
 * v1.4 F2：connStateTag/credentialsTag 外包 NTooltip 语义（文案常量 types/database.ts，
 * 治理页/浏览页摘要条复用同一助手自动获益）；F1：redundantSocketTag 冗余 Socket 标注（D-21）。
 */
import { h } from 'vue'
import { NTag, NTooltip } from 'naive-ui'
import type { VNode } from 'vue'
import {
  DB_CONN_MODE,
  DB_ENGINE,
  INSTANCE_CONN_STATE,
  boolTag,
  dictItem
} from '@/types/enums'
import type { DbInstanceItem } from '@/types/database'
import {
  CONN_STATE_TOOLTIP,
  CREDENTIALS_TOOLTIP,
  REDUNDANT_SOCKET_TOOLTIP
} from '@/types/database'

/** redis 引擎紫色（PageSpec 第 3 节 db_engine::redis=purple） */
const REDIS_TAG_COLOR = {
  color: 'rgba(112,64,208,0.14)',
  textColor: 'rgba(112,64,208,1)',
  borderColor: 'rgba(112,64,208,0.32)'
} as const

function tag(label: string, type: 'default' | 'info' | 'success' | 'warning' | 'error'): VNode {
  return h(NTag, { type, size: 'small', bordered: false }, { default: () => label })
}

/** 引擎 Tag：mysql/mariadb=blue（info）、redis=purple（rgba 语义色）、mongodb=green（success，v1.2） */
export function engineTag(engine: string): VNode {
  if (engine === 'redis') {
    const d = dictItem(DB_ENGINE, engine)
    return h(
      NTag,
      { size: 'small', bordered: false, color: REDIS_TAG_COLOR },
      { default: () => d.label }
    )
  }
  const d = dictItem(DB_ENGINE, engine)
  return tag(d.label, d.tag === 'info' ? 'info' : d.tag === 'success' ? 'success' : 'default')
}

/** 连接方式 Tag（db_conn_mode，无语义色） */
export function connModeTag(connMode: string): VNode {
  const d = dictItem(DB_CONN_MODE, connMode)
  return tag(d.label, 'default')
}

/** tooltip 包裹（v1.4 F2：trigger=Tag 本体，default=语义文案） */
function withTooltip(trigger: VNode, text: string): VNode {
  return h(NTooltip, null, { trigger: () => trigger, default: () => text })
}

/** 连接状态 Tag（reachable=green / unreachable=orange）+ NTooltip 语义（v1.4 F2） */
export function connStateTag(connState: string): VNode {
  const d = dictItem(INSTANCE_CONN_STATE, connState)
  return withTooltip(
    tag(d.label, d.tag === 'warning' ? 'warning' : 'success'),
    CONN_STATE_TOOLTIP
  )
}

/** 凭据 Tag（credentialsSet 布尔：true=已配置 green / false=未配置 gray）+ NTooltip 语义（v1.4 F2） */
export function credentialsTag(credentialsSet: boolean): VNode {
  const d = boolTag(credentialsSet, '已配置', '未配置')
  return withTooltip(
    tag(d.label, d.tag === 'success' ? 'success' : 'default'),
    CREDENTIALS_TOOLTIP
  )
}

/**
 * 冗余 Socket 标注（v1.4 F1/D-21：redundantSocket=true 行——已配凭据的冗余 socket 档案
 * 保留+标注，保守清理口径不做凭据自动迁移；NTag(gray)+NTooltip 引导改配 TCP 行后手动移除）。
 */
export function redundantSocketTag(): VNode {
  return withTooltip(tag('冗余 Socket', 'default'), REDUNDANT_SOCKET_TOOLTIP)
}

/**
 * 实例标识文本（PageSpec 4.1：呈现 host:port 或 socket 路径——api D-01 组装规则的
 * 展示形态；identity_key 原串（engine|socket:{path} / engine|tcp:{host}:{port}）不下发）。
 */
export function identityText(item: Pick<
  DbInstanceItem,
  'connMode' | 'socketPath' | 'host' | 'port'
>): string {
  if (item.connMode === 'socket') return item.socketPath ?? '—'
  return `${item.host ?? ''}:${item.port ?? '—'}`
}

/** ---- 函数式组件形态（治理页摘要条模板内直接使用，Vue3 函数组件=返回 VNode 的函数） ---- */

export function EngineTag(props: { engine: string }): VNode {
  return engineTag(props.engine)
}

export function ConnStateTag(props: { connState: string }): VNode {
  return connStateTag(props.connState)
}

/** 凭据 Tag 函数组件（摘要条模板内使用；v1.4 F2 同款 tooltip 助手） */
export function CredentialsTag(props: { credentialsSet: boolean }): VNode {
  return credentialsTag(props.credentialsSet)
}
