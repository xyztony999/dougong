<script setup lang="ts">
/**
 * 在线编辑页（PageSpec 第 5 节，路由 /files/edit?path=…，code 类型页）。
 * admin 专属——readonly 组件内角色兜底跳回 /files（负空间；Monaco 不加载）；
 * 进入：entryContent（FILE_006 二进制/超 1MiB → warning+返回，D-05；FILE_002/003 → 错误+返回）；
 * 保存：entryContentSave 全量覆盖（I-09）+ Ctrl/Cmd+S 双通道；FILE_011/013 → error 内容保留待重试（N2）；
 * dirty 拦截：路由离开 useDialog confirm + beforeunload（P3；无自动保存/无草稿，OUT-2）；
 * 无版本历史/协同 UI（OUT-2；D-05 最后写者胜）。
 * Monaco 经动态 import 按需加载（编辑器完整工作区；readonly/未带 path 不触达）。
 */
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'
import { useDialog, useMessage } from 'naive-ui'
import type * as MonacoNs from 'monaco-editor'
import { entryContentSave } from '@/api/file'
import { ApiError } from '@/api/http'
import { useAuthStore } from '@/stores/auth'
import { useEditorStore } from '@/stores/editor'
import type { EditorLoadError } from '@/stores/editor'
import { formatBytes, formatDateTime } from '@/utils/format'
import { fileActionError } from '../errors'

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const store = useEditorStore()
const message = useMessage()
const dialog = useDialog()

const containerRef = ref<HTMLElement | null>(null)
const monacoLoading = ref(true)
const saving = ref(false)

let editor: MonacoNs.editor.IStandaloneCodeEditor | null = null
let model: MonacoNs.editor.ITextModel | null = null
let monacoNs: typeof MonacoNs | null = null
let darkQuery: MediaQueryList | null = null

/** 扩展名 → Monaco 语言（局部映射，诚实声明：未识别 → plaintext，PageSpec 第 5 节） */
const LANGUAGE_MAP: ReadonlyArray<readonly [string, string]> = [
  ['.ts', 'typescript'],
  ['.js', 'javascript'],
  ['.mjs', 'javascript'],
  ['.go', 'go'],
  ['.py', 'python'],
  ['.sh', 'shell'],
  ['.md', 'markdown'],
  ['.json', 'json'],
  ['.yaml', 'yaml'],
  ['.yml', 'yaml'],
  ['.conf', 'ini'],
  ['.ini', 'ini'],
  ['.sql', 'sql'],
  ['.css', 'css'],
  ['.html', 'html'],
  ['.xml', 'xml'],
  ['.c', 'c'],
  ['.h', 'c'],
  ['.cpp', 'cpp'],
  ['.java', 'java'],
  ['.rs', 'rust']
]

function languageOf(path: string): string {
  const lower = path.toLowerCase()
  for (const [ext, lang] of LANGUAGE_MAP) {
    if (lower.endsWith(ext)) return lang
  }
  return 'plaintext'
}

/**
 * Monaco 按需装配（动态 import，懒加载 chunk；readonly/缺 path 不触达）。
 * monaco-editor 0.57 根入口（esm/vs/index.js）为 worker-free 形态——语言定义为懒加载
 * monarch 词法（languages/definitions/*），编辑器全程主线程运行，无需 MonacoEnvironment/worker 通道。
 */
async function loadMonaco(): Promise<typeof MonacoNs> {
  return await import('monaco-editor')
}

function editorTheme(): string {
  return darkQuery?.matches === true ? 'vs-dark' : 'vs'
}

/** 拒绝进入的错误分流（PageSpec 第 5 节：FILE_006 warning / FILE_002、FILE_003 错误提示，均返回） */
function onLoadError(kind: EditorLoadError): void {
  if (kind === 'not_editable') {
    message.warning('该文件为二进制或超过 1MiB，请下载后本地编辑')
  } else if (kind === 'out_of_scope') {
    message.error('该路径不受管理（越界或命中排除目录）')
  } else if (kind === 'not_found') {
    message.error('条目不存在（可能已被删除）')
  }
  void router.back()
}

/** 保存（按钮 + Ctrl/Cmd+S 双通道；FILE_011/013 → error 内容保留待重试，api N2） */
async function onSave(): Promise<void> {
  if (editor === null || saving.value || !store.dirty) return
  saving.value = true
  try {
    const data = await entryContentSave(store.path, editor.getValue())
    message.success('已保存')
    store.applySaved(data, editor.getValue())
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

/** 窗口级 Ctrl/Cmd+S（编辑器外焦点时兜底；编辑器内由 monaco addCommand 承载） */
function onWindowKeydown(e: KeyboardEvent): void {
  if ((e.ctrlKey || e.metaKey) && (e.key === 's' || e.key === 'S')) {
    e.preventDefault()
    void onSave()
  }
}

function onBeforeUnload(e: BeforeUnloadEvent): void {
  e.preventDefault()
  e.returnValue = ''
}

/** dirty → beforeunload 监听配对（watch 绑定/解绑 + onUnmounted 兜底） */
watch(
  () => store.dirty,
  (dirty) => {
    if (dirty) window.addEventListener('beforeunload', onBeforeUnload)
    else window.removeEventListener('beforeunload', onBeforeUnload)
  }
)

/** dirty 路由离开拦截（P3：confirm「有未保存更改，确认离开？」） */
onBeforeRouteLeave(() => {
  if (!store.dirty) return true
  return new Promise<boolean>((resolve) => {
    dialog.warning({
      title: '未保存的更改',
      content: '有未保存更改，确认离开？',
      positiveText: '离开',
      negativeText: '留下',
      onPositiveClick: () => resolve(true),
      onNegativeClick: () => resolve(false),
      onClose: () => resolve(false),
      onMaskClick: () => resolve(false)
    })
  })
})

const metaText = computed(() => {
  if (store.savedMeta === null) return '—'
  return `${formatBytes(store.savedMeta.size)} · ${formatDateTime(store.savedMeta.mtime)}`
})

function backToList(): void {
  void router.push('/files')
}

onMounted(async () => {
  // query path 必带（PageSpec 第 7 节：缺失 → 返回列表）
  const q = route.query.path
  const target = typeof q === 'string' && q !== '' ? q : null
  if (target === null) {
    void router.replace('/files')
    return
  }
  // readonly 角色兜底（readonly 文档第 7 节：入口已负空间，直访兜底跳回）
  if (!authStore.isAdmin) {
    void router.replace('/files')
    return
  }

  const err = await store.load(target)
  if (err !== null) {
    onLoadError(err)
    return
  }

  try {
    monacoNs = await loadMonaco()
  } catch {
    message.error('编辑器加载失败，请重试')
    monacoLoading.value = false
    return
  }

  if (containerRef.value === null || monacoNs === null) return
  darkQuery = window.matchMedia('(prefers-color-scheme: dark)')
  model = monacoNs.editor.createModel(store.content, languageOf(store.path))
  editor = monacoNs.editor.create(containerRef.value, {
    model,
    theme: editorTheme(),
    automaticLayout: true,
    minimap: { enabled: false },
    scrollBeyondLastLine: false,
    fontSize: 13,
    tabSize: 4
  })
  editor.onDidChangeModelContent(() => {
    if (editor === null) return
    store.setDirty(editor.getValue() !== store.content)
  })
  editor.addCommand(monacoNs.KeyMod.CtrlCmd | monacoNs.KeyCode.KeyS, () => void onSave())
  darkQuery.addEventListener('change', onThemeChange)
  window.addEventListener('keydown', onWindowKeydown)
  monacoLoading.value = false
})

function onThemeChange(): void {
  monacoNs?.editor.setTheme(editorTheme())
}

onUnmounted(() => {
  window.removeEventListener('keydown', onWindowKeydown)
  window.removeEventListener('beforeunload', onBeforeUnload)
  darkQuery?.removeEventListener('change', onThemeChange)
  editor?.dispose()
  editor = null
  model?.dispose()
  model = null
  store.reset()
})
</script>

<template>
  <div class="page">
    <n-card size="small">
      <div class="header">
        <div class="file">
          <span class="path" :title="store.path">{{ store.path }}</span>
          <span class="meta">{{ metaText }}</span>
          <n-tag v-if="store.dirty" type="warning" size="small" :bordered="false">未保存</n-tag>
        </div>
        <div class="actions">
          <n-button size="small" @click="backToList">返回列表</n-button>
          <n-button
            size="small"
            type="primary"
            :disabled="!store.dirty"
            :loading="saving"
            @click="onSave"
          >
            保存（Ctrl/Cmd+S）
          </n-button>
        </div>
      </div>
    </n-card>

    <n-card size="small" class="editor-card">
      <n-spin :show="monacoLoading || store.loading" size="small">
        <div ref="containerRef" class="editor-container"></div>
      </n-spin>
    </n-card>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 16px;
  height: calc(100vh - 96px);
}
.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}
.file {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  flex-wrap: wrap;
}
.path {
  font-size: 13px;
  font-weight: 500;
  word-break: break-all;
}
.meta {
  font-size: 12px;
  opacity: 0.6;
}
.actions {
  display: flex;
  gap: 8px;
}
.editor-card {
  flex: 1;
  min-height: 0;
}
.editor-container {
  height: 100%;
  min-height: 420px;
}
</style>
