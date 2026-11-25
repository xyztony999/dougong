<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import type { FormInst, FormRules } from 'naive-ui'
import { ApiError } from '@/api/http'
import { certRequest, certRetryRenewal, certUpload, configUpdate } from '@/api/panel'
import { useAuthStore } from '@/stores/auth'
import { usePanelStore } from '@/stores/panel'
import {
  CONFIG_EFFECT_MODE,
  PANEL_CERT_SOURCE,
  PANEL_CERT_STATUS,
  dictItem
} from '@/types/enums'
import type { HttpsMode } from '@/types/enums'
import type { PanelConfigDetail } from '@/types/panel'
import { PANEL_ERROR_TEXT, certIssuerText } from '@/types/panel'
import { formatDateTime } from '@/utils/format'

const props = defineProps<{ detail: PanelConfigDetail }>()
const message = useMessage()
const authStore = useAuthStore()
const panelStore = usePanelStore()

interface ModeOption {
  value: HttpsMode
  title: string
  desc: string
}

const MODE_TILES: ModeOption[] = [
  { value: 'off', title: '关闭', desc: '面板走明文 HTTP' },
  { value: 'self_signed', title: '自签证书', desc: '浏览器会提示不安全' },
  { value: 'imported', title: '上传证书', desc: '粘贴 PEM 证书和私钥' },
  { value: 'acme', title: "Let's Encrypt", desc: '自动签发，需已绑域名' }
]

const selected = ref<HttpsMode>('off')

watch(
  () => props.detail.https.mode,
  (m) => {
    selected.value = m
  },
  { immediate: true }
)

const effectTag = computed(() => dictItem(CONFIG_EFFECT_MODE, props.detail.https.effectMode))
const cert = computed(() => props.detail.https.cert)
const certStatus = computed(() => panelStore.certStatusData)
const certLoading = computed(() => panelStore.certStatusLoading)
const statusCert = computed(() => certStatus.value?.cert ?? null)

const acmeDomain = computed(
  () => certStatus.value?.acme.domain ?? props.detail.bindDomain.value
)
const domainBound = computed(() => acmeDomain.value !== null && acmeDomain.value !== '')

const certSourceTag = computed(() => {
  const c = cert.value
  return c === null ? null : dictItem(PANEL_CERT_SOURCE, c.source)
})

const certStatusTag = computed(() => {
  const c = statusCert.value
  if (c === null || c.source !== 'acme' || c.status === null) return null
  return dictItem(PANEL_CERT_STATUS, c.status)
})

const validityText = computed(() => {
  const c = cert.value
  if (c === null) return '—'
  return `${formatDateTime(c.notBefore)} ~ ${formatDateTime(c.notAfter)}`
})

const expiringSoon = computed(() => {
  const na = cert.value?.notAfter ?? null
  if (na === null) return false
  const t = new Date(na).getTime()
  return !Number.isNaN(t) && t - Date.now() < 30 * 24 * 60 * 60 * 1000
})

const lastError = computed(() => {
  const c = statusCert.value
  if (c === null || c.status !== 'renew_failed') return null
  return c.lastError !== null && c.lastError !== '' ? c.lastError : null
})

const issuerText = computed(() => certIssuerText(cert.value?.issuer ?? statusCert.value?.issuer))

function hasUsableAcmeCert(): boolean {
  const c = cert.value
  if (c === null || c.source !== 'acme' || c.notAfter === null) return false
  const t = new Date(c.notAfter).getTime()
  return !Number.isNaN(t) && t > Date.now()
}

const savingMode = ref(false)

const hasImportedCert = computed(() => cert.value !== null && cert.value.source === 'imported')
const canEnableImported = computed(
  () => hasImportedCert.value && props.detail.https.mode !== 'imported'
)
const canEnableAcme = computed(
  () => hasUsableAcmeCert() && props.detail.https.mode !== 'acme'
)
const showAcmeRequest = computed(() => !hasUsableAcmeCert())
const replacingImport = ref(false)

watch(selected, () => {
  replacingImport.value = false
})

async function applyMode(target: HttpsMode): Promise<void> {
  if (target === 'imported' && !hasImportedCert.value) {
    message.error(PANEL_ERROR_TEXT['PANEL_003'] ?? '请先导入证书与私钥')
    return
  }
  if (target === 'acme' && !hasUsableAcmeCert()) {
    message.error('请先申请 Let\'s Encrypt 证书')
    return
  }
  savingMode.value = true
  try {
    await configUpdate({ httpsMode: target })
    if (target === 'self_signed') message.success('已启用自签证书')
    else if (target === 'off') message.success('已关闭 HTTPS')
    else if (target === 'imported') message.success('已启用导入的证书')
    else message.success('已启用 Let\'s Encrypt')
    await panelStore.loadConfig()
  } catch {
    // 拦截器已提示
  } finally {
    savingMode.value = false
  }
}

const importFormRef = ref<FormInst | null>(null)
interface ImportFormData {
  certPem: string
  keyPem: string
}
const importForm = ref<ImportFormData>({ certPem: '', keyPem: '' })
const importing = ref(false)
const keyRevealed = ref(false)

const importRules: FormRules = {
  certPem: { required: true, message: '请粘贴证书 PEM 内容', trigger: 'blur' },
  keyPem: { required: true, message: '请粘贴私钥 PEM 内容', trigger: 'blur' }
}

async function onImport(): Promise<void> {
  try {
    await importFormRef.value?.validate()
  } catch {
    return
  }
  importing.value = true
  try {
    await certUpload({ certPem: importForm.value.certPem, keyPem: importForm.value.keyPem })
    await configUpdate({ httpsMode: 'imported' })
    message.success('证书已导入并启用')
    importForm.value = { certPem: '', keyPem: '' }
    importFormRef.value?.restoreValidation()
    await Promise.all([panelStore.loadConfig(), panelStore.loadCertStatus()])
  } catch {
    // PANEL_002 由拦截器提示
  } finally {
    importing.value = false
  }
}

function dataString(err: ApiError, key: string): string | null {
  if (typeof err.data !== 'object' || err.data === null) return null
  const v = (err.data as Record<string, unknown>)[key]
  return typeof v === 'string' && v !== '' ? v : null
}

function onActionError(err: unknown): void {
  if (err instanceof ApiError) {
    if (err.is('PANEL_009')) {
      const reason = dataString(err, 'reason')
      message.error(`签发失败${reason !== null ? `：${reason}` : ''}`)
      return
    }
    if (err.is('PANEL_010')) {
      message.error('当前状态无需重试')
      return
    }
    if (err.is('PANEL_011')) {
      const notAfter = dataString(err, 'notAfter')
      message.error(
        `已有有效证书（${notAfter !== null ? formatDateTime(notAfter) : ''} 到期），续期请点重试`
      )
      return
    }
    return
  }
  message.error('操作失败，请稍后重试')
}

const requesting = ref(false)
const WAIT_HINT = '正在签发，经 nginx 验证域名，最长约 2 分钟'

async function onRequest(): Promise<void> {
  if (requesting.value) return
  requesting.value = true
  try {
    await certRequest()
    message.success('证书已签发，HTTPS 已切换')
    await Promise.all([panelStore.loadCertStatus(), panelStore.loadConfig()])
  } catch (err) {
    onActionError(err)
  } finally {
    requesting.value = false
  }
}

const retrying = ref(false)
const retryVisible = computed(
  () =>
    authStore.isAdmin &&
    statusCert.value !== null &&
    statusCert.value.source === 'acme' &&
    statusCert.value.status === 'renew_failed'
)

async function onRetry(): Promise<void> {
  if (retrying.value) return
  retrying.value = true
  try {
    await certRetryRenewal()
    message.success('续期成功')
    await Promise.all([panelStore.loadCertStatus(), panelStore.loadConfig()])
  } catch (err) {
    onActionError(err)
  } finally {
    retrying.value = false
  }
}

function gotoBindCard(): void {
  document.getElementById('panel-bind-domain-card')?.scrollIntoView({ behavior: 'smooth' })
}

function onSelectTile(mode: HttpsMode): void {
  if (!authStore.isAdmin) return
  selected.value = mode
}
</script>

<template>
  <n-card title="HTTPS">
    <template #header-extra>
      <n-tag :type="effectTag.tag" size="small" :bordered="false">{{ effectTag.label }}</n-tag>
    </template>

    <div class="status-band">
      <div class="status-kicker">当前证书</div>
      <div v-if="cert === null && !certLoading" class="status-empty">未配置</div>
      <div v-else class="status-grid">
        <div>
          <span class="k">来源</span>
          <n-tag v-if="certSourceTag !== null" :type="certSourceTag.tag" size="small" :bordered="false">
            {{ certSourceTag.label }}
          </n-tag>
          <span v-else>—</span>
          <n-tag
            v-if="certStatusTag !== null"
            :type="certStatusTag.tag"
            size="small"
            :bordered="false"
            class="ml"
          >
            {{ certStatusTag.label }}
          </n-tag>
        </div>
        <div>
          <span class="k">颁发者</span>
          <span>{{ issuerText }}</span>
        </div>
        <div>
          <span class="k">有效期</span>
          <span>{{ validityText }}</span>
          <n-tag v-if="expiringSoon" type="warning" size="small" :bordered="false" class="ml">
            30 天内到期
          </n-tag>
        </div>
      </div>
    </div>

    <div class="tiles" role="listbox" :aria-disabled="!authStore.isAdmin">
      <button
        v-for="tile in MODE_TILES"
        :key="tile.value"
        type="button"
        class="tile"
        :class="{ active: selected === tile.value, current: detail.https.mode === tile.value }"
        :disabled="!authStore.isAdmin"
        @click="onSelectTile(tile.value)"
      >
        <span class="tile-title">{{ tile.title }}</span>
        <span class="tile-desc">{{ tile.desc }}</span>
        <span v-if="detail.https.mode === tile.value" class="tile-now">正在使用</span>
      </button>
    </div>

    <div v-if="selected === 'off'" class="panel">
      <p class="hint">关闭后面板以 HTTP 明文提供服务。</p>
      <n-button
        v-if="authStore.isAdmin"
        :loading="savingMode"
        :disabled="detail.https.mode === 'off'"
        @click="applyMode('off')"
      >
        关闭 HTTPS
      </n-button>
    </div>

    <div v-else-if="selected === 'self_signed'" class="panel">
      <p class="hint">保存后立即生成自签证书。浏览器会提示不安全，适合内网或临时使用。</p>
      <n-button
        v-if="authStore.isAdmin"
        type="primary"
        :loading="savingMode"
        @click="applyMode('self_signed')"
      >
        {{ detail.https.mode === 'self_signed' ? '重新生成自签证书' : '启用自签证书' }}
      </n-button>
    </div>

    <div v-else-if="selected === 'imported'" class="panel">
      <template v-if="hasImportedCert">
        <p class="hint">已有导入的证书{{ issuerText !== '—' ? `（${issuerText}）` : '' }}。</p>
        <div class="import-actions">
          <n-button
            v-if="authStore.isAdmin && canEnableImported"
            type="primary"
            :loading="savingMode"
            @click="applyMode('imported')"
          >
            使用这张证书
          </n-button>
          <n-button
            v-if="authStore.isAdmin && !replacingImport"
            size="small"
            @click="replacingImport = true"
          >
            更换证书
          </n-button>
        </div>
      </template>
      <n-form
        v-if="authStore.isAdmin && (!hasImportedCert || replacingImport)"
        ref="importFormRef"
        :model="importForm"
        :rules="importRules"
        label-placement="top"
      >
        <n-form-item label="证书 PEM" path="certPem">
          <n-input
            v-model:value="importForm.certPem"
            type="textarea"
            :rows="5"
            placeholder="-----BEGIN CERTIFICATE-----&#10;…"
            class="mono"
          />
        </n-form-item>
        <n-form-item label="私钥 PEM" path="keyPem">
          <n-input
            v-model:value="importForm.keyPem"
            type="textarea"
            :rows="5"
            placeholder="-----BEGIN PRIVATE KEY-----&#10;…"
            :class="['mono', { 'key-masked': !keyRevealed }]"
          />
          <n-button text size="tiny" class="key-toggle" @click="keyRevealed = !keyRevealed">
            {{ keyRevealed ? '隐藏私钥' : '显示私钥' }}
          </n-button>
        </n-form-item>
        <n-button type="primary" :loading="importing" @click="onImport">导入并启用</n-button>
      </n-form>
      <p v-if="!hasImportedCert || replacingImport" class="hint">私钥导入后不会再显示。成功后输入框会清空。</p>
    </div>

    <div v-else class="panel">
      <n-alert v-if="!domainBound" type="info" :closable="false">
        申请证书前先绑定访问域名
        <n-button text type="primary" size="small" class="goto-bind" @click="gotoBindCard">
          前往绑定
        </n-button>
      </n-alert>
      <div v-else class="acme-meta">
        <div><span class="k">域名</span><span class="mono">{{ acmeDomain }}</span></div>
        <div><span class="k">验证</span>HTTP-01（走本机 nginx 80 端口）</div>
        <div><span class="k">续期</span>到期前 30 天自动续，失败后可手动重试</div>
      </div>
      <n-alert v-if="lastError !== null" type="error" title="上次续期失败" :closable="false">
        {{ lastError }}
      </n-alert>
      <div v-if="authStore.isAdmin" class="acme-actions">
        <n-button
          v-if="retryVisible"
          type="warning"
          ghost
          :loading="retrying"
          :disabled="requesting"
          @click="onRetry"
        >
          重试续期
        </n-button>
        <n-button
          v-if="canEnableAcme"
          type="primary"
          :loading="savingMode"
          :disabled="requesting || retrying"
          @click="applyMode('acme')"
        >
          启用 Let's Encrypt
        </n-button>
        <n-button
          v-if="showAcmeRequest"
          type="primary"
          :loading="requesting"
          :disabled="!domainBound || retrying"
          @click="onRequest"
        >
          申请证书
        </n-button>
      </div>
      <p v-if="requesting || retrying" class="wait-hint">{{ WAIT_HINT }}</p>
    </div>
  </n-card>
</template>

<style scoped>
.status-band {
  padding: 14px 16px;
  margin-bottom: 16px;
  border-radius: 8px;
  border: 1px solid var(--n-border-color);
  background: var(--n-color-embedded, rgba(127, 127, 127, 0.08));
}
.status-kicker {
  font-size: 12px;
  opacity: 0.55;
  margin-bottom: 8px;
}
.status-empty {
  font-size: 13px;
  opacity: 0.7;
}
.status-grid {
  display: grid;
  gap: 8px;
  font-size: 13px;
}
.k {
  display: inline-block;
  width: 52px;
  opacity: 0.55;
  font-size: 12px;
}
.ml {
  margin-left: 8px;
}
.tiles {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  margin-bottom: 16px;
}
.tile {
  text-align: left;
  padding: 12px 14px;
  border-radius: 8px;
  border: 1px solid var(--n-border-color);
  background: transparent;
  color: inherit;
  cursor: pointer;
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-height: 76px;
}
.tile:disabled {
  cursor: default;
  opacity: 0.7;
}
.tile.active {
  border-color: var(--n-primary-color, #18a058);
  box-shadow: inset 0 0 0 1px var(--n-primary-color, #18a058);
}
.tile-title {
  font-weight: 600;
  font-size: 14px;
}
.tile-desc {
  font-size: 12px;
  opacity: 0.65;
  line-height: 1.4;
}
.tile-now {
  font-size: 11px;
  opacity: 0.55;
}
.panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.hint {
  margin: 0;
  font-size: 12px;
  opacity: 0.65;
}
.mono :deep(textarea),
.mono :deep(input),
span.mono {
  font-family: ui-monospace, SFMono-Regular, Consolas, 'Courier New', monospace;
  font-size: 12px;
}
.key-masked :deep(textarea) {
  -webkit-text-security: disc;
}
.key-toggle {
  margin-top: 4px;
}
.acme-meta {
  display: grid;
  gap: 8px;
  font-size: 13px;
}
.import-actions,
.acme-actions {
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
}
.acme-actions {
  justify-content: flex-end;
}
.goto-bind {
  margin-left: 8px;
}
.wait-hint {
  margin: 0;
  font-size: 12px;
  color: var(--n-warning-color, #d97600);
}
</style>
