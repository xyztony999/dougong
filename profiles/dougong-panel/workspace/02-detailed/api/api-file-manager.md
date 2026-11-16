# 斗拱面板 文件管理器（file-manager）API 设计

| 属性 | 值 |
|------|-----|
| 版本 | v1.0 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-12 |
| 输入 | prd-file-manager.md v1.0（完备性检查通过：G 5/5、T 适用项全过、N 4/4）、domain-model.md v1.5（file-manager 域：1 聚合根 trash_entry + 2 域内对象 file_entry/受管根配置）、d0-clarification-file-manager.md、D1 三文档（BA/AA/FL v1.0） |
| 接口规约 | 无激活技术栈（ACTIVE_STACKS 为空）——按通用 REST/JSON 契约设计；认证=HttpOnly Cookie 会话（R-02）；鉴权=RBAC 两角色边界矩阵（ADR-018）；持久化=trash_entry 单表 + fs_path_scope 单行表（其余文件数据零持久化，kb 拍板） |

## 1. 模块设计概述

### 1.1 业务背景及目标

文件管理器是斗拱面板 M2 里程碑的入口模块：服务器上的文件本就在那里，面板不导入、不镜像、不重建，直接在受管根范围内提供浏览与操作能力——「接管存量环境」哲学在文件维度的落点。安全是第一约束：全部路径类接口经「EvalSymlinks 解析真实路径 → 受管根前缀校验 → 排除目录校验」两步强制前置（R-04，files 适配器/安全路径层），路径逃逸测试为里程碑验收硬项（D0 §2）；删除不直接销毁而是入站回收站（R-17，ADR-021），trash_entry 为本模块唯一持久化业务对象。服务对象：单机自托管的管理员（admin）与只读用户（readonly）——admin 执行全部变更与数据取出（下载），readonly 同等查看目录内容、导航与回收站列表（ADR-018：禁变更、禁上传下载）。

### 1.2 边界说明

| 项 | 说明 |
|----|------|
| 本模块做 | 路径白名单校验机制与受管根配置（R-04）、目录浏览/导航、上传（单请求+分块）/下载、在线编辑、新建/重命名/移动/删除（入回收站）、权限/属主修改、压缩/解压（含 zip slip 防护）、回收站列表/还原/清空 |
| 本模块不做（接口边界） | ① 磁盘配额、编辑器版本历史/协同、目录打包下载、断点续传、回收站自动清理——D0 边界 OUT/二期；② 路径输入跳转——不做（PRD OUT-6 从紧把握）；③ 系统用户/组的管理（创建/删除系统账号）——本模块只读系统实况供 chown 候选；④ 面板页面外壳与导航——归 panel-core；⑤ 认证/鉴权/审计本体——复用 account-auth 中间件与 AuditService.Record（本模块仅声明依赖）；⑥ 面板外（shell 等）对文件与 `.dougong-trash/` 的直接改动——不在治理范围（列表按实况对账，PRD N2） |
| 与存储的关系 | SQLite 单文件（与 account-auth 同库）；trash_entry 表 + fs_path_scope 单行表（5.3）；file_entry 为文件系统实时投影不落库（kb 拍板） |

### 1.3 设计决策

| # | 决策 | 理由 |
|---|------|------|
| D-01 | 路径寻址与两步校验：全部携带路径的接口先经 PathScopeGuard（EvalSymlinks 解析真实路径 → 受管根前缀 → 排除清单），校验通过后**以真实路径执行**；任何一步不通过返回 FILE_002 并明确提示越界原因；校验机制为全接口强制前置，无白名单旁路 | R-04 强制前置（PRD FR-01）；越界拒绝不记审计（R-03 语义为变更留痕，PRD 第五部分口径）。解压场景延伸为逐条目校验（D-07） |
| D-02 | 受管根配置载体=**fs_path_scope 单行表**（非配置文件）：修改与审计**同事务**落库，事务提交后刷新进程内内存副本，对后续请求即予生效 | 与 account-auth D-10（会话策略=配置文件重启生效）的差异点：本配置有「修改即生效」业务要求（PRD FR-02/D1 BA 2.1.1.2），配置文件需热加载复杂度高；单行表沿 panel_config/init_state 单例先例。**关闭 prd-file-manager.md 待对齐-1** |
| D-03 | 目录浏览与回收站列表**全量返回不分页**：目录按 sort（name\|size\|mtime，默认 name）+order（asc\|desc，默认 asc）服务端排序；回收站按 deleted_at 倒序固定 | 单目录条目数十至数千条量级（PRD N1）、回收站为低频删除积累（PRD 第七部分）；全量返回免分页往返；视图刷新=按需拉取（操作响应内附刷新语义，D4 定呈现节奏，service-mgmt D-03 同口径）。**关闭 prd-file-manager.md 待对齐-6/-7** |
| D-04 | 分块参数拍板：**单请求直传上限=分块大小=5MiB**（超限返回 FILE_011 提示走分块）；分块接口三段式 chunk-init/chunk/chunk-complete；分块必须带 uploadId+index+total，complete 时校验累计大小=totalSize 后按 index 序合并、rename 原子落盘 | D0 建议 5MB 采纳为定值（编译期常量）；断点续传二期（OUT-4），任务中断即作废。**关闭 prd-file-manager.md 待对齐-3** |
| D-05 | 在线编辑**大小上限 1MiB**（全量加载内存安全边界）；可编辑性检测=前 8KB 空字节（\0）检测 + UTF-8 有效性探测，二进制或超限返回 FILE_006 并提示下载后本地处理 | PRD FR-08/待对齐-4；无版本历史（OUT-2），保存为全量覆盖写、最后写者胜（无乐观锁，PRD N2 口径）。**关闭 prd-file-manager.md 待对齐-4** |
| D-06 | 权限修改**不提供递归**：chmod/chown 仅作用于目标条目自身，目录不做子树递归 | 递归改权限为高危不可逆放大操作，D0/D1 IN 未列（PRD 待对齐-5）；从紧把握，如需属二期扩展。**关闭 prd-file-manager.md 待对齐-5** |
| D-07 | 冲突决策=**onConflict 请求参数**（`ask`（默认）\|`overwrite`\|`skip`；还原为 `ask`\|`overwrite`）：ask 时服务端先做**全量冲突预检**，任一冲突即返回 FILE_005（data.conflicts[] 清单）且**零落盘**（预检在先=天然整体回滚）；overwrite/skip 直接执行；「放弃」=客户端不再重发 | 上传/移动/解压冲突显式决策（D1 解释性决策）；解压逐条目越界（zip slip）同为全量预检（FILE_009 整体失败不落盘，PRD FR-15「整体失败回滚」的落法）。**关闭 prd-file-manager.md 待对齐-2** |
| D-08 | 下载审计 category=**security**（audit_action）：文件内容离开系统属数据取出，对齐 account-auth `audit_exported` 归 security 先例（解释性决策，D1 FL 附录B） | 只读角色禁下载（ADR-018）的同一安全语义在审计侧的表达；其余变更类操作 category=change |
| D-09 | **操作与审计非同事务**（文件系统操作）：动作在文件系统侧（非 SQLite 事务域），完成后经 AuditService.Record 独立写入，失败重试一次、仍失败记面板运行日志兜底（service-mgmt D-06 同口径）；**例外**：fs_path_scope 修改与审计同事务（均在 SQLite，D-02）；trash 入站登记与删除审计同事务（均在 SQLite，D-12） | account-auth I-05「同事务」前提是变更落 SQLite；文件系统动作不可回滚，强一致不可达成——如实降级并显式声明 |
| D-10 | 分块上传任务为**进程内内存态**（uploadId→任务 map + 临时分块目录），1 小时无活动自动作废清理；面板重启全部任务作废（客户端整任务重传） | 断点续传二期（OUT-4）；分块任务为过程数据 P7（PRD 第九部分），不落库 |
| D-11 | 混合模式（MIXED）声明：全部对象实际为自管（数据源=本机文件系统，无外部同步源，R-09），不设 manage_mode 字段，权限管控按角色区分（ADR-018） | 分册 Step 2 默认混合模式；本模块无托管侧 |
| D-12 | trash_entry 冗余 operator_name 快照（对齐 account-auth D-06）：面板用户删除后回收站记录仍可读；操作者仅作审计留痕，不作跨域聚合引用（domain-model.md 第五部分声明） | 数据溯源冗余策略 |
| D-13 | 重命名与压缩产物的同名冲突=**直接错误**（FILE_004），不提供覆盖决策 | 重命名误覆盖/压缩产物覆盖为意外损失，从紧把握（PRD FR-09/FR-14 未列覆盖语义）；上传/移动/解压的覆盖属操作者明确意图故走 D-07 决策流 |
| D-14 | 系统用户/组候选清单**每次请求实时读取**（读系统用户与组实况），无缓存、不落库 | 候选清单为下拉选项过程数据（P4，PRD 第九部分）；实时读取即最新实况（service-mgmt I-01 同口径）；本模块不做系统账号管理（1.2 边界） |
| D-15 | trash 内唯一标识子目录名=`{deletedAt 时间戳}-{8 字节随机串}`；入站=条目 rename 至 `.dougong-trash/{标识}` + trash_entry 落库**同一事务**；跨设备 EXDEV 降级为 copy+delete，降级事实记审计（detail.degraded=true），降级失败则整次删除失败、原条目不动 | D1 BA 2.3.1.1 落定方案；rename 与落库同事务保证「记录↔内容」一致性边界（domain-model.md trash_entry A1） |
| D-16 | 流式响应账：download/content 的错误必须发生在开流之前——校验链全部通过后才写响应头开流；开流后 IO 错误以截断流呈现、服务端日志兜底 | 通用 HTTP 约束（200 头已发不可改写 JSON），如实声明不做伪错误通道；审计仍按动作结果如实记录（D-09） |

### 1.4 领域对象总览

| 聚合根 | 域内对象/值对象 | 子流程 | 关联领域对象 |
|--------|---------------|--------|-------------|
| trash_entry（主数据类子形态：生命周期状态机，非流程流转） | file_entry（实体，文件系统实时投影不落库）、受管根配置（域级单例配置实体，落 fs_path_scope） | —（分块上传为单任务过程，入站登记为删除动作同步链路，domain-model.md 第四部分声明） | 无（消费终模块；trash_entry.operator_name 为审计留痕快照非聚合引用） |

无领域事件与 MQ（单机单体，R-09；审计为进程内 OHS 出站调用，D-09）。

**功能类型判定**（分册 Step 1）：聚合根 trash_entry 存在、由本模块管理且有写操作（还原/清空）、状态为**数据生命周期**（in_trash → restored/purged，非流程流转、无审批）→ **主数据类子形态**；模块整体为「主数据型聚合根 + 工具类文件动作集合」的混合形态——无 CRUD+options 对外服务（服务消费终模块，1.5）、无审批子流程。据此裁剪：第 2 章不适用（单聚合根）、4.1.2 状态模型按 trash_entry 状态机填写、4.2 按领域对象组织（file_entry 无状态机，service-mgmt 同构处理）、第 5 章必填（有 DDL）。

### 1.5 上下文映射

| 上下文 | 关系 | 机制 |
|--------|------|------|
| account-auth | 遵奉者 + OHS 消费 | 全部接口位于 Cookie 会话后（R-02，AuthRequired）；查看两角色同等（含回收站列表）、变更与下载 admin 专属（ADR-018，RoleRequired + AUTH_008 边界拒绝）；写接口 CSRF（R-02，CsrfGuard）；审计写入 AuditService.Record（R-03，D-09 降级语义与两处同事务例外） |
| panel-core | 消费其 HTTP 骨架 | `/files` 路由组挂载；入口面/外壳归 panel-core（R-10） |
| 本机文件系统 | 数据源与动作通道 | files 适配器（安全路径层，R-04 技术规约）+ FSGateway 包装（4.3.5）；路径逃逸测试为 M2 验收硬项（D0 §2）；非外部业务系统对接，无 ACL 必要（R-09 零外呼） |
| 本机系统账号实况 | 只读数据源 | chown 候选清单实时读取（D-14）；不做系统账号管理（1.2 边界） |
| 前端文件管理页 | 唯一消费方 | 仅经 22 个 HTTP 接口消费（5.1）；无其他业务模块依赖本模块 |

> 跨聚合根设计（第 2 章）：不适用——本模块单聚合根（trash_entry），无跨聚合根约束/编排（裁剪矩阵第 2 章「仅多聚合根模块」）；file_entry/受管根配置为无守门域内对象（domain-model.md 第一部分判定），对象级约束以不变量形式列于 4.1/4.2。

## 3. 应用/基础设施层设计

### 3.1 接口规范

| 项 | 规范 |
|----|------|
| 路径格式 | `/files/{resource}/{action}` 三段式；资源单数（path-scope/entry/upload/trash/system-accounts） |
| HTTP 方法 | 读=GET，写=POST（面板内部契约，account-auth D-07 同一约定）；下载为 GET 文件流 |
| 响应格式 | `{"code": 0, "message": "ok", "data": {...}}`；code=0 成功，非 0 见 3.4 错误码（与 account-auth D-07 同一契约）；下载/内容读取的流式成功为 200 + 文件流（校验先行，通过后才开流，D-16） |
| 时间格式 | ISO-8601 UTC（公共约定） |
| 认证 | 复用 account-auth：HttpOnly Cookie `dg_session`（AuthRequired） |
| CSRF | 全部 POST 动作须带 `X-CSRF-Token` 头（R-02，CsrfGuard）；GET 豁免 |
| 中间件链 | AuthRequired → CsrfGuard（POST）→ RoleRequired（按接口，3.3 矩阵）→ PathScopeGuard（携带路径参数的接口，D-01） |
| 用户类型标注 | 查看接口（entry/list、trash/list）：admin+readonly（ADR-018 全可看）；全部写接口与下载/编辑读取：admin（readonly 无界面入口 + 越界 AUTH_008 全局边界拒绝）；白名单配置读写：admin（安全配置，FL readonly 章无此功能） |

### 3.2 依赖与约束

| 依赖 | 类型 | 约束 |
|------|------|------|
| account-auth 中间件 | 框架 | AuthRequired/CsrfGuard/RoleRequired 注册顺序见 3.1；不得自建会话解析与角色判定 |
| account-auth AuditService.Record | OHS | 操作审计经其写入（R-03）；文件系统操作失败重试一次后日志兜底（D-09），不得自建审计表 |
| files 适配器（安全路径层）+ 本机文件系统 | 数据源与动作通道 | 全部路径先 EvalSymlinks 解析真实路径再白名单校验（R-04/D-01）；以真实路径执行；fs 通道错误归一 FILE_013；路径逃逸测试 M2 验收硬项 |
| panel-core | 框架 | 路由组挂载 `/files`；不得越界承载业务逻辑 |
| SQLite | 存储 | trash_entry/fs_path_scope 两表（5.3）；单连接写串行；D-09/D-12/D-15 事务语义 |

### 3.3 权限模型

角色：admin / readonly（panel_role 固定枚举）；本模块无访客可达面（全部位于登录会话后）。

**接口权限矩阵**：

| 接口 | admin | readonly | CSRF |
|------|-------|----------|------|
| GET /files/path-scope/get | ✓ | ✗（安全配置，readonly 无入口） | —（GET） |
| POST /files/path-scope/update | ✓ | ✗（同上） | ✓ |
| GET /files/entry/list | ✓ | ✓（ADR-018 全可看） | —（GET） |
| POST /files/entry/upload | ✓ | ✗（ADR-018 禁上传） | ✓ |
| POST /files/upload/chunk-init / chunk / chunk-complete | ✓ | ✗（同上） | ✓ |
| GET /files/entry/download | ✓ | ✗（ADR-018 禁下载） | —（GET 只读） |
| GET /files/entry/content | ✓ | ✗（编辑前置读取，ADR-018 禁变更） | —（GET 只读） |
| POST /files/entry/content-save | ✓ | ✗（ADR-018 禁变更） | ✓ |
| POST /files/entry/create / rename / move / delete | ✓ | ✗（无界面入口 + AUTH_008 边界拒绝） | ✓ |
| POST /files/entry/chmod / chown | ✓ | ✗（同上） | ✓ |
| GET /files/system-accounts/list | ✓ | ✗（chown 候选数据源，随写接口 admin） | —（GET） |
| POST /files/entry/archive / unarchive | ✓ | ✗（同上） | ✓ |
| GET /files/trash/list | ✓ | ✓（ADR-018 全可看） | —（GET） |
| POST /files/trash/restore / purge | ✓ | ✗（ADR-018 禁变更） | ✓ |

数据权限规则：无行级隔离——目录内容与回收站列表两角色可见集完全一致（含隐藏条目全量呈现，D1 解释性决策）；角色差异仅体现在写接口与下载/编辑读取。readonly 越界由全局中间件 RoleRequired 拦截返回 AUTH_008（附边界原因），接口层不自建权限分支。

### 3.4 公共规范

**错误码**（FILE_ 前缀为本模块新增 15 个；鉴权/CSRF/越界复用 account-auth 码族）：

| code | 标识 | HTTP 语义 | 说明 |
|------|------|----------|------|
| 0 | OK | 成功 | — |
| FILE_001 | INVALID_PATH | 参数错误 | 路径形态非法（非绝对路径/含非法段，R-14） |
| FILE_002 | PATH_OUT_OF_SCOPE | 拒绝 | 路径越界：不受管根覆盖或命中排除目录（R-04/D-01，data.reason 注明两类之一） |
| FILE_003 | ENTRY_NOT_FOUND | 拒绝 | 目标条目不存在（已删除/路径错误，实况校验） |
| FILE_004 | ENTRY_EXISTS | 拒绝 | 目标已存在且该操作不支持覆盖（新建/重命名/压缩产物，D-13） |
| FILE_005 | CONFLICT_DETECTED | 待决策 | onConflict=ask 预检发现同名冲突；data.conflicts[] 逐项给出；**零落盘**（D-07），携带决策重发 |
| FILE_006 | NOT_EDITABLE | 拒绝 | 不可在线编辑：二进制（空字节/UTF-8 探测）或超 1MiB（D-05），提示下载后本地处理 |
| FILE_007 | INVALID_NAME | 参数错误 | 名称非法：含路径分隔符/空/保留名（R-14） |
| FILE_008 | INVALID_ARCHIVE_FORMAT | 参数错误 | format 不在 archive_format 取值内 |
| FILE_009 | ARCHIVE_ENTRY_UNSAFE | 拒绝 | 解压条目越界（zip slip，R-04 归档延伸）；全量预检失败**整体不落盘**（D-07） |
| FILE_010 | CHUNK_INVALID | 参数错误 | 分块非法：uploadId 无效/已作废、index 越界、与 total 不一致（D-04/D-10） |
| FILE_011 | TOO_LARGE | 参数错误 | 超限：直传单文件>5MiB（提示走分块，D-04）或编辑内容>1MiB（D-05） |
| FILE_012 | NOT_A_DIRECTORY | 拒绝 | 目标目录不存在或不是目录（上传/移动/解压目标） |
| FILE_013 | FILESYSTEM_ERROR | 操作失败 | 文件系统错误（磁盘满/权限不足/EXDEV 降级失败/IO 错误），data.reason |
| FILE_014 | TRASH_ENTRY_NOT_FOUND | 拒绝 | 回收站条目不存在或不在站（已还原/已清空/面板外被清，FR-17 语义） |
| FILE_015 | INVALID_PERMISSION | 参数错误 | chmod 权限位非法（非 0~0777 八进制）/chown 候选不存在（D-14 实况校验） |
| AUTH_001/002 | （复用） | 401 | 未登录/会话过期——中间件归 account-auth |
| AUTH_003 | （复用） | 403 | CSRF 校验失败（POST 动作） |
| AUTH_008 | （复用）FORBIDDEN | 拒绝 | readonly 越界，附边界原因（ADR-018） |

**枚举**（均在 workspace/01-solution/04-business-enum.md 定义；archive_format/trash_entry_status/file_type 为 D2 定稿，本次为引用无新增）：

| 枚举标识 | 取值 | 用途 |
|---------|------|------|
| archive_format | tar \| tar.gz \| zip | 压缩格式三选一（archive 请求参数 + unarchive 支持格式，FR-14/15） |
| trash_entry_status | in_trash \| restored \| purged | 回收站条目状态（trash/list 仅列 in_trash；restored/purged 终态不出现，FR-17/第六部分状态机） |
| file_type | file \| directory \| symlink | 目录条目类型（entry/list 响应 entryType，FR-03） |
| panel_role | admin \| readonly | 权限模型角色（引用，account-auth 定稿） |
| audit_action / audit_result | change \| security / success \| failure | 操作审计类别与结果（引用；下载=security，D-08） |

**业务常量**（审计细粒度动作码，非枚举池条目）：`path_scope_updated / file_uploaded / file_downloaded / file_content_saved / file_created / file_renamed / file_moved / file_deleted / file_chmod / file_chown / file_archived / file_unarchived / trash_restored / trash_purged`（action 列存此码，category 见 4.x 各接口详情；object_type=`file_entry`/`trash_entry`/`fs_path_scope`，object_id=受管根内绝对路径或 trashId）。

**业务常量**（操作参数与阈值，非枚举池条目）：

| 常量 | 值 | 说明 |
|------|-----|------|
| DIRECT_UPLOAD_LIMIT | 5MiB | 单请求直传单文件上限（D-04；超限 FILE_011 走分块） |
| UPLOAD_CHUNK_SIZE | 5MiB | 分块大小（D-04；编译期常量，D0 建议采纳） |
| CHUNK_TASK_TTL | 1h | 分块任务无活动作废时限（D-10；断点续传二期 OUT-4） |
| EDIT_SIZE_LIMIT | 1MiB | 在线编辑大小上限（D-05） |
| EDIT_BINARY_PROBE | 前 8KB | 二进制检测窗口：空字节 + UTF-8 有效性探测（D-05） |
| SORT_FIELDS / ORDERS | name\|size\|mtime / asc\|desc | 目录排序参数取值域（接口参数，非业务枚举） |
| ON_CONFLICT | ask\|overwrite\|skip（还原为 ask\|overwrite） | 冲突决策参数取值域（D-07；ask 默认） |
| TRASH_DIR_NAME | `.dougong-trash/`（受管根下） | 单一回收站目录（R-17/ADR-021；标识子目录规则见 D-15） |

**配置项**：无——阈值/上限/排序域均为编译期常量（D-03/D-04/D-05 定值，无用户可配置面）；白名单边界本身可配置（经 fs_path_scope 表与 /files/path-scope/update 接口，非配置键）。

## 4. 聚合根与领域对象详细设计

### 4.1 trash_entry（聚合根，主数据类子形态）

#### 4.1.1 领域模型

**表结构组合**：trash_entry 单表（唯一持久化业务对象，ADR-021）。

**字段清单**：

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | 聚合根标识（trashId） |
| original_path | TEXT | NOT NULL | 原路径（受管根内绝对路径；还原目标，FR-18） |
| trash_name | TEXT | NOT NULL UNIQUE | trash 内唯一标识子目录名（D-15 规则） |
| entry_type | TEXT | NOT NULL CHECK(in file,directory) | 条目类型（符号链接按 file 归类；与 file_type 枚举值域不同，仅两值） |
| size | INTEGER | NOT NULL DEFAULT 0 | 删除时条目大小（目录为累计值） |
| operator_id | INTEGER | NULL | panel_user.id（留痕） |
| operator_name | TEXT | NOT NULL | 冗余快照（D-12） |
| status | TEXT | NOT NULL DEFAULT 'in_trash' CHECK(in in_trash,restored,purged) | trash_entry_status |
| deleted_at | DATETIME | NOT NULL | 删除入站时间 |
| resolved_at | DATETIME | NULL | 还原时间（purged 行物理删除、无 resolved_at） |

索引：idx_trash_status(status, deleted_at DESC)（列表查询，D-03）。

**不变量**（聚合内）：

| 编号 | 不变量 | 违反处理 |
|------|--------|---------|
| I-01 | 一致性边界：trash_entry 记录与 `.dougong-trash/{trash_name}` 条目内容必须同步——入站（rename+INSERT 同事务，D-15）、还原（移回+UPDATE restored 同事务）、清空（内容删除+记录 DELETE 同事务） | 三动作各自同事务（D-15/4.1.4）；破坏（面板外干预）时列表按实况对账剔除孤儿记录并标注（PRD N2） |
| I-02 | 状态机单向：in_trash → restored（记录保留）/ in_trash → purged（记录物理删除）；restored/purged 为终态，不可逆 | 非在站条目的 restore/purge 请求返回 FILE_014 |
| I-03 | 唯一标识唯一：trash_name 全局唯一（UNIQUE），含时间戳+随机串不重名（D-15） | 生成冲突重试（概率可忽略） |
| I-04 | 还原目标经 PathScopeGuard（R-04）：原路径已不受管（如白名单收紧）时还原拒绝 FILE_002 | D-01 前置 |
| I-05 | 操作者快照：operator_name 落库后不随用户删除/改名变化（D-12） | 冗余字段不可变 |

**领域服务**：TrashService（入站登记/还原/清空编排 + I-01 事务边界 + EXDEV 降级）、TrashIdGenerator（D-15 标识生成）。

#### 4.1.2 状态模型

`in_trash → restored`（还原成功，记录保留供追溯，resolved_at 落库）/ `in_trash → purged`（清空，记录物理删除，销毁事实由审计留痕）。restored/purged 均终态不可逆；无驳回/撤回环节（还原失败条目保持 in_trash，PRD N2）。状态历史：还原/清空动作经 audit_log（trash_restored/trash_purged）追溯，purged 记录本身不留行。与 PRD 第六部分辨析一致：file_entry 无面板侧状态机（投影）、分块任务状态为过程数据。

#### 4.1.3 子流程设计

无（入站登记为删除动作的同步机制链路 FR-16；分块上传为单任务过程——domain-model.md 第四部分声明）。

#### 4.1.4 接口设计

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /files/trash/list | GET | admin+readonly | 在站条目清单（D-03 全量） |
| /files/trash/restore | POST | admin | 还原至原路径（onConflict: ask\|overwrite，D-07） |
| /files/trash/purge | POST | admin | 清空全部在站条目（终局销毁） |

**接口详情**：

- **GET /files/trash/list**：无参数（全量在站）。处理：按 status=in_trash 查询（I-02）→ 按 trash 实况对账（trash 目录已不存在的孤儿记录剔除并附 warning 标注，PRD N2）→ deleted_at 倒序（D-03）。响应 data=`{items:[{id, originalPath, entryType, size, operatorName, deletedAt}], total}`。审计：—（查看类）。
- **POST /files/trash/restore**：req `{trashId, onConflict?}`（onConflict 默认 ask，取值 ask\|overwrite——「放弃」=客户端不重发，D-07）。校验顺序：条目存在且 in_trash（FILE_014）→ 原路径经 PathScopeGuard（FILE_002，I-04）→ 预检原路径占用：未占用直接执行；占用且 ask → FILE_005（data.conflicts=[{path}]）零动作；占用且 overwrite → 覆盖。执行：条目 rename 回原路径 + status=restored、resolved_at=now（同事务，I-01）。响应 data=`{id, originalPath, status:"restored"}`。审计：trash_restored（change；被覆盖时 detail.overwritten=true）。
- **POST /files/trash/purge**：无 req（清空全部在站）。执行：逐条删除 `.dougong-trash/{trash_name}` 内容 + DELETE 记录（同事务，I-01；purged 为物理删除语义，I-02）。响应 data=`{purgedCount}`。审计：trash_purged（change，detail={purgedCount}）；空站清空为幂等成功（purgedCount=0，PRD N3）。

**跨接口复用规则**：在站校验统一 TrashGuard（FILE_014）；还原目标校验复用 PathScopeGuard（D-01）；审计统一走 `AuditService.Record`（D-09）。

#### 4.1.5 领域事件与集成

仓储契约：`TrashRepository { Insert(tx), GetByID, ListInTrash, UpdateRestored(tx), DeleteAllInTrash(tx), Delete(tx) }`。无 MQ、无领域事件；关联域调用：AuditService.Record（出站，D-09）。

### 4.2 file_entry（域内对象-实体，文件系统投影链路）

#### 4.2.1 领域模型

**表结构组合**：无表（kb 拍板零持久化）。file_entry 驻留进程内，为每次目录读取时重建的投影条目；全部动作（读/写/移/删）以真实路径直达文件系统（FSGateway），不经投影对象中转持久化。

**投影条目字段清单**（entry/list 与单点读取响应的共同数据源）：

| 字段 | 类型 | 说明 |
|------|------|------|
| name | string | 条目名（不含路径） |
| path | string | 受管根内绝对路径（单点寻址标识，D-01） |
| entryType | enum(file_type) | file/directory/symlink |
| size | integer | 字节数（目录为 0，目录大小不计子树） |
| mode | string | 权限位八进制（如 `0644`；rwx 分组呈现归 D4） |
| ownerUser / ownerGroup | string | 属主用户/组名（系统实况） |
| mtime | string | 修改时间（ISO-8601 UTC） |

**不变量**：

| 编号 | 不变量 | 违反处理 |
|------|--------|---------|
| I-06 | 投影真实性：list 响应恒为当次目录读取快照，无缓存层、无陈旧回退（含隐藏条目全量呈现，D1 解释性决策）；generatedAt 标注投影时刻 | 读取侧保证（每次请求即时读取，D-03） |
| I-07 | 零持久化：file_entry 不落库，面板重启无本地文件台账；操作历史经 audit_log（file_* 动作码） | 结构保证（无表） |
| I-08 | 寻址校验前置：任何携带路径的接口先 PathScopeGuard（真实路径在受管根内且未命中排除清单），后实况存在性校验（FILE_003），再执行（D-01） | PathScopeGuard 统一前置 |
| I-09 | 原子落盘：上传/编辑保存走「临时文件 + rename」，覆盖类替换无中间态（R-05）；失败清理临时文件、不留半写文件 | 写侧编排 |
| I-10 | 隐藏条目透明：不设过滤开关，`.` 开头条目随全量返回（运维场景直视 `.env` 等，D1 解释性决策） | 读取侧保证 |

**领域服务**：FileProjector（目录投影组装 + 排序）、EntryActionService（新建/重命名/移动/删除编排）、TransferService（上传/分块/下载编排 + UploadChunkStore，D-10）、EditService（可编辑性检测 + 全量覆盖保存，D-05）、MetaService（chmod/chown 编排 + 候选清单读取，D-14/D-06）、ArchiveService（压缩/解压编排 + 条目级 PathScopeGuard 预检，D-07）、PathScopeGuard（R-04 两步校验，全接口前置）、FSGateway（文件系统通道包装，4.2.5）。

#### 4.2.2 状态模型

面板侧无自有状态机（I-07 零持久化投影，PRD 第六部分辨析①）；条目属性变化=文件系统实况变迁的投影刷新。分块任务「进行中/完成/失败」为过程数据 P7 不入状态模型（D-10）。无审批/驳回环节；无状态历史表（操作追溯经 audit_log）。

#### 4.2.3 子流程设计

无（分块上传为「init→chunk×n→complete」单任务过程，无独立业务生命周期状态；压缩/解压为单动作执行——domain-model.md 第四部分声明）。

#### 4.2.4 接口设计

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /files/path-scope/get | GET | admin | 白名单配置查看（FR-02，⚠️A12） |
| /files/path-scope/update | POST | admin | 受管根与排除目录修改（同事务审计+刷新副本，D-02） |
| /files/entry/list | GET | admin+readonly | 目录内容浏览（FR-03/04；S-20-02 导航为前端路径拆分+复用本接口，无独立接口） |
| /files/entry/upload | POST | admin | 单请求直传（≤5MiB/文件，多文件，D-04/D-07） |
| /files/upload/chunk-init | POST | admin | 分块任务初始化（D-10） |
| /files/upload/chunk | POST | admin | 分块上传（逐块校验） |
| /files/upload/chunk-complete | POST | admin | 合并落盘（校验+原子 rename+审计） |
| /files/entry/download | GET | admin | 单文件流式下载（数据取出，D-08） |
| /files/entry/content | GET | admin | 读取文本内容（编辑打开，D-05 可编辑性检测） |
| /files/entry/content-save | POST | admin | 全量覆盖保存（I-09，最后写者胜） |
| /files/entry/create | POST | admin | 新建空文件/空目录（FR-09） |
| /files/entry/rename | POST | admin | 原目录内重命名（同名直接错误，D-13） |
| /files/entry/move | POST | admin | 跨目录移动（含子树；EXDEV 降级同 D-15 口径） |
| /files/entry/delete | POST | admin | 删除入回收站（FR-11/FR-16 同事务，D-15） |
| /files/entry/chmod | POST | admin | 权限位修改（不递归，D-06） |
| /files/entry/chown | POST | admin | 属主修改（候选经 system-accounts/list） |
| /files/system-accounts/list | GET | admin | 系统用户/组候选清单（实时读取，D-14） |
| /files/entry/archive | POST | admin | 压缩（archive_format 三选一；产物同名直接错误，D-13） |
| /files/entry/unarchive | POST | admin | 解压（逐条目防 zip slip 全量预检，D-07/D-01） |

**接口详情**（请求/响应/校验；全部写接口隐含 CSRF 与 AUTH_008 角色边界；携带路径接口隐含 PathScopeGuard→FILE_002 与实况校验→FILE_003 前置，不再逐条重复）：

- **GET /files/path-scope/get**：data=`{rootPath, excludedDirs[]}`（fs_path_scope 当前值）。审计：—。
- **POST /files/path-scope/update**：req `{rootPath, excludedDirs[]}`。校验（R-14）：rootPath 为绝对路径形态（FILE_001）；excludedDirs 每项为绝对路径（FILE_001）且均不等于/不包含 rootPath 本身（排除受管根=全禁无意义，FILE_002 语义拒绝）。执行：UPDATE fs_path_scope + 审计**同事务**（D-02/D-09 例外）→ 提交后刷新进程内副本（后续请求即予生效）。响应 data=`{rootPath, excludedDirs}`（刷新值）。审计：path_scope_updated（change）。
- **GET /files/entry/list**：query `{path, sort?(name\|size\|mtime，默认 name), order?(asc\|desc，默认 asc)}`。校验：path 形态（FILE_001）→ PathScopeGuard（FILE_002）→ 目标为目录（FILE_012）。处理：即时读取目录条目（I-06）→ 服务端排序（D-03）。响应 data=`{path, items:[{name, path, entryType, size, mode, ownerUser, ownerGroup, mtime}], total, generatedAt}`（符号链接不展开目标、按链接自身呈现）。审计：—（查看类）。
- **POST /files/entry/upload**：multipart/form-data（`dir` 字段 + 多个 `files` 字段 + `onConflict` 字段默认 ask）。校验：dir 为目录（FILE_012）；单文件 ≤5MiB（超限 FILE_011 提示走分块，D-04）；onConflict 取值域（D-07）。处理：全量同名预检（ask）→ FILE_005（data.conflicts=[{name}]）零落盘；overwrite/skip → 逐文件临时文件+rename（I-09）。响应 data=`{results:[{name, path, status:"ok"\|"skipped"}], uploadedCount}`（ask 冲突时整体 FILE_005）。审计：file_uploaded（change，detail={names, uploadedCount}；skip 不计入）。
- **POST /files/upload/chunk-init**：req `{dir, fileName, totalSize, totalChunks}`。校验：dir 目录（FILE_012）；totalSize/totalChunks 正整数且 totalChunks=ceil(totalSize/5MiB)（FILE_010）；同名预检（ask → FILE_005 零动作）。响应 data=`{uploadId}`（内存任务，D-10）。审计：—（未生效）。
- **POST /files/upload/chunk**：multipart（`uploadId` + `index` + `chunk`）。校验：任务存在未作废、index∈[0,totalChunks)、无重复（FILE_010）。执行：分块写入临时目录。响应 data=`{uploadId, received, total}`。审计：—（过程块）。
- **POST /files/upload/chunk-complete**：req `{uploadId}`。校验：任务存在（FILE_010）；全部块齐且累计大小=totalSize（FILE_010）。执行：按 index 序合并至临时文件 → rename 原子落盘（I-09）→ 清理临时目录与任务。响应 data=`{path, size}`。审计：file_uploaded（change，detail={chunked:true, totalSize}）。
- **GET /files/entry/download**：query `{path}`。校验：PathScopeGuard → 存在 → 为文件（目录 FILE_012 拒绝，目录打包下载 OUT-3）。响应：校验全过才开流——200 + 文件流（Content-Disposition: attachment; filename，D-16 账）；流前置校验失败返回 JSON 错误体。审计：file_downloaded（**security**，D-08）。
- **GET /files/entry/content**：query `{path}`。校验：存在 → 为文件 → 大小 ≤1MiB（FILE_011，D-05）→ 前 8KB 空字节 + UTF-8 探测（FILE_006）。响应 data=`{path, size, mtime, content}`（UTF-8 文本全量）。审计：—（打开未变更）。
- **POST /files/entry/content-save**：req `{path, content}`。校验：UTF-8 编码、长度 ≤1MiB（FILE_011）。执行：临时文件+rename 全量覆盖（I-09；最后写者胜无乐观锁，D-05）。响应 data=`{path, size, mtime}`（新值供前端刷新）。审计：file_content_saved（change）。
- **POST /files/entry/create**：req `{dir, name, type}`（type=file\|directory）。校验：name 合法（FILE_007）；同名已存在 → FILE_004 直接错误（D-13 不覆盖）。执行：创建空文件/空目录。响应 data=`{path, entryType}`。审计：file_created（change，detail={type}）。
- **POST /files/entry/rename**：req `{path, newName}`。校验：newName 合法（FILE_007）；同目录同名已存在 → FILE_004（D-13）。执行：同目录 rename。响应 data=`{path}`（新路径）。审计：file_renamed（change，detail={oldPath}）。
- **POST /files/entry/move**：req `{sourcePath, targetDir, onConflict?}`（默认 ask）。校验：targetDir 为目录（FILE_012）；目标同名预检（ask → FILE_005 零动作；skip → 跳过该项；overwrite → 覆盖，D-07）。执行：rename；EXDEV → copy+delete 降级（detail.degraded=true，D-15 同口径）。响应 data=`{moved:[{from, to}], skippedCount}`。审计：file_moved（change）。
- **POST /files/entry/delete**：req `{path}`。执行：TrashService 入站登记——生成 trash_name（D-15）→ rename 至 `.dougong-trash/{trash_name}`（EXDEV 降级 copy+delete，detail.degraded=true）+ trash_entry INSERT **同一事务**（I-01/D-15）。响应 data=`{trashId, trashName}`。审计：file_deleted（change；降级事实 detail.degraded=true 同事务落审计）。
- **POST /files/entry/chmod**：req `{path, mode}`（八进制字符串，如 `0644`）。校验：mode ∈ 0~0777（FILE_015）。执行：仅目标自身，不递归（D-06）。响应 data=`{path, mode}`。审计：file_chmod（change，detail={mode}）。
- **POST /files/entry/chown**：req `{path, ownerUser?, ownerGroup?}`（至少一项）。校验：候选实况存在（FILE_015，D-14）。执行：仅目标自身（D-06）。响应 data=`{path, ownerUser, ownerGroup}`。审计：file_chown（change）。
- **GET /files/system-accounts/list**：实时读取系统用户与组清单（D-14；仅 name/uid、name/gid，home/shell 不下发——载荷考虑）。响应 data=`{users:[{name, uid}], groups:[{name, gid}]}`。审计：—（查看类）。
- **POST /files/entry/archive**：req `{paths[], format, targetName?}`。校验：format ∈ archive_format（FILE_008）；paths 全量 PathScopeGuard 预检（任一越界 FILE_002 整体不执行）；产物名缺省=`archive.{ext}`（按 format）、产物同名已存在 → FILE_004（D-13）。执行：多选打包（含目录递归）落当前浏览目录。响应 data=`{path, size, format}`。审计：file_archived（change，detail={format, count}）。
- **POST /files/entry/unarchive**：req `{archivePath, targetDir, onConflict?}`（默认 ask）。校验：format 识别（不识别 FILE_008）；targetDir 为目录（FILE_012）。处理：**全量条目预检**——①逐条目解析条目声明路径的还原真实路径并过 PathScopeGuard（任一越界 → FILE_009，整体不落盘=D-07 回滚语义，PRD FR-15）；②目标同名预检（ask → FILE_005 零落盘；overwrite/skip 照 D-07）。预检全过才解压落盘。响应 data=`{extractedCount, targetDir}`。审计：file_unarchived（change，detail={format, extractedCount}）。

**D-16（流式响应账）**：download/content 的成功响应为流/大载荷，错误必须发生在开流之前——校验链全部通过后才写响应头开流；开流后 IO 错误以截断流呈现并由服务端日志兜底（无法改写已发的 200 头，通用 HTTP 约束，如实声明）。

**跨接口复用规则**：PathScopeGuard 为全接口统一前置（D-01/I-08，SVC 无关码 FILE_002/009）；冲突决策统一 ConflictResolver（D-07：预检→FILE_005→零动作）；分块任务统一 UploadChunkStore（D-10）；入站登记统一 TrashService（D-15）；审计统一走 `AuditService.Record`（D-09）。

**幂等语义**：restore/purge 重复提交——前者第二请求报 FILE_014（条目已不在站，如实），后者幂等成功（purgedCount=0，PRD N3）；create/rename 重复提交报 FILE_004（如实）；上传重复提交产生重复文件（无去重语义，操作者可见后果后自行处理）；分块 chunk 重复提交拒绝（FILE_010 重复块）。防重复提交为前端在途态（P3 过程数据），非服务端一致性手段。

#### 4.2.5 领域事件与集成

无 MQ、无领域事件。「仓储契约」两分：**SQLite 侧**——`PathScopeRepository { Get, Update(tx) }`（fs_path_scope；进程内副本由 PathScopeGuard 持有、D-02 写后刷新）；**文件系统侧网关契约**（进程内适配层，非 SQL；全部真实路径执行，R-04 出口收敛）：

```
FSGateway {
  ListDir(realPath) ([]EntrySnapshot, error)         // 目录投影（I-06；含隐藏条目）
  Stat(realPath) (EntryStat, error)                  // 单点属性
  ReadFile(realPath, limit) ([]byte, error)          // 编辑打开（D-05 上限）
  WriteFileAtomic(realPath, data) error              // 临时文件+rename（I-09）
  Mkdir / CreateFile(realPath) error                 // 新建
  Rename(from, to) error                             // 重命名/移动（EXDEV 错误上抛触发降级）
  CopyThenRemove(from, to) error                     // EXDEV 降级（D-15，调用方记审计）
  RemoveTree(realPath) error                         // 递归删除（仅 purge 清 trash 内容用）
  Chmod / Chown(realPath, ...) error                 // 元数据修改（D-06 不递归）
  OpenReadStream(realPath) (stream, error)           // 下载（校验后开流，D-16）
  ListSystemAccounts() (users, groups, error)        // 候选清单实时读取（D-14）
}
约束：入参一律为 PathScopeGuard 校验后的真实路径（唯一放行口径）；错误归一 FILE_013 语义
```

ArchiveService 基于 FSGateway 流式读写归档，条目路径还原与逐条目 PathScopeGuard 预检在 ArchiveService 内完成（D-07）。关联域调用（出站）：AuditService.Record（account-auth OHS，R-03，D-09）；无其他出站；被调用契约：无（服务消费终模块，1.5）。

## 5. 模块清单汇总

### 5.1 对外服务清单（HTTP 接口）

| 服务（接口） | 消费方 | 契约要点 |
|-------------|--------|---------|
| GET /files/path-scope/get · POST /files/path-scope/update | 前端文件管理页（admin） | 白名单配置治理（⚠️A12；同事务审计+副本刷新，D-02） |
| GET /files/entry/list | 前端文件管理页（两角色） | 目录全量投影 + 服务端排序（D-03/I-06） |
| POST /files/entry/upload · /files/upload/{chunk-init,chunk,chunk-complete} | 前端文件管理页（admin） | 直传 ≤5MiB / 分块三段式（D-04/D-10）；冲突 ask 预检（D-07） |
| GET /files/entry/download | 前端文件管理页（admin） | 单文件流式下载；审计 security（D-08/D-16） |
| GET /files/entry/content · POST /files/entry/content-save | 前端文件管理页（admin） | 1MiB 上限 + 二进制探测（D-05）；全量覆盖最后写者胜 |
| POST /files/entry/{create,rename,move,delete} | 前端文件管理页（admin） | 整理动作；删除经 TrashService 入站同事务（D-15） |
| POST /files/entry/{chmod,chown} · GET /files/system-accounts/list | 前端文件管理页（admin） | 元数据修改不递归（D-06）；候选实时读取（D-14） |
| POST /files/entry/{archive,unarchive} | 前端文件管理页（admin） | archive_format 三格式；zip slip 全量预检（D-07） |
| GET /files/trash/list · POST /files/trash/{restore,purge} | 前端文件管理页（list 两角色） | 回收站治理；状态机 I-02；还原冲突决策（D-07） |
| 中间件 AuthRequired / CsrfGuard / RoleRequired / PathScopeGuard | （复用/本模块） | 前三者 account-auth 提供；PathScopeGuard 本模块（D-01） |
| AuditService.Record | （出站调用） | 操作审计写入（R-03，D-09） |

本模块无对外 OHS 服务契约——服务消费终模块（1.5：无其他业务模块依赖）。

### 5.2 SQL 清单

| # | SQL | 场景 |
|---|-----|------|
| 1 | INSERT INTO fs_path_scope(id,root_path,excluded_dirs,...) VALUES(1,'/','/proc,/sys,/dev',...) OR IGNORE | 启动初始化（D-02） |
| 2 | SELECT * FROM fs_path_scope WHERE id=1 | 启动加载内存副本（D-02） |
| 3 | UPDATE fs_path_scope SET root_path=?, excluded_dirs=?, updated_at=? WHERE id=1 | 白名单修改（同事务审计，D-02） |
| 4 | INSERT INTO trash_entry(...) / GET by id | 入站登记（D-15 同事务）/ 还原前置校验 |
| 5 | SELECT * FROM trash_entry WHERE status='in_trash' ORDER BY deleted_at DESC | 回收站列表（D-03/I-02） |
| 6 | UPDATE trash_entry SET status='restored', resolved_at=? WHERE id=? AND status='in_trash' | 还原（同事务 rename，I-01/I-02） |
| 7 | DELETE FROM trash_entry WHERE status='in_trash' | 清空（同事务内容删除，I-01/I-02） |

### 5.3 DDL

```sql
CREATE TABLE IF NOT EXISTS trash_entry (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  original_path TEXT    NOT NULL,
  trash_name    TEXT    NOT NULL UNIQUE,
  entry_type    TEXT    NOT NULL CHECK (entry_type IN ('file','directory')),
  size          INTEGER NOT NULL DEFAULT 0,
  operator_id   INTEGER,
  operator_name TEXT    NOT NULL,
  status        TEXT    NOT NULL DEFAULT 'in_trash' CHECK (status IN ('in_trash','restored','purged')),
  deleted_at    DATETIME NOT NULL,
  resolved_at   DATETIME
);
CREATE INDEX IF NOT EXISTS idx_trash_status ON trash_entry(status, deleted_at DESC);

CREATE TABLE IF NOT EXISTS fs_path_scope (
  id            INTEGER PRIMARY KEY CHECK (id = 1),
  root_path     TEXT    NOT NULL DEFAULT '/',
  excluded_dirs TEXT    NOT NULL DEFAULT '/proc,/sys,/dev',
  updated_at    DATETIME NOT NULL
);
```

## 6. 更新记录

### 6.1 变更记录

| 版本 | 日期 | 变更类型 | 变更摘要 | 目标节号 |
|------|------|---------|---------|---------|
| v1.0 | 2026-11-12 | initial | 首次生成：22 个 HTTP 接口（/files/{path-scope,entry,upload,trash,system-accounts}/*，读=GET/写=POST；查看类 entry/list 与 trash/list 两角色，白名单配置/全部写/下载/编辑读取 admin+CSRF）+ 2 表 DDL（trash_entry 唯一持久化业务对象 + fs_path_scope 单行配置）+ 1 个 FSGateway 文件系统网关契约 + 15 错误码（FILE_001~015）+ 复用 account-auth 码族（AUTH_001/002/003/008）+ 5 枚举引用（archive_format/trash_entry_status/file_type D2 已定稿，panel_role/audit_action/audit_result 引用，无新增）+ 14 审计动作常量 + 16 项设计决策（D-01 两步校验、D-02 配置单行表即予生效、D-03 全量不分页、D-04 分块 5MiB、D-05 编辑 1MiB+二进制探测、D-06 不递归、D-07 onConflict 预检零落盘、D-08 下载审计 security、D-09 审计非同事务降级+两处例外、D-10 分块任务内存态、D-12 操作者快照、D-13 同名直接错误、D-14 候选实时读、D-15 入站同事务+EXDEV 降级、D-16 校验先行开流）；关闭 prd-file-manager.md 待对齐-1~-7；无旧版对比 | — |

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
