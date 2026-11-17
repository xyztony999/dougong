# PageSpec · database-mgmt · 只读用户（readonly）

| 属性 | 值 |
|------|-----|
| 版本 | v1.2 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-17 |
| 用户类型 | readonly |
| 唯一数据源 | 02-detailed/api/api-database-mgmt.md v2.0 |
| 公共页定义 | 复用 admin 文档（../admin/database-mgmt/database-mgmt.md）：登录/框架/守卫链（account-auth PageSpec 定义）；本模块无自助凭证操作页（凭据配置非本模块页面） |
| 边界依据 | ADR-018：全可看 + 禁一切变更 + 禁上传下载；D0 readonly 查看清单=实例/库/用户/状态概览/进程列表/慢日志（key 浏览不含——key 值属数据内容取出） |
| 组件体系 | Vue 3 + Naive UI（同 admin 文档） |

## 第 1 节 模块元信息

| 项 | 值 |
|----|-----|
| 模块编码 | database-mgmt |
| 业务类型 | master_data（只读视角） |
| 页面类型 | db-list=config；mysql-manage=hybrid（三 Tab）；redis-manage=hybrid（两 Tab）；mongo-manage=hybrid（三 Tab，v1.2 追加） |
| 可见页面 | db-list（只读视图）、mysql-manage（库/用户/观测三 Tab 只读）、redis-manage（状态/慢日志两 Tab 只读）、mongo-manage（状态/数据库/用户三 Tab 只读，v1.2 追加） |
| 错误处理 | 同 admin 拦截器；AUTH_008 边界提示文案明确「只读角色禁止变更/上传下载操作」（ADR-018）；DBM_003/004 提示同 admin（只读观测不依赖凭据，DBM_004 仅治理入口相关——readonly 视图下呈现「待管理员配置凭据」标注） |

## 第 2 节 Action 定义

| Action | 方法 | 路径 | 页面 | 说明 |
|--------|------|------|------|------|
| instanceList | GET | /db/instance/list | db-list | 只读视图 |
| mysqlStatus | GET | /db/mysql/status | mysql-manage·Tab 观测 | 只读 |
| mysqlProcesslist | GET | /db/mysql/processlist | mysql-manage·Tab 观测 | 只读（D1 解释性决策：与慢日志同敏感级开放） |
| databaseList | GET | /db/database/list | mysql-manage·Tab 库 | 只读 |
| dbuserList | GET | /db/dbuser/list | mysql-manage·Tab 用户 | 只读（凭据不回显，D1 解释性决策） |
| redisInfo | GET | /db/redis/info | redis-manage·Tab 状态 | 只读 |
| redisSlowlog | GET | /db/redis/slowlog | redis-manage·Tab 慢日志 | 只读 |
| mongoStatus | GET | /db/mongo/status | mongo-manage·Tab 状态 | 只读（v1.2 追加） |
| mongoDatabaseList | GET | /db/mdb/list | mongo-manage·Tab 数据库 | 只读（v1.2 追加） |
| mongoUserList | GET | /db/muser/list | mongo-manage·Tab 用户 | 只读（凭据不回显，v1.2 追加） |

**负空间（readonly 不可见 Action，ADR-018/D0 矩阵，共 20 个，v1.2 +mongo 4）**：instanceRescan / instanceCredentials / instanceRemove / mysqlRootPassword / databaseCharsets（建库辅助）/ databaseCreate / databaseDelete / dbuserCreate / dbuserDelete / dbuserGrant / dbuserRevoke / backupExport（数据取出+禁下载）/ backupImport（变更+禁上传）/ redisPassword / redisKeyList / redisKeyTtl（key 值属数据内容取出，D0 readonly 清单未含）/ mongoUserCreate / mongoUserDelete / mongoUserGrant / mongoUserRevoke（变更类，v1.2 追加）——前端按 role===admin 隐藏入口/Tab + 后端 AUTH_008 双保险；越界直达请求返回 403 语义页提示边界原因。

## 第 3 节 字典 Key 清单

与 admin 文档第 3 节完全一致（同一 enums.ts 常量源）：db_engine / db_conn_mode / instance_conn_state / grant_level / mongo_role（v1.2 +mongodb 值与 mongo_role，用于 mongo 用户 Tab 角色概况 tag 只读呈现）；颜色方案同源。

## 第 4 节 列表页

### 4.1 实例清单（db-list，只读视图）— 路由 /databases

- 查询条件/表格列：与 admin 版 4.1 完全一致（ADR-018 全可看；凭据列呈现「已配置/未配置」标注，无任何凭据值）。
- **差异**：无工具栏（无「重新探测」）；操作列仅「查看」入口（无配置凭据/移除按钮）。

## 第 5 节 表单页

无——本模块全部表单均为 admin 治理动作（凭据/建删/授权/改密/导入导出），readonly 无任何表单；公共页（登录/个人安全设置）复用 account-auth PageSpec 定义。

## 第 6 节 详情页（治理页只读视图）

### 6.1 MySQL/MariaDB 治理页（mysql-manage，只读）— 路由 /databases/mysql/:instanceId

- 页面头部：实例摘要条同 admin；**无**「修改 root 密码」按钮；unreachable NAlert 同 admin（只读提示）。
- **Tab 1 数据库**：表格=库名/字符集（无操作列、无工具栏）；generatedAt 快照标注。
- **Tab 2 用户**：表格=用户名/host/授权概况 tag（无操作列、无工具栏；无密码字段）。
- **Tab 3 状态与进程**：与 admin 版 6.1 Tab 3 完全一致（状态概览 NDescriptions + 进程列表只读表格——本 Tab 无任何操作语义，两视角全等）。

### 6.2 Redis 治理页（redis-manage，只读）— 路由 /databases/redis/:instanceId

- 页面头部：实例摘要条；**无**「设置密码」按钮。
- **Tab 1 状态概览**：与 admin 版一致（info 白名单 NDescriptions）。
- **Tab 2 慢日志**：与 admin 版一致（条数选择保留——查询参数非变更）。
- **Tab 3 key 浏览**：**不渲染**（admin 专属 Tab，D0 口径；菜单/Tab 均无入口）。

### 6.3 MongoDB 治理页（mongo-manage，只读）— 路由 /databases/mongo/:instanceId（v1.2 追加）

- 页面头部：实例摘要条（engine tag green）；无任何动作按钮。
- **Tab 1 状态概览**：与 admin 版一致（serverStatus 白名单 NDescriptions）。
- **Tab 2 数据库**：表格=库名/大小/集合数（无工具栏/操作列——admin 版亦无，D-18 只读投影）。
- **Tab 3 用户**：表格=用户名/认证机制/角色概况 tag（无工具栏/操作列；无密码字段）；变更类入口负空间（5.9~5.11 不渲染）。

## 第 7 节 路由定义

| 路由 | meta | 与 admin 差异 |
|------|------|--------------|
| /databases、/databases/mysql/:instanceId、/databases/redis/:instanceId、/databases/mongo/:instanceId | 同 admin | 同路由只读渲染（按 authStore.user.role 分支：去工具栏/操作列/key Tab；mongo 页去用户 Tab 工具栏/操作列） |
| 菜单 | 「数据库」一级入口 | 无治理动作入口 |

## 第 8 节 全局一致性

| # | 检查 | 结果 |
|---|------|------|
| 1 | Action 与 admin 文档/API 文档一致，无 readonly 专属虚构接口 | ✅ 10/30 只读子集（v1.2 +mongo 3） |
| 2 | 负空间 20 个 Action 与 ADR-018/D0 边界矩阵逐项对应（含 key 浏览×2、备份×2、凭据×3、mongo 变更×4 等） | ✅ 第 2 节 |
| 3 | 只读视图与 admin 视图共用列定义（无列差异、仅按钮/Tab 差异；观测 Tab 两视角全等） | ✅ 第 6 节 |
| 4 | CSRF/token 纪律同 admin（第 8 节 3/4 项）；readonly 无 POST Action | ✅ |
| 5 | key 浏览 Tab 不渲染（非禁用置灰——负空间呈现口径与 FL「界面不提供」一致） | ✅ 6.2 |
| 6 | 字典同源 enums.ts | ✅ |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-12 | initial | 首次生成：只读视角 3 页面（实例清单去操作按钮 + mysql 三 Tab 去写面 + redis 两 Tab 无 key）；7 可见 Action + 16 负空间标注 |
| v1.1 | 2026-11-12 | field_changed | Gate 1 W2 修正：第 1 节页面类型行 mysql-manage 计数笔误「两 Tab」→「三 Tab」（与第 21 行可见页面/第 6.1 节/FL/API 四方证据一致）；本模块 readonly 视角无 key 路径引用，F-1 改名不涉及本文档正文 | 1 |
| v1.2 | 2026-11-17 | interface_added | v0.1.4 反馈第 3 项 MongoDB 增量（API v2.0/admin PageSpec v1.2 同步）：+3 可见 Action（mongoStatus/mongoDatabaseList/mongoUserList，只读子集 10/30）+ 新页面 6.3 mongo-manage 只读视图（三 Tab：状态/数据库/用户——全部无工具栏无操作列）；负空间 +4（mongoUserCreate/Delete/Grant/Revoke）；字典同源 +mongo_role/+mongodb 值 | 1/2/3/6/7 |

> FX 清单执行方式同 admin 文档第 9 节注记（清单文件未随 payload 发布，按 9 节结构落实 + 第 8 节自查；全量留待 Gate 1）。
