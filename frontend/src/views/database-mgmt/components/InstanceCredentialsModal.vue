<script setup lang="ts">
/**
 * 配置凭据弹窗（PageSpec 5.1；instanceCredentials，D-03 先验证后落库）。
 * 连接方式下拉（I-07 互斥显隐：socket 模式显 socket 路径、tcp 模式显主机+端口）；
 * 密码 type=password 不回显（⚠️A18/I-02——无任何读出口）。
 * DBM_005/006/021 内联错误（均不落库语义，弹窗保持打开；DBM_021 客户端通道
 * 不可用=v0.1.4 修复轮新增）；DBM_020 连接目标纠正冲突内联提示。
 * v0.1.7 UX 增强：DBM_005/006 内联追加 data.reason 服务器原文（此前固定文案
 * 丢弃 reason）；DBM_005 另按引擎族呈现可操作排查指引（dbmAuthRejectedTips）——
 * 1045 不区分账户不存在/host 落空/密码不符，不再断言「用户名或密码错误」。
 */
import { computed, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import type { FormInst, FormRules, SelectOption } from 'naive-ui'
import { instanceCredentials } from '@/api/database'
import { ApiError } from '@/api/http'
import { DB_CONN_MODE } from '@/types/enums'
import type { DbConnMode } from '@/types/enums'
import type { CredentialsResult, DbInstanceItem } from '@/types/database'
import { dbmAuthRejectedTips, reasonOf } from '@/types/database'

const show = defineModel<boolean>('show', { default: false })

const props = defineProps<{
  /** 目标实例档案（null=未选定，弹窗不渲染表单） */
  instance: DbInstanceItem | null
}>()

const emit = defineEmits<{
  /** 验证通过并落库（页面行级直更 connState/credentialsSet/version） */
  saved: [result: CredentialsResult]
}>()

const message = useMessage()
const formRef = ref<FormInst | null>(null)
const saving = ref(false)
/** 内联错误（DBM_005/006/020——不落库语义：弹窗保持打开，PageSpec 5.1） */
const inlineError = ref<string | null>(null)
/** 服务器原文原因（DBM_005/006 data.reason 如实呈现，v0.1.7） */
const inlineReason = ref<string | null>(null)
/** 可操作排查指引（DBM_005 按引擎族，v0.1.7） */
const inlineTips = ref<string[]>([])

interface CredForm {
  connMode: DbConnMode
  socketPath: string
  host: string
  port: number | null
  username: string
  password: string
}

/** 引擎默认端口（PageSpec 5.1：档案现值或引擎默认端口——MySQL 族 3306 / Redis 6379） */
const ENGINE_DEFAULT_PORT: Readonly<Record<string, number>> = { mysql: 3306, mariadb: 3306, redis: 6379, mongodb: 27017 }

const form = ref<CredForm>({
  connMode: 'tcp',
  socketPath: '',
  host: '',
  port: null,
  username: '',
  password: ''
})

/** 打开时以档案现值回填（连接目标纠正字段 D-03；密码永不回显） */
watch(show, (v) => {
  if (!v || props.instance === null) return
  const it = props.instance
  form.value = {
    connMode: it.connMode,
    socketPath: it.socketPath ?? '',
    host: it.host ?? 'localhost',
    port: it.port ?? ENGINE_DEFAULT_PORT[it.engine] ?? 3306,
    username: '',
    password: ''
  }
  inlineError.value = null
  inlineReason.value = null
  inlineTips.value = []
  formRef.value?.restoreValidation()
})

const connModeOptions: SelectOption[] = (Object.keys(DB_CONN_MODE) as DbConnMode[]).map((v) => ({
  label: DB_CONN_MODE[v].label,
  value: v
}))

const isSocket = computed(() => form.value.connMode === 'socket')

/** I-08：socket 路径为绝对路径形态 */
const rules = computed<FormRules>(() => ({
  connMode: [{ required: true, message: '请选择连接方式', trigger: ['blur', 'change'] }],
  socketPath: isSocket.value
    ? [
        {
          validator: (_rule, value: string) => value.startsWith('/'),
          message: 'socket 路径须为绝对路径（以 / 开头，I-08）',
          trigger: ['blur', 'input']
        }
      ]
    : [],
  host: isSocket.value
    ? []
    : [{ required: true, message: '请输入主机', trigger: ['blur', 'input'] }],
  port: isSocket.value
    ? []
    : [
        {
          validator: (_rule, value: number | null) => value !== null && value >= 1 && value <= 65535,
          message: '端口范围 1~65535（I-08）',
          trigger: ['blur', 'change']
        }
      ],
  username: [{ required: true, message: '请输入用户名', trigger: ['blur', 'input'] }],
  password: [{ required: true, message: '请输入密码', trigger: ['blur', 'input'] }]
}))

async function onSubmit(): Promise<void> {
  if (props.instance === null) return
  const valid = await formRef.value?.validate().then(() => true).catch(() => false)
  if (valid !== true) return
  inlineError.value = null
  inlineReason.value = null
  inlineTips.value = []
  saving.value = true
  try {
    const result = await instanceCredentials({
      instanceId: props.instance.id,
      connMode: form.value.connMode,
      socketPath: isSocket.value ? form.value.socketPath : undefined,
      host: isSocket.value ? undefined : form.value.host,
      port: isSocket.value ? undefined : (form.value.port ?? undefined),
      username: form.value.username,
      password: form.value.password
    })
    message.success('凭据已验证并保存')
    emit('saved', result)
    show.value = false
  } catch (e) {
    // DBM_005/006：不落库语义 → 内联提示、弹窗保持打开（PageSpec 5.1）
    if (e instanceof ApiError) {
      if (e.is('DBM_005')) {
        inlineError.value = '认证被拒（未保存）：服务器拒绝了该账户的登录'
        inlineReason.value = reasonOf(e.data)
        inlineTips.value = dbmAuthRejectedTips(props.instance.engine)
        return
      }
      if (e.is('DBM_006')) {
        inlineError.value = '连接失败（未保存）：网络或 socket 不可达'
        inlineReason.value = reasonOf(e.data)
        return
      }
      if (e.is('DBM_021')) {
        inlineError.value =
          '引擎客户端通道不可用：宿主机未安装 mysql/redis-cli 客户端（实例本身可能正常；安装对应客户端后重试）'
        return
      }
      if (e.is('DBM_020')) {
        inlineError.value = '该连接目标已存在其他实例档案（DBM_020）'
        return
      }
      // 其余错误已由拦截器提示（网络/AUTH 族）
    }
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <n-modal
    v-model:show="show"
    preset="card"
    title="配置实例凭据"
    style="width: 480px"
    :mask-closable="false"
  >
    <n-form
      v-if="instance !== null"
      ref="formRef"
      :model="form"
      :rules="rules"
      label-placement="top"
    >
      <n-form-item label="连接方式" path="connMode">
        <n-select v-model:value="form.connMode" :options="connModeOptions" />
      </n-form-item>
      <n-form-item v-if="isSocket" label="socket 路径" path="socketPath">
        <n-input v-model:value="form.socketPath" placeholder="/var/run/mysqld/mysqld.sock" />
      </n-form-item>
      <template v-else>
        <n-form-item label="主机" path="host">
          <n-input v-model:value="form.host" placeholder="localhost" />
        </n-form-item>
        <n-form-item label="端口" path="port">
          <n-input-number
            v-model:value="form.port"
            :min="1"
            :max="65535"
            style="width: 100%"
          />
        </n-form-item>
      </template>
      <n-form-item label="用户名" path="username">
        <n-input v-model:value="form.username" placeholder="连接账户名" />
      </n-form-item>
      <n-form-item label="密码" path="password">
        <n-input
          v-model:value="form.password"
          type="password"
          show-password-on="click"
          placeholder="密码不回显（仅验证后密文保存）"
        />
      </n-form-item>
      <!-- DBM_005/006/020/021 内联错误（不落库语义，弹窗保持打开）；
           v0.1.7：DBM_005/006 追加服务器原文 + DBM_005 可操作排查指引 -->
      <n-alert v-if="inlineError !== null" type="error" :show-icon="false">
        <div>{{ inlineError }}</div>
        <div v-if="inlineReason !== null" class="inline-reason">服务器原文：{{ inlineReason }}</div>
        <ul v-if="inlineTips.length > 0" class="inline-tips">
          <li v-for="tip in inlineTips" :key="tip">{{ tip }}</li>
        </ul>
      </n-alert>
    </n-form>
    <template #footer>
      <div class="modal-footer">
        <n-button :disabled="saving" @click="show = false">取消</n-button>
        <n-button type="primary" :loading="saving" @click="onSubmit">验证并保存</n-button>
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
.inline-reason {
  margin-top: 6px;
  font-size: 12px;
  word-break: break-all;
  opacity: 0.85;
}
.inline-tips {
  margin: 6px 0 0;
  padding-left: 18px;
  font-size: 12px;
}
.inline-tips li {
  margin-top: 2px;
}
</style>
