# D0 诊断简报：v0.1.6 后实测反馈——MySQL 凭据验证 DBM_005 认证被拒（database-mgmt）

> 引擎 design 阶段 D0 产物（2026-11-17，增量模式·诊断+可能小修轮）。
> 用户服务器 104.129.180.140:16097（Ubuntu noble）实测：v0.1.5 的
> DBM_021/translatePingErr/--protocol=TCP 修复已生效（错误形态从裸 INTERNAL
> 变为正确业务码），本次 DBM_005 是服务器真实认证拒绝。
> 版本事实订正：任务输入称「最新 release=v0.1.5」，实测 git tag 已到 **v0.1.6**
> （ACME HTTP-01 跨文件拓扑轮，d594f3b）；本轮若需发版为 **v0.1.7**。

## 1. 反馈结构化

| 项 | 内容 |
|----|------|
| 请求 | POST /db/instance/credentials `{"instanceId":3,"connMode":"tcp","host":"localhost","port":3306,"username":"tonyxyz","password":"TonyXyZ@D2026"}` |
| 响应 | `{"code":"DBM_005","message":"凭据验证失败：认证被拒","data":{"reason":"authentication rejected: ERROR 1045 (28000): Access denied for user 'tonyxyz'@'localhost' (using password: YES)"}}` |
| 关键上下文 | MySQL 经面板 env-mgmt **apt 安装**（原生 MySQL 8，非 Docker）；mysql-client 已装（否则报 DBM_021 而非 005） |
| 错误形态解读 | ① "using password: YES"=密码已送达服务器（非空）；② 错误中 host 显示 **'localhost'**（非 127.0.0.1）=服务器名字解析开启（skip_name_resolve=OFF，否则显示 IP）；③ 1045=服务器真实拒绝（账户不存在/密码不符/host 匹配落空/插件不符，MySQL 不区分这些情形统一回 1045） |

## 2. 读码复核实证（面板侧验证链路——逐环核验，禁止臆测纪律）

| # | 环节 | 落点 | 结论 |
|---|------|------|------|
| 1 | 前端提交 | InstanceCredentialsModal.vue:115-123 | password 原样透传（无 trim/转义）；connMode/host/port 按 I-07 互斥组装 |
| 2 | 入参校验 | service/dbmgmt/instance.go:72-86（SetCredentials） | username 非空、password 非空且 ≤128 字符（consts.go:161）；**无截断/改写**；13 字符密码远在限内 |
| 3 | 目标求值 | service/dbmgmt/identity.go:92-（ConnTargetSpec.resolve） | tcp+localhost+3306 按请求纠正，I-07 互斥校验；无静默改道 |
| 4 | CLI 参数 | adapter/dbmysql/gateway.go:118-126（connArgs） | tcp → `-h localhost -P 3306 --protocol=TCP`（v0.1.5 修复在位） |
| 5 | 密码注入 | adapter/dbmysql/gateway.go:388-405（query） | `-u tonyxyz --batch -N -e "SELECT 1"` 字面量参数数组（R-08，无 shell）；`cmd.Env = os.Environ() + "MYSQL_PWD="+c.Password`（backup.go:155 cmdEnv）——env 真实注入、不进 argv；'@' 在 env 值中无特殊语义 |
| 6 | 错误归一 | adapter/dbmysql/gateway.go:408-421（classifyRun） | "Access denied" → ErrAuthRejected（携带 stderr 首行） |
| 7 | 业务码映射 | service/dbmgmt/service.go:183-185/220-230 | ErrAuthRejected → DBM_005 + data.reason（translatePingErr→translateGatewayErr）——与实拍响应逐字一致，**映射正确** |
| 8 | 建用户 SQL | adapter/dbmysql/gateway.go:265-274 + parse.go:22-（quoteString） | `CREATE USER 'u'@'h' IDENTIFIED BY '<转义密码>'`；quoteString 全量转义 ' " \ 控制字符，'@' 原样——无密码失真面 |
| 9 | 安装配置 | env-mgmt 全仓 grep（skip_name_resolve/my.cnf/bind-address） | **零命中**——面板 apt 安装不写任何 my.cnf 覆盖，服务器保持 apt 默认配置 |

**复核结论：面板侧验证链路（传参/env 注入/错误映射）无缺陷。DBM_005 是服务器真实拒绝的如实呈现。**

## 3. 面板侧已确认的 UX 缺口（两分支共通小修面）

| # | 缺口 | 落点 | 现状 |
|---|------|------|------|
| U1 | DBM_005 内联固定文案 | InstanceCredentialsModal.vue:130-131 | 「认证失败：用户名或密码错误（未保存）」——**丢弃 data.reason**（服务器 1045 原文含 host 形态，诊断价值高），且断言「用户名或密码错误」过窄（1045 亦含账户不存在/host 匹配落空） |
| U2 | DBM_006 内联固定文案 | InstanceCredentialsModal.vue:134-135 | 同样丢弃 data.reason（对照 DBM_021 已呈现 reason 的先例） |
| U3 | 无操作指引 | 同上 | 认证被拒时应给出可操作排查提示（账户是否存在/host 形态是否匹配连接来源/密码是否一致/是否经面板用户管理创建），而非仅透传或仅断言密码错 |
| U4 | 建用户 host 默认值 | CreateUserModal.vue:39/43 | 默认 **host='localhost'**（后端 host 必填无默认，identity.go:209-215 白名单校验）——默认值本身与 tcp localhost 验证形态**兼容**（名字解析开启时 TCP-from-localhost 解析为 localhost 可匹配）；是否构成缺陷待取证（若用户服务器 skip_name_resolve=ON 则 'localhost' 账户不匹配 TCP 来源——但本次错误形态显示名字解析开启，此假设暂被错误形态本身削弱） |

## 4. 根因假设矩阵（待服务器取证裁决，不预设结论）

| # | 假设 | 机理 | 裁决证据 |
|---|------|------|----------|
| H1 | tonyxyz 账户不存在 | 从未创建（或创建时用户名不同）；MySQL 对不存在账户统一回 1045 不泄露存在性 | 命令①空结果 |
| H2 | 账户存在但密码不符 | 创建时密码与本次验证输入不一致（手误/经其他渠道创建）；或面板创建时密码与用户记忆不一致 | 命令①有行+命令④复现 1045+面板审计 dbuser_created 记录比对 |
| H3 | 匿名账户遮蔽 | 存在 ''@'localhost' 匿名账户时，MySQL host 特异性排序使 TCP-from-localhost 先匹配匿名账户 → 密码不符 1045（经典陷阱；MySQL 8 apt 默认不建匿名账户，需实证） | 命令②全账户概览 |
| H4 | auth_socket 插件 | tonyxyz@localhost 若 plugin=auth_socket，TCP 密码认证被拒 | 命令① plugin 列 |
| H5 | host 形态落空 | 账户仅 'tonyxyz'@'127.0.0.1' 或特定网段而来源解析为 localhost；或 skip_name_resolve=ON 时 'localhost' 账户不匹配 TCP | 命令① host 列+命令③（错误形态已显示 localhost，H5 概率低） |

**推断注记**：错误中 host='localhost' 证明名字解析开启；若命令①显示 tonyxyz@localhost 存在且 plugin 为 caching_sha2_password/mysql_native_password，则密码正确时 TCP 验证**应当成功**——届时 H2（密码不符）为唯一剩余解释，可用命令④⑤双形态复现锁定。caching_sha2_password 兼容性：mysql-client 8.0 CLI 默认自动请求服务器公钥完成非 SSL 握手，noble 客户端无兼容缺口（且插件不兼容的错误形态是 2061/"requires secure connection" 而非 1045）。

## 5. 取证命令清单（只读，经主会话转达用户在服务器执行）

```bash
# ① tonyxyz 账户存在性/host 形态/认证插件
sudo mysql -e "SELECT user,host,plugin FROM mysql.user WHERE user='tonyxyz';"

# ② 全账户概览（匿名账户遮蔽排查）
sudo mysql -e "SELECT user,host,plugin FROM mysql.user ORDER BY user,host;"

# ③ 版本与名字解析开关
sudo mysql -e "SELECT @@version, @@version_comment, @@skip_name_resolve;"

# ④ 逐字复现面板调用形态（MYSQL_PWD env + --protocol=TCP + SELECT 1，只读）
MYSQL_PWD='TonyXyZ@D2026' mysql --protocol=TCP -h localhost -P 3306 -u tonyxyz --batch -N -e "SELECT 1"

# ⑤ 对照：127.0.0.1 字面主机（区分名字解析/host 匹配）
MYSQL_PWD='TonyXyZ@D2026' mysql --protocol=TCP -h 127.0.0.1 -P 3306 -u tonyxyz --batch -N -e "SELECT 1"

# ⑥ 面板审计：tonyxyz 是否经面板用户管理创建（含 host；面板 SQLite 只读查询）
sudo sqlite3 /var/lib/dougong/data/dougong.db "SELECT created_at,actor_name,object_id,detail,result FROM audit_log WHERE action='dbuser_created' ORDER BY id DESC LIMIT 10;"
```

注：④⑤密码经 env 前缀注入（与面板同形态，不进 argv）；命令会留在 shell 历史，取证后可自行清理。⑥路径为 systemd 安装布局默认（config/path.go:7）；若自定义 db_path 以实际为准。sqlite3 未装时可跳过⑥改在面板「审计日志」页筛选 dbuser_created。

## 6. 处置分支（取证后裁决）

- **分支 A（面板缺陷）**：若取证暴露面板侧根因（如建用户 host 默认与验证形态在特定服务器配置下不一致）→ D-lite 修复+回归测试，不放过真实拒绝、不改验证逻辑语义。
- **分支 B（用户侧账户/密码问题）**：如实告知修正方法（经面板用户管理重建账户/改密，或 mysql CLI ALTER USER）；面板侧仅做 **DBM_005/006 UX 指引增强小修**（U1-U3：内联呈现 data.reason + 可操作排查指引；文案不再断言「用户名或密码错误」）。**两分支均含 U1-U3 小修**（已确认缺口，非臆测）。
- 纪律：不为「修复」而改验证逻辑放过真实拒绝（D-03 先验证后落库语义不动）；CI 耦合预防——mysql 连接类测试注入化（fake 网关/纯函数面），不依赖实况引擎。

**实施记录（2026-11-17，取证等待期）**：U1-U3 UX 小修已先行实施（两分支共通已确认
缺口，非臆测；验证逻辑零改动）——types/database.ts（DBM_005 文案订正+dbmErrorText
reason 集扩+dbmAuthRejectedTips 纯函数）+InstanceCredentialsModal.vue（inlineReason/
inlineTips 三态）+7 用例；api v2.1/PageSpec v1.3 同步；vitest 188/188+build exit 0+
F-01~05 零命中+dbmgmt 六包 -race ok+dist 同步；轻量 Gate 报告
05-reports/gate2-dbm005-ux-2026-11-17.md（本轮改动面 PASS）；engine-cli check
C-01~C-11 全 PASS。根因终裁与发版决策待取证输出+用户确认。跨轮观察：工作区含未提交
ACME v2.4 改动，全仓 -race 唯一 FAIL（panel acme_test.go:488 webroot 旧断言）归属该轮。

## 6.1 取证终裁（2026-11-17，用户服务器输出已到）

**裁决：分支 B——H1 账户不存在（用户侧问题，面板零缺陷，验证逻辑零改动成立）。**

取证输出：
- ① `WHERE user='tonyxyz'` → 空（账户不存在）
- ② 全账户表仅 `debian-sys-maint` / `mysql.infoschema` / `mysql.session` / `mysql.sys` / `root@localhost`（**auth_socket**）——无任何应用账户
- ③ MySQL `8.0.46-0ubuntu0.24.04.5 (Ubuntu)`，`skip_name_resolve=0`（名字解析开启，与 1045 host='localhost' 一致）
- ④⑤ TCP localhost 与 127.0.0.1 均 1045（账户不存在）
- ⑥ `audit_log dbuser_created` → 空（从未经面板建用户）

用户确认：MySQL 经面板 apt 安装后「什么都还没有操作」——误把**面板登录账户** tonyxyz 当 MySQL 凭据；MySQL 独立用户体系仅有 root@localhost（auth_socket，无密码、仅本机 socket）。

**修正方法（已转达用户）**：服务器 socket 引导建第一个密码账户——
`sudo mysql -e "CREATE USER 'tonyxyz'@'localhost' IDENTIFIED BY '<密码>'; GRANT ALL PRIVILEGES ON *.* TO 'tonyxyz'@'localhost' WITH GRANT OPTION; FLUSH PRIVILEGES;"`
之后面板凭据验证即通。此即「装完≠配置就绪」痛点的实证。

**发版决策**：用户拍板**先不发版**，转向更大产品方向——「环境安装后自动配置就绪 + 多版本切换」（另起设计轮，宝塔做法与 apt 多版本可行性调研进行中）。本轮 U1-U3 UX 小修保留，留待与该方向合并发布。

**产品洞察（驱动下一轮）**：好面板的标准是「装完即配置就绪」，解决 Linux 运维配置难题。MySQL 安全模型决定第一个密码账户须 socket 引导（面板 TCP+密码连不上 auth_socket root，无法自助建首个用户）——env-mgmt 应在安装后自动做引导配置（建面板管理账户/随机强密码密文落库等），而非留给用户裸 apt 状态。

## 7. 执行模式声明

- 启动模式：**增量**（database-mgmt 全阶段 done，v0.1.6 已发；本轮=诊断+小修，非功能增量，不走 D1-D4 全量）。
- 注册表解析：引擎默认五键（design/gate1=session·skills/design·skills/validation；backend/frontend=subagent·stack orchestrator；gate2=session）；profile 无 execution 覆盖。
- 双轨探测：本执行环境（subagent）无 CreateWorkflow/Agent 工具 → **session 轨**执行（往轮均 session 轨，降级如实声明）；D-lite 小修由本会话直接实施，Gate 轻量落盘+回报等确认（不原地等待）。