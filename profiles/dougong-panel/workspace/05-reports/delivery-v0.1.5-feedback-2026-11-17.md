# 交付报告 — v0.1.4 用户反馈混合轮（env-mgmt / database-mgmt）

**执行日**：2026-11-17 · **执行模式**：增量（INDEX 全 done 无断点）· **执行轨**：session（本环境无 CreateWorkflow/Agent 工具，双轨探测降级结论，与前轮一致）
**注册表**：design/gate1/gate2=session（design/validation skill）；backend/frontend=subagent 资产按 session 轨执行（go-vue3 栈规约内联）
**范围**：F1 env-mgmt 前端刷新修复（D-lite）+ F2 dbmgmt 凭据 INTERNAL 修复（D-lite，高优）+ F3 dbmgmt MongoDB 增量（D1-D4→Gate 1→B/F→Gate 2 正规增量轮）

## 1. 各阶段与 Gate 结果

| 阶段 | 结果 | 产物 |
|------|------|------|
| D0 增量澄清 | 完成 | workspace/00-architecture/d0-clarification-2026-11-17-feedback.md（含拍板记录与 F2 根因实证） |
| F1/F2 直修 | 完成（先行于 Gate 1，bug 类先例） | 见 §3 文件清单；全绿（后端 race 全量/前端 181 用例） |
| D1 | 完成 | BA/AA/FL v1.1（+S-55 MongoDB 实例治理与观测） |
| D2 | 完成 | prd-database-mgmt v1.1（4.8 FR-21~23）+ domain-model v1.21 + 枚举池（db_engine +mongodb；mongo_role 入池）；doccenter 已再生成 |
| D3 | 完成 | api-database-mgmt v2.0（+7 接口/迁移 011/D-15~D-19；v1.2=同轮 F2 rule_added） |
| D4 | 完成 | PageSpec admin/readonly database-mgmt v1.2 + env-mgmt v1.2 双侧（S7 30/30、S8 5/5 机械校验过） |
| Gate 1 | WARN→确认通过 | 05-reports/gate1-v0.1.4-feedback-2026-11-17.md；自查修正 CRITICAL（mongo 路径四段式→三段式 mongo/mdb/muser）；W-1~4 用户经主会话确认 |
| B1-B7（F3 后端） | 完成 | dbmongo 适配器/迁移 011/30 接口路由/三族 I-05 重构/装配/测试 |
| F1-F4（F3 前端） | 完成 | mongo-manage 三 Tab 页+2 弹窗/字典/路由/30 Action |
| 编译门控 | 全绿 | vue-tsc 0 错 / vitest 181（17 文件）/ npm build / go build+vet / go test -race 全量 / dist 同步+重编译 / linux-amd64 交叉编译 |
| Gate 2 | WARN→确认通过 | 05-reports/gate2-v0.1.4-feedback-2026-11-17.md；CRITICAL 0/契约断裂 0；W-1~4 用户确认（体积 41.7MB/N+1 降级/clusterMonitor 指引/mongodump 二期） |
| 收尾 | 完成 | INDEX gates/validation note 落定（engine-cli check 11/11 PASS）+ CHANGELOG v0.1.5 + 本报告 |

## 2. F2 根因实证（复述存档）

translateGatewayErr 对 ErrUnavailable/ErrQueryFailed/ErrCommandFailed 三哨兵无映射 → response.Fail 兜底 INTERNAL/内部错误/data:null（与实拍一致）；触发面最可能=宿主机无 mysql 客户端（MySQL 跑容器仅发布 3306——面板 docker-mgmt 支持，实例由端口探测纳管）。用户「@ 破坏 DSN」假设证伪：全仓无 go-sql-driver/DSN 拼接，密码经 MYSQL_PWD/REDISCLI_AUTH env 注入。修复=DBM_021+translatePingErr+--protocol=TCP+DBM_022（mongo 权限不足）。

## 3. F3 拍板与实施要点

- 拍板（2026-11-17 经主会话）：Q1=A 对齐面（发现+凭据+库列表+用户管理三档角色+状态概览；集合/文档级浏览二期 OUT）；Q2=A mongo-driver Go 库（goproxy.cn 可达，v2.10.0，go.sum 锁定）。
- 实施内发现并修复 2 缺陷：①checkEngine 布尔二值下 redis 族接口误放行 mongodb 实例（api 测试实证→三族枚举重构）；②mongo-driver v2 顶层不导出拓扑错误类型（classify 改 mongo.IsNetworkError+文本判定，免 x/ 内部包）。
- CI 耦合第 6 起预防：mongo 连接类测试零 runner 实况（适配器纯函数/service-api 全替身/探测注入化）。

## 4. 文件清单

**设计产物（profiles/dougong-panel/workspace/）**：00-architecture/d0-clarification-2026-11-17-feedback.md（新）；{business,application}-arch-database-mgmt.md v1.1、function-list-database-mgmt.md v1.1；02-detailed/prd/prd-database-mgmt.md v1.1、domain-model.md v1.21、api/api-database-mgmt.md v2.0（含 v1.2 修复行）；02-detailed/interaction/{admin,readonly}/database-mgmt/database-mgmt.md v1.2、{admin,readonly}/env-mgmt/env-mgmt.md v1.2；01-solution/04-business-enum.md（db_engine+mongodb、mongo_role）；INDEX.md（阶段 note 增量+gates/validation 落定）；doc-center.json（再生成）；05-reports/gate1-*.md、gate2-*.md、delivery-*.md（本文件）。

**后端（backend/）**：go.mod/go.sum（+go.mongodb.org/mongo-driver/v2 v2.10.0）；internal/store/migrations/011_database_mongo.sql（新）；internal/store/dbmgmt/models.go（EngineMongoDB/IsMongoEngine）；internal/adapter/dbconn/target.go（枚举注释）；internal/adapter/dbmongo/gateway.go+gateway_test.go（新）；internal/adapter/dbmysql/gateway.go（--protocol=TCP）+gateway_test.go（新）；internal/errcode/dbmgmt.go（DBM_021/022）；internal/service/dbmgmt/{service,instance,prober,ports,consts,dto}.go（三族重构+mongo 装配/翻译/探测/审计码/DTO）+mongo.go+mongo_test.go（新）+branch_test.go/dbmgmt_test.go（扩展）；internal/api/dbmgmt/{handler,routes,module_test}.go（+7 接口）；cmd/dougong/main.go（mongoGw 装配）。

**前端（frontend/src/）**：stores/env.ts（终态跃迁重拉）+__tests__/env.test.ts（+2）；types/enums.ts（DB_ENGINE+mongodb/MONGO_ROLE）；types/database.ts（mongo 类型+DBM_021/022 文案）+__tests__/database.test.ts（新）；api/database.ts（+7 函数）；views/database-mgmt/{index.vue,render.ts,components/InstanceCredentialsModal.vue}；views/database-mgmt/mongo/index.vue+components/{MongoCreateUserModal,MongoRoleManageModal}.vue（新）；router/index.ts（mongo 路由+守卫三向）。

**根目录**：CHANGELOG.md（v0.1.5 小节）；backend/web/dist（同步重建）；release/dougong-linux-amd64（41.7MB 交叉编译产物）。

## 5. 构建验证（全绿）

go build ./... ✓ / go vet ./... ✓ / go test -race ./... 全量 ✓ / GOOS=linux GOARCH=amd64 构建 ✓ / vue-tsc ✓ / vitest 181 用例 ✓ / npm run build ✓ / dist 同步+embed 重编译 ✓ / engine-cli check 11/11 ✓。

## 6. 发布建议

- **建议发 v0.1.5**（CHANGELOG 已备）；F2 为用户被阻断的修复，宜尽快。
- git 操作（commit/push/tag/Release）由主会话统一执行（本轮不做）。
- 升级后建议验证四项（已写入 CHANGELOG）：①凭据保存成功或提示装 mysql-client；②任务完成后环境卡片自动刷新；③MongoDB 发现+凭据+库列表+用户管理全链；④权限不足时 DBM_022 提示。

## 7. 待跟进项（不阻塞发布）

- mongodump/mongorestore 备份导出导入（二期，mysqldump 先例可后继）；MongoDB 集合/文档级浏览（二期 OUT）。
- dbmgmt 其余未映射哨兵残余面（DDL 语境 ErrQueryFailed 仍透传 INTERNAL——非本轮用户路径，未扩码面；后续如遇可按 DBM_022 同法补齐）。
- DiscoveryProbeSet 无 mariadb 专属目标（Ubuntu MariaDB 与 mysql 同端口/同 socket 被归并为 mysql 档案——历史形态，非本轮引入；如需区分待后续轮）。
