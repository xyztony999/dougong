# 更新日志 / Changelog

发布说明按版本倒序排列；每个版本的完整说明也会作为 GitHub Release 正文发布。

## v0.1.13 - 2026-11-19

修复 MongoDB 浏览系统集合报「参数错误」（v0.1.12 表级/文档治理增量实机热修）。

### 变更内容

**修复：MongoDB 文档浏览系统集合被误拦（AUTH_012）**
- 痛点：集合清单会列出 MongoDB 系统集合（`system.profile` / `system.js` 等），点「浏览文档」报 `AUTH_012 参数错误：集合名须非空且不含 …/system. 保留前缀`
- 根因：浏览/治理已存在集合的共用校验路径误用了**建集合专属**的 `system.` 保留前缀禁令——该禁令本意是「用户不可创建系统集合」，不该限制浏览真实存在的系统集合
- 修复：拆分校验——通用形态校验（非空/禁 `$` 前缀/禁空字符/长度上限，任何操作均禁）与建集合专属 `system.` 禁令分离；访问已存在集合（含存在性核验）只走形态校验，**系统集合可正常浏览**；建集合仍禁 `system.` 前缀（创建约束不变）
- 回归测试：浏览 `system.profile` 放行 + 形态校验仍拒空名/`$`/空字符 + 建集合仍拒 `system.`

### 安装与升级

```bash
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash
dougong version   # 应为 v0.1.13
```

升级后验证：MongoDB 库 → 浏览集合 → 点系统集合（如 `system.profile`）「浏览文档」不再报参数错误，可正常分页查看。

## v0.1.12 - 2026-11-18

数据库管理**体验与表级治理**大增量：实例清单不再被冗余连接困扰，MySQL 能管到表与行，MongoDB 不再只读，浏览有面包屑返回导航（api-database-mgmt v2.3.1，接口 30→44）。

### 变更内容

**改进：实例清单 TCP 优先纳管（实机反馈「为什么有好几个连接」）**
- 痛点：同一台本机 MySQL 的 socket 端点与 TCP 端点各登记一行，平铺呈现令人困惑（宝塔等面板均走 TCP，socket 管理极少见）
- 修复：探测改为 **TCP 优先两阶段**——本机 TCP 可达时 socket 不再登记为独立实例；仅当实例关闭 TCP（`skip-networking`）时 socket 才兜底发现（不丢发现能力）。清单常见情况天然一行 TCP
- 存量清理：重新探测时保守清理**无凭据**的冗余 socket 行（审计留痕）；**已配凭据**的 socket 行保留并标注，引导改配 TCP 后手动移除（不自动迁移凭据，避免丢失）

**改进：连接状态/凭据标签语义可见（实机反馈「绿条是什么」）**
- 「连接状态」绿/橙标签与「凭据」标签加悬停提示与图例：绿=可达（面板能连上）、橙=不可达；仅反映连通性，不代表凭据正确

**新增：MySQL/MariaDB 表级治理（实机反馈「为什么不能管理库里的表」）**
- 数据库行新增「浏览」入口 → **表清单**（表名/引擎/行数/数据大小/排序规则/注释）→ **表详情**（分页数据浏览 + 结构两 Tab）
- 行级治理：**单列主键**表支持行编辑/删除（无单列主键的表禁用并如实提示原因）；删表不可逆强确认
- 数据浏览经 JSON 逐行聚合通道，含换行/制表符的值不再解析脆弱

**新增：MongoDB 库/集合/文档治理（实机反馈「为什么只能只读」，废止 D-18 只读投影，ADR-031）**
- 数据库行「浏览集合」「删除库」（系统库保护）→ **集合清单**（新建集合/浏览文档/删除集合）→ **文档浏览**（JSON 分页 + 插入/编辑/删除）
- MongoDB 库随首集合写入产生（隐式建库语义），界面如实说明

**新增：顶栏面包屑返回导航（实机反馈「点浏览后怎么回去」）**
- 数据库 → 治理·实例标识 → 库 → 表/集合，逐级可点返回；动态段取实例档案与路由参数

**角色口径（沿 Redis key 浏览先例）**
- 表/集合/结构等**元数据**两角色（admin/readonly）均可看；**行数据/文档内容**与全部写操作 **admin 专属**，readonly 见「数据内容仅管理员可见」负空间

**修复：DBM_029 跨层错误码同步（A-DB13）**
- 引擎操作被拒（凭据账户权限不足）错误码后端/前端双侧对齐，Mongo 治理接口此类错误不再被静默吞掉；两个 Mongo 弹窗判定分支同步订正

### 安装与升级

```bash
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash
dougong version   # 应为 v0.1.12
```

升级后验证（数据库管理）：①实例清单同一本机服务仅一行 TCP（重新探测可清理冗余 socket 行）；②MySQL 库行「浏览」可见表清单/表数据/结构，单列主键表可编辑/删除行；③MongoDB 库行可浏览集合/文档并增删改、可删库；④进入治理/浏览页顶栏面包屑可逐级返回。已有实例与凭据不受影响（探测策略仅影响新登记与冗余清理，已配凭据 socket 行保留）。

## v0.1.11 - 2026-11-17

环境安装后**配置就绪**：装完数据库环境即可在面板直接治理，不再「装完 ≠ 可用」。

### 变更内容

**新增：环境安装后自动引导配置（env-mgmt + database-mgmt 联动，api-env-mgmt v1.2 / api-database-mgmt v2.2）**
- 痛点：经环境管理 apt 装完 MySQL 只有 root@localhost（auth_socket 无密码），面板走 TCP+密码连不上，用户无法自助建第一个账户（DBM_005 诊断轮终裁）
- 修复：MySQL/Redis/MongoDB 安装/升级成功后，worker 自动执行引导——建管理账户/设随机强密码并注册实例与凭据到数据库管理（**装完即可管**）：
  - **MySQL**：经 unix socket 以 root auth_socket 免密建 `dougong@localhost`（ALL PRIVILEGES WITH GRANT OPTION，**不动 root**）
  - **Redis**：CONFIG SET requirepass 随机密码 + CONFIG REWRITE 持久化（仅未设置时写）+ bind 本机校验
  - **MongoDB**：localhost exception 建 admin 库 root 角色用户
- 安全：随机强密码（crypto/rand）AES-256-GCM 密文落库、**永不回显**；账户限本机、bind 本机不公网暴露；引导动作审计留痕
- 幂等保守：已配置凭据/账户已存在/requirepass 已设/已有用户 → 跳过不覆盖用户已有配置
- 引导结果独立呈现：任务状态仍为包安装结果，新增引导结果维度（任务列表「引导」列 + 任务详情横幅）；**安装成功但引导失败**如实标注「已安装·引导失败」+ 原因 + 「重新安装/升级以重试引导」指引（不混淆「已装」与「可治理」）
- PHP/nginx/Apache/Docker 装完即用无引导

**随附（本批攒批，详见下方草稿节）**
- DBM_005 凭据弹窗 UX 小修：DBM_005/006 内联服务器原文 reason + 按引擎族排查指引 + 文案订正（database-mgmt v2.1，前端域内）
- ACME v2.4 链路修复：HTTP-01 webroot 迁出 0700 数据目录（v0.1.7）→ 挑战注入/复原后 reload nginx（v0.1.8）→ 存量纳管跳过面板托管路径影子行（v0.1.9）
- **证书弹窗 UI 三项修复（实机反馈）**：①申请/部署/解除部署/替换/DNS 凭证五个弹窗右上角 X 点击无反应——onClose 未回写受控 show，五弹窗统一补回写；②「DNS 凭证管理」标题去掉冗余括注；③新建站点/站点详情/证书申请三处域名输入由 select tag 形态改为**动态标签**（消除下拉箭头与「无数据」空态噪音，匹配「输入域名后回车添加」交互）

### 安装与升级

```bash
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash
dougong version   # 应为 v0.1.11
```

升级后验证（配置就绪）：在环境管理装 MySQL/Redis/MongoDB 后，系统自动创建管理账户并注册实例到数据库管理——打开数据库页即可直接治理（无需手动建第一个账户）；若引导失败，任务详情独立呈现「已安装·引导失败」与原因，重新安装/升级可幂等重试。已有环境/已有配置不受影响（保守跳过不覆盖）。

## v0.1.10 - 2026-11-17

reload 对存量坏 pid 文件鲁棒化：`nginx -s reload` 因 pid 文件空/缺失失败时，回退解析 master PID 直接 SIGHUP。

### 变更内容

**修复：reload 回退通道（nginxx v2.7，D-05 补强）**
- 实机根因：存量 nginx 的 `/run/nginx.pid` 为**空文件**——`nginx -s reload` 报 `invalid PID number ""`，systemd `ExecReload`（`kill -HUP $(cat /run/nginx.pid)`）同败，而 master 实际存活 → 部署/续期写盘的新配置永不生效（表现为「证书还是老的」；用户被迫 `pkill -9` 重启才生效）
- 回退链：`nginx -s reload` 失败且输出为 pid 文件类错误 → `systemctl show nginx.service --property=MainPID` → 否则扫 `/proc` cmdline 的 `nginx: master process` → 经 Go `os.Process.Signal(SIGHUP)`（无 shell，R-08）；回退成功在输出注明 master pid；解析/发信号失败保留原错误（D-05 配置保留语义不变）
- 回归锁：TestIsPidFileErr / TestMasterPidFromProc（procfs 目录注入，跨平台可跑）
- 服务器侧仍建议根治：查明 pid 文件为何为空（`grep -n pid /etc/nginx/nginx.conf`、`cat /etc/systemd/system/nginx.service.d/override.conf`），必要时 `systemctl restart nginx` 一次让 master 重写

### 安装与升级

```bash
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash
dougong version   # 应为 v0.1.10
```

## v0.1.9 - 2026-11-17

修复证书「存量导入」影子行：ACME 证书部署到站点后，存量自动纳管把面板托管证书文件又导入为一行「存量导入」（与 ACME 行同到期日）。

### 变更内容

**修复：存量纳管跳过面板托管路径（api-site-mgmt v2.6，A-SM5/FR-25 判重补全）**
- 根因：`importExistingCerts` 判重集只含 imported 行的 OriginPath；部署使站点 `ssl_certificate` 指向面板托管路径（ACME 行 StoragePath），该路径不在判重集 → 每次扫描新增一行影子「存量导入」
- 修复：判重集并入**全部证书行的 StoragePath**（托管路径一律不纳管）；回归锁 TestImportSkipsManagedStoragePath
- 已产生的影子行：升级后在证书列表直接删除该行即可（imported 删除=仅解除管理关系，不触碰任何文件），不再复发

### 安装与升级

```bash
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash
dougong version   # 应为 v0.1.9
```

## v0.1.8 - 2026-11-17

修复 ACME HTTP-01 持续 404 的最终根因：挑战配置写盘后**未 reload nginx**（v0.1.4~v0.1.7 实机全部命中）。

### 变更内容

**修复：挑战注入/复原后 reload nginx（site-mgmt）**
- 根因：`applyPipeline` 按 D-05 设计不含 reload（由调用方呈现），而 HTTP-01 挑战注入路径从不调用——挑战 location 写进了配置文件，**运行中的 nginx 仍是旧配置**：80 端口裸 `return 301` 把 LE 带到无挑战入口的 443 → 每次尝试均 404（fake 网关测试无法暴露，故连逃四个版本）
- 修复：注入写回成功后 reload（失败=复原+报错，不留半注入态）；cleanup 复原后 reload（best-effort，D-05 日志呈现）；单文件/dormant 与 v2.3 多文件两条路径均覆盖
- 回归锁：E2E 断言 reload ≥2 次（注入+复原）
- 面板证书 ACME 通道本就带 reload（panel/acme.go），仅 site-mgmt 挑战路径缺失

### 安装与升级

```bash
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash
dougong version   # 应为 v0.1.8
```

升级后对 renew_failed 证书点「更新证书」重试。HTTP-01 至此五层全通：同文件多块注入+跳转包裹（v2.2）→ 跨文件+catch-all 兜底（v2.3）→ webroot 公共可读（v2.4）→ **reload 生效（v2.5/v0.1.8）**；存量 HTTPS 域名仍推荐 DNS-01 最稳。

## v0.1.7 - 2026-11-17

HTTP-01 签发链路第二层根因修复：挑战 webroot 迁出 0700 数据目录（v0.1.6 修好注入后仍会 403 的隐藏缺陷）。

### 变更内容

**修复：ACME HTTP-01 webroot 迁移至 /var/www/dougong-acme（api-site-mgmt v2.4，D-19）**
- 根因：webroot 原为 `{数据目录}/acme-webroot`，而数据目录权限 0700 root——nginx worker（www-data/nginx）不可穿越，挑战文件永远读不到（UNIX 权限 403；RHEL SELinux 下 var_t 上下文同样拒绝 httpd 读取）
- 迁移至 `/var/www/dougong-acme`：Debian/RHEL 标准 httpd 内容目录（SELinux httpd_sys_content_t 覆盖），目录 0755、挑战文件 0644；certbot webroot 插件同族路径
- 三处派生同源收敛（acme 网关 / 面板证书通道 / site-mgmt），账户密钥、写前备份、托管证书仍留 0700 数据目录——安全面零变化
- 与 v0.1.6 的跨文件/默认兜底块注入（v2.3）叠加后，HTTP-01 覆盖：单文件多块、跨文件 80/443、全局 catch-all 重定向、worker 可读性四层全通

### 安装与升级

```bash
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash   # 重复执行即升级
```

升级后对 renew_failed 证书点「更新证书」重试。**若仍失败**请附：`dougong version` 输出 + `nginx -T 2>/dev/null | grep -B2 -A18 "server_name <你的域名>"` 片段，用于定位剩余拓扑。

## v0.1.6 - 2026-11-17

修复存量 nginx 拓扑下 ACME HTTP-01 签发 404（v0.1.5 修复未覆盖的两类拓扑，二次实机反馈）。

### 变更内容

**修复：HTTP-01 挑战注入覆盖全部存量拓扑（api-site-mgmt v2.3，D-19）**
- 实机报错：`Invalid response from https://<域名>/.well-known/acme-challenge/…: 404`——v0.1.5 的多块注入只覆盖 siteId 所在**单个文件**；两类常见存量拓扑仍落空：①80 与 443 server 块**分属不同 conf 文件**；②域名仅 443 在服、http 首跳落在**全局 catch-all 重定向块**（`listen 80 default_server; server_name _; return 301 https://…`，server_name 不匹配域名故被 v0.1.5 跳过）
- v2.3：注入范围扩至**全扫描跨文件**的所有匹配 server_name 块；匹配块中无 80 端口时追加注入 **80 默认兜底块**（default_server 优先、否则扫描序首个 80 块——LE 首跳实际落点）并包裹其 server 级跳转；逐文件经 R-06 安全管线写回、cleanup 逆序逐文件字节级复原
- 回归锁：TestCertRequestHTTP01CrossFile 与 TestCertRequestHTTP01DefaultCatchAll（onIssue 钩子快照注入态 + 复原断言）

### 安装与升级

```bash
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash   # 重复执行即升级
```

升级后对 renew_failed 证书点「更新证书」重试，或重新「申请证书」。国内服务器见 README「国内服务器安装」三通道（镜像前缀/指定版本/本地二进制）。

## v0.1.5 - 2026-11-17

数据库管理 MongoDB 支持轮 + 两项 v0.1.4 实测反馈修复（环境页刷新 / 凭据验证 INTERNAL）。

### 变更内容

**修复：数据库实例凭据保存/验证返回「内部错误」（阻断级）**
- 根因（代码实证）：凭据验证失败时引擎侧错误未映射业务码——当宿主机没有 `mysql`/`redis-cli` 客户端（如数据库跑在 Docker 容器、仅映射端口）或客户端输出未被识别的报错时，错误一路透传被兜底为裸 `INTERNAL`。此前版本在实例列表看到的「凭据验证失败：认证被拒/连接失败」提示在这类场景下根本走不到
- 与密码字符无关：经查全部通道为 `MYSQL_PWD`/`REDISCLI_AUTH` 环境变量注入子进程，密码含 `@`/数字/空格均不受影响（此前用户反馈中的猜测已证伪并在测试中固化）
- 修复：新增错误码 **DBM_021（引擎客户端通道不可用）**——明确提示「宿主机未安装 mysql/redis-cli」，与「网络不通」区分；凭据验证语境下未分类失败统一归 DBM_006；凭据弹窗内联呈现两类提示
- 附带修复：TCP 连接模式显式 `--protocol=TCP`——mysql 客户端对 `-h localhost` 会特例改走本机 socket 并忽略端口，容器/远端 TCP 实例此前连错通道

**修复：环境页任务完成后状态不刷新**
- 任务详情轮询到终态（成功/失败）即停止，但环境卡片仍显示旧的「已安装/版本/服务状态/任务进行中」。现任务一到终态自动重拉环境全景与任务列表；轮询纪律不变（执行中 5 秒、无后台长连接）

**新增：数据库管理支持 MongoDB**
- 实例发现：mongod 默认 socket（`/tmp/mongodb-27017.sock`）与 `localhost:27017` 端口探测，随实例清单统一纳管（凭据弹窗默认端口 27017）
- 凭据连接：SCRAM 认证（admin 库）；连接通道为进程内官方 mongo-driver（新增 Go 依赖，纯 Go 静态链接——**不依赖宿主机安装 mongosh**，从根上避开本轮 DBM_021 修复的「客户端缺席」缺陷类）
- 库列表：名称/大小/集合数只读投影（MongoDB 为隐式建库语义，不提供建/删库——与 MySQL 面的差异如实裁剪）
- 用户管理：创建（用户名/密码/首库首角色）/删除（连带回收全部角色）/库级角色三档（只读 read / 读写 readWrite / 库管理 dbAdmin）授予与回收
- 状态概览：serverStatus 白名单摘要（版本/运行时长/连接/内存/增删改查计数）
- 新增错误码 **DBM_022（引擎操作被拒）**：凭据账户权限不足（如查看 serverStatus 需 clusterMonitor 类权限）时如实提示，不再裸「内部错误」
- 集合/文档级浏览、mongodump 备份导出导入为二期（后续版本）

**内部改进（随本版带出）**
- 数据库接口引擎校验由两族重构为三族（MySQL 族/Redis/MongoDB）——重构前 Redis 系接口会误放行 MongoDB 实例
- 面板单二进制体积约 33MB → 41.7MB（mongo-driver 静态链接的预期代价）
- 证书：修复 HTTP-01 验证在服务器级 301 重定向之后的注入失效（多 server 块场景改注入到重定向目标块）
- 品牌标识：斗拱几何 Logo 上线（浏览器页签 / 侧栏 / 登录页 / 关于页），README 增加徽章行（Release / CI / License / Go 版本 / 下载量）

### 安装与升级

- 从 v0.1.x 升级：服务器上重跑安装命令即可（自动下载最新版、sha256 校验、原地替换二进制并重启 systemd 服务；数据目录 `/var/lib/dougong` 不动）
- 新安装：`curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | bash`
- 国内服务器：`DOUGONG_MIRROR=https://ghproxy.com sudo -E bash install.sh`
- 数据库迁移 011（实例档案表 engine 扩 MongoDB 值）随启动自动执行，既有实例档案与凭据密文不受影响

### 升级后建议验证

1. **凭据保存**：此前报「内部错误」的实例重新配置凭据——若宿主机缺客户端应看到明确提示「引擎客户端通道不可用（宿主机未安装 mysql/redis-cli）」，安装 `mysql-client`/`redis-tools` 后即可正常治理；密码含特殊字符不再有任何影响
2. **环境页刷新**：发起安装/升级/卸载后盯任务详情到终态——环境卡片的已安装状态/版本/服务态与「任务进行中」标记应自动落定，无需手动刷新
3. **MongoDB 全链**：环境页安装 MongoDB → 数据库页应自动发现实例（端口/socket 探测）→ 配置凭据（admin 库管理员账户）→ 库列表/建用户/角色授予回收
4. **权限提示**：若配置的凭据账户权限不足，状态概览等操作应提示「引擎操作被拒（凭据账户权限不足）」而非「内部错误」——serverStatus 需 clusterMonitor 类权限，用户/角色管理需 userAdmin 类权限

## v0.1.4 - 2026-11-17

环境管理升级链路热修（v0.1.3 服务器实测反馈）。

### 变更内容

- **修复环境升级在 conffile 冲突时失败**：升级中若新包携带的配置文件（如 `/etc/nginx/nginx.conf`）与本地已修改版本冲突，dpkg 会弹出交互式 Y/N 提示——无人值守环境下无 stdin 可读，直接失败（实机案例：Ubuntu 原生 nginx 1.24 经 nginx.org 源升 1.30 时卡死）。现采用无人值守标准三保险：`DEBIAN_FRONTEND=noninteractive` + `--force-confdef` + `--force-confold`——**本地已修改的配置保持现版不被覆盖**，发行版新版落 `.dpkg-dist` 旁置文件供事后对比
- **修复 MongoDB 源代配后签名验证失败（NO_PUBKEY）**：生成的 apt 单行源里 `arch` 与 `signed-by` 两个选项误用逗号分隔——apt 把整串并入 arch 值，`signed-by` 被静默丢弃，退回默认信任库后无法验证 MongoDB 官方源签名。已改为空格分隔并修正组件名（Ubuntu 用 `multiverse`、Debian 用 `main`，不再附加多余 `main`）；密钥本身无误（armored `.asc` 在 Ubuntu 24.04 实测可用）
- **修复刷新页面偶发裸 JSON 404**：九个页面路由与后端模块 API 前缀同名（`/env` `/sites` `/services` `/files` `/monitoring` `/terminal` `/firewall` `/docker` `/cron`），侧栏点击走前端路由无感，但**直接打开/刷新/收藏夹**这些地址会被后端按「模块前缀未知路径」吐 `NOT_FOUND` JSON 而非页面。现按浏览器导航特征（`Accept: text/html`）区分：导航回 SPA 页面，API 客户端维持 404 包络契约
- **修复未安装环境显示「已停止/自启关」**：systemd 的 LoadUnit 对不存在的单元也返回 `not-found` 形态的属性（inactive），被误映射为服务已停止。现该形态服务态如实为空，未安装环境不再渲染服务状态行
- 包操作命令由 `apt` 改为 `apt-get`（脚本场景稳定接口，消除 apt 自身的 WARNING 噪音）

### 安装与升级

- 服务器重跑安装命令即可（同 v0.1.3 方式；数据不动）
- **若你的服务器已卡在半配置状态**（升级报 `end of file on stdin at conffile prompt`），先手动解卡再升级面板：
  `sudo DEBIAN_FRONTEND=noninteractive apt-get -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" -f install`
  然后在面板重新发起升级，即可带新策略完成

## v0.1.3 - 2026-11-17

环境管理实机缺陷修复轮（v0.1.2 上线后的第一批服务器实测反馈）+ 面板 UI 打磨轮。

### 变更内容

**环境管理修复**
- 修复已安装服务「启用」报错 `org.freedesktop.DBus.Error.InvalidArgs`：目录表 systemd 单元名全部改为规范全名（如 `nginx.service`）——`EnableUnitFiles` 不像 `StartUnit` 那样自动补全后缀
- 修复 Docker / MySQL / Redis / PHP-FPM 卡片误显「探测不可用」：`dpkg-query` 对未装包的输出未被「未安装」判定覆盖，现改用包状态字段判定（不受系统语言影响），未装环境将如实显示灰色「未安装」
- 包名多候选检测：自装 `docker-ce`、`mysql-community-server` 等官方源形态可被正确识别为已安装
- 新增 **MongoDB 8.0** 环境（第七个环境）：自动配置 MongoDB 官方软件源（apt/dnf/yum 三系，含公钥下载与审计）后安装 `mongodb-org`

**面板 UI**
- 修复菜单图标重复：服务管理、文件管理及「用户与安全」四个子菜单此前因图标键未映射全部回退为同一兜底图标，现各有专属图标
- 清理提示文案：403 页内部术语（ADR-018）、站点空态、防火墙状态卡重复前缀、环境卡片「安装后可配合「X」模块使用」软引导行等
- 环境卡片描述去除模块括注；`/env/env/list` 响应移除 `relatedModule` 字段

**其他修复（随本版带出）**
- CLI / 服务端按已安装服务解析数据目录，不再依赖启动时的工作目录
- 证书：已有导入证书的域名允许发起 ACME 接管（CERT_003 语义修正）
- 证书页补充说明：导入证书为何不显示「更新证书」按钮
- 安装脚本：中英双语交互选择；大陆服务器下载路径友好化（镜像前缀 + 版本固定）

### 安装与升级

- **新安装**：`curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | bash`
- **从 v0.1.1 / v0.1.2 升级**：服务器上重跑同一条命令即可（自动下载最新版、sha256 校验、原地替换二进制并重启 systemd 服务；数据目录 `/var/lib/dougong` 不动）
- **国内服务器**（github 直连易被重置）：`DOUGONG_MIRROR=https://ghproxy.com sudo -E bash install.sh`；镜像不代理 API 时可固定版本：`DOUGONG_VERSION=v0.1.3`
- 数据库迁移 010（env_type 枚举扩 MongoDB）随启动自动执行

### 升级后建议验证

1. 环境管理页对已装 nginx 执行 启用/停用/启动 不再报错
2. 未安装的环境显示灰「未安装」而非橙「探测不可用」
3. MongoDB 安装全链（源代配 → 安装 → mongod 服务实况）
4. 若此前自装 docker-ce：Docker 卡片应显示已安装

## v0.1.2 - 2026-11-16

（本版 tag 曾按用户指示强制重指向，包含下列全部内容）

- 新增**环境管理**模块：nginx / Apache / Docker / MySQL / Redis / PHP-FPM 六环境安装·升级·卸载（apt 与 dnf/yum 双包管理器，异步任务台账）
- **站点列表聚合**：同配置文件的 80/443 server 块合并为一条目，可展开块清单
- **证书增强**：新增更新/替换证书；DNS-01 自动化验证（Cloudflare / 阿里云 DNS / DNSPod 凭证管理、自动 TXT、泛域名证书）
- **面板 IA 重构**：顶栏显示当前页标题；侧栏新增「用户与安全」分组（用户/会话/审计/个人安全，修复此前不可达）；设置页四分组重排；首启/登录页 dark mode 完整适配与文案净化
- 新增服务端管理 CLI：`dougong status / info / reset-password / set-port / entrypath / service`
- 数据库迁移 008/009；升级方式同上（重跑 install.sh）

## v0.1.1 - 2026-11-16

- 修复登录后侧栏整体消失（后端叶子菜单 `children:null` 挡崩前端渲染）
- CLI 优化：默认静默 gin 调试横幅（`DOUGONG_DEBUG=1` 恢复），新增 `dougong version`

## v0.1.0 - 2026-11-16

首个发布版本：单二进制面板（监控 / 服务 / 文件 / 网站 / 数据库 / 终端 / 防火墙 / 容器 / 计划任务与备份 / 账户与安全 十一模块），Linux amd64/arm64，一键安装脚本。
