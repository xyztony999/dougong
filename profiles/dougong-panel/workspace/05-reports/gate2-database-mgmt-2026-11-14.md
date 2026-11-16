# Gate 2 验证报告 — 代码→交付门控（database-mgmt）

**模块**：数据库管理（database-mgmt，M4）
**检查时间**：2026-11-15（报告文件名沿派发脚本命名 2026-11-14）
**检查员**：交付检查员（独立轮，只查不改）
**契约基准**：api-database-mgmt.md v1.1（Gate 1 复检拍板口径：B/F 验收以 v1.1 为准）
**规则链**：引擎 GR-01~GR-06（engine-rules.md 缺席，按 validation SKILL Gate 2 分层说明执行）⊕ backend-rules §7 G-01~G-07 / frontend-rules §6 F-01~F-06（🔴 CRITICAL 与 ⚠️ 项）⊕ business-concepts R-01~R-18 适用项
**编译门控**（引用派发脚本，未重跑）：build=0 vet=0 test=0 web=0
**扫描路径**：backend/internal/{api/dbmgmt,service/dbmgmt,store/dbmgmt,adapter/dbconn,adapter/dbmysql,adapter/dbredis,errcode} + cmd/dougong/main.go + migrations/005；frontend/src/{types/database.ts,api/database.ts,stores/database.ts,views/database-mgmt/**,router/index.ts 追加段}

## 判定：WARN（CRITICAL 0 / 契约断裂 0；2 项 WARN 建议项）

---

## 1. 后端栈规则扫描（backend-rules §7 G-01~G-07，实跑）

| # | 检查 | 结果 | 证据 |
|---|------|------|------|
| G-01 | 无 shell 拼接 | PASS | `grep -rnE 'exec\.Command(Weak)?Context?\(.+"(-c\|/c)"' --include="*.go" .` → 0 命中（rc=1）。全仓 `grep -rn "exec\.Command"` 9 命中：本模块真实调用 4 处——dbmysql/gateway.go:393、dbmysql/backup.go:108/:132、dbredis/gateway.go:204，全部 `exec.CommandContext(ctx, bin, args...)` 参数数组形态（逐处人工核对源码）；nginxx/gateway.go:303/:327 属 site-mgmt；其余 3 处为包注释文字 |
| G-02 | 无 SQL 字符串拼接 | PASS | `grep -rnE 'Sprintf\(.+(SELECT\|INSERT\|UPDATE\|DELETE)' --include="*.go" .` → 0 命中（rc=1） |
| G-03 | 无硬编码凭据 | PASS | `grep -rnEi '(password\|secret\|token)\s*=\s*"[^"]{8,}"' --include="*.go" . \| grep -v _test` → 0 命中（rc=1）。引擎凭据运行时经 MYSQL_PWD/REDISCLI_AUTH env 注入子进程（dbmysql/gateway.go:394、dbredis/gateway.go:205） |
| G-04 | SQL 只在 store 层 | PASS（含 A-DB10 裁定独立复核） | 面板 SQLite SQL 全部在 internal/store/dbmgmt/instance_repo.go:28-169 且全参数绑定（逐行核读：List/GetByID/GetByIdentity/UpsertDiscovered/UpdateCredentials/UpdateTargetAndCredentials/UpdateConnState/Delete 均 `?` 占位）。层外命中两类：①注释文字——service/dbmgmt/prober.go:15、ports.go:24；②adapter/dbmysql/gateway.go:128-354 引擎侧语句（SHOW/SELECT/CREATE/GRANT 等）——**独立复核维持 A-DB10 裁定**：该语句组装是 api §4.1.5 DbGateway 契约方法本体（CreateDatabase(inst,name,charset) 等签名即要求适配器组装引擎 SQL），非面板 SQLite SQL；注入防线=标识符白名单正则（service/dbmgmt/identity.go:184-215，R-14/AUTH_012 先行）+ 密码 quoteString 全量转义（dbmysql/parse.go:22-47）；api §3.2/D-13 参数绑定约束明文限定「SQLite 侧」 |
| G-05 | 无 panic 滥用 | PASS | `grep -rn "panic(" --include="*.go" . \| grep -v _test` → 0 命中（rc=1） |
| G-06 | goroutine 有退出路径 | PASS | `grep -rn "go func"` → 6 处全在 cmd/dougong/main.go:268/:283/:298/:314/:352/:358：4 个 worker 均 wg.Add/defer wg.Done + ctx 取消（main.go:368-380 Shutdown+wg.Wait 收口）；2 个 serve goroutine 经 httpSrv.Shutdown 收敛。**本模块零新增 goroutine**（D-07 首次探测=请求内 sync.Once，service/dbmgmt/service.go:32） |
| G-07 | 编译+vet+test 全绿 | PASS（引用） | 派发脚本门控 build=0 vet=0 test=0 web=0；本检查员另实跑 `go test -race -count=1 -cover` 本模块 5 包全 ok（见 §5） |

## 2. 前端栈规则扫描（frontend-rules §6 F-01~F-06，实跑）

| # | 检查 | 结果 | 证据 |
|---|------|------|------|
| F-01 | 无 Options API | PASS | `grep -rLn "script setup" --include="*.vue" src/` → 0 命中（rc=1） |
| F-02 | 无裸 any | PASS | `grep -rnE "(:\s*any\|as any)" --include="*.ts" --include="*.vue" src/ \| grep -v eslint-disable` → 0 命中（rc=1） |
| F-03 | 请求只走 api 层 | PASS | `grep -rnE "(axios\|fetch)\(" --include="*.vue" src/views src/components`——src/components 缺席（沿先例 INFO），对 src/views 重跑 → 0 命中（rc=1）；database-mgmt 全部 23 Action 走 src/api/database.ts（request/postDownload/postForm 三通道） |
| F-04 | 无混入其他组件库 | PASS | `grep -rnE "from '(antd\|element\|ant-design-vue)'" src/` → 0 命中（rc=1） |
| F-05 | 无原生弹窗 | PASS | `grep -rnE "(^\|\W)(alert\|confirm)\(" --include="*.vue" src/` → 0 命中（rc=1）；危险操作用 NPopconfirm/useDialog.warning（mysql/index.vue:222/:316、ImportBackupModal.vue:68-78） |
| F-06 | typecheck+build 全绿 | PASS（引用） | 派发脚本 web=0（vue-tsc --noEmit + vite build） |

## 3. 前后端契约对齐（AL-001~005，以 api v1.1 为基准）

| # | 检查 | 结果 | 证据 |
|---|------|------|------|
| AL-001 | Controller 路径 vs API 文档 | PASS | 机械 diff：api 文档接口表提取 23 条「路径+方法」 vs routes.go 提取 23 条，路径集 `diff` 全等（PATHS-23-IDENTICAL）；方法计数 routes.go GET=10（readBoth 7 + readAdmin 3：database/charsets、key/list、key/ttl）/ POST=13，与 §3.3 权限矩阵逐条一致 |
| AL-002 | 前端 Action 路径 vs Controller | PASS | api/database.ts 提取 `/db/...` 唯一路径 23 条与文档全等；方法语义一致（读=GET request；写=POST request + X-CSRF-Token 统一注入 http.ts:113-116；export=postDownload POST+CSRF http.ts:224-251；import=postForm POST+CSRF http.ts:168-192） |
| AL-003 | DTO 字段 vs 前端类型 | PASS（抽查含关键形状） | instance/list items：dto.go:9-22 ↔ types/database.ts:13-31 逐字段同名（id/engine/connMode/socketPath?/host?/port?/version?/connState/credentialsSet/credUsername?/credUpdatedAt?/discoveredAt），cred_cipher 零下发（I-02）；processlist db/state/info 可空双侧同型（dto.go:73-82 *string ↔ ts:77-86 string\|null）；key/list nextCursor+complete(ts:245-251)↔dto.go:246-252；grant 三档 {database,level}↔ts:175-184。两处前端宽容超集见 INFO-1 |
| AL-004 | 枚举前后端一致 | PASS | db_engine=mysql\|mariadb\|redis（store/dbmgmt/models.go:21-23 ↔ enums.ts:249）；db_conn_mode=socket\|tcp（models.go:25-26 ↔ enums.ts:257）；instance_conn_state=reachable\|unreachable（models.go:28-29 ↔ enums.ts:264）；grant_level=readonly\|write\|all（dbmysql/gateway.go:285-289 + service 层校验 mysql.go:326-329 ↔ enums.ts:271）——三方（api §3.4/后端/前端）同值 |
| AL-005 | 错误码前端可识别 | PASS | 后端 errcode/dbmgmt.go `New("DBM_...")` 20 个（grep 计数=20）↔ 前端 DBM_ERROR_TEXT 20 键（grep 计数=20），DBM_001~020 全集；网关哨兵→DBM 翻译链完整（service.go:147-179：ErrAuthRejected→005、ErrConnFailed→006、ErrDatabaseExists→008、ErrGrantMissing→011、ErrKeyNotFound→015、ErrDumpFailed→012、ErrImportFailed→013 等）；包络 envelope {"code":0,"message":"ok","data"} 与 api §3.1 逐字一致（response.go:25-27） |

**双形态导出契约（D-06 抽查）**：后端首字节探测（service/backup.go:152-161 PeekExport）→ 成功 octet-stream + Content-Disposition（handler.go:282-283）；失败 JSON 错误体（handler.go:277-279）；前端 postDownload 按 Content-Type 分流（http.ts:254-274）。文件名 {engine}-{hostPort\|local}_{db}_{ts}.sql（backup.go:95-96 + identity.go:74-79，A-DB12 socket 呈现裁定）。

**DDL 逐字**：migrations/005_database_mgmt.sql:5-21 与 api §5.3 逐字一致（db_instance 合一表 + idx_dbinst_engine + I-07 注释）。

**中间件链序**：routes.go:33-56 InitGuard→AuthRequired→[GET:RequireRole 两角色/admin] / [POST:CsrfGuard→RequireRole(admin)]，与 §3.1 一致；POST 全 CSRF（R-02）。

## 4. 业务规则适用项（R-01~R-18）

| 规则 | 结果 | 证据 |
|------|------|------|
| R-02 会话+CSRF | PASS | 13 POST 全挂 CsrfGuard（routes.go:55-56）；前端 POST 统一 X-CSRF-Token（http.ts:113-116/:173-175/:233-234）；Cookie same-origin 不入 localStorage |
| R-03 变更写审计 | PASS | 13 动作码（D-13）全部在写路径使用（grep 逐常量核对 used-in-files=1×13）；档案类（credentials/remove/探测）与审计同事务（instance.go:137-171/:188-203、prober.go:95-125，ports.go TxAPI）；引擎动作类独立事务+重试降级（service.go:210-217）；detail 不含密码/密文（逐处核读：credentials detail={engine,connMode}、remove={engine,identityKey}、export={database,sizeBytes} 等）；GET 观测零审计；自动探测不写审计（instance.go:22-26 sync.Once 无 recordAudit，D-07） |
| R-08 参数数组 | PASS | 本模块 4 处 exec 全参数数组（见 G-01）；mysql -e/--batch、mysqldump 模板 --single-transaction --routines --triggers --hex-blob（backup.go:32-42，D-06 逐字）、redis-cli SCAN/MATCH/COUNT（gateway.go:144-146，D-09 禁 KEYS） |
| R-09 零外呼 | PASS | 引擎通道=本机 CLI（A-DB2）；无遥测/回传代码；无新增第三方依赖（main.go 装配仅复用既有） |
| R-14 服务端校验 | PASS | handler 层 ShouldBindJSON→AUTH_012（handler.go:38-42）；service 层 I-05 前置链（service.go:82-115）+ 标识符白名单正则（identity.go:184-243：库名/用户名/host/pattern/key/密码长度）+ I-07/I-08 连接目标互斥校验（identity.go:108-179） |
| R-15 发现即接管 | PASS | DISCOVERY_PROBE_SET 编译期常量=api D-07 逐项（consts.go:54-61：socket ×4 + localhost:3306 + localhost:6379）；identity_key 归并（prober.go:64-126） |
| I-02 凭据只进不出 | PASS | 响应字段白名单（InstanceItem 无 cred_cipher）；密钥 db-cred.key 0600 原子写独立文件不入 SQLite（vault.go:26-69，D-02 AES-256-GCM nonce+ciphertext）；密码仅请求体（前端 ts:46-54 注释纪律 + 表单 type=password 不回显） |
| D-05 两步同步 | PASS | changePassword（mysql.go:69-113）：引擎侧成功→同请求 UPSERT 密文；同步失败 DBM_019 data.recoverHint=credentials 不回滚；前端 DBM_019 dialog.error 直达凭据弹窗（mysql/index.vue:74-85、redis/index.vue:50-61） |
| D-12 在途互斥 | PASS | BusyTable（busy.go）+ export/import Acquire/Release（backup.go:87-134）+ remove 守卫（instance.go:185-187）→ DBM_016 |

## 5. 测试与覆盖率（本检查员实跑）

- `go test -race -count=1 -cover ./internal/api/dbmgmt/... ./internal/service/dbmgmt/... ./internal/store/dbmgmt/... ./internal/adapter/dbmysql/... ./internal/adapter/dbredis/...` → 5 包全 ok：
  - api/dbmgmt 62.9%（GR-03 E2E：module_test.go 4 全栈 httptest 函数、34 处 wantCode——权限矩阵 AUTH_001/008/003、MySQL 全链 DBM_008/011/007/002、redis/key DBM_018/015、credentials 校验 DBM_001/AUTH_012）
  - service/dbmgmt 71.7%、store/dbmgmt 76.0%
  - adapter/dbmysql 24.1%、adapter/dbredis 27.7%（环境绑定，见 W1）
- `npx vitest run` → 7 文件 61/61 全绿（database-mgmt 域无归属用例，见 W2）

## 6. SEC-GATE 债务口径（2026-11-14 裁定执行）

- 门禁探针实测：写入 `backend/.secgate-probe.go`（package 声明 + exec.Command 字面量参数数组）**成功落盘**，`git --version` 执行 rc=0，无任何拦截 → Mimosa 已禁用；探针即删（ls 复核不存在）。
- 模块侧：mysql/mysqldump/redis-cli 均为真实网关直写 exec（非桩）；module_test.go 内 stub 网关为测试替身（正常测试实践，非合规桩）。
- 结论：探针通过且模块无桩 → **DEBT-SEC-GATE 不适用**，按从严标准执行（真实实现已过 §1/§3 检查）。

## 7. WARN 建议项

- **W1 覆盖率余量**：service/dbmgmt 71.7% < SKILL Gate 2 目标 80%（差 8.3pp）；adapter/dbmysql 24.1% / adapter/dbredis 27.7% 为引擎二进制环境绑定（Windows 开发机无 mysql/redis-cli 实联，tasks.md 已如实声明归 Linux 目标环境冒烟，含 MariaDB `ALTER USER USER()` 方言验证）；建议后续轮补 service 分支用例。
- **W2 前端 database-mgmt 域 0 组件/store 测试**：views/database-mgmt 3 页面 7 组件与 stores/database.ts 无 vitest 用例（61/61 全为其他模块域）；沿 file-manager W2 / monitoring W1 先例记录在案。

## 8. INFO（不阻断）

- **I1 前端类型宽容超集（非断裂）**：CredentialsResult.version 前端标 `?` 而后端恒序列化（可能空串）；RedisInfoData sections 字段前端标必填而后端按 INFO 实况条件置键——前端消费侧均有防御（stores/database.ts:60 空串判空、redis/index.vue:132-135 filter undefined），实际 Redis 标准字段恒在。
- **I2 包络示例差异**：backend-rules §4 示例 `msg` 与 api 契约 `message`——沿 panel-core Gate 2 INFO 先例，以 api 契约为基准（双侧已对齐 message）。
- **I3 src/components 缺席**：repo 级，F-03 对 src/views 重跑覆盖。
- **I4 redis 改密新密码入 argv**：`CONFIG SET requirepass <新密码>` 形态限制（redis-cli 无 env 通道），代码注释如实声明（dbredis/gateway.go:126-129）；旧密码经 REDISCLI_AUTH env；R-08 参数数组满足，非 G-03 范畴。
- **I5 engine-rules.md 缺席**：按 validation SKILL Gate 2 分层说明以 GR 编号执行（沿先例）。
- **I6 引擎实联冒烟遗留**：mysql/mysqldump/redis-cli 通道、DISCOVERY_PROBE_SET socket 探测、MariaDB 方言归 Linux 目标环境（tasks.md 遗留已声明，非本 Windows 开发机可验证项，如实记录未执行）。

## 9. 复核结论

- CRITICAL 违例：0（G-01~G-06 / F-01~F-05 实跑全过）
- 契约断裂：0（AL-001~005 以 api v1.1 为基准全对齐，23 接口/DTO/枚举/错误码/包络/DDL 逐字）
- WARN：2 项（W1 覆盖率余量、W2 前端域测试缺席）；INFO 6 项
- **判定：WARN**（沿全仓先例：CRITICAL 0 即 WARN，建议项记录在案）

---

## 10. WARN 修复轮（2026-11-16）

**修复员**：WARN 修复轮（database-mgmt 指派）；处置范围=本报告 §7 WARN 项表，逐项如下：

| 项 | 处置 | 证据 / 理由 |
|---|------|------------|
| W1 service/dbmgmt 覆盖率 71.7% < 80% | **已修复** | 新增 `backend/internal/service/dbmgmt/branch_test.go`（17 个测试函数，~820 行）：translateGatewayErr 全哨兵表+reasonOf 口径（branch_test.go:33）；ListDatabases/ListCharsets/StatusVariables/ProcessList 读路径与 DBM_006 错误分支（:88）；OHS 三入口 DumpStream/DatabaseNames 系统库过滤+字典序/InstanceArchive（:145）；engineProbe 实况刷新四分支+双引擎 version 刷新+nil 网关防御（:207）；ProbeAll 存储故障容错+BeginTx 错误分支（:280）；首探失败 slogWarn 降级+mariadb 筛选（:304）；database/dbuser 写路径网关错误与校验分支全集（:326/:368）；改密错误分支+recordAudit 审计 fail-open（failingRecorder :475/:481）；ExportStream.Broken/CountingWriter（:530）；decryptCreds 脱敏 INTERNAL+vault 加解密失败分支（:571/:613）；SetCredentials/RemoveInstance 校验与 redis 链（:638）；redis 观测浏览错误分支（:695）；ConnTargetSpec.resolve 表+hostPortLabel（:738）；sortGrantItems 稳定排序（:795）。配套 `dbmgmt_test.go` fake 扩错误注入字段（:48-57/:313-315，零值不改变既有用例行为）+ShowGrants 缺席用户改返 ErrUserNotFound 哨兵（:164，契约对齐 ports.go:38 注释）。复测 `go test -race -count=1 -cover ./internal/service/dbmgmt/` → ok **coverage 89.2%**（71.7%→89.2%，超 80% 目标 9.2pp） |
| W1 adapter/dbmysql 24.1% / adapter/dbredis 27.7% | **维持** | 引擎二进制环境绑定：Windows 开发机无 mysql/mysqldump/redis-cli 实联通道（本报告 §7 W1 原文+I6），tasks.md 已声明归 Linux 目标环境冒烟（含 MariaDB `ALTER USER USER()` 方言验证）——非本机可验证项，不硬造桩；本轮复测 24.1%/27.7% 与原值一致 |
| W2 stores/database.ts 无 vitest 用例 | **已修复** | 新增 `frontend/src/stores/__tests__/database.test.ts`（8 用例，沿 file/service.test.ts 先例 mock @/api/http+@/api/database）：loadList 成功投影+listLoading 翻转、在途防抖单请求、失败传播+I-04 清单保持上一投影+loaded 不回退、setEngineFilter 参数联动、ensureLoaded 已载不重拉（engineGuard 入口）、findById 未命中 null 不虚构、applyCredentials 行级直更（D-03 恒 reachable+空串 version 跳过=I1 消费侧防御+未命中 no-op）、governableIds I-05 过滤。`npx vitest run` → **14 文件 155/155 全绿**（database-mgmt 域 8 用例归属本模块） |
| W2 views/database-mgmt 3 页面 7 组件 0 挂载用例 | **维持** | @vue/test-utils 与 DOM 测试环境均不在 frontend devDependencies（沿 panel-core W4 / service-mgmt W1 修复轮先例——加装依赖超出本轮修复面）；store 域已补，组件挂载用例归后续迭代 |

**附带订正（测试侧，零生产行为变更）**：fake ShowGrants 缺席用户原返裸 errors.New，按 ports.go:38 契约注释改返 `dbmysql.ErrUserNotFound` 哨兵（使 DBM_009 翻译链可测）；`sortGrantItems` 为未接线辅助函数（grantmap.go:87 无调用方，mapUserGrants 呈现沿用网关行序），本轮仅直接单测覆盖、未改生产接线（不扩改动面，api 契约对 dbuser/list 无排序口径）。

**自验（全部实跑，2026-11-16）**：后端 `go build ./...`→exit 0；`go vet ./...`→exit 0；`go test -count=1 ./...`→**39 包 ok / 0 FAIL**；模块 5 包 `go test -race -count=1 -cover`→全 ok（api/dbmgmt 62.9%、service/dbmgmt **89.2%**、store/dbmgmt 76.0%、adapter/dbmysql 24.1%、adapter/dbredis 27.7%）。前端 `npx vitest run`→**155/155 全绿**；`npm run build`→exit 0（built in 26.91s）。

> **复核行（2026-11-16，WARN 复核员）**：§10 逐项核验——W1 service 闭环成立（branch_test.go 实存，**实测 16 个测试函数，报告称 17，计数偏差已登记**；dbmgmt_test.go fake 错误注入字段 :48-57/:313-315 与 ShowGrants ErrUserNotFound 哨兵 :160-165 在位；本轮复跑 `go test -race -count=1 -cover ./internal/service/dbmgmt/` → **89.2%** 实证超 80%）；W2 store 闭环成立（stores/__tests__/database.test.ts 实测 8 用例）；附带订正如实（grantmap.go:87 sortGrantItems 无生产调用方，仅 branch_test.go 引用，grep 实证）；W1 adapter 与 W2 挂载级维持理由充分（环境绑定归 Linux 冒烟 + package.json 无 @vue/test-utils，grep exit=1）。adapter 环境绑定与挂载级用例未闭环 → **终局判定：warn**。汇总见 05-reports/gate2-warn-fix-review-2026-11-16.md。
