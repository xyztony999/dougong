# 斗拱面板 数据库工具集成（db-tools）PRD

| 属性 | 值 |
|------|-----|
| 版本 | v1.2 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-19（v1.2 自动 nginx 落地增量轮：+FR-11~14 自动落地/域名联动/HostGuard 内部端点豁免/reason 可见，D0 拍板 Q1/Q1b/Q2/Q3） |
| 输入 | D0 澄清拍板（d0-clarification-db-tools-integration-2026-11-19.md，Q1 一步到位/Q2 会话 gating 反代/Q3 轻量入口并存 + 安全底线；**v1.2 增量输入：d0-clarification-dbtools-auto-landing-2026-11-19.md，Q1 自动落地+域名联动/Q1b HostGuard 内部端点豁免/Q2 reason 可见/Q3 安全不变**）、D1 三文档（business-arch-db-tools.md v1.1/application-arch-db-tools.md v1.2/function-list-db-tools.md v1.1，⚠️A1~A17 + 安全架构 BA 2.4 五道防线/AA 附录C）、domain-model.md v1.25（v1.26 本轮 D2 增量落盘）、prd-env-mgmt.md v1.3（Node 环境 FR-08）、idea.md §3/§7（R-02/R-03/R-08/R-09/R-14/ADR-018）、install.sh 下载分发先例（钉版本+sha256+DOUGONG_MIRROR 镜像） |
| 下游 | D3 API 设计（02-detailed/api/api-db-tools.md）、D4 交互设计（02-detailed/interaction/{admin,readonly}/db-tools/） |

## 第一部分：业务目标（G1）

为登录运维者提供**数据库高级管理的现成工具集成能力**：面板自 v0.1.12/13 已具备面板内表/文档浏览与基础 CRUD（database-mgmt S-56/S-57 轻量入口），但深度数据库管理（复杂 SQL 编辑、导入导出、索引与结构可视化、聚合查询等）自造一套成本高且难追专业工具。本模块走宝塔式路线（用户 2026-11-19 拍板，ADR-032）——**集成现成 Web 工具**：phpMyAdmin（PHP，管 MySQL/MariaDB）与 mongo-express（Node，管 MongoDB），复用面板已管理的运行环境（env-mgmt PHP-FPM 已就绪 + 本轮新增 Node 环境〔prd-env-mgmt.md FR-08〕）与站点通道（site-mgmt php_fpm/reverse_proxy 站点），不违背「环境管理本就要装运行时」的产品哲学。

**访问安全为产品底线（Q2 拍板，不可妥协）**：工具仅绑 localhost 不暴露独立公网端口，唯一访问路径=面板域下工具路径反代，经 nginx auth_request 校验面板会话 + admin 角色放行；phpMyAdmin 以 signon 桥接面板会话（签名令牌防伪 + vault MySQL 凭据服务端注入，凭据永不进浏览器）；mongo-express 以面板生成的 basic auth 经 nginx 服务端注入（浏览器无感知）。凭据 AES-256-GCM 密文零回显（I-02）；卸载工具绝不删除数据库数据；下载钉版本 + sha256 校验；零遥测（R-09，下载为用户主动发起例外）。

**与轻量入口并存（Q3 拍板）**：面板内表/文档浏览保留作快速查看/基础 CRUD 轻量入口，集成工具作高级管理入口；实例/凭据/库/用户治理仍留面板（database-mgmt），工具不接管治理台账。

**v1.2 增量目标（自动 nginx 落地轮，d0-clarification-dbtools-auto-landing-2026-11-19.md）**：实机 v0.1.15 取证根因——用户经 **IP+端口直连面板、未绑域名**（install.sh Go 直连形态），nginx 无面板域 server 块，落点选择 selectLanding 0 匹配→两工具安装均 DBT_012 拒绝；次生 UX 缺陷——worker finish 错误文本仅「DBT_012: 部署操作失败」吞 data.reason，任务详情看不到具体原因。本轮四项拍板：**Q1 自动落地**——0 匹配时自动创建专用 nginx 工具块（listen 18090 编译期常量）让 IP 直连形态安装即可用，**域名联动**对齐宝塔「绑域名后仅域名访问」语义（未绑 server_name _ /已绑 BindDomain+非匹配 Host 404 镜像 HostGuard）；**Q1b 必修 bug**——HostGuard 全局包装器（cmd/dougong/main.go:487-488）误拦内部 gate/verify·signon/verify 端点，绑域名后 auth_request 子请求 Host≠域名被 404 致 gating 失效，修复=内部 2 端点路径最小化豁免 HostGuard/EntryPathGuard（安全由 loopback+秘密头+会话/令牌承载）；**Q2 reason 可见**——任务失败错误文本含 errcode data.reason；**Q3 安全不变**——工具本体仅 localhost、自动块每请求 auth_request gating、signon 防伪/凭据零回显/ConfigApply R-06 全量适用。

与既有模块的分界（D1 AA 附录B）：工具本体部署与安全访问=本模块；运行环境本体（PHP-FPM/Node/nginx 软件包）=env-mgmt；工具路径 nginx 配置落盘通道=site-mgmt；mongo-express 服务单元通用治理=service-mgmt；数据库实例/凭据/库/用户治理台账=database-mgmt（本模块消费其凭据 OHS）；面板会话签发/吊销/角色体系=account-auth（本模块消费其会话校验与审计）。服务对象：管理员（admin）执行全部变更与工具访问；只读用户（readonly）查看工具目录与任务状态输出（ADR-018），**工具本体不可达**（gating 角色拒绝，⚠️A10）。

## 第二部分：功能范围（G2）

**IN（做什么）**：

| # | 能力 | 依据 |
|---|------|------|
| 1 | 工具列表查看：内置两工具目录条目（phpMyAdmin/mongo-express，显示名/描述/目标引擎/钉死版本）+ 实况（部署状态/已装版本/运行态/依赖就绪度/在途任务标记）合并呈现 | D1 AA S-58-01（⚠️A1/A2） |
| 2 | 工具实况探测（机制）：逐工具部署状态与版本查询（程序目录/npm 包）+ 运行态查询（mongo-express systemd 单元；phpMyAdmin 站点与 PHP-FPM pool 可用性）+ 依赖就绪度（消费 env-mgmt 环境实况：PHP-FPM/Node/nginx + PHP 扩展检测）；纯读每请求实时探测不缓存 | D1 AA S-58-02 |
| 3 | 工具安装：依赖前置检测（缺失如实拒绝并引导到环境管理）→ 下载分发（钉版本+sha256+镜像，⚠️A12）→ 部署落位（phpMyAdmin 解压+config.inc.php+php_fpm 站点；mongo-express npm 安装+systemd 服务+reverse_proxy 站点）→ 工具凭据生成密文落库 → 异步任务执行 | D1 AA C-15-01（⚠️A2/A3/A6/A9/A12） |
| 4 | 工具升级：部署到面板目录钉死新版本（phpMyAdmin 替换程序目录保留配置与 signon 密钥；mongo-express 重装 npm 包重启服务）；凭据与站点路径不变；异步任务执行 | D1 AA C-15-02（⚠️A4） |
| 5 | 工具卸载：强确认（知情提示：高级入口不可用，轻量浏览与数据库数据不受影响）→ 解除部署（程序目录/npm 包+服务+站点配置+凭据密文）→ **绝不删除数据库数据** → 异步任务执行 | D1 AA C-15-03（⚠️A5） |
| 6 | 工具部署任务跟踪：任务状态（pending → running → succeeded/failed）、动作、目标工具、输出尾部（排障）、失败原因；同工具在途互斥；任务台账跨重启留存 | D1 AA C-15-04（⚠️A3） |
| 7 | 会话 gating 反代（机制）：工具路径每请求经 auth_request 校验面板会话 cookie + admin 角色（200 放行/401 拒绝）；鉴权端点仅 loopback 子请求通道；工具仅绑 localhost | D1 AA S-59-01（⚠️A6/A7/A10） |
| 8 | phpMyAdmin signon 桥接（机制）：打开时签发签名令牌（HMAC 防伪+短时效+一次性+绑定实例）→ signon 服务端回调校验 → vault MySQL 凭据经 loopback 注入 phpMyAdmin 会话（凭据永不进浏览器） | D1 AA S-59-02（⚠️A8） |
| 9 | mongo-express 凭据注入与轮换：basic auth 面板生成经 nginx 注入 Authorization 头（浏览器无感知）；Mongo 连接凭据 vault→systemd env 注入；basic auth 口令轮换（admin 显式动作，旧口令即时失效） | D1 AA S-59-03（⚠️A9） |
| 10 | 工具打开入口：工具管理页「打开」与 database-mgmt 高级管理入口共用打开能力（phpMyAdmin 走 signon、mongo-express 走 gating 直达）；未装引导安装；打开动作审计；**v1.2：打开 URL 按落点端口分支（自动块形态 http://IP:18090/... 或 http://域名:18090/...）+打开时惰性校正自动块 server_name** | D1 AA S-59-04（⚠️A11/A15） |
| 11 | **工具落地自动供给（v1.2 新增，机制）**：落点 0 匹配（IP 直连未绑域名）自动创建专用 nginx 工具块（{conf.d}/dougong-tools.conf listen ToolLandingPort=18090 编译期常量，绑定 0.0.0.0）；端口占用→DBT_012 reason 如实；既有匹配块优先沿用/多匹配仍引导；幂等复用两工具共用；siteRef +Port+AutoCreated；两工具均卸载后清自动块（非自动块不动）；写入/清块经 site-mgmt ConfigApply/ConfigRemove（R-06）；安装成功提示放行端口（仅提示不自动放行） | D1 AA C-15-05（⚠️A14） |
| 12 | **域名联动（v1.2 新增，机制）**：自动块 server_name 跟随面板域名绑定状态（未绑 _ /已绑 BindDomain+非匹配 Host 404 镜像 HostGuard，绑域名后工具也只认域名）；域名变更同步=panel-core 配置变更钩子主通道+工具 install/open 惰性校正兜底（D1 裁定） | D1 AA C-15-05/S-59-04（⚠️A15） |
| 13 | **HostGuard/EntryPathGuard 内部端点豁免（v1.2 新增，panel-core 机制侧变更，跨模块待同步项）**：全局包装器对内部 2 端点路径前缀 /dbtool/gate/、/dbtool/signon/ 直通放行（修复绑域名后 gating/signon 失效）；豁免范围最小化，非内部路径强校验不变；内部端点安全由 loopback+秘密头+会话/令牌承载 | D0 Q1b / D1 AA S-59-01/S-59-02（⚠️A16） |
| 14 | **失败原因可见（v1.2 新增）**：worker finish 错误文本含 errcode data.reason 拼接——任务详情可见具体失败原因（DBT_012 端口占用/多候选/扫描失败等不再吞 reason），沿 env-mgmt worker 错误形态先例 | D0 Q2 / D1 AA C-15-04（⚠️A17） |

**OUT（不做什么）**：

| # | 事项 | 归属 |
|---|------|------|
| 1 | Python 环境与其他工具（phpRedisAdmin/redis-commander/Adminer/pgAdmin） | 不做（D0 范围边界；目录扩展走面板版本迭代 ⚠️A1） |
| 2 | Redis 集成工具 | 不做（D0 范围边界；保留面板 key 浏览） |
| 3 | 工具内细粒度权限映射到面板 readonly | 不做（D0 范围边界；工具访问整体 admin 专属 ⚠️A10） |
| 4 | 面板内表/文档浏览下线 | 不做（Q3 保留并存 ⚠️A13；轻量入口归 database-mgmt S-56/S-57） |
| 5 | 工具上游新版本检测/降级/自选版本 | 不做（⚠️A4 OUT；升级=部署目录钉死新版本，用户主动发起） |
| 6 | 任务取消 | 不做（⚠️A3 OUT；下载/npm 中断态不安全，沿 env-mgmt 同判） |
| 7 | 工具本体汉化/主题定制 | 不做（集成现成工具原样形态） |
| 8 | 运行环境代装（PHP-FPM/Node/nginx 缺失时静默安装） | 不做（⚠️A2 依赖前置如实拒绝+软引导到 env-mgmt；环境供给归 env-mgmt） |
| 9 | 实例/凭据/库/用户治理台账 | database-mgmt（实例层；本模块仅消费凭据 OHS，不接管治理） |
| 10 | 工具路径独立域名/工具进程独立公网端口 | 不做（⚠️A6/A7 工具进程 localhost 绑定+面板 nginx 工具路径反代唯一入口；**v1.2 修订：自动落地块 listen 18090 为 nginx 接入层端口非工具进程端口，每请求 gating，不构成工具公网直暴露**） |
| 11 | Windows/无 systemd 环境支持 | 不做（install.sh 支持矩阵外，如实报错） |
| 12 | 工具页面外壳与导航挂载 | panel-core（应用骨架与导航） |
| 13 | database-mgmt 高级管理入口按钮渲染 | database-mgmt PageSpec（跨模块待同步项，D1 AA 附录B；本模块提供状态与打开能力） |
| 14 | **自动配置防火墙放行（v1.2）** | 不做（自动落地 D0 范围边界；firewall 模块联动归后续，本轮仅安装成功提示放行 ToolLandingPort） |
| 15 | **工具端口用户可配（v1.2）** | 不做（ToolLandingPort=18090 编译期常量；可配归后续） |
| 16 | **HTTPS 工具落地（v1.2）** | 不做（自动块 http；TLS 归绑域名形态既有能力） |
| 17 | **面板自身经 nginx 承载的改造（v1.2）** | 不做（不动面板 Go 直连形态；自动块仅承载工具路径，面板本体仍 Go 直连） |

## 第三部分：角色定义（G3）

| 角色 | 英文 | 职责与权限 | 获得方式 |
|------|------|-----------|---------|
| 管理员 | admin | 查看工具全景与任务 + 全部变更类操作（安装/升级/卸载/凭据轮换）+ 工具访问（打开/gating 放行/signon 桥接） | account-auth 初始化/用户管理创建 |
| 只读用户 | readonly | 同等查看工具列表（目录+实况）与任务状态/输出（ADR-018 全可看）；一切变更无入口 + 越界请求被全局边界管控拒绝；**工具本体不可达**——gating 角色校验拒绝（401/403，⚠️A10），打开/轮换入口不渲染 | 管理员创建并指派 |

访客（未认证操作者）不涉及：本模块全部功能位于登录会话之后（R-02）；工具路径无会话经 gating 拒绝（⚠️A7）。角色差异边界：全部写类操作与工具访问为 admin 专属；查看类动作不写审计（R-03 语义为「变更」留痕）；异步任务终态由任务执行链写审计；gating 逐请求校验不落业务审计（⚠️A11 防写放大）。

## 第四部分：功能需求明细

> 与 AA（application-arch-db-tools.md v1.2）11 个功能子模块对应（FR-01~10 对应既有 10 子模块，**FR-11/12 对应 v1.2 新增 C-15-05 工具落地自动供给〔机制型〕，FR-13 为 panel-core 机制侧变更需求〔跨模块待同步项，本模块消费〕，FR-14 为 C-15-04 错误文本构造扩展**），覆盖 FL（function-list-db-tools.md v1.1）admin 8 个菜单行（readonly 2 个只读行同 FR 只读视角，零新增菜单）；变更类 FR 全部记审计；安全架构五道防线（BA 2.4/AA 附录C，v1.2 四→五）映射见第五部分末表。

### 4.1 工具目录与实况（S-58）

| 编号 | 需求 | 主流程与规则 | 审计 |
|------|------|-------------|------|
| FR-01 | 工具列表查看（S-58-01） | 呈现内置两工具（⚠️A1：phpMyAdmin→MySQL/MariaDB、mongo-express→MongoDB）全景列表：显示名、描述、目标引擎、目录钉死版本、部署状态（未装/已装+已装版本）、运行态、依赖就绪度（PHP-FPM/扩展/Node/nginx）、在途任务标记；实况每请求实时探测不缓存不虚构；未装条目呈现安装入口、已装呈现升级/卸载/打开入口（admin；readonly 负空间）；两角色同等可看（ADR-018） | —（查看类） |
| FR-02 | 工具实况探测（S-58-02，机制型） | 逐工具探测三路：①部署状态与版本——phpMyAdmin 查管理目录（面板托管路径，目录存在+版本文件读取）、mongo-express 查 npm 包（面板托管安装形态，版本经包元数据）；②运行态——mongo-express 经 systemd 单元查询（复用 service-mgmt SysdGateway 适配层资产）、phpMyAdmin 经工具站点与 PHP-FPM pool 可用性判定（无独立进程，站点可达即运行态成立）；③依赖就绪度——消费 env-mgmt 环境实况（进程内读契约：PHP-FPM/Node/nginx 已装态与版本）+ PHP 扩展检测（phpMyAdmin 所需 mysqli/mbstring/zip/gd/json/curl 等，清单 D3 定稿）；纯读机制，被列表/安装/升级/卸载/打开共同消费；探测失败如实呈现（不虚构已装/运行态） | —（查看类机制） |

### 4.2 工具部署与变更（C-15）

| 编号 | 需求 | 主流程与规则 | 审计 |
|------|------|-------------|------|
| FR-03 | 工具安装（C-15-01） | 安装目录工具：①依赖前置检测（⚠️A2）——phpMyAdmin 需 PHP-FPM 已装+扩展齐备+nginx 已装；mongo-express 需 Node 已装（env-mgmt 本轮 +Node，prd-env-mgmt.md FR-08）+nginx 已装；缺失如实拒绝并呈现到环境管理的安装引导（不静默代装，OUT-8）；②发起即创建异步任务（pending，⚠️A3）并立即返回 taskId；③任务执行链：**phpMyAdmin**——下载官方 tarball（钉版本+sha256 强制校验+DOUGONG_MIRROR 同构镜像选项，⚠️A12；校验失败=任务 failed 不降级不跳过）→解压到面板管理目录→生成 config.inc.php（signon 认证模式配置，⚠️A8）→经 site-mgmt 建 php_fpm 站点（面板域工具路径+auth_request gating 配置，⚠️A7）；**mongo-express**——npm 安装精确版本（registry 镜像选项同构）→建 systemd 服务（env 注入：Mongo 连接凭据取自 vault〔database-mgmt 实例凭据 OHS〕+面板生成 basic auth+baseUrl 路径前缀+bind 127.0.0.1:8081，⚠️A6/A9）→经 site-mgmt 建 reverse_proxy 站点（上游 127.0.0.1:8081+auth_request gating+Authorization 头注入）；**落地选择（v1.2 修订，FR-11 机制）**：既有匹配块（绑域名 server_name 匹配/反代面板）优先沿用不新建；0 匹配（IP 直连未绑域名）经 FR-11 自动建专用块（端口占用→DBT_012 data.reason 如实）；多匹配仍 DBT_012 列候选引导明确；落点引用 siteRef +Port+AutoCreated（值对象扩展，零 DDL 变更）；自动块形态安装成功输出含放行 ToolLandingPort 提示（仅提示不自动放行，OUT-14）；④工具凭据生成（basic auth 随机强口令/signon 密钥 crypto/rand）AES-256-GCM 密文落库（⚠️A9，I-02 零回显）；⑤部署档案登记（db_tool：工具类型/已装版本/部署路径/站点引用/服务单元引用/凭据密文）与任务终态同事务；外部命令一律字面量参数数组（R-08）；任务输出尾部与失败原因经 FR-06 呈现，不虚构成功 | dbt_install_started（创建任务）+ 任务终态审计（change，detail 含工具/版本，**不含凭据**） |
| FR-04 | 工具升级（C-15-02） | 升级已装工具到面板目录钉死新版本（⚠️A4，非拉取上游最新）：phpMyAdmin 下载新 tarball（sha256 校验）替换程序目录，**保留 config.inc.php 与 signon 密钥**（配置不覆盖，conffile 保守先例）；mongo-express 重装 npm 新版本并重启服务（凭据注入形态不变）；升级不改变工具凭据与站点路径；目录版本=已装版本时如实呈现「当前已为目录版本」（不虚构升级）；异步任务执行（同 FR-03 通道）；档案版本字段随终态更新 | dbt_upgrade_started + 任务终态审计 |
| FR-05 | 工具卸载（C-15-03） | 卸载已装工具：强确认交互——知情提示「高级管理入口将不可用，面板内轻量浏览与数据库数据不受影响」（⚠️A5）；解除工具部署：删除程序目录/npm 包、停止并移除 systemd 服务（mongo-express）、经 site-mgmt 移除工具站点配置（含 gating/注入配置，工具 location 撤除）、删除工具凭据密文与部署档案；**两工具均卸载后移除自动落地块（v1.2，FR-11 机制：AutoCreated 标志判定，非自动块不动——不误删用户站点）**；**绝不删除数据库数据**（沿 database-mgmt instanceRemove 哲学——工具仅为访问通道，数据归引擎）；异步任务执行；卸载后工具条目回未装态；未装工具发起卸载如实拒绝（NOT_INSTALLED 语义） | dbt_uninstall_started + 任务终态审计（detail 含知情提示呈现记录，不含凭据） |
| FR-06 | 工具部署任务跟踪（C-15-04） | 异步任务的观测与结果确认：任务状态 pending → running → succeeded / failed（⚠️A3）；呈现目标工具、动作（install/upgrade/uninstall）、发起者、发起时间、命令输出尾部（排障，行数上限编译期常量）、失败原因（sha256 校验失败/依赖缺失/下载失败/npm 失败/站点写入失败等如实分类；**v1.2 ⚠️A17/FR-14：错误文本含 errcode data.reason 具体原因——DBT_012 端口占用/多候选/扫描失败等任务详情可见，不再吞 reason**）；同工具在途任务互斥（pending/running 存在时新变更请求如实拒绝）；任务台账持久化（跨重启留存）；面板重启时在途 running 任务标 failed（error=面板重启中断，如实呈现）；任务取消不做（OUT-6）；两角色可看（ADR-018） | 任务终态结果一并留痕（succeeded/failed 均记） |

### 4.3 工具安全访问（S-59）

| 编号 | 需求 | 主流程与规则 | 审计 |
|------|------|-------------|------|
| FR-07 | 会话 gating 反代（S-59-01，机制型） | 面板 nginx 工具路径的访问闸门：每个工具路径请求（**含自动落地块 18090 端口每请求，v1.2 ⚠️A14**）经 auth_request 子请求到面板鉴权端点——校验面板会话 cookie（有效性/过期/吊销）+ admin 角色，200 放行反代到 localhost 工具上游 / 401 拒绝（⚠️A7）；鉴权端点仅接受 nginx loopback 内部子请求通道（internal location），不对外暴露为可独立调用的公开接口；防 CSRF/越权：gating 为纯读校验（无状态变更），会话 cookie 随子请求原样透传，角色判定服务端权威（不信任任何客户端声明）；**约束叠加（v1.2 修订）**：既有面板域落地块形态下面板入口路径/域名绑定约束同链生效（panel-core HostGuard/EntryPathGuard）；自动块形态下域名约束经块内 server_name+非匹配 Host 404 镜像 HostGuard（FR-12），入口路径约束不适用独立工具端口；**内部端点豁免（v1.2 ⚠️A16/FR-13）**：gate/verify·signon/verify 内部 2 端点路径豁免 HostGuard/EntryPathGuard（绑域名后子请求/回调不被误拦 404），豁免范围最小化、安全由 loopback+秘密头+会话/令牌承载；面板会话吊销/改密级联即时切断工具访问（逐请求实时校验，无独立长时会话）；readonly/无会话一律不可达工具（⚠️A10）；gating 逐请求校验不落业务审计（⚠️A11 防写放大，ADR-020 保留约束）；可行性关键——面板会话 cookie host-only（端口无关），浏览器访问 IP:18090 同样携带会话（D0 取证） | —（逐请求校验不落审计；拒绝事件安全日志通道归 D3） |
| FR-08 | phpMyAdmin signon 桥接（S-59-02，机制型） | phpMyAdmin 无感登录桥接（⚠️A8）：①打开工具时面板生成签名令牌——HMAC 签名防伪（密钥 vault 密文存储）+短时效（60s 量级，D3 定值）+一次性消费防重放+绑定目标实例与会话；②phpMyAdmin signon 流程经服务端回调到面板 API 校验令牌（签名/时效/消费态/实例绑定四重校验；**回调端点 /dbtool/signon/verify 豁免 HostGuard/EntryPathGuard，v1.2 ⚠️A16/FR-13：绑域名后 loopback 回调不被误拦，loopback+秘密头防护不变**）；③校验通过后消费 database-mgmt 实例凭据 OHS 从 vault 取出目标实例 MySQL 凭据，经 loopback 服务端通道注入 phpMyAdmin 会话（SignonScript 服务端回调形态）——**凭据永不进入浏览器**（I-02 零回显：浏览器全程只见令牌不见凭据）；④令牌过期/重放/跨实例重用/篡改一律如实拒绝；⑤目标实例未纳管或凭据未配置 → 打开拒绝并引导到 database-mgmt 配置凭据（不虚构可登录）；⑥令牌消费台账为内存态（短时效+一次性，面板重启全部失效=安全保守，不持久化）；桥接动作（访问开启）审计留痕 | dbt_tool_accessed（访问开启，object_type=db_tool，detail 含工具/实例标识，**不含凭据与令牌明文**；v1.1 W1 回填统一为 api D-16 权威码） |
| FR-09 | mongo-express 凭据注入与轮换（S-59-03） | ①注入机制：面板生成的 basic auth 用户名/口令经 nginx 反代注入 Authorization 头（proxy_set_header，浏览器无感知不手工输入，⚠️A9）；Mongo 连接凭据取自 vault（database-mgmt 实例凭据 OHS）经 systemd 环境注入工具进程（不进 argv/日志）；②口令轮换（admin 显式动作）：重新生成随机强口令→更新服务 env 与站点注入配置（经 site-mgmt 安全写入管线 R-06）→重启服务→旧口令即时失效；③口令与连接凭据全程零回显（不呈现明文/密文，仅呈现轮换结果）；④Mongo 连接凭据于部署/重部署时取自 vault，实例凭据变更后的注入刷新时机归 D3 裁定（保守口径=升级/重装刷新，D1 待对齐-1）；⑤目标实例凭据缺失 → 安装/打开如实拒绝并引导配置 | dbt_credential_rotated（change，detail 含工具标识，**不含口令**） |
| FR-10 | 工具打开入口（S-59-04） | 管理员打开已装工具的统一入口：工具管理页「打开」动作与 database-mgmt 实例治理页/库行「高级管理」入口（跨模块呈现消费，OUT-13）共用同一打开能力——按工具类型分发：phpMyAdmin 走 signon 令牌签发与桥接（FR-08）、mongo-express 走 gating 反代直达（FR-07，basic auth 服务端注入无感知）；打开前置：已装态+运行态依据（FR-02），未装工具呈现安装引导（不虚构可打开）；目标实例凭据就绪校验（FR-08⑤/FR-09⑤）；**打开 URL 基址按落点端口分支（v1.2 ⚠️A15/FR-12）：siteRef.Port≠0 → scheme://hostname:Port（hostname 取 BindDomain 优先，否则 requestHost 去端口——IP 直连 http://IP:18090/dbtool/...，绑域名 http://域名:18090/...），Port=0 沿旧形态 scheme://host；打开时惰性校正自动块 server_name（FR-12 兜底通道）**；打开动作（访问开启）审计留痕（⚠️A11）；readonly 无打开入口（负空间）且工具路径 gating 不可达（⚠️A10） | dbt_tool_accessed（访问开启，同 FR-08；v1.1 W1 回填统一 api D-16 权威码） |

### 4.4 工具落地自动供给与跨模块修复（v1.2 新增，C-15-05 + panel-core 机制侧）

| 编号 | 需求 | 主流程与规则 | 审计 |
|------|------|-------------|------|
| FR-11 | 工具落地自动供给（C-15-05，机制型） | 工具访问落地通道的自动供给与生命周期管理（⚠️A14）：①**自动建块**——落点选择（selectLanding）0 匹配（IP+端口直连面板、未绑域名、nginx 无面板域 server 块）时自动创建专用 nginx 工具 server 块：文件 {nginx conf.d}/dougong-tools.conf（编译期常量路径，沿 site-mgmt 扫描目录），listen ToolLandingPort=18090（编译期常量，避开常用端口，绑定 0.0.0.0），块内含 gating auth_request location 与工具 location；②**端口占用**——ToolLandingPort 被占用→DBT_012 data.reason「工具端口被占用」如实引导释放（不静默换端口，OUT-15 不可配）；③**选择纪律**——既有匹配块（绑域名 server_name 匹配/反代面板 proxyTargetsPanel）优先沿用不新建；多匹配仍 DBT_012 列候选引导到站点管理明确（不变）；④**幂等与共用**——自动块已存在则复用（标记/文件存在判定），两工具共用一块（gate location 去重）；落点引用 siteRef +Port（落地监听端口，既有 80/443 块 Port=0 表默认）+AutoCreated 标志（值对象扩展，JSON 载荷扩字段零 DDL 变更）；⑤**清块**——卸载移除工具 location；两工具均卸载后移除自动块（AutoCreated 判定，非自动块不动——不误删用户站点）；⑥**安全写入**——自动块创建/刷新/移除经 site-mgmt ConfigApply（allowNew）/ConfigRemove 管线（nginx -t 自检+reload，失败字节复原 R-06，不产生半配置）；⑦**端口放行提示**——安装成功输出/文档提示放行 ToolLandingPort（防火墙/云安全组，仅提示不自动放行，OUT-14）；安全边界=自动块公网可达但每请求 auth_request gating（FR-07），工具本体仍仅 localhost（⚠️A6） | 随宿主安装/卸载任务审计（不独立记审计——机制步骤非独立变更动作；清块结果随卸载终态 detail 呈现，不含凭据） |
| FR-12 | 域名联动（C-15-05/S-59-04，机制型） | 自动块 server_name 跟随面板域名绑定状态（⚠️A15，对齐宝塔「绑域名后仅域名访问」语义）：①**未绑域名**——server_name _（IP/任意 Host 可达）；②**已绑域名**——server_name=BindDomain + 非匹配 Host 返回 404（nginx if/default_server 写法 D3 定，镜像 panel-core HostGuard 语义）→绑域名后工具也只认域名，与面板一致；③**域名变更同步（D1 裁定双通道）**——主通道=panel-core 配置变更通知钩子（bindDomain 绑定/解绑成功〔config/update，D-P12〕→进程内通知 db-tools→自动块存在则重渲染 server_name 经 ConfigApply 刷新；钩子刷新失败不阻断配置变更，记日志）；兜底=工具 install/open 时按当前 BindDomain 惰性校正块（不一致则 ConfigApply 刷新，自愈钩子失败/重启窗口）；④**打开 URL 端口分支**——toolBaseHost：siteRef.Port≠0 → base=scheme://hostname:Port（hostname 取 BindDomain 优先，否则 requestHost 去端口），Port=0 沿旧形态 scheme://host（FR-10 消费）；⑤既有面板域落地块形态不受本 FR 影响（其域名约束经 HostGuard 同链，不变） | —（机制型；钩子刷新/惰性校正为配置同步动作，随宿主任务或配置变更审计留痕，不独立新增审计码——D3 定稿） |
| FR-13 | HostGuard/EntryPathGuard 内部端点豁免（panel-core S-07 机制侧，跨模块待同步项） | 必修 bug 修复需求（⚠️A16，D0 Q1b 取证 cmd/dougong/main.go:487-488）：HostGuard 为全局 net/http 包装器（包裹整个 gin 引擎，作用于全部路由），绑域名后 nginx auth_request 子请求/SignonScript 回调的 Host≠域名→HostGuard 标准 404→gating/signon 失效工具全挂；修复=**内部 2 端点路径前缀（/dbtool/gate/、/dbtool/signon/）豁免 HostGuard 与 EntryPathGuard 直通放行**；豁免范围最小化（仅内部 2 路径前缀，非内部路径强校验不变——IP 直连面板仍 404，fail-closed 不变）；豁免的是 Host/入口路径校验而非鉴权——内部端点安全由既有 loopback 源+秘密头+会话/令牌防护链承载（api/dbtools/module.go:64/:208 不变）；**落点归 panel-core（S-07-02/S-07-03 机制侧变更），panel-core 三文档同步登记为跨模块待同步项，本轮 db-tools D2 不代改其 PRD** | —（网关机制变更归 panel-core 审计口径；内部端点拒绝事件沿安全日志通道） |
| FR-14 | 失败原因可见（C-15-04 错误文本构造扩展） | worker finish 错误文本构造修复（⚠️A17，D0 Q2 取证 deploy.go:200-204 `msg := opErr.Error()` 仅 symbol+message 吞 data.reason）：任务 failed 时错误文本须含 errcode data.reason 拼接（errcode.Error 取 Data["reason"]，沿 env-mgmt worker 错误形态先例）——任务详情可见具体失败原因（DBT_012 端口占用/多候选/扫描失败等如实呈现，不再仅「部署操作失败」）；无 reason 载荷的错误沿既有 symbol+message 形态（不虚构原因）；输出尾部与终态定格语义不变（FR-06） | —（错误文本构造为任务终态落库的一部分，沿 FR-06 终态审计，不新增审计码） |

## 第五部分：业务规则汇总

| 编号 | 规则 | 来源 |
|------|------|------|
| R-03 | 全部变更类操作写审计：安装/升级/卸载发起与终态、凭据轮换、打开（访问开启）；任务终态结果一并留痕；同事务脱敏（detail 不含凭据/口令/令牌明文）；经 account-auth 审计契约（OHS） | idea.md §4.1 M1 / kb / D0 安全底线 |
| R-08 | 外部命令一律字面量参数数组：npm/systemctl/解压/sha256 校验等——不拼 shell 字符串、不注入用户输入 | idea.md §7 / kb / D0 安全底线 |
| R-09 | 零遥测：面板自身不主动外呼；工具分发下载（tarball/npm）为**用户主动发起例外**（与 ACME 同判）；工具本体遥测（如 phpMyAdmin 版本检查）经配置关闭 | idea.md §3 / kb / D0 安全底线 / ⚠️A12 |
| R-14 | 服务端校验全部输入：toolType ∈ 两值目录、任务动作 ∈ 枚举、令牌四重校验、轮换/打开前置——不信任前端 | idea.md §7 / kb |
| R-02 | 本模块全部接口位于 HttpOnly Cookie 会话之后；遵奉同源 + CSRF 全站协议；工具路径另经 gating 逐请求校验（FR-07） | idea.md §5.4 / kb |
| I-02 | 凭据永不回显：MySQL/Mongo 连接凭据（database-mgmt vault）、basic auth 口令、signon 密钥全部 AES-256-GCM 密文落库；任何接口响应/日志/审计 detail 不含明文；凭据注入仅走服务端 loopback/systemd env 通道 | D0 安全底线 / db_instance 凭据先例（vault.go:13-19） |
| ADR-018 | 只读边界：工具列表/任务状态输出两角色同等；一切变更 admin 专属；**工具访问整体 admin 专属**（⚠️A10，readonly 工具本体 gating 不可达——ADR-018 工具面具体化，同 Web 终端先例但保留查看面） | D0 / kb |
| ⚠️A1 | 两工具静态目录（编译期数据非用户可配：phpMyAdmin/mongo-express，版本钉死）；目录扩展走面板版本迭代 | D0 Q1 拍板 |
| ⚠️A2 | 依赖前置：phpMyAdmin=PHP-FPM+扩展+nginx；mongo-express=Node+nginx；缺失如实拒绝并引导到环境管理（不静默代装） | D0 复用积木取证 |
| ⚠️A3 | 异步任务 pending→running→succeeded/failed；同工具在途互斥；取消不做；任务台账持久化（落 SQLite，D2 落定） | D0 / 沿 env-mgmt ⚠️A3 同判 |
| ⚠️A4 | 升级=部署目录钉死新版本（面板版本迭代携带）；配置/凭据保留；上游检测/降级/自选版本 OUT | D0 / D1 |
| ⚠️A5 | 卸载=解除部署（程序/服务/站点/凭据密文），**绝不删除数据库数据**；强确认+知情提示 | D0 安全底线 / instanceRemove 哲学 |
| ⚠️A6 | localhost 绑定：mongo-express 127.0.0.1:8081（端口编译期定死）；phpMyAdmin 无独立监听端口（PHP-FPM pool 经 nginx FastCGI）；工具进程无独立公网端口/防火墙放行（**v1.2 修订：nginx 接入层自动块 listen 18090 为新增公网面，每请求 gating，非工具进程端口**） | D0 Q2 拍板 / 自动落地 D0 Q1 |
| ⚠️A7 | 会话 gating：auth_request 校验会话 cookie+admin 角色，200/401；鉴权端点仅 loopback 子请求通道（**v1.2 修订：既有块入口路径/域名绑定同链；自动块域名约束经 server_name+404 镜像；内部端点豁免 HostGuard/EntryPathGuard ⚠️A16**） | D0 Q2 拍板 / 自动落地 D0 Q1b |
| ⚠️A8 | signon 令牌：HMAC 防伪+短时效+一次性消费+绑定实例与会话；凭据仅 loopback 服务端注入永不进浏览器；过期/重放/跨实例/篡改拒绝 | D0 Q2 拍板 / 安全底线 |
| ⚠️A9 | basic auth 面板生成（随机强口令）+nginx 服务端注入；Mongo 凭据 vault→systemd env；密文零回显；轮换即时失效 | D0 Q2 拍板 |
| ⚠️A10 | 工具访问整体 admin 专属；readonly 目录/任务可看、工具本体不可达；细粒度映射 OUT | D0 范围边界 |
| ⚠️A11 | 安装/升级/卸载/打开/轮换审计（R-03 同事务脱敏）；gating 逐请求不落审计（防写放大）；拒绝事件通道归 D3 | D0 / ADR-020 |
| ⚠️A12 | 下载钉版本+sha256 强制校验（失败=任务 failed 不降级不跳过）+DOUGONG_MIRROR 同构镜像；npm registry 镜像同构 | D0 / install.sh:122-129/:38-45 先例 |
| ⚠️A13 | 轻量入口（面板内浏览）保留并存；治理台账仍归 database-mgmt | D0 Q3 拍板 |
| ⚠️A14 | 自动落地：0 匹配自动建 dougong-tools.conf listen 18090（ToolLandingPort 编译期常量）；占用→DBT_012 reason；既有块优先/多匹配引导不变；幂等复用两工具共用；siteRef+Port+AutoCreated；两工具卸载清块（AutoCreated 判定非自动块不动）；ConfigApply/ConfigRemove R-06；端口放行仅提示；cookie host-only 跨端口可行性 | 自动落地 D0 Q1 拍板（v1.2） |
| ⚠️A15 | 域名联动：未绑 server_name _ /已绑 BindDomain+非匹配 Host 404（镜像 HostGuard，对齐宝塔）；同步=panel-core 配置变更钩子主通道+install/open 惰性校正兜底（D1 裁定）；toolBaseHost Port≠0→scheme://hostname:Port（BindDomain 优先否则 requestHost 去端口） | 自动落地 D0 Q1 二次拍板（v1.2） |
| ⚠️A16 | 内部端点豁免：HostGuard/EntryPathGuard 对 /dbtool/gate/、/dbtool/signon/ 两路径前缀最小化直通；非内部路径强校验不变（IP 直连面板仍 404，fail-closed 不变）；安全由 loopback+秘密头+会话/令牌承载；落点归 panel-core（跨模块待同步项） | 自动落地 D0 Q1b 必修 bug（v1.2） |
| ⚠️A17 | reason 可见：worker finish 错误文本含 errcode data.reason 拼接，任务详情可见具体原因；无 reason 沿 symbol+message 不虚构 | 自动落地 D0 Q2 拍板（v1.2） |
| 本域推导 | 凭据安全：basic auth/signon 密钥 crypto/rand 随机强口令（不弱默认）；密文落库沿 CredentialVault 先例（独立密钥文件 0600，文件名 D3 定）；明文仅内存态注入使用，不落库不入审计不入输出 | ⚠️A8/A9 + I-02 + db_instance/dns_credential 先例 |
| 本域推导 | 幂等语义：安装幂等（已装再安装=升级语义分流或如实拒绝，D3 定）；升级幂等（同版本重复升级终态一致）；卸载幂等（未装时 NOT_INSTALLED 如实拒绝非静默成功）；令牌一次性（消费即失效，重放拒绝）；轮换即时生效（旧口令失效）；**v1.2：自动块幂等（已存在则复用不重复建块，标记/文件存在判定）；域名联动刷新幂等（server_name 已一致则零写入；钩子与惰性重复触发终态一致）；清块幂等（块已不存在视为成功不报错）** | ⚠️A4/⚠️A8/⚠️A14/A15 + env-mgmt 幂等先例 |
| 本域推导（v1.2） | 钩子失败语义：panel-core 配置变更钩子刷新自动块失败**不阻断配置变更**（panel_config 为权威，配置变更已成功；块刷新失败记日志，惰性校正兜底自愈）；惰性校正失败不阻断打开动作本身（块 server_name 不一致时打开 URL 仍按当前 BindDomain 生成，如实呈现失败原因——D3 定降级口径）；自动块写入失败沿 R-06 字节复原不遗留半配置 | ⚠️A15 + D1 裁定（AA 附录B）+ R-06 |
| 本域推导 | 越界拒绝不记审计：R-03 语义为「变更」留痕（与 site-mgmt/service-mgmt/env-mgmt 同口径）；gating 拒绝走安全日志通道非业务审计（⚠️A11） | R-03 字面口径 |
| 本域推导 | 数据保全边界：工具卸载/升级/凭据轮换均不触及引擎数据目录与实例档案（database-mgmt db_instance 聚合不受本模块写动作影响）；工具仅为访问通道 | ⚠️A5 + Q3 治理台账归属 |

**安全架构五道防线 → FR 映射（承接 BA 2.4/AA 附录C，v1.2 四→五，不可裁剪）**：

| 防线 | 约束 | 承载 FR | 验收口径（供 Gate 1/Gate 2） |
|------|------|--------|---------------------------|
| 一：网络面收敛 | 工具进程 localhost 绑定，工具进程无独立公网端口，唯一路径=面板 nginx 工具路径反代（既有块或自动块）；**v1.2：新增公网面=仅自动块 nginx 接入层 listen 0.0.0.0:18090（每请求受防线二/五约束）；已绑域名时自动块 server_name=BindDomain+非匹配 Host 404 镜像 HostGuard；端口放行仅提示不自动配置防火墙** | FR-03（部署绑定）/FR-07（反代）/FR-11（自动块 listen/server_name）/FR-12（域名联动） | 公网端口扫描无工具进程端口（18090 为 nginx 接入层受 gating）；localhost 绑定实证；绑域名后非匹配 Host 访问 18090→404 |
| 二：会话 gating | auth_request 逐请求校验会话+admin（**含自动块 18090 每请求**）；端点仅 loopback 子请求通道；纯读防 CSRF/越权；吊销级联即时切断；**v1.2 内部端点豁免：HostGuard/EntryPathGuard 对 /dbtool/gate/、/dbtool/signon/ 最小化直通——豁免 Host/入口路径校验而非鉴权，loopback+秘密头+会话/令牌防护链不变，非内部路径强校验不变** | FR-07/FR-13 | 无会话/非 admin → 401/403 实测（含 18090）；会话吊销后工具即时不可达；**HostGuard 豁免回归：绑域名后内部 gate/signon 端点（loopback+秘密头）仍可达不被 404；非内部路径 IP 直连面板仍 404；豁免范围最小化** |
| 三：凭据零回显与桥接防伪 | 三类凭据密文零回显；令牌 HMAC+短时效+一次性+绑定实例；凭据仅服务端通道注入 | FR-03（生成）/FR-08/FR-09 | 令牌过期/重放/跨实例/篡改四路拒绝回归；响应/日志/审计 detail 凭据扫描零命中 |
| 四：变更安全与数据保全 | sha256 强制；R-08 参数数组；R-03 同事务脱敏；卸载不删数据；R-09；**v1.2：任务失败原因含 data.reason 如实可见（不吞原因不虚构成功）** | FR-03/04/05/06/FR-14 | sha256 失败=failed 回归；卸载后数据库数据完整性实证；**worker reason 可见回归（task.Error 含 reason 文本）** |
| 五：自动落地块安全（v1.2 新增） | 自动块公网可达但每请求 gating（无会话/readonly→401）；域名联动强制（钩子+惰性双通道不留长期不一致窗口）；写入/刷新/清块经 ConfigApply/ConfigRemove（nginx -t+reload+失败字节复原 R-06 不产生半配置）；端口占用如实 DBT_012 reason 不静默换端口；多匹配引导不擅自选择；幂等复用不重复建块；两工具均卸载清块（AutoCreated 判定非自动块不动）；signon/vault/gate 防护在自动块形态全量适用；端口放行仅提示 | FR-11/FR-12/FR-07 | **自动落地专项**：0 匹配自动建块回归（fake nginx 替身：listen 18090/server_name 跟随绑定/gate+tool location）；端口占用→DBT_012 reason；既有块优先/多匹配引导/幂等复用/卸载清块；域名联动回归（未绑 _ /绑定后 server_name=域名+非匹配 Host 404/绑定解绑同步刷新）；toolBaseHost Port 分支回归（IP:18090/域名:18090 URL） |

## 第六部分：状态流转（T2）

本域有 **1 个状态机**：工具部署任务 db_tool_task_status（⚠️A3，持久化）。

| 当前状态 | 目标状态 | 触发 | 备注 |
|---------|---------|------|------|
| pending | running | worker 取出任务开始执行 | 创建即 pending（发起接口同步返回 taskId） |
| running | succeeded | 部署命令链执行成功（下载校验/解压/npm/站点写入/服务创建全链成功） | 终态；输出尾部定格；部署档案登记/更新同事务 |
| running | failed | 任一步骤失败（sha256 校验失败/依赖缺失/下载失败/npm 失败/站点写入失败/服务创建失败/超时；**v1.2 +自动建块失败：ToolLandingPort 占用/自动块写入 nginx -t 不通过等，DBT_012 data.reason 如实**） | 终态；error+输出尾部定格；失败步骤如实分类（**error 文本含 data.reason，FR-14**） |
| running | failed | 面板重启恢复（启动扫描 running 任务） | error=「面板重启中断」（如实呈现，不虚构成功） |

**辨析表**（易混淆项澄清，防伪状态机）：

| 易混淆项 | 辨析 | 结论 |
|---------|------|------|
| 工具部署状态（未装/已装） | db_tool 档案存在性+实况探测结论（部署档案为面板自持，版本/运行态为实况投影），非面板侧状态迁移存储 | 档案存在性+实况投影，非状态机 |
| 工具运行态 | mongo-express systemd 实况/phpMyAdmin 站点可用性投影，随系统实况实时变化 | 实况投影，非状态机 |
| 依赖就绪度 | env-mgmt 环境实况消费结论（PHP-FPM/Node/nginx/扩展），整值刷新 | 域级判定值，非状态机 |
| signon 令牌生命周期（签发→消费/过期） | 短时效一次性安全凭据的内存态消费台账（TTL 清扫），无跨重启留存语义，面板重启全部失效=安全保守 | 过程数据（P7 敏感），非状态机 |
| basic auth 口令轮换 | 凭据值对象替换式更新（旧口令即时失效），无中间态 | 值对象替换，非状态机 |
| 任务取消 | OUT-6 不做（下载/npm 中断态不安全）；「不提供」即无取消状态 | 显式 OUT |
| 自动落地块生命周期（建块/复用/刷新/清块，v1.2） | 块存在性=nginx 配置文件事实（dougong-tools.conf 存在与否），server_name=配置值随绑定状态刷新——面板不存储块状态，无面板侧状态迁移；建块/清块为安装/卸载任务链的机制步骤（结果随任务终态），刷新为钩子/惰性同步机制 | 配置文件事实+机制链路，非状态机（与 site 零站点台账「配置文件是唯一事实源」同哲学） |
| 域名联动同步（钩子/惰性，v1.2） | bindDomain 变更事件触发的配置同步动作（成功=块 server_name 一致，失败=记日志+惰性兜底），无中间态无独立生命周期 | 事件触发同步机制，非状态机（panel_config.bind_domain 本身亦无状态机，变更即生效） |
| AutoCreated 标志（v1.2） | siteRef 值对象的布尔属性（标记落地块是否面板自动创建，清块判定用），非对象状态字段 | 值对象属性，非状态机（与 totp_enabled 布尔语义同口径不入池） |

无审批类状态流转（T3 N/A 依据）：工具部署与访问为管理员即时发起的异步执行/实时校验，无提交/授权环节；唯一授权边界=角色判定（ADR-018/⚠️A10），属访问控制非审批流。

## 第七部分：查询与导出需求（T4/T5）

**查询（T4）**：

| 查询形态 | 筛选/排序维度 | 说明 |
|---------|-------------|------|
| 工具全景查询 | 无筛选（两条目全量，依赖就绪度随响应） | 目录+实况合并呈现；两角色同等 |
| 任务列表查询 | toolType? / status? 筛选，发起时间倒序 | 任务台账（低频操作量级小）；两角色同等 |
| 单任务详情 | taskId 定位 | 输出尾部+失败原因；两角色同等 |
| 工具状态查询（跨模块读契约） | 按目标引擎（mysql/mariadb→phpMyAdmin；mongodb→mongo-express） | database-mgmt 高级管理入口消费：部署状态+打开能力可用性（进程内读契约，D3 定签名） |

**导出（T5）**：无集合导出需求（工具两条目/任务低频台账；审计导出归 account-auth）。

**被引用关系（T1a）**：

| 被引用能力 | 消费方 | 方式 |
|-----------|--------|------|
| 工具状态与打开能力 | database-mgmt 实例治理页/库行「高级管理」入口 | 进程内读契约（工具状态按引擎）+ 打开能力共用（FR-10）；入口渲染归 database-mgmt PageSpec（跨模块待同步项，无持久 ID 引用） |
| 依赖缺失引导（软引导） | 本模块 → env-mgmt | 安装拒绝时呈现「到环境管理安装 PHP-FPM/Node/nginx」指引链接（无跨模块 API 硬依赖） |
| 消费方视角 | account-auth | 消费其审计写入（R-03）、会话鉴权与只读边界（R-02/ADR-018）、**gating 会话校验消费——db-tools 自持 GET /dbtool/gate/verify 内部端点，复用 account-auth 既有只读会话校验 OHS（VerifyWs），account-auth 零新增 HTTP（api D-11 定稿，v1.1 W2 回填）** |
| 消费方视角 | env-mgmt | 消费其环境实况（PHP-FPM/Node/nginx 已装态，进程内读契约）；Node 环境为其本轮增量供给（prd-env-mgmt.md FR-08） |
| 消费方视角 | site-mgmt | 消费其站点创建/移除通道（S-27-02/S-27-03/S-28）与安全写入治理（S-30，R-06）落工具路径/gating/注入配置 |
| 消费方视角 | service-mgmt | 共享 SysdGateway systemd 适配层资产（mongo-express 单元创建/启停/查询，进程内共享非模块调用） |
| 消费方视角 | database-mgmt | 消费其实例凭据 OHS（vault 取出 MySQL/Mongo 连接凭据，进程内单向，凭据不出服务端边界）——与 env-mgmt 消费 BootstrapAfterInstall 同构 |
| 消费方视角（v1.2） | panel-core | ①消费配置变更通知钩子（bindDomain 绑定/解绑 → 自动块 server_name 刷新，进程内单向，D1 裁定主通道；钩子契约签名归 D3，panel-core 侧提供为跨模块待同步项）；②依赖 HostGuard/EntryPathGuard 内部端点豁免（FR-13，panel-core S-07 机制侧变更，跨模块待同步项——绑域名后 gating/signon 内部端点方可达）；③消费 panel_config.bind_domain 当前值（PanelAccess 读契约既有，toolBaseHost/惰性校正/server_name 初值数据源） |

## 第八部分：非功能性要求与异常场景

**N1 非功能性（已锁定项）**：两工具目录为编译期数据（版本钉死）；实况探测每请求实时（无缓存层）；任务台账量级小（低频部署操作积累）；部署操作超时上限与任务输出尾部行数上限为编译期常量（D3 定值）；任务台账全量保留（低频事件，不建清理任务——避免常驻 goroutine，沿 env-mgmt 先例）；signon 令牌短时效（60s 量级）+内存态消费台账（TTL 清扫，面板重启全部失效）；下载/npm/站点写入/服务操作全注入化（HTTP 下载/文件 IO/npm runner/sysd/nginx 写回均经网关接口注入替身，零 runner 实况依赖——CI 环境耦合预防沿 env-mgmt 第 7 起先例；引擎/工具实联归 Linux 目标环境冒烟）；gating 逐请求校验为纯读无写放大（⚠️A11）。**v1.2 锁定项**：ToolLandingPort=18090 编译期常量（不可用户配，OUT-15）；自动块文件路径 {nginx conf.d}/dougong-tools.conf 编译期常量（沿 site-mgmt 扫描目录）；自动块 http（HTTPS 工具落地 OUT-16）；域名联动双通道（钩子主+惰性兜底，D1 裁定）；siteRef +Port+AutoCreated 为 JSON 值对象扩展（db_tool.site_ref TEXT 列不变，**零 DDL 变更零迁移**——取证 inject.go:20-25/api-db-tools.md:237/:554）；钩子刷新失败不阻断配置变更（记日志+自愈）。未锁定项（移交 D3）：sha256 值/版本号/端口/目录布局/扩展清单/令牌时效具体值/超时常量；db_tool/tool_deploy_task DDL 与密钥文件名；gating 端点契约；工具路径形态（**v1.2 部分裁定：既有块路径前缀/0 匹配独立自动块，D1 待对齐-3 关闭；块模板细节/占用探测方式/钩子签名/惰性触发点/reason 文案细分归 D3，AA 待对齐-5**）。

**N2 异常场景**：

| 场景 | 处理 |
|------|------|
| 依赖缺失（PHP-FPM/Node/nginx 未装或 PHP 扩展不齐） | 安装如实拒绝（DEPENDENCY_MISSING 语义）+ 缺失清单 + 到环境管理安装引导；不静默代装（OUT-8） |
| sha256 校验失败 | 任务终态 failed（error 明示校验失败）；不降级不跳过不安装（⚠️A12）；已下载文件清理 |
| 下载网络失败（tarball/npm registry 不可达） | 任务终态 failed + 输出尾部 + 失败原因如实；镜像选项可重试（操作者决策后重新发起）；不自动重试 |
| npm 安装失败/解压失败/目录写入失败 | 任务终态 failed + 失败步骤如实分类；已落盘部分清理或如实呈现（D3 定清理口径） |
| 站点配置写入失败（nginx -t 不通过，含自动块创建/刷新，v1.2） | 经 site-mgmt 安全写入管线回滚（R-06 四步，失败字节复原）；任务终态 failed（error 含 data.reason，FR-14）；不遗留半配置 |
| 同工具重复发起变更 | 在途互斥拒绝（pending/running 存在时，TASK_BUSY 语义，data.taskId 指向在途任务） |
| 面板重启时任务在途 | 启动恢复：running 任务标 failed（error=面板重启中断）；不虚构 succeeded |
| 无会话访问工具路径 | gating 401 拒绝（⚠️A7）；工具本体不可达 |
| readonly 访问工具路径/发起打开 | gating 角色校验拒绝（401/403 语义 D3 定，⚠️A10）；界面无打开入口（负空间） |
| 面板会话吊销/改密级联后访问工具 | gating 逐请求实时校验即时拒绝（无独立长时会话，FR-07） |
| signon 令牌过期 | 校验拒绝（TOKEN_EXPIRED 语义）；重新打开签发新令牌 |
| signon 令牌重放（已消费） | 一次性消费台账拒绝（TOKEN_REPLAYED 语义）；消费即失效 |
| signon 令牌跨实例重用/篡改 | 实例绑定校验/HMAC 签名校验拒绝（TOKEN_INVALID 语义） |
| 目标实例未纳管/凭据未配置 | 打开/安装如实拒绝并引导到 database-mgmt 配置凭据（不虚构可登录，FR-08⑤/FR-09⑤） |
| vault 取出凭据失败（密钥缺失/解密失败） | 打开/注入失败如实呈现（INTERNAL 语义）；不降级为明文/空凭据 |
| 未装工具发起卸载/升级/打开 | 如实拒绝（NOT_INSTALLED 语义），非静默成功；打开呈现安装引导 |
| 工具运行态探测失败（服务单元查询异常/站点不可达） | 运行态如实呈现未知/不可达（不虚构运行中）；打开前置校验如实拒绝 |
| basic auth 轮换后旧口令访问 | 旧口令即时失效（nginx 注入配置已更新+服务重启）；浏览器无感知（注入服务端） |
| readonly 越界发起任何变更 | 全局边界管控拒绝并提示边界原因（ADR-018）；界面本就无入口 |
| 空任务列表 | 返回空列表 + 引导，非错误 |
| **ToolLandingPort 被占用（v1.2）** | 自动建块前探测占用→任务终态 failed，DBT_012 data.reason「工具端口被占用」如实引导释放（不静默换端口，OUT-15）；占用探测方式 D3 定（AA 待对齐-5） |
| **落点多候选（v1.2 沿既有口径）** | 多匹配仍 DBT_012 列候选引导到站点管理明确后重试（不擅自选择，不变）；error 含候选清单 reason（FR-14） |
| **域名变更钩子刷新自动块失败（v1.2）** | 不阻断配置变更（panel_config 权威已成功）；记日志；工具 install/open 惰性校正兜底自愈（FR-12③）；不留长期不一致窗口 |
| **惰性校正失败（v1.2）** | 不阻断打开动作本身；打开 URL 仍按当前 BindDomain 生成；失败原因如实呈现（降级口径 D3 定，AA 待对齐-5） |
| **绑域名后非匹配 Host 访问工具端口（v1.2）** | 自动块 server_name=BindDomain+非匹配 Host 返回 404（镜像 HostGuard，FR-12②）——绑域名后工具也只认域名，IP 直连 18090 不可达 |
| **内部端点豁免边界（v1.2）** | 豁免仅限 /dbtool/gate/、/dbtool/signon/ 两路径前缀；非内部路径 HostGuard/EntryPathGuard 强校验不变（绑域名后 IP 直连面板仍 404，fail-closed 不变）；内部端点无 loopback/秘密头仍裸 404（防护链不变，FR-13） |
| **清块判定边界（v1.2）** | 仅 AutoCreated=true 的自动块在两工具均卸载后移除；既有面板域落地块/用户站点块不动（不误删）；块已不存在视为清块成功（幂等） |

**N3 业务规则细节**：见第五部分规则汇总；补充——任务创建与首次状态落库同事务（创建即持久，防发起即失忆）；互斥判定与任务创建同事务原子（防并发双开）；**部署档案登记/更新与任务终态同事务**（安装成功=档案+凭据密文落库原子；卸载=档案+密文删除原子）；**凭据轮换与审计同事务**（R-03）；signon 令牌消费与凭据注入为服务端同步链路（消费成功即注入，失败即拒绝不留半态）；打开动作审计与令牌签发同事务。**v1.2 补充**：自动块创建/刷新/清块经 ConfigApply/ConfigRemove 管线原子（nginx -t 失败字节复原，不产生半配置）；钩子刷新与惰性校正为幂等同步（server_name 已一致零写入）；钩子失败不阻断配置变更（panel_config 权威）；清块判定以 AutoCreated 标志+两工具档案均不存在为条件（非自动块不动）；siteRef JSON 载荷扩 Port/AutoCreated 字段向后兼容（旧档案 Port=0/AutoCreated=false 缺省语义，沿旧形态）。

**N4 历史需求**：任务台账即工具部署历史（动作/发起者/终态/输出尾部）；操作追溯经 account-auth 审计日志（object_type=db_tool/tool_deploy_task）；无独立工具变更历史表（台账即历史，沿 env-mgmt 先例）。

## 第九部分：假设与待对齐项

**跨业务域集成**（Step 9）：

| 模式 | 集成点 | 说明 |
|------|--------|------|
| 遵奉者 | 会话鉴权与只读边界协议 | 全部接口位于登录会话后（R-02）；查看两角色同等、变更与工具访问 admin 专属（ADR-018/⚠️A10） |
| OHS（消费） | 审计写入接口（account-auth S-04） | 全部变更类操作 + 任务终态 + 打开/轮换写审计（R-03 同事务脱敏） |
| OHS（消费） | account-auth 只读会话校验（VerifyWs，gating 消费） | 工具路径 auth_request 子请求校验会话 cookie+admin 角色（200/401，仅 loopback 内部通道）——**db-tools 自持 GET /dbtool/gate/verify 内部端点，复用 account-auth 既有只读会话校验 OHS（VerifyWs 语义 session.go:59，只读不续期），account-auth 零新增 HTTP 接口**（api D-11 定稿，OHS 消费已登记 api-account-auth.md v1.2；v1.1 W2 回填） |
| OHS（消费） | env-mgmt 环境实况读契约 | 依赖就绪度数据源（PHP-FPM/Node/nginx 已装态与版本，进程内读，单向）；Node 环境为 env-mgmt 本轮增量（prd-env-mgmt.md FR-08） |
| OHS（消费） | site-mgmt 站点创建/移除 + 安全写入治理 | php_fpm/reverse_proxy 站点落工具路径；gating/basic auth 注入配置经 R-06 四步管线；卸载经站点移除通道 |
| 共享适配层（非业务域集成） | service-mgmt SysdGateway | mongo-express 服务单元创建/启停/重启/移除与运行态查询复用同一 systemd 网关实现（进程内共享） |
| OHS（消费，新增） | database-mgmt 实例凭据 OHS | signon 桥接与 mongo-express 注入消费 vault 凭据取出能力（进程内单向，凭据仅服务端 loopback/systemd env 通道流转）——与 env-mgmt 消费 BootstrapAfterInstall 同构；契约签名归 D3（api-database-mgmt.md §5.1 登记） |
| OHS（消费，v1.2 新增） | panel-core 配置变更通知钩子 | bindDomain 绑定/解绑成功（config/update，D-P12）→ 进程内通知 db-tools → 自动块存在则重渲染 server_name 经 ConfigApply 刷新（域名联动主通道，D1 裁定）；钩子失败不阻断配置变更（记日志+惰性兜底）；钩子契约签名归 D3；**panel-core 侧钩子提供为跨模块待同步项（panel-core 三文档同步登记，本轮不代改）**——与 env-mgmt 消费 BootstrapAfterInstall OHS 同构（进程内观察者，main 装配注册） |
| 机制侧变更依赖（v1.2 新增，panel-core） | HostGuard/EntryPathGuard 内部端点豁免 | 全局包装器（cmd/dougong/main.go:487-488 Entry→Host→gin 链）对 /dbtool/gate/、/dbtool/signon/ 两路径前缀直通放行（FR-13，⚠️A16）——修复绑域名后 gating/signon 失效；豁免范围最小化，非内部路径强校验不变；内部端点 loopback+秘密头防护链不变；**panel-core S-07 机制侧变更，跨模块待同步项，本轮不代改** |
| OHS（消费，v1.2 扩展） | site-mgmt ConfigApply/ConfigRemove（自动落地块） | 自动块创建（allowNew）/域名联动刷新/清块经既有安全写入管线（configpipeline.go:18/:29，nginx -t+reload+失败字节复原 R-06）——既有 OHS 复用零新增契约，块配置语义归本模块 D3 |
| ACL（防腐层） | phpMyAdmin/mongo-express（外部现成工具） | 工具协议（signon 回调/basic auth/env 注入/baseUrl）经适配器封装——工具内部模型不渗透面板领域模型；工具本体为用户主动部署的现成软件（R-09 例外语义与 ACME 同判：下载用户主动发起，工具本体遥测配置关闭）；工具访问仅 localhost 经 gating（⚠️A6/A7） |
| 本机资源（非业务域） | nginx/PHP-FPM/systemd/npm | 经既有适配器通道（nginxx/phpfpm/sysd）与 npm runner（R-08 参数数组）执行；本机系统组件访问非外部业务系统对接 |

**过程数据分类**（前置过滤，均不入领域对象清单）：

| 数据项 | 类别 | 处理方式 |
|--------|------|---------|
| 工具列表/任务列表筛选参数（toolType/status） | P1 查询条件 | 接口查询参数，服务端校验（R-14） |
| 工具运行态、依赖就绪度、已装版本实况字段 | P2 展示数据 | 实时探测投影（响应即消亡，与 systemd_service 投影同构）；部署档案持久字段（db_tool）除外 |
| 任务在途标记（列表条目上） | P2 展示数据 | 由在途任务台账派生 |
| 安装/升级/卸载在途状态（按钮 loading） | P3 UI 交互状态 / P7 计算中间值 | 任务创建同步返回 + 任务详情轮询呈现 |
| 命令输出尾部 ring 缓冲（执行中） | P7 计算中间值 | 执行链内存态，终态定格落库（output_tail 字段） |
| signon 令牌与消费台账 | P7 计算中间值 / 敏感 | 短时效一次性安全凭据：内存态签发/消费/TTL 清扫，面板重启全部失效（安全保守）；令牌明文不入审计不入日志；凭据注入仅 loopback 服务端通道（⚠️A8） |
| basic auth 口令明文 / Mongo 连接凭据明文 / signon 密钥明文 | P7 计算中间值 / 敏感 | 仅生成与注入内存态使用，加密后密文落 db_tool（basic auth/signon 密钥）或取自 db-mgmt vault（连接凭据）；明文不落库不入审计不入输出（I-02） |
| 工具目录条目（两工具静态清单+版本/sha256/分发形态） | 编译期常量（技术载体） | 非用户可配数据，不入领域对象清单（与枚举池同性质，db_tool_type 枚举入池承载标识语义） |
| 打开 redirect URL / 高级入口按钮态 | P2 展示数据 / P4 请求响应结构 | 打开能力响应载体；入口渲染归 database-mgmt PageSpec；**v1.2：URL 按 siteRef.Port 分支生成（FR-12④），仍为响应载体不持久化** |
| **落点引用 siteRef（v1.2 扩展）** | 值对象（持久化于 db_tool.site_ref JSON 列） | File/Index/ServerName/Scheme 既有 +Port/AutoCreated 扩展——安装落档时定格（FR-03），撤除/打开/清块寻址用；**非过程数据**（持久化值对象，domain-model v1.26 显式建模）；旧档案缺省 Port=0/AutoCreated=false 向后兼容 |
| **配置变更钩子事件载荷（v1.2）** | P5 集成载荷 | bindDomain 变更通知（进程内观察者事件，非持久化消息）；载荷=新绑定状态（域名/空），db-tools 据此重渲染自动块 server_name；契约签名归 D3 |
| **端口占用探测结果 / 块存在性判定（v1.2）** | P7 计算中间值 | 自动建块前一次性探测（listen 探针/文件标记判定，方式 D3 定），判定即消亡不持久化；块存在性事实源=nginx 配置文件（零块台账） |

**假设与待对齐清单**：

| 编号 | 内容 | 状态 |
|------|------|------|
| 待对齐-1 | db_tool/tool_deploy_task 表结构 DDL（凭据密文列形态/密钥文件名/索引）；任务输出尾部行数上限与部署超时上限具体值；signon 令牌时效具体值与消费台账 TTL 清扫形态 | D3 拍板 |
| 待对齐-2 | gating 鉴权端点契约形态 | **D3 已定稿关闭（api D-11，v1.1 W2 回填）**：db-tools 自持 GET /dbtool/gate/verify（裸 200/401，loopback+X-Dbtool-Internal 秘密头，复用 account-auth 既有 VerifyWs 只读会话校验+admin 角色判定，account-auth 零新增 HTTP）；OHS 消费已登记 api-account-auth.md v1.2 |
| 待对齐-3 | 工具路径形态（面板域下 location 前缀 vs 独立 server 块）与入口路径/域名绑定约束叠加；site-mgmt 消费契约细化（php_fpm/reverse_proxy 建站参数与 gating/注入配置写回） | **D1 已裁定（v1.2，AA 待对齐-3 关闭）**：既有匹配块=面板域下路径前缀反代（不变）；0 匹配=独立自动 server 块（dougong-tools.conf listen 18090，FR-11）；约束叠加=既有块 HostGuard/EntryPathGuard 同链、自动块 server_name+非匹配 Host 404 镜像（FR-12）、内部 2 端点豁免两网关（FR-13）、入口路径约束不适用独立工具端口；剩余块模板/建站参数细节归 D3（待对齐-7） |
| 待对齐-4 | 编译期常量表：phpMyAdmin 版本+sha256、mongo-express 版本、Node LTS 版本号（env-mgmt D3）、PHP 扩展清单与检测方式、端口 8081/目录布局/baseUrl 前缀 | D3 定稿 |
| 待对齐-5 | mongo-express 注入凭据在 database-mgmt 实例凭据变更后的刷新时机（保守口径=升级/重装刷新，不自动传播；phpMyAdmin signon 每次打开实时取 vault 无此问题） | D3 裁定（D1 遗留待对齐-1） |
| 待对齐-6 | 工具列表/任务呈现节奏（轮询间隔/手动刷新组合）；打开工具新窗口形态 | D4 定 |
| 待对齐-7（v1.2 新增） | 自动落地块模板细节（gate/tool location 排布、非匹配 Host 404 镜像写法 if/default_server）；ToolLandingPort 占用探测方式（listen 探针 vs nginx -t 失败归类）；panel-core 配置变更钩子契约签名与注册点（main 装配，沿 BootstrapAfterInstall 先例）；惰性校正触发点细节与失败降级口径；DBT_012 reason 文案细分（端口占用/多候选/扫描失败/建块失败）；siteRef JSON 载荷 Port/AutoCreated 字段名与缺省语义 | D3 定稿（api v1.3 常量表/契约/错误码 reason；承接 AA 待对齐-5） |
| 说明 | ⚠️A1~A17 为 D0/D1 拍板口径（记录于 BA/AA 附录与 d0-clarification-db-tools-integration-2026-11-19.md / d0-clarification-dbtools-auto-landing-2026-11-19.md），本 PRD 沿用为 FR 依据；Gate 1 呈现全文供用户复核，改判需同步 BA/AA/FL/PRD 四文档 | 已登记（D0/D1） |
| 说明（v1.2） | FR-13 HostGuard/EntryPathGuard 内部端点豁免与配置变更钩子提供为 **panel-core 机制侧变更**（S-07/S-11-01），panel-core 三文档同步登记为跨模块待同步项——本轮 db-tools D2 不代改 prd-panel-core.md（沿 database-mgmt 高级入口先例）；本 PRD 仅登记需求与消费契约 | 模块边界声明 |
| 说明 | Node 环境供给 FR 归 prd-env-mgmt.md FR-08（env-mgmt 模块边界，D1 落定）；本 PRD 仅消费其环境实况与安装引导（IN-3/FR-03 依赖前置），不重复定义 Node 安装流程 | 模块边界声明 |

## 附录：PRD 完备性检查报告

**功能类型**：台账混合型 + 集成对接型——工具部署档案（db_tool 聚合根：档案+凭据密文）+ 部署任务单据（tool_deploy_task 四态状态机、异步执行、无审批）+ 现成工具集成（phpMyAdmin signon 协议/mongo-express basic auth+env 注入，ACL 封装）；无集合导出。

### 第一层：通用必检项 G1-G5

| # | 检查项 | 结果 | 依据 |
|---|--------|------|------|
| G1 | 业务目标 | ✅ | 第一部分：高级管理工具集成补位（宝塔式路线 ADR-032）、安全底线、轻量入口并存、模块分界 |
| G2 | 功能范围 | ✅ | 第二部分：IN 14 项 / OUT 17 项（v1.2 +自动落地/域名联动/HostGuard 豁免/reason 可见 4 IN +自动防火墙放行/端口可配/HTTPS 工具落地/面板 nginx 化 4 OUT） |
| G3 | 角色定义 | ✅ | 第三部分：2 角色 + 访客不涉及声明 + 工具访问 admin 专属具体化（v1.2 零变化——自动落地为机制型无新角色面） |
| G4 | 领域对象 | ✅ | domain-model.md v1.26：db_tool 聚合根（部署档案+凭据密文，A1/A2/A3/A4 通过）+ tool_deploy_task 聚合根（四态机，与 env_install_task 同构）+ 工具凭据值对象 + 工具条目投影实体 + 目录映射值对象 + **落地引用（siteRef）值对象（v1.26 显式建模，+Port/AutoCreated，JSON 载荷扩展零 DDL）——零新聚合根** |
| G5 | 对象关系 | ✅ | domain-model.md「与上级关系」列；无子流程（任务状态机为聚合根本体，signon 桥接/自动落地/域名联动为同步机制链路，v1.2 辨析 +3 行）；关联领域对象 db_instance 引用（凭据 OHS 消费，ID 引用不复制模型）；panel_config.bind_domain 为实况值消费非 ID 引用（v1.26） |

### 第二层：按类型必检项 T

| # | 检查项 | 适用性 | 结果 | 依据 |
|---|--------|--------|------|------|
| T1 | 业务字段 | 适用 | ✅ | domain-model.md 领域对象清单（db_tool 档案字段/任务全字段/凭据值对象/条目投影字段） |
| T1a | 被引用关系 | 适用 | ✅ | 第七部分：database-mgmt 高级入口消费 + 软引导 + 消费方视角七条（v1.2 +panel-core：配置变更钩子消费/HostGuard 豁免依赖/bind_domain 实况值消费） |
| T2 | 状态流转 | 适用 | ✅ | 第六部分：db_tool_task_status 四态机（**仍 1 个状态机**）+ 9 项辨析（v1.2 +自动落地块生命周期/域名联动同步/AutoCreated 标志三项非状态机） |
| T3 | 审批规则 | 无审批语义 | ➖ N/A | 管理员即时发起异步执行/实时校验，无授权环节 |
| T4 | 查询筛选条件 | 适用 | ✅ | 第七部分：工具全景/任务筛选/单任务/跨模块状态查询四形态 |
| T5 | 导出需求 | 无导出 | ➖ N/A | 第七部分 |
| T6 | 对接系统 | 适用（集成对接） | ✅ | 第九部分：phpMyAdmin/mongo-express 外部工具标识与接口方式（signon 回调/basic auth/env 注入，ACL 封装）+ 六模块 OHS 消费（v1.2 +panel-core 配置变更钩子/HostGuard 豁免机制侧依赖/site-mgmt ConfigApply 自动块扩展） |
| T7 | 数据映射 | 适用（集成对接） | ✅ | FR-03/FR-08/FR-09：vault 凭据→signon 会话注入/systemd env 注入/basic auth Authorization 头映射；工具版本→目录钉死值映射 |
| T8 | 幂等策略 | 适用（集成对接） | ✅ | 第五部分本域推导：安装/升级/卸载幂等 + 令牌一次性防重放 + 轮换即时失效 + 同工具互斥 + **v1.2 自动块幂等复用/域名联动刷新幂等（一致零写入）/清块幂等（已不存在视为成功）/钩子失败不阻断配置变更** |
| T9 | 输入输出规格 | 适用 | ✅ | FR-01~14 的 I/O 描述（任务输出尾部/实况投影/终态反馈〔含 data.reason〕/打开 redirect〔Port 分支〕/令牌校验结果/自动块配置产物/钩子事件载荷） |

### 第三层：加分项 N1-N4

| # | 检查项 | 结果 | 说明 |
|---|--------|------|------|
| N1 | 非功能性要求 | ✅（部分锁定） | 两条目/实时探测/量级/台账全量保留/令牌内存态/全注入化已锁定；**v1.2 +ToolLandingPort=18090/块文件路径/http 形态/双通道同步/siteRef 零 DDL 扩展/钩子失败语义锁定**；常量值与 DDL/端点契约 D3 定（待对齐-1~-4/-7） |
| N2 | 异常场景 | ✅ | 27 个异常分支（v1.2 20→27：+端口占用/多候选/钩子刷新失败/惰性校正失败/绑域名后非匹配 Host/内部端点豁免边界/清块判定边界；站点写入失败行扩自动块） |
| N3 | 业务规则细节 | ✅ | 第五部分规则汇总（⚠️A1~A17 + I-02 + 凭据安全/幂等/数据保全/钩子失败语义推导）+ 事务补充（档案与终态同事务/轮换与审计同事务/自动块管线原子/siteRef 向后兼容）+ 安全五防线映射表（v1.2 四→五） |
| N4 | 历史需求 | ✅ | 任务台账即历史 + 审计追溯 |

### 检查结论（v1.2 增量复检）

```
## PRD 完备性检查 — 通过

功能类型：台账混合型 + 集成对接型（工具部署档案与凭据聚合 + 部署任务单据 + 现成工具 ACL 集成）
检查项：G 5/5 通过；T 适用项 8/8 通过（T1/T1a/T2/T4/T6/T7/T8/T9，T3/T5 N/A 有据）；N 4/4

可进入 API 设计阶段（D3，本技能 d3-api.md）
```

### 本次变更影响范围（v1.2 增量，供 D3 变更摘要）

| 影响面 | 结论 |
|--------|------|
| 领域模型 | **零新聚合根/零新状态机/零 DDL 变更**——db_tool/tool_deploy_task 双聚合根与字段不变；落地引用（siteRef）值对象显式建模 +Port/AutoCreated（db_tool.site_ref JSON 载荷扩字段，TEXT 列不变，旧档案缺省向后兼容）；domain-model.md v1.26 |
| 已有接口 | 预期无新增 HTTP 端点（自动落地/域名联动为 worker/钩子机制，reason 可见为 task/detail 响应 error 文本内容扩展）——**最终接口面以 D3 api v1.3 定稿为准**；task/detail 的 error 字段语义扩展（含 data.reason）；access/open·reopen 响应 URL 含端口分支 |
| 状态流转 | 无新增状态/无流转路径修改（四态机不变；自动块生命周期/域名联动/AutoCreated 三项辨析为非状态机） |
| 枚举 | 零新增零扩值（AutoCreated 布尔/Port 数值非枚举；db_tool_type/db_tool_task_action/db_tool_task_status 三枚举不变） |
| 错误码 | DBT_012 语义扩展（+端口占用/自动建块失败 reason 细分，文案 D3 定）；预期零新增符号码——D3 定稿 |
| 跨模块 | panel-core 两条待同步项（配置变更钩子提供 S-11-01/HostGuard·EntryPathGuard 内部端点豁免 S-07）；site-mgmt ConfigApply/ConfigRemove 既有 OHS 复用零新增契约 |

## 更新记录

| 版本 | 日期 | 更新说明 |
|------|------|---------|
| v1.0 | 2026-11-19 | 初始生成：基于 D0 澄清拍板（Q1~Q3 + 安全底线）与 D1 三文档 v1.0（⚠️A1~A13）成文；FR-01~10 覆盖 AA 全部 10 子模块（3 机制型 FR-02/07/08）；完备性检查通过（G 5/5、T 适用 8/8〔含集成对接 T6/T7/T8〕、N 4/4）；领域建模落盘 domain-model.md v1.24（支撑域判定 + 2 聚合根 db_tool〔部署档案+凭据密文，A1-A4 全过落 SQLite〕+ tool_deploy_task〔四态机落 SQLite，与 env_install_task 同构〕+ 工具凭据值对象 + 工具条目投影实体〔零台账〕+ 目录映射值对象〔编译期〕；signon 令牌为 P7 敏感过程数据内存态）；枚举 3 项新增定稿入池（db_tool_type/db_tool_task_action/db_tool_task_status）+ db_tool_state 不采用（部署状态=档案存在性+实况投影非状态机，理由见池行）；安全四防线→FR 映射表显式成节；待对齐 6 项移交 D3/D4（含 D1 遗留 4 项承接） |
| v1.1 | 2026-11-19 | Gate 1 WARN 修复轮（W1/W2，gate1-db-tools-2026-11-19.md，措辞回填不改 FR 语义/完备性结论）：**W1** FR-08/FR-10 审计动作码回填统一为 api D-16 权威码 dbt_tool_accessed（语义同一=访问开启，api 为下游权威，原别名废止）；**W2** gating 端点归属措辞回填 api D-11 定稿——第七部分消费方视角/第九部分 OHS 消费行/待对齐-2 由「account-auth 新增内部能力跨模块待同步」改为「db-tools 自持 GET /dbtool/gate/verify + 复用既有 VerifyWs（account-auth 零新增 HTTP，OHS 消费已登记 api-account-auth.md v1.2）」，待对齐-2 标记 D3 已定稿关闭；FR-01~10/完备性 G5/5·T8/8·N4/4/安全四防线映射/枚举均零变更 |
| v1.2 | 2026-11-19 | 自动 nginx 落地增量 D2（d0-clarification-dbtools-auto-landing-2026-11-19.md 拍板 Q1/Q1b/Q2/Q3，D1 三文档 BA v1.1/AA v1.2/FL v1.1 ⚠️A14~A17）：**+FR-11 工具落地自动供给**（C-15-05 机制型：0 匹配自动建 dougong-tools.conf listen 18090/端口占用 DBT_012 reason/既有块优先/多匹配引导/幂等复用/siteRef+Port+AutoCreated/两工具卸载清块/ConfigApply R-06/端口放行仅提示）+**FR-12 域名联动**（未绑 _ /已绑 BindDomain+非匹配 Host 404 镜像 HostGuard/钩子主通道+惰性兜底 D1 裁定/toolBaseHost Port 分支 URL）+**FR-13 HostGuard/EntryPathGuard 内部端点豁免**（panel-core S-07 机制侧跨模块待同步项，最小化豁免 /dbtool/gate/·/dbtool/signon/，非内部路径强校验不变）+**FR-14 失败原因可见**（worker finish 错误文本含 data.reason，沿 env-mgmt 先例）；FR-03/05/06/07/08/10 六既有 FR 口径扩展（落地选择/清块/reason/自动块 gating+豁免/signon 回调豁免/Port URL+惰性校正）；IN 10→14/OUT 13→17（+自动防火墙放行/端口可配/HTTPS 工具落地/面板 nginx 化不做，OUT-10 修订 nginx 接入层端口非工具进程端口）；规则汇总 +⚠️A14~A17+A6/A7 修订注记+幂等扩自动块/钩子失败语义推导行；安全四→五防线映射（+防线五自动落地块安全，防线一/二/四扩展+验收五专项）；状态流转仍 1 个状态机+辨析 6→9（自动块生命周期/域名联动/AutoCreated 非状态机）；T1a +panel-core 消费方视角；N1 +v1.2 锁定项（18090/块路径/http/双通道/零 DDL）；N2 20→27 异常分支；N3 +管线原子/向后兼容；第九部分集成 +panel-core 钩子 OHS/豁免机制侧依赖/ConfigApply 自动块扩展三行+过程数据 +siteRef 值对象〔非过程数据〕/钩子载荷 P5/探测结果 P7；待对齐-3 D1 裁定关闭+待对齐-7 新增（块模板/占用探测/钩子签名/惰性触发点/reason 文案/siteRef 字段名归 D3）；**领域建模增量 domain-model.md v1.26：零新聚合根/零 DDL——落地引用（siteRef）值对象显式建模 +Port/AutoCreated（JSON 载荷扩展），panel_config.bind_domain 实况值消费+钩子集成登记**；完备性增量复检通过 G 5/5·T 适用 8/8·N 4/4+变更影响范围表（供 D3 变更摘要）；枚举零新增零扩值 |
