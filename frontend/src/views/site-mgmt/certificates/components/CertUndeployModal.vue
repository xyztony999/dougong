<script setup lang="ts">
/**
 * 解除部署弹窗（PageSpec 4.2；certUndeploy）。
 * 目标站点 = 该证书 referencedSites（D-11 实时推导）；文案「将恢复该站点为部署前原证书引用」
 * （D-11 恢复原值语义）；CERT_006 → 如实提示「该站点未部署此证书」。
 */
import { computed, ref } from 'vue'
import { useDialog, useMessage } from 'naive-ui'
import type { FormInst, FormRules, SelectOption } from 'naive-ui'
import { certUndeploy } from '@/api/site'
import { ApiError } from '@/api/http'
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
  /** 解除成功（页面刷新证书清单 + 站点列表） */
  done: []
}>()

const message = useMessage()
const dialog = useDialog()
const formRef = ref<FormInst | null>(null)
const undeploying = ref(false)
const rollback = ref<SiteRollbackData | null>(null)

const form = ref<{ siteId: string | null }>({ siteId: null })

const siteOptions = computed<SelectOption[]>(() =>
  (props.cert?.referencedSites ?? []).map((s) => ({
    label: `${s.serverNames.join(', ')}（${s.siteId}）`,
    value: s.siteId
  }))
)

const rules: FormRules = {
  siteId: [{ required: true, message: '请选择引用站点', trigger: ['blur', 'change'] }]
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
  undeploying.value = true
  try {
    await certUndeploy(props.cert.id, form.value.siteId)
    message.success('已解除部署，站点恢复部署前原证书引用')
    show.value = false
    resetForm()
    emit('done')
  } catch (e) {
    if (e instanceof ApiError) {
      if (e.is('CERT_006')) {
        message.error('该站点当前未部署此证书（CERT_006），请刷新列表确认引用关系')
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
    undeploying.value = false
  }
}
</script>

<template>
  <n-modal
    :show="show"
    preset="card"
    title="解除部署"
    style="width: 520px"
    :mask-closable="false"
    @update:show="onClose"
  >
    <n-form ref="formRef" :model="form" :rules="rules" label-placement="top">
      <n-form-item label="证书">
        <span class="cert-label">{{ cert?.primaryDomain ?? '—' }}</span>
      </n-form-item>
      <n-form-item label="引用站点" path="siteId">
        <n-select
          v-model:value="form.siteId"
          filterable
          clearable
          :options="siteOptions"
          placeholder="选择站点"
        />
      </n-form-item>
      <n-alert type="info" :show-icon="true">
        将恢复该站点为部署前原证书引用（接管不破坏：解除部署=回到面板接入前状态）。
      </n-alert>
    </n-form>
    <template #footer>
      <div class="footer">
        <n-button :disabled="undeploying" @click="show = false">取消</n-button>
        <n-button type="warning" :loading="undeploying" @click="onSubmit">解除部署</n-button>
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
