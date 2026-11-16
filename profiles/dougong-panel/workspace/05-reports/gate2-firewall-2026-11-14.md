# Gate 2 验证报告 — 防火墙（firewall）代码→交付门控

**模块**：firewall（M7 主机网络入口安全）
**扫描路径**：backend/internal/{api/firewall, service/firewall, adapter/firewallx, adapter/netlisten, errcode/firewall.go, api/middleware, cmd/dougong/main.go 装配段} + frontend/src/{types/firewall.ts, types/enums.ts(firewall 段), api/firewall.ts, stores/firewall.ts, views/firewall/**, router/index.ts 追加段, layouts/MainLayout.vue 追加段}
**扫描时间**：2026-11-15（检查员轮，报告名沿派发口径 gate2-firewall-2026-11-14.md）
**契约基准**：profiles/dougong-panel/workspace/02-detailed/api/api-firewall.md v1.0（2026-11-14）
**规则链**：引擎 GR-01~GR-06（角色指令）⊕ backend-rules.md / frontend-rules.md（🔴 CRITICAL 与 🟡 项）⊕ business-concepts/index.md（R-01~R-18 适用项）

> 环境说明：`.claude/rules/` 下仅有 backend-rules.md / frontend-rules.md，无 engine-rules.md——GR-01~06（扫描必执行/编译必过/E2E 必补/前后端契约对齐）按角色指令与 SKILL.md「分层说明」执行（先例：panel-core/monitoring 等 Gate 2 报告同口径 INFO）。

---

## 1. 编译与门禁（GR-02 编译必过 / G-07 / F-06）

| # | 检查 | 结果 | 证据 |
|---|------|------|------|
| 1.1 | `go build ./...` `go vet ./...` `go test ./...` | **引用脚本门控（ask 提供，未重跑）**：build=0 vet=0 test=0 | ask 明示「不必重跑」；tasks.md:725 记录同一门禁 31 包全 ok |
| 1.2 | 前端 `vue-tsc --noEmit + vite build`（F-06） | **引用脚本门控**：web=0 | ask 提供；INDEX.md:126 记录 npm run build exit 0 + vitest 77/77 |
| 1.3 | 模块包覆盖率补测（本人实跑） | `go test -count=1 -cover ./internal/service/firewall/ ./internal/api/firewall/ ./internal/adapter/firewallx/` → `ok service/firewall 0.671s coverage: 89.3%` / `ok api/firewall 1.644s coverage: 83.9%` / `ok adapter/firewallx 0.527s coverage: 42.9%` | 本轮命令输出 |

---

## 2. 后端栈规则扫描（backend-rules §7 G-01~G-07，全部本人实跑）

工作目录 `backend/`，grep 为 Git Bash（Windows）。

### B-1（G-01 无 shell 拼接）— PASS
- `grep -rnE 'exec\.Command(Weak)?Context?\(.+\"(-c|/c)\"' --include="*.go" .` → **0 命中**（exit=1）。
- 全仓 `grep -rn "exec\.Command" --include="*.go" .` 共 12 行命中：firewallx.go:185（本模块）+ dbmysql×2 / dbredis:204 / ptyx:44,50 / nginxx:303,327（前序模块，各模块 Gate 2 已裁定）+ 5 处注释行。全部为 `exec.CommandContext(...)` 参数数组形态，无 `exec.Command(` 裸调用。
- 本模块唯一调用点 `firewallx/firewallx.go:185`：`cmd := exec.CommandContext(cctx, name, args...)`——`name` 为包级常量 `ufwBin="ufw"`/`fwcmdBin="firewall-cmd"`（firewallx.go:38-41，经 LookPath 探测、不信任外部传入路径，firewallx.go:170-178）；`args` 由纯函数组装：`ufwAddArgs`（ufw.go:144-153，port/proto/action/source 均经 FW_001/FW_002 值域校验后作独立 argv 元素）、`ufwDeleteArgs`（ufw.go:185-187，ruleKey 词元化且经存在性实况复核 service.go:234-247）、firewalld `--add-port=/--add-rich-rule=/--remove-*=` 单 flag 元素（firewalld.go:164-168,192-197——rich rule 含空格仍是单 argv 元素，无 shell 语义）。30s 超时经 `context.WithTimeout`（firewallx.go:183-185，CommandTimeout=30s 编译期常量 :45）。

### B-2（G-02 无 SQL 拼接）— PASS
- `grep -rnE 'Sprintf\(.+(SELECT|INSERT|UPDATE|DELETE)' --include="*.go" .` → **0 命中**（exit=1）。本模块零 SQL（D-10 零持久化），无 Sprintf 拼 SQL 面。

### B-3（G-03 无硬编码凭据）— PASS
- `grep -rnEi '(password|secret|token)\s*=\s*"[^"]{8,}"' --include="*.go" . | grep -v _test` → **0 命中**（exit=1）。

### B-4（G-04 SQL 只在 store 层）— PASS（firewall 零新增命中）
- `grep -rlnE "(SELECT|INSERT|UPDATE|DELETE) " --include="*.go" . | grep -v internal/store | grep -v _test` 输出 20 文件，**firewall 相关四包（api/firewall、service/firewall、adapter/firewallx、adapter/netlisten）均不在清单**。层外命中（dbmysql 引擎侧 A-DB10、service 层注释文本）与前序模块 T07 裁定一致（tasks.md:722）。
- 佐证：本模块零持久化——`backend/internal/store/migrations/` 仅 001~005，无 firewall 迁移（D-10；tasks.md:733「无表无 DDL 无 SQL 无迁移」与实况一致）。
- 勘误说明：因 Windows 路径反斜杠，`grep -v internal/store` 未滤掉 `internal\store\*` 的 10 个 store 文件——它们本就属 store 层合法命中，不影响判定。

### B-5（G-05 无 panic 滥用）— PASS
- `grep -rn "panic(" --include="*.go" . | grep -v _test` → **0 命中**（exit=1）。全仓非测试代码零 panic（含本模块）。

### B-6（G-06 goroutine 有退出路径）— PASS
- `grep -rn "go func" --include="*.go" . | grep -v _test` → 7 处，全部在 `cmd/dougong/main.go:311,326,341,357,373,411,417`（main 统一编排：审计 worker/monitoring collector/chunk sweeper/cert renewal/terminal lifecycle/TLS·plain serve，前序模块逐个核对有 ctx 取消与 wg 编排，tasks.md:724）。**本模块新增 0 处 goroutine**（firewallx 惰性探测 D-01 按需即时，无后台任务）。

### B-7（G-07 编译+vet+test 全绿）— PASS（引用）
- 见 §1.1：脚本门控 build=0 vet=0 test=0；本人补测三包 `-cover` 全 ok（§1.3）。

### 后端 🔴/🟡 补充项

| # | 项（规则出处） | 结果 | 证据 |
|---|------|------|------|
| B-8 | 错误包装 `%w`（§1.1 🔴） | PASS | 模块内原始错误全部 `%w` 包装：firewallx.go:125,141,157；ufw.go:37,165,198；firewalld.go:45,56,175,203；netlisten/source.go:41；service.go:90,117,285,307（"firewall panel listen port: %w" 等） |
| B-9 | 裸 `return nil, err` 透传（§1.1 🔴） | PASS（人工核对） | `grep -rn "return nil, err$" <firewall 四包>` 4 处：service.go:71,75（parseEnumFilter 返回值，validate.go:57-70 返回的已是 errcode FW_001 契约值）、:157,218（fail() 闭包，入参全部为 errcode 化错误）——errcode.Error 自带 symbol/message/data 上下文且为 response.Fail 设计通道，非无上下文原始错误透传，符合规则意图 |
| B-10 | 无 `fmt.Print` 调试输出（§1.1 🟡） | PASS | `grep -rn "fmt.Print" --include="*.go" . | grep -v _test | grep -v fmt.Errorf` → 0 命中 |
| B-11 | 日志 slog 结构化、无 `log.Print`（§5 🔴/🟡） | PASS | `grep -rn "log\.Print" --include="*.go" . | grep -v slog | grep -v _test` → 0 命中；审计降级用 `slog.WarnContext`（service.go:418） |
| B-12 | ctx 首参（§1.1 🟡） | PASS | Gateway 四方法与 ListenPortSource.List 均 ctx 首参（ports.go:21-37）；staticPanelPort.GetListenPort 无 ctx 为纯内存读，豁免有注释（ports.go:47-52） |
| B-13 | handler 薄壳（§4 🔴） | PASS | handler.go 五接口=绑定→service→response.OK/Fail；业务逻辑零驻留（值域校验唯一权威在 service，R-14 单一校验通道注释 handler.go:19-21） |

---

## 3. 前端栈规则扫描（frontend-rules §6 F-01~F-06，全部本人实跑）

工作目录 `frontend/`。

| # | 检查 | 命令 | 结果 |
|---|------|------|------|
| F-1 | 无 Options API | `grep -rLn "script setup" --include="*.vue" src/` | **0 命中**（exit=1） |
| F-2 | 无裸 any | `grep -rnE "(:\s*any\|as any)" --include="*.ts" --include="*.vue" src/` | **0 命中**（exit=1）；模块内 unknown+收窄先例：index.vue:124-128 isDangerInfo、:175-181 execReason |
| F-3 | 请求只走 api 层 | `grep -rnE "(axios\|fetch)\(" --include="*.vue" src/views src/components` | **0 命中**（exit=1）；模块请求全部经 api/firewall.ts → http.ts request()（唯一 fetch 出口） |
| F-4 | 无混入其他组件库 | `grep -rnE "from '(antd\|element\|ant-design-vue)'" src/` | **0 命中**（exit=1） |
| F-5 | 无原生弹窗 | `grep -rnE "(^|\W)(alert\|confirm)\(" --include="*.vue" src/` | **0 命中**（exit=1）；危险确认用 n-modal + NPopconfirm |
| F-6 | typecheck+build 全绿 | `vue-tsc --noEmit && vite build` | **引用脚本门控**：web=0（§1.2） |

### 前端 🔴/🟡 补充项

| # | 项（规则出处） | 结果 | 证据 |
|---|------|------|------|
| F-7 | 状态/枚举列 n-tag+映射表（§4.2 🔴） | PASS | dictTag（index.vue:308-311）绑定 FIREWALL_RULE_ACTION/LISTEN_PORT_STATE 映射表（enums.ts:282-309），文案在 types 映射表而非 render 硬编码 |
| F-8 | 表单 n-form rules+validate（§4.3 🔴） | PASS | AddRuleModal.vue:120-139 FormRules、:152 `formRef.validate()`；预校验同契约：PORT_PATTERN :81 = api §3.4 端口形态正则，CIDR 预校验 :109-118 |
| F-9 | 危险操作确认（§4.2 🟡） | PASS | 删除=NPopconfirm+error 型按钮（index.vue:370-381）；FW_005 服务端权威二次确认 Modal :560-596，确认后原请求追加 confirmDanger=true 重提交（:151-162，不记忆） |
| F-10 | 定时器清理（§4.5 🟡） | PASS（不适用面） | `grep -rnE "setInterval\|setTimeout" src/views/firewall src/stores/firewall.ts` → 0 命中；无轮询设计（stores/firewall.ts:9-10 注释） |
| F-11 | 无大面积硬编码 hex（§4.1 🟡） | PASS | `grep -rnE "#[0-9a-fA-F]{6}" --include="*.vue" src/views/firewall/` → 0 命中；风险行用 rgba 语义色（index.vue:674-676） |
| F-12 | 路由懒加载（§2 🟡） | PASS | router/index.ts:178 `() => import('@/views/firewall/index.vue')` |

---

## 4. 前后端契约对齐抽查（AL-001~005，基准 = api-firewall.md v1.0）

| # | 检查 | 结果 | 证据 |
|---|------|------|------|
| C-1（AL-001） | Controller 路径/方法 vs API 文档 | **全等** | routes.go:39-47：GET /firewall/{backend/status, rule/list, port/list} + POST /firewall/{rule/create, rule/delete} ↔ api §3.1/§4.1.4/§5.1 五接口表逐字一致（三段式路径、读 GET/写 POST） |
| C-2（AL-002） | 前端 Action 路径 vs Controller | **全等** | api/firewall.ts:26,31,38,43,51 五函数路径与 methods ↔ routes.go；筛选参数 query {action?, protocol?} 多值逗号（api §4.1.4 ↔ handler.go:50 c.Query ↔ stores/firewall.ts:127-129 join(',')） |
| C-3（AL-003） | DTO 字段 vs 前端 TS 类型 | **全对齐** | dto.go ↔ types/firewall.ts 逐形状：BackendStatusData{backend,active,version,active→*string↔string\|null,supportedActions,generatedAt}（dto.go:10-16 ↔ :13-20）；RuleItem 8 字段含 isPanelPort（dto.go:19-28 ↔ :23-38）；RuleListData{items,total,backend,generatedAt}（:38-43 ↔ :47-52）；PortItem{proto,addr,port,process,external,openState}+ProcessInfo 可空（dto.go:47-61 ↔ :55-72）；CreateData 4 键/DeleteData{deleted,ruleKey}（dto.go:71-83 ↔ :94-111）；create 请求 {port,protocol,source?,action,confirmDanger?}（handler.go:22-28 ↔ :82-91，source undefined→JSON 省略→Go *string nil→空=任意来源）；FW_005 data {panelPort,rulePort,action,hint}（dto.go:87-92 ↔ :114-119 + 收窄 index.vue:124-128） |
| C-4（AL-004） | 枚举值前后端一致 | **三方同值** | firewall_backend ufw/firewalld/none（firewallx.go:31-35 ↔ api §3.4 ↔ enums.ts:281）；firewall_rule_action allow/deny/reject/limit（consts.go:8-11 ↔ enums.ts:289）；firewall_rule_protocol tcp/udp（consts.go:14-15 ↔ enums.ts:298）；listen_port_state allowed_by_rule/no_allow_rule（consts.go:18-19 ↔ enums.ts:305）；direction 恒 "ingress"（consts.go:22 ↔ RuleItem.Direction，前端常量列 index.vue:333） |
| C-5（AL-005） | 错误码前端可识别 | **全对齐** | FW_001~010 后端 errcode/firewall.go:8-27（10 个，逐条带 §3.4 语义注释）↔ 前端 FW_ERROR_TEXT 十键全（types/firewall.ts:127-138）+ 页面分流 FW_003/005/006/007/008/009/010（index.vue:201-243,274-304）；AUTH_001/002→全局跳登录、AUTH_003/AUTH_008→toast、AUTH_012→fields 提示（http.ts:70-99）；包络 {code,message,data} code=0/符号串（response.go:16-45 ↔ http.ts:136-162 normalizeCode 直传），HTTP 一律 200（account-auth D-07 同一契约） |
| C-6 | 权限矩阵与中间件链 | **对齐** | api §3.1 链 AuthRequired→CsrfGuard(POST)→RoleRequired：routes.go:37-47 读组（AuthRequired+RequireRole(admin,readonly)）、写组（AuthRequired+CsrfGuard+RequireRole(admin)），GET 豁免 CSRF ↔ §3.3 矩阵（读=admin+readonly/写=admin+CSRF）；readonly 越界由 RoleRequired 全局拦截 AUTH_008，接口层不自建分支（routes.go:30-32 注释） |

设计决策落点核对（D-01~D-10 抽查）：D-01 权威动作面 SupportedActions（consts.go:54-63：ufw 四值/firewalld 仅 allow/none 空）+ FW_004 前置（service.go:182-184）；D-02 服务端筛选排序（service.go:319-357 端口数值升序、同端口 ruleKey 字典序）；D-03 ruleKey 原生定位串（ufw.go:111,134-140 / firewalld.go:72,103）；D-04 对照两态+external（service.go:362-376 + validate.go:110-147，tcp 族匹配 tcp/tcp6）；D-05 校验集（validate.go 全部服务端）；D-06 涉险判定基准=进程实际绑定端口（main.go:260-277 DOUGONG_ADDR 覆盖后 panelListenPort→StaticPanelListen，ports.go:39-55）；D-07 等价/缺失如实（ufw.go:161-163,194-196 / firewalld.go:171-173,199-201 → FW_006/FW_003）；D-08 双写两步不用 --reload（firewalld.go:161-214；runtime 失败 FW_007 不回滚 service.go:202-205,263-266）；D-09 审计重试一次降级日志（service.go:416-421）；D-10 零持久化（migrations 001~005 无新增，§B-4）。I-05 涉险两段式服务端强制（service.go:275-315，FW_005 拒绝路径不经 fail() → 零审计，I-07 明文豁免）；I-07 每调用恰一条审计（auditMutation service.go:398-422，成功/失败分支全覆盖）。

---

## 5. 业务规则适用项（business-concepts R-01~R-18）

| # | 规则 | 结果 | 证据 |
|---|------|------|------|
| R-a | R-02 Cookie 会话+CSRF | PASS | AuthRequired 全组（routes.go:37,44）；POST 全走 CsrfGuard（:45）；前端 POST 统一注入 X-CSRF-Token（http.ts:113-116），token 不入 localStorage |
| R-b | R-03 变更写审计 | PASS | create/delete 每调用恰一条（auditMutation service.go:398-422；动作码 firewall_rule_created/deleted consts.go:28-30 ↔ api §3.4 业务常量）；FW_005 拒绝零审计符合 I-07 字面口径 |
| R-c | R-08 命令参数数组禁 shell | PASS | §B-1 实证（唯一 exec 点参数数组+常量二进制名+值域校验后独立 argv 元素） |
| R-d | R-09 零遥测零外呼 | PASS | 模块出站仅本机 ufw/firewalld CLI 与内核套接字表（gopsutil 纯库，netlisten/source.go:39）；无 HTTP client、无外部网络请求（grep 模块内无 net/http 使用） |
| R-e | R-14 服务端校验不信任前端 | PASS | 值域校验全部在 service（validate.go:17-70 + service.go:160-172/221-224）；handler 不做 required 绑定（handler.go:19-21 单一校验通道声明）；前端预校验仅为 UX（AddRuleModal 注释 :8-9） |
| R-f | R-15 识别即接管不安装 | PASS | Detect 经 LookPath 即时探测 ufw 优先→firewalld→none（firewallx.go:107-178），不安装不切换（api §1.1）；Windows 开发机如实返回 none（tasks.md:727 冒烟） |
| R-g | ADR-018 readonly 全可看+写边界 | PASS | 后端读组两角色/写组 admin（routes.go:37-47）；前端 readonly 负空间：无添加按钮（index.vue:501-508 v-if isAdmin）、操作列整列不渲染（:358-385）、AddRuleModal 不渲染（:551-553）、端口对照页处置线索提示（:533） |
| R-h | 不适用项 | N/A | R-01（登录锁定）/R-04·05（文件）/R-06·07·16（nginx）/R-10（监听配置）/R-11（备份轮转）/R-12·13（容器）/R-17（回收站）/R-18（终端）——均归属 account-auth/file-man/site-mgmt/panel-core/cron-backup/docker-mgmt/terminal 模块，本模块无承载面 |

---

## 6. 测试覆盖率（SKILL Gate 2 §3）与 E2E（GR-03）

| 层级 | 覆盖率（本人实跑 §1.3） | 达标线 | 判定 |
|------|------|------|------|
| 后端 Service 层（service/firewall） | **89.3%** | 80%+ | YES |
| 后端 Controller 层（api/firewall） | **83.9%** | 60%+ | YES |
| adapter/firewallx | 42.9% | （表外） | INFO-I1：未覆盖面=run/detect 需真实 ufw/firewalld 二进制（Windows 不可达），解析与命令组装纯函数已表驱动全覆盖（firewallx_test.go 9 Test 函数，tasks.md:709/727 实机交互归 Linux 冒烟） |
| 前端组件 | **0 测试** | 60%+ | **NO → W1**（views/firewall 与 stores 无任何 *.spec/*.test 文件；现存 stores/__tests__ 仅 file.test.ts/trash.test.ts 属他模块） |

E2E（GR-03 E2E 必补）：api/firewall/module_test.go 7 个全栈 httptest 用例（真实 :memory: SQLite+真实审计服务、网关 fake）——未登录 AUTH_001×5、双角色三读接口全键断言、list 门控与筛选（FW_001/009/010/008）、readonly 越界 AUTH_008 零提交零审计、CSRF 三态拒绝、create/delete HTTP 全流程（FW_005 data→confirmDanger 成功→畸形 body AUTH_012→审计条数断言）（module_test.go:314-592 函数清单；tasks.md:711）。模块合计 31 个 Test 函数（firewallx 9 + service 15 + api 7，grep -c 实数）。

---

## 7. SEC-GATE 债务口径（2026-11-14 裁定）

- **探针**：本轮多命令实跑均成功执行未被拦（go test -cover §1.3 正常返回、grep 全量正常返回）= 门禁探针未被 Mimosa 拦截（与 INDEX.md:125「Mimosa 已禁用探针验证」记录一致）。
- **桩检查**：模块**无合规桩**——firewallx 为真实 exec.CommandContext 实现（firewallx.go:182-192），无任何「外部命令桩/echo 假实现」；tasks.md firewall 段无 SEC-GATE 阻塞记录（grep "SEC-GATE|合规桩" 0 命中）。
- **结论**：探针通过且无桩 → **DEBT-SEC-GATE 不适用**，本模块无该项债务。

---

## 8. WARN / INFO 汇总

**WARN（1 项）**
- **W1**：firewall 前端 0 组件/store 测试——`find src/views/firewall src/stores -name "*.spec.*" -o -name "*.test.*"` 仅命中他模块的 file.test.ts/trash.test.ts；F-6 引用门禁 web=0 与 vitest 77/77（INDEX.md:126）均为他模块存量用例。沿 terminal/dbmgmt/file-manager 同项 WARN 先例。

**INFO（5 项，不阻断）**
- **I1**：adapter/firewallx 覆盖率 42.9%——未覆盖面为实机 exec/detect 路径（Windows 无 ufw/firewalld），纯函数面全覆盖 + Linux 冒烟归属（tasks.md:727/736）。
- **I2**：api-firewall.md §3.4 未显式列出 AUTH_012（畸形 JSON body 绑定错误实际返回 AUTH_012，handler.go:118-122；errcode/firewall.go:3-5 注释已声明「参数绑定复用 AUTH 码族」，module_test 覆盖该路径）——建议 doc §3.4 补注一行（file-manager Gate 2 先例同项）。
- **I3**：FW_007 data 双层状态后端如实携带（dto.go:95-99 DualLayerData）但前端 FW_007 分支呈现 PageSpec 5.4 固定文案、不读 e.data（index.vue:598-617）——设计接受的简化，非契约断裂。
- **I4**：MainLayout.vue:73-75 注释按「flame」预置防火墙菜单图标，而后端实际登记 icon="shield"（main.go:279）；shield 映射在位（MainLayout.vue:53）运行时命中，flame 条目成冗余+注释滞后，零运行时影响（兜底机制 MainLayout.vue:78-80）。
- **I5**：`.claude/rules/engine-rules.md` 缺席（目录仅 backend/frontend-rules.md），GR-01~06 按角色指令与 SKILL 分层说明执行——沿先例记录。

---

## 9. 总结

**判定：WARN**

- CRITICAL 违例：**0**（后端 G-01~G-07 实跑全过、前端 F-01~F-06 全过、🔴 补充项全过）
- 契约断裂：**0**（AL-001~005 以 api-firewall.md v1.0 为基准全对齐，D-01~D-10/I-01~I-08 落点抽查一致）
- 业务规则适用项：R-02/03/08/09/14/15 + ADR-018 全过（其余 R 项不适用）
- SEC-GATE：探针未被拦 + 模块无桩 → DEBT-SEC-GATE 不适用
- WARN 1 项（W1 前端 0 组件/store 测试）；INFO 5 项（I1~I5）

W1 为交付建议项，与 panel-core/monitoring/service-mgmt/file-manager/terminal 已交付模块同类遗留一致，不构成阻断。
