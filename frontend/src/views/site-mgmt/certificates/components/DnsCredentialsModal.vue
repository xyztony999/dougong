<script setup lang="ts">
/**
 * DNS 凭证管理弹窗（PageSpec 4.2 v1.2「DNS 凭证管理弹窗」；dnsCredList/
 * dnsCredCreate/dnsCredDelete——D-20，⚠️A20）：凭证列表（凭据材料零呈现）+
 * 新建（provider 驱动凭据表单形态）+ 删除（引用证书警示不硬阻断 I-C01）。
 * admin 专属入口（工具栏「DNS 凭证」；readonly 负空间——敏感配置最小暴露）。
 */
import { computed, h, onMounted, ref, watch } from 'vue'
import { useDialog, useMessage } from 'naive-ui'
import { NButton, NPopconfirm, NTag, NTooltip } from 'naive-ui'
import type { DataTableColumns, FormInst, SelectOption } from 'naive-ui'
import { dnsCredCreate, dnsCredDelete, dnsCredList } from '@/api/site'
import { ApiError } from '@/api/http'
import { DNS_PROVIDER, dictItem } from '@/types/enums'
import type { DnsProvider } from '@/types/enums'
import type { DnsCredentialItem, DnsCredentialPayload } from '@/types/site'
import { formatDateTime } from '@/utils/format'

const show = defineModel<boolean>('show', { default: false })

const emit = defineEmits<{ changed: [] }>()

const message = useMessage()
const dialog = useDialog()
const items = ref<DnsCredentialItem[]>([])
const loading = ref(false)

const formRef = ref<FormInst | null>(null)
const creating = ref(false)
const form = ref<{ name: string; provider: DnsProvider; payload: DnsCredentialPayload }>({
  name: '',
  provider: 'cloudflare',
  payload: {}
})

const providerOptions: SelectOption[] = (Object.keys(DNS_PROVIDER) as DnsProvider[]).map((v) => ({
  label: DNS_PROVIDER[v].label,
  value: v
}))

async function load(): Promise<void> {
  loading.value = true
  try {
    items.value = (await dnsCredList()).items
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  if (show.value) void load()
})
watch(show, (v) => {
  if (v) void load()
})

function resetForm(): void {
  form.value = { name: '', provider: 'cloudflare', payload: {} }
  formRef.value?.restoreValidation()
}

async function onCreate(): Promise<void> {
  await formRef.value?.validate()
  creating.value = true
  try {
    await dnsCredCreate({
      name: form.value.name.trim(),
      provider: form.value.provider,
      credential: form.value.payload
    })
    message.success('DNS 凭证已创建（加密存储，不再显示）')
    resetForm()
    await load()
    emit('changed')
  } catch (e) {
    if (e instanceof ApiError) {
      const field = (e.data as Record<string, unknown> | null)?.field
      message.error(typeof field === 'string' ? field : '创建失败：凭证格式校验未通过')
      return
    }
    throw e
  } finally {
    creating.value = false
  }
}

async function onDelete(row: DnsCredentialItem): Promise<void> {
  try {
    const res = await dnsCredDelete(row.id)
    if (res.referencedCerts.length > 0) {
      dialog.warning({
        title: '凭证已删除（存在引用证书）',
        content:
          `以下 ${res.referencedCerts.length} 张 DNS-01 证书的自动续期将失败（证书行保留不受影响）：\n` +
          res.referencedCerts.map((c) => `· #${c.certId} ${c.primaryDomain}`).join('\n'),
        positiveText: '知道了'
      })
    } else {
      message.success('DNS 凭证已删除')
    }
    await load()
    emit('changed')
  } catch (e) {
    if (e instanceof ApiError && e.is('CERT_009')) {
      message.error('凭证不存在（可能已被删除），已刷新清单')
      await load()
      return
    }
    throw e
  }
}

const columns = computed<DataTableColumns<DnsCredentialItem>>(() => [
  { title: '名称', key: 'name', width: 160, ellipsis: { tooltip: true } },
  {
    title: '服务商',
    key: 'provider',
    width: 130,
    render: (row) =>
      h(
        NTag,
        { type: dictItem(DNS_PROVIDER, row.provider).tag, size: 'small', bordered: false },
        { default: () => dictItem(DNS_PROVIDER, row.provider).label }
      )
  },
  {
    title: '引用证书',
    key: 'referencedCertCount',
    width: 90,
    render: (row) =>
      row.referencedCertCount > 0
        ? h(NTooltip, null, {
            trigger: () => h('span', { class: 'cred-refs' }, `${row.referencedCertCount} 张`),
            default: () => '删除凭证后这些 DNS-01 证书的自动续期将失败（I-C01）'
          })
        : '—'
  },
  { title: '创建时间', key: 'createdAt', width: 170, render: (row) => formatDateTime(row.createdAt) },
  {
    title: '操作',
    key: 'actions',
    width: 90,
    render: (row) =>
      h(
        NPopconfirm,
        { onPositiveClick: () => void onDelete(row) },
        {
          trigger: () =>
            h(NButton, { size: 'small', type: 'error', ghost: true }, { default: () => '删除' }),
          default: () =>
            row.referencedCertCount > 0
              ? `该凭证被 ${row.referencedCertCount} 张 DNS-01 证书引用，删除后其自动续期将失败；凭证内容不可恢复。确认删除？`
              : '凭证内容不可恢复。确认删除？'
        }
      )
  }
])
</script>

<template>
  <n-modal
    :show="show"
    preset="card"
    title="DNS 凭证管理"
    style="width: 720px"
    :mask-closable="false"
    @update:show="(v: boolean) => (show = v)"
  >
    <n-alert type="info" :show-icon="true" style="margin-bottom: 12px">
      凭证将以加密形式存储且不再显示；有效性在申请证书时由服务商验证。服务商初始清单：Cloudflare / 阿里云 DNS / 腾讯云 DNSPod。
    </n-alert>

    <n-card size="small" title="已有凭证" style="margin-bottom: 12px">
      <n-data-table
        :columns="columns"
        :data="items"
        :loading="loading"
        :row-key="(r: DnsCredentialItem) => r.id"
        size="small"
      >
        <template #empty>
          <n-empty description="暂无 DNS 凭证" size="small" />
        </template>
      </n-data-table>
    </n-card>

    <n-card size="small" title="新建凭证">
      <n-form ref="formRef" :model="form" label-placement="left" label-width="88" inline>
        <n-form-item label="名称" path="name" :rule="{ required: true, message: '请输入显示名（≤64）' }">
          <n-input v-model:value="form.name" placeholder="如 cf-main" :maxlength="64" style="width: 160px" />
        </n-form-item>
        <n-form-item label="服务商" path="provider" :rule="{ required: true }">
          <n-select v-model:value="form.provider" :options="providerOptions" style="width: 150px" />
        </n-form-item>
      </n-form>
      <n-form label-placement="left" label-width="88">
        <!-- provider 驱动凭据表单形态（D-20：cloudflare=API Token 单值；aliyun=id+secret；dnspod=secretId+key） -->
        <template v-if="form.provider === 'cloudflare'">
          <n-form-item label="API Token" :required="true">
            <n-input
              v-model:value="form.payload.apiToken"
              type="password"
              show-password-on="click"
              placeholder="Cloudflare API Token"
            />
          </n-form-item>
        </template>
        <template v-else-if="form.provider === 'aliyun'">
          <n-form-item label="AccessKeyId" :required="true">
            <n-input v-model:value="form.payload.accessKeyId" placeholder="阿里云 AccessKeyId" />
          </n-form-item>
          <n-form-item label="Secret" :required="true">
            <n-input
              v-model:value="form.payload.accessKeySecret"
              type="password"
              show-password-on="click"
              placeholder="阿里云 AccessKeySecret"
            />
          </n-form-item>
        </template>
        <template v-else>
          <n-form-item label="SecretId" :required="true">
            <n-input v-model:value="form.payload.secretId" placeholder="腾讯云 SecretId" />
          </n-form-item>
          <n-form-item label="SecretKey" :required="true">
            <n-input
              v-model:value="form.payload.secretKey"
              type="password"
              show-password-on="click"
              placeholder="腾讯云 SecretKey"
            />
          </n-form-item>
        </template>
        <n-form-item label=" " label-width="88">
          <n-button type="primary" :loading="creating" @click="onCreate">创建凭证</n-button>
        </n-form-item>
      </n-form>
    </n-card>

    <template #footer>
      <div class="footer">
        <n-button @click="show = false">关闭</n-button>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.footer {
  display: flex;
  justify-content: flex-end;
}
.cred-refs {
  cursor: help;
}
</style>
