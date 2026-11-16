<script setup lang="ts">
/**
 * 实例清单页 db-list（PageSpec 4.1，路由 /databases；admin+readonly 同路由组件内子集渲染）。
 * readonly 负空间（readonly 文档 4.1）：无工具栏（无重新探测）、操作列仅「查看」；
 * 查询条件/表格列与 admin 完全一致（ADR-018 全可看，凭据列仅布尔标注无任何凭据值）。
 * 行点击/查看 → 按引擎进入治理页（mysql/mariadb→mysql-manage、redis→redis-manage）。
 */
import { computed, h, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useMessage } from 'naive-ui'
import { NButton, NPopconfirm } from 'naive-ui'
import type { DataTableColumns, SelectOption } from 'naive-ui'
import { instanceRemove, instanceRescan } from '@/api/database'
import { ApiError } from '@/api/http'
import { useAuthStore } from '@/stores/auth'
import { useDbStore } from '@/stores/database'
import { DB_ENGINE } from '@/types/enums'
import type { DbEngine } from '@/types/enums'
import type { DbInstanceItem } from '@/types/database'
import { dbmErrorText } from '@/types/database'
import { formatDateTime } from '@/utils/format'
import InstanceCredentialsModal from './components/InstanceCredentialsModal.vue'
import { connModeTag, connStateTag, credentialsTag, engineTag, identityText } from './render'

const router = useRouter()
const message = useMessage()
const authStore = useAuthStore()
const store = useDbStore()

/** ---- 查询条件（api 4.1.4 instance/list query：engine eq 筛选，含「全部」） ---- */

const engineOptions: SelectOption[] = [
  ...(Object.keys(DB_ENGINE) as DbEngine[]).map((v) => ({
    label: DB_ENGINE[v].label,
    value: v
  }))
]

function onEngineFilter(v: DbEngine | null): void {
  store.setEngineFilter(v)
}

/** ---- 工具栏（admin only）：重新探测（PageSpec 4.1：成功 toast 新增/更新计数） ---- */

const rescanLoading = ref(false)

async function onRescan(): Promise<void> {
  rescanLoading.value = true
  try {
    const data = await instanceRescan()
    message.success(`重新探测完成：新增 ${data.discovered} 个实例，更新 ${data.updated} 个状态`)
    void store.loadList()
  } catch (e) {
    toastDbm(e)
  } finally {
    rescanLoading.value = false
  }
}

/** ---- 错误分流（PageSpec 第 1 节：DBM 码页面级提示——拦截器不含 DBM 码集） ---- */

function toastDbm(e: unknown): void {
  if (!(e instanceof ApiError)) return
  const text = dbmErrorText(e.code, e.data)
  if (text !== null) message.error(text)
  // 其余错误（网络/AUTH 族）已由拦截器统一提示
}

/** ---- 凭据弹窗（5.1；DBM_004 未配置行主按钮态引导） ---- */

const credentialsTarget = ref<DbInstanceItem | null>(null)
const credentialsShow = ref(false)

function openCredentials(row: DbInstanceItem): void {
  credentialsTarget.value = row
  credentialsShow.value = true
}

/** ---- 移除纳管（5.8：⚠️A20 仅解除面板管理，不卸载实例、不删除数据） ---- */

const removingId = ref<number | null>(null)

async function onRemove(row: DbInstanceItem): Promise<void> {
  removingId.value = row.id
  try {
    await instanceRemove(row.id)
    message.success('已解除纳管（实例与数据不受影响）')
    void store.loadList()
  } catch (e) {
    toastDbm(e) // DBM_016 在途 → 提示稍后重试（第 1 节）
  } finally {
    removingId.value = null
  }
}

/** ---- 行点击进入治理页（按引擎路由；readonly 同入口，治理页内再按角色渲染子集） ---- */

function navigateToManage(row: DbInstanceItem): void {
  void router.push(
    row.engine === 'redis'
      ? `/databases/redis/${row.id}`
      : `/databases/mysql/${row.id}`
  )
}

function stopAnd(fn: () => void): (e: MouseEvent) => void {
  return (e: MouseEvent) => {
    e.stopPropagation() // 行点击与行内按钮互不串扰
    fn()
  }
}

/** ---- 表格列（PageSpec 4.1 列定义；readonly 操作列仅「查看」） ---- */

const columns = computed<DataTableColumns<DbInstanceItem>>(() => {
  const base: DataTableColumns<DbInstanceItem> = [
    { title: '序号', key: 'index', width: 60, render: (_row, index) => index + 1 },
    { title: '引擎', key: 'engine', width: 110, render: (row) => engineTag(row.engine) },
    {
      title: '实例标识',
      key: 'identity',
      width: 240,
      fixed: 'left',
      render: (row) =>
        h(
          NButton,
          {
            text: true,
            type: 'primary',
            onClick: stopAnd(() => navigateToManage(row))
          },
          { default: () => identityText(row) }
        )
    },
    { title: '连接方式', key: 'connMode', width: 100, render: (row) => connModeTag(row.connMode) },
    {
      title: '版本',
      key: 'version',
      width: 120,
      render: (row) => row.version ?? '—' // 探测初期如实为空（api 4.1.1）
    },
    { title: '连接状态', key: 'connState', width: 110, render: (row) => connStateTag(row.connState) },
    {
      title: '凭据',
      key: 'credentialsSet',
      width: 100,
      render: (row) => credentialsTag(row.credentialsSet)
    },
    {
      title: '发现时间',
      key: 'discoveredAt',
      width: 180,
      render: (row) => formatDateTime(row.discoveredAt)
    }
  ]
  if (authStore.isAdmin) {
    base.push({
      title: '操作',
      key: 'actions',
      width: 240,
      fixed: 'right',
      render: (row) => {
        const removing = removingId.value === row.id
        return [
          // credentialsSet=false 时为主按钮态（未配置行引导配置，PageSpec 4.1）
          h(
            NButton,
            {
              size: 'small',
              type: row.credentialsSet ? 'default' : 'primary',
              disabled: removing,
              onClick: stopAnd(() => openCredentials(row))
            },
            { default: () => '配置凭据' }
          ),
          h(
            NPopconfirm,
            { onPositiveClick: () => void onRemove(row) },
            {
              trigger: () =>
                h(
                  NButton,
                  {
                    size: 'small',
                    type: 'error',
                    ghost: true,
                    disabled: removing,
                    loading: removing,
                    onClick: stopAnd(() => undefined)
                  },
                  { default: () => '移除' }
                ),
              default: () =>
                '确认移除纳管？仅解除面板管理，不卸载实例、不删除数据。'
            }
          ),
          h(
            NButton,
            {
              size: 'small',
              onClick: stopAnd(() => navigateToManage(row))
            },
            { default: () => '查看' }
          )
        ]
      }
    })
  } else {
    base.push({
      title: '操作',
      key: 'actions',
      width: 100,
      fixed: 'right',
      render: (row) =>
        h(
          NButton,
          { size: 'small', onClick: stopAnd(() => navigateToManage(row)) },
          { default: () => '查看' }
        )
    })
  }
  return base
})

const scrollX = computed(() => (authStore.isAdmin ? 1260 : 1120))

function rowKey(row: DbInstanceItem): number {
  return row.id
}

function rowProps(row: DbInstanceItem): { onClick: () => void } {
  return { onClick: () => navigateToManage(row) }
}

onMounted(() => {
  void store.loadList()
})
</script>

<template>
  <div class="page">
    <!-- 查询卡片（引擎筛选，两角色一致） -->
    <n-card size="small">
      <div class="controls-row">
        <n-select
          :value="store.engineFilter"
          clearable
          :options="engineOptions"
          class="ctrl-engine"
          placeholder="引擎（空=全部）"
          @update:value="onEngineFilter"
        />
        <!-- 工具栏（admin only）：重新探测/刷新；readonly 无工具栏（readonly 文档 4.1：进入页面即拉取） -->
        <template v-if="authStore.isAdmin">
          <n-button :loading="rescanLoading" @click="onRescan">重新探测</n-button>
          <n-button :loading="store.listLoading" @click="store.loadList()">刷新</n-button>
        </template>
      </div>
    </n-card>

    <n-card size="small" title="实例清单">
      <template #header-extra>
        <span class="meta">
          共 {{ store.items.length }} 个实例 · 数据生成于 {{ formatDateTime(store.generatedAt) }}
        </span>
      </template>
      <n-data-table
        :columns="columns"
        :data="store.items"
        :loading="store.listLoading"
        :scroll-x="scrollX"
        :row-key="rowKey"
        :row-props="rowProps"
      >
        <!-- 空态（R-15 发现哲学：未发现任何实例；admin 可手动重新探测） -->
        <template #empty>
          <n-empty description="未发现 MySQL/MariaDB/Redis 实例" class="empty">
            <template v-if="authStore.isAdmin" #extra>
              <n-button size="small" :loading="rescanLoading" @click="onRescan">
                手动重新探测
              </n-button>
            </template>
          </n-empty>
        </template>
      </n-data-table>
    </n-card>

    <!-- 配置凭据弹窗（5.1） -->
    <instance-credentials-modal
      v-model:show="credentialsShow"
      :instance="credentialsTarget"
      @saved="store.applyCredentials"
    />
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.controls-row {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.ctrl-engine {
  width: 200px;
}
.meta {
  font-size: 12px;
  opacity: 0.6;
}
.empty {
  padding: 24px 0;
}
</style>
