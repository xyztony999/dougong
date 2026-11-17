/**
 * env-mgmt API 类型 —— 与 api-env-mgmt.md v1.1 4.1.4 对齐（唯一事实源）。
 * - 环境条目为实时探测投影（D-01/I-06 v1.1 三态 ⚠️A9）：installed=null 仅条目级
 *   探测真错误（通道级不可用由 channel.available=false 页头横幅承载）；
 *   false=未装（含包名不在包数据库形态，D-14）；服务状态经 systemd 单元查询；
 * - env_install_task 四态任务台账（⚠️A3）：running 态详情 5s 轮询终态即停；
 * - children:null 契约（v0.1.1 教训）：菜单分组启用后 MenuNode.children 在叶节点
 *   为 JSON null——TS 类型如实 `T[] | null`，消费侧判空。
 */
import type { DnsProvider } from './enums'
import type { BootstrapStatus, EnvTaskAction, EnvTaskStatus, EnvType, PackageManagerFamily } from './enums'

/** 通道状态（D-03 探测结论；family=null 呈现「探测不可用」） */
export interface EnvChannel {
  family: PackageManagerFamily | null
  available: boolean
}

/** 环境全景条目（目录+实况+在途标记） */
export interface EnvItem {
  envType: EnvType
  displayName: string
  description: string
  installed: boolean | null
  version: string | null
  serviceRunning: boolean | null
  serviceEnabled: boolean | null
  unitName: string
  runningTaskId: number | null
}

/** GET /env/env/list 响应 data */
export interface EnvListData {
  channel: EnvChannel
  items: EnvItem[]
  generatedAt: string
}

/** POST /env/env/install · uninstall 响应（taskId 即刻返回，D-04） */
export interface EnvTaskCreated {
  taskId: number
  action: EnvTaskAction
}

/** 任务清单项 */
export interface EnvTaskItem {
  id: number
  envType: EnvType
  action: EnvTaskAction
  status: EnvTaskStatus
  requestedBy: string
  createdAt: string
  startedAt: string | null
  finishedAt: string | null
  error: string | null
  /** 引导结果（v1.2 安装后配置就绪，D-18）：null=无引导步骤；与 status 独立。 */
  bootstrapStatus: BootstrapStatus | null
  bootstrapNote: string | null
}

/** GET /env/task/list 响应 data */
export interface EnvTaskListData {
  items: EnvTaskItem[]
  total: number
}

/** GET /env/task/detail 响应 data（+输出尾部） */
export interface EnvTaskDetail extends EnvTaskItem {
  outputTail: string
}

/** POST /env/service/{start,stop,restart,enable,disable} 响应 */
export interface EnvServiceOpResult {
  envType: EnvType
  unit: string
}

/** 服务动作路径段（D-09：目录环境单元限定） */
export type EnvServiceOp = 'start' | 'stop' | 'restart' | 'enable' | 'disable'

/** DnsProvider 再导出（凭证管理在 site-mgmt 域；此处避免循环引用时直用 enums） */
export type { DnsProvider }
