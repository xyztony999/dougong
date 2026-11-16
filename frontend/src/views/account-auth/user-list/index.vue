<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { useMessage } from 'naive-ui'
import { NButton, NPopconfirm, NTag } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { userDelete, userList } from '@/api/auth'
import { useAuthStore } from '@/stores/auth'
import { PANEL_ROLE, PANEL_USER_STATUS, boolTag, dictItem } from '@/types/enums'
import type { UserListItem } from '@/types/auth'
import { formatDateTime } from '@/utils/format'
import ResetPasswordModal from './components/ResetPasswordModal.vue'
import UserCreateModal from './components/UserCreateModal.vue'

const message = useMessage()
const authStore = useAuthStore()

const records = ref<UserListItem[]>([])
const loading = ref(false)
const loadError = ref(false)

const showCreate = ref(false)
const resetTarget = ref<UserListItem | null>(null)

/** 列定义（admin 与 readonly 共用列；操作列仅 admin 渲染——readonly 负空间，ADR-018） */
const columns = computed<DataTableColumns<UserListItem>>(() => {
  const base: DataTableColumns<UserListItem> = [
    { title: '序号', key: 'index', width: 60, render: (_row, index) => index + 1 },
    { title: '用户名', key: 'username', width: 180, ellipsis: { tooltip: true } },
    {
      title: '角色',
      key: 'role',
      width: 100,
      render: (row) => dictItem(PANEL_ROLE, row.role).label
    },
    {
      title: '状态',
      key: 'status',
      width: 100,
      render: (row) => {
        const item = dictItem(PANEL_USER_STATUS, row.status)
        return h(NTag, { type: item.tag, size: 'small', bordered: false }, { default: () => item.label })
      }
    },
    {
      title: 'TOTP',
      key: 'totpEnabled',
      width: 80,
      render: (row) => {
        const item = boolTag(row.totpEnabled, '已启用', '未启用')
        return h(NTag, { type: item.tag, size: 'small', bordered: false }, { default: () => item.label })
      }
    },
    { title: '锁定至', key: 'lockedUntil', width: 180, render: (row) => formatDateTime(row.lockedUntil) },
    { title: '创建时间', key: 'createdAt', width: 180, render: (row) => formatDateTime(row.createdAt) }
  ]
  if (authStore.isAdmin) {
    base.push({
      title: '操作',
      key: 'actions',
      width: 160,
      fixed: 'right',
      render: (row) => [
        h(
          NButton,
          { size: 'small', onClick: () => onResetPassword(row) },
          { default: () => '重置密码' }
        ),
        h(
          NPopconfirm,
          { onPositiveClick: () => void onDelete(row) },
          {
            trigger: () =>
              h(
                NButton,
                { size: 'small', type: 'error', ghost: true },
                { default: () => '删除' }
              ),
            default: () => `确认删除用户 ${row.username}？其全部会话将被吊销。`
          }
        )
      ]
    })
  }
  return base
})

async function load(): Promise<void> {
  loading.value = true
  loadError.value = false
  try {
    const data = await userList()
    records.value = data.items
  } catch {
    loadError.value = true
    // 错误提示已由拦截器统一处理
  } finally {
    loading.value = false
  }
}

function onResetPassword(row: UserListItem): void {
  resetTarget.value = row
}

async function onDelete(row: UserListItem): Promise<void> {
  try {
    await userDelete({ userId: row.id })
    message.success(`用户 ${row.username} 已删除`)
  } catch {
    // AUTH_010（唯一管理员保护）/AUTH_014（不能删除自己）已由拦截器提示
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
    <n-card title="用户管理">
      <template #header-extra>
        <!-- 工具栏：新增用户（admin only；readonly 无工具栏） -->
        <n-button v-if="authStore.isAdmin" type="primary" @click="showCreate = true">
          新增用户
        </n-button>
      </template>
      <!-- v1.3 加载失败态（loadError NAlert+重试，不呈现裸空表格——v0.1.2 反馈第 5 项「正文空白」修复） -->
      <n-alert v-if="!loading && loadError" type="error" title="用户列表加载失败">
        请求未成功，可重试。
        <n-button size="small" @click="load">重试</n-button>
      </n-alert>
      <n-data-table
        v-else
        :columns="columns"
        :data="records"
        :loading="loading"
        :scroll-x="1060"
        :row-key="(row: UserListItem) => row.id"
      />
      <n-empty v-if="!loading && !loadError && records.length === 0" description="暂无用户" class="empty" />
    </n-card>

    <user-create-modal v-model:show="showCreate" @created="load" />
    <reset-password-modal :user="resetTarget" @close="resetTarget = null" @reset="load" />
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.empty {
  padding: 24px 0;
}
</style>
