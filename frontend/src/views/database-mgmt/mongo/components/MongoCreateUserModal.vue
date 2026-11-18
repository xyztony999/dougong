<script setup lang="ts">
/**
 * MongoDB 建用户弹窗（PageSpec v1.2 5.9；admin）。首库首角色（api D-19）；
 * 密码 type=password 不回显（⚠️A18/I-02）；DBM_010 重名内联提示。
 */
import { computed, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import type { FormInst, FormRules, SelectOption } from 'naive-ui'
import { mongoUserCreate } from '@/api/database'
import { ApiError } from '@/api/http'
import { MONGO_ROLE } from '@/types/enums'
import type { MongoRole } from '@/types/enums'
import type { MongoDatabaseItem } from '@/types/database'

const show = defineModel<boolean>('show', { default: false })

const props = defineProps<{
  instanceId: number
  /** 库清单实况（候选；MongoDB 允许对不存在库预授权——候选外形态走 DBM_007 白名单校验） */
  databases: MongoDatabaseItem[]
}>()

const emit = defineEmits<{
  /** 创建成功（用户清单重拉） */
  created: []
}>()

const message = useMessage()
const formRef = ref<FormInst | null>(null)
const submitting = ref(false)
const inlineError = ref<string | null>(null)

const form = ref({
  username: '',
  password: '',
  db: '',
  role: 'read' as MongoRole
})

watch(show, (v) => {
  if (!v) return
  form.value = { username: '', password: '', db: '', role: 'read' }
  inlineError.value = null
  formRef.value?.restoreValidation()
})

const dbOptions = computed<SelectOption[]>(() =>
  props.databases.map((d) => ({ label: d.name, value: d.name }))
)
const roleOptions: SelectOption[] = (Object.keys(MONGO_ROLE) as MongoRole[]).map((v) => ({
  label: `${MONGO_ROLE[v].label}（${v}）`,
  value: v
}))

const rules: FormRules = {
  username: [
    {
      validator: (_rule, value: string) => /^[A-Za-z0-9_]{1,64}$/.test(value),
      message: '用户名须为 1~64 字符的字母/数字/下划线（R-14）',
      trigger: ['blur', 'input']
    }
  ],
  password: [{ required: true, message: '请输入密码', trigger: ['blur', 'input'] }],
  db: [{ required: true, message: '请选择目标库', trigger: ['blur', 'change'] }],
  role: [{ required: true, message: '请选择角色', trigger: ['blur', 'change'] }]
}

async function onSubmit(): Promise<void> {
  const valid = await formRef.value?.validate().then(() => true).catch(() => false)
  if (valid !== true) return
  inlineError.value = null
  submitting.value = true
  try {
    await mongoUserCreate({
      instanceId: props.instanceId,
      username: form.value.username,
      password: form.value.password,
      db: form.value.db,
      role: form.value.role
    })
    message.success(`用户 ${form.value.username} 已创建（admin 库 + 首角色 ${form.value.db}:${form.value.role}）`)
    emit('created')
    show.value = false
  } catch (e) {
    if (e instanceof ApiError) {
      if (e.is('DBM_010')) {
        inlineError.value = `同名用户已存在（admin 库唯一域）：${form.value.username}`
        return
      }
      // DBM_029 引擎操作被拒（A-DB13：旧 DBM_022 判定已随编号裁定改判，DBM_022=表不存在）
      if (e.is('DBM_029')) {
        inlineError.value = '引擎操作被拒：当前凭据账户权限不足（需用户管理权限，如 userAdminAnyDatabase）'
        return
      }
    }
    if (e instanceof ApiError) {
      const text = e.message
      if (text !== '') {
        inlineError.value = text
        return
      }
    }
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <n-modal
    v-model:show="show"
    preset="card"
    title="新建 MongoDB 用户"
    style="width: 480px"
    :mask-closable="false"
  >
    <n-form ref="formRef" :model="form" :rules="rules" label-placement="top">
      <n-form-item label="用户名" path="username">
        <n-input v-model:value="form.username" placeholder="连接账户名（创建于 admin 库）" />
      </n-form-item>
      <n-form-item label="密码" path="password">
        <n-input
          v-model:value="form.password"
          type="password"
          show-password-on="click"
          placeholder="密码不回显（仅写入引擎）"
        />
      </n-form-item>
      <n-form-item label="目标库" path="db">
        <n-select v-model:value="form.db" :options="dbOptions" filterable tag placeholder="首角色生效的库（可输入候选外库名——预授权）" />
      </n-form-item>
      <n-form-item label="角色" path="role">
        <n-select v-model:value="form.role" :options="roleOptions" />
      </n-form-item>
      <n-alert type="info" :show-icon="false" style="margin-bottom: 10px">
        创建即首库首角色（D-19）：用户建于 admin 库，首个角色为「目标库 + 所选角色」；后续可在用户行「角色管理」增减。
      </n-alert>
      <n-alert v-if="inlineError !== null" type="error" :show-icon="false">
        {{ inlineError }}
      </n-alert>
    </n-form>
    <template #footer>
      <div class="footer">
        <n-button :disabled="submitting" @click="show = false">取消</n-button>
        <n-button type="primary" :loading="submitting" @click="onSubmit">创建</n-button>
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
