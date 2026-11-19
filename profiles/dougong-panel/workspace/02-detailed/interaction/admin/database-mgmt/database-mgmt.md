# PageSpec · database-mgmt · 管理员（admin）

| 属性 | 值 |
|------|-----|
| 版本 | v1.5 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-19（v1.5 db-tools 高级管理入口增量） |
| 用户类型 | admin |
| 唯一数据源 | 02-detailed/api/api-database-mgmt.md v2.4（本模块接口）+ 02-detailed/api/api-db-tools.md v1.0（跨模块高级入口消费：dbtoolList/dbtoolOpen） |
| 组件体系 | Vue 3 + Naive UI（knowledge/architecture-choices 栈版本表；无激活栈 profile，不使用 vue3-engine 栈 Scm* 组件） |
| 确认方式 | headless run：7 项交互确认依据 D3 契约（api-database-mgmt.md）+ ADR-018 + FL v1.0 推导，标注于各节 |

## 第 1 节 模块元信息

| 项 | 值 | 确认依据 |
|----|-----|---------|
| 模块编码 | database-mgmt（域前缀 /db） | api 3.1 |
| 业务类型 | master_data（实例档案：无 MQ、无状态机流转——conn_state 为探测态；库/用户为引擎实况投影） | 分册 2.1 判据；api 4.1.2 |
| 页面类型 | db-list=config（查询+表格+行内弹窗）；mysql-manage=hybrid（三 Tab 独立区块）；redis-manage=hybrid（Tab 切换，分册 2.2「列表页有 Tab 切换」等价应用于治理页）；mongo-manage=hybrid（三 Tab，v1.2 追加）；**v1.4 追加 4 页面：mysql-tables=config（表清单+行内弹窗）、mysql-table-detail=hybrid（数据浏览/结构两 Tab）、mongo-collections=config（集合清单+行内弹窗）、mongo-documents=config（文档 JSON 分页+编辑弹窗）** | 分册 2.2 |
| UI 框架 | Naive UI（NDataTable/NTabs/NForm/NModal/NPopconfirm/NTag/NDescriptions/NUpload） | 栈版本表 |
| 状态管理 | Pinia：authStore（role/csrfToken）+ dbStore（当前实例档案缓存） | 栈版本表 |
| 错误处理 | 统一拦截器按 api 3.4 分流：DBM_002→返回列表+toast；DBM_003→提示实例不可达（启停归 service-mgmt，引导跳转）；DBM_004→引导「配置凭据」；DBM_005/006/021→凭据弹窗内联错误（不落库语义；DBM_021=通道不可用 v0.1.4 修复轮新增）；DBM_019→强提示「密码已修改但面板同步失败」+引导重新配置凭据；DBM_016→提示操作在途稍后重试；AUTH_008→边界原因提示；**v1.4 新增分流（api v2.3 DBM_022~028）：DBM_022 表不存在→toast+刷新表清单；DBM_023 无单列主键→行写入口禁用态 tooltip 如实提示原因（服务端拒绝时内联「该表无单列主键，不支持行级编辑/删除」）；DBM_024 行不存在→toast「行已不存在或已被修改」+刷新数据页；DBM_025 集合不存在→toast+刷新集合清单；DBM_026 JSON 非法→编辑弹窗内联校验错误（前端拦截为先，服务端兜底）；DBM_027 文档不存在→toast「文档已不存在」+刷新文档页；DBM_028 系统库保护→删除库弹窗内联「系统库（admin/config/local）不可删除」** | api 3.4/1.3 |

## 第 2 节 Action 定义（与 API 一一对应）

| Action | 方法 | 路径 | 页面 | 权限 | CSRF |
|--------|------|------|------|------|------|
| instanceList | GET | /db/instance/list | db-list | admin+readonly | — |
| instanceRescan | POST | /db/instance/rescan | db-list | admin | ✓ |
| instanceCredentials | POST | /db/instance/credentials | db-list | admin | ✓ |
| instanceRemove | POST | /db/instance/remove | db-list | admin | ✓ |
| mysqlStatus | GET | /db/mysql/status | mysql-manage（Tab 观测） | admin+readonly | — |
| mysqlProcesslist | GET | /db/mysql/processlist | mysql-manage（Tab 观测） | admin+readonly | — |
| mysqlRootPassword | POST | /db/mysql/root-password | mysql-manage（Tab 观测） | admin | ✓ |
| databaseList | GET | /db/database/list | mysql-manage（Tab 库） | admin+readonly | — |
| databaseCharsets | GET | /db/database/charsets | mysql-manage（Tab 库·建库弹窗） | admin | — |
| databaseCreate | POST | /db/database/create | mysql-manage（Tab 库） | admin | ✓ |
| databaseDelete | POST | /db/database/delete | mysql-manage（Tab 库） | admin | ✓ |
| dbuserList | GET | /db/dbuser/list | mysql-manage（Tab 用户） | admin+readonly | — |
| dbuserCreate | POST | /db/dbuser/create | mysql-manage（Tab 用户） | admin | ✓ |
| dbuserDelete | POST | /db/dbuser/delete | mysql-manage（Tab 用户） | admin | ✓ |
| dbuserGrant | POST | /db/dbuser/grant | mysql-manage（Tab 用户） | admin | ✓ |
| dbuserRevoke | POST | /db/dbuser/revoke | mysql-manage（Tab 用户） | admin | ✓ |
| backupExport | POST | /db/backup/export | mysql-manage（Tab 库·行内） | admin | ✓ |
| backupImport | POST | /db/backup/import | mysql-manage（Tab 库·行内弹窗） | admin | ✓ |
| redisInfo | GET | /db/redis/info | redis-manage（Tab 状态） | admin+readonly | — |
| redisSlowlog | GET | /db/redis/slowlog | redis-manage（Tab 慢日志） | admin+readonly | — |
| redisPassword | POST | /db/redis/password | redis-manage（页面头部） | admin | ✓ |
| redisKeyList | GET | /db/key/list | redis-manage（Tab key·admin 专属） | admin | — |
| redisKeyTtl | GET | /db/key/ttl | redis-manage（Tab key·admin 专属） | admin | — |
| mongoStatus | GET | /db/mongo/status | mongo-manage（Tab 状态） | admin+readonly | — |
| mongoDatabaseList | GET | /db/mdb/list | mongo-manage（Tab 数据库） | admin+readonly | — |
| mongoUserList | GET | /db/muser/list | mongo-manage（Tab 用户） | admin+readonly | — |
| mongoUserCreate | POST | /db/muser/create | mongo-manage（Tab 用户） | admin | ✓ |
| mongoUserDelete | POST | /db/muser/delete | mongo-manage（Tab 用户） | admin | ✓ |
| mongoUserGrant | POST | /db/muser/grant | mongo-manage（Tab 用户·行内） | admin | ✓ |
| mongoUserRevoke | POST | /db/muser/revoke | mongo-manage（Tab 用户·行内） | admin | ✓ |
| tableList | GET | /db/table/list | mysql-tables（v1.4） | admin+readonly | — |
| tableStructure | GET | /db/table/structure | mysql-table-detail（Tab 结构，v1.4） | admin+readonly | — |
| tableDrop | POST | /db/table/drop | mysql-tables（行内，v1.4） | admin | ✓ |
| rowList | GET | /db/row/list | mysql-table-detail（Tab 数据浏览，v1.4） | admin（数据内容 Q6） | — |
| rowUpdate | POST | /db/row/update | mysql-table-detail（Tab 数据浏览·行内弹窗，v1.4） | admin | ✓ |
| rowDelete | POST | /db/row/delete | mysql-table-detail（Tab 数据浏览·行内，v1.4） | admin | ✓ |
| mongoCollectionList | GET | /db/mcoll/list | mongo-collections（v1.4） | admin+readonly | — |
| mongoCollectionCreate | POST | /db/mcoll/create | mongo-collections（工具栏弹窗，v1.4） | admin | ✓ |
| mongoCollectionDrop | POST | /db/mcoll/drop | mongo-collections（行内，v1.4） | admin | ✓ |
| mongoDatabaseDrop | POST | /db/mdb/dropdb | mongo-manage（Tab 数据库·行内，v1.4） | admin | ✓ |
| mongoDocumentList | GET | /db/mdoc/list | mongo-documents（v1.4） | admin（数据内容 Q6） | — |
| mongoDocumentInsert | POST | /db/mdoc/insert | mongo-documents（工具栏弹窗，v1.4） | admin | ✓ |
| mongoDocumentUpdate | POST | /db/mdoc/update | mongo-documents（行内弹窗，v1.4） | admin | ✓ |
| mongoDocumentDelete | POST | /db/mdoc/delete | mongo-documents（行内，v1.4） | admin | ✓ |

> S7 一致性：44 Action 与 api-database-mgmt.md v2.3 44 接口一一对应（一接口一 Action，无未覆盖接口、无虚构接口；v1.2 +mongo 7、v1.4 +table/row 6 +mcoll/mdoc/mdb-dropped 8）；backupExport 成功=文件流下载（双形态响应，按 Content-Type 分流，api D-06）。**v1.4 数据内容 Action（rowList/mongoDocumentList）admin 专属（Q6/I-11，沿 redisKeyList 先例）；元数据 Action（tableList/tableStructure/mongoCollectionList）两角色（readonly 视角见 readonly 文档）**。
>
> **跨模块 Action 引用（v1.5，非本模块 S7 计数——本模块 api 零新增接口）**：`dbtoolList`（GET /dbtool/tool/list，db-tools，admin+readonly）+ `dbtoolOpen`（POST /dbtool/access/open，db-tools，admin+CSRF）——高级管理入口消费（6.8）：工具状态判定（installed/running）+ 打开分发（新标签页，signon/gating）；Action 定义权威归 db-tools PageSpec v1.0 第 2 节，本模块仅引用不重复定义（沿 F5 面包屑消费 panel-core 能力先例——呈现层跨模块消费，本模块 S7 44/44 不变）。

## 第 3 节 字典 Key 清单

| Key | 取值（=01-solution/04-business-enum.md 定稿） | 用途 |
|-----|--------------------------------------------|------|
| db_engine | mysql / mariadb / redis / mongodb（v1.2 +mongodb） | 引擎列+筛选（NSelect） |
| db_conn_mode | socket / tcp | 连接方式列+凭据弹窗下拉 |
| instance_conn_state | reachable / unreachable | 连接状态列（colorTag） |
| grant_level | readonly / write / all | 授权弹窗下拉+授权概况 tag |
| mongo_role | read / readWrite / dbAdmin（v1.2 新增） | mongo 角色弹窗下拉+角色概况 tag |

**数据源规范偏差声明**：不实现 `POST /{domain}/dict/batch`（分册 3.6）——面板单机、枚举全部固定值，字典由前端 TS 常量提供（从 04-business-enum.md 定稿值生成 enums.ts，单一事实源）；与 account-auth PageSpec 第 3 节同一偏差声明。字符集候选非字典（实例实况开放集合，动态 options=databaseCharset­s 响应，api D-11）。**v1.4 零新增字典 Key**（api v2.3 零新增枚举）：表存储引擎/排序规则为实例实况开放集合（沿字符集先例，text 呈现非 dict）；表结构 key 列（PRI/UNI/MUL）为引擎原值开放字符串（text 呈现）；redundantSocket 为布尔派生标注（非字典）。

**状态颜色方案**（分册 3.1 语义映射）：

| Key::值 | 颜色 |
|---------|------|
| instance_conn_state::reachable | green |
| instance_conn_state::unreachable | orange（异常态） |
| db_engine::mysql / mariadb | blue（MySQL 族） |
| db_engine::redis | purple |
| db_engine::mongodb | green（v1.2） |
| 布尔 credentialsSet | true=已配置(green) / false=未配置(gray) |
| grant_level::readonly | gray |
| grant_level::write | blue |
| grant_level::all | red（高危着色提示） |
| mongo_role::read | gray |
| mongo_role::readWrite | blue |
| mongo_role::dbAdmin | red（高危着色提示） |

## 第 4 节 列表页

### 4.1 实例清单（db-list）— 路由 /databases

**权限**：admin+readonly 可见（ADR-018 全可看）；治理入口 readonly 可进（只读渲染，见 readonly 文档）；变更按钮 admin 专属。

**查询条件**（对应 api 4.1.4 instance/list query，确认依据=API 参数面，不虚构）：

| 条件 | 字段 | 组件 | 查询方式 |
|------|------|------|---------|
| 引擎 | engine | NSelect（db_engine，含「全部」） | eq |

**表格列**（NDataTable，列顺序按分册 3.1）：

| 列 | 字段 | 组件 | 宽度 | 配置 |
|----|------|------|------|------|
| 序号 | — | index | 60 | — |
| 引擎 | engine | dict db_engine | 110 | colorTag（3 节配色） |
| 实例标识 | identityKey | text | 240 fixed:left | 呈现 `host:port` 或 socket 路径（api D-01 组装规则）；行点击进入治理页（mysql/redis 按引擎路由） |
| 连接方式 | connMode | dict db_conn_mode | 100 | — |
| 版本 | version | text | 120 | null 显示 —（探测初期如实，api 4.1.1） |
| 连接状态 | connState | dict instance_conn_state | 110 | colorTag **+ NTooltip 语义包裹（v1.4 F2）**：「绿色=可达（面板能连上该实例）；橙色=不可达。仅反映连通性，不代表凭据正确。」 |
| 凭据 | credentialsSet | 布尔 tag | 100 | 已配置(green)/未配置(gray)；未配置行提示引导配置；**+ NTooltip 语义包裹（v1.4 F2）**：「是否已在面板配置该实例登录凭据（加密存储、不回显）。未配置时治理与观测不可用。」 |
| 发现时间 | discoveredAt | text | 180 | 本地时区展示 |
| 操作 | — | action | 240 fixed:right | operList（admin）；readonly 仅「查看」链接 |

**TCP 优先呈现与图例（v1.4 F1/F2，api D-21/D-24 呈现面）**：
- TCP 优先登记策略下清单**天然一行 TCP**（socket-only 实例一行 socket）——**无归并展开 UI**（初版「合并展开子行」方案经用户二次拍板废止，后端探测策略承载去重）。
- 清单卡片 header-extra 或说明行补图例文案：「面板优先经 TCP 纳管；仅当实例关闭 TCP（skip-networking）时经本机 Socket 兜底发现。」+ 状态图例（绿=可达/橙=不可达，仅连通性）。
- **redundantSocket=true 行**（api v2.3 instance/list 新增字段：已配凭据的冗余 socket 行，同引擎本机 TCP 可达）：实例标识列追加 NTag(gray/orange)「冗余 Socket」+ NTooltip「该实例已可经 TCP 纳管；此 Socket 档案因已配置凭据而保留。建议改配 TCP 行凭据后手动移除本行。」（保守清理口径 D-21，不做凭据自动迁移）。
- 治理页摘要条同款 connState/credentialsSet Tag 复用同一 tooltip 助手（6.1/6.2/6.3 页面头部自动获益）。

**operList**（按钮可见性 = role===admin，readonly 整列仅剩查看入口）：

| 按钮 | Action | 交互 |
|------|--------|------|
| 配置凭据 | instanceCredentials | NModal 表单弹窗（见 5.1）；credentialsSet=false 时为主按钮态 |
| 高级管理（v1.5，admin only） | dbtoolOpen（跨模块 db-tools） | 引擎∈mysql/mariadb→phpMyAdmin、engine=mongodb→mongo-express（redis 行无此按钮——Redis 集成工具 OUT）；工具状态判定+打开/未装引导（6.8）；readonly 不渲染 |
| 移除 | instanceRemove | NPopconfirm 强确认：文案明示「仅解除面板管理，不卸载实例、不删除数据」（⚠️A20）；DBM_016 在途时按钮禁用+提示 |
| 查看（readonly 同） | —（路由跳转） | 按引擎进入 mysql-manage / redis-manage |

**工具栏**（admin only）：`重新探测`（instanceRescan）——成功 toast「新增 n 个实例，更新 m 个状态」（data.discovered/updated）；**v1.4：merged>0 时追加「清理 k 个冗余 Socket 档案」（data.merged，api D-21 保守清理——仅无凭据行；已配凭据行保留并以 redundantSocket 标注）**。

**空态**：无任何实例 → NEmpty 引导文案「未发现 MySQL/MariaDB/Redis/MongoDB 实例」（v1.2 含 MongoDB）（R-15 发现哲学空态，PRD FR-01；可手动重新探测）。

### 4.2 mysql-manage 库 Tab 表格（页面级表格，见第 6 节页面组织）

（表格定义归入第 6 节详情区块 Card——本模块治理页为详情型页面，列表页仅 4.1 一个。）

## 第 5 节 表单页（全部 Dialog 弹窗形态，字段数 <5 或行内操作语境）

### 5.1 配置凭据弹窗（instanceCredentials，db-list 行内）

**确认依据**：api 4.1.4 credentials req；I-07 互斥显隐。

| 字段 | 组件 | 校验 | 默认 |
|------|------|------|------|
| 连接方式 | NSelect（db_conn_mode） | 必填 | 档案现值 |
| socket 路径 | NInput | connMode=socket 时必填+绝对路径形态（I-08）；tcp 时隐藏 | 档案现值 |
| 主机 | NInput | connMode=tcp 时必填；socket 时隐藏 | 档案现值或 localhost |
| 端口 | NInputNumber | connMode=tcp 时必填 1~65535（I-08）；socket 时隐藏 | 档案现值或引擎默认端口 |
| 用户名 | NInput | 必填 | — |
| 密码 | NInput type=password | 必填；**不回显**（无任何读出口，⚠️A18/api I-02） | — |

**交互**：提交即验证（D-03）——DBM_005 内联「认证被拒（未保存）：服务器拒绝了该账户的登录」+ **data.reason 服务器原文**（v1.3）+ **按引擎族可操作排查指引**（v1.3，dbmAuthRejectedTips：MySQL/MariaDB 三则=账户存在性[MySQL 对不存在账户同样回 Access denied]/host 形态覆盖连接来源[TCP 连 localhost 须 localhost/127.0.0.1/%，建用户默认 host=localhost]/密码一致[@ 等特殊字符原样传递，ALTER USER 重置或用户管理删除重建]；Redis=requirepass/ACL；MongoDB=admin 库 authSource=admin）；DBM_006 内联「连接失败（未保存）：网络或 socket 不可达」+ data.reason 服务器原文（v1.3）（均不落库语义，弹窗保持打开；文案不再断言「用户名或密码错误」——1045 不区分账户不存在/host 落空/密码不符）；成功 toast「凭据已验证并保存」+ 行状态刷新（connState=reachable、credentialsSet=true）。连接目标纠正字段变更触发 DBM_020 时提示「该连接目标已存在其他实例档案」。

### 5.2 建库弹窗（databaseCreate，mysql-manage 库 Tab 工具栏）

| 字段 | 组件 | 校验 | 默认 |
|------|------|------|------|
| 库名 | NInput | 必填；库名标识符白名单形态（R-14）；失焦本地预校验，重复以 DBM_008 服务端为准 | — |
| 字符集 | NSelect（动态 options=databaseCharsets 实况） | 必填 | utf8mb4（D-11） |

### 5.3 建用户弹窗（dbuserCreate，mysql-manage 用户 Tab 工具栏）

| 字段 | 组件 | 校验 | 默认 |
|------|------|------|------|
| 用户名 | NInput | 必填；标识符形态（R-14） | — |
| host | NInput | 必填；支持 `%` 通配惯例（api 4.3.4）；失焦提示 | localhost |
| 密码 | NInput type=password | 必填；不回显 | — |

### 5.4 授权管理弹窗（dbuserGrant/dbuserRevoke，用户 Tab 行内）

**结构**（一弹窗两区，复用同一 dbuserList 响应的 grants 数据）：

- **新增授权区**：库（NSelect，options=databaseList 实况）+ 档位（NSelect grant_level，D-10 三档说明文案）→ `授权`（dbuserGrant）；DBM_009/007 目标不存在内联提示。
- **既有授权区**：该用户 grants 列表（`库名` + grant_level tag），每条 `回收`（dbuserRevoke，NPopconfirm）——DBM_011 如实提示。

### 5.5 root 密码修改弹窗（mysqlRootPassword，mysql-manage 观测 Tab 工具栏）

| 字段 | 组件 | 校验 |
|------|------|------|
| 新密码 | NInput type=password | 必填 |
| 确认新密码 | NInput type=password | 前端一致性比对 |

**交互**：成功 toast「root 密码已修改，面板凭据已同步」；**DBM_019 强提示**（api D-05）：「实例密码已修改成功，但面板凭据同步失败——请立即重新配置凭据」+ 按钮直达 5.1 弹窗（recoverHint）。

### 5.6 备份导入弹窗（backupImport，mysql-manage 库 Tab 行内，预选目标库）

| 字段 | 组件 | 校验 |
|------|------|------|
| 目标库 | NSelect（options=databaseList；行内入口预选锁定） | 必填 |
| SQL 文件 | NUpload（单文件，accept=.sql） | 必填；≤512MiB 前端预检（DBM_014 服务端为准，D-06） |

**交互**：提交前 NModal 强确认，文案明示「导入为数据变更且**非幂等**（重复导入数据叠加），失败不自动回滚」（PRD FR-16/N2）；执行中按钮在途禁用（DBM_016 互斥，D-12）；失败 DBM_013 展示 data.reason。

### 5.7 Redis 密码设置弹窗（redisPassword，redis-manage 页面头部）

字段同 5.5（新密码+确认）；DBM_019 处置同 5.5（api D-05：面板建连即旧密码验证，修改场景天然满足）。

### 5.8 行内无弹窗动作

| 动作 | Action | 交互 |
|------|--------|------|
| 备份导出 | backupExport | 库行内按钮 → 直接触发（POST 文件流下载，Content-Disposition 文件名，D-06）；在途禁用（DBM_016）；失败 JSON 体 DBM_012 展示 reason；成功 toast「导出已开始下载，动作已记入审计」 |
| 删除库 | databaseDelete | NPopconfirm 强确认（文案「不可逆：连同库内数据一并删除」） |
| 删除用户 | dbuserDelete | NPopconfirm（文案含用户名@host；提示「既有授权一并回收」） |
| 移除纳管 | instanceRemove | 见 4.1 operList |

### 5.9 MongoDB 建用户弹窗（mongoUserCreate，mongo-manage 用户 Tab 工具栏，v1.2 追加）

| 字段 | 组件 | 校验 | 默认 |
|------|------|------|------|
| 用户名 | NInput | 必填；标识符白名单形态（R-14） | — |
| 密码 | NInput type=password | 必填；不回显 | — |
| 目标库 | NSelect（options=mongoDatabaseList 实况） | 必填 | — |
| 角色 | NSelect（mongo_role 三档，D-19 说明文案） | 必填 | read |

**交互**：创建即首库首角色（api D-19）；DBM_010 重名内联提示；成功 toast+清单刷新。

### 5.10 MongoDB 角色管理弹窗（mongoUserGrant/mongoUserRevoke，用户 Tab 行内，v1.2 追加）

**结构**（一弹窗两区，复用 mongoUserList 响应的 roles 数据，与 5.4 同构）：

- **新增授权区**：库（NSelect，options=mongoDatabaseList）+ 角色（NSelect mongo_role）→ `授予`（mongoUserGrant）；目标库不存在时如实放行提示（api 4.5.2——MongoDB 允许对不存在库预授权）。
- **既有授权区**：该用户 roles 列表（`库名` + mongo_role tag），每条 `回收`（mongoUserRevoke，NPopconfirm）——DBM_011 如实提示。

### 5.11 行内无弹窗动作（v1.2 追加）

| 动作 | Action | 交互 |
|------|--------|------|
| 删除 MongoDB 用户 | mongoUserDelete | NPopconfirm（文案含用户名；提示「其全部库级角色一并回收（引擎语义）」） |

### 5.12 行编辑弹窗（rowUpdate，mysql-table-detail 数据浏览 Tab 行内，v1.4 追加）

**确认依据**：api 4.7.4 row/update req；单列主键约束 I-10（D-22）。

**前置**：仅单列主键表开放（tableStructure 响应 singlePk=true）；singlePk=false 时行内「编辑」「删除」按钮**禁用态 + NTooltip 如实提示**「该表无单列主键（无主键/复合主键），不支持行级编辑/删除」（DBM_023 同源判定，服务端为最终守卫）。

| 字段 | 组件 | 校验 | 默认 |
|------|------|------|------|
| 主键值（只读展示） | NText（pkColumn=pkValue，定位键不可编辑） | — | 行现值 |
| 各列值（逐列动态渲染，列集=tableStructure.columns 排除主键列） | NInput（文本输入，MySQL 隐式类型转换，api D-22）+ 每列「NULL」NCheckbox | 勾选 NULL → 提交 null（SET col=NULL）；未勾选按文本提交 | 行现值（null 值列默认勾选 NULL） |

**交互**：提交 values 仅含变更列（未改列不提交，减少隐式转换面）；DBM_024 内联「行已不存在或已被修改（主键旧值定位失败）」+刷新数据页；成功 toast「行已更新」+当前页刷新（rowList 重拉）。**本轮无插入行入口**（OUT-12，工具栏不渲染「插入行」）。

### 5.13 新建集合弹窗（mongoCollectionCreate，mongo-collections 工具栏，v1.4 追加）

| 字段 | 组件 | 校验 | 默认 |
|------|------|------|------|
| 集合名 | NInput | 必填；集合名形态白名单（R-14：非空/禁 $ 前缀/禁空字符，api 4.8.4）；失焦本地预校验，重复以 DBM_008 服务端为准 | — |

**交互**：弹窗顶部说明文案「MongoDB 库随首集合写入产生——新建集合即完成建库（隐式建库语义）」（api D-25）；DBM_008 重名内联提示；成功 toast+集合清单刷新（mongoCollectionList）。

### 5.14 文档插入/编辑弹窗（mongoDocumentInsert/mongoDocumentUpdate，mongo-documents，v1.4 追加）

**结构**（一弹窗两态复用，JSON 编辑形态）：

| 字段 | 组件 | 校验 | 默认 |
|------|------|------|------|
| 文档 JSON | NInput type=textarea（等宽字体，rows≥12） | **前端 JSON.parse 校验先行拦截**（非法即时内联错误，不提交）；服务端 DBM_026 兜底（api D-23 双层校验） | 插入态=`{}` 模板；编辑态=行文档 JSON 格式化（_id 置顶） |
| _id（编辑态只读展示） | NText（定位键，JSON 内 _id 不可变更——与 id 不一致服务端 AUTH_012，api 4.8.4） | — | 行现值 |

**交互**：插入态提交 mongoDocumentInsert（document._id 可选，缺省引擎自动生成）；编辑态提交 mongoDocumentUpdate（id=行文档 _id 原值透传，ObjectId `{"$oid":"hex"}` 形态如实）；DBM_027 内联「文档已不存在」+刷新；_id 冲突 DBM_008 语义 data.reason 内联；成功 toast+当前页刷新（mongoDocumentList）。

### 5.15 行内无弹窗动作（v1.4 追加）

| 动作 | Action | 交互 |
|------|--------|------|
| 删除表 | tableDrop | NPopconfirm 强确认（文案「不可逆：连同表内数据与结构一并删除（DROP TABLE）」）；DBM_022 如实提示；成功刷新表清单 |
| 删除行 | rowDelete | NPopconfirm 确认（文案含主键值；单列主键约束同 5.12 禁用态）；DBM_024 如实提示；成功刷新数据页 |
| 删除集合 | mongoCollectionDrop | NPopconfirm 强确认（文案「不可逆：连同集合内全部文档移除（dropCollection）」）；DBM_025 如实提示 |
| 删除库（MongoDB） | mongoDatabaseDrop | NPopconfirm 强确认（文案「不可逆：连同库内全部集合与文档移除（dropDatabase）」）；**系统库 admin/config/local 按钮禁用态 + NTooltip「系统库不可删除」（DBM_028/I-12 前端镜像，服务端硬校验）** |
| 删除文档 | mongoDocumentDelete | NPopconfirm 强确认（文案含 _id 呈现值）；DBM_027 如实提示 |

## 第 6 节 详情页（治理页=hybrid DetailPage，Tab 组织）

### 6.1 MySQL/MariaDB 治理页（mysql-manage）— 路由 /databases/mysql/:instanceId

**进入条件**：db-list 行点击（引擎∈mysql/mariadb，engineGuard）；进入时 instanceList 缓存档案已含（dbStore）。

**页面头部**：实例摘要条（identityKey + engine tag + connState tag + version）+ `修改 root 密码`（admin，5.5 弹窗）。connState=unreachable 时页面顶部 NAlert「实例不可达——治理操作将无法执行（实例启停请前往服务管理）」（DBM_003 语义前置提示，api 1.5）。

**Tab 1 数据库**（admin+readonly；readonly 无工具栏与操作列）：

- 工具栏（admin）：`新建数据库`（5.2 弹窗）。
- 表格列：序号/库名(text, 240)/字符集(text, 140)/操作(action, 320 fixed:right：**「浏览」两角色可见（v1.4 F3）**+ **「高级管理」admin only（v1.5，6.8）**+ admin only：导出/导入/删除)。
- **「浏览」入口（v1.4）**：库行「浏览」按钮 → router.push `/databases/mysql/:instanceId/browse/:database`（mysql-tables 表清单页，6.4）；admin+readonly 均可见（Q6 元数据入口；readonly 数据区负空间见 readonly 文档）。
- **「高级管理」入口（v1.5）**：库行「高级管理」按钮 → phpMyAdmin 打开分发（6.8，dbtoolOpen toolType=phpmyadmin instanceId=当前实例；库名不参与 signon 绑定——phpMyAdmin 进入后自选库，工具内全库可达 admin 专属）；admin only（readonly 负空间）。
- 数据源：databaseList（实时投影，响应 generatedAt 标注「快照时间」，D-04）。

**Tab 2 用户**（admin+readonly；readonly 无工具栏与操作列）：

- 工具栏（admin）：`新建用户`（5.3 弹窗）。
- 表格列：序号/用户名(text,180)/host(text,140)/授权概况(grants → NTag 列表，`库名:档位`，grant_level 三色 tag；无授权显示 —)/操作(action, 160 fixed:right, admin only：授权管理(5.4)/删除(5.8))。
- 数据源：dbuserList；无任何密码字段（I-02）。

**Tab 3 状态与进程**（admin+readonly）：

- Card 1 状态概览（NDescriptions）：mysqlStatus 响应键值对（version + variables 白名单 ≤10 项，D-08；只读展示，无写副作用）。
- Card 2 进程列表（NDataTable）：列 序号/id/user/host/db/command/timeSec/state/info(text ellipsis+NTooltip)；mysqlProcesslist 全量+generatedAt；操作列=无（只读观测）。

### 6.2 Redis 治理页（redis-manage）— 路由 /databases/redis/:instanceId

**进入条件**：db-list 行点击（engine=redis，engineGuard）。

**页面头部**：实例摘要条（同 6.1）+ `设置密码`（admin，5.7 弹窗）。unreachable NAlert 同 6.1。

**Tab 1 状态概览**（admin+readonly）：NDescriptions 按 redisInfo sections 分组（server/clients/memory/stats 白名单，D-08）。

**Tab 2 慢日志**（admin+readonly）：工具栏条数选择（NSelect 50/100/200，D-09 上限）；表格列：id/timestamp(本地时区)/durationUs→ms 呈现(text, 右对齐)/command(text ellipsis)；redisSlowlog。

**Tab 3 key 浏览**（**admin 专属 Tab**——readonly 不渲染本 Tab，D0 口径：key 值属数据内容取出）：

- 工具栏：pattern（NInput，默认 `*`，占位提示支持通配；DBM_017 形态错误内联）+ `查询`（redisKeyList cursor=0 起始）。
- 表格列：key(text, 320)/type(dict 呈现 text tag)/TTL(action：`查 TTL` 按钮 → redisKeyTtl → 行内展示 ttlSec，-1 显示「永不过期」，DBM_015 提示「key 不存在（可能已过期）」)。
- **游标分页**（D-09）：非页码分页——底部 `上一页/下一页`（nextCursor 驱动；游标栈支持回退；complete=true 时禁用下一页并标注「已遍历完」；每页 COUNT=200 由服务端常量承载，前端不传）。

### 6.3 MongoDB 治理页（mongo-manage）— 路由 /databases/mongo/:instanceId（v1.2 追加）

**进入条件**：db-list 行点击（engine=mongodb，engineGuard）。

**页面头部**：实例摘要条（identityKey + engine tag(green) + connState tag + version）。unreachable NAlert 同 6.1。无页面级动作（无改密入口——MongoDB 改密经用户治理承载，MVP 不设面板建连账户改密同步）。

**Tab 1 状态概览**（admin+readonly）：NDescriptions 呈现 mongoStatus 白名单（version/uptimeSec/connections.current/available/memResidentMb/opcounters×4，D-08 同构）。

**Tab 2 数据库**（admin+readonly；**v1.4 修订：D-18 只读口径废止，ADR-031/D-25 可管理清单**）：表格列 序号/库名(text,240)/大小(sizeBytes→人类可读呈现,140,右对齐)/集合数(collections,120,右对齐)/操作(action,280 fixed:right,**v1.4 新增**：「浏览集合」两角色可见 +「删除库」admin only；**v1.5 新增**：「高级管理」admin only)；无工具栏（无独立建库入口——隐式建库经集合清单页「新建集合」达成，5.13 说明文案）；generatedAt 快照标注。
- **「浏览集合」入口**：库行 → router.push `/databases/mongo/:instanceId/browse/:database`（mongo-collections 集合清单页，6.6）。
- **「高级管理」入口（v1.5）**：库行「高级管理」按钮 → mongo-express 打开分发（6.8，dbtoolOpen toolType=mongo_express；instanceId 部署时已绑定——**工具绑定实例与当前治理实例不一致时入口提示明示绑定实例并引导重新部署**（6.8 步骤 4，boundInstanceId 判定）；admin only（readonly 负空间）。
- **「删除库」**：mongoDatabaseDrop（5.15 强确认）；系统库 admin/config/local 禁用态+tooltip（DBM_028/I-12）；readonly 操作列整列不渲染。

**Tab 3 用户**（admin+readonly；readonly 无工具栏与操作列）：

- 工具栏（admin）：`新建用户`（5.9 弹窗）。
- 表格列：序号/用户名(text,200)/认证机制(mechanisms→tag 列表,140)/角色概况(roles → NTag 列表 `库名:角色`，mongo_role 三色 tag；无角色显示 —)/操作(action, 200 fixed:right, admin only：角色管理(5.10)/删除(5.11))。
- 数据源：mongoUserList；无任何密码字段（I-02）。

### 6.4 MySQL 表清单页（mysql-tables）— 路由 /databases/mysql/:instanceId/browse/:database（v1.4 追加）

**进入条件**：mysql-manage 库 Tab 行「浏览」（6.1）；engineGuard mysql 族同链；dbStore 档案缓存已含。

**页面头部**：实例摘要条（identityKey + engine tag + connState tag[tooltip 同 4.1]）+ 库名标注（路由 params.database）；面包屑 crumb 链见 7.2。

**表格列**（NDataTable，tableList 响应 items）：

| 列 | 字段 | 组件 | 宽度 | 配置 |
|----|------|------|------|------|
| 序号 | — | index | 60 | — |
| 表名 | name | text | 240 fixed:left | 行点击/「浏览数据」进入表详情页（6.5） |
| 存储引擎 | engine | text | 110 | 实况开放集合非字典（第 3 节声明） |
| 行数(估) | rowEstimate | text | 110 右对齐 | null 显示 —；列头 NTooltip「InnoDB 统计估算值，非精确计数」（api 4.7.4） |
| 数据大小 | dataSizeBytes | text | 120 右对齐 | 人类可读呈现（B/KB/MB）；null 显示 — |
| 排序规则 | collation | text | 160 | null 显示 — |
| 注释 | comment | text ellipsis+NTooltip | 200 | null 显示 — |
| 操作 | — | action | 240 fixed:right | admin only：「浏览数据」「结构」（→6.5 对应 Tab）「删除表」（5.15 tableDrop 强确认）；readonly 整列不渲染 |

**工具栏**：无新建入口（**本轮不做建表**，OUT-12）；刷新按钮（tableList 重拉）。

**数据源**：tableList（information_schema 实时投影，generatedAt 快照标注，I-04）；空态 NEmpty「该库暂无表」（建表归后续迭代）。

### 6.5 MySQL 表详情页（mysql-table-detail）— 路由 /databases/mysql/:instanceId/browse/:database/:table（v1.4 追加）

**进入条件**：mysql-tables 行「浏览数据」/「结构」/表名点击（query.tab 定位初始 Tab，默认数据浏览）。

**页面头部**：实例摘要条 + 库名/表名标注 + **singlePk 状态标注**（tableStructure 派生：singlePk=false 时 NTag(gray)「无单列主键——行级编辑/删除不可用」+NTooltip 原因，DBM_023 同源）。

**Tab 1 数据浏览**（**admin 专属数据内容，Q6/I-11**；readonly 见负空间呈现）：

- 工具栏（admin）：刷新；**无「插入行」入口**（OUT-12 本轮不做）。
- 表格（NDataTable，rowList 响应）：列头=columns 数组动态渲染（来自结构列序）；单元格值按 JSON 类型呈现——**null 显式 NTag(gray)「NULL」**（JSON null≠TS undefined，api 4.7.4）、长文本截断 ellipsis+NTooltip 全文、数值右对齐。
- 行操作（admin，singlePk=true 时）：「编辑」（5.12 弹窗）「删除」（5.15 rowDelete）；singlePk=false 时两按钮禁用态+tooltip（I-10）。
- **分页**（NPagination）：page/pageSize 驱动（默认 20，pageSize 可选 20/50/100，上限 100=TABLE_PAGE_LIMIT，api D-22）；total 显示「共 n 行」；翻页重拉 rowList。
- 数据源：rowList（JSON_OBJECT 逐行聚合通道，api D-22）；DBM_022/024 分流见第 1 节。

**Tab 2 结构**（admin+readonly，元数据 Q6）：

- 表格（tableStructure 响应 columns）：列名(column,180)/类型(type,180)/可空(nullable→YES/NO tag,80)/键(key,80：PRI 高亮 tag)/默认(default,140，null 显示 —)/额外(extra,140)/注释(comment,200 ellipsis+tooltip)。
- 只读呈现无操作列；generatedAt 快照标注。

### 6.6 MongoDB 集合清单页（mongo-collections）— 路由 /databases/mongo/:instanceId/browse/:database（v1.4 追加）

**进入条件**：mongo-manage 数据库 Tab 行「浏览集合」（6.3）；engineGuard mongodb 同链。

**页面头部**：实例摘要条（engine tag green + connState tag[tooltip]）+ 库名标注；面包屑见 7.2。

**工具栏**（admin）：`新建集合`（5.13 弹窗，含隐式建库说明文案）；刷新。readonly 无工具栏。

**表格列**（mongoCollectionList 响应 items）：

| 列 | 字段 | 组件 | 宽度 | 配置 |
|----|------|------|------|------|
| 序号 | — | index | 60 | — |
| 集合名 | name | text | 280 fixed:left | 行点击/「浏览文档」进入文档浏览页（6.7） |
| 文档数 | docCount | text | 120 右对齐 | 列头 NTooltip「estimatedDocumentCount 估算值」（api 4.8.4） |
| 大小 | sizeBytes | text | 120 右对齐 | 人类可读呈现 |
| 操作 | — | action | 200 fixed:right | admin only：「浏览文档」（→6.7）「删除集合」（5.15 mongoCollectionDrop 强确认）；readonly 整列不渲染 |

**数据源**：mongoCollectionList（实况投影，generatedAt 标注）；空态 NEmpty「该库暂无集合——新建集合即完成建库（MongoDB 库随首集合写入产生）」。

### 6.7 MongoDB 文档浏览页（mongo-documents）— 路由 /databases/mongo/:instanceId/browse/:database/:collection（v1.4 追加）

**进入条件**：mongo-collections 行「浏览文档」/集合名点击。

**页面头部**：实例摘要条 + 库名/集合名标注；面包屑见 7.2。

**工具栏**（admin）：`插入文档`（5.14 弹窗插入态）；刷新。readonly 无工具栏。

**文档呈现**（**admin 专属数据内容，Q6/I-11**；readonly 负空间）：

- 列表形态（NDataTable 或卡片列表）：每行一文档——_id 置顶列（JSON 值呈现，ObjectId `{"$oid":"hex"}` 缩写显示+tooltip 全文）+ 文档 JSON 摘要列（格式化单行截断 ellipsis+NTooltip 全文）+ 操作列（admin：「编辑」5.14 编辑态、「删除」5.15 mongoDocumentDelete 强确认）。
- **分页**（NPagination）：page/pageSize 驱动（默认 20，可选 20/50/100，上限 100=DOC_PAGE_LIMIT，api D-23）；total=CountDocuments「共 n 篇文档」。
- 数据源：mongoDocumentList（find skip/limit，BSON→JSON 映射如实——ObjectId/$date 扩展 JSON 形态透传，api 4.8.4）；DBM_025/026/027 分流见第 1 节。
- 空态 NEmpty「该集合暂无文档」（插入文档入口引导）。

### 6.8 高级管理入口规格（v1.5 新增，db-tools 集成消费，admin 专属）

**定位**：database-mgmt 实例治理面打开集成工具（phpMyAdmin/mongo-express）的统一入口（D0 Q3 高级入口，与面板内轻量浏览并存）；能力归 db-tools（api-db-tools.md v1.0 D-09/D-11/4.4），本模块仅呈现层消费（跨模块 Action 引用 dbtoolList/dbtoolOpen，第 2 节）。

**入口位置**（三处，共用本规格）：

| 位置 | 引擎映射 | 行级语境 |
|------|---------|---------|
| 4.1 db-list 实例行 operList「高级管理」 | mysql/mariadb→phpmyadmin；mongodb→mongo_express；**redis 行无入口**（Redis 集成工具 OUT，D0 范围边界） | 实例级 |
| 6.1 mysql-manage 库 Tab 库行「高级管理」 | phpmyadmin | 库级（进入工具后自选库，signon 绑定实例不绑定库） |
| 6.3 mongo-manage 数据库 Tab 库行「高级管理」 | mongo_express | 库级（工具绑定实例，进入后全库可达） |

**交互流程**（admin only；readonly 入口不渲染——见 readonly 文档负空间）：

1. **工具状态判定**：首次点击入口时懒加载 dbtoolList（dbtoolStore 缓存，手动刷新语义同 db-tools PageSpec 4.1）；按引擎映射取目标工具条目。
2. **已装分支**（installed=true）：
   - running=true → 直接 dbtoolOpen（phpmyadmin 携当前 instanceId；mongo_express 部署时已绑定）→ 响应 url → **window.open 新标签页**（db-tools PageSpec 4.4 裁定，noopener）；
   - running=false/null → NModal 提示「工具服务未运行」+「到工具管理查看」（/db-tools 链接）+ 可选重试打开（DBT_012 服务端兜底）。
3. **未装分支**（installed=false）：NModal 引导「{工具 displayName} 未安装——安装后可经面板安全访问进行高级数据库管理」+ 依赖就绪度呈现（depsReady=false 时列出缺失项 + 「到环境管理安装」链接 /env）+「到工具管理安装」主按钮（router.push /db-tools）。
4. **mongo-express 实例绑定校验**：工具部署时绑定目标实例（api D-07）；dbtoolList 条目 `boundInstanceId`（api v1.1 新增，mongo_express=绑定实例 id，phpmyadmin=null）与当前治理实例 id 比对——**不一致时**入口 NModal 明示「mongo-express 部署时绑定实例 {boundInstanceId 经 instanceList 缓存反查 identityKey}，打开将进入该实例」+「仍要打开」/「到工具管理重新部署」（/db-tools，卸载重装切换绑定）二选；一致时直接打开。
5. **错误分流**：DBT_006（实例未就绪/凭据未配置）→「到数据库管理配置凭据」链接（本模块 5.1 弹窗语境）；DBT_004（未装）→ 未装分支引导；AUTH_008（readonly 越界）→ 边界原因提示（前端本不渲染入口，双保险）。

**安全口径呈现**：入口 tooltip「经面板会话安全访问（仅管理员）；工具仅监听本机，凭据加密存储不回显」（⚠️A6~A10 用户面文案）；打开后浏览器经 gating 进入工具（readonly/无会话不可达，网络层闸门）。

## 第 7 节 路由定义

| 路由 | 页面 | meta |
|------|------|------|
| /databases | db-list | `{requiresAuth:true}`（readonly 可见只读视图） |
| /databases/mysql/:instanceId | mysql-manage | `{requiresAuth:true, engineGuard:'mysql'}`（mariadb 同路由，guard 校验 engine∈mysql/mariadb） |
| /databases/redis/:instanceId | redis-manage | `{requiresAuth:true, engineGuard:'redis'}` |
| /databases/mongo/:instanceId | mongo-manage | `{requiresAuth:true, engineGuard:'mongodb'}`（v1.2 追加） |
| /databases/mysql/:instanceId/browse/:database | mysql-tables | `{requiresAuth:true, engineGuard:'mysql', crumb:'browse-database'}`（v1.4 追加） |
| /databases/mysql/:instanceId/browse/:database/:table | mysql-table-detail | `{requiresAuth:true, engineGuard:'mysql', crumb:'browse-table'}`（v1.4 追加） |
| /databases/mongo/:instanceId/browse/:database | mongo-collections | `{requiresAuth:true, engineGuard:'mongodb', crumb:'browse-database'}`（v1.4 追加） |
| /databases/mongo/:instanceId/browse/:database/:collection | mongo-documents | `{requiresAuth:true, engineGuard:'mongodb', crumb:'browse-collection'}`（v1.4 追加） |

**守卫顺序**（api 3.1 中间件链前端镜像）：sessionCurrent 探测（AUTH_001→/login）→ requiresAuth → engineGuard（以 dbStore 档案校验：不匹配 → DBM_018 语义提示页 + 返回 /databases）→ DBM_002 档案不存在 → 返回 /databases + toast。**v1.4 浏览页同链**（browse 路由继承引擎守卫；档案缺失/引擎不符同语义回退）。

**侧边菜单（admin 视角）**：数据库（/databases）一级入口；治理页为二级路由（不占菜单位）；**v1.4 浏览页为三/四级路由（不占菜单位，返回导航经面包屑 7.2）**。

### 7.2 顶栏面包屑返回导航规格（v1.4 F5，MainLayout 通用能力消费）

**能力归属**：panel-core S-09-01 布局与路由骨架（D1 AA 附录B 跨模块契约）——MainLayout headerTitle 升级为 n-breadcrumb；**本模块登记消费口径与 crumb 链**（能力本体增量归 panel-core，跨模块待同步项）。

**crumb 链（数据库域）**：

| 路由层级 | crumb 呈现 | 点击行为 |
|---------|-----------|---------|
| /databases | 「数据库」 | router.push('/databases') |
| /databases/{mysql\|redis\|mongo}/:instanceId | 「数据库 / {引擎} 治理·{identity}」 | 前级可点；末级（当前页）不可点 |
| …/browse/:database | 「… / {database}」 | 治理级可点回治理页 |
| …/browse/:database/:table\|:collection | 「… / {database} / {table\|collection}」 | 库级可点回表清单/集合清单页 |

**动态段组装**：
- identity：dbStore 档案缓存（instanceList 响应 identityKey 呈现 host:port/socket 路径）；档案未加载时先 ensureLoaded（I-05 不虚构——找不到档案显示 instanceId 原值兜底）。
- database/table/collection：route.params 原值（URL decode）。
- 引擎显示名：db_engine 字典映射（MySQL/MariaDB/Redis/MongoDB）。
- 静态段：路由 meta.crumb 声明（上表）；MainLayout 按 route.name+params+dbStore 组装。

**交互规则**：末级 crumb 为当前页不可点；其余各级可点 router.push 对应层级；**其余模块（文件/站点等）保持单标题兜底不强制改**（panel-core 通用能力渐进接入）；治理页/浏览页摘要条保留「返回实例清单」文字按钮作冗余返回入口（可选，面包屑为主）。两角色同获益（readonly 同 crumb 链）。

## 第 8 节 全局一致性

| # | 检查 | 结果 |
|---|------|------|
| 1 | Action ↔ api-database-mgmt.md v2.3 接口一一对应（S7） | ✅ 44/44 接口全映射，无虚构（v1.2 +mongo 7、v1.4 +14） |
| 2 | 字典 Key ↔ 04-business-enum.md 定稿枚举一一对应（S8） | ✅ 5/5（db_engine（+mongodb 同值 S8）/db_conn_mode/instance_conn_state/grant_level/mongo_role）；v1.4 零新增（存储引擎/排序规则实况开放集合非字典） |
| 3 | 所有 POST Action（26 个，v1.2 +mongo 4、v1.4 +9）均携带 X-CSRF-Token（拦截器统一注入）；GET 无 CSRF | ✅ |
| 4 | 凭据零回显（⚠️A18/I-02）：密码输入组件均 type=password 且无回显出口；任何列表/详情无密码字段 | ✅ 5.1/5.3/5.5/5.7/5.9 |
| 5 | ADR-018 + Q6 边界：readonly 按钮隐藏=前端 role 判断 + 后端 AUTH_008 双保险；数据内容（行/文档）admin 专属负空间 | ✅（详见 readonly 文档负空间 31 项） |
| 6 | 错误文案与 api 3.4 错误码一一对应（DBM_001~028 分流见第 1 节；v1.4 +DBM_022~028 七码） | ✅ |
| 7 | 字典数据源偏差（无 dict/batch，前端常量）已声明 | ✅ 第 3 节 |
| 8 | 无 📎 Scm* 组件（无 vue3-engine 栈 profile，Naive UI 等价映射） | ✅ |
| 9 | 时间展示本地时区、ISO UTC 提交（公共约定）；TTL/时长类数值按 Redis 语义如实透传（-1=永不过期）；**v1.4 行值/文档 JSON null≠TS undefined 显式呈现（NULL tag/扩展 JSON 透传）** | ✅ |
| 10 | 引擎守卫与页面路由一一对应（mysql 族/redis/mongo 三治理路由 + v1.4 四浏览路由同链，DBM_018 前端镜像） | ✅ 第 7 节 |
| 11（v1.4） | 面包屑 crumb 链与路由层级一一对应（7.2）；动态段 dbStore+params 组装不虚构（档案缺失兜底 instanceId） | ✅ |
| 12（v1.4） | 单列主键行写禁用态与服务端 I-10 同源判定（singlePk 派生，DBM_023 兜底）；系统库删除禁用态与 I-12 同源（DBM_028 兜底） | ✅ 5.12/5.15/6.5 |
| 13（v1.5） | 高级管理入口跨模块消费：dbtoolList/dbtoolOpen 引用归 db-tools PageSpec（本模块 S7 44/44 不变，api 零新增接口）；入口 admin only（readonly 负空间）；引擎映射 mysql/mariadb→phpmyadmin、mongodb→mongo_express、redis 无入口（OUT）；绑定校验 boundInstanceId（api v1.1）；打开=新标签页（db-tools 4.4 裁定） | ✅ 4.1/6.1/6.3/6.8 |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-12 | initial | 首次生成：3 页面（db-list config + mysql-manage hybrid 三 Tab + redis-manage hybrid 三 Tab[含 admin 专属 key Tab]）、23 Action、4 字典 Key、8 个表单弹窗/行内动作 |
| v1.1 | 2026-11-12 | interface_changed | Gate 1 F-1 同步：redisKeyList/redisKeyTtl 路径随 API v1.1 由四段式 /db/redis/key/{list,ttl} 改为三段式 /db/key/{list,ttl}（Action 名/页面归属/权限不变，S7 映射 23/23 保持） | 2 |
| v1.2 | 2026-11-17 | interface_added / enum_added | v0.1.4 反馈第 3 项 MongoDB 增量（API v2.0 同步）：+7 Action（S7 30/30）——mongoStatus/mongoDatabaseList/mongoUserList（两角色）+mongoUserCreate/Delete/Grant/Revoke（admin+CSRF）；新页面 6.3 mongo-manage（三 Tab：状态概览白名单/数据库只读投影（名称/大小/集合数，无建删库 D-18）/用户治理（三档库级角色 D-19））；表单 5.9 建用户/5.10 角色管理/5.11 删除行内；字典 +mongo_role 三值 + db_engine +mongodb（S8 5/5 同值）；路由 +/databases/mongo/:instanceId（engineGuard mongodb）；配色 mongodb=green、mongo_role 三色 | 2/3/5/6/7/8 |
| v1.3 | 2026-11-17 | rule_changed | v0.1.6 后 DBM_005 诊断轮 UX 增强（API v2.1 同步，d0-clarification-2026-11-17-dbm005.md）：5.1 凭据弹窗 DBM_005/006 内联追加 data.reason 服务器原文；DBM_005 另按引擎族呈现可操作排查指引（dbmAuthRejectedTips 纯函数）；DBM_005 文案订正不再断言「用户名或密码错误」；Action/接口/字典零变更（S7 30/30 保持）；readonly 无凭据配置面（负空间）无需同步 | 5.1 |
| v1.4 | 2026-11-18 | interface_added / rule_changed | database-mgmt v1.4 用户实机反馈轮（API v2.3 同步，d0-clarification-database-mgmt-ux-2026-11-18.md Q1~Q6）：①+14 Action（S7 44/44）——tableList/tableStructure/tableDrop/rowList/rowUpdate/rowDelete + mongoCollectionList/Create/Drop + mongoDatabaseDrop + mongoDocumentList/Insert/Update/Delete；②+4 页面（6.4 mysql-tables 表清单/6.5 mysql-table-detail 数据浏览+结构两 Tab/6.6 mongo-collections 集合清单/6.7 mongo-documents 文档 JSON 分页）+4 路由（browse 层级，engineGuard 同链）；③库行「浏览」入口（6.1 两角色）+mongo 库行「浏览集合」「删除库」（6.3，D-18 只读口径废止 ADR-031，系统库 DBM_028 禁用态）；④表单 5.12 行编辑（单列主键约束禁用态 I-10/NULL 勾选/主键只读）/5.13 新建集合（隐式建库说明）/5.14 文档 JSON 编辑（双层校验）/5.15 五个强确认行内动作；⑤rule_changed——4.1 实例清单 TCP 优先呈现（天然一行 TCP，无归并展开 UI）+说明文案+connState/credentialsSet NTooltip 语义包裹+图例（F2）+redundantSocket 冗余标注+rescan toast merged 统计（F1/D-21）；⑥7.2 MainLayout 面包屑规格（F5，crumb 链+动态段 dbStore+params，能力归 panel-core S-09-01 消费）；⑦错误分流 +DBM_022~028；字典/配色零新增（沿用 5 Key）；分页默认 20 上限 100（TABLE/DOC_PAGE_LIMIT）；本轮无插入行/建表入口（OUT-12） | 1/2/3/4/5/6/7/8 |
| v1.5 | 2026-11-19 | interface_added（跨模块引用） | db-tools 高级管理入口增量（api-db-tools.md v1.0/v1.1 消费，D0 Q3 高级入口兑现，api-database-mgmt 待同步项闭环）：①4.1 db-list 实例行 operList +「高级管理」（admin only，mysql/mariadb→phpmyadmin、mongodb→mongo_express、redis 无入口 OUT）；②6.1 mysql-manage 库行 +「高级管理」（操作列 260→320）；③6.3 mongo-manage 库行 +「高级管理」（操作列 200→280，绑定实例校验提示）；④新增 6.8 高级管理入口规格（工具状态懒加载判定/已装打开〔新标签页，db-tools 4.4 裁定〕/未装引导安装〔依赖就绪度+环境管理链接〕/mongo-express boundInstanceId 绑定校验〔api v1.1 联动〕/错误分流 DBT_004/006/012）；⑤第 2 节跨模块 Action 引用声明（dbtoolList/dbtoolOpen，**本模块 S7 44/44 不变，api 零新增接口**——沿 F5 面包屑消费先例）；⑥第 8 节 +检查 13；readonly 同步（入口负空间，见 readonly v1.5）；字典/配色/路由零新增 | 2/4.1/6.1/6.3/6.8/8 |

> FX 质量清单说明：FX-001~FX-141 逐条清单文件未随引擎 0.5.2 payload 发布（find 验证无 checklist.md，与 account-auth PageSpec 同判），本节按分册 9 节结构逐节落实，并以第 8 节 10 项全局一致性自查替代可机械执行部分；全量 141 项留待 Gate 1 validation 执行。
