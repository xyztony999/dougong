# Gate 2 增量验证报告 — 代码→交付门控（panel-core 增量复检）

**模块**：panel-core（面板基座）
**检查时间**：2026-11-16
**检查者**：增量检查员·panel-core·G2（validation skill · Gate 2；机制编号源 engine-rules.md GR-01~GR-06，扫描清单源 go-vue3 栈 `.claude/rules/backend-rules.md` §7（G-01~G-07）/`frontend-rules.md` §6（F-01~F-06），业务规则源 knowledge/business-concepts R-01~R-18）
**增量范围**（`git status`/`git diff` 实看）：backend 13 改 7 新（panel api/service/store/errcode、middleware/hostguard*、service/panel/acme*·certstatus*、migrations/007_panel_core_acme.sql、store/panel/acme_store_test.go、main.go）；frontend 9 改 2 新（api/types/stores、HttpsCard、settings/index、BindDomainCard.vue、PanelCertCard.vue）。增量内容 = ADR-028 域名绑定强校验（HostGuard + bind_domain）+ ADR-029 面板证书 ACME 经已有 nginx（cert 三接口 + 迁移 007 + 4.1.3 每日续期调度）。
**契约基线**：api-panel-core.md **v2.0**（任务书写「v1.2」，Gate 1 I1 已澄清：v1.2 为 prd 变更记录行，api 实为 v1.0→v2.0；本轮以 v2.0 为对齐基准）；PageSpec admin/readonly v1.1。
**编译门控**：流水线脚本实跑 build=0 / vet=0 / test=0 / web=0（任务书给定，本轮转述）；本轮另实跑 `go build ./...`=0（exit 0）、增量四包 `go test -count=1`（api/panel·service/panel·store/panel·api/middleware 全 ok）、前端 `npx vitest run src/stores/__tests__/panel.test.ts` 8/8 passed。

## 一、🔴 CRITICAL 扫描（栈规则 grep 实跑摘录）

### 后端 G-01~G-07（backend-rules §7；cwd=backend/）

| # | 检查 | 实跑命令与结果 | 判定 |
|---|------|--------------|------|
| G-01 | 无 shell 拼接 | `grep -rnE "exec\.Command(Weak)?Context?\(.+\"(-c\|/c)\"" --include="*.go"` → 唯一命中 `internal/adapter/cronx/runner.go:145`（存量：cron 代执行语义，命令串为独立 argv 元素零拼接，tasks.md T07 已登记，本轮零改动——`git log`/`git diff` 实证 cronx 未触）；全量 `exec.Command` 21 处逐一核对均 argv 参数数组；**增量 ACME 通道消费 nginxx 网关 TestConfig/Reload（gateway.go:303 `-t`、:327 `-s reload` 字面量数组）与 acme lego 库（零 shell）** | PASS |
| G-02 | 无 SQL 拼接 | `grep -rnE "Sprintf\(.+(SELECT\|INSERT\|UPDATE\|DELETE)" --include="*.go"` → 0 命中（exit 1） | PASS |
| G-03 | 无硬编码凭据 | `grep -rnEi "(password\|secret\|token\|apikey)\s*=\s*\"[^\"{}]{8,}\"" --include="*.go" \| grep -v _test` → 0 命中（exit 1） | PASS |
| G-04 | SQL 只在 store 层 | 层外命中逐一核对：service 层 10 行全为流程注释（auth/user.go:114、cronback、dbmgmt、fileman、sitemgmt certsvc.go:80/scanner.go:160）；adapter/dbmysql 真实 SQL 字面量为存量已裁定引擎协议通道（A-DB10，tasks.md:614/624 登记）。**增量 panel SQL 全部落 internal/store/panel/{config_repo,cert_repo}.go（5 处，`grep -rn "SELECT\|INSERT" internal/store/panel/*.go \| grep -v _test \| wc -l`=5），repos 全部 `?` 参数化实读复核** | PASS |
| G-05 | 无 panic 滥用 | `grep -rn "panic(" --include="*.go" \| grep -v _test` → 唯一 `api/server.go:43` embed 启动期不可达（存量同判）；增量文件 0 命中 | PASS |
| G-06 | goroutine 有退出路径 | `grep -rn "go func" --include="*.go" \| grep -v _test` → main.go 10 处 + dockerx/logs.go:165 + handler_ws.go:86（均存量）。**增量新增 1 处 main.go:434（面板证书续期调度）**：wg.Add/Done + defer recover + `RunPanelRenewalScheduler` 双 `select ctx.Done` 返回（acme.go:191-205）+ `runPanelRenewalPass` 单轮 recover（:211-215）——退出路径三重在位 | PASS |
| G-07 | 编译+vet+test 全绿 | 脚本门控 build/vet/test=0（任务书给定）；本轮复验 `go build ./...` exit 0 + 增量四包 `go test -count=1` 全 ok（api/panel 0.670s / service/panel 0.313s / store/panel 0.059s / api/middleware 0.067s） | PASS |

附带 🔴 子规则：错误包装 `%w`（增量文件裸 `return err` 唯一命中 service.go:142 = 存量 withTx 透传 fn 已包装错误，实读上下文判定）；`_ = err` 全仓 0 命中；`log.Print`/`fmt.Print` 调试输出 0 命中（两 grep exit 1）；HostGuard/服务层 ctx 首参实读符合。

### 前端 F-01~F-06（frontend-rules §6；cwd=frontend/）

| # | 检查 | 实跑命令与结果 | 判定 |
|---|------|--------------|------|
| F-01 | 无 Options API | `grep -rLn "script setup" --include="*.vue" src/` → 0 命中（exit 1） | PASS |
| F-02 | 无裸 any | `grep -rnE "(:\s*any\|as any)" --include="*.ts" --include="*.vue" src/` → 0 命中 | PASS |
| F-03 | 请求只走 api 层 | `grep -rnE "(axios\|fetch)\(" --include="*.vue" src/views src/components` → 0 命中 | PASS |
| F-04 | 无混入其他组件库 | `grep -rnE "from '(antd\|element\|ant-design-vue)'" src/` → 0 命中 | PASS |
| F-05 | 无原生弹窗 | `grep -rnE "(^|\W)(alert\|confirm)\(" --include="*.vue" --include="*.ts" src/` → 0 命中 | PASS |
| F-06 | typecheck+build 全绿 | 脚本门控 web=0（任务书给定）；本轮 vitest spot-run panel store 8/8 passed | PASS |

附带：定时器清理 `grep setInterval\|setTimeout src/views/panel-core/` 0 命中（PanelCertCard 显式无轮询，同步等待 loading 态）；表单校验 BindDomainCard `FormRules`+`formRef.validate()` 前置实读在位（frontend-rules §4.3 🔴）；硬编码 hex 全 views 仅 PanelCertCard.vue:286 一处（见 W2，🟡 人工核对小面积文字色非背景）。

## 二、增量契约对齐（GR-05 / AL-001~005 / CT 系列）

| # | 检查 | 结果 | 证据 |
|---|------|------|------|
| 1 | **新接口 vs api v2.0**（AL-001） | PASS | routes.go 实读 10 接口：GET 4（config/detail、**cert/status**、menu/list、system/version）挂 sess 组（AuthRequired=两角色）；POST 6（config/update、regenerate-entry-path、upload-certificate、**cert/request、cert/retry-renewal**、system/update-check）挂 write 组（CsrfGuard→RequireRole(admin)）——与 api §3.3 权限矩阵 10 行逐行一致（含方法/角色/CSRF 列）；接口说明与 4.1.4 详情行语义相符（无 req、校验链、审计动作） |
| 2 | **Action vs PageSpec vs 前端调用**（AL-002/CT-001/CT-004） | PASS | 四方零差：admin PageSpec 第 2 节 10 Action（:28-37）↔ routes.go 10 路由 ↔ frontend `grep "request<.*>('/panel"` 10 URL（panel.ts:23-72）↔ api 4.1.4 表——路径/方法/权限逐行一致；readonly 4 只读 Action + 负空间 6（readonly:第 2 节）= admin 全集，负空间与 api 3.3 ✗ 项一一对应；域名绑定/解绑无独立接口=并入 configUpdate.bindDomain（D-P12）三方同口径 |
| 3 | **DDL vs 迁移 vs 仓储** | PASS | api §5.3 DDL ↔ `007_panel_core_acme.sql` 两表重建逐列核对一致（panel_config_v2 12 列含 bind_domain、https_mode CHECK 四值；panel_cert_v2 9 列含 last_error、source CHECK 三值——列序与 CHECK 取值逐字同 §5.3）；迁移 runner `applyMigration` 单事务包裹（migrate.go:62-75，modernc sqlite 多语句 Exec），001~007 文件名序实存（`ls migrations/`）；存量 002_panel_core.sql 零改动（git status 未列）；repos 列清单与 DDL 一致（config_repo.go configCols +bind_domain、cert_repo.go Get +last_error，实读） |
| 4 | SQL 清单 #1~7 ↔ 实现 | PASS（附 W3） | #1 EnsureSingleton（+bind_domain NULL 初值）、#2 Get、#3 Update（+bind_domain 分支，空串置 NULL）、#4 Upsert（last_error 恒 NULL）、#5 Get（+last_error）、#6/#7 UpdateAcmeResult 成/败两分支——与 api §5.2 七条逐一对应实读；**#6 成功分支生产未接线**（见 W3，语义由 #4 替换式超集覆盖，测试已覆盖） |
| 5 | DTO ↔ 前端类型（AL-003/CT-002） | PASS | bindDomain 分组（BindDomainView{value *string,effectMode} ↔ PanelBindDomainConfig，dto.go:39-42↔panel.ts:67-70）；update.bindDomain 三态（json.RawMessage 区分 null/缺省 ↔ `bindDomain?: string\|null`，前后端解绑均发 null/空串）；cert/status data（CertStatusResult.Cert{source,status *string,issuer,notBefore,notAfter,lastError}+Acme{domain,challenge,autoRenew} ↔ PanelCertStatusData/PanelCertStatusItem/PanelCertAcmeSummary 逐字段）；cert/request·retry 响应（CertAcmeResult↔PanelCertActionResultData 含 httpsMode:'acme'）；错误 data 载荷 ReasonData/CurrentSourceData/CurrentStatusData/NotAfterData ↔ 前端 dataString 消费 |
| 6 | 枚举前后端一致（AL-004/CT-003） | PASS | HTTPS_MODE +acme（tag 'info'=PageSpec blue）、PANEL_CERT_SOURCE +acme（'info'）；PANEL_CERT_STATUS=Exclude<CertStatus,'pending'> 4 值子集（valid=success/green、expiring_soon=warning/orange「30 天内到期」、renew_failed=error/red、expired=default/gray）↔ PageSpec 第 3 节颜色表逐值一致；池源 CertStatus（enums.ts:246）5 值与 04-business-enum cert_status 同值；状态列经 dict 映射表渲染无 render 硬编码文案（frontend-rules §4.2 🔴） |
| 7 | 错误码可识别（AL-005） | PASS（附 W1/W4） | PANEL_007/008 拦截器文案与 PageSpec 第 1 节逐字（'请先绑定域名再申请证书'、'nginx 不可用或 80 端口未监听（HTTP-01 挑战通道不可用）'）；PANEL_009（data.reason）/010（「当前状态无需重试」兜底）/011（notAfter 联动文案）页面级处理与 Card 5 规格逐字一致、避免拦截器双提示（types/panel.ts 注释明示分界）；后端 errcode PANEL_001~011 与 api §3.4 语义/标识/HTTP 语义行一致（PANEL_003 文案扩展双端已同步，前端拦截器文案滞后见 W1） |
| 8 | UI 行为 vs PageSpec 5.1 | PASS | Card 3：占位/校验文案/绑定 NModal 强确认文案/成功 toast/解绑 NPopconfirm 逐字；Card 5：WAIT_HINT「正在签发…最长约 2 分钟…」逐字、未绑定 NAlert+锚点跳转、申请按钮未绑定禁用（PANEL_007 预判）、手动重试仅 source=acme&&renew_failed 可见、readonly 负空间（authStore.isAdmin 双组件门控）；HttpsCard 四态 hint+to acme 前置引导；8 Card 顺序 1~8 与 PageSpec 一致；settings 路由零改动 |
| 9 | 跨模块消费（GR-05 跨栈） | PASS | AcmeIssuer/NginxChannel 接口（ports.go:33-59）↔ site-mgmt acme.Gateway/nginxx.Gateway 由 main.go 同实例注入（编译通过即接口满足）；装配顺序 Entry→Host→gin 链（main.go:361-362）= api §5.1 中间件链序行 + TestHostGuardChainOrder 测试锁定 |

## 三、R-02/03/08/10/14/18 适用项

| 规则 | 结果 | 证据 |
|------|------|------|
| R-02 会话+CSRF | PASS | 新 POST cert/request·retry-renewal 挂 CsrfGuard→RequireRole（routes.go write 组实读）；前端 POST 统一注入 X-CSRF-Token（http.ts:114-115）；csrfToken/loginTicket 仅内存不落 localStorage（stores/auth.ts:3 注释+实读）；新 GET 挂 AuthRequired |
| R-03 变更写审计 | PASS | bindDomain 绑定/解绑经 panel_config_changed detail.fieldDelta（config.go 增量段 2 处，含 before/after）；panel_cert_requested 成功同事务/失败独立审计重试一次降级日志（acme.go:96-117、290-308）；panel_cert_renewal_retried 成败均审计（:156-157、265-288）；调度 panel_cert_renewed source=system 成败均审计（auditSystemEntry，service.go:285-296 + acme.go:253-254）；cert/status 查看类不审计（口径同存量 detail） |
| R-08 参数数组禁 sh -c | PASS | 增量零新增 exec：ACME=lego 库协议；nginx -t/-s reload 为 nginxx 网关字面量 argv（gateway.go:303/327）；全仓唯一 sh -c 为存量 cronx（T07 登记，cron 语义+argv 独立元素） |
| R-10 监听/入口/HTTPS | PASS（代码侧） | listen/entry_path 行为零改动（diff 实看仅 https_mode 扩 acme 分支）；acme 为 ADR-029 拍板第三证书路径，切换校验 I-P04（PANEL_003）双端在位；存量 R-10 文案笔误「建议但强制」为文档侧（Gate 1 W6 延续，见 W4） |
| R-14 服务端校验 | PASS | bindDomain 服务端独立校验 parseBindDomain/validDomain（RFC1123/≤253/禁 :/?#@，config.go:288-337——不信任前端正则）；cert/request 校验链 PANEL_007→008→011、retry-renewal PANEL_010→007（acme.go 实读）；httpsMode to acme 服务端 PANEL_003 双保险（config.go:213-227）；HostGuard Host 头服务端强校验（hostguard.go:27-47） |
| R-18 Web 终端 | N/A | 本轮未触 terminal 模块（git status 无 terminal 文件）；无新增会话/PTY 面 |

## 四、存量抽查回归

| # | 回归项 | 结果 | 证据 |
|---|--------|------|------|
| 1 | 存量 7 接口不动 | PASS | routes.go diff 纯增量（+3 行注册），存量 7 路由路径/方法/分组零改动；module_test.go 存量用例（TestPanelEndpointsRequireSession/TestPanelReadEndpointsBothRoles/TestPanelWriteGuards/TestPanelValidationOverHTTP 等）随本轮 spot-run 全 ok |
| 2 | 存量前端 Card/枚举不动 | PASS | settings index.vue 6 存量 Card 保留+顺序顺延为 1~8；LISTEN_SCOPE/INIT_STATUS 等存量枚举零改动（enums.ts diff 仅 HttpsMode/PanelCertSource 扩值+新增 PANEL_CERT_STATUS）；PANEL_ERROR_TEXT 存量 001/002/004/005/006 文案逐字未动（003 见 W1） |
| 3 | 存量迁移/SQL 语义 | PASS | 002_panel_core.sql 未触（git status）；007 对存量行的 INSERT SELECT 携带迁移 + last_error 显式 NULL 实读；EnsureSingleton/Update 存量分支语义保持（diff 上下文实读） |
| 4 | 存量行为链路 | PASS | TLS CertMaterial/detail/buildDetail 存量路径仅插入 bindDomain 分组（service.go diff）；HttpsCard 存量 imported 前置/PANEL_002 文案保持；测试回归：go 增量四包 ok + vitest panel store 8/8（存量 loadConfig/loadMenu 用例含于其中） |
| 5 | 上轮 Gate 2 遗留处置 | 未变（非本轮范围） | 上轮 W1/W3 已闭环、W4 终局 warn（挂载级用例，@vue/test-utils 不在 devDeps）——本轮增量未引入新视图测试债（新增逻辑落 store 层已有测试面 + 后端 31 个增量测试函数）；上轮结论维持 |

## 五、GR-03/GR-04 测试覆盖（增量接口）

新增/变更接口测试实存（`grep -n "^func Test"` 实跑）：后端 31 个增量测试函数——module_test.go（TestCertEndpointsGuards/TestHostGuardOverHTTP 等 HTTP 层）、hostguard_test.go 3（含链序）、acme_test.go 18（CalcCertStatus/validDomain/parseBindDomain/UpdateConfigBindDomain/RequestCert 守卫+成功+失败无残留+注入回滚/RetryRenewal 成败/落点选择与复用/调度轮/CertStatusView）、acme_store_test.go 3（**TestMigration007Shape 迁移形态、TestMigration007CarriesExistingRow 存量行携带、TestCertLastErrorAndAcmeResult**）；前端 panel.test.ts +loadCertStatus 3 用例（v2.0 契约 fixture 锁定）。全链等价覆盖（配置类无 CRUD 列表链，守卫→成功→失败→回滚齐全）。本轮实跑全绿（见编译门控节）。

## 六、WARN / INFO 清单

**WARN（供确认，均非 CRITICAL）**

- **W1（拦截器文案滞后）** `frontend/src/types/panel.ts:33` PANEL_003 拦截器文案仍为 v1.0「请先导入证书与私钥」，未随 v2.0 acme 分支扩展更新——PageSpec v1.1 第 1 节为「需先导入/申请对应来源证书」、后端 errcode 文案已扩（panel.go「尚无对应来源的可用证书，请先上传/申请证书」）。实际暴露面小：to acme 前端有页面级预判（HttpsCard「请先在下方…申请 ACME 证书」），拦截器文案仅在服务端双保险竞态（如证书恰好过期）触发。建议一行改为第 1 节文案。
- **W2（硬编码色值）** `PanelCertCard.vue:286` wait-hint `color: #d97600`——全 views 唯一 hex 命中（存量基线 0），小面积文字色非大面积背景（🟡 规则人工核对通过），但暗色模式下不随主题联动，建议 `useThemeVars().warningColor`。
- **W3（SQL #6 生产未接线，文档措辞级）** `UpdateAcmeResult` 成功分支（SQL #6）无生产调用点（grep 实跑：唯一调用 acme.go:269 为失败分支 #7）；request/retry/调度成功路径均走 Upsert（#4）——代码注释已论证（acme.go:162-164：lego 每次签发新密钥对，材料须随行替换，#6 语义由 #4 超集覆盖），且 #6 有测试覆盖（TestCertLastErrorAndAcmeResult）；但 api 4.1.4 retry-renewal 行文与 §5.2 #6 场景列仍按 UPDATE 措辞。语义无漂移，建议下次触达 api 文档时补一句「成功回写经 #4 替换式承载」或在代码侧收敛 #6。
- **W4（文档侧遗留延续，非代码缺陷）** Gate 1 增量报告 W1~W7（prd 头版本 v1.1 滞后、domain-model 行内枚举未回填 acme、admin:206 残留 Card 6 引用、readonly:37 字典清单缺 cert_status、R-10「建议但强制」×3 等）本轮未修复——属设计文档层待办；新增同源观察：PageSpec 内 PANEL_008 文案第 1 节与 Card 5 两处口径不一（前端取第 1 节，Card 5「无法完成域名验证」为页面语境变体）。

**INFO（记录在案）**

- I1 任务书「api v1.2」口径与实际 v2.0 不符（Gate 1 I1 同判），本轮以 v2.0 为基准，产物内部自洽。
- I2 存量命中转述：cronx runner.go:145 sh -c（cron 语义 T07 登记）、dbmysql 引擎侧 SQL（A-DB10 裁定）——均非本轮文件，判定与前序轮次一致。
- I3 引号体例：HttpsCard/PanelCertCard 两处提示「面板证书」vs PageSpec『面板证书』（中文引号层级体例差异，无语义影响）。
- I4 G-07/F-06 全量门控（build/vet/test/web）由流水线脚本实跑、任务书给定结果 0；本轮复核性实跑为 go build=0 + 增量四包 go test + vitest panel store——未复跑全仓 vet/test/web 全量，如实注明。

## 七、总结

**判定：WARN**

- CRITICAL 0：后端 G-01~G-07、前端 F-01~F-06 全过（grep 实跑摘录见第一节）；GR-01/GR-02 满足。
- 契约断裂 0：接口/Action/DDL/DTO/枚举/错误码/UI 行为八方对齐零差（第二节）；GR-05 满足。
- R-02/03/08/10/14 全过、R-18 N/A；存量抽查回归 5 项全过。
- GR-03/GR-04：增量接口测试 31+3 用例实存、本轮实跑全绿。
- 4 项 WARN（W1 前端一行文案、W2 一行色值、W3 文档措辞、W4 文档侧遗留）均不阻断交付，建议随下轮小修；确认记录即本报告第六节。
