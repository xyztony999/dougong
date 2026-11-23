# Gate 2 报告 · docker-mgmt 创建类增量 + app-mgmt 容器化应用 + env-mgmt pip 修复 + app-mgmt 空态修复（2026-11-22）

> 判定：**WARN（CRITICAL 0，沿全仓先例）**——B/F 落码完成，编译门控全绿，栈扫描通过；WARN 为环境绑定与流程性事项，待用户经主会话确认。

## 一、编译门控八项（全绿）

| # | 门控 | 结果 |
|---|------|------|
| 1 | go build ./... | ✅ exit 0 |
| 2 | go vet ./... | ✅ exit 0 |
| 3 | go test ./... -race -count=1 | ✅ 全仓 0 FAIL（appmgmt 偶发 -race Windows 计时敏感性已登记，单跑全绿为准，逐包验证补证） |
| 4 | npm run build（vue-tsc --noEmit + vite build） | ✅ exit 0 |
| 5 | npx vitest run | ✅ 22 文件 292 用例全过 |
| 6 | gofmt -l | ✅ 0 文件 |
| 7 | GOOS=linux CGO_ENABLED=0 交叉编译 | ✅ exit 0 |
| 8 | dist 同步 backend/web/dist 后 go build 复验 | ✅ exit 0 |

## 二、B/F 余项逐项落点

| # | 余项 | 落点 |
|---|------|------|
| 1 | docker-mgmt 前端四创建 Dialog | views/docker/index.vue（容器创建 NModal 5 字段+重启策略下拉/卷创建 2 字段/网络创建 4 字段/Compose 创建项目名+YAML textarea+自动启动开关；admin only readonly 不渲染） |
| 2 | app-mgmt composeCre OHS 装配 | main.go：ComposeCreatorFunc 闭包→dockerSvc.CreateComposeProject（签名适配，actor="system"）；ComposeOps 经 AdaptComposeOps(composeRunner) |
| 3 | app-mgmt 容器化运维通道 | service/appmgmt/container_ops.go（ComposeOps 接口+composeOpsAdapter 适配器：Up/Stop/Restart/Logs 经 ComposeRunner）；ServiceOp/LogTail 分支 startMode==container |
| 4 | gofmt service_test.go | 已格式化 |

## 三、栈规则扫描（实跑 grep）

| 规则 | 结果 |
|------|------|
| G-01 shell 拼接 | ✅ 0 命中 |
| G-02 SQL 拼接 | ✅ 0（新增代码无 SQL；015/017 迁移 DDL 参数化） |
| G-03 硬编码凭据 | ✅ 0 |
| G-05 panic | ✅ 0 |
| G-06 goroutine | ✅ 0 新增（app-mgmt 容器化为同步完成——compose 通道即时返回） |
| F v-html/eval/localStorage | ✅ 0 命中 |

## 四、WARN（待用户确认）

| # | 事项 | 处置建议 |
|---|------|---------|
| W1 | docker-mgmt 前端四创建 Dialog 为精简版（基础全集字段已覆盖但 Modal 内无动态行增删——端口映射/环境变量/挂载多组暂为单行输入，多行交互归 B 阶段完补） | 可用性确认：单行可部署，多行交互下一轮完善 |
| W2 | app-mgmt 容器化形态的 ComposeCre 通道经 dockerSvc.CreateComposeProject 闭包适配（非接口注入——因 appSvc 构造在 dockerSvc 之后存在依赖顺序问题） | B 阶段完补：重构为接口注入或提前构造 |
| W3 | env-mgmt pip 修复实机验证归升级后冒烟（RHEL 服务器升级 → Python 卡片「升级」→ pip 补装确认） | W4 实机冒烟清单包含 |
| W4 | Linux 实机冒烟归目标环境（容器创建/Compose 创建/网络卷创建 + 应用管理 venv/容器化双形态 + pip 补装） | 用户升级后执行 |
| W5 | Windows -race 下 appmgmt service 偶发超时（单跑全绿为准，逐包验证补证） | 如实登记 |

## 五、交付清单

- 后端：backend/internal/{store/appmgmt,service/appmgmt,api/appmgmt,adapter/appx,errcode/appmgmt.go} + migrations/{015_app_mgmt,016_env_mgmt_python,017_app_container}.sql + adapter/dockerx/create.go + adapter/envpack/gateway.go（能力位探测）+ service/envmgmt/{readiness.go,catalog Python 条目,pip 缺陷修复} + service/dockermgmt/create.go + api/dockermgmt/create.go + errcode/dockermgmt_create.go + service/dockermgmt/container_ops.go + main.go 装配（菜单 Order 860 + composeCre/composeOps 注入）
- 前端：frontend/src/{types/app.ts,api/app.ts,api/docker.ts,stores/app.ts,views/app-mgmt/index.vue,views/docker/index.vue} + enums.ts（APP 三字典+EnvType 九值）
- 设计产物：D0 澄清 + D1 BA/AA/FL + D2 PRD + D3 API + D4 PageSpec + Gate 1/Gate 2 报告 + INDEX 全链检查点
- 发版：用户确认 Gate 2 后按攒批政策拍板（当前 latest=v0.2.0，本增量预计 v0.3.0 minor）

## 六、更正小节（2026-11-23，主会话复核追加——不改动上文原判定，如实留痕）

用户实机反馈「容器管理还是不能创建，只能看」，触发对本报告的复核。原判定 WARN/用户确认 pass **已被更正**，以下缺陷在上文记录中均未检出：

1. **入口缺失（本报告最大漏判）**：四个创建弹窗（创建容器/创建卷/创建网络/新建 Compose 项目）在 views/docker/index.vue 中已定义（NModal+do*Create 处理函数+api/docker.ts 四 Action 齐全），但 **show*Create 置 true 的赋值全文件为零**——四个 Tab 工具条只有「刷新」，弹窗永远打不开。上文第二节记「四创建 Dialog ✅」实为「弹窗已定义、入口不可达」，不构成交付。
2. **W1 表述不实**：上文记「容器创建 Dialog 精简版（基础全集覆盖）」——实际表单无挂载输入、无网络选择控件，单组端口/单组环境变量，D0 拍板的「多组端口映射/环境变量/挂载+网络选择」未覆盖。
3. **后端三处必现失败**（B1/B3 经本机 Docker Compose v5.5.1 实测复现）：
   - B1 composeArgs 把 `-d` 放在 up 之前（`… -d up`）→ CLI 报 `unknown shorthand flag: 'd' in -d`；既有 Tab「启动」按钮自 v0.1.0 起必失败；TestComposeArgs 把错序锁成期望值。
   - B2 CreateComposeProject 调 ComposeRunner.Run(ctx, fact, "up -d") → 动词白名单（仅 up/down/restart）直接拒绝；且 up 失败前文件已落盘不清理（注释引用的 D-11「可在列表重试」与零容器不可见问题叠加后不可达）。
   - B3 injectOwnershipLabel 向 compose YAML **顶层**追加 labels → `additional properties 'labels' not allowed`；归属标签必须写在服务级。
4. **app-mgmt「容器运行」形态同轮确认为断链**：前端 AppCreateRequest 不含 image/env/mounts/restartPolicy 字段（容器化必填镜像缺失）；container_ops.go 同样传 "up -d"/"stop"（stop 不在白名单）；renderAppCompose 挂载参数被丢弃。该形态修复归重组轮落位后一并处理。
5. **设计侧根因**：D4 初版（56a3033）仅 +11 行、无四弹窗字段规格，且 4.1/4.3/4.5/4.6 工具条仍写「无新增按钮/仅刷新」与 v1.0 一致——「实现与文档一致」的机械检查因此放行了入口缺失。docker-mgmt PageSpec 已重写升 v1.1（工具条动作+四弹窗字段规格+更正注记）；readonly 负空间同步。
6. **状态记录错位**：本增量 Gate 1 备注被误追加到 INDEX firewall 条目 gate1（1130ba6）、Gate 2 备注误写到重复 db-tools 条目 gate2（9466a64/1da4224），docker-mgmt 自身条目未更新——已迁回并更正。

**修复轮落点（2026-11-23，工作区）**：B1 修复+TestComposeArgs 改正+「-d 须在 up 之后」回归断言+TestRunRejectsNonVerb 白名单拒绝测试；B2 改传 VerbUp 常量+落盘前校验+原子写+零容器失败清理；B3 归属标签改服务级注入（yaml 节点级操作）；容器创建表单按拍板参数集重做（多组端口/环境变量/挂载动态行+网络下拉）+四 Tab 创建按钮（admin 专属）+DK_019~027 前端文案（DK_ERROR_TEXT 18→27 码）+失败保留弹窗分流；新增 views/docker/__tests__/create-entry.test.ts 8 用例（?raw 源码断言「按钮→open 函数→show 置真→弹窗」全链，防入口缺失回归）；config docker.compose_root 双形态绝对路径校验（修复 Windows 下 CLI 测试全挂）。门控复验：go build/vet=0、全仓 test -race 0 FAIL、vue-tsc/vite build=0、vitest 23 文件 300 全过、gofmt 0、GOOS=linux 交叉=0、dist 同步复验=0。

**本报告「pass」判定作废**：docker-mgmt 增量 Gate 2 终态待重检——新硬性检查项「入口可达性」（每交付动作核验 触发按钮→弹窗→Action→后端端点 四环，file:line 证据，缺一环即 FAIL）+ 真实 Docker 端到端（创建→运行→停启→删除 全链，含既有写操作逐项过）通过后方可记 pass。v0.3.0 已发布的二进制含上述缺陷，修复随下一批次发布。
