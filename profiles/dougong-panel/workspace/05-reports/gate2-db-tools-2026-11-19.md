# Gate 2 验证报告 — 代码→交付门控（db-tools v1.0 + env-mgmt Node 增量）

**模块**：db-tools（新模块，ADR-032）+ env-mgmt Node 环境增量（B/F 产物）
**扫描路径**：backend/（D:/Project/dougong-panel/backend）+ frontend/src/（D:/Project/dougong-panel/frontend）
**检查时间**：2026-11-19
**编号源**：engine-rules.md GR-01~06（机制层）；go-vue3 栈 backend-rules.md G-01~07 / frontend-rules.md F-01~06（执行清单）；业务规则 knowledge/business-concepts/index.md R-02/03/08/09/14 + I-02；validation SKILL Gate 2 节 AL-001~005
**D0 基准**：00-architecture/d0-clarification-db-tools-integration-2026-11-19.md（Q1 一步到位 / Q2 面板会话 gating 反代〔不可妥协〕/ Q3 轻量入口并存 + 第四节安全底线 + 第六节验收口径）
**契约基准**：api-db-tools.md v1.2（11 端点）/ api-env-mgmt.md v1.2（七值，Node 增量待同步项 D-18）/ 04-business-enum.md（env_type 八值含 node）

---

## 一、编译门控（本轮实跑复核，非引用脚本结果）

| # | 命令（工作目录） | 结果 |
|---|------------------|------|
| C1 | `go build ./...`（backend/） | exit=0 |
| C2 | `go vet ./...`（backend/） | exit=0 |
| C3 | `go test ./... -race`（backend/） | exit=0，全仓测试包 ok 0 FAIL（含 api/dbtools 15.986s、service/dbtools 14.319s、store/dbtools 2.995s、adapter/toolx、service/envmgmt 6.743s） |
| C4 | `npx vue-tsc --noEmit`（frontend/） | exit=0 |
| C5 | `npm run build`（frontend/） | exit=0（vite built in 35.95s；chunk>500kB 警告为既有形态 INFO） |
| C6 | `npx vitest run`（frontend/） | exit=0，21 文件 264 用例全过（含 types/__tests__/dbtool.test.ts 26 + stores/__tests__/dbtool.test.ts 9） |
| C7 | `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./...`（backend/） | exit=0 |
| C8 | `gofmt -l`（本轮全部新增/改动包） | 0 输出 |
| C9 | dist 同步核验 | frontend/dist 与 backend/web/dist 资产哈希一致（index-OXBY_78w.js / index-j80ugB2H.js / index-B4cfUvMM.css）；Makefile:7-11 web 目标承载同步；backend/web/embed.go:12 `//go:embed all:dist` |

## 二、后端栈规则 G-01~07（实扫）

| # | 检查 | 命令 | 结果 |
|---|------|------|------|
| G-01 | 无 shell 拼接 | `grep -rnE 'exec\.Command(Weak)?Context?\(.+"(-c\|/c)"' --include="*.go" .` + 全量 `exec.Command` 逐点核对 | **PASS（增量面）**——唯一命中 internal/adapter/cronx/runner.go:145（`sh -c` argv 数组执行 cron 任务原文）为**既有 cron-backup 模块**（git status 未改动，gate2-cron-backup-2026-11-14 已审）；本轮新增 toolx/gateway.go:464、envpack/gateway.go:279、envpack BinaryVersion（`node --version` argv）、sysd DaemonReload（dbus Manager.Reload 非 shell）全参数数组 |
| G-02 | 无 SQL 拼接 | `grep -rnE "Sprintf\(.+(SELECT\|INSERT\|UPDATE\|DELETE)" --include="*.go" .` | **PASS**——全仓 0 命中；store/dbtools/task_repo.go·tool_repo.go 逐条核对全 `?` 参数化（List 动态 WHERE 仅追加固定片段+占位符，task_repo.go:84-102，用户输入不进 SQL 文本） |
| G-03 | 无硬编码凭据 | `grep -rnEi '(password\|secret\|token)\s*=\s*"[^"]{8,}"' --include="*.go" \| grep -v _test` + 补充 `(Password\|Secret\|Token\|APIKey):\s*"[^"]{6,}"` 结构体字面量扫描 | **PASS**——两式均 0 命中；凭据全部 crypto/rand 生成（vault.go:120-126 randomBytes / deploy.go:329-338 signon 密钥+blowfish / access.go:159-171 basic auth 口令） |
| G-04 | SQL 只在 store 层 | `grep -rlnE "(SELECT\|INSERT\|UPDATE\|DELETE) " --include="*.go" \| grep -v internal/store \| grep -v _test` 命中逐点核对 | **PASS（增量面）**——增量文件 0 实 SQL（envmgmt/service.go:337 等为注释文本）；层外实 SQL 仅既有 dbmysql 适配器（外部受管 MySQL 治理通道，gate2-database-mgmt 已审）与注释 |
| G-05 | 无 panic 滥用 | `grep -rn "panic(" --include="*.go" \| grep -v _test` | **PASS**——全仓唯一 internal/api/server.go:43 embed 启动期不可达不变式（规则允许）；增量 0 panic |
| G-06 | goroutine 退出路径 | `grep -rn "go func\|^\s*go " --include="*.go" \| grep -v _test` 逐点核对 | **PASS**——增量两处：①main.go:626-640 signon 台账清扫 goroutine 入 wg+ctx 取消退出+recover 隔离（RunTokenSweeper signon.go:215-226 select ctx.Done）；②deploy.go:159 `go s.runTask(taskID)` 生命周期=有限命令链（每命令 DEPLOY_OP_TIMEOUT=600s / DOWNLOAD_TIMEOUT=300s 注入，toolx run:453-461 WithTimeout），链终自然终止；重启不续跑（RecoverInFlight deploy.go:264-271，main 启动调用）。既有 main/dockerx/ptyx goroutine 非本轮 |
| G-07 | 编译+vet+test 全绿 | 见第一节 C1~C3 | **PASS** |

## 三、前端栈规则 F-01~06（实扫）

| # | 检查 | 命令（frontend/） | 结果 |
|---|------|------------------|------|
| F-01 | 无 Options API | `grep -rLn "script setup" --include="*.vue" src/` | **PASS** 0 命中 |
| F-02 | 无裸 any | `grep -rnE "(:\s*any\b\|as any)" --include="*.ts" --include="*.vue" src/` | **PASS** 0 命中（dbtReasonOf/dbtMissingOf/dbtTaskIdOf 均 unknown+类型收窄，types/dbtool.ts:118-149） |
| F-03 | 请求只走 api 层 | `grep -rnE "(axios\|fetch)\(" --include="*.vue" src/views src/components` | **PASS** 0 命中（8 fetch Action 全在 api/dbtool.ts 经 http.ts request） |
| F-04 | 无混入其他组件库 | `grep -rnE "from '(antd\|element\|ant-design-vue)" src/` | **PASS** 0 命中 |
| F-05 | 无原生弹窗 | `grep -rnE "(^\|\W)(alert\|confirm)\(" --include="*.vue" --include="*.ts" src/` | **PASS** 0 命中（卸载强确认=NModal 输入锁定 displayName，views/db-tools/index.vue:190-212） |
| F-06 | typecheck+build+单测 | C4/C5/C6 | **PASS** |

面板类专项：任务详情 5s 轮询终态即停+失败即停（stores/dbtool.ts:112-131），页面 onUnmounted 配对 stopPoll+clearTaskDetail（views/db-tools/index.vue:365-368）——frontend-rules §4.5 定时器清理过。

## 四、契约对齐 AL-001~005（机械 diff，本轮实跑）

脚本：`D:/tmp/gate2-dbtools/check.py` 与 `enum2.py`（`python -I` 隔离执行，UTF-8；转义管道 `\|` 保护 split）。

| # | 检查 | 方法 | 结果 |
|---|------|------|------|
| AL-001 | Controller 路径方法 vs api 文档 | check.py CHECK 1：module.go 路由正则提取（组前缀 /dbtool）↔ api 3.3 矩阵行双向 diff | **PASS 11/11**——controller-only=∅、api-only=∅（GET tool/list·task/list·task/detail·access/reopen·gate/verify + POST install·upgrade·uninstall·access/open·access/rotate·signon/verify） |
| AL-002 | 前端 Action 路径 vs Controller | check.py CHECK 2：api/dbtool.ts request 路径+方法提取 ↔ 面板面 8 接口（reopen 为 302 顶层导航非 fetch、内部 2 服务端调用，PageSpec 第 2 节显式声明） | **PASS 8/8**——双向 diff ∅；admin PageSpec S7 9/9（含 dbtoolReopen 非 fetch 声明） |
| AL-003 | DTO 可空逐形状 | check.py CHECK 3：dto.go `*T json` 集合 ↔ types/dbtool.ts `\|null` 集合 | **PASS 9/9**——installedVersion/boundInstanceId/running/runningTaskId/instanceId/startedAt/finishedAt/error/tokenExpiresAt 双侧显式可空；后端 *T 无 omitempty（0 命中）；脚本 frontend-only={version} 为函数参数误匹配（installedStateTag 形参，types/dbtool.ts:186），人工复核排除 |
| AL-004 | 枚举四方同值 | enum2.py：池（04-business-enum.md）↔ api 3.4 ↔ 前端 enums.ts ↔ 后端代码常量 | **db-tools PASS**——db_tool_type{phpmyadmin,mongo_express}/db_tool_task_action{install,upgrade,uninstall}/db_tool_task_status{pending,running,succeeded,failed} 四方逐值相等（store/dbtools/models.go:22-34）；db_engine 池==api==前端{mysql,mariadb,redis,mongodb}，后端目录子集{mysql,mariadb,mongodb}⊆池（redis 无工具 OUT 正确）。**env_type 四方不等（W2）**——池==后端八值含 node（store/envmgmt/models.go EnvNode + 迁移 014 八值 CHECK）；api-env-mgmt.md:135 与前端 enums.ts:598 EnvType 仍七值（无 node）；api==前端（七值一致）。已声明延后项：api-db-tools D-18 + backend/tasks.md:1154① + frontend/tasks.md:620③ + Gate 1 I4；db-tools 侧透传不硬编码（depMissingText dictItem 兜底原值，types/dbtool.ts:175-178）符合 D-18 |
| AL-005 | 错误码双侧 | check.py CHECK 5：errcode/dbtools.go New 符号集 ↔ api 3.4 DBT 行 ↔ 前端 DBT_ERROR_TEXT 键集 | **PASS**——DBT_001~012 三方逐字相等连续无缺号；AUTH_001/002/003/008 复用（module_test 回归 AUTH_001/003/008 在位）；内部端点拒绝语义一致（gate 裸 401/signon 包络 DBT_007/008/010/loopback·秘密头裸 404） |
| 包络 | 统一响应 | response.go Envelope{code,message,data} 实读 | **PASS**——`{"code":0,"message":"ok","data":...}` 与 api 3.1 一致（message 为全项目声明偏差，api 表头接口规约行）；失败 code=符号码串，前端 http.ts String 比对沿先例 |
| 审计码 | 三方同值 | enum2.py：consts.go AuditAction* ↔ api dbt_* 全集 ↔ enums.ts AUDIT_ACTION_TEXT | **PASS 6/6**——dbt_install_started/upgrade_started/uninstall_started/task_finished/tool_accessed/credential_rotated 三方相等（Gate 1 W1 已经 prd v1.1 回填闭环，prd-db-tools.md:90/92） |
| DDL | 迁移 vs api 5.3 | check.py CHECK 8：013_db_tools.sql ↔ api DDL 归一化比对 | **PASS**——db_tool/tool_deploy_task 两表 match=True，两索引（tool_type+status / created_at）在位；014_env_mgmt_node.sql 八值 CHECK 表重建保留 012 bootstrap_status/note 两列全量拷贝（沿 010 先例） |

## 五、业务规则 R-02/03/08/09/14 + I-02 + readonly 负空间

| 规则 | 核验（实读/实跑） | 结果 |
|------|------------------|------|
| R-02 会话+CSRF | api/dbtools/module.go:43-57 面板面 InitGuard→AuthRequired→CsrfGuard(POST)→RequireRole 链；HttpOnly dg_session 复用 account-auth；reopen GET 豁免 CSRF 有 api 3.1 显式安全口径（会话 gating+令牌双绑定承载）；回归 TestWriteEndpointsAdminOnly（缺 CSRF→AUTH_003）/TestReadEndpointsBothRoles（未登录→AUTH_001）实跑过 | **PASS** |
| R-03 变更审计 | 任务创建+发起审计同事务（deploy.go:124-150 createTask）/终态+档案+审计同事务（deploy.go:215-260 finishTask）/轮换密文+审计同事务（access.go:214-228）——TestTxTaskAuditAtomic/TestTerminalAuditSameTx 实跑过；open/reopen 审计 recordAudit 重试兜底（service.go:291-299，无伴随状态写，api「同事务」措辞差异见 I4）；detail 脱敏回归 TestOpenValidation 断言不含令牌/凭据/URL（security_test.go:278-288）；gating 逐请求不落审计（⚠️A11，gate.go:20） | **PASS** |
| R-08 参数数组 | G-01 全点核对；TestNpmArgvArrays（toolx gateway_test.go:149）断言 npm argv；TestInstallMongoExpressChain 断言凭据不进 argv（deploy_test.go:223-227）；凭据经 systemd Environment 注入（templates.go:211-238，单元 0600） | **PASS** |
| R-09 零遥测 | phpMyAdmin `$cfg['VersionCheck']=false`+`SendErrorReports='never'`（templates.go:105-106，TestInstallPHPMyAdminChain 断言）；下载为用户主动发起例外（toolx DownloadTarball 唯一外联，D-05 与 ACME 同判）；增量 service 代码 0 其他外联（grep net/http 实扫）；NodeSource 源代配为用户主动安装步骤 | **PASS** |
| R-14 服务端校验 | toolType 经 CatalogByType 白名单（DBT_002，deploy.go:29-31 等）/status 筛选取值域校验（service.go:244-249）/taskId>0（module.go:196-201）/instanceId 必填语义（deploy.go:36-40、access.go:60-63）/reopen ParseInt>0（module.go:173-177）/ShouldBindJSON 绑定错误→AUTH_012；回归 TestWriteEndpointsAdminOnly（非法 toolType→DBT_002 不触发 worker）实跑过 | **PASS** |
| I-02 凭据零出口 | ToolVault AES-256-GCM+dbtool-cred.key 0600 原子写（vault.go:45-70）；DTO 无凭据字段（dto.go 全量实读——ToolItem/TaskItem/OpenData{url,tokenExpiresAt}/RotateData{rotatedAt} 无口令）；cred_cipher 仅 vault 加解密路径消费（signon.go:69/deploy.go:727-736/access.go:137）；signon 凭据响应仅 loopback 内部端点（module.go:64-66 防护链+SignonVerifyData 注释 dto.go:90-97）；dbmgmt/toolgateway.go 明文用毕即弃不落库不入日志（全文件实读 0 日志调用）；审计/日志脱敏回归在位（TestOpenValidation/TestSignonVerifyGuard 断言无泄漏）；rotate 响应无口令字段（api 4.1.4 一致） | **PASS** |
| readonly 负空间 | 6 负空间 Action（Install/Upgrade/Uninstall/Rotate/Open/Reopen）三层防护：①前端 views/db-tools/index.vue:470 操作区 `v-if="authStore.isAdmin"` 整区不渲染 + AdvancedManageEntry.vue:148 角色兜底 + 父级 database-mgmt index/mysql/mongo 均 isAdmin 门控（实 grep 在位）；②后端 RequireRole(admin) 写接口 AUTH_008 回归 + reopen handler 内 role!=admin→302 /db-tools（module.go:169-171，TestReopenRedirects 实跑过）；③工具路径网络层 gating readonly→401（TestGateVerifyGuard 实跑过）；readonly PageSpec v1.1 负空间 6 项与实现逐项对应（3 读+6 负空间=面板 9 全集）；轻量浏览保留可达（Q3） | **PASS** |

## 六、安全专项实证（Q2 不可妥协，逐项）

### S1 工具仅绑 localhost（无 0.0.0.0 公网监听）— PASS（代码+测试实证）

- mongo-express：编译期常量 `MongoExpressBind="127.0.0.1"`（consts.go:30）→ 单元模板 `ME_CONFIG_SITE_BIND_ADDRESS`（templates.go:228）显式覆盖 mongo-express 默认 0.0.0.0；端口 8081 编译期定死（consts.go:28）。**回归断言**：TestInstallMongoExpressChain 逐字断言单元含 `ME_CONFIG_SITE_BIND_ADDRESS="127.0.0.1"`+`ME_CONFIG_SITE_PORT="8081"`（deploy_test.go:235-236），实跑过。
- phpMyAdmin：无独立监听端口——FastCGI 经 PHP-FPM pool unix socket（templates.go:39-63，TestInstallPHPMyAdminChain 断言 `fastcgi_pass unix:/run/php/php8.1-fpm.sock`）。
- nginx 工具 location proxy_pass 上游亦 127.0.0.1:8081（templates.go:73，回归断言 deploy_test.go:264）。
- 面板自身默认监听 127.0.0.1:8687 配置注入（config.go:94）；panel ListenScope=all 时 0.0.0.0 为面板既有形态（D-P02），内部端点另有 loopback 源校验兜底（S2）。
- 未执行项如实：实机 `ss -tlnp` 端口观测归 Linux 目标环境冒烟（本轮无 Linux 运行时）。

### S2 nginx 工具路径 auth_request 会话+admin gating — PASS（模板+HTTP 级回归实证）

- 注入模板：phpMyAdmin 外层 location（templates.go:48）**与嵌套 regex php location**（templates.go:52）双 auth_request；mongo-express location auth_request（templates.go:71）；内部 gate location `internal;`+秘密头+Cookie 透传+request_body off（templates.go:23-31）。
- 端点防护链：loopbackGuard RemoteAddr∈{127.0.0.1,::1} 否则裸 404（module.go:210-223）→ internalSecretGuard 常量时间比对否则裸 404（module.go:227-238）→ GateVerify 消费 VerifyWs 只读不续期+role==admin 服务端权威（gate.go:22-37；session.go:59 实读）。
- **HTTP 级回归（httptest 全栈，本轮 go test -race 实跑过）**：TestGateVerifyGuard（module_test.go:491-515）——秘密头缺失→404、错误秘密→404、**无会话→401、readonly→401、失效会话→401、admin→200**（裸状态无包络，auth_request 语义）。
- 注入往返回归：TestInjectionRoundTripBothTools（inject_test.go:100-155）——gate 幂等不重复+nginxx.ValidateSyntax 语法自检+撤除 byte 级还原+幂等；TestInstallPHPMyAdminChain/TestInstallMongoExpressChain 断言注入 conf 含 `auth_request /dbtool-gate-internal`。
- 吊销级联：VerifyWs 逐请求实时判定 revoked/expired（session.go:68-71），无独立长时会话。
- 未执行项如实：真实 nginx auth_request 子请求联调归 Linux 冒烟（本轮经 httptest 直打内部端点等价验证判定链）。

### S3 signon 令牌签名防伪+短时效+一次性（防重放回归）— PASS（四重校验全回归实证）

- 规约实现：HMAC-SHA256（signon.go:109-112/149-153 subtle.ConstantTimeCompare）+TTL 60s（consts.go:51，服务端时钟权威 signon.go:162）+jti 一次性内存台账（消费即标记 signon.go:166-176，缺席保守拒绝）+instanceId/sessionDigest 双绑定（signon.go:177-191，sessionDigestOf SHA-256 前 8 位与 account-auth 同口径）+30s TTL 清扫（RunTokenSweeper 入 wg）+重启全失效（内存态）。
- **回归（本轮实跑过）**：TestSignonTampered（payload 跨实例篡改/签名本体伪造/形态非法→DBT_007）、TestSignonExpired（时钟推进超 60s→DBT_008）、TestSignonReplay（消费后重放→DBT_010；新台账模拟重启→DBT_010）、TestSignonBinding（跨会话/失效会话/未转发 cookie/跨实例→DBT_007）、TestSignonKeyIsolation（卸载除档后令牌永久不可校验）、TestTokenSweeper（清扫后窗口收敛）、TestSessionDigestDerivation（digest 口径+HMAC 确定性）——security_test.go:56-371。
- HTTP 级：TestSignonVerifyGuard（module_test.go:519-551）缺秘密头裸 404+伪造令牌包络 DBT_007+响应无凭据泄漏断言。
- W4 闭环：SignonScript 显式转发 `$_COOKIE['dg_session']`+dg_tool_instance（templates.go:140-142），Verify 消费 forwardedSession 校验（signon.go:183-191）。

### S4 凭据 vault 永不回显 — PASS（见第五节 I-02 行，此处专项复核）

- 三类凭据载体分清：MySQL/Mongo 连接凭据取 db-mgmt vault 本域不持久化（toolgateway.go:48-73 用毕即弃）；basic auth 口令+signon 密钥+blowfish AES-256-GCM 密文落 db_tool.cred_cipher（deploy.go:339/453）；密钥文件 dbtool-cred.key 0600（vault.go:63）。
- 浏览器全程无凭据：open 响应仅 {toolType,url,tokenExpiresAt}（令牌非凭据，api 口径显式）；mongo-express basic auth 经 nginx include 0600 秘密文件服务端注入（templates.go:260-264，浏览器无感知）；signon 凭据仅 loopback 响应→$_SESSION（templates.go:155-164）。
- 权限分级（A-DBT5 声明裁定）：internal-secret/mongo-express-auth.conf/systemd 单元/dbtool-cred.key=0600（TestWriteSecretFileMode/TestInternalSecret/TestInstallMongoExpressChain 断言 0600）；config.inc.php/signon.php=0644（PHP-FPM 非 root worker 必读；秘密头为纵深防御，持秘密仍须有效 admin 会话/合法令牌——api 4.4.4 措辞差异见 I2）。
- 回归断言：TestOpenValidation 审计 detail 不含令牌/凭据；TestSignonVerifyGuard 响应不含 mysql-pass；TestInstallMongoExpressChain 凭据不进 argv。

### S5 卸载不删数据库数据 — PASS（白名单回归实证）

- 执行链白名单（deploy.go:595-653）：phpMyAdmin=撤除注入+删工具目录+fastcgi 模板；mongo-express=停服/禁用/移除单元/daemon-reload→撤除注入→npm uninstall→删 auth 秘密文件。**0 处触及引擎数据目录或 db_instance 档案**（全文件实读核对）。
- **回归（本轮实跑过）**：TestUninstallDataPreservation（deploy_test.go:275-347，两工具子测试）——全部 remove/removeall 操作路径断言 ∈ {toolDir 前缀, MongoExpressUnitPath}，禁词断言 /var/lib/mysql、/var/lib/mongodb、/var/lib/docker、db_instance 零触及；nginx conf byte 级还原原字节；档案同事务删除。
- 前端强确认：NModal 输入锁定（须键入 displayName 方可确认，index.vue:194-197）+文案「卸载任务已创建（不删除数据库数据）」（index.vue:211）+PageSpec 两段警示。
- sha256 强制不降级（D-05 附证）：TestInstallSha256Mismatch——失配=failed+文件清理+不解压不注入不留半档案（deploy_test.go:162-191）；TestExtractTarZipSlipRejected/SymlinkRejected（toolx gateway_test.go:103/130）路径逃逸/链接条目拒绝。

## 七、测试覆盖率（本轮 go test -cover 实测）

| 包 | 覆盖率 | 目标（validation SKILL） | 达标 |
|----|--------|--------------------------|------|
| service/dbtools | 73.0% | Service 80%+ | **NO（W3）** |
| api/dbtools | 73.5% | Controller 60%+ | YES |
| adapter/toolx | 64.4% | —（适配器全注入 fake） | 参考 |
| store/dbtools | 76.5% | — | 参考 |
| service/envmgmt | 53.0% | —（既有模块全量稀释，Node 增量 5+5 用例独立在位） | 参考 |

安全核心路径（signon 四重/gating/vault/卸载白名单/sha256/注入往返/轮换回滚）专用回归全覆盖；未覆盖余量主要为 deploy 执行链错误分支组合（W3 如实上报）。增量测试函数计数：service/dbtools 26 + api/dbtools 6 + toolx 10 + store/dbtools 3 + envmgmt node_env 5 + node_vendorrepo 5 + dbmgmt/toolgateway 4 + panel/toolaccess 1 + sitemgmt/configpipeline 2。

## 八、WARN 项（非 CRITICAL，待用户确认）

- **W1 db-tools 菜单重复登记**：cmd/dougong/main.go:328 与 main.go:462 两次 `Menus().Register(MenuItem{Key:"db-tools",...})`（icon 一为 "dbtool" 一为 "database"，Order 均 550）。MenuRegistry.Register 纯 append 无去重（service/panel/menu.go:47-51），Tree 亦不去重；前端 toMenuOptions 无去重且两项 key 同为 "/db-tools"（MainLayout.vue:111-127）→ 侧边栏将渲染两个「数据库工具」项（n-menu 重复 key）。非安全缺陷，功能/UX 正确性问题，建议删除其一（保留 icon=dbtool 项，与 MainLayout.vue:104 图标映射注释一致）。
- **W2 env_type 四方同值缺口（已声明延后，本轮未闭环）**：池==后端代码==迁移 014 八值（含 node）；api-env-mgmt.md:135/:124（ENV_002「七值」）/:327（DDL CHECK 七值）与前端 enums.ts:598-607（EnvType 七值无 node）仍七值。声明链：api-db-tools D-18「env-mgmt api Node 增量为其 D3 增量轮待同步项」+ backend/tasks.md:1154①「设计侧文档同步归设计轮」+ frontend/tasks.md:620③「env_type node 为 env-mgmt 待同步增量，dictItem 兜底原值」+ Gate 1 I4。注意 014_env_mgmt_node.sql 头注「跨模块待同步项本轮落定」仅 DDL 面成立，api/PageSpec/前端 enums 三面未落定。运行时影响：env/list 返回 node 条目经前端 dictItem 兜底原样呈现（不崩溃），env-mgmt 页 node 服务操作入口未特化（后端 TestNodeServiceOpRejected 如实拒绝兜底）。**建议设计轮同步 api-env-mgmt（env_type 八值/ENV_002/DDL CHECK/常量 NODE_LTS_TRACK）+ env-mgmt PageSpec + 前端 EnvType/ENV_TYPE 后方记 env-mgmt Node 增量完整交付**；db-tools 模块本身透传设计（D-18）不受影响。
- **W3 service/dbtools 覆盖率 73.0% < 80% 目标**：安全核心专用回归全覆盖（第六节），余量为 deploy 错误分支组合；沿 account-auth W1 覆盖率余量先例如实上报待确认。

## 九、INFO 项

- **I1** signon/verify 响应 code 含 +host/+port（dto.go:92-97），api 4.4.2 正文仍 {user,password}——A-DBT4 已声明 additive 裁定（tasks.md:1110，phpMyAdmin 建连必需），建议 api 正文回填。
- **I2** api 4.4.4 末行将 config.inc.php 列入「WriteSecretFile 0600」秘密文件清单，与代码/测试/A-DBT5 分级（config.inc.php/signon.php 0644 PHP-FPM worker 必读）措辞不一致——安全理由已声明（loopback 主防线+秘密头纵深防御），建议 api 措辞订正为分级表述。
- **I3** FailRunningAll 将 pending 一并终态化（task_repo.go:146-150 `status IN ('running','pending')`），api SQL #8 仅 running——保守扩展（重启后 pending 孤儿任务不续跑），代码注释已声明，建议 api 补注。
- **I4** open/reopen 审计经 recordAudit 非事务通道（service.go:291-299 重试+日志兜底），api 4.1.4 措辞「同事务」——打开动作无伴随状态写（审计即唯一写），沿 env-mgmt D-12 口径，不构成 R-03 违例。
- **I5** INDEX db-tools 行 frontend/validation stage 仍 todo（INDEX.md:42-43），而 F 产物已实存（views/db-tools + api/dbtool.ts + stores/dbtool.ts + 35 前端用例全过）——stage 行更新归工作流脚本，本 Gate 仅按 ask 更新 gates.gate2。
- **I6** vite chunk>500kB 警告（index-OXBY_78w.js 4.1MB）为既有形态（monaco/echarts 等），非本轮引入。
- **I7** A-DBT6 注入时效：entryPath/面板域变更后须重装工具刷新注入，期间 gate 子请求 404→工具路径 401 fail-closed（无安全洞，可用性降级）——已声明跨模块变更钩子 OUT 本轮。

## 十、未执行项如实声明

- **Linux 实机冒烟（D0 验收口径末项）本轮未执行**：PHP-FPM+扩展装 phpMyAdmin 实登录管 MySQL、Node 装 mongo-express 实管 Mongo、真实 nginx auth_request 子请求、实机端口观测、卸载后数据目录实查——归目标环境（本轮 Windows 开发机无 Linux 运行时/npm/php/systemd 实况）。等价验证经全注入 fake 替身 httptest 全栈回归承载（第六节逐项），非桩假安全逻辑（判定链真实执行，仅外部执行体伪化）。
- govulncheck（backend-rules §6 CI 周期项）本轮未执行（非 Gate 2 必跑清单）。
- 工作区存在本轮外未提交改动（git status 含 env-mgmt/site-mgmt 等前轮文件）——沿历轮 Gate 2 先例如实登记，不影响本轮增量面判定。

## 十一、总结

**判定：WARN**（CRITICAL 0 / 契约断裂 0〔db-tools 模块面〕/ 安全底线五专项全实证通过）

- 编译门控 C1~C9 本轮实跑全绿；G-01~07 增量面全过（唯一 sh -c 为既有 cronx 豁免）；F-01~06 全 0 命中。
- AL-001~005 db-tools 模块面机械 diff 零差（11 端点/8 fetch Action/9 可空字段/3 枚举四方+db_engine/12 错误码三方/6 审计码三方/包络/DDL）；env_type 四方缺口为已声明跨模块延后项（W2）。
- Q2 安全专项 S1~S5 逐项代码取证+回归实跑实证：localhost 绑定（单元 env 断言）/auth_request 无会话·非 admin→401 回归/signon 防伪·60s·一次性·双绑定全回归/vault 零回显（响应·审计·日志·argv 四通道断言）/卸载白名单不删数据回归。
- WARN W1（菜单重复登记）/W2（env_type 设计轮同步）/W3（覆盖率余量）待用户经主会话确认；确认后 Gate 2 记 pass。
- 报告路径：05-reports/gate2-db-tools-2026-11-19.md