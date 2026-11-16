<script setup lang="ts">
/**
 * 密码修改弹窗（PageSpec 5.5 root 密码 / 5.7 Redis 密码共用——两节字段与交互同构：
 * 新密码 + 确认新密码，前端一致性比对；密码 type=password 不回显 ⚠️A18/I-02）。
 * 提交与 DBM_019 强提示由父页面处理（成功关窗由父置 show=false；失败弹窗保持打开）。
 */
import { ref, watch } from 'vue'
import type { FormInst, FormRules } from 'naive-ui'

const show = defineModel<boolean>('show', { default: false })

const props = defineProps<{
  /** 弹窗标题（「修改 root 密码」/「设置 Redis 密码」） */
  title: string
  /** 提交中（父页面在途态：按钮 loading + 禁用） */
  submitting: boolean
}>()

const emit = defineEmits<{
  /** 前端校验通过（父页面调用 API：mysqlRootPassword / redisPassword） */
  submit: [newPassword: string]
}>()

const formRef = ref<FormInst | null>(null)

interface PwdForm {
  newPassword: string
  confirm: string
}

const form = ref<PwdForm>({ newPassword: '', confirm: '' })

watch(show, (v) => {
  if (v) {
    form.value = { newPassword: '', confirm: '' }
    formRef.value?.restoreValidation()
  }
})

const rules: FormRules = {
  newPassword: [{ required: true, message: '请输入新密码', trigger: ['blur', 'input'] }],
  confirm: [
    { required: true, message: '请再次输入新密码', trigger: ['blur', 'input'] },
    {
      validator: (_rule, value: string) => value === form.value.newPassword,
      message: '两次输入的密码不一致',
      trigger: ['blur', 'input']
    }
  ]
}

async function onSubmit(): Promise<void> {
  const valid = await formRef.value?.validate().then(() => true).catch(() => false)
  if (valid !== true) return
  emit('submit', form.value.newPassword)
}
</script>

<template>
  <n-modal
    v-model:show="show"
    preset="card"
    :title="props.title"
    style="width: 440px"
    :mask-closable="false"
  >
    <n-form ref="formRef" :model="form" :rules="rules" label-placement="top">
      <n-form-item label="新密码" path="newPassword">
        <n-input
          v-model:value="form.newPassword"
          type="password"
          show-password-on="click"
          placeholder="密码不回显"
        />
      </n-form-item>
      <n-form-item label="确认新密码" path="confirm">
        <n-input
          v-model:value="form.confirm"
          type="password"
          show-password-on="click"
          placeholder="再次输入新密码"
        />
      </n-form-item>
    </n-form>
    <template #footer>
      <div class="modal-footer">
        <n-button :disabled="props.submitting" @click="show = false">取消</n-button>
        <n-button type="primary" :loading="props.submitting" @click="onSubmit">
          确认修改
        </n-button>
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
</style>
