# Gate 2 增量复检报告 — 安装后配置就绪（env-mgmt 主导 + database-mgmt OHS 联动）

| 属性 | 值 |
|------|-----|
| 模块 | env-mgmt（主）+ database-mgmt（OHS 提供方） |
| 检查时间 | 2026-11-17 |
| 轮次 | /pipeline 增量轮（安装后配置就绪，B/F 代码增量） |
| 基线 | api-env-mgmt.md v1.2 / api-database-mgmt.md v2.2 / PageSpec v1.3 |
| 执行轨 | session |
| 判定 | **WARN**（CRITICAL 0 / 契约断裂 0；编译门控全绿；W1~W4 建议项呈请用户确认） |

## 1. 编译门控（实跑全绿）

| 门控 | 命令 | 结果 |
|------|------|------|
| 前端类型 | `npx vue-tsc --noEmit` | exit 0 |
| 前端测试 | `npx vitest run` | 18 文件 / **195 全过**（+7 本轮：types/env S8 锚点 6 + store 引导字段流 1） |
| 前端构建 | `npm run build`（vue-tsc + vite） | exit 0（25.11s；chunk 体积警告为既有 INFO 形态） |
| dist 同步 | rm -rf backend/web/dist + cp frontend/dist + touch .gitkeep | 同步后 `go build ./...` exit 0（embed 生效） |
| 后端构建 | `go build ./...` | exit 0 |
| 后端静态 | `go vet`（本轮 9 包） | exit 0 |
| 后端测试 | `go test -race -count=1`（本轮 9 包：store/envmgmt·service/envmgmt·service/dbmgmt·adapter/dbmysql·dbredis·dbmongo·api/envmgmt·api/dbmgmt·cmd/dougong） | **9 包全 ok 0 FAIL** |
| Linux 交叉 | `GOOS=linux GOARCH=amd64 go build ./...` | exit 0 |
| gofmt | `gofmt -l`（本轮改动文件） | 0 文件 |

> 全仓 `go test ./...` 含 service/panel 包 **acme_test.go:490 编译错误（env.dir undefined）归属 ACME v2.4 轮未提交改动**，非本轮（W4）；本轮 vet/test  scoped 至本轮 9 包，不触碰 ACME 轮文件。

## 2. 后端栈规则扫描（G-01~G-07，本轮新增文件）

| 编号 | 检查 | 结果 |
|------|------|------|
| G-01 | shell 拼接/exec/sh -c（bootstrap.go ×5） | 0 命中（引导经 g.query/g.run/withClient 参数数组通道，R-08） |
| G-02 | 面板 SQLite SQL 拼接（service 层） | 0 命中（dbmgmt/bootstrap.go 经 InstanceRepository 参数绑定；引擎侧 SQL 经 quoteString A-DB10） |
| G-03 | 硬编码凭据 | 0 命中（密码 crypto/rand 24B→base64url；root 空密码=auth_socket 引导通道非硬编码秘密；MYSQL_PWD/REDISCLI_AUTH env 注入不进 argv） |
| G-04 | 层外 SQL | 0 命中（service 层经仓储；SQL 仅 store/envmgmt task_repo） |
| G-05 | panic | 0 命中 |
| G-06 | 新 goroutine | 0 新增（引导运行于既有 worker 有限时 goroutine 内，无长驻） |
| G-07 | vet/test 引用门控 | exit 0（见 §1） |

## 3. 前端栈规则扫描（F-01~F-05，本轮改动文件）

| 编号 | 检查 | 结果 |
|------|------|------|
| F-01 | any（types/env.ts·enums.ts·index.vue·EnvTaskDrawer.vue·测试） | 0 命中 |
| F-02 | 原生 alert/confirm | 0 命中（NAlert/NTooltip） |
| F-03 | views 层 fetch/axios | 0 命中（经 api/env + envStore） |
| F-04 | 定时器（setInterval/setTimeout） | 0 新增（轮询纪律不变，envStore 既有承载） |
| F-05 | Options API | 0 命中（script setup） |

## 4. 契约对齐（AL-001~005）

| 编号 | 检查 | 结果 |
|------|------|------|
| AL-001 | 路由 vs api（10 接口，无新增） | PASS（引导=worker 机制+OHS，非 HTTP） |
| AL-002 | 前端 Action 路径 vs 后端 | PASS（10 Action 不变） |
| AL-003 | DTO 字段 vs 前端类型 | PASS（task/list·detail +bootstrapStatus/bootstrapNote：后端 json tag ↔ EnvTaskItem 字段逐字；NULL 语义双侧一致） |
| AL-004 | 枚举五方同值 | PASS（bootstrap_status：池/DDL CHECK/后端 store 常量/前端 BOOTSTRAP_STATUS/双 PageSpec 均 succeeded·failed·skipped；env_type 七值不变） |
| AL-005 | 错误码前端可识别 | PASS（无新 HTTP 错误码；引导失败为异步结果维度 bootstrap_status，非 HTTP 错误；ENV_001~006 不变） |

## 5. 增量测试与覆盖率

| 层 | 覆盖 | 说明 |
|----|------|------|
| service/dbmgmt | **86.4%**（超 80% 目标） | bootstrap.go：BootstrapAfterInstall 83.3% / bootstrapEngine 92.9% / registerBootstrapped 73.1% / genBootstrapPassword 75%；11 引导用例（succeeded/skipped×3/verifyFailed/connRetry/unsupported/notConfigured/password 强度） |
| service/envmgmt | runBootstrap **100%** / bootstrapResult 100% / recordBootstrapAudit 66.7% | 5 引导用例（succeeded/nonDB/failed/nilGateway/OHSError）；包总 26.7% 为环境绑定（worker/EnvList 需实况 apt/systemd，沿先例） |
| adapter/dbredis | configGetValue/isLoopbackBind **100%** | BootstrapRequirepass 0%（需实况 redis-cli，归 Linux 冒烟） |
| adapter/dbmysql·dbmongo | BootstrapAdminUser 0% | 需实况 mysql socket/mongod（归 Linux 冒烟） |
| store/envmgmt | 迁移 012 回归 | TestBootstrapResultRoundTrip（引导结果定格/独立维度/NULL/CHECK 拒绝）+ 既有 4 用例 |
| 前端 | types/env S8 锚点 6 + store 引导字段流 1 | vitest 195 全过 |

**CI 第 7 起预防落实**：引导类测试全注入化（fakeMySQLBoot/fakeRedisBoot/fakeMongoBoot/fakeBoot 替身 + dbredis 纯函数），零 runner 实况引擎依赖；引擎实联（socket root auth_socket / localhost exception / CONFIG REWRITE）归 Linux 目标环境冒烟。

## 6. WARN 建议项（呈请用户确认，不阻断）

| # | 项 | 说明 | 处置建议 |
|---|----|------|---------|
| W1 | 适配器引导方法环境绑定覆盖率 | dbmysql/dbmongo BootstrapAdminUser、dbredis BootstrapRequirepass 需实况引擎（Windows 开发机无），0% 归 Linux 冒烟；service 层编排全注入测试覆盖（86.4%/runBootstrap 100%） | 接受（沿全仓 adapter 环境绑定先例；发布后实机冒烟验证） |
| W2 | 工作区本轮外未提交改动 | ACME v2.4 轮 acme_test.go:490 编译错误（env.dir）致全仓 go test ./... 该包 FAIL，归属 ACME 轮非本轮；DBM_005 UX 轮 frontend/types/database.ts 等未提交；本轮专注自身文件不触碰，git 主会话统一 | 知悉（本轮 scoped 9 包全绿；ACME 轮由其主线修复） |
| W3 | MySQL host 名字解析依赖（Gate 1 W1 延续） | dougong@localhost 依赖 skip-name-resolve=OFF；实机冒烟需验证默认态 TCP 可治理 | 接受（实机冒烟清单） |
| W4 | 前端 chunk 体积警告 | index-B2riUq4a.js 4.1MB（gzip 1.07MB）既有形态（naive-ui 全量），非本轮新增 | 知悉（既有 INFO，优化归后续） |

## 7. 实机冒烟清单（发布后 Linux 目标环境验证）

1. apt 装 MySQL → 引导经 socket root auth_socket 建 dougong@localhost → 面板数据库管理 TCP 验证可治理（skip-name-resolve 默认 OFF）。
2. apt 装 Redis → CONFIG SET requirepass + REWRITE 持久化 → 重启 redis 密码仍在 → 面板可治理；bind 127.0.0.1 校验。
3. apt 装 MongoDB（mongodb-org 8.0 代配）→ localhost exception 建 admin root 用户 → 面板可治理。
4. 幂等：重复安装/升级 → 已配置跳过（bootstrap_status=skipped）；用户已有账户/requirepass/凭据不覆盖。
5. 引导失败呈现：模拟引擎未就绪 → 任务 succeeded + bootstrap_status=failed + 任务 Drawer 横幅「已安装·引导失败」+ 重试指引。
6. 密码零回显：审计 detail/任务输出/响应/列表均不含密码。

## 8. 总结

**判定：WARN**（CRITICAL 0 / 契约断裂 0）

- 编译门控九项实跑全绿（vue-tsc/vitest 195/build/dist 同步/go build/vet/test -race 9 包/Linux 交叉/gofmt）
- G-01~G-07 / F-01~F-05 本轮改动面 0 违例；AL-001~005 契约零差（bootstrap_status 五方同值）
- 增量测试：后端 16 引导用例 + store 迁移回归 + 前端 7 用例，-race 全过；service/dbmgmt 86.4% 超目标
- W1~W4 建议项呈请用户经主会话确认；确认后记 pass 交付

**待用户确认**：W1~W4 全过 → Gate 2 记 pass，本轮交付（发版决策由主会话问用户，当前持「先不发版」态度）。

## 9. 用户确认记录

2026-11-17 用户经主会话转达确认：**确认通过**——W1~W4 全部接受（适配器引导实机冒烟 / 他轮改动 / MySQL host 名字解析 / chunk 体积 INFO）。Gate 2 由 WARN 收敛为 **PASS（用户确认）**。

跨轮知会：主会话已修 ACME 轮 acme_test.go 两处测试缺陷（env.dir→env.Dir 编译笔误 + webroot 断言 filepath.ToSlash 归一，匹配 acme.go:487），全仓 go build/test 现 0 FAIL；本轮产物不受影响。

**发版决策：用户拍板「先不发」**（继续攒批，下一批可能是 PHP 多版本）；本轮不执行任何 git 操作，CHANGELOG v0.1.8 仅草稿。交付报告见 05-reports/delivery-env-bootstrap-2026-11-17.md。