<script setup lang="ts">
/**
 * 证书管理列表页（PageSpec 4.2，config；路由 /certificates；admin+readonly 同路由按角色渲染子集）。
 * 查询条件仅 API 真实参数（source/status）；主域名 link → 行内只读详情 Modal（数据源=行数据，
 * api 无证书详情接口——不虚构请求）；expiring_soon 行整行橙色浅底；renew_failed 行状态附
 * lastError NTooltip（FR-24 排障呈现，readonly 保留）。
 * operList（admin）：部署/解除部署/续期重试（仅 renew_failed）/删除（NPopconfirm 强确认，
 * 文案区分来源 I-04/⚠️A16）；CERT_005 → NModal 引用站点清单 +「去解除部署」引导。
 */
import { computed, h, onMounted, onUnmounted, ref } from 'vue'
import { useDialog, useMessage } from 'naive-ui'
import { NButton, NModal, NPopconfirm, NTag, NTooltip } from 'naive-ui'
import type { DataTableColumns, SelectOption } from 'naive-ui'
import type { VNode } from 'vue'
import { certDelete, certRetryRenewal } from '@/api/site'
import { ApiError } from '@/api/http'
import { useAuthStore } from '@/stores/auth'
import { useCertsStore, useSitesStore } from '@/stores/site'
import { ACME_CHALLENGE, CERT_SOURCE, CERT_STATUS, dictItem } from '@/types/enums'
import type { CertSource, CertStatus } from '@/types/enums'
import type { CertItem, SiteRollbackData } from '@/types/site'
import { fieldOfString, isCertSitesData, isSiteRollbackData } from '@/types/site'
import { formatDateTime, formatTime } from '@/utils/format'
import { siteActionError } from '../errors'
import RollbackDiffModal from '../components/RollbackDiffModal.vue'
import CertDeployModal from './components/CertDeployModal.vue'
import CertDetailModal from './components/CertDetailModal.vue'
import CertRequestModal from './components/CertRequestModal.vue'
import CertUndeployModal from './components/CertUndeployModal.vue'

const authStore = useAuthStore()
const certsStore = useCertsStore()
const sitesStore = useSitesStore()
const message = useMessage()
const dialog = useDialog()

/** ---- 查询条件（api cert/list 真实参数 source/status） ---- */

const sourceOptions: SelectOption[] = (Object.keys(CERT_SOURCE) as CertSource[]).map((v) => ({
  label: CERT_SOURCE[v].label,
  value: v
}))
const statusOptions: SelectOption[] = (Object.keys(CERT_STATUS) as CertStatus[]).map((v) => ({
  label: CERT_STATUS[v].label,
  value: v
}))

const sourceFilter = computed<CertSource | null>({
  get: () => certsStore.sourceFilter,
  set: (v) => certsStore.setSourceFilter(v)
})
const statusFilter = computed<CertStatus | null>({
  get: () => certsStore.statusFilter,
  set: (v) => certsStore.setStatusFilter(v)
})

/** ---- 行内只读详情 Modal（数据源=行数据） ---- */

const detailCert = ref<CertItem | null>(null)

/** ---- 申请 / 部署 / 解除部署弹窗 ---- */

const requestShow = ref(false)
const deployCert = ref<CertItem | null>(null)
const deployShow = ref(false)
const undeployCert = ref<CertItem | null>(null)
const undeployShow = ref(false)

function openDeploy(cert: CertItem): void {
  deployCert.value = cert
  deployShow.value = true
}

function openUndeploy(cert: CertItem): void {
  undeployCert.value = cert
  undeployShow.value = true
}

/** CERT_005 引用清单 Modal（不可强制删除）+「去解除部署」引导 */
const referencedCert = ref<CertItem | null>(null)

function onWriteDone(): void {
  void certsStore.loadList()
  void sitesStore.loadList() // 站点列表证书状态列联动（D-11 实时推导）
}

/** CERT_003 查看已有证书引导（api 无证书详情接口——清单行数据源，不虚构请求）。
 *  未命中当前筛选结果时清除筛选重拉全量清单后再呈现（如实兑现「已清除筛选」文案）。 */
async function onViewCert(certId: number): Promise<void> {
  let cert = certsStore.items.find((c) => c.id === certId) ?? null
  if (cert === null) {
    certsStore.sourceFilter = null
    certsStore.statusFilter = null
    await certsStore.loadList()
    cert = certsStore.items.find((c) => c.id === certId) ?? null
    if (cert !== null) message.info('已有证书不在当前筛选结果中，已清除筛选')
    else message.warning('未在证书清单中找到该证书（可能已被删除）')
  }
  if (cert !== null) detailCert.value = cert
}

/** ---- 续期重试（仅 renew_failed 行；同步等待终态 loading 文案同申请） ---- */

const renewingId = ref<number | null>(null)

async function onRetryRenewal(row: CertItem): Promise<void> {
  renewingId.value = row.id
  const loadingMsg = message.loading('正在向 ACME 服务申请，最长约 2 分钟…', { duration: 0 })
  try {
    await certRetryRenewal(row.id)
    message.success('续期成功，已替换并重新部署')
    onWriteDone()
  } catch (e) {
    if (e instanceof ApiError && e.is('CERT_004')) {
      const reason = fieldOfString(e.data, 'reason') ?? '（服务端未返回原因）'
      dialog.error({ title: '续期失败', content: `${reason}\n（证书保持「续期失败」状态，可修正后重试）` })
      onWriteDone() // 刷新 lastError（FR-24）
      return
    }
    handleRowError(e)
  } finally {
    loadingMsg.destroy()
    renewingId.value = null
  }
}

/** ---- 删除（NPopconfirm 强确认，文案区分来源；CERT_005 → 引用清单 Modal） ---- */

async function onDelete(row: CertItem): Promise<void> {
  try {
    await certDelete(row.id)
    message.success(row.source === 'acme' ? '证书与托管文件已删除' : '已解除面板管理（源站证书文件未动）')
    onWriteDone()
  } catch (e) {
    if (e instanceof ApiError && e.is('CERT_005')) {
      if (isCertSitesData(e.data)) {
        const merged: CertItem = { ...row, referencedSites: e.data.sites }
        referencedCert.value = merged
        return
      }
    }
    handleRowError(e)
  }
}

/** 行级写动作错误兜底（SITE_003 回滚弹窗 / SITE_004 output 弹窗共用） */
const rollback = ref<SiteRollbackData | null>(null)

function handleRowError(e: unknown): void {
  if (!(e instanceof ApiError)) return
  if (e.is('SITE_003')) {
    if (isSiteRollbackData(e.data)) {
      rollback.value = e.data
      return
    }
  }
  const info = siteActionError(e)
  if (info === null) return
  if (info.output !== undefined) dialog.error({ title: info.text, content: info.output })
  else message.error(info.text)
}

/** ---- 表格列（4.2 列顺序；readonly 无操作列） ---- */

const columns = computed<DataTableColumns<CertItem>>(() => {
  const base: DataTableColumns<CertItem> = [
    { title: '序号', key: 'index', width: 60, render: (_row, index) => index + 1 },
    {
      title: '主域名',
      key: 'primaryDomain',
      width: 180,
      fixed: 'left',
      render: (row) =>
        h(
          NButton,
          { text: true, type: 'primary', onClick: () => (detailCert.value = row) },
          { default: () => row.primaryDomain }
        )
    },
    {
      title: '域名',
      key: 'domains',
      width: 160,
      render: (row): VNode => {
        const first = row.domains[0] ?? '—'
        const rest = row.domains.length - 1
        const full = row.domains.join(', ')
        const tags: VNode[] = [
          h(NTag, { size: 'small', bordered: false }, { default: () => first })
        ]
        if (rest > 0) {
          tags.push(
            h(NTag, { size: 'small', bordered: false, type: 'info' }, { default: () => `+${rest}` })
          )
        }
        return h(NTooltip, null, { trigger: () => h('span', { class: 'domain-tags' }, tags), default: () => full })
      }
    },
    { title: '签发者', key: 'issuer', width: 140, render: (row) => (row.issuer !== '' ? row.issuer : '—') },
    {
      title: '来源',
      key: 'source',
      width: 90,
      render: (row) =>
        h(
          NTag,
          { type: dictItem(CERT_SOURCE, row.source).tag, size: 'small', bordered: false },
          { default: () => dictItem(CERT_SOURCE, row.source).label }
        )
    },
    {
      title: '挑战方式',
      key: 'challenge',
      width: 110,
      render: (row) => (row.challenge != null ? dictItem(ACME_CHALLENGE, row.challenge).label : '—')
    },
    {
      title: '状态',
      key: 'status',
      width: 120,
      render: (row) => {
        const dict = dictItem(CERT_STATUS, row.status)
        const tag = h(
          NTag,
          { type: dict.tag, size: 'small', bordered: false },
          { default: () => dict.label }
        )
        // renew_failed 行附 lastError NTooltip（FR-24 排障呈现）
        if (row.status === 'renew_failed' && row.lastError != null && row.lastError !== '') {
          return h(NTooltip, null, { trigger: () => tag, default: () => row.lastError ?? '' })
        }
        return tag
      }
    },
    { title: '有效期至', key: 'notAfter', width: 180, render: (row) => formatDateTime(row.notAfter ?? null) },
    {
      title: '引用站点',
      key: 'referencedSites',
      width: 120,
      render: (row) => {
        if (row.referencedSites.length === 0) return '—'
        const full = row.referencedSites
          .map((s) => `${s.serverNames.join(', ')}（${s.siteId}）`)
          .join('\n')
        return h(NTooltip, null, {
          trigger: () => h('span', null, `${row.referencedSites.length} 站`),
          default: () => h('span', { class: 'ref-list' }, full)
        })
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
        const buttons: VNode[] = []
        buttons.push(
          h(NButton, { size: 'small', type: 'primary', ghost: true, onClick: () => openDeploy(row) }, { default: () => '部署' })
        )
        buttons.push(
          h(
            NButton,
            {
              size: 'small',
              disabled: row.referencedSites.length === 0,
              onClick: () => openUndeploy(row)
            },
            { default: () => '解除部署' }
          )
        )
        if (row.status === 'renew_failed') {
          buttons.push(
            h(
              NPopconfirm,
              {
                onPositiveClick: () => void onRetryRenewal(row)
              },
              {
                trigger: () =>
                  h(
                    NButton,
                    { size: 'small', type: 'warning', ghost: true, loading: renewingId.value === row.id },
                    { default: () => '续期重试' }
                  ),
                default: () =>
                  `上次失败原因：${row.lastError != null && row.lastError !== '' ? row.lastError : '（无记录）'}。将重新发起 ACME 续期（同步等待 ≤2 分钟）。`
              }
            )
          )
        }
        buttons.push(
          h(
            NPopconfirm,
            { onPositiveClick: () => void onDelete(row) },
            {
              trigger: () =>
                h(NButton, { size: 'small', type: 'error', ghost: true }, { default: () => '删除' }),
              default: () =>
                row.source === 'acme'
                  ? '将删除证书与托管文件及其续期任务，确认删除？'
                  : '仅解除面板管理关系，不删除源站证书文件，确认删除？'
            }
          )
        )
        return buttons
      }
    })
  }
  return base
})

/** readonly 视角无 fixed:right 操作列 */
const scrollX = computed(() => (authStore.isAdmin ? 1380 : 1160))

/** expiring_soon 行整行橙色浅底（PageSpec 4.2） */
function rowClassName(row: CertItem): string {
  return row.status === 'expiring_soon' ? 'cert-expiring-row' : ''
}

function rowKey(row: CertItem): number {
  return row.id
}

onMounted(() => {
  void certsStore.loadList()
  // 站点数据源：申请/部署弹窗需要（admin 专属入口；readonly 不拉取——phpPoolsList 类比不触达）
  if (authStore.isAdmin) void sitesStore.loadList()
})

onUnmounted(() => {
  renewingId.value = null
})
</script>

<template>
  <div class="page">
    <!-- 查询区（readonly 同可筛选）+ 工具栏（admin：申请证书） -->
    <n-card size="small">
      <div class="controls-row">
        <n-select
          v-model:value="sourceFilter"
          clearable
          :options="sourceOptions"
          class="ctrl"
          placeholder="来源（空=全部）"
        />
        <n-select
          v-model:value="statusFilter"
          clearable
          :options="statusOptions"
          class="ctrl"
          placeholder="状态（空=全部）"
        />
        <n-button type="primary" :loading="certsStore.listLoading" @click="certsStore.loadList()">
          刷新
        </n-button>
        <div class="spacer"></div>
        <n-button v-if="authStore.isAdmin" type="primary" @click="requestShow = true">
          申请证书
        </n-button>
      </div>
    </n-card>

    <n-card size="small" title="证书列表">
      <template #header-extra>
        <span class="meta"> 共 {{ certsStore.items.length }} 张 · 生成于 {{ formatTime(certsStore.generatedAt) }} </span>
      </template>
      <n-data-table
        :columns="columns"
        :data="certsStore.items"
        :loading="certsStore.listLoading"
        :scroll-x="scrollX"
        :row-key="rowKey"
        :row-class-name="rowClassName"
      >
        <template #empty>
          <n-empty description="未匹配到证书" class="empty" />
        </template>
      </n-data-table>
    </n-card>

    <!-- 行内只读详情（数据源=行数据） -->
    <cert-detail-modal :cert="detailCert" @close="detailCert = null" />

    <!-- admin 弹窗族 -->
    <template v-if="authStore.isAdmin">
      <cert-request-modal
        v-model:show="requestShow"
        @done="onWriteDone"
        @view-cert="onViewCert"
      />
      <cert-deploy-modal
        v-model:show="deployShow"
        :cert="deployCert"
        @done="onWriteDone"
      />
      <cert-undeploy-modal
        v-model:show="undeployShow"
        :cert="undeployCert"
        @done="onWriteDone"
      />
    </template>

    <!-- CERT_005：引用站点清单 + 去解除部署引导（不可强制删除） -->
    <n-modal
      :show="referencedCert !== null"
      preset="card"
      title="证书仍被站点引用"
      style="width: 520px"
      @update:show="(v: boolean) => { if (!v) referencedCert = null }"
    >
      <template v-if="referencedCert !== null">
        <n-alert type="warning" :show-icon="true" class="ref-alert">
          以下站点仍在引用该证书（CERT_005）。请先解除部署，再执行删除——面板不提供强制删除。
        </n-alert>
        <ul class="ref-sites">
          <li v-for="site in referencedCert.referencedSites" :key="site.siteId" class="ref-site">
            <span>{{ site.serverNames.join(', ') }}</span>
            <span class="mono">{{ site.siteId }}</span>
          </li>
        </ul>
      </template>
      <template #footer>
        <div class="footer">
          <n-button @click="referencedCert = null">关闭</n-button>
          <n-button
            type="warning"
            @click="
              () => {
                if (referencedCert !== null) {
                  const target = referencedCert
                  referencedCert = null
                  openUndeploy(target)
                }
              }
            "
          >
            去解除部署
          </n-button>
        </div>
      </template>
    </n-modal>

    <!-- SITE_003 回滚 diff 弹窗（cert 行写动作共用） -->
    <rollback-diff-modal :data="rollback" @close="rollback = null" />
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
.ctrl {
  width: 180px;
}
.spacer {
  flex: 1;
}
.meta {
  font-size: 12px;
  opacity: 0.6;
}
.domain-tags {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
:deep(.cert-expiring-row) td {
  background-color: rgba(240, 160, 32, 0.09);
}
.ref-list {
  white-space: pre-line;
  font-family: Consolas, Menlo, monospace;
  font-size: 12px;
}
.ref-alert {
  margin-bottom: 10px;
}
.ref-sites {
  margin: 0;
  padding: 4px 0 4px 20px;
}
.ref-site {
  display: flex;
  flex-direction: column;
  line-height: 1.8;
}
.mono {
  font-family: Consolas, Menlo, monospace;
  font-size: 11px;
  opacity: 0.6;
  word-break: break-all;
}
.footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
.empty {
  padding: 24px 0;
}
</style>
