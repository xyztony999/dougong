# 斗拱面板 环境管理（env-mgmt）API 设计

| 属性 | 值 |
|------|-----|
| 版本 | v1.2 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-17（v1.2 安装后配置就绪增量轮） |
| 输入 | prd-env-mgmt.md v1.2（完备性检查通过：G 5/5、T 适用 6/6、N 4/4；FR-07 安装后配置就绪）、domain-model.md v1.22（env_install_task 聚合根 +bootstrap_status/note 引导结果维度）、D1 三文档（BA/AA/FL v1.1，⚠️A1~A15）、d0-clarification-2026-11-17-bootstrap.md（拍板 Q1~Q4） |
| 接口规约 | 栈规约 backend-02-api-conventions.md（go-vue3）；认证=HttpOnly Cookie 会话 + CSRF token（R-02，复用 account-auth）；鉴权=RBAC 两角色边界矩阵（ADR-018）；外部命令一律字面量参数数组（R-08） |

## 1. 模块设计概述

### 1.1 业务背景及目标

环境管理是面板的运行环境供给模块（v0.1.2 用户反馈第 4 项，参照宝塔「软件商店」按本项目定位裁剪）：经发行版包管理器（apt / dnf·yum 双系探测择一，⚠️A2）安装、升级、卸载七类运行环境（nginx/Apache/Docker/MySQL/Redis/PHP-FPM/MongoDB，⚠️A1——v1.1 扩 MongoDB），提供已装环境版本/服务状态观测与服务启停自启入口（复用 service-mgmt SysdGateway 通道资产）。安装/卸载为异步任务（四态状态机 ⚠️A3，任务台账落 SQLite），升级=重装幂等（⚠️A4），卸载保守（⚠️A6）。与既有模块四层分界：本模块管软件包层；实例层归 database-mgmt、容器层归 docker-mgmt、通用单元层归 service-mgmt、站点层归 site-mgmt。服务对象：admin 执行全部变更；readonly 查看环境全景与任务状态输出（ADR-018）。

### 1.2 边界说明

| 项 | 说明 |
|----|------|
| 本模块做 | 环境全景（目录+实况+通道状态）、安装/升级/卸载（异步任务+官方仓库代配）、任务台账观测、已装环境服务启停自启 |
| 本模块不做（接口边界） | ① 源码编译安装/多版本切换/降级指定版本——OUT；② 任务取消——OUT（⚠️A3）；③ purge 与数据清理——二期；④ 可升级版本检测——二期（⚠️A4）；⑤ 跨模块动态数据计数（容器数/实例数/站点数）——OUT（无跨模块硬调用）；⑥ 运行环境内部配置——归 site-mgmt/database-mgmt/docker-mgmt/service-mgmt；⑦ 认证/鉴权/审计本体——复用 account-auth；⑧ 页面外壳与导航——归 panel-core |
| 与存储的关系 | SQLite 单文件（与全模块同库）；env_install_task 单表（5.3，本模块唯一持久化对象）；环境实况零台账（包管理器/systemd 实时探测投影） |

### 1.3 设计决策

| # | 决策 | 理由 |
|---|------|------|
| D-01 | **实时探测投影**：env/list 每请求即时探测七环境实况（已装/版本/服务状态）与通道可用性，无缓存层、无陈旧回退，响应带 generatedAt；**三态如实区分（v1.1 精确化，⚠️A9）**：通道级不可用（channel.available=false，全条目 installed/version=null + 页头横幅）/ 条目级探测失败（installed=null「探测不可用」角标——仅真错误：查询命令超时/通道异常；包名不在包数据库**不算**错误）/ 未装（installed=false）；服务状态仍可经 systemd 探测 | site-mgmt D-01「投影真实性」同口径；包管理器数据库与 systemd 为事实源；七条目探测成本可接受；W1 实机缺陷 2（未装误显「探测不可用」）修复口径 |
| D-02 | **编译期目录与映射表**（⚠️A1/A7）：env_type 七值目录（v1.1 六→七 +MongoDB；显示名/描述/关联模块提示）与每发行版族映射为编译期常量，非用户可配；**映射表结构 v1.1**：包名=有序候选数组（检测任一命中即已装；安装取首选〔发行版原生优先，MongoDB 例外=mongodb-org〕；卸载移除实际命中者）+ systemd 单元名=规范全名（.service 后缀，⚠️A8/D-13）+ php-fpm apt 系版本化单元（phpX.Y-fpm.service）经 systemd ListUnits 前缀扫描解析；目录扩展走面板版本迭代 | BA ⚠️A1/A7 拍板 + v1.1 修订（多候选覆盖官方源自装形态 docker-ce/mysql-community-server；Ubuntu php-fpm 版本化单元实况） |
| D-03 | **通道探测与双系择一**（⚠️A2）：exec.LookPath 探测 apt/dnf/yum（R-08，非 shell、非外呼）；择一规则=apt 存在→apt；否则 dnf→dnf；否则 yum→yum；皆无→通道不可用（ENV_001 前置拒绝变更接口，列表如实标注）；全部安装/升级/卸载命令经择一族执行 | Debian/RHEL 双系支持矩阵（install.sh 同源）；探测为二进制存在性检查零副作用 |
| D-04 | **异步任务与同环境互斥**（⚠️A3）：install/uninstall 同步校验（通道/envType/已装态/在途互斥 ENV_003）→ INSERT env_install_task（status=pending）与发起审计同事务 → 立即返回 taskId；worker goroutine（wg 编排，创建即触发非轮询调度）消费：running 落库 → 执行命令链 → 终态（succeeded/failed）落库 + 终态审计；互斥=创建事务内判定同 env_type 存在 pending/running 即拒（SQLite 单连接写串行保证原子）；任务取消不做（OUT-4）；**重启恢复**：启动扫描 running 任务标 failed（error=「面板重启中断」），worker 不续跑（包管理器中断态不安全） | prd FR-03/FR-05/N2；与 backup_run 执行互斥（idx 部分唯一索引）同构但互斥粒度为 env_type（一环境一任务） |
| D-05 | **升级=重装幂等**（⚠️A4）：install 对已装环境不拒绝——任务 action 按实况判定（未装=install / 已装=upgrade）；执行链统一「刷新包索引 → [仓库代配] → 安装仓库最新版」；重复升级终态一致。**v0.1.3a 实机补丁**：apt 系一律 `apt-get`（apt 无稳定脚本接口）+ `DEBIAN_FRONTEND=noninteractive` + `--force-confdef/--force-confold`（dpkg conffile 交互提示无 stdin 必败；本地已改配置保持现版、新版 .dpkg-dist 旁置） | ⚠️A4 拍板；「检测可升级版本并提示」二期 OUT（不主动检测，用户主动发起）；conffile 保守策略=无人值守绝不静默覆盖生产配置 |
| D-06 | **官方仓库代配**（⚠️A2/⚠️A10）：映射表 needsVendorRepo 标志的环境-族组合（v1.1 清单：MongoDB 三系〔mongodb-org 不在任何发行版官方源〕+ RHEL 系 docker-ce；apt 系 docker 装发行版原生 docker.io 不代配；MySQL 官方源不代配——Debian12 无 mysql-server 属已知限制如实失败呈现），任务执行链在**索引刷新之前**写入发行版仓库源配置（v1.1 次序前移：新源须参与本次 update/makecache）；实现（v1.1 落定，v1.0 为空转日志）：按 /etc/os-release 判定（ID/ID_LIKE + VERSION_CODENAME/VERSION_ID）目标发行版与代际，支持矩阵外组合任务 failed 如实（不写任何文件）；apt=写 /etc/apt/sources.list.d/mongodb-org-8.0.list（signed-by 引用面板 HTTPS 下载的官方公钥 .asc，经 Go net/http 落 /usr/share/keyrings/mongodb-server-8.0.asc，不依赖 curl/gpg 外部命令）；dnf/yum=写 /etc/yum.repos.d/{mongodb-org-8.0,docker-ce}.repo（gpgkey URL 内联，$releasever/$basearch 由包管理器展开）；文件写入经 os.WriteFile 字面量路径（R-08 非命令拼接；公钥内容校验 PGP armored 头防错页落盘）；写入动作独立审计 env_repo_configured（object_type=env，object_id=envType，detail 含路径与发行版判定结论）；写入/下载失败任务终态 failed（不进入安装步骤）；公钥下载与文件写入提供注入点（测试环境无关，CI 预防） | ⚠️A2 拍板（面板代配并审计留痕）+ v1.1 ⚠️A10 补真实现（MongoDB 8.0 轨道 ⚠️待主会话终确认）；R-08 文件写入非命令拼接。**v0.1.3b 实机补丁**：apt 单行源多选项以空格分隔（逗号把 signed-by 并入 arch 值被静默吞掉 → NO_PUBKEY；WSL noble 复现+验证），组件仅 multiverse/main 无附加 |
| D-07 | **卸载保守与风险提示两层**（⚠️A6，PRD FR-04 落地口径）：uninstall 前置已装校验（ENV_004）；执行包 remove（保留系统配置与数据目录，无 purge 选项）；强确认交互归前端（NModal 呈现：本域服务运行实况警示 + 关联模块静态影响文案），**服务端不硬阻断**（知情决策）；风险提示数据=env/list 实况（服务 running 态），无跨模块动态计数（OUT-9） | ⚠️A6 拍板；与 firewall FW_005 两段式的差异=无危险端口类硬阻断语义，知情决策口径 |
| D-08 | **任务输出尾部采集**：执行中 ring 缓冲采集 stdout/stderr 合并尾部 TASK_OUTPUT_TAIL_LINES=200 行；running 阶段每次追加即更新 output_tail 列（低频写入量级小，避免 running 中详情无输出可看），终态定格 | prd FR-05 排障呈现；行数上限防失控 |
| D-09 | **服务操作复用 SysdGateway**：start/stop/restart/enable/disable 经 service-mgmt 同一 systemd 网关实现（进程内适配器共享非模块调用）；单元名经映射表（⚠️A7）；操作等待终态语义与 service-mgmt 一致；操作对象限定目录环境单元（不开放任意单元名入参，R-14：请求仅 envType） | AA 附录B 契约；防任意单元操作越界面（service-mgmt 保持通用治理） |
| D-10 | **包操作最小权限与超时**：全部包命令字面量参数数组（R-08）；PKG_OP_TIMEOUT=600s（安装可达数分钟）单命令超时，超时终止进程组任务终态 failed（error=timeout）；命令执行不经 shell、无用户输入拼接（envType 经映射表转包名，无注入面） | prd 第五部分 R-08/最小权限拍板；timeout 按 failed 终态记不扩枚举（backup_run 超时同构） |
| D-11 | **任务台账全量保留**：不建清理任务（低频操作量级小；避免常驻清理 goroutine）；列表分页查询（公共分页约定） | N1 锁定项 |
| D-12 | 混合模式（MIXED）声明：全部对象实际为自管（无外部数据同步源；包管理器为本机通道非数据源），不设 manage_mode 字段，权限管控按角色区分（ADR-018） | 分册 Step 2 默认混合模式；本模块无托管侧 |
| D-13 | **systemd 单元名规范全名**（⚠️A8，v1.1）：目录映射单元名一律含 .service 后缀（nginx.service 等；mongod.service 随 MongoDB 入目）。根因：EnableUnitFiles/DisableUnitFiles 的 DBus 契约对单元名做 unit_name_to_type 校验、**无后缀补全**——无后缀名直接回 org.freedesktop.DBus.Error.InvalidArgs（W1 实机缺陷 1：enable nginx 报错逐字吻合；StartUnit/StopUnit/RestartUnit/LoadUnit 有服务端 .service 补全故 start 正常仅 enable/disable 报错）；SysdGateway 编组（as+bb / as+b）本身正确零改动 | W1 实机缺陷 1 根因定位（systemd dbus-manager.c 行为）；统一全名消除两类方法不对称 |
| D-14 | **已装查询未装判定口径**（⚠️A9，v1.1）：apt 分支改 `dpkg-query -W -f=${Status} ${Version}`——以 Status 数据字段判定（install ok installed / hold ok installed=已装；deinstall ok config-files / unknown ok not-installed=未装），不依赖本地化消息文本；命令 Env 注入 LC_ALL=C（沿 DEBIAN_FRONTEND 注入先例，输出确定化）；退出码非 0 且输出含 "no packages found matching" = **未装**（包名完全不在 dpkg 数据库——W1 缺陷 2 根因：该形态被误归通道错误）；rpm 分支退出码 1 且输出含 "not installed" = 未装；其余非 0 = 条目级探测错误（installed=null，D-01 三态）；候选包名数组由 service 层逐候选调用本查询编排（任一命中即已装，全部未装=未装，任一真错误且无命中=探测不可用） | W1 实机缺陷 2 根因（dpkg-query no-packages-found 分支未覆盖 + locale 影响 + 单候选映射）；数据字段判定优先于消息文本匹配 |
| D-15 | **安装后配置就绪触发与归属**（⚠️A11，v1.2）：worker 执行链在 Install/Upgrade 包操作**成功后**追加 post-install bootstrap 独立步骤（卸载与非数据库环境无此步骤）；仅 env_type ∈ {mysql,redis,mongodb} 触发。职责切分：**env-mgmt** 负责服务就绪（经 SysdGateway 幂等 start 目录单元，D-19）+ 消费 OHS + 引导结果回写任务台账；**database-mgmt** 负责建账户/设密码/注册实例与凭据（经进程内 OHS `BootstrapAfterInstall(ctx, engine, actorName) → BootstrapOutcome{Status, Note, InstanceID}`，单向依赖 env-mgmt→db-mgmt，与 cron-backup 消费 DumpStream 同构，契约见 api-database-mgmt.md §5.1）。引导失败**不改变包安装终态**（D-18） | D0 拍板（装完即配置就绪）；实例与凭据生命周期归 db-mgmt 实例层（四层分界），env-mgmt 止步触发与呈现 |
| D-16 | **引导通道与账户**（⚠️A12，v1.2，D0 拍板 Q1/Q2/Q4）：引导通道区别于 TCP+密码验证通道——**MySQL** 经 unix socket 以 root auth_socket 免密建连（面板 root 运行，dbmysql 网关 `--socket` + `-u root` + 空 MYSQL_PWD；socket 取 DISCOVERY_PROBE_SET 候选逐一尝试），CREATE USER **dougong@localhost** IDENTIFIED BY 随机强密码 + **GRANT ALL PRIVILEGES ON *.* WITH GRANT OPTION**（不动 root auth_socket）；**Redis** redis-cli 连 localhost:6379（初始免认证）CONFIG GET requirepass 判未设→CONFIG SET requirepass 随机密码 + CONFIG REWRITE 持久化（新密码认证）+ CONFIG GET bind 校验本机；**MongoDB** mongo-driver 免认证连 localhost:27017（localhost exception）admin 库建 dougong root 角色用户随机密码。随机强密码 crypto/rand（BOOTSTRAP_PASSWORD_BYTES=24 → base64url 32 字符）；注册经治理通道 TCP Ping 验证新凭据→AES-256-GCM 密文 UPSERT db_instance（db-mgmt CredentialVault/db-cred.key）；**密码永不回显**（审计 detail/任务输出/响应/BootstrapOutcome.Note 均不含密码） | D0 拍板 Q1（dougong@localhost）/Q2（ALL WITH GRANT OPTION——支撑 db-mgmt 三档授权 all 档）/Q4（CONFIG SET+REWRITE）；MySQL host 依赖默认名字解析（skip-name-resolve=OFF），开启后 TCP 落空如实呈现引导失败 |
| D-17 | **引导幂等保守**（⚠️A13，v1.2）：db-mgmt BootstrapAfterInstall 前置——目标本机默认 identity 已配置凭据（credentialsSet）→ skipped；引擎侧已存在配置（MySQL dougong 账户已存在 / Redis requirepass 已设 / MongoDB 已有用户或已开鉴权）→ skipped（不覆盖用户已有配置、不重置密码，conffile 保守先例）；skipped 携带 Note 说明。已装/已配置环境重复安装/升级不重复引导（重跑终态一致）。引导动作审计 env_bootstrap（env-mgmt 侧）+ instance_bootstrapped（db-mgmt 侧，脱敏） | D0 拍板；用户已有配置不覆盖；与 D-05 升级幂等同哲学 |
| D-18 | **引导失败独立维度**（⚠️A14，v1.2，D0 拍板 Q3）：env_install_task **+bootstrap_status TEXT NULL CHECK(in succeeded,failed,skipped) + bootstrap_note TEXT NULL**（NULL=无引导步骤）；任务 status 仍为包操作四态结果——安装成功但引导失败 → status=succeeded + bootstrap_status=failed + note 原因；引导结果与任务终态**同事务定格**（Finish 扩展，SQL #7/#10）；task/list·detail 响应 +bootstrapStatus/bootstrapNote（interface_changed，无新接口）；前端任务详情引导结果横幅（succeeded=已就绪可到数据库管理治理 / failed=已安装·引导失败+原因+「重新安装/升级以重试引导」/ skipped=保守跳过说明）；恢复路径=重新安装/升级（幂等重跑引导，无新重试接口） | D0 拍板 Q3；不混淆「已装」与「可治理」；引导结果非新状态机（domain-model v1.22 辨析） |
| D-19 | **引导服务就绪与超时**（⚠️A15，v1.2）：bootstrap 前 env-mgmt 经 SysdGateway 幂等 start 目录 DB 单元（start 对已运行无副作用；enable 不强制，沿 ⚠️A5）；db-mgmt 引导建连有界重试等待引擎就绪（**BOOTSTRAP_READY_TIMEOUT=60s**，间隔 2s 重试，超时→bootstrap_status=failed note=引擎未就绪）；引导各步受 ctx 超时约束（包操作链路上界外另计，单引擎引导 ≤BOOTSTRAP_READY_TIMEOUT+建连/命令超时）；**全注入化**——MySQL socket 建连/Redis CONFIG/MongoDB 建用户/凭据注册均经网关接口注入替身，零 runner 实况引擎依赖（CI 环境耦合第 7 起预防；引擎实联归 Linux 冒烟） | D0 拍板（⚠️A5 细化）；MySQL 首装初始化数据目录可达数十秒，有界重试承载；CI 预防沿 v0.1.2 教训 |

### 1.4 领域对象总览

| 聚合根 | 域内对象/值对象 | 子流程 | 关联领域对象 |
|--------|---------------|--------|-------------|
| env_install_task（主数据类子形态：四态任务状态机，非流程流转） | 环境条目（实体，目录常量+实况探测合并投影不落库）、发行版映射（值对象，编译期内嵌目录条目） | 无（任务状态机为聚合根本体，与 backup_run 同构） | 无（软引导被消费方向；SysdGateway 为共享适配层非业务域） |

无领域事件与 MQ（单机单体；审计为进程内 OHS 出站调用）。

**功能类型判定**（分册 Step 1）：聚合根 env_install_task 存在、由本模块管理且有写操作（任务创建）、状态为**数据生命周期**（四态任务状态机 ⚠️A3，非审批流转）→ **主数据类子形态**；模块整体为「单据型聚合根（env_install_task）+ 投影实体（环境条目）+ 工具类服务动作集合（启停自启）」混合形态。据此裁剪：第 2 章不适用（单聚合根）、4.2 按 env_task_status 状态机填写、第 5 章必填（有 DDL）。

### 1.5 上下文映射

| 上下文 | 关系 | 机制 |
|--------|------|------|
| account-auth | 遵奉者 + OHS 消费 | 全部接口位于 Cookie 会话后（R-02，AuthRequired）；环境列表/任务两角色同等（ADR-018）、全部变更 admin 专属（RoleRequired + AUTH_008）；写接口 CSRF（CsrfGuard）；审计写入 AuditService.Record（R-03，发起/终态/仓库写入/服务操作） |
| panel-core | 消费其 HTTP 骨架 | `/env` 路由组挂载；外壳与菜单归 panel-core（本轮 B 批增量同步登记菜单「环境管理」） |
| service-mgmt | 共享适配层 | SysdGateway systemd 网关实现共享（进程内适配器，非模块间调用）；本模块服务操作限定目录环境单元 |
| docker-mgmt / database-mgmt / site-mgmt | 被消费（软引导） | 关联模块未装引导态呈现「到环境管理安装」指引链接（PageSpec 层，无跨模块 API 硬依赖） |
| database-mgmt（v1.2 安装后配置就绪） | **OHS 消费（进程内，单向硬依赖）** | 数据库类环境安装成功后 worker 消费 `BootstrapAfterInstall(ctx, engine, actorName) → BootstrapOutcome{Status, Note, InstanceID}`（建账户/设密码/注册实例与凭据归 db-mgmt 实例层，D-15/D-16）；引导结果回写任务台账（bootstrap_status/note）；契约见 api-database-mgmt.md §5.1；与 cron-backup 消费 DumpStream 同构（env-mgmt→db-mgmt 单向，无环） |
| apt/dnf/yum、systemd（本机系统组件） | 数据源与动作通道 | envpack 适配器（R-08 参数数组）承载包操作与通道探测；SysdGateway 承载服务操作；本机资源非外部业务系统对接，无 ACL 必要 |

> 跨聚合根设计（第 2 章）：不适用——本模块单聚合根（env_install_task）；环境条目为无守门投影实体（domain-model.md 第一部分判定）。

## 3. 应用/基础设施层设计

### 3.1 接口规范

| 项 | 规范 |
|----|------|
| 路径格式 | `/env/{resource}/{action}` 三段式；资源单数（env/task/service） |
| HTTP 方法 | 读=GET，写=POST（面板内部契约） |
| 响应格式 | `{"code": 0, "message": "ok", "data": {...}}`（全项目同一契约） |
| 时间格式 | ISO-8601 UTC（公共约定） |
| 认证 | 复用 account-auth：HttpOnly Cookie `dg_session`（AuthRequired） |
| CSRF | 全部 POST 动作须带 `X-CSRF-Token` 头（R-02）；GET 豁免 |
| 中间件链 | AuthRequired → CsrfGuard（POST）→ RoleRequired（按接口，3.3 矩阵）；通道可用性/互斥校验为服务层编排（4.1/4.2），非中间件 |
| 用户类型标注 | 查看接口（env/list、task/list、task/detail）：admin+readonly；全部写接口（install/uninstall/service/*）：admin |

### 3.2 依赖与约束

| 依赖 | 类型 | 约束 |
|------|------|------|
| account-auth 中间件 | 框架 | AuthRequired/CsrfGuard/RoleRequired；不得自建会话解析 |
| account-auth AuditService.Record | OHS | 操作审计（R-03）；任务创建/终态与审计同事务（SQLite 侧）；服务操作同事务 |
| envpack 适配器 + 包管理器二进制 | 动作通道 | 通道探测/索引刷新/装卸/版本查询（R-08 参数数组，D-03/D-10 超时）；通道错误归一 ENV_001/任务 failed 语义 |
| service-mgmt SysdGateway | 共享适配层 | 服务启停自启与状态查询（D-09）；不复制 systemd 通道实现 |
| SQLite | 存储 | env_install_task 单表（5.3）；单连接写串行承载互斥原子性（D-04） |
| panel-core | 框架 | 路由组挂载 `/env`；菜单登记「环境管理」 |

### 3.3 权限模型

角色：admin / readonly（panel_role）；无访客可达面。

**接口权限矩阵**：

| 接口 | admin | readonly | CSRF |
|------|-------|----------|------|
| GET /env/env/list | ✓ | ✓（ADR-018 全可看） | —（GET） |
| POST /env/env/install | ✓ | ✗（ADR-018 禁变更） | ✓ |
| POST /env/env/uninstall | ✓ | ✗（同上） | ✓ |
| GET /env/task/list | ✓ | ✓（ADR-018） | —（GET） |
| GET /env/task/detail | ✓ | ✓（ADR-018） | —（GET） |
| POST /env/service/start / stop / restart | ✓ | ✗（ADR-018 禁变更） | ✓ |
| POST /env/service/enable / disable | ✓ | ✗（同上） | ✓ |

数据权限规则：无行级隔离——环境全景与任务台账两角色可见集完全一致；角色差异仅体现在写接口。readonly 越界由 RoleRequired 拦截返回 AUTH_008（附边界原因）。

### 3.4 公共规范

**错误码**（ENV_ 前缀本模块新增 6 个；鉴权/CSRF/越界复用 account-auth 码族）：

| code | 标识 | HTTP 语义 | 说明 |
|------|------|----------|------|
| 0 | OK | 成功 | — |
| ENV_001 | CHANNEL_UNAVAILABLE | 拒绝 | 包管理器通道不可用（非 Linux / apt 与 dnf·yum 皆未检出，D-03）；data.reason；变更接口前置拒绝，列表不报错（channel.available=false 呈现） |
| ENV_002 | INVALID_ENV_TYPE | 参数错误 | envType 不在 env_type 七值内（R-14；v1.1 七值） |
| ENV_003 | TASK_IN_FLIGHT | 参数冲突 | 同环境在途任务互斥（D-04，data.taskId 指向在途任务） |
| ENV_004 | NOT_INSTALLED | 拒绝 | 环境未安装（uninstall/service 前置；data.installed=false 实况） |
| ENV_005 | TASK_NOT_FOUND | 拒绝 | 任务不存在（台账实况） |
| ENV_006 | UNIT_OP_FAILED | 操作失败 | systemd 服务操作失败（D-09）；data.reason（网关错误详情） |
| AUTH_001/002/003/008 | （复用） | — | 未登录/过期/CSRF 失败/readonly 越界（account-auth 归口） |

**枚举**（本模块 D2 定稿入池 4 项，本次引用）：

| 枚举标识 | 取值 | 用途 |
|---------|------|------|
| env_type | nginx \| apache \| docker \| mysql \| redis \| php_fpm \| mongodb | 环境标识（全部接口 envType 参数/响应；目录键；v1.1 增 mongodb，七值） |
| env_task_action | install \| upgrade \| uninstall | 任务动作（task/list·detail 响应；D-05 实况判定） |
| env_task_status | pending \| running \| succeeded \| failed | 任务状态（task/list·detail 响应与筛选；D-04 四态机） |
| package_manager_family | apt \| dnf \| yum | 通道族（env/list channel.family；探测结论） |
| bootstrap_status | succeeded \| failed \| skipped | 引导结果（v1.2，task/list·detail bootstrapStatus；NULL=无引导步骤非枚举值；D-18；与 env_task_status 独立） |
| panel_role / audit_action / audit_result | （引用） | 权限与审计（account-auth 定稿） |

**业务常量**（审计细粒度动作码，非枚举池条目）：`env_install_started / env_upgrade_started / env_uninstall_started / env_task_finished（终态，detail 含 action/status/error 摘要）/ env_repo_configured / env_bootstrap（v1.2 引导动作，detail 含 envType/bootstrapStatus，**不含密码**）/ env_service_started / env_service_stopped / env_service_restarted / env_service_enabled / env_service_disabled`（category 均为 change；object_type=`env_install_task`/`env`，object_id=taskId 或 envType）。db-mgmt 侧引导注册审计 `instance_bootstrapped`（脱敏，见 api-database-mgmt.md）。

**业务常量**（操作参数与阈值，非枚举池条目）：

| 常量 | 值 | 说明 |
|------|-----|------|
| PKG_OP_TIMEOUT | 600s | 单包操作命令超时上限（D-10；超时任务终态 failed error=timeout） |
| TASK_OUTPUT_TAIL_LINES | 200 | 任务输出尾部采集行数（D-08；终态定格 output_tail） |
| TASK_LIST_PAGE_SIZE | 50 | 任务列表默认分页（公共分页约定内取值） |
| MONGODB_ORG_TRACK | 8.0 | MongoDB 仓库源版本轨道（⚠️A10；repo.mongodb.org baseurl/list 路径段；待主会话终确认 ⚠️） |
| VENDOR_KEYRING_DIR | /usr/share/keyrings | apt 系第三方公钥落位目录（D-06；signed-by 引用） |
| VENDOR_APT_LIST_DIR | /etc/apt/sources.list.d | apt 系第三方源文件目录（D-06） |
| VENDOR_YUM_REPO_DIR | /etc/yum.repos.d | dnf/yum 系第三方源文件目录（D-06） |
| QUERY_TIMEOUT | 30s | 已装查询命令超时（D-14；dpkg-query/rpm） |
| BOOTSTRAP_READY_TIMEOUT | 60s | 引导等待引擎就绪上限（D-19；间隔 2s 有界重试，超时 bootstrap_status=failed） |
| BOOTSTRAP_PASSWORD_BYTES | 24 | 引导随机强密码随机字节数（D-16；crypto/rand → base64url 32 字符，不弱默认） |
| BOOTSTRAP_ENV_TYPES | mysql,redis,mongodb | 触发安装后配置就绪的环境集合（D-15；编译期常量，非数据库环境无引导） |

## 4. 聚合根与领域对象详细设计

### 4.1 env_install_task（聚合根，主数据类子形态）

#### 4.1.1 领域模型

**表结构组合**：env_install_task 单表（本模块唯一持久化对象；环境实况零台账投影不落库，D-01）。

**字段清单**：

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | 聚合根标识（taskId） |
| env_type | TEXT | NOT NULL CHECK(in 七值，v1.1) | 目标环境（env_type） |
| action | TEXT | NOT NULL CHECK(in install,upgrade,uninstall) | env_task_action（D-05 实况判定落库） |
| status | TEXT | NOT NULL DEFAULT 'pending' CHECK(in pending,running,succeeded,failed) | env_task_status（四态机 D-04） |
| output_tail | TEXT | NOT NULL DEFAULT '' | 命令输出尾部（D-08 ring 采集，running 追加更新/终态定格） |
| error | TEXT | NULL | 失败原因（failed 终态；含 timeout/面板重启中断） |
| requested_by | TEXT | NOT NULL | 发起者用户名（admin） |
| created_at / updated_at | DATETIME | NOT NULL | 发起时间 / 状态更新时间 |
| started_at / finished_at | DATETIME | NULL | 执行开始 / 终态时间 |
| bootstrap_status | TEXT | NULL CHECK(in succeeded,failed,skipped) | 引导结果（v1.2，D-18；NULL=无引导步骤〔非数据库环境/卸载/包操作失败未达引导〕；终态定格） |
| bootstrap_note | TEXT | NULL | 引导摘要/原因（v1.2；不含密码；终态定格） |

**不变量**（聚合内）：

| 编号 | 不变量 | 违反处理 |
|------|--------|---------|
| I-01 | 同环境在途唯一：同 env_type 至多一个 pending/running 任务 | 创建事务内判定（ENV_003，data.taskId）；SQLite 单写者串行保证原子 |
| I-02 | 终态不可逆：succeeded/failed 不再迁移；重新发起=新任务行 | 写侧编排（D-04） |
| I-03 | 任务创建与发起审计同事务；终态落库与终态审计同事务 | D-12 同构（均 SQLite 侧） |
| I-04 | 重启恢复：启动扫描 running 任务标 failed（error=「面板重启中断」），不续跑 | main 装配启动环节（D-04） |
| I-05 | 输出尾部上限：output_tail 恒为最近 ≤TASK_OUTPUT_TAIL_LINES 行 | 采集侧保证（D-08） |
| I-09 | 引导结果与任务终态同事务定格（v1.2）：bootstrap_status/note 随 Finish 同事务落库；引导失败不改任务 status（包操作结果独立，D-18）；bootstrap_note/审计/输出不含密码 | D-18 同事务（Finish 扩展，SQL #7/#10） |

**领域服务**：EnvCatalog（编译期目录+映射，D-02）、ChannelDetector（D-03）、TaskWorker（D-04 执行链：索引刷新→仓库代配→装卸→**安装后配置就绪引导〔D-15~D-19，数据库类 install/upgrade 成功后〕**→终态+审计）、EnvProber（实况探测编排：EnvPackGateway 版本查询+SysdGateway 单元状态）、ServiceOps（D-09 启停自启编排+审计）、**BootstrapTrigger（v1.2：服务就绪〔SysdGateway 幂等 start〕+ 消费 db-mgmt BootstrapAfterInstall OHS + 引导结果回写，D-15/D-19）**。

#### 4.1.2 状态模型

env_task_status 四态状态机（⚠️A3）：

| 当前状态 | 目标状态 | 触发 | 接口 |
|---------|---------|------|------|
| pending | running | worker 取出任务开始执行 | install/uninstall 创建后异步 |
| running | succeeded | 命令链全部成功（退出码 0） | worker 终态回写 |
| running | failed | 命令失败 / 仓库写入失败 / 超时（PKG_OP_TIMEOUT） | worker 终态回写（error+输出定格） |
| running | failed | 面板重启恢复 | 启动扫描（I-04，error=面板重启中断） |

无审批/驳回环节（管理员即时发起异步执行）；状态变迁历史经任务台账本体（每任务一行全生命周期）+ 审计（started/finished 两段）。

#### 4.1.3 子流程设计

无（任务执行状态机为聚合根本体——与 backup_run 同构判定，domain-model.md 第四部分注记；worker 为后台机制链路非独立调度周期任务）。

#### 4.1.4 接口设计

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /env/env/list | GET | admin+readonly | 环境全景（目录+实况+通道状态；实时探测 D-01） |
| /env/env/install | POST | admin | 安装/升级（已装=升级幂等 D-05；异步任务 D-04；仓库代配 D-06） |
| /env/env/uninstall | POST | admin | 卸载（保守 remove D-07；已装前置；异步任务） |
| /env/task/list | GET | admin+readonly | 任务台账（envType/status 筛选，分页） |
| /env/task/detail | GET | admin+readonly | 单任务详情（输出尾部排障） |
| /env/service/start · stop · restart | POST | admin | 服务启停重启（SysdGateway D-09） |
| /env/service/enable · disable | POST | admin | 自启开关（同上） |

**接口详情**（全部写接口隐含 CSRF 与 AUTH_008 角色边界；install/uninstall 隐含通道可用性 ENV_001 前置与互斥 ENV_003，不再逐条重复）：

- **GET /env/env/list**：无参。处理：ChannelDetector 探测（D-03）→ EnvCatalog 七条目 × EnvProber 逐环境探测（已装/版本经 EnvPackGateway 多候选查询〔D-02/D-14〕，服务运行/自启经 SysdGateway 单元名映射〔⚠️A8 全名；php-fpm apt 版本化单元经 ListUnits 前缀扫描〕）→ 在途任务标记（I-01 查询 pending/running 归组）。响应 data=`{channel:{family: package_manager_family|null, available: boolean}, items:[{envType, displayName, description, installed: boolean|null, version: string|null, serviceRunning: boolean|null, serviceEnabled: boolean|null, unitName: string, runningTaskId: number|null}], generatedAt}`（三态 D-01：通道不可用=全条目 null+页头横幅；条目级真错误=该条目 null「探测不可用」角标；未装=installed=false 含包名不在包数据库形态；unitName 随映射表下发供呈现）。审计：—（查看类）。
- **POST /env/env/install**：req `{envType}`。校验：envType ∈ env_type（ENV_002）；通道可用（ENV_001，data.reason）；I-01 互斥（ENV_003，data.taskId）。执行：action 按实况判定（未装=install/已装=upgrade〔多候选任一命中即已装〕，D-05）→ INSERT 任务（pending）+ 发起审计同事务（I-03）→ worker 异步执行（D-06 仓库代配〔needsVendorRepo 时**先于索引刷新**，env_repo_configured 审计；发行版不支持组合任务 failed〕→索引刷新→安装〔候选首选〕→**安装后配置就绪引导〔v1.2，仅 env_type∈BOOTSTRAP_ENV_TYPES 且包操作成功：服务就绪（SysdGateway 幂等 start，D-19）→消费 db-mgmt BootstrapAfterInstall OHS（建账户/设密码/注册实例凭据，D-15/D-16）→引导结果 bootstrap_status/note，幂等保守 D-17；引导失败不改包安装 succeeded，D-18〕**→终态落库+审计〔bootstrap_status/note 同事务定格，I-09〕）。响应 data=`{taskId, action: env_task_action}`（引导异步执行，结果经 task/detail 呈现）。审计：env_install_started / env_upgrade_started（change，detail={envType, action}）+ env_repo_configured（代配时）+ **env_bootstrap（引导时，detail={envType, bootstrapStatus}，不含密码）** + env_task_finished（终态）。
- **POST /env/env/uninstall**：req `{envType}`。校验：envType（ENV_002）；通道（ENV_001）；已装（ENV_004，data.installed）；I-01 互斥（ENV_003）。执行：INSERT 任务（action=uninstall，pending）+审计 → worker 执行 remove（保留配置与数据，D-07；卸载目标=候选数组中实际命中〔已装〕的包）。响应 data=`{taskId, action:"uninstall"}`。审计：env_uninstall_started + env_task_finished。
- **GET /env/task/list**：query `{envType?(env_type), status?(env_task_status), page?, pageSize?(默认 50)}`。响应 data=`{items:[{id, envType, action, status, requestedBy, createdAt, startedAt?, finishedAt?, error?, bootstrapStatus?: bootstrap_status|null, bootstrapNote?: string|null}], total}`（created_at 倒序；**v1.2 +bootstrapStatus/bootstrapNote**——NULL=无引导步骤，前端列表引导角标数据源，D-18）。审计：—（查看类）。
- **GET /env/task/detail**：query `{taskId}`。校验：任务存在（ENV_005）。响应 data=`{...list 项全字段（含 bootstrapStatus/bootstrapNote）, outputTail}`（outputTail 含 [bootstrap] 行；前端详情引导结果横幅数据源，D-18）。审计：—（查看类）。
- **POST /env/service/{start|stop|restart|enable|disable}**：req `{envType}`（路径段承载服务动作）。校验：envType（ENV_002）；已装（ENV_004——服务操作对象为已装环境单元）；单元存在（单元不存在经网关错误 ENV_006）。执行：SysdGateway 按映射单元名操作（D-09，等待终态语义同 service-mgmt）→ 同事务审计。响应 data=`{envType, unit: unitName, result: "ok"|失败 ENV_006 data.reason}`。审计：env_service_started/stopped/restarted/enabled/disabled（change，detail={envType, unit}）。

**跨接口复用规则**：实况探测统一 EnvProber（list/安装 action 判定/卸载前置共用）；任务执行统一 TaskWorker（install/uninstall 共用命令链骨架）；服务操作统一 ServiceOps；审计统一 AuditService.Record（I-03 同事务）。

**幂等语义**：install 重复提交经 I-01 互斥拒绝（在途时 ENV_003 如实；终态后重复 install=新升级任务，幂等终态）；uninstall 未装报 ENV_004（如实）；服务操作对已处目标态幂等（systemd 语义，start 对已运行无副作用）；task/detail 重复读无副作用。防重复提交为前端在途态（P3 过程数据）。

#### 4.1.5 领域事件与集成

无 MQ、无领域事件。仓储契约：`EnvTaskRepository { Create(tx), GetByID, List(筛选+分页), UpdateStatus(tx)(running/终态), AppendOutputTail, Finish(tx)(status/error/bootstrap_status/bootstrap_note 同事务，v1.2 扩展), FailRunningAll(tx)(重启恢复) }`。**跨模块 OHS 消费契约（v1.2，进程内，单向 env-mgmt→db-mgmt）**：`BootstrapGateway { BootstrapAfterInstall(ctx, engine string, actorName string) (BootstrapOutcome, error) }`——db-mgmt Service 实现，main 装配注入；`BootstrapOutcome { Status string(succeeded|failed|skipped), Note string(摘要/原因，不含密码), InstanceID *int64(注册成功时) }`；契约权威定义见 api-database-mgmt.md §5.1。本机资源侧网关契约（进程内适配层，非 SQL）：

```
EnvPackGateway {
  DetectChannel() (family string, available bool)          // exec.LookPath apt/dnf/yum（D-03，非外呼）
  QueryInstalled(family, pkgName) (version string, installed bool, err)  // D-14：dpkg Status 字段判定 + LC_ALL=C；"no packages found matching"/rpm not installed=未装非错误；候选数组编排归 service 层
  RefreshIndex(family, timeout) (outputTail string, err)   // apt update / dnf makecache（D-05）
  EnsureVendorRepo(envType, family) (summary string, err)  // 仓库源代配（D-06 v1.1 实现：os-release 判定 + apt keyring 下载/yum repo 写入；summary 进任务输出；探测/文件写入/HTTP 可注入测试）
  Install(family, pkgName, timeout) (outputTail, err)      // 参数数组（D-10）
  Remove(family, pkgName, timeout) (outputTail, err)
}
SysdGateway（复用 service-mgmt 适配器，D-09）{ UnitState(unit) / Start / Stop / Restart / Enable / Disable / ListUnits }
约束：全部命令字面量参数数组（R-08）；超时 PKG_OP_TIMEOUT；TaskWorker 组合 EnsureVendorRepo→RefreshIndex→Install/Remove 完成命令链（D-04/D-05/D-06 v1.1 次序）；单元名一律规范全名（⚠️A8/D-13）
```

### 4.2 环境条目（域内对象-实体，目录+实况投影链路）

#### 4.2.1 领域模型

**表结构组合**：无表（零环境台账 D2 判定——包管理器数据库与 systemd 为事实源，实况每次请求实时探测 D-01；目录条目为编译期常量 D-02）。

**投影条目字段清单**（env/list 响应数据源）：见 4.1.4 env/list 响应结构（目录字段+实况字段+在途标记合并条目）。

**不变量**：

| 编号 | 不变量 | 违反处理 |
|------|--------|---------|
| I-06 | 投影真实性：env/list 恒为当次探测快照，无缓存层（D-01）；generatedAt 标注；探测失败字段为 null（不虚构已装/运行态） | 读取侧保证 |
| I-07 | 零持久化：环境条目不落库，面板重启无环境台账；变更历史经任务台账+审计 | 结构保证（无表） |
| I-08 | 目录常量：七条目与映射表编译期内置（v1.1 六→七），运行期只读（⚠️A1/A7）；无用户配置入口 | 编译期保证 |

**领域服务**：见 4.1.1（EnvCatalog/EnvProber 为本对象投影编排主体）。

#### 4.2.2 状态模型

面板侧无自有状态机（I-07 零持久化投影）；服务运行/自启状态为 systemd 实况投影（随系统实况实时变化，非面板状态迁移）；已装/未装为包管理器数据库探测结论（prd-env-mgmt.md 第六部分辨析）。

#### 4.2.3 子流程设计

无。

#### 4.2.4 接口设计

环境条目无独立写接口（全部写动作作用于 env_install_task 聚合或经 ServiceOps 直达 systemd）；读取经 env/list（4.1.4）。

#### 4.2.5 领域事件与集成

无。「仓储契约」：SQLite 侧无（零台账）；本机资源侧网关契约见 4.1.5（EnvPackGateway/SysdGateway）。

## 5. 模块清单汇总

### 5.1 对外服务清单（HTTP 接口）

| 服务（接口） | 消费方 | 契约要点 |
|-------------|--------|---------|
| GET /env/env/list | 前端环境管理页（两角色） | 目录+实况+通道实时投影（D-01/I-06）；在途任务标记（I-01） |
| POST /env/env/install | 前端环境管理页（admin） | 安装/升级幂等（D-05）；异步任务（D-04）；仓库代配（D-06）；ENV_001/002/003 |
| POST /env/env/uninstall | 前端环境管理页（admin） | 保守 remove（D-07）；已装前置 ENV_004；异步任务；强确认归前端 |
| GET /env/task/list · GET /env/task/detail | 前端环境管理页（两角色） | 任务台账与输出尾部（D-08）；ADR-018 全可看 |
| POST /env/service/{start,stop,restart,enable,disable} | 前端环境管理页（admin） | SysdGateway 复用（D-09）；目录单元限定（I-08/R-14）；ENV_004/006 |
| 环境未装实况（软引导，非 HTTP） | docker-mgmt/database-mgmt/site-mgmt 引导态 | 关联模块 PageSpec 指引链接（无跨模块 API 硬依赖） |
| 中间件 AuthRequired / CsrfGuard / RoleRequired | （复用） | account-auth 提供（3.1） |
| AuditService.Record | （出站调用） | 操作审计（R-03，I-03 同事务） |
| **db-mgmt BootstrapAfterInstall（v1.2 消费）** | **（出站调用，进程内 OHS）** | **安装后配置就绪：建账户/设密码/注册实例与凭据（D-15/D-16）；BootstrapOutcome 回传引导结果；契约见 api-database-mgmt.md §5.1** |

本模块无对外提供的 OHS 服务契约（HTTP 消费方仅前端环境管理页；软引导为 PageSpec 层指引非 API）；**v1.2 起消费 db-mgmt BootstrapAfterInstall OHS（单向硬依赖，安装后配置就绪）**。

### 5.2 SQL 清单

| # | SQL | 场景 |
|---|-----|------|
| 1 | INSERT INTO env_install_task(env_type, action, status, requested_by, created_at, updated_at) VALUES(...) | 任务创建（pending，同事务发起审计，D-04/I-03） |
| 2 | SELECT * FROM env_install_task WHERE env_type=? AND status IN ('pending','running') | 互斥判定（I-01，创建事务内） |
| 3 | SELECT * FROM env_install_task WHERE id=? | 任务详情 |
| 4 | SELECT * FROM env_install_task WHERE [env_type=?] [AND status=?] ORDER BY created_at DESC LIMIT ? OFFSET ? | 任务列表（筛选+分页） |
| 5 | UPDATE env_install_task SET status='running', started_at=?, updated_at=? WHERE id=? | 执行开始（worker） |
| 6 | UPDATE env_install_task SET output_tail=?, updated_at=? WHERE id=? | 输出尾部追加/定格（D-08） |
| 7 | UPDATE env_install_task SET status=?, error=?, bootstrap_status=?, bootstrap_note=?, finished_at=?, updated_at=? WHERE id=? | 终态回写（v1.2 +bootstrap_status/note 同事务定格，I-03/I-09；无引导步骤时 bootstrap 两列 NULL） |
| 8 | UPDATE env_install_task SET status='failed', error='面板重启中断', finished_at=?, updated_at=? WHERE status='running' | 重启恢复（I-04，启动扫描） |
| 9 | SELECT DISTINCT env_type, id FROM env_install_task WHERE status IN ('pending','running') | 环境列表在途标记（I-01 查询） |

### 5.3 DDL

```sql
CREATE TABLE IF NOT EXISTS env_install_task (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  env_type     TEXT    NOT NULL CHECK (env_type IN ('nginx','apache','docker','mysql','redis','php_fpm','mongodb')),
  action       TEXT    NOT NULL CHECK (action IN ('install','upgrade','uninstall')),
  status       TEXT    NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','succeeded','failed')),
  output_tail  TEXT    NOT NULL DEFAULT '',           -- 命令输出尾部（D-08，≤200 行定格）
  error        TEXT,                                   -- 失败原因（failed；timeout/面板重启中断等）
  requested_by TEXT    NOT NULL,                       -- 发起者用户名（admin）
  created_at   DATETIME NOT NULL,
  updated_at   DATETIME NOT NULL,
  started_at   DATETIME,
  finished_at  DATETIME,
  bootstrap_status TEXT CHECK (bootstrap_status IN ('succeeded','failed','skipped')),  -- v1.2 引导结果（NULL=无引导步骤，D-18）
  bootstrap_note   TEXT                                -- v1.2 引导摘要/原因（不含密码）
);
CREATE INDEX IF NOT EXISTS idx_env_task_env_status ON env_install_task(env_type, status);
CREATE INDEX IF NOT EXISTS idx_env_task_created ON env_install_task(created_at);
```

> 迁移：008_env_mgmt.sql（初建，env_type 六值）；v1.1 env_type CHECK 扩七值（+mongodb）经 **010_env_mgmt_mongodb.sql**（009 已被 site-mgmt DNS-01 占用）——SQLite CHECK 约束不可 ALTER，迁移内表重建（CREATE 新表（七值 CHECK）→ INSERT SELECT 全量拷贝 → DROP 旧表 → RENAME → 重建两索引；AUTOINCREMENT 序列经 sqlite_sequence 保持，无既有行丢失）。**v1.2 +bootstrap_status/bootstrap_note 经 012_env_bootstrap.sql（011 已被 database-mgmt MongoDB 占用）——新增可空列用 ALTER TABLE ADD COLUMN（SQLite 支持带 CHECK 的可空列追加，无需表重建）：`ALTER TABLE env_install_task ADD COLUMN bootstrap_status TEXT CHECK(...)` + `ADD COLUMN bootstrap_note TEXT`；既有行两列 NULL（无引导步骤），无数据丢失。**

## 6. 更新记录

### 6.1 变更记录
| v1.1a | 2026-11-17 | interface_changed（响应字段移除） | env/list 响应移除 relatedModule 字段、目录 description 去「（X 模块的运行环境）」括注（v0.1.3 UI 轮用户拍板：删除 AI 味软引导提示；PageSpec 4.1 卡片行同步） |

| 版本 | 日期 | 变更类型 | 变更摘要 | 目标节号 |
|------|------|---------|---------|---------|
| v1.0 | 2026-11-16 | initial | 首次生成：10 个 HTTP 接口（/env/{env,task,service}/*——GET 3 两角色（env/list、task/list、task/detail）/ POST 7 admin+CSRF（install、uninstall、service/{start,stop,restart,enable,disable}））+ 1 表 DDL（env_install_task：四态任务台账，env_type+status 索引承载互斥查询）+ 2 网关契约（EnvPackGateway 包操作 R-08 参数数组+PKG_OP_TIMEOUT=600s；SysdGateway 复用 service-mgmt 适配层）+ 6 错误码（ENV_001~006 + 复用 AUTH 族）+ 4 枚举引用（本域 D2 定稿入池）+ 10 审计动作常量 + 12 项设计决策（D-01 实时探测/D-02 编译期目录映射/D-03 双系通道/D-04 异步任务互斥+重启恢复/D-05 升级幂等/D-06 仓库代配/D-07 卸载保守/D-08 输出尾部 200 行/D-09 服务操作复用通道/D-10 超时与最小权限/D-11 台账全量保留/D-12 混合模式声明）；关闭 prd-env-mgmt.md 待对齐-1（DDL/上限值定稿：TASK_OUTPUT_TAIL_LINES=200）/待对齐-2（仓库代配落点=编译期模板内置，D-06）；待对齐-3（在途呈现节奏）归 D4；无旧版对比 | — |
| v1.1 | 2026-11-17 | enum_value_added + rule_changed×4 + rule_added×2 + table_changed | W1 实机冒烟缺陷修复轮（v0.1.2 实测 3 项）：①enum_value_added：env_type +mongodb（七值；DDL CHECK 同步→迁移 010 表重建；ENV_002 取值域七值；4.1.1 字段约束/I-08 目录七条目）；②rule_changed：D-01 三态如实区分（通道级/条目级/未装，W1 缺陷 2 修复口径）、D-02 映射表多候选包名+单元名规范全名+php-fpm 版本化单元解析、D-05/D-07 多候选编排（安装首选/卸载命中者）、D-06 代配补真（os-release 判定/apt keyring/yum repo 文件/env_repo_configured 审计落地/次序前移至索引刷新前）；③rule_added：D-13 单元名规范全名（EnableUnitFiles 无后缀补全→InvalidArgs，W1 缺陷 1 根因）、D-14 QueryInstalled 未装判定口径（dpkg Status 字段+LC_ALL=C+no-packages-found=未装）；④table_changed：env_install_task env_type CHECK 六值→七值（010_env_mgmt_mongodb.sql 表重建，009 已占用）；⑤业务常量 +4（MONGODB_ORG_TRACK=8.0 ⚠️待主会话终确认/VENDOR_* 路径×3/QUERY_TIMEOUT=30s）；4.1.5 契约 EnsureVendorRepo 签名落定 | 1.3（D-01/D-02/D-06/D-13/D-14）、3.4（枚举/常量/ENV_002）、4.1.1（字段约束）、4.1.4（env/list·install·uninstall）、4.1.5、5.3（DDL+迁移）、4.2 I-08 |
| v1.2 | 2026-11-17 | rule_added×5 + field_added×2 + enum_added + interface_changed×2 + table_changed | 安装后配置就绪增量轮（d0-clarification-2026-11-17-bootstrap.md，D0 拍板 Q1~Q4）：①rule_added D-15~D-19（触发与归属〔worker 后置步骤+db-mgmt BootstrapAfterInstall OHS 单向消费〕/引导通道与账户〔MySQL socket root auth_socket→dougong@localhost ALL WITH GRANT OPTION、Redis CONFIG SET+REWRITE、MongoDB localhost exception admin root、随机强密码密文零回显〕/幂等保守〔已配置跳过〕/引导失败独立维度〔bootstrap_status/note，任务 status 仍包操作结果〕/服务就绪与超时〔幂等 start+BOOTSTRAP_READY_TIMEOUT=60s 有界重试+全注入化 CI 预防〕）；②field_added：env_install_task +bootstrap_status(CHECK 三值可空)/bootstrap_note（迁移 012 ADD COLUMN，011 已占用）；③enum_added：bootstrap_status{succeeded,failed,skipped} 入池（NULL=无引导步骤非值）；④interface_changed：task/list·task/detail 响应 +bootstrapStatus/bootstrapNote（无新接口，S7 Action 数不变 10）；install 执行链 +引导步骤；⑤table_changed：DDL +两列；⑥业务常量 +env_bootstrap 审计动作 +BOOTSTRAP_READY_TIMEOUT/BOOTSTRAP_PASSWORD_BYTES/BOOTSTRAP_ENV_TYPES；⑦仓储契约 Finish 扩展 +BootstrapGateway OHS 消费契约；⑧I-09 引导结果与终态同事务；1.5/3.2/5.1 +db-mgmt OHS 消费登记；关闭 prd 待对齐-4 | 1.3（D-15~D-19）、1.5、3.4（枚举/常量/审计）、4.1.1（字段/I-09/领域服务）、4.1.4（install·task/list·task/detail）、4.1.5、5.1、5.2（SQL#7）、5.3（DDL+迁移 012） |

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
