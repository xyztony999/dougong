# Gate 2 验证报告 — 代码→交付门控（monitoring）

**模块**：系统监控（monitoring）
**扫描路径**：backend/internal/{api/monitoring, service/monitoring, errcode, api/middleware, store(migrations), cmd/dougong} + frontend/src/{types,api,stores,views/monitoring,router}
**扫描时间**：2026-11-14
**契约基准**：workspace/02-detailed/api/api-monitoring.md v1.0（2026-11-12）
**规则链**：引擎 GR（SKILL.md Gate 2 分层说明：扫描必执行/编译必过/E2E 必补/前后端契约对齐——注：`.claude/rules/engine-rules.md` 未挂载于本工作区，按 SKILL.md 内联语义执行）⊕ backend-rules §7 G-01~G-07 ⊕ frontend-rules §6 F-01~F-06（含 §4.1/§4.5 专项）⊕ 业务规则 R-01~R-18 适用项
**编译门控（脚本已执行，本报告引用未重跑）**：go build=0 / go vet=0 / go test=0 / 前端 typecheck+build(web)=0

## 判定总览

**判定：FAIL**（1 项契约断裂 + 1 项呈现层缺陷；栈规则 CRITICAL 扫描 0 违例）

- 通过：G-01~G-07 全过、F-01~F-06 全过、AL-001/002/004/005 过、R-02/03/08/09/14 适用项全过、零持久化（ADR-014）实证
- CRITICAL/契约断裂：1 项（E1，AL-003 processes 字段可空性 B/F 断裂 → 前端运行时崩溃路径）
- 缺陷（须随 E1 一并修复）：1 项（E2 磁盘用量进度条百分比计算错误）
- WARN：2 项（W1 views/monitoring 组件 0 测试；W2 src/components 目录缺席——account-auth Gate 2 W4 同判）

---

## 1. 断裂项与缺陷（阻断）

### E1（FAIL 主因）· AL-003 契约断裂：processes.{cpuPercent,memBytes} 可空性前后端不一致 + 前端无守卫解引用 → TOP 表格运行时崩溃

- **后端（可空）**：`backend/internal/service/monitoring/types.go:14-19` `ProcessInfo{CPUPercent *float64 `json:"cpuPercent"`; MemBytes *uint64 `json:"memBytes"`}`，注释明示「cpuPercent/memBytes 为 null 表该进程本轮读取失败或无基线——D-08 不虚构默认值」。null 为**常态可达**：`sampler.go:203-210` 进程 CPU 占比仅在 `hasBaseline && prev seen` 时赋值，**新出现进程首轮必 nil**（采集 3s 一轮，进程消长常态）。
- **前端（非空）**：`frontend/src/types/monitoring.ts:24-29` `ProcessItem{cpuPercent: number; memBytes: number}`，注释自述「失败走 MON_004 整体，字段非空」——与后端实现直接矛盾。
- **崩溃点**：`frontend/src/views/monitoring/components/ProcessesCard.vue:24` `render: (row) => row.cpuPercent.toFixed(1)` —— 后端下发 `cpuPercent: null`（新进程）时 TypeError，n-data-table 整卡渲染失败。概览页 3s 轮询（stores/monitoring.ts:30），运行时段内必现。
- **基准对照**：api-monitoring.md §4.1.4 processes 字段未标注可空性，D-08「快照字段置空」精神被后端延伸到进程字段（合理），前端按非空理解并裸解引用（错误）。按「交叉比对字段名/类型/必填」口径，B/F 一致性检查不通过 = 契约断裂。
- **修复方向（二选一，推荐 a）**：a) 前端 `ProcessItem.cpuPercent: number | null; memBytes: number | null`，ProcessesCard render 加 null 守卫（null → '—'，与 D-08 缺失不虚构一致）；b) 后端固定输出 0 并由 degraded 承担标注（违背 D-08，不推荐）。`formatBytes` 已兼容 null（utils/format.ts:55-56），仅 cpuPercent 崩溃。

### E2 · 呈现层缺陷：磁盘用量进度条百分比恒满格

- `frontend/src/views/monitoring/components/SnapshotCard.vue:49-51` `diskPercent(row)` 直接返回 `row.usedBytes`（**字节数**）作为 `n-progress` 的 `:percentage`（期望 0–100）。真实磁盘已用数十 GB → 百分比恒超界 → 进度条恒显示满格，视觉信息错误；应 `usedBytes / totalBytes * 100`。同卡文本 `diskUsageText`（:43-46）正确，仅进度条错误。D-02「人类可读格式化归前端」职责落错。

---

## 2. 后端规则扫描（backend-rules §7 G-01~G-07，实跑摘录）

| 编号 | 检查项 | 结果 | 证据（命令 + 输出 / 文件:行） |
|------|--------|------|------|
| G-01 | 无 shell 拼接 | PASS | `grep -rnE "exec\.Command(Weak)?Context?\(.+\"(-c\|/c)\"" --include="*.go" .` → 0 命中（exit=1）；`grep -rn "exec.Command" --include="*.go" .` → 0 命中（exit=1，本模块乃至全仓无 exec 调用，R-08 天然满足） |
| G-02 | 无 SQL 拼接 | PASS | `grep -rnE "Sprintf\(.+(SELECT\|INSERT\|UPDATE\|DELETE)" --include="*.go" .` → 0 命中（exit=1） |
| G-03 | 无硬编码凭据 | PASS | `grep -rnEi "(password\|secret\|token)\s*=\s*\"[^\"]{8,}\"" --include="*.go" . \| grep -v _test` → 0 命中（exit=1） |
| G-04 | SQL 只在 store 层 | PASS | `grep -rlnE "(SELECT\|INSERT\|UPDATE\|DELETE) " --include="*.go" . \| grep -v internal/store \| grep -v _test` → 唯一命中 `internal\service\auth\user.go:114` 为注释文本「FK ON DELETE CASCADE」（非 SQL 语句，且属 account-auth 模块）；monitoring 零 SQL（ADR-014） |
| G-05 | 无 panic 滥用 | PASS | `grep -rn "panic(" --include="*.go" . \| grep -v _test` → 0 命中（exit=1）；采集侧仅 `recover()` 兜底（collector.go:45-49、sampler.go:260-266） |
| G-06 | goroutine 有退出路径 | PASS | `grep -rn "go func" --include="*.go" . \| grep -v _test` → 4 处全在 `cmd/dougong/main.go:168/183/221/227`，均 main 统一编排；monitoring collector（main.go:183-193）ctx 取消即返回 + `wg.Wait()`（:249），测试双证 `TestCollectorRunExitsOnCancel`（monitoring_test.go:465-491，断言取消后 3s 内退出） |
| G-07 | 编译+vet+test 全绿 | PASS（引用） | 脚本门控 build=0 / vet=0 / test=0（ask 声明已执行，本报告未重跑） |

补充 🟡 项：`grep -rn "fmt.Print" ... | grep -v _test` → 0 命中；`grep -rn "log\.Print" ... | grep -v slog | grep -v _test` → 0 命中（日志全 slog 结构化，sampler.go:263/269 slog.ErrorContext/WarnContext 带 ctx）。

**分层依赖**（backend-rules §3）：api/monitoring/handler.go 薄壳（绑定→service→response，无业务逻辑）；service/monitoring 不依赖 store/HTTP（零持久化，consts.go:1-4 包注释声明）；errcode 位于共享内核（errcode/monitoring.go，service→api 无反向依赖）。✅

## 3. 前端规则扫描（frontend-rules §6 F-01~F-06 + 专项，实跑摘录）

| 编号 | 检查项 | 结果 | 证据 |
|------|--------|------|------|
| F-01 | 无 Options API | PASS | `grep -rLn "script setup" --include="*.vue" src/` → 0 命中（exit=1） |
| F-02 | 无裸 any | PASS | `grep -rnE "(:\s*any\|as any)" --include="*.ts" --include="*.vue" src/` → 0 命中（exit=1） |
| F-03 | 请求只走 api 层 | PASS | `grep -rnE "(axios\|fetch)\(" --include="*.vue" src/views` → 0 命中（exit=1；首次带 src/components 路径因目录不存在 exit=2，重跑仅 src/views 干净） |
| F-04 | 无混入其他组件库 | PASS | `grep -rnE "from '(antd\|element\|ant-design-vue)'" src/` → 0 命中（exit=1） |
| F-05 | 无原生弹窗 | PASS | `grep -rnE "(^\|\W)(alert\|confirm)\(" --include="*.vue" src/` → 0 命中（exit=1） |
| F-06 | typecheck+build 全绿 | PASS（引用） | 脚本门控 web=0（ask 声明已执行，未重跑） |
| §4.5 | 轮询定时器配对清理 | PASS | `grep -rn "setInterval\|setTimeout" src/views/monitoring src/stores/monitoring.ts` → 仅 stores/monitoring.ts:63/64/115/120 两处 setInterval；clearTimers（:126-135）+ stopRealtimePolling（:149-155，含 visibilitychange 解绑）由 views/monitoring/index.vue:19-21 onUnmounted 调用；TrendChart.vue:115-119 resize 监听 + dispose 配对清理 |
| §4.5 | 监控轮询纪律 | PASS | 间隔常量集中（stores/monitoring.ts:30-33：3s/60s/滞后阈值）；document.hidden 暂停 + 恢复即拉（:104-111）；趋势无自动轮询（api/monitoring.ts:19-20 注释 + history/index.vue:64-67 仅 onMounted 拉取）——D-01 兑现 |
| §4.1 | 无硬编码 hex 色 | PASS | `grep -rnE "#[0-9a-fA-F]{6}" --include="*.vue" src/views/monitoring` → 0 命中（exit=1）；TrendChart/SnapshotCard 均走 useThemeVars（TrendChart.vue:39/93、SnapshotCard.vue:15/123-133） |

## 4. 前后端接口对齐（AL-001~005，基准 api-monitoring.md v1.0）

| 编号 | 检查项 | 结果 | 证据 |
|------|--------|------|------|
| AL-001 | Controller 路径 vs API 文档 | PASS | 4 接口逐一比对：`GET /monitoring/metrics/snapshot`、`GET /monitoring/metrics/history`、`GET /monitoring/processes/top`、`GET /monitoring/system/info`（routes.go:35-38 + Prefix()=「/monitoring」routes.go:26）= api §4.1.4/§4.2.4/§5.1；全 GET 只读、无写接口 ✓ §3.1 |
| AL-002 | 前端 Action 路径 vs Controller | PASS | api/monitoring.ts:15-42 四函数路径与 routes.go 完全一致；router/index.ts:84-96 两页面路由 /monitoring、/monitoring/history（requiresAuth 继承，无 roles 限制 = admin+readonly ✓ §3.3） |
| AL-003 | DTO 字段 vs 前端 VO 字段 | **FAIL** | snapshot/history/top/systemInfo 顶层与嵌套字段名逐字对齐（dto.go:11-100 vs types/monitoring.ts:37-113 = api §4.1.4/§4.2.4；module_test.go:348-418 形状断言双证）；**唯一断裂 = processes.{cpuPercent,memBytes} 可空性**，详见 E1 |
| AL-004 | 枚举值前后端一致 | PASS | metric_window{last_1h,last_24h}（consts.go:23-25 / enums.ts:108-112）、metric_dimension{cpu,memory,disk,network,load}（consts.go:45-50 / enums.ts:115-122）、metric_scope{total,per_core}（consts.go:66-68 / enums.ts:125-129）三方与 api §3.4 同值；前端字典常量替代 dict/batch 有 PageSpec 第 3 节偏差声明（enums.ts:3-5） |
| AL-005 | 错误码前端可识别 | PASS | MON_001~004（errcode/monitoring.go:8-14）↔ 前端 MON_ERROR_TEXT（types/monitoring.ts:15-19）+ MON_004 空态分流（stores/monitoring.ts:80-85 initializing，轮询不停止）；包络 `{"code","message","data"}` 失败 code 上线符号串（response.go:18-27,34）↔ 前端 Envelope number\|string + normalizeCode 双通道（types/api.ts:7-11、http.ts:148-155、errcode.ts:16-26）；服务端逐参数校验 MON_001~003 映射（service.go:38-50）= R-14，module_test.go:319-344 双角色×6 非法参数用例实证 |

**语义抽查（与设计决策对照）**：D-03 series 统一形状+`v=null` 缺口+startTime/partial（series.go:16-66，测试 :161-311 覆盖含磁盘挂载点并集/卸载历史 A-M9）；D-04 scope 仅 cpu 合法（service.go:44-50 + api/monitoring.ts:25-26 前端按维度裁剪发送）；D-07 空数据 series=[]+startTime=null+code=0（service.go:58-63，module_test.go:284-297）；D-08 单指标失败字段 null+degraded（service.go:94-126，module_test.go:299-314）；MON_004 快照/TOP 判据与 ProcessesOK 区分（service.go:76-82）；I-02 环形淘汰（buffer.go:45-61，测试 :26-46）；I-06 TOP ≤10 降序（sampler.go:227-245）。中间件链序 InitGuard→AuthRequired→RoleRequired(admin,readonly)（routes.go:30-34）= api §3.1/§3.3；GET 无 CSRF ✓ §3.1。

## 5. 业务规则适用项（R-01~R-18）

| 规则 | 结果 | 证据 |
|------|------|------|
| R-02 Cookie 会话鉴权复用 | PASS | 全部接口位于 AuthRequired 后（routes.go:33-38）；前端 credentials same-origin、token 不入 localStorage（http.ts:124）；访客不可达实证 module_test.go:241-251（AUTH_001） |
| R-03 变更类操作写审计 | PASS（不适用面） | 本模块 4 接口全只读 GET（api §3.1/§1.2 边界），无变更操作即无审计面 |
| R-08 命令执行参数数组 | PASS | 模块无 exec 调用（G-01 双 grep 0 命中）；数据采集经 gopsutil 库读取 |
| R-09 零遥测零外呼 | PASS | 仅本机读取（sampler.go/systeminfo.go 全部 gopsutil/os 本机调用），无任何出站请求 |
| R-14 服务端输入校验 | PASS | History 三参数服务端枚举校验（service.go:38-50）；handler 薄壳不含校验逻辑（handler.go:32-40）；测试覆盖缺省/非法组合 6 例（monitoring_test.go:323-359） |
| 其余 R-01/04~07/10~13/15~18 | 不适用 | 无登录流程/文件路径/nginx/docker/监听配置/备份/终端交互面（模块边界 api §1.2） |

**ADR-014 零持久化实证**：`ls backend/internal/store/migrations/` → 仅 `001_account_auth.sql`、`002_panel_core.sql`，无 monitoring 表；service/monitoring 无 store 依赖、无 SQL 文件 ✓ api §5.2/§5.3「无 SQL/无 DDL」兑现。菜单登记（main.go:132-137，Order 100/110，admin+readonly）与 PageSpec 第 7 节路由一致。

## 6. 测试与 E2E（GR「E2E 必补」）

| 项 | 结果 | 证据 |
|----|------|------|
| 后端模块级 E2E（真实 httptest 全栈 + :memory: SQLite） | PASS | module_test.go 10 个测试函数：会话拦截/双角色同权/MON_004 就绪性/参数校验 6 例/快照·趋势·TOP·系统信息形状逐字段断言（:241-558）；service 层 monitoring_test.go 21 用例（缓冲不变量/聚合/投影/门面/采集冒烟/取消退出） |
| 前端 monitoring 组件/store 测试 | **WARN（W1）** | views/monitoring 与 stores/monitoring.ts 0 测试；vitest 33/33 为 utils 公共件（E1 崩溃路径若有无守卫单测本可在交付前暴露） |

## 7. WARN 项汇总

- **W1**：views/monitoring 组件 + monitorStore 0 测试（frontend-rules §5「composables/工具函数必须有测试」精神；account-auth/panel-core Gate 2 W4 同判）。
- **W2**：src/components 目录不存在（F-03 首跑 exit=2 的原因）；公共组件尚未沉淀，与 account-auth Gate 2 W4 记录一致。

## 总结

**判定：FAIL**

- 栈规则 CRITICAL 扫描：后端 G-01~G-07 全过、前端 F-01~F-06 全过（0 违例）
- 契约对齐：AL-001/002/004/005 PASS；**AL-003 FAIL（E1：processes.cpuPercent/memBytes 后端可空 vs 前端非空且裸解引用 → TOP 表格运行时崩溃）**
- 附加缺陷：E2（磁盘用量进度条百分比错误，SnapshotCard.vue:49-51）
- 修复 E1+E2 后复检本门控（复检范围：types/monitoring.ts、ProcessesCard.vue、SnapshotCard.vue + 相关测试），其余项可引用本报告不重扫。

---

# 复检第 1 轮（2026-11-14）

**复检范围**：上轮 FAIL 项 E1/E2 修复核验 + 栈规则/契约抽查回归（其余项引用首轮，未重扫）。
**编译门控（脚本复跑，ask 提供结果引用）**：go build=0 / vet=0 / test=0 / web(typecheck+build)=0；本轮另实跑 monitoring 相关测试与 grep 重扫补证（见下）。

## R1.1 E1 修复核验 —— PASS（闭环）

- **类型对齐（修复方向 a，后端未动）**：`frontend/src/types/monitoring.ts:30-35` `ProcessItem{cpuPercent: number | null; memBytes: number | null}`，:25-29 注释两层降级口径（字段级 null vs 整体 MON_004）与后端 D-08 一致；backend `internal/service/monitoring/types.go:14-19` 保持 `*float64/*uint64` 不变（首轮 E1 描述的「新进程首轮无基线必 null」语义未动，前端已对齐）。
- **渲染守卫**：`ProcessesCard.vue:26/33` 改为 `formatPercent(row.cpuPercent)` / `formatBytes(row.memBytes)`，无裸 `.toFixed`；`utils/format.ts:55-58/69-72` null → '—'（D-08 缺失不虚构）。
- **全仓消费点排查**：`grep -rn "cpuPercent\|memBytes" src/` → 渲染消费仅 ProcessesCard.vue:26/33 一处（其余为类型定义与注释），无其他未守卫点。
- **漏网排查旁证**：`TrendChart.vue:47/53` 残留 toFixed 为 TS `number` 入参的纯格式化函数，调用点 :65-68 先 `typeof value === 'number'` 守卫、null/'-' → '缺失'，非崩溃路径。

## R1.2 E2 修复核验 —— PASS（闭环）

- `frontend/src/utils/format.ts:97-111` 新增 `usagePercent(used, total)`：used/total×100（1 位小数四舍五入）；任一缺失或 total≤0 → null（不虚构百分比）。
- `SnapshotCard.vue:40` `percent: usagePercent(row.usedBytes, row.totalBytes)`；模板 :111-118 `v-if="row.percent !== null"` + `:percentage="row.percent ?? 0"`——字节数直传进度条已消除（首轮 :49-51 缺陷点不复存在）。
- **回归测试**：`utils/__tests__/format.test.ts:152-177` usagePercent 4 用例（null 任一缺失→null / ×100 1 位小数 / total≤0→null / 「GiB 级 used 对 KiB 级 total 才可能 >100」E2 回归锚）。

## R1.3 回归抽查（本轮实跑）

| 项 | 结果 | 证据（命令 + 输出） |
|----|------|------|
| 后端 monitoring 测试 | PASS | `go test -count=1 ./internal/service/monitoring/... ./internal/api/monitoring/...` → 2 包 ok（module_test 形状断言未动，processes 可空契约后端侧无变化） |
| 前端 vitest | PASS | `npx vitest run` → 4 files / **37 tests 全过**（较首轮 33 → format 21→25，新增即 usagePercent E2 回归用例） |
| G-01~G-03 | PASS | 三 pattern 重扫各 0 命中（exec -c 拼接 / Sprintf-SQL / 硬编码凭据） |
| G-04 | PASS | SQL 关键字非 store 唯一命中 `internal/service/auth/user.go:114`（注释文本 FK ON DELETE CASCADE，account-auth 模块）；其余 8 文件全在 internal/store；monitoring 零 SQL（ADR-014）不变 |
| G-05/G-06 | PASS | panic 0 命中；goroutine 仍 4 处全在 `cmd/dougong/main.go:168/183/221/227`（与首轮一致，main 编排+退出路径首轮已测试双证） |
| F-01~F-05 | PASS | 五 pattern 重扫 0 命中（Options API / 裸 any / views 内请求 / 混入组件库 / 原生弹窗） |
| AL-001/AL-002 | PASS | `routes.go:26,35-38`（Prefix /monitoring + 4 GET）↔ `api/monitoring.ts:16/21/36/41` 四路径逐一比对一致，无回归 |
| AL-003（E1 断裂面） | PASS | processes 顶层字段名/可空性 B/F 现一致（见 R1.1），其余 snapshot/history/systemInfo 字段首轮已对齐且后端未动 |

## R1.4 WARN 现状（沿用，非阻断）

- **W1 维持**：views/monitoring 组件与 stores/monitoring.ts 仍 0 测试（全仓测试文件仅 `src/utils/__tests__/` 4 个）；usagePercent 已有 utils 级回归测试，E1 null 守卫路径仍无组件级测试。
- **W2 维持**：`src/components` 目录仍不存在（`ls` 报 No such file or directory）；与 account-auth/panel-core Gate 2 同判记录。

## 复检判定

**判定：WARN（首轮 FAIL 已闭环）**

- E1（契约断裂）/E2（呈现层缺陷）修复核验通过：契约断裂 0、CRITICAL 0
- 栈规则 G-01~G-06 / F-01~F-05 重扫与 AL-001/002/003 抽查回归无退化；monitoring 测试与 vitest 本轮实跑全绿
- 遗留 WARN 2 项（W1 组件/store 0 测试、W2 src/components 缺席）记录在案，用户确认后交付

---

# 复检第 2 轮（2026-11-14，交付检查员独立全量重扫）

**执行方式**：本轮为新派发的独立 Gate 2 重扫——下列所有扫描/读取均本轮亲跑；编译门控引用脚本结果 build=0/vet=0/test=0/web=0（ask 声明，未重跑）。SEC-GATE 债务口径（2026-11-14 裁定）核验：本轮门禁探针（下述全部 grep）执行未被拦；monitoring 范围 `grep -rniE "stub|桩|合规桩|外部命令" backend/internal/{api/monitoring,service/monitoring,errcode/monitoring.go}` → 0 命中（exit=1）→ **DEBT-SEC-GATE 不适用**。`.claude/rules/engine-rules.md` 未挂载本工作区，GR-01~06 按插件缓存 0.5.2 版语义执行（GR-01/05/06 由本轮扫描/核对落实，GR-02/04 引用脚本门控，GR-03 见 R2-30）。

## R2.1 后端栈规则扫描（backend-rules §7；工作目录 backend/，输出摘录）

| # | 检查 | 结果 | 证据（命令 + 输出） |
|---|------|------|------|
| R2-1 | G-01 无 shell 拼接 | PASS | `grep -rnE "exec\.Command(Weak)?Context?\(.+\"(-c\|/c)\"" --include="*.go" .` → 0 命中（exit=1）；`grep -rn "exec.Command" --include="*.go" .` → 唯一命中为注释 `internal\adapter\nginxx\parser.go:3`，全仓零实际 exec 调用（R-08 天然满足） |
| R2-2 | G-02 无 SQL 拼接 | PASS | `grep -rnE "Sprintf\(.+(SELECT\|INSERT\|UPDATE\|DELETE)" --include="*.go" .` → 0 命中（exit=1） |
| R2-3 | G-03 无硬编码凭据 | PASS | `grep -rnEi "(password\|secret\|token)\s*=\s*\"[^\"]{8,}\"" --include="*.go" . \| grep -v _test` → 0 命中（exit=1） |
| R2-4 | G-04 SQL 只在 store 层 | PASS | Windows 反斜杠路径使 `grep -v internal/store` 失效；`tr '\\' '/'` 归一后非 _test 命中 14 文件中 11 个位于 internal/store；余 3 文件逐行核对全为注释——`service/auth/user.go:114`（FK ON DELETE CASCADE 注释）、`service/fileman/scope.go:20`、`service/fileman/trash.go:36,81`（均 `//` 行，且属其他模块）；排除 store 与注释行后终扫 → 0 命中（exit=1）。monitoring 零 SQL（ADR-014） |
| R2-5 | G-05 无 panic 滥用 | PASS | `grep -rn "panic(" --include="*.go" . \| grep -v _test` → 0 命中（exit=1）；采集侧仅 recover() 兜底（collector.go:45-49、sampler.go:260-274 safeRead） |
| R2-6 | G-06 goroutine 退出路径 | PASS | `grep -rn "go func" --include="*.go" . \| grep -v _test` → 5 处全在 `cmd/dougong/main.go:213/228/243/281/287`（较 R1 的 4 处新增 file chunk 清理 243 与 TLS/明文双分支监听 281/287——同期 file-manager 交付物）；均 main 统一编排、ctx（NotifyContext）取消退出、wg 配对 Done + `wg.Wait()`（main.go:309）；monitoring collector（main.go:228-238）→ `Collector.Run` ctx.Done 返回 nil（collector.go:33-41），测试双证 `TestCollectorRunExitsOnCancel`（monitoring_test.go:465） |
| R2-7 | G-07 编译+vet+test 全绿 | PASS（引用） | ask 声明脚本已执行 build=0/vet=0/test=0，本轮未重跑 |
| R2-8 | 🟡 补充扫描 | PASS | `grep -rn "fmt.Print" … \| grep -v _test \| grep -v fmt.Errorf` → 0 命中（exit=1）；`grep -rn "log\.Print" … \| grep -v slog \| grep -v _test` → 0 命中（exit=1）；日志全 slog 带 ctx（sampler.go:263/269） |

## R2.2 前端栈规则扫描（frontend-rules §6；工作目录 frontend/）

| # | 检查 | 结果 | 证据 |
|---|------|------|------|
| R2-9 | F-01 无 Options API | PASS | `grep -rLn "script setup" --include="*.vue" src/` → 0 命中（exit=1） |
| R2-10 | F-02 无裸 any | PASS | `grep -rnE "(:\s*any\|as any)" --include="*.ts" --include="*.vue" src/` → 0 命中（exit=1）；monitoring 范围复核 `grep -rn "any" src/views/monitoring src/{types,api,stores}/monitoring.ts` → 0 命中 |
| R2-11 | F-03 请求只走 api 层 | PASS | `grep -rnE "(axios\|fetch)\(" --include="*.vue" src/views` → 0 命中（exit=1；src/components 目录不存在，首跑含该路径 exit=2，重跑收窄 src/views） |
| R2-12 | F-04 无混入其他组件库 | PASS | `grep -rnE "from '(antd\|element\|ant-design-vue)'" src/` → 0 命中（exit=1） |
| R2-13 | F-05 无原生弹窗 | PASS | `grep -rnE "(^\|\W)(alert\|confirm)\(" --include="*.vue" --include="*.ts" src/` → 0 命中（exit=1） |
| R2-14 | F-06 typecheck+build | PASS（引用） | ask 声明脚本已执行 web=0，本轮未重跑 |
| R2-15 | §4.5 轮询纪律 | PASS | 间隔常量集中（stores/monitoring.ts:30-33：3s/60s/滞后 6s）；document.hidden 暂停 + 恢复即拉（:104-111）；stopRealtimePolling（清定时器 + 解绑 visibilitychange，:149-155）由 views/monitoring/index.vue:19-21 onUnmounted 配对调用；TrendChart.vue:110-119 resize 监听 + dispose 配对清理；`grep -rn "setInterval\|setTimeout" --include="*.vue" src/` 命中仅 account-auth/login:27,66 与 service-mgmt:60,68（他模块范围，非本轮裁定对象；monitoring 范围零命中） |
| R2-16 | §4.1 硬编码色值 | PASS | `grep -rnE "#[0-9a-fA-F]{6}" --include="*.vue" src/` → 0 命中（exit=1）；SnapshotCard/TrendChart 色值全走 useThemeVars（SnapshotCard.vue:16/129-139、TrendChart.vue:39/91-94） |

## R2.3 前后端契约对齐（基准 api-monitoring.md v1.0）

| # | 检查 | 结果 | 证据 |
|---|------|------|------|
| R2-17 | AL-001 路径/方法 | PASS | routes.go:26 Prefix="/monitoring" + :35-38 四 GET（/metrics/snapshot、/metrics/history、/processes/top、/system/info）与 api §4.1.4/§4.2.4/§5.1 逐字一致；全 GET 只读无写接口 ✓ §3.1 |
| R2-18 | AL-002 前端 Action 路径 | PASS | api/monitoring.ts:16/21/36/41 四函数同路径 GET（request 默认 GET）；scope 仅 dimension==='cpu' 时发送（:26）= D-04 MON_003 预防；router/index.ts:85-96 两页面 requiresAuth 继承、无 roles 限制（=admin+readonly）✓ §3.3/ADR-018 |
| R2-19 | AL-003 DTO↔前端类型 | PASS | dto.go:11-100 ↔ types/monitoring.ts:30-119 字段逐字对齐（ts/degraded/cpu{totalPercent,perCore}/memory{usedBytes,totalBytes,usedPercent}/disk{capacity[{mount,totalBytes,usedBytes}],ioReadBps,ioWriteBps}/net{upBps,downBps}/load{l1,l5,l15}/processes；history{window,dimension,scope,startTime,partial,series[{key,label,unit,points[{ts,v}]}]}；top{ts,processes}；systeminfo{distro,kernel,hostname,uptimeSeconds,errors[{field,reason}]}）；**E1 修复点复核**：ProcessItem.cpuPercent/memBytes=number\|null（types/monitoring.ts:33-34）↔ backend *float64/*uint64（types.go:17-18），ProcessesCard.vue:26/33 走 formatPercent/formatBytes（format.ts:78-81/64-68 null→'—'）无裸 toFixed；**E2 修复点复核**：SnapshotCard.vue:40 `usagePercent(row.usedBytes, row.totalBytes)` + :111-118 v-if null 分支（format.ts:106-115）。前端组级 `\|null` 为后端 struct 恒对象的安全超集，非断裂 |
| R2-20 | AL-004 枚举一致 | PASS | metric_window{last_1h,last_24h}/metric_dimension{cpu,memory,disk,network,load}/metric_scope{total,per_core} 三方同值（consts.go:23-25/45-50/66-68 ↔ enums.ts:108-129 ↔ api §3.4） |
| R2-21 | AL-005 错误码/包络 | PASS | MON_001~004 符号串上线（errcode/monitoring.go:8-14、response.go:33-34 Envelope.Code=any：成功数值 0/失败 string）↔ 前端 `env.code===0` 取 data、否则 normalizeCode 串透传（errcode.ts:17 `typeof code === 'string'` 原样返回）+ MON_ERROR_TEXT（types/monitoring.ts:16-20）+ MON_004 空态轮询不停（stores/monitoring.ts:80-85）；服务端三参数枚举校验 MON_001~003 映射（service.go:38-50）= R-14 |
| R2-22 | 语义抽查 | PASS | D-03 series 统一形状 + v=null 缺口 + startTime/partial（series.go:16-66，槽位恒出点 A-M4）；D-04 省略 scope 归一 total 且非 cpu 携带即 MON_003（service.go:44-50）；D-07 空数据 series=[]+startTime=null+code=0（service.go:52-63，Series make 保证 [] 非 null）；D-08 degraded=\|\|!ProcessesOK 自持有（service.go:97/122-124）；I-02 环形淘汰（buffer.go:45-61 minuteIndex 覆盖最旧槽）；I-06 TOP ≤10 降序 nil 排尾（sampler.go:227-245）；中间件链 InitGuard→AuthRequired→RoleRequired(admin,readonly)（routes.go:30-38）= §3.1/§3.3；GET 无 CSRF ✓ §3.1 |

## R2.4 业务规则适用项（R-01~R-18）

| # | 规则 | 结果 | 证据 |
|---|------|------|------|
| R2-23 | R-02 Cookie 会话 | PASS | 四接口全在 AuthRequired（dg_session Cookie，routes.go:33-38）；前端 credentials same-origin、token 不入 localStorage（http.ts:124） |
| R2-24 | R-03 变更类审计 | PASS（不适用面） | 4 接口全只读 GET（§3.1/§1.2），无变更操作即无审计面 |
| R2-25 | R-08 参数数组执行 | PASS | 模块零 exec 调用（R2-1 双 grep）；采集全经 gopsutil 进程内读取（sampler.go:10-16） |
| R2-26 | R-09 零遥测零外呼 | PASS | 仅本机读取（sampler.go/systeminfo.go 全 gopsutil/os），无任何出站请求 |
| R2-27 | R-14 服务端校验 | PASS | History 三参数服务端枚举校验（service.go:38-50）；handler 薄壳零业务逻辑（handler.go:21-62） |
| R2-28 | 其余 R-01/04~07/10~13/15~18 | 不适用 | 模块边界 §1.2：无登录/文件路径/nginx/docker/监听配置/备份/终端交互面 |
| R2-29 | ADR-014 零持久化 | PASS | `ls backend/internal/store/migrations/` → 001_account_auth/002_panel_core/003_file_manager/004_site_mgmt，无 monitoring 表；service/monitoring 无 store 依赖（consts.go:1-4 包注释）= §5.2/§5.3 兑现 |

## R2.5 GR-03 E2E 测试

| # | 项 | 结果 | 证据 |
|---|----|------|------|
| R2-30 | 后端 E2E | PASS | module_test.go 8 测试全栈 httptest（真实 :memory: SQLite + 中间件链，newTestApp :48-83 镜像 main 装配）：会话拦截/双角色同权/MON_004 就绪性/参数校验/四接口形状逐字段断言（:241-558）；service 层 21 测试函数（缓冲不变量/聚合/投影/门面/采集冒烟/取消退出，monitoring_test.go:26-519）；只读模块无 save→delete 链，四读接口全覆盖 = GR-03 语义满足 |

## R2.6 WARN 项（本轮判定依据）

- **R2-W1**：views/monitoring 组件与 stores/monitoring.ts 0 测试——全仓测试文件仅 `stores/__tests__/{file,trash}.test.ts` + `utils/__tests__/{audit,errcode,format,totp}.test.ts`（find 实证）；frontend-rules §5「composables/工具函数必须有测试」精神，监控轮询/MON_004 分流/store 逻辑无组件级测试。
- **R2-W2**：src/components 目录不存在（`ls src/` 实证，F-03 首跑 exit=2 成因）；公共组件未沉淀，与 account-auth/panel-core/file-manager Gate 2 同判记录。

## R2.7 与前轮差异记录

- G-06 命中由 R1 的 4 处（main.go:168/183/221/227）变为 5 处（213/228/243/281/287）：同期 file-manager 交付新增 chunk 清理 goroutine 与 TLS/明文双分支监听，逐一核对均有退出路径，无违例。
- G-04 因 Windows 反斜杠路径致 `grep -v internal/store` 失效，本轮以 `tr '\\' '/'` 归一重跑，结论与 R1 一致（非 store 命中均为注释）。

## 第 2 轮判定

**判定：WARN**

- 栈规则 CRITICAL：0 违例（后端 G-01~G-06 实跑全过、前端 F-01~F-05 实跑 0 命中；G-07/F-06 引用脚本门控）
- 契约断裂：0（AL-001~005 全对齐；首轮 E1/E2 修复点本轮独立复核闭环）
- 业务规则适用项 5/5 过 + ADR-014 零持久化实证；SEC-GATE 探针未被拦且模块无桩 → DEBT-SEC-GATE 不适用
- WARN 2 项记录在案（R2-W1 组件/store 0 测试、R2-W2 src/components 缺席），用户确认后交付
