<script setup lang="ts">
import { computed, h, onMounted, onUnmounted, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { NButton, NPopconfirm, NSwitch, NTag } from 'naive-ui'
import type { DataTableColumns, SelectOption } from 'naive-ui'
import type { VNode } from 'vue'
import {
  unitDisable,
  unitEnable,
  unitRestart,
  unitStart,
  unitStop
} from '@/api/service'
import { useAuthStore } from '@/stores/auth'
import { useServiceStore } from '@/stores/service'
import {
  SERVICE_ACTIVE_STATE,
  SERVICE_ENABLED_STATE,
  UNIT_TYPE,
  dictItem
} from '@/types/enums'
import type { DictItem, ServiceActiveState } from '@/types/enums'
import type { ServiceUnitFailResult, ServiceUnitItem } from '@/types/service'
import { formatDateTime } from '@/utils/format'
import { routeActionError } from './actionError'
import UnitDetailDrawer from './components/UnitDetailDrawer.vue'

/**
 * 服务管理列表页（PageSpec 第 4 节，路由 /services；admin 全功能）。
 * readonly 子集（ADR-018 负空间，组件内按角色渲染）：操作列整列不渲染、
 * 自启列恒 NTag 无开关；工具条/Drawer 与 admin 全等（全可看）。
 * 刷新纪律（D-03）：进入页面/参数变更/刷新按钮拉取，动作成功以响应快照直更不重拉，无轮询。
 */
const store = useServiceStore()
const authStore = useAuthStore()
const message = useMessage()

type UnitAction = 'start' | 'stop' | 'restart'

/** 动作成功文案（PageSpec 4.3：message.success「已启动/已停止/已重启」） */
const ACTION_DONE_TEXT: Readonly<Record<UnitAction, string>> = {
  start: '已启动',
  stop: '已停止',
  restart: '已重启'
}

/** ---- 工具条（4.1：参数变更即触发 unitList） ---- */

/** 状态过滤下拉（字典单一事实源 enums.ts；多选值由 store 拼接逗号 query） */
const stateOptions: SelectOption[] = (
  Object.keys(SERVICE_ACTIVE_STATE) as ServiceActiveState[]
).map((value) => ({ label: SERVICE_ACTIVE_STATE[value].label, value }))

const stateFilter = ref<ServiceActiveState[]>([])
const searchTerm = ref('')

/** 名称搜索防抖 300ms（PageSpec 4.1；定时器卸载配对清理） */
const SEARCH_DEBOUNCE_MS = 300
let searchTimer: ReturnType<typeof setTimeout> | null = null

watch(stateFilter, (v) => {
  store.setStateFilter(v)
})

watch(searchTerm, (v) => {
  if (searchTimer !== null) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    searchTimer = null
    store.setSearch(v)
  }, SEARCH_DEBOUNCE_MS)
})

/** ---- 列表元信息（表格上方：共 N 个 unit + generatedAt 本地时区） ---- */

const generatedAtText = computed(() => formatDateTime(store.generatedAt))

/** ---- SVC_007 结果框（4.3：失败原因 + 实际终态，NModal） ---- */

const failResult = ref<ServiceUnitFailResult | null>(null)
const failStateDict = computed<DictItem | null>(() => {
  if (failResult.value === null) return null
  const state = failResult.value.activeState
  return state === null ? null : dictItem(SERVICE_ACTIVE_STATE, state)
})

/** ---- 动作与错误分流（4.3 在途互斥与终态反馈 + 第 1 节错误处理） ---- */

/** 八分支分流决策在 ./actionError（纯函数，W1 修复轮抽取可测）；此处仅按 route.kind 落地副作用 */
function handleActionError(e: unknown): void {
  const route = routeActionError(e)
  switch (route.kind) {
    case 'fail-result':
      failResult.value = route.data
      // 行数据以响应实际终态直更；activeState=null（调用期拒绝无终态，I1）无可直更
      if (route.data.activeState !== null) {
        store.applyActiveStateRaw(route.data.name, route.data.activeState)
      }
      return
    case 'warn':
      // SVC_006：等待已达服务端 90s 上限，行 inFlight 由 finally 解除
      message.warning(route.text)
      return
    case 'warn-reload':
      // SVC_003：集合实况同步（api I-03）
      message.warning(route.text)
      void store.loadList()
      return
    case 'unavailable':
      // SVC_008：页面级横幅 + 不显示过期数据（I-01）
      store.markListUnavailable()
      return
    case 'error':
      message.error(route.text)
      return
    case 'silent':
      return
  }
}

async function onOperate(row: ServiceUnitItem, action: UnitAction): Promise<void> {
  const name = row.name
  store.setInFlight(name) // 该行全部操作按钮与自启开关置 loading/禁用
  try {
    const data =
      action === 'start'
        ? await unitStart(name)
        : action === 'stop'
          ? await unitStop(name)
          : await unitRestart(name)
    message.success(ACTION_DONE_TEXT[action])
    store.applySnapshot({
      name: data.name,
      activeState: data.activeState,
      subState: data.subState,
      enabledState: data.enabledState
    })
  } catch (e) {
    handleActionError(e)
  } finally {
    store.releaseInFlight(name)
  }
}

/** 自启切换（4.2 列定义）：开关语义即操作语义，无需确认弹层；NSwitch 回弹到响应值（不信任乐观更新） */
async function onToggleAutostart(row: ServiceUnitItem, targetEnabled: boolean): Promise<void> {
  const name = row.name
  store.setInFlight(name)
  try {
    const data = targetEnabled ? await unitEnable(name) : await unitDisable(name)
    message.success(targetEnabled ? '已启用开机自启' : '已关闭开机自启')
    store.applySnapshot({ name: data.name, enabledState: data.enabledState })
  } catch (e) {
    handleActionError(e)
  } finally {
    store.releaseInFlight(name)
  }
}

/** ---- 表格列（4.2 列定义；readonly 操作列整列不渲染） ---- */

function dictTag(map: Readonly<Record<string, DictItem>>, value: string): VNode {
  const item = dictItem(map, value) // 非两态（static 等）降级原值灰字 Tag——⚠️A14/D-05
  return h(NTag, { type: item.tag, size: 'small', bordered: false }, { default: () => item.label })
}

const columns = computed<DataTableColumns<ServiceUnitItem>>(() => {
  const base: DataTableColumns<ServiceUnitItem> = [
    { title: '序号', key: 'index', width: 60, render: (_row, index) => index + 1 },
    {
      title: '名称',
      key: 'name',
      width: 220,
      fixed: 'left',
      render: (row) =>
        h(
          NButton,
          { text: true, type: 'primary', onClick: () => store.openDrawer(row.name) },
          { default: () => row.name }
        )
    },
    {
      title: '类型',
      key: 'unitType',
      width: 100,
      render: (row) => dictItem(UNIT_TYPE, row.unitType).label
    },
    {
      title: '运行状态',
      key: 'activeState',
      width: 110,
      render: (row) => dictTag(SERVICE_ACTIVE_STATE, row.activeState)
    },
    {
      title: '状态细分',
      key: 'subState',
      width: 130,
      render: (row) => row.subState // 原值透传（D-04：不翻译不归类）
    },
    {
      title: '开机自启',
      key: 'enabledState',
      width: 130,
      render: (row) => {
        // canToggleAutostart 为服务端权威值（D-05/I-07）；readonly 恒 NTag（负空间，ADR-018）
        if (authStore.isAdmin && row.canToggleAutostart) {
          const inFlight = store.isInFlight(row.name)
          return h(NSwitch, {
            value: row.enabledState === 'enabled',
            size: 'small',
            loading: inFlight,
            disabled: inFlight,
            'onUpdate:value': (v: boolean) => void onToggleAutostart(row, v)
          })
        }
        return dictTag(SERVICE_ENABLED_STATE, row.enabledState)
      }
    }
  ]
  if (authStore.isAdmin) {
    base.push({
      title: '操作',
      key: 'actions',
      width: 220,
      fixed: 'right',
      render: (row) => {
        const inFlight = store.isInFlight(row.name)
        const buttons: VNode[] = []
        // 按钮可见性：canOperate 权威布尔 + activeState 呈现层优化（4.3；R-14 职责分离）
        if (row.canOperate && row.activeState !== 'active') {
          buttons.push(
            h(
              NButton,
              { size: 'small', disabled: inFlight, onClick: () => void onOperate(row, 'start') },
              { default: () => '启动' }
            )
          )
        }
        if (row.canOperate && row.activeState === 'active') {
          buttons.push(
            h(
              NPopconfirm,
              { onPositiveClick: () => void onOperate(row, 'stop') },
              {
                trigger: () =>
                  h(
                    NButton,
                    { size: 'small', type: 'error', ghost: true, disabled: inFlight },
                    { default: () => '停止' }
                  ),
                default: () => '确认停止该服务？运行中的服务将被中断。'
              }
            )
          )
        }
        if (row.canOperate) {
          buttons.push(
            h(
              NPopconfirm,
              { onPositiveClick: () => void onOperate(row, 'restart') },
              {
                trigger: () =>
                  h(
                    NButton,
                    { size: 'small', type: 'warning', ghost: true, disabled: inFlight },
                    { default: () => '重启' }
                  ),
                default: () => '确认重启该服务？服务将短暂中断。'
              }
            )
          )
        }
        // 属性：恒显（打开 Drawer；名称列 link 同入口）
        buttons.push(
          h(
            NButton,
            { size: 'small', disabled: inFlight, onClick: () => store.openDrawer(row.name) },
            { default: () => '属性' }
          )
        )
        return buttons
      }
    })
  }
  return base
})

/** readonly 视角无 fixed:right 操作列（负空间：fixed:right 列消失，名称列仍 fixed:left） */
const scrollX = computed(() => (authStore.isAdmin ? 970 : 750))

function rowKey(row: ServiceUnitItem): string {
  return row.name
}

/** ---- SVC_002 兜底提示（store 无 Message 上下文，页面 watch——monitoring 先例） ---- */

watch(
  () => store.listBizError,
  (msg) => {
    if (msg !== null) message.error(msg)
  }
)

/** 进入页面拉取列表（D-03 按需拉取；离开页面清理防抖定时器与 Drawer/inFlight 态） */
onMounted(() => {
  void store.loadList()
})

onUnmounted(() => {
  if (searchTimer !== null) {
    clearTimeout(searchTimer)
    searchTimer = null
  }
  store.resetView()
})
</script>

<template>
  <div class="page">
    <!-- 查询工具条（4.1：状态过滤多选 + 名称搜索防抖 300ms + 刷新 loading 防抖） -->
    <n-card size="small">
      <div class="controls-row">
        <n-select
          v-model:value="stateFilter"
          multiple
          clearable
          :options="stateOptions"
          class="ctrl-state"
          placeholder="运行状态（空=全部）"
        />
        <n-input v-model:value="searchTerm" clearable class="ctrl-search" placeholder="名称搜索" />
        <n-button type="primary" :loading="store.listLoading" @click="store.loadList()">
          刷新
        </n-button>
      </div>
    </n-card>

    <!-- SVC_008：页面级错误横幅 + 重试，表格区域不渲染（I-01 不显示过期数据） -->
    <n-card v-if="store.listUnavailable" size="small">
      <n-alert type="error" title="systemd 服务通道不可用，无法获取服务列表">
        <n-button size="small" @click="store.loadList()">重试</n-button>
      </n-alert>
    </n-card>

    <n-card v-else size="small" title="服务列表">
      <template #header-extra>
        <span class="meta">
          共 {{ store.total }} 个 unit · 数据生成于 {{ generatedAtText }}
        </span>
      </template>
      <!-- 全量返回不分页（D-02）；空结果 NEmpty（搜索无结果为常态非错误） -->
      <n-data-table
        :columns="columns"
        :data="store.units"
        :loading="store.listLoading"
        :scroll-x="scrollX"
        :row-key="rowKey"
      >
        <template #empty>
          <n-empty description="未匹配到服务" class="empty" />
        </template>
      </n-data-table>
    </n-card>

    <!-- 属性 Drawer（admin 与 readonly 全等：属性查看为只读能力） -->
    <unit-detail-drawer />

    <!-- SVC_007 结果框：操作失败 + 失败原因 + 实际终态 Tag（4.3） -->
    <n-modal
      :show="failResult !== null"
      preset="card"
      title="操作失败"
      style="width: 440px"
      :mask-closable="true"
      @update:show="
        (v: boolean) => {
          if (!v) failResult = null
        }
      "
    >
      <n-descriptions v-if="failResult !== null" :column="1" label-placement="left" bordered size="small">
        <n-descriptions-item label="服务">{{ failResult.name }}</n-descriptions-item>
        <n-descriptions-item label="失败原因">{{ failResult.reason }}</n-descriptions-item>
        <n-descriptions-item label="实际终态">
          <n-tag v-if="failStateDict !== null" :type="failStateDict.tag" size="small" :bordered="false">
            {{ failStateDict.label }}
          </n-tag>
          <!-- activeState=null（调用期拒绝无终态，I1）或字典未命中：空值展示「—」 -->
          <template v-else>—</template>
        </n-descriptions-item>
      </n-descriptions>
      <template #footer>
        <div class="modal-footer">
          <n-button @click="failResult = null">关闭</n-button>
        </div>
      </template>
    </n-modal>
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
.ctrl-state {
  width: 260px;
}
.ctrl-search {
  width: 240px;
}
.meta {
  font-size: 12px;
  opacity: 0.6;
}
.empty {
  padding: 24px 0;
}
.modal-footer {
  display: flex;
  justify-content: flex-end;
}
</style>
