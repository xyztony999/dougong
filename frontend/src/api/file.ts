/**
 * file-manager api 层 —— 22 Action 与 api-file-manager.md 4.1.4/4.2.4 一一对应（S7，无虚构接口）。
 * 读 6 GET（entry/list、trash/list 两角色；path-scope/get、download、content、system-accounts/list admin）
 * + 写 16 POST（admin + CSRF——http.ts 统一注入 X-CSRF-Token）。函数名 = PageSpec 第 2 节 Action 名。
 * 特殊通道：upload/chunk 走 multipart（postForm）、download 走流式（downloadFile，D-16）。
 */
import type { ArchiveFormat } from '@/types/enums'
import type {
  ChunkCompleteData,
  ChunkInitData,
  ChunkProgressData,
  ConflictStrategy,
  ContentSaveData,
  CreateEntryType,
  EntryArchiveData,
  EntryChmodData,
  EntryChownData,
  EntryContentData,
  EntryCreateData,
  EntryDeleteData,
  EntryListData,
  EntryMoveData,
  EntryRenameData,
  EntrySortField,
  EntrySortOrder,
  EntryUnarchiveData,
  PathScopeData,
  SystemAccountsData,
  TrashListData,
  TrashPurgeData,
  TrashRestoreData,
  UploadData
} from '@/types/file'
import { downloadFile, postForm, request } from './http'

/** ---- 路径边界（⚠️A12，admin 专属；D-02 修改即生效） ---- */

export function pathScopeGet(): Promise<PathScopeData> {
  return request<PathScopeData>('/files/path-scope/get')
}

export function pathScopeUpdate(rootPath: string, excludedDirs: string[]): Promise<PathScopeData> {
  return request<PathScopeData>('/files/path-scope/update', {
    method: 'POST',
    body: { rootPath, excludedDirs }
  })
}

/** ---- 目录浏览（两角色；D-03 全量返回 + 服务端排序） ---- */

export interface EntryListQuery {
  /**
   * 受管根内绝对路径；undefined = 受管根自身（初始浏览语义）。
   * root-default 为后端 A-F20 裁定（Gate 2 E1：projector.go 缺省 path=guard RootPath，
   * 宽容缺省不升版 api 契约）；readonly 无 path-scope/get 权、无从预知受管根，缺省浏览
   * 只能由服务端解析。注意：仅 entry/list 有此语义——写操作 dir/targetDir 必填具体绝对路径。
   */
  path?: string
  sort?: EntrySortField
  order?: EntrySortOrder
}

export function entryList(q: EntryListQuery = {}): Promise<EntryListData> {
  return request<EntryListData>('/files/entry/list', {
    query: { path: q.path, sort: q.sort, order: q.order }
  })
}

/** ---- 上传（D-04/D-10；admin+CSRF） ---- */

/** 直传（≤5MiB/文件，多文件；multipart：dir + files[] + onConflict 默认 ask，D-07） */
export function entryUpload(
  dir: string,
  files: File[],
  onConflict: ConflictStrategy = 'ask'
): Promise<UploadData> {
  const form = new FormData()
  form.append('dir', dir)
  for (const f of files) form.append('files', f, f.name)
  form.append('onConflict', onConflict)
  return postForm<UploadData>('/files/entry/upload', form)
}

/** 分块第 1 步：任务初始化（内存任务 D-10；同名预检 FILE_005——无 onConflict 参数，如实承接） */
export function chunkInit(
  dir: string,
  fileName: string,
  totalSize: number,
  totalChunks: number
): Promise<ChunkInitData> {
  return request<ChunkInitData>('/files/upload/chunk-init', {
    method: 'POST',
    body: { dir, fileName, totalSize, totalChunks }
  })
}

/** 分块第 2 步：逐块上传（multipart：uploadId + index + chunk；顺序上传，D-04） */
export function chunkUpload(uploadId: string, index: number, chunk: Blob): Promise<ChunkProgressData> {
  const form = new FormData()
  form.append('uploadId', uploadId)
  form.append('index', String(index))
  form.append('chunk', chunk)
  return postForm<ChunkProgressData>('/files/upload/chunk', form)
}

/** 分块第 3 步：合并落盘（校验累计大小后按 index 序合并 + rename 原子落盘） */
export function chunkComplete(uploadId: string): Promise<ChunkCompleteData> {
  return request<ChunkCompleteData>('/files/upload/chunk-complete', {
    method: 'POST',
    body: { uploadId }
  })
}

/** ---- 下载（admin；GET 只读无 CSRF；审计 security D-08；流式 D-16） ---- */

/** 下载并触发浏览器保存（PageSpec 4.3：window 触发流下载） */
export async function entryDownload(path: string): Promise<void> {
  const { blob, filename } = await downloadFile('/files/entry/download', { path })
  const fallback = path.split('/').filter(Boolean).pop() ?? 'download'
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename ?? fallback
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

/** ---- 在线编辑（D-05：1MiB + 二进制探测；全量覆盖保存 I-09） ---- */

export function entryContent(path: string): Promise<EntryContentData> {
  return request<EntryContentData>('/files/entry/content', { query: { path } })
}

export function entryContentSave(path: string, content: string): Promise<ContentSaveData> {
  return request<ContentSaveData>('/files/entry/content-save', {
    method: 'POST',
    body: { path, content }
  })
}

/** ---- 整理动作（admin+CSRF） ---- */

export function entryCreate(
  dir: string,
  name: string,
  type: CreateEntryType
): Promise<EntryCreateData> {
  return request<EntryCreateData>('/files/entry/create', {
    method: 'POST',
    body: { dir, name, type }
  })
}

export function entryRename(path: string, newName: string): Promise<EntryRenameData> {
  return request<EntryRenameData>('/files/entry/rename', {
    method: 'POST',
    body: { path, newName }
  })
}

export function entryMove(
  sourcePath: string,
  targetDir: string,
  onConflict: ConflictStrategy = 'ask'
): Promise<EntryMoveData> {
  return request<EntryMoveData>('/files/entry/move', {
    method: 'POST',
    body: { sourcePath, targetDir, onConflict }
  })
}

export function entryDelete(path: string): Promise<EntryDeleteData> {
  return request<EntryDeleteData>('/files/entry/delete', {
    method: 'POST',
    body: { path }
  })
}

/** ---- 元数据（D-06 不递归） ---- */

/** chmod：mode 为八进制字符串（如 `0755`，0~0777，FILE_015） */
export function entryChmod(path: string, mode: string): Promise<EntryChmodData> {
  return request<EntryChmodData>('/files/entry/chmod', {
    method: 'POST',
    body: { path, mode }
  })
}

/** chown：ownerUser/ownerGroup 至少一项（候选经 systemAccountsList，D-14） */
export function entryChown(
  path: string,
  ownerUser?: string,
  ownerGroup?: string
): Promise<EntryChownData> {
  return request<EntryChownData>('/files/entry/chown', {
    method: 'POST',
    body: { path, ownerUser, ownerGroup }
  })
}

/** 系统用户/组候选清单（实时读取无缓存 D-14；随属主 Dialog 每次打开加载一次） */
export function systemAccountsList(): Promise<SystemAccountsData> {
  return request<SystemAccountsData>('/files/system-accounts/list')
}

/** ---- 压缩/解压 ---- */

/** 压缩：多选打包落当前浏览目录（产物名缺省 archive.{ext}；同名 FILE_004 直接错误 D-13） */
export function entryArchive(
  paths: string[],
  format: ArchiveFormat,
  targetName?: string
): Promise<EntryArchiveData> {
  return request<EntryArchiveData>('/files/entry/archive', {
    method: 'POST',
    body: { paths, format, targetName }
  })
}

/** 解压：逐条目 PathScopeGuard 预检（FILE_009 zip slip 整体不落盘）+ 同名冲突决策 D-07 */
export function entryUnarchive(
  archivePath: string,
  targetDir: string,
  onConflict: ConflictStrategy = 'ask'
): Promise<EntryUnarchiveData> {
  return request<EntryUnarchiveData>('/files/entry/unarchive', {
    method: 'POST',
    body: { archivePath, targetDir, onConflict }
  })
}

/** ---- 回收站（list 两角色；restore/purge admin+CSRF；R-17/ADR-021） ---- */

export function trashList(): Promise<TrashListData> {
  return request<TrashListData>('/files/trash/list')
}

/** 还原至原路径（onConflict 仅 ask|overwrite——「放弃」=客户端不重发，D-07） */
export function trashRestore(
  trashId: number,
  onConflict: Exclude<ConflictStrategy, 'skip'> = 'ask'
): Promise<TrashRestoreData> {
  return request<TrashRestoreData>('/files/trash/restore', {
    method: 'POST',
    body: { trashId, onConflict }
  })
}

/** 清空全部在站条目（终局销毁，I-02；空站幂等成功 purgedCount=0，N3） */
export function trashPurge(): Promise<TrashPurgeData> {
  return request<TrashPurgeData>('/files/trash/purge', {
    method: 'POST'
  })
}
