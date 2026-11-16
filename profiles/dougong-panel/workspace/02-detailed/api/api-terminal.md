# 斗拱面板 Web 终端（terminal）API 设计

| 属性 | 值 |
|------|-----|
| 版本 | v1.1 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-14 |
| 输入 | prd-terminal.md v1.0（完备性检查通过：G 5/5、T 适用 4/4、N 4/4）、domain-model.md v1.11（terminal 域：1 聚合根 terminal_session 内存态 + 2 内嵌值对象）、d0-clarification-terminal.md、D1 三文档（BA/AA/FL v1.0） |
| 接口规约 | 无激活技术栈（ACTIVE_STACKS 为空）——按通用 REST/JSON 契约设计；认证=HttpOnly Cookie 会话（R-02，WS 握手级鉴权复用 R-18）；鉴权=RBAC 两角色边界矩阵（ADR-018，本模块 admin 专属）；**零持久化**（D0 拍板：无表、无 DDL、无 SQL；会话内存态断开即释放） |

## 1. 模块设计概述

### 1.1 业务背景及目标

Web 终端是斗拱面板 M6 里程碑模块、全面板安全暴露面最大的能力：为管理员提供浏览器内服务器 shell（xterm.js ↔ WebSocket ↔ 服务端 PTY），免去 SSH 客户端；以「复用安全基座 + 严格限流」为中心——握手复用面板会话与角色判定（R-18，readonly 整域拒绝 ADR-018）、每用户并发 5 / 空闲 30 分钟 / 单连接硬上限 8 小时（ADR-023 标准档）、面板会话失效联动断开（R-18 反向联动）；会话零持久化，断开即无痕，开合两端留安全审计（R-03 精神，D1 解释性决策）。服务对象：单机自托管的管理员（admin）——本模块无 readonly 可达面。

### 1.2 边界说明

| 项 | 说明 |
|----|------|
| 本模块做 | 终端 WS 通道（握手准入 + 双向帧流 + 断开通知）、系统用户候选清单查询（指定身份）、会话生命周期自动管控（空闲/硬上限/面板会话失效联动/统一清理） |
| 本模块不做（接口边界） | ① 容器内终端——docker-mgmt 二期；② 会话录制/回放——非目标；③ SFTP——file-manager；④ 多机终端——非目标（§4.2 单机）；⑤ 会话清单页面——非目标（PRD OUT-5）；⑥ 会话限流阈值配置——固定标准档非配置项（PRD OUT-6）；⑦ 认证/鉴权/审计本体——复用 account-auth（SessionAuthService.VerifyWs / AuditService.Record，本模块仅声明依赖）；⑧ 终端页面外壳与导航——panel-core |
| 与存储的关系 | **零持久化**：无表、无 DDL、无 SQL（D0 拍板；domain-model.md 存储映射说明「terminal→无表」）；terminal_session 为进程内对象，面板重启即全部消亡（I-04） |

### 1.3 设计决策

| # | 决策 | 理由 |
|---|------|------|
| D-01 | WS 消息协议=**JSON 文本帧 + data Base64**：客户端→服务端 `input`（键入字节）/`resize`（cols/rows）；服务端→客户端 `ready`（会话元信息，连接后首帧）/`output`（输出字节）/`closed`（断开通知：reason/closedAt/durationSeconds）。单帧 data 解码后 ≤16KB，超限或未知帧类型=协议违规断开（I-06） | JSON 帧可扩展且前端零二进制协议负担；Base64 承载 PTY 任意原始字节（控制字符/非 UTF-8 序列）不破坏 JSON 结构；ready 帧使前端获得 sessionId/身份/限额的权威值。**关闭 prd-terminal.md 待对齐-1** |
| D-02 | system_user 候选清单=**本模块直读系统实况**（SysUserDirectory 网关，/etc/passwd 语义经适配器，服务端过滤非交互 shell），不复用 file-manager /files/system-accounts 接口 | 跨模块 HTTP 自调用违背单体进程内协作惯例；两模块各自直读本机资源（与 monitoring gopsutil / service-mgmt dbus 同口径），零耦合；shell 过滤口径（排除 nologin/false/halt/shutdown 等）为本模块 D1 落定边界。**关闭 prd-terminal.md 待对齐-2** |
| D-03 | 断连检测=**WS 标准心跳**：服务端每 30s 发 ping，连续 2 次未收 pong 判 network_error 断开并走统一清理；孤儿进程回收随清理即时（进程组 Kill + Wait 收割） | WS 标准协议层探活，无需应用层心跳帧；30s×2 粒度覆盖浏览器崩溃/断网/休眠场景。**关闭 prd-terminal.md 待对齐-3** |
| D-04 | 输出通道=**每会话 256KB 环形缓冲**：高速输出（如 cat 大文件）超出消费速率时丢弃最旧数据并在恢复时发 `{type:"output", truncated:true}` 标记；不因输出洪峰断开连接、不重置空闲计时 | 背压策略保连接可用性优先（终端滚屏本就是丢弃式消费）；丢弃标记保呈现诚实。**关闭 prd-terminal.md 待对齐-4** |
| D-05 | 断开前预告=**MVP 不做**：空闲/硬上限临期不发预告帧，断开事实经 closed 帧（含原因与时长）一次性完整反馈 | D0/D1 从未承诺预告（PRD 待对齐-5 明示「如承诺需回补 BA」）；预告通道增加扫描与帧复杂度，收益有限。**关闭 prd-terminal.md 待对齐-5（决定=不做）** |
| D-06 | 审计口径分建立/断开两段：**建立审计失败→拒绝建立会话**（TERM_006，回收已 spawn 的 PTY，root shell 不容无痕开）；**断开审计失败→运行日志兜底**（断开既成事实不可回滚，继续完成清理），成功路径两段各恰好一条（I-07） | 建立是可前置拒绝的守卫点（PRD N2 从严）；断开与 service-mgmt D-06 同降级口径（不可回滚动作如实降级并日志兜底）。**关闭 prd-terminal.md 待对齐-6** |
| D-07 | 面板会话失效联动=**握手 VerifyWs + 存续期定时复核**（每 30s 对每条会话经 SessionAuthService.VerifyWs 只读复核，失效即联动断开 reason=panel_session_invalid）；已登记对 account-auth 的后续协作项：提供失效通知契约（订阅式）可消除复核延迟 | 单体进程内零新增契约即可满足 M5 验收（「面板会话过期后终端连接断开」，最大延迟 30s）；通知式联动为优化项非依赖项（account-auth api v1.1 无该契约，不越权变更他模块文档） |
| D-08 | 混合模式（MIXED）声明：terminal_session 为自管运行时对象（无外部同步源，R-09），不设 manage_mode 字段；无数据 CRUD 侧（建立/断开为生命周期动作非数据变更） | 分册 Step 2 默认混合模式；本模块无托管侧、无持久化数据写入侧 |
| D-09 | WS 握手安全=**Cookie 会话鉴权 + Origin 同源校验**：握手请求带 dg_session（浏览器 WS 自动携带同源 Cookie，SameSite=Strict 不阻同站升级请求）→ VerifyWs 鉴权 → 角色判定；帧层不设 CSRF（R-02 CSRF 面在 HTTP POST 表单，WS 连接自身已绑定会话且不可被跨站表单触发）；Origin 失败复用 AUTH_003 | R-18 逐字复用面板会话；浏览器 WebSocket API 不支持自定义头，Cookie 为唯一原生通道（account-auth D-01 契约兼容）；PRD 第五部分 R-02 行「D3 复核」落定 |

### 1.4 领域对象总览

| 聚合根 | 域内对象/值对象 | 子流程 | 关联领域对象 |
|--------|---------------|--------|-------------|
| terminal_session（主数据类·内存态会话变体：无 CRUD 列表，建立/交互/断开三动作） | 运行身份（内嵌值对象）、限流策略（域级固定值对象，ADR-023） | —（空闲/硬上限判定为聚合内定时机制，domain-model.md 第四部分声明） | —（鉴权会话引用为运行时校验凭据非聚合 ID；system_user 为系统实况只读引用，见 domain-model.md 第五部分） |

### 1.5 上下文映射

| 上下文 | 关系 | 机制 |
|--------|------|------|
| account-auth | 遵奉者 + OHS 消费 | WS 握手与存续期复核经 SessionAuthService.VerifyWs（R-18，只读验证）；审计经 AuditService.Record（security 类，D-06 两段口径）；readonly 拒绝复用 AUTH_008 边界语义（ADR-018） |
| system_user（本机系统实况） | 只读引用源 | SysUserDirectory 网关直读（D-02），非面板域对象、无 ACL 必要（R-09 零外呼） |
| PTY（本机，creack/pty） | 运行时载体 | PtyGateway 参数数组 spawn（R-08 禁 shell 拼接）；su 语义代启指定用户登录 shell、不代输密码（D1 落定） |
| panel-core | 消费其 HTTP 骨架 | `/terminal` 路由组挂载；入口面/外壳归 panel-core（R-10） |
| 前端终端页 | 唯一消费方 | 1 HTTP + 1 WS 端点（5.1）；docker-mgmt 容器内终端为二期预期消费方（不预先建模） |

> 跨聚合根设计（第 2 章）：不适用——本模块单聚合根（裁剪矩阵第 2 章「仅多聚合根模块」），对象级约束以不变量形式列于 4.1.1。

## 3. 应用/基础设施层设计

### 3.1 接口规范

| 项 | 规范 |
|----|------|
| 路径格式 | `/terminal/{resource}/{action}` 三段式；资源单数（user/session）；本模块含 1 个 HTTP 接口与 1 个 WebSocket 升级端点（connect 动作） |
| HTTP 方法 | 读=GET（options）；WS 端点=GET + `Upgrade: websocket`（握手即准入，见 D-09） |
| 响应格式 | HTTP 同全局 `{"code": 0, "message": "ok", "data": {...}}`；WS 握手拒绝=HTTP 4xx/5xx + 错误码 JSON（升级前仍是 HTTP 语义）；升级成功后按 D-01 帧协议 |
| 时间格式 | ISO-8601 UTC（公共约定）；closedAt 为断开时刻 |
| 认证 | 复用 account-auth：HttpOnly Cookie `dg_session`（握手级，R-18/R-02）；浏览器 WS 自动携带同源 Cookie |
| CSRF | HTTP GET 豁免；WS 端点豁免（D-09：握手会话鉴权 + Origin 同源校验替代，帧层无 CSRF 面） |
| 中间件链 | HTTP：AuthRequired → RoleRequired（admin）；WS：TerminalHandshakeGuard（Origin 同源 → VerifyWs → 角色判定 → 准入编排，4.1.4） |
| 用户类型标注 | 全部端点：admin（readonly 无界面入口 + 越界 AUTH_008 边界拒绝，ADR-018——本模块为全项目唯一 readonly 完全无功能模块） |

### 3.2 依赖与约束

| 依赖 | 类型 | 约束 |
|------|------|------|
| account-auth SessionAuthService.VerifyWs | OHS | 握手与 30s 存续期复核共用（R-18，只读不续期）；不得自建会话解析与角色判定 |
| account-auth AuditService.Record | OHS | 会话开合两段审计（D-06；建立失败拒建 TERM_006 / 断开失败日志兜底）；不得自建审计表 |
| 本机 PTY（creack/pty，PtyGateway） | 运行时载体 | spawn 一律参数数组（R-08 禁 shell 拼接）；指定用户=su 语义代启登录 shell、不代输密码；进程组 Kill+Wait 收割（D-03） |
| 系统用户实况（SysUserDirectory） | 只读数据源 | /etc/passwd 语义直读（D-02）；服务端过滤非交互 shell；不可用返回 TERM_007 |
| panel-core | 框架 | 路由组挂载 `/terminal`；WS 升级透传；不得越界承载业务逻辑 |

### 3.3 权限模型

角色：admin / readonly（panel_role 固定枚举）；本模块无访客与 readonly 可达面（R-18 握手即拒）。

**端点权限矩阵**：

| 端点 | admin | readonly | CSRF |
|------|-------|----------|------|
| GET /terminal/user/options | ✓ | ✗（无界面入口 + AUTH_008 边界拒绝，ADR-018） | —（GET） |
| WS /terminal/session/connect | ✓ | ✗（握手角色判定拒绝，AUTH_008 附边界原因） | 豁免（D-09） |

数据权限规则：无行级隔离亦无集合共享——readonly 对本模块零可达；admin 仅能操作自己建立的会话（会话归操作者私有，无跨用户会话操作面，I-05 并发计数同口径）。

### 3.4 公共规范

**错误码**（TERM_ 前缀为本模块新增 7 个；鉴权/CSRF/越界复用 account-auth 码族）：

| code | 标识 | HTTP 语义 | 说明 |
|------|------|----------|------|
| 0 | OK | 成功 | — |
| TERM_001 | CONCURRENT_LIMIT | 拒绝 | 同操作者存续终端连接已达上限 5（ADR-023）；data=`{limit:5, current:5}`，提示先关闭既有会话 |
| TERM_002 | SYSTEM_USER_NOT_FOUND | 拒绝 | 指定的系统用户不存在（R-14 兜底校验） |
| TERM_003 | NO_LOGIN_SHELL | 拒绝 | 指定用户的 shell 非交互（nologin/false 等，D1 落定边界） |
| TERM_004 | PTY_SPAWN_FAILED | 失败 | PTY 进程创建失败（系统资源不足等）；不产生会话对象、不占并发配额 |
| TERM_005 | VALIDATION_FAILED | 参数错误 | data.fields[] 逐字段错误（R-14：identitySource 枚举值/username 形态） |
| TERM_006 | AUDIT_UNAVAILABLE | 拒绝 | 建立审计写入失败→从严拒建（D-06，回收已 spawn 的 PTY） |
| TERM_007 | SYSTEM_USER_DIR_UNAVAILABLE | 失败 | 系统用户实况读取失败（候选清单接口，不返回虚构数据） |
| AUTH_001/002 | （复用） | 401 | 握手未登录/面板会话过期（VerifyWs 判定） |
| AUTH_003 | （复用） | 403 | 握手 Origin 非同源（D-09） |
| AUTH_008 | （复用）FORBIDDEN | 拒绝 | readonly 越界（ADR-018 整域禁用，附边界原因） |

**枚举**（均在 workspace/01-solution/04-business-enum.md 定义；本模块 2 枚举 D2 已定稿，D3 对 terminal_end_reason 增补 1 取值，无新增枚举）：

| 枚举标识 | 取值 | 用途 |
|---------|------|------|
| terminal_end_reason | self_close \| idle_timeout \| hard_limit \| panel_session_invalid \| network_error \| protocol_error | closed 帧 reason + 断开审计 detail.reason；protocol_error 为 D3 增补（协议违规断开：未知帧类型/超限帧，I-06——接口层防御路径，非 PRD 第六部分 5 类业务触发） |
| terminal_identity_source | panel_run_user \| specified_system_user | 握手 identitySource 参数 + 会话运行身份载体（R-18 两分支） |
| panel_role | admin \| readonly | 权限模型角色（引用，account-auth 定稿） |
| audit_action / audit_result / audit_source | change \| security / success \| failure / web \| cli \| system | 会话开合审计类别/结果/来源（引用，account-auth 定稿；closed 事件 source 按 D-06 映射） |

**接口协议常量**（WS 帧类型与协议参数，非枚举池条目——纯传输协议无业务取值语义，与 account-auth「审计动作码非枚举池」同口径）：

| 常量 | 值 | 说明 |
|------|-----|------|
| frame_type | ready \| input \| resize \| output \| closed | D-01 帧协议；ready=服务端首帧（会话元信息），input/resize=客户端帧，output/closed=服务端帧 |
| 单帧 data 解码后上限 | 16KB | 超限=协议违规断开（I-06） |
| 输出环形缓冲 | 256KB/会话 | D-04 背压；溢出丢弃最旧 + truncated 标记 |

**业务常量**（限流与调度参数，非枚举池条目）：

| 常量 | 值 | 说明 |
|------|-----|------|
| 每用户并发上限 | 5 | ADR-023 编译期常量（TERM_001） |
| 空闲超时 | 30min | ADR-023；以无操作者输入为准（input/resize 帧更新 lastActiveAt，服务端持续输出不重置——D1 从严口径） |
| 单连接硬上限 | 8h | ADR-023；=createdAt+8h，与活跃无关 |
| 生命周期扫描 tick | 10s | 空闲/硬上限判定精度 |
| 面板会话复核间隔 | 30s | D-07 联动断开最大延迟 |
| WS 心跳 | ping 每 30s，连续 2 次未答判 network_error | D-03 |
| 非交互 shell 排除集 | nologin、false、halt、shutdown 及空 shell | D1 落定边界（候选清单服务端过滤 + TERM_003 判定共用） |
| 审计动作码 | terminal_session_opened / terminal_session_closed | action 列存此码，category=audit_action.security，object_type=`terminal_session`，object_id=sessionId |

**配置项**：无——全部参数为编译期常量（ADR-023 固定标准档，PRD OUT-6 明示不做面板化配置）；无新增配置键。

## 4. 聚合根详细设计

### 4.1 terminal_session（主数据类·内存态会话变体）

> Step 1 判定：有聚合根、有写操作（建立/断开）、状态=生命周期（活跃→断开·销毁）→ 主数据类；变体说明——无 CRUD 列表（PRD OUT-5）、无 options 集合管理、对象不持久化（I-04），接口形态=WS 通道三动作（connect 建立 / 帧交互 / 断开）。

#### 4.1.1 领域模型

**表结构组合**：无表（I-04 零持久化）。terminal_session 驻留进程内 TerminalSessionRegistry（并发安全 map + 定时扫描）；「仓储契约」为零持久化替代（4.1.5）。

**运行时字段清单**（进程内对象，非 DDL）：

| 字段 | 类型 | 说明 |
|------|------|------|
| sessionId | string | 会话标识（8 字节随机 hex）；审计弱引用 object_id 与日志定位 |
| userId | int | 操作者 panel_user.id（并发计数与私有归属口径，I-05） |
| panelSessionRef | 运行时凭据 | 面板会话引用（握手 Cookie 会话的复核凭据，D-07 30s 复核与失效联动触发源；非聚合 ID，见 domain-model 第五部分；Gate 1 W1 补列） |
| identity | 值对象 | `{source: terminal_identity_source, runUser: string}`；会话期内不可变（换身份=新建会话，D1 落定） |
| createdAt | time | 建立时刻；hardDeadlineAt=createdAt+8h |
| lastActiveAt | time | 最近一次 input/resize 帧时刻（空闲判定锚点，D1 从严口径：输出不更新） |
| sourceIp / userAgent | string | 连接来源（审计 detail 载荷） |
| pty | PtyHandle | 运行时载体（进程组，R-08） |
| wsConn | WSConn | 帧通道（心跳与背压绑定，D-03/D-04） |
| outBuf | ring 256KB | 输出环形缓冲（D-04） |

**不变量**：

| 编号 | 不变量 | 违反处理 |
|------|--------|---------|
| I-01 | 三重准入：未经「鉴权（VerifyWs）→ 角色（admin）→ 并发（<5）→ 身份（合法且可登录）」全部通过的连接不存在 | TerminalHandshakeGuard 握手编排（AUTH_001/002 → AUTH_008 → TERM_001 → TERM_002/003/005） |
| I-02 | 同生共死：会话记录、PTY 进程、并发配额三者同时创建/同时销毁；清理幂等（重复清理无害）；PTY 不遗留孤儿（进程组 Kill+Wait 收割） | TerminalCleanupService 统一清理（TERM_004/006 时对称回收） |
| I-03 | 双阈值独立生效：idle=lastActiveAt+30min（仅输入刷新）与 hard=createdAt+8h（与活跃无关）任一先到即断；扫描 tick 10s 为判定精度 | TerminalLifecycleService 定时扫描 |
| I-04 | 零持久化：会话对象仅存进程内；面板重启即全部消亡（连接断开）；断开即无痕（无任何落库残留，开合事实由审计承载） | 结构保证（无表） |
| I-05 | 限额与归属：并发计数按 userId 全局累计（跨标签页/跨浏览器）；会话归建立者私有，无跨用户操作面 | Registry.CountByUser 计数（TERM_001） |
| I-06 | 帧协议完整性：仅接受 input/resize 客户端帧；未知类型或单帧解码后 >16KB=协议违规，立即断开（reason=protocol_error） | 帧解码层守卫 |
| I-07 | 可审计：每会话恰好 1 条 terminal_session_opened + 1 条 terminal_session_closed 审计记录；closed 由统一清理点唯一写入（含 reason/durationSeconds）；source 映射：opened=web，closed 的 self_close=web、其余（idle/hard/panel_session_invalid/network_error/protocol_error）=system | TerminalAdmission/CleanupService 编排（D-06） |

**领域服务**：TerminalAdmissionService（握手准入编排 I-01 + 建立审计）、TerminalLifecycleService（10s tick：空闲/硬上限判定 + 30s 面板会话复核 + 30s 心跳，D-03/D-07）、TerminalCleanupService（统一断开清理：closed 帧 → Kill PTY → 注销会话 → 释放配额 → closed 审计，I-02/I-07）、PtyGateway / SysUserDirectory（4.1.5 网关契约）。

#### 4.1.2 状态模型

`活跃（内存态唯一存在态）→ 断开（终态=对象销毁，closed 无持久观察窗）`——6 类触发（terminal_end_reason）：

| 触发 | reason | 判定方 |
|------|--------|--------|
| 客户端 close 帧 / 标签页关闭 | self_close | WS 标准关闭事件 |
| 空闲 30min 无输入 | idle_timeout | Lifecycle 扫描（I-03） |
| createdAt+8h 到期 | hard_limit | Lifecycle 扫描（I-03） |
| 面板会话失效（过期/登出/强制下线） | panel_session_invalid | 30s 复核（D-07） |
| 心跳 2×30s 未答（崩溃/断网） | network_error | 心跳探活（D-03） |
| 协议违规（未知帧/超限帧） | protocol_error | 帧解码守卫（I-06） |

辨析：① 断开即销毁，无 closed 持久态——终态事实经 closed 帧（前端提示）+ terminal_session_closed 审计（account-auth 落库）留痕，与 account-auth session_status{expired,revoked} 落库语义的差异已在 PRD 第六部分辨析；② 并发配额占用/释放为计数语义非状态机；③ 「重连」恒为新建会话（重走 I-01 准入），无断点续接（无录制回放 OUT-2 推论）；④ 身份为建立时参数，会话期内无「切换身份」动作。无审批/驳回环节（PRD 第六部分末注）；无状态历史表（开合追溯经 audit_log）。

#### 4.1.3 子流程设计

无（空闲/硬上限/联动断开为聚合内定时机制与事件消费，无独立生命周期状态——domain-model.md 第四部分已声明）。

#### 4.1.4 接口设计

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /terminal/user/options | GET | admin | 可指定系统用户候选清单（具备登录 shell） |
| /terminal/session/connect | WS（GET+Upgrade） | admin | 终端通道：握手准入 → 帧交互 → 断开（I-01/D-01） |

**接口详情**（请求/响应/校验）：

- **GET /terminal/user/options**：无参数。处理：SysUserDirectory 直读实况（D-02）→ 服务端过滤非交互 shell（排除集见 3.4 常量）。响应 data=`{items:[{name, uid, shell}], generatedAt}`（按 name 字典序升序；单机数十至数百条量级，全量不分页——service-mgmt D-02 同口径）；实况读取失败 → TERM_007（不返回虚构数据）。审计：无（只读候选）。
- **WS /terminal/session/connect**：query `{identitySource, username?}`。握手校验顺序（I-01）：① Origin 同源（失败 AUTH_003，D-09）→ ② VerifyWs 面板会话鉴权（AUTH_001/002）→ ③ 角色=admin（AUTH_008 附边界原因）→ ④ 参数校验：identitySource∈terminal_identity_source、specified_system_user 时 username 必填且形态合法（TERM_005）→ ⑤ 并发判定 CountByUser(userId)<5（TERM_001，data={limit, current}）→ ⑥ 指定身份：用户存在（TERM_002）+ shell 交互（TERM_003）→ ⑦ PtyGateway spawn（参数数组，R-08；失败 TERM_004，对称回收）→ ⑧ 建立审计 terminal_session_opened（security/web；detail={identitySource, runUser, sourceIp}；失败→Kill PTY→TERM_006，D-06）→ ⑨ 登记会话 + 101 升级成功。升级后服务端首帧：`ready`，data=`{sessionId, identity:{source, runUser}, limits:{maxConcurrent:5, idleTimeoutSeconds:1800, hardLimitSeconds:28800}}`（前端权威值）。
- **帧交互**：客户端 `input`（data=Base64 键入字节 → 写 PTY + 更新 lastActiveAt）/ `resize`（cols/rows → PTY 尺寸同步 + 更新 lastActiveAt）；服务端 `output`（PTY 输出 → Base64，缓冲背压 D-04）。校验：帧类型白名单 + 16KB 上限（I-06，违规即 protocol_error 断开）。
- **断开**：任一触发（4.1.2）→ 服务端发 `closed` 帧 data=`{reason, closedAt, durationSeconds}` → WS 关闭 → TerminalCleanupService 统一清理（Kill PTY 进程组 → 注销会话 → 释放配额 → closed 审计 terminal_session_closed，detail={reason, durationSeconds}；source 按 I-07 映射；审计失败日志兜底不阻断清理，D-06）。客户端主动关闭（close 帧）走同一清理路径（reason=self_close）。

**跨接口复用规则**：握手准入链统一走 TerminalHandshakeGuard（I-01 顺序与错误码映射固定）；全部断开路径收敛 TerminalCleanupService 单点（I-02/I-07 保证幂等与恰好一条 closed 审计）；审计统一 `AuditService.Record`（account-auth OHS）。

**幂等语义**：无重复消息处理面（帧流无业务幂等需求）；清理幂等（重复触发无害，I-02）；重连=新会话（无幂等风险）。

#### 4.1.5 领域事件与集成

无 MQ、无领域事件（单机单体，R-09）。「仓储契约」为零持久化替代——**进程内网关与注册表契约（非 SQL）**：

```
TerminalSessionRegistry {
  Create(s TerminalSession) error            // 同写并发配额（原子 CAS，I-05）
  Get(sessionId) (TerminalSession, bool)
  CountByUser(userId int) int                // TERM_001 判定
  ListByUser(userId int) []TerminalSession   // 联动断开全量寻址
  TouchActivity(sessionId)                   // input/resize 更新 lastActiveAt（I-03）
  Remove(sessionId)                          // 清理终点（幂等）
  Scan(tick)                                 // Lifecycle 扫描遍历（空闲/硬上限/复核/心跳）
}

PtyGateway {
  Spawn(identity RunIdentity) (PtyHandle, error)
    // 参数数组（R-08 禁 shell 拼接）；panel_run_user=以面板运行用户直启默认 shell；
    // specified_system_user=su 语义代启指定用户登录 shell（setuid/setgid/login env），不代输密码
  RunIdentity { Source terminal_identity_source; SystemUsername string }
  PtyHandle { Read(p []byte) (n, err); Write(p []byte) error; Resize(cols, rows uint16) error;
              Kill() error; Wait() error }   // Kill=进程组终止；Wait=收割防孤儿（I-02）
}

SysUserDirectory {
  ListLoginShellUsers() ([]SystemUser, error)  // /etc/passwd 语义直读（D-02）+ 非交互 shell 过滤
  Get(name string) (SystemUser, bool)          // TERM_002/003 判定共用
  SystemUser { Name string; UID uint32; Shell string }
}
```

关联域调用（出站）：`SessionAuthService.VerifyWs(rawToken)`（account-auth OHS，握手 + 30s 复核共用，只读不续期，R-18）；`AuditService.Record`（account-auth OHS，开合两段审计 D-06）。被调用契约：无（消费终模块，1.5；docker-mgmt 容器内终端为二期预期不预先建模）。

## 5. 模块清单汇总

### 5.1 对外服务清单（HTTP/WS 端点）

| 端点 | 消费方 | 契约要点 |
|------|--------|---------|
| GET /terminal/user/options | 前端终端页（admin，唯一消费方） | 指定身份候选（登录 shell 过滤，D-02） |
| WS /terminal/session/connect | 前端终端页（admin） | 三重准入（I-01）+ 帧协议（D-01）+ 生命周期管控（I-03/D-03/D-07）+ 统一清理（I-02） |
| 中间件 AuthRequired / RoleRequired / TerminalHandshakeGuard | （复用+本模块） | account-auth 提供 + 本模块 WS 守卫（3.1） |
| SessionAuthService.VerifyWs / AuditService.Record | （出站调用） | account-auth OHS（R-18/R-03，D-06/D-07） |

本模块无对外 OHS 服务契约——消费终模块（1.5）。

### 5.2 SQL 清单

**无**——本模块零持久化（I-04）：terminal_session 为进程内对象，无任何 SQLite 表读写（domain-model.md 存储映射说明「terminal→无表」）；会话开合审计落库由 account-auth 的 AuditRepository 承载（非本模块 SQL）。

### 5.3 DDL

**无**——零持久化（同 5.2；本模块不新增任何表，面板库 schema 无变更）。

## 6. 更新记录

### 6.1 变更记录

| 版本 | 日期 | 变更类型 | 变更摘要 | 目标节号 |
|------|------|---------|---------|---------|
| v1.0 | 2026-11-14 | initial | 首次生成：2 个端点（1 HTTP：GET /terminal/user/options；1 WebSocket：/terminal/session/connect）+ 帧协议 5 类型（ready/input/resize/output/closed，D-01）+ 3 个进程内网关/注册表契约（TerminalSessionRegistry/PtyGateway/SysUserDirectory，零持久化：无表/无 DDL/无 SQL）+ 7 错误码（TERM_001~007）+ 复用 account-auth 码族（AUTH_001/002/003/008）+ 4 枚举引用（无新增枚举）+ 2 审计动作常量 + 9 项设计决策（D-01 JSON+Base64 帧、D-02 实况直读、D-03 心跳 30s×2、D-04 256KB 环形缓冲、D-05 断开预告不做、D-06 审计两段口径、D-07 失效联动=复核 30s+协作项登记、D-08 MIXED、D-09 握手 Cookie+Origin）；关闭 prd-terminal.md 待对齐-1/-2/-3/-4/-5/-6；无旧版对比 | — |
| v1.1 | 2026-11-14 | field_added | Gate 1 WARN W1 收尾（05-reports/gate1-terminal-2026-11-12.md）：4.1.1 运行时字段清单补「panelSessionRef（运行时凭据）」行——对齐 domain-model.md:168 与 BA 附录B 五要素口径；纯文档盘点补列，端点/帧协议/契约零变更（此前 9 项 → 10 项，D-07 30s 复核的数据承载显式化） | 4.1.1 |
| v1.0 | 2026-11-14 | enum_value_added | terminal_end_reason 增补第 6 取值 protocol_error（协议违规断开：未知帧类型/超限帧，I-06 接口层防御路径）；枚举池 04-business-enum.md 同步行注明 D3 增补理由 | 3.4 / 4.1.2 |

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
