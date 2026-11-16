<script setup lang="ts">
/**
 * 防火墙页（PageSpec 第 4 节，路由 /firewall；hybrid·Tab 布局）。
 * 结构：页头后端状态 Card（4.0，恒为第一区块，含刷新）→ backend=none 整页引导态（NEmpty，
 * 不渲染双 Tab 与任何规则/对照内容）| FW_010 页面级 NAlert（不显示过期数据）| 双 Tab
 * （4.1~4.4 端口规则 / 监听端口对照，懒加载）+ 添加规则 Dialog（第 5 节，admin 专属）。
 * readonly 子集（ADR-018 负空间）：工具栏无「添加规则」按钮、操作列整列不渲染；
 * 筛选/刷新/页头/引导态/对照高亮全部保留（全可看）。
 * 刷新纪律（D-01 按需拉取）：status 先行 / Tab 懒加载 / 写后重拉规则+对照标记过期 / 无轮询无定时器。
 */
import { computed, h, onMounted, onUnmounted, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { NButton, NPopconfirm, NTag } from 'naive-ui'
import type { DataTableColumns, SelectOption } from 'naive-ui'
import type { VNode } from 'vue'
import { ApiError } from '@/api/http'
import { ruleCreate, ruleDelete } from '@/api/firewall'
import { useAuthStore } from '@/stores/auth'
import { useFirewallStore } from '@/stores/firewall'
import {
  FIREWALL_BACKEND,
  FIREWALL_RULE_ACTION,
  FIREWALL_RULE_PROTOCOL,
  LISTEN_PORT_STATE,
  boolTag,
  dictItem
} from '@/types/enums'
import type { DictItem, FirewallRuleAction, FirewallRuleProtocol } from '@/types/enums'
import type {
  FirewallDangerInfo,
  FirewallPortItem,
  FirewallRuleCreateReq,
  FirewallRuleItem
} from '@/types/firewall'
import { FW_ERROR_TEXT } from '@/types/firewall'
import { formatDateTime } from '@/utils/format'
import AddRuleModal from './components/AddRuleModal.vue'

const store = useFirewallStore()
const authStore = useAuthStore()
const message = useMessage()

/** ---- Tab 框架（懒加载：进入端口对照 Tab 且未加载/已过期时拉取；规则 Tab 由进入页面先行拉取） ---- */

const activeTab = ref<'rules' | 'ports'>('rules')

watch(activeTab, (tab) => {
  if (tab === 'ports' && !store.noBackend && (!store.portLoaded || store.portStale)) {
    void store.loadPorts()
  }
})

/** ---- 页头（4.0 后端状态 Card） ---- */

const backendDict = computed<DictItem | null>(() =>
  store.backend === null ? null : dictItem(FIREWALL_BACKEND, store.backend.backend)
)
const activeDict = computed<DictItem | null>(() =>
  store.backend === null ? null : boolTag(store.backend.active, '运行中', '未启用')
)
const statusGeneratedAtText = computed(() => formatDateTime(store.backend?.generatedAt ?? null))

/** 动作支持面灰字说明（supportedActions 服务端权威面，D-01；firewalld 仅 allow 时串中自明） */
const supportedText = computed(() => {
  const acts = store.backend?.supportedActions ?? []
  if (acts.length === 0) return '—'
  return acts.map((a) => dictItem(FIREWALL_RULE_ACTION, a).label).join('，')
})

/** 页头刷新：重拉 backendStatus + 当前 Tab 清单（loading 防抖在 store） */
async function refreshAll(): Promise<void> {
  await store.loadStatus()
  if (store.backend === null || store.noBackend) return
  if (activeTab.value === 'ports') void store.loadPorts()
  else void store.loadRules()
}

/** ---- Tab 1 工具条（4.1 参数变更即触发 ruleList；readonly 无添加按钮——负空间） ---- */

const actionOptions: SelectOption[] = (Object.keys(FIREWALL_RULE_ACTION) as FirewallRuleAction[]).map(
  (v) => ({ label: FIREWALL_RULE_ACTION[v].label, value: v })
)
const protocolOptions: SelectOption[] = (
  Object.keys(FIREWALL_RULE_PROTOCOL) as FirewallRuleProtocol[]
).map((v) => ({ label: FIREWALL_RULE_PROTOCOL[v].label, value: v }))

const actionFilter = ref<FirewallRuleAction[]>([...store.listParams.action])
const protocolFilter = ref<FirewallRuleProtocol | null>(store.listParams.protocol)

watch(actionFilter, (v) => store.setActionFilter(v))
watch(protocolFilter, (v) => store.setProtocolFilter(v))

const ruleGeneratedAtText = computed(() => formatDateTime(store.ruleGeneratedAt))
const portGeneratedAtText = computed(() => formatDateTime(store.portGeneratedAt))

/** ---- 添加规则 Dialog（第 5 节，admin 专属；提交由本页编排以承载 FW_005/006/007/008 分流） ---- */

const createSubmitting = ref(false)
const createServerErrors = ref<{ port: string | null; source: string | null }>({
  port: null,
  source: null
})

function setCreateErrors(port: string | null, source: string | null): void {
  createServerErrors.value = { port, source }
}

function openAddDialog(): void {
  setCreateErrors(null, null)
  store.openAddDialog()
}

/** ---- FW_005 两段式危险确认（5.3：服务端权威判定；确认后原 req 追加 confirmDanger 重提交，不记忆） ---- */

type PendingDanger =
  | { kind: 'create'; req: Omit<FirewallRuleCreateReq, 'confirmDanger'> }
  | { kind: 'delete'; ruleKey: string }

const pendingDanger = ref<PendingDanger | null>(null)
const dangerInfo = ref<FirewallDangerInfo | null>(null)
const dangerBusy = ref(false)

/** FW_005 data 收窄（ApiError.data 为 unknown；panelPort×rulePort×action 命中详情） */
function isDangerInfo(d: unknown): d is FirewallDangerInfo {
  if (typeof d !== 'object' || d === null) return false
  const o = d as Record<string, unknown>
  return typeof o.panelPort === 'number' && typeof o.rulePort === 'string' && typeof o.action === 'string'
}

function openDanger(pending: PendingDanger, data: unknown): void {
  pendingDanger.value = pending
  dangerInfo.value = isDangerInfo(data) ? data : null
}

function closeDanger(): void {
  pendingDanger.value = null
  dangerInfo.value = null
}

/** 警示文案：data.hint（api FW_005）优先，缺失时用 PageSpec 5.3 固定文案 */
const dangerHintText = computed(
  () =>
    (dangerInfo.value?.hint ?? '') !== ''
      ? dangerInfo.value?.hint ?? ''
      : '此操作可能导致面板失去访问！确认执行前请确保保有服务器本地/SSH 救援通道。'
)
const dangerActionDict = computed<DictItem | null>(() =>
  dangerInfo.value === null ? null : dictItem(FIREWALL_RULE_ACTION, dangerInfo.value.action)
)

async function onDangerConfirm(): Promise<void> {
  const pending = pendingDanger.value
  if (pending === null) return
  dangerBusy.value = true
  try {
    if (pending.kind === 'create') await execCreate(pending.req, true)
    else await execDelete(pending.ruleKey, true)
  } finally {
    dangerBusy.value = false
    closeDanger() // 本次涉险提交已终结（成功/终态错误均由各自路径反馈）
  }
}

/** ---- FW_007 部分生效说明（5.4：如实反馈两层状态，不虚构全成功；关闭后重拉 ruleList） ---- */

const partialVisible = ref(false)

function closePartial(): void {
  partialVisible.value = false
  void store.loadRules()
}

/** ---- FW_008 data.reason 收窄 ---- */

function execReason(data: unknown): string {
  if (typeof data === 'object' && data !== null) {
    const reason = (data as Record<string, unknown>).reason
    if (typeof reason === 'string' && reason !== '') return reason
  }
  return '未知原因'
}

/** ---- 写动作（4.3 在途互斥：行级 inFlight + 添加按钮禁用；成功重拉规则+对照标记过期） ---- */

async function execCreate(
  req: Omit<FirewallRuleCreateReq, 'confirmDanger'>,
  confirmed: boolean
): Promise<void> {
  try {
    await ruleCreate({ ...req, confirmDanger: confirmed ? true : undefined })
    message.success('规则已添加')
    store.closeAddDialog()
    setCreateErrors(null, null)
    await store.loadRules()
    store.markPortsStale()
  } catch (e) {
    handleCreateError(e, req)
  }
}

function handleCreateError(e: unknown, req: Omit<FirewallRuleCreateReq, 'confirmDanger'>): void {
  if (!(e instanceof ApiError)) return
  if (e.is('FW_005')) {
    openDanger({ kind: 'create', req }, e.data)
    return
  }
  if (e.is('FW_006')) {
    // 等价规则已存在，不静默（D-07）；未发生变更 → 关闭 Dialog、不重拉
    message.warning(FW_ERROR_TEXT['FW_006'])
    store.closeAddDialog()
    return
  }
  if (e.is('FW_007')) {
    partialVisible.value = true
    store.closeAddDialog()
    store.markPortsStale()
    return
  }
  if (e.is('FW_008')) {
    message.error(`${FW_ERROR_TEXT['FW_008']}：${execReason(e.data)}`) // Dialog 保持打开供修正重试
    return
  }
  if (e.is('FW_001')) {
    setCreateErrors(FW_ERROR_TEXT['FW_001'], null) // Dialog 内字段错误（正常 UI 预校验不可达）
    return
  }
  if (e.is('FW_002')) {
    setCreateErrors(null, FW_ERROR_TEXT['FW_002'])
    return
  }
  if (e.is('FW_009')) {
    store.markNoBackend() // 兜底跳引导态（正常流程不可达，status 先行）
    store.closeAddDialog()
    return
  }
  if (e.is('FW_010')) {
    store.markUnavailable()
    store.closeAddDialog()
    return
  }
  const text = FW_ERROR_TEXT[e.code]
  if (text !== undefined) message.error(text) // FW_004 兜底（动作下拉仅列权威面，正常 UI 不可达）
}

async function onCreateSubmit(
  values: { port: string; protocol: FirewallRuleProtocol; source?: string; action: FirewallRuleAction }
): Promise<void> {
  createSubmitting.value = true
  try {
    await execCreate(values, false)
  } finally {
    createSubmitting.value = false
  }
}

async function onDelete(row: FirewallRuleItem): Promise<void> {
  await execDelete(row.ruleKey, false)
}

async function execDelete(ruleKey: string, confirmed: boolean): Promise<void> {
  store.setInFlight(ruleKey) // 该行按钮 loading/禁用 + 添加按钮禁用（防重复提交）
  try {
    await ruleDelete({ ruleKey, confirmDanger: confirmed ? true : undefined })
    message.success('规则已删除')
    await store.loadRules()
    store.markPortsStale()
  } catch (e) {
    handleDeleteError(e, ruleKey)
  } finally {
    store.releaseInFlight(ruleKey)
  }
}

function handleDeleteError(e: unknown, ruleKey: string): void {
  if (!(e instanceof ApiError)) return
  if (e.is('FW_005')) {
    openDanger({ kind: 'delete', ruleKey }, e.data)
    return
  }
  if (e.is('FW_003')) {
    message.warning(FW_ERROR_TEXT['FW_003'])
    void store.loadRules() // 实况同步（可能已在面板外删除，api D-07）
    return
  }
  if (e.is('FW_007')) {
    partialVisible.value = true
    store.markPortsStale()
    return
  }
  if (e.is('FW_008')) {
    message.error(`${FW_ERROR_TEXT['FW_008']}：${execReason(e.data)}`)
    return
  }
  if (e.is('FW_009')) {
    store.markNoBackend()
    return
  }
  if (e.is('FW_010')) {
    store.markUnavailable()
    return
  }
  const text = FW_ERROR_TEXT[e.code]
  if (text !== undefined) message.error(text) // FW_001（ruleKey 非空）正常 UI 不可达兜底
}

/** ---- 表格列（4.2/4.4；readonly 操作列整列不渲染——负空间） ---- */

function dictTag(map: Readonly<Record<string, DictItem>>, value: string): VNode {
  const item = dictItem(map, value)
  return h(NTag, { type: item.tag, size: 'small', bordered: false }, { default: () => item.label })
}

/** 监听进程：null → 「—」；否则 `{name} ({pid})`（字段缺失不虚构，api 4.1.1） */
function processText(row: FirewallPortItem): string {
  const p = row.process
  if (p === null) return '—'
  if (p.name !== null && p.pid !== null) return `${p.name} (${p.pid})`
  if (p.name !== null) return p.name
  if (p.pid !== null) return String(p.pid)
  return '—'
}

const ruleColumns = computed<DataTableColumns<FirewallRuleItem>>(() => {
  const base: DataTableColumns<FirewallRuleItem> = [
    { title: '序号', key: 'index', width: 60, render: (_row, index) => index + 1 },
    { title: '端口', key: 'port', width: 140, render: (row) => row.port },
    {
      title: '协议',
      key: 'protocol',
      width: 90,
      render: (row) => dictItem(FIREWALL_RULE_PROTOCOL, row.protocol).label
    },
    { title: '方向', key: 'direction', width: 90, render: () => '入站' }, // direction 恒 "ingress" 常量列（api 4.1.1）
    {
      title: '来源限制',
      key: 'source',
      width: 170,
      render: (row) =>
        row.source === null ? h('span', { class: 'muted' }, '任意') : row.source // null=任意来源灰字
    },
    { title: '动作', key: 'action', width: 110, render: (row) => dictTag(FIREWALL_RULE_ACTION, row.action) },
    {
      title: '面板端口',
      key: 'isPanelPort',
      width: 100,
      render: (row) =>
        row.isPanelPort
          ? h(NTag, { type: 'warning', size: 'small', bordered: false }, { default: () => '面板端口' })
          : null // false → 空白（预警线索标注，权威判定在服务端 FW_005）
    },
    {
      title: '后端原始语义',
      key: 'rawSpec',
      width: 260,
      render: (row) => h('span', { class: 'mono' }, row.rawSpec) // 原值透传不翻译，等宽呈现
    }
  ]
  if (authStore.isAdmin) {
    base.push({
      title: '操作',
      key: 'actions',
      width: 120,
      fixed: 'right',
      render: (row) => {
        const inFlight = store.isInFlight(row.ruleKey)
        // isPanelPort 仅为预警线索前置（4.3）；权威兜底=服务端 FW_005 两段式（D-06/I-05）
        const confirmText = row.isPanelPort
          ? `该规则直接命中面板自身端口（${row.port}），删除后可能导致面板失去访问！`
          : '确认删除该规则？删除后即时生效。'
        return h(
          NPopconfirm,
          { onPositiveClick: () => void onDelete(row) },
          {
            trigger: () =>
              h(
                NButton,
                { size: 'small', type: 'error', ghost: true, loading: inFlight, disabled: inFlight },
                { default: () => '删除' }
              ),
            default: () => confirmText
          }
        )
      }
    })
  }
  return base
})

/** readonly 视角无 fixed:right 操作列（负空间：fixed:right 列消失） */
const ruleScrollX = computed(() => (authStore.isAdmin ? 1140 : 1020))

const portColumns: DataTableColumns<FirewallPortItem> = [
  { title: '序号', key: 'index', width: 60, render: (_row, index) => index + 1 },
  { title: '协议', key: 'proto', width: 100, render: (row) => row.proto }, // 原值透传（tcp/tcp6/udp/udp6 开放集合不翻译）
  { title: '监听地址', key: 'addr', width: 170, render: (row) => row.addr },
  { title: '端口', key: 'port', width: 100, render: (row) => row.port },
  { title: '监听进程', key: 'process', width: 200, render: processText },
  {
    title: '对外',
    key: 'external',
    width: 90,
    render: (row) =>
      h(
        NTag,
        { type: row.external ? 'info' : 'default', size: 'small', bordered: false },
        { default: () => (row.external ? '对外' : '仅本机') } // loopback 维度（D-04）
      )
  },
  { title: '开放状态', key: 'openState', width: 140, render: (row) => dictTag(LISTEN_PORT_STATE, row.openState) }
]

/** 风险敞口行高亮：openState=no_allow_rule ∧ external=true（4.4，PRD FR-04） */
function portRowClass(row: FirewallPortItem): string {
  return row.external && row.openState === 'no_allow_rule' ? 'risk-row' : ''
}

function ruleRowKey(row: FirewallRuleItem): string {
  return row.ruleKey
}

function portRowKey(row: FirewallPortItem): string {
  return `${row.proto}|${row.addr}|${row.port}`
}

/** ---- 生命周期（进入页面 status 先行；卸载清理 Dialog/inFlight/Tab 过期标记） ---- */

onMounted(() => {
  void store.loadStatus().then(() => {
    if (store.backend !== null && !store.noBackend) void store.loadRules()
  })
})

onUnmounted(() => {
  store.resetView()
})
</script>

<template>
  <div class="page">
    <!-- 4.0 页头：后端状态 Card（恒为第一区块；引导态下保留刷新=安装后重试入口） -->
    <n-card size="small" title="防火墙后端">
      <template #header-extra>
        <n-button size="small" :loading="store.statusLoading" @click="refreshAll()">刷新</n-button>
      </template>
      <n-spin v-if="store.statusLoading && store.backend === null" size="small" class="header-state" />
      <n-alert v-else-if="store.statusFailed" type="error" title="后端状态获取失败">
        <n-button size="small" @click="refreshAll()">重试</n-button>
      </n-alert>
      <template v-else-if="backendDict !== null && activeDict !== null && store.backend !== null">
        <div class="status-row">
          <span class="status-label">后端类型</span>
          <n-tag :type="backendDict.tag" size="small" :bordered="false">{{ backendDict.label }}</n-tag>
          <span class="status-label">运行状态</span>
          <n-tag :type="activeDict.tag" size="small" :bordered="false">{{ activeDict.label }}</n-tag>
          <span class="status-label">版本</span>
          <span class="meta">{{ store.backend.version ?? '—' }}</span>
          <span class="status-label">动作支持</span>
          <span class="meta">当前后端支持动作：{{ supportedText }}</span>
        </div>
        <div class="meta">数据生成于 {{ statusGeneratedAtText }}</div>
      </template>
    </n-card>

    <!-- 引导态（backend=none）：整页降级 NEmpty，不渲染双 Tab 与任何规则/对照内容（FL S-42-02） -->
    <n-card v-if="store.noBackend" size="small">
      <n-empty
        description="未检测到受支持的防火墙后端（ufw / firewalld）。请在服务器上安装并启用其一后刷新重试。"
        class="empty"
      />
    </n-card>

    <!-- FW_010：后端已识别但不可用——页面级 NAlert + 重试，不渲染过期数据（I-01） -->
    <n-card v-else-if="store.listUnavailable" size="small">
      <n-alert type="error" title="防火墙后端不可用（如 firewalld 服务停止）">
        <n-button size="small" @click="refreshAll()">重试</n-button>
      </n-alert>
    </n-card>

    <!-- 双 Tab：端口规则 / 监听端口对照 -->
    <n-card v-else size="small">
      <n-tabs v-model:value="activeTab" type="line">
        <!-- Tab 1 端口规则（4.1 工具条 + 4.2 表格） -->
        <n-tab-pane name="rules" tab="端口规则">
          <div class="controls-row">
            <n-select
              v-model:value="actionFilter"
              multiple
              clearable
              :options="actionOptions"
              class="ctrl-filter"
              placeholder="动作（空=全部）"
            />
            <n-select
              v-model:value="protocolFilter"
              clearable
              :options="protocolOptions"
              class="ctrl-filter"
              placeholder="协议（空=全部）"
            />
            <!-- 添加入口在工具栏（admin 专属；在途互斥期间禁用） -->
            <n-button
              v-if="authStore.isAdmin"
              type="primary"
              :disabled="store.hasInFlight"
              @click="openAddDialog()"
            >
              添加规则
            </n-button>
            <n-button :loading="store.ruleLoading" @click="store.loadRules()">刷新</n-button>
          </div>
          <div class="meta">数据生成于 {{ ruleGeneratedAtText }} · 共 {{ store.ruleTotal }} 条规则</div>
          <n-data-table
            :columns="ruleColumns"
            :data="store.ruleItems"
            :loading="store.ruleLoading"
            :scroll-x="ruleScrollX"
            :row-key="ruleRowKey"
          >
            <template #empty>
              <n-empty description="后端无端口规则" class="empty" />
            </template>
          </n-data-table>
        </n-tab-pane>

        <!-- Tab 2 监听端口对照（4.4：无筛选参数全量视图；风险敞口橙底高亮） -->
        <n-tab-pane name="ports" tab="监听端口对照">
          <div class="controls-row">
            <span class="meta">橙底行=对外监听且无放行规则</span>
            <n-button :loading="store.portLoading" @click="store.loadPorts()">刷新</n-button>
          </div>
          <div class="meta">数据生成于 {{ portGeneratedAtText }} · 共 {{ store.portTotal }} 个监听端口</div>
          <!-- readonly 处置线索（无处置入口，不虚构操作路径） -->
          <div v-if="!authStore.isAdmin" class="meta">如需处置，请联系管理员或经服务器本地操作</div>
          <n-data-table
            :columns="portColumns"
            :data="store.portItems"
            :loading="store.portLoading"
            :scroll-x="860"
            :row-key="portRowKey"
            :row-class-name="portRowClass"
          >
            <template #empty>
              <n-empty description="当前无监听端口" class="empty" />
            </template>
          </n-data-table>
        </n-tab-pane>
      </n-tabs>
    </n-card>

    <!-- 添加规则 Dialog（admin 专属；readonly 不渲染整组件——无入口即无表单形态） -->
    <add-rule-modal
      v-if="authStore.isAdmin"
      v-model:show="store.addDialogVisible"
      :supported-actions="store.backend?.supportedActions ?? []"
      :submitting="createSubmitting"
      :server-errors="createServerErrors"
      @submit="onCreateSubmit"
    />

    <!-- FW_005 危险确认 Modal（5.3：面板端口保护两段式——服务端权威判定触发） -->
    <n-modal
      :show="pendingDanger !== null"
      preset="card"
      title="危险操作：面板端口保护"
      style="width: 480px"
      :mask-closable="false"
    >
      <n-alert type="error">{{ dangerHintText }}</n-alert>
      <n-descriptions
        v-if="dangerInfo !== null"
        :column="1"
        label-placement="left"
        bordered
        size="small"
        class="danger-detail"
      >
        <n-descriptions-item label="面板端口">{{ dangerInfo.panelPort }}</n-descriptions-item>
        <n-descriptions-item label="规则端口">{{ dangerInfo.rulePort }}</n-descriptions-item>
        <n-descriptions-item label="动作">
          <n-tag
            v-if="dangerActionDict !== null"
            :type="dangerActionDict.tag"
            size="small"
            :bordered="false"
          >
            {{ dangerActionDict.label }}
          </n-tag>
        </n-descriptions-item>
      </n-descriptions>
      <template #footer>
        <div class="modal-footer">
          <n-button :disabled="dangerBusy" @click="closeDanger()">取消</n-button>
          <n-button type="error" :loading="dangerBusy" @click="onDangerConfirm()">确认执行</n-button>
        </div>
      </template>
    </n-modal>

    <!-- FW_007 部分生效说明（5.4：permanent 已写入/runtime 失败，不回滚不虚构；关闭后重拉 ruleList） -->
    <n-modal
      :show="partialVisible"
      preset="card"
      title="部分生效"
      style="width: 480px"
      :mask-closable="false"
      @update:show="
        (v: boolean) => {
          if (!v) closePartial()
        }
      "
    >
      <n-alert type="warning">永久配置已写入，运行态应用失败——规则将在防火墙服务重启后生效。</n-alert>
      <template #footer>
        <div class="modal-footer">
          <n-button @click="closePartial()">关闭</n-button>
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
.status-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.status-label {
  font-size: 12px;
  opacity: 0.6;
}
.header-state {
  display: inline-flex;
}
.controls-row {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 8px;
}
.ctrl-filter {
  width: 220px;
}
.meta {
  font-size: 12px;
  opacity: 0.6;
  margin-bottom: 8px;
}
.muted {
  opacity: 0.55;
}
.mono {
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', monospace;
  font-size: 12px;
}
.empty {
  padding: 24px 0;
}
.danger-detail {
  margin-top: 12px;
}
.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
/* 风险敞口行浅橙底高亮（PageSpec 4.4；rgba 语义色，随明暗主题叠加） */
:deep(.risk-row td) {
  background: rgba(240, 160, 32, 0.12);
}
</style>
