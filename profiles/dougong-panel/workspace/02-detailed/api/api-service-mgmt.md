# 斗拱面板 systemd 服务管理（service-mgmt）API 设计

| 属性 | 值 |
|------|-----|
| 版本 | v1.0 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-12 |
| 输入 | prd-service-mgmt.md v1.0（完备性检查通过：G 5/5、T 适用 4/4、N 4/4）、domain-model.md v1.3（service-mgmt 域：1 实体 systemd_service，无聚合根）、d0-clarification-service-mgmt.md、D1 三文档（BA/AA/FL v1.0） |
| 接口规约 | 无激活技术栈（ACTIVE_STACKS 为空）——按通用 REST/JSON 契约设计；认证=HttpOnly Cookie 会话（R-02）；鉴权=RBAC 两角色边界矩阵（ADR-018）；**零持久化**（kb 拍板：无表、无 DDL、无 SQL；systemd 交互经 dbus，R-08） |

## 1. 模块设计概述

### 1.1 业务背景及目标

systemd 服务管理是斗拱面板「发现即接管」哲学（R-15）的最直接承载：system 级管理器上已装的 unit 不重装、不注册，经 dbus 实时投影自动纳入面板服务列表（零持久化、无服务台账）；管理员对其执行启动/停止/重启与开机自启切换，全部操作等待执行终态并如实反馈（D-01），且逐笔留痕审计（R-03）。只读用户同等查看服务全集（ADR-018「全可看」）。服务对象：单机自托管的管理员（admin）与只读用户（readonly）。

### 1.2 边界说明

| 项 | 说明 |
|----|------|
| 本模块做 | system 级 unit 清单投影（全类型）、服务列表/过滤搜索/unit 属性只读查询、.service 的 start/stop/restart/enable/disable 五个动作接口（等待终态） |
| 本模块不做（接口边界） | ① 进程 TOP 列表——归 monitoring；② journalctl 服务日志——二期（PRD OUT-2）；③ unit 文件编辑——不做；④ user 级管理器 unit——非目标（仅 system 级）；⑤ 服务页面外壳与导航——归 panel-core；⑥ 认证/鉴权/审计本体——复用 account-auth 中间件与 AuditService.Record（本模块仅声明依赖） |
| 与存储的关系 | **零持久化**：无表、无 DDL、无 SQL（kb 拍板；domain-model.md 存储映射说明「service-mgmt→无表」）；systemd_service 为 dbus 实时投影 |

### 1.3 设计决策

| # | 决策 | 理由 |
|---|------|------|
| D-01 | 启停/重启为**同步等待终态**：接口保持至 systemd 侧 Job 完成（成功/失败）或 90s 上限；超时返回 SVC_006（任务仍在执行，终态经列表刷新确认），审计记 failure + detail.timeout=true；**不做 202+轮询异步形态**、不做「提交即成功」假反馈（PRD D1 落定口径） | 单机运维操作通常秒级完成；90s 对齐 systemd 默认 TimeoutStartSec/TimeoutStopSec 上限情形；同步形态使一次请求即得终态，免轮询通道。**关闭 prd-service-mgmt.md 待对齐-1** |
| D-02 | 服务列表**全量返回不分页**：按 unit 名称字典序升序；状态过滤（多值）与名称搜索在服务端执行（R-14） | 单机 unit 数量级为数十至数百，量级有界（PRD N1）；全量返回使前端过滤/搜索零额外往返。**关闭 prd-service-mgmt.md 待对齐-2 分页部分** |
| D-03 | 投影刷新=**按需拉取**：列表/属性每次请求即时读取 dbus（I-01 无缓存层）；操作响应内含操作后状态快照（免二次查询）；无服务端推送/长轮询（monitoring D-01 同口径，轻量哲学） | dbus 读取为轻量本机调用，按需拉取即最新实况；页面自动刷新节奏归 D4 呈现。**关闭 prd-service-mgmt.md 待对齐-2 刷新部分** |
| D-04 | subState=**字符串透传**（systemd 实际值，如 running/exited/waiting），不建模枚举；展示归类归 D4 | sub 状态跨 unit 类型不统一（service/timer/mount 各异），为 systemd 开放集合，枚举化会失真或频繁扩充。**关闭 prd-service-mgmt.md 待对齐-3** |
| D-05 | 自启状态=**透传 systemd 实际值**（enabledState 字符串，两态 enabled/disabled 为 kb 拍板主轴）；响应附服务端权威计算的 `canToggleAutostart`（=enabledState∈两态 ∧ canOperate，⚠️A14）；非两态值（static/indirect/masked 等）只读展示、切换接口对之返回 SVC_005 | 列表呈现不因两态模型失真；切换入口判定由服务端权威给出（R-14 精神：前端不自行推断）。**关闭 prd-service-mgmt.md 待对齐-4** |
| D-06 | **操作与审计非同事务**：业务动作在 systemd 侧（非 SQLite 事务域），动作完成后经 AuditService.Record 独立写入；写入失败重试一次，仍失败则记面板运行日志兜底、不影响操作结果返回 | account-auth I-05「同事务」的前提是变更落 SQLite；本模块动作不可回滚，强一致不可达成——如实降级并显式声明（与 account-auth 语义差异点） |
| D-07 | 零持久化声明：全部数据为 dbus 投影（进程内临时对象），无任何表/DDL/SQL；面板重启无本地服务状态 | kb 拍板不落库；无台账即无幽灵条目（PRD FR-02） |
| D-08 | 混合模式（MIXED）声明：systemd_service 实际为自管投影（数据源=本机 systemd，无外部同步源，R-09），不设 manage_mode 字段；写侧为动作转发（start/stop/restart/enable/disable）非数据 CRUD | 分册 Step 2 默认混合模式；本模块无托管侧、无数据写入侧 |
| D-09 | 操作目标寻址=**unit 全名**（如 `nginx.service`，含类型后缀）；服务端校验名称形态（官方后缀集）→ 存在性 → 类型边界 →（自启切换）两态主轴，逐级返回 SVC_001/003/004/005 | 名称含后缀即自校验类型，避免歧义；R-14 全部服务端校验 |

### 1.4 领域对象总览

| 领域对象 | 分类 | 承载 | 服务接口 |
|---------|------|------|---------|
| systemd_service（systemd 服务投影，unit 名称标识） | 实体 | dbus 实时投影集合（进程内，读取时整集合刷新，D-07 无表） | /services/unit/{list,detail,start,stop,restart,enable,disable} |

无聚合根（D2 判定：A1-A4 全不通过——状态事实源在 systemd 侧、终模块无跨域引用、面板无生命周期入口、无域内子对象）；无子流程（启停为「提交→等待终态→反馈」单动作过程，domain-model.md 第四部分声明）；无关联领域对象（不持有任何面板域聚合根 ID）；无领域事件与 MQ（单机单体，R-09；审计为进程内 OHS 出站调用，D-06）。

### 1.5 上下文映射

| 上下文 | 关系 | 机制 |
|--------|------|------|
| account-auth | 遵奉者 + OHS 消费 | 全部接口位于 Cookie 会话后（R-02，AuthRequired）；readonly 同等查看、操作 admin 专属（ADR-018，RoleRequired + AUTH_008 边界拒绝）；动作接口 CSRF（R-02，CsrfGuard）；审计写入 AuditService.Record（R-03，D-06 降级语义） |
| panel-core | 消费其 HTTP 骨架 | `/services` 路由组挂载；入口面/外壳归 panel-core（R-10） |
| systemd（本机） | 动作与投影源 | dbus 通道（sysd 适配器，R-08 参数化禁 shell）；非外部业务系统对接，无 ACL 必要（R-09 零外呼）；平台前提 ADR-008 仅主流 systemd 发行版 |
| 前端服务管理页 | 唯一消费方 | 仅经 7 个 HTTP 接口消费（5.1）；无其他业务模块依赖本模块 |

> 跨聚合根设计（第 2 章）：不适用——本模块无聚合根（工具类裁剪矩阵第 2 章「—」），对象级约束以不变量形式列于 4.1.1。

## 3. 应用/基础设施层设计

### 3.1 接口规范

| 项 | 规范 |
|----|------|
| 路径格式 | `/services/unit/{action}` 三段式（域=services、资源=unit 个体语义、动作段含 2 查询 + 5 动作）；资源单数（与 account-auth 资源单数风格一致） |
| HTTP 方法 | 读=GET（list/detail），写=POST（五动作，面板内部契约，account-auth D-07 同一约定） |
| 响应格式 | `{"code": 0, "message": "ok", "data": {...}}`；code=0 成功，非 0 见 3.4 错误码（与 account-auth D-07 同一契约） |
| 时间格式 | ISO-8601 UTC（公共约定）；generatedAt 为投影生成时刻 |
| 认证 | 复用 account-auth：HttpOnly Cookie `dg_session`（AuthRequired） |
| CSRF | 全部 POST 动作须带 `X-CSRF-Token` 头（R-02，CsrfGuard）；GET 豁免 |
| 中间件链 | AuthRequired → CsrfGuard（POST）→ RoleRequired（读=admin+readonly，写=admin） |
| 用户类型标注 | 读接口：admin+readonly（ADR-018 全可看）；写接口：admin（readonly 无界面入口 + 越界 AUTH_008 全局边界拒绝） |

### 3.2 依赖与约束

| 依赖 | 类型 | 约束 |
|------|------|------|
| account-auth 中间件 | 框架 | AuthRequired/CsrfGuard/RoleRequired 注册顺序见 3.1；不得自建会话解析与角色判定 |
| account-auth AuditService.Record | OHS | 操作审计经其写入（R-03）；失败重试一次后日志兜底（D-06），不得自建审计表 |
| 本机 systemd（dbus，sysd 适配器） | 数据源与动作通道 | 全部交互参数化接口调用（R-08，禁 shell 拼接）；通道不可用返回 SVC_008（不返回过期数据，I-01）；ADR-008 平台前提 |
| panel-core | 框架 | 路由组挂载 `/services`；不得越界承载业务逻辑 |

### 3.3 权限模型

角色：admin / readonly（panel_role 固定枚举）；本模块无访客可达面（全部位于登录会话后）。

**接口权限矩阵**：

| 接口 | admin | readonly | CSRF |
|------|-------|----------|------|
| GET /services/unit/list | ✓ | ✓（ADR-018 全可看） | —（GET） |
| GET /services/unit/detail | ✓ | ✓（ADR-018 全可看） | —（GET） |
| POST /services/unit/start | ✓ | ✗（无界面入口 + AUTH_008 边界拒绝，ADR-018） | ✓ |
| POST /services/unit/stop | ✓ | ✗（同上） | ✓ |
| POST /services/unit/restart | ✓ | ✗（同上） | ✓ |
| POST /services/unit/enable | ✓ | ✗（同上） | ✓ |
| POST /services/unit/disable | ✓ | ✗（同上） | ✓ |

数据权限规则：无行级隔离——服务全集两角色可见集完全一致；角色差异仅体现在动作接口（admin 专属）。readonly 越界由全局中间件 RoleRequired 拦截返回 AUTH_008（附边界原因），接口层不自建权限分支。

### 3.4 公共规范

**错误码**（SVC_ 前缀为本模块新增 8 个；鉴权/CSRF/越界复用 account-auth 码族）：

| code | 标识 | HTTP 语义 | 说明 |
|------|------|----------|------|
| 0 | OK | 成功 | — |
| SVC_001 | INVALID_UNIT_NAME | 参数错误 | unit 名称形态非法（非 `名称.类型后缀` 官方格式，D-09/R-14） |
| SVC_002 | INVALID_STATE_FILTER | 参数错误 | state 过滤值非法（不在 service_active_state 取值内） |
| SVC_003 | UNIT_NOT_FOUND | 拒绝 | 目标 unit 不存在（已卸载或名称错误，I-03 实况校验） |
| SVC_004 | UNIT_TYPE_NOT_ACTIONABLE | 拒绝 | 非 .service 类型发起操作（D1 落定边界；I-04） |
| SVC_005 | AUTOSTART_NOT_TOGGLEABLE | 拒绝 | 自启状态不可切换（enabledState 非两态主轴，如 static；⚠️A14/I-04） |
| SVC_006 | OPERATION_TIMEOUT | 超时 | 等待终态超 90s 上限（D-01）；Job 仍在执行，终态经列表刷新确认 |
| SVC_007 | OPERATION_FAILED | 操作失败 | systemd 侧 Job 执行失败；data 携带失败原因与实际终态 |
| SVC_008 | SYSTEMD_UNAVAILABLE | 通道不可用 | dbus 通道/systemd 不可达（列表/属性/动作共前置依赖；不返回过期数据，I-01） |
| AUTH_001/002 | （复用） | 401 | 未登录/会话过期——中间件归 account-auth |
| AUTH_003 | （复用） | 403 | CSRF 校验失败（POST 动作） |
| AUTH_008 | （复用）FORBIDDEN | 拒绝 | readonly 越界动作，附边界原因（ADR-018） |

**枚举**（均在 workspace/01-solution/04-business-enum.md 定义；三个服务枚举 D2 定稿，本次为引用无新增）：

| 枚举标识 | 取值 | 用途 |
|---------|------|------|
| service_active_state | active \| inactive \| failed | 运行维度状态（过滤参数 + 响应字段 activeState） |
| service_enabled_state | enabled \| disabled | 自启维度状态主轴（实际透传可含非两态值，D-05） |
| unit_type | service \| socket \| timer \| mount \| path \| target \| automount \| slice \| swap \| scope \| device | unit 类型（响应字段 + SVC_004 边界判定取值域） |
| panel_role | admin \| readonly | 权限模型角色（引用，account-auth 定稿） |
| audit_action / audit_result | change \| security / success \| failure | 操作审计类别与结果（引用，account-auth 定稿） |

**业务常量**（操作审计细粒度动作码，非枚举池条目）：`service_started / service_stopped / service_restarted / service_enabled / service_disabled`（action 列存此码，category=audit_action.change，object_type=`systemd_service`，object_id=unit 全名）。

**业务常量**（操作参数，非枚举池条目）：

| 常量 | 值 | 说明 |
|------|-----|------|
| 操作终态等待上限 | 90s | D-01；对齐 systemd 默认 TimeoutStartSec/TimeoutStopSec 上限情形 |
| 自启切换调用形态 | 轻量同步（无 Job 等待） | enable/disable 即时返回，结果反馈口径与 D-01 一致（如实反馈） |
| unit 名称形态 | `^[A-Za-z0-9@._-]+\.(service|socket|timer|mount|path|target|automount|slice|swap|scope|device)$` | D-09/SVC_001 校验依据（后缀集=unit_type 官方全集） |

**配置项**：无——等待上限/操作形态为编译期常量（D-01 定值，无用户可配置面）；无新增配置键。

## 4. 领域对象与服务详细设计

> 本模块无聚合根（工具类），本章按领域对象组织；各节保持与先例一致的子节结构（领域模型→状态模型→子流程→接口设计→事件与集成）。

### 4.1 systemd_service（systemd 服务投影，实体 + 发现/动作链路）

#### 4.1.1 领域模型

**表结构组合**：无表（D-07 零持久化）。systemd_service 驻留进程内，为每次 dbus 读取时重建的投影集合（读取即刷新，无驻留缓存层）；操作动作直达 systemd（动作契约），不经投影对象中转持久化。

**投影条目字段清单**（列表/属性/动作响应的共同数据源）：

| 字段 | 类型 | 说明 |
|------|------|------|
| name | string | unit 全名（如 `nginx.service`），投影标识与动作寻址（D-09） |
| unitType | enum(unit_type) | unit 类型（由 name 后缀归一，官方 11 类全集） |
| activeState | enum(service_active_state) | 运行维度状态（active/inactive/failed） |
| subState | string | 运行细分透传（running/exited/waiting…，跨类型不统一，D-04） |
| enabledState | string | 自启状态透传（enabled/disabled 为主轴，可含 static 等，⚠️A14/D-05） |
| description | string \| null | unit 描述（detail 返回；list 不返回，载荷考虑） |
| loadState | string \| null | 加载状态透传（loaded/not-found/errored…，detail 返回） |
| canOperate | bool | 服务端计算：unitType==service（操作入口判定，D-05 同权威口径） |
| canToggleAutostart | bool | 服务端计算：canOperate ∧ enabledState∈{enabled,disabled}（⚠️A14） |

**不变量**：

| 编号 | 不变量 | 违反处理 |
|------|--------|---------|
| I-01 | 投影真实性：list/detail 响应恒为当次 dbus 读取快照；无缓存层、无陈旧数据回退；通道不可用返回 SVC_008 而非过期数据 | 读取侧保证（每次请求即时读取，D-03） |
| I-02 | 零持久化：systemd_service 不落库，面板重启无本地服务状态；操作历史不在本模块留存（审计归 account-auth） | 结构保证（无表） |
| I-03 | 集合实况一致：列表内容=当次读取时系统已装 unit 全集（新装自动出现、卸载自动消失，PRD FR-02）；动作目标的实况存在性前置校验（SVC_003） | 读取+动作侧校验 |
| I-04 | 动作边界：动作接口仅接受 unitType=service（SVC_004）；自启切换仅接受 enabledState∈两态主轴（SVC_005）；逐级校验顺序见 D-09 | UnitActionGuard 统一校验 |
| I-05 | 终态反馈：start/stop/restart 在 Job 终态（成功/失败）或 90s 超时前不返回；任何情况下不返回「提交即成功」语义 | 动作编排保证（D-01） |
| I-06 | 操作可审计：每个动作接口调用恰好产生一条审计记录（success/failure/超时=failure+detail.timeout，R-03）；审计写入失败按 D-06 降级并日志兜底 | UnitActionService 编排 |
| I-07 | 自启透真：enabledState 恒为 systemd 实际值；canToggleAutostart 恒按 I-04 口径由服务端计算，前端不自行推断 | D-05 权威计算 |

**领域服务**：ServiceProjector（清单/属性投影组装 + 状态过滤/名称搜索）、UnitActionService（动作编排：校验→提交→等待终态→审计）、SysdGateway（dbus 通道包装，R-08；唯一 systemd 交互出口）。

#### 4.1.2 状态模型

面板侧无自有状态机写入（I-02 零持久化）；双维度状态机为 **systemd 侧事实源的投影呈现**（kb 拍板，两维度独立）：

| 维度 | 状态（枚举） | 流转（systemd 侧） | 面板触发 |
|------|-------------|-------------------|---------|
| 运行 | service_active_state：active ⇄ inactive / failed | 启动/停止/重启 Job 终态决定 | POST /services/unit/{start,stop,restart}（FR-06/07） |
| 自启 | service_enabled_state：enabled ⇄ disabled（两态主轴） | enable/disable 配置动作 | POST /services/unit/{enable,disable}（FR-09） |

辨析：① activating/deactivating 等 systemd 瞬时执行态不进入面板状态模型——以「操作在途」（接口未返回/前端进行中态）表达，终态仍落三态（I-05）；② subState 为运行细分展示属性而非独立状态机（D-04）；③ 自启切换不产生运行状态流转（两维度正交，kb 拍板）；④ 投影状态变更=下次读取刷新（D-03），面板不写状态。无审批/驳回环节（PRD 第六部分末注）；无状态历史表（操作追溯经 account-auth audit_log）。

#### 4.1.3 子流程设计

无（启停/重启为单动作过程，无独立生命周期状态；自启切换为单配置动作；服务发现纳管为后台投影链路——domain-model.md 第四部分已声明）。

#### 4.1.4 接口设计

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /services/unit/list | GET | admin+readonly | 服务全集列表（状态过滤 + 名称搜索） |
| /services/unit/detail | GET | admin+readonly | 单 unit 基本属性快照 |
| /services/unit/start | POST | admin | 启动（等待终态，D-01） |
| /services/unit/stop | POST | admin | 停止（等待终态） |
| /services/unit/restart | POST | admin | 重启（等待终态；failed 服务首要恢复动作） |
| /services/unit/enable | POST | admin | 开机自启启用（轻量同步） |
| /services/unit/disable | POST | admin | 开机自启禁用（轻量同步） |

**接口详情**（请求/响应/校验）：

- **GET /services/unit/list**：query `{state?, search?}`。校验（R-14）：state 非空时为逗号分隔的 service_active_state 多值（如 `active,failed`），任一值非法 → SVC_002；search 为名称子串（不区分大小写）。处理：即时读取全量清单（I-01/I-03）→ 服务端过滤/搜索（D-02）→ 名称字典序升序。响应 data=`{items:[{name, unitType, activeState, subState, enabledState, canOperate, canToggleAutostart}], total, generatedAt}`——total=过滤后条数；generatedAt=投影生成时刻（数据新旧可辨识，monitoring D-08 同口径）；列表不含 description（单点 detail 承载，载荷考虑）。
- **GET /services/unit/detail**：query `{name}`。校验：名称形态（SVC_001）→ 存在性（SVC_003）。响应 data=`{name, unitType, activeState, subState, enabledState, canOperate, canToggleAutostart, description, loadState}`。
- **POST /services/unit/start**：req `{name}`。校验顺序（D-09/I-04）：名称形态（SVC_001）→ 存在性（SVC_003）→ unitType=service（SVC_004）→ 提交 systemd 并等待终态（≤90s）。成功 data=`{name, activeState, subState, enabledState}`（操作后投影快照，免二次查询）；Job 失败 → SVC_007，data=`{name, reason, activeState}`（失败原因 + 实际终态）；超时 → SVC_006，data=`{name, note}`（note=「任务仍在执行，请稍后刷新列表确认终态」）。审计：service_started（change；result 按 成功=success / Job 失败与超时=failure，超时 detail.timeout=true，D-01/D-06）。
- **POST /services/unit/stop**：同 start 结构（等待终态）；审计：service_stopped。
- **POST /services/unit/restart**：同 start 结构（active/inactive/failed 均可发起，failed 为首要恢复对象）；审计：service_restarted。
- **POST /services/unit/enable**：req `{name}`。校验：形态（SVC_001）→ 存在性（SVC_003）→ unitType=service（SVC_004）→ canToggleAutostart（SVC_005）。轻量同步调用（无 Job 等待），成功 data=`{name, enabledState:"enabled"}`（刷新值）；systemd 拒绝（如配置只读）→ SVC_007。审计：service_enabled（change；结果如实）。
- **POST /services/unit/disable**：同 enable 结构，成功 data=`{name, enabledState:"disabled"}`；审计：service_disabled。

**跨接口复用规则**：unit 名称形态校验/存在性校验/类型边界/两态主轴校验统一走 UnitActionGuard（I-04，SVC_001/003/004/005 映射）；读接口的存在性校验复用同一 Guard（SVC_003）；操作审计统一走 `AuditService.Record`（D-06）；等待终态统一由 SysdGateway 的 Job 等待包装承载（I-05）。

**幂等语义**：start/stop/restart/enable/disable 重复提交在 systemd 侧安全无副作用（如对 active 服务 start 为 no-op 成功，systemd Job 语义）；面板侧防重复提交为前端在途态（PRD N2/P3 过程数据），非服务端一致性手段——服务端不设逐 unit 操作锁（并发重复动作安全）。

#### 4.1.5 领域事件与集成

无 MQ、无领域事件（单机单体，R-09）。「仓储契约」为零持久化替代——**dbus 网关契约（进程内适配层，非 SQL；R-08 唯一 systemd 交互出口）**：

```
SysdGateway {
  ListUnits() ([]UnitSnapshot, error)              // system 级全量清单投影（I-01/I-03）
  GetUnit(name) (UnitDetail, error)                // 单 unit 属性（含 description/loadState）
  StartUnit(name) (JobResult, error)               // 提交 + 等待 Job 终态（≤90s，I-05）
  StopUnit(name) (JobResult, error)
  RestartUnit(name) (JobResult, error)
  EnableUnit(name) error                           // 轻量同步（无 Job 等待）
  DisableUnit(name) error
}
JobResult { Succeeded bool; Reason string; ActiveState string; SubState string }
约束：全部经 dbus 参数化调用（R-08 禁 shell 拼接）；通道错误归一为 SVC_008 语义；等待包装含超时中断（返回超时语义而非挂起）
```

关联域调用（出站）：`AuditService.Record`（account-auth OHS，操作审计 R-03；D-06 降级语义）；无其他出站；被调用契约：无（本模块为服务消费终模块，1.5）。

## 5. 模块清单汇总

### 5.1 对外服务清单（HTTP 接口）

| 服务（接口） | 消费方 | 契约要点 |
|-------------|--------|---------|
| GET /services/unit/list | 前端服务管理页（唯一消费方） | 全量投影 + 服务端过滤/搜索（D-02/D-03） |
| GET /services/unit/detail | 前端服务管理页 | 单 unit 属性快照（含 description/loadState） |
| POST /services/unit/start / stop / restart | 前端服务管理页（admin） | 同步等待终态 ≤90s（D-01）；响应含操作后快照 |
| POST /services/unit/enable / disable | 前端服务管理页（admin） | 轻量同步；两态主轴边界（SVC_005，⚠️A14） |
| 中间件 AuthRequired / CsrfGuard / RoleRequired | （复用） | account-auth 提供，顺序见 3.1 |
| AuditService.Record | （出站调用） | 操作审计写入（R-03，D-06） |

本模块无对外 OHS 服务契约——服务消费终模块（1.5：无其他业务模块依赖）。

### 5.2 SQL 清单

**无**——本模块零持久化（D-07）：systemd_service 为 dbus 实时投影，无任何 SQLite 表读写（domain-model.md 存储映射说明「service-mgmt→无表」）；操作审计落库由 account-auth 的 AuditRepository 承载（非本模块 SQL）。

### 5.3 DDL

**无**——零持久化（同 5.2；本模块不新增任何表，面板库 schema 无变更）。

## 6. 更新记录

### 6.1 变更记录

| 版本 | 日期 | 变更类型 | 变更摘要 | 目标节号 |
|------|------|---------|---------|---------|
| v1.0 | 2026-11-12 | initial | 首次生成：7 个 HTTP 接口（GET /services/unit/{list,detail} + POST /services/unit/{start,stop,restart,enable,disable}，读=admin+readonly / 写=admin+CSRF）+ 1 个 dbus 网关契约 SysdGateway（零持久化：无表/无 DDL/无 SQL）+ 8 错误码（SVC_001~008）+ 复用 account-auth 码族（AUTH_001/002/003/008）+ 6 枚举引用（三个服务枚举 D2 已定稿，无新增）+ 5 个审计动作常量 + 9 项设计决策（D-01 同步等待终态 90s、D-02 全量不分页、D-03 按需拉取、D-04 subState 透传、D-05 自启透真+权威可切换判定、D-06 操作与审计非同事务降级）；关闭 prd-service-mgmt.md 待对齐-1/-2/-3/-4；无旧版对比 | — |

### 6.2 变更类型词汇表

| 变更类型 | 含义 | 需要目标节号 |
|---------|------|------------|
| `initial` | 首次生成 | 否 |
| `structural` | 章节增删/重组 | 否 |
| `table_added` / `table_changed` | 新增/变更数据表 | 是 |
| `field_added` / `field_changed` / `field_removed` | 字段增删改 | 是 |
| `enum_added` / `enum_value_added` / `enum_value_changed` | 枚举增删改 | 是 |
| `interface_added` / `interface_changed` / `interface_removed` | 接口增删改 | 是 |
| `state_added` / `state_changed` / `flow_added` / `flow_changed` | 状态/流转增删改 | 是 |
| `rule_added` / `rule_changed` | 规则增删改 | 是 |
| `event_added` / `event_changed` | 领域事件增删改 | 是 |

### 6.3 版本号规则

语义化版本：vMAJOR.MINOR——结构/接口增删改升 MAJOR；字段/枚举/规则微调升 MINOR；本文档随模块迭代在 6.1 追加行。
