# PageSpec · 容器管理（docker-mgmt） · 只读用户（readonly）

| 属性 | 值 |
|------|-----|
| 版本 | v1.2 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-23（v1.2 宝塔式重组 D4：「应用」页签可看（清单与日志）；负空间 +7；Compose 列表可见未运行项目） |
| 用户类型 | readonly |
| 唯一数据源 | 02-detailed/api/api-docker-mgmt.md v1.1 |
| 公共页定义 | **以 admin 文档（../admin/docker-mgmt/docker-mgmt.md）为单一事实源**——页头 daemon 状态 Card/引导态/五 Tab/表格列/Drawer/错误处理/刷新纪律逐字复用；本文档仅声明子集差异与负空间（本模块存在 13 个写接口，readonly 为子集视角，对照 firewall/service-mgmt 模式而非 monitoring 全等模式） |
| 边界依据 | ADR-018：全模块可查看 + 禁一切变更/终端/上传下载——容器启停/重启/暂停恢复/删除、镜像拉取/删除、卷删除、网络删除、Compose up/down/restart 均为变更类动作，readonly 界面无入口、越界请求被全局边界管控拒绝（api 3.3 权限矩阵 13 个 ✗ 项 + AUTH_008）；容器与 Compose 日志（含 WS 跟随）、资源占用为查看类，readonly 全保留 |
| 组件体系 | Vue 3 + Naive UI（同 admin 文档） |

## 第 1 节 模块元信息

| 项 | 值 |
|----|-----|
| 模块编码 | docker-mgmt |
| 业务类型 | 工具类（同 admin 判定） |
| 页面类型 | hybrid ×1（容器管理页：页头状态 Card + 引导态 + 五 Tab，同 admin） |
| 可见页面 | /docker——与 admin 同路由（readonly 可进入查看五类清单、容器与 Compose 日志、资源占用、接管结论全集，FL 2.2「全可看」） |
| 错误处理 | 与 admin 一致，另含 AUTH_008 全局边界拒绝提示「权限不足：只读角色禁止容器管理变更操作」（正常 UI 无入口不触发——api 3.3 readonly 写全 ✗ + 全局中间件拦截）；DK_003~018 中写侧错误码（DK_004~006/008/010~013/016/018）正常流程对 readonly 不可达（无写入口） |

## 第 2a 节 v1.1 新增负空间（创建类，2026-11-22）

containerCreate / volumeCreate / networkCreate / composeCreate 四 Action readonly 全部负空间不渲染（ADR-018：创建类为变更操作 admin 专属）；四 Tab 工具栏无「创建」入口。

## 第 2 节 Action 定义

| Action | 方法 | 路径 | 页面 | 说明 |
|--------|------|------|------|------|
| daemonStatus | GET | /docker/daemon/status | docker·页头状态 Card | 与 admin 一致（同刷新纪律；readonly 亦见引导态与不可用 detail） |
| containerList | GET | /docker/container/list | docker·Tab 容器 | 与 admin 一致（同筛选参数——筛选为查看能力，ADR-018 全可看） |
| containerLogs | GET | /docker/container/logs | docker·容器详情 Drawer·日志 Card | 与 admin 一致（日志属查看类） |
| containerLogsFollow | WS | /docker/container/logs-follow | docker·容器详情 Drawer·跟随开关 | 与 admin 一致（Cookie 握手 + Origin；查看类流，ADR-018 可看） |
| containerStats | GET | /docker/container/stats | docker·容器详情 Drawer·资源占用 Card | 与 admin 一致 |
| imageList | GET | /docker/image/list | docker·Tab 镜像 | 与 admin 一致 |
| volumeList | GET | /docker/volume/list | docker·Tab 卷 | 与 admin 一致 |
| networkList | GET | /docker/network/list | docker·Tab 网络 | 与 admin 一致 |
| composeList | GET | /docker/compose/list | docker·Tab Compose | 与 admin 一致（含 locatable 预警呈现——查看内容） |
| composeLogs | GET | /docker/compose/logs | docker·Compose 日志 Drawer | 与 admin 一致（含 locatable=false 的 DK_015 降级提示视角） |
| composeLogsFollow | WS | /docker/compose/logs-follow | docker·Compose 日志 Drawer·跟随开关 | 与 admin 一致 |

**负空间声明**（api 3.3 权限矩阵 13 个 ✗ 项逐项对应）：

| admin Action（readonly 不注册） | api 权限矩阵 ✗ 项 | 负空间落法 |
|--------------------------------|------------------|-----------|
| containerStart | POST /docker/container/start = ✗ | 容器表操作列**整列不渲染**（六按钮与 supportedActions 渲染逻辑整体不存在；fixed:right 列消失，其余列定义不变） |
| containerStop | POST /docker/container/stop = ✗ | 同上（整列不渲染） |
| containerRestart | POST /docker/container/restart = ✗ | 同上 |
| containerPause | POST /docker/container/pause = ✗ | 同上 |
| containerUnpause | POST /docker/container/unpause = ✗ | 同上 |
| containerDelete | POST /docker/container/delete = ✗ | 同上（删除 Popconfirm 加强文案相应不可达） |
| imagePull | POST /docker/image/pull = ✗ | Tab 镜像工具栏**「拉取镜像」按钮不渲染**（非禁用置灰——不渲染使无入口可探）；拉取 Dialog 相应不可达 |
| imageDelete | POST /docker/image/delete = ✗ | 镜像表操作列**整列不渲染** |
| volumeDelete | POST /docker/volume/delete = ✗ | 卷表操作列**整列不渲染**（数据销毁确认相应不存在） |
| networkDelete | POST /docker/network/delete = ✗ | 网络表操作列**整列不渲染** |
| composeUp | POST /docker/compose/up = ✗ | Compose 表操作列**整列不渲染**（up/down/restart 与其 Popconfirm 整体不存在；「日志」入口保留——经项目名 link 打开 Drawer 承载，link 列非操作列） |
| composeDown | POST /docker/compose/down = ✗ | 同上 |
| composeRestart | POST /docker/compose/restart = ✗ | 同上 |

> 越界兜底：即使伪造请求，全局中间件 RoleRequired 拦截返回 AUTH_008（api 3.3「接口层不自建权限分支」）；本 PageSpec 层面以不渲染保证无入口。页头状态/筛选/刷新/Tab 懒加载/Drawer 观测（stats+日志+跟随）/locatable 预警等 readonly 全部保留（只读能力对齐 ADR-018「全可看」——状态列、Compose 归属列、日志与快照为查看内容，保留呈现）。

## 第 3 节 字典 Key 清单

与 admin 文档第 3 节完全一致（同一 enums.ts 常量源）：container_status / docker_daemon_state / compose_action（+panel_role 复用）；颜色方案同源（running=green/paused=orange/stopped=gray；过渡态透传 blue；available=green/unavailable=orange；locatable 失败=red）。

## 第 4 节 列表页（/docker，只读视角）

与 admin 文档第 4 节逐字一致，仅以下差异：

| 项 | admin | readonly |
|----|-------|----------|
| Tab 1 容器工具条（4.1） | 状态/Compose 筛选 + 刷新 | **完全一致**（筛选为查看能力，ADR-018 全可看；本 Tab 无 admin 写入口） |
| Tab 1 操作列（4.2） | supportedActions 渲染六按钮 + 在途互斥 | **整列不渲染**（负空间，第 2 节）；过渡态行「执行中」灰字提示保留（查看内容） |
| Tab 2 Compose（4.3） | up/down/restart/日志操作列 + locatable 驱动 | **操作列整列不渲染**（负空间）；项目名 link→日志 Drawer 保留（查看类）；locatable 红色预警 Tag 保留（readonly 亦需知悉定位失败实况，处置线索为「联系管理员或经服务器本地操作」文案，不虚构 readonly 操作路径） |
| Tab 3 镜像（4.4） | 拉取按钮 + 删除操作列 | 工具栏**无「拉取镜像」按钮** + 操作列**整列不渲染**（负空间） |
| Tab 4 卷（4.5）/ Tab 5 网络（4.6） | 创建按钮 + 删除操作列 | 工具栏**无「创建卷」「创建网络」按钮**（v1.1）+ 操作列**整列不渲染**（负空间） |
| Tab 1 容器（4.1）/ Tab 2 Compose（4.3） | 创建按钮（v1.1） | 工具栏**无「创建容器」「新建项目」按钮**（负空间，第 2a 节） |
| 页头（4.0） | 状态 Card + 刷新 | **完全一致**（含引导态与不可用 detail——readonly 亦需知道「未装/不可达」实况） |
| 容器详情 Drawer（5.1）/ Compose 日志 Drawer（5.2） | stats + 日志 + 跟随 | **完全一致**（全部为查看类；WS 跟随对 readonly 开放，ADR-018 日志可看） |

空态/加载态/骨架/引导态与 admin 一致。

## 第 5 节 表单页

无表单页（同 admin 第 5 节：拉取镜像 Dialog 为 admin 专属——readonly 无拉取入口即无表单形态；筛选为查询条件非表单；各删除确认随操作列负空间不存在）。

## 第 6 节 详情页

无独立详情路由页（同 admin 第 6 节：观测 Drawer 对 readonly 同样可见——stats/logs/logs-follow 均为查看类接口）。

## 第 7 节 路由定义

| 路由 | meta | 与 admin 差异 |
|------|------|--------------|
| /docker | `{requiresAuth:true}` | 同路由同页面组件（组件内部按角色渲染子集视图）；无独立 readonly 路由 |
| 框架菜单 | — | menuList 下发的容器管理菜单项对 admin/readonly 均可见（服务端按 ADR-018「全可看」下发） |

## 第 8 节 全局一致性

| # | 检查 | 结果 |
|---|------|------|
| 1 | readonly Action 与 api 只读接口一致、无虚构接口 | ✅ 11 Action（9 GET + 2 WS）= api 3.3 readonly ✓ 项全集 |
| 2 | 负空间检查：api 3.3 权限矩阵 ✗ 项逐项对应 | ✅ 13/13（容器六写→操作列整列不渲染；pull→无拉取按钮；镜像/卷/网络/Compose 删除与 up/down/restart→各操作列整列不渲染，第 2 节表） |
| 3 | 子集视角与 admin 公共定义单一事实源（无字段差异、无查询差异、观测 Drawer 全等） | ✅ 第 4 节引用声明 |
| 4 | S8 字典同源 enums.ts（三枚举取值逐字一致） | ✅ 第 3 节 |
| 5 | AUTH_008 边界拒绝文案与全局拦截器一致（正常 UI 不可达） | ✅ 第 1 节 |
| 6 | ADR-018 三禁落法：禁变更=13 写接口负空间；禁终端=本模块无终端类功能（容器内终端二期 OUT）；禁上传下载=本模块无上传下载接口（拉取为 daemon 侧动作非面板上传） | ✅ 第 2 节 |
| 7 | 引导态与 locatable 预警对 readonly 同呈现（无角色差异；处置线索文案不虚构 readonly 操作路径） | ✅ 第 4 节（api DK_002/D-01/D-06/I-01） |
| 8 | WS 跟随对 readonly 开放（日志属查看类，ADR-018）；end 帧分级提示同 admin | ✅ 第 2/4 节（api 3.3 WS admin+readonly） |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-14 | initial | 首次生成：只读视角为 admin 子集（11 Action=9 GET+2 WS、同页面、五 Tab 操作列整列不渲染 + 无拉取按钮）；api 3.3 权限矩阵 13 个 ✗ 项逐项负空间对应；引导态/locatable 预警/观测 Drawer（含 WS 跟随）同 admin 全等；公共页定义单一事实源 = admin 文档 |
| v1.1 | 2026-11-23 | increment | 创建类四 Action 负空间补充登记（2a 节既有声明落进 4.x 逐 Tab 表）：Tab 1/2 工具栏无「创建容器」「新建项目」按钮、Tab 4/5 无「创建卷」「创建网络」按钮——与 admin v1.1 四创建入口逐项对应 |
| v1.2 | 2026-11-23 | 增量 | 宝塔式重组 D4（ADR-033）：「应用」页签对只读用户可看（应用清单、应用日志，2 读 Action）；负空间 +7（创建应用、应用操作列五项、Compose 删除项目）；Compose 列表可见「未运行」项目 |

> FX 清单执行方式同 admin 文档第 9 节注记（清单文件未随 payload 发布，按 9 节结构落实 + 第 8 节自查；全量留待 Gate 1）。

## 增量 v1.2（2026-11-23）：宝塔式重组 D4

- **「应用」页签**：应用清单与状态可看；项目名可打开应用日志 Drawer（观测类，2 读 Action：appList / appLogs）。
- **负空间（+7）**：工具栏无「创建应用」按钮；应用操作列（启动 / 停止 / 重启 / 建站 / 删除）整列不渲染；Compose 页签无「删除项目」。
- **Compose 页签**：列表可见「未运行」项目（落盘存在但无容器）。
