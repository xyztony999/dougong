# PageSpec · 计划任务与备份（cron-backup） · 只读用户（readonly）

| 属性 | 值 |
|------|-----|
| 版本 | v1.0 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-14 |
| 用户类型 | readonly |
| 唯一数据源 | 02-detailed/api/api-cron-backup.md v1.0 |
| 公共页定义 | **以 admin 文档（../admin/cron-backup/cron-backup.md）为单一事实源**——两页双 Tab/表格列/筛选/错误处理/刷新与轮询纪律逐字复用；本文档仅声明子集差异与负空间（本模块存在 10 个 admin 专属接口，readonly 为子集视角，对照 firewall/docker-mgmt 模式而非 monitoring 全等模式） |
| 边界依据 | ADR-018：全模块可查看 + 禁一切变更/终端/上传下载——计划任务增删改/手动触发/表达式校验、备份任务增删改/立即执行为变更类动作；产物下载为数据取出动作，readonly 界面无入口、越界请求被全局边界管控拒绝（api 3.3 权限矩阵 10 个 ✗ 项 + AUTH_008） |
| 组件体系 | Vue 3 + Naive UI（同 admin 文档） |

## 第 1 节 模块元信息

| 项 | 值 |
|----|-----|
| 模块编码 | cron-backup |
| 业务类型 | 混合型（同 admin 判定） |
| 页面类型 | hybrid ×2（计划任务页 / 备份页，同 admin） |
| 可见页面 | /cron 与 /cron/backup——与 admin 同路由（readonly 可进入查看任务清单/执行日志/备份任务/历史/详情全集，FL 2.2「全可看」） |
| 错误处理 | 与 admin 一致，另含 AUTH_008 全局边界拒绝提示「权限不足：只读角色禁止计划任务与备份变更操作」（正常 UI 无入口不触发——api 3.3 readonly 写全 ✗ + 全局中间件拦截） |

## 第 2 节 Action 定义

| Action | 方法 | 路径 | 页面 | 说明 |
|--------|------|------|------|------|
| jobList | GET | /cron/job/list | cron·Tab 计划任务（含系统预置位只读分区） | 与 admin 一致 |
| logTail | GET | /cron/log/tail | cron·Tab 执行日志 | 与 admin 一致（行数档位同） |
| backupJobList | GET | /cron/backup-job/list | backup·Tab 备份任务 | 与 admin 一致（含 nextFireAt/lastRun 派生列） |
| backupRunList | GET | /cron/backup-run/list | backup·Tab 备份历史 | 与 admin 一致（同筛选与分页） |
| backupRunDetail | GET | /cron/backup-run/detail | backup·执行详情 Drawer | 与 admin 一致（含轮询纪律；**产物清单无下载按钮**，见负空间） |

**负空间声明**（api 3.3 权限矩阵 10 个 ✗ 项逐项对应）：

| admin Action（readonly 不注册） | api 权限矩阵 ✗ 项 | 负空间落法 |
|--------------------------------|------------------|-----------|
| jobCreate | POST /cron/job/create = ✗ | 工具栏**「新建任务」按钮不渲染**；任务 Dialog 不存在 |
| jobUpdate | POST /cron/job/update = ✗ | 操作列无「编辑」按钮 |
| jobDelete | POST /cron/job/delete = ✗ | 操作列无「删除」按钮 |
| jobValidate | POST /cron/job/validate = ✗ | 无表单即无校验预览区（机制型随宿主表单不可达；只读查看不触发校验） |
| jobTrigger | POST /cron/job/trigger = ✗ | 操作列无「手动触发」按钮；触发结果 Modal 整体不存在 |
| backupJobCreate | POST /cron/backup-job/create = ✗ | 工具栏**「新建备份任务」按钮不渲染**；任务 Drawer 不存在 |
| backupJobUpdate | POST /cron/backup-job/update = ✗ | 操作列无「编辑」按钮 |
| backupJobDelete | POST /cron/backup-job/delete = ✗ | 操作列无「删除」按钮 |
| backupJobRun | POST /cron/backup-job/run = ✗ | 操作列无「立即备份」按钮 |
| backupRunDownload | GET /cron/backup-run/download = ✗（admin 专属 GET，ADR-018 禁上传下载） | 详情 Drawer 产物清单表**「下载」按钮不渲染**（文件名/大小清单保留可见——产物台账为查看内容；表尾灰字「产物下载需管理员权限」指引文案，不虚构 readonly 操作路径） |

> 计划任务表与备份任务表的操作列在 readonly 视角**整列不渲染**（全部行级按钮均为写动作，列消失、其余列定义不变，firewall 同判）；越界兜底：伪造请求经全局中间件 RoleRequired 拦截返回 AUTH_008。

## 第 3 节 字典 Key 清单

与 admin 文档第 3 节完全一致（同一 enums.ts 常量源）：cron_source / backup_source_type / backup_target_type / backup_trigger / backup_run_status（+panel_role 复用）；颜色方案同源（running=blue/success=green/failure=red；panel=blue/existing=gray；local=green/rsync=blue；cron=gray/manual=blue）。

## 第 4 节 列表页（只读视角）

与 admin 文档第 4 节逐字一致，仅以下差异：

| 项 | admin | readonly |
|----|-------|----------|
| cron·Tab 1 工具条（4.1.1） | 新建任务 + 刷新 | **仅刷新**（新建按钮不渲染，负空间） |
| cron·操作列（4.1.3） | 编辑/手动触发/删除 | **整列不渲染** |
| 系统预置位分区（4.1.4） | 只读折叠呈现 | **完全一致**（本就只读，无角色差异，D1 口径-1） |
| cron·Tab 2 执行日志（4.1.5） | 行数档位 + 刷新 | **完全一致**（查看类） |
| backup·Tab 1 工具条（4.2.1） | 新建备份任务 + 刷新 | **仅刷新** |
| backup·操作列（4.2.3） | 立即备份/编辑/删除 | **整列不渲染** |
| backup·Tab 2 历史筛选（4.2.4） | 筛选+分页 | **完全一致**（筛选为查看能力） |
| 历史·操作列「详情」 | 行内打开详情 Drawer | **保留**（详情为查看入口，api detail=✓；Drawer 内下载按钮不渲染） |

空态/加载态/骨架与 admin 一致；lastRun=running 行 readonly 亦见 loading 图标（进行中实况）。

## 第 5 节 表单页

无表单页（同 admin 第 5 节形态判定：任务 Dialog/任务 Drawer/触发 Modal 均为 admin 专属——readonly 无新建/编辑/触发入口即无表单形态；历史筛选为查询条件非表单）。

## 第 6 节 详情页

执行详情 Drawer 与 admin 第 6 节一致（执行信息/备份源明细/产物清单三 Card 均可见，轮询纪律同），仅产物清单行**无「下载」按钮**（负空间第 10 项）；cron 任务无详情形态（同 admin 判定）。

## 第 7 节 路由定义

| 路由 | meta | 与 admin 差异 |
|------|------|--------------|
| /cron、/cron/backup | `{requiresAuth:true}` | 同路由同页面组件（组件内部按角色渲染子集视图）；无独立 readonly 路由 |
| 框架菜单 | — | 计划任务/备份两菜单项对 admin/readonly 均可见（服务端按 ADR-018「全可看」下发） |

## 第 8 节 全局一致性

| # | 检查 | 结果 |
|---|------|------|
| 1 | readonly Action 与 api 只读接口一致、无虚构接口 | ✅ 5 Action（jobList/logTail/backupJobList/backupRunList/backupRunDetail）= api 3.3 readonly ✓ 项全集 |
| 2 | 负空间检查：api 3.3 权限矩阵 10 个 ✗ 项逐项对应 | ✅ 10/10（9 写 Action 按钮/表单不渲染 + download 按钮不渲染，第 2 节表） |
| 3 | 子集视角与 admin 公共定义单一事实源（列定义/筛选/预置位/日志档期全等） | ✅ 第 4 节引用声明 |
| 4 | S8 字典同源 enums.ts（五枚举取值逐字一致） | ✅ 第 3 节 |
| 5 | AUTH_008 边界拒绝文案与全局拦截器一致（正常 UI 不可达） | ✅ 第 1 节 |
| 6 | ADR-018 三禁落法：禁变更=10 项负空间；禁终端=本模块无终端对象；禁上传下载=download 按钮不渲染（产物清单可见、下载指引文案不虚构路径） | ✅ 第 2 节 |
| 7 | 详情 Drawer 轮询纪律对 readonly 同样生效（running 态 5s 轮询只读刷新） | ✅ 第 2/6 节（api D-07） |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-14 | initial | 首次生成：只读视角为 admin 子集（5 Action、同 2 页面、任务两表操作列整列不渲染、无全部新建/编辑/删除/触发/立即执行入口、详情 Drawer 无下载按钮）；api 3.3 权限矩阵 10 个 ✗ 项逐项负空间对应；预置位分区/执行日志/历史筛选/详情轮询与 admin 全等；公共页定义单一事实源 = admin 文档 |

> FX 清单执行方式同 admin 文档第 9 节注记（清单文件未随 payload 发布，按 9 节结构落实 + 第 8 节自查；全量留待 Gate 1）。
