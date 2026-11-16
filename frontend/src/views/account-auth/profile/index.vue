<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useMessage } from 'naive-ui'
import type { FormInst, FormItemRule, FormRules } from 'naive-ui'
import { passwordChange, userList } from '@/api/auth'
import { useAuthStore } from '@/stores/auth'
import TotpBindModal from './components/TotpBindModal.vue'
import TotpDisableModal from './components/TotpDisableModal.vue'

const message = useMessage()
const authStore = useAuthStore()

/** ---- Card 1 修改密码（自助凭证操作，readonly 亦可用——FL 附录B） ---- */

interface PwdFormData {
  oldPassword: string
  newPassword: string
  confirmPassword: string
}

const pwdFormRef = ref<FormInst | null>(null)
const pwdForm = ref<PwdFormData>({ oldPassword: '', newPassword: '', confirmPassword: '' })
const pwdSaving = ref(false)

const pwdRules: FormRules = {
  oldPassword: { required: true, message: '请输入旧密码', trigger: 'blur' },
  newPassword: [
    { required: true, message: '请输入新密码', trigger: 'blur' },
    { min: 8, message: '密码长度至少 8 位（A10）', trigger: 'blur' }
  ],
  confirmPassword: [
    { required: true, message: '请再次输入新密码', trigger: 'blur' },
    {
      trigger: 'blur',
      validator: (_rule: FormItemRule, value: string): true | Error =>
        value === pwdForm.value.newPassword ? true : new Error('两次输入的密码不一致')
    }
  ]
}

async function onPwdSubmit(): Promise<void> {
  await pwdFormRef.value?.validate()
  pwdSaving.value = true
  try {
    await passwordChange({
      oldPassword: pwdForm.value.oldPassword,
      newPassword: pwdForm.value.newPassword
    })
    // api FR-12：保留当前会话，吊销其他会话
    message.success('密码已修改，其他会话已下线')
    pwdForm.value = { oldPassword: '', newPassword: '', confirmPassword: '' }
    pwdFormRef.value?.restoreValidation()
  } catch {
    // 错误已由拦截器统一提示
  } finally {
    pwdSaving.value = false
  }
}

/** ---- Card 2 TOTP 两步验证 ---- */

/** 自身 totpEnabled 状态：经 userList 匹配自身 id（API 无 profile 详情端点，tasks.md 偏差 3） */
const totpEnabled = ref<boolean | null>(null)
const showBind = ref(false)
const showDisable = ref(false)

async function loadTotpStatus(): Promise<void> {
  const selfId = authStore.currentUser?.id
  if (selfId === undefined) return
  try {
    const data = await userList()
    const self = data.items.find((u) => u.id === selfId)
    totpEnabled.value = self?.totpEnabled ?? false
  } catch {
    totpEnabled.value = null
    // 错误已由拦截器统一提示
  }
}

function onTotpEnabled(): void {
  totpEnabled.value = true
  showBind.value = false
}

function onTotpDisabled(): void {
  totpEnabled.value = false
  showDisable.value = false
}

onMounted(() => {
  void loadTotpStatus()
})
</script>

<template>
  <div class="page">
    <n-card title="修改密码">
      <n-form
        ref="pwdFormRef"
        :model="pwdForm"
        :rules="pwdRules"
        label-placement="top"
        style="max-width: 420px"
      >
        <n-form-item label="旧密码" path="oldPassword">
          <n-input v-model:value="pwdForm.oldPassword" type="password" show-password-on="click" />
        </n-form-item>
        <n-form-item label="新密码" path="newPassword">
          <n-input v-model:value="pwdForm.newPassword" type="password" show-password-on="click" placeholder="至少 8 位" />
        </n-form-item>
        <n-form-item label="确认新密码" path="confirmPassword">
          <n-input v-model:value="pwdForm.confirmPassword" type="password" show-password-on="click" />
        </n-form-item>
        <div class="actions">
          <n-button
            @click="
              pwdForm = { oldPassword: '', newPassword: '', confirmPassword: '' };
              pwdFormRef?.restoreValidation()
            "
          >
            取消
          </n-button>
          <n-button type="primary" :loading="pwdSaving" @click="onPwdSubmit">保存</n-button>
        </div>
      </n-form>
    </n-card>

    <n-card title="TOTP 两步验证">
      <div class="totp-row">
        <template v-if="totpEnabled === null">
          <n-tag size="small" :bordered="false">检测中</n-tag>
        </template>
        <template v-else-if="totpEnabled">
          <n-tag type="success" size="small" :bordered="false">已启用</n-tag>
          <n-button size="small" @click="showDisable = true">解绑</n-button>
        </template>
        <template v-else>
          <n-tag size="small">未启用</n-tag>
          <n-button size="small" type="primary" @click="showBind = true">开始绑定</n-button>
        </template>
      </div>
      <p class="hint">绑定后登录需在密码之后输入认证器动态码（两步式）；解绑需输入登录密码确认。</p>
    </n-card>

    <totp-bind-modal v-model:show="showBind" @enabled="onTotpEnabled" />
    <totp-disable-modal v-model:show="showDisable" @disabled="onTotpDisabled" />
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-width: 720px;
}
.actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
.totp-row {
  display: flex;
  align-items: center;
  gap: 12px;
}
.hint {
  margin: 8px 0 0;
  font-size: 12px;
  opacity: 0.65;
}
</style>
