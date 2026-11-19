# Gate 2 验证报告 — 代码→交付门控（db-tools v1.3 自动 nginx 落地增量）

**模块**：db-tools（v1.3 增量轮，ADR-032 自动 nginx 落地）+ panel-core 机制侧（ConfigChangeHook 提供 S-11-01 / HostGuard·EntryPathGuard 内部端点豁免 S-07）+ site-mgmt ConfigRemove origin-absent 泛化
**扫描路径**：backend/（D:/Project/dougong-panel/backend）+ frontend/src/（D:/Project/dougong-panel/frontend）
**检查时间**：2026-11-19（本轮实跑）
**编号源**：engine-rules.md GR-01~06（机制层）；go-vue3 栈 backend-rules.md G-01~07 / frontend-rules.md F-01~06（执行清单）；业务规则 knowledge/business-concepts/index.md R-02/03/08/09/14 + I-02；validation SKILL Gate 2 节 AL-001~005
**D0 基准**：00-architecture/d0-clarification-dbtools-auto-landing-2026-11-19.md（Q1 自动落地+域名联动 / Q1b HostGuard 内部端点豁免最小化 / Q2 reason 可见 / Q3 安全底线 / 第四节验收口径）
**契约基准**：api-db-tools.md v1.3（11 端点零新增，D-19~D-23）/ PageSpec admin·readonly v1.2 / 04-business-enum.md
**前序报告**：gate2-db-tools-2026-11-19.md + gate2-db-tools-recheck-2026-11-19.md（v1.2 基线 WARN：W1 菜单重复/W2 env_type 已闭环，W3 覆盖率余量待确认）；gate1-dbtools-auto-landing-2026-11-19.md（设计面 WARN W1 panel-core 提供方登记待同步）

---

## 一、编译门控（本轮实跑 + ask 脚本结果引用，分列如实）

| # | 命令（工作目录） | 执行方 | 结果 |
|---|------------------|--------|------|
| C1 | `go build ./...`（backend/） | 本轮实跑 | exit=0（BUILD_OK） |
| C2 | `go vet ./...`（backend/） | 本轮实跑 | exit=0（VET_OK） |
| C3 | `go test -race -count=1 ./internal/service/dbtools/... ./internal/api/dbtools/... ./internal/api/middleware/... ./internal/service/panel/... ./internal/service/sitemgmt/... ./internal/adapter/toolx/...`（backend/，本轮全部改动包） | 本轮实跑 | exit=0，6 包全 ok（service/dbtools 12.148s、api/dbtools 10.619s、api/middleware 1.123s、service/panel 10.990s、service/sitemgmt 10.496s、adapter/toolx 1.123s） |
| C3′ | `go test ./... -race` 全仓 | ask 脚本（本轮未自跑全仓） | exit=0（ask 给定「test -race=0」；本轮实跑面=增量 6 包子集 C3，非全仓替代声明） |
| C4 | `npx vue-tsc --noEmit`（frontend/） | 本轮实跑 | exit=0（TSC_OK） |
| C5 | `npm run build`（frontend/） | 本轮实跑 | exit=0（vite built in 28.75s；chunk>500kB 警告为既有形态 INFO） |
| C6 | `npx vitest run src/types/__tests__/dbtool.test.ts src/stores/__tests__/dbtool.test.ts`（frontend/，db-tools 面） | 本轮实跑 | exit=0，2 文件 59 用例全过（types 50 + stores 9） |
| C6′ | `vitest` 全量 | ask 脚本（本轮未自跑全量） | exit=0（ask 给定「vitest=0」；本轮实跑面=dbtool 2 文件子集 C6） |
| C7 | `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./...`（backend/） | 本轮实跑 | exit=0（CROSS_OK） |
| C8 | `gofmt -l`（本轮全部改动包：service/dbtools·api/dbtools·api/middleware·service/panel·service/sitemgmt·adapter/toolx·cmd/dougong） | 本轮实跑 | 0 输出 |
| C9 | dist 同步核验 | 本轮实跑 | frontend/dist 与 backend/web/dist 全部资产 md5 逐一相等；唯一差异=backend/web/dist/.gitkeep 空占位（Makefile:11 `touch` 产物，非内容差异）；backend/web/embed.go `//go:embed all:dist` 承载 |

## 二、后端栈规则 G-01~07（实扫）

| # | 检查 | 命令 | 结果 |
|---|------|------|------|
| G-01 | 无 shell 拼接 | `grep -rnE 'exec\.Command(Weak)?Context?\(.+"(-c\|/c)"' --include="*.go" .` + 全量 `exec.Command` 逐点核对 | **PASS（增量面）**——唯一命中 internal/adapter/cronx/runner.go:145（`sh -c` argv 数组执行 cron 任务原文）为既有 cron-backup 模块（git status 未改动，历轮已审）；增量新增执行体仅 toolx probePortAvailable（net.Listen 非 exec，gateway.go:468-478）；toolx run:492 参数数组不变 |
| G-02 | 无 SQL 拼接 | `grep -rnE "Sprintf\(.+(SELECT\|INSERT\|UPDATE\|DELETE)" --include="*.go" .` | **PASS**——全仓 0 命中 |
| G-03 | 无硬编码凭据 | `grep -rnEi '(password\|secret\|token)\s*=\s*"[^"]{8,}"' --include="*.go" \| grep -v _test` + 补充 `(Password\|Secret\|Token\|APIKey):\s*"[^"]{6,}"` 结构体字面量扫描 | **PASS**——两式均 0 命中；增量凭据面零新增（landing/confighook 不涉凭据） |
| G-04 | SQL 只在 store 层 | `grep -rlnE "(SELECT\|INSERT\|UPDATE\|DELETE) " --include="*.go" \| grep -v internal/store \| grep -v _test` + 增量文件逐文件复核 | **PASS（增量面）**——增量 11 个改动/新增后端文件（landing.go/confighook.go/hostguard.go/entrypath.go/inject.go/deploy.go/consts.go/templates.go/dto.go/access.go/service.go/ports.go/panel config.go·service.go/sitemgmt configpipeline.go·pipeline.go/toolx gateway.go/main.go）实 SQL 0 命中（逐文件 grep 空输出）；层外命中均为既有模块（dbmysql 外部受管通道等，历轮已审） |
| G-05 | 无 panic 滥用 | `grep -rn "panic(" --include="*.go" \| grep -v _test` | **PASS**——全仓唯一 internal/api/server.go:43 embed 启动期不可达不变式（规则允许）；增量 0 panic（confighook.go:42 recover 为钩子隔离非 panic 发起） |
| G-06 | goroutine 退出路径 | `grep -rnE "go func\|^[[:space:]]*go [a-zA-Z(]" --include="*.go" \| grep -v _test` 逐点核对 | **PASS**——增量零新增 goroutine（landing.go/confighook.go 全同步；钩子进程内同步调用 confighook.go:32-36）；既有 deploy.go:161 `go s.runTask(taskID)` 生命周期=有限命令链（超时注入）不变；main.go goroutine 群为既有形态（wg+ctx，历轮已审） |
| G-07 | 编译+vet+test 全绿 | 见第一节 C1~C3 | **PASS** |

## 三、前端栈规则 F-01~05（实扫）

| # | 检查 | 命令（frontend/） | 结果 |
|---|------|------------------|------|
| F-01 | 无 Options API | `grep -rLn "script setup" --include="*.vue" src/` | **PASS** 0 命中 |
| F-02 | 无裸 any | `grep -rnE "(:[[:space:]]*any\b\|as any)" --include="*.ts" --include="*.vue" src/` | **PASS** 0 命中（增量助手 landingTag/landingAddress/landingHint/installReadyAlertText/dbtFailureGuide 均类型化，types/dbtool.ts:183-317） |
| F-03 | 请求只走 api 层 | `grep -rnE "(axios\|fetch)\(" --include="*.vue" src/views src/components` | **PASS** 0 命中（增量零新 fetch——landingPort/autoCreated 经既有 dbtoolList 投影；打开 URL 后端权威下发前端不拼接，index.vue:254-258 注释+D-23） |
| F-04 | 无混入其他组件库 | `grep -rnE "from '(antd\|element\|ant-design-vue)" src/` | **PASS** 0 命中 |
| F-05 | 无原生弹窗 | `grep -rnE "(^\|[^a-zA-Z])(alert\|confirm)\(" --include="*.vue" --include="*.ts" src/` | **PASS** 0 命中（增量提示均 NAlert/NTag，DbtoolTaskDrawer.vue:102-105/141-155、index.vue:506-528） |
| F-06 | typecheck+build+单测 | C4/C5/C6 | **PASS** |

面板类专项：任务详情 5s 轮询终态即停机制不变（stores/dbtool.ts，本轮 stores 测试 9 用例过）；页面 onUnmounted 清理在位（index.vue 既有形态，增量未触及）。

## 四、契约对齐 AL-001~005（本轮实读比对）

| # | 检查 | 方法 | 结果 |
|---|------|------|------|
| AL-001 | Controller 路径方法 vs api 文档 | module.go:36 前缀 /dbtool + :46-66 路由逐行 ↔ api 3.3 矩阵（:154-164）11 行双向比对 | **PASS 11/11**——GET tool/list·task/list·task/detail·access/reopen·gate/verify + POST tool/install·upgrade·uninstall·access/open·access/rotate·signon/verify；v1.3 零新接口（api 3.1:130 接口面变更纪律）路由面零变化 |
| AL-002 | 前端 Action 路径 vs Controller | api/dbtool.ts:23-75 request 路径+方法提取 ↔ 面板面 8 接口（本轮 api/dbtool.ts 未改动，git status 核实） | **PASS 8/8**——tool/list·install·upgrade·uninstall·task/list·task/detail·access/open·access/rotate 路径方法逐一对应；reopen 为 302 顶层导航非 fetch、内部 2 服务端调用（PageSpec 第 2 节显式声明，readonly v1.2:23 同） |
| AL-003 | DTO 可空逐形状 | dto.go `*T json` 集合 ↔ types/dbtool.ts `\|null` 集合 | **PASS 11/11**——既有 9（installedVersion/boundInstanceId/running/runningTaskId/instanceId/startedAt/finishedAt/error/tokenExpiresAt）+ v1.3 additive 2（landingPort: number\|null ↔ LandingPort *int dto.go:29；autoCreated: boolean\|null ↔ AutoCreated *bool dto.go:30；前端 types/dbtool.ts:48/:51）；后端 *T 无 omitempty（dto.go 唯一 grep 命中为 :4 注释文本「无 omitempty」非 tag）；ToolStatusInfo.BoundInstanceID（dto.go:92）为进程内 OHS 非 HTTP 面不计；未装=null/null、旧档案缺省 0/false 投影回归 TestToolListLandingFields 实跑过 |
| AL-004 | 枚举四方同值 | 池（04-business-enum.md:63-65）↔ api 3.4（:195-197）↔ 前端 enums.ts:640-660 ↔ 后端 store/dbtools/models.go:23-33 逐值比对 | **PASS**——db_tool_type{phpmyadmin,mongo_express}/db_tool_task_action{install,upgrade,uninstall}/db_tool_task_status{pending,running,succeeded,failed} 四方逐值相等；v1.3 零枚举变更（port 数值/autoCreated 布尔非枚举不入池，api D-19/域模型口径）。**前轮 W2 env_type 缺口本轮核实已闭环**：池:57 八值含 node == api-env-mgmt v1.3:124（ENV_002 八值）/:342（DDL CHECK 八值）== 前端 enums.ts:599（EnvType 含 'node'）== 后端 models.go:28 EnvNode + 迁移 014:9 八值 CHECK |
| AL-005 | 错误码双侧 | errcode/dbtools.go:11-39 New 符号集 ↔ api 3.4 DBT 行（:175-186）↔ 前端 DBT_ERROR_TEXT 键集（types/dbtool.ts:109-122） | **PASS**——DBT_001~012 三方逐字相等连续无缺号；v1.3 零新错误码（DBT_012 reason 细分文案经 data.reason 承载，形状不变 api:186/:189）；AUTH_001/002/003/008 复用不变；内部端点拒绝语义不变（gate 裸 401/signon 包络 DBT_007/008/010/loopback·秘密头裸 404） |
| 包络 | 统一响应 | api 3.1:123 ↔ 既有 response 包络（本轮未改动） | **PASS**——{"code":0,"message":"ok","data":...}；gate/verify 裸状态例外显式 |

## 五、业务规则 R-02/03/08/09/14 + I-02 + readonly 负空间

| 规则 | 核验（实读/实跑） | 结果 |
|------|------------------|------|
| R-02 会话+CSRF | module.go:43-57 面板面 InitGuard→AuthRequired→CsrfGuard(POST)→RequireRole 链本轮零改动（实读）；reopen GET 豁免 CSRF 沿 api 3.1:126 显式安全口径；增量豁免（hostguard/entrypath）仅作用内部 2 端点路径前缀，面板面中间件链不受影响（internal_exempt_test.go 非内部路径用例实证）；TestWriteEndpointsAdminOnly（缺 CSRF→AUTH_003/readonly→AUTH_008）/TestReadEndpointsBothRoles（未登录→AUTH_001）本轮 -race 实跑过 | **PASS** |
| R-03 变更审计 | 增量零新审计面：自动建块/清块为 worker 执行链步骤（终态审计 finishTask 同事务机制不变，deploy.go runTask 终态路径实读）；域名联动钩子仅刷 nginx 配置无 DB 状态写（panel_config 变更审计归 panel-core UpdateConfig 既有机制）；open/reopen 惰性校正不新增状态写（审计沿既有 recordAudit）；TestOpenValidation 审计脱敏断言本轮实跑过 | **PASS** |
| R-08 参数数组 | G-01 全点核对；增量唯一新执行体 probePortAvailable=net.Listen("tcp", net.JoinHostPort("", strconv.Itoa(port)))（toolx/gateway.go:468-478，参数化非常量拼接，非 shell）；landing.go 零 exec；TestInstallMongoExpressChain 凭据不进 argv 断言本轮实跑过 | **PASS** |
| R-09 零遥测 | 增量零外联：landing.go imports 无 net/http（实读 :3-16，仅 nginx 文件读+管线+本地端口探针）；confighook.go 进程内同步；phpMyAdmin VersionCheck=false/SendErrorReports='never' 不变（templates.go:105-106 实读）；下载为用户主动例外沿 D-05 | **PASS** |
| R-14 服务端校验 | toolType 白名单/status 取值域/taskId>0/instanceId 必填语义本轮零改动（历轮回归 TestWriteEndpointsAdminOnly 本轮实跑过）；增量新输入面=BindDomain 经 panel-core D-P12 RFC1123 上游校验（config.go UpdateConfig 既有）+模板拼接前 regexp.QuoteMeta 转义防注入（landing.go:173）；自动块路径来自 nginxx 受管目录扫描非用户输入（landingConfPath:77-94） | **PASS** |
| I-02 凭据零出口 | 增量 DTO 仅 landingPort/autoCreated（非敏感，dto.go:26-30）；landing.go/confighook.go 零凭据消费（全文件实读）；slog 仅 file/err 字段（landing.go:282-283/:296/:300-301）；放行提示仅文件路径+端口（appendLandingHint:307-313）；SignonVerifyData 仍仅 loopback 内部端点响应（module.go:64-66 防护链+ dto.go:95-102）；vault AES-256-GCM+0600 密钥文件机制零改动；TestOpenValidation/TestSignonVerifyGuard 无泄漏断言本轮实跑过 | **PASS** |
| readonly 负空间 | 6 负空间（Install/Upgrade/Uninstall/Rotate/Open/Reopen）三层防护本轮零削弱：①前端 index.vue:531 操作区 `v-if="authStore.isAdmin"` 整区不渲染（实 grep 在位；增量访问形态区块为纯观测信息两角色同等呈现，readonly PageSpec 4.1:34 显式）；②后端 RequireRole(admin) 写接口 AUTH_008 + reopen handler role!=admin→302 /db-tools（module.go:169）+ TestReopenRedirects 本轮实跑过；③工具路径网络层 gating readonly→401（gate.go:32-33 role!=admin 拒绝，TestGateVerifyGuard 本轮实跑过——自动块形态同模板注入 auth_request，TestAutoLandingInstallUnbound 断言）；readonly PageSpec v1.2:23 负空间 6 项+4.3 工具本体不可达登记（+18090 gating 401 同语义/绑域名非匹配 Host 404）与实现逐项对应；3 读+6 负空间=面板 9 全集 | **PASS** |

## 六、安全专项实证（D0 Q3 + ask 安全专项，逐项本轮 -race 实跑回归）

> 回归执行命令：`go test -race -count=1 -v -run 'TestAutoLanding|TestDomainHookRefresh|TestLazyRefreshOnOpen|TestToolBaseHostPortBranch|TestToolListLandingFields|TestOpenReopenAutoURL|TestGateVerifyGuard|TestSignonVerifyGuard|TestSignonTampered|TestSignonExpired|TestSignonReplay|TestSignonBinding|TestSignonKeyIsolation|TestTokenSweeper|TestUninstallDataPreservation|TestOpenValidation|TestInjectionRoundTripBothTools|TestInstallPHPMyAdminChain|TestInstallMongoExpressChain|TestInternalExempt|TestUpdateConfigBindDomainHook|TestConfigRemove|TestWriteEndpointsAdminOnly|TestReadEndpointsBothRoles|TestReopenRedirects' ./internal/service/dbtools/... ./internal/api/dbtools/... ./internal/api/middleware/... ./internal/service/panel/... ./internal/service/sitemgmt/...` + 补跑 `-run 'TestHostGuardInternalExemptFullStack|TestGateVerifyGuard|TestSignonVerifyGuard' ./internal/api/dbtools/...`——38 个命名测试全 PASS（0 FAIL）。

### S1 工具本体仅 localhost — PASS

- mongo-express：MongoExpressBind="127.0.0.1"/MongoExpressPort=8081 编译期常量（consts.go:31-33 实读）→ 单元 ME_CONFIG_SITE_BIND_ADDRESS 显式覆盖默认 0.0.0.0；**TestInstallMongoExpressChain 本轮实跑过**（逐字断言单元 env）。
- phpMyAdmin：无独立监听端口——FastCGI 经 PHP-FPM pool unix socket（templates.go:39-63 实读）；**TestInstallPHPMyAdminChain 本轮实跑过**。
- 自动块 listen 18090 为 **nginx 接入层**非工具进程端口（renderLandingConf landing.go:144-158，nginx `listen 18090;` 缺省绑 0.0.0.0=ToolLandingBind consts.go:71-73 注释口径）；工具 location proxy_pass 上游仍 127.0.0.1:8081/unix socket（templates.go:54/:73）。
- 未执行项如实：实机 `ss -tlnp` 端口观测归 Linux 目标环境冒烟。

### S2 自动块每请求 gating（无会话/readonly→401 回归）— PASS

- 自动块工具 location 注入沿既有模板：phpMyAdmin 外层+嵌套 php location 双 auth_request（templates.go:48/:52）、mongo-express auth_request（templates.go:71）、gate location internal;+秘密头+Cookie 透传+request_body off（templates.go:23-31）——**TestAutoLandingInstallUnbound 本轮实跑过**：断言自动块含 `auth_request /dbtool-gate-internal`+`location = /dbtool-gate-internal`+`internal;`+`proxy_pass http://127.0.0.1:8687/dbtool/gate/verify`+nginxx.ValidateSyntax 语法自检。
- **TestGateVerifyGuard 本轮实跑过**（module_test.go:494-515）：秘密头缺失/错误→裸 404、**无会话→401、readonly→401、失效会话→401、admin→200**（裸状态无包络 auth_request 语义）。
- 幂等复用两工具共用一块 gate 去重：**TestAutoLandingIdempotentReuse 本轮实跑过**（gate BEGIN 计数=1+两工具 marker 共存+语法自检+复用先于端口探针 probeCalls=1）。
- 吊销级联：VerifyWs 逐请求实时判定（gate.go 只读不续期机制零改动）。
- 未执行项如实：真实 nginx auth_request 子请求联调归 Linux 冒烟（httptest 直打内部端点等价验证判定链）。

### S3 HostGuard 豁免内部端点（绑域名后可达回归+非内部强校验不变+豁免最小化）— PASS

- 豁免集合最小化：internalExemptPrefixes=[]string{"/dbtool/gate/","/dbtool/signon/"} 编译期常量（hostguard.go:26 实读，仅内部 2 端点前缀）；HostGuard 前置直通（hostguard.go:53-56）+EntryPathHandler 前置直通（entrypath.go:36-39，携前缀剥前缀既有语义不变 entrypath.go:52-61）；链序 Entry→Host→gin 不变（main.go:491-492 实读）。
- **TestInternalExemptHostGuard 本轮实跑过**（12 用例）：gate/signon Host≠域名（IP 直连）→200 豁免；**非内部 tool/list·access/open·access/reopen·panel/config/detail IP Host→404 强校验不变**；匹配 Host 非内部照常 200；**最小化负空间 4 用例**：/dbtool/gate（无尾斜杠）/dbtool/signon/dbtool/gateX/verify（前缀近似）/dbtool 裸段→均 404 不豁免；404 须标准 net/http plain 无业务码断言。
- **TestInternalExemptHostGuardReadFailure 本轮实跑过**：读态失败非内部 fail-closed 404 不变；内部豁免前置不受读态影响（其安全由 loopback+秘密头承载）。
- **TestInternalExemptEntryPath 本轮实跑过**（8 用例）：内部携/未携入口前缀均可达（防御性兜底）；非内部未携/错携前缀仍 404；入口根语义不变；读态失败同核。
- **TestInternalExemptChainOrder 本轮实跑过**（5 用例）：Entry→Host 链序协同——内部携/未携前缀+IP Host 可达；非内部强校验不变。
- **TestHostGuardInternalExemptFullStack 本轮实跑过**（module_test.go:668-733，httptest 全栈+真实 gin 引擎+真实会话/秘密头）：绑域名 panel.example.test 后——内部 gate Host=127.0.0.1:8687+loopback+秘密头+admin 会话→**200 可达**；**无秘密头→裸 404（防护链不放宽）**；**无会话→401（gating 不放宽）**；内部 signon 伪造令牌→包络 DBT_007（HTTP 200 可达非 404，四重校验不放宽）；非内部 tool/list IP Host→404；匹配 Host→200。
- 豁免边界=Host/入口路径校验而非鉴权：module.go:64 内部组 loopbackGuard+internalSecretGuard 零改动（实读）；api 4.4.5 验收五条逐项对应上述回归。

### S4 域名联动（未绑 _/绑定后域名+非匹配 404/变更同步）— PASS

- 未绑：renderLandingConf→`server_name _;` 无 if 行（landing.go:162-167）；**TestAutoLandingInstallUnbound 本轮实跑过**（断言 server_name _;+无 `if ($host`）。
- 已绑：`server_name {BindDomain};`+块内首行 `if ($host !~* "^{域名}$") { return 404; }`（landing.go:150-151/:172-174，大小写不敏感全等镜像 HostGuard stripPort+EqualFold；regexp.QuoteMeta 转义防注入）；**TestAutoLandingInstallBound 本轮实跑过**（断言 `if ($host !~* "^tools\.example\.test$") { return 404; }` 逐字+语法自检+Open URL http://tools.example.test:18090/...）。
- 变更同步主通道：panel UpdateConfig bindDomain 变更成功→notifyBindDomainChanged（config.go:285-290 实读）→OnBindDomainChanged 刷新（landing.go:276-285，失败仅 slog 不阻断配置变更）；**TestUpdateConfigBindDomainHook 本轮实跑过**（绑定通知新值/同值零通知/解绑通知 ""/非 bindDomain 字段零通知/**钩子 panic 不阻断配置变更且权威已落库**）；**TestDomainHookRefresh 本轮实跑过**（绑定→server_name+if 行；同值重复触发零写入幂等；换绑 if 随域名更新旧域名零残留；解绑→server_name _+if 移除；工具 location 字节不受影响；无自动块钩子零写入；每步语法自检）。
- 惰性兜底：install 复用路径 refreshLandingServerName（landing.go:48-51，失败=任务 failed reason 如实）+open/reopen lazyRefreshLanding（access.go:52-54/:105-107，失败不阻断打开 slog 降级）；**TestLazyRefreshOnOpen 本轮实跑过**（陈旧 server_name→open 自愈刷新+URL 按当前 BindDomain；ConfigApply 失败仍放行打开）。
- 刷新仅改 server_name/if 两 span（landing.go:179-269 tree.Apply 外科手术），工具 location 注入字节不动（TestDomainHookRefresh marker 断言）。
- 未执行项如实：真实 nginx 非匹配 Host 404 行为归 Linux 冒烟（模板逐字断言+语法自检承载）。

### S5 signon 防伪（HMAC+60s+一次性+双绑定）— PASS（本轮全回归实跑）

- TestSignonTampered（篡改/伪造/形态非法→DBT_007）/TestSignonExpired（超 60s→DBT_008）/TestSignonReplay（消费后重放/重启新台账→DBT_010）/TestSignonBinding（跨会话/失效会话/未转发 cookie/跨实例→DBT_007）/TestSignonKeyIsolation（卸载除档令牌永久不可校验）/TestTokenSweeper（清扫窗口收敛）——security_test.go，本轮 -race 全 PASS。
- TestSignonVerifyGuard（缺秘密头裸 404+伪造令牌包络 DBT_007+响应无凭据泄漏）本轮实跑过；TestHostGuardInternalExemptFullStack 绑域名后伪造令牌仍 DBT_007（豁免不放宽防伪）。
- signon.go 四重校验机制本轮零改动（git status 核实未列入改动）。

### S6 凭据 vault 零回显 — PASS

- 见第五节 I-02 行；增量零凭据面扩展；TestOpenValidation（审计 detail 不含令牌/凭据/URL）+TestSignonVerifyGuard（响应不含 mysql-pass）本轮实跑过；rotate 响应无口令字段（dto.go:80-83 RotateData{toolType,rotatedAt} 实读）。

### S7 卸载不删数据+清自动块 — PASS

- 数据保全白名单零改动：**TestUninstallDataPreservation 本轮实跑过**（两工具子测试——remove/removeall 路径 ∈{toolDir 前缀, MongoExpressUnitPath}，禁词 /var/lib/mysql·/var/lib/mongodb·/var/lib/docker·db_instance 零触及，nginx conf byte 级还原）。
- 清块判定：cleanupAutoLanding（landing.go:320-345）——autoCreated=false 绝不动；另一工具档案仍在→共享块保留；两工具均卸载→ConfigRemove 整块；块已不存在=幂等成功；失败=DBT_012 reason 档案不变（I-C01）。
- **TestAutoLandingCleanupOnUninstall 本轮实跑过**：卸 phpMyAdmin（mongo 仍在）→块保留+零 remove 调用；再卸 mongo→块移除且经 ConfigRemove（"remove:"+file 断言）；**非自动块形态卸载零 remove 调用+既有块 byte 级还原原字节**。
- **TestAutoLandingCleanupFailure 本轮实跑过**：清块失败→任务 failed「DBT_012: 部署操作失败: 清块失败: reload failed (fake)」+档案不变。
- ConfigRemove origin-absent 语义（configpipeline.go:29-40+pipeline.go:69-74 标记）：**TestConfigRemoveOriginAbsent 本轮实跑过**（allowNew 新建→后续快照含块内容→Remove 仍删除文件+reload（标记优先于快照恢复）+幂等）；TestConfigRemoveOHS 既有快照恢复语义不变本轮实跑过。

### S8 reason 可见（D-22）— PASS

- worker finish：errors.As 取 *errcode.Error 且 Data()["reason"] 非空 string→追加 ": "+reason（deploy.go:206-216 实读）；无 reason 沿 symbol+message 不虚构。
- **TestAutoLandingPortOccupied 本轮实跑过**：task.Error 逐字=「DBT_012: 部署操作失败: 工具端口 18090 被占用，请释放后重试」+失败不留档案+不建块+探针恰 1 次。
- **TestAutoLandingWriteFailureReason 本轮实跑过**：「DBT_012: 部署操作失败: 自动落地块写入失败: nginx -t failed (fake)」；TestAutoLandingNoConfDir（reason「nginx 配置目录不可用」）/TestAutoLandingCleanupFailure（reason「清块失败」）同轮过。
- 前端呈现：DbtoolTaskDrawer.vue:141-155 error 全文 NAlert+dbtFailureGuide 引导（端口占用→释放引导/多候选→/sites 链接，types/dbtool.ts:183-189）；dbtErrorText DBT_012 追加 data.reason（types/dbtool.ts:164-176）；types 测试 50 用例含 reason/guide 断言本轮实跑过。

### S9 端口分支与 Open URL（D-23）+ 既有块优先/多匹配/幂等 — PASS

- **TestToolBaseHostPortBranch 本轮实跑过**（7 用例表驱动）：自动块+绑域名→http://域名:18090；未绑+请求 Host 去端口→http://IP:18090；IPv6 [::1]:18090 规范化；无请求语境→动态回退；port=0 三用例沿旧形态。
- **TestOpenReopenAutoURL 本轮实跑过**：open/reopen 同口径 http://10.0.0.8:18090/dbtool/phpmyadmin/?dg_signon=...；mongo_express URL 无令牌（basic auth 服务端注入）。
- **TestAutoLandingExistingBlockPriority 本轮实跑过**：既有匹配块优先不新建+零探针+siteRef port=0/autoCreated=false。
- 多匹配仍 DBT_012 列候选（inject.go:107-115 零改动实读）；自动块排除面板域候选防第二工具误判（inject.go:66-71，tasks.md A-DBT13 声明）。
- SignonURL 面板直连端口口径（A-DBT11，tasks.md:1167 裁定声明）：TestAutoLandingInstallUnbound 断言动态表达式 preg_replace 去 HTTP_HOST 端口+:8687 面板端口+**不得含 :18090**；TestAutoLandingInstallBound 断言静态 http://tools.example.test:8687——本轮实跑过。
- 安装成功放行提示（D-19⑦）：TestAutoLandingInstallUnbound 断言 outputTail 含「请放行 TCP 18090 端口（防火墙/云安全组）」；前端 installReadyAlertText/landingHint 同口径（types 测试过）。

## 七、测试覆盖率（本轮 go test -cover 实测）

| 包 | 覆盖率 | 目标（validation SKILL） | 达标 |
|----|--------|--------------------------|------|
| service/dbtools | 75.8%（前轮 73.0%→+2.8pp，landing 回归新增） | Service 80%+ | **NO（W2）** |
| api/dbtools | 73.5% | Controller 60%+ | YES |

安全核心路径（gating/signon 四重/vault/卸载白名单/自动建块/域名联动/HostGuard 豁免全栈/清块/reason）专用回归全覆盖（第六节 38 命名测试本轮实跑）；未覆盖余量主要为 deploy 执行链错误分支组合与 landing 刷新错误分支（W2 如实上报）。

## 八、WARN 项（非 CRITICAL，待用户确认）

- **W1 panel-core 提供方设计登记未兑现（承 Gate 1 W1，代码面已实现）**：ConfigChangeHook 提供（S-11-01，confighook.go 新文件+config.go:285-290 通知+main.go:461-464 注册）与 HostGuard/EntryPathGuard 内部端点豁免（S-07，hostguard.go:26/entrypath.go:36）**机制代码本轮已实现且回归实证全过**（S3/S4），但提供方 api-panel-core.md 仍 v2.2——实查 grep ConfigChangeHook/OnBindDomainChanged/INTERNAL_EXEMPT/S-11-01 无命中（仅 D-P03 通用描述 :31），prd/BA-panel-core 同。设计文档同步归 panel-core 增量轮（db-tools api D-20⑤/D-21⑤ 显式声明「本轮不代改」沿先例）。非代码缺陷，不阻断交付；建议 panel-core 设计轮补齐提供方登记。
- **W2 service/dbtools 覆盖率 75.8% < 80% 目标（承前轮 W3）**：较前轮 73.0% 提升 2.8pp（landing 回归新增）；安全核心专用回归全覆盖（第六节），余量为 deploy/landing 错误分支组合；沿 account-auth W1 覆盖率余量先例如实上报待确认（前轮复检已「接受不修」口径，本轮数值更新）。

## 九、INFO 项

- **I1 前轮 W1/W2 闭环核实**：W1 菜单重复登记——grep 'Key: "db-tools"' 全仓唯一 main.go:328（本轮实扫，原 :462 重复点已删改注释 main.go:462-463）；W2 env_type 四方缺口——本轮四方实读比对八值同值（AL-004 行）。
- **I2 A-DBT11~A-DBT13 实现裁定**（backend/tasks.md:1163-1194 声明）：SignonURL 面板直连端口口径（D-23「烘焙基址同口径」落码歧义裁定——reopen 为面板后端 API 非工具落点，自动块形态基址含 ListenPort 而非 18090）/ConfigRemove origin-absent 泛化（D-19⑥「文件不存在态」语义兑现）/selectLanding 排除自动块候选（回归发现的误判 bug 修复）。建议 api D-23 正文回填 SignonURL 面板端口口径注记。
- **I3 dist 同步唯一差异=.gitkeep 空占位**（Makefile:11 touch 产物）；全部资产 md5 逐一相等（C9）。
- **I4 vite chunk>500kB 警告**为既有形态（monaco/echarts 等），非本轮引入。
- **I5 编译门控执行方分列**：全仓 go test -race 与全量 vitest 为 ask 脚本结果（=0）引用；本轮实跑面=增量 6 包 -race 子集+dbtool 2 文件 vitest 子集+build/vet/tsc/linux 交叉/gofmt/dist 全量（第一节 C1~C9 逐行标注）。
- **I6 工作区存在本轮外未提交改动**（git status 40 文件含设计文档/tasks.md 等）——沿历轮 Gate 2 先例如实登记，不影响本轮增量面判定。

## 十、未执行项如实声明

- **Linux 实机冒烟（D0 验收口径末项）本轮未执行**：IP 直连面板装 phpMyAdmin/mongo-express 自动建块、经 IP:18090 打开（放行端口后）、无会话访问 18090 工具路径→401、真实 nginx auth_request 子请求/非匹配 Host 404、实机端口观测——归目标环境（本轮 Windows 开发机无 Linux 运行时）。等价验证经全注入 fake 替身 httptest 全栈回归承载（第六节逐项，判定链真实执行仅外部执行体伪化）。
- govulncheck（backend-rules §6 CI 周期项）本轮未执行（非 Gate 2 必跑清单）。
- 全仓 go test -race / 全量 vitest 本轮未自跑（ask 脚本已跑=0，本轮实跑增量子集，第一节 C3′/C6′ 分列）。

## 十一、总结

**判定：WARN**（CRITICAL 0 / 契约断裂 0 / 安全专项九项全实证通过）

- 编译门控：build/vet/tsc/npm build/linux 交叉/gofmt/dist 本轮实跑全绿；增量 6 包 -race 全 ok；全仓 race/全量 vitest 引 ask 脚本=0（分列如实）。
- G-01~07 增量面全过（唯一 sh -c 为既有 cronx 豁免；增量零新 goroutine/零 panic/零 SQL/零硬编码凭据）；F-01~05 全 0 命中。
- AL-001~005 零差（11 端点/8 fetch Action/11 可空字段含 v1.3 additive 2/3 枚举四方+env_type 八值闭环/DBT_001~012 三方）。
- 安全专项 S1~S9 逐项代码取证+38 命名回归本轮 -race 实跑实证：localhost 绑定/自动块每请求 gating 无会话·readonly→401/**HostGuard 豁免绑域名后内部端点可达+非内部强校验不变+最小化 4 负空间用例+全栈回归**/**域名联动未绑 _·绑定 if 404·钩子换绑解绑幂等·惰性自愈**/signon 防伪四重/vault 零回显/卸载不删数据+清自动块（非自动块不动）/reason 逐字可见/端口分支 URL。
- R-02/03/08/09/14+I-02+readonly 6 负空间三层防护零削弱。
- WARN W1（panel-core 提供方设计登记，代码已实现回归已过）/W2（覆盖率 75.8%<80% 余量）待用户经主会话确认；确认后 Gate 2 记 pass。
- 报告路径：05-reports/gate2-dbtools-auto-landing-2026-11-19.md
