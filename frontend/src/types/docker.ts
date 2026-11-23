/**
 * docker API 类型 —— 与 api-docker-mgmt.md v1.0 第 4 章对齐（唯一事实源）。
 * 零持久化投影（D-10）：五类对象均为 daemon/label 实况快照（I-01 无缓存），
 * daemon unavailable 访问业务面返回 DK_002（不返回空清单——未装≠无容器，I-01）。
 * - containerId/imageId/volumeName/networkId/project 均为不透明引用（D-11），
 *   来自各 list 响应原样透传回操作端点，前端不解析内容；
 * - supportedActions 为服务端权威动作面（D-02/D-04：按当前态 × BA 附录B.2 矩阵计算，
 *   过渡态为 []），前端操作入口照此渲染、不自行推断；
 * - locatable 为 Compose 项目服务端定位判定（D-06：false → 操作/日志端点 DK_015）。
 * 无分页（D-02 全量返回）；generatedAt=投影生成时刻（ISO-8601 UTC）。
 */
import type {
  ComposeAction,
  ContainerAction,
  ContainerStatus,
  DockerDaemonState,
  DockerLogEndReason
} from './enums'

/** GET /docker/daemon/status 响应 data（D-01 即时探测：available 时 version 伴随、无 detail） */
export interface DockerDaemonStatusData {
  state: DockerDaemonState
  version: string | null
  /** unavailable 时的探测结论细节透传（not_installed / socket_unreachable——透传属性非枚举，available 为 null） */
  detail: string | null
  generatedAt: string
}

/** 容器投影条目（api 4.1.1 字段清单） */
export interface DockerContainerItem {
  /** daemon 64-hex ID（D-11 不透明引用，操作/日志/快照寻址） */
  containerId: string
  name: string
  /** 镜像引用（名称:tag） */
  image: string
  /** 核心三态（映射归并 BA 附录B.1）；过渡态时透传原始值（restarting/removing） */
  state: ContainerStatus | string
  /** 所属 Compose 项目（label 透传；无项目为 null） */
  composeProject: string | null
  /** 服务端权威动作面（六动作子集；过渡态为 []，D-02/D-04） */
  supportedActions: ContainerAction[]
}

/** container/list 随行携带的 daemon 摘要（宿主 S-45-01 页头呈现） */
export interface DockerDaemonSummary {
  state: DockerDaemonState
  version: string | null
}

/** GET /docker/container/list 响应 data（全量不分页 D-02；服务端筛选+名称字典序） */
export interface DockerContainerListData {
  items: DockerContainerItem[]
  total: number
  daemon: DockerDaemonSummary
  generatedAt: string
}

/** GET /docker/{container,compose}/logs 响应 data（D-03 尾部读取：JSON 行数组） */
export interface DockerLogsData {
  lines: string[]
  generatedAt: string
}

/** GET /docker/container/stats 响应 data（D-07 单次快照，过程数据） */
export interface DockerStatsData {
  cpuPercent: number
  memUsageBytes: number
  memLimitBytes: number
  memPercent: number
  generatedAt: string
}

/** POST /docker/container/{start|stop|restart|pause|unpause} 请求 */
export interface DockerContainerActionReq {
  containerId: string
}

/** 上述五动作成功 data（操作后即读实况） */
export interface DockerContainerActionResult {
  containerId: string
  currentState: string
}

/** POST /docker/container/delete 请求（无 force 字段，I-04/I-05） */
export interface DockerContainerDeleteReq {
  containerId: string
}

/** POST /docker/container/delete 成功 data */
export interface DockerContainerDeleteResult {
  deleted: boolean
  containerId: string
}

/** 镜像投影条目（api 4.2.1；引用情况不做前置推导标注，I-05） */
export interface DockerImageItem {
  imageId: string
  /** 名称:tag */
  ref: string
  sizeBytes: number
}

/** GET /docker/image/list 响应 data */
export interface DockerImageListData {
  items: DockerImageItem[]
  total: number
  generatedAt: string
}

/** POST /docker/image/pull 请求（ref 格式 `名称[:tag]`，DK_012） */
export interface DockerImagePullReq {
  ref: string
}

/** POST /docker/image/pull 成功 data（同步终态，D-05） */
export interface DockerImagePullResult {
  imageId: string
  ref: string
}

/** POST /docker/image/delete 请求 */
export interface DockerImageDeleteReq {
  imageId: string
}

/** POST /docker/image/delete 成功 data */
export interface DockerImageDeleteResult {
  deleted: boolean
  imageId: string
}

/** 卷投影条目（volumeName 原生即稳定标识，D-11） */
export interface DockerVolumeItem {
  volumeName: string
  mountpoint: string
  sizeBytes: number
}

/** GET /docker/volume/list 响应 data */
export interface DockerVolumeListData {
  items: DockerVolumeItem[]
  total: number
  generatedAt: string
}

/** POST /docker/volume/delete 请求 */
export interface DockerVolumeDeleteReq {
  volumeName: string
}

/** POST /docker/volume/delete 成功 data */
export interface DockerVolumeDeleteResult {
  deleted: boolean
  volumeName: string
}

/** 网络投影条目（预定义网络如实呈现，删除时 DK_011，I-05） */
export interface DockerNetworkItem {
  networkId: string
  name: string
  /** 原值透传（bridge/host/overlay 等开放集合不枚举化） */
  driver: string
  /** 无 IPAM 子网时后端发空串（api §4.2.1 钉死 string 口径），null 为超集容忍；判空（空串/null）渲染「—」不虚构 */
  subnet: string | null
}

/** GET /docker/network/list 响应 data */
export interface DockerNetworkListData {
  items: DockerNetworkItem[]
  total: number
  generatedAt: string
}

/** POST /docker/network/delete 请求 */
export interface DockerNetworkDeleteReq {
  networkId: string
}

/** POST /docker/network/delete 成功 data */
export interface DockerNetworkDeleteResult {
  deleted: boolean
  networkId: string
}

/** Compose 项目容器同域引用（api 4.3.1 containers[]，对 container 投影的引用集合） */
export interface DockerComposeContainerRef {
  containerId: string
  name: string
  state: string
}

/** Compose 项目投影条目（label 聚合派生，读取时聚合刷新，D-10） */
export interface DockerComposeItem {
  /** label `com.docker.compose.project` 聚合键（D-11 不透明引用） */
  project: string
  /** label `.working_dir` 透传（R-13） */
  workingDir: string | null
  /** label `.config_files` 透传 */
  configFile: string | null
  containers: DockerComposeContainerRef[]
  /** 服务端定位判定（工作目录存在且配置文件可读；false → 操作/日志端点 DK_015，D-06） */
  locatable: boolean
}

/** GET /docker/compose/list 响应 data */
export interface DockerComposeListData {
  items: DockerComposeItem[]
  total: number
  generatedAt: string
}

/** POST /docker/compose/{up|down|restart} 请求 */
export interface DockerComposeActionReq {
  project: string
}

/** POST /docker/compose/{up|down|restart} 成功 data */
export interface DockerComposeActionResult {
  project: string
  verb: string
  done: boolean
}

/** DK_004 data（api 3.4/4.1.4：{currentState, action, allowedActions}——allowedActions 与 list 同源计算） */
export interface DockerActionMismatchInfo {
  currentState: string
  action: string
  allowedActions: string[]
}

/** DK_015 data（api 3.4：{workingDir, configFile, missing}——label 记录路径呈现，D1 口径-2/D-06） */
export interface DockerComposeLocateFailedInfo {
  workingDir: string | null
  configFile: string | null
  missing: string
}

/** ---- WS 帧协议（D-03：服务端帧两类，end 后服务端关闭；客户端无上行帧） ---- */

/** 服务端帧：log 行 / end 终止（reason ∈ 常量表五值，api 3.4 业务常量非枚举池条目） */
export type DockerLogFrame =
  | { type: 'log'; line: string }
  | { type: 'end'; reason: DockerLogEndReason }

/** 帧文本 → 收窄类型（残缺/未知类型/未知 reason → null=协议违规，由调用方本地收口） */
export function parseDockerLogFrame(text: string): DockerLogFrame | null {
  let raw: unknown
  try {
    raw = JSON.parse(text)
  } catch {
    return null
  }
  if (typeof raw !== 'object' || raw === null) return null
  const o = raw as Record<string, unknown>
  if (o.type === 'log') {
    if (typeof o.line !== 'string') return null
    return { type: 'log', line: o.line }
  }
  if (o.type === 'end') {
    if (
      o.reason === 'container_removed' ||
      o.reason === 'container_stopped' ||
      o.reason === 'project_gone' ||
      o.reason === 'locate_failed' ||
      o.reason === 'daemon_lost'
    ) {
      return { type: 'end', reason: o.reason }
    }
  }
  return null
}

/** DK_004 data 收窄（ApiError.data 为 unknown） */
export function isActionMismatchInfo(d: unknown): d is DockerActionMismatchInfo {
  if (typeof d !== 'object' || d === null) return false
  const o = d as Record<string, unknown>
  return (
    typeof o.currentState === 'string' &&
    typeof o.action === 'string' &&
    Array.isArray(o.allowedActions) &&
    o.allowedActions.every((a) => typeof a === 'string')
  )
}

/** DK_015 data 收窄 */
export function isComposeLocateFailedInfo(d: unknown): d is DockerComposeLocateFailedInfo {
  if (typeof d !== 'object' || d === null) return false
  const o = d as Record<string, unknown>
  return (
    (o.workingDir === null || typeof o.workingDir === 'string') &&
    (o.configFile === null || typeof o.configFile === 'string') &&
    typeof o.missing === 'string'
  )
}

/** data.reason 收窄（DK_006/008/010/013/016/017 透传 daemon/CLI 原文，不虚构） */
export function dockerErrorReason(data: unknown): string | null {
  if (typeof data === 'object' && data !== null) {
    const reason = (data as Record<string, unknown>).reason
    if (typeof reason === 'string' && reason !== '') return reason
  }
  return null
}

/**
 * 容器六动作中文文案（PageSpec 4.2 操作列按钮 + DK_004「可用：{allowedActions 中文串}」）。
 * PageSpec 第 3 节字典清单未列此 Key（compose_action 为 Compose 操作集口径）——本表为
 * 操作列渲染文案，非 dict 组件用途，不入 enums.ts 枚举族。
 */
export const CONTAINER_ACTION_TEXT: Readonly<Record<ContainerAction, string>> = {
  start: '启动',
  stop: '停止',
  restart: '重启',
  pause: '暂停',
  unpause: '恢复',
  delete: '删除'
}

/** Compose 操作集中文文案（PageSpec 4.3：up/down/restart 按钮组 + 日志入口；compose_action 权威口径） */
export const COMPOSE_ACTION_TEXT: Readonly<Record<ComposeAction, string>> = {
  up: 'up',
  down: 'down',
  restart: 'restart',
  logs: '日志'
}

/** daemon 不可用细节文案（PageSpec 4.0：detail 透传属性→用户可知会文案；其余值原样透传） */
export function daemonDetailText(detail: string | null): string {
  if (detail === 'not_installed') return '未检测到已安装的 Docker'
  if (detail === 'socket_unreachable') return 'daemon socket 不可达'
  return detail ?? ''
}

/**
 * DK_001~018 错误文案（PageSpec 第 1 节错误处理，逐条对应 api 3.4）：
 * 001=「参数非法」兜底（控件与服务端权威面约束，正常 UI 不可达，R-14）；002=兜底跳引导态；
 * 003~005/007/009=不存在族（重拉对应清单）；004=状态不匹配（data 携权威面，视图层拼装）；
 * 006/008/010=in-use 族（+data.reason）；011=预定义网络；012=拉取 Dialog 字段错误；
 * 013/016/017=失败族（+data.reason）；014=项目不存在；015=定位失败（data 携 label 路径）；
 * 018=同项目并发互斥；019~022=创建字段格式（容器名/端口映射/环境变量/挂载）；
 * 023~025=创建执行失败族（+data.reason）；026=compose.yml 校验失败（reason 带行号）；
 * 027=Compose 项目已存在（目录占用）。
 */
export const DK_ERROR_TEXT: Readonly<Record<string, string>> = {
  DK_001: '参数非法',
  DK_002: 'Docker daemon 不可用',
  DK_003: '该容器已不存在（可能已在面板外删除）',
  DK_004: '容器当前状态不允许该操作',
  DK_005: '目标不存在（可能已在面板外删除）',
  DK_006: '镜像被容器引用，无法删除',
  DK_007: '目标不存在（可能已在面板外删除）',
  DK_008: '卷被容器挂载，无法删除',
  DK_009: '目标不存在（可能已在面板外删除）',
  DK_010: '网络被容器接入，无法删除',
  DK_011: '预定义网络（bridge/host/none 等）不可删除',
  DK_012: '镜像引用格式非法（应为 名称[:tag]）',
  DK_013: '镜像拉取失败',
  DK_014: '该项目已不存在',
  DK_015: '工作目录或配置文件缺失，无法执行项目级操作',
  DK_016: 'compose 命令执行失败',
  DK_017: 'Docker 操作失败',
  DK_018: '该项目已有操作进行中，请稍候',
  DK_019: '容器名格式非法（小写字母/数字/连字符）',
  DK_020: '端口映射格式非法（端口须为 1~65535 的数字）',
  DK_021: '环境变量格式非法（键不能为空且不能含空格）',
  DK_022: '挂载格式非法（须提供宿主机来源与容器内目标路径）',
  DK_023: '容器创建失败',
  DK_024: '卷创建失败',
  DK_025: '网络创建失败',
  DK_026: 'compose.yml 校验失败',
  DK_027: 'Compose 项目已存在（换一个项目名）'
}
