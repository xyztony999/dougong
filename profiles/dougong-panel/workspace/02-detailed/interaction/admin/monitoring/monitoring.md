# PageSpec · 系统监控（monitoring） · 管理员（admin）

| 属性 | 值 |
|------|-----|
| 版本 | v1.0 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-12 |
| 用户类型 | admin（readonly 视角与本视角完全一致——本模块无任何变更操作，详见 readonly 文档） |
| 唯一数据源 | 02-detailed/api/api-monitoring.md v1.0 |
| 组件体系 | Vue 3 + Naive UI + **echarts**（趋势折线图；Naive UI 为栈版本表选型，echarts 为 idea.md §5.3 前端关键组件材料给定选型；无激活栈 profile，不使用 vue3-engine 栈 Scm* 组件） |
| 确认方式 | headless run：7 项交互确认依据 D3 契约（api-monitoring.md D-01/D-03/D-04/D-08）+ ADR-014/ADR-018 + ⚠️A12/A13 + FL v1.0 推导，标注于各节 |

## 第 1 节 模块元信息

| 项 | 值 | 确认依据 |
|----|-----|---------|
| 模块编码 | monitoring（域前缀 /monitoring） | api 文档 3.1 |
| 业务类型 | **工具类**（分册 2.1 三型之外的诚实标注：无聚合根、无状态机、无 MQ、无写接口——D2/D3 判定；只读观测数据展示） | api 1.4/4.x；domain-model.md monitoring 域 |
| 页面类型 | **hybrid**（监控概览 = 快照/TOP/系统信息多独立区块组合；趋势页 = 参数交互 + echarts 图表集成） | 分册 2.2「详情页多个独立区块组合→hybrid」+「第三方组件深度集成」 |
| UI 框架 | Naive UI（NSelect/NButton/NDataTable/NAlert/NTag/NEmpty/NProgress/NDescriptions）+ echarts（折线图，connectNulls=false） | 栈版本表 + idea.md §5.3 |
| 状态管理 | Pinia：monitorStore（latestSnapshot/trendData/trendParams/systemInfo/pollTimers）；枚举常量 store（与 account-auth 共用 enums.ts） | 栈版本表 |
| 错误处理 | 统一拦截器按 api 3.4 code 分流：AUTH_001/002→跳登录（复用全局拦截器）、MON_001/002/003→「查询参数非法」兜底提示（正常 UI 由枚举选择器约束不会触发）、MON_004→空态「监控数据初始化中」内联提示（轮询不停止）；网络失败→保留上次数据 + 「连接中断」角标，恢复后自动续更 | api 3.4 + D-08 |

## 第 2 节 Action 定义（与 API 一一对应）

| Action | 方法 | 路径 | 页面 | 权限 | CSRF |
|--------|------|------|------|------|------|
| metricsSnapshot | GET | /monitoring/metrics/snapshot | monitoring（快照 Card + TOP 进程 Card，processes 同源同响应） | admin+readonly | —（GET） |
| metricsHistory | GET | /monitoring/metrics/history | history（趋势 Card） | admin+readonly | —（GET） |
| processesTop | GET | /monitoring/processes/top | （独立契约保留，监控概览页默认用快照内 processes；panel-core 仪表盘单取时经此 Action） | admin+readonly | —（GET） |
| systemInfo | GET | /monitoring/system/info | monitoring·系统信息 Card | admin+readonly | —（GET） |

> S7 一致性：4 Action 与 api-monitoring.md 4 接口一一对应（一接口一 Action，无未覆盖接口、无虚构接口）。本模块无写接口，全部 Action 为 GET 只读；`processesTop` 在监控概览页默认不单独调用（api 4.1.4：快照响应已含 processes，一次轮询取全避免错拍），该 Action 作为独立契约保留（api 保留理由原文）——前端 Action 注册表登记但不入默认轮询序列，供仪表盘/后续页面单取。

**轮询纪律**（确认项 2/7 交叉，api D-01 契约的前端落法）：

| Action | 轮询策略 |
|--------|---------|
| metricsSnapshot | 3s 固定间隔（=采样周期 ⚠️A12，D-01）；`document.hidden` 时暂停、恢复可见立即拉取一次；页面卸载清理定时器 |
| metricsHistory | **无自动轮询**——仅在进入页面/切换窗口/维度/口径/点击「刷新」时拉取（D-01 原文「趋势仅在切换窗口/维度时拉取一次」）；手动刷新按钮为唯一主动重拉入口 |
| systemInfo | 进入页面拉取一次 + 60s 慢轮询（运行时长与准静态信息的合理刷新粒度；前端节流决策，不违反 D-01——D-01 仅约束快照/趋势） |
| processesTop | 不轮询（同源快照；见上） |

## 第 3 节 字典 Key 清单

| Key | 取值（=01-solution/04-business-enum.md 定稿） | 用途 |
|-----|--------------------------------------------|------|
| metric_window | last_1h / last_24h | 趋势窗口下拉（默认 last_1h） |
| metric_dimension | cpu / memory / disk / network / load | 趋势维度下拉（默认 cpu） |
| metric_scope | total / per_core | CPU 口径下拉（默认 total；仅 dimension=cpu 时渲染，D-04 条件显隐） |
| panel_role | admin / readonly | 菜单可见性判定（authStore 复用；本模块两角色无视图差异） |

**数据源规范偏差声明**（同 account-auth/panel-core 先例）：不实现 `POST /{domain}/dict/batch`（分册 3.6）——枚举全部固定值，字典由前端 TS 常量提供（enums.ts 单一事实源，共用同一文件）。此偏差为设计决策，待 Gate 1 复核。

**状态颜色方案**：本模块**无状态字段**——metric/system_info 均为数值/文本投影，无状态机无生命周期（api 4.1.2/4.2.2），无 colorTag 需求。partial=true（趋势窗口存在缺失）用 NTag orange「窗口内存在缺失时段」表达（唯一 tag，语义=数据完整性提示而非业务状态）。数值严重程度着色（如 CPU 高占用红色）**不做**——材料与 API 均未定义阈值语义，不虚构告警口径（PRD OUT-2：告警为非目标）。

**度量单位格式化**（api D-02 的前端落法）：bps→自适应 B/s、KB/s、MB/s（1024 进制，1 位小数）；bytes→自适应 B/MB/GB/TiB 同法；percent→原值 +「%」；uptimeSeconds→「N 天 N 小时 N 分钟」（≥2 级粒度）；时间戳 ts→本地时区展示（接口原值 ISO-8601 UTC 不改写）。

## 第 4 节 列表页

本模块**无台账列表页**：metric 为时序观测数据（环形缓冲非持久化实体，api 1.2/4.1.1 零持久化），无 CRUD 台账场景——诚实对应接口，不虚构列表。交互确认第 4 项 operList 判定为 **N/A**：本模块无任何写接口（api 全部 GET），无操作按钮、无 operList 语义；TOP 进程「仅观测无操作入口」（api I-06/PRD OUT-3）——进程行无操作列（负空间声明：kill/优先级等操作不在产品范围）。

## 第 5 节 表单页

本模块**无表单页**：无 save/update 类接口（api 全册仅 4 个 GET），无任何表单语义——交互确认第 5 项判定为 N/A。趋势查询参数（window/dimension/scope）为**查询条件**而非表单字段（无提交、无持久化、切换即查），定义见第 6 节 6.2 控件条。

## 第 6 节 详情页（展示页，hybrid）

### 6.1 监控概览（monitoring）— 路由 /monitoring

**权限**：admin+readonly 完全一致（ADR-018 全可看；api 3.3 权限矩阵无 ✗ 项，两角色可见集相同）。

**页面级横幅**（每次 metricsSnapshot 刷新后计算）：

| 条件 | 展示 |
|------|------|
| snapshot.degraded=true | NAlert type=info「本轮部分指标采集失败，缺失项暂不显示，不影响其他指标」（api D-08：不虚构数据） |
| snapshot.ts 滞后 > 6s（2×采样周期）或网络失败 | 顶角 NTag「数据滞后/连接中断」(orange)，恢复后自动消失（轮询不停，api D-01） |
| AUTH_002 | 全局拦截器跳登录（非本页处理） |

#### Card A 系统信息（systemInfo）

| 元素 | 组件 | 说明 |
|------|------|------|
| 发行版 | NDescriptions 只读项 | data.distro（本地原样展示） |
| 内核版本 | NDescriptions 只读项 | data.kernel |
| 主机名 | NDescriptions 只读项 | data.hostname |
| 运行时长 | NDescriptions 只读项 | data.uptimeSeconds 按 D-02 格式化（「N 天 N 小时」） |
| 部分失败提示 | 逐项「暂不可用」灰字 | data.errors[] 存在时：对应字段显示「暂不可用」，NDescriptions 下方内联列出失败原因（api D-08：errors 置空标注，不虚构默认值） |

60s 慢轮询（第 2 节纪律）；无操作按钮。

#### Card B 实时快照（metricsSnapshot，3s 轮询）

| 区块 | 组件 | 数据映射（api snapshot 响应） |
|------|------|--------------------------|
| CPU | 大数字（totalPercent +「%」）+ 分核横向迷你条组（NProgress perCore[]，每核一条） | cpu.totalPercent / cpu.perCore[] |
| 内存 | NProgress 环形（usedPercent）+ 已用/总量文本（bytes 格式化） | memory.usedPercent / usedBytes / totalBytes |
| 磁盘容量 | 每挂载点一行：mount 名 + NProgress（used/total）+ 用量文本 | disk.capacity[]{mount,totalBytes,usedBytes} |
| 磁盘 IO | 读/写速率数字（bps 格式化，↑read ↓write 双色） | disk.ioReadBps / ioWriteBps |
| 网络 | 上行/下行速率数字（↑up ↓down 双色，bps 格式化） | net.upBps / downBps |
| 负载 | 三个数字（1m/5m/15m） | load.l1 / l5 / l15 |

每次刷新整卡替换数据（无增量动画——3s 节拍下数字直更即为「实时感」，echarts 不用于快照区避免重绘开销）。

#### Card C TOP 进程（快照同源 processes[]）

| 列 | 类型 | 说明 |
|----|------|------|
| PID | text | processes[].pid |
| 进程名 | text | processes[].name（超长截断 + tooltip，截断长度前端样式决策） |
| CPU % | amount 样式（右对齐，无 currency） | cpuPercent（降序展示，api I-06 已排序） |
| 内存 | 右对齐 | memBytes（bytes 格式化） |

NDataTable ≤10 行、无分页、无排序控件（服务端已排序）、**无操作列**（第 4 节负空间声明）；数据随 Card B 同一 metricsSnapshot 响应刷新（不同步二次请求）。

### 6.2 历史趋势（history）— 路由 /monitoring/history

**权限**：admin+readonly 完全一致（同上）。

**控件条**（查询条件，确认项 2 的落位；参数变更即触发 metricsHistory 拉取）：

| 控件 | 组件 | 取值与校验 | 显隐规则 |
|------|------|-----------|---------|
| 时间窗口 | NSelect | metric_window：近 1 小时（last_1h）/ 近 24 小时（last_24h）；默认 last_1h | 常显 |
| 指标维度 | NSelect | metric_dimension：CPU（cpu）/ 内存（memory）/ 磁盘（disk）/ 网络（network）/ 负载（load）；默认 cpu | 常显 |
| 统计口径 | NSelect | metric_scope：总体（total，默认）/ 分核（per_core） | **仅 dimension=cpu 时渲染**（D-04 条件显隐；切换维度离开 cpu 时 scope 重置为 total） |
| 刷新 | NButton | 以当前参数重拉一次（loading 防抖） | 常显 |

**趋势图**（echarts 折线）：

| 项 | 说明 |
|----|------|
| 数据源 | metricsHistory data.series[]——每条 series 一个折线（legend 多序列：cpu+per_core 为每核一条、disk 为每挂载点容量 + IO 读写、network 上下行、load 三线，api D-03 统一形状） |
| y 轴单位 | 按 series.unit 自适应标签（percent=%、bytes=字节自适应、bps=速率自适应、value=原值）；多序列单位同源一致（同一 dimension 内 unit 一致，api series 契约） |
| 缺失表达 | v=null 点 → 线条断开（connectNulls=false）——「断开=数据缺失」的诚实呈现（D-03/ADR-014；不做插值补线） |
| 完整性提示 | partial=true → 图右上 NTag「窗口内存在缺失时段」(orange)；startTime 非空 → 图下方灰字「数据自 {startTime 本地时区} 起（重启后历史清零，缓冲保留 24 小时）」（ADR-014 边界明示，FL S-15-01 功能点要求） |
| 空态 | series=[] 或 startTime=null → echarts 空图 + NEmpty 居中「重启后暂无历史数据」（MON_004 语义同空态处理，轮询语义不适用——趋势按需拉取） |
| 加载态 | 拉取中图表骨架/loading 遮罩；错误 → MON_001/002/003 兜底提示（正常 UI 不可达：选择器约束取值，R-14 服务端兜底） |

确认项 7（页面类型覆盖）：本页为 hybrid 的图表集成形态（echarts），无需 code 型覆盖（无拖拽/画布/实时试算）。

## 第 7 节 路由定义与框架行为

| 路由 | 页面 | meta |
|------|------|------|
| /monitoring | monitoring（监控概览） | `{requiresAuth:true}` |
| /monitoring/history | history（历史趋势） | `{requiresAuth:true}` |

**守卫**：复用全局守卫（initCheck → sessionCurrent → roles）；本模块无新增守卫（全部接口要求会话，未登录由全局守卫拦截）。

**框架级边界**（复用 panel-core 框架行为，见 ../admin/panel-core/panel-core.md 第 7 节）：菜单渲染/安全入口前缀透明/顶栏均为 panel-core 宿主职责；本模块 2 个路由经模块页面挂载点接入（panel-core S-10），菜单项由 ModuleMenuRegistry 登记、服务端按角色下发（monitoring 菜单对 admin/readonly 同样可见——ADR-018 全可看）。页面挂载与导航位置不在本 PageSpec 展开。

## 第 8 节 全局一致性

| # | 检查 | 结果 |
|---|------|------|
| 1 | Action ↔ api-monitoring.md 接口一一对应（S7） | ✅ 4/4 接口全映射，无虚构；processesTop 默认不轮询的契约依据已注记（api 4.1.4） |
| 2 | 字典 Key ↔ 04-business-enum.md 定稿枚举一一对应（S8） | ✅ 3/3（metric_window/metric_dimension/metric_scope）+ panel_role 复用；取值逐字一致 |
| 3 | 全部 Action 为 GET 只读、无 CSRF 注入需求 | ✅ 第 2 节（无任何写接口） |
| 4 | ADR-018：readonly 视图与本视图完全一致（api 3.3 无 ✗ 项，无负空间 Action、无禁用控件差异） | ✅ readonly 文档全文一致声明 |
| 5 | MON_001~004 + AUTH_001/002 错误文案与 api 3.4 一一对应 | ✅ 第 1 节错误处理行 + 6.2 空态 |
| 6 | 轮询纪律符合 D-01：仅快照 3s 轮询、趋势按需拉取、隐藏页暂停、卸载清理 | ✅ 第 2 节纪律表 |
| 7 | 清零边界呈现完整：startTime 起点文案 + 空态 NEmpty（ADR-014/FL S-15-01） | ✅ 6.2 |
| 8 | 不虚构数据与阈值：degraded/errors 诚实呈现、null 断线不插值、无高占用着色/告警语义（PRD OUT-2） | ✅ 横幅/6.2/第 3 节 |
| 9 | 字典数据源偏差（无 dict/batch，前端常量 enums.ts）已声明 | ✅ 第 3 节 |
| 10 | 无 📎 Scm* 组件；echarts 为 idea.md §5.3 材料给定选型 | ✅ 头部组件体系 + 6.2 |
| 11 | 无列表页/无表单/无 operList 的诚实性声明（工具类只读观测，零持久化） | ✅ 第 4/5 节 |
| 12 | 时间本地时区展示、ISO UTC 原值不改写；度量单位按 D-02 前端格式化 | ✅ 第 3 节格式化行 + 6.2 |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-12 | initial | 首次生成：2 页面（监控概览 3 Card + 历史趋势 1 Card 控件条）、4 Action、3 字典 Key + panel_role 复用、0 列表页/0 表单页/2 hybrid 展示页；轮询纪律（快照 3s/趋势按需/系统信息 60s）与清零边界呈现（startTime+空态）为本阶段核心交互定义 |

> FX 质量清单说明：FX-001~FX-141 逐条清单文件未随引擎 0.5.2 payload 发布（panel-core 先例同口径复核），本节按分册 9 节结构逐节落实，并以第 8 节 12 项全局一致性自查替代可机械执行部分；全量 141 项留待 Gate 1 validation 执行。
