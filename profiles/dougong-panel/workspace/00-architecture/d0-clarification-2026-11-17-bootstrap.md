# D0 增量澄清 — 环境安装后配置就绪（post-install bootstrap）

| 属性 | 值 |
|------|-----|
| 日期 | 2026-11-17 |
| 轮次 | /pipeline 增量轮（env-mgmt 为主 + database-mgmt OHS 联动） |
| 启动模式 | 增量（env-mgmt / database-mgmt 全阶段 done，section-6 changelog 驱动） |
| 执行轨 | session（本会话无 CreateWorkflow / Agent 工具，工作流轨与 subagent 轨均降级；与往轮一致） |
| 产品定调 | 「好面板的标准是装完即配置就绪，解决 Linux 运维配置难题」 |

## 1. 需求与实证痛点

DBM_005 诊断轮终裁实证：env-mgmt 经 apt 装完 MySQL 只有 `root@localhost`（auth_socket 无密码），面板 db-mgmt 走 TCP+密码验证通道连不上，用户无法自助建第一个账户——**装完 ≠ 可用**。本轮目标：**环境安装后自动引导配置到面板可直接治理**。

## 2. 范围（用户拍板，经主会话 AskUserQuestion 确认）

- **IN（本轮）**：仅「安装后配置就绪」——MySQL / Redis / MongoDB 三类数据库环境安装成功后的 post-install bootstrap（建管理账户/设密码 + 自动注册 db_instance 与凭据到 db-mgmt，装完即可管）。
- **OUT（二期，PRD 登记）**：
  - PHP 多版本并存（sury.org/ondrej PPA 提供 5.6~8.6 coinstallable，各版本独立 phpX.Y-fpm）——另起轮。
  - MySQL 版本升级路径（mysql-apt-config 单版本原地升级，不支持降级/禁跳系列/须备份）——另起轮。
- PHP/nginx/Apache/Docker：装完即用，**无引导**（nginx 不强制 enable+start，沿 ⚠️A5；apt 包脚本默认已自启）。

## 3. 关键拍板（四项，均经主会话 AskUserQuestion 用户确认，采纳推荐）

| # | 议题 | 拍板 | 理由/约束 |
|---|------|------|----------|
| Q1 | MySQL 管理账户 host 形态 | **dougong@localhost**（单账户，随机强密码密文落库） | fresh install 默认 skip-name-resolve=OFF，TCP 127.0.0.1/::1 经名字解析均匹配 localhost，socket 亦匹配；改密单点；恒限本机。风险登记：用户日后开启 skip-name-resolve 则 TCP 落空（需重新引导，如实呈现）。账户名 dougong。 |
| Q2 | MySQL 治理权限集 | **ALL PRIVILEGES ON *.* WITH GRANT OPTION** | db-mgmt 既有三档授权含 all=ALL PRIVILEGES；MySQL 规定「授出某权限须自身持有该权限 + GRANT OPTION」，最小治理集无法支撑 all 档（会外溢裁剪 database-mgmt 既有能力）。安全靠 host 限本机 + 随机强密码 + AES-256-GCM 密文落库 + 零回显。与宝塔 root 管理同构。 |
| Q3 | 引导失败状态语义 | **独立引导结果维度** | 任务 status=包操作结果（安装成功=succeeded）；新增 bootstrap_status(succeeded/failed/skipped，NULL=无引导步骤) + bootstrap_note 入任务台账与详情。安装成功但引导失败 → status=succeeded + bootstrap_status=failed，呈现「已安装·引导失败」与原因；恢复=重新安装/升级（幂等重跑引导）。语义最诚实，不混淆「已装」与「可治理」。需 env_install_task 加列迁移。 |
| Q4 | Redis 配置写入方式 | **CONFIG SET requirepass + CONFIG REWRITE** | Redis 原生持久化，免重启无中断；仅 requirepass 未设置时执行（保守不覆盖用户已有密码）；bind 经 CONFIG GET 校验（fresh install 已 127.0.0.1 ::1，无需写）。无需定位 conf 路径。CONFIG REWRITE 规范化整个 conf（fresh 默认 conf 无用户配置，可接受）。 |

## 4. 设计要点（D 阶段细化依据）

### 4.1 引导触发点与跨模块边界
- env-mgmt worker 安装成功（install/upgrade 动作）后执行 post-install bootstrap，**独立步骤**，结果入任务输出与 bootstrap_status/note。卸载与非数据库环境无引导。
- **架构归属**：bootstrap 的「建账户/设密码 + 注册 db_instance + 凭据密文」归 **database-mgmt**（其已持有 dbmysql/dbredis/dbmongo 适配器 + CredentialVault + db_instance 仓储），经**进程内 OHS** `BootstrapAfterInstall` 暴露；env-mgmt worker 消费该 OHS（与 cron-backup 消费 DumpStream 同构，单向依赖 env-mgmt → db-mgmt，无环）。
- **服务就绪**归 env-mgmt（持有 SysdGateway + 目录单元映射）：bootstrap 前确保 DB 服务 active（必要时 start，幂等）；db-mgmt bootstrap 内有界重试等待引擎就绪。⚠️A5 细化：引导路径可为配置而启动 DB 服务，enable 仍不强制。

### 4.2 各引擎引导通道（区别于 TCP+密码验证通道）
- **MySQL**：面板以 root 运行（systemd），dbmysql 网关经 `--socket <path>` + `-u root` + 空 MYSQL_PWD 命中 root@localhost 的 auth_socket 免密建连（引导通道）。socket 路径取 DISCOVERY_PROBE_SET 候选逐一尝试。CREATE USER dougong@localhost IDENTIFIED BY 随机强密码 + GRANT ALL ON *.* WITH GRANT OPTION。**不动 root 的 auth_socket**。
- **Redis**：redis-cli 连 localhost:6379（初始无 requirepass 免认证）→ CONFIG GET requirepass 判已设置则跳过 → 否则 CONFIG SET requirepass 随机密码 + CONFIG REWRITE（新密码认证）持久化；CONFIG GET bind 校验本机。
- **MongoDB**：mongo-driver 进程内连 localhost:27017 **免认证**（localhost exception：无用户时可经 localhost 建首个 admin 用户）→ admin 库建 dougong 用户 root 角色随机密码；已有用户/已开鉴权则保守跳过。
- **随机强密码**：crypto/rand 生成（不弱默认）；AES-256-GCM 密文落 db_instance.cred_cipher（沿 CredentialVault/db-cred.key 0600 先例）；**密码永不回显**（审计 detail/任务输出/响应均不含密码）。

### 4.3 自动注册与幂等/安全
- 引导成功后经治理通道（TCP）Ping 验证新凭据可连 → 加密 UPSERT db_instance（本机默认 identity：mysql tcp localhost:3306 / redis tcp localhost:6379 / mongodb tcp localhost:27017）+ conn_state=reachable + version 刷新 + 审计 instance_bootstrapped。**装完即可管**。
- **幂等/保守**：目标 identity 已配置凭据（credentialsSet）→ 跳过不覆盖；MySQL dougong 已存在 / Redis requirepass 已设 / MongoDB 已有用户 → 保守跳过（不覆盖用户已有配置，conffile 保守先例）。已装/已配置环境不重复引导。
- **安全**：账户 host 限本机；bind 本机不公网暴露；引导动作审计（env_bootstrap 类 + instance_bootstrapped）；密码密文存储零回显。

### 4.4 呈现
- 引导结果主呈现于**任务 Drawer**（bootstrap 结果横幅 + output_tail [bootstrap] 行）；env/list 卡片本轮不加角标（控制范围）。失败横幅含原因与「重新安装/升级以重试引导」恢复指引。
- 无新增 HTTP 接口（bootstrap 由 worker 内部触发 + OHS）；task/list·task/detail 响应增 bootstrapStatus/bootstrapNote 字段（interface_changed）；S7 Action 数不变（10）。

## 5. 受影响阶段（增量映射）

D1（BA/AA/FL env-mgmt + db-mgmt OHS 登记）→ D2（prd-env-mgmt + domain-model + 枚举池 bootstrap_status）→ D3（api-env-mgmt + api-database-mgmt）→ D4（PageSpec env-mgmt 任务 Drawer）→ Gate 1 → B/F → 编译门控 → Gate 2 → 收尾。

## 6. 工作区纪律

含其他轮未提交改动（DBM_005 UX 小修 frontend/types/database.ts 等 + ACME v2.4 后端改动，acme_test.go:488 旧断言 FAIL 归属 ACME 轮）——本轮专注自身文件，不触碰/不修复 ACME 轮改动，git 操作一律不做（主会话统一处理）。CI 环境耦合预防：mysql socket/mongo/redis 引导类测试全注入化，零 runner 实况引擎依赖。