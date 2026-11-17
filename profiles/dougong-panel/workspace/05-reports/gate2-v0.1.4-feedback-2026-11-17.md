# Gate 2 验证报告 — 代码→交付门控（v0.1.4 用户反馈混合轮）

**模块**：env-mgmt（F1）+ database-mgmt（F2 修复 / F3 MongoDB 增量）
**扫描路径**：backend/（internal+cmd）+ frontend/src/
**检查时间**：2026-11-17
**执行方式**：session 轨（validation skill Gate 2 + go-vue3 栈 rules——G-01~G-07 grep 方法与前端 selfcheck，沿本仓先例；Java/Escm 专属 BE-xx/F3-xx 按栈适配豁免）

## 0. 范围

| 项 | 形态 | 实施摘要 |
|----|------|---------|
| F1 | 前端修复 | envStore 任务详情「在途→终态」跃迁触发 loadList+loadTasks（轮询纪律不变） |
| F2 | 后端修复 | DBM_021 新码+translateGatewayErr/translatePingErr 映射补齐+connArgs --protocol=TCP |
| F3 | 全栈增量 | dbmongo 适配器（mongo-driver v2）+迁移 011+7 接口+mongo-manage 页+三族 I-05 重构+DBM_022 |

## 1. 后端规则扫描（CRITICAL，G-01~G-07 栈适配清单）

| 编号 | 检查项 | 结果 | 说明 |
|------|--------|------|------|
| G-01 | 无 shell 拼接 | PASS | exec.Command 全景 11 文件逐一核对：dbmongo **零 exec 调用**（进程内 driver——本轮新增面无该攻击面）；cronx runner.go `sh -c <原文单参数>` 为 cron 模块既有设计豁免（历轮 Gate 2 已登记，R-08 例外语义：用户 crontab 原文必须经 shell）；其余全部字面量参数数组 |
| G-02 | 无 SQL 拼接 | PASS | grep 0 命中；迁移 011 SQL 在 store/migrations（embed，DDL 非 query）；新代码零 SQL |
| G-03 | 无硬编码凭据 | PASS | grep 0 命中；dbmongo 凭据仅内存态 options.Credential（⚠️A18：不落日志/不进响应/不进 URI） |
| G-04 | SQL 只在 store 层 | PASS | 本轮新增零 store 外 SQL |
| G-05 | 无 panic 滥用 | PASS | 仅 api/server.go embed 启动期不变式（既有登记）；新增代码零 panic |
| G-06 | goroutine 有退出路径 | PASS | 新增代码零 goroutine（dbmongo 每操作同步建连即断，D-15） |
| G-07 | 编译+vet+test 全绿 | PASS | go build ./... ✓ / go vet ./... ✓ / go test -race ./... 全量绿（含 14 个 api 包）；linux/amd64 交叉编译 ✓（release/dougong-linux-amd64） |

## 2. 前端规则扫描（栈 selfcheck 适配）

| 区域 | 结果 | 说明 |
|------|------|------|
| vue-tsc --noEmit | PASS | 0 错误 |
| vitest 全量 | PASS | 17 文件 181 用例全绿（+6 新增：env 跃迁 2+S8 锚点 4 组） |
| npm run build | PASS | 生产构建通过（chunk 体积警告为既有形态） |
| CSRF 纪律 | PASS | 4 个新 POST（muser/*）经统一 request() 通道（http.ts 拦截器注入 X-CSRF-Token，与既有 13 POST 同管线） |
| 凭据零回显 | PASS | mongo 建用户密码 type=password 不回显；用户清单无密码字段（I-02） |
| readonly 负空间 | PASS | mongo 页三 Tab readonly 渲染：无新建用户按钮/无角色管理/无删除（role 判断，ADR-018） |
| dist 同步 | PASS | frontend/dist → backend/web/dist（go:embed 源）已同步并重编译 |

## 3. 前后端接口对齐（AL-001~005）

| 编号 | 检查项 | 结果 | 说明 |
|------|--------|------|------|
| AL-001 | Controller 路径 vs api 文档 v2.0 | PASS | 7 新路由（/db/mongo/status、/db/mdb/list、/db/muser/{list,create,delete,grant,revoke}）三方一致（routes.go ↔ api §3.3/§4.5.2 ↔ 文档 grep 复核），三段式 G1-08 全过 |
| AL-002 | 前端 Action 路径 vs Controller | PASS | api/database.ts 7 函数路径逐一 equals 后端注册（grep 机械比对 7/7） |
| AL-003 | DTO vs 前端类型 | PASS | MongoStatusData/MongoDatabaseItem{name,sizeBytes,collections}/MongoUserItem{username,mechanisms,roles{db,role}}/create·grant·revoke payload 字段名逐一对应（dto.go ↔ types/database.ts） |
| AL-004 | 枚举值前后端一致 | PASS | db_engine 四值（DB_ENGINE 含 mongodb=green/success）；mongo_role 三档（MONGO_ROLE ↔ dbmongo.MongoRoleValid 同值 read/readWrite/dbAdmin）——types/__tests__ S8 锚点用例固化 |
| AL-005 | 错误码前端可识别 | PASS | DBM_021（凭据弹窗内联+dbmErrorText）/DBM_022（mongo 页内联弹窗+dbmErrorText reason 附加）；DBM_009/010/011 复用既有文案 |

## 4. 测试覆盖（本轮增量清单）

| 层 | 新增/扩展 | 说明 |
|----|----------|------|
| 适配器（dbmongo） | gateway_test.go：4 组 | classify 错误码表（18/13/11/51003/其余/文本穿透/连接层）+connectURI（tcp/socket %2F 编码——凭据不进 URI 实证锚点）+MongoRoleValid+Available 恒 nil |
| 适配器（dbmysql） | gateway_test.go 新建：2 组 | connArgs --protocol=TCP（localhost socket 特例修复）+classifyRun 哨兵回归 |
| service（dbmgmt） | mongo_test.go：5 组（fakeMongo 替身）；branch_test +3 组；env 无后端改动 | 观测三路+DBM_018 双向/用户治理全流程（DBM_009/010/011 语义——含 revoke 引擎不报错时 service 前置判据）/SetCredentials mongo 分支（特殊字符密码逐字达网关）/探测 mongo 分支（注入 reachable）/翻译表 mongo 哨兵/translatePingErr/用户实拍形态回归（ErrUnavailable→DBM_021、ErrQueryFailed→DBM_006） |
| api（dbmgmt） | module_test +2 | 权限矩阵 mongo 扩展（readonly 3 GET 通/写 AUTH_008）+mongo 治理 HTTP 全流程（CSRF/角色白名单/跨族 DBM_018——该用例暴露并驱动 checkEngine 三族重构） |
| 前端 | env.test.ts +2；types/__tests__/database.test.ts 新建 | 终态跃迁重拉/首拉即终态不重拉；DBM_021/022 文案与 reason；字典 S8 锚点 |

**CI 环境耦合预防（第 6 起）**：mongo-driver 连接类测试零 runner 实况依赖——适配器仅纯函数（不 dial）；service/api 全替身；探测经注入（27017 仅以 URI 组装字面量出现在断言中，grep 复核）；既有 mysql/redis 连接类测试维持既有注入格局未新增实况耦合。

## 5. 实施中发现并修复的缺陷（本轮内闭环）

| # | 缺陷 | 修复 |
|---|------|------|
| D-1 | checkEngine 布尔二值（MySQL 族/其余）：mongodb 入列后 redis 族接口误放行 mongodb 实例（api 模块测试实证：/db/redis/info 对 mongodb 实例返回 0） | 重构三族枚举判定（familyMySQL/familyRedis/familyMongo），governTarget(ID) 签名联动；DBM_018 双向用例固化 |
| D-2 | mongo v2 顶层包不导出拓扑错误类型 | classify 改用 mongo.IsNetworkError+文本形态判定，免 x/ 内部包依赖 |

## 6. WARN 项（供用户确认）

| # | 项 | 说明 | 建议 |
|---|----|------|------|
| W-1 | 二进制体积增长 | mongo-driver v2 引入后 linux/amd64 单二进制 ~41.7MB（此前 ~33MB 量级）——纯 Go 静态链接的预期代价 | 接受（Q2=A 拍板面已含） |
| W-2 | dbStats 逐库 N+1 | mdb/list 对每库追加一次 dbStats 取集合数（库数量级个位~十位，实测面可忽略；dbStats 失败该库集合数降级 0 不阻断） | 接受 |
| W-3 | serverStatus 权限门槛 | 状态概览需凭据账户具 clusterMonitor 类权限（Unauthorized→DBM_022 如实呈现并指引）——面板文档口径：配置 admin 级凭据 | 接受（DBM_022 内联已指引） |
| W-4 | mongodump 备份二期 | MongoDB 备份导出/导入未含（设计 OUT；mysqldump 先例可后继） | 接受（Q1=A 拍板面已含） |

## 7. 总结

**判定：WARN（4 项建议项待用户确认；CRITICAL 0；契约断裂 0）**

- 通过：G-01~G-07 全过（cronx 既有豁免）、前端 selfcheck 全过、AL-001~005 全过（7/7 机械比对）
- CRITICAL 违规：0
- 实施内缺陷 2 项均已当场修复并以用例固化（D-1 三族重构/D-2 错误类型适配）
- 构建门控：vue-tsc ✓ / vitest 181 ✓ / npm build ✓ / go build+vet ✓ / go test -race 全量 ✓ / dist 同步 ✓ / linux 交叉编译 ✓

**用户确认 W-1~W-4 后交付**（INDEX 收尾与发布建议随确认一并落地）。
