# PageSpec · env-mgmt · 管理员（admin）

| 属性 | 值 |
|------|-----|
| 版本 | v1.0 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-16 |
| 用户类型 | admin |
| 唯一数据源 | 02-detailed/api/api-env-mgmt.md v1.0 |
| 组件体系 | Vue 3 + Naive UI（NGrid/NCard/NTag/NButton/NModal/NPopconfirm/NDrawer/NAlert/NSwitch/NSpin） |
| 确认方式 | headless run：7 项交互确认依据 D3 契约（api-env-mgmt.md D-01~D-12）+ ⚠️A1~A7 + FL v1.0 + PRD FR-01~06 推导，标注于各节 |

## 第 1 节 模块元信息

| 项 | 值 | 确认依据 |
|----|-----|---------|
| 模块编码 | env-mgmt（域前缀 /env） | api 3.1 |
| 业务类型 | master_data 子形态 + 工具动作混合（env_install_task：四态任务状态机；环境条目：实况投影） | 分册 2.1 判据；api 1.4 功能类型判定 |
| 页面类型 | env-list = **hybrid**（环境卡片列表 + 任务详情 Drawer 组合 + 任务历史双视图） | 分册 2.2；cron-backup 详情 Drawer 先例 |
| UI 框架 | Naive UI | 栈版本表 |
| 状态管理 | Pinia：envStore（环境投影缓存/generatedAt/通道状态/刷新、任务列表/当前任务详情） | 栈版本表 |
| 错误处理 | 统一拦截器按 api 3.4 code 分流：AUTH_001/002→跳登录、AUTH_003→提示刷新、AUTH_008→边界原因提示（ADR-018）；ENV_001→安装/卸载按钮禁用+通道不可用横幅（列表不报错，channel.available=false 呈现）、ENV_003→「该环境有任务进行中」+查看任务引导（data.taskId）、ENV_004→「环境未安装」提示、ENV_005→任务不存在提示刷新、ENV_006→失败原因展示（data.reason） | api 3.4 错误码表 |

**7 项交互确认**（headless 推导，逐项落点）：① 模块元信息见本节；② 查询条件见 4.2（任务列表 envType/status 筛选——环境全景无筛选 API 参数，诚实不虚构）；③ 表格列见 4.1/4.2；④ operList 见 4.1；⑤ 表单分组见 4.1 安装/卸载确认弹窗；⑥ 详情 Card 见 4.3 任务 Drawer；⑦ 页面类型覆盖：env-list=hybrid（卡片+Drawer 组合）。

## 第 2 节 Action 定义（与 API 一一对应）

| Action | 方法 | 路径 | 页面 | 权限 | CSRF |
|--------|------|------|------|------|------|
| envList | GET | /env/env/list | env-list | admin+readonly | — |
| envInstall | POST | /env/env/install | env-list（安装/升级按钮） | admin | ✓ |
| envUninstall | POST | /env/env/uninstall | env-list（卸载强确认） | admin | ✓ |
| envTaskList | GET | /env/task/list | env-list（任务 Drawer 列表 Tab） | admin+readonly | — |
| envTaskDetail | GET | /env/task/detail | env-list（任务 Drawer 详情） | admin+readonly | — |
| envServiceStart | POST | /env/service/start | env-list（卡片操作区） | admin | ✓ |
| envServiceStop | POST | /env/service/stop | env-list（卡片操作区） | admin | ✓ |
| envServiceRestart | POST | /env/service/restart | env-list（卡片操作区） | admin | ✓ |
| envServiceEnable | POST | /env/service/enable | env-list（自启开关） | admin | ✓ |
| envServiceDisable | POST | /env/service/disable | env-list（自启开关） | admin | ✓ |

> S7 一致性：10 Action 与 api-env-mgmt.md 10 接口一一对应（无未覆盖接口、无虚构接口）。

## 第 3 节 字典 Key 清单

| Key | 取值（=01-solution/04-business-enum.md 定稿） | 用途 |
|-----|--------------------------------------------|------|
| env_type | nginx / apache / docker / mysql / redis / php_fpm | 环境卡片标识与任务筛选 |
| env_task_action | install / upgrade / uninstall | 任务动作 Tag（Drawer） |
| env_task_status | pending / running / succeeded / failed | 任务状态 Tag（colorTag）+筛选 |
| package_manager_family | apt / dnf / yum | 通道状态呈现 |
| panel_role | admin / readonly | 角色判断（按钮可见性） |

**数据源规范偏差声明**：不实现 `POST /{domain}/dict/batch`（分册 3.6）——5 枚举全部固定值，字典由前端 TS 常量提供（单一事实源 04-business-enum.md；全仓既有偏差先例）。

**状态颜色方案**（分册 3.1 语义映射）：

| Key::值 | 颜色 |
|---------|------|
| env_task_status::pending | gray |
| env_task_status::running | blue（进行中，Spin 图标伴随） |
| env_task_status::succeeded | green |
| env_task_status::failed | red |
| 服务运行 running / 自启 on | green |
| 服务停止 / 自启 off | gray |
| 通道 available=true | green；false=orange（变更不可用警示） |

## 第 4 节 列表页

### 4.1 环境管理（env-list，hybrid）— 路由 /env

**权限**：admin+readonly 可见（ADR-018 全可看）；全部变更按钮 admin 专属。

**页头**：通道状态条（channel.available=true → NTag green「包管理通道：{family}」；false → NTag orange「包管理通道不可用（变更功能已禁用）」+ 说明 tooltip）+ 刷新按钮（重新 envList，D-01 实时探测）+ generatedAt「探测于 HH:mm:ss」。

**环境卡片网格**（NGrid 响应式 3 列 × NCard，六条目 D-02）：

每卡片字段与操作（数据源 envList items[]）：

| 区块 | 内容 | 呈现 |
|------|------|------|
| 标题 | displayName + env_type NTag | — |
| 描述 | description + relatedModule（「安装后可配合 网站管理 模块使用」类提示，软引导文案） | text |
| 实况 | installed：null →「探测不可用」orange（通道不可用）；true → 版本 version + 服务状态（serviceRunning→NTag green「运行中」/gray「已停止」）+ 自启（serviceEnabled）；false →「未安装」gray | D-01 实时投影 |
| 在途标记 | runningTaskId 非空 → NTag blue「任务进行中」+「查看」链接（跳任务 Drawer 定位该任务） | I-01 |
| 操作区（admin only；readonly 整区不渲染） | 未装：`安装`（primary）；已装：`升级` + `卸载`（error 色）+ 服务操作组（启动/停止/重启 NPopconfirm + 自启 NSwitch） | ENV_001 时安装/升级/卸载禁用；服务操作不受通道限制（systemd 通道） |

**安装/升级确认**（NPopconfirm）：文案「将通过 {family} 安装/升级 {displayName} 到仓库最新版（先刷新包索引，可能需要数分钟）；需配置官方仓库源的环境将自动写入源配置并留痕」。提交 envInstall → 成功 toast「任务已创建」+ 打开任务 Drawer 定位新任务（taskId 响应）；ENV_003 → 「该环境有任务进行中」+ 查看引导。

**卸载强确认**（NModal，两段警示 ⚠️A6/D-07）：

- 警示一（本域实况）：服务运行中 → NAlert warning「{displayName} 服务当前正在运行」；
- 警示二（关联模块静态影响）：NAlert error「卸载后对应功能将失去运行环境——Docker：容器管理；MySQL：数据库管理；nginx/PHP-FPM：网站管理；请先到对应模块确认数据已处置」；
- 操作锁定：NInput 输入环境显示名确认（与 certDelete 类强确认先例对齐：高危双因子）；
- 提交文案明示「将执行包卸载（保留系统配置与数据目录）」→ envUninstall。

### 4.2 任务列表（env-list 任务 Drawer「历史」Tab）

**查询条件**（对应 api task/list query 参数）：

| 条件 | 字段 | 组件 | 查询方式 |
|------|------|------|---------|
| 环境 | envType | NSelect（env_type，含「全部」） | eq |
| 状态 | status | NSelect（env_task_status，含「全部」） | eq |

**表格列**（NDataTable）：

| 列 | 字段 | 组件 | 宽度 | 配置 |
|----|------|------|------|------|
| 序号 | — | index | 60 | — |
| ID | id | link | 70 | 点击打开任务详情（4.3） |
| 环境 | envType | dict env_type | 110 | displayName 渲染 |
| 动作 | action | dict env_task_action | 90 | install=「安装」/upgrade=「升级」/uninstall=「卸载」 |
| 状态 | status | dict env_task_status | 110 | colorTag；failed 行附 error NTooltip |
| 发起者 | requestedBy | text | 100 | — |
| 发起时间 | createdAt | text | 170 | 本地时区 |
| 结束时间 | finishedAt | text | 170 | 未结束「—」 |

**刷新纪律**：进入 Tab 拉取一次（D-03 待对齐-3 落定：无自动轮询，手动刷新 + 写后刷新）；在途任务存在时 Drawer 顶部 NAlert「有任务进行中，手动刷新查看进度」（不自动轮询——cron-backup 详情 5s 轮询为单任务详情场景，列表页保守手动，R-09 无关但减无效请求）。

### 4.3 任务详情（env-list 任务 Drawer「详情」区，宿主 env-list）

**打开方式**：任务列表行 id link / 环境卡片在途标记「查看」/ 安装·卸载发起成功后自动定位。

**内容**（envTaskDetail）：

| 区块 | 内容 |
|------|------|
| 概要 | envType（displayName）/action/status（colorTag）/requestedBy/createdAt/startedAt/finishedAt |
| 失败原因 | error 非空 → NAlert error（含 timeout/面板重启中断如实呈现，D-04/I-04） |
| 命令输出尾部 | outputTail 只读等宽文本块（≤200 行，D-08）；running 态伴随 Spin +「执行中」 |
| 刷新 | 手动刷新按钮；**running/pending 态自动轮询 5s**（终态即停，onUnmounted 清理——cron-backup 详情 Drawer 同构先例） |

**readonly 视角差异**：任务列表与详情同等可看（ADR-018），无操作按钮。

## 第 5 节 表单页

无独立表单路由页——安装确认（NPopconfirm）、卸载强确认（NModal）、服务操作（NPopconfirm/NSwitch）均为即时动作 Dialog 形态（无草稿态）。

## 第 6 节 详情页

无独立详情路由页——任务详情=Drawer 详情区（4.3，hybrid 宿主 env-list）。

## 第 7 节 路由定义

| 路由 | 页面 | meta |
|------|------|------|
| /env | env-list | `{requiresAuth:true, title:'环境管理'}`（readonly 只读视图） |

**守卫顺序**（与全局框架一致）：sessionCurrent 探测 → roles 校验（两角色均可达；unauthorized → 403 页）。

**侧边菜单（本模块贡献）**：环境管理（/env，Order 位于容器管理之后——构建期登记 D-P18/api-panel-core v2.2）；两角色同项。

## 第 8 节 全局一致性

| # | 检查 | 结果 |
|---|------|------|
| 1 | Action ↔ api-env-mgmt.md 接口一一对应（S7） | ✅ 10/10 全映射，无虚构 |
| 2 | 字典 Key ↔ 04-business-enum.md 定稿枚举一一对应（S8） | ✅ 5/5（env_type/env_task_action/env_task_status/package_manager_family/panel_role，取值逐字对齐定稿） |
| 3 | 所有 POST Action 均携带 X-CSRF-Token（拦截器统一注入） | ✅ |
| 4 | ADR-018 边界：readonly 操作区/安装/卸载/服务操作不渲染 + 后端 AUTH_008 双保险 | ✅ 4.1 |
| 5 | 错误文案与 api 3.4 错误码一一对应（ENV_001~006/AUTH 族） | ✅ 第 1 节错误处理 |
| 6 | 查询条件仅含 API 真实参数（任务列表 envType/status；环境全景无筛选）——不虚构 | ✅ 4.2 |
| 7 | 字典数据源偏差（无 dict/batch，前端常量）已声明 | ✅ 第 3 节 |
| 8 | 实况如实呈现：通道不可用 installed=null「探测不可用」≠「未安装」；面板重启中断任务如实展示 | ✅ 4.1/4.3（D-01/I-04） |
| 9 | ⚠️A6 卸载强确认两段警示 + 操作锁定；⚠️A5 安装后不强制启动（无「安装并启动」选项） | ✅ 4.1 |
| 10 | 时间展示本地时区、提交 ISO UTC；generatedAt 投影时刻呈现（D-01） | ✅ 4.1 |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-16 | initial | 首次生成：1 hybrid 页面（环境卡片网格 + 任务 Drawer 双区）+ 10 Action、5 字典 Key、0 独立表单路由/0 独立详情路由；安装/升级 NPopconfirm、卸载强确认 NModal（两段警示+输入锁定）、服务操作组；任务详情 running 态 5s 轮询终态即停 |

> FX 质量清单说明：FX-001~FX-141 逐条清单文件未随引擎 payload 发布，本节按分册 9 节结构逐节落实，并以第 8 节 10 项全局一致性自查替代可机械执行部分（全仓先例）；全量 141 项留待 Gate 1 validation 执行。
