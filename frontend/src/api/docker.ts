/**
 * docker api 层 —— 24 Action 与 api-docker-mgmt.md 第 4 章端点一一对应（S7，无虚构接口）：
 * 读 9 GET（admin+readonly，ADR-018 全可看）+ 写 13 POST（admin 专属 + CSRF——http.ts 统一注入
 * X-CSRF-Token）+ 日志跟随 2 WS（admin+readonly，Cookie 会话握手 + Origin 同源，D-03/D-09）。
 * 函数名 = PageSpec 第 2 节 Action 名；WS 传输层封装于本层（组件不裸连，frontend-rules §3；
 * terminal 先例：一通道一对象，无自动重连——end 帧后服务端关闭，重试=用户重开跟随开关）。
 */
import type { DockerLogEndReason } from '@/types/enums'
import type {
  DockerComposeActionReq,
  DockerComposeActionResult,
  DockerComposeListData,
  DockerContainerActionReq,
  DockerContainerActionResult,
  DockerContainerDeleteReq,
  DockerContainerDeleteResult,
  DockerContainerListData,
  DockerDaemonStatusData,
  DockerImageDeleteReq,
  DockerImageDeleteResult,
  DockerImageListData,
  DockerImagePullReq,
  DockerImagePullResult,
  DockerLogsData,
  DockerNetworkDeleteReq,
  DockerNetworkDeleteResult,
  DockerNetworkListData,
  DockerStatsData,
  DockerVolumeDeleteReq,
  DockerVolumeDeleteResult,
  DockerVolumeListData
} from '@/types/docker'
import { parseDockerLogFrame } from '@/types/docker'
import { buildQuery, request } from './http'

/** daemonStatus：接管结论即时探测（D-01：状态/版本/不可用细节；无缓存按需拉取） */
export function daemonStatus(): Promise<DockerDaemonStatusData> {
  return request<DockerDaemonStatusData>('/docker/daemon/status')
}

export interface ContainerListQuery {
  /** container_status 多值逗号拼接（含过渡态原始值域）；空=全部（D-02） */
  state?: string
  /** Compose 项目名等值匹配；空=全部 */
  composeProject?: string
}

/** containerList：容器投影清单（全量不分页 D-02；服务端筛选+名称字典序+权威动作面） */
export function containerList(q: ContainerListQuery = {}): Promise<DockerContainerListData> {
  return request<DockerContainerListData>('/docker/container/list', {
    query: { state: q.state, composeProject: q.composeProject }
  })
}

export interface LogsQuery {
  tail?: number
}

/** containerLogs：容器日志尾部读取（D-03：tail 缺省 500、取值域 1-2000） */
export function containerLogs(
  containerId: string,
  q: LogsQuery = {}
): Promise<DockerLogsData> {
  return request<DockerLogsData>('/docker/container/logs', {
    query: { containerId, tail: q.tail }
  })
}

/** containerStats：资源占用单次快照（D-07：无轮询，刷新=用户手动） */
export function containerStats(containerId: string): Promise<DockerStatsData> {
  return request<DockerStatsData>('/docker/container/stats', { query: { containerId } })
}

/** ---- 容器六动作（POST，矩阵守卫 DK_004 在服务端，D-04；成功后操作端实况返回） ---- */

export function containerStart(req: DockerContainerActionReq): Promise<DockerContainerActionResult> {
  return request<DockerContainerActionResult>('/docker/container/start', { method: 'POST', body: req })
}

export function containerStop(req: DockerContainerActionReq): Promise<DockerContainerActionResult> {
  return request<DockerContainerActionResult>('/docker/container/stop', { method: 'POST', body: req })
}

export function containerRestart(req: DockerContainerActionReq): Promise<DockerContainerActionResult> {
  return request<DockerContainerActionResult>('/docker/container/restart', { method: 'POST', body: req })
}

export function containerPause(req: DockerContainerActionReq): Promise<DockerContainerActionResult> {
  return request<DockerContainerActionResult>('/docker/container/pause', { method: 'POST', body: req })
}

export function containerUnpause(req: DockerContainerActionReq): Promise<DockerContainerActionResult> {
  return request<DockerContainerActionResult>('/docker/container/unpause', { method: 'POST', body: req })
}

/** containerDelete：删除（仅 stopped 态开放、无 force 字段，I-04/I-05；确认交互在前端） */
export function containerDelete(req: DockerContainerDeleteReq): Promise<DockerContainerDeleteResult> {
  return request<DockerContainerDeleteResult>('/docker/container/delete', { method: 'POST', body: req })
}

/** ---- 镜像 ---- */

/** imageList：镜像投影清单（引用情况不做前置推导标注，I-05） */
export function imageList(): Promise<DockerImageListData> {
  return request<DockerImageListData>('/docker/image/list')
}

/** imagePull：同步拉取（≤600s 终态反馈 D-05；失败 DK_013 data.reason 透传） */
export function imagePull(req: DockerImagePullReq): Promise<DockerImagePullResult> {
  return request<DockerImagePullResult>('/docker/image/pull', { method: 'POST', body: req })
}

/** imageDelete：删除（被引用 DK_006 以 daemon 拒绝为最终权威，D-08） */
export function imageDelete(req: DockerImageDeleteReq): Promise<DockerImageDeleteResult> {
  return request<DockerImageDeleteResult>('/docker/image/delete', { method: 'POST', body: req })
}

/** ---- 卷 ---- */

export function volumeList(): Promise<DockerVolumeListData> {
  return request<DockerVolumeListData>('/docker/volume/list')
}

/** volumeDelete：删除（数据销毁确认在前端；被挂载 DK_008，D-08） */
export function volumeDelete(req: DockerVolumeDeleteReq): Promise<DockerVolumeDeleteResult> {
  return request<DockerVolumeDeleteResult>('/docker/volume/delete', { method: 'POST', body: req })
}

/** ---- 网络 ---- */

export function networkList(): Promise<DockerNetworkListData> {
  return request<DockerNetworkListData>('/docker/network/list')
}

/** networkDelete：删除（预定义网络 DK_011 / 被接入 DK_010，D-08） */
export function networkDelete(req: DockerNetworkDeleteReq): Promise<DockerNetworkDeleteResult> {
  return request<DockerNetworkDeleteResult>('/docker/network/delete', { method: 'POST', body: req })
}

/** ---- Compose（label 聚合投影 + CLI 动词端点，D-06） ---- */

export function composeList(): Promise<DockerComposeListData> {
  return request<DockerComposeListData>('/docker/compose/list')
}

export function composeUp(req: DockerComposeActionReq): Promise<DockerComposeActionResult> {
  return request<DockerComposeActionResult>('/docker/compose/up', { method: 'POST', body: req })
}

export function composeDown(req: DockerComposeActionReq): Promise<DockerComposeActionResult> {
  return request<DockerComposeActionResult>('/docker/compose/down', { method: 'POST', body: req })
}

export function composeRestart(req: DockerComposeActionReq): Promise<DockerComposeActionResult> {
  return request<DockerComposeActionResult>('/docker/compose/restart', { method: 'POST', body: req })
}

/** composeLogs：项目聚合日志尾部读取（校验含 DK_015 定位判定，api 4.3.4） */
export function composeLogs(project: string, q: LogsQuery = {}): Promise<DockerLogsData> {
  return request<DockerLogsData>('/docker/compose/logs', {
    query: { project, tail: q.tail }
  })
}

/** ---- 日志跟随 WS（D-03 双通道之流式通道；Cookie 会话握手 + Origin 同源，api 3.1） ---- */

export interface DockerLogSocketEvents {
  /** log 帧：新日志行追加 */
  onLog(line: string): void
  /** end 帧：服务端终止原因（end 后服务端关闭连接） */
  onEnd(reason: DockerLogEndReason): void
  /** 非法帧（未知类型/残缺字段/坏 JSON）——协议违规，调用方本地收口断开 */
  onInvalidFrame(): void
  /** WS 关闭（end 帧已收到=正常终止；否则=握手拒绝或网络层异常，调用方按需收口） */
  onClose(info: { endReceived: boolean; code: number }): void
}

export interface DockerLogSocket {
  /** 客户端主动关闭（关闭 Drawer 即断开，无残留订阅） */
  close(): void
}

/** 同源 WS 地址（https→wss；开发态经 vite proxy /docker ws 透传，禁止硬编码后端地址） */
function dockerWsUrl(path: string, query: Record<string, string | number | undefined>): string {
  const proto = window.location.protocol === 'https:' ? 'wss' : 'ws'
  return `${proto}://${window.location.host}${path}${buildQuery(query)}`
}

/**
 * 日志跟随通用传输层（containerLogsFollow / composeLogsFollow 共用，D-03 帧协议一致）：
 * 握手即回放尾部 tail 行 → 流式推送 → end 帧后服务端关闭；客户端无上行帧。
 */
function connectLogFollow(
  path: string,
  query: Record<string, string | number | undefined>,
  events: DockerLogSocketEvents
): DockerLogSocket {
  const ws = new WebSocket(dockerWsUrl(path, query))
  let endReceived = false
  let closeSent = false

  ws.onmessage = (ev: MessageEvent): void => {
    if (typeof ev.data !== 'string') {
      events.onInvalidFrame() // 协议为 JSON 文本帧（D-03），二进制帧=违规
      return
    }
    const frame = parseDockerLogFrame(ev.data)
    if (frame === null) {
      events.onInvalidFrame()
      return
    }
    if (frame.type === 'log') events.onLog(frame.line)
    else {
      endReceived = true
      events.onEnd(frame.reason)
    }
  }

  ws.onclose = (ev: CloseEvent): void => {
    events.onClose({ endReceived, code: ev.code })
  }

  return {
    close(): void {
      if (!closeSent && ws.readyState === WebSocket.OPEN) {
        closeSent = true
        ws.close(1000, '')
      }
    }
  }
}

/** containerLogsFollow Action：容器日志实时跟随（握手 query {containerId, tail?}） */
export function connectContainerLogsFollow(
  params: { containerId: string; tail?: number },
  events: DockerLogSocketEvents
): DockerLogSocket {
  return connectLogFollow('/docker/container/logs-follow', params, events)
}

/** composeLogsFollow Action：Compose 项目聚合日志实时跟随（握手 query {project, tail?}） */
export function connectComposeLogsFollow(
  params: { project: string; tail?: number },
  events: DockerLogSocketEvents
): DockerLogSocket {
  return connectLogFollow('/docker/compose/logs-follow', params, events)
}


// ---- v1.1 创建类 Action（api-docker-mgmt.md v1.1 FR-19~22）----

/** 容器创建请求体 */
export interface DockerContainerCreateRequest {
  name: string
  image: string
  ports?: { hostPort: string; containerPort: string; protocol?: string }[]
  env?: { key: string; value: string }[]
  mounts?: { source: string; target: string; mode?: string }[]
  restartPolicy?: string
  network?: string
}

/** 容器创建响应 */
export interface DockerContainerCreateData {
  containerId: string
  name: string
  state: string
}

/** containerCreate：创建并启动容器（镜像缺失自动拉取；DK_019~023） */
export function containerCreate(req: DockerContainerCreateRequest): Promise<DockerContainerCreateData> {
  return request<DockerContainerCreateData>('/docker/container/create', { method: 'POST', body: req })
}

/** volumeCreate：创建命名卷（DK_024） */
export function volumeCreate(name: string, driver?: string): Promise<{ name: string; driver: string }> {
  return request('/docker/volume/create', { method: 'POST', body: { name, driver } })
}

/** networkCreate：创建自定义网络（DK_025） */
export function networkCreate(name: string, driver?: string, subnet?: string, gateway?: string): Promise<{ name: string; driver: string }> {
  return request('/docker/network/create', { method: 'POST', body: { name, driver, subnet, gateway } })
}

/** Compose 项目创建响应 */
export interface ComposeCreateData {
  projectName: string
  workingDir: string
  configFile: string
  started: boolean
}

/** composeCreate：新建 Compose 项目（YAML 校验+落盘+自动 up -d；DK_026/027） */
export function composeCreate(projectName: string, composeYaml: string, autoUp?: boolean): Promise<ComposeCreateData> {
  return request<ComposeCreateData>('/docker/compose/create', { method: 'POST', body: { projectName, composeYaml, autoUp } })
}
