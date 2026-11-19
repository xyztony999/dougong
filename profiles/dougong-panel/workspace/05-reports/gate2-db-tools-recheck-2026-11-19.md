# Gate 2 复检报告 — db-tools W1/W2 修复后独立复核（复跑确认轮）

**模块**：db-tools（ADR-032）+ env-mgmt Node 增量（W2 修复面）
**扫描路径**：backend/（D:/Project/dougong-panel/backend）+ frontend/src/（D:/Project/dougong-panel/frontend）
**检查时间**：2026-11-19（复检 ask 再次下达，全部命名检查于本轮重新实跑，非引用前次结果；只查不改）
**编号源**：engine-rules.md GR-01~06；go-vue3 栈 backend-rules.md G-01~07 / frontend-rules.md F-01~06；validation SKILL Gate 2 节 AL-001~005
**前序报告**：05-reports/gate2-db-tools-2026-11-19.md（判定 WARN：W1 菜单重复登记 / W2 env_type 四方缺口 / W3 覆盖率余量）
**复检范围**：W1/W2 闭环实证 + 编译门控全量重跑 + 安全五专项与 AL-001~005 无回归抽查 + W3 现状复测

---

## 一、W1 菜单重复登记 — 闭环确认（PASS，本轮重跑）

- 实跑 `grep -rn 'Key: *"db-tools"' --include="*.go" .`（backend/ 全仓）：**唯一命中 cmd/dougong/main.go:328**（Icon: "dbtool"，Order 550，两角色 Visibility），计数=1。
- 原第二登记点（前报告 main.go:462）已删除，现为主装配区注释「菜单已在构建期登记（icon=dbtool，Order 550）——此处不再重复 Register（W1 修复：MenuRegistry 纯 append 无去重…）」（main.go:461-462 实读）。
- 前端 MainLayout.vue:104 `dbtool: ConstructOutline` 图标映射在位（前轮实读，本轮菜单唯一性已复跑确认）。
- 结论：侧边栏双项根因（后端重复 append）已消除，W1 闭环。

## 二、W2 env_type 四方同步 — 闭环确认（PASS，本轮重跑）

### 2.1 四方取值机械 diff（本轮实跑）

`python -I D:/tmp/gate2-dbtools/enum2.py` 输出：

- **`env_type four-way equal: True`**——pool/api-env-mgmt/frontend/backend 四面均为 {apache, docker, mongodb, mysql, nginx, node, php_fpm, redis} 八值。
- 四面取证行号：池 04-business-enum.md:57；api-env-mgmt.md v1.3:135（枚举表）/:124（ENV_002 八值）/:342（DDL CHECK）；前端 enums.ts:599-608（本轮重读 EnvType 联合类型含 node）；后端 store/envmgmt/models.go:21-28 + 迁移 014_env_mgmt_node.sql:9（本轮重读 CHECK 八值逐字）。
- api 配套面（前轮实读在位）：NODE_LTS_TRACK=22（:152）、4.2.1 Node 目录条目（:268-274）、ENV_006 node 拒绝（:128/:234）、changelog v1.3（:371）。

### 2.2 前端测试与呈现门控（本轮重读）

- frontend/src/types/__tests__/env.test.ts:15-27：ENV_TYPE 八值键断言（含 node）。
- frontend/src/types/__tests__/dbtool.test.ts:65-69：node→「Node.js 环境未安装」+ unknown_env 兜底分支保留（本轮逐字重读）。
- frontend/src/views/env-mgmt/index.vue:290 服务态行 `v-if="item.installed !== false && item.unitName !== ''"`；:319 服务操作组 `<template v-if="item.unitName !== ''">`——Node 运行时型不渲染无效入口（⚠️A17，本轮 grep 重确认）。

### 2.3 PageSpec 与后端拒绝兜底（本轮重确认）

- admin/readonly PageSpec v1.4 标记计数 grep：admin 9 处 / readonly 6 处（Node 条目/服务操作不渲染/readonly 负空间，前轮逐行实读 :47/:79/:87/:89/:183 与 :34/:75）。
- 后端前置拒绝：service/envmgmt/dto.go:166-169（本轮重读）——单元映射空 → ENV_006 data.reason「运行时环境无常驻服务单元，服务操作不适用（node）」，先于已装校验与任何 systemd 调用。
- TestNodeServiceOpRejected（node_env_test.go:181）本轮含于全仓 `go test -race -count=1` 实跑 ok（service/envmgmt 6.336s）。

结论：W2 四面（api/PageSpec/前端 enums/测试）全部落定，四方同值机械核验 True，W2 闭环。

## 三、编译门控（本轮全部实跑）

| # | 命令（工作目录） | 结果 |
|---|------------------|------|
| C1 | `go build ./...`（backend/） | exit=0 |
| C2 | `go vet ./...`（backend/） | exit=0 |
| C3 | `go test ./... -race -count=1`（backend/，输出重定向 D:/tmp/gate2-recheck-test.log 后取 $?） | **exit=0，52 包 ok，0 FAIL**（api/dbtools 14.283s、service/dbtools 13.121s、store/dbtools 2.503s、toolx 1.275s、envpack 1.264s、service/envmgmt 6.336s、api/envmgmt 8.446s、store/envmgmt 3.619s） |
| C4 | `npx vue-tsc --noEmit`（frontend/） | exit=0 |
| C5 | `npm run build`（frontend/） | exit=0（vite built in 32.43s；chunk>500kB 警告既有形态 INFO） |
| C6 | `npx vitest run`（frontend/） | exit=0，**21 文件 265 用例全过** |
| C7 | `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./...`（backend/） | exit=0 |
| C8 | `gofmt -l cmd internal`（backend/） | 0 输出 |
| C9 | dist 同步核验 | frontend/dist ↔ backend/web/dist 全量 sha256 比对（排除 .gitkeep 占位）：**DIST_ASSET_HASH_MATCH=True**（index-sVEFpc7n.js / index-DErweyZa.js / index-B4cfUvMM.css）；backend/web/embed.go:12 `//go:embed all:dist` |

## 四、栈规则无回归复扫（本轮实跑）

- G-01：唯一命中 internal/adapter/cronx/runner.go:145（既有 cron-backup 豁免，本轮未改动）。
- G-02 Sprintf SQL：0 命中。G-03 硬编码凭据：0 命中。
- G-05：唯一 internal/api/server.go:43 embed 启动期不变式（规则允许）。
- G-06/G-07：改动面无新 goroutine；编译+vet+test 见 C1~C3。
- F-01~05（绝对路径重跑；首次相对路径调用因工作目录漂移 grep 报目录不存在，已作废重跑）：**全 0 命中**。

## 五、AL-001~005 db-tools 面无回归（本轮实跑）

`python -I D:/tmp/gate2-dbtools/check.py` + `enum2.py`：

| # | 结果 |
|---|------|
| AL-001 | controller↔api 3.3 矩阵 **11/11**，双向 diff ∅ |
| AL-002 | 前端 8 fetch Action↔controller 面板面 **8/8** 双向 ∅（reopen 302 非 fetch+内部 2 服务端调用沿前轮声明） |
| AL-003 | 可空字段 **9/9**（*T 无 omitempty 0 命中）；frontend-only={version} 为 types/dbtool.ts:186 函数形参误匹配（前轮复核，非 DTO 字段） |
| AL-004 | db_tool_type/db_tool_task_action/db_tool_task_status **四方 True**；db_engine 池==api==前端+后端子集 True；**env_type 四方 True** |
| AL-005 | DBT_001~012 三方相等，双向 diff ∅ |
| 审计码 | 6 个 dbt_* 三方同值（enum2.py 双向 diff ∅） |
| DDL | 迁移 013 两表 match=True+两索引在位；014 CHECK 八值↔api-env-mgmt.md:342 逐字一致（本轮重读） |

脚本口径：check.py CHECK 4/7 为转义管道/反引号正则解析局限（前轮已声明），以 enum2.py 修复版为准。

## 六、安全五专项无回归抽查（本轮重取证 + 回归本轮实跑）

- 回归文件在位（本轮 grep -l 确认）：service/dbtools/security_test.go、deploy_test.go、api/dbtools/module_test.go、service/envmgmt/node_env_test.go——TestSignonTampered/Expired/Replay/Binding、TestGateVerifyGuard、TestSignonVerifyGuard、TestOpenValidation、TestUninstallDataPreservation、TestInjectionRoundTripBothTools、TestNodeServiceOpRejected 全部含于 C3 全仓 -race 实跑 ok。
- S1：consts.go:30 `MongoExpressBind = "127.0.0.1"`（本轮重读）→ templates.go:228 单元 env（前轮实读）。
- S2：templates.go auth_request 计数=6（本轮 grep；含 :48/:52 双注入，前轮实读）。
- S3：signon.go:151 subtle.ConstantTimeCompare（本轮重读）。
- S4/S5：TestOpenValidation/TestUninstallDataPreservation 泄漏与白名单断言本轮实跑 ok。
- readonly：views/db-tools/index.vue:470 `v-if="authStore.isAdmin"`（本轮重读）。

## 七、W3 覆盖率现状（本轮 -count=1 复测，未修）

- `go test -count=1 -cover ./internal/service/dbtools/ ./internal/api/dbtools/` → **service/dbtools 73.0%**（<80% 目标，无回归）；**api/dbtools 73.5%**（≥60% 达标）。
- 修复员沿 account-auth W1 先例接受不修；安全核心专用回归全覆盖。**W3 仍为待用户确认项**——本轮无用户确认记录，gate2 维持 warn。

## 八、未执行项如实声明

- Linux 实机冒烟（phpMyAdmin/mongo-express 实装登录/真实 nginx auth_request/端口观测/卸载数据目录实查）：本轮 Windows 开发机无 Linux 运行时，未执行（归目标环境）。
- govulncheck（CI 周期项）：未执行。
- 工作区未提交改动 72 文件（本轮 `git status --short | wc -l`，含本轮外前轮文件与本报告）——沿历轮先例如实登记。

## 九、总结

**判定：WARN**（CRITICAL 0 / 契约断裂 0 / 安全底线无回归）

- **W1 闭环**：db-tools 菜单全仓唯一登记（main.go:328，本轮计数=1）。
- **W2 闭环**：env_type 八值四方同值（enum2.py 本轮实跑 True）；前端八值断言+node 文案+unitName 门控、PageSpec v1.4、后端 ENV_006 前置拒绝+回归全在位且本轮实跑过。
- 编译门控 C1~C9 本轮全量实跑全绿（52 包 -race 0 FAIL / vitest 265）；G/F 复扫 0 新违例；AL-001~005 零差；安全五专项无回归。
- **余 W3**（service/dbtools 73.0%<80%，沿先例接受不修）待用户经主会话确认后记 pass；Linux 实机冒烟归目标环境。
- 报告路径：05-reports/gate2-db-tools-recheck-2026-11-19.md
