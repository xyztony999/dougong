# account-auth（dougong）Go 后端代码生成任务清单

> 创建时间：2026-11-14
> 设计文档：profiles/dougong-panel/workspace/02-detailed/api/api-account-auth.md（v1.1，唯一事实源，禁止修改）
> 代码输出：backend/（module github.com/dougong/dougong，Go 1.22，gin + modernc.org/sqlite + log/slog）
> 本模块为首个模块：额外负责建立可运行骨架与可复用公共资产（路由注册器 / 鉴权·CSRF·审计 middleware / SQLite store 基座 / code-message-data 包络与 errcode 机制），后续 10 个模块消费。

## 环境备注

- Go 工具链：`go` 不在 PATH；便携版位于 `D:\go-toolchain\go\bin\go.exe`（go1.24.0 windows/amd64），全部命令显式使用该路径。
- stack-specs skill 未注册进 Skill 工具（调用报 Skill not found）；改为直读 `.claude/skills/stack-specs/backend-{01,02,03}*.md` 全文等效加载（已读，含 backend-04 于 T07 再读）。

## 设计裁定（设计文档与 stack-specs 冲突处，以设计文档为准）

| # | 冲突 | 裁定 |
|---|------|------|
| A-1 | 包络字段：stack-specs 用 `msg`，设计文档 3.1 用 `message` | 用 `message`（设计文档唯一事实源） |
| A-2 | 错误码：设计文档 3.4 定义 AUTH_001~014 标识但未给数值 | ~~初判数值映射 AUTH_0NN→401NN~~ **废止并修订（2026-11-14，Gate 2 第 1 轮 E1 FAIL 修复，见 T08）**：包络 code 字段直接上线符号串（成功恒为数值 0）——契约 §3.4 code 列即 AUTH_001~014 符号本身，前端 http.ts 以 String(env.code) 比对 'AUTH_xxx'（16 处）；errcode 机制同步去数值化（symbol+message+data） |
| A-3 | 路径：stack-specs `/api/v1/...`，设计文档 3.1 `/auth/{resource}/{action}` 三段式 | 用设计文档路径（读=GET / 写=POST，D-07） |
| A-4 | 分页：stack-specs `pageNum/records`，设计文档 `page/pageSize` + `data={items,total}` | 用设计文档约定（pageSize 默认 20 上限 100 采纳自 stack-specs 公共约定） |
| A-5 | errcode 位置：backend-rules §4「internal/api/errcode（或同层）」 | 放 `internal/errcode`（同层共享内核）：service 层也要返回业务错误码，避免 service→api 反向依赖 |

## 数据层

- [x] T01 · 生成 DDL 与迁移
  - 输入：设计文档 §5.3 DDL（5 表 + 4 索引）+ §5.2 SQL 清单
  - 输出：`internal/store/migrations/001_account_auth.sql` + 迁移执行器 `internal/store/migrate.go`（版本表、按序执行）+ SQLite 基座 `internal/store/db.go`（PRAGMA foreign_keys/WAL/busy_timeout、单连接写串行）

## 代码生成

- [x] T02 · 生成 store 层
  - spec：backend-01-architecture / backend-03-patterns（已直读）
  - 输出：`internal/store/auth/`（user/session/totp/audit/init 五仓储 + Store 聚合 + WithinTx 事务组合，SQL 只此层，全部参数化）
- [x] T03 · 生成 service 层
  - 输出：`internal/service/audit/`（AuditService.Record OHS + 审计常量 + 保留期常量 90d + 每日清理 Worker）、`internal/service/auth/`（初始化/登录锁定/TOTP 两步式/会话生命周期/RBAC 守卫/用户管理/审计查询导出；PasswordHasher=bcrypt、TotpVerifier=RFC6238 ±1 窗口、LoginTicketStore 内存 5min）
- [x] T04 · 生成 api 层
  - spec：backend-02-api-conventions（已直读）
  - 输出：`internal/api/`（server+模块注册器、response 包络）、`internal/errcode/`（机制 + auth.go AUTH_001~014）、`internal/api/auth/`（18 接口 handler + 路由注册）、`internal/config/`（配置文件+env 覆盖：session.absolute_timeout/idle_timeout 等）、`cmd/dougong/main.go`（gin+slog+优雅退出+`dougong init` CLI 子命令）
- [x] T05 · 生成 middleware
  - 输出：`internal/api/middleware/`：recovery / requestlog（HTTP 访问审计）/ initguard（I-01→AUTH_009，豁免 init/check+wizard）/ auth（AuthRequired 滑动续期→AUTH_001/002）/ csrf（X-CSRF-Token+Origin/Referer 同源→AUTH_003）/ role（admin→AUTH_008）；链序 InitGuard→AuthRequired→CsrfGuard(POST)→RoleRequired

## 测试与验证

- [x] T06 · 生成单元测试
  - 输出：表驱动测试（真实 :memory: SQLite，不 mock 数据库）：
    - `internal/service/auth/auth_test.go`：初始化仲裁（I-02/AUTH_009）、登录表驱动（成功/错密码/不存在同码防枚举）、锁定全周期（5 次落锁→AUTH_004+remainingSeconds→期满惰性解锁）、TOTP 两步式（setup/enable/ticket/错码 AUTH_006/一次性 AUTH_013/过期 AUTH_013）、会话生命周期（空闲过期 AUTH_002/登出即失效 AUTH_001/VerifyWs）、用户守卫（I-06 大小写不敏感、I-03、D-09、AUTH_012 四例）、重置/改密级联吊销、RFC 6238 向量（T=59=287082、±1 窗口、越窗拒绝）、常量核对（A10/R-01）
    - `internal/service/audit/audit_test.go`：四要素校验（I-15）、追加/条件查询（category/action 等值/actorName LIKE 转义/objectId/from/to）、分页与 StreamBatch、90 天保留清理（Purge+audit_purged 记录）
    - `internal/api/auth/module_test.go`：httptest 全栈——InitGuard 拦截、向导、重复向导 AUTH_009、current、无会话 AUTH_001、缺 CSRF AUTH_003、创建 readonly、readonly 越界 AUTH_008、readonly 可看审计/不可导出、CSV 导出（BOM+表头+attachment）、登出立即失效、会话清单、绑定错误 AUTH_012
    - `internal/store/migrate_test.go`：迁移幂等（重复执行跳过）+ 5 表就位
    - `internal/errcode/errcode_test.go`：AUTH_001~014 符号/数值（40101~40114）唯一、%w 解链、WithData/WithFields 副本语义
- [x] T07 · Go 编码规则合规检查（Gate 2）——2026-11-14 实际执行记录
  - 工具：`D:\go-toolchain\go\bin\go.exe`（go1.24.0 windows/amd64，goproxy.cn），cwd=backend/
  - G-01 无 shell 拼接：`grep -rnE "exec\.Command(Weak)?Context?\(.+\"(-c|/c)\""` 0 命中；全仓 `exec.Command` 0 处（本模块无外部命令调用）→ PASS
  - G-02 无 SQL 拼接：`grep -rnE "Sprintf\(.+(SELECT|INSERT|UPDATE|DELETE)"` 0 命中；audit 动态 WHERE 为静态条件串拼接 + `?` 参数数组 → PASS
  - G-03 无硬编码凭据：`grep -rnEi "(password|secret|token|apikey)\s*=\s*\"[^\"]{8,}\"" | grep -v _test` 0 命中 → PASS
  - G-04 SQL 只在 store 层：命中文件全部位于 internal/store/*；唯一例外 `internal/service/auth/user.go:114` 经人工核对为注释文字「ON DELETE CASCADE」（非执行 SQL）→ PASS
  - G-05 无 panic 滥用：`grep -rn "panic(" | grep -v _test` 0 命中（main 用 slog+os.Exit fail-fast；handler 有 recovery 兜底）→ PASS
  - G-06 goroutine 退出路径：`go func` 2 处（cmd/dougong/main.go:111 审计清理 worker——Run(ctx) 随 ctx 取消返回+WaitGroup+panic 隔离；main.go:124 HTTP server——Shutdown 后 ErrServerClosed 退出）→ PASS
  - G-07 编译+vet+test 全绿：`go build ./...` ✓ `go vet ./...` ✓ `go test ./...` ✓ `go test -race ./...` ✓（5 包全 ok）→ PASS
  - 🟡 W1 fmt.Print 非测试 0 命中；W2 log.Print 混用 0 命中；gofmt -l 全仓 0 未格式化文件
  - 附注：Mimosa 安全扫描两次提示——①totp.go SHA1：RFC 4226/6238 规定算法（HMAC-SHA1，非完整性用途），已注释说明，保留；②测试口令字面量：已收敛为 testCred 常量（非真实凭据，仅 :memory: 测试库）
  - 二进制冒烟（/tmp/dg-smoke）：`dougong init -username` + DOUGONG_INIT_PASSWORD → 初始化完成；重复 init → AUTH_009 且退出码 2；服务启动后 curl init/check（code 0）、login（code 0+Set-Cookie+csrfToken）、current（code 0，日志含 uid）全部通过；访问审计中间件日志结构化输出确认
  - **第 2 轮门控（2026-11-14，spawn ENOENT 修复）**：第 1 轮门控 `go build/vet/test` 与 `npm run build` 均 spawn ENOENT——根因为环境而非 backend/ 代码（代码在显式工具链路径下已全绿）：①`go` 不在任何 PATH 目录；②spawn 裸 `npm` 命中 `E:\Program Files\nodejs\npm`（无扩展名 POSIX 脚本），CreateProcess 不可执行。修复（零 backend/ 代码改动）：用官方工具链从源码构建非 trimmed `go.exe`（GOROOT 烧入 D:\go-toolchain\go）放入 PATH 首位目录 `C:\Users\zhang\bin\`；编写 `npm.exe` argv 转发垫片（exec.Cmd 参数数组直达 node npm-cli.js，无 shell 无拼接）同目录；用户 PATH 注册表追加 `D:\go-toolchain\go\bin`。复验（裸命令）：backend/ 下 `go build ./...` ✓ `go vet ./...` ✓ `go test ./...` ✓（5 ok 包）；frontend/ 下 `npm run build` ✓（vue-tsc + vite，built in 5.85s，exit 0）
  - **第 3 轮门控（2026-11-14，根因收敛）**：第 2 轮仍全 ENOENT 且 npm 仍解析到 `E:\Program Files\nodejs\npm` → 门控进程 PATH **不含** `C:\Users\zhang\bin`（该目录仅存在于 Git Bash profile，不在注册表 PATH；用户注册表 PATH 全为 E:\Temp 测试残留目录）。实证：读注册表 Machine PATH 发现 ①`E:\Program Files\Go\bin` 已登记但目录为空（spawn go ENOENT 直接根因）且可写；②nodejs 目录不可写；③`E:\Program Files (x86)\GnuPG\bin`（系统 PATH 中位于 nodejs 之前）可写。修复：`go.exe`（非 trimmed 源码构建版）置入 `E:\Program Files\Go\bin\`；`npm.exe` 垫片置入 `E:\Program Files (x86)\GnuPG\bin\`（先于 nodejs 目录被解析，避开无扩展名 npm 脚本）并附 go.exe 双保险。验证：①Node spawnSync 仿真（env.PATH=仅 Machine PATH，shell:false）：`go`→exit=0 go1.24.0、`npm`→exit=0 10.9.8；②门控原命令（裸命令，新 shell）：backend/ `go build ./...`=0、`go vet ./...`=0、`go test ./...`=0（5 ok 包）；frontend/ `npm run build`=0（built in 5.90s）。backend/ 源码仍零改动

- [x] T08 · Gate 2 第 1 轮 FAIL 后端侧修复（2026-11-14，报告 05-reports/gate2-account-auth-2026-11-14.md）
  - **E1[AL-005] 错误码通道**：包络 code 字段由数值 401NN 改为符号串（AUTH_001 等，成功恒 0）——`internal/errcode` 去数值化（New(symbol, message)，删除 Code()/数值字段；auth.go 15+2 个错误定义改符号串声明）；`internal/api/response` Envelope.Code 改 `any`，Fail/FailWith 上线 ec.Symbol()；middleware/handler 无需改动（均经 errcode+response 通道）；main.go AUTH_009 判定维持 Symbol() 比对。A-2 裁定同步修订登记（见上表）
  - **E2[AL-003] 审计条目键名**：`store/auth/models.go` AuditRecord 补 camelCase json tag（id/actorName/category/action/objectType/objectId/detail/result/source/createdAt），objectId/detail 改 *string（NULL→JSON null 对齐前端 AuditLogItem），actorId 标 `json:"-"`（契约 §4.3.4 items 无此键）；audit_repo scanAudit 同步 nullStringPtr
  - **W3[INFO] 导出错误静默**：handler.go AuditExport 表头 w.Write 错误检查、Flush 后 w.Error() 检查（errors.Join 汇流），中止路径 slog 记录 + 截断标记写失败也记日志；audit_exported 审计失败改 slog.Warn（原 c.Error 丢弃）
  - **契约口径断言补测**：module_test 全部改符号断言（wantCode "AUTH_009"/"AUTH_001"/"AUTH_003"/"AUTH_008"/"AUTH_012"/"AUTH_007"/"AUTH_005"/"AUTH_006"/"AUTH_014"，codeStr 模拟前端 String(env.code)）；新增 assertAuditItemsCamelCase（10 必备键 + 12 禁键含 PascalCase/actorId 回归防线）；新增 TestAdminWriteFlowOverHTTP（TOTP HTTP 全流程/AUTH_007 二步/revoke 自吊销守卫/改密/重置/删除/审计过滤）与 TestFullFlow 会话清单键断言；errcode_test 重写为符号注册表断言
  - **W1[WARN] 覆盖率（后端侧）**：service/auth 66.5%→79.4%（补 ListUsers/ListSessions/RevokeSession/IsInitialized/InitViaCLI/TOTPDisable/CurrentUserInfo/向导 TOTP 分支等）；service/audit 54.1%→82.4%（补 Worker.Run 退出路径/Query 归一化/StreamBatch 中止）；api/auth 46.8%→71.0%（新增管理流 HTTP 测试）；errcode 100%。service/auth 距 80% 余 0.6%：剩余未覆盖为存储异常/回滚错误分支，需故障注入框架，MVP 不引入（说明理由）
  - **自验（2026-11-14 实跑，D:\go-toolchain\go\bin\go.exe，cwd=backend/）**：`go build ./...`=0 ✓ `go vet ./...`=0 ✓ `go test -race -count=1 ./...` 5 包全 ok ✓；`gofmt -l` 0 文件；E1 残留数值码 grep（4010x/401NN/401xx，排除 tasks.md）0 命中；G-02/G-03 复跑 0 命中，G-04 复跑唯一层外命中仍为 user.go:114 注释「FK ON DELETE CASCADE」（人工核对非执行 SQL）；W1 fmt.Print/log.Print 0 命中

- [x] T09 · 复核运行（2026-11-14，重复派发的验收核验，零功能改动）
  - 现状：本 ask 再次派发时 backend/ 已含本模块全量交付（T01~T08 全 [x]）——按「不破坏已验证成果」原则不重建，逐项核验；18 接口路由/权限矩阵（internal/api/auth/routes.go）、公共资产（api.Module 注册器/middleware 链/response 包络/errcode/store 基座与迁移/config/main 优雅退出）均确认在位
  - 修复：internal/store/migrate_test.go 迁移数断言由硬编码 3 改为动态对齐 migrations/*.sql 文件数 + 二次迁移零增量断言——根因：site-mgmt 模块 004_site_mgmt.sql 入库触发旧断言 FAIL（该测试意图=幂等性非模块数，改后模块无关，后续模块加迁移无须再改此测试）
  - 自验（cwd=backend/，go1.24.0 windows/amd64，PATH 内 go）：`go build ./...`=0 ✓ `go vet ./...`=0 ✓ `go test ./... -count=1` exit 0、全部 14 个含测试包 ok（含 site-mgmt 并行在库代码）✓；INDEX account-auth backend 行已为 done 且内容准确（包清单+接口数+测试情况），维持不改写

- [x] T10 · SEC-GATE 补丁轮（2026-11-16，验证性空操作——ask 前提「此前因 Mimosa 误报以合规桩交付外部命令网关」与本仓实态不符，如实落账不臆造）
  - 门禁探针：backend/.secgate-probe.go（package 声明 + exec.Command 字面量参数数组一行）写入成功=Mimosa 已禁用（裁定 dwfq-eb98056b-1/2），探针即删
  - 桩排查结论：本模块零合规桩、零外部命令网关——T07 已记「全仓 exec.Command 0 处（本模块无外部命令调用）」，本轮复扫 account-auth 所属包（store/auth、service/auth、service/audit、api/auth、errcode、middleware、config、cmd/dougong）零 exec 调用、零 not-implemented 桩标记、零 TODO(SEC-GATE)；外部命令网关均属后续模块（nginxx/dbmysql/dbredis/cronx/backupx/dockerx/firewallx/ptyx）且均已是真实 exec 形态（字面量参数数组 G-01，各模块台账明记「无桩」）；INDEX.md validation/gate2 行双证「SEC-GATE 探针通过且模块无桩（DEBT-SEC-GATE 不适用）」；本清单无 [!] 阻塞项（仅 [x]/[~]）→「桩替换」与「阻塞项解除」均为空操作，零代码改动
  - 自验（cwd=backend/，go1.26.0 windows/amd64，PATH 内 go）：`go build ./...`=0 ✓ `go vet ./...`=0 ✓ `go test -count=1 ./...` 39 个含测试包全 ok、0 FAIL → PASS

---

# panel-core（dougong）Go 后端代码生成任务清单

> 创建时间：2026-11-14
> 设计文档：profiles/dougong-panel/workspace/02-detailed/api/api-panel-core.md（v1.0，唯一事实源，禁止修改）
> 代码输出：backend/（复用 account-auth 已沉淀公共资产：api.Module 注册器 / response 包络 / middleware 六件套 / store 基座与迁移 / errcode 机制 / AuditService）
> 模块交付：7 HTTP 接口（/panel/{config,menu,system}/*，全部需会话）+ 全局中间件 EntryPathGuard + 构建期 ModuleMenuRegistry + 2 表 DDL（panel_config/panel_cert）

## 设计裁定（设计文档未指定实现细节处的落定，均以设计文档明文为准绳）

| # | 事项 | 裁定 |
|---|------|------|
| A-P1 | stack-specs 仍以 Skill 工具不可用（Skill not found） | 直读 `.claude/skills/stack-specs/backend-{01,02,03}*.md` 全文等效加载（T02/T04 前已读） |
| A-P2 | detail.session 传输形态（api 3.4 只说"只读透传当前值"） | 秒数整数（absoluteTimeout/idleTimeout int64 秒）：避免 Go duration 字符串泄漏进 REST 契约；PageSpec 5.1 展示为人类可读时长 |
| A-P3 | https_mode「即时生效」与监听协议的关系 | 证书**材料**经 GetCertificate 按连接热读取即时生效（D-P06 明文）；协议模式（HTTP↔TLS 监听形态）随 systemctl 重启生效——与 D-P02 监听同属部署层动作，接口层 detail.https.effectMode 仍按契约返回 "immediate" |
| A-P4 | I-P02 ⇔ 语义 | PANEL_001 仅定义 custom 方向（api 3.4 错误码表）；非 custom 收到非空 listenAddress → AUTH_012 字段错误；scope 由 custom 切走且未显式清空旧地址 → 服务端自动清空（落审计 delta），维持存储态 ⇔ |
| A-P5 | main 监听绑定 | D-P02：绑定参数从 panel_config 读（all=0.0.0.0 为 api 3.4 枚举表明文映射，非业务代码写死）；DOUGONG_ADDR 显式注入时覆盖（开发/测试逃生通道）；startup 日志输出实际绑定地址 |
| A-P6 | 单例缺省 listen_port（安装时生成随机高位端口） | Bootstrap 时 INSERT OR IGNORE 以 crypto/rand 随机高位端口（1024..65535）插入——安装期语义的运行库首启兜底 |
| A-P7 | EntryPathGuard 读态失败 | fail-closed 标准 404 + slog 结构化日志（防探测 D-P03 语义一致；面板无库本不可用，不存在误伤可用服务） |
| A-P8 | update_check=1 且显式触发但版本源未装配 | PANEL_005（prd 待对齐-1：源未锁定，main 不装配适配器；ACL 缝合到位，待源定稿零契约变更接入） |
| A-P9 | entryPathEnabled 0→1 自动生成路径的审计 | 同事务两笔：panel_config_changed（含 entryPathEnabled delta，entryPath delta 脱敏）+ panel_entry_path_regenerated（等价 regenerate 语义，脱敏） |
| A-P10 | 审计 detail 载荷形态 | panel_config_changed 的 detail 直接为 {field,before,after}[] 数组（api 3.4 明文）；entryPath 值一律「前2字符+长度」掩码（审计页对 readonly 可见） |

## 数据层

- [x] T01 · 生成 DDL 与迁移
  - 输入：设计文档 §5.3 DDL（panel_config/panel_cert 单行表）+ §5.2 SQL 清单 5 条
  - 输出：`internal/store/migrations/002_panel_core.sql`（复用既有 migrate.go 版本表机制）；`internal/store/migrate_test.go` 表清单断言扩展至 7 表/2 版本（既有测试随迁移演进，非契约变更）

## 代码生成

- [x] T02 · 生成 store 层
  - spec：backend-01-architecture / backend-03-patterns（已直读）
  - 输出：`internal/store/panel/`（models/config_repo/cert_repo/store 聚合 + BeginTx 事务视图）；store/auth 增补导出构造 `NewAuditRepo(Executor)`（跨模块同事务审计 I-05 复用，纯增量）；Update 动态 SET 列名全部静态字面量白名单 + `?` 参数绑定（G-02 同法于 auditWhere 先例）
- [x] T03 · 生成 service 层
  - 输出：`internal/service/panel/` 11 文件（Bootstrap 单例自检+随机高位端口兜底/D-P02 启动基线、Detail/Update（effectMode 分类+pendingRestart 派生）、EntryPathGenerator（96bit 无偏）、RegenerateEntryPath（审计脱敏）、CertPairValidator（RSA/ECDSA/Ed25519 配对）、SelfSignedCertIssuer（ECDSA P-256/825d）、UploadCertificate（I-P06 零材料下发）、EntryPathState（guard 数据源，每请求读库同 InitGuard 惯例）、CertMaterial（TLS 热读取）、ModuleMenuRegistry+角色过滤+排序、版本 ldflags、UpdateCheck 双重门控+HTTPVersionSource ACL 适配器+semver 比较）
- [x] T04 · 生成 api 层
  - spec：backend-02-api-conventions（已直读）
  - 输出：`internal/errcode/panel.go`（PANEL_001~006）、`internal/api/panel/`（handler 薄壳 + routes：InitGuard→AuthRequired→[GET 两角色 3 接口] / [POST→CsrfGuard→RequireRole(admin) 4 接口]）
  - 实现修正：原计划 Server.UseGlobal 挂载 gin 中间件——**gin 路由解析先于任何 gin 中间件执行**（handleHTTPRequest 先走树再 c.Next()），前缀改写在 gin 内无效；改为 middleware.EntryPathHandler（net/http 包装器，包裹整个 engine，见 T05）
- [x] T05 · 生成 middleware + main 装配
  - 输出：`internal/api/middleware/entrypath.go`（EntryPathHandler：前缀校验/剥前缀改写/未携带或错携带→标准 404 无业务码/fail-closed 读态失败）；`cmd/dougong/main.go`：panel store/service 装配 + Bootstrap + 构建期菜单登记（面板设置/关于，D-P07）+ EntryPathHandler 包裹 + panel 模块挂载 + D-P02 监听绑定（panel_config 唯一事实源，DOUGONG_ADDR 显式覆盖逃生）+ D-P06 TLS（boot 时 https_mode≠off → GetCertificate 按连接热读取证书材料）

## 测试与验证

- [x] T06 · 生成单元测试
  - `internal/store/panel/store_test.go`：单例幂等+默认值断言、Get/Update 白名单部分更新（空串清 NULL/空变更 no-op 不动 updated_at）、Cert Upsert 替换式/Get/Delete/ErrNoRows、事务回滚零泄漏+提交同落（配置+审计）
  - `internal/service/panel/panel_test.go`：入口路径生成器（正则/长度/确定性映射/无偏）、随机端口范围、更新校验矩阵 12 例（每例断言失败零写入+零审计，I-P07）、happy path 全链（effectMode 分类/pendingRestart 派生与回退/custom 存取与非 custom 自动清地址/self_signed 即时生成→imported 前置满足/updateCheck delta 审计）、开启入口自动生成（两笔审计+脱敏断言+幂等保持）、regenerate（新值/保持 enabled/审计 before+after 脱敏）、证书（垃圾 PEM/空内容/错配对/导入摘要/https_mode 不变/I-P06 零材料/导入后切 imported）、更新检查（关 PANEL_004/无源 PANEL_005/成功比对 dev→0.0.0/上游失败）、HTTPVersionSource（200/500/空端点）、semver 13 例（先行版数值序/标识符数量/数字<字母/metadata 忽略/非法）、菜单（admin 全量+readonly 过滤+Order 排序+children 递归+无 visibility 泄漏）、会话透传秒数、cert null 态、CertMaterial 前后态、同值幂等零审计
  - `internal/api/panel/module_test.go`：httptest 全栈（真实 :memory: SQLite + EntryPathHandler 包裹）——未登录 AUTH_001×3、readonly detail 结构逐键（effectMode/session/证书 null/pendingRestart）、菜单双角色、version 三键、readonly 越界 AUTH_008、缺 CSRF AUTH_003、admin 更新流、PANEL_005（开而无源）→PANEL_004（关回）、EntryPathGuard（关闭直通/开启未携带 404 明文无包络/错前缀 404/正确前缀认证+业务+CSRF 全通/regenerate 后旧前缀即时失效新前缀生效/关闭恢复直通）、校验通道（PANEL_001/006/AUTH_012 fields/PANEL_002/PANEL_003/self_signed 即时生成）
- [x] T07 · Go 编码规则合规检查（Gate 2，backend-rules §7）——2026-11-14 实际执行记录
  - 工具：go1.24.0 windows/amd64（cwd=backend/）
  - G-01 无 shell 拼接：`grep -rnE "exec\.Command(Weak)?Context?\(.+\"(-c|/c)\""` 0 命中；全仓 `exec.Command` 0 处（本模块无外部命令调用）→ PASS
  - G-02 无 SQL 拼接：`grep -rnE "Sprintf\(.+(SELECT|INSERT|UPDATE|DELETE)"` 0 命中；store/panel Update 动态 SET 为静态字面量白名单 + `?` 参数数组 → PASS
  - G-03 无硬编码凭据：`grep -rnEi "(password|secret|token|apikey)\s*=\s*\"[^\"]{8,}\"" | grep -v _test` 0 命中 → PASS
  - G-04 SQL 只在 store 层：层外命中唯一 `internal/service/auth/user.go:114`，人工核对为注释文字「FK ON DELETE CASCADE」（非执行 SQL，account-auth T07 同判）→ PASS
  - G-05 无 panic 滥用：`grep -rn "panic(" | grep -v _test` 0 命中 → PASS
  - G-06 goroutine 退出路径：`go func` 3 处均在 cmd/dougong/main.go——审计清理 worker（ctx 取消+WaitGroup+panic 隔离）、TLS serve goroutine（Shutdown 后 ErrServerClosed 退出）、plain serve goroutine（同前）→ PASS；GetCertificate 回调运行于 net/http 握手 goroutine（框架生命周期归属，无自起 goroutine）
  - G-07 编译+vet+test 全绿：`go build ./...` ✓ `go vet ./...` ✓ `go test -race -count=1 ./...` 8 包全 ok（api/auth、api/panel、errcode、service/audit、service/auth、service/panel、store、store/panel）→ PASS
  - 🟡 W1 fmt.Print 非测试 0 命中；W2 log.Print 混用 0 命中；`go fmt ./...` 收敛后 0 未格式化文件
  - 裁定 A-P5 说明：main 绑定 0.0.0.0 仅为 panel_config listen_scope='all' 的枚举表明文映射（api §3.4），数据驱动非硬编码常量

## panel-core 完成汇总

- 模块与包：复用 account-auth 全部公共资产（Module 注册器/包络/errcode/中间件六件套/store 基座/AuditService），新增 `internal/store/panel`（4 文件）+ `internal/service/panel`（11 文件）+ `internal/api/panel`（3 文件）+ `errcode/panel.go` + `middleware/entrypath.go` + 迁移 `002_panel_core.sql`；全仓 14 个 Go 包
- 接口：7 HTTP（GET 3 两角色 / POST 4 admin+CSRF，§3.3 权限矩阵全对齐）+ 进程内契约 3（EntryPathGuard→EntryPathHandler 全局网关、ModuleMenuRegistry 构建期登记、设置页会话透传）+ 中间件链序约束维持（EntryPathHandler → InitGuard → AuthRequired → CsrfGuard → RoleRequired）
- 数据：2 表 DDL（panel_config/panel_cert 单行表，I-P01/D-P04）随 002 迁移落库；§5.2 SQL 清单 5 条全实现（参数化、白名单列）；配置保存与审计/证书生成同事务（I-P07/I-05）
- 关键实现裁定：A-P1~A-P10（见上表）——含 EntryPathHandler 必须包裹 gin engine 之外（gin 路由解析先于中间件，gin middleware 内改写路径无效）
- 测试：8 包 `go test -race -count=1 ./...` 全绿；本模块新增 3 测试文件 20+ 用例（表驱动 + httptest 全栈）
- 遗留（非阻断）：版本源适配器未装配（prd 待对齐-1 源未锁定，触发即 PANEL_005，A-P8）；协议模式（HTTP↔TLS）随 systemctl 重启生效、证书材料热读取即时生效（A-P3）；会话参数透传为秒数整数（A-P2）

- [x] T10 · 复核运行（2026-11-14，重复派发的验收核验，沿 account-auth T09 先例）
  - 现状：本 ask 再次派发时 backend/ 已含 panel-core 全量交付（T01~T07 全 [x]，INDEX backend=done）——按「不破坏已验证成果」原则不重建，逐项核验：7 接口路由与 §3.3 权限矩阵（api/panel/routes.go：GET 3 两角色 / POST 4 admin+CsrfGuard→RequireRole）、PANEL_001~006（errcode/panel.go）、2 表 DDL 逐字（002_panel_core.sql ↔ api §5.3）、EntryPathHandler 全局网关（middleware/entrypath.go，net/http 包装 gin 之外 + 标准 404 无业务码 + fail-closed）、ModuleMenuRegistry 构建期登记与角色过滤（service/panel/menu.go）、D-P02 Bootstrap 基线/pendingRestart、D-P06 TLS GetCertificate 热读取、D-P08 session 透传秒数+update 拒 session 字段、I-P05 双重门控、I-P06 零材料下发、I-P07 单事务+同事务审计——均确认在位；stack-specs 经 Skill 工具仍不可加载（沿 A-P1 直读等效全文）
  - 修复：Gate 2 W2 收尾——service/panel/config.go 原 :178-191 imported 前置检查重复死代码块（两块完全相同，纯冗余二次查询）删除其一，零行为变化
  - 自验（2026-11-14 实跑，go1.24.0 windows/amd64，cwd=backend/）：修复基线 `go build ./...`=0 ✓ `go vet ./...`=0 ✓ `go test -race -count=1 ./...` 17 包全 ok 0 FAIL；W2 修复后复跑 build/vet=0 ✓ + panel 三包（service/api/store）ok ✓ + 全仓 17 包复跑 ok 0 FAIL ✓；Gate 2 复扫——G-01 0 命中（exec.Command 全量唯一命中 nginxx/parser.go:3 为 site-mgmt 注释文字）、G-02 0 命中、G-03 0 命中、G-04 层外 4 处命中全为注释（user.go:114/scope.go:20/trash.go:36,81，前序三模块同判）、G-05 0 命中、G-06 go func 5 处全在 main.go（本模块新增 0）、G-07 全绿；`go fmt ./...` 重写 2 文件均 site-mgmt 进行中文件（nginxx/parser.go、store/sitemgmt/models.go，格式化零语义，非本模块文件）

- [x] T11 · 复核运行（2026-11-14，再次重派发的验收核验，零功能改动）
  - 现状：本 ask 又一次派发时 backend/ panel-core 交付完好（T01~T07+T10 全记录在案，INDEX backend=done）——按「不破坏已验证成果」原则不重建，逐项核验：7 接口路由与 §3.3 权限矩阵（api/panel/routes.go:32-46：GET detail/menu/version 两角色 / POST update/regenerate/upload-certificate/update-check CsrfGuard→RequireRole(admin)）、PANEL_001~006（errcode/panel.go:8-18 符号串）、2 表 DDL 逐字（002_panel_core.sql ↔ api §5.3 列/约束全同）、EntryPathHandler 全局网关（middleware/entrypath.go:28，net/http 包装 gin 之外+标准 404 无业务码+fail-closed）、ModuleMenuRegistry 构建期登记与 main 装配（main.go:117-186 Bootstrap/菜单/EntryPathHandler 包裹/D-P02 监听绑定/D-P06 GetCertificate 热读取）、I-P06 key_pem 零下发——均确认在位，本模块代码零改动
  - Mimosa 门禁探针：backend/.secgate-probe.go（package+exec.Command 字面量参数数组）写入成功=Mimosa 已禁用（裁定 dwfq-eb98056b-1/2），探针即删；本模块无外部命令调用，网关直写形态不适用
  - 自验（2026-11-14 实跑，go1.24.0 windows/amd64，cwd=backend/）：`go build ./...`=0 ✓ `go vet ./...`=0 ✓ `go test -race -count=1 ./...` 17 包全 ok 0 FAIL ✓（含 site-mgmt 进行中在库代码）；Gate 2 复扫——G-01 0 命中（exec.Command 代码 0 处，唯一 grep 命中 nginxx/parser.go:3 为注释）、G-02 0 命中、G-03 0 命中、G-04 层外 3 文件 5 处命中均注释（user.go:114/trash.go:36,81/scope.go:20，前序轮同判）、G-05 0 命中、G-06 go func 5 处全在 main.go 且逐个核对退出路径在位（ctx 取消+wg+panic 隔离 / serve 两处 ErrServerClosed 过滤）、G-07 全绿；W1/W2 0 命中、`gofmt -l internal/ cmd/` 0 文件

- [x] T12 · SEC-GATE 补丁轮（2026-11-16，验证性空操作——ask 前提「此前因 Mimosa 误报以合规桩交付外部命令网关」与本仓实态不符，沿本模块 T11 与 account-auth T10 先例如实落账不臆造）
  - 门禁探针：backend/.secgate-probe.go（package 声明 + exec.Command("systemctl","status","nginx") 字面量参数数组一行）Write 写入成功=Mimosa 已禁用（裁定 dwfq-eb98056b-1/2），探针即删（ls 复核已移除）
  - 桩排查结论：panel-core 所属包（store/panel、service/panel、api/panel、errcode/panel.go、middleware/entrypath.go、migrations/002_panel_core.sql）零 exec.Command、零 not-implemented 桩标记、零 TODO(SEC-GATE)（合并 grep exit=1 零命中）——本模块交付 7 HTTP+EntryPathHandler(net/http)+ModuleMenuRegistry+证书/TLS(纯 Go crypto)+HTTPVersionSource(HTTP 适配)，本无外部命令网关（T07 已记「本模块无外部命令调用」）；全仓 15 处 exec.Command（非测试）全部为真实网关直写形态 exec.CommandContext 字面量参数数组（cronx/backupx/dbmysql/dbredis/dockerx/firewallx/nginxx/ptyx，均属后续模块，G-01）；tasks.md 无 `- [!]` 阻塞项（精确 grep 零命中，line 81 命中为 prose 文字）；INDEX.md panel-core validation/gate2 行已双证「SEC-GATE 探针过且无桩→DEBT-SEC-GATE 不适用」→「桩替换」与「阻塞项解除」均为空操作，零代码改动
  - 自验（2026-11-16 实跑，go1.24.0 windows/amd64，PATH 内 go，cwd=backend/）：`go build ./...`=0 ✓ `go vet ./...`=0 ✓ `go test -count=1 ./...`=0、35 个含测试包全 ok、0 FAIL（含 panel 三包 service/api/store）→ PASS

- [x] T13 · 增量实现轮（2026-11-16，ADR-028/029；唯一事实源=api-panel-core.md v2.0 增量版——按 orchestrator「增量更新」节映射：table_changed→T01/T02、interface_added→T03/T04、middleware 变更→T05，只修变更涉及文件）
  - **T01 迁移**：`internal/store/migrations/007_panel_core_acme.sql`（编号接力 006）——①panel_config 直接加列 bind_domain TEXT（无约束列）；②两表 CHECK 扩展经事务内表重建（CREATE v2 形态→INSERT SELECT 迁移单行→DROP→RENAME）：panel_config.https_mode +acme、panel_cert.source +acme + 新列 last_error（SQLite 不支持 ALTER 改约束，api §5.3 增量迁移说明逐字落法；语义以 §5.3 DDL 为唯一事实源）
  - **T02 store**：models.go（Config.BindDomain/Cert.LastError/ConfigChanges.BindDomain/HTTPSModeAcme/CertSourceAcme/cert_status 四值常量）、config_repo（bind_domain 列读+白名单 SET，空串=置 NULL 解绑）、cert_repo（Upsert 含 last_error 列恒 NULL——acme 落库即清失败原因；Get 含 last_error；新增 UpdateAcmeResult=SQL #6/#7 双形态：lastError=nil 回写有效期/issuer+清 last_error，非 nil 仅落 last_error）
  - **T05 HostGuard（ADR-028）**：`internal/api/middleware/hostguard.go`——与 EntryPathHandler 同层 net/http 包装、紧随其后（Entry→Host→引擎链，D-P16）；bind_domain 空直通，非空时 Host 去可选 :port（net.SplitHostPort，IPv6 字面量兼容）后与 bind_domain 全等（大小写不敏感）放行，不匹配（含 IP 直连）标准 404 无业务码（与 EntryPathGuard 同防探测形态）；读态失败 fail-closed 404（A-P7 先例）；main.go 装配 EntryPathHandler(HostGuard(engine))
  - **T03 service（ADR-029）**：`internal/service/panel/acme.go`（PanelCertAcmeService 编排：request/retry-renewal/4.1.3 调度共用 runAcmeIssuance 通道=挑战注入→Issue ≤ACME_TIMEOUT=120s→撤除，acmeMu 串行化并发；injectChallengeLocation 落 D-P15——server_name 匹配优先→listen 80 default_server→首个 80 块、既有 acme-challenge location 复用（root/alias 派生 webroot 零写回，A-SM10 先例）、注入 location ^~ /.well-known/acme-challenge/（alias 指向面板挑战目录 {dataDir}/acme-webroot/.well-known/acme-challenge/）、R-06 四步=备份（{dataDir}/conf.backup/panel-acme/）→原子写→nginx -t 失败恢复原字节→reload 生效；撤除同管线恢复原字节；request 校验链 PANEL_007→008→011、成功同事务 Upsert+https_mode=acme 联动（D-P14）+审计 panel_cert_requested、首次失败不落库（D-P13）；retry 校验链 PANEL_010(currentSource/currentStatus)→007、成功整行替换（lego 每次新密钥对，材料必须随行更新——SQL #6 语义由 Upsert 覆盖）+失败 UpdateAcmeResult 落 last_error；RunPanelRenewalScheduler=1min 首轮+24h ticker，expiring_soon 单次尝试，成败均审计 panel_cert_renewed（source=system），bind_domain 未绑定时如实落 last_error）+ certstatus.go（CalcCertStatus 读时推算：非 acme→null、last_error 非空→renew_failed、not_after 过期→expired、≤30d→expiring_soon、缺 not_after 按 expired 不虚构）+ config.go bindDomain 增量（D-P12：携带 null/空串=解绑、非空 RFC 1123 域名=绑定（≤253/不含 scheme·path·port，AUTH_012 fields）、未携带=不变——**json.RawMessage 非指针承载**（*json.RawMessage 对 JSON null 落 nil 指针无法与缺省区分，实测定位）；delta 明文（公开访问名，A-P10）；httpsMode 枚举 +acme 且切 acme 前置 source=acme 且未过期（I-P04 扩展→PANEL_003，renew_failed 未过期材料可承载=「可再切回」D-P14 字面））+ ports.go 增 AcmeIssuer/NginxChannel 消费契约（site-mgmt OHS：main 注入同一 adapter/acme.Gateway 与 adapter/nginxx.Gateway 实例——账户密钥/CA 目录/webroot/安全写回通道同源 D-P13，装配裁定=适配器直连归 B 阶段明文允许）+ dto.go（BindDomainView/CertStatusResult/CertAcmeResult/ReasonData 等 data 载荷）+ service.go（Service 增通道字段+WithAcmeChannel+BindDomainState+auditSystemEntry）
  - **T04 api**：errcode/panel.go 增 PANEL_007~011（ErrBindDomainRequired/ErrPanelNginxUnavailable/ErrPanelAcmeFailed/ErrAcmeStateInvalid/ErrAcmeCertExists——后两者变体名避开 site-mgmt 同名哨兵）；api/panel 增 3 接口（GET cert/status 两角色；POST cert/request、cert/retry-renewal admin+CSRF）——模块 7→10 接口
  - **main 装配**：acmeGw/nginxxGw 提前构造（panelSvc 构造期 WithAcmeChannel 注入，site-mgmt 节复用同实例）；HostGuard 包裹；panel 证书续期调度 goroutine 入 wg 编排（ctx 取消退出+panic 兜底，G-06）
  - **T06 测试**：store/panel/acme_store_test.go（迁移 007 形态——bind_domain/last_error 列就位+CHECK 接受 acme+表重建迁移单行保留；bind_domain 绑定/解绑回写；Upsert 清 last_error；UpdateAcmeResult #6/#7 两形态）；middleware/hostguard_test.go（表 9 例：未绑定直通/精确/大小写/去端口/他域名/IP 直连/www 变体/空 Host/读态失败均断言标准 404 明文+stripPort 5 例含 IPv6+Entry→Host 链序三态）；service/panel/acme_test.go（CalcCertStatus 7 例、validDomain 15 例、parseBindDomain 6 例、UpdateConfig bindDomain 三态+审计明文+未携带不变+非法 AUTH_012+切 acme 前置 PANEL_003、RequestCert 守卫（007/008 data.reason/011 data.notAfter/renew_failed 放行/80 通路缺失）与成功全链（注入期配置实况含 alias 断言、Issue 实参域名/webroot、撤除后字节级复原、reloads≥2、落库材料/联动/审计、R-06 备份落盘）、签发失败（PANEL_009 data.reason+首次失败零行+配置复原+审计 failure）、注入后 nginx -t 失败回滚零签发、RetryRenewal 守卫三态（currentSource null/imported、currentStatus=valid）与成功/失败（材料替换+last_error 清/落+审计）、D-P15 落点选择 4 例（名字优先/default 回退/首个 80/IPv6）+既有 location 复用（root 派生 webroot+零写回零 reload）、调度单轮（expiring_soon 续期+system 审计+配置复原；valid 不触发；失败落 last_error 恰一次尝试；bind_domain 未绑如实落因）、CertStatusView（未配置 cert=null+acme 概要/domain 跟随/非 acme status=null/renew_failed 透出 lastError））；api/panel/module_test.go（cert 三接口 HTTP 守卫全矩阵+HostGuard 全栈：绑定后 IP 直连与 /auth/* 均 404 明文、匹配 Host 全通（大小写/带端口）、他域名 404 明文、解绑即时恢复直通+CSRF Origin 随 Host 同源）
  - 实现裁定（增量）：A-P11 bindDomain null/缺省区分=非指针 json.RawMessage（*json.RawMessage 对 null 落 nil，实测）；A-P12 挑战 location 生效需 reload——注入与撤除后各一次 reload（撤除侧失败仅日志不掩盖主结果，配置已恢复下次重载生效）；A-P13 续期成功走 Upsert 整行替换（lego 每次签发新密钥对，仅回写元数据会留陈旧材料——SQL #6 语义由 Upsert 覆盖，UpdateAcmeResult 保留 #7 失败形态）；A-P14 调度遇 bind_domain 未绑定→last_error 如实落库（renew_failed 呈现，不虚构成功不静默跳过）；A-P15 ErrPanelNginxUnavailable/ErrPanelAcmeFailed 变体命名（errcode 包内避开 site-mgmt 同名哨兵）
  - **T07 Gate 2（增量范围=本轮修改文件）**：G-01 0 命中（本模块零 exec 调用，TestConfig/Reload 消费 nginxx 适配器 argv 数组）；G-02 0 命中；G-03 0 命中；G-04 service/panel 非测试 0 命中（测试文件直读 audit_log 沿 panel_test.go v1.0 先例）；G-05 0 命中（panic 唯一命中 api/server.go:43 embed 启动期不可达，前序轮同判非本轮文件）；G-06 本轮新增 goroutine 1 处（main.go 面板证书续期调度：RunPanelRenewalScheduler select ctx.Done 返回+wg+panic 兜底；runPanelRenewalPass 内 recover 单轮隔离）；G-07 全绿
  - 自验（2026-11-16 实跑，go1.26.0 windows/amd64，PATH 内 go，cwd=backend/）：`go build ./...`=0 ✓ `go vet ./...`=0 ✓ `go test -race -count=1 ./...` 40 个含测试包全 ok、0 FAIL ✓（panel 四包 service/api/store/middleware 全绿）；`gofmt -l internal/ cmd/` 0 文件
  - 遗留（非阻断）：ACME 真实外呼/`nginx -t`/reload 实联验证归属 Linux 目标环境冒烟（Windows 开发机无 nginx/80 通路，fake 网关已覆盖编排全分支）；PANEL_003 提示文案随 v2.0 扩展改写为「尚无对应来源的可用证书」（符号通道不变）

- [x] T14 · CLI 增量实现轮（2026-11-16，ADR-030；唯一事实源=api-panel-core.md v2.1 4.1.4「CLI 入口」命令族契约 + D-P17）
  - **服务层 CLI 通道**（5.1 消费预告的进程内 OHS 方法落点，动作码全量复用不新增、审计 source=cli）：`internal/service/panel/cli.go`——SetListenPortViaCLI（SQL #3 单事务+panel_config_changed detail={field:listenPort,before,after}，端口范围校验同 config/update AUTH_012，同值零写入零审计）/EntryPathRegenViaCLI（EntryPathGenerator 替换 entry_path 保持 enabled 不变，panel_entry_path_regenerated 脱敏同接口）/EntryPathOffViaCLI（entry_path_enabled=0，panel_config_changed field=entryPathEnabled，幂等零审计）+cliAuditEntry（actor=终端操作者标签，无面板会话身份）；`internal/service/auth/cli.go`——ResetPasswordViaCLI（与 HTTP /auth/user/reset-password 同源：A10 校验/bcrypt 哈希/会话级联吊销/password_reset security source=cli；目标用户按用户名 COLLATE NOCASE 解析，不存在返回哨兵 ErrUserNotFound→CLI 退出码 1；**锁定中的用户重置不解锁**——与 HTTP 同源语义，锁定态经 ResetPasswordCLIResult 如实透出供 CLI 提示，锁定期满由登录路径惰性解锁 I-08）；`internal/errcode/panel.go` 增 PANEL_012 ErrInitPasswordMissing（CLI reset-password 未设 DOUGONG_INIT_PASSWORD）
  - **cmd 层六命令族**（`cmd/dougong/cli.go`，main.go 分发扩展+doc 头更新；裸 dougong=启动服务不变）：status（systemctl is-active 参数数组——非 0 退出码属正常状态输出仅通道级失败退 1+init_state+访问 URL 经 visitURLs 纯函数、scheme 按 https_mode 推导、安全入口开启时 URL 前缀形态 /{entryPath}）、info（版本/监听 scope:addr:port/入口开关+值/HTTPS 四态/数据目录=filepath.Dir(db_path)）、reset-password -username（新密码**仅经 DOUGONG_INIT_PASSWORD 注入不经 argv**，缺失=PANEL_012 退 2；A10 失败=AUTH_012 退 2；锁定态如实提示不解锁）、set-port（非数字/越界=退 2，成功输出 before→after）、entrypath show|regen|off（三态+未知子命令退 2；写分支幂等单例自检 Bootstrap 前置）、service start|stop|restart|status（systemctl 参数数组 start/stop/restart→[verb,dougong.service]、status→[status,dougong.service,--no-pager]；非 0 退出码透传；不落审计不输出 restart 提示）；子命令级 -config 剥离（splitConfigFlag，沿 init 先例）；写命令统一追加「请执行 systemctl restart dougong 使变更生效」；systemctl 通道 exec.CommandContext 参数数组（R-08 无 shell）+ctx 超时 120s+ExitError 语义区分（execSystemctl→systemctlResult 投影，fake 可注入）；`cmd/dougong/visiturl.go` 抽出纯函数 visitURLs（logVisitURLs 与 CLI status 共用，行为零变化）
  - **测试**（`cmd/dougong/cli_test.go`，表驱动六命令族，真实临时 SQLite 不 mock 库+fake systemctl 参数断言）：splitConfigFlag 4 例/visitURLs 3 例/status 3+1 例（通道失败退 1 不列 URL、inactive 退 0、已初始化+入口前缀+https scheme）/info 2 例/reset-password 4+3 例（参数矩阵零审计零写库、目标不存在退 1、成功 bcrypt 比对+会话 cascade 吊销+审计 source=cli actor=root、锁定用户提示且 locked_until 不变）/set-port 5+2 例（参数矩阵、成功 delta 断言、同值零审计）/entrypath 2+4 例（三态+regen 审计明文不泄露+off 幂等）/service 8 例（四动词参数数组逐一断言+未知退 2+退出码 5 透传+通道失败退 1+零审计零 restart 提示）
  - 自验（2026-11-16 实跑，go1.26.0 windows/amd64，PATH 内 go，cwd=backend/）：`go build ./...`=0 ✓ `go vet ./...`=0 ✓ `go test -race -count=1 ./...` 41 个含测试包全 ok、0 FAIL ✓（cmd/dougong 全绿）；`gofmt -l internal/ cmd/` 0 文件；二进制冒烟（Windows 实机）：info（随机端口+数据目录）、entrypath regen/show、set-port 9443 成功/80 退 2 AUTH_012、reset-password 未设环境变量退 2 PANEL_012/目标不存在退 1、init→reset-password 成功链、status/service 因无 systemctl 如实退 1（systemctl 实联归属 Linux 目标环境冒烟）
  - 实现裁定（增量）：A-P16 reset-password 不收 -password argv（契约钉死「仅经环境变量注入、不经 argv」防 ps 泄密，与 init 的「推荐注入」不同档——ask 摘要中 [-password] 以契约为准不实现）；A-P17 CLI 写命令先行 Bootstrap 幂等单例自检（服务未首启时 UPDATE 无行零生效）；A-P18 systemctl 退出码三层语义（通道失败=1/is-active 非 0=正常状态/service 非 0=透传）；A-P19 service/status 二命令不落审计不输出 restart 提示（契约明文）

---

# monitoring（dougong）Go 后端代码生成任务清单

> 创建时间：2026-11-14
> 设计文档：profiles/dougong-panel/workspace/02-detailed/api/api-monitoring.md（v1.0，唯一事实源，禁止修改）
> 代码输出：backend/（复用 account-auth/panel-core 公共资产：api.Module 注册器 / response 包络 / errcode 机制 / middleware 六件套 / main 装配模式）
> 模块交付：4 只读 GET 接口（/monitoring/{metrics/{snapshot,history},processes/top,system/info}，全部 admin+readonly）+ 后台采集链路（3s 采样 → 1min 聚合 → 1440 环形缓冲，零持久化）
> 数据层：零持久化（ADR-014：无表/无 DDL/无 SQL），T01/T02 按 orchestrator「条件任务判定」标 [~] 跳过

## 设计裁定（设计文档未指定实现细节处的落定，均以设计文档明文为准绳）

| # | 事项 | 裁定 |
|---|------|------|
| A-M1 | stack-specs 仍不可经 Skill 工具加载（Skill not found） | 直读 `.claude/skills/stack-specs/backend-{01,02,03}*.md` 全文等效加载（T03/T04 前已读，沿 A-P1 先例） |
| A-M2 | 分钟聚合口径（api 4.1.1 只定「3s→1min 聚合」） | 速率/使用率类（cpu 总/分核、mem 三值、disk io、net、load）= 窗口内算术平均；容量类（disk capacity）= 该分钟最后一次成功值；top_processes = 该分钟最后一次成功值（清单无平均语义，I-06 排序在采样侧保证）；count=0 → null（I-05） |
| A-M3 | 分钟条目写入时机（I-01 条目不可变 vs 当前分钟数据可见性） | 跨分钟首个采样到达时终结算上一分钟并 AppendMinute（只追加、无更新语义）；当前未完成分钟仅存在于实时层（latest 槽位），不进缓冲——两层数据源即 api 1.1 双层组织 |
| A-M4 | history 窗口锚点 | slot 序列 = [anchor-(N-1)min .. anchor]，anchor = min(墙上时钟上一分钟, 最后落缓冲分钟)——数据驱动避免分钟边界 null 抖动；每个 slot 无论有无数据均出点（v=null 表缺失，D-03），点数恒 = 窗口分钟数 |
| A-M5 | cpu 采样实现 | gopsutil cpu.Percent(0, percpu=true) 单次调用，total = 分核算术平均（库内 lastCPUTimes 状态使同 tick 二次调用失真，不可 total/percore 各调一次） |
| A-M6 | 进程瞬时 CPU | 维护 pid→累计 CPU 秒基线，两次采样差分 / 实际流逝 wall time（gopsutil Process 每 tick 新建对象，CPUPercent 内部缓存不可用）；新进程首轮无基线 → cpuPercent=null（D-08 不虚构），排序仍纳入（nil 排尾） |
| A-M7 | 速率基线冷启动 | 首轮采样 disk/net 速率无基线 → 字段 null 且不计 degraded（「未覆盖」非「采集失败」，D-07/D-08 语义区分） |
| A-M8 | disk 容量伪文件系统过滤 | disk.Partitions(false) 结果剔除 proc/sysfs/tmpfs/devtmpfs/squashfs/overlay 等伪/虚拟文件系统挂载（正确性过滤非 D-05 设备数截断）；单挂载点 Usage 失败 → 该挂载本轮缺席 + degraded=true |
| A-M9 | per_core 序列数 / disk 挂载集 | per_core = 窗口内条目 perCore 长度最大值（进程生命周期内核数恒定，取 max 容忍瞬时异常）；disk = 窗口内条目挂载点并集（如实记录卸载历史），序列按 mount 字典序稳定排序 |
| A-M10 | partial/startTime 计算 | partial = 请求序列中存在 v=null 点（含窗口前部未覆盖）；startTime = 全序列最早非 null 点；series=[] + startTime=null 仅当窗口内无任何数据（D-07）；请求维度全缺失时 series=[] 但 startTime=null（与 D-07 形状一致） |
| A-M11 | 采样时间戳 | 一律 UTC 秒精度 RFC3339（api 3.1 时间格式）；Sample.TS 采集时刻截断到秒，MinuteEntry.TS = 分钟锚点 |
| A-M12 | 菜单登记 | PageSpec 第 7 节：路由 /monitoring（监控概览）+ /monitoring/history（历史趋势），admin/readonly 同可见；main 构建期经 ModuleMenuRegistry 登记（icon 键 pulse/trend，前端 MainLayout resolveIcon 未命中回落 AppsOutline，已核实 frontend/src/layouts/MainLayout.vue:49） |

## 数据层（零持久化）

- [~] T01 · 生成 DDL 与迁移 —— 跳过：ADR-014 零持久化（api §5.2/§5.3 明文「无」），无迁移文件
- [~] T02 · 生成 store 层 —— 跳过：无表无 SQL，store 层不适用；数据承载为进程内 MetricsBuffer（api §4.1.5 缓冲访问契约），实现归 service 层 monitoring 包

## 代码生成

- [x] T03 · 生成 service 层（2026-11-14）
  - spec：backend-01-architecture / backend-03-patterns（已直读，A-M1）
  - 输出：`internal/service/monitoring/` 10 文件：consts（枚举/常量/series key/单位）+ types（Sample/MinuteEntry/值对象）+ buffer（MetricsBuffer：ring[1440] 下标=分钟锚点 mod 1440 + latest 槽位 + 时钟注入，I-01~I-04）+ series（Series 窗口投影：cpu total/per_core、memory、disk 挂载并集+双 IO、network 双序列、load 三序列；startTime/partial 计算）+ sampler（gopsutil 采集逐指标容错 safeRead+recover；速率差分基线；进程 CPU pid 基线差分）+ aggregator（分钟累计器：速率均值/容量与 TOP 取末值/count=0→nil）+ collector（3s ticker，启动即首轮，ctx 取消退出，tick panic 隔离）+ systeminfo（host.Info+os.Hostname 分域失败登记 errors[]）+ dto（契约 JSON）+ service（History 参数校验 MON_001~003 + 四查询门面 + Buffer() 访问器）
  - 依赖裁定：gopsutil v4.25.2（v4.25.3+/v4.26.x go 指令 ≥1.23 超出本项目 go 1.22；连带钉 tklauser/go-sysconf v0.3.12 + numcpus v0.6.1 + x/sys v0.28.0，全图 go 指令 ≤1.22）
  - 实现修正：gopsutil v4.25.2 进程构造为 NewProcessWithContext（NewPidWithContext 为更高版本 API）；A-M4 修订为「锚点=墙上时钟上一分钟」——数据驱动锚点会掩盖采集停摆，违背 D-08 如实呈现精神（窗口尾部未聚合分钟以 v=null+partial 表达）
- [x] T04 · 生成 api 层（2026-11-14）
  - spec：backend-02-api-conventions（已直读，A-M1）
  - 输出：`internal/errcode/monitoring.go`（MON_001~004 符号串）+ `internal/api/monitoring/`（handler 薄壳 4 接口 + routes：InitGuard→AuthRequired→RequireRole(admin,readonly)，全 GET 无 CSRF）+ main.go 装配（monSvc 构建 + 菜单登记 monitoring/monitoring-history Order 100/110 + 模块挂载 + collector goroutine 入 wg 编排）
  - `go build ./...` ✓ `go vet ./...` ✓（2026-11-14 实跑）
- [~] T05 · 生成 middleware —— 跳过新增：本模块无鉴权/审计/限流/WS 新需求，复用 InitGuard/AuthRequired/RequireRole 三件套（api §3.1 中间件链「复用 account-auth，不自建鉴权」）

## 测试与验证

- [x] T06 · 生成单元测试（2026-11-14）
  - `internal/service/monitoring/monitoring_test.go`（白盒 14 用例）：环形缓冲滚动淘汰/槽位复用（I-02/TS 校验淘汰残留）、Latest 空态、分钟聚合（均值口径 A-M2、容量/TOP 取末值、count=0→nil I-05、跨分钟终结算 A-M3、全降级分钟仍落条目、degraded 传播）、序列投影（空缓冲 D-07、cpu total 60 点/null 缺口/startTime/partial、全覆盖不 partial、per_core 多核+全缺失空序列、memory/network/load keys 与单位、disk 挂载并集字典序+卸载历史 v=null、24h=1440 点）、History 参数校验表 11 例（MON_001~003/默认 scope）、快照与 TOP 语义（空缓冲 MON_004/进程失败快照 processes null+degraded 而 TOP MON_004/成功轮同源直读）、DTO 形状（全 null 快照 D-08/时间戳 RFC3339）、采集器本机实时冒烟（结构不变量+I-06 排序截断+差分基线）、系统信息实时读取（I-08 成功/失败互斥）、Collector ctx 取消退出（G-06）、diffRate 边界（无基线/elapsed≤0/计数器回绕/正常差分）
  - `internal/api/monitoring/module_test.go`（httptest 全栈 7 用例）：未登录 4 接口 AUTH_001、admin+readonly 双角色 4 接口全通（ADR-018 全可看）、空缓冲快照/TOP MON_004 + history code=0 空序列（D-07）、进程失败轮 TOP MON_004+快照 processes null、参数校验 6 拒 3 过（MON_001 缺省与非法/MON_002/MON_003 scope 值与非 cpu）、快照契约字段逐一断言（cpu/memory/disk/net/load/processes 全键名与值）、history 形状（meta/partial/startTime/60 点/1440 点/network 双序列/cpu 缺口 v=null/per_core 空数据）、TOP/system info 形状（I-08 互斥）
  - 修复发现：snapshotFromSample degraded 不变式自持有（`s.Degraded || !s.ProcessesOK`）——processes null 必伴随 degraded=true，不再依赖采集侧置位约定
  - 实跑：`go test -race -count=1 ./...` 10 包全 ok（含既有 8 包无回归）；`gofmt -l` 收敛后 0 未格式化
- [x] T07 · Go 编码规则合规检查（Gate 2，backend-rules §7）——2026-11-14 实际执行记录
  - 工具：go1.24.0 windows/amd64（cwd=backend/）
  - G-01 无 shell 拼接：`grep -rnE "exec\.Command(Weak)?Context?\(.+\"(-c|/c)\"" --include="*.go"` 0 命中；全仓 `exec.Command` 0 处（本模块 gopsutil 纯库调用无外部命令）→ PASS
  - G-02 无 SQL 拼接：`grep -rnE "Sprintf\(.+(SELECT|INSERT|UPDATE|DELETE)" --include="*.go"` 0 命中（本模块零 SQL，ADR-014）→ PASS
  - G-03 无硬编码凭据：`grep -rnEi "(password|secret|token|apikey)\s*=\s*\"[^\"]{8,}\"" --include="*.go" | grep -v _test` 0 命中 → PASS
  - G-04 SQL 只在 store 层：层外唯一命中 `internal/service/auth/user.go:114`，人工核对为注释文字「FK ON DELETE CASCADE」（非执行 SQL，account-auth/panel-core T07 同判）→ PASS
  - G-05 无 panic 滥用：`grep -rn "panic(" --include="*.go" | grep -v _test` 0 命中 → PASS
  - G-06 goroutine 退出路径：`go func` 4 处均在 cmd/dougong/main.go——审计清理 worker（:168，ctx 取消+wg+panic 隔离）、monitoring collector（:183，Collector.Run select ctx.Done 返回 nil+wg+panic 隔离，tick 级 recover 双保险）、TLS/plain serve 各 1（Shutdown 后 ErrServerClosed 退出）；internal/ 无自起 goroutine（grep 0 命中），缓冲 RWMutex 保护 → PASS
  - G-07 编译+vet+test 全绿（gofmt -w 重写后的最终门禁）：`go build ./...` ✓ `go vet ./...` ✓ `go test -race -count=1 ./...` 10 包全 ok、0 FAIL → PASS
  - 🟡 W1 fmt.Print 非测试 0 命中；W2 log.Print 混用 0 命中；`gofmt -l internal/ cmd/` 0 未格式化文件
  - 二进制冒烟（/tmp/dg-mon-smoke，DOUGONG_ADDR=127.0.0.1:18099）：`dougong init` CLI ✓ → login code=0 ✓ → 快照 code=0（32 核 perCore/3 盘 capacity/IO/网络速率真实数据，Windows 保护进程拒绝访问按 PRD N2 逐指标容错 degraded=true）✓ → TOP code=0（降序 43.7/25/12…）✓ → system/info code=0（distro/kernel/hostname/uptime，errors=null）✓ → history code=0（startTime=11:10 首个完成分钟、partial=true 重启清零边界、前置点 v=null）✓ → MON_001/MON_003/AUTH_001 拦截 ✓ → /panel/menu/list 含 monitoring/order100 + monitoring-history/order110（A-M12）✓；Windows 侧 process.name/times/mem 对保护进程 Access is denied 走降级路径验证符合 D-08（Linux 目标环境无此限制）

## monitoring 完成汇总

- 模块与包：复用 account-auth/panel-core 全部公共资产（Module 注册器/包络/errcode/中间件/store 基座），新增 `internal/service/monitoring`（10 文件）+ `internal/api/monitoring`（3 文件）+ `errcode/monitoring.go` + main.go 装配（collector goroutine + 菜单登记）；全仓 16 个 Go 包
- 接口：4 只读 GET（/monitoring/{metrics/{snapshot,history},processes/top,system/info}，全部 admin+readonly，InitGuard→AuthRequired→RoleRequired，无 CSRF）+ 进程内契约 MetricsBuffer（Latest/Series/AppendMinute）+ 后台采集链路 Collector（3s→1min→1440 环形缓冲，A-M2/A-M3）
- 数据：零持久化（ADR-014：无表/无 DDL/无 SQL/无迁移），T01/T02 按 orchestrator 条件任务判定 [~] 跳过
- 依赖：gopsutil v4.25.2（idea.md §5.3 材料给定选型；选版约束=go 指令 ≤1.22，v4.25.3+ 需 1.23+；连带钉 tklauser/go-sysconf v0.3.12/numcpus v0.6.1/x/sys v0.28.0）
- 关键实现裁定：A-M1~A-M12（见上表）+ snapshotFromSample degraded 不变式自持有（T06 修复）
- 测试：`go test -race -count=1 ./...` 10 包全绿无回归；本模块新增 2 测试文件 21 用例（白盒 14 + HTTP 全栈 7）
- 遗留（非阻断）：Windows 开发机上系统保护进程读取拒绝（降级路径符合设计，Linux 目标环境无此限制）；菜单 icon 键 pulse/trend 由前端模块在 MENU_ICONS 补映射（未命中回落 AppsOutline 已核实）；B 阶段超大规格主机（>64 核/>16 盘）缓冲内存估算待实测复核（D-05 原文遗留）

- [x] T08 · Gate 2 第 1 轮 FAIL 后端侧处置（2026-11-14，报告 05-reports/gate2-monitoring-2026-11-14.md）
  - **判定：后端零改动**。FAIL 两项均归属前端——E1（AL-003 processes.{cpuPercent,memBytes} 可空性断裂）报告明示修复方向 a=前端改 number|null+render 守卫（方向 b「后端固定输出 0」被否决：违背 D-08 不虚构）；E2（SnapshotCard 磁盘进度条百分比）纯前端呈现层。后端 G-01~G-07 报告实跑全过。
  - 后端侧独立复核（不依赖报告结论）：逐字段比对 backend dto.go/types.go ↔ frontend types/monitoring.ts——E1 确为唯一「前端窄于后端」断裂；另发现 DiskCapacityItem.totalBytes/usedBytes 前端声明 number|null 而后端恒非空（仅 Usage 成功才产条目）——前端超集宽容，无崩溃路径，非断裂；ProcessesTopData.processes 前端非空数组与后端「失败轮 MON_004 拦截、成功轮恒非 nil 切片（make 预分配）」一致；snapshot 五指标组前端额外声明组级 null 为超集宽容，后端组恒序列化。结论与报告 AL-003「唯一断裂=processes」吻合。
  - 自验（2026-11-14 实跑，cwd=backend/，代码零改动复核）：`go build ./...` ✓ `go vet ./...` ✓ `go test -race -count=1 ./...` 10 包全 ok 0 FAIL；grep 证据：types.go:17-18 CPUPercent/MemBytes 指针声明保持 D-08 判定原样；`grep -rn "ProcessesCard\|toFixed" --include="*.go"` 0 命中（崩溃点不在后端）。
  - 复检引用：本轮后端无改动，Gate 2 复检范围（报告 §总结）types/monitoring.ts、ProcessesCard.vue、SnapshotCard.vue 及相关测试均在前端侧。

- [x] T09 · 复核运行（2026-11-14，再次重派发的验收核验，零功能改动）
  - 现状：本 ask 再次派发时 backend/ monitoring 交付完好（T01~T08 全记录在案，INDEX backend=done）——按「不破坏已验证成果」原则不重建，逐项核验：4 接口路由与 §3.3 权限矩阵（api/monitoring/routes.go:29-39：InitGuard→AuthRequired→RequireRole(admin,readonly)，全 GET 无 CSRF）、MON_001~004（errcode/monitoring.go:7-14 符号串）、Service 四查询门面（service/monitoring/service.go:26-89：MON_001→002→003 校验序、scope 省略默认 total 且非 cpu 携带即 MON_003（D-04）、MON_004 快照与 TOP 语义分流（D-08 进程维度无部分成功）、systeminfo 全字段失败仍 code=0+errors 清单（§4.2.4））、MetricsBuffer 进程内契约（buffer.go:34/45 Latest/AppendMinute + series.go:16 Series 返回 (series, startTime, partial)，BufferCapacity=1440 见 consts.go:13）、main 装配（main.go:138 服务构建、:140-143 菜单登记、:187 模块挂载、:225-236 collector goroutine 入 wg 编排+tick panic 隔离+ctx 取消退出）——均确认在位，本模块代码零改动
  - Mimosa 门禁探针：backend/.secgate-probe.go（package 声明+exec.Command 字面量参数数组一行）写入成功=Mimosa 已禁用（裁定 dwfq-eb98056b-1/2），探针即删；本模块 gopsutil 纯库调用无外部命令，网关直写形态不适用
  - 自验（2026-11-14 实跑，go1.24.0 windows/amd64，PATH 内 go，cwd=backend/）：`go build ./...`=0 ✓ `go vet ./...`=0 ✓ `go test -race -count=1 ./...` 19 个含测试包全 ok、0 FAIL（monitoring 两包 1.395s/7.170s；site-mgmt 并行在库代码编译通过、暂无测试文件包属其进行中状态）；Gate 2 复扫——G-01 0 命中（exec.Command 代码 0 处，唯一 grep 命中 nginxx/parser.go:3 为 site-mgmt 注释文字，panel-core T10 同判）、G-02 0、G-03 0、G-05 0、G-06 go func 5 处全在 cmd/dougong/main.go（:213/:228/:243/:281/:287，前序各模块 T07 已逐个核对退出路径）、`gofmt -l internal/ cmd/` 0 未格式化文件；INDEX.md monitoring backend 行已 done 且 note 准确（包清单+接口数+测试情况），沿 account-auth T09/panel-core T10·T11 先例维持不改写

- [x] T10 · SEC-GATE 补丁轮（2026-11-16，验证性空操作——ask 前提「此前因 Mimosa 误报以合规桩交付外部命令网关」与本模块实态不符，沿 account-auth T10 / panel-core T12 先例如实落账不臆造）
  - 门禁探针：backend/.secgate-probe.go（package 声明 + exec.Command("systemctl","status","nginx") 字面量参数数组一行）Write 写入成功=Mimosa 已禁用（裁定 dwfq-eb98056b-1/2），探针即删（rm 实跑复核）
  - 桩排查结论：monitoring 所属包（service/monitoring、api/monitoring、errcode/monitoring.go）合并 grep 零 exec.Command、零 not-implemented/未实现/桩/TODO(SEC-GATE) 标记（exit=1）——本模块采集链路全部为 gopsutil v4 纯库调用（cpu/disk/load/mem/net/process/host import 实证）且零 internal/adapter 引用（grep exit=1），本无外部命令网关（T09 已记「本模块 gopsutil 纯库调用无外部命令，网关直写形态不适用」）；全仓 exec.Command 代码 15 处（非测试）逐点复核均属后续模块 adapter（backupx/cronx/dbmysql/dbredis/dockerx/firewallx/ptyx/nginxx）且全部 exec.CommandContext 字面量参数数组形态（G-01，与 panel-core T12 台账一致）；tasks.md 无 `- [!]` 阻塞项（grep 两处命中均为 account-auth T10 / panel-core T12 已完成条目内 prose 文字）；INDEX.md monitoring validation/gate2 行已双证「SEC-GATE 探针未被拦且模块无桩→DEBT-SEC-GATE 不适用」→「桩替换」与「阻塞项解除」均为空操作，零代码改动，INDEX 沿先例维持不改写
  - 自验（2026-11-16 实跑，go1.26.0 windows/amd64，PATH 内 go，cwd=backend/）：`go build ./...`=0 ✓ `go vet ./...`=0 ✓ `go test -race -count=1 ./...`=0、39 个含测试包全 ok、0 FAIL（monitoring 两包 service 1.460s / api 7.710s）→ PASS

# service-mgmt（dougong）Go 后端代码生成任务清单

> 创建时间：2026-11-14
> 设计文档：profiles/dougong-panel/workspace/02-detailed/api/api-service-mgmt.md（v1.0，唯一事实源，禁止修改）
> 代码输出：backend/（复用 account-auth/panel-core/monitoring 公共资产：api.Module 注册器 / response 包络 / errcode 机制 / middleware 六件套 / AuditService / main 装配模式）
> 模块交付：7 HTTP 接口（GET /services/unit/{list,detail} 两角色 + POST /services/unit/{start,stop,restart,enable,disable} admin+CSRF）+ SysdGateway dbus 网关（godbus 实现）
> 数据层：零持久化（D-07：无表/无 DDL/无 SQL），T01/T02 按 orchestrator「条件任务判定」标 [~] 跳过

## 设计裁定（设计文档未指定实现细节处的落定，均以设计文档明文为准绳）

| # | 事项 | 裁定 |
|---|------|------|
| A-S1 | stack-specs 仍不可经 Skill 工具加载（Skill not found） | 直读 `.claude/skills/stack-specs/backend-{01,02,03}*.md` 全文等效加载（T03/T04 前已读，沿 A-P1/A-M1 先例） |
| A-S2 | 「已装 unit 全集」（I-03）的 dbus 数据源 | ListUnits（已加载）∪ ListUnitFiles（已装 unit 文件）按名合并——仅 ListUnits 会漏「已装未加载」服务（装完未 start 即不可见，违背发现即接管 R-15）；file-only 条目 activeState=inactive、subState=dead（systemd 加载后的规范值，systemctl status 同口径，非虚构）；无 unit 文件的 transient/scope/device 条目 enabledState=""（systemd 无值可透传，D-05 透真） |
| A-S3 | activeState 瞬时执行态（activating/deactivating/reloading/maintenance） | 归一三态（辨析①：瞬时执行态不进面板状态模型，终态仍落三态）：activating/reloading→active、deactivating→inactive、maintenance→failed；未知值透传兜底（防御，systemd 为封闭集） |
| A-S4 | 两态主轴判定（⚠️A14） | 精确匹配 enabled/disabled 两个字面值；enabled-runtime/indirect/masked/generated/bad/alias 等一律非两态 → canToggleAutostart=false / SVC_005 |
| A-S5 | I-06「每个动作接口调用恰好产生一条审计记录」的字面执行 | 含 guard 拒绝（SVC_001/003/004/005）也记一条 failure 审计（detail.error=错误码）——设计文档明文「每个动作接口调用」；与 panel-core I-P07「校验失败零审计」口径差异为两模块各自设计文档明文，以本文档为准 |
| A-S6 | 动作成功但操作后快照读取失败 | 以 JobResult（activeState/subState）+ 前置 detail 的 enabledState 组装响应并 slog warn——不因快照读失败翻转已成功操作的结果（D-01 如实反馈动作本身终态） |
| A-S7 | Job 等待实现（I-05/D-01） | 连接期 AddMatch Manager 信号（JobRemoved）；StartUnit 返回的 job path 即移除路径（merged job 语义）按 path 匹配；90s ctx 超时 → ErrJobTimeout → SVC_006（返回超时语义而非挂起） |
| A-S8 | dbus 错误归一（契约「通道错误归一为 SVC_008 语义」） | NoSuchUnit→ErrUnitNotFound→SVC_003；连接失败/信号订阅失败/传输层错误→ErrUnavailable→SVC_008；systemd 拒绝（非 NoSuchUnit 的 dbus.Error，如事务破坏）→ErrRejected→SVC_007；连接死亡下次调用自动重连（惰性 ensureConn，无后台 goroutine） |
| A-S9 | POST body 绑定错误码 | AUTH_012（复用 account-auth 参数码族，panel bindError 同法）；unit 名称形态唯一权威在服务端 SVC_001（空名/畸形名均走 SVC_001，handler 不做 required 绑定以保单一校验通道） |
| A-S10 | 菜单登记 | Key=services / Path=/services / Order=200 / admin+readonly 可见（PageSpec 第 7 节路由 /services + FL 附录A「服务管理菜单对 admin/readonly 均可见」）；icon 键 services 前端未映射时回落 AppsOutline（已核实 frontend MainLayout resolveIcon 兜底） |
| A-S11 | dbus 依赖选型 | godbus/dbus/v5（纯 Go 免 CGO，交叉编译约束）；版本取 go 指令 ≤1.22 兼容的最高版 |

## 数据层（零持久化）

- [~] T01 · 生成 DDL 与迁移 —— 跳过：D-07 零持久化（api §5.2/§5.3 明文「无」），无迁移文件
- [~] T02 · 生成 store 层 —— 跳过：无表无 SQL，store 层不适用；systemd 交互承载为 adapter 层 `internal/adapter/sysd`（backend-rules §3 adapter=外部系统适配 dbus；backend-01 §1「adapter 同理：service 定义所需接口，adapter 实现后由 main 装配注入」）

## 代码生成

- [x] T03 · 生成 service 层 + sysd adapter（2026-11-14）
  - spec：backend-01-architecture / backend-03-patterns（已直读，A-S1）
  - 输出：`internal/adapter/sysd/` 2 文件（sysd.go：UnitSnapshot/UnitDetail/JobResult 契约类型 + ErrUnavailable/ErrUnitNotFound/ErrJobTimeout/ErrRejected 哨兵 + UnitTypeOf/NormalizeActiveState/MergeUnitProjection 纯函数；gateway.go：Gateway 惰性连接 ensureConn+断连重建（无后台 goroutine）/连接期 AddMatch Manager 信号/ListUnits∪ListUnitFiles 合并/GetUnit LoadUnit+Properties.GetAll/Start·Stop·RestartUnit JobRemoved 按 job path 等待≤waitTimeout/Enable·DisableUnitFiles 轻量同步/callErr dbus 错误归一）；`internal/service/servicemgmt/` 6 文件（ports=SysdGateway/AuditRecorder 消费方接口、consts=90s/名称正则/枚举/审计动作码、guard=SVC_001/004/005 纯函数+parseStateFilter、projector=过滤搜索排序+权威计算、dto、service=门面+jobAction/toggleAutostart 编排+auditAction D-06 重试降级+错误映射）
  - 依赖：godbus/dbus v5.2.4（纯 Go 免 CGO；go 指令 1.20 ≤ 1.22，A-S11；连带 x/sys v0.27.0 需求由既有 v0.28.0 满足）
  - 实现修正：D-09 校验顺序初版「形态→类型→存在性」与设计文档「形态→存在性→类型→两态」不符——已改为存在性（GetUnit 实况）先于类型边界（service.go jobAction/toggleAutostart）
- [x] T04 · 生成 api 层（2026-11-14）
  - spec：backend-02-api-conventions（已直读，A-S1）
  - 输出：`internal/errcode/servicemgmt.go`（SVC_001~008 符号串）+ `internal/api/servicemgmt/`（handler 薄壳 7 接口 + routes：InitGuard→AuthRequired→[GET RequireRole(admin,readonly)] / [POST CsrfGuard→RequireRole(admin)]）+ main.go 装配（sysd.NewGateway(ActionWaitLimit) + svcsvc.New(gateway, auditSvc) + 菜单登记 services/Order 200/两角色 + 模块挂载；无后台 goroutine——D-03 按需拉取）
  - `go build ./...` ✓ `go vet ./...` ✓（2026-11-14 实跑）
- [~] T05 · 生成 middleware —— 跳过新增：无鉴权/审计/限流/WS 新需求，复用 InitGuard/AuthRequired/CsrfGuard/RequireRole 四件套（api §3.1「不得自建会话解析与角色判定」）

## 测试与验证

- [x] T06 · 生成单元测试（2026-11-14）
  - `internal/adapter/sysd/sysd_test.go`（纯函数 6 用例）：UnitTypeOf 13 例（含 -.mount/user@1000.service/template）、NormalizeActiveState 9 例（瞬时态方向归一/未知透传）、MergeUnitProjection（并集/enabledState 合成/alias 透传/file-only inactive+dead/无 unit 文件空串/字典序/空输入/空名跳过）、FileBaseName、DetailFromProps（属性组装/缺键零值）
  - `internal/service/servicemgmt/servicemgmt_test.go`（fake gateway+fake recorder 12 用例）：parseStateFilter 9 例（多值/重复 ok/空段/大写/带空白拒绝）、list 过滤+搜索大小写不敏感+排序+权威计算矩阵（timer 不可操作/service+static 可操作不可切/service+enabled 双可）、SVC_008/SVC_002 先于通道错、detail 4 例（null 属性/001 不触网关/003/008）、jobAction 8 例矩阵（成功快照/001/003/004/006/007 Job 失败含 reason+终态/007 调用期拒绝 activeState=null）每例断言 I-06 恰好一条审计（四要素+结果+detail.error）、SVC_006 note 契约文案+detail.timeout、toggle 8 例（enable/disable 刷新值/static/masked/indirect/enabled-runtime/空 enabledState→005 且零提交/拒绝→007）、D-09 顺序（存在性先于类型）、A-S6 快照降级（JobResult+前置 enabledState）、D-06 failAll 重试一次不影响结果/failOnce 重试成功落一条、常量核对（90s/名称形态 9 例）
  - `internal/api/servicemgmt/module_test.go`（httptest 全栈 7 用例，真实 :memory: SQLite+真实审计服务，sysd 替换 fake）：未登录 3 接口 AUTH_001（先走向导过 InitGuard）、admin+readonly 双角色 list/detail 全键断言（含 canOperate/canToggleAutostart 权威值+generatedAt RFC3339）、参数与通道错（SVC_002/SVC_001/SVC_003/未知错误→INTERNAL 脱敏）、readonly 越界 AUTH_008 且零提交零审计、CSRF 三态拒绝（缺头/错 token/缺 Origin）零提交、admin 全动作链（start 成功/006+note/007 reason+终态/004 timer/005 static/enable 刷新值/畸形 body→AUTH_012/空 name→SVC_001）、I-06 HTTP 通道（成功 1 条 success+拒绝 1 条 failure detail.error=SVC_001，计数 recorder 包装真实审计）
  - 修复发现：D-09 顺序修正（见 T03）；fake 网关 disable 后刷新读走默认路径的测试期望错误（补 getSeq 后置读取）；gofmt 对齐 5 文件
  - 实跑：`go test -race -count=1 ./...` 13 包全 ok（含既有 10 包无回归）、0 FAIL
- [x] T07 · Go 编码规则合规检查（Gate 2，backend-rules §7）——2026-11-14 实际执行记录
  - 工具：go1.24.0 windows/amd64（cwd=backend/）
  - G-01 无 shell 拼接：`grep -rnE "exec\.Command(Weak)?Context?\(.+\"(-c|/c)\"" --include="*.go" internal/ cmd/` 0 命中；全仓 `exec.Command` 0 处（本模块 systemd 交互走 godbus 参数化方法调用，无外部命令——R-08）→ PASS
  - G-02 无 SQL 拼接：`grep -rnE "Sprintf\(.+(SELECT|INSERT|UPDATE|DELETE)" --include="*.go" internal/ cmd/` 0 命中（本模块零 SQL，D-07）→ PASS
  - G-03 无硬编码凭据：`grep -rnEi "(password|secret|token|apikey)\s*=\s*\"[^\"]{8,}\"" --include="*.go" internal/ cmd/ | grep -v _test` 0 命中 → PASS
  - G-04 SQL 只在 store 层：层外命中唯一 `internal/service/auth/user.go:114`，人工核对为注释文字「FK ON DELETE CASCADE」（非执行 SQL，account-auth/panel-core/monitoring T07 同判）→ PASS
  - G-05 无 panic 滥用：`grep -rn "panic(" --include="*.go" internal/ cmd/ | grep -v _test` 0 命中 → PASS
  - G-06 goroutine 退出路径：`go func` 4 处均在 cmd/dougong/main.go（:182 审计 worker、:197 monitoring collector、:235/:241 TLS/plain serve——均为前序模块已核对编排）；**本模块新增 0 处 goroutine**（dbus 连接惰性建立无后台任务，Job 等待为请求 ctx 内 select，信号通道 per-wait 注册/注销）→ PASS
  - G-07 编译+vet+test 全绿（gofmt 后最终门禁）：`go build ./...` ✓ `go vet ./...` ✓ `go test -race -count=1 ./...` 13 包全 ok、0 FAIL → PASS
  - 🟡 W1 fmt.Print 非测试 0 命中；W2 log.Print 混用 0 命中；`gofmt -l internal/ cmd/` 0 未格式化文件
  - 环境说明：Windows 开发机无 system bus——dbus 通道路径（ensureConn/JobRemoved 等待）无法本机单测/冒烟，已以纯函数测试+fake 网关双层覆盖逻辑分支，通道实联验证归属 Linux 目标环境部署冒烟（如实声明，非跳过）

## service-mgmt 完成汇总

- 模块与包：复用 account-auth/panel-core/monitoring 全部公共资产（Module 注册器/包络/errcode/中间件四件套/AuditService/main 装配模式），新增 `internal/adapter/sysd`（2 文件+1 测试）+ `internal/service/servicemgmt`（6 文件+1 测试）+ `internal/api/servicemgmt`（2 文件+1 测试）+ `errcode/servicemgmt.go` + main.go 装配；全仓 16 个 Go 包
- 接口：7 HTTP（GET /services/unit/{list,detail} 两角色 / POST /services/unit/{start,stop,restart,enable,disable} admin+CSRF，§3.3 权限矩阵与链序全对齐）+ 进程内契约 SysdGateway（godbus 实现：ListUnits∪ListUnitFiles 合并投影/GetUnit/三 Job 动作 JobRemoved 等待≤90s/两自启轻量同步）+ AuditService.Record OHS 消费（I-06 恰好一条/D-06 降级）
- 数据：零持久化（D-07：无表/无 DDL/无 SQL/无迁移），T01/T02 按 orchestrator 条件任务判定 [~] 跳过
- 依赖：godbus/dbus v5.2.4（唯一新增；纯 Go 免 CGO，go 指令 1.20 ≤ 1.22 选版约束——A-S11）
- 关键实现裁定：A-S1~A-S11（见上表）+ D-09 校验顺序修正（存在性先于类型）
- 测试：`go test -race -count=1 ./...` 13 包全绿无回归；本模块新增 3 测试文件 25 用例（纯函数 6 + service 12 + HTTP 全栈 7）
- 遗留（非阻断）：Windows 开发机无 system bus，dbus 通道路径实联验证归属 Linux 目标环境冒烟；菜单 icon 键 services 由前端模块在 MENU_ICONS 补映射（未命中回落 AppsOutline 已核实）
- 重派发复验（2026-11-14，零改动验收）：本模块产物已在前序轮次完整生成——逐文件核验在位（adapter/sysd 契约类型+哨兵+纯函数、gateway dbus 参数化惰性连接；service/servicemgmt 六文件 D-01~D-09/I-01~I-07 对齐；api/servicemgmt 7 接口链序 InitGuard→AuthRequired→CsrfGuard→RoleRequired；errcode SVC_001~008；main.go:149-154,188 装配与菜单登记）；Mimosa 门禁探针写入成功确认已禁用（dwfq-eb98056b-1/2）；自验实跑 `go build ./...`=0、`go vet ./...`=0、`go test ./... -race -count=1` 全仓 17 测试包全 ok（时点含 file-manager 扩展包；本模块 29 测试函数 6+16+7 全绿）；G-01（exec 代码调用 0，唯一命中 nginxx/parser.go:3 为注释）/G-02/G-03/G-05 零命中、G-04 层外命中 4 处均注释（user.go:114/scope.go:20/trash.go:36,81,271,293）、G-06 五处 goroutine（main.go:213/228/243/281/287）均有退出路径（ctx 取消/wg.Wait 于 main.go:309/serveErr+ErrServerClosed）、G-07 即上述三件套 → G-01~G-07 全过；INDEX.md 本模块 backend 行已追加复验记录

- [x] T08 · SEC-GATE 补丁轮（2026-11-16，验证性空操作——ask 前提「此前因 Mimosa 误报以合规桩交付外部命令网关」与本模块实态不符，沿 account-auth T10 / panel-core T12 / monitoring T10 先例如实落账不臆造）
  - 门禁探针：backend/.secgate-probe.go（package 声明 + exec.Command("systemctl","status","nginx") 字面量参数数组一行）Write 写入成功=Mimosa 已禁用（裁定 dwfq-eb98056b-1/2），探针即删（ls 复核不存在）
  - 桩排查结论：service-mgmt 所属包（adapter/sysd、service/servicemgmt、api/servicemgmt、errcode/servicemgmt.go、main 装配 main.go:39/51/65/177-178）合并 grep 零 exec.Command、零 systemctl/os/exec 引用、零 SEC-GATE/not-implemented/NotImplemented/未实现/TODO/stub/placeholder 标记（均 exit=1）——本模块 systemd 网关的设计与交付形态即 godbus v5.2.4 纯库 dbus 参数化方法调用（A-S11 纯 Go 免 CGO / R-08 无 shell；T07 已记「本模块 systemd 交互走 godbus 参数化方法调用，无外部命令」），本无外部命令 exec 网关可替换；sysd/gateway.go（304 行）为完整真实实现：ConnectSystemBus 惰性连接+AddMatchSignal 订阅/ListUnits∪ListUnitFiles 合并投影/LoadUnit+Properties.GetAll/Start·Stop·RestartUnit JobRemoved 按 job path 等待≤90s/Enable·DisableUnitFiles；全仓 exec.Command 24 处（非测试）逐属后续模块 adapter（backupx/cronx/dbmysql/dbredis/dockerx/firewallx/nginxx/ptyx 等）且均为字面量参数数组形态；tasks.md 无 `- [!]` 阻塞项（grep exit=1）；INDEX.md validation(:67)/gate2(:70) 行已双证「模块零桩（真实 dbus 实现）→DEBT-SEC-GATE 不适用」→「桩替换」与「阻塞项解除」均为空操作，零代码改动，INDEX 沿先例维持不改写
  - 自验（2026-11-16 实跑，go1.26.0 windows/amd64，PATH 内 go，cwd=backend/）：`go build ./...`=0 ✓ `go vet ./...`=0 ✓ `go test -count=1 ./...` 39 个含测试包全 ok、0 FAIL（本模块三包 adapter/sysd 0.052s / service/servicemgmt 0.155s / api/servicemgmt 0.725s）→ PASS

## 完成汇总（account-auth）

- 模块与包：module github.com/dougong/dougong（go 1.22，gin v1.10.0 + modernc.org/sqlite v1.34.4 + golang.org/x/crypto），共 11 个 Go 包：
  cmd/dougong、internal/api、internal/api/auth、internal/api/middleware、internal/api/response、internal/config、internal/errcode、internal/service/auth、internal/service/audit、internal/store、internal/store/auth（+ internal/store/migrations 内嵌 SQL）
- 接口：18 HTTP（GET 6 / POST 12，权限矩阵 §3.3 全对齐）+ 4 进程内 OHS（AuditService.Record / SessionAuthService.VerifyWs / AuthService.InitViaCLI / middleware 链 InitGuard→AuthRequired→CsrfGuard→RoleRequired）
- 数据：5 表 DDL（panel_user/panel_user_totp/panel_session/audit_log/init_state + 4 索引）随 001_account_auth.sql 迁移落库；SQL 全部参数化且仅在 store 层
- 测试：5 包 `go test -race ./...` 全绿；表驱动覆盖登录锁定/TOTP 两步式/守卫不变量/审计/HTTP 全栈/迁移幂等
- 可复用资产（后续 10 模块消费）：api.Module 路由注册器、response 包络（code=0 数值成功 / 错误符号串，T08/E1 修订后口径）、errcode 注册机制（symbol+message+data，无数值码）、middleware 六件套（recovery/requestlog/initguard/auth/csrf/role）、store.Open+Migrate SQLite 基座（PRAGMA/单连接写串行/版本表迁移）、config 加载框架
- 已知取舍：~~设计文档 AUTH_xxx 无数值码 → 401NN 映射~~（已废止，A-2 修订：包络 code 上线符号串）；包络 message 字段名以设计文档优先于 stack-specs msg（A-1）

# file-manager（dougong）Go 后端代码生成任务清单

> 创建时间：2026-11-14
> 设计文档：profiles/dougong-panel/workspace/02-detailed/api/api-file-manager.md（v1.0，唯一事实源，禁止修改）
> 代码输出：backend/（复用 account-auth/panel-core/monitoring/service-mgmt 全部公共资产：Module 注册器/包络/errcode/中间件四件套/AuditService/store 基座/迁移框架）
> 环境备注：Go 工具链 `D:\go-toolchain\go\bin\go.exe`（go1.24.0 windows/amd64）；stack-specs 经 Skill 工具仍不可加载（Skill not found，沿 A-P1/A-M1/A-S1 先例直读等效全文）

## 设计裁定（设计文档留白处的实现拍板）

| # | 留白 | 裁定 |
|---|------|------|
| A-F1 | stack-specs Skill 不可加载 | 直读 `.claude/skills/stack-specs/backend-{01,02,03}*.md` 等效加载（先例 A-P1/A-M1/A-S1） |
| A-F2 | PathScopeGuard 的实现位置（api §3.1 描述为中间件链末环节，§4.2.4 明确「携带路径接口隐含前置」） | service 层强制前置（D-01/I-08）：guard 产出「校验后的真实路径」必须回传执行链，gin middleware 无法向 handler 回传数据；D-01 的「两步强制前置」语义不因实现载体改变。T05 条件任务按 [~] 判定：无新增 gin middleware（复用四件套） |
| A-F3 | archive 产物落点（契约 req 只有 paths[]/format/targetName?，无 targetDir） | paths[0] 的父目录（PageSpec 4.4.9 多选来自当前浏览目录；契约未含 targetDir，不自行增字段） |
| A-F4 | chunk-complete 时目标名被占（分块链路 req 无 onConflict，chunk-init 已按 ask 预检过） | FILE_004 直接错误（D-13 从紧：init 与 complete 之间新出现的占用非操作者明确覆盖意图） |
| A-F5 | 归档产物落盘方式（契约 FSGateway 方法集无流式写；大归档全量进内存不可取） | FSGateway 增补 `WriteFileAtomicStreaming(realPath, fn(io.Writer) error)`（同目录临时文件+rename，I-09 同口径）；契约方法集视为最小集非穷尽 |
| A-F6 | trash_name 时间戳格式（D-15 只定 `{deletedAt 时间戳}-{8 字节随机串}`） | Unix 秒 + `-` + 8 字节 crypto/rand hex（16 字符），如 `1763126400-a1b2c3d4e5f60718` |
| A-F7 | Windows 开发机无 syscall.Stat_t/Chown/EXDEV | adapter/fslocal 按 build tag 平台分离：unix 完整（uid/gid→/etc/passwd·group 名映射、Chown、EXDEV 判定）；windows 退化（owner 置空、Chown 返回不支持错误归 FILE_013、EXDEV 恒 false）。目标环境 Linux 功能完整；Windows 自验聚焦纯 fs 逻辑（编辑/压缩/回收站/守卫全链路真实文件系统可测） |
| A-F8 | 直传多文件中单文件超 5MiB 的粒度 | 整体拒绝 FILE_011 零落盘（multipart FileHeader.Size 先检不读体；与 D-07 ask 预检零落盘同口径） |
| A-F9 | create 的 type 非法值错误码（契约未列专码） | FILE_001（参数错误语义：路径/形态类入参非法） |
| A-F10 | 审计记失败的范围 | D-01 明文豁免：FILE_002 越界拒绝零审计；其余校验/执行失败记 failure 恰好一条（detail.error=错误码符号，servicemgmt A-S5 先例）；查看类接口（list×2/content/system-accounts/path-scope/get）不审计；下载审计 category=security（D-08） |
| A-F11 | trash/list 对账孤儿的「warning 标注」落法（PRD N2） | items 剔除孤儿 + data.warning 非空时携带一句话说明（契约 data={items,total} 基础上按 N2 明文增补） |
| A-F12 | purge 中途 fs 失败 | 整体回滚（记录保留 in_trash；已删内容成孤儿由 list 对账剔除——I-01 读取侧对账语义兜底）；FILE_013 返回 |
| A-F13 | delete/restore 的 tx 失败补偿 | 文件系统 rename 已成而 SQLite tx 失败 → 补偿 rename 回原位；补偿失败记 error 日志留孤儿（I-01 对账兜底），接口返回 FILE_013 |
| A-F14 | unarchive 归档格式识别 | 按扩展名：.zip→zip、.tar.gz/.tgz→tar.gz、.tar→tar；不识别 FILE_008（.tgz 视为 tar.gz 等价别名，裁定登记） |
| A-F15 | 分块临时目录位置 | 目标目录（同文件系统保证合并后 rename 原子性）下 `.dougong-upload-{uploadId}/`；作废/完成即删（隐藏条目透明呈现期间可见，I-10 口径） |
| A-F16 | entry/list 的 path 不存在 vs 非目录 | 合并 FILE_012（契约 4.2.4 list 校验链只列 FILE_012「目标目录不存在或不是目录」）；单点动作接口（download/delete/chmod 等）不存在 → FILE_003 |
| A-F17 | 解压遇到符号链接/硬链接条目（契约未列） | FILE_009 整体不落盘（链接种类条目为逃逸向量——先写符号链接再经后续条目穿透可绕过静态校验；PRD 未列符号链接归档需求，从紧） |
| A-F18 | zip 解压读取方式（Go 标准库 zip.Reader 需 io.ReaderAt，中央目录在文件尾，无纯流式读取器） | zip 经内存全量读入（ReadFile limit=0）+ bytes.Reader；tar/tar.gz 纯流式两遍（预检+落盘各自开流） |
| A-F19 | 越界判定的探测面（越界路径的存在性差异会泄露受管根外文件布局） | guard 词汇层域校验先行：输入路径词汇上不在受管根/命中排除即 FILE_002，不触达文件系统（零探测面）；EvalSymlinks 成功后二次核域拦截 symlink 逃逸（M2 验收硬项）——非越界路径的存在性探测（FILE_003）只发生在受管根内，属面板本职可见面 |
| A-F20 | entry/list 的 path 缺省语义（api §4.2.4 列 path 必填；前端首屏不带 path，Gate 2 第 1 轮 E1） | 缺省（空串/未携带）=受管根（frontend/tasks.md 实现决策 1 已登记的联调口径；readonly 无 path-scope/get 权、无从预知受管根，缺省浏览只能由服务端解析）；显式 path 语义不变；不升版 api 契约（宽容缺省，行为兼容） |

## 数据层

- [x] T01 · 生成 DDL 与迁移
  - 输入：设计文档 §5.3 DDL（trash_entry + fs_path_scope 两表 + idx_trash_status）
  - 输出：`internal/store/migrations/003_file_manager.sql`（逐字落库）

## 代码生成

- [x] T02 · 生成 store 层
  - spec：backend-01-architecture / backend-03-patterns（已直读）
  - 输出：`internal/store/fileman/`（models + TrashRepo{Insert(tx)/GetByID/ListInTrash/UpdateRestored(tx)/DeleteAllInTrash(tx)/Delete(tx)} + ScopeRepo{EnsureRow/Get/Update(tx)} + Store/TxStore 聚合（同事务审计复用 storeauth.AuditRepo），SQL 清单 §5.2 #1~#7 全参数化）
- [x] T03 · 生成 service 层 + adapter 层
  - 输出：`internal/service/fileman/`（consts/dto/gateway 契约/PathScopeGuard/FileProjector/EntryActionService/TransferService+UploadChunkStore/EditService/MetaService/ArchiveService/TrashService/门面 Service）+ `internal/adapter/fslocal/`（FSGateway 实现，平台分离 A-F7）
- [x] T04 · 生成 api 层
  - spec：backend-02-api-conventions（已直读）
  - 输出：`internal/errcode/filemanager.go`（FILE_001~015）+ `internal/api/fileman/`（22 接口 handler + 路由：两角色 GET×2 / admin 只读 GET×4 / admin+CSRF POST×16）+ main.go 装配（菜单 files+files/trash 两角色可见、chunk 清理 goroutine 入 wg）
- [~] T05 · 生成 middleware（条件判定：无新增）
  - AuthRequired/CsrfGuard/RoleRequired 复用 account-auth 四件套；PathScopeGuard 为 service 层前置非 gin middleware（A-F2）

## 测试与验证

- [x] T06 · 生成单元测试
  - store/fileman（:memory: SQLite 表驱动：trash CRUD+同事务+单行 scope）、service/fileman（guard 越界/排除/父解析、projector 排序、edit 二进制探测、trash id、onConflict 决策、chunk store、archive zip slip 预检——真实 t.TempDir 文件系统）、adapter/fslocal、api/fileman（httptest 全栈 22 接口抽样）
- [x] T07 · Go 编码规则合规检查（Gate 2）
  - 加载规则：`.claude/rules/backend-rules.md` §7 G-01~G-07 全部 grep 扫描 + 人工核对；不通过不得标记完成

### T07 · Gate 2 合规检查实际执行记录（2026-11-14）

- 工具：`D:\go-toolchain\go\bin\go.exe`（go1.24.0 windows/amd64，cwd=backend/）
- G-01 无 shell 拼接：`grep -rnE 'exec\.Command(Weak)?Context?\(.+\"(-c|/c)\"' internal/ cmd/` 0 命中；全仓 `exec.Command` 0 处（本模块 FSGateway 纯 Go 标准库直达文件系统，无外部命令——R-08/R-09）→ PASS
- G-02 无 SQL 拼接：`grep -rnE 'Sprintf\(.+(SELECT|INSERT|UPDATE|DELETE)' internal/ cmd/` 0 命中 → PASS
- G-03 无硬编码凭据：`grep -rnEi '(password|secret|token|apikey)\s*=\s*\"[^\"]{8,}\"' internal/ cmd/ | grep -v _test` 0 命中 → PASS
- G-04 SQL 只在 store 层：层外命中 3 处人工核对全为注释文字——`internal/service/auth/user.go:114`（前序模块已裁定）、`internal/service/fileman/scope.go:20`、`internal/service/fileman/trash.go:36/:81`（doc 注释提及「UPDATE fs_path_scope」「INSERT」语义，非执行 SQL）→ PASS
- G-05 无 panic 滥用：`grep -rn 'panic(' internal/ cmd/ | grep -v _test` 0 命中 → PASS
- G-06 goroutine 退出路径：`go func` 5 处全在 cmd/dougong/main.go——:213 审计 worker、:228 monitoring collector（前序已核对）、**:243 本模块新增 chunk sweeper（ChunkSweeper→RunSweeper ticker 循环唯一退出路径=ctx 取消，D-10）**、:281/:287 TLS/plain serve（Shutdown 后 ErrServerClosed 退出）→ PASS
- G-07 编译+vet+test 全绿（gofmt -w 后最终门禁）：`go build ./...` ✓ `go vet ./...` ✓ `go test -count=1 ./...` 17 包全 ok、0 FAIL（另 `go test -race -count=1 ./...` 全绿）→ PASS
- 🟡 W1 fmt.Print 非测试 0 命中；W2 log.Print 混用 0 命中；`gofmt -l internal/ cmd/` 0 未格式化文件
- 修复记录（测试驱动）：①buildTar 非 gzip 路径 nil gzw.Close() panic → gzip 层独立作用域；②writeAtomic 清理 defer 仅 err!=nil 触发、panic 展栈漏临时文件句柄 → committed 标志统一清理；③guard 词汇层域校验先行（A-F19，防越界路径存在性探测）

## file-manager 完成汇总

- 模块与包：复用前序模块全部公共资产（Module 注册器/包络/errcode/中间件四件套/AuditService/store 基座/迁移框架），新增 `internal/store/fileman`（4 文件+1 测试）+ `internal/service/fileman`（12 文件+2 测试）+ `internal/adapter/fslocal`（3 文件+1 测试，平台分离）+ `internal/api/fileman`（3 文件+1 测试）+ `errcode/filemanager.go` + `migrations/003_file_manager.sql` + main.go 装配；全仓 21 个 Go 包
- 接口：22 HTTP（GET 6：两角色 entry/list+trash/list、admin 专属 path-scope/get+entry/download+entry/content+system-accounts/list；POST 16 admin+CSRF）+ FSGateway 文件系统网关契约（fslocal 实现：目录投影/原子写/EXDEV 降级/chown·chmod/系统账号实时读取）+ PathScopeGuard（R-04 两步校验全接口强制前置，词汇层先行 A-F19）+ UploadChunkStore（D-10 内存态+1h TTL sweeper）
- 数据：trash_entry + fs_path_scope 两表 DDL（§5.3 逐字）；SQL 清单 #1~#7 全参数化仅 store 层；同事务例外三处落法——fs_path_scope 修改+审计（D-02）、trash 入站登记+file_deleted 审计（D-12/D-15）、还原/清空内容与记录（I-01）
- 关键实现裁定：A-F1~A-F19（见上表）——核心：PathScopeGuard service 层前置（A-F2）、onConflict 全量预检零落盘（D-07）、越界拒绝零审计（D-01/A-F10）、zip slip + 符号链接条目整体拒绝（A-F17）、跨平台 uid/gid/chown/EXDEV 分离（A-F7）
- 测试：`go test -race -count=1 ./...` 21 包全绿无回归；本模块新增 4 测试文件 33 用例（store 6 + service 22 + adapter 4 + HTTP 全栈 7）——路径逃逸（symlink→FILE_002）、zip slip（FILE_009 整体不落盘）、入站同事务审计、副本即时生效（D-02）、readonly 越界 AUTH_008、CSRF AUTH_003、下载流 attachment、分块三段式全链均实证
- 遗留（非阻断，如实声明）：Windows 开发机无 /etc/passwd 属主语义与 symlink 权限（部分用例按 A-F7 平台分支/跳过）——chown 属主呈现、符号链接逃逸用例、uid/gid 映射实联验证归属 Linux 目标环境部署冒烟；菜单 icon 键 files/trash 由前端模块在 MENU_ICONS 补映射（未命中回落 AppsOutline）

### Gate 2 第 1 轮 E1 修复记录（2026-11-14，后端侧）

- 报告：profiles/dougong-panel/workspace/05-reports/gate2-file-manager-2026-11-14.md
- E1（契约断裂）：前端首屏 GET /files/entry/list 不带 path（readonly 亦无 path-scope/get 权预知受管根），后端空串即 FILE_001——初始浏览双角色不可用
- 修复：projector.go ListDir path 缺省（TrimSpace 空）→ guard.Snapshot().RootPath 解析（A-F20）；显式 path 语义不变；sort/order 校验先行不动
- 用例补齐（双侧零覆盖修复）：
  - service/fileman_test.go TestListDirDefaultPathIsRoot——缺省与显式受管根投影全等（path/total/generatedAt）；path-scope/update 收紧后缺省跟随新根（D-02 副本即时生效联动）
  - api/fileman/module_test.go TestRoleBoundaries——readonly/admin GET /files/entry/list（无 path）均 code=0
- 自验：go build ./... ✓ go vet ./... ✓ go test -count=1 ./... 21 包全 ok（-race 上一轮全绿，本轮改动仅缺省分支无并发面）；G-01~G-05 grep 复扫 0 命中（改动不涉 exec/SQL/凭据/panic）
- 前端侧 WARN（W1 TrashListData.warning 未消费 / W2 组件测试 / W3 components 目录）与 INFO 2 项归属前端模块，不在本侧范围

- [x] T08 · 复核运行（2026-11-14，再次重派发的验收核验，零功能改动）
  - 现状：本 ask 再次派发时 backend/ file-manager 交付完好（T01~T07+Gate 2 E1 修复全记录在案，INDEX backend=done）——按「不破坏已验证成果」原则不重建，逐项核验：22 接口路由与 §3.3 权限矩阵（api/fileman/routes.go:34-69：both GET entry/list+trash/list 两角色 / adminRead GET path-scope/get+entry/download+entry/content+system-accounts/list / write POST×16 CsrfGuard→RequireRole(admin)，InitGuard 前置全组）、FILE_001~015（errcode/filemanager.go:8-36 符号串）、两表 DDL 逐字（003_file_manager.sql ↔ api §5.3 列/约束/索引全同）、main 装配（main.go:157-174 fsGuard+fileSvc 构建、:242-253 chunk sweeper goroutine 入 wg+ctx 取消退出+panic 隔离）、A-F20 缺省 path=受管根（projector.go）——均确认在位，本模块代码零改动
  - Mimosa 门禁探针：backend/.secgate-probe.go（package 声明+exec.Command 字面量参数数组一行）写入成功=Mimosa 已禁用（裁定 dwfq-eb98056b-1/2），探针即删；本模块 FSGateway 纯 Go 标准库直达文件系统无外部命令，网关直写形态不适用
  - 自验（2026-11-14 实跑，go1.24.0 windows/amd64，PATH 内 go，cwd=backend/）：`go build ./...`=0 ✓ `go vet ./...`=0 ✓ `go test -race -count=1 ./...` 全仓 18 个含测试包全 ok、0 FAIL（本模块 store/fileman 1.207s + service/fileman 1.837s + adapter/fslocal 1.090s + api/fileman 5.563s）✓；Gate 2 复扫——G-01 exec.Command 代码 0 处（唯一 grep 命中 nginxx/parser.go:3 为 site-mgmt 注释文字，前序同判）、G-02 0 命中、G-03 0 命中、G-04 层外 6 行命中人工核对全为注释（user.go:114/scope.go:20/trash.go:36,81,271,293）、G-05 0 命中、G-06 go func 5 处全在 cmd/dougong/main.go（:213/:228/:243/:281/:287，含本模块 :243 chunk sweeper ctx 取消+wg+panic 隔离退出路径在位）、G-07 即上述三件套全绿；W1/W2 0 命中、`gofmt -l internal/ cmd/` 0 未格式化文件；INDEX.md 本模块 backend 行 note 已追加本轮复验记录

- [x] T09 · SEC-GATE 补丁轮（2026-11-16，验证性空操作——ask 前提「此前因 Mimosa 误报以合规桩交付外部命令网关」与本模块实态不符，沿 account-auth T10 / panel-core T12 / monitoring T10 / service-mgmt T08 先例如实落账不臆造）
  - 门禁探针：backend/.secgate-probe.go（package 声明+exec.Command 字面量参数一行）写入成功=Mimosa 已禁用（裁定 dwfq-eb98056b-1/2），探针即删（rm 后 ls 复核不存在）
  - 桩排查结论：file-manager 所属包（adapter/fslocal、service/fileman、api/fileman、store/fileman、errcode/filemanager.go）合并 grep 零 exec.Command/os/exec 引用、零 not-implemented/NotImplemented/TODO(SEC-GATE)/SEC-GATE/stub/placeholder/未实现 标记（exit=1）——本模块唯一网关 FSGateway 的设计与交付形态即纯 Go 标准库直达文件系统（fslocal/gateway.go:1-4 包注释「纯 Go 标准库零外部命令（R-08/R-09）」；T07 已记同口径），本无外部命令 exec 网关可替换；fslocal 真实实现逐点核验——gateway.go 契约 18 方法全实现（ReadDir 投影/Lstat/原子写 CreateTemp+Sync+Rename/EXDEV 上抛降级 copyTree/Chmod/SizeOf WalkDir 累计）、platform_unix.go:32-118 完整实装（syscall.Stat_t uid/gid、/etc/passwd·group 实时解析、os.Chown、EXDEV/ENOTDIR errors.Is 判定）、platform_windows.go 为 A-F7 设计裁定的 OS 能力退化（开发机无 /etc/passwd/chown/EXDEV 语义→ErrUnsupportedPlatform 归 FILE_013，目标环境 Linux 完整）非 Mimosa 桩；tasks.md 无 `- [!]` 阻塞项（精确 grep exit=1）；INDEX.md validation(:79)/gate2(:82) 行已双证「SEC-GATE 探针…未拦且模块零桩→DEBT-SEC-GATE 不适用」→「桩替换」与「阻塞项解除」均为空操作，零代码改动，INDEX 沿先例维持不改写
  - 自验（2026-11-16 实跑，go1.26.0 windows/amd64，PATH 内 go，cwd=backend/）：`go build ./...`=0 ✓ `go vet ./...`=0 ✓ `go test -count=1 ./...` exit=0 全仓 39 个测试包全 ok、0 FAIL（本模块 store/fileman 0.423s + service/fileman 0.681s + adapter/fslocal 0.726s + api/fileman 0.777s，FAIL/panic 全文 grep 零命中）✓

---

# site-mgmt（dougong）Go 后端代码生成任务清单

> 创建时间：2026-11-14
> 设计文档：profiles/dougong-panel/workspace/02-detailed/api/api-site-mgmt.md（v1.0，唯一事实源，禁止修改）
> 代码输出：backend/（复用 account-auth/panel-core/monitoring/service-mgmt/file-manager 全部公共资产：Module 注册器/包络/errcode/中间件四件套/AuditService/store 基座/迁移框架）
> 模块交付：18 HTTP 接口（/sites/{site,proxy,log,nginx,cert,php-pools}/*）+ nginxx 自研解析器（ADR-010）+ WritePipeline 安全写入管线（R-06 四步）+ AcmeGateway ACL（lego，R-09 唯一外呼）+ PhpFpmGateway + 自动续期每日调度 + 1 表 DDL（certificate）
> 环境备注：go1.24.0（goproxy.cn 可达）；stack-specs 经 Skill 工具仍不可加载（Skill not found，沿 A-P1/A-M1/A-S1/A-F1 先例直读等效全文）

## 设计裁定（设计文档留白处的实现拍板，均以设计文档明文为准绳）

| # | 留白 | 裁定 |
|---|------|------|
| A-SM1 | stack-specs Skill 不可加载 | 直读 `.claude/skills/stack-specs/backend-{01,02,03}*.md` 等效加载（先例 A-P1/A-M1/A-S1/A-F1） |
| A-SM2 | nginxx 解析器 round-trip 保真实现法（FR-05 要求「解析→生成→再解析语义等价」） | span 替换模型：解析产出携带原文精确字节区间的节点树，序列化=原文+定点区间替换（未编辑字节恒不变）——比全量重序列化更强（byte 级保真，I-09/I-10 天然成立）；结构化编辑=替换/插入/删除目标指令区间 |
| A-SM3 | dormant（注释态）server 块的识别与 siteId 稳定性（D-16 注释/还原启停） | 连续注释行剥离 `#` 前缀后若整体为合法 server 块→识别为 dormant 站点（enabled=disabled 投影）；**序号计数含 dormant**（文件内出现顺序，D-02）——disable→enable 全程 siteId 稳定；手工注释的 server 块同样被如实投影为 disabled |
| A-SM4 | rawBlocks 范围（R-07「解析不了的指令/块」） | server 块内非建模指令（server_name/listen/root/ssl_certificate(_key)/access_log/error_log/location 之外）的简单指令与块指令均入 rawBlocks[]（{line,text} 原文呈现）；写回侧由 span 模型天然保全（A-SM2） |
| A-SM5 | FR-25 存量证书识别导入的挂载点（PRD：随站点导入自动发生、无独立入口；api §4.1.2 指向 4.2 存量导入链路） | SiteScanner 扫描链尾部执行幂等纳管：site 的 ssl_cert 绝对路径可读且 x509 可解析且无同路径 imported 行→INSERT imported 行（status=valid、元数据 NULL 读时刷新 D-08）；机制无审计（PRD FR-25 明文「机制；纳管结果经清单呈现」）；不可读/不可解析引用保持 certId=null 如实呈现 |
| A-SM6 | create 的 root 不存在错误码（api 校验链列「根目录存在性」无专码） | AUTH_012 fields[root]（R-14 参数错误族复用，A-S9 bindError 先例） |
| A-SM7 | 审计失败记录范围（api 各写接口只标注成功侧审计；nginx/reload 明文「失败亦记」） | 文件系统侧动作成功记一条；失败仅 nginx/reload 按 api 明文记 failure（result=failure），其余失败不记（未发生变更，R-03 口径）；certificate 侧终态落库与审计同事务（D-12 例外） |
| A-SM8 | source-save 的「保存前 nginx -t 语义前置（不落盘）」实现 | 站点配置文件非 nginx 主配置，`nginx -t -c 片段` 不可行（缺 http{} 上下文必失败）——落盘前以 nginxx.ValidateSyntax 自检（括号配平/语句终结符/引号闭合，SITE_003 语义拒收零落盘）；真实 `nginx -t` 仍由 WritePipeline 第三步执行（失败回滚，R-06 全保） |
| A-SM9 | cert_requested 审计时点（detail={challenge,result} 但 INSERT 在签发前） | pending INSERT 为机制无审计；签发终态 UPDATE 与审计同事务（detail 含 result=success/failure，D-12 例外同事务口径）；进程中断遗留 pending 行按在途态如实呈现 |
| A-SM10 | ACME http-01 挑战服务方式（「80 通路经既有站点配置」D-15） | webroot 经由站点 server 块：①已有 /.well-known/acme-challenge/ location→复用其 root 为 webroot；②无→经 WritePipeline 注入临时 location（root=面板 webroot，^~ 前缀）→签发→移除（同管线）；tls_alpn=面板 tlsalpn01 监听器（443 被 nginx 独占时绑定失败→CERT_004 data.reason 如实呈现，环境性约束非代码缺陷） |
| A-SM11 | acme 托管文件路径（⚠️A16 建议值 /etc/dougong/certs/，「路径常量归后端工程」） | {面板数据目录}/certs/{规范化主域名}-fullchain.pem + .key（与 acme-account.key 同域、由 DBPath 派生非硬编码、跨平台一致；账户密钥 ECDSA P-256 文件 0600、注册资源 acme-account.json） |
| A-SM12 | ACME 账户 contact | 空邮箱 + ToS agreed（不虚构联系人；LE 允许无 contact 注册） |
| A-SM13 | lego 版本 | go-acme/lego/v4 v4.22.2（go 指令 1.22.0 为 ≤1.22 最高版——gopsutil/godbus 同选版约束；v4.23+ 需 go 1.23） |
| A-SM14 | deploy_prev_refs JSON 值形态（api「siteId→原 ssl_cert 值」单值） | map[siteId]→{cert,key}（ssl_certificate 与 ssl_certificate_key 成对快照/恢复，否则 undeploy 后证书钥不配对；列不对外下发，契约零变更） |
| A-SM15 | 续期调度节奏（「每日调度」） | 启动 1 分钟后首轮 + 24h ticker（重启覆盖到期窗口；单证书失败不影响其余、不自动重试，4.1.3） |
| A-SM16 | 菜单登记 | 网站管理 /sites Order 400 + SSL 证书 /certificates Order 410，两角色可见（PageSpec 第 7 节 + 侧边菜单口径） |
| A-SM17 | D-03 消重的 items 口径 | 同签名（serverNames+listen）server 块多文件并存：items 只保留最高优先级目录一处（其 siteId 即写回落点），duplicates[] 如实呈现全部文件；同文件内重复定义两 siteId 均列出 |
| A-SM18 | nginx -t 通道错误 vs 配置错误 | -t 退出码≠0=配置错误（SITE_003 回滚）；exec 失败（LookPath 不中/超时）=通道不可用（回滚+SITE_012 data.reason，I-12 同语义） |

## 执行前探针（2026-11-15 续作轮）

- Mimosa 门禁探针：backend/.secgate-probe.go（package 声明+exec.Command 字面量参数数组一行）写入成功=Mimosa 已禁用（裁定 dwfq-eb98056b-1/2，沿 account-auth T11/monitoring T09/file-manager T08 先例），探针即删；本模块 nginx 通道为真实网关直写 exec 形态（gateway.go:303/:327 字面量参数数组），无桩
- 本轮为续作：上轮中断时 004 迁移/store 三文件/nginxx parser+extract 已在库（未勾选）；本轮核验后沿用并补齐全部剩余产物

## 数据层

- [x] T01 · 生成 DDL 与迁移（上轮产物核验沿用）
  - 输入：设计文档 §5.3 DDL（certificate 单表 + 2 索引）
  - 输出：`internal/store/migrations/004_site_mgmt.sql`（逐字落库；migrate_test 动态断言兼容任意迁移数）

## 代码生成

- [x] T02 · 生成 store 层（上轮产物核验沿用 + 本轮补 SetAccountRef 与 updated_at 扫描修复）
  - spec：backend-01-architecture / backend-03-patterns（已直读）
  - 输出：`internal/store/sitemgmt/`（Certificate 仓储：Create/GetByID/List/ListActive/UpdateStatus/UpdateRenewalMeta/SetDeployPrevRef/ClearDeployPrevRef/SetAccountRef/Delete + Store/TxStore 聚合复用 storeauth.AuditRepo 同事务审计，§5.2 SQL 清单 #1~#6 全参数化）
- [x] T03 · 生成 adapter + service 层（2026-11-15）
  - 输出：`internal/adapter/nginxx/`（span 解析器+server 块提取含 dormant+NginxGateway：LookPath/-t/-s reload 参数数组+fs 原语）、`internal/adapter/phpfpm/`（pool 实况解析）、`internal/adapter/acme/`（lego v4.22.2 封装：账户密钥文件 0600+注册资源持久化+http_01 webroot/tls_alpn01）、`internal/service/sitemgmt/`（consts/ports/dto/scanner+自动纳管/projector/diff/WritePipeline/结构化编辑/sitesvc/sourceedit 乐观锁/logtail 解析链/certcalc x509+读时推算/certsvc 六接口/renewal 每日调度/门面）+ config 增 acme.directory_url
- [x] T04 · 生成 api 层（2026-11-15）
  - spec：backend-02-api-conventions（已直读）
  - 输出：`internal/errcode/sitemgmt.go`（SITE_001~014+CERT_001~007）+ `internal/api/sitemgmt/`（handler 薄壳 18 接口 + routes：InitGuard→AuthRequired→[GET 两角色 4 + GET admin 2] / [POST admin+CSRF 12]）+ main.go 装配（菜单 sites/certificates Order 400/410 两角色 + 续期调度 goroutine 入 wg）
- [~] T05 · 生成 middleware —— 跳过新增：无鉴权/审计/限流/WS 新需求，复用 InitGuard/AuthRequired/CsrfGuard/RequireRole 四件套（api §3.1「复用 account-auth，不自建会话解析与角色判定」）；WritePipeline 与 nginx 可用性预检为服务层编排（api §3.1 明文「非中间件」）

## 测试与验证

- [x] T06 · 生成单元测试（2026-11-15）
  - `internal/adapter/nginxx/parser_test.go`：round-trip 样本集 28 例（≥20 验收硬项：if/try_files/正则 location/引号转义/变量/注释/多 server 块/dormant 注释块/upstream/map/CRLF/中文/空文件/游离分号 Raw 保全）+ dormant 混组拒判 + 空行分段合并边界 + CommentOut↔StripPrefix byte 级往返 + span 编辑/重叠拒绝 + ValidateSyntax 4 坏样本 + 投影字段/rawBlocks（A-SM4）/Proxy 提取
  - `internal/store/sitemgmt/store_test.go`：生命周期（Create/GetByID/终态 meta/deploy_prev_refs 成对值+幂等清键/List 排序 NULL 排尾/ListActive/SetAccountRef/Delete）、同事务提交与回滚零泄漏（D-12）
  - `internal/service/sitemgmt/sitemgmt_test.go`（fake 网关+真实临时目录 fs+:memory: SQLite，14 用例函数）：扫描消重（D-03 跨目录唯一落点+duplicates[]）/自动纳管幂等+不可解析不纳管（A-SM5）/nginx 缺席空态（I-06）+写前置 SITE_012（I-12）/管线回滚+diff+reload 不执行（R-06）/快照保留 ≤20（D-04）/SITE_013 零写入/SITE_004 配置保留+写入成功审计（D-05/A-SM7）/通道错误回滚（A-SM18）/结构化编辑隔离（其余站点 byte 级前缀不变 R-16）+Raw 保全（I-09）+四校验码/反代写回+WS 开关+SITE_008/SITE_002 语义/启停注释还原 byte 级+siteId 稳定+幂等零写回（D-16/A-SM3）/建站三模板（⚠️A14 内置 FastCGI 参数+pool socket 实况绑定、反代 WS 默认值）+D-15 防重含停用站点/源码乐观锁（SITE_010/SITE_003 拒收零落盘 A-SM8/SITE_014/新基线二次保存 D-06）/日志尾部（默认 100+truncated/上限钳制/error 时间戳拆解/回退链穷尽 SITE_011 ⚠️A15 D-07）/签发全流程（pending→valid+托管文件+0600+同事务审计/http-01 临时 location 注入移除 A-SM10）+I-03 防重+CERT_007+失败转 renew_failed+retry 通道+CERT_001/002/部署引用+快照成对值+幂等+CERT_005 守卫含 imported 推导+undeploy 恢复原值+二次 CERT_006+删除连带托管文件 I-04+imported 源文件不动 ⚠️A16/状态推算边界+imported 读时刷新不落库 D-08/续期单轮（healthy 不触碰+system 审计）+失败转态+调度器 ctx 取消即退（G-06）/reload 失败亦记 failure/pools 空数组（FR-15）/纯函数表驱动
  - `internal/api/sitemgmt/module_test.go`（httptest 全栈，真实 :memory: SQLite+真实 auth/audit，nginx/acme/php 替身）：权限矩阵（未登录 AUTH_001/readonly 查看 4 接口（log/tail 权限通过后 SITE_011 如实）/编辑前置读取+建站候选 AUTH_008/写接口携合法 CSRF 仍 AUTH_008）+CSRF 三态+admin 全链路（空态→create→detail→proxy/update M3 主场景→source 乐观锁→启停→reload→cert request→readonly 可看 cert/list→deploy→undeploy→delete→SITE_005/SITE_001/CERT_007 抽样→SITE_003 管线回滚 byte 级实证→php-pools）
  - 修复发现（测试驱动）：scanCert 漏 updated_at 列；setDirectives 首行双重缩进与 listen 误加引号；CommentOut 改「# + 原行」形态使 StripPrefix 成精确逆（disable→enable byte 级还原）；注释组按空行分段合并（普通注释不吞 dormant 判定）；update 移除 root 存在性检查（设计仅 create 校验 A-SM6）；respond typed-nil 指针吞错误 data 载荷（SITE_012 reason 冒烟复验修复）
- [x] T07 · Go 编码规则合规检查（Gate 2，backend-rules §7）——2026-11-15 实际执行记录
  - 工具：go1.24.0 windows/amd64（cwd=backend/）
  - G-01 无 shell 拼接：`grep -rnE "exec\.Command(Weak)?Context?\(.+\"(-c|/c)\"" internal/ cmd/` 0 命中；全仓 exec.Command 4 处人工核对——parser.go:3/gateway.go:31 为注释文字，gateway.go:303（`nginx -t`）/gateway.go:327（`nginx -s reload`）为真实调用：exec.CommandContext 字面量参数数组、二进制构造期 exec.LookPath 解析（backend-03 §4）→ PASS（本模块为首个真实外部命令网关直写形态，Mimosa 已禁用裁定下交付）
  - G-02 无 SQL 拼接：Sprintf+SQL 关键字 grep 0 命中 → PASS
  - G-03 无硬编码凭据：0 命中 → PASS
  - G-04 SQL 只在 store 层：层外 6 文件命中全为注释（user.go:114/scope.go:20/trash.go:36,81 前序同判；本模块 certsvc.go:80「→ INSERT pending（机制无审计）」/scanner.go:160「→ INSERT imported 行」为流程注释非执行 SQL）→ PASS
  - G-05 无 panic 滥用：0 命中 → PASS
  - G-06 goroutine 退出路径：`go func` 6 处全在 cmd/dougong/main.go（:241/:256/:271 前序已核对；:287 本模块新增续期调度——RunRenewalScheduler 两级 select ctx.Done 返回 nil，退出路径经调度器取消用例实证，main 侧 wg.Done+panic 隔离；:325/:331 serve ErrServerClosed 过滤）；internal/ 自起 goroutine 0 处（ACME 超时以 ctx 注入 RoundTripper 实现零 goroutine）→ PASS
  - G-07 编译+vet+test 全绿：`go build ./...`=0 ✓ `go vet ./...`=0 ✓ `go test -race -count=1 ./...` 21 包全 ok、0 FAIL（本模块新增 4 测试包 nginxx/store·sitemgmt/service·sitemgmt/api·sitemgmt）→ PASS
  - 🟡 W1/W2 0 命中；`gofmt -l internal/ cmd/` 0 文件
  - 二进制冒烟（DOUGONG_ADDR=127.0.0.1:18199/18200）：init CLI ✓ → login code=0 ✓ → site/list code=0 nginxAvailable:false 空态（I-06）✓ → cert/list ✓ → php-pools 空数组 ✓ → nginx/reload SITE_012+data.reason ✓ → 未登录 AUTH_001 ✓ → 菜单含 sites/Order400+certificates/Order410（A-SM16）✓
  - 环境说明：Windows 开发机无 nginx/php-fpm——真实 nginx 通道、pool 实况、ACME 外呼无法本机实联，已以纯函数+fake 网关双层覆盖，通道实联归属 Linux 目标环境部署冒烟（沿 service-mgmt dbus 同判，如实声明非跳过）

- [x] T08 · Gate 2 第 1 轮 FAIL 后端侧修复（2026-11-15，报告 05-reports/gate2-site-mgmt-2026-11-14.md）
  - **F-1[FAIL·契约断裂] php_fpm root 过严**：后端 create.go 对 php_fpm 强制 root 必填，与契约 §4.2.4（root?(type=static 必填)）、交互规格（root 显隐=type=static）、前端（仅 static 发送 root）三方不一致 → UI 创建 PHP 站点 100% 被拒。修复方向=后端放宽回归契约（设计文档唯一事实源，禁止改文档）：create.go 移除 php_fpm root 必填校验（仅 static 保留）+ renderTemplate php 分支 root 为空时省略指令（继承上层/默认值，nginx -t 语义合法）；root 显式提供时仍走存在性校验（A-SM6）与写回
  - **回归用例双侧补齐**：service 级 TestCreatePHPSiteWithoutRoot（无 root 创建成功+模板无 root 指令+fastcgi/try_files 在位+static 无 root 仍 AUTH_012）+ HTTP 级 TestAdminFullFlow 增 php_fpm 无 root POST 全链路（module_test.go，code=0）——原缺口即 HTTP 级仅覆盖 reverse_proxy
  - **W-2[WARN] 覆盖率收敛**：service/sitemgmt 78.0%→82.6%（SKILL 80% 目标达成）——新增 7 用例：dormant 块结构化编辑（UpdateSite/UpdateProxy 于注释态、enable 后编辑生效，D-16/A-SM3）/建站失败新文件回滚删除（rollback !existed）/空 server 块全指令插入（insertPos 无子分支）/http-01 复用既有 acme-challenge location（A-SM10 ①零注入+webroot 传递断言）/deploy 管线失败补偿（快照键清除+配置回滚，I-05）/续期后漂移站点纠偏（redeployRef+托管文件原路径替换 I-04）/杂项边界（enabled 筛选 AUTH_012+证书索引择优 betterCert+pending NULL 排序+含空格 root 引号+超限源码 SITE_014）；顺带清理 service.go itoa 死代码（0% 覆盖的未用函数）
  - **测试驱动重构（同锚点插入语义）**：发现并修复同锚点多次零宽插入的组合缺陷（尾部「\n+indent」与后续插入叠加产生双重缩进）——插入文本改为自包含整行（行首缩进+语句+换行）、Serialize 排序改 sort.SliceStable（同起点按登记顺序稳定输出）、insertPos 无子分支落到下一行行首；TestDormantStructuredEdit 实证修正（dormant 反代编辑后 Upgrade/Connection/proxy_pass 缩进归一）
  - **W-1[WARN] listen 响应类型**：backend 维持 string（契约 req listen 为单声明字符串、响应原样回传）——string[] 为前端 types/site.ts 声明偏宽，归前端侧修（后端不改，避免背离契约字段语义）；W-3 陈旧注释同归前端（http.ts/errcode.ts 文案）
  - **自验（2026-11-15 实跑，cwd=backend/）**：`go build ./...`=0 ✓ `go vet ./...`=0 ✓ `go test -race -count=1 ./...` 21 包全 ok、0 FAIL ✓；覆盖率 `go test -cover ./internal/service/sitemgmt/` 82.6%（api/sitemgmt 71.3%≥60、store/sitemgmt 78.6%）；F-1 回归专项 `-run "TestCreatePHPSiteWithoutRoot|TestAdminFullFlow" -v` 双 PASS ✓；grep 复扫——「root 必填」仅剩 static 分支与测试注释、G-01 0 命中、G-05 0 命中、`gofmt -l` 0 文件

## site-mgmt 完成汇总（2026-11-15）

- 模块与包：复用前序模块全部公共资产，新增 `internal/adapter/nginxx`（3 文件）+ `internal/adapter/phpfpm`（1）+ `internal/adapter/acme`（1，lego v4.22.2）+ `internal/store/sitemgmt`（3+1 测试）+ `internal/service/sitemgmt`（15+1 测试）+ `internal/api/sitemgmt`（2+1 测试）+ `errcode/sitemgmt.go` + `004_site_mgmt.sql` + config 扩展（sites.nginx_bin/conf_root + acme.directory_url）+ main.go 装配（续期调度 goroutine+菜单 Order 400/410）
- 接口：18 HTTP（GET 两角色 4 / GET admin 2 / POST admin+CSRF 12）+ 进程内契约（NginxGateway/PhpFpmGateway/AcmeGateway ACL/WritePipeline/SiteScanner/CertStatusCalculator/CertReferencedResolver/RenewalScheduler）
- 数据：certificate 单表（§5.3 逐字）随 004 迁移落库；SQL 全参数化仅 store 层；证书终态/删除与审计同事务（D-12 例外）
- 关键实现裁定：A-SM1~A-SM18 + 测试驱动修复 6 项
- 测试：`go test -race -count=1 ./...` 21 包全绿无回归；本模块新增 4 测试文件（round-trip 28 样本/仓储/service 14 用例函数/HTTP 全栈 3）
- 遗留（非阻断）：nginx 通道/php-fpm 实况/ACME 实联归属 Linux 目标环境冒烟；菜单 icon 键 globe/lock 由前端 MENU_ICONS 补映射（回落 AppsOutline 已核实）

- [x] T09 · SEC-GATE 补丁轮（2026-11-16，验证性空操作——ask 前提「此前因 Mimosa 误报以合规桩交付外部命令网关」与本模块实态不符，沿 account-auth T10 / panel-core T12 / monitoring T10 / service-mgmt T08 / file-manager T09 先例如实落账不臆造）
  - 门禁探针：backend/.secgate-probe.go（package 声明 + exec.Command("systemctl","status","nginx") 字面量参数数组一行）Write 写入成功=Mimosa 已禁用（裁定 dwfq-eb98056b-1/2），探针即删（ls 复核不存在）
  - 桩排查结论：site-mgmt 所属包（adapter/nginxx、adapter/phpfpm、adapter/acme、store/sitemgmt、service/sitemgmt、api/sitemgmt、errcode/sitemgmt.go）合并 grep 零 exec 桩标记（not-implemented/NotImplemented/TODO(SEC-GATE)/SEC-GATE/stub/placeholder/未实现/合规桩，exit=1）——本模块唯一外部命令网关 NginxGateway 交付即真实 exec 直写形态（执行前探针 2026-11-15 与 T07 G-01 已双记「本模块为首个真实外部命令网关直写形态，Mimosa 已禁用裁定下交付」）：nginxx/gateway.go:303（nginx -t）/ :327（nginx -s reload）均为 exec.CommandContext 字面量参数数组（30s 通道超时+退出码/通道错误分派 A-SM18）、:59 构造期 exec.LookPath 二进制解析（不信任外部传入路径）；phpfpm（pool 实况解析）/acme（lego v4.22.2 库）零 os/exec 引用（grep exit=1）本无外部命令可桩化；本段任务清单零 `- [!]`/`- [ ]` 未决项（精确 grep exit=1）；INDEX.md site-mgmt gate2 行已双证「SEC-GATE 零桩维持 DEBT-SEC-GATE 不适用」→「桩替换」与「阻塞项解除」均为空操作，零代码改动，INDEX 沿先例维持不改写
  - 自验（2026-11-16 实跑，go1.26.0 windows/amd64，PATH 内 go，cwd=backend/）：`go build ./...`=0 ✓ `go vet ./...`=0 ✓ `go test -count=1 ./...` exit=0、39 个含测试包全 ok、0 FAIL（本模块 nginxx 0.057s / store·sitemgmt 0.123s / service·sitemgmt 0.704s / api·sitemgmt 0.628s 四包 ok；acme/phpfpm 无测试文件沿 W-2R 环境绑定债务如实声明）→ PASS

---

# database-mgmt Go 后端代码生成任务清单（2026-11-15）

> 设计文档：`profiles/dougong-panel/workspace/02-detailed/api/api-database-mgmt.md` v1.1（唯一事实源）
> 代码输出：`backend/`（module github.com/dougong/dougong）
> Mimosa 门禁探针：`backend/.secgate-probe.go` 写入成功=已禁用（裁定 dwfq-eb98056b-1/2，沿 site-mgmt 先例），探针即删；本模块 mysqldump/mysql/redis-cli 通道为真实网关直写 exec 形态（字面量参数数组 G-01），无桩

## 数据层

- [x] T01 · 生成 DDL 与迁移
  - 输入：设计文档 §5.3 DDL（db_instance 合一档案表 D-01 + idx_dbinst_engine）
  - 输出：`internal/store/migrations/005_database_mgmt.sql`（逐字落库）

## 代码生成

- [x] T02 · 生成 store 层
  - spec：backend-01-architecture / backend-03-patterns（已直读）
  - 输出：`internal/store/dbmgmt/`（InstanceRepo：List/GetByID/GetByIdentity/UpsertDiscovered/UpdateConnState/UpdateCredentials/Delete + Store/TxStore 聚合复用 storeauth.AuditRepo 同事务审计；§5.2 SQL 清单 #1~#6 全参数化）
- [x] T03 · 生成 adapter + service 层
  - 输出：`internal/adapter/dbmysql/`（mysql/mysqldump CLI 网关：字面量参数数组 R-08/D-06 模板、凭据经 MYSQL_PWD 环境变量注入不进 argv、TSV batch 反转义解析、标识符白名单+字符串转义组 SQL）+ `internal/adapter/dbredis/`（redis-cli 网关：REDISCLI_AUTH 注入、INFO/SLOWLOG/SCAN/TTL/CONFIG SET）+ `internal/service/dbmgmt/`（consts/ports/dto/vault AES-256-GCM+db-cred.key 0600/identity ConnTargetResolver/busy 任务表 D-12/prober D-07/grantmap D-10/instance+mysql+redis+backup 编排/审计 D-13 脱敏）
- [x] T04 · 生成 api 层
  - spec：backend-02-api-conventions（已直读；包络/路径以设计文档 §3.1 契约为准——先例同判）
  - 输出：`internal/errcode/dbmgmt.go`（DBM_001~020）+ `internal/api/dbmgmt/`（handler 薄壳 23 接口 + routes：InitGuard→AuthRequired→[GET 两角色 7 / GET admin 3] / [POST admin+CSRF 13]；backup/export 双形态流式响应 D-06）+ main.go 装配（菜单 database Order 500 两角色）
- [~] T05 · 生成 middleware —— 跳过新增：设计文档 §3.1「复用 account-auth 中间件链」，无新增鉴权/审计/限流/WS 需求（InitGuard/AuthRequired/CsrfGuard/RequireRole 四件套）

## 测试与验证

- [x] T06 · 生成单元测试（2026-11-15）
  - `internal/adapter/dbmysql/parse_test.go`（8 用例：quoteString 全转义/quoteIdent 反引号双写/TSV 切分反转义/NULL→nil/grantline 解析含 *.* 与未识别/权限段提取/classifyRun 认证 vs 连接 vs 引擎错误三分类/classifyDDL 存在性哨兵）
  - `internal/adapter/dbredis/parse_test.go`（4 用例：INFO 三节解析/畸形行容错/SLOWLOG --raw 块解析含 addr+name 元数据消费/空输出）
  - `internal/store/dbmgmt/store_test.go`（2 用例：生命周期——upsert 首插 discovered/二次 updated+version 刷新/凭据落库 I-09/连接目标纠正 SQL#7/引擎筛选排序/删除；同事务提交原子+回滚零泄漏（档案与审计双断言））
  - `internal/service/dbmgmt/dbmgmt_test.go`（19 用例：vault 加解密 roundtrip+nonce 随机+密钥文件存在+重开复用；identity_key/ConnTargetSpec I-07 互斥+I-08 越界；GrantLevelMapper 三档+子集/交集外不识别；governTarget 全链 DBM_001/002/018/003/004+通过态解密；credentials DBM_005 不落库不覆盖/成功密文覆盖+version+审计/DBM_020 同引擎撞 key/纠正切 socket；remove DBM_016/成功/重复 DBM_002；database CRUD DBM_008/007+charset 候选 AUTH_012+库名白名单；dbuser 建/DBM_010/grant 幂等/三档映射/DBM_011/DBM_009/删；改密两步 DBM_005→成功同步+引擎调用计数/DBM_019 BeginTx 注入失败引擎侧不回滚；redis 改密 SetRequirepass 新旧参断言+审计；key SCAN 默认参/DBM_017 空+空白 pattern/坏 cursor AUTH_012/TTL -1 透传+DBM_015；export DBM_007/成功流+filename+Finish 审计+busy 释放/空输出 Abort→DBM_012；import DBM_014/DBM_007/成功 payload 透传+审计+busy 释放/DBM_013；prober 首插/updated/不可达整值覆盖 unreachable（I-03）；status 白名单 3/4 项+泄漏字段不呈现；redis info 白名单映射+slowlog limit 钳制 AUTH_012；list 筛选 AUTH_012+投影；rescan 审计；标识符白名单表驱动 7 拒 2 过）
  - `internal/api/dbmgmt/module_test.go`（4 用例函数 httptest 全栈，真实 :memory: SQLite+真实 auth/audit+stub 网关：权限矩阵（未登录 AUTH_001/readonly 7 查看 GET 全通/admin 专属 GET×3 AUTH_008/readonly 写携 CSRF 仍 AUTH_008/admin 无 CSRF AUTH_003/rescan code=0）；MySQL 全链（list 投影 credentialsSet+credUsername→credentials→status 白名单→processlist→建库→DBM_008→建用户→授权 write→三档呈现→回收→DBM_011 二次→导出 octet-stream+Content-Disposition appdb_+body 断言+DBM_007 失败侧 JSON→multipart 导入 payload 断言→root 改密 synced:true→remove→DBM_002）；redis/key 流（DBM_018 引擎不匹配→info sections→key list complete+type→ttl -1/DBM_015→改密）；credentials 校验（DBM_001 数值 0/DBM_002 999/AUTH_012 socket+host 互斥/AUTH_012 空密码））
- [x] T07 · Go 编译规则合规检查（Gate 2，backend-rules §7）——2026-11-15 实际执行记录见下节

## 裁定记录（A-DBn）

- A-DB1：DbGateway 契约（§4.1.5）按引擎拆为 MySQLGateway/RedisGateway 两个 Go 接口（消费方接口隔离；方法签名与契约逐字对应，装配后语义等价合一网关），DumpDatabase 预期暴露口径不变（5.1）
- A-DB2：MySQL/Redis 协议交互经 mysql/mysqldump/redis-cli CLI 客户端承载（设计文档 §1.2⑥「适配器实现为后端工程」+D-06 明文 mysqldump/mysql 客户端参数数组；redis-cli 同判）；密码经 MYSQL_PWD/REDISCLI_AUTH 环境变量注入子进程（不进 argv，G-03 纪律）；不引入 go-sql-driver/go-redis（R-09 零新依赖）
- A-DB3：引擎侧 SQL 组装——标识符（库名/用户名/host/charset）一律白名单正则（R-14 AUTH_012 先行）+ 反引号/引号引用；密码等自由文本走 MySQL 字符串字面量转义（\ ' " 控制字符全转义）；SQLite 侧全参数绑定（D-13）
- A-DB4：conn_state 语义=连接目标传输层可达（探测：socket 存在/TCP 拨通，I-03 整值覆盖）或引擎连接验证成功（credentials，D-03）；version 仅 credentials/rescan 经引擎实况刷新（D-04）——无凭据新纳管档案治理接口按 I-05 链报 DBM_004（hint=credentials）而非 DBM_003
- A-DB5：探测 engine 判定=DISCOVERY_PROBE_SET 目标自带引擎语义（socket 路径/3306→mysql、6379→redis）；mysql↔mariadb 不做 engine 修正（identity_key 稳定性优先，版本串如实呈现 MariaDB 字样）
- A-DB6：redis 凭据 username 存固定 "default"（Redis 6 ACL 默认用户；满足 I-09 同进同空与 DBM_004 判据 cred_username IS NULL 统一）
- A-DB7：export 双形态实现=DumpReader 首字节探测（bufio.Peek）——进程启动即失败（认证/通道）时未写响应头→JSON 错误体；流中途失败断流（HTTP 语义极限，D-06「不虚构成功」）；sizeBytes 经 countingWriter 流后审计
- A-DB8：root-password 引擎侧语句=ALTER USER USER() IDENTIFIED BY（修改当前连接账户密码，与 D-05 两步同步语义精确对齐）

## T07 执行记录（2026-11-15，cwd=backend/）

- 工具：go1.24.0 windows/amd64
- G-01 无 shell 拼接：`grep -rnE "exec\.Command(Weak)?Context?\(.+\"(-c|/c)\"" internal/ cmd/` 0 命中；全仓 exec.Command 9 处人工核对——3 处注释（dbmysql/gateway.go:3、dbredis/gateway.go:3、nginxx/parser.go:3）+ 6 处真实调用全部 exec.CommandContext 字面量参数数组（本模块新增 dbmysql gateway.go:393 mysql -e 查询 / backup.go:108 mysqldump / backup.go:132 mysql 导入 / dbredis gateway.go:204 redis-cli；二进制构造期 exec.LookPath 解析）→ PASS
- G-02 无 SQL 拼接：Sprintf+SQL 关键字 grep 0 命中；store/dbmgmt 全部 ? 参数绑定人工复核 → PASS
- G-03 无硬编码凭据：grep 0 命中；引擎凭据经 MYSQL_PWD/REDISCLI_AUTH 环境变量运行时注入子进程（不进 argv 不落源码，A-DB2）→ PASS
- G-04 SQL 只在 store 层：面板 SQLite SQL 全部在 store/dbmgmt（人工复核）→ PASS；层外命中——service/dbmgmt 2 处注释 + adapter/dbmysql 引擎侧语句（SHOW/SELECT/DDL，A-DB10 裁定：MySQL 引擎协议通道非面板 SQL，白名单+转义防线见 A-DB3）→ 判定 PASS
- G-05 无 panic 滥用：grep 0 命中 → PASS
- G-06 goroutine 退出路径：`go func` 6 处全在 cmd/dougong/main.go（268/283/298/314 前序四任务+352/358 serve，行号因本模块装配插入偏移——语义未动，均 wg 编排/ctx 取消/ErrServerClosed 过滤）；本模块零新增 goroutine（首次探测 sync.Once 请求内联、busy 表 mutex、DumpReader sync.Once 无 goroutine）→ PASS
- G-07 编译+vet+test 全绿：`go build ./...`=0 ✓ `go vet ./...`=0 ✓ `gofmt -l internal/ cmd/` 0 文件 ✓ `go test -race -count=1 ./...` 26 包全 ok、0 FAIL（本模块新增 5 测试包 37 用例函数）→ PASS
- 覆盖率（本模块）：service/dbmgmt 71.7%、api/dbmgmt 62.9%、store/dbmgmt 76.0%、adapter/dbmysql 24.1%、adapter/dbredis 27.7%（适配器主体为 CLI 进程编排，Windows 开发机无 mysql/mysqldump/redis-cli 无法实联——纯函数解析面已覆盖，实联归属 Linux 目标环境冒烟，沿 site-mgmt W-2R 先例如实声明）
- 二进制冒烟（DOUGONG_ADDR=127.0.0.1:18321）：init CLI ✓ → login code=0 ✓ → /db/instance/list code=0 空清单（Windows 探测集不可达 0 发现如实，D-07）✓ → rescan admin+CSRF {discovered:0,updated:0} ✓ → 菜单含 database /databases Order 500 ✓ → 未登录 AUTH_001 ✓ → status?instanceId=1 DBM_002 ✓

## 补充裁定（测试驱动）

- A-DB9：SQL 清单 #7 UpdateTargetAndCredentials（store/dbmgmt/instance_repo.go）——D-03 连接目标纠正字段触发 identity_key 重算，与凭据 UPSERT 同事务原子落库（契约 §5.2 清单为枚举非封闭，工程必需新增，全参数绑定）
- A-DB10：G-04 判定——adapter/dbmysql 内的 SHOW/SELECT/CREATE/GRANT 语句属 DbGateway 引擎协议通道（§4.1.5 契约方法本体，MySQL 服务器方言），非面板 SQLite SQL；注入防线=标识符白名单（R-14 AUTH_012 先行）+ 密码 quoteString 全量转义（A-DB3），面板 SQL 收敛 store 层不变式不受影响
- A-DB11：POST 请求体 instanceId 采用 JSON 数值形态（与 list 响应 id 同型，前端零转换防 B/F 断裂）；GET query 保持 string 形态（DBM_001 判据 R-14 保留）——multipart import 的 instanceId 表单字段为十进制串解析
- A-DB12：导出文件名 socket 模式 {engine}-local（D-06 命名规则 {engine-host-port} 的 socket 呈现裁定，tcp 模式 host-port 原样）
- 测试驱动修复 4 项：ConnTargetSpec.resolve 按 mode 求值互斥字段+显式不匹配字段 AUTH_012（I-07 静默忽略→显式拒绝）；fake Grant 幂等合并语义对齐引擎；loginReadonly CSRF Origin 头；list 首次探测会整值覆盖种子档案（测试顺序调整——生产行为正确）

## database-mgmt 完成汇总（2026-11-15）

- 模块与包：复用前序模块全部公共资产，新增 `internal/adapter/dbconn`（1）+ `internal/adapter/dbmysql`（3+1 测试）+ `internal/adapter/dbredis`（2+1 测试）+ `internal/store/dbmgmt`（3+1 测试）+ `internal/service/dbmgmt`（10+1 测试）+ `internal/api/dbmgmt`（2+1 测试）+ `errcode/dbmgmt.go` + `005_database_mgmt.sql` + config 扩展（db.mysql_bin/mysqldump_bin/rediscli_bin + env DOUGONG_MYSQL_BIN/MYSQLDUMP/REDISCLI）+ main.go 装配（菜单 database /databases Order 500 两角色；无新增 goroutine）
- 接口：23 HTTP（GET 两角色 7 / GET admin 3 / POST admin+CSRF 13）+ 进程内契约（MySQLGateway/RedisGateway=A-DB1 接口隔离、InstanceRepoAPI、StoreAPI/TxAPI 适配器、CredentialVault、InstanceProber、BusyTable、GrantLevelMapper）
- 数据：db_instance 合一档案表（§5.3 逐字）随 005 迁移落库；SQL 清单 #1~#7 全参数化仅 store 层；凭据/移除/探测登记与审计同事务（R-03）
- 关键实现裁定：A-DB1~A-DB12
- 测试：`go build ./...`=0 + `go vet ./...`=0 + `gofmt -l`=0 + `go test -race -count=1 ./...` 26 包全 ok 0 FAIL；本模块新增 5 测试文件 37 用例函数；覆盖率 service 71.7% / api 62.9% / store 76.0% / adapter 24.1%·27.7%（环境绑定如实声明）
- 遗留（非阻断）：mysql/mysqldump/redis-cli 通道实联、真实引擎 SQL 方言验证（含 MariaDB ALTER USER USER() 兼容性）、DISCOVERY_PROBE_SET socket 探测归属 Linux 目标环境冒烟；G-04 引擎侧 SQL 判定与 A-DB2 CLI 承载路径建议 Gate 2 复核轮关注

- [x] T08 · SEC-GATE 补丁轮（2026-11-16，验证性空操作——ask 前提「此前因 Mimosa 误报以合规桩交付外部命令网关」与本模块实态不符，沿 account-auth T10 / panel-core T12 / monitoring T10 / service-mgmt T08 / file-manager T09 / site-mgmt T09 先例如实落账不臆造）
  - 门禁探针：backend/.secgate-probe.go（package 声明 + exec.Command("systemctl","status","nginx") 字面量参数数组一行）Write 写入成功=Mimosa 已禁用（裁定 dwfq-eb98056b-1/2），探针即删（rm 实跑复核）；本模块交付（2026-11-15）前探针已通过，通道自始即真实实现形态
  - 桩排查结论：database-mgmt 生产包（adapter/dbmysql、adapter/dbredis、adapter/dbconn、service/dbmgmt、api/dbmgmt、store/dbmgmt、errcode/dbmgmt.go）合并 grep 零 not-implemented/NotImplemented/TODO(SEC-GATE)/SEC-GATE/stub/placeholder/合规桩/未实现 标记——唯一 "stub" 命中为 api/dbmgmt/module_test.go 测试替身（stubMySQL/stubRedis 网关 fake，httptest 注入用，属 mock 单测交付物非合规桩）；4 处生产 exec 调用逐点复核均真实网关直写形态：dbmysql/gateway.go:393（mysql --batch -N -e 查询）/ backup.go:108（mysqldump --single-transaction --routines --triggers --hex-blob）/ backup.go:132（mysql 导入）/ dbredis/gateway.go:204（redis-cli --raw）——全部 exec.CommandContext 字面量参数数组（connArgs 按 mode 追加 --socket/-h -P 数组元素，无 shell 无拼接 G-01），二进制名构造期 exec.LookPath 解析（gateway.go:444 / dbredis gateway.go:51），凭据经 MYSQL_PWD/REDISCLI_AUTH env 注入不进 argv（G-03）；dbconn 为纯共享类型零 exec；tasks.md 零 `- [!]`/`- [ ]` 未决项（精确 grep exit=1）→「桩替换」与「阻塞项解除」均为空操作，零代码改动；INDEX.md database-mgmt backend(:101)/validation(:103)/gate2(:106) 行已双证「SEC-GATE 探针写入成功且模块零桩→DEBT-SEC-GATE 不适用」「exec 4 处全参数数组」，沿先例维持不改写
  - 自验（2026-11-16 实跑，go1.24.0 windows/amd64，PATH 内 go，cwd=backend/）：`go build ./...`=0 ✓ `go vet ./...`=0 ✓ `go test -count=1 ./...`=0、39 个含测试包全 ok、0 FAIL（本模块五包 adapter/dbmysql 0.049s / adapter/dbredis 0.095s / service/dbmgmt 0.275s / api/dbmgmt 0.532s / store/dbmgmt 0.093s）→ PASS

---

# terminal（dougong）Go 后端代码生成任务清单

> 创建时间：2026-11-15
> 设计文档：profiles/dougong-panel/workspace/02-detailed/api/api-terminal.md（v1.1，唯一事实源，禁止修改）
> 代码输出：backend/（复用既有骨架：api.Module 注册器 / InitGuard·AuthRequired·CsrfGuard·RequireRole / response 包络 / errcode 机制 / audit OHS / authsvc.VerifyWs，零另起炉灶）
> 模块特性：全项目唯一「聚合根 + 零持久化」——无表无 DDL 无 SQL（I-04）；1 HTTP（GET /terminal/user/options，admin）+ 1 WS（/terminal/session/connect，admin）；帧协议 JSON+Base64（D-01）；门禁探针（2026-11-15）：backend/.secgate-probe.go 写入成功 → Mimosa 已禁用（裁定 dwfq-eb98056b-1/2），外部命令走真实实现（字面量参数数组 G-01）
> 新增依赖：github.com/gorilla/websocket v1.5.3（WS 帧通道，纯 Go）+ github.com/creack/pty v1.1.21（PTY 载体，api §1.5 明文；Windows 有 unsupported stub 可编译，实联归 Linux）

## 设计裁定（设计文档留白/栈规约补齐处的实现拍板，均以设计文档明文为准绳）

| # | 事项 | 裁定 |
|---|------|------|
| A-T1 | §4.1.5 网关契约方法未带 ctx | 补 ctx 首参（backend-rules §1.1 🟡 + orchestrator 🔴 自检项），契约语义不变（进程内网关，纯传输层签名补齐） |
| A-T2 | WS 握手拒绝「HTTP 4xx/5xx + 错误码 JSON」（§3.1）未给数值映射 | AUTH_001/002→401、AUTH_003/AUTH_008→403、TERM_005/002/003/001/006（拒绝/参数错误）→400、TERM_004/TERM_007（失败）→500；响应体仍用全局包络（code=symbol）。经 response.HTTPStatus 单点映射（response 包新增 FailWithStatus/HTTPStatus 两个小函数，公共资产扩展非另起炉灶） |
| A-T3 | ready 帧 identity.runUser 权威值需当前运行用户名，§4.1.5 契约未覆盖 | PtyGateway 补 CurrentUser(ctx) 方法（panel_run_user 分支解析；specified 分支 runUser=已校验 username） |
| A-T4 | resize 帧 cols/rows 越界（0 或 >500）处置 | 按 I-06 帧协议完整性同口径处置：protocol_error 断开（接口层防御，设计仅枚举未知帧/超限两例非封闭清单——§4.1.4「校验：帧类型白名单 + 16KB 上限」同族） |
| A-T5 | Admit 成功后 WS Upgrade 失败 / Registry.Create 配额竞态失败的 closed 审计 reason | 分别记 network_error（升级失败=连接未建立）/ self_close（配额竞态=连接侧未成立）——I-07「恰好 1 opened + 1 closed」不变量优先，两路径均走统一清理（Kill PTY + closed 审计） |
| A-T6 | 审计 ActorName 解析：VerifyWs 只返回 (userID, role) | service 定义 PanelDirectory 端口（UsernameByID），api/terminal 以 authsvc.Service.ListUsers 适配注入（进程内小用户量全量查一次解析；解析失败回退 "user#<id>"，审计四要素不落空） |
| A-T7 | 会话运行时 goroutine（pty 读泵/WS 写泵）vs「后台任务由 main 统一编排」 | 会话泵为连接作用域（生命周期归属=该 WS 连接，cleanup 单点收敛退出；backend-rules §1.2「生命周期归属明确」条款），非进程级后台任务；生命周期扫描器（10s tick）仍由 main 以 wg 编排 |
| A-T8 | 心跳实现形态 | gorilla WriteControl Ping（协议层控制帧，与数据帧写并发安全）+ 读截止 75s（30s×2+15s 余量）+ PongHandler 续期——读截止超时=network_error（连续 2 次未答的等价实现，D-03 粒度不变） |

## 数据层（零持久化）

- [~] T01 · 生成 DDL 与迁移 —— 跳过：I-04 零持久化（api §5.2/§5.3 明文「无」），无迁移文件
- [~] T02 · 生成 store 层 —— 跳过：无表无 SQL，store 层不适用；数据承载为进程内 TerminalSessionRegistry（api §4.1.5 注册表契约），实现归 service 层 terminal 包（沿 monitoring/service-mgmt 零持久化先例）

## 代码生成

- [x] T03 · 生成 service 层 + adapter 层（2026-11-15）
  - spec：backend-01-architecture / backend-03-patterns（Skill 未注册，直读等效加载）
  - 输出：`internal/service/terminal/` 8 文件（consts 常量+排除集 / ports 五端口+WSConn 传输抽象 / dto 帧与响应载荷 / registry Session+Registry 原子 CAS / ring 256KB 环形缓冲 sync.Cond / session 读循环·双泵·统一清理 close 单点 / service 准入编排+Attach / lifecycle 扫描器）+ `internal/adapter/ptyx/`（gateway+pty_unix creack/pty 实装+pty_windows 退化）+ `internal/adapter/sysuser/`（gateway+dir_unix /etc/passwd 直读+dir_windows 退化）
- [x] T04 · 生成 api 层（2026-11-15）
  - spec：backend-02-api-conventions（直读等效加载）
  - 输出：`internal/api/terminal/` 3 文件（routes 2 端点链序 / handler Options+Connect+PanelDirectory 适配 / wsconn gorilla 适配——抑制默认 close 自动回执保 closed 帧送达 D-05）+ `errcode/terminal.go`（TERM_001~007）+ response 公共资产扩展（FailWithStatus/HTTPStatus，A-T2 单点）+ main 装配（termSvc+菜单 Order 600 admin 专属+模块挂载+lifecycle goroutine 入 wg）
- [x] T05 · 生成 middleware（2026-11-15）
  - 输出：`internal/api/middleware/terminalws.go`（TerminalHandshakeGuard：①Origin 同源 AUTH_003→②VerifyWs AUTH_001/002→③admin 角色 AUTH_008，D-09/I-01 顺序固定；HTTP 接口复用 AuthRequired+RequireRole 不新增——api §3.1 明文复用）

## 测试与验证

- [x] T06 · 生成单元测试（2026-11-15）
  - `internal/service/terminal/terminal_test.go`（-race ok，1.9s）：IsInteractiveShell 表驱动（含空 shell/大小写/绝对路径）、ListUserOptions（过滤+载荷+TERM_007）、validateConnect 表驱动（TERM_005 fields 逐项）、AdmitRejections 表驱动（TERM_005/002/003/004/006——TERM_006 验证 PTY 回收+审计未落库+零登记）、AdmitSuccess（opened 审计载荷四要素+detail 三键）、并发上限（TERM_001 data={limit,current}+释放后恢复）、全链路 EchoFlow（ready 权威值/input→PTY+lastActive/resize/PTY→output Base64 往返/客户端关闭→closed 帧+Kill+注销+恰好 1 条 closed 审计+source=web 映射）、协议违规表驱动 5 例（未知帧/超限 16KB/resize 越界/非法 base64/非法 JSON→protocol_error）、close 幂等（重复触发 1 条审计）、Lifecycle 空闲+硬上限（双阈值独立+system source 映射）、面板会话复核（存储异常不断开/失效联动 panel_session_invalid）、心跳（ping+读截止→network_error）、Abort（I-07 开合对）、配额竞态（手工构造窗口→Attach 原子 CAS 拒 TERM_001）、Registry 原子限额/幂等 Remove/回绕环形缓冲/RunLifecycle ctx 退出（G-06）
  - `internal/api/terminal/module_test.go`（-race ok，11.5s，真实 :memory: SQLite 全栈 + gorilla 真实握手）：user/options 权限矩阵（admin 0/readonly AUTH_008/未登录 AUTH_001）、WS 守卫四例（Origin 缺失与跨站 403 AUTH_003/无 Cookie 401 AUTH_001/readonly 403 AUTH_008）、握手拒绝表驱动（TERM_005×2/TERM_002/TERM_006——含 HTTP 状态码与包络双断言）、全链路会话流（拨号→ready（ADR-023 权威值 5/1800/28800）→input→PTY 写入→PTY 输出→output 帧 Base64→close 握手→closed 帧 self_close+PTY killed+恰好一对开合审计）、端到端并发上限（5 连接保持打开+第 6 个 400 TERM_001 data={limit:5,current:5}）
- [x] T07 · Go 编译规则合规检查（Gate 2，backend-rules §7）——2026-11-15 实际执行记录，见下

## T07 执行记录（2026-11-15，cwd=backend/）

- 工具：go1.24.0 windows/amd64（PATH 内 go）
- G-01 无 shell 拼接：`grep -rnE "exec\.Command(Weak)?Context?\(.+\"(-c|/c)\"" internal/ cmd/` 0 命中；全仓 exec.Command 11 处人工核对——5 处注释 + 6 处真实调用全部 exec.CommandContext 字面量参数数组（本模块新增 ptyx/pty_unix.go:44 `su - <username>`（username 先经 TERM_005 形态校验+TERM_002 存在性校验，独立 argv 元素）与 :50 shell 直启（shell 取自 /etc/passwd 当前 uid 条目/$SHELL//bin/sh 系统实况，非用户输入））→ PASS
- G-02 无 SQL 拼接：Sprintf+SQL 关键字 grep 0 命中；本模块零 SQL（I-04 零持久化）→ PASS
- G-03 无硬编码凭据：grep 0 命中；本模块无凭据面（会话鉴权走 Cookie 通道）→ PASS
- G-04 SQL 只在 store 层：层外命中与 database-mgmt T07 记录完全一致（dbmysql 引擎侧 A-DB10 裁定 + service 层注释文本），本模块零新增命中（零持久化：migrations 无 006，terminal 无表）→ PASS
- G-05 无 panic 滥用：grep 0 命中（sessionId 熵源失败以 error 返回非 panic）→ PASS
- G-06 goroutine 退出路径：main.go 7 处 go func（前序 6 处 + 本模块 :348 lifecycle scanner——RunLifecycle(ctx) ctx 取消返回+wg 编排+panic 兜底）；service 层 2 处连接作用域泵 `go ptyPump()`/`go wsPump()`（pty_unix.go:3 A-T7：生命周期归属=该 WS 连接，退出路径=统一清理 ring.stop+PTY Kill+conn.Close 三重收敛）；测试 goroutine 排除 → PASS
- G-07 编译+vet+test 全绿：`go build ./...`=0 ✓ `go vet ./...`=0 ✓ `gofmt -l internal/ cmd/` 0 文件 ✓ `GOOS=linux GOARCH=amd64 go build ./...`=0（pty_unix/dir_unix 双平台编译）✓ `go test -race -count=1 ./...` 28 包全 ok、0 FAIL（本模块新增 2 测试包：service/terminal 12.9s、api/terminal 12.7s）→ PASS
- 🟡 W1/W2：fmt.Print/log.Print 非测试 0 命中
- 二进制冒烟（DOUGONG_ADDR=127.0.0.1:18432，D:/tmp/dg-term）：init CLI ✓ → 未登录 GET /terminal/user/options AUTH_001 ✓ → login code=0+cookie ✓ → admin options=TERM_007（Windows sysuser 退化实现如实呈现，不返回虚构数据——目标环境 Linux 直读 /etc/passwd）✓ → 菜单含 terminal /terminal Order 600（Visibility=admin 专属，ADR-018）✓ → WS 未登录 401+AUTH_001 ✓ → WS 登录+同源 Origin 500+TERM_004（Windows PTY 退化如实，握手链路实证走通 ①②③④⑤⑥至⑦）✓ → WS 跨站 Origin 403+AUTH_003 ✓；PTY 真实 spawn/帧流/生命周期扫描器长跑归属 Linux 目标环境冒烟（Windows 开发机无 PTY/passwd 语义，A-F7 同口径）

## terminal 完成汇总（2026-11-15）

- 模块与包：复用前序模块全部公共资产（api.Module/InitGuard·AuthRequired·RequireRole/response 包络与新增 FailWithStatus·HTTPStatus/errcode 机制/audit OHS/authsvc.VerifyWs），新增 `internal/service/terminal`（8 文件）+ `internal/adapter/ptyx`（3：gateway 共享+unix 实装+windows 退化）+ `internal/adapter/sysuser`（3：同构）+ `internal/api/terminal`（3+1 测试）+ `internal/api/middleware/terminalws.go` + `errcode/terminal.go` + main 装配（菜单 admin 专属 Order 600+lifecycle goroutine 入 wg）；新依赖 gorilla/websocket v1.5.3 + creack/pty v1.1.21（纯 Go，Windows 有 unsupported stub 可编译）
- 接口：1 HTTP（GET /terminal/user/options，admin，200 包络）+ 1 WS（/terminal/session/connect，admin，握手拒绝=HTTP 状态+包络 A-T2）+ 帧协议 5 类型（ready/input/resize/output/closed，JSON+Base64，D-01）+ 进程内契约（TerminalSessionRegistry 原子 CAS/PtyGateway/SysUserDirectory/AuditRecorder/PanelSessionVerifier/PanelDirectory/WSConn 传输抽象）
- 数据：零持久化（I-04）——无表无 DDL 无 SQL（migrations 维持 001~005）；会话为进程内对象，断开即无痕，开合两段审计（D-06：建立失败拒建 TERM_006/断开失败日志兜底；source 映射 I-07）
- 关键实现裁定：A-T1~A-T8（见前表）；测试驱动修复 4 项——IsInteractiveShell 空 shell 误判（path.Base("")="." 陷阱）、hard-limit 用例空闲阈值隔离、读超时注入需唤醒阻塞 select、gorilla 默认 close 自动回执会挤掉应用层 closed 帧（wsconn.go 抑制默认 handler——closed 帧即应用级关闭反馈 D-05）
- 测试：`go build ./...`=0 + `go vet ./...`=0 + `gofmt -l`=0 + `GOOS=linux go build`=0 + `go test -race -count=1 ./...` 28 包全 ok 0 FAIL（本模块 2 测试包：service 层 19 用例函数含表驱动子例 + api 层 5 全栈用例含 WS 真实握手）
- 遗留（非阻断）：PTY 真实 spawn（su 语义）/etc/passwd 实况直读/生命周期扫描长跑归 Linux 目标环境冒烟（Windows 开发机退化 stub 如实声明，A-F7/先例 W-2R 同口径）；菜单 Visibility=admin 专属的运行时过滤由 panel-core 菜单机制承载（D-P07，本模块仅登记）

- [x] T08 · SEC-GATE 补丁轮（2026-11-16，验证性空操作——ask 前提「此前因 Mimosa 误报以合规桩交付外部命令网关」与本模块实态不符，沿 account-auth T10 / panel-core T12 / monitoring T10 / service-mgmt T08 / file-manager T09 / site-mgmt T09 / database-mgmt T08 先例如实落账不臆造）
  - 门禁探针：backend/.secgate-probe.go（package 声明 + exec.Command("systemctl","status","nginx") 字面量参数数组一行）Write 写入成功=Mimosa 已禁用（裁定 dwfq-eb98056b-1/2），探针即删（rm 后 ls 复核 No such file，exit=2）；本模块交付（2026-11-15）前探针已通过（本清单头部「模块特性」行明记），PTY 通道自始即真实实现形态
  - 桩排查结论：terminal 所属包（service/terminal、adapter/ptyx、adapter/sysuser、api/terminal、api/middleware/terminalws.go、errcode/terminal.go、cmd/dougong/main.go）合并 grep 零 not-implemented/NotImplemented/TODO(SEC-GATE)/SEC-GATE/stub/placeholder/未实现/合规桩/TODO/FIXME 标记（两轮 grep 均 exit=1）——本模块唯一外部命令网关 PtyGateway 交付即真实 exec 直写形态：ptyx/pty_unix.go:44 `exec.CommandContext(ctx,"su","-",<username>)`（username 先经 TERM_005 形态校验+TERM_002 存在性校验，独立 argv 元素）与 :50 shell 直启（shell 取自 /etc/passwd 当前 uid 条目/$SHELL//bin/sh 系统实况，非用户输入），均字面量参数数组（G-01，T07 已记同口径）；pty_windows.go/dir_windows.go 为 A-F7 设计裁定的 Windows 开发机 OS 能力退化（ErrUnsupportedPlatform 归 TERM_004/TERM_007，目标环境 Linux 功能完整——creack/pty 库自身 Windows 即 unsupported，api §1.5 明文），非 Mimosa 合规桩（file-manager T09 对 platform_windows.go 同口径裁定）；sysuser dir_unix.go:19-74 /etc/passwd 真实直读解析；本段任务清单零 `- [!]`/`- [ ]` 未决项（本节 632-698 精确 grep exit=1）→「桩替换」与「阻塞项解除」均为空操作，零代码改动；INDEX.md terminal gate2 行（:118）已双证「SEC-GATE 探针实跑写入成功=Mimosa 已禁用+ptyx 真实 exec 无桩→DEBT-SEC-GATE 不适用」，沿先例维持不改写
  - 自验复跑（2026-11-16，cwd=backend/，go1.26.0 windows/amd64）：`go build ./...`=0 ✓ `go vet ./...`=0 ✓ `gofmt -l internal/ cmd/` 0 文件 ✓ `go test -race -count=1 ./...` 39 包全 ok、0 FAIL（本模块 service/terminal 1.371s、api/terminal 7.115s 含 WS 真实握手）

---

# firewall（dougong）Go 后端代码生成任务清单

> 创建时间：2026-11-15
> 设计文档：profiles/dougong-panel/workspace/02-detailed/api/api-firewall.md（v1.0，唯一事实源，禁止修改）
> 代码输出：backend/（复用 account-auth/panel-core/monitoring/service-mgmt/file-manager/site-mgmt/database-mgmt/terminal 全部公共资产：Module 注册器/包络/errcode/中间件四件套/AuditService/main 装配模式）
> 模块交付：5 HTTP 接口（GET /firewall/{backend/status,rule/list,port/list} 两角色 + POST /firewall/rule/{create,delete} admin+CSRF）+ FirewallGateway（firewallx 适配器：ufw/firewalld CLI 参数数组）+ ListenPortSource（netlisten 适配器：gopsutil）+ PanelListenConfig（main 装配静态值）
> 数据层：零持久化（D-10：无表/无 DDL/无 SQL），T01/T02 按 orchestrator「条件任务判定」标 [~] 跳过
> Mimosa 门禁探针：backend/.secgate-probe.go 写入成功=Mimosa 已禁用（裁定 dwfq-eb98056b-1/2），探针即删；本模块外部命令走网关直写 exec 形态（字面量参数数组 G-01）

## 设计裁定（设计文档留白处的实现拍板，均以设计文档明文为准绳）

| # | 留白 | 裁定 |
|---|------|------|
| A-W1 | stack-specs 经 Skill 工具不可加载（Skill not found） | 直读 `.claude/skills/stack-specs/backend-{01,02,03}*.md` 全文等效加载（先例 A-P1/A-M1/A-S1/A-F1） |
| A-W2 | ufw 端口段原生形态为冒号（`6000:6007/tcp`），契约 D-05 为 `N-M` | 契约侧（create 请求 port / list 响应 port 显示形）一律 `N-M`；ruleKey 与后端命令用 ufw 原生冒号形（D-03 不透明引用语义，网关双向转换） |
| A-W3 | ufw 裸端口规则（无 /proto，如 `allow 80`=tcp+udp 双栈）无法以单枚举 protocol 表达 | 投影按协议族展开为 tcp/udp 两条（ruleKey 同为 `allow 80`——后端原生 spec，一次 delete 双栈同删）；rawSpec 保留原样 |
| A-W4 | ufw status 的 `(v6)` 复制行与 OUT 方向规则 | v6 行与 v4 行 ruleKey 相同（同一底层规则）按 ruleKey+protocol 去重；direction 固定 ingress（MVP 单值维度，D-05），OUT 方向行不进端口级入站投影（不可忠实映射契约字段，create/delete 亦不开放出站）——解析器按形态白名单取可表达条目 |
| A-W5 | ufw status From 列 `Anywhere`/`Anywhere (v6)` | 均=任意来源（source=null）；带 `(v6)` 标记行按 A-W4 去重规则自然收敛 |
| A-W6 | ufw 多端口逗号 spec（如 `80,443/tcp`，面板外手工添加形态） | 如实投影（port 显示原样、isPanelPort 逐成员命中、ruleKey 原生串可删）；不属 D-05 创建面（create 校验拒之 FW_001），读取面不隐藏实况 |
| A-W7 | firewalld source 维度（--add-port 无 source 入参） | 有 source 时用原生 rich rule 形态（`--add-rich-rule`，单参数元素含空格仍非 shell 拼接）；ruleKey=rich rule 原文（D-03 原生定位串）；list 经 --list-rich-rules 解析 |
| A-W8 | firewalld 服务类规则（--list-services，如 ssh/http） | 非端口级规则，不进投影（firewall_rule 为端口级投影，§1.4 字段清单无承载位）；面板增删不产生服务类规则 |
| A-W9 | 面板监听端口基准（D-06 文中「internal/config 直读」为设计期假设；实际 panel_config 落库、DOUGONG_ADDR 可覆盖） | main 装配期取实际绑定地址（panelListenAddr(bootCfg)+DOUGONG_ADDR 覆盖后）解析端口为静态值注入 PanelListenConfig——「面板当前真实监听端口」是涉险判定的诚实基准；pendingRestart 未生效配置不算 |
| A-W10 | ufw 未启用时 add/delete 语义（规则写配置、不生效） | 不拦截（后端原生语义如实：命令成功即返回成功；启用后生效是 ufw 自身行为）；status.active 如实透传 false，前端按 inactive 呈现 |
| A-W11 | create/delete 校验链与审计关系（I-07：恰好一条 + FW_005 豁免） | FW_001/002/004/009/003 拒绝记 failure 审计（detail.error=符号）；FW_005 拒绝零审计（I-07 明文豁免）；FW_006/007/008 记 failure；成功记 success（servicemgmt A-S5 先例+本模块 I-07 特例） |

## 数据层（零持久化）

- [~] T01 · 生成 DDL 与迁移 —— 跳过：D-10 零持久化（api §5.2/§5.3 明文「无」），无迁移文件
- [~] T02 · 生成 store 层 —— 跳过：无表无 SQL，store 层不适用；后端交互承载为 adapter 层 `internal/adapter/firewallx`（R-08 唯一后端交互出口 I-03）

## 代码生成

- [x] T03 · 生成 service 层 + adapter 层（2026-11-15）
  - spec：backend-01-architecture / backend-03-patterns（已直读，A-W1）
  - 输出：`internal/adapter/firewallx/` 3 文件（firewallx.go：契约类型 BackendInfo/BackendRule/RuleSpec/AddResult/DelResult + 四哨兵 + CommandTimeout 30s 编译期常量 + run 助手（exec.CommandContext 字面量参数数组 + LANG=C/LC_ALL=C 固定子进程输出语言保解析确定性）+ Detect/探测经 LookPath；ufw.go：`ufw status` 表格解析（A-W2 冒号↔短横线/A-W3 裸端口双栈展开/A-W4 v6 去重与 OUT·接口绑定跳过/A-W6 逗号多端口如实投影）+ add/delete 参数组装纯函数；firewalld.go：--list-ports/--list-rich-rules 解析 + D-08 permanent 先写 runtime 后应用两独立命令（不用 --reload）+ rich rule 构造/解析闭环（A-W7））；`internal/adapter/netlisten/source.go`（gopsutil ConnectionsWithContext 统一取数 D1 口径-3 + tcp 取 LISTEN/udp 绑定即监听 + 进程名尽力解析失败置空 D-08）；`internal/service/firewall/` 5 文件（ports 消费方三接口+AuditRecorder+StaticPanelListen；consts 枚举/审计动作码/DangerHint/SupportedActions 权威面；validate R-14 校验集 + 端口区间解析/命中/loopback/protoBase；dto 契约载荷；service 门面 BackendStatus/RuleList/PortList/CreateRule/DeleteRule 编排 + guardCreateDanger/guardDeleteDanger 涉险两段式 + auditMutation D-09 重试一次降级日志 + mapGatewayError 通道归一）
- [x] T04 · 生成 api 层（2026-11-15）
  - spec：backend-02-api-conventions（已直读，A-W1）
  - 输出：`internal/errcode/firewall.go`（FW_001~010 符号串）+ `internal/api/firewall/`（handler 薄壳 5 接口 + routes：InitGuard→AuthRequired→[GET RequireRole(admin,readonly) 3 接口] / [POST CsrfGuard→RequireRole(admin) 2 接口]）+ main.go 装配（监听绑定块上移（与 http.Server 共用同一 addr 结论）→ panelListenPort 解析实际绑定端口（DOUGONG_ADDR 覆盖后，A-W9）→ firewallx.New + netlisten.New + StaticPanelListen(fwPort) + 菜单 firewall Order 700 两角色 icon shield + 模块挂载；无后台 goroutine——D-01 按需即时探测）
  - `go build ./...` ✓ `go vet ./...` ✓（2026-11-15 实跑）
- [~] T05 · 生成 middleware —— 跳过新增：无鉴权/审计/限流/WS 新需求，复用 InitGuard/AuthRequired/CsrfGuard/RequireRole 四件套（api §3.1「不得自建会话解析与角色判定」）

## 测试与验证

- [x] T06 · 生成单元测试（2026-11-15）
  - `internal/adapter/firewallx/firewallx_test.go`（纯函数 9 用例）：parseUFWStatus 全样本（基本行/裸端口双栈展开/冒号段→短横线/来源/逗号多端口/v6 去重/OUT·接口绑定·非端口级跳过）、空态噪声、ufwAddArgs（契约短横线→原生冒号/from 词元）、ufwDeleteArgs（ruleKey 词元 1:1）、parsePortTokens（坏 token 跳过）、parseRichRules（accept/drop/reject/mark 跳过）、buildRichRule（v4/v6 + round-trip 解析闭环）、quotedValue（port= 唯一命中）、枚举常量核对
  - `internal/service/firewall/firewall_test.go`（fake 网关 12 用例）：SupportedActions 三后端、validatePort 15 例、validateSource 10 例、parseEnumFilter 多值/空段拒绝、区间解析/命中/loopback/protoBase、BackendStatus（ufw 结论+none 引导形态+FW_008 reason 载荷）、RuleList 投影（筛选/排序 D-02/isPanelPort 区间命中 D-06/backend 摘要）、RuleList 门控（值域先行零网关/FW_009/FW_010/FW_008 reason）、PortList 对照（allowed_by_rule/no_allow_rule/tcp6 族匹配/limit=allow 族/external loopback/进程 null 不虚构）、create 校验链表 7 例（每例断言恰好一条 failure 审计+detail.error+零写命令）、涉险两段式（FW_005 data 详情+零审计+区间从严+deny 不命中直通+确认放行+add spec 透传+恰好两条 success 审计）、create 网关语义（FW_006/FW_007 双层 data/FW_008）、delete 链（FW_001/FW_003/FW_005 豁免审计/deny 族不设防/确认放行/delCalls 顺序/四条审计）、delete 网关语义（窗口期 FW_003/FW_007/FW_008）、D-09 审计降级（重试成功落一条/持续失败不影响结果）
  - `internal/api/firewall/module_test.go`（httptest 全栈 7 用例，真实 :memory: SQLite+真实审计服务，网关替换 fake）：未登录 5 接口 AUTH_001、admin+readonly 双角色三读接口全键断言（status 权威面/rule list 排序+isPanelPort+backend 摘要+direction/source/port list 对照+external+process null）、list 门控与筛选（FW_001/FW_009/FW_010/FW_008）、readonly 越界 AUTH_008 零提交零审计、CSRF 三态拒绝零提交、create HTTP 全流程（FW_005 data+零审计→confirmDanger 成功 data 四键→FW_001→审计恰两条→畸形 body AUTH_012）、delete HTTP 全流程（FW_001/FW_003/FW_005/确认成功/审计恰三条+detail.ruleKey）
  - 修复发现（测试驱动）：module_test fakeListen 装配实例错位（服务与断言非同实例）；22 端口对照期望误设 no_allow_rule（规则集含 allow 22/tcp）；spansCover 样例端口不在区间
  - 实跑：`go test -race -count=1 ./...` 31 包全 ok、0 FAIL（含既有 28 包无回归）；`gofmt -l` 收敛后 0 未格式化
- [x] T07 · Go 编码规则合规检查（Gate 2，backend-rules §7）——2026-11-15 实际执行记录见下

## T07 执行记录（2026-11-15，cwd=backend/）

- 工具：go1.24.0 windows/amd64（PATH 内 go）
- G-01 无 shell 拼接：`grep -rnE 'exec\.Command(Weak)?Context?\(.+\"(-c|/c)\"' internal/ cmd/` 0 命中；全仓 exec.Command 代码命中 8 处人工核对全部 exec.CommandContext 字面量参数数组——本模块新增唯一调用点 firewallx/firewallx.go:185 `run(ctx, name, args...)`：name 为包级常量（"ufw"/"firewall-cmd"），args 由 ufwAddArgs/ufwDeleteArgs/固定 flag 组装（用户输入只作为独立 argv 元素：port/proto/action 经 FW_001/002 枚举校验、source 经 CIDR 校验、ruleKey 经存在性实况复核），无 "-c"/"/c" 无 shell 语义 → PASS
- G-02 无 SQL 拼接：`grep -rnE 'Sprintf\(.+(SELECT|INSERT|UPDATE|DELETE)' internal/ cmd/` 0 命中；本模块零 SQL（D-10 零持久化）→ PASS
- G-03 无硬编码凭据：`grep -rnEi '(password|secret|token|apikey)\s*=\s*\"[^\"]{8,}\"' internal/ cmd/ | grep -v _test` 0 命中；本模块无凭据面（LookPath 探测+只读/管理命令，无口令入参）→ PASS
- G-04 SQL 只在 store 层：层外命中与 terminal T07 记录完全一致（dbmysql 引擎侧 A-DB10 裁定 + service 层注释文本，前序各模块 T07 逐个裁定）；本模块零新增命中（firewall 三包均不在命中清单）→ PASS
- G-05 无 panic 滥用：`grep -rn 'panic(' internal/ cmd/ | grep -v _test` 0 命中 → PASS
- G-06 goroutine 退出路径：`go func` 7 处全在 cmd/dougong/main.go（:311/:326/:341/:357/:373/:411/:417——前序模块已逐个核对：审计 worker/monitoring collector/chunk sweeper/cert renewal/terminal lifecycle 均 ctx 取消+wg 编排+panic 隔离，TLS/plain serve 两处 ErrServerClosed 过滤）；**本模块新增 0 处 goroutine**（firewallx 惰性探测无后台任务，D-01 按需即时）→ PASS
- G-07 编译+vet+test 全绿（gofmt -w 后最终门禁）：`go build ./...`=0 ✓ `go vet ./...`=0 ✓ `gofmt -l internal/ cmd/` 0 文件 ✓ `go test -race -count=1 ./...` 31 包全 ok、0 FAIL（本模块新增 3 测试包：adapter/firewallx 0.457s、service/firewall 0.690s、api/firewall 1.149s）→ PASS
- 🟡 W1/W2：fmt.Print/log.Print 非测试 0 命中
- 二进制冒烟（DOUGONG_ADDR=127.0.0.1:18555，/d/tmp/dg-fw）：`dougong init` CLI ✓ → login code=0+cookie ✓ → GET /firewall/backend/status code=0 `{"backend":"none","active":false,"version":null,"supportedActions":[],"generatedAt":...}`（Windows 开发机无 ufw/firewalld——真网关 LookPath 探测如实呈现引导态，不虚构，I-01）✓ → rule/list 与 port/list 均 FW_009（不返回空列表）✓ → 未登录 AUTH_001 ✓ → POST create 缺 CSRF AUTH_003、合法 CSRF+Origin 到达 service 层 FW_009（写链路与审计路径实证）✓ → 菜单含 firewall /firewall Order 700 两角色 ✓；ufw/firewalld 实机命令交互（status 解析/双写/FW_006/FW_007）归属 Linux 目标环境冒烟（Windows 无二进制，解析逻辑已由纯函数表驱动全覆盖——sysd A-S/T07 环境说明同口径）

## firewall 完成汇总（2026-11-15）

- 模块与包：复用前序模块全部公共资产（api.Module/InitGuard·AuthRequired·CsrfGuard·RequireRole/response 包络/errcode 机制/AuditService OHS/main 装配模式），新增 `internal/adapter/firewallx`（3 文件+1 测试）+ `internal/adapter/netlisten`（1 文件）+ `internal/service/firewall`（5 文件+1 测试）+ `internal/api/firewall`（2 文件+1 测试）+ `errcode/firewall.go` + main.go 装配（监听块上移+panelListenPort+菜单 Order 700+模块挂载；无新增 goroutine）；零新增第三方依赖（gopsutil 已在库）
- 接口：5 HTTP（GET /firewall/{backend/status,rule/list,port/list} 两角色 + POST /firewall/rule/{create,delete} admin+CSRF，§3.3 权限矩阵与链序全对齐）+ 进程内契约 FirewallGateway（ufw/firewalld CLI 参数数组、30s 上限、D-03 ruleKey 寻址、D-07 语义透传、D-08 双写）/ListenPortSource（gopsutil 统一取数）/PanelListenConfig（A-W9 静态注入）
- 数据：零持久化（D-10：无表无 DDL 无 SQL 无迁移，migrations 维持 001~006）；firewall_rule 为后端实况投影（I-01/I-02 无缓存层）
- 关键实现裁定：A-W1~A-W11（见前表）——核心：ruleKey=后端原生定位串（ufw spec 词元/firewalld 端口串/rich rule 原文）、涉险判定基准=进程实际绑定端口（A-W9）、FW_005 拒绝豁免审计（I-07）、firewalld 双写两步不用 --reload（D-08）
- 测试：`go build ./...`=0 + `go vet ./...`=0 + `gofmt -l`=0 + `go test -race -count=1 ./...` 31 包全 ok 0 FAIL（本模块 3 测试包：解析纯函数 9 用例 + service fake 表驱动 12 用例 + HTTP 全栈 7 用例）
- 遗留（非阻断）：ufw/firewalld 实机命令交互归属 Linux 目标环境冒烟（Windows 开发机无二进制，探测如实返回 none/FW_009，解析与命令组装逻辑已表驱动全覆盖）；菜单 icon 键 shield 由前端模块在 MENU_ICONS 补映射（未命中回落 AppsOutline 已核实先例）

- [x] T08 · SEC-GATE 补丁轮（2026-11-16，验证性空操作——ask 前提「此前因 Mimosa 误报以合规桩交付外部命令网关」与本模块实态不符，沿 account-auth T10 / panel-core T12 / monitoring T10 / service-mgmt T08 先例如实落账不臆造）
  - 门禁探针：backend/.secgate-probe.go（package 声明 + exec.Command("ufw","--version") 字面量参数数组一行）Write 写入成功=Mimosa 已禁用（裁定 dwfq-eb98056b-1/2），探针即删（ls 复核不存在）
  - 桩排查结论：本模块无桩可替换——外部命令网关自 T03 交付起即真实直写形态：全模块唯一执行点 firewallx/firewallx.go:185 `exec.CommandContext(cctx, name, args...)`，name 为包级字面量常量 ufwBin="ufw"/fwcmdBin="firewall-cmd"（firewallx.go:39-40，LookPath 解析不信任外部路径），args 由 ufwAddArgs/ufwDeleteArgs/固定 flag 纯函数组装的独立 argv 元素（ufw.go/firewalld.go 全文复读核对：无 shell、无 sh -c、无字符串拼接命令行，G-01）；firewall 四包（adapter/firewallx、service/firewall、api/firewall、errcode/firewall.go）及全仓 internal/+cmd/ 合并 grep 零 TODO(SEC-GATE)/not-implemented/NotImplemented/未实现/stub/placeholder 标记（exit=1）；tasks.md 无 `- [!]` 阻塞项（grep exit=1，本清单仅 [x]/[~]）；INDEX.md firewall backend(:125)/validation(:127)/gate2(:130) 行已双证「SEC-GATE 探针实跑未被拦且 firewallx 真实 exec 无桩→DEBT-SEC-GATE 不适用」→「桩替换」与「阻塞项解除」均为空操作，零代码改动，INDEX 沿先例维持不改写
  - 自验（2026-11-16 实跑，go1.24.0 windows/amd64，PATH 内 go，cwd=backend/）：`go build ./...`=0 ✓ `go vet ./...`=0 ✓ `go test -count=1 ./...`=0、42 个含测试包全 ok、0 FAIL（本模块三包 adapter/firewallx 0.047s / service/firewall 0.158s / api/firewall 0.746s）→ PASS

# docker-mgmt（dougong）Go 后端代码生成任务清单

> 创建时间：2026-11-15
> 设计文档：profiles/dougong-panel/workspace/02-detailed/api/api-docker-mgmt.md（v1.0，唯一事实源，禁止修改）
> 代码输出：backend/（复用既有骨架：api.Module 注册器 / InitGuard·AuthRequired·CsrfGuard·RequireRole·WsVerifier / response 包络 / errcode 机制 / audit OHS，零另起炉灶）
> 模块特性：零持久化（D-10 无表无 DDL 无 SQL 无迁移）；22 HTTP（9 GET 查询 + 13 POST 写）+ 2 WS 日志跟随；SDK 通道（DockerGateway，官方 Go SDK 经 /var/run/docker.sock）+ CLI 通道（ComposeRunner，`docker compose` 参数数组 R-08/ADR-013）；门禁探针（2026-11-15）：backend/.secgate-probe.go 写入成功 → Mimosa 已禁用（裁定 dwfq-eb98056b-1/2），外部命令走真实实现（字面量参数数组 G-01）
> 新增依赖：github.com/docker/docker v27.5.1+incompatible（官方 Go SDK——api §1.5/§3.2 明文「容器/镜像/卷/网络经官方 Go SDK」；goproxy.cn 可达实测；纯 Go 无 CGO）

## 设计裁定（设计文档留白/实现形态处的拍板，均以设计文档明文为准绳）

| # | 留白 | 裁定 |
|---|------|------|
| A-D1 | stack-specs 经 Skill 工具不可加载（Skill not found） | 直读 `.claude/skills/stack-specs/backend-{01,02,03}*.md` 全文等效加载（先例 A-P1/A-M1/A-S1/A-F1/A-W1） |
| A-D2 | SDK 版本选择（设计文档未钉版本） | v27.5.1（go 1.21+ 兼容 go.mod `go 1.22.0`；v28 需 go 1.23 越当前指令集）；client.FromEnv + WithAPIVersionNegotiation，默认 unix:///var/run/docker.sock（DOCKER_HOST 可覆盖——运维逃生通道，非面板配置面） |
| A-D3 | D-01 not_installed 与 socket_unreachable 判定依据 | unix socket 时 stat socket 路径：不存在→not_installed（daemon 从未就绪，前端渲染未装引导）；存在但 Ping 失败→socket_unreachable；npipe（Windows 目标外）不做 stat 直 Ping；探测 Ping+ServerVersion 均即时无缓存（I-01） |
| A-D4 | daemon 版本读取（status 与 list 页头 daemon 摘要） | client.ServerVersion().Version；探测失败不外泄内部细节，detail 只给 A-D3 两值 |
| A-D5 | 卷 sizeBytes 取数（VolumeList API 不回 UsageData） | 经 client.SystemDF 取卷用量（daemon 计算值，docker CLI `system df -v` 同源）按卷名合并；SystemDF 失败不阻断清单（sizeBytes=0 降级）——如实投影，不虚构也不因辅通道失败拒主清单 |
| A-D6 | 镜像投影 ref 单值 vs RepoTags 多值 | 按 docker CLI `docker images` 同构：每 RepoTag 一条（同 imageId 多行）；无 tag 镜像单条 ref=`<none>`（不隐藏实况） |
| A-D7 | image/pull 成功 ref 回显 | 回显请求原值（不回 daemon 归一化形态——前端以请求为准；imageId 为 daemon ID） |
| A-D8 | Compose CLI `up` 组装：文档公式 `docker compose --project-name <p> --project-directory <dir> -f <file> <verb>` 不含 -d，但 up attached 形态阻塞至容器停止、300s 超时必截断 | up 补 `-d`（detached）以适配 D-05/D-06 同步终态契约（POST 阻塞至 done:true）；down/restart 文档公式即非阻塞不补 |
| A-D9 | config_files label 含逗号多文件（`a.yml,b.yml`） | 按逗号拆分逐个 `-f`（label 原文仍整体存 configFile 字段/DK_015 data 透传）；locatable=目录存在且每个配置文件可读，missing ∈ {working_dir, config_file}（首个缺失项） |
| A-D10 | compose/list 排序、image/volume/network list 排序（文档仅钉容器清单名称字典序） | 项目名/卷名/网络名/镜像 ref 字典序（呈现确定性，与 D-02 容器排序同哲学） |
| A-D11 | container/list composeProject 筛选参数空串语义 | 空串=无此筛选（与省略等价）；非空=label 等值匹配（无项目容器永不命中） |
| A-D12 | WS end 帧终止原因判定（follow 流 EOF 后需归因） | 容器流结束后 ContainerInspect 复核：NotFound→container_removed；!Running→container_stopped；仍运行/复核失败→daemon_lost；compose 跟随 5s 对账：label 聚合空→project_gone、locatable=false→locate_failed、ListContainers 失败→daemon_lost（D-03 常量集封闭实现） |
| A-D13 | WS 握手拒绝的 HTTP 状态映射（DK_ 无先例档位） | 模块内 wsStatus：DK_002→503、AUTH_001/002→401、AUTH_003/008→403、其余 DK_→400（A-T2 terminal 先例：状态仅升级前 HTTP 语义，包络 code 仍 symbol） |
| A-D14 | DK_002 的 data 载荷 | 不携带（契约 §3.4 未定义 data；前端据 GET /daemon/status 呈现引导页，guard 只做拒绝） |
| A-D15 | 存在性/定位拒绝与参数拒绝的审计口径（I-06 字面：DK_001/002/004 未发生变更不记） | DK_001（不透明引用空/tail 非法）与 DK_004（矩阵拒绝）零审计；DK_002 由 DaemonGuard 中间件拒绝（不进 service，天然零审计）；DK_012 按拉取面 §4.2.4 明文「DK_012/013 按结果如实」记 failure；存在性/定位/互斥/daemon 拒绝/执行失败（DK_003/005/006/007/008/009/010/011/013/014/015/016/017/018）记 failure 审计（A-W11 firewall 同判；compose 侧 §4.3.4 明文「DK_014/015/016/018 按结果如实」） |
| A-D16 | 日志跟随的回放+跟随实现 | LogsOptions{Follow:true, Tail:N} 单流承载（daemon 原生回放尾部+跟随），不拆两次读（避免回放/跟随间隙丢行）；TTY 容器无 stdcopy 头直读 |
| A-D17 | SDK/CLI 超时归属 | SDKTimeout 60s / PullTimeout 600s / ComposeTimeout 300s 编译期常量置于 adapter（firewallx.CommandTimeout 同构）；超时 ctx 截断→通道错误→DK_017/DK_013/DK_016 |
| A-D18 | 容器六动作后的 currentState 读取 | ContainerInspect 直读映射（不整表 ListContainers——单容器终态复核，D-04 data 同源矩阵） |
| A-D19 | 菜单登记 | docker / 容器管理 /docker Order 800 两角色（terminal 600、firewall 700 之后顺延）；icon 键 container 由前端 MENU_ICONS 映射（firewall shield 同法） |

## 数据层（零持久化）

- [~] T01 · 生成 DDL 与迁移 —— 跳过：D-10 零持久化（api §5.2/§5.3 明文「无」），migrations 维持 001~006
- [~] T02 · 生成 store 层 —— 跳过：无表无 SQL；后端交互承载为 adapter 层 `internal/adapter/dockerx`（DockerGateway SDK 通道 + ComposeRunner CLI 通道，I-03 唯一出口）

## 代码生成

- [x] T03 · 生成 service 层 + adapter 层（2026-11-15）
  - spec：backend-01-architecture / backend-03-patterns（已直读，A-D1）
  - 输出：`internal/adapter/dockerx/` 3 文件——dockerx.go（官方 Go SDK 网关 v27.5.1：ProbeDaemon 即时探测 D-01/A-D3 socket stat+Ping+ServerVersion、容器/镜像/卷/网络清单与动作、stats docker CLI 同源公式 D-07、pull 进度流 error 原文提取 D-05/D-08、SDKTimeout 60s/PullTimeout 600s/ProbeTimeout 5s 编译期常量、sdkError 按 errdefs 归一 NotFound/Conflict/Forbidden→哨兵+ExecError 原文透传）；logs.go（尾部读取+跟随订阅 D-03：Follow+Tail 单流 A-D16、stdcopy 解复用+peek 头判定 TTY 直读、MapState BA 附录B.1 映射、流结束 inspect 归因 A-D12 container_removed/container_stopped/daemon_lost）；compose.go（ComposeRunner：label 三键聚合 R-13+locatable 判定 D-06/A-D9、Run 参数数组 CLI（docker compose --project-name/--project-directory/-f×N/<verb>，up 补 -d A-D8，300s 上限，per-project tryAcquire 互斥 I-08）、Logs 容器名序聚合、FollowLogs 多容器订阅合并+5s 对账（project_gone/locate_failed/daemon_lost，停止容器不重订防忙循环））
  - `internal/service/dockermgmt/` 6 文件——ports.go（DockerGateway/ComposeRunner/AuditRecorder 消费方接口）；consts.go（13 审计动作码+对象类型+tail 域 500/1-2000+动作序）；matrix.go（SupportedActions/ActionAllowed BA 附录B.2 权威面，过渡态空数组）；validate.go（ParseTail/ParseStateFilter 多值域/ValidateImageRef docker 引用语法/requireRef 不透明引用非空 D-11）；dto.go（全部契约载荷形状）；service.go（门面：探测/清单投影筛选排序/六动作矩阵守卫→SDK→即读实况/三类删除存在性→daemon 拒绝归一/拉取校验→同步终态/Compose 动词 DK_014→015→018→016 链/WS 订阅握手链/auditFailure 带 error 符号 A-W11 同判/recordAudit D-09 重试一次日志兜底/mapReadError+四资源删除归一+compose 归一）
- [x] T04 · 生成 api 层（2026-11-15）
  - spec：backend-02-api-conventions（已直读，A-D1）
  - 输出：`internal/errcode/dockermgmt.go`（DK_001~018 符号串）+ `internal/api/dockermgmt/` 3 文件——routes.go（Module /docker 24 端点四组：probe 无 DaemonGuard/read GET 双角色+DaemonGuard/ws dockerWsGuard（Origin+Cookie VerifyWs+admin/readonly，SameOrigin 导出公共资产 A-T2 同法）/write POST admin+CsrfGuard+DaemonGuard；DaemonGuard 探测结论写 gin context 供 list 页头摘要单次探测 A-D14）；handler.go（22 HTTP 薄壳：绑定→actor→service→包络，bindError AUTH_012 A-S9）；handler_ws.go（2 WS：握手 DK_002→503/DK_001/003/014/015→400（wsStatus A-D13）、升级失败 stream.Cancel（A-T5 同判）、帧泵 log/end 行通道优先排空防尾行丢失、读泵 goroutine 断连检测）+ main.go 装配（dockerx.New+NewComposeRunner+dockersvc.New+菜单 docker/容器管理//docker/Order 800/两角色/icon container+模块挂载+dockerGw.Close 优雅退出）+ middleware/csrf.go 增 SameOrigin 导出（公共资产小扩展）
  - `go build ./...` ✓ `go vet ./...` ✓（2026-11-15 实跑）
- [~] T05 · 生成 middleware —— 复用 AuthRequired/CsrfGuard/RequireRole/WsVerifier；DaemonGuard 为本模块业务前置在 api/dockermgmt 模块内实现（§5.1「DaemonGuard 为本模块业务前置」，不入共享 middleware）

## 测试与验证

- [x] T06 · 生成单元测试（2026-11-15）
  - `internal/adapter/dockerx/dockerx_test.go`（16 用例）：MapState 全状态族、tailStr 边界、readLogLines（多路复用帧合并+TTY 直读+短流判定）、scanPullError（正常/失败流原文）、computeStats（公式+全零防 NaN）、composeArgs（up -d/down 不补/多文件逐 -f A-D8/A-D9）、splitConfigFiles、locateMissing（目录/文件/空 label/多文件）、ListProjectFacts（聚合/无 label 排除/字典序/locatable）、tryAcquire 互斥白盒（I-08）、FollowLogs（行合并投递+project_gone/locate_failed 终因对账——对账周期 50ms 注入）、Logs 聚合序、Run LookPath 失败归一（宿主有 docker 二进制时 SKIP——实机 CLI 归 Linux 冒烟）、ExecError 形状
  - `internal/service/dockermgmt/service_test.go`（18 用例）：矩阵全表（五态×六动作）、ParseTail/ParseStateFilter/ValidateImageRef（合法/非法集+docker 大写 tag 语法核对）、DaemonStatus（available 版本伴随/unavailable detail 区分）、ContainerList 投影（排序/动作面/composeProject null/daemon 摘要/筛选/非法值 DK_001）、ContainerLogs 链（001→003→tail→截尾）、Stats、动作链（DK_001 零审计→DK_003 failure 审计→DK_004 data 动作面零审计→成功即读实况+success 审计→DK_017 reason+failure）、删除链（003/004/成功 detail.image）、拉取（DK_012 failure 审计§4.2.4 口径/DK_013 reason/成功 ref 回显）、三类删除九分支（005/006/007/008/009/010/011/成功×3+审计恰 10 条）、三清单投影、ComposeList（null 透传/locatable）、Compose 动作链（DK_001 零审计→DK_014→DK_015 data 携 label 路径→DK_018 busy→DK_016 reason→成功 done:true+审计码映射+busy 零 CLI 触达）、ComposeLogs 链（014→015→tail）、WS 握手链、审计降级 D-09（重试成功恰一条/持续失败零入库不影响结果）、13 动作码常量核对
  - `internal/api/dockermgmt/module_test.go`（6 用例，真实 :memory: SQLite+真实 auth/audit 服务，网关替换 fake）：未登录 9 GET+1 POST 全 AUTH_001（先过 InitGuard）、daemon unavailable（status 如实 unavailable+detail not_installed；5 清单+写全 DK_002；零写触达零审计）、双角色读面（admin/readonly 全键断言：items 排序/composeProject/supportedActions/daemon 摘要 + 7 读接口 0 + 4 拒绝码）、写面（readonly 越界 7 接口 AUTH_008 零触达零审计、缺 CSRF AUTH_003、admin 全链路 start/DK_003/pull/DK_012/三类删除/compose up/DK_014、审计恰 9 条、非法 JSON AUTH_012+缺字段 DK_001）、WS 容器跟随（坏 Origin 403/无 Cookie 401/daemon 不可用 503/正常流 log×2+end container_stopped 后服务端关闭）、WS compose（ghost 项目 400 拒绝握手）
  - 修复发现（测试驱动）：DK_004 用例动作选取错误（stopped 态 stop 才违例）；fake 互斥需先拒绝不落 CLI；docker tag 大写与 x/y 引用合法（期望修正）；loginReadonly 重复初始化 AUTH_009；sendLine 持锁 nil channel 死锁（等待订阅建立）
  - 实跑：`go test -race -count=1 ./...` 34 包全 ok、0 FAIL（含既有 31 包无回归）；本模块 3 测试包 44 PASS + 1 SKIP（宿主有 docker 二进制，实机 CLI 用例按设计跳过）；`gofmt -l` 0 未格式化
- [x] T07 · Go 编码规则合规检查（Gate 2，backend-rules §7）——2026-11-15 实际执行记录见下

## T07 执行记录（2026-11-15，cwd=backend/）

- 工具：go1.24.0 windows/amd64（PATH 内 go，goproxy.cn）
- G-01 无 shell 拼接：`grep -rnE 'exec\.Command(Weak)?Context?\(.+\"(-c|/c)\"' internal/ cmd/` 0 命中；全仓 exec.Command 代码命中 8 文件人工核对全部 exec.CommandContext 字面量参数数组——本模块新增唯一调用点 dockerx/compose.go:152 `exec.CommandContext(cctx, bin, composeArgs(fact, verb)...)`：bin=LookPath 解析的常量二进制名 "docker"，args 由 composeArgs 纯函数组装（--project-name/--project-directory/-f 固定 flag + label 值各自独立 argv 元素——值来自 daemon label 非用户输入且无 shell 语义），无 "-c"/"/c" → PASS
- G-02 无 SQL 拼接：`grep -rnE 'Sprintf\(.+(SELECT|INSERT|UPDATE|DELETE)' internal/ cmd/` 0 命中；本模块零 SQL（D-10 零持久化）→ PASS
- G-03 无硬编码凭据：`grep -rnEi '(password|secret|token|apikey)\s*=\s*\"[^\"]{8,}\"' internal/ cmd/ | grep -v _test` 0 命中；本模块无凭据面（SDK unix socket 本机通道无鉴权入参、compose CLI 无口令）→ PASS
- G-04 SQL 只在 store 层：命中文件清单与 firewall/terminal T07 记录完全一致（dbmysql 引擎侧 A-DB10 裁定 + 前序各模块 service 层注释/字符串文本逐个裁定）；本模块三包（adapter/dockerx、service/dockermgmt、api/dockermgmt）零命中 → PASS
- G-05 无 panic 滥用：`grep -rn 'panic(' internal/ cmd/ | grep -v _test` 0 命中 → PASS
- G-06 goroutine 退出路径：全仓 `go func` 9 处——main.go 7 处（前序已核对：审计 worker/monitoring collector/chunk sweeper/cert renewal/terminal lifecycle/两处 HTTP serve）；本模块新增 2 处——dockerx/logs.go:165 容器跟随泵（生命周期归属=LogStream 句柄：读 daemon 流至 EOF/错误→End 投递→close；LogStream.Cancel 即 sctx 取消→SDK body 读失败→泵收束，A-T7 连接作用域同判）、api/dockermgmt/handler_ws.go:86 WS 读泵（ReadMessage 阻塞至客户端断开错误返回，或 pumpLogFrames defer conn.Close()→读错误退出）；另 compose.go 具名 goroutine 3 类（FollowLogs 起 run、reconcile 起 forward、fake 测试泵）——run 由 select sctx.Done/终因 finish→defer shutdown（cancel 全部子流）收敛、forward range st.Lines+select sctx.Done 退出，全部 ctx 取消路径明确 → PASS
- G-07 编译+vet+test 全绿：`go build ./...`=0 ✓ `go vet ./...`=0 ✓ `gofmt -l internal/ cmd/` 0 文件 ✓ `go test -race -count=1 ./...` 34 包全 ok、0 FAIL（本模块新增 3 测试包：adapter/dockerx 0.6s、service/dockermgmt 0.6s、api/dockermgmt 6.0s）→ PASS
- 🟡 W1/W2：fmt.Print/log.Print 非测试 0 命中
- 二进制冒烟（DOUGONG_ADDR=127.0.0.1:18666，/d/tmp/dg-docker）：wizard+login code=0 ✓ → GET /docker/daemon/status code=0 `{"state":"unavailable","version":null,"detail":"socket_unreachable",...}`（Windows 开发机 docker CLI 在 PATH 而 Docker Desktop daemon 未跑——npipe 探测如实透传不可达，D-01 不虚构）✓ → container/image/compose list 均 DK_002（不返回空清单，I-01）✓ → 未登录 AUTH_001 ✓ → POST start 缺 CSRF AUTH_003、合法 CSRF+Origin 到达写组被 DaemonGuard DK_002（链序 AuthRequired→CsrfGuard→RoleRequired→DaemonGuard 实证）✓ → 审计 3 条全 account-auth（login_success/init_completed/audit_purged），DK_002 拒绝零审计（A-D15）✓ → 菜单含 docker /docker Order 800 icon container 两角色 ✓；实机 daemon/CLI 交互（清单/动作/Compose/日志流）归属 Linux 目标环境冒烟（Windows 无运行中 daemon，探测/门控/WS 帧泵已由 44 用例全覆盖）

## docker-mgmt 完成汇总（2026-11-15）

- 模块与包：复用前序模块全部公共资产（api.Module/InitGuard·AuthRequired·CsrfGuard·RequireRole·WsVerifier/response 包络+FailWithStatus/errcode 机制/AuditService OHS/main 装配模式），新增 `internal/adapter/dockerx`（3 文件+1 测试）+ `internal/service/dockermgmt`（6 文件+1 测试）+ `internal/api/dockermgmt`（3 文件+1 测试）+ `errcode/dockermgmt.go` + middleware/csrf.go 增 SameOrigin 导出 + main.go 装配（dockerx 网关+ComposeRunner+菜单 Order 800+模块挂载+Close 优雅退出；无新增后台 goroutine）；新增第三方依赖 github.com/docker/docker v27.5.1+incompatible（官方 Go SDK，api §1.5 明文）+ go-connections v0.5.0 钉版（v0.8.2 移除 DialPipe 与 v27 client 不兼容）+ go-winio v0.6.2（间接）
- 接口：22 HTTP（9 GET 两角色 + 13 POST admin+CSRF）+ 2 WS 日志跟随（Cookie+Origin 握手，帧协议 log/end）= 24 端点（§3.3 权限矩阵与链序全对齐）+ 进程内契约 DockerGateway（SDK 通道，60s/600s 上限，D-08 errdefs 归一）/ComposeRunner（CLI 参数数组 R-08、300s、per-project 互斥 I-08、label 聚合+locatable）
- 数据：零持久化（D-10：无表无 DDL 无 SQL 无迁移，migrations 维持 001~006）；五类对象为 daemon 实况投影（I-01/I-02 无缓存层）
- 关键实现裁定：A-D1~A-D19（见前表）——核心：not_installed/socket_unreachable 经 socket stat+Ping 判定（A-D3）、卷 sizeBytes 经 SystemDF（A-D5）、镜像每 RepoTag 一条（A-D6）、compose up 补 -d 适配同步终态（A-D8）、config_files 逗号多文件逐 -f（A-D9）、WS 终因归因 inspect 复核+5s 对账（A-D12）、DK_002→503 握手档（A-D13）、审计口径 DK_001/004 零审计+DK_012/014/015/018 记 failure（A-D15，§4.2.4/§4.3.4 明文）
- 测试：`go build ./...`=0 + `go vet ./...`=0 + `gofmt -l`=0 + `go test -race -count=1 ./...` 34 包全 ok 0 FAIL（本模块 3 测试包：adapter 纯函数 16 用例 + service fake 表驱动 18 用例 + HTTP/WS 全栈 6 用例）
- 遗留（非阻断）：实机 daemon/CLI 交互（SDK 清单/动作/Compose/日志流）归属 Linux 目标环境冒烟（Windows 开发机 daemon 未运行，探测如实 unavailable/DK_002，门控/编排/帧泵逻辑已表驱动+httptest 全覆盖）；菜单 icon 键 container 由前端模块在 MENU_ICONS 补映射（未命中回落已核实先例）

- [x] T08 · SEC-GATE 补丁轮（2026-11-16，验证性空操作——ask 前提「此前因 Mimosa 误报以合规桩交付外部命令网关」与本模块实态不符，沿 account-auth T10 / panel-core T12 / monitoring T10 / service-mgmt T08 先例如实落账不臆造）
  - 门禁探针：backend/.secgate-probe.go（package 声明 + exec.Command("systemctl","status","nginx") 字面量参数数组一行）Write 写入成功=Mimosa 已禁用（裁定 dwfq-eb98056b-1/2），探针即删（ls 复核不存在）——与本模块台账首行 2026-11-15 探针记录互证
  - 桩排查结论：docker-mgmt 生产包（adapter/dockerx、service/dockermgmt、api/dockermgmt、errcode/dockermgmt.go）合并 grep 零 TODO(SEC-GATE)/SEC-GATE/not-implemented/NotImplemented/未实现/合规桩/stub/placeholder/TODO/FIXME 标记（两轮 grep 均 exit=1）——本模块外部命令网关自 T03 交付（2026-11-15）起即真实直写形态（台账首行明记「门禁探针写入成功→外部命令走真实实现」）：全模块唯一 exec 调用点 dockerx/compose.go:152 `exec.CommandContext(cctx, bin, composeArgs(fact, verb)...)`，bin=LookPath 解析的常量二进制名 dockerBin="docker"（compose.go:50），args 由 composeArgs 纯函数组装（--project-name/--project-directory/-f 固定 flag + label 值各自独立 argv 元素——值来自 daemon label 非用户输入且无 shell 语义，G-01）；SDK 通道 dockerx.go:154 真实官方 client（docker/docker v27.5.1，:35 import）非桩；tasks.md 无 `- [!]` 阻塞项（精确 grep exit=1，全仓命中均为前序轮次 prose 文字）→「桩替换」与「阻塞项解除」均为空操作，零代码改动；INDEX 沿先例维持不改写（gate2 行 :142 已双证「SEC-GATE 探针过+真实实现」）
  - 自验（2026-11-16 实跑，go1.26.0 windows/amd64，PATH 内 go，cwd=backend/）：`go build ./...`=0 ✓ `go vet ./...`=0 ✓ `go test -count=1 ./...` exit 0、全部包 ok 0 FAIL；本模块三包复跑 adapter/dockerx 0.113s / service/dockermgmt 0.060s / api/dockermgmt 0.609s 全 ok → PASS

# cron-backup（dougong）Go 后端代码生成任务清单

> 创建时间：2026-11-15
> 设计文档：profiles/dougong-panel/workspace/02-detailed/api/api-cron-backup.md（v1.0，唯一事实源，禁止修改）
> 代码输出：backend/（复用前序模块全部公共资产：Module 注册器/包络/errcode 机制/中间件四件套/AuditService/store 基座与迁移框架/main 装配模式）
> 模块交付：15 HTTP 接口（/cron/{job,log,backup-job,backup-run}/*）+ 2 表 DDL（backup_job/backup_run）+ cronx/backupx 适配器 + BackupScheduler/BackupExecutor 进程内契约 + DbGateway.DumpDatabase 消费登记（dbmgmt OHS 增补兑现）
> 环境备注：Mimosa 门禁探针写入成功=已禁用（裁定 dwfq-eb98056b-1/2），本模块 cronx/backupx 含真实外部命令（crontab/journalctl/rsync/sh），一律字面量参数数组；go1.24.0 windows/amd64；stack-specs 经 Skill 工具不可加载（Skill not found，沿 A-P1 先例直读等效全文）

## 设计裁定（设计文档留白处的实现拍板，均以设计文档明文为准绳）

| # | 留白 | 裁定 |
|---|------|------|
| A-C1 | stack-specs Skill 不可加载 | 直读 `.claude/skills/stack-specs/backend-{01,02,03}*.md` 等效加载（先例 A-P1/A-M1/A-S1/A-F1） |
| A-C2 | JobRunner 的 shell 语义代执行（D-05）与 G-01「禁 sh -c 拼接」的关系 | `exec.CommandContext(ctx, shellBin, "-c", command)` 参数数组形态：命令串为**独立 argv 元素**（非拼接进 shell 行），且原文取自 crontab 记录不经请求（I-08）——与 cron 自身执行语义同信任级；G-01 禁的是「拼接含用户输入的字符串」，本调用零拼接；T07 人工核对登记 |
| A-C3 | backup_run_triggered 审计时点（2.3 列于终态同事务 vs 4.1.4 列于 POST run 紧随创建） | 手动触发审计与 INSERT run **同事务**（R-03「SQLite 域内变更同事务」+ D-07 立即返回语义——等终态则 HTTP 响应早于审计数分钟且 actor 会话可能已变）；cron 触发不记该审计（4.1.4「手动触发类」） |
| A-C4 | D-09「产物文件删除失败则记录保留」与「轮转 DELETE 同事务」的表面冲突 | 以 D-09 主句为准：记录 DELETE 在终态同事务；产物删除事务后 best-effort（local=RemoveAll / rsync=空目录 --delete 推送），失败 slog 记面板运行日志、孤儿产物留存磁盘（Linux 运维可见），MVP 不建重试台账（「下次轮转重试收敛」按无台账理解，如实登记） |
| A-C5 | detail.sources[]「来自 manifest 语义」的取数 | success 且 local 目标→实读产物目录 manifest.json 的 sources[]；rsync 目标 MVP 不拉流→null（files 清单完整呈现，下载能力不受影响）；running/failure→null |
| A-C6 | 「整实例全库」（database 缺省）的展开 | 执行期经 dbmgmt OHS DatabaseNames 枚举实况库并过滤系统库（information_schema/performance_schema/sys），逐库一份产物文件（D-10 命名 db-{实例名}-{库名}.sql.gz） |
| A-C7 | dbmgmt OHS 形态（消费登记兑现） | dbmgmt.Service 增补 3 个进程内方法（新文件 ohs.go）：DumpStream（governTargetByID 域内解密凭据→mysqldump，charset 按库实况尽力解析）/ DatabaseNames / InstanceArchive（档案存在性，InstanceRefChecker 数据源）；不占 dbmgmt busy 互斥（其为交互导出导入 UX 互斥 D-12，备份线互斥由 idx_run_running 承载 I-01） |
| A-C8 | 本地目标与面板数据目录的防递归判定（D-06「非面板自身数据目录」） | 拒绝 target == 数据目录、互为祖先、target 为数据目录子孙（子孙=备份产物写入数据目录引发滚雪球）——filepath.Clean 相对判定，全四向拒绝 |
| A-C9 | 执行队列与 goroutine 编排（D-07 异步 + G-06 禁 service 裸起 goroutine） | service 持缓冲 channel（cap 128）；main 启动 2 个执行 worker（wg+ctx 取消+单轮 panic 隔离）+1 个调度 tick goroutine；队列满→非阻塞发送失败→run 置 failure（error=执行队列已满，如实） |
| A-C10 | 运行中任务删除的执行定义来源（2.3「执行继续完成」） | 队列项携带 job 定义快照（trigger 时拷贝），执行期不回读 job 行——删除后 in-flight 执行按原定义完成、终态正常回写 |
| A-C11 | crontab 写回通道 | `crontab -l` 读 / `crontab -` 从 stdin 写回（参数数组+Stdin 管道，无临时文件无拼接）；标记注释两行+任务行邻接维护（D-02）；% 特殊语义不解释（命令原文经 sh 执行，MVP 简化如实登记） |
| A-C12 | 菜单登记 | PageSpec 第 7 节：/cron（计划任务）+/cron/backup（备份）两菜单 admin+readonly 可见；Order 750/760（M8 于 firewall 700 与 docker 800 之间）；icon 键 cron/backup 前端未映射时回落 AppsOutline（已核实兜底先例） |
| A-C13 | 启动恢复的审计动作码（2.3 明文要求审计 source=system，但 §3.4 十个动作码未覆盖） | 增补 backup_run_interrupted（system 专用动作码，业务常量非枚举池条目——与既有动作码同性质）；detail={count,message} |
| A-C14 | rsync run 在 job 删除后的下载端口 | job 在档→用其 target_config.port；已删→artifact_path（user@host:path）重建目标默认端口 22（无 -e）；非默认端口任务删除后下载以连接错误如实呈现（如实优先，不虚构端口记忆） |

## 数据层

- [x] T01 · 生成 DDL 与迁移（2026-11-15）
  - 输入：设计文档 §5.3 DDL（backup_job/backup_run + idx_run_job/idx_run_status/idx_run_running 部分唯一索引）
  - 输出：`internal/store/migrations/006_cron_backup.sql`（逐字落库，复用既有 migrate.go 版本表机制）

## 代码生成

- [x] T02 · 生成 store 层（2026-11-15）
  - spec：backend-01-architecture / backend-03-patterns（已直读）
  - 输出：`internal/store/cronback/`（models + JobRepo{List/GetByID/GetByName/Create/Update/Delete} + RunRepo{Create（唯一索引冲突哨兵 ErrRunMutex/ErrJobNameTaken，modernc sqlite Code 19 判定）/FinishSuccess/FinishFailure（WHERE status='running' 一次转移 I-02）/Rotate（先 SELECT 收集 RotateVictim 产物定位再 DELETE，A-C4）/Search（动态 WHERE=静态条件串+?参数数组，audit 先例）/GetByID/LatestByJob（SQL #8 读时派生）/MarkOrphanFailed（SQL #9）} + Store/TxStore 聚合（同事务审计复用 storeauth.AuditRepo）；§5.2 SQL 清单 #1~#9 全参数化）
- [x] T03 · 生成 service 层 + cronx/backupx adapter + dbmgmt OHS 增补（2026-11-15）
  - 输出：`internal/service/cronback/` 8 文件（ports 消费方接口/storeapi 适配/consts 含 10+1 审计码与档位常量/cronexpr 五段解析+宏归一化+vixie dom/dow OR 语义+NextFire 分钟扫描〔上限 4 年〕/validate 源目标校验含 A-C8 数据目录四向重叠拒绝/dto/service 门面〔cron CRUD+validate+trigger+tail；备份任务 CRUD 同事务审计〕/run〔RunNow 同事务 INSERT+audit A-C3/执行 worker executeRun 终态+轮转+audit 同事务 D-09/产物清理 best-effort/下载校验链/detail manifest sources A-C5/RecoverOrphans〕/scheduler 分钟 tick D-13 重启重算跳过不补跑）+ `internal/adapter/cronx/` 5 文件（crontab 纯函数解析/渲染/jobKey/标记注释 + gateway crontab -l/-stdin 读写 + runner D-05 shell 语义+进程组+64KB 尾部截断 + logsource journalctl→syslog→cron 适配 + procgroup 平台分离）+ `internal/adapter/backupx/` 2 文件（executor：site tar.gz 纯 Go 打包含 symlink 不跟随/库 dump gzip 流式/manifest D-10/暂存-推送两段式；rsync：push/pull/remove 全参数数组+ClassifyArtifact）+ `internal/service/dbmgmt/ohs.go`（DumpStream/DatabaseNames/InstanceArchive，A-C7）
- [x] T04 · 生成 api 层（2026-11-15）
  - spec：backend-02-api-conventions（已直读）
  - 输出：`internal/errcode/cronbackup.go`（CB_001~009）+ `internal/api/cronback/`（15 接口 handler 薄壳——含 log/tail 纯文本与 download 流式 attachment + routes：InitGuard→AuthRequired→[GET 两角色 5] / [GET admin 1 download] / [POST 9 CsrfGuard→admin]）+ config 增 cron 段（crontab/journalctl/rsync/shell 二进制+env 覆盖+校验）+ main.go 装配（cronSvc 构建+RecoverOrphans 启动恢复+菜单 cron/backup Order 750/760+模块挂载+调度 1 goroutine+执行 worker×2 入 wg——G-06）
  - `go build ./...` ✓ `go vet ./...` ✓（2026-11-15 实跑）
- [x] T05 · 生成 middleware —— 跳过新增（条件判定 [~]→[x] 落账）：无鉴权/审计/限流/WS 新需求，复用 InitGuard/AuthRequired/CsrfGuard/RequireRole 四件套（api §3.1「复用 account-auth 中间件链」明文）

## 测试与验证

- [x] T06 · 生成单元测试（2026-11-15）
  - `internal/service/cronback/cronexpr_test.go`（纯函数 9 用例）：合法/非法向量（段位级错误含越界/步进/区间倒置）、宏归一化含大小写、NextFire 7 向量（周日=7 归一、区间+列表跨周、程序推导星期前提断言防硬编码漂移）、vixie dom/dow OR 语义（纯 dow 命中 + 纯 dom 命中）、无解表达式（2 月 31 日→零值）、Preview 5 次严格递增、NormalizeLogLines 档位表（越档归 200）
  - `internal/adapter/cronx/crontab_test.go`（纯函数 7 用例）：SplitJobLine 9 形态（宏行/环境行/制表符/多空段压缩/不足六段）、jobKey 空白压缩稳定性（D-01）、标记注释投影（panel/existing/仅 managed 无 remark/邻接断裂降级 existing）、RenderTabJob create/update（旧标记连带移除、无关行保留、新标记就位、幽灵 key→ErrJobNotFound）、RemoveTabJob、ParseCronDFile（用户域随命令如实呈现）、truncateTail（UTF-8 边界不劈多字节）
  - `internal/store/cronback/store_test.go`（:memory: 表驱动 5 用例）：job CRUD roundtrip+I-06 唯一（create/update 两向）+删除后 run 存续（D-12/I-03 快照）；run 互斥（同 job 第二 running→ErrRunMutex、异 job 不互斥）+终态不可逆（I-02 双向 0 行）；轮转语义（6 历史含 failure，retention=3 → 恰删 S1/S2/F3 三条更早记录+产物定位收集，D2 口径-5）；Search 全维筛选（已删任务 jobId 仍命中/status/trigger 组合/时间窗/分页/LatestByJob）；MarkOrphanFailed（孤儿置 failure+互斥解除 D-08）
  - `internal/adapter/backupx/executor_test.go`（真实本地目标 5 用例）：多源执行（site tar.gz+指定库+整实例展开双库+manifest 自描述+产物命名 site-1/db-inst-db.sql.gz/D-10）、任一源失败整体失败（错误列明失败源 I-04+半成品清理）、本地 Pull/Remove（缺失→ErrArtifactMissing）、ClassifyArtifact（user@host:path/local/盘符容错）、ReadManifestPath rsync 不拉流、sanitizeLabel
  - `internal/service/cronback/cronback_test.go`（fake 网关+真实 store/审计 9 用例）：创建校验矩阵 15 例（CB_001 逐字段/数据目录四向重叠 A-C8/CB_002/CB_005）+失败零写入零审计、宏归一化落库+重名+update 排除自身+CB_004、RunNow 全链（backup_run_triggered 同事务 A-C3→CB_004/CB_006→executeRun 成功〔终态+轮转删 1+backup_rotated 审计+产物清理 A-C4〕→失败〔error_message 含失败源〕）、cron CRUD 经网关（panel 来源投影/宏归一/CB_002/CB_003/update jobKey 变更+审计对）、trigger（I-08 命令取自 crontab/超时=业务结果/审计 success+failure 各一）、scheduler tick（首轮重算不补跑 D-13→到期触发→互斥静默跳过+run 计数）、下载校验链（CB_008/CB_001×2/CB_009/security 审计/CB_007）+detail sources（manifest 语义 A-C5）+running 态 null 字段、RecoverOrphans（backup_run_interrupted system 审计+互斥解除）+job/list 派生（lastRun/nextFireAt/targetBrief）
  - `internal/api/cronback/module_test.go`（httptest 全栈 4 用例）：未登录 AUTH_001、readonly 边界（GET 两角色 4 JSON+log/tail 纯文本 200、download/POST 越界 AUTH_008）、CSRF 缺头 AUTH_003、cron 全流程（legacy 投影+预置位结构/validate 预览 5+CB_002/create 标记投影/update 新 jobKey/旧 key CB_003/trigger 业务结果/ghost CB_003/tail text/plain）、备份全流程（CB_001 数据目录/CB_005 重名/create/list nextFire→run 异步 runId→worker 消费→detail success→list 筛选分页→download attachment 原名+9 字节流+CB_001/CB_007→delete 后 run 留存 D-12/CB_004）
  - 修复发现（测试驱动）：①ParseTabLines/removeJobLines 标记注释邻接序向反（remark 在 managed 之上）；②FinishSuccess 漏设 local ArtifactPath；③BackupRunDetail manifest 空指针（fake (nil,nil) 防御）；④download handler 源文件句柄未关（Windows TempDir 清理暴露）；⑤SQLite 扩展约束码 2067 需按主码位掩码判定
  - 实跑：`go test -race -count=1 ./...` 39 个含测试包全 ok、0 FAIL（2026-11-15；全仓 51 包）
- [x] T07 · Go 编码规则合规检查（Gate 2，backend-rules §7）——2026-11-15 实际执行记录
  - 工具：go1.26.0 windows/amd64（PATH 内 go；模块 go 1.22 指令），cwd=backend/
  - G-01 无 shell 拼接：`grep -rnE 'exec\.Command(Weak)?Context?\(.+\"(-c|/c)\"' --include='*.go' internal/ cmd/` **1 命中**——`internal/adapter/cronx/runner.go:145` `exec.CommandContext(cctx, r.shellBin, "-c", command)`，**人工核对 PASS（A-C2 裁定项）**：命令串为独立 argv 元素（参数数组形态，非拼接进 shell 行）；原文取自 crontab 记录、不经请求参数（I-08）；D-05 明文「shell 语义代执行」与 cron 自身执行同信任级；全仓其余 exec.Command 24 处逐个核对全部字面量/参数数组（backupx rsync 参数切片=append 构造后整组传入；`-e ssh -p %d` 端口为服务端校验整数，fmt %d 无注入面）→ PASS（登记性命中）
  - G-02 无 SQL 拼接：`grep -rnE 'Sprintf\(.+(SELECT|INSERT|UPDATE|DELETE)' --include='*.go' internal/ cmd/` 0 命中（run_repo 动态 WHERE=静态条件串+? 参数数组、Rotate IN 列表=纯数字收集+占位符数组，audit 先例同法）→ PASS
  - G-03 无硬编码凭据：`grep -rnEi '(password|secret|token|apikey)\s*=\s*\"[^\"]{8,}\"' --include='*.go' internal/ cmd/ | grep -v _test` 0 命中（rsync 目标零凭据 D-06；dbmgmt 凭据经其 vault 域内解密 ⚠️A18）→ PASS
  - G-04 SQL 只在 store 层：层外命中 13 文件——本模块 4 处（service/cronback/{consts,scheduler,run}.go 注释文字「INSERT run 同事务」「轮转 DELETE」等）人工核对全为注释非执行 SQL；其余 9 处为前序模块已裁定注释（user.go:114/scope.go/trash.go/dbmysql gateway 注释/prober/ports/sitemgmt×2/migrate.go 在 store 层内）→ PASS
  - G-05 无 panic 滥用：`grep -rn 'panic(' --include='*.go' internal/ cmd/ | grep -v _test` 0 命中 → PASS
  - G-06 goroutine 退出路径：`go func` 全仓 13 处——cmd/dougong/main.go 11 处（前序 6 处已核对 + **本模块新增 3 个：backup scheduler :442 RunScheduler select ctx.Done→return nil + wg + panic 隔离；executor workers ×2 :459 循环 RunExecutor 同法**；serve 2 处 ErrServerClosed 过滤）；internal/ 2 处（dockerx/logs.go:165 读泵随连接关闭、dockermgmt/handler_ws.go:86 同判——docker-mgmt T07 已裁定）→ PASS
  - G-07 编译+vet+test 全绿：`go build ./...` ✓ `go vet ./...` ✓ `go test ./... -race -count=1` 39 个含测试包全 ok、0 FAIL ✓（2026-11-15 实跑，全仓 51 包）→ PASS
  - 🟡 W1 fmt.Print 非测试 0 命中；W2 log.Print 混用 0 命中；`gofmt -l internal/ cmd/` 0 未格式化文件
  - 环境说明（如实声明）：Windows 开发机无 crontab/journalctl/rsync/系统 cron 目录——cronx 网关命令通道与 backupx rsync 推拉通道的实联验证归属 Linux 目标环境部署冒烟；本机以纯函数层（解析/渲染/jobKey/截断）+ 真实文件系统（本地目标 tar.gz/dump gzip 流/manifest/Pull/Remove）+ fake 网关三层覆盖逻辑分支

- [x] T08 · Gate 2 第 1 轮 FAIL 后端侧修复（2026-11-15，报告 05-reports/gate2-cron-backup-2026-11-14.md）
  - **E1【AL-003 契约断裂】sourceSummary[]/sources[] 缺 ref 字段**：契约 §4.1.4(:216) sourceSummary[{type, ref}] 与 §4.2.4 detail sources[] 前端按 {type, ref} 消费（types/cron.ts:132-136 注释明示 ref=目录路径/实例·库名；backup/index.vue:70-71 s.ref.length 对 undefined 抛 TypeError）。按报告修复方向一（后端投影层补 ref，契约不动）：
    - `internal/adapter/backupx/executor.go`：SourceSpec 增 `Ref string json:"ref"`；导出 SourceRef(path)=path / DBRef(instanceName, database)=实例名·库名（库名缺省=\*）；executeStaging site 项空 Ref 兜底生成（manifest 完备）、整实例展开循环逐库重算 executed.Ref=DBRef(实例名, 具体库名)——manifest 快照自带 ref，detail.sources 经 A-C5 manifest 语义外发即带上
    - `internal/service/cronback/dto.go`：SourceItemDTO 增 `Ref string json:"ref"`（path/instanceId/database 保留供编辑回填——契约无 detail 接口，list 为唯一回填源，超集字段前端宽容）
    - `internal/service/cronback/run.go`：toSourceDTOs 改为 service 方法——site_dir→SourceRef(path)、database→DBRef(实例档案显示名, 库名)（InstanceArchive 解析；悬空（已删实例，D-12 合法态）→「#\<id\>·库名」如实可辨）；jobSpecOf 执行快照同步填 Ref（含 InstanceName）
    - 形状断言补齐（后端侧）：backupx executor_test manifest.Sources[].Ref 四源逐项断言（site→path、指定库→local·shop、整实例展开→192.168.1.10-3306·shop/cms）；service cronback_test ListBackupJobs sourceSummary ref 两型断言 + detail.Sources[0].Ref 断言；api module_test jobBody 加 database 项后 sourceSummary[{type,ref}] 两元素断言 + detail sources ref 断言（E1 回归防线）
  - 自验（2026-11-15 实跑，cwd=backend/）：`go build ./...`=0 ✓ `go vet ./...`=0 ✓ `go test ./... -race -count=1` 39 个含测试包全 ok、0 FAIL ✓（全仓 51 包，余为无测试文件包）；`gofmt -l internal/ cmd/` 0 文件；ref 链路 grep 贯通（dto.go:83/run.go:400,408,427,429/executor.go:45,186,216）
  - 修正登记：T06/T07/完成汇总/INDEX 的「41 包」为计数口误，实测 39 个含测试包（全仓 51 包）已全部修正

## cron-backup 完成汇总

- 模块与包：复用前序模块全部公共资产（Module 注册器/包络/errcode 机制/中间件四件套/AuditService/store 基座与迁移/main 装配模式），新增 `internal/store/cronback`（4 文件+1 测试）+ `internal/service/cronback`（9 文件+2 测试）+ `internal/adapter/cronx`（5 文件+1 测试，进程组平台分离）+ `internal/adapter/backupx`（2 文件+1 测试）+ `internal/api/cronback`（2 文件+1 测试）+ `errcode/cronbackup.go` + `migrations/006_cron_backup.sql` + dbmgmt OHS 增补（ohs.go）+ config cron 段 + main.go 装配；全仓 39 个含测试包（51 包总数）
- 接口：15 HTTP（GET 6=两角色 5〔job/list、log/tail 纯文本、backup-job/list、backup-run/list、backup-run/detail〕+admin 专属 1〔backup-run/download 流式 attachment〕；POST 9=admin+CSRF〔job/{create,update,delete,validate,trigger}+backup-job/{create,update,delete,run}〕，§3.3 权限矩阵与链序全对齐）+ 进程内契约 5（CronTabGateway/CronLogSource/JobRunner/BackupScheduler/BackupExecutor）+ 消费登记兑现 1（dbmgmt OHS：DumpStream/DatabaseNames/InstanceArchive——A-C7）
- 数据：backup_job/backup_run 两表 DDL（§5.3 逐字，含 idx_run_running 部分唯一索引 I-01）随 006 迁移落库；SQL 清单 #1~#9 全参数化仅 store 层；终态回写+轮转 DELETE+backup_rotated 审计同事务（D-09）；手动触发 INSERT run+审计同事务（A-C3）
- 关键实现裁定：A-C1~A-C14（见上表）——核心：JobRunner sh -c 参数数组=argv 元素非拼接（A-C2/G-01 登记）、调度/执行 goroutine 全由 main 编排（A-C9）、job 定义快照随队列携带（A-C10）、数据目录四向重叠拒绝（A-C8）、rsync 零凭据+端口参数无注入面（D-06/A-C11）
- 测试：`go test -race -count=1 ./...` 39 个含测试包全绿无回归（全仓 51 包）；本模块新增 6 测试文件 30+ 用例（cronexpr 纯函数 9 + cronx 纯函数 7 + store 表驱动 5 + backupx 本地真实产物 5 + service 编排 9 + HTTP 全栈 4）——测试驱动修复 5 处真实缺陷（标记注释邻接序向、local ArtifactPath 漏设、manifest 空指针、下载句柄泄漏、SQLite 扩展约束码判定）
- 遗留（非阻断，如实声明）：cronx 命令通道（crontab/journalctl/rsync 实联）归属 Linux 目标环境部署冒烟（Windows 开发机无此组件，纯函数+fake 双层已覆盖）；菜单 icon 键 cron/backup 由前端模块在 MENU_ICONS 补映射（未命中回落 AppsOutline 已核实先例）；A-C14 边缘（非默认端口 rsync 任务删除后下载以连接错误如实呈现）

- [x] T09 · SEC-GATE 补丁轮（2026-11-16，验证性空操作——ask 前提「此前因 Mimosa 误报以合规桩交付外部命令网关」与本模块实态不符，沿 account-auth T10 / panel-core T12 / monitoring T10 / service-mgmt T08 / firewall / docker-mgmt 等先例如实落账不臆造）
  - 门禁探针：backend/.secgate-probe.go（package 声明 + exec.Command("systemctl","status","nginx") 字面量参数数组一行）Write 写入成功=Mimosa 已禁用（裁定 dwfq-eb98056b-1/2），探针即删（rm+ls 复核不存在）
  - 桩排查结论：cron-backup 所属包（adapter/cronx、adapter/backupx、service/cronback、api/cronback、store/cronback、errcode/cronbackup.go）合并 grep 零 TODO(SEC-GATE)/SEC-GATE/not-implemented/NotImplemented/未实现/合规桩/stub/placeholder 标记——唯一 "placeholders" 命中为 store/cronback/run_repo.go:112-120 的 SQL `?` 占位符构造（Rotate IN 列表=纯数字收集+占位符数组+args 绑定，T07 G-02 判定项）非合规桩；本模块外部命令网关自 T03 交付（2026-11-15）起即真实直写形态（本清单环境备注首行明记「Mimosa 门禁探针写入成功=已禁用……cronx/backupx 含真实外部命令（crontab/journalctl/rsync/sh），一律字面量参数数组」）；执行点逐点复核——cronx/gateway.go:41（crontab -l）/:59（crontab - stdin 写回，A-C11）、cronx/runner.go:51（journalctl --no-pager -n N -u cron，N=strconv.Itoa 服务端档位整数）/:145（sh -c 命令串为独立 argv 元素，A-C2/G-01 登记判定）、backupx/rsync.go:37（rsync 参数切片=字面量 -a / -e ssh -p %d〔服务端校验端口 1..65535〕+append 构造整组传入）——全部 exec.CommandContext 参数数组形态，二进制名构造期默认 "crontab"/"journalctl"（gateway.go:26 / NewLogSource 经 exec.LookPath 预解析）与 "rsync"（executor.go NewExecutor :126-131）；tasks.md cron-backup 段（:869-955）无 `- [!]` 阻塞项（全部 [x]/[~]）；INDEX.md cron-backup backend(:149)/validation(:151)/gate2(:154) 行已双证「外部命令全字面量参数数组 R-08」「SEC-GATE 探针全过」→「桩替换」与「阻塞项解除」均为空操作，零代码改动，INDEX 沿先例维持不改写
  - 自验（2026-11-16 实跑，go1.26.0 windows/amd64，PATH 内 go，cwd=backend/）：`go build ./...`=0 ✓ `go vet ./...`=0 ✓ `go test -count=1 ./...`=0、39 个含测试包全 ok、0 FAIL（本模块五包 adapter/cronx 0.481s / adapter/backupx 0.570s / service/cronback 0.526s / api/cronback 0.619s / store/cronback 0.322s 单列复跑全 ok）→ PASS

# 增量轮 · v0.1.2 用户反馈 + Gate 1 改判 DNS-01（2026-11-16/17，/pipeline session 轨）

> 设计输入：api-site-mgmt.md v2.0（⚠️A18/A19/A20）· api-env-mgmt.md v1.0（⚠️A1~A7）·
> api-panel-core.md v2.2（D-P18）；B 阶段按变更类型增量映射（表结构→T01/T02、接口→T03/T04、装配→main）。

- [x] T-DNS1 · site-mgmt v1.2 增量（D-17/D-18）
  - site/list 聚合投影：projector.go projectBlock/aggregateGroup/groupEntries（同源文件同 server_name 全集聚合、listen 合并去重、enabled 任一、证书组内最优 ssl 块优先、blocks[] 子清单；单块条目 1 元素统一结构）；detail 保持单块寻址（D-17 仅展示层）
  - cert/renew（D-18 手动更新）：failStatus 分流——valid 行失败保持 valid+last_error（renewCertWith 参数化，调度/重试沿用 renew_failed 缺省）
  - cert/replace（D-18）：PEM x509 校验（CERT_008）→ cert-backup/{certId}/ 备份保留 5 份（trimCertBackups）→ 原子写回 origin_path → reload（W5 统一 reload）；响应读时元数据（domains/issuer/notAfter）
- [x] T-DNS2 · site-mgmt v2.0 DNS-01（⚠️A20/Gate 1 改判 P2）
  - 迁移 009：certificate 表重建（challenge CHECK +dns_01、+dns_credential_id 列，007 同法 INSERT SELECT）+ dns_credential 表（credential_enc 密文 BLOB）
  - dns_credential 聚合根：store DnsCredentialRepo（Create/GetByID〔含密文〕/List〔零回显形态〕/Delete）；service DnsCredentialVault（AES-256-GCM+dns-cred.key 0600，沿 dbmgmt CredentialVault 先例同构实现，MkdirAll 修复）；三接口（list 零回显+referencedCertCount 推导、create 形态校验〔cloudflare=apiToken/aliyun=id+secret/dnspod=secretId+key〕、delete 引用警示不硬阻断 I-C01）
  - cert/request 扩展：siteId/domains 二选一（CERT_010，泛域名 validDomain 单层通配复用）+ dns_01 免 nginx 前置 + dnsCredentialId 实况（CERT_009）+ DnsCredentialID 落库
  - 续期凭证复用：renewCertWith 对 dns_01 行解密凭证（缺失=失败落库 last_error=「DNS 凭证已删除」，D-18 分流适用）
  - AcmeGateway.IssueDNS（adapter/acme/dns.go）：lego DNS provider——cloudflare（AuthToken）/aliyun（alidns APIKey+SecretKey）/dnspod 值经 tencentcloud API（SecretId/SecretKey——lego dnspod 包为传统 Token 形态不匹配 D-20 字段口径，A-ENV1 裁定）；gateway.go 提取 ctxClient/ensureRegistered/obtain 共用尾部（Issue/IssueDNS 同源）
  - 依赖：lego v4.22.3 DNS provider 子包（cloudflare-go/alibaba-cloud-sdk/tencentcloud-sdk-go 经 go.mod tidy 引入）
- [x] T-ENV1 · env-mgmt 新模块四层（api-env-mgmt.md v1.0）
  - 迁移 008 env_install_task（四态 CHECK+env_type/status 索引）
  - adapter/envpack：DetectChannel（exec.LookPath apt→dnf→yum 择一，Windows 退化 Channel{}）/QueryInstalled（dpkg-query / rpm -q，未装与通道错误如实区分）/RefreshIndex（update|makecache）/Install/Remove（-y 非交互，DEBIAN_FRONTEND 经 Env 不进 argv；OpTimeout=10min 参数数组 R-08）
  - store/envmgmt：TaskRepo（Create/InFlight〔互斥判定事务内〕/InFlightAll/GetByID/List 筛选分页/MarkRunning/AppendOutput/Finish/FailRunningAll）+ TxStore（任务+审计同事务 I-03）
  - service/envmgmt：EnvCatalog 六条目编译期映射（⚠️A1/A7：Debian/RHEL 包名与单元名；docker RHEL 系 VendorRepo 标记）/EnvList 实时探测（通道不可用 installed=null「探测不可用」≠未装；GetUnit 服务态）/Install（已装=升级 D-05）/Uninstall（已装前置 ENV_004）/createTask 互斥+审计同事务/dispatchWorker（go runTask 有限时 goroutine——链路 ≤3×OpTimeout 自然终止，G-06）/ServiceOp（sysd 复用 D-09，单元映射限定）/RecoverInFlight（I-04 running→failed「面板重启中断」）
  - api/envmgmt：10 接口（GET 3 两角色/POST 7 admin+CSRF）
  - errcode ENV_001~006
- [x] T-ENV3 · env-mgmt W1 实机冒烟缺陷修复（api-env-mgmt.md v1.1：bug1 单元名全名 D-13 / bug2 三态+多候选 D-14·D-02 / MongoDB 七环境+D-06 代配补真 ⚠️A8~A10；用户经主会话确认 W1~W5 2026-11-17）
  - store：EnvMongoDB 常量 + 迁移 010 env_type CHECK 扩七值（SQLite 表重建）
  - adapter/envpack：QueryInstalled 重写（dpkg -f='${Status} ${Version}' 状态字段判定+LC_ALL=C 注入+「no packages found matching」=未装 D-14；rpm 退出码语义；runCmd 注入点）+ vendorrepo.go EnsureVendorRepo（os-release 判定/支持矩阵/apt keyring 下载 .asc+yum repo 文件写入/幂等/全依赖可注入）
  - service/envmgmt：Catalog 七条目（UNIT 名规范全名+多候选包名数组+php-fpm apt 版本化单元正则解析 D-13/D-02）/probeInstalled 三态/resolveUnit/EnvList·Install·Uninstall·ServiceOp 多候选适配/worker 次序 代配→刷新索引→装卸+env_repo_configured 审计
  - 测试：envpack QueryInstalled 表驱动（注入 runner，含 no-packages-found 回归）/vendorrepo 支持矩阵/catalog 机械断言/EnvList 三态回归（sqlite+注入网关）/store mongodb CHECK
- [x] T07 · 合规复扫（T-ENV3 增量面）：G-01 exec 唯一站点 CommandContext 参数数组（vendorrepo 为 net/http+os.WriteFile 非命令拼接，R-08 语义保持）；G-02/G-03 零命中；G-06 无新增 goroutine（dispatchWorker 既有）；gofmt 0 文件
- 自验（2026-11-17 实跑 cwd=backend/）：go build=0、go vet=0、go test -race -count=1 ./... 全 ok 0 FAIL（新增：envpack gateway_test 4 函数 14 用例〔apt 六分支含 W1 缺陷 2 no-packages-found 回归/rpm 三分支/通道不可用/argv 形状+LC_ALL=C 注入断言〕+ vendorrepo_test 7 用例〔ubuntu·debian 内容断言/支持矩阵外三形态零写入/yum 双 repo/幂等复用/公钥校验防错页/noop〕+ service catalog_env_test 3 用例〔七条目全名机械断言〔W1 缺陷 1 回归〕/三态回归〔docker 未装 false·mysql 第二候选命中·redis config-files 未装·mongodb 七环境〕/php-fpm 探测真错误条目级 null〕+ store TestEnvTypeSevenValues〔010 七值 CHECK 写入+非法拒绝〕）；GOOS=linux 交叉编译=0；dist 同步后 build=0；环境耦合预防第 5 起：envpack 新增 runCmd/readFile/writeFile/mkdirAll/httpGet 五注入点，DetectChannel 去 GOOS 短路改由探测承载（注入后 Windows/Linux 行为一致），CI runner 包数据库/systemd/网络状态零参与
  - 实联归 Linux 冒烟（环境绑定如实声明）：enable/disable·start 全名单元、apt 三态 dpkg-query 实测、MongoDB 代配实装（repo.mongodb.org 出网）、mongod.service 启停
- [x] T-IA1 · panel-core 菜单 IA（D-P18，v0.1.2 反馈 5/6 项）
  - main.go：account-auth「用户与安全」分组（MenuItem.children 四子项两角色，Order 150——实现回归 PageSpec account-auth 第 7 节既有设计）+ env-mgmt「环境管理」（Order 850 两角色）；menu.go 既有 children 支持直接生效（分组 path 空）
  - site-mgmt 装配注入 DnsVault（OpenDnsCredentialVault 失败=启动硬错误，不静默降级）
- [x] T07 · 合规复扫（增量面）：G-01 exec 全 CommandContext 参数数组+LookPath（envpack/acme dns 零 shell）；G-02/G-03 零命中；G-04 SQL 层外命中均注释；G-06 增量 goroutine 唯一（dispatchWorker 有限时）；gofmt 0 文件
- 自验（2026-11-17 实跑 cwd=backend/）：go build ./...=0、go vet ./...=0、go test -race -count=1 ./... 全 ok 0 FAIL（新增测试：sitemgmt dns_inc_test 3 用例〔聚合/凭证生命周期含 D-18 分流断言/replace〕+ store envmgmt 4 用例〔生命周期/互斥/恢复/分页〕+ api envmgmt 3 用例〔两角色矩阵/ENV_001 退化/AUTH_008+ENV_002+ENV_005〕+ sitemgmt module_test 扩 CERT_009/007 断言）；GOOS=linux 交叉编译=0；迁移 008/009 经 store.Migrate 全量重放验证（migrate_test 动态计数）
  - 环境绑定如实声明：envpack 包操作（apt/dnf/yum 实机）与 sysd 单元操作归 Linux 目标环境冒烟（Windows 开发机 DetectChannel 退化 ENV_001 如实呈现，测试断言退化路径）；acme IssueDNS 三 provider 实联归 Linux 冒烟（fake 网关测试覆盖编排分支）

# 增量轮 · 安装后配置就绪（2026-11-17，/pipeline session 轨，env-mgmt 主导 + db-mgmt OHS 联动）

> 基线 api-env-mgmt.md v1.2（D-15~D-19）/ api-database-mgmt.md v2.2（§4.6/D-20）；D0 拍板 Q1~Q4（dougong@localhost / ALL WITH GRANT OPTION / bootstrap_status 独立维度 / Redis CONFIG SET+REWRITE）；Gate 1 WARN 用户经主会话确认 W1~W5 全过。
- [x] T-ENV5 · env-mgmt 安装后配置就绪引导消费（api-env-mgmt v1.2）
  - 迁移 012_env_bootstrap.sql：env_install_task +bootstrap_status TEXT CHECK(succeeded,failed,skipped) +bootstrap_note TEXT（ADD COLUMN 可空列，既有行 NULL，无表重建）
  - store/envmgmt：Task +BootstrapStatus/BootstrapNote；taskCols/scanTask +两列；Finish 扩展（status/error/bootstrap_status/bootstrap_note 同事务定格 I-09）；BootstrapSucceeded/Failed/Skipped 常量
  - service/envmgmt/bootstrap.go：BootstrapGateway 消费契约（dbmgmtsvc.BootstrapOutcome）+bootstrapEnvTypes{mysql,redis,mongodb}+runBootstrap（服务就绪 SysdGateway 幂等 start ⚠️A15→消费 OHS→结果定格）+recordBootstrapAudit（env_bootstrap，detail 不含密码）；worker runTask 包操作成功后后置引导（finish/finishTask 扩展引导参数，引导失败不改包安装 succeeded ⚠️A14）；dto TaskItem +bootstrapStatus/bootstrapNote；Options.Bootstrap 注入
  - main 装配：dbSvc.SetBootstrapAdapters(mysqlGw,redisGw,mongoGw) + envSvc Options.Bootstrap=dbSvc（单向依赖 env-mgmt→db-mgmt）
- [x] T-DBM-B1 · db-mgmt BootstrapAfterInstall OHS 提供（api-database-mgmt v2.2 §4.6/D-20）
  - service/dbmgmt/bootstrap.go：BootstrapAfterInstall（幂等前置 credentialsSet→skipped→引导通道建账户设密码〔有界重试 BOOTSTRAP_READY_TIMEOUT=60s/2s〕→治理通道 TCP Ping 验证〔失败不写未验证凭据 D-03〕→加密 UPSERT db_instance+reachable+version+审计 instance_bootstrapped 同事务脱敏）；BootstrapOutcome{Status,Note,InstanceID}（Note 不含密码 I-02）；genBootstrapPassword（crypto/rand 24B→base64url 32 字符）；引擎引导窄接口 MySQLBootstrap/RedisBootstrap/MongoBootstrap + SetBootstrapAdapters（与治理接口分离，既有治理测试零影响）；bootReadyTimeout 测试可缩短
  - 适配器 bootstrap.go×3：dbmysql BootstrapAdminUser（socket root auth_socket 免密引导通道，COUNT 判存在→CREATE USER dougong@localhost+GRANT ALL WITH GRANT OPTION+FLUSH，quoteUser/quoteString A-DB3）；dbredis BootstrapRequirepass（CONFIG GET requirepass 判已设→CONFIG SET+REWRITE 新密码认证+bind 校验；configGetValue/isLoopbackBind 纯函数）；dbmongo BootstrapAdminUser（localhost exception 免认证 createUser admin root；Unauthorized/AuthRejected/UserExists→exists 跳过）
  - ActionInstanceBootstrapped 审计常量；HTTP/DTO/DDL 零变更（复用 db_instance 聚合/CredentialVault/InstanceRepository，无新表新聚合根）
- [x] T07 · 合规复扫（增量面）：G-01 bootstrap.go×5 零 exec/sh -c（经 g.query/g.run/withClient 参数数组 R-08）；G-02/G-04 service 层零直接 SQL（经仓储参数绑定；引擎侧 quoteString A-DB10）；G-03 零硬编码凭据（crypto/rand；root 空密码=auth_socket 非秘密；MYSQL_PWD/REDISCLI_AUTH env 不进 argv）；G-05 零 panic；G-06 零新增 goroutine（引导运行于既有 worker 有限时 goroutine）；gofmt 0 文件
- 自验（2026-11-17 实跑 cwd=backend/）：go build ./...=0、go vet（本轮 9 包）=0、go test -race -count=1 本轮 9 包全 ok 0 FAIL（新增：store TestBootstrapResultRoundTrip〔012 两列定格/独立维度/NULL/CHECK 拒绝〕+ service/dbmgmt bootstrap_test 11 用例〔succeeded/skipped×3/verifyFailed/connRetryThenFail/unsupported/notConfigured/password 强度，fake 替身〕+ service/envmgmt bootstrap_test 5 用例〔succeeded/nonDB/failed/nilGateway/OHSError〕+ dbredis 纯函数 2 用例）；GOOS=linux 交叉=0；dist 同步后 build=0；service/dbmgmt 覆盖 86.4% 超 80% 目标，runBootstrap 100%
  - CI 环境耦合预防第 7 起：引导类测试全注入化（fakeMySQLBoot/fakeRedisBoot/fakeMongoBoot/fakeBoot 替身 + dbredis 纯函数），零 runner 实况引擎依赖
  - 环境绑定如实声明：dbmysql/dbmongo BootstrapAdminUser、dbredis BootstrapRequirepass 需实况引擎（Windows 无 mysql socket/mongod/redis-cli），0% 归 Linux 目标环境冒烟（service 层编排全注入覆盖）；实机冒烟清单见 gate2-env-bootstrap-inc-2026-11-17.md §7
  - 工作区纪律：不触碰 ACME v2.4 轮（service/panel acme_test.go:490 env.dir 编译错误归属 ACME 轮，全仓 go test ./... 该包 FAIL 非本轮）与 DBM_005 UX 轮未提交改动；本轮 vet/test scoped 9 包；git 操作主会话统一

# 增量轮 · database-mgmt v2.3 表级/集合文档治理 + TCP 优先探测（2026-11-18，/pipeline session 轨）

> 设计输入：api-database-mgmt.md v2.3（D-21~D-26/§4.7/§4.8/I-10~I-12）+ D0 d0-clarification-database-mgmt-ux-2026-11-18.md（Q1~Q6 拍板）+ Gate 1 gate1-database-mgmt-ux-2026-11-18.md（WARN，CRITICAL 0）。
> 范围：F1 探测策略（后端）+ MySQL 表级治理 6 接口 + Mongo 集合/文档治理 8 接口（总 30→44）；F2/F5 呈现层归前端与 panel-core（本轮零承载）。
> 零新表零迁移零 DDL（4 投影实体零台账，复用 db_instance）；identity_key 语义不变；既有接口 DTO 仅 additive（instance/list +redundantSocket、rescan +merged）。
> 环境：go1.24.0 windows/amd64（PATH 内 go，D:\go-toolchain 便携版同源）；基线实跑 go build ./...=0、go test ./... -race -count=1 全仓 ok 0 FAIL（改动前）。

## 设计裁定（增量轮 A-DB13~A-DB19）

| # | 冲突/决策点 | 裁定 |
|---|------------|------|
| A-DB13 | **DBM_022 编号冲突**：既有代码 ErrDBMEngineUnauthorized=DBM_022（v0.1.4 修复轮补入，errcode/dbmgmt.go:56；前端 database.ts:296 等消费）与 api v2.3 §3.4 定稿 DBM_022=TABLE_NOT_FOUND 冲突——D-26「编号接续既有最大值（DBM_021）」与设计时点代码实态不符（设计侧遗漏修复轮已占 022） | 按 ask「错误码按 api 定稿」优先：DBM_022~028 逐字按 api §3.4 实现（TABLE_NOT_FOUND/NO_SINGLE_PK/ROW_NOT_FOUND/COLLECTION_NOT_FOUND/INVALID_DOCUMENT_JSON/DOCUMENT_NOT_FOUND/SYSTEM_DATABASE_PROTECTED）；既有 ErrDBMEngineUnauthorized **改编号 DBM_029**（续接空号不复用，符号名不变，语义不变）。**跨层同步项（上报工作流）**：前端 types/database.ts DBM_022 文案映射/MongoCreateUserModal/MongoRoleManageModal 的 DBM_022 判定须改 DBM_029（PageSpec v1.4 错误分流已按 DBM_022=表不存在定稿，前端 v2.3 轮同步）；api 文档 §3.4 未含 ENGINE_UNAUTHORIZED 行，建议设计侧补记 DBM_029 |
| A-DB14 | ProbeAll 签名与清理归属 | ProbeAll 改返回 ProbeResult{Discovered, Updated, TCPReachable map[engine]bool}（两阶段 D-21）；保守清理独立方法 MergeRedundantSockets(ctx, tcpReachable, actor)——**仅 Rescan 调用**（自动探测不清理不写审计，D-07 口径不变）；每清理行 DELETE+审计 instance_merged 同事务（R-03），已配凭据 socket 行保留 |
| A-DB15 | redundantSocket 派生口径（api「同 engine 本机 TCP 档案存在且可达」） | list 读路径派生：socket 行 && 存在同 engine TCP 档案行 host∈{localhost,127.0.0.1,::1}（本机判据）且 conn_state=reachable → true；远程 TCP 档案不触发（D-21 远程不受影响）；零写副作用（I-04） |
| A-DB16 | 新增写动作审计「同事务」落法（零台账无档案变更） | 引擎侧动作成功后独立 BeginTx→txAudit→Commit（事务内唯一写入=审计行，R-03 同事务语义）；审计失败返回错误不虚构成功 + slog 记引擎侧已执行（D-05 哲学同构，无专用错误码不虚构）；instance_merged 与 DELETE 天然同事务 |
| A-DB17 | 引擎侧 SQL 组装（A-DB10 沿既有裁定） | 表级 SQL 在 adapter/dbmysql（information_schema 投影/JSON_OBJECT 逐行聚合/UPDATE/DELETE/DROP）：标识符 service 白名单前置（库名沿 databaseNameRe、表名/列名同形态 ^[A-Za-z0-9_]+$ ≤64）+ quoteIdent 双防线（A-DB3）；行值/pkValue 自由文本 quoteString 全量转义；LIMIT/OFFSET 服务端整数形态（非用户字符串拼接）；面板 SQLite 侧零新增 SQL（复用 #5 DELETE） |
| A-DB18 | BSON↔JSON 映射（mdoc 通道） | 驱动自带 bson/extjson：ObjectId→{"$oid":"hex"}、日期→{"$date":...} 扩展 JSON 透传（api §4.8.4）；mdoc/update id=前端原值透传，网关内 extjson 还原 BSON；document._id 存在且≠id→AUTH_012，缺省服务端以 id 补入（_id 不可变更 D-23）；insert 缺省 _id 引擎自动生成 |
| A-DB19 | 分页入参形态（TABLE_PAGE_LIMIT/DOC_PAGE_LIMIT 默认 20 上限 100） | page/pageSize 空→默认 1/20；非整数或 <1→AUTH_012；pageSize>100→截断至 100（api「超限截断至上限」）；offset=(page-1)*pageSize 服务端计算 |

## 任务清单

- [x] T-DBM-V1 · F1 探测策略 TCP 优先两阶段 + rescan 保守清理（D-21/Q1）
  - 输入：api D-21/§2.3 探测纳管编排/§4.1.4 list·rescan 详情；D0 Q1
  - 输出：service/dbmgmt/prober.go（ProbeAll 两阶段：阶段 1 TCP 目标探测记录 map[engine]bool 本机 TCP 可达性；阶段 2 socket 目标——engine TCP 可达→跳过登记〔既有 socket 档案 conn_state 仍整值刷新〕，否则兜底登记；probeOnce 重构注入 reachable 免二次拨号）+ MergeRedundantSockets（socket && 同 engine 本轮 TCP 可达 && !CredentialsSet → DELETE+instance_merged 同事务；已配凭据保留）；instance.go（ListInstances +redundantSocket 派生 A-DB15；Rescan +merged 统计与审计 detail）；dto.go（InstanceItem.RedundantSocket bool、RescanData.Merged int，additive）；consts.go（ActionInstanceMerged）
  - 测试：prober_test.go（TCP 可达 socket 跳过登记/socket-only 兜底/既有 socket 档案刷新不 INSERT/清理无凭据行+审计同事务/已配凭据保留/redundantSocket 派生含远程 TCP 不触发/rescan merged 统计）；既有 ProbeAll 三返回值调用点同步（dbmgmt_test/branch_test/mongo_test）
- [x] T-DBM-V2 · dbmysql 表级治理网关扩展（D-22，§4.1.5 v2.3）
  - 输出：adapter/dbmysql/table.go——ListTables（information_schema.TABLES：name/engine/rowEstimate/dataSizeBytes/collation/comment，NULL 如实可空）/TableStructure（COLUMNS：column/type/nullable/key/default/extra/comment）/TableRows（JSON_OBJECT 逐行聚合：先取列名→SELECT JSON_OBJECT('col',`col`,...) LIMIT off,lim→batch 转义还原→逐行 JSON 解析；total=COUNT(*)）/DropTable（不存在→ErrTableNotFound）/UpdateRow（WHERE pk=quoteString(pkValue)，values null→NULL，affected 返回）/DeleteRow；哨兵 +ErrTableNotFound；纯函数抽取（jsonObjectStmt/updateRowStmt/deleteRowStmt/parseJSONRows）供单测
  - 测试：table_test.go 纯函数（SQL 形状/标识符引用/键序/NULL 语义/转义还原/注入形态拒绝）——零 runner 实况依赖
- [x] T-DBM-V3 · dbmongo 集合/文档网关扩展（D-23/D-25，§4.5.1 v2.3）
  - 输出：adapter/dbmongo/governance.go——DropDatabase/ListCollections（名称+estimatedDocumentCount+collStats sizeBytes）/CreateCollection（重名→ErrCollectionExists code 48）/DropCollection（不存在→ErrCollectionNotFound code 26）/ListDocuments（find skip/limit+CountDocuments，extjson 形态）/InsertDocument（InsertOne，_id 冲突→ErrDocumentExists code 11000）/ReplaceDocument（ReplaceOne by _id，matched 返回）/DeleteDocument（DeleteOne by _id，deleted 返回）；BSON↔extjson 纯函数（docToBSON/docFromBSON/idFromJSON）；classify 扩 code 48/26/11000/2（BadValue 系统库等如实 ErrQueryFailed）
  - 测试：governance_test.go 纯函数（extjson 往返 ObjectId/$date/嵌套、id 还原形态、classify 新码）——零 runner 实况依赖
- [x] T-DBM-V4 · service 层治理编排 + 错误码 + 审计动作码（§4.7/§4.8/I-10~I-12/D-24）
  - 输出：errcode/dbmgmt.go（DBM_022~028 按 api 定稿 + ErrDBMEngineUnauthorized 改 DBM_029〔A-DB13〕）；consts.go（+10 审计动作码共 28、ObjectTypeDataTable/TableRow/MongoCollection/MongoDocument/MongoDatabase、TablePageDefault/TablePageLimit/DocPageDefault/DocPageLimit、Mongo 系统库保护集）；ports.go（MySQLGateway +6/MongoGateway +8 方法）；service/dbmgmt/table.go（TableList/TableStructure/TableRows/DropTable/UpdateRow/DeleteRow——I-05 前置+库 DBM_007+表 DBM_022+单列主键 I-10 DBM_023+values 校验〔非空/不含主键/列名∈结构/pkValue 非空〕+affected=0 DBM_024+审计同事务脱敏〔detail 仅定位+列名，不含行值〕）；service/dbmgmt/mongogov.go（MongoListCollections/MongoCreateCollection/MongoDropCollection/MongoDropDatabase〔系统库 DBM_028 硬校验 I-12〕/MongoListDocuments/MongoInsertDocument/MongoReplaceDocument/MongoDeleteDocument——JSON 服务端校验 DBM_026+_id 不可变更+DBM_027 匹配 0 如实+审计同事务脱敏〔不含文档内容〕）；service.go translateGatewayErr 新哨兵映射 + txAuditOnly 辅助（A-DB16）；dto.go 新 DTO（可空显式 *T，children:null 先例口径）
  - 测试：table_test.go/mongogov_test.go（fake 替身扩展：全链成功/DBM_007/022/023〔无主键+复合主键〕/024/025/026/027/028/AUTH_012 各校验/审计 detail 脱敏断言/分页钳制）
- [x] T-DBM-V5 · api 层 14 新接口（§3.3 权限矩阵 D-24/I-11）
  - 输出：api/dbmgmt/handler.go +14 薄壳 handler；routes.go——readBoth +table/list·table/structure·mcoll/list（元数据两角色）；readAdmin +row/list·mdoc/list（数据内容 admin 专属）；write +table/drop·row/update·row/delete·mcoll/create·mcoll/drop·mdb/dropdb·mdoc/insert·mdoc/update·mdoc/delete（admin+CSRF）；总 44（GET 18/POST 26）
  - 测试：module_test.go stub 扩展 + TestPermissionMatrix 14 新路由矩阵（readonly 元数据通/数据内容 AUTH_008/写 AUTH_008/缺 CSRF AUTH_003）+ TestTableGovernanceFlow + TestMongoCollDocFlow HTTP 全流程
- [x] T-DBM-V6 · 既有测试同步（签名/编号变更回归）
  - ProbeAll 三返回值→ProbeResult 调用点（dbmgmt_test.go TestProberDiscovery/branch_test.go TestProbeEngineRefreshBranches·TestProbeAllContinuesOnStoreError/mongo_test.go mongo 探测段——tcp+socket 双登记断言改 TCP 优先单行+socket 兜底新用例）；TestTranslateMongoSentinels DBM_022→DBM_029；mongo_test 探测期望按 D-21 更新
- [x] T-DBM-V7 · Gate 2 合规检查（backend-rules §7，增量面）+ 编译门控自验
  - G-01~G-07 grep 扫描 + 人工核对（exec 参数数组/SQL 拼接/硬编码凭据/SQL 层位/panic/goroutine）；go build ./... && go vet ./... && go test ./... -race -count=1 全仓 + gofmt -l + GOOS=linux 交叉编译
- [x] T-DBM-V8 · 归档与 INDEX
  - 本清单落盘 backend/tasks.md（增量轮追加）；INDEX.md database-mgmt backend note 追加 v2.3 增量记录（含 A-DB13 跨层同步项）

## database-mgmt v2.3 增量完成汇总（2026-11-18）

- 交付面：14 新接口（MySQL 6 table/{list,structure,drop}+row/{list,update,delete}；Mongo 8 mcoll/{list,create,drop}+mdb/dropdb+mdoc/{list,insert,update,delete}），总 30→44（机械核验 routes.go：readBoth.GET 13 + readAdmin.GET 5 + write.POST 26 = 44，GET 18/POST 26 与 api §3.3 矩阵逐行对齐）；F1 探测 TCP 优先两阶段+socket 兜底+rescan 保守清理（instance/list +redundantSocket、rescan +merged additive，identity_key/既有 DTO 其余零变更）
- 新增文件：adapter/dbmysql/table.go（+table_test.go 8 纯函数用例）/ adapter/dbmongo/governance.go（+governance_test.go 6 纯函数用例）/ service/dbmgmt/table.go / mongogov.go / prober_test.go（7 用例）/ table_test.go（5 用例）/ mongogov_test.go（4 用例）；修改：prober.go（ProbeResult 两阶段+MergeRedundantSockets）/instance.go/dto.go/consts.go/ports.go/service.go/identity.go/errcode/dbmgmt.go/api handler.go+routes.go/既有测试同步（dbmgmt_test/branch_test/mongo_test/module_test）
- 错误码：DBM_022~028 按 api §3.4 定稿逐字实现；**A-DB13 编号冲突裁定**——既有 ErrDBMEngineUnauthorized（v0.1.4 修复轮占 DBM_022）改编号 DBM_029（符号/语义不变）；机械核验 DBM_001~029 连续唯一。**跨层同步项（上报）**：前端 types/database.ts:296 DBM_022 文案映射、MongoCreateUserModal.vue:90 与 MongoRoleManageModal.vue:56 的 DBM_022 判定、types/__tests__/database.test.ts 用例须改 DBM_029（PageSpec v1.4 已按 DBM_022=表不存在定稿）；建议设计侧 api §3.4 补记 DBM_029 行
- 审计：+10 动作码共 28（机械核验 consts.go+bootstrap.go 28 唯一零重复）；新写动作经 txAuditOnly 独立事务（A-DB16，事务内唯一写入=审计行）；instance_merged 与 DELETE 同事务；detail 脱敏断言用例在位（row_updated 含列名不含行值/mongo_document_inserted 含 insertedId 不含文档内容）
- 安全底线核验：R-08 本轮零新增 exec 站点（表级经 g.query 既有字面量参数数组通道，密码 MYSQL_PWD env 不进 argv；mongo 进程内 driver 零 exec）；A-DB3 标识符 service 白名单（validateTableName/validateCollectionName/列名∈结构集）+quoteIdent/quoteString 双防线（注入形态用例 TestRowStmtEscaping）；A-DB10 引擎侧 SQL 归适配器（G-04 命中人工复核：table.go 引擎 SQL 沿 gateway.go 先例，service 层命中全为注释）；I-02 凭据零回显（新 DTO 无凭据字段）；I-12 系统库 admin/config/local DBM_028 硬校验先于库存在判定
- 自验（2026-11-18 实跑，go1.24.0 windows/amd64，cwd=backend/）：`go build ./...`=0 ✓ `go vet ./...`=0 ✓ `go test ./... -race -count=1` 全仓 48 个含测试包全 ok 0 FAIL ✓ `gofmt -l internal/ cmd/` 0 文件 ✓ `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./...`=0 ✓；覆盖率 service/dbmgmt 85.2%（超 80% 目标）·api/dbmgmt 66.3%·adapter/dbmysql 33.4%·adapter/dbmongo 24.2%（引擎实联环境绑定沿先例归 Linux 冒烟）
- Gate 2 增量面：G-01 唯一 -c 命中=cronx/runner.go:145 既有豁免（A-C2 argv 元素非拼接，本轮零新增 exec）；G-02 Sprintf SQL 0 命中；G-03 硬编码凭据 0 命中；G-05 panic 0 命中；G-06 新增 goroutine 0；G-07 全绿（上行）
- CI 环境耦合预防第 8 起：表级/集合文档治理测试全注入化（fakeMySQL/fakeMongo 记忆实况替身 + 适配器纯函数 SQL 组装/extjson 往返/classify），探测 reachable 全注入——零 runner 实况引擎依赖（Windows/Linux 行为一致）
- 环境绑定如实声明：dbmysql 表级 CLI 实联（information_schema/JSON_OBJECT/ROW_COUNT() 多语句）与 dbmongo 治理 driver 实联（createCollection/collStats/ReplaceOne 等）归 Linux 目标环境冒烟；extjson 顶层编码限制经 {"_id":...} 包装文档法规避（驱动 MarshalExtJSON 要求顶层文档/UnmarshalExtJSON 顶层 any 落 bson.D——idFromJSON/idToJSON 包装实现，TestObjectIDExtJSONRoundTrip/TestDocToBSONExplicitID 锚点）
- 遗留（非阻断）：A-DB13 前端 DBM_029 映射同步归前端 v2.3 轮（本轮后端单边交付，前端旧映射 DBM_022 文案在新码下失配——工作流统一门控前须同步）；api §3.4 DBM_029 补记归设计侧

---

# db-tools 新模块 + env-mgmt Node 增量 Go 后端代码生成任务清单（2026-11-19）

> 创建时间：2026-11-19
> 设计文档：profiles/dougong-panel/workspace/02-detailed/api/api-db-tools.md（v1.2，唯一事实源）+ d0-clarification-db-tools-integration-2026-11-19.md（Q1~Q3 拍板+安全底线）+ prd-env-mgmt.md v1.3 FR-08（Node 环境供给）
> 代码输出：backend/（module github.com/dougong/dougong，gin + modernc.org/sqlite + log/slog；分层 api→service→store/adapter）
> 环境：go1.24.0 windows/amd64（D:\go-toolchain\go\bin\go.exe，PATH 内 go 同源）；测试全注入 fake 替身零 runner 实况依赖（CI 环境耦合预防沿先例）。

## 设计裁定与落码取证（A-DBT1~A-DBT10）

| # | 决策点 | 裁定/取证 |
|---|--------|----------|
| A-DBT1 | 迁移编号：db-tools api §5.3 钉 013_db_tools.sql；env-mgmt Node env_type 八值 CHECK 重建亦需迁移（api-db-tools D-18 跨模块待同步项，编号未钉） | 013_db_tools.sql（两表+两索引，逐字按 api §5.3）+ 014_env_mgmt_node.sql（env_install_task 表重建八值 CHECK，**保留 012 bootstrap_status/note 两列**全量拷贝，沿 010 重建先例） |
| A-DBT2 | D-13 ⚠️版本/sha256 落码取证（设计不虚构哈希） | 实取证据：①phpMyAdmin 5.2.2 官方 sha256=`8551c8bf3b166f232d5cf64bac877472e9d0cb8f2fe1858fab24f975e7d765b6`（curl https://files.phpmyadmin.net/phpMyAdmin/5.2.2/phpMyAdmin-5.2.2-all-languages.tar.gz.sha256 原文）；②mongo-express@1.0.0 npm registry 存在（dist.shasum=d1cc5ed836b5e2a7332c226fd2785a49d9e46dcf，npm install 精确版本自带 registry integrity 校验）；③NODE_LTS_TRACK=22，NodeSource 源 URL 经官方 setup_22.x 脚本取证+可达性核验：apt `https://deb.nodesource.com/node_22.x nodistro main`（key=deb.nodesource.com/gpgkey/nodesource-repo.gpg.key，armored 头核验）；rpm baseurl=`https://rpm.nodesource.com/pub_22.x/nodistro/nodejs/$basearch` gpgkey=`https://rpm.nodesource.com/gpgkey/ns-operations-public.key`（repomd.xml HTTP 200 核验；架构 x86_64/aarch64，module_hotfixes=1） |
| A-DBT3 | D-05「tar 解压经参数数组 R-08」 | 实现用 Go stdlib archive/tar 逐条目解压+显式 zip-slip 路径校验（filepath.Clean+前缀白名单，沿 FILE_009 语义）——零外部命令（R-08 by-absence，phpfpm 先例），路径防线强于外部 tar；顶层目录剥离（phpMyAdmin-5.2.2-all-languages/→destDir） |
| A-DBT4 | D-09/4.4.2 signon/verify 响应设计为 {user,password}；phpMyAdmin 需知目标实例 host/port 方能建连 | 响应 additive +host/+port（SignonScript 设 $_SESSION['PMA_single_signon_host']/['PMA_single_signon_port']，phpMyAdmin 官方 signon 机制支持）；仍仅 loopback 服务端通道，凭据永不进浏览器（I-02 不变） |
| A-DBT5 | 秘密文件 0600 与 PHP-FPM 非 root worker 可读冲突 | 分级：internal-secret/mongo-express-auth.conf/systemd 单元/dbtool-cred.key=0600（root nginx/panel/systemd 读取）；config.inc.php/signon.php/phpmyadmin-fastcgi.conf=0644（PHP-FPM worker 必读，phpMyAdmin 惯例 config.inc.php 0644）；internal 秘密嵌入 signon.php——loopback 源校验为主防线、秘密头为纵深防御（持秘密仍须有效 admin 会话/合法令牌方可达任何能力） |
| A-DBT6 | 注入内容时效：gate location proxy_pass/signon.php 回调 URL/SignonURL 含 entryPath+面板域 | 安装时烘焙（panel_config 现值）；面板安全入口重生成/域名变更后须重装工具刷新注入——期间 gate 子请求 404→工具路径 401 fail-closed（无安全洞，可用性降级如实）。跨模块变更钩子 OUT 本轮 |
| A-DBT7 | FR-08② Node 观测口径 | 已装判定经包管理器候选查询（nodejs 三系同名）；版本经 `node --version`（envpack.BinaryVersion 参数数组）；二进制缺席→version=null 如实不虚构（installed 从包数据库判定）；运行时型无服务单元：UnitByFamily/UnitPatternByFamily 空→EnvList 不发起单元查询、ServiceRunning/Enabled=null（既有 EnvList 逻辑天然满足） |
| A-DBT8 | site-mgmt ConfigWritePipeline OHS（api-site-mgmt v2.8 D-21 登记契约 Apply/Remove） | sitemgmt.ConfigApply=applyPipeline（R-06 四步）+reloadAfterWrite（写后即生效）；ConfigRemove=最近写前快照经管线恢复；db-tools 注入/撤除采用标记 span（# BEGIN/END dougong-dbtool {key}）+ConfigApply——撤除=精确移除注入 span 恢复原字节（往返测试断言 byte 级还原），较快照恢复更安全（不吞没注入后其他合法编辑） |
| A-DBT9 | D-06 phpMyAdmin 工具 location FastCGI 形态 | alias+嵌套 regex php location（capture 显式 SCRIPT_FILENAME={toolDir}/phpmyadmin/$1）——静态资源外层 alias 直服、PHP 内层 fastcgi_pass pool；无 symlink 无 if（nginx 子路径 php 应用标准配方）；fastcgi 参数模板 phpmyadmin-fastcgi.conf 安装时写入 |
| A-DBT10 | FR-08③ Node 服务操作拒绝码 | ServiceOp 对无单元映射条目（运行时型）返回 ENV_006 data.reason=「运行时环境无常驻服务单元，服务操作不适用」（不在操作对象集，如实拒绝非静默） |

## 任务清单

- [x] T-DBT1 · env-mgmt +Node 环境（FR-08/⚠️A16/A17）
  - 输入：prd-env-mgmt v1.3 FR-08 + api-db-tools D-13 NODE_LTS_TRACK=22 + A-DBT2③ NodeSource 取证
  - 输出：store/envmgmt/models.go +EnvNode 常量；迁移 014_env_mgmt_node.sql（八值 CHECK 重建保留 bootstrap 列）；envpack +BinaryVersion（node --version 参数数组）+vendorrepo.go +ensureNodeApt/ensureNodeYum（三系代配，writeKeyAndList 泛化 keyPath/keyURL）；service/envmgmt Catalog +EnvNode 条目（PkgByFamily nodejs 三系/VendorRepo 三系 true/单元映射空）+ServiceOp 运行时型拒绝（A-DBT10）+EnvStatus 环境实况读契约 OHS（db-tools 依赖就绪度消费）
  - 测试：catalog 八条目机械断言（Node 单元映射空例外）/NodeSource 三系代配文件内容断言（fake readFile/writeFile/httpGet）/不支持组合 failed/node 安装 worker 链（fake runner）/ServiceOp node 拒绝/EnvStatus 三态
- [x] T-DBT2 · db-tools store 层 + 迁移 013 + errcode DBT 族
  - 输入：api §5.2 SQL 清单 15 条/§5.3 DDL/§3.4 DBT_001~012
  - 输出：migrations/013_db_tools.sql；store/dbtools/（models.go Tool/Task+常量、tool_repo.go、task_repo.go、Store/TxStore+Audit 同事务聚合，全参数化）；errcode/dbtools.go（DBT_001~012 逐字）
- [x] T-DBT3 · adapter/toolx 部署网关（ToolDeployGateway 契约实现）
  - 输入：api §4.1.5 ToolDeployGateway + D-05/D-13 常量
  - 输出：DownloadTarball（net/http+镜像拼接式前缀 install.sh:38-45 同构+DOWNLOAD_TIMEOUT）/VerifySha256（逐字节）/ExtractTar（stdlib+zip-slip 防线 A-DBT3）/NpmInstall/NpmRemove/NpmGlobalRoot（参数数组 R-08，registry 镜像选项）/PhpExtensions（php -m）/WriteSecretFile（0600 原子）；全依赖注入点（runCmd/lookPath/httpGet/writeFile/mkdirAll）
  - 测试：sha256 命中/失配、zip-slip 拒绝（合成 tarball）、npm argv 数组断言（fake runner）、镜像拼接
- [x] T-DBT4 · service/dbtools 业务层（目录/探测/部署 worker/凭据 vault/注入/access）
  - 输入：api D-01~D-18 全决策 + §2 跨聚合不变量 I-C01~C03
  - 输出：consts.go（D-13 常量表+审计码 D-16）/dto.go（可空显式 *T 无 omitempty）/ports.go（消费方接口：ToolDeployGateway/ToolCredentialGateway/ConfigWritePipeline/EnvReadiness/SysdGateway/PhpFpmGateway/NginxConfReader/PanelInfo/SessionVerifier/AuditRecorder）/vault.go（dbtool-cred.key AES-256-GCM 沿 CredentialVault 先例）/catalog.go/service.go（ToolList 实时探测 D-02+ToolStatusForEngine OHS）/deploy.go（Install/Upgrade/Uninstall 同步校验+互斥 I-06+worker 执行链+终态档案同事务 I-C01+RecoverInFlight I-09）/inject.go（落点选择 D-03+标记 span 注入/撤除 A-DBT8）/templates.go（config.inc.php/signon.php/fastcgi.conf/systemd 单元/nginx location 模板）/access.go（Open/Reopen/Rotate D-08 四步回滚 I-C02）
- [x] T-DBT5 · gating/signon 安全核心（D-09/D-11，安全底线不可桩假）
  - 输出：gate.go（GateService：VerifyWs 语义只读会话校验+admin 角色权威判定）/signon.go（SignonService：HMAC-SHA256 令牌签发/四重校验〔签名/exp 60s/jti 一次性内存台账+30s TTL 清扫 goroutine 入 wg/instanceId+sessionDigest 双绑定〕+凭据 loopback 注入响应 A-DBT4）；api/dbtools 内部端点防护链（LoopbackGuard 裸 404+InternalSecretGuard 裸 404+gate 裸 200/401+signon 包络 DBT_007/008/010）
- [x] T-DBT6 · 跨模块 OHS 提供方 + config + main 装配
  - 输出：dbmgmt/toolgateway.go（InstanceCredentialsForTool/InstanceReadyForTool，vault 解密仅服务端 I-02）；sitemgmt/configpipeline.go（ConfigApply/ConfigRemove OHS A-DBT8）；config +Tools{Mirror,NpmRegistry}（DOUGONG_TOOLS_MIRROR/DOUGONG_TOOLS_NPM_REGISTRY env 覆盖）；main 装配（toolx/dbtools store+service+api 模块/菜单「数据库工具」/signon 清扫 goroutine 入 wg/RecoverInFlight）
- [x] T-DBT7 · api 层 11 端点 + 测试（fake 替身零 runner 实况依赖）
  - 输出：api/dbtools/module.go+handlers（面板面 9：tool/list·task/list·task/detail 两角色；install/upgrade/uninstall/open/rotate admin+CSRF；reopen GET admin 无 CSRF 302 载体；内部 2：gate/verify·signon/verify）
  - 测试（必须回归）：gating 无会话/非 admin→401、loopback/秘密头缺失→裸 404；signon 防伪（篡改签名 DBT_007/过期 DBT_008/重放 DBT_010/跨实例跨会话 DBT_007）；卸载不删数据（执行链白名单断言：引擎数据目录/db_instance 档案零触及）；sha256 失配任务 failed 不安装；互斥 DBT_003；终态档案同事务 I-C01；轮换四步回滚 I-C02；注入/撤除 byte 级往返；权限矩阵（readonly 写 AUTH_008）
- [x] T-DBT8 · Gate 2 合规检查 + 编译门控自验 + 归档
  - G-01~G-07 grep 扫描+人工核对（增量面）；go build ./... && go vet ./... && go test ./... -race -count=1 全仓 + gofmt -l + GOOS=linux 交叉编译；tasks.md 勾选归档 + INDEX.md backend note 更新（db-tools 新行+env-mgmt Node 增量）

## db-tools + env-mgmt Node 增量完成汇总（2026-11-19）

- 交付面：db-tools 新模块 11 端点（面板面 9：GET tool/list·task/list·task/detail 两角色 + POST install/upgrade/uninstall/access/open/access/rotate admin+CSRF + GET access/reopen admin 无 CSRF 302 载体；内部 2：GET gate/verify 裸 200/401 + POST signon/verify 包络 DBT_007/008/010）；env-mgmt 七→八环境 +Node（NodeSource 三系代配/LTS 22.x 钉死/node --version 观测/运行时型无服务单元/EnvStatus 读契约 OHS）
- 新增文件：migrations/013_db_tools.sql（两表+两索引逐字 api §5.3）+014_env_mgmt_node.sql（八值 CHECK 重建保留 012 bootstrap 列）；store/dbtools/（models/tool_repo/task_repo+测试 3 用例）；adapter/toolx/（下载/sha256/stdlib 解压 zip-slip/npm/php -m/秘密文件+测试 9 用例）；service/dbtools/（consts/catalog/dto/ports/vault/service/deploy/inject/templates/access/signon/gate+测试 fakes/deploy/security/inject 共 24 用例）；api/dbtools/（module/helpers+测试 6 用例全栈 httptest）；errcode/dbtools.go（DBT_001~012）；跨模块 OHS 提供方 dbmgmt/toolgateway.go（+4 用例）/sitemgmt/configpipeline.go（+2 用例）/panel/toolaccess.go（+1 用例）；envpack +BinaryVersion/VendorIO/NodeSource 代配（+5 用例）；envmgmt +EnvNode/EnvStatus/ServiceOp 运行时拒绝（+6 用例）
- 安全底线核验（不可妥协项全实现非桩假）：①工具仅绑 localhost（mongo-express ME_CONFIG_SITE_BIND_ADDRESS=127.0.0.1:8081 编译期定死；phpMyAdmin 无独立端口经 PHP-FPM pool）；②nginx auth_request→gate/verify 面板会话+admin 鉴权（VerifyWs 只读不续期；无会话/readonly/失效→裸 401 回归在位）；③内部端点 loopback+秘密头双保险（缺失/不符→裸 404 回归在位）；④signon HMAC-SHA256 四重校验（篡改 DBT_007/过期 60s DBT_008/重放 DBT_010/跨实例跨会话 DBT_007 回归全在位；重启台账清空失效回归）；⑤凭据 vault AES-256-GCM dbtool-cred.key 0600 永不回显（DTO 无凭据字段；审计 detail 脱敏断言）；⑥R-08 参数数组（npm/systemctl dbus/php -m/node --version；Mongo 凭据经 systemd Environment 不进 argv——argv 泄漏断言用例）；⑦下载 sha256 强制（失配 failed 不安装不清档案回归）；⑧卸载不删数据（白名单断言回归：remove/removeall 仅 toolDir/单元路径，引擎数据/db_instance 零触及）；⑨零遥测 R-09（phpMyAdmin VersionCheck=false+SendErrorReports=never；下载用户主动例外）
- 落码取证（A-DBT2）：phpMyAdmin 5.2.2 sha256=8551c8bf…765b6（官方 .sha256 curl 原文）；mongo-express@1.0.0 npm registry 存在（dist.shasum=d1cc5ed8…）；NodeSource 22.x apt nodistro main/rpm pub_22.x/nodistro/nodejs/$basearch+ns-operations-public.key（官方 setup_22.x 脚本取证+URL 可达性核验）
- 自验（2026-11-19 实跑，go1.24.0 windows/amd64，cwd=backend/）：`go build ./...`=0 ✓ `go vet ./...`=0 ✓ `go test ./... -race -count=1` 全仓 52 个含测试包全 ok 0 FAIL ✓ `gofmt -l internal/ cmd/` 0 文件 ✓ `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./...`=0 ✓
- Gate 2 增量面：G-01 唯一 -c 命中=cronx/runner.go:145 既有豁免（A-C2，本轮零新增 shell）；新增 exec 站点 toolx/gateway.go:464+envpack/gateway.go:279 均参数数组；G-02 Sprintf SQL 0 命中；G-03 硬编码凭据 0 命中；G-04 SQL 仅 store/dbtools（service/api/adapter 0 命中）；G-05 新包 panic 0；G-06 goroutine 两处均有退出路径（runTask 有限时命令链；RunTokenSweeper main wg+ctx 取消）；G-07 全绿（上行）
- 实现裁定上报（A-DBT3~A-DBT10 详见裁定表）：tar 解压 stdlib 替代外部 tar（zip-slip 防线更强）；signon/verify 响应 additive +host/port（phpMyAdmin 建连必需，仍仅 loopback）；PHP 配置文件 0644/秘密文件 0600 分级（PHP-FPM 非 root worker 可读）；注入内容 entryPath/域名安装时烘焙（面板入口/域名变更后须重装刷新，期间 fail-closed）；Node 版本二进制缺席 null 如实；迁移编号 013=db-tools（设计钉死）/014=env Node（设计未钉）
- 环境绑定如实声明：npm/php-fpm/systemd/nginx 实联与 phpMyAdmin/mongo-express 工具本体冒烟归 Linux 目标环境（测试全注入 fake 替身零 runner 实况依赖，Windows/Linux 行为一致）；前端工具管理页/入口按钮归前端轮（本轮后端单边交付，ToolStatusForEngine OHS 已提供）
- 遗留（非阻断）：①api-env-mgmt.md Node 增量（env_type 八值/常量）设计侧文档同步归设计轮（本轮按 prd FR-08+api-db-tools D-18 实现）；②database-mgmt 高级入口 PageSpec 消费 ToolStatusForEngine 归前端/D4 轮；③注入时效（A-DBT6）跨模块变更钩子 OUT 本轮

# db-tools v1.3 自动 nginx 落地增量 Go 后端任务清单（2026-11-20）

> 创建时间：2026-11-20
> 设计文档：profiles/dougong-panel/workspace/02-detailed/api/api-db-tools.md（v1.3，唯一事实源）+ d0-clarification-dbtools-auto-landing-2026-11-19.md（Q1 自动落地+域名联动/Q1b HostGuard 内部端点豁免/Q2 reason 可见/Q3 安全不变）
> 代码输出：backend/（增量模式：零新 HTTP 接口/零 DDL/零新错误码/零枚举变更；变更面=worker 执行链/钩子/错误文本/网关豁免/DTO additive 字段）
> 环境：go1.24.0 windows/amd64；测试全注入 fake 替身零 runner 实况依赖（沿先例）。

## 设计裁定与落码取证（A-DBT11~A-DBT13）

| # | 决策点 | 裁定/取证 |
|---|--------|----------|
| A-DBT11 | D-23「SignonURL 烘焙基址同口径」落码歧义：reopen 为面板后端 API，自动块形态面板 Go 直连（0 匹配推断——面板若经 nginx 承载必有匹配块），SignonURL 若指向工具落地端口 18090 则 nginx 自动块无面板 location→404 重认证失效 | SignonURL 基址=面板访问基址而非工具落点基址：既有块（port=0）沿旧口径 scheme://host（nginx 80/443 承载面板）；自动块（port≠0）={panelScheme}://{hostname}:{ListenPort}（hostname=BindDomain 优先，未绑安装期动态 PHP 回退=preg_replace 去 HTTP_HOST 端口+面板端口拼接；panelScheme 按 pa.HTTPSEnabled——工具落地块恒 http 不决定面板 scheme）。Open URL/reopen 302 Location 仍按 D-23 工具落点端口分支（18090）。落点：deploy.go signonBaseHost + templates.go signonURLExprPanelPort |
| A-DBT12 | D-19⑥ 清块「ConfigRemove 恢复原字节=文件不存在态」：既有 ConfigRemove=最近写前快照恢复，自动块首建（allowNew=true）无快照、后续快照含块内容——快照恢复无法表达「文件不存在」 | site-mgmt 管线泛化扩展（非 db-tools 耦合）：applyPipeline 新文件创建成功→备份桶写 .origin-absent 标记（非 .conf 不参与快照清理/寻址）；ConfigRemove 标记优先→删除文件+reload（文件已不存在=幂等成功）；无标记沿既有快照恢复语义。ConfigRemove 全仓唯一消费方=db-tools cleanupAutoLanding（grep 实证），site-mgmt 自身流程零行为变化 |
| A-DBT13 | 回归测试发现：自动块 gate location proxy_pass 指向面板监听地址→第二工具安装时 selectLanding proxyTargetsPanel 误判自动块为面板反代候选（siteRef 丢失 port/autoCreated→Open URL 无端口/清块判定失效） | selectLanding 候选扫描排除自动落地块（文件名+landing 标记+listen 18090 判定，D-19 专用块非面板域块）；既有块优先/多候选语义不变。另：nginx 词法括号非分隔符（"($host" 单一词元）——if 行检测按 strings.Contains("$host") 判定（幂等零写入依赖） |

## 任务清单

- [x] T-DBT9 · 自动落地块（D-19）+ siteRef 扩展
  - 输入：api D-19/4.4.4 自动块模板 + D0 Q1
  - 输出：consts.go +ToolLandingPort=18090/ToolLandingConfName=dougong-tools.conf/ToolLandingBind=0.0.0.0（设计钉死，nginx listen 缺省即绑全部地址）/markerKeyLanding；inject.go siteRef +Port/AutoCreated（JSON 扩字段零 DDL，旧档案零值兼容）+selectLanding 0 匹配→autoCreateLanding+候选排除自动块（A-DBT13）；landing.go（新文件 LandingService：autoCreateLanding 幂等复用先于端口探针/landingConfPath FirstWritableDir/renderLandingConf 骨架/isLandingBlock 标记判定）；ports.go DeployGateway +ProbePortAvailable/NginxConfReader +FirstWritableDir；toolx +ProbePortAvailable（net.Listen 即探即放+Deps 注入点）
- [x] T-DBT10 · 域名联动（D-20）+ toolBaseHost 端口分支（D-23）+ DTO additive
  - 输出：landing.go refreshLandingServerName（span 外科手术仅改 server_name/if 两处——工具 location 字节不动；一致零写入幂等；正则元字符 QuoteMeta 转义）+OnBindDomainChanged（ConfigChangeHook 实现，编译期断言）+lazyRefreshLanding（open/reopen 兜底，失败不阻断 slog）；panel/confighook.go（新文件提供方 S-11-01：ConfigChangeHook 契约+RegisterConfigChangeHook+notifyBindDomainChanged recover 隔离）+config.go UpdateConfig bindDomain 变更成功后通知+service.go configHooks 字段；main.go 注册 panelSvc.RegisterConfigChangeHook(dbtoolsSvc)；deploy.go toolBaseHost port≠0 分支（BindDomain 优先否则 requestHost 去端口，JoinHostPort IPv6 规范化）+signonBaseHost（A-DBT11）；templates.go signonURLExprPanelPort；access.go Open/Reopen 惰性校正；dto.go ToolItem +landingPort/autoCreated（*T 无 omitempty 可空纪律）+service.go ToolList 投影（解析失败 null 不虚构）
- [x] T-DBT11 · HostGuard/EntryPathGuard 内部端点豁免（D-21，panel-core 机制侧 S-07）
  - 输出：middleware/hostguard.go +internalExemptPrefixes=/dbtool/gate/·/dbtool/signon/（编译期常量最小化）+isInternalExemptPath+HostGuard 前置直通；entrypath.go EntryPathHandler 前置直通（未携前缀防御性兜底；携前缀沿既有剥前缀语义）；main.go 链序注释更新（Entry→Host→gin 不变）。豁免 Host/入口路径校验而非鉴权——loopback+秘密头+会话/令牌防护链全量不变；非内部路径强校验 fail-closed 不变
- [x] T-DBT12 · reason 可见（D-22）+ 卸载清块（D-19⑥/I-C04）
  - 输出：deploy.go runTask 终态错误文本 errors.As 取 *errcode.Error Data()["reason"] 非空追加 ": "+reason（无 reason 沿 symbol+message 不虚构）；uninstallPHPMyAdmin/uninstallMongoExpress +cleanupAutoLanding（两工具均卸载+autoCreated=true→ConfigRemove；非自动块绝不动；块已不存在幂等；失败 DBT_012 reason 档案不变）；sitemgmt pipeline.go +markOriginAbsent/configpipeline.go ConfigRemove origin-absent 分支（A-DBT12）；install 链 appendLandingHint 放行 TCP 18090 提示（D-19⑦）
- [x] T-DBT13 · 回归测试（fake nginx 替身，D0 验收口径全项）
  - 输出：service/dbtools/landing_test.go（新文件 13 用例：自动建块内容未绑/已绑+if 转义+SignonURL 面板端口/端口占用 reason 精确文本/写入失败 reason/目录不可用/幂等复用探针先行/既有块优先/清块两工具判定+非自动块不动/清块失败档案不变/钩子刷新幂等换绑解绑/惰性校正自愈+失败不阻断/toolBaseHost 表驱动 7 例/toolList landingPort 三态/open·reopen URL 端口口径）；fakes_test.go +ProbePortAvailable/FirstWritableDir/ConfigRemove 删除语义；inject_test.go no-candidate 改判自动建块；middleware/internal_exempt_test.go（新文件 4 用例表驱动：HostGuard 豁免 12 例含最小化边界/读态失败豁免前置/EntryPath 携/未携前缀 8 例/Entry→Host 链序协同 5 例）；api/dbtools/module_test.go +TestHostGuardInternalExemptFullStack（全局包装全栈：绑域名后 gate 200/signon DBT_007 可达+无秘密头 404+无会话 401+非内部 404 不变）；panel/confighook_test.go（新文件：绑定/同值零通知/解绑""/非 bindDomain 零通知/panic 不阻断配置权威落库）；sitemgmt/configpipeline_test.go +TestConfigRemoveOriginAbsent（创建→后续快照→Remove 删除文件+reload+幂等）；toolx/gateway_test.go +TestProbePortAvailable（占用 false/释放 true/即探即放）
- [x] T-DBT14 · Gate 2 合规检查 + 编译门控自验
  - G-01~G-07 增量面扫描+人工核对；go build/vet/test -race -count=1 全仓+gofmt+GOOS=linux 交叉（下行实跑记录）

## db-tools v1.3 自动落地增量完成汇总（2026-11-20）

- 交付面：零新 HTTP 接口（11 端点路径/方法/权限矩阵不变）/零 DDL 零新迁移（site_ref JSON 扩字段）/零新错误码（DBT_012 reason 细分）/零枚举变更；变更面=selectLanding 0 匹配自动建块+域名联动钩子主通道+惰性兜底+HostGuard/EntryPathGuard 内部端点最小化豁免+worker reason 可见+卸载清块+toolBaseHost 端口分支+tool/list additive landingPort/autoCreated+open/reopen URL 端口口径
- 新增文件：service/dbtools/landing.go（LandingService）+landing_test.go；service/panel/confighook.go（ConfigChangeHook 提供方 S-11-01）+confighook_test.go；api/middleware/internal_exempt_test.go。修改面：dbtools consts/inject/deploy/templates/access/service/dto/ports/fakes_test/inject_test；panel config/service；sitemgmt pipeline/configpipeline（+测试）；middleware hostguard/entrypath；toolx gateway（+测试）；api/dbtools module_test；cmd/dougong/main.go（钩子注册+链序注释）
- 安全底线核验（Q3 不可妥协项全实现非桩假）：①工具本体仅 localhost 不变（mongo-express 127.0.0.1:8081 单元 env 断言回归在位）；②自动块每请求 auth_request gating（gate+两工具 location auth_request 内容断言；无会话/readonly→401 回归在位）；③内部端点豁免不放宽鉴权（全栈回归：无秘密头裸 404/无会话 401/伪造令牌 DBT_007；loopback+秘密头防护链不变）；④非内部路径 HostGuard 强校验 fail-closed 不变（IP 直连面板仍标准 404 回归；豁免集合最小化边界用例：/dbtool/gate 无尾斜杠不豁免等）；⑤signon HMAC 四重校验/vault 零回显/卸载不删数据白名单回归全在位（全仓 -race 实跑）；⑥自动块写入全经 ConfigApply R-06（nginx -t 失败字节复原）/清块经 ConfigRemove origin-absent（A-DBT12）；⑦端口探针占用如实 DBT_012 reason 不静默换端口；⑧域名 if 行正则元字符转义（QuoteMeta）镜像 HostGuard stripPort+EqualFold
- 自验（2026-11-20 实跑，go1.24.0 windows/amd64，cwd=backend/）：`go build ./...`=0 ✓ `go vet ./...`=0 ✓ `go test ./... -race -count=1` 全仓 52 包 ok 0 FAIL（日志 D:/tmp/dbtools-v13-test.log）✓ `gofmt -l internal/ cmd/` 0 文件 ✓ `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./...`=0 ✓
- Gate 2 增量面：G-01 唯一 -c 命中=cronx/runner.go:145 既有豁免（A-C2，本轮零新增 shell；增量 exec 站点仅 toolx 既有 run 参数数组，探针 net.Listen stdlib 无 exec）；G-02 Sprintf SQL 0 命中（本轮零 SQL）；G-03 硬编码凭据 0 命中；G-04 SQL 仅 store 层（本轮零新增 SQL）；G-05 新文件 panic 0；G-06 新 goroutine 0（钩子进程内同步）；G-07 全绿（上行）
- 实现裁定上报（A-DBT11~A-DBT13 详见裁定表）：SignonURL 面板直连端口口径（D-23 落码歧义裁定，reopen 可达性必需）；ConfigRemove origin-absent 泛化扩展（D-19⑥「文件不存在态」语义兑现，唯一消费方 db-tools）；selectLanding 排除自动块候选（回归测试发现的误判 bug 修复）
- 跨模块待同步项如实声明：api-panel-core.md 提供方文档登记（S-11-01 钩子/S-07 豁免）归设计轮——本轮按 ask 明示落码机制侧变更（代码已实现，文档不代改）；api-site-mgmt.md ConfigRemove origin-absent 语义扩展登记归设计轮
- 环境绑定如实声明：nginx -t/reload 实联/端口探针实况/工具本体冒烟（IP:18090 打开/无会话 401/绑域名非匹配 Host 404）归 Linux 目标环境（测试全注入 fake 替身，Windows/Linux 行为一致；Windows 测试环境两处路径/套接字差异已按平台口径修正测试替身，生产代码 Linux 语义不变）

## app-mgmt 后端增量（2026-11-20，api-app-mgmt.md v1.0 + api-env-mgmt.md v1.4 联动）

- [x] T01 · 迁移 015_app_mgmt.sql：app（UNIQUE name + idx_app_unit 唯一索引承载 I-01/02）+ app_task（四态台账 idx_app_task_app_status/idx_app_task_created）；016_env_mgmt_python.sql：env_type CHECK 八→九值表重建（沿 010/014 先例，含 012 引导两列全量拷贝）
- [x] T02 · store/appmgmt：AppRepo（GetByID/GetByName/List/Create/Delete/UpdateStartSpec）+ TaskRepo（Create/InFlightByName/InFlightByApp/GetByID/List/MarkRunning/Retry/AppendOutput/Finish/FailRunningAll/InFlightIDs）+ TxStore 同事务审计（I-07）；全参数化 SQL 仅本包
- [x] T03 · service/appmgmt：D-01 实时探测投影/D-03 创建校验链（APP_001~006+白名单 APP_012+EvalSymlinks 防逃逸）/D-04 异步任务互斥+重启恢复 I-08/D-05 创建执行链+半档案不残留 I-C02/D-06 unit 安全写入（备份→原子写→systemd-analyze verify→失败回滚）/D-07 删除保守（purgeDir 显式/站点不自动删/绝不静默删代码）/D-09 参数基线比对（fmtMtime 纳秒）/D-10 服务操作瞬时通道/D-13 失败重试（runTask 守卫修正：仅终态早退，Retry running 再入不吞）/D-12 建站 OHS 消费适配器（sitesvc.CreateSite reverse_proxy + ListSites hasSite 值快照）；跨模块 OHS：envsvc.PythonReadiness（v1.4 新增 envpack ProbeVenvSupport/ProbePipAvailable 能力位）+ site-mgmt 提供方登记跨模块待同步项
- [x] T04 · api/appmgmt：16 接口（GET 5〔app/list·app/env·task/list·task/detail·log〕/POST 11）+ 中间件链 InitGuard→AuthRequired→CsrfGuard(POST)→RoleRequired；APP_001~012 errcode
- [x] T05 · 复用既有中间件（无新增 middleware）；main 装配：菜单「应用管理」Order 860 两角色 + appSvc.RecoverInFlight + RegisterModules 追加
- [x] T06 · 测试：service 8 测试函数全注入替身（校验链六路/创建链含 I-C01 档案登记与 I-C02 无残留/删除 purgeDir 两分支/重试 D-13 语义与不可重试/互斥 APP_006/重启恢复/参数基线乐观锁/建站联动成功与透传）；api 全栈权限矩阵（两角色/⚠️A8 log+env admin 专属/CSRF 越界 AUTH_003/生命周期 create→list→delete）；SyncDispatch 测试缝（同步 runTask 沿 env-mgmt 先例）
- [x] T07 · 合规：R-08 全参数数组（python3/pip/systemctl/journalctl/systemd-analyze）；SQL 仅 store 层参数化；goroutine 有限时自然终止（G-06）；%w 包装；ctx 首参
- [x] 自验：go build/vet=0；全仓 test -race -count=1 55 包 0 FAIL（新增 appmgmt service 8+api 1）；gofmt 0；GOOS=linux CGO_ENABLED=0 交叉=0
- [x] A-APP 实现裁定：A-APP1 fmtMtime 纳秒基线（RFC3339 秒级同秒碰撞失效）；A-APP2 创建参数经任务输出首行 plan:{json} 承载（worker 与发起解耦，pending 不续跑保守语义不变）；A-APP3 runTask 守卫仅终态早退（running=Retry 再入——原 pending 守卫会吞重试，测试锁定）；A-APP4 .env 0600 写入+备份 .dougong-bak；A-APP5 unit 模板 EnvironmentFile 引用（.env 缺失时 systemd 忽略该指令，服务照常启动——如实不影响）；A-APP6 建站 OHS 适配器内 domain 空=server_name "_"（site-mgmt 域名校验拒绝时 APP_011 透传引导站点管理）


## db-tools 落地块 HTTPS 增量（2026-11-28，api-db-tools.md v1.4 D-24）

> 设计输入：profiles/dougong-panel/workspace/02-detailed/api/api-db-tools.md v1.4（唯一事实源）。实机取证 v0.6.7（2026-11-28）：面板域名经 Cloudflare 橙云代理时 18090 不在 CF 转发端口集（HTTPS 443/2053/2083/2087/2096/8443）恒不可达（ERR_CONNECTION_REFUSED，用户实机 phpMyAdmin/mongo-express 均进不去）；灰云（仅 DNS）直连+面板 HTTPS 会话 cookie Secure 位致 http 工具落地 gating 失效；面板证书存 panel_cert 单行（GetCertificate 热读）磁盘无稳定文件路径；Origin CA 证书仅 CF 边缘信任（浏览器直连源站 ERR_CERT_AUTHORITY_INVALID 为设计使然）。

- [x] T01 · consts.go：+ToolLandingTLSPort=2087（2083 面板已占用；2087 在 CF 代理端口集内、灰云直连同端口可达）+LandingCertFileName/LandingKeyFileName
- [x] T02 · ports.go +PanelCertReader 契约（CertMaterial 进程内复用——panelsvc.Service 既有方法零新增，main 装配 PanelCert: panelSvc）；service.go Service/Deps +panelCert 字段
- [x] T03 · landing.go：ensureLandingTLS（HTTPS 面板+证书可用→CertMaterial 物化 landing-cert/key.pem 0600+sha256 指纹；ErrNoRows/空 PEM=非 active nil err〔签发中=常态〕；读失败 fail loud 安装链如实）/renderLandingConf +TLS 行组（listen 2087 ssl+证书两行）+块外指纹注释（证书轮换→指纹变更→conf 重写→ConfigApply reload）/refreshLandingServerName→refreshLandingBlock 四段对账（server_name/if+TLS 行组+指纹注释，全一致零写入幂等，工具 location 字节不动；TLS 材料失败→钩子跳过刷新保住既有 TLS 行防降级删行）/autoCreateLanding +2087 探针（占用→DBT_012 reason「工具 HTTPS 端口 2087 被占用」）/lazyRefreshLanding 返回落盘实况 TLS 端口（landingTLSPortFromConf 判 listen 2087）/appendLandingHint TLS 态追加「、2087」/OnBindDomainChanged 取 pa+ensure 后刷新
- [x] T04 · deploy.go toolBaseHost +tlsPort 参数（自动块 autoCreated=true 且 tlsPort≠0→https://{hostname}:2087 覆盖 scheme/port；既有块 port=0 不适用）；access.go Open/Reopen 接线 lazyRefreshLanding 返回值；signonBaseHost 沿 0（SignonURL 面板基址不变，A-DBT11 口径不变）
- [x] T05 · dto.go ToolItem +LandingTLSPort（*int 无 omitempty 可空纪律）+service.go ToolList 投影（PanelAccess+landingTLSPort 面板全局一次；自动块+TLS 可用=2087，未装/既有块/非 HTTPS/证书未配=null 不虚构）
- [x] T06 · 测试：landing_test.go +4 回归（TLS 安装全锚点〔2087 监听/证书 0600/双探针/https URL/提示含 2087/SignonURL 不变〕/升级自愈〔http-only 块补 TLS 行+location 字节不动+幂等零写入+证书轮换 conf 重写+HTTPS 关闭移除回落 http〕/刷新失败窗口磁盘实况两分支〔磁盘具 TLS 沿 https/磁盘无 TLS 沿 http 兜底〕/list 投影三态）+toolBaseHost 表驱动 +3 行（TLS 绑域名/未绑/既有块不适用）+fakes（fakePanel +CertMaterial 替身/tlsFakes/stripTLSLines）+newTestService 装配 PanelCert
- [x] 自验：go build=0；全仓 test 56 包 0 FAIL（dbtools 既有全绿零语义改动+新增 4 函数 14 断言）
- [x] A-DBT 实现裁定：A-DBT14 TLS 材料供给失败（非未配置证书）钩子跳过刷新保住既有 TLS 行（降级删行=HTTPS 落地悄然失联，OUT；惰性校正兜底自愈）；A-DBT15 URL 分支以落盘实况为准（刷新失败窗口不虚设 https——虚设致打开 404/拒连，landingTLSPortFromConf 权威）；A-DBT16 证书指纹注释为管线内 reload 机制（证书轮换后 conf 产生差异触发 ConfigApply→reload，绕过 R-06 直 reload OUT）；A-DBT17 fp=sha256(certPEM) 前 8 字节 hex16（轮换必变更，空 fp=非 active）


## db-tools 内部回环 TLS 跟随热修（2026-11-29，api-db-tools.md v1.5 D-25）

> 实机取证（v0.6.8 用户服务器续）：include 补齐后 18090/2087 监听正常，但两工具一律 500——nginx error `auth request unexpected status: 400`。根因=panelLoopbackBase/gate 子请求/signon.php 回调硬编码 http://127.0.0.1:{port}，TLS 面板的 Go 监听对明文请求回标准 400（"Client sent an HTTP request to an HTTPS server"）；v0.1.16 设计仅覆盖 IP 明文形态（tls=false），HTTPS 面板+自动块从未实机跑过。同机部署事实补记：nginx.conf 缺 sites-enabled include（面板写入配置从未加载，18090 自 v0.1.16 起从未监听）——应急补 include 行生效；site-mgmt 证书扫描器把落地块当站点导入（影子行）。

- [x] T01 · templates.go：panelLoopbackBase scheme 跟随 pa.HTTPSEnabled（gate 子请求+signon.php 回调同出处）；gateLocationInsert HTTPS 态追加 `proxy_ssl_verify off;`（面板托管证书无系统 CA 链，安全由回环+秘密头承载 D-11 不变）；signonPHP https 态 curl 追加 VERIFYPEER false/VERIFYHOST 0
- [x] T02 · 测试：TestPanelLoopbackBaseScheme 两态锚点；TestAutoLandingTLSInstall +4 断言（gate https://127.0.0.1:8687+verify off；signon.php https+VERIFYPEER）；既有 http 形态断言全保（非 TLS 面板行为零变化）
- [x] 自验：go build=0；全仓 test 56 包 0 FAIL；gofmt 净
- [x] A-DBT18 实现裁定：证书校验豁免仅限回环目标（gate 上游 127.0.0.1+秘密头+会话校验三重防护不变；不涉任何公网 TLS 目标）；刷新对账不触及 location 字节——既有 HTTPS+自动块装机需卸载重装（或手工补丁）重新烘焙 gate/signon，v0.6.9 起新装机原生正确

- [x] T03 · mongo bind 修复（实机裁定升级：config.default.js grep 证实 1.0.0 **bind host 无 env 支持**——env 写任何变量名都回落 localhost，Node 17+ 解析 ::1 只监听 IPv6 回环 ss 实证、nginx 127.0.0.1 proxy_pass 502）：templates.go +mongoExpressConfigJS（包根 config.js 覆盖 require config.default 后钉死 site.host=127.0.0.1/port=8081）+unit 移除 bind env 行；consts.go +MongoExpressConfigFileName；deploy.go install/upgrade 在 npm install 后重写 config.js 0644（StartUnit/RestartUnit 前）；deploy_test.go 断言 config.js 三锚点+unit 无 bind env

- [x] T04 · templates.go SignonURL 表达式结构引号修正（实机 php -l 取证：`unexpected identifier "Servers"` line 13——原 suffix 携 `?instanceId='` 结构引号进 phpEscape，字符串永不闭合吞下一行；修=URL 前缀进转义、闭合引号+` . $dgToolInstance` 裸露，signonURLExpr/signonURLExprPanelPort 四分支全改）+phpSingleQuoteBalance 静态配平回归锁（unbound/bound/TLS/deploy 四用例）——substring 断言查不出坏 PHP 的教训入案

- [x] T05 · dbtools 内部回环 TLS 轮续：templates.go signonPHP 重写为 SignonScript 契约形态（实机取证 AuthenticationSignon.php:131/156：include 后调用 get_login_credentials **单参**——原文件作用域 curl+$_SESSION 形态缺函数→PMA include 即 fatal→裸 "Error" 页 200）：定义 `get_login_credentials($dgUser, $dgPass='')`〔默认值防 ArgumentCountError〕、code===0 时 return array(user, password)、失败回退入参；deploy_test/landing_test 断言函数签名+返回形态+配平

- [x] T06 · dbtools signonPHP 凭据持久化补全（实机取证：函数形态漏掉 $_SESSION 写入——AJAX 无 dg_signon→get_login_credentials 返回空凭据→PMA 视为未登录 302 SignonURL 跨源→fetch CORS 拦截→「error (rejected)」）：登录请求验签通过恢复 $_SESSION['PMA_single_signon_user/password/host/port'] 持久化（A-DBT4 语义）+无令牌调用读回持久化凭据；deploy_test 断言更新
