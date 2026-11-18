# 斗拱面板 数据库管理（database-mgmt）API 设计

| 属性 | 值 |
|------|-----|
| 版本 | v2.3.1 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-18（v2.3.1 §3.4 补记 DBM_029 ENGINE_UNAUTHORIZED——A-DB13 编号裁定跨层同步闭环；v2.3 表级/集合文档治理 + TCP 优先探测增量） |
| 输入 | prd-database-mgmt.md v1.2、domain-model.md v1.23、d0-clarification-database-mgmt.md、D1 三文档 v1.2；**v2.3 增量**：d0-clarification-database-mgmt-ux-2026-11-18.md（用户实机反馈 5 项拍板 Q1~Q6）、ADR-031（D-18 修订）、prd-database-mgmt.md v1.2 FR-24~39 与待对齐-11；v2.2 安装后配置就绪联动输入沿旧（d0-clarification-2026-11-17-bootstrap.md、api-env-mgmt.md v1.2、prd-env-mgmt.md v1.2 FR-07） |
| 接口规约 | 无激活技术栈（ACTIVE_STACKS 为空）——按通用 REST/JSON 契约设计；认证=HttpOnly Cookie 会话 + CSRF token（R-02）；鉴权=RBAC 两角色边界矩阵（ADR-018）；与 api-account-auth.md 同一响应/错误码契约 |

## 1. 模块设计概述

### 1.1 业务背景及目标

数据库管理是斗拱面板 M4 里程碑模块：解决「自托管运维者如何用面板接管并治理服务器上已装的 MySQL/MariaDB 与 Redis 实例」的问题——本机探测发现即纳管（R-15，不要求重装）、凭据密文接管（⚠️A18 只进不出）、MySQL 库/用户/库级授权/root 密码治理、双引擎实况观测（状态概览/进程列表/info/慢日志）、mysqldump 备份导出导入（R-08 参数数组）、Redis key 只读浏览（MVP 严格只读）。M4 验收硬项：发现存量 MySQL → 建库/建用户/授权/改密码/备份导出再导入成功；Redis 状态与慢日志展示正确；所有操作可审计（idea.md §6 M4）。服务对象：管理员（admin，全部治理与数据动作）、只读用户（readonly，元数据查看接口）。**v2.3 增量（2026-11-18 用户实机反馈轮 Q1~Q6）**：治理深度下探——MySQL/MariaDB 表级治理（表清单/结构/分页行数据/单列主键行写/删表，D-22）与 MongoDB 库/集合/文档治理（删库/建集合/删集合/文档 CRUD，D-23/D-25 修订 D-18）；探测登记改 TCP 优先两阶段+socket 兜底+rescan 保守清理（D-21）；角色口径细化 Q6（D-24：元数据两角色/数据内容+写操作 admin 专属）；呈现层 F2/F5 归前端与 panel-core（本 API 文档零接口承载，面包屑动态段取 instance/list 既有 DTO）。

### 1.2 边界说明

| 项 | 说明 |
|----|------|
| 本模块做 | 实例探测纳管/凭据配置/移除（⚠️A20）、库与用户 CRUD 与库级授权、root/Redis 密码修改、状态概览/进程列表/info/慢日志观测、库级备份导出导入、key 只读浏览；**v2.3 +MySQL 表级治理（表清单/结构/行数据分页/单列主键行写/删表）与 MongoDB 库/集合/文档治理（删库/建集合/删集合/文档分页/插入/编辑/删除）** |
| 本模块不做（接口边界） | ① PostgreSQL（二期，ADR-007）与 SQL 查询工作台/在线执行 SQL（不做，D0 OUT）；② Redis 数据编辑/删除 key/发布订阅（MVP 只读）；③ 实例进程启停/自启（service-mgmt，systemd 层）；④ 定时备份任务与轮转（cron-backup M8——其数据库备份源拟复用本模块 DbGateway.DumpDatabase，契约由该模块 D2/D3 定义）；⑤ 凭据任何形式的回显/导出（⚠️A18：无任何读取出口）；⑥ dbmysql/dbredis 适配器实现（后端工程，本模块经 DbGateway 网关契约消费，4.1.5）；**⑦ v2.3 新增 OUT：MySQL 插入行/建表/多列主键行写（后续迭代）、MongoDB 聚合管道/索引管理/分片管理（后续迭代）、凭据自动迁移 socket→TCP（不做，D-21 保守口径）、远程实例新增探测目标（探测集仍编译期常量）、mongodump/mongorestore（二期）** |
| 与存储的关系 | SQLite 单文件（ADR-011）；唯一持久化=实例档案合一表 db_instance（D-01，含凭据密文列 D-02）；database/db_user/key 等引擎内对象零台账不落库（domain-model v1.9 D2 判定）；**v2.3：data_table/table_row/mongo_collection/mongo_document 同为零台账投影实体（domain-model v1.23），DDL 零变更无新表无迁移** |

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
| D-15 | **MongoDB 通道=进程内官方 mongo-driver Go 库**（go.mongodb.org/mongo-driver/v2，go.mod 新直接依赖；dbmongo 适配器承载 MongoGateway 网关契约）：连接超时 5s/命令超时 30s 通道级防御；每操作独立建连即断（不驻留连接池长连——与 CLI 通道每次 exec 的生命期语义对齐） | v0.1.4 用户拍板 Q2=A：纯 Go 无外部二进制依赖——根除「宿主机客户端缺席致通道不可用」缺陷类（同日 DBM_021 修复的实发根因）；凭据经 options.Credential 结构化传递，无 URI 转义面（mongosh 连接串需 URL 编码——@ 类字符真实风险点）；R-09 零外呼维持（仅连本机实例）；审计面=go.sum 锁定+依赖最小（driver v2 纯 Go） |
| D-16 | **MongoDB 凭据验证=SCRAM、authSource=admin 固定**（credentials 接口对 mongodb 实例以 admin 库验证；不暴露 authSource 配置面）；验证失败映射复用 DBM_005/006/021（translatePingErr 同链） | 面板纳管的 mongod 语义=管理员用户建在 admin 库（官方运维惯例）；配置面克制（MVP 不做多 authSource）；通道不可用类（DNS/超时/协议错误）经 translatePingErr 归 DBM_006（连接类）——无 CLI 二进制依赖故 ErrUnavailable 哨兵不适用于 dbmongo |
| D-17 | **探测集扩展 +2 目标**：{engine=mongodb, socket, /tmp/mongodb-27017.sock} 与 {engine=mongodb, tcp, localhost:27017}（mongod 默认 socket 路径与端口；D-07 探测时机/审计口径不变） | R-15 发现哲学同构：env-mgmt 已可装 mongodb-org（mongod.service），端口/socket 探测即纳管；socket 路径=官方默认 unixSocketPath（可配置面不追——探测不到即未按默认装，如实） |
| D-18 | **MongoDB 库清单=只读投影（名称/大小/集合数，ListDatabases 实况）**：不提供建库/删库接口（无 mysql 族 database/create、delete 对应物） | MongoDB 隐式建库语义（库随首集合/首文档写入产生，显式 DDL 无对应物）；与 MySQL 面的差异如实裁剪不虚构等价动作；零库台账 D-04 口径同构。**⚠️v2.3 修订（ADR-031，D-25）：「只读/无删库」口径废止——库清单可管理（浏览集合入口+删库 mdb/dropdb），仍无独立建库接口（隐式建库经显式建集合 mcoll/create 达成）；ListDatabases 投影形态与 mdb/list DTO 不变** |
| D-19 | **MongoDB 用户治理=admin 库 users + 三档库级角色 mongo_role ∈ {read, readWrite, dbAdmin}**（原生角色名直用）：创建=db.createUser（用户名/密码/首库+首角色）；删除=db.dropUser（连带回收全部角色，引擎语义）；授予/回收=grantRolesToUser/revokeRolesFromUser；用户清单=users 实况投影（用户名/mechanisms/角色概况 {db, role}）；重名冲突 DBM_010/不存在 DBM_009/授权关系不存在 DBM_011 语义复用 | v0.1.4 用户拍板 Q1=A：对齐 MySQL 用户管理面（创建/删除/授权/回收三档）；mongo_role 与 grant_level 平行不合并（角色 vs 权限集合语义不同构）；密码只写不读（⚠️A18 同构）；mongod 未开鉴权时面板建连即 localhost exception 语境——操作成败如实返回 |
| D-20 | **安装后配置就绪 OHS（v2.2，BootstrapAfterInstall）**：本模块提供进程内 OHS `BootstrapAfterInstall(ctx, engine, actorName) → BootstrapOutcome{Status(succeeded\|failed\|skipped), Note, InstanceID}` 供 env-mgmt worker 消费（数据库类环境安装成功后，装完即可管）。编排：①幂等前置——本机默认 identity（mysql tcp localhost:3306 / redis tcp localhost:6379 / mongodb tcp localhost:27017）已 credentialsSet → skipped；②引导通道建账户/设密码（**MySQL** dbmysql 经 unix socket root auth_socket 免密〔DISCOVERY_PROBE_SET socket 候选逐一尝试〕CREATE USER dougong@localhost + GRANT ALL ON *.* WITH GRANT OPTION，账户已存在→skipped；**Redis** dbredis CONFIG GET requirepass 判未设→CONFIG SET 随机密码 + CONFIG REWRITE + bind 本机校验，已设→skipped；**MongoDB** dbmongo 免认证 localhost exception 建 admin 库 dougong root 角色，已有用户/已开鉴权→skipped）；③治理通道 TCP Ping 验证新凭据（失败→failed，不写未验证凭据，沿 D-03 哲学）；④CredentialVault 加密 UPSERT db_instance + conn_state=reachable + version 刷新 + 审计 instance_bootstrapped（同事务，脱敏）。随机强密码 crypto/rand（24B→base64url 32 字符）；**密码明文仅引导建连与加密内存态，零回显**（BootstrapOutcome.Note/审计/日志均不含密码）；引擎侧引导方法经窄接口注入（测试替身，零 runner 实况依赖）；服务就绪（start/等待）归 env-mgmt，本模块不操作进程 | env-mgmt 安装后配置就绪增量联动（d0-clarification-2026-11-17-bootstrap.md 拍板 Q1~Q4）；实例与凭据生命周期归本模块（四层分界），故引导建账户/注册归本模块 OHS 而非 env-mgmt 自持；与 D-03 先验证后落库/D-02 密文零出口同哲学 |
| D-21 | **探测登记策略=TCP 优先两阶段 + socket 兜底 + rescan 保守清理（v2.3，Q1 二次拍板取代展示层归并）**：ProbeAll 改两阶段——阶段 1 探 DISCOVERY_PROBE_SET 全部 TCP 目标并记录各 engine 本机 TCP 可达性（map[engine]bool）；阶段 2 探 socket 目标：该 engine 本机 TCP 可达 → socket 即使可达也**跳过登记**（视为同一服务的冗余通道，不 INSERT 新档案；既有 socket 档案的 conn_state 仍整值刷新如实），仅该 engine 无任何本机 TCP 可达时 socket 兜底登记（覆盖 skip-networking）。**rescan 保守清理**：探测完成后，对「conn_mode=socket 且同 engine 本机 TCP 本轮可达 且 !CredentialsSet()」的既有档案行执行 DELETE（解除纳管，审计 instance_merged 同事务 R-03）；「已配凭据」的 socket 行**保留**（避免凭据丢失），instance/list 响应项 +redundantSocket:boolean 标注（同 engine 本机 TCP 档案存在且可达时派生 true），引导用户改配 TCP 行后手动 remove；**不做凭据自动迁移**。identity_key 语义不变（D-01）、instance/list 与 rescan 接口形态不变（rescan 响应 +merged:n 统计字段， additive）；远程 TCP 实例不受影响；D-07 探测时机/审计口径不变 | 用户 2026-11-18 二次拍板：宝塔等面板均走 TCP，socket 平行铺行冗余困惑；完全砍 socket 漏 skip-networking 违背 R-15 发现哲学，故 TCP 优先+兜底而非删除；保守清理防误删已配凭据档案（凭据无法从实况推导，D-02 哲学）；代码锚点 prober.go:42 ProbeAll/:66 probeOnce/:95 CredentialsSet()、instance.go:60 Rescan |
| D-22 | **MySQL 表级治理通道与行写约束（v2.3，Q3）**：表清单/结构走 information_schema（TABLES/COLUMNS 实况投影，零台账 D-04 同构）；行数据分页=CLI query() 通道 **JSON_OBJECT 逐行聚合**——先取列名（结构查询）再构造 `SELECT JSON_OBJECT('col1',col1,...) FROM ... LIMIT ?,?`，batch 转义还原后 JSON.parse（避免 TSV 对含换行/制表符值的解析脆弱）；分页 page/pageSize 默认 20 上限 100（TABLE_PAGE_LIMIT 常量）；**行写仅限单列主键表**（结构键信息判定：恰好 1 列 PRIMARY KEY）——WHERE 主键=旧值参数化定位（quoteString A-DB3 标识符白名单、引擎侧 SQL 裁定 A-DB10 沿既有口径），无单列主键 → DBM_023 禁用并如实提示；行编辑按列文本输入（MySQL 隐式类型转换；null 值→NULL 语义，列值 JSON null≠TS undefined 逐形状标注）；主键列不可编辑（定位键）；删表=DROP TABLE 不可逆强确认；**本轮不做插入行/建表/多列主键行写**（OUT） | PRD FR-24~29/待对齐-11 关闭：JSON_OBJECT 构造细节（列名转义/键序）归实现，契约锁定逐行 JSON 形态；单列主键约束=稳定行标识的最小充分条件（多列主键/无主键表行定位歧义，从紧）；数据内容 admin 专属（D-24） |
| D-23 | **MongoDB 集合与文档治理（v2.3，Q4）**：集合清单=ListCollections 实况投影（集合名/文档数/大小，estimatedDocumentCount+collStats 口径）；建集合=createCollection 显式（隐式建库语义——库随首集合写入产生，UI 如实说明，无独立建库接口 D-25）；删集合=dropCollection；文档分页=find skip/limit（默认 20 上限 100，DOC_PAGE_LIMIT 常量），JSON 呈现 _id 置顶；文档插入=InsertOne、编辑=ReplaceOne（按 _id 定位，_id 不可变更）、删除=DeleteOne（按 _id）；**JSON 校验双层**——前端拦截+服务端校验（非法 JSON → DBM_026，R-14）；通道沿用 dbmongo 进程内 mongo-driver（D-15，既有 dial/withClient）；本轮不做聚合管道/索引/分片（OUT） | PRD FR-31~37/待对齐-11 关闭；ReplaceOne 整体替换语义（非字段级 patch，MVP 克制）；_id 为文档稳定标识（domain-model v1.23 mongo_document 实体判定） |
| D-24 | **角色口径 Q6（v2.3）：元数据两角色 / 数据内容+写操作 admin 专属**——元数据 GET（table/list、table/structure、mcoll/list、mdb/list 既有）admin+readonly；数据内容 GET（row/list、mdoc/list）admin 专属（沿 key/list D0 先例「key 值属数据内容取出」）；全部新增 POST admin+CSRF；readonly 进入浏览页可见元数据，数据区负空间「数据内容仅管理员可见」（前端不请求数据内容接口；越界请求 AUTH_008 全局边界拒绝） | D0 Q6 拍板；ADR-018 全可看口径在数据内容维度的从紧细化，与既有 key 浏览判例一致；权限矩阵见 3.3 |
| D-25 | **D-18 修订（ADR-031）+ Mongo 系统库删除保护（v2.3）**：库清单由只读投影改为可管理——mdb/list DTO 不变（前端操作列承载），+mdb/dropdb（dropDatabase 不可逆强确认）；**系统库 admin/config/local 拒绝删除**（DBM_028，服务端硬校验——删除 admin 库将摧毁面板纳管凭据语境，config/local 为引擎内部库）；无独立建库接口（隐式建库经 mcoll/create 达成，D-23）；集合/文档治理见 D-23 | ADR-031（kb design-decisions）：D-18 把「隐式建库语义」过度解读为「不可管理」，用户要管理（F4）；系统库保护为本域推导（防自毁纳管根基，R-14 服务端校验不信任前端） |
| D-26 | **资源段命名与编号裁定（v2.3，关闭待对齐-11）**：新增资源段 table（MySQL 表元数据+删表）/row（MySQL 行数据+行写）/mcoll（Mongo 集合，mc 前缀沿 mdb/muser 先例）/mdoc（Mongo 文档）；删库复用既有资源段 mdb（mdb/dropdb，与 mdb/list 同资源）；全三段式 /db/{resource}/{action}（G1-08/M2）；错误码续编 DBM_022~028（7 个）；审计动作码 +10（table_dropped/row_updated/row_deleted/mongo_database_dropped/mongo_collection_created/mongo_collection_dropped/mongo_document_inserted/mongo_document_updated/mongo_document_deleted/instance_merged）；总接口 30→44（GET 18/POST 26） | 资源段语义自洽（table=元数据面、row=数据内容面，权限分裂自然映射 D-24）；mcoll/mdoc 与 mdb/muser 复合缩写同构保三段式；编号接续既有最大值（DBM_021/动作码 18 个）不复用 |

### 1.4 领域对象总览

| 聚合根 | 域内对象/值对象 | 子流程 | 关联领域对象 |
|--------|---------------|--------|-------------|
| db_instance（主数据类子形态） | 凭据（内嵌值对象：cred_username/cred_cipher）、连接状态（探测态字段组） | — | 无 |
| redis_instance（主数据类子形态） | 凭据（内嵌值对象，同上） | — | 无 |
| database（域内对象-实体，引擎实况投影，无表） | — | — | — |
| db_user（域内对象-实体，引擎实况投影，无表） | 授权关系（值对象投影：{database, level}[]） | — | — |
| key 条目/慢日志条目/进程列表条目/概览摘要 | 过程数据（引擎实况只读投影，不入领域模型，PRD 第九部分） | — | — |
| data_table（域内对象-实体，引擎实况投影，无表，v2.3） | 列结构（元数据字段组：列名/类型/可空/键/默认/额外/注释） | — | — |
| table_row（域内对象-实体，引擎实况投影，无表，v2.3） | 列值（JSON 内容；标识=单列主键值，D-22） | — | — |
| mongo_collection（域内对象-实体，引擎实况投影，无表，v2.3） | — | — | — |
| mongo_document（域内对象-实体，引擎实况投影，无表，v2.3） | JSON 内容（标识=_id，D-23） | — | — |

### 1.5 上下文映射

| 上下文 | 关系 | 机制 |
|--------|------|------|
| account-auth | 消费方 | 遵奉者：Cookie+CSRF 协议（R-02）、只读边界（ADR-018）；OHS 消费：AuditService.Record（R-03，同事务，凭据脱敏 D-13） |
| MySQL/MariaDB/Redis 引擎（本机系统组件） | 数据源与动作通道 | dbmysql/dbredis 适配器（idea.md §5.3）承载连接/查询/命令，经 **DbGateway 网关契约**（4.1.5）收敛——引擎协议概念止步于此（非外部业务系统，无 ACL 必要，domain-model v1.9 第六部分） |
| cron-backup（M8，后续模块） | 被依赖（预期） | 数据库备份源拟复用 DbGateway.DumpDatabase（4.1.5）；契约口径由该模块 D2/D3 定义，本模块不预先建模 |
| **env-mgmt（v2.2 安装后配置就绪）** | **被消费（进程内 OHS 提供）** | **本模块提供 BootstrapAfterInstall OHS（§5.1/§4.6）：env-mgmt worker 数据库类环境安装成功后消费——建管理账户/设随机强密码 + 注册 db_instance 与凭据密文（装完即可管）；单向依赖 env-mgmt→db-mgmt（与 cron-backup 消费 DumpStream 同构）；引导通道（MySQL socket root auth_socket / Redis 免认证 / MongoDB localhost exception）区别于 TCP+密码治理通道；幂等保守不覆盖用户已有配置；引导结果 BootstrapOutcome 回传（不含密码）** |
| service-mgmt | 边界互斥 | 实例进程启停/自启经 systemd 归 service-mgmt；本模块对 unreachable 实例只如实呈现与报错（DBM_003），不操作进程（**安装后配置就绪的服务启动归 env-mgmt 经 SysdGateway，本模块不操作进程**） |
| **panel-core（v2.3 呈现层消费）** | **消费方（呈现能力，非领域集成）** | **顶栏面包屑返回导航（F5）：本模块多级页面（数据库→治理·identity→库→表/集合）消费 panel-core S-09-01 通用面包屑能力——动态段 identity 取 instance/list 既有 DTO（dbStore 档案缓存）、database/table/collection 取路由 params；本 API 文档零新增接口承载（呈现层变更归 D4 PageSpec 与 panel-core）；能力本体增量归 panel-core（跨模块待同步项，D1 AA 附录B 登记）** |

## 2. 跨聚合根设计

### 2.1 聚合根关系图

```
db_instance（档案，engine ∈ mysql|mariadb|mongodb）   redis_instance（档案，engine=redis）
    │ 1:N 经实例连接寻址（无 FK，投影链路）                │ 1:N 只读寻址（SCAN 游标）
    ├── database（实况投影，零台账）                       └── key 条目（实况投影，过程数据）
    │     ├── data_table（MySQL 族，实况投影，零台账，v2.3）
    │     │     └── table_row（实况投影，标识=单列主键值，v2.3）
    │     └── mongo_collection（mongodb，实况投影，零台账，v2.3）
    │           └── mongo_document（实况投影，标识=_id，v2.3）
    └── db_user（实况投影，零台账）
              └── 授权关系 {database, level}（值对象投影）
两聚合根共驻合一档案表 db_instance（D-01，engine 列区分，identity_key 唯一）；
database/db_user/data_table/table_row/mongo_collection/mongo_document 不落库：
事实源在引擎实况，恒经 DbGateway/MongoGateway 实时读取（I-04）
```

### 2.2 跨聚合根不变量

| 编号 | 不变量 | 违反处理 |
|------|--------|---------|
| I-01 | identity_key 全表唯一（探测消重锚点，D-01）；探测/纠正连接目标均按 identity_key 归并 | UNIQUE 约束 + DBM_020（纠正冲突） |
| I-02 | 凭据只进不出：cred_cipher 任何接口不回显、审计 detail 不含密码/密文、密钥不经接口下发（⚠️A18，D-02/D-13） | 接口层保证（响应字段白名单） |
| I-03 | conn_state 为探测态：仅探测动作（自动探测/rescan）整值覆盖，无流转语义；观测/治理接口不修改它 | 读取侧保证（D-07/D-08） |
| I-04 | 零台账一致性：database/db_user/key/慢日志/进程列表恒为当次 DbGateway 实况读取，无缓存陈旧回退；GET 观测接口对 db_instance 表零写副作用（version 仅 credentials/rescan 刷新，D-04）；**v2.3 扩展：data_table/table_row/mongo_collection/mongo_document 同构（DbGateway/MongoGateway 实况读取，零台账无缓存）** | 读取侧保证（每请求实时） |
| I-05 | 治理操作统一前置链：instanceId 形态（DBM_001）→ 档案存在（DBM_002）→ 引擎匹配（DBM_018：mysql/database/dbuser/backup 接口要求 engine∈mysql/mariadb，redis/key 接口要求 engine=redis；**v2.3：table/row 接口要求 engine∈mysql/mariadb，mcoll/mdoc/mdb/dropdb 接口要求 engine=mongodb**）→ reachable（DBM_003）→ 凭据已配置（DBM_004）→ 执行 | 统一前置校验链 |
| I-06 | 改密同步：引擎侧改密成功与面板密文 UPSERT 于同一请求编排（D-05）；同步失败不回滚引擎侧（改密是用户意图），返回 DBM_19 显式恢复路径 | 接口编排保证 |
| I-10（v2.3） | 单列主键行写约束：row/update、row/delete 仅对「恰好 1 列 PRIMARY KEY」的表开放（结构键信息服务端判定）；无单列主键 → DBM_023 拒绝（不进入引擎执行）；行定位 WHERE 主键=旧值（参数化，quoteString A-DB3）；主键列值不可编辑 | 服务端校验（D-22，R-14 不信任前端禁用态） |
| I-11（v2.3） | 数据内容 admin 专属（Q6）：row/list、mdoc/list 与全部新增 POST 仅 admin（readonly AUTH_008）；元数据 GET（table/list、table/structure、mcoll/list）两角色；readonly 负空间由前端呈现（不请求数据内容接口），服务端边界为最终守卫 | 角色中间件 + 前端负空间（D-24） |
| I-12（v2.3） | Mongo 系统库删除保护：mdb/dropdb 对 admin/config/local 一律 DBM_028 拒绝（服务端硬校验，防自毁纳管凭据语境） | 服务端校验（D-25） |

### 2.3 跨聚合根编排

| 编排 | 触发 | 步骤 |
|------|------|------|
| 探测纳管编排 | 进程内首次 list / 手动 rescan（D-07） | **v2.3 两阶段（D-21）**：阶段 1 DISCOVERY_PROBE_SET TCP 目标探测 → 记录各 engine 本机 TCP 可达性 → 新 identity INSERT 档案（engine/conn_mode/identity_key，无凭据）→ 既有档案 conn_state 整值更新 → version 随可达连接实况刷新；阶段 2 socket 目标探测 → 该 engine 本机 TCP 可达则跳过登记（既有 socket 档案 conn_state 仍整值刷新）→ 否则兜底登记；**（仅 rescan）保守清理**：同 engine 本机 TCP 可达且 !CredentialsSet() 的 socket 档案 DELETE + 审计 instance_merged（同事务）→ 审计 instance_rescanned（detail +merged 统计） |
| 凭据接管编排 | POST /db/instance/credentials（D-03） | 组装连接目标 → DbGateway 连接验证（DBM_005/006 不落库）→ 成功：加密凭据（AES-GCM，D-02）UPSERT + conn_state=reachable + version 刷新 → 审计（脱敏） |
| 改密同步编排 | root-password / redis/password（D-05） | I-05 前置链 → 引擎侧改密 → 面板凭据密文 UPSERT（同请求）→ 失败返回 DBM_019（不回滚引擎侧）→ 审计（脱敏） |
| 备份导出编排 | POST /db/backup/export（D-06/D-12） | I-05 前置链 + 库实况存在（DBM_007）+ 在途检查（DBM_016）→ 标记 busy → mysqldump 参数数组执行 → 流式响应（成功流/失败 JSON 双形态）→ 解除 busy → 审计（detail 含 sizeBytes） |
| 备份导入编排 | POST /db/backup/import（D-06/D-12） | I-05 前置链 + 库存在 + 文件大小校验（DBM_014）→ 标记 busy → 落临时文件（用后即删）→ mysql 客户端参数数组执行 → 解除 busy → 审计；**导入非幂等**（SQL 重放语义，交互层二次确认前置，PRD FR-16） |
| 表行写编排（v2.3） | POST /db/row/update、/db/row/delete（D-22） | I-05 前置链（mysql/mariadb）+ 库存在（DBM_007）+ 表存在（DBM_022）+ 单列主键判定（I-10，无则 DBM_023）→ WHERE 主键=旧值参数化执行（quoteString A-DB3）→ 匹配 0 行如实反馈（DBM_024，不静默成功）→ 审计 row_updated/row_deleted（同事务脱敏，detail 不含行数据明文） |
| Mongo 文档写编排（v2.3） | POST /db/mdoc/insert、/db/mdoc/update、/db/mdoc/delete（D-23） | I-05 前置链（mongodb）+ 库/集合存在（DBM_007/025）+ JSON 服务端校验（DBM_026，insert/update）→ InsertOne/ReplaceOne(by _id)/DeleteOne(by _id) → _id 不存在/匹配 0 如实反馈（DBM_027，update/delete）→ 审计 mongo_document_inserted/updated/deleted（同事务脱敏，detail 不含文档内容明文） |

## 3. 应用/基础设施层设计

### 3.1 接口规范

| 项 | 规范 |
|----|------|
| 路径格式 | `/db/{resource}/{action}` 三段式（G1-08/M2，无双级资源例外）；资源单数（instance/database/dbuser/backup/mysql/redis/key + v2.0 新增 mongo/mdb/muser——mdb=MongoDB 库、muser=MongoDB 用户；复合缩写与 dbuser 先例同构，保三段式 G1-08）；**v2.3 新增 table/row/mcoll/mdoc（D-26：table=MySQL 表元数据面、row=MySQL 行数据面、mcoll=Mongo 集合、mdoc=Mongo 文档；删库复用 mdb 资源段 mdb/dropdb）** |
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
| dbmongo 适配器（mongo-driver v2，v2.0 新增） | MongoDB 数据源与动作通道 | 进程内官方 Go 库 go.mongodb.org/mongo-driver/v2（新直接依赖，D-15：拍板 Q2=A——纯 Go 无外部二进制依赖，无「宿主机客户端缺席」缺陷类；凭据经 options.Credential 结构化传递，无 URI 转义面；R-09 零外呼维持——仅连本机实例；连接超时 5s/命令超时 30s 通道级防御；SCRAM 认证 authSource=admin 固定 D-16） |

### 3.3 权限模型

角色：admin / readonly（panel_role 固定枚举）；无公开接口。

**接口权限矩阵**（44 接口，v2.3 +table/row 6 +mcoll/mdoc/mdb-dropped 8；v2.0 +mongo 7）：

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
| GET /db/mongo/status | ✓ | ✓（D0 含状态概览，同 mysql/redis 观测口径） | — |
| GET /db/mdb/list | ✓ | ✓（ADR-018 全可看） | — |
| GET /db/muser/list | ✓ | ✓（解释性决策：与 dbuser/list 同判，凭据不回显） | — |
| POST /db/muser/create / delete | ✓ | ✗（AUTH_008） | ✓ |
| POST /db/muser/grant / revoke | ✓ | ✗（AUTH_008） | ✓ |
| GET /db/table/list（v2.3） | ✓ | ✓（元数据 Q6，D-24） | — |
| GET /db/table/structure（v2.3） | ✓ | ✓（元数据 Q6，D-24） | — |
| POST /db/table/drop（v2.3） | ✓ | ✗（AUTH_008） | ✓ |
| GET /db/row/list（v2.3） | ✓ | ✗（AUTH_008：行数据属数据内容取出，Q6 沿 key/list 先例，I-11） | —（GET 只读） |
| POST /db/row/update / delete（v2.3） | ✓ | ✗（AUTH_008） | ✓ |
| GET /db/mcoll/list（v2.3） | ✓ | ✓（元数据 Q6，D-24） | — |
| POST /db/mcoll/create / drop（v2.3） | ✓ | ✗（AUTH_008） | ✓ |
| POST /db/mdb/dropdb（v2.3） | ✓ | ✗（AUTH_008） | ✓ |
| GET /db/mdoc/list（v2.3） | ✓ | ✗（AUTH_008：文档属数据内容取出，Q6 沿 key/list 先例，I-11） | —（GET 只读） |
| POST /db/mdoc/insert / update / delete（v2.3） | ✓ | ✗（AUTH_008） | ✓ |

数据权限规则：无行级隔离——readonly「全可看」含全部实例/库/用户行；变更与数据取出能力即角色边界本身（ADR-018）。**v2.3 Q6 细化（D-24/I-11）：readonly「全可看」扩展至表清单/表结构/集合清单（元数据），数据内容（行/文档）与全部写操作 admin 专属——readonly 浏览页数据区负空间「数据内容仅管理员可见」（前端不请求 row/list、mdoc/list），越界请求 AUTH_008 全局边界拒绝。**敏感字段脱敏：任何响应不含 cred_cipher/cred_username 之外的凭据形态（cred_username 可见——连接账户名非秘密；密码/密文/密钥零出口，I-02）。

### 3.4 公共规范

**错误码**（DBM_ 前缀本模块共 29 个：v2.3 +DBM_022~028，v2.3.1 补记 DBM_029〔A-DB13 编号裁定，后端 errcode 已实现、前端映射已同步〕；鉴权/CSRF/越界/参数校验复用 account-auth 码族 AUTH_001/002/003/008/012）：

| code | 标识 | HTTP 语义 | 说明 |
|------|------|----------|------|
| DBM_001 | INVALID_INSTANCE_ID | 参数错误 | instanceId 非法（R-14） |
| DBM_002 | INSTANCE_NOT_FOUND | 拒绝 | 实例档案不存在（已移除/从未纳管） |
| DBM_003 | INSTANCE_UNREACHABLE | 拒绝 | 连接状态 unreachable，治理/观测前置拒绝（data.reason；启停实例归 service-mgmt） |
| DBM_004 | CREDENTIALS_NOT_SET | 拒绝 | 实例未配置凭据（仅探测到，不可治理；data.hint=credentials） |
| DBM_005 | CREDENTIALS_INVALID | 拒绝 | 凭据验证失败=认证被拒（账户不存在/host 匹配落空/密码不符等——MySQL 1045 不区分诸因统一拒绝；data.reason=服务器原文；不落库，D-03。v2.1：前端凭据弹窗内联呈现 reason+按引擎族可操作排查指引，文案不再断言「用户名或密码错误」） |
| DBM_006 | CONN_TEST_FAILED | 操作失败 | 凭据验证失败=网络/socket 层（连不上，区别于 DBM_005；data.reason=服务器原文，v2.1 前端内联呈现；不落库） |
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
| DBM_021 | CHANNEL_UNAVAILABLE | 操作失败 | 引擎客户端通道不可用（宿主机未检出 mysql/redis-cli 二进制；data.reason；区别于 DBM_006——实例可能正常，缺的是客户端；v1.2 补齐——此前该哨兵无映射裸落 INTERNAL） |
| DBM_022（v2.3） | TABLE_NOT_FOUND | 拒绝 | 目标表不存在（实况校验；table/structure、table/drop、row/* 前置） |
| DBM_023（v2.3） | NO_SINGLE_PK | 拒绝 | 表无单列主键，行写禁用（I-10/D-22；data.reason 如实提示原因——无主键/复合主键；前端入口禁用态同源判定，服务端为最终守卫） |
| DBM_024（v2.3） | ROW_NOT_FOUND | 拒绝 | 行定位失败（WHERE 主键=旧值匹配 0 行——行已删除或主键值已被面板外修改；不静默成功，PRD N2 v1.2） |
| DBM_025（v2.3） | COLLECTION_NOT_FOUND | 拒绝 | 目标集合不存在（实况校验；mcoll/drop、mdoc/* 前置） |
| DBM_026（v2.3） | INVALID_DOCUMENT_JSON | 参数错误 | 文档 JSON 非法（服务端校验 R-14；前端拦截为先，服务端为最终守卫；mdoc/insert、mdoc/update） |
| DBM_027（v2.3） | DOCUMENT_NOT_FOUND | 拒绝 | 目标文档不存在（_id 定位匹配 0——已删除/从未存在；mdoc/update、mdoc/delete；不静默成功） |
| DBM_028（v2.3） | SYSTEM_DATABASE_PROTECTED | 拒绝 | Mongo 系统库（admin/config/local）删除保护（I-12/D-25；mdb/dropdb 服务端硬校验） |
| DBM_029（v2.3.1 补记） | ENGINE_UNAUTHORIZED | 拒绝 | 引擎操作被拒——凭据账户权限不足（身份对但无权执行该命令，区别于 DBM_005 认证被拒〔身份错〕；data.reason=引擎原文，如 MongoDB Unauthorized code 13；触达 mongo/status、muser/*、v2.3 全部 mongo 治理接口〔translateGatewayErr 对 dbmongo.ErrUnauthorized 统一翻译〕。编号沿革 A-DB13：v0.1.4 修复轮原占 DBM_022，v2.3 定稿 DBM_022=TABLE_NOT_FOUND 后续接空号改 DBM_029，符号 ErrDBMEngineUnauthorized/语义不变；本行为设计侧补记——后端已实发、前端 DBM_ERROR_TEXT/两 Mongo 弹窗判定已同步〔Gate 2 E1/AL-005 闭环〕） |

**枚举**（引用均在 workspace/01-solution/04-business-enum.md 定稿；本模块 D2 定稿 3 项 + D3 新增 grant_level/mongo_role 2 项；**v2.3 零新增枚举**——表存储引擎/排序规则为实例实况开放集合沿字符集先例不入池，审计动作码为常量非池枚举；v2.0 遗漏本表补正：db_engine +mongodb、mongo_role 行补列）：

| 枚举标识 | 取值 | 用途 |
|---------|------|------|
| db_engine | mysql \| mariadb \| redis \| mongodb | 档案引擎列/清单筛选/接口路由（I-05；mongodb v2.0 扩值，迁移 011） |
| db_conn_mode | socket \| tcp | 连接方式（identity_key 组装，D-01） |
| instance_conn_state | reachable \| unreachable | 探测态（I-03；无流转语义） |
| grant_level | readonly \| write \| all | 库级授权三档（D-10；v1.0 入池） |
| mongo_role | read \| readWrite \| dbAdmin | MongoDB 库级角色三档（D-19；v2.0 入池，本表 v2.3 补列） |
| audit_result（复用） | success \| failure | 审计结果（OHS 契约） |

**业务常量**：审计动作码（D-13 基础 13 个 + v2.0 mongo_user_created/deleted/granted/revoked 4 个 + v2.2 instance_bootstrapped + **v2.3 新增 10 个：table_dropped/row_updated/row_deleted/mongo_database_dropped/mongo_collection_created/mongo_collection_dropped/mongo_document_inserted/mongo_document_updated/mongo_document_deleted/instance_merged**，共 28 个，action 列存码、category=change；instance_bootstrapped detail={engine, connMode, status}，**不含密码**，安装后配置就绪注册审计 §4.6；**v2.3 detail 脱敏口径：行写/文档写 detail 仅含对象定位（instanceId/database/table/主键值或 _id）与动作结果，不含行数据/文档内容明文**；instance_merged detail={engine, identityKey, reason:"redundant_socket_tcp_reachable"}，rescan 保守清理专用 D-21）；**v2.3 分页常量：TABLE_PAGE_LIMIT/DOC_PAGE_LIMIT 默认 20 上限 100（D-22/D-23）**；DISCOVERY_PROBE_SET（D-07 探测集）；MYSQL_STATUS_FIELDS / REDIS_INFO_FIELDS（D-08 白名单）；KEY_SCAN_COUNT=200、SLOWLOG_LIMIT 默认 50 上限 200（D-09）；DUMP_SIZE_LIMIT=512MiB、mysqldump 参数模板 `--single-transaction --routines --triggers --hex-blob`（D-06）；grant_level→权限映射表（D-10：readonly→SELECT、write→SELECT,INSERT,UPDATE,DELETE、all→ALL PRIVILEGES，库级 `db.*`）；建库默认 charset=utf8mb4（D-11）。

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

- **GET /db/instance/list**：query `{engine?(db_engine)}`。处理：进程内未探测过则先触发自动探测（系统机制不写审计，D-07；**v2.3 两阶段 TCP 优先登记 D-21**）→ 读表按 discovered_at 排序 → engine 筛选。响应 data=`{items:[{id, engine, connMode, socketPath?:string|null, host?:string|null, port?:number|null, version?:string|null, connState, credentialsSet:boolean, credUsername?:string|null, credUpdatedAt?:string|null, discoveredAt, redundantSocket:boolean（v2.3 新增，D-21：conn_mode=socket 且同 engine 本机 TCP 档案存在且可达时派生 true，其余 false——已配凭据冗余 socket 行保留+标注口径）}], generatedAt}`（cred_cipher/密钥零下发，I-02；**可空字段 JSON null≠TS undefined，前端类型须显式 | null——children:null 先例同口径**）。审计：—（查看类）。**v2.3 呈现注（F1/F2，D4 承载）**：TCP 优先登记下常见情况天然一行 TCP；清单说明文案与状态/凭据 Tag 语义 tooltip+图例归前端（绿=可达/橙=不可达，仅连通性不代表凭据正确），本接口 DTO 除 redundantSocket 外零变更。
- **POST /db/instance/rescan**：无 req。处理：探测编排（2.3，**v2.3 两阶段 D-21**）：TCP 目标探测登记/更新 → socket 目标按引擎 TCP 可达性跳过登记或兜底 → **保守清理：同 engine 本机 TCP 可达且 !credentialsSet 的 socket 档案 DELETE（审计 instance_merged 同事务）；已配凭据 socket 行保留（redundantSocket 标注经 list 呈现）**。响应 data=`{discovered:n, updated:n, merged:n（v2.3 新增：本轮保守清理的冗余 socket 行数，additive 字段）}`。审计：instance_rescanned（change，detail={discovered,updated,merged}）+ 每清理行 instance_merged（change，同事务）。
- **POST /db/instance/credentials**：req `{instanceId, connMode?, socketPath?, host?, port?, username, password}`（连接目标纠正字段可选，D-03）。校验：instanceId（DBM_001/002）；纠正字段 I-07/I-08（AUTH_012）；identity 重算冲突 DBM_020。执行：凭据接管编排（2.3）——DbGateway 连接验证：认证失败 DBM_005（不落库）、网络层失败 DBM_006（不落库，含未分类引擎失败归并）、客户端通道不可用 DBM_021（不落库）；成功：CredentialVault 加密 UPSERT + conn_state=reachable + version 实况刷新。响应 data=`{instanceId, connState:"reachable", version}`。审计：instance_credentials_set（change，detail={engine, connMode}，**不含密码**）。
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
  // MySQL 表级治理（v2.3，D-22；dbmysql 承载，CLI query() 通道 R-08 参数数组）
  ListTables(inst, db) ([]{name, engine, rowEstimate, dataSizeBytes, collation, comment}, error)
      // information_schema.TABLES 实况投影；行数估算（TABLE_ROWS，InnoDB 统计值如实标注估算语义）
  TableStructure(inst, db, table) ([]{column, type, nullable, key, default, extra, comment}, error)
      // information_schema.COLUMNS 实况投影；key 含 PRI 判定单列主键（I-10）
  TableRows(inst, db, table, offset, limit) (rows []map[string]any, total int64, error)
      // JSON_OBJECT 逐行聚合（先取列名再构造 SELECT JSON_OBJECT(...) LIMIT ?,?，batch 转义还原后解析）
      // 列值 JSON null=NULL 语义（≠TS undefined）；total=COUNT(*) 实况
  DropTable(inst, db, table) error                    // DROP TABLE（不存在→DBM_022；不可逆）
  UpdateRow(inst, db, table, pkColumn, pkValue, values map[string]any) (affected int64, error)
      // WHERE pkColumn=pkValue 参数化（quoteString A-DB3）；values 含 null→SET col=NULL；affected=0→DBM_024
  DeleteRow(inst, db, table, pkColumn, pkValue) (affected int64, error)
      // WHERE pkColumn=pkValue 参数化；affected=0→DBM_024
}
InstanceProber 组合 DISCOVERY_PROBE_SET 两阶段探测（TCP 优先/socket 兜底，D-21）+ InstanceRepository 归并登记（D-07）+ rescan 保守清理（D-21）
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

### 4.5 MongoDB 治理（mongo 实况投影，v2.0 新增）

#### 4.5.1 领域对象与网关

- 领域对象：mongodb 实例=db_instance 聚合根 engine 扩值（D-01 合一表迁移 011）；mongo 库/用户=实况实时投影零落库（I-04 同构，D-18/D-19）。
- 网关契约：**MongoGateway**（进程内 mongo-driver 通道，D-15）——Available（恒 nil，无二进制依赖）/Ping/ServerVersion/ListDatabases/StatusSummary/ListUsers/CreateUser/DropUser/GrantRole/RevokeRole；哨兵复用 dbmysql 族（ErrAuthRejected/ErrConnFailed/ErrQueryFailed 归一）由 translateGatewayErr/translatePingErr 翻译（D-16）。**v2.3 扩展（D-23/D-25，dbmongo 承载，既有 dial/withClient）**：

```
MongoGateway 扩展方法（v2.3）{
  DropDatabase(inst, db) error                        // dropDatabase（系统库保护 I-12 由服务层前置，DBM_028）
  ListCollections(inst, db) ([]{name, docCount, sizeBytes}, error)
      // ListCollections 实况 + estimatedDocumentCount/collStats（文档数估算语义如实）
  CreateCollection(inst, db, name) error              // createCollection（重名→DBM_008 语义复用）
  DropCollection(inst, db, name) error                // dropCollection（不存在→DBM_025）
  ListDocuments(inst, db, coll, skip, limit) (docs []map[string]any, total int64, error)
      // find skip/limit + CountDocuments；_id 置顶呈现由前端排序（响应保引擎自然序）
  InsertDocument(inst, db, coll, doc map[string]any) (insertedId any, error)
      // InsertOne；_id 冲突→引擎错误如实（DBM_008 语义复用，data.reason）
  ReplaceDocument(inst, db, coll, id any, doc map[string]any) (matched int64, error)
      // ReplaceOne by _id（_id 不可变更，服务层剥离/校验）；matched=0→DBM_027
  DeleteDocument(inst, db, coll, id any) (deleted int64, error)
      // DeleteOne by _id；deleted=0→DBM_027
}
```

- 前置链：I-05 同构——mongo 族接口要求 engine=mongodb（DBM_018）；reachable/凭据前置（DBM_003/004）。**v2.3：mcoll/mdoc/mdb/dropdb 同链（I-05 扩展）+ 集合存在 DBM_025（mdoc/* 与 mcoll/drop）+ JSON 校验 DBM_026（mdoc/insert/update）+ 系统库保护 DBM_028（mdb/dropdb）**。

#### 4.5.2 接口详情（7 接口；POST 隐含 CSRF+admin，D-19 角色矩阵见 3.3）

- **GET /db/mongo/status**：query `{instanceId}`。响应 data=`{version, uptimeSec, connections:{current,available}, memResidentMb, opcounters:{insert,query,update,delete}}`（serverStatus 白名单 D-08 同构 ≤10 项：version/uptime/connections.current/connections.available/mem.resident/opcounters.insert/query/update/delete）。审计：—。
- **GET /db/mdb/list**：query `{instanceId}`。响应 data=`{items:[{name, sizeBytes, collections}], generatedAt}`（ListDatabases 实况投影；系统库 admin/config/local 如实返回）。审计：—。**v2.3 修订（rule_changed，ADR-031/D-25）**：D-18「只读投影」口径废止——本清单为可管理清单（前端行操作「浏览集合」→mcoll/list、「删除库」→mdb/dropdb，admin 专属）；**DTO 零变更**（操作入口归前端呈现，系统库删除保护 DBM_028 服务端承载）。
- **GET /db/muser/list**：query `{instanceId}`。响应 data=`{items:[{username, mechanisms:[...], roles:[{db, role}]}], generatedAt}`（admin 库 users 实况投影；密码/凭证零下发 I-02）。审计：—。
- **POST /db/muser/create**：req `{instanceId, username, password, db, role}`。校验：username/db 标识符白名单（R-14，AUTH_012）；role∈mongo_role（AUTH_012）；重名 DBM_010（实况）。执行：db.createUser（首库首角色 D-19）；密码止步网关（I-02）。响应 data=`{instanceId, username}`。审计：mongo_user_created（change，detail={username, db, role}，不含密码）。
- **POST /db/muser/delete**：req `{instanceId, username}`。校验：DBM_009（不存在，实况）。执行：db.dropUser（连带回收全部角色）。响应 data=`{instanceId, username}`。审计：mongo_user_deleted。
- **POST /db/muser/grant**：req `{instanceId, username, db, role}`。校验：用户 DBM_009；目标库建议取自 database/list 实况（不存在库授权 MongoDB 允许——库不存在时如实放行并提示，不虚构 DBM_007 阻断）。执行：grantRolesToUser（幂等）。响应 data=`{instanceId, username, db, role}`。审计：mongo_user_granted。
- **POST /db/muser/revoke**：req `{instanceId, username, db, role}`。校验：用户 DBM_009；授权关系不存在 DBM_011（实况）。执行：revokeRolesFromUser。响应 data=`{instanceId, username, db, role}`。审计：mongo_user_revoked。

#### 4.5.3 领域事件与集成

无（实况投影链路；mongo-driver 为进程内库非外部集成——R-09 口径，ACL 不适用）。

### 4.6 安装后配置就绪 OHS（v2.2 新增，BootstrapAfterInstall）

**定位**：本模块对 env-mgmt 提供的进程内 OHS（非 HTTP 接口，无权限矩阵/CSRF——调用方 env-mgmt worker 已在 admin 发起的安装任务上下文内）。数据库类环境安装成功后，env-mgmt 消费本 OHS 完成「建管理账户/设随机强密码 + 注册实例与凭据」，使面板装完即可治理（D-20）。**无新领域对象/无新表**——复用 db_instance 聚合与 cred_cipher 密文列、CredentialVault、InstanceRepository；引导结果为 env-mgmt 任务台账字段（bootstrap_status/note），本模块仅回传 BootstrapOutcome。

**OHS 签名**：

```
BootstrapAfterInstall(ctx context.Context, engine string, actorName string) (BootstrapOutcome, error)
  engine ∈ {mysql, redis, mongodb}（其余值返回错误，调用方 env-mgmt 仅对 BOOTSTRAP_ENV_TYPES 触发）
  actorName = 安装任务发起者用户名（审计 actor；env_install_task.requested_by）
BootstrapOutcome {
  Status     string   // succeeded | failed | skipped（与 env-mgmt bootstrap_status 同值域）
  Note       string   // 人类可读摘要/原因（**不含密码/密文/密钥**）
  InstanceID *int64   // 注册成功时指向 db_instance.id（succeeded）；skipped 已有档案时亦返回；failed 为 nil
}
```

**编排（每引擎，幂等保守）**：

| 步 | 动作 | 失败/跳过语义 |
|----|------|--------------|
| 1 | 本机默认 identity 归并：GetByIdentity（mysql tcp localhost:3306 / redis tcp localhost:6379 / mongodb tcp localhost:27017）；已 credentialsSet → **skipped**（Note=已配置凭据，不覆盖） | 保护用户已有凭据（D-03 哲学） |
| 2 | 引导通道建账户/设密码（引擎窄接口，见下）：MySQL socket root auth_socket 免密建 dougong@localhost（ALL WITH GRANT OPTION）/ Redis CONFIG SET requirepass+REWRITE / MongoDB localhost exception 建 admin root；随机强密码 crypto/rand | 账户已存在/requirepass 已设/已有用户 → **skipped**（不覆盖）；建连失败/CREATE 被拒/超时 → **failed**（Note 如实原因） |
| 3 | 治理通道 TCP Ping 验证新凭据（mysql/redis/mongo.Ping，复用既有网关） | 验证失败 → **failed**（不写未验证凭据，Note 含排查指引） |
| 4 | CredentialVault.Encrypt → 同事务 UPSERT db_instance（identity 不存在则创建）+ cred_username/cred_cipher + conn_state=reachable + version 刷新 + 审计 instance_bootstrapped（脱敏） | 加密/落库失败 → **failed**；成功 → **succeeded**（InstanceID 返回） |

**引擎引导窄接口**（适配器实现，main 注入；与治理接口分离——既有治理测试不受影响，引导测试用替身，零 runner 实况依赖）：

```
MySQLBootstrap {
  // 经 unix socket 以 root auth_socket 免密建连（socketPath 候选由调用方逐一尝试）；
  // 账户已存在返回 exists=true（不创建）；否则 CREATE USER + GRANT ALL WITH GRANT OPTION
  BootstrapAdminUser(ctx, socketPath, username, password string) (exists bool, err error)
}
RedisBootstrap {
  // 免认证连 localhost:port；CONFIG GET requirepass 已设返回 alreadySet=true；
  // 否则 CONFIG SET requirepass newPassword + CONFIG REWRITE（新密码认证）；bind 经 CONFIG GET 校验本机
  BootstrapRequirepass(ctx, host string, port int, newPassword string) (alreadySet bool, bindLoopback bool, err error)
}
MongoBootstrap {
  // 免认证连 localhost:port（localhost exception）；已有用户/已开鉴权返回 exists=true；
  // 否则 admin 库 createUser username root 角色
  BootstrapAdminUser(ctx, host string, port int, username, password string) (exists bool, err error)
}
```

**约束**：引导通道命令一律参数数组（R-08，MySQL/Redis CLI）/ mongo-driver 结构化凭据（D-15）；密码经 MYSQL_PWD/REDISCLI_AUTH env 注入不进 argv（G-03）；密码明文仅引导建连与加密内存态使用，**BootstrapOutcome.Note/审计 detail/运行日志均不含密码**（I-02 零出口）；服务就绪（start/等待引擎）归 env-mgmt（SysdGateway），本 OHS 不操作进程（service-mgmt 边界）；本 OHS 不占 busy 互斥（D-12 为交互导出/导入 UX 互斥，引导为安装后置机制）。

#### 4.6.1 领域事件与集成

无 MQ、无领域事件。出站：AuditService.Record（instance_bootstrapped，同事务脱敏）。消费方：env-mgmt BootstrapTrigger（进程内 OHS，单向）。仓储复用 InstanceRepository（UpsertDiscovered/UpdateCredentials/GetByIdentity）+ CredentialVault。

### 4.7 MySQL 表级治理（data_table/table_row 实况投影，v2.3 新增）

#### 4.7.1 领域模型

**表结构组合**：无表（domain-model v1.23 D2 判定零表/行台账——事实源在 MySQL/MariaDB 实况，information_schema 与行数据恒经 DbGateway 实时读取，I-04 扩展）。投影对象：data_table（标识=所属实例+库名+表名；字段=表名/引擎/行数估/大小/排序规则/注释/列结构）、table_row（标识=所属实例+库+表+单列主键值；字段=各列值 JSON）。

**不变量**：I-04（零台账）；I-10（单列主键行写约束，D-22）；I-11（行数据 admin 专属，D-24）；表名/库名标识符白名单 quoteString（A-DB3）、引擎侧 SQL 裁定（A-DB10）沿既有口径。

**领域服务**：TableGovernanceService（表清单/结构/行分页/行写/删表编排，复用 DbGateway v2.3 扩展方法 4.1.5 + 单列主键判定 + 审计同事务）。

#### 4.7.2 状态模型 / 4.7.3 子流程设计

无状态机（表/行为无状态实况对象，PRD v1.2 第六部分辨析）；无子流程（行写为单动作过程）。

#### 4.7.4 接口设计（6 接口；POST 隐含 CSRF+admin；mysql 族隐含 I-05 引擎匹配 DBM_018 与 reachable/凭据前置 DBM_003/004）

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /db/table/list | GET | admin+readonly | 表清单（元数据 Q6，实时投影） |
| /db/table/structure | GET | admin+readonly | 表结构（元数据 Q6，含单列主键判定） |
| /db/table/drop | POST | admin | 删表（DROP TABLE 不可逆，强确认前置） |
| /db/row/list | GET | admin | 行数据分页（数据内容 Q6 专属，I-11） |
| /db/row/update | POST | admin | 行编辑（单列主键约束 I-10） |
| /db/row/delete | POST | admin | 行删除（单列主键约束 I-10） |

**接口详情**（DTO 逐形状；**可空字段显式 string|null/number|null 标注——JSON null≠TS undefined，前端类型须 | null，children:null 先例同口径**）：

- **GET /db/table/list**：query `{instanceId, database}`。校验：I-05 前置；库存在 DBM_007（实况）。执行：DbGateway.ListTables（information_schema.TABLES）。响应 data=`{items:[{name:string, engine:string, rowEstimate:number|null, dataSizeBytes:number|null, collation:string|null, comment:string|null}], generatedAt:string}`（rowEstimate=TABLE_ROWS 统计估算——InnoDB 非精确值如实标注估算语义，视图/特殊引擎可空；comment 无注释时 null）。审计：—（查看类，元数据两角色）。
- **GET /db/table/structure**：query `{instanceId, database, table}`。校验：I-05 前置；库 DBM_007/表 DBM_022（实况）。执行：DbGateway.TableStructure（information_schema.COLUMNS）。响应 data=`{table:string, columns:[{column:string, type:string, nullable:boolean, key:string, default:string|null, extra:string, comment:string|null}], singlePk:boolean, pkColumn:string|null, generatedAt:string}`（key=引擎 COLUMN_KEY 原值 ""/PRI/UNI/MUL 开放字符串非枚举；default=COLUMN_DEFAULT 无默认 null；singlePk=恰好 1 列 key=PRI 派生，pkColumn 无单列主键时 null——前端行写入口禁用判定同源，服务端 I-10 为最终守卫）。审计：—。
- **GET /db/row/list**：query `{instanceId, database, table, page?(默认 1), pageSize?(默认 20，上限 100，TABLE_PAGE_LIMIT)}`。校验：I-05 前置；库 DBM_007/表 DBM_022；page/pageSize 形态（AUTH_012，超限截断至上限）。执行：DbGateway.TableRows（JSON_OBJECT 逐行聚合，D-22；offset=(page-1)*pageSize）。响应 data=`{columns:[string], rows:[{[column:string]: string|number|boolean|null}], total:number, page:number, pageSize:number, generatedAt:string}`（columns 来自结构列序；**行值 JSON null=NULL 语义（≠TS undefined）**，类型随列类型 JSON_OBJECT 原生映射；total=COUNT(*) 实况；长文本截断归前端呈现）。审计：—（查看类；**admin 专属——行数据属数据内容取出，I-11/D-24，readonly AUTH_008**）。
- **POST /db/table/drop**：req `{instanceId, database, table}`。校验：I-05 前置；库 DBM_007/表 DBM_022（不静默成功）。执行：DbGateway.DropTable（DROP TABLE，标识符 quoteString A-DB3）。响应 data=`{instanceId, database, table}`。审计：table_dropped（change，detail={instanceId, database, table}；不可逆操作，交互层强制二次确认）。
- **POST /db/row/update**：req `{instanceId, database, table, pkValue:string, values:{[column:string]: string|null}}`。校验：I-05 前置；库 DBM_007/表 DBM_022；**单列主键判定 I-10（无则 DBM_023，data.reason 如实提示）**；values 非空且不含主键列（主键为定位键不可编辑，AUTH_012）；列名 ∈ 结构列集（AUTH_012）；pkValue 非空（AUTH_012）。执行：DbGateway.UpdateRow（WHERE 主键=旧值参数化；values 文本值经 MySQL 隐式类型转换，**null→SET col=NULL**）；affected=0 → DBM_024（行已删除/主键值已变更，不静默成功）。响应 data=`{instanceId, database, table, pkValue, affected:1}`。审计：row_updated（change，detail={instanceId, database, table, pkValue, columns:[列名]}，**不含行值明文**，同事务脱敏）。
- **POST /db/row/delete**：req `{instanceId, database, table, pkValue:string}`。校验：同 row/update（I-10/DBM_023；pkValue 非空）。执行：DbGateway.DeleteRow（WHERE 主键=旧值参数化）；affected=0 → DBM_024。响应 data=`{instanceId, database, table, pkValue, affected:1}`。审计：row_deleted（change，detail={instanceId, database, table, pkValue}，同事务脱敏；交互层确认前置）。

**幂等语义**：table/drop 非幂等（不存在 DBM_022）；row/update、row/delete 按主键旧值定位（匹配 0 行 DBM_024 如实，不静默）；重复提交同一行编辑（值未变）affected=1 成功（引擎语义如实）。防重复提交为前端在途态（P3）。

#### 4.7.5 领域事件与集成

无 MQ、无领域事件（R-09）。出站：AuditService.Record（table_dropped/row_updated/row_deleted，同事务脱敏）。网关：DbGateway v2.3 扩展（4.1.5，dbmysql CLI query() 通道 R-08 参数数组；JSON_OBJECT 构造与 batch 转义还原细节归实现，D-22）。

### 4.8 MongoDB 集合与文档治理（mongo_collection/mongo_document 实况投影 + mdb/dropdb，v2.3 新增）

#### 4.8.1 领域模型

**表结构组合**：无表（domain-model v1.23 零集合/文档台账——事实源在 MongoDB 实况，恒经 MongoGateway 实时读取，I-04 扩展）。投影对象：mongo_collection（标识=所属实例+库名+集合名；字段=集合名/文档数/大小）、mongo_document（标识=所属实例+库+集合+_id；字段=JSON 内容）。

**不变量**：I-04（零台账）；I-11（文档内容 admin 专属，D-24）；I-12（系统库删除保护，D-25）；_id 为文档定位键不可变更（D-23）；JSON 双层校验（前端拦截+服务端 DBM_026，R-14）。

**领域服务**：MongoGovernanceService（集合清单/建删集合/删库/文档分页/CRUD 编排，复用 MongoGateway v2.3 扩展方法 4.5.1 + 审计同事务）。

#### 4.8.2 状态模型 / 4.8.3 子流程设计

无状态机（集合/文档为无状态实况对象）；无子流程（文档 CRUD 为单动作过程）。

#### 4.8.4 接口设计（8 接口；POST 隐含 CSRF+admin；mongo 族隐含 I-05 引擎匹配 DBM_018 与 reachable/凭据前置 DBM_003/004）

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /db/mcoll/list | GET | admin+readonly | 集合清单（元数据 Q6，实时投影） |
| /db/mcoll/create | POST | admin | 建集合（createCollection；隐式建库经此达成，D-25） |
| /db/mcoll/drop | POST | admin | 删集合（dropCollection 不可逆，强确认前置） |
| /db/mdb/dropdb | POST | admin | 删库（dropDatabase 不可逆；系统库保护 DBM_028） |
| /db/mdoc/list | GET | admin | 文档分页（数据内容 Q6 专属，I-11） |
| /db/mdoc/insert | POST | admin | 文档插入（InsertOne，JSON 校验） |
| /db/mdoc/update | POST | admin | 文档编辑（ReplaceOne by _id，JSON 校验） |
| /db/mdoc/delete | POST | admin | 文档删除（DeleteOne by _id，强确认前置） |

**接口详情**（DTO 逐形状；可空字段显式标注，JSON null≠TS undefined）：

- **GET /db/mcoll/list**：query `{instanceId, database}`。校验：I-05 前置（engine=mongodb）；库存在 DBM_007（实况）。执行：MongoGateway.ListCollections。响应 data=`{items:[{name:string, docCount:number, sizeBytes:number}], generatedAt:string}`（docCount=estimatedDocumentCount 估算语义如实；空库返回空列表非错误）。审计：—（查看类，元数据两角色）。
- **POST /db/mcoll/create**：req `{instanceId, database, name}`。校验：I-05 前置；name 集合名形态白名单（R-14，AUTH_012——非空/禁 $ 前缀/禁空字符等引擎规则服务端校验）；同名集合实况校验 DBM_008（语义复用，data.reason）。执行：MongoGateway.CreateCollection（**隐式建库语义：库随首集合写入产生，无独立建库接口，D-25；UI 如实说明归 D4**）。响应 data=`{instanceId, database, name}`。审计：mongo_collection_created（change，detail={instanceId, database, name}）。
- **POST /db/mcoll/drop**：req `{instanceId, database, name}`。校验：I-05 前置；集合存在 DBM_025（不静默成功）。执行：MongoGateway.DropCollection。响应 data=`{instanceId, database, name}`。审计：mongo_collection_dropped（change；不可逆，交互层强制二次确认）。
- **POST /db/mdb/dropdb**：req `{instanceId, database}`。校验：I-05 前置；**系统库 admin/config/local → DBM_028（I-12 服务端硬校验）**；库存在 DBM_007（实况，不静默成功）。执行：MongoGateway.DropDatabase（dropDatabase，连同全部集合与文档移除）。响应 data=`{instanceId, database}`。审计：mongo_database_dropped（change，detail={instanceId, database}；不可逆，交互层强制二次确认）。
- **GET /db/mdoc/list**：query `{instanceId, database, collection, page?(默认 1), pageSize?(默认 20，上限 100，DOC_PAGE_LIMIT)}`。校验：I-05 前置；库 DBM_007/集合 DBM_025；page/pageSize 形态（AUTH_012，超限截断至上限）。执行：MongoGateway.ListDocuments（find skip/limit，skip=(page-1)*pageSize；total=CountDocuments）。响应 data=`{documents:[{[field:string]: any}], total:number, page:number, pageSize:number, generatedAt:string}`（documents 保引擎自然序，**_id 置顶呈现归前端排序**；文档值为 BSON→JSON 映射——ObjectId 呈 `{"$oid":"hex"}` 扩展 JSON 形态透传，日期 `{"$date":...}`，**JSON null≠TS undefined**；嵌套对象/数组如实）。审计：—（查看类；**admin 专属——文档属数据内容取出，I-11/D-24，readonly AUTH_008**）。
- **POST /db/mdoc/insert**：req `{instanceId, database, collection, document:object}`。校验：I-05 前置；库 DBM_007/集合 DBM_025；**document 为合法 JSON 对象（服务端解析校验，非法 DBM_026；前端拦截为先）**；document._id 可选（缺省引擎自动生成 ObjectId）；_id 冲突 → DBM_008 语义复用（data.reason 引擎原文）。执行：MongoGateway.InsertDocument。响应 data=`{instanceId, database, collection, insertedId:any}`（insertedId=插入文档 _id 的 JSON 形态，ObjectId 为 `{"$oid":"hex"}`）。审计：mongo_document_inserted（change，detail={instanceId, database, collection, insertedId}，**不含文档内容明文**，同事务脱敏）。
- **POST /db/mdoc/update**：req `{instanceId, database, collection, id:any, document:object}`。校验：I-05 前置；库 DBM_007/集合 DBM_025；document 合法 JSON（DBM_026）；**id=mdoc/list 响应中 _id 原值透传（ObjectId `{"$oid":"hex"}`/字符串/数值等，网关内还原 BSON）**；document._id 若存在必须与 id 一致（_id 不可变更，不一致 AUTH_012；缺省由服务端以 id 补入）。执行：MongoGateway.ReplaceDocument（ReplaceOne by _id 整体替换，非字段级 patch）；matched=0 → DBM_027（文档已删除/从未存在，不静默成功）。响应 data=`{instanceId, database, collection, id, matched:1}`。审计：mongo_document_updated（change，detail={instanceId, database, collection, id}，不含文档内容明文）。
- **POST /db/mdoc/delete**：req `{instanceId, database, collection, id:any}`。校验：I-05 前置；库 DBM_007/集合 DBM_025；id 形态同 update。执行：MongoGateway.DeleteDocument（DeleteOne by _id）；deleted=0 → DBM_027。响应 data=`{instanceId, database, collection, id, deleted:1}`。审计：mongo_document_deleted（change，detail={instanceId, database, collection, id}；交互层强确认前置）。

**幂等语义**：mcoll/create 非幂等（同名 DBM_008）；mcoll/drop、mdb/dropdb 非幂等（不存在 DBM_025/007）；mdoc/insert _id 冲突报错（缺省 _id 时每次插入为新文档，非幂等语义如实）；mdoc/update、mdoc/delete 按 _id 定位（不存在 DBM_027 如实，不静默）。防重复提交为前端在途态（P3）。

#### 4.8.5 领域事件与集成

无 MQ、无领域事件（R-09，mongo-driver 进程内库非外部集成）。出站：AuditService.Record（mongo_database_dropped/mongo_collection_created/mongo_collection_dropped/mongo_document_inserted/mongo_document_updated/mongo_document_deleted，同事务脱敏）。网关：MongoGateway v2.3 扩展（4.5.1，dbmongo 既有 dial/withClient，D-15）。

## 5. 模块清单汇总

### 5.1 对外服务清单（进程内 OHS/内部服务）

| 服务方法 | 消费方 | 契约要点 |
|---------|--------|---------|
| AuditService.Record(entry)（account-auth OHS，消费） | 本模块 → account-auth | R-03 四要素；同事务；凭据脱敏（D-13）；见 api-account-auth.md 4.3.4 |
| DbGateway.DumpDatabase（预期暴露） | cron-backup（M8，待其 D2/D3 定义） | 数据库备份源复用方向（1.5）；本模块内为唯一实例——导出接口与未来备份任务共用同一网关方法与参数模板（D-06），届时按其模块契约正式登记 |
| InstanceProber / CredentialVault / BackupService | 本模块内部 | 4.1.1 领域服务；busy 任务表 D-12；**v2.3 InstanceProber 两阶段 TCP 优先探测 + rescan 保守清理（D-21）** |
| **TableGovernanceService / MongoGovernanceService（v2.3 新增）** | **本模块内部** | **4.7.1/4.8.1 领域服务：表级治理（单列主键判定 I-10 + JSON_OBJECT 行分页 D-22）与集合文档治理（_id 定位 + JSON 校验 D-23 + 系统库保护 I-12）；审计同事务脱敏（行/文档内容不入 detail）** |
| **BootstrapAfterInstall(ctx, engine, actorName) → BootstrapOutcome（v2.2 提供）** | **env-mgmt（安装后配置就绪 worker 消费）** | **进程内 OHS（§4.6/D-20）：建管理账户/设随机强密码 + 注册 db_instance 与凭据密文（装完即可管）；幂等保守（已配置/已存在跳过不覆盖）；治理通道 TCP 验证后落库（D-03 哲学）；BootstrapOutcome{Status,Note,InstanceID} 回传（Note 不含密码）；审计 instance_bootstrapped 同事务脱敏；引擎引导窄接口注入（零 runner 实况依赖）** |
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

（库/用户/key/慢日志/进程列表零 SQL——引擎实况经 DbGateway 读取，I-04。**v2.3：表/行/集合/文档同为零 SQL——引擎实况经 DbGateway/MongoGateway 读取；rescan 保守清理复用 SQL #5 DELETE（D-21，按 id 参数绑定，同事务审计 instance_merged）；本轮零新增 SQL 语句、零 DDL 变更、零迁移**。）

### 5.3 DDL

**v2.3 声明：DDL 零变更**——表/行/集合/文档为零台账投影（domain-model v1.23），无新表、无列变更、无迁移；db_instance 合一档案表与迁移 011 维持 v2.0 定稿如下。

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
```

**迁移 011_database_mongo.sql（v2.0，engine CHECK 扩值——SQLite 表重建法，010 先例）**：

```sql
-- db_instance.engine CHECK 扩值 +mongodb（enum_value_added db_engine）。
-- SQLite 无 ALTER CHECK：临时表→拷贝→重建→改名（事务内；identity_key 唯一性保持）。
CREATE TABLE db_instance_new (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  identity_key    TEXT    NOT NULL UNIQUE,
  engine          TEXT    NOT NULL CHECK (engine IN ('mysql','mariadb','redis','mongodb')),
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
INSERT INTO db_instance_new SELECT id, identity_key, engine, conn_mode, socket_path, host,
  port, version, conn_state, cred_username, cred_cipher, cred_updated_at, discovered_at, updated_at
  FROM db_instance;
DROP TABLE db_instance;
ALTER TABLE db_instance_new RENAME TO db_instance;
CREATE INDEX IF NOT EXISTS idx_dbinst_engine ON db_instance(engine);
-- I-07 由服务端校验承载（SQLite CHECK 无法跨列表达互斥空语义）
```

## 6. 更新记录

### 6.1 变更记录

| 版本 | 日期 | 变更类型 | 变更摘要 | 目标节号 |
|------|------|---------|---------|---------|
| v1.0 | 2026-11-12 | initial | 首次生成：23 个 HTTP 接口（/db/{instance,mysql,database,dbuser,backup,redis,key}/*：GET 10——其中 readonly 可用 7、admin 专属 GET 3（charsets/key×2）；POST 13=admin+CSRF）+ 1 个进程内 DbGateway 网关契约（dbmysql/dbredis 承载，R-08 参数数组）+ 1 表 DDL（db_instance 合一档案表，D-01）+ 20 错误码（DBM_001~020，复用 AUTH_001/002/003/008/012）+ 4 枚举引用（db_engine/db_conn_mode/instance_conn_state 池中已定稿 + grant_level 本次入池）+ 13 审计动作码；D-01~D-14 设计决策逐项关闭 prd-database-mgmt.md 待对齐-1~-10；无旧版对比 | — |
| v1.1 | 2026-11-12 | interface_changed | Gate 1 F-1 修正（G1-08/M2/GL-009）：key 浏览 2 接口由四段式 `/db/redis/key/{list,ttl}` 改名为三段式 `/db/key/{list,ttl}`，并删除 3.1 中失实的「双级资源/site-mgmt 先例」例外声明（实跑 site-mgmt 20 路径全三段）；接口数 23 不变、方法/权限/语义均不变；同步 admin PageSpec 与 INDEX api note | 3.1 / 3.3 / 4.4.4 / 6.1 |

| v2.0 | 2026-11-17 | interface_added / enum_value_added / table_changed | v0.1.4 用户反馈第 3 项 MongoDB 管理增量（拍板 Q1=A 范围/Q2=A 通道）：+7 接口（mongo/status、mdb/list、muser/{list,create,delete,grant,revoke}——三段式 G1-08；GET 3 两角色+POST 4 admin+CSRF，总 30）；db_engine +mongodb（迁移 011 表重建）；新枚举 mongo_role{read,readWrite,dbAdmin} 入池；D-15~D-19（driver 通道/authSource=admin/探测集 +2/库清单只读/用户三档角色）；复用 DBM_001~021 错误族与既有审计结构（+mongo_user_* 4 动作码）；+dbmongo 适配器（mongo-driver v2 新直接依赖，3.2 登记） | 1.3 / 3.1 / 3.2 / 3.3 / 4.5 / 5.3 / 6.1 |
| v1.2 | 2026-11-17 | rule_added | v0.1.4 用户反馈 bug 修复轮（凭据验证返回裸 INTERNAL）：①错误映射补齐——ErrUnavailable（客户端二进制未检出）新增 DBM_021 承载；credentials Ping 语境未分类引擎失败（ErrQueryFailed/ErrCommandFailed）归 DBM_006（D-03 契约「连接类失败一律业务码」落实）；②tcp 模式连接参数显式 --protocol=TCP（mysql 客户端对 -h localhost 特例走 Unix socket 忽略 -P，tcp 语义被静默改道）；接口无增删改 | 3.4 / 4.1 / 6.1 |
| v2.1 | 2026-11-17 | rule_changed | v0.1.6 后实测反馈 DBM_005 诊断轮（d0-clarification-2026-11-17-dbm005.md）：读码复核确认面板验证链路无缺陷（传参/MYSQL_PWD env 注入/错误映射逐环核验），DBM_005 为服务器真实 1045 拒绝的如实呈现；UX 规则增强——①DBM_005/006 凭据弹窗内联追加 data.reason 服务器原文（此前固定文案丢弃 reason）；②DBM_005 按引擎族呈现可操作排查指引（MySQL 族：账户存在性/host 形态覆盖连接来源/密码一致三则；redis：requirepass/ACL；mongodb：admin 库 authSource）；③DBM_005 文案订正——不再断言「用户名或密码错误」（1045 不区分账户不存在/host 落空/密码不符）；④dbmErrorText toast 路径 DBM_005/006 追加 reason。接口/DTO/错误码语义零变更（D-03 先验证后落库不动）；根因归属待服务器取证裁决 | 3.4 / 6.1 |
| v2.2 | 2026-11-17 | rule_added + event_added（OHS） | 安装后配置就绪联动增量（env-mgmt 主导轮，d0-clarification-2026-11-17-bootstrap.md 拍板 Q1~Q4，api-env-mgmt.md v1.2 消费方）：**新增进程内 OHS BootstrapAfterInstall（§4.6/D-20）供 env-mgmt worker 消费**——建管理账户/设随机强密码 + 注册 db_instance 与凭据密文（装完即可管）；①D-20 引导编排（幂等前置 credentialsSet→skipped / 引导通道建账户〔MySQL socket root auth_socket→dougong@localhost ALL WITH GRANT OPTION、Redis CONFIG SET requirepass+REWRITE、MongoDB localhost exception admin root〕/ 治理通道 TCP 验证 / 加密 UPSERT+审计）；②引擎引导窄接口 MySQLBootstrap/RedisBootstrap/MongoBootstrap（与治理接口分离，注入替身零 runner 实况依赖）；③审计动作码 +instance_bootstrapped（脱敏）；④1.5 上下文映射 +env-mgmt 被消费行；⑤5.1 OHS 清单 +BootstrapAfterInstall。**HTTP 接口/DTO/错误码/DDL 零变更**（复用 db_instance 聚合与 cred_cipher 密文列、CredentialVault、InstanceRepository；无新表无新聚合根）；服务就绪归 env-mgmt（本模块不操作进程）；密码零回显（I-02） | 1.3（D-20）/ 1.5 / 3.4（审计动作码）/ 4.6（新增）/ 5.1 / 6.1 |
| v2.3 | 2026-11-18 | interface_added / rule_changed / rule_added | database-mgmt v1.4 用户实机反馈轮（d0-clarification-database-mgmt-ux-2026-11-18.md 拍板 Q1~Q6，prd v1.2 FR-24~39/domain-model v1.23，关闭待对齐-11）：**①interface_added +14 接口（总 30→44，GET 18/POST 26）**——MySQL 6：table/{list,structure,drop} + row/{list,update,delete}（4.7 新增；行写单列主键约束 I-10，JSON_OBJECT 逐行聚合 D-22，分页默认 20 上限 100）；Mongo 8：mcoll/{list,create,drop} + mdb/dropdb + mdoc/{list,insert,update,delete}（4.8 新增；ReplaceOne/DeleteOne by _id，JSON 双层校验，系统库保护 I-12）；资源段 table/row/mcoll/mdoc 裁定 D-26 全三段式；GET 元数据两角色/数据内容 GET admin 专属/POST admin+CSRF（D-24/I-11，Q6 沿 redis key 先例）；**②rule_changed**——D-18 修订（ADR-031/D-25：mdb/list 只读投影→可管理清单，DTO 零变更）、探测登记策略 TCP 优先两阶段+socket 兜底+rescan 保守清理无凭据冗余 socket 行（D-21：instance/list +redundantSocket:boolean、rescan +merged:n 均 additive，identity_key 语义不变，已配凭据 socket 行保留+标注不做凭据自动迁移）；**③rule_added**——错误码 +DBM_022~028（TABLE_NOT_FOUND/NO_SINGLE_PK/ROW_NOT_FOUND/COLLECTION_NOT_FOUND/INVALID_DOCUMENT_JSON/DOCUMENT_NOT_FOUND/SYSTEM_DATABASE_PROTECTED）、审计动作码 +10（table_dropped/row_updated/row_deleted/mongo_database_dropped/mongo_collection_created/mongo_collection_dropped/mongo_document_inserted/mongo_document_updated/mongo_document_deleted/instance_merged，共 28；行/文档内容不入 detail）、不变量 +I-10~I-12、DbGateway/MongoGateway 扩展方法（4.1.5/4.5.1）、分页常量 TABLE_PAGE_LIMIT/DOC_PAGE_LIMIT；**④DDL 零变更/零迁移/零新表**（4 投影实体零台账）；**⑤零新增枚举**（存储引擎/排序规则实况开放集合沿字符集先例）；3.4 枚举表补正 v2.0 遗漏（db_engine +mongodb/mongo_role 行）；1.5 +panel-core 面包屑呈现层消费（F5 零接口承载，归 D4/panel-core）；F2 tooltip/图例归前端呈现（D4） | 1.1 / 1.2 / 1.3（D-18 修订+D-21~D-26）/ 1.4 / 1.5 / 2.1 / 2.2 / 2.3 / 3.1 / 3.3 / 3.4 / 4.1.4 / 4.1.5 / 4.5.1 / 4.5.2 / 4.7（新增）/ 4.8（新增）/ 5.1 / 5.2 / 5.3 / 6.1 |
| v2.3.1 | 2026-11-18 | rule_added（补记） | Gate 2 E1/AL-005 闭环（A-DB13「统一门控前须同步」项）：§3.4 错误码表补记 DBM_029 ENGINE_UNAUTHORIZED 行——引擎操作被拒（凭据账户权限不足，身份对但无权执行；data.reason=引擎原文；区别 DBM_005 认证被拒）。编号沿革：v0.1.4 修复轮 ErrDBMEngineUnauthorized 原占 DBM_022，与 v2.3 定稿 DBM_022=TABLE_NOT_FOUND 冲突，后端增量轮按 A-DB13 裁定改编号 DBM_029（续接空号不复用，符号/语义不变，errcode/dbmgmt.go + service.go translateGatewayErr 对 dbmongo.ErrUnauthorized code 13 翻译上线，触达 mongo/status、muser/*、v2.3 全部 mongo 治理接口）；本行为设计侧补记（此前 §3.4 缺行致机械 diff BE=29/FE=28）。跨层同步已闭环：前端 types/database.ts DBM_ERROR_TEXT +DBM_029（dbmErrorText 不再 null 静默吞错，reason 追加分支 +029）、MongoCreateUserModal/MongoRoleManageModal 旧 DBM_022 判定/语义映射改判 DBM_029、vitest +4 用例（含 DBM_001~029 全覆盖锚点）。**HTTP 接口/DTO/DDL/枚举零变更**（错误码补记非新码——后端已实发） | 3.4 / 6.1 |

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
