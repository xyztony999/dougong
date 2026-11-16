# PageSpec · Web 终端（terminal） · 管理员（admin）

| 属性 | 值 |
|------|-----|
| 版本 | v1.0 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-14 |
| 用户类型 | admin（本模块 readonly 为**零可达整域禁用**，差异见 ../readonly/terminal/terminal.md——全项目唯一 readonly 无任何功能模块） |
| 唯一数据源 | 02-detailed/api/api-terminal.md v1.0 |
| 组件体系 | Vue 3 + Naive UI（NTabs/NModal/NRadioGroup/NSelect/NButton/NTag/NAlert）+ xterm.js 终端组件（含尺寸自适应 addon，idea.md §5.3 前端关键组件）；无激活栈 profile，不使用 vue3-engine 栈 Scm* 组件 |
| 确认方式 | headless run：7 项交互确认依据 D3 契约（api-terminal.md D-01~D-09/I-01~I-07）+ ADR-018 + ADR-023 + D1 落定口径（多标签上限=并发阈值）+ FL v1.0 推导，标注于各节 |

## 第 1 节 模块元信息

| 项 | 值 | 确认依据 |
|----|-----|---------|
| 模块编码 | terminal（域前缀 /terminal） | api 3.1 |
| 业务类型 | **工具类·会话通道**（分册 2.1 三型之外的诚实标注：内存态会话聚合无 CRUD 列表/无 MQ/无审批——D2/D3 判定「主数据类·内存态会话变体」；页面侧为交互通道 + 建立对话框，无单据/无状态机表单） | api 4.1 开注 / domain-model.md terminal 域 |
| 页面类型 | **code**（第三方组件深度集成：xterm.js 终端模拟器承载全部交互，WS 双向帧流——分册 2.2 code 触发条件第三款） | api D-01 帧协议 / idea.md §5.3 |
| UI 框架 | xterm.js（终端渲染 + 尺寸自适应 addon）+ Naive UI 辅助（NTabs/NModal/NRadioGroup/NSelect/NButton/NTag/NAlert） | 栈版本表 |
| 状态管理 | Pinia：terminalStore（tabs[]（sessionId/runUser/limits/status）/userOptions/connecting 标志）；枚举常量 store（与 account-auth/monitoring 共用 enums.ts） | 栈版本表 |
| 错误处理 | 统一拦截器按 api 3.4 code 分流：AUTH_001/002→跳登录（全局）、AUTH_003→「终端连接被拒绝：非同源请求」、AUTH_008→「权限不足：只读角色禁止使用 Web 终端」（正常 admin UI 不可达）、TERM_001→warning「已达并发上限（{current}/{limit}），请先关闭其他终端会话」、TERM_002/003→「指定的系统用户不存在 / 无可登录 shell」+ 建议刷新候选、TERM_004→error「终端进程创建失败，请稍后重试」、TERM_005→对话框字段级校验提示、TERM_006→error「审计服务暂不可用，会话未建立」、TERM_007→候选清单加载失败横幅 + 重试（不显示虚构数据） | api 3.4 + I-01 |

## 第 2 节 Action 定义（与 API 一一对应）

| Action | 方法 | 路径 | 页面 | 权限 | CSRF |
|--------|------|------|------|------|------|
| userOptions | GET | /terminal/user/options | terminal·新建会话对话框 | admin | —（GET） |
| sessionConnect | WS（GET+Upgrade） | /terminal/session/connect | terminal·标签页 | admin | 豁免（api D-09：握手 Cookie 会话鉴权 + Origin 同源校验替代，帧层无 CSRF 面） |

> S7 一致性：2 Action 与 api-terminal.md 2 端点一一对应（一端点一 Action，无未覆盖、无虚构）。WS 帧协议（ready/input/resize/output/closed，api D-01）为 sessionConnect Action 的消息层，不单列 Action。

**刷新纪律**（确认项 2/7 交叉，api D-02/D-07/D-03 契约的前端落法）：

| 触发 | 行为 |
|------|------|
| 打开「新建会话」对话框 | userOptions 重拉（loading 态；实况直读 D-02，不缓存——每次打开均取最新系统用户清单） |
| userOptions 拉取失败（TERM_007） | 对话框内 NAlert error + 「重试」；「指定系统用户」不可用，「默认身份」仍可连接 |
| 会话数据 | **无轮询**——output 帧为服务端实时推送（WS 通道本身即数据流，api D-01）；无任何 HTTP 轮询面 |
| 会话元信息 | ready 帧到达即写入 tab（sessionId/runUser/limits，连接后首帧，api 4.1.4） |
| 断开 | closed 帧到达 → tab 状态转 closed + 断开反馈（第 6 节）；不自动重连（重连=用户显式「重新连接」新建会话，api 4.1.2 辨析③） |
| 页面卸载/路由离开 | 全部活跃 WS 发送 close（尽力而为；孤儿由服务端心跳/空闲兜底回收，api D-03/I-02，前端不承担一致性责任） |

## 第 3 节 字典 Key 清单

| Key | 取值（=01-solution/04-business-enum.md 定稿） | 用途 |
|-----|--------------------------------------------|------|
| terminal_end_reason | self_close / idle_timeout / hard_limit / panel_session_invalid / network_error / protocol_error | closed 帧断开反馈文案 + 颜色（第 6 节；protocol_error 为 D3 增补取值） |
| terminal_identity_source | panel_run_user / specified_system_user | 新建会话对话框身份来源 radio（api R-18 两分支） |
| panel_role | admin / readonly | 菜单可见性判定（authStore 复用；本模块菜单仅 admin 可见——readonly 零可达，见 readonly 文档） |

**数据源规范偏差声明**（同 account-auth/panel-core/monitoring/service-mgmt 先例）：不实现 `POST /{domain}/dict/batch`（分册 3.6）——枚举全部固定值，字典由前端 TS 常量提供（enums.ts 单一事实源，共用同一文件）。此偏差为设计决策，待 Gate 1 复核。

**断开原因颜色与文案方案**（分册状态颜色语义映射）：

| 值 | 颜色 | 文案 |
|----|------|------|
| self_close | gray | 会话已关闭 |
| idle_timeout | orange | 会话因 30 分钟无操作已自动断开（服务端持续输出不重置计时） |
| hard_limit | orange | 会话已达最长 8 小时上限被断开，如需继续请新建会话 |
| panel_session_invalid | blue | 面板登录会话已失效，终端已断开——请重新登录（附跳登录引导） |
| network_error | red | 连接中断（网络异常或浏览器休眠） |
| protocol_error | red | 连接因协议异常被断开 |

## 第 4 节 终端页（核心页，/terminal，code 型）

**权限**：admin 专属（readonly 菜单不下发、路由守卫拒绝，见 readonly 文档）。

**确认项 2/3 判定**：本模块**无列表页、无查询条件、无表格列**（api 全册仅 1 HTTP GET（候选清单）+ 1 WS 端点，无集合查询语义——PRD OUT-5 会话清单不做）——交互确认第 2、3 项判定为 **N/A**，本节按 code 型页面布局定义。

### 4.1 页面布局（三区）

| 区 | 组件 | 说明 |
|----|------|------|
| 标签栏 | NTabs（type: card，closable）+ 尾部「+」NButton | 每标签=一条终端会话（tab 标题=ready 帧 runUser，如 `root` / `www-data`；连接中态标题=`连接中…`）；「+」达上限（活跃标签数=limits.maxConcurrent=5）时禁用 + tooltip「已达并发上限 5，请先关闭其他会话」（ADR-023/D1 落定：多标签上限=并发阈值） |
| 终端主体 | xterm.js 容器（每标签一实例，keep-alive 保活，仅激活标签可见） | output 帧 Base64 解码后 write；光标主题/字号随全局设置（不虚构未定义设置面，固定默认值） |
| 状态条（底部） | 灰字文本 + NTag | 当前激活会话：`{runUser} · 会话 {sessionId 前 8 位} · 空闲上限 30min · 最长 8h`（ready 帧 limits 权威值）+ 连接状态 Tag（connected=green / closed=gray） |

### 4.2 交互通道行为（api D-01/I-06 前端落法）

| 交互 | 行为 |
|------|------|
| 键入 | xterm onData → `input` 帧（data=Base64）；无需回车确认（流式） |
| 窗口尺寸变化 | xterm 尺寸自适应 addon onResize → `resize` 帧（cols/rows）；两帧均更新会话活跃（服务端 lastActiveAt，api I-03 从严口径前端无感知） |
| 输出渲染 | `output` 帧 → Base64 解码 → xterm write；`truncated:true` 帧 → 终端内黄色提示行「⚠ 输出缓冲溢出，部分历史输出已被丢弃」（api D-04 背压标记） |
| 帧上限 | 单帧解码后 ≤16KB 为服务端校验（I-06）；前端不构造超限帧（键入流式天然细粒度） |

### 4.3 标签操作（operList 语义的本模块等价物，确认项 4）

无列表 operList（无表格）；操作集承载于标签：

| 操作 | 可见性 | 确认 | 依据 |
|------|--------|------|------|
| 新建会话（「+」） | 活跃标签数 <5 | —（打开对话框，第 5 节） | ADR-023 |
| 关闭标签（×） | 恒显 | 无（关闭=断开会话；WS close→服务端 self_close 清理，api 4.1.4；closed 标签关闭为纯前端移除） | api 4.1.2 |
| 重新连接（closed 标签内） | 仅 closed 态标签 | —（重走新建流程=新会话，不继承任何状态） | api 4.1.2 辨析③ |

## 第 5 节 新建会话对话框（表单，确认项 5）

**表单类型判定**：2 字段 <5 → **NModal Dialog**（分册 3.2 表单类型规则）；无独立表单路由页。

| 字段 | 组件 | 取值与校验 | 条件显隐 |
|------|------|-----------|---------|
| 身份来源 | NRadioGroup | terminal_identity_source 两值：默认身份（面板运行用户，通常 root）/ 指定系统用户；默认=panel_run_user | 恒显 |
| 系统用户 | NSelect（filterable，clearable） | 选项=userOptions items（label=name，辅显 uid/shell）；服务端已滤非交互 shell（api D-02） | 仅 specified_system_user 时显示；未选择时「连接」禁用 |

**按钮**：连接（发起 WS sessionConnect，query identitySource+username）/ 取消。无保存/提交语义（无持久化表单——api D-08 无数据 CRUD 侧）。

**连接流程与逐级反馈**（api 4.1.4 握手校验顺序的前端落法）：

| 环节 | 行为 |
|------|------|
| 点击「连接」 | 对话框关闭 → 新建标签（标题「连接中…」，xterm 容器就绪后发起 WS） |
| 升级成功 + ready 帧 | 标题定稿为 runUser、状态条填充 sessionId/limits、开始帧交互 |
| AUTH_001/002 | 全局跳登录（面板会话失效场景与 closed 帧 panel_session_invalid 路径互补） |
| TERM_001 | message.warning（文案见第 1 节）+ 移除「连接中…」标签 + 「+」按钮刷新禁用态 |
| TERM_002/003 | message + 自动重拉 userOptions（实况可能变化）+ 保留对话框上下文重开 |
| TERM_004/006 | message.error + 移除连接中标签（不占并发配额，api I-02 对称回收） |
| TERM_005 | 前端控件已约束（radio 枚举 + select 白名单）；服务端校验为兜底（R-14 职责分离） |

## 第 6 节 会话运行反馈（详情页语义的等价物，确认项 6）

**判定**：无独立详情页/Drawer——会话元信息经 ready 帧展示于标签与状态条（第 4.1 节），断开事实经 closed 帧展示于标签内反馈区（数据源=WS 帧，非 HTTP detail 接口）——交互确认第 6 项判定为 **N/A（以帧反馈等价承载）**。

**断开反馈**（closed 帧 → 标签内 NAlert，标签保留可查看尾部输出）：

| reason | 呈现 |
|--------|------|
| self_close | 灰色 Alert（第 3 节文案）；仅「关闭标签」按钮 |
| idle_timeout / hard_limit | 橙色 Alert + 「重新连接」按钮（新会话，第 4.3） |
| panel_session_invalid | 蓝色 Alert + 「重新登录」按钮（跳登录页；该操作者全部标签同步转此态——api D-07 联动全量） |
| network_error / protocol_error | 红色 Alert + 「重新连接」按钮 |

无审批记录 Card、无操作日志 Card（会话开合追溯经 account-auth 审计日志查询，PRD N4；终端 I/O 内容不留痕 OUT-2）。

## 第 7 节 路由定义与框架行为

| 路由 | 页面 | meta |
|------|------|------|
| /terminal | terminal（终端页，多标签 code 页） | `{requiresAuth:true, requiresAdmin:true}` |

**守卫**：复用全局守卫（initCheck → sessionCurrent → roles）+ 本模块 requiresAdmin（菜单与路由双拦截——terminal 为全项目唯一 admin 专属路由模块）。

**框架级边界**（复用 panel-core 框架行为，见 ../admin/panel-core/panel-core.md 第 7 节）：菜单渲染/安全入口前缀透明/顶栏均为 panel-core 宿主职责；本模块 1 个路由经模块页面挂载点接入（panel-core S-10），菜单项由 ModuleMenuRegistry 登记、服务端按角色下发——**terminal 菜单仅对 admin 下发**（ADR-018 禁 Web 终端 + FL readonly 章零菜单；对照 service-mgmt 菜单两角色可见的差异已声明）。

## 第 8 节 全局一致性

| # | 检查 | 结果 |
|---|------|------|
| 1 | Action ↔ api-terminal.md 端点一一对应（S7） | ✅ 2/2（userOptions/sessionConnect），无虚构、无遗漏；WS 帧协议为消息层不单列 |
| 2 | 字典 Key ↔ 04-business-enum.md 定稿枚举一一对应（S8） | ✅ terminal_end_reason（6 值含 D3 增补）/terminal_identity_source + panel_role 复用；取值逐字一致 |
| 3 | CSRF 落法：GET 豁免 + WS 豁免（api D-09 握手级鉴权替代，非漏注） | ✅ 第 2 节 |
| 4 | 无列表/无查询/无表格/无详情页的诚实性声明（N/A 判定逐项有据） | ✅ 第 4/6 节 |
| 5 | 多标签上限=并发阈值 5（D1 落定）+ 上限禁用反馈（TERM_001 前置） | ✅ 第 4.1/5 节 |
| 6 | 断开反馈闭环：closed 帧 6 原因全映射（颜色/文案/按钮：重连/重登/关闭）+ 不自动重连 | ✅ 第 3/6 节 |
| 7 | 错误处理与 api 3.4 一一对应（TERM_001~007 + AUTH_001/002/003/008） | ✅ 第 1 节 + 第 5 节流程表 |
| 8 | 背压标记呈现（output truncated 帧 → 终端内提示行，api D-04） | ✅ 第 4.2 节 |
| 9 | 刷新纪律：userOptions 开框即拉不缓存（D-02）、无轮询（WS 即数据流）、页面卸载尽力 close | ✅ 第 2 节 |
| 10 | ADR-018：菜单仅 admin 下发 + requiresAdmin 守卫（readonly 零可达，负空间见 readonly 文档） | ✅ 第 7 节 |
| 11 | 无 📎 Scm* 组件；xterm.js + Naive UI 为栈版本表选型 | ✅ 头部组件体系 |
| 12 | 权威值来源：limits（ready 帧）/候选清单（userOptions）/上限判定——前端不自行推断（R-14 精神） | ✅ 第 4.1/5 节 |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-14 | initial | 首次生成：1 页面（终端页 /terminal，code 型——xterm.js 深度集成）、2 Action（1 GET + 1 WS）、2 字典 Key + panel_role 复用、1 Dialog 表单（新建会话：身份来源 radio + 系统用户 select）、0 列表页/0 详情页（N/A 判定有据：帧反馈等价承载）；多标签上限=并发阈值 5、closed 帧 6 原因断开反馈（重连/重登/关闭分级）、背压丢弃提示、菜单仅 admin 下发为本阶段核心交互定义 |

> FX 质量清单说明：FX-001~FX-141 逐条清单文件未随引擎 0.5.2 payload 发布（monitoring/panel-core/account-auth/service-mgmt 先例同口径复核），本节按分册 9 节结构逐节落实，并以第 8 节 12 项全局一致性自查替代可机械执行部分；全量 141 项留待 Gate 1 validation 执行。
