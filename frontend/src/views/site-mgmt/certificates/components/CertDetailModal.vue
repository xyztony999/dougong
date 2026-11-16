<script setup lang="ts">
/**
 * 证书只读详情 Modal（PageSpec 4.2/第 6 节）：数据源=行数据（api 无证书详情接口——不虚构请求）。
 */
import { computed } from 'vue'
import { ACME_CHALLENGE, CERT_SOURCE, CERT_STATUS, dictItem } from '@/types/enums'
import type { CertItem } from '@/types/site'
import { formatDateTime } from '@/utils/format'

const props = defineProps<{
  /** 行数据（null=关闭态） */
  cert: CertItem | null
}>()

const emit = defineEmits<{
  close: []
}>()

const show = computed<boolean>({
  get: () => props.cert !== null,
  set: (v) => {
    if (!v) emit('close')
  }
})

const sourceDict = computed(() =>
  props.cert !== null ? dictItem(CERT_SOURCE, props.cert.source) : null
)
const statusDict = computed(() =>
  props.cert !== null ? dictItem(CERT_STATUS, props.cert.status) : null
)
const challengeDict = computed(() =>
  props.cert?.challenge != null ? dictItem(ACME_CHALLENGE, props.cert.challenge) : null
)
const validRange = computed(() => {
  const c = props.cert
  if (c === null) return '—'
  const from = c.notBefore ?? null
  const to = c.notAfter ?? null
  if (from === null && to === null) return '—'
  return `${formatDateTime(from)} ~ ${formatDateTime(to)}`
})
const referencedText = computed(() => {
  const c = props.cert
  if (c === null || c.referencedSites.length === 0) return '无引用站点'
  return c.referencedSites.map((s) => s.serverNames.join(', ')).join('；')
})
</script>

<template>
  <n-modal v-model:show="show" preset="card" title="证书详情" style="width: 560px">
    <n-descriptions v-if="cert !== null" :column="1" label-placement="left" bordered size="small">
      <n-descriptions-item label="主域名">{{ cert.primaryDomain }}</n-descriptions-item>
      <n-descriptions-item label="域名（SAN）">{{ cert.domains.join(', ') }}</n-descriptions-item>
      <n-descriptions-item label="签发者">{{ cert.issuer !== '' ? cert.issuer : '—' }}</n-descriptions-item>
      <n-descriptions-item label="来源">
        <n-tag v-if="sourceDict !== null" :type="sourceDict.tag" size="small" :bordered="false">
          {{ sourceDict.label }}
        </n-tag>
      </n-descriptions-item>
      <n-descriptions-item label="挑战方式">
        <template v-if="challengeDict !== null">{{ challengeDict.label }}</template>
        <span v-else class="muted">—（导入证书无挑战方式）</span>
      </n-descriptions-item>
      <n-descriptions-item label="状态">
        <n-tag v-if="statusDict !== null" :type="statusDict.tag" size="small" :bordered="false">
          {{ statusDict.label }}
        </n-tag>
      </n-descriptions-item>
      <n-descriptions-item label="有效期">{{ validRange }}</n-descriptions-item>
      <n-descriptions-item v-if="cert.originPath != null" label="源站路径">
        <span class="mono">{{ cert.originPath }}</span>
      </n-descriptions-item>
      <n-descriptions-item v-if="cert.lastError != null" label="最近失败原因">
        <span class="mono">{{ cert.lastError }}</span>
      </n-descriptions-item>
      <n-descriptions-item label="引用站点">{{ referencedText }}</n-descriptions-item>
      <n-descriptions-item label="入册时间">{{ formatDateTime(cert.createdAt) }}</n-descriptions-item>
    </n-descriptions>
    <template #footer>
      <div class="footer">
        <n-button @click="emit('close')">关闭</n-button>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.footer {
  display: flex;
  justify-content: flex-end;
}
.muted {
  opacity: 0.55;
}
.mono {
  font-family: Consolas, Menlo, monospace;
  font-size: 12px;
  word-break: break-all;
}
</style>
