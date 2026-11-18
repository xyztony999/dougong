# Gate 1 验证报告 — 设计→代码门控（database-mgmt v1.4 增量）

**模块**：database-mgmt（domain=supporting）
**检查时间**：2026-11-18
**轮次**：v1.4 用户实机反馈轮增量复检（D0 Q1~Q6，2026-11-18）
**基线产物**：api-database-mgmt.md v2.3 / PageSpec admin+readonly v1.4 / prd-database-mgmt.md v1.2 / domain-model.md v1.23 / BA·AA·FL v1.2 / D0 d0-clarification-database-mgmt-ux-2026-11-18.md
**编号源**：engine-rules.md G1-01~G1-11（唯一编号源）；validation skill Gate 1 节为执行展开；S7/S8 为设计侧预检口径，不与 G1 编号混用。
**检查方式**：全部检查项本轮实跑（python -I 机械 diff + grep + 逐节读码），命令与输出摘要见各节。

## 判定

**WARN** —— CRITICAL 0 / 契约断裂 0；2 项建议项（W1/W2）待用户确认，6 项 INFO。

## 一、重点检查项（ask 指定）

### 1. S7 Action↔API 映射（G1-11 / CT-001 / CT-004）— PASS

实跑：python -I 脚本提取 api 接口详情 bullet（`^- \*\*(GET|POST) /db/…\*\*`）44 条、admin PageSpec 第 2 节 Action 表 44 行，双向集合 diff + 方法逐行比对。

- 输出：`API detail bullets: 44 unique: 44` / `admin Actions: 44 unique paths: 44` / `S7 admin-vs-api(bullets): IDENTICAL` / `method mismatches: []` / `API methods: Counter({'POST': 26, 'GET': 18})`（admin 同）。
- api 3.3 权限矩阵展开（含合并行 `create / delete` 与 `（v2.3）` 后缀）= 44 条，`matrix missing: set() extra: set()`；矩阵 vs admin PageSpec 权限/CSRF 逐行比对 `permission/csrf issues: NONE`（POST 26 全 ✓ CSRF，GET 无）。
- 矩阵 readonly ✓ 13 接口 == readonly PageSpec 可见 13 Action（集合全等）。
- 4.5.2 七接口以 bullet 承载（无接口表行），3.3 矩阵与 admin Action 表均覆盖，非缺漏。

### 2. 三段式路径（G1-08 / GL-009 / M2）— PASS

实跑：44 路径 `len(p.strip('/').split('/'))!=3` 扫描 → `non-3-segment paths: []`；全文 `/db/x/y/z` 四段式正则扫描 → `4-segment occurrences in api doc: []`。资源段 table/row/mcoll/mdoc 裁定 D-26（api:54），删库复用 mdb 资源段（mdb/dropdb），全 `/db/{resource}/{action}`。

### 3. 枚举入池四方同值（G1-07 / GL-021/022/026）— PASS（值）+ WARN W1（池行格式）

实跑：escaped-pipe 感知解析（`re.split(r'(?<!\\)\|')`）比对 04-business-enum.md ↔ api 3.4 枚举表 ↔ admin 第 3 节字典 ↔ readonly 第 3 节（声明与 admin 完全一致，同一 enums.ts 源）。

| 枚举 | 池 | api 3.4 | admin dict | readonly | 结论 |
|------|----|---------|-----------|----------|------|
| db_engine | mysql/mariadb/redis/mongodb（池:44） | 同值 | 同值 | 同源 | 四方同值 |
| db_conn_mode | socket/tcp（池:45） | 同值 | 同值 | 同源 | 四方同值 |
| instance_conn_state | reachable/unreachable（池:46） | 同值 | 同值 | 同源 | 四方同值 |
| grant_level | readonly/write/all（池:47） | 同值 | 同值 | 同源 | 四方同值 |
| mongo_role | read/readWrite/dbAdmin（池:48） | 同值 | 同值 | 同源 | 值同值；**池行格式缺陷见 W1** |

- 池唯一性：53 个标识符 `duplicates: none`；本模块 5 枚举无重复定义。
- v2.3 零新增枚举声明核验：表存储引擎/排序规则实况开放集合（沿字符集 D-11 先例）、审计动作码常量非池枚举——api:229 / admin:85 / prd 第九部分说明行 / domain-model:429 四方一致。
- api 3.4 补正 v2.0 遗漏（db_engine +mongodb / mongo_role 行）在位（api:233/237）。
- audit_result 复用 success|failure（api 3.4 行在位）。

### 4. readonly 负空间（G1-04 / ADR-018 / Q6）— PASS

实跑：readonly 第 2 节负空间名单提取 31 个 Action 名 → 映射路径与「admin 全集 − readonly 可见」计算差集比对：`neg declared == computed: True diff: set()` / `visible+neg == 44: True`（13+31=44）。

- 数据内容负空间：rowList/mongoDocumentList 在负空间名单（Q6 沿 redis key 先例）；readonly 6.5/6.7 明文「不请求 rowList/mongoDocumentList」+ NEmpty「数据内容仅管理员可见」（非禁用置灰）；元数据 tableList/tableStructure/mongoCollectionList 可见。
- 写操作列整列不渲染：readonly 6.4/6.6 + 5.15 对应 admin 强确认动作；mongoDatabaseDrop 系统库禁用态 admin 侧镜像（DBM_028/I-12）。
- FL v1.2 逐项对应：function-list-database-mgmt.md:495「表数据浏览（负空间）」/:511「文档浏览（负空间）」/:467 删除库负空间/:483 操作列整列不渲染。
- readonly 无 POST Action（13 全 GET）；越界 AUTH_008 先于业务码（readonly:22）。
- 版本跳过说明：readonly v1.2→v1.4 跳过 v1.3 有理由登记（readonly:138，DBM_005 诊断轮无凭据配置面）。

### 5. DTO 可空标注（G1-03 / G1-05）— PASS

逐节读码 api 4.7.4/4.8.4（api:579-630）：

- table/list：`rowEstimate:number|null, dataSizeBytes:number|null, collation:string|null, comment:string|null`（估算/视图可空语义注明）。
- table/structure：`default:string|null, pkColumn:string|null`；`singlePk:boolean` 派生、key 为引擎原值开放字符串非枚举（声明在位）。
- row/list：行值 `string|number|boolean|null`，「行值 JSON null=NULL 语义（≠TS undefined）」显式；columns/total/page/pageSize 非空。
- mcoll/list：docCount/sizeBytes number 非空（估算语义注明）；mdoc/list：documents any（BSON→JSON 扩展形态 $oid/$date 透传声明）。
- 既有接口 additive 字段：instance/list `redundantSocket:boolean`（api:308）、rescan `merged:n`（api:309）——非空派生/统计，DTO 其余零变更声明与 D0 Q1「接口/DTO 零变更」一致。
- 全局口径句「可空字段 JSON null≠TS undefined，前端类型须显式 | null——children:null 先例同口径」在 4.1.4/4.7.4/4.8.4 三处在位。
- 前端消费对齐：admin 6.4 null 显示 —、6.5 NULL 显式 NTag(gray)、5.12 NULL 勾选→提交 null、结构 Tab default null 显示 —（admin:361-364/380/255/387）。

### 6. D0 拍板 Q1~Q6 落地 — PASS（Q5 跨模块依赖见 W2）

| 拍板 | D0 口径 | 落点核验（本轮读码） |
|------|---------|---------------------|
| Q1 TCP 优先/socket 兜底（二次拍板） | 两阶段探测+rescan 保守清理无凭据行+已配凭据保留标注+不做凭据迁移+identity_key 不变 | api D-21（:49）+2.3 编排（:118）+instance/list·rescan 详情（:308-309）；prd FR-01（:102）/FR-39（:198）；BA 2.1.1.1（:74）/2.1.1.3（:82）；AA S-31-01 rule_changed（changelog:683）；domain-model:208/429；admin 4.1 TCP 优先呈现+redundantSocket 标注+merged toast（:130-144）；readonly 4.1 同款获益（:55） |
| Q2 状态语义 tooltip+图例 | connState/credentialsSet NTooltip 文案+清单图例+治理页摘要条复用 | admin 4.1（:125-126/132/134）文案与 D0 Q2 逐字一致；prd FR-39；BA:82；readonly:55 |
| Q3 MySQL 表级治理 | 浏览入口+表清单+结构+数据分页（20/100）+单列主键行写+删表；不做插入行/建表 | api 4.7 六接口+D-22+I-10+DBM_022/023/024；prd FR-24~29（4.9）；AA S-56 六子模块（2.1.9）；FL 2.1.1.9；admin 6.4/6.5/5.12/5.15（OUT-12 无插入行入口:257/367/379）；domain-model data_table/table_row（:251-252） |
| Q4 Mongo 库/集合/文档治理 | 推翻 D-18 只读，新 ADR；删库/建集合/删集合/文档 CRUD；系统库保护 | ADR-031 登记 knowledge/design-decisions/index.md:40；api D-25/D-23+4.8 八接口+I-12+DBM_025~028+D-18 修订行（:46）；prd FR-22 修订/FR-30~37；BA 2.5.1.4/2.5.2；AA S-55-04/S-57；admin 6.3/6.6/6.7/5.13/5.14（隐式建库说明文案:265）；domain-model mongo_collection/mongo_document（:253-254） |
| Q5 顶栏面包屑 | MainLayout 通用面包屑，crumb 链四级，动态段 dbStore+params，末级不可点 | admin 7.2 规格（:442-461）+路由 crumb meta（:433-436）；readonly 7 节同获益（:117）；prd FR-38；AA 附录B:632 消费 panel-core S-09-01+**跨模块待同步项声明**；api 1.5 panel-core 行（:79）零接口承载。**能力本体 panel-core 三文档未同步 → W2** |
| Q6 角色口径 | 元数据两角色/数据内容+写 admin 专属/readonly 负空间 | api D-24/I-11（:52/111）+3.3 矩阵（row/list·mdoc/list ✗ readonly）；prd 第三部分细化段（:92）；BA:62；AA 各新子模块标注；readonly 13+31 机械核验（上文第 4 项） |

D0 §四验收口径核对：新增 MySQL 6+Mongo 8=14、总 30→44 ✓；错误码 DBM_022~028 七码 ✓；审计动作码 D0 列 9 个+Q1 允许的新码 instance_merged=10 ✓（api 3.4 共 28=13+4+1+10）；DDL 零变更零迁移 ✓（api 5.3:666 声明+domain-model 存储映射:395）。

## 二、G1-01~G1-11 逐项

| 编号 | 检查项 | 结果 | 证据 |
|------|--------|------|------|
| G1-01 | 架构文档与模块设计一致 | PASS | BA 10 L3/37 L4 == AA 37 子模块（S-code）== FL 全覆盖（实跑：`BA L3: 10 L4: 37` / `AA L4 with S-code: 37` / `AA codes not referenced in FL: []` / `BA-AA L4 name diff: none`）；S-56/S-57 全仓 application-arch-*.md 唯一（扫描无他模块占用）；三文档头 v1.2+changelog v1.2 行各 1 |
| G1-02 | PRD 覆盖业务场景与角色 | PASS | FR-01~39 连续无缺（实跑 39 定义行 unique 39，1..39 全在）；IN 36/OUT 16（含 v1.2 +16/+5 与 D0 §三边界逐项对应）；完备性附录 G5/5·T 适用 4/4·N4/4（prd:403）；Q6 角色细化段在位 |
| G1-03 | API 设计完整（路径/请求/响应/错误码） | PASS | 14 新接口逐条 bullet 含 query/req/校验/执行/响应/审计（api:581-630）；DBM_001~028 连续唯一（实跑 `continuous 1..28: True dups: False`）；分页常量 TABLE/DOC_PAGE_LIMIT 默认 20 上限 100；幂等语义节在位（4.7.4/4.8.4 末） |
| G1-04 | 交互覆盖页面类型 | PASS | +4 页面（mysql-tables config/mysql-table-detail hybrid/mongo-collections config/mongo-documents config）+4 browse 路由（engineGuard 同链+crumb meta）；表单 5.12~5.15；无审批类（模块无审批语义，prd T3 N/A 有据） |
| G1-05 | API 字段与交互规格对齐 | PASS | 6.4 列↔table/list items、6.5 结构列↔table/structure columns、6.5 数据动态列头↔row/list columns、6.6↔mcoll/list、6.7↔mdoc/list、5.12 values/pkValue↔row/update req、5.14 document/id↔mdoc insert/update、4.1 redundantSocket/merged↔api additive 字段——逐形状读码比对无臆造字段 |
| G1-06 | 状态机定义完整 | PASS | 本域无业务状态机（沿 D0 判定）：api 4.7.2/4.8.2「无状态机」声明；prd 第六部分 v1.2 +4 行辨析（表/行/集合/文档、单列主键资格、编辑在途、冗余 socket 标注均判非状态机，实跑 4 行在位）；conn_state 探测态 I-03 不变 |
| G1-07 | 枚举统一无冲突 | PASS（值）/W1（格式） | 见重点项 3：四方同值+池 53 标识唯一；池 mongo_role 行未转义竖线致表格列错位（W1） |
| G1-08 | 接口路径规范 | PASS | 见重点项 2：44/44 三段式，0 四段式 |
| G1-09 | changelog 已填写 | PASS | api 6.1 v2.3 行（:731，interface_added/rule_changed/rule_added+目标节号）；admin 第 9 节 v1.4 行（:488）；readonly 第 9 节 v1.4 行（:138）；prd 更新记录 v1.2 行；domain-model v1.23 行（:429）；BA/AA/FL v1.2 行各 1；ADR-031 入 knowledge index:40；doc-center.json 含 4 新实体（实跑 JSON 解析 True×4） |
| G1-10 | INDEX 模块状态更新 | PASS（本轮落盘） | 本报告落盘后 INDEX database-mgmt gates.gate1 由 pending 更新为 warn（note 含本轮结论与报告路径）；requirement/api/interaction note 已含 v1.4/v2.3/v1.4 增量记录（INDEX:110-112 读码核验） |
| G1-11 | 跨阶段模型一致性 | PASS | API 1.4 总览/2.1 关系图 ↔ domain-model v1.23 四投影实体（标识/属性/零台账判定逐一对应：data_table=实例+库+表名、table_row=单列主键值、mongo_collection=实例+库+集合名、mongo_document=_id）；PageSpec Action↔API 44/44（重点项 1）；api 输入行引 prd v1.2/domain-model v1.23 与实际版本一致（api:8） |

## 三、机械自检补充（M 系列口径）

- M4 占位符扫描（9 份增量产物，正则 TODO|TBD|待填|待补|占位|FIXME|XXX|???|<待|待定稿）：实质占位 0——命中 3 处均为正常正文（domain-model:79「无占位者」竞品叙述/:420「占位行关闭」历史 changelog/admin:326「占位提示」输入框 placeholder 语义）。
- GL-001a 文档头日期：api 头 2026-11-18 == 6.1 最新行 v2.3 2026-11-18；admin/readonly 头 2026-11-18 == v1.4 行日期。
- 上轮遗留闭环：前轮 INFO N1「两 PageSpec 唯一数据源仍写 api v1.0」本轮已订正为 v2.3（admin:9/readonly:9）。
- 工作区改动面：git status 所列改动文件全部归属本轮（D0 新增+BA/AA/FL+prd/domain-model+api+双 PageSpec+INDEX+doc-center+knowledge index），无本轮外未提交改动。

## 四、建议项与 INFO

### WARN（待用户确认）

- **W1 枚举池 mongo_role 行格式缺陷**（01-solution/04-business-enum.md:48）：取值列 `read | readWrite | dbAdmin` 竖线未转义（`\|`），markdown 表格该行裂为 10 列（正常 8 列），模块/状态/说明列错位。值本身 read/readWrite/dbAdmin 齐全且与 api 3.4/admin/readonly 四方同值（语义无冲突），属 v2.0 轮（2026-11-17）入池时遗留的渲染格式缺陷，非本轮新引入。建议订正为 `read \| readWrite \| dbAdmin`（与池内其余行同格式）。
- **W2 panel-core S-09-01 面包屑能力本体未同步**（跨模块待同步项）：F5 面包屑能力归属 panel-core（AA 附录B:632 显式声明「panel-core 三文档同步不在本轮 database-mgmt D1 范围，登记为跨模块待同步项」；api 1.5:79 同口径）。本轮 database-mgmt 侧消费规格完备（admin 7.2 crumb 链+动态段组装+路由 meta），但 panel-core BA/AA/FL/api/PageSpec 尚未登记该能力增量——B/F 阶段实现 MainLayout 面包屑前需先完成 panel-core 文档同步（或经用户确认归并后续轮次）。

### INFO

- **I1** 「mdb-dropped」措辞笔误 2 处（api:155 矩阵标题行「+mcoll/mdoc/mdb-dropped 8」、admin:73 S7 注同文）：应为资源动作名 mdb-dropdb；实际接口路径/Action/矩阵行全部正确，仅计数注释措辞。
- **I2** prd 待对齐-11 状态列仍为「D3 拍板（api v2.3）」未回填「已关闭」——沿待对齐-1~-10 先例（PRD 记移交、API 记关闭）；api v2.3 changelog 与 D-26 已显式关闭（资源段/错误码/审计码/JSON_OBJECT/行写并发/系统库保护六项全落定）。
- **I3** prd IN#1 摘要行措辞滞后（「socket 常见路径 + localhost:3306/6379」未含 27017 与 TCP 优先策略）——权威口径 FR-01/FR-39 已承载 v1.2 策略，摘要行级小修。
- **I4** dict/batch 数据源偏差延续（admin 第 3 节声明，前端 enums.ts 常量单一事实源；account-auth 等五先例同判，复核接受）。
- **I5** FX 全量清单（FX-001~141）未随 payload 发布，PageSpec 按 9 节结构+第 8 节自查替代，全量归 Gate 2/栈接入（admin:490/readonly:140 注记）。
- **I6** 上轮 INFO N1（PageSpec 唯一数据源版本滞后）本轮闭环（两文档均 v2.3）。

## 五、总结

**判定：WARN**

- CRITICAL 失败：0；契约断裂：0。
- 重点项全过：S7 44/44 IDENTICAL（GET18/POST26，权限/CSRF 逐行零差）；三段式 44/44、四段式 0；枚举 5 项四方同值+池 53 唯一（W1 格式缺陷除外）；readonly 13 可见+31 负空间=44 机械全等（数据内容 NEmpty 负空间口径与 FL 逐项对应）；DTO 可空显式 |null 标注+前端消费对齐；D0 Q1~Q6 全链落地（Q5 跨模块依赖 W2 待确认）。
- G1-01~G1-11 全过（G1-10 随本报告落盘更新 INDEX）。
- WARN 2 项（W1 池行格式订正 / W2 panel-core 同步归属）待用户经主会话确认后记 pass；INFO 6 项记录在案。
- B/F 验收口径 = api v2.3 + PageSpec v1.4（44 接口/44 Action/13+31 readonly 边界）。
