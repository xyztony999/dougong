# 斗拱面板 数据库管理（database-mgmt）API 设计

| 属性 | 值 |
|------|-----|
| 版本 | v1.1 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-12 |
| 输入 | prd-database-mgmt.md v1.0（完备性检查通过 G5/5·T4/4·N4/4）、domain-model.md v1.9、d0-clarification-database-mgmt.md、D1 三文档 v1.0 |
| 接口规约 | 无激活技术栈（ACTIVE_STACKS 为空）——按通用 REST/JSON 契约设计；认证=HttpOnly Cookie 会话 + CSRF token（R-02）；鉴权=RBAC 两角色边界矩阵（ADR-018）；与 api-account-auth.md 同一响应/错误码契约 |

## 1. 模块设计概述

### 1.1 业务背景及目标

数据库管理是斗拱面板 M4 里程碑模块：解决「自托管运维者如何用面板接管并治理服务器上已装的 MySQL/MariaDB 与 Redis 实例」的问题——本机探测发现即纳管（R-15，不要求重装）、凭据密文接管（⚠️A18 只进不出）、MySQL 库/用户/库级授权/root 密码治理、双引擎实况观测（状态概览/进程列表/info/慢日志）、mysqldump 备份导出导入（R-08 参数数组）、Redis key 只读浏览（MVP 严格只读）。M4 验收硬项：发现存量 MySQL → 建库/建用户/授权/改密码/备份导出再导入成功；Redis 状态与慢日志展示正确；所有操作可审计（idea.md §6 M4）。服务对象：管理员（admin，全部治理与数据动作）、只读用户（readonly，7 个查看接口）。

### 1.2 边界说明

| 项 | 说明 |
|----|------|
| 本模块做 | 实例探测纳管/凭据配置/移除（⚠️A20）、库与用户 CRUD 与库级授权、root/Redis 密码修改、状态概览/进程列表/info/慢日志观测、库级备份导出导入、key 只读浏览 |
| 本模块不做（接口边界） | ① PostgreSQL（二期，ADR-007）与 SQL 查询工作台/在线执行 SQL（不做，D0 OUT）；② Redis 数据编辑/删除 key/发布订阅（MVP 只读）；③ 实例进程启停/自启（service-mgmt，systemd 层）；④ 定时备份任务与轮转（cron-backup M8——其数据库备份源拟复用本模块 DbGateway.DumpDatabase，契约由该模块 D2/D3 定义）；⑤ 凭据任何形式的回显/导出（⚠️A18：无任何读取出口）；⑥ dbmysql/dbredis 适配器实现（后端工程，本模块经 DbGateway 网关契约消费，4.1.5） |
| 与存储的关系 | SQLite 单文件（ADR-011）；唯一持久化=实例档案合一表 db_instance（D-01，含凭据密文列 D-02）；database/db_user/key 等引擎内对象零台账不落库（domain-model v1.9 D2 判定） |

### 1.3 设计决策

| # | 决策 | 理由 |
|---|------|------|
| D-01 | **实例档案合一表 db_instance**（engine 列区分 mysql/mariadb/redis），不按聚合根分两表：identity_key 唯一标识（socket 模式 `engine\|socket:{path}`、tcp 模式 `engine\|tcp:{host}:{port}`，UNIQUE 列承载 D1 多实例消重口径） | 两引擎档案字段完全同构（标识/连接方式/版本/状态/凭据），分表无独立一致性需求；统一清单查询免 UNION；聚合边界在领域层（domain-model 两聚合根判定不变），不由表数决定——与 ADR-011 单文件轻量哲学一致。**落定 domain-model 存储映射「分表 vs 合一」待定项** |
| D-02 | **凭据密文=AES-256-GCM**（base64(nonce+ciphertext) 内嵌 cred_cipher 列）；**密钥=首次初始化时随机生成 32 字节，存面板数据目录独立文件 `db-cred.key`（权限 0600，不入 SQLite、不经任何接口下发）**；密钥轮换 MVP 不做（密钥文件随数据目录整体备份/迁移） | ⚠️A18 承接：密文落库、明文仅内存态建连（DbGateway 内解密）、不回显；密钥与密文分离存储（拖走 SQLite 文件不等于拿到凭据）；密钥文件先例=site-mgmt acme-account.key（D-09）。**关闭 prd 待对齐-1** |
| D-03 | **凭据配置=先验证后落库**：credentials 接口组装连接目标 → DbGateway 连接验证 → 认证失败 DBM_005 / 网络层失败 DBM_006（均不落库、不覆盖既有有效凭据）→ 成功才 UPSERT 密文并刷新 version 与 conn_state；连接目标纠正字段（connMode/socketPath/host/port 可选变更）触发 identity_key 重算，冲突返回 DBM_020 | PRD FR-02/D2 判定「验证失败不生效」（防坏凭据致实例失管）；凭据永不以未验证形态落库 |
| D-04 | **库/用户清单=每请求实时查询，无缓存无推送**：单机量级小（数十），DbGateway 实时读取，响应带 generatedAt；操作后由前端刷新 | PRD 待对齐-2 关闭；与 site-mgmt 投影真实性（I-06 每请求即时扫描）同构；GET 类观测接口对档案表零写副作用（readonly 调用不产生变更语义） |
| D-05 | **改密两步同步=同请求编排**：root-password/redis/password 先引擎侧改密 → 成功后同一请求 UPSERT 面板凭据密文；同步失败返回 DBM_019（引擎侧已改、不回滚，data.recoverHint 指向 credentials 重新配置恢复） | PRD FR-12/FR-19 与 N2「两步非原子」的接口落法：不虚构原子性，失败路径显式可恢复；Redis 修改密码时面板建连即旧密码验证（requirepass 语义天然满足「验证旧密码」） |
| D-06 | **备份导出=HTTP 响应文件流（不留存服务器目录）；导入=multipart 上传临时文件（执行后即删）**；mysqldump 参数模板=`--single-transaction --routines --triggers --hex-blob`（+ 按库字符集组装 --default-character-set），导入经 mysql 客户端参数数组执行；产物/文件大小上限 DUMP_SIZE_LIMIT=512MiB（DBM_014）；全部命令一律参数数组（R-08） | PRD 待对齐-5/-8 关闭：零台账哲学（无产物目录治理负担与落位权限面）；--single-transaction 取 InnoDB 一致性快照避免锁表；导出为变更类（数据取出）动作故 POST+CSRF+审计；成功=application/octet-stream 流（Content-Disposition: attachment），失败=JSON 错误体（双形态约定，前端按 Content-Type 分流） |
| D-07 | **探测时机=面板进程内首次进入实例页（任一角色首次 list 触发，进程级一次）+ 手动 rescan（admin）**；默认探测集 DISCOVERY_PROBE_SET=socket 路径 {/tmp/mysql.sock、/var/run/mysqld/mysqld.sock、/run/mysqld/mysqld.sock、/var/lib/mysql/mysql.sock} + TCP localhost:3306（MySQL/MariaDB）/ localhost:6379（Redis），编译期常量 | PRD 待对齐-6 关闭、⚠️A19 承接；自动探测为系统机制（R-15 发现即接管）不写审计、可被 readonly 触发（系统行为非用户变更，ADR-018 不受影响）；手动 rescan 为 admin 显式动作写审计；移除纳管后重新发现（面板重启后的自动探测或手动 rescan）=再纳管，符合 ⚠️A20 语义（永久移除应先停实例，启停归 service-mgmt，PRD N2） |
| D-08 | **状态概览白名单常量**：MySQL status=version + variables 白名单 ≤10 项（version_comment、max_connections、character_set_server、collation_server、innodb_buffer_pool_size、max_allowed_packet、wait_timeout、connect_timeout、slow_query_log、general_log）；Redis info=server/version、clients/connected_clients、memory/used_memory_human、memory/maxmemory_human、stats/total_connections_received、stats/keyspace_hits、stats/keyspace_misses（REDIS_INFO_FIELDS 常量）；不透传全量 variables/info | PRD 待对齐-4 关闭：载荷克制与呈现口径统一；观测 GET 对档案表零写副作用（version 仅在 credentials/rescan 落库刷新，D-04） |
| D-09 | **key 浏览=SCAN 游标分页**：cursor 默认 0、MATCH pattern 默认 `*`、COUNT 200（KEY_SCAN_COUNT 常量），返回 nextCursor+items[{key,type}]，nextCursor=0 即遍历完；TTL 单 key 查询（-1=无过期如实返回）；禁 KEYS 全量命令（pattern 服务端校验 DBM_017）；慢日志条数默认 50 上限 200（SLOWLOG_LIMIT）；processlist 全量返回（连接数量级小） | PRD 待对齐-7 关闭：万级 key 量级经游标承载（PRD N1）；只读浏览无任何写出口（MVP 边界 OUT-3） |
| D-10 | **库级授权=预设三档 grant_level ∈ {readonly, write, all}**（映射 SELECT / SELECT,INSERT,UPDATE,DELETE / ALL PRIVILEGES，库级 `db.*`）；revoke 按库回收全部三档权限；授权概况=实况读取映射回档位呈现（未识别的库外权限不呈现、不回收） | PRD 待对齐-3 关闭：MVP 不做精细 GRANT 子集编辑（攻击面与复杂度）；三档覆盖自托管场景（应用账号/运维账号）；grant_level 新增枚举入池（3.4） |
| D-11 | **建库字符集候选=实例实况读取**（SHOW CHARACTER SET，charsets 接口开放集合返回），默认 utf8mb4（charset 请求参数可空时取默认） | PRD 待对齐-10 关闭：实况候选（与 PHP-FPM pool 候选同为实况读取先例），非固定清单虚构 |
| D-12 | **在途互斥（内存任务表）**：导出/导入执行期间实例标记 busy（进程内 map，P7 过程数据）；busy 时 remove/再次 export/import 返回 DBM_016；任务结束（成功/失败）即解除 | PRD 待对齐-9 关闭；防「导出进行中移除档案致连接悬空」与并发 dump 竞争；单机单进程内存态足够（无集群） |
| D-13 | **审计动作码 13 个**（instance_rescanned/instance_credentials_set/instance_removed/database_created/database_dropped/dbuser_created/dbuser_deleted/dbuser_granted/dbuser_revoked/instance_root_password_changed/database_exported/database_imported/redis_password_set，category 全部=change）；**detail 一律不含密码/密文/密钥**；SQLite 侧全部 SQL 参数绑定（与 R-08 引擎侧命令纪律并行） | R-03/PRD 第五部分；凭据脱敏（⚠️A18）；Mimosa 约束：外部输入禁止拼接 SQL |
| D-14 | 混合模式（MIXED）声明：全部对象实际为自管（本机实况接管，无外部数据源，R-09），不设 manage_mode 字段，权限管控按角色区分 | 分册 Step 2 默认混合模式；与 account-auth D-08 同口径 |

### 1.4 领域对象总览

| 聚合根 | 域内对象/值对象 | 子流程 | 关联领域对象 |
|--------|---------------|--------|-------------|
| db_instance（主数据类子形态） | 凭据（内嵌值对象：cred_username/cred_cipher）、连接状态（探测态字段组） | — | 无 |
| redis_instance（主数据类子形态） | 凭据（内嵌值对象，同上） | — | 无 |
| database（域内对象-实体，引擎实况投影，无表） | — | — | — |
| db_user（域内对象-实体，引擎实况投影，无表） | 授权关系（值对象投影：{database, level}[]） | — | — |
| key 条目/慢日志条目/进程列表条目/概览摘要 | 过程数据（引擎实况只读投影，不入领域模型，PRD 第九部分） | — | — |

### 1.5 上下文映射

| 上下文 | 关系 | 机制 |
|--------|------|------|
| account-auth | 消费方 | 遵奉者：Cookie+CSRF 协议（R-02）、只读边界（ADR-018）；OHS 消费：AuditService.Record（R-03，同事务，凭据脱敏 D-13） |
| MySQL/MariaDB/Redis 引擎（本机系统组件） | 数据源与动作通道 | dbmysql/dbredis 适配器（idea.md §5.3）承载连接/查询/命令，经 **DbGateway 网关契约**（4.1.5）收敛——引擎协议概念止步于此（非外部业务系统，无 ACL 必要，domain-model v1.9 第六部分） |
| cron-backup（M8，后续模块） | 被依赖（预期） | 数据库备份源拟复用 DbGateway.DumpDatabase（4.1.5）；契约口径由该模块 D2/D3 定义，本模块不预先建模 |
| service-mgmt | 边界互斥 | 实例进程启停/自启经 systemd 归 service-mgmt；本模块对 unreachable 实例只如实呈现与报错（DBM_003），不操作进程 |

## 2. 跨聚合根设计

### 2.1 聚合根关系图

```
db_instance（档案，engine ∈ mysql|mariadb）     redis_instance（档案，engine=redis）
    │ 1:N 经实例连接寻址（无 FK，投影链路）          │ 1:N 只读寻址（SCAN 游标）
    ├── database（实况投影，零台账）                 └── key 条目（实况投影，过程数据）
    └── db_user（实况投影，零台账）
              └── 授权关系 {database, level}（值对象投影）
两聚合根共驻合一档案表 db_instance（D-01，engine 列区分，identity_key 唯一）；
database/db_user 不落库：事实源在引擎实况，恒经 DbGateway 实时读取（I-04）
```

### 2.2 跨聚合根不变量

| 编号 | 不变量 | 违反处理 |
|------|--------|---------|
| I-01 | identity_key 全表唯一（探测消重锚点，D-01）；探测/纠正连接目标均按 identity_key 归并 | UNIQUE 约束 + DBM_020（纠正冲突） |
| I-02 | 凭据只进不出：cred_cipher 任何接口不回显、审计 detail 不含密码/密文、密钥不经接口下发（⚠️A18，D-02/D-13） | 接口层保证（响应字段白名单） |
| I-03 | conn_state 为探测态：仅探测动作（自动探测/rescan）整值覆盖，无流转语义；观测/治理接口不修改它 | 读取侧保证（D-07/D-08） |
| I-04 | 零台账一致性：database/db_user/key/慢日志/进程列表恒为当次 DbGateway 实况读取，无缓存陈旧回退；GET 观测接口对 db_instance 表零写副作用（version 仅 credentials/rescan 刷新，D-04） | 读取侧保证（每请求实时） |
| I-05 | 治理操作统一前置链：instanceId 形态（DBM_001）→ 档案存在（DBM_002）→ 引擎匹配（DBM_018：mysql/database/dbuser/backup 接口要求 engine∈mysql/mariadb，redis/key 接口要求 engine=redis）→ reachable（DBM_003）→ 凭据已配置（DBM_004）→ 执行 | 统一前置校验链 |
| I-06 | 改密同步：引擎侧改密成功与面板密文 UPSERT 于同一请求编排（D-05）；同步失败不回滚引擎侧（改密是用户意图），返回 DBM_19 显式恢复路径 | 接口编排保证 |

### 2.3 跨聚合根编排

| 编排 | 触发 | 步骤 |
|------|------|------|
| 探测纳管编排 | 进程内首次 list / 手动 rescan（D-07） | DISCOVERY_PROBE_SET 探测 → 新 identity INSERT 档案（engine/conn_mode/identity_key，无凭据）→ 既有档案 conn_state 整值更新 → version 随可达连接实况刷新 →（仅 rescan）审计 instance_rescanned |
| 凭据接管编排 | POST /db/instance/credentials（D-03） | 组装连接目标 → DbGateway 连接验证（DBM_005/006 不落库）→ 成功：加密凭据（AES-GCM，D-02）UPSERT + conn_state=reachable + version 刷新 → 审计（脱敏） |
| 改密同步编排 | root-password / redis/password（D-05） | I-05 前置链 → 引擎侧改密 → 面板凭据密文 UPSERT（同请求）→ 失败返回 DBM_019（不回滚引擎侧）→ 审计（脱敏） |
| 备份导出编排 | POST /db/backup/export（D-06/D-12） | I-05 前置链 + 库实况存在（DBM_007）+ 在途检查（DBM_016）→ 标记 busy → mysqldump 参数数组执行 → 流式响应（成功流/失败 JSON 双形态）→ 解除 busy → 审计（detail 含 sizeBytes） |
| 备份导入编排 | POST /db/backup/import（D-06/D-12） | I-05 前置链 + 库存在 + 文件大小校验（DBM_014）→ 标记 busy → 落临时文件（用后即删）→ mysql 客户端参数数组执行 → 解除 busy → 审计；**导入非幂等**（SQL 重放语义，交互层二次确认前置，PRD FR-16） |

## 3. 应用/基础设施层设计

### 3.1 接口规范

| 项 | 规范 |
|----|------|
| 路径格式 | `/db/{resource}/{action}` 三段式（G1-08/M2，无双级资源例外）；资源单数（instance/database/dbuser/backup/mysql/redis/key） |
| HTTP 方法 | 读=GET，写=POST（面板内部契约，account-auth D-07 同款）；backup/export 为 POST（数据取出动作+审计，D-06） |
| 响应格式 | `{"code": 0, "message": "ok", "data": {...}}`；code=0 成功，非 0 见 3.4；**例外**：backup/export 成功=application/octet-stream 文件流（Content-Disposition: attachment; filename=`{engine-host-port}_{db}_{yyyyMMddHHmmss}.sql`），失败=JSON 错误体（双形态，D-06）；backup/import=multipart/form-data 请求 |
| 时间格式 | ISO-8601 UTC；TTL 为整数秒（-1=无过期，Redis 语义如实透传） |
| 认证/CSRF | 复用 account-auth 中间件链（AuthRequired → CsrfGuard(POST) → RoleRequired）：全部接口需会话；POST 需 X-CSRF-Token（R-02） |
| 用户类型标注 | 每接口标注 admin / admin+readonly（ADR-018；本模块无公开接口——全部位于登录会话之后） |
| 审计 | 全部 POST 接口（除 rescan 见 D-07 注）写审计（R-03，经 AuditService.Record 同事务）；GET 观测接口不写审计（查看类） |

### 3.2 依赖与约束

| 依赖 | 类型 | 约束 |
|------|------|------|
| dbmysql / dbredis 适配器（idea.md §5.3） | 数据源与动作通道 | 全部引擎交互收敛于 DbGateway（4.1.5）；mysqldump/mysql 客户端命令一律参数数组（R-08，D-06）；连接凭据仅在网关内解密使用（⚠️A18，D-02） |
| SQLite (modernc.org/sqlite) | 存储 | 档案表 db_instance（D-01）；全部 SQL 参数绑定（D-13，外部输入禁拼接）；审计与档案变更同事务 |
| AES-256-GCM + 密钥文件 | 凭据保护 | db-cred.key（0600，数据目录，D-02）；首次初始化生成、随数据目录迁移；轮换 MVP 不做 |
| account-auth | 横切 | 会话/CSRF/角色边界中间件；AuditService.Record（OHS） |
| 进程内任务表 | 在途互斥 | 导出/导入 busy 标记（内存 map，D-12，P7 过程数据；单机单进程足够） |

### 3.3 权限模型

角色：admin / readonly（panel_role 固定枚举）；无公开接口。

**接口权限矩阵**（23 接口）：

| 接口 | admin | readonly | CSRF |
|------|-------|----------|------|
| GET /db/instance/list | ✓ | ✓（ADR-018 全可看） | — |
| POST /db/instance/rescan | ✓ | ✗（AUTH_008） | ✓ |
| POST /db/instance/credentials | ✓ | ✗（AUTH_008） | ✓ |
| POST /db/instance/remove | ✓ | ✗（AUTH_008） | ✓ |
| GET /db/mysql/status | ✓ | ✓（D0 含状态概览） | — |
| GET /db/mysql/processlist | ✓ | ✓（解释性决策：与慢日志同敏感级） | — |
| POST /db/mysql/root-password | ✓ | ✗（AUTH_008） | ✓ |
| GET /db/database/list | ✓ | ✓（ADR-018） | — |
| GET /db/database/charsets | ✓ | ✗（建库辅助数据，无建库入口） | — |
| POST /db/database/create / delete | ✓ | ✗（AUTH_008） | ✓ |
| GET /db/dbuser/list | ✓ | ✓（解释性决策，凭据不回显） | — |
| POST /db/dbuser/create / delete / grant / revoke | ✓ | ✗（AUTH_008） | ✓ |
| POST /db/backup/export / import | ✓ | ✗（AUTH_008：导出=数据取出+ADR-018 禁上传下载；导入=变更） | ✓ |
| GET /db/redis/info | ✓ | ✓（D0 含状态概览） | — |
| GET /db/redis/slowlog | ✓ | ✓（D0 含慢日志） | — |
| POST /db/redis/password | ✓ | ✗（AUTH_008） | ✓ |
| GET /db/key/list / ttl | ✓ | ✗（AUTH_008：key 值属数据内容取出，D0 readonly 清单未含） | —（GET 只读） |

数据权限规则：无行级隔离——readonly「全可看」含全部实例/库/用户行；变更与数据取出能力即角色边界本身（ADR-018）。敏感字段脱敏：任何响应不含 cred_cipher/cred_username 之外的凭据形态（cred_username 可见——连接账户名非秘密；密码/密文/密钥零出口，I-02）。

### 3.4 公共规范

**错误码**（DBM_ 前缀为本模块新增 20 个；鉴权/CSRF/越界/参数校验复用 account-auth 码族 AUTH_001/002/003/008/012）：

| code | 标识 | HTTP 语义 | 说明 |
|------|------|----------|------|
| DBM_001 | INVALID_INSTANCE_ID | 参数错误 | instanceId 非法（R-14） |
| DBM_002 | INSTANCE_NOT_FOUND | 拒绝 | 实例档案不存在（已移除/从未纳管） |
| DBM_003 | INSTANCE_UNREACHABLE | 拒绝 | 连接状态 unreachable，治理/观测前置拒绝（data.reason；启停实例归 service-mgmt） |
| DBM_004 | CREDENTIALS_NOT_SET | 拒绝 | 实例未配置凭据（仅探测到，不可治理；data.hint=credentials） |
| DBM_005 | CREDENTIALS_INVALID | 拒绝 | 凭据验证失败=认证被拒（用户名/密码错误，data.reason；不落库，D-03） |
| DBM_006 | CONN_TEST_FAILED | 操作失败 | 凭据验证失败=网络/socket 层（连不上，区别于 DBM_005；不落库） |
| DBM_007 | DATABASE_NOT_FOUND | 拒绝 | 目标库不存在（实况校验） |
| DBM_008 | DATABASE_EXISTS | 参数冲突 | 同名库已存在（实况，PRD N2） |
| DBM_009 | DBUSER_NOT_FOUND | 拒绝 | 目标用户不存在（实况） |
| DBM_010 | DBUSER_EXISTS | 参数冲突 | 同名同 host 用户已存在（实况） |
| DBM_011 | GRANT_NOT_FOUND | 参数冲突 | revoke 目标授权关系不存在（实况，D-10） |
| DBM_012 | DUMP_FAILED | 操作失败 | mysqldump 执行失败（磁盘/权限/锁，data.reason；不虚构成功，PRD N2） |
| DBM_013 | IMPORT_FAILED | 操作失败 | 导入执行失败（非法 SQL/中断，data.reason；不自动回滚，PRD N2） |
| DBM_014 | DUMP_SIZE_EXCEEDED | 拒绝 | 导入文件超过 DUMP_SIZE_LIMIT=512MiB（D-06） |
| DBM_015 | KEY_NOT_FOUND | 拒绝 | TTL 查询的 key 不存在（实况；过期即消失） |
| DBM_016 | INSTANCE_BUSY | 操作拒绝 | 实例有导出/导入在途（D-12 内存任务表） |
| DBM_017 | INVALID_PATTERN | 参数错误 | key 匹配模式非法（R-14；禁 KEYS 全量语义） |
| DBM_018 | ENGINE_MISMATCH | 参数错误 | 接口与实例引擎不符（I-05 前置链：mysql 族接口对 redis 实例或反之） |
| DBM_019 | CRED_SYNC_FAILED | 部分失败 | 引擎侧改密成功、面板密文同步失败（D-05；data.recoverHint=credentials；不回滚引擎侧） |
| DBM_020 | IDENTITY_CONFLICT | 参数冲突 | 连接目标纠正后 identity_key 与既有档案冲突（I-01，D-03） |

**枚举**（引用均在 workspace/01-solution/04-business-enum.md 定稿；本模块 D2 定稿 3 项 + 本次 D3 新增 1 项）：

| 枚举标识 | 取值 | 用途 |
|---------|------|------|
| db_engine | mysql \| mariadb \| redis | 档案引擎列/清单筛选/接口路由（I-05） |
| db_conn_mode | socket \| tcp | 连接方式（identity_key 组装，D-01） |
| instance_conn_state | reachable \| unreachable | 探测态（I-03；无流转语义） |
| grant_level | readonly \| write \| all | 库级授权三档（D-10；**本次入池**） |
| audit_result（复用） | success \| failure | 审计结果（OHS 契约） |

**业务常量**：审计动作码 13 个（D-13，action 列存码、category=change）；DISCOVERY_PROBE_SET（D-07 探测集）；MYSQL_STATUS_FIELDS / REDIS_INFO_FIELDS（D-08 白名单）；KEY_SCAN_COUNT=200、SLOWLOG_LIMIT 默认 50 上限 200（D-09）；DUMP_SIZE_LIMIT=512MiB、mysqldump 参数模板 `--single-transaction --routines --triggers --hex-blob`（D-06）；grant_level→权限映射表（D-10：readonly→SELECT、write→SELECT,INSERT,UPDATE,DELETE、all→ALL PRIVILEGES，库级 `db.*`）；建库默认 charset=utf8mb4（D-11）。

## 4. 聚合根与领域对象详细设计

### 4.1 db_instance / redis_instance（聚合根，合一档案表）

#### 4.1.1 领域模型

**表结构组合**：db_instance 单表（合一档案，D-01；凭据为内嵌值对象列组，domain-model v1.9）。

**字段清单**：

db_instance：

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | 聚合根标识（instanceId） |
| identity_key | TEXT | NOT NULL UNIQUE | 消重锚点（I-01）：socket=`engine\|socket:{path}`、tcp=`engine\|tcp:{host}:{port}` |
| engine | TEXT | NOT NULL CHECK(in mysql,mariadb,redis) | db_engine；mysql/mariadb→db_instance 聚合、redis→redis_instance 聚合 |
| conn_mode | TEXT | NOT NULL CHECK(in socket,tcp) | db_conn_mode |
| socket_path | TEXT | NULL | socket 模式必填/唯一来源；tcp 模式 NULL |
| host | TEXT | NULL | tcp 模式必填（默认 localhost）；socket 模式 NULL |
| port | INTEGER | NULL | tcp 模式必填（1~65535）；socket 模式 NULL |
| version | TEXT | NULL | 连接建立后实况刷新（D-04：仅 credentials/rescan 写）；探测初期可空，如实呈现 |
| conn_state | TEXT | NOT NULL DEFAULT 'unreachable' CHECK(in reachable,unreachable) | instance_conn_state 探测态（I-03） |
| cred_username | TEXT | NULL | 凭据值对象-账户名（可见，3.3 脱敏声明）；NULL=未配置凭据（DBM_004 判据） |
| cred_cipher | TEXT | NULL | 凭据值对象-密码密文（AES-256-GCM，D-02）；**零出口**（I-02） |
| cred_updated_at | DATETIME | NULL | 凭据最近更新（呈现用） |
| discovered_at | DATETIME | NOT NULL | 首次发现时间 |
| updated_at | DATETIME | NOT NULL | — |

**不变量**（聚合内）：

| 编号 | 不变量 |
|------|--------|
| I-07 | conn_mode=socket ⇔ socket_path 非空且 host/port 为空；conn_mode=tcp ⇔ host/port 非空且 socket_path 为空（服务端校验 R-14，AUTH_012） |
| I-08 | port ∈ 1~65535；socket_path 为绝对路径形态 |
| I-09 | cred_cipher 与 cred_username 同进同空（凭据值对象整体替换，D-05「同步」语义）；密文可解性由写入路径保证（加密失败=接口失败不落库） |

**领域服务**：InstanceProber（DISCOVERY_PROBE_SET 探测 + identity 归并登记，D-07）、CredentialVault（AES-256-GCM 加解密 + db-cred.key 管理，D-02）、ConnTargetResolver（identity_key 组装/校验，D-01/D-03）、CredentialService（先验证后落库编排，D-03）、PasswordSyncService（改密两步编排，D-05）、BackupService（导出/导入 + busy 任务表，D-06/D-12）、ObservationService（概览/进程/慢日志/key 白名单读取，D-08/D-09）。

#### 4.1.2 状态模型

无状态机（domain-model v1.9 D2 判定 + PRD 第六部分辨析）：conn_state 为探测态（I-03，每次探测整值覆盖）；纳管/移除=记录存在/删除（⚠️A20）；凭据已配置/未配置=派生属性（cred_username 非空）。无审批环节；无状态历史表（操作追溯经 audit_log，动作码 D-13）。

#### 4.1.3 子流程设计

无（导出/导入为单动作过程（D-12 在途互斥），探测为后台机制链路——domain-model v1.9 第四部分声明）。

#### 4.1.4 接口设计

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /db/instance/list | GET | admin+readonly | 双引擎统一实例清单（触发进程内首次探测，D-07） |
| /db/instance/rescan | POST | admin | 手动重新探测（写审计；自动探测不写，D-07） |
| /db/instance/credentials | POST | admin | 凭据配置与连接验证（先验证后落库，D-03） |
| /db/instance/remove | POST | admin | 移除纳管（在途守卫 DBM_016；不卸载不删数据，⚠️A20） |
| /db/mysql/status | GET | admin+readonly | MySQL 状态概览（白名单 D-08） |
| /db/mysql/processlist | GET | admin+readonly | 进程列表实时快照 |
| /db/mysql/root-password | POST | admin | root 密码修改（两步同步，D-05） |
| /db/redis/info | GET | admin+readonly | Redis info 概览（白名单 D-08） |
| /db/redis/slowlog | GET | admin+readonly | 慢日志清单（条数上限 D-09） |
| /db/redis/password | POST | admin | Redis 密码设置（两步同步，D-05） |
| /db/backup/export | POST | admin | mysqldump 导出=文件流（D-06；双形态响应） |
| /db/backup/import | POST | admin | SQL 文件导入（multipart；非幂等，二次确认前置） |

**接口详情**（全部 POST 隐含 CSRF 与 AUTH_008 角色边界；mysql 族接口隐含 I-05 引擎匹配 DBM_018、redis 族同理；观测/治理接口隐含 reachable/凭据前置 DBM_003/004，不再逐条重复）：

- **GET /db/instance/list**：query `{engine?(db_engine)}`。处理：进程内未探测过则先触发自动探测（系统机制不写审计，D-07）→ 读表按 discovered_at 排序 → engine 筛选。响应 data=`{items:[{id, engine, connMode, socketPath?, host?, port?, version?, connState, credentialsSet:boolean, credUsername?, credUpdatedAt?, discoveredAt}], generatedAt}`（cred_cipher/密钥零下发，I-02）。审计：—（查看类）。
- **POST /db/instance/rescan**：无 req。处理：探测编排（2.3）：新 identity INSERT（无凭据档案）→ 既有档案 conn_state/version 整值更新。响应 data=`{discovered:n, updated:n}`。审计：instance_rescanned（change，detail={discovered,updated}）。
- **POST /db/instance/credentials**：req `{instanceId, connMode?, socketPath?, host?, port?, username, password}`（连接目标纠正字段可选，D-03）。校验：instanceId（DBM_001/002）；纠正字段 I-07/I-08（AUTH_012）；identity 重算冲突 DBM_020。执行：凭据接管编排（2.3）——DbGateway 连接验证：认证失败 DBM_005（不落库）、网络层失败 DBM_006（不落库）；成功：CredentialVault 加密 UPSERT + conn_state=reachable + version 实况刷新。响应 data=`{instanceId, connState:"reachable", version}`。审计：instance_credentials_set（change，detail={engine, connMode}，**不含密码**）。
- **POST /db/instance/remove**：req `{instanceId}`。校验：DBM_001/002；在途检查 DBM_016（D-12）。执行：DELETE 档案行（连带凭据密文；不卸载实例/不停进程/不删数据，⚠️A20；交互层二次确认文案明示「不删除数据」）。响应 data=`{instanceId}`。审计：instance_removed（change，detail={engine, identityKey}）。
- **GET /db/mysql/status**：query `{instanceId}`。I-05 前置（engine∈mysql/mariadb）。响应 data=`{version, variables:{...MYSQL_STATUS_FIELDS 白名单 ≤10 项}}`（实时读取，不落库不缓存，D-04/D-08）。审计：—。
- **GET /db/mysql/processlist**：query `{instanceId}`。I-05 前置。响应 data=`{items:[{id, user, host, db, command, timeSec, state, info}], generatedAt}`。审计：—。
- **POST /db/mysql/root-password**：req `{instanceId, newPassword}`。I-05 前置。执行：改密同步编排（2.3，D-05）。响应：成功 data=`{instanceId, synced:true}`；同步失败 DBM_019（data.recoverHint="credentials"）。审计：instance_root_password_changed（change，detail 不含新旧密码）。
- **GET /db/redis/info**：query `{instanceId}`。I-05 前置（engine=redis）。响应 data=`{sections:{server:{version,...}, clients:{connectedClients,...}, memory:{usedMemoryHuman, maxMemoryHuman}, stats:{totalConnectionsReceived, keyspaceHits, keyspaceMisses}}}`（REDIS_INFO_FIELDS 白名单，D-08）。审计：—。
- **GET /db/redis/slowlog**：query `{instanceId, limit?(默认 50，上限 200)}`。I-05 前置。响应 data=`{items:[{id, timestamp, durationUs, command}], generatedAt}`。审计：—。
- **POST /db/redis/password**：req `{instanceId, newPassword}`。I-05 前置。执行：SetRequirepass（面板建连即旧密码验证——修改场景天然满足验证语义，D-05）→ 同请求同步密文。响应/DBM_019 同 root-password。审计：redis_password_set（change，脱敏）。
- **POST /db/backup/export**：req `{instanceId, database}`（JSON）。I-05 前置（mysql/mariadb）+ 库实况存在（DBM_007）+ 在途 DBM_016。执行：备份导出编排（2.3，D-06）——mysqldump 参数数组执行（--single-transaction --routines --triggers --hex-blob + charset 按库实况组装）。响应：成功=SQL 文件流（octet-stream + Content-Disposition，D-06 命名规则）；失败=DBM_012（data.reason）JSON 体。审计：database_exported（change，detail={database, sizeBytes}）。
- **POST /db/backup/import**：multipart `{instanceId, database, file}`。校验：I-05 前置 + 库存在（DBM_007）+ 文件非空且 ≤512MiB（DBM_014）+ 在途 DBM_016；**非幂等**（SQL 重放，交互层二次确认前置，PRD FR-16）。执行：临时文件落盘（执行后即删，D-06）→ mysql 客户端参数数组导入。响应 data=`{instanceId, database, sizeBytes}`；失败 DBM_013（data.reason，不自动回滚）。审计：database_imported（change）。

**跨接口复用规则**：连接目标解析统一 ConnTargetResolver（I-01/I-07）；验证与建连统一 DbGateway.Ping（credentials 与全部治理接口共用）；凭据加解密统一 CredentialVault（零出口）；改密同步统一 PasswordSyncService；导出/导入统一 BackupService（busy 任务表互斥）；审计统一 AuditService.Record（D-13 脱敏）；观测白名单统一 ObservationService（D-08/D-09）。

**幂等语义**：credentials 可重入（重复验证通过即覆盖生效）；remove 重复提交报 DBM_002（如实）；grant 幂等（重复授权结果一致）；revoke 第二次报 DBM_011（如实）；export 重复提交经在途 DBM_016 拒绝（执行完成后可再次导出新产物）；import **非幂等**（二次确认前置）；create 类非幂等（DBM_008/010 如实报错）；delete 类非幂等（DBM_007/009 如实报错，不静默成功）。防重复提交为前端在途态（P3），非服务端一致性手段。

#### 4.1.5 领域事件与集成

无 MQ、无领域事件（R-09 单机零外呼）。仓储契约：`InstanceRepository { List, GetById, GetByIdentity, Create, UpdateConnState, UpdateCredentials, Delete }`（全部参数绑定，D-13）。关联域调用（出站）：AuditService.Record（account-auth OHS，R-03）。**本机资源侧网关契约**（进程内适配层，非 SQL；dbmysql/dbredis 适配器承载，idea.md §5.3；引擎协议概念止步于此）：

```
DbGateway（MySQL/MariaDB 由 dbmysql 承载、Redis 由 dbredis 承载）{
  // MySQL/MariaDB
  Ping(inst, target) error                            // 连接验证（认证/网络错误归一为 DBM_005/006 语义）
  ListDatabases(inst) ([]{name, charset}, error)      // 实况库清单（I-04 实时）
  CreateDatabase(inst, name, charset) error           // 建库（重名→DBM_008）
  DropDatabase(inst, name) error                      // 删库（不存在→DBM_007）
  ShowCharsets(inst) ([]string, error)                // 字符集实况（D-11）
  ListUsers(inst) ([]{username, host, grants:[{db, raw}]}, error)
  CreateUser(inst, username, host, password) error    // 密码止步于网关内（I-02）
  DropUser(inst, username, host) error
  Grant(inst, username, host, db, level) error        // grant_level→权限映射（D-10 常量表）
  Revoke(inst, username, host, db) error              // 按库回收三档权限
  AlterRootPassword(inst, newPassword) error
  StatusVariables(inst) (version string, vars map, error)   // MYSQL_STATUS_FIELDS 白名单（D-08）
  ProcessList(inst) ([]ProcessRow, error)
  DumpDatabase(inst, db) (io.ReadCloser, error)       // mysqldump 参数数组（D-06 模板；R-08）
  ImportDatabase(inst, db, r io.Reader) error         // mysql 客户端参数数组（临时文件由调用方治理）
  // Redis
  Ping(inst, target) error
  InfoFields(inst) (sections map, error)              // REDIS_INFO_FIELDS 白名单（D-08）
  Slowlog(inst, n) ([]SlowlogEntry, error)
  SetRequirepass(inst, newPassword) error
  ScanKeys(inst, cursor, pattern, count) (nextCursor, []{key, type}, error)  // D-09；禁 KEYS
  KeyTTL(inst, key) (ttlSec int64, error)             // -1=无过期；不存在→DBM_015
}
InstanceProber 组合 DISCOVERY_PROBE_SET 探测 + InstanceRepository 归并登记（D-07）
```

约束：全部引擎命令/查询经 dbmysql/dbredis 适配器执行，外部命令一律参数数组（R-08，D-06）；凭据明文仅在网关内解密后建连使用，不外泄任何调用方（⚠️A18，D-02）；引擎方言（SQL 转义/协议错误）翻译为 DBM_ 错误语义，不渗透接口层。

### 4.2 database（域内对象-实体，引擎实况投影）

#### 4.2.1 领域模型

**表结构组合**：无表（domain-model v1.9 D2 判定零库台账——库的事实源在 MySQL 实况，面板外建库自动反映，与 site 零站点台账同哲学）。投影条目字段：`{name, charset}`（经 DbGateway.ListDatabases 每请求实时读取，D-04）。

**不变量**：I-04（零台账一致性，恒实况读取）；库名标识=所属实例+库名（建删/授权/备份按此寻址，实况校验 DBM_007/008）。

**领域服务**：复用 DbGateway（ListDatabases/CreateDatabase/DropDatabase/ShowCharsets）。

#### 4.2.2 状态模型 / 4.2.3 子流程设计

无状态机（PRD 第六部分辨析）；无子流程。

#### 4.2.4 接口设计

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /db/database/list | GET | admin+readonly | 库清单（实时投影） |
| /db/database/charsets | GET | admin | 建库字符集实况候选（D-11） |
| /db/database/create | POST | admin | 建库（name+charset） |
| /db/database/delete | POST | admin | 删库（不可逆；二次确认前置） |

**接口详情**：

- **GET /db/database/list**：query `{instanceId}`。I-05 前置（mysql/mariadb）。响应 data=`{items:[{name, charset}], generatedAt}`。审计：—。
- **GET /db/database/charsets**：query `{instanceId}`。I-05 前置。响应 data=`{items:["utf8mb4", ...]}`（实况开放集合，非固定枚举，D-11）。审计：—。
- **POST /db/database/create**：req `{instanceId, name, charset?}`（charset 空取默认 utf8mb4，D-11）。校验：I-05 前置；name 库名形态白名单（R-14，AUTH_012）；charset ∈ 实况候选（AUTH_012）；同名库实况校验 DBM_008。响应 data=`{instanceId, name, charset}`。审计：database_created（change）。
- **POST /db/database/delete**：req `{instanceId, name}`。校验：I-05 前置；库存在 DBM_007（不静默成功）。响应 data=`{instanceId, name}`。审计：database_dropped（change；不可逆操作，交互层强制二次确认）。

### 4.3 db_user（域内对象-实体，引擎实况投影）

#### 4.3.1 领域模型

**表结构组合**：无表（零台账同 4.2.1）。投影条目字段：`{username, host, grants:[{database, level}]}`（授权关系为值对象投影：实况 raw 权限经 D-10 映射表归并三档呈现；库外/未识别权限不呈现不回收，如实口径）。

**不变量**：I-04（零台账）；用户标识=所属实例+用户名+host；密码只写不读（任何响应不含密码形态字段，I-02 同源纪律）。

**领域服务**：复用 DbGateway（ListUsers/CreateUser/DropUser/Grant/Revoke）+ GrantLevelMapper（D-10 映射）。

#### 4.3.2 状态模型 / 4.3.3 子流程设计

无状态机；无子流程。

#### 4.3.4 接口设计

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /db/dbuser/list | GET | admin+readonly | 用户清单（凭据不回显） |
| /db/dbuser/create | POST | admin | 建用户（username+host+password） |
| /db/dbuser/delete | POST | admin | 删用户（连带回收既有授权，引擎语义） |
| /db/dbuser/grant | POST | admin | 库级授权（三档 grant_level，D-10） |
| /db/dbuser/revoke | POST | admin | 按库回收授权 |

**接口详情**：

- **GET /db/dbuser/list**：query `{instanceId}`。I-05 前置。响应 data=`{items:[{username, host, grants:[{database, level}]}], generatedAt}`（无任何密码字段；readonly 同等可用，D1 解释性决策）。审计：—。
- **POST /db/dbuser/create**：req `{instanceId, username, host, password}`。校验：I-05 前置；username/host 形态（R-14，host 支持 `%` 通配惯例）；同名同 host 实况校验 DBM_010。响应 data=`{instanceId, username, host}`。审计：dbuser_created（change，detail 不含密码）。
- **POST /db/dbuser/delete**：req `{instanceId, username, host}`。校验：用户存在 DBM_009。执行：DropUser（引擎语义连带回收授权）。响应 data=`{instanceId, username, host}`。审计：dbuser_deleted（change）。
- **POST /db/dbuser/grant**：req `{instanceId, username, host, database, level(grant_level)}`。校验：I-05 前置；用户存在 DBM_009、库存在 DBM_007；level ∈ grant_level（AUTH_012）。响应 data=`{instanceId, username, host, database, level}`。审计：dbuser_granted（change）。
- **POST /db/dbuser/revoke**：req `{instanceId, username, host, database}`。校验：授权关系存在 DBM_011（实况，如实报错）。响应 data=`{instanceId, username, host, database}`。审计：dbuser_revoked（change）。

### 4.4 key 浏览（Redis 实况只读投影，过程数据链路）

#### 4.4.1 领域模型

**表结构组合**：无表、无领域对象（key 条目/类型/TTL 为引擎实况只读投影，PRD 第九部分过程数据 P2/P4——事实源在 Redis 侧，面板无留存语义；区别于 monitoring metric 的面板侧缓冲）。链路恒经 DbGateway.ScanKeys/KeyTTL（D-09），**严格只读无任何写出口**（MVP 边界 OUT-3：不做编辑/删除/发布订阅）。

#### 4.4.2 状态模型 / 4.4.3 子流程设计

无状态机（TTL 为倒计时属性）；无子流程。

#### 4.4.4 接口设计

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /db/key/list | GET | admin | key 清单（SCAN 游标分页，admin 专属——key 值属数据内容取出） |
| /db/key/ttl | GET | admin | 单 key TTL 查看 |

**接口详情**：

- **GET /db/key/list**：query `{instanceId, cursor?(默认 "0"), pattern?(默认 "*")}`。I-05 前置（engine=redis）；pattern 形态校验（DBM_017，禁 KEYS 全量语义，R-14）。执行：SCAN cursor MATCH pattern COUNT 200（D-09）。响应 data=`{nextCursor, complete:boolean(nextCursor="0"), items:[{key, type}], generatedAt}`。审计：—（查看类；admin 专属查看）。
- **GET /db/key/ttl**：query `{instanceId, key}`。I-05 前置；key 存在校验 DBM_015（实况——过期即消失，如实）。响应 data=`{key, ttlSec}`（-1=无过期，Redis 语义如实透传）。审计：—。

#### 4.4.5 领域事件与集成

无（只读链路；网关契约见 4.1.5 DbGateway Redis 节）。

## 5. 模块清单汇总

### 5.1 对外服务清单（进程内 OHS/内部服务）

| 服务方法 | 消费方 | 契约要点 |
|---------|--------|---------|
| AuditService.Record(entry)（account-auth OHS，消费） | 本模块 → account-auth | R-03 四要素；同事务；凭据脱敏（D-13）；见 api-account-auth.md 4.3.4 |
| DbGateway.DumpDatabase（预期暴露） | cron-backup（M8，待其 D2/D3 定义） | 数据库备份源复用方向（1.5）；本模块内为唯一实例——导出接口与未来备份任务共用同一网关方法与参数模板（D-06），届时按其模块契约正式登记 |
| InstanceProber / CredentialVault / BackupService | 本模块内部 | 4.1.1 领域服务；busy 任务表 D-12 |
| 中间件 AuthRequired / CsrfGuard / RoleRequired | 复用 account-auth | 顺序见 3.1 |

### 5.2 SQL 清单（SQLite 侧，全部参数绑定，D-13）

| # | SQL | 场景 |
|---|-----|------|
| 1 | SELECT * FROM db_instance ORDER BY discovered_at（engine=? 可选筛选） | 实例清单 |
| 2 | INSERT INTO db_instance(identity_key, engine, conn_mode, ...) / ON CONFLICT(identity_key) DO UPDATE conn_state/version | 探测登记与状态整值更新（I-01/I-03，D-07） |
| 3 | UPDATE db_instance SET cred_username=?, cred_cipher=?, cred_updated_at=?, conn_state='reachable', version=? WHERE id=? | 凭据验证通过后落库（D-03；先验证后写） |
| 4 | UPDATE db_instance SET conn_state=?, version=? WHERE id=? | 探测状态刷新 |
| 5 | DELETE FROM db_instance WHERE id=? | 移除纳管（⚠️A20，连带凭据密文） |
| 6 | SELECT * FROM db_instance WHERE identity_key=? | 连接目标纠正冲突校验（DBM_020） |

（库/用户/key/慢日志/进程列表零 SQL——引擎实况经 DbGateway 读取，I-04。）

### 5.3 DDL

```sql
CREATE TABLE IF NOT EXISTS db_instance (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  identity_key    TEXT    NOT NULL UNIQUE,
  engine          TEXT    NOT NULL CHECK (engine IN ('mysql','mariadb','redis')),
  conn_mode       TEXT    NOT NULL CHECK (conn_mode IN ('socket','tcp')),
  socket_path     TEXT,
  host            TEXT,
  port            INTEGER CHECK (port IS NULL OR (port BETWEEN 1 AND 65535)),
  version         TEXT,
  conn_state      TEXT    NOT NULL DEFAULT 'unreachable' CHECK (conn_state IN ('reachable','unreachable')),
  cred_username   TEXT,
  cred_cipher     TEXT,
  cred_updated_at DATETIME,
  discovered_at   DATETIME NOT NULL,
  updated_at      DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_dbinst_engine ON db_instance(engine);
-- I-07 由服务端校验承载（SQLite CHECK 无法跨列表达互斥空语义）
```

## 6. 更新记录

### 6.1 变更记录

| 版本 | 日期 | 变更类型 | 变更摘要 | 目标节号 |
|------|------|---------|---------|---------|
| v1.0 | 2026-11-12 | initial | 首次生成：23 个 HTTP 接口（/db/{instance,mysql,database,dbuser,backup,redis,key}/*：GET 10——其中 readonly 可用 7、admin 专属 GET 3（charsets/key×2）；POST 13=admin+CSRF）+ 1 个进程内 DbGateway 网关契约（dbmysql/dbredis 承载，R-08 参数数组）+ 1 表 DDL（db_instance 合一档案表，D-01）+ 20 错误码（DBM_001~020，复用 AUTH_001/002/003/008/012）+ 4 枚举引用（db_engine/db_conn_mode/instance_conn_state 池中已定稿 + grant_level 本次入池）+ 13 审计动作码；D-01~D-14 设计决策逐项关闭 prd-database-mgmt.md 待对齐-1~-10；无旧版对比 | — |
| v1.1 | 2026-11-12 | interface_changed | Gate 1 F-1 修正（G1-08/M2/GL-009）：key 浏览 2 接口由四段式 `/db/redis/key/{list,ttl}` 改名为三段式 `/db/key/{list,ttl}`，并删除 3.1 中失实的「双级资源/site-mgmt 先例」例外声明（实跑 site-mgmt 20 路径全三段）；接口数 23 不变、方法/权限/语义均不变；同步 admin PageSpec 与 INDEX api note | 3.1 / 3.3 / 4.4.4 / 6.1 |

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
