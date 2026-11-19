# PageSpec · database-mgmt · 只读用户（readonly）

| 属性 | 值 |
|------|-----|
| 版本 | v1.5 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-19（v1.5 db-tools 高级管理入口负空间增量） |
| 用户类型 | readonly |
| 唯一数据源 | 02-detailed/api/api-database-mgmt.md v2.4 |
| 公共页定义 | 复用 admin 文档（../admin/database-mgmt/database-mgmt.md v1.5）：登录/框架/守卫链（account-auth PageSpec 定义）；本模块无自助凭证操作页（凭据配置非本模块页面） |
| 边界依据 | ADR-018：全可看 + 禁一切变更 + 禁上传下载；D0 readonly 查看清单=实例/库/用户/状态概览/进程列表/慢日志（key 浏览不含——key 值属数据内容取出）；**v1.4 Q6 细化（D0 2026-11-18，沿 redis key 先例）：元数据（表清单/表结构/集合清单/库清单）两角色全可看；数据内容（行数据/文档/key 值）admin 专属——readonly 浏览页数据区呈现「数据内容仅管理员可见」负空间（不请求数据内容接口）；全部写操作 admin+CSRF，操作列整列不渲染** |
| 组件体系 | Vue 3 + Naive UI（同 admin 文档） |

## 第 1 节 模块元信息

| 项 | 值 |
|----|-----|
| 模块编码 | database-mgmt |
| 业务类型 | master_data（只读视角） |
| 页面类型 | db-list=config；mysql-manage=hybrid（三 Tab）；redis-manage=hybrid（两 Tab）；mongo-manage=hybrid（三 Tab，v1.2 追加）；**v1.4 追加：mysql-tables=config（只读）、mysql-table-detail=hybrid（结构 Tab 可见+数据浏览 Tab 负空间）、mongo-collections=config（只读）、mongo-documents=config（文档区负空间）** |
| 可见页面 | db-list（只读视图）、mysql-manage（库/用户/观测三 Tab 只读）、redis-manage（状态/慢日志两 Tab 只读）、mongo-manage（状态/数据库/用户三 Tab 只读，v1.2 追加）、**mysql-tables（表清单只读，v1.4）、mysql-table-detail（结构可见/数据区负空间，v1.4）、mongo-collections（集合清单只读，v1.4）、mongo-documents（文档区负空间，v1.4）** |
| 错误处理 | 同 admin 拦截器；AUTH_008 边界提示文案明确「只读角色禁止变更/上传下载操作」（ADR-018）；DBM_003/004 提示同 admin（只读观测不依赖凭据，DBM_004 仅治理入口相关——readonly 视图下呈现「待管理员配置凭据」标注）；**v1.4：DBM_022/025（表/集合不存在）toast+刷新同 admin；DBM_023/024/026/027/028 为写操作/数据内容专属错误码——readonly 无入口不触发（越界直达请求 AUTH_008 先于业务码拒绝）** |

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
| tableList | GET | /db/table/list | mysql-tables | 只读（元数据 Q6，v1.4 追加） |
| tableStructure | GET | /db/table/structure | mysql-table-detail·Tab 结构 | 只读（元数据 Q6，v1.4 追加） |
| mongoCollectionList | GET | /db/mcoll/list | mongo-collections | 只读（元数据 Q6，v1.4 追加） |

**负空间（readonly 不可见 Action，ADR-018/D0/Q6 矩阵，共 31 个，v1.2 +mongo 4、v1.4 +11）**：instanceRescan / instanceCredentials / instanceRemove / mysqlRootPassword / databaseCharsets（建库辅助）/ databaseCreate / databaseDelete / dbuserCreate / dbuserDelete / dbuserGrant / dbuserRevoke / backupExport（数据取出+禁下载）/ backupImport（变更+禁上传）/ redisPassword / redisKeyList / redisKeyTtl（key 值属数据内容取出，D0 readonly 清单未含）/ mongoUserCreate / mongoUserDelete / mongoUserGrant / mongoUserRevoke（变更类，v1.2 追加）/ **tableDrop（删表变更）/ rowList（行数据属数据内容取出，Q6 沿 key 先例）/ rowUpdate / rowDelete（行写变更，单列主键约束）/ mongoCollectionCreate / mongoCollectionDrop（集合变更）/ mongoDatabaseDrop（删库变更，系统库保护）/ mongoDocumentList（文档属数据内容取出，Q6）/ mongoDocumentInsert / mongoDocumentUpdate / mongoDocumentDelete（文档 CRUD 变更）（v1.4 追加 11 个）**——前端按 role===admin 隐藏入口/Tab/操作列 + 后端 AUTH_008 双保险；越界直达请求返回 403 语义页提示边界原因。

**跨模块负空间（v1.5，非本模块 31 负空间计数——跨模块 Action 引用）**：db-tools `dbtoolOpen`（POST /dbtool/access/open）+ `dbtoolInstall/dbtoolUpgrade/dbtoolUninstall/dbtoolRotate`（工具变更类）——高级管理入口（admin PageSpec 6.8）对 readonly 整体不渲染（⚠️A10 工具访问整体 admin 专属）；工具路径直访经 nginx gating 角色校验 401/403（网络层闸门，db-tools readonly PageSpec 4.3 三重防护登记）；**面板内轻量浏览（表/集合元数据）保留可达**（Q3 并存，Q6 数据内容仍 admin 专属负空间不变）。

**数据内容区负空间（v1.4 Q6 呈现口径，区别于入口隐藏）**：mysql-table-detail 数据浏览 Tab 与 mongo-documents 文档区对 readonly **可达但呈现负空间**——NEmpty「数据内容仅管理员可见」（不请求 rowList/mongoDocumentList，非禁用置灰）；元数据（表清单/结构/集合清单）正常呈现。与 FL v1.2「表数据浏览（负空间）/文档浏览（负空间）」菜单行逐项对应。

## 第 3 节 字典 Key 清单

与 admin 文档第 3 节完全一致（同一 enums.ts 常量源）：db_engine / db_conn_mode / instance_conn_state / grant_level / mongo_role（v1.2 +mongodb 值与 mongo_role，用于 mongo 用户 Tab 角色概况 tag 只读呈现）；颜色方案同源。

## 第 4 节 列表页

### 4.1 实例清单（db-list，只读视图）— 路由 /databases

- 查询条件/表格列：与 admin 版 4.1 完全一致（ADR-018 全可看；凭据列呈现「已配置/未配置」标注，无任何凭据值）。
- **v1.4 同款获益（F1/F2）**：TCP 优先呈现（天然一行 TCP，无归并展开 UI）+清单说明文案+connState/credentialsSet NTooltip 语义包裹+图例；redundantSocket=true 行「冗余 Socket」标注+tooltip 同 admin（只读呈现，无移除入口）。
- **差异**：无工具栏（无「重新探测」）；操作列仅「查看」入口（无配置凭据/移除按钮）；**「高级管理」入口不渲染（v1.5，跨模块 dbtoolOpen 负空间——工具访问整体 admin 专属 ⚠️A10，admin PageSpec 6.8）**。

## 第 5 节 表单页

无——本模块全部表单均为 admin 治理动作（凭据/建删/授权/改密/导入导出），readonly 无任何表单；公共页（登录/个人安全设置）复用 account-auth PageSpec 定义。

## 第 6 节 详情页（治理页只读视图）

### 6.1 MySQL/MariaDB 治理页（mysql-manage，只读）— 路由 /databases/mysql/:instanceId

- 页面头部：实例摘要条同 admin；**无**「修改 root 密码」按钮；unreachable NAlert 同 admin（只读提示）。
- **Tab 1 数据库**：表格=库名/字符集 + **「浏览」入口可见（v1.4 F3，进入 mysql-tables 表清单页——元数据 Q6）**；无其他操作列、无工具栏（导出/导入/删除负空间）；**「高级管理」入口不渲染（v1.5，dbtoolOpen 跨模块负空间 ⚠️A10）**；generatedAt 快照标注。
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
- **Tab 2 数据库**（v1.4 修订，ADR-031/D-25）：表格=库名/大小/集合数 + **「浏览集合」入口可见（进入 mongo-collections 集合清单页——元数据 Q6）**；**「删除库」负空间（mongoDatabaseDrop 不渲染，admin 专属）**；**「高级管理」入口不渲染（v1.5，dbtoolOpen 跨模块负空间 ⚠️A10）**；无工具栏（无独立建库入口同 admin）。
- **Tab 3 用户**：表格=用户名/认证机制/角色概况 tag（无工具栏/操作列；无密码字段）；变更类入口负空间（5.9~5.11 不渲染）。

### 6.4 MySQL 表清单页（mysql-tables，只读）— 路由 /databases/mysql/:instanceId/browse/:database（v1.4 追加）

- 进入：mysql-manage 库 Tab「浏览」入口；页面头部实例摘要条+库名标注+面包屑（同 admin 7.2，两角色同获益）。
- 表格列：与 admin 版 6.4 完全一致（表名/存储引擎/行数(估)/数据大小/排序规则/注释——元数据 Q6 全可看）；**操作列整列不渲染**（浏览数据/结构入口经表名行点击进入 mysql-table-detail；删除表负空间）。
- 无工具栏新建入口（本轮不做建表同 admin）；刷新保留（查询参数非变更）；空态同 admin。

### 6.5 MySQL 表详情页（mysql-table-detail，只读+数据区负空间）— 路由 /databases/mysql/:instanceId/browse/:database/:table（v1.4 追加）

- 页面头部：实例摘要条+库名/表名标注；singlePk 状态标注同 admin（只读呈现）。
- **Tab 1 数据浏览**：**负空间呈现（Q6/I-11）**——NEmpty「数据内容仅管理员可见」，**不请求 rowList**（行数据属数据内容取出，沿 redis key 先例）；无行编辑/行删除入口（负空间，非禁用置灰）；无分页器（无数据请求）。
- **Tab 2 结构**：与 admin 版 6.5 Tab 2 完全一致（列名/类型/可空/键/默认/额外/注释——元数据两角色全可看，tableStructure 正常请求）；无操作列。
- Tab 切换保留（结构 Tab 为默认可见内容；数据浏览 Tab 可达呈负空间）。

### 6.6 MongoDB 集合清单页（mongo-collections，只读）— 路由 /databases/mongo/:instanceId/browse/:database（v1.4 追加）

- 进入：mongo-manage 数据库 Tab「浏览集合」入口；页面头部同 admin。
- 表格列：与 admin 版 6.6 完全一致（集合名/文档数/大小——元数据 Q6）；**工具栏「新建集合」不渲染 + 操作列整列不渲染**（删除集合负空间）；「浏览文档」经集合名行点击进入 mongo-documents。
- 空态文案同 admin（隐式建库说明只读呈现）。

### 6.7 MongoDB 文档浏览页（mongo-documents，负空间）— 路由 /databases/mongo/:instanceId/browse/:database/:collection（v1.4 追加）

- 进入：mongo-collections 集合名行点击；页面头部实例摘要条+库名/集合名标注+面包屑。
- **文档区负空间（Q6/I-11）**：NEmpty「数据内容仅管理员可见」，**不请求 mongoDocumentList**（文档属数据内容取出）；无工具栏「插入文档」、无行编辑/删除入口、无分页器。
- 集合元数据（库名/集合名标注）可见——与 FL v1.2「文档浏览（负空间）」菜单行对应。

## 第 7 节 路由定义

| 路由 | meta | 与 admin 差异 |
|------|------|--------------|
| /databases、/databases/mysql/:instanceId、/databases/redis/:instanceId、/databases/mongo/:instanceId | 同 admin | 同路由只读渲染（按 authStore.user.role 分支：去工具栏/操作列/key Tab；mongo 页去用户 Tab 工具栏/操作列） |
| /databases/mysql/:instanceId/browse/:database、/databases/mysql/:instanceId/browse/:database/:table、/databases/mongo/:instanceId/browse/:database、/databases/mongo/:instanceId/browse/:database/:collection（v1.4 追加） | 同 admin（engineGuard 同链 + crumb meta） | 同路由只读渲染：表清单/集合清单去操作列与工具栏；表详情数据浏览 Tab 与文档浏览页呈数据内容负空间（6.5/6.7） |
| 菜单 | 「数据库」一级入口 | 无治理动作入口 |
| 面包屑（v1.4 F5） | 同 admin 7.2 crumb 链 | 两角色同获益（返回导航非变更能力）；动态段 dbStore+params 同源 |

## 第 8 节 全局一致性

| # | 检查 | 结果 |
|---|------|------|
| 1 | Action 与 admin 文档/API 文档一致，无 readonly 专属虚构接口 | ✅ 13/44 只读子集（v1.2 +mongo 3、v1.4 +tableList/tableStructure/mongoCollectionList 3） |
| 2 | 负空间 31 个 Action 与 ADR-018/D0/Q6 边界矩阵逐项对应（含 key 浏览×2、备份×2、凭据×3、mongo 变更×4、v1.4 表级×4[含 rowList 数据内容]、集合文档×7[含 mdocList 数据内容]） | ✅ 第 2 节（13+31=44 全集） |
| 3 | 只读视图与 admin 视图共用列定义（无列差异、仅按钮/Tab/数据区差异；观测 Tab 与结构 Tab 两视角全等） | ✅ 第 6 节 |
| 4 | CSRF/token 纪律同 admin（第 8 节 3/4 项）；readonly 无 POST Action | ✅ |
| 5 | key 浏览 Tab 不渲染（非禁用置灰——负空间呈现口径与 FL「界面不提供」一致）；**v1.4 数据内容区负空间同口径（rowList/mongoDocumentList 不请求，NEmpty「数据内容仅管理员可见」）** | ✅ 6.2/6.5/6.7 |
| 6 | 字典同源 enums.ts（v1.4 零新增） | ✅ |
| 7（v1.4） | 元数据入口（库行浏览/浏览集合）两角色可见与 Q6 一致；写操作列整列不渲染（tableDrop/mongoDatabaseDrop 等） | ✅ 6.1/6.3/6.4/6.6 |
| 8（v1.5） | 高级管理入口跨模块负空间（dbtoolOpen 等不渲染，⚠️A10）；工具路径 gating 网络层闸门登记；轻量浏览保留可达（Q3/Q6 不变）；本模块只读子集 13/44 与负空间 31 不变（跨模块引用非本模块 Action） | ✅ 第 2 节/4.1/6.1/6.3 |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-12 | initial | 首次生成：只读视角 3 页面（实例清单去操作按钮 + mysql 三 Tab 去写面 + redis 两 Tab 无 key）；7 可见 Action + 16 负空间标注 |
| v1.1 | 2026-11-12 | field_changed | Gate 1 W2 修正：第 1 节页面类型行 mysql-manage 计数笔误「两 Tab」→「三 Tab」（与第 21 行可见页面/第 6.1 节/FL/API 四方证据一致）；本模块 readonly 视角无 key 路径引用，F-1 改名不涉及本文档正文 | 1 |
| v1.2 | 2026-11-17 | interface_added | v0.1.4 反馈第 3 项 MongoDB 增量（API v2.0/admin PageSpec v1.2 同步）：+3 可见 Action（mongoStatus/mongoDatabaseList/mongoUserList，只读子集 10/30）+ 新页面 6.3 mongo-manage 只读视图（三 Tab：状态/数据库/用户——全部无工具栏无操作列）；负空间 +4（mongoUserCreate/Delete/Grant/Revoke）；字典同源 +mongo_role/+mongodb 值 | 1/2/3/6/7 |
| v1.4 | 2026-11-18 | interface_added / rule_changed | database-mgmt v1.4 用户实机反馈轮（API v2.3/admin PageSpec v1.4 同步，Q1~Q6；版本号对齐 admin v1.4——v1.3 DBM_005 诊断轮 readonly 无凭据配置面未涉及本文档，跳过 v1.3）：①+3 可见 Action（tableList/tableStructure/mongoCollectionList，元数据 Q6，只读子集 13/44）；②+4 页面只读视图（6.4 mysql-tables 去操作列/6.5 mysql-table-detail 结构 Tab 可见+数据浏览 Tab 负空间/6.6 mongo-collections 去工具栏操作列/6.7 mongo-documents 文档区负空间）+4 browse 路由同 admin；③负空间 20→31（+tableDrop/rowList/rowUpdate/rowDelete/mongoCollectionCreate/Drop/mongoDatabaseDrop/mongoDocumentList/Insert/Update/Delete 11 个，rowList/mongoDocumentList 属数据内容取出 Q6 沿 key 先例）；④rule_changed——6.1 库 Tab +浏览入口、6.3 数据库 Tab +浏览集合入口/删除库负空间（ADR-031 D-18 废止）、4.1 TCP 优先呈现+tooltip/图例+redundantSocket 标注同款获益（F1/F2）；⑤面包屑同 admin 7.2 两角色同获益（F5）；⑥数据内容区负空间呈现口径登记（NEmpty 不请求数据接口，与 FL v1.2 负空间菜单行逐项对应）；字典零新增 | 1/2/4/6/7/8 |
| v1.5 | 2026-11-19 | rule_added（跨模块负空间） | db-tools 高级管理入口负空间增量（admin PageSpec v1.5 同步，api-db-tools.md v1.0/v1.1）：①4.1 实例行/6.1 库行/6.3 库行「高级管理」入口不渲染（dbtoolOpen 跨模块负空间，⚠️A10 工具访问整体 admin 专属）；②第 2 节跨模块负空间声明（dbtoolOpen/install/upgrade/uninstall/rotate + gating 网络层闸门登记 + 轻量浏览保留可达 Q3/Q6 不变）；③第 8 节 +检查 8；**本模块只读子集 13/44 与负空间 31 不变**（跨模块引用非本模块 Action，api 零新增接口）；字典/路由零新增 | 2/4.1/6.1/6.3/8 |

> FX 清单执行方式同 admin 文档第 9 节注记（清单文件未随 payload 发布，按 9 节结构落实 + 第 8 节自查；全量留待 Gate 1）。
