# 斗拱面板 网站管理（site-mgmt）API 设计

| 属性 | 值 |
|------|-----|
| 版本 | v1.0 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-12 |
| 输入 | prd-site-mgmt.md v1.0（完备性检查通过：G 5/5、T 7/7、N 4/4）、domain-model.md v1.7（核心域判定 + certificate 聚合根 + site 投影实体）、d0-clarification-site-mgmt.md、D1 三文档（BA/AA/FL v1.0） |
| 接口规约 | 无激活技术栈（ACTIVE_STACKS 为空）——按通用 REST/JSON 契约设计；认证=HttpOnly Cookie 会话 + CSRF token（R-02，复用 account-auth）；鉴权=RBAC 两角色边界矩阵（ADR-018）；安全写入=R-06 四步强制前置（本模块核心约束） |

## 1. 模块设计概述

### 1.1 业务背景及目标

网站管理是斗拱面板的核心卖点模块（M3，材料明示工作量最大）：存量 nginx 站点不重装、不重建——扫描惯例配置目录解析为结构化站点对象（R-15），面板内结构化与源码双模式编辑后经固定安全流程写回原文件（R-06 备份→修改→`nginx -t`→失败自动回滚；R-07 Raw 原文保留；R-16 其余站点零影响），并提供三模板建站、站点启停/日志/重载与证书全生命周期治理（ACME HTTP-01/TLS-ALPN 经 ACL 申请、部署、自动续期、存量识别导入，ADR-024 六状态机）。round-trip 保真是全部写回能力的共同前提（样本集 ≥ 20 为验收硬项，idea.md §6 M3）。服务对象：管理员（admin）与只读用户（readonly）——readonly 查看站点列表/详情、证书清单与日志尾部（D0 口径 + ADR-018）。

### 1.2 边界说明

| 项 | 说明 |
|----|------|
| 本模块做 | 存量扫描导入（扫描→指令树解析→server 块提取→写回落点登记）、安全写入管线（R-06 四步+R-16 隔离）、站点列表/详情/结构化编辑/源码编辑、三模板建站、站点启停/日志尾部/重载、证书签发（ACME）/部署/解除/自动续期/手动重试/清单/删除 |
| 本模块不做（接口边界） | ① Apache/Caddy 等 web server、多 nginx 实例——D0 OUT；② DNS-01 通配符证书、HSTS/安全头面板——二期 OUT；③ 全局主配置（nginx.conf）编辑——⚠️A17 不提供入口；④ 站点删除——OUT-6（停用≠删除）；⑤ PHP 安装/pool 创建——OUT-7（仅对接已有 pool，只读实况）；⑥ nginx 主配置改造（include 注入）——接管不改造（PRD OUT-9）；⑦ 认证/鉴权/审计本体——复用 account-auth 中间件与 AuditService.Record（仅声明依赖）；⑧ 站点页面外壳与导航——归 panel-core |
| 与存储的关系 | SQLite 单文件（与 account-auth 同库）；certificate 单表（5.3，本模块唯一持久化对象）；site 为配置文件实时投影不落库（D2 判定零站点台账） |

### 1.3 设计决策

| # | 决策 | 理由 |
|---|------|------|
| D-01 | **实时扫描投影**：site/list、site/detail 每次请求即时扫描解析配置目录，无缓存层、无陈旧回退，响应带 generatedAt；扫描触发时机=请求时实时（无后台常驻扫描、无文件监听） | file-manager I-06「投影真实性/无缓存」同口径（file-manager D-03）；配置文件是唯一事实源，面板外修改自动反映（PRD N2）；单机站点数十量级、单文件 KB 级，实时扫描成本可接受。**关闭 prd-site-mgmt.md 待对齐-2 的触发时机部分**（列表刷新=按需拉取，呈现节奏 D4 定） |
| D-02 | **站点标识 siteId=`{来源文件绝对路径}#{server 块序号}`**（序号 0 起，按文件内出现顺序；如 `/etc/nginx/sites-enabled/app.conf#0`）：跨扫描稳定寻址、写回落点与证书部署引用的键基础；外部重排文件导致标识变化的边界由投影真实性兜底（列表如实反映实况，PRD N2 已声明） | 确定性可推导（无需持久化身份映射，零站点台账自洽）；同文件多 server 块用序号区分。**关闭 prd-site-mgmt.md 待对齐-1** |
| D-03 | **多配置目录消重与新建落点**：同一 server 块多目录并存时按优先级 sites-enabled > conf.d > vhost.d 登记唯一写回落点（原文件）；重复定义在 list 响应 duplicates[] 如实呈现（不静默丢弃）；**新建站点文件落点同优先级**（首个存在的可写目录），文件名 `dougong-{主域名规范化}.conf` | ⚠️A13 消重口径的落法（Debian 惯例 sites-enabled 为启用态权威）；接管不破坏=不删不改存量重复定义，只消重写回落点。**关闭 prd-site-mgmt.md 待对齐-2 的重复定义呈现部分** |
| D-04 | **安全写入管线 WritePipeline 为全部配置写接口的强制前置**（site/update、proxy/update、source-save、create、enable、disable、cert/deploy、cert/undeploy、cert/delete 的引用解除）：备份（conf.backup/时间戳/）→ 修改 → `nginx -t` → 失败自动恢复写前备份并报错 diff（SITE_003，data.errors[]+data.diff）；快照每来源文件保留最近 20 份（CONF_BACKUP_KEEP，写入流程尾部清理最旧） | R-06 四步 + R-16 隔离写回的唯一放行口径（无管线旁路，I-11）；「改不坏」机制承诺。**关闭 prd-site-mgmt.md 待对齐-10** |
| D-05 | **`nginx -t` 通过但 reload 失败=保留配置+报错**（SITE_004，data.output），不自动回滚 | reload 失败多为环境性原因（端口占用/权限/二进制异常），配置本身已通过语法校验；自动回滚会放大不确定性且与 R-06「校验失败回滚」语义冲突（校验失败=配置问题必须回滚，reload 失败=环境问题保留+如实呈现）。**关闭 prd-site-mgmt.md 待对齐-8** |
| D-06 | **源码编辑乐观锁**：site/source 响应带 baseMtime（来源文件 mtime），source-save 请求必须回传 baseMtime，服务端与当前文件 mtime 比对——不一致返回 SITE_010（data.currentMtime），操作者重新加载后编辑；基线一致时保存为最后写者胜 | 打开→保存窗口内面板外修改（shell/file-manager 通道）的冲突保护（PRD N2/待对齐-6）；结构化编辑（site/update/proxy/update）为字段级语义写回（重新序列化），无窗口漂移问题不需此锁。**关闭 prd-site-mgmt.md 待对齐-6 的接口部分**（交互呈现 D4 定） |
| D-07 | **日志尾部读取**：GET /sites/log/tail 参数 lines 默认 100、上限 1000（LOG_TAIL_MAX 编译期常量）；只读当前主日志文件（轮转归档不跟踪）；路径解析链=server 块 access_log/error_log 指令 → nginx 默认路径回退（⚠️A15）→ 仍缺失返回 SITE_011（如实提示，不虚构内容） | PRD FR-18/待对齐-7；日志尾部为排障入口非全文检索（PRD T4）。**关闭 prd-site-mgmt.md 待对齐-7** |
| D-08 | **证书状态读时惰性推算**：valid/expiring_soon/expired 由 not_after 与当前时间即时推算（CERT_EXPIRING_DAYS=30 编译期常量，ADR-024 阈值固定不可配）；pending/renew_failed 为落库态；**imported 证书元数据（issuer/not_before/not_after）读时重读源站证书文件刷新**（不落库过期值，防面板外替换漂移）；acme 证书元数据在签发/续期成功时落库更新 | account-auth D-03「计算态」同构先例（session 过期惰性判定）；证书数量少读时解析成本可忽略。**关闭 prd-site-mgmt.md 待对齐-3** |
| D-09 | **ACME 经 lego ACL**：ACME 协议交互（account/challenge/order）止步于 AcmeGateway（lego 封装，idea.md §5.3 选型），证书产物翻译为 certificate 对象字段；账户密钥存储=面板数据目录独立文件（`acme-account.key`，权限 0600，不入 SQLite、不经任何接口下发）；CA 目录 URL 为配置文件键 `acme.directory_url`（默认 Let's Encrypt 生产，可指向 staging，重启生效——account-auth D-10 配置文件先例）；**自动续期=每日调度对 expiring_soon 证书单次尝试**，失败转 renew_failed 等待手动重试（不自动无限重试）；签发/续期为同步阻塞调用（ACME_TIMEOUT=120s 上限，超时转 renew_failed） | R-09 除外条款的唯一外呼路径，防腐层防外部模型渗透（PRD 第九部分，本项目首个外部系统 ACL）；核心域不被外部概念污染。**关闭 prd-site-mgmt.md 待对齐-4** |
| D-10 | **同域名重复申请防重**：cert/request 前置校验同域名集合（SAN 全集）已有 pending/valid/expiring_soon 证书 → CERT_003 拒绝（data.existingCertId 提示复用部署）；renew_failed/expired 不阻塞重新申请（新证书行） | PRD FR-23/待对齐-5；防重复签发浪费 CA 速率配额。**关闭 prd-site-mgmt.md 待对齐-5** |
| D-11 | **部署引用关系实时推导、不落冗余列**：引用=扫描站点 ssl_cert 值与证书 storage_path（acme）/origin_path（imported）匹配；cert/delete 守卫与 cert/list 的 referencedSites 均按实时推导；部署时将站点原 ssl_cert 值快照入 certificate.deploy_prev_refs（JSON map siteId→原引用值），**cert/undeploy 恢复部署前原引用值**（接管不破坏：解除部署=回到面板接入前状态），多站点部署各自记录 | 引用关系从站点配置实况推导零漂移（D-01 同哲学）；恢复原值语义沿 PRD 待对齐-9 推导方向。**关闭 prd-site-mgmt.md 待对齐-9 的接口部分**（交互呈现 D4 定） |
| D-12 | **操作与审计事务边界**：配置文件写入类动作在文件系统侧（非 SQLite 事务域），完成后经 AuditService.Record 独立写入，失败重试一次、仍失败记面板运行日志兜底（file-manager D-09 同口径）；**例外**：certificate 表写（签发落库/续期更新/删除）与审计**同事务**（均在 SQLite，account-auth I-05 口径） | account-auth I-05「同事务」前提是变更落 SQLite；文件系统动作不可回滚，如实降级并显式声明 |
| D-13 | 混合模式（MIXED）声明：全部对象实际为自管（无外部数据同步源；ACME 为用户主动动作的外呼通道非数据源，R-09），不设 manage_mode 字段，权限管控按角色区分（ADR-018） | 分册 Step 2 默认混合模式；本模块无托管侧 |
| D-14 | **PHP-FPM pool 候选清单每次请求实时读取**（解析本机 php-fpm 配置的 pool 名与 socket），无缓存、不落库；不做 pool 管理（OUT-7） | 候选清单为下拉选项过程数据（P4，PRD 第九部分）；file-manager D-14 同口径。 |
| D-15 | **新建站点防重**：create 前置校验同「主域名+监听端口」的 server 块已存在（扫描实况）→ SITE_005 拒绝（data.existingSiteId）；nginx 未检出/不可用时全部接口前置拒绝 SITE_012（空态引导，PRD FR-04 本域推导） | 防同端口冲突导致 nginx -t 失败循环；nginx 未检出时无任何可操作对象（R-15 发现哲学的自然边界） |
| D-16 | **站点启停实现=来源文件内该 server 块整体注释/还原**（WritePipeline 原子替换，序列化保真由 round-trip 样本集约束）：enabled 状态即「server 块是否生效」的配置侧投影，无独立状态存储；停用保留全部配置与证书引用（停用≠删除，OUT-6） | kb 状态机「enabled ⇄ disabled 经 R-06→reload」的落法；不引入 include 治理（PRD OUT-9 接管不改造） |

### 1.4 领域对象总览

| 聚合根 | 域内对象/值对象 | 子流程 | 关联领域对象 |
|--------|---------------|--------|-------------|
| certificate（主数据类子形态：数据生命周期状态机，非流程流转） | site（实体，配置文件实时投影不落库）、反代上游 upstream（值对象，site 内嵌）、Raw 未识别指令块（值对象，site 内嵌）、写回落点映射（值对象，随 site 派生） | 自动续期任务（每日调度，4.1.3） | 无（ACME CA 为外部系统经 ACL 对接非业务域聚合引用；证书⇄站点为域内引用实时推导，D-11） |

无领域事件与 MQ（单机单体 R-09；审计为进程内 OHS 出站调用，D-12）。

**功能类型判定**（分册 Step 1）：聚合根 certificate 存在、由本模块管理且有写操作（签发/部署/解除/续期/删除）、状态为**数据生命周期**（cert_status 六状态机 ADR-024，非流程流转、无审批）→ **主数据类子形态**；模块整体为「主数据型聚合根（certificate）+ 配置投影实体（site）+ 工具类运维动作集合」的混合形态——无 CRUD+options 对外服务（消费终模块，1.5）、无审批子流程（pending 为外部 CA 异步签发非审批）。据此裁剪：第 2 章不适用（单聚合根 certificate）、4.1.2 按 cert_status 状态机填写、4.2 按领域对象组织（site 无面板侧状态机，enabled 为配置侧投影，service-mgmt/file-manager 同构处理）、第 5 章必填（有 DDL）。

### 1.5 上下文映射

| 上下文 | 关系 | 机制 |
|--------|------|------|
| account-auth | 遵奉者 + OHS 消费 | 全部接口位于 Cookie 会话后（R-02，AuthRequired）；站点列表/详情/证书清单/日志尾部两角色同等（D0 口径）、全部变更 admin 专属（ADR-018，RoleRequired + AUTH_008 边界拒绝）；写接口 CSRF（R-02，CsrfGuard）；审计写入 AuditService.Record（R-03，D-12 事务边界） |
| panel-core | 消费其 HTTP 骨架 | `/sites` 路由组挂载；入口面/外壳归 panel-core（R-10） |
| ACME CA（Let's Encrypt 等） | 外部系统，**ACL（防腐层）** | 本项目首个真实外部系统 ACL：ACME 协议交互经 AcmeGateway（lego 封装）翻译为 certificate 对象，外部概念不渗透领域模型；R-09 除外条款的唯一外呼路径（用户主动申请/既有证书续期调度）；D-09 账户密钥与 CA 配置 |
| nginx / php-fpm（本机系统组件） | 数据源与动作通道 | nginxx 适配器（ADR-010 自研核心资产）承载指令树解析/序列化与 `nginx -t`/reload 通道（NginxGateway，4.2.5）；php-fpm pool 实况读取（PhpFpmGateway，D-14）；本机资源非外部业务系统对接，无 ACL 必要 |
| 前端网站管理页 | 唯一消费方 | 仅经 18 个 HTTP 接口消费（5.1）；cron-backup 等后续模块如引用站点对象另行在对应模块 D2 定义（PRD T1a） |

> 跨聚合根设计（第 2 章）：不适用——本模块单聚合根（certificate），无跨聚合根约束/编排（裁剪矩阵第 2 章「仅多聚合根模块」）；site/upstream/Raw 块/写回落点映射为无守门域内对象（domain-model.md 第一部分判定），证书⇄站点域内引用关系以不变量 I-05（部署引用一致性）与实时推导（D-11）表达，对象级约束列于 4.1/4.2。

## 3. 应用/基础设施层设计

### 3.1 接口规范

| 项 | 规范 |
|----|------|
| 路径格式 | `/sites/{resource}/{action}` 三段式；资源单数（site/proxy/log/nginx/cert/php-pools） |
| HTTP 方法 | 读=GET，写=POST（面板内部契约，account-auth D-07 同一约定） |
| 响应格式 | `{"code": 0, "message": "ok", "data": {...}}`；code=0 成功，非 0 见 3.4 错误码（与 account-auth/file-manager 同一契约） |
| 时间格式 | ISO-8601 UTC（公共约定） |
| 认证 | 复用 account-auth：HttpOnly Cookie `dg_session`（AuthRequired） |
| CSRF | 全部 POST 动作须带 `X-CSRF-Token` 头（R-02，CsrfGuard）；GET 豁免 |
| 中间件链 | AuthRequired → CsrfGuard（POST）→ RoleRequired（按接口，3.3 矩阵）；安全写入管线（WritePipeline）与 nginx 可用性预检为服务层编排（4.2），非中间件 |
| 用户类型标注 | 查看接口（site/list、site/detail、cert/list、log/tail）：admin+readonly（D0 口径 + ADR-018 全可看）；全部写接口与编辑前置读取（site/source）、建站候选（php-pools/list）：admin（readonly 无界面入口 + 越界 AUTH_008 全局边界拒绝） |

### 3.2 依赖与约束

| 依赖 | 类型 | 约束 |
|------|------|------|
| account-auth 中间件 | 框架 | AuthRequired/CsrfGuard/RoleRequired 注册顺序见 3.1；不得自建会话解析与角色判定 |
| account-auth AuditService.Record | OHS | 操作审计经其写入（R-03）；文件系统侧动作失败重试一次后日志兜底（D-12），不得自建审计表 |
| nginxx 适配器 + nginx 二进制 | 数据源与动作通道 | 指令树解析/序列化（R-07 round-trip，样本集 ≥ 20 验收）、`nginx -t` 校验与 reload 执行；全部配置写入经 WritePipeline（D-04，无旁路）；通道错误归一 SITE_003/SITE_004/SITE_012 语义 |
| lego ACME 客户端（ACL） | 外部系统通道 | 仅 AcmeGateway 可触网（R-09 唯一例外）；账户密钥文件权限 0600（D-09）；超时 ACME_TIMEOUT=120s |
| SQLite | 存储 | certificate 单表（5.3）；单连接写串行；certificate 表写与审计同事务（D-12 例外） |
| panel-core | 框架 | 路由组挂载 `/sites`；不得越界承载业务逻辑 |

### 3.3 权限模型

角色：admin / readonly（panel_role 固定枚举）；本模块无访客可达面（全部位于登录会话后）。

**接口权限矩阵**：

| 接口 | admin | readonly | CSRF |
|------|-------|----------|------|
| GET /sites/site/list | ✓ | ✓（D0 口径 + ADR-018 全可看） | —（GET） |
| GET /sites/site/detail | ✓ | ✓（同上） | —（GET） |
| POST /sites/site/create | ✓ | ✗（ADR-018 禁变更） | ✓ |
| POST /sites/site/update | ✓ | ✗（同上） | ✓ |
| POST /sites/proxy/update | ✓ | ✗（同上） | ✓ |
| GET /sites/site/source | ✓ | ✗（编辑前置读取，ADR-018 禁变更，file-manager content 同口径） | —（GET 只读） |
| POST /sites/site/source-save | ✓ | ✗（ADR-018 禁变更） | ✓ |
| POST /sites/site/enable / disable | ✓ | ✗（同上） | ✓ |
| GET /sites/log/tail | ✓ | ✓（D0 口径：readonly 查看日志尾部） | —（GET） |
| POST /sites/nginx/reload | ✓ | ✗（ADR-018 禁变更） | ✓ |
| GET /sites/php-pools/list | ✓ | ✗（建站候选数据源，随写接口 admin，D-14） | —（GET） |
| GET /sites/cert/list | ✓ | ✓（ADR-018 全可看） | —（GET） |
| POST /sites/cert/request | ✓ | ✗（ADR-018 禁变更） | ✓ |
| POST /sites/cert/deploy / undeploy | ✓ | ✗（同上） | ✓ |
| POST /sites/cert/retry-renewal | ✓ | ✗（同上） | ✓ |
| POST /sites/cert/delete | ✓ | ✗（同上） | ✓ |

数据权限规则：无行级隔离——站点列表/详情/证书清单/日志尾部两角色可见集完全一致（含 Raw 块与来源文件路径呈现）；角色差异仅体现在写接口与编辑前置读取。readonly 越界由全局中间件 RoleRequired 拦截返回 AUTH_008（附边界原因），接口层不自建权限分支。

### 3.4 公共规范

**错误码**（SITE_/CERT_ 前缀为本模块新增 21 个；鉴权/CSRF/越界复用 account-auth 码族）：

| code | 标识 | HTTP 语义 | 说明 |
|------|------|----------|------|
| 0 | OK | 成功 | — |
| SITE_001 | INVALID_SITE_ID | 参数错误 | siteId 形态非法（非「文件路径#序号」结构，R-14） |
| SITE_002 | SITE_NOT_FOUND | 拒绝 | 站点不存在（扫描实况校验：文件已删/server 块已移除/序号越界，D-01） |
| SITE_003 | NGINX_TEST_FAILED | 操作失败 | `nginx -t` 校验失败：**写回已自动回滚**（R-06，配置回到写前原样）；data.errors[]（nginx 错误行）+ data.diff（本次写回内容与原文件的差异，供排障） |
| SITE_004 | RELOAD_FAILED | 操作失败 | reload 失败：**配置保留**（D-05，nginx -t 已通过）；data.output（nginx 输出），提示检查环境后手动重试 |
| SITE_005 | SITE_ALREADY_EXISTS | 参数冲突 | 同主域名+监听端口的 server 块已存在（D-15，data.existingSiteId） |
| SITE_006 | INVALID_LISTEN | 参数错误 | 监听端口非法（非 1~65535 / 与地址组合形态非法，R-14） |
| SITE_007 | INVALID_DOMAIN | 参数错误 | server_name 形态非法（空/含空格/通配符越界，R-14） |
| SITE_008 | INVALID_UPSTREAM | 参数错误 | 反代上游地址非法（非 http(s)://host[:port] 形态，R-14） |
| SITE_009 | POOL_NOT_FOUND | 参数错误 | 指定 php-fpm pool 不存在（实况校验，D-14） |
| SITE_010 | SOURCE_STALE | 待决策 | 源码编辑基线过期：文件在打开后被外部修改（D-06）；data.currentMtime，重新加载后编辑 |
| SITE_011 | LOG_NOT_FOUND | 拒绝 | 日志文件不存在/不可读（⚠️A15 回退链穷尽，D-07） |
| SITE_012 | NGINX_UNAVAILABLE | 拒绝 | nginx 未检出或 `nginx -t` 通道不可用（D-15 空态；data.reason） |
| SITE_013 | WRITE_BACKUP_FAILED | 操作失败 | 写前快照失败：本次写入整体不执行（FR-01，配置未动），data.reason |
| SITE_014 | INVALID_SOURCE | 参数错误 | 源码内容非法：超编辑大小上限 1MiB 或非 UTF-8（R-14，沿 file-manager D-05 上限先例） |
| CERT_001 | CERT_NOT_FOUND | 拒绝 | 证书不存在（certificate 表实况） |
| CERT_002 | INVALID_CERT_STATE | 拒绝 | 证书状态不允许该操作（部署/续期重试要求 valid/expiring_soon/renew_failed 等，data.currentStatus） |
| CERT_003 | CERT_DUPLICATE_DOMAIN | 参数冲突 | 同域名集合已有 pending/valid/expiring_soon 证书（D-10，data.existingCertId） |
| CERT_004 | ACME_FAILED | 操作失败 | ACME 签发/续期失败（挑战不通/CA 拒绝/超时）；证书落 renew_failed，data.reason（ADR-024 状态机） |
| CERT_005 | CERT_REFERENCED | 拒绝 | 证书仍被站点引用，先解除部署（D-11 实时推导守卫）；data.sites[]（引用站点清单含 siteId/serverNames） |
| CERT_006 | UNDEPLOY_NOT_DEPLOYED | 参数冲突 | 该站点当前未部署此证书（I-05 引用一致性校验） |
| CERT_007 | INVALID_CHALLENGE | 参数错误 | challenge 不在 acme_challenge 取值内（R-14） |
| AUTH_001/002 | （复用） | 401 | 未登录/会话过期——中间件归 account-auth |
| AUTH_003 | （复用） | 403 | CSRF 校验失败（POST 动作） |
| AUTH_008 | （复用）FORBIDDEN | 拒绝 | readonly 越界，附边界原因（ADR-018） |

**枚举**（均在 workspace/01-solution/04-business-enum.md 定义；site_type/site_enabled/cert_source/acme_challenge/cert_status/site_log_type 为本模块 D2 定稿，本次为引用无新增）：

| 枚举标识 | 取值 | 用途 |
|---------|------|------|
| site_type | static \| php_fpm \| reverse_proxy | 新建站点模板类型（site/create 请求 type，FR-14~16） |
| site_enabled | enabled \| disabled | 站点启停状态（site/list 响应 enabled；投影自配置侧状态机，FR-17） |
| cert_source | acme \| imported | 证书来源（cert/list 筛选与响应；FR-20~22/25） |
| acme_challenge | http_01 \| tls_alpn | ACME 挑战方式（cert/request 请求 challenge；续期复用原值，FR-20~21/23） |
| cert_status | pending \| valid \| expiring_soon \| expired \| renew_failed | 证书状态（读时惰性推算，D-08；六状态机 ADR-024，4.1.2） |
| site_log_type | access \| error | 站点日志类型（log/tail 请求 type，FR-18/D-07） |
| panel_role | admin \| readonly | 权限模型角色（引用，account-auth 定稿） |
| audit_action / audit_result | change \| security / success \| failure | 操作审计类别与结果（引用） |

**业务常量**（审计细粒度动作码，非枚举池条目）：`site_created / site_updated / proxy_updated / site_source_saved / site_enabled / site_disabled / nginx_reloaded / cert_requested / cert_deployed / cert_undeployed / cert_renewal_retried / cert_deleted / cert_renewed`（action 列存此码；category：变更类=change，cert_renewed 为 system 来源后台动作=change；object_type=`site`/`certificate`，object_id=siteId 或 certId）。

**业务常量**（操作参数与阈值，非枚举池条目）：

| 常量 | 值 | 说明 |
|------|-----|------|
| CERT_EXPIRING_DAYS | 30 | expiring_soon 推算阈值（ADR-024 拍板固定，不可配；D-08） |
| LOG_TAIL_DEFAULT / LOG_TAIL_MAX | 100 / 1000 | 日志尾部默认行数/上限（D-07） |
| ACME_TIMEOUT | 120s | 签发/续期同步等待上限，超时转 renew_failed（D-09） |
| CONF_BACKUP_KEEP | 20 | 每来源文件 conf.backup 快照保留份数（D-04） |
| SCAN_DIR_PRIORITY | sites-enabled > conf.d > vhost.d | 多目录消重与新建落点优先级（D-03） |
| SOURCE_SIZE_LIMIT | 1MiB | 源码编辑大小上限（SITE_014，file-manager D-05 同值先例） |
| NEW_SITE_FILE_PREFIX | `dougong-` | 新建站点配置文件名前缀（D-03） |

**配置项**（载体=配置文件，重启生效——account-auth D-10 先例；无表载体）：

| 配置 | 默认 | 可配置 | 说明 |
|------|------|--------|------|
| acme.directory_url | Let's Encrypt 生产 | ✓ | CA 目录 URL（D-09）；可指向 staging 测试环境；重启生效 |

## 4. 聚合根与领域对象详细设计

### 4.1 certificate（聚合根，主数据类子形态）

#### 4.1.1 领域模型

**表结构组合**：certificate 单表（本模块唯一持久化对象；部署引用关系不落冗余列，实时推导，D-11）。

**字段清单**：

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | 聚合根标识（certId） |
| primary_domain | TEXT | NOT NULL | 主域名（SAN 全集首个） |
| domains | TEXT | NOT NULL | SAN 域名列表（JSON 数组） |
| issuer | TEXT | NOT NULL DEFAULT '' | 签发者（imported 读时刷新呈现，D-08） |
| source | TEXT | NOT NULL CHECK(in acme,imported) | cert_source |
| status | TEXT | NOT NULL CHECK(in pending,valid,expiring_soon,expired,renew_failed) | cert_status（pending/renew_failed 为落库态，其余读时推算，D-08） |
| challenge | TEXT | NULL CHECK(in http_01,tls_alpn) | acme_challenge（acme 来源必填；续期复用） |
| storage_path | TEXT | NULL | 面板托管路径（acme 来源；⚠️A16 建议值 /etc/dougong/certs/ 下，路径常量归后端工程） |
| origin_path | TEXT | NULL | 源站原路径（imported 来源；⚠️A16 不迁移） |
| not_before / not_after | DATETIME | NULL | 有效期（acme 落库；imported 读时从文件解析即时呈现不落库，D-08） |
| deploy_prev_refs | TEXT | NULL | 部署前引用快照 JSON map（siteId→原 ssl_cert 值；undeploy 恢复用，D-11） |
| last_error | TEXT | NULL | renew_failed 失败原因（排障呈现，FR-24） |
| acme_account_ref | TEXT | NULL | ACME 账户标识（D-09；密钥本体在文件，不入库） |
| created_at / updated_at | DATETIME | NOT NULL | ISO-8601 UTC |

**不变量**（聚合内）：

| 编号 | 不变量 | 违反处理 |
|------|--------|---------|
| I-01 | source/challenge 组装约束：acme → challenge 必填 + storage_path 必填 + origin_path 禁填；imported → origin_path 必填 + challenge/storage_path 禁填 | 写入侧校验，CERT_002 语义 |
| I-02 | 删除守卫：引用站点集（实时推导）非空时禁止删除 | CERT_005（data.sites[]，D-11） |
| I-03 | 同域名集合唯一性：SAN 全集相同的 active 证书（pending/valid/expiring_soon）至多一张 | CERT_003（D-10） |
| I-04 | 托管文件归属：acme 证书文件由面板全权管理（签发/续期原路径替换、删除时移除）；imported 证书永不改动源站原文件（⚠️A16） | 写侧编排 |
| I-05 | 部署引用一致性：deploy 后目标站点 ssl_cert=storage_path 且 deploy_prev_refs 记录原值；undeploy 恢复原值并清除该站点键；实时推导引用与站点配置实况恒一致 | 部署/解除编排（CERT_006） |

**领域服务**：CertStatusCalculator（D-08 读时推算 + imported 元数据刷新）、CertReferencedResolver（D-11 引用实时推导）、AcmeGateway（ACL，lego 封装，4.1.5）、RenewalScheduler（子流程，4.1.3）。

#### 4.1.2 状态模型

cert_status 六状态机（ADR-024；**枚举命名 5 个状态即 ADR-024 全部命名状态，ADR-024 文本「6 状态」计数差异已在 prd-site-mgmt.md 第六部分登记**）：

| 当前状态 | 目标状态 | 触发 | 接口 |
|---------|---------|------|------|
| pending | valid | ACME 签发成功 | cert/request（同步返回终态，D-09） |
| pending | renew_failed | 签发失败（挑战不通/CA 拒绝/超时） | cert/request；data.reason=CERT_004 |
| valid | expiring_soon | 到期前 30 天（读时推算，非落库迁移） | 自动（CERT_EXPIRING_DAYS） |
| expiring_soon | valid | 续期成功（原路径替换+重部署引用站点） | 自动续期任务（4.1.3）/ cert/retry-renewal |
| expiring_soon | renew_failed | 续期失败 | 自动续期任务；last_error 落库 |
| expiring_soon | expired | 到期仍未续期成功 | 读时推算 |
| renew_failed | valid | 手动重试成功 | cert/retry-renewal |
| renew_failed | expired | 到期仍未成功 | 读时推算 |

imported 证书以 valid 态入册（存量识别导入机制，4.2 存量导入链路），无续期通道，沿 valid → expiring_soon → expired 自然流转。无审批/驳回环节（pending 为外部 CA 异步签发非审批）；状态变迁历史经 audit_log（cert_requested/cert_renewed 等动作码）。

#### 4.1.3 子流程设计——自动续期任务

| 项 | 契约 |
|----|------|
| 触发 | 每日后台调度（进程内 cron）；对读时推算为 expiring_soon 的 acme 来源证书逐张发起 |
| 输入 | certificate 行（domains、challenge、storage_path、acme_account_ref） |
| 处理 | AcmeGateway 续期（同步，ACME_TIMEOUT）→ 成功：托管文件原路径替换（I-04）→ 对 deploy_prev_refs 中全部站点重执行部署写回（WritePipeline）→ status=valid 落库；失败：status=renew_failed + last_error 落库 |
| 输出 | 写入 certificate 聚合根（status/last_error/有效期字段）；成功/失败均写审计 cert_renewed（change，source=system，D-12 同事务） |
| 失败处理 | 单证书失败不影响其余证书；不自动重试（等待次日调度或手动 cert/retry-renewal，D-09） |

#### 4.1.4 接口设计

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /sites/cert/list | GET | admin+readonly | 证书清单（来源/状态筛选；读时推算状态，D-08） |
| /sites/cert/request | POST | admin | ACME 申请（challenge 参数覆盖 HTTP-01/TLS-ALPN 两菜单；同步终态，D-09/D-10） |
| /sites/cert/deploy | POST | admin | 部署到站点（写回 ssl_cert 引用，经 WritePipeline + reload） |
| /sites/cert/undeploy | POST | admin | 解除部署（恢复部署前原引用值，D-11） |
| /sites/cert/retry-renewal | POST | admin | renew_failed 手动重试（进入 4.1.3 同一通道） |
| /sites/cert/delete | POST | admin | 删除（引用守卫 CERT_005；acme 连带托管文件，imported 仅删行，I-04） |

**接口详情**（全部写接口隐含 CSRF 与 AUTH_008 角色边界；涉及站点配置写回的 deploy/undeploy 隐含 WritePipeline → SITE_003/SITE_004 与 nginx 可用性 SITE_012 前置，不再逐条重复）：

- **GET /sites/cert/list**：query `{source?(cert_source), status?(cert_status)}`。处理：读全表 → CertStatusCalculator 逐行推算（D-08；imported 元数据重读文件刷新）→ CertReferencedResolver 推导 referencedSites → 筛选排序（not_after 升序，临期在前）。响应 data=`{items:[{id, primaryDomain, domains[], issuer, source, status, challenge?, notBefore?, notAfter?, originPath?, lastError?, referencedSites:[{siteId, serverNames[]}], createdAt}], generatedAt}`（storage_path/acme_account_ref 不下发——内部实现细节，载荷与安全考虑）。审计：—（查看类）。
- **POST /sites/cert/request**：req `{siteId, challenge}`。校验：siteId 解析+实况存在（SITE_001/002）；challenge ∈ acme_challenge（CERT_007）；I-03 同域名防重（CERT_003，data.existingCertId）；nginx 可用（SITE_012——HTTP-01 需 80 通路/TLS-ALPN 需 443 通路，经既有站点配置）。执行：INSERT certificate（status=pending，challenge，domains=站点 serverNames）→ AcmeGateway 签发（同步 ≤120s）→ 成功：写托管文件 + status=valid + not_before/not_after/issuer 落库；失败：status=renew_failed + last_error。响应：成功 data=`{certId, status:"valid", notAfter}`；失败 CERT_004（data.reason）。审计：cert_requested（change，detail={challenge, result}；同事务，D-12 例外）。
- **POST /sites/cert/deploy**：req `{certId, siteId}`。校验：证书存在（CERT_001）且状态推算 ∈ {valid, expiring_soon}（CERT_002，data.currentStatus）；站点存在（SITE_002）。执行：写前快照原 ssl_cert 值入 deploy_prev_refs[siteId] → WritePipeline 将站点 ssl_cert 改写为 storage_path（R-06 四步）→ reload（D-05）。响应 data=`{certId, siteId, certRef}`（新引用值）。审计：cert_deployed（change）。
- **POST /sites/cert/undeploy**：req `{certId, siteId}`。校验：证书/站点存在；I-05 该站点在 deploy_prev_refs 中（否则 CERT_006）。执行：WritePipeline 将站点 ssl_cert 恢复为 deploy_prev_refs[siteId] 原值 → 清除该键 → reload。响应 data=`{certId, siteId, certRef}`（恢复后的原引用值）。审计：cert_undeployed（change）。
- **POST /sites/cert/retry-renewal**：req `{certId}`。校验：证书存在（CERT_001）且 status=renew_failed（CERT_002，valid/expiring_soon 无需重试、pending 在途、expired 需重新 request）。执行：进入 4.1.3 同一续期通道（同步返回终态）。响应：成功 data=`{certId, status:"valid", notAfter}`；失败 CERT_004。审计：cert_renewal_retried（change；结果一并 detail）。
- **POST /sites/cert/delete**：req `{certId}`。校验：证书存在（CERT_001）；I-02 引用守卫——CertReferencedResolver 非空 → CERT_005（data.sites[]，先 undeploy）。执行：acme → 删行 + 删托管文件（I-04，同一事务语义为「删行成功后删文件，文件删除失败记面板日志兜底」）；imported → 仅删行（源站原文件不动，⚠️A16）。响应 data=`{certId}`。审计：cert_deleted（change，detail={source}；同事务，D-12 例外）。

**跨接口复用规则**：状态推算统一 CertStatusCalculator（D-08）；引用推导统一 CertReferencedResolver（D-11，deploy/undeploy/delete/list 共用）；签发/续期统一 AcmeGateway 通道（D-09，request 与 4.1.3/retry 共用）；站点配置写回统一 WritePipeline（D-04，4.2）；审计统一 AuditService.Record（D-12）。

**幂等语义**：deploy 重复提交=幂等（引用已为目标值时直接成功）；undeploy 第二次提交报 CERT_006（如实）；delete 重复提交报 CERT_001（如实）；request 重复提交经 I-03 防重返回 CERT_003（如实，指向已有证书）；retry-renewal 在非 renew_failed 态报 CERT_002（如实）。防重复提交为前端在途态（P3 过程数据），非服务端一致性手段。

#### 4.1.5 领域事件与集成

无 MQ、无领域事件。仓储契约：`CertificateRepository { Create, GetByID, List, UpdateStatus, UpdateRenewalMeta, SetDeployPrevRef, ClearDeployPrevRef, Delete }`。关联域调用（出站）：AuditService.Record（account-auth OHS，R-03，D-12）；**AcmeGateway（ACL 出站，唯一触网通道）**：

```
AcmeGateway（lego 封装，外部 ACME CA 概念止步于此——ACL，D-09）{
  Issue(domains, challenge, accountRef, timeout) (certPEM, keyPEM, notBefore, notAfter, issuer, error)
  Renew(certMeta, accountRef, timeout) (certPEM, keyPEM, notBefore, notAfter, error)  // 复用原 challenge
}
约束：唯一网络出站口（R-09 除外条款）；账户密钥文件读写收敛于此（0600）；CA 目录取配置键 acme.directory_url；
     ACME order/challenge/authorization 概念不进入面板领域模型——出参仅证书产物与有效期元数据
```

### 4.2 site（域内对象-实体，配置文件投影链路）

#### 4.2.1 领域模型

**表结构组合**：无表（D2 判定零站点台账，配置文件是唯一事实源）。site 驻留进程内，为每次请求即时扫描解析的投影条目（D-01）；全部写动作经 WritePipeline 直达来源文件，不经投影对象中转持久化。

**投影条目字段清单**（site/list 与 site/detail 响应的共同数据源）：

| 字段 | 类型 | 说明 |
|------|------|------|
| siteId | string | `{来源文件绝对路径}#{server 块序号}`（寻址标识，D-02） |
| serverNames | string[] | server_name 列表（首个为主域名） |
| listen | string[] | 监听声明列表（如 `80`、`443 ssl`） |
| root | string | 根目录 |
| enabled | enum(site_enabled) | 启停投影（server 块生效态，D-16） |
| certRef | string \| null | ssl_cert 引用值（无证书引用为 null） |
| certId / certStatus | integer / enum(cert_status) \| null | 匹配到的纳管证书及其推算状态（实时推导，D-11；未纳管引用为 null） |
| proxy | {upstream, hostHeader, websocket} \| null | 主反代段参数（upstream 值对象；非反代站点为 null；多反代 location 仅呈现首个，全量经源码编辑，1.3 D-03 优先级口径） |
| locations | {path, proxyPass?, fastcgiPass?}[] | location 清单（解析投影） |
| rawBlocks | {line, text}[] | Raw 未识别指令块原文（R-07） |
| sourceFile | string | 来源文件路径（写回落点映射呈现，⚠️A13） |

**不变量**：

| 编号 | 不变量 | 违反处理 |
|------|--------|---------|
| I-06 | 投影真实性：list/detail 恒为当次扫描解析快照，无缓存层、无陈旧回退（D-01）；generatedAt 标注投影时刻；nginx 未检出返回空列表 + nginxAvailable:false（SITE_012 语义空态，非错误） | 读取侧保证（每请求即时扫描） |
| I-07 | 零持久化：site 不落库，面板重启无站点台账；操作历史经 audit_log（site_* 动作码） | 结构保证（无表） |
| I-08 | 寻址前置：任何携带 siteId 的接口先解析（SITE_001）→ 扫描实况存在性校验（SITE_002）→ 执行（D-02） | 统一前置链 |
| I-09 | Raw 保全：全部结构化写回重新序列化时，Raw 未识别指令块按原文本输出、零丢失（R-07）；完整性由 round-trip 样本集 ≥ 20 验收兜底 | 写侧编排（NginxGateway 序列化） |
| I-10 | 隔离写回：写回仅作用目标站点来源文件，其余站点 byte-level 或语义级零变化（R-16，round-trip 保真前提） | WritePipeline 保证 |
| I-11 | 管线强制：全部配置写入（含 cert/deploy、cert/undeploy 的引用写回）经 WritePipeline 四步（备份→修改→`nginx -t`→失败回滚+diff），无旁路（R-06）；快照失败 SITE_013 写入整体不执行 | WritePipeline 统一编排（D-04） |
| I-12 | nginx 可用性：写接口与 reload 前置 `nginx -t` 通道可用性校验，nginx 未检出/不可用拒绝 SITE_012（D-15；GET 类接口不前置——list 空态即可用性信号） | 接口前置校验 |

**领域服务**：SiteScanner（D-01 实时扫描 + D-03 消重/落点 + CertReferencedResolver 数据源）、SiteProjector（投影组装 + 筛选排序）、SiteEditService（结构化字段/反代参数语义写回编排）、SourceEditService（源码读取 + baseMtime 乐观锁保存，D-06）、SiteCreateService（三模板生成 + D-15 防重）、SiteToggleService（启停注释/还原编排，D-16）、LogTailService（⚠️A15 路径解析链 + 尾部读取，D-07）、WritePipeline（R-06 四步 + R-16 隔离 + 快照保留清理，D-04）、NginxReloader（reload 通道 + D-05 语义）、PhpFpmGateway（pool 实况读取，D-14）。

#### 4.2.2 状态模型

面板侧无自有状态机（I-07 零持久化投影，PRD 第六部分辨析）；site_enabled 为**配置侧状态投影**（server 块生效态，kb 拍板 enabled ⇄ disabled 经 R-06 → reload；D-16 注释/还原实现）——切换动作经 site/enable、site/disable 即时落盘生效，无面板侧独立状态存储。无审批/驳回环节；无状态历史表（操作追溯经 audit_log；配置原样历史=conf.backup 快照目录，D-04）。

#### 4.2.3 子流程设计

无（安全写入四步为同步机制链路，无独立生命周期状态——domain-model.md 第四部分声明；自动续期任务挂 certificate 聚合根，见 4.1.3）。

#### 4.2.4 接口设计

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /sites/site/list | GET | admin+readonly | 站点列表（enabled 筛选；实时投影 D-01；duplicates[] 重复定义呈现 D-03） |
| /sites/site/detail | GET | admin+readonly | 站点详情（locations/Raw 块/来源文件） |
| /sites/site/create | POST | admin | 三模板建站（type 参数覆盖 static/php_fpm/reverse_proxy 三菜单；D-15 防重） |
| /sites/site/update | POST | admin | 站点基本信息结构化编辑（serverNames/listen/root；I-09 Raw 保全） |
| /sites/proxy/update | POST | admin | 反代参数管理（主反代段上游/Host 头/WS 透传；M3 验收主场景） |
| /sites/site/source | GET | admin | 读取配置原文（Monaco 编辑打开；baseMtime 基线，D-06） |
| /sites/site/source-save | POST | admin | 源码保存（baseMtime 校验 + 保存前自动 `nginx -t`，⚠️A17 对象限定站点配置文件） |
| /sites/site/enable | POST | admin | 启用站点（D-16 注释还原 + reload） |
| /sites/site/disable | POST | admin | 停用站点（D-16 整块注释 + reload；停用≠删除） |
| /sites/log/tail | GET | admin+readonly | 访问/错误日志尾部（site_log_type；⚠️A15 路径解析链，D-07） |
| /sites/nginx/reload | POST | admin | 用户主动平滑 reload（D-05 失败语义） |
| /sites/php-pools/list | GET | admin | php-fpm pool 候选清单（实时读取，D-14） |

**接口详情**（请求/响应/校验；全部写接口隐含 CSRF 与 AUTH_008 角色边界；携带 siteId 接口隐含 I-08 前置链（SITE_001/002）与写接口的 I-12/SITE_012、I-11 管线前置（SITE_003/SITE_004/SITE_013），不再逐条重复）：

- **GET /sites/site/list**：query `{enabled?(site_enabled)}`。处理：SiteScanner 即时扫描（D-01/D-03）→ SiteProjector 组装 + enabled 筛选 → 每站点 certRef 匹配纳管证书（D-11）。响应 data=`{nginxAvailable:boolean, items:[{siteId, serverNames, listen, root, enabled, certRef, certId?, certStatus?, proxy?, sourceFile}], duplicates:[{serverNames, listen, files[]}], generatedAt}`。审计：—（查看类）。
- **GET /sites/site/detail**：query `{siteId}`。响应 data=`{...list 项全字段, locations[], rawBlocks[], listen 全量声明}`。审计：—（查看类）。
- **POST /sites/site/create**：req `{type(site_type), serverNames[], listen, root?(type=static 必填), poolName?(type=php_fpm 必填), upstream?(type=reverse_proxy 必填), hostHeader?, websocket?}`。校验：type ∈ site_type；域名形态（SITE_007）/端口（SITE_006）/根目录存在性/上游形态（SITE_008）/pool 实况存在（SITE_009，D-14）；D-15 同主域名+端口防重（SITE_005，data.existingSiteId）。执行：按模板生成 server 块（php_fpm 含 FastCGI 参数模板内置 ⚠️A14；reverse_proxy 含 Host 头与 WS 透传默认值）→ 写入 D-03 落点文件（`dougong-{主域名}.conf`）→ WritePipeline + reload。响应 data=`{siteId, type, serverNames, listen, sourceFile}`。审计：site_created（change，detail={type}）。
- **POST /sites/site/update**：req `{siteId, serverNames[], listen[], root}`。校验：域名/端口/根目录（SITE_006/007，R-14）。执行：SiteEditService 字段级语义写回——重新序列化指令树（I-09 Raw 保全）→ WritePipeline（R-06 四步，I-10 隔离）→ reload。响应 data=`{siteId, serverNames, listen, root}`（新值）。审计：site_updated（change，detail={fields:[...]}）。
- **POST /sites/proxy/update**：req `{siteId, upstream, hostHeader?, websocket?}`。校验：站点存在反代段（无 proxy → SITE_002 语义拒绝）；upstream 形态（SITE_008）。执行：主反代段参数语义写回（I-09/I-10/I-11）→ reload。响应 data=`{siteId, proxy:{upstream, hostHeader, websocket}}`。审计：proxy_updated（change）。**M3 验收主场景**：本接口改写反代地址 → `nginx -t` 通过且站点生效、其余站点 byte/语义级无变化（R-16/D-04）。
- **GET /sites/site/source**：query `{siteId}`。校验：来源文件可读。响应 data=`{siteId, sourceFile, content, baseMtime, size}`（超 SOURCE_SIZE_LIMIT 拒绝 SITE_014，D-06）。审计：—（打开未变更）。
- **POST /sites/site/source-save**：req `{siteId, content, baseMtime}`。校验：content UTF-8 且 ≤1MiB（SITE_014）；baseMtime 与当前文件 mtime 一致（SITE_010，data.currentMtime——外部修改检测，D-06）。执行：解析校验（`nginx -t` 语义前置——直接写临时副本执行校验，不合法拒收 SITE_003 语义但不落盘）→ WritePipeline 落盘 → reload。响应 data=`{siteId, baseMtime}`（新基线）。审计：site_source_saved（change，detail={size}）。
- **POST /sites/site/enable** / **POST /sites/site/disable**：req `{siteId}`。执行：D-16 序列化注释/还原该 server 块 → WritePipeline → reload；重复启用（已 enabled）幂等成功（D-16/I-06 实况语义）。响应 data=`{siteId, enabled}`（新投影值）。审计：site_enabled / site_disabled（change）。
- **GET /sites/log/tail**：query `{siteId, type(site_log_type), lines?(默认 100，上限 1000)}`。处理：⚠️A15 路径解析链（server 块指令 → 默认路径回退）→ 仍缺失 SITE_011（D-07）；读取尾部 lines 行。响应 data=`{siteId, type, logPath, lines:[{ts?, text}], truncated}`（无时间戳行原样呈现）。审计：—（查看类）。
- **POST /sites/nginx/reload**：无 req。执行：nginx reload；成功 data=`{reloaded:true}`；失败 SITE_004（配置保留语义 D-05，data.output）。审计：nginx_reloaded（change；失败亦记，result=failure）。
- **GET /sites/php-pools/list**：实时解析本机 php-fpm pool（D-14）。响应 data=`{pools:[{name, socket}]}`；无 pool 返回空数组（FR-15 如实提示口径）。审计：—（查看类）。

**跨接口复用规则**：扫描与投影统一 SiteScanner/SiteProjector（D-01/D-03，list/detail/create 防重/引用推导共用同一扫描结果——单请求内一次扫描多次消费）；配置写回统一 WritePipeline（D-04/I-11，全部写接口与 cert/deploy/undeploy 共用）；日志路径解析统一 LogTailService（⚠️A15/D-07）；审计统一 AuditService.Record（D-12）。

**幂等语义**：enable/disable 对已处目标态的站点幂等成功（实况即真，I-06）；update/proxy/source-save 重复提交=重复一次语义写回（终态一致，幂等）；create 重复提交报 SITE_005（如实，D-15）；reload 幂等（重复重载无副作用）；nginx -t 失败的写回已自动回滚（SITE_003），重试前需修正内容（回滚语义非幂等重放，操作者知情后重发）。防重复提交为前端在途态（P3 过程数据），非服务端一致性手段。

#### 4.2.5 领域事件与集成

无 MQ、无领域事件。「仓储契约」两分：**SQLite 侧**——无（site 零持久化，I-07；certificate 仓储见 4.1.5）；**本机资源侧网关契约**（进程内适配层，非 SQL；nginxx 适配器承载，ADR-010）：

```
NginxGateway {
  ScanDirs(dirs) ([]ConfigFileSnapshot, error)        // 配置目录读取（D-01；文件清单+原文）
  Parse(source) (DirectiveTree, error)                // 词法解析 → 指令树（Raw 节点保留，R-07）
  ExtractSites(tree) ([]SiteSnapshot, error)          // server 块提取（D-02 定位）
  Serialize(tree) (source, error)                     // 序列化写回源码（I-09 Raw 保全）
  TestConfig() (ok bool, errors []string, error)      // nginx -t（R-06 第三步）
  Reload() (output string, error)                     // 平滑重载（D-05）
}
SiteScanner 组合 ScanDirs+Parse+ExtractSites 完成导入管线（FR-04~06）；
WritePipeline 组合 快照 Copy→Serialize 定点替换→TestConfig→失败 Restore（R-06 四步 + CONF_BACKUP_KEEP 清理，D-04）
PhpFpmGateway { ListPools() ([]{name, socket}, error) }   // pool 实况读取（D-14）
```

约束：WritePipeline 为全部配置写入唯一放行口径（I-11）；`nginx -t`/reload 经 nginx 二进制参数数组执行（R-08 全项目命令纪律）；round-trip 保真由样本集验收兜底（idea.md §6 M3）。关联域调用（出站）：AuditService.Record（account-auth OHS，R-03，D-12）；无其他出站；被调用契约：无（cron-backup 等后续模块如引用站点对象另行定义，1.5）。

## 5. 模块清单汇总

### 5.1 对外服务清单（HTTP 接口）

| 服务（接口） | 消费方 | 契约要点 |
|-------------|--------|---------|
| GET /sites/site/list · GET /sites/site/detail | 前端网站管理页（两角色） | 实时扫描投影（D-01/I-06）；duplicates[] 呈现（D-03）；证书状态列实时推导（D-11） |
| POST /sites/site/create | 前端网站管理页（admin） | 三模板（site_type）；D-15 防重；落点与消重同优先级（D-03） |
| POST /sites/site/update · POST /sites/proxy/update | 前端网站管理页（admin） | 结构化/反代参数语义写回（I-09 Raw 保全）；经 WritePipeline（D-04）；M3 验收主场景 |
| GET /sites/site/source · POST /sites/site/source-save | 前端网站管理页（admin） | Monaco 源码编辑；baseMtime 乐观锁（D-06）；保存前自动 `nginx -t` |
| POST /sites/site/{enable,disable} | 前端网站管理页（admin） | 启停投影状态机（D-16；停用≠删除）；幂等 |
| GET /sites/log/tail | 前端网站管理页（两角色） | site_log_type；⚠️A15 路径解析链（D-07）；100/1000 行 |
| POST /sites/nginx/reload | 前端网站管理页（admin） | 平滑 reload；失败保留配置+报错（D-05） |
| GET /sites/php-pools/list | 前端网站管理页（admin） | pool 候选实时读取（D-14） |
| GET /sites/cert/list | 前端网站管理页（两角色） | 状态读时推算（D-08）；引用站点推导（D-11） |
| POST /sites/cert/request | 前端网站管理页（admin） | ACME 签发（acme_challenge）；同步终态；D-10 防重；ACL 出站（D-09） |
| POST /sites/cert/{deploy,undeploy} | 前端网站管理页（admin） | 部署/恢复原引用（I-05/D-11）；经 WritePipeline |
| POST /sites/cert/retry-renewal | 前端网站管理页（admin） | renew_failed 手动重试（进入 4.1.3 通道） |
| POST /sites/cert/delete | 前端网站管理页（admin） | 引用守卫 CERT_005；acme 连带托管文件、imported 仅删行（I-04） |
| 自动续期任务（后台，非 HTTP） | —（系统调度） | 4.1.3 契约；结果写审计 cert_renewed（source=system） |
| 中间件 AuthRequired / CsrfGuard / RoleRequired | （复用） | account-auth 提供（3.1） |
| AuditService.Record | （出站调用） | 操作审计写入（R-03，D-12；certificate 侧同事务例外） |

本模块无对外 OHS 服务契约——消费终模块（1.5：cron-backup 等后续依赖另行定义）。

### 5.2 SQL 清单

| # | SQL | 场景 |
|---|-----|------|
| 1 | INSERT INTO certificate(...) | 签发落库（pending，同事务审计，D-12） |
| 2 | SELECT * FROM certificate WHERE id=? / 全表 | 详情/清单（D-08 推算输入） |
| 3 | SELECT * FROM certificate ORDER BY not_after ASC | 清单临期排序（cert/list） |
| 4 | UPDATE certificate SET status=?, last_error=?, not_before=?, not_after=?, issuer=?, updated_at=? WHERE id=? | 签发/续期终态落库（4.1.3/cert/request/retry-renewal） |
| 5 | UPDATE certificate SET deploy_prev_refs=? WHERE id=? | 部署原值快照 / undeploy 清键（D-11/I-05） |
| 6 | DELETE FROM certificate WHERE id=? | 删除（I-02 守卫后；同事务审计，D-12） |

### 5.3 DDL

```sql
CREATE TABLE IF NOT EXISTS certificate (
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  primary_domain   TEXT    NOT NULL,
  domains          TEXT    NOT NULL,            -- SAN 域名列表 JSON 数组
  issuer           TEXT    NOT NULL DEFAULT '',
  source           TEXT    NOT NULL CHECK (source IN ('acme','imported')),
  status           TEXT    NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','valid','expiring_soon','expired','renew_failed')),
  challenge        TEXT    CHECK (challenge IN ('http_01','tls_alpn')),          -- acme 来源必填（I-01，应用层校验）
  storage_path     TEXT,                                          -- acme 托管路径（⚠️A16）
  origin_path      TEXT,                                          -- imported 源站原路径（⚠️A16 不迁移）
  not_before       DATETIME,                                      -- acme 落库；imported 读时解析呈现（D-08）
  not_after        DATETIME,
  deploy_prev_refs TEXT,                                          -- 部署前引用快照 JSON map siteId→原 ssl_cert 值（D-11）
  last_error       TEXT,                                          -- renew_failed 失败原因（FR-24）
  acme_account_ref TEXT,                                          -- ACME 账户标识（D-09；密钥在文件不入库）
  created_at       DATETIME NOT NULL,
  updated_at       DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_cert_not_after ON certificate(not_after);
CREATE INDEX IF NOT EXISTS idx_cert_source_status ON certificate(source, status);
```

## 6. 更新记录

### 6.1 变更记录

| 版本 | 日期 | 变更类型 | 变更摘要 | 目标节号 |
|------|------|---------|---------|---------|
| v1.0 | 2026-11-12 | initial | 首次生成：18 个 HTTP 接口（/sites/{site,proxy,log,nginx,cert,php-pools}/*，读=GET/写=POST；查看类 site/list·site/detail·cert/list·log/tail 两角色（D0 口径），全部写与编辑前置读取/建站候选 admin+CSRF）+ 1 表 DDL（certificate 本模块唯一持久化对象，deploy_prev_refs JSON 列承载部署原值快照）+ 4 个网关/服务契约（NginxGateway+WritePipeline 安全写入管线、AcmeGateway ACL（本项目首个外部系统防腐层，R-09 唯一外呼）、SiteScanner 实时扫描、PhpFpmGateway）+ 1 个后台子流程（自动续期任务，ADR-024 六状态机）+ 21 错误码（SITE_001~014 + CERT_001~007）+ 复用 account-auth 码族（AUTH_001/002/003/008）+ 8 枚举引用（本域 6 项 D2 已定稿 + panel_role/audit_action/audit_result 引用，无新增）+ 13 审计动作常量 + 16 项设计决策（D-01 实时扫描投影、D-02 siteId 路径#序号、D-03 消重/落点优先级、D-04 WritePipeline 强制管线+快照保留 20 份、D-05 reload 失败保留配置、D-06 baseMtime 乐观锁、D-07 日志尾部 100/1000、D-08 状态读时推算+imported 刷新、D-09 ACME 账户文件/CA 配置键/每日单次调度、D-10 同域名防重、D-11 引用实时推导+undeploy 恢复原值、D-12 审计事务边界、D-13 混合模式声明、D-14 pool 实时读、D-15 建站防重+nginx 空态、D-16 启停注释/还原实现）；关闭 prd-site-mgmt.md 待对齐-1~-10；无旧版对比 | — |

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
