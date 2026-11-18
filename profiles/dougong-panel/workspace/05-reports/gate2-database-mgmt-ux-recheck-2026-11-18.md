# Gate 2 复检报告 — 代码→交付门控（database-mgmt v1.4 修复后复检）

| 属性 | 值 |
|------|-----|
| 模块 | database-mgmt（v1.4 用户实机反馈轮修复复检） |
| 复检对象 | 上轮 FAIL 报告 05-reports/gate2-database-mgmt-ux-2026-11-18.md（唯一 CRITICAL：E1/AL-005 DBM_029 跨层错误码断裂） |
| 契约基准 | 02-detailed/api/api-database-mgmt.md **v2.3.1**（44 接口 GET 18/POST 26；错误码 29=DBM_001~029） |
| 扫描路径 | backend/（D:/Project/dougong-panel/backend）、frontend/（D:/Project/dougong-panel/frontend） |
| 检查时间 | 2026-11-18 |
| 检查员 | Gate 2 复检员（只查不改；全部扫描与门控命令本轮实跑，非引用修复员自验） |
| 规则层 | engine-rules GR-01~06 ⊕ go-vue3 backend-rules G-01~07 / frontend-rules F-01~06 ⊕ validation skill Gate 2 AL-001~005 |

## 判定

**PASS** — 上轮唯一 CRITICAL（E1/AL-005）已闭环：DBM 错误码机械集合 diff BE=29/FE=29 双向零缺项，DBM_029 前端可识别（文案+reason 分支+两弹窗改判+防回归锚点用例），api §3.4 补记行在位（W1 同闭）。F-01~05/G-01~07 本轮实扫无回归，编译门控本轮实跑全绿，AL-001/002 三方机械 diff 44/44/44 IDENTICAL。余留 W2~W4 均存量非阻断项（沿历轮先例登记，见 §6）。

---

## 1. 重点复检：E1/AL-005 DBM_029 跨层同步闭环

### 1.1 机械集合 diff（本轮实跑，ask 指定方法）

命令（cwd=D:/Project/dougong-panel）：

```
grep -oE 'DBM_[0-9]{3}' backend/internal/errcode/dbmgmt.go | sort -u > be_dbm.txt
grep -oE 'DBM_[0-9]{3}' frontend/src/types/database.ts | sort -u > fe_dbm.txt
comm -23 be_dbm.txt fe_dbm.txt   # BE-only
comm -13 be_dbm.txt fe_dbm.txt   # FE-only
```

输出：**BE count=29，FE count=29；BE-only=空；FE-only=空；交集=29**。上轮唯一缺项 DBM_029 闭环（上轮 BE29/FE28）。

### 1.2 前端可识别链（本轮读码取证）

| 环节 | 证据 | 结果 |
|------|------|------|
| 文案映射 | frontend/src/types/database.ts:316 `DBM_029: '引擎操作被拒：当前凭据账户权限不足'`（DBM_ERROR_TEXT 29 键，:287-317） | ✓ |
| reason 分支 | types/database.ts:339-346 dbmErrorText 追加集纳入 `code === 'DBM_029'`（:342），data.reason 引擎原文追加（后端 WithData reason 对齐） | ✓ |
| 建用户弹窗 | MongoCreateUserModal.vue:91 `if (e.is('DBM_029'))` 专属内联指引（:92 需 userAdminAnyDatabase 提示）——上轮 :90 `e.is('DBM_022')` 死分支已改判 | ✓ |
| 角色管理弹窗 | MongoRoleManageModal.vue:57 本地 map `DBM_029: '引擎操作被拒：当前凭据账户权限不足'`——上轮 :56 旧 DBM_022 语义映射已改判 | ✓ |
| 旧语义残留排查 | `grep -rn DBM_022 frontend/src` 全部命中逐条核对：仅 TABLE_NOT_FOUND 合法语义（types:309 文案、tables.vue:98 删表刷新分流、table-detail.vue:83,129,194 回表清单分流、api/database.ts:309 注释、测试锚点）——零旧引擎权限语义残留 | ✓ |
| toastDbm 消费面 | collections.vue:45 / documents.vue:55 / mongo/index.vue:57 等仍 `dbmErrorText(e.code, e.data)`——DBM_029 不再返回 null，静默吞错路径消除 | ✓ |
| 防回归测试 | types/__tests__/database.test.ts:69-95 新增 4 用例：DBM_029 基础文案非 null（:70-72）/reason 附加+空 data 不附加（:74-80）/DBM_022 语义锚点不含「权限不足」（:82-85）/**DBM_001~029 逐码非 null 全覆盖锚点+键数=29 断言**（:87-94）；该文件本轮实跑 34 用例全过（上轮 30，+4 与修复员汇报一致） | ✓ |

### 1.3 后端实发链（维持，本轮复核）

errcode/dbmgmt.go:81 `ErrDBMEngineUnauthorized = New("DBM_029", …)`（:75-80 编号沿革注释 A-DB13）；service/dbmgmt/service.go:199-201 `errors.Is(err, dbmongo.ErrUnauthorized) → ErrDBMEngineUnauthorized.WithData(reason)`；adapter/dbmongo/gateway.go:333 classify code 13 → ErrUnauthorized。触达面 mongo/status、muser/*、v2.3 全部 mongo 治理接口不变。

### 1.4 文档侧 W1 闭环（本轮读码取证）

api-database-mgmt.md：头部版本 **v2.3.1**（:5）+最后更新行（:7）；§3.4 错误码总数声明 29（:196）；**DBM_029 ENGINE_UNAUTHORIZED 补记行在位**（:228，含语义/data.reason/触达面/编号沿革/「设计侧补记非新码」声明）；§6.1 changelog v2.3.1 行（:733，rule_added 补记，HTTP 接口/DTO/DDL/枚举零变更声明）。INDEX api note（:111）与 frontend note（:114）均已含 v2.3.1/DBM_029 同步记录（本轮 grep 取证）。

**结论：E1/AL-005 PASS，A-DB13「统一门控前须同步」项闭环。**

## 2. 编译门控（本轮实跑）

| 命令（cwd） | exit | 备注 |
|------|------|------|
| `go build ./...`（backend/） | 0 | — |
| `go vet ./...`（backend/） | 0 | — |
| `go test ./... -race -count=1`（backend/） | 0 | 全仓含测试包全 ok 0 FAIL（api/dbmgmt 9.441s、service/dbmgmt 15.989s、store/dbmgmt 1.857s、cmd/dougong 21.888s 等） |
| `npm run build`（=vue-tsc --noEmit && vite build，frontend/） | 0 | typecheck 绿 + 构建 27.76s（chunk 体积警告为既有 INFO） |
| `npx vitest run`（frontend/） | 0 | **19 文件 / 229 用例全过**（上轮 225，+4=DBM_029 闭环用例；含 types/database 34、stores/database 9、useDbCrumbs 11） |

ask 所述脚本门控（npm build/vitest/go test -race 均 exit=0）与本轮独立实跑结果一致。

覆盖率（本轮 `go test -count=1 -cover` 实跑）：service/dbmgmt **85.2%**（>80 ✓）、api/dbmgmt **66.3%**（>60 ✓）、store/dbmgmt 75.4%、adapter/dbmysql 33.4% / dbmongo 24.2%（引擎实联环境绑定归 Linux 冒烟，W3 沿先例）。

dist 同步核验（本轮实跑）：`diff -rq frontend/dist backend/web/dist` 仅差 `.gitkeep`（embed 占位，预期）；本轮 npm run build 重跑后复 diff 仍仅差 `.gitkeep`——产物确定性与同步态均确认。

## 3. 后端栈规则扫描 G-01~07（本轮实跑，无回归）

| 编号 | 检查 | 结果 | 证据 |
|------|------|------|------|
| G-01 | 无 shell 拼接 | PASS | `grep -rnE 'exec\.Command(Weak)?Context?\(.+"(-c\|/c)"' --include="*.go" .` 全仓唯一命中 internal/adapter/cronx/runner.go:145——`git diff -- backend/internal/adapter/cronx/` 0 行（存量 A-C2 登记豁免，非 dbmgmt）；dbmgmt 域 exec 4 处全字面量参数数组（dbmysql/gateway.go:396、backup.go:108,132、dbredis/gateway.go:204，本轮 grep 复核）；密码 MYSQL_PWD/REDISCLI_AUTH env 注入不进 argv（上轮已核，本轮代码未变） |
| G-02 | 无 SQL 拼接 | PASS | `grep -rnE "Sprintf\(.+(SELECT\|INSERT\|UPDATE\|DELETE)" --include="*.go" .` → 0 命中；引擎侧 dbmysql/table.go quoteIdent/quoteString+service 白名单双防线上轮已逐行复核（A-DB3/A-DB10），本轮修复面未触及 |
| G-03 | 无硬编码凭据 | PASS | `grep -rnEi "(password\|secret\|token)\s*=\s*\"[^\"]{8,}\"" --include="*.go" . \| grep -v _test` → 0 命中 |
| G-04 | SQL 只在 store 层 | PASS | 层外命中文件逐条核对：service 层 16 处命中**全部注释文本**（本轮逐行输出核对：dbmgmt/prober.go:18,54,120、ports.go:25、service.go:235 + auth/cronback/envmgmt/fileman/sitemgmt 存量注释）；真实引擎 SQL 仅 dbmysql 适配器（bootstrap.go/table.go/gateway.go，A-DB10 裁定）；`git status --porcelain -- backend/internal/store/` 空——面板 SQLite 侧零改动 |
| G-05 | 无 panic 滥用 | PASS | `grep -rn "panic(" --include="*.go" . \| grep -v _test` 全仓唯一 internal/api/server.go:43（启动期 embed 不变式，存量，规则允许；git diff 0 行） |
| G-06 | goroutine 退出路径 | PASS | `grep -rn "go func"` + `grep -rnE "^[[:space:]]*go [a-zA-Z]"`（排除 _test）：dbmgmt 五目录 **0 goroutine**；全仓站点均存量（main.go×10、dockerx/logs.go:165、dockerx/compose.go:279,371、dockermgmt/handler_ws.go:86、envmgmt/worker.go:44、terminal/service.go:256-257，历轮已核退出路径） |
| G-07 | 编译+vet+test 全绿 | PASS | 见 §2 本轮实跑（build=0/vet=0/test -race -count=1 全包 ok 0 FAIL） |

## 4. 前端栈规则扫描 F-01~06（本轮实跑，cwd=frontend/）

| 编号 | 检查 | 结果 | 证据 |
|------|------|------|------|
| F-01 | 无 Options API | PASS | `grep -rLn "script setup" --include="*.vue" src/` → 0 命中（exit=1） |
| F-02 | 无裸 any | PASS | `grep -rnE "(:[[:space:]]*any\|as any)" --include="*.ts" --include="*.vue" src/` → 0 命中 |
| F-03 | 请求只走 api 层 | PASS | `grep -rnE "(axios\|fetch)\(" --include="*.vue" src/views src/components` → 0 命中 |
| F-04 | 无混入其他组件库 | PASS | `grep -rnE "from '(antd\|element\|ant-design-vue)" src/` → 0 命中 |
| F-05 | 无原生弹窗 | PASS | `grep -rnE "(^\|[^a-zA-Z])(alert\|confirm)\(" --include="*.vue" --include="*.ts" src/` → 0 命中 |
| F-06 | typecheck+build+单测 | PASS | §2 实跑 npm run build=0（vue-tsc 含在内）+ vitest 229/229 |

## 5. 上轮其余项闭环/回归核验

| 项 | 上轮状态 | 本轮结果 | 证据 |
|----|----------|----------|------|
| AL-001/002 路径三方对齐 | PASS | PASS（本轮重跑机械 diff） | 脚本展开 api §3.3 紧凑矩阵行（create / delete 等）后三方排序 diff：routes.go（/db 前缀+相对路径提取）↔ api v2.3.1 §3.3 ↔ api/database.ts（request 默认 GET/method:'POST'、postDownload/postForm=POST）→ **IDENTICAL 44/44/44，双向零差**；权限分组本轮读 routes.go:39-96 逐行核对 readBoth 13（:41-53）/readAdmin 5（:61-65）/write 26（:70-96，AuthRequired→CsrfGuard→RequireRole(admin)）与 §3.3 矩阵全等 |
| AL-003 DTO 形状 | PASS（附 W2） | 无回归（修复面未触及 DTO） | W2 存量偏差本轮抽查维持：MongoStatusData 双侧扁平 dto.go:105,108（connectionsCurrent/opInsert）↔ types/database.ts:412,415 同型 |
| AL-004 枚举 | PASS | 无回归 | FE enums.ts:266-271 db_engine 四值（mysql/mariadb/redis/mongodb）本轮 grep 在位；修复面零枚举变更（api v2.3.1 changelog 声明一致） |
| R-02 CSRF/会话 | PASS | 无回归 | http.ts:115 X-CSRF-Token 注入本轮 grep 在位（postDownload/postForm 通道 :175,:234 同）；stores/auth.ts localStorage 仅 :3 注释「不落 localStorage」，零实际使用 |
| R-03/08/09/14、I-02、readonly 负空间、D-21、F1/F2/F5、GR-03 | PASS | 无回归（本轮修复面=FE 错误映射/两弹窗/测试/文档，未触及后端业务面） | 全仓 go test -race -count=1 本轮实跑全绿含 TestPermissionMatrix/TestTableGovernanceFlow/TestMongoCollDocFlow/prober_test 等上轮取证用例；G-01~06 本轮实扫零新增违例 |
| W1 api §3.4 缺 DBM_029 行 | WARN（E1 伴生） | **闭环** | 见 §1.4（v2.3.1 :228 补记行+:733 changelog） |

## 6. WARN / INFO 沿存（非阻断，均存量）

| # | 项 | 说明 |
|---|-----|------|
| W2 | MongoStatusData 文档形状偏差（存量） | api §4.5.2 文字嵌套 vs 双侧实现扁平——FE↔BE 对齐（本轮抽查 dto.go:105,108↔types:412,415），v0.1.4 轮 Gate 2 已判 PASS（gate2-v0.1.4-feedback-2026-11-17.md:46），归设计侧文档订正 |
| W3 | 引擎实联环境绑定（存量） | adapter/dbmysql 33.4%/dbmongo 24.2%（本轮实跑）——mysql/redis-cli/mongod 实联归 Linux 目标环境冒烟，沿历轮 WARN 先例 |
| W4 | 前端挂载级组件测试缺席（存量） | @vue/test-utils 不在 devDeps，沿 panel-core W4 先例；纯函数/store/composable 面本轮 54 用例覆盖（types 34+store 9+crumbs 11） |
| INFO | chunk 体积 | vite build 主 chunk 4.1MB 警告为既有形态（monaco/echarts/xterm），非本轮新增 |
| INFO | 工作区未提交改动 | B/F 产物均在工作区未提交（git status M），与历轮交付节奏一致，提交归工作流后续步骤 |

## 7. 总结

- 本轮实跑：AL-005 机械集合 diff（29/29 零缺项）+ AL-001/002 三方机械 diff（44/44/44）+ G-01~07 全项 + F-01~06 全项 + go build/vet/test -race -count=1 + 覆盖率 5 包 + npm run build + vitest 229 + dist 同步 diff。
- CRITICAL 违规：**0**（上轮 E1 闭环）。
- WARN：3 沿存（W2~W4，均存量非阻断，历轮已登记/确认先例）；INFO：2。

**判定：PASS** — E1/AL-005 修复闭环，全部 CRITICAL 检查本轮实跑通过，放行交付。
