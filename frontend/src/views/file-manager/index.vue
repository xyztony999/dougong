<script setup lang="ts">
/**
 * 文件管理列表页（PageSpec 第 4 节，路由 /files）。
 * admin 全功能；readonly 子集（ADR-018 负空间：无工具栏写按钮/无选择列/无操作列/
 * 无拖拽监听/名称列 file 点击无动作——20 个 ✗ 接口无入口）。
 * 刷新纪律（D-03）：手动 + 操作驱动无轮询；内容类操作成功重拉、元数据类（chmod/chown）行直更。
 * 上传管线（4.5，D-04/D-10）：≤5MiB 直传、>5MiB 逐文件分块（chunk-init→chunk×n→chunk-complete），
 * 冲突 ask→FILE_005 统一决策 Modal（4.4.8）；分块同名冲突无覆盖通道（如实承接 api 契约）。
 */
import { computed, h, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useDialog, useMessage } from 'naive-ui'
import { NButton, NIcon, NPopconfirm } from 'naive-ui'
import type { DataTableColumns, DropdownOption, SelectOption } from 'naive-ui'
import {
  AddOutline,
  ArchiveOutline,
  ArrowDownOutline,
  ArrowUpOutline,
  CloudUploadOutline,
  GridOutline,
  ListOutline,
  SettingsOutline
} from '@vicons/ionicons5'
import { ApiError } from '@/api/http'
import { chunkComplete, chunkInit, chunkUpload, entryDelete, entryDownload, entryUpload } from '@/api/file'
import { useAuthStore } from '@/stores/auth'
import { useFileStore } from '@/stores/file'
import {
  DIRECT_UPLOAD_LIMIT,
  ENTRY_SORT_FIELD,
  FILE_ERROR_TEXT,
  UPLOAD_CHUNK_SIZE,
  isConflictList
} from '@/types/file'
import type { EntrySortField, FileEntry } from '@/types/file'
import { formatBytes, formatDateTime } from '@/utils/format'
import { fileActionError } from './errors'
import { entryIcon, isArchiveName } from './icons'
import type { ConflictPayload } from './components/ConflictModal.vue'
import ArchiveModal from './components/ArchiveModal.vue'
import ChmodModal from './components/ChmodModal.vue'
import ChownModal from './components/ChownModal.vue'
import ConflictModal from './components/ConflictModal.vue'
import CreateEntryModal from './components/CreateEntryModal.vue'
import MoveModal from './components/MoveModal.vue'
import PathScopeModal from './components/PathScopeModal.vue'
import RenameModal from './components/RenameModal.vue'
import UnarchiveModal from './components/UnarchiveModal.vue'

const store = useFileStore()
const authStore = useAuthStore()
const message = useMessage()
const dialog = useDialog()
const router = useRouter()

/** ---- 面包屑（4.1：currentPath 逐段拆分，受管根为根段；逐级点击=切换目录重拉） ---- */

interface Crumb {
  label: string
  path: string
}

const displayPath = computed(() => store.resolvedPath ?? store.currentPath)

const crumbs = computed<Crumb[]>(() => {
  const segs = displayPath.value.split('/').filter(Boolean)
  const list: Crumb[] = [{ label: '受管根', path: '' }]
  segs.forEach((seg, i) => {
    list.push({ label: seg, path: `/${segs.slice(0, i + 1).join('/')}` })
  })
  return list
})

const atRoot = computed(() => displayPath.value.split('/').filter(Boolean).length === 0)

function goUp(): void {
  const segs = displayPath.value.split('/').filter(Boolean)
  segs.pop()
  void store.navigate(segs.length === 0 ? '' : `/${segs.join('/')}`)
}

const generatedAtText = computed(() => formatDateTime(store.generatedAt))

/** ---- 排序（服务端排序 D-03；变更即重拉） ---- */

const sortFieldOptions: SelectOption[] = (Object.keys(ENTRY_SORT_FIELD) as EntrySortField[]).map((v) => ({
  label: ENTRY_SORT_FIELD[v],
  value: v
}))

function onSortFieldChange(v: EntrySortField): void {
  store.setSort(v, store.sortParams.order)
}

function toggleOrder(): void {
  store.setSort(store.sortParams.sort, store.sortParams.order === 'asc' ? 'desc' : 'asc')
}

const orderAscending = computed(() => store.sortParams.order === 'asc')

/** 卡片标题（当前浏览目录；root 显示「受管根」） */
const currentTitle = computed(() => (store.currentPath === '' ? '受管根' : store.currentPath))

/** 写操作寻址目录（受管根下=rootPath 具体路径；E1 修复——新建/移动起始用） */
const writeDir = computed(() => store.writeDir())

/** 图标视图卡片点击：与名称列同语义（directory→进入；file→admin 打开编辑；symlink→无动作） */
function onCardOpen(row: FileEntry): void {
  if (row.entryType === 'directory') void store.navigate(row.path)
  else if (row.entryType === 'file' && authStore.isAdmin) openEditor(row)
}

/** ---- 编辑页跳转（4.2 名称列 file 点击 / 4.3 编辑按钮；readonly 无动作） ---- */

function openEditor(row: FileEntry): void {
  void router.push({ path: '/files/edit', query: { path: row.path } })
}

/** ---- 行操作集（4.3：可见性由 entryType + 归档扩展名识别控制，R-14 职责分离） ---- */

interface RowAction {
  key: string
  label: string
}

function rowActions(row: FileEntry): RowAction[] {
  const list: RowAction[] = []
  if (row.entryType === 'file') {
    list.push({ key: 'download', label: '下载' }, { key: 'edit', label: '编辑' })
  }
  list.push({ key: 'rename', label: '重命名' }, { key: 'move', label: '移动' })
  if (isArchiveName(row.name)) list.push({ key: 'unarchive', label: '解压' })
  list.push({ key: 'chmod', label: '权限' }, { key: 'chown', label: '属主' }, { key: 'delete', label: '删除' })
  return list
}

/** ---- Dialog 状态（4.4 集合） ---- */

const createShow = ref(false)
const renameShow = ref(false)
const renameTarget = ref<FileEntry | null>(null)
const moveShow = ref(false)
const moveTarget = ref<FileEntry | null>(null)
const chmodShow = ref(false)
const chmodTarget = ref<FileEntry | null>(null)
const chownShow = ref(false)
const chownTarget = ref<FileEntry | null>(null)
const archiveShow = ref(false)
const unarchiveShow = ref(false)
const unarchiveTarget = ref<FileEntry | null>(null)
const pathScopeShow = ref(false)

/** FILE_005 统一冲突决策（4.4.8；上传/移动/解压共用） */
const pendingConflict = ref<ConflictPayload | null>(null)

function onAction(key: string, row: FileEntry): void {
  switch (key) {
    case 'download':
      void onDownload(row)
      break
    case 'edit':
      openEditor(row)
      break
    case 'rename':
      renameTarget.value = row
      renameShow.value = true
      break
    case 'move':
      moveTarget.value = row
      moveShow.value = true
      break
    case 'unarchive':
      unarchiveTarget.value = row
      unarchiveShow.value = true
      break
    case 'chmod':
      chmodTarget.value = row
      chmodShow.value = true
      break
    case 'chown':
      chownTarget.value = row
      chownShow.value = true
      break
    case 'delete':
      // 图标视图菜单承载（列表视图按钮层为 NPopconfirm 直触发，不经此分支）——同文案轻确认（R-17）
      dialog.warning({
        title: '删除',
        content: '确认删除？条目将移入回收站，可在回收站还原。',
        positiveText: '删除',
        negativeText: '取消',
        onPositiveClick: () => void onDelete(row)
      })
      break
  }
}

/** ---- 错误分流（第 1 节：FILE_001~015 逐码分流；FILE_005 冲突流在调用方处理） ---- */

function handleActionError(e: unknown): void {
  if (!(e instanceof ApiError)) return
  if (e.is('FILE_005')) return // 冲突流由调用方承接（此处兜底静默，避免双提示）
  const info = fileActionError(e)
  if (info === null) return
  if (info.level === 'warning') message.warning(info.text)
  else message.error(info.text)
  if (info.refreshAfter) void store.loadList()
}

/** ---- 删除（4.3：R-17 可逆删除轻确认；批量=逐行循环+单一确认+失败汇总不回滚） ---- */

async function onDelete(row: FileEntry): Promise<void> {
  store.setInFlight(row.path)
  try {
    await entryDelete(row.path)
    await store.loadList() // 改变目录内容类 → 重拉
  } catch (e) {
    handleActionError(e)
  } finally {
    store.releaseInFlight(row.path)
  }
}

async function onBatchDelete(): Promise<void> {
  const targets = [...store.selectedEntries]
  if (targets.length === 0) return
  const failures: string[] = []
  for (const t of targets) {
    store.setInFlight(t.path)
    try {
      await entryDelete(t.path)
    } catch {
      failures.push(t.name) // 部分失败汇总呈现；成功项不回滚（各自已入回收站）
    } finally {
      store.releaseInFlight(t.path)
    }
  }
  store.clearSelection()
  if (failures.length > 0) {
    message.warning(`部分删除失败（成功项已入回收站不回滚）：${failures.join('、')}`)
  }
  await store.loadList()
}

/** ---- 下载（4.3：entryType=file；window 触发流下载，审计 security D-08 前端无感知） ---- */

async function onDownload(row: FileEntry): Promise<void> {
  store.setInFlight(row.path)
  try {
    await entryDownload(row.path)
  } catch (e) {
    handleActionError(e)
  } finally {
    store.releaseInFlight(row.path)
  }
}

/** ---- 上传管线（4.5：直传 ≤5MiB；任一 >5MiB 整组走分块逐文件三段式） ---- */

const canUpload = computed(() => authStore.isAdmin)
const fileInputRef = ref<HTMLInputElement | null>(null)
const uploading = ref(false)
const dragActive = ref(false)
const uploadProgress = ref<{ fileName: string; percent: number } | null>(null)

function pickFiles(): void {
  fileInputRef.value?.click()
}

function onInputChange(e: Event): void {
  const input = e.target as HTMLInputElement
  const files = Array.from(input.files ?? [])
  input.value = '' // 允许再次选择同一文件
  if (files.length > 0) void handleUpload(files)
}

function onDragOver(e: DragEvent): void {
  if (!canUpload.value) return
  e.preventDefault() // dragover preventDefault 后 drop 才触发
  dragActive.value = true
}

function onDragLeave(): void {
  dragActive.value = false
}

function onDrop(e: DragEvent): void {
  dragActive.value = false
  if (!canUpload.value) return
  e.preventDefault()
  const files = Array.from(e.dataTransfer?.files ?? [])
  if (files.length > 0) void handleUpload(files)
}

async function handleUpload(files: File[]): Promise<void> {
  if (uploading.value || files.length === 0) return
  uploading.value = true
  const dir = store.writeDir() // 受管根下用 rootPath 具体路径（upload dir 空串 FILE_001，E1 修复）
  try {
    if (files.every((f) => f.size <= DIRECT_UPLOAD_LIMIT)) {
      await directUpload(dir, files)
    } else {
      await chunkedUpload(dir, files)
    }
  } finally {
    uploading.value = false
    uploadProgress.value = null
  }
}

/** 直传（≤5MiB/文件）：entryUpload onConflict=ask → FILE_005 → 4.4.8 决策重发 */
async function directUpload(dir: string, files: File[]): Promise<void> {
  try {
    const data = await entryUpload(dir, files, 'ask')
    message.success(`已上传 ${data.uploadedCount} 个文件`)
    await store.loadList()
  } catch (e) {
    if (e instanceof ApiError && e.is('FILE_005') && isConflictList(e.data)) {
      pendingConflict.value = {
        conflicts: e.data,
        allowSkip: true,
        retry: async (strategy) => {
          const data = await entryUpload(dir, files, strategy)
          message.success(`已上传 ${data.uploadedCount} 个文件`)
          await store.loadList()
        }
      }
      return
    }
    handleActionError(e)
  }
}

/** 分块（>5MiB 触发；逐文件：chunk-init → chunk×n（NProgress=已完成块/总块）→ chunk-complete） */
async function chunkedUpload(dir: string, files: File[]): Promise<void> {
  let successCount = 0
  for (const f of files) {
    const totalChunks = Math.max(1, Math.ceil(f.size / UPLOAD_CHUNK_SIZE))
    uploadProgress.value = { fileName: f.name, percent: 0 }
    try {
      const init = await chunkInit(dir, f.name, f.size, totalChunks)
      for (let i = 0; i < totalChunks; i++) {
        const blob = f.slice(i * UPLOAD_CHUNK_SIZE, Math.min((i + 1) * UPLOAD_CHUNK_SIZE, f.size))
        await chunkUpload(init.uploadId, i, blob)
        uploadProgress.value = { fileName: f.name, percent: Math.round(((i + 1) / totalChunks) * 100) }
      }
      await chunkComplete(init.uploadId)
      successCount += 1
    } catch (e) {
      // chunk-init 同名预检 FILE_005：api 无 onConflict 参数——提示改名/取消，不虚构覆盖通道（4.5 修正声明）
      if (e instanceof ApiError && e.is('FILE_005')) {
        message.warning(`目标已存在同名文件：${f.name}（请本地重命名后重传或取消）`)
        continue
      }
      if (e instanceof ApiError && (e.is('FILE_010') || e.is('FILE_013'))) {
        // D-10：中断即整任务作废（无断点续传 OUT-4）；终止该文件管线，继续下一文件
        message.error(`「${f.name}」上传已终止（任务作废，重传需重新开始）`)
        continue
      }
      handleActionError(e)
    }
  }
  if (successCount > 0) {
    message.success(`已上传 ${successCount} 个文件`)
    await store.loadList()
  }
}

/** ---- 冲突决策（4.4.8：覆盖/跳过=携带决策重发；取消=放弃不重发） ---- */

async function onConflictDecide(strategy: 'overwrite' | 'skip'): Promise<void> {
  const payload = pendingConflict.value
  pendingConflict.value = null
  if (payload === undefined || payload === null) return
  try {
    await payload.retry(strategy)
  } catch (e) {
    handleActionError(e)
  }
}

function onConflictCancel(): void {
  pendingConflict.value = null
}

/** ---- 表格列（4.2；readonly 无选择列/无操作列，名称列仍 fixed:left） ---- */

function renderName(row: FileEntry): ReturnType<typeof h> {
  const icon = entryIcon(row.name, row.entryType)
  const iconNode = h(NIcon, { component: icon.icon, color: icon.color, size: 18 })
  const wrap = (children: ReturnType<typeof h>[]): ReturnType<typeof h> =>
    h('span', { style: { display: 'inline-flex', alignItems: 'center', gap: '6px', minWidth: 0 } }, children)
  if (row.entryType === 'directory') {
    return wrap([
      iconNode,
      h(
        NButton,
        { text: true, type: 'primary', onClick: () => void store.navigate(row.path) },
        { default: () => row.name }
      )
    ])
  }
  if (row.entryType === 'symlink') {
    // 链接图标（紫）+ 名称尾注「→ 目标」；不展开目标（api 4.2.4）
    return wrap([
      iconNode,
      h('span', { style: { overflow: 'hidden', textOverflow: 'ellipsis' } }, row.name),
      h('span', { style: { fontSize: '12px', opacity: 0.55, flexShrink: 0 } }, '→ 目标')
    ])
  }
  if (authStore.isAdmin) {
    return wrap([
      iconNode,
      h(NButton, { text: true, type: 'primary', onClick: () => openEditor(row) }, { default: () => row.name })
    ])
  }
  // readonly：file 点击无动作（编辑为负空间，readonly 文档第 2 节）
  return wrap([iconNode, h('span', { style: { overflow: 'hidden', textOverflow: 'ellipsis' } }, row.name)])
}

const columns = computed<DataTableColumns<FileEntry>>(() => {
  const base: DataTableColumns<FileEntry> = []
  if (authStore.isAdmin) {
    base.push({ type: 'selection', width: 50 }) // 多选（压缩 4.4.9 + 批量删除 4.3）
  }
  base.push(
    { title: '名称', key: 'name', width: 260, fixed: 'left', render: renderName },
    {
      title: '类型',
      key: 'entryType',
      width: 100,
      render: (row) => {
        const icon = entryIcon(row.name, row.entryType)
        return h('span', { style: { display: 'inline-flex', alignItems: 'center', gap: '4px' } }, [
          h(NIcon, { component: icon.icon, color: icon.color, size: 14 }),
          row.entryType === 'file' ? '文件' : row.entryType === 'directory' ? '目录' : '符号链接'
        ])
      }
    },
    {
      title: '大小',
      key: 'size',
      width: 120,
      align: 'right',
      render: (row) => formatBytes(row.size) // 目录恒 0（api 4.2.1）；B/KB/MB 前端格式化
    },
    { title: '权限', key: 'mode', width: 110, render: (row) => row.mode }, // 八进制原值（PageSpec 4.2 从简）
    {
      title: '属主',
      key: 'owner',
      width: 160,
      render: (row) => `${row.ownerUser} / ${row.ownerGroup}`
    },
    { title: '修改时间', key: 'mtime', width: 180, render: (row) => formatDateTime(row.mtime) }
  )
  if (authStore.isAdmin) {
    base.push({
      title: '操作',
      key: 'actions',
      width: 320,
      fixed: 'right',
      render: (row) => {
        const inFlight = store.isInFlight(row.path)
        return h(
          'div',
          { style: { display: 'flex', flexWrap: 'wrap', gap: '4px' } },
          rowActions(row).map((a) => {
            if (a.key === 'delete') {
              return h(
                NPopconfirm,
                { onPositiveClick: () => void onDelete(row) },
                {
                  trigger: () =>
                    h(
                      NButton,
                      { size: 'small', type: 'error', ghost: true, disabled: inFlight },
                      { default: () => a.label }
                    ),
                  default: () => '确认删除？条目将移入回收站，可在回收站还原。'
                }
              )
            }
            return h(
              NButton,
              { size: 'small', secondary: true, disabled: inFlight, onClick: () => onAction(a.key, row) },
              { default: () => a.label }
            )
          })
        )
      }
    })
  }
  return base
})

/** readonly 视角无 fixed:right 操作列（负空间；名称列仍 fixed:left） */
const scrollX = computed(() => (authStore.isAdmin ? 1300 : 930))

function rowKey(row: FileEntry): string {
  return row.path
}

function onCheckedChange(keys: (string | number)[]): void {
  store.setSelection(keys.map(String))
}

/** ---- 图标视图（4.2：NGrid 卡片=图标+名称；操作经选中/卡片菜单，同一 Action 集） ---- */

function dropdownOptions(row: FileEntry): DropdownOption[] {
  return rowActions(row).map((a) => ({ key: a.key, label: a.label }))
}

/** ---- 兜底提示（store 无 Message 上下文，页面 watch——service-mgmt 先例） ---- */

watch(
  () => store.listBizError,
  (msg) => {
    if (msg !== null) message.warning(msg)
  }
)

/** FILE_002 返回受管根（4.2：重置到受管根重拉） */
function backToRoot(): void {
  void store.navigate('')
}

/** 进入页面拉取（D-03）；离开清理（选择/inFlight/picker 态，PageSpec 第 2 节刷新纪律末行） */
onMounted(() => {
  void store.loadList()
})

onUnmounted(() => {
  store.resetView()
})
</script>

<template>
  <div class="page">
    <n-card size="small">
      <div class="toolbar">
        <div class="nav">
          <n-button size="small" :disabled="atRoot" @click="goUp">
            <template #icon>
              <n-icon :component="ArrowUpOutline" />
            </template>
            上级
          </n-button>
          <n-breadcrumb>
            <n-breadcrumb-item v-for="c in crumbs" :key="c.path" @click="store.navigate(c.path)">
              {{ c.label }}
            </n-breadcrumb-item>
          </n-breadcrumb>
        </div>
        <div class="ctrl">
          <n-select
            :value="store.sortParams.sort"
            :options="sortFieldOptions"
            size="small"
            class="ctrl-sort"
            @update:value="(v: EntrySortField) => onSortFieldChange(v)"
          />
          <n-button size="small" @click="toggleOrder">
            <template #icon>
              <n-icon :component="orderAscending ? ArrowUpOutline : ArrowDownOutline" />
            </template>
            {{ orderAscending ? '升序' : '降序' }}
          </n-button>
          <n-button-group size="small">
            <n-button :type="store.viewMode === 'list' ? 'primary' : 'default'" @click="store.setViewMode('list')">
              <template #icon>
                <n-icon :component="ListOutline" />
              </template>
              列表
            </n-button>
            <n-button :type="store.viewMode === 'grid' ? 'primary' : 'default'" @click="store.setViewMode('grid')">
              <template #icon>
                <n-icon :component="GridOutline" />
              </template>
              图标
            </n-button>
          </n-button-group>
          <template v-if="authStore.isAdmin">
            <input
              ref="fileInputRef"
              type="file"
              multiple
              class="upload-input"
              @change="onInputChange"
            />
            <n-button size="small" type="primary" :loading="uploading" @click="pickFiles">
              <template #icon>
                <n-icon :component="CloudUploadOutline" />
              </template>
              上传
            </n-button>
            <n-button size="small" @click="createShow = true">
              <template #icon>
                <n-icon :component="AddOutline" />
              </template>
              新建
            </n-button>
            <n-button size="small" :disabled="store.selection.length === 0" @click="archiveShow = true">
              <template #icon>
                <n-icon :component="ArchiveOutline" />
              </template>
              压缩
            </n-button>
            <n-button size="small" quaternary @click="pathScopeShow = true">
              <template #icon>
                <n-icon :component="SettingsOutline" />
              </template>
              路径边界设置
            </n-button>
          </template>
          <n-button size="small" :loading="store.listLoading" @click="store.loadList()">刷新</n-button>
        </div>
      </div>
      <div class="meta-row">
        <span class="meta">
          共 {{ store.total }} 项<template v-if="store.generatedAt !== null"> · 数据生成于 {{ generatedAtText }}</template>
        </span>
        <span v-if="authStore.isAdmin && store.selection.length > 0" class="meta">
          已选 {{ store.selection.length }} 项
          <n-popconfirm @positive-click="onBatchDelete">
            <template #trigger>
              <n-button size="tiny" type="error" ghost>删除所选</n-button>
            </template>
            确认删除所选 {{ store.selection.length }} 项？将移入回收站。
          </n-popconfirm>
        </span>
      </div>
      <div v-if="uploadProgress !== null" class="upload-progress">
        <span class="meta">{{ uploadProgress.fileName }}（分块上传中）</span>
        <n-progress type="line" :percentage="uploadProgress.percent" :height="8" :show-indicator="false" />
      </div>
    </n-card>

    <!-- FILE_002 页面级横幅（正常导航不可达，符号链接外指等兜底） -->
    <n-card v-if="store.scopeError !== null" size="small">
      <n-alert type="error" :title="FILE_ERROR_TEXT['FILE_002']">
        <n-button size="small" @click="backToRoot">返回受管根</n-button>
      </n-alert>
    </n-card>

    <n-card v-else size="small" :title="currentTitle">
      <!-- 拖拽放置（admin；readonly 监听不注册——负空间，PageSpec readonly 第 2 节） -->
      <div
        class="dropzone"
        :class="{ dragging: dragActive }"
        v-on="authStore.isAdmin ? { dragover: onDragOver, dragleave: onDragLeave, drop: onDrop } : {}"
      >
        <n-data-table
          v-if="store.viewMode === 'list'"
          :columns="columns"
          :data="store.entries"
          :loading="store.listLoading"
          :scroll-x="scrollX"
          :row-key="rowKey"
          :checked-row-keys="store.selection"
          @update:checked-row-keys="onCheckedChange"
        >
          <template #empty>
            <n-empty description="此目录为空" class="empty" />
          </template>
        </n-data-table>

        <template v-else>
          <n-spin :show="store.listLoading" size="small">
            <n-empty v-if="store.entries.length === 0" description="此目录为空" class="empty" />
            <n-grid v-else cols="2 s:3 m:4 l:5" responsive="screen" :x-gap="10" :y-gap="10">
              <n-gi v-for="row in store.entries" :key="row.path">
                <div class="entry-card" :class="{ selected: store.selection.includes(row.path) }">
                  <div class="card-head">
                    <n-checkbox
                      v-if="authStore.isAdmin"
                      :checked="store.selection.includes(row.path)"
                      @update:checked="(v: boolean) => store.toggleSelection(row.path, v)"
                    />
                    <n-dropdown
                      v-if="authStore.isAdmin"
                      trigger="click"
                      :options="dropdownOptions(row)"
                      @select="(key: string) => onAction(key, row)"
                    >
                      <n-button size="tiny" quaternary>操作</n-button>
                    </n-dropdown>
                  </div>
                  <div class="card-body" @click="onCardOpen(row)">
                    <n-icon
                      :component="entryIcon(row.name, row.entryType).icon"
                      :color="entryIcon(row.name, row.entryType).color"
                      :size="34"
                    />
                    <span class="card-name" :title="row.name">{{ row.name }}</span>
                  </div>
                </div>
              </n-gi>
            </n-grid>
          </n-spin>
        </template>
      </div>
    </n-card>

    <!-- Dialog 集合（4.4；readonly 触发入口均不渲染，整组不挂载不触发） -->
    <template v-if="authStore.isAdmin">
      <create-entry-modal v-model:show="createShow" :dir="writeDir" @created="store.loadList()" />
      <rename-modal v-model:show="renameShow" :entry="renameTarget" @renamed="store.loadList()" />
      <move-modal
        v-model:show="moveShow"
        :entry="moveTarget"
        :start-dir="writeDir"
        @moved="store.loadList()"
        @conflict="(p: ConflictPayload) => (pendingConflict = p)"
      />
      <chmod-modal
        v-model:show="chmodShow"
        :entry="chmodTarget"
        @done="(path: string, mode: string) => store.applyMetaPatch(path, { mode })"
      />
      <chown-modal
        v-model:show="chownShow"
        :entry="chownTarget"
        @done="(path: string, u: string, g: string) => store.applyMetaPatch(path, { ownerUser: u, ownerGroup: g })"
      />
      <archive-modal v-model:show="archiveShow" :selected="store.selectedEntries" @done="store.loadList()" />
      <unarchive-modal
        v-model:show="unarchiveShow"
        :entry="unarchiveTarget"
        @done="store.loadList()"
        @conflict="(p: ConflictPayload) => (pendingConflict = p)"
      />
      <path-scope-modal
        v-model:show="pathScopeShow"
        @updated="
          () => {
            store.invalidateRoot() // 受管根可能已变：失效 rootPath，由本次 root-default 重解（E1 修复）
            void store.loadList()
          }
        "
      />
    </template>

    <!-- FILE_005 冲突决策（4.4.8；上传/移动/解压共用） -->
    <conflict-modal :payload="pendingConflict" @decide="onConflictDecide" @cancel="onConflictCancel" />
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.toolbar {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}
.nav {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  min-width: 0;
}
.ctrl {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.ctrl-sort {
  width: 120px;
}
.meta-row {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-top: 10px;
}
.meta {
  font-size: 12px;
  opacity: 0.6;
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
.upload-progress {
  margin-top: 8px;
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-width: 420px;
}
.upload-input {
  display: none;
}
.dropzone {
  border: 1px dashed transparent;
  border-radius: 4px;
  padding: 2px;
  min-height: 120px;
}
.dropzone.dragging {
  border-color: rgba(24, 160, 88, 0.6);
}
.empty {
  padding: 32px 0;
}
.entry-card {
  border: 1px solid rgba(128, 128, 128, 0.24);
  border-radius: 6px;
  padding: 8px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.entry-card.selected {
  border-color: rgba(24, 160, 88, 0.55);
}
.card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 22px;
}
.card-body {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding: 8px 4px;
  cursor: pointer;
  min-width: 0;
}
.card-name {
  font-size: 12px;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  word-break: break-all;
}
</style>
