<script setup lang="ts">
/**
 * 站点详情 Drawer（PageSpec 4.4，hybrid 宿主 site-list；width 640）。
 * 顶部：serverNames 标题 + enabled 开关（admin：NSwitch+NPopconfirm；readonly 只读 tag）。
 * Card：基本信息（admin 编辑态 siteUpdate）/ 反向代理（admin 反代站点编辑 proxyUpdate——
 * M3 验收主场景，成功反馈含「其余站点无变化」R-16 语义）/ 证书（未纳管 orange 提示 +
 * 去证书页路由链接）/ location 只读清单 / Raw 块只读（R-07 原样保留提示 + admin 源码编辑链接）/
 * 日志（NTabs access/error + 行数 100/500/1000 + 刷新；两角色可看，readonly 唯一可操作区=刷新）。
 * SITE_003 → 共用回滚 diff 弹窗；SITE_004 → output 弹窗；SITE_011 → 日志空态如实呈现。
 */
import { computed, nextTick, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useDialog, useMessage } from 'naive-ui'
import type { DataTableColumns, FormInst, FormRules } from 'naive-ui'
import {
  logTail,
  proxyUpdate,
  siteDetail as fetchSiteDetail,
  siteDisable,
  siteEnable,
  siteUpdate
} from '@/api/site'
import { ApiError } from '@/api/http'
import { useAuthStore } from '@/stores/auth'
import { CERT_STATUS, SITE_ENABLED, dictItem } from '@/types/enums'
import type { SiteLogType } from '@/types/enums'
import type { LogTailData, SiteDetail, SiteRollbackData } from '@/types/site'
import { isSiteRollbackData } from '@/types/site'
import { siteActionError } from '../errors'
import RollbackDiffModal from './RollbackDiffModal.vue'

const props = defineProps<{
  /** 目标站点（null=关闭态） */
  siteId: string | null
  /** 打开即定位日志 Card（operList「日志」便捷入口） */
  initialLog: boolean
}>()

const emit = defineEmits<{
  /** 任一写动作成功（页面重拉列表投影，D-01 实况） */
  'list-changed': []
}>()

const show = defineModel<boolean>('show', { default: false })

const router = useRouter()
const authStore = useAuthStore()
const message = useMessage()
const dialog = useDialog()

const detail = ref<SiteDetail | null>(null)
const detailLoading = ref(false)
/** SITE_002 站点不存在（面板外改动） */
const detailNotFound = ref(false)

const rollback = ref<SiteRollbackData | null>(null)
const toggling = ref(false)

async function loadDetail(): Promise<void> {
  const siteId = props.siteId
  if (siteId === null) return
  detailLoading.value = true
  detailNotFound.value = false
  detail.value = null
  try {
    const data = await fetchSiteDetail(siteId)
    if (!show.value || props.siteId !== siteId) return // 迟到响应不写入已切换/关闭的 Drawer
    detail.value = data
    // operList「日志」便捷入口：定位日志 Card（PageSpec 4.1）——一次性滚动锚点
    if (scrollLogOnce) {
      scrollLogOnce = false
      void nextTick(() => logAnchorRef.value?.scrollIntoView({ behavior: 'smooth', block: 'start' }))
    }
  } catch (e) {
    if (e instanceof ApiError && e.is('SITE_002')) detailNotFound.value = true
    // 其余（网络等）已由拦截器提示
  } finally {
    detailLoading.value = false
  }
}

/** ---- 顶部启停开关（admin；readonly 只读 tag） ---- */

const enabledDict = computed(() =>
  detail.value !== null ? dictItem(SITE_ENABLED, detail.value.enabled) : null
)

async function onToggle(target: boolean): Promise<void> {
  if (detail.value === null) return
  const siteId = detail.value.siteId
  toggling.value = true
  try {
    const data = target ? await siteEnable(siteId) : await siteDisable(siteId)
    if (detail.value !== null) detail.value.enabled = data.enabled
    message.success(
      data.enabled === 'enabled' ? '站点已启用并生效' : '站点已停用（配置与证书引用已保留，停用≠删除）'
    )
    emit('list-changed')
  } catch (e) {
    handleWriteError(e)
  } finally {
    toggling.value = false
  }
}

/** ---- 写动作错误共用分流（SITE_003/SITE_004/SITE_002 专项） ---- */

function handleWriteError(e: unknown): void {
  if (!(e instanceof ApiError)) return
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

/** ---- 基本信息 Card（serverNames/listen/root；admin 编辑态 siteUpdate） ---- */

const editingInfo = ref(false)
const infoFormRef = ref<FormInst | null>(null)
const infoSaving = ref(false)
const infoForm = ref<{ serverNames: string[]; listen: string; root: string }>({
  serverNames: [],
  listen: '',
  root: ''
})

const DOMAIN_RE = /^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$/

const infoRules: FormRules = {
  serverNames: [
    {
      validator: (_rule, value: string[]) =>
        value.length > 0 && value.every((d) => DOMAIN_RE.test(d)),
      message: '至少 1 个合法域名（不允许空值、空格或通配符，SITE_007）',
      trigger: ['blur', 'change']
    }
  ],
  listen: [
    {
      validator: (_rule, value: string) =>
        value
          .split(',')
          .map((s) => s.trim())
          .filter((s) => s !== '')
          .every((p) => /^\d+$/.test(p) && Number(p) >= 1 && Number(p) <= 65535),
      message: '监听端口非法：逗号分隔，每项 1~65535（SITE_006）',
      trigger: ['blur', 'input']
    }
  ],
  root: [{ required: true, message: '根目录必填', trigger: ['blur', 'input'] }]
}

function startEditInfo(): void {
  if (detail.value === null) return
  infoForm.value = {
    serverNames: [...detail.value.serverNames],
    listen: detail.value.listen.join(','),
    root: detail.value.root
  }
  editingInfo.value = true
}

async function submitInfoEdit(): Promise<void> {
  if (detail.value === null) return
  await infoFormRef.value?.validate()
  infoSaving.value = true
  const f = infoForm.value
  try {
    await siteUpdate({
      siteId: detail.value.siteId,
      serverNames: f.serverNames,
      listen: f.listen.split(',').map((s) => s.trim()).filter((s) => s !== ''),
      root: f.root
    })
    message.success('站点信息已保存并生效')
    editingInfo.value = false
    emit('list-changed')
    await loadDetail()
  } catch (e) {
    handleWriteError(e)
  } finally {
    infoSaving.value = false
  }
}

/** ---- 反向代理 Card（proxyUpdate——M3 验收主场景） ---- */

const editingProxy = ref(false)
const proxyFormRef = ref<FormInst | null>(null)
const proxySaving = ref(false)
const proxyForm = ref<{ upstream: string; hostHeader: string; websocket: boolean }>({
  upstream: '',
  hostHeader: '',
  websocket: false
})

const UPSTREAM_RE = /^https?:\/\/[^\s/]+(?::\d{1,5})?(?:[/?#]\S*)?$/

const proxyRules: FormRules = {
  upstream: [
    {
      validator: (_rule, value: string) => UPSTREAM_RE.test(value),
      message: '上游地址须为 http(s)://host[:port] 形态（SITE_008）',
      trigger: ['blur', 'input']
    }
  ]
}

function startEditProxy(): void {
  if (detail.value?.proxy == null) return
  proxyForm.value = {
    upstream: detail.value.proxy.upstream,
    hostHeader: detail.value.proxy.hostHeader, // string，""=无覆写（W-1 对齐）
    websocket: detail.value.proxy.websocket
  }
  editingProxy.value = true
}

async function submitProxyEdit(): Promise<void> {
  if (detail.value === null) return
  await proxyFormRef.value?.validate()
  proxySaving.value = true
  const f = proxyForm.value
  try {
    await proxyUpdate({
      siteId: detail.value.siteId,
      upstream: f.upstream,
      hostHeader: f.hostHeader !== '' ? f.hostHeader : undefined,
      websocket: f.websocket
    })
    // M3 验收主场景反馈（PageSpec 第 8 节 #11：目标站点已生效、其余站点无变化 R-16）
    message.success('反代配置已保存：目标站点已生效，其余站点无变化')
    editingProxy.value = false
    emit('list-changed')
    await loadDetail()
  } catch (e) {
    handleWriteError(e)
  } finally {
    proxySaving.value = false
  }
}

function goCertificates(): void {
  void router.push('/certificates')
}

function goSource(): void {
  if (detail.value === null) return
  void router.push(`/sites/${encodeURIComponent(detail.value.siteId)}/source`)
}

/** ---- location 清单（只读；结构化单 location 编辑不做——全量经源码编辑） ---- */

const locationColumns: DataTableColumns<{ path: string; proxyPass?: string; fastcgiPass?: string }> = [
  { title: 'path', key: 'path', width: 150 },
  { title: 'proxy_pass', key: 'proxyPass' },
  { title: 'fastcgi_pass', key: 'fastcgiPass' }
]

/** ---- 日志 Card（两角色可看；readonly Drawer 唯一可操作区=刷新，D0 口径） ---- */

const LOG_LINES_OPTIONS = [100, 500, 1000]
const logType = ref<SiteLogType>('access')
const logLinesCount = ref(100)
const logData = ref<LogTailData | null>(null)
const logLoading = ref(false)
/** SITE_011：日志文件不存在或不可读（⚠️A15 回退链穷尽，如实呈现） */
const logMissing = ref(false)

async function loadLog(): Promise<void> {
  if (props.siteId === null) return
  logLoading.value = true
  logMissing.value = false
  try {
    const data = await logTail(props.siteId, logType.value, logLinesCount.value)
    logData.value = data
  } catch (e) {
    if (e instanceof ApiError && e.is('SITE_011')) {
      logMissing.value = true
      logData.value = null
    }
    // 其余（网络等）已由拦截器提示
  } finally {
    logLoading.value = false
  }
}

function onLogTypeChange(): void {
  void loadLog()
}

function onLogLinesChange(): void {
  void loadLog()
}

/** ---- 打开/切换装载 ---- */

/** 日志 Card 滚动锚点（operList「日志」打开即定位，一次性消费） */
const logAnchorRef = ref<HTMLElement | null>(null)
let scrollLogOnce = false

watch([show, () => props.siteId], ([open]) => {
  if (open && props.siteId !== null) {
    scrollLogOnce = props.initialLog
    void loadDetail()
    if (props.initialLog) logType.value = 'access'
    void loadLog()
  }
})
</script>

<template>
  <n-drawer v-model:show="show" :width="640" placement="right">
    <n-drawer-content closable>
      <template #header>
        <div class="drawer-header">
          <span class="drawer-header__title" :title="detail?.siteId ?? ''">
            {{ detail?.serverNames.join(', ') ?? '站点详情' }}
          </span>
          <!-- admin：开关语义即启停动作（确认弹层 + 响应值回写）；readonly 只读 tag -->
          <n-popconfirm
            v-if="authStore.isAdmin && detail !== null"
            @positive-click="onToggle(detail.enabled !== 'enabled')"
          >
            <template #trigger>
              <n-switch
                size="small"
                :value="detail.enabled === 'enabled'"
                :loading="toggling"
                :disabled="toggling"
              />
            </template>
            {{
              detail.enabled === 'enabled'
                ? '确认停用该站点？将整体注释 server 块并重载 nginx；保留配置与证书引用，停用≠删除。'
                : '确认启用该站点？将还原 server 块并重载 nginx。'
            }}
          </n-popconfirm>
          <n-tag
            v-else-if="enabledDict !== null"
            :type="enabledDict.tag"
            size="small"
            :bordered="false"
          >
            {{ enabledDict.label }}
          </n-tag>
        </div>
      </template>

      <div class="cards">
        <!-- 站点不存在（SITE_002：面板外改动实况） -->
        <n-empty
          v-if="detailNotFound"
          description="站点不存在（可能已被面板外改动），请刷新列表"
          class="state-block"
        />
        <n-spin v-else-if="detailLoading && detail === null" class="state-block" />
        <template v-else-if="detail !== null">
          <!-- Card 1：基本信息 -->
          <n-card size="small" title="基本信息">
            <template #header-extra>
              <n-button
                v-if="authStore.isAdmin && !editingInfo"
                size="tiny"
                @click="startEditInfo"
              >
                编辑
              </n-button>
            </template>
            <n-descriptions v-if="!editingInfo" :column="1" label-placement="left" bordered size="small">
              <n-descriptions-item label="域名">
                {{ detail.serverNames.join(', ') }}
              </n-descriptions-item>
              <n-descriptions-item label="监听">
                {{ detail.listen.join(', ') }}
              </n-descriptions-item>
              <n-descriptions-item label="根目录">{{ detail.root }}</n-descriptions-item>
              <n-descriptions-item label="来源文件">
                <span class="mono">{{ detail.sourceFile }}</span>
              </n-descriptions-item>
            </n-descriptions>
            <n-form
              v-else
              ref="infoFormRef"
              :model="infoForm"
              :rules="infoRules"
              label-placement="top"
              size="small"
            >
              <n-form-item label="域名（首个为主域名）" path="serverNames">
                <n-select
                  v-model:value="infoForm.serverNames"
                  multiple
                  filterable
                  tag
                  clearable
                  :options="[]"
                  placeholder="输入域名后回车添加"
                />
              </n-form-item>
              <n-form-item label="监听（多值逗号分隔）" path="listen">
                <n-input v-model:value="infoForm.listen" placeholder="如 80,443 ssl" />
              </n-form-item>
              <n-form-item label="根目录" path="root">
                <n-input v-model:value="infoForm.root" />
              </n-form-item>
              <div class="form-actions">
                <n-button size="small" @click="editingInfo = false">取消</n-button>
                <n-button size="small" type="primary" :loading="infoSaving" @click="submitInfoEdit">
                  保存并生效
                </n-button>
              </div>
            </n-form>
          </n-card>

          <!-- Card 2：反向代理（非反代站点整 Card「—」） -->
          <n-card size="small" title="反向代理">
            <template #header-extra>
              <n-button
                v-if="authStore.isAdmin && detail.proxy !== null && !editingProxy"
                size="tiny"
                @click="startEditProxy"
              >
                编辑
              </n-button>
            </template>
            <span v-if="detail.proxy === null" class="muted">—（该站点非反向代理类型）</span>
            <n-descriptions
              v-else-if="!editingProxy"
              :column="1"
              label-placement="left"
              bordered
              size="small"
            >
              <n-descriptions-item label="上游地址">
                <span class="mono">{{ detail.proxy.upstream }}</span>
              </n-descriptions-item>
              <n-descriptions-item label="Host 头">
                {{ detail.proxy.hostHeader !== '' ? detail.proxy.hostHeader : '—（透传原 Host）' }}
              </n-descriptions-item>
              <n-descriptions-item label="WebSocket 透传">
                {{ detail.proxy.websocket ? '开启' : '关闭' }}
              </n-descriptions-item>
            </n-descriptions>
            <n-form
              v-else
              ref="proxyFormRef"
              :model="proxyForm"
              :rules="proxyRules"
              label-placement="top"
              size="small"
            >
              <n-form-item label="上游地址" path="upstream">
                <n-input v-model:value="proxyForm.upstream" placeholder="http://127.0.0.1:8080" />
              </n-form-item>
              <n-form-item label="Host 头（可空）" path="hostHeader">
                <n-input v-model:value="proxyForm.hostHeader" placeholder="默认透传原 Host" />
              </n-form-item>
              <n-form-item label="WebSocket 透传" path="websocket">
                <n-switch v-model:value="proxyForm.websocket" size="small" />
              </n-form-item>
              <div class="form-actions">
                <n-button size="small" @click="editingProxy = false">取消</n-button>
                <n-button size="small" type="primary" :loading="proxySaving" @click="submitProxyEdit">
                  保存并生效
                </n-button>
              </div>
            </n-form>
          </n-card>

          <!-- Card 3：证书（certRef/certStatus/certId；未纳管 orange 提示） -->
          <n-card size="small" title="证书">
            <template #header-extra>
              <n-button size="tiny" @click="goCertificates"> 去证书页部署/管理 </n-button>
            </template>
            <n-descriptions :column="1" label-placement="left" bordered size="small">
              <n-descriptions-item label="证书引用">
                <span v-if="detail.certRef !== null" class="mono">{{ detail.certRef }}</span>
                <span v-else class="muted">无引用</span>
              </n-descriptions-item>
              <n-descriptions-item label="纳管证书">
                <template v-if="detail.certId !== undefined && detail.certStatus !== undefined">
                  #{{ detail.certId }} ·
                  <n-tag
                    :type="dictItem(CERT_STATUS, detail.certStatus).tag"
                    size="small"
                    :bordered="false"
                  >
                    {{ dictItem(CERT_STATUS, detail.certStatus).label }}
                  </n-tag>
                </template>
                <span v-else-if="detail.certRef !== null" class="muted">未纳管</span>
                <span v-else class="muted">—</span>
              </n-descriptions-item>
            </n-descriptions>
            <n-alert
              v-if="detail.certRef !== null && detail.certId === undefined"
              type="warning"
              :show-icon="true"
              class="card-alert"
            >
              该引用未纳入面板管理，可在证书页识别后部署。
            </n-alert>
          </n-card>

          <!-- Card 4：location 清单（只读） -->
          <n-card size="small" title="location 清单">
            <n-data-table
              v-if="detail.locations.length > 0"
              :columns="locationColumns"
              :data="detail.locations"
              size="small"
              :bordered="false"
            />
            <span v-else class="muted">—（无 location 块）</span>
          </n-card>

          <!-- Card 5：Raw 块（只读原文；R-07 写回原样保留） -->
          <n-card size="small" title="Raw 未识别指令块">
            <template #header-extra>
              <n-button v-if="authStore.isAdmin" size="tiny" @click="goSource">源码编辑</n-button>
            </template>
            <template v-if="detail.rawBlocks.length > 0">
              <div class="raw-hint">以下指令块在结构化写回时按原文本原样保留（R-07 保全承诺）。</div>
              <div v-for="block in detail.rawBlocks" :key="block.line" class="raw-block">
                <span class="raw-line-no">L{{ block.line }}</span>
                <pre class="raw-text">{{ block.text }}</pre>
              </div>
            </template>
            <span v-else class="muted">—（无未识别指令块）</span>
          </n-card>

          <!-- Card 6：日志（两角色可看；行数选择 + 刷新） -->
          <n-card size="small" title="日志">
            <!-- 滚动定位锚点（operList「日志」便捷入口；零高度不影响布局） -->
            <span ref="logAnchorRef" class="log-anchor"></span>
            <div class="log-controls">
              <n-tabs v-model:value="logType" type="segment" size="small" @update:value="onLogTypeChange">
                <n-tab name="access">访问日志</n-tab>
                <n-tab name="error">错误日志</n-tab>
              </n-tabs>
              <div class="log-actions">
                <n-select
                  v-model:value="logLinesCount"
                  size="small"
                  class="log-lines"
                  :options="LOG_LINES_OPTIONS.map((n) => ({ label: `${n} 行`, value: n }))"
                  @update:value="onLogLinesChange"
                />
                <n-button size="small" :loading="logLoading" @click="loadLog()">刷新</n-button>
              </div>
            </div>
            <n-alert v-if="logData !== null && logData.truncated" type="info" :show-icon="true" class="card-alert">
              日志较长，仅展示最近 {{ logLinesCount }} 行。
            </n-alert>
            <n-empty
              v-if="logMissing"
              description="日志文件不存在或不可读（该站点未配置日志路径，且 nginx 默认路径不可用）"
              class="state-block small"
            />
            <n-spin v-else-if="logLoading && logData === null" class="state-block small" />
            <div v-else-if="logData !== null" class="log-list">
              <div v-for="(line, i) in logData.lines" :key="i" class="log-line">
                <span v-if="line.ts !== undefined" class="log-ts">{{ line.ts }}</span>
                <span class="log-text">{{ line.text }}</span>
              </div>
              <div v-if="logData.lines.length === 0" class="muted log-empty">（空日志）</div>
            </div>
            <div v-if="logData !== null && logData.logPath !== ''" class="log-path mono">
              路径：{{ logData.logPath }}
            </div>
          </n-card>
        </template>
      </div>

      <rollback-diff-modal :data="rollback" @close="rollback = null" />
    </n-drawer-content>
  </n-drawer>
</template>

<style scoped>
.drawer-header {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  min-width: 0;
}
.drawer-header__title {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.cards {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.muted {
  opacity: 0.55;
}
.mono {
  font-family: Consolas, Menlo, monospace;
  font-size: 12px;
  word-break: break-all;
}
.card-alert {
  margin-top: 10px;
}
.form-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}
.raw-hint {
  font-size: 12px;
  opacity: 0.6;
  margin-bottom: 8px;
}
.raw-block {
  display: flex;
  gap: 8px;
  padding: 6px 8px;
  border-radius: 4px;
  background-color: rgba(128, 128, 128, 0.08);
  margin-bottom: 6px;
}
.raw-line-no {
  font-size: 11px;
  opacity: 0.5;
  min-width: 36px;
  text-align: right;
}
.raw-text {
  margin: 0;
  font-family: Consolas, Menlo, monospace;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
}
.log-controls {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 10px;
}
.log-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.log-lines {
  width: 100px;
}
.log-list {
  max-height: 320px;
  overflow: auto;
  border-radius: 4px;
  background-color: rgba(128, 128, 128, 0.08);
  padding: 8px 10px;
}
.log-line {
  font-family: Consolas, Menlo, monospace;
  font-size: 12px;
  line-height: 1.7;
  word-break: break-all;
}
.log-ts {
  opacity: 0.55;
  margin-right: 8px;
}
.log-empty {
  padding: 8px 0;
}
.log-path {
  font-size: 11px;
  opacity: 0.55;
  margin-top: 8px;
}
.state-block {
  padding: 48px 0;
}
.log-anchor {
  display: block;
  height: 0;
}
.state-block.small {
  padding: 24px 0;
}
</style>
