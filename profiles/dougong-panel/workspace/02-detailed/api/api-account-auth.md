# 斗拱面板 账户与安全基座（account-auth）API 设计

| 属性 | 值 |
|------|-----|
| 版本 | v1.2 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-19（v1.2 db-tools gating OHS 消费登记，零 HTTP/DDL 变更；表头版本订正——v1.1 Gate 1 W2/W3 修正行早已在 6.1 而表头滞后 v1.0） |
| 输入 | prd-account-auth.md v1.0（完备性检查通过）、domain-model.md v1.0、d0-clarification-account-auth.md |
| 接口规约 | 无激活技术栈（ACTIVE_STACKS 为空）——按通用 REST/JSON 契约设计；认证=HttpOnly Cookie 会话 + CSRF token（R-02/ADR-019）；鉴权=RBAC 两角色边界矩阵（ADR-018） |

## 1. 模块设计概述

### 1.1 业务背景及目标

账户与安全基座是斗拱面板 M1 里程碑的横切安全底座：解决「自托管运维者如何安全取得并持续管控面板访问权」的问题——初始化双入口取得控制权（ADR-017）、密码+TOTP 登录（R-01 锁定防暴力破解）、HttpOnly Cookie 会话 + CSRF 防劫持（R-02）、admin/readonly 两角色差异化授权（ADR-018）、全变更审计与 90 天保留治理（R-03/ADR-020）。服务对象：面板管理员（admin）、只读观察者（readonly）、未认证访客。

### 1.2 边界说明

| 项 | 说明 |
|----|------|
| 本模块做 | 初始化引导（Web 向导 + CLI）、登录（密码+TOTP）、登录锁定、会话签发/续期/吊销/CSRF、RBAC 边界、审计记录/查询/导出/90 天清理 |
| 本模块不做（接口边界） | ① 各业务模块审计埋点实现——只暴露进程内 `AuditService.Record` 服务契约；② 面板 HTTPS/监听/安全入口——归 panel-core；③ Web 终端 WS 端点与连接限流——归 terminal（本模块仅提供 `SessionAuthService` 供其握手调用）；④ 角色 CRUD——role 固定枚举 |
| 与存储的关系 | SQLite 单文件（ADR-011，modernc.org/sqlite 纯 Go），表 DDL 见 5.3 |

### 1.3 设计决策

| # | 决策 | 理由 |
|---|------|------|
| D-01 | 认证令牌：Cookie 存 256-bit 随机 token，库中仅存其 SHA-256 哈希 | 库文件泄露不等于会话劫持；HttpOnly + SameSite=Strict + Secure(HTTPS 时) |
| D-02 | TOTP 登录采用两步式：密码通过且启用 TOTP → 返回一次性 `login_ticket`（内存态，5 分钟有效）→ `/auth/session/totp-verify` 换会话 | 动态码阶段与密码阶段失败计数解耦，避免密码正确前的暴力猜码；单实例内存 map 足够（无集群） |
| D-03 | session 过期为**计算态**：不落 status 列，`active = revoked_at IS NULL AND now < expires_at AND now < idle_expires_at` | 惰性判定免后台扫描；滑动续期只更新 `last_active_at`/`idle_expires_at` |
| D-04 | 登录锁定状态内嵌 panel_user 表（fail_count/locked_until 两列），不建独立表 | 1:1 生命周期完全随用户；D2 存储映射已声明 |
| D-05 | 审计写入为进程内服务调用（单体单进程），不引入 MQ/事件总线 | R-09 零外呼 + 单机部署；审计与业务操作同事务语义（PRD N2：不允许"改了但没记录"） |
| D-06 | actor_name 冗余存审计表 | 用户删除后审计仍可读（数据溯源冗余策略） |
| D-07 | HTTP 层统一 200 + 业务 code；文件导出为 200 + CSV 流（Content-Disposition: attachment） | 面板内部契约，前端统一拦截器按 code 分流；无激活栈 profile，通用 REST/JSON |
| D-08 | 混合模式（MIXED）声明：全部对象实际为自管（无外部数据源，R-09），不设 manage_mode 字段，权限管控按角色区分 | 分册 Step 2 默认混合模式；本模块无托管侧 |
| D-09 | 禁止删除当前登录账户自身 | 防误操作自锁；唯一管理员保护另由不变量 I-03 保证 |
| D-10 | 会话有效期配置由本域自持，**载体=面板配置文件**（internal/config：session.absolute_timeout/idle_timeout 两键，重启生效 MVP），非表载体、不依赖 panel_config 管理页 | D2 边界决策；Gate 1 W2 修正（2026-11-12 用户拍板）：为两个可配值建表不合 ADR-009 单二进制轻量哲学；与 panel-core 对齐后可迁移统一配置管理 |

### 1.4 领域对象总览

| 聚合根 | 域内对象/值对象 | 子流程 | 关联领域对象 |
|--------|---------------|--------|-------------|
| panel_user（主数据类） | TOTP 凭证（1:1 实体）、登录锁定状态（内嵌字段组） | — | 无（本域为被依赖方） |
| session（主数据类） | 会话有效期策略（域级配置） | — | 无 |
| audit_log（台账报表类） | 变更内容快照（值对象）、审计保留策略（值对象） | 审计过期清理任务（每日） | 无 |
| 初始化状态（单行资源） | — | — | 无 |

### 1.5 上下文映射

| 上下文 | 关系 | 机制 |
|--------|------|------|
| 全部业务域（site-mgmt/file-manager/…） | 被依赖 | OHS：`AuditService.Record`（审计四要素，R-03）；遵奉者：HTTP 域走 Cookie+CSRF 协议（R-02） |
| terminal（Web 终端） | 被依赖 | OHS：`SessionAuthService.VerifyWs`（R-18）；连接限流归 terminal |
| **db-tools（数据库工具集成，v1.2 登记）** | **被依赖** | **OHS：`SessionAuthService.VerifyWs`（gating 只读会话校验，同 I-11 判定只读不续期）——工具路径 auth_request 内部端点由 db-tools 自持（api-db-tools.md D-11 /dbtool/gate/verify），本模块不新增 HTTP 接口；readonly/无会话经 gating 拒绝（⚠️A10 工具访问 admin 专属）** |
| panel-core | 消费其 HTTP 骨架 | 路由挂载/入口面归 panel-core（R-10）；会话有效期配置暂自持（D-10） |

## 2. 跨聚合根设计

### 2.1 聚合根关系图

```
初始化状态 (init_state, 单行)
    │ 1:0..1 初始化创建
    ▼
panel_user 1 ──────── * session        （用户删除/重置 → 级联吊销会话）
    │ 1:1                    │ 1:0..*
    ├─ TOTP 凭证             └─ 产生 * audit_log（会话类安全事件）
    └─ 登录锁定状态（内嵌）
panel_user 1 ─── * audit_log（作为 actor）  （冗余 actor_name，D-06）
audit_log * ─── 引用任意业务对象（object_type/object_id 弱引用，不做 FK）
```

### 2.2 跨聚合根不变量

| 编号 | 不变量 | 违反处理 |
|------|--------|---------|
| I-01 | 系统初始化前禁止一切业务接口与会话签发（ADR-017） | InitGuard 拦截，重定向/返回 AUTH_009 |
| I-02 | 初始化状态全局唯一且单向（单行 id=1，uninitialized→initialized 不可逆） | 双入口并发经单行 INSERT 仲裁，后到者 AUTH_009 |
| I-03 | 任意时刻至少存在 1 个 admin（删除用户前置校验） | AUTH_010 拒绝 |
| I-04 | session.user_id 必须指向存在且 status=active 的 panel_user；用户删除/锁定期间新登录被拒 | 删除时级联吊销全部会话；锁定时登录拒绝（AUTH_004） |
| I-05 | 每条审计记录必须原子落库于其描述的业务变更同一事务 | 变更失败整体回滚（D-05） |
| I-06 | 用户名全局唯一（大小写不敏感比较） | AUTH_011 拒绝 |

### 2.3 跨聚合根编排

| 编排 | 触发 | 步骤 |
|------|------|------|
| 登录编排 | `/auth/session/login` | 校验 init_state → 校验 panel_user（存在/active/未锁定）→ 校验密码 → [启用 TOTP: 签发 login_ticket] → 签发 session → 审计 |
| 级联吊销编排 | 用户删除/密码重置/自助改密 | 变更 panel_user → 按规则批量 revoke session（写 end_reason）→ 审计 |
| 初始化编排 | 向导/CLI | init_state 单行仲裁 → 创建首个 admin panel_user → 标记 initialized → 审计（同一事务） |
| 审计清理编排 | 每日调度 | 按保留策略删 audit_log 过期批 → 写清理审计（独立事务） |

## 3. 应用/基础设施层设计

### 3.1 接口规范

| 项 | 规范 |
|----|------|
| 路径格式 | `/auth/{resource}/{action}` 三段式；资源单数（user/session/audit-log/init） |
| HTTP 方法 | 读=GET，写=POST（面板内部契约，D-07） |
| 响应格式 | `{"code": 0, "message": "ok", "data": {...}}`；code=0 成功，非 0 见 3.4 错误码；分页 data=`{items, total}`（公共约定） |
| 时间格式 | ISO-8601 UTC（公共约定） |
| 认证 | HttpOnly Cookie `dg_session`（值=256-bit token；库存 SHA-256 哈希，D-01）；SameSite=Strict；Secure（HTTPS 时）；Max-Age=绝对过期配置 |
| CSRF | 变更类（POST）须带 `X-CSRF-Token` 头 = 会话绑定 csrf_token；服务端同做 Origin/Referer 同源校验（R-02）；GET 与无会话接口豁免 |
| 中间件链 | InitGuard（I-01）→ AuthRequired → CsrfGuard（POST）→ RoleRequired（按接口） |
| 用户类型标注 | 每接口标注：公开 / admin / admin+readonly（全可看，ADR-018） |

### 3.2 依赖与约束

| 依赖 | 类型 | 约束 |
|------|------|------|
| panel-core HTTP 骨架/gin | 框架 | 路由组挂载 `/auth`；中间件注册顺序固定（3.1） |
| SQLite (modernc.org/sqlite) | 存储 | 单连接写串行；审计写入与业务变更同事务（I-05） |
| terminal / db-tools（gating，v1.2） | 下游消费方 | 仅经 `SessionAuthService.VerifyWs`，不得直查 session 表 |
| 密码哈希 | 算法 | 抗暴力慢哈希（bcrypt 类，cost ≥10）；常量时间比较 |
| TOTP | 算法 | RFC 6238，30 秒窗口 ±1，6 位数字码 |

### 3.3 权限模型

角色：admin / readonly（panel_role 固定枚举）；访客=未认证操作者。

**接口权限矩阵**：

| 接口 | 公开 | admin | readonly | CSRF |
|------|------|-------|----------|------|
| GET /auth/init/check | ✓（InitGuard 例外） | — | — | — |
| POST /auth/init/wizard | ✓（仅未初始化） | — | — | 豁免（无会话） |
| POST /auth/session/login | ✓（仅已初始化） | — | — | 豁免（无会话） |
| POST /auth/session/totp-verify | ✓（凭 login_ticket） | — | — | 豁免 |
| GET /auth/session/current | 会话 | ✓ | ✓ | — |
| POST /auth/session/logout | 会话 | ✓ | ✓ | ✓ |
| POST /auth/user/password-change | 会话 | ✓ | ✓ | ✓ |
| POST /auth/user/totp-setup / totp-enable / totp-disable | 会话 | ✓ | ✓ | ✓ |
| GET /auth/user/list | 会话 | ✓ | ✓（全可看，脱敏） | — |
| POST /auth/user/create / delete / reset-password | 会话 | ✓ | ✗（AUTH_008） | ✓ |
| GET /auth/session/list | 会话 | ✓ | ✓（全可看） | — |
| POST /auth/session/revoke | 会话 | ✓ | ✗（AUTH_008） | ✓ |
| GET /auth/audit-log/list | 会话 | ✓ | ✓（ADR-018 含审计） | — |
| GET /auth/audit-log/export | 会话 | ✓ | ✗（AUTH_008，解释性决策） | —（GET 只读） |

数据权限规则：无行级隔离——readonly 的"全可看"含全部数据行；变更能力即角色边界本身（ADR-018）。敏感字段脱敏：用户列表不下发 password_hash / TOTP secret；会话清单不下发 token 哈希与 csrf_token。

### 3.4 公共规范

**错误码**：

| code | 标识 | HTTP 语义 | 说明 |
|------|------|----------|------|
| 0 | OK | 成功 | — |
| AUTH_001 | UNAUTHENTICATED | 未登录 | 无/无效会话 |
| AUTH_002 | SESSION_EXPIRED | 会话过期 | 绝对或空闲到期，前端转登录 |
| AUTH_003 | CSRF_FAILED | 请求被拒绝 | token 缺失/不匹配或非同源 |
| AUTH_004 | ACCOUNT_LOCKED | 登录被拒 | data.remainingSeconds |
| AUTH_005 | BAD_CREDENTIALS | 登录失败 | 用户名或密码错误（不区分提示，防枚举） |
| AUTH_006 | TOTP_INVALID | 二次验证失败 | 动态码错误，计一次失败 |
| AUTH_007 | TOTP_REQUIRED | 需要二次验证 | data.loginTicket（5 分钟一次性） |
| AUTH_008 | FORBIDDEN | 权限不足 | readonly 越界/非 admin，附边界原因（ADR-018） |
| AUTH_009 | ALREADY_INITIALIZED | 拒绝 | 重复初始化（I-02） |
| AUTH_010 | LAST_ADMIN | 拒绝 | 唯一管理员保护（I-03） |
| AUTH_011 | USERNAME_TAKEN | 参数冲突 | 用户名已存在（I-06） |
| AUTH_012 | VALIDATION_FAILED | 参数错误 | data.fields[] 逐字段错误（R-14） |
| AUTH_013 | TICKET_INVALID | 二次验证失败 | ticket 无效/过期 |
| AUTH_014 | SELF_DELETE | 拒绝 | 不能删除当前登录账户（D-09） |

**枚举**（均在 workspace/01-solution/04-business-enum.md 定义，本次定稿）：

| 枚举标识 | 取值 | 用途 |
|---------|------|------|
| panel_role | admin \| readonly | 用户角色 |
| panel_user_status | active \| locked | 用户状态（R-01） |
| session_status | active \| expired \| revoked | 会话展示态（派生，D-03） |
| session_end_reason | self_logout \| forced_logout \| cascade | 吊销原因（revoked_at 非空时必填） |
| audit_action | change \| security | 审计类别 |
| audit_result | success \| failure | 审计结果 |
| audit_source | web \| cli \| system | 审计来源 |
| init_status | uninitialized \| initialized | 初始化状态 |

**业务常量**（审计细粒度动作码，非枚举池条目）：`login_success / login_failed / account_locked / account_unlocked / logout / force_logout / user_created / user_deleted / password_reset / password_changed / totp_enabled / totp_disabled / init_completed / audit_exported / audit_purged`（action 列存此码，category 存 audit_action 枚举）。

**配置项**（会话有效期策略，D-10 自持；载体=配置文件非表，Gate 1 W2 修正）：

| 配置 | 默认 | 可配置 | 说明 |
|------|------|--------|------|
| session.absolute_timeout | 12h | ✓（ADR-019） | 配置文件键（internal/config），重启生效（MVP）；自签发起算 |
| session.idle_timeout | 2h | ✓（ADR-019） | 配置文件键（internal/config），重启生效（MVP）；活跃滑动顺延 |
| audit.retention_days | 90 | ✗（ADR-020 固定） | 编译期常量（非配置键、不入库），每日清理任务读取 |
| password.min_length | 8 | ✗（A10 从宽固定） | 编译期常量（非配置键、不入库），服务端校验 |

## 4. 聚合根详细设计

### 4.1 panel_user（主数据类）

#### 4.1.1 领域模型

**表结构组合**：panel_user（主表，内嵌登录锁定字段组）+ panel_user_totp（1:1 子表，D-04/存储映射）。

**字段清单**：

panel_user：

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | 聚合根标识 |
| username | TEXT | UNIQUE NOT NULL | 登录名；唯一性大小写不敏感（I-06） |
| password_hash | TEXT | NOT NULL | 慢哈希（bcrypt 类 cost≥10） |
| role | TEXT | NOT NULL CHECK(in admin,readonly) | panel_role |
| status | TEXT | NOT NULL DEFAULT 'active' CHECK(in active,locked) | panel_user_status |
| fail_count | INTEGER | NOT NULL DEFAULT 0 | 连续失败计数（R-01，内嵌锁定状态） |
| locked_until | DATETIME | NULL | 锁定截止；NULL=未锁定 |
| created_at / updated_at | DATETIME | NOT NULL | ISO-8601 UTC |

panel_user_totp：

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| user_id | INTEGER | PK, FK→panel_user(id) ON DELETE CASCADE | 1:1 |
| secret | TEXT | NOT NULL | Base32 密钥；enabled=0 时为待确认态（可覆盖重生成） |
| enabled | INTEGER | NOT NULL DEFAULT 0 | 0/1（公共布尔约定） |
| confirmed_at | DATETIME | NULL | 首次验证通过时间 |

**不变量**（聚合内）：

| 编号 | 不变量 |
|------|--------|
| I-07 | role ∈ panel_role；status ∈ panel_user_status |
| I-08 | locked=active ⇔ now < locked_until；锁定解除时 fail_count 归零 |
| I-09 | 密码明文永不存储/记录/下发；TOTP secret 不经任何接口回传（setup 仅返回一次） |
| I-10 | 密码长度 ≥8（A10）；用户名非空、≤32 字符、`[a-zA-Z0-9_.-]` |

**领域服务**：PasswordHasher（哈希/常量时间比较）、TotpVerifier（RFC 6238 窗口 ±1）、LockoutPolicy（5 次/15 分钟判定）。

#### 4.1.2 状态模型

panel_user：`active ⇄ locked`（R-01）——locked 由 `locked_until` 表达；锁定→active 期满自动（读时惰性判定+落库归零）。无审批/驳回环节；无独立状态历史表（锁定事件经 audit_log 追溯）。

#### 4.1.3 子流程设计

无（TOTP 绑定为两接口协作的即时流程，非独立聚合）。

#### 4.1.4 接口设计

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /auth/user/list | GET | admin+readonly | 用户列表（脱敏） |
| /auth/user/create | POST | admin | 创建并指派角色 |
| /auth/user/delete | POST | admin | 删除（I-03/D-09 守卫） |
| /auth/user/reset-password | POST | admin | 重置并吊销该用户全部会话 |
| /auth/user/password-change | POST | admin+readonly | 自助改密 |
| /auth/user/totp-setup | POST | admin+readonly | 生成待确认密钥+otpauth URI（一次性返回） |
| /auth/user/totp-enable | POST | admin+readonly | 验证动态码激活 |
| /auth/user/totp-disable | POST | admin+readonly | 密码确认后解绑 |

**接口详情**（请求/响应/校验）：

- **POST /auth/user/create**：req `{username, password, role}`；校验：I-10 全部字段（R-14）、role∈panel_role、I-06 唯一性；响应 data=`{id, username, role, createdAt}`；审计：user_created（change）。
- **POST /auth/user/delete**：req `{userId}`；校验：目标存在、≠当前登录者（AUTH_014）、若目标为 admin 且系统仅剩 1 个 admin → AUTH_010（I-03）；成功级联吊销其全部 session（end_reason=cascade）；审计：user_deleted（change）。
- **POST /auth/user/reset-password**：req `{userId, newPassword}`；校验：长度 ≥8（A10）；成功 → 吊销该用户全部会话（cascade）；审计：password_reset（security）。
- **POST /auth/user/password-change**：req `{oldPassword, newPassword}`；校验：旧密码常量时间比对、新密码 ≥8；成功 → 吊销本人**其他**会话（cascade，保留当前）；审计：password_changed（security）。
- **POST /auth/user/totp-setup**：无 req；生成新 secret（覆盖未激活旧值）→ data=`{otpauthUri, secret}`（仅此一次下发，I-09）；审计无（未生效）。
- **POST /auth/user/totp-enable**：req `{code}`；校验 TotpVerifier；成功 enabled=1、confirmed_at=now；审计：totp_enabled（security）。
- **POST /auth/user/totp-disable**：req `{password}`；校验密码比对；成功删除行；审计：totp_disabled（security）。
- **GET /auth/user/list**：无分页（用户量极小）；data=`items[{id, username, role, status, totpEnabled, lockedUntil, createdAt}]`。

**跨接口复用规则**：会话级联吊销统一走 `SessionReconciler.RevokeByUser(userId, reason)`（删除=全部、重置=全部、改密=除当前外）；审计写入统一走 `AuditService.Record`（4.4 audit_log 服务规格）。

#### 4.1.5 领域事件与集成

无 MQ（D-05）。仓储契约：`UserRepository { GetByID, GetByUsername, List, Create, Update, Delete }`；`TotpRepository { GetByUser, Upsert, Delete }`。关联域调用：无出站。

### 4.2 session（主数据类）

#### 4.2.1 领域模型

**表结构**：panel_session 单表（token 存哈希，D-01）。

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| token_hash | TEXT | PK | SHA-256(session token)；Cookie 值为原 token |
| user_id | INTEGER | NOT NULL FK→panel_user(id) ON DELETE CASCADE | 归属 |
| csrf_token | TEXT | NOT NULL | 会话绑定 CSRF 令牌 |
| created_at | DATETIME | NOT NULL | — |
| last_active_at | DATETIME | NOT NULL | 滑动续期锚点 |
| expires_at | DATETIME | NOT NULL | 绝对过期 = created_at + absolute_timeout |
| idle_expires_at | DATETIME | NOT NULL | = last_active_at + idle_timeout（活跃时刷新） |
| revoked_at | DATETIME | NULL | 非空=已吊销 |
| end_reason | TEXT | NULL CHECK(in self_logout,forced_logout,cascade) | revoked_at 非空时必填 |
| source_ip / user_agent | TEXT | NULL | 来源（会话清单展示） |

**不变量**：I-11 会话有效 ⇔ revoked_at IS NULL ∧ now < expires_at ∧ now < idle_expires_at（计算态，D-03）；I-12 csrf_token 与会话一一绑定、不重复使用于其他会话；I-13 登出/吊销立即生效（下次请求即 401 语义 AUTH_001/002）。

**领域服务**：TokenIssuer（256-bit 随机 + SHA-256）、SessionValidator（I-11 判定 + 滑动续期写）、LoginTicketStore（内存 map，5 分钟 TTL 一次性，D-02）。

#### 4.2.2 状态模型

`active（计算态）→ expired（expires_at 或 idle_expires_at 到）→ revoked（revoked_at 落库）`；expired/revoked 均终态；过期不落库变更（惰性判定），吊销落库并记 end_reason。历史追溯：登录/登出/吊销经 audit_log。

#### 4.2.3 子流程设计

无。

#### 4.2.4 接口设计

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /auth/session/login | POST | 公开（已初始化） | 密码登录（两步式入口） |
| /auth/session/totp-verify | POST | 公开（凭 ticket） | TOTP 二阶段换会话 |
| /auth/session/current | GET | 会话 | 当前登录态 |
| /auth/session/list | GET | admin+readonly | 在线会话清单 |
| /auth/session/revoke | POST | admin | 强制下线 |
| /auth/session/logout | POST | 会话 | 登出 |

**接口详情**：

- **POST /auth/session/login**：req `{username, password, code?}`——code 字段废弃（两步式 D-02，不接受同请求动态码）；校验顺序：I-01 已初始化 → I-04 用户存在且 active → I-08 未锁定（锁定返回 AUTH_004+remainingSeconds，不计失败）→ 密码比对（错误：fail_count+1，达 5 次落 locked_until=now+15min 并审计 account_locked，返回 AUTH_005）→ 启用 TOTP：签发 ticket 返回 AUTH_007 `{loginTicket}`；未启用：直接签发会话。成功响应：Set-Cookie dg_session + data=`{csrfToken, user:{id,username,role}}`；审计：login_success / login_failed（security，source=web）。
- **POST /auth/session/totp-verify**：req `{loginTicket, code}`；校验：ticket 存在未用过未过期（AUTH_013）→ 动态码验证（错误：AUTH_006，计一次失败入 R-01 计数）→ 吊销 ticket → 签发会话（响应同 login 成功）；审计：login_success（含 totp 标记）。
- **GET /auth/session/current**：data=`{authenticated:true, user:{id,username,role}, csrfToken}`；未登录返回 AUTH_001（非错误流，前端首屏探测）。
- **GET /auth/session/list**：data=`items[{id(会话标识=token_hash 前 8 位摘要), username, createdAt, lastActiveAt, sourceIp, current:boolean}]`，按 lastActiveAt 倒序；仅列 I-11 有效会话。
- **POST /auth/session/revoke**：req `{sessionId}`；校验目标会话有效；吊销落库 end_reason=forced_logout；不能吊销当前会话自身（登出走 logout）；审计：force_logout（change）。
- **POST /auth/session/logout**：无 req；吊销当前会话 end_reason=self_logout + 清 Cookie（Max-Age=0）；审计：logout（security）。

**会话续期**：无显式接口——SessionValidator 中间件对每次认证请求写 last_active_at/idle_expires_at（滑动，ADR-019）。

#### 4.2.5 领域事件与集成

仓储契约：`SessionRepository { Create, GetByTokenHash, Touch(续期), Revoke, ListActive, DeleteByUser, DeleteExpired }`。对外集成：`SessionAuthService.VerifyWs(rawToken) → (userId, role, err)`（terminal WS 握手复用，R-18；**db-tools gating 复用，v1.2 登记**；验证同一 I-11 逻辑，只读不续期）。

### 4.3 audit_log（台账报表类）

#### 4.3.1 领域模型

**表结构**：audit_log 单表（只追加，无 UPDATE/DELETE 语义——删除仅限清理任务）。

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | — |
| actor_id | INTEGER | NULL | panel_user.id；CLI/系统动作为 NULL |
| actor_name | TEXT | NOT NULL | 冗余快照（D-06，删除用户后可读） |
| category | TEXT | NOT NULL CHECK(in change,security) | audit_action |
| action | TEXT | NOT NULL | 细粒度动作码（3.4 常量表） |
| object_type | TEXT | NOT NULL | 对象类型（user/session/audit_log/init/各业务对象） |
| object_id | TEXT | NULL | 弱引用，不做 FK |
| detail | TEXT | NULL | JSON 变更快照（变更内容四要素之一，R-03） |
| result | TEXT | NOT NULL CHECK(in success,failure) | audit_result |
| source | TEXT | NOT NULL CHECK(in web,cli,system) | audit_source |
| created_at | DATETIME | NOT NULL | 索引列 |

索引：idx_audit_created(created_at DESC)、idx_audit_actor(actor_id)、idx_audit_action(action)。

**不变量**：I-14 只追加——应用层不提供 UPDATE/DELETE（除清理任务按保留策略物理删除）；I-15 四要素必填（操作者/时间/对象/结果，R-03）；I-16 写入与业务变更同事务（I-05）。

**值对象**：变更内容快照（detail JSON：`{field, before, after}[]` 或描述性载荷）；审计保留策略（90 天/每日，配置常量）。

#### 4.3.2 状态模型

无状态流转（append-only；台账报表类）。

#### 4.3.3 子流程设计——审计过期清理任务

| 项 | 契约 |
|----|------|
| 触发 | 每日后台调度（进程内 cron） |
| 输入 | 当前时间 + audit.retention_days（90） |
| 处理 | DELETE FROM audit_log WHERE created_at < now-90d（分批 LIMIT 循环，防长事务） |
| 输出 | 写入 audit_log 一条 audit_purged 记录（{deletedCount, cutoff}，source=system） |
| 失败处理 | 批次失败保留已删批次、记录失败审计（result=failure），次日重试 |

#### 4.3.4 接口设计

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /auth/audit-log/list | GET | admin+readonly | 条件分页查询 |
| /auth/audit-log/export | GET | admin | CSV 导出（当前筛选全集） |

**接口详情**：

- **GET /auth/audit-log/list**：query `{from?, to?, actorName?, category?(audit_action), action?, objectType?, objectId?, page, pageSize}`（公共分页约定；objectId 等值匹配，Gate 1 W3 修正 2026-11-12）；data=`{items:[{id, actorName, category, action, objectType, objectId, detail, result, source, createdAt}], total}`；按 created_at 倒序。
- **GET /auth/audit-log/export**：同上筛选参数；响应 CSV 流（UTF-8 BOM + Content-Disposition），列 = id,createdAt,actorName,category,action,objectType,objectId,result,source,detail；行数上限 100,000（防失控，超限拒绝 AUTH_012 提示缩小范围）；写审计 audit_exported（security）。

**写入侧（无 HTTP 接口）——OHS 服务方法规格**：

```
AuditService.Record(ctx, AuditEntry) error
AuditEntry { ActorID *int64, ActorName string, Category audit_action,
             Action string, ObjectType string, ObjectID string,
             Detail any(→JSON), Result audit_result, Source audit_source }
约束：与调用方业务变更同事务（I-05/I-16）；失败即整体失败（PRD N2）
```

#### 4.3.5 领域事件与集成

仓储契约：`AuditRepository { Append, Search(cond, page), DeleteBefore(cutoff) batch }`。关联域调用：无出站；被调用契约 = AuditService.Record（全模块，1.5）。

### 4.4 初始化状态（单行资源）

#### 4.4.1 领域模型

**表结构**：init_state 单行（id=1 CHECK）。

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK CHECK(id=1) | 单行保证（I-02） |
| initialized | INTEGER | NOT NULL DEFAULT 0 | 0/1 |
| method | TEXT | NULL CHECK(in web,cli) | 初始化方式 |
| initialized_at | DATETIME | NULL | — |

**不变量**：I-02 单行单向（0→1 不可逆）；初始化事务 = INSERT OR IGNORE id=1 抢占（affected=0 → 后到者拒绝 AUTH_009）+ 创建 admin + 审计 init_completed，同一事务。

#### 4.4.2 状态模型

`uninitialized → initialized`（单向，无回退；重置=删除数据库文件重建，属安装层语义，非本模块接口）。

#### 4.4.3 子流程设计

无。

#### 4.4.4 接口设计

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /auth/init/check | GET | 公开 | 前端路由判断依据 |
| /auth/init/wizard | POST | 公开（仅未初始化） | 创建首个 admin |

**接口详情**：

- **GET /auth/init/check**：data=`{initialized:boolean, allowWizard:boolean}`；未初始化时全站中间件（InitGuard）依据此态拦截。
- **POST /auth/init/wizard**：req `{username, password, totpSecret?, totpCode?}`（TOTP 可选一步完成：前端先本地或经 setup 类流程取得 secret——简化：向导含可选 TOTP 时，req 直接带 `totpSecret+code`，服务端验证后一并落库）；校验：I-10/A10、I-02 仲裁；成功：Set-Cookie（直接登录）+ data=`{csrfToken, user}`（等同登录成功响应）；已初始化返回 AUTH_009；审计：init_completed（security，source=web）。

**CLI 入口（非 HTTP）——服务方法规格**：

```
AuthService.InitViaCLI(ctx, username, password, totpOpt) error
约束：与向导共用同一 init_state 事务仲裁（I-02）；输出人类可读结果到 stdout；
审计 source=cli；已初始化时打印拒绝信息并退出码非 0（AUTH_009 语义）
```

#### 4.4.5 领域事件与集成

仓储契约：`InitStateRepository { Get, TryClaim(tx) bool }`。

## 5. 模块清单汇总

### 5.1 对外服务清单（进程内 OHS/内部服务）

| 服务方法 | 消费方 | 契约要点 |
|---------|--------|---------|
| AuditService.Record(entry) | 全部业务模块 | R-03 四要素；同事务；见 4.3.4 |
| SessionAuthService.VerifyWs(rawToken) | terminal 等 WS 域；**db-tools gating（v1.2 登记）** | 返回 (userId, role)；只读验证（R-18）；**db-tools 工具路径 auth_request 内部端点复用同判定（只读不续期，避免逐请求滑动续期写放大，api-db-tools.md D-11）；本模块不新增 HTTP 接口** |
| AuthService.InitViaCLI(...) | CLI 子命令（cmd/dougong） | 与向导同仲裁；见 4.4.4 |
| 中间件 InitGuard / AuthRequired / CsrfGuard / RoleRequired | api 路由层 | 顺序见 3.1 |

### 5.2 SQL 清单

| # | SQL | 场景 |
|---|-----|------|
| 1 | INSERT INTO init_state(id,initialized,...) VALUES(1,0,...) OR IGNORE | 启动自检（I-02） |
| 2 | UPDATE init_state SET initialized=1,... WHERE id=1 AND initialized=0 | 初始化仲裁抢占 |
| 3 | INSERT INTO panel_user(...) / SELECT by username / by id / LIST | 用户 CRUD（I-06 唯一索引） |
| 4 | UPDATE panel_user SET fail_count=?, locked_until=? WHERE id=? | 锁定计数（R-01） |
| 5 | INSERT/UPSERT/DELETE panel_user_totp | TOTP 绑定生命周期 |
| 6 | INSERT INTO panel_session(...) / GET by token_hash / UPDATE last_active_at,idle_expires_at | 会话签发/验证/续期 |
| 7 | UPDATE panel_session SET revoked_at=?, end_reason=? WHERE token_hash=? | 登出/强制下线 |
| 8 | UPDATE/DELETE panel_session WHERE user_id=? | 级联吊销（I-04） |
| 9 | SELECT panel_session WHERE 有效条件 ORDER BY last_active_at DESC | 在线清单（I-11） |
| 10 | INSERT INTO audit_log(...) / SELECT 带条件分页 / DELETE created_at < ? LIMIT n | 审计追加/查询/清理 |

### 5.3 DDL

```sql
CREATE TABLE IF NOT EXISTS panel_user (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  username      TEXT    NOT NULL UNIQUE COLLATE NOCASE,
  password_hash TEXT    NOT NULL,
  role          TEXT    NOT NULL CHECK (role IN ('admin','readonly')),
  status        TEXT    NOT NULL DEFAULT 'active' CHECK (status IN ('active','locked')),
  fail_count    INTEGER NOT NULL DEFAULT 0,
  locked_until  DATETIME,
  created_at    DATETIME NOT NULL,
  updated_at    DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS panel_user_totp (
  user_id      INTEGER PRIMARY KEY REFERENCES panel_user(id) ON DELETE CASCADE,
  secret       TEXT    NOT NULL,
  enabled      INTEGER NOT NULL DEFAULT 0,
  confirmed_at DATETIME
);

CREATE TABLE IF NOT EXISTS panel_session (
  token_hash      TEXT    PRIMARY KEY,
  user_id         INTEGER NOT NULL REFERENCES panel_user(id) ON DELETE CASCADE,
  csrf_token      TEXT    NOT NULL,
  created_at      DATETIME NOT NULL,
  last_active_at  DATETIME NOT NULL,
  expires_at      DATETIME NOT NULL,
  idle_expires_at DATETIME NOT NULL,
  revoked_at      DATETIME,
  end_reason      TEXT CHECK (end_reason IN ('self_logout','forced_logout','cascade')),
  source_ip       TEXT,
  user_agent      TEXT
);
CREATE INDEX IF NOT EXISTS idx_session_user ON panel_session(user_id);
CREATE INDEX IF NOT EXISTS idx_session_active ON panel_session(idle_expires_at);

CREATE TABLE IF NOT EXISTS audit_log (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  actor_id    INTEGER,
  actor_name  TEXT    NOT NULL,
  category    TEXT    NOT NULL CHECK (category IN ('change','security')),
  action      TEXT    NOT NULL,
  object_type TEXT    NOT NULL,
  object_id   TEXT,
  detail      TEXT,
  result      TEXT    NOT NULL CHECK (result IN ('success','failure')),
  source      TEXT    NOT NULL CHECK (source IN ('web','cli','system')),
  created_at  DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_log(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_actor ON audit_log(actor_id);
CREATE INDEX IF NOT EXISTS idx_audit_action ON audit_log(action);

CREATE TABLE IF NOT EXISTS init_state (
  id             INTEGER PRIMARY KEY CHECK (id = 1),
  initialized    INTEGER NOT NULL DEFAULT 0,
  method         TEXT CHECK (method IN ('web','cli')),
  initialized_at DATETIME
);
```

## 6. 更新记录

### 6.1 变更记录

| 版本 | 日期 | 变更类型 | 变更摘要 | 目标节号 |
|------|------|---------|---------|---------|
| v1.0 | 2026-11-12 | initial | 首次生成：18 个 HTTP 接口（/auth/{init,session,user,audit-log}/*）+ 4 个进程内 OHS 服务契约（AuditService.Record / SessionAuthService.VerifyWs / AuthService.InitViaCLI / 中间件链）+ 5 表 DDL + 8 枚举定稿 + 15 错误码（AUTH_001~014 + OK）；无旧版对比 | — |
| v1.1 | 2026-11-12 | rule_changed | Gate 1 W2 修正：D-10 会话有效期配置载体由"策略表单行配置"改为面板配置文件（internal/config，重启生效 MVP），5 表 DDL 不变；两个固定值（审计保留 90 天/密码最小 8 位）明确为编译期常量 | 1.3 / 3.4 |
| v1.1 | 2026-11-12 | interface_changed | Gate 1 W3 修正：/auth/audit-log/list 与 /auth/audit-log/export 的 query 补 objectId?（等值匹配），对齐 PRD T4"对象标识"筛选维度；接口数不变 18 | 4 |
| v1.2 | 2026-11-19 | rule_added（OHS 消费登记） | db-tools 集成联动登记（api-db-tools.md v1.0 D-11 消费方，同轮提供方登记沿 v2.2 先例）：SessionAuthService.VerifyWs OHS 消费方 +db-tools gating（工具路径 auth_request 内部端点 /dbtool/gate/verify 由 db-tools 自持，复用同 I-11 只读判定不续期——避免工具静态资源逐请求滑动续期写放大）；**本模块零新增 HTTP 接口/零 DDL 变更/零错误码变更**（gating 端点归 db-tools，会话校验能力既有）；1.5/3.2/4.2.5/5.1 消费登记同步；表头版本订正（v1.1 W2/W3 行早在 6.1 而表头滞后 v1.0，本轮升 v1.2 一并闭环） | 1.5 / 3.2 / 4.2.5 / 5.1 |

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
