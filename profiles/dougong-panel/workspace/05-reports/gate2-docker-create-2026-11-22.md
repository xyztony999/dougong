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

## 七、真实 Docker 端到端（2026-11-23，主会话实跑——修复后的验证）

**方式**：编译真实二进制（内嵌最新前端产物，字节级核对含新文案与四个创建入口文案），隔离数据目录与 Compose 落盘根，在本机 Docker Desktop（daemon 29.8.1 / Compose v5.5.1）上启动面板，走真实 HTTP：CLI 初始化管理员 → 登录取 CSRF → 逐项调用接口，并用 `docker inspect` 核对 daemon 侧真实效果。只创建 `dg-smoke-` 前缀资源，结束后与基线全面比对。

**结果：94 项全过，0 失败。** 覆盖与结论：

| 范围 | 结论 |
|------|------|
| 会话 | 登录、CSRF 校验（无令牌写请求被拒 AUTH_003 且无副作用）、daemon 接管探测 |
| 创建容器 | 创建即启动；端口映射/环境变量/挂载/重启策略/网络接入在 daemon 侧逐项核对生效 |
| 创建卷 / 创建网络 | daemon 侧存在，子网/网关生效，列表可见 |
| 新建 Compose 项目 | 粘贴 yml 落盘 + 自动 up 成功（B2 修复验证）；归属标签写在**服务级**，落盘文件通过 `docker compose config` 校验（B3 修复验证） |
| 既有写操作 | 容器 停止/启动/重启（进程已更换）/暂停/恢复/删除、资源统计、日志；Compose 重启/日志/停止（down）/**启动（up，B1 修复验证——修复前此处必失败）**；卷、网络、镜像删除 |
| 拒绝与校验 | 运行中删除 DK_004、卷被挂载 DK_008、网络被接入 DK_010、预定义网络 DK_011（bridge 未被误删）、各不存在码；字段校验 DK_019~022、DK_001/009/012、YAML 语法 DK_026（带行号）、项目名重复 DK_027 |
| 清理 | 无基线之外的新增容器/卷/网络/悬空镜像；基线资源全部仍在 |

**端到端过程中额外查出并已修复的两处问题**（均非本轮创建类代码引入）：

1. **网络删除拒绝归类错误（v0.1.0 起既有）**：真实 daemon 对「预定义网络」和「有活动端点的网络」都返回 403 Forbidden，网关层原按 errdefs 类别归类（Conflict→被接入、Forbidden→预定义），导致删除被容器接入的网络时用户看到的是「预定义网络不可删除」（DK_011）而非「被容器接入」（DK_010）——DK_010 在真实 daemon 下不可达。替身测试直接返回「被占用」错误故从未暴露。修复：`mapNetworkRemoveError` 按 daemon 报错原文区分（含 `pre-defined network` 为预定义，其余 Forbidden 为被接入），回归 `TestMapNetworkRemoveError`（用例文本取自探针实测原文）；端到端复验 DK_010/011/009 归类正确。依据：一次性探针实测 daemon 对网络/卷/镜像三类「被占用、预定义、不存在」的返回类别（卷与镜像为 409 Conflict，归类原本正确）。
2. **容器名提示文案与契约不符（本轮重做表单时引入）**：占位提示与 DK_019 文案写成「小写字母/数字/连字符」，而 api D-09 白名单为 `^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`（大写、下划线、点合法，与 Docker 原生命名规则一致），后端行为正确。已订正前端占位、DK_019 文案与 PageSpec 5.5。

**已知遗留（非本修复范围）**：Compose 项目 down 后零容器，列表按容器 label 聚合故不可见、无法再次 up（DK_014）、项目名被目录占住——用户已拍板随重组轮一并修（D0 Q4，d0-clarification-baota-restructure-2026-11-23.md）。

**未覆盖**：真实镜像拉取（本机已有基础镜像，未联网拉取）；WS 日志跟随；readonly 角色界面。

**过程记录（如实）**：端到端脚本前两次运行因脚本自身缺陷失败（cookie 文件路径未转换、JSON 解析函数竞态），其间一个负向用例创建了不带前缀的容器 `Bad_Name` 未被清理脚本覆盖，已手工删除并清除其悬空镜像层，机器恢复基线；脚本已修正并增加「基线之外有无新增」的全面比对。
