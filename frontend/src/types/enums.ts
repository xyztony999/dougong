/**
 * 字典常量单一事实源（PageSpec 第 3 节；取值 = 01-solution/04-business-enum.md 定稿值）。
 * 设计决策：面板单机、8 枚举全固定值，不实现 POST /{domain}/dict/batch，
 * 字典由前端 TS 常量提供（PageSpec 第 3 节偏差声明）。
 * 颜色方案 = PageSpec 第 3 节语义映射（NTag type）。
 */
import type { Component } from 'vue'

export type TagType = 'default' | 'primary' | 'info' | 'success' | 'warning' | 'error'

export interface DictItem {
  label: string
  tag: TagType
}

/** panel_role：admin / readonly */
export type PanelRole = 'admin' | 'readonly'
export const PANEL_ROLE: Record<PanelRole, DictItem> = {
  admin: { label: '管理员', tag: 'default' },
  readonly: { label: '只读', tag: 'default' }
}

/** panel_user_status：active / locked */
export type PanelUserStatus = 'active' | 'locked'
export const PANEL_USER_STATUS: Record<PanelUserStatus, DictItem> = {
  active: { label: '正常', tag: 'success' },
  locked: { label: '已锁定', tag: 'error' }
}

/** session_status：active / expired / revoked（派生展示，预留） */
export type SessionStatus = 'active' | 'expired' | 'revoked'
export const SESSION_STATUS: Record<SessionStatus, DictItem> = {
  active: { label: '有效', tag: 'success' },
  expired: { label: '已过期', tag: 'default' },
  revoked: { label: '已吊销', tag: 'error' }
}

/** session_end_reason：self_logout / forced_logout / cascade（预留） */
export type SessionEndReason = 'self_logout' | 'forced_logout' | 'cascade'
export const SESSION_END_REASON: Record<SessionEndReason, DictItem> = {
  self_logout: { label: '主动登出', tag: 'default' },
  forced_logout: { label: '强制下线', tag: 'error' },
  cascade: { label: '级联吊销', tag: 'warning' }
}

/** audit_action（审计类别）：change / security */
export type AuditCategory = 'change' | 'security'
export const AUDIT_CATEGORY: Record<AuditCategory, DictItem> = {
  change: { label: '变更', tag: 'info' },
  security: { label: '安全', tag: 'warning' }
}

/** audit_result：success / failure */
export type AuditResult = 'success' | 'failure'
export const AUDIT_RESULT: Record<AuditResult, DictItem> = {
  success: { label: '成功', tag: 'success' },
  failure: { label: '失败', tag: 'error' }
}

/** audit_source：web / cli / system */
export type AuditSource = 'web' | 'cli' | 'system'
export const AUDIT_SOURCE: Record<AuditSource, DictItem> = {
  web: { label: 'Web', tag: 'default' },
  cli: { label: 'CLI', tag: 'default' },
  system: { label: '系统', tag: 'default' }
}

/** init_status：uninitialized / initialized（路由守卫判定，不展示） */
export type InitStatus = 'uninitialized' | 'initialized'
export const INIT_STATUS: Record<InitStatus, DictItem> = {
  uninitialized: { label: '未初始化', tag: 'warning' },
  initialized: { label: '已初始化', tag: 'success' }
}

/** https_mode：off / self_signed / imported（panel-core，R-10 三态） */
export type HttpsMode = 'off' | 'self_signed' | 'imported'
export const HTTPS_MODE: Record<HttpsMode, DictItem> = {
  off: { label: '关闭（HTTP 明文）', tag: 'default' },
  self_signed: { label: '自签证书', tag: 'info' },
  imported: { label: '导入证书', tag: 'success' }
}

/** listen_scope：all / localhost / custom（默认 all=0.0.0.0） */
export type ListenScope = 'all' | 'localhost' | 'custom'
export const LISTEN_SCOPE: Record<ListenScope, DictItem> = {
  all: { label: '全部地址（0.0.0.0）', tag: 'default' },
  localhost: { label: '仅本机（localhost）', tag: 'default' },
  custom: { label: '自定义地址', tag: 'default' }
}

/** panel_cert_source：self_signed / imported（证书摘要来源 tag） */
export type PanelCertSource = 'self_signed' | 'imported'
export const PANEL_CERT_SOURCE: Record<PanelCertSource, DictItem> = {
  self_signed: { label: '自签', tag: 'info' },
  imported: { label: '导入', tag: 'success' }
}

/** config_effect_mode：immediate / restart_required（Card 角标，仅 listen.* 为 restart_required） */
export type ConfigEffectMode = 'immediate' | 'restart_required'
export const CONFIG_EFFECT_MODE: Record<ConfigEffectMode, DictItem> = {
  immediate: { label: '即时生效', tag: 'success' },
  restart_required: { label: '需重启', tag: 'warning' }
}

/** ---- monitoring 字典（api-monitoring.md 3.4；取值 = 04-business-enum.md 定稿，趋势查询条件非状态枚举 → default 色） ---- */

/** metric_window：last_1h / last_24h（趋势时间窗口，默认 last_1h） */
export type MetricWindow = 'last_1h' | 'last_24h'
export const METRIC_WINDOW: Record<MetricWindow, DictItem> = {
  last_1h: { label: '近 1 小时', tag: 'default' },
  last_24h: { label: '近 24 小时', tag: 'default' }
}

/** metric_dimension：cpu / memory / disk / network / load（趋势指标维度，默认 cpu） */
export type MetricDimension = 'cpu' | 'memory' | 'disk' | 'network' | 'load'
export const METRIC_DIMENSION: Record<MetricDimension, DictItem> = {
  cpu: { label: 'CPU', tag: 'default' },
  memory: { label: '内存', tag: 'default' },
  disk: { label: '磁盘', tag: 'default' },
  network: { label: '网络', tag: 'default' },
  load: { label: '负载', tag: 'default' }
}

/** metric_scope：total / per_core（CPU 口径，默认 total；仅 dimension=cpu 时合法，D-04） */
export type MetricScope = 'total' | 'per_core'
export const METRIC_SCOPE: Record<MetricScope, DictItem> = {
  total: { label: '总体', tag: 'default' },
  per_core: { label: '分核', tag: 'default' }
}

/** ---- file-manager 字典（api-file-manager.md 3.4；取值 = 04-business-enum.md 定稿，PageSpec 第 3 节） ---- */

/** file_type：file / directory / symlink（目录条目类型；图标/颜色映射见 views/file-manager/icons.ts） */
export type FileEntryType = 'file' | 'directory' | 'symlink'
export const FILE_TYPE: Record<FileEntryType, DictItem> = {
  file: { label: '文件', tag: 'default' },
  directory: { label: '目录', tag: 'info' },
  symlink: { label: '符号链接', tag: 'warning' }
}

/** archive_format：tar / tar.gz / zip（技术标识原值透传不翻译，PageSpec 第 8 节 #12） */
export type ArchiveFormat = 'tar' | 'tar.gz' | 'zip'
export const ARCHIVE_FORMAT: Record<ArchiveFormat, DictItem> = {
  tar: { label: 'tar', tag: 'default' },
  'tar.gz': { label: 'tar.gz', tag: 'default' },
  zip: { label: 'zip', tag: 'default' }
}

/** ---- service-mgmt 字典（api-service-mgmt.md 3.4；取值 = 04-business-enum.md 定稿，PageSpec 第 3 节颜色方案） ---- */

/** service_active_state：active / inactive / failed（运行维度；active=green/inactive=gray/failed=red） */
export type ServiceActiveState = 'active' | 'inactive' | 'failed'
export const SERVICE_ACTIVE_STATE: Record<ServiceActiveState, DictItem> = {
  active: { label: '运行中', tag: 'success' },
  inactive: { label: '已停止', tag: 'default' },
  failed: { label: '失败', tag: 'error' }
}

/** activeState 三态收窄（SVC_007 data.activeState 为透传 string，入行前判定） */
export function isServiceActiveState(v: unknown): v is ServiceActiveState {
  return v === 'active' || v === 'inactive' || v === 'failed'
}

/** service_enabled_state：enabled / disabled（两态主轴 enabled=blue/disabled=gray；响应可透传 static 等非两态值——dict 未命中降级原值灰字，⚠️A14/D-05） */
export type ServiceEnabledState = 'enabled' | 'disabled'
export const SERVICE_ENABLED_STATE: Record<ServiceEnabledState, DictItem> = {
  enabled: { label: '已启用', tag: 'info' },
  disabled: { label: '未启用', tag: 'default' }
}

/** unit_type：官方 11 类全集（类型列 dict，无语义色） */
export type UnitType =
  | 'service'
  | 'socket'
  | 'timer'
  | 'mount'
  | 'path'
  | 'target'
  | 'automount'
  | 'slice'
  | 'swap'
  | 'scope'
  | 'device'
export const UNIT_TYPE: Record<UnitType, DictItem> = {
  service: { label: '服务', tag: 'default' },
  socket: { label: '套接字', tag: 'default' },
  timer: { label: '定时器', tag: 'default' },
  mount: { label: '挂载', tag: 'default' },
  path: { label: '路径', tag: 'default' },
  target: { label: '目标', tag: 'default' },
  automount: { label: '自动挂载', tag: 'default' },
  slice: { label: '分片', tag: 'default' },
  swap: { label: '交换分区', tag: 'default' },
  scope: { label: '作用域', tag: 'default' },
  device: { label: '设备', tag: 'default' }
}

/** ---- site-mgmt 字典（api-site-mgmt.md 3.4；取值 = 04-business-enum.md 定稿，PageSpec 第 3 节颜色方案） ---- */

/** site_type：static / php_fpm / reverse_proxy（新建站点模板类型；无语义色） */
export type SiteType = 'static' | 'php_fpm' | 'reverse_proxy'
export const SITE_TYPE: Record<SiteType, DictItem> = {
  static: { label: '静态站点', tag: 'default' },
  php_fpm: { label: 'PHP 站点', tag: 'default' },
  reverse_proxy: { label: '反向代理', tag: 'default' }
}

/** site_enabled：enabled / disabled（启停投影，enabled=green/disabled=gray） */
export type SiteEnabled = 'enabled' | 'disabled'
export const SITE_ENABLED: Record<SiteEnabled, DictItem> = {
  enabled: { label: '已启用', tag: 'success' },
  disabled: { label: '已停用', tag: 'default' }
}

/** cert_source：acme / imported（acme=blue/imported=gray） */
export type CertSource = 'acme' | 'imported'
export const CERT_SOURCE: Record<CertSource, DictItem> = {
  acme: { label: 'ACME 签发', tag: 'info' },
  imported: { label: '存量导入', tag: 'default' }
}

/** acme_challenge：http_01 / tls_alpn（ACME 挑战方式；无语义色） */
export type AcmeChallenge = 'http_01' | 'tls_alpn'
export const ACME_CHALLENGE: Record<AcmeChallenge, DictItem> = {
  http_01: { label: 'HTTP-01', tag: 'default' },
  tls_alpn: { label: 'TLS-ALPN', tag: 'default' }
}

/** cert_status：pending/valid/expiring_soon/expired/renew_failed（pending=blue、valid=green、expiring_soon=orange、expired/renew_failed=red） */
export type CertStatus = 'pending' | 'valid' | 'expiring_soon' | 'expired' | 'renew_failed'
export const CERT_STATUS: Record<CertStatus, DictItem> = {
  pending: { label: '申请中', tag: 'info' },
  valid: { label: '有效', tag: 'success' },
  expiring_soon: { label: '即将到期', tag: 'warning' },
  expired: { label: '已过期', tag: 'error' },
  renew_failed: { label: '续期失败', tag: 'error' }
}

/** site_log_type：access / error（Drawer 日志 Card Tab；无语义色） */
export type SiteLogType = 'access' | 'error'
export const SITE_LOG_TYPE: Record<SiteLogType, DictItem> = {
  access: { label: '访问日志', tag: 'default' },
  error: { label: '错误日志', tag: 'default' }
}

/** ---- database-mgmt 字典（api-database-mgmt.md 3.4；取值 = 04-business-enum.md 定稿，PageSpec 第 3 节颜色方案） ---- */

/** db_engine：mysql / mariadb / redis（mysql/mariadb=blue→info；redis=purple——NTag 无紫色语义型，视图层 rgba 语义色承载，见 views/database-mgmt/render.ts） */
export type DbEngine = 'mysql' | 'mariadb' | 'redis'
export const DB_ENGINE: Record<DbEngine, DictItem> = {
  mysql: { label: 'MySQL', tag: 'info' },
  mariadb: { label: 'MariaDB', tag: 'info' },
  redis: { label: 'Redis', tag: 'default' }
}

/** db_conn_mode：socket / tcp（连接方式列+凭据弹窗下拉；无语义色） */
export type DbConnMode = 'socket' | 'tcp'
export const DB_CONN_MODE: Record<DbConnMode, DictItem> = {
  socket: { label: 'Socket', tag: 'default' },
  tcp: { label: 'TCP', tag: 'default' }
}

/** instance_conn_state：reachable / unreachable（探测态 I-03；reachable=green / unreachable=orange） */
export type InstanceConnState = 'reachable' | 'unreachable'
export const INSTANCE_CONN_STATE: Record<InstanceConnState, DictItem> = {
  reachable: { label: '可达', tag: 'success' },
  unreachable: { label: '不可达', tag: 'warning' }
}

/** grant_level：readonly / write / all（库级授权三档 D-10；readonly=gray / write=blue / all=red 高危着色） */
export type GrantLevel = 'readonly' | 'write' | 'all'
export const GRANT_LEVEL: Record<GrantLevel, DictItem> = {
  readonly: { label: '只读', tag: 'default' },
  write: { label: '读写', tag: 'info' },
  all: { label: '全部权限', tag: 'error' }
}

/** ---- firewall 字典（api-firewall.md 3.4；取值 = 04-business-enum.md 定稿，PageSpec 第 3 节颜色方案） ---- */

/** firewall_backend：ufw / firewalld / none（识别结论；ufw/firewalld=blue→info，none 引导态降级灰） */
export type FirewallBackend = 'ufw' | 'firewalld' | 'none'
export const FIREWALL_BACKEND: Record<FirewallBackend, DictItem> = {
  ufw: { label: 'UFW', tag: 'info' },
  firewalld: { label: 'firewalld', tag: 'info' },
  none: { label: '未检测到', tag: 'default' }
}

/** firewall_rule_action：allow=green / deny=red / reject=orange→warning / limit=blue→info（PageSpec 第 3 节） */
export type FirewallRuleAction = 'allow' | 'deny' | 'reject' | 'limit'
export const FIREWALL_RULE_ACTION: Record<FirewallRuleAction, DictItem> = {
  allow: { label: '放行', tag: 'success' },
  deny: { label: '丢弃', tag: 'error' },
  reject: { label: '拒绝', tag: 'warning' },
  limit: { label: '限速放行', tag: 'info' }
}

/** firewall_rule_protocol：tcp / udp（无语义色） */
export type FirewallRuleProtocol = 'tcp' | 'udp'
export const FIREWALL_RULE_PROTOCOL: Record<FirewallRuleProtocol, DictItem> = {
  tcp: { label: 'TCP', tag: 'default' },
  udp: { label: 'UDP', tag: 'default' }
}

/** listen_port_state：allowed_by_rule=green / no_allow_rule=orange→warning（PageSpec 第 3 节；风险行另加橙底高亮 4.4） */
export type ListenPortState = 'allowed_by_rule' | 'no_allow_rule'
export const LISTEN_PORT_STATE: Record<ListenPortState, DictItem> = {
  allowed_by_rule: { label: '已被规则放行', tag: 'success' },
  no_allow_rule: { label: '无放行规则', tag: 'warning' }
}

/** ---- terminal 字典（api-terminal.md 3.4；取值 = 04-business-enum.md 定稿，PageSpec 第 3 节颜色方案） ---- */

/**
 * terminal_end_reason：closed 帧断开原因（6 值含 D3 增补 protocol_error）。
 * 颜色：self_close=gray / idle_timeout·hard_limit=orange / panel_session_invalid=blue / network_error·protocol_error=red。
 */
export type TerminalEndReason =
  | 'self_close'
  | 'idle_timeout'
  | 'hard_limit'
  | 'panel_session_invalid'
  | 'network_error'
  | 'protocol_error'
export const TERMINAL_END_REASON: Record<TerminalEndReason, DictItem> = {
  self_close: { label: '会话已关闭', tag: 'default' },
  idle_timeout: { label: '会话因 30 分钟无操作已自动断开（服务端持续输出不重置计时）', tag: 'warning' },
  hard_limit: { label: '会话已达最长 8 小时上限被断开，如需继续请新建会话', tag: 'warning' },
  panel_session_invalid: { label: '面板登录会话已失效，终端已断开——请重新登录', tag: 'info' },
  network_error: { label: '连接中断（网络异常或浏览器休眠）', tag: 'error' },
  protocol_error: { label: '连接因协议异常被断开', tag: 'error' }
}

/** terminal_identity_source：panel_run_user / specified_system_user（新建会话身份来源 radio，api R-18 两分支） */
export type TerminalIdentitySource = 'panel_run_user' | 'specified_system_user'
export const TERMINAL_IDENTITY_SOURCE: Record<TerminalIdentitySource, DictItem> = {
  panel_run_user: { label: '默认身份（面板运行用户，通常 root）', tag: 'default' },
  specified_system_user: { label: '指定系统用户', tag: 'default' }
}

/** closed 帧 reason 收窄（parseServerFrame 入库前判定；未知值不构造 closed 帧） */
export function isTerminalEndReason(v: unknown): v is TerminalEndReason {
  return (
    v === 'self_close' ||
    v === 'idle_timeout' ||
    v === 'hard_limit' ||
    v === 'panel_session_invalid' ||
    v === 'network_error' ||
    v === 'protocol_error'
  )
}

/** identitySource 收窄（ready 帧 identity.source / 握手参数共用） */
export function isTerminalIdentitySource(v: unknown): v is TerminalIdentitySource {
  return v === 'panel_run_user' || v === 'specified_system_user'
}

/** ---- cron-backup 字典（api-cron-backup.md 3.4；取值 = 04-business-enum.md 定稿，PageSpec 第 3 节颜色方案） ---- */

/** cron_source：panel / existing（panel=blue→info 活跃管理态 / existing=gray 接管存量中性，D-02） */
export type CronSource = 'panel' | 'existing'
export const CRON_SOURCE: Record<CronSource, DictItem> = {
  panel: { label: '面板创建', tag: 'info' },
  existing: { label: '系统既有', tag: 'default' }
}

/** backup_source_type：site_dir / database（site_dir=green 就近稳定 / database=blue→info） */
export type BackupSourceType = 'site_dir' | 'database'
export const BACKUP_SOURCE_TYPE: Record<BackupSourceType, DictItem> = {
  site_dir: { label: '站点目录', tag: 'success' },
  database: { label: '数据库', tag: 'info' }
}

/** backup_target_type：local / rsync（local=green 就近稳定 / rsync=blue→info 传输态中性） */
export type BackupTargetType = 'local' | 'rsync'
export const BACKUP_TARGET_TYPE: Record<BackupTargetType, DictItem> = {
  local: { label: '本地目录', tag: 'success' },
  rsync: { label: 'rsync 远程', tag: 'info' }
}

/** backup_trigger：cron / manual（cron=gray 系统常态 / manual=blue→info 即时动作态） */
export type BackupTrigger = 'cron' | 'manual'
export const BACKUP_TRIGGER: Record<BackupTrigger, DictItem> = {
  cron: { label: '定时触发', tag: 'default' },
  manual: { label: '手动触发', tag: 'info' }
}

/** backup_run_status：running / success / failure（running=blue→info 附 loading / success=green / failure=red，ADR-016） */
export type BackupRunStatus = 'running' | 'success' | 'failure'
export const BACKUP_RUN_STATUS: Record<BackupRunStatus, DictItem> = {
  running: { label: '执行中', tag: 'info' },
  success: { label: '成功', tag: 'success' },
  failure: { label: '失败', tag: 'error' }
}

/** ---- docker-mgmt 字典（api-docker-mgmt.md 3.4；取值 = 04-business-enum.md 定稿，PageSpec 第 3 节颜色方案） ---- */

/** container_status：running / stopped / paused（running=green / paused=orange / stopped=gray） */
export type ContainerStatus = 'running' | 'stopped' | 'paused'
export const CONTAINER_STATUS: Record<ContainerStatus, DictItem> = {
  running: { label: '运行中', tag: 'success' },
  stopped: { label: '已停止', tag: 'default' },
  paused: { label: '已暂停', tag: 'warning' }
}

/** container 六动作标识（supportedActions 服务端权威面的值域，api D-02/D-04；中文文案见 types/docker.ts） */
export type ContainerAction = 'start' | 'stop' | 'restart' | 'pause' | 'unpause' | 'delete'

/** docker_daemon_state：available / unavailable（available=green「已接管」/ unavailable=orange 引导态，D-01） */
export type DockerDaemonState = 'available' | 'unavailable'
export const DOCKER_DAEMON_STATE: Record<DockerDaemonState, DictItem> = {
  available: { label: '已接管 Docker', tag: 'success' },
  unavailable: { label: 'Docker 不可用', tag: 'warning' }
}

/** compose_action：up / down / restart / logs（操作集权威口径——操作列按钮组渲染依据；非 dict 组件用途，无语义色） */
export type ComposeAction = 'up' | 'down' | 'restart' | 'logs'
export const COMPOSE_ACTION: Record<ComposeAction, DictItem> = {
  up: { label: 'up', tag: 'default' },
  down: { label: 'down', tag: 'default' },
  restart: { label: 'restart', tag: 'default' },
  logs: { label: '日志', tag: 'default' }
}

/** WS end 帧 reason 收窄（帧文本入库前判定；未知值不构造 end 帧由调用方按协议违规收口） */
export function isDockerLogEndReason(v: unknown): v is DockerLogEndReason {
  return (
    v === 'container_removed' ||
    v === 'container_stopped' ||
    v === 'project_gone' ||
    v === 'locate_failed' ||
    v === 'daemon_lost'
  )
}

/**
 * docker 日志跟随 end 帧终止原因（api 3.4 业务常量非枚举池条目，D-03）。
 * 颜色：container_removed/container_stopped/project_gone/locate_failed=info 提示级；
 * daemon_lost=red 连接中断（PageSpec 5.1/5.2 分级：前两者 info、daemon_lost error）。
 */
export type DockerLogEndReason =
  | 'container_removed'
  | 'container_stopped'
  | 'project_gone'
  | 'locate_failed'
  | 'daemon_lost'
export const DOCKER_LOG_END_REASON: Record<DockerLogEndReason, DictItem> = {
  container_removed: { label: '容器已删除，日志流结束', tag: 'info' },
  container_stopped: { label: '容器已停止，日志流结束', tag: 'info' },
  project_gone: { label: '项目已不存在，日志流结束', tag: 'info' },
  locate_failed: { label: '工作目录或配置文件缺失，日志流结束', tag: 'info' },
  daemon_lost: { label: 'daemon 连接中断', tag: 'error' }
}

/**
 * 审计细粒度动作码（api-account-auth.md 3.4 业务常量 15 个 + api-panel-core.md 3.4 3 个，
 * 值逐字对齐）。中文释义为前端展示层文案（api 常量表未给文案）。
 */
export const AUDIT_ACTION_TEXT: Readonly<Record<string, string>> = {
  login_success: '登录成功',
  login_failed: '登录失败',
  account_locked: '账户锁定',
  account_unlocked: '账户解锁',
  logout: '登出',
  force_logout: '强制下线',
  user_created: '创建用户',
  user_deleted: '删除用户',
  password_reset: '重置密码',
  password_changed: '修改密码',
  totp_enabled: '启用 TOTP',
  totp_disabled: '解绑 TOTP',
  init_completed: '初始化完成',
  audit_exported: '导出审计',
  audit_purged: '审计清理',
  panel_config_changed: '面板配置变更',
  panel_cert_updated: '面板证书更新',
  panel_entry_path_regenerated: '重新生成安全入口',
  // site-mgmt（api-site-mgmt.md 3.4 业务常量 13 个，值逐字对齐）
  site_created: '创建站点',
  site_updated: '编辑站点',
  proxy_updated: '编辑反代参数',
  site_source_saved: '保存站点源码',
  site_enabled: '启用站点',
  site_disabled: '停用站点',
  nginx_reloaded: '重载 nginx',
  cert_requested: '申请证书',
  cert_deployed: '部署证书',
  cert_undeployed: '解除部署证书',
  cert_renewal_retried: '重试证书续期',
  cert_deleted: '删除证书',
  cert_renewed: '证书自动续期',
  // database-mgmt（api-database-mgmt.md 3.4 业务常量 D-13 审计动作码 13 个，值逐字对齐）
  instance_rescanned: '手动重新探测实例',
  instance_credentials_set: '配置实例凭据',
  instance_removed: '移除实例纳管',
  database_created: '创建数据库',
  database_dropped: '删除数据库',
  dbuser_created: '创建数据库用户',
  dbuser_deleted: '删除数据库用户',
  dbuser_granted: '库级授权',
  dbuser_revoked: '回收库级授权',
  instance_root_password_changed: '修改 MySQL root 密码',
  database_exported: '导出数据库备份',
  database_imported: '导入数据库备份',
  redis_password_set: '设置 Redis 密码',
  // terminal（api-terminal.md 3.4 业务常量 2 个，值逐字对齐；开合审计经 account-auth 落库，D-06）
  terminal_session_opened: '开启终端会话',
  terminal_session_closed: '关闭终端会话',
  // firewall（api-firewall.md 3.4 业务常量 2 个，值逐字对齐；审计经 account-auth AuditService.Record 落库，D-09）
  firewall_rule_created: '添加防火墙规则',
  firewall_rule_deleted: '删除防火墙规则',
  // docker-mgmt（api-docker-mgmt.md 3.4 业务常量 13 个，值逐字对齐；审计经 account-auth AuditService.Record 落库，D-09）
  container_started: '启动容器',
  container_stopped: '停止容器',
  container_restarted: '重启容器',
  container_paused: '暂停容器',
  container_unpaused: '恢复容器',
  container_deleted: '删除容器',
  image_pulled: '拉取镜像',
  image_deleted: '删除镜像',
  volume_deleted: '删除卷',
  network_deleted: '删除网络',
  compose_up: 'Compose up',
  compose_down: 'Compose down',
  compose_restarted: '重启 Compose 项目',
  // cron-backup（api-cron-backup.md 3.4 业务常量 10 个，值逐字对齐；审计经 account-auth AuditService.Record 落库，3.2）
  cron_job_created: '创建计划任务',
  cron_job_updated: '编辑计划任务',
  cron_job_deleted: '删除计划任务',
  cron_job_triggered: '手动触发计划任务',
  backup_job_created: '创建备份任务',
  backup_job_updated: '编辑备份任务',
  backup_job_deleted: '删除备份任务',
  backup_run_triggered: '立即执行备份',
  backup_artifact_downloaded: '下载备份产物',
  backup_rotated: '备份轮转清理'
}

export const AUDIT_ACTION_CODES: readonly string[] = Object.keys(AUDIT_ACTION_TEXT)

/** 字典安全取值：未知值（如后端透传的过渡态）原值展示、default 色 */
export function dictItem(
  map: Readonly<Record<string, DictItem>>,
  value: string
): DictItem {
  return map[value] ?? { label: value, tag: 'default' }
}

/** 布尔 tag（totpEnabled 等）：true=green / false=gray（PageSpec 第 3 节） */
export function boolTag(value: boolean, trueLabel: string, falseLabel: string): DictItem {
  return value ? { label: trueLabel, tag: 'success' } : { label: falseLabel, tag: 'default' }
}

/** 图标类型别名（菜单/按钮用） */
export type IconComponent = Component
