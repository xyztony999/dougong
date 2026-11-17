# 斗拱面板 环境管理（env-mgmt）PRD

| 属性 | 值 |
|------|-----|
| 版本 | v1.2 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-17（v1.2 安装后配置就绪增量轮） |
| 输入 | D0 增量澄清拍板（随 business-arch-env-mgmt.md ⚠️A1~A15 承载；本轮 d0-clarification-2026-11-17-bootstrap.md 拍板 Q1~Q4）、D1 三文档（business-arch/application-arch/function-list-env-mgmt.md v1.1）、domain-model.md v1.22、idea.md §3/§4.1/§7（R-02/R-03/R-08/R-14/ADR-018）、install.sh 支持矩阵（Debian 11+/Ubuntu 20.04+/RHEL 9 系） |
| 下游 | D3 API 设计（02-detailed/api/api-env-mgmt.md）、D4 交互设计（02-detailed/interaction/{admin,readonly}/env-mgmt/） |

## 第一部分：业务目标（G1）

为登录运维者提供**运行环境本体的供给与治理能力**：面板此前只能「接管已有环境」（site-mgmt 接管 nginx、database-mgmt 纳管已运行数据库实例、docker-mgmt 管理已有 daemon 的容器），缺失「从无到有安装环境本体」的能力（v0.1.2 用户反馈第 4 项）。本模块补位软件包层：经发行版包管理器（apt / dnf·yum 双系探测择一，⚠️A2）安装、升级、卸载七类常用运行环境（nginx、Apache、Docker、MySQL、Redis、PHP-FPM、MongoDB，⚠️A1——v1.1 扩 MongoDB〔数据库类用户点名缺失〕），提供已装环境的版本/服务状态观测与服务启停自启入口（复用 systemd 通道，衔接 service-mgmt 通用单元治理）。安装/卸载为**异步任务**（包安装可达数分钟，⚠️A3），任务台账持久化（面板重启后可观测、在途互斥跨重启一致）。安全纪律贯穿：apt/yum 包操作最小权限、幂等（升级=重装，⚠️A4）、审计留痕（R-03）、外部命令一律字面量参数数组（R-08）、服务端校验（R-14），沿用 R-01~R-18 与 SEC-GATE 纪律。服务对象：管理员（admin）执行全部变更，只读用户（readonly）查看环境全景与任务状态输出（ADR-018）。

**v1.2 安装后配置就绪增量（产品定调「好面板的标准是装完即配置就绪」）**：实证痛点（DBM_005 诊断轮终裁）——env-mgmt 经 apt 装完 MySQL 只有 root@localhost（auth_socket 无密码），面板 db-mgmt 走 TCP+密码验证通道连不上，用户无法自助建第一个账户，**装完 ≠ 可用**。本轮补位：数据库类环境（MySQL/Redis/MongoDB）安装/升级成功后，worker 自动执行 post-install bootstrap——经引导通道建管理账户/设随机强密码（MySQL dougong@localhost 全权账户 / Redis requirepass / MongoDB admin root 用户），并自动注册实例与凭据到 database-mgmt（**装完即可管**）；引导为独立后置步骤，结果经任务台账独立维度呈现（安装成功但引导失败如实标注，⚠️A11~A15）。PHP/nginx/Apache/Docker 装完即用无引导。

与既有模块的四层分界（AA 附录B）：环境本体（软件包层）=本模块；已运行数据库实例（实例层）=database-mgmt；容器（容器层）=docker-mgmt；全部 systemd 单元通用治理（通用单元层）=service-mgmt；nginx 站点配置与证书（站点层）=site-mgmt。**安装后配置就绪的边界**：触发与结果呈现（worker 后置步骤/服务就绪/引导结果入任务台账）归本模块；建账户/设密码/注册实例与凭据归 database-mgmt 实例层（经进程内 OHS BootstrapAfterInstall，单向消费，⚠️A12）。

## 第二部分：功能范围（G2）

**IN（做什么）**：

| # | 能力 | 依据 |
|---|------|------|
| 1 | 环境列表查看：内置七环境目录条目（v1.1 六→七：+MongoDB；显示名/描述/关联模块提示）+ 实况（已装/版本/服务运行/自启/通道可用性/在途任务标记）合并呈现 | D1 AA S-53-01（⚠️A1/A2/A7）+ v1.1 ⚠️A1/A10 |
| 2 | 环境实况探测（机制）：逐环境包管理器查询已装版本（⚠️A7 映射，v1.1 起为有序候选包名数组——任一命中即已装，包名不在包数据库=未装而非探测失败，⚠️A9）+ systemd 单元状态查询（⚠️A8 规范全名）+ 包管理器通道探测（apt/dnf·yum 择一）；纯读每请求实时探测不缓存；三态如实（通道级不可用/条目级探测失败/未装，⚠️A9） | D1 AA S-53-02 + v1.1 ⚠️A9 |
| 3 | 环境安装与升级：通道可用性前置→先刷新包索引→安装（已装环境同动作为升级到仓库最新版，幂等 ⚠️A4）→需第三方官方仓库的环境（v1.1 清单：MongoDB 三系 + RHEL 系 docker-ce，⚠️A10）先由面板写入仓库源配置（审计留痕，⚠️A2；v1.0 实现为空转日志，v1.1 补真实现）→异步任务执行 | D1 AA C-14-01 + v1.1 ⚠️A10 |
| 4 | 环境卸载：强确认交互（风险提示：本域服务运行实况 + 关联模块影响静态警示）→包 remove（保留系统配置与数据目录，⚠️A6）→异步任务执行 | D1 AA C-14-02 |
| 5 | 环境变更任务跟踪：任务状态（pending → running → succeeded/failed，⚠️A3）、动作、目标环境、命令输出尾部（排障）、失败原因；同环境在途互斥；任务台账跨重启留存 | D1 AA C-14-03 |
| 6 | 环境服务启停与自启：对已装环境执行服务 start/stop/restart 与自启 enable/disable（经 systemd 通道，操作对象限定目录环境单元 ⚠️A7）；结果如实反馈 | D1 AA S-54-01 |
| 7 | 安装后配置就绪（v1.2，C-14-04 机制型）：数据库类环境（MySQL/Redis/MongoDB）安装/升级成功后自动引导——确保 DB 服务就绪（幂等 start，⚠️A15）→ 经 database-mgmt OHS 建管理账户/设随机强密码 + 注册实例与凭据（装完即可管，⚠️A11/A12）→ 引导结果独立维度入任务台账（bootstrap_status/note，⚠️A14）；幂等保守不覆盖用户已有配置（⚠️A13）；引导审计、密码密文零回显 | D1 AA C-14-04 + D0 拍板 Q1~Q4（2026-11-17） |

**OUT（不做什么）**：

| # | 事项 | 归属 |
|---|------|------|
| 1 | 源码编译安装（仅发行版仓库包，含个别官方仓库源配置） | 不做（D0 增量澄清 OUT） |
| 2 | PHP 多版本并存与切换（sury.org/ondrej PPA 5.6~8.6 coinstallable，各版本独立 phpX.Y-fpm） | **二期另起轮**（v1.2 登记方向；本轮 OUT，用户拍板） |
| 2a | MySQL 版本升级路径（mysql-apt-config 单版本原地升级，不支持降级/禁跳系列/须备份） | **二期另起轮**（v1.2 登记方向；本轮 OUT，用户拍板） |
| 3 | 「应用市场/插件商店」形态（目录内置六环境，按本项目轻量定位裁剪宝塔软件商店面） | 不做（D0 OUT；目录扩展走面板版本迭代 ⚠️A1） |
| 4 | 任务取消（包管理器中断态不安全） | 不做（⚠️A3 OUT） |
| 5 | purge 卸载与数据目录清理 | 二期（⚠️A6 OUT） |
| 6 | 可升级版本检测与提示（「有新版本可升」主动检测） | 二期（⚠️A4 OUT；升级=用户主动发起） |
| 7 | 降级/指定版本安装 | 不做（D0 OUT） |
| 8 | 运行环境内部配置管理（nginx 站点配置归 site-mgmt、MySQL 实例归 database-mgmt、容器归 docker-mgmt、全部系统服务清单归 service-mgmt） | 不做（四层分界，AA 附录B）；**例外**：安装后配置就绪的建账户/设密码/注册实例与凭据归 database-mgmt 实例层，本模块经 OHS 触发并呈现结果（IN-7，非本模块自行治理实例内部配置） |
| 9 | 卸载前跨模块动态数据计数（容器数/纳管实例数/站点数） | 不做（无跨模块硬调用，AA 附录B 软引导口径；风险提示=静态警示文案+本域 systemd 服务实况，见 4.2 FR-04 落地口径） |
| 10 | Windows/无 systemd 环境支持 | 不做（install.sh 支持矩阵外，如实报错） |
| 11 | 环境页面外壳与导航挂载 | panel-core（应用骨架与导航） |

## 第三部分：角色定义（G3）

| 角色 | 英文 | 职责与权限 | 获得方式 |
|------|------|-----------|---------|
| 管理员 | admin | 查看环境全景与任务 + 全部变更类操作（安装/升级/卸载/服务启停自启） | account-auth 初始化/用户管理创建 |
| 只读用户 | readonly | 同等查看环境列表（目录+实况）与任务状态/输出（ADR-018 全可看）；一切变更无入口（界面不提供）+ 越界请求被全局边界管控拒绝（account-auth S-03-02） | 管理员创建并指派 |

访客（未认证操作者）不涉及：本模块全部功能位于登录会话之后（R-02）。角色差异边界：全部写类操作为 admin 专属；查看类动作不写审计（R-03 语义为「变更」留痕）；异步任务终态（succeeded/failed）由任务执行链写审计。

## 第四部分：功能需求明细

> 与 AA（application-arch-env-mgmt.md）6 个功能子模块一一对应，覆盖 FL（function-list-env-mgmt.md）admin 7 个菜单行（readonly 2 个只读行同 FR 只读视角）；变更类 FR 全部记审计。

### 4.1 环境目录与实况（S-53）

| 编号 | 需求 | 主流程与规则 | 审计 |
|------|------|-------------|------|
| FR-01 | 环境列表查看（S-53-01） | 呈现内置七环境（⚠️A1，v1.1 六→七 +MongoDB）全景列表：显示名、描述、关联模块提示（Docker→docker-mgmt、MySQL/MongoDB→database-mgmt、nginx/PHP-FPM→site-mgmt 等）、已装版本（⚠️A7 映射查询）、服务运行/自启状态（systemd 单元查询）、包管理器通道可用性（apt/dnf·yum 探测择一，⚠️A2——通道不可用时变更能力如实标注不可用）、在途任务标记；实况每请求实时探测不缓存不虚构（nginx 未检出返回未安装，与 site-mgmt 投影真实性同哲学）；未装条目呈现安装入口、已装呈现升级/卸载/服务运维入口（admin；readonly 负空间）；两角色同等可看（ADR-018） | —（查看类） |
| FR-02 | 环境实况探测（S-53-02，机制型） | 逐环境探测三路：①已装与版本——按发行版族包名映射（⚠️A7 编译期数据；v1.1 起每环境为**有序候选包名数组**：任一命中即已装〔含用户经官方源自装的 docker-ce/mysql-community-server 等形态〕，全部候选均查无〔包名不在包数据库〕=未装而非探测错误；dpkg 侧按 Status 字段判定 install/hold ok installed——数据字段判定，不受 locale 本地化消息影响，⚠️A9）；②服务运行/自启——按单元名映射（⚠️A8 规范全名 .service 后缀）经 systemd 通道查询（复用 service-mgmt SysdGateway 适配层资产）；③通道可用性——apt/dnf/yum 二进制存在性探测择一（Debian 系 apt、RHEL 系 dnf 无则回退 yum）；纯读机制，被列表/安装/卸载/服务运维共同消费；探测失败如实呈现（不虚构已装/运行态；条目级探测失败 installed=null「探测不可用」与通道级不可用〔channel.available=false 页头横幅〕与未装〔installed=false〕三态如实区分，⚠️A9） | —（查看类机制） |

### 4.2 环境安装与变更（C-14）

| 编号 | 需求 | 主流程与规则 | 审计 |
|------|------|-------------|------|
| FR-03 | 环境安装与升级（C-14-01） | 安装目录环境（已装环境同动作为升级，幂等 ⚠️A4）：通道可用性前置（不可用如实拒绝）；发起即创建异步任务（pending，⚠️A3）并立即返回 taskId；任务执行链：仓库源代配（v1.1 次序前移至索引刷新**之前**——新写入源须参与本次 makecache/update；需第三方官方仓库的环境清单 v1.1 扩为：MongoDB〔mongodb-org，apt/dnf/yum 三系均需〕+ RHEL 系 docker-ce，⚠️A10；写入发行版仓库源配置并审计留痕 ⚠️A2；不支持的目标发行版/代际→任务 failed 如实，不写任何文件）→ 刷新包索引（apt update / dnf makecache）→ 包安装（最小权限、字面量参数数组 R-08；安装目标=候选数组首选〔发行版原生包优先；mongodb-org 例外〕）→ 终态落库；安装完成后不强制 start/enable（尊重包脚本默认行为，⚠️A5；数据库类引导路径可启动服务，⚠️A15），实况经 FR-02 如实反映；**数据库类环境安装成功后衔接 FR-07 安装后配置就绪引导（worker 后置步骤）**；任务输出尾部与失败原因经 FR-05 呈现，不虚构成功 | env_install_started（创建任务）+ env_repo_configured（代配写入时，v1.1 补实现）+ 任务终态审计（change，detail 含动作/版本） |
| FR-04 | 环境卸载（C-14-02） | 卸载已装环境：强确认交互——风险提示分两层（**落地口径，对 BA ⚠️A6「列出警示」的细化**）：①本域实况——目标环境服务单元正在运行时警示加重（systemd 实况，本模块通道）；②关联模块影响——静态警示文案（「卸载 Docker/Nginx/MySQL/MongoDB 后对应容器/站点/数据库模块将失去运行环境，请先自行到对应模块确认数据已处置」，v1.1 补 MongoDB），**不做跨模块动态计数**（OUT-9，无跨模块硬调用）；知情决策不硬阻断；执行包 remove（保留系统配置与数据目录，⚠️A6；v1.1 卸载目标=候选数组中实际命中〔已装〕的包，非盲删首选）；异步任务执行（同 FR-03 通道）；卸载后环境条目回未装态（已配第三方源文件保留不回收——重装即用，源文件无安全残留风险〔仅含仓库 URL 与公钥〕） | env_uninstall_started + 任务终态审计（detail 含风险提示呈现记录） |
| FR-05 | 环境变更任务跟踪（C-14-03） | 异步任务的观测与结果确认：任务状态 pending → running → succeeded / failed（⚠️A3）；呈现目标环境、动作（install/upgrade/uninstall）、发起者、发起时间、命令输出尾部（排障，行数上限编译期常量）、失败原因、**引导结果（bootstrap_status succeeded/failed/skipped，NULL=无引导步骤 + bootstrap_note 摘要，v1.2 ⚠️A14）**；同环境在途任务互斥（pending/running 存在时新变更请求如实拒绝）；任务台账持久化（跨重启留存）；面板重启时在途 running 任务标 failed（error=面板重启中断，如实呈现）；任务取消不做（OUT-4）；两角色可看（ADR-018） | 任务终态结果一并留痕（succeeded/failed 均记） |
| FR-07 | 安装后配置就绪（C-14-04，v1.2 机制型） | 数据库类环境（MySQL/Redis/MongoDB）安装/升级成功后的自动配置引导（独立后置步骤，⚠️A11）：①服务就绪——确保 DB 服务 active（幂等 start，等待引擎就绪有界重试，⚠️A15；enable 不强制）；②建账户/设密码——经 database-mgmt 进程内 OHS（BootstrapAfterInstall）由引导通道执行（⚠️A12）：**MySQL** 经 unix socket 以 root auth_socket 免密建连（引导通道，区别于 TCP+密码验证通道）建 dougong@localhost（ALL PRIVILEGES ON *.* WITH GRANT OPTION，随机强密码，**不动 root 的 auth_socket**）；**Redis** CONFIG GET requirepass 判未设置→CONFIG SET requirepass 随机密码 + CONFIG REWRITE 持久化 + bind 本机校验（仅未设置时写）；**MongoDB** localhost exception 免认证连 localhost 建 admin 库 dougong root 角色用户随机密码；③注册——经治理通道（TCP）Ping 验证新凭据可连→AES-256-GCM 加密 UPSERT db_instance（本机默认 identity）+ conn_state=reachable + version 刷新（装完即可管）；④幂等保守（⚠️A13）——目标实例已配置凭据 / MySQL 账户已存在 / Redis requirepass 已设 / MongoDB 已有用户 → 跳过不覆盖（bootstrap_status=skipped）；⑤结果——bootstrap_status(succeeded/failed/skipped)+note 入任务台账与输出尾部 [bootstrap] 行（⚠️A14）；安装成功但引导失败 → 任务 status=succeeded + bootstrap_status=failed，呈现「已安装·引导失败」+原因+「重新安装/升级以重试引导」恢复指引（幂等重跑）；密码 crypto/rand 随机强密码、密文落库、**永不回显**（审计 detail/输出/响应均不含密码）；PHP/nginx/Apache/Docker 装完即用无引导，卸载无引导 | env_bootstrap（引导动作，object_type=env，detail 含 envType/status，**不含密码**）+ database-mgmt instance_bootstrapped（实例注册，脱敏） |

### 4.3 环境服务运维（S-54）

| 编号 | 需求 | 主流程与规则 | 审计 |
|------|------|-------------|------|
| FR-06 | 环境服务启停与自启（S-54-01） | 对已装环境执行服务启动/停止/重启与自启 enable/disable：经 systemd 通道生效（复用 service-mgmt SysdGateway 适配层资产，进程内共享非模块调用）；操作对象限定目录环境对应服务单元（⚠️A7 映射，不开放任意单元——全部系统服务清单治理归 service-mgmt）；结果与失败原因如实呈现；未装环境的服务操作拒绝 | env_service_started/stopped/restarted/enabled/disabled（change） |

## 第五部分：业务规则汇总

| 编号 | 规则 | 来源 |
|------|------|------|
| R-03 | 全部变更类操作写审计：安装/升级/卸载发起与终态、仓库源写入、服务启停自启；任务终态结果一并留痕；经 account-auth 审计契约（OHS） | idea.md §4.1 M1 / kb |
| R-08 | 外部命令一律字面量参数数组：apt/dnf/yum 及其子命令、systemctl——不拼 shell 字符串、不注入用户输入 | idea.md §7 / kb |
| R-14 | 服务端校验全部输入：envType ∈ 六值目录、服务动作 ∈ 枚举、任务查询参数——不信任前端 | idea.md §7 / kb |
| R-02 | 本模块全部接口位于 HttpOnly Cookie 会话之后；遵奉同源 + CSRF 全站协议 | idea.md §5.4 / kb |
| ADR-018 | 只读边界：环境列表/任务状态输出两角色同等；一切变更 admin 专属——readonly 界面无入口 + 越界请求被全局边界管控拒绝并提示原因 | D0 / kb |
| ⚠️A1 | 七环境静态目录（编译期数据非用户可配；v1.1 六→七：+MongoDB〔env_type=mongodb〕）；目录扩展走面板版本迭代 | D0 增量澄清（2026-11-16）+ v1.1 增量轮（2026-11-17，用户点名） |
| ⚠️A8 | systemd 单元名一律规范全名（含 .service 后缀，映射表承载）：EnableUnitFiles/DisableUnitFiles 的 DBus 契约不自动补后缀（无后缀名 systemd 直接回 InvalidArgs——W1 实机缺陷 1 根因；StartUnit/LoadUnit 有服务端补全故仅 enable/disable 报错），目录统一全名消除不对称 | v1.1 增量轮拍板（2026-11-17，根因见 api D-13） |
| ⚠️A9 | 已装探测三态如实区分：通道级不可用（channel.available=false，页头横幅）/ 条目级探测失败（installed=null「探测不可用」——仅真错误：命令超时/通道异常等）/ 未装（installed=false——含包名完全不在包数据库形态）；dpkg 侧以 Status 字段判定（install/hold ok installed=已装；deinstall ok config-files=未装），包名不在数据库按未装呈现非错误；命令环境注入 LC_ALL=C 消除输出本地化影响；候选包名数组任一命中即已装 | v1.1 增量轮拍板（2026-11-17，W1 实机缺陷 2 根因修复口径，经主会话确认中 ⚠️） |
| ⚠️A10 | 第三方源代配机制（v1.0 设计声明 D-06、实现空转，v1.1 补真）：按 /etc/os-release 判定（ID+VERSION_CODENAME / ID_LIKE+VERSION_ID）目标发行版与代际；apt=写 /etc/apt/sources.list.d/mongodb-org-8.0.list（signed-by 引用面板经 HTTPS 下载的官方公钥 .asc，落 /usr/share/keyrings/，不依赖 curl/gpg 命令）；dnf/yum=写 /etc/yum.repos.d/{mongodb-org-8.0,docker-ce}.repo（gpgkey URL 内联）；MongoDB 版本轨道 8.0 ⚠️（现行稳定主轨，Debian12/Ubuntu22.04+/RHEL8/9 支持；待主会话确认）；代配清单：MongoDB 三系 + docker-ce RHEL 系（apt 系 docker 装发行版原生 docker.io 不代配；MySQL 官方源不代配——Debian12 无 mysql-server 属已知限制如实失败）；不支持组合任务 failed 如实 | v1.1 增量轮拍板（2026-11-17，经主会话确认中 ⚠️） |
| ⚠️A2 | apt + dnf·yum 双系探测择一（Debian 系 apt / RHEL 系 dnf 无则 yum）；第三方官方仓库源由面板代配（写入审计留痕）；两者皆无（非 Linux/无包管理器）时变更能力如实不可用 | D0 拍板 |
| ⚠️A3 | 异步任务 pending → running → succeeded/failed；同环境在途互斥；取消不做；任务台账持久化 | D0 拍板（持久化口径 D2 落定：**落 SQLite**） |
| ⚠️A4 | 已装环境再次安装=升级（先刷新包索引，安装仓库最新版，幂等）；可升级检测二期不做 | D0 拍板 |
| ⚠️A5 | 安装完成后不强制 start/enable，尊重发行版包脚本默认行为；实况如实呈现 | D0 拍板 |
| ⚠️A6 | 卸载=包 remove（保留系统配置与数据目录）；强确认+风险提示（本域服务实况+关联模块静态警示，动态计数不做）；purge 二期不做 | D0 拍板（落地口径本 PRD FR-04 细化） |
| ⚠️A7 | 每环境每发行版族的包名、systemd 单元名、版本查询方式经目录条目编译期映射表承载（v1.1 起包名=有序候选数组、单元名=规范全名：Debian 系 nginx=[nginx]/apache=[apache2]/docker=[docker.io,docker-ce]/mysql=[mysql-server,mysql-community-server]/redis=[redis-server]/php-fpm=[php-fpm]/mongodb=[mongodb-org]；RHEL 系 nginx=[nginx]/apache=[httpd]/docker=[docker-ce,docker]/mysql=[mysql-server,mysql-community-server]/redis=[redis]/php-fpm=[php-fpm]/mongodb=[mongodb-org]；单元名 apt：nginx.service/apache2.service/docker.service/mysql.service/redis-server.service/php-fpm 版本化解析/mongod.service，dnf·yum：nginx.service/httpd.service/docker.service/mysqld.service/redis.service/php-fpm.service/mongod.service） | D0 拍板 + v1.1 增量轮修订（多候选+全名） |
| ⚠️A11 | 安装后配置就绪：数据库类（MySQL/Redis/MongoDB）安装/升级成功后 worker 自动引导（建账户/设密码+注册实例凭据到 database-mgmt，装完即可管）；PHP/nginx/Apache/Docker 装完即用无引导；卸载无引导；引导为独立后置步骤 | v1.2 D0 拍板（2026-11-17，d0-clarification-2026-11-17-bootstrap.md） |
| ⚠️A12 | 引导通道与账户：MySQL unix socket root auth_socket 免密建 dougong@localhost（ALL PRIVILEGES *.* WITH GRANT OPTION，不动 root）/ Redis CONFIG SET requirepass+CONFIG REWRITE（仅未设置时写）+bind 本机校验 / MongoDB localhost exception 建 admin root 用户；随机强密码 crypto/rand，AES-256-GCM 密文落库零回显；经 db-mgmt 进程内 OHS BootstrapAfterInstall | v1.2 D0 拍板 Q1/Q2/Q4 |
| ⚠️A13 | 引导幂等保守：目标实例已配置凭据 / MySQL 账户已存在 / Redis requirepass 已设 / MongoDB 已有用户 → 跳过不覆盖（bootstrap_status=skipped）；已装/已配置不重复引导；引导动作审计留痕 | v1.2 D0 拍板（conffile 保守先例） |
| ⚠️A14 | 引导失败独立维度：任务 status=包操作结果；bootstrap_status(succeeded/failed/skipped，NULL=无引导步骤)+bootstrap_note；安装成功但引导失败 → succeeded+bootstrap_status=failed，呈现「已安装·引导失败」+原因+恢复指引（重新安装/升级幂等重跑）；不混淆「已装」与「可治理」 | v1.2 D0 拍板 Q3 |
| ⚠️A15 | 引导服务联动细化（⚠️A5 细化）：引导路径可为配置而幂等 start DB 服务并等待引擎就绪（有界重试），enable 仍不强制；非引导环境维持 ⚠️A5 | v1.2 D0 拍板 |
| 本域推导 | 引导凭据安全：随机强密码（crypto/rand，不弱默认）；账户 host 限本机、bind 本机不公网暴露；密码密文落库（沿 CredentialVault/db-cred.key 0600 先例）永不回显（审计 detail/任务输出/响应均不含密码/密文/密钥） | v1.2 ⚠️A12/A13 + R-03 脱敏先例 |
| 本域推导 | 幂等语义：升级幂等（重复安装=终态一致）；服务启停幂等（重复 start 对已运行服务无副作用）；卸载幂等（未装时报 NOT_INSTALLED 如实拒绝，非静默成功）；引导幂等（已配置跳过，重新安装/升级重跑终态一致） | ⚠️A4/⚠️A13/同构先例 |
| 本域推导 | 越界拒绝不记审计：R-03 语义为「变更」留痕（与 site-mgmt/service-mgmt 同口径） | R-03 字面口径 |
| 本域推导 | 任务执行最小权限：包操作经专用低权执行通道（root 身份运行面板时以能力最小化方式执行；实现细节归 D3/后端工程，业务口径=不提权不扩散） | D0「最小权限」拍板 |

## 第六部分：状态流转（T2）

本域有 **1 个状态机**：环境变更任务 env_task_status（⚠️A3，持久化）。

| 当前状态 | 目标状态 | 触发 | 备注 |
|---------|---------|------|------|
| pending | running | worker 取出任务开始执行 | 创建即 pending（发起接口同步返回 taskId） |
| running | succeeded | 包操作命令链执行成功（退出码 0） | 终态；输出尾部定格 |
| running | failed | 包操作失败 / 仓库源写入失败 / 执行超时（上限编译期常量） | 终态；error+输出尾部定格 |
| running | failed | 面板重启恢复（启动扫描 running 任务） | error=「面板重启中断」（如实呈现，不虚构成功） |

**辨析表**（易混淆项澄清，防伪状态机）：

| 易混淆项 | 辨析 | 结论 |
|---------|------|------|
| 服务运行/自启状态 | systemd 实况投影（运行中/停止/自启开/关），随系统实况实时变化，无面板侧状态迁移存储 | 实况投影，非状态机 |
| 环境已装/未装 | 包管理器数据库实况的探测结论（已装+版本），非面板侧状态 | 实况投影，非状态机 |
| 「探测不可用」两义（v1.1 澄清，⚠️A9） | 通道级（页头：包管理器通道不存在）与条目级（卡片角标：该环境查询真错误）不同层语义，前者禁变更操作、后者仅该环境实况未知 | 分层判定值，非状态机 |
| 任务取消 | OUT-4 不做（包管理器中断态不安全）；「不提供」即无取消状态 | 显式 OUT |
| 通道可用性 | 环境探测的整值刷新结论（apt/dnf/yum/null），非对象状态 | 域级判定值 |
| 引导结果 bootstrap_status（v1.2，⚠️A14） | 安装后配置就绪引导的一次性结果维度（succeeded/failed/skipped，NULL=无引导步骤），随任务终态定格，无流转语义；**与任务 status 独立**——任务 status 仍为包操作四态机，引导失败不改变任务 succeeded（包已装），仅 bootstrap_status=failed 标注「已安装·引导失败」 | 结果维度，非状态机（不新增状态机，仍 1 个 env_task_status） |

无审批类状态流转（T3 N/A 依据）：环境变更为管理员即时发起的异步执行，无提交/授权环节；唯一授权边界=角色判定（ADR-018），属访问控制非审批流。

## 第七部分：查询与导出需求（T4/T5）

**查询（T4）**：

| 查询形态 | 筛选/排序维度 | 说明 |
|---------|-------------|------|
| 环境全景查询 | 无筛选（七条目全量，通道状态随响应） | 目录+实况合并呈现；两角色同等 |
| 任务列表查询 | envType? / status? 筛选，发起时间倒序 | 任务台账（低频操作量级小）；含引导结果角标（bootstrap_status）；两角色同等 |
| 单任务详情 | taskId 定位 | 输出尾部+失败原因+引导结果（bootstrap_status/note）；两角色同等 |

**导出（T5）**：无集合导出需求（环境七条目/任务低频台账；审计导出归 account-auth）。

**被引用关系（T1a）**：

| 被引用能力 | 消费方 | 方式 |
|-----------|--------|------|
| 环境未装实况（软引导） | docker-mgmt / database-mgmt / site-mgmt 引导态 | 关联模块 PageSpec 呈现「到环境管理安装」指引链接（无跨模块 API 硬依赖，AA 附录B） |
| 消费方视角 | account-auth | 消费其审计写入（R-03）、会话鉴权与只读边界（R-02/ADR-018）契约 |
| 消费方视角 | service-mgmt | 共享 SysdGateway systemd 适配层资产（进程内适配器共享，非模块调用） |
| 消费方视角（v1.2） | database-mgmt | 消费其进程内 OHS BootstrapAfterInstall（建账户/设密码/注册实例与凭据，⚠️A12）——安装后配置就绪引导（单向依赖，与 cron-backup 消费 DumpStream 同构）；引导结果回传任务台账 |

## 第八部分：非功能性要求与异常场景

**N1 非功能性（已锁定项）**：七环境目录为编译期数据（v1.1 六→七）；实况探测每请求实时（无缓存层）；任务台账量级小（低频安装/卸载积累）；包操作超时上限与任务输出尾部行数上限为编译期常量（D3 定值）；任务台账全量保留（低频事件，不建清理任务——避免常驻 goroutine）；代配公钥下载与源文件写入可注入测试（探测/文件 IO/HTTP 依赖注入——CI runner 环境无关，v0.1.2 CI 环境耦合 4 起教训预防）；**安装后配置就绪引导全注入化（v1.2）——MySQL socket root 建连/Redis CONFIG/MongoDB localhost exception 建用户/凭据注册均经网关接口注入替身，零 runner 实况引擎依赖（CI 环境耦合第 7 起预防；引擎实联归 Linux 目标环境冒烟）**。未锁定项（移交 D3）：任务输出尾部行数上限与包操作超时上限具体值；仓库源写入的具体路径与文件名；RHEL 系 Docker CE 等需官方仓库的环境-发行版组合清单（编译期映射表内容）；面板数据目录/日志对任务执行的承载细节；引导服务就绪等待上限与随机密码长度（D3 定值）。

**N2 异常场景**：

| 场景 | 处理 |
|------|------|
| 包管理器通道不可用（非 Linux / apt 与 dnf·yum 皆无） | 变更类接口如实拒绝（ENV_001 语义）；列表照常呈现目录+通道不可用标注（观测不依赖通道——版本探测经通道时已装态呈现「探测不可用」，如实） |
| 逐环境包查询失败 vs 未装（v1.1 三态区分，⚠️A9） | 包名不在包数据库（dpkg "no packages found matching" / rpm "not installed"）=**未装**如实呈现（installed=false），非错误；仅真探测错误（命令超时/通道异常）条目级 installed=null「探测不可用」；通道级不可用与条目级失败分列呈现（页头横幅 vs 卡片角标） |
| 目标发行版/代际不在 MongoDB 等第三方源支持矩阵（v1.1，⚠️A10） | 任务终态 failed（error 明示不支持组合），不写任何源文件；面板不虚构仓库可用性 |
| 包安装失败（网络不通/仓库不可达/依赖冲突） | 任务终态 failed + 命令输出尾部 + 失败原因如实呈现；不自动重试（操作者决策后重新发起，升级幂等） |
| 包操作超时 | 任务终态 failed（超时上限编译期常量）；输出尾部定格 |
| 同环境重复发起变更 | 在途互斥拒绝（pending/running 存在时，ENV_003 语义，data.taskId 指向在途任务） |
| 面板重启时任务在途 | 启动恢复：running 任务标 failed（error=面板重启中断）；不虚构 succeeded |
| 卸载时服务仍在运行 | 风险提示加重（本域 systemd 实况），知情决策不硬阻断 |
| 仓库源写入失败 | 任务终态 failed（不进入安装步骤）；已写入部分如实呈现于输出尾部 |
| 服务操作失败（systemd 拒绝/单元不存在） | 失败原因如实呈现（ENV_006 语义）；不虚构成功 |
| 未装环境发起卸载/服务操作 | 如实拒绝（NOT_INSTALLED 语义），非静默成功 |
| readonly 越界发起任何变更 | 全局边界管控拒绝并提示边界原因（ADR-018）；界面本就无入口 |
| 空任务列表 | 返回空列表 + 引导，非错误 |
| 安装成功但引导失败（v1.2，⚠️A14） | 任务 status=succeeded（包已装）+ bootstrap_status=failed + note 原因（如 socket 建连失败/CREATE USER 被拒/验证通道连不上/注册失败）；呈现「已安装·引导失败」+「重新安装/升级以重试引导」恢复指引；不虚构可治理 |
| 引导通道建连失败（MySQL socket 无 root auth_socket / 服务未就绪 / Redis/MongoDB 连不上） | 有界重试等待引擎就绪后仍失败 → bootstrap_status=failed + note 如实原因；不影响包安装 succeeded |
| 用户已有配置（账户已存在 / requirepass 已设 / MongoDB 已有用户 / 实例已配凭据） | 保守跳过不覆盖（bootstrap_status=skipped + note 说明）；不重置用户密码、不覆盖已有凭据（⚠️A13） |
| 引导注册验证失败（新凭据经 TCP 治理通道 Ping 不通） | bootstrap_status=failed + note（账户已建但面板验证通道连不上，如实呈现排查指引）；不写入未验证凭据（沿 D-03 先验证后落库哲学） |
| 非数据库环境 / 卸载 | 无引导步骤（bootstrap_status=NULL），任务照常终态 |

**N3 业务规则细节**：见第五部分规则汇总；补充——任务创建与首次状态落库同事务（创建即持久，防发起即失忆）；互斥判定与任务创建同事务原子（防并发双开）；服务启停对已处目标态幂等；**引导结果与任务终态同事务落库（bootstrap_status/note 随终态定格，防引导结果失忆）；引导凭据注册与审计同事务（db-mgmt 侧 R-03）**。

**N4 历史需求**：任务台账即环境变更历史（动作/发起者/终态/输出尾部）；操作追溯经 account-auth 审计日志（object_type=env_install_task）；无独立环境变更历史表（台账即历史）。

## 第九部分：假设与待对齐项

**跨业务域集成**（Step 9）：

| 模式 | 集成点 | 说明 |
|------|--------|------|
| 遵奉者 | 会话鉴权与只读边界协议 | 全部接口位于登录会话后（R-02）；查看两角色同等、变更 admin 专属（ADR-018） |
| OHS（消费） | 审计写入接口（account-auth S-04） | 全部变更类操作 + 任务终态写审计（R-03） |
| OHS（消费，v1.2） | database-mgmt BootstrapAfterInstall | 安装后配置就绪引导：建账户/设密码/注册实例与凭据归 db-mgmt 实例层（进程内 OHS，单向依赖 env-mgmt→db-mgmt，⚠️A12）；引导结果（status+摘要，不含密码）回传任务台账；契约见 api-database-mgmt.md §5.1 |
| 共享适配层（非业务域集成） | service-mgmt SysdGateway | systemd 查询与操作复用同一适配器实现（进程内共享）；本模块服务操作限定目录环境单元，service-mgmt 保持全部单元治理；引导前确保 DB 服务 active 亦经此通道（⚠️A15） |
| 本机资源（非业务域） | apt/dnf/yum、systemd | 经 envpack 适配器（R-08 参数数组）执行包操作；本机系统组件访问非外部业务系统对接（与 monitoring/service-mgmt 本机口径一致） |

**过程数据分类**（前置过滤，均不入领域对象清单）：

| 数据项 | 类别 | 处理方式 |
|--------|------|---------|
| 环境列表/任务列表筛选参数（envType/status） | P1 查询条件 | 接口查询参数，服务端校验（R-14） |
| 服务运行/自启状态、已装版本、通道可用性 | P2 展示数据 | 实时探测投影（响应即消亡，与 systemd_service 投影同构） |
| 任务在途标记（列表条目上） | P2 展示数据 | 由在途任务台账派生 |
| 安装/卸载/服务操作在途状态（按钮 loading） | P3 UI 交互状态 / P7 计算中间值 | 任务创建同步返回 + 任务详情轮询呈现；服务操作同步终态 |
| 命令输出尾部 ring 缓冲（执行中） | P7 计算中间值 | 执行链内存态，终态定格落库（output_tail 字段） |
| 引导随机强密码明文（v1.2） | P7 计算中间值 / 敏感 | 仅引导建连与加密内存态使用，加密后密文落 db_instance.cred_cipher（db-mgmt 侧），明文不落库不入审计不入输出（⚠️A12 零回显）；引导结果摘要（bootstrap_note）不含密码 |
| 环境目录条目（七环境静态清单+包名/单元名映射） | 编译期常量（技术载体） | 非用户可配数据，不入领域对象清单（与枚举池同性质，env_type 枚举入池承载标识语义） |

**假设与待对齐清单**：

| 编号 | 内容 | 状态 |
|------|------|------|
| 待对齐-1 | env_install_task 表结构 DDL（输出尾部字段形态、索引）；任务输出尾部行数上限与包操作超时上限具体值 | D3 拍板 |
| 待对齐-2 | 官方仓库源代配的落点路径与文件名约定；需代配的环境-发行版组合清单（编译期映射表内容） | D3 定 |
| 待对齐-3 | 环境列表页在途任务的呈现节奏（轮询间隔/手动刷新组合） | D4 定 |
| 待对齐-4（v1.2） | 安装后配置就绪引导细则：BootstrapAfterInstall OHS 签名与引导结果结构、各引擎引导通道网关方法、本机默认 identity 与 socket 候选、服务就绪等待上限与随机密码长度、bootstrap_status/note 字段与迁移、env_bootstrap/instance_bootstrapped 审计动作码 | D3 拍板（api-env-mgmt + api-database-mgmt） |
| 说明 | ⚠️A1~A15 为 D0 增量澄清拍板（A1~A7 2026-11-16；A8~A10 W1 修复轮；A11~A15 本轮 2026-11-17，记录于 BA/AA 附录与 d0-clarification-2026-11-17-bootstrap.md），本 PRD 沿用为 FR 依据；Gate 1 呈现全文供用户复核，改判需同步 BA/AA/FL/PRD 四文档 | 已登记（D0/D1） |
| 说明 | BA ⚠️A6「列出警示（Docker 有容器/MySQL 有实例/nginx 有站点时列出）」在本 PRD 落地为「本域服务实况警示+关联模块静态影响文案」，动态跨模块计数显式 OUT-9（无跨模块硬调用纪律）；如需动态计数二期与关联模块 OHS 联动设计 | 本 PRD 细化（Gate 1 呈现） |

## 附录：PRD 完备性检查报告

**功能类型**：主数据/台账混合型——环境全景为实况投影台账（只读）+ 环境变更任务为单据型（四态状态机、异步执行、无审批）；无集合导出；无外部系统集成（apt/dnf·yum/systemd 为本机系统组件）。

### 第一层：通用必检项 G1-G5

| # | 检查项 | 结果 | 依据 |
|---|--------|------|------|
| G1 | 业务目标 | ✅ | 第一部分：环境本体供给能力补位（软件包层）、四层分界、安全纪律 |
| G2 | 功能范围 | ✅ | 第二部分：IN 7 项（v1.2 +安装后配置就绪）/ OUT 12 项（v1.2 +PHP 多版本/MySQL 升级二期登记） |
| G3 | 角色定义 | ✅ | 第三部分：2 角色 + 访客不涉及声明 |
| G4 | 领域对象 | ✅ | domain-model.md（v1.22：env_install_task 聚合根 +引导结果字段 bootstrap_status/note；环境条目投影 + 发行版映射不变；引导为聚合根后置机制非新聚合根） |
| G5 | 对象关系 | ✅ | domain-model.md「与上级关系」列；无子流程（任务状态机为聚合根本体）；无关联领域对象 |

### 第二层：按类型必检项 T

| # | 检查项 | 适用性 | 结果 | 依据 |
|---|--------|--------|------|------|
| T1 | 业务字段 | 适用 | ✅ | domain-model.md 领域对象清单（任务全字段/条目投影字段） |
| T1a | 被引用关系 | 适用 | ✅ | 第七部分：软引导被消费 + 消费方视角四条（v1.2 +database-mgmt BootstrapAfterInstall OHS） |
| T2 | 状态流转 | 适用 | ✅ | 第六部分：env_task_status 四态机 + 4 项辨析 |
| T3 | 审批规则 | 无审批语义 | ➖ N/A | 管理员即时发起异步执行，无授权环节 |
| T4 | 查询筛选条件 | 适用 | ✅ | 第七部分：环境全景/任务筛选/单任务三形态 |
| T5 | 导出需求 | 无导出 | ➖ N/A | 第七部分 |
| T6 | 对接系统 | 无外部系统 | ➖ N/A | 包管理器/systemd 为本机组件（第九部分声明） |
| T9 | 输入输出规格 | 适用 | ✅ | FR-01~07 的 I/O 描述（任务输出尾部/实况投影/终态反馈/引导结果 bootstrap_status/note） |

### 第三层：加分项 N1-N4

| # | 检查项 | 结果 | 说明 |
|---|--------|------|------|
| N1 | 非功能性要求 | ✅（部分锁定） | 七条目/实时探测/量级/台账全量保留/引导全注入化已锁定；上限值与映射表内容、引导等待上限/密码长度 D3 定（待对齐-1/-2/-4） |
| N2 | 异常场景 | ✅ | 16 个异常分支（v1.2 +引导失败/引导通道建连失败/用户已有配置跳过/注册验证失败/非数据库无引导 5 项） |
| N3 | 业务规则细节 | ✅ | 第五部分规则汇总（v1.2 +⚠️A11~A15 + 引导凭据安全/幂等）+ 事务补充（引导结果与终态同事务） |
| N4 | 历史需求 | ✅ | 任务台账即历史 + 审计追溯 |

### 检查结论

```
## PRD 完备性检查 — 通过

功能类型：主数据/台账混合型（环境实况投影台账 + 环境变更任务单据 + 安装后配置就绪后置机制）
检查项：G 5/5 通过；T 适用项 6/6 通过（T1/T1a/T2/T4/T9 + 幂等并入 N3，T3/T5/T6 N/A 有据）；N 4/4
v1.2 增量复核：FR-07 安装后配置就绪（机制型，无新状态机——引导结果为独立维度并入 T2 辨析）；G2 IN 7/OUT 12；T1a +db-mgmt OHS 消费；N2 +5 引导异常分支；领域对象 env_install_task +引导结果字段（domain-model v1.22）；枚举 +bootstrap_status 入池

可进入 API 设计阶段（D3，本技能 d3-api.md）
```

## 更新记录

| 版本 | 日期 | 更新说明 |
|------|------|---------|
| v1.2 | 2026-11-17 | 安装后配置就绪增量轮（d0-clarification-2026-11-17-bootstrap.md，D0 拍板 Q1~Q4）：①G1 +产品定调「装完即配置就绪」与 DBM_005 实证痛点、引导边界（触发/呈现归 env-mgmt，建账户/注册归 db-mgmt OHS）；②G2 IN +1（IN-7 安装后配置就绪 C-14-04 机制型）/ OUT +2 二期方向登记（PHP 多版本 sury 源 / MySQL 升级 mysql-apt-config，本轮 OUT）+OUT-8 引导例外注；③FR-07 安装后配置就绪（服务就绪→建账户/设密码〔MySQL dougong@localhost ALL WITH GRANT OPTION socket root auth_socket 引导通道 / Redis CONFIG SET+REWRITE / MongoDB localhost exception admin root〕→TCP 验证注册→幂等保守→独立结果维度）+FR-03 衔接/FR-05 引导结果字段；④第五部分 +⚠️A11~A15 + 引导凭据安全/幂等推导规则；⑤T2 辨析 +bootstrap_status 结果维度（非新状态机）；⑥T4 任务详情+引导字段/T1a +db-mgmt OHS 消费；⑦N1 引导全注入化（CI 第 7 起预防）/N2 +5 引导异常分支/N3 引导结果与终态同事务；⑧第九部分集成 +OHS 消费 db-mgmt/过程数据 +引导密码明文 P7 敏感/待对齐-4；⑨完备性增量复核通过（G 5/5、T 适用 6/6、N 4/4）；domain-model v1.22（env_install_task +bootstrap_status/note）；枚举 +bootstrap_status 入池 |
| v1.1 | 2026-11-17 | W1 实机冒烟缺陷修复轮（补录 changelog，此前正文已升 v1.1 而行缺失）：env_type 六→七（+MongoDB，用户点名）；⚠️A8 单元名规范全名/⚠️A9 探测三态+多候选+dpkg Status 字段/⚠️A10 代配补真（os-release/apt keyring/yum repo/次序前移）；FR-01~06 同步七环境与三态/多候选口径；MongoDB 8.0 轨道 ⚠️ |
| v1.0 | 2026-11-16 | 初始生成：基于 D0 增量澄清拍板（⚠️A1~A7）与 D1 三文档 v1.0 成文；完备性检查通过（G 5/5、T 适用 6/6、N 4/4）；领域建模落盘 domain-model.md v1.19（支撑域判定 + 1 聚合根 env_install_task〔四态任务状态机落 SQLite〕+ 1 域内对象-实体 环境条目投影〔零台账，包管理器/systemd 实况同 site 投影哲学〕+ 1 值对象 发行版映射〔编译期〕）；枚举 4 项新增定稿入池（env_type/env_task_action/env_task_status/package_manager_family）；D2 落定：任务台账持久化/重启恢复 running→failed/卸载风险提示两层口径（本域实况+静态警示，动态计数 OUT-9）；待对齐 3 项移交 D3/D4 |
| v1.1 | 2026-11-17 | W1 实机冒烟缺陷修复轮（v0.1.2 用户服务器 cheerful-bliss-1 实测反馈 3 项）：①缺陷 1（enable nginx 报 DBus InvalidArgs）→ ⚠️A8 systemd 单元名规范全名拍板；②缺陷 2（Docker/MySQL/Redis/PHP-FPM 卡片误显「探测不可用」）→ ⚠️A9 三态如实区分（dpkg Status 字段判定 + LC_ALL=C + 包名不在数据库=未装非错误）+ ⚠️A7 映射表多候选包名（docker.io/docker-ce、mysql-server/mysql-community-server 覆盖官方源自装形态）；③需求 3（用户点名缺 MongoDB）→ ⚠️A1 六→七环境目录（env_type +mongodb 三方 S8 同值同步：枚举池/api DDL CHECK/PageSpec/前端）+ ⚠️A10 第三方源代配补真（v1.0 D-06 声明空转，本轮实现：os-release 判定 + apt keyring/dnf repo 文件写入 + env_repo_configured 审计；MongoDB 8.0 轨道 ⚠️待主会话终确认）；N2 异常分支 +2、辨析表 +1（探测不可用两义）；N1 补可注入测试纪律（CI 环境耦合预防第 5 起） |
