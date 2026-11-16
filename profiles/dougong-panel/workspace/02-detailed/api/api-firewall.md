# 斗拱面板 防火墙（firewall）API 设计

| 属性 | 值 |
|------|-----|
| 版本 | v1.0 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-14 |
| 输入 | prd-firewall.md v1.0（完备性检查通过：G 5/5、T 适用 4/4、N 4/4）、domain-model.md v1.12（firewall 域：1 实体 firewall_rule，无聚合根）、d0-clarification-firewall.md、D1 三文档（BA/AA/FL v1.0） |
| 接口规约 | 无激活技术栈（ACTIVE_STACKS 为空）——按通用 REST/JSON 契约设计；认证=HttpOnly Cookie 会话（R-02）；鉴权=RBAC 两角色边界矩阵（ADR-018）；**零持久化**（kb 拍板：无表、无 DDL、无 SQL；ufw/firewalld 交互经 firewallx 适配器命令通道，R-08 参数数组） |

## 1. 模块设计概述

### 1.1 业务背景及目标

防火墙是斗拱面板 M7 里程碑的主机网络入口安全模块，也是「识别即接管」哲学（R-15）在防火墙维度的承载：服务器上既有的 ufw / firewalld 后端不安装、不切换、不替换——进入防火墙业务时自动探测（ufw 优先），均无时以引导提示呈现（不将「无后端」误呈现为「无规则」）；识别后提供端口级规则的查看、添加、删除（命令一律参数数组 R-08，操作即时生效并审计 R-03）与监听端口开放状态对照。本模块误操作风险为全面板最高（错误删除可封锁面板自身访问入口），面板端口保护为服务端强制的两段式确认（M7 明示硬项）。只读用户同等查看规则与对照视图（ADR-018「全可看」）。服务对象：单机自托管的管理员（admin）与只读用户（readonly）。

### 1.2 边界说明

| 项 | 说明 |
|----|------|
| 本模块做 | 后端识别结论查询（类型/运行状态/动作支持面）、端口规则清单（含面板端口标注）、监听端口对照清单（含开放状态）、规则添加/删除（面板端口保护两段式确认） |
| 本模块不做（接口边界） | ① firewalld zone/富规则管理——二期（PRD OUT-1）；② IP 黑白名单——二期（OUT-2）；③ 出站规则/端口扫描/入侵检测——不做（OUT-3/4）；④ 防火墙后端安装与启停开关——不做（OUT-5；firewalld 服务启停经 service-mgmt）；⑤ 面板监听配置管理——归 panel-core（本模块进程内只读消费其配置值，D-06）；⑥ 认证/鉴权/审计本体——复用 account-auth 中间件与 AuditService.Record（本模块仅声明依赖）；⑦ 模块页面外壳——归 panel-core |
| 与存储的关系 | **零持久化**：无表、无 DDL、无 SQL（kb 拍板；domain-model.md 存储映射说明「firewall→无表」）；firewall_rule 为后端实况投影 |

### 1.3 设计决策

| # | 决策 | 理由 |
|---|------|------|
| D-01 | 后端探测=**每次 GET /firewall/backend/status 即时探测**（无缓存层、按需拉取，monitoring D-01 / service-mgmt D-03 同口径）；响应含 `active/version/supportedActions`，其中 **supportedActions 为服务端权威计算的动作支持面**（ufw→全四值 / firewalld→[allow] / none→[]，D1 口径-2），前端添加入口照此渲染、不自行推断（service-mgmt D-05 canToggleAutostart 同构） | 探测为轻量本机判定，按需拉取即最新结论；动作面判定属后端能力事实，收归服务端权威（R-14 精神）。**关闭 prd-firewall.md 待对齐-1/-9** |
| D-02 | 规则清单**全量返回不分页**：服务端筛选（action?/protocol? 多值逗号分隔）+ 排序（端口数值升序、同端口按 ruleKey 字典序）；响应含 `isPanelPort` 服务端标注（直接命中面板监听端口的规则，D1 口径-5） | 单机规则数量级为数十~百条（PRD N1 量级锁定），全量返回免分页往返；标注判定由服务端统一计算。**关闭 prd 待对齐-5（含分页复核）** |
| D-03 | 规则删除寻址=**ruleKey（后端原生命令定位串）**：ufw 为其 `delete` 子命令接受的 spec 形态（如 `allow 22/tcp`、`deny 8080-8090/udp from 10.0.0.0/8`），firewalld 为端口串（如 `8080/tcp`）；ruleKey 由服务端在 rule/list 组装、前端作**不透明引用**透传回 delete；不使用 ufw 数字序号（其随删除重排、跨请求不稳定） | 规避 ufw numbered 序号不稳定问题；spec 形态即后端删除命令原生入参，无二次翻译失真。**关闭 prd 待对齐-2** |
| D-04 | 监听端口对照口径：**allow 族（allow/limit）规则命中（端口+协议；tcp 族匹配 tcp/tcp6、udp 族匹配 udp/udp6）→ allowed_by_rule，否则 no_allow_rule**；deny/reject 族与来源限制维度不参与两态判定（对照为一阶提示非精确仿真，细粒度研判见规则清单）；`external`=监听地址非 loopback（服务端计算，127.0.0.1/::1 为 loopback） | 两态对照忠实回答「有无放行规则」（PRD FR-04），不虚构精确防火墙仿真；loopback 维度支撑「对外监听且无放行规则」的风险敞口提示。**关闭 prd 待对齐-3** |
| D-05 | 校验规则集（R-14 全部服务端执行）：port=`"N"` 或 `"N-M"`（1-65535 且 N≤M）；source 缺省=任意来源、非缺省时为合法 IPv4/IPv6 CIDR；protocol/action ∈ 枚举值域；方向固定入站（MVP 单值维度），**不作为请求参数**（firewall_rule_protocol 单值维度说明见枚举池行） | PRD N1 校验缺口一次落定；方向单值不开放参数避免伪维度。**关闭 prd 待对齐-4** |
| D-06 | 面板端口保护=**服务端强制两段式**：涉险判定=规则端口数值**直接命中**面板监听端口（不分协议、从严口径，MVP 无 HTTPS 协议维度推导）；触发面=create 提交 deny/reject 命中面板端口 + delete 目标为命中面板端口的 allow 族规则（D1 口径-4）；命中且请求未携带 `confirmDanger:true` → FW_005（data 携带命中详情，驱动前端警示与二次确认），确认后重提交放行——保护不阻止操作本身；判定基准=panel-core 监听配置**进程内直读**（internal/config，单体非 HTTP 自调用） | M7 明示硬项落为服务端守卫（前端警示可绕过、服务端不可）；「端口数值命中」从严禁漏判（协议维度推导留二期同 OUT-7）；进程内直读为零外呼最短路径。**关闭 prd 待对齐-6** |
| D-07 | 等价规则语义**如实透传不静默**：ufw「Skipping adding existing rule」/ firewalld「ALREADY_ENABLED」→ FW_006；删除目标不存在（面板外已删）→ FW_003 | 实况是唯一事实源（零规则台账），后端原生语义如实反馈，与「删不存在不静默成功」幂等口径一致。**关闭 prd 待对齐-7** |
| D-08 | firewalld 写入=**permanent 先写、runtime 后应用两次独立命令**（`--permanent --add-port` 成功后再 `--add-port` 应用运行态；不用 `--reload`——其将全量 permanent 回灌 runtime，可能冲掉面板外 runtime-only 变更）；permanent 成功 + runtime 失败 → FW_007（data 明示两层状态；不回滚 permanent——写入是用户意图，重启后仍生效，PRD N2） | 双写两步非原子（D1 口径-1）的实现落定；--reload 的旁路风险显式规避。**关闭 prd 待对齐-8** |
| D-09 | **操作与审计非同事务**：业务动作在防火墙后端侧（非 SQLite 事务域），动作完成后经 AuditService.Record 独立写入；写入失败重试一次，仍失败记面板运行日志兜底、不影响操作结果返回 | account-auth I-05「同事务」前提是变更落 SQLite；本模块动作不可回滚，强一致不可达成——如实降级并显式声明（service-mgmt D-06 同判） |
| D-10 | 零持久化与混合模式（MIXED）声明：firewall_rule 实际为自管投影（数据源=本机防火墙后端，无外部同步源，R-09），不设 manage_mode 字段；写侧为命令转发（create/delete）非数据 CRUD | kb 拍板不落库；分册 Step 2 默认混合模式，本模块无托管侧（service-mgmt D-07/D-08 同构） |

### 1.4 领域对象总览

| 领域对象 | 分类 | 承载 | 服务接口 |
|---------|------|------|---------|
| firewall_rule（防火墙端口规则投影，ruleKey 标识） | 实体 | 后端实况投影集合（进程内，读取时整集合刷新，D-10 无表） | /firewall/{backend/status, rule/list, rule/create, rule/delete, port/list} |

无聚合根（D2 判定：A1-A4 全不通过——规则事实源在 ufw/firewalld 侧、终模块无跨域引用、面板增删仅为通道之一、无状态机）；无子流程（增删为单动作即时执行，domain-model.md 第四部分声明）；无关联领域对象（panel-core 监听配置为实况值消费非聚合 ID 引用，D-06）；无领域事件与 MQ（单机单体，R-09；审计为进程内 OHS 出站调用，D-09）。监听端口清单条目与对照结论为过程数据（读取响应即消亡，domain-model.md 第三部分注），经 /firewall/port/list 呈现、不入领域对象。

### 1.5 上下文映射

| 上下文 | 关系 | 机制 |
|--------|------|------|
| account-auth | 遵奉者 + OHS 消费 | 全部接口位于 Cookie 会话后（R-02，AuthRequired）；readonly 同等查看、写操作 admin 专属（ADR-018，RoleRequired + AUTH_008 边界拒绝）；写接口 CSRF（R-02，CsrfGuard）；审计写入 AuditService.Record（R-03，D-09 降级语义） |
| panel-core | 消费（配置值读取） | 面板监听端口经 internal/config 进程内直读（D-06），作 isPanelPort 标注与涉险判定基准；外壳挂载归 panel-core |
| 本机 ufw / firewalld | 投影与动作源 | firewallx 适配器命令通道（R-08 参数数组禁 shell）；本机系统组件非外部业务系统，无 ACL 必要（R-09 零外呼）；非外部系统对接（与 monitoring/service-mgmt/database-mgmt 本机资源口径一致） |
| 前端防火墙页 | 唯一消费方 | 仅经 5 个 HTTP 接口消费（5.1）；无其他业务模块依赖本模块 |

> 跨聚合根设计（第 2 章）：不适用——本模块无聚合根（工具类裁剪矩阵第 2 章「—」），对象级约束以不变量形式列于 4.1.1。

## 3. 应用/基础设施层设计

### 3.1 接口规范

| 项 | 规范 |
|----|------|
| 路径格式 | `/firewall/{resource}/{action}` 三段式（域=firewall；资源=backend / rule / port；动作段含 3 查询 + 2 写）；资源单数（与 account-auth/service-mgmt 资源单数风格一致） |
| HTTP 方法 | 读=GET（status/list×2），写=POST（create/delete，面板内部契约，account-auth D-07 同一约定） |
| 响应格式 | `{"code": 0, "message": "ok", "data": {...}}`；code=0 成功，非 0 见 3.4 错误码（与 account-auth D-07 同一契约） |
| 时间格式 | ISO-8601 UTC（公共约定）；generatedAt 为投影生成时刻 |
| 认证 | 复用 account-auth：HttpOnly Cookie `dg_session`（AuthRequired） |
| CSRF | 全部 POST 须带 `X-CSRF-Token` 头（R-02，CsrfGuard）；GET 豁免 |
| 中间件链 | AuthRequired → CsrfGuard（POST）→ RoleRequired（读=admin+readonly，写=admin） |
| 用户类型标注 | 读接口：admin+readonly（ADR-018 全可看）；写接口：admin（readonly 无界面入口 + 越界 AUTH_008 全局边界拒绝） |

### 3.2 依赖与约束

| 依赖 | 类型 | 约束 |
|------|------|------|
| account-auth 中间件 | 框架 | AuthRequired/CsrfGuard/RoleRequired 注册顺序见 3.1；不得自建会话解析与角色判定 |
| account-auth AuditService.Record | OHS | 操作审计经其写入（R-03）；失败重试一次后日志兜底（D-09），不得自建审计表 |
| 本机 ufw/firewalld（firewallx 适配器） | 数据源与动作通道 | 全部交互参数数组执行（R-08，禁 shell 拼接）；命令超时 30s（编译期常量）；通道失败归一 FW_008/FW_010 语义 |
| panel-core internal/config | 配置值读取 | 面板监听端口进程内只读直读（D-06）；不反向写、不持有其聚合引用 |
| 监听端口取数通道 | 系统实况读取 | 与系统监控采集统一取数来源（D1 口径-3），单一只读通道；部分失败条目置空标注不虚构 |
| panel-core | 框架 | 路由组挂载 `/firewall`；不得越界承载业务逻辑 |

### 3.3 权限模型

角色：admin / readonly（panel_role 固定枚举）；本模块无访客可达面（全部位于登录会话后）。

**接口权限矩阵**：

| 接口 | admin | readonly | CSRF |
|------|-------|----------|------|
| GET /firewall/backend/status | ✓ | ✓（ADR-018 全可看，含无后端结论） | —（GET） |
| GET /firewall/rule/list | ✓ | ✓（ADR-018 全可看） | —（GET） |
| GET /firewall/port/list | ✓ | ✓（ADR-018 全可看） | —（GET） |
| POST /firewall/rule/create | ✓ | ✗（无界面入口 + AUTH_008 边界拒绝，ADR-018） | ✓ |
| POST /firewall/rule/delete | ✓ | ✗（同上） | ✓ |

数据权限规则：无行级隔离——规则与对照视图两角色可见集完全一致；角色差异仅体现在写接口（admin 专属）。readonly 越界由全局中间件 RoleRequired 拦截返回 AUTH_008（附边界原因），接口层不自建权限分支。

### 3.4 公共规范

**错误码**（FW_ 前缀为本模块新增 10 个；鉴权/CSRF/越界复用 account-auth 码族）：

| code | 标识 | HTTP 语义 | 说明 |
|------|------|----------|------|
| 0 | OK | 成功 | — |
| FW_001 | INVALID_RULE_PARAM | 参数错误 | 端口/端口段/协议/动作取值非法（1-65535、N≤M、枚举值域；R-14/D-05） |
| FW_002 | INVALID_SOURCE | 参数错误 | 来源限制 CIDR 非法（IPv4/IPv6 网段格式，D-05） |
| FW_003 | RULE_NOT_FOUND | 拒绝 | 删除目标不存在（面板外已删，D-07；不静默成功） |
| FW_004 | ACTION_NOT_SUPPORTED | 拒绝 | 动作不在当前后端支持面（firewalld×deny/reject/limit，I-04/D-01） |
| FW_005 | PANEL_PORT_AT_RISK | 需二次确认 | 面板端口涉险且未确认（data 携带命中详情；req.confirmDanger=true 后放行，I-05/D-06） |
| FW_006 | RULE_ALREADY_EXISTS | 参数冲突 | 等价规则已存在（ufw skipping/firewalld already enabled 语义透传，D-07） |
| FW_007 | RUNTIME_APPLY_FAILED | 部分生效 | firewalld permanent 已写入、runtime 应用失败（data 两层状态；不回滚，D-08） |
| FW_008 | BACKEND_EXEC_FAILED | 操作失败 | 后端命令执行失败（权限不足/后端内部错误；data.reason 如实携带） |
| FW_009 | NO_BACKEND_DETECTED | 拒绝 | backend=none 时访问规则面（前端应据 status 呈现引导页；**不返回空规则列表**，I-01） |
| FW_010 | BACKEND_NOT_ACTIVE | 通道不可用 | 后端已识别但不可用（如 firewalld 服务停止）；不返回过期数据（I-01） |
| AUTH_001/002 | （复用） | 401 | 未登录/会话过期——中间件归 account-auth |
| AUTH_003 | （复用） | 403 | CSRF 校验失败（POST） |
| AUTH_008 | （复用）FORBIDDEN | 拒绝 | readonly 越界写操作，附边界原因（ADR-018） |

**枚举**（均在 workspace/01-solution/04-business-enum.md 定义；四个防火墙枚举 D2 已定稿，本次为引用无新增）：

| 枚举标识 | 取值 | 用途 |
|---------|------|------|
| firewall_backend | ufw \| firewalld \| none | 识别结论（status 响应 backend 字段；none=无后端引导态） |
| firewall_rule_action | allow \| deny \| reject \| limit | 规则动作（rule 响应 action 字段 + create 请求参数 + supportedActions 权威面） |
| firewall_rule_protocol | tcp \| udp | 规则协议（rule 响应 protocol 字段 + create 请求参数 + list 筛选参数） |
| listen_port_state | allowed_by_rule \| no_allow_rule | 监听端口开放状态（port/list 响应 openState 字段，读取时派生，D-04） |
| panel_role | admin \| readonly | 权限模型角色（引用，account-auth 定稿） |
| audit_action / audit_result | change \| security / success \| failure | 操作审计类别与结果（引用，account-auth 定稿） |

**业务常量**（操作审计细粒度动作码，非枚举池条目）：`firewall_rule_created / firewall_rule_deleted`（action 列存此码，category=audit_action.change，object_type=`firewall_rule`，object_id=ruleKey）。

**业务常量**（操作参数，非枚举池条目）：

| 常量 | 值 | 说明 |
|------|-----|------|
| 后端命令超时上限 | 30s | firewallx 适配器单命令等待上限（编译期常量；超时归一 FW_008） |
| 端口形态 | `^([1-9]\d{0,4})(-([1-9]\d{0,4}))?$` + 1-65535 + N≤M | FW_001 校验依据（D-05） |
| confirmDanger 字段 | boolean，缺省 false | FW_005 两段式确认载体（I-05/D-06）；每次涉险操作独立要求、不记忆 |

**配置项**：无——命令超时/对照口径为编译期常量与代码逻辑（无用户可配置面）；无新增配置键。

## 4. 领域对象与服务详细设计

> 本模块无聚合根（工具类），本章按领域对象组织；各节保持与先例一致的子节结构（领域模型→状态模型→子流程→接口设计→事件与集成）。

### 4.1 firewall_rule（防火墙端口规则投影，实体 + 识别/对照链路）

#### 4.1.1 领域模型

**表结构组合**：无表（D-10 零持久化）。firewall_rule 驻留进程内，为每次后端读取时重建的投影集合（读取即刷新，无驻留缓存层）；写操作直达后端（命令契约），不经投影对象中转持久化。

**投影条目字段清单**（status/rule/port 响应的共同数据源）：

| 字段 | 类型 | 说明 |
|------|------|------|
| ruleKey | string | 后端原生命令定位串（D-03），投影标识与删除寻址（不透明引用） |
| port | string | 端口显示形（"80" 或 "8080-8090"，D-05 形态） |
| protocol | enum(firewall_rule_protocol) | tcp / udp |
| direction | "ingress"（常量） | 方向固定入站（MVP 单值维度，非枚举池条目；响应呈现用） |
| source | string \| null | 来源限制（CIDR 网段；null=任意来源） |
| action | enum(firewall_rule_action) | allow / deny / reject / limit |
| rawSpec | string | 后端原始语义标识（ufw 规则原文 / firewalld 端口串），与后端自身工具输出互认 |
| isPanelPort | bool | 服务端计算：规则端口数值直接命中面板监听端口（D-06 从严口径，D1 口径-5） |

**监听端口对照条目**（port/list 载荷，过程数据——读取响应即消亡，不入领域对象）：

| 字段 | 类型 | 说明 |
|------|------|------|
| proto / addr / port | string / string / number | 监听套接字三元组（proto 透传 tcp/tcp6/udp/udp6；addr 原样监听地址） |
| process | {pid:number\|null, name:string\|null} \| null | 监听进程（取不到时 null，不虚构——monitoring system_info 同口径） |
| external | bool | 服务端计算：监听地址非 loopback（D-04） |
| openState | enum(listen_port_state) | 对照派生值（allow 族命中端口+协议→allowed_by_rule，D-04） |

**不变量**：

| 编号 | 不变量 | 违反处理 |
|------|--------|---------|
| I-01 | 投影真实性：rule/port 响应恒为当次后端/系统实况读取快照；无缓存层、无陈旧数据回退；backend=none 访问规则面返回 FW_009（**不返回空规则列表**——无后端≠无规则）；后端已识别但不可用返回 FW_010 | 读取侧保证（每次请求即时读取，D-01） |
| I-02 | 零持久化：firewall_rule 不落库，面板重启无本地规则状态；面板外增删经下次读取如实反映；规则历史不在本模块留存（审计归 account-auth） | 结构保证（无表） |
| I-03 | 命令纪律：全部后端交互经 FirewallGateway 参数数组执行（R-08 禁 shell 拼接），单命令 ≤30s；Gateway 为唯一后端交互出口 | 结构保证（适配层唯一出口） |
| I-04 | 动作面边界：动作支持面由后端能力决定（ufw 全四值 / firewalld 仅 allow，D1 口径-2）；越面提交 FW_004；supportedActions 恒为服务端权威计算 | RuleMutationService 校验（D-01） |
| I-05 | 面板端口保护强制：涉险操作（create deny/reject 命中面板端口、delete allow 族规则命中面板端口）未携 confirmDanger=true 必拒（FW_005）；确认后放行——保护为警示语义不阻止操作本身 | RuleMutationService 编排（D-06） |
| I-06 | 双写如实反馈：firewalld 两步命令结果如实呈现（全成=ok / runtime 失败=FW_007 携两层状态 / permanent 失败=FW_008）；任何情况不虚构全成功 | FirewallGateway 结果归一（D-08） |
| I-07 | 可审计：每个 create/delete 调用恰好产生一条审计记录（success/failure 如实；FW_005 拒绝未发生变更不记，R-03 字面口径）；审计写入失败按 D-09 降级并日志兜底 | RuleMutationService 编排 |
| I-08 | 等价规则不静默：已存在 → FW_006；删除不存在 → FW_003（不静默成功，幂等口径） | RuleMutationService 校验（D-07） |

**领域服务**：BackendDetector（探测编排：ufw 优先→firewalld，结论+运行状态+动作面组装）、RuleProjector（规则投影组装+筛选排序+isPanelPort 标注）、PortComparisonService（监听端口×规则投影对照+external 计算）、RuleMutationService（写编排：校验→涉险保护→命令执行→审计）。

#### 4.1.2 状态模型

面板侧无自有状态机写入（I-02 零持久化）；「规则有无即状态」为 D0 拍板口径，规则存在性=后端侧记录的存在与删除，无状态字段流转。

辨析：① backend=ufw/firewalld/none 为每次探测整值刷新的**判定值**（firewall_backend），非状态机；② 后端运行状态（active）为后端自身实况透传属性（面板不提供启停，PRD OUT-5）；③ openState 为规则投影与监听实况的**读取时派生值**（与 instance_conn_state 探测态同构，PRD 第六部分辨析）；④ create/delete 在途为一次性执行过程（P7）；⑤ 无后端引导呈现为 status 结论 none 的降级视图。无审批/驳回环节（面板端口保护二次确认为交互层危险操作防护，非审批流——PRD 第六部分末注）；无状态历史表（操作追溯经 account-auth audit_log）。

#### 4.1.3 子流程设计

无（后端识别为进入模块的只读判定链路、规则增删为单动作即时执行、面板端口保护为提交动作的同步前置守卫、firewalld 双写为命令执行口径——均无独立生命周期状态，domain-model.md 第四部分已声明）。

#### 4.1.4 接口设计

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /firewall/backend/status | GET | admin+readonly | 后端识别结论（类型/运行状态/动作支持面） |
| /firewall/rule/list | GET | admin+readonly | 规则投影清单（筛选+面板端口标注） |
| /firewall/port/list | GET | admin+readonly | 监听端口对照清单（开放状态） |
| /firewall/rule/create | POST | admin | 添加规则（面板端口保护两段式） |
| /firewall/rule/delete | POST | admin | 删除规则（面板端口保护两段式） |

**接口详情**（请求/响应/校验）：

- **GET /firewall/backend/status**：无参数。处理：即时探测（D-01，I-01 无缓存）。响应 data=`{backend: firewall_backend, active: bool, version: string|null, supportedActions: [firewall_rule_action], generatedAt}`——backend=none 时 active=false、version=null、supportedActions=[]（前端据此渲染引导提示页，不调用规则面）；backend=ufw/firewalld 但通道不可用时探测结论仍返回（backend/active 如实），后续规则面调用返回 FW_010。审计：无（查看类）。
- **GET /firewall/rule/list**：query `{action?, protocol?}`——多值逗号分隔（如 `action=allow,limit`），任一值非法 → FW_001（R-14）。处理：后端可用性前置（backend=none → FW_009；不活跃 → FW_010）→ 即时读取全量投影（I-01/I-02）→ 服务端筛选+排序（D-02）→ isPanelPort 标注（D-06）。响应 data=`{items:[{ruleKey, port, protocol, direction, source, action, rawSpec, isPanelPort}], total, backend:{backend, active}, generatedAt}`——backend 摘要随行携带（页头呈现，宿主 S-42-01 结论）。审计：无（查看类）。
- **GET /firewall/port/list**：无参数。处理：后端可用性前置（同上）→ 监听端口实况读取（统一取数通道）+ 规则投影对照（D-04）。响应 data=`{items:[{proto, addr, port, process, external, openState}], total, generatedAt}`——process 取不到为 null（不虚构）；对照仅入站方向（出站 OUT-3）；风险敞口提示由前端按 `openState=no_allow_rule ∧ external=true` 渲染（D4）。审计：无（查看类）。
- **POST /firewall/rule/create**：req `{port, protocol, source?, action, confirmDanger?}`。校验顺序（R-14/I-04/I-05）：参数值域（port/protocol/action → FW_001；source → FW_002）→ 后端可用（FW_009/FW_010）→ 动作支持面（FW_004，对 supportedActions）→ 面板端口涉险（action∈{deny,reject} 且命中面板端口且 confirmDanger≠true → FW_005，data=`{panelPort, rulePort, action, hint}`）→ 后端执行（等价规则 FW_006；firewalld runtime 失败 FW_007；其他失败 FW_008）。成功 data=`{ruleKey, port, protocol, action}`（写入后定位引用）。审计：firewall_rule_created（change；detail={port, protocol, source, action, confirmDanger}；结果按执行实况——FW_006/007/008 为 failure，I-07）。
- **POST /firewall/rule/delete**：req `{ruleKey, confirmDanger?}`。校验顺序：ruleKey 非空（FW_001）→ 后端可用（FW_009/FW_010）→ 存在性（FW_003，实况复核）→ 面板端口涉险（目标为 allow 族且命中面板端口且 confirmDanger≠true → FW_005，data 同上）→ 后端执行（firewalld runtime 失败 FW_007；其他失败 FW_008）。成功 data=`{deleted: true, ruleKey}`。审计：firewall_rule_deleted（change；detail={ruleKey}；结果如实，I-07）。

**跨接口复用规则**：后端可用性前置（FW_009/FW_010 判定）统一由 BackendDetector 结论+通道探测组合承载；涉险判定统一走 PanelPortGuard（读 panel-core 监听配置 + 端口数值直接命中，I-05/D-06）；写操作审计统一走 `AuditService.Record`（D-09）；命令执行统一经 FirewallGateway（I-03）。

**幂等语义**：create 非幂等（等价规则重复提交 FW_006 报错，不静默合并）；delete 非幂等（不存在 FW_003，不静默成功）；status/rule/port 读取无副作用（纯投影）；confirmDanger 每次涉险操作独立要求（不记忆确认，PRD N3）。

#### 4.1.5 领域事件与集成

无 MQ、无领域事件（单机单体，R-09）。「仓储契约」为零持久化替代——**进程内网关/取数契约（非 SQL；R-08 唯一后端交互出口）**：

```
FirewallGateway {
  Detect() (BackendInfo, error)                    // ufw 优先→firewalld 探测（只读；BackendInfo={Type, Active, Version})
  ListRules() ([]BackendRule, error)               // 端口级规则实况投影（I-01/I-02；含原文语义）
  AddRule(spec RuleSpec) (AddResult, error)        // ufw: allow/deny/reject/limit（参数数组）；firewalld: permanent+runtime 双写（D-08，AddResult 含两层状态）
  DeleteRule(ruleKey string) (DelResult, error)    // ufw: delete <spec>；firewalld: --remove-port 双写（D-08）
}
ListenPortSource { List() ([]ListenPort, error) }  // 监听端口实况（与系统监控采集统一来源，D1 口径-3；单一只读通道）
PanelListenConfig { GetListenPort() (uint16, error) } // panel-core internal/config 进程内只读直读（D-06）
约束：全部命令参数数组执行（R-08 禁 shell 拼接）；单命令 ≤30s；通道错误归一 FW_008/FW_010 语义；
      等价规则/双层结果等后端原生语义如实上抛（D-07/D-08），由 RuleMutationService 映射错误码
```

关联域调用（出站）：`AuditService.Record`（account-auth OHS，操作审计 R-03；D-09 降级语义）；`PanelListenConfig.GetListenPort`（panel-core 配置值读取，D-06）；无其他出站。被调用契约：无（本模块为消费终模块，1.5）。

## 5. 模块清单汇总

### 5.1 对外服务清单（HTTP 接口）

| 服务（接口） | 消费方 | 契约要点 |
|-------------|--------|---------|
| GET /firewall/backend/status | 前端防火墙页（唯一消费方） | 即时探测结论 + 服务端权威动作面（D-01） |
| GET /firewall/rule/list | 前端防火墙页 | 全量投影 + 筛选（D-02）+ isPanelPort 标注 |
| GET /firewall/port/list | 前端防火墙页 | 监听×规则对照（D-04）+ external 维度 |
| POST /firewall/rule/create | 前端防火墙页（admin） | 动作面/涉险两段守卫（FW_004/FW_005）；firewalld 双写（D-08） |
| POST /firewall/rule/delete | 前端防火墙页（admin） | ruleKey 寻址（D-03）+ 涉险两段守卫 |
| 中间件 AuthRequired / CsrfGuard / RoleRequired | （复用） | account-auth 提供，顺序见 3.1 |
| AuditService.Record | （出站调用） | 操作审计写入（R-03，D-09） |

本模块无对外 OHS 服务契约——服务消费终模块（1.5：无其他业务模块依赖）。

### 5.2 SQL 清单

**无**——本模块零持久化（D-10）：firewall_rule 为后端实况投影，无任何 SQLite 表读写（domain-model.md 存储映射说明「firewall→无表」）；操作审计落库由 account-auth 的 AuditRepository 承载（非本模块 SQL）。

### 5.3 DDL

**无**——零持久化（同 5.2；本模块不新增任何表，面板库 schema 无变更）。

## 6. 更新记录

### 6.1 变更记录

| 版本 | 日期 | 变更类型 | 变更摘要 | 目标节号 |
|------|------|---------|---------|---------|
| v1.0 | 2026-11-14 | initial | 首次生成：5 个 HTTP 接口（GET /firewall/{backend/status, rule/list, port/list} + POST /firewall/rule/{create,delete}，读=admin+readonly / 写=admin+CSRF）+ 3 个进程内网关/取数契约（FirewallGateway / ListenPortSource / PanelListenConfig；零持久化：无表/无 DDL/无 SQL）+ 10 错误码（FW_001~010）+ 复用 account-auth 码族（AUTH_001/002/003/008）+ 6 枚举引用（四个防火墙枚举 D2 已定稿，无新增）+ 2 个审计动作常量 + 10 项设计决策（D-01 即时探测+权威动作面、D-02 全量筛选、D-03 ruleKey 寻址、D-04 对照口径、D-05 校验集、D-06 面板端口保护服务端两段式、D-07 等价规则如实、D-08 双写两步、D-09 审计非同事务、D-10 零持久化）；关闭 prd-firewall.md 待对齐-1~-9 全部九项；无旧版对比 | — |

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
