<script setup lang="ts">
/**
 * 替换证书文件弹窗（PageSpec 4.2 v1.2 operList「替换证书文件」；certReplace——
 * imported 行专属，D-18）：粘贴新证书 PEM（≤1MiB 前端预校验）→ 提示备份/写回/
 * reload 与私钥范围声明 → 成功呈现新元数据（域名差异如实展示不阻断）；
 * CERT_008 → 内联「PEM 解析失败」；SITE_004 → 已替换但重载失败（全局语义）。
 */
import { ref } from 'vue'
import { useDialog, useMessage } from 'naive-ui'
import { certReplace } from '@/api/site'
import { ApiError } from '@/api/http'
import type { CertItem, CertReplaceResult } from '@/types/site'
import { CERT_ERROR_TEXT } from '@/types/site'
import { formatDateTime } from '@/utils/format'
import { siteActionError } from '../../errors'

const props = defineProps<{ cert: CertItem | null }>()
const show = defineModel<boolean>('show', { default: false })

const emit = defineEmits<{ done: [] }>()

const message = useMessage()
const dialog = useDialog()
const pem = ref('')
const replacing = ref(false)
const result = ref<CertReplaceResult | null>(null)

const PEM_LIMIT = 1 << 20

function reset(): void {
  pem.value = ''
  result.value = null
}

function onClose(v: boolean): void {
  show.value = v // X 关闭回写（defineModel 受控）
  if (!v) reset()
}

async function onSubmit(): Promise<void> {
  if (props.cert === null) return
  const text = pem.value.trim()
  if (text === '') {
    message.warning('请粘贴新证书 PEM（-----BEGIN CERTIFICATE----- 开头）')
    return
  }
  if (text.length > PEM_LIMIT) {
    message.error(CERT_ERROR_TEXT['CERT_008'])
    return
  }
  replacing.value = true
  try {
    result.value = await certReplace({ certId: props.cert.id, certPem: text })
    message.success('证书文件已替换并重载 nginx')
    emit('done')
  } catch (e) {
    if (e instanceof ApiError) {
      if (e.is('CERT_008')) {
        message.error('PEM 解析失败：内容不是有效的证书（CERT_008）')
        return
      }
      if (e.is('SITE_004')) {
        dialog.error({
          title: '文件已替换，但 nginx 重载失败',
          content: '证书内容已写入源站路径；请检查 nginx 环境后手动重试重载（SITE_004）。',
          positiveText: '知道了'
        })
        emit('done')
        return
      }
    }
    const info = siteActionError(e)
    if (info !== null) {
      if (info.output !== undefined) dialog.error({ title: info.text, content: info.output })
      else message.error(info.text)
      return
    }
    throw e
  } finally {
    replacing.value = false
  }
}
</script>

<template>
  <n-modal
    :show="show"
    preset="card"
    title="替换证书文件（存量导入证书）"
    style="width: 620px"
    :mask-closable="false"
    @update:show="onClose"
  >
    <n-alert type="warning" :show-icon="true" style="margin-bottom: 12px">
      将备份原文件后写回源站路径并重载 nginx。私钥不在替换范围（续期同 key 场景；更换私钥请经源码/文件管理通道）。
    </n-alert>
    <n-form label-placement="top">
      <n-form-item label="新证书 PEM">
        <n-input
          v-model:value="pem"
          type="textarea"
          :rows="10"
          placeholder="粘贴 PEM（-----BEGIN CERTIFICATE----- … -----END CERTIFICATE-----）"
          :disabled="replacing"
        />
      </n-form-item>
    </n-form>
    <n-descriptions v-if="result !== null" label-placement="left" :column="1" size="small" bordered style="margin-top: 8px">
      <n-descriptions-item label="新签发者">{{ result.issuer || '—' }}</n-descriptions-item>
      <n-descriptions-item label="新有效期至">{{ formatDateTime(result.notAfter ?? null) }}</n-descriptions-item>
      <n-descriptions-item label="域名">{{ result.domains.join(', ') }}</n-descriptions-item>
    </n-descriptions>
    <template #footer>
      <div class="footer">
        <n-button :disabled="replacing" @click="show = false">关闭</n-button>
        <n-button type="primary" :loading="replacing" @click="onSubmit">替换并重载</n-button>
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
