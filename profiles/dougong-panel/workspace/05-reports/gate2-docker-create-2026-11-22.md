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
