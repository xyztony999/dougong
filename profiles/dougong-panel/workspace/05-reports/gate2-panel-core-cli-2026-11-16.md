# Gate 2 验证报告 — panel-core CLI 增量（代码→交付门控，增量口径）

**模块**：panel-core（CLI 管理通道增量，ADR-030 / api-panel-core.md v2.1 4.1.4）
**检查时间**：2026-11-16
**扫描路径（增量变更集）**：`backend/cmd/dougong/{cli.go,cli_test.go,main.go,visiturl.go}`、`backend/internal/service/{auth,panel}/cli.go`、`backend/internal/errcode/panel.go`
**检查人**：增量检查员·cli·G2（只查不改，本报告与 INDEX 对应行除外）
**编号源**：engine-rules.md GR-01~06 + go-vue3 栈 backend-rules.md G-01~G-07（Gate 2 执行清单）；业务规则 R-03/08/14（business-concepts §规则池）
**编译门控**：全量 build/vet/test 已由流水线脚本先行实跑全过（任务书声明，本报告不重复，仅增量包补跑见 §5）

---

## 1. 🔴 CRITICAL 扫描（栈规则 grep 实跑摘录）

全部命令在 `backend/`（cwd）实跑，非引用既往结论。

| # | 检查 | 实跑命令（增量文件集） | 输出 | 判定 |
|---|------|----------------------|------|------|
| G-01 | 无 shell 拼接 | `grep -rnE 'exec\.Command(Weak)?Context?\(.+"(-c|/c)"' --include="*.go" cmd/dougong/cli.go cmd/dougong/cli_test.go cmd/dougong/main.go cmd/dougong/visiturl.go internal/service/auth/cli.go internal/service/panel/cli.go` | **0 命中（exit=1）** | ✅ PASS |
| G-01' | exec 参数数组人工核对 | `grep -rn "exec\.Command" --include="*.go" cmd/ internal/service/auth/cli.go internal/service/panel/cli.go internal/errcode/panel.go` | 唯一命中 `cmd/dougong/cli.go:72`：`exec.CommandContext(ctx, "systemctl", args...)`——二进制名+参数数组；`args` 仅两处来源且全为字面量/白名单：`cli.go:239`（`"is-active", cliUnitName`）、`cli.go:515-524`（verb 经 start/stop/restart/status 字面量 switch 收敛后拼 `[verb, cliUnitName]` 或 `["status", cliUnitName, "--no-pager"]`），无任何用户输入直达 argv 元素 | ✅ PASS（R-08） |
| G-02 | 无 SQL 拼接 | `grep -rnE 'Sprintf\(.+(SELECT\|INSERT\|UPDATE\|DELETE)' --include="*.go" cmd/dougong/ internal/service/auth/cli.go internal/service/panel/cli.go internal/errcode/panel.go` | **0 命中（exit=1）**；全仓同式 grep 亦 0 命中（grep_exit=1） | ✅ PASS |
| G-03 | 无硬编码凭据 | `grep -rnEi '(password\|secret\|token\|apikey)\s*=\s*"[^"]{8,}"' --include="*.go" <增量文件> \| grep -v _test` | **0 命中（exit=1）**；新密码仅经 `DOUGONG_INIT_PASSWORD` 环境变量读取（cli.go:343），reset-password FlagSet 只定义 `-username`（cli.go:335），**无 `-password` flag 可供凭据进 argv** | ✅ PASS |
| G-04 | SQL 仅 store 层 | `grep -rnE "(SELECT\|INSERT\|UPDATE\|DELETE) " --include="*.go" <增量文件>` | 非测试增量文件 0 命中；5 处命中全部位于 `cmd/dougong/cli_test.go`（断言用只读 SELECT，含 `?` 参数绑定），规则口径 `_test` 排除 | ✅ PASS |
| G-05 | 无 panic 滥用 | `grep -rn "panic(" --include="*.go" <增量非测试文件>` | **0 命中（exit=1）** | ✅ PASS |
| G-06 | goroutine 退出路径 | `grep -rn "go func" --include="*.go" <增量非测试文件>` | 新增 cli.go / 服务层两文件 **0 处**；main.go 10 处命中全为存量后台任务（main.go:400-557，git diff 证明本轮零触碰——见 §4），均 ctx 取消+wg 编排退出（既往门控已核） | ✅ PASS |
| G-07 | 编译+vet+test | 全量编译门控由脚本先行实跑全过（任务书给定）；本轮增量补跑 `go vet ./cmd/dougong/... ./internal/service/auth/... ./internal/service/panel/...` → **vet=0**；`go test -race -count=1`（同三包）→ **全 ok**（§5） | ✅ PASS |

**G-01 全仓附注**：全仓 grep 唯一命中 `internal/adapter/cronx/runner.go:145`（`sh -c` 命令串为独立 argv 元素）——存量 cron-backup 模块 T07 已登记判定（backend/tasks.md:978），非本增量变更集，不构成本轮违规。

## 2. 契约对齐 — 六命令族 vs api-panel-core.md v2.1 4.1.4（逐项）

唯一事实源：`profiles/dougong-panel/workspace/02-detailed/api/api-panel-core.md` §4.1.4（:293-306）+ §3.4 动作码注（:170）+ §3.3 PANEL_012（:156）+ D-P17（:45）。实现：`backend/cmd/dougong/cli.go`、`backend/internal/service/{auth,panel}/cli.go`。

### 2.1 通用约定

| 契约条款 | 实现证据 | 判定 |
|---------|---------|------|
| 进程内复用、无 HTTP 绕行、不经中间件链 | `openCLIServices`（cli.go:199-225）直开 SQLite→store→service，不 import api 层；cli.go imports 无 `internal/api` | ✅ |
| 退出码 0/2/1（0 成功/2 参数校验 AUTH_012·PANEL_012 语义/1 执行失败） | cli.go:49-53 常量；全部出口经 `cliExitOK/Fail/Usage`（逐命令见 2.2） | ✅ |
| stdout 打印 code 与原因 | 业务码经 `printErrcode`（cli.go:549-561）打印 `SYMBOL: message`+逐字段；裸错误打印原因 | ✅（见 I-2 观察项） |
| 写命令统一追加「请执行 systemctl restart dougong 使变更生效」 | `restartHint` 常量（cli.go:45）——reset-password 成功（:381）、set-port 变更（:427）、entrypath regen（:483）/off（:500）四写分支均输出；测试断言在位（cli_test.go:458/559/640/659） | ✅ |
| 审计 source=cli，动作码全量复用不新增 | `audit.SourceCLI="cli"` 为既有常量（internal/service/audit/audit.go:32，且 :216 校验白名单本就含 cli=枚举池既有值）；panel_config_changed（panel/consts.go:54）/panel_entry_path_regenerated（:56）/password_reset（audit.go:46）全部复用既有常量，零新增 | ✅ |
| 裸 `dougong`（无子命令）=启动服务 | main.go:107-112 分发 switch 仅匹配六子命令名；len(args)==0 直落 `run()`（main.go:117），启动路径零变化 | ✅ |

### 2.2 逐命令

| 命令 | 参数 | 退出码 | 审计 | 生效提示 | 判定 |
|------|------|--------|------|----------|------|
| `status` | 无参（-config 剥离，沿 init 先例） | is-active 非 0 退出码（inactive=3 等）视为正常状态输出→0（cli.go:245-248，测试 :290-296）；通道级失败→1 并 stdout 呈现原因（:241-244） | 无（读命令） | 无 | ✅ |
| `info` | 无参 | 0（读失败路径=1，通用约定） | 无 | 无（测试 :358 断言无 restartHint） | ✅ |
| `reset-password` | `-username` 必填缺失→AUTH_012 退 2（cli.go:339-342）；DOUGONG_INIT_PASSWORD 缺失→PANEL_012 退 2（:343-347，errcode/panel.go:32-35 与 api:156 文案同源）；**无 -password flag**（凭据不经 argv） | A10 长度失败→AUTH_012 退 2（服务层 validatePassword→printErrcode usage 分支 :362-364）；目标用户不存在→1（ErrUserNotFound :358-361，契约钉死区别于 HTTP 的 AUTH_012 语义）；成功→0 | `password_reset`（security，source=cli，同事务）——auth/cli.go:68-78；与 HTTP ResetPassword（user.go:130-158）同源：同 validatePassword/同 bcrypt/同 UpdatePassword→RevokeByUser(cascade)→Audit 单事务（逐行比对通过） | 成功后输出（:381；契约明示「会话吊销即时不依赖重启，提示为统一口径」） | ✅ |
| `set-port` | 非整数/多参/缺参→退 2（:389-397）；范围 1024..65535 双层校验——cmd 层 Atoi + 服务层 ListenPortMin/Max（panel/cli.go:40-45，consts.go:14-15 与 I-P02 同值）违规 AUTH_012 退 2 | 0/2/1（落库失败=1） | `panel_config_changed` detail={field:"listenPort",before,after}（panel/cli.go:54,59-60；fieldDelta JSON 标签 {field,before,after}——service.go:302-306）；**同值零写入零审计**（:51-53，I-05 一致，测试 :581-594） | 变更后输出（:426-427）；同值「无需变更」不输出（未发生变更，契约「完成后」语义成立） | ✅ |
| `entrypath show\|regen\|off` | 未知/缺子命令→退 2（:437-445）；三态白名单 | show=0；regen/off 0/2/1 | regen=`panel_entry_path_regenerated` **脱敏口径同接口**（panel/cli.go:83-86 maskEntryPath 复用 service.go:310；测试 :645-647 断言明文不入审计）；off=`panel_config_changed` field=entryPathEnabled（panel/cli.go:112）；show 无；off 已关闭幂等零审计（:109-110，测试 :672-681）；regen 保持 enabled 不变（I-P03，测试 :637-639） | regen/off 写分支输出（:483/:500——off 幂等分支亦输出，契约「写分支完成后」字面满足，见 I-1 观察项）；show 无 | ✅ |
| `service start\|stop\|restart\|status` | 未知/缺子命令→退 2（:510-523） | 0；2=用法错误；非 0 透传 systemctl 退出码（:540-541，测试 exit=5 透传 :722-726）；通道级失败=1（:537-539） | 无（OS 层治理，FR-02/§3.4 :170「服务启停不落审计」；测试 :755-757 断言零审计） | **不输出 restart 提示**（:754 测试断言；契约「本命令族即启停治理本身」） | ✅ |

### 2.3 附带核验

- **status 输出三要素**：systemd 状态（is-active 投影）+初始化态（EnsureRow+IsInitialized，cli.go:257-265）+访问 URL（`visitURLs` 纯函数 + scheme 按 https_mode≠off 推导 + 入口开启时 `/{entryPath}` 前缀，:272-281；测试 :326-341 含 https+前缀+http 负断言）✅
- **info 五要素**：版本（GetVersionInfo——与 `/panel/system/version` 同一函数，api/panel/handler.go:152 与 cli.go:311 同源）+监听 scope:addr:port+入口开关与值+HTTPS 四态+数据目录=filepath.Dir(db_path)（:311-324）✅
- **PANEL_012**：errcode/panel.go 新增仅此一项，注释标 v2.1/ADR-030；存量 PANEL_001~011 零改动（diff 证据）✅

## 3. R-03 / R-08 / R-14 适用项

| 规则 | 口径 | 核验 | 判定 |
|------|------|------|------|
| R-03 | 所有变更类操作写审计（谁/何时/改了什么/结果） | 四个写路径全部同事务落审计（§2.2）：set-port/entrypath regen·off 走 `withTx` 内 `tx.Audit().Append`（panel/cli.go:55-61/87-93/114-120）；reset-password 同（auth/cli.go:61-79）；actor=终端系统用户标签（cliOperator，cli.go:116-121，兜底 "cli"）；读命令与服务启停按 §3.4 :170 契约明示不落审计；零变更零审计（I-05「仅审计实际变更」先例） | ✅ PASS |
| R-08 | 命令执行一律参数数组，禁止 sh -c 拼接 | 增量唯一 exec 点 `execSystemctl`（cli.go:71-91）参数数组；G-01/G-01' grep 实跑 0 命中+人工核对全字面量（§1）；systemctlRunner 可注入 fake 供参数数组断言（cli_test.go:37-45/317-322/742-748） | ✅ PASS |
| R-14 | 全部输入服务端校验，不信任前端 | CLI 侧等效落实：子命令白名单（runCLI/entrypath/service 三层，cli.go:145-161/442-445/516-524）、-username 必填+TrimSpace（:339）、DOUGONG_INIT_PASSWORD 存在性（:343）、端口 Atoi+服务层 1024..65535 权威校验（panel/cli.go:40-45）、A10 密码长度在服务层复验（auth/cli.go:44）——cmd 层校验仅为提示映射，服务层为不变式 | ✅ PASS |

## 4. 存量回归（init / version / 启动路径）

| 项 | 证据 | 判定 |
|----|------|------|
| `dougong init` 零改动 | `git diff backend/cmd/dougong/main.go`：变更仅 doc 头 7 行+六子命令分发块 9 行（插入于 version 分发之后、gin 静音之前）；`runInit`（main.go:623-678）零触碰；`init`/`version` 不在六子命令 switch 内，无遮蔽 | ✅ |
| `dougong version` 零改动 | version 分发（main.go:99-103）在新增 switch 之前且零改动；数据源 GetVersionInfo 未动 | ✅ |
| 裸 `dougong` 启动路径零改动 | len(args)==0 不入任何分发，直落 `run()`（main.go:113-120）；gin 静音逻辑与 `run()` 全函数（:125-592）diff 零触碰；main.go 10 处 goroutine 全为存量（diff 证明） | ✅ |
| visiturl.go 重构零行为变化 | `git diff`：`logVisitURLs` 抽出纯函数 `visitURLs`；旧行为=SplitHostPort 失败早退不打印、否则必打印（hosts 兜底保证 ≥1 URL，git show HEAD 对照）；新行为=仅 len(urls)==0（即解析失败）不打印——等价；TestVisitURLs 三例锁定（cli_test.go:250-271）；启动侧唯一消费点 logVisitURLs（main.go:569）签名不变 | ✅ |
| errcode 存量零破坏 | diff 仅追加 PANEL_012+注释更新，PANEL_001~011 定义原样 | ✅ |
| 存量测试回归 | `go test -race -count=1 ./internal/service/auth/... ./internal/service/panel/...` 含全部存量套件（auth_test.go/panel_test.go 等）→ **ok（0 FAIL）**；cmd/dougong 在 HEAD 无测试文件（git ls-tree 证），cli_test.go 为首个测试文件，无存量用例被顶替 | ✅ |

## 5. 测试实跑（本轮）

命令（cwd=backend/）：`go test -race -count=1 ./cmd/dougong/... ./internal/service/auth/... ./internal/service/panel/...`

```
ok  github.com/dougong/dougong/cmd/dougong          15.174s
ok  github.com/dougong/dougong/internal/service/auth 24.966s
ok  github.com/dougong/dougong/internal/service/panel  4.756s
```

增量测试覆盖面（cli_test.go，770 行表驱动）：splitConfigFlag 矩阵、visitURLs 纯函数、status 三态+初始化/前缀/https、info 五要素、reset-password 四类退 2+退 1+成功链（bcrypt 更新/会话 cascade 吊销/审计四要素/锁定不解锁）、set-port 五类退 2+成功 delta+同值幂等、entrypath 三态+脱敏+幂等、service 八例（参数数组逐字断言/透传/通道失败/零审计零提示）、未知子命令兜底。systemctl 通道注入 fake（不 mock 数据库，backend-03 §6 纪律）。

## 6. 观察项（INFO，不阻断）

- **I-1 生效提示的幂等分支不统一**：`entrypath off` 已关闭态仍输出 restartHint（cli.go:495-500）而 `set-port` 同值不输出（:422-425）。两处均落在契约字面之内（「写分支完成后输出」/「完成后输出（变更场景）」），仅风格不一致，建议后续轮统一口径（零变更分支不提示或都提示）。
- **I-2 退 1 路径的 code 呈现**：reset-password 目标用户不存在打印「目标用户 %q 不存在」未带业务码符号（该场景 HTTP 侧复用 AUTH_012 而 CLI 契约钉死退 1，无专属码可打）；其余退 1 路径（通道失败/落库失败）为基础设施错误本无业务码。契约「stdout 同步打印 code 与原因」在「对应错误码语义」存在处均已满足（printErrcode）。建议后续如需严格化可评估哨兵码化。

## 7. 总结

**判定：PASS（CLI 增量范围）**

- 🔴 CRITICAL：G-01~G-07 增量范围全过（grep 实跑，摘录见 §1）
- 契约对齐：六命令族 vs api v2.1 4.1.4 逐项零差（参数/退出码/审计动作码/生效提示/通用约定，§2）
- R-03/R-08/R-14 适用项全过（§3）
- 存量回归：init/version/启动路径零改动或等价重构，存量测试全绿（§4-5）
- INFO 2 项（§6），无新增 WARN；模块级既往 WARN（前端侧 W4 等）不属本增量变更集，延续原状态

**未执行/引用项如实声明**：全仓 41 包编译+全量测试门控未在本轮重复实跑（任务书明示「编译门控已由脚本实跑全过」，本报告仅引用该声明并补跑增量三包 vet+test）；systemctl/systemd 真实通道联调属 Linux 目标环境冒烟（Windows 开发机无 systemd），本轮以 fake 注入+参数数组字面断言覆盖，与 T14 任务记录口径一致。
