<script setup lang="ts">
/**
 * 部署证书弹窗（PageSpec 4.2；certDeploy）。
 * 目标站点 = sitesStore 投影中 enabled 站点；CERT_002 → 提示 data.currentStatus；
 * 成功 toast「已部署并重载生效」；SITE_003 回滚弹窗 / SITE_004 output 弹窗共用分流。
 */
import { computed, ref } from 'vue'
import { useDialog, useMessage } from 'naive-ui'
import type { FormInst, FormRules, SelectOption } from 'naive-ui'
import { certDeploy } from '@/api/site'
import { ApiError } from '@/api/http'
import { useSitesStore } from '@/stores/site'
import { CERT_STATUS, dictItem } from '@/types/enums'
import type { CertItem, SiteRollbackData } from '@/types/site'
import { isSiteRollbackData } from '@/types/site'
import { CERT_ERROR_TEXT } from '@/types/site'
import { siteActionError } from '../../errors'
import RollbackDiffModal from '../../components/RollbackDiffModal.vue'

const show = defineModel<boolean>('show', { default: false })

const props = defineProps<{
  /** 目标证书（null=关闭态） */
  cert: CertItem | null
}>()

const emit = defineEmits<{
  /** 部署成功（页面刷新证书清单 + 站点列表） */
  done: []
}>()

const sitesStore = useSitesStore()
const message = useMessage()
const dialog = useDialog()
const formRef = ref<FormInst | null>(null)
const deploying = ref(false)
const rollback = ref<SiteRollbackData | null>(null)

const form = ref<{ siteId: string | null }>({ siteId: null })

/** 仅 enabled 站点可部署（PageSpec 4.2） */
const siteOptions = computed<SelectOption[]>(() =>
  sitesStore.items
    .filter((s) => s.enabled === 'enabled')
    .map((s) => ({ label: s.serverNames.join(', '), value: s.siteId }))
)

const rules: FormRules = {
  siteId: [{ required: true, message: '请选择目标站点（仅已启用站点）', trigger: ['blur', 'change'] }]
}

function resetForm(): void {
  form.value = { siteId: null }
  formRef.value?.restoreValidation()
}

function onClose(v: boolean): void {
  if (!v) resetForm()
}

async function onSubmit(): Promise<void> {
  await formRef.value?.validate()
  if (props.cert === null || form.value.siteId === null) return
  deploying.value = true
  try {
    await certDeploy(props.cert.id, form.value.siteId)
    message.success('已部署并重载生效')
    show.value = false
    resetForm()
    emit('done')
  } catch (e) {
    if (e instanceof ApiError) {
      if (e.is('CERT_002')) {
        const raw = (e.data as Record<string, unknown> | null)?.currentStatus
        const current = typeof raw === 'string' ? dictItem(CERT_STATUS, raw).label : null
        message.error(
          `证书状态不允许部署（CERT_002）${current !== null ? `：当前状态为「${current}」，仅有效/即将到期可部署` : ''}`
        )
        return
      }
      if (e.is('SITE_003')) {
        if (isSiteRollbackData(e.data)) {
          rollback.value = e.data
          return
        }
      }
      const info = siteActionError(e)
      if (info !== null) {
        if (info.output !== undefined) dialog.error({ title: info.text, content: info.output })
        else message.error(info.text)
        return
      }
      const text = CERT_ERROR_TEXT[e.code]
      if (text !== undefined) message.error(text)
    }
  } finally {
    deploying.value = false
  }
}
</script>

<template>
  <n-modal
    :show="show"
    preset="card"
    title="部署证书"
    style="width: 480px"
    :mask-closable="false"
    @update:show="onClose"
  >
    <n-form ref="formRef" :model="form" :rules="rules" label-placement="top">
      <n-form-item label="证书">
        <span class="cert-label">{{ cert?.primaryDomain ?? '—' }}（SAN {{ cert?.domains.length ?? 0 }} 个域名）</span>
      </n-form-item>
      <n-form-item label="目标站点（仅已启用站点）" path="siteId">
        <n-select
          v-model:value="form.siteId"
          filterable
          clearable
          :options="siteOptions"
          :loading="sitesStore.listLoading"
          placeholder="选择站点"
        />
      </n-form-item>
    </n-form>
    <template #footer>
      <div class="footer">
        <n-button :disabled="deploying" @click="show = false">取消</n-button>
        <n-button type="primary" :loading="deploying" @click="onSubmit">部署并生效</n-button>
      </div>
    </template>
    <rollback-diff-modal :data="rollback" @close="rollback = null" />
  </n-modal>
</template>

<style scoped>
.footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
.cert-label {
  font-size: 13px;
}
</style>
