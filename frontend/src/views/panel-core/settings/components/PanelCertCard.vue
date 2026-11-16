<script setup lang="ts">
import { computed, ref } from 'vue'
import { useMessage } from 'naive-ui'
import { ApiError } from '@/api/http'
import { certRequest, certRetryRenewal } from '@/api/panel'
import { useAuthStore } from '@/stores/auth'
import { usePanelStore } from '@/stores/panel'
import { CONFIG_EFFECT_MODE, PANEL_CERT_SOURCE, PANEL_CERT_STATUS, dictItem } from '@/types/enums'
import type { PanelConfigDetail } from '@/types/panel'
import { formatDateTime } from '@/utils/format'

/**
 * Card 5 面板证书（PageSpec 5.1 v1.1，ADR-029/D-P13）：
 * 数据源 certStatus（store，进入页面与 configDetail 并行拉取；申请/重试成功后 re-fetch）；
 * 申请/重试为同步终态（≤ACME_TIMEOUT=120s）——loading 等待态 + 防重复触发，无轮询定时器。
 * readonly：状态全可见（certStatus 两角色），无「申请证书」「手动重试续期」按钮（负空间，外呼从紧）。
 */
const props = defineProps<{ detail: PanelConfigDetail }>()
const message = useMessage()
const authStore = useAuthStore()
const panelStore = usePanelStore()

/** Card 角标：证书动作（申请/重试/自动续期）即时生效（PageSpec Card 5；cert 无独立 effectMode 分组） */
const effectTag = dictItem(CONFIG_EFFECT_MODE, 'immediate')

/** null=未加载（页面并行拉取中） */
const certStatus = computed(() => panelStore.certStatusData)
const certLoading = computed(() => panelStore.certStatusLoading)
const cert = computed(() => certStatus.value?.cert ?? null)
/** ACME 域名 = bind_domain 当前值（certStatus.acme.domain）；certStatus 未加载时回退 detail.bindDomain.value（同一落库值，避免加载窗误报「未绑定」） */
const acmeDomain = computed(() => certStatus.value?.acme.domain ?? props.detail.bindDomain.value)
/** 未绑定时申请按钮禁用（PANEL_007 前端预判） */
const domainBound = computed(() => acmeDomain.value !== null && acmeDomain.value !== '')

const certSourceTag = computed(() =>
  cert.value === null ? null : dictItem(PANEL_CERT_SOURCE, cert.value.source)
)
/** 状态 tag 仅 acme 来源显示（source≠acme 时 status=null——自签/导入无续期语义，D-P13） */
const certStatusTag = computed(() => {
  if (cert.value === null || cert.value.source !== 'acme' || cert.value.status === null) return null
  return dictItem(PANEL_CERT_STATUS, cert.value.status)
})

const validityText = computed(() => {
  const c = cert.value
  if (c === null) return '—'
  return `${formatDateTime(c.notBefore)} ~ ${formatDateTime(c.notAfter)}`
})

/** notAfter 30 天内 → 橙色临近过期提示（CERT_EXPIRING_DAYS=30，ADR-024 阈值同值） */
const expiringSoon = computed(() => {
  const na = cert.value?.notAfter ?? null
  if (na === null) return false
  const t = new Date(na).getTime()
  return !Number.isNaN(t) && t - Date.now() < 30 * 24 * 60 * 60 * 1000
})

/** 上次失败原因：renew_failed 落库态呈现（FR-18 排障口径；首次申请失败无行不落库，D-P13） */
const lastError = computed(() => {
  const c = cert.value
  if (c === null || c.status !== 'renew_failed') return null
  return c.lastError !== null && c.lastError !== '' ? c.lastError : null
})

/** 手动重试可见性：仅 cert.source=acme 且 status=renew_failed（PageSpec Card 5 按钮 2） */
const retryVisible = computed(
  () => authStore.isAdmin && cert.value !== null && cert.value.source === 'acme' && cert.value.status === 'renew_failed'
)

/** ApiError.data（unknown）安全取字符串字段（PANEL_008/009 reason、PANEL_011 notAfter） */
function dataString(err: ApiError, key: string): string | null {
  if (typeof err.data !== 'object' || err.data === null) return null
  const v = (err.data as Record<string, unknown>)[key]
  return typeof v === 'string' && v !== '' ? v : null
}

/** 页面级错误反馈（PANEL_009/010/011 不在拦截器 PANEL_ERROR_TEXT，避免无数据双提示）：
 * PANEL_009→展示 data.reason 不自动重试（D-P13）；PANEL_010→兜底「当前状态无需重试」；
 * PANEL_011→「已有有效 ACME 证书（notAfter 到期），续期请使用下方重试」。PANEL_007/008 由拦截器统一文案提示。 */
function onActionError(err: unknown, action: 'request' | 'retry'): void {
  if (err instanceof ApiError) {
    if (err.is('PANEL_009')) {
      const reason = dataString(err, 'reason')
      message.error(`ACME 签发/续期失败${reason !== null ? `：${reason}` : ''}`)
      return
    }
    if (err.is('PANEL_010')) {
      message.error('当前状态无需重试')
      return
    }
    if (err.is('PANEL_011')) {
      const notAfter = dataString(err, 'notAfter')
      message.error(
        `已有有效 ACME 证书（${notAfter !== null ? formatDateTime(notAfter) : ''} 到期），续期请使用${
          action === 'request' ? '下方' : ''
        }重试`
      )
      return
    }
    // PANEL_007/008 及网络错误已由拦截器统一提示
    return
  }
  message.error('操作失败，请稍后重试')
}

/** 成功后统一刷新：certStatus（摘要刷新）+ configDetail（https_mode 联动 acme，D-P14） */
async function refreshAfterAction(): Promise<void> {
  await Promise.all([panelStore.loadCertStatus(), panelStore.loadConfig()])
}

/** 申请证书：同步等待 ≤120s——按钮 loading + 明示时长文案（PageSpec Card 5 按钮 1；无轮询定时器） */
const requesting = ref(false)
const WAIT_HINT = '正在签发（经 nginx 验证域名，最长约 2 分钟），请勿关闭页面'

async function onRequest(): Promise<void> {
  if (requesting.value) return // loading 防重复触发
  requesting.value = true
  try {
    await certRequest()
    message.success('证书已签发，HTTPS 已切换为 ACME 证书（记入审计）')
    await refreshAfterAction()
  } catch (err) {
    onActionError(err, 'request')
  } finally {
    requesting.value = false
  }
}

/** 手动重试续期：同一通道同步等待（PageSpec Card 5 按钮 2） */
const retrying = ref(false)

async function onRetry(): Promise<void> {
  if (retrying.value) return
  retrying.value = true
  try {
    await certRetryRenewal()
    message.success('续期成功，证书已更新（记入审计）')
    await refreshAfterAction()
  } catch (err) {
    onActionError(err, 'retry')
  } finally {
    retrying.value = false
  }
}

/** 未绑定提示 → 滚动至域名绑定 Card（不改 URL hash，避免路由扰动） */
function gotoBindCard(): void {
  document.getElementById('panel-bind-domain-card')?.scrollIntoView({ behavior: 'smooth' })
}
</script>

<template>
  <n-card title="面板证书">
    <template #header-extra>
      <n-tag :type="effectTag.tag" size="small" :bordered="false">{{ effectTag.label }}</n-tag>
    </template>

    <!-- ACME 域名（=bind_domain 当前值）；未绑定 → 引导至域名绑定 Card -->
    <n-alert v-if="!domainBound" type="info" :closable="false" class="domain-alert">
      请先在上方「域名绑定」Card 绑定域名
      <n-button text type="primary" size="small" class="goto-bind" @click="gotoBindCard">
        前往绑定
      </n-button>
    </n-alert>
    <n-descriptions v-else label-placement="left" :column="1" bordered size="small" class="meta-desc">
      <n-descriptions-item label="ACME 域名">
        <n-code :code="acmeDomain ?? ''" class="domain-code" />
      </n-descriptions-item>
    </n-descriptions>

    <n-descriptions label-placement="left" :column="1" bordered size="small" class="meta-desc">
      <n-descriptions-item label="挑战方式">HTTP-01（经服务器已有 nginx 验证）</n-descriptions-item>
      <n-descriptions-item label="自动续期">开启</n-descriptions-item>
    </n-descriptions>
    <p class="hint">
      ACME 证书到期前 30 天自动续期（每日后台调度，失败不自动轰炸）；失败后可手动重试。
    </p>

    <!-- 证书摘要：cert=null→未配置；存在→来源 tag/状态 tag（仅 acme）/颁发者/有效期；无「查看私钥」入口（I-P06） -->
    <n-descriptions title="证书摘要" label-placement="left" :column="1" bordered size="small" class="cert-desc">
      <n-descriptions-item label="证书">
        <span v-if="certLoading && cert === null">加载中…</span>
        <n-tag v-else-if="cert === null" size="small" :bordered="false">未配置证书</n-tag>
        <template v-else>
          <n-tag
            v-if="certSourceTag !== null"
            :type="certSourceTag.tag"
            size="small"
            :bordered="false"
          >
            {{ certSourceTag.label }}
          </n-tag>
          <n-tag
            v-if="certStatusTag !== null"
            :type="certStatusTag.tag"
            size="small"
            :bordered="false"
            class="status-tag"
          >
            {{ certStatusTag.label }}
          </n-tag>
        </template>
      </n-descriptions-item>
      <n-descriptions-item label="颁发者">{{ cert?.issuer ?? '—' }}</n-descriptions-item>
      <n-descriptions-item label="有效期">
        <span>{{ validityText }}</span>
        <n-tag v-if="expiringSoon" type="warning" size="small" :bordered="false" class="expire-tag">
          30 天内到期
        </n-tag>
      </n-descriptions-item>
    </n-descriptions>

    <!-- 上次失败原因（renew_failed 落库态，FR-18 排障） -->
    <n-alert v-if="lastError !== null" type="error" title="上次续期失败原因" :closable="false" class="last-error">
      {{ lastError }}
    </n-alert>

    <!-- 动作区：admin 专属（readonly 负空间）；申请按钮未绑定域名禁用（PANEL_007 前端预判） -->
    <div v-if="authStore.isAdmin" class="actions">
      <n-button
        v-if="retryVisible"
        type="warning"
        ghost
        :loading="retrying"
        :disabled="requesting"
        @click="onRetry"
      >
        手动重试续期
      </n-button>
      <n-button
        type="primary"
        :loading="requesting"
        :disabled="!domainBound || retrying"
        @click="onRequest"
      >
        申请证书
      </n-button>
    </div>
    <!-- 同步等待提示（≤120s 明示时长，D-P13） -->
    <p v-if="requesting || retrying" class="wait-hint">{{ WAIT_HINT }}</p>
  </n-card>
</template>

<style scoped>
.domain-alert {
  margin-bottom: 12px;
}
.goto-bind {
  margin-left: 8px;
}
.meta-desc {
  max-width: 560px;
  margin-bottom: 12px;
}
.domain-code {
  padding: 2px 8px;
  border-radius: 3px;
  font-size: 13px;
}
.hint {
  margin: 0 0 12px;
  font-size: 12px;
  opacity: 0.65;
}
.cert-desc {
  max-width: 560px;
}
.status-tag {
  margin-left: 8px;
}
.expire-tag {
  margin-left: 8px;
}
.last-error {
  margin-top: 12px;
}
.actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  margin-top: 12px;
}
.wait-hint {
  margin: 8px 0 0;
  font-size: 12px;
  color: #d97600;
}
</style>
