<script setup lang="ts">
import { ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import type { FormInst, FormRules } from 'naive-ui'
import { totpEnable, totpSetup } from '@/api/auth'
import { ApiError } from '@/api/http'
import type { TotpSetupData } from '@/types/auth'

const show = defineModel<boolean>('show', { default: false })

const emit = defineEmits<{
  enabled: []
}>()

const message = useMessage()
const formRef = ref<FormInst | null>(null)
const code = ref('')
const saving = ref(false)

/** setup 数据仅此一次下发（api I-09）；关闭弹窗即弃（secret 不持久化，R-02） */
const setupData = ref<TotpSetupData | null>(null)
const loadingSetup = ref(false)

watch(show, (v) => {
  if (v) {
    void fetchSetup()
  } else {
    setupData.value = null
    code.value = ''
    formRef.value?.restoreValidation()
  }
})

async function fetchSetup(): Promise<void> {
  loadingSetup.value = true
  try {
    setupData.value = await totpSetup()
  } catch {
    // 错误已由拦截器统一提示
  } finally {
    loadingSetup.value = false
  }
}

const rules: FormRules = {
  code: [
    { required: true, message: '请输入 6 位动态码', trigger: 'blur' },
    { pattern: /^\d{6}$/, message: '动态码为 6 位数字', trigger: 'blur' }
  ]
}

async function onSubmit(): Promise<void> {
  await formRef.value?.validate()
  saving.value = true
  try {
    await totpEnable({ code: code.value })
    message.success('TOTP 两步验证已启用')
    emit('enabled')
  } catch (e) {
    if (e instanceof ApiError && e.is('AUTH_006')) {
      message.error('动态码错误，请重输')
    }
    // 其余错误已由拦截器统一提示
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <n-modal
    :show="show"
    preset="card"
    title="绑定 TOTP 认证器"
    style="width: 480px"
    :mask-closable="false"
  >
    <n-spin :show="loadingSetup">
      <template v-if="setupData !== null">
        <div class="qr-area">
          <n-qr-code :value="setupData.otpauthUri" :size="160" error-correction-level="M" />
          <div class="qr-meta">
            <p class="hint">使用认证器 App 扫码，或手动输入密钥：</p>
            <n-code :code="setupData.secret" word-wrap />
            <p class="warn">密钥仅此一次展示（I-09），关闭弹窗后不可再查看。</p>
          </div>
        </div>
        <n-form ref="formRef" :model="{ code }" :rules="rules" label-placement="top">
          <n-form-item label="动态码" path="code">
            <n-input
              v-model:value="code"
              placeholder="认证器显示的 6 位数字"
              maxlength="6"
              inputmode="numeric"
              @keyup.enter="onSubmit"
            />
          </n-form-item>
        </n-form>
      </template>
      <n-empty v-else description="正在生成绑定密钥……" />
    </n-spin>
    <template #footer>
      <div class="footer">
        <n-button @click="show = false">取消</n-button>
        <n-button
          type="primary"
          :loading="saving"
          :disabled="setupData === null"
          @click="onSubmit"
        >
          验证并启用
        </n-button>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.qr-area {
  display: flex;
  gap: 16px;
  align-items: flex-start;
  margin-bottom: 12px;
}
.qr-meta {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.hint {
  font-size: 12px;
  opacity: 0.65;
}
.warn {
  font-size: 12px;
  opacity: 0.8;
}
.footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
</style>
