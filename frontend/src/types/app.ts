/**
 * app-mgmt 类型层 —— DTO 与 api-app-mgmt.md §4.3 逐形状对齐（可空显式
 * |null：JSON null≠undefined；实况探测失败字段 null 如实不虚构，D-01）。
 */
import type { AppStartMode, AppTaskAction, AppTaskStatus } from './enums'

/** Python 就绪度（GET /app/app/list 页头；venvSupport/pipAvailable 仅已装时非 null，D-08） */
export interface PythonReadiness {
  installed: boolean
  version: string | null
  venvSupport: boolean | null
  pipAvailable: boolean | null
}

/** 应用列表条目（档案+实况投影合并；null=探测不可用如实，D-01/I-06） */
export interface AppItem {
  id: number
  name: string
  dir: string
  entry: string
  startMode: AppStartMode
  listenPort: number
  serviceRunning: boolean | null
  serviceEnabled: boolean | null
  venvExists: boolean
  depsInstalled: boolean | null
  portListening: boolean | null
  runningTaskId: number | null
  hasSite: boolean | null
  unitFailed: boolean
  createdAt: string
  createdBy: string
}

/** GET /app/app/list 响应 */
export interface AppListData {
  pythonReadiness: PythonReadiness
  items: AppItem[]
  generatedAt: string
}

/** POST /app/app/create 请求（startSpec 按模式：preset=module+app；custom=command） */
export interface AppCreateRequest {
  name: string
  dir: string
  entry: string
  startMode: AppStartMode
  startSpec: { module?: string; app?: string; args?: string; command?: string }
  listenPort: number
  startNow?: boolean
}

/** 任务发起响应（create/delete/deps/retry） */
export interface AppTaskCreated {
  taskId: number
}

/** POST /app/app/delete 请求（purgeDir 默认 false——绝不静默删代码 ⚠️A5） */
export interface AppDeleteRequest {
  appId: number
  purgeDir?: boolean
}

/** .env 现文件内容（updatedAt=基线 mtime，D-09 乐观锁载体） */
export interface AppEnvData {
  appId: number
  lines: string[]
  updatedAt: string | null
}

/** POST /app/app/params 请求（startSpec 可选=仅变更段提交） */
export interface AppParamsSaveRequest {
  appId: number
  envLines?: string[]
  startSpec?: { module?: string; app?: string; args?: string; command?: string }
  baseMtime?: string
}

/** 参数保存响应（needRestart 恒 true——提示归前端，不自动重启 ⚠️A10） */
export interface AppParamsSaveData {
  appId: number
  needRestart: boolean
}

/** 服务操作响应 */
export interface AppServiceOpData {
  appId: number
  unit: string
}

/** 任务条目（retryable=D-13 重试按钮态数据源：failed 且 create/deps_install） */
export interface AppTaskItem {
  id: number
  appId: number
  appName: string
  action: AppTaskAction
  status: AppTaskStatus
  purgeDir: boolean
  requestedBy: string
  createdAt: string
  startedAt?: string
  finishedAt?: string
  error?: string
  retryable: boolean
}

/** GET /app/task/list 响应 */
export interface AppTaskListData {
  items: AppTaskItem[]
  total: number
}

/** GET /app/task/detail 响应 */
export interface AppTaskDetailData extends AppTaskItem {
  outputTail: string
}

/** GET /app/log 响应（admin 专属 ⚠️A8） */
export interface AppLogData {
  appId: number
  unit: string
  lines: number
  content: string
}

/** POST /app/site/link 响应（站点引用值快照，不落库 D-12） */
export interface AppSiteLinkData {
  appId: number
  note: string
}

/** APP 错误码文案表（api §3.4 APP_001~012；UI 文案人话化——不含内部术语） */
export const APP_ERROR_TEXT: Record<string, string> = {
  APP_001: '应用名非法：请用小写字母、数字、连字符（不超过 32 个字符）',
  APP_002: '应用名已存在，请换一个名称',
  APP_003: '代码目录或入口文件不存在，请先上传代码（可用文件管理器）',
  APP_004: 'Python 环境未就绪，请到环境管理安装 Python',
  APP_005: '端口已被占用，请换一个端口',
  APP_006: '该应用有任务进行中，请等待完成后再操作',
  APP_007: '应用不存在，请刷新列表',
  APP_008: '任务不存在，请刷新任务列表',
  APP_009: '运行环境（venv）缺失，请删除应用后重新创建',
  APP_010: '服务操作失败',
  APP_011: '建站失败，请到站点管理检查或手工创建',
  APP_012: '参数校验失败'
}
