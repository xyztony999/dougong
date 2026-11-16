/**
 * fileStore —— PageSpec 第 1 节声明的跨页面共享状态：
 * currentPath/面包屑/viewMode（列表图标）/sortParams/entries/generatedAt/inFlightSet/
 * selection（多选承载压缩与批量删除）/目录选择器态（picker*，4.4.3/4.4.6 内嵌 mini 浏览器）。
 *
 * 刷新纪律（api D-03 + PageSpec 第 2 节）：手动 + 操作驱动，无自动轮询（本 store 不持定时器）；
 * 改变目录内容的操作成功 → loadList 重拉（响应无投影快照）；仅改元数据（chmod/chown）→
 * applyMetaPatch 以响应刷新值直更对应行不整表重拉；页面卸载 resetView() 清理选择/inFlight/picker。
 * FILE_002 → scopeError 页面横幅（不显示过期数据）；edit 页与 trash 页独立 store。
 */
import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { ApiError } from '@/api/http'
import { entryList } from '@/api/file'
import type { FileEntry } from '@/types/file'
import type { EntrySortField, EntrySortOrder } from '@/types/file'

/** 目录选择器用途（4.4.3 移动 / 4.4.6 解压目标目录） */
export type PickerMode = 'move' | 'unarchive'

export interface FileListParams {
  sort: EntrySortField
  order: EntrySortOrder
}

export const useFileStore = defineStore('file', () => {
  /** ---- 列表（'' = 受管根，初始浏览省略 path 参数——A-F20 root-default 契约，见 tasks.md 实现决策 1） ---- */
  const currentPath = ref('')
  /**
   * 受管根绝对路径（Gate 2 E1 修复）：由 root-default 首次响应 data.path 捕获——
   * 后端仅 entry/list 有缺省=受管根语义（projector.go A-F20），写操作（upload/create/move/
   * unarchive 的 dir/targetDir）经 validatePathForm 空串即 FILE_001，故受管根下的一切写操作
   * 必须发具体绝对路径（rootPath ?? '/' 兜底）。pathScope 更改后置 null 由下次 root-default 重解。
   */
  const rootPath = ref<string | null>(null)
  /** 最近一次响应解析出的实际目录（面包屑依据；与请求 path 可能因服务端解析不同） */
  const resolvedPath = ref<string | null>(null)
  const entries = ref<FileEntry[]>([])
  const total = ref(0)
  const generatedAt = ref<string | null>(null)
  const listLoading = ref(false)
  /** FILE_002 页面级横幅（reason 为服务端越界原因两类之一；true 时表格区不渲染） */
  const scopeError = ref<string | null>(null)
  /** FILE_012 兜底文案（store 无 Message 上下文，页面 watch 提示——service-mgmt 先例） */
  const listBizError = ref<string | null>(null)
  const sortParams = ref<FileListParams>({ sort: 'name', order: 'asc' })
  /** 视图呈现态（P3：list=NDataTable / grid=NGrid 卡片，不调接口） */
  const viewMode = ref<'list' | 'grid'>('list')

  /** ---- 行级在途互斥（PageSpec 4.3：该行全部操作按钮置 loading/禁用；无假进度） ---- */
  const inFlightSet = ref<ReadonlySet<string>>(new Set<string>())

  /** ---- 多选（admin；承载压缩 4.4.9 与批量删除 4.3；readonly 不渲染选择列） ---- */
  const selection = ref<string[]>([])
  const selectedEntries = computed<FileEntry[]>(() => {
    const picked = new Set(selection.value)
    return entries.value.filter((e) => picked.has(e.path))
  })

  /** ---- 目录选择器（PageSpec §1 声明：4.4.3/4.4.6 共用 mini 浏览器，起始=当前浏览/归档所在目录） ---- */
  const pickerMode = ref<PickerMode | null>(null)
  const pickerPath = ref('')
  const pickerDirs = ref<FileEntry[]>([])
  const pickerLoading = ref(false)
  const pickerScopeError = ref<string | null>(null)

  async function loadList(): Promise<void> {
    if (listLoading.value) return // 刷新按钮 loading 防抖（service-mgmt 先例）
    listLoading.value = true
    scopeError.value = null
    listBizError.value = null
    const requestRoot = currentPath.value === ''
    try {
      const data = await entryList({
        path: requestRoot ? undefined : currentPath.value,
        sort: sortParams.value.sort,
        order: sortParams.value.order
      })
      resolvedPath.value = data.path
      entries.value = data.items
      total.value = data.total
      generatedAt.value = data.generatedAt
      if (requestRoot) {
        // root-default 响应 data.path=受管根规范路径：捕获 rootPath（写操作寻址用）+ currentPath 规范化
        rootPath.value = data.path
        currentPath.value = data.path
      }
      // 条目可能已被面板外改动：清掉指向已消失条目的陈旧选择
      const alive = new Set(data.items.map((e) => e.path))
      selection.value = selection.value.filter((p) => alive.has(p))
    } catch (e) {
      if (e instanceof ApiError && e.is('FILE_002')) {
        scopeError.value = '该路径不受管理（越界或命中排除目录）'
        entries.value = []
        total.value = 0
        generatedAt.value = null
      } else if (e instanceof ApiError && e.is('FILE_012')) {
        // PageSpec 第 1 节：FILE_012 warning + 刷新——目标目录已不存在，回受管根重拉（防死循环：仅非根路径时）；
        // 递归重拉入口会复位 listBizError，故重拉完成后回填（页面 watch 呈现 warning 且已回根）
        const msg = '目标目录不存在'
        if (currentPath.value !== '') {
          currentPath.value = ''
          listLoading.value = false
          await loadList()
          listBizError.value = msg
          return
        }
        listBizError.value = msg
      }
      // 其余（网络/AUTH 等）已由 http 拦截器统一提示
    } finally {
      listLoading.value = false
    }
  }

  /** 面包屑/上级导航/进入目录/返回受管根：切换目录重拉（刷新纪律第 1 行；重入守卫在 loadList） */
  async function navigate(path: string): Promise<void> {
    currentPath.value = path
    selection.value = []
    await loadList()
  }

  /** 排序变更即重拉（服务端排序 D-03） */
  function setSort(sort: EntrySortField, order: EntrySortOrder): void {
    sortParams.value = { sort, order }
    void loadList()
  }

  function setViewMode(mode: 'list' | 'grid'): void {
    viewMode.value = mode
  }

  /** ---- 在途互斥 ---- */

  function setInFlight(path: string): void {
    const next = new Set(inFlightSet.value)
    next.add(path)
    inFlightSet.value = next
  }

  function releaseInFlight(path: string): void {
    const next = new Set(inFlightSet.value)
    next.delete(path)
    inFlightSet.value = next
  }

  function isInFlight(path: string): boolean {
    return inFlightSet.value.has(path)
  }

  /** ---- 元数据直更（刷新纪律：chmod/chown 响应含刷新值，不整表重拉） ---- */

  function applyMetaPatch(path: string, patch: Partial<Pick<FileEntry, 'mode' | 'ownerUser' | 'ownerGroup'>>): void {
    const row = entries.value.find((e) => e.path === path)
    if (row === undefined) return
    if (patch.mode !== undefined) row.mode = patch.mode
    if (patch.ownerUser !== undefined) row.ownerUser = patch.ownerUser
    if (patch.ownerGroup !== undefined) row.ownerGroup = patch.ownerGroup
  }

  /** ---- 多选 ---- */

  function setSelection(paths: string[]): void {
    selection.value = [...paths]
  }

  function toggleSelection(path: string, checked: boolean): void {
    if (checked) selection.value = [...selection.value, path]
    else selection.value = selection.value.filter((p) => p !== path)
  }

  function clearSelection(): void {
    selection.value = []
  }

  /** ---- 目录选择器（4.4.3/4.4.6 共用；readonly 无入口不触发） ---- */

  async function loadPicker(): Promise<void> {
    if (pickerLoading.value) return
    pickerLoading.value = true
    pickerScopeError.value = null
    const requestRoot = pickerPath.value === ''
    try {
      const data = await entryList({
        path: requestRoot ? undefined : pickerPath.value,
        sort: 'name',
        order: 'asc'
      })
      pickerDirs.value = data.items.filter((e) => e.entryType === 'directory')
      // 规范化 pickerPath 为具体绝对路径（move/unarchive 的 targetDir 必填具体值——空串 FILE_001，E1 修复）
      if (requestRoot) {
        rootPath.value = data.path
        pickerPath.value = data.path
      } else {
        pickerPath.value = data.path
      }
    } catch (e) {
      if (e instanceof ApiError && e.is('FILE_002')) {
        pickerScopeError.value = '该路径不受管理（越界或命中排除目录）'
        pickerDirs.value = []
      }
      // 其余由拦截器提示
    } finally {
      pickerLoading.value = false
    }
  }

  function openPicker(mode: PickerMode, startPath: string): void {
    pickerMode.value = mode
    pickerPath.value = startPath
    pickerDirs.value = []
    void loadPicker()
  }

  function enterPickerDir(path: string): void {
    pickerPath.value = path
    void loadPicker()
  }

  function closePicker(): void {
    pickerMode.value = null
    pickerPath.value = ''
    pickerDirs.value = []
    pickerScopeError.value = null
    pickerLoading.value = false
  }

  /** 页面卸载：清理选择/inFlight/picker 态（PageSpec 第 2 节刷新纪律末行；rootPath 保留供再进入写操作寻址） */
  function resetView(): void {
    closePicker()
    inFlightSet.value = new Set<string>()
    selection.value = []
    listBizError.value = null
  }

  /**
   * 写操作寻址目录（Gate 2 E1 修复）：受管根下返回 rootPath 具体绝对路径——
   * 后端仅 entry/list 有缺省=受管根（A-F20），upload/create/move/unarchive 的
   * dir/targetDir 空串即 FILE_001；列表加载前 rootPath 未知时 '/' 兜底（越界由服务端 FILE_002 拦截）。
   */
  function writeDir(): string {
    if (currentPath.value !== '') return currentPath.value
    return rootPath.value ?? '/'
  }

  /** pathScope 更改后失效 rootPath（受管根可能已变），由下次 root-default 请求重解 */
  function invalidateRoot(): void {
    rootPath.value = null
  }

  return {
    currentPath,
    rootPath,
    resolvedPath,
    entries,
    total,
    generatedAt,
    listLoading,
    scopeError,
    listBizError,
    sortParams,
    viewMode,
    inFlightSet,
    selection,
    selectedEntries,
    pickerMode,
    pickerPath,
    pickerDirs,
    pickerLoading,
    pickerScopeError,
    loadList,
    navigate,
    setSort,
    setViewMode,
    setInFlight,
    releaseInFlight,
    isInFlight,
    applyMetaPatch,
    setSelection,
    toggleSelection,
    clearSelection,
    openPicker,
    enterPickerDir,
    closePicker,
    loadPicker,
    writeDir,
    invalidateRoot,
    resetView
  }
})
