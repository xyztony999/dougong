<script setup lang="ts">
/**
 * 添加端口规则 Dialog（PageSpec 第 5 节；宽 520px，标题「添加端口规则」）。
 * 4 字段：port（正则+范围预校验 D-05/FW_001）/ protocol（tcp 默认）/ source（可空，CIDR 预校验 FW_002）/
 * action（options=backendStatus.supportedActions 服务端权威面 D-01/I-04，下拉项附中文释义）。
 * direction 不入表单（固定入站单值维度，D-05）。
 * 提交由父页编排（FW_005 两段式/FW_006/007/008 结果反馈在父页分流）：
 * 本组件只做前端预校验 + emit('submit')；FW_001/FW_002 服务端兜底错误经 serverErrors prop 回填字段。
 */
import { computed, ref, watch } from 'vue'
import type { FormInst, FormRules, SelectOption } from 'naive-ui'
import { FIREWALL_RULE_ACTION, FIREWALL_RULE_PROTOCOL } from '@/types/enums'
import type { FirewallRuleAction, FirewallRuleProtocol } from '@/types/enums'

const show = defineModel<boolean>('show', { default: false })

const props = defineProps<{
  /** 服务端权威动作支持面（D-01/I-04：ufw 全四值 / firewalld 仅 allow；前端不自行推断） */
  supportedActions: FirewallRuleAction[]
  /** 父页提交在途（提交按钮 loading，防重复提交） */
  submitting: boolean
  /** FW_001/FW_002 服务端字段错误兜底（正常 UI 预校验不可达；null=无错误） */
  serverErrors: { port: string | null; source: string | null }
}>()

const emit = defineEmits<{
  /** 前端预校验通过（source 空串转为 undefined=任意来源） */
  submit: [values: { port: string; protocol: FirewallRuleProtocol; source?: string; action: FirewallRuleAction }]
}>()

interface AddRuleForm {
  port: string
  protocol: FirewallRuleProtocol
  source: string
  action: FirewallRuleAction | null
}

const formRef = ref<FormInst | null>(null)
const form = ref<AddRuleForm>({ port: '', protocol: 'tcp', source: '', action: null })

/** 服务端字段错误本地副本（用户改动即清除——修正后重新提交） */
const fieldPortError = ref<string | null>(null)
const fieldSourceError = ref<string | null>(null)

watch(
  () => props.serverErrors,
  (errs) => {
    fieldPortError.value = errs.port
    fieldSourceError.value = errs.source
  }
)

watch(show, (visible) => {
  if (!visible) return
  // 每次打开复位（confirmDanger 不记忆——api 幂等语义，涉险确认每次独立）
  form.value = {
    port: '',
    protocol: 'tcp',
    source: '',
    action: props.supportedActions.includes('allow') ? 'allow' : (props.supportedActions[0] ?? null)
  }
  fieldPortError.value = null
  fieldSourceError.value = null
  formRef.value?.restoreValidation()
})

const protocolOptions: SelectOption[] = (Object.keys(FIREWALL_RULE_PROTOCOL) as FirewallRuleProtocol[]).map(
  (v) => ({ label: FIREWALL_RULE_PROTOCOL[v].label, value: v })
)

/** 动作下拉：仅列服务端权威支持面（PageSpec 5.1；firewalld 环境仅 allow——FW_004 正常 UI 不可达） */
const actionOptions = computed<SelectOption[]>(() =>
  props.supportedActions.map((v) => ({
    label: `${FIREWALL_RULE_ACTION[v].label}（${v}）`,
    value: v
  }))
)

/** ---- 前端预校验（D-05 规则集；服务端 R-14 兜底 FW_001/FW_002） ---- */

const PORT_PATTERN = /^([1-9]\d{0,4})(-([1-9]\d{0,4}))?$/

function portError(value: string): string | null {
  if (!PORT_PATTERN.test(value)) return '端口格式：80 或 8080-8090'
  const segs = value.split('-').map(Number)
  if (segs.some((n) => n < 1 || n > 65535)) return '端口须在 1-65535 范围内'
  if (segs.length === 2 && segs[1] !== undefined && segs[1] < segs[0]) return '端口段须 N≤M'
  return null
}

function isValidIpv4(addr: string): boolean {
  const parts = addr.split('.')
  if (parts.length !== 4) return false
  return parts.every((p) => /^\d{1,3}$/.test(p) && Number(p) <= 255)
}

/** IPv6 结构性校验（'::' 至多一次；十六进制组 1-4 位；内嵌 IPv4 形态与完整压缩校验由服务端 FW_002 终审） */
function isValidIpv6(addr: string): boolean {
  const chunks = addr.split('::')
  if (chunks.length > 2) return false
  const head = chunks[0] === '' ? [] : chunks[0].split(':')
  const tail = chunks.length === 2 ? (chunks[1] === '' ? [] : chunks[1].split(':')) : []
  const groups = [...head, ...tail]
  if (groups.length > 8) return false
  if (groups.length === 0 && chunks.length !== 2) return false
  return groups.every((g) => /^[0-9a-fA-F]{1,4}$/.test(g))
}

function isValidCidr(value: string): boolean {
  const first = value.indexOf('/')
  const last = value.lastIndexOf('/')
  if (first <= 0 || first !== last) return false
  const addr = value.slice(0, first)
  const prefix = value.slice(first + 1)
  if (!/^\d{1,3}$/.test(prefix)) return false
  const len = Number(prefix)
  return addr.includes(':') ? isValidIpv6(addr) && len <= 128 : isValidIpv4(addr) && len <= 32
}

const rules: FormRules = {
  port: [
    {
      required: true,
      validator: (_rule, value: string) => portError(value) === null,
      message: '端口格式：80 或 8080-8090（1-65535，N≤M）',
      trigger: ['blur', 'input']
    }
  ],
  protocol: [{ required: true, message: '请选择协议', trigger: ['blur', 'change'] }],
  source: [
    {
      // 空=任意来源（D-05 缺省语义）；非空须为合法 IPv4/IPv6 CIDR（FW_002 预校验）
      validator: (_rule, value: string) => value === '' || isValidCidr(value),
      message: '须为合法 IPv4/IPv6 CIDR（如 10.0.0.0/8 或 2001:db8::/32）',
      trigger: ['blur', 'input']
    }
  ],
  action: [{ required: true, message: '请选择动作', trigger: ['blur', 'change'] }]
}

function onInputPort(value: string): void {
  form.value.port = value
  if (fieldPortError.value !== null) fieldPortError.value = null
}

function onInputSource(value: string): void {
  form.value.source = value
  if (fieldSourceError.value !== null) fieldSourceError.value = null
}

async function onSubmit(): Promise<void> {
  await formRef.value?.validate()
  if (form.value.action === null) return
  emit('submit', {
    port: form.value.port,
    protocol: form.value.protocol,
    source: form.value.source === '' ? undefined : form.value.source,
    action: form.value.action
  })
}
</script>

<template>
  <n-modal
    v-model:show="show"
    preset="card"
    title="添加端口规则"
    style="width: 520px"
    :mask-closable="false"
  >
    <n-form
      ref="formRef"
      :model="form"
      :rules="rules"
      label-placement="top"
    >
      <n-form-item label="端口" path="port" :validation-status="fieldPortError !== null ? 'error' : undefined" :feedback="fieldPortError ?? undefined">
        <n-input :value="form.port" placeholder="80 或 8080-8090" @update:value="onInputPort" />
      </n-form-item>
      <n-form-item label="协议" path="protocol">
        <n-select v-model:value="form.protocol" :options="protocolOptions" />
      </n-form-item>
      <n-form-item label="来源限制" path="source" :validation-status="fieldSourceError !== null ? 'error' : undefined" :feedback="fieldSourceError ?? undefined">
        <n-input
          :value="form.source"
          placeholder="留空=任意来源"
          @update:value="onInputSource"
        />
      </n-form-item>
      <n-form-item label="动作" path="action">
        <n-select v-model:value="form.action" :options="actionOptions" placeholder="请选择动作" />
      </n-form-item>
    </n-form>
    <!-- 表单底部灰字说明（api D-08 双写口径的用户可知会项） -->
    <div class="hint">规则经当前后端即时生效（firewalld 环境同时写入永久配置与运行态）</div>
    <template #footer>
      <div class="modal-footer">
        <n-button :disabled="props.submitting" @click="show = false">取消</n-button>
        <n-button type="primary" :loading="props.submitting" @click="onSubmit">添加</n-button>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
.hint {
  font-size: 12px;
  opacity: 0.6;
}
</style>
