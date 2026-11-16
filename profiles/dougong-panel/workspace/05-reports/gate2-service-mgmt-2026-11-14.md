# Gate 2 验证报告 — 代码→交付门控

**模块**：service-mgmt（systemd 服务管理）
**轮次**：2026-11-14 二轮复验（本轮全量重跑扫描与契约抽查，覆盖当日早轮同名报告；产物未变更，证据按当前工作树行号）
**扫描路径**：backend/internal/{api/servicemgmt, service/servicemgmt, adapter/sysd, errcode/servicemgmt.go, api/middleware, cmd/dougong/main.go 装配段, store/migrations 核对} + frontend/src/{types/service.ts, types/enums.ts 服务段, api/service.ts, stores/service.ts, views/service-mgmt/**, router/index.ts 追加段}
**扫描时间**：2026-11-14
**契约基准**：profiles/dougong-panel/workspace/02-detailed/api/api-service-mgmt.md v1.0（2026-11-12）
**规则链**：引擎 GR-01~GR-06 ⊕ backend-rules §7（G-01~G-07）⊕ frontend-rules §6（F-01~F-06）⊕ business-concepts R-01~R-18（适用项）
**编译门控（脚本预跑，本报告引用未重跑）**：build=0 vet=0 test=0 web=0

> 编号源声明：SKILL.md Gate 2 的 BE-xx/F-xx 清单为 Java/Vue 大屏栈模板（{service}-core 多模块结构），本仓为 Go 单体 + Vue3 单应用，按 SKILL Gate 2 分层说明以激活栈清单落地：后端执行 backend-rules §7 G-01~G-07，前端执行 frontend-rules §6 F-01~F-06。
> 引擎层说明：`.claude/rules/` 仅含 backend-rules.md 与 frontend-rules.md，engine-rules.md 文件不存在；GR-01~06 按 SKILL.md Gate 2 分层说明语义执行（扫描必执行/编译必过/E2E 必补/前后端契约对齐），见 §5。

---

## 1. 后端规则扫描（backend-rules §7，本轮实跑 grep）

| 编号 | 检查项 | 结果 | 证据（命令 + 输出摘录 / 文件:行） |
|------|--------|------|----------------------------------|
| G-01 | 无 shell 拼接执行 | PASS | `grep -rnE "exec\.Command(Weak)?Context?\(.+\"(-c\|/c)\"" --include="*.go" .`（backend/ 全仓）→ 0 命中（exit=1）；`grep -rn -e "-c\"" --include="*.go" . \| grep -iE "sh\|bash\|cmd"` → 0 命中（exit=1）；全量核对 `grep -rn "exec\.Command" --include="*.go" .` → 唯一命中 internal/adapter/nginxx/parser.go:3 为**注释**（「外部命令经 exec.CommandContext 参数数组执行」），无代码调用。本模块 systemd 交互 100% 走 dbus 参数化方法调用（adapter/sysd/gateway.go:123/127/182/200/222 CallWithContext·EnableUnitFiles·StartUnit 等），无任何 exec 通道——R-08 在本模块的落地形态 |
| G-02 | 无 SQL 字符串拼接 | PASS | `grep -rnE "Sprintf\(.+(SELECT\|INSERT\|UPDATE\|DELETE)" --include="*.go" .` → 0 命中（exit=1）；本模块三包零 SQL（D-07 零持久化，见 G-04） |
| G-03 | 无硬编码凭据 | PASS | `grep -rnEi "(password\|secret\|token)\s*=\s*\"[^\"]{8,}\"" --include="*.go" . \| grep -v _test` → 0 命中（exit=1） |
| G-04 | SQL 只在 store 层 | PASS | `grep -rlE "(SELECT\|INSERT\|UPDATE\|DELETE) " --include="*.go" . \| grep -v internal/store \| grep -v _test` → 命中 3 文件，行级复核**全部为注释**：service/auth/user.go:114（「FK ON DELETE CASCADE」）、service/fileman/scope.go:20、service/fileman/trash.go:36/81（模块注释提到 UPDATE/INSERT 字样）；store 层 10 文件 + migrate.go 为 SQL 合法驻地。**本模块（api/servicemgmt、service/servicemgmt、adapter/sysd）零命中**；`ls internal/store/migrations/` → 仅 001_account_auth/002_panel_core/003_file_manager/004_site_mgmt，无 service-mgmt 迁移 = 文档 §5.2/5.3「无 SQL 无 DDL」成立 |
| G-05 | 无 panic 滥用 | PASS | `grep -rn "panic(" --include="*.go" . \| grep -v _test` → 0 命中（exit=1）；错误一律 error 返回，未知错误经 mapGatewayError default 分支 %w 包装（service.go:233/250） |
| G-06 | goroutine 有退出路径 | PASS | `grep -rn "go func" --include="*.go" . \| grep -v _test` → 5 命中全部位于 cmd/dougong/main.go:213/228/243/281/287（main 统一编排，§1.2 允许）：213 审计清理 worker、228 monitoring 采集、243 file-manager 分块清理（三者均 ctx 取消退出 + defer wg.Done() + panic recover，main.go:211-261；wg.Wait 收口 main.go:309）；281/287 HTTP/TLS serve（serveErr channel 上报 + ErrServerClosed 判定，main.go:279-292）。**service-mgmt 自身零新增 goroutine**——sysd 网关惰性连接（gateway.go:59-84 注释明示「无后台 goroutine」），JobRemoved 等待为同步 select 带 ctx.Done 分支（gateway.go:242-262） |
| G-07 | 编译+vet+test 全绿 | PASS（引用） | 脚本预跑 build=0 vet=0 test=0 web=0（任务书供引用，本轮未重跑）。本模块测试函数计数实查 `grep -c "^func Test"`：adapter/sysd/sysd_test.go=6、service/servicemgmt/servicemgmt_test.go=16、api/servicemgmt/module_test.go=7，共 29 个随全仓 test 通过 |
| 附1 | 🟡 fmt.Print 调试输出 | PASS | `grep -rn "fmt\.Print" --include="*.go" . \| grep -v _test \| grep -v fmt.Errorf` → 0 命中（exit=1） |
| 附2 | 🟡 log.Print 混用 | PASS | `grep -rn "log\.Print" --include="*.go" . \| grep -v slog \| grep -v _test` → 0 命中（exit=1）；模块内日志均 slog 结构化（service.go:139/185/214 WarnContext） |
| 附3 | 🟡 监听地址 0.0.0.0 写死 | PASS | `grep -rn '"0\.0\.0\.0"' --include="*.go" . \| grep -v _test` → 唯一命中 cmd/dougong/main.go:319 panelListenAddr——panel_config 监听枚举→绑定地址的明文映射（注释引 D-P02/api-panel-core §3.4），入口装配非业务代码，且属 panel-core 域 |
| 附4 | 🔴 §1.1 `_ = err` 字面形态 | PASS（人工核对） | 模块内 `grep -rn "_ = " internal/service/servicemgmt internal/api/servicemgmt internal/adapter/sysd` → 2 命中（sysd/gateway.go:79/91，均 `_ = conn.Close()`）+ 1 处测试文件（module_test.go:344，测试代码豁免）。两处均为**teardown 弃错**：:79 关闭「信号订阅已失败」的新建连接（主错误 ErrUnavailable 已 %w 返回，gateway.go:80）、:91 dropConn 关闭已死连接（仅由错误路径调用，gateway.go:87-94 有注释）；被弃错误不参与任何业务判定。全仓同型 20 处（file-manager 19 处 defer 清理已过其 Gate 2），按一致性口径不计违例——登记 I5 备案 |
| 附5 | 🔴 §1.1 错误包装 %w | PASS | mapGatewayError/mapGatewayErrorWithReason default 分支 `fmt.Errorf("service-mgmt gateway: %w", err)`（service.go:233/250）；网关侧哨兵包装 gateway.go:73/80/103-111（%w 保 errors.Is 解链）；errcode.Error 业务错误为终态语义直接上包络（response.Fail，errors.As 提取） |

**小结**：后端 7 项 CRITICAL 全过，0 违例。

## 2. 前端规则扫描（frontend-rules §6，本轮实跑 grep）

| 编号 | 检查项 | 结果 | 证据（命令 + 输出摘录 / 文件:行） |
|------|--------|------|----------------------------------|
| F-01 | 无 Options API | PASS | `grep -rLn "script setup" --include="*.vue" src/` → 0 命中（exit=1）；service-mgmt 两文件均 `<script setup lang="ts">`（index.vue:1、UnitDetailDrawer.vue:1，本轮已读全文核对） |
| F-02 | 无裸 any | PASS | `grep -rnE "(:\s*any\|as any)" --include="*.ts" --include="*.vue" src/` → 0 命中（exit=1）；SVC_007 data 收窄用 unknown + 类型守卫 isFailResult（index.vue:90-98） |
| F-03 | 请求只走 api 层 | PASS | `grep -rnE "(axios\|fetch)\(" --include="*.vue" src/views src/layouts` → 0 命中（exit=1）。注：规则原文含 src/components，该目录全仓不存在（`ls src/` 佐证，monitoring Gate 2 已记录），扫描面落 views+layouts（service-mgmt 局部组件 UnitDetailDrawer.vue 在 views 内已覆盖） |
| F-04 | 无混入其他组件库 | PASS | `grep -rnE "from '(antd\|element\|ant-design-vue)'" src/` → 0 命中（exit=1）；service-mgmt 全部 naive-ui（index.vue:4 NButton/NPopconfirm/NSwitch/NTag） |
| F-05 | 无原生弹窗 | PASS | `grep -rnE "(^|\W)(alert\|confirm)\(" --include="*.vue" src/` → 0 命中（exit=1）；确认弹层 NPopconfirm（index.vue:250/267 停止/重启），消息 useMessage（index.vue:37） |
| F-06 | typecheck + build 全绿 | PASS（引用） | 脚本预跑 web=0（任务书供引用，本轮未重跑；= vue-tsc --noEmit + vite build，见 frontend-rules §5） |
| 附1 | 🟡 §4.5 定时器清理 | PASS | `grep -rn "setInterval\|setTimeout" --include="*.vue" src/views/service-mgmt` → 仅 index.vue:60/68 防抖 timer，onUnmounted 配对 clearTimeout（index.vue:316-322）；store 无定时器（stores/service.ts 头注声明 + 全文核对）；模块无轮询（D-03 手动+操作驱动） |
| 附2 | 🟡 §4.1 硬编码 hex 色 | PASS | `grep -rnE "#[0-9a-fA-F]{6}" --include="*.vue" src/views/service-mgmt` → 0 命中（exit=1） |
| 附3 | 🔴 §4.2 枚举列映射 | PASS | 状态/自启列 render 走 types 映射表 dictTag→dictItem(SERVICE_ACTIVE_STATE/SERVICE_ENABLED_STATE)（index.vue:169-171/199/223），文案与语义色集中 types/enums.ts:152-169；非两态值 dictItem 未命中降级原值灰字（⚠️A14/D-05）；subState 原值透传为契约 D-04 要求（index.vue:205）非违例 |
| 附4 | 🟡 §4.2 危险操作确认 | PASS | 停止/重启均 NPopconfirm 二次确认 + error/warning ghost 类型（index.vue:246-279）；启动/自启为低风险无确认（开关回弹到响应值，不乐观更新 index.vue:153-165） |

**小结**：前端 6 项全过，0 违例。

## 3. 前后端接口对齐（契约基准 api-service-mgmt.md，AL-001~005 本轮重查）

| 编号 | 检查项 | 结果 | 证据 |
|------|--------|------|------|
| AL-001 | Controller 路径 vs API 文档 | PASS | 文档 §4.1.4/§5.1 七接口 ↔ routes.go:39-49：GET /services/unit/list、GET /services/unit/detail、POST /services/unit/{start,stop,restart,enable,disable}，前缀 /services（routes.go:29）。7/7 一致无多余接口 |
| AL-002 | 前端 Action 路径 vs Controller 路径 | PASS | api/service.ts:23/30/38/45/52/60/67 七函数路径逐一相同（unitList/unitDetail GET query {state,search}/{name}；五动作 POST body {name}）；读 GET 写 POST 分工=文档 §3.1；POST 由 http.ts:113-116 统一注入 X-CSRF-Token，GET 豁免 |
| AL-003 | DTO 字段 vs 前端类型字段 | PASS | UnitListItem（dto.go:10-18）↔ ServiceUnitItem（types/service.ts:13-26）7 字段同名同义；ListData{items,total,generatedAt}（dto.go:22-26）↔ ServiceUnitListData（types/service.ts:29-33）；DetailData 增 description/loadState *string 可空（dto.go:38-39）↔ `string \| null`（types/service.ts:37-38）且 list 不含 description（文档载荷考虑）；ActionData（dto.go:43-48）↔ ServiceUnitActionResult（types/service.ts:42-47）；AutostartData（dto.go:65-68）↔ ServiceUnitEnableResult（types/service.ts:50-53）；SVC_006 note 文案两侧逐字同源（consts.go:15 = 文档 §4.1.4「任务仍在执行，请稍后刷新列表确认终态」）。HTTP 层字段断言 module_test.go:368-405（list/detail 全键 + generatedAt RFC3339 + canOperate/canToggleAutostart 权威值） |
| AL-004 | 枚举值前后端一致 | PASS | service_active_state 三值（consts.go:24-28 ↔ enums.ts:152-157 ↔ 文档 §3.4）；service_enabled_state 两态主轴（consts.go:31-34 ↔ enums.ts:165-169；非两态值双侧透传+兜底 ⚠️A14/D-05：guard.go:36-41 / dictItem 未命中降级原值）；unit_type 11 类全集（sysd.go:61-65 ↔ enums.ts:172-196）；unit 名称形态正则两侧逐字同源（consts.go:19 = 文档 §3.4 业务常量正则） |
| AL-005 | 错误码前端可识别 | PASS | SVC_001~008 后端定义 errcode/servicemgmt.go:8-22（symbol 即契约 code 列）↔ 前端 SVC_ERROR_TEXT 8 条全量（types/service.ts:68-77）；复用 AUTH_001/002/003/008 符号（errcode/auth.go:7/9/11/21）；中间件链 InitGuard→AuthRequired→[CsrfGuard(POST)→RoleRequired(admin)] / [RoleRequired(admin,readonly)]（routes.go:34-49，顺序=文档 §3.1）；包络 {"code","message","data"}（response.go Envelope:18-22，失败 code 上线符号串、成功数值 0）↔ 前端 Envelope<T>（types/api.ts:7-11）+ normalizeCode 双通道归一（utils/errcode.ts:16-26，string 直通）。符号通道经 HTTP 全栈测试断言：module_test.go:345/351（AUTH_001）、413（SVC_002）、416（SVC_001）、422（SVC_003）、443（AUTH_008 readonly 越界）、461-472（AUTH_003 CSRF 三态/006/007/004/005/AUTH_012），随脚本 test=0 通过 |

**契约要点抽查**：① 权限矩阵=ADR-018——读组 RequireRole(admin,readonly)、写组 RequireRole(admin)（routes.go:37-44），readonly 越界 AUTH_008（module_test.go:440-444 断言，且带合法 CSRF 证明到达角色层）；② D-01 90s=编译期常量 ActionWaitLimit（consts.go:11），main.go:149 注入网关，超时 ctx.Done→ErrJobTimeout→SVC_006+TimeoutData（gateway.go:218/242-249，service.go:107-112）；③ D-06 审计降级重试一次→运行日志兜底（service.go:212-217，专项 TestAuditWriteDegradedD06/TestAuditRetrySucceedsD06）；④ D-09 逐级校验 形态→存在性→类型→两态（service.go:91-104/150-167，guard.go，专项 TestToggleGuardOrder）；⑤ I-06 恰好一条审计含 guard 拒绝记 failure+detail.error（auditAction service.go:194-218，TestAuditExactlyOnceOverHTTP）；⑥ 零持久化——无迁移文件、三包零 SQL（§1 G-04）= 文档 §5.2/5.3；⑦ D-05 权威计算 canOperate/canToggleAutostart 服务端下发（guard.go:44-50 + projector.go:32-33/50-51），前端只消费不自算（index.vue:213/237/246）。

## 4. 业务规则核验（business-concepts R-01~R-18 适用项）

| 规则 | 结果 | 证据 |
|------|------|------|
| R-02 会话+CSRF | PASS | 后端 AuthRequired/CsrfGuard 链（routes.go:37-44；csrf.go:26-37 常量时间比较+同源校验；module_test.go CSRF 三态拒绝断言）；前端 credentials same-origin + token 不入 localStorage（http.ts:120-125/55-59，CSRF 内存态 provider） |
| R-03 变更留审计 | PASS | 五动作全路径恰好一条审计（jobAction/toggleAutostart 各分支 + auditAction service.go:194-218：ActorID/Name、action 常量、object_type=systemd_service、object_id=unit 全名、result、detail）；module_test.go TestAuditExactlyOnceOverHTTP 断言 2 调用=2 条 + failure detail.error=SVC_001 |
| R-08 禁 shell 拼接 | PASS | 模块 systemd 交互 100% dbus 参数化（gateway.go 全文 CallWithContext，G-01 三重 grep 0 代码命中）——kb「dbus，禁 shell 拼接」落地 |
| R-09 零遥测零回传 | PASS | 模块出站仅本机 dbus（ConnectSystemBus gateway.go:71）与进程内 AuditService.Record（ports.go:14-32），无外部 HTTP/网络调用 |
| R-14 服务端全量校验 | PASS | SVC_001/002 纯函数（guard.go:17-22/55-71）+ SVC_003 实况 GetUnit + SVC_004/005 边界（guard.go:27-41），全在 service 层；handler 不做 required 绑定以保单一校验通道（handler.go:22-26 注释；空 name→SVC_001 有测试）；state 过滤任一值非法整体拒绝（guard.go:64-67） |
| R-15 发现即接管 | PASS | ListUnits ∪ ListUnitFiles 合并投影、file-only 条目补 inactive/dead（gateway.go:116-142 + sysd.go MergeUnitProjection:116-154）；新装自动出现/卸载自动消失=I-03 |
| ADR-018 边界（附） | PASS | 后端读两角色/写 admin（routes.go:37-44）+ 前端 readonly 负空间：操作列整列不渲染（index.vue:227 if(authStore.isAdmin)）、自启恒 NTag 无开关（index.vue:213）、工具条/Drawer 与 admin 全等 |
| R-01/R-04~R-07/R-10~R-13/R-16~R-18 | N/A | 分属 account-auth（R-01）/file-manager（R-04/05/17）/site-mgmt（R-06/07/16）/cron-backup（R-11）/docker（R-12/13）/terminal（R-18）域；R-10 监听归 panel-core（§1 附3） |

## 5. 引擎 GR 项与测试覆盖率

| 项 | 结果 | 证据 |
|----|------|------|
| GR 扫描必执行 | PASS | §1/§2 全部 grep 本轮实跑（backend/ 与 frontend/src 全仓），输出已摘录 |
| GR 编译必过 | PASS（引用） | 脚本 build=0 vet=0 test=0 web=0（任务书供引用，未重跑） |
| GR E2E 必补 | PASS | module_test.go 为 httptest 全栈（真实 :memory: SQLite + 真 gin 路由 + fake dbus 网关），7 用例覆盖 7 接口 × 认证/CSRF/RBAC/参数/通道/动作终态/审计；Windows 开发机无 system bus，dbus 实联冒烟归 Linux 目标环境（测试以 fake 隔离，adapter 纯函数 6 用例独立覆盖 MergeUnitProjection/NormalizeActiveState/UnitTypeOf/DetailFromProps） |
| GR 前后端契约对齐 | PASS | §3 AL-001~005 |
| 后端 Service 层覆盖 | 达标 | servicemgmt_test.go 16 用例（过滤/搜索/排序/权威计算矩阵/守卫顺序/降级路径 D-06/常量契约 TestContractConstants，`grep -c "^func Test"`=16） |
| 后端 Controller 层覆盖 | 达标 | module_test.go 7 用例（接口契约逐字段断言） |
| 前端组件覆盖 | **未达标（W1）** | vitest 套件仅 utils/__tests__ 4 套 + stores/__tests__ 2 套（file/trash），views/service-mgmt 与 stores/service.ts 0 测试；SKILL Gate 2 覆盖率表（前端组件 60%+）无法量化认定。注：frontend-rules §5 强制项为「composables/工具函数必须有测试」——本模块未新增 composable/util，不触发该条 |

## 6. SEC-GATE 债务核对（2026-11-14 裁定口径）

| 项 | 结论 | 证据 |
|----|------|------|
| 模块是否留「外部命令合规桩」 | **无桩** | `grep -rn -i "stub\|桩\|probe" internal/adapter/sysd internal/service/servicemgmt internal/api/servicemgmt` → 0 命中（exit=1）；sysd 网关为真实 godbus 实现（go.mod 依赖 github.com/godbus/dbus/v5，gateway.go:10 import），非命令桩——DEBT-SEC-GATE **不适用本模块** |
| 门禁探针状态 | Mimosa 未拦截（clear） | .mimosa/hook-status/sess_dwf-dwfrun-2c80caf4…json（2026-11-14T14:22:56Z）记录 Write backend/.secgate-probe.go → outcome=clear、hostState=hook_complete；backend/ 下无残留 .secgate-probe.go（`ls` 核对已清理）；本轮报告文件写入本身亦经同一钩子无阻（写入成功即证） |
| 判定 | 无债务 | 探针通过 ∧ 模块无桩 → 按裁定不产生 DEBT-SEC-GATE 条目，亦不构成 FAIL |

## 7. WARN / INFO 汇总

| 编号 | 级别 | 内容 | 建议 |
|------|------|------|------|
| W1 | ⚠️ WARN | 前端 views/service-mgmt（index.vue/UnitDetailDrawer.vue）与 serviceStore 无任何 vitest 测试；动作错误分流（handleActionError 八分支 index.vue:100-126）、搜索防抖、行级 inFlight 互斥、迟到响应守卫（stores/service.ts:146 drawerOpen 检查）等交互逻辑仅靠人工验证 | 补 useServiceStore + handleActionError 分流针对性用例（参照 SKILL 前端组件 60%+ 目标） |
| I1 | INFO | SVC_007 data.activeState 后端可为 null（调用期拒绝无终态：dto.go:52-56 *string + service.go:248 WithData 不带终态；Job 失败路径则有实况终态 service.go:117-128），前端 ServiceUnitFailResult.activeState 声明非空 string（types/service.ts:60）；运行时 isFailResult typeof 收窄兜底（index.vue:90-98），null 时降级为通用文案不弹 Modal——无契约断裂，属可空性类型声明不精确 | 前端类型改 `string \| null` 并在 Modal 空值展示「—」 |
| I2 | INFO | src/components 目录全仓缺席（monitoring Gate 2 已记录）；本模块无跨页复用组件，UnitDetailDrawer 为 views/service-mgmt/components 局部组件，符合 frontend-rules §2 | 沿用 monitoring 记录，不重复计违例 |
| I3 | INFO | http.ts:157 注释「后端包络 code 上线数值 401NN」与现行后端符号串通道（response.go/errcode.go 2026-11-14 E1 修复后）表述滞后；normalizeCode 双通道兼容使行为正确（string 直通 utils/errcode.ts:17） | 下次触碰该文件时同步注释 |
| I4 | INFO | `.claude/rules/engine-rules.md` 不存在（目录仅 backend/frontend 两栈规则）；GR-01~06 按 SKILL.md Gate 2 分层说明语义执行，未按缺失文件逐条编号 | 由引擎所有者补齐文件或以 SKILL 说明为准 |
| I5 | INFO | sysd/gateway.go:79/91 `_ = conn.Close()` 为 §1.1「禁止 `_ = err`」字面形态，人工核对为 teardown 弃错（主错误均已 %w 上报、被弃错误不影响任何业务判定），与全仓 19 处 defer 清理同型（file-manager 已过其 Gate 2），按一致性口径不计违例 | 可选：改为 `_ = conn.Close() //nolint:errcheck // teardown` 或 slog.Debug 显式化 |

## 8. 总结

**判定**：**WARN**

- 🔴 CRITICAL 违例：**0**（后端 G-01~G-07、前端 F-01~F-06 本轮实跑全过）
- 契约断裂：**0**（AL-001~005 对齐：7 接口/DTO 字段/枚举三方同值/错误码符号通道/包络/中间件链）
- 业务规则适用项（R-02/03/08/09/14/15 + ADR-018）：全过，其余 12 项不适用
- SEC-GATE：模块零桩（真实 dbus 实现），探针 clear——无债务
- ⚠️ WARN：1 项（W1 前端组件/store 测试缺失）
- INFO：5 项（I1~I5）

---

## 9. WARN 修复轮（2026-11-16）

**处置范围**：§7 WARN/INFO 表 6 项（W1 + I1~I5）逐项处置；后端代码改动仅 I5 一处（附带修正先在编译错误，见该行）。

| # | 处置 | 证据 / 理由 |
|---|------|-------------|
| W1 | **已修复（挂载级边界维持）** | ① 分流可测化抽取：handleActionError 八分支决策抽为纯函数 `routeActionError`（frontend/src/views/service-mgmt/actionError.ts:32-49，isFailResult 收窄同迁 :21-29），index.vue:88-127 改为按 route.kind 落地副作用（Modal/message/store 直更与重拉），行为与抽取前逐分支一致（silent 分支含非 ApiError 与未知码两源）；② 新增 views/service-mgmt/__tests__/actionError.test.ts **11 用例**——八分支全覆盖（非 ApiError→silent、SVC_007 fail 形态透传、SVC_007+activeState=null 仍 fail-result（I1 联动）、SVC_007 形态不符→silent、SVC_006→warn 文案、SVC_003→warn-reload、SVC_008→unavailable、SVC_001/002/004/005→error「参数非法」、AUTH_008 未知码→silent）+ isFailResult 收窄 3 形态；③ 新增 stores/__tests__/service.test.ts **16 用例**——loadList 成功落位/SVC_008 横幅+清数据（I-01）/SVC_002 兜底文案/在途防抖不重复请求、setStateFilter 逗号拼接 query/setSearch 联动、行级 inFlight set/release 互斥、applySnapshot 命中行三字段直更+不重拉（D-03）/未命中行数据不动、applyActiveStateRaw 非三态（activating）拒绝、markListUnavailable、Drawer 成功/SVC_003→notfound/SVC_008→unavailable/**迟到响应守卫**（closeDrawer 后到达不写入，锁 stores/service.ts:146）/refreshDetail/resetView 清理；vitest 实跑 13 文件/147 用例全过（新增 27，既有 120 无回归）。**维持**：搜索防抖 300ms 定时器行为等挂载级交互用例——@vue/test-utils 与 DOM 测试环境（jsdom/happy-dom）均不在 frontend/package.json devDependencies（本轮实查），引入新依赖超出修复面，沿 panel-core W4 同判先例，归组件测试基建后续迭代（防抖 onUnmounted 配对清理已有 §2 附1 人工核验在案） |
| I1 | **已修复** | ① 前端类型：types/service.ts:60 `activeState: string | null`（对齐后端 dto.go:52-56 ActionFailureData.ActiveState *string——「调用期拒绝无终态时 activeState=null」即 I1 所述可空源）；② 收窄守卫：actionError.ts:27 isFailResult 接受 `string \|\| null`（原 typeof==='string' 单通道使 null 数据降级不弹 Modal，现 null 终态照常弹 Modal）；③ 展示：index.vue:80-85 failStateDict 对 null 短路返回 null + index.vue:384-388 Modal 实际终态 `v-else` 展示「—」；④ 测试锁定：actionError.test.ts:49-56（null→fail-result）+ :76-79（isFailResult null true）；index.vue:93-97 activeState=null 时跳过 applyActiveStateRaw（无终态可直更） |
| I2 | **维持** | 建议原文即「沿用 monitoring 记录，不重复计违例」——src/components 目录全仓缺席（monitoring Gate 2 已记录），本模块无跨页复用组件，UnitDetailDrawer 为 views/service-mgmt/components 局部组件，符合 frontend-rules §2；本轮无新增跨页复用件（actionError.ts 为 views/service-mgmt 页内决策模块） |
| I3 | **已他轮闭环（本轮核验在位，零改动）** | 本轮实读 http.ts:157-158：现行注释已述「包络边界归一：现行后端 symbol 符号串直上（response.go Fail→ec.Symbol()，Gate 2 E1 双侧修复）；数值 401NN 为 E1 期间后端历史通道，normalizeCode 保留归一作兼容兜底——W-3 订正」，与本轮复核的后端符号串通道现状一致（§3 AL-005）；订正出处=site-mgmt 复检第 1 轮 2026-11-15（INDEX.md site-mgmt gates.gate2 note 记 W-3 闭环），panel-core/account-auth WARN 修复轮（2026-11-16）均核验在位。建议原文「下次触碰该文件时同步注释」——本轮未触碰 http.ts，条件未触发亦已就位 |
| I4 | **维持** | 建议原文「由引擎所有者补齐文件或以 SKILL 说明为准」——`.claude/rules/` 目录治理归引擎所有者，模块修复轮无权代建引擎规则文件；本报告头部已声明按 SKILL.md Gate 2 分层说明语义执行 GR-01~06，模块侧无待改代码 |
| I5 | **已修复（采纳 nolint 注释形态）+ 附带修正先在编译错误** | gateway.go:79/:91 两处 teardown 弃错按建议补 `//nolint:errcheck // teardown 弃错` 显式化注释（:79 订阅失败即弃新建连接、主错误已 %w 上报；:91 仅错误路径关闭已死连接）。**附带发现**：gateway.go:91 原文即 `_ = conn.Close()`——`conn` 标识符未定义，为先在于本轮的编译错误（本报告 §1 G-07 编译门控系「引用脚本预跑」未复跑故未暴露）；本轮自验 `go build ./...` 实跑即失败定位（`internal\adapter\sysd\gateway.go:91:7: undefined: conn`），按语义修正为 `_ = g.conn.Close()`（关闭被跟踪连接后置 nil，dropConn 注释 :86 语义自洽），修正后 build/vet/test 复跑全绿（见下） |

**自验（本轮实跑）**：

- `npm run test`（cwd=frontend/，vitest run）→ **13 文件 / 147 用例全过**（本轮新增 actionError.test.ts 11 + service.test.ts 16 = 27；既有 120 无回归；首轮 service.test.ts 1 例因共享 fixture 被 applySnapshot 原地改写跨用例污染而 FAIL，改 `structuredClone` 深拷贝注入后复跑全绿）
- `npm run build`（cwd=frontend/，vue-tsc --noEmit && vite build）→ **exit 0**（built in 27.85s）
- `go build ./...` + `go vet ./...`（cwd=backend/）→ **exit 0**（首轮 build FAIL 暴露 gateway.go:91 先在编译错误，I5 行内修正后复跑通过）
- `go test ./...`（cwd=backend/）→ **全部 ok**；模块三包强制重跑 `go test -count=1 ./internal/adapter/sysd/ ./internal/service/servicemgmt/ ./internal/api/servicemgmt/` → 均 ok（sysd 0.595s / service 0.565s / api 0.894s）

**修复轮结论**：修复 3 项（W1 主体收敛+挂载级边界维持、I1、I5 含先在编译错误修正）、他轮已闭环核验 1 项（I3）、如实维持 2 项（I2/I4）；附带暴露并修正 §1 G-07 引用门控未发现的 gateway.go:91 先在编译错误，backend 自验门控自此恢复实跑口径全绿。

> **复核行（2026-11-16，WARN 复核员）**：§9 六项逐一核验——W1 证据在位（actionError.ts:32-49 routeActionError/:21-29 isFailResult；actionError.test.ts 11 用例、service.test.ts 16 用例实测计数；index.vue:88-118 按 route.kind 落地）；I1 在位（types/service.ts:60 string|null、actionError.ts:27 收窄接受 null、index.vue:80-84 短路/:387-388 空值「—」/:93-97 跳过直更）；I5 在位（gateway.go:79/:91 nolint 注释，:91 已修正为 g.conn.Close 并注明先在编译错误）；I2/I4 维持理由充分、I3 http.ts:157-158 现状一致；@vue/test-utils 缺席实证。挂载级用例与 I4（engine-rules.md 归引擎所有者）未闭环 → **终局判定：warn**。汇总见 05-reports/gate2-warn-fix-review-2026-11-16.md。
