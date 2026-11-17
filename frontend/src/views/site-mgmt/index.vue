<script setup lang="ts">
/**
 * 站点管理列表页（PageSpec 4.1，hybrid；路由 /sites；admin+readonly 同路由按角色渲染子集）。
 * - 空态（D-01/D-15）：nginxAvailable=false → 空态引导卡（SITE_012 语义），隐藏工具栏与筛选区；
 * - duplicates[] 非空 → NAlert warning（D-03 接管不破坏口径）；
 * - 工具栏 admin：新建站点 / 重载 nginx（NPopconfirm；SITE_004 → output 弹窗）；
 *   readonly 无管理动作入口（负空间，ADR-018）；generatedAt「扫描于 HH:mm:ss」+ 刷新两角色可见；
 * - 列表为当次扫描投影（D-01 无缓存：刷新即重新 siteList）；写动作成功后重拉列表。
 */
import { computed, h, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useDialog, useMessage } from 'naive-ui'
import { NAlert, NButton, NEmpty, NPopconfirm, NTag, NTooltip } from 'naive-ui'
import type { DataTableColumns, SelectOption } from 'naive-ui'
import type { VNode } from 'vue'
import { nginxReload, siteDisable, siteEnable } from '@/api/site'
import { ApiError } from '@/api/http'
import { useAuthStore } from '@/stores/auth'
import { useSitesStore } from '@/stores/site'
import { CERT_STATUS, SITE_ENABLED, dictItem } from '@/types/enums'
import type { SiteEnabled } from '@/types/enums'
import type { SiteListItem, SiteRollbackData } from '@/types/site'
import { fieldOfString, isSiteRollbackData } from '@/types/site'
import { formatTime } from '@/utils/format'
import { siteActionError } from './errors'
import RollbackDiffModal from './components/RollbackDiffModal.vue'
import SiteCreateModal from './components/SiteCreateModal.vue'
import SiteDetailDrawer from './components/SiteDetailDrawer.vue'

const router = useRouter()
const authStore = useAuthStore()
const store = useSitesStore()
const message = useMessage()
const dialog = useDialog()

/** ---- 查询条件（api site/list 唯一真实参数 enabled——不虚构关键字搜索） ---- */

const enabledOptions: SelectOption[] = (Object.keys(SITE_ENABLED) as SiteEnabled[]).map((v) => ({
  label: SITE_ENABLED[v].label,
  value: v
}))

const enabledFilter = computed<SiteEnabled | null>({
  get: () => store.enabledFilter,
  set: (v) => store.setEnabledFilter(v)
})

/** ---- 详情 Drawer 态（页面宿主） ---- */

const drawerShow = ref(false)
const drawerSiteId = ref<string | null>(null)
const drawerInitialLog = ref(false)

function openDetail(row: SiteListItem, atLog = false): void {
  /** v1.2 聚合条目（D-17）：Drawer 定位组内首块（详情内可切换块） */
  drawerSiteId.value = row.blocks[0]?.siteId ?? null
  drawerInitialLog.value = atLog
  drawerShow.value = true
}

function openDetailBySiteId(siteId: string): void {
  drawerSiteId.value = siteId
  drawerInitialLog.value = false
  drawerShow.value = true
}

/** ---- SITE_003 / SITE_004 共用分流 ---- */

const rollback = ref<SiteRollbackData | null>(null)
const reloading = ref(false)

function handleWriteError(e: unknown): void {
  if (!(e instanceof ApiError)) return
  if (e.is('SITE_002')) {
    message.error('站点不存在（可能已被面板外改动）')
    void store.loadList()
    return
  }
  if (e.is('SITE_003')) {
    if (isSiteRollbackData(e.data)) {
      rollback.value = e.data
      return
    }
  }
  const info = siteActionError(e)
  if (info === null) return
  if (info.output !== undefined) {
    dialog.error({
      title: info.text,
      content: `${info.output}\n（配置已保留，请检查环境后重试）`
    })
  } else {
    message.error(info.text)
  }
}

async function onToggle(row: SiteListItem, target: boolean): Promise<void> {
  /** v1.2 组级启停（D-17）：对 blocks 顺序逐块调用（单块条目=原语义）；
   * 任一失败如实提示并停止后续（前端在途防重入，幂等语义由服务端保证）。 */
  try {
    for (const b of row.blocks) {
      if (b.enabled === (target ? 'enabled' : 'disabled')) continue // 已处目标态跳过
      if (target) await siteEnable(b.siteId)
      else await siteDisable(b.siteId)
    }
    message.success(
      target ? '站点已启用并生效' : '站点已停用（配置与证书引用已保留，停用≠删除）'
    )
    void store.loadList() // 投影实况对齐（D-01）
  } catch (e) {
    handleWriteError(e)
    void store.loadList() // 部分块已生效——如实刷新呈现
  }
}

/** 工具栏：重载 nginx（admin；SITE_004 → output 弹窗，配置已保留语义 D-05） */
async function onReloadNginx(): Promise<void> {
  reloading.value = true
  try {
    await nginxReload()
    message.success('nginx 已平滑重载')
  } catch (e) {
    if (e instanceof ApiError && e.is('SITE_004')) {
      const output = fieldOfString(e.data, 'output') ?? '（无输出）'
      dialog.error({
        title: '重载失败：配置已保留',
        content: `${output}\n（nginx -t 已通过，重载失败多为环境性原因，请检查环境后重试）`
      })
      return
    }
    handleWriteError(e)
  } finally {
    reloading.value = false
  }
}

/** ---- 表格列（4.1 列顺序；readonly 无操作列——operList 为空则不渲染 action 列） ---- */

function certCell(row: SiteListItem): VNode {
  // 证书列：null=「无引用」gray；有引用未纳管=「未纳管」orange（certId 缺失，D-11 推导）
  const certRef = row.certRef
  const certStatus = row.certStatus
  const inner: VNode =
    certRef === null
      ? h(NTag, { size: 'small', bordered: false }, { default: () => '无引用' })
      : certStatus !== undefined && row.certId !== undefined
        ? h(
            NTag,
            { type: dictItem(CERT_STATUS, certStatus).tag, size: 'small', bordered: false },
            { default: () => dictItem(CERT_STATUS, certStatus).label }
          )
        : h(
            NTag,
            { type: 'warning', size: 'small', bordered: false },
            { default: () => '未纳管' }
          )
  if (certRef !== null) {
    return h(NTooltip, null, {
      trigger: () => inner,
      default: () => h('span', { class: 'cell-mono' }, certRef)
    })
  }
  return inner
}

const columns = computed<DataTableColumns<SiteListItem>>(() => {
  const base: DataTableColumns<SiteListItem> = [
    /** v1.2 聚合条目行展开（D-17）：多块条目展开呈现块级清单（siteId/listen/enabled/
     * 证书引用）；单块条目展开图标同样可用（结构统一）。 */
    {
      type: 'expand',
      renderExpand: (row) =>
        h(
          'div',
          { class: 'block-list' },
          row.blocks.map((b) =>
            h('div', { class: 'block-row' }, [
              h('span', { class: 'block-mono' }, b.siteId),
              h('span', null, `监听 ${b.listen.join(' / ')}`),
              h('span', null, b.enabled === 'enabled' ? '启用' : '停用'),
              h('span', null, b.certRef ?? '无证书引用')
            ])
          )
        )
    },
    { title: '序号', key: 'index', width: 60, render: (_row, index) => index + 1 },
    {
      title: '主域名',
      key: 'serverNames',
      width: 200,
      fixed: 'left',
      render: (row) =>
        h(
          NButton,
          { text: true, type: 'primary', onClick: () => openDetail(row) },
          { default: () => row.serverNames[0] ?? row.sourceFile }
        )
    },
    {
      title: '全部域名',
      key: 'allNames',
      width: 200,
      render: (row) => {
        const text = row.serverNames.join(', ')
        if (row.serverNames.length <= 1) return text
        return h(NTooltip, null, { trigger: () => h('span', null, text), default: () => text })
      }
    },
    {
      title: '监听',
      key: 'listen',
      width: 140,
      /** v1.2 聚合合并呈现（D-17：如「80 / 443 ssl」；块级明细经行展开） */
      render: (row) => row.listen.join(' / ')
    },
    {
      title: '根目录',
      key: 'root',
      width: 180,
      ellipsis: { tooltip: true },
      render: (row) => row.root
    },
    { title: '证书', key: 'cert', width: 120, render: (row) => certCell(row) },
    {
      title: '反代上游',
      key: 'upstream',
      width: 180,
      ellipsis: { tooltip: true },
      render: (row) => row.proxy?.upstream ?? '—'
    },
    {
      title: '启停',
      key: 'enabled',
      width: 90,
      render: (row) =>
        h(
          NTag,
          { type: dictItem(SITE_ENABLED, row.enabled).tag, size: 'small', bordered: false },
          { default: () => dictItem(SITE_ENABLED, row.enabled).label }
        )
    },
    {
      title: '来源文件',
      key: 'sourceFile',
      width: 200,
      ellipsis: { tooltip: true },
      render: (row) => row.sourceFile
    }
  ]
  if (authStore.isAdmin) {
    base.push({
      title: '操作',
      key: 'actions',
      width: 200,
      fixed: 'right',
      render: (row) => {
        const buttons: VNode[] = []
        // 启用/停用按行当前 enabled 二选一渲染
        buttons.push(
          row.enabled === 'enabled'
            ? h(
                NPopconfirm,
                { onPositiveClick: () => void onToggle(row, false) },
                {
                  trigger: () =>
                    h(
                      NButton,
                      { size: 'small', type: 'warning', ghost: true },
                      { default: () => '停用' }
                    ),
                  default: () => '确认停用？保留配置与证书引用，停用≠删除。'
                }
              )
            : h(
                NPopconfirm,
                { onPositiveClick: () => void onToggle(row, true) },
                {
                  trigger: () =>
                    h(NButton, { size: 'small', type: 'primary', ghost: true }, { default: () => '启用' }),
                  default: () => '确认启用该站点？将还原 server 块并重载生效。'
                }
              )
        )
        buttons.push(
          h(
            NButton,
            {
              size: 'small',
              onClick: () => void router.push(`/sites/${encodeURIComponent(row.blocks[0]?.siteId ?? '')}/source`)
            },
            { default: () => '源码编辑' }
          )
        )
        buttons.push(
          h(NButton, { size: 'small', onClick: () => openDetail(row, true) }, { default: () => '日志' })
        )
        return buttons
      }
    })
  }
  return base
})

/** readonly 视角无 fixed:right 操作列（负空间） */
const scrollX = computed(() => (authStore.isAdmin ? 1550 : 1350))

function rowKey(row: SiteListItem): string {
  /** v1.2 聚合键（D-17）：sourceFile+serverNames 全集稳定 */
  return `${row.sourceFile}#${row.serverNames.join(',')}`
}

const createShow = ref(false)

/** nginx 可用性空态文案：SITE_012 data.reason 如实呈现（不虚构列表） */
const nginxUnavailableReason = computed(() =>
  !store.nginxAvailable ? 'nginx 未检出或 nginx -t 通道不可用（SITE_012）。' : ''
)

onMounted(() => {
  void store.loadList()
})
</script>

<template>
  <div class="page">
    <!-- 空态引导卡（SITE_012 语义；隐藏工具栏与筛选区，R-15 发现哲学） -->
    <n-card v-if="store.loaded && !store.nginxAvailable" size="small">
      <n-empty description="未检出 nginx，无可接管站点" class="empty">
        <template #extra>
          <div class="empty-hint">
            {{ nginxUnavailableReason }}
            在本机安装并启动 nginx 后，重新进入本页会自动识别已有站点。
          </div>
        </template>
      </n-empty>
    </n-card>

    <template v-else>
      <!-- 查询区 + 管理工具栏（readonly 无管理动作入口；刷新与扫描时刻两角色可见） -->
      <n-card size="small">
        <div class="controls-row">
          <n-select
            v-model:value="enabledFilter"
            clearable
            :options="enabledOptions"
            class="ctrl-enabled"
            placeholder="启停状态（空=全部）"
          />
          <n-button type="primary" :loading="store.listLoading" @click="store.loadList()">刷新</n-button>
          <div class="spacer"></div>
          <n-button v-if="authStore.isAdmin" type="primary" @click="createShow = true">
            新建站点
          </n-button>
          <n-popconfirm v-if="authStore.isAdmin" @positive-click="onReloadNginx">
            <template #trigger>
              <n-button :loading="reloading">重载 nginx</n-button>
            </template>
            将平滑重载 nginx 使配置生效。
          </n-popconfirm>
        </div>
      </n-card>

      <!-- duplicates[]：同域名多文件重复定义提醒（D-03，接管不破坏） -->
      <n-card v-if="store.duplicates.length > 0" size="small">
        <n-alert type="warning" title="检测到同域名多文件重复定义" :show-icon="true">
          <div v-for="(dup, i) in store.duplicates" :key="i" class="dup-row">
            {{ dup.serverNames.join(', ') }}（{{ dup.listen.join(', ') }}）：
            <span class="cell-mono">{{ dup.files.join('、') }}</span>
            —— 面板已按消重优先级（sites-enabled &gt; conf.d &gt; vhost.d）登记写回落点，重复定义未被修改。
          </div>
        </n-alert>
      </n-card>

      <n-card size="small" title="站点列表">
        <template #header-extra>
          <span class="meta"> 共 {{ store.items.length }} 个站点 · 扫描于 {{ formatTime(store.generatedAt) }} </span>
        </template>
        <n-data-table
          :columns="columns"
          :data="store.items"
          :loading="store.listLoading"
          :scroll-x="scrollX"
          :row-key="rowKey"
        >
          <template #empty>
            <n-empty v-if="store.loaded && store.items.length === 0" description="未匹配到站点" class="empty" />
          </template>
        </n-data-table>
      </n-card>
    </template>

    <!-- 详情 Drawer（6 Card；admin 编辑态 / readonly 全只读 + 日志 Tab） -->
    <site-detail-drawer
      v-model:show="drawerShow"
      :site-id="drawerSiteId"
      :initial-log="drawerInitialLog"
      @list-changed="store.loadList()"
    />

    <!-- 新建站点弹窗（admin） -->
    <site-create-modal
      v-model:show="createShow"
      @created="store.loadList()"
      @view-site="openDetailBySiteId"
    />

    <!-- SITE_003 回滚 diff 弹窗（列表写动作共用） -->
    <rollback-diff-modal :data="rollback" @close="rollback = null" />
  </div>
</template>

<style scoped>
.block-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 4px 12px;
  font-size: 12px;
}
.block-row {
  display: flex;
  gap: 16px;
  align-items: baseline;
}
.block-mono {
  font-family: monospace;
  opacity: 0.75;
}
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
.ctrl-enabled {
  width: 200px;
}
.spacer {
  flex: 1;
}
.meta {
  font-size: 12px;
  opacity: 0.6;
}
.dup-row {
  line-height: 1.8;
  word-break: break-all;
}
.cell-mono {
  font-family: Consolas, Menlo, monospace;
  font-size: 12px;
}
.empty {
  padding: 32px 0;
}
.empty-hint {
  font-size: 12px;
  opacity: 0.65;
  max-width: 480px;
  line-height: 1.8;
}
</style>
