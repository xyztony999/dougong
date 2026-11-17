<script setup lang="ts">
/**
 * 申请证书弹窗（PageSpec 4.2 v1.2；certRequest——单接口覆盖三挑战方式）。
 * - 文件验证 HTTP-01（⚠️A19/D-19：验证文件由面板自动放置——优先网站根目录，
 *   自动复用/临时注入挑战路径；需域名可经 80 端口 HTTP 访问）；
 * - TLS-ALPN（需 443 端口 HTTPS，无需 80）；
 * - DNS-01（⚠️A20/D-20：面板经 DNS 服务商 API 自动添加/验证/清理 TXT 记录，
 *   全程自动；适用 80/443 不可达场景与泛域名证书，需已配置 DNS 凭证）。
 * 域名来源二选一（v1.2）：选择站点（serverNames 即 SAN）/ 手动输入域名
 * （仅 dns_01，支持 *.domain 泛域名，CERT_010）。
 * 提交同步终态 ≤120s（D-09）；CERT_003 → 查看已有证书引导；CERT_004 → 失败
 * 原因展示；CERT_009 → 凭证缺失引导（去配置 DNS 凭证）。
 */
import { computed, ref, watch } from 'vue'
import { useDialog, useMessage } from 'naive-ui'
import type { FormInst, FormRules, SelectOption } from 'naive-ui'
import { certRequest, dnsCredList } from '@/api/site'
import { ApiError } from '@/api/http'
import { useSitesStore } from '@/stores/site'
import type { AcmeChallenge } from '@/types/enums'
import { CERT_ERROR_TEXT, fieldOfString } from '@/types/site'
import type { DnsCredentialItem } from '@/types/site'

const show = defineModel<boolean>('show', { default: false })

const emit = defineEmits<{
  /** 申请成功（页面刷新证书清单 + 站点列表证书状态列） */
  done: []
  /** CERT_003 查看已有证书引导 */
  'view-cert': [certId: number]
  /** CERT_009 / dns_01 无凭证 → 打开 DNS 凭证管理弹窗 */
  'manage-credentials': []
}>()

const sitesStore = useSitesStore()
const message = useMessage()
const dialog = useDialog()
const formRef = ref<FormInst | null>(null)
const requesting = ref(false)

type DomainMode = 'site' | 'manual'
const form = ref<{
  domainMode: DomainMode
  siteId: string | null
  domains: string[]
  challenge: AcmeChallenge
  dnsCredentialId: number | null
}>({ domainMode: 'site', siteId: null, domains: [], challenge: 'http_01', dnsCredentialId: null })

/** 站点选项（v1.2 聚合条目：寻址经 blocks[0].siteId——组内首块，D-17） */
const siteOptions = computed<SelectOption[]>(() =>
  sitesStore.items.map((s) => ({
    label: s.serverNames.join(', '),
    value: s.blocks[0]?.siteId ?? ''
  }))
)

/** DNS 凭证清单（challenge=dns_01 时拉取；空则引导配置） */
const credentials = ref<DnsCredentialItem[]>([])
const credLoading = ref(false)

async function loadCredentials(): Promise<void> {
  credLoading.value = true
  try {
    credentials.value = (await dnsCredList()).items
  } catch {
    credentials.value = [] // admin 专属接口失败如实降级为空清单
  } finally {
    credLoading.value = false
  }
}

watch(
  () => form.value.challenge,
  (c) => {
    if (c === 'dns_01') {
      if (credentials.value.length === 0) void loadCredentials()
    } else {
      form.value.domainMode = 'site'
      form.value.domains = []
      form.value.dnsCredentialId = null
    }
  }
)
watch(show, (v) => {
  if (v) void sitesStore.loadList()
})

/** 挑战方式选项（PageSpec 4.2 v1.2 文案） */
const challengeOptions: SelectOption[] = [
  {
    label: '文件验证 HTTP-01（验证文件由面板自动放置；需域名可经 80 端口 HTTP 访问）',
    value: 'http_01'
  },
  { label: 'TLS-ALPN（需 443 端口 HTTPS，无需 80）', value: 'tls_alpn' },
  {
    label: 'DNS-01（面板经 DNS 服务商 API 自动管理 TXT 记录；适用 80/443 不可达与泛域名）',
    value: 'dns_01'
  }
]

const credOptions = computed<SelectOption[]>(() =>
  credentials.value.map((c) => ({ label: `${c.name}（${c.provider}）`, value: c.id }))
)

const isDns = computed(() => form.value.challenge === 'dns_01')

const rules = computed<FormRules>(() => ({
  siteId:
    !isDns.value || form.value.domainMode === 'site'
      ? [{ required: true, message: '请选择站点（域名集合即证书 SAN）', trigger: ['blur', 'change'] }]
      : [],
  domains:
    isDns.value && form.value.domainMode === 'manual'
      ? [
          {
            required: true,
            type: 'array' as const,
            message: '请输入至少一个域名（支持 *.domain 泛域名）',
            trigger: ['blur', 'change']
          }
        ]
      : [],
  dnsCredentialId: isDns.value
    ? [{ required: true, message: 'DNS-01 需选择 DNS 服务商凭证', trigger: ['blur', 'change'] }]
    : []
}))

function resetForm(): void {
  form.value = {
    domainMode: 'site',
    siteId: null,
    domains: [],
    challenge: 'http_01',
    dnsCredentialId: null
  }
  formRef.value?.restoreValidation()
}

function onClose(v: boolean): void {
  // X/遮罩关闭必须回写 show（defineModel 受控）——否则弹窗无法关闭（实机反馈）。
  show.value = v
  if (!v) resetForm()
}

async function onSubmit(): Promise<void> {
  await formRef.value?.validate()
  const f = form.value
  if (isDns.value && f.domainMode === 'manual' && f.domains.length === 0) return
  if (isDns.value && f.dnsCredentialId === null) return
  if ((!isDns.value || f.domainMode === 'site') && f.siteId === null) return
  requesting.value = true
  try {
    await certRequest(
      isDns.value && f.domainMode === 'manual'
        ? { domains: f.domains, challenge: f.challenge, dnsCredentialId: f.dnsCredentialId ?? undefined }
        : { siteId: f.siteId ?? undefined, challenge: f.challenge }
    )
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
          content:
            '该域名已有生效中的 ACME 证书或进行中的申请（CERT_003）——可直接部署已有证书；' +
            '如需续期请用证书列表行的「更新证书」。',
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
      if (e.is('CERT_009')) {
        dialog.warning({
          title: '缺少 DNS 凭证',
          content: '所选 DNS 凭证不存在或未选择。请先在 DNS 凭证管理中配置服务商 API 凭证（CERT_009）。',
          positiveText: '去配置 DNS 凭证',
          onPositiveClick: () => {
            show.value = false
            emit('manage-credentials')
          }
        })
        return
      }
      if (e.is('CERT_010')) {
        message.error(fieldOfString(e.data, 'reason') ?? CERT_ERROR_TEXT['CERT_010'])
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
      message.error(CERT_ERROR_TEXT[e.code] ?? '申请失败')
    } else {
      throw e
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
    style="width: 560px"
    :mask-closable="false"
    @update:show="onClose"
  >
    <n-form ref="formRef" :model="form" :rules="rules" label-placement="top">
      <n-form-item label="挑战方式" path="challenge">
        <n-select v-model:value="form.challenge" :options="challengeOptions" />
      </n-form-item>
      <template v-if="isDns">
        <n-form-item label="域名来源" path="domainMode">
          <n-radio-group v-model:value="form.domainMode">
            <n-radio value="site">选择站点</n-radio>
            <n-radio value="manual">手动输入域名（泛域名）</n-radio>
          </n-radio-group>
        </n-form-item>
        <n-form-item v-if="form.domainMode === 'manual'" label="域名" path="domains">
          <n-dynamic-tags v-model:value="form.domains" />
        </n-form-item>
        <n-form-item label="DNS 凭证" path="dnsCredentialId">
          <n-select
            v-model:value="form.dnsCredentialId"
            :options="credOptions"
            :loading="credLoading"
            placeholder="选择 DNS 服务商 API 凭证"
          />
          <n-button
            v-if="credentials.length === 0 && !credLoading"
            quaternary
            type="primary"
            size="small"
            style="margin-left: 8px"
            @click="emit('manage-credentials')"
          >
            先配置 DNS 凭证
          </n-button>
        </n-form-item>
      </template>
      <n-form-item v-if="!isDns || form.domainMode === 'site'" label="站点（域名集合即证书 SAN）" path="siteId">
        <n-select
          v-model:value="form.siteId"
          filterable
          clearable
          :options="siteOptions"
          :loading="sitesStore.listLoading"
          placeholder="选择站点"
        />
      </n-form-item>
      <n-alert v-if="requesting" type="info" :show-icon="true">
        正在向 ACME 服务申请，最长约 2 分钟（DNS-01 含 TXT 记录传播等待，请勿关闭弹窗）。
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
