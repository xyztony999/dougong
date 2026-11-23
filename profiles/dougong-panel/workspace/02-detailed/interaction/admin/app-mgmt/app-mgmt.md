# PageSpec · app-mgmt · 管理员（admin）

| 属性 | 值 |
|------|-----|
| 版本 | v1.1 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-23（v1.1 宝塔式重组 D4：独立页面改为网站管理页内嵌面板（按运行时实例化）；创建表单去掉容器运行、增 Node 启动方式；Node 就绪度提示） |
| 用户类型 | admin |
| 唯一数据源 | 02-detailed/api/api-app-mgmt.md v1.0 |
| 组件体系 | Vue 3 + Naive UI（NGrid/NCard/NTag/NButton/NModal/NPopconfirm/NDrawer/NAlert/NSwitch/NSpin/NInputNumber/NSelect/NDataTable） |
| 确认方式 | headless run：7 项交互确认依据 D3 契约（api-app-mgmt.md D-01~D-15）+ ⚠️A1~A12 + FL v1.0 + PRD FR-01~12 推导，标注于各节；呈现裁定沿 env-mgmt/db-tools 先例（任务 Drawer 5s 轮询终态即停、强确认 NModal 输入锁定、实况三态如实） |

## 第 1 节 模块元信息

| 项 | 值 | 确认依据 |
|----|-----|---------|
| 模块编码 | app-mgmt（域前缀 /app） | api 3.1 |
| 业务类型 | master_data 子形态（app 档案 + app_task 四态任务状态机；实况投影） | 分册 2.1 判据；api 1.4 功能类型判定 |
| 页面类型 | app-list = **hybrid**（应用列表 + 创建向导 Modal + 应用详情运维 Drawer + 任务 Drawer 双区） | 分册 2.2；env-mgmt 详情 Drawer 先例 |
| UI 框架 | Naive UI | 栈版本表 |
| 状态管理 | Pinia：appStore（应用投影缓存/generatedAt/就绪度/刷新、任务列表/当前任务详情、详情运维态） | 栈版本表 |
| 错误处理 | 统一拦截器按 api 3.4 code 分流：AUTH_001/002→跳登录、AUTH_008→边界原因提示（含 log/env 只读越界 ⚠️A8）；APP_001/002→创建表单内联错误；APP_003→「目录或入口文件不存在」+文件管理器指引；APP_004→就绪度分层缺失提示+「到环境管理安装 Python」链接（data.missing）；APP_005→「端口已被占用」（data.port）；APP_006→「该应用有任务进行中」+查看任务引导（data.taskId）；APP_007/008→刷新提示；APP_009→「运行环境（venv）缺失」；APP_010→失败原因（data.reason）；APP_011→建站失败原因（data.reason）；APP_012→「文件已被外部修改，请刷新」/白名单不通过提示（data.reason） | api 3.4 错误码表 |

**7 项交互确认**（headless 推导）：① 元信息见本节；② 查询条件见 4.2（任务列表 action/status/appId 筛选；应用列表无筛选 API 参数——全量返回，诚实不虚构）；③ 表格列见 4.1；④ operList 见 4.1；⑤ 表单分组见 4.1 创建向导（两 Card 分组）；⑥ 详情 Card 见 4.3 应用运维 Drawer + 4.4 任务 Drawer；⑦ 页面类型覆盖：app-list=hybrid。

## 第 1a 节 v1.1 增量（容器化应用双形态，2026-11-22）

创建向导 Modal 扩应用形态二选一：venv 运行（既有字段组）或容器运行（镜像必填/端口映射/环境变量/挂载/重启策略；无代码目录/入口/Python 就绪度校验）；容器化形态由面板生成 compose 编排并启动（经 docker-mgmt 通道，带归属标签）；「建站」操作对容器化形态同等适用（反代到容器映射端口）。详表见第 2 节以下各节沿用。

## 第 2 节 Action 定义（与 API 一一对应）

| Action | 方法 | 路径 | 页面 | 权限 | CSRF |
|--------|------|------|------|------|------|
| appList | GET | /app/app/list | app-list | admin+readonly | — |
| appCreate | POST | /app/app/create | app-list（创建向导 Modal） | admin | ✓ |
| appDelete | POST | /app/app/delete | app-list（删除强确认） | admin | ✓ |
| appEnvGet | GET | /app/app/env | 详情运维 Drawer（参数 Tab） | admin | — |
| appParamsSave | POST | /app/app/params | 详情运维 Drawer（参数保存） | admin | ✓ |
| appDepsReinstall | POST | /app/app/deps | 应用列表（重装依赖）+ 详情运维 Drawer | admin | ✓ |
| appTaskList | GET | /app/task/list | 任务 Drawer 历史 Tab | admin+readonly | — |
| appTaskDetail | GET | /app/task/detail | 任务 Drawer 详情区 | admin+readonly | — |
| appTaskRetry | POST | /app/task/retry | 任务 Drawer 详情区（重试按钮） | admin | ✓ |
| appServiceStart | POST | /app/service/start | 应用列表操作区 | admin | ✓ |
| appServiceStop | POST | /app/service/stop | 应用列表操作区 | admin | ✓ |
| appServiceRestart | POST | /app/service/restart | 应用列表操作区 | admin | ✓ |
| appServiceEnable | POST | /app/service/enable | 应用列表操作区（自启开关） | admin | ✓ |
| appServiceDisable | POST | /app/service/disable | 应用列表操作区（自启开关） | admin | ✓ |
| appLogGet | GET | /app/log | 详情运维 Drawer（日志 Tab） | **admin 专属** | — |
| appSiteLink | POST | /app/site/link | 详情运维 Drawer（建站联动） | admin | ✓ |

> S7 一致性：16 Action 与 api-app-mgmt.md 16 接口一一对应（无未覆盖接口、无虚构接口）。

## 第 3 节 字典 Key 清单

| Key | 取值（=01-solution/04-business-enum.md 定稿） | 用途 |
|-----|--------------------------------------------|------|
| app_task_action | create / deps_install / delete | 任务动作 Tag（Drawer 渲染「创建应用」/「安装依赖」/「删除应用」） |
| app_task_status | pending / running / succeeded / failed | 任务状态 Tag（colorTag）+筛选+retryable 按钮 |
| app_start_mode | preset_fastapi / custom | 启动方式（创建向导 NSelect + 列表呈现「FastAPI 预设」/「自定义命令」） |
| env_type | （引用，九值） | 就绪度引导呈现（python 项；创建拒绝提示上下文） |
| panel_role | admin / readonly | 角色判断（按钮可见性） |

**数据源规范偏差声明**：不实现 `POST /{domain}/dict/batch`（分册 3.6）——枚举全部固定值，字典由前端 TS 常量提供（单一事实源 04-business-enum.md；全仓既有偏差先例）。

**状态颜色方案**（分册 3.1 语义映射，沿 env-mgmt 先例）：

| Key::值 | 颜色 |
|---------|------|
| app_task_status::pending | gray |
| app_task_status::running | blue（执行中，Spin 伴随） |
| app_task_status::succeeded | green |
| app_task_status::failed | red |
| 服务运行 running | green |
| 服务运行 已停止/failed | gray/orange（failed=服务异常橙底+查看日志引导） |
| 自启 on/off | green / gray |
| 就绪度 installed+venvSupport+pipAvailable 全 true | green「Python 环境就绪」 |
| 任一 false | orange「Python 环境不完整」+缺失项明细 |
| 探测失败（null） | gray 不虚构 |
| 依赖安装 depsInstalled true/false | green「已装」/ gray「未装」 |

## 第 4 节 列表页

### 4.1 应用管理（app-list，hybrid）— 路由 /app

**权限**：admin+readonly 可见（ADR-018 全可看）；全部变更按钮与 env/log 读取 admin 专属。

**页头**：Python 就绪度状态条（pythonReadiness：全就绪 → NTag green「Python 环境就绪（{version}）」；缺失 → NTag orange「Python 环境不完整：缺 {missing 项中文}」+「到环境管理安装」链接（/env）+ tooltip 说明 venv/pip 能力位；探测失败 → NTag gray「状态未探测」）+ 刷新按钮（重新 appList，D-01 实时探测）+ generatedAt「探测于 HH:mm:ss」。

**应用表格**（NDataTable，数据源 appList items[]；空态 NEmpty「还没有应用，点击右上角创建」+ 创建按钮引导）：

| 列 | 字段 | 组件 | 宽度 | 配置 |
|----|------|------|------|------|
| 序号 | — | index | 60 | — |
| 应用名 | name | link | 160 | fixed: left；点击打开应用运维 Drawer（4.3） |
| 启动方式 | startMode | dict app_start_mode | 110 | 「FastAPI 预设」/「自定义命令」 |
| 端口 | listenPort | text | 90 | — |
| 服务状态 | serviceRunning | 状态组 | 130 | running→NTag green「运行中」/false→gray「已停止」/null→gray「未探测」；failed 单元（unit 存在但服务 failed）→orange「服务异常」+tooltip「查看日志」引导 |
| 自启 | serviceEnabled | NTag | 80 | on→green「已自启」/off→gray「未自启」/null→「—」 |
| 依赖 | depsInstalled | NTag | 90 | true→green「已安装」/false→gray「未安装」/null→「—」；venv 缺失→orange「环境缺失」tooltip |
| 在途标记 | runningTaskId | NTag+链接 | 110 | 非空→blue「任务进行中」+「查看」（跳任务 Drawer 定位） |
| 站点 | hasSite | text+链接 | 130 | true→「已建站」+前往站点管理链接（/sites）；false→gray「仅本机可访问」tooltip「可经建站联动对外暴露」；null→「—」 |
| 操作列 | — | action | 300 | fixed: right；admin only（readonly 整列不渲染，ADR-018） |

**操作列按钮组**（admin；按钮态由实况驱动，在途任务时变更按钮禁用 I-05）：

| 按钮 | 条件 | 形态 | 说明 |
|------|------|------|------|
| 启动/停止/重启 | 服务实况驱动按钮集 | NPopconfirm | 运行中→停止/重启；已停止→启动；服务异常→启动/查看日志引导；瞬时通道不占任务互斥（D-10） |
| 自启开关 | — | NSwitch | enable/disable（D-10） |
| 重装依赖 | venv 存在（venvExists） | NPopconfirm「按 requirements.txt 重新安装依赖，需重启应用生效」 | venv 缺失→禁用+tooltip；完成 toast「需重启应用生效」（D-09 语义） |
| 参数 | — | NButton | 打开运维 Drawer 参数 Tab |
| 日志 | — | NButton | 打开运维 Drawer 日志 Tab（admin 专属 ⚠️A8） |
| 建站 | — | NButton/NTag | 未建站→「建站暴露」打开联动 Modal；已建站→「已建站」禁用+前往站点管理 |
| 删除 | 无在途任务 | NButton error | 打开删除强确认 Modal（I-05 在途禁用） |

**创建向导**（NModal，两分组 Card；提交 appCreate）：

| 分组 | 字段 | 组件 | 校验/说明 |
|------|------|------|----------|
| 基本信息 | 应用名 | NInput | placeholder「小写字母、数字、连字符（≤32 字符）」；APP_001/002 内联错误 |
| 基本信息 | 代码目录 | NInput | placeholder「/opt/myapp」；APP_003 内联错误 + 「经文件管理器上传代码」软链接 |
| 基本信息 | 入口文件 | NInput | placeholder「app.py（相对代码目录）」；APP_003 |
| 基本信息 | 监听端口 | NInputNumber | 1024-65535；默认仅本机可访问说明（⚠️A4）；APP_005 |
| 启动配置 | 启动方式 | NSelect（app_start_mode） | 「FastAPI 预设（uvicorn）」/「自定义命令」 |
| 启动配置 | 模块:实例（条件显隐 preset_fastapi） | NInput | placeholder「app:app」；渲染 uvicorn 启动命令模板预览行（灰字） |
| 启动配置 | 自定义命令（条件显隐 custom） | NInput | placeholder「.venv/bin/你的启动命令」；说明「须以 .venv 内相对路径开头」（APP_012 白名单） |
| 启动配置 | 立即启动 | NSwitch | 默认关（⚠️A5 语义——创建后不强制启动） |

提交成功 → toast「创建任务已发起」+ 打开任务 Drawer 定位新任务（taskId）；失败 APP_004 → 模内 NAlert 分层缺失 + 环境管理链接。

**删除强确认**（NModal 两段警示 ⚠️A5/D-07）：

- 警示一：NAlert warning「将停止服务并移除系统服务条目；代码目录默认保留」；
- 警示二：NSwitch「同时删除代码目录（含运行环境，不可恢复）」默认关 + 已建站提示 NAlert info「关联站点不会自动删除，请到站点管理自行处置」；
- 操作锁定：NInput 输入应用名确认（高危双因子先例）；
- 提交 appDelete（purgeDir=开关值）→ 任务 Drawer 定位。

### 4.2 任务列表（任务 Drawer「历史」Tab）

**查询条件**：

| 条件 | 字段 | 组件 | 查询方式 |
|------|------|------|---------|
| 应用 | appId | NSelect（应用清单，含「全部」） | eq |
| 动作 | action | NSelect（app_task_action，含「全部」） | eq |
| 状态 | status | NSelect（app_task_status，含「全部」） | eq |

**表格列**：序号/ID（link→详情）/应用（appName）/动作（dict app_task_action）/状态（colorTag+failed error NTooltip）/范围（purgeDir 仅 delete 行「含删目录」orange 角标）/发起者/发起时间/结束时间。

**刷新纪律**：进入 Tab 拉取一次（无自动轮询，手动刷新+写后刷新——env-mgmt 先例）；在途任务 Drawer 顶部 NAlert 提示手动刷新。

### 4.3 应用运维 Drawer（宿主 app-list，行应用名 link 打开；三 Tab）

| Tab | 内容 | 数据源/Action |
|-----|------|--------------|
| 概要 | 应用名/启动方式/端口/代码目录/入口/服务状态/自启/依赖就绪度/venv 存在/端口监听（portListening 三态）/创建者/创建时间 | appList 投影 |
| 参数（admin） | .env 行编辑（NInput textarea 逐行，KEY=VALUE 渲染）+ 启动参数段（preset=模块:实例/参数输入；custom=命令输入）+ 保存（appParamsSave；APP_012 基线不一致→NAlert warning「文件已被外部修改」+刷新按钮；保存成功→toast「已保存，需重启应用生效」不自动重启）+ NAlert info「.env 与启动参数保存后需重启生效」 | appEnvGet/appParamsSave |
| 日志（admin） | 档位 NSelect（200/500/1000 行，D-11）+ 只读等宽文本块 + 手动刷新；running 服务提示「日志实时跟随请使用外部终端或刷新查看」；LOG 读取失败如实呈现 | appLogGet |

### 4.4 任务详情（任务 Drawer「详情」区）

**打开方式**：任务列表行 ID link / 列表在途标记「查看」/ 创建·删除·依赖发起成功后自动定位。

| 区块 | 内容 |
|------|------|
| 概要 | appName/action（dict）/status（colorTag）/purgeDir（delete 行呈现删除范围）/requestedBy/createdAt/startedAt/finishedAt |
| 失败原因 | error 非空 → NAlert error（含 data.reason/timeout/面板重启中断如实呈现，I-08） |
| 重试 | retryable=true（failed 且 action∈{create,deps_install}，api retryable 字段）→ NButton warning「重试」（appTaskRetry；delete 不可重试无按钮——D-13）；重试成功 toast + 详情回 running 轮询 |
| 命令输出尾部 | outputTail 只读等宽文本块（≤200 行）；running 态伴随 Spin |
| 刷新 | 手动刷新；running/pending 态自动轮询 5s（终态即停，onUnmounted 清理——env-mgmt 先例）；**终态跃迁触发应用全景与任务清单重拉**（创建成功→列表新行落定/删除成功→行消失） |

**readonly 视角差异**：任务列表与详情同等可看（ADR-018）；重试按钮不渲染；概要/输出同等呈现。

## 第 5 节 表单页

无独立表单路由页——创建向导（NModal 两分组）、删除强确认（NModal）、服务操作/依赖（NPopconfirm）、建站联动（NModal：域名输入可选+说明「域名绑定与 HTTPS 请到站点管理配置」）均为即时 Dialog 形态（无草稿态）。

## 第 6 节 详情页

无独立详情路由页——应用运维=Drawer 三 Tab（4.3）、任务详情=Drawer 详情区（4.4，hybrid 宿主 app-list）。

## 第 7 节 路由定义

| 路由 | 页面 | meta |
|------|------|------|
| /app | app-list | `{requiresAuth:true, title:'应用管理'}`（readonly 只读视图） |

**守卫顺序**（与全局框架一致）：sessionCurrent 探测 → roles 校验（两角色均可达；unauthorized → 403 页）。

**侧边菜单（本模块贡献）**：应用管理（/app，Order 位于环境管理之后——构建期登记 D-P18/api-panel-core v2.2 形态）；两角色同项。

## 第 8 节 全局一致性

| # | 检查 | 结果 |
|---|------|------|
| 1 | Action ↔ api-app-mgmt.md 接口一一对应（S7） | ✅ 16/16 全映射，无虚构 |
| 2 | 字典 Key ↔ 04-business-enum.md 定稿枚举一一对应（S8） | ✅ 5/5（app_task_action/app_task_status/app_start_mode〔三值 D2 定稿〕/env_type〔引用〕/panel_role，取值逐字对齐定稿） |
| 3 | 所有 POST Action 均携带 X-CSRF-Token（拦截器统一注入） | ✅ |
| 4 | ADR-018 边界：readonly 操作列/创建/删除/重试/建站/参数/日志入口不渲染 + 后端 AUTH_008 双保险（log/env 读取面 admin 专属 ⚠️A8） | ✅ 4.1/4.3 |
| 5 | 错误文案与 api 3.4 错误码一一对应（APP_001~012/AUTH 族） | ✅ 第 1 节错误处理 |
| 6 | 查询条件仅含 API 真实参数（任务列表 appId/action/status；应用列表无筛选）——不虚构 | ✅ 4.2 |
| 7 | 字典数据源偏差（无 dict/batch，前端常量）已声明 | ✅ 第 3 节 |
| 8 | 实况如实呈现：服务态/依赖/端口监听三态（含 null「未探测」）≠ 混同；面板重启中断任务如实展示；就绪度分层呈现（缺 python/venv/pip 明细） | ✅ 4.1/4.3（D-01/D-08） |
| 9 | ⚠️A5 删除强确认两段警示+范围开关默认保留+操作锁定；创建「立即启动」默认关；⚠️A10 参数保存后提示重启不自动重启 | ✅ 4.1 |
| 10 | 时间展示本地时区、提交 ISO UTC；generatedAt 投影时刻呈现（D-01） | ✅ 4.1 |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-20 | initial | 首次生成：1 hybrid 页面（应用表格 + 创建向导 Modal + 应用运维 Drawer 三 Tab〔概要/参数/日志〕+ 任务 Drawer 双区）+ 16 Action（13 admin/3 两角色）、5 字典 Key、0 独立表单路由/0 独立详情路由；创建向导两分组（基本信息/启动配置，mode 条件显隐）；删除强确认 NModal（两段警示+范围开关默认保留+输入锁定）；服务操作组实况驱动；任务 retry 按钮态（retryable）；详情 running 5s 轮询终态即停+终态跃迁重拉（env-mgmt 先例）；UI 文案人话化（「创建应用/重装依赖/开机自启/建站暴露」，无内部术语） |
| v1.1 | 2026-11-23 | 增量 | 宝塔式重组 D4（ADR-033）：应用管理不再独立成页——原页面改为可嵌入面板，由网站管理页「Python 项目」「Node 项目」分栏各实例化一次；路由 /app 重定向 /sites?tab=python，侧栏菜单项移除；创建表单去掉「容器运行」，Node 运行时下启动方式为启动文件 / npm 脚本；Action 无新增（appCreate 请求增 runtime、appList 可按运行时过滤） |

> FX 质量清单说明：FX-001~FX-141 逐条清单文件未随引擎 payload 发布，本节按分册 9 节结构逐节落实，并以第 8 节 10 项全局一致性自查替代可机械执行部分（全仓先例）；全量 141 项留待 Gate 1 validation 执行。

## 增量 v1.1（2026-11-23）：宝塔式重组 D4——项目面板

> 依据 baota-restructure-design-2026-11-23.md 第二、三、四、九章。本节是权威增量。

- **不再是独立页面**：原页面改为可嵌入的面板组件（属性 runtime：python 或 node），由网站管理页的两个分栏各实例化一次；路由 /app 重定向到 /sites?tab=python；侧栏「应用管理」菜单项移除。
- **列表**：仅呈现当前运行时的项目；列与第 4 节既有相同。「启动方式」列取值随运行时——Python：FastAPI 预设 / 自定义命令；Node：启动文件 / npm 脚本。
- **创建弹窗**：去掉「容器运行」选项。
  - Python 分栏：同第 5 节既有（启动方式二选一：FastAPI 预设 / 自定义命令）。
  - Node 分栏：名称 / 代码目录 / 监听端口 / 启动方式（启动文件或 npm 脚本）/ 启动文件（启动文件方式；占位「如 server.js、bin/www」）/ 脚本名（npm 脚本方式；缺省 start）/ 立即启动开关；表单内说明「项目里有 package.json 时会自动安装依赖」。
- **就绪度提示**：Node 分栏顶部在 Node 环境不完整（缺 node 或 npm）时呈现提示「Node 环境不完整：缺 node」「缺 npm」并列出缺失项，附「前往环境管理」链接；创建提交被拒（APP_015）时就地提示。
- **任务与运维**：任务抽屉、服务操作、参数、日志、建站、删除对两种运行时共用同一套界面；「重装依赖」在 Node 项目上执行 npm install，确认文案注明。
- **Action**：无新增；appCreate 请求体增 runtime；appList 可带 runtime 过滤。
