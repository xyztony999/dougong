/**
 * database-mgmt 模块内共享渲染助手（3 页面共用：引擎/连接状态/凭据/授权档位 Tag、
 * 实例标识文本组装）。Tag 语义色 = PageSpec 第 3 节颜色方案；redis 引擎紫色无 NTag
 * 语义型 → rgba 语义色（site-mgmt 先例，frontend-rules §4.1 禁大面积硬编码 hex 界内）。
 */
import { h } from 'vue'
import { NTag } from 'naive-ui'
import type { VNode } from 'vue'
import {
  DB_CONN_MODE,
  DB_ENGINE,
  INSTANCE_CONN_STATE,
  boolTag,
  dictItem
} from '@/types/enums'
import type { DbInstanceItem } from '@/types/database'

/** redis 引擎紫色（PageSpec 第 3 节 db_engine::redis=purple） */
const REDIS_TAG_COLOR = {
  color: 'rgba(112,64,208,0.14)',
  textColor: 'rgba(112,64,208,1)',
  borderColor: 'rgba(112,64,208,0.32)'
} as const

function tag(label: string, type: 'default' | 'info' | 'success' | 'warning' | 'error'): VNode {
  return h(NTag, { type, size: 'small', bordered: false }, { default: () => label })
}

/** 引擎 Tag：mysql/mariadb=blue（info）、redis=purple（rgba 语义色） */
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
  return tag(d.label, d.tag === 'info' ? 'info' : 'default')
}

/** 连接方式 Tag（db_conn_mode，无语义色） */
export function connModeTag(connMode: string): VNode {
  const d = dictItem(DB_CONN_MODE, connMode)
  return tag(d.label, 'default')
}

/** 连接状态 Tag（reachable=green / unreachable=orange） */
export function connStateTag(connState: string): VNode {
  const d = dictItem(INSTANCE_CONN_STATE, connState)
  return tag(d.label, d.tag === 'warning' ? 'warning' : 'success')
}

/** 凭据 Tag（credentialsSet 布尔：true=已配置 green / false=未配置 gray，PageSpec 第 3 节） */
export function credentialsTag(credentialsSet: boolean): VNode {
  const d = boolTag(credentialsSet, '已配置', '未配置')
  return tag(d.label, d.tag === 'success' ? 'success' : 'default')
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
