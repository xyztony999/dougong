/**
 * file-manager API 类型 —— 与 api-file-manager.md v1.0 4.1.4/4.2.4 对齐（唯一事实源）。
 * - file_entry 为文件系统实时投影（零持久化 I-07）：list 响应恒为当次读取快照（I-06 含隐藏条目）；
 * - 无分页（D-03 全量返回 + 服务端排序）；trash/list 无 generatedAt（api 4.1.4——页面不渲染投影时刻）；
 * - 冲突决策 onConflict（D-07）：ask=预检发现冲突返回 FILE_005 data.conflicts[] 零落盘；
 * - 回收站 entryType 为内联两值域 file/directory（api 4.1.1 CHECK，与 file_type 三值不同——symlink 删除归 file）。
 */
import type { ArchiveFormat, FileEntryType } from './enums'

/** ---- 投影条目（entry/list items，api 4.2.1 字段清单） ---- */
export interface FileEntry {
  /** 条目名（不含路径） */
  name: string
  /** 受管根内绝对路径（单点寻址标识，D-01） */
  path: string
  entryType: FileEntryType
  /** 字节数（目录恒 0，api 4.2.1） */
  size: number
  /** 权限位八进制原值（如 `0644`；rwx 分组呈现归后续迭代，PageSpec 4.2 从简） */
  mode: string
  ownerUser: string
  ownerGroup: string
  /** 修改时间（ISO-8601 UTC） */
  mtime: string
}

/** GET /files/entry/list 响应 data（符号链接不展开目标、按链接自身呈现，api 4.2.4） */
export interface EntryListData {
  path: string
  items: FileEntry[]
  total: number
  generatedAt: string
}

/** ---- 路径边界（path-scope，⚠️A12） ---- */
export interface PathScopeData {
  rootPath: string
  excludedDirs: string[]
}

/** ---- 上传（D-04/D-10：直传 ≤5MiB / 分块三段式） ---- */
export interface UploadResultItem {
  name: string
  path: string
  status: 'ok' | 'skipped'
}

/** POST /files/entry/upload 响应 data */
export interface UploadData {
  results: UploadResultItem[]
  uploadedCount: number
}

/** POST /files/upload/chunk-init 响应 data */
export interface ChunkInitData {
  uploadId: string
}

/** POST /files/upload/chunk 响应 data（过程块进度） */
export interface ChunkProgressData {
  uploadId: string
  received: number
  total: number
}

/** POST /files/upload/chunk-complete 响应 data（合并落盘终值） */
export interface ChunkCompleteData {
  path: string
  size: number
}

/** ---- 在线编辑（D-05） ---- */
export interface EntryContentData {
  path: string
  size: number
  mtime: string
  /** UTF-8 文本全量（≤1MiB，服务端探测通过才返回） */
  content: string
}

/** POST /files/entry/content-save 响应 data（新值供前端刷新） */
export interface ContentSaveData {
  path: string
  size: number
  mtime: string
}

/** ---- 整理动作 ---- */
export interface EntryCreateData {
  path: string
  entryType: FileEntryType
}

export interface EntryRenameData {
  /** 新路径 */
  path: string
}

export interface MoveResultItem {
  from: string
  to: string
}

export interface EntryMoveData {
  moved: MoveResultItem[]
  skippedCount: number
}

export interface EntryDeleteData {
  trashId: number
  trashName: string
}

/** ---- 元数据（D-06 不递归；响应含刷新值供行直更） ---- */
export interface EntryChmodData {
  path: string
  mode: string
}

export interface EntryChownData {
  path: string
  ownerUser: string
  ownerGroup: string
}

export interface SystemAccountItem {
  name: string
  uid: number
}

export interface SystemAccountGroup {
  name: string
  gid: number
}

/** GET /files/system-accounts/list 响应 data（D-14 实时读取；仅 name/uid、name/gid） */
export interface SystemAccountsData {
  users: SystemAccountItem[]
  groups: SystemAccountGroup[]
}

/** ---- 压缩/解压 ---- */
export interface EntryArchiveData {
  /** 产物路径（落当前浏览目录） */
  path: string
  size: number
  format: ArchiveFormat
}

export interface EntryUnarchiveData {
  extractedCount: number
  targetDir: string
}

/** ---- 回收站（trash_entry，api 4.1.1/4.1.4） ---- */
export interface TrashItem {
  /** trashId 寻址键（不渲染为可见列，PageSpec 4A.2） */
  id: number
  /** 原路径（还原目标） */
  originalPath: string
  /** 内联两值域（symlink 删除归 file） */
  entryType: TrashEntryType
  /** 删除入站时快照（目录为累计值，与 4.2 实时投影「目录恒 0」不同） */
  size: number
  /** 操作者快照（D-12） */
  operatorName: string
  deletedAt: string
}

export interface TrashListData {
  items: TrashItem[]
  total: number
  /** 孤儿对账剔除事实标注（后端 A-F11 增补，omitempty——非空时页面 NAlert 呈现，PRD N2） */
  warning?: string
}

export interface TrashRestoreData {
  id: number
  originalPath: string
  status: 'restored'
}

export interface TrashPurgeData {
  purgedCount: number
}

/** ---- FILE_005 冲突载荷（D-07：ask 预检零落盘，携带决策重发） ----
 * api 逐场景形状：upload=[{name}]（4.2.4）、restore=[{path}]（4.1.4）、move/unarchive 为
 * data.conflicts[] 未逐字段声明——统一为可选双字段 + 展示器取 path ?? name（诚实承接，不虚构必填键）。 */
export interface ConflictEntry {
  name?: string
  path?: string
}

/** 冲突项展示文案（名称或路径，二者皆缺降级占位） */
export function conflictLabel(c: ConflictEntry): string {
  return c.path ?? c.name ?? '未知条目'
}

/** ApiError.data（unknown）→ ConflictEntry[] 收窄 */
export function isConflictList(d: unknown): d is ConflictEntry[] {
  return (
    Array.isArray(d) &&
    d.every(
      (it) =>
        typeof it === 'object' &&
        it !== null &&
        (typeof (it as Record<string, unknown>).name === 'string' ||
          typeof (it as Record<string, unknown>).path === 'string')
    )
  )
}

/** ApiError.data → FILE_002 data.reason 收窄（PageSpec 第 1 节：越界原因两类之一） */
export function scopeReason(d: unknown): string | null {
  if (typeof d !== 'object' || d === null) return null
  const reason = (d as Record<string, unknown>).reason
  return typeof reason === 'string' ? reason : null
}

/** ApiError.data → FILE_013 data.reason 收窄（磁盘满/权限等如实呈现） */
export function fsReason(d: unknown): string | null {
  return scopeReason(d)
}

/** ---- 局部常量（PageSpec 第 3 节：非枚举池条目，前端 TS 常量） ---- */

/** 排序域（api 3.4 SORT_FIELDS/ORDERS：接口参数非业务枚举；默认 name/asc，D-03） */
export type EntrySortField = 'name' | 'size' | 'mtime'
export type EntrySortOrder = 'asc' | 'desc'

export const ENTRY_SORT_FIELD: Readonly<Record<EntrySortField, string>> = {
  name: '名称',
  size: '大小',
  mtime: '修改时间'
}

export const ENTRY_SORT_ORDER: Readonly<Record<EntrySortOrder, string>> = {
  asc: '升序',
  desc: '降序'
}

/** 冲突策略（api 3.4 ON_CONFLICT：ask 默认；还原仅 ask|overwrite，D-07） */
export type ConflictStrategy = 'ask' | 'overwrite' | 'skip'

export const CONFLICT_STRATEGY: Readonly<Record<ConflictStrategy, string>> = {
  ask: '逐项询问',
  overwrite: '覆盖',
  skip: '跳过'
}

/** 回收站条目类型（api 4.1.1 内联 CHECK 两值，与 file_type 值域不同） */
export type TrashEntryType = 'file' | 'directory'

export const TRASH_ENTRY_TYPE: Readonly<Record<TrashEntryType, string>> = {
  file: '文件',
  directory: '目录'
}

/** 新建类型（api 4.2.4 create req：type=file|directory 两值，无 symlink） */
export type CreateEntryType = 'file' | 'directory'

/** 上传阈值（api 3.4 业务常量 D-04：单请求直传上限=分块大小=5MiB，前端分支判定） */
export const DIRECT_UPLOAD_LIMIT = 5 * 1024 * 1024
export const UPLOAD_CHUNK_SIZE = 5 * 1024 * 1024

/**
 * FILE_001~015 错误文案（PageSpec 第 1 节错误处理，逐条对应 api 3.4）：
 * FILE_005（冲突决策 Modal）与 FILE_006（编辑页拒绝进入）为行为分支不经本表 toast；
 * FILE_002 页面横幅 / FILE_003 自动刷新 / FILE_004 warning 等在页面处理路径引用本表。
 */
export const FILE_ERROR_TEXT: Readonly<Record<string, string>> = {
  FILE_001: '参数非法',
  FILE_002: '该路径不受管理（越界或命中排除目录）',
  FILE_003: '条目不存在（可能已被删除）',
  FILE_004: '目标已存在',
  FILE_005: '目标位置存在同名条目',
  FILE_006: '该文件为二进制或超过 1MiB，请下载后本地编辑',
  FILE_007: '参数非法',
  FILE_008: '参数非法',
  FILE_009: '压缩包包含越界条目（安全拦截），已整体取消解压',
  FILE_010: '参数非法',
  FILE_011: '超过单请求 5MiB 上限',
  FILE_012: '目标目录不存在',
  FILE_013: '文件系统错误',
  FILE_014: '回收站条目不存在或已处理',
  FILE_015: '参数非法'
}
