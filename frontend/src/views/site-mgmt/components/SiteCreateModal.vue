<script setup lang="ts">
/**
 * 新建站点弹窗（PageSpec 4.1；siteCreate）。
 * 字段按 type 条件显隐（主字段 ≤4 + 高级选项折叠 ≤2）；三模板：
 * static=根目录必填 / php_fpm=pool 必填（候选实时读取 D-14，空清单禁用提交并如实提示）/
 * reverse_proxy=上游必填 + 高级选项（Host 头/WS 透传）。
 * SITE_007/006/008 前端预校验（n-form rules）；SITE_005 → 同域名+端口已存在 +
 * existingSiteId 查看引导；SITE_003 → 共用回滚 diff 弹窗。
 */
import { computed, ref, watch } from 'vue'
import { useDialog, useMessage } from 'naive-ui'
import type { FormInst, FormRules, SelectOption } from 'naive-ui'
import { phpPoolsList, siteCreate } from '@/api/site'
import { ApiError } from '@/api/http'
import { SITE_TYPE } from '@/types/enums'
import type { SiteType } from '@/types/enums'
import type { PhpPool, SiteRollbackData } from '@/types/site'
import { fieldOfString, isSiteRollbackData } from '@/types/site'
import { siteActionError } from '../errors'
import RollbackDiffModal from './RollbackDiffModal.vue'

const show = defineModel<boolean>('show', { default: false })

const emit = defineEmits<{
  /** 创建成功（页面刷新列表） */
  created: []
  /** SITE_005 查看已有站点引导 */
  'view-site': [siteId: string]
}>()

const message = useMessage()
const dialog = useDialog()
const formRef = ref<FormInst | null>(null)
const saving = ref(false)
const rollback = ref<SiteRollbackData | null>(null)

interface CreateForm {
  type: SiteType
  serverNames: string[]
  listen: string
  root: string
  poolName: string | null
  upstream: string
  hostHeader: string
  websocket: boolean
}

const form = ref<CreateForm>({
  type: 'static',
  serverNames: [],
  listen: '80',
  root: '',
  poolName: null,
  upstream: '',
  hostHeader: '',
  websocket: false
})

const typeOptions: SelectOption[] = (Object.keys(SITE_TYPE) as SiteType[]).map((v) => ({
  label: SITE_TYPE[v].label,
  value: v
}))

/** ---- php-fpm pool 候选（D-14 每次请求实时读取：切到 php_fpm 即重拉，无缓存） ---- */
const pools = ref<PhpPool[]>([])
const poolsLoading = ref(false)

async function loadPools(): Promise<void> {
  poolsLoading.value = true
  try {
    const data = await phpPoolsList()
    pools.value = data.pools
  } catch {
    // 网络等错误已由拦截器提示；清单保持空（提交按钮禁用 + 如实提示）
  } finally {
    poolsLoading.value = false
  }
}

const poolOptions = computed<SelectOption[]>(() =>
  pools.value.map((p) => ({ label: `${p.name}（${p.socket}）`, value: p.name }))
)

/** ---- 校验（SITE_006/007/008 前端预校验） ---- */

const DOMAIN_RE = /^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$/
const UPSTREAM_RE = /^https?:\/\/[^\s/]+(?::\d{1,5})?(?:[/?#]\S*)?$/

function listenError(value: string): string | null {
  const parts = value.split(',').map((s) => s.trim()).filter((s) => s !== '')
  if (parts.length === 0) return '请输入监听端口'
  for (const p of parts) {
    if (!/^\d+$/.test(p)) return '端口须为数字（多值用逗号分隔）'
    const n = Number(p)
    if (n < 1 || n > 65535) return '端口范围 1~65535（SITE_006）'
  }
  return null
}

const rules = computed<FormRules>(() => ({
  serverNames: [
    {
      validator: (_rule, value: string[]) =>
        value.length > 0 && value.every((d) => DOMAIN_RE.test(d)),
      message: '至少 1 个合法域名（不允许空值、空格或通配符，SITE_007）',
      trigger: ['blur', 'change']
    }
  ],
  listen: [
    {
      validator: (_rule, value: string) => listenError(value) === null,
      message: '监听端口非法：逗号分隔，每项 1~65535（SITE_006）',
      trigger: ['blur', 'input']
    }
  ],
  root:
    form.value.type === 'static'
      ? [{ required: true, message: '静态站点须填写根目录', trigger: ['blur', 'input'] }]
      : [],
  poolName:
    form.value.type === 'php_fpm'
      ? [{ required: true, message: 'PHP 站点须选择 php-fpm pool', trigger: ['blur', 'change'] }]
      : [],
  upstream:
    form.value.type === 'reverse_proxy'
      ? [
          {
            validator: (_rule, value: string) => UPSTREAM_RE.test(value),
            message: '上游地址须为 http(s)://host[:port] 形态（SITE_008）',
            trigger: ['blur', 'input']
          }
        ]
      : []
}))

/** php_fpm 候选为空 → 禁用提交 + 如实提示（FR-15/D-14） */
const poolsEmptyBlocked = computed(() => form.value.type === 'php_fpm' && pools.value.length === 0)
const submitDisabled = computed(() => poolsEmptyBlocked.value || saving.value)

watch(
  () => form.value.type,
  (t) => {
    if (t === 'php_fpm') void loadPools()
  }
)

watch(show, (open) => {
  if (open && form.value.type === 'php_fpm') void loadPools()
})

function resetForm(): void {
  form.value = {
    type: 'static',
    serverNames: [],
    listen: '80',
    root: '',
    poolName: null,
    upstream: '',
    hostHeader: '',
    websocket: false
  }
  pools.value = []
  formRef.value?.restoreValidation()
}

function onClose(v: boolean): void {
  if (!v) resetForm()
}

async function onSubmit(): Promise<void> {
  await formRef.value?.validate()
  saving.value = true
  try {
    const f = form.value
    await siteCreate({
      type: f.type,
      serverNames: f.serverNames,
      listen: f.listen,
      root: f.type === 'static' ? f.root : undefined,
      poolName: f.type === 'php_fpm' ? (f.poolName ?? undefined) : undefined,
      upstream: f.type === 'reverse_proxy' ? f.upstream : undefined,
      hostHeader: f.type === 'reverse_proxy' && f.hostHeader !== '' ? f.hostHeader : undefined,
      websocket: f.type === 'reverse_proxy' ? f.websocket : undefined
    })
    message.success('站点已创建并生效')
    show.value = false
    resetForm()
    emit('created')
  } catch (e) {
    if (e instanceof ApiError) {
      if (e.is('SITE_005')) {
        const existingSiteId = fieldOfString(e.data, 'existingSiteId')
        dialog.error({
          title: '站点已存在',
          content: '同域名+监听端口的站点已存在（SITE_005），可直接查看已有站点。',
          positiveText: existingSiteId !== null ? '查看已有站点' : '关闭',
          negativeText: '关闭',
          onPositiveClick: () => {
            if (existingSiteId !== null) {
              show.value = false
              emit('view-site', existingSiteId)
            }
          }
        })
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
        if (info.output !== undefined) {
          dialog.error({ title: info.text, content: info.output })
        } else message.error(info.text)
      }
    }
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <n-modal
    :show="show"
    preset="card"
    title="新建站点"
    style="width: 520px"
    :mask-closable="false"
    @update:show="onClose"
  >
    <n-form ref="formRef" :model="form" :rules="rules" label-placement="top">
      <n-form-item label="站点类型" path="type">
        <n-select v-model:value="form.type" :options="typeOptions" />
      </n-form-item>
      <n-form-item label="域名（首个为主域名）" path="serverNames">
        <n-select
          v-model:value="form.serverNames"
          multiple
          filterable
          tag
          clearable
          :options="[]"
          placeholder="输入域名后回车添加"
        />
      </n-form-item>
      <n-form-item label="监听端口（多值逗号分隔）" path="listen">
        <n-input v-model:value="form.listen" placeholder="如 80 或 80,443 ssl" @keydown.enter.prevent />
      </n-form-item>
      <n-form-item v-if="form.type === 'static'" label="根目录" path="root">
        <n-input v-model:value="form.root" placeholder="/var/www/example" @keydown.enter.prevent />
      </n-form-item>
      <n-form-item v-if="form.type === 'php_fpm'" label="PHP-FPM Pool" path="poolName">
        <n-select
          v-model:value="form.poolName"
          :options="poolOptions"
          :loading="poolsLoading"
          placeholder="选择本机 pool（实时读取）"
        />
      </n-form-item>
      <n-alert v-if="poolsEmptyBlocked" type="warning" :show-icon="true" class="pool-empty">
        未检出 php-fpm pool（SITE_009 口径：仅对接已有 pool，不做 pool 管理）。请先在本机配置 pool 后重试。
      </n-alert>
      <template v-if="form.type === 'reverse_proxy'">
        <n-form-item label="上游地址" path="upstream">
          <n-input v-model:value="form.upstream" placeholder="http://127.0.0.1:8080" @keydown.enter.prevent />
        </n-form-item>
        <n-collapse class="advanced">
          <n-collapse-item title="高级选项" name="advanced">
            <n-form-item label="Host 头（可空）" path="hostHeader">
              <n-input v-model:value="form.hostHeader" placeholder="默认透传原 Host" @keydown.enter.prevent />
            </n-form-item>
            <n-form-item label="WebSocket 透传" path="websocket">
              <n-switch v-model:value="form.websocket" />
            </n-form-item>
          </n-collapse-item>
        </n-collapse>
      </template>
    </n-form>
    <template #footer>
      <div class="footer">
        <n-button @click="show = false">取消</n-button>
        <n-button type="primary" :loading="saving" :disabled="submitDisabled" @click="onSubmit">
          创建并生效
        </n-button>
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
.pool-empty {
  margin-bottom: 12px;
}
.advanced {
  margin-bottom: 4px;
}
</style>
