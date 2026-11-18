# D0 增量澄清：database-mgmt 体验与表级治理（2026-11-18 用户反馈轮）

> 触发：用户 /pipeline 实机反馈（5 项，附 5 张截图；截图内容经代码取证还原）。
> 模式：增量（database-mgmt 全阶段 done，本轮为 UX + 功能增量）。
> 本轮代号：database-mgmt v1.4（设计）/ api v2.3 / PageSpec v1.4。

## 一、逐项取证与根因

### F1 「为什么有好几个连接， confusing」
- 现象：实例清单同一台 MySQL 出现多行（如 `/var/run/mysqld/mysqld.sock` 与 `localhost:3306`）。
- 根因：`DISCOVERY_PROBE_SET`（backend/internal/service/dbmgmt/consts.go:59）按「连接端点」逐个探测登记，
  identity_key = engine|connMode|address（prober.go:69 BuildIdentityKey）。同一台本机数据库的
  socket 端点与 TCP 端点各登记一行——这是设计语义（D-07），不是 bug，但呈现层未解释也未归并，用户困惑。
- 取证：index.vue 实例清单逐行平铺，无分组、无说明文案。

### F2 「localhost:3306 旁边的绿条是什么」
- 现象：实例标识右侧一条绿色小条。
- 根因：那是「连接状态」列的 NTag（render.ts:50 connStateTag，reachable=success 绿色）。
  无任何 tooltip/图例解释，用户误以为是进度条。
- 取证：render.ts:49-53；index.vue:144 列定义。

### F3 「为什么不能管理每个库里的表（sheets）」
- 现象：MySQL 治理页「数据库」Tab 只有库名/字符集 + 导出/导入/删除，点进库看不到表。
- 根因：功能缺口。mysql/index.vue:187 dbColumns 仅 name/charset；后端无 table 级接口
  （api-database-mgmt.md 30 接口无 table/rows/structure）。
- 取证：mysql/index.vue:131-239；adapter/dbmysql/gateway.go 方法清单（无 ListTables）。

### F4 「MongoDB 为什么只能只读，不能管理库」
- 现象：mongo 治理页「数据库」Tab 明示「只读投影（MongoDB 隐式建库，无建/删库操作）」，admin 亦无操作。
- 根因：D-18 刻意只读（mongo/index.vue:102,305-324）。但 MongoDB 实际支持显式建集合（隐式建库）、
  dropCollection、dropDatabase——D-18 把「隐式建库语义」过度解读为「不可管理」。用户要管理。
- 取证：mongo/index.vue:121-132 dbColumns 无操作列；adapter/dbmongo/gateway.go 无 collection/document 方法。

### F5 「点『浏览』后顶栏要有返回导航」
- 现象：从实例清单点「查看」进入治理页（/databases/mysql/:id 等），顶栏只显示「MySQL 治理」，
  无面包屑，返回只能靠侧栏。新增表/数据浏览页后层级更深，返回导航缺失更突出。
- 根因：MainLayout.vue:130 headerTitle 仅取 route.meta.title 单值，无面包屑；治理页路由无层级 crumb。
- 取证：MainLayout.vue:128-136,218-227；router/index.ts:160-177（治理页 meta 仅 title）。

## 二、拍板决策（主会话 D0，沿用既定工作流模式；不再逐项追问用户）

### Q1（F1）TCP 优先探测、socket 兜底 —— 用户 2026-11-18 二次拍板（取代初版「展示层归并」）
- 用户反馈：宝塔等面板均走 TCP，socket 管理极少见；socket 与 TCP 平行铺行既不符合习惯也冗余。
  取舍：完全砍 socket 会漏掉 skip-networking 的 socket-only 实例（违背「接管已有环境」发现哲学），
  故采「TCP 优先、socket 仅兜底」而非彻底删除。
- 探测注册策略（backend prober.go ProbeAll/probeOnce）：对同一 engine 的本机目标 TCP 优先——
  当本机 TCP（localhost/127.0.0.1）目标可达时，socket 目标即使可达也不登记为独立实例（视为同一服务的冗余通道）；
  仅当该 engine 无任何本机 TCP 可达时，socket 才兜底登记（覆盖 skip-networking）。
  实现：ProbeAll 两阶段——先探 TCP 目标记录各 engine 本机 TCP 可达性，再探 socket 目标时按该可达性跳过登记。
- 存量冗余 socket 行清理（rescan）：若某 socket 档案存在且同 engine 本机 TCP 现已可达 →
  保守清理「无凭据（credentialsSet=false）」的冗余 socket 行（解除纳管，审计 instance_removed 或新动作码 instance_merged，R-03 同事务）；
  「已配凭据」的 socket 行保留并在清单标注（避免凭据丢失），引导用户改配 TCP 行后手动移除。本轮不做凭据自动迁移。
- 呈现：常见情况清单天然一行 TCP（socket-only 时一行 socket）；取消初版「合并展开子行」UI（前端简化）。
  清单说明文案改为：「面板优先经 TCP 纳管；仅当实例关闭 TCP（skip-networking）时经本机 Socket 兜底发现。」
- 远程 TCP 实例不受影响；identity_key 语义不变（仍 engine|connMode|address），仅注册策略变。
- 接口/DTO 零变更（instance/list、rescan 形态不变；rescan 统计可含归并清理数）。F1 由「纯前端」改为「后端探测策略 + 前端简化」。

### Q2（F2）状态语义可见 —— 加 tooltip + 图例
- connStateTag/credentialsTag 外包 NTooltip：
  - 连接状态：「绿色=可达（面板能连上该实例）；橙色=不可达。仅反映连通性，不代表凭据正确。」
  - 凭据：「是否已在面板配置该实例登录凭据（加密存储、不回显）。未配置时治理与观测不可用。」
- 实例清单卡片 header-extra 或说明行补一句图例。治理页摘要条同款 Tag 复用同一助手，自动获益。

### Q3（F3）MySQL/MariaDB 表级治理 —— 新增「浏览」入口 + 表清单 + 结构 + 数据浏览 + 行级写（受限）
- 数据库 Tab 行操作增「浏览」按钮（admin+readonly 均可见入口；数据内容按 Q6 角色口径）。
- 新路由 /databases/mysql/:instanceId/browse/:database → 表清单页：
  表名/引擎/行数(估)/数据大小/排序规则/注释；admin 操作列「浏览数据」「结构」「删除表」（强确认）。
- 新路由 /databases/mysql/:instanceId/browse/:database/:table → 表详情页两 Tab：
  - 数据浏览：分页（page/pageSize，默认 20，上限 100），列头来自结构；单元格 NULL 显式标注；长文本截断+tooltip。
  - 结构：列名/类型/可空/键/默认/额外/注释（information_schema.COLUMNS）。
- 行级写（admin）：仅对「单列主键」表开放编辑/删除（WHERE 主键=旧值，参数化/quoteString A-DB3）；
  无单列主键的表禁用行写并如实提示原因。编辑弹窗按列呈现文本输入（MySQL 隐式类型转换；空值→NULL 勾选）。
  本轮不做「插入行」（全列默认值/自增语义复杂，归后续迭代）。
- 通道：沿用 dbmysql CLI query()；数据浏览用 JSON_OBJECT 逐行聚合避免 TSV 对含换行/制表符值的解析脆弱
  （先取列名再构造 SELECT JSON_OBJECT(...)，batch 转义还原后 JSON.parse）；结构/表清单走 information_schema。
- 删除表 = DROP TABLE（不可逆强确认，审计 table_dropped）。本轮不做建表（列定义 UI 复杂，归后续）。

### Q4（F4）MongoDB 库/集合/文档治理 —— 推翻 D-18 只读，新增 ADR
- 数据库 Tab 增操作列（admin）：「浏览集合」「删除库」（dropDatabase，不可逆强确认）。
- 新路由 /databases/mongo/:instanceId/browse/:database → 集合清单页：
  集合名/文档数/大小；工具栏「新建集合」（显式 createCollection，UI 说明「MongoDB 库随首集合写入产生」）；
  行操作「浏览文档」「删除集合」（强确认）。
- 新路由 /databases/mongo/:instanceId/browse/:database/:collection → 文档浏览页：
  分页 find（skip/limit，默认 20 上限 100），文档以 JSON 呈现（_id 置顶）；
  admin 行操作「编辑」（JSON 编辑弹窗，ReplaceOne by _id）「删除」（DeleteOne by _id 强确认）；
  工具栏「插入文档」（JSON 编辑弹窗，InsertOne）。JSON 校验失败前端拦截。
- 通道：dbmongo mongo-driver 进程内（既有 dial/withClient）。
- D-18 修订：只读投影 → 「库清单可管理（建集合/删集合/删库）+ 文档 CRUD」；新 ADR 记录语义修订与理由。
- readonly：库/集合清单与结构可见，文档内容与全部写操作 admin 专属（Q6）。

### Q5（F5）顶栏面包屑返回导航 —— MainLayout 通用面包屑
- 顶栏 headerTitle 升级为 n-breadcrumb：按路由层级渲染可点击 crumb，末级为当前页（不可点）。
- 数据库域 crumb 链：
  - /databases → 「数据库」
  - /databases/{mysql|redis|mongo}/:id → 「数据库 / MySQL 治理·{identity}」（identity 取 dbStore 档案）
  - …/browse/:database → 「… / {database}」
  - …/browse/:database/:table|:collection → 「… / {database} / {table|collection}」
- 实现：路由 meta 增 crumb 声明（静态段）+ 动态段由 MainLayout 按 route.name + params + dbStore 组装；
  点击 crumb router.push 对应层级。其余模块（文件/站点等）保持单标题兜底，不强制改。
- 治理页/浏览页摘要条保留「返回实例清单」文字按钮作为冗余返回入口（可选，面包屑为主）。

### Q6 角色口径（ADR-018 + redis key 先例）
- 元数据（表清单/结构/集合清单/库清单）：admin+readonly 全可看。
- 数据内容（MySQL 行数据 / Mongo 文档 / Redis key 值）：admin 专属（沿用 redis key 浏览 D0 先例
  「key 值属数据内容取出」）；readonly 进入浏览页见元数据，数据区呈现「数据内容仅管理员可见」负空间。
- 全部写操作（删表/行写/建集合/删集合/删库/文档 CRUD）：admin+CSRF；readonly 操作列整列不渲染。

## 三、范围边界（本轮不做，如实登记后续候选）
- MySQL 插入行 / 建表 / 多列主键行写 / SQL 控制台。
- Mongo 聚合管道 / 索引管理 / 分片管理。
- 凭据自动迁移（socket→TCP）：本轮保守保留已配凭据的 socket 行，不做自动迁移。
- 远程实例新增探测目标（探测集仍编译期常量）。
- 注：F1 本轮改探测注册策略（TCP 优先/socket 兜底，见 Q1），identity_key 语义本身不变。

## 四、验收口径（供 Gate 1/Gate 2）
- 接口：新增 MySQL 6（table/list·rows·structure·drop + row/update·delete）+ Mongo 8
  （mdb/collections·documents·create·drop·dropdb + doc insert/update/delete，资源段命名以 D3 三段式裁定为准），
  全三段式；GET 两角色（元数据）/数据内容 GET admin 专属；POST admin+CSRF。总接口 30→44（以 D3 定稿为准）。
- 错误码：复用 DBM 族 + 新增（表不存在/无单列主键/JSON 非法/集合不存在等，D3 定稿编号）。
- 审计：新增动作码（table_dropped/row_updated/row_deleted/mongo_collection_created/mongo_collection_dropped/
  mongo_database_dropped/mongo_document_inserted/mongo_document_updated/mongo_document_deleted），R-03 同事务脱敏。
- 后端探测：ProbeAll TCP 优先两阶段 + socket 兜底跳过登记 + rescan 保守清理无凭据冗余 socket 行（Q1）；prober 单测覆盖。
- 前端：4 新页面 + 面包屑 + 清单 TCP 优先呈现（取消归并展开 UI）+ tooltip；readonly 负空间机械校验；vitest 覆盖新增纯函数/store。
- 安全：R-08 参数数组（CLI 通道）/标识符白名单 quoteString（A-DB3）/引擎侧 SQL 判定 A-DB10/凭据零回显 I-02。
- 编译门控：go build/vet/test -race + vue-tsc/vite build/vitest + linux 交叉 + gofmt + dist 同步。