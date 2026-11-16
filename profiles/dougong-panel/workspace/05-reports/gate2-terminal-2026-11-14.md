# Gate 2 验证报告 — 代码→交付门控

**模块**：Web 终端（terminal）
**扫描路径**：backend/（internal/service/terminal、internal/adapter/ptyx、internal/adapter/sysuser、internal/api/terminal、internal/api/middleware/terminalws.go、internal/errcode/terminal.go、cmd/dougong/main.go 装配段）+ frontend/src/（types/api/stores/utils/views/terminal、components/AppTerminal.vue、router 追加段）
**扫描时间**：2026-11-15（报告日期沿用流水线批次 2026-11-14 命名）
**契约基准**：profiles/dougong-panel/workspace/02-detailed/api/api-terminal.md v1.1
**规则链**：引擎 GR-01~GR-06 ⊕ backend-rules §7 G-01~G-07 / frontend-rules §6 F-01~F-06 ⊕ business-concepts R-01~R-18（适用项）
**编译门控（脚本已执行，本报告引用不重跑）**：build=0、vet=0、test=0、web=0

---

## 1. 后端规则扫描（backend-rules §7，实跑 grep）

| # | 检查项 | 结果 | 证据（命令+输出/文件:行） |
|---|--------|------|--------------------------|
| G-01 | 无 shell 拼接 | PASS | `grep -rnE "exec\.Command(Weak)?Context?\(.+\"(-c\|/c)\"" --include="*.go" .`（backend/ 全仓）0 命中（exit=1）。全仓 `exec.Command` 人工核对 12 处全为参数数组；本模块 2 处：pty_unix.go:44 `exec.CommandContext(ctx, "su", "-", identity.SystemUsername)`（username 经 TERM_005 正则+TERM_002 存在性校验后作独立 argv 元素）、pty_unix.go:50 `exec.CommandContext(ctx, shell)` + 登录 shell argv0 语义（:51） |
| G-02 | 无 SQL 拼接 | PASS | `grep -rnE "Sprintf\(.+(SELECT\|INSERT\|UPDATE\|DELETE)" --include="*.go" .` 0 命中（exit=1）；本模块零持久化（I-04）本无 SQL |
| G-03 | 无硬编码凭据 | PASS | `grep -rnEi "(password\|secret\|token)\s*=\s*\"[^\"]{8,}\"" --include="*.go" . \| grep -v _test` 0 命中（exit=1） |
| G-04 | SQL 只在 store 层 | PASS | `grep -rlnE "(SELECT\|INSERT\|UPDATE\|DELETE) " --include="*.go" . \| grep -v internal/store \| grep -v _test` 输出中 **terminal 模块零命中**（service/terminal、adapter/ptyx、adapter/sysuser、api/terminal 均不在列表）；层外命中逐条核验：service/auth/user.go:114、service/fileman/{trash,scope}.go、service/sitemgmt/{scanner,certsvc}.go、service/dbmgmt/ports.go 均为注释文本；service/dbmgmt/prober.go:128 `"SELECT 1"` 与 adapter/dbmysql/gateway.go:24 `"SELECT VERSION()"` 为 database-mgmt 既有已裁定项（tasks.md T07/A-DB10）；internal/store/* 条目为 Windows 反斜杠路径未被 `-v internal/store` 过滤的扫描工件（文件确在 store 层）。migrations 维持 001~005 无 006（零持久化 ✓，`ls backend/internal/store/migrations/` 实跑） |
| G-05 | 无 panic 滥用 | PASS | `grep -rn "panic(" --include="*.go" . \| grep -v _test` 0 命中（exit=1）；main.go:351 生命周期 goroutine 仅为 recover 防御不是 panic 调用 |
| G-06 | goroutine 有退出路径 | PASS | `grep -rn "go func" --include="*.go" . \| grep -v _test` 7 处全在 cmd/dougong/main.go（286/301/316/332/348/386/392，均 ctx 取消退出 + wg.Done + panic 隔离；terminal 扫描器=main.go:344-358）。模块内非 `go func` 形态 goroutine 核对：service.go:256-257 `go pending.ptyPump()`/`go pending.wsPump()`——ptyPump 退出=PTY.Read 错误（session.go:92-101，Kill 关 master 必达）；wsPump 退出=ring.stop() 或写失败（session.go:107-128，写截止 writeWait=10s wsconn.go:14,43）；serve 读循环退出=readLoop 错误兜底 close（session.go:16-25）；全部路径收敛 close() 单点（closeOnce 幂等），生命周期扫描硬上限 8h（lifecycle.go:44-47）保证终态可达，无泄漏 |
| G-07 | 编译+vet+test 全绿 | PASS（引用） | 脚本实跑 build=0/vet=0/test=0/web=0（ask 提供，未重跑）；本检查员另实跑 `go test -cover -count=1 ./internal/service/terminal/ ./internal/api/terminal/` = ok 0.929s coverage 89.3% / ok 1.342s coverage 75.4% |

**SEC-GATE 探针（本检查员实跑）**：向 backend/.secgate-probe.go 写入 `package dougong` + `exec.Command("true")` 字面量参数数组 → 写入成功且可读，随后删除（输出：`PROBE: write+read OK (Mimosa disabled)`）。探针通过 + 本模块外部命令为真实实现（pty_unix.go su/shell 直连，无合规桩；pty_windows.go/dir_windows.go 为平台退化哨兵 ErrUnsupportedPlatform，目标环境 Linux 实装在 *_unix.go）→ **DEBT-SEC-GATE 不适用**，不构成 WARN/FAIL。

## 2. 前端规则扫描（frontend-rules §6，实跑 grep）

| # | 检查项 | 结果 | 证据 |
|---|--------|------|------|
| F-01 | 无 Options API | PASS | `grep -rLn "script setup" --include="*.vue" src/` 0 命中（exit=1）；terminal 5 个 .vue（index/ConnectModal/SessionPane/AppTerminal）逐读全为 `<script setup lang="ts">` |
| F-02 | 无裸 any | PASS | `grep -rnE "(:\s*any\|as any)" --include="*.ts" --include="*.vue" src/` 0 命中（exit=1）；types/terminal.ts 全量 unknown+收窄（parseServerFrame :65-128） |
| F-03 | 请求只走 api 层 | PASS | `grep -rnE "(axios\|fetch)\(" --include="*.vue" src/views src/components` 0 命中（exit=1）；WS 封装于 api/terminal.ts:64-105（connectTerminalSession），组件不裸连 |
| F-04 | 无混入其他组件库 | PASS | `grep -rnE "from '(antd\|element\|ant-design-vue)'" src/` 0 命中（exit=1）；UI 全 n-*（NTabs/NModal/NAlert/NTag 等） |
| F-05 | 无原生弹窗 | PASS | `grep -rnE "(^|\W)(alert\|confirm)\(" --include="*.vue" --include="*.ts" src/` 0 命中（exit=1）；反馈走 useMessage（index.vue:22,92-113） |
| F-06 | typecheck+build 全绿 | PASS（引用） | 脚本 web=0（ask 提供，未重跑） |

栈规则 ⚠️ 项抽查：WS 封装为 api 层函数（§3 🟡 composable 形态以 api 层模块函数承载，断线不自动重连符合 api 4.1.2 辨析③）；AppTerminal 为 xterm.js 封装 + FitAddon + onUnmounted 配对清理（AppTerminal.vue:70-76，§4.5 ✓）；WS 地址同源推导+开发态 vite proxy、无硬编码后端地址（api/terminal.ts:53-58 ✓）；暗色硬编码色仅终端面板背景 rgb(18,21,28)（SessionPane.vue:148/AppTerminal.vue:102，终端主体为 xterm 画布非 UI 主题面，人工核对放行）。

## 3. 前后端接口对齐（以 api-terminal.md v1.1 为基准）

| # | 检查项 | 结果 | 证据 |
|---|--------|------|------|
| AL-001 | 路径/方法 vs API 文档 | PASS | routes.go:31 Prefix=/terminal；:40 `read.GET("/user/options")` → GET /terminal/user/options；:44 `ws.GET("/session/connect")` → WS /terminal/session/connect；与 api §4.1.4 两端点一致，全项目既有模块无路由冲突（/terminal 前缀唯一） |
| AL-002 | 前端调用路径 vs 后端 | PASS | api/terminal.ts:19 `request('/terminal/user/options')`；:57 `${proto}://${location.host}/terminal/session/connect?identitySource=...&username=...`（buildQuery）；query 参数名与 api §4.1.4 `{identitySource, username?}` 一致（后端读取 handler.go:56 `c.Query("identitySource")/c.Query("username")`） |
| AL-003 | DTO 字段 vs 前端类型 | PASS | options：dto.go:9-20 `{items:[{name,uid,shell}], generatedAt}` ↔ types/terminal.ts:11-20；ready 帧：dto.go:44-55 `{sessionId, identity:{source,runUser}, limits:{maxConcurrent, idleTimeoutSeconds, hardLimitSeconds}}` ↔ types:24-34 逐字段同名同义；closed 帧：dto.go:58-62 `{reason, closedAt, durationSeconds}` ↔ types:38-42；output 帧 truncated 标记：session.go:113-117 ↔ types:54；客户端帧 input/resize：dto.go:23-28 ↔ types:47-49；TERM_001 data `{limit, current}`：service.go:136-137 ↔ types:163-166 |
| AL-004 | 枚举值前后端一致 | PASS | terminal_end_reason 6 值：consts.go:22-27 ↔ enums.ts:284-290（self_close/idle_timeout/hard_limit/panel_session_invalid/network_error/protocol_error 逐字相等）；terminal_identity_source 2 值：consts.go:16-17 ↔ enums.ts:301；closed 帧未知 reason 前端不构造（isTerminalEndReason 收窄 enums.ts:308-317） |
| AL-005 | 错误码前端可识别 | PASS | TERM_001~007 errcode/terminal.go:9-21 符号逐字对齐；前端 terminalErrorText（types:161-186）覆盖 TERM_001~006+AUTH_003/008，TERM_007 独立文案 TERM_007_TEXT:189，AUTH_001/002 由 index.vue:92-96 转登录；HTTP 状态档 response.go:57-68（AUTH_001/002→401、AUTH_003/008→403、TERM_004/007→500、其余 TERM→400）与 api §3.4「拒绝/参数/失败」档位一致 |
| 包络 | 响应包络对齐 | PASS | response.go:18-27 `{"code":0,"message":"ok","data":...}` ↔ http.ts:5,153-159 `{code,message,data}` ↔ api-terminal.md §3.1（"message"）；注：backend-rules §4 通用示例写 "msg"，全仓以 account-auth D-07 定稿 "message" 为准且 api 文档即 "message"——以 api 文档为基准判 PASS（INFO 记录） |
| 常量 | 协议/限流常量对齐 | PASS | 5/1800/28800（consts.go:52-56=MaxConcurrentPerUser/IdleTimeout/HardLimit）↔ api §3.4；ScanTick 10s（:58）、复核 30s（:60）、ping 30s（:62）、帧 16KB（:66）、环形 256KB（:68）、非交互排除集 nologin/false/halt/shutdown/空（:78-84）↔ §3.4 常量表逐字一致；ready 帧下发权威值 service.go:239-247 |
| 握手链 | I-01 ①~⑨ 顺序 | PASS | ①Origin 同源→AUTH_003（terminalws.go:39-42，ErrCSRF 符号=AUTH_003 auth.go:11；sameOrigin 复用 csrf.go:43-59）→②Cookie dg_session+VerifyWs→AUTH_001/002（:45-64）→③admin→AUTH_008（:66-69）→④~⑧ service.Admit（service.go:128-226：TERM_005→TERM_001→TERM_002/003→TERM_004→审计失败 TERM_006 回收 PTY）→⑨ 升级后 Attach（handler.go:61-72 + service.go:231-260 原子登记+ready 首帧）——与 api §4.1.4 握手校验顺序逐项对应 |

## 4. 业务规则适用项（R-01~R-18）

| 规则 | 结果 | 证据 |
|------|------|------|
| R-02 会话 Cookie/CSRF | PASS | WS 握手读 HttpOnly `dg_session`（terminalws.go:45；SessionCookieName=auth/consts.go:33）；token 不入前端代码（stores/terminal.ts 无 localStorage）；WS 帧层豁免 CSRF=D-09 拍板（Origin 同源+握手会话绑定）；HTTP GET 豁免 |
| R-03 变更写审计 | PASS | 开合两段审计：opened service.go:208-218（actor/时间/object_type=terminal_session/object_id=sessionId/detail 三键/result/source=web）；closed session.go:183-195（detail={reason,durationSeconds}，source 映射 self_close=web 其余=system :177-180）；closeOnce 保证恰好一对（I-07）；建立失败拒建 TERM_006（:218-224）、断开失败日志兜底不阻断清理（:195-198）=D-06 两段口径 |
| R-08 参数数组禁拼接 | PASS | 见 G-01；su/shell 均 argv 数组，无 sh -c |
| R-14 服务端校验不信任前端 | PASS | validateConnect（service.go:100-122：identitySource 枚举白名单+username 必填+POSIX 形态正则 :97）；指定用户存在性 TERM_002+shell 交互 TERM_003（:143-152）；resize 越界防护（session.go:72-74，A-T4）；帧类型白名单+16KB 上限+Base64 解码失败=protocol_error（session.go:47-84，I-06）；前端镜像守卫 parseServerFrame（types/terminal.ts:65-128） |
| R-18 终端身份/复用会话/限流 | PASS | 默认面板运行用户+可选指定系统用户（ptyx gateway.go:7-9 su 语义不代输密码）；复用面板会话鉴权（握手+30s 复核 lifecycle.go:51-61，失效联动 panel_session_invalid）；并发 5（Registry 原子 CAS registry.go:107-115 + Admit 预检 service.go:135-137 + Attach 竞态兜底 :233-236）/空闲 30min（仅输入刷新 lifecycle.go:40-42）/硬上限 8h（:44-47）=ADR-023 |
| R-09 零遥测 | PASS（N/A 面） | 模块无外呼：仅 /etc/passwd 直读+本机 PTY spawn，无 HTTP 客户端依赖（api/terminal.ts 前端同源 WS） |
| R-01/R-04~R-07/R-10~R-17 | N/A | 登录锁定/nginx/文件/备份/容器/站点规则非本模块面 |

## 5. 测试覆盖率（本检查员实跑）

| 层级 | 覆盖率 | 达标 | 证据 |
|------|--------|------|------|
| 后端 Service 层 | 89.3% | YES（≥80%） | `go test -cover -count=1 ./internal/service/terminal/` ok 0.929s；19 个测试函数（terminal_test.go:415-1052：准入拒绝表驱动/回声全链路/协议违规 5 例/清理幂等/双阈值/复核/心跳/配额竞态/环形缓冲） |
| 后端 Controller/传输层 | 75.4% | YES（≥60%） | `go test -cover -count=1 ./internal/api/terminal/` ok 1.342s；5 个全栈用例（module_test.go:341-625：权限矩阵/WS 守卫四例/握手拒绝/全链路会话流/端到端并发上限） |
| 前端组件 | 未测量（无组件/store 测试） | NO | 仅有 utils/__tests__/terminal.test.ts（16 用例，`grep -c` 实数）；views/components/stores 无测试 → **W1** |

## 6. WARN / INFO 清单

- **W1（WARN）**：前端 terminal 域仅 utils Base64 编解码有测试（16 用例），views（index/ConnectModal/SessionPane）、AppTerminal 组件、terminalStore 无组件/store 测试——SKILL Gate 2「前端组件 60%+」无实证（沿 database-mgmt gate2 W2 同判）。建议后续补 store 状态机用例（connecting→connected→closed 分流、握手失败码分流）。
- **I1（INFO）**：心跳实现为读截止等价式（PongWait=75s=2×30s+余量，consts.go:63-64 + session.go:17-25 A-T8），与 api D-03「连续 2 次未收 pong」语义等价（tasks.md 裁定 A-T8 在案）。
- **I2（INFO）**：Windows 开发机为退化实现（pty_windows.go:15-17 / dir_windows.go:16-22 → TERM_004/TERM_007 如实呈现）；Linux 实装在 pty_unix.go/dir_unix.go（G-07 引用交叉编译 GOOS=linux=0 见 tasks.md T 记录），PTY 实联冒烟归 Linux 目标环境——非 SEC-GATE 合规桩（探针已过+真实实装存在）。
- **I3（INFO）**：backend-rules §4 包络示例 "msg" 与全仓定稿 "message"（account-auth D-07 + api-terminal.md §3.1）不一致为通用规则文案滞后，非本模块违例（以 api 文档为基准）。

## 7. 总结

**判定：WARN**

- 通过：后端 G-01~G-07 全过（G-07 引用脚本门控）、前端 F-01~F-06 全过（F-06 引用脚本门控）、AL-001~AL-005+包络+常量+握手链全对齐、业务规则适用项（R-02/03/08/09/14/18）全过、SEC-GATE 探针通过且无桩（DEBT-SEC-GATE 不适用）
- CRITICAL 违规：0；契约断裂：0
- WARN：1（W1 前端组件/store 测试缺失）；INFO：3
