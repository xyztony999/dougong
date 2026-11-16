<script setup lang="ts">
/**
 * 路径边界设置 Dialog（PageSpec 4.4.7，pathScopeGet/pathScopeUpdate，⚠️A12——admin 专属入口）。
 * 打开时 pathScopeGet 加载；受管根 NInput + 排除目录 NDynamicTags（预填拆分）；
 * 前端预校验：绝对路径形态 + 排除项不等于受管根（FILE_001/FILE_002 兜底）；
 * 保存成功 message「配置已保存并生效」（D-02 同事务审计+副本刷新，前端无感）+ 关闭。
 */
import { ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import type { FormInst, FormRules } from 'naive-ui'
import { pathScopeGet, pathScopeUpdate } from '@/api/file'
import { ApiError } from '@/api/http'
import { fileActionError } from '../errors'

const show = defineModel<boolean>('show', { default: false })

const emit = defineEmits<{
  /** 保存成功（页面刷新当前目录——可能变为越界，FILE_002 横幅兜底，api FR-02） */
  updated: []
}>()

const message = useMessage()
const formRef = ref<FormInst | null>(null)
const loading = ref(false)
const saving = ref(false)
const rootPath = ref('')
const excludedDirs = ref<string[]>([])

function isAbsolute(p: string): boolean {
  return p.startsWith('/')
}

const rules: FormRules = {
  rootPath: [
    { required: true, message: '请输入受管根', trigger: ['blur', 'input'] },
    {
      validator: (_rule, value: string) => isAbsolute(value.trim()),
      message: '受管根须为绝对路径（以 / 开头）',
      trigger: 'blur'
    }
  ]
}

/** 打开时加载当前配置（D-14 类同：打开即取，不缓存） */
watch(show, (v) => {
  if (!v) return
  loading.value = true
  rootPath.value = ''
  excludedDirs.value = []
  void pathScopeGet()
    .then((data) => {
      rootPath.value = data.rootPath
      excludedDirs.value = [...data.excludedDirs]
    })
    .catch(() => {
      // 加载失败已由拦截器提示；Dialog 保留空表单
    })
    .finally(() => {
      loading.value = false
    })
})

/** 排除目录校验：每项绝对路径；不含受管根本身（api 4.2.4：排除受管根=全禁无意义） */
function dirError(): string | null {
  const root = rootPath.value.trim().replace(/\/+$/, '') || '/'
  for (const d of excludedDirs.value) {
    if (!isAbsolute(d)) return `排除目录须为绝对路径：${d}`
    const norm = d.replace(/\/+$/, '') || '/'
    if (norm === root) return '排除目录不能等于受管根本身'
  }
  return null
}

const dirFeedback = ref<string | null>(null)

async function onSubmit(): Promise<void> {
  await formRef.value?.validate()
  dirFeedback.value = dirError()
  if (dirFeedback.value !== null) return
  saving.value = true
  try {
    await pathScopeUpdate(rootPath.value.trim(), [...excludedDirs.value])
    message.success('配置已保存并生效')
    show.value = false
    emit('updated')
  } catch (e) {
    if (e instanceof ApiError) {
      const info = fileActionError(e)
      if (info !== null) {
        if (info.level === 'warning') message.warning(info.text)
        else message.error(info.text)
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
    title="路径边界设置"
    style="width: 520px"
    :mask-closable="false"
    @update:show="(v: boolean) => (show = v)"
  >
    <n-spin :show="loading" size="small">
      <n-form ref="formRef" :model="{ rootPath }" :rules="rules" label-placement="top">
        <n-form-item label="受管根" path="rootPath">
          <n-input v-model:value="rootPath" placeholder="绝对路径，如 /srv" />
        </n-form-item>
        <n-form-item label="排除目录">
          <n-dynamic-tags v-model:value="excludedDirs" />
        </n-form-item>
        <div v-if="dirFeedback !== null" class="feedback">{{ dirFeedback }}</div>
        <div class="note">保存后即时生效；排除目录不可包含受管根本身。修改后当前目录可能变为越界。</div>
      </n-form>
    </n-spin>
    <template #footer>
      <div class="footer">
        <n-button @click="show = false">取消</n-button>
        <n-button type="primary" :loading="saving" @click="onSubmit">保存</n-button>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.feedback {
  font-size: 12px;
  color: crimson;
  margin-bottom: 8px;
}
.note {
  font-size: 12px;
  opacity: 0.6;
}
.footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
</style>
