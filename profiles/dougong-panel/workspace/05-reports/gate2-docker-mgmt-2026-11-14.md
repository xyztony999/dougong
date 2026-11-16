# Gate 2 验证报告 — 代码→交付门控（docker-mgmt）

**模块**：容器管理（docker-mgmt）
**检查时间**：2026-11-15（报告名按派发指令定为 2026-11-14）
**检查员**：交付检查员·docker-mgmt（只查不改）
**扫描路径**：backend/internal/{api/dockermgmt, service/dockermgmt, adapter/dockerx, errcode, api/middleware, cmd/dougong} + frontend/src/{types,api,stores,views/docker,router,layouts}
**契约基准**：profiles/dougong-panel/workspace/02-detailed/api/api-docker-mgmt.md（v1.0，2026-11-14）
**规则链**：引擎 GR-01~GR-06 ⊕ backend-rules §7 G-01~G-07 / frontend-rules §6 F-01~F-06 ⊕ business-concepts R-01~R-18 适用项
**编译门控（脚本已执行，本报告引用未重跑）**：`go build ./...`=0、`go vet ./...`=0、`go test ./...`=0、前端 `npm run build`（vue-tsc + vite）=0（即 G-07 / F-06）

---

## 0. 裁定与范围

- **SEC-GATE 债务口径（2026-11-14 裁定）执行结果**：本轮实跑门禁探针——写入 `backend/.secgate-probe.go`（package 声明 + `exec.CommandContext` 字面量参数数组一行）成功（`write_exit=0`，文件落盘后即删）→ **Mimosa 已禁用、门禁不拦**。且本模块外部命令为**真实实现**（adapter/dockerx/compose.go:152 `exec.CommandContext(cctx, bin, composeArgs(fact, verb)...)`，非合规桩）→ **无 DEBT-SEC-GATE 债务，不适用桩从严条款**。
- 零持久化（D-10）核实：本模块无迁移文件（`internal/store/migrations/` 实存 001~005 五文件，无 docker 条目）、无 store 包、全模块无 SQL——与 api §5.2/§5.3「无」一致。

---

## 1. 后端规则扫描（backend-rules §7 G-01~G-07，实跑）

| # | 检查项 | 实跑命令（cwd=backend/） | 输出/证据 | 结果 |
|---|--------|--------------------------|-----------|------|
| G-01 | 无 shell 拼接 | `grep -rnE 'exec\.Command(Weak)?Context?\(.+\"(-c\|/c)\"' internal/ cmd/ --include="*.go"` | **0 命中**（exit=1）。全仓 `exec.Command` 代码命中 14 行人工核对全部 `exec.CommandContext` 参数数组形态；本模块唯一调用点 `internal/adapter/dockerx/compose.go:152`——bin=LookPath 解析常量名 `"docker"`（compose.go:50）、args 由纯函数 `composeArgs`（compose.go:175-185）组装：固定 flag `--project-name/--project-directory/-f` + label 值各自独立 argv 元素，无 `"-c"/"/c"` 无 shell 语义 | **PASS** |
| G-02 | 无 SQL 拼接 | `grep -rnE 'Sprintf\(.+(SELECT\|INSERT\|UPDATE\|DELETE)' internal/ cmd/ --include="*.go"` | **0 命中**（exit=1；本模块零 SQL，D-10） | **PASS** |
| G-03 | 无硬编码凭据 | `grep -rnEi '(password\|secret\|token\|apikey)\s*=\s*\"[^\"]{8,}\"' internal/ cmd/ --include="*.go" \| grep -v _test` | **0 命中**（exit=1） | **PASS** |
| G-04 | SQL 只在 store 层 | `grep -rlE "(SELECT\|INSERT\|UPDATE\|DELETE) " internal/ cmd/ --include="*.go" \| grep -v internal/store \| grep -v _test` | 层外命中 8 文件**全部为前序模块**（dbmysql 引擎侧 A-DB10 裁定 + auth/fileman/dbmgmt/sitemgmt 注释文本，历轮已逐个裁定）；**本模块三包（adapter/dockerx、service/dockermgmt、api/dockermgmt）零命中** | **PASS** |
| G-05 | 无 panic 滥用 | `grep -rn 'panic(' internal/ cmd/ --include="*.go" \| grep -v _test` | **0 命中**（exit=1） | **PASS** |
| G-06 | goroutine 退出路径 | `grep -rn "go func" internal/ cmd/ --include="*.go" \| grep -v _test` → 9 处，逐个读码核对 | main.go 7 处（:332 审计 worker / :347 monitoring collector / :362 chunk sweeper / :378 cert renewal / :394 terminal lifecycle——均 `defer wg.Done()` + panic recover 隔离 + ctx 取消退出；:432/:438 TLS/plain serve——`ErrServerClosed` 过滤后退出）；**本模块新增 2 处**：`api/dockermgmt/handler_ws.go:86` WS 读泵（`ReadMessage` 阻塞至客户端断开错误返回，或 `pumpLogFrames` defer `conn.Close()` → 读错误退出）、`adapter/dockerx/logs.go:165` 容器跟随泵（读 daemon 流至 EOF/错误 → End 投递 → close(lines)；`LogStream.Cancel` 即 sctx 取消 → SDK body 读失败收束）——退出路径均明确 | **PASS** |
| G-07 | 编译+vet+test 全绿 | 脚本门控（本报告引用） | build=0、vet=0、test=0（tasks.md T07 记录 34 包 `go test -race -count=1` 全 ok 0 FAIL，本模块 3 测试包 44 PASS+1 SKIP） | **PASS**（引用） |
| 🟡 W1/W2 | fmt.Print / log.Print | `grep -rn "fmt.Print" … \| grep -v _test \| grep -v fmt.Errorf`；`grep -rn "log\.Print" … \| grep -v slog \| grep -v _test` | 双双 **0 命中** | PASS |

## 2. 前端规则扫描（frontend-rules §6 F-01~F-06，实跑）

| # | 检查项 | 实跑命令（cwd=frontend/） | 输出 | 结果 |
|---|--------|--------------------------|------|------|
| F-01 | 无 Options API | `grep -rLn "script setup" --include="*.vue" src/` | **0 命中**（exit=1） | **PASS** |
| F-02 | 无裸 any | `grep -rnE "(:\s*any\|as any)" --include="*.ts" --include="*.vue" src/` | **0 命中**（exit=1） | **PASS** |
| F-03 | 请求只走 api 层 | `grep -rnE "(axios\|fetch)\(" --include="*.vue" src/views src/components` | **0 命中**（exit=1） | **PASS** |
| F-04 | 无混入其他组件库 | `grep -rnE "from '(antd\|element\|ant-design-vue)'" src/` | **0 命中**（exit=1） | **PASS** |
| F-05 | 无原生弹窗 | `grep -rnE "(^\|\W)(alert\|confirm)\(" --include="*.vue" --include="*.ts" src/` | **0 命中**（exit=1） | **PASS** |
| F-06 | typecheck + build 全绿 | 脚本门控（本报告引用） | web=0（vue-tsc + vite build） | **PASS**（引用） |
| 附检 | 定时器清理（§4.5） | `grep -rn "setInterval\|setTimeout" --include="*.vue" src/views/docker src/components` | docker 视图唯一命中 `index.vue:110/114`（composeProject 筛选 300ms 去抖），`onUnmounted`（index.vue:773-778）配对 `clearTimeout`；store 无定时器（无轮询，D-07 单次快照） | PASS |
| 附检 | 禁止硬编码后端地址 | `grep -rn "localhost:\|127.0.0.1\|http://" src/api/docker.ts src/types/docker.ts` | **0 命中**；WS 地址同源拼接（api/docker.ts:184-187 `window.location`），开发态经 vite proxy（ws:true） | PASS |

## 3. 前后端接口对齐（AL-001~AL-005，以 api-docker-mgmt.md 为基准抽查）

| # | 检查项 | 证据 | 结果 |
|---|--------|------|------|
| AL-001 | Controller 路径 vs API 文档 | routes.go:48-92 注册 24 端点与 §3.3/§4/§5.1 逐一比对全同：9 GET（daemon/status 无 DaemonGuard=探测面例外 D-01；container/{list,logs,stats}+image/volume/network/compose list/logs 挂 DaemonGuard）+ 13 POST（admin+CsrfGuard+DaemonGuard）+ 2 WS（GET 升级，dockerWsGuard） | **PASS** |
| AL-002 | 前端 Action 路径 vs Controller 路径 | api/docker.ts 24 个函数路径与 routes.go 逐字全同（`/docker/daemon/status`、`/docker/container/{list,logs,stats,start,stop,restart,pause,unpause,delete,logs-follow}`、`/docker/image/{list,pull,delete}`、`/docker/volume/{list,delete}`、`/docker/network/{list,delete}`、`/docker/compose/{list,up,down,restart,logs,logs-follow}`）；读 GET/写 POST 方法一致 | **PASS** |
| AL-003 | DTO 字段 vs 前端 VO 字段 | service/dockermgmt/dto.go ↔ frontend/src/types/docker.ts 逐形状比对：DaemonStatusData{state,version\|null,detail\|null,generatedAt}、ContainerItem{containerId,name,image,state,composeProject\|null,supportedActions}、ContainerListData{items,total,daemon{state,version},generatedAt}、LogsData{lines,generatedAt}、StatsData{cpuPercent,memUsageBytes,memLimitBytes,memPercent,generatedAt}、动作/删除/pull/compose 载荷、DK_004 data{currentState,action,allowedActions}、DK_015 data{workingDir,configFile,missing}、WS 帧 `{type:"log",line}`/`{type:"end",reason}`——全部一致；请求体 {containerId}/{ref}/{imageId}/{volumeName}/{networkId}/{project} 与 handler.go:34-53 请求结构体一致，delete 均无 force 字段（I-04/I-05） | **PASS** |
| AL-004 | 枚举值前后端一致 | container_status running/stopped/paused（dockerx.go:63-66 ↔ enums.ts:360）、过渡态 restarting/removing 透传（logs.go MapState ↔ types/docker.ts 注释 + index.vue:443-451 原值 Tag）、docker_daemon_state available/unavailable、compose_action up/down/restart/logs、WS end reason 五值（logs.go:24-29 ↔ enums.ts:402-407 + parseDockerLogFrame 收窄）、detail 二值 not_installed/socket_unreachable（dockerx.go:51-53 ↔ daemonDetailText） | **PASS** |
| AL-005 | 错误码前端可识别 | errcode/dockermgmt.go DK_001~018 共 18 个符号串 ↔ types/docker.ts DK_ERROR_TEXT 18 键（docker.test.ts 断言 18/18 全覆盖）；DK_002/DK_004/DK_015/data.reason 分流路径在 store/视图逐码落地（stores/docker.ts:195-197、index.vue:159-248）；AUTH_001/002/003/008/012 复用经 http.ts 拦截器 + 中间件（module_test.go 六用例覆盖） | **PASS** |

## 4. 契约语义抽查（api 文档关键裁定落地）

| # | 裁定 | 证据 | 结果 |
|---|------|------|------|
| C-1 | D-02 权威动作面 + BA 附录B.2 矩阵 | matrix.go:11-33：start←stopped；stop/restart←running\|paused；pause←running；unpause←paused；delete←stopped；过渡态空数组——与 D-04 逐条一致；list 响应与 DK_004 data 同源 `SupportedActions`（service.go:94/191） | PASS |
| C-2 | D-01/D-02 不返回空清单（I-01） | routes.go:105-116 DaemonGuard：unavailable → `ErrDkDaemonUnavailable`（DK_002）Abort；status 豁免（探测面）；module_test.go TestDaemonUnavailableGate 覆盖 5 清单+写全 DK_002 | PASS |
| C-3 | D-03 日志双通道 + WS 帧协议 | REST 尾部（GetContainerLogs tail 域 1-2000 缺省 500，validate.go:30-39）；WS 握手链（Origin→Cookie VerifyWs→角色，routes.go:132-163；DK_002→503/其余 DK_→400 wsStatus A-D13，handler_ws.go:153-169）；帧泵 log/end + 行通道优先排空防尾丢（handler_ws.go:81-137）；客户端无上行帧、断开即 Cancel | PASS |
| C-4 | D-06 Compose 定位/互斥/CLI | compose.go：label 三键（:22-26，R-13）、locatable=目录存在且每个配置文件可读（:204-220，A-D9 逗号多文件逐 -f :175-185）、per-project tryAcquire 互斥（:223-238，I-08→ErrBusy→DK_018 service.go:664-665）、300s 上限（:43）、up 补 -d（A-D8，:181-183）；服务层链 DK_001→014→015→018→016（service.go:446-476）与 §4.3.4 校验顺序一致 | PASS |
| C-5 | D-08 daemon 拒绝归一 | dockerx.go:479-490 sdkError：NotFound→ErrNotFound、Conflict→ErrInUse、Forbidden→ErrPredefinedNetwork；服务层 mapImageDeleteError/mapVolumeDeleteError/mapNetworkDeleteError/mapComposeRunError（service.go:621-673）分归 DK_006/008/010/011/005/007/009/014；ExecError 原文 reason 透传不虚构（DK_013/016/017 data.reason） | PASS |
| C-6 | D-09 审计非同事务 + I-06 口径 | service.go:600-609 recordAudit：失败重试一次→slog 兜底不影响结果；13 审计动作码（consts.go:9-23）；DK_001/004 零审计、DK_002 中间件拒绝零审计（A-D15，I-06 字面口径，与 api §4.1.4「DK_004/017 为 failure」即 004 除外记注一致——004 明文零审计见 §4.1.4 动作行「DK_004 拒绝未发生变更不记」语义，I-06 行明文 DK_001/002/004）；service_test.go TestAuditFallback/TestAuditActionCodes 断言 | PASS |
| C-7 | A-D14/超时常量 | SDK 60s/pull 600s/Compose 300s/Probe 5s 编译期常量（dockerx.go:40-47）；container/list daemon 摘要取 DaemonGuard 当次探测（routes.go:26-28/119-126 + handler.go:69-77） | PASS |
| C-8 | 零持久化 | 无迁移（migrations 实存 001~005 无 docker 条目）、无 store 包、模块零 SQL；清单恒即时读取（ListContainers All:true 含停止态，R-15 接管哲学） | PASS |

## 5. 业务规则适用项（business-concepts R-01~R-18）

| 规则 | 落地证据 | 结果 |
|------|----------|------|
| R-02 会话/CSRF | AuthRequired 全组前置；写组 CsrfGuard（X-CSRF-Token+同源，csrf.go:14-40）；WS Cookie+Origin（routes.go:132-163 复用 SameOrigin 导出 csrf.go:61） | PASS |
| R-03 审计 | 13 写动作经 AuditService.Record（category=change，四要素，service.go:572-583）；D-09 降级 | PASS |
| R-08 参数数组 | compose.go:152 exec.CommandContext 参数数组（G-01 实证）；SDK 通道无 shell | PASS |
| R-09 零遥测 | 模块无外呼（SDK→本机 sock、CLI→本机 docker；pull 外呼在 daemon 侧用户主动，I-07） | PASS |
| R-12 只接管不重装 | ProbeDaemon D-01 判定 + 未装引导（前端 unavailable → NEmpty 引导态，不渲染空清单）；无应用商店动作面（前端容器 Tab 无新增按钮） | PASS |
| R-13 Compose label 识别 + CLI 参数数组 | label 三键聚合（compose.go:98-132 无 label 容器不归属）+ composeArgs 参数数组 | PASS |
| R-14 服务端校验 | 单一校验通道在 service（validate.go requireRef/ParseTail/ParseStateFilter/ValidateImageRef；handler 无 required 绑定，bindError→AUTH_012）；矩阵/存在性/格式全套服务端 | PASS |
| R-15 存量接管 | ListContainers All:true；Compose 经 label 自动识别；面板外变更如实（DK_003/005/007/009 不静默成功） | PASS |
| R-01/04/05/06/07/10/11/16/17/18 | 非本模块职责（account-auth/file-manager/site-mgmt/panel-core 域），无涉 | N/A |

## 6. 测试覆盖引用

- 脚本门控 test=0（引用）；tasks.md T06 记录：adapter/dockerx 16 用例（TestMapState/TestComposeArgs/TestLocateMissing/TestTryAcquireBusy/TestFollowLogsProjectGone 等——grep 实证 17 个 Test 函数）、service/dockermgmt 19 用例（矩阵全表/校验链/动作链/删除九分支/Compose 链/审计降级）、api/dockermgmt 6 用例（未登录/daemon 门/双角色读面/写面/WS 容器跟随/WS compose 拒绝）——grep 实证 42 个 Test 函数在位（module_test.go:405-637）。
- 前端：utils/__tests__/docker.test.ts（帧收窄/错误 data 收窄/文案映射/DK 18 码断言）在位；vitest 全绿随 web=0 门控（引用 INDEX frontend note 87/87）。

## 7. 发现项

| # | 级别 | 内容 | 证据 |
|---|------|------|------|
| W1 | ⚠️ WARN | **菜单图标键前后端不一致**：后端 main.go:301 登记 `Icon: "container"`（tasks.md A-D19「icon 键 container 由前端 MENU_ICONS 映射」），前端 `MENU_ICONS`（layouts/MainLayout.vue:79）映射的键是 `docker: CubeOutline`，**无 `container` 键** → `resolveIcon("container")` 走 AppsOutline 兜底（MainLayout.vue:83），菜单图标呈通用图标而非立方体。功能无损（兜底在位、PageSpec 未钉图标键、api 契约不含此项），但 A-D19 前后端约定未闭环。建议：MENU_ICONS 增 `container: CubeOutline`（或后端改发 `docker`）二选一对齐 | main.go:301；MainLayout.vue:77-80 |
| I1 | INFO | network 投影 `subnet` 可空口径：后端 `NetworkItem.Subnet string`（无 IPAM 子网时序列化为 `""`，dockerx.go:500-508 firstSubnet 返回空串），前端声明 `string \| null` 且仅 null 渲「—」（index.vue:711）——空串呈空白单元格而非「—」。api §4.2.1 未钉 subnet 可空性，前端类型为超集宽容，无崩溃路径，非契约断裂（monitoring 门同判先例）。建议：后端空子网归一 null 或前端 `!row.subnet` 判空 | dto.go:127-132；types/docker.ts:158-165 |
| I2 | INFO | tasks.md docker 节 T01 行「migrations 维持 001~006」计数笔误：`internal/store/migrations/` 实存 001~005 五文件（无 006）。实质结论正确（本模块零迁移），仅文档计数失准 | ls migrations/；tasks.md:772 |

## 8. 总结

**判定：WARN**

- 后端 G-01~G-07：7/7 通过（G-07 引用脚本门控；G-01~G-06 本轮实跑 grep + 逐点读码核对）
- 前端 F-01~F-06：6/6 通过（F-06 引用脚本门控；F-01~F-05 本轮实跑 grep）
- 契约对齐 AL-001~AL-005：5/5 通过（24 端点路径/方法/字段/枚举/错误码逐项比对，以 api-docker-mgmt.md 为基准）
- 契约语义抽查 C-1~C-8：8/8 通过
- 业务规则适用项：R-02/03/08/09/12/13/14/15 全过，其余 N/A
- SEC-GATE：探针通过（Mimosa 已禁用）+ 真实实现 → 无债务
- **CRITICAL 违规：0；契约断裂：0；⚠️ WARN：1（W1 菜单图标键不一致，建议项）；INFO：2（I1 subnet 空串呈现、I2 tasks.md 迁移计数笔误）**

WARN 项不阻断交付（图标兜底在位无功能影响），建议随下轮迭代对齐。

---

## 9. WARN 修复轮（2026-11-16）

**处置范围**：§7 发现项表逐项（W1 + I1 + I2）。修复方向均按该表「建议」列与 tasks.md 既有裁定（A-D19 / 完成汇总遗留行）。

| # | 级别 | 处置 | 证据 / 理由 |
|---|------|------|-------------|
| W1 | ⚠️ WARN | **已修复**（建议二选一取「前端 MENU_ICONS 增 container 键」——tasks.md A-D19「icon 键 container 由前端 MENU_ICONS 映射」+ docker 完成汇总遗留行「菜单 icon 键 container 由前端模块在 MENU_ICONS 补映射」既定方向，后端 main.go:305 `Icon: "container"` 不动） | frontend/src/layouts/MainLayout.vue:79-84（原 :81 docker 键下新增 `container: CubeOutline`，注释同步「后端已登记 icon 键 container（A-D19），此处补映射闭环」；`docker` 预置键保留兼容）——`resolveIcon("container")` 不再走 AppsOutline 兜底 |
| I1 | INFO | **已修复**（建议二选一取「前端 `!row.subnet` 判空」，后端 DTO 形状不动；并按建议列所述「api §4.2.1 未钉 subnet 可空性」补钉契约） | frontend/src/views/docker/index.vue:711（`row.subnet === null` → `!row.subnet`，空串/null 均渲染「—」）；frontend/src/types/docker.ts:163-164（注释钉死「后端发空串，null 为超集容忍；判空渲染」）；api-docker-mgmt.md §4.2.1 network 行（subnet 恒 string、无 IPAM 子网时为空串 "" 非 null、前端判空渲染「—」） |
| I2 | INFO | **维持-理由**：gate2 时点（2026-11-15）migrations 实存 001~005、文面「维持 001~006」失准属实；但本轮实查 `ls backend/internal/store/migrations/` 现存 001~006 六文件——后续 cron-backup 模块落地已带来 `006_cron_backup.sql`（INDEX.md cron-backup 节，2026-11-15 落盘），tasks.md docker 节文面「维持 001~006」（backend/tasks.md:817/:859）与当前实况**已一致**，时点性笔误自然消解；若回改为 001~005 反而制造新失准，故不修 | `ls migrations/` 六文件实查；backend/tasks.md:817/:859 |

**自验（本轮实跑）**：前端 `npm run build`（cwd=frontend/，vue-tsc + vite）= exit 0（built in 27.97s，chunk 体积提示为既有 advisory 非错误）。后端零代码改动（W1/I1 均前端+文档侧处置，I2 维持），go build/vet/test 不涉、未跑。

**改动文件清单**：frontend/src/layouts/MainLayout.vue、frontend/src/views/docker/index.vue、frontend/src/types/docker.ts、02-detailed/api/api-docker-mgmt.md（§4.2.1 单行钉死）、05-reports/gate2-docker-mgmt-2026-11-14.md（本节）、INDEX.md（gate2 note 追加修复轮摘要）。

> **复核行（2026-11-16，WARN 复核员）**：§9 三项逐一核验——W1 在位（frontend/src/layouts/MainLayout.vue:83 `container: CubeOutline`，:79-80 注释注明 A-D19 补映射闭环；后端 main.go:305 Icon=container 未动，grep 实证）；I1 三处在位（views/docker/index.vue:711 `!row.subnet` 判空渲染「—」、types/docker.ts:163-165 注释钉死空串口径、api-docker-mgmt.md:279 §4.2.1 network 行「subnet 恒 string，无 IPAM 子网时为空串 ""（非 null，gate2 I1 修复轮钉死）」）；I2 维持-理由成立（本轮 `ls backend/internal/store/migrations/` 实存 001~006 六文件，006_cron_backup.sql 已随后续模块落地，tasks.md 文面与实况一致）。三项全部闭环，本轮另复跑 npm run build exit 0 → **终局判定：pass**。汇总见 05-reports/gate2-warn-fix-review-2026-11-16.md。
