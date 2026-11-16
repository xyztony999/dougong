<script setup lang="ts">
/**
 * 新建/编辑任务 Dialog（PageSpec 5.1；admin 专属，readonly 无入口不渲染宿主）。
 * 3 字段：表达式（等宽+校验预览区）/ 命令（textarea 2 行等宽）/ 备注（可空）。
 * 编辑态按 jobKey 预填（来源 panel/existing 不展示为可编辑项，api D-02）；
 * 表达式校验预览为提交前置（valid 前保存禁用）；CB_001 字段级/CB_002 段位级回填、
 * CB_003 实况已删 → warning + 关闭 + 重拉（changed 事件）。
 */
import { computed, reactive, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import type { FormInst, FormRules } from 'naive-ui'
import { jobCreate, jobUpdate } from '@/api/cron'
import { ApiError } from '@/api/http'
import { useCronStore } from '@/stores/cron'
import { isFieldErrorList } from '@/types/auth'
import { CB_ERROR_TEXT, cronValidateErrors } from '@/types/cron'
import ExprPreview from './ExprPreview.vue'
import { useExprValidate } from '../expr-validate'

const show = defineModel<boolean>('show', { default: false })
const emit = defineEmits<{ changed: [] }>()

const store = useCronStore()
const message = useMessage()

const editing = computed(() => store.editingJob)

const form = reactive({ expr: '', command: '', remark: '' })
const formRef = ref<FormInst | null>(null)
const submitting = ref(false)
/** 服务端字段级错误（CB_001/CB_002 兜底回填；前端预校验不可达位） */
const serverErrors = ref<{ expr: string | null; command: string | null }>({
  expr: null,
  command: null
})

const exprRef = computed(() => form.expr)
const preview = useExprValidate(exprRef)

watch(show, (v) => {
  if (!v) return
  serverErrors.value = { expr: null, command: null }
  const job = store.editingJob
  form.expr = job !== null ? job.expr : ''
  form.command = job !== null ? job.command : ''
  form.remark = job !== null ? (job.remark ?? '') : ''
  preview.reset()
  formRef.value?.restoreValidation()
})

const rules: FormRules = {
  expr: { required: true, message: '请输入表达式', trigger: 'blur' },
  command: { required: true, message: '请输入命令（R-14 非空）', trigger: 'blur' }
}

/** 预览为提交前置（PageSpec 5.1：未通过校验禁止保存按钮） */
const canSave = computed(
  () => preview.state.value === 'valid' && form.command.trim() !== '' && !submitting.value
)

async function onSave(): Promise<void> {
  if (!canSave.value) return
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  const remark = form.remark.trim()
  const payload = {
    expr: form.expr.trim(),
    command: form.command,
    remark: remark === '' ? undefined : remark
  }
  submitting.value = true
  try {
    if (editing.value === null) {
      await jobCreate(payload)
      message.success('任务已创建')
    } else {
      await jobUpdate({ jobKey: editing.value.jobKey, ...payload })
      message.success('任务已保存')
    }
    show.value = false
    emit('changed') // 重拉 jobList（crontab 实况已变，PageSpec 第 2 节纪律）
  } catch (e) {
    handleError(e)
  } finally {
    submitting.value = false
  }
}

/** CB_001（字段级）/CB_002（段位级）/CB_003（实况已删）分流（PageSpec 第 1 节） */
function handleError(e: unknown): void {
  if (!(e instanceof ApiError)) return
  if (e.is('CB_002')) {
    const errors = cronValidateErrors(e.data)
    serverErrors.value.expr =
      errors.length > 0 ? errors.join('；') : CB_ERROR_TEXT['CB_002']
    return
  }
  if (e.is('CB_001')) {
    const fields = isFieldErrorList(e.data) ? e.data : []
    serverErrors.value = {
      expr: fields.find((f) => f.field === 'expr')?.message ?? null,
      command: fields.find((f) => f.field === 'command')?.message ?? null
    }
    if (serverErrors.value.expr === null && serverErrors.value.command === null) {
      message.error(
        fields.length > 0
          ? fields.map((f) => `${f.field}: ${f.message}`).join('；')
          : CB_ERROR_TEXT['CB_001']
      )
    }
    return
  }
  if (e.is('CB_003')) {
    // 编辑目标已在面板外删除（实况投影 D-01）：提示 + 关闭 + 重拉
    message.warning(CB_ERROR_TEXT['CB_003'])
    show.value = false
    emit('changed')
    return
  }
  const text = CB_ERROR_TEXT[e.code]
  if (text !== undefined) message.error(text)
}
</script>

<template>
  <n-modal
    v-model:show="show"
    preset="card"
    :title="editing === null ? '新建任务' : '编辑任务'"
    style="width: 560px"
    :mask-closable="false"
  >
    <n-form
      ref="formRef"
      :model="form"
      :rules="rules"
      label-placement="top"
      :show-require-mark="true"
    >
      <n-form-item label="表达式" path="expr" :feedback="serverErrors.expr ?? undefined">
        <n-input
          v-model:value="form.expr"
          class="mono-input"
          placeholder="五段式（分 时 日 月 周），支持 * , - /步进 与标准宏（保存时归一化为五段）"
          clearable
        />
      </n-form-item>
      <expr-preview
        :state="preview.state.value"
        :next-fires="preview.nextFires.value"
        :error-text="preview.errorText.value"
      />
      <n-form-item
        label="命令"
        path="command"
        class="field-gap"
        :feedback="serverErrors.command ?? undefined"
      >
        <n-input
          v-model:value="form.command"
          type="textarea"
          :rows="2"
          class="mono-input"
          placeholder="原样写入 root crontab"
        />
      </n-form-item>
      <n-form-item label="备注（可空）" path="remark">
        <n-input v-model:value="form.remark" placeholder="空=不写备注注释行" clearable />
      </n-form-item>
    </n-form>
    <template #footer>
      <div class="modal-footer">
        <n-button :disabled="submitting" @click="show = false">取消</n-button>
        <n-button type="primary" :disabled="!canSave" :loading="submitting" @click="onSave()">
          保存
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
.field-gap {
  margin-top: 8px;
}
.mono-input :deep(input),
.mono-input :deep(textarea) {
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', monospace;
}
</style>
