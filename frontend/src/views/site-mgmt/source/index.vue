<script setup lang="ts">
/**
 * 源码编辑页（PageSpec 4.3，code；路由 /sites/:siteId/source，admin 专属——
 * roles:['admin'] 路由守卫 → readonly 直达 403 边界页，ADR-018/⚠️A17）。
 * 交互流（api D-06 乐观锁）：
 * - 进入 siteSource → content + baseMtime 记忆；SITE_014（超 1MiB）→ 提示下载后本地处理并返回；
 * - 保存 siteSourceSave {content, baseMtime}：code=0 →「已保存并重载生效」+ 更新基线；
 *   SITE_010 → 基线冲突弹窗（重新加载=丢弃本地编辑/取消）；SITE_003 → 回滚 diff 弹窗 +
 *   编辑器内容同步刷新为服务端原样（保存已回滚）+「请修正后重试」；SITE_004 →「内容已保存，
 *   nginx 重载失败」+ output（内容保持已保存态，静默重取基线）；
 * - SITE_014 提交前前端预校验（UTF-8 字节数）优先；
 * - dirty 路由离开确认 + beforeunload；Monaco 动态加载（worker-free 根入口，
 *   file-manager edit 先例），nginx 语法高亮落 ini 词法（Monaco 无内置 nginx 语言）。
 */
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'
import { useDialog, useMessage } from 'naive-ui'
import type * as MonacoNs from 'monaco-editor'
import { siteSource, siteSourceSave } from '@/api/site'
import { ApiError } from '@/api/http'
import { useSitesStore } from '@/stores/site'
import type { SiteRollbackData, SiteStaleData } from '@/types/site'
import { fieldOfString, isSiteRollbackData, isSiteStaleData } from '@/types/site'
import { formatBytes } from '@/utils/format'
import { siteActionError } from '../errors'
import RollbackDiffModal from '../components/RollbackDiffModal.vue'

/** SOURCE_SIZE_LIMIT = 1MiB（api 3.4 业务常量，SITE_014 前端预校验口径） */
const SOURCE_SIZE_LIMIT = 1024 * 1024

const route = useRoute()
const router = useRouter()
const sitesStore = useSitesStore()
const message = useMessage()
const dialog = useDialog()

const containerRef = ref<HTMLElement | null>(null)
const loading = ref(true)
const saving = ref(false)

/** 路由参数 siteId（vue-router 解码；编码见跳转方 encodeURIComponent） */
const siteId = computed(() => {
  const v = route.params.siteId
  return typeof v === 'string' ? v : Array.isArray(v) ? (v[0] ?? '') : ''
})

const sourceFile = ref('')
const baselineContent = ref('')
const baseMtime = ref('')
const sourceSize = ref(0)
const dirty = ref(false)
/** 基线过期（SITE_010 后置位，重新加载后复位） */
const stale = ref(false)

/** 站点标识（serverNames 来自 sitesStore 投影；未命中回退 siteId，不虚构） */
const siteLabel = computed(() => {
  const site = sitesStore.findBySiteId(siteId.value)
  return site !== null ? site.serverNames.join(', ') : siteId.value
})

let editor: MonacoNs.editor.IStandaloneCodeEditor | null = null
let model: MonacoNs.editor.ITextModel | null = null
let monacoNs: typeof MonacoNs | null = null
let darkQuery: MediaQueryList | null = null

async function loadMonaco(): Promise<typeof MonacoNs> {
  return await import('monaco-editor')
}

/** 站点配置文件原文（nginx 语法——Monaco 无内置 nginx 语言，落 ini 词法：同为指令/键值风格） */
function languageOf(): string {
  return 'ini'
}

function editorTheme(): string {
  return darkQuery?.matches === true ? 'vs-dark' : 'vs'
}

/** ---- 装载（SITE_014 拒载 → 提示下载后本地处理并返回） ---- */

async function loadSource(): Promise<boolean> {
  loading.value = true
  try {
    const data = await siteSource(siteId.value)
    sourceFile.value = data.sourceFile
    baselineContent.value = data.content
    baseMtime.value = data.baseMtime
    sourceSize.value = data.size
    stale.value = false
    dirty.value = false
    if (model !== null) model.setValue(data.content)
    return true
  } catch (e) {
    if (e instanceof ApiError) {
      if (e.is('SITE_014')) {
        message.warning('文件超过 1MiB 编辑上限，请下载后本地处理')
      } else {
        const info = siteActionError(e)
        if (info !== null) message.error(info.text)
      }
    }
    void router.replace('/sites')
    return false
  } finally {
    loading.value = false
  }
}

/** ---- 保存（SITE_010/SITE_003/SITE_004/SITE_014 分流） ---- */

const conflict = ref<SiteStaleData | null>(null)
const rollback = ref<SiteRollbackData | null>(null)

async function onSave(): Promise<void> {
  if (editor === null || saving.value) return
  const content = editor.getValue()
  // SITE_014 前端预校验优先（UTF-8 字节数 ≤ 1MiB）
  if (new TextEncoder().encode(content).length > SOURCE_SIZE_LIMIT) {
    message.warning('内容超过 1MiB 上限（SITE_014），请精简后保存或下载后本地处理')
    return
  }
  saving.value = true
  try {
    const data = await siteSourceSave(siteId.value, content, baseMtime.value)
    message.success('已保存并重载生效')
    baseMtime.value = data.baseMtime
    baselineContent.value = content
    stale.value = false
    dirty.value = false
  } catch (e) {
    if (e instanceof ApiError) {
      if (e.is('SITE_010')) {
        if (isSiteStaleData(e.data)) {
          conflict.value = e.data
          stale.value = true
          return
        }
      }
      if (e.is('SITE_003')) {
        if (isSiteRollbackData(e.data)) {
          rollback.value = e.data
          return
        }
      }
      if (e.is('SITE_004')) {
        // 内容已保存（文件已落盘），nginx 重载失败为环境性原因（D-05）：内容保持已保存态，
        // 静默重取基线（mtime 已变，不重取将误报 SITE_010）
        const output = fieldOfString(e.data, 'output') ?? '（无输出）'
        dialog.error({
          title: '内容已保存，nginx 重载失败',
          content: `${output}\n（请检查环境后手动重试重载）`
        })
        await refreshBaselineOnly()
        return
      }
      const info = siteActionError(e)
      if (info !== null) message.error(info.text)
    }
  } finally {
    saving.value = false
  }
}

/** 仅重取基线（SITE_004 后：服务端内容与编辑器一致，重取对齐 baseMtime/size） */
async function refreshBaselineOnly(): Promise<void> {
  try {
    const data = await siteSource(siteId.value)
    baseMtime.value = data.baseMtime
    sourceSize.value = data.size
    if (editor !== null && editor.getValue() === data.content) baselineContent.value = data.content
  } catch {
    // 静默失败：下次保存将以 SITE_010 如实暴露
  }
}

/** SITE_003 回滚弹窗关闭：文件为原内容——编辑器同步刷新为服务端原样 + 提示修正后重试 */
async function onRollbackClosed(): Promise<void> {
  rollback.value = null
  message.warning('保存已回滚，文件为修改前原内容；请修正后重试')
  await loadSource()
}

/** SITE_010 冲突弹窗「重新加载」：强确认丢弃本地编辑 */
function onConflictReload(): void {
  dialog.warning({
    title: '重新加载（丢弃本地编辑）',
    content: '重新加载将丢弃当前全部未保存修改，以服务端最新内容为准。确认继续？',
    positiveText: '丢弃并重新加载',
    negativeText: '取消',
    onPositiveClick: () => {
      conflict.value = null
      void loadSource()
    }
  })
}

/** ---- dirty 拦截（路由离开 + beforeunload） ---- */

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

watch(dirty, (v) => {
  if (v) window.addEventListener('beforeunload', onBeforeUnload)
  else window.removeEventListener('beforeunload', onBeforeUnload)
})

onBeforeRouteLeave(() => {
  if (!dirty.value) return true
  return new Promise<boolean>((resolve) => {
    dialog.warning({
      title: '未保存的修改',
      content: '未保存的修改将丢失，确认离开？',
      positiveText: '离开',
      negativeText: '留下',
      onPositiveClick: () => resolve(true),
      onNegativeClick: () => resolve(false),
      onClose: () => resolve(false),
      onMaskClick: () => resolve(false)
    })
  })
})

function backToList(): void {
  void router.push('/sites')
}

function onThemeChange(): void {
  monacoNs?.editor.setTheme(editorTheme())
}

onMounted(async () => {
  if (siteId.value === '') {
    void router.replace('/sites')
    return
  }
  // 顶栏站点标识数据源（admin 页面权限内）；装载失败不阻断编辑
  void sitesStore.loadList()

  const ok = await loadSource()
  if (!ok) return

  try {
    monacoNs = await loadMonaco()
  } catch {
    message.error('编辑器加载失败，请重试')
    return
  }

  if (containerRef.value === null || monacoNs === null) return
  darkQuery = window.matchMedia('(prefers-color-scheme: dark)')
  model = monacoNs.editor.createModel(baselineContent.value, languageOf())
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
    dirty.value = editor.getValue() !== baselineContent.value
  })
  editor.addCommand(monacoNs.KeyMod.CtrlCmd | monacoNs.KeyCode.KeyS, () => void onSave())
  darkQuery.addEventListener('change', onThemeChange)
  window.addEventListener('keydown', onWindowKeydown)
  loading.value = false
})

onUnmounted(() => {
  window.removeEventListener('keydown', onWindowKeydown)
  window.removeEventListener('beforeunload', onBeforeUnload)
  darkQuery?.removeEventListener('change', onThemeChange)
  editor?.dispose()
  editor = null
  model?.dispose()
  model = null
})
</script>

<template>
  <div class="page">
    <n-card size="small">
      <div class="header">
        <div class="site">
          <span class="title">{{ siteLabel }}</span>
          <span class="meta" :title="sourceFile">{{ sourceFile }}</span>
          <span class="meta">{{ formatBytes(sourceSize) }}</span>
          <n-tag v-if="stale" type="warning" size="small" :bordered="false">基线过期</n-tag>
          <n-tag v-else type="success" size="small" :bordered="false">基线一致</n-tag>
          <n-tag v-if="dirty" type="error" size="small" :bordered="false">未保存</n-tag>
        </div>
        <div class="actions">
          <n-button size="small" @click="backToList">返回列表</n-button>
          <n-button size="small" type="primary" :loading="saving" :disabled="!dirty" @click="onSave">
            保存（Ctrl/Cmd+S）
          </n-button>
        </div>
      </div>
    </n-card>

    <n-card size="small" class="editor-card">
      <n-spin :show="loading" size="small">
        <div ref="containerRef" class="editor-container"></div>
      </n-spin>
    </n-card>

    <!-- SITE_010 基线冲突（D-06 乐观锁）：重新加载=丢弃本地编辑 / 取消 -->
    <n-modal
      :show="conflict !== null"
      preset="card"
      title="配置已被外部修改（基线冲突）"
      style="width: 520px"
      :mask-closable="false"
      @update:show="(v: boolean) => { if (!v) conflict = null }"
    >
      <template v-if="conflict !== null">
        <n-alert type="warning" :show-icon="true">
          文件在打开后被面板外渠道修改（shell / file-manager 通道，SITE_010）。
        </n-alert>
        <div class="conflict-baseline">
          当前基线 mtime：<span class="mono">{{ baseMtime }}</span>
        </div>
        <div class="conflict-baseline">
          服务端最新 mtime：<span class="mono">{{ conflict.currentMtime }}</span>
        </div>
      </template>
      <template #footer>
        <div class="footer">
          <n-button @click="conflict = null">取消</n-button>
          <n-button type="warning" @click="onConflictReload">重新加载（丢弃本地编辑）</n-button>
        </div>
      </template>
    </n-modal>

    <!-- SITE_003 回滚 diff 弹窗（保存已回滚，文件为原内容） -->
    <rollback-diff-modal :data="rollback" @close="onRollbackClosed" />
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
.site {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  flex-wrap: wrap;
}
.title {
  font-size: 13px;
  font-weight: 500;
  word-break: break-all;
}
.meta {
  font-size: 12px;
  opacity: 0.6;
  max-width: 420px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
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
.conflict-baseline {
  margin-top: 10px;
  font-size: 13px;
}
.mono {
  font-family: Consolas, Menlo, monospace;
  font-size: 12px;
}
.footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
</style>
