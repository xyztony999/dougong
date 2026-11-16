<script setup lang="ts">
/**
 * 回收站列表页（PageSpec 第 4A 节，路由 /files/trash）。
 * admin 全功能（还原/清空强警告）；readonly 子集——仅浏览（无清空按钮、无操作列，负空间）。
 * 无排序（deleted_at 倒序服务端固定 D-03）、无分页（全量）、无选择列（还原逐行/清空全站）；
 * trash/list 无 generatedAt——不渲染投影时刻（api 4.1.4，不虚构字段）；
 * 还原 FILE_005 → 冲突决策（无「跳过全部」，api D-07）；purge=NModal 强警告终局销毁（R-17 两级确认口径）。
 */
import { computed, h, onMounted, onUnmounted, ref } from 'vue'
import { useMessage } from 'naive-ui'
import { NButton, NIcon } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { ApiError } from '@/api/http'
import { trashPurge, trashRestore } from '@/api/file'
import { useAuthStore } from '@/stores/auth'
import { useTrashStore } from '@/stores/trash'
import { TRASH_ENTRY_TYPE, isConflictList } from '@/types/file'
import type { TrashEntryType, TrashItem } from '@/types/file'
import { formatBytes, formatDateTime } from '@/utils/format'
import { fileActionError } from '../errors'
import { entryIcon } from '../icons'
import type { ConflictPayload } from '../components/ConflictModal.vue'
import ConflictModal from '../components/ConflictModal.vue'

const store = useTrashStore()
const authStore = useAuthStore()
const message = useMessage()

/** ---- 还原（4A.3：在站条目恒可还原；冲突走 4.4.8 决策非轻确认） ---- */

const pendingConflict = ref<ConflictPayload | null>(null)

async function runRestore(item: TrashItem, onConflict: 'ask' | 'overwrite'): Promise<void> {
  store.setInFlight(item.id)
  try {
    const data = await trashRestore(item.id, onConflict)
    message.success(`已还原至 ${data.originalPath}`)
    await store.loadList()
  } catch (e) {
    if (e instanceof ApiError && e.is('FILE_005') && isConflictList(e.data)) {
      // 还原场景：覆盖全部/取消——无「跳过全部」（api D-07）
      pendingConflict.value = {
        conflicts: e.data,
        allowSkip: false,
        // 还原决策通道仅 overwrite（D-07）；「取消」=不重发即放弃
        retry: async (_strategy) => {
          await runRestore(item, 'overwrite')
        }
      }
      return
    }
    if (e instanceof ApiError) {
      const info = fileActionError(e)
      if (info !== null) {
        if (info.level === 'warning') message.warning(info.text)
        else message.error(info.text)
        if (info.refreshAfter) await store.loadList()
      }
    }
  } finally {
    store.releaseInFlight(item.id)
  }
}

function onRestore(item: TrashItem): void {
  void runRestore(item, 'ask')
}

async function onConflictDecide(): Promise<void> {
  const payload = pendingConflict.value
  pendingConflict.value = null
  if (payload === null) return
  try {
    await payload.retry('overwrite')
  } catch {
    // runRestore 内部已分流提示
  }
}

/** ---- 清空（4A.1：NModal 强警告——终局销毁不可还原；空站禁用避免无意义请求） ---- */

const purgeConfirmShow = ref(false)
const purging = ref(false)

const purgeDisabled = computed(() => store.total === 0)

async function onPurge(): Promise<void> {
  purging.value = true
  try {
    const data = await trashPurge()
    message.success(`已清空 ${data.purgedCount} 条`)
    purgeConfirmShow.value = false
    await store.loadList()
  } catch (e) {
    if (e instanceof ApiError) {
      const info = fileActionError(e)
      if (info !== null) {
        if (info.level === 'warning') message.warning(info.text)
        else message.error(info.text)
      }
    }
  } finally {
    purging.value = false
  }
}

/** ---- 表格列（4A.2：原路径只读无动作；id 不渲染；类型两值内联域） ---- */

function trashTypeLabel(t: TrashEntryType): string {
  return TRASH_ENTRY_TYPE[t]
}

const columns = computed<DataTableColumns<TrashItem>>(() => {
  const base: DataTableColumns<TrashItem> = [
    {
      title: '原路径',
      key: 'originalPath',
      width: 360,
      fixed: 'left',
      // 只读展示、点击无动作——原路径当前存在与否未知，不做导航跳转（4A.2）
      render: (row) => {
        const icon = entryIcon('', row.entryType)
        return h(
          'span',
          { style: { display: 'inline-flex', alignItems: 'center', gap: '6px', minWidth: 0 } },
          [
            h(NIcon, { component: icon.icon, color: icon.color, size: 16 }),
            h('span', { style: { overflow: 'hidden', textOverflow: 'ellipsis' } }, row.originalPath)
          ]
        )
      }
    },
    {
      title: '类型',
      key: 'entryType',
      width: 100,
      render: (row) => trashTypeLabel(row.entryType)
    },
    {
      title: '大小',
      key: 'size',
      width: 120,
      align: 'right',
      render: (row) => formatBytes(row.size) // 删除入站时快照（目录为累计值，api 4.1.1）
    },
    { title: '删除人', key: 'operatorName', width: 140 },
    { title: '删除时间', key: 'deletedAt', width: 180, render: (row) => formatDateTime(row.deletedAt) }
  ]
  if (authStore.isAdmin) {
    base.push({
      title: '操作',
      key: 'actions',
      width: 120,
      fixed: 'right',
      render: (row) =>
        h(
          NButton,
          {
            size: 'small',
            secondary: true,
            disabled: store.isInFlight(row.id),
            onClick: () => onRestore(row)
          },
          { default: () => '还原' }
        )
    })
  }
  return base
})

/** readonly 无 fixed:right 操作列（负空间；原路径列仍 fixed:left） */
const scrollX = computed(() => (authStore.isAdmin ? 1020 : 900))

function rowKey(row: TrashItem): number {
  return row.id
}

const totalText = computed(() => `共 ${store.total} 条`)

const purgeAlertText = computed(
  () =>
    `清空回收站为终局销毁：全部 ${store.total} 条条目将被永久删除，不可还原、不可恢复。确认清空？`
)

/** 进入页面拉取（trash 页同构刷新纪律）；离开清理在途态 */
onMounted(() => {
  void store.loadList()
})

onUnmounted(() => {
  store.resetView()
})
</script>

<template>
  <div class="page">
    <n-card size="small" title="回收站">
      <template #header-extra>
        <span class="meta">{{ totalText }}</span>
      </template>
      <div class="toolbar">
        <n-button size="small" :loading="store.listLoading" @click="store.loadList()">刷新</n-button>
        <n-button
          v-if="authStore.isAdmin"
          size="small"
          type="error"
          ghost
          :disabled="purgeDisabled"
          @click="purgeConfirmShow = true"
        >
          清空
        </n-button>
      </div>
      <!-- 孤儿对账剔除事实（data.warning，A-F11/PRD N2——服务端完整文案直接呈现，W1 修复） -->
      <n-alert v-if="store.listWarning !== null" type="warning" :bordered="false" class="warning">
        {{ store.listWarning }}
      </n-alert>
      <n-data-table
        class="table"
        :columns="columns"
        :data="store.items"
        :loading="store.listLoading"
        :scroll-x="scrollX"
        :row-key="rowKey"
      >
        <template #empty>
          <n-empty description="回收站为空" class="empty" />
        </template>
      </n-data-table>
    </n-card>

    <!-- 清空强警告（4A.1：终局销毁——红字「不可还原」+ 条数明示；取消不请求） -->
    <n-modal
      v-model:show="purgeConfirmShow"
      preset="dialog"
      type="error"
      title="清空回收站"
      positive-text="确认清空"
      negative-text="取消"
      :loading="purging"
      @positive-click="onPurge"
    >
      <div class="purge-body">{{ purgeAlertText }}</div>
      <div class="purge-strong">此操作不可还原、不可恢复。</div>
    </n-modal>

    <!-- FILE_005 冲突决策（还原：覆盖全部/取消——无跳过，api D-07） -->
    <conflict-modal :payload="pendingConflict" @decide="onConflictDecide" @cancel="pendingConflict = null" />
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 12px;
}
.warning {
  margin-bottom: 12px;
}
.meta {
  font-size: 12px;
  opacity: 0.6;
}
.table {
  flex: 1;
}
.empty {
  padding: 32px 0;
}
.purge-body {
  font-size: 13px;
}
.purge-strong {
  margin-top: 8px;
  font-size: 13px;
  font-weight: 600;
  color: crimson;
}
</style>
