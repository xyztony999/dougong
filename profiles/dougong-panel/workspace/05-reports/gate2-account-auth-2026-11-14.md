# Gate 2 验证报告 — 代码→交付门控（account-auth）· R3 独立重验

| 属性 | 值 |
|------|-----|
| 模块 | account-auth（账户与安全基座） |
| 检查时间 | 2026-11-14（第 4 次派发：首轮 FAIL → R1 WARN → R2 WARN → 本轮 R3 独立重验，全部结论本轮重新取证） |
| 扫描路径 | backend/ 本模块相关：internal/{api/auth, api/middleware, api/response, errcode/auth.go, service/auth, service/audit, store/auth} + store/migrations/001_account_auth.sql + cmd/dougong/main.go（装配段）；frontend/src/ 本模块相关：api/{auth,http}.ts、types/{api,auth,enums}.ts、stores/auth.ts、views/account-auth/**（5 页面 + 5 局部组件）、router/ 追加段、utils/{errcode,audit,totp}.ts |
| 契约基准 | workspace/02-detailed/api/api-account-auth.md v1.1（18 HTTP 接口 + 4 OHS + 5 表 DDL + 8 枚举 + 15 动作码 + AUTH_001~014 符号码） |
| 规则链 | 引擎 GR（validation SKILL.md Gate 2 节：扫描必执行/编译必过/E2E 必补/前后端契约对齐）⊕ 栈规则 backend-rules §7 G-01~G-07 / frontend-rules §6 F-01~F-06 ⊕ 业务规则 R-01~R-18 适用项 |
| 编译门控 | 引用本 ask 提供的脚本结果（未重跑）：build=0、vet=0、test=0、web=0；本轮另实跑 `go test -count=1 -cover`（4 包 ok，覆盖率实测）与 `npx vitest run`（51/51 过） |

## 判定：WARN

- 🔴 CRITICAL 违例：**0 项**（后端 G-01~G-06、前端 F-01~F-05 本轮实跑 grep 全 0 命中）
- 契约断裂：**0 项**（AL-001~AL-005 全对齐，DDL 与契约 §5.3 逐字一致）
- WARN：3 项（W1 覆盖率余量、N1 过期注释 7 处、W4 src/components 缺席），均不阻断
- SEC-GATE 债务口径（2026-11-14 裁定）：**DEBT-SEC-GATE 不适用于本模块**——本轮门禁探针 `git --version` 直接执行成功（exit=0，未被拦），且模块内无任何『外部命令合规桩』（stub/TODO 专项扫描仅命中测试 clockStub 合法替身），按裁定无需判 WARN 债务、亦无『探针已过仍留桩』的从严触发项

### 轮次沿革

| 轮次 | 判定 | 驱动项 |
|------|------|--------|
| 首轮（2026-11-14） | FAIL | E1 错误码通道（数值 401NN vs 契约符号）、E2 审计清单 PascalCase 键 |
| 复检 R1（2026-11-14） | WARN | E1/E2 修复核验通过；遗留 W1/N1/W4 |
| R2（2026-11-14） | WARN | 全量独立重验通过，W1/N1/W4 维持 |
| **本轮 R3（2026-11-14）** | **WARN** | 独立重验：栈扫描/契约/业务规则/覆盖率/SEC-GATE 探针全部重新实跑取证，结论与 R2 一致 |

---

## 一、前轮契约修复保持核验（本轮重新取证）

1. **E1（AL-005）错误码符号通道：在位**。`backend/internal/errcode/auth.go:5-33` 全部错误以符号声明（AUTH_001~AUTH_014 + NOT_FOUND/INTERNAL），文件头注明「symbol 即契约 code 列原值，直接经响应包络 code 字段上线」；`internal/api/response/response.go:18-22` Envelope.Code 为 `any`，`OK`（:25-27）成功恒数值 0，`Fail`（:31-40）/`FailWith`（:43-45）输出 `ec.Symbol()` 符号串。middleware 全链走符号通道（本轮通读：authguard.go:79/87/92、csrf.go:22/28/34、role.go:19/24、recovery.go:21）。测试锁定：`internal/api/auth/module_test.go` 内 `grep -c wantCode` = 40 处符号断言调用点 + `internal/errcode/errcode_test.go`（本轮 `go test` 全 ok 佐证）。前端 `api/http.ts:153-160` 以 `normalizeCode(env.code)` 包络边界归一（符号直通），16 处 `is('AUTH_xxx')`/分流集比对本轮逐一 Read 在位（http.ts:70-103、stores/auth.ts:53-54、login/index.vue:100-118、audit-log-list/index.vue:187-191、UserCreateModal.vue:73-78、init-wizard/index.vue:103-111、TotpBindModal.vue:60、TotpDisableModal.vue:39）。
2. **E2（AL-003）审计清单 camelCase 键：在位**。`backend/internal/store/auth/models.go:100-115` AuditRecord 带全量 camelCase json tag（id/actorName/category/action/objectType/objectId/detail/result/source/createdAt），`ActorID` 标 `json:"-"`（契约 §4.3.4 items 无此键）；handler 原样输出 `gin.H{"items", "total"}`（api/auth/handler.go:301）。测试锁定：module_test.go:210 调用 assertAuditItemsCamelCase（定义 :263-288，:279 banKeys 禁 PascalCase 全集）。前端 `utils/audit.ts` normalizeAuditItem/normalizeAuditListResult 为双通道容错垫片（camelCase 直通），`__tests__/audit.test.ts` 5 用例随本轮 vitest 51/51 实跑通过。

## 二、后端规则扫描（backend-rules §7 G-01~G-07，本轮实跑 grep，cwd=backend/）

| # | 检查项 | 结果 | 证据（命令 + 输出） |
|---|--------|------|---------------------|
| G-01 | 无 shell 拼接 | PASS | `grep -rnE 'exec\.Command(Weak)?Context?\(.+\"(-c\|/c)\"' --include="*.go" .` → 0 命中（exit=1）；全量 `grep -rn "exec\.Command" --include="*.go" . \| grep -v _test` → 唯一命中 `internal\adapter\nginxx\parser.go:3` 为注释文本（site-mgmt 范畴，非调用、非本模块）；本模块 9 包零 exec/shell 调用 |
| G-02 | 无 SQL 拼接 | PASS | `grep -rnE 'Sprintf\(.+(SELECT\|INSERT\|UPDATE\|DELETE)' --include="*.go" .` → 0 命中（exit=1）；store 层人工复核（本轮通读 store/auth 全部 5 仓储）：语句全部 `?` 参数绑定；auditWhere（audit_repo.go:43-75）条件串为静态字面量拼接、值一律参数绑定、LIKE 通配符 escapeLike（:77-80）转义 |
| G-03 | 无硬编码凭据 | PASS | `grep -rnEi '(password\|secret\|token)\s*=\s*"[^"]{8,}"' --include="*.go" . \| grep -v _test` → 0 命中（exit=1）；CLI 口令走环境变量注入（cmd/dougong/main.go:336 flag 说明「推荐改用 DOUGONG_INIT_PASSWORD 环境变量注入」，本轮 Read 实证） |
| G-04 | SQL 只在 store 层 | PASS | `grep -rlnE "(SELECT\|INSERT\|UPDATE\|DELETE) " --include="*.go" . \| grep -v internal/store \| grep -v _test` → 命中 service/auth/user.go 等 3 个 service 文件，逐一 `grep -n` 核对全部为注释文本：`internal/service/auth/user.go:114`「FK ON DELETE CASCADE」、fileman scope.go:20 / trash.go:36/:81（他模块注释）；实际 SQL 全部位于 internal/store/**（本模块 store/auth 五仓储 + 基座 migrate.go） |
| G-05 | 无 panic 滥用 | PASS | `grep -rn "panic(" --include="*.go" . \| grep -v _test` → 0 命中（exit=1）；handler panic 由 Recovery 中间件兜底（middleware/recovery.go:16-23，CustomRecovery→ErrInternal 符号） |
| G-06 | goroutine 有退出路径 | PASS | `grep -rn "go func" --include="*.go" . \| grep -v _test` → 5 处全在 `cmd/dougong/main.go:213/228/243/281/287`；本轮通读逐处核对：:213 审计清理 worker、:228 monitoring collector、:243 file chunk sweeper（均 ctx 取消退出 + defer wg.Done + panic 隔离 recover）、:281/:287 TLS/plain serve（`errors.Is(err, http.ErrServerClosed)` 不入错）；main 统一编排 `httpSrv.Shutdown` 10s 超时（:304-308）+ `wg.Wait()`（:309）；internal/ 各层零自起 goroutine；ticketStore 以签发时惰性清扫替代常驻清理 goroutine（service/auth/ticket.go:9-11 设计注释） |
| G-07 | 编译+vet+test 全绿 | PASS（引用） | 本 ask 提供脚本结果 build=0/vet=0/test=0（未重跑）；本轮另实跑 `go test -count=1 -cover ./internal/api/auth/ ./internal/errcode/ ./internal/service/auth/ ./internal/service/audit/` → 4 包全 ok（见第六节） |

## 三、前端规则扫描（frontend-rules §6 F-01~F-06，本轮实跑 grep，cwd=frontend/）

| # | 检查项 | 结果 | 证据（命令 + 输出） |
|---|--------|------|---------------------|
| F-01 | 无 Options API | PASS | `grep -rLn "script setup" --include="*.vue" src/` → 0 命中（exit=1；本模块 10 个 .vue 全 `<script setup lang="ts">`，本轮逐一 Read 实证） |
| F-02 | 无裸 any | PASS | `grep -rnE '(:\s*any\|as any)' --include="*.ts" --include="*.vue" src/` → 0 命中（exit=1）；错误 data 用 unknown + 类型守卫（types/auth.ts:194-229 isFieldErrorList/extractRemainingSeconds/extractLoginTicket/fieldErrorsToText） |
| F-03 | 请求只走 api 层 | PASS | 规则命令对 `src/components` 报目录不存在（ugrep exit=2，见 W4）；重跑 `grep -rnE "(axios\|fetch)\(" --include="*.vue" src/views` → 0 命中；追加全量 `--include="*.vue" src/` → 0 命中；fetch 仅出现于 api 层（api/http.ts:120/178/204 统一出口 + api/auth.ts:141 CSV 导出流），符合 §2/§3「src/api 即唯一出口」 |
| F-04 | 无混入其他组件库 | PASS | `grep -rnE "from '(antd\|element\|ant-design-vue)'" src/` → 0 命中（exit=1；Naive UI 唯一，n-* 组件） |
| F-05 | 无原生弹窗 | PASS | `grep -rnE "(^|\W)(alert\|confirm)\(" --include="*.vue" src/` → 0 命中（exit=1；useMessage/notify.ts；危险操作 NPopconfirm 确认——user-list:69-79、session-list:48-63） |
| F-06 | typecheck + build 全绿 | PASS（引用） | 本 ask 提供脚本结果 web=0（未重跑） |

🟡 前端补充（本轮 Read 取证）：锁定倒计时 setInterval 配对 onUnmounted 清理（login/index.vue:62-80 stopLockCountdown）；csrfToken/loginTicket 仅内存态 Pinia state（stores/auth.ts:17-19），R-02 纪律成立。

## 四、前后端接口对齐（AL-001~AL-005，以 api-account-auth.md v1.1 为基准）

| # | 检查项 | 结果 | 证据（本轮逐一比对） |
|---|--------|------|---------------------|
| AL-001 | Controller 路径 vs API 文档 | PASS | `internal/api/auth/routes.go:32-62` 注册 18 路由与文档 §3.3 权限矩阵逐行一致：公开 4（GET init/check、POST init/wizard、POST session/login、POST session/totp-verify；InitGuard 豁免面 authguard.go:40-43 与文档一致）；会话 GET 5（session/current、session/list、user/list、audit-log/list、audit-log/export[admin :49]）；会话+CSRF POST 9（logout、revoke[admin :54]、password-change、totp-setup/enable/disable、user/create\|delete\|reset-password[admin :59-61]）；读 GET/写 POST 与文档 §3.1 一致 |
| AL-002 | 前端 Action 路径 vs Controller 路径 | PASS | `frontend/src/api/auth.ts` 18 函数与 routes.go 路径逐一相同（initCheck:35、initWizard:40、login:50、totpVerify:58、sessionCurrent:67、logout:72、sessionList:76、sessionRevoke:80、userList:86、userCreate:90、userDelete:94、userResetPassword:98、passwordChange:102、totpSetup:106、totpEnable:110、totpDisable:114、auditList:124、exportAuditLog:141），GET/POST 方法一致 |
| AL-003 | DTO 字段 vs 前端 VO 字段 | PASS | `service/auth/dto.go` ↔ `types/auth.ts` 逐字段一致（camelCase）：login {username,password}（code 废弃字段双侧均无，D-02）、totp-verify {loginTicket,code}、create {username,password,role}→{id,username,role,createdAt}、reset {userId,newPassword}、password-change {oldPassword,newPassword}、delete {userId}、totp-enable {code}、totp-disable {password}、setup→{otpauthUri,secret}、init/check {initialized,allowWizard}、wizard {username,password,totpSecret?,totpCode?}、current {authenticated,user,csrfToken}、user/list items{id,username,role,status,totpEnabled,lockedUntil,createdAt}（dto.go:53-61）、session/list items{id,username,createdAt,lastActiveAt,sourceIp,current}（dto.go:43-50；后端 SourceIP nil→""，前端 string\|null 为超集宽容）；audit-log/list 条目键 camelCase（models.go:103-115，E2 修复保持）+ data={items,total}（handler.go:301）；audit query 十键与 handler.go:430-475 parseAuditQuery 逐键一致（含 Gate 1 W3 的 objectId 等值匹配 :470）；无分页接口（user/list、session/list）契约 data 仅 items，视图零消费 total |
| AL-004 | 枚举值前后端一致 | PASS | 8 枚举三方同值：panel_role/panel_user_status（store/auth/models.go:19-25）、session_end_reason（:28-32）、audit_action/audit_result/audit_source（service/audit/audit.go:18-34）= 契约 §3.4 = 前端 types/enums.ts:17-73；15 个审计动作码逐字一致（audit.go:37-53 ↔ enums.ts:250-265 AUDIT_ACTION_TEXT 前 15 项） |
| AL-005 | 错误码前端可识别 | PASS | 契约 §3.4 code 列 AUTH_001~014 符号 = 后端 errcode/auth.go:7-33 符号声明（经 response 包络上线）= 前端 normalizeCode 归一 + 16 处符号比对（E1 修复保持）；AUTH_004 data.remainingSeconds（login.go:161-165 ↔ extractRemainingSeconds types/auth.ts:208-214）、AUTH_007 data.loginTicket（handler.go:79-81 ↔ extractLoginTicket :217-223）、AUTH_012 data.fields[]（errcode FieldError ↔ isFieldErrorList :194-205）双侧结构一致 |

**DDL 对齐**：`backend/internal/store/migrations/001_account_auth.sql` 与 api 文档 §5.3 逐字一致（5 表 panel_user/panel_user_totp/panel_session/audit_log/init_state + 5 索引，含全部 CHECK/COLLATE NOCASE/ON DELETE CASCADE），本轮 cat 全文逐行比对；文件头注明「唯一事实源，逐字落库，禁止自行增删列」。

**包络对齐**：`{"code","message","data"}` 字段名双侧一致（response.go:18-22 ↔ types/api.ts:7-11；成功=数值 0/失败=符号串，前端 `code: number | string` 联合类型兼容）；统一 HTTP 200 + 业务 code（D-07）；CSV 导出 200 流 + UTF-8 BOM（handler.go:330）+ Content-Disposition attachment（:325-327）+ 100,000 行上限超限 AUTH_012 拒绝（:313-324，MaxExportRows=100000 audit.go:67）；导出中止保护（Flush 后 w.Error() 检查 + [EXPORT ABORTED] 标记 :354-365）。

## 五、业务规则核对（R-01~R-18 适用项，本轮代码取证）

| 规则 | 结果 | 证据 |
|------|------|------|
| R-01 登录失败锁定 5 次/15 分钟 | PASS | consts.go:26-29（MaxFailAttempts=5、LockoutDuration=15min 编译期常量）；login.go:171-207 recordLoginFailure（fail_count+1 → 达 5 落 locked+account_locked 审计同事务）；login.go:134-166 惰性解锁+归零+account_unlocked 审计；AUTH_004+remainingSeconds（:161-165）；锁定期间不计失败（resolveLockState 先行返回）；成功/TOTP 票据签发前清零（login.go:69-74、service.go:129-134） |
| R-02 HttpOnly Cookie 会话 + CSRF，token 不入 localStorage | PASS | handler.go:397-407 setSessionCookie（HttpOnly+SameSiteStrict+Secure 开关+Max-Age=绝对过期）、:410-420 登出清 Cookie；csrf.go:18-59（X-CSRF-Token subtle.ConstantTimeCompare 常量时间比对 + Origin/Referer 同源双校验，二者全缺拒绝）；POST 豁免面=公开组 4 接口（routes.go:36-40 无 CsrfGuard）与文档 §3.3 一致；前端 csrfToken/loginTicket 仅内存态（stores/auth.ts:17-19）、POST 统一注入头（http.ts:113-116） |
| R-03 全变更审计（谁/何时/改了什么/结果） | PASS | 全部写路径同事务落审计（本轮通读逐一核对）：user_created/user_deleted（user.go:74-79/:121-124）、password_reset/password_changed（:153-156/:186-189）、totp_enabled/totp_disabled（:236-238/:262-264）、logout/force_logout（session.go:91/:141-143）、init_completed（init.go:68-78/:131-141）、login_success/login_failed/account_locked/account_unlocked（login.go+service.go:126-185）、audit_exported（handler.go:369-380）、audit_purged（audit.go:159-196）；四要素强制校验 validateEntry（audit.go:200-220，I-15）；withTx 原子性（service.go:74-89，I-05） |
| R-08 命令执行参数数组 | PASS（无命中） | 本模块零 exec.Command 调用（G-01 全量+模块专项扫描 0 命中，R-08 合规-by-absence） |
| R-09 零遥测零外呼 | PASS | 模块包内 `grep -rnE "http\.Get\|http\.Post\|http\.Client\|http\.NewRequest"` → 仅 module_test.go 3 处（httptest 本地测试服务器请求，合法），非测试 0 命中；无 MQ/事件总线（D-05） |
| R-10 监听地址配置注入 | PASS | 模块包内 `grep -rn "0\.0\.0\.0"` → 0 命中（main.go panelListenAddr 为 panel_config 枚举表明文映射，非本模块业务代码） |
| R-14 服务端全量校验 | PASS | handler 层 binding tag（各 ShouldBindJSON + bindError→AUTH_012 handler.go:423-427）；service 层语义校验 validation.go:12-42（用户名 ^[a-zA-Z0-9_.-]{1,32}$=I-10、密码 ≥8=A10+72 字节上限、role 枚举）；TOTP 成对校验（init.go:155-157）；AUTH_012 fields[] 逐字段回执 |
| R-18 Web 终端复用面板会话鉴权 | PASS | SessionAuthService.VerifyWs OHS 已实现（session.go:57-82）：同一 I-11 判定（revoked/绝对/空闲三查）、只读不续期（无 Touch 调用）、返回 (userID, role)——terminal 模块契约就绪 |

不适用（归其他模块）：R-04/R-05（file-manager）、R-06/R-07/R-16（site-mgmt）、R-11/R-12/R-13（cron-backup/docker-mgmt）、R-15（存量发现）、R-17（file-manager ADR-021）。

## 六、SEC-GATE 债务口径核查（2026-11-14 裁定）

1. **门禁探针**：本轮实跑 `git --version` → `git version 2.56.1.windows.1` exit=0——外部命令未被拦截（Mimosa 未对本运行拦截）。
2. **模块留桩检查**：account-auth 前后端产物 `grep -rniE "stub|合规桩|TODO|FIXME|not implemented|unimplemented"` → 仅命中 auth_test.go/audit_test.go 的 clockStub（测试时钟替身，合法），无任何外部命令合规桩。
3. **tasks.md 佐证**：backend/tasks.md 通读 grep——无本模块「外部命令合规桩」阻塞记录；仅 :61 记录 Mimosa 扫描对 totp.go SHA1（RFC 4226 规定算法）与测试口令字面量两处提示的处置（均为注释说明/常量收敛，非桩）。
4. **结论**：探针已通过 + 模块无桩 → DEBT-SEC-GATE 不适用，且无『探针已过仍留桩按未实现功能从严』触发项；其余检查照常从严（结果见上）。

## 七、测试覆盖率（本轮实跑，2026-11-14）

| 层级 | 实测 | 目标 | 达标 |
|------|------|------|------|
| 后端 Service 层（internal/service/auth） | 79.4% | 80%+ | **NO（差 0.6pp → W1）** |
| 后端 Service 层（internal/service/audit） | 82.4% | 80%+ | YES |
| 后端 Controller 层（internal/api/auth，HTTP 集成测试） | 71.0% | 60%+ | YES |
| 后端 errcode | 100% | — | — |
| 前端组件/composables/工具 | vitest 51/51 全过（6 文件；本模块归属 errcode 4 + audit 5 + totp 3 = 12 用例）；views/account-auth 组件级 0 测试 | 60%+ | **NO（→ W1）** |

实跑命令与输出（本轮）：`go test -count=1 -cover ./internal/api/auth/ ./internal/errcode/ ./internal/service/auth/ ./internal/service/audit/` → `ok api/auth 0.688s coverage: 71.0%`、`ok errcode 0.034s coverage: 100.0%`、`ok service/auth 2.053s coverage: 79.4%`、`ok service/audit 0.064s coverage: 82.4%`；`npx vitest run`（cwd=frontend/）→ `Test Files 6 passed (6)，Tests 51 passed (51)`。

E2E（引擎 GR「E2E 必补」）：后端有 HTTP 层集成测试 module_test.go（真实 :memory: SQLite + 完整中间件链 + Cookie/CSRF 流转，wantCode 符号断言 40 处调用点 + assertAuditItemsCamelCase 契约防线 :210/:263-288）；前端无浏览器级 E2E（沿用 R1/R2 判定，归属部署冒烟）。

## 八、WARN 项（不阻断判定）

- **W1 覆盖率余量**：service/auth 79.4% vs 80% 目标差 0.6pp（未覆盖为存储异常/回滚错误分支，tasks.md T08 已说明需故障注入框架 MVP 不引入）；views/account-auth 组件级 0 vitest 测试（本轮 find 实证：测试文件仅 stores/__tests__ 2 + utils/__tests__ 4）。建议后续迭代补测。
- **N1 过期注释 7 处（本轮逐一 Read 实证在位）**：前端 `api/http.ts:157`「后端包络 code 上线数值 401NN」、`api/auth.ts:120-121`「后端 AuditRecord 无 json tag」、`api/auth.ts:153`「数值 401NN 归一」、`utils/errcode.ts:6-7`「Envelope.Code 为 int」、`utils/audit.ts:6-7`「AuditRecord 无 json tag」——均描述 E1/E2 修复前旧后端行为，与现状矛盾；后端 `middleware/authguard.go:89`「脱敏 50000」与 `middleware/recovery.go:15`「响应脱敏 50000」引用已废弃数值码（实际 ErrInternal 符号）。normalizeCode/normalizeAuditItem 防御垫片无害且有测试，注释应更新以免误导后续模块复用。
- **W4 src/components 目录不存在**：F-03 扫描路径之一（本轮 ugrep exit=2 警告实证）；本模块无跨页复用组件（局部组件均在 views/account-auth/*/components/），不构成违规，记录在案。

INFO（记录不计 WARN）：前端 `ListResult<T>` 对无分页接口为超集声明（含 total 而契约无），视图零消费 total（本轮通读 user-list/session-list 实证）。

## 九、总结

- **通过（本轮实跑/取证）**：后端 G-01~G-06 全过（0 CRITICAL，G-07 引用脚本+本轮 cover 复跑）、前端 F-01~F-05 全过（F-06 引用脚本）、AL-001~AL-005 全对齐（18 接口/DTO/8 枚举+15 动作码三方同值/DDL 逐字/包络）、E1/E2 修复保持（代码+双侧测试锁定）、业务规则 R-01/02/03/08/09/10/14/18 全过、SEC-GATE 探针通过且模块无桩（不适用）。
- **失败**：无。
- **WARN**：W1 覆盖率余量、N1 过期注释 7 处、W4 components 目录缺席。

**判定：WARN**——无 CRITICAL 违例、无契约断裂，可交付；W1 补测与 N1 注释清理建议随后续迭代处理。

---

## 十、WARN 修复轮（2026-11-16）

处置范围：第八节 WARN 项逐项，N1 按其 7 处点位逐处列示。修复仅涉注释文本，零行为变更；normalizeCode/normalizeAuditItem 防御垫片与其测试（errcode 4 + audit 5 用例）按第八节结论保留不动。

| 项 | 处置 | 证据 / 理由 |
|----|------|-------------|
| W1 覆盖率余量（service/auth 79.4% 差 0.6pp + views/account-auth 组件级 0 测试） | 维持 | 第八节修复建议即「后续迭代补测」：未覆盖分支为存储异常/回滚错误，需故障注入框架，backend/tasks.md:71 已说明「MVP 不引入（说明理由）」，不硬修不臆造；views 组件级 vitest 同属后续迭代补测范围 |
| N1-1 前端 api/http.ts:157「后端包络 code 上线数值 401NN」 | 已修复（前轮在位，本轮核验） | site-mgmt Gate 2 修复轮 2026-11-15 W-3 订正顺带闭环（见 gate2-site-mgmt-2026-11-14.md 复检节与 INDEX site-mgmt gates.gate2 note）；现行 http.ts:157-158 述「现行后端 symbol 符号串直上……数值 401NN 为 E1 期间后端历史通道，normalizeCode 保留归一作兼容兜底」，与现状一致 |
| N1-2 前端 api/auth.ts:120-121「后端 AuditRecord 无 json tag」 | 已修复（本轮） | api/auth.ts:119-122 订正为「后端 AuditRecord 已带全量 camelCase json tag（store/auth/models.go），条目键即契约 camelCase 直通；normalizeAuditListResult 的 PascalCase 回退仅作 E2 前历史形态兼容垫片」——与 store/auth/models.go:100-115 实况一致 |
| N1-3 前端 api/auth.ts:153「数值 401NN 归一」 | 已修复（本轮） | api/auth.ts:153 订正为「与 request() 同口径——现行后端 symbol 符号串直上，数值 401NN 仅历史兜底归一（utils/errcode.ts）」 |
| N1-4 前端 utils/errcode.ts:6-7「Envelope.Code 为 int」 | 已修复（前轮在位，本轮核验） | site-mgmt 修复轮同批订正（errcode.ts:2 注明「W-3 注释订正 2026-11-15」）；现行 errcode.ts:6-7 述「E1 双侧修复后 Fail 直接上线 errcode symbol 符号串（成功恒为数值 0），不再发送数值错误码」 |
| N1-5 前端 utils/audit.ts:6-7「AuditRecord 无 json tag」 | 已修复（本轮） | utils/audit.ts:6-10 订正为「后端现状（E2 双侧修复后）AuditRecord 已带全量 camelCase json tag（ActorID json:"-" 隐藏），条目键即契约 camelCase 直通；PascalCase 回退为 E2 前历史通道兼容垫片」 |
| N1-6 后端 middleware/authguard.go:89「脱敏 50000」 | 已修复（本轮） | 订正为「脱敏为 INTERNAL 符号码（ErrInternal），细节进日志」——errcode/auth.go:41 `ErrInternal = New("INTERNAL", "内部错误")` |
| N1-7 后端 middleware/recovery.go:15「响应脱敏 50000」 | 已修复（本轮） | 订正为「响应脱敏为 INTERNAL 符号码（不外泄细节）」 |
| W4 src/components 目录缺席 | 维持（事实已消解，无需动作） | 第八节判定「不构成违规，记录在案」；本模块无跨页复用组件（局部组件均在 views/account-auth/*/components/），不为此建空目录；该目录已由 terminal 模块落位（frontend/src/components/AppTerminal.vue，2026-11-15 实存），F-03 扫描路径现已有效 |

**自验（2026-11-16 实跑）**：cwd=backend/ 依次 `go build ./...`（exit 0，BUILD_OK）→ `go vet ./...`（exit 0，VET_OK）→ `go test ./...`（exit 0，全部包 ok 无 FAIL）；cwd=frontend/ `npm run build`（vue-tsc --noEmit + vite build）exit 0（✓ built in 26.76s；chunk >500kB 为既有体积告警非错误）。

> **复核行（2026-11-16，WARN 复核员）**：第十节 9 项逐一核验——N1-1~N1-5 五处前端注释（http.ts:157-158、api/auth.ts:119-122 与 :153、utils/errcode.ts:6-7、utils/audit.ts:6-10）与 N1-6/N1-7 两处后端注释（authguard.go:89、recovery.go:15，均改述 INTERNAL 符号码，与 errcode/auth.go:41 一致）本轮 Read 全部在位；W1 维持理由成立（backend/tasks.md:71 逐字核验「需故障注入框架，MVP 不引入（说明理由）」）、W4 维持成立（frontend/src/components/AppTerminal.vue 实存，F-03 路径有效）；另本轮复跑全仓 go build/vet exit 0 + go test 39 包 0 FAIL + vitest 15 文件 167/167 + npm run build exit 0。W1（service/auth 79.4% 覆盖率余量 + views 组件测试）仍未闭环 → **终局判定：warn**。汇总见 05-reports/gate2-warn-fix-review-2026-11-16.md。
