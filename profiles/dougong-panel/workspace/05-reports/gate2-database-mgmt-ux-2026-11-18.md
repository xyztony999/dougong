# Gate 2 验证报告 — 代码→交付门控（database-mgmt v1.4 增量）

| 属性 | 值 |
|------|-----|
| 模块 | database-mgmt（v1.4 用户实机反馈轮：api v2.3 / PageSpec v1.4×2 / prd v1.2 / domain-model v1.23） |
| D0 基准 | 00-architecture/d0-clarification-database-mgmt-ux-2026-11-18.md（Q1~Q6） |
| 契约基准 | 02-detailed/api/api-database-mgmt.md v2.3（44 接口，GET 18/POST 26） |
| 扫描路径 | backend/（D:/Project/dougong-panel/backend）、frontend/（D:/Project/dougong-panel/frontend） |
| 检查时间 | 2026-11-18 |
| 检查员 | 交付检查员（只查不改；全部扫描与门控命令本轮实跑） |
| 规则层 | engine-rules GR-01~06 ⊕ go-vue3 backend-rules G-01~07 / frontend-rules F-01~06 ⊕ 业务规则 R-02/03/08/09/14、I-02（api-database-mgmt.md §2.2）、Q6/D-24/I-11 |

## 判定

**FAIL** — CRITICAL 1 项（AL-005：DBM_029 跨层错误码断裂，A-DB13 裁定的「统一门控前须同步」项未闭环）。其余检查全过。修复后重跑本 Gate。

---

## 1. 编译门控（本轮实跑，非引用脚本）

| 命令（cwd） | exit | 备注 |
|------|------|------|
| `go build ./...`（backend/） | 0 | — |
| `go vet ./...`（backend/） | 0 | — |
| `go test ./... -race -count=1`（backend/） | 0 | 全仓含测试包全 ok 0 FAIL（api/dbmgmt 10.08s、service/dbmgmt 19.58s、store/dbmgmt 2.05s 等） |
| `npm run build`（=vue-tsc --noEmit && vite build，frontend/） | 0 | typecheck 绿 + 构建 28.82s（chunk 体积警告为既有 INFO） |
| `npx vitest run`（frontend/） | 0 | 19 文件 / 225 用例全过（含 types/database 30、stores/database 9、useDbCrumbs 11） |

ask 所述脚本门控（go build/vet/test -race、npm build、vitest 均 exit=0）与本轮实跑结果一致，无 stderr 归属问题。

覆盖率（本轮 `go test -count=1 -cover` 实跑）：service/dbmgmt **85.2%**（>80 ✓）、api/dbmgmt **66.3%**（>60 ✓）、store/dbmgmt 75.4%、adapter/dbmysql 33.4% / dbmongo 24.2%（引擎实联环境绑定归 Linux 冒烟，沿历轮 WARN 先例）。

## 2. 后端栈规则扫描 G-01~07（backend-rules §7 方法实跑）

| 编号 | 检查 | 结果 | 证据 |
|------|------|------|------|
| G-01 | 无 shell 拼接 | PASS | `grep -rnE 'exec\.Command(Weak)?Context?\(.+"(-c\|/c)"' --include="*.go" .` 全仓唯一命中 internal/adapter/cronx/runner.go:145——**存量**（git log 初始发布即有，本轮 diff 零改动），A-C2 登记豁免（cron 命令原文为独立 argv 元素零拼接，与 cron 自身执行语义同信任级，tasks.md T07），非本模块。全部 exec.Command 站点人工核对：dbmgmt 域 4 处（dbmysql/gateway.go:396、backup.go:108,132、dbredis/gateway.go:204）全字面量参数数组；新增 dbmysql/table.go 零新增 exec，经既有 g.query 通道（R-08）；密码 MYSQL_PWD/REDISCLI_AUTH env 注入不进 argv（gateway.go:397、backup.go:109,133、dbredis/gateway.go:205） |
| G-02 | 无 SQL 拼接 | PASS | `grep -rnE "Sprintf\(.+(SELECT\|INSERT\|UPDATE\|DELETE)" --include="*.go" .` → 0 命中。引擎侧 SQL（新增 dbmysql/table.go）人工复核：标识符 quoteIdent 反引号双写（parse.go:9-11）+ 值 quoteString 全量转义（parse.go:22-46，' " \ \n \r NUL 0x1a）；service 白名单前置 validateDatabaseName/validateTableName `^[A-Za-z0-9_]+$`≤64（identity.go:184,191-197,249-255）；LIMIT/OFFSET strconv.Itoa 服务端整数（table.go:221-223）；行值 string\|null 服务端校验（service/table.go:146-151）——A-DB3/A-DB10 裁定口径 |
| G-03 | 无硬编码凭据 | PASS | `grep -rnEi "(password\|secret\|token)\s*=\s*\"[^\"]{8,}\"" --include="*.go" . \| grep -v _test` → 0 命中 |
| G-04 | SQL 只在 store 层 | PASS | `grep -rlnE "(SELECT\|INSERT\|UPDATE\|DELETE) " --include="*.go" \| grep -v internal/store \| grep -v _test` 命中逐条核对：service 层全部为注释文本（dbmgmt/prober.go:18,54,120、ports.go:25、service.go:235 等）；真实引擎侧 SQL 仅 dbmysql 适配器（A-DB10 裁定：DbGateway 契约通道非面板 SQL）；面板 SQLite 侧本轮**零新增 SQL/零 DDL/零迁移**（git status 无 internal/store 改动，api §5.2/5.3 声明一致；rescan 清理复用既有 DELETE 参数绑定） |
| G-05 | 无 panic 滥用 | PASS | `grep -rn "panic(" --include="*.go" \| grep -v _test` 全仓唯一 internal/api/server.go:43（启动期 embed 不变式，存量，规则允许）；增量 0 |
| G-06 | goroutine 退出路径 | PASS | `grep -rn "go func"` + `grep -rnE "^[[:space:]]*go [a-zA-Z]"`（排除 _test）：dbmgmt 五目录（service/adapter×3/api）**0 goroutine**；全仓站点均存量（main.go 编排×10、dockerx、terminal、envmgmt worker，历轮已核退出路径）；prober.go diff 无新增并发原语 |
| G-07 | 编译+vet+test 全绿 | PASS | 见 §1 本轮实跑（build=0/vet=0/test -race 全包 ok） |

## 3. 前端栈规则扫描 F-01~06（frontend-rules §6 方法实跑）

| 编号 | 检查 | 结果 | 证据（cwd=frontend/） |
|------|------|------|------|
| F-01 | 无 Options API | PASS | `grep -rLn "script setup" --include="*.vue" src/` → 0 命中 |
| F-02 | 无裸 any | PASS | `grep -rnE "(:[[:space:]]*any\|as any)" --include="*.ts" --include="*.vue" src/` → 0 命中（文档值用递归 MongoDocValue/unknown 收窄，types/database.ts:632-641） |
| F-03 | 请求只走 api 层 | PASS | `grep -rnE "(axios\|fetch)\(" --include="*.vue" src/views src/components` → 0 命中 |
| F-04 | 无混入其他组件库 | PASS | `grep -rnE "from '(antd\|element\|ant-design-vue)" src/` → 0 命中 |
| F-05 | 无原生弹窗 | PASS | `grep -rnE "(^\|[^a-zA-Z])(alert\|confirm)\(" --include="*.vue" --include="*.ts" src/` → 0 命中（强确认全 NPopconfirm/NModal） |
| F-06 | typecheck+build 全绿 | PASS | §1 实跑 npm run build=0（vue-tsc --noEmit 含在内）+ vitest 225/225 |

## 4. 前后端契约对齐 AL-001~005（api v2.3 为基准）

| 编号 | 检查 | 结果 | 证据 |
|------|------|------|------|
| AL-001 | Controller 路径/方法 vs API 文档 | PASS | 机械 diff：routes.go 提取 44 条（GET/POST+路径）排序 vs api §3.3 矩阵 44 条清单 → `diff` 零差（IDENTICAL 44/44）。权限分组逐行核对：readBoth 13（含 v2.3 元数据 table/list、table/structure、mcoll/list）/ readAdmin 5（charsets、key×2、row/list、mdoc/list）/ write 26（admin+CsrfGuard+RequireRole，routes.go:39-96）与 §3.3 矩阵全等；全三段式 /db/{resource}/{action} |
| AL-002 | 前端 Action 路径/方法 vs Controller | PASS | 机械 diff：api/database.ts 逐函数解析（request 默认 GET/method:'POST'、postDownload/postForm=POST）44 条 vs 同清单 → 零差（IDENTICAL 44/44）；admin PageSpec §2 Action 表 grep 计数=44；readonly PageSpec 13 可见+31 负空间=44 |
| AL-003 | DTO 逐形状（含可空） | PASS（附 W2 存量偏差） | v2.3 全部新 DTO 双侧逐形状核对：TableItem↔DataTableItem（rowEstimate/dataSizeBytes *int64↔number\|null、collation/comment *string↔string\|null，后端**无 omitempty** 显式 null，dto.go:362-374）；ColumnItem↔TableColumnDef（default/comment 可空同型）；TableStructureData（singlePk bool/pkColumn *string↔string\|null）；RowListData（rows []map[string]any↔Record<string, string\|number\|boolean\|null>，total/page/pageSize/generatedAt）；RowUpdateRequest values（service 校验 string\|null/不含主键/列名∈结构列集）↔Record<string,string\|null>；mcoll/mdb/mdoc 全 req/data（document json.RawMessage↔MongoDocument、id/insertedId any↔MongoDocValue、matched/deleted/affected int64↔number）；additive redundantSocket bool↔boolean、merged int↔number。instance/list 可空字段 omitempty+TS `?:` 与 api「?:string\|null」可选形态一致。**W2（存量）**：MongoStatusData 双侧扁平（connectionsCurrent/opInsert…，dto.go:102-112↔types/database.ts:405-415）vs api §4.5.2 文字嵌套 connections{}/opcounters{}——FE↔BE 对齐，v0.1.4 轮 Gate 2 已按 FE↔BE 判 PASS（gate2-v0.1.4-feedback-2026-11-17.md:46），文档形状偏差延续，归设计侧订正 |
| AL-004 | 枚举四方同值 | PASS | 5 枚举逐值核对：db_engine（DDL 011 CHECK 'mysql','mariadb','redis','mongodb' = api §3.4 = 池 04-business-enum.md:44 = FE enums.ts:267-272；BE 探测集 consts.go:100-109 同值）/ db_conn_mode（socket,tcp 四方）/ instance_conn_state（reachable,unreachable 四方）/ grant_level（readonly,write,all：dbmysql/gateway.go:289-291 = api D-10 = 池:47 = FE:289-294）/ mongo_role（read,readWrite,dbAdmin：mongo.go:114,217 校验 = api D-19 = 池:48 = FE:297-302）。v2.3 零新增枚举（表引擎/排序规则实况开放集合不入池，api §3.4 声明与 FE 注释一致） |
| AL-005 | 错误码双侧+包络 | **FAIL（E1）** | 包络对齐：response.go:19-21 {code,message,data} ↔ http.ts code===0→data 否则 ApiError；POST X-CSRF-Token 统一注入（http.ts:115）。DBM_001~028 双侧 28/28 文案映射在位（types/database.ts:287-316）。**但后端实发 29 码**：errcode/dbmgmt.go:75-80 ErrDBMEngineUnauthorized=**DBM_029**（A-DB13 由 DBM_022 改号），service.go:199-201 将 dbmongo.ErrUnauthorized（code 13）翻译上线——机械集合 diff：BE 29 / FE 28，唯一缺项 **DBM_029**。详见 §6 E1 |

## 5. 业务规则适用项 + I-02 + readonly 负空间 + D-21

| 项 | 结果 | 证据 |
|----|------|------|
| R-02（Cookie 会话+CSRF） | PASS | routes.go:68-69 POST 组 AuthRequired→CsrfGuard→RequireRole(admin)；FE http.ts:3,115 HttpOnly dg_session same-origin + X-CSRF-Token 内存态注入；stores/auth.ts:3,19 csrfToken 仅 Pinia 内存不落 localStorage |
| R-03（变更审计同事务脱敏） | PASS | 动作码 28=13 基础+4 mongo_user+1 instance_bootstrapped+10 新增（consts.go:20-49，与 D-26 清单逐字一致）；新写操作 txAuditOnly 独立 SQLite 事务（service.go:302-319，A-DB16 引擎侧不可入面板事务口径）；instance_merged 与 DELETE 同事务（prober.go:211-228）；detail 脱敏逐处核对：row_updated={instanceId,database,table,pkValue,columns[列名]}（table.go:162-165）/row_deleted 无行值（:190-193）/mdoc 三码={…,id/insertedId} 无文档内容（mongogov.go:166-169,212-215,243-246）/instance_merged={engine,identityKey,reason:"redundant_socket_tcp_reachable"}（prober.go:219-221）/credentials={engine,connMode} 无密码（instance.go:199-201） |
| R-08（参数数组） | PASS | 见 G-01；新增 table.go 全部语句经 g.query 字面量参数数组通道（table.go:13 注释+59,89,131,139,152,185 调用点），零新增 exec |
| R-09（零外呼） | PASS | 新增四文件 grep http.Get/http.Post/http.NewRequest → 0 命中；dbmongo governance.go 仅 withClient 连本机纳管目标（进程内 driver，D-15） |
| R-14（服务端校验不信任前端） | PASS | 标识符白名单（identity.go:191-267：库名/表名/集合名〔禁 $ 前缀/空字符/system.〕）；分页 parsePagination 非整数/<1→AUTH_012、超限截断（identity.go:271-293）；values 非空/不含主键/列名∈结构/string\|null（table.go:128-153）；pkValue 1~1024（table.go:254-257）；单列主键服务端判定 I-10（table.go:240-253,263-276，前端禁用态仅镜像）；JSON 服务端校验 DBM_026（mongogov.go:295-312，UseNumber 保精度+拒尾随内容）；_id 不可变更（mongogov.go:196-204）；系统库 DBM_028 硬校验先于库存在（mongogov.go:105-107）；前端双层拦截镜像（parseDocumentJson/validateCollectionName/clampBrowsePageSize，types/database.ts:716-775） |
| I-02（凭据只进不出） | PASS | password 字段仅存于 4 个请求 DTO（dto.go:53,150,186,266），全部响应 DTO 零密码/密文/密钥字段；credUsername 可见为 api §3.3 显式允许（账户名非秘密）；db-cred.key 仅 vault.go/consts.go 引用，无接口下发；审计 detail 无凭据（见 R-03）；slog 凭据泄漏 grep → 0 命中 |
| readonly 负空间（Q6/D-24/I-11） | PASS | 前端：tables.vue:157 操作列仅 isAdmin push（表名行点击两角色可达）；table-detail.vue:111 loadRows `!authStore.isAdmin→return`（不请求 rowList）+:296 `<n-empty v-else description="数据内容仅管理员可见">`+readonly 无分页器+默认结构 Tab（:61-69）；documents.vue:69 同守卫+:296 NEmpty 整区负空间；collections.vue:146 操作列/:229 新建集合仅 admin；mongo/index.vue:168 删除库 admin only+:176 系统库禁用态镜像；mysql/index.vue:202-261 浏览入口两角色+:475 readonly「待管理员配置凭据」标注；index.vue:274,307 rescan/凭据/移除 admin only。后端双保险：TestPermissionMatrix（module_test.go:656+）断言 readonly 元数据 GET 三接口全通、row/list+mdoc/list→AUTH_008、v2.3 全部 9 写接口携合法 CSRF→AUTH_008、缺 CSRF→AUTH_003——本轮 -race 实跑全过 |
| D-21（Q1 TCP 优先两阶段+保守清理） | PASS | prober.go:59-116 两阶段（阶段 1 TCP 记录 TCPReachable[engine]；阶段 2 allowInsert=!TCPReachable[engine]，既有 socket 档案 conn_state 仍整值刷新 :127-166）；MergeRedundantSockets 仅 socket+!CredentialsSet()+tcpReachable[engine] DELETE+审计同事务（:196-232），已配凭据保留、零自动迁移；instance.go Rescan +merged additive、List redundantSocket 派生（本机 TCP 档案可达，isLocalTCPHost localhost/127.0.0.1/::1，远程不触发）；prober_test.go 7 测试函数本轮 -race 全过 |
| F1/F2/F5 呈现（Q2/Q5） | PASS | TCP_FIRST_LEGEND 图例（index.vue:289）+redundantSocketTag 标注（index.vue:151、render.ts）；connStateTag/credentialsTag NTooltip 语义包裹（render.ts:58-70，文案常量 types/database.ts:739-748，治理页摘要条复用）；MainLayout n-breadcrumb（:234-239）+useDbCrumbs 纯函数（crumb 链数据库→治理·identity→库→表/集合，末级不可点，identity 取 dbStore 档案缺失兜底 instanceId）；router 4 browse 路由 engineGuard+crumb meta（router/index.ts:186-207）；useDbCrumbs 11 vitest 用例过 |
| GR-03（E2E 补测） | PASS | 新增全栈用例 TestTableGovernanceFlow（module_test.go:1016）/TestMongoCollDocFlow（:1102）覆盖 14 新接口完整链路+权限矩阵；service/adapter 新增 table_test 5+8、mongogov_test 4、governance_test 6、prober_test 7 测试函数；本轮 go test -race 全绿 |

## 6. CRITICAL 发现

### E1（AL-005 / GR-05）DBM_029 跨层错误码断裂 —— A-DB13 门控前同步项未闭环

**事实链（全部本轮读码/实跑取证）**：
1. 后端实发 DBM_029：errcode/dbmgmt.go:75-80 `ErrDBMEngineUnauthorized = New("DBM_029", "引擎操作被拒（凭据账户权限不足）")`；service.go:199-201 `errors.Is(err, dbmongo.ErrUnauthorized) → ErrDBMEngineUnauthorized.WithData(reason)`；dbmongo/gateway.go classify code 13 → ErrUnauthorized。触达面：mongo/status、muser/* 与 **v2.3 全部 mongo 治理接口**（mcoll/mdoc/mdb/dropdb）在凭据账户权限不足时均返回 DBM_029。
2. 前端不可识别：机械集合 diff（`grep -oE 'DBM_[0-9]{3}'` 双侧排序 comm）BE=29 / FE=28，唯一缺项 DBM_029。DBM_ERROR_TEXT（types/database.ts:287-316）无 DBM_029 → `dbmErrorText` 返回 null → 全域 `toastDbm` 模式 `if (text !== null) message.error(text)` **静默吞错**（index.vue:76、mongo/index.vue:57、collections.vue:45、documents.vue:55、CreateCollectionModal.vue:74、DocumentEditModal.vue:108、tables.vue:46、table-detail.vue:54 等）——用户操作失败无任何反馈。
3. 旧映射残留：MongoCreateUserModal.vue:90 `e.is('DBM_022')` 权限不足分支成死分支（后端已改发 DBM_029，落入泛型 e.message 兜底，专属指引丢失）；MongoRoleManageModal.vue:56 本地 map `DBM_022: '引擎操作被拒…'` 语义过期（DBM_022 现为 TABLE_NOT_FOUND）且无 DBM_029 项。
4. 裁定依据：backend/tasks.md:1048（A-DB13）/1085/1092 明示「前端 types/database.ts DBM_022 文案映射/MongoCreateUserModal/MongoRoleManageModal 的 DBM_022 判定须改 DBM_029……**工作流统一门控前须同步**」。前端 v1.4 轮已交付（INDEX frontend note）但该同步项遗漏；frontend/src 全仓 grep DBM_029 = 0 命中（含测试）。
5. 文档侧：api-database-mgmt.md §3.4 错误码表止于 DBM_028，无 DBM_029 行（A-DB13 建议设计侧补记，未落）。

**修复清单（返回 B/F 阶段）**：
- F：types/database.ts DBM_ERROR_TEXT +DBM_029 文案（并评估 reason 追加集 :337-338 纳入 DBM_029，后端带 data.reason）；MongoCreateUserModal.vue:90 与 MongoRoleManageModal.vue:56 的 DBM_022 判定/映射改 DBM_029；types/__tests__/database.test.ts 补 DBM_029 用例。
- D：api §3.4 补记 DBM_029（ENGINE_UNAUTHORIZED）行 + 6.1 changelog（设计侧）。
- 修复后重跑本 Gate（AL-005 机械集合 diff 须 29/29）。

## 7. WARN / INFO（非阻断，沿历轮先例登记）

| # | 项 | 说明 |
|---|-----|------|
| W1 | api §3.4 缺 DBM_029 行 | E1 的文档侧伴生项（设计侧补记，随 E1 修复闭环） |
| W2 | MongoStatusData 文档形状偏差（存量） | api §4.5.2 文字嵌套 connections{current,available}/opcounters{…} vs 双侧实现扁平 connectionsCurrent/opInsert…——FE↔BE 对齐且 v0.1.4 轮 Gate 2 已判 PASS（gate2-v0.1.4-feedback-2026-11-17.md:46），非本轮增量；归设计侧文档订正 |
| W3 | 引擎实联环境绑定（存量） | adapter/dbmysql 33.4%/dbmongo 24.2%——mysql/redis-cli/mongod 实联归 Linux 目标环境冒烟（Windows 开发机纯函数+注入替身覆盖），沿历轮 WARN 先例 |
| W4 | 前端挂载级组件测试缺席（存量） | 新增 4 页面/3 弹窗无 @vue/test-utils 挂载用例（devDeps 不含，沿 panel-core W4 先例）；纯函数/store/composable 面 50 用例已覆盖（types 30+store 9+crumbs 11） |
| INFO | chunk 体积 | vite build 主 chunk 4.1MB 警告为既有形态（monaco/echarts/xterm），非本轮新增 |
| INFO | 工作区未提交改动 | 本轮 B/F 产物均在工作区未提交（git status M/??），与历轮交付节奏一致，提交归工作流后续步骤 |

## 8. 总结

- 检查执行：G-01~07（7）+ F-01~06（6）+ AL-001~005（5）+ R-02/03/08/09/14（5）+ I-02 + readonly 负空间 + D-21 + F1/F2/F5 + GR-03，全部本轮实跑/读码取证。
- 通过：除 AL-005 外全部 CRITICAL 检查项。
- CRITICAL 违规：**1**（E1 DBM_029 跨层断裂）。
- WARN：4（W1~W4，均存量或 E1 伴生）；INFO：2。

**判定：FAIL** —— 阻断交付，返回 F（+D 文档补记）修复 E1 后重跑本 Gate。
