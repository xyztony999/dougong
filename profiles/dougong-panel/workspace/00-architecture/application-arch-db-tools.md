# 斗拱面板 数据库工具集成（db-tools）应用架构

| 属性 | 值 |
|------|-----|
| 版本 | v1.2 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-19（v1.2 自动 nginx 落地增量轮：C-15-05 工具落地自动供给+域名联动+HostGuard 内部端点豁免+reason 可见，BA v1.1 ⚠️A14~A17） |

## 第一部分：文档概述

### 1.1 背景与目标

将《斗拱面板 数据库工具集成（db-tools）业务架构》（business-arch-db-tools.md v1.1）的 3 个 L3 关键业务按 DDD 四层归类为应用域，组织为功能模块/功能子模块，标注涉及用户类型与模块间调用关系，作为 PRD（D2）、API 设计（D3）与交互设计（D4）的功能组织依据。工具分发（tarball 下载/npm 安装，R-08 参数数组）、nginx 反代与 systemd 服务为技术实现承载，其业务纪律（localhost 绑定/会话 gating/凭据零回显/signon 防伪/卸载不删数据，⚠️A5~A12；v1.1 增量：自动落地/域名联动/内部端点豁免/reason 可见，⚠️A14~A17）落在本架构的模块说明与安全架构专节（附录C）中，实现归 D3 与后端工程。

**v1.2 增量范围（自动 nginx 落地轮，d0-clarification-dbtools-auto-landing-2026-11-19.md）**：新增功能子模块 C-15-05 工具落地自动供给（机制型，BA L4 2.2.1.5 映射）；C-15-01/03/04、S-59-01/04 五既有子模块口径扩展；跨模块消费 +panel-core 两条（配置变更通知钩子/HostGuard·EntryPathGuard 内部端点豁免，后者为 panel-core 机制侧变更跨模块待同步项）；DDD 归类不变（C-15-05 随 C-15 核心业务域——落地供给是部署主链的组成步骤，同 env-mgmt C-14-04 安装后配置就绪机制型先例；L3 集合/应用域/模块编码 C-15·S-58·S-59 均不变）。

DDD 归类结论（依据 d1 分册归类原则逐条判定）：

- **核心业务域**：工具部署与变更（承载 tool_deploy_task 聚合根候选与四态任务状态机 pending → running → succeeded/failed，是工具供给的单据型主链动作，互斥/审计/sha256 校验业务规则齐备）——满足原则一（状态机 + 聚合根候选 + 主链变更动作 + 产生任务记录）；与 env-mgmt C-14（env_install_task）同构判定；
- **支撑业务域**：工具目录与实况（含目录范围/依赖就绪度/探测领域规则，被供给与访问两链消费）、工具安全访问（含 localhost 绑定/会话 gating/signon 防伪/凭据零回显等强业务安全规则，被打开入口与 database-mgmt 高级入口消费）——两者含数据库运维领域特有业务规则、被主链消费，自身无独立状态机（gating 为逐请求实时判定非面板状态机，工具运行态为 systemd/站点实况投影）；
- **基础数据域**：本模块无基础数据 L3——工具目录虽为内置静态引用数据，但其 L3 整体含实况探测运行时规则（非纯字典），按整体归类支撑业务域（与 env-mgmt S-53 同判）；
- **平台支撑域**：本模块无零业务语义的纯技术 L3——nginx auth_request 配置/systemd 单元/PHP-FPM pool 为实现细节（分别复用 site-mgmt 安全写入管线、service-mgmt SysdGateway、env-mgmt PHP-FPM 环境资产），业务规则承载于 S-58/C-15/S-59。

> tool_deploy_task（工具部署任务）为本模块聚合根候选（四态状态机 + 同工具互斥 + 审计单据），持久化口径（任务台账落库）归 D2 领域建模落定；工具目录为编译期常量（非领域对象候选）；工具凭据（basic auth 口令/signon 密钥）为密文值对象候选（载体表归 D2/D3）。

> 模块编码衔接说明（编码规则的跨模块执行口径）：功能模块编码全局唯一，承接既有占用——account-auth C-01~C-03/S-01~S-05、panel-core S-06~S-12、monitoring S-13~S-16、service-mgmt C-04~C-05/S-17~S-18、file-manager C-06/S-19~S-25、site-mgmt C-07~C-10/S-26~S-30、database-mgmt S-31~S-37/S-55~S-57、terminal C-11/S-38~S-41、firewall S-42~S-44、docker-mgmt C-12/S-45~S-50、cron-backup C-13/S-51~S-52、env-mgmt C-14/S-53~S-54；本模块核心业务域模块编为 C-15（承接全局最大核心序号 C-14），支撑业务域模块自 S-58 起编至 S-59（承接全局最大支撑序号 S-57，依据「新模块在域内最大序号后递增」；已核验全仓 application-arch-*.md 无 C-15/S-58/S-59 占用）。

### 1.2 文档定位

| 定位 | 说明 |
|------|------|
| 本文档做什么 | 将业务架构映射为应用域、功能模块、功能子模块的层级结构，标注用户类型、模块间调用关系及应用域分层 |
| 本文档不做什么 | 不涉及具体功能点描述、不涉及系统详细设计、不涉及技术实现和部署方案 |

### 1.3 范围

| 范围 | 值 |
|------|-----|
| 业务域 | 数据库工具集成（db-tools） |
| 应用域 | 核心业务域（工具部署与变更）、支撑业务域（工具目录与实况、工具安全访问） |
| 用户类型 | 管理员（admin）、只读用户（readonly） |

> 用户类型语义（与既有十二模块应用架构一致）：**管理员（admin）**——全部工具变更类操作（安装/升级/卸载/凭据轮换）与工具访问（打开/gating 放行）的执行者；**只读用户（readonly）**——按 ADR-018「全模块可查看」：查看工具目录、实况与部署任务状态/输出，禁一切变更；**工具访问整体 admin 专属**（⚠️A10）：readonly 经 gating 角色校验不可达工具本体（401/403），此为 ADR-018 在工具访问面的具体化（同 Web 终端整体禁用先例，但保留目录/任务查看面）。**访客（未认证操作者）不涉及**：本模块全部功能位于登录会话之后，工具路径无会话不可达（⚠️A7）。

## 第二部分：应用架构

> 本章按应用域逐层描述功能模块与功能子模块。每个应用域有其概述，每个功能模块/子模块有功能说明及涉及用户类型。

### 本架构概览

应用架构按 DDD 四层聚为两个应用域：

- **核心业务域**承载工具供给状态链：工具部署与变更维持 tool_deploy_task 四态任务状态机（pending → running → succeeded/failed，同工具互斥），安装/升级/卸载三动作共用任务通道与审计纪律（sha256 强制校验、卸载不删数据库数据）；**v1.2：落地通道自动供给（C-15-05）——落点 0 匹配自动建专用 nginx 工具块（listen 18090）+域名联动（server_name 跟随绑定状态，钩子+惰性双通道）+幂等复用+两工具卸载清块，IP 直连面板形态工具可用（⚠️A14/A15）；任务失败原因含 data.reason 如实可见（⚠️A17）**；
- **支撑业务域**承载「看得见 + 安全用得着」：工具目录与实况以内置目录叠加实时探测供给工具全景（供给与访问的共同数据底座，database-mgmt 高级入口状态源），工具安全访问提供 localhost 绑定 + 会话 gating + signon 桥接 + basic auth 注入的安全访问能力（凭据全程零回显）。

调用关系总脉络：实况探测结果是安装/升级/卸载/打开按钮态的前置依据；变更任务完成后实况刷新；部署产出（站点路径/服务/凭据密文/**落地块引用 siteRef+Port+AutoCreated**）是安全访问的配置来源；打开入口按工具类型分发到 signon 桥接或 gating 反代直达（**v1.2：URL 基址按落点端口分支 ⚠️A15**）；**跨模块消费六模块（v1.2 五模块→六模块，+panel-core 两条契约）**——env-mgmt 环境实况与 Node/PHP-FPM 环境供给（依赖前置）、site-mgmt 站点创建/移除与安全写入管线（工具路径落盘，**含自动落地块 ConfigApply/ConfigRemove**）、service-mgmt SysdGateway（mongo-express 服务单元）、database-mgmt 实例凭据 OHS（vault 取出连接凭据，进程内单向）、account-auth 会话校验与审计（gating 端点 + R-03）、**panel-core〔契约一：配置变更通知钩子（bindDomain 绑定/解绑 → 自动块 server_name 刷新，进程内单向，D1 裁定钩子主通道）；契约二：HostGuard/EntryPathGuard 内部端点豁免（gating/signon 内部 2 端点绑域名后可达，panel-core 机制侧变更，跨模块待同步项）〕**；**跨模块被消费一条**——database-mgmt 实例治理页/库行「高级管理」入口消费工具状态与打开能力（呈现层，入口渲染归 database-mgmt，登记为跨模块待同步项）。全部变更类操作经 account-auth 审计契约留痕（R-03）；全部接口位于登录会话之后并受只读边界管控（ADR-018）。安全架构五道防线（v1.2 +防线五自动落地块安全）见附录C。

### 应用域分层

> 按 DDD 战略设计，将应用域归入不同分层，决定后续详细设计的建模深度和投入策略。AA 应用域严格按照 DDD 四层划分，每个 BA L3 按 L3→DDD 四层归类原则归入对应分层。

| 分层 | 说明 | 包含功能模块 |
|------|------|-------------|
| 核心业务域 | 承载核心业务价值，有状态机和聚合根，深度建模 | 工具部署与变更（C-15） |
| 支撑业务域 | 为核心业务提供必要的业务支撑能力，包含业务规则，独立上下文 | 工具目录与实况（S-58）、工具安全访问（S-59） |
| 基础数据域 | 系统级引用数据，无业务状态机，薄适配 | —（本模块无；工具目录为编译期常量随 S-58 承载，实况为运行时投影，二者不构成独立基础数据 L3） |
| 平台支撑域 | 纯技术能力，零业务语义，可复用 | —（本模块无；nginx/systemd/PHP-FPM 通道复用 site-mgmt/service-mgmt/env-mgmt 既有资产，业务规则承载于 S-58/C-15/S-59） |

### 2.1 核心业务域

**应用域概述**：承载工具供给的变更主链与任务状态机：安装/升级/卸载经异步任务执行（互斥、审计、sha256 校验、输出如实），任务记录构成工具部署的可追溯台账。服务的用户群体：管理员（admin）执行全部变更；只读用户（readonly）仅可查看任务状态与输出（ADR-018）。

#### 2.1.1 工具部署与变更（C-15）

**功能说明**：工具本体的供给主链：安装（依赖前置 + 下载分发 + 站点/服务落位 + 凭据生成）、升级（部署目录钉死新版本，配置保留）、卸载（强确认 + 解除部署不删数据库数据）、异步任务跟踪（四态状态机，输出尾部排障，**失败原因含 data.reason ⚠️A17**）、**落地自动供给（v1.2：0 匹配自动建块/域名联动/幂等复用/卸载清块，⚠️A14/A15）**；安全纪律贯穿（参数数组 R-08、审计 R-03、sha256 强制校验、凭据密文零回显、自动块写入经 ConfigApply R-06）。

**涉及用户类型**：- [✓] 管理员（admin） - [✓] 只读用户（readonly）（变更动作仅管理员，任务状态与输出两角色可看）

##### 2.1.1.1 工具安装（C-15-01）

**功能说明**：安装目录工具：依赖前置检测（phpMyAdmin=PHP-FPM+PHP 扩展+nginx；mongo-express=Node+nginx，缺失如实拒绝并引导到环境管理，⚠️A2）；phpMyAdmin 下载 tarball（钉版本+sha256+镜像，⚠️A12）→解压管理目录→生成 config.inc.php（signon 配置）→经 site-mgmt 建 php_fpm 站点；mongo-express npm 安装（精确版本）→建 systemd 服务（env 注入 vault Mongo 凭据+面板 basic auth+baseUrl+bind 127.0.0.1）→经 site-mgmt 建 reverse_proxy 站点；**落地选择（v1.2 修订）**：既有匹配块优先沿用，0 匹配经 C-15-05 自动建块（端口占用→DBT_012 reason），多匹配仍引导；工具凭据生成密文落库（⚠️A9）；异步任务化执行（⚠️A3）；安装成功输出含 ToolLandingPort 放行提示（自动块形态，⚠️A14）。

**涉及用户类型**：- [✓] 管理员（admin） - [ ] 只读用户（readonly）

**调用关系**：

| 调用模块/子模块 | 调用说明 |
|---------------|---------|
| 工具实况探测（S-58-02） | 安装前置：未装态/依赖就绪度依据 |
| 工具落地自动供给（C-15-05，v1.2） | 落地选择：既有匹配块优先沿用；0 匹配自动建块（含域名联动 server_name 初值）；端口占用→DBT_012 reason；产出 siteRef（+Port+AutoCreated）供注入与打开 URL |
| 工具部署任务跟踪（C-15-04） | 安装动作产生异步任务，进度与结果经任务跟踪呈现 |
| env-mgmt 环境目录与实况（S-53，跨模块） | 依赖就绪度数据源：PHP-FPM/Node/nginx 已装态（进程内读契约）；缺失引导到环境管理安装（软引导） |
| site-mgmt 新建站点（S-27-02/S-27-03，跨模块） | php_fpm/reverse_proxy 站点创建落工具路径配置（经安全写入治理 S-30，R-06） |
| service-mgmt SysdGateway（跨模块，共享适配层资产） | mongo-express systemd 服务单元创建与启动 |
| database-mgmt 实例凭据 OHS（跨模块，进程内单向） | 安装时校验目标实例凭据可取（vault 存在性，不取明文外发）；Mongo 连接凭据注入准备 |
| 审计记录（account-auth S-04） | 安装为变更类操作，写审计（R-03，detail 不含凭据） |

##### 2.1.1.2 工具升级（C-15-02）

**功能说明**：升级已装工具到面板目录钉死新版本（⚠️A4）：phpMyAdmin 下载新 tarball（sha256 校验）替换程序目录，保留 config.inc.php 与 signon 密钥；mongo-express 重装 npm 新版本并重启服务（凭据注入形态不变）；升级不改变工具凭据与站点路径；异步任务化执行。

**涉及用户类型**：- [✓] 管理员（admin） - [ ] 只读用户（readonly）

**调用关系**：

| 调用模块/子模块 | 调用说明 |
|---------------|---------|
| 工具实况探测（S-58-02） | 升级前置：已装态与当前版本依据 |
| 工具部署任务跟踪（C-15-04） | 升级动作产生异步任务 |
| service-mgmt SysdGateway（跨模块，共享适配层资产） | mongo-express 升级后服务重启 |
| 审计记录（account-auth S-04） | 升级为变更类操作，写审计（R-03） |

##### 2.1.1.3 工具卸载（C-15-03）

**功能说明**：卸载已装工具：强确认交互（呈现「高级管理入口将不可用，面板内轻量浏览与数据库数据不受影响」知情提示，⚠️A5）；解除工具部署——删除程序目录/npm 包、移除 systemd 服务（mongo-express）、经 site-mgmt 移除工具站点配置（工具 location 撤除）、删除工具凭据密文；**两工具均卸载后经 C-15-05 移除自动落地块**（v1.2，⚠️A14：AutoCreated 判定，非自动块不动）；**绝不删除数据库数据**；异步任务化执行，卸载后工具回未装态。

**涉及用户类型**：- [✓] 管理员（admin） - [ ] 只读用户（readonly）

**调用关系**：

| 调用模块/子模块 | 调用说明 |
|---------------|---------|
| 工具实况探测（S-58-02） | 卸载前置：已装态依据 |
| 工具落地自动供给（C-15-05，v1.2） | 工具 location 撤除后判定：两工具均卸载且 AutoCreated → 移除自动落地块（经 ConfigRemove/ConfigApply R-06） |
| 工具部署任务跟踪（C-15-04） | 卸载动作产生异步任务 |
| site-mgmt 站点信息管理（S-28，跨模块） | 工具站点配置移除（经安全写入治理 S-30，R-06） |
| service-mgmt SysdGateway（跨模块，共享适配层资产） | mongo-express 服务停止与单元移除 |
| 审计记录（account-auth S-04） | 卸载为高危变更类操作，写审计（R-03，detail 含知情提示呈现记录，不含凭据） |

##### 2.1.1.4 工具部署任务跟踪（C-15-04）

**功能说明**：异步任务的观测与结果确认：任务状态（pending → running → succeeded/failed）、目标工具与动作、输出尾部（排障）、失败原因（sha256 校验失败/依赖缺失等如实分类；**v1.2 ⚠️A17：worker finish 错误文本含 errcode data.reason 拼接——DBT_012 端口占用/多候选/扫描失败等具体原因任务详情可见，沿 env-mgmt worker 错误形态先例**）；同工具在途互斥（重复提交如实拒绝，⚠️A3）；任务列表与单任务详情。

**涉及用户类型**：- [✓] 管理员（admin） - [✓] 只读用户（readonly）（ADR-018 全可看）

**调用关系**：

| 调用模块/子模块 | 调用说明 |
|---------------|---------|
| 审计记录（account-auth S-04） | 任务终态（成功/失败）结果一并留痕（R-03） |

##### 2.1.1.5 工具落地自动供给（C-15-05，v1.2 新增）

**功能说明**：工具访问落地通道的自动供给与生命周期管理（机制型，BA L4 2.2.1.5，⚠️A14/A15）：

- **自动建块**：落点选择（selectLanding）0 匹配（IP+端口直连面板、未绑域名、nginx 无面板域 server 块）时自动创建专用 nginx 工具 server 块——{nginx conf.d}/dougong-tools.conf（编译期常量路径，沿 site-mgmt 扫描目录），listen ToolLandingPort=18090（编译期常量，绑定 0.0.0.0），块内含 gating auth_request location 与工具 location；端口被占用→DBT_012 data.reason「工具端口被占用」如实引导释放（不静默换端口）；
- **域名联动**：server_name 跟随面板域名绑定状态——未绑 _（IP/任意 Host 可达）；已绑 BindDomain+非匹配 Host 404（镜像 panel-core HostGuard 语义，绑域名后工具也只认域名，对齐宝塔）；域名变更同步=**panel-core 配置变更通知钩子主通道**（bindDomain 绑定/解绑成功 → 进程内通知 → 自动块存在则重渲染 server_name 经 ConfigApply 刷新；钩子刷新失败不阻断配置变更，记日志）+**工具 install/open 惰性校正兜底**（按当前 BindDomain 校正块，自愈钩子失败/重启窗口）；
- **选择纪律**：既有匹配块（绑域名 server_name 匹配/反代面板 proxyTargetsPanel）优先沿用不新建；多匹配仍 DBT_012 列候选引导明确（不变）；
- **幂等与共用**：自动块已存在则复用（标记/文件存在判定），两工具共用一块（gate location 去重）；落点引用 siteRef +Port（落地监听端口，既有 80/443 块 Port=0 表默认）+AutoCreated 标志；
- **清块**：卸载移除工具 location；两工具均卸载后移除自动块（AutoCreated 判定，非自动块不动）；
- **安全写入**：自动块创建/刷新/移除经 site-mgmt ConfigApply/ConfigRemove 管线（nginx -t 自检+reload，失败字节复原 R-06）；
- **端口放行提示**：安装成功输出/文档提示放行 ToolLandingPort（防火墙/云安全组，仅提示不自动放行，OUT 边界）。

**涉及用户类型**：- [✓] 管理员（admin） - [ ] 只读用户（readonly）（机制型：随安装/卸载/域名变更自动触发，无独立入口；结果经任务跟踪与打开 URL 呈现）

**调用关系**：

| 调用模块/子模块 | 调用说明 |
|---------------|---------|
| 工具安装（C-15-01） | 宿主：安装落地选择 0 匹配时触发自动建块，产出 siteRef 供 location 注入 |
| 工具卸载（C-15-03） | 宿主：卸载撤除 location 后判定两工具均卸载 → 清自动块 |
| 工具打开入口（S-59-04） | 打开时惰性校正自动块 server_name（兜底通道）；siteRef.Port 供 URL 基址分支 |
| site-mgmt 安全写入治理（S-30，跨模块） | 自动块创建/域名联动刷新/清块经 ConfigApply/ConfigRemove 管线（nginx -t+reload+失败字节复原 R-06） |
| panel-core 面板配置管理（S-11-01，跨模块，v1.2 新增） | 消费配置变更通知钩子：bindDomain 绑定/解绑 → 自动块 server_name 刷新（进程内单向，钩子契约签名归 D3；panel-core 侧钩子提供为跨模块待同步项） |
| panel-core 监听与入口接入（S-07，跨模块，v1.2 新增，被消费语义） | 自动块 server_name+非匹配 Host 404 镜像 HostGuard（S-07-03）语义；gating/signon 内部端点依赖 HostGuard/EntryPathGuard 豁免（⚠️A16，panel-core 机制侧变更，跨模块待同步项） |

### 2.2 支撑业务域

**应用域概述**：承载「看得见 + 安全用得着」：工具目录与实况供给工具全景（内置目录 + 实时探测，供给与访问的共同数据底座，database-mgmt 高级入口状态源），工具安全访问提供 localhost 绑定 + 会话 gating + 凭据桥接的安全访问能力。服务的用户群体：管理员（admin）查看与执行（含工具访问）；只读用户（readonly）查看目录/实况/任务（ADR-018），工具本体不可达（⚠️A10 gating 角色拒绝），无任何变更入口。

#### 2.2.1 工具目录与实况（S-58）

**功能说明**：内置两工具目录（⚠️A1）与运行时实况（部署状态/版本/运行态/依赖就绪度，⚠️A2）的合并呈现；纯读能力，是供给与访问两链的状态依据，也是 database-mgmt 高级管理入口的引导数据源。

**涉及用户类型**：- [✓] 管理员（admin） - [✓] 只读用户（readonly）（ADR-018 全可看）

##### 2.2.1.1 工具目录呈现（S-58-01）

**功能说明**：工具全景列表：显示名、描述、目标引擎、部署状态（未装/已装/版本）、运行态、依赖就绪度、在途任务标记；未装工具呈现安装入口（admin），已装呈现升级/卸载/打开入口；工具管理页为本模块唯一页面载体。

**涉及用户类型**：- [✓] 管理员（admin） - [✓] 只读用户（readonly）（变更与打开入口仅管理员可见，ADR-018 负空间）

**调用关系**：

| 调用模块/子模块 | 调用说明 |
|---------------|---------|
| 工具实况探测（S-58-02） | 列表条目的实况字段为探测结果投影 |

##### 2.2.1.2 工具实况探测（S-58-02）

**功能说明**：逐工具探测：部署状态与版本（程序目录/npm 包查询）、运行态（mongo-express 经 systemd 单元查询；phpMyAdmin 经站点与 PHP-FPM pool 可用性判定）、依赖就绪度（消费 env-mgmt 环境实况：PHP-FPM/Node/nginx 已装态 + PHP 扩展检测）；纯读机制，被目录呈现/安装/升级/卸载/打开共同消费。

**涉及用户类型**：- [ ] 管理员（admin） - [ ] 只读用户（readonly）（系统内部机制，结果经 S-58-01 等呈现）

**调用关系**：

| 调用模块/子模块 | 调用说明 |
|---------------|---------|
| env-mgmt 环境目录与实况（S-53，跨模块） | 依赖就绪度数据源：PHP-FPM/Node/nginx 已装态（进程内读契约，单向消费） |
| service-mgmt SysdGateway（跨模块，共享适配层资产） | mongo-express 运行态查询（单元 active 状态） |

#### 2.2.2 工具安全访问（S-59）

**功能说明**：已部署工具的安全访问管控：localhost 绑定（⚠️A6）+ 面板会话 gating 反代（admin 专属，⚠️A7/A10）+ phpMyAdmin signon 桥接（令牌防伪 + 凭据零回显注入，⚠️A8）+ mongo-express basic auth 注入与轮换（⚠️A9）+ 统一打开入口；安全架构四道防线见附录C。

**涉及用户类型**：- [✓] 管理员（admin） - [ ] 只读用户（readonly）（工具访问整体 admin 专属，⚠️A10；readonly 经 gating 不可达工具本体）

##### 2.2.2.1 会话 gating 反代（S-59-01）

**功能说明**：面板 nginx 工具路径的访问闸门（机制型）：每请求（**含自动落地块 18090 端口，v1.2 ⚠️A14**）经 auth_request 子请求到面板鉴权端点，校验面板会话 cookie（有效性/过期/吊销）+ admin 角色——200 放行反代到 localhost 工具上游 / 401 拒绝（⚠️A7）；鉴权端点仅接受 loopback 内部子请求通道，不对外暴露；角色判定服务端权威（防 CSRF/越权：纯读校验无状态变更，不信任客户端声明）；既有面板域落地块形态下面板入口路径/域名绑定约束同链生效，自动块形态下域名约束经块内 server_name+非匹配 Host 404 镜像（⚠️A15）；**内部端点豁免（v1.2 ⚠️A16）**：gate/verify·signon/verify 内部 2 端点路径豁免 HostGuard/EntryPathGuard（绑域名后子请求/回调 Host≠域名不被误拦 404；豁免范围最小化，安全由 loopback+秘密头+会话/令牌承载，非内部路径强校验不变；豁免落点归 panel-core，跨模块待同步项）；会话吊销/改密级联即时切断工具访问；逐请求校验不落业务审计（⚠️A11）。

**涉及用户类型**：- [✓] 管理员（admin） - [ ] 只读用户（readonly）（机制型；gating 对 admin 放行、对 readonly/无会话拒绝，⚠️A10）

**调用关系**：

| 调用模块/子模块 | 调用说明 |
|---------------|---------|
| account-auth 访问控制（S-03，跨模块） | 消费会话校验能力：gating 鉴权端点校验会话 cookie + admin 角色（**db-tools 自持 GET /dbtool/gate/verify 内部端点，复用 account-auth 既有只读会话校验 OHS VerifyWs，account-auth 零新增 HTTP——api D-11 定稿，v1.1 W2 回填**） |
| site-mgmt 安全写入治理（S-30，跨模块） | 工具路径 auth_request 配置经安全写入管线落盘（R-06，安装/卸载时写入/移除；v1.2：自动落地块 gate location 同管线） |
| panel-core 监听与入口接入（S-07-02/S-07-03，跨模块，v1.2 新增） | 内部端点豁免依赖：HostGuard/EntryPathGuard 对 /dbtool/gate/、/dbtool/signon/ 路径前缀直通放行（⚠️A16，panel-core 机制侧变更，跨模块待同步项——本轮 db-tools D1 不代改） |

##### 2.2.2.2 phpMyAdmin signon 桥接（S-59-02）

**功能说明**：phpMyAdmin 无感登录桥接（机制型）：打开工具时面板生成签名令牌（HMAC 防伪 + 短时效 + 一次性消费，绑定目标实例，⚠️A8）→ phpMyAdmin signon 流程经服务端回调到面板 API 校验令牌（**回调端点 /dbtool/signon/verify 豁免 HostGuard/EntryPathGuard，v1.2 ⚠️A16：绑域名后 loopback 回调不被误拦；loopback+秘密头防护不变**）→ 校验通过后消费 database-mgmt 实例凭据 OHS 从 vault 取出 MySQL 凭据，经 loopback 服务端通道注入 phpMyAdmin 会话——凭据永不进入浏览器（I-02 零回显）；令牌过期/重放/跨实例重用如实拒绝；桥接动作（访问开启）审计留痕。

**涉及用户类型**：- [✓] 管理员（admin） - [ ] 只读用户（readonly）（机制型；仅 admin 打开动作触发，⚠️A10）

**调用关系**：

| 调用模块/子模块 | 调用说明 |
|---------------|---------|
| 工具打开入口（S-59-04） | 宿主：打开 phpMyAdmin 时触发令牌签发与桥接 |
| database-mgmt 实例凭据 OHS（跨模块，进程内单向） | 令牌校验通过后取出目标实例 MySQL 凭据（vault 解密仅服务端，凭据不出 loopback 通道） |
| 审计记录（account-auth S-04） | 访问开启（令牌签发）审计留痕（R-03，detail 不含凭据与令牌明文） |

##### 2.2.2.3 mongo-express 凭据注入（S-59-03）

**功能说明**：mongo-express 无感认证注入与凭据轮换：面板生成的 basic auth 用户名/口令经 nginx 反代注入 Authorization 头（浏览器无感知，⚠️A9）；Mongo 连接凭据取自 vault（database-mgmt 实例凭据 OHS）经 systemd 环境注入工具进程；basic auth 口令轮换（admin 显式动作：重新生成 + 服务/站点配置更新 + 旧口令即时失效 + 审计）；口令与连接凭据全程零回显（I-02）。

**涉及用户类型**：- [✓] 管理员（admin） - [ ] 只读用户（readonly）（轮换为变更类操作仅管理员；注入机制随部署/打开生效）

**调用关系**：

| 调用模块/子模块 | 调用说明 |
|---------------|---------|
| database-mgmt 实例凭据 OHS（跨模块，进程内单向） | Mongo 连接凭据取出（vault 解密仅服务端，经 systemd env 注入，不进 argv/日志） |
| site-mgmt 安全写入治理（S-30，跨模块） | basic auth 注入配置（proxy_set_header）经安全写入管线落盘/轮换更新（R-06） |
| service-mgmt SysdGateway（跨模块，共享适配层资产） | 轮换后 mongo-express 服务重启（env 更新生效） |
| 审计记录（account-auth S-04） | 凭据轮换为变更类操作，写审计（R-03，detail 不含口令） |

##### 2.2.2.4 工具打开入口（S-59-04）

**功能说明**：管理员打开已装工具的统一入口：工具管理页「打开」动作与 database-mgmt 实例治理页/库行「高级管理」入口（跨模块呈现消费）共用同一打开能力——按工具类型分发（phpMyAdmin 走 signon 令牌签发〔S-59-02〕、mongo-express 走 gating 反代直达〔S-59-01〕）；**打开 URL 基址按落点端口分支（v1.2 ⚠️A15）**：siteRef.Port≠0 → scheme://hostname:Port（hostname 取 BindDomain 优先，否则 requestHost 去端口——IP 直连 http://IP:18090/dbtool/...，绑域名 http://域名:18090/...），Port=0 沿旧形态 scheme://host；打开时经 C-15-05 惰性校正自动块 server_name（域名联动兜底通道）；未装工具呈现安装引导（不虚构可打开）；打开动作（访问开启）审计留痕（⚠️A11）。

**涉及用户类型**：- [✓] 管理员（admin） - [ ] 只读用户（readonly）（工具访问整体 admin 专属，⚠️A10，readonly 负空间）

**调用关系**：

| 调用模块/子模块 | 调用说明 |
|---------------|---------|
| 工具实况探测（S-58-02） | 打开前置：已装态与运行态依据（未装引导安装） |
| 工具落地自动供给（C-15-05，v1.2） | 打开时惰性校正自动块 server_name（域名联动兜底）；siteRef.Port 供 URL 基址分支（⚠️A15） |
| phpMyAdmin signon 桥接（S-59-02） | phpMyAdmin 打开分发：令牌签发与桥接 |
| 会话 gating 反代（S-59-01） | mongo-express 打开分发：gating 放行后反代直达 |
| 审计记录（account-auth S-04） | 打开动作（访问开启）审计留痕（R-03） |
| database-mgmt 实例治理（S-31/S-55，跨模块，被消费） | 实例治理页/库行「高级管理」入口消费本打开能力与工具状态（呈现层渲染归 database-mgmt PageSpec，登记为跨模块待同步项） |

## 第三部分：应用架构树

> **由第二部分内容对应生成，第二部分任何变更必须同步更新本视图。** 结构：应用域 > 功能模块 > 功能子模块（含功能说明）。

```
斗拱面板 数据库工具集成（db-tools）
├── 核心业务域
│   └── 工具部署与变更（C-15）
│       ├── 工具安装（C-15-01） — 依赖前置→下载分发（sha256）→站点/服务落位→凭据生成，异步任务
│       ├── 工具升级（C-15-02） — 部署目录钉死新版本，配置与凭据保留，异步任务
│       ├── 工具卸载（C-15-03） — 强确认+解除部署（站点/服务/凭据），绝不删数据库数据，异步任务
│       ├── 工具部署任务跟踪（C-15-04） — pending→running→succeeded/failed，输出尾部排障（含 data.reason），同工具互斥
│       └── 工具落地自动供给（C-15-05，v1.2） — 0 匹配自动建块（listen 18090）+域名联动（钩子+惰性）+幂等复用+卸载清块（机制型）
├── 支撑业务域
│   ├── 工具目录与实况（S-58）
│   │   ├── 工具目录呈现（S-58-01） — 两工具全景列表：目录条目+实况+依赖就绪度+入口
│   │   └── 工具实况探测（S-58-02） — 部署状态/版本/运行态/依赖就绪度（机制型）
│   └── 工具安全访问（S-59）
│       ├── 会话 gating 反代（S-59-01） — auth_request 校验面板会话+admin 角色，localhost 上游，内部端点豁免 HostGuard/EntryPathGuard（机制型）
│       ├── phpMyAdmin signon 桥接（S-59-02） — 签名令牌防伪+vault 凭据 loopback 注入，零回显（机制型）
│       ├── mongo-express 凭据注入（S-59-03） — basic auth 服务端注入+口令轮换，Mongo 凭据 env 注入
│       └── 工具打开入口（S-59-04） — 统一打开能力（signon/gating 分发），database-mgmt 高级入口消费
├── 基础数据域
│   └── —（无；工具目录为编译期常量随 S-58 承载，实况为运行时投影）
└── 平台支撑域
    └── —（无；nginx/systemd/PHP-FPM 通道复用 site-mgmt/service-mgmt/env-mgmt 既有资产）
```

## 附录

### 附录A：机制型子模块与功能清单覆盖映射

> 部分功能子模块为系统机制型（无独立菜单页面），其业务行为经宿主菜单或全局行为在功能清单（function-list-db-tools.md）中覆盖，映射如下：

| 机制型子模块 | 承载菜单/全局行为 | 功能清单覆盖位置 |
|-------------|------------------|----------------|
| 工具实况探测（S-58-02） | 工具列表实况字段的自动探测（无独立入口） | 工具列表功能点描述内体现 |
| 会话 gating 反代（S-59-01） | 工具路径逐请求会话校验（无独立入口），打开工具与高级管理入口的访问前置 | 打开工具功能点描述（gating 前置）+ 全局行为（readonly/无会话不可达说明） |
| phpMyAdmin signon 桥接（S-59-02） | 打开 phpMyAdmin 时自动令牌签发与凭据注入（无独立入口） | 打开工具功能点描述（signon 桥接分发）内体现 |
| 工具落地自动供给（C-15-05，v1.2） | 安装时自动建块/域名变更钩子刷新/打开时惰性校正/卸载清块（无独立入口） | 安装工具/卸载工具/打开工具功能点描述 + 任务详情失败原因（端口占用 reason）内体现 |

### 附录B：跨模块契约、模块边界与 D1 设计口径

**跨模块契约**：

| 契约 | 方向 | 内容 |
|------|------|------|
| 审计写入（account-auth S-04） | 本模块消费 | 全部变更类操作（安装/升级/卸载/打开/凭据轮换）按 R-03 写入审计（同事务脱敏，detail 不含凭据）；任务终态结果一并留痕 |
| 会话鉴权与只读边界（account-auth S-03） | 本模块消费 | 全部接口位于登录会话之后；只读角色查看目录/实况/任务，全部变更被全局边界管控拒绝（ADR-018）；**gating 鉴权端点=db-tools 自持 GET /dbtool/gate/verify**（校验会话 cookie+admin 角色，200/401，仅 loopback 子请求通道），复用 account-auth 既有只读会话校验 OHS（VerifyWs 语义 session.go:59，只读不续期），**account-auth 零新增 HTTP 接口**（api D-11 定稿，OHS 消费已登记 api-account-auth.md v1.2；v1.1 W2 回填） |
| 环境实况消费（env-mgmt S-53） | 本模块消费（进程内读契约） | 依赖就绪度数据源：PHP-FPM/Node/nginx 已装态与版本（单向读，无 HTTP 绕行）；Node 环境为 env-mgmt 本轮增量供给（ba/aa/fl v1.2 +Node） |
| 环境安装引导（env-mgmt） | 本模块软引导 | 依赖缺失时安装拒绝并呈现「到环境管理安装 PHP-FPM/Node/nginx」指引链接；无跨模块 API 硬依赖（沿 env-mgmt 被消费软引导先例反向） |
| 站点创建/移除（site-mgmt S-27-02/S-27-03/S-28 + S-30） | 本模块消费 | phpMyAdmin 经 php_fpm 站点、mongo-express 经 reverse_proxy 站点落工具路径配置；auth_request/basic auth 注入配置同经安全写入治理（R-06 四步管线）；卸载经站点移除通道；站点形态=面板域下路径反代（非独立域名站点），契约细节归 D3；**v1.2：自动落地块（dougong-tools.conf）创建/域名联动刷新/清块经 ConfigApply（allowNew）/ConfigRemove 管线（configpipeline.go:18/:29 既有 OHS，nginx -t+reload+失败字节复原）** |
| 配置变更通知钩子（panel-core S-11-01，v1.2 新增） | 本模块消费（进程内单向，待同步项） | bindDomain 绑定/解绑成功（config/update，D-P12）→ panel-core 轻量钩子通知 db-tools → 自动块存在则重渲染 server_name 经 ConfigApply 刷新（域名联动主通道，D1 裁定）；钩子失败不阻断配置变更（记日志+惰性校正兜底）；钩子契约签名归 D3；**panel-core 侧钩子提供为跨模块待同步项（panel-core 三文档同步登记，本轮 db-tools D1 不代改，沿 database-mgmt 高级入口先例）** |
| HostGuard/EntryPathGuard 内部端点豁免（panel-core S-07-02/S-07-03，v1.2 新增） | 本模块消费（panel-core 机制侧变更，待同步项） | 全局包装器（cmd/dougong/main.go:487-488 Entry→Host→gin 链）对内部 2 端点路径前缀 /dbtool/gate/、/dbtool/signon/ 直通放行（⚠️A16）——修复绑域名后 auth_request 子请求/SignonScript 回调 Host≠域名被误拦 404 致 gating 失效；豁免范围最小化（仅内部 2 路径前缀），非内部路径强校验不变（IP 直连面板仍 404，fail-closed 不变）；内部端点安全由 loopback+秘密头+会话/令牌承载（api/dbtools/module.go:64/:208 防护链不变）；**panel-core 三文档同步登记为跨模块待同步项，本轮 db-tools D1 不代改** |
| systemd 通道复用（service-mgmt SysdGateway） | 本模块消费（共享适配层资产） | mongo-express 服务单元创建/启停/重启/移除与运行态查询复用同一 systemd 网关实现（进程内适配器共享，非 HTTP 调用）；全部系统服务清单治理仍归 service-mgmt |
| 实例凭据 OHS（database-mgmt，新增） | 本模块消费（进程内 OHS，单向硬依赖） | signon 桥接与 mongo-express 注入消费 db-mgmt 实例凭据取出能力（vault 解密仅服务端，凭据经 loopback/systemd env 通道流转，不出服务端边界）——与 env-mgmt 消费 BootstrapAfterInstall 同构；契约签名归 D3（api-database-mgmt.md §5.1 登记） |
| 高级管理入口（database-mgmt S-31/S-55） | 本模块被消费（呈现层） | 实例治理页/库行「高级管理（phpMyAdmin/mongo-express）」入口消费工具状态与打开能力（admin 专属；未装引导安装；readonly 负空间）；入口渲染归 database-mgmt PageSpec——database-mgmt 三文档同步登记为跨模块待同步项（沿 F5 先例），本轮 db-tools D1 不代改 |

**模块边界（与既有模块分界，沿 D0 与 BA 1.4）**：

| 事项 | 归属 |
|------|------|
| 工具本体部署（下载/安装/升级/卸载/任务台账） | db-tools（本模块 C-15） |
| 工具安全访问（gating/signon/basic auth 注入/打开/轮换） | db-tools（本模块 S-59） |
| 运行环境本体（PHP-FPM/Node/nginx 软件包安装） | env-mgmt（环境层；本模块仅消费实况与引导，不代装环境） |
| 工具路径 nginx 配置落盘与安全写入 | site-mgmt（站点层通道；本模块为消费方，配置语义归本模块 D3） |
| 自动落地块配置语义（块模板/server_name 联动/Port/清块判定，v1.2） | db-tools（本模块 C-15-05；落盘通道仍归 site-mgmt ConfigApply/ConfigRemove） |
| 面板入口网关豁免规则（HostGuard/EntryPathGuard 内部端点路径前缀，v1.2） | panel-core（S-07 机制侧；本模块为消费方/需求提出方，跨模块待同步项） |
| 面板配置变更通知钩子提供（bindDomain 变更事件，v1.2） | panel-core（S-11-01 配置权威侧；本模块注册消费，跨模块待同步项） |
| 工具端口防火墙放行（v1.2） | 本轮仅安装成功提示（OUT 自动放行；firewall 模块联动归后续） |
| mongo-express 服务单元通用治理（全部系统服务清单） | service-mgmt（通用单元层；本模块仅工具单元直达操作） |
| 数据库实例/凭据/库/用户治理台账 | database-mgmt（实例层；本模块消费凭据 OHS，不接管治理） |
| 面板内表/文档浏览（轻量入口） | database-mgmt S-56/S-57（保留并存，Q3/⚠️A13） |
| 高级管理入口按钮渲染（实例治理页/库行） | database-mgmt PageSpec（跨模块待同步项，消费本模块打开能力） |
| 面板会话签发/吊销/角色体系 | account-auth（本模块消费会话校验，不自建会话） |
| Python 环境/其他工具/Redis 工具/readonly 细粒度映射 | 不做（OUT，BA 1.4） |

**D1 设计口径落定（承接 BA ⚠️A1~A17，D2/D3 细化落点）**：

| 编号 | 口径 | 说明 |
|------|------|------|
| ⚠️A1 | 两工具静态目录（phpMyAdmin/mongo-express），版本钉死编译期 | D0 Q1 拍板；目录扩展走面板版本迭代 |
| ⚠️A2 | 依赖前置：PHP-FPM+扩展+nginx / Node+nginx；缺失如实拒绝并引导 | 不静默代装环境；扩展清单 D3 定稿 |
| ⚠️A3 | 异步任务 pending→running→succeeded/failed；同工具互斥；取消 OUT | 任务台账持久化口径归 D2（tool_deploy_task 聚合根候选） |
| ⚠️A4 | 升级=部署目录钉死新版本；配置/凭据保留；上游检测/降级 OUT | 面板版本迭代携带新工具版本 |
| ⚠️A5 | 卸载=解除部署，绝不删数据库数据；强确认+知情提示 | 沿 instanceRemove 哲学 |
| ⚠️A6 | localhost 绑定：mongo-express 127.0.0.1:8081；phpMyAdmin 无独立端口（**v1.1 修订：工具进程 localhost 不变；nginx 接入层自动块 listen 18090 为新增公网面，每请求 gating**） | 端口编译期定死；工具进程无公网直暴露 |
| ⚠️A7 | 会话 gating：auth_request 校验会话+admin，200/401（**v1.1 修订：既有块入口路径/域名绑定同链；自动块域名约束经 server_name+404 镜像；内部端点豁免 HostGuard/EntryPathGuard ⚠️A16**） | 鉴权端点仅 loopback 子请求通道 |
| ⚠️A8 | signon 令牌：HMAC 防伪+短时效+一次性+绑定实例；凭据仅 loopback 注入 | 令牌参数（时效/签名算法细节）D3 定 |
| ⚠️A9 | basic auth 面板生成+nginx 注入；Mongo 凭据 vault→systemd env；密文零回显 | 轮换即时生效；I-02 同构 |
| ⚠️A10 | 工具访问整体 admin 专属；readonly 目录/任务可看、工具不可达 | ADR-018 工具面具体化（同终端先例但保留查看面） |
| ⚠️A11 | 安装/升级/卸载/打开/轮换审计（R-03 同事务脱敏）；gating 逐请求不落审计 | 防写放大（ADR-020）；拒绝事件通道归 D3 |
| ⚠️A12 | 下载钉版本+sha256+镜像（DOUGONG_MIRROR 同构）；校验失败=failed | R-09 用户主动发起例外（与 ACME 同判） |
| ⚠️A13 | 轻量入口（面板内浏览）保留并存；治理台账仍归 database-mgmt | D0 Q3 拍板 |
| ⚠️A14 | 自动落地：0 匹配自动建 dougong-tools.conf listen 18090（ToolLandingPort 编译期常量）；占用→DBT_012 reason；既有块优先/多匹配引导不变；幂等复用两工具共用；siteRef+Port+AutoCreated；两工具卸载清块；ConfigApply R-06；端口放行仅提示 | v1.1 新增（D0 Q1）；块模板/端口占用探测方式归 D3 |
| ⚠️A15 | 域名联动：未绑 server_name _ / 已绑 BindDomain+非匹配 Host 404（镜像 HostGuard）；同步=钩子主通道+install/open 惰性兜底（D1 裁定）；toolBaseHost Port≠0 → scheme://hostname:Port | v1.1 新增（D0 Q1 二次拍板）；钩子契约签名归 D3 |
| ⚠️A16 | 内部端点豁免：HostGuard/EntryPathGuard 对 /dbtool/gate/、/dbtool/signon/ 最小化直通；非内部路径强校验不变；安全由 loopback+秘密头+会话/令牌承载 | v1.1 新增（D0 Q1b 必修 bug）；落点归 panel-core（跨模块待同步项） |
| ⚠️A17 | reason 可见：worker finish 错误文本含 errcode data.reason 拼接，任务详情可见具体原因 | v1.1 新增（D0 Q2）；沿 env-mgmt worker 错误形态先例 |

**D1 遗留待对齐项（归 D2/D3 裁定，不阻塞本轮归档）**：

| # | 事项 | 保守口径（暂按） | 裁定归属 |
|---|------|----------------|---------|
| 待对齐-1 | mongo-express 注入凭据在 database-mgmt 实例凭据变更后的刷新时机（systemd env 为部署时快照，非实时） | 升级/重装工具时刷新注入（不自动传播）；phpMyAdmin signon 无此问题（每次打开实时取 vault） | D3（是否需凭据变更联动刷新通道） |
| 待对齐-2 | gating 鉴权端点契约形态 | **D3 已定稿关闭（api D-11，v1.1 W2 回填）**：db-tools 自持 GET /dbtool/gate/verify，200 放行/401 拒绝，纯读校验，nginx internal location 子请求专用，复用 account-auth 既有 VerifyWs 只读会话校验（account-auth 零新增 HTTP） | D3 已关闭 |
| 待对齐-3 | 工具路径形态（面板域下 location 前缀 vs 独立 server 块）与入口路径/域名绑定约束的具体叠加 | **D1 已裁定关闭（v1.2，自动落地增量）**：既有匹配块=面板域下路径前缀反代（不变）；0 匹配=独立自动 server 块（dougong-tools.conf listen 18090，⚠️A14）；约束叠加=既有块 HostGuard/EntryPathGuard 同链、自动块 server_name+非匹配 Host 404 镜像 HostGuard（⚠️A15）、内部 2 端点豁免两网关（⚠️A16）、入口路径约束不适用独立工具端口 | D1 已关闭（块模板细节归 D3 待对齐-5） |
| 待对齐-4 | PHP 扩展依赖清单与检测方式、Node LTS 具体版本号、mongo-express 端口/目录等编译期常量 | 扩展清单 mysqli/mbstring/zip/gd/json/curl 等（D0 列举）；端口 8081（D0 列举） | D3 常量表定稿 |
| 待对齐-5（v1.2 新增） | 自动落地块模板细节（gate/tool location 排布、404 镜像写法 if/default_server）、ToolLandingPort 占用探测方式（listen 探针 vs nginx -t 失败归类）、panel-core 配置变更钩子契约签名与注册点、惰性校正触发点细节、DBT_012 reason 文案细分 | 保守口径：块模板沿既有注入模板同构（templates.go）；占用探测先 listen 探针后 nginx -t 兜底；钩子=进程内观察者（main 装配注册，沿 BootstrapAfterInstall OHS 先例）；惰性校正=install/open 时按当前 BindDomain 比对块 server_name 不一致则 ConfigApply 刷新 | D3（api v1.3 常量表/契约/错误码 reason 定稿） |

### 附录C：安全架构（显式专节，承接 BA 2.4 五道防线〔v1.2 四→五〕）

> 本节为安全底线的应用架构落点：防线→承载子模块→消费契约的映射。任何裁剪不得削弱本节约束；D3 须将本节约束落为接口/配置/测试口径。

| 防线 | 约束 | 承载子模块 | 跨模块消费/资产复用 |
|------|------|-----------|-------------------|
| 一：网络面收敛 | 工具进程仅绑 localhost（mongo-express 127.0.0.1:8081；phpMyAdmin 无独立端口）；工具进程无独立公网端口/防火墙放行；唯一路径=面板 nginx 工具路径反代（既有面板域落地块或自动落地块）；**v1.2：新增公网面=仅自动块 nginx 接入层 listen 0.0.0.0:18090（每请求受防线二/五约束）；自动块已绑域名时 server_name=BindDomain+非匹配 Host 404 镜像 HostGuard（绑域名后工具也只认域名）；ToolLandingPort 仅提示放行不自动配置防火墙** | C-15-01（部署绑定）/C-15-05（自动块 listen/server_name，v1.2）/S-59-01（反代） | site-mgmt 站点通道（面板域路径+自动块 ConfigApply）；panel-core HostGuard/EntryPathGuard 同链（既有块）/server_name 404 镜像（自动块，S-07-03 语义） |
| 二：会话 gating | auth_request 逐请求校验面板会话 cookie+admin 角色，200/401（**含自动块 18090 每请求，v1.2**）；鉴权端点仅 loopback 子请求通道不对外暴露；纯读校验防 CSRF/越权（服务端权威角色判定）；会话吊销/改密级联即时切断；**v1.2 内部端点豁免（⚠️A16）：HostGuard/EntryPathGuard 对 /dbtool/gate/、/dbtool/signon/ 两路径前缀最小化直通——豁免 Host/入口路径校验而非鉴权，loopback+秘密头+会话/令牌防护链不变，非内部路径强校验不变（IP 直连面板仍 404，fail-closed 不变）** | S-59-01/S-59-02（signon 回调端点） | account-auth S-03 既有只读会话校验 OHS（VerifyWs，db-tools 自持 GET /dbtool/gate/verify 消费，account-auth 零新增 HTTP，api D-11，v1.1 W2 回填）；site-mgmt S-30 安全写入（auth_request 配置）；**panel-core S-07-02/S-07-03 豁免规则（跨模块待同步项，v1.2）** |
| 三：凭据零回显与桥接防伪 | 三类凭据（连接凭据/basic auth/signon 密钥）AES-256-GCM 密文零回显（I-02）；signon 令牌 HMAC+短时效+一次性+绑定实例；凭据仅 loopback/systemd env 服务端通道注入，永不进浏览器；轮换即时失效 | S-59-02/S-59-03/C-15-01（凭据生成） | database-mgmt 实例凭据 OHS（vault 取出）；CredentialVault AES-GCM 资产同构（vault.go:13-19 先例） |
| 四：变更安全与数据保全 | sha256 强制校验失败=failed；R-08 参数数组；R-03 同事务审计脱敏；卸载不删数据库数据；R-09 零遥测（下载为用户主动例外，工具本体遥测配置关闭）；**v1.2：任务失败原因含 data.reason 如实可见（⚠️A17，不吞原因不虚构成功）** | C-15-01/02/03/04 | install.sh sha256+镜像先例（install.sh:122-129/:38-45）；account-auth S-04 审计；env-mgmt worker 错误形态先例（reason 拼接） |
| 五：自动落地块安全（v1.2 新增，⚠️A14/A15） | 自动块公网可达但每请求 auth_request gating（无会话/readonly→401）；域名联动强制（已绑域名非匹配 Host 404，钩子+惰性双通道不留长期不一致窗口）；写入/刷新/清块经 ConfigApply/ConfigRemove（nginx -t+reload+失败字节复原 R-06，不产生半配置）；端口占用如实 DBT_012 reason（不静默换端口）；多匹配引导不擅自选择；幂等复用不重复建块；两工具均卸载清块（AutoCreated 判定，非自动块不动——不误删用户站点）；signon HMAC+60s+一次性+双绑定/凭据 vault 零回显/gate loopback+秘密头双防护在自动块形态全量适用；端口放行仅提示 | C-15-05（建块/联动/清块）/C-15-01/03（宿主）/S-59-01（gating）/S-59-04（惰性校正+URL） | site-mgmt S-30 ConfigApply/ConfigRemove（configpipeline.go:18/:29）；panel-core S-11-01 配置变更钩子（待同步项）；panel-core HostGuard 语义镜像（S-07-03） |

**安全验收口径（供 Gate 1/Gate 2，承接 D0 第六部分 + 自动落地 D0 第四部分）**：无会话/非 admin 访问工具路径 → 401/403 实测（**含自动块 18090 端口，v1.2**）；localhost 绑定实证（公网端口扫描无工具进程端口；18090 为 nginx 接入层受 gating）；signon 令牌过期/重放/跨实例/篡改四路拒绝回归；凭据零回显（响应/日志/审计 detail 扫描）；卸载后数据库数据完整性实证；**v1.2 自动落地专项**：0 匹配自动建块回归（fake nginx 替身：listen 18090/server_name 跟随绑定/gate+tool location）；端口占用→DBT_012 reason；既有块优先/多匹配引导/幂等复用/卸载清块；域名联动回归（未绑 _ /绑定后 server_name=域名+非匹配 Host 404/绑定解绑同步刷新）；**HostGuard 豁免回归（绑域名后内部 gate/signon 端点 loopback+秘密头仍可达不被 404；非内部路径强校验不变 IP 直连面板仍 404；豁免范围最小化）**；toolBaseHost Port 分支回归（IP:18090/域名:18090 URL）；worker reason 可见回归（task.Error 含 reason 文本）。

## 更新记录

| 版本 | 日期 | 更新说明 |
|------|------|---------|
| v1.0 | 2026-11-19 | 初始生成：3 个 BA L3 按 DDD 归类为核心业务域（C-15 工具部署与变更，tool_deploy_task 聚合根候选四态状态机）/支撑业务域（S-58 工具目录与实况、S-59 工具安全访问），10 个 L4 映射为功能子模块（3 个机制型：S-58-02/S-59-01/S-59-02），标注用户类型与调用关系；基础数据域/平台支撑域经判定为空并注明理由；承接既有编码 C-01~C-14、S-01~S-57（已核验 C-15/S-58/S-59 无占用）；跨模块消费五条登记（env-mgmt 环境实况/Node 供给、site-mgmt 站点通道、service-mgmt SysdGateway、database-mgmt 实例凭据 OHS、account-auth 会话校验+审计）+被消费一条（database-mgmt 高级管理入口，待同步项）；安全架构显式成节（附录C 四道防线→子模块映射+验收口径）；落定 ⚠️A1~A13 口径表与模块边界表；附录B +D1 遗留待对齐项 4（凭据刷新时机/gating 端点契约/工具路径形态/编译期常量，归 D2/D3 裁定不阻塞归档） |
| v1.1 | 2026-11-19 | Gate 1 W2 修复轮（措辞回填，gate1-db-tools-2026-11-19.md）：gating 端点归属由「account-auth 新增内部会话校验能力跨模块待同步」回填为 api D-11 定稿「db-tools 自持 GET /dbtool/gate/verify + 复用 account-auth 既有只读会话校验 OHS VerifyWs（account-auth 零新增 HTTP，OHS 消费已登记 api-account-auth.md v1.2）」——S-59-01 调用关系/附录B 跨模块契约/附录C 防线二消费列/待对齐-2（标记 D3 已关闭）四处同步；应用域/功能模块/子模块集合/编码 C-15·S-58·S-59/用户类型/安全四防线约束均零变更（不削弱安全架构） |
| v1.2 | 2026-11-19 | 自动 nginx 落地增量 D1（BA v1.1 ⚠️A14~A17，d0-clarification-dbtools-auto-landing-2026-11-19.md Q1/Q1b/Q2/Q3）：新增功能子模块 C-15-05 工具落地自动供给（机制型，BA L4 2.2.1.5 映射，子模块 10→11；0 匹配自动建 dougong-tools.conf listen 18090/域名联动 server_name 跟随绑定+非匹配 Host 404 镜像 HostGuard/既有块优先/多匹配引导不变/幂等复用/siteRef+Port+AutoCreated/两工具卸载清块/ConfigApply R-06/端口放行仅提示）；**D1 裁定域名同步=panel-core 配置变更钩子主通道+install/open 惰性校正兜底**（钩子失败不阻断配置变更，记日志+自愈）；C-15-01/03/04、S-59-01/02/04 六既有子模块口径扩展（落地选择/清块/reason 可见 ⚠️A17/内部端点豁免 ⚠️A16/Port URL 分支+惰性校正 ⚠️A15）；跨模块消费五模块→六模块（+panel-core 两条契约：配置变更通知钩子 S-11-01/HostGuard·EntryPathGuard 内部端点豁免 S-07——均登记 panel-core 三文档跨模块待同步项，本轮不代改）；附录A +C-15-05 机制映射行；附录B 契约表 +2 行+site-mgmt 行扩 ConfigApply/边界表 +4 行/口径表 +A14~A17+A6/A7 修订注记/待对齐-3 裁定关闭+待对齐-5 新增（块模板/占用探测/钩子签名/惰性触发点/reason 文案归 D3）；附录C 安全架构四→五道防线（防线一/二扩展+防线五自动落地块安全）+安全验收口径 +自动落地/域名联动/HostGuard 豁免/Port URL/reason 五专项；DDD 归类/应用域/模块编码 C-15·S-58·S-59/用户类型不变；第三部分树同步；安全约束零削弱（gating/signon/vault/localhost/卸载保全全量适用自动块形态） |
