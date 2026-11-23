# 斗拱面板 环境管理（env-mgmt）业务架构

| 属性 | 值 |
|------|-----|
| 版本 | v1.4 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-23（宝塔式重组增量轮，ADR-033：Node 环境随装 PM2 与 Node 就绪度读契约；2026-11-20 v1.3 Python 环境增量轮，app-mgmt 集成联动，d0-clarification-app-mgmt-2026-11-20.md） |

## 第一部分：文档概述

### 1.1 背景与目标

环境管理（env-mgmt）是斗拱面板的运行环境供给模块（v0.1.2 用户反馈第 4 项，/pipeline 增量轮新增模块，参照宝塔「软件商店」按本项目定位裁剪）：面板此前只能「接管已有环境」（site-mgmt 接管 nginx、database-mgmt 纳管已运行数据库实例、docker-mgmt 管理已有 daemon 的容器），缺失「从无到有安装环境本体」的能力。本模块补位软件包层：经发行版包管理器（apt / dnf·yum 双系）安装、升级、卸载八类常用运行环境（nginx、Apache、Docker、MySQL、Redis、PHP-FPM、MongoDB、Node.js，v1.1 扩 MongoDB，v1.2 扩 Node.js），并提供已装环境的版本/服务状态观测与服务启停自启入口。**v1.2 Node 环境增量**（db-tools 集成联动，D0 拍板 2026-11-19，d0-clarification-db-tools-integration-2026-11-19.md Q1）：db-tools 集成 mongo-express（Node 承载）需 Node 运行时，本模块目录 +Node.js 条目——NodeSource 官方源三系代配（⚠️A10 同构）+ LTS 大版本轨道钉死（⚠️A16），npm 随 Node 包供给；Node 为运行时型环境（无常驻服务单元），服务启停自启入口对其不适用（⚠️A17）。**v1.1 安装后配置就绪增量**：产品定调「好面板的标准是装完即配置就绪」——实证痛点（DBM_005 诊断轮终裁）为 env-mgmt 经 apt 装完 MySQL 只有 root@localhost（auth_socket 无密码），面板走 TCP+密码连不上，用户无法自助建第一个账户，**装完 ≠ 可用**。本轮补位「安装后自动引导配置到面板可直接治理」：数据库类环境（MySQL/Redis/MongoDB）安装成功后自动建管理账户/设随机强密码并注册实例与凭据到 database-mgmt（装完即可管）。业务输入来源：v0.1.2 用户反馈（D0 增量澄清拍板 2026-11-16——范围=六环境 + 双包管理器系）、v1.1 用户点名补 MongoDB、安装后配置就绪增量（D0 增量澄清拍板 2026-11-17，d0-clarification-2026-11-17-bootstrap.md）、Node 环境增量（db-tools 集成 D0 拍板 2026-11-19，d0-clarification-db-tools-integration-2026-11-19.md Q1）、项目支持矩阵（install.sh：Debian 11+ / Ubuntu 20.04+ / RHEL 9 系）、既有模块边界（database-mgmt=实例层、docker-mgmt=容器层、service-mgmt=通用 systemd 单元层、site-mgmt=站点层、db-tools=工具集成层〔本轮新增，消费本模块环境实况与 Node/PHP-FPM 供给〕）。

本业务架构设计目标：

1. 以 L1-L4 层级刻画「观测 → 供给（含安装后配置就绪）→ 运行」的环境全生命周期主线；
2. 将安全纪律落到业务口径（apt/yum 包操作最小权限、幂等、审计，沿用 R-01~R-18 与 SEC-GATE；外部命令一律字面量参数数组 R-08；引导凭据随机强密码密文落库零回显）；
3. 明确与既有模块的边界与交叠（环境本体/实例/容器/通用单元四层分界），为关联模块提供安装引导回链；安装后配置就绪经 database-mgmt 进程内 OHS 联动（实例与凭据归实例层治理）；
4. 为应用架构（AA）与功能清单（FL）提供唯一业务依据。

> ⚠️假设（本设计新增，承接 D0 增量澄清与边界裁定，如裁剪需同步三文档）：
>
> - ⚠️A1 目录范围口径：环境目录为面板内置静态清单，八条目——nginx、Apache（httpd）、Docker、MySQL、Redis、PHP-FPM、MongoDB、Node.js（D0 增量澄清拍板 2026-11-16 六条目；v1.1 W1 修复轮用户点名 +MongoDB；v1.2 db-tools 集成联动 +Node.js）；每条目含显示名、描述与发行版族包名映射（⚠️A7）；目录扩展（更多环境）走面板版本迭代。
> - ⚠️A2 双包管理器口径：Debian 系（Debian/Ubuntu）经 apt，RHEL 系（RHEL/CentOS Stream/Rocky/Alma 9+）经 dnf（无 dnf 回退 yum）；运行时探测择一，两者皆无（或非 Linux/无 systemd）时环境变更能力如实不可用（报错提示，不虚构）；个别环境在特定发行版需第三方官方仓库（典型：RHEL 系 Docker CE 不在 AppStream 内）时，由面板写入发行版仓库源配置（/etc/apt/sources.list.d/ 或 /etc/yum.repos.d/）后再安装，仓库配置写入同样审计留痕（D0 拍板：apt+dnf/yum 双系）。
> - ⚠️A3 异步任务口径：安装/卸载为异步变更任务（包安装可达数分钟，同步等待必超时）：任务状态 pending → running → succeeded / failed；同环境同时至多一个在途任务（互斥）；任务失败原因与命令输出尾部如实呈现；任务取消 MVP 不做（OUT——包管理器中断态不安全）。
> - ⚠️A4 升级口径：已装环境再次执行安装=升级语义（先刷新包索引 apt update / dnf makecache，再安装仓库最新版，幂等）；「检测可升级版本并提示」MVP 不做（OUT 二期）；降级/指定版本安装不做（OUT）。
> - ⚠️A5 服务联动口径：安装完成后不强制 start/enable（尊重发行版包脚本默认行为），服务实况经环境实况探测如实呈现；启停/自启经环境服务运维入口显式操作。**v1.1 引导路径细化（⚠️A15）**：数据库类环境安装后配置引导可为配置而启动服务（幂等），enable 仍不强制。
> - ⚠️A6 卸载口径：卸载=包 remove（保留系统配置与数据目录，安全保守），purge 与数据目录清理二期 OUT；卸载前强确认（呈现关联模块在用风险提示——如 Docker 有运行容器、MySQL 有 database-mgmt 纳管实例、nginx 有已纳管站点时列出警示，知情决策不硬阻断），卸载动作审计留痕。
> - ⚠️A7 包名与单元映射口径：每环境每发行版族的包名、systemd 服务单元名、版本查询方式经目录条目内置映射表承载（v1.1 起包名=有序候选数组、单元名=规范全名，详 ⚠️A8/A9 与 prd-env-mgmt.md），映射表为面板编译期数据非用户可配。
> - ⚠️A8 单元名规范全名口径（v1.1 W1 修复轮）：systemd 单元名一律含 .service 后缀——EnableUnitFiles/DisableUnitFiles 的 DBus 契约无后缀补全（无后缀名直接 InvalidArgs，W1 实机缺陷 1 根因）；php-fpm apt 系版本化单元（phpX.Y-fpm.service）经 ListUnits 前缀扫描解析。详见 api-env-mgmt.md D-13。
> - ⚠️A9 已装探测三态口径（v1.1 W1 修复轮）：通道级不可用 / 条目级探测真错误（installed=null）/ 未装（installed=false，含包名不在包数据库形态）三态如实区分；dpkg 以 Status 字段判定 + LC_ALL=C；候选包名数组任一命中即已装。详见 api-env-mgmt.md D-14。
> - ⚠️A10 第三方源代配补真口径（v1.1 W1 修复轮）：按 /etc/os-release 判定发行版与代际；MongoDB 三系 + RHEL 系 docker-ce 需代配（apt keyring HTTPS 下载 / yum repo 文件），写入审计 env_repo_configured；次序前移至索引刷新之前；矩阵外组合任务 failed 如实。详见 api-env-mgmt.md D-06。
> - ⚠️A11 安装后配置就绪口径（v1.1 本轮新增，D0 拍板 2026-11-17）：数据库类环境（MySQL/Redis/MongoDB）安装/升级成功后，worker 自动执行 post-install bootstrap——建管理账户/设随机强密码 + 自动注册实例与凭据到 database-mgmt（装完即可管）；PHP/nginx/Apache/Docker/Node 装完即用无引导（Node 为 v1.2 新增运行时型，⚠️A17）；卸载无引导。引导为独立后置步骤，结果经任务台账呈现（⚠️A14）。
> - ⚠️A12 引导通道与账户口径（v1.1 本轮新增，D0 拍板 Q1/Q2/Q4）：MySQL 经 unix socket 以 root auth_socket 免密建连（引导通道，区别于 TCP+密码验证通道）建 **dougong@localhost**（**ALL PRIVILEGES ON *.* WITH GRANT OPTION**，不动 root 的 auth_socket）；Redis 经 **CONFIG SET requirepass + CONFIG REWRITE**（仅未设置时写）+ bind 本机校验；MongoDB 经 localhost exception 建 admin 库 root 角色用户。随机强密码 crypto/rand，AES-256-GCM 密文落库零回显。
> - ⚠️A13 引导幂等与保守口径（v1.1 本轮新增）：目标实例已配置凭据 / MySQL 账户已存在 / Redis requirepass 已设 / MongoDB 已有用户 → 保守跳过不覆盖（用户已有配置不覆盖，conffile 保守先例）；已装/已配置环境不重复引导；引导动作审计留痕。
> - ⚠️A14 引导失败语义口径（v1.1 本轮新增，D0 拍板 Q3）：独立引导结果维度——任务 status=包操作结果，新增 bootstrap_status（succeeded/failed/skipped，NULL=无引导步骤）+ bootstrap_note；安装成功但引导失败 → status=succeeded + bootstrap_status=failed，如实呈现「已安装·引导失败」与原因，恢复=重新安装/升级（幂等重跑引导）。不混淆「已装」与「可治理」。
> - ⚠️A15 引导服务联动细化（v1.1 本轮新增，⚠️A5 细化）：引导路径可为配置而启动数据库服务（幂等 start，等待引擎就绪有界重试），enable 仍不强制；非引导环境维持 ⚠️A5 不强制 start/enable。
> - ⚠️A16 Node 环境供给口径（v1.2 本轮新增，db-tools D0 拍板 Q1）：Node.js 经 NodeSource 官方源安装——三系均需代配（apt keyring HTTPS 下载 / yum repo 文件，⚠️A10 同构，写入审计 env_repo_configured）；LTS 大版本轨道钉死（NodeSource 仓库轨道为面板编译期选定，具体 LTS 版本号归 D3 定稿；⚠️A4 升级语义不变=轨道内仓库最新版）；包名 nodejs（三系同），npm 随 Node 包供给不单独安装；版本查询经 node --version（R-08 参数数组）；Python 环境本轮不做（OUT，归未来）。
> - ⚠️A17 运行时型环境无服务口径（v1.2 本轮新增）：Node.js 为运行时型环境（无常驻 systemd 服务单元）——目录条目单元映射为空，服务运行/自启状态字段不适用（null 如实呈现，不虚构），服务启停/自启入口对 Node 条目禁用（呈现「运行时环境无常驻服务」说明）；⚠️A5/A11 服务联动与安装后引导对 Node 不适用（Node 装完即用，无引导，同 PHP/nginx/Apache/Docker 判）。
> - ⚠️A18 Python 环境供给口径（v1.3 本轮新增，app-mgmt D0 拍板 Q2）：Python 经发行版仓库安装（无第三方源代配需求——发行版源自带，区别于 Node/MongoDB；⚠️A7 有序候选数组：apt 系 python3/python3-venv/python3-pip 三包候选〔Debian 系 venv 支撑需 python3-venv 单独包〕、rpm 系 python3〔venv/pip 自含〕）；版本查询经 python3 --version（R-08 参数数组）；Python 为运行时型环境（⚠️A17 同构：单元映射空/服务态 null/服务操作入口禁用/装完即用无引导）；venv 与 pip 能力位纳入依赖就绪度读契约（app-mgmt 创建前置消费，⚠️A2）；具体候选包名矩阵归 D3 定稿。
> - ⚠️A19 Node 环境随装 PM2（v1.4 本轮新增，ADR-033，d0-clarification-baota-restructure-2026-11-23.md，Gate 1 用户拍板「Node 项目像宝塔那样用 PM2 托管」）：安装或升级 Node 环境时，连带用 npm 全局安装 PM2 并注册 PM2 开机自启（Linux 上由 PM2 生成启动单元，开机恢复已保存的进程列表）。两步都是尽力而为：失败只记入任务输出，不改变 Node 环境安装的终态——Node 项目创建时会因缺 PM2 被拒并引导回环境管理升级补装（与 v1.3 的 pip 补装同一机制）。卸载 Node 不动 PM2 进程列表（面板无法判断是否仍有项目依赖），仅如实提示。PM2 是 Node 环境的随装件，不是新的环境条目：目录范围仍为九环境，L2/L3/L4 集合不变。Node 就绪度读契约相应扩展（node / npm / PM2 是否可用 + node 版本），供 app-mgmt 创建 Node 项目做前置校验。

### 1.2 文档定位

| 定位 | 说明 |
|------|------|
| 本文档做什么 | 描述环境管理业务域内 L2/L3/L4 的层级结构和业务关系，不涉及系统实现 |
| 本文档不做什么 | 不划分应用模块、不标注用户类型、不涉及技术选型和系统架构 |

### 1.3 层级定义

| 层级 | 名称 | 说明 |
|------|------|------|
| L1 | 业务域 | 系统所覆盖的业务范围 |
| L2 | 业务阶段 | 业务的时间阶段划分 |
| L3 | 关键业务 | 各阶段内的关键业务能力 |
| L4 | 业务活动 | 关键业务下的具体业务活动 |

### 1.4 范围

| 范围 | 说明 |
|------|------|
| 业务域 | 环境管理（env-mgmt）：环境目录与实况、环境安装与变更（含安装后配置就绪）、环境服务运维 |
| 业务阶段 | 环境观测阶段、环境供给阶段、环境运行阶段 |

边界声明（OUT，沿 D0 增量澄清结论）：源码编译安装不做（仅发行版仓库包，含个别官方仓库源配置）；**Python 多版本共存（pyenv/自选版本）不做（系统 python3 单版本，v1.3 Python 兑现登记：Python 环境本体已由 v1.2 OUT 转为本轮供给，⚠️A18；沿 PHP 多版本二期先例）**；**PHP 多版本并存（sury.org/ondrej PPA 5.6~8.6 coinstallable，各版本独立 phpX.Y-fpm）二期另起轮**（v1.1 登记）；**MySQL 版本升级路径（mysql-apt-config 单版本原地升级，不支持降级/禁跳系列/须备份）二期另起轮**（v1.1 登记）；「应用市场/插件商店」形态不做（目录内置九环境，按本项目轻量定位裁剪宝塔软件商店面）；任务取消不做（⚠️A3）；purge 卸载与数据目录清理二期（⚠️A6）；可升级版本检测二期（⚠️A4）；Windows/无 systemd 环境不支持（install.sh 支持矩阵外，如实报错）；运行环境内部配置管理不做（nginx 站点配置归 site-mgmt、MySQL 实例管理归 database-mgmt、容器归 docker-mgmt——本模块止步软件包层；**安装后配置就绪的「建账户/设密码/注册实例与凭据」归 database-mgmt 实例层，本模块经 OHS 触发并呈现引导结果**，⚠️A11/A12）。

## 第二部分：业务架构

> 本章按 L2→L3→L4 逐层描述业务架构。每层有其概述和业务说明。

### 本架构概览

环境管理遵循「看得见实况 → 装得上环境 → 管得住服务」的业务主线，按环境供给生命周期划分三个业务阶段：

- **环境观测阶段**：环境目录呈现（内置九环境条目，v1.2 +Node.js，v1.3 +Python）与环境实况探测（包管理器查询已装版本、systemd 查询服务状态、通道可用性探测；运行时型环境 Node/Python 服务态不适用，⚠️A17/A18）——是供给与运行两阶段的共同数据底座；
- **环境供给阶段**：经包管理器安装/升级/卸载环境本体，异步任务化执行与跟踪，安全纪律（强确认、审计、幂等）贯穿；**数据库类环境安装成功后自动执行安装后配置就绪引导（建管理账户/设密码 + 注册实例与凭据到 database-mgmt，装完即可管，⚠️A11~A15）**；
- **环境运行阶段**：已装环境的服务启停与自启管理（复用 systemd 通道），衔接 service-mgmt 的通用单元治理。

三阶段为顺序依赖关系：观测产出实况 → 供给改变实况 → 运行作用于供给产物；观测同时为运行阶段提供状态依据。

### 2.1 环境观测阶段

**业务说明**：环境目录与实况的数据底座：面板内置九环境静态目录（⚠️A1，v1.2 +Node.js，v1.3 +Python），叠加运行时实况探测（已装/版本/服务状态/通道可用性）形成环境全景；观测结果同时是供给动作的前置依据（未装才可安装、已装才可卸载/启停）与关联模块的引导数据源（Docker 未装 → docker-mgmt 引导态提示可安装；PHP-FPM/Node 未装 → db-tools 依赖缺失引导，v1.2；Python 未装 → app-mgmt 创建拒绝引导，v1.3）。

##### 2.1.1 环境目录与实况

**业务说明**：内置环境目录与运行时实况的合并呈现能力。所属 L2：环境观测阶段。目录为面板版本内置（非用户可配），实况每请求实时探测（不缓存不虚构——nginx 未检出返回未安装，与 site-mgmt 投影真实性同哲学）。

###### 2.1.1.1 环境目录呈现

**业务说明**：呈现内置九环境条目（v1.2 +Node.js，v1.3 +Python）：显示名、描述、关联模块提示（Docker → docker-mgmt、MySQL → database-mgmt、nginx/PHP-FPM → site-mgmt、PHP-FPM/Node → db-tools、Python → app-mgmt 等，安装后即打通对应模块）；目录随面板版本迭代扩展。所属 L3：环境目录与实况。

###### 2.1.1.2 环境实况探测

**业务说明**：逐环境探测实况：已装状态与版本（经包管理器查询，⚠️A7 映射；Node 版本经 node --version 查询，⚠️A16；Python 版本经 python3 --version 查询且 venv/pip 能力位一并探测，⚠️A18）、服务运行状态与自启状态（经 systemd 单元查询；运行时型环境 Node/Python 无服务单元，服务态字段不适用 null 如实呈现，⚠️A17/A18）、包管理器通道可用性（apt/dnf·yum 探测择一，⚠️A2——通道不可用时变更能力如实标注不可用）；探测为纯读操作。所属 L3：环境目录与实况。

### 2.2 环境供给阶段

**业务说明**：环境本体的安装/升级/卸载能力：经发行版包管理器执行（⚠️A2 双系），异步任务化（⚠️A3），升级与安装同通道幂等（⚠️A4），卸载强确认保守执行（⚠️A6）；**数据库类环境安装成功后自动执行安装后配置就绪引导（⚠️A11~A15）**；全部变更动作审计留痕（R-03），外部命令一律参数数组（R-08），包操作以最小权限执行。

##### 2.2.1 环境安装与变更

**业务说明**：环境供给的变更主链。所属 L2：环境供给阶段。安装/升级/卸载三个动作共用异步任务通道与审计纪律；任务产出（命令输出尾部、失败原因、引导结果）如实呈现，不虚构成功。

###### 2.2.1.1 环境安装与升级

**业务说明**：安装目录环境：先刷新包索引再经包管理器安装（已装环境同动作为升级到仓库最新版，幂等，⚠️A4）；需第三方官方仓库的环境由面板先配置仓库源再安装（⚠️A2，仓库写入审计）；安装完成后服务状态尊重包脚本默认行为（⚠️A5；数据库类引导路径可启动服务，⚠️A15），实况经观测阶段如实反映；**数据库类环境安装成功后衔接安装后配置就绪引导（2.2.1.4）**。所属 L3：环境安装与变更。

###### 2.2.1.2 环境卸载

**业务说明**：卸载已装环境：强确认交互（呈现关联模块在用风险提示——Docker 有容器/MySQL 有纳管实例/nginx 有站点时列警示，知情决策不硬阻断，⚠️A6）；执行包 remove（保留系统配置与数据目录）；卸载后环境条目回到未装态。所属 L3：环境安装与变更。

###### 2.2.1.3 环境变更任务跟踪

**业务说明**：异步任务的观测与结果确认：任务状态（pending → running → succeeded / failed，⚠️A3）、执行动作、命令输出尾部（排障）、**引导结果（bootstrap_status/note，⚠️A14）**；同环境在途任务互斥（重复提交如实拒绝）。所属 L3：环境安装与变更。

###### 2.2.1.4 安装后配置就绪

**业务说明**：数据库类环境（MySQL/Redis/MongoDB）安装/升级成功后的自动配置引导（⚠️A11，独立后置步骤）：经引导通道建管理账户/设随机强密码（MySQL 经 unix socket root auth_socket 免密建 dougong@localhost 全权账户；Redis CONFIG SET requirepass+REWRITE；MongoDB localhost exception 建 admin root 用户，⚠️A12），并自动注册实例与凭据到 database-mgmt（装完即可管）；幂等保守——已配置/已存在则跳过不覆盖（⚠️A13）；引导结果独立呈现（安装成功但引导失败如实标注，恢复=重新安装/升级，⚠️A14）；引导动作审计留痕、密码密文落库零回显。PHP/nginx/Apache/Docker/Node 装完即用无引导（Node 为 v1.2 新增运行时型，⚠️A17）。所属 L3：环境安装与变更。

### 2.3 环境运行阶段

**业务说明**：已装环境的服务层运维：启停/重启/自启管理（经 systemd 单元通道，与 service-mgmt 共用同一通道资产）；操作对象限定有服务单元的目录环境——运行时型环境 Node 无服务单元，服务运维入口对其不适用（⚠️A17，v1.2）；细粒度单元治理（全部系统服务清单）仍归 service-mgmt，本域仅面向目录环境的直达入口。

##### 2.3.1 环境服务运维

**业务说明**：已装环境的服务管控入口。所属 L2：环境运行阶段。操作对象限定目录环境对应的服务单元（⚠️A7 映射），结果如实反馈。

###### 2.3.1.1 环境服务启停与自启

**业务说明**：对已装环境执行服务启动/停止/重启与自启开关：经 systemd 通道生效，结果与失败原因如实呈现；操作对象限定有服务单元的目录环境（Node 运行时型不适用，⚠️A17）；操作审计留痕（R-03）。所属 L3：环境服务运维。

## 第三部分：业务架构树

> **由第二部分内容对应生成，第二部分任何变更必须同步更新本视图。** 结构：L1 > L2 > L3 > L4。

```
环境管理（env-mgmt）
├── 环境观测阶段
│   └── 环境目录与实况
│       ├── 环境目录呈现
│       └── 环境实况探测
├── 环境供给阶段
│   └── 环境安装与变更
│       ├── 环境安装与升级
│       ├── 环境卸载
│       ├── 环境变更任务跟踪
│       └── 安装后配置就绪
└── 环境运行阶段
    └── 环境服务运维
        └── 环境服务启停与自启
```

## 附录

### 附录A：L3 业务依赖关系

> 描述 L3 关键业务之间的上下游依赖关系，为后续应用架构中模块调用分析提供依据。

| 上游 L3 | 依赖方向 | 下游 L3 | 依赖说明 |
|---------|---------|---------|---------|
| 环境目录与实况 | → | 环境安装与变更 | 实况（已装态/通道可用性）是安装/卸载动作的前置依据与按钮态数据源 |
| 环境安装与变更 | → | 环境目录与实况 | 变更任务完成后实况刷新（安装结果经观测反映） |
| 环境目录与实况 | → | 环境服务运维 | 服务运行/自启状态是启停入口的状态依据 |
| 环境目录与实况 | → | docker-mgmt（他域）/ database-mgmt（他域）/ site-mgmt（他域） | 环境未装实况为关联模块引导态提供数据（软引导，无跨模块硬调用） |
| 环境目录与实况 | → | db-tools（他域，v1.2 新增被消费） | 工具依赖就绪度数据源：PHP-FPM/Node/nginx 已装态与版本（进程内读契约，单向）；PHP-FPM/Node 未装实况为 db-tools 安装拒绝引导提供数据 |
| 环境安装与变更 | → | db-tools（他域，v1.2 新增供给） | Node 环境供给（NodeSource 三系代配 + LTS 轨道钉死，⚠️A16）为 db-tools mongo-express 集成的运行前置；db-tools 不代装环境，依赖缺失经软引导回本模块 |
| 环境目录与实况 | → | app-mgmt（他域，v1.3 新增被消费） | 应用依赖就绪度数据源：Python 运行环境就绪度（python3/venv/pip 已装态与能力位，进程内读契约，单向）；Python 未装实况为 app-mgmt 创建拒绝引导提供数据 |
| 环境安装与变更 | → | app-mgmt（他域，v1.3 新增供给） | Python 环境供给（发行版仓库包，⚠️A18）为 app-mgmt 应用创建的运行前置；app-mgmt 不代装环境，依赖缺失经软引导回本模块 |
| 环境目录与实况 | → | app-mgmt（他域，v1.4 新增被消费） | Node 项目依赖就绪度数据源：Node 运行环境就绪度（node / npm / PM2 是否可用 + node 版本，进程内读契约，单向）；缺项时 app-mgmt 创建 Node 项目拒绝并引导回本模块 |
| 环境安装与变更 | → | app-mgmt（他域，v1.4 新增供给） | Node 环境连带 PM2 的供给（⚠️A19）为 app-mgmt Node 项目托管的运行前置；app-mgmt 不代装，依赖缺失经软引导回本模块 |
| 环境安装与变更 | → | database-mgmt（他域） | **安装后配置就绪（v1.1，⚠️A11/A12）**：数据库类环境安装成功后经进程内 OHS（BootstrapAfterInstall）触发实例与凭据注册——建账户/设密码/注册归 database-mgmt 实例层，本模块消费其 OHS（单向依赖，与 cron-backup 消费 DumpStream 同构）；引导结果回传任务台账呈现 |

### 附录B：业务术语表

> 统一业务术语定义，后续领域对象命名以此为准。

| 术语 | 定义 | 备注 |
|------|------|------|
| 环境（environment） | 面板可供给的运行环境软件包条目：nginx/Apache/Docker/MySQL/Redis/PHP-FPM/MongoDB/Node.js/Python 九类 | ⚠️A1 目录范围（v1.3 九值） |
| 运行时型环境（runtime environment） | 无常驻 systemd 服务单元的环境条目（Node.js/Python）：服务态字段不适用（null），服务启停自启入口禁用，装完即用无引导 | ⚠️A17/A18（v1.2 新增，v1.3 +Python） |
| NodeSource 源 | Node.js 官方第三方仓库（apt keyring / yum repo，三系均需代配）；LTS 大版本轨道钉死，npm 随 nodejs 包供给 | ⚠️A16（v1.2 新增）；⚠️A10 代配同构 |
| 安装后配置就绪（post-install bootstrap） | 数据库类环境安装成功后自动建管理账户/设随机强密码并注册实例与凭据到 database-mgmt，使面板装完即可治理 | ⚠️A11~A15；独立后置步骤 |
| 引导通道（bootstrap channel） | 区别于 TCP+密码验证通道的初始建连通道：MySQL unix socket root auth_socket 免密 / Redis 无 requirepass 免认证 / MongoDB localhost exception 免认证 | ⚠️A12；仅用于建账户/设密码 |
| 管理账户（governance account） | 引导创建的面板治理账户：MySQL dougong@localhost（ALL WITH GRANT OPTION）/ Redis requirepass / MongoDB admin root 用户；随机强密码密文落库零回显 | ⚠️A12；不动 root auth_socket |
| 引导结果（bootstrap status） | 独立引导结果维度：succeeded/failed/skipped（NULL=无引导步骤）；任务 status 仍为包操作结果 | ⚠️A14；不混淆已装与可治理 |
| 环境目录 | 面板内置的环境静态清单（显示名/描述/关联模块/发行版包名映射） | 编译期数据非用户可配（⚠️A7） |
| 环境实况 | 运行时探测结果：已装/版本/服务状态/自启/通道可用性 | 每请求实时探测不缓存 |
| 包管理器通道 | apt（Debian 系）或 dnf·yum（RHEL 系）的发行版包操作能力 | ⚠️A2 双系探测择一 |
| 官方仓库源 | 个别环境所需的第三方仓库配置（如 RHEL 系 Docker CE、MongoDB 三系、Node.js 三系 NodeSource） | 面板代写仓库源配置并审计 |
| 环境变更任务 | 安装/升级/卸载的异步执行单元：pending → running → succeeded/failed | ⚠️A3；同环境互斥 |
| 升级（upgrade） | 已装环境再次安装到仓库最新版的幂等语义 | ⚠️A4；先刷新包索引 |
| 卸载（remove） | 包卸载但保留系统配置与数据目录 | ⚠️A6；purge 二期 OUT |

## 更新记录

| 版本 | 日期 | 更新说明 |
|------|------|---------|
| v1.4 | 2026-11-23 | 宝塔式重组增量轮（ADR-033，d0-clarification-baota-restructure-2026-11-23.md；设计依据 baota-restructure-design-2026-11-23.md 第三章 3.4）：①目录范围九环境不变，3 阶段 / 3 关键业务 / 7 业务活动不变（PM2 是 Node 环境的随装件，不是环境条目，L2/L3/L4 集合零变化）；②新增 ⚠️A19 Node 环境随装 PM2 口径（安装 / 升级 Node 时连带全局安装 PM2 并注册开机自启，尽力而为不阻断安装终态；卸载不触碰 PM2 进程列表）；③Node 就绪度读契约扩展 node / npm / PM2 可用性与版本；④附录A +app-mgmt 被消费一行（Node 就绪度）与供给一行（Node 连带 PM2）。本行为已于同轮落码，文档在 Gate 2 核对时补登 |
| v1.3 | 2026-11-20 | Python 环境增量轮（app-mgmt 集成联动，d0-clarification-app-mgmt-2026-11-20.md Q2 拍板）：①目录范围八→九环境（+Python，⚠️A1）——L2/L3/L4 集合零变化（Python 为目录条目非新业务活动，沿 Node 先例），3 阶段 / 3 关键业务 / 7 业务活动不变；②新增 ⚠️A18 Python 供给口径（发行版仓库包无第三方源代配〔apt python3/python3-venv/python3-pip 三包候选·rpm python3〕+ python3 --version 查询 + venv/pip 能力位探测 + 运行时型 ⚠️A17 同构；候选包名矩阵归 D3）；③2.1 概览/2.1.1/2.1.1.1/2.1.1.2 同步九环境与 Python 语义；④OUT 修订：Python 本体由 OUT 转为本轮供给，Python 多版本共存（pyenv/自选版本）登记 OUT；⑤附录A +app-mgmt 被消费两行（环境实况读契约/Python 供给）；附录B 术语环境九值、运行时型环境 +Python；⑥业务架构树零变化（L4 集合未变，核验一致） |
| v1.2 | 2026-11-19 | Node 环境增量轮（db-tools 集成联动，d0-clarification-db-tools-integration-2026-11-19.md Q1 拍板）：①目录范围七→八环境（+Node.js，⚠️A1）——L2/L3/L4 集合零变化（Node 为目录条目非新业务活动），3 阶段 / 3 关键业务 / 7 业务活动不变；②新增 ⚠️A16 Node 供给口径（NodeSource 三系代配 ⚠️A10 同构 + LTS 大版本轨道钉死〔版本号归 D3〕+ 包名 nodejs 三系同 + npm 随包 + node --version 查询）与 ⚠️A17 运行时型环境无服务口径（单元映射空/服务态 null/启停自启入口禁用/装完即用无引导）；③2.1/2.1.1.1/2.1.1.2/2.3.1/2.3.1.1/2.2.1.4 同步八环境与 Node 语义（含订正 v1.1 遗留「六环境」滞后两处）；④OUT +Python 环境（归未来）；⑤附录A +db-tools 被消费两行（环境实况读契约/Node 供给）；附录B 术语环境八值 +运行时型环境/NodeSource 源两术语、官方仓库源例补 NodeSource；⑥业务架构树零变化（L4 集合未变，核验一致） |
| v1.1 | 2026-11-17 | 安装后配置就绪增量轮（d0-clarification-2026-11-17-bootstrap.md，D0 拍板 Q1~Q4）：①新增 L4 2.2.1.4 安装后配置就绪（3 阶段 / 3 关键业务 / 6→7 业务活动）——数据库类环境安装成功后自动建管理账户/设随机强密码 + 注册实例与凭据到 database-mgmt（装完即可管）；②新增 ⚠️A11~A15 五项口径（引导范围/引导通道与账户〔dougong@localhost ALL WITH GRANT OPTION、Redis CONFIG SET+REWRITE、MongoDB localhost exception〕/幂等保守/引导失败独立维度/引导服务联动细化）；③补录 ⚠️A8~A10（W1 修复轮单元全名/探测三态/代配补真，此前 BA 滞后于 api/prd v1.1，本轮同步）；④目录范围六→七环境同步（+MongoDB，⚠️A1）；⑤OUT 登记二期方向（PHP 多版本 sury 源 / MySQL 版本升级 mysql-apt-config）；⑥附录A 新增 env-mgmt→database-mgmt 进程内 OHS 硬依赖（BootstrapAfterInstall，单向）；附录B 术语表 +4（安装后配置就绪/引导通道/管理账户/引导结果）；业务架构树同步 |
| v1.0 | 2026-11-16 | 初始生成：依据 v0.1.2 用户反馈第 4 项与 D0 增量澄清拍板（六环境 + apt/dnf·yum 双系）建立环境管理 L1-L4 业务架构（3 阶段 / 3 关键业务 / 6 业务活动）；落定 A1~A7 七项口径（目录范围/双包管理器/异步任务/升级/服务联动/卸载/包名映射）；边界与 database-mgmt（实例层）/docker-mgmt（容器层）/service-mgmt（通用单元层）/site-mgmt（站点层）分界登记 |
