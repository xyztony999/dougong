# PageSpec · 容器管理（docker-mgmt） · 管理员（admin）

| 属性 | 值 |
|------|-----|
| 版本 | v1.2 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-23（v1.2 宝塔式重组 D4：新增「应用」页签（清单/创建/启停/日志/建站/删除）；Compose 页签呈现未运行项目并新增删除项目；Action 28→35） |
| 用户类型 | admin（readonly 视角为本文档子集，差异见 ../readonly/docker-mgmt/docker-mgmt.md） |
| 唯一数据源 | 02-detailed/api/api-docker-mgmt.md v1.1 |
| 组件体系 | Vue 3 + Naive UI（NTabs/NDataTable/NSelect/NInput/NButton/NTag/NPopconfirm/NModal/NAlert/NEmpty/NDrawer/NSwitch/NSpin/NTooltip；日志跟随为滚动文本流呈现，非终端仿真——不引入 xterm.js，不入 code 型；无激活栈 profile，不使用 vue3-engine 栈 Scm\* 组件） |
| 确认方式 | headless run：7 项交互确认依据 D3 契约（api-docker-mgmt.md D-01~D-11 + I-01~I-09）+ ADR-018 + BA 附录B 矩阵 + FL v1.0 推导，标注于各节 |

## 第 1 节 模块元信息

| 项 | 值 | 确认依据 |
|----|-----|---------|
| 模块编码 | docker-mgmt（域前缀 /docker） | api 3.1 |
| 业务类型 | **工具类**（分册 2.1 三型之外的诚实标注：无聚合根、无 MQ——D2/D3 判定；五类投影清单 + 13 写接口，无面板自持状态机无审批） | api 1.4/4.1.2；domain-model.md docker-mgmt 域 |
| 页面类型 | **hybrid**（列表页 Tab 切换：容器 / Compose / 镜像 / 卷 / 网络 五 Tab → 分册 2.2「列表页有 Tab 切换」触发 hybrid，layout: 'tab'；无左树右表/图表/拖拽，不触发 code——日志跟随为流式滚动文本非终端仿真） | 分册 2.2；FL 2.1.2.1~2.1.2.6 六模块菜单同页承载（idea.md §5.2 单一 views/docker 目录） |
| UI 框架 | Naive UI（NTabs/NDataTable/NDrawer/NModal/NPopconfirm/NSwitch 等） | 栈版本表 |
| 状态管理 | Pinia：dockerStore（daemonStatus/containerItems/composeItems/imageItems/volumeItems/networkItems/generatedAt×5/listParams/inFlight×12/logDrawer 态/WS 订阅句柄）；枚举常量 store（与 account-auth/firewall 等共用 enums.ts） | 栈版本表 |
| 错误处理 | 统一拦截器按 api 3.4 code 分流：AUTH_001/002→跳登录（全局拦截器）、AUTH_003→CSRF 失败提示（全局）、AUTH_008→「权限不足：只读角色禁止容器管理变更操作」（全局边界拒绝，正常 UI 无入口不触发）、DK_001→正常 UI 不可达（控件与服务端权威面约束，R-14）兜底「参数非法」提示、DK_002→正常流程不可达（status 先行引导态，第 4.0 节）兜底跳引导态（api D-01/I-01）、DK_003→warning「该容器已不存在（可能已在面板外删除）」+ 自动重拉容器清单、DK_004→warning「容器当前状态（{currentState}）不允许该操作，可用：{allowedActions 中文串}」+ 重拉容器清单（服务端权威动作面为准，api D-04/I-04）、DK_005/DK_007/DK_009→warning「目标不存在（可能已在面板外删除）」+ 重拉对应清单、DK_006/DK_008/DK_010→warning「{镜像被容器引用/卷被容器挂载/网络被容器接入}，无法删除」+ data.reason、DK_011→warning「预定义网络（bridge/host/none 等）不可删除」、DK_012→拉取 Dialog 字段错误提示（ref 格式，R-14）、DK_013→error「镜像拉取失败：{data.reason}」（Dialog 保持打开）、DK_014→warning「该项目已不存在」+ 重拉 Compose 清单、DK_015→NAlert warning 呈现 label 记录路径 + 提示「可经容器页逐个操作兜底」（api D-06/I-06）、DK_016→error「compose 命令执行失败：{data.reason}」、DK_017→error「Docker 操作失败：{data.reason}」、DK_018→warning「该项目已有操作进行中，请稍候」（api D-06/I-08） | api 3.4 + D-01/D-04/D-05/D-06/D-08/I-01~I-08 |

## 第 2a 节 v1.1 新增 Action（创建类，2026-11-22）

| Action | 方法 | 路径 | 页面 | 权限 | CSRF |
|--------|------|------|------|------|------|
| containerCreate | POST | /docker/container/create | 容器 Tab（创建容器 Dialog） | admin | ✓ |
| volumeCreate | POST | /docker/volume/create | 卷 Tab（创建卷 Dialog） | admin | ✓ |
| networkCreate | POST | /docker/network/create | 网络 Tab（创建网络 Dialog） | admin | ✓ |
| composeCreate | POST | /docker/compose/create | Compose Tab（新建项目 Dialog） | admin | ✓ |

> readonly 四 Action 全部负空间不渲染（ADR-018）；admin 表格工具栏 +「创建」按钮（容器/卷/网络/Compose 四 Tab 各自入口）。
>
> ⚠️ 2026-11-23 更正：本节初版（56a3033）只登记了 Action 表，第 4.1/4.3/4.5/4.6 工具条描述与第 5 节弹窗规格未同步（工具条仍写「无新增按钮/仅刷新」）——实现据此漏做全部触发按钮，v0.3.0 交付断裂（弹窗无入口）。v1.1 已补 4.x 工具条动作与 5.5~5.8 四弹窗字段规格；「入口可达性」自此列入 Gate 2 硬性检查项（每动作：触发按钮→弹窗→Action→端点 四环逐项核验，缺一环即 FAIL）。

## 第 2 节 Action 定义（与 API 一一对应）

| Action | 方法 | 路径 | 页面 | 权限 | CSRF |
|--------|------|------|------|------|------|
| daemonStatus | GET | /docker/daemon/status | docker·页头状态 Card（含引导态判定） | admin+readonly | —（GET） |
| containerList | GET | /docker/container/list | docker·Tab 容器 | admin+readonly | —（GET） |
| containerLogs | GET | /docker/container/logs | docker·容器详情 Drawer·日志 Card | admin+readonly | —（GET） |
| containerLogsFollow | WS | /docker/container/logs-follow | docker·容器详情 Drawer·日志 Card（跟随开关） | admin+readonly | —（WS，Cookie 握手 + Origin） |
| containerStats | GET | /docker/container/stats | docker·容器详情 Drawer·资源占用 Card | admin+readonly | —（GET） |
| containerStart | POST | /docker/container/start | docker·Tab 容器·操作列 | admin | ✓ |
| containerStop | POST | /docker/container/stop | docker·Tab 容器·操作列 | admin | ✓ |
| containerRestart | POST | /docker/container/restart | docker·Tab 容器·操作列 | admin | ✓ |
| containerPause | POST | /docker/container/pause | docker·Tab 容器·操作列 | admin | ✓ |
| containerUnpause | POST | /docker/container/unpause | docker·Tab 容器·操作列 | admin | ✓ |
| containerDelete | POST | /docker/container/delete | docker·Tab 容器·操作列 | admin | ✓ |
| imageList | GET | /docker/image/list | docker·Tab 镜像 | admin+readonly | —（GET） |
| imagePull | POST | /docker/image/pull | docker·Tab 镜像·拉取 Dialog | admin | ✓ |
| imageDelete | POST | /docker/image/delete | docker·Tab 镜像·操作列 | admin | ✓ |
| volumeList | GET | /docker/volume/list | docker·Tab 卷 | admin+readonly | —（GET） |
| volumeDelete | POST | /docker/volume/delete | docker·Tab 卷·操作列 | admin | ✓ |
| networkList | GET | /docker/network/list | docker·Tab 网络 | admin+readonly | —（GET） |
| networkDelete | POST | /docker/network/delete | docker·Tab 网络·操作列 | admin | ✓ |
| composeList | GET | /docker/compose/list | docker·Tab Compose | admin+readonly | —（GET） |
| composeUp | POST | /docker/compose/up | docker·Tab Compose·操作列 | admin | ✓ |
| composeDown | POST | /docker/compose/down | docker·Tab Compose·操作列 | admin | ✓ |
| composeRestart | POST | /docker/compose/restart | docker·Tab Compose·操作列 | admin | ✓ |
| composeLogs | GET | /docker/compose/logs | docker·Compose 日志 Drawer | admin+readonly | —（GET） |
| composeLogsFollow | WS | /docker/compose/logs-follow | docker·Compose 日志 Drawer（跟随开关） | admin+readonly | —（WS，Cookie 握手 + Origin） |

> S7 一致性：24 Action 与 api-docker-mgmt.md 24 端点一一对应（22 HTTP + 2 WS，一接口一 Action，无未覆盖接口、无虚构接口）。CSRF Token 由全局拦截器统一注入 `X-CSRF-Token`（account-auth R-02 机制）；WS 端点豁免 CSRF（无变更语义，api 3.1）。

**刷新纪律**（确认项 2/7 交叉，api D-01「按需拉取」契约的前端落法；与 firewall D-01 同口径）：

| 触发 | 行为 |
|------|------|
| 进入页面 | daemonStatus 先行 → state=available 时激活五 Tab 懒加载（首次切 Tab 拉取对应清单，切回不重复拉取除非已标记过期） |
| 点击「刷新」按钮（页头/Tab 内） | 重拉 daemonStatus / 当前 Tab 清单（loading 态） |
| 容器六动作成功 | 重拉 containerList；**composeList 标记过期**（compose 项目容器实况随容器操作变化，Tab 切回 Compose 时重拉，api 4.3.1 containers 同源）；imageList/volumeList/networkList 不标记（引用解除不影响清单内容） |
| imagePull/imageDelete/volumeDelete/networkDelete 成功 | 重拉对应 Tab 清单；containerList 标记过期（镜像删除不影响容器、卷/网络删除不可达 in-use——保守标记仅在 delete 成功时，实际 in-use 被拒不触发） |
| composeUp/Down/Restart 成功 | 重拉 composeList + containerList（项目容器集合与状态已变） |
| Drawer 打开（容器详情） | containerStats + containerLogs（tail=500）按需拉取；「跟随」开启时建立 WS 订阅，关闭 Drawer 即断开（无残留订阅） |
| 无自动轮询 | 不设定时轮询（firewall/monitoring D-01 同口径；stats 为打开 Drawer 时的单次快照，api D-07；页面刷新节奏=手动+操作驱动，PRD 无自动刷新需求不虚构） |
| WS end 帧 | 停止跟随 + 按 reason 提示（第 5.3 节），不自动重连（用户手动重开跟随=重试入口） |
| `document.hidden` | 无定时器需要暂停；WS 跟随在页面隐藏期间保持（日志流为 daemon 实况，断开即止由用户控制） |
| 页面卸载 | 清理 Dialog/inFlight/过期标记/全部 WS 订阅 |

## 第 3 节 字典 Key 清单

| Key | 取值（=01-solution/04-business-enum.md 定稿） | 用途 |
|-----|--------------------------------------------|------|
| container_status | running / stopped / paused | 容器状态筛选下拉（多选）+ 容器表状态列 dict + colorTag；过渡态（restarting/removing）为透传原值非枚举成员（api D-02，与 service_enabled_state 透传口径同构） |
| docker_daemon_state | available / unavailable | 页头接管状态 Tag + 引导态判定（unavailable，api D-01） |
| compose_action | up / down / restart / logs | Compose 操作集权威口径——Tab Compose 操作列按钮组（up/down/restart）与日志入口（logs）的渲染依据（api D-06：枚举不作请求参数，路径动作段承载；非 dict 组件用途） |
| panel_role | admin / readonly | 菜单可见性判定（authStore 复用；本模块 readonly 为子集视角，差异见 readonly 文档） |

**数据源规范偏差声明**（同 account-auth/firewall/monitoring/service-mgmt 先例）：不实现 `POST /{domain}/dict/batch`（分册 3.6）——枚举全部固定值，字典由前端 TS 常量提供（enums.ts 单一事实源，共用同一文件）。此偏差为设计决策，待 Gate 1 复核。

**状态颜色方案**（分册状态颜色语义映射）：

| 值 | 颜色 | 语义依据 |
|----|------|---------|
| state=running | green | 运行中（分册「成功/进行正常」绿） |
| state=paused | orange | 已暂停（分册「异常/需注意」橙——冻结警示态，区别于 stopped） |
| state=stopped | gray | 已停止（分册「已关闭」灰） |
| 过渡态 restarting / removing（透传原值） | blue | 瞬时执行中（分册「进行中」蓝；操作入口禁用，api D-02 supportedActions=[]） |
| daemon=available / unavailable | green / orange | 接管可用 / 不可用（orange 驱动引导态，api D-01） |
| locatable=true / false（Compose 项目） | — / red | 正常 / 定位失败（红色警示 + DK_015 详情，api D-06） |

## 第 4 节 列表页（核心页，/docker，hybrid · 五 Tab 布局）

**权限**：admin 全功能；readonly 为子集（无拉取按钮、容器/镜像/卷/网络/Compose 操作列整列不渲染，见 readonly 文档）。

### 4.0 页头：daemon 接管状态 Card 与引导态（daemonStatus，确认项 1）

页头 Card 恒为页面第一区块（五 Tab 之上）：

| 元素 | 呈现 |
|------|------|
| 接管状态 | NTag（docker_daemon_state dict 颜色：available→green「已接管 Docker」/ unavailable→orange「Docker 不可用」） |
| 版本 | text（version=null →「—」，不虚构；available 时呈现「Docker {version}」） |
| 不可用细节 | unavailable 时灰字 detail（not_installed→「未检测到已安装的 Docker」/ socket_unreachable→「daemon socket 不可达」，透传属性，api D-01） |
| 刷新 | NButton（loading 防抖，重拉 daemonStatus + 当前 Tab 清单） |
| 数据生成于 | 灰字 {generatedAt 本地时区}（api generatedAt 契约） |

**引导态**（state=unavailable，api DK_002/I-01「未装≠无容器」契约的前端落法）：页头下方整页降级为 NEmpty 引导视图——「未检测到可用的 Docker daemon（{detail 文案}）。请安装 Docker 后刷新重试。」**不渲染五 Tab 与任何清单内容、不呈现空容器列表**（FL S-45-02；提示页不提供代为安装入口，PRD FR-02/OUT-9）；保留页头刷新按钮（安装后重试入口）。

### 4.1 Tab 1 容器（工具条 + 表格，确认项 2/3）

**工具条**（参数变更即触发 containerList）：

| 控件 | 组件 | 取值与校验 | 查询方式 |
|------|------|-----------|---------|
| 状态筛选 | NSelect multiple（clearable） | container_status 三值，空=全部 | 多值逗号拼接 query `state`（api：逗号分隔多值，含过渡态原始值；任一值非法→DK_001，正常 UI 下拉不可达） |
| Compose 项目筛选 | NInput（clearable） | 项目名精确输入 | query `composeProject`（api：等值匹配） |
| 刷新 | NButton（loading 防抖） | 以当前参数重拉 | — |

表格上方灰字「数据生成于 {generatedAt}」+「共 N 个容器」。**工具条含「创建容器」按钮**（primary，admin 专属，readonly 不渲染——v1.1 PRD OUT-7 已翻转，FR-19）→ 打开 5.5 创建容器 Dialog。

**表格列**（NDataTable）：

| 列 | 类型 | 宽度 | 说明 |
|----|------|------|------|
| 序号 | index | 60 | — |
| 名称 | link | 200 | items[].name 点击 → 容器详情 Drawer（第 5.1 节；详情跳转经名称列链接，分册 3.1） |
| 镜像 | text | 220 | items[].image（名称:tag） |
| 状态 | dict(container_status) + colorTag | 110 | items[].state（第 3 节颜色方案）；过渡态透传原值 Tag blue + Tooltip「操作进行中，完成后自动回落」 |
| Compose 项目 | text | 160 | items[].composeProject（null → 「—」灰字「独立容器」Tooltip；有值时呈现项目名，与 Tab Compose 同源，api D-02 label 透传） |
| 操作 | action | 300 fixed:right | 见 4.2（readonly 视角整列不渲染） |

无分页（全量返回 api D-02）、无排序控件（服务端名称字典序）；空结果 → NEmpty「当前无容器」（daemon 可用且无容器为常态，与引导态语义区分，api N2）；加载中 → 表格骨架。

### 4.2 Tab 1 操作列（operList，确认项 4）——supportedActions 权威渲染

**核心原则**（api D-02/D-04/I-04 的前端落法，与 firewall 动作面同构）：操作列按钮**由 items[].supportedActions 服务端权威面驱动渲染**（按当前态 × BA 附录B.2 矩阵计算），前端维护完整按钮集合与事件映射、不自行推断可用性：

| 按钮 | 可见性 | 二次确认 | 依据 |
|------|--------|---------|------|
| 启动 | supportedActions 含 start | 无（低风险） | api D-04 矩阵：start←stopped |
| 停止 | 含 stop | NPopconfirm「确认停止容器 {name}？」 | stop←running/paused |
| 重启 | 含 restart | NPopconfirm「确认重启容器 {name}？」 | restart←running/paused |
| 暂停 | 含 pause | NPopconfirm「确认暂停容器 {name}？暂停后进程冻结、资源保留。」 | pause←running（ADR-025 排障冻结场景） |
| 恢复 | 含 unpause | 无 | unpause←paused |
| 删除 | 含 delete | NPopconfirm **加强文案**「确认删除容器 {name}（镜像 {image}）？该操作不可恢复。」 | delete←stopped（MVP 无强制删除，api I-04；运行中按钮不渲染即不可达——先停止路径自然引导） |

过渡态容器 supportedActions=[] → 操作列仅呈现灰字「执行中」（无按钮）；DK_004 为伪造直调的服务端兜底（正常 UI 不可达，第 1 节）。

**在途互斥与结果反馈**（确认项 5 交叉，api I-06 前端落法）：

| 环节 | 行为 |
|------|------|
| 点击任一操作 | 行级 inFlight 登记——该行全部操作按钮置 loading/禁用（防重复提交；api 单容器操作语义） |
| 成功（code=0） | message.success「{动作}指令已执行」+ 重拉 containerList + composeList 标记过期（第 2 节纪律） |
| DK_003/DK_004 | 第 1 节反馈 + 自动重拉 containerList（实况与权威面同步） |
| DK_017 | error + data.reason（第 1 节） |

### 4.3 Tab 2 Compose（确认项 2/3）

**工具条**：「新建项目」按钮（primary，admin 专属，v1.1 FR-22）→ 打开 5.8 新建 Compose 项目 Dialog + 「刷新」按钮（compose/list 无筛选参数，api 4.3.4）。表格上方灰字「数据生成于 {generatedAt}」+「共 N 个项目」。

**表格列**：

| 列 | 类型 | 宽度 | 说明 |
|----|------|------|------|
| 序号 | index | 60 | — |
| 项目名 | link | 180 | items[].project 点击 → Compose 日志 Drawer（第 5.2 节） |
| 工作目录 | text | 260 | items[].workingDir 原值透传（label 元数据，R-13；等宽字体） |
| 配置文件 | text | 200 | items[].configFile 原值透传 |
| 容器 | text | 200 | `{n} 个容器` + NTooltip 列出各容器 `{name}（{state}）`摘要（api containers[] 同源实况） |
| 定位 | Tag | 110 | locatable=true → NTag green「可操作」；false → NTag red「定位失败」（DK_015 预警线索，Tooltip 呈现「工作目录或配置文件缺失」） |
| 操作 | action | 240 fixed:right | 见下（readonly 视角整列不渲染） |

**操作列**（locatable 驱动 + api D-06/I-08）：

| 按钮 | 可见性 | 二次确认 | 依据 |
|------|--------|---------|------|
| up | locatable=true | NPopconfirm「确认执行 up？将按配置文件创建/启动项目全部服务。」 | api D-06 verb 端点 |
| down | locatable=true | NPopconfirm **加强文案**「确认执行 down？将停止并移除项目 {project} 的全部容器。」 | down 影响面=项目全容器 |
| restart | locatable=true | NPopconfirm「确认重启项目 {project} 全部服务？」 | — |
| 日志 | 恒显（打开 Drawer，第 5.2 节） | 无 | composeLogs/composeLogsFollow 查看类 |

locatable=false：up/down/restart 按钮**禁用置灰** + Tooltip「工作目录或配置文件缺失，无法执行项目级操作；可在容器页逐个操作兜底」（api D-06/I-06 兜底口径）；日志按钮保持可用？——**不可用**（logs/logs-follow 同经 CLI 定位，DK_015 同判；api 4.3.4 logs 校验顺序含 DK_015），禁用 + 同款 Tooltip。在途互斥：项目级 inFlight（DK_018 服务端兜底，前端同项目按钮互斥禁用）；成功后重拉 composeList + containerList。

### 4.4 Tab 3 镜像（确认项 2/3/5）

**工具条**：拉取镜像 NButton（primary，admin 专属）→ Dialog（第 5.4 节）+ 刷新。表格上方灰字「数据生成于 {generatedAt}」+「共 N 个镜像」。

| 列 | 类型 | 宽度 | 说明 |
|----|------|------|------|
| 序号 | index | 60 | — |
| 镜像 | text | 300 | items[].ref（名称:tag，等宽字体） |
| 大小 | text（右对齐） | 120 | sizeBytes 人类可读格式化（MB/GB） |
| 操作 | action | 100 fixed:right | 删除（readonly 整列不渲染） |

删除：NPopconfirm「确认删除镜像 {ref}？被容器引用时将被拒绝。」→ imageDelete；DK_006 → warning「镜像被容器引用，无法删除」+ data.reason（api D-08 归一；引用情况不做前置推导，api I-05）。空结果 → NEmpty「当前无镜像」。

### 4.5 Tab 4 卷（确认项 3/4）

工具条：「创建卷」按钮（primary，admin 专属，v1.1 FR-21）→ 打开 5.6 创建卷 Dialog + 刷新。表格上方灰字「数据生成于 {generatedAt}」+「共 N 个卷」。

| 列 | 类型 | 宽度 | 说明 |
|----|------|------|------|
| 序号 | index | 60 | — |
| 卷名 | text | 260 | items[].volumeName（等宽字体） |
| 挂载点 | text | 300 | items[].mountpoint 原值 |
| 大小 | text（右对齐） | 120 | sizeBytes 人类可读格式化 |
| 操作 | action | 100 fixed:right | 删除（readonly 整列不渲染） |

删除：NPopconfirm **加强文案**「确认删除卷 {volumeName}？卷内数据将随删除**永久销毁、不可恢复**。」（PRD FR-13 数据销毁确认）→ volumeDelete；DK_008 → warning「卷被容器挂载，无法删除」+ data.reason。空结果 → NEmpty「当前无卷」。

### 4.6 Tab 5 网络（确认项 3/4）

工具条：「创建网络」按钮（primary，admin 专属，v1.1 FR-20/21）→ 打开 5.7 创建网络 Dialog + 刷新。表格上方灰字「数据生成于 {generatedAt}」+「共 N 个网络」。

| 列 | 类型 | 宽度 | 说明 |
|----|------|------|------|
| 序号 | index | 60 | — |
| 名称 | text | 220 | items[].name |
| 驱动 | text | 140 | items[].driver 原值透传（bridge/host/overlay 等开放集合不枚举化） |
| 子网 | text | 200 | items[].subnet（无子网网络 → 「—」，不虚构） |
| 操作 | action | 100 fixed:right | 删除（readonly 整列不渲染） |

删除：NPopconfirm「确认删除网络 {name}？」→ networkDelete；DK_010 → warning「网络被容器接入，无法删除」；DK_011 → warning「预定义网络不可删除」（bridge/host/none 等如实呈现于清单、删除以服务端权威拒绝为准，api D-08/I-05）。空结果 → NEmpty「当前无网络」。

## 第 5 节 Drawer / Dialog（确认项 5）

### 5.1 容器详情 Drawer（containerStats + containerLogs + containerLogsFollow）

**形态依据**：容器观测三接口（stats/logs/logs-follow）无独立路由页——名称列 link 点击 → NDrawer（宽 720px，标题「容器 {name}」）；详情跳转经列表 link（分册 3.1）。含两张 Card：

**Card A 资源占用快照**（containerStats，api D-07 单次快照）：

| 元素 | 呈现 |
|------|------|
| CPU 占用 | text `{cpuPercent}%` |
| 内存 | text `{memUsageBytes 格式化} / {memLimitBytes 格式化}（{memPercent}%）` |
| 刷新 | NButton「刷新快照」（单次重拉；**无自动轮询**，api D-07） |
| 数据生成于 | 灰字 {generatedAt} |

**Card B 日志**（containerLogs 尾部 + containerLogsFollow 跟随，api D-03 双通道）：

| 元素 | 呈现 |
|------|------|
| 日志区 | 滚动文本区（等宽字体，自动滚底；呈现 containerLogs 返回的 lines[]） |
| 行数 | NSelect（200/500/1000/2000，默认 500——tail 参数取值域 1-2000 的档位化，api D-03 常量） |
| 跟随开关 | NSwitch「实时跟随」——开启建立 WS 订阅（containerLogsFollow，Cookie 握手同会话），新行按 `{"type":"log","line"}` 帧追加；关闭/关闭 Drawer 即断开（无残留订阅） |
| end 帧处理 | `{"type":"end",reason}` → 停止跟随 + 按 reason 提示：container_removed→info「容器已删除，日志流结束」；container_stopped→info「容器已停止，日志流结束」；daemon_lost→error「daemon 连接中断」（api D-03 帧协议） |

**5.3 end 帧与断流提示**（并入上表 reason 行；WS 无自动重连——重开跟随开关=重试入口，第 2 节纪律）。

### 5.2 Compose 日志 Drawer（composeLogs + composeLogsFollow）

项目名 link 点击 → NDrawer（宽 720px，标题「Compose 项目 {project}」）：单日志 Card，元素与 5.1 Card B 完全一致（行数档位/跟随开关/end 帧处理；reason ∈ project_gone→「项目已不存在」/ locate_failed→「工作目录或配置文件缺失」/ daemon_lost，api D-03/D-06）；locatable=false 时打开即 NAlert warning 呈现 DK_015 语义（api 4.3.4 logs 校验含 DK_015，Drawer 内如实降级提示 + 引导容器页兜底）。页头附灰字工作目录/配置文件（R-13 label 透传）。

### 5.4 拉取镜像 Dialog（imagePull，确认项 5）

**形态依据**：拉取 1 字段（ref，< 5 字段 → Dialog，分册 3.2）；入口：Tab 3 工具栏「拉取镜像」→ NModal（宽 480px）。

| 字段 | 组件 | 取值与校验（前端预校验 + 服务端 R-14 兜底 DK_012） |
|------|------|------------------------------------------------------|
| 镜像引用 ref | NInput | 占位「如 nginx:1.27 或 ubuntu」；非空 + `名称[:tag]` 格式（api DK_012） |

表单底部灰字说明「拉取经本机 Docker daemon 执行（面向公有仓库），大镜像可能耗时较长」（api D-05 同步 600s 契约的用户可知会项；D4 以前端 pending 态呈现——提交后按钮 loading + 「拉取中…」文案，无进度流）。按钮：**拉取 / 取消**（工具类无审批语义，分册按钮组合参照 master_data 的 保存/取消 形态）。成功 → message.success「镜像已拉取：{ref}」+ 关闭 Dialog + 重拉 imageList；DK_013 → error + data.reason（Dialog 保持打开供修正重试）。

### 5.5 创建容器 Dialog（containerCreate，v1.1 FR-19）

**形态依据**：7 组字段 > 5 → NModal（宽 640px）；入口：Tab 1 工具栏「创建容器」按钮（4.1）。字段集=D0 拍板基础全集（d0-clarification-docker-mgmt-create-2026-11-21.md Q1）：

| 字段 | 组件 | 取值与校验（服务端 R-14 权威） |
|------|------|------|
| 容器名 | NInput | 占位「字母或数字开头，可含 . _ -，如 my-app」；白名单 ^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$（api D-09，与 Docker 原生命名规则一致——大写/下划线/点合法）；DK_019 |
| 镜像 | NInput | 占位「nginx:latest（本地不存在会自动拉取）」；必填（服务端 DK_001+reason「镜像必填」）；缺失自动拉取复用 PullImage 通道（Q2） |
| 端口映射（多组） | NInput×2 动态行 | 主机端口:容器端口，行级「添加端口映射/删除」，空行提交前过滤；端口 1~65535 数字，DK_020 |
| 环境变量（多组） | NInput×2 动态行 | KEY=value，行级增删；键非空且不含空格，DK_021 |
| 挂载（多组） | NInput×2 动态行 | 宿主机路径或卷名:容器内路径，行级增删；两段必填、source 禁 `..`，DK_022 |
| 重启策略 | NSelect | 除非手动 stopped（默认）/总是/失败时/不重启（四选项）；DK_001 兜底 |
| 网络 | NSelect clearable | 选项=网络 Tab 清单同源投影（store.networkItems 打开弹窗时惰性加载）；留空=默认 bridge；选择后服务端 ensureNetworkExists 前置拦截（不存在 DK_009） |

按钮：**创建并启动**（创建即 up，Q2；loading 在途互斥）。成功 → message.success「容器已创建并启动：{name}」+ 关闭 + 重拉容器清单（镜像/网络清单标记过期）。失败分流：DK_019~022 字段格式 → Dialog 内 NAlert error 呈现映射文案（**Dialog 保持打开**供修正重试）；DK_023 创建失败/DK_013 拉取失败 → NAlert「{文案}：{data.reason}」（保持打开）；DK_002 → 跳引导态（4.0）。

### 5.6 创建卷 Dialog（volumeCreate，v1.1 FR-21）

NModal 宽 420px；入口：Tab 4 工具栏「创建卷」（4.5）。字段：卷名 NInput（占位「如 site-data」，DK_001 兜底）+ 驱动 NInput（占位「local」，可空）。按钮 **创建**。成功 → message.success + 关闭 + 重拉卷清单；DK_024 → NAlert「卷创建失败：{data.reason}」（保持打开）。

### 5.7 创建网络 Dialog（networkCreate，v1.1 FR-20）

NModal 宽 460px；入口：Tab 5 工具栏「创建网络」（4.6）。字段：网络名 NInput（DK_001 兜底）+ 驱动 NInput（占位「bridge」，可空）+ 子网 NInput（占位「172.18.0.0/16（可选）」）+ 网关 NInput（占位「172.18.0.1（可选）」）。按钮 **创建**。成功 → message.success + 关闭 + 重拉网络清单；DK_025 → NAlert「网络创建失败：{data.reason}」（保持打开）。创建的网络即时出现在 5.5 容器创建的网络下拉（同源投影）。

### 5.8 新建 Compose 项目 Dialog（composeCreate，v1.1 FR-22，D-11）

NModal 宽 640px；入口：Tab 2 工具栏「新建项目」（4.3）。字段：

| 字段 | 组件 | 取值与校验 |
|------|------|------|
| 项目名 | NInput | 小写字母/数字/连字符（=落盘目录名）；已占用 → DK_027「换一个项目名」（保持打开） |
| compose.yml | NInput textarea rows=14 | 粘贴全文；服务端 YAML 语法校验，DK_026 报错带行号（NAlert 呈现 reason，保持打开） |
| 创建后自动启动 | NSwitch（默认开） | 开=落盘后自动 `compose up -d`（Q3）；关=仅落盘，项目出现在列表后手动启动；按钮文案随开关切换「创建并启动/创建」 |

成功 → message.success「Compose 项目已创建并启动：{name}」（未启动态如实注明）+ 关闭 + 重拉 Compose 与容器清单。落盘位置 /opt/dougong-compose/{项目名}/docker-compose.yml（api D-11，config docker.compose_root 可配）。

## 第 6 节 详情页

**无独立详情路由页**（确认项 6 判定）：api-docker-mgmt.md 无 detail 类路由接口——容器观测（stats/logs）经 Drawer 承载（5.1，打开时按需拉取）、Compose 日志经 Drawer 承载（5.2）；全部清单字段已在表格行内呈现（含 label 透传列即与 daemon/CLI 互认的详情承载，api 4.3.1）；无子表 Card、无审批记录 Card、无操作日志 Card（操作历史经 account-auth 审计日志查询，PRD N4）。

## 第 7 节 路由定义与框架行为

| 路由 | 页面 | meta |
|------|------|------|
| /docker | docker（容器管理页：页头 daemon 状态 Card + 引导态 + 五 Tab + 容器详情 Drawer + Compose 日志 Drawer + 拉取 Dialog） | `{requiresAuth:true}` |

**守卫**：复用全局守卫（initCheck → sessionCurrent → roles）；本模块无新增守卫。

**框架级边界**（复用 panel-core 框架行为，见 ../admin/panel-core/panel-core.md 第 7 节）：菜单渲染/安全入口前缀透明/顶栏均为 panel-core 宿主职责；本模块 1 个路由经模块页面挂载点接入（panel-core S-10），菜单项由 ModuleMenuRegistry 登记、服务端按角色下发（容器管理菜单对 admin/readonly 均可见——readonly 可进入查看，操作面为其不可见，FL/ADR-018）。

## 第 8 节 全局一致性

| # | 检查 | 结果 |
|---|------|------|
| 1 | Action ↔ api-docker-mgmt.md 接口一一对应（S7） | ✅ 24/24 端点全映射（22 HTTP + 2 WS），无虚构、无遗漏 |
| 2 | 字典 Key ↔ 04-business-enum.md 定稿枚举一一对应（S8） | ✅ container_status/docker_daemon_state/compose_action（+panel_role 复用）；取值逐字一致；过渡态为透传原值非枚举成员已声明 |
| 3 | 写 Action CSRF 注入完整（13 POST 全部 ✓）；2 WS 豁免有据（无变更语义） | ✅ 第 2 节 + 全局拦截器 |
| 4 | 操作列权威来源=服务端 supportedActions（容器六按钮照此渲染），前端不自行推断；DK_004 为伪造直调兜底 | ✅ 4.2 节（api D-02/D-04/I-04） |
| 5 | 状态映射呈现=BA 附录B 唯一口径：三态 dict+colorTag、过渡态透传 blue+入口禁用、delete 仅 stopped（无 force） | ✅ 4.1/4.2 节（api I-04；ADR-025） |
| 6 | 刷新纪律符合 D-01 按需拉取：status 先行/Tab 懒加载/写后重拉+关联清单标记过期/无轮询/WS 断开即止 | ✅ 第 2 节纪律表 |
| 7 | DK_001~018 + AUTH_001/002/003/008 错误文案与 api 3.4 一一对应 | ✅ 第 1 节错误处理 + 4.2/4.3/4.4/5.4 节 |
| 8 | 引导态落法：unavailable 整页降级引导、不呈现空容器列表（未装≠无容器）；DK_002 正常流程不可达兜底；detail 区分未装/不可达 | ✅ 4.0 节（api D-01/DK_002/I-01，PRD FR-02） |
| 9 | Compose 定位失败：locatable 红色预警 + 操作禁用 Tooltip + DK_015 路径呈现 + 容器级兜底提示；DK_018 同项目互斥 | ✅ 4.3 节 + 第 1 节（api D-06/I-06/I-08） |
| 10 | 日志双通道落法：tail 档位（默认 500，域 1-2000）+ WS 跟随开关 + end 帧五 reason 分级提示 + 断开即止无残留订阅 | ✅ 5.1/5.2 节（api D-03） |
| 11 | 危险删除确认分级：容器删除（含镜像信息）/卷删除（数据永久销毁加强）/Compose down（项目全容器）Popconfirm 加强文案；镜像/网络删除普通确认 | ✅ 4.2/4.3/4.4/4.5/4.6 节 |
| 12 | ADR-018：readonly 为子集视角（无拉取按钮、五 Tab 操作列整列不渲染），13 个 ✗ 项逐项负空间 | ✅ readonly 文档对应 |
| 13 | 无 📎 Scm\* 组件；Naive UI 为栈版本表选型；日志流不引入 xterm.js（非终端仿真） | ✅ 头部组件体系 |
| 14 | 无独立详情路由诚实性声明（观测经 Drawer 按需拉取，label 透传列承载互认语义）；时间本地时区展示；原值透传不翻译（workingDir/configFile/driver/reason） | ✅ 第 6 节 + 4.3/4.6 |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-14 | initial | 首次生成：1 hybrid 页面（页头 daemon 状态 Card + 引导态 + 容器/Compose/镜像/卷/网络五 Tab）、2 Drawer（容器详情=资源快照+日志双 Card；Compose 日志）、1 Dialog（拉取镜像）、24 Action（9 GET + 2 WS 读 + 13 POST 写）、3 字典 Key + panel_role 复用、0 独立表单路由页/0 详情路由页；supportedActions 权威渲染（容器六按钮）、locatable 预警+禁用（Compose 定位失败）、日志双通道（tail 档位+WS 跟随+end 帧分级）、刷新纪律（status 先行/Tab 懒加载/写后关联清单标记过期）为本阶段核心交互定义 |
| v1.1 | 2026-11-22 | increment | 创建类增量 D4（56a3033 初版）：仅新增第 2a 节 Action 表——⚠️ 未同步 4.x 工具条与弹窗规格（交付断裂根因，见 2a 节更正注记） |
| v1.1 | 2026-11-23 | fix-rewrite | 修复轮重写：4.1/4.3/4.5/4.6 工具条补四创建按钮（admin 专属）+ 5.5~5.8 四创建 Dialog 字段规格逐项（容器基础全集七组字段含多组端口/环境变量/挂载动态行、网络下拉同源投影；Compose 粘贴校验带行号+自动启动开关）+ 失败保留弹窗错误口径（019~022 格式/023~026 执行失败/027 换名）+ 第 8 节全局一致性待 Gate 2 复检（含入口可达性硬性项） |
| v1.2 | 2026-11-23 | 增量 | 宝塔式重组 D4（ADR-033）：新增「应用」页签（位于「容器」之后）——清单、创建应用弹窗、启停/重启/日志/建站/删除；Compose 页签零容器项目呈现「未运行」、新增行内「删除项目」、DK_027 提示更新；新增 7 个 Action（admin 28→35）；错误处理新增 DK_028~032 |

> FX 质量清单说明：FX-001~FX-141 逐条清单文件未随引擎 0.5.2 payload 发布（monitoring/panel-core/account-auth/service-mgmt/firewall 先例同口径复核），本节按分册 9 节结构逐节落实，并以第 8 节 14 项全局一致性自查替代可机械执行部分；全量 141 项留待 Gate 1 validation 执行。

## 增量 v1.2（2026-11-23）：宝塔式重组 D4——「应用」页签与 Compose 生命周期

> 依据 baota-restructure-design-2026-11-23.md 第五、六、九章。本节是权威增量。

### 「应用」页签（新增，位于「容器」之后）

- **工具条**：「创建应用」按钮（primary，admin 专属）+「刷新」；下方灰字「数据生成于 {generatedAt}」+「共 N 个应用」。
- **列表列**：序号 / 名称 / 镜像 / 本机端口（呈现 127.0.0.1:{主机端口} → {容器端口}）/ 状态（运行中=绿、已停止=灰、部分运行=橙）/ 站点（已建站→站点链接；未建站→「仅本机可访问」）/ 操作。
- **操作列**（admin；readonly 整列不渲染）：启动（未运行时）/ 停止（运行中时）/ 重启 / 日志 / 建站（未建站时）/ 删除（红色；强确认）。
- **创建应用 Dialog**（NModal 宽 640px）：名称（占位「小写字母、数字、连字符，如 my-app」）/ 镜像（占位「nginx:latest」）/ 主机端口 / 容器端口 / 环境变量（多组 KEY=value 动态行）/ 挂载（多组 来源:容器内路径 动态行）/ 重启策略（下拉，选项同创建容器）；表单内说明「应用端口只在本机可访问，建站后对外提供访问」。失败保留弹窗并显示原因（DK_029 / DK_030 / DK_031；重名 DK_027 提示换名）。
- **日志**：复用 Compose 日志 Drawer（项目名=应用名）。
- **删除**：强确认「将停止并移除容器，并删除项目目录 /opt/dougong-compose/{名称}，不可恢复；已建的站点不会删除」。
- **建站**：弹窗填写域名（与 app-mgmt 建站弹窗同形态），提交后提示站点已创建。
- **空态**：「还没有应用，点击「创建应用」开始」（含创建按钮，admin）。

### Compose 页签变更

- 落盘存在但无容器的项目并入列表，状态呈现「未运行」；该类项目的操作：启动 / 日志 / 删除项目。
- 新增行内「删除项目」（admin，强确认）：面板创建的项目文案「将停止并移除项目容器，并删除项目目录」；面板外项目文案「将停止并移除项目容器；项目目录不在面板管理范围内，不会删除」。
- DK_027 提示更新为「Compose 项目已存在：可在 Compose 列表查看或删除同名项目后重建」。

### Action 与错误处理

- 新增 7 个 Action：appList（GET）/ appCreate（POST）/ appService（POST）/ appLogs（GET）/ appDelete（POST）/ appSite（POST）/ composeDelete（POST）；管理员 Action 总数 28 → 35。
- 错误处理新增：DK_028→warning「应用不存在」并重拉应用清单；DK_029 / DK_030 / DK_031→创建弹窗内 NAlert（保持打开）；DK_032→error「项目删除失败：{data.reason}」。
