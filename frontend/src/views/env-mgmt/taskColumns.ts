import { h } from 'vue'
import { NButton, NTag, NTooltip } from 'naive-ui'
import type { DataTableColumns, SelectOption } from 'naive-ui'
import type { EnvTaskItem } from '@/types/env'
import type { EnvTaskStatus, EnvType } from '@/types/enums'
import {
  BOOTSTRAP_STATUS,
  ENV_TASK_ACTION,
  ENV_TASK_STATUS,
  ENV_TYPE,
  dictItem
} from '@/types/enums'
import { formatDateTime } from '@/utils/format'

export const envTypeOptions: SelectOption[] = (Object.keys(ENV_TYPE) as EnvType[]).map((v) => ({
  label: ENV_TYPE[v].label,
  value: v
}))

export const statusOptions: SelectOption[] = (Object.keys(ENV_TASK_STATUS) as EnvTaskStatus[]).map(
  (v) => ({
    label: ENV_TASK_STATUS[v].label,
    value: v
  })
)

export function envTaskColumns(onDetail: (id: number) => void): DataTableColumns<EnvTaskItem> {
  return [
    { title: 'ID', key: 'id', width: 70 },
    {
      title: '环境',
      key: 'envType',
      width: 110,
      render: (row) => dictItem(ENV_TYPE, row.envType).label
    },
    {
      title: '动作',
      key: 'action',
      width: 90,
      render: (row) => dictItem(ENV_TASK_ACTION, row.action).label
    },
    {
      title: '状态',
      key: 'status',
      width: 110,
      render: (row) =>
        h(
          NTag,
          { type: dictItem(ENV_TASK_STATUS, row.status).tag, size: 'small', bordered: false },
          { default: () => dictItem(ENV_TASK_STATUS, row.status).label }
        )
    },
    {
      title: '引导',
      key: 'bootstrapStatus',
      width: 90,
      render: (row) => {
        if (!row.bootstrapStatus) return '—'
        const d = dictItem(BOOTSTRAP_STATUS, row.bootstrapStatus)
        const tag = h(NTag, { type: d.tag, size: 'small', bordered: false }, { default: () => d.label })
        if (!row.bootstrapNote) return tag
        return h(NTooltip, { trigger: 'hover' }, { trigger: () => tag, default: () => row.bootstrapNote })
      }
    },
    { title: '发起者', key: 'requestedBy', width: 100 },
    { title: '发起时间', key: 'createdAt', width: 170, render: (row) => formatDateTime(row.createdAt) },
    { title: '结束时间', key: 'finishedAt', width: 170, render: (row) => formatDateTime(row.finishedAt) },
    {
      title: '操作',
      key: 'actions',
      width: 80,
      render: (row) =>
        h(
          NButton,
          { size: 'small', text: true, type: 'primary', onClick: () => onDetail(row.id) },
          { default: () => '详情' }
        )
    }
  ]
}
