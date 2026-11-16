# 斗拱面板 计划任务与备份（cron-backup）API 设计

| 属性 | 值 |
|------|-----|
| 版本 | v1.0 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-14 |
| 输入 | prd-cron-backup.md v1.0（完备性检查通过 G 5/5·T 适用 6/6·N 4/4）、domain-model.md v1.14、d0-clarification-cron-backup.md、D1 三文档 v1.0 |
| 接口规约 | 无激活技术栈（ACTIVE_STACKS 为空）——按通用 REST/JSON 契约设计；认证=HttpOnly Cookie 会话 + CSRF token（R-02）；鉴权=RBAC 两角色边界矩阵（ADR-018）；与 api-account-auth.md 同一响应/错误码契约 |

## 1. 模块设计概述

### 1.1 业务背景及目标

计划任务与备份是斗拱面板 M8 里程碑模块：解决「自托管运维者如何接管既有 cron 定时任务并以面板统一治理备份」的问题——计划任务线接管 root crontab（读写唯一域+系统预置位只读，D1 口径-1），提供任务 CRUD、表达式校验预览、cron 执行日志尾部、面板代执行的手动触发；备份线提供 backup_job 定义（源=站点目录+数据库实例/库、目标=本地/rsync、保留份数、调度）、面板内置调度器定时执行与立即执行（与系统 cron 解耦不写 crontab，D1 口径-2）、backup_run 执行历史（running→success/failure，ADR-016）与产物下载、按保留份数轮转清理（R-11）。服务对象：管理员（admin，全部变更/触发/下载）、只读用户（readonly，全部查看，ADR-018）。

### 1.2 边界说明

| 项 | 说明 |
|----|------|
| 本模块做 | 计划任务 CRUD/校验预览/日志尾部/手动触发、备份任务 CRUD/立即执行/定时调度、备份历史/详情/产物下载、保留轮转（R-11） |
| 本模块不做（接口边界） | ① 备份自动还原流程（MVP 不做，产物手动下载后自行恢复）；② 云存储目标 S3/OSS、增量/去重备份（二期）；③ 跨服务器 rsync 管理界面（MVP 仅目标地址配置，D-06）；④ 备份任务启停开关（二期 OUT-5，删除任务即停调度）；⑤ cron 服务自身启停（service-mgmt）；⑥ 备份产物日常浏览通道（file-manager，本模块仅按 run 记录下载）；⑦ 站点/数据库实例/库的候选清单接口——复用 site-mgmt `/sites/site/list` 与 database-mgmt `/db/instance/list`、`/db/database/list` 既有接口（前端表单联动直接消费，本模块不建代理接口）；⑧ cronx/backupx 适配器实现（后端工程，本模块经网关契约消费，3.2） |
| 与存储的关系 | SQLite 单文件（ADR-011）；持久化=backup_job + backup_run 两表（D0 拍板，DDL 见 5.3）；cron_job 零任务台账不落库（D2 口径-1，crontab 实况投影） |

### 1.3 设计决策

| # | 决策 | 理由 |
|---|------|------|
| D-01 | cron_job 寻址=**jobKey（crontab 行内容规范串）**：表达式 + `\t` + 命令原文（首行规范化：多空段压缩）；update/delete/trigger 均按 jobKey 寻址，服务端在 job/list 组装、前端作**不透明引用**透传回——不使用行号（crontab 行号随面板外编辑漂移、跨请求不稳定） | 内容寻址跨读取稳定（同一任务面板外未动即同 key）；编辑语义=删旧行+写新行（jobKey 随内容变更，如实）。**关闭 prd-cron-backup.md 待对齐-1（标识形态部分）** |
| D-02 | 面板标记注释语法=任务行**紧邻上一行**两条注释：`# dugong: managed`（来源标注载体，存在⇔cron_source=panel）+ `# dugong: remark: <备注文本>`（备注载体，无则仅 managed 行）；面板创建/编辑时自动维护两条注释与任务行的邻接；标记缺失（面板外被删）⇔cron_source=existing（如实呈现，任务仍可管理）；列表排序=crontab 文件行序如实呈现（不重排） | 零任务台账下标注与备注的唯一自持载体=crontab 自身（D2 口径-1 落地形态）；面板外删除标记即自然降级为 existing（实况是唯一事实源）。**关闭 prd 待对齐-1（注释语法+排序部分）** |
| D-03 | 表达式语法=**标准五段（分 时 日 月 周）**，支持 `*` `,` `-` `/步进`；`@yearly/@monthly/@weekly/@daily/@hourly` 标准宏**仅在输入侧接受、写入时归一化为等价五段**（crontab 存五段，与系统语义一致）；校验接口 **POST**（表达式串常含特殊字符且预览为计算服务，不以查询串承载）；预览未来 **5** 次执行时间（编译期常量） | 取值域收敛到系统 cron 原生子集（R-14）；宏归一化避免双写方言。**关闭 prd 待对齐-3** |
| D-04 | cron 执行日志尾部=**GET /cron/log/tail?lines=**：lines 档位 `[50,100,200,500,1000]`、默认 200；来源按可用性适配（journalctl -u cron → /var/log/syslog → /var/log/cron，cronx 适配器内部判定）；响应为纯文本片段（非 JSON 数组——日志本质为行流） | 档位防失控拉取；发行版差异收归适配器。**关闭 prd 待对齐-4** |
| D-05 | 手动触发=**同步等待语义**：请求仅携带 jobKey（I-08，命令原文取自 crontab 不经请求）、以面板运行用户身份 shell 语义代执行、**超时 120s**（编译期常量）进程组终止；stdout+stderr 合并回传、**尾部截断 64KB**（超限保留尾部+头部省略标记）；响应 data=`{exitCode, output, timedOut, durationMs}`——命令执行失败/超时为**业务结果非请求错误**（200+code=0，审计记 failure 结果）；同任务并发代执行不互斥（与系统 cron 并发语义一致，D2 口径-4 仅约束备份执行） | cron 任务通常短时（分钟级调度任务），同步反馈最直接；R-08 边界口径落为接口形态。**关闭 prd 待对齐-5** |
| D-06 | rsync 目标=**MVP 仅 ssh 免密**（密钥/agent 由运行环境提供）：target_config 只含 `host/user/port/path`，面板**不存储任何 rsync 凭据**（密码/密钥模式二期随 rsync 管理界面）；本地目标=面板数据目录外任意绝对路径（服务端校验非面板自身数据目录——防备份递归覆盖自身 SQLite，R-14） | 材料未定义 rsync 认证形态，MVP 收敛为零凭据通道（最简且零泄露面）；数据库建连凭据复用 database-mgmt 已存密文（经其网关，不入本域）。**关闭 prd 待对齐-6** |
| D-07 | 立即执行=**异步提交语义**：POST /cron/backup-job/run 校验（存在+执行互斥）→ 创建 backup_run(running) → **立即返回 runId**；执行由 BackupExecutor 后台进行（备份 GB 级产物分钟级时长，不受 HTTP 超时约束）；进度经 GET /cron/backup-run/detail 轮询（D4 定轮询节奏） | 同步等待 GB 级备份必然超时；run 即进度载体（running 态+detail）。**关闭 prd 待对齐-5（备份侧语义）** |
| D-08 | 执行互斥=**DB 级保证**：`CREATE UNIQUE INDEX idx_run_running ON backup_run(job_id) WHERE status='running'`（SQLite 部分索引）——并发提交时后到者违反唯一约束→手动侧 CB_006、调度侧跳过该周期（D2 口径-4）；面板重启遗留孤儿 running=启动恢复编排置 failure（error=面板重启中断，2.3） | 互斥从应用检查升级为存储约束（竞态不可能）；孤儿 run 不清理将永久堵塞该任务 |
| D-09 | 轮转清理=**与终态回写同一 SQLite 事务**：执行成功→（UPDATE run 终态 + DELETE 轮转记录集 + audit backup_rotated）原子提交；轮转 SQL 语义=按 job_id 取最近 N 条 success（N=retention_count）为保留集，**删除 id < 保留集最小 id 的全部记录**（含更早 success 与更早 failure，D2 口径-5）；产物文件删除在事务提交后执行——删除失败则记录保留（下次轮转重试收敛）、记面板运行日志，不阻塞新执行；清理动作记 **system 审计 backup_rotated**（{deletedRuns}，对齐 account-auth audit_purged 先例：删除真实数据须留痕） | 记录↔产物一致性以记录侧事务为准（产物侧最终一致）；R-11 轮转语义一次落定。**关闭 prd 待对齐-7** |
| D-10 | 产物目录=**`{目标根}/{job 名}/{yyyyMMdd-HHmmss}/`**，内含 `manifest.json`（{jobId, jobName, sources[], startedAt, finishedAt, files:[{name,size}]}）+ 每源一份产物文件（站点目录项→`site-{序号}.tar.gz`；数据库项→`db-{实例名}-{库名}.sql.gz`）；artifact_path 记录目录定位（local=绝对路径 / rsync=远端路径）；download 按 `{runId, file}` 下载 manifest 内单文件（流式，file-manager D-16 流式响应账同判：校验全过才开流）；下载文件名=manifest 内原名 | 多源任务产物为文件集非单文件，manifest 为自描述清单（恢复时的核对依据）；D0 建议方向落地。**关闭 prd 待对齐-8（命名/清单/文件名部分）** |
| D-11 | 历史查询=**筛选 `{jobId?, status?, trigger?, from?, to?}` + 公共分页 `{page, pageSize}`**，按 id 倒序；job/list 的「最近执行结论」=**读时子查询派生**（每 job 取最新 run 摘要，不落 last_run 冗余列——account-auth D-03 计算态同构，避免双写漂移）；run 不存耗时列（finished_at-started_at 派生） | 任务量级个位数、run 总量有界（Σ保留份数），子查询零成本。**关闭 prd 待对齐-8（筛选/分页部分）** |
| D-12 | run 对 job=**弱引用+快照**：job_id 不设 FK（任务删除后历史留存，FR-09/D2 判定）+ `job_name` 冗余快照列（任务删除/改名后历史仍可读——account-auth D-06 actor_name 同构）；历史列表的 jobId 筛选对已删任务仍有效（按快照归组呈现） | 独立执行事实（ADR-016）不以任务存亡为转移 |
| D-13 | 调度器=**进程内分钟级 tick**：每分钟检查全部 job 的 schedule_cron 到期（判定误差 <1min，编译期常量）；面板重启后重算各任务下次触发点，**错过的调度点跳过不补跑、不落任何跳过记录**（D2 口径-3；面板运行日志可见）；到期触发与立即执行共用同一 BackupExecutor 通道 | 分钟级即 cron 语义精度上限；跳过不留痕=run 只记实际发生的执行（无事实不虚构）。**关闭 prd 待对齐-9** |
| D-14 | 混合模式（MIXED）声明：全部对象实际为自管（数据源=本机 crontab 实况与面板自有任务/记录，无外部同步源，R-09），不设 manage_mode 字段，权限管控按角色区分 | 分册 Step 2 默认混合模式；本模块无托管侧（service-mgmt D-07/firewall D-10 同构） |

### 1.4 领域对象总览

| 聚合根 | 域内对象/值对象 | 子流程 | 关联领域对象 |
|--------|---------------|--------|-------------|
| backup_job（主数据类） | 备份源清单（1:N 值集合）、备份目标（1:1 值对象） | — | db_instance（database-mgmt，源清单 database 项 ID 引用——**全项目首个**，domain-model v1.14 第五部分） |
| backup_run（业务流程类） | — | — | 经 job 间接引用（job_id 弱引用+job_name 快照，D-12） |
| cron_job（通道投影型，无聚合根） | — | — | 无（站点目录项为路径值快照，site 无聚合根 ID） |

### 1.5 上下文映射

| 上下文 | 关系 | 机制 |
|--------|------|------|
| account-auth | 消费 | OHS：AuditService.Record（R-03，任务 CRUD/触发/下载同事务或紧随写入）；遵奉者：Cookie+CSRF 协议（R-02）+ 只读边界（ADR-018） |
| database-mgmt | 消费（**首个关联领域对象**） | OHS（进程内）：DbGateway.DumpDatabase(inst, db)——数据库备份源建连导出（其 4.1.5/5.1 已预留，本模块正式登记消费）；实例/库清单经其既有 HTTP 接口由前端联动消费（1.2⑦）；凭据密文不离开其域（⚠️A18 精神） |
| site-mgmt | 消费 | 站点目录候选经其 `/sites/site/list` 前端联动消费（docRoot 字段）；保存后为路径值快照、不持续引用 |
| rsync 远程目标（用户基础设施） | ACL | backupx 适配器推送（R-08 参数数组；用户主动配置，R-09 例外语义，与 ACME 同判） |
| cron 本机组件 | 数据源与动作通道 | cronx 适配器承载 crontab 读写/日志读取/代执行，经 3.2 网关契约收敛（非外部业务系统，无 ACL 必要） |
| panel-core | 消费其 HTTP 骨架 | 路由挂载 `/cron` |

## 2. 跨聚合根设计

### 2.1 聚合根关系图

```
backup_job 1 ───── * backup_run   （弱引用 job_id + 冗余快照 job_name，D-12；无 FK）
    │ 1:N 备份源清单（值集合）        （任务删除不级联——run 为独立执行事实，ADR-016）
    │ 1:1 备份目标（值对象）
    │ * 消费 db_instance（database-mgmt 聚合根，跨域 ID 引用）
    └─ 定义保留份数 N ──驱动──> run 集合的轮转水位（R-11，跨聚合治理规则）
cron_job（无聚合根：crontab 实况投影，零台账）
```

### 2.2 跨聚合根不变量

| 编号 | 不变量 | 违反处理 |
|------|--------|---------|
| I-01 | 同一 backup_job 至多一条 status=running（执行互斥，D2 口径-4） | 部分唯一索引 idx_run_running（D-08）：手动侧 CB_006、调度侧跳过周期 |
| I-02 | run 终态不可逆：success/failure 一经写入不回改；重新执行=新 run | 存储层无 UPDATE 终态路径（仅 running→终态一次转移） |
| I-03 | job_name 为创建时快照；任务改名/删除不回写历史 | D-12；历史呈现以快照为准 |
| I-04 | status=success ⇔ 该 run 产物清单非空（manifest 落盘完成才转 success；任一源失败→整体 failure，error_message 列明失败源） | 执行编排保证（2.3）；failure 不可下载（CB_008） |
| I-05 | 每 job 的 success 记录数 ≤ retention_count（轮转收敛） | D-09 轮转事务；删除任务后存量记录自然冻结（无新成功即不再轮转） |
| I-06 | backup_job.name 全局唯一 | CB_005 |
| I-07 | schedule_cron 为合法五段表达式（宏已归一化，D-03） | CB_002 |
| I-08 | 手动触发的命令原文取自 crontab 记录、不经请求参数传入（R-08 边界口径） | 接口契约（trigger 仅收 jobKey） |

### 2.3 跨聚合根编排

| 编排 | 触发 | 步骤 |
|------|------|------|
| 执行编排（定时/立即共用） | 调度 tick 到期（D-13）/ POST run（D-07） | 互斥检查（I-01）→ INSERT run(running, trigger) → BackupExecutor 逐源执行（site_dir=tar 打包 / database=DbGateway.DumpDatabase 流导出，R-08）→ 产物写目标（本地/rsync）+ manifest → 同事务（终态回写 + 轮转 DELETE + audit backup_rotated〔成功时〕+ audit backup_run_triggered〔手动时〕）→ 产物清理（事务后，失败重试收敛，D-09） |
| 启动恢复编排 | 面板进程启动 | 扫描 status=running 的孤儿记录（进程已死）→ 置 failure（error=面板重启中断）+ 审计（source=system）——否则 I-01 互斥永久堵塞该任务（D-08） |
| 任务删除编排 | POST delete | 删除 backup_job 行（run/产物不级联，FR-09）→ 审计；调度器下一 tick 自然不再触发 |
| 手动触发编排（cron 侧） | POST trigger | jobKey 寻址读 crontab 原文 → JobRunner 代执行（120s 超时，D-05）→ 审计（结果=exitCode/timedOut） |

## 3. 应用/基础设施层设计

### 3.1 接口规范

| 项 | 规范 |
|----|------|
| 路径格式 | `/cron/{resource}/{action}` 三段式；资源：job / log / backup-job / backup-run |
| HTTP 方法 | 读=GET，写=POST；validate 为计算服务用 POST（表达式串不以查询串承载，D-03）；download 为 GET 流式（file-manager D-16 流式响应账同判） |
| 响应格式 | `{"code": 0, "message": "ok", "data": {...}}`；分页 data=`{items, total}`（公共约定）；log/tail 为 200+纯文本；download 为 200+文件流（Content-Disposition: attachment） |
| 时间格式 | ISO-8601 UTC（公共约定） |
| 认证/CSRF | HttpOnly Cookie 会话 + `X-CSRF-Token`（POST 必须；GET 豁免）——复用 account-auth 中间件链（AuthRequired → CsrfGuard → RoleRequired） |
| 用户类型标注 | 每接口标注：admin / admin+readonly（全可看，ADR-018） |

### 3.2 依赖与约束

| 依赖 | 类型 | 约束 |
|------|------|------|
| CronTabGateway | 进程内网关契约（cronx 适配器承载） | crontab 读写投影：List()/Upsert(job)/Delete(jobKey)/ReadRaw()；标记注释维护（D-02）；R-08 参数数组 |
| CronLogSource | 进程内契约（cronx） | Tail(lines)：journalctl → /var/log/syslog → /var/log/cron 适配（D-04） |
| JobRunner | 进程内契约（cronx） | Run(jobKey, timeout)→(exitCode, output, timedOut)：shell 语义代执行、进程组终止（D-05） |
| BackupScheduler | 进程内契约 | 分钟级 tick（D-13）；重启重算触发点、跳过不留痕 |
| BackupExecutor | 进程内网关契约（backupx 适配器承载） | Execute(run, job)：tar/mysqldump 流导出/rsync 推送全部参数数组（R-08）；manifest 写入（D-10） |
| DbGateway.DumpDatabase(inst, db) | 进程内 OHS（database-mgmt 提供，其 4.1.5） | 数据库源导出流；凭据解密在其域内完成（⚠️A18）；本模块**正式登记消费**（其 5.1 预告兑现） |
| AuditService.Record | 进程内 OHS（account-auth） | R-03；SQLite 域内变更同事务、命令/传输类动作紧随写入（D-09 口径） |
| SQLite (modernc.org/sqlite) | 存储 | backup_job/backup_run 两表（5.3）；轮转与终态回写同事务（D-09） |

### 3.3 权限模型

角色：admin / readonly（panel_role 固定枚举，池中定稿）；访客不涉及（全接口位于会话后）。

**接口权限矩阵**：

| 接口 | admin | readonly | CSRF |
|------|-------|----------|------|
| GET /cron/job/list | ✓ | ✓（全可看，含系统预置位分区） | — |
| POST /cron/job/create / update / delete / validate / trigger | ✓ | ✗（AUTH_008） | ✓ |
| GET /cron/log/tail | ✓ | ✓（查看类） | — |
| GET /cron/backup-job/list | ✓ | ✓ | — |
| POST /cron/backup-job/create / update / delete / run | ✓ | ✗（AUTH_008） | ✓ |
| GET /cron/backup-run/list / detail | ✓ | ✓ | — |
| GET /cron/backup-run/download | ✓ | ✗（AUTH_008，ADR-018 禁上传下载） | —（GET 只读） |

数据权限规则：无行级隔离——readonly 全可看含失败原因与产物路径；变更/触发/下载能力即角色边界（ADR-018）。敏感面：rsync target_config 无凭据字段（D-06）；cron 命令原文对两角色可见（crontab 本身可读，非面板敏感面）。

### 3.4 公共规范

**错误码**（复用 AUTH_001/002/003/008/012 会话与边界码族；本模块业务码 CB_001~009）：

| code | 标识 | HTTP 语义 | 说明 |
|------|------|----------|------|
| CB_001 | VALIDATION_FAILED | 参数错误 | data.fields[] 逐字段错误（R-14；名称/保留份数/路径/host 等） |
| CB_002 | CRON_EXPR_INVALID | 参数错误 | 表达式解析失败，data.errors[]（段位级错误） |
| CB_003 | CRON_JOB_NOT_FOUND | 拒绝 | 计划任务不存在（面板外已删——如实报错不静默） |
| CB_004 | BACKUP_JOB_NOT_FOUND | 拒绝 | 备份任务不存在 |
| CB_005 | BACKUP_JOB_NAME_TAKEN | 参数冲突 | 任务名称已存在（I-06） |
| CB_006 | BACKUP_JOB_RUNNING | 拒绝 | 同任务已有执行进行中（I-01 手动侧） |
| CB_007 | BACKUP_RUN_NOT_FOUND | 拒绝 | 执行记录不存在 |
| CB_008 | RUN_NOT_DOWNLOADABLE | 拒绝 | 非 success 状态（I-04，failure/running 不可下载） |
| CB_009 | ARTIFACT_MISSING | 拒绝 | 产物文件缺失（面板外被删——记录为历史事实不回改，PRD N2） |

**枚举**（均在 workspace/01-solution/04-business-enum.md 定义，D2 已定稿）：

| 枚举标识 | 取值 | 用途 |
|---------|------|------|
| cron_source | panel \| existing | 计划任务来源标注（D-02 判定） |
| backup_source_type | site_dir \| database | 源清单项判别 |
| backup_target_type | local \| rsync | 目标类型 |
| backup_trigger | cron \| manual | run 触发方式 |
| backup_run_status | running \| success \| failure | run 状态机（ADR-016） |
| audit_action / audit_result / audit_source | （复用 account-auth） | 审计写入 |

**业务常量**（审计动作码与档位，非枚举池条目）：动作码 `cron_job_created / cron_job_updated / cron_job_deleted / cron_job_triggered / backup_job_created / backup_job_updated / backup_job_deleted / backup_run_triggered / backup_artifact_downloaded / backup_rotated`（object_type=cron_job/backup_job/backup_run）；档位常量 `TRIGGER_TIMEOUT=120s`、`OUTPUT_MAX=64KB（尾部截断）`、`PREVIEW_COUNT=5`、`LOG_LINES 默认 200 档位 [50,100,200,500,1000]`、`SCHEDULER_TICK=1min`（均编译期常量，不入库不配置）。

## 4. 聚合根详细设计

### 4.1 backup_job（主数据类）

#### 4.1.1 领域模型

**表结构组合**：backup_job 单表（源清单/目标为 JSON 值列内嵌，domain-model v1.14 D2 判定）。

**字段清单**：

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | 聚合根标识 |
| name | TEXT | NOT NULL UNIQUE | 任务名（I-06；≤64 字符，R-14） |
| source_items | TEXT | NOT NULL | JSON 值集合：`[{type, path?}|{type, instanceId, database?}]`（backup_source_type 判别；database 缺省=整实例全库——mysqldump --databases 语义由执行侧定，MVP 允许库级缺省为全部库逐库导出） |
| target_type | TEXT | NOT NULL CHECK(in local,rsync) | backup_target_type |
| target_config | TEXT | NOT NULL | JSON：local→`{path}`（绝对路径，≠面板数据目录，D-06）；rsync→`{host, user, port?, path}`（无凭据字段，D-06） |
| retention_count | INTEGER | NOT NULL CHECK(1..100) | 保留份数（I-05 水位） |
| schedule_cron | TEXT | NOT NULL | 五段表达式（宏已归一化，I-07/D-03） |
| created_at / updated_at | DATETIME | NOT NULL | ISO-8601 UTC |

**不变量**（聚合内）：I-06 名称唯一；I-07 表达式合法；I-09 source_items 非空且每项类型合法（site_dir 项 path 为绝对路径 / database 项 instanceId 为正整数、database 为合法库名或缺省）；I-10 target_config 与 target_type 匹配（local 必 path、rsync 必 host+user+path）。

**领域服务**：CronExpr（校验+预览，D-03）；SourceValidator（I-09/I-10，R-14）；InstanceRefChecker（database 项的实例存在性检查——创建/编辑时对 db_instance 档案只读校验，悬空拒绝 CB_001；执行期悬空→run.failure 不级联）。

#### 4.1.2 状态模型

无状态机（配置单据；「最近执行结论」为 run 读时派生，D-11——domain-model v1.14 判定）。

#### 4.1.3 子流程设计

无（定时调度为进程内机制链路，BackupScheduler tick，D-13——非独立聚合子流程）。

#### 4.1.4 接口设计

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /cron/backup-job/list | GET | admin+readonly | 任务清单（含最近执行结论派生回显） |
| /cron/backup-job/create | POST | admin | 创建任务（四要素） |
| /cron/backup-job/update | POST | admin | 整体替换定义（值对象替换语义） |
| /cron/backup-job/delete | POST | admin | 删除任务（不级联 run/产物） |
| /cron/backup-job/run | POST | admin | 立即执行（异步返回 runId，D-07） |

**接口详情**：

- **GET /cron/backup-job/list**：无分页（任务个位数~十余）；data=`items[{id, name, sourceSummary[{type, ref}], targetType, targetBrief, retentionCount, scheduleCron, nextFireAt(调度器推算), lastRun{id, status, trigger, startedAt, finishedAt}|null}]`（lastRun 读时派生 D-11）；按 id 升序。
- **POST /cron/backup-job/create**：req `{name, sourceItems[], targetType, targetConfig, retentionCount, scheduleCron}`；校验：I-06（CB_005）/I-07（CB_002）/I-09/I-10（CB_001）/database 项实例存在（InstanceRefChecker，CB_001）；成功 data=`{id}`；审计 backup_job_created（change）。
- **POST /cron/backup-job/update**：req `{jobId, name, sourceItems[], targetType, targetConfig, retentionCount, scheduleCron}`；校验同 create（名称唯一排除自身）+ CB_004；编辑=整体替换（调度器下一 tick 按新定义）；审计 backup_job_updated（change）。
- **POST /cron/backup-job/delete**：req `{jobId}`；校验 CB_004；成功仅删 job 行（run/产物留存至自身轮转，FR-09）；运行中任务删除→执行继续完成、终态正常回写（run 不级联）；审计 backup_job_deleted（change）。
- **POST /cron/backup-job/run**：req `{jobId}`；校验 CB_004 → 互斥（I-01：存在该 job 的 running → CB_006）；成功创建 run(running, trigger=manual) 并立即返回 data=`{runId}`（异步执行，D-07）；审计 backup_run_triggered（change，手动触发类）。

**跨接口复用规则**：立即执行与调度触发共用 BackupExecutor.Execute（2.3 执行编排）；审计统一经 AuditService.Record。

#### 4.1.5 领域事件与集成

无 MQ（单体进程内）。仓储契约：`BackupJobRepository { List, GetByID, Create, Update, Delete }`。关联域调用（出站）：DbGateway.DumpDatabase（database-mgmt OHS，3.2）；SiteDocRoot 无调用（值快照经前端联动获取，1.2⑦）。

### 4.2 backup_run（业务流程类）

#### 4.2.1 领域模型

**表结构**：backup_run 单表（append+终态转移，无 UPDATE 终态路径——I-02）。

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | 聚合根标识（前端 runId） |
| job_id | INTEGER | NOT NULL（无 FK，D-12） | 归属任务（弱引用——任务删除后悬空为合法态） |
| job_name | TEXT | NOT NULL | 冗余快照（I-03；任务删除/改名后历史可读） |
| trigger | TEXT | NOT NULL CHECK(in cron,manual) | backup_trigger |
| status | TEXT | NOT NULL DEFAULT 'running' CHECK(in running,success,failure) | backup_run_status（ADR-016） |
| started_at | DATETIME | NOT NULL | — |
| finished_at | DATETIME | NULL | 终态时写；running 时 NULL |
| artifact_path | TEXT | NULL | 产物目录定位（local 绝对路径 / rsync 远端路径；success 必填 I-04） |
| artifact_size | INTEGER | NULL | 目录内文件总字节（success 时写） |
| error_message | TEXT | NULL | 失败原因（failure 时写；含失败源标识与超时/传输/写入错误摘要） |
| files | TEXT | NULL | JSON：manifest files 清单快照 `[{name,size}]`（success 时写——detail/download 的文件枚举依据） |

索引：idx_run_job(job_id, id DESC)、idx_run_status(status)、**idx_run_running 部分唯一**（job_id WHERE status='running'，I-01/D-08）。

**不变量**：I-01（部分唯一索引承载）；I-02（仅 running→success/failure 一次转移，无终态 UPDATE 路径）；I-03（job_name 快照）；I-04（success ⇔ artifact_path/files 非空）；I-05（轮转收敛，D-09）。

**领域服务**：BackupExecutor（执行编排，2.3）；RotationPolicy（保留集计算：最近 N 条 success 为保留集、删除 id<保留集最小 id 的全部记录，D-09）。

#### 4.2.2 状态模型

```
running ──执行完成/成功──> success（终态：artifact_path/size/files 写入）
   │
   └─任一源失败/超时/传输失败/面板重启中断──> failure（终态：error_message 写入）
```

- 触发角色：success/failure 由 BackupExecutor（系统）回写；manual 触发的创建由 admin 发起（POST run）、cron 触发由调度器（system）。
- 无驳回/撤回（无审批语义，PRD 第六部分）；终态不可逆（I-02），重新执行=新 run。
- 状态历史：run 表本身即历史（append+轮转，R-11）；无独立状态历史表。

#### 4.2.3 子流程设计

无独立聚合子流程（执行状态机为 run 聚合内本体；轮转为终态回写事务内的治理环节，D-09——domain-model v1.14 第四部分判定）。

#### 4.2.4 接口设计

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /cron/backup-run/list | GET | admin+readonly | 历史分页检索 |
| /cron/backup-run/detail | GET | admin+readonly | 单条完整信息 |
| /cron/backup-run/download | GET | admin | 产物单文件流式下载 |

**接口详情**：

- **GET /cron/backup-run/list**：query `{jobId?, status?(backup_run_status), trigger?(backup_trigger), from?, to?, page, pageSize}`（公共分页；jobId 对已删任务仍有效——按弱引用归组，D-12）；data=`{items[{id, jobId, jobName, trigger, status, startedAt, finishedAt, durationMs(派生), artifactSize, errorBrief}], total}`；按 id 倒序。**关闭 prd 待对齐-8**。
- **GET /cron/backup-run/detail**：query `{runId}`；校验 CB_007；data=`{id, jobId, jobName, trigger, status, startedAt, finishedAt, durationMs, artifactPath, artifactSize, files[{name,size}], errorMessage, sources[](执行时源清单快照——来自 manifest 语义)}`；running 态 finishedAt 及以下产物字段为 null。
- **GET /cron/backup-run/download**：query `{runId, file}`；校验：CB_007 → status=success（CB_008）→ file ∈ run.files 清单（CB_009 语义之外的非法名按 CB_001）→ 文件存在（CB_009：面板外被删，如实报错）。响应：校验全过才开流（D-16 流式响应账）——200 + 文件流（Content-Disposition: attachment; filename=原名，D-10）；rsync 目标经 backupx 拉回流。审计 backup_artifact_downloaded（**security**，数据取出口径对齐 file-manager file_downloaded 先例）。

#### 4.2.5 领域事件与集成

无 MQ。仓储契约：`BackupRunRepository { Create, Finish(tx: id→终态+产物字段), ListByJob, Search(cond,page), ListRunning, MarkOrphanFailed(启动恢复) }`。关联域调用：经 job 间接（DbGateway.DumpDatabase 于执行编排内，2.3）。

### 4.3 cron_job（通道投影型——无聚合根）

#### 4.3.1 领域模型

**表结构**：无表（crontab 实况投影，零任务台账，D2 口径-1——domain-model v1.14 判定；cronx 适配器承载读写）。

**运行时字段清单**（读取时由 CronTabGateway.List 投影组装）：

| 字段 | 来源 | 说明 |
|------|------|------|
| jobKey | 任务行内容规范串（D-01） | 寻址引用（不透明透传） |
| expr / command | crontab 行解析 | 五段表达式+命令原文 |
| remark | `# dugong: remark:` 注释行（D-02） | 无则 null |
| source | cron_source：标记注释存在⇔panel，缺失⇔existing（D-02） | 来源标注 |
| systemEntries | /etc/cron.d/* 文件（任务行粒度）+ hourly/daily/weekly/monthly 目录（脚本文件名清单） | 只读分区，与 root 任务分列（D-02 关闭待对齐-2：文件粒度呈现） |

**不变量**：I-08（手动触发命令原文不经请求）；写操作仅限 root crontab 域（系统预置位只读，D1 口径-1）；标记注释邻接维护（D-02）。

#### 4.3.2 状态模型

无状态机（任务存在即状态；source 为标记判定值——domain-model v1.14 辨析）。

#### 4.3.3 子流程设计

无。

#### 4.3.4 接口设计

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /cron/job/list | GET | admin+readonly | root 任务投影 + 系统预置位只读分区 |
| /cron/job/create | POST | admin | 写入 root crontab（含标记注释） |
| /cron/job/update | POST | admin | 删旧行+写新行（jobKey 随内容变更） |
| /cron/job/delete | POST | admin | 删除任务行+其标记注释 |
| /cron/job/validate | POST | admin | 表达式校验+未来 5 次预览（D-03） |
| /cron/job/trigger | POST | admin | 面板代执行（同步 120s，D-05） |
| /cron/log/tail | GET | admin+readonly | cron 执行日志尾部（纯文本，D-04） |

**接口详情**：

- **GET /cron/job/list**：无参数（crontab 行序如实呈现，D-02）；data=`{items[{jobKey, expr, command, remark, source}], systemEntries:{cronDFiles[{file, entries[{expr, command}]}], cronDirs[{dir, scripts[]}]} }`；readonly 同视角（无操作入口为前端约定，D4）。
- **POST /cron/job/create**：req `{expr, command, remark?}`；校验：expr 解析（CB_002）/command 非空（CB_001）/宏归一化（D-03）；写入=标记注释两行+任务行（D-02）；data=`{jobKey}`；审计 cron_job_created（change，detail 含 expr+command）。
- **POST /cron/job/update**：req `{jobKey, expr, command, remark?}`；校验同 create + 目标存在（CB_003——面板外已删如实报错）；执行=删旧行（含旧注释）+写新行；data=`{jobKey(新)}`；审计 cron_job_updated。
- **POST /cron/job/delete**：req `{jobKey}`；校验 CB_003；删除任务行+紧邻标记注释行；审计 cron_job_deleted。
- **POST /cron/job/validate**：req `{expr}`；纯计算无副作用；成功 data=`{nextFires[5]}`（ISO-8601）；失败 CB_002（data.errors[] 段位级）。机制型接口：创建/编辑表单内嵌消费（FL 附录A）。
- **POST /cron/job/trigger**：req `{jobKey}`；校验 CB_003 → JobRunner 代执行（120s 超时/64KB 尾部截断，D-05；I-08：命令原文取自 crontab）；data=`{exitCode|null(未正常退出), output, timedOut, durationMs}`——执行失败/超时为业务结果非请求错误；审计 cron_job_triggered（change，result=exitCode==0&&!timedOut ? success : failure，detail 含 timedOut 与输出摘要）。
- **GET /cron/log/tail**：query `{lines?}`（档位 [50,100,200,500,1000]，默认 200，越档归 200）；响应 200+纯文本（text/plain；尾部片段）；来源适配归 cronx（D-04）。

#### 4.3.5 领域事件与集成

仓储契约：无（零台账）；CronTabGateway 即读写通道（3.2）。关联域调用：无出站。

## 5. 模块清单汇总

### 5.1 对外服务清单（HTTP 接口 + 进程内契约）

| 服务（接口/契约） | 消费方 | 契约要点 |
|----------------|--------|---------|
| GET /cron/job/list · POST /cron/job/{create,update,delete,validate,trigger} | 前端计划任务页 | jobKey 寻址（D-01）；手动触发同步语义（D-05） |
| GET /cron/log/tail | 前端计划任务页 | 纯文本尾部（D-04） |
| GET /cron/backup-job/list · POST /cron/backup-job/{create,update,delete,run} | 前端备份任务页 | 立即执行异步返回 runId（D-07） |
| GET /cron/backup-run/{list,detail,download} | 前端备份历史页 | download 流式+两段校验（D-10/D-16） |
| 中间件 AuthRequired / CsrfGuard / RoleRequired | （复用） | account-auth 提供 |
| **DbGateway.DumpDatabase（消费登记）** | database-mgmt 提供 | 本模块正式登记消费（其 5.1 预告兑现）：数据库源导出流、凭据不出其域（3.2） |
| AuditService.Record（消费） | account-auth 提供 | R-03 审计写入（D-09 口径） |
| CronTabGateway / CronLogSource / JobRunner / BackupScheduler / BackupExecutor | 本模块内进程内契约 | cronx/backupx 适配器承载（3.2，R-08） |

本模块无对外 OHS 服务契约——服务消费终模块+被引用方（db_instance 引用经 database-mgmt 契约解析，1.5）。

### 5.2 SQL 清单

| # | SQL | 场景 |
|---|-----|------|
| 1 | INSERT INTO backup_job(...) / SELECT by id / by name / LIST | 任务 CRUD（I-06 唯一索引） |
| 2 | UPDATE backup_job SET 全量定义列 WHERE id=? | 整体替换（值对象替换语义） |
| 3 | DELETE FROM backup_job WHERE id=? | 任务删除（run 不级联，D-12） |
| 4 | INSERT INTO backup_run(job_id, job_name, trigger, status='running', started_at) | 执行创建（I-01 冲突→CB_006/调度跳过） |
| 5 | UPDATE backup_run SET status=?, finished_at=?, artifact_path=?, artifact_size=?, files=?, error_message=? WHERE id=? AND status='running' | 终态一次性转移（I-02：仅 running 可转移） |
| 6 | DELETE FROM backup_run WHERE job_id=? AND id < (SELECT MIN(id) FROM (SELECT id FROM backup_run WHERE job_id=? AND status='success' ORDER BY id DESC LIMIT ?)) | 轮转清理（D-09：保留最近 N 条 success，更早记录全删；与终态回写同事务） |
| 7 | SELECT 带筛选分页（job_id?/status?/trigger?/时间窗）ORDER BY id DESC | 历史检索 |
| 8 | SELECT id, status, trigger, started_at, finished_at FROM backup_run WHERE job_id=? ORDER BY id DESC LIMIT 1 | 最近执行结论读时派生（D-11，job/list 子查询） |
| 9 | UPDATE backup_run SET status='failure', finished_at=?, error_message='面板重启中断' WHERE status='running' | 启动恢复编排（D-08，进程启动时） |

### 5.3 DDL

```sql
CREATE TABLE IF NOT EXISTS backup_job (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  name            TEXT    NOT NULL UNIQUE,
  source_items    TEXT    NOT NULL,              -- JSON 值集合：[{type:'site_dir',path}|{type:'database',instanceId,database?}]
  target_type     TEXT    NOT NULL CHECK (target_type IN ('local','rsync')),
  target_config   TEXT    NOT NULL,              -- JSON：{path} | {host,user,port,path}（无凭据字段，D-06）
  retention_count INTEGER NOT NULL CHECK (retention_count BETWEEN 1 AND 100),
  schedule_cron   TEXT    NOT NULL,              -- 五段表达式（宏已归一化，D-03）
  created_at      DATETIME NOT NULL,
  updated_at      DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS backup_run (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  job_id        INTEGER NOT NULL,                -- 弱引用（无 FK，D-12：任务删除后历史留存）
  job_name      TEXT    NOT NULL,                -- 冗余快照（I-03）
  trigger       TEXT    NOT NULL CHECK (trigger IN ('cron','manual')),
  status        TEXT    NOT NULL DEFAULT 'running' CHECK (status IN ('running','success','failure')),
  started_at    DATETIME NOT NULL,
  finished_at   DATETIME,
  artifact_path TEXT,
  artifact_size INTEGER,
  error_message TEXT,
  files         TEXT                             -- JSON：[{name,size}]（success 时写，I-04）
);
CREATE INDEX IF NOT EXISTS idx_run_job    ON backup_run(job_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_run_status ON backup_run(status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_run_running ON backup_run(job_id) WHERE status='running';  -- I-01 执行互斥（D-08）
```

## 6. 更新记录

### 6.1 变更记录

| 版本 | 日期 | 变更类型 | 变更摘要 | 目标节号 |
|------|------|---------|---------|---------|
| v1.0 | 2026-11-14 | initial | 首次生成：15 个 HTTP 接口（/cron/{job,log,backup-job,backup-run}/*：GET 6——readonly 可用 4[list×2/run×2/tail]、admin 专属 GET 1[download]；POST 9=admin+CSRF）+ 2 表 DDL（backup_job/backup_run，含 I-01 部分唯一索引）+ 5 进程内契约（CronTabGateway/CronLogSource/JobRunner/BackupScheduler/BackupExecutor）+ 1 消费契约正式登记（DbGateway.DumpDatabase，database-mgmt 预告兑现）+ 9 错误码（CB_001~009，复用 AUTH 码族）+ 5 枚举引用（池中 D2 定稿）+ 10 审计动作码；D-01~D-14 设计决策逐项关闭 prd-cron-backup.md 待对齐-1~-9；无旧版对比 | — |

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
