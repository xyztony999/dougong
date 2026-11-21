# PageSpec · app-mgmt · 只读用户（readonly）

| 属性 | 值 |
|------|-----|
| 版本 | v1.0 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-20（初始生成） |
| 用户类型 | readonly |
| 唯一数据源 | 02-detailed/api/api-app-mgmt.md v1.0 |
| 组件体系 | Vue 3 + Naive UI（NGrid/NTag/NDataTable/NDrawer/NAlert/NEmpty） |
| 确认方式 | headless run：7 项交互确认依据 D3 契约推导（api-app-mgmt.md 权限矩阵 3.3 + ⚠️A8 日志 admin 专属 + ADR-018 负空间口径），标注于各节 |

## 第 1 节 模块元信息

| 项 | 值 | 确认依据 |
|----|-----|---------|
| 模块编码 | app-mgmt（域前缀 /app） | api 3.1 |
| 业务类型 | master_data 子形态（同 admin 视角） | 分册 2.1 |
| 页面类型 | app-list = **hybrid**（应用列表只读 + 任务 Drawer 只读；运维/创建/日志面整体负空间） | 分册 2.2 |
| 错误处理 | AUTH_001/002→跳登录；AUTH_008→边界原因提示（全部写接口 + env/log 读取越界——**应用日志 admin 专属 ⚠️A8**，沿数据库数据内容先例） | api 3.4 |

## 第 1a 节 v1.1 增量（容器化应用双形态，2026-11-22）

创建向导双形态（venv/容器化）对 readonly 整体负空间不变——容器化形态的镜像/端口/环境变量等表单与创建按钮均 admin 专属；readonly 视角零变化。

## 第 2 节 Action 定义（只读子集 3 + 负空间 13）

| Action | 方法 | 路径 | 页面 | 权限 |
|--------|------|------|------|------|
| appList | GET | /app/app/list | app-list | admin+readonly |
| appTaskList | GET | /app/task/list | 任务 Drawer 历史 Tab | admin+readonly |
| appTaskDetail | GET | /app/task/detail | 任务 Drawer 详情区 | admin+readonly |

**负空间 13 项**（ADR-018：写类操作全部不渲染；admin 专属读取面不渲染）：appCreate（无创建按钮/向导）、appDelete（无操作列删除）、appEnvGet/appParamsSave（参数 Tab 整体不渲染）、appDepsReinstall（无重装依赖）、appTaskRetry（详情无重试按钮）、appServiceStart/Stop/Restart（无服务操作组）、appServiceEnable/Disable（无自启开关）、appLogGet（**日志 Tab 整体不渲染**——日志内容 admin 专属 ⚠️A8）、appSiteLink（无建站入口）。3 读+13 负空间=16 全集（机械核验）。

## 第 3 节 字典 Key 清单

| Key | 取值 | 用途 |
|-----|------|------|
| app_task_action | create / deps_install / delete | 任务动作 Tag |
| app_task_status | pending / running / succeeded / failed | 任务状态 Tag（colorTag 同 admin 配色） |
| app_start_mode | preset_fastapi / custom | 启动方式呈现 |
| env_type | （引用） | 就绪度缺失提示上下文 |
| panel_role | admin / readonly | 角色判断 |

**状态颜色方案**：与 admin PageSpec 第 3 节逐字同源（两角色同数据同配色）。

## 第 4 节 列表页

### 4.1 应用管理（app-list，hybrid）— 路由 /app

**页头**：Python 就绪度状态条（两角色同等呈现——只读观测语义）+ 刷新按钮 + generatedAt（无创建按钮——负空间）。

**应用表格**（NDataTable；列同 admin 视角数据，但无操作列）：

| 列 | 字段 | 组件 | 配置 |
|----|------|------|------|
| 序号 | — | index | — |
| 应用名 | name | link | 打开运维 Drawer 概要 Tab（**参数/日志 Tab 不渲染**——参数 admin 专属操作面、日志内容 admin 专属 ⚠️A8；概要 Tab 呈现实况观测，无操作按钮） |
| 启动方式 | startMode | dict app_start_mode | — |
| 端口 | listenPort | text | — |
| 服务状态 | serviceRunning | 状态组 | 三态如实（同 admin 配色；服务异常 orange，无「查看日志」引导——日志负空间，tooltip 呈现「服务异常，请联系管理员」） |
| 自启 | serviceEnabled | NTag | 只读呈现 |
| 依赖 | depsInstalled | NTag | 只读呈现 |
| 在途标记 | runningTaskId | NTag+链接 | 「查看」跳任务 Drawer（只读） |
| 站点 | hasSite | text | 已建站链接保留（站点管理只读视图可达）；未建站 gray「仅本机可访问」 |

**负空间声明**：无创建按钮/操作列整列不渲染/运维 Drawer 参数与日志 Tab 不渲染（ADR-018；⚠️A8 日志 admin 专属——readonly 概要 Tab 亦不含日志入口）。

### 4.2 任务列表（任务 Drawer「历史」Tab，只读）

查询条件（appId/action/status）与表格列同 admin 视角（purgeDir 范围角标同等呈现）；无重试按钮列语义（retryable 字段两角色同数据，按钮仅 admin 渲染——负空间）。

### 4.3 任务详情（任务 Drawer「详情」区，只读）

概要/失败原因/输出尾部/刷新轮询（running 5s 终态即停）与 admin 同等（ADR-018）；**重试按钮不渲染**（负空间）。

## 第 5 节 表单页

无（全部表单/操作 Dialog 为 admin 专属，readonly 无入口）。

## 第 6 节 详情页

无独立详情路由页（同 admin，Drawer 承载；运维 Drawer 仅概要 Tab 可达）。

## 第 7 节 路由定义

| 路由 | 页面 | meta |
|------|------|------|
| /app | app-list | `{requiresAuth:true, title:'应用管理'}`（readonly 只读视图） |

守卫顺序同全局框架；侧边菜单：应用管理（两角色同项）。

## 第 8 节 全局一致性

| # | 检查 | 结果 |
|---|------|------|
| 1 | 只读 Action ↔ api 权限矩阵 readonly 列（S7 只读子集） | ✅ 3/3（appList/appTaskList/appTaskDetail），与 admin 16 Action 的 readonly 列逐行一致 |
| 2 | 字典 Key 同源（S8） | ✅ 5/5 与 admin/池同值 |
| 3 | 负空间机械核验 | ✅ 3 读+13 负空间=16 全集（含 log/env admin 专属读取面 ⚠️A8） |
| 4 | 只读越界双保险（前端不渲染 + 后端 AUTH_008） | ✅ |
| 5 | 查询条件仅含 API 真实参数 | ✅ 4.2 |
| 6 | 实况如实三态（null 不虚构） | ✅ 4.1 |
| 7 | 时间本地时区/generatedAt | ✅ |
| 8 | 数据与 admin 视角一致（ADR-018 全可看语义：清单/任务同数据；仅操作面与日志负空间） | ✅ |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-20 | initial | 首次生成：readonly 只读视角（1 hybrid 页面 + 任务 Drawer 只读）——3 读 Action（appList/appTaskList/appTaskDetail）+ 13 负空间（写操作全部 + 参数/日志 admin 专属 Tab ⚠️A8）；应用表格无操作列；任务 retry 不渲染；数据与 admin 同源同配色（ADR-018 全可看） |
