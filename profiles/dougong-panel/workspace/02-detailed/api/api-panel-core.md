# 斗拱面板 面板基座（panel-core）API 设计

| 属性 | 值 |
|------|-----|
| 版本 | v2.0 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-16 |
| 输入 | prd-panel-core.md v1.1（完备性增量复核通过，ADR-028/029）、domain-model.md v1.16、d0-clarification-panel-core.md、D1 三文档（BA/AA/FL v1.1） |
| 接口规约 | 无激活技术栈（ACTIVE_STACKS 为空）——按通用 REST/JSON 契约设计；认证/鉴权/CSRF 复用 account-auth 中间件契约（R-02/ADR-018，api-account-auth.md 3.1）；全局入口前置=安全入口路径网关（R-10） |

## 1. 模块设计概述

### 1.1 业务背景及目标

面板基座是斗拱面板的全模块宿主基座，API 层解决四个问题：① **面板自身配置的唯一读写入口**——panel_config 单例（监听/安全入口/域名绑定/HTTPS/更新检查/语言）的查看与变更，含即时生效与需重启生效的分类提示（R-10、D1 AA 附录C、ADR-028）；② **全模块统一外壳的数据供给**——按角色过滤的菜单树下发（ADR-018）与版本信息可观测；③ **克制的更新检查**——默认关闭、显式开启后单次手动触发的外呼比对（R-09 零外呼承诺）；④ **面板自身证书 ACME 签发与续期**——HTTP-01 挑战经服务器已有 nginx，复用 site-mgmt AcmeGateway 与 nginx 安全写回通道，同步终态（ADR-029）。服务对象：面板管理员（admin）、只读观察者（readonly）；全部 `/panel/*` 接口无公开端点（未认证访客仅触达 account-auth 认证接口与本模块入口网关）。

### 1.2 边界说明

| 项 | 说明 |
|----|------|
| 本模块做 | panel_config 配置读写与生效分类（含绑定域名 bind_domain）、安全入口路径网关与域名绑定 Host 强校验网关（全局中间件）、面板自身 HTTPS 证书管理（自签/导入/ACME 签发与续期经 site-mgmt 通道）、菜单树按角色下发、版本信息、更新检查开关与手动触发、构建期菜单挂载登记约定 |
| 本模块不做（接口边界） | ① 登录/会话/RBAC/审计接口——归 account-auth（本模块仅消费其中间件与 AuditService.Record）；② 站点 SSL 证书（站点侧 ACME/续期）——归 site-mgmt，本模块仅管面板自身证书（其 ACME 签发/挑战写回复用 site-mgmt 通道，ADR-029，见 1.5/3.2）；③ 面板服务重启/自升级——重启经 systemctl（FR-02，OS 层），无面板自重启/自升级接口（idea.md §4.2）；④ 监控/业务页面数据接口——归各业务模块（本模块仅供给菜单树骨架）；⑤ DNS-01 通配符证书与独立 80 端口监听——二期/非目标（ADR-029） |
| 与存储的关系 | SQLite 单文件（ADR-011），panel_config/panel_cert 两表，DDL 见 5.3；会话有效期参数不在库——载体为配置文件（account-auth D-10，见 3.4 配置项） |

### 1.3 设计决策

| # | 决策 | 理由 |
|---|------|------|
| D-P01 | panel_config 为**结构化单例表**（id=1 CHECK，强类型列 + CHECK 约束），非 KV 键值表 | 关闭 prd 待对齐-3：字段可约束（枚举 CHECK/端口范围）、可演进；单例语义与 init_state 同法（api-account-auth.md 4.4） |
| D-P02 | 监听配置**重启生效**：进程启动时从 panel_config 读绑定参数；运行期修改仅落库并置「待重启」派生标识，不热重绑 | 绑定套接字热迁移复杂度不值当（MVP 单实例）；重启动作归 systemctl，无自重启接口 |
| D-P03 | 安全入口校验为全局前置中间件 **EntryPathGuard**，置于 account-auth 中间件链（InitGuard 等）之前，作用于全部路由（含 /auth/* 与未认证访问）；未携带/错携带一律返回**标准 404**（无业务错误码包裹） | AA S-07-02 前置网关语义；404 不泄露「这是一个面板」的存在性（防探测，PRD N2） |
| D-P04 | HTTPS 证书独立 **panel_cert 单行表**（id=1）；key_pem 永不经任何接口下发，接口仅返回摘要（source/issuer/notAfter） | 关闭 prd 待对齐-4（独立表定稿）；私钥材料零下发对齐 I-09 先例（TOTP secret） |
| D-P05 | 证书上传以 **PEM 文本 JSON 字段**（certPem/keyPem）传输，不用 multipart | PEM 即文本，避免 multipart 边界与二进制编码分歧；服务端解析、配对校验并提取 issuer/有效期（R-14） |
| D-P06 | 切换 https_mode=self_signed 时**即时**生成自签证书落 panel_cert（source=self_signed）；TLS 证书经 GetCertificate 回调按连接热读取（除监听外全部配置项即时生效） | R-10 三态中仅监听需重启（D1 AA 附录C 生效分类）；热回调免重启 |
| D-P07 | **菜单树服务端下发**（GET /panel/menu/list）：构建期 ModuleMenuRegistry 登记为挂载点单一数据源，运行期按操作者角色过滤；前端不硬编码菜单 | FR-09/FR-10 的接口化；readonly 客户端不接触 admin 菜单定义（可见性即服务端判定，ADR-018） |
| D-P08 | 会话参数**透传只读**：GET /panel/config/detail 只读下发 internal/config 会话键当前值（absoluteTimeout/idleTimeout）；POST /panel/config/update 拒绝 session 字段 | account-auth D-10：会话策略载体=配置文件（panel-core 自有 internal/config 包），维护=改文件+重启，无 API；D2 关闭的跨域对齐落地 |
| D-P09 | 更新检查外呼**双重门控**：update_check=1 且 admin 显式触发才发生一次外呼；版本源经 ACL 适配器隔离（源标识与清单格式未锁定，prd 待对齐-1）；失败不自动重试 | R-09 零外呼承诺的接口化表达；I-P05 不变量 |
| D-P10 | 混合模式（MIXED）声明：全部对象实际为自管（无外部数据源，R-09），不设 manage_mode 字段，权限管控按角色区分 | 分册 Step 2 默认混合模式；本模块无托管侧 |
| D-P11 | `/panel/*` **无公开接口**：全部要求有效会话（版本信息亦然） | 未登录界面无版本/菜单诉求；最小暴露（idea.md §7） |
| D-P12 | **域名绑定并入 config/update**（bindDomain 字段）：携带 null/空串=解绑、非空合法域名=绑定、未携带=不变；即时生效（HostGuard 下一请求即用）。区别于 entry_path（随机生成值仅经 regenerate）——bind_domain 为用户输入值，update 即其唯一写入口；解绑不设独立接口（置空语义即解绑，避免双入口）。校验走 AUTH_012 fields（域名格式 RFC 1123 主机名、长度 ≤253、不含 scheme/path/端口，与端口范围/地址格式同通道）——不设专用错误码（site-mgmt SITE_007 为业务对象字段先例，配置字段沿用本模块 AUTH_012 惯例）；匹配口径（关闭 prd 待对齐-7）=请求 Host 头去除可选 `:port` 后与 bind_domain **全等比较（大小写不敏感）**，www 变体不自动匹配（从紧防误绑，需 www 需显式绑定该值）；绑定值对 readonly 可见（ADR-018 全可看，与 entry_path 同口径，FL 附录B） | ADR-028 强校验语义的最小接口面；宝塔同款「绑定即锁 Host」交互由 D4 承担确认警示 |
| D-P13 | **面板证书 ACME 复用 site-mgmt 通道（OHS 消费）**：签发/续期经 AcmeGateway（lego ACL），HTTP-01 挑战 location 临时注入/撤除经 nginx 安全写回（WritePipeline R-06 四步）；ACME 账户密钥（acme-account.key 0600）与 CA 目录（acme.directory_url 配置键）复用 site-mgmt 既有载体（D-09 同源），panel_cert 不另存账户引用；不新增独立 80 端口监听；**同步终态**（ACME_TIMEOUT=120s 上限，同值先例，超时即失败）。状态**读时惰性推算**（沿 site-mgmt D-08 同构）：valid/expiring_soon/expired 由 not_after 推算（CERT_EXPIRING_DAYS=30，ADR-024 阈值同值）、renew_failed 为落库态（last_error 非空呈现）；**首次申请失败不落库**（无证书行即「未配置」，失败原因仅本次响应与面板运行日志；续期失败落 last_error 于既有行）——关闭 prd 待对齐-6 状态字段形态部分 | ADR-029 跨模块消费契约的落法；不落 pending 态（同步终态无停留）；单例行无需多实例状态机 |
| D-P14 | **https_mode 扩展 acme 四态**：ACME 申请成功联动切 https_mode='acme'（唯一自动切换路径，R-10 三态基础上的来源态）；update httpsMode=acme 前置 panel_cert.source=acme 且推算未过期（PANEL_003 扩展语义）；手动切至其他态后 acme 证书行保留（可再切回）——关闭 prd 待对齐-6 模式联动部分 | ADR-029「https_mode 联动」的定稿形态：模式=当前 HTTPS 以何种来源证书运行 |
| D-P15 | **挑战 location 注入落点**（关闭 prd 待对齐-6 nginx 侧部分）：优先 server_name 匹配 bind_domain 的 80 端口 server 块，无则回退默认 server（listen 80 default_server，再无则首个 listen 80 块）；注入临时 location `/.well-known/acme-challenge/`（alias 指向面板临时挑战目录）；签发结束（成败皆然）撤除并 `nginx -t` 校验、失败回滚保障——全部写回经 site-mgmt WritePipeline（R-06/R-16，隔离写回其余站点零影响） | nginx 侧落点唯一未定项的从简落法：域名精确匹配优先、默认兜底；挑战文件由 AcmeGateway 写入临时目录 |
| D-P16 | **HostGuard 全局前置**：与 EntryPathGuard 同层 net/http 包装、紧随其后（Entry → Host → 引擎中间件链）；绑定后 Host 不匹配（含 IP 直连）一律**标准 404 无业务码**（与 D-P03 同防探测语义）；读态失败 fail-closed 404（沿 A-P7 先例）；作用于全部路由含未认证访问；绑定/解绑即时生效 | ADR-028 强校验语义；两网关失败响应同为标准 404，先后顺序不泄露信息 |

### 1.4 领域对象总览

| 聚合根 | 域内对象/值对象 | 子流程 | 关联领域对象 |
|--------|---------------|--------|-------------|
| panel_config（主数据类，配置单例） | HTTPS 证书（1:1 实体，来源 self_signed/imported/acme）、配置生效分类（域级策略值对象） | 面板证书自动续期任务（1，ADR-029，4.1.3） | 无（跨域协作均为 OHS 消费：审计写入；配置文件读取：会话策略载体；site-mgmt ACME 签发通道与 nginx 安全写回：面板证书，不持有 certificate 聚合根 ID） |

过程数据（不入库、不建接口对象）：版本比对结果（P2/P7，触发时计算即返回）、菜单树（P2，构建期登记派生）、「待重启」标识（P2，由生效分类与落库值对比派生）。

### 1.5 上下文映射

| 上下文 | 关系 | 机制 |
|--------|------|------|
| account-auth | 消费 | 遵奉者：Cookie+CSRF/RBAC 中间件链（InitGuard→AuthRequired→CsrfGuard→RoleRequired，本模块 EntryPathGuard/HostGuard 置于其前）；OHS：`AuditService.Record`（配置/证书变更审计，R-03）；internal/config 会话键为其会话策略载体（D-10）——本域只读透传（D-P08） |
| site-mgmt | 消费（OHS） | **跨模块消费契约（ADR-029，与 D1 被依赖登记对应 api-site-mgmt.md 1.5/5.1 v1.1）**：面板证书 ACME 签发/续期复用其 AcmeGateway（4.1.5 ACL）与 nginx 安全写回 WritePipeline/NginxGateway（4.2.5）——HTTP-01 挑战 location 临时注入/撤除走 R-06 四步；不新增独立 80 端口监听；ACME 账户密钥与 CA 目录复用其载体（D-P13）；契约要点见 5.1 消费表 |
| ACME CA（经 site-mgmt ACL 间接） | 间接外呼（受限） | 面板不直连 CA、不自建 ACL；R-09 例外语义同 site-mgmt ACME 判定（用户主动申请/续期调度，D-P13）；外部 ACME 概念经 AcmeGateway 翻译为止于证书产物（防腐层归 site-mgmt） |
| 全部业务模块 | 被依赖 | 构建期：ModuleMenuRegistry 挂载登记（菜单树单一数据源，D-P07）；运行期：EntryPathGuard 全局入口拦截 + HostGuard 域名绑定强校验（AA 附录B 契约，D-P16） |
| 版本源（外部） | 消费（受限） | 仅 update_check=1 且显式触发时单次外呼（I-P05）；ACL 适配器隔离源格式（待对齐-1） |
| install.sh / systemd | 部署契约（非接口） | 脚本每行可读（ADR-009）；unit 生成与开机自启（FR-01/02）；启动绑定参数读 panel_config（D-P02） |

## 2. 跨聚合根设计（单聚合根声明）

### 2.1 聚合根关系图

```
panel_config (panel_config, 单行 id=1)
    │ 1:1
    └─ HTTPS 证书 (panel_cert, 单行 id=1；自签/导入/acme 共用一行，替换式更新)
配置生效分类（值对象，域级策略：需重启项={listen.*}，即时项=其余）
```

### 2.2 跨聚合根不变量

单聚合根模块，无跨聚合根不变量；聚合内不变量见 4.1.1（I-P01~I-P08）。跨域约束：未初始化前 `/panel/*` 不可达——由 account-auth InitGuard（I-01）在 EntryPathGuard 之后拦截，本模块不重复实现。

### 2.3 跨聚合根编排

无跨聚合根编排。配置保存为 panel_config 单聚合原子事务 + 同事务审计写入（AuditService.Record，I-05 同事务语义）；证书切换与配置更新为两个接口协作的顺序操作（先 upload-certificate 后 update httpsMode），无独立流程聚合；面板证书 ACME 申请/续期为单动作同步接口（cert/request、cert/retry-renewal），挑战注入→签发→撤除的编排（D-P15）在服务层 PanelCertAcmeService 内完成，亦无独立流程聚合（子流程形态见 4.1.3）。

## 3. 应用/基础设施层设计

### 3.1 接口规范

| 项 | 规范 |
|----|------|
| 路径格式 | `/panel/{resource}/{action}` 三段式；资源=config/menu/system/cert |
| 入口前缀 | 开启安全入口后，实际访问路径 = `/{entryPath}` + 本文档路径（前缀由 EntryPathGuard 校验，接口文档一律省略前缀描述）；绑定域名后 Host 头须匹配 bind_domain（HostGuard 校验，D-P16） |
| HTTP 方法 | 读=GET，写=POST（面板内部契约，同 api-account-auth.md D-07） |
| 响应格式 | `{"code": 0, "message": "ok", "data": {...}}`；code=0 成功，非 0 见 3.4 错误码 |
| 时间格式 | ISO-8601 UTC（公共约定） |
| 认证 | HttpOnly Cookie `dg_session`（account-auth 签发，本模块只消费校验结果）；全部接口要求会话（D-P11） |
| CSRF | 变更类（POST）须带 `X-CSRF-Token` 头（R-02）；GET 豁免 |
| 中间件链 | **EntryPathGuard（本模块，全局第一环，D-P03）** → **HostGuard（本模块，域名绑定强校验，D-P16，绑定后不匹配 Host 一律标准 404）** → InitGuard → AuthRequired → CsrfGuard（POST）→ RoleRequired（按接口）——后四者归 account-auth（api-account-auth.md 3.1），顺序固定 |
| 用户类型标注 | 每接口标注：admin / admin+readonly（全可看，ADR-018）；无公开接口 |

### 3.2 依赖与约束

| 依赖 | 类型 | 约束 |
|------|------|------|
| account-auth 中间件链 | 框架契约 | EntryPathGuard 置于其前；本模块不自行解析会话/角色，消费中间件注入的 operator 上下文 |
| SQLite (modernc.org/sqlite) | 存储 | panel_config/panel_cert 单行表；配置保存与审计写入同事务（I-05 语义） |
| internal/config（配置文件） | 配置载体 | 会话键 session.absolute_timeout/idle_timeout 只读（D-10/D-P08）；监听启动值不走此文件——从 panel_config 表读（D-P02） |
| site-mgmt AcmeGateway + nginx 安全写回（WritePipeline/NginxGateway） | 进程内 OHS 消费 | 面板证书 ACME 签发/续期唯一通道（D-P13）：AcmeGateway.Issue/Renew（lego ACL，≤ACME_TIMEOUT=120s 同步）+ 挑战 location 注入/撤除经 WritePipeline R-06 四步（D-P15 落点）；契约见 api-site-mgmt.md 4.1.5/4.2.5、被依赖登记 1.5/5.1（v1.1）；具体装配（接口注入 vs 适配器直连）归 B 阶段 |
| crypto/x509 + crypto/tls | 证书 | PEM 解析、配对校验、自签生成（标准库）；TLS GetCertificate 回调热读取（D-P06） |
| 版本源 HTTP 客户端 | 外呼（受限） | 双重门控（I-P05）；单次触发单次请求，无自动重试（R-09）；超时上限为编译期常量（N1 待 B 阶段实测校准） |

### 3.3 权限模型

角色：admin / readonly（panel_role 固定枚举，消费 account-auth RoleRequired 判定）。

**接口权限矩阵**：

| 接口 | admin | readonly | CSRF |
|------|-------|----------|------|
| GET /panel/config/detail | ✓ | ✓（全可看，含安全入口路径值——FL 附录B 决策） | — |
| POST /panel/config/update | ✓ | ✗（AUTH_008） | ✓ |
| POST /panel/config/regenerate-entry-path | ✓ | ✗（AUTH_008） | ✓ |
| POST /panel/config/upload-certificate | ✓ | ✗（AUTH_008） | ✓ |
| GET /panel/cert/status | ✓ | ✓（证书状态与 ACME 概要可看，ADR-018） | — |
| POST /panel/cert/request | ✓ | ✗（AUTH_008） | ✓ |
| POST /panel/cert/retry-renewal | ✓ | ✗（AUTH_008，外呼动作从紧，同 update-check 先例） | ✓ |
| GET /panel/menu/list | ✓（全量） | ✓（服务端过滤为查看类） | — |
| GET /panel/system/version | ✓ | ✓ | — |
| POST /panel/system/update-check | ✓ | ✗（AUTH_008，从紧：外呼动作 PRD 推导） | ✓ |

数据权限规则：无行级隔离（配置单例）；敏感字段脱敏——panel_cert.key_pem 永不下发（I-P06），配置响应仅含证书摘要；bind_domain 值对 readonly 可见（ADR-018 全可看，与 entry_path 同口径）。

### 3.4 公共规范

**错误码**（PANEL_ 前缀为本模块新增；鉴权/CSRF/参数校验复用 account-auth 码族）：

| code | 标识 | HTTP 语义 | 说明 |
|------|------|----------|------|
| 0 | OK | 成功 | — |
| AUTH_001/002/003 | （复用） | 401/403 | 未登录/会话过期/CSRF 失败——中间件归 account-auth |
| AUTH_008 | （复用）FORBIDDEN | 拒绝 | readonly 越界，附边界原因（ADR-018） |
| AUTH_012 | （复用）VALIDATION_FAILED | 参数错误 | data.fields[] 逐字段错误（R-14：端口范围/地址格式/取值枚举） |
| PANEL_001 | CUSTOM_ADDRESS_REQUIRED | 参数冲突 | listen_scope=custom 但 listen_address 缺失或非法（I-P02） |
| PANEL_002 | CERT_INVALID | 参数错误 | 证书 PEM 解析失败或证书/私钥不配对（I-P04，R-14） |
| PANEL_003 | CERT_REQUIRED | 状态冲突 | https_mode 切至 imported/acme 但 panel_cert 无对应来源的可用证书行（I-P04；v2.0 扩展 acme 分支，D-P14） |
| PANEL_004 | UPDATE_CHECK_DISABLED | 状态冲突 | update_check=0（默认）时触发检查被拒（I-P05，R-09） |
| PANEL_005 | UPDATE_CHECK_FAILED | 上游失败 | 版本源不可达/超时/清单格式不符；不自动重试（D-P09） |
| PANEL_006 | LANG_NOT_SUPPORTED | 参数错误 | 语言取值不受支持（MVP 仅 zh-CN，I-P08） |
| PANEL_007 | BIND_DOMAIN_REQUIRED | 状态冲突 | cert/request·retry-renewal 前置 bind_domain 未绑定（ACME 域名=bind_domain，D-P12/D-P13，ADR-028） |
| PANEL_008 | NGINX_UNAVAILABLE | 拒绝 | nginx 未检出或 80 挑战通道不可用（data.reason；语义沿 site-mgmt SITE_012 空态先例，D-P15） |
| PANEL_009 | ACME_FAILED | 操作失败 | ACME 签发/续期失败（挑战不通/CA 拒绝/超时 ≤ACME_TIMEOUT），data.reason（沿 site-mgmt CERT_004/D-09 口径，不自动无限重试）；续期失败落 panel_cert.last_error（renew_failed 呈现，D-P13） |
| PANEL_010 | ACME_STATE_INVALID | 状态冲突 | 证书非 acme 来源或当前状态不允许该操作（retry 要求 renew_failed；request 在非 acme 来源行上为替换语义不触发本码），data.currentStatus |
| PANEL_011 | ACME_CERT_EXISTS | 参数冲突 | 已有 source=acme 且推算 valid/expiring_soon 的证书再次申请（沿 site-mgmt D-10 同域名防重先例，防 CA 配额浪费），data.notAfter——续期走 retry-renewal/自动调度 |

**枚举**（均在 workspace/01-solution/04-business-enum.md 定义；v2.0 增量：https_mode/panel_cert_source 取值扩展、新增 cert_status 引用）：

| 枚举标识 | 取值 | 用途 |
|---------|------|------|
| https_mode | off \| self_signed \| imported \| acme | HTTPS 模式四态（R-10 三态 + v2.0 扩展 acme：ACME 申请成功联动切换，D-P14，ADR-029） |
| listen_scope | all \| localhost \| custom | 监听范围（默认 all=0.0.0.0） |
| panel_cert_source | self_signed \| imported \| acme | 面板证书来源（acme 为 D2 增量扩展，ADR-029） |
| cert_status | pending \| valid \| expiring_soon \| expired \| renew_failed | 面板证书状态**引用**（site-mgmt D2 定稿，ADR-024 六状态语义）：本模块呈现 4 值子集 valid/expiring_soon/renew_failed/expired（同步终态无 pending 停留，D-P13 读时推算；cert/status 响应与 detail 摘要共用） |
| config_effect_mode | immediate \| restart_required | 配置生效方式（D1 AA 附录C：仅 listen.* 为 restart_required；bind_domain 为 immediate） |

布尔约定：entry_path_enabled / update_check 以 INTEGER 0/1 存储（公共布尔约定，同 api-account-auth.md panel_user_totp.enabled）；lang 为字符串，MVP 固定 zh-CN（多语言二期再登记 panel_lang 枚举，prd 待对齐项已注明）。

**业务常量**（审计细粒度动作码，非枚举池条目；category=audit_action.change）：`panel_config_changed`（detail 携带 {field, before, after}[]——v2.0 起 field 可为 bindDomain，值为公开访问名明文记录）/ `panel_cert_updated` / `panel_entry_path_regenerated` / `panel_cert_requested` / `panel_cert_renewed`（source=system 后台动作）/ `panel_cert_renewal_retried`（object_type ∈ panel_config / panel_cert）。域名绑定/解绑经 config/update 落 panel_config_changed（不另设动作码，D-P12）。手动更新检查触发不落审计（PRD FR-16 审计口径：仅开关变更；外呼受开关+显式触发双重约束）。

**业务常量**（操作参数与阈值，非枚举池条目）：

| 常量 | 值 | 说明 |
|------|-----|------|
| ACME_TIMEOUT | 120s | 面板证书 ACME 签发/续期同步等待上限，超时即失败（D-P13；值沿 site-mgmt D-09 同值先例） |
| CERT_EXPIRING_DAYS | 30 | expiring_soon 推算阈值（D-P13；ADR-024 阈值同值，不可配） |
| ACME_CHALLENGE（面板侧） | http_01（固定） | 面板证书挑战方式固定 HTTP-01（TLS-ALPN/DNS-01 不适用面板场景，ADR-029）；非 acme_challenge 枚举请求参数 |

**配置项**：

| 配置 | 载体 | 默认 | 可改 | 说明 |
|------|------|------|------|------|
| session.absolute_timeout / session.idle_timeout | 配置文件 internal/config（D-10） | 12h / 2h | 改文件+重启（无 API，D-P08） | GET /panel/config/detail 只读透传当前值（ADR-019） |
| update-check.source / update-check.timeout | 编译期常量 | 待对齐-1 | ✗ | 版本源标识与清单格式未锁定；ACL 适配器隔离（D-P09） |
| listen.*（监听地址/端口） | panel_config 表 | all + 安装时生成随机高位端口 | ✓（POST /panel/config/update，重启生效） | 启动时从表读绑定（D-P02） |
| acme.directory_url | site-mgmt 配置文件（D-09，**复用不另设**） | Let's Encrypt 生产 | ✗（归 site-mgmt 治理） | 面板证书与站点证书经同一 CA 目录签发（D-P13；staging 切换随其配置） |

## 4. 聚合根详细设计

### 4.1 panel_config（主数据类）

#### 4.1.1 领域模型

**表结构组合**：panel_config（主表，单行 id=1）+ panel_cert（1:1 子表，单行 id=1，D-P04）。

**字段清单**：

panel_config：

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK CHECK(id=1) | 单例保证（I-P01） |
| listen_scope | TEXT | NOT NULL DEFAULT 'all' CHECK(in all,localhost,custom) | listen_scope |
| listen_address | TEXT | NULL | listen_scope=custom 时的监听地址（I-P02 必填） |
| listen_port | INTEGER | NOT NULL | 监听端口；安装时生成随机高位端口，运行期校验 1024..65535（I-P02）；修改重启生效 |
| entry_path_enabled | INTEGER | NOT NULL DEFAULT 0 | 0/1（R-10 可选开关） |
| entry_path | TEXT | NULL | 安全入口路径值；enabled=1 时必填，随机生成（I-P03），仅经 regenerate 接口变更 |
| bind_domain | TEXT | NULL | 绑定域名（v2.0 新增，ADR-028）：NULL=未绑定（放行全部 Host）；非空时 HostGuard 强校验（I-P09），修改即时生效；唯一写入口=config/update.bindDomain（D-P12） |
| https_mode | TEXT | NOT NULL DEFAULT 'off' CHECK(in off,self_signed,imported,acme) | https_mode 四态（R-10 三态 + v2.0 扩展 acme，D-P14） |
| update_check | INTEGER | NOT NULL DEFAULT 0 | 0/1，默认关（R-09/I-P05） |
| lang | TEXT | NOT NULL DEFAULT 'zh-CN' | 界面语言（MVP 仅 zh-CN，I-P08） |
| updated_at | DATETIME | NOT NULL | 最近保存时间 |

panel_cert：

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK CHECK(id=1) | 单行；导入/自签/acme 共用一行替换式更新（D-P04） |
| cert_pem | TEXT | NOT NULL | 证书 PEM |
| key_pem | TEXT | NOT NULL | 私钥 PEM；永不下发（I-P06） |
| source | TEXT | NOT NULL CHECK(in self_signed,imported,acme) | panel_cert_source（v2.0 扩展 acme，ADR-029） |
| issuer | TEXT | NULL | 颁发者摘要（自签=面板自身） |
| not_before / not_after | DATETIME | NULL | 有效期摘要（设置页只读展示；acme 态为推算输入，D-P13） |
| last_error | TEXT | NULL | ACME 签发/续期失败原因（v2.0 新增；acme 来源排障，renew_failed 落库态呈现；首次申请失败无行不落库，D-P13） |
| updated_at | DATETIME | NOT NULL | — |

**不变量**（聚合内）：

| 编号 | 不变量 |
|------|--------|
| I-P01 | panel_config 全局单行（id=1，启动时 INSERT OR IGNORE 确保存在，同 init_state 先例） |
| I-P02 | listen_scope=custom ⇔ listen_address 为合法 IP/主机名；listen_port ∈ 1024..65535 |
| I-P03 | entry_path_enabled=1 ⇔ entry_path 匹配 `^[A-Za-z0-9_-]{8,32}$`（生成器输出空间）；修改即时生效 |
| I-P04 | https_mode=imported ⇔ panel_cert 存在且 cert/key 配对（切换时校验，失败 PANEL_002/003 拒绝）；https_mode=acme ⇔ panel_cert.source=acme 且推算未过期（切换/联动时校验，PANEL_003，v2.0 扩展） |
| I-P05 | update_check=0 时一切外呼禁止（R-09）；=1 时仅显式触发产生单次外呼 |
| I-P06 | key_pem 及一切私钥材料永不经任何接口下发；证书仅摘要可见（3.3 脱敏） |
| I-P07 | 配置保存原子（单事务写 panel_config），失败保持原值，不允许半写（PRD N2） |
| I-P08 | lang ∈ {zh-CN}（MVP）；其余取值 PANEL_006 拒绝 |
| I-P09 | bind_domain 为空 ⇔ 未绑定（放行全部 Host）；非空时请求 Host 头去除可选 `:port` 后与 bind_domain 全等（大小写不敏感）方放行，不匹配（含 IP 直连）一律标准 404 无业务码；校验态读取失败按 fail-closed 拒绝（404）——HostGuard 强校验（D-P12/D-P16，ADR-028） |
| I-P10 | panel_cert 单一行替换式更新（D-P04）：acme 申请成功即 UPSERT（source=acme）并联动 https_mode=acme（D-P14）；acme 状态读时推算（valid/expiring_soon/expired 由 not_after、renew_failed=last_error 非空，D-P13），无独立状态列；ACME 编排（挑战注入→签发→撤除）成败均恢复 nginx 配置原状（R-06 回滚保障） |

**领域服务**：EntryPathGenerator（随机路径，I-P03 字符集）、SelfSignedCertIssuer（自签生成并落库，D-P06）、CertPairValidator（PEM 解析/配对/摘要提取，R-14）、RestartClassifier（生效分类判定，驱动响应内 effectMode 与 pendingRestart）、UpdateCheckService（门控+单次外呼+比对，ACL 适配版本源，D-P09）、HostGuard（Host 强校验判定：I-P09 匹配口径与 fail-closed，D-P16）、PanelCertAcmeService（面板证书 ACME 申请/续期编排：前置校验→挑战 location 注入（D-P15 落点）→AcmeGateway 签发（经 site-mgmt OHS）→撤除→落库/last_error，D-P13）、PanelCertStatusCalculator（acme 状态读时推算，D-P13）、RenewalScheduler（每日后台调度，4.1.3）。

#### 4.1.2 状态模型

无生命周期状态机（D0 拍板：配置为单例资源）。辨析（prd 第六部分）：https_mode 为模式枚举可任意切换（v2.0 含 acme 四态，ACME 申请成功为唯一自动切换路径，D-P14）、entry_path_enabled/update_check 为布尔开关、bind_domain 为可空配置字段（I-P09 门槛语义，非状态）、「待重启」为派生展示态（落库值 ≠ 进程启动绑定值时成立，重启后消失，不落库）、面板证书 acme 状态为读时推算的简化标注（valid/expiring_soon/expired/renew_failed，沿 ADR-024 语义简化引用，非多实例生命周期状态机，D-P13）——均非状态流转。历史追溯：全部变更经 audit_log（R-03），无配置历史表（PRD N4）。

#### 4.1.3 子流程设计

**面板证书自动续期任务**（v2.0 新增，ADR-029；与 site-mgmt 自动续期任务同构、经跨模块通道执行，I/O 契约对齐 domain-model.md v1.16 第四部分）：

| 项 | 契约 |
|----|------|
| 触发 | 每日后台调度（进程内 cron）；panel_cert.source=acme 且读时推算为 expiring_soon 时发起单次尝试 |
| 输入 | panel_cert（not_after 推算输入）+ panel_config.bind_domain（续期域名——域名变更后续期即按新域名签发，替换式更新 D-P04） |
| 处理 | PanelCertAcmeService 同一通道（cert/retry-renewal 共用）：挑战 location 注入（D-P15）→ AcmeGateway 签发（≤ACME_TIMEOUT=120s，经 site-mgmt OHS）→ 撤除+`nginx -t`（WritePipeline R-06）；成功：not_before/not_after/issuer 回写 + last_error=NULL（推算回 valid）；失败：last_error 落库（renew_failed 呈现） |
| 输出 | panel_cert 证书字段与 last_error；成功/失败均写审计 panel_cert_renewed（change，source=system，R-03） |
| 失败处理 | 不自动重试（等待次日调度或管理员手动 cert/retry-renewal，D-P13 沿 site-mgmt D-09 每日单次口径）；nginx 不可用（PANEL_008 语义）时本次跳过并记 last_error |

#### 4.1.4 接口设计

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /panel/config/detail | GET | admin+readonly | 配置单例全量读取（含生效状态与会话透传） |
| /panel/config/update | POST | admin | 配置项部分更新（生效分类判定；含绑定域名 bindDomain，D-P12） |
| /panel/config/regenerate-entry-path | POST | admin | 重新生成随机安全入口路径 |
| /panel/config/upload-certificate | POST | admin | 导入证书与私钥（PEM） |
| /panel/cert/status | GET | admin+readonly | 面板证书状态视图（来源/推算状态/失败原因/ACME 概要，D-P13） |
| /panel/cert/request | POST | admin | 面板证书 ACME 申请（HTTP-01 经已有 nginx，同步终态，D-P13/D-P15） |
| /panel/cert/retry-renewal | POST | admin | renew_failed 手动重试（进入 4.1.3 同一通道，同步终态） |
| /panel/menu/list | GET | admin+readonly | 菜单树（按角色过滤） |
| /panel/system/version | GET | admin+readonly | 版本与构建信息 |
| /panel/system/update-check | POST | admin | 手动触发一次版本检查 |

**机制型能力（无独立接口）**：FR-07 首访 HTTPS 建议——前端以请求 scheme 对比 detail.https.mode 派生提示，无接口；FR-08 布局与路由骨架——构建期路由，无接口；FR-11 认证页面承载——路由容器复用 account-auth 的 /auth/init/*、/auth/session/* 接口；监听重启动作——systemctl（FR-02），无接口。

**接口详情**（请求/响应/校验）：

- **GET /panel/config/detail**：无参；data=`{listen:{scope, address|null, port, effectMode:"restart_required"}, entryPath:{enabled, path, effectMode:"immediate"}, bindDomain:{value:string|null, effectMode:"immediate"}, https:{mode, cert:{source, issuer, notBefore, notAfter}|null, effectMode:"immediate"}, updateCheck:{enabled, effectMode:"immediate"}, lang:{value, supported:["zh-CN"], effectMode:"immediate"}, session:{absoluteTimeout:int64, idleTimeout:int64}, pendingRestart:boolean}`；bindDomain.value=panel_config.bind_domain 当前值（null=未绑定，readonly 可见，I-P09）；pendingRestart=listen 组落库值 ≠ 进程启动绑定值（D-P02 派生标识，readonly 可见）。session 键为 internal/config 当前值只读透传（D-P08），两值为**秒数整数 int64**（tasks A-P2 裁定；2026-11-16 WARN 修复轮 W1 钉死，与后端 SessionView 一致；前端展示格式化为人类可读时长）。审计无。
- **POST /panel/config/update**：req 为部分字段集 `{listenScope?, listenAddress?, listenPort?, entryPathEnabled?, bindDomain?, httpsMode?, updateCheck?, lang?}`——entry_path 值不可经此接口设置（仅 regenerate）；bindDomain：携带 null/空串=解绑、非空=绑定（域名格式 RFC 1123 主机名、≤253、不含 scheme/path/port，AUTH_012 fields 错误，D-P12）、未携带=不变；含 session 字段 → AUTH_012（field 错误，D-P08）。校验顺序：字段合法性（AUTH_012 fields：端口范围/地址格式/域名格式/枚举取值，R-14）→ I-P02 custom 跨字段（PANEL_001）→ I-P08（PANEL_006）→ httpsMode 切换前置：to imported 校验证书行存在且配对（PANEL_002/003）、to acme 校验 panel_cert.source=acme 且推算未过期（PANEL_003，D-P14）、to self_signed 即时生成自签证书（D-P06）、to off 直接生效；entryPathEnabled 0→1 且无路径值时自动生成（等价 regenerate 语义，落 audit detail）。处理：单事务原子保存（I-P07）+ 同事务审计 panel_config_changed（detail={field,before,after}[]，object_type=panel_config；bindDomain 明文——公开访问名非机密）；监听项修改不热生效（D-P02），bindDomain 即时生效（HostGuard 下一请求即用，D-P16），其余即时生效。响应 data=更新后配置视图（结构同 detail）。
- **POST /panel/config/regenerate-entry-path**：无 req；EntryPathGenerator 生成新值替换 entry_path（保持 enabled 状态不变），即时生效（EntryPathGuard 下一请求即用新值）；审计 panel_entry_path_regenerated（object_type=panel_config，detail 不含明文路径值——审计页面对 readonly 可见，路径值脱敏为长度+前 2 字符掩码）；响应 data=`{entryPath}`。
- **POST /panel/config/upload-certificate**：req=`{certPem, keyPem}`（PEM 文本，D-P05）；校验 CertPairValidator：解析失败/不配对 → PANEL_002（R-14）；处理：UPSERT panel_cert（source=imported，提取 issuer/notBefore/notAfter，last_error 置 NULL）；https_mode 不变（模式切换走 update）；审计 panel_cert_updated（object_type=panel_cert）；响应 data=`{cert:{source:"imported", issuer, notBefore, notAfter}}`。
- **GET /panel/cert/status**：无参；data=`{cert:{source(panel_cert_source), status:cert_status|null, issuer, notBefore, notAfter, lastError:string|null}|null, acme:{domain:string|null（=bind_domain 当前值）, challenge:"http_01", autoRenew:true（内置每日调度，固定开启，4.1.3）}}`；cert=null 即未配置证书；status 由 PanelCertStatusCalculator 读时推算（D-P13：valid/expiring_soon/expired 由 notAfter、renew_failed=lastError 非空；source≠acme 时 status=null——自签/导入无续期语义）。审计无（查看类）。
- **POST /panel/cert/request**：无 req（域名=bind_domain、挑战=http_01 固定，D-P13）。校验链：bind_domain 非空（PANEL_007）→ nginx 可用（PANEL_008，data.reason）→ 防重：panel_cert.source=acme 且推算 ∈ {valid, expiring_soon} → PANEL_011（data.notAfter，提示走续期通道）；source=self_signed/imported 或无行 → 放行（替换式更新语义）。执行（PanelCertAcmeService，同步 ≤ACME_TIMEOUT=120s）：WritePipeline 注入挑战 location（D-P15 落点，R-06 备份→注入）→ AcmeGateway.Issue(domains=[bind_domain], http_01，经 site-mgmt OHS）→ 撤除+`nginx -t`（失败回滚保障）→ 成功：UPSERT panel_cert（source=acme、issuer/notBefore/notAfter、last_error=NULL）+ https_mode='acme' 联动（D-P14）；失败：挑战注入已撤除/回滚，返回 PANEL_008/009（data.reason），首次申请失败无证书行不落库（D-P13）。审计 panel_cert_requested（change，detail={domain, result}，object_type=panel_cert）。响应 data=`{cert:{source:"acme", status, issuer, notBefore, notAfter}, httpsMode:"acme"}`。
- **POST /panel/cert/retry-renewal**：无 req。校验链：panel_cert 行存在且 source=acme（否则 PANEL_010，data.currentSource）→ 推算/落库态=renew_failed（valid/expiring_soon 走自动调度无需重试、expired 重新 request，PANEL_010 data.currentStatus）→ bind_domain 非空（PANEL_007）。执行：进入 4.1.3 同一通道（PanelCertAcmeService，同步终态；续期域名=当前 bind_domain，域名变更后续期即换域名证书）；成功：UPDATE not_before/not_after/issuer + last_error=NULL（推算回 valid）+（域名变更场景）domains 随证书产物更新；失败：UPDATE last_error（保持 renew_failed 呈现）+ PANEL_009（data.reason）。审计 panel_cert_renewal_retried（change，结果一并 detail）。响应 data 同 request 形态。
- **GET /panel/menu/list**：无参；ModuleMenuRegistry（构建期登记）为数据源，按操作者角色过滤：admin=全量、readonly=仅 visibility 含 readonly 的查看类节点（D-P07）；data=`{items:[{key, title, path, icon, order, children:[...]}]}`；审计无。
- **GET /panel/system/version**：无参；数据源=构建 ldflags 注入（FR-03）；data=`{version, gitCommit, buildTime}`；审计无。
- **POST /panel/system/update-check**：无 req；前置 I-P05：update_check=0 → PANEL_004；UpdateCheckService 单次外呼（ACL 适配版本源，源未锁定=prd 待对齐-1，超时常量见 3.4）比对语义化版本；成功 data=`{currentVersion, latestVersion|null, updateAvailable, checkedAt}`；失败 PANEL_005（不自动重试，D-P09）；结果不落库（过程数据）；审计无（PRD FR-16 口径，见 3.4 常量注）。

**跨接口复用规则**：审计写入统一走 `AuditService.Record`（account-auth OHS，同事务 I-05）；生效分类判定统一走 RestartClassifier（响应 effectMode/pendingRestart 单一来源）；证书摘要提取统一走 CertPairValidator（upload 与 detail 共用）；ACME 通道统一 PanelCertAcmeService（request / retry-renewal / 4.1.3 调度三者共用，经 site-mgmt AcmeGateway 与 nginx 安全写回 OHS，D-P13/D-P15）；acme 状态推算统一 PanelCertStatusCalculator（cert/status 与 detail 摘要共用，D-P13）；HostGuard 与 EntryPathGuard 同层装配（顺序固定，D-P16）。

#### 4.1.5 领域事件与集成

无 MQ/事件总线（单机单进程，同 api-account-auth.md D-05）。仓储契约：`ConfigRepository { Get, Update(tx), EnsureSingleton }`；`CertRepository { Get, Upsert, UpdateAcmeResult(tx)（续期回写 not_before/not_after/issuer/last_error）, Delete }`。关联域调用：出站 AuditService.Record（account-auth OHS，配置/证书变更审计）；面板证书 ACME 经 **site-mgmt OHS 消费**（5.1 消费表）；版本源外呼经 UpdateCheckService ACL 适配（双重门控 I-P05）。被依赖契约：EntryPathGuard、HostGuard（全局中间件，5.1）、ModuleMenuRegistry（构建期登记）。

## 5. 模块清单汇总

### 5.1 对外服务清单（进程内契约/中间件）

| 服务/契约 | 消费方 | 契约要点 |
|----------|--------|---------|
| EntryPathGuard（中间件） | api 路由层全局第一环（含 /auth/* 与全部业务路由） | 未携带/错携带安全入口 → 标准 404 无业务码（D-P03）；开关关闭直通；作用于未认证访问 |
| HostGuard（中间件，v2.0 新增） | api 路由层全局第二环（EntryPathGuard 之后，D-P16） | bind_domain 非空时 Host 不匹配（含 IP 直连）→ 标准 404 无业务码（I-P09/ADR-028）；未绑定直通；读态失败 fail-closed 404；作用于全部路由含未认证访问 |
| ModuleMenuRegistry（构建期登记约定） | 全部业务模块 | 模块以 {key,title,path,icon,order,visibility} 登记；GET /panel/menu/list 单一数据源（D-P07） |
| 设置页透传区（detail.session 键） | account-auth 会话策略展示 | 只读透传 internal/config 会话键（D-P08/D-10），本模块不提供其修改接口 |
| 中间件链顺序约束 | api 路由层 | EntryPathGuard → HostGuard → InitGuard → AuthRequired → CsrfGuard → RoleRequired（3.1，顺序固定） |

**跨模块消费契约（本模块引入，v2.0；与 D1 被依赖登记对应 api-site-mgmt.md 1.5/5.1 v1.1）**：

| 消费契约 | 提供方 | 契约要点 |
|---------|--------|---------|
| AcmeGateway（lego ACL） | site-mgmt（api-site-mgmt.md 4.1.5，D-09） | `Issue(domains, challenge=http_01, timeout≤120s)` / `Renew(...)`——面板证书签发/续期的 ACME 协议交互；外部 CA 概念止步于其防腐层；ACME 账户密钥（acme-account.key 0600）与 CA 目录（acme.directory_url）复用其载体（D-P13）；R-09 例外语义（用户主动申请/续期调度） |
| nginx 安全写回（WritePipeline/NginxGateway） | site-mgmt（api-site-mgmt.md 4.2.5，R-06/R-16） | HTTP-01 挑战 location 的临时注入与撤除：备份→注入 `/.well-known/acme-challenge/`（D-P15 落点）→签发→撤除→`nginx -t`→失败回滚；隔离写回其余站点零影响；不新增独立 80 端口监听（ADR-029） |

### 5.2 SQL 清单

| # | SQL | 场景 |
|---|-----|------|
| 1 | INSERT INTO panel_config(id,...) VALUES(1,...) OR IGNORE | 启动自检确保单例（I-P01） |
| 2 | SELECT * FROM panel_config WHERE id=1 | 启动绑定读取（D-P02）+ GET detail + HostGuard 读态（I-P09 fail-closed 输入） |
| 3 | UPDATE panel_config SET {变更列}, updated_at=? WHERE id=1 | 配置保存（单事务，I-P07，同事务审计；v2.0 起变更列可含 bind_domain） |
| 4 | INSERT OR REPLACE INTO panel_cert(id, cert_pem, key_pem, source, issuer, not_before, not_after, last_error, updated_at) VALUES(1,...) | 证书导入/自签生成/ACME 申请成功落库（acme 时 last_error=NULL，D-P13） |
| 5 | SELECT cert_pem, key_pem, source, issuer, not_before, not_after, last_error FROM panel_cert WHERE id=1 | TLS 启动/回调读取 + detail/cert-status 摘要与推算输入 |
| 6 | UPDATE panel_cert SET not_before=?, not_after=?, issuer=?, last_error=NULL, updated_at=? WHERE id=1 | ACME 续期成功回写（4.1.3 / retry-renewal，推算回 valid） |
| 7 | UPDATE panel_cert SET last_error=?, updated_at=? WHERE id=1 | ACME 续期失败落库（renew_failed 呈现，D-P13） |

### 5.3 DDL

```sql
CREATE TABLE IF NOT EXISTS panel_config (
  id                 INTEGER PRIMARY KEY CHECK (id = 1),
  listen_scope       TEXT    NOT NULL DEFAULT 'all' CHECK (listen_scope IN ('all','localhost','custom')),
  listen_address     TEXT,
  listen_port        INTEGER NOT NULL CHECK (listen_port BETWEEN 1024 AND 65535),
  entry_path_enabled INTEGER NOT NULL DEFAULT 0,
  entry_path         TEXT,
  bind_domain        TEXT,
  https_mode         TEXT    NOT NULL DEFAULT 'off' CHECK (https_mode IN ('off','self_signed','imported','acme')),
  update_check       INTEGER NOT NULL DEFAULT 0,
  lang               TEXT    NOT NULL DEFAULT 'zh-CN',
  updated_at         DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS panel_cert (
  id         INTEGER PRIMARY KEY CHECK (id = 1),
  cert_pem   TEXT    NOT NULL,
  key_pem    TEXT    NOT NULL,
  source     TEXT    NOT NULL CHECK (source IN ('self_signed','imported','acme')),
  issuer     TEXT,
  not_before DATETIME,
  not_after  DATETIME,
  last_error TEXT,
  updated_at DATETIME NOT NULL
);
```

**增量迁移（v2.0，编号接力既有 001~006 → 007_panel_core_acme.sql；SQLite ALTER 限制说明）**：

1. `ALTER TABLE panel_config ADD COLUMN bind_domain TEXT;`（新增无约束列，直接加列）；
2. panel_config（https_mode CHECK 扩展 +acme）与 panel_cert（source CHECK 扩展 +acme、新增 last_error 列）涉及 CHECK 约束变更，SQLite 不支持 ALTER 修改约束——需事务内表重建流程承载（CREATE 新表（本节 DDL 形态）→ INSERT SELECT 迁移既有单行 → DROP 旧表 → RENAME；实现归 B 阶段迁移脚本，语义以本节 DDL 为唯一事实源，禁止自行增删列）。

## 6. 更新记录

### 6.1 变更记录

| 版本 | 日期 | 变更类型 | 变更摘要 | 目标节号 |
|------|------|---------|---------|---------|
| v1.0 | 2026-11-12 | initial | 首次生成：7 个 HTTP 接口（/panel/{config,menu,system}/*，全部需会话、无公开端点）+ 全局中间件 EntryPathGuard + 构建期 ModuleMenuRegistry 登记契约 + 2 表 DDL（panel_config/panel_cert 单行表）+ 4 枚举（D2 已定稿，无新增）+ 6 新增错误码（PANEL_001~006，复用 AUTH_001/002/003/008/012）；关闭 prd 待对齐-3（结构化单例表）/待对齐-4（独立证书表）；无旧版对比 | — |
| v2.0 | 2026-11-16 | interface_added / field_added / enum_value_added / rule_changed / table_changed | 增量 D3（ADR-028/029，输入 prd v1.1/domain-model v1.16/D1 三文档 v1.1）：**新增接口 3 个**——GET /panel/cert/status（两角色）、POST /panel/cert/request（ACME 申请，HTTP-01 经已有 nginx、同步终态 ≤120s）、POST /panel/cert/retry-renewal（手动重试，4.1.3 同通道），域名绑定/解绑**并入** config/update（bindDomain 字段，D-P12 设计判断：用户输入值走 update 唯一写入口，解绑=置空，不设独立接口）；**字段新增**——panel_config.bind_domain（HostGuard 强校验 I-P09/D-P16）、panel_cert.last_error（acme 排障）；**枚举扩展**——https_mode +acme（D-P14 联动，池行同步）、panel_cert_source +acme（D2 已扩，本文对齐）、新增 cert_status 引用（4 值子集呈现）；**规则**——设计决策 D-P12~D-P16、不变量 I-P04 扩展 + I-P09/I-P10、子流程 4.1.3 面板证书自动续期任务、中间件链 +HostGuard、跨模块消费契约（site-mgmt AcmeGateway + nginx 安全写回，5.1 消费表，与 D1 登记对应）；**DDL**——两表 CHECK 扩展与新列 + 增量迁移 007（表重建流程，5.3）；**错误码** PANEL_007~011 接力（PANEL_003 语义扩展 acme 分支）；关闭 prd 待对齐-6/-7；M1~M5 增量重跑通过 | 1.2/1.3/1.4/1.5/2.1/2.3/3.1/3.2/3.3/3.4/4.1.1/4.1.2/4.1.3/4.1.4/4.1.5/5.1/5.2/5.3 |

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
