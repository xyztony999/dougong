# PageSpec · systemd 服务管理（service-mgmt） · 管理员（admin）

| 属性 | 值 |
|------|-----|
| 版本 | v1.0 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-12 |
| 用户类型 | admin（readonly 视角为本文档子集，差异见 ../readonly/service-mgmt/service-mgmt.md） |
| 唯一数据源 | 02-detailed/api/api-service-mgmt.md v1.0 |
| 组件体系 | Vue 3 + Naive UI（NDataTable/NSelect/NInput/NButton/NSwitch/NTag/NPopconfirm/NDrawer/NModal/NAlert/NEmpty/NDescriptions；无激活栈 profile，不使用 vue3-engine 栈 Scm* 组件） |
| 确认方式 | headless run：7 项交互确认依据 D3 契约（api-service-mgmt.md D-01~D-09）+ ADR-018 + ⚠️A14 + FL v1.0 推导，标注于各节 |

## 第 1 节 模块元信息

| 项 | 值 | 确认依据 |
|----|-----|---------|
| 模块编码 | service-mgmt（域前缀 /services） | api 文档 3.1 |
| 业务类型 | **工具类**（分册 2.1 三型之外的诚实标注：无聚合根、无 MQ——D2/D3 判定；有投影列表 + 5 动作接口，状态机为 systemd 侧投影非页面状态机） | api 1.4/4.1.2；domain-model.md service-mgmt 域 |
| 页面类型 | **config**（标准列表页：无 Tab 切换、无左树右表、详情单区块 Drawer、无图表/拖拽集成） | 分册 2.2 默认 config；不满足 hybrid/code 任一触发条件 |
| UI 框架 | Naive UI（NDataTable/NSelect multiple/NInput/NButton/NSwitch/NTag/NPopconfirm/NDrawer） | 栈版本表 |
| 状态管理 | Pinia：serviceStore（units/total/generatedAt/listParams/unitDetail/inFlightSet Drawer 态）；枚举常量 store（与 account-auth/monitoring 共用 enums.ts） | 栈版本表 |
| 错误处理 | 统一拦截器按 api 3.4 code 分流：AUTH_001/002→跳登录（全局拦截器）、AUTH_003→CSRF 失败提示（全局）、AUTH_008→「权限不足：只读角色禁止服务操作」（全局边界拒绝，正常 UI 无入口不触发）、SVC_001/002/004/005→「参数非法」兜底提示（正常 UI 由控件与 canOperate/canToggleAutostart 权威值约束不可达，api D-05/R-14）、SVC_003→「该服务已不存在（可能已被卸载）」warning + 自动刷新列表、SVC_006→warning「操作仍在执行，请稍后刷新列表确认终态」、SVC_007→结果 Dialog（失败原因 + 实际终态）、SVC_008→页面级错误横幅 + 重试（不显示过期数据，api I-01） | api 3.4 + D-01/D-05/I-01 |

## 第 2 节 Action 定义（与 API 一一对应）

| Action | 方法 | 路径 | 页面 | 权限 | CSRF |
|--------|------|------|------|------|------|
| unitList | GET | /services/unit/list | services（列表主数据） | admin+readonly | —（GET） |
| unitDetail | GET | /services/unit/detail | services·属性 Drawer | admin+readonly | —（GET） |
| unitStart | POST | /services/unit/start | services·操作列 | admin | ✓ |
| unitStop | POST | /services/unit/stop | services·操作列 | admin | ✓ |
| unitRestart | POST | /services/unit/restart | services·操作列 | admin | ✓ |
| unitEnable | POST | /services/unit/enable | services·自启列开关 | admin | ✓ |
| unitDisable | POST | /services/unit/disable | services·自启列开关 | admin | ✓ |

> S7 一致性：7 Action 与 api-service-mgmt.md 7 接口一一对应（一接口一 Action，无未覆盖接口、无虚构接口）。CSRF Token 由全局拦截器统一注入 `X-CSRF-Token`（值来自登录响应 csrfToken，account-auth R-02 机制）。

**刷新纪律**（确认项 2/7 交叉，api D-03「按需拉取」契约的前端落法）：

| 触发 | 行为 |
|------|------|
| 进入页面 / 点击「刷新」按钮 | unitList 重拉（loading 态） |
| 动作接口成功返回 | **不重拉列表**——以响应内操作后快照直更该行字段（api 4.1.4：响应含 activeState/subState/enabledState，免二次查询）；generatedAt 同步更新为响应时刻 |
| 无自动轮询 | 列表页不设定时轮询（D-03：无服务端推送/长轮询；页面刷新节奏=手动+操作驱动；自动刷新开关不在产品范围，PRD 无此需求不虚构） |
| `document.hidden` | 无定时器需要暂停（本无轮询）；恢复可见不自动重拉（保持按需纪律） |
| 页面卸载 | 清理 Drawer/inFlight 状态 |

## 第 3 节 字典 Key 清单

| Key | 取值（=01-solution/04-business-enum.md 定稿） | 用途 |
|-----|--------------------------------------------|------|
| service_active_state | active / inactive / failed | 状态过滤下拉（多选）+ 运行状态列 dict + colorTag |
| service_enabled_state | enabled / disabled | 自启状态列 dict 主轴（⚠️A14：响应可能透传非两态值——dict 匹配不到时降级为原值灰字 Tag，见第 4 节列定义） |
| unit_type | service / socket / timer / mount / path / target / automount / slice / swap / scope / device | 类型列 dict |
| panel_role | admin / readonly | 菜单可见性判定（authStore 复用；本模块 readonly 为子集视角，差异见 readonly 文档） |

**数据源规范偏差声明**（同 account-auth/panel-core/monitoring 先例）：不实现 `POST /{domain}/dict/batch`（分册 3.6）——枚举全部固定值，字典由前端 TS 常量提供（enums.ts 单一事实源，共用同一文件）。此偏差为设计决策，待 Gate 1 复核。

**状态颜色方案**（分册状态颜色语义映射）：

| 值 | 颜色 | 语义依据 |
|----|------|---------|
| activeState=active | green | 运行中（健康） |
| activeState=inactive | gray | 已停止（分册「已关闭」语义） |
| activeState=failed | red | 异常（分册「已驳回/异常」语义） |
| enabledState=enabled | blue | 自启配置生效（分册「进行中」蓝=配置态激活） |
| enabledState=disabled | gray | 自启关闭 |
| enabledState=其他值（static 等） | 灰字原值（无色 Tag） | ⚠️A14：非两态只读展示（api D-05） |

## 第 4 节 列表页（核心页，/services）

**权限**：admin 全功能；readonly 为子集（无操作列、自启列无开关，见 readonly 文档）。

### 4.1 工具条（查询条件，确认项 2 的落位；参数变更即触发 unitList）

| 控件 | 组件 | 取值与校验 | 查询方式 |
|------|------|-----------|---------|
| 状态过滤 | NSelect multiple（clearable） | service_active_state 三值（运行中/已停止/失败），空=全部 | 多选值逗号拼接 query `state`（api：逗号分隔多值） |
| 名称搜索 | NInput（clearable，防抖 300ms） | 名称子串，不区分大小写 | query `search`（like，服务端执行 D-02） |
| 刷新 | NButton（loading 防抖） | 以当前参数重拉 | — |

表格上方灰字「数据生成于 {generatedAt 本地时区}」（api generatedAt 契约：数据新旧可辨识，monitoring D-08 同口径）。

### 4.2 表格列（NDataTable，确认项 3）

| 列 | 类型 | 宽度 | 说明 |
|----|------|------|------|
| 序号 | index | 60 | — |
| 名称 | link | 220 fixed:left | items[].name（unit 全名）；点击打开属性 Drawer（详情跳转经名称列，分册「详情跳转经编码列」同构；本模块无独立编码字段，名称即标识 api D-09） |
| 类型 | dict(unit_type) | 100 | items[].unitType |
| 运行状态 | dict(service_active_state) + colorTag | 110 | items[].activeState（第 3 节颜色方案） |
| 状态细分 | text | 130 | items[].subState 原值透传（api D-04：跨类型不统一，不翻译不归类） |
| 开机自启 | 开关/Tag 复合 | 130 | canToggleAutostart=true → NSwitch（checked=(enabledState=='enabled')，切换即调 unitEnable/unitDisable，loading 期间禁用）；canToggleAutostart=false → NTag（enabledState 两态走 dict 颜色，非两态灰字原值，⚠️A14/D-05：static 等只读展示无开关） |
| 操作 | action | 220 fixed:right | 见 4.3（readonly 视角整列不渲染） |

无分页（全量返回 D-02）、无排序控件（服务端名称字典序升序）、行 hover 高亮；total 展示于表格上方（「共 N 个 unit」）；空结果 → NEmpty「未匹配到服务」（搜索无结果为常态非错误，api N2）。加载中 → 表格骨架。SVC_008 → 页面级 NAlert error「systemd 服务通道不可用，无法获取服务列表」+「重试」按钮，表格区域不渲染（I-01 不显示过期数据）。

### 4.3 操作列（operList，确认项 4）

**按钮集合与可见性**（前端维护完整按钮集，可见性由后端权威布尔控制——api D-05 canOperate/canToggleAutostart + 呈现层状态细化）：

| 按钮 | 可见性条件 | 二次确认 | 依据 |
|------|-----------|---------|------|
| 启动 | canOperate ∧ activeState≠active | 无（启动无破坏性） | 呈现优化；服务端幂等兜底（api 幂等语义：active 服务 start 为 no-op） |
| 停止 | canOperate ∧ activeState=active | NPopconfirm「确认停止该服务？运行中的服务将被中断。」 | 破坏性动作确认 |
| 重启 | canOperate（三态均可发起） | NPopconfirm「确认重启该服务？服务将短暂中断。」 | 破坏性动作确认；failed 行为首要恢复入口（api 4.1.4） |
| 属性 | 恒显 | — | 打开 Drawer（不走 operList 语义，同分册「detail 不显示为按钮」→ 以名称列 link 承载；此处保留行内入口便于触达） |

自启切换不经操作列按钮——由「开机自启」列的 NSwitch 承载（4.2 列定义；开关语义即操作语义，无需确认弹层）。

**在途互斥与终态反馈**（确认项 5 交叉，api D-01/I-05/I-06 的前端落法）：

| 环节 | 行为 |
|------|------|
| 点击动作 | 行级 inFlightSet 登记该 unit——该行全部操作按钮与自启开关置 loading/禁用（防重复提交，PRD N2 前端在途态） |
| 等待 | 无超时兜底 UI（服务端 90s 上限保证必返回，api D-01/I-05）；不显示假进度 |
| 成功（code=0） | message.success「已启动/已停止/已重启」+ 该行以响应快照直更（activeState/subState/enabledState 刷新，generatedAt 更新）——**不整表重拉**（D-03） |
| SVC_007 | NModal 结果框：「操作失败」+ data.reason（失败原因）+ data.activeState（实际终态，Tag 渲染）+ 关闭按钮；行数据以响应实际终态直更 |
| SVC_006 | message.warning「操作仍在执行，请稍后刷新列表确认终态」；行 inFlight 解除（等待已达服务端上限） |
| SVC_003 | message.warning「该服务已不存在（可能已被卸载）」+ 自动重拉列表（集合实况同步，api I-03） |
| enable/disable 成功 | message + 该行 enabledState 直更（响应刷新值）；NSwitch 回弹到响应值（不信任本地乐观更新，api D-05 透真） |

> 显隐条件中的 activeState 判断为呈现层优化（启动按钮对 active 服务无意义）；服务端不做状态前置校验（动作幂等安全，api 4.1.4 幂等语义）——前端显隐与服务端校验职责分离（R-14：服务端校验类型/存在性/两态主轴，前端只做体验约束）。

## 第 5 节 表单页

本模块**无表单页**：无 save/update 类接口（api 全册仅 2 GET + 5 动作 POST，无数据创建/编辑语义——零持久化 D-07，「发现即接管」无新增服务入口）——交互确认第 5 项判定为 **N/A**。状态过滤与名称搜索为查询条件（第 4.1 节），非表单字段；enable/disable 为动作接口（第 4.3/4.2 节），非表单提交。

## 第 6 节 详情页（属性 Drawer，确认项 6）

**形态依据**：unit 属性共 9 字段（api detail 响应），单区块、无子表、无审批记录——独立详情页过重；运维高频场景需不离开列表上下文（查看属性→紧接操作），故取 **NDrawer（右侧抽屉，宽 480px）** 而非独立路由页。打开方式：名称列 link 或「属性」行内按钮 → unitDetail（query name，loading 态）。

| 字段 | 组件 | 说明 |
|------|------|------|
| 名称 | NDescriptions 只读项 | data.name（unit 全名） |
| 类型 | NDescriptions + NTag | data.unitType（dict） |
| 运行状态 | NDescriptions + NTag(colorTag) | data.activeState（第 3 节颜色方案） |
| 状态细分 | NDescriptions 只读项 | data.subState 原值（D-04） |
| 开机自启 | NDescriptions + NTag | data.enabledState（两态 dict 颜色；非两态灰字原值 ⚠️A14——Drawer 内只读展示，不提供开关） |
| 描述 | NDescriptions 只读项 | data.description（null → 「—」） |
| 加载状态 | NDescriptions 只读项 | data.loadState 原值透传（null → 「—」） |

Drawer 内**无操作按钮**（操作集中在列表操作列，避免双入口状态同步复杂度）；数据为打开时刻快照（不自动刷新，Drawer 头部提供「刷新」图标按钮重拉 unitDetail）；SVC_003 → Drawer 内 NEmpty「该服务已不存在」；SVC_008 → NAlert 错误。无子表 Card、无审批记录 Card、无操作日志 Card（操作历史经 account-auth 审计日志查询，PRD N4）。

## 第 7 节 路由定义与框架行为

| 路由 | 页面 | meta |
|------|------|------|
| /services | services（服务管理列表页，含属性 Drawer） | `{requiresAuth:true}` |

**守卫**：复用全局守卫（initCheck → sessionCurrent → roles）；本模块无新增守卫。

**框架级边界**（复用 panel-core 框架行为，见 ../admin/panel-core/panel-core.md 第 7 节）：菜单渲染/安全入口前缀透明/顶栏均为 panel-core 宿主职责；本模块 1 个路由经模块页面挂载点接入（panel-core S-10），菜单项由 ModuleMenuRegistry 登记、服务端按角色下发（服务管理菜单对 admin/readonly 均可见——readonly 可进入查看，操作面为其不可见，FL 附录A/ADR-018）。

## 第 8 节 全局一致性

| # | 检查 | 结果 |
|---|------|------|
| 1 | Action ↔ api-service-mgmt.md 接口一一对应（S7） | ✅ 7/7 接口全映射，无虚构、无遗漏 |
| 2 | 字典 Key ↔ 04-business-enum.md 定稿枚举一一对应（S8） | ✅ 3/3（service_active_state/service_enabled_state/unit_type）+ panel_role 复用；取值逐字一致；非两态透传降级呈现已声明（⚠️A14/D-05） |
| 3 | 写 Action CSRF 注入完整（5 POST 全部 ✓） | ✅ 第 2 节 + 全局拦截器 |
| 4 | 操作列可见性权威来源=服务端 canOperate/canToggleAutostart，activeState 显隐仅为呈现优化且声明职责分离 | ✅ 第 4.3 节 |
| 5 | 等待终态反馈完整：成功快照直更 / SVC_007 结果框 / SVC_006 超时提示 / 无假进度（D-01/I-05） | ✅ 第 4.3 节 |
| 6 | 刷新纪律符合 D-03：手动+操作驱动、无自动轮询、动作成功不整表重拉 | ✅ 第 2 节纪律表 |
| 7 | SVC_001~008 + AUTH_001/002/003/008 错误文案与 api 3.4 一一对应 | ✅ 第 1 节错误处理 + 4.2/4.3/第 6 节 |
| 8 | ⚠️A14 落法：非两态自启值灰字原值、无开关、Drawer 只读 | ✅ 第 3/4.2/6 节 |
| 9 | ADR-018：readonly 为子集视角（操作列整列不渲染、自启无开关），5 个 ✗ 项逐项负空间 | ✅ readonly 文档对应 |
| 10 | 无 📎 Scm* 组件；Naive UI 为栈版本表选型 | ✅ 头部组件体系 |
| 11 | 无表单页诚实性声明（零持久化无新增/编辑语义，「发现即接管」） | ✅ 第 5 节 |
| 12 | 时间本地时区展示（generatedAt）、枚举原值透传不翻译（subState/loadState/enabledState 非两态） | ✅ 第 4.1/4.2 节 |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-12 | initial | 首次生成：1 页面（服务管理列表页 + 属性 Drawer）、7 Action（2 读 + 5 写）、3 字典 Key + panel_role 复用、1 列表页/0 表单页/1 Drawer 详情；操作终态反馈（快照直更/结果框/超时提示）、自启 NSwitch 与非两态降级（⚠️A14）、刷新纪律（手动+操作驱动无轮询）为本阶段核心交互定义 |

> FX 质量清单说明：FX-001~FX-141 逐条清单文件未随引擎 0.5.2 payload 发布（monitoring/panel-core/account-auth 先例同口径复核），本节按分册 9 节结构逐节落实，并以第 8 节 12 项全局一致性自查替代可机械执行部分；全量 141 项留待 Gate 1 validation 执行。
