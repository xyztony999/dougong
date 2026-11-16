<script setup lang="ts">
/**
 * 拉取镜像 Dialog（PageSpec 5.4；宽 480px；1 字段 < 5 字段 → Dialog 形态，分册 3.2）。
 * ref 前端结构性预校验（非空 + `名称[:tag]` 形态，api DK_012 口径）+ 服务端 R-14 终审
 * （DK_012 经 serverError prop 回填字段）。提交由父页编排（DK_013 拉取失败 Dialog 保持
 * 打开供修正重试）；同步拉取 D-05 ≤600s——submitting 期按钮 loading + 「拉取中…」。
 */
import { ref, watch } from 'vue'
import type { FormInst, FormRules } from 'naive-ui'

const show = defineModel<boolean>('show', { default: false })

const props = defineProps<{
  /** 同步拉取在途（D-05；按钮 loading + 文案切换「拉取中…」） */
  submitting: boolean
  /** DK_012 服务端字段错误兜底（正常 UI 预校验不可达；null=无错误） */
  serverError: string | null
}>()

const emit = defineEmits<{
  submit: [ref: string]
}>()

const formRef = ref<FormInst | null>(null)
const refValue = ref('')
const fieldError = ref<string | null>(null)

watch(
  () => props.serverError,
  (err) => {
    fieldError.value = err
  }
)

watch(show, (visible) => {
  if (!visible) return
  refValue.value = ''
  fieldError.value = null
  formRef.value?.restoreValidation()
})

/**
 * 镜像引用结构性预校验（api DK_012 `名称[:tag]` 口径的宽松形态检查——
 * 无空白、不以/结尾开头、':' 至多两段（registry:port 与 tag）、tag 段合法字符；
 * 完整规则（registry 域名/命名路径深度等）由服务端 R-14 终审）
 */
function refError(value: string): string | null {
  if (value === '') return '请输入镜像引用'
  if (/\s/.test(value)) return '镜像引用不能包含空白字符'
  if (value.startsWith(':') || value.startsWith('/') || value.endsWith(':') || value.endsWith('/')) {
    return '镜像引用格式：名称[:tag]（如 nginx:1.27 或 ubuntu）'
  }
  const lastSlash = value.lastIndexOf('/')
  const tagPart = value.slice(lastSlash + 1)
  const colon = tagPart.indexOf(':')
  if (colon >= 0) {
    const tag = tagPart.slice(colon + 1)
    if (!/^[A-Za-z0-9_][A-Za-z0-9._-]{0,127}$/.test(tag)) {
      return 'tag 须以字母/数字/下划线开头，仅含字母数字 . _ -'
    }
  }
  return null
}

const rules: FormRules = {
  ref: [
    {
      required: true,
      validator: (_rule, value: string) => refError(value) === null,
      message: '镜像引用格式：名称[:tag]（如 nginx:1.27 或 ubuntu）',
      trigger: ['blur', 'input']
    }
  ]
}

function onInput(value: string): void {
  refValue.value = value
  if (fieldError.value !== null) fieldError.value = null
}

async function onSubmit(): Promise<void> {
  await formRef.value?.validate()
  emit('submit', refValue.value)
}
</script>

<template>
  <n-modal
    v-model:show="show"
    preset="card"
    title="拉取镜像"
    style="width: 480px"
    :mask-closable="false"
  >
    <n-form ref="formRef" :model="{ ref: refValue }" :rules="rules" label-placement="top">
      <n-form-item
        label="镜像引用"
        path="ref"
        :validation-status="fieldError !== null ? 'error' : undefined"
        :feedback="fieldError ?? undefined"
      >
        <n-input
          :value="refValue"
          placeholder="如 nginx:1.27 或 ubuntu"
          :disabled="props.submitting"
          @update:value="onInput"
        />
      </n-form-item>
    </n-form>
    <!-- 表单底部灰字说明（api D-05 同步 600s 契约的用户可知会项） -->
    <div class="hint">拉取经本机 Docker daemon 执行（面向公有仓库），大镜像可能耗时较长</div>
    <template #footer>
      <div class="modal-footer">
        <n-button :disabled="props.submitting" @click="show = false">取消</n-button>
        <n-button type="primary" :loading="props.submitting" @click="onSubmit">
          {{ props.submitting ? '拉取中…' : '拉取' }}
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
.hint {
  font-size: 12px;
  opacity: 0.6;
}
</style>
