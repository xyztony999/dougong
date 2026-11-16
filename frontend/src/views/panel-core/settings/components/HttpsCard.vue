<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import type { FormInst, FormRules, SelectOption } from 'naive-ui'
import { certUpload, configUpdate } from '@/api/panel'
import { useAuthStore } from '@/stores/auth'
import { usePanelStore } from '@/stores/panel'
import { CONFIG_EFFECT_MODE, HTTPS_MODE, PANEL_CERT_SOURCE, dictItem } from '@/types/enums'
import type { HttpsMode } from '@/types/enums'
import type { PanelConfigDetail } from '@/types/panel'
import { PANEL_ERROR_TEXT } from '@/types/panel'
import { formatDateTime } from '@/utils/format'

const props = defineProps<{ detail: PanelConfigDetail }>()
const message = useMessage()
const authStore = useAuthStore()
const panelStore = usePanelStore()

interface ModeFormData {
  mode: HttpsMode | null
}

const modeForm = ref<ModeFormData>({ mode: null })

watch(
  () => props.detail,
  (d) => {
    modeForm.value = { mode: d.https.mode }
  },
  { immediate: true }
)

const mode = computed(() => modeForm.value.mode)

const modeOptions: SelectOption[] = (Object.keys(HTTPS_MODE) as HttpsMode[]).map((value) => ({
  label: HTTPS_MODE[value].label,
  value
}))

const effectTag = computed(() => dictItem(CONFIG_EFFECT_MODE, props.detail.https.effectMode))
const cert = computed(() => props.detail.https.cert)
const showImportArea = computed(() => mode.value === 'imported' && authStore.isAdmin)

const modeHint = computed(() => {
  switch (mode.value) {
    case 'self_signed':
      return '保存后系统自动生成自签证书'
    case 'imported':
      return '需先导入证书与私钥，再保存切换为导入模式'
    default:
      return '关闭 HTTPS 后，面板以 HTTP 明文方式提供服务'
  }
})

const certSourceTag = computed(() => {
  const c = cert.value
  return c === null ? null : dictItem(PANEL_CERT_SOURCE, c.source)
})

const validityText = computed(() => {
  const c = cert.value
  if (c === null) return '—'
  return `${formatDateTime(c.notBefore)} ~ ${formatDateTime(c.notAfter)}`
})

/** notAfter 30 天内 → 橙色临近过期提示 */
const expiringSoon = computed(() => {
  const na = cert.value?.notAfter ?? null
  if (na === null) return false
  const t = new Date(na).getTime()
  return !Number.isNaN(t) && t - Date.now() < 30 * 24 * 60 * 60 * 1000
})

const savingMode = ref(false)

/** 保存模式（PANEL_003 前置：切 imported 需已有证书；服务端双保险） */
async function onSaveMode(): Promise<void> {
  const target = mode.value ?? 'off'
  if (target === 'imported' && cert.value === null) {
    message.error(PANEL_ERROR_TEXT['PANEL_003'] ?? '请先导入证书与私钥')
    return
  }
  savingMode.value = true
  try {
    await configUpdate({ httpsMode: target })
    if (target === 'self_signed') message.success('已保存，自签证书已生成')
    else message.success('已保存')
    await panelStore.loadConfig()
  } catch {
    // 错误已由拦截器统一提示（PANEL_002/003 等）
  } finally {
    savingMode.value = false
  }
}

/** ---- 导入证书（certUpload；I-P06：keyPem 零回显，成功后双清空） ---- */

const importFormRef = ref<FormInst | null>(null)
interface ImportFormData {
  certPem: string
  keyPem: string
}
const importForm = ref<ImportFormData>({ certPem: '', keyPem: '' })
const importing = ref(false)

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
    message.success('证书已导入并记入审计')
    importForm.value = { certPem: '', keyPem: '' }
    importFormRef.value?.restoreValidation()
    await panelStore.loadConfig()
  } catch {
    // PANEL_002 已由拦截器统一提示
  } finally {
    importing.value = false
  }
}
</script>

<template>
  <n-card title="HTTPS 设置">
    <template #header-extra>
      <n-tag :type="effectTag.tag" size="small" :bordered="false">{{ effectTag.label }}</n-tag>
    </template>

    <n-form
      :model="modeForm"
      label-placement="top"
      :disabled="!authStore.isAdmin"
      style="max-width: 420px"
    >
      <n-form-item label="HTTPS 模式" path="mode">
        <n-select v-model:value="modeForm.mode" :options="modeOptions" />
      </n-form-item>
    </n-form>
    <p class="hint">{{ modeHint }}</p>

    <!-- 证书导入区：仅 imported 模式且 admin 展开（readonly 不渲染，负空间） -->
    <n-form
      v-if="showImportArea"
      ref="importFormRef"
      :model="importForm"
      :rules="importRules"
      label-placement="top"
      class="import-area"
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
      <!-- 粘贴态遮罩：Naive UI textarea 无遮罩能力，私钥用 password 输入承载（tasks 偏差 1） -->
      <n-form-item label="私钥 PEM" path="keyPem">
        <n-input
          v-model:value="importForm.keyPem"
          type="password"
          show-password-on="click"
          placeholder="-----BEGIN PRIVATE KEY-----…"
          class="mono"
        />
      </n-form-item>
      <div class="actions">
        <n-button type="primary" :loading="importing" @click="onImport">导入证书</n-button>
      </div>
      <p class="hint">导入后证书摘要下方可见；私钥不会回显，成功后输入框自动清空。</p>
    </n-form>

    <!-- 当前证书摘要（来源/颁发者/有效期；无「查看私钥」入口，api 3.3 脱敏） -->
    <n-descriptions title="当前证书" label-placement="left" :column="1" bordered size="small" class="cert-desc">
      <n-descriptions-item label="来源">
        <n-tag
          v-if="certSourceTag !== null"
          :type="certSourceTag.tag"
          size="small"
          :bordered="false"
        >
          {{ certSourceTag.label }}
        </n-tag>
        <span v-else>—</span>
      </n-descriptions-item>
      <n-descriptions-item label="颁发者">{{ cert?.issuer ?? '—' }}</n-descriptions-item>
      <n-descriptions-item label="有效期">
        <span>{{ validityText }}</span>
        <n-tag v-if="expiringSoon" type="warning" size="small" :bordered="false" class="expire-tag">
          30 天内到期
        </n-tag>
      </n-descriptions-item>
    </n-descriptions>

    <div v-if="authStore.isAdmin" class="actions card-actions">
      <n-button type="primary" :loading="savingMode" @click="onSaveMode">保存</n-button>
    </div>
  </n-card>
</template>

<style scoped>
.hint {
  margin: 8px 0 12px;
  font-size: 12px;
  opacity: 0.65;
}
.mono :deep(textarea),
.mono :deep(input) {
  font-family: ui-monospace, SFMono-Regular, Consolas, 'Courier New', monospace;
  font-size: 12px;
}
.import-area {
  padding: 12px;
  border-radius: 4px;
  margin-bottom: 12px;
}
.cert-desc {
  max-width: 560px;
}
.expire-tag {
  margin-left: 8px;
}
.actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
.card-actions {
  margin-top: 12px;
}
</style>
