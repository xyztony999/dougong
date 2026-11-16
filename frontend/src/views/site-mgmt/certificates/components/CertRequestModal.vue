<script setup lang="ts">
/**
 * 申请证书弹窗（PageSpec 4.2；certRequest——单接口覆盖 HTTP-01/TLS-ALPN 两菜单）。
 * 站点 select 数据源 = sitesStore 投影（域名集合即 SAN）；提交同步终态 ≤120s（D-09）
 * ——按钮 loading + 「正在向 ACME 服务申请，最长约 2 分钟」；
 * CERT_003 → 同域名证书已存在 + existingCertId「查看已有证书」引导；
 * CERT_004 → 失败原因展示并提示可重试。
 */
import { computed, ref } from 'vue'
import { useDialog, useMessage } from 'naive-ui'
import type { FormInst, FormRules, SelectOption } from 'naive-ui'
import { certRequest } from '@/api/site'
import { ApiError } from '@/api/http'
import { useSitesStore } from '@/stores/site'
import type { AcmeChallenge } from '@/types/enums'
import { CERT_ERROR_TEXT, fieldOfString } from '@/types/site'

const show = defineModel<boolean>('show', { default: false })

const emit = defineEmits<{
  /** 申请成功（页面刷新证书清单 + 站点列表证书状态列） */
  done: []
  /** CERT_003 查看已有证书引导 */
  'view-cert': [certId: number]
}>()

const sitesStore = useSitesStore()
const message = useMessage()
const dialog = useDialog()
const formRef = ref<FormInst | null>(null)
const requesting = ref(false)

const form = ref<{ siteId: string | null; challenge: AcmeChallenge }>({
  siteId: null,
  challenge: 'http_01'
})

const siteOptions = computed<SelectOption[]>(() =>
  sitesStore.items.map((s) => ({
    label: s.serverNames.join(', '),
    value: s.siteId
  }))
)

/** 选项描述文案（PageSpec 4.2：两挑战方式前提条件） */
const challengeOptions: SelectOption[] = [
  { label: 'HTTP-01（需域名可经 80 端口 HTTP 访问）', value: 'http_01' },
  { label: 'TLS-ALPN（需 443 端口 HTTPS，无需 80）', value: 'tls_alpn' }
]

const rules: FormRules = {
  siteId: [{ required: true, message: '请选择站点（域名集合即证书 SAN）', trigger: ['blur', 'change'] }],
  challenge: [{ required: true, message: '请选择挑战方式', trigger: ['blur', 'change'] }]
}

function resetForm(): void {
  form.value = { siteId: null, challenge: 'http_01' }
  formRef.value?.restoreValidation()
}

function onClose(v: boolean): void {
  if (!v) resetForm()
}

async function onSubmit(): Promise<void> {
  await formRef.value?.validate()
  if (form.value.siteId === null) return
  requesting.value = true
  try {
    await certRequest(form.value.siteId, form.value.challenge)
    message.success('证书申请成功，已签发并生效')
    show.value = false
    resetForm()
    emit('done')
  } catch (e) {
    if (e instanceof ApiError) {
      if (e.is('CERT_003')) {
        const existing = (e.data as Record<string, unknown> | null)?.existingCertId
        const existingCertId = typeof existing === 'number' ? existing : null
        dialog.error({
          title: '证书已存在',
          content: '同域名集合已有生效中证书（CERT_003），可直接部署已有证书。',
          positiveText: existingCertId !== null ? '查看已有证书' : '关闭',
          negativeText: '关闭',
          onPositiveClick: () => {
            if (existingCertId !== null) {
              show.value = false
              emit('view-cert', existingCertId)
            }
          }
        })
        return
      }
      if (e.is('CERT_004')) {
        const reason = fieldOfString(e.data, 'reason') ?? '（服务端未返回原因）'
        dialog.error({
          title: CERT_ERROR_TEXT['CERT_004'],
          content: `${reason}\n（证书已转入「续期失败」状态，修正挑战前提后可重试）`
        })
        emit('done') // 刷新列表呈现 renew_failed 行与 lastError
        return
      }
      const text = CERT_ERROR_TEXT[e.code]
      if (text !== undefined) message.error(text)
    }
  } finally {
    requesting.value = false
  }
}
</script>

<template>
  <n-modal
    :show="show"
    preset="card"
    title="申请证书（ACME）"
    style="width: 480px"
    :mask-closable="false"
    @update:show="onClose"
  >
    <n-form ref="formRef" :model="form" :rules="rules" label-placement="top">
      <n-form-item label="站点（域名集合即证书 SAN）" path="siteId">
        <n-select
          v-model:value="form.siteId"
          filterable
          clearable
          :options="siteOptions"
          :loading="sitesStore.listLoading"
          placeholder="选择站点"
        />
      </n-form-item>
      <n-form-item label="挑战方式" path="challenge">
        <n-select v-model:value="form.challenge" :options="challengeOptions" />
      </n-form-item>
      <n-alert v-if="requesting" type="info" :show-icon="true">
        正在向 ACME 服务申请，最长约 2 分钟（同步等待签发结果，请勿关闭弹窗）。
      </n-alert>
    </n-form>
    <template #footer>
      <div class="footer">
        <n-button :disabled="requesting" @click="show = false">取消</n-button>
        <n-button type="primary" :loading="requesting" @click="onSubmit">申请</n-button>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
</style>
