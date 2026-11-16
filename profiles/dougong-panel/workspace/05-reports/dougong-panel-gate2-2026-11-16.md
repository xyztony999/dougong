# Gate 2 验证报告 — 代码→交付门控（项目级全仓复验）

**模块**：dougong-panel（全仓 11 模块汇总复验）
**扫描路径**：backend/（Go 全仓）+ frontend/src/
**扫描时间**：2026-11-16
**规则链**：引擎 GR-01~06 ⊕ go-vue3 栈规则（backend-rules §1~§7 / frontend-rules §6）⊕ 业务规则 R-01~R-18
**方式**：主会话 session 模式执行（执行注册表 gate2=session）；本报告为各模块 Gate 2（05-reports/gate2-*-2026-11-14.md）之上的**独立全仓复扫 + 汇总判定**

## 一、后端规则扫描（CRITICAL 复扫，主会话实跑）

| 检查 | grep 实跑结果 | 判定 |
|------|--------------|------|
| 🔴 shell 拼接（sh -c/bash -c/cmd /c） | 唯一命中 `internal/adapter/cronx/runner.go:145`——`exec.CommandContext(cctx, shellBin, "-c", command)` **argv 元素形态**：命令串为独立 argv 元素非拼接，原文取自 crontab 记录不经请求参数（I-08），admin+CSRF+审计；**A-C2 裁定登记在案（tasks.md:882）**，与 cron 自身执行语义同信任级 | PASS（登记性命中） |
| 🔴 exec 非参数数组形态 | `exec.Command(`（非 Context）0 命中；全仓 exec 调用均为 CommandContext 字面量/参数数组 | PASS |
| 🔴 SQL 字符串拼接 | `fmt.Sprintf(.*SELECT/INSERT/UPDATE/DELETE)` 0 命中 | PASS |
| 🔴 硬编码密钥/口令 | 凭据模式 grep 0 命中（db 凭据 AES-256-GCM 加密落库） | PASS |
| 🔴 handler/service panic | 0 命中 | PASS |
| 🔴 goroutine 退出路径 | 11 处裸 `go func` 逐一核对：main.go 8 处（wg.Add/Done 配对 + ctx 取消编排 + tick 级 panic 隔离，G-06 注释在案）、dockerx/logs.go（sctx.Done）、handler_ws.go 读泵 ×2（连接关闭即 return）——各模块 T07/G-06 复核在案 | PASS |
| 🟡 fmt.Print 调试输出 | 0 命中 | PASS |

## 二、前端规则扫描（F-01~F-06，主会话实跑）

| 检查 | 结果 | 判定 |
|------|------|------|
| F-01 无 Options API（全 .vue 含 script setup） | 0 命中 | PASS |
| F-02 无裸 any | 0 命中 | PASS |
| F-03 请求只走 api 层（views/components 无 axios/fetch） | 0 命中 | PASS |
| F-04 无混入其他组件库 | 0 命中 | PASS |
| F-05 无原生 alert/confirm | 0 命中 | PASS |
| F-06 typecheck+build | `vue-tsc --noEmit && vite build` exit 0（27.79s） | PASS |

## 三、编译与测试门控（GR-02/GR-04，确定性命令）

| 门控 | 结果 |
|------|------|
| `go build -C backend ./...` | exit 0 |
| `go vet -C backend ./...` | exit 0 |
| `go test -C backend -count=1 ./...` | **39 测试包全 ok、0 FAIL**（另 8 个无测试文件的薄适配层包；各模块含 -race 全绿记录在案，覆盖抽样 service 71.7%/api 62.9%/store 76.0% 等） |
| `npm --prefix frontend run build` | exit 0（vitest 105 用例全过记录在各模块 F07） |
| `engine-cli check --json`（0.5.4 CLI） | C-01~C-11 全过 |

## 四、前后端契约对齐（GR-05/06，汇总各模块 AL 检查）

11 份模块级 Gate 2 报告均含 AL-001~005 逐项比对（Controller 路径↔API 文档、Action↔路由、DTO↔VO 字段、枚举、错误码），其中 cron-backup 曾发现 1 处契约断裂（E1）经修复+测试锁定后收敛；本次复验无新增断裂。**汇总：契约断裂 0**。

## 五、各模块判定汇总（详证据见各自报告）

| 模块 | 判定 | 遗留建议项 |
|------|------|-----------|
| account-auth | WARN | W1 补测建议 + N1 注释清理 |
| panel-core | WARN | W1 session 透传类型未钉（契约补钉或前端格式化）、W3、W4 |
| monitoring / service-mgmt | WARN | service-mgmt 1 项 |
| file-manager / site-mgmt / terminal / firewall | WARN | 0 项（判定性 WARN） |
| database-mgmt | WARN | 2 项 |
| docker-mgmt | WARN | 1 项 |
| cron-backup | WARN | 3 项（初检 FAIL E1 已修复锁定） |

**共性遗留（非阻断，各报告如实登记）**：nginx/mysql/redis/PTY/compose/ufw 等实机通道归 Linux 目标环境冒烟（Windows 开发机 fake 网关双层覆盖）；go:embed 前端入二进制 + install.sh 端到端冒烟属里程碑验收。

## 总结

**判定：WARN**

- CRITICAL 违规：**0**
- 契约断裂：**0**
- 编译/测试/构建/check 门控：**全过**（确定性命令实跑）
- 遗留建议项：**约 12 项**（5 个模块），均为文档/测试补充/类型钉死类，不阻断交付
- 按门控语义：WARN → 用户确认后交付

---

## 附：WARN 修复轮（2026-11-16 用户指派 /pipeline，run dwfrun-ecd79119）

6 个含建议项模块经全栈修复员逐项处置 + 独立复核员逐项核验（证据见 `gate2-warn-fix-review-2026-11-16.md` 与各报告「WARN 修复轮」表）：

| 模块 | 修复轮 | 终局 | 未闭环（均为维持-理由充分项） |
|------|--------|------|------------------------------|
| docker-mgmt | 3 项 | **pass** | 无 |
| account-auth | 9 项（N1×7 注释全部订正） | warn | 覆盖率余量需故障注入框架（MVP 不引入）；views 组件挂载级测试 |
| panel-core | 3 项（**W1 契约钉死 int64 + 前端 number + formatUptime 人性化 + 5 用例**） | warn | EntryPathCard 挂载级用例（需 test-dom dev 依赖决策） |
| service-mgmt | 6 项 | warn | 挂载级用例；I4 归引擎所有者 |
| database-mgmt | 4 项 | warn | adapter 覆盖率环境绑定；挂载级用例 |
| cron-backup | 3 项 | warn | 挂载级用例；W3 环境绑定 |

修复后门控复跑：go build/vet/test + npm build 全过（run 内实跑）。**项目级终局：WARN**——全部可修项已闭环，余项均为带理由的显式延期（挂载级组件测试需引入 jsdom/happy-dom 开发依赖的决策、故障注入框架、实机环境绑定），不构成交付阻断。
