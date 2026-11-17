# PageSpec · env-mgmt · 管理员（admin）

| 属性 | 值 |
|------|-----|
| 版本 | v1.3 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-17（v1.3 安装后配置就绪引导结果呈现增量） |
| 用户类型 | admin |
| 唯一数据源 | 02-detailed/api/api-env-mgmt.md v1.2 |
| 组件体系 | Vue 3 + Naive UI（NGrid/NCard/NTag/NButton/NModal/NPopconfirm/NDrawer/NAlert/NSwitch/NSpin） |
| 确认方式 | headless run：7 项交互确认依据 D3 契约（api-env-mgmt.md D-01~D-19）+ ⚠️A1~A15 + FL v1.1 + PRD FR-01~07 推导，标注于各节 |

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
| env_type | nginx / apache / docker / mysql / redis / php_fpm / mongodb（v1.1 七值，与池/DLL CHECK/api 三方 S8 同值） | 环境卡片标识与任务筛选 |
| env_task_action | install / upgrade / uninstall | 任务动作 Tag（Drawer） |
| env_task_status | pending / running / succeeded / failed | 任务状态 Tag（colorTag）+筛选 |
| package_manager_family | apt / dnf / yum | 通道状态呈现 |
| bootstrap_status | succeeded / failed / skipped（v1.3，与池/DDL CHECK/api 三方 S8 同值；NULL=无引导步骤非枚举值，不渲染角标） | 任务引导结果角标（列表）+ 详情横幅（4.2/4.3） |
| panel_role | admin / readonly | 角色判断（按钮可见性） |

**数据源规范偏差声明**：不实现 `POST /{domain}/dict/batch`（分册 3.6）——6 枚举全部固定值，字典由前端 TS 常量提供（单一事实源 04-business-enum.md；全仓既有偏差先例）。

**状态颜色方案**（分册 3.1 语义映射）：

| Key::值 | 颜色 |
|---------|------|
| env_task_status::pending | gray |
| env_task_status::running | blue（进行中，Spin 图标伴随） |
| env_task_status::succeeded | green |
| env_task_status::failed | red |
| bootstrap_status::succeeded | green（已就绪） |
| bootstrap_status::failed | orange（已安装·引导失败——区别任务 failed 红，包已装故非红） |
| bootstrap_status::skipped | gray（保守跳过） |
| 服务运行 running / 自启 on | green |
| 服务停止 / 自启 off | gray |
| 通道 available=true | green；false=orange（变更不可用警示） |

## 第 4 节 列表页

### 4.1 环境管理（env-list，hybrid）— 路由 /env

**权限**：admin+readonly 可见（ADR-018 全可看）；全部变更按钮 admin 专属。

**页头**：通道状态条（channel.available=true → NTag green「包管理通道：{family}」；false → NTag orange「包管理通道不可用（变更功能已禁用）」+ 说明 tooltip）+ 刷新按钮（重新 envList，D-01 实时探测）+ generatedAt「探测于 HH:mm:ss」。

**环境卡片网格**（NGrid 响应式 3 列 × NCard，七条目 D-02，v1.1 六→七 +MongoDB〔displayName=MongoDB〕；v1.1a 移除 relatedModule 软引导行）：

每卡片字段与操作（数据源 envList items[]）：

| 区块 | 内容 | 呈现 |
|------|------|------|
| 标题 | displayName + env_type NTag | — |
| 描述 | description（v1.1a 起：仅环境本体描述，无模块引导文案） | text |
| 实况 | installed 三态（v1.1 ⚠️A9 精确化）：null →「探测不可用」orange（**仅条目级真错误**——查询超时/通道异常；通道级不可用时整列 null 并由页头横幅承载语义，角标不再重复渲染通道语义）；true → 版本 version + 服务状态（serviceRunning→NTag green「运行中」/gray「已停止」）+ 自启（serviceEnabled）；false →「未安装」gray（**含包名不在包数据库形态**——多候选全未装即未装，非探测失败） | D-01 实时投影三态 |
| 在途标记 | runningTaskId 非空 → NTag blue「任务进行中」+「查看」链接（跳任务 Drawer 定位该任务） | I-01 |
| 操作区（admin only；readonly 整区不渲染） | 未装：`安装`（primary）；已装：`升级` + `卸载`（error 色）+ 服务操作组（启动/停止/重启 NPopconfirm + 自启 NSwitch） | ENV_001 时安装/升级/卸载禁用；服务操作不受通道限制（systemd 通道） |

**安装/升级确认**（NPopconfirm）：文案「将通过 {family} 安装/升级 {displayName} 到仓库最新版（先刷新包索引，可能需要数分钟）；需配置官方仓库源的环境将自动写入源配置并留痕（MongoDB/Docker CE〔RHEL 系〕需第三方官方源，面板代配）」。提交 envInstall → 成功 toast「任务已创建」+ 打开任务 Drawer 定位新任务（taskId 响应）；ENV_003 → 「该环境有任务进行中」+ 查看引导。

**卸载强确认**（NModal，两段警示 ⚠️A6/D-07）：

- 警示一（本域实况）：服务运行中 → NAlert warning「{displayName} 服务当前正在运行」；
- 警示二（关联模块静态影响）：NAlert error「卸载后对应功能将失去运行环境——Docker：容器管理；MySQL/MongoDB：数据库管理；nginx/PHP-FPM：网站管理；请先到对应模块确认数据已处置」（v1.1 补 MongoDB）；
- 操作锁定：NInput 输入环境显示名确认（与 certDelete 类强确认先例对齐：高危双因子）；
- 提交文案明示「将执行包卸载（保留系统配置与数据目录；已配第三方源文件保留不回收）」→ envUninstall。

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
| 引导 | bootstrapStatus | dict bootstrap_status | 90 | v1.3：非空→colorTag（succeeded=「已就绪」green/failed=「引导失败」orange/skipped=「跳过」gray）+ bootstrapNote NTooltip；NULL（无引导步骤）→「—」不渲染角标 |
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
| 引导结果（v1.3，bootstrapStatus 非空时渲染） | **succeeded** → NAlert success「配置就绪：已创建管理账户并注册到数据库管理，可直接治理」+「前往数据库管理」链接（/databases）；**failed** → NAlert warning「已安装·引导失败」+ bootstrapNote 原因 + 恢复指引「可重新安装/升级以重试引导（幂等）」（任务 status 仍 succeeded——包已装，引导失败独立维度，D-18/⚠️A14）；**skipped** → NAlert info「保守跳过：」+ bootstrapNote（如「实例已配置凭据」/「管理账户已存在」，不覆盖用户已有配置，⚠️A13）；NULL（无引导步骤）→ 不渲染 |
| 命令输出尾部 | outputTail 只读等宽文本块（≤200 行，D-08；含 [bootstrap] 引导步骤行）；running 态伴随 Spin +「执行中」 |
| 刷新 | 手动刷新按钮；**running/pending 态自动轮询 5s**（终态即停，onUnmounted 清理——cron-backup 详情 Drawer 同构先例）；**终态跃迁触发环境全景与任务清单重拉**（v1.2：任务完成后 installed/version/service 态与在途标记落定——v0.1.4 反馈第 1 项；发起 install/upgrade/uninstall 成功后的写后刷新既有语义复核保持） |

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
| 1 | Action ↔ api-env-mgmt.md 接口一一对应（S7） | ✅ 10/10 全映射，无虚构（v1.3 引导为 worker 机制+OHS，无新 HTTP Action；task/list·detail 仅响应字段扩展） |
| 2 | 字典 Key ↔ 04-business-enum.md 定稿枚举一一对应（S8） | ✅ 6/6（env_type〔七值 v1.1〕/env_task_action/env_task_status/package_manager_family/**bootstrap_status〔三值 v1.3〕**/panel_role，取值逐字对齐定稿） |
| 3 | 所有 POST Action 均携带 X-CSRF-Token（拦截器统一注入） | ✅ |
| 4 | ADR-018 边界：readonly 操作区/安装/卸载/服务操作不渲染 + 后端 AUTH_008 双保险 | ✅ 4.1 |
| 5 | 错误文案与 api 3.4 错误码一一对应（ENV_001~006/AUTH 族） | ✅ 第 1 节错误处理 |
| 6 | 查询条件仅含 API 真实参数（任务列表 envType/status；环境全景无筛选）——不虚构 | ✅ 4.2 |
| 7 | 字典数据源偏差（无 dict/batch，前端常量）已声明 | ✅ 第 3 节 |
| 8 | 实况如实呈现：三态区分（通道级不可用=页头横幅+整列 null；条目级真错误=「探测不可用」角标；未装=「未安装」含包名不在数据库形态）≠ 混同；面板重启中断任务如实展示 | ✅ 4.1/4.3（D-01/I-04，v1.1 ⚠️A9） |
| 9 | ⚠️A6 卸载强确认两段警示 + 操作锁定；⚠️A5 安装后不强制启动（无「安装并启动」选项） | ✅ 4.1 |
| 10 | 时间展示本地时区、提交 ISO UTC；generatedAt 投影时刻呈现（D-01） | ✅ 4.1 |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-16 | initial | 首次生成：1 hybrid 页面（环境卡片网格 + 任务 Drawer 双区）+ 10 Action、5 字典 Key、0 独立表单路由/0 独立详情路由；安装/升级 NPopconfirm、卸载强确认 NModal（两段警示+输入锁定）、服务操作组；任务详情 running 态 5s 轮询终态即停 |
| v1.1a | 2026-11-17 | content_changed | UI 轮用户拍板：卡片移除「安装后可配合「X」模块使用」软引导行（relatedModule 字段随 api v1.1a 移除）；描述行仅环境本体描述 |
| v1.1 | 2026-11-17 | dict_changed + content_changed | W1 实机冒烟缺陷修复轮（数据源升 api v1.1）：①字典 env_type 六值→七值（+mongodb，S8 与池/DDL/api 同值）；②4.1 卡片网格六→七条目（+MongoDB 卡片：displayName=MongoDB/relatedModule=数据库管理）；③实况三态呈现精确化（⚠️A9：null=条目级真错误角标，通道级不可用归页头横幅不重复渲染；false=未安装含包名不在数据库形态）；④安装确认文案提及 MongoDB/Docker CE 第三方源代配；卸载警示二补 MongoDB、提交文案补源文件保留说明；⑤第 8 节检查 2/8 同步修订 |
| v1.2 | 2026-11-17 | content_changed | v0.1.4 用户反馈第 1 项（环境页刷新机制缺陷修复）：任务详情在途→终态跃迁时触发环境全景（loadList）与任务清单（loadTasks）重拉——修复「任务完成后环境列表不重拉、用户看到旧 installed/版本/服务态」；轮询纪律不变（running 5s/终态即停/无 SSE/WS，D 阶段口径保持）；readonly 同步（任务 Drawer 两角色可看） | 4.3 |
| v1.3 | 2026-11-17 | dict_changed + content_changed | 安装后配置就绪增量（数据源升 api v1.2，D-15~D-19/⚠️A11~A15）：①字典 +bootstrap_status（三值 succeeded/failed/skipped，S8 6/6 与池/DDL/api 同值；NULL=无引导步骤不渲染）+颜色方案（succeeded green/failed orange〔包已装故非红〕/skipped gray）；②4.2 任务列表 +「引导」列（bootstrapStatus colorTag + bootstrapNote NTooltip）；③4.3 任务详情 +引导结果横幅（succeeded=已就绪+前往数据库管理链接 / failed=已安装·引导失败+原因+重新安装/升级重试指引 / skipped=保守跳过说明；任务 status 仍包操作结果，引导失败独立维度 D-18）；④outputTail 含 [bootstrap] 行注记；⑤第 8 节检查 1/2 同步（S7 10/10 不变——引导无新 HTTP Action）。**Action/路由/表单零变更**（引导由 worker 自动触发，结果经既有 task/detail 呈现）；readonly 同步（引导结果两角色可看） | 3 / 4.2 / 4.3 / 8 |

> FX 质量清单说明：FX-001~FX-141 逐条清单文件未随引擎 payload 发布，本节按分册 9 节结构逐节落实，并以第 8 节 10 项全局一致性自查替代可机械执行部分（全仓先例）；全量 141 项留待 Gate 1 validation 执行。
