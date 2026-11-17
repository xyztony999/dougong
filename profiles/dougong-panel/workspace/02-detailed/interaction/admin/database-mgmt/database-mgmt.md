# PageSpec · database-mgmt · 管理员（admin）

| 属性 | 值 |
|------|-----|
| 版本 | v1.3 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-17 |
| 用户类型 | admin |
| 唯一数据源 | 02-detailed/api/api-database-mgmt.md v2.1 |
| 组件体系 | Vue 3 + Naive UI（knowledge/architecture-choices 栈版本表；无激活栈 profile，不使用 vue3-engine 栈 Scm* 组件） |
| 确认方式 | headless run：7 项交互确认依据 D3 契约（api-database-mgmt.md）+ ADR-018 + FL v1.0 推导，标注于各节 |

## 第 1 节 模块元信息

| 项 | 值 | 确认依据 |
|----|-----|---------|
| 模块编码 | database-mgmt（域前缀 /db） | api 3.1 |
| 业务类型 | master_data（实例档案：无 MQ、无状态机流转——conn_state 为探测态；库/用户为引擎实况投影） | 分册 2.1 判据；api 4.1.2 |
| 页面类型 | db-list=config（查询+表格+行内弹窗）；mysql-manage=hybrid（三 Tab 独立区块）；redis-manage=hybrid（Tab 切换，分册 2.2「列表页有 Tab 切换」等价应用于治理页）；mongo-manage=hybrid（三 Tab，v1.2 追加） | 分册 2.2 |
| UI 框架 | Naive UI（NDataTable/NTabs/NForm/NModal/NPopconfirm/NTag/NDescriptions/NUpload） | 栈版本表 |
| 状态管理 | Pinia：authStore（role/csrfToken）+ dbStore（当前实例档案缓存） | 栈版本表 |
| 错误处理 | 统一拦截器按 api 3.4 分流：DBM_002→返回列表+toast；DBM_003→提示实例不可达（启停归 service-mgmt，引导跳转）；DBM_004→引导「配置凭据」；DBM_005/006/021→凭据弹窗内联错误（不落库语义；DBM_021=通道不可用 v0.1.4 修复轮新增）；DBM_019→强提示「密码已修改但面板同步失败」+引导重新配置凭据；DBM_016→提示操作在途稍后重试；AUTH_008→边界原因提示 | api 3.4/1.3 |

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

> S7 一致性：30 Action 与 api-database-mgmt.md v2.0 30 接口一一对应（一接口一 Action，无未覆盖接口、无虚构接口；v1.2 +mongo 7）；backupExport 成功=文件流下载（双形态响应，按 Content-Type 分流，api D-06）。

## 第 3 节 字典 Key 清单

| Key | 取值（=01-solution/04-business-enum.md 定稿） | 用途 |
|-----|--------------------------------------------|------|
| db_engine | mysql / mariadb / redis / mongodb（v1.2 +mongodb） | 引擎列+筛选（NSelect） |
| db_conn_mode | socket / tcp | 连接方式列+凭据弹窗下拉 |
| instance_conn_state | reachable / unreachable | 连接状态列（colorTag） |
| grant_level | readonly / write / all | 授权弹窗下拉+授权概况 tag |
| mongo_role | read / readWrite / dbAdmin（v1.2 新增） | mongo 角色弹窗下拉+角色概况 tag |

**数据源规范偏差声明**：不实现 `POST /{domain}/dict/batch`（分册 3.6）——面板单机、枚举全部固定值，字典由前端 TS 常量提供（从 04-business-enum.md 定稿值生成 enums.ts，单一事实源）；与 account-auth PageSpec 第 3 节同一偏差声明。字符集候选非字典（实例实况开放集合，动态 options=databaseCharset­s 响应，api D-11）。

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
| 连接状态 | connState | dict instance_conn_state | 110 | colorTag |
| 凭据 | credentialsSet | 布尔 tag | 100 | 已配置(green)/未配置(gray)；未配置行提示引导配置 |
| 发现时间 | discoveredAt | text | 180 | 本地时区展示 |
| 操作 | — | action | 240 fixed:right | operList（admin）；readonly 仅「查看」链接 |

**operList**（按钮可见性 = role===admin，readonly 整列仅剩查看入口）：

| 按钮 | Action | 交互 |
|------|--------|------|
| 配置凭据 | instanceCredentials | NModal 表单弹窗（见 5.1）；credentialsSet=false 时为主按钮态 |
| 移除 | instanceRemove | NPopconfirm 强确认：文案明示「仅解除面板管理，不卸载实例、不删除数据」（⚠️A20）；DBM_016 在途时按钮禁用+提示 |
| 查看（readonly 同） | —（路由跳转） | 按引擎进入 mysql-manage / redis-manage |

**工具栏**（admin only）：`重新探测`（instanceRescan）——成功 toast「新增 n 个实例，更新 m 个状态」（data.discovered/updated）。

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

## 第 6 节 详情页（治理页=hybrid DetailPage，Tab 组织）

### 6.1 MySQL/MariaDB 治理页（mysql-manage）— 路由 /databases/mysql/:instanceId

**进入条件**：db-list 行点击（引擎∈mysql/mariadb，engineGuard）；进入时 instanceList 缓存档案已含（dbStore）。

**页面头部**：实例摘要条（identityKey + engine tag + connState tag + version）+ `修改 root 密码`（admin，5.5 弹窗）。connState=unreachable 时页面顶部 NAlert「实例不可达——治理操作将无法执行（实例启停请前往服务管理）」（DBM_003 语义前置提示，api 1.5）。

**Tab 1 数据库**（admin+readonly；readonly 无工具栏与操作列）：

- 工具栏（admin）：`新建数据库`（5.2 弹窗）。
- 表格列：序号/库名(text, 240)/字符集(text, 140)/操作(action, 200 fixed:right, admin only：导出/导入/删除)。
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

**Tab 2 数据库**（admin+readonly）：表格列 序号/库名(text,240)/大小(sizeBytes→人类可读呈现,140,右对齐)/集合数(collections,120,右对齐)；无工具栏/操作列（只读投影 D-18——隐式建库语义无建/删库入口）；generatedAt 快照标注。

**Tab 3 用户**（admin+readonly；readonly 无工具栏与操作列）：

- 工具栏（admin）：`新建用户`（5.9 弹窗）。
- 表格列：序号/用户名(text,200)/认证机制(mechanisms→tag 列表,140)/角色概况(roles → NTag 列表 `库名:角色`，mongo_role 三色 tag；无角色显示 —)/操作(action, 200 fixed:right, admin only：角色管理(5.10)/删除(5.11))。
- 数据源：mongoUserList；无任何密码字段（I-02）。

## 第 7 节 路由定义

| 路由 | 页面 | meta |
|------|------|------|
| /databases | db-list | `{requiresAuth:true}`（readonly 可见只读视图） |
| /databases/mysql/:instanceId | mysql-manage | `{requiresAuth:true, engineGuard:'mysql'}`（mariadb 同路由，guard 校验 engine∈mysql/mariadb） |
| /databases/redis/:instanceId | redis-manage | `{requiresAuth:true, engineGuard:'redis'}` |
| /databases/mongo/:instanceId | mongo-manage | `{requiresAuth:true, engineGuard:'mongodb'}`（v1.2 追加） |

**守卫顺序**（api 3.1 中间件链前端镜像）：sessionCurrent 探测（AUTH_001→/login）→ requiresAuth → engineGuard（以 dbStore 档案校验：不匹配 → DBM_018 语义提示页 + 返回 /databases）→ DBM_002 档案不存在 → 返回 /databases + toast。

**侧边菜单（admin 视角）**：数据库（/databases）一级入口；治理页为二级路由（不占菜单位）。

## 第 8 节 全局一致性

| # | 检查 | 结果 |
|---|------|------|
| 1 | Action ↔ api-database-mgmt.md v2.0 接口一一对应（S7） | ✅ 30/30 接口全映射，无虚构（v1.2 +mongo 7） |
| 2 | 字典 Key ↔ 04-business-enum.md 定稿枚举一一对应（S8） | ✅ 5/5（db_engine（+mongodb 同值 S8）/db_conn_mode/instance_conn_state/grant_level/mongo_role） |
| 3 | 所有 POST Action（17 个，v1.2 +mongo 4）均携带 X-CSRF-Token（拦截器统一注入）；GET 无 CSRF | ✅ |
| 4 | 凭据零回显（⚠️A18/I-02）：密码输入组件均 type=password 且无回显出口；任何列表/详情无密码字段 | ✅ 5.1/5.3/5.5/5.7 |
| 5 | ADR-018 边界：readonly 按钮隐藏=前端 role 判断 + 后端 AUTH_008 双保险 | ✅（详见 readonly 文档负空间） |
| 6 | 错误文案与 api 3.4 错误码一一对应（DBM_001~021 分流见第 1 节；DBM_021 通道不可用内联提示 v0.1.4 修复轮已入凭据弹窗） | ✅ |
| 7 | 字典数据源偏差（无 dict/batch，前端常量）已声明 | ✅ 第 3 节 |
| 8 | 无 📎 Scm* 组件（无 vue3-engine 栈 profile，Naive UI 等价映射） | ✅ |
| 9 | 时间展示本地时区、ISO UTC 提交（公共约定）；TTL/时长类数值按 Redis 语义如实透传（-1=永不过期） | ✅ |
| 10 | 引擎守卫与页面路由一一对应（mysql 族/redis/mongo 三路由，DBM_018 前端镜像） | ✅ 第 7 节 |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-12 | initial | 首次生成：3 页面（db-list config + mysql-manage hybrid 三 Tab + redis-manage hybrid 三 Tab[含 admin 专属 key Tab]）、23 Action、4 字典 Key、8 个表单弹窗/行内动作 |
| v1.1 | 2026-11-12 | interface_changed | Gate 1 F-1 同步：redisKeyList/redisKeyTtl 路径随 API v1.1 由四段式 /db/redis/key/{list,ttl} 改为三段式 /db/key/{list,ttl}（Action 名/页面归属/权限不变，S7 映射 23/23 保持） | 2 |
| v1.2 | 2026-11-17 | interface_added / enum_added | v0.1.4 反馈第 3 项 MongoDB 增量（API v2.0 同步）：+7 Action（S7 30/30）——mongoStatus/mongoDatabaseList/mongoUserList（两角色）+mongoUserCreate/Delete/Grant/Revoke（admin+CSRF）；新页面 6.3 mongo-manage（三 Tab：状态概览白名单/数据库只读投影（名称/大小/集合数，无建删库 D-18）/用户治理（三档库级角色 D-19））；表单 5.9 建用户/5.10 角色管理/5.11 删除行内；字典 +mongo_role 三值 + db_engine +mongodb（S8 5/5 同值）；路由 +/databases/mongo/:instanceId（engineGuard mongodb）；配色 mongodb=green、mongo_role 三色 | 2/3/5/6/7/8 |
| v1.3 | 2026-11-17 | rule_changed | v0.1.6 后 DBM_005 诊断轮 UX 增强（API v2.1 同步，d0-clarification-2026-11-17-dbm005.md）：5.1 凭据弹窗 DBM_005/006 内联追加 data.reason 服务器原文；DBM_005 另按引擎族呈现可操作排查指引（dbmAuthRejectedTips 纯函数）；DBM_005 文案订正不再断言「用户名或密码错误」；Action/接口/字典零变更（S7 30/30 保持）；readonly 无凭据配置面（负空间）无需同步 | 5.1 |

> FX 质量清单说明：FX-001~FX-141 逐条清单文件未随引擎 0.5.2 payload 发布（find 验证无 checklist.md，与 account-auth PageSpec 同判），本节按分册 9 节结构逐节落实，并以第 8 节 10 项全局一致性自查替代可机械执行部分；全量 141 项留待 Gate 1 validation 执行。
