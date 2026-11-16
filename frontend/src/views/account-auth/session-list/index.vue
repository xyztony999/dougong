<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { useMessage } from 'naive-ui'
import { NButton, NPopconfirm, NTag } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { sessionList, sessionRevoke } from '@/api/auth'
import { useAuthStore } from '@/stores/auth'
import type { SessionListItem } from '@/types/auth'
import { formatDateTime } from '@/utils/format'

const message = useMessage()
const authStore = useAuthStore()

const records = ref<SessionListItem[]>([])
const loading = ref(false)

/** 列定义：API 全量返回有效会话、按 lastActiveAt 倒序（无查询参数——诚实无查询区） */
const columns = computed<DataTableColumns<SessionListItem>>(() => {
  const base: DataTableColumns<SessionListItem> = [
    { title: '序号', key: 'index', width: 60, render: (_row, index) => index + 1 },
    { title: '用户名', key: 'username', width: 140 },
    { title: '会话标识', key: 'id', width: 140, render: (row) => row.id },
    { title: '建立时间', key: 'createdAt', width: 180, render: (row) => formatDateTime(row.createdAt) },
    { title: '最后活跃', key: 'lastActiveAt', width: 180, render: (row) => formatDateTime(row.lastActiveAt) },
    {
      title: '来源',
      key: 'sourceIp',
      width: 140,
      render: (row) => row.sourceIp ?? '—'
    },
    {
      title: '标记',
      key: 'current',
      width: 100,
      render: (row) =>
        row.current
          ? h(NTag, { type: 'info', size: 'small', bordered: false }, { default: () => '当前会话' })
          : null
    }
  ]
  if (authStore.isAdmin) {
    base.push({
      title: '操作',
      key: 'actions',
      width: 120,
      fixed: 'right',
      render: (row) =>
        h(
          NPopconfirm,
          {
            // 当前会话禁用强制下线（登出走框架登出，PageSpec 4.2）
            onPositiveClick: () => void onRevoke(row)
          },
          {
            trigger: () =>
              h(
                NButton,
                { size: 'small', type: 'error', ghost: true, disabled: row.current },
                { default: () => '强制下线' }
              ),
            default: () => `确认强制下线 ${row.username}？该用户将立即被退出登录。`
          }
        )
    })
  }
  return base
})

async function load(): Promise<void> {
  loading.value = true
  try {
    const data = await sessionList()
    records.value = data.items
  } catch {
    // AUTH_002 会话失效：拦截器已跳登录；其余错误已统一提示
  } finally {
    loading.value = false
  }
}

async function onRevoke(row: SessionListItem): Promise<void> {
  try {
    await sessionRevoke({ sessionId: row.id })
    message.success(`已强制下线 ${row.username}`)
  } catch {
    // 错误已由拦截器统一提示
  } finally {
    await load()
  }
}

onMounted(() => {
  void load()
})
</script>

<template>
  <div class="page">
    <n-card title="在线会话">
      <template #header-extra>
        <n-button size="small" @click="load">刷新</n-button>
      </template>
      <n-data-table
        :columns="columns"
        :data="records"
        :loading="loading"
        :scroll-x="960"
        :row-key="(row: SessionListItem) => row.id"
      />
    </n-card>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
</style>
