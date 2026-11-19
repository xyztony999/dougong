# PageSpec · db-tools · 管理员（admin）

| 属性 | 值 |
|------|-----|
| 版本 | v1.1 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-19（v1.1 Gate 1 W3 修复轮：+dbtoolReopen SignonURL 重认证落地 Action，S7 9/9） |
| 用户类型 | admin |
| 唯一数据源 | 02-detailed/api/api-db-tools.md v1.2 |
| 组件体系 | Vue 3 + Naive UI（NGrid/NCard/NTag/NButton/NModal/NPopconfirm/NDrawer/NAlert/NSelect/NDataTable/NSpin/NInput） |
| 确认方式 | headless run：7 项交互确认依据 D3 契约（api-db-tools.md D-01~D-18）+ ⚠️A1~A13 + FL v1.0 + PRD FR-01~10 推导，标注于各节 |

## 第 1 节 模块元信息

| 项 | 值 | 确认依据 |
|----|-----|---------|
| 模块编码 | db-tools（域前缀 /dbtool） | api 3.1/D-01 |
| 业务类型 | master_data 子形态 + 工具动作混合（db_tool：部署档案+凭据密文无状态机；tool_deploy_task：四态任务状态机；工具条目：实况投影） | 分册 2.1 判据；api 1.4 功能类型判定 |
| 页面类型 | tool-list = **hybrid**（工具卡片列表 + 任务详情 Drawer 组合 + 任务历史双视图） | 分册 2.2；env-mgmt env-list 同构先例 |
| UI 框架 | Naive UI | 栈版本表 |
| 状态管理 | Pinia：dbtoolStore（工具投影缓存/generatedAt/刷新、任务列表/当前任务详情、在途标记） | 栈版本表 |
| 错误处理 | 统一拦截器按 api 3.4 code 分流：AUTH_001/002→跳登录、AUTH_003→提示刷新、AUTH_008→边界原因提示（ADR-018）；DBT_001→依赖缺失内联清单（data.missing）+「到环境管理安装」链接（/env）；DBT_002→参数错误提示；DBT_003→「该工具有任务进行中」+查看任务引导（data.taskId）；DBT_004→「工具未安装」提示（引导安装）；DBT_005→任务不存在提示刷新；DBT_006→「实例未就绪」+「到数据库管理配置凭据」链接（/databases，data.instanceId）；DBT_009→轮换不支持提示（phpmyadmin 无轮换入口，前端本不渲染）；DBT_011→「当前已为目录版本」提示；DBT_012→失败原因展示（data.reason）；DBT_007/008/010→signon 内部端点错误码（前端不直接消费——浏览器经工具侧呈现拒绝，面板侧打开失败引导重新打开，4.4） | api 3.4 错误码表 |

**7 项交互确认**（headless 推导，逐项落点）：① 模块元信息见本节；② 查询条件见 4.2（任务列表 toolType/status 筛选——工具全景无筛选 API 参数，诚实不虚构）；③ 表格列见 4.1/4.2；④ operList 见 4.1；⑤ 表单分组见 4.1 安装/卸载/轮换确认弹窗 + 4.4 打开分发；⑥ 详情 Card 见 4.3 任务 Drawer；⑦ 页面类型覆盖：tool-list=hybrid（卡片+Drawer 组合）；**打开方式裁定=新标签页**（4.4，非内嵌 iframe）。

## 第 2 节 Action 定义（与 API 一一对应）

| Action | 方法 | 路径 | 页面 | 权限 | CSRF |
|--------|------|------|------|------|------|
| dbtoolList | GET | /dbtool/tool/list | tool-list | admin+readonly | — |
| dbtoolInstall | POST | /dbtool/tool/install | tool-list（安装按钮/弹窗） | admin | ✓ |
| dbtoolUpgrade | POST | /dbtool/tool/upgrade | tool-list（升级按钮） | admin | ✓ |
| dbtoolUninstall | POST | /dbtool/tool/uninstall | tool-list（卸载强确认） | admin | ✓ |
| dbtoolTaskList | GET | /dbtool/task/list | tool-list（任务 Drawer 列表 Tab） | admin+readonly | — |
| dbtoolTaskDetail | GET | /dbtool/task/detail | tool-list（任务 Drawer 详情） | admin+readonly | — |
| dbtoolOpen | POST | /dbtool/access/open | tool-list（打开按钮）+ database-mgmt 高级入口（跨模块消费） | admin | ✓ |
| dbtoolReopen | GET | /dbtool/access/reopen | SignonURL 重认证落地（v1.1 W3；浏览器 302 顶层导航非 fetch Action——phpMyAdmin 会话过期重定向触发，admin 会话 gating 重签令牌回工具 URL，4.4） | admin | —（GET） |
| dbtoolRotate | POST | /dbtool/access/rotate | tool-list（mongo-express 轮换按钮） | admin | ✓ |

> S7 一致性：9 Action 与 api-db-tools.md v1.2 面板面 9 接口一一对应（无未覆盖接口、无虚构接口；v1.1 W3 +dbtoolReopen）。**dbtoolReopen 为 GET 重定向落地 Action**——浏览器顶层导航（phpMyAdmin SignonURL 302 GET）触发，非前端 fetch 调用，但属面板面 admin 专属接口故入 Action 清单（api 4.1.4）。**内部端点 2（GET /dbtool/gate/verify、POST /dbtool/signon/verify）非前端 Action**——调用方为 nginx auth_request 子请求与 PHP-FPM SignonScript 服务端回调（api D-11/4.4），浏览器不经前端代码调用，故不入 Action 清单（11 端点 = 9 前端 Action + 2 内部服务端端点）。
>
> **跨模块 Action 引用（非本模块 S7 计数）**：`instanceList`（GET /db/instance/list，database-mgmt，admin+readonly）——mongo-express 安装实例选择器数据源（4.1，engine=mongodb 筛选 + credentialsSet 就绪判定）；本模块高级入口被 database-mgmt PageSpec 消费（dbtoolList/dbtoolOpen，见 database-mgmt v1.5 增量）。

## 第 3 节 字典 Key 清单

| Key | 取值（=01-solution/04-business-enum.md 定稿） | 用途 |
|-----|--------------------------------------------|------|
| db_tool_type | phpmyadmin / mongo_express（D2 定稿，与池/api 三方 S8 同值） | 工具卡片标识与任务筛选 |
| db_tool_task_action | install / upgrade / uninstall | 任务动作 Tag（Drawer） |
| db_tool_task_status | pending / running / succeeded / failed | 任务状态 Tag（colorTag）+筛选 |
| db_engine | mysql / mariadb / redis / mongodb（引用 database-mgmt 定稿） | 工具目标引擎呈现（targetEngines）+安装实例选择器筛选 |
| panel_role | admin / readonly | 角色判断（按钮可见性） |

**数据源规范偏差声明**：不实现 `POST /{domain}/dict/batch`（分册 3.6）——5 枚举全部固定值，字典由前端 TS 常量提供（单一事实源 04-business-enum.md；全仓既有偏差先例）。depsMissing[].kind=env 的 name 为 env_type 值（env-mgmt 字典，透传呈现 displayName 映射）；kind=extension 的 name 为 PHP 扩展名（开放集合非枚举，原样呈现）。

**状态颜色方案**（分册 3.1 语义映射）：

| Key::值 | 颜色 |
|---------|------|
| db_tool_task_status::pending | gray |
| db_tool_task_status::running | blue（进行中，Spin 图标伴随） |
| db_tool_task_status::succeeded | green |
| db_tool_task_status::failed | red |
| 部署 installed=true | green「已安装」+ installedVersion；false=gray「未安装」 |
| 运行 running=true | green「运行中」；false=gray「已停止」；null=orange「运行态未知」（探测失败不虚构） |
| 依赖 depsReady=true | green「依赖就绪」；false=orange「依赖缺失」+ depsMissing 清单 |
| db_engine::mysql/mariadb | blue；mongodb=green；redis=red（沿 database-mgmt 配色先例） |

## 第 4 节 列表页

### 4.1 工具管理（tool-list，hybrid）— 路由 /db-tools

**权限**：admin+readonly 可见（ADR-018 全可看）；全部变更与打开按钮 admin 专属（⚠️A10 工具访问整体 admin 专属）。

**页头**：说明条（NAlert info「集成工具经面板会话安全访问（仅管理员）；工具仅监听本机，不暴露公网端口。面板内表/文档浏览（轻量入口）保留并存。」）+ 刷新按钮（重新 dbtoolList，D-02 实时探测）+ generatedAt「探测于 HH:mm:ss」。

**工具卡片网格**（NGrid 响应式 2 列 × NCard，两条目 D-13：phpMyAdmin/mongo-express）：

每卡片字段与操作（数据源 dbtoolList items[]）：

| 区块 | 内容 | 呈现 |
|------|------|------|
| 标题 | displayName + db_tool_type NTag + targetEngines（db_engine 色 tag 列表） | — |
| 描述 | description + 目录版本 catalogVersion（「目录版本 {catalogVersion}」） | text |
| 部署状态 | installed=true → NTag green「已安装」+ installedVersion；false → NTag gray「未安装」；**mongo_express 已装时追加「绑定实例 {boundInstanceId 经 instanceList 反查 identityKey}」呈现（phpmyadmin boundInstanceId=null 不渲染——打开时绑定）** | D-02 档案存在性投影（boundInstanceId api v1.1） |
| 运行态 | running=true → NTag green「运行中」；false → gray「已停止」；null → orange「运行态未知」 | 实况投影（不虚构） |
| 依赖就绪度 | depsReady=true → NTag green「依赖就绪」；false → NTag orange「依赖缺失」+ depsMissing 清单（kind=env → 「{name} 环境未安装」+「到环境管理安装」链接 /env；kind=extension → 「PHP 扩展 {name} 缺失」+ 补装指引文案） | DBT_001 前置数据源（⚠️A2） |
| 在途标记 | runningTaskId 非空 → NTag blue「任务进行中」+「查看」链接（跳任务 Drawer 定位该任务） | I-06 |
| 操作区（admin only；readonly 整区不渲染） | 未装：`安装`（primary；depsReady=false 时禁用 + tooltip 列出缺失项 + 环境管理链接）；已装：`打开`（primary，4.4）+ `升级` + `卸载`（error 色）+ `轮换凭据`（**仅 mongo_express 渲染**，phpmyadmin 无轮换入口 D-08/DBT_009） | 在途任务时变更按钮禁用（I-06） |

**安装确认**（分工具形态）：

- **phpMyAdmin**（NPopconfirm）：文案「将下载 phpMyAdmin {catalogVersion} 官方包（sha256 校验，可能需要数分钟）并部署到面板管理目录，经面板域路径安全访问；需 PHP-FPM 环境与扩展就绪（缺失将拒绝并引导补装）」。提交 dbtoolInstall {toolType:'phpmyadmin'} → 成功 toast「任务已创建」+ 打开任务 Drawer 定位新任务。
- **mongo-express**（NModal 实例选择弹窗，字段数 <5 Dialog 形态）：NSelect 目标 Mongo 实例（数据源=跨模块 instanceList 筛选 engine=mongodb；选项呈现 identityKey + credentialsSet 状态；**credentialsSet=false 选项禁用 + tooltip「请先到数据库管理配置凭据」+ 链接 /databases**，DBT_006 前置）；说明文案「将 npm 安装 mongo-express {catalogVersion} 并创建本机服务（仅监听 127.0.0.1:8081），注入所选实例凭据（加密存储不回显）与面板访问凭据；需 Node 环境就绪」。提交 dbtoolInstall {toolType:'mongo_express', instanceId} → 任务 Drawer 定位。

**升级确认**（NPopconfirm）：文案「将升级 {displayName} 到目录版本 {catalogVersion}（phpMyAdmin 保留配置与密钥；mongo-express 重装并重启服务，凭据与访问路径不变）」。提交 dbtoolUpgrade → 任务 Drawer；DBT_011 → toast「当前已为目录版本，无需升级」。

**卸载强确认**（NModal，两段警示 ⚠️A5/D-15）：

- 警示一（功能影响）：NAlert warning「卸载后 {displayName} 高级管理入口将不可用；面板内表/文档浏览（轻量入口）不受影响」；
- 警示二（数据保全）：NAlert info「**卸载仅解除工具部署（程序/服务/站点配置/访问凭据），绝不删除数据库数据**（沿实例移除哲学）」；
- 操作锁定：NInput 输入工具显示名确认（高危双因子，沿 env-mgmt 卸载先例）；
- 提交文案明示「将执行工具卸载（不删除数据库数据）」→ dbtoolUninstall → 任务 Drawer。

**轮换凭据确认**（NPopconfirm，仅 mongo-express）：文案「将重新生成 mongo-express 访问口令（旧口令即时失效）；浏览器无感知（服务端注入），无需重新登录」。提交 dbtoolRotate → 成功 toast「凭据已轮换」（rotatedAt 呈现，**无口令字段** I-02）。

### 4.2 任务列表（tool-list 任务 Drawer「历史」Tab）

**查询条件**（对应 api task/list query 参数）：

| 条件 | 字段 | 组件 | 查询方式 |
|------|------|------|---------|
| 工具 | toolType | NSelect（db_tool_type，含「全部」） | eq |
| 状态 | status | NSelect（db_tool_task_status，含「全部」） | eq |

**表格列**（NDataTable）：

| 列 | 字段 | 组件 | 宽度 | 配置 |
|----|------|------|------|------|
| 序号 | — | index | 60 | — |
| ID | id | link | 70 | 点击打开任务详情（4.3） |
| 工具 | toolType | dict db_tool_type | 130 | displayName 渲染 |
| 动作 | action | dict db_tool_task_action | 90 | install=「安装」/upgrade=「升级」/uninstall=「卸载」 |
| 状态 | status | dict db_tool_task_status | 110 | colorTag；failed 行附 error NTooltip |
| 实例 | instanceId | text | 90 | null 显示 —（mongo-express 安装溯源） |
| 发起者 | requestedBy | text | 100 | — |
| 发起时间 | createdAt | text | 170 | 本地时区 |
| 结束时间 | finishedAt | text | 170 | 未结束「—」 |

**刷新纪律**：进入 Tab 拉取一次（无自动轮询，手动刷新 + 写后刷新——env-mgmt 4.2 同口径）；在途任务存在时 Drawer 顶部 NAlert「有任务进行中，手动刷新查看进度」。

### 4.3 任务详情（tool-list 任务 Drawer「详情」区，宿主 tool-list）

**打开方式**：任务列表行 id link / 工具卡片在途标记「查看」/ 安装·升级·卸载发起成功后自动定位。

**内容**（dbtoolTaskDetail）：

| 区块 | 内容 |
|------|------|
| 概要 | toolType（displayName）/action/status（colorTag）/instanceId（null=—）/requestedBy/createdAt/startedAt/finishedAt |
| 失败原因 | error 非空 → NAlert error（sha256_mismatch/下载超时/npm 失败/落点冲突/面板重启中断如实呈现，D-04/D-05/I-09） |
| 命令输出尾部 | outputTail 只读等宽文本块（≤200 行，D-13）；running 态伴随 Spin +「执行中」 |
| 刷新 | 手动刷新按钮；**running/pending 态自动轮询 5s**（终态即停，onUnmounted 清理——env-mgmt/cron-backup 详情 Drawer 同构先例）；**终态跃迁触发工具全景与任务清单重拉**（任务完成后 installed/installedVersion/running 态与在途标记落定——env-mgmt v1.2 先例） |

**readonly 视角差异**：任务列表与详情同等可看（ADR-018），无操作按钮。

### 4.4 打开工具分发（dbtoolOpen，admin 专属）

**打开方式裁定（关闭 prd 待对齐-6）：新标签页**（`window.open(data.url, '_blank', 'noopener')`），**不内嵌 iframe**。理由：①工具为独立 Web 应用（phpMyAdmin/mongo-express 自有会话与导航），iframe 内接受工具 X-Frame-Options/CSP 限制且会话 cookie 第三方上下文受限；②新标签页同源（面板域工具路径），gating cookie scope 覆盖、signon 令牌 sessionDigest 绑定成立（api D-09）；③面板会话 idle 超时语义保持（工具内停留不续期面板会话，api D-11——超时后工具路径 gating 拒绝，回面板刷新即恢复）。

**分发流程**（点击卡片「打开」/ database-mgmt 高级入口）：

| 工具 | 流程 |
|------|------|
| phpmyadmin | 需 instanceId（卡片打开弹窗 NSelect 选择目标 MySQL/MariaDB 实例——跨模块 instanceList 筛选 engine∈mysql/mariadb + credentialsSet 就绪判定，同 4.1 安装选择器口径；**单实例时自动选中免弹窗**）→ dbtoolOpen {toolType:'phpmyadmin', instanceId} → 响应 {url, tokenExpiresAt} → window.open(url)（url 附 signon 令牌，60s 时效，浏览器经 gating 进入 phpMyAdmin 自动登录）；tokenExpiresAt 呈现于 toast「已打开（令牌 60 秒内有效）」 |
| mongo_express | dbtoolOpen {toolType:'mongo_express'}（instanceId 部署时已绑定，无需选择；卡片已呈现绑定实例，高级入口绑定校验见 database-mgmt PageSpec 6.8）→ 响应 {url, tokenExpiresAt:null} → window.open(url)（gating + basic auth 服务端注入，浏览器无感知直接进工具） |

**失败处理**：DBT_004（未装）→ 引导安装弹窗；DBT_006（实例未就绪）→「到数据库管理配置凭据」链接；DBT_012（运行态未就绪 data.reason）→「工具服务未运行，请先安装/升级或查看任务」提示；**signon 拒绝（DBT_007/008/010）发生在工具侧服务端回调**——浏览器见 phpMyAdmin 登录失败页，面板侧指引「重新打开工具以签发新令牌」（令牌一次性/短时效，重开即新令牌，api D-09）。

**凭据零回显**：打开响应仅 url + tokenExpiresAt（**无凭据字段**）；signon 令牌经 url 片段下发给 admin 浏览器用于 signon 重定向（api D-09 口径——令牌非凭据，凭据永不进浏览器）；轮换响应无口令字段（I-02）。

**工具内会话过期重认证（v1.1 W3，dbtoolReopen 二次流程）**：phpMyAdmin 自身会话过期后，工具内操作触发 phpMyAdmin 302 GET SignonURL=`{面板域}{entryPath}/dbtool/access/reopen?instanceId={id}`（api D-06/D-09）——浏览器顶层导航携面板会话 cookie，reopen 经 admin 会话 gating 重签新令牌后 302 回工具 URL（附新令牌片段），SignonScript 再注入凭据，**admin 无感重新登录**（无需回面板手动重开）。**前端无 reopen 按钮/路由**（后端 GET 重定向落地，非 SPA 页面）；面板会话已过期/吊销时 reopen 302 回面板登录入口（重认证后重新打开），readonly/实例未就绪等 302 回 /db-tools 呈现引导（api 4.1.4 失败呈现）。主打开流程（dbtoolOpen→window.open）不变。

## 第 5 节 表单页

无独立表单路由页——安装确认（NPopconfirm/NModal 实例选择）、卸载强确认（NModal）、升级/轮换确认（NPopconfirm）、打开实例选择（NModal/NSelect）均为即时动作 Dialog 形态（无草稿态）。

## 第 6 节 详情页

无独立详情路由页——任务详情=Drawer 详情区（4.3，hybrid 宿主 tool-list）。

## 第 7 节 路由定义

| 路由 | 页面 | meta |
|------|------|------|
| /db-tools | tool-list | `{requiresAuth:true, title:'数据库工具'}`（readonly 只读视图） |

**守卫顺序**（与全局框架一致）：sessionCurrent 探测 → roles 校验（两角色均可达页面；unauthorized → 403 页）。**工具本体路径（/dbtool/phpmyadmin/、/dbtool/mongo-express/）非面板路由**——nginx 工具 location 经 gating 直达工具上游（api D-03/4.4），readonly/无会话经 gating 401 不可达（⚠️A10，前端无路由守卫承载——网络层闸门）。**/dbtool/access/reopen 亦非 SPA 路由**（v1.1 W3）——后端 GET 重定向落地端点（phpMyAdmin SignonURL 302 目标，admin 会话 gating，api 4.1.4），前端无对应页面/守卫。

**侧边菜单（本模块贡献）**：数据库工具（/db-tools，Order 位于数据库管理之后——构建期登记 D-P18/api-panel-core）；两角色同项（readonly 只读视图）。

## 第 8 节 全局一致性

| # | 检查 | 结果 |
|---|------|------|
| 1 | Action ↔ api-db-tools.md v1.2 面板接口一一对应（S7） | ✅ 9/9 全映射，无虚构（v1.1 W3 +dbtoolReopen GET 重定向落地；内部端点 2 非前端 Action，服务端调用方 nginx/PHP-FPM，第 2 节声明） |
| 2 | 字典 Key ↔ 04-business-enum.md 定稿枚举一一对应（S8） | ✅ 5/5（db_tool_type/db_tool_task_action/db_tool_task_status/db_engine〔引用〕/panel_role，取值逐字对齐定稿） |
| 3 | 所有 POST Action 均携带 X-CSRF-Token（拦截器统一注入） | ✅ |
| 4 | ADR-018 + ⚠️A10 边界：readonly 操作区/打开/轮换不渲染 + 后端 AUTH_008 双保险 + 工具路径 gating 网络层闸门 | ✅ 4.1/4.4/第 7 节 |
| 5 | 错误文案与 api 3.4 错误码一一对应（DBT_001~012/AUTH 族；007/008/010 内部端点前端不直接消费已声明） | ✅ 第 1 节错误处理 |
| 6 | 查询条件仅含 API 真实参数（任务列表 toolType/status；工具全景无筛选）——不虚构 | ✅ 4.2 |
| 7 | 字典数据源偏差（无 dict/batch，前端常量）已声明 | ✅ 第 3 节 |
| 8 | 实况如实呈现：部署状态=档案存在性、运行态三态（true/false/null 不虚构）、依赖就绪度清单；面板重启中断任务如实展示 | ✅ 4.1/4.3（D-02/I-09） |
| 9 | ⚠️A5 卸载强确认两段警示 + 操作锁定 + **不删数据文案**；安装依赖前置禁用态（DBT_001 数据源） | ✅ 4.1 |
| 10 | 凭据零回显（I-02）：打开/轮换响应无凭据字段；signon 令牌非凭据（url 片段下发 admin）；实例选择器 credentialsSet 就绪判定 | ✅ 4.1/4.4 |
| 11 | 时间展示本地时区、提交 ISO UTC；generatedAt 投影时刻呈现（D-02）；DTO 可空显式 null 呈现（installedVersion/boundInstanceId/running/instanceId/finishedAt/error/tokenExpiresAt） | ✅ 4.1/4.2/4.3/4.4 |
| 12 | 打开方式裁定=新标签页（非内嵌），理由与 signon 绑定/gating cookie scope 一致性 | ✅ 4.4（关闭 prd 待对齐-6） |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-19 | initial | 首次生成：1 hybrid 页面（工具卡片网格两条目 + 任务 Drawer 双区）+ 8 Action（内部端点 2 非前端 Action 声明）、5 字典 Key、0 独立表单路由/0 独立详情路由；安装分工具形态（phpMyAdmin NPopconfirm/mongo-express NModal 实例选择〔跨模块 instanceList 引用〕）、卸载强确认 NModal（两段警示+输入锁定+**不删数据文案**）、升级/轮换 NPopconfirm；**打开裁定=新标签页**（window.open noopener，signon/gating 分发，关闭 prd 待对齐-6）；任务详情 running 态 5s 轮询终态即停 + 终态跃迁重拉全景（env-mgmt 先例）；跨模块高级入口被 database-mgmt PageSpec v1.5 消费（dbtoolList/dbtoolOpen） |
| v1.1 | 2026-11-19 | interface_added | Gate 1 W3 修复轮（数据源 api v1.2）：+dbtoolReopen Action（GET /dbtool/access/reopen，phpMyAdmin SignonURL 重认证落地——浏览器 302 顶层导航非 fetch Action，admin 会话 gating 重签令牌回工具 URL）；S7 8/8→9/9（面板面 9 接口，内部端点 2 不变，11 端点=9 前端 Action+2 内部）；4.4 +工具内 PMA 会话过期重认证二次流程注记（前端无 reopen 按钮/路由）；第 7 节 +reopen 非 SPA 路由声明；readonly 负空间同步 +dbtoolReopen（admin 专属 ⚠️A10）；字典/路由/表单零变更 |

> FX 质量清单说明：FX-001~FX-141 逐条清单文件未随引擎 payload 发布，本节按分册 9 节结构逐节落实，并以第 8 节 12 项全局一致性自查替代可机械执行部分（全仓先例）；全量 141 项留待 Gate 1 validation 执行。
