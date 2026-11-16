# Gate 2 验证报告 — 代码→交付门控（panel-core 独立重验轮）

**模块**：面板基座（panel-core，domain: core）
**扫描路径**：backend/internal/{api/panel,api/middleware/entrypath.go,service/panel,store/panel,store/migrations/002_panel_core.sql,errcode/panel.go} + cmd/dougong/main.go（装配）+ frontend/src/{types/panel.ts,api/panel.ts,api/http.ts,stores/panel.ts,router/index.ts,layouts/MainLayout.vue,views/panel-core/**}
**检查时间**：2026-11-14（复验轮：本轮为独立全量重验，非引用前轮结论；覆盖同名前轮报告）
**契约基准**：workspace/02-detailed/api/api-panel-core.md v1.0（7 接口 / 6 错误码 PANEL_001~006 / 4 枚举 / 2 表 DDL / EntryPathGuard / ModuleMenuRegistry）
**规则链**：引擎 GR-01~GR-06 ⊕ backend-rules §7 G-01~G-07 ⊕ frontend-rules §6 F-01~F-06 ⊕ business-concepts R-01~R-18（适用项）
**脚本门控（引用，未重跑）**：build=0 vet=0 test=0 web=0

**判定**：**WARN**（🔴 CRITICAL 0 项、契约断裂 0 项；⚠️ 建议项 3 项 W1/W3/W4；前轮 W2 已闭环）

## 0. 口径与前置说明

| # | 项 | 说明 |
|---|----|------|
| P-1 | 引擎规则载体缺席 | `.claude/rules/engine-rules.md` 不存在（.claude/rules/ 仅 backend/frontend 两文件，前轮报告引 engine-rules.mdc:52-57 系错误引用）。按 SKILL.md 分层说明执行 GR 语义（扫描必执行/编译必过/E2E 必补/前后端契约对齐），与 service-mgmt 轮先例一致。INFO-1 记录，不阻断 |
| P-2 | SEC-GATE 债务口径 | 按 2026-11-14 裁定执行：探针通过且模块无桩 → DEBT-SEC-GATE 不适用（见 §3） |
| P-3 | 包络字段名 | backend-rules §4 示例为 `{code,msg,data}`（🟡 级描述性条款）；本模块契约 §3.1 明文 `{"code":0,"message":"ok","data":...}`。按任务指令以 api 文档为基准判定。INFO-3 记录 |
| P-4 | grep 环境 | Windows Git Bash，grep 输出路径为反斜杠（影响 G-04 过滤器，已人工核对层外命中，见 §1） |

## 1. 后端栈规则扫描（G-01~G-07，实跑）

扫描根：`backend/`（全仓扫描后按模块归属核对）。

| 编号 | 检查 | 命令（实跑，cwd=backend/） | 输出 | 判定 |
|------|------|-------------|------|------|
| G-01 | 无 shell 拼接 | `grep -rnE "exec\.Command(Weak)?Context?\(.+\"(-c\|/c)\"" --include="*.go" .` | 0 命中（exit=1） | **PASS** |
| G-01b | exec.Command 全量人工核对（R-08） | `grep -rn "exec\.Command" --include="*.go" .` | 仅 1 命中：`internal\adapter\nginxx\parser.go:3` 为注释文本（「外部命令经 exec.CommandContext 参数数组执行…」），全仓 **0 个实际 exec 调用** → R-08 by-absence 成立 | **PASS** |
| G-02 | 无 SQL 拼接 | `grep -rnE "Sprintf\(.+(SELECT\|INSERT\|UPDATE\|DELETE)" --include="*.go" .` | 0 命中（exit=1）。store/panel 动态 SET 列名为静态字面量白名单、值一律 `?` 参数（config_repo.go:64-109） | **PASS** |
| G-03 | 无硬编码凭据 | `grep -rnEi "(password\|secret\|token)\s*=\s*\"[^\"]{8,}\"" --include="*.go" . \| grep -v _test` | 0 命中（exit=1） | **PASS** |
| G-04 | SQL 只在 store 层 | `grep -rlnE "(SELECT\|INSERT\|UPDATE\|DELETE) " --include="*.go" . \| grep -v internal/store \| grep -v _test` | 反斜杠路径致过滤器失效；层外实际命中 3 文件，逐行核对全为**注释文本**：`service/auth/user.go:114`（ON DELETE CASCADE 说明）、`service/fileman/scope.go:20`、`service/fileman/trash.go:36,81` | **PASS**（4 命中均非代码） |
| G-05 | 无 panic 滥用 | `grep -rn "panic(" --include="*.go" . \| grep -v _test` | 0 命中（exit=1） | **PASS** |
| G-06 | goroutine 有退出路径 | `grep -rn "go func" --include="*.go" . \| grep -v _test` | 5 命中全在 `cmd/dougong/main.go`：213/228/243（审计清理/监控采集/分块清理三 worker，`wg.Add+defer wg.Done`+ctx 取消编排）、281/287（HTTP/TLS serve，`httpSrv.Shutdown` 收口 + serveErr 通道）——逐一核对均有退出路径 | **PASS** |
| G-07 | 编译+vet+test 全绿 | 引用脚本门控 build=0 / vet=0 / test=0（任务指令供引用，未重跑）；本轮另实跑 `go test -count=1 -cover` 模块 3 包全 ok（见 §6） | — | **PASS** |

## 2. 前端栈规则扫描（F-01~F-06，实跑）

扫描根：`frontend/`。

| 编号 | 检查 | 命令（实跑，cwd=frontend/） | 输出 | 判定 |
|------|------|-------------|------|------|
| F-01 | 无 Options API | `grep -rLn "script setup" --include="*.vue" src/` | 0 命中（exit=1） | **PASS** |
| F-02 | 无裸 any | `grep -rnE "(:\s*any\|as any)" --include="*.ts" --include="*.vue" src/` | 0 命中（exit=1） | **PASS** |
| F-03 | 请求只走 api 层 | `grep -rnE "(axios\|fetch)\(" --include="*.vue" src/views src/components` | src/components 目录不存在（INFO-2）整链 exit=2 → 重跑 `--include="*.vue" src/views` 与 `--include="*.ts" src/views` 均 0 命中；panel-core 6 Card + about 页人工核读全部经 `@/api/panel` | **PASS** |
| F-04 | 无混入其他组件库 | `grep -rnE "from '(antd\|element\|ant-design-vue)'" src/` | 0 命中（exit=1） | **PASS** |
| F-05 | 无原生弹窗 | `grep -rnE "(^\\\|\\W)(alert\|confirm)\(" --include="*.vue" src/`；补全口径加 `--include="*.ts"` | 两次均 0 命中（exit=1） | **PASS** |
| F-06 | typecheck + build 全绿 | 引用脚本门控 web=0（任务指令供引用，未重跑） | — | **PASS** |

## 3. SEC-GATE 门禁探针（裁定口径）

| 项 | 结果 |
|----|------|
| 探针 | `git --version` → `git version 2.56.1.windows.1`，`PROBE_EXIT=0`——**未被拦，Mimosa 已禁用** |
| 模块桩排查 | `grep -rniE "合规桩\|compliance.?stub\|stubbed\|mimosa" internal/{api/panel,service/panel,store/panel} errcode/panel.go api/middleware/entrypath.go migrations/002_panel_core.sql` → 0 命中（exit=1）；`t.Skip\|testing.Short` 在 3 个测试文件均 0 计数 |
| 结论 | 探针通过且模块无桩 → **DEBT-SEC-GATE 不适用**，无「未实现功能留桩」从严判定情形 |

## 4. 前后端接口对齐（AL-001~005，以 api-panel-core.md 为基准）

| 编号 | 检查 | 证据 | 判定 |
|------|------|------|------|
| AL-001 | Controller 路径/方法/权限 vs API 文档 | routes.go:32-47：GET `/panel/{config/detail,menu/list,system/version}`（两角色）+ POST `/panel/{config/update,config/regenerate-entry-path,config/upload-certificate,system/update-check}`（admin+CSRF），与 §3.3 权限矩阵/§4.1.4 接口表 7/7 一致；链序 InitGuard→AuthRequired→CsrfGuard→RoleRequired（routes.go:33-42）与 §3.1 一致；EntryPathGuard 经 main.go:195 以 net/http 包装器全局前置（gin 路由解析先于中间件——entrypath.go:17-27 论证成立），未携带/错携带 → 标准 404 无业务码（entrypath.go:48-50）符合 D-P03 | **PASS** |
| AL-002 | 前端 Action 路径 vs Controller | api/panel.ts:19-53 七函数路径与 routes.go 逐一相同（7 条，GET 3/POST 4）；PageSpec 7 Action 全覆盖无虚构接口 | **PASS** |
| AL-003 | DTO 字段 vs 前端 VO | dto.go 与 types/panel.ts 逐组对齐：listen{scope,address\|null,port,effectMode}、entryPath{enabled,path\|null,effectMode}、https{mode,cert{source,issuer,notBefore,notAfter}\|null,effectMode}、updateCheck、lang{value,supported,effectMode}、pendingRestart、regenerate{entryPath}、upload{certPem,keyPem}→{cert}、version{version,gitCommit,buildTime}、update-check{currentVersion,latestVersion\|null,updateAvailable,checkedAt}、menu{items:[{key,title,path,icon,order,children}]}——唯一偏差：session 透传后端 `int64` 秒（dto.go:68-71）vs 前端 `string`（types/panel.ts:68-71）→ **W1**（契约 §4.1.4 未钉类型，非断裂） | **PASS（含 W1）** |
| AL-004 | 枚举三方同值 | https_mode{off,self_signed,imported} / listen_scope{all,localhost,custom} / panel_cert_source{self_signed,imported} / config_effect_mode{immediate,restart_required}：api §3.4 = store/panel/models.go:23-34 + service/panel/consts.go:7-10 = frontend enums.ts:77-103（四 Record 键逐值相同） | **PASS** |
| AL-005 | 错误码前端可识别 | PANEL_001~006 后端 errcode/panel.go:7-18（symbol 即契约 code 列原值）经 response.go:34 `ec.Symbol()` 上线；前端 types/panel.ts:18-25 PANEL_ERROR_TEXT 六码全配 + http.ts:90-94 拦截器统一提示；module_test.go 以契约符号断言 HTTP 通道：AUTH_001(:231)/AUTH_008(:335)/AUTH_003(:339)/PANEL_005(:355)/PANEL_004(:361)/PANEL_001(:452)/PANEL_006(:455)/AUTH_012(:458)/PANEL_002(:471)/PANEL_003(:475) | **PASS** |

**包络**：response.go:18-27 `{"code":0,"message":"ok","data":...}` 与契约 §3.1 逐字一致；HTTP 一律 200、失败 code=符号串（数值错误码不使用）。

**DDL 逐字**：migrations/002_panel_core.sql:6-28 与契约 §5.3 完全一致（panel_config 10 列 + panel_cert 8 列、CHECK 含 listen_port BETWEEN 1024 AND 65535、单行 id=1）。

## 5. 业务规则适用项（R-01~R-18）与关键不变量

本模块接口面适用 R-02/03/08/09/10/14（R-01/15/16/17/18 等归 account-auth/site-mgmt/file-manager/terminal 域）：

| 编号 | 规则 | 证据 | 判定 |
|------|------|------|------|
| R-02 | HttpOnly Cookie 会话 + CSRF | /panel/* 全部挂 AuthRequired（routes.go:36），4 个 POST 挂 CsrfGuard（routes.go:42）；前端 http.ts:113-116 POST 统一注入 X-CSRF-Token、credentials same-origin、token 不入 localStorage | **PASS** |
| R-03 | 变更类操作写审计（谁/何时/改了什么/结果） | config.go：UpdateConfig 同事务审计 panel_config_changed detail={field,before,after}[]（:222-226）、entryPath 0→1 自动生成补 panel_entry_path_regenerated（:227-236）、Regenerate（:263-268）、UploadCertificate panel_cert_updated（:307-312）；auditEntry 四要素（service.go:230-242）；update-check 触发不落审计=契约 §3.4 明文口径（PRD FR-16） | **PASS** |
| R-08 | 禁 shell 拼接 | §1 G-01/G-01b：全仓 0 个 exec 调用，无 sh -c | **PASS**（by-absence） |
| R-09 | 零遥测零外呼 | update_check 默认 0（DDL DEFAULT 0）→ PANEL_004 拒绝（updatecheck.go:24-26）；=1 且显式触发才单次外呼、无自动重试（:33-39）；模块无其他外呼点；版本源未装配触发即 PANEL_005（:27-32，A-P8） | **PASS** |
| R-10 | 监听默认/随机高位端口、安全入口可选、HTTPS 建议不强制 | EnsureSingleton 缺失时随机高位端口插入（service.go:133-147 randomListenPort [1024,65535]）；panelListenAddr 枚举映射（main.go:318-329）；entry_path_enabled 默认 0；https_mode 默认 off，前端仅 info 级建议横幅（settings/index.vue:41-43，FR-07） | **PASS** |
| R-14 | handler 层服务端校验 | UpdateConfig 字段级校验序列（config.go:39-133：端口范围/地址格式/枚举取值→AUTH_012 fields）、跨字段 PANEL_001（:137-156）、I-P08 PANEL_006（:159-161）、CertPairValidator PANEL_002（config.go:289-294）、upload 入参 binding required（dto.go:103-104）——不信任前端 | **PASS** |

**关键不变量抽查（源码+测试双证）**：

| 编号 | 证据 |
|------|------|
| I-P01 单例 | DDL CHECK(id=1) + EnsureSingleton INSERT OR IGNORE（config_repo.go:19-27）；TestEnsureSingletonIdempotent |
| I-P02 custom⇔address | config.go:135-156（PANEL_001 + 非 custom 携非空地址拒绝 + 存量残留自动清空落审计）；TestUpdateConfigValidationMatrix / module_test.go:452 |
| I-P03 路径正则 | entrypath.go:14 `^[A-Za-z0-9_-]{8,32}$` + 16 字符 96bit 熵生成器（:19-29）；TestGenerateEntryPath |
| I-P04 imported⇔cert 行 | config.go:178-184 PANEL_003 / self_signed 即时生成 :166-177（D-P06）；module_test.go:473-475 |
| I-P05 双重门控 | updatecheck.go:24-32；module_test.go:353-361（开+无源→PANEL_005，关→PANEL_004）；TestUpdateCheckGating |
| I-P06 key_pem 零下发 | dto.go CertSummary 仅 source/issuer/notBefore/notAfter；CertMaterial 仅 main TLS GetCertificate 回调消费（main.go:267-279）；HttpsCard 无「查看私钥」入口 + 成功双清空（HttpsCard.vue:121,181） |
| I-P07 原子保存 | withTx 事务（service.go:112-127）+ Update 无变更短路（config_repo.go:65-67）；TestTxAuditSameTx |
| I-P08 lang∈{zh-CN} | config.go:159-161 PANEL_006；module_test.go:455 |
| 审计脱敏 | maskEntryPath 前 2 字符+长度掩码（service.go:259-268），Regenerate 与 0→1 自动生成均走脱敏（config.go:200,229-232）——readonly 可见审计页不泄路径明文 |
| 菜单服务端过滤（D-P07） | menu.go:83-93 visibleTo（admin 全量/其余须显式列入），MenuNode 不含 visibility 字段（:26-33）；TestMenuRegistryRoleFilter/ChildrenRecursion；MainLayout.vue:91/95 消费 loadMenu，前端不硬编码菜单 |

## 6. 测试覆盖率（本轮实跑）

命令：`go test -count=1 -cover ./internal/service/panel/ ./internal/api/panel/ ./internal/store/panel/`

| 包 | 覆盖率 | 阈值 | 达标 |
|----|--------|------|------|
| internal/service/panel | 81.6% | Service 层 80%+ | YES |
| internal/api/panel | 69.7% | Controller 层 60%+ | YES |
| internal/store/panel | 80.2% | （参照 Service 80%） | YES |

- 模块测试函数 28 个（service 19 + api 5 + store 4），含全栈 HTTP 级用例（TestEntryPathGuardOverHTTP / TestPanelWriteGuards / TestPanelValidationOverHTTP，真实 gin engine + SQLite）——GR「E2E 必补」由 HTTP 全栈用例满足（随本轮 -cover 实跑全 ok）。
- 前端组件层：**views/panel-core 与 panelStore 0 测试**（stores/__tests__/ 仅 file/trash 两文件；views/panel-core 下无 *.test.*）→ W4，模块级未达 60% 组件覆盖口径。

## 7. WARN / INFO 汇总

**WARN（建议项，不阻断）**：

| # | 项 | 证据 | 建议 |
|---|----|------|------|
| W1 | session 透传 B/F 类型不一致：后端 int64 秒（dto.go:68-71，A-P2 裁定）vs 前端 `string`（types/panel.ts:68-71），SessionCard 原样插值展示裸秒数（SessionCard.vue:12-13，无单位换算）。契约 §4.1.4 未钉类型 → 非断裂，沿前轮判定维持 WARN | types/panel.ts:67 注释自认「值格式未在契约中定义」 | 契约补钉类型，或前端改 `number` 并格式化为人类可读时长 |
| W3 | 过期注释仍述「已废弃的数值码通道」：utils/errcode.ts:6-7 称后端 Envelope.Code 为 int 401NN、符号仅存日志——与现状不符（response.go:34 / errcode.go 头注：符号串直出）；http.ts:157 同类陈旧注释。normalizeCode 双通道兼容（string 透传）行为正确，仅注释失真 | errcode.ts:6-7 vs response.go:29-35 | 删改两处注释（纯文档修复） |
| W4 | views/panel-core 6 组件 + about 页 + panelStore 0 vitest 用例（前端组件层模块级覆盖 0） | stores/__tests__/ 无 panel 文件、find views/panel-core 无 *.test.* | 补 panelStore 加载/菜单流与 EntryPathCard 开关确认流用例 |

**前轮 W2（config.go:178-191 重复死代码）已闭环**：本轮全文件核读 config.go，imported 前置检查仅 :178-184 一处活代码，无重复块（INDEX 后端 note 记 T10 轮已删，实证一致）。

**INFO**：

| # | 项 |
|---|------|
| INFO-1 | .claude/rules/engine-rules.md 缺席——GR-01~06 按 SKILL.md 分层说明执行（先例：service-mgmt 轮同口径）；前轮报告引用的 engine-rules.mdc 不存在于本仓 |
| INFO-2 | frontend/src/components 目录不存在——F-03 扫描范围实际作用于 src/views（0 命中）；目录缺席非违规，沿前轮记录 |
| INFO-3 | backend-rules §4 示例包络字段 `msg` vs 模块契约 `message`——按任务指令以 api 文档为基准（response.go 与契约逐字一致） |

## 8. 总结

**判定：WARN**

- 后端 G-01~G-07 实跑全过（4 个层外 SQL 关键字命中均注释、5 个 goroutine 均有退出路径、0 exec 调用）
- 前端 F-01~F-06 全过（F-03 因目录缺席调整范围后 0 命中）
- SEC-GATE：探针通过 + 模块无桩 → DEBT-SEC-GATE 不适用
- AL-001~005 以 api-panel-core.md v1.0 为基准全部对齐；包络/DDL 逐字一致；4 枚举三方同值
- 业务规则适用项 R-02/03/08/09/10/14 全过；I-P01~I-P08 源码+测试双证
- 覆盖率：service/panel 81.6% / api/panel 69.7% / store/panel 80.2%（实跑达标）
- CRITICAL 违规：0；契约断裂：0；遗留 WARN：3（W1/W3/W4）

---

## 9. WARN 修复轮（2026-11-16）

**处置范围**：§7 WARN 表 3 项逐项处置；后端零代码改动（W1 后端侧 SessionView 本为 int64 秒——backend/internal/service/panel/dto.go:68-71，tasks A-P2 裁定，修复方向=契约钉死+前端对齐）。

| # | 处置 | 证据 / 理由 |
|---|------|-------------|
| W1 | **已修复** | ① 契约钉死：api-panel-core.md §4.1.4 GET /panel/config/detail data 内 `session:{absoluteTimeout:int64, idleTimeout:int64}` + 尾注「两值为秒数整数 int64（tasks A-P2 裁定；2026-11-16 WARN 修复轮 W1 钉死，与后端 SessionView 一致；前端展示格式化为人类可读时长）」（api-panel-core.md:235）；② 前端类型同步：frontend/src/types/panel.ts:67-73 PanelSessionTransit 两字段 `string`→`number`，注释删除「值格式未在契约中定义」自认、改述契约钉死；③ 展示格式化：SessionCard.vue:3 引入 formatUptime、:13-14 改 `formatUptime(detail.session.*)`（utils/format.ts:84-94 秒→「N 天 N 小时 N 分钟」人类可读，format.test.ts:127-151 既有断言覆盖；默认 12h/2h → 「12 小时 0 分钟」/「2 小时 0 分钟」）；④ 测试锁定：stores/__tests__/panel.test.ts:29 fixture session={absoluteTimeout:43200, idleTimeout:7200} number 字面量——types 若残留 string 将 typecheck FAIL。契约/B/F 三方现为 int64 秒一致 |
| W3 | **已修复（前轮 site-mgmt W-3 订正，本轮核验在位，零改动）** | 本轮实读 utils/errcode.ts:1-12：现述「后端现状（Gate 2 E1 双侧修复后）……Fail 直接上线 errcode symbol 符号串（成功恒为数值 0），不再发送数值错误码」，与 response.go:31-40 现状一致（`Code: ec.Symbol()` 符号串直出），§7 所指「称 Envelope.Code 为 int 401NN、符号仅存日志」失真表述已不存在；http.ts:157-158 同步订正（「现行后端 symbol 符号串直上……——W-3 订正」）。订正出处：site-mgmt 复检第 1 轮 2026-11-15（INDEX.md site-mgmt gates.gate2 note「W-3 闭环（http.ts:157-158/errcode.ts:4-10 注释订正 symbol 直上）」）；account-auth WARN 修复轮（2026-11-16）核验在位，本轮独立复读一致。normalizeCode 双通道兼容行为未动 |
| W4 | **部分修复（panelStore 用例已补；EntryPathCard 挂载用例维持）** | 已补：stores/__tests__/panel.test.ts 5 用例——loadConfig 成功落位 / 失败 configError 置位且既有 detail 保留 / pendingRestart 派生（D-P02 横幅源）+ loadMenu 成功服务端集直存（D-P07 不二次过滤）/ 失败重挂载先清残留（换角色登录不残留上一角色菜单）；vitest 实跑 5/5 过（见下自验）。维持：EntryPathCard 0→1 确认 Modal 与 1→0 NPopconfirm 开关确认流**挂载级**用例——@vue/test-utils 与 DOM 测试环境（jsdom/happy-dom）均不在 frontend/package.json devDependencies（本轮实查 devDeps 仅 vitest/vue-tsc/vite 等，node_modules 亦无上述两包），引入新依赖超出本轮修复面；该写通道行为已有后端 HTTP 级测试侧证（module_test.go TestPanelWriteGuards / TestEntryPathGuardOverHTTP 覆盖 entryPathEnabled 与 regenerate），组件挂载级用例归组件测试基建后续迭代 |

**自验（本轮实跑）**：

- `npm run test`（cwd=frontend/，vitest run）→ **11 文件 / 120 用例全过**（含本轮新增 panel.test.ts 5 用例，无回归）
- `npm run build`（cwd=frontend/，vue-tsc --noEmit && vite build）→ **exit 0**（built in 26.68s）
- 后端零代码改动 → go build/vet/test 非本轮涉及项未整跑；额外交叉复核 `go build ./...`（cwd=backend/）→ exit 0

**修复轮结论**：W1 闭环、W3 前轮已闭环（本轮核验在位、零改动）、W4 部分收敛（panelStore 0→5 用例；EntryPathCard 挂载用例维持归后续组件测试基建迭代）。

> **复核行（2026-11-16，WARN 复核员）**：§9 三项逐一核验——W1 四点证据在位（api-panel-core.md:235 钉死 session int64 秒；types/panel.ts:67-73 PanelSessionTransit 两字段 number；SessionCard.vue:3/:13-14 formatUptime；utils/format.ts:84-94；stores/__tests__/panel.test.ts:29 fixture 43200/7200 number 字面量 + 5 用例实测）；W3 两处注释（errcode.ts:1-12、http.ts:157-158）现状一致在位；W4 部分修复属实（panel.test.ts 5 用例、frontend/package.json 无 @vue/test-utils/jsdom/happy-dom，grep exit=1 实证）。EntryPathCard 挂载级用例仍未闭环 → **终局判定：warn**。汇总见 05-reports/gate2-warn-fix-review-2026-11-16.md。
