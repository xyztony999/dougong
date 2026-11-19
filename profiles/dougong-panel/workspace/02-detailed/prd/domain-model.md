# 斗拱面板 领域模型（全模块聚合视图）

> 本文件为引擎规范名聚合视图（doc-center.json rich 态数据源，按模块逐次累积）。
> 各模块权威领域建模说明（判定过程、子流程契约、跨域集成论证）见对应 PRD 文档（如 prd-account-auth.md）。
> 累积规则：每完成一个模块的 D2，将其业务域行与领域对象行追加到对应表。
> 表头契约：业务域清单含「上下文名/领域分层/聚合根/核心职责/关联上下文」；领域对象清单含「对象名/分类/所属域/与上级关系/核心业务字段/关键枚举字段/备注」（分类值含"聚合根"者进入聚合根清单）。

| 属性 | 值 |
|------|-----|
| 版本 | v1.26 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-19（v1.26 db-tools 自动 nginx 落地增量 D2：落地引用 siteRef 值对象显式建模 +Port/AutoCreated〔JSON 载荷扩展零 DDL〕，零新聚合根；panel-core 配置变更钩子/HostGuard 豁免集成登记） |

## 第一部分：战略设计（本域判定）

PRD 范围判定：PRD 标注模块名 account-auth → 范围明确。领域分层判定（区别于 AA 的 DDD 四层应用域归类，此处为战略层 核心域/支撑域/通用域）：

| 项目 | 内容 |
|------|------|
| 本域名称 | 账户与安全基座（account-auth） |
| 领域分层 | 支撑域——判定依据：非主差异化卖点（ADR-005 核心卖点=接管已有环境/网站管理），但承载面板特有安全规则（R-01/R-02/R-03、ADR-017~020），含自托管零遥测约束，无法直接采购通用 IAM 方案，故不归通用域；同时是「可信」产品支柱（idea.md §1/§3）的承载者，服务全部核心域 |
| 聚合根 | panel_user、session、audit_log、初始化状态 |

### 面板基座（panel-core）本域判定

PRD 范围判定：PRD 标注模块名 panel-core → 范围明确（跳过 Step 1~3 业务域识别，直接判定本域分层）。

| 项目 | 内容 |
|------|------|
| 本域名称 | 面板基座（panel-core） |
| 领域分层 | 支撑域——判定依据：基础设施模块，非主差异化卖点（核心卖点=存量接管/网站管理），但含面板特有规则（R-09 零外呼、R-10 入口面、ADR-009 可审计安装）无法直接采购通用方案，故不归通用域；服务全部业务模块（交付载体/入口面/外壳/配置权威） |
| 聚合根 | panel_config |

> CLI 管理通道注记（v1.17 增量，ADR-030）：`dougong` 子命令族（status/info/reset-password/set-port/entrypath/service）为服务器终端本地通道，**无新领域对象**——读写复用既有聚合 panel_config（set-port/entrypath/info，经进程内 service 同一配置权威）与 account-auth panel_user（reset-password，环境变量注入口径同初始化，ADR-017 同族）；命令输入输出为过程数据，不入对象清单（权威说明见 prd-panel-core.md FR-19）。

### 系统监控（monitoring）本域判定

PRD 范围判定：PRD 标注模块名 monitoring → 范围明确（跳过 Step 1~3 业务域识别，直接判定本域分层）。

| 项目 | 内容 |
|------|------|
| 本域名称 | 系统监控（monitoring） |
| 领域分层 | 支撑域——判定依据：只读观测能力，非主差异化卖点（核心卖点=存量接管/网站管理，idea.md D5/ADR-005）；但承载监控领域特有口径与留存规则（ADR-014 环形缓冲不落库、⚠️A12 采样/缓冲参数、⚠️A13 聚合口径）并受 <60MB 内存预算约束，无法采购通用方案（监控 SaaS 需外呼，违反 R-09 零外呼），故不归通用域；为全部模块的运行可观测提供数据 |
| 聚合根 | —（无聚合根：metric 条目 A1-A4 全不通过——无一致性边界/无全局引用/无生命周期入口/无状态驱动；本域为只读观测流，无单据无状态机，消费方经读取契约访问缓冲集合，不需要聚合根守门） |

### systemd 服务管理（service-mgmt）本域判定

PRD 范围判定：PRD 标注模块名 service-mgmt → 范围明确（跳过 Step 1~3 业务域识别，直接判定本域分层）。

| 项目 | 内容 |
|------|------|
| 本域名称 | systemd 服务管理（service-mgmt） |
| 领域分层 | 支撑域——判定依据：服务管控为运维面板行业通用能力，非主差异化卖点（核心卖点=存量接管/网站管理，idea.md D5/ADR-005）——「接管存量」哲学在本域的体现是「发现即接管」（R-15 自动纳管）这一接入方式，而非服务管控能力本身的特异性；且含面板特有规则（R-08 dbus 参数化禁 shell、实时投影零持久化、ADR-008 仅 systemd 发行版）无法采购通用方案，故不归通用域；为单机运维高频入口，服务全部登录角色 |
| 聚合根 | —（无聚合根：systemd_service A1-A4 全不通过——面板侧无一致性边界（运行/自启状态的事实源在 systemd 侧，面板仅投影）/无跨域全局引用（本域为服务消费终模块）/无生命周期入口（unit 的装/卸由系统包管理器与 systemd 管理，面板无创建/删除动作）/无域内其他对象受其状态驱动（投影集合内无子对象）；本域为外部状态的投影流，查看经读取契约访问投影集合、操作经动作契约直达 systemd，不需要聚合根守门） |

### 文件管理器（file-manager）本域判定

PRD 范围判定：PRD 标注模块名 file-manager → 范围明确（跳过 Step 1~3 业务域识别，直接判定本域分层）。

| 项目 | 内容 |
|------|------|
| 本域名称 | 文件管理器（file-manager） |
| 领域分层 | 支撑域——判定依据：文件管理为运维面板行业通用能力（通用域典型例「文件服务」），非主差异化卖点（核心卖点=存量接管/网站管理，idea.md D5/ADR-005）——「接管存量」哲学在本域的体现是「直接在受管根操作真实文件系统、不导入不镜像」这一接入方式，而非文件操作能力本身的特异性；且含面板特有安全规则（R-04 两步防逃逸、R-05 原子落盘、R-17 回收站 ADR-021）无法采购通用文件服务方案（外部文件服务需外呼/镜像，违反 R-09 零外呼与零台账承诺），故不归通用域；为单机运维高频入口（里程碑 M2，D0 §2），服务全部登录角色 |
| 聚合根 | trash_entry |

聚合根与对象分类判定（详证见 prd-file-manager.md 第六部分辨析与第九部分过程数据表）：

- **trash_entry → 聚合根**：A1 ✓（一致性边界——trash_entry 记录与 `.dougong-trash/` 内条目内容必须同步一致：还原=条目移回+记录转态、清空=内容删除+记录删除，两半不可分）；A2 △（无跨域引用——消费终模块；但作为本域唯一持久化业务单据被列表/还原/清空直接寻址）；A3 ✓（生命周期入口——删除入站登记 FR-16 是唯一创建入口，还原/清空是唯一终结入口）；A4 ✓（在站→已还原/已清空状态驱动列表呈现与治理动作可用性）。3+ → 聚合根（kb/ADR-021 拍板：落 SQLite 唯一持久化对象）；
- **file_entry → 域内对象-实体（非聚合根）**：A1-A4 全不通过——无面板侧一致性边界（属性事实源在文件系统）/无跨域引用/无生命周期入口（投影随文件系统实况进出，面板不「创建对象」）/无域内对象受其状态驱动。实体（非值对象）：标识=受管根内绝对路径，跨读取稳定、可寻址（编辑/下载/chmod 均按路径寻址同一对象）；属性可变（大小/权限/属主/修改时间随操作与实况变迁为同一对象的属性更新）——与 systemd_service 同构判定；
- **受管根配置 → 域内对象-配置实体**：域级单例（与 account-auth 会话有效期策略同构：域级策略、唯一管理入口、修改记审计），不经聚合根守门（单例无集合无子对象）；R-04 白名单业务边界的载体（⚠️A12）；存储载体与生效机制 D3 定（prd-file-manager.md 待对齐-1）。

### 网站管理（site-mgmt）本域判定

PRD 范围判定：PRD 标注模块名 site-mgmt → 范围明确（跳过 Step 1~3 业务域识别，直接判定本域分层）。

| 项目 | 内容 |
|------|------|
| 本域名称 | 网站管理（site-mgmt） |
| 领域分层 | **核心域**（本项目首个核心域）——判定依据：idea.md D5/ADR-005 核心卖点=接管已有环境，本域是其直接承载（idea.md §3「存量接管（核心壁垒）」第一条即 nginx 站点导入→可视化编辑→安全写回），差异化直接来源于本域的 round-trip 写回能力（nginxx 为 ADR-010 自研核心资产，样本集 ≥ 20 为 M3 验收硬项）；含面板特有规则（R-06 安全写入、R-07 Raw 保留、R-15 存量发现、R-16 隔离写回）无法采购通用方案（竞品调研 §9.2：主流建站面板无一支持存量导入，「存量接管+现代技术栈」组合无占位者），故不归通用域；ACME 外部 CA 对接经 ACL 防腐（本域为核心域不被外部模型污染） |
| 聚合根 | certificate、dns_credential（v1.20 增量） |

聚合根与对象分类判定（详证见 prd-site-mgmt.md 第六部分辨析与第九部分过程数据表）：

- **certificate → 聚合根**：A1 ✓（一致性边界——证书记录 ↔ 面板托管文件 ↔ 站点部署引用三者必须一致：续期成功=文件替换+状态回 valid+重部署，三半不可分）；A2 ✓（全局引用——被站点配置 ssl_cert 引用、被证书清单/站点列表证书状态列寻址）；A3 ✓（生命周期入口——ACME 签发（C-08）与存量识别导入（C-10-01）是唯一创建入口，证书删除（C-10-03）是唯一终结入口）；A4 ✓（cert_status 六状态机 ADR-024 拍板，驱动续期调度与清单呈现）。4/4 → 聚合根；**落 SQLite**——ACME 续期任务状态（挑战方式/账户/到期跟踪）无法从配置文件推导，必须面板自持（本模块唯一持久化对象）；
- **dns_credential → 聚合根（v1.20 增量，⚠️A20/Gate 1 改判）**：A1 ✓（一致性边界——凭证记录 ↔ 凭据密文 ↔ 引用证书的续期通道三者必须一致：新建=密文落库即刻可用、删除=引用证书续期通道失效如实呈现，两半不可分；**凭据密文无法从任何外部状态推导，必须面板自持**，与 certificate「续期任务状态无法从配置文件推导」/db_instance 凭据同构论证）；A2 ✓（域内全局引用——certificate.dns_credential_id 引用、证书清单/申请弹窗按 id 寻址）；A3 ✓（生命周期入口——凭证新建/删除（C-08-05）是唯一创建/终结入口）；A4 ✗（无状态机——存在即有效，校验态不缓存〔每次签发时经验证〕）。3 项通过且 A1 通过 → 聚合根，**落 SQLite**（凭据材料 AES-256-GCM 密文列+独立密钥文件 dns-cred.key 0600——沿 database-mgmt CredentialVault 先例；明文永不回显）；与 certificate 为同域两个独立聚合（凭证与证书各自一致性边界；证书续期消费凭证为同域聚合引用——backup_run 按 job 归组同构）；
- **site → 域内对象-实体（非聚合根）**：A1-A4 全不通过——无面板侧一致性边界（配置文件是唯一事实源，结构化字段为解析投影）/无跨域聚合引用（站点状态列消费 certificate 为同域读取，非 ID 引用）/无生命周期入口（导入与新建产出的都是配置文件，站点对象随文件实况进出投影，面板不「创建对象」）/无域内对象受其状态驱动（enabled 为配置侧状态投影）。实体（非值对象）：标识=来源文件内 server 块定位（跨扫描稳定、可寻址，稳定键形态 D3 定，prd-site-mgmt.md 待对齐-1）；属性可变（编辑/启停后重新解析仍为同一站点对象的属性更新）——与 systemd_service/file_entry 同构判定；**零站点台账不落库**（D2 判定：配置文件是唯一事实源，面板外修改自动反映，与 file-manager「不导入不镜像」同哲学；持久化台账反而制造镜像漂移风险）；
- **反代上游（upstream）→ 值对象**：无独立标识（值相同即等价）、不可变只能替换（改上游=替换值）——D0 留 D1/D2 的「是否独立对象」事项关闭：site 内嵌结构（D1 落定），不独立建模；
- **Raw 未识别指令块 / 写回落点映射 → 值对象**：Raw 块为解析保留的原文本（无标识、替换式更新，R-07 零丢失承诺的载体）；写回落点映射为扫描派生的「站点→来源文件」关系（⚠️A13，随投影派生、无独立生命周期）。

### 数据库管理（database-mgmt）本域判定

PRD 范围判定：PRD 标注模块名 database-mgmt → 范围明确（跳过 Step 1~3 业务域识别，直接判定本域分层）。

| 项目 | 内容 |
|------|------|
| 本域名称 | 数据库管理（database-mgmt） |
| 领域分层 | 支撑域——判定依据：数据库管理为运维面板行业通用能力（宝塔核心功能之一），非主差异化卖点（核心卖点=存量接管/网站管理，idea.md D5/ADR-005）——「接管存量」哲学在本域的体现是「发现即接管、不要求重装」（R-15）这一接入方式，而非数据库治理能力本身的特异性；且含面板特有规则与安全约束（R-08 参数数组纪律、⚠️A18 凭据密文不回显、改密后面板凭据同步、ADR-007 范围边界）无法采购通用方案（外部数据库管理服务需外呼/云托管，违反 R-09 零外呼），故不归通用域；为单机运维高频入口（里程碑 M4，验收硬项含「所有操作可审计」），服务全部登录角色 |
| 聚合根 | db_instance、redis_instance |

聚合根与对象分类判定（详证见 prd-database-mgmt.md 第六部分辨析与第九部分过程数据表）：

- **db_instance → 聚合根**：A1 ✓（一致性边界——实例档案与凭据密文必须一致：root 密码修改成功→面板侧密文同步更新，两半不可分；**凭据密文无法从实例实况推导，必须面板自持**，与 certificate「续期任务状态无法从配置文件推导」同构论证）；A2 △（域内全局入口——database/db_user/key 全部经实例标识寻址访问「查库先定位实例」；跨域引用现阶段无，cron-backup 备份源为预期方向不预先建模）；A3 ✓（生命周期入口——纳管登记（探测+凭据配置验证）是唯一创建入口，移除纳管（⚠️A20）是唯一终结入口）；A4 △（连接状态为探测态非状态机，其呈现与操作可用性为派生语义）。按判定规则（2 项及以上通过且 A1 通过）→ **聚合根，落 SQLite**；
- **redis_instance → 聚合根**：与 db_instance 同构判定（A1 ✓ 密码设置成功→密文同步 / A3 ✓ 纳管与移除为唯一生命周期入口 / A2 △ key 浏览经实例标识寻址）；与 db_instance 无共享一致性边界（两引擎档案与凭据各自独立），故为**独立聚合根**而非合一对象；落 SQLite；
- **database → 域内对象-实体（非聚合根）**：A1-A4 全不通过——无面板侧一致性边界（库的事实源在 MySQL 实况，面板为投影与操作通道）/无跨聚合引用（经实例标识寻址）/无生命周期入口（面板建库删库为通道之一，mysql client 亦可，库随实况进出投影）/无域内对象受其状态驱动。实体（非值对象）：标识=所属实例+库名（跨读取稳定、可寻址）；属性可变（字符集调整后仍为同一库对象的属性更新）——与 site/file_entry/systemd_service 同构判定；**零库台账不落库**（D2 判定：实例实况是唯一事实源，面板外建库自动反映，与 site 零站点台账/file-manager 不导入不镜像同哲学）；
- **db_user → 域内对象-实体（非聚合根）**：与 database 同构判定（A1-A4 全不通过；标识=所属实例+用户名+host；密码只写不读）；删除时既有授权一并回收（引擎语义）；零台账实时投影；
- **凭据 → 值对象**：内嵌于实例聚合根——无独立标识（用户名+密码值相同即等价）、不可变只能替换（改密=替换凭据值，root 密码修改/Redis 密码设置的「同步」即替换语义）；⚠️A18 密文落库、明文仅内存态建连、不回显不入审计明文；密钥来源与列形态归 D3（prd-database-mgmt.md 待对齐-1）。

v1.23 增量判定（database-mgmt v1.4 用户实机反馈轮，D0 Q1~Q6，权威说明 prd-database-mgmt.md v1.2 4.9~4.11）：**零新聚合根**——凭据与档案仍 db_instance/redis_instance 聚合（一致性边界不变）；表/行/集合/文档为实例内实况投影实体（与 database/db_user 同构判定，零台账不落库）：

- **data_table（数据表）→ 域内对象-实体（非聚合根）**：A1-A4 全不通过——无面板侧一致性边界（表的事实源在 MySQL/MariaDB 实况，面板为投影与操作通道）/无跨聚合引用（经实例+库寻址）/无生命周期入口（面板删表为通道之一，mysql client 亦可，表随实况进出投影；本轮无建表）/无域内对象受其状态驱动。实体（非值对象）：标识=所属实例+库名+表名（跨读取稳定、可寻址）；属性可变（行数/大小/注释随实况变迁）；列结构为其元数据字段（information_schema.COLUMNS 投影，字段级不单独建模——D2 对象级判定粒度）；**零表台账不落库**（D2 判定沿零库台账哲学）；
- **table_row（数据行）→ 域内对象-实体（非聚合根）**：A1-A4 全不通过（同 data_table；行随引擎侧增删改进出投影）。实体（非值对象）：标识=所属实例+库+表+**单列主键值**（行写仅对单列主键表开放——无单列主键的表无稳定行标识，行编辑/删除禁用并如实提示，Q3 拍板）；属性=各列值（JSON 内容，数据内容 admin 专属 Q6）；行写按主键旧值定位（WHERE 主键=旧值，参数化/quoteString A-DB3）；**零行台账不落库**（数据内容读取即投影、响应即消亡，写为引擎侧动作）；
- **mongo_collection（集合）→ 域内对象-实体（非聚合根）**：A1-A4 全不通过（事实源在 MongoDB 实况；面板 createCollection/dropCollection 为通道之一，mongo shell 亦可；集合随实况进出投影）。实体（非值对象）：标识=所属实例+库名+集合名；属性=文档数/大小（实况投影）；**零集合台账不落库**；隐式建库语义——库随首集合写入产生（ADR-031：显式建集合达成建库，无独立建库动作）；
- **mongo_document（文档）→ 域内对象-实体（非聚合根）**：A1-A4 全不通过（同 mongo_collection）。实体（非值对象）：标识=所属实例+库+集合+**_id**（跨读取稳定、ReplaceOne/DeleteOne 按其寻址同一文档）；属性=JSON 内容（_id 置顶呈现，数据内容 admin 专属 Q6）；插入 InsertOne/编辑 ReplaceOne/删除 DeleteOne 均引擎侧动作；**零文档台账不落库**；
- **database 实体备注修订（v1.23，ADR-031）**：mongodb 项由「只读投影、无建/删动作」改为「可管理清单——浏览集合入口+删库 dropDatabase（admin），无独立建库动作（隐式建库经显式建集合达成）」；MySQL 项+浏览入口（表级治理寻址前缀）。对象分类不变（仍域内对象-实体）。

### Web 终端（terminal）本域判定

PRD 范围判定：PRD 标注模块名 terminal → 范围明确（跳过 Step 1~3 业务域识别，直接判定本域分层）。

| 项目 | 内容 |
|------|------|
| 本域名称 | Web 终端（terminal） |
| 领域分层 | 支撑域——判定依据：Web 终端为运维面板行业通用能力（ttyd/gotty 等通用方案存在），非主差异化卖点（核心卖点=存量接管/网站管理，idea.md D5/ADR-005）；但与面板安全基座深度耦合（R-18 会话鉴权复用+失效联动、ADR-018 角色禁用、ADR-023 限流标准档），通用终端方案无法直接采购（无面板会话/角色/联动语义），故不归通用域；为单机运维高频入口（里程碑 M5 验收硬项），服务管理员（admin 专属） |
| 聚合根 | terminal_session |

聚合根与对象分类判定（详证见 prd-terminal.md 第六部分辨析与第九部分过程数据表）：

- **terminal_session → 聚合根**：A1 ✓（一致性边界——会话记录 ↔ PTY 进程 ↔ 并发配额三者同生共死：断开=进程回收+记录销毁+配额释放，任一半失败即遗留孤儿 shell 或永久堵塞后续准入）；A2 △（域内集合入口——并发准入与联动断开以「操作者+存续会话集合」寻址；跨域引用现阶段无，docker-mgmt 容器内终端为二期预期方向不预先建模）；A3 ✓（生命周期入口——三重准入流程（鉴权→并发→身份）是唯一创建入口，统一断开清理是唯一终结入口）；A4 ✓（存续状态驱动空闲判定输入源、面板会话失效联动、并发准入与新连接反馈）。3+ → 聚合根；**内存态不落库（D0 拍板）**——全项目唯一「聚合根+零持久化」组合（聚合边界在领域层不由存储形态决定；对照 certificate「落 SQLite 因续期状态必须面板自持」——终端会话无跨重启留存语义，「断开即无痕」即产品语义）；
- **运行身份 → 值对象**：内嵌于 terminal_session——无独立标识（来源+用户名值相同即等价）、不可变只能替换（会话期间不变更，换身份=新建会话，D1 落定）；承载 R-18 两分支（面板运行用户 / 指定系统用户）与 su 语义不代输密码口径；
- **限流策略 → 值对象**：域级固定策略——并发 5 / 空闲 30min / 硬上限 8h（ADR-023 拍板标准档，MVP 固定不可配置，对照 account-auth 会话有效期策略 ADR-019 明示可配置的差异）；空闲从严口径（以无输入为准、输出不重置计时）为策略内规则。

### 防火墙（firewall）本域判定

PRD 范围判定：PRD 标注模块名 firewall → 范围明确（跳过 Step 1~3 业务域识别，直接判定本域分层）。

| 项目 | 内容 |
|------|------|
| 本域名称 | 防火墙（firewall） |
| 领域分层 | 支撑域——判定依据：防火墙端口规则管理为运维面板行业通用能力（宝塔核心功能之一），非主差异化卖点（核心卖点=存量接管/网站管理，idea.md D5/ADR-005）——「接管存量」哲学在本域的体现是「识别既有 ufw/firewalld 后端、不安装不替换」（R-15）这一接入方式，而非规则治理能力本身的特异性；且含面板特有规则与安全约束（面板端口保护硬项 M7 明示、R-08 命令参数数组纪律、firewalld 永久+运行态双写口径、无后端不得呈现为无规则的降级语义）无法采购通用方案（外部防火墙管理服务需外呼/云托管，违反 R-09 零外呼），故不归通用域；为单机运维安全入口（里程碑 M5 验收硬项：ufw 与 firewalld 各自通过规则增删测试），服务全部登录角色 |
| 聚合根 | —（无聚合根：firewall_rule A1-A4 全不通过——无面板侧一致性边界（规则事实源在 ufw/firewalld 后端侧，面板为投影与操作通道）/无跨域全局引用（本域为消费终模块，无对外契约）/无生命周期入口（面板添加/删除为通道之一，管理员亦可经服务器本地直接增删，规则随实况进出投影）/无域内对象受其状态驱动（无状态机，D0 拍板「规则有无即状态」）；本域为外部状态的投影流，查看经读取契约访问投影集合、操作经动作契约直达后端，不需要聚合根守门——与 monitoring/service-mgmt 同构判定） |

聚合根与对象分类判定（详证见 prd-firewall.md 第六部分辨析与第九部分过程数据表）：

- **firewall_rule → 域内对象-实体（非聚合根）**：A1-A4 全不通过（见上）；实体（非值对象）：标识=后端侧规则定位（ufw 规则编号/位置、firewalld 规则定位——稳定标识形态 D3 定，prd-firewall.md 待对齐-2；跨读取稳定、可寻址，删除操作按其寻址同一对象）；属性随实况变迁（面板外增删后重新投影仍按标识对位）——与 systemd_service/file_entry/database 同构判定（外部实况投影实体，面板为通道之一非唯一入口）；**零规则台账不落库**（D2 判定：后端实况是唯一事实源，面板外增删自动反映，与 site 零站点台账/database-mgmt 零库用户台账同哲学）；监听端口清单条目与开放状态对照结论为系统实况只读投影+读取时派生值（响应即消亡，与 database-mgmt 慢日志条目同口径），不单独建模为领域对象；来源限制（CIDR）与端口段为规则字段（对象级判定，字段归 D3），不独立建模。

### 容器管理（docker-mgmt）本域判定

PRD 范围判定：PRD 标注模块名 docker-mgmt → 范围明确（跳过 Step 1~3 业务域识别，直接判定本域分层）。

| 项目 | 内容 |
|------|------|
| 本域名称 | 容器管理（docker-mgmt） |
| 领域分层 | 支撑域——判定依据：容器管理为运维面板行业通用能力（宝塔/1Panel/DPanel/CasaOS 均有，idea.md §9 竞品调研），非主差异化卖点（核心卖点=存量接管/网站管理，idea.md D5/ADR-005）——「接管存量」哲学在本域的体现是「连接既有 daemon、已有容器/镜像/卷/网络/Compose 项目自动纳入、不重装不重建」（R-12/R-15）这一接入方式，而非容器治理能力本身的特异性（AA 层 C-12 容器生命周期管理归核心业务域为战术建模深度判定，与 service-mgmt C-04/C-05 先例一致，不改变本域战略分层）；且含面板特有规则与边界（R-12 不做应用商店的路线切割、R-13/ADR-013 label 识别 + CLI 参数数组编排、ADR-025 状态映射与动作矩阵、daemon 拒绝为删除约束最终权威）无法采购通用方案（通用容器面板如 Portainer 为独立重型部署，与单二进制 <60MB 轻量承诺冲突，且不带面板会话/RBAC/审计语义），故不归通用域；为单机运维容器入口（§4.1 M9），服务全部登录角色 |
| 聚合根 | —（无聚合根：container/image/volume/network/compose_project A1-A4 均不通过——无面板侧一致性边界（全部对象事实源在 Docker daemon 侧，面板为投影与操作通道，与 systemd_service/firewall_rule 同构）/无跨域全局引用（本域为消费终模块；terminal 容器内终端为二期预期方向不预先建模）/无生命周期入口（容器/镜像创建经 docker run/Compose/构建等面板外通道，面板动作集不含创建（OUT）；删除亦非唯一终结通道——docker CLI 亦可删）/无域内对象受面板侧状态驱动（状态映射矩阵为 daemon 实况的动作可用性派生语义）；本域为外部状态投影流，查看经读取契约访问投影集合、操作经动作契约直达 daemon，不需要聚合根守门——与 monitoring/service-mgmt/firewall 同构，差异在对象有稳定标识且被生命周期与删除操作寻址（实体而非值对象） |

聚合根与对象分类判定（详证见 prd-docker-mgmt.md 第六部分辨析与第九部分过程数据表）：

- **container → 域内对象-实体（非聚合根）**：A1-A4 全不通过（见上；A3 关键：容器创建不在面板动作集，docker run/Compose 等面板外通道皆可创建，面板非唯一生命周期入口）。实体（非值对象）：标识=容器 ID/名称（跨读取稳定、可寻址，启停/暂停/删除/日志均按其寻址同一对象）；属性可变（状态随 daemon 实况变迁为同一对象的属性更新）——与 systemd_service/firewall_rule/file_entry 同构判定；**零容器台账不落库**（D2 判定：daemon 实况是唯一事实源，面板外变更自动反映，与 site 零站点台账/database-mgmt 零库用户台账/firewall 零规则台账同哲学）；状态映射与动作矩阵唯一权威口径=BA 附录B（ADR-025）；
- **image / volume / network → 域内对象-实体（非聚合根）**：与 container 同构判定（A1-A4 全不通过；标识=镜像 ID/卷名/网络 ID，删除操作按其寻址）；**零台账不落库**；删除约束以 daemon 拒绝为最终权威（被引用/被挂载/被接入即拒绝，无前置推导无强制绕过，D1 口径-4）；预定义网络不可删如实呈现；
- **compose_project → 域内对象-实体（非聚合根）**：label 聚合派生投影（kb 拍板不落库）——A1-A4 全不通过（事实源在容器 label 与 daemon 实况，面板无登记动作；up/down/restart 为直达 CLI 的操作通道非生命周期唯一入口）；实体（非值对象）：标识=项目名（label 聚合键，跨扫描稳定，up/down/restart/logs 按其寻址）；容器清单为对 container 的同域引用集合（非跨域引用）；无自有状态机（项目运行态=容器组实况汇总呈现）；定位失败（工作目录/配置文件缺失）为判定态非状态机（D1 口径-2）；
- **daemon 接管结论（docker_daemon_state：available/unavailable）**：域级判定值——每次探测整值刷新（与 firewall_backend/instance_conn_state 同构），驱动未装引导降级判定（FR-02）与页头呈现，不构成对象状态字段；
- **资源占用快照 / 日志流内容 / 拉取与操作在途**：读取时派生值与一次性执行过程（P2/P7 过程数据，与 firewall 监听端口对照/service-mgmt 操作任务同口径），不建模为领域对象。

### 计划任务与备份（cron-backup）本域判定

PRD 范围判定：PRD 标注模块名 cron-backup → 范围明确（跳过 Step 1~3 业务域识别，直接判定本域分层）。

| 项目 | 内容 |
|------|------|
| 本域名称 | 计划任务与备份（cron-backup） |
| 领域分层 | 支撑域——判定依据：计划任务与备份为运维面板行业通用能力（宝塔/1Panel 均有），非主差异化卖点（核心卖点=存量接管/网站管理，idea.md D5/ADR-005）——「接管存量」哲学在本域的体现是「root crontab 读写接管、既有任务自动纳入」（R-15 精神）与「备份源联动既有站点/数据库清单」这一接入方式，而非定时/备份能力本身的特异性（AA 层 C-13 备份执行与历史管理归核心业务域为战术建模深度判定——backup_run 状态机+聚合根+R-11 轮转，与 service-mgmt C-04/C-05、docker-mgmt C-12 先例一致，不改变本域战略分层）；且含面板特有规则与口径（R-11 保留轮转、ADR-016 backup_run 状态机、R-08 手动触发边界口径、备份调度器与系统 cron 解耦不写 crontab、cron 接管边界、执行互斥/错过不补跑）无法采购通用方案（通用备份 SaaS/计划任务服务需外呼/云托管，违反 R-09 零外呼），故不归通用域；为单机运维自动化与数据保护入口（里程碑 M8），服务全部登录角色 |
| 聚合根 | backup_run、backup_job |

聚合根与对象分类判定（详证见 prd-cron-backup.md 第六部分辨析与第九部分过程数据表）：

- **backup_run → 聚合根**：A1 ✓（一致性边界——记录 ↔ 产物 ↔ 轮转水位三者必须一致：执行成功=记录转 success+产物落盘+按保留份数轮转（记录与产物同删），两半不可分；**执行事实与产物台账无法从外部推导，必须面板自持**，与 certificate「续期任务状态无法从配置文件推导」同构论证）；A2 ✓（全局引用——备份历史/详情/产物下载按 run 标识直接寻址；任务清单「最近执行结论」按任务归组取最新回显）；A3 ✓（生命周期入口——定时调度执行（C-13-01）与立即执行（C-13-02）是唯一创建入口，终态回写+轮转清理是唯一终结入口）；A4 ✓（ADR-016 状态机 running→success/failure 驱动历史呈现、产物下载可用性〔仅 success〕与轮转触发）。4/4 → 聚合根，落 SQLite（D0 拍板）；
- **backup_job → 聚合根**：A1 ✓（一致性边界——任务定义整体〔源清单+目标+保留份数+调度〕为不可分的完整单据，调度器消费的必须是完整定义，编辑=整体替换）；A2 ✓（任务清单与调度器按任务标识寻址，backup_run 按其归组）；A3 ✓（任务创建/编辑/删除是唯一定义与终结入口）；A4 ✗（无状态机——无启停开关〔OUT〕，「最近执行结论」为 run 回显非任务状态字段）。3 项通过且 A1 通过 → 聚合根，落 SQLite（D0 拍板）；与 backup_run 为**两个独立聚合**（定义单据与执行事实各自一致性边界；轮转保留份数为跨聚合治理规则〔job 定义策略、run 集合执行轮转〕，run 按 job 标识归组为同域聚合引用——与 site-mgmt「site 与 certificate 同域各自聚合」同构）；
- **cron_job → 域内对象-实体（非聚合根）**：A1-A4 全不通过——无面板侧一致性边界（crontab 文件是唯一事实源，面板为投影与读写通道）/无跨域引用（消费终模块）/无生命周期入口（crontab -e 等面板外通道皆可增删任务，面板非唯一入口）/无域内对象受其状态驱动（无状态机，任务存在即状态）。实体（非值对象）：标识=crontab 内任务行定位（跨读取可寻址，编辑/删除/手动触发按其寻址同一对象；稳定形态 D3 定）——与 systemd_service/firewall_rule/file_entry 同构判定（外部实况投影实体，面板为通道之一非唯一入口）；**零任务台账不落库**（D2 口径-1：crontab 文件是唯一事实源，面板侧备注与来源标注以 crontab 注释行承载、面板外编辑如实反映——与 site 零站点台账/firewall 零规则台账/database-mgmt 零库用户台账同哲学）；
- **备份源清单 / 备份目标 → 值对象**：内嵌于 backup_job——无独立标识（同型同址即等价）、不可变只能替换（编辑任务=整体替换定义）；源清单承载本域唯一关联领域对象载体（database 项=db_instance 引用+库名；站点目录项=路径值快照，site 无聚合根 ID 不构成持续引用，见第五部分）；rsync 目标认证凭据承载待 D3（若需按密文值对象与 db_instance 凭据同构，prd-cron-backup.md 待对齐-6）。

### 环境管理（env-mgmt）本域判定（v1.19 追加）

PRD 范围判定：PRD 标注模块名 env-mgmt → 范围明确（跳过 Step 1~3 业务域识别，直接判定本域分层）。

| 项目 | 内容 |
|------|------|
| 本域名称 | 环境管理（env-mgmt） |
| 领域分层 | 支撑域——判定依据：运行环境供给为运维面板行业通用能力（宝塔软件商店/1Panel 应用商店均有的裁剪形态），非主差异化卖点；且含面板特有规则与口径（apt/dnf·yum 双系探测 ⚠️A2、异步任务互斥与重启恢复 ⚠️A3、升级幂等 ⚠️A4、卸载保守语义 ⚠️A6、编译期包名/单元名映射 ⚠️A7、R-08 包操作参数数组纪律）无法采购通用方案（通用方案需外呼软件源元数据服务，违反 R-09 零外呼精神——本域仅经发行版本地包管理器通道），故不归通用域；为单机环境供给入口（v0.1.2 用户反馈第 4 项补位），服务全部登录角色（AA 层 C-14 环境安装与变更归核心业务域为战术建模深度判定——env_install_task 状态机+聚合根+审计单据，与 service-mgmt C-04/C-05、docker-mgmt C-12、cron-backup C-13 先例一致，不改变本域战略分层） |
| 聚合根 | env_install_task |

聚合根与对象分类判定（详证见 prd-env-mgmt.md 第六部分辨析与第九部分过程数据表）：

- **env_install_task → 聚合根**：A1 ✓（一致性边界——任务记录 ↔ 执行进程 ↔ 同环境互斥约束三者必须一致：任务创建=台账落库+互斥生效（同事务原子），终态=结果落库+互斥释放，两半不可分；**执行事实与输出台账无法从包管理器状态完整推导**（已装≠面板装的成功记录），必须面板自持，与 certificate「续期任务状态无法从配置文件推导」/backup_run「执行事实无法从外部推导」同构论证）；A2 ✓（域内清单与详情按 taskId 直接寻址；环境列表在途标记按 env_type 归组派生）；A3 ✓（生命周期入口——安装/升级/卸载动作（C-14-01/02）是唯一创建入口，终态回写是唯一终结入口〔面板重启恢复 running→failed 亦为终态化路径〕）；A4 ✓（四态状态机 pending→running→succeeded/failed ⚠️A3 驱动任务详情呈现、列表在途标记与按钮态）。4/4 → 聚合根，**落 SQLite**（D2 落定：任务台账持久化——跨重启可观测、互斥跨重启一致；与 backup_run 台账同构）；
- **环境条目（env entry）→ 域内对象-实体（非聚合根）**：A1-A4 全不通过——无面板侧一致性边界（包管理器数据库与 systemd 为事实源，面板为探测投影）/无跨域引用（消费终模块，仅软引导被消费）/无生命周期入口（apt/yum 与 shell 等面板外通道皆可装卸环境，面板非唯一入口）/无域内对象受其状态驱动（已装/服务状态为实况投影非状态机）。实体（非值对象）：标识=env_type 目录键（八值编译期枚举，v1.24 +node；v1.19 原文「六值」为滞后，v1.22 七值未回填本处，本轮一并订正），跨请求稳定可寻址（服务操作/变更发起按其寻址；**node 为运行时型环境无服务单元——服务态字段不适用 null，服务操作不在其动作集，⚠️A17**）——与 systemd_service/site/firewall_rule 同构判定（外部实况投影实体，面板为通道之一非唯一入口）；**零环境台账不落库**（已装/版本/服务状态每次请求实时探测，与 site 零站点台账同哲学；目录条目本体为编译期常量随代码分发）；
- **发行版映射（distro mapping）→ 值对象**：内嵌于环境条目（编译期）——每环境每发行版族的包名、systemd 单元名、版本查询方式（⚠️A7）；无独立标识（环境+族唯一确定）、不可变（随面板版本更新）；非用户可配数据。**v1.24 +node 条目**：包名 nodejs（三系同）、单元映射空（运行时型 ⚠️A17）、版本查询 node --version、需官方仓库源=true（NodeSource 三系代配 ⚠️A16，LTS 大版本轨道钉死，npm 随包）。

### 数据库工具集成（db-tools）本域判定（v1.24 追加）

PRD 范围判定：PRD 标注模块名 db-tools → 范围明确（跳过 Step 1~3 业务域识别，直接判定本域分层）。

| 项目 | 内容 |
|------|------|
| 本域名称 | 数据库工具集成（db-tools） |
| 领域分层 | 支撑域——判定依据：数据库高级管理工具集成为运维面板行业通用能力（宝塔 phpMyAdmin 集成先例），非主差异化卖点（核心卖点=存量接管/网站管理，idea.md D5/ADR-005）——本域差异化在「集成现成工具+面板会话 gating 安全桥接」的接入方式而非工具本体（工具为外部现成软件 phpMyAdmin/mongo-express，ACL 封装）；且含面板特有安全规则（localhost 绑定 ⚠️A6、auth_request 会话 gating ⚠️A7、signon 令牌防伪与凭据零回显 ⚠️A8/A9/I-02、卸载不删数据 ⚠️A5、sha256 强制校验 ⚠️A12、R-08/R-03/R-09 纪律）无法采购通用方案（通用工具集成方案不带面板会话/RBAC/审计/凭据 vault 语义，且公网直暴露违反安全底线），故不归通用域；为数据库高级管理入口（ADR-032 用户拍板 2026-11-19），服务管理员（工具访问 admin 专属 ⚠️A10）与全部登录角色（目录/任务查看面 ADR-018）（AA 层 C-15 工具部署与变更归核心业务域为战术建模深度判定——tool_deploy_task 状态机+聚合根+审计单据，与 env-mgmt C-14、cron-backup C-13 先例一致，不改变本域战略分层） |
| 聚合根 | db_tool、tool_deploy_task |

聚合根与对象分类判定（详证见 prd-db-tools.md 第六部分辨析与第九部分过程数据表）：

- **db_tool（工具部署档案）→ 聚合根**：A1 ✓（一致性边界——部署档案 ↔ 工具凭据密文 ↔ 部署落位（站点/服务引用）必须一致：安装成功=档案登记+凭据密文落库同事务原子，卸载=档案+密文删除+部署解除，轮换=密文替换+注入配置更新，两半不可分；**工具凭据密文（basic auth 口令/signon 密钥）无法从任何外部状态推导，必须面板自持**，与 db_instance「凭据密文无法从实例实况推导」/dns_credential 同构论证）；A2 ✓（域内全局入口——gating/signon/打开/轮换/升级/卸载全部经工具标识寻址；跨域被消费：database-mgmt 高级管理入口经读契约消费工具状态与打开能力，不持有持久 ID 引用）；A3 ✓（生命周期入口——工具安装（C-15-01）是唯一创建入口〔面板托管目录部署，面板外无登记通道〕，卸载（C-15-03）是唯一终结入口）；A4 ✓（档案存在性驱动打开/升级/卸载入口可用性与未装引导；已装版本驱动升级判定）。4/4 → 聚合根，**落 SQLite**（D2 落定：凭据密文必须面板自持；部署档案为面板托管部署的记录——区别于环境条目零台账〔apt 为多通道事实源〕，工具部署为面板专属通道且凭据不可推导）；**运行态/依赖就绪度为实况投影字段不持久化**（mongo-express systemd 实况/phpMyAdmin 站点可用性/env-mgmt 环境实况消费，与 db_instance 连接状态探测态同口径）；
- **tool_deploy_task（工具部署任务）→ 聚合根**：A1 ✓（一致性边界——任务记录 ↔ 执行进程 ↔ 同工具互斥约束三者必须一致：创建=台账落库+互斥生效同事务原子，终态=结果落库+互斥释放+部署档案登记/更新同事务；**执行事实与输出台账无法从部署实况完整推导**〔已装≠面板装的成功记录〕，必须面板自持，与 env_install_task/backup_run 同构论证）；A2 ✓（域内清单与详情按 taskId 直接寻址；工具列表在途标记按 toolType 归组派生）；A3 ✓（生命周期入口——安装/升级/卸载动作（C-15-01/02/03）是唯一创建入口，终态回写是唯一终结入口〔面板重启恢复 running→failed 亦为终态化路径〕）；A4 ✓（四态状态机 pending→running→succeeded/failed ⚠️A3 驱动任务详情呈现、列表在途标记与按钮态）。4/4 → 聚合根，**落 SQLite**（与 env_install_task 台账同构）；与 db_tool 为**同域两个独立聚合**（部署档案与执行事实各自一致性边界；任务终态驱动档案登记/更新为跨聚合同事务协作——与 backup_job/backup_run 同域双聚合先例同构）；
- **工具凭据 → 值对象**：内嵌于 db_tool 聚合根——basic auth 用户名+口令密文（mongo-express）/signon 密钥密文（phpMyAdmin）；无独立标识（值相同即等价）、不可变只能替换（轮换=替换凭据值，旧口令即时失效）；AES-256-GCM 密文落库、明文仅内存态生成与注入使用、任何接口不回显、审计脱敏（I-02，沿 db_instance 凭据/dns_credential 先例）；**数据库连接凭据（MySQL/Mongo）不归本值对象**——取自 database-mgmt vault（关联领域对象消费，见第五部分），本域不复制不持久化对方凭据；
- **工具条目（tool entry）→ 域内对象-实体（非聚合根）**：A1-A4 全不通过——无面板侧一致性边界（目录为编译期常量，运行态/依赖就绪度事实源在 systemd/站点/env-mgmt 实况，面板为探测投影）/无跨域聚合引用（database-mgmt 高级入口为读契约消费非 ID 引用）/无生命周期入口（条目随面板版本内置，非面板动作创建）/无域内对象受其状态驱动。实体（非值对象）：标识=db_tool_type 目录键（两值编译期枚举），跨请求稳定可寻址——与 env entry/systemd_service 同构判定；**零工具台账不落库**（目录条目本体为编译期常量随代码分发；部署状态= db_tool 档案存在性+实况探测合并呈现，运行态/依赖就绪度实时探测不缓存）；
- **目录映射（tool catalog mapping）→ 值对象**：内嵌于工具条目（编译期）——每工具的钉死版本、分发形态（tarball+sha256 / npm 包+精确版本）、目标引擎、部署形态常量（端口/目录/baseUrl 前缀，D3 定值）；无独立标识（工具唯一确定）、不可变（随面板版本更新）；非用户可配数据（与发行版映射同性质）。
- **落地引用（siteRef）→ 值对象（v1.26 显式建模）**：内嵌于 db_tool 聚合根（site_ref JSON 列整体序列化，取证 inject.go:20-25/api-db-tools.md:237/:554）——来源文件路径 File+server 块定位 Index+ServerName+Scheme（既有）+**Port（落地监听端口，既有 80/443 块 Port=0 表默认）+AutoCreated（是否面板自动创建的落地块，清块判定用）**（v1.26 扩展，⚠️A14）；无独立标识（落点定位值相同即等价）、不可变只能替换（重新安装/域名联动刷新=替换引用值）；**JSON 载荷扩字段零 DDL 变更**（TEXT 列不变，旧档案缺省 Port=0/AutoCreated=false 向后兼容沿旧形态）；安装落档时定格（撤除/打开/清块寻址用），区别于过程数据——持久化于部署档案；**自动落地块本体零台账**（块存在性事实源=nginx 配置文件 dougong-tools.conf，与 site 零站点台账「配置文件是唯一事实源」同哲学，面板不存储块状态）。
- **v1.26 增量判定结论（自动 nginx 落地轮，⚠️A14~A17）**：**零新聚合根/零新状态机/零 DDL 变更**——自动建块/域名联动/清块为安装/卸载任务链与钩子/惰性同步的机制步骤（结果随任务终态或配置同步，无独立生命周期状态）；ToolLandingPort=18090/块文件路径为编译期常量（随目录映射同性质，非领域对象）；AutoCreated 为布尔属性/Port 为数值（非枚举不入池）；HostGuard 内部端点豁免为 panel-core 机制侧变更（跨模块待同步项，非本域对象）；reason 可见为 tool_deploy_task 失败原因字段的文本构造扩展（error 含 errcode data.reason，字段语义不变）。


## 第二部分：业务域清单

| 上下文名 | 领域分层 | 聚合根 | 核心职责 | 关联上下文 |
|---------|---------|--------|---------|-----------|
| 账户与安全基座（account-auth） | 支撑域 | panel_user、session、audit_log、初始化状态 | 初始化双入口仲裁、登录认证与锁定、会话全生命周期治理、RBAC 边界、审计记录与治理 | 全部业务域（被依赖：审计接口/会话鉴权/权限基线）；panel-core（入口面归其）、terminal（限流归其）；**db-tools（v1.24 被消费：gating 会话校验 OHS——db-tools 自持 GET /dbtool/gate/verify 内部端点复用既有 VerifyWs 只读会话校验，仅 loopback 子请求通道，account-auth 零新增 HTTP，api D-11 定稿 v1.25 W2 回填）** |
| 面板基座（panel-core） | 支撑域 | panel_config | 可审计安装交付（ADR-009/D9）、入口面治理（监听/安全入口/域名绑定 Host 强校验 ADR-028/HTTPS，R-10）、面板证书 ACME 签发与续期（HTTP-01 经已有 nginx，ADR-029）、全模块外壳挂载与菜单可见性、panel_config 配置单例治理与生效分类、版本可观测与更新检查克制治理（R-09） | account-auth（遵奉会话鉴权/RBAC；消费审计写入与会话策略 OHS）；site-mgmt（消费：AcmeGateway 签发通道与 nginx 安全写回 OHS，ADR-029，被依赖登记见 api-site-mgmt.md 1.5/5.1）；全部业务模块（被依赖：外壳挂载点/入口拦截）；更新检查版本源（待对齐，R-09 默认关闭）；**db-tools（v1.26 被消费：配置变更通知钩子提供〔bindDomain 变更→自动块 server_name 刷新，S-11-01 配置权威侧，跨模块待同步项〕+HostGuard/EntryPathGuard 内部端点豁免〔S-07-02/S-07-03 机制侧变更，/dbtool/gate/·/dbtool/signon/ 最小化直通，非内部路径强校验不变，跨模块待同步项〕）** |
| 系统监控（monitoring） | 支撑域 | —（无聚合根，只读观测流） | 实时指标周期采样与环形缓冲留存（ADR-014 不落库、重启清零）、实时快照/TOP 进程/1h-24h 趋势/系统信息只读视图（两角色同等，ADR-018）、监控数据读取契约（供 panel-core 仪表盘消费） | account-auth（遵奉会话鉴权 R-02）；panel-core（消费方：仪表盘页读取契约）；枚举共享 metric_window/metric_dimension（与 panel-core 共享口径） |
| systemd 服务管理（service-mgmt） | 支撑域 | —（无聚合根，dbus 实时投影流） | system 级 unit 实时发现与自动纳管（R-15 发现即接管，零持久化无台账）、服务列表/过滤搜索/属性只读视图（两角色同等，ADR-018）、启停/重启/自启切换管控（admin 专属，dbus 通道 R-08，等待终态反馈）、变更操作经审计契约留痕（R-03） | account-auth（遵奉会话鉴权与只读边界 R-02/ADR-018；消费审计写入 OHS R-03）；枚举共享 audit_result（与 account-auth 共享口径）；systemd（本机 dbus 通道，非业务域） |
| 文件管理器（file-manager） | 支撑域 | trash_entry | 全部路径操作白名单强制前置（R-04 EvalSymlinks+前缀校验，防逃逸为 M2 验收硬项）、目录浏览/导航实时投影视图（零文件台账，两角色同等 ADR-018）、上传（原子落盘/分块）/下载（数据取出口径）/在线编辑/整理/权限属主/压缩解压变更动作集（admin 专属，ADR-018 禁 readonly 上传下载）、删除入站回收站可逆闭环（R-17/ADR-021，还原冲突显式决策、默认仅手动清空） | account-auth（遵奉会话鉴权与只读边界 R-02/ADR-018；消费审计写入 OHS R-03）；枚举共享 audit_result（与 account-auth 共享口径）；本机文件系统（files 适配器/安全路径层，非业务域） |
| 网站管理（site-mgmt） | 核心域 | certificate、dns_credential（v1.20+） | 存量 nginx 站点接管（R-15 扫描→指令树解析→server 块提取→写回落点登记，零站点台账投影）、安全写入治理（R-06 四步+R-16 隔离，全部写回强制前置）、双模式站点编辑（结构化+Monaco 源码，R-07 Raw 保留）、三模板建站与站点启停/日志/重载、证书全生命周期治理（ACME HTTP-01〔文件验证〕/TLS-ALPN/DNS-01 三种挑战申请、部署、自动续期六状态机 ADR-024、存量识别导入；DNS-01〔v1.20〕经 DNS 服务商 API 自动 TXT+泛域名，dns_credential 凭证聚合+三服务商初始清单 ⚠️A20） | account-auth（遵奉会话鉴权与只读边界 R-02/ADR-018；消费审计写入 OHS R-03）；枚举共享 audit_result 与本域枚举（acme_challenge 扩 dns_01+dns_provider 新增，v1.20）；ACME CA（外部系统，ACL 经 lego 封装，R-09 外呼例外）；DNS 服务商 API（Cloudflare/阿里云 DNS/腾讯云 DNSPod，v1.20——ACL 经 lego DNS provider 封装，R-09 例外语义与 ACME 同判）；nginx/php-fpm（本机系统组件，nginxx 适配器 ADR-010，非业务域） |
| 数据库管理（database-mgmt） | 支撑域 | db_instance、redis_instance | 存量多引擎实例自动探测与纳管（MySQL/MariaDB/Redis/MongoDB——R-15 发现即接管，⚠️A19 限定本机探测不扫网段；mongodb v1.21 增量：探测集扩 mongod 默认 socket+27017；**v1.23 TCP 优先两阶段登记+socket 兜底+rescan 保守清理无凭据冗余 socket 行，Q1 二次拍板，identity_key 语义不变**）、凭据密文治理（⚠️A18 只进不出：不回显/审计脱敏/密文落库，验证失败不生效）、MySQL 库/用户/库级授权/root 密码治理（零库用户台账实时投影）、**MySQL 表级治理（v1.23：表清单/结构/分页数据浏览/单列主键行写/删表，零表行台账；元数据两角色/行数据 admin 专属 Q6）**、状态概览/进程列表/慢日志只读观测（两角色同等 ADR-018）、mysqldump 备份导出导入（R-08 参数数组，admin 专属）、Redis 密码设置与 key 只读浏览（MVP 严格只读）、MongoDB 治理线（v1.21 增量，**v1.23 修订 ADR-031 推翻 D-18 只读**：状态概览/库清单可管理（名称/大小/集合数+浏览集合入口+删库 dropDatabase，隐式建库经显式建集合达成）/用户与角色治理（db.createUser/dropUser + mongo_role 三档库级角色授予回收）/**集合与文档治理（集合清单/建集合/删集合/文档分页浏览/插入/编辑/删除，零集合文档台账；文档内容 admin 专属 Q6）**；通道=进程内 mongo-driver Go 库，R-09 零外呼维持仅连本机）、**呈现层（v1.23：清单 TCP 优先呈现+状态语义 tooltip/图例 F2、顶栏面包屑返回导航 F5 消费 panel-core S-09-01）** | account-auth（遵奉会话鉴权与只读边界 R-02/ADR-018；消费审计写入 OHS R-03）；枚举共享 audit_result 与本域 3 枚举（入池共享口径）；cron-backup（被依赖：数据库备份源复用 DumpStream OHS）；**env-mgmt（v1.22 被消费：BootstrapAfterInstall 实例引导注册 OHS——安装后配置就绪建账户/设密码/注册实例凭据）**；**db-tools（v1.24 被消费：实例凭据 OHS——signon 桥接/mongo-express 注入取出 vault 凭据，进程内单向；高级管理入口经读契约消费 db-tools 工具状态与打开能力，跨模块待同步项）**；MySQL/Redis/MongoDB 引擎（本机系统组件，dbmysql/dbredis/dbmongo 适配器，非业务域） |
| Web 终端（terminal） | 支撑域 | terminal_session | 浏览器 shell 交互通道（xterm.js↔WS↔PTY，admin 专属、readonly 整域禁用 ADR-018）、三重准入（WS 握手复用面板会话鉴权 R-18+角色判定+并发 5 ADR-023）、运行身份选择（默认面板运行用户（通常 root）/指定系统用户 su 语义不代输密码，R-18）、会话生命周期管控（空闲 30min 从严口径/硬上限 8h/面板会话失效联动断开）、统一断开清理（进程回收+记录销毁+配额释放，内存态不落库断开即无痕） | account-auth（遵奉会话握手与失效联动 R-18；消费审计写入 OHS R-03）；system_user（系统实况只读引用，非面板域对象）；docker-mgmt（被依赖预期：容器内终端二期，不预先建模） |
| 防火墙（firewall） | 支撑域 | —（无聚合根，后端实况投影流） | 既有 ufw/firewalld 双后端自动识别接管（R-15 ufw 优先，均无引导提示、不呈现空列表）、端口规则列表/监听端口对照只读投影视图（两角色同等 ADR-018，零规则台账不落库）、规则增删即时生效管控（admin 专属，firewallx 通道 R-08，firewalld 永久+运行态双写）、面板端口保护硬项（直接命中面板端口的删放行/加阻断警示+二次确认，M7 明示）、变更操作经审计契约留痕（R-03） | account-auth（遵奉会话鉴权与只读边界 R-02/ADR-018；消费审计写入 OHS R-03）；panel-core（消费：面板监听配置为标注与涉险判定基准）；枚举共享 audit_result 与本域 4 枚举（入池共享口径）；ufw/firewalld（本机系统组件，firewallx 适配器，非业务域） |
| 容器管理（docker-mgmt） | 支撑域 | —（无聚合根，daemon 实况投影流） | 本机既有 Docker daemon 接管式连接（R-12/R-15 sock 探测，未装引导不呈现空列表）、容器/镜像/卷/网络/Compose 项目五类实况只读投影视图（两角色同等 ADR-018，零台账不落库）、容器生命周期六动作管控（admin 专属，状态映射与动作矩阵 ADR-025、MVP 无强制删除）、镜像拉取（daemon 侧外呼、用户主动）、资源删除以 daemon 拒绝为最终权威、Compose label 自动识别 + `docker compose` CLI 参数数组操作（R-13/ADR-013，定位失败容器级兜底）、变更操作经审计契约留痕（R-03） | account-auth（遵奉会话鉴权与只读边界 R-02/ADR-018；消费审计写入 OHS R-03）；terminal（被依赖预期：容器内终端二期，不预先建模）；枚举共享 audit_result 与本域 3 枚举（入池共享口径）；Docker daemon（本机系统组件，dockerx 适配器=官方 Go SDK + compose CLI，非业务域） |
| 计划任务与备份（cron-backup） | 支撑域 | backup_job、backup_run | root crontab 接管式治理（读写唯一域+系统预置位只读 D1 口径-1；表达式校验预览/执行日志尾部/手动触发面板代执行+超时控制 R-08 边界口径；零任务台账、备注与来源标注注释行承载 D2 口径-1）、backup_job 定义治理（源=站点目录值快照+数据库实例引用、目标本地/rsync、保留份数、调度 cron 表达式 D2 口径-2）、面板内置调度器定时执行与立即执行（触发互斥 D2 口径-4、错过不补跑 D2 口径-3、与系统 cron 解耦不写 crontab D1 口径-2）、backup_run 执行历史与产物消费（running→success/failure ADR-016、产物下载 admin 专属 ADR-018）、按保留份数轮转清理（R-11，记录产物同删 D2 口径-5） | account-auth（遵奉会话鉴权与只读边界 R-02/ADR-018；消费审计写入 OHS R-03）；database-mgmt（消费：实例清单联动+执行凭据建连；**被引用：db_instance 为全项目首个关联领域对象，见第五部分**）；site-mgmt（消费：站点清单联动选取）；rsync 远程目标（用户基础设施，ACL 经 backupx 适配器，用户主动 R-09 例外语义）；cron（本机系统组件，cronx/backupx 适配器，非业务域） |
| 环境管理（env-mgmt）（v1.19 追加；v1.22 安装后配置就绪；v1.24 +Node） | 支撑域 | env_install_task | 运行环境本体供给（软件包层，v0.1.2 反馈第 4 项补位）：内置八环境目录+实况实时探测合并呈现（nginx/Apache/Docker/MySQL/Redis/PHP-FPM/MongoDB/Node.js，⚠️A1/A7/A9，零环境台账投影——包管理器/systemd 为事实源；**v1.24 +Node.js：NodeSource 三系代配+LTS 轨道钉死+npm 随包 ⚠️A16，运行时型无服务单元——服务态 null/启停入口禁用/装完即用无引导 ⚠️A17**）、apt/dnf·yum 双系通道探测择一与官方仓库源代配（⚠️A2/A10，写入审计）、异步变更任务四态机 pending→running→succeeded/failed（⚠️A3，同环境互斥、任务台账落 SQLite、重启恢复 running→failed）、安装与升级同通道幂等（⚠️A4，先刷新索引）、卸载保守语义（⚠️A6 remove 保留配置数据+强确认风险提示）、**安装后配置就绪（v1.22，⚠️A11~A15：数据库类安装成功后 worker 经 db-mgmt OHS 建管理账户/设随机强密码+注册实例凭据，装完即可管；引导结果独立维度 bootstrap_status/note 入任务台账，幂等保守不覆盖）**、已装环境服务启停自启（复用 service-mgmt SysdGateway 共享适配层，操作限定目录环境单元，Node 无单元不适用）、变更与任务终态审计留痕（R-03）、包操作参数数组纪律（R-08） | account-auth（遵奉会话鉴权与只读边界 R-02/ADR-018；消费审计写入 OHS R-03）；service-mgmt（共享 SysdGateway systemd 适配层资产，进程内共享非模块调用）；**database-mgmt（v1.22 消费其 BootstrapAfterInstall 进程内 OHS——安装后配置就绪建账户/注册实例凭据，单向硬依赖）**；docker-mgmt/database-mgmt/site-mgmt/db-tools（被消费：环境未装实况软引导；**db-tools v1.24 消费环境实况读契约〔PHP-FPM/Node/nginx 依赖就绪度〕与 Node 供给**）；apt/dnf·yum/systemd（本机系统组件，envpack 适配器，非业务域） |
| 数据库工具集成（db-tools）（v1.24 追加；v1.26 自动落地增量） | 支撑域 | db_tool、tool_deploy_task | 数据库高级管理现成工具集成（宝塔式路线 ADR-032，用户拍板 2026-11-19）：内置两工具目录（phpMyAdmin→MySQL/MariaDB、mongo-express→MongoDB，版本钉死编译期 ⚠️A1）+实况实时探测合并呈现（部署状态/版本/运行态/依赖就绪度，零工具台账投影）、工具部署与变更（安装〔依赖前置 ⚠️A2+下载钉版本 sha256 强制校验+镜像 ⚠️A12+站点/服务落位+凭据生成密文落库〕/升级〔目录钉死新版本，配置凭据保留 ⚠️A4〕/卸载〔解除部署**绝不删数据库数据** ⚠️A5〕，异步任务四态机同工具互斥台账落 SQLite ⚠️A3）、**工具安全访问（localhost 绑定 ⚠️A6+面板会话 gating 反代 auth_request 校验会话+admin 角色 ⚠️A7/A10+phpMyAdmin signon 桥接〔HMAC 令牌防伪+短时效+一次性+绑定实例，vault MySQL 凭据 loopback 注入永不进浏览器〕⚠️A8+mongo-express basic auth 服务端注入与轮换〔Mongo 凭据 vault→systemd env〕⚠️A9+统一打开入口）、凭据 AES-256-GCM 密文零回显（I-02）、变更/打开/轮换审计同事务脱敏（R-03）、外部命令参数数组（R-08）、零遥测下载用户主动例外（R-09）**；**v1.26 自动 nginx 落地（⚠️A14~A17）：落点 0 匹配自动建专用块 dougong-tools.conf listen 18090（ToolLandingPort 编译期常量，占用→DBT_012 reason）+域名联动（server_name 跟随绑定：未绑 _ /已绑 BindDomain+非匹配 Host 404 镜像 HostGuard，钩子主通道+install/open 惰性兜底 D1 裁定）+既有块优先/多匹配引导/幂等复用/两工具卸载清块（AutoCreated 判定）+siteRef 值对象 +Port/AutoCreated（零 DDL）+打开 URL 端口分支+worker 失败原因含 data.reason 可见；自动块每请求 gating，工具本体仍 localhost；HostGuard/EntryPathGuard 内部 2 端点豁免为 panel-core 机制侧变更（跨模块待同步项）**；轻量入口（面板内浏览）保留并存（Q3/⚠️A13），治理台账仍归 database-mgmt | account-auth（遵奉会话鉴权与只读边界 R-02/ADR-018；消费审计写入 OHS R-03；**消费 gating 会话校验 OHS——db-tools 自持 GET /dbtool/gate/verify 复用既有 VerifyWs（account-auth 零新增 HTTP，api D-11 定稿 v1.25 W2 回填）**）；env-mgmt（消费环境实况读契约：PHP-FPM/Node/nginx 依赖就绪度；Node 供给为其 v1.24 增量；依赖缺失软引导回其安装页）；site-mgmt（消费站点创建/移除 S-27-02/S-27-03/S-28+安全写入治理 S-30 R-06：工具路径/gating/注入配置落盘；**v1.26 +自动落地块 ConfigApply〔allowNew〕/ConfigRemove 既有 OHS 复用**）；service-mgmt（共享 SysdGateway 适配层资产：mongo-express 单元创建/启停/查询）；**database-mgmt（消费其实例凭据 OHS——vault 取出 MySQL/Mongo 连接凭据，进程内单向，凭据不出服务端边界；被消费：高级管理入口经读契约消费工具状态与打开能力）**；**panel-core（v1.26：消费配置变更通知钩子〔bindDomain 绑定/解绑→自动块 server_name 刷新，进程内单向，钩子提供为跨模块待同步项〕+依赖 HostGuard/EntryPathGuard 内部端点豁免〔S-07 机制侧变更，跨模块待同步项〕+消费 panel_config.bind_domain 实况值〔PanelAccess 读契约既有，非 ID 引用〕）**；phpMyAdmin/mongo-express（外部现成工具，ACL 封装，工具本体 localhost 经 gating 访问；v1.26 自动块 nginx 接入层 18090 每请求 gating）；nginx/PHP-FPM/systemd/npm（本机系统组件，非业务域） |

> 全部业务域（13 个）已完成 D2 累积，本表闭环（v1.24 +db-tools）。

## 第三部分：领域对象清单

| 对象名 | 分类 | 所属域 | 与上级关系 | 核心业务字段 | 关键枚举字段 | 备注 |
|--------|------|--------|-----------|------------|------------|------|
| panel_user | 聚合根 | 账户与安全基座（account-auth） | — | 用户名、密码哈希、角色、状态、创建时间 | panel_role{admin,readonly}；panel_user_status{active,locked} | 一致性边界=账户凭证与角色；删除受唯一管理员保护；枚举以 01-solution/04-business-enum.md 定稿为准（D3 对齐） |
| TOTP 凭证 | 域内对象-实体 | 账户与安全基座（account-auth） | 1:1 隶属 panel_user | TOTP 密钥、启用开关、绑定时间 | totp_enabled{on,off} | RFC 6238，30 秒窗口 ±1 |
| 登录锁定状态 | 域内对象-实体 | 账户与安全基座（account-auth） | 1:1 隶属 panel_user | 连续失败计数、锁定截止时间 | lock_status{none,locked} | R-01：5 次失败锁 15 分钟，期满自动解除 |
| session | 聚合根 | 账户与安全基座（account-auth） | — | 令牌、所属用户、创建时间、最后活跃时间、绝对过期时间、空闲截止时间、CSRF 令牌、吊销标记、来源 | session_status{active,expired,revoked} | HttpOnly Cookie 承载（R-02）；被终端 WS 复用（R-18） |
| 会话有效期策略 | 域内对象-配置实体 | 账户与安全基座（account-auth） | 隶属 session（域级策略） | 绝对过期时长（默认 12h）、空闲超时时长（默认 2h） | — | 两项均可配置（ADR-019） |
| audit_log | 聚合根 | 账户与安全基座（account-auth） | — | 操作者、时间、动作、对象类型、对象标识、变更内容、结果、来源 | audit_action{change,security}；audit_result{success,failure} | 只追加不可改删（R-03）；90 天保留（ADR-020） |
| 变更内容快照 | 值对象 | 账户与安全基座（account-auth） | 内嵌于 audit_log | 变更前后值、字段清单 | — | 不可变，随记录写入 |
| 审计保留策略 | 值对象 | 账户与安全基座（account-auth） | 隶属 audit_log（域级策略） | 保留天数（固定 90）、清理周期（每日） | — | ADR-020 固定策略，清理动作本身记审计 |
| 初始化状态 | 聚合根 | 账户与安全基座（account-auth） | — | 初始化标记、初始化方式、初始化时间 | init_status{uninitialized,initialized} | 单行记录；双入口先落库者胜出仲裁（ADR-017）；单向不可逆 |
| panel_config | 聚合根 | 面板基座（panel-core） | — | 监听地址、监听端口、安全入口开关、安全入口路径值、绑定域名（bind_domain，可空=未绑定，v1.16 增量 ADR-028）、HTTPS 模式、更新检查开关、界面语言 | https_mode{off,self_signed,imported}；listen_scope{all,localhost,custom}；entry_path_enabled{off,on}；update_check{off,on} | 单例聚合根（A1-A4：全局引用/唯一入口/单例一致）；无状态机（D0 拍板，变更即生效/提示重启）；绑定域名为 Host 级强校验门槛字段（绑定后仅该 Host 可达、IP/其他 Host 标准 404、读态失败 fail-closed，ADR-028，即时生效）；默认值与生效分类见 prd-panel-core.md 引 D1 AA 附录C |
| HTTPS 证书 | 域内对象-实体 | 面板基座（panel-core） | 1:1 隶属 panel_config | 证书内容、私钥、有效期、颁发者、签发状态（acme 来源）、上次失败原因（acme 续期失败排障） | panel_cert_source{self_signed,imported,acme}（acme 为 v1.16 扩展，ADR-029） | 自签态系统生成；导入态随配置上传（配对性服务端校验，R-14）；acme 态经已有 nginx HTTP-01 签发/续期（ADR-029，复用 site-mgmt AcmeGateway 与 nginx 安全写回 OHS，不新增独立 80 端口监听；DNS-01 二期）；签发状态沿用 ADR-024 六状态语义按需简化引用（单例对象非多实例生命周期状态机，valid/expiring_soon/renew_failed/expired 主路径，字段形态 D3 定稿 prd 待对齐-6）；物理存储已定稿 panel_cert 单行表（api-panel-core.md D-P04，key_pem 零下发） |
| 配置生效分类 | 值对象 | 面板基座（panel-core） | 隶属 panel_config（域级策略） | 需重启生效项清单（监听地址/端口）、即时生效项清单 | config_effect_mode{immediate,restart_required} | D1 判定的域级规则（AA 附录C），驱动保存后的生效提示（FR-13） |
| metric | 值对象 | 系统监控（monitoring） | 环形缓冲集合条目（1 缓冲:N 条目，滚动淘汰） | 时间戳、CPU 总/分核使用率、内存占用、磁盘容量（按挂载点）、磁盘 IO、网络上下行速率、负载（1/5/15min）、TOP 进程清单（前 10 含内存占用） | —（对象自身无枚举字段；查询参数枚举 metric_window/metric_dimension/metric_scope 入池共享） | 内存态不落库（ADR-014）；不可变，随采样产生、随缓冲滚动淘汰；3s 采样/1min 聚合 ×1440 条 ≈24h（⚠️A12）；多设备聚合口径 ⚠️A13；读取契约见 api-monitoring.md（D-03 series 统一形状） |
| system_info | 值对象 | 系统监控（monitoring） | 独立只读投影（不经缓冲） | 发行版、内核版本、主机名、运行时长 | — | 实时获取不持久化（kb 对象表）；部分获取失败时失败字段置空并标注原因，不虚构默认值 |
| systemd_service | 域内对象-实体 | systemd 服务管理（service-mgmt） | 投影集合条目（经读取契约以 unit 名称直接寻址，无聚合根守门） | 名称（unit 全名如 nginx.service）、unit 类型、运行状态、sub 状态、加载状态（loadState，透传属性）、开机自启状态、描述 | service_active_state{active,inactive,failed}；service_enabled_state{enabled,disabled}（两态为可切换主轴，实际透传含非两态值 ⚠️A14）；unit_type{service,socket,timer,mount,path,target,automount,slice,swap,scope,device} | 内存态投影不落库（kb 拍板，D1 AA 基础数据域判定）；标识=unit 名称（跨读取稳定、可寻址）故为实体而非值对象——同一服务状态随实况变迁（active→inactive 为同一对象的状态更新，非替换）；状态事实源在 systemd 侧，投影随读取整集合刷新；操作以名称寻址经动作契约直达 systemd |
| trash_entry | 聚合根 | 文件管理器（file-manager） | — | 原路径、删除时间、操作者、类型（文件/目录）、大小、trash 内标识 | trash_entry_status{in_trash,restored,purged} | 落 SQLite（ADR-021，本模块唯一持久化对象）；一致性边界=trash 记录与 `.dougong-trash/` 条目内容；还原转 restored 记录保留供追溯、清空转 purged 记录物理删除（销毁事实由审计留痕）；操作者字段为会话上下文审计留痕快照，非跨域聚合引用；还原冲突显式决策覆盖/放弃（D1 落定） |
| file_entry | 域内对象-实体 | 文件管理器（file-manager） | 投影集合条目（经读取契约以受管根内绝对路径直接寻址，无聚合根守门） | 路径、类型、大小、权限、属主（用户/组）、修改时间 | file_type{file,directory,symlink} | 文件系统实时投影不落库（kb 拍板）；标识=绝对路径（跨读取稳定、可寻址）故为实体而非值对象——属性随实况变迁（chmod 后仍为同一对象的属性更新，与 systemd_service 同构）；路径白名单校验（R-04）为全部寻址强制前置；含隐藏条目全量呈现（D1 解释性决策） |
| 受管根配置 | 域内对象-配置实体 | 文件管理器（file-manager） | 域级单例（域配置，不经聚合根） | 受管根路径（默认 `/`）、排除目录清单（默认建议 `/proc`、`/sys`、`/dev`） | —（配置值为路径列表，无枚举字段） | R-04 白名单业务边界载体（⚠️A12，D1 新增假设）；修改记审计、对后续操作即予生效；载体=fs_path_scope 单行表（D3 定稿，api-file-manager.md D-02：同事务审计+写后刷新内存副本即予生效，区别于 account-auth D-10 配置文件重启生效） |
| certificate | 聚合根 | 网站管理（site-mgmt） | — | 域名列表、issuer、有效期、来源、托管路径（acme）、续期挑战方式（acme）、失败原因（renew_failed）、导入原路径（imported）、关联站点引用 | cert_status{pending,valid,expiring_soon,expired,renew_failed}；cert_source{acme,imported}；acme_challenge{http_01,tls_alpn} | 落 SQLite（本模块唯一持久化对象：续期任务状态无法从配置文件推导）；六状态机 ADR-024（A2 关闭）；一致性边界=证书记录↔托管文件↔站点部署引用（续期成功=替换+回 valid+重部署三半不可分）；ADR-024 文本「6 状态」与枚举命名 5 状态的计数差异已登记（prd-site-mgmt.md 第六部分，待 ADR-024 侧澄清） |
| dns_credential | 聚合根 | 网站管理（site-mgmt）（v1.20 追加） | —（certificate.dns_credential_id 引用为同域聚合引用，非父子隶属） | 名称（显示名）、服务商（dns_provider）、凭据材料（内嵌值对象，密文列） | dns_provider{cloudflare,aliyun,dnspod} | 落 SQLite（v1.20/Gate 1 改判 ⚠️A20：凭据材料 AES-256-GCM 密文+独立密钥文件 dns-cred.key 0600——沿 db_instance 凭据先例；明文永不回显、审计脱敏）；无状态机（存在即有效，校验态不缓存）；删除引用警示不硬阻断（引用证书自动续期将失败如实呈现）；admin 专属（readonly 全负空间，解释性决策） |
| DNS 凭据材料 | 值对象 | 网站管理（site-mgmt）（v1.20 追加） | 内嵌于 dns_credential（密文列整体加密） | 按 provider 形态：API Token（cloudflare）/ AccessKeyId+AccessKeySecret（aliyun）/ SecretId+SecretKey（dnspod） | — | 密文整体存储（JSON 序列化后 AES 加密单列）；不可变替换式更新（轮换=删除重建，MVP 无独立 update）；明文仅内存态签发/续期建连（db_instance 凭据同构） |
| site | 域内对象-实体 | 网站管理（site-mgmt） | 投影集合条目（经扫描读取契约以来源文件内 server 块定位寻址，无聚合根守门） | server_name 列表、listen、root、ssl_cert 引用、location 列表、反代上游（upstream 内嵌值对象）、enabled、Raw 块（内嵌值对象）、来源文件路径（写回落点映射） | site_enabled{enabled,disabled}；site_type{static,php_fpm,reverse_proxy}（创建时模板来源标记） | 配置文件实时投影不落库（D2 判定：配置文件是唯一事实源，零站点台账，与 file-manager「不导入不镜像」同哲学）；标识=来源文件内 server 块定位（跨扫描稳定可寻址，稳定键形态 D3 定待对齐-1）；证书状态列为 certificate 同域数据只读消费；v1.18：列表聚合呈现（同源文件同 server_name 全集多块合并，⚠️A18）为投影组装规则非新对象，siteId 寻址不变 |
| 反代上游（upstream） | 值对象 | 网站管理（site-mgmt） | 内嵌于 site | 上游地址、Host 头、WebSocket 透传开关 | —（透传开关为布尔语义，不入池，与 totp_enabled 同口径） | D0「是否独立对象」事项关闭：site 内嵌结构不独立建模（D1 落定）；不可变替换语义 |
| Raw 未识别指令块 | 值对象 | 网站管理（site-mgmt） | 内嵌于 site | 原文本、所在文件位置 | — | R-07 原文保留写回（零丢失底线）；round-trip 样本集 ≥ 20 验收兜底 |
| 写回落点映射 | 值对象 | 网站管理（site-mgmt） | 随 site 派生（1:1） | 来源文件路径、所属配置目录 | — | ⚠️A13 多配置目录（conf.d/sites-enabled）消重、写回原文件；扫描派生随投影刷新 |
| db_instance | 聚合根 | 数据库管理（database-mgmt） | — | 引擎类别、标识（host:port / socket 路径）、连接方式、版本、连接状态（探测态）、凭据密文（内嵌值对象） | db_engine{mysql,mariadb,mongodb}（v1.21 扩值；redis 归 redis_instance）；db_conn_mode{socket,tcp}；instance_conn_state{reachable,unreachable} | 落 SQLite（本模块持久化对象①：凭据密文无法从实例实况推导必须面板自持，⚠️A18 密文不回显）；一致性边界=实例档案↔凭据密文（root 密码修改成功→密文同步，纳管不失效）；标识=引擎类别+host:port（socket 路径参与，多实例消重，D1 口径）；连接状态为探测态非状态机（D0 拍板） |
| redis_instance | 聚合根 | 数据库管理（database-mgmt） | — | 标识（host:port / socket 路径）、连接方式、版本、连接状态（探测态）、凭据密文（内嵌值对象） | db_engine{redis}；db_conn_mode{socket,tcp}；instance_conn_state{reachable,unreachable} | 落 SQLite（持久化对象②，与 db_instance 同构判定、独立聚合无共享一致性边界）；密码设置成功→密文同步；key 浏览以实例标识为前缀只读寻址 |
| database | 域内对象-实体 | 数据库管理（database-mgmt） | 1:N 隶属 db_instance（经实例连接寻址） | 名称、字符集 | —（字符集为实例实况开放集合非固定枚举，候选实况读取，prd 待对齐-10） | MySQL/MongoDB 实况实时投影不落库（D2 判定零库台账：事实源在实例内，面板外建库自动反映，与 site 零站点台账同哲学；mongodb 项=名称/大小/集合数，**v1.23 修订 ADR-031：可管理清单——浏览集合入口+删库 dropDatabase（admin），无独立建库动作（隐式建库经显式建集合达成）**；MySQL 项 v1.23 +浏览入口→表级治理）；标识=所属实例+库名；建/删经实例连接执行（面板为通道之一非唯一入口，A3 不通过）；授权（用户⇄库）为关系数据投影非独立对象 |
| db_user | 域内对象-实体 | 数据库管理（database-mgmt） | 1:N 隶属 db_instance（经实例连接寻址） | 用户名、host、授权关系（库级）、密码（只写不读） | —（授权权限集合粒度 D3 拍板，prd 待对齐-3） | 实时投影不落库（与 database 同构；mongodb 项=admin 库 users 实况投影、标识=所属实例+用户名（无 host 维度）、角色概况随查呈现——v1.21 增量）；标识=所属实例+用户名+host（mongodb 无 host）；删除时既有授权/角色一并回收（引擎语义）；密码任何角色不回显 |
| 凭据 | 值对象 | 数据库管理（database-mgmt） | 内嵌于 db_instance / redis_instance | 用户名、密码密文 | — | ⚠️A18：密文落库、明文仅内存态建连、任何接口不回显、审计脱敏；不可变替换式更新（改密=替换，「同步」语义）；密钥来源与列形态 D3（prd 待对齐-1） |
| data_table（数据表） | 域内对象-实体 | 数据库管理（database-mgmt）（v1.23 追加） | 1:N 隶属 database（经实例+库连接寻址） | 表名、存储引擎、行数（估算）、数据大小、排序规则、注释、列结构（列名/类型/可空/键/默认/额外/注释） | —（存储引擎/排序规则为实例实况开放集合非固定枚举，沿字符集先例不入池） | MySQL/MariaDB information_schema 实况实时投影不落库（零表台账，与 database 同构判定 A1-A4 全不通过）；标识=所属实例+库名+表名；删表 DROP TABLE 经实例连接执行（面板为通道之一，本轮无建表）；元数据两角色可看（Q6）；列结构为表元数据字段不单独建模（对象级判定粒度） |
| table_row（数据行） | 域内对象-实体 | 数据库管理（database-mgmt）（v1.23 追加） | 1:N 隶属 data_table（经实例+库+表寻址） | 各列值（JSON 内容，NULL 显式语义） | — | 实况投影不落库（零行台账；数据内容读取即投影响应即消亡）；标识=所属实例+库+表+单列主键值——**行写仅限单列主键表**（无单列主键无稳定行标识→行编辑/删除禁用并如实提示，Q3）；行写 WHERE 主键=旧值（参数化/quoteString A-DB3）；**数据内容 admin 专属（Q6 沿 redis key 先例），readonly 负空间**；分页默认 20 上限 100；CLI 通道 JSON_OBJECT 逐行聚合（构造细节 D3） |
| mongo_collection（集合） | 域内对象-实体 | 数据库管理（database-mgmt）（v1.23 追加） | 1:N 隶属 database（mongodb 项，经实例+库连接寻址） | 集合名、文档数、大小 | — | MongoDB 实况实时投影不落库（零集合台账，与 database 同构判定 A1-A4 全不通过）；标识=所属实例+库名+集合名；createCollection/dropCollection 经实例连接执行（面板为通道之一非唯一入口）；隐式建库语义——库随首集合写入产生（ADR-031）；元数据两角色可看（Q6） |
| mongo_document（文档） | 域内对象-实体 | 数据库管理（database-mgmt）（v1.23 追加） | 1:N 隶属 mongo_collection（经实例+库+集合寻址） | JSON 内容（_id 置顶呈现） | — | 实况投影不落库（零文档台账）；标识=所属实例+库+集合+_id（ReplaceOne/DeleteOne 按其寻址）；InsertOne/ReplaceOne/DeleteOne 引擎侧动作，JSON 校验前端拦截+服务端校验（R-14）；**数据内容 admin 专属（Q6 沿 redis key 先例），readonly 负空间**；分页 skip/limit 默认 20 上限 100 |
| terminal_session | 聚合根 | Web 终端（terminal） | — | 运行身份（内嵌值对象）、建立时间、最后活跃时间、连接来源、鉴权会话引用、空闲截止/硬上限截止（限流策略推导的运行时字段） | terminal_identity_source{panel_run_user,specified_system_user}（断开原因为事件载荷枚举 terminal_end_reason，见备注） | **内存态不落库（D0 拍板）**——全项目唯一「聚合根+零持久化」组合；一致性边界=会话记录↔PTY 进程↔并发配额同生共死；活跃→断开×5 触发、断开即销毁（closed 无持久态，终态事实经断开通知+审计事件 terminal_end_reason 留痕）；鉴权会话引用为运行时校验凭据与联动断开触发源、非跨域聚合 ID 引用（见第五部分）；并发上限内会话集合以操作者标识寻址（A2 域内入口） |
| 运行身份 | 值对象 | Web 终端（terminal） | 内嵌于 terminal_session | 身份来源、系统用户名（specified_system_user 时） | terminal_identity_source{panel_run_user,specified_system_user} | R-18 两分支载体；su 语义代启登录 shell、不代输密码（D1 落定）；不可变（会话期间不变更，换身份=新建会话）；指定范围=具备登录 shell 的系统用户（nologin/false 不入候选） |
| 限流策略 | 值对象 | Web 终端（terminal） | 隶属 terminal_session（域级固定策略） | 并发上限（5）、空闲超时（30min）、连接硬上限（8h） | —（固定数值策略，无枚举字段） | ADR-023 拍板标准档，MVP 固定不可配置（对照 ADR-019 会话有效期策略可配置的差异已明示）；空闲从严口径（无输入为准、服务端持续输出不重置计时）为策略内规则（D1 落定） |
| firewall_rule | 域内对象-实体 | 防火墙（firewall） | 投影集合条目（经读取契约以后端侧规则定位直接寻址，无聚合根守门） | 端口/端口段、协议、方向（固定入站）、来源限制（任意/CIDR）、动作、后端原始语义标识 | firewall_rule_action{allow,deny,reject,limit}（ufw 全动作，firewalld MVP 仅 allow，D1 口径-2）；firewall_rule_protocol{tcp,udp} | 后端实况投影不落库（kb 拍板，D2 沿用零规则台账——与 site/database 同哲学）；标识=后端侧规则定位（形态 D3 定 prd-firewall.md 待对齐-2）；面板端口标注为与 panel-core 监听配置的直接命中派生标记（D1 口径-5）；firewalld 写入永久+运行态双写（D1 口径-1）；域级判定值 firewall_backend 与对照派生值 listen_port_state 为整值刷新枚举、不构成对象状态字段（见下方注） |
| container | 域内对象-实体 | 容器管理（docker-mgmt） | 投影集合条目（经读取契约以容器 ID/名称直接寻址，无聚合根守门） | 名称、镜像引用、状态（核心三态映射呈现）、所属 Compose 项目（label 透传）、资源占用快照（读取时派生）、日志（实况流读取，不持久化） | container_status{running,stopped,paused}（映射值：created/exited/dead→stopped 归并，restarting/removing 过渡态透传原始值——与 service_enabled_state 透传口径同构，D1 口径-1/BA 附录B） | daemon 实况投影不落库（零容器台账 D2 判定——与 site/database/firewall 同哲学）；标识=容器 ID/名称（跨读取稳定可寻址，生命周期/删除/日志操作按其寻址同一对象）故实体而非值对象——与 systemd_service/firewall_rule 同构；状态映射与动作矩阵唯一权威口径=BA 附录B（ADR-025）；A3 关键不通过项：容器创建不在面板动作集（docker run/Compose 等面板外通道皆可创建） |
| image | 域内对象-实体 | 容器管理（docker-mgmt） | 投影集合条目（经读取契约以镜像 ID 直接寻址，无聚合根守门） | 名称:tag、大小 | — | daemon 实况投影不落库（与 container 同构判定）；标识=镜像 ID；被容器引用时 daemon 拒绝删除（无前置推导无强制绕过，D1 口径-4）；引用情况不做清单标注（删除以 daemon 校验为准）；清单附加字段集 D3 定（prd-docker-mgmt.md 待对齐-1） |
| volume | 域内对象-实体 | 容器管理（docker-mgmt） | 投影集合条目（经读取契约以卷名直接寻址，无聚合根守门） | 名称、挂载点、大小 | — | daemon 实况投影不落库（与 container 同构判定）；标识=卷名；删除即数据销毁（确认文案明示不可恢复）；被容器挂载时 daemon 拒绝删除（D1 口径-4） |
| network | 域内对象-实体 | 容器管理（docker-mgmt） | 投影集合条目（经读取契约以网络 ID/名称直接寻址，无聚合根守门） | 名称、驱动、子网 | — | daemon 实况投影不落库（与 container 同构判定）；标识=网络 ID/名称；被容器接入时 daemon 拒绝删除；预定义网络（bridge/host/none 等）不可删、如实呈现拒绝语义（D1 口径-4） |
| compose_project | 域内对象-实体 | 容器管理（docker-mgmt） | label 聚合投影条目（经识别契约以项目名直接寻址，无聚合根守门；容器清单为对 container 的同域引用集合） | 项目名、工作目录、配置文件（label 元数据）、容器清单 | compose_action{up,down,restart,logs}（项目操作请求参数枚举，非对象状态字段） | 容器 label 聚合的派生投影不落库（kb 拍板；识别无登记动作，R-13）；标识=项目名（label 聚合键，跨扫描稳定，up/down/restart/logs 按其寻址）；无自有状态机（项目运行态=容器组实况汇总呈现）；定位失败（工作目录/配置文件缺失）为判定态，项目级操作不可用时容器级操作兜底可达（D1 口径-2）；操作经 `docker compose` CLI 参数数组（ADR-013） |
| backup_job | 聚合根 | 计划任务与备份（cron-backup） | —（backup_run 按本对象标识归组，为同域聚合引用非父子隶属） | 名称、备份源清单（内嵌值对象集合）、目标（内嵌值对象：类型+本地路径/rsync 地址）、保留份数、调度表达式（cron 语义，D2 口径-2）、创建/更新时间 | backup_source_type{site_dir,database}；backup_target_type{local,rsync} | 落 SQLite（D0 拍板，本模块持久化对象①：任务定义无法从外部推导必须面板自持）；一致性边界=任务定义整体（源清单+目标+保留份数+调度不可分，编辑=整体替换——调度器消费的必须是完整定义）；无状态机（无启停开关 OUT；「最近执行结论」为 backup_run 回显非任务状态字段）；A1 ✓/A2 ✓（任务清单与调度器按任务标识寻址）/A3 ✓（创建/编辑/删除为唯一定义与终结入口）/A4 ✗ |
| backup_run | 聚合根 | 计划任务与备份（cron-backup） | —（按 job 标识归组：同域聚合引用） | 任务引用、触发方式、状态、开始时间、结束时间、耗时（派生）、产物路径、产物大小、失败原因 | backup_run_status{running,success,failure}；backup_trigger{cron,manual} | 落 SQLite（D0 拍板，持久化对象②：执行事实与产物台账无法从外部推导必须面板自持）；一致性边界=记录↔产物↔轮转水位（成功=记录转 success+产物落盘、轮转=记录与产物同删，两半不可分）；状态机 ADR-016（running→success/failure 终态不可逆，重新执行=新 run；超时按 failure 终态记不扩枚举）；同任务执行互斥（D2 口径-4）；轮转=保留最近 N 条 success 及产物、更早 success 与更早 failure 一并清理（D2 口径-5）；A1-A4 全过（4/4） |
| 备份源清单 | 值对象 | 计划任务与备份（cron-backup） | 内嵌于 backup_job（1:N 值集合） | 源类型 + 站点目录路径（site_dir：值快照）或 实例引用+库名（database：db_instance 关联引用） | backup_source_type{site_dir,database} | 无独立标识（同型同址即等价）、不可变只能替换（编辑任务=整体替换源清单）；站点目录项为路径值快照（site 为实况投影无聚合根 ID，选取时联动 site-mgmt 站点清单、保存后不持续引用）；database 项为本域唯一关联领域对象载体（见第五部分） |
| 备份目标 | 值对象 | 计划任务与备份（cron-backup） | 内嵌于 backup_job（1:1） | 目标类型 + 本地目录路径（local）或 rsync 远程地址（rsync） | backup_target_type{local,rsync} | 无独立标识（同型同址即等价）、不可变只能替换；rsync 认证凭据承载待 D3（若需按密文值对象与 db_instance 凭据同构，prd-cron-backup.md 待对齐-6）；rsync 推送为用户主动配置传输（R-09 例外语义） |
| cron_job | 域内对象-实体 | 计划任务与备份（cron-backup） | 投影集合条目（经读取契约以 crontab 内任务行定位直接寻址，无聚合根守门；系统预置位条目为只读投影分区） | cron 表达式、命令、备注（面板侧，注释行承载）、来源标注 | cron_source{panel,existing} | crontab 实况投影不落库（D2 口径-1 零任务台账：crontab 文件是唯一事实源，面板外编辑如实反映——与 site 零站点台账/firewall 零规则台账/database-mgmt 零库用户台账同哲学）；备注与来源标注以 crontab 注释行承载（面板写回附加标记注释，缺失按 existing 呈现；语法 D3 定 prd-cron-backup.md 待对齐-1）；标识=crontab 内任务行定位（跨读取可寻址，稳定形态 D3 定）故实体而非值对象——与 systemd_service/firewall_rule 同构；无状态机（任务存在即状态） |
| env_install_task | 聚合根 | 环境管理（env-mgmt）（v1.19 追加；v1.22 +引导结果字段） | —（环境列表在途标记按 env_type 归组派生，非父子隶属） | 目标环境（env_type）、动作、状态、发起者、发起时间、开始/结束时间、命令输出尾部（终态定格）、失败原因、**引导结果（bootstrap_status，终态定格，NULL=无引导步骤）、引导摘要（bootstrap_note，不含密码）** | env_task_status{pending,running,succeeded,failed}；env_task_action{install,upgrade,uninstall}；**bootstrap_status{succeeded,failed,skipped}（v1.22 入池）** | 落 SQLite（D2 落定：执行事实与输出台账无法从包管理器状态完整推导必须面板自持，与 backup_run 同构）；一致性边界=任务记录↔执行进程↔同环境互斥约束（创建=台账落库+互斥生效同事务原子，终态=结果落库+互斥释放）；四态状态机 ⚠️A3（终态不可逆，重新发起=新任务；面板重启恢复 running→failed 如实呈现）；**安装后配置就绪为聚合根后置机制（v1.22，⚠️A11~A15）：数据库类安装成功后 worker 经 db-mgmt OHS 引导，bootstrap_status/note 随终态同事务定格——引导结果为独立维度非新状态机（任务 status 仍为包操作四态，引导失败不改 succeeded，⚠️A14）；引导非新聚合根（无独立一致性边界，结果依附任务台账；建账户/注册实例与凭据归 db-mgmt db_instance 聚合）**；A1-A4 全过（4/4） |
| 环境条目（env entry） | 域内对象-实体 | 环境管理（env-mgmt）（v1.19 追加；v1.22 七值同步；v1.24 八值同步） | 投影集合条目（目录条目编译期常量+实况运行时探测合并，无聚合根守门） | 目录字段（显示名/描述，编译期；v1.1a 去关联模块提示）+ 实况字段（已装、版本、服务运行态、自启态、通道可用性、在途任务标记——每请求实时探测；**node 服务态/自启态不适用 null，⚠️A17**） | env_type{nginx,apache,docker,mysql,redis,php_fpm,mongodb,node}（v1.24 八值同步 +node）；package_manager_family{apt,dnf,yum}（通道探测结论） | 零环境台账不落库（D2 判定：包管理器数据库与 systemd 为事实源，实况探测投影——与 site 零站点台账/systemd_service 投影同哲学；目录条目本体为编译期常量随代码分发非用户可配 ⚠️A1）；标识=env_type 目录键（八值稳定可寻址，服务操作/变更发起按其寻址；node 运行时型无服务单元不在服务操作集 ⚠️A17）故实体而非值对象——与 systemd_service 同构；已装/服务状态为实况投影非状态机（prd-env-mgmt.md 第六部分辨析） |
| 发行版映射（distro mapping） | 值对象 | 环境管理（env-mgmt）（v1.19 追加；v1.24 +node 条目） | 内嵌于环境条目（编译期，1:N per 发行版族） | 每族的包名、systemd 服务单元名、版本查询方式、是否需官方仓库源 | —（族判定经通道探测结论承载，非对象字段枚举） | ⚠️A7 编译期映射表（Debian：nginx/apache2/docker.io/mysql-server/redis-server/php-fpm；RHEL 系：nginx/httpd/docker-ce/mysql-server/redis/php-fpm 等；**v1.24 +node：nodejs 三系同/单元映射空/版本查询 node --version/需官方仓库源=true〔NodeSource 三系代配 ⚠️A16，LTS 轨道钉死，npm 随包〕**）；无独立标识（环境+族唯一确定）、不可变（随面板版本更新）；非用户可配数据（与枚举池同性质） |
| db_tool（工具部署档案） | 聚合根 | 数据库工具集成（db-tools）（v1.24 追加） | —（tool_deploy_task 终态驱动其登记/更新为同域跨聚合同事务协作；工具列表在途标记按 toolType 归组派生） | 工具类型、已装版本、部署路径、站点引用、服务单元引用（mongo-express）、目标实例引用（mongo-express 项，db_instance 关联引用）、工具凭据密文（内嵌值对象） | db_tool_type{phpmyadmin,mongo_express} | 落 SQLite（D2 落定：**工具凭据密文无法从外部状态推导必须面板自持**，与 db_instance/dns_credential 同构论证；部署档案为面板托管部署记录——工具部署为面板专属通道，区别于环境条目零台账〔apt 多通道〕）；一致性边界=部署档案↔凭据密文↔部署落位（安装=档案+密文同事务落库，卸载=同删，轮换=密文替换+注入配置更新）；**运行态/依赖就绪度为实况投影字段不持久化**（systemd/站点/env-mgmt 实况消费，与 instance_conn_state 探测态同口径）；无状态机（部署状态=档案存在性+实况投影，非状态机——db_tool_state 不采用入池，理由见池行）；phpMyAdmin 项不持固定实例引用（signon 每次打开绑定实例，多实例可达）；mongo-express 项部署时绑定目标实例（凭据变更刷新时机待对齐-5）；**v1.26：站点引用=落地引用 siteRef 值对象（+Port/AutoCreated，JSON 载荷扩展零 DDL，见本表落地引用行）** |
| tool_deploy_task（工具部署任务） | 聚合根 | 数据库工具集成（db-tools）（v1.24 追加） | —（与 db_tool 为同域两个独立聚合：执行事实与部署档案各自一致性边界，终态驱动档案登记/更新同事务协作——backup_job/backup_run 同构） | 目标工具（db_tool_type）、动作、状态、发起者、发起时间、开始/结束时间、命令输出尾部（终态定格）、失败原因 | db_tool_task_status{pending,running,succeeded,failed}；db_tool_task_action{install,upgrade,uninstall} | 落 SQLite（D2 落定：执行事实与输出台账无法从部署实况完整推导必须面板自持，与 env_install_task/backup_run 同构）；一致性边界=任务记录↔执行进程↔同工具互斥约束（创建=台账落库+互斥生效同事务原子，终态=结果落库+互斥释放+档案登记/更新）；四态状态机 ⚠️A3（终态不可逆，重新发起=新任务；面板重启恢复 running→failed 如实呈现）；A1-A4 全过（4/4）；**v1.26：失败原因字段文本构造扩展——error 含 errcode data.reason 拼接（⚠️A17/FR-14，DBT_012 端口占用/多候选等具体原因任务详情可见，字段语义与状态机不变）** |
| 工具凭据 | 值对象 | 数据库工具集成（db-tools）（v1.24 追加） | 内嵌于 db_tool（密文列整体加密） | basic auth 用户名+口令密文（mongo-express）/ signon 密钥密文（phpMyAdmin） | — | AES-256-GCM 密文落库（密钥文件 0600 沿 CredentialVault 先例，文件名 D3 定）；明文仅内存态生成与注入使用、任何接口不回显、审计脱敏（I-02）；不可变替换式更新（轮换=替换，旧口令即时失效）；**数据库连接凭据不归本值对象**——取自 database-mgmt vault（关联领域对象消费，本域不复制不持久化对方凭据） |
| 工具条目（tool entry） | 域内对象-实体 | 数据库工具集成（db-tools）（v1.24 追加） | 投影集合条目（目录条目编译期常量+实况运行时探测合并，无聚合根守门） | 目录字段（显示名/描述/目标引擎/钉死版本，编译期）+ 实况字段（部署状态〔db_tool 档案存在性+版本〕、运行态、依赖就绪度、在途任务标记——每请求实时探测） | db_tool_type{phpmyadmin,mongo_express} | 零工具台账不落库（目录条目本体为编译期常量随代码分发；运行态/依赖就绪度实时探测不缓存——与 env entry 同哲学）；标识=db_tool_type 目录键（两值稳定可寻址）故实体而非值对象——与 env entry/systemd_service 同构；部署状态非状态机（档案存在性+实况投影） |
| 目录映射（tool catalog mapping） | 值对象 | 数据库工具集成（db-tools）（v1.24 追加） | 内嵌于工具条目（编译期，1:1 per 工具） | 钉死版本、分发形态（tarball+sha256 / npm 包+精确版本）、目标引擎、部署形态常量（端口/目录/baseUrl 前缀） | —（分发形态为编译期常量非枚举字段） | ⚠️A1/A12 编译期映射（phpMyAdmin 官方 tarball+sha256；mongo-express npm 精确版本；DOUGONG_MIRROR 同构镜像选项）；无独立标识（工具唯一确定）、不可变（随面板版本更新）；非用户可配数据（与发行版映射同性质）；具体常量值 D3 定稿（prd-db-tools.md 待对齐-4）；**v1.26 +ToolLandingPort=18090/自动块文件路径 {conf.d}/dougong-tools.conf 编译期常量（⚠️A14，不可用户配 OUT）** |
| 落地引用（siteRef） | 值对象 | 数据库工具集成（db-tools）（v1.26 显式建模） | 内嵌于 db_tool（site_ref JSON 列整体序列化） | 来源文件路径（File）、server 块定位（Index）、ServerName、Scheme（既有）+ **Port（落地监听端口，既有 80/443 块 Port=0 表默认）、AutoCreated（是否面板自动创建，清块判定用）**（v1.26 扩展） | —（Port 数值/AutoCreated 布尔，非枚举不入池，与 totp_enabled 同口径） | ⚠️A14/A15：安装落档时定格（撤除/打开/清块寻址用）；无独立标识（落点定位值相同即等价）、不可变只能替换（重装/域名联动刷新=替换引用值）；**JSON 载荷扩字段零 DDL 变更**（TEXT 列不变，旧档案缺省 Port=0/AutoCreated=false 向后兼容沿旧形态，取证 inject.go:20-25/api-db-tools.md:237/:554）；toolBaseHost Port≠0→scheme://hostname:Port URL 分支消费（FR-12）；自动落地块本体零台账（块存在性事实源=nginx 配置文件，与 site 零站点台账同哲学） |

> role 为固定枚举非实体（MVP 不做角色 CRUD）；「关联领域对象」在本域不存在（见第五部分）。
> 面板基座（panel-core）：版本标识/模块挂载点清单/更新检查比对结果均为过程数据（prd-panel-core.md 过程数据分类表），不入本清单；面板证书 ACME 申请/续期在途与挑战 location 临时注入/撤除过程为机制链路过程数据（ADR-029，v1.16 增量），不入本清单；无关联领域对象（见第五部分）。
> 系统监控（monitoring）：环形缓冲为 metric 值对象的进程内集合载体（基础设施承载），不单独建模为领域对象；无实体与聚合根——只读观测流无可变个体对象（metric 条目按「属性+时间戳等价、不可变、无跨聚合引用」判值对象，判定见 prd-monitoring.md 第六部分辨析与第一部分聚合根判定）；时间窗口/维度选择参数、快照与序列响应、趋势图呈现状态均为过程数据（prd-monitoring.md 过程数据分类表），不入本清单；无关联领域对象（见第五部分）。
> systemd 服务管理（service-mgmt）：投影集合为 systemd_service 实体的内存态集合载体（dbus 读取时整集合刷新，基础设施承载），不单独建模为领域对象；无聚合根（第一部分判定）——投影流无守门需求，与 monitoring 同构、差异在对象有稳定标识且状态可变（实体而非值对象，判定见 prd-service-mgmt.md 第六部分辨析）；状态过滤/名称搜索参数、unit 属性视图响应、操作在途状态、操作任务（Job）等待中间态均为过程数据（prd-service-mgmt.md 过程数据分类表），不入本清单；无关联领域对象（见第五部分）。
> 文件管理器（file-manager）：投影集合为 file_entry 实体的实时投影载体（目录读取即呈现，基础设施承载），不单独建模；`.dougong-trash/` 物理目录与 trash 内唯一标识子目录为 trash_entry 聚合的存储载体，不单独建模为领域对象；属主候选清单（system_user/system_group 本机系统读取）、分块上传任务状态、编辑器内容缓冲、面包屑/浏览位置、冲突决策交互、trash 内标识生成规则均为过程数据（prd-file-manager.md 过程数据分类表），不入本清单；无关联领域对象（见第五部分）。
> 网站管理（site-mgmt）：站点投影集合为 site 实体的扫描投影载体（配置文件是唯一事实源，扫描读取即呈现，基础设施承载），不单独建模；conf.backup/时间戳/ 快照目录为安全写入机制的存储载体（恢复点+配置原样历史，保留策略 prd-site-mgmt.md 待对齐-10），与 `.dougong-trash/` 同口径不建模；ACME 账户密钥与 lego 客户端内部状态（order/authorization）为 ACL 技术载体不入领域模型；PHP-FPM pool 候选清单、nginx -t/reload/续期在途状态、编辑器未保存编辑态、失败 diff 与引用提示呈现均为过程数据（prd-site-mgmt.md 过程数据分类表），不入本清单；location 列表为 site 解析投影字段（无独立业务语义）不单独建模。
> 数据库管理（database-mgmt）：慢日志条目/进程列表条目/key 条目/状态概览摘要为引擎实况只读投影（事实源在引擎侧，面板无留存语义，读取响应即消亡——区别于 monitoring metric 的面板侧缓冲留存），不单独建模为领域对象；备份导出 SQL 文件为动作产物存储载体（数据快照，落位 D3 定，与 conf.backup 快照同口径）；探测管线为后台机制链路；key 类型与字符集候选为实况开放集合（过程数据），不入本清单；无关联领域对象（见第五部分）。v1.23 补充：表/行/集合/文档为投影实体入清单（上表 4 行，有标识可寻址——行写/文档 CRUD 按其定位，区别于 key 条目只读浏览的过程数据口径）；列结构为 data_table 元数据字段不单独建模；表存储引擎/排序规则为实况开放集合（沿字符集先例不入枚举池）；分页参数/JSON 编辑态/面包屑动态段/tooltip 文案/单列主键判定结果为过程数据（prd-database-mgmt.md v1.2 过程数据表）。
> Web 终端（terminal）：终端输出/输入字节流、窗口尺寸同步、断开通知呈现、多标签 UI 状态、系统用户候选清单均为过程数据（prd-terminal.md 过程数据分类表），不入本清单；PTY 进程为聚合的运行时载体（基础设施承载）不单独建模；terminal_end_reason 为断开事件载荷枚举（内存态对象无持久状态字段，经审计事件落 account-auth），不构成会话对象的状态字段。
> 防火墙（firewall）：监听端口清单条目与开放状态对照结论为系统实况只读投影+规则对照读取时派生值（事实源在系统网络栈与防火墙后端，面板无留存语义，读取响应即消亡，与 database-mgmt 慢日志/进程列表条目同口径），不入本清单；域级判定值 firewall_backend（识别结论）与对照派生值 listen_port_state 为整值刷新的枚举值，不构成对象状态字段；无聚合根（第一部分判定）——投影流无守门需求，与 monitoring/service-mgmt 同构、差异在对象有稳定标识且被删除操作寻址（实体而非值对象，判定见 prd-firewall.md 第六部分辨析）；规则清单筛选参数、页头后端状态摘要、添加/删除在途、无后端引导视图、保护警示交互均为过程数据（prd-firewall.md 过程数据分类表），不入本清单；无关联领域对象（见第五部分）。
> 容器管理（docker-mgmt）：五类清单（容器/镜像/卷/网络/Compose 项目）为各自实体的 daemon 实况投影载体（SDK 读取即呈现，基础设施承载），不单独建模为领域对象；域级判定值 docker_daemon_state（接管结论：available/unavailable）为整值刷新的枚举值，不构成对象状态字段（与 firewall_backend/instance_conn_state 同构）；容器资源占用快照与 Compose 项目运行态汇总为读取时派生值（响应即消亡，与 firewall 监听端口对照同口径）；日志流内容为 daemon 实况流式读取（不持久化不轮转，OUT）；无聚合根（第一部分判定）——投影流无守门需求，与 monitoring/service-mgmt/firewall 同构、差异在对象有稳定标识且被生命周期与删除操作寻址（实体而非值对象，判定见 prd-docker-mgmt.md 第六部分辨析）；五类清单筛选参数、页头接管结论摘要、启停/拉取/up-down 在途、未装引导视图、删除确认与数据销毁警示交互均为过程数据（prd-docker-mgmt.md 过程数据分类表），不入本清单；无关联领域对象（见第五部分）。
> 计划任务与备份（cron-backup）：crontab 投影集合与系统预置位清单为 cron_job 实体的实况投影载体（读取即呈现，基础设施承载），不单独建模为领域对象；面板标记注释行为来源标注与备注的存储载体（crontab 文件的一部分，非领域对象）；backup_run 的 running 为聚合内过程态（非独立状态机）；表达式校验与预览结果、执行日志尾部内容、手动触发输出、备份在途进度、站点/数据库选取候选清单、调度器内部计时状态均为过程数据（prd-cron-backup.md 过程数据分类表），不入本清单；关联领域对象仅 db_instance 一项（database 项引用，见第五部分——站点目录项为值快照不构成引用）。
> 环境管理（env-mgmt，v1.19 追加）：环境条目为「编译期目录常量+运行时实况探测」的合并投影载体（包管理器/systemd 为事实源，读取即呈现，基础设施承载），实况字段不落库；命令输出尾部 ring 缓冲（执行中）为 P7 计算中间值（终态定格落 env_install_task.output_tail）；服务操作在途、安装/卸载发起在途、风险提示确认交互、任务列表筛选参数均为过程数据（prd-env-mgmt.md 过程数据分类表），不入本清单；无关联领域对象（见第五部分——docker-mgmt/database-mgmt/site-mgmt 为软引导被消费方向，不持有对方聚合根 ID）。v1.24 补充：node 条目为运行时型环境（无服务单元，服务态 null/启停入口禁用 ⚠️A17），对象分类不变（仍投影实体）；NodeSource 源代配为 envpack 适配器机制（⚠️A16，⚠️A10 同构），不新增对象。
> 数据库工具集成（db-tools，v1.24 追加）：工具条目为「编译期目录常量+运行时实况探测」的合并投影载体（部署状态=db_tool 档案存在性+实况，运行态/依赖就绪度事实源在 systemd/站点/env-mgmt 实况，读取即呈现，基础设施承载），实况字段不落库；命令输出尾部 ring 缓冲（执行中）为 P7 计算中间值（终态定格落 tool_deploy_task.output_tail）；**signon 令牌与消费台账为 P7 敏感过程数据（短时效一次性安全凭据，内存态签发/消费/TTL 清扫，面板重启全部失效=安全保守，不持久化——区别于 db_tool 凭据密文的持久化语义）**；basic auth 口令明文/Mongo 连接凭据明文/signon 密钥明文为 P7 敏感（仅生成与注入内存态使用，密文落 db_tool 或取自 db-mgmt vault，零回显 I-02）；安装/升级/卸载在途、打开 redirect URL、高级入口按钮态、任务列表筛选参数均为过程数据（prd-db-tools.md 过程数据分类表），不入本清单；关联领域对象 db_instance 一项（mongo-express 目标实例引用+凭据 OHS 消费，见第五部分）。**v1.26 补充：落地引用（siteRef）为持久化值对象入清单（上表行，db_tool.site_ref JSON 列，+Port/AutoCreated 零 DDL）——区别于过程数据；自动落地块本体零台账不入清单（块存在性事实源=nginx 配置文件 dougong-tools.conf，server_name 为配置值随绑定状态刷新，面板不存储块状态，与 site 零站点台账同哲学）；ToolLandingPort/块文件路径为编译期常量（随目录映射承载）；配置变更钩子事件载荷为 P5 集成载荷、端口占用探测结果/块存在性判定为 P7 计算中间值（prd-db-tools.md v1.2 过程数据表），不入本清单。**

## 第四部分：子流程 I/O 契约

| 子流程 | 触发条件 | 输入（来自父聚合根） | 输出（写入父聚合根） | 输出写入哪个对象 |
|--------|---------|------------------|------------------|---------------|
| 审计过期清理任务 | 后台每日调度 | 当前时间 + 审计保留策略（90 天） | 过期审计批次删除结果（批次条数） | audit_log（清理动作审计记录） |
| 自动续期任务（site-mgmt） | 证书转 expiring_soon（到期前 30 天，ADR-024）后台调度；renew_failed 手动重试进入同一通道；v1.18：手动立即续期（FR-28，valid/expiring_soon/renew_failed 发起）进入同一通道——失败迁移分流（valid 保持 valid+last_error，其余转 renew_failed）；v1.20：dns_01 证书续期输入含 dns_credential 引用（凭证缺失=失败 last_error=凭证缺失） | certificate（域名列表、挑战方式 acme_challenge 含 dns_01、托管路径、ACME 账户引用、DNS 凭证引用〔dns_01，v1.20〕） | 续期结果：成功=托管证书文件原路径替换（⚠️A16）+状态回 valid+触发重部署引用站点；失败=转 renew_failed+失败原因（valid 手动续期失败例外：保持 valid） | certificate 聚合根（cert_status 字段与托管文件） |
| 面板证书自动续期任务（panel-core，v1.16 增量） | 面板证书（acme 来源）转 expiring_soon（到期前 30 天，沿 ADR-024 阈值）后台调度；renew_failed 手动重试进入同一通道 | panel_cert（绑定域名、挑战方式固定 http_01、ACME 账户引用） | 续期结果：成功=证书替换+状态回 valid+HTTPS 模式联动确认；失败=转 renew_failed+失败原因落库 | panel_cert（panel_config 聚合内 HTTPS 证书实体的状态与证书材料，ADR-029） |

> 登录尝试计数、会话滑动续期为聚合内即时行为，不构成独立聚合子流程。
> 面板基座（panel-core）：1 个子流程——面板证书自动续期任务（v1.16 增量，ADR-029，与 site-mgmt 自动续期任务同构、经跨模块通道执行，见第四部分表行）；配置保存即生效/重启提示为聚合内即时行为；安装脚本为部署期服务器侧一次性流程（非面板页面）；更新检查为单动作调用，均不构成独立聚合子流程。
> 系统监控（monitoring）：无子流程——周期采样与分钟聚合为后台机制链路（非业务聚合，无独立生命周期状态）；历史趋势查询为只读读取；均不构成独立聚合子流程。
> systemd 服务管理（service-mgmt）：无子流程——启停/重启为「提交→等待终态→反馈」的单动作过程（FR-08 机制，无独立生命周期状态，非多轮/审批形态）；自启切换为单配置动作；服务发现纳管为后台机制链路；均不构成独立聚合子流程。
> 文件管理器（file-manager）：无子流程——分块上传为「分块依次上传→合并原子落盘」的单任务过程（一次性执行状态不持久化，断点续传二期 OUT-4）；压缩/解压为单动作执行；回收站入站登记为删除动作的同步机制链路（FR-16，无独立生命周期状态）；均不构成独立聚合子流程。
> 网站管理（site-mgmt）：1 个子流程——自动续期任务（外部 CA 异步签发过程，有独立执行生命周期，与 account-auth 审计清理任务同构）；安全写入四步（备份→修改→nginx -t→回滚）为同步机制链路无独立生命周期状态（与 file-manager 回收站入站登记同口径），不构成子流程；单次续期执行的排队/执行中状态为过程数据（P7），不单独建模。
> 数据库管理（database-mgmt）：无子流程——备份导出/导入为「提交→执行→产出/反馈」的单动作过程（一次性执行无独立生命周期状态，与压缩/解压同口径）；存量探测为后台机制链路（与 nginx 扫描/服务发现纳管同口径）；连接验证为保存动作的同步环节；均不构成独立聚合子流程。
> Web 终端（terminal）：无子流程——空闲/硬上限判定为聚合内后台定时机制（无独立生命周期状态，与登录尝试计数/会话滑动续期同口径）；面板会话失效联动为跨域事件触发的即时断开（消费事件执行动作，非本域多轮过程）；均不构成独立聚合子流程。
> 防火墙（firewall）：无子流程——后端识别为进入模块的只读判定链路（探测即结论、无独立生命周期）、规则增删为单动作即时执行（与 database-mgmt 建删库同口径）、面板端口保护警示确认为提交动作的同步前置环节（交互层危险操作防护、非审批流）、firewalld 双写为命令执行口径非独立过程；均不构成独立聚合子流程。
> 容器管理（docker-mgmt）：无子流程——容器启停/重启/暂停恢复/删除为「提交→daemon 执行→终态反馈」的单动作过程（与 service-mgmt 启停同构）；镜像拉取为「提交→执行→进度反馈」的单动作过程（一次性执行无独立生命周期状态，与 database-mgmt 备份导出/压缩解压同口径）；Compose up/down/restart 为单动作 CLI 执行（终态反馈即消亡）；label 识别为读取时聚合的机制链路（与 nginx 扫描/服务发现纳管同口径）；日志跟踪为流式读取非过程聚合；均不构成独立聚合子流程。
> 计划任务与备份（cron-backup）：无子流程——备份执行过程（running→success/failure）为 backup_run 聚合根自身的状态机（run 即聚合根本体，非服务于父聚合根的独立过程）；定时调度执行为面板内置调度器的后台机制链路（触发即创建 run，自身无独立生命周期状态，与服务发现纳管/nginx 扫描同口径）；轮转清理为成功执行后随终态回写触发的治理环节（区别于审计过期清理任务/自动续期任务的独立调度形态——两者为后台定时任务，本域轮转无独立调度周期）；手动触发为单动作代执行（终态反馈即消亡）；均不构成独立聚合子流程。
> 环境管理（env-mgmt，v1.19 追加）：无子流程——环境变更任务过程（pending→running→succeeded/failed）为 env_install_task 聚合根自身的状态机（task 即聚合根本体，非服务于父聚合根的独立过程，与 backup_run 同构判定）；实况探测为请求时只读机制链路（与 nginx 扫描/服务发现纳管同口径）；服务启停为「提交→等待终态→反馈」的单动作过程（与 service-mgmt 启停同构，复用同一通道资产）；任务执行 worker 为后台机制链路（消费任务台账非独立调度周期任务——无定时触发，创建即触发）；面板重启恢复为启动扫描的即时环节；均不构成独立聚合子流程。
> 数据库工具集成（db-tools，v1.24 追加）：无子流程——工具部署任务过程（pending→running→succeeded/failed）为 tool_deploy_task 聚合根自身的状态机（与 env_install_task/backup_run 同构判定）；**signon 桥接为「打开动作→令牌签发→服务端回调校验→凭据注入」的同步机制链路（无独立生命周期状态，令牌消费台账为 P7 过程数据，与连接验证为保存动作同步环节同口径）**；gating 为逐请求只读校验机制（无状态变更无生命周期）；实况探测为请求时只读机制链路；部署 worker 为后台机制链路（创建即触发非定时调度）；面板重启恢复为启动扫描即时环节；凭据轮换为「提交→配置更新→重启→反馈」单动作过程；均不构成独立聚合子流程。**v1.26 补充：自动建块/清块为安装/卸载任务链的机制步骤（结果随任务终态，无独立生命周期状态）；域名联动刷新（钩子/惰性）为 bindDomain 变更事件触发的配置同步机制（成功=块 server_name 一致，失败=记日志+惰性兜底，无中间态）——与「连接验证为保存动作同步环节」同口径，不构成子流程；HostGuard 内部端点豁免为 panel-core 网关机制变更（非本域过程）。**

## 第五部分：关联领域对象

| 关联领域对象 | 所属业务域 | 关联字段 | 需获取信息 | 触发关系 | 映射模式 |
|------------|--------|---------|----------|---------|---------|
| —（无） | — | — | 本域不持有其他域聚合根 ID 引用；跨域协作方向为「被依赖」（审计接口被调用、会话鉴权被复用） | — | OHS（见第六部分） |
| db_instance（数据库备份源引用） | 数据库管理（database-mgmt） | backup_job 源清单 database 项：实例引用 + 库名 | 实例/库清单（任务创建/编辑联动选取）；实例连接（执行时经 database-mgmt 契约用其已存凭据建连，凭据值不进入本域） | 创建/编辑时选取联动；执行时建连取数；实例移除纳管后引用悬空→执行失败如实记录（不级联删任务，不管理对方生命周期） | OHS（消费）+ ID 引用（不复制对方模型） |
| db_instance（工具凭据 OHS 引用，v1.24 追加） | 数据库管理（database-mgmt） | db_tool 目标实例引用（mongo-express 项，部署时绑定）；signon 令牌实例绑定（phpMyAdmin 打开时选取，令牌为过程数据不持久化） | 实例凭据取出（vault 解密仅服务端，经 loopback/systemd env 通道注入工具，凭据值不进入本域持久化）；实例纳管态与凭据就绪校验（打开/安装前置） | 安装/打开时凭据就绪校验；部署时 Mongo 凭据注入；signon 打开时 MySQL 凭据注入；实例移除纳管/凭据删除后→打开/注入如实拒绝并引导配置（不级联删工具，不管理对方生命周期；mongo-express 注入刷新时机待对齐-5） | OHS（消费，进程内单向）+ ID 引用（不复制对方凭据模型） |

> 待跨域对齐：若 panel-core 将全局配置统一收编 panel_config，会话有效期策略可转为对其的关联引用（prd-account-auth.md 第九部分已标注）。
> 面板基座（panel-core）：无关联领域对象——配置变更经 account-auth OHS 写审计（R-03）、会话参数经其 OHS 只读透传展示（ADR-019），均不持有对方聚合根 ID；面板证书 ACME 签发/续期对 site-mgmt AcmeGateway 与 nginx 安全写回为 OHS 服务消费（ADR-029，第六部分），不持有 certificate 聚合根 ID，仍不构成关联领域对象；上条 account-auth 待跨域对齐经本域 D2 关闭：会话有效期策略仍归 account-auth 自持，panel-core 仅透传展示、不收编。
> 系统监控（monitoring）：无关联领域对象——不持久化任何外部域 ID（ADR-014 不落库），跨域方向为「被依赖」（监控读取契约被 panel-core 仪表盘消费，见第六部分）。
> systemd 服务管理（service-mgmt）：无关联领域对象——零持久化不持有任何域聚合根 ID（投影对象标识为 systemd 侧 unit 名称，非面板域对象）；跨域方向为「消费方」（遵奉会话鉴权、消费审计写入 OHS，见第六部分）。
> 文件管理器（file-manager）：无关联领域对象——trash_entry 操作者字段为会话上下文的审计留痕快照（字符串），非 panel_user 聚合根引用（与 audit_log 操作者字段同口径）；属主修改候选 system_user/system_group 为本机系统实况实时读取（下拉选项过程数据，kb 对象），非面板域对象；跨域方向为「消费方」（遵奉会话鉴权、消费审计写入 OHS，见第六部分）。
> 网站管理（site-mgmt）：无关联领域对象——site 与 certificate 同属本域（证书⇄站点为域内引用关系，非跨域聚合引用）；certificate 操作者/审计字段口径同上；PHP-FPM pool 候选为本机系统实况读取（过程数据），ACME CA 为外部系统经 ACL 对接（见第六部分，非业务域聚合根）；跨域方向为「消费方」（遵奉会话鉴权、消费审计写入 OHS、经 ACL 外呼 ACME，R-09 唯一例外）。
> 数据库管理（database-mgmt）：无关联领域对象——不持有任何外部域聚合根 ID（审计操作者字段为会话上下文快照字符串，与 audit_log 同口径）；被依赖方向已兑现三轮：cron-backup 备份源复用 DumpStream（v1.14/v1.15）、env-mgmt 安装后配置就绪 BootstrapAfterInstall（v1.22）、**db-tools 实例凭据 OHS（v1.24——signon 桥接/mongo-express 注入取出 vault 凭据，进程内单向，凭据不出服务端边界）**；跨域方向为「消费方」（遵奉会话鉴权、消费审计写入 OHS，见第六部分）。
> Web 终端（terminal）：无关联领域对象——不持有任何外部域聚合根 ID（terminal_session 的鉴权会话引用为运行时校验凭据与联动断开触发源，会话失效语义经事件消费而非聚合引用；操作者字段为会话上下文快照字符串，与 audit_log 同口径）；system_user 为本机系统实况只读引用（候选清单为过程数据，与 file-manager 属主候选同口径，非面板域对象）；docker-mgmt 容器内终端为二期「被依赖」预期方向（不预先建模，与 cron-backup 同口径）；跨域方向为「消费方」（遵奉会话握手与失效联动、消费审计写入 OHS，见第六部分）。
> 防火墙（firewall）：无关联领域对象——零持久化不持有任何域聚合根 ID；面板端口标注与涉险判定的基准为 panel-core 监听配置的实况值消费（panel_config 单例值只读读取，非聚合根 ID 引用，读取通道 D3 定 prd-firewall.md 待对齐-6）；跨域方向为「消费方」（遵奉会话鉴权、消费审计写入 OHS 与面板监听配置，见第六部分）。
> 容器管理（docker-mgmt）：无关联领域对象——零持久化不持有任何域聚合根 ID（审计操作者字段为会话上下文快照字符串，与 audit_log 同口径；compose_project 的容器清单为对 container 的同域引用集合，非跨域引用）；terminal 容器内终端为二期「被依赖」预期方向（terminal 域第五部分已同口径登记，二期前不预先建模，与 database-mgmt 对 cron-backup 同口径）；跨域方向为「消费方」（遵奉会话鉴权、消费审计写入 OHS，见第六部分）。
> 计划任务与备份（cron-backup）：**全项目首个真实关联领域对象引用**——db_instance（database-mgmt 聚合根）经 backup_job 源清单 database 项以 ID 引用（上表行，database-mgmt 第二部分「被依赖预期」就此兑现）；站点目录项为路径值快照（site 为实况投影无聚合根 ID，选取时联动 site-mgmt 站点清单、保存后不持续引用——区别于 database 项的持续 ID 引用）；backup_run 的任务引用为同域聚合引用（cron-backup 域内 job→run，非跨域）；跨域方向为「消费方」（遵奉会话鉴权、消费审计写入 OHS 与 database-mgmt/site-mgmt 清单契约，见第六部分）。
> 环境管理（env-mgmt，v1.19 追加）：无关联领域对象——零环境台账不持有任何域聚合根 ID（审计操作者字段为会话上下文快照字符串，与 audit_log 同口径）；环境未装实况被 docker-mgmt/database-mgmt/site-mgmt 引导态**软引导消费**（无跨模块 API 硬依赖，关联模块 PageSpec 指引链接，AA 附录B）；与 service-mgmt 的 SysdGateway 共享为平台支撑层适配器资产共享（非业务域集成，见第六部分注行）；跨域方向为「消费方」（遵奉会话鉴权、消费审计写入 OHS，见第六部分）。v1.24 补充：+db-tools 软引导被消费方向（依赖缺失引导）与环境实况读契约被消费（PHP-FPM/Node/nginx 依赖就绪度，进程内读非 ID 引用），仍无关联领域对象。
> 数据库工具集成（db-tools，v1.24 追加）：**关联领域对象 db_instance 一项**（上表行——mongo-express 项 db_tool 目标实例引用为持久 ID 引用〔部署时绑定〕；phpMyAdmin signon 令牌实例绑定为过程数据不持久化；凭据经 OHS 取出仅服务端通道流转，本域不复制不持久化对方凭据——与 cron-backup「凭据值不进入本域」同口径）；工具状态与打开能力被 database-mgmt 高级管理入口**读契约消费**（进程内读非持久 ID 引用，入口渲染归 database-mgmt PageSpec 跨模块待同步项）；与 service-mgmt 的 SysdGateway 共享为平台支撑层适配器资产共享（非业务域集成）；account-auth gating 会话校验为 OHS 消费（db-tools 自持 GET /dbtool/gate/verify 复用既有 VerifyWs 只读会话校验，account-auth 零新增 HTTP，api D-11 定稿 v1.25 W2 回填）；跨域方向为「消费方」（遵奉会话鉴权、消费审计写入 OHS/env-mgmt 实况/site-mgmt 站点通道/db-mgmt 凭据 OHS，见第六部分）。**v1.26 补充：panel_config.bind_domain 为实况值消费（PanelAccess 读契约既有，toolBaseHost/惰性校正/server_name 初值数据源）非聚合根 ID 引用——与 firewall 消费面板监听配置同口径，不构成新增关联领域对象（关联领域对象仍 db_instance 一项）；panel-core 配置变更通知钩子为进程内事件消费（P5 载荷，非持久引用）；HostGuard/EntryPathGuard 内部端点豁免为 panel-core 机制侧变更依赖（跨模块待同步项，非领域引用）。**

## 第六部分：跨业务域集成

| 模式 | 集成点 | 说明 |
|------|--------|------|
| OHS（开放主机服务） | 审计写入接口约定（FR-17） | 本域向全部业务域暴露标准写入服务，不暴露内部模型 |
| PL（发布语言） | 审计事件载荷 | 事件 Schema（主体/动作/对象/变更内容/结果）为跨域契约（R-03 四要素） |
| 遵奉者 | 会话鉴权协议 | 全部 HTTP 业务域遵奉 Cookie+CSRF 协议（R-02）；终端等 WS 域遵奉会话握手协议（R-18） |
| 共享内核 | role/审计结果枚举 | admin/readonly 与审计结果枚举被多域共享，D3 时登记枚举池（workspace/01-solution） |
| ACL（防腐层） | 不适用 | 无外部系统集成（R-09 零外呼零回传） |
| 遵奉者（消费，panel-core） | 会话鉴权与 RBAC 协议 | panel-core 全部接口遵奉 Cookie+CSRF（R-02）；菜单可见性遵奉 admin/readonly 角色判定（ADR-018） |
| OHS（消费，panel-core） | 审计写入接口 | 配置/证书变更写审计（R-03，经 account-auth OHS，同事务）；会话参数透传展示读取 internal/config 会话键（account-auth D-10 载体=配置文件，非 OHS 调用；ADR-019，只读） |
| 共享内核（消费，panel-core） | panel_role 枚举 | 菜单可见性按 admin/readonly 过滤（枚举池已定稿） |
| OHS（消费，panel-core） | ACME 签发通道与 nginx 安全写回（site-mgmt 提供） | 面板自身证书 ACME HTTP-01 签发/续期复用 site-mgmt AcmeGateway（lego ACL）与 WritePipeline/NginxGateway（挑战 location 临时注入/撤除走 R-06 四步；ADR-029，v1.16 增量；被依赖登记见 api-site-mgmt.md 1.5/5.1）；R-09 例外语义与 site-mgmt ACME 申请同判（用户主动申请/续期调度），面板经跨模块通道外呼、不自建 ACL |
| ACL（防腐层，panel-core） | 不适用（经 site-mgmt ACL 通道复用，不自建） | 面板证书 ACME 对外部 CA 的协议交互经 site-mgmt AcmeGateway ACL 复用（ADR-029，见上 OHS 行），本域不直连 CA；更新检查显式开启后的版本源拉取若 D3 落地需以 ACL 包装（prd-panel-core.md 待对齐-1） |
| OHS（开放主机服务，monitoring 提供） | 监控数据读取契约 | 实时快照/趋势序列/TOP 进程/系统信息的标准读取语义；消费方=panel-core 仪表盘页（D0）；REST 拉取与 WS 推送取舍 D3 定（prd-monitoring.md 待对齐-1） |
| 遵奉者（monitoring） | 会话鉴权协议 | 监控接口全部位于登录会话后（R-02）；readonly 全可看（ADR-018）；本模块无变更接口，无只读越权拒绝面 |
| 共享内核（monitoring） | metric_window / metric_dimension 枚举 | 查询参数枚举 D2 定稿入池（04-business-enum.md），与 panel-core 仪表盘共享口径 |
| ACL（防腐层，monitoring） | 不适用 | 数据源=本机系统运行时读取（gopsutil 选型，idea.md §5.3），非外部系统对接（R-09 零外呼，本模块无外呼路径） |
| 遵奉者（service-mgmt） | 会话鉴权与只读边界协议 | 全部接口位于登录会话后（R-02）；查看两角色同等、操作 admin 专属（ADR-018），越界经 account-auth S-03-02 边界拒绝并提示原因 |
| OHS（消费，service-mgmt） | 审计写入接口 | 全部服务操作（启动/停止/重启/自启切换）写审计（R-03，经 account-auth OHS，同事务）；操作结果复用 audit_result 枚举 |
| 共享内核（service-mgmt） | audit_result + 服务枚举 | 操作结果枚举复用 account-auth 定稿；service_active_state / service_enabled_state / unit_type 本模块 D2 定稿入池（04-business-enum.md），与 D4 页面口径共享 |
| ACL（防腐层，service-mgmt） | 不适用 | systemd 为本机系统组件：经 dbus 通道（sysd 适配器，R-08）访问，非外部业务系统对接（R-09 零外呼，本模块无外呼路径）；dbus 模型→投影翻译属技术实现（D1 AA 平台支撑域判定），与 monitoring 本机运行时口径一致 |
| 遵奉者（file-manager） | 会话鉴权与只读边界协议 | 全部接口位于登录会话后（R-02）；查看两角色同等（含回收站列表）、变更与下载 admin 专属（ADR-018 禁变更禁上传下载），越界经 account-auth S-03-02 边界拒绝并提示原因 |
| OHS（消费，file-manager） | 审计写入接口 | 全部变更类操作（上传/编辑保存/整理/权限属主/压缩解压/还原/清空/白名单配置修改）+ 下载（数据取出口径，解释性决策）写审计（R-03，经 account-auth OHS，同事务）；结果复用 audit_result 枚举 |
| 共享内核（file-manager） | audit_result + 文件枚举 | audit_result 复用 account-auth 定稿；archive_format（既有 draft 定稿）/ trash_entry_status / file_type 本模块 D2 定稿入池（04-business-enum.md），与 D4 页面口径共享 |
| ACL（防腐层，file-manager） | 不适用 | 本机文件系统经 files 适配器（安全路径层，R-04 技术规约）访问，非外部业务系统对接（R-09 零外呼，本模块无外呼路径）；EvalSymlinks→前缀校验翻译属技术实现（D1 AA 平台支撑域判定），与 monitoring/service-mgmt 本机资源口径一致 |
| 遵奉者（site-mgmt） | 会话鉴权与只读边界协议 | 全部接口位于登录会话后（R-02）；站点列表/详情/证书清单/日志尾部两角色同等（D0 口径）、全部变更 admin 专属（ADR-018），越界经 account-auth S-03-02 边界拒绝并提示原因 |
| OHS（消费，site-mgmt） | 审计写入接口 | 全部变更类操作（编辑/源码保存/建站/启停/重载/证书申请/部署/续期重试/删除）+ 自动续期结果写审计（R-03，经 account-auth OHS，同事务）；结果复用 audit_result 枚举 |
| ACL（防腐层，site-mgmt，DNS 服务商 API——v1.20 增补） | Cloudflare / 阿里云 DNS / 腾讯云 DNSPod（外部系统） | DNS-01 TXT 自动化通道：经 lego DNS provider 封装（服务商 API 模型不渗透面板领域模型，出止于 TXT 添加/验证/清理语义）；R-09 例外语义与 ACME CA 同判（用户主动申请/续期调度）；凭证密文经 dns_credential 聚合承载（⚠️A20，独立密钥文件沿 CredentialVault 先例） |
| 共享内核（site-mgmt） | audit_result + 站点/证书枚举 | audit_result 复用 account-auth 定稿；site_type / site_enabled / cert_source / acme_challenge / cert_status（既有 draft 定稿，⚠️A2 随 ADR-024 关闭）/ site_log_type（新增）本模块 D2 定稿入池（04-business-enum.md），与 D4 页面口径共享 |
| ACL（防腐层，site-mgmt） | ACME CA（Let's Encrypt 等） | **本项目首个真实外部系统 ACL**：ACME 协议交互（order/challenge）经 lego 客户端（idea.md §5.3 选型）封装，证书产物翻译为 certificate 对象——外部 CA 概念不渗透面板领域模型；R-09 除外条款的唯一外呼路径（用户主动申请/既有证书续期调度） |
| 遵奉者（database-mgmt） | 会话鉴权与只读边界协议 | 全部接口位于登录会话后（R-02）；查看两角色同等、变更与导出导入/key 浏览 admin 专属（ADR-018），越界经 account-auth S-03-02 边界拒绝并提示原因 |
| OHS（消费，database-mgmt） | 审计写入接口 | 全部变更类操作（凭据配置/移除纳管/建删库/建删用户/授权/root 密码/Redis 密码/导出/导入）写审计（R-03，经 account-auth OHS，同事务）；凭据值与密文一律脱敏；结果复用 audit_result 枚举 |
| 共享内核（database-mgmt） | audit_result + 本域枚举 | audit_result 复用 account-auth 定稿；db_engine / db_conn_mode / instance_conn_state 本模块 D2 定稿入池（04-business-enum.md），与 D4 页面口径共享；既有 draft db_type 不采用（被 db_engine 取代，理由见池行注明） |
| ACL（防腐层，database-mgmt） | 不适用 | MySQL/Redis 为本机系统组件：经 dbmysql/dbredis 适配器（idea.md §5.3）建连访问，非外部业务系统对接（R-09 零外呼，本模块无外呼路径）；引擎协议/结果集→面板投影翻译属技术实现（D1 AA 平台支撑域判定），与 monitoring（gopsutil）/service-mgmt（dbus）/file-manager（files 适配器）本机资源口径一致 |
| 遵奉者（terminal） | 会话鉴权协议（WS 握手） | 终端握手遵奉面板会话与角色判定协议（R-18，account-auth S-03-03）；readonly 整域拒绝并提示原因（ADR-018）；鉴权载体同 R-02 同源 Cookie 会话 |
| 遵奉者（消费事件，terminal） | 会话失效联动事件 | 面板会话过期/登出/强制下线（account-auth C-03-01/02/03）触发该操作者全部终端连接联动断开（R-18 反向联动，M5 验收硬项；事件通知机制形态 D3 定） |
| OHS（消费，terminal） | 审计写入接口 | 会话建立/断开作为安全事件写入（R-03 精神，D1 解释性决策；拒绝类准入行为不记）；结果复用 audit_result 枚举 |
| 共享内核（terminal） | audit_result + 本域枚举 | audit_result 复用 account-auth 定稿；terminal_end_reason / terminal_identity_source 本模块 D2 定稿入池（04-business-enum.md），与 D3/D4 事件载荷及新建会话口径共享 |
| ACL（防腐层，terminal） | 不适用 | PTY/系统用户为本机系统组件：creack/pty 与系统用户实况读取（idea.md §5.3）均非外部业务系统对接（R-09 零外呼，本模块无外呼路径）；PTY 模型→终端流翻译属技术实现（D1 AA 平台支撑域判定），与 monitoring（gopsutil）/service-mgmt（dbus）本机资源口径一致 |
| 遵奉者（firewall） | 会话鉴权与只读边界协议 | 全部接口位于登录会话后（R-02）；查看两角色同等（含无后端引导视角）、规则增删 admin 专属（ADR-018），越界经 account-auth S-03-02 边界拒绝并提示原因 |
| OHS（消费，firewall） | 审计写入接口 | 全部变更类操作（规则添加/删除）写审计（R-03，经 account-auth OHS，同事务）；结果复用 audit_result 枚举；规则参数即业务内容、无凭据类脱敏面 |
| OHS（消费，firewall） | 面板监听配置读取（panel-core） | 面板端口标注与涉险判定基准：panel_config 单例值（监听端口/协议）只读消费，不持有其聚合根 ID；读取通道（进程内直读 vs 内部接口）D3 定（prd-firewall.md 待对齐-6） |
| 共享内核（firewall） | audit_result + 本域枚举 | audit_result 复用 account-auth 定稿；firewall_backend（既有 draft 定稿）/ firewall_rule_action / firewall_rule_protocol / listen_port_state 本模块 D2 定稿入池（04-business-enum.md），与 D3/D4 接口与页面口径共享 |
| ACL（防腐层，firewall） | 不适用 | ufw/firewalld 为本机系统组件：经 firewallx 适配器（idea.md §5.1/§5.3）命令通道访问，非外部业务系统对接（R-09 零外呼，本模块无外呼路径）；后端规则模型→面板投影翻译属技术实现（D1 AA 平台支撑域判定），与 monitoring（gopsutil）/service-mgmt（dbus）/database-mgmt（dbmysql/dbredis）本机资源口径一致 |
| 遵奉者（docker-mgmt） | 会话鉴权与只读边界协议 | 全部接口位于登录会话后（R-02）；查看两角色同等（含未装引导视角、五类清单、容器与 Compose 日志、资源占用）、变更 admin 专属（ADR-018），越界经 account-auth S-03-02 边界拒绝并提示原因 |
| OHS（消费，docker-mgmt） | 审计写入接口 | 全部变更类操作（容器启停/重启/暂停恢复/删除、镜像拉取/删除、卷删除、网络删除、Compose up/down/restart）写审计（R-03，经 account-auth OHS，同事务）；结果复用 audit_result 枚举；无凭据类敏感值（对象参数即业务内容） |
| 共享内核（docker-mgmt） | audit_result + 本域枚举 | audit_result 复用 account-auth 定稿；container_status / compose_action（既有 draft 定稿，ADR-025/R-13 依据）/ docker_daemon_state（新增）本模块 D2 定稿入池（04-business-enum.md），与 D3/D4 接口与页面口径共享 |
| ACL（防腐层，docker-mgmt） | 不适用 | Docker daemon 为本机系统组件：经 dockerx 适配器（官方 Go SDK + `docker compose` CLI，idea.md §5.1/§5.3）访问，非外部业务系统对接；镜像拉取外呼发生在 daemon 侧且为用户主动发起（面板程序自身零外呼不变，R-09 口径不破，与 ACME 申请同例外语义、面板不直接对接 registry）；daemon 模型→面板投影翻译属技术实现（D1 AA 平台支撑域判定），与 monitoring（gopsutil）/service-mgmt（dbus）/database-mgmt（dbmysql/dbredis）/firewall（firewallx）本机资源口径一致 |
| 遵奉者（cron-backup） | 会话鉴权与只读边界协议 | 全部接口位于登录会话后（R-02）；查看两角色同等（含系统预置位视角）、变更/触发/下载 admin 专属（ADR-018），越界经 account-auth S-03-02 边界拒绝并提示原因 |
| OHS（消费，cron-backup） | 审计写入接口 | 全部变更类操作（计划任务增删改、备份任务增删改）与手动触发（计划任务手动触发、备份立即执行）+ 产物下载（数据取出口径，对齐 file-manager 下载先例）写审计（R-03，经 account-auth OHS，同事务）；结果复用 audit_result 枚举；手动触发命令原文与凭据类值脱敏口径 D3 定 |
| OHS（消费，cron-backup） | 数据库实例清单与凭据建连（database-mgmt） | 备份源 database 项：清单联动选取（实例/库实况）+ 执行时经其契约用已存凭据建连取数（凭据值不进入本域，⚠️A18 精神）；db_instance 为全项目首个关联领域对象引用（第五部分表行）；契约通道形态（进程内契约 vs 内部接口）D3 定（prd-cron-backup.md 待对齐-6）；实例移除纳管后引用悬空→执行失败如实记录、不级联删任务 |
| OHS（消费，cron-backup） | 站点清单（site-mgmt） | 备份源站点目录项的联动选取来源（S-28 站点实况只读）；保存后为路径值快照不持续引用（site 无聚合根 ID，区别于 database 项） |
| 共享内核（cron-backup） | audit_result + 本域枚举 | audit_result 复用 account-auth 定稿；backup_target_type / backup_run_status（既有 draft 定稿，R-11/ADR-016 依据）/ backup_trigger / backup_source_type / cron_source（新增）本模块 D2 定稿入池（04-business-enum.md），与 D3/D4 接口与页面口径共享 |
| ACL（防腐层，cron-backup） | rsync 远程目标（用户基础设施） | 备份产物经 backupx 适配器（R-08 参数数组）推送到用户配置的远程地址——外部系统经 ACL（rsync 协议与远端路径模型不渗透面板领域模型，传输载荷=产物文件流）；**用户主动配置的数据传输**（材料 §4.1 M8 明示目标形态），与 ACME 申请同「用户主动」例外语义、R-09 零遥测口径不破（面板无任何 phone-home）；本地目录目标为本机文件系统写入（与 file-manager files 适配器同口径，非外部系统）；cron 本机组件（crontab 读写/cron 日志）经 cronx 适配器同 monitoring/service-mgmt 本机资源口径；rsync 认证凭据承载 D3 定（prd-cron-backup.md 待对齐-6） |
| 遵奉者（env-mgmt，v1.19 追加） | 会话鉴权与只读边界协议 | 全部接口位于登录会话后（R-02）；环境列表/任务状态输出两角色同等、全部变更 admin 专属（ADR-018），越界经 account-auth S-03-02 边界拒绝并提示原因 |
| OHS（消费，env-mgmt） | 审计写入接口 | 全部变更类操作（安装/升级/卸载发起与终态、仓库源写入、服务启停自启）写审计（R-03，经 account-auth OHS，同事务）；任务终态结果一并留痕；结果复用 audit_result 枚举 |
| 共享适配层（env-mgmt，v1.19 追加，非业务域集成） | SysdGateway（service-mgmt 适配器资产） | 环境服务启停自启与实况服务状态查询复用同一 systemd 网关实现（进程内适配器共享，非模块间调用）；本模块服务操作限定目录环境单元（⚠️A7 映射），service-mgmt 保持全部单元治理 |
| 共享内核（env-mgmt） | audit_result + 本域枚举 | audit_result 复用 account-auth 定稿；env_type / env_task_action / env_task_status / package_manager_family（均新增）本模块 D2 定稿入池（04-business-enum.md），与 D3/D4 接口与页面口径共享 |
| ACL（防腐层，env-mgmt） | 不适用 | apt/dnf·yum/systemd 为本机系统组件：经 envpack 适配器（R-08 参数数组）访问，非外部业务系统对接（R-09 零外呼，本模块无外呼路径——包索引刷新/安装均经发行版本地包管理器通道，不直接触网）；仓库源代配为本机文件写入（与 site-mgmt 配置写回同口径）；与 monitoring/service-mgmt 本机资源口径一致 |
| 遵奉者（db-tools，v1.24 追加） | 会话鉴权与只读边界协议 | 全部接口位于登录会话后（R-02）；工具列表/任务状态输出两角色同等、全部变更 admin 专属（ADR-018）；**工具访问整体 admin 专属（⚠️A10）——工具路径经 gating 逐请求校验会话+admin 角色（FR-07），readonly/无会话不可达工具本体** |
| OHS（消费，db-tools） | 审计写入接口 | 全部变更类操作（安装/升级/卸载发起与终态、凭据轮换）+ 打开（访问开启）写审计（R-03，经 account-auth OHS，同事务脱敏——detail 不含凭据/口令/令牌明文）；任务终态结果一并留痕；gating 逐请求校验不落业务审计（⚠️A11 防写放大）；结果复用 audit_result 枚举 |
| OHS（消费，db-tools） | account-auth 只读会话校验（VerifyWs，gating 消费） | 工具路径 auth_request 子请求校验会话 cookie+admin 角色（200/401，仅 loopback 内部子请求通道，纯读防 CSRF/越权）——**db-tools 自持 GET /dbtool/gate/verify 内部端点，复用 account-auth 既有只读会话校验 OHS（VerifyWs 语义 session.go:59，只读不续期），account-auth 零新增 HTTP 接口**（api D-11 定稿，OHS 消费已登记 api-account-auth.md v1.2；v1.25 W2 回填，prd-db-tools.md 待对齐-2 已关闭） |
| OHS（消费，db-tools） | env-mgmt 环境实况读契约 | 依赖就绪度数据源（PHP-FPM/Node/nginx 已装态与版本，进程内读单向）；Node 环境为 env-mgmt v1.24 增量供给（prd-env-mgmt.md FR-08）；依赖缺失软引导回 env-mgmt 安装页（无跨模块 API 硬依赖） |
| OHS（消费，db-tools） | site-mgmt 站点创建/移除 + 安全写入治理 | phpMyAdmin 经 php_fpm 站点（S-27-02）、mongo-express 经 reverse_proxy 站点（S-27-03）落工具路径；gating/basic auth 注入配置经安全写入治理（S-30，R-06 四步管线）；卸载经站点移除通道（S-28）；工具路径形态 D1 已裁定（既有块路径前缀/0 匹配独立自动块，prd-db-tools.md 待对齐-3 关闭）；**v1.26 +自动落地块创建（ConfigApply allowNew）/域名联动刷新/清块（ConfigRemove）经既有管线（configpipeline.go:18/:29，nginx -t+reload+失败字节复原 R-06），零新增契约，块模板细节 D3（待对齐-7）** |
| OHS（消费，db-tools，v1.26 新增） | panel-core 配置变更通知钩子 | bindDomain 绑定/解绑成功（config/update，D-P12）→ 进程内通知 db-tools → 自动块存在则重渲染 server_name 经 ConfigApply 刷新（域名联动主通道，D1 裁定；钩子失败不阻断配置变更，记日志+install/open 惰性校正兜底）；钩子契约签名归 D3（main 装配注册，沿 BootstrapAfterInstall OHS 先例）；**panel-core 侧钩子提供为跨模块待同步项（S-11-01 配置权威侧，本轮不代改）** |
| 机制侧变更依赖（db-tools→panel-core，v1.26 新增） | HostGuard/EntryPathGuard 内部端点豁免 | 全局包装器（cmd/dougong/main.go:487-488 Entry→Host→gin 链，作用于全部路由）对 /dbtool/gate/、/dbtool/signon/ 两路径前缀直通放行（⚠️A16/FR-13）——修复绑域名后 auth_request 子请求/SignonScript 回调 Host≠域名被误拦 404 致 gating/signon 失效；豁免范围最小化，非内部路径强校验不变（IP 直连面板仍 404，fail-closed 不变）；内部端点 loopback+秘密头+会话/令牌防护链不变（api/dbtools/module.go:64/:208）；**panel-core S-07-02/S-07-03 机制侧变更，跨模块待同步项，本轮不代改** |
| 共享适配层（db-tools，非业务域集成） | SysdGateway（service-mgmt 适配器资产） | mongo-express 服务单元创建/启停/重启/移除与运行态查询复用同一 systemd 网关实现（进程内适配器共享，非模块间调用）；全部系统服务清单治理仍归 service-mgmt |
| OHS（消费，db-tools，新增） | database-mgmt 实例凭据 OHS | signon 桥接与 mongo-express 注入消费 vault 凭据取出能力（进程内单向硬依赖，与 env-mgmt 消费 BootstrapAfterInstall 同构）；凭据解密仅服务端，经 loopback/systemd env 通道流转不出服务端边界；db_instance 为关联领域对象（第五部分表行）；契约签名归 D3（api-database-mgmt.md §5.1 登记） |
| 共享内核（db-tools） | audit_result + 本域枚举 | audit_result 复用 account-auth 定稿；db_tool_type / db_tool_task_action / db_tool_task_status（均新增）本模块 D2 定稿入池（04-business-enum.md），与 D3/D4 接口与页面口径共享；**db_tool_state 不采用**（部署状态=档案存在性+实况投影非状态机，理由见池行） |
| ACL（防腐层，db-tools） | phpMyAdmin / mongo-express（外部现成工具） | 工具协议（signon 回调/basic auth/env 注入/baseUrl 路径前缀）经适配器封装——工具内部模型不渗透面板领域模型（出止于部署/注入/桥接语义）；工具本体为用户主动部署的现成软件（下载 tarball/npm 为 R-09 用户主动发起例外，与 ACME 同判；工具本体遥测经配置关闭）；工具进程仅绑 localhost 经面板 nginx 工具路径 gating 访问（⚠️A6/A7，工具进程无公网直暴露；**v1.26：自动落地块 nginx 接入层 listen 0.0.0.0:18090 为新增公网面，每请求 auth_request gating+已绑域名非匹配 Host 404，非工具进程端口**）；nginx/PHP-FPM/systemd/npm 为本机系统组件（nginxx/phpfpm/sysd 适配器与 npm runner，R-08 参数数组），与 monitoring/service-mgmt 本机资源口径一致 |

**存储映射**：panel_user→panel_user/panel_user_totp；session→panel_session；audit_log→audit_log；初始化状态→init_state；panel_config→panel_config；HTTPS 证书→panel_cert；配置生效分类→无表；trash_entry→trash_entry；file_entry→无表；受管根配置→fs_path_scope；certificate→certificate；site/upstream/Raw 块/写回落点映射→无表；db_instance/redis_instance→db_instance（合一档案表，engine 列区分，D3 定稿 D-01）；database/db_user→无表；data_table/table_row/mongo_collection/mongo_document→无表（v1.23：实例内实况投影零台账）；凭据→实例档案内嵌密文列（AES-256-GCM，无独立表）；terminal_session→无表（内存态不落库）；运行身份/限流策略→无表（内嵌值对象/域级固定策略）；firewall_rule→无表（后端实况投影零规则台账）；container/image/volume/network/compose_project→无表（daemon 实况投影零台账）；backup_job→backup_job、backup_run→backup_run（SQLite 两表，D0 拍板，DDL 归 D3）；备份源清单/备份目标→无表（backup_job 内嵌值对象）；cron_job→无表（crontab 实况投影零任务台账）；db_tool→db_tool、tool_deploy_task→tool_deploy_task（SQLite 两表，v1.24 D2 落定，DDL 归 D3）；工具凭据→db_tool 内嵌密文列（AES-256-GCM，无独立表）；工具条目/目录映射→无表（编译期常量+实况投影零工具台账）；**落地引用（siteRef）→db_tool.site_ref 内嵌 JSON 值（v1.26 +Port/AutoCreated 载荷扩字段，TEXT 列不变零 DDL 零迁移）；自动落地块→无表（nginx 配置文件事实源零块台账）**

> 存储映射说明（panel-core，D3 定稿）：panel_config 为结构化单例表（api-panel-core.md D-P01）；HTTPS 证书为 1:1 独立单行表 panel_cert（D-P04，key_pem 零下发）；配置生效分类为域级规则值对象无表；v1.16 增量（ADR-028/029）：panel_config 增 bind_domain 列、panel_cert.source 扩展 acme 取值（签发状态/失败原因字段形态与 https_mode 取值联动归 D3 DDL 修订，prd 待对齐-6/-7）。
> 存储映射说明（monitoring，D2 定稿）：metric→无表（进程内环形缓冲，ADR-014 拍板不落库）；system_info→无表（实时投影不持久化）——本模块零持久化，无 DDL。
> 存储映射说明（service-mgmt，D2 定稿）：systemd_service→无表（dbus 实时投影，kb 拍板不落库）——本模块零持久化，无 DDL、无数据侧设计。
> 存储映射说明（file-manager，D3 定稿回填）：trash_entry→SQLite 表 trash_entry（ADR-021 拍板，本模块唯一持久化业务对象，DDL 见 api-file-manager.md 5.3）；file_entry→无表（文件系统实时投影，kb 拍板不落库）；受管根配置→fs_path_scope 单行表（api-file-manager.md D-02 拍板：修改与审计同事务、提交后刷新进程内副本即予生效——区别于 account-auth D-10 配置文件载体，关闭 prd-file-manager.md 待对齐-1）。
> 存储映射说明（site-mgmt，D2 判定 + D3 定稿回填）：certificate→SQLite 表 certificate（本模块唯一持久化对象；DDL 见 api-site-mgmt.md 5.3：deploy_prev_refs JSON 列承载部署前引用快照、challenge/storage_path/origin_path 按 I-01 组装约束、acme_account_ref 仅存标识密钥在文件）；site→无表（配置文件实时投影，零站点台账 D2 判定——配置文件是唯一事实源，面板外修改自动反映）；upstream/Raw 块/写回落点映射→无表（site 内嵌值对象/派生映射）；conf.backup 快照目录为机制存储载体非领域对象（保留 20 份，api-site-mgmt.md D-04 定稿，关闭 prd 待对齐-10）；证书状态读时惰性推算（api-site-mgmt.md D-08，关闭 prd 待对齐-3）。
> 存储映射说明（database-mgmt，D2 判定 + D3 定稿回填）：db_instance/redis_instance→SQLite **合一档案表 db_instance**（api-database-mgmt.md 5.3 DDL：D-01 engine 列区分双聚合根、identity_key 唯一消重、凭据密文内嵌列——两聚合根判定不变，聚合边界在领域层不由表数决定）；凭据密文=AES-256-GCM、密钥=面板数据目录 db-cred.key 文件（0600，D-02，关闭 prd 待对齐-1，acme-account.key 同先例）；database/db_user→无表（实例内实况实时投影，零库用户台账——与 site 零站点台账同哲学）；凭据→实例档案内嵌密文列（值对象无独立表）。**v1.23 增量：data_table/table_row/mongo_collection/mongo_document→无表**（实例内实况投影零台账，沿零库用户台账哲学；表/行/集合/文档的事实源在引擎内，面板外变更自动反映；唯一持久化对象仍为实例档案——本轮零 DDL 变更、零迁移）。
> 存储映射说明（terminal，D2 定稿）：terminal_session→无表（内存态不落库、断开即无痕——全项目唯一「聚合根+零持久化」组合，聚合边界在领域层不由存储形态决定）；运行身份/限流策略→无表（内嵌值对象/域级固定策略）——本模块零持久化，无 DDL、无数据侧设计（与 monitoring/service-mgmt 同口径）。
> 存储映射说明（firewall，D2 定稿）：firewall_rule→无表（ufw/firewalld 后端实况投影，kb 拍板不落库，零规则台账——与 site 零站点台账/database-mgmt 零库用户台账同哲学）；监听端口对照结论为读取时派生——本模块零持久化，无 DDL、无数据侧设计（与 monitoring/service-mgmt/terminal 同口径）。
> 存储映射说明（docker-mgmt，D2 定稿）：container/image/volume/network/compose_project→无表（Docker daemon 实况投影，kb 拍板不落库，零台账——与 firewall 零规则台账同哲学，五对象同判）；容器状态为 daemon 实况映射呈现、无面板侧状态留存；日志为实况流无留存——本模块零持久化，无 DDL、无数据侧设计（与 monitoring/service-mgmt/terminal/firewall 同口径）。
> 存储映射说明（cron-backup，D2 判定 + D3 定稿回填）：backup_job/backup_run→SQLite 两表（DDL 见 api-cron-backup.md 5.3：backup_job.source_items/target_config 为 JSON 值列内嵌；backup_run.job_id 弱引用无 FK + job_name 冗余快照 + idx_run_running 部分唯一索引承载执行互斥 I-01）；备份源清单/备份目标→无表（backup_job 内嵌 JSON 值列）；cron_job→无表（crontab 实况投影零任务台账，D2 口径-1——备注与来源标注以 crontab 注释行承载，与 site/firewall/database-mgmt 零台账同哲学）；DbGateway.DumpDatabase 消费契约经 api-database-mgmt.md 4.1.5/5.1 预告位由本模块 D3 正式登记兑现。
> 存储映射说明（env-mgmt，D2 判定；v1.22 增量；v1.24 Node 增量）：env_install_task→SQLite 表（DDL 归 D3：env_type/action/status/输出尾部/失败原因/发起者/时间字段，env_type+status 组合索引承载在途互斥查询；任务台账全量保留不建清理任务；**v1.22 +bootstrap_status/bootstrap_note 两列——引导结果随终态定格，ADD COLUMN 迁移**；**v1.24 env_type CHECK 扩 node 八值——表重建迁移归 D3**）；环境条目→无表（包管理器/systemd 实况实时探测投影，零环境台账——目录条目为编译期常量随代码分发；node 条目单元映射空/服务态 null ⚠️A17）；发行版映射→无表（编译期常量内嵌目录条目；+node：nodejs/NodeSource 代配/LTS 轨道 ⚠️A16）。**安装后配置就绪的建账户/设密码/注册实例与凭据归 database-mgmt db_instance 聚合（cred_cipher 密文列），env-mgmt 不持有凭据——引导结果（status/note）依附任务台账，引导密码明文为过程数据（P7 敏感，加密后密文落 db-mgmt，零回显）。**
> 存储映射说明（db-tools，v1.24 D2 判定）：db_tool→SQLite 表 db_tool（DDL 归 D3：工具类型唯一键/已装版本/部署路径/站点引用/服务单元引用/目标实例引用〔mongo-express 项〕/凭据密文列——basic auth 口令与 signon 密钥 AES-256-GCM 整体加密，密钥文件 0600 沿 CredentialVault 先例〔文件名 D3 定〕；运行态/依赖就绪度不持久化〔实况投影〕）；tool_deploy_task→SQLite 表 tool_deploy_task（DDL 归 D3：toolType/action/status/输出尾部/失败原因/发起者/时间字段，toolType+status 组合索引承载在途互斥查询；任务台账全量保留不建清理任务，沿 env-mgmt 先例）；工具凭据→db_tool 内嵌密文列（值对象无独立表）；工具条目/目录映射→无表（编译期常量+实况投影零工具台账）；signon 令牌消费台账→无表（内存态 TTL 清扫，面板重启全部失效=安全保守，P7 敏感过程数据）。**数据库连接凭据（MySQL/Mongo）不落本域任何表——取自 database-mgmt vault 经服务端通道注入（关联领域对象消费，I-02 零回显）。** **v1.26 增量（自动 nginx 落地）：零 DDL 变更零迁移——落地引用 siteRef +Port/AutoCreated 为 db_tool.site_ref JSON 载荷扩字段（TEXT 列不变，旧档案缺省 Port=0/AutoCreated=false 向后兼容）；tool_deploy_task 失败原因字段文本构造扩展（error 含 data.reason，⚠️A17，列语义不变）；自动落地块本体无表（nginx 配置文件 dougong-tools.conf 为事实源，零块台账）；ToolLandingPort=18090/块文件路径编译期常量；域名联动钩子为进程内事件（P5 载荷无持久化）。**

## 更新记录

| 版本 | 日期 | 更新说明 |
|------|------|---------|
| v1.0 | 2026-11-12 | 初始聚合：纳入 account-auth 域（1 业务域 / 4 聚合根 / 5 域内对象与值对象 / 1 子流程契约），权威说明见 prd-account-auth.md |
| v1.1 | 2026-11-12 | 追加 panel-core 域（1 业务域 / 1 聚合根 panel_config / 1 实体 + 1 值对象 / 无子流程 / 无关联领域对象）；account-auth 第五部分「会话策略待跨域对齐」经 panel-core D2 关闭（策略仍归 account-auth，本域仅透传展示）；4 个新增枚举入池（04-business-enum.md），权威说明见 prd-panel-core.md |
| v1.2 | 2026-11-12 | 追加 monitoring 域（1 业务域 / 无聚合根——只读观测流经 A1-A4 判定 / 2 值对象 metric+system_info / 无子流程 / 无关联领域对象；OHS 读取契约供 panel-core 消费 + 共享内核 2 枚举）；零持久化无 DDL；权威说明见 prd-monitoring.md（完备性检查通过：G 5/5、T 适用 3/3、N 4/4） |
| v1.3 | 2026-11-12 | 追加 service-mgmt 域（1 业务域 / 无聚合根——dbus 实时投影流经 A1-A4 判定 / 1 域内对象-实体 systemd_service（有稳定标识且状态可变，区别于 monitoring 值对象）/ 无子流程 / 无关联领域对象；遵奉会话鉴权 + 消费审计 OHS + 共享内核 audit_result 与 3 个服务枚举）；零持久化无 DDL；权威说明见 prd-service-mgmt.md（完备性检查通过：G 5/5、T 适用 4/4、N 4/4） |
| v1.4 | 2026-11-12 | Gate 1 W1 修正（2026-11-12 复检）：systemd_service 核心业务字段列补「加载状态（loadState，透传属性）」——BA 2.2.1.3 / FL / PRD FR-05 / API 4.1.1 / PageSpec Drawer 五方均含而本表遗漏，按权威盘点惯例补录；不改分类/枚举/存储映射，不影响下游契约 |
| v1.5 | 2026-11-12 | 追加 file-manager 域（1 业务域 / 1 聚合根 trash_entry——A1/A3/A4 通过，本模块唯一持久化对象 ADR-021 / 2 域内对象：file_entry 实体（绝对路径标识，与 systemd_service 同构）+ 受管根配置配置实体（域级单例，⚠️A12）/ 无子流程 / 无关联领域对象；遵奉会话鉴权 + 消费审计 OHS + 共享内核 audit_result 与 3 枚举 archive_format/trash_entry_status/file_type）；持久化=trash_entry 单表（DDL 归 D3）；权威说明见 prd-file-manager.md（完备性检查通过：G 5/5、T 适用项全过、N 4/4） |
| v1.6 | 2026-11-12 | D3 定稿回填（file-manager 本模块行）：受管根配置存储载体落定 fs_path_scope 单行表（api-file-manager.md D-02，同事务审计+写后刷新即予生效，关闭 prd 待对齐-1）；存储映射主行与说明行同步（trash_entry DDL 已见于 api-file-manager.md 5.3）；不改对象分类/枚举/业务字段 |
| v1.7 | 2026-11-12 | 追加 site-mgmt 域（**核心域，本项目首个**——ADR-005 核心卖点直接承载 / 1 聚合根 certificate——A1-A4 全过、落 SQLite 唯一持久化对象，续期任务状态无法从配置文件推导 / 4 域内值对象：site 实体（配置文件实时投影零站点台账 D2 判定，与 file-manager「不导入不镜像」同哲学）+ upstream/Raw 块/写回落点映射值对象（D0 upstream 归属事项关闭）/ 1 子流程自动续期任务（ADR-024 六状态机，与审计清理任务同构）/ 无关联领域对象；集成=本项目首个真实外部 ACL（ACME CA 经 lego，R-09 唯一外呼例外）+ 遵奉会话鉴权 + 消费审计 OHS + 共享内核 6 枚举）；持久化=certificate 单表（DDL 归 D3 待对齐-3）；权威说明见 prd-site-mgmt.md（完备性检查通过：G 5/5、T 适用 7/7、N 4/4）；表头版本号同步至 v1.7（v1.6 回填时表头未及更新的偏差一并修正） |
| v1.8 | 2026-11-12 | D3 定稿回填（site-mgmt 本模块行）：certificate 存储映射说明更新——DDL 落定（api-site-mgmt.md 5.3，deploy_prev_refs JSON 列/challenge 组装约束/acme_account_ref 仅存标识）；conf.backup 保留 20 份（D-04，关闭 prd 待对齐-10）；证书状态读时惰性推算（D-08，关闭 prd 待对齐-3）；不改对象分类/枚举/业务字段，主存储映射行 certificate→certificate 不变 |
| v1.9 | 2026-11-12 | 追加 database-mgmt 域（1 业务域 / 2 聚合根 db_instance+redis_instance——A1 一致性边界=档案↔凭据密文（必须面板自持，与 certificate 同构论证）、独立聚合互不共享边界；A3 纳管/移除唯一生命周期入口 / 2 域内对象-实体 database+db_user（实例实况实时投影，零库用户台账 D2 判定，与 site/file_entry 同构）/ 1 内嵌值对象凭据（⚠️A18 密文替换式更新）/ 无子流程 / 无关联领域对象；集成=遵奉会话鉴权 + 消费审计 OHS + 共享内核 audit_result 与 3 新增枚举 db_engine/db_conn_mode/instance_conn_state（既有 draft db_type 不采用被 db_engine 取代）+ ACL 不适用（dbmysql/dbredis 本机适配器））；持久化=db_instance/redis_instance 实例档案（表形态 D3 定）；权威说明见 prd-database-mgmt.md（完备性检查通过：G 5/5、T 适用 4/4、N 4/4） |
| v1.10 | 2026-11-12 | D3 定稿回填（database-mgmt 本模块行）：实例档案存储载体落定**合一表 db_instance**（api-database-mgmt.md D-01：engine 列区分、identity_key 唯一消重，DDL 见其 5.3；两聚合根判定不变）；凭据密文方案落定 AES-256-GCM + db-cred.key 密钥文件 0600（D-02，关闭 prd 待对齐-1）；不改对象分类/枚举/业务字段 |
| v1.11 | 2026-11-14 | 追加 terminal 域（1 业务域 / 1 聚合根 terminal_session——A1/A3/A4 通过（一致性边界=会话记录↔PTY 进程↔并发配额同生共死），**内存态不落库为全项目唯一「聚合根+零持久化」组合**（D0 拍板）/ 2 内嵌值对象：运行身份（R-18 两分支载体）+ 限流策略（ADR-023 固定标准档）/ 无子流程（空闲/硬上限判定为聚合内定时机制）/ 无关联领域对象（system_user 为系统实况只读引用，鉴权会话引用为运行时凭据非聚合 ID）；集成=遵奉会话握手（R-18）+ 遵奉会话失效联动事件 + 消费审计 OHS + 共享内核 audit_result 与 2 新增枚举 terminal_end_reason/terminal_identity_source + ACL 不适用（creack/pty 本机组件））；零持久化无 DDL；权威说明见 prd-terminal.md（完备性检查通过：G 5/5、T 适用 4/4、N 4/4） |
| v1.12 | 2026-11-14 | 追加 firewall 域（1 业务域 / 无聚合根——后端实况投影流经 A1-A4 判定，与 monitoring/service-mgmt 同构 / 1 域内对象-实体 firewall_rule（后端侧规则定位标识、删除操作可寻址，零规则台账不落库）/ 无子流程 / 无关联领域对象（panel-core 监听配置为实况值消费非 ID 引用）；集成=遵奉会话鉴权 + 消费审计 OHS + 消费面板监听配置 OHS + 共享内核 audit_result 与 4 枚举（firewall_backend 既有 draft 定稿 + firewall_rule_action/firewall_rule_protocol/listen_port_state 新增）+ ACL 不适用（firewallx 本机适配器））；零持久化无 DDL；权威说明见 prd-firewall.md（完备性检查通过：G 5/5、T 适用 4/4、N 4/4） |
| v1.13 | 2026-11-14 | 追加 docker-mgmt 域（1 业务域 / 无聚合根——daemon 实况投影流经 A1-A4 判定，与 monitoring/service-mgmt/firewall 同构（AA 层 C-12 核心业务域为战术判定不改战略分层，与 service-mgmt C-04/C-05 先例一致）/ 5 域内对象-实体 container+image+volume+network+compose_project（daemon/label 实况投影零台账不落库；container 状态映射与动作矩阵唯一口径=BA 附录B/ADR-025；compose_project 为 label 聚合派生投影、容器清单为同域引用集合）/ 无子流程 / 无关联领域对象（terminal 容器内终端为二期被依赖预期不预先建模）；集成=遵奉会话鉴权 + 消费审计 OHS + 共享内核 audit_result 与 3 枚举（container_status/compose_action 既有 draft 定稿 + docker_daemon_state 新增）+ ACL 不适用（dockerx 本机适配器，拉取外呼在 daemon 侧 R-09 口径不破））；零持久化无 DDL；权威说明见 prd-docker-mgmt.md（完备性检查通过：G 5/5、T 适用 6/6、N 4/4） |
| v1.14 | 2026-11-14 | 追加 cron-backup 域（1 业务域 / 2 聚合根 backup_run〔A1-A4 全过：记录↔产物↔轮转水位一致性；ADR-016 状态机；落 SQLite〕+ backup_job〔A1/A2/A3 通过：定义单据整体一致性、无状态机；落 SQLite；与 run 为两个独立聚合，run 按 job 归组同域引用〕/ 1 域内对象-实体 cron_job（crontab 投影零任务台账，备注与来源标注注释行承载 D2 口径-1）/ 2 内嵌值对象 备份源清单+备份目标 / 无子流程（执行状态机为 run 聚合内、调度与轮转为机制链路）/ **全项目首个关联领域对象 db_instance 引用**〔源清单 database 项 ID 引用+执行建连；站点目录项为路径值快照不构成引用；database-mgmt「被依赖预期」就此兑现〕；集成=遵奉会话鉴权 + 消费审计 OHS + 消费 database-mgmt 实例清单与凭据建连 OHS + 消费 site-mgmt 站点清单 OHS + 共享内核 audit_result 与 5 枚举（backup_target_type/backup_run_status 既有 draft 定稿 + backup_trigger/backup_source_type/cron_source 新增）+ ACL rsync 远程目标〔用户主动，R-09 例外语义，与 ACME 同判〕）；持久化=backup_job/backup_run 两表（DDL 归 D3）；业务域清单占位行关闭（11 域全部完成 D2 累积）；权威说明见 prd-cron-backup.md（完备性检查通过：G 5/5、T 适用 6/6、N 4/4） |
| v1.15 | 2026-11-14 | D3 定稿回填（cron-backup 本模块行）：存储载体落定——backup_job/backup_run 两表 DDL 见 api-cron-backup.md 5.3（source_items/target_config JSON 值列内嵌、job_id 弱引用+job_name 快照、idx_run_running 部分唯一索引承载 I-01 执行互斥）；DbGateway.DumpDatabase 消费契约正式登记（database-mgmt 5.1 预告兑现）；关闭 prd-cron-backup.md 待对齐-1~-9（api D-01~D-14）；不改对象分类/枚举/业务字段 |
| v1.16 | 2026-11-16 | panel-core 增量 D2（ADR-028/029，权威说明 prd-panel-core.md v1.1）：panel_config 聚合根核心业务字段 +绑定域名（bind_domain，Host 强校验门槛）；HTTPS 证书实体 source 扩展 acme 取值 +签发状态/上次失败原因字段（状态沿用 ADR-024 六状态语义按需简化引用，单例对象非多实例状态机，对象分类不变）并订正其备注滞后的「物理存储 D3 复核」为已定稿 D-P04；新增子流程「面板证书自动续期任务」（第四部分表行，与 site-mgmt 自动续期任务同构）；业务域清单 panel-core 行职责/关联上下文同步；第六部分新增「OHS（消费，panel-core）ACME 签发通道与 nginx 安全写回」行并将 ACL 行订正为「经 site-mgmt ACL 通道复用」；第五/三部分 panel-core 注脚补 ACME 过程数据与无关联领域对象口径；存储映射说明补增量注 |
| v1.17 | 2026-11-16 | panel-core CLI 增量 D2（ADR-030，权威说明 prd-panel-core.md v1.3）：**零新领域对象/零分类变化**——panel-core 本域判定表后增一行注记：CLI 管理通道（`dougong` 六命令族）为服务器终端本地通道，读写复用既有聚合 panel_config 与 account-auth panel_user（经进程内 service，ADR-017 同族），命令输入输出为过程数据不入对象清单；业务域清单/领域对象清单/子流程/关联领域对象/存储映射均零改动（对象分类仍为 1 聚合根 + 1 实体 + 1 值对象 + 1 子流程契约） |
| v1.18 | 2026-11-16 | site-mgmt 增量 D2（v0.1.2 用户反馈三项，权威说明 prd-site-mgmt.md v1.1）：**零新领域对象/零新枚举**——site 行备注补列表聚合注记（同源文件同 server_name 全集多块合并呈现为投影组装规则，⚠️A18，仅展示层、siteId 寻址不变）；第四部分自动续期任务触发行扩展手动立即续期入口（FR-28：valid/expiring_soon/renew_failed 发起复用同一通道，失败迁移分流 valid 保持 valid+last_error）；imported 文件替换（FR-28）为聚合根字段更新动作（origin_path 文件写前备份原子替换+元数据读时刷新 D-08 通道），无状态机扩展；其余各部分零改动 |
| v1.19 | 2026-11-16 | 追加 env-mgmt 域（v0.1.2 用户反馈第 4 项，权威说明 prd-env-mgmt.md v1.0）：1 业务域 / 1 聚合根 env_install_task（A1-A4 全过：任务记录↔执行进程↔互斥约束一致性，执行事实无法从包管理器状态推导必须面板自持；四态状态机 ⚠️A3；落 SQLite）/ 1 域内对象-实体 环境条目（目录常量+实况探测合并投影，零环境台账——与 site/systemd_service 同哲学）/ 1 值对象 发行版映射（编译期 ⚠️A7）/ 无子流程（任务状态机为聚合根本体，与 backup_run 同构）/ 无关联领域对象（软引导被消费方向）；集成=遵奉会话鉴权 + 消费审计 OHS + SysdGateway 共享适配层（service-mgmt，非业务域集成）+ 共享内核 audit_result 与 4 新增枚举（env_type/env_task_action/env_task_status/package_manager_family）+ ACL 不适用（apt/dnf·yum/systemd 本机组件）；业务域清单 12 域闭环更新；存储映射=env_install_task 单表（DDL 归 D3）；doccenter 已同步 |
| v1.20 | 2026-11-16 | site-mgmt DNS-01 增量 D2（Gate 1 改判 P2，⚠️A20，权威说明 prd-site-mgmt.md v1.2）：**新增聚合根 dns_credential**（A1/A2/A3 通过：凭据密文无法从外部推导必须面板自持+certificate 引用+CRUD 唯一入口；无状态机；落 SQLite，密文 AES-256-GCM+dns-cred.key 0600 沿 database-mgmt CredentialVault 先例）+ 值对象 DNS 凭据材料（按 provider 三形态，密文单列）；certificate 与 dns_credential 为同域两个独立聚合（证书续期消费凭证为同域聚合引用）；业务域清单 site-mgmt 行聚合根/职责/关联上下文同步（+DNS 服务商 API ACL 行）；子流程自动续期任务输入补 dns_01 凭证引用（凭证缺失=失败如实）；枚举 acme_challenge +dns_01、dns_provider 新增（池同步）；第三部分清单 +2 行；第六部分 +ACL 行；其余零改动 |
| v1.21 | 2026-11-17 | database-mgmt MongoDB 增量 D2（v0.1.4 用户反馈第 3 项，拍板 Q1=A/Q2=A，权威说明 prd-database-mgmt.md v1.1 4.8）：**零新领域对象/零聚合根增删**——db_instance 聚合根 db_engine 枚举扩值 +mongodb（合一档案表 engine 列扩值，D3 迁移表重建；redis 归 redis_instance 判定不变）；database/db_user 实体备注扩 mongodb 投影口径（库=名称/大小/集合数只读、隐式建库语义无建删；用户=admin 库 users 投影、无 host 维度、角色概况 mongo_role 三档）；业务域清单 database-mgmt 行职责同步；新枚举 mongo_role{read,readWrite,dbAdmin} 入池（D2 定稿）；集成面无变化（mongo-driver 为进程内库，ACL 不适用行维持）；doccenter 待 D3 后再生成同步 |
| v1.22 | 2026-11-17 | env-mgmt 安装后配置就绪增量 D2（d0-clarification-2026-11-17-bootstrap.md，拍板 Q1~Q4，权威说明 prd-env-mgmt.md v1.2）：**零新聚合根/零新领域对象**——env_install_task 聚合根核心业务字段 +引导结果（bootstrap_status，终态定格，NULL=无引导步骤）+引导摘要（bootstrap_note，不含密码）；安装后配置就绪为聚合根后置机制（数据库类安装成功后 worker 经 db-mgmt OHS 引导，结果随终态同事务定格；引导结果为独立维度非新状态机——任务 status 仍包操作四态，引导失败不改 succeeded ⚠️A14；引导非新聚合根，建账户/注册实例凭据归 db-mgmt db_instance 聚合）；新枚举 bootstrap_status{succeeded,failed,skipped} 入池（D2 定稿）；环境条目 env_type 七值同步（+mongodb，v1.21 已扩 DDL，本表滞后补正）；业务域清单 env-mgmt 行职责 +安装后配置就绪、关联上下文 +消费 db-mgmt BootstrapAfterInstall OHS（单向硬依赖），database-mgmt 行关联上下文 +被 env-mgmt 消费；存储映射 +bootstrap_status/note 两列（ADD COLUMN 迁移）+引导凭据归 db-mgmt 密文列口径；集成面 env-mgmt→db-mgmt 进程内 OHS（与 cron-backup 消费 DumpStream 同构）；doccenter 待 D3 后再生成同步 |
| v1.23 | 2026-11-18 | database-mgmt v1.4 表级/集合文档治理增量 D2（d0-clarification-database-mgmt-ux-2026-11-18.md，拍板 Q1~Q6，权威说明 prd-database-mgmt.md v1.2 4.9~4.11）：**零新聚合根/零 DDL 变更**——凭据与档案仍 db_instance/redis_instance 聚合；+4 域内对象-实体（投影，零台账不落库）：data_table（标识=实例+库+表名，information_schema 投影，列结构为元数据字段不单独建模）/table_row（标识=实例+库+表+单列主键值，行写限单列主键表 Q3，数据内容 admin 专属 Q6）/mongo_collection（标识=实例+库+集合名，createCollection/dropCollection 通道之一，隐式建库经显式建集合达成 ADR-031）/mongo_document（标识=实例+库+集合+_id，InsertOne/ReplaceOne/DeleteOne，数据内容 admin 专属 Q6）——A1-A4 全不通过与 database/db_user 同构判定；database 实体备注修订（mongodb 项只读投影→可管理清单+删库，ADR-031 推翻 D-18；MySQL 项+浏览入口）；业务域清单 database-mgmt 行职责同步（TCP 优先登记 Q1/表级治理/集合文档治理/呈现层 F2·F5）；**零新增枚举**（表存储引擎/排序规则实况开放集合沿字符集先例，审计动作码常量非池枚举）；存储映射 +4 对象→无表；集成面 +消费 panel-core S-09-01 面包屑呈现能力（非领域引用）；doccenter 归档后同步 |
| v1.24 | 2026-11-19 | db-tools 集成初始轮 D2 + env-mgmt Node 环境增量（d0-clarification-db-tools-integration-2026-11-19.md，拍板 Q1~Q3，ADR-032，权威说明 prd-db-tools.md v1.0 / prd-env-mgmt.md v1.3）：**追加 db-tools 域**（1 业务域 / 2 聚合根 db_tool〔部署档案+凭据密文，A1-A4 全过：凭据密文无法从外部推导必须面板自持+面板专属部署通道唯一生命周期入口；落 SQLite；运行态/依赖就绪度实况投影不持久化〕+ tool_deploy_task〔四态机，与 env_install_task 同构 4/4；落 SQLite；与 db_tool 同域双聚合，终态驱动档案登记同事务协作〕/ 1 值对象 工具凭据〔basic auth/signon 密钥密文，I-02 零回显；数据库连接凭据不归本域——取自 db-mgmt vault〕/ 1 域内对象-实体 工具条目〔编译期目录+实况投影零台账〕/ 1 值对象 目录映射〔编译期版本/sha256/分发形态〕/ 无子流程〔signon 桥接为同步机制链路，令牌消费台账 P7 敏感内存态〕/ **关联领域对象 db_instance 一项**〔mongo-express 目标实例持久引用+凭据 OHS 消费，全项目第二处真实关联引用，沿 cron-backup 先例〕；集成=遵奉会话鉴权+消费审计 OHS+消费 account-auth gating 会话校验端点〔新增内部能力待同步项〕+消费 env-mgmt 环境实况读契约+消费 site-mgmt 站点通道+SysdGateway 共享适配层+消费 db-mgmt 实例凭据 OHS+共享内核 audit_result 与 3 新增枚举〔db_tool_type/db_tool_task_action/db_tool_task_status〕+ACL phpMyAdmin/mongo-express 外部工具〔R-09 下载用户主动例外〕）；**db_tool_state 不采用入池**（部署状态=档案存在性+实况投影非状态机，沿 instance_conn_state 辨析先例）；**env-mgmt Node 增量**：环境条目 env_type 八值同步（+node，订正 v1.19 原文「六值」滞后）+node 运行时型语义（服务态 null/启停入口禁用 ⚠️A17）、发行版映射 +node 条目（nodejs/NodeSource 三系代配/LTS 轨道/npm 随包 ⚠️A16）、业务域清单 env-mgmt 行八环境+被 db-tools 消费同步、存储映射 env_type CHECK 扩值迁移归 D3；业务域清单 database-mgmt/account-auth 行关联上下文 +db-tools 消费/被消费登记；业务域清单 13 域闭环；存储映射 +db_tool/tool_deploy_task 两表（DDL 归 D3）+工具凭据密文列+令牌台账无表；doccenter 归档后同步 |
| v1.25 | 2026-11-19 | D3 定稿回填（db-tools 本模块行，Gate 1 W2 修复轮，gate1-db-tools-2026-11-19.md，沿 v1.6/v1.8/v1.10 先例）：gating 端点归属措辞由「account-auth 新增内部能力跨模块待同步」回填为 api D-11 定稿「db-tools 自持 GET /dbtool/gate/verify 内部端点 + 复用 account-auth 既有只读会话校验 OHS VerifyWs（account-auth 零新增 HTTP，OHS 消费已登记 api-account-auth.md v1.2）」——业务域清单 account-auth/db-tools 两行关联上下文、第五部分 db-tools 注脚、第六部分 OHS 消费行四处同步；**零新领域对象/零聚合根增删/零枚举/零存储映射变更**（db_tool/tool_deploy_task 双聚合根与字段不变，gating 为逐请求只读机制非状态机）；prd-db-tools.md 待对齐-2 就此关闭 |
| v1.26 | 2026-11-19 | db-tools 自动 nginx 落地增量 D2（d0-clarification-dbtools-auto-landing-2026-11-19.md 拍板 Q1/Q1b/Q2/Q3，权威说明 prd-db-tools.md v1.2 FR-11~14，D1 BA v1.1/AA v1.2/FL v1.1 ⚠️A14~A17）：**零新聚合根/零新状态机/零 DDL 变更/零枚举**——①落地引用（siteRef）值对象显式建模（第一部分判定 bullet+第三部分清单行：内嵌 db_tool.site_ref JSON 列，File/Index/ServerName/Scheme 既有 +Port/AutoCreated 扩展，JSON 载荷扩字段 TEXT 列不变，旧档案缺省向后兼容；取证 inject.go:20-25/api-db-tools.md:237/:554）；②db_tool 行站点引用注记+tool_deploy_task 行失败原因文本构造扩展（error 含 data.reason ⚠️A17，字段语义不变）；③目录映射行 +ToolLandingPort=18090/块文件路径编译期常量；④业务域清单 db-tools 行职责 +自动落地/域名联动/豁免/reason、关联上下文 +panel-core（钩子消费/豁免依赖/bind_domain 实况值），panel-core 行关联上下文 +db-tools 被消费（钩子提供 S-11-01/豁免 S-07 两跨模块待同步项）；⑤第三/四/五部分 db-tools 注脚 +自动块零台账（配置文件事实源）/机制链路非子流程/bind_domain 实况值消费非 ID 引用（关联领域对象仍 db_instance 一项）；⑥第六部分 +panel-core 配置变更钩子 OHS 消费行+HostGuard 豁免机制侧依赖行+site-mgmt 行扩自动块 ConfigApply/ConfigRemove+ACL 行公网面修订；⑦存储映射 +落地引用内嵌 JSON/自动块无表+说明行 v1.26 零 DDL 注；自动建块/域名联动/清块为机制步骤非状态机（辨析见 prd 第六部分 +3 行）；AutoCreated 布尔/Port 数值非枚举不入池；doccenter 归档后同步 |
