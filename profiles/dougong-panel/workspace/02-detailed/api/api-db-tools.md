# 斗拱面板 数据库工具集成（db-tools）API 设计

| 属性 | 值 |
|------|-----|
| 版本 | v1.2 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-19（v1.2 Gate 1 WARN 修复轮：W3 +GET /dbtool/access/reopen SignonURL 重认证落地载体〔面板面 8→9，总端点 10→11〕+W4 SignonScript 会话 cookie 转发显式化） |
| 输入 | prd-db-tools.md v1.0（完备性检查通过：G 5/5、T 适用 8/8、N 4/4；FR-01~10）、domain-model.md v1.24（db_tool + tool_deploy_task 双聚合根、工具凭据值对象、工具条目投影、目录映射值对象、关联领域对象 db_instance）、D1 三文档（BA/AA/FL v1.0，⚠️A1~A13 + 安全架构 BA 2.4/AA 附录C）、d0-clarification-db-tools-integration-2026-11-19.md（拍板 Q1~Q3 + 安全底线）、prd-env-mgmt.md v1.3（FR-08 Node 环境供给） |
| 接口规约 | 栈规约 backend-02-api-conventions.md（go-vue3）；认证=HttpOnly Cookie 会话 + CSRF token（R-02，复用 account-auth）；鉴权=RBAC 两角色边界矩阵（ADR-018）+ 工具访问 admin 专属 gating（⚠️A10）；外部命令一律字面量参数数组（R-08）；**包络字段沿全项目既有契约 `message`（栈规约 §1 `msg` 的项目级统一偏差，沿 api-env-mgmt.md 等十二模块先例）；路径沿项目三段式 `/domain/resource/action`（G1-08，栈规约 §3 REST 形态的项目级裁剪）；分页沿项目 `page/pageSize + items/total` 先例** |

## 1. 模块设计概述

### 1.1 业务背景及目标

数据库工具集成是面板的数据库高级管理能力补位模块（用户 2026-11-19 拍板「宝塔式集成现成 Web 工具」，ADR-032）：集成 phpMyAdmin（PHP，管 MySQL/MariaDB）与 mongo-express（Node，管 MongoDB）两个现成工具，复用 env-mgmt 已管理环境（PHP-FPM 已就绪 + 本轮 +Node）与 site-mgmt 站点通道。**访问安全为不可妥协底线（Q2）**：工具仅绑 localhost（mongo-express 127.0.0.1:8081；phpMyAdmin 无独立端口），唯一访问路径=面板域下工具路径 nginx 反代，经 auth_request 校验面板会话 + admin 角色；phpMyAdmin signon 桥接（HMAC 令牌防伪 + 短时效 + 一次性 + vault 凭据 loopback 注入永不进浏览器）；mongo-express basic auth 面板生成经 nginx 服务端注入。凭据 AES-256-GCM 密文零回显（I-02）；卸载不删数据库数据；下载钉版本 + sha256；零遥测（R-09 下载用户主动例外）。面板内轻量浏览保留并存（Q3），治理台账仍归 database-mgmt。服务对象：admin 执行全部变更与工具访问；readonly 查看工具目录与任务（ADR-018），工具本体 gating 不可达（⚠️A10）。

### 1.2 边界说明

| 项 | 说明 |
|----|------|
| 本模块做 | 工具全景（目录+实况+依赖就绪度）、安装/升级/卸载（异步任务+sha256 校验+站点/服务落位+凭据生成）、任务台账观测、会话 gating 反代（内部鉴权端点）、signon 桥接（令牌签发/校验/凭据注入）、basic auth 注入与轮换、统一打开入口 |
| 本模块不做（接口边界） | ① Python/其他工具/Redis 工具——OUT（⚠️A1）；② 工具内 readonly 细粒度映射——OUT（⚠️A10 整体 admin 专属）；③ 上游新版本检测/降级/自选版本——OUT（⚠️A4）；④ 任务取消——OUT（⚠️A3）；⑤ 运行环境代装——归 env-mgmt（依赖缺失软引导）；⑥ 实例/凭据/库/用户治理台账——归 database-mgmt（本模块消费凭据 OHS）；⑦ 认证/鉴权/审计本体——复用 account-auth；⑧ 页面外壳与导航——归 panel-core；⑨ 高级管理入口按钮渲染——归 database-mgmt PageSpec（跨模块待同步项，本模块提供状态读契约与打开能力）；⑩ 面板内轻量浏览——归 database-mgmt S-56/S-57（保留并存 Q3） |
| 与存储的关系 | SQLite 单文件（与全模块同库）；db_tool + tool_deploy_task 两表（5.3，迁移 013）；工具运行态/依赖就绪度零台账（systemd/站点/env-mgmt 实况实时探测投影）；signon 令牌消费台账内存态（P7 敏感，面板重启全部失效） |

### 1.3 设计决策

| # | 决策 | 理由 |
|---|------|------|
| D-01 | **资源段裁定 `dbtool`**：路径 `/dbtool/{tool,task,access,gate,signon}/{action}` 三段式（G1-08）；tool=部署档案与变更、task=任务台账、access=打开/重开与轮换（open=面板主动打开、reopen=SignonURL 重认证 GET 落地，v1.2）、gate/signon=内部安全端点。不用 `tool` 裸段（语义泛化与他模块潜在冲突），不用 `db-tool` 连字符（项目资源段先例均单词：env/task/service/db/sites） | 三段式硬约束；域前缀 dbtool 与模块名 db-tools 对应（db→dbtool 沿 database-mgmt→db 先例） |
| D-02 | **实时探测投影**：tool/list 每请求即时探测两工具实况（部署状态/版本/运行态/依赖就绪度），无缓存层、无陈旧回退，响应带 generatedAt；部署状态=db_tool 档案存在性+已装版本（面板自持）叠加运行态实况投影；探测失败字段为 null（不虚构运行态）；依赖就绪度消费 env-mgmt 环境实况（进程内读契约，PHP-FPM/Node/nginx 已装态）+ PHP 扩展检测（php -m 参数数组 R-08） | site-mgmt D-01/env-mgmt D-01 投影真实性同口径；domain-model v1.24 db_tool 运行态不持久化判定 |
| D-03 | **工具路径形态=面板域 server 块持久 location 注入**（关闭 prd 待对齐-3）：工具路径不落独立站点（site-mgmt site/create 为 server 块模型，不匹配路径反代形态），改为向**面板域对应 nginx server 块**持久注入工具 location（机制沿 panel-core injectChallengeLocation 先例〔acme.go:360-437，span 插入+备份回滚〕的持久化 variant，经 site-mgmt R-06 安全写入管线 OHS）。**落点选择**：panel_config.bind_domain 非空→server_name 匹配块（优先）；未绑定→面板反代站点块（site-mgmt 投影中 proxy_pass 指向面板监听地址者，nginxx ServerBlock.Proxy() extract.go:42-50 识别）；多候选→任务 failed 如实（DBT_012 data.reason 列出候选，操作者经站点管理明确后面板重试）；无候选→任务 failed（引导先建立面板域 nginx 反代访问）。**注入内容**：工具 location（phpMyAdmin FastCGI/mongo-express proxy_pass）+ auth_request 指向内部 gate location + 内部 gate location（internal; proxy_pass 面板后端 /dbtool/gate/verify + X-Dbtool-Internal 秘密头）+ mongo-express Authorization 注入（include 秘密文件）；**卸载=撤除注入恢复原字节**（R-06 备份回滚同构） | D0 Q2「经面板 nginx 路径反代」拍板；panel-core ACME location 注入为已验证机制先例；独立站点形态与面板域 cookie scope 覆盖冲突（⚠️A7） |
| D-04 | **异步任务与同工具互斥**：install/upgrade/uninstall 同步校验（toolType/依赖前置/已装态/在途互斥 DBT_003）→ INSERT tool_deploy_task（status=pending）与发起审计同事务 → 立即返回 taskId；worker goroutine（wg 编排，创建即触发）消费：running 落库 → 执行命令链 → 终态落库 + 终态审计 + **部署档案登记/更新/删除同事务**（I-C01）；互斥=创建事务内判定同 tool_type 存在 pending/running 即拒（SQLite 单连接写串行）；**重启恢复**：启动扫描 running 任务标 failed（error=「面板重启中断」），worker 不续跑（下载/npm 中断态不安全） | prd FR-03~06；与 env-mgmt D-04 同构（互斥粒度 tool_type） |
| D-05 | **下载分发钉版本 + sha256 强制**（⚠️A12）：phpMyAdmin 官方 tarball URL 编译期钉死（版本+sha256 常量表 D-13）；下载经 Go net/http（DOUGONG_MIRROR 同构拼接式镜像前缀，install.sh:38-45 先例）→ sha256 逐字节校验（失败=任务 failed error=sha256_mismatch，**不降级不跳过不安装**，已下载文件清理）→ 解压到面板管理目录（tar 解压经参数数组 R-08，防 zip slip 路径校验沿 file-manager FILE_009 先例）；mongo-express 经 npm install 精确版本（`npm install --global mongo-express@{version}` 参数数组；registry 镜像选项 `--registry` 同构）；下载/npm 为用户主动发起 R-09 例外（与 ACME 同判） | install.sh:122-129 sha256 校验先例；D0 安全底线 |
| D-06 | **phpMyAdmin 部署形态**：解压目录={dataDir}/tools/phpmyadmin（编译期常量）；config.inc.php 经模板生成（auth_type=signon + **SignonURL 指向面板重开落地端点 GET /dbtool/access/reopen?instanceId={instanceId}**〔v1.2 W3——SignonURL 为 phpMyAdmin 未登录/会话过期时 302 GET 重定向目标，须为 GET 可达载体，access/open 为 POST+CSRF 不可作 SignonURL；instanceId 经面板打开时 Set-Cookie 下发的非敏感实例标识 cookie `dg_tool_instance`〔HttpOnly，Path=/dbtool/phpmyadmin/，SameSite=Lax，仅 db_instance.id 非凭据〕承载，config.inc.php 模板经 PHP 动态拼接 `$_COOKIE['dg_tool_instance']`——工具路径在面板域下 cookie scope 覆盖〕 + SignonScript 服务端回调脚本 + blowfish_secret 随机生成密文存储）；**PHP-FPM pool 复用 env-mgmt 已装 PHP-FPM 默认 pool**（不新建 pool——site-mgmt php_fpm 站点模板对接已有 pool 先例 S-27-02；pool 实况经 PhpFpmGateway 读取）；工具 location 经 D-03 注入（FastCGI pass 至 PHP-FPM pool 监听地址 + SCRIPT_FILENAME 映射 + auth_request）；**PHP 扩展依赖检测**：php -m 输出比对必需清单（PHPMYADMIN_REQUIRED_EXTS=mysqli,mbstring,zip,gd,json,curl,openssl，D-13），缺失=DBT_001 拒绝并列出缺失项引导补装 | D0 复用积木取证（PHP-FPM 已就绪 service.go:94 版本化 unit 扫描）；扩展清单 D0 列举定稿 |
| D-07 | **mongo-express 部署形态**：npm 全局安装精确版本；systemd 服务单元 dougong-mongo-express.service（单元文件经秘密文件通道 0600 写入 + SysdGateway DaemonReload/启停，单元文件编译期模板：ExecStart=node {globalRoot}/mongo-express/app.js，Environment 注入 ME_CONFIG_MONGODB_URL/ME_CONFIG_BASICAUTH_USERNAME/ME_CONFIG_BASICAUTH_PASSWORD/ME_CONFIG_SITE_BASEURL=/dbtool/mongo-express/ + **bind 127.0.0.1:8081**〔ME_CONFIG_SITE_PORT=8081，mongo-express 默认监听 0.0.0.0 须显式 ME_CONFIG_SITE_BIND_ADDRESS=127.0.0.1，D-13〕）；Mongo 连接凭据取自 vault（database-mgmt 实例凭据 OHS，D-10）构造 ME_CONFIG_MONGODB_URL（mongodb://user:pass@host:port/?authSource=admin，凭据 URL 编码）；**凭据仅经 systemd Environment 注入不进 argv/日志**（单元文件 0600 经 WriteSecretFileAtomic nginxx gateway.go:183 同构机制）；工具 location proxy_pass http://127.0.0.1:8081 + auth_request + Authorization 注入（D-08） | D0 Q2 拍板（bind 127.0.0.1:8081 + baseUrl 路径前缀 + env 注入）；R-08 参数数组 |
| D-08 | **basic auth 服务端注入与轮换**（⚠️A9）：面板生成随机强口令（crypto/rand 24B→base64url 32 字符，BOOTSTRAP_PASSWORD_BYTES 同值）；用户名固定 `dougong`；口令 AES-256-GCM 密文落 db_tool.cred_cipher（I-02 零回显）；nginx 注入经秘密文件 {dataDir}/tools/mongo-express-auth.conf（0600，内容=`proxy_set_header Authorization "Basic {base64(dougong:口令)}";`，工具 location include）——**口令明文仅生成与写秘密文件内存态，浏览器全程无感知**；轮换（access/rotate）=重新生成→更新密文+秘密文件+systemd env（ME_CONFIG_BASICAUTH_*）→重启服务→旧口令即时失效（同事务审计 dbt_credential_rotated，detail 不含口令）；**phpMyAdmin 无 basic auth**（signon 桥接，D-09），rotate 对 phpmyadmin 拒绝 DBT_009 | D0 Q2「mongo-express 面板设 basic auth」；nginx proxy_set_header 注入为服务端行为（浏览器不见凭据） |
| D-09 | **signon 桥接与令牌规约**（⚠️A8，安全核心）：phpMyAdmin auth_type=signon——浏览器访问工具路径未登录时 phpMyAdmin 重定向 SignonURL（**面板重开落地端点 GET /dbtool/access/reopen?instanceId={id}**，D-06，v1.2 W3）；**打开流程**：admin 经 access/open（toolType=phpmyadmin, instanceId）→ 面板签发令牌 → 返回工具 URL（附令牌片段）→ 浏览器经 gating 进入 phpMyAdmin → phpMyAdmin signon 流程执行 SignonScript（PHP 服务端脚本）→ **SignonScript 经 loopback HTTP POST 面板后端 /dbtool/signon/verify**（携令牌，D-11）→ 面板四重校验通过 → 消费 db-mgmt 凭据 OHS 取出 MySQL 凭据 → 响应 {user, password}（**仅 loopback 服务端通道，永不进浏览器**）→ SignonScript 设 $_SESSION['PMA_single_signon_user']/['password'] → phpMyAdmin 登录完成。**重认证流程（v1.2 W3，工具内 PMA 会话过期二次流程）**：phpMyAdmin 自身会话过期→302 GET SignonURL（reopen?instanceId={id}，浏览器顶层导航携面板会话 cookie dg_session 与 dg_tool_instance）→ reopen 经 admin 会话 gating（AuthRequired+RoleRequired admin，GET 无 CSRF）→ 校验实例就绪后 SignonService **重签新令牌**（sessionDigest=当前 gating 会话、instanceId 绑定、jti 新入台账）→ **302 回工具 URL（附新令牌片段）**→ 浏览器经 gating 再入 phpMyAdmin→SignonScript 再注入凭据（同打开流程后半段）——主打开流程（access/open）不变，reopen 为其 GET 重认证等价载体（4.1.4）。**令牌规约**：payload={jti(随机 16B hex), toolType, instanceId, sessionDigest(签发时会话标识), iat, exp}；签名=HMAC-SHA256(payload JSON, signon 密钥)；令牌=base64url(payload).base64url(sig)；**时效 SIGNON_TOKEN_TTL=60s**；**一次性**：消费台账内存 map[jti]consumed（TTL 清扫 goroutine 入 wg，面板重启全部失效=安全保守）；**四重校验**：①HMAC 签名（防篡改）②exp 时效（防过期）③jti 未消费（防重放，消费即标记）④instanceId 绑定+sessionDigest 匹配当前 gating 会话（防跨实例/跨会话重用）；signon 密钥=crypto/rand 32B，AES-256-GCM 密文落 db_tool.cred_cipher（phpmyadmin 项），密钥文件 dbtool-cred.key 0600（沿 db-cred.key 先例 vault.go:14-24） | D0 Q2 signon 桥接拍板；SignonScript 服务端回调为 phpMyAdmin 官方 signon 机制（凭据不经浏览器）；令牌四重校验对应 prd N2 四路拒绝 |
| D-10 | **实例凭据 OHS 消费**（进程内单向 db-tools→db-mgmt）：`ToolCredentialGateway { InstanceCredentialsForTool(ctx, instanceID int64) (ToolCreds, error); InstanceReadyForTool(ctx, instanceID int64) (ReadyInfo, error) }`——db-mgmt Service 实现（vault 解密仅服务端），main 装配注入；ToolCreds{Username, Password, Host, Port, Engine}（**明文仅服务端内存态，调用方用毕即弃，不落库不入日志不入审计**）；InstanceReadyForTool 校验实例存在+凭据已配置（credentialsSet）+引擎匹配（phpmyadmin→mysql/mariadb；mongo_express→mongodb）；未就绪→DBT_006 引导到 database-mgmt 配置凭据；**本模块不复制不持久化对方凭据**（domain-model v1.24 关联领域对象判定）；契约权威定义见 api-database-mgmt.md §4.9/§5.1（v2.4 同轮提供方登记） | 与 env-mgmt 消费 BootstrapAfterInstall 同构（api-database-mgmt.md §4.6 先例）；I-02 凭据零回显 |
| D-11 | **内部端点安全形态**（关闭 prd 待对齐-2，防 CSRF/越权）：**gate/verify**（nginx auth_request 子请求专用）与 **signon/verify**（PHP-FPM SignonScript 回调专用）均为内部端点，三重防护：①**loopback 源校验**（RemoteAddr 必须 127.0.0.1/::1，非 loopback 直接 HTTP 404 无包络——防探测，沿 HostGuard 标准 404 语义）；②**内部秘密头**（X-Dbtool-Internal: {随机秘密}，nginx/SignonScript 注入，面板比对；秘密=crypto/rand 32B hex 落 {dataDir}/tools/internal-secret 0600，安装时生成）；③**不挂会话/CSRF 中间件**（gate 消费只读会话校验 OHS 自行判定；signon 以令牌为凭据）——gating 为纯读校验无状态变更（CSRF 无攻击面：不产生副作用），角色判定服务端权威（不信任客户端声明）；**gate/verify 响应为裸 HTTP 状态**（200 放行/401 拒绝，auth_request 语义，无包络——nginx 仅认状态码）；signon/verify 响应为包络（成功 data={user,password}，失败 code=DBT_007/008/010）；**account-auth 无需新增 HTTP 接口**——gating 消费既有只读会话校验 OHS（VerifyWs 语义，session.go:59，同 I-11 判定只读不续期），account-auth OHS 消费已登记 api-account-auth.md v1.2（无新增 HTTP 接口） | auth_request 语义要求裸状态码；内部端点不对外暴露（loopback+秘密头双保险）；VerifyWs 只读不续期避免工具静态资源逐请求滑动续期写放大（会话续期经面板 API 正常发生；工具内停留超 idle 超时→gating 拒绝=安全预期，prd FR-07） |
| D-12 | **错误码族裁定：新 DBT 码族**（DBT_001~012）：工具域语义（依赖缺失/sha256/令牌三路/凭据未就绪等）与 DBM（引擎治理）/ENV（环境包操作）不同构，复用会语义混淆；鉴权/CSRF/越界复用 AUTH 族（AUTH_001/002/003/008）；内部端点拒绝用裸 HTTP 状态（gate）或 DBT 码（signon） | D0「新 DBT 码或复用，D3 裁定」；沿 ENV/DBM/SITE 各域独立码族先例 |
| D-13 | **编译期常量表**（⚠️A1/A12，关闭 prd 待对齐-4 部分）：PHPMYADMIN_VERSION=5.2.2（官方 tarball，sha256 编译期钉死——**具体 sha256 值后端落码时经官方发布页取证填入，设计侧钉版本不钉哈希文本**⚠️）；MONGO_EXPRESS_VERSION=1.0.0（npm 精确版本⚠️同上落码取证）；NODE_LTS_TRACK=22（NodeSource 22.x LTS 轨道，env-mgmt D3 定稿同步⚠️）；MONGO_EXPRESS_PORT=8081/MONGO_EXPRESS_BIND=127.0.0.1；TOOL_DIR={dataDir}/tools；TOOL_PATH_PREFIX_PHPMYADMIN=/dbtool/phpmyadmin/；TOOL_PATH_PREFIX_MONGO_EXPRESS=/dbtool/mongo-express/；PHPMYADMIN_REQUIRED_EXTS=mysqli,mbstring,zip,gd,json,curl,openssl；SIGNON_TOKEN_TTL=60s；TASK_OUTPUT_TAIL_LINES=200；DEPLOY_OP_TIMEOUT=600s；DOWNLOAD_TIMEOUT=300s；TASK_LIST_PAGE_SIZE=50 | D0 钉版本拍板；版本/sha256 具体值落码取证（设计不虚构哈希）；端口/前缀 D0 列举定稿 |
| D-14 | **升级=部署目录钉死新版本**（⚠️A4）：upgrade 前置已装（DBT_004）；目录版本=已装版本→DBT_011 如实拒绝（不虚构升级）；phpMyAdmin 下载新 tarball（sha256）→替换程序目录**保留 config.inc.php 与 signon 密钥**（conffile 保守先例 env-mgmt D-05）；mongo-express npm 重装新版本→重启服务（凭据注入形态不变）；升级不改变凭据与站点路径；档案版本随终态更新（I-C01） | ⚠️A4 拍板；配置不覆盖 |
| D-15 | **卸载保守与数据保全**（⚠️A5）：uninstall 前置已装（DBT_004）；执行链=停止并移除 systemd 服务（mongo-express）→撤除 nginx location 注入（恢复原字节，R-06）→删除程序目录/npm 包→删除 db_tool 档案与凭据密文（同事务+审计）；**绝不触及引擎数据目录与 db_instance 档案**（工具仅为访问通道）；强确认归前端（NModal 知情提示「高级入口不可用，轻量浏览与数据库数据不受影响」），服务端不硬阻断 | D0 安全底线「卸载不删数据」；instanceRemove 哲学 |
| D-16 | **审计动作码**（R-03 同事务脱敏）：`dbt_install_started / dbt_upgrade_started / dbt_uninstall_started / dbt_task_finished（终态，detail 含 action/status/error 摘要）/ dbt_tool_accessed（打开/访问开启，detail 含 toolType/instanceId，**不含凭据与令牌明文**）/ dbt_credential_rotated（轮换，detail 含 toolType，**不含口令**）`（category 均 change；object_type=db_tool/tool_deploy_task）；**gating 逐请求校验不落审计**（⚠️A11 防写放大）；signon 校验失败不落业务审计（安全日志通道 slog.Warn，拒绝事件归 D3 安全日志口径）；任务创建/终态与审计同事务（I-03 同构） | prd 第五部分 R-03/⚠️A11；ask 审计码要求（tool_installed/upgraded/removed/accessed 语义以 dbt_* 前缀承载，沿 env_* 命名先例） |
| D-17 | **混合模式（MIXED）声明**：全部对象实际为自管（无外部数据同步源；工具为本机部署通道非数据源），不设 manage_mode 字段，权限管控按角色区分（ADR-018 + ⚠️A10 工具访问 admin 专属） | 分册 Step 2 默认混合模式；本模块无托管侧 |
| D-18 | **Node 环境复用 env-mgmt 接口**（ask 明示）：本模块**不定义 Node 安装/管理接口**——Node 环境供给归 env-mgmt（POST /env/env/install envType=node，prd-env-mgmt.md FR-08）；本模块仅消费 env-mgmt 环境实况读契约（依赖就绪度）+ 依赖缺失软引导（PageSpec 指引链接）；**env-mgmt api Node 增量（env_type 八值/DDL CHECK 迁移/常量）为跨模块待同步项**（api-env-mgmt.md v1.2 尚为七值，其 D3 增量轮落定；本模块 DBT_001 依赖判定以 env-mgmt 实况读契约返回为准，不硬编码七值/八值） | 模块边界（D1 落定）；避免跨模块重复定义 |

### 1.4 领域对象总览

| 聚合根 | 域内对象/值对象 | 子流程 | 关联领域对象 |
|--------|---------------|--------|-------------|
| db_tool（主数据类子形态：部署档案+凭据密文，无状态机） | 工具凭据（值对象，basic auth 口令/signon 密钥密文内嵌）、工具条目（实体，目录常量+实况投影不落库）、目录映射（值对象，编译期） | 无（signon 桥接为同步机制链路，令牌消费台账 P7 内存态） | **db_instance（database-mgmt）**：mongo-express 项目标实例持久引用 + 凭据 OHS 消费（D-10）；phpMyAdmin signon 打开时实例绑定（令牌过程数据） |
| tool_deploy_task（主数据类子形态：四态任务状态机，非流程流转） | —（输出尾部/失败原因为聚合字段） | 无（任务状态机为聚合根本体，与 env_install_task/backup_run 同构） | 无（终态驱动 db_tool 档案登记为同域跨聚合协作，第 2 章） |

无领域事件与 MQ（单机单体；审计为进程内 OHS 出站调用）。

**功能类型判定**（分册 Step 1）：聚合根 db_tool/tool_deploy_task 存在、由本模块管理且有写操作、状态为**数据生命周期**（档案存在性+四态任务状态机，非审批流转）→ **主数据类子形态**；模块整体为「档案聚合根（db_tool）+ 单据聚合根（tool_deploy_task）+ 投影实体（工具条目）+ 安全机制服务集合（gating/signon/注入）」混合形态。据此裁剪：第 2 章启用（双聚合根跨聚合编排 I-C01）、4.2 按 db_tool_task_status 状态机填写、第 5 章必填（有 DDL）。

### 1.5 上下文映射

| 上下文 | 关系 | 机制 |
|--------|------|------|
| account-auth | 遵奉者 + OHS 消费 | 面板接口位于 Cookie 会话后（R-02，AuthRequired）；工具列表/任务两角色同等（ADR-018）、变更与打开/轮换 admin 专属（RoleRequired + AUTH_008）；写接口 CSRF（CsrfGuard）；审计写入 AuditService.Record（R-03 同事务脱敏）；**gating 消费只读会话校验 OHS（VerifyWs 语义 session.go:59，D-11）——account-auth 无需新增 HTTP 接口，OHS 消费已登记 api-account-auth.md v1.2** |
| panel-core | 消费其 HTTP 骨架 + 配置 | `/dbtool` 路由组挂载；外壳与菜单归 panel-core（B 批增量登记菜单「数据库工具」）；panel_config.bind_domain/监听地址/入口路径为 D-03 落点选择与内部端点 URL 构造输入（进程内读） |
| env-mgmt | OHS 消费（进程内读契约） | 环境实况读契约（PHP-FPM/Node/nginx 已装态与版本，依赖就绪度数据源）；Node 环境供给归 env-mgmt（D-18，POST /env/env/install envType=node）；依赖缺失软引导回 env-mgmt 安装页（PageSpec 层）；**env-mgmt api Node 增量（env_type 八值/迁移）为跨模块待同步项** |
| site-mgmt | OHS 消费（进程内） | **ConfigWritePipeline OHS（R-06 四步安全写入管线泛化提供：Apply(ctx,file,newContent,allowNew)/Remove(ctx,file)，pipeline.go:21 applyPipeline 先例）**——D-03 location 注入/撤除经此通道；nginxx 适配层共享（ExtractServers/ServerBlock.Proxy() 落点识别）；PhpFpmGateway pool 实况读取（D-06）；**已登记 api-site-mgmt.md v2.8（D-21/§5.1 同轮提供方登记）** |
| service-mgmt | 共享适配层 | SysdGateway systemd 网关共享（mongo-express 单元创建/启停/重启/移除/状态查询，进程内适配器非模块调用）；单元文件写入经秘密文件机制（0600） |
| database-mgmt | **OHS 消费（进程内，单向硬依赖）+ 被消费（读契约）** | 消费 ToolCredentialGateway（D-10：InstanceCredentialsForTool/InstanceReadyForTool，vault 凭据取出仅服务端）——契约已登记 api-database-mgmt.md §4.9/§5.1（v2.4）；**被消费：ToolStatusForEngine 读契约（高级管理入口状态源，5.1）+ 打开能力共用（access/open）**——database-mgmt PageSpec 入口渲染已兑现（v1.5，D4 轮） |
| phpMyAdmin / mongo-express（外部现成工具） | ACL（适配器封装） | 工具协议（signon 回调/basic auth/env 注入/baseUrl）经适配器封装，工具内部模型不渗透面板领域模型；工具仅绑 localhost 经 gating 访问（⚠️A6/A7）；下载为用户主动 R-09 例外；工具本体遥测经配置关闭（phpMyAdmin $cfg['VersionCheck']=false） |
| nginx/PHP-FPM/systemd/npm（本机系统组件） | 数据源与动作通道 | nginxx/phpfpm/sysd 适配器 + npm runner（R-08 参数数组）；本机资源非外部业务系统对接 |

## 2. 跨聚合根设计

### 2.1 聚合根关系图

```
db_tool（部署档案+凭据密文）
   ▲ 终态驱动登记/更新/删除（同事务，I-C01）
   │
tool_deploy_task（部署任务四态机）── 同工具互斥（I-01，tool_type 粒度）

db_tool ── 目标实例引用（mongo-express 项）──▶ db_instance（database-mgmt，关联领域对象，凭据 OHS 消费 D-10）
db_tool ── 工具凭据值对象（密文内嵌）
工具条目（投影实体）── 部署状态 = db_tool 档案存在性 + 实况探测（无守门，经读取契约访问）
```

### 2.2 跨聚合根不变量

| 编号 | 不变量 | 违反处理 |
|------|--------|---------|
| I-C01 | **任务终态与档案一致**：install succeeded → db_tool 档案登记（版本/路径/站点引用/服务单元引用/凭据密文）同事务；upgrade succeeded → 档案版本更新同事务；uninstall succeeded → 档案+凭据密文删除同事务；任务 failed → 档案不变（安装失败不留半档案） | worker 终态回写事务内编排（D-04）；违反=事务回滚任务终态与档案同失败 |
| I-C02 | **凭据密文与部署落位一致**：basic auth 轮换 → db_tool.cred_cipher 替换 + nginx 秘密文件更新 + systemd env 更新 + 服务重启，四步链路失败回滚（密文不更新，旧口令保持有效）；signon 密钥随档案创建/删除 | 轮换编排事务+文件回滚（D-08）；失败=DBT_012 如实呈现 |
| I-C03 | **卸载数据保全**：uninstall 执行链不触及引擎数据目录与 db_instance 档案（跨域硬约束） | 执行链白名单（仅工具目录/单元/注入配置/本域档案，D-15） |

### 2.3 跨聚合根编排

| 编排 | 触发 | 步骤 | 一致性 |
|------|------|------|--------|
| 安装落档 | install 任务 succeeded | worker 终态事务：INSERT db_tool（凭据密文+部署引用）+ UPDATE task 终态 + 终态审计 | 同事务原子（I-C01） |
| 升级换版 | upgrade 任务 succeeded | worker 终态事务：UPDATE db_tool.version + UPDATE task 终态 + 审计（凭据/路径不变） | 同事务原子 |
| 卸载除档 | uninstall 任务 succeeded | worker 终态事务：DELETE db_tool + UPDATE task 终态 + 审计（部署解除在执行链先行完成） | 同事务原子 |
| 凭据轮换 | access/rotate（同步） | 生成新口令→更新秘密文件+systemd env→重启服务→成功后 UPDATE cred_cipher+审计（同事务）；失败回滚文件 | I-C02 |

## 3. 应用/基础设施层设计

### 3.1 接口规范

| 项 | 规范 |
|----|------|
| 路径格式 | `/dbtool/{resource}/{action}` 三段式（D-01）；资源单数（tool/task/access/gate/signon） |
| HTTP 方法 | 读=GET，写=POST（面板内部契约） |
| 响应格式 | `{"code": 0, "message": "ok", "data": {...}}`（全项目同一契约）；**例外：gate/verify 为裸 HTTP 状态（200/401，auth_request 语义，D-11）** |
| 时间格式 | ISO-8601 UTC（公共约定） |
| 认证 | 面板接口复用 account-auth：HttpOnly Cookie `dg_session`（AuthRequired）；内部端点（gate/signon）不挂会话中间件（loopback+秘密头+自行校验，D-11） |
| CSRF | 全部面板 POST 动作须带 `X-CSRF-Token` 头（R-02）；GET 豁免（**含 access/reopen——SignonURL 302 GET 重定向载体须 GET 可达，无 CSRF 头可携；其安全性由 admin 会话 gating + 令牌 sessionDigest/instanceId 双绑定 + 一次性短时效承载，CSRF 诱导的 GET 仅能为已认证 admin 自身会话重开工具，无跨会话/越权面，v1.2**）；内部端点豁免（纯读校验/令牌凭据，无 CSRF 攻击面，D-11） |
| 中间件链 | 面板接口：AuthRequired → CsrfGuard（POST）→ RoleRequired（3.3 矩阵）；内部端点：LoopbackGuard → InternalSecretGuard → 服务层自行校验（gate 会话+角色/signon 令牌四重） |
| 用户类型标注 | 查看接口（tool/list、task/list、task/detail）：admin+readonly；面板写接口与工具访问接口（install/upgrade/uninstall/open/**reopen**/rotate）：admin（reopen 为 GET 重定向落地但 admin 专属，⚠️A10）；内部端点：非用户面（nginx/PHP-FPM 服务端调用） |
| DTO 可空纪律 | **JSON null ≠ TS undefined**：可空字段一律显式 `|null` 下发（后端 *T 无 omitempty），前端类型逐形状对齐；未携带=null 语义的字段不出现（沿 database-mgmt v2.3 AL-003 先例） |

### 3.2 依赖与约束

| 依赖 | 类型 | 约束 |
|------|------|------|
| account-auth 中间件 + VerifyWs OHS | 框架 + OHS | AuthRequired/CsrfGuard/RoleRequired；gating 消费只读会话校验（session.go:59，不得自建会话解析） |
| account-auth AuditService.Record | OHS | 操作审计（R-03）；任务创建/终态/打开/轮换与审计同事务；detail 脱敏（不含凭据/口令/令牌明文） |
| env-mgmt 环境实况读契约 | OHS（进程内读） | 依赖就绪度（PHP-FPM/Node/nginx 已装态）；Node 安装归 env-mgmt（D-18） |
| site-mgmt ConfigWritePipeline + nginxx + PhpFpmGateway | OHS + 共享适配层 | R-06 四步管线（location 注入/撤除/秘密文件）；落点识别（ExtractServers/Proxy()）；pool 实况 |
| service-mgmt SysdGateway | 共享适配层 | mongo-express 单元生命周期与状态查询；不复制 systemd 通道实现 |
| database-mgmt ToolCredentialGateway | OHS（进程内单向） | vault 凭据取出（D-10）；凭据明文仅服务端内存态，本域不持久化 |
| SQLite | 存储 | db_tool + tool_deploy_task 两表（5.3）；单连接写串行承载互斥与跨聚合同事务原子性 |
| panel-core | 框架 | 路由组挂载 `/dbtool`；菜单登记；panel_config 读（bind_domain/监听/入口路径） |
| npm/PHP-FPM/nginx/systemd 本机二进制 | 动作通道 | R-08 参数数组；DEPLOY_OP_TIMEOUT/DOWNLOAD_TIMEOUT；全注入化（CI 预防） |

### 3.3 权限模型

角色：admin / readonly（panel_role）；无访客可达面；**工具本体访问整体 admin 专属（⚠️A10，gating 角色校验）**。

**接口权限矩阵**：

| 接口 | admin | readonly | CSRF | 备注 |
|------|-------|----------|------|------|
| GET /dbtool/tool/list | ✓ | ✓（ADR-018 全可看） | —（GET） | 目录+实况+依赖就绪度 |
| POST /dbtool/tool/install | ✓ | ✗（ADR-018 禁变更） | ✓ | 异步任务 |
| POST /dbtool/tool/upgrade | ✓ | ✗（同上） | ✓ | 异步任务 |
| POST /dbtool/tool/uninstall | ✓ | ✗（同上） | ✓ | 异步任务 |
| GET /dbtool/task/list | ✓ | ✓（ADR-018） | —（GET） | |
| GET /dbtool/task/detail | ✓ | ✓（ADR-018） | —（GET） | |
| POST /dbtool/access/open | ✓ | ✗（⚠️A10 工具访问 admin 专属） | ✓ | 访问开启审计 |
| GET /dbtool/access/reopen | ✓ | ✗（⚠️A10 工具访问 admin 专属） | —（GET 重定向） | SignonURL 重认证落地载体（v1.2 W3）；admin 会话 gating，重签令牌 302 回工具 URL；访问开启审计 |
| POST /dbtool/access/rotate | ✓ | ✗（ADR-018 禁变更） | ✓ | 仅 mongo_express |
| GET /dbtool/gate/verify | —（内部） | —（内部） | —（纯读） | nginx auth_request 子请求；loopback+秘密头；会话+admin 校验（readonly/无会话→401） |
| POST /dbtool/signon/verify | —（内部） | —（内部） | —（令牌凭据） | PHP-FPM SignonScript 回调；loopback+秘密头；令牌四重校验 |

数据权限规则：无行级隔离——工具全景与任务台账两角色可见集完全一致；角色差异体现在写接口与工具访问面（readonly 工具路径经 gating 401/403 不可达，界面打开/轮换入口不渲染）。readonly 越界由 RoleRequired 拦截返回 AUTH_008（附边界原因）。

### 3.4 公共规范

**错误码**（DBT_ 前缀本模块新增 12 个，D-12；鉴权/CSRF/越界复用 account-auth 码族）：

| code | 标识 | HTTP 语义 | 说明 |
|------|------|----------|------|
| 0 | OK | 成功 | — |
| DBT_001 | DEPENDENCY_MISSING | 拒绝 | 依赖前置未满足（PHP-FPM/Node/nginx 未装或 PHP 扩展缺失，D-06）；data.missing=[{kind: env\|extension, name}]；引导到环境管理 |
| DBT_002 | INVALID_TOOL_TYPE | 参数错误 | toolType 不在 db_tool_type 两值内（R-14） |
| DBT_003 | TASK_IN_FLIGHT | 参数冲突 | 同工具在途任务互斥（D-04，data.taskId 指向在途任务） |
| DBT_004 | NOT_INSTALLED | 拒绝 | 工具未安装（upgrade/uninstall/open/rotate 前置；data.installed=false） |
| DBT_005 | TASK_NOT_FOUND | 拒绝 | 任务不存在（台账实况） |
| DBT_006 | INSTANCE_NOT_READY | 拒绝 | 目标实例未纳管/凭据未配置/引擎不匹配（D-10 InstanceReadyForTool）；data.instanceId?；引导到 database-mgmt |
| DBT_007 | TOKEN_INVALID | 拒绝 | signon 令牌签名篡改/实例绑定不符/会话绑定不符（D-09 校验①④） |
| DBT_008 | TOKEN_EXPIRED | 拒绝 | signon 令牌过期（D-09 校验②，exp 超 60s） |
| DBT_009 | ROTATE_NOT_SUPPORTED | 拒绝 | 轮换仅 mongo_express 支持（phpmyadmin signon 密钥无轮换入口，D-08） |
| DBT_010 | TOKEN_REPLAYED | 拒绝 | signon 令牌已消费（D-09 校验③，一次性防重放） |
| DBT_011 | ALREADY_CURRENT_VERSION | 拒绝 | 升级时目录版本=已装版本（D-14，不虚构升级） |
| DBT_012 | DEPLOY_OP_FAILED | 操作失败 | 部署/轮换操作失败（落点冲突/注入失败/服务创建失败/文件回滚等）；data.reason |
| AUTH_001/002/003/008 | （复用） | — | 未登录/过期/CSRF 失败/readonly 越界（account-auth 归口） |

> 内部端点拒绝语义：gate/verify 失败=裸 HTTP 401（无包络，D-11）；signon/verify 失败=包络 DBT_007/008/010（HTTP 200）；loopback/秘密头校验失败=裸 HTTP 404（防探测，D-11）。任务执行链失败（sha256_mismatch/下载超时/npm 失败等）落 task.error 文本分类（非接口错误码，worker 终态语义沿 env-mgmt 先例）。

**枚举**（本域 D2 定稿入池 3 项 + 引用，M3 核对）：

| 枚举标识 | 取值 | 用途 |
|---------|------|------|
| db_tool_type | phpmyadmin \| mongo_express | 工具标识（全部接口 toolType 参数/响应；目录键；D2 定稿） |
| db_tool_task_action | install \| upgrade \| uninstall | 任务动作（task/list·detail 响应；D2 定稿） |
| db_tool_task_status | pending \| running \| succeeded \| failed | 任务状态（task/list·detail 响应与筛选；D-04 四态机；D2 定稿） |
| env_type | nginx \| apache \| docker \| mysql \| redis \| php_fpm \| mongodb \| node | 依赖就绪度 items.envType（env-mgmt 实况读契约返回；**八值含 node 为 env-mgmt D3 增量待同步项，本模块透传不硬编码**，D-18） |
| db_engine | mysql \| mariadb \| redis \| mongodb | 工具目标引擎（tool/list targetEngines；phpmyadmin→mysql/mariadb，mongo_express→mongodb；D-10 引擎匹配） |
| panel_role / audit_action / audit_result / audit_source | （引用） | 权限与审计（account-auth 定稿） |

> db_tool_state 不采用（D2 裁定：部署状态=档案存在性+实况投影非状态机，池行注明）。

**业务常量**（审计细粒度动作码，D-16，非枚举池条目）：`dbt_install_started / dbt_upgrade_started / dbt_uninstall_started / dbt_task_finished / dbt_tool_accessed / dbt_credential_rotated`（category 均 change；object_type=`db_tool`/`tool_deploy_task`，object_id=toolType 或 taskId；detail 脱敏不含凭据/口令/令牌明文）。

**业务常量**（操作参数与阈值，D-13，非枚举池条目）：

| 常量 | 值 | 说明 |
|------|-----|------|
| PHPMYADMIN_VERSION | 5.2.2 ⚠️ | phpMyAdmin 钉死版本（官方 tarball；sha256 落码取证填入，设计不虚构哈希） |
| MONGO_EXPRESS_VERSION | 1.0.0 ⚠️ | mongo-express npm 精确版本（落码取证确认） |
| NODE_LTS_TRACK | 22 ⚠️ | NodeSource LTS 轨道（env-mgmt D3 定稿同步） |
| MONGO_EXPRESS_PORT | 8081 | mongo-express 监听端口（编译期定死，⚠️A6） |
| MONGO_EXPRESS_BIND | 127.0.0.1 | mongo-express 绑定地址（ME_CONFIG_SITE_BIND_ADDRESS，⚠️A6） |
| TOOL_DIR | {dataDir}/tools | 工具管理目录根（phpmyadmin 子目录/mongo-express 秘密文件/internal-secret） |
| TOOL_PATH_PREFIX_PHPMYADMIN | /dbtool/phpmyadmin/ | 面板域工具路径前缀（baseUrl） |
| TOOL_PATH_PREFIX_MONGO_EXPRESS | /dbtool/mongo-express/ | 同上（ME_CONFIG_SITE_BASEURL） |
| PHPMYADMIN_REQUIRED_EXTS | mysqli,mbstring,zip,gd,json,curl,openssl | PHP 扩展必需清单（D-06，php -m 比对） |
| SIGNON_TOKEN_TTL | 60s | signon 令牌时效（D-09） |
| SIGNON_REOPEN_PATH | /dbtool/access/reopen | phpMyAdmin SignonURL 重认证落地路径（D-06/D-09，v1.2 W3；GET admin 会话 gating，重签令牌 302 回工具 URL） |
| TOOL_INSTANCE_COOKIE | dg_tool_instance | 打开时 Set-Cookie 下发的非敏感实例标识 cookie（值=db_instance.id，HttpOnly，Path=/dbtool/phpmyadmin/，SameSite=Lax；config.inc.php SignonURL 经 PHP 动态拼接 $_COOKIE 取 instanceId，D-06，v1.2；非凭据，I-02 不受影响） |
| SIGNON_KEY_BYTES | 32 | signon 密钥随机字节数（crypto/rand） |
| BASIC_AUTH_PASSWORD_BYTES | 24 | basic auth 口令随机字节数（crypto/rand→base64url 32 字符） |
| BASIC_AUTH_USERNAME | dougong | basic auth 固定用户名（D-08） |
| TASK_OUTPUT_TAIL_LINES | 200 | 任务输出尾部采集行数（终态定格） |
| DEPLOY_OP_TIMEOUT | 600s | 单部署命令超时（npm/解压/服务操作） |
| DOWNLOAD_TIMEOUT | 300s | tarball 下载超时 |
| TASK_LIST_PAGE_SIZE | 50 | 任务列表默认分页 |
| TOKEN_SWEEP_INTERVAL | 30s | 令牌消费台账 TTL 清扫间隔（goroutine 入 wg） |

**配置项**（config 扩展，沿 sites.*/cron.* 先例）：`tools.mirror`（下载镜像前缀，DOUGONG_MIRROR 同构，默认空=官方源）；`tools.npm_registry`（npm registry 镜像，默认空=官方 registry）；其余编译期常量不入配置（⚠️A1 非用户可配）。

## 4. 聚合根与领域对象详细设计

### 4.1 db_tool（聚合根，主数据类子形态）

#### 4.1.1 领域模型

**表结构组合**：db_tool 单表（部署档案+凭据密文；工具运行态/依赖就绪度零台账投影不落库，D-02）。

**字段清单**：

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | 聚合根标识 |
| tool_type | TEXT | NOT NULL UNIQUE CHECK(in phpmyadmin,mongo_express) | 工具类型（一工具一档案，UNIQUE 承载） |
| installed_version | TEXT | NOT NULL | 已装版本（安装/升级终态更新，I-C01） |
| deploy_path | TEXT | NOT NULL | 程序目录（phpmyadmin={TOOL_DIR}/phpmyadmin；mongo_express=npm 全局根） |
| site_ref | TEXT | NOT NULL | 注入落点标识（来源文件路径+server 块定位，D-03；撤除寻址用） |
| unit_name | TEXT | NULL | systemd 单元名（mongo_express=dougong-mongo-express.service；phpmyadmin=NULL 无独立单元） |
| instance_id | INTEGER | NULL | 目标实例引用（mongo_express 项→db_instance.id；phpmyadmin=NULL 打开时绑定，D-10） |
| cred_cipher | TEXT | NOT NULL | 工具凭据密文（AES-256-GCM 整体加密 JSON：mongo_express={basicAuthPassword}；phpmyadmin={signonKey,blowfishSecret}；I-02 零回显） |
| created_at / updated_at | DATETIME | NOT NULL | 安装时间 / 更新时间（轮换/升级） |

**不变量**（聚合内）：

| 编号 | 不变量 | 违反处理 |
|------|--------|---------|
| I-01 | 一工具一档案：tool_type UNIQUE | 创建事务内判定（已装再安装=DBT_004 语义分流升级，D-14） |
| I-02 | 凭据零回显：cred_cipher 任何接口响应/日志/审计 detail 不含明文/密文/密钥；明文仅生成与注入内存态 | 读取侧保证（DTO 无凭据字段）；dbtool-cred.key 0600 不经接口下发 |
| I-03 | 档案变更与审计同事务（安装落档/升级换版/卸载除档/轮换） | D-04/D-08 同事务编排 |
| I-04 | 凭据密文与部署落位一致（I-C02 聚合内投影）：轮换四步链路失败回滚 | D-08 编排 |
| I-05 | 卸载数据保全（I-C03）：执行链白名单不触及引擎数据 | D-15 结构保证 |

**领域服务**：ToolCatalog（编译期目录+映射，D-13）、ToolProber（实况探测编排：部署状态/版本+运行态+依赖就绪度，D-02）、DeployWorker（D-04 执行链：依赖检测→下载校验→部署落位→凭据生成→档案登记）、AccessService（打开分发+令牌签发 D-09+轮换 D-08）、GateService（D-11 会话+角色校验）、SignonService（D-09 令牌四重校验+凭据注入）、ToolCredentialVault（AES-256-GCM，dbtool-cred.key 0600，沿 CredentialVault 先例）。

#### 4.1.2 状态模型

db_tool **无自有状态机**（部署状态=档案存在性+实况投影，domain-model v1.24 辨析；db_tool_state 不采用入池）：

| 易混淆项 | 辨析 | 结论 |
|---------|------|------|
| 部署状态（未装/已装） | 档案存在性（面板自持）+已装版本，非状态迁移 | 档案存在性，非状态机 |
| 运行态 | mongo-express systemd 实况/phpMyAdmin 站点+pool 可用性投影 | 实况投影，非状态机 |
| 依赖就绪度 | env-mgmt 环境实况消费结论，整值刷新 | 域级判定值，非状态机 |
| 凭据轮换 | 值对象替换式更新（旧口令即时失效），无中间态 | 值对象替换，非状态机 |

#### 4.1.3 子流程设计

无（signon 桥接为「打开→签发→回调校验→注入」同步机制链路，令牌消费台账 P7 内存态非独立聚合；部署 worker 为后台机制链路创建即触发）。

#### 4.1.4 接口设计

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /dbtool/tool/list | GET | admin+readonly | 工具全景（目录+实况+依赖就绪度；实时探测 D-02） |
| /dbtool/tool/install | POST | admin | 安装（依赖前置+sha256+站点/服务落位+凭据生成；异步任务 D-04） |
| /dbtool/tool/upgrade | POST | admin | 升级（目录钉死新版本 D-14；异步任务） |
| /dbtool/tool/uninstall | POST | admin | 卸载（解除部署不删数据 D-15；异步任务） |
| /dbtool/access/open | POST | admin | 打开工具（signon 签发/gating 直达分发；访问开启审计） |
| /dbtool/access/reopen | GET | admin | SignonURL 重认证落地载体（v1.2 W3；phpMyAdmin 会话过期 302 GET 重签令牌回工具 URL；访问开启审计） |
| /dbtool/access/rotate | POST | admin | basic auth 轮换（仅 mongo_express，D-08） |

**接口详情**（全部面板写接口隐含 CSRF 与 AUTH_008 角色边界；install/upgrade/uninstall 隐含互斥 DBT_003，不再逐条重复）：

- **GET /dbtool/tool/list**：无参。处理：ToolCatalog 两条目 × ToolProber 逐工具探测（部署状态/版本经 db_tool 档案+程序目录/npm 包查询；运行态 mongo_express 经 SysdGateway 单元状态、phpmyadmin 经注入 location 存在+PhpFpmGateway pool 可用性；依赖就绪度经 env-mgmt 环境实况读契约〔PHP-FPM/Node/nginx〕+php -m 扩展比对〔D-06〕）→ 在途任务标记（pending/running 归组）。响应 data=`{items:[{toolType: db_tool_type, displayName, description, targetEngines: db_engine[], catalogVersion, installed: boolean, installedVersion: string|null, boundInstanceId: number|null, running: boolean|null, depsReady: boolean, depsMissing: [{kind: "env"|"extension", name: string}], runningTaskId: number|null}], generatedAt}`（installed=档案存在性；installedVersion 未装=null；**boundInstanceId=部署绑定实例（mongo_express 项→db_tool.instance_id；phpmyadmin=null 打开时绑定，v1.1 新增——database-mgmt 高级入口绑定校验数据源，PageSpec 6.8**）；running 探测失败=null 不虚构；depsMissing 空数组=就绪）。审计：—（查看类）。
- **POST /dbtool/tool/install**：req `{toolType: db_tool_type, instanceId?: number}`（mongo_express 必填 instanceId——部署时绑定目标实例 D-07；phpmyadmin 忽略 instanceId——打开时绑定 D-09）。校验：toolType（DBT_002）；依赖前置（DBT_001，data.missing）；mongo_express 实例就绪（DBT_006，InstanceReadyForTool）；未装（已装→DBT_004 语义提示走升级）；I-01 互斥（DBT_003）。执行：INSERT 任务（pending）+发起审计同事务 → worker 异步（D-05 下载 sha256→D-06/D-07 部署落位→D-03 location 注入→凭据生成密文→终态+档案登记同事务 I-C01）。响应 data=`{taskId, action: "install"}`。审计：dbt_install_started + dbt_task_finished。
- **POST /dbtool/tool/upgrade**：req `{toolType}`。校验：toolType（DBT_002）；已装（DBT_004）；目录版本≠已装版本（DBT_011）；互斥（DBT_003）。执行：INSERT 任务（action=upgrade）+审计 → worker（D-14 替换程序目录保留配置/凭据→mongo_express 重启服务→终态+档案版本更新同事务）。响应 data=`{taskId, action: "upgrade"}`。审计：dbt_upgrade_started + dbt_task_finished。
- **POST /dbtool/tool/uninstall**：req `{toolType}`。校验：toolType（DBT_002）；已装（DBT_004）；互斥（DBT_003）。执行：INSERT 任务（action=uninstall）+审计 → worker（D-15 停止移除服务→撤除注入恢复原字节→删除程序目录/npm 包→终态+档案删除同事务；**不触及引擎数据**）。响应 data=`{taskId, action: "uninstall"}`。审计：dbt_uninstall_started + dbt_task_finished。
- **POST /dbtool/access/open**：req `{toolType, instanceId?: number}`（phpmyadmin 必填 instanceId——令牌绑定实例 D-09；mongo_express 忽略——部署时已绑定）。校验：toolType（DBT_002）；已装（DBT_004）；运行态就绪（running=false/null→DBT_012 data.reason 服务未运行）；phpmyadmin 实例就绪（DBT_006）。执行：**phpmyadmin**——SignonService 签发令牌（D-09 payload+HMAC，jti 入消费台账未消费态）→ 构造工具 URL（{面板域}{TOOL_PATH_PREFIX_PHPMYADMIN}?dg_signon={token}）→ 响应 Set-Cookie `dg_tool_instance={instanceId}`（HttpOnly，Path=/dbtool/phpmyadmin/，SameSite=Lax，非敏感实例标识——供 config.inc.php SignonURL 动态拼接 instanceId 承载重认证落地，D-06/TOOL_INSTANCE_COOKIE，v1.2）；**mongo_express**——构造工具 URL（{面板域}{TOOL_PATH_PREFIX_MONGO_EXPRESS}，gating+basic auth 注入服务端处理）→ 访问开启审计（同事务）。响应 data=`{toolType, url: string, tokenExpiresAt: string|null}`（phpmyadmin tokenExpiresAt=iat+60s ISO-8601；mongo_express=null——**令牌明文仅此次响应下发给 admin 浏览器用于 signon 重定向，凭据永不下发**；url 为面板域工具路径，浏览器经 gating 进入）。审计：dbt_tool_accessed（detail={toolType, instanceId?}，不含令牌/凭据）。
- **GET /dbtool/access/reopen**（v1.2 W3，phpMyAdmin SignonURL 重认证落地载体，phpmyadmin 专属——mongo_express 无 signon 不经此端点）：query `{instanceId: number}`（必填；instanceId 来源=D-06 `dg_tool_instance` cookie 经 config.inc.php SignonURL 动态拼接）。**权限**：AuthRequired（面板会话 cookie `dg_session`）+ RoleRequired admin（⚠️A10 工具访问 admin 专属）；**GET 重定向流程无 CSRF**（3.1——SignonURL 302 GET 须可达，安全性由会话 gating+令牌双绑定承载）。校验：instanceId 实例就绪（DBT_006，InstanceReadyForTool）；phpmyadmin 已装（DBT_004）；运行态就绪（running=false/null→DBT_012 data.reason 服务未运行）。执行：SignonService **重签新令牌**（D-09 payload+HMAC，sessionDigest=当前 gating 会话 Identity.SessionDigest、instanceId 绑定、jti 新入消费台账未消费态）→ 构造工具 URL（{面板域}{TOOL_PATH_PREFIX_PHPMYADMIN}?dg_signon={newtoken}）→ 访问开启审计（同事务）。响应：**HTTP 302**，`Location=`工具 URL（附新令牌片段）——**非包络 JSON**（浏览器顶层导航形态，phpMyAdmin 未登录/会话过期 302 GET 可达）；令牌明文仅附于 302 Location 工具 URL 片段下发 admin 浏览器用于 signon 重定向（同 access/open 口径，**凭据永不下发**）。**失败呈现（浏览器导航形态，不返回 JSON）**：无会话/过期/吊销→302 回面板登录入口（重认证后可重新打开）；readonly 越界（AUTH_008 语义）/未装（DBT_004）/实例未就绪（DBT_006）/运行态未就绪（DBT_012）→302 回面板 /db-tools 工具管理页（呈现对应引导/边界，不虚构可登录）。审计：dbt_tool_accessed（detail={toolType:'phpmyadmin', instanceId}，不含令牌/凭据）。**安全口径**：与 access/open 同一令牌规约（HMAC+60s+一次性+instanceId/sessionDigest 双绑定），reopen 仅换取新令牌**不放宽任何校验**；重签前实时会话校验（吊销/改密级联即时切断）；凭据零回显（I-02）。
- **POST /dbtool/access/rotate**：req `{toolType}`。校验：toolType（DBT_002）；toolType=mongo_express（phpmyadmin→DBT_009）；已装（DBT_004）。执行：D-08 四步链路（生成新口令→秘密文件+systemd env 更新→重启服务→成功后 cred_cipher 替换+审计同事务；失败回滚 DBT_012）。响应 data=`{toolType, rotatedAt: string}`（**无口令字段**，零回显）。审计：dbt_credential_rotated（detail={toolType}，不含口令）。

**跨接口复用规则**：实况探测统一 ToolProber（list/open/reopen 前置/安装 action 判定共用）；部署执行统一 DeployWorker；令牌签发/校验统一 SignonService（open/reopen 签发共用同一规约 D-09）；审计统一 AuditService.Record（I-03 同事务）。

**幂等语义**：install 已装报 DBT_004（引导升级，非静默成功）；upgrade 同版本报 DBT_011；uninstall 未装报 DBT_004；在途互斥 DBT_003；rotate 重复提交=新口令替换（幂等终态=最新口令生效）；open/reopen 重复提交=新令牌签发（旧令牌未消费仍有效至 exp，一次性消费互不干扰；reopen 为 GET 重认证天然可重复触发）；task/detail 重复读无副作用。防重复提交为前端在途态（P3）。

#### 4.1.5 领域事件与集成

无 MQ、无领域事件。仓储契约：`ToolRepository { Create(tx), GetByType, UpdateVersion(tx), UpdateCredCipher(tx), Delete(tx), List }`。**跨模块 OHS 消费契约（进程内单向 db-tools→db-mgmt，D-10）**：

```
ToolCredentialGateway {
  // vault 解密仅服务端；明文用毕即弃，不落库不入日志不入审计（I-02）
  InstanceCredentialsForTool(ctx, instanceID int64) (ToolCreds, error)
  // 实例存在+credentialsSet+引擎匹配校验（phpmyadmin→mysql/mariadb；mongo_express→mongodb）
  InstanceReadyForTool(ctx, instanceID int64) (ReadyInfo, error)
}
ToolCreds { Username, Password, Host string; Port int; Engine string }
ReadyInfo { Ready bool; Reason string; Engine string }
// db-mgmt Service 实现，main 装配注入；契约权威定义见 api-database-mgmt.md §4.9/§5.1（v2.4 同轮登记）
```

**site-mgmt ConfigWritePipeline OHS 消费（D-03，已登记 api-site-mgmt.md v2.8）**：

```
ConfigWritePipeline {
  Apply(ctx, file, newContent string, allowNew bool) error   // R-06 四步：备份→写入→nginx -t→失败回滚（pipeline.go:21 先例）
  Remove(ctx, file string) error                             // 撤除恢复原字节
}
// site-mgmt Service 提供（泛化管线 OHS），main 装配注入；本模块组合注入内容（location 模板）
```

本机资源侧网关契约（进程内适配层，非 SQL）：

```
ToolDeployGateway {
  DownloadTarball(ctx, url, mirror string, timeout) (localPath string, err error)   // Go net/http，D-05
  VerifySha256(localPath, expected string) (ok bool, err error)                     // 逐字节校验，失败=任务 failed
  ExtractTar(localPath, destDir string) (err error)                                 // 参数数组 R-08，防 zip slip
  NpmInstall(ctx, pkg, version, registry string, timeout) (outputTail string, err error)  // npm install --global pkg@version
  NpmRemove(ctx, pkg string, timeout) (outputTail string, err error)
  PhpExtensions(ctx) (exts []string, err error)                                     // php -m（D-06）
  WriteSecretFile(path string, content []byte) error                                // 0600 原子写（nginxx gateway.go:183 同构）
}
SysdGateway（复用 service-mgmt 适配器）{ UnitState / Start / Stop / Restart / Enable / Disable / ListUnits / DaemonReload }
// DaemonReload 为新增扩展（dbus Manager.Reload / systemctl daemon-reload 参数数组）；单元文件写入/移除经文件通道（WriteSecretFile 0600 / RemoveFile），非 dbus 方法（systemd 无创建单元文件 dbus 契约）；既有方法零改动
PhpFpmGateway（复用 site-mgmt 适配器）{ ListPools() }   // pool 实况（D-06 默认 pool 复用，phpfpm gateway.go:54 先例）
约束：全部命令字面量参数数组（R-08）；超时 DEPLOY_OP_TIMEOUT/DOWNLOAD_TIMEOUT；全注入化（CI 预防）
```

### 4.2 tool_deploy_task（聚合根，主数据类子形态）

#### 4.2.1 领域模型

**表结构组合**：tool_deploy_task 单表（任务台账；与 db_tool 同域双聚合，第 2 章）。

**字段清单**：

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | 聚合根标识（taskId） |
| tool_type | TEXT | NOT NULL CHECK(in phpmyadmin,mongo_express) | 目标工具 |
| action | TEXT | NOT NULL CHECK(in install,upgrade,uninstall) | db_tool_task_action |
| status | TEXT | NOT NULL DEFAULT 'pending' CHECK(in pending,running,succeeded,failed) | db_tool_task_status（四态机 D-04） |
| instance_id | INTEGER | NULL | 安装时目标实例（mongo_express install；审计/排障溯源） |
| output_tail | TEXT | NOT NULL DEFAULT '' | 命令输出尾部（ring 采集 ≤200 行，running 追加/终态定格） |
| error | TEXT | NULL | 失败原因（failed；sha256_mismatch/下载超时/npm 失败/落点冲突/面板重启中断等分类） |
| requested_by | TEXT | NOT NULL | 发起者用户名（admin） |
| created_at / updated_at | DATETIME | NOT NULL | 发起 / 状态更新时间 |
| started_at / finished_at | DATETIME | NULL | 执行开始 / 终态时间 |

**不变量**（聚合内）：

| 编号 | 不变量 | 违反处理 |
|------|--------|---------|
| I-06 | 同工具在途唯一：同 tool_type 至多一个 pending/running 任务 | 创建事务内判定（DBT_003，data.taskId） |
| I-07 | 终态不可逆：succeeded/failed 不再迁移；重新发起=新任务行 | 写侧编排（D-04） |
| I-08 | 任务创建与发起审计同事务；终态落库与终态审计+档案登记同事务（I-C01） | D-04 同事务 |
| I-09 | 重启恢复：启动扫描 running 任务标 failed（error=面板重启中断），不续跑 | main 装配启动环节 |
| I-10 | 输出尾部上限 ≤TASK_OUTPUT_TAIL_LINES 行 | 采集侧保证 |

**领域服务**：DeployWorker（D-04 执行链，与 db_tool 共用）、TaskRecovery（I-09 启动扫描）。

#### 4.2.2 状态模型

db_tool_task_status 四态状态机（⚠️A3）：

| 当前状态 | 目标状态 | 触发 | 接口 |
|---------|---------|------|------|
| pending | running | worker 取出任务开始执行 | install/upgrade/uninstall 创建后异步 |
| running | succeeded | 部署命令链全部成功 | worker 终态回写（+档案登记 I-C01） |
| running | failed | 任一步骤失败（sha256/下载/npm/注入/服务/超时） | worker 终态回写（error+输出定格） |
| running | failed | 面板重启恢复 | 启动扫描（I-09） |

无审批/驳回环节；状态变迁历史经任务台账本体+审计（started/finished 两段）。

#### 4.2.3 子流程设计

无（任务状态机为聚合根本体，与 env_install_task/backup_run 同构）。

#### 4.2.4 接口设计

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /dbtool/task/list | GET | admin+readonly | 任务台账（toolType/status 筛选，分页） |
| /dbtool/task/detail | GET | admin+readonly | 单任务详情（输出尾部排障） |

**接口详情**：

- **GET /dbtool/task/list**：query `{toolType?(db_tool_type), status?(db_tool_task_status), page?, pageSize?(默认 50)}`。响应 data=`{items:[{id, toolType, action, status, instanceId: number|null, requestedBy, createdAt, startedAt: string|null, finishedAt: string|null, error: string|null}], total}`（created_at 倒序；可空字段显式 null）。审计：—（查看类）。
- **GET /dbtool/task/detail**：query `{taskId}`。校验：任务存在（DBT_005）。响应 data=`{...list 项全字段, outputTail: string}`。审计：—（查看类）。

#### 4.2.5 领域事件与集成

无 MQ。仓储契约：`ToolTaskRepository { Create(tx), GetByID, List(筛选+分页), UpdateStatus(tx), AppendOutputTail, Finish(tx)(status/error 同事务+档案编排), FailRunningAll(tx) }`。

### 4.3 工具条目（域内对象-实体，目录+实况投影链路）

#### 4.3.1 领域模型

**表结构组合**：无表（零工具台账 D2 判定——目录为编译期常量 D-13，运行态/依赖就绪度实时探测 D-02；部署状态=db_tool 档案存在性投影）。

**投影条目字段清单**：见 4.1.4 tool/list 响应结构。

**不变量**：

| 编号 | 不变量 | 违反处理 |
|------|--------|---------|
| I-11 | 投影真实性：tool/list 恒为当次探测快照，无缓存层；generatedAt 标注；探测失败字段 null | 读取侧保证 |
| I-12 | 零持久化：工具条目不落库；变更历史经任务台账+审计 | 结构保证（无表） |
| I-13 | 目录常量：两条目与映射编译期内置，运行期只读（⚠️A1）；无用户配置入口 | 编译期保证 |

#### 4.3.2 状态模型

面板侧无自有状态机（I-12 零持久化投影）；运行态为 systemd/站点实况投影；部署状态为档案存在性（prd-db-tools.md 第六部分辨析）。

#### 4.3.3 子流程设计

无。

#### 4.3.4 接口设计

工具条目无独立写接口（写动作作用于 db_tool/tool_deploy_task 聚合）；读取经 tool/list（4.1.4）。**跨模块读契约（5.1）**：ToolStatusForEngine(engine) 供 database-mgmt 高级管理入口消费。

#### 4.3.5 领域事件与集成

无。仓储契约 SQLite 侧无（零台账）；本机资源侧网关见 4.1.5。

### 4.4 工具安全访问内部端点（db_tool 聚合领域服务机制，D-09/D-11）

> 本节为安全架构核心（BA 2.4 防线二/三的应用层落点）：gating 与 signon 内部端点 + 令牌规约 + nginx 配置规约。内部端点非用户面（无权限矩阵行，3.3 已标注），不挂会话/CSRF 中间件。

#### 4.4.1 GET /dbtool/gate/verify（nginx auth_request 子请求专用）

**调用方**：nginx 工具 location 的 auth_request 子请求（internal location proxy_pass，携面板会话 cookie 原样透传 + X-Dbtool-Internal 秘密头）。

**防护链**：LoopbackGuard（RemoteAddr ∈ 127.0.0.1/::1，否则裸 404）→ InternalSecretGuard（X-Dbtool-Internal 比对 {dataDir}/tools/internal-secret，否则裸 404）→ GateService。

**处理**：从 Cookie 提取 dg_session → 消费 account-auth 只读会话校验 OHS（VerifyWs 语义 session.go:59：I-11 判定 revoked/expired，**只读不续期**——避免工具静态资源逐请求滑动续期写放大，D-11）→ 角色判定 role==admin（服务端权威，不信任客户端声明）。

**响应**：**裸 HTTP 状态**（auth_request 语义，无包络）——会话有效+admin → **200**（放行反代）；无会话/过期/吊销/readonly → **401**（拒绝；nginx 返回 401 给浏览器）。

**安全口径**：纯读校验无状态变更（CSRF 无攻击面）；会话吊销/改密级联即时切断（逐请求实时校验，无独立长时会话）；逐请求不落业务审计（⚠️A11）；拒绝事件 slog.Debug 安全日志（不落 audit_log）。

#### 4.4.2 POST /dbtool/signon/verify（PHP-FPM SignonScript 服务端回调专用）

**调用方**：phpMyAdmin SignonScript（PHP 服务端脚本，经 loopback HTTP 直连面板后端，**不经浏览器/nginx 工具路径**）。

**防护链**：LoopbackGuard → InternalSecretGuard（SignonScript 携 X-Dbtool-Internal，安装时写入 config.inc.php 模板）→ SignonService。

**req**：`{token: string}`（access/open 签发的令牌，phpMyAdmin 经 SignonURL 重定向携带→SignonScript 提取）。

**处理（四重校验，D-09）**：①解析 payload+HMAC-SHA256 签名比对（signon 密钥从 db_tool.cred_cipher 解密，仅服务端内存态）→ 失败 DBT_007；②exp 时效（iat+SIGNON_TOKEN_TTL）→ 过期 DBT_008；③jti 消费台账判定（已消费→DBT_010；未消费→标记 consumed，**校验通过即消费，一次性**）；④instanceId 绑定+sessionDigest 匹配（令牌 payload.sessionDigest 与当前请求会话标识比对——**SignonScript 从浏览器工具路径请求读 `$_COOKIE['dg_session']` 并随 loopback 回调转发给面板后端供绑定校验**〔v1.2 W4 显式化：工具路径在面板域下 cookie scope 覆盖，SignonScript 运行于浏览器请求上下文（PHP-FPM 经 nginx 工具 location 接收浏览器请求），故能读到面板会话 cookie；转发的是会话标识校验通道，与「凭据响应不经浏览器」不矛盾——后者指 {user,password} 凭据响应仅走 loopback 服务端通道，浏览器全程只见令牌与会话 cookie、不见凭据〕；跨实例/跨会话→DBT_007）→ 消费 ToolCredentialGateway.InstanceCredentialsForTool(instanceId)（D-10，vault 取出 MySQL 凭据）。

**响应**（包络，**仅 loopback 服务端通道**）：成功 data=`{user: string, password: string}`（SignonScript 设 $_SESSION['PMA_single_signon_user']/['password']，**凭据永不进浏览器**——浏览器全程只见令牌不见凭据，I-02）；失败 code=DBT_007/008/010，data=null。

**安全口径**：凭据明文仅此次 loopback 响应（服务端→服务端），不落库不入日志不入审计；令牌明文不入审计（dbt_tool_accessed 在 access/open 侧已记）；校验失败 slog.Warn 安全日志（含 jti 摘要，不含令牌/凭据）；面板重启令牌台账清空=全部令牌失效（安全保守）。

#### 4.4.3 signon 令牌规约（D-09 汇总）

| 项 | 规约 |
|----|------|
| payload | `{jti: 随机16B hex, toolType: "phpmyadmin", instanceId: number, sessionDigest: string(签发时 Identity.SessionDigest), iat: unix秒, exp: iat+60}` |
| 签名 | HMAC-SHA256(payload JSON 规范化序列化, signonKey)；signonKey=crypto/rand 32B，密文落 db_tool.cred_cipher |
| 令牌形态 | base64url(payload JSON) + "." + base64url(sig)（URL 安全，附于工具 URL ?dg_signon= 片段） |
| 时效 | SIGNON_TOKEN_TTL=60s（exp 校验，服务端时钟权威） |
| 一次性 | 内存 map[jti]consumed + TTL 清扫（TOKEN_SWEEP_INTERVAL=30s，goroutine 入 wg）；消费即标记，重放 DBT_010 |
| 绑定 | instanceId（跨实例拒绝）+ sessionDigest（跨会话拒绝，防令牌窃取重用） |
| 防伪 | HMAC 签名（篡改 DBT_007）；密钥 vault 密文存储（dbtool-cred.key 0600） |
| 失效 | 面板重启全部失效（内存态，安全保守）；工具卸载密钥删除（令牌永久不可校验） |

#### 4.4.4 nginx 配置规约（D-03 注入模板）

**工具 location（phpMyAdmin）**：`location ^~ /dbtool/phpmyadmin/ { auth_request /dbtool-gate-internal; include {TOOL_DIR}/phpmyadmin-fastcgi.conf; fastcgi_pass {pool监听}; fastcgi_param SCRIPT_FILENAME {TOOL_DIR}/phpmyadmin$fastcgi_script_name; ... }`（FastCGI 参数模板内置，site-mgmt php_fpm 站点模板先例）。

**工具 location（mongo-express）**：`location ^~ /dbtool/mongo-express/ { auth_request /dbtool-gate-internal; include {TOOL_DIR}/mongo-express-auth.conf; proxy_pass http://127.0.0.1:8081/dbtool/mongo-express/; proxy_set_header Host $host; ... }`（auth.conf=basic auth Authorization 注入秘密文件 0600，D-08）。

**内部 gate location**：`location = /dbtool-gate-internal { internal; proxy_pass http://127.0.0.1:{panelPort}{entryPath}/dbtool/gate/verify; proxy_set_header X-Dbtool-Internal {secret}; proxy_pass_request_body off; proxy_set_header Content-Length ""; }`（internal 指令外部请求 404；cookie 原样透传供会话校验）。

**注入/撤除**：经 ConfigWritePipeline OHS（R-06 四步，D-03）；落点=面板域 server 块（bind_domain 匹配优先/面板反代站点块）；卸载撤除恢复原字节；秘密文件（internal-secret/mongo-express-auth.conf/phpmyadmin config.inc.php）经 WriteSecretFile 0600。

## 5. 模块清单汇总

### 5.1 对外服务清单

**HTTP 接口（面板面 9 + 内部 2，v1.2 +access/reopen）**：

| 服务（接口） | 消费方 | 契约要点 |
|-------------|--------|---------|
| GET /dbtool/tool/list | 前端工具管理页（两角色） | 目录+实况+依赖就绪度实时投影（D-02/I-11）；在途标记 |
| POST /dbtool/tool/install · upgrade · uninstall | 前端工具管理页（admin） | 异步任务（D-04）；sha256（D-05）；依赖前置 DBT_001；互斥 DBT_003；卸载数据保全 D-15 |
| GET /dbtool/task/list · detail | 前端工具管理页（两角色） | 任务台账与输出尾部；ADR-018 全可看 |
| POST /dbtool/access/open | 前端工具管理页 + database-mgmt 高级入口（admin） | signon 签发/gating 直达分发（D-09）；访问开启审计；凭据永不下发 |
| GET /dbtool/access/reopen | phpMyAdmin SignonURL 302 GET 重定向（admin 浏览器顶层导航，v1.2 W3） | SignonURL 重认证落地载体；admin 会话 gating（GET 无 CSRF）；重签令牌 302 回工具 URL（D-06/D-09）；访问开启审计；凭据永不下发 |
| POST /dbtool/access/rotate | 前端工具管理页（admin） | basic auth 轮换（D-08，仅 mongo_express）；零回显 |
| GET /dbtool/gate/verify | nginx auth_request（内部） | 裸 200/401；loopback+秘密头；只读会话校验+admin（D-11） |
| POST /dbtool/signon/verify | phpMyAdmin SignonScript（内部 loopback） | 令牌四重校验；凭据 loopback 注入响应（D-09/D-11）；永不进浏览器 |

**进程内 OHS（提供）**：

| 服务 | 消费方 | 契约要点 |
|------|--------|---------|
| ToolStatusForEngine(ctx, engine string) → ToolStatusInfo{ToolType, Installed, Running, Openable, BoundInstanceId} | database-mgmt 高级管理入口（读契约） | 按引擎映射工具（mysql/mariadb→phpmyadmin；mongodb→mongo_express）；部署状态+运行态+打开可用性+绑定实例（v1.1 +BoundInstanceId，高级入口绑定校验）；**入口渲染归 database-mgmt PageSpec（v1.5 已兑现）** |
| 打开能力共用（access/open） | database-mgmt 高级入口 | 高级入口点击→前端调 access/open（同契约）；无独立 OHS |

**进程内 OHS（消费）**：

| 服务 | 提供方 | 契约要点 |
|------|--------|---------|
| ToolCredentialGateway（InstanceCredentialsForTool/InstanceReadyForTool） | database-mgmt | vault 凭据取出仅服务端（D-10）；**已登记 api-database-mgmt.md §4.9/§5.1（v2.4 同轮联动）** |
| ConfigWritePipeline（Apply/Remove） | site-mgmt | R-06 四步管线泛化（D-03）；**已登记 api-site-mgmt.md v2.8（D-21/§5.1）** |
| 只读会话校验（VerifyWs 语义） | account-auth | gating 消费（D-11，session.go:59）；**OHS 消费已登记 api-account-auth.md v1.2（无新 HTTP 接口）** |
| 环境实况读契约 | env-mgmt | 依赖就绪度（PHP-FPM/Node/nginx）；**env-mgmt api Node 增量待同步项（D-18）** |
| SysdGateway / PhpFpmGateway / nginxx | service-mgmt / site-mgmt | 共享适配层（非业务域集成） |
| AuditService.Record | account-auth | R-03 同事务脱敏（D-16） |

### 5.2 SQL 清单（SQLite 侧，全部参数绑定）

| # | SQL | 场景 |
|---|-----|------|
| 1 | INSERT INTO tool_deploy_task(tool_type, action, status, instance_id, requested_by, created_at, updated_at) VALUES(...) | 任务创建（pending，同事务发起审计） |
| 2 | SELECT * FROM tool_deploy_task WHERE tool_type=? AND status IN ('pending','running') | 互斥判定（I-06，创建事务内） |
| 3 | SELECT * FROM tool_deploy_task WHERE id=? | 任务详情 |
| 4 | SELECT * FROM tool_deploy_task WHERE [tool_type=?] [AND status=?] ORDER BY created_at DESC LIMIT ? OFFSET ? | 任务列表 |
| 5 | UPDATE tool_deploy_task SET status='running', started_at=?, updated_at=? WHERE id=? | 执行开始 |
| 6 | UPDATE tool_deploy_task SET output_tail=?, updated_at=? WHERE id=? | 输出尾部追加/定格 |
| 7 | UPDATE tool_deploy_task SET status=?, error=?, finished_at=?, updated_at=? WHERE id=? | 终态回写（同事务档案编排 I-C01） |
| 8 | UPDATE tool_deploy_task SET status='failed', error='面板重启中断', finished_at=?, updated_at=? WHERE status='running' | 重启恢复（I-09） |
| 9 | SELECT tool_type, id FROM tool_deploy_task WHERE status IN ('pending','running') | 工具列表在途标记 |
| 10 | INSERT INTO db_tool(tool_type, installed_version, deploy_path, site_ref, unit_name, instance_id, cred_cipher, created_at, updated_at) VALUES(...) | 安装落档（install succeeded 同事务，I-C01） |
| 11 | SELECT * FROM db_tool WHERE tool_type=? | 档案读取（部署状态/凭据解密/落点撤除寻址） |
| 12 | UPDATE db_tool SET installed_version=?, updated_at=? WHERE tool_type=? | 升级换版（upgrade succeeded 同事务） |
| 13 | UPDATE db_tool SET cred_cipher=?, updated_at=? WHERE tool_type=? | 凭据轮换（rotate 成功后同事务审计） |
| 14 | DELETE FROM db_tool WHERE tool_type=? | 卸载除档（uninstall succeeded 同事务） |
| 15 | SELECT * FROM db_tool | 工具列表档案存在性投影 |

### 5.3 DDL

```sql
CREATE TABLE IF NOT EXISTS db_tool (
  id                INTEGER PRIMARY KEY AUTOINCREMENT,
  tool_type         TEXT    NOT NULL UNIQUE CHECK (tool_type IN ('phpmyadmin','mongo_express')),
  installed_version TEXT    NOT NULL,
  deploy_path       TEXT    NOT NULL,
  site_ref          TEXT    NOT NULL,                  -- 注入落点标识（来源文件+server 块定位，D-03）
  unit_name         TEXT,                              -- mongo_express=dougong-mongo-express.service；phpmyadmin=NULL
  instance_id       INTEGER,                           -- mongo_express 目标实例引用（db_instance.id）；phpmyadmin=NULL
  cred_cipher       TEXT    NOT NULL,                  -- AES-256-GCM 密文（I-02 零回显；dbtool-cred.key 0600）
  created_at        DATETIME NOT NULL,
  updated_at        DATETIME NOT NULL
);
CREATE TABLE IF NOT EXISTS tool_deploy_task (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  tool_type    TEXT    NOT NULL CHECK (tool_type IN ('phpmyadmin','mongo_express')),
  action       TEXT    NOT NULL CHECK (action IN ('install','upgrade','uninstall')),
  status       TEXT    NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','succeeded','failed')),
  instance_id  INTEGER,                               -- 安装时目标实例（溯源）
  output_tail  TEXT    NOT NULL DEFAULT '',           -- ≤200 行定格
  error        TEXT,
  requested_by TEXT    NOT NULL,
  created_at   DATETIME NOT NULL,
  updated_at   DATETIME NOT NULL,
  started_at   DATETIME,
  finished_at  DATETIME
);
CREATE INDEX IF NOT EXISTS idx_dbtool_task_type_status ON tool_deploy_task(tool_type, status);
CREATE INDEX IF NOT EXISTS idx_dbtool_task_created ON tool_deploy_task(created_at);
```

> 迁移：**013_db_tools.sql**（001~012 已占用，012=env_bootstrap；013 为下一空号，backend/internal/store/migrations/ 实存核验）——两表初建+两索引；db_tool.tool_type UNIQUE 承载一工具一档案（I-01）；tool_deploy_task tool_type+status 组合索引承载互斥查询（I-06）；无既有数据迁移（新模块）。凭据密钥文件 dbtool-cred.key（dataDir，0600，首次初始化 crypto/rand 32B，沿 db-cred.key 先例 vault.go:14-24）；internal-secret/mongo-express-auth.conf 秘密文件安装时生成（0600）。

## 6. 更新记录

### 6.1 变更记录

| 版本 | 日期 | 变更类型 | 变更摘要 | 目标节号 |
|------|------|---------|---------|---------|
| v1.0 | 2026-11-19 | initial | 首次生成：10 端点（面板面 8：GET 3 两角色〔tool/list、task/list、task/detail〕+ POST 5 admin+CSRF〔install/upgrade/uninstall/access/open/access/rotate〕；内部 2：gate/verify〔裸 200/401，auth_request〕+ signon/verify〔令牌四重校验+凭据 loopback 注入〕）+ 2 表 DDL（db_tool 部署档案+凭据密文 UNIQUE tool_type / tool_deploy_task 四态台账，迁移 013）+ 12 错误码（DBT_001~012 + 复用 AUTH 族）+ 3 枚举引用（D2 定稿入池）+ 6 审计动作码（dbt_*，detail 脱敏）+ 18 设计决策（D-01 资源段 dbtool/D-02 实时投影/D-03 面板域 location 注入〔关闭待对齐-3〕/D-04 异步互斥/D-05 sha256 强制/D-06 phpMyAdmin 形态/D-07 mongo-express 形态/D-08 basic auth 注入轮换/D-09 signon 令牌规约〔HMAC+60s+一次性+绑定〕/D-10 凭据 OHS/D-11 内部端点三重防护〔关闭待对齐-2，account-auth 无新 HTTP 接口〕/D-12 DBT 码族/D-13 编译期常量〔关闭待对齐-4 部分，版本 sha256 落码取证⚠️〕/D-14 升级/D-15 卸载数据保全/D-16 审计码/D-17 混合模式/D-18 Node 复用 env-mgmt）+ 跨聚合根 I-C01~C03 + 聚合内 I-01~I-13 + 网关契约（ToolDeployGateway/ToolCredentialGateway/ConfigWritePipeline/SysdGateway 扩展 DaemonReload〔单元文件经文件通道非 dbus〕）+ 跨模块同轮提供方登记 3 完成（db-mgmt v2.4 凭据 OHS §4.9/site-mgmt v2.8 管线 OHS D-21/account-auth v1.2 会话校验消费，沿 v2.2 先例）+ 待同步项 2（env-mgmt Node api 增量〔env_type 八值/DDL CHECK 迁移，其 D3 轮〕/database-mgmt 高级入口 PageSpec 渲染〔其 D4 轮〕）；关闭 prd-db-tools 待对齐-1（DDL/常量定稿）/-2（内部端点形态）/-3（路径形态）/-4（常量表，版本哈希落码取证）；待对齐-5（凭据刷新时机）保守口径=升级/重装刷新维持待 D3 后续轮裁定、待对齐-6 归 D4；无旧版对比 | — |
| v1.1 | 2026-11-19 | interface_changed | D4 高级入口绑定校验联动（database-mgmt PageSpec v1.5 消费需求）：tool/list 响应 items +boundInstanceId: number|null（mongo_express=部署绑定实例 db_tool.instance_id；phpmyadmin=null 打开时绑定）——高级入口判定工具绑定实例与当前治理实例一致性（PageSpec 6.8 步骤 4）；5.1 ToolStatusForEngine 读契约 +BoundInstanceId；**零新增接口/零 DDL 变更**（db_tool.instance_id 既有列投影，additive 响应字段）；database-mgmt PageSpec 待同步项就此兑现（v1.5） | 4.1.4（tool/list）/ 5.1 |
| v1.2 | 2026-11-19 | interface_added | Gate 1 WARN 修复轮（W3/W4，gate1-db-tools-2026-11-19.md）：**W3 +GET /dbtool/access/reopen**（phpMyAdmin SignonURL 重认证 GET 落地载体——admin 会话 gating〔AuthRequired+RoleRequired admin〕，GET 重定向流程无 CSRF，query instanceId，重签 signon 令牌后 302 回工具 URL 附新令牌片段；承载工具内 PMA 会话过期后的 302 GET 重认证二次流程，主打开流程 access/open 不变）；面板面 8→9、总端点 10→11（内部 2 不变）；D-01 access 资源段 +reopen / D-06 SignonURL 改指向 reopen（instanceId 经非敏感 dg_tool_instance cookie 承载，config.inc.php 经 PHP 动态拼接 $_COOKIE）/ D-09 +重认证流程；3.3 矩阵 +reopen 行 / 3.1 用户类型标注+CSRF GET 豁免安全口径 / 4.1.4 接口清单+详情 bullet+跨接口复用+幂等 / 常量 +SIGNON_REOPEN_PATH·TOOL_INSTANCE_COOKIE / 5.1 +reopen 行；**零 DDL 变更**（无新表/列，令牌消费台账仍内存态）；admin PageSpec Action +1（dbtoolReopen，S7 9/9）+readonly 负空间 +dbtoolReopen（admin 专属 ⚠️A10）。**W4 4.4.2④ sessionDigest 绑定校验显式化**：SignonScript 从浏览器工具路径请求读 $_COOKIE['dg_session'] 并随 loopback 回调转发（会话标识校验来源；与「凭据响应不经浏览器」不矛盾）——rule_changed，安全约束本身不削弱。 | 3.1/3.3/3.4/4.1.4/4.4.2/5.1（D-01/D-06/D-09） |

### 6.2 变更类型词汇表

| 变更类型 | 含义 | 需要目标节号 |
|---------|------|------------|
| `initial` | 首次生成 | 否 |
| `structural` | 章节增删/重组 | 否 |
| `table_added` / `table_changed` | 新增/变更数据表 | 是 |
| `field_added` / `field_changed` / `field_removed` | 字段增删改 | 是 |
| `enum_added` / `enum_value_added` / `enum_value_changed` | 枚举增删改 | 是 |
| `interface_added` / `interface_changed` / `interface_removed` | 接口增删改 | 是 |
| `state_added` / `state_changed` / `flow_added` / `flow_changed` | 状态/流转 | 是 |
| `rule_added` / `rule_changed` | 规则增删改 | 是 |
| `event_added` / `event_changed` | 领域事件 | 是 |

### 6.3 版本号规则

语义化版本：vMAJOR.MINOR——结构/接口增删改升 MAJOR；字段/枚举/规则微调升 MINOR；本文档随模块迭代在 6.1 追加行。
