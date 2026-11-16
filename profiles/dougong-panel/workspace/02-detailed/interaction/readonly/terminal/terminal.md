# PageSpec · Web 终端（terminal） · 只读用户（readonly）

| 属性 | 值 |
|------|-----|
| 版本 | v1.0 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-14 |
| 用户类型 | readonly |
| 唯一数据源 | 02-detailed/api/api-terminal.md v1.0 |
| 模式声明 | **零可达整域禁用**（全项目第三种 readonly 模式，对照先例：monitoring=双视角全等、panel-core/service-mgmt=子集视角）——本模块对 readonly 无任何页面、路由、菜单与 Action；本文档为负空间的显式登记（权限配置依据），不存在以 admin 文档为事实源的子集复用 |
| 边界依据 | ADR-018「禁 Web 终端」：readonly 整体拒绝（比「禁上传下载」更彻底的整域禁用）；FL v1.0 readonly 章显式登记「无任何菜单与功能」；api 3.3 权限矩阵 readonly 两端点全 ✗（AUTH_008） |
| 组件体系 | 无（不渲染任何本模块组件） |

## 第 1 节 模块元信息

| 项 | 值 |
|----|-----|
| 模块编码 | terminal |
| 业务类型 | 工具类·会话通道（同 admin 判定；对本角色零可达） |
| 页面类型 | **无页面**（0 页面 / 0 路由 / 0 菜单 / 0 Action） |
| 可见性实现 | 菜单层：ModuleMenuRegistry 服务端按角色下发——terminal 菜单项对 readonly **不下发**（admin 文档第 7 节）；路由层：/terminal 挂 requiresAdmin 守卫（admin 文档第 7 节 meta），直接输入 URL 被守卫拦截 |
| 错误处理 | 越界兜底：伪造 WS 握手（/terminal/session/connect）→ 握手角色判定拒绝返回 AUTH_008「权限不足：只读角色禁止使用 Web 终端」+ 边界原因（api I-01/3.4）；伪造 GET /terminal/user/options → 全局 RoleRequired 拦截 AUTH_008——均为正常 UI 不可达的防御路径 |

## 第 2 节 Action 定义

**无 Action**（0 个）。

**负空间声明**（api 3.3 权限矩阵 ✗ 项逐项对应）：

| admin Action（readonly 不注册） | api 权限矩阵 ✗ 项 | 负空间落法 |
|--------------------------------|------------------|-----------|
| userOptions | GET /terminal/user/options = ✗ | 菜单不下发 + 无页面入口 + 无路由可达（守卫拦截）；不存在任何触发此请求的 UI 路径 |
| sessionConnect | WS /terminal/session/connect = ✗ | 同上；越界握手由服务端 TerminalHandshakeGuard 角色判定拒绝（AUTH_008，api I-01 顺序②③） |

> 双层保障：UI 层零入口（菜单/路由/组件均不存在）+ 服务端层边界拒绝（AUTH_008 附边界原因）；与 service-mgmt「操作列不渲染」负空间同构，范围扩大至整域（无任何可渲染子集）。

## 第 3 节 字典 Key 清单

无本模块字典消费（0 个）——不渲染任何本模块 UI，terminal_end_reason / terminal_identity_source 仅存在于 admin 视角（enums.ts 常量源全局共享，readonly 会话不引用本模块条目）。

## 第 4 节 核心页

**无**（无终端页、无列表页、无查询条件、无表格——整域零可达，第 1 节）。

## 第 5 节 表单页

**无**（新建会话对话框为 admin 专属；readonly 无任何表单）。

## 第 6 节 详情页

**无**（帧反馈等价物亦不存在——无会话即无 ready/closed 帧）。

## 第 7 节 路由定义

| 项 | 值 |
|----|-----|
| 路由 | 无本模块路由注册（/terminal 对 readonly 不可达：requiresAdmin 守卫） |
| 框架菜单 | menuList 不含 terminal 菜单项（服务端按 ADR-018 不下发） |

## 第 8 节 全局一致性

| # | 检查 | 结果 |
|---|------|------|
| 1 | readonly Action 与 api 只读接口一致 | ✅ 0 Action = api 3.3 readonly ✓ 项全集（空集——两端点均 ✗） |
| 2 | 负空间检查：api 3.3 权限矩阵 ✗ 项逐项对应 | ✅ 2/2（userOptions/sessionConnect → 菜单不下发 + 守卫拦截 + 服务端 AUTH_008，第 2 节表） |
| 3 | 零可达模式显式声明（区别于全等/子集先例，无事实源复用） | ✅ 头部模式声明 |
| 4 | ADR-018 三禁落法：禁终端=本模块整域负空间（本模块即「禁终端」条款的直接对象）；禁变更/禁上传下载=本模块无可作用对象 | ✅ 头部边界依据 |
| 5 | FL v1.0 readonly 章一致性（「无任何菜单与功能」显式登记） | ✅ FL 2.2 章 |
| 6 | 越界兜底文案与全局拦截器一致（正常 UI 不可达） | ✅ 第 1 节 |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-14 | initial | 首次生成：零可达整域禁用模式（0 页面/0 路由/0 菜单/0 Action）——api 3.3 权限矩阵 2 个 ✗ 项逐项负空间对应（菜单不下发 + requiresAdmin 守卫 + 服务端 AUTH_008 三层）；本文件为权限配置依据的显式登记，非子集复用 |

> FX 清单执行方式同 admin 文档第 9 节注记（清单文件未随 payload 发布，按 9 节结构落实 + 第 8 节自查；全量留待 Gate 1）。
