# Gate 2 WARN 修复轮复核汇总 — 2026-11-16

**复核员**：WARN 复核员（只查不改；本轮仅写本报告、各 gate2 报告复核行与 INDEX.md gates.gate2 行）
**复核范围**：6 模块（account-auth / panel-core / service-mgmt / database-mgmt / docker-mgmt / cron-backup）各 gate2 报告（05-reports/gate2-*-2026-11-14.md）「WARN 修复轮（2026-11-16）」表逐项对照代码/文档实态——「已修复」项核验证据成立（file:line 可查）、「维持」项核验理由充分
**规则链**：引擎 GR-01~GR-06（validation SKILL.md Gate 2 节）⊕ backend-rules §7 / frontend-rules §6（🔴 CRITICAL 与 ⚠️ 项）⊕ business-concepts R-01~R-18（已在前轮 Gate 2 全量核验，本轮聚焦修复轮增量，不重复全扫）
**判定口径**（本 ask）：全部闭环→pass；仍有未闭环建议项→warn

## 总判定：WARN（6 模块中 1 pass / 5 warn）

| 模块 | 修复轮项数 | 已修复核验成立 | 维持核验理由充分 | 未闭环遗留 | 终局 |
|------|-----------|---------------|-----------------|-----------|------|
| account-auth | 9（W1+N1×7+W4） | N1×7 全部在位 | W1（tasks.md:71）、W4（事实消解）成立 | W1 覆盖率余量+views 组件测试 | **warn** |
| panel-core | 3（W1/W3/W4） | W1 四点+W3 在位 | W4 部分修复（5 用例已补） | W4 EntryPathCard 挂载级用例 | **warn** |
| service-mgmt | 6（W1+I1~I5） | W1 主体/I1/I5 在位 | I2/I4 理由充分、I3 他轮闭环核验 | W1 挂载级用例、I4（engine-rules.md 归引擎所有者） | **warn** |
| database-mgmt | 4（W1×2/W2×2） | W1-service、W2-store 在位 | W1-adapter（环境绑定）、W2-挂载级 | adapter 覆盖率环境绑定、挂载级用例 | **warn** |
| docker-mgmt | 3（W1/I1/I2） | W1/I1 在位 | I2 维持-理由（事实消解）成立 | 无 | **pass** |
| cron-backup | 3（W1/W2/W3） | W1、W2-store 在位 | W3（环境绑定）成立 | W2 挂载级用例、W3 环境绑定 | **warn** |

## 一、account-auth（报告 §10）— 终局 warn

| 项 | 处置宣称 | 复核证据（本轮 Read/grep/实跑） | 结论 |
|----|---------|-------------------------------|------|
| N1-1 http.ts:157 | 已修复（前轮在位） | frontend/src/api/http.ts:157-158 现述「现行后端 symbol 符号串直上……数值 401NN 为 E1 期间后端历史通道……W-3 订正」，与 response.go 符号通道现状一致 | 成立 |
| N1-2 api/auth.ts:119-122 | 已修复（本轮） | :119-122 述「后端 AuditRecord 已带全量 camelCase json tag（store/auth/models.go）……PascalCase 回退仅作 E2 前历史形态兼容垫片」 | 成立 |
| N1-3 api/auth.ts:153 | 已修复（本轮） | :153 述「与 request() 同口径——现行后端 symbol 符号串直上，数值 401NN 仅历史兜底归一」 | 成立 |
| N1-4 errcode.ts:6-7 | 已修复（前轮在位） | utils/errcode.ts:2「W-3 注释订正 2026-11-15」、:6-7 述「Fail 直接上线 errcode symbol 符号串（成功恒为数值 0），不再发送数值错误码」 | 成立 |
| N1-5 audit.ts:6-10 | 已修复（本轮） | utils/audit.ts:6-10 述「E2 双侧修复后 AuditRecord 已带全量 camelCase json tag（ActorID json:"-" 隐藏），条目键即契约 camelCase 直通」 | 成立 |
| N1-6 authguard.go:89 | 已修复（本轮） | middleware/authguard.go:89「脱敏为 INTERNAL 符号码（ErrInternal），细节进日志」，与 errcode/auth.go:41 `ErrInternal = New("INTERNAL", …)` 一致 | 成立 |
| N1-7 recovery.go:15 | 已修复（本轮） | middleware/recovery.go:15「响应脱敏为 INTERNAL 符号码（不外泄细节）」 | 成立 |
| W1 覆盖率余量 | 维持 | backend/tasks.md:71 逐字核验：「剩余未覆盖为存储异常/回滚错误分支，需故障注入框架，MVP 不引入（说明理由）」；views 组件测试归后续迭代 | 理由充分，**未闭环** |
| W4 src/components 缺席 | 维持（事实消解） | `ls frontend/src/components` → AppTerminal.vue 实存，F-03 扫描路径现已有效 | 成立（消解） |

## 二、panel-core（报告 §9）— 终局 warn

| 项 | 处置宣称 | 复核证据 | 结论 |
|----|---------|---------|------|
| W1 session 透传 int64 | 已修复（四点） | ① api-panel-core.md:235 `session:{absoluteTimeout:int64, idleTimeout:int64}`+尾注「2026-11-16 WARN 修复轮 W1 钉死」；② types/panel.ts:67-73 PanelSessionTransit 两字段 `number`；③ SessionCard.vue:3 引入 formatUptime、:13-14 `formatUptime(detail.session.*)`，utils/format.ts:84-94 秒→「N 天 N 小时 N 分钟」；④ stores/__tests__/panel.test.ts:29 `session: { absoluteTimeout: 43200, idleTimeout: 7200 }` number 字面量 + 全文件 5 `it(` 用例 | 成立 |
| W3 过期注释 | 已修复（前轮，零改动） | errcode.ts:1-12 / http.ts:157-158 本轮复读与后端现状一致（见 account-auth N1-1/N1-4 同点位） | 成立 |
| W4 0 组件/store 测试 | 部分修复 | panel.test.ts 5 用例实测（loadConfig 成败/pendingRestart/loadMenu 成败+清残留）；`grep -nE '"@vue/test-utils"|"jsdom"|"happy-dom"' frontend/package.json` → 0 命中（exit=1），挂载级维持理由属实 | 部分成立，**挂载级未闭环** |

## 三、service-mgmt（报告 §9）— 终局 warn

| 项 | 处置宣称 | 复核证据 | 结论 |
|----|---------|---------|------|
| W1 前端 0 测试 | 已修复（挂载级边界维持） | actionError.ts:32-49 routeActionError 八分支纯函数+:21-29 isFailResult（:27 接受 `string || null`）；actionError.test.ts 实测 11 `it(`；service.test.ts 实测 16 `it(`；index.vue:88-118 改按 route.kind 落地副作用；挂载级维持（devDeps 无 @vue/test-utils，grep exit=1） | 主体成立，**挂载级未闭环** |
| I1 activeState 可空 | 已修复 | types/service.ts:60 `activeState: string \| null`；index.vue:80-84 failStateDict null 短路、:93-97 null 跳过 applyActiveStateRaw、:387-388 Modal 空值「—」 | 成立 |
| I2 src/components 缺席 | 维持 | actionError.ts 为 views/service-mgmt 页内模块，无新增跨页复用件 | 理由充分 |
| I3 http.ts:157 注释 | 已他轮闭环（零改动） | http.ts:157-158 现状一致（W-3 订正在位） | 成立 |
| I4 engine-rules.md 缺席 | 维持 | 引擎规则文件治理归引擎所有者，模块修复轮无权代建；报告头部已声明按 SKILL 分层说明执行 | 理由充分，**未闭环（INFO 级）** |
| I5 `_ = conn.Close()` | 已修复（+先在编译错误修正） | gateway.go:79/:91 均 `//nolint:errcheck // teardown 弃错…`；:91 已改 `_ = g.conn.Close()`（注释明示原 `conn` 未定义为先在编译错误）；本轮全仓 go build/vet/test 复跑全绿佐证 | 成立 |

## 四、database-mgmt（报告 §10）— 终局 warn

| 项 | 处置宣称 | 复核证据 | 结论 |
|----|---------|---------|------|
| W1 service 覆盖率 71.7%→89.2% | 已修复 | branch_test.go 实存（branch_test.go:33-795）；本轮复跑 `go test -race -count=1 -cover ./internal/service/dbmgmt/` → **ok coverage: 89.2%** 实证超 80%；dbmgmt_test.go fake 错误注入字段（:48-57 mysql/:313-315 redis）与 ShowGrants 缺席用户返 ErrUserNotFound 哨兵（:160-165）在位 | 成立（**计数偏差登记**：报告称「17 个测试函数」，实测 `grep -c "^func Test"` = **16**，实质结论不受影响） |
| W1 adapter 24.1%/27.7% | 维持 | 引擎二进制环境绑定（Windows 无 mysql/mysqldump/redis-cli 实联），tasks.md 已声明归 Linux 冒烟；不硬造桩符合如实声明纪律 | 理由充分，**未闭环** |
| W2 stores/database.ts 0 用例 | 已修复 | stores/__tests__/database.test.ts 实测 8 `it(` 用例（loadList 成败/防抖/I-04 投影保持/setEngineFilter/ensureLoaded/findById/applyCredentials/governableIds） | 成立 |
| W2 views 挂载级 0 用例 | 维持 | devDeps 无 @vue/test-utils（grep exit=1），沿 panel-core W4 先例 | 理由充分，**未闭环** |
| 附带订正 | 如实 | grantmap.go:87 sortGrantItems 全仓 grep 仅 branch_test.go 引用（:395 注释/:793-811 直测），无生产调用方——「未接线仅直测」声明属实 | 成立 |

## 五、docker-mgmt（报告 §9）— 终局 pass

| 项 | 处置宣称 | 复核证据 | 结论 |
|----|---------|---------|------|
| W1 菜单图标键 | 已修复 | MainLayout.vue:83 `container: CubeOutline`（:79-80 注释「后端已登记 icon 键 container（A-D19）…此处补映射闭环」，docker 预置键 :82 保留）；后端 main.go:305 `Icon: "container"` 未动（grep 实证） | 成立 |
| I1 subnet 空串呈现 | 已修复 | views/docker/index.vue:711 `!row.subnet ? … '—'`；types/docker.ts:163-165 注释钉死「后端发空串（api §4.2.1 钉死 string 口径），null 为超集容忍；判空渲染」；api-docker-mgmt.md:279 §4.2.1 network 行「subnet 恒 string，无 IPAM 子网时为空串 ""（非 null，gate2 I1 修复轮钉死），前端判空渲染占位—」 | 成立 |
| I2 tasks.md 迁移计数 | 维持-理由（事实消解） | `ls backend/internal/store/migrations/` → 001~006 六文件（006_cron_backup.sql 已随后续 cron-backup 模块落地），文面「维持 001~006」与实况一致 | 成立（消解） |

三项全部闭环 → **pass**（INDEX gates.gate2 status 已更新 warn→pass）。

## 六、cron-backup（报告 WARN 修复轮节）— 终局 warn

| 项 | 处置宣称 | 复核证据 | 结论 |
|----|---------|---------|------|
| W1 service 覆盖率 72.8%→93.8% | 已修复（闭环） | warnfix_test.go 实测 15 `^func Test`（与报告一致）；cronback_test.go:121/:146-147 manifestErr 注入点在位；本轮复跑 `go test -race -count=1 -cover ./internal/service/cronback/` → **ok coverage: 93.8%** 实证超 80% | 成立 |
| W2 前端 0 挂载测试 | 部分修复（store 级闭环） | stores/__tests__/cron.test.ts 实测 12 `it(`；全量 vitest 本轮实测 **15 文件/167 用例全过**（**登记**：修复轮自验表「13 文件 167/167」文件计数为笔误，用例总数一致）；挂载级维持（devDeps 无 @vue/test-utils） | store 级成立，**挂载级未闭环** |
| W3 cronx 35.1%/backupx 58.8% | 维持 | crontab/journalctl/rsync 二进制 Windows 开发机不可达，纯函数层已覆盖；不硬造桩符合如实声明纪律，归 Linux 冒烟 | 理由充分，**未闭环** |

## 七、全仓门控复核（本轮实跑，2026-11-16）

| 检查 | 命令（cwd） | 结果 |
|------|------------|------|
| 后端编译 | `go build ./...`（backend/） | exit 0（BUILD_OK） |
| 后端 vet | `go vet ./...`（backend/） | exit 0（VET_OK） |
| 后端全量测试 | `go test -count=1 ./...`（backend/） | 39 包 ok、0 FAIL（grep FAIL 无命中） |
| dbmgmt 覆盖率复跑 | `go test -race -count=1 -cover ./internal/service/dbmgmt/` | ok，coverage 89.2%（W1 声明实证） |
| cronback 覆盖率复跑 | `go test -race -count=1 -cover ./internal/service/cronback/` | ok，coverage 93.8%（W1 声明实证） |
| 前端全量测试 | `npx vitest run`（frontend/） | 15 文件 / 167 用例全过 |
| 前端构建 | `npm run build`（frontend/，vue-tsc --noEmit + vite build） | exit 0（✓ built in 24.95s；chunk 体积提示为既有 advisory） |

## 八、登记事项（不改变终局判定）

1. **database-mgmt 计数偏差**：修复轮表称 branch_test.go「17 个测试函数」，实测 `grep -c "^func Test"` = 16（branch_test.go:33-795 共 16 个）。覆盖率 89.2% 与门控全绿为实测实质成立，仅函数计数笔误。
2. **cron-backup 文件计数笔误**：修复轮自验表称 vitest「13 文件 167/167」，本轮实测 15 文件/167 用例（用例总数一致；时序上 database-mgmt 轮 14 文件/155 → cron-backup 轮 +1 应为 15 文件）。
3. 未闭环项均为测试覆盖/环境绑定/引擎文件治理类建议项，无 CRITICAL、无契约断裂；挂载级组件测试统一阻塞点为 @vue/test-utils 与 DOM 测试环境不在 devDependencies（4 模块一致声明，本轮 grep 实证），建议后续迭代统一引入组件测试基建后批量补齐。

## 九、结论

- 6 模块修复轮「已修复」项证据全部核验成立（含 2 处计数笔误登记）；「维持」项理由均充分（tasks.md 佐证/环境绑定如实声明/引擎所有者归属/事实消解）。
- 全仓门控本轮实跑全绿：go build/vet/test 39 包 0 FAIL、覆盖率两项声明实证（89.2%/93.8%）、vitest 15 文件 167/167、npm run build exit 0。
- 终局：docker-mgmt **pass**；account-auth / panel-core / service-mgmt / database-mgmt / cron-backup **warn**（遗留建议项清单见各节「未闭环」列）。
- INDEX.md 六模块 gates.gate2 已更新（docker-mgmt warn→pass，余五模块维持 warn 并追加复核轮摘要）；各 gate2 报告已追加复核行。
