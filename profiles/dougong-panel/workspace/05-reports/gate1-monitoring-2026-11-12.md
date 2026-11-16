# Gate 1 验证报告 — 设计→代码门控

**模块**：monitoring（系统监控）
**检查时间**：2026-11-12
**检查者**：门控检查员·monitoring（validation skill · Gate 1；检查项编号唯一来源：engine-rules.md G1-01~G1-11；流程预检 S1~S8 见 design SKILL.md「Gate 1 阶段汇总预检」）
**产物版本**：BA/AA/FL v1.0、PRD v1.0、domain-model v1.2（monitoring 域段落）、API v1.0、PageSpec admin/readonly v1.0
**Profile 绑定**：dougong-panel（`.claude/settings.json` 实证 `ACTIVE_PROFILE: dougong-panel`；`ACTIVE_STACKS: "go-vue3"` 但 `profiles/go-vue3` 目录**不存在**——无栈规则文件可叠加，详见 I2；本 Gate 实际可执行层 = 引擎机制 + 业务规则，栈强绑定条目按 account-auth/panel-core 先例口径 N/A，见「栈特化条目 N/A 声明」）

## 检查对象

| 阶段 | 文件（均本次实读） |
|------|------|
| D1 | 00-architecture/business-arch-monitoring.md、application-arch-monitoring.md、function-list-monitoring.md |
| D2 | 02-detailed/prd/prd-monitoring.md、prd/domain-model.md（monitoring 段落：本域判定 :34-42/业务域行 :50/对象 2 行 :70-71/注释 :75/子流程注 :85/关联注 :95/集成 4 行 :110-113/存储映射 :118） |
| D3 | 02-detailed/api/api-monitoring.md |
| D4 | 02-detailed/interaction/admin/monitoring/monitoring.md、interaction/readonly/monitoring/monitoring.md |
| 参考 | 01-solution/04-business-enum.md（27 条，其中 monitoring 定稿 3 条）、workspace/INDEX.md、engine-rules.md、validation SKILL.md、design SKILL.md 与 d3-api.md 分册、application-arch-account-auth.md/application-arch-panel-core.md（编码连续性旁证） |

## S1~S8 阶段汇总预检结论（判定主表）

| # | 预检项 | 规则对应 | 结果 | 关键证据（实读/实跑） |
|---|--------|---------|------|------|
| S1 | 三文档全局检查 | G1-01 | **PASS** | BA 层级完整：2 L2/4 L3/9 L4（business-arch:63-121），第三部分树 9 L4 与第二部分一一对应（:127-144），更新记录自述「2 阶段/4 关键业务/9 业务活动」（:177）与实数一致；AA 4 功能模块 S-13~S-16 + 9 子模块均带功能说明+用户类型复选框+调用关系（application-arch:73-197），树=正文（:203-225）；模块编码连续性**机械实证**：account-auth AA 占 C-01~C-03/S-01~S-05、panel-core AA 占 S-06~S-12（grep 集合输出），monitoring 承接 S-13~S-16 无冲突（AA:24）；机制型覆盖闭环：3 个 S-13 子模块无页面，经 FL:42 全局行为段 + AA 附录A 恰 3 行映射（:233-237）覆盖，6 个交互子模块在 FL 两角色章节各 6 菜单行一一对应（FL:56-110、:124-178）；FL 第三部分视图=第二部分（:184-226）；BA 附录A 依赖（采集→快照/趋势，:152-155）与 AA 调用关系逐条同构（:129/:141/:159/:171）；A12/A13/ADR-014/ADR-018/<60MB 六个口径在 BA/AA/FL/PRD/API/PageSpec 六文档数值一致（3s、1min×1440≈24h、TOP10、全可看、不落库重启清零） |
| S2 | PRD 完备性检查 | G1-02 | **PASS** | G 5/5（目标 prd-monitoring:11-15、IN 4/OUT 8 :17-39、2 角色+访客不涉及声明+获得方式 :41-48、领域对象指向 domain-model v1.2 :174、对象关系 :175）；T 适用项 3/3：T1 字段=metric 九类+system_info 四字段（:181 ↔ domain-model:70-71）、T1a 两条被引用链（:124-129）、T4 窗口+维度双枚举查询（:115-121）；T2/T3/T5/T6/T7/T8/T9 共 7 项 N/A 均有据（无状态机辨析 :99-111、不落库无导出 :122、R-09 零外呼 :93）；N 4/4（N2 九个异常分支 :135-147）；FR-01~FR-09 ↔ AA 9 子模块一一映射（:56-81，S-13-01→FR-01 … S-16-02→FR-09）；PRD 附录自查结论（:203-210）与本次独立复验一致 |
| S3 | 领域模型分离 | G1-06 / G1-11 | **PASS**（附 I1） | 5 类对象分类完成：聚合根=无且 A1-A4 逐条判定（domain-model:42）、实体=无（:75 注明理由：只读观测流无可变个体）、值对象=metric+system_info（:70-71）、子流程=无（:85 注明：后台机制链路非业务聚合）、关联领域对象=无（:95 注明：不持久化外部域 ID）；过程数据已归类不入清单（:75）；分离落盘：独立聚合视图 domain-model.md v1.2，PRD:174/API:8 引用一致；存储映射 monitoring→无表（:118）↔ API 1.2/5.2/5.3 零持久化同口径；G1-06 状态机：无生命周期状态机为 D0 拍板且 PRD 第六部分对 5 个易混淆项逐一辨析（窗口/维度=查询枚举、清零=数据边界、滚动覆盖=内存行为，prd:101-109），API 4.1.2/4.2.2 同口径（api:169/:233） |
| S4 | API 全局模板合规（M1~M5 + GL） | G1-03 / G1-08 | **PASS** | M1 章节结构：**d3 裁剪矩阵实证**——工具类（无聚合根→工具类，d3-api.md 判定规则 1 + PageSpec 元信息同判）第 2 章=「—」可裁剪（api:55 显式声明不适用并给出理由）、第 1/4/6 章必填全在（api:11-55/:127-251/:273-297）、第 3 章选填存在（:57-125）、第 5 章非必填存在（:253-271）；M2 实跑 `grep -oE "(GET\|POST) /monitoring/[a-z]+/[a-z_]+"` = 4/4 三段式 `/monitoring/{metrics,processes,system}/{action}`；M3 4 枚举均在池（04-business-enum:18/:34/:35/:36）；M4 实跑占位符 grep（按需填写/TODO/FIXME/TBD）API+PRD+domain-model+两 PageSpec 全部 exit=1 无残留；M5 6.1 存在含 v1.0 initial 行（api:279）；GL-001a（头部 2026-11-12=更新记录日期）/GL-001b/GL-002（2 章/4.1.2/4.1.3/配置项/SQL/DDL 均注明原因）/GL-003（D-06 混合模式）/GL-004（1.4 两对象↔4.1/4.2）/GL-005/006（无子流程/无关联，两处同口径）/GL-007/008（无事件无消费方，一致）/GL-009 通过；GL-010/011 按无栈通用契约口径（全 GET 只读 :64、`{code,message,data}` 包装 :65 与 account-auth D-07 同契约，见 I4）；GL-012 N/A-通过（零写接口，且 4 接口权限列齐备 :88-91）；GL-021/022/026/027/028 枚举闭环通过（来源为文件级 04-business-enum.md 标注 :107，池单文件无歧义）；GL-029/030 N/A（无 detail 子表开关场景）；GL-031 N/A（非主数据类，无 CRUD/options 语义）；GL-032 通过（实跑 grep「dict」API 无独立字典端点；前端偏差声明见 I3） |
| S5 | API 变更摘要 | G1-09 | **PASS** | api 6.1 v1.0 initial 行含变更类型/变更摘要/目标节号三列（:279）；6.2 变更类型词汇表、6.3 语义化版本规则齐（:281-297）；全部 8 份设计文档 changelog 齐：BA:173-177、AA:263-267、FL:251-255、PRD:212-216、domain-model:120-126（v1.0/v1.1/v1.2 三行累积）、api:275-297、PageSpec admin:171-175、readonly:65-69 |
| S6 | PageSpec 输出校验 | G1-04 / G1-05 | **PASS** | 两份 PageSpec 9 节结构齐（admin:13-177、readonly:14-71）；模块元信息含唯一数据源=api-monitoring.md v1.0（admin:9）、业务类型=工具类诚实判定（admin:18）；Action 表 4 行含方法/路径/页面/权限/CSRF 列（admin:26-31）+ 轮询纪律表（:37-42）；字典 Key 3+1 同源池（:46-51）；无列表页/无表单页诚实声明（admin:59-65，与零持久化口径一致）；详情 2 页 4 Card+控件条+echarts 渲染齐（admin:69-141）；路由 2 条+框架级边界落位 panel-core（:145-152）；readonly 全等视角声明、负空间=0 与 api 3.3 权限矩阵无 ✗ 行一致（readonly:33、:59 ↔ api:88-91）；G1-05 字段逐项对齐（见 CT-002）；FX-001~141 清单未随 payload 发布经本次 `find` 实证（payload 无 FX/前端清单文件），结构级+机械项已执行（同 panel-core I4 口径） |
| S7 | 跨阶段一致性 | G1-11 | **PASS** | 领域对象：D2（domain-model:70-71）↔ API 1.4 总览（api:42-43）↔ 4.1/4.2 字段清单（:139-152/:219-225）↔ 存储映射 无表（domain-model:118 ↔ api:267/:271）逐层一致（metric 九类字段两侧同名同义、system_info 四字段+errors D-08 两侧一致）；**Action 机械 diff 实跑**：API 4 接口路径 == admin PageSpec 4 Action == readonly PageSpec 4 Action（`diff` 逐路径一致、无虚构、无遗漏）；方法（GET）/权限（admin+readonly）/CSRF（—）三列逐行一致（api:88-91 ↔ admin:26-31 ↔ readonly:26-31）；D3 关闭 PRD 待对齐-1/-3/-4 的闭环在 PRD:156-159 与 api D-01/D-03/D-04 两侧同记 |
| S8 | 枚举池无冲突 | G1-07 | **PASS** | 实跑池标识符 `sort | uniq -c` 重复检查 = **空**（27 条全唯一，无跨域命名冲突）；API 引用 4 枚举（metric_window/metric_dimension/metric_scope/panel_role）取值与池逐字一致：04:34/35/36/18 ↔ api:111-114 ↔ PageSpec:48-51 **三方同值**；metric_scope 为 D3 新增定稿并已入池登记（04:36，注明关闭 prd 待对齐-4）；监控 3 枚举 D2/D3 定稿标注与 domain-model v1.2 更新行（:126「共享内核 2 枚举」+D3 补 scope）及 PRD:216 自述一致 |

**判定规则回执**：S1~S6 全过 → 非 FAIL；S7~S8 全过 → 非 WARN。**总体判定 = PASS**（1 项 INFO 级文档修正建议 + 4 项观察记录在案，均不阻断、无需重跑本 Gate）。

## G1-01~G1-11 逐项结论

| 编号 | 检查项 | 结果 | 依据（本次实读/实跑） |
|------|--------|------|----------------------|
| G1-01 | 架构文档与模块设计一致 | **PASS** | AA 1.1 承接 BA v1.0 并按 L3→DDD 归类（application-arch:13-20）；PRD 输入列明 D0+D1 三文档（prd:8）；API 输入列明 PRD+domain-model+D1（api:8）；FR 编号按 AA S-13~S-16 组织（prd:54-81）；PageSpec 唯一数据源=api v1.0（admin:9）；BA→AA→FL→PRD→API→PageSpec 六文档 A12/A13/ADR-014/ADR-018/预算口径数值一致（证据见 S1） |
| G1-02 | PRD 覆盖所有业务场景和角色 | **PASS** | 2 角色+职责+获得方式+访客不涉及声明（prd:43-48）；9 L4 → FR-01~09 一一映射（prd:56-81）；异常场景 9 条覆盖重启清零/采样失败空洞/热插拔/readonly 放行/参数非法（:135-147）；readonly 边界（ADR-018 全可看、无拒绝面）与 FL 附录A 对照一致（FL:230-239 ↔ prd:95） |
| G1-03 | API 设计完整（路径/请求/响应/错误码） | **PASS** | 实跑接口计数 = **4**（3.3 矩阵 :88-91、4.1.4/4.2.4 详情 :185-193/:247、5.1 清单 :259-262 三处同数）；每接口含 query/校验规则/响应 data 形状/错误码（:185-193/:247）；错误码 MON_001~004 + 复用 AUTH_001/002（:101-105）覆盖 PRD N2 全部 9 个异常分支中接口侧可表达的项（参数非法→MON_001~003+R-14、数据未就绪→MON_004、采集失败→degraded/errors，N2 缓冲并发为 B 阶段实现细节 prd:147 两文档同口径）；进程内 MetricsBuffer 契约（:199-209）替代仓储契约 + 零持久化声明（5.2/5.3 :265-271） |
| G1-04 | 交互设计覆盖所有页面类型 | **PASS** | 列表页/表单页 N/A 均有显式诚实声明（admin:59-65：无台账/无表单语义，负空间声明进程行无操作列）；详情（展示）页=监控概览 3 Card + 历史趋势 1 Card（admin:69-141）；审批 N/A 有据（本域无审批语义，prd:111）；hybrid 页面类型判定与分册规则对应（admin:19）；readonly 视图全等声明（readonly:33/:45） |
| G1-05 | API 字段与交互规格字段对齐 | **PASS** | 逐字段核对：system/info data 5 键（distro/kernel/hostname/uptimeSeconds/errors，api:247）↔ Card A 5 行（admin:85-89）；snapshot data 9 组键（ts/degraded/cpu/memory/disk/net/load/processes，api:185）↔ 页面横幅+Card B 六区块+Card C 四列（admin:77-78/:97-103/:109-114）；history query 3 参 + data 6 键（window/dimension/scope/startTime/partial/series，api:186-192）↔ 控件条 4 控件 + 趋势图 5 行为（admin:125-139）；MON_001~004+AUTH_001/002 文案 ↔ api 3.4（admin:22 ↔ api:101-105）——无臆造字段 |
| G1-06 | 状态机定义完整 | **PASS**（以「无状态机」完整论证满足） | 本域无生命周期状态机为 D0 拍板；PRD 第六部分对 5 个易混淆项逐一辨析（prd:101-109）；API 4.1.2/4.2.2 同口径（api:169/:233）；T3 审批 N/A 有依据（prd:111） |
| G1-07 | 枚举统一且无冲突 | **PASS** | 实跑：池 27 标识符无重复；API 3.4 的 4 枚举取值与池逐条一致（04:18/34/35/36 ↔ api:111-114）；PageSpec 字典 Key 三方同值（admin:48-51）；metric_scope 与既有枚举无命名/取值冲突（唯一性实证）；series key 前缀为业务常量非枚举，两文档口径一致（api:120-123、PageSpec 第 3 节未误登记为字典） |
| G1-08 | 接口路径符合 /domain/resource/action | **PASS** | 4 接口全部 `/monitoring/{resource}/{action}` 三段式（api:63 声明 + grep 明细）；资源 metrics/processes/system |
| G1-09 | 设计文档 changelog 已填写 | **PASS** | 8 份文档均有版本属性表+更新记录表（明细见 S5）；API 6.1 使用词汇表内 `initial` 类型（api:279/:281-293） |
| G1-10 | 状态索引模块状态已更新 | **PASS** | INDEX.md:50-52 monitoring requirement/api/interaction 均 done 且 note 含产物路径与计数口径（「S7 4/4 Action 映射 + S8 4/4 枚举机械校验通过」与本次独立复验一致）；gates.gate1 pending 由本次检查落盘为 pass |
| G1-11 | 跨阶段模型一致性 | **PASS** | D2 无聚合根+2 值对象 ↔ API 1.4 总览 ↔ 4.1/4.2（无 DDL 与存储映射「无表」闭环）；无子流程/无关联领域对象/无领域事件 三文档同口径（domain-model:85/:95、api:45/:199-211、prd 第六部分）；PageSpec 4 Action ↔ API 4 接口一一对应（机械 diff 实证，见 S7） |

## 执行展开（validation skill Gate 1 三大块）

### 1. PRD 就绪检查

| 层级 | 通过/总数 | 状态 |
|------|----------|------|
| 第一层 G1-G5 | 5/5 | PASS（prd 附录自查 :169-175 与实读一致） |
| 第二层 T1-T9 | 适用 3/3，N/A 7 项有据 | PASS（T1 :181、T1a :182、T4 :185；T2/T3 辨析 :99-111、T5 :122、T6/T7/T8 R-09 零外呼 :93、T9 I/O 在第四部分 :190） |
| 第三层 N1-N4 | 4/4（N1 部分锁定已声明） | INFO（未锁定项经 D-01/D-02/D-03/D-04/D-05 关闭或登记，见 I5） |

### 2. API 文档完整性（M1~M5 机械 + GL 全局）

| 章节 | 结果 | 说明 |
|------|------|------|
| M1 章节结构 | PASS | 1.1~1.5/3.1~3.4/4.1~4.2（各含 4.1.1~4.1.5、4.2.1~4.2.5）/5.1~5.3/6.1~6.3 全部存在且非空；第 2 章按 d3 裁剪矩阵对工具类=「—」裁剪，且 api:55 显式声明不适用+理由（跨对象约束以不变量 I-01~I-08 列于 4.1.1/4.2.1，替代第 2 章不变量职能） |
| M2 路径格式 | PASS | 4/4 三段式（grep 实证） |
| M3 枚举引用有定义 | PASS | 4/4 在池（04:18/34/35/36），无重复 |
| M4 占位符残留 | PASS | grep exit=1（API + PRD + domain-model + 两 PageSpec 五文件均无 `{按需填写}`/TODO/FIXME/TBD） |
| M5 变更记录 | PASS | 6.1 存在含 v1.0 initial 行（api:279） |
| GL 全局一致性 | PASS | GL-001a/b、002、003、004~009、012、021~022、026~028 通过（明细见 S4 行）；GL-010/011 按 D-07 无栈通用契约口径（全 GET 只读、code/message/data 包装，见 I4）；GL-029/030/031 N/A（只读投影无子表开关、非主数据类）；GL-032 通过（API 无 dict 端点 grep 实证；前端常量偏差见 I3） |

### 3. 前后端契约一致性（CT）

| 编号 | 检查项 | 状态 |
|------|--------|------|
| CT-001 | API 路径与 PageSpec Action 路径对齐 | PASS（4/4 机械 diff 一致，API↔admin↔readonly 三方） |
| CT-002 | 请求/响应字段与页面字段对应 | PASS（G1-05 明细：Card A/B/C、控件条、横幅逐键对应） |
| CT-003 | 枚举值前后端一致 | PASS（metric_window/metric_dimension/metric_scope/panel_role 池=API=PageSpec 三方同值） |
| CT-004 | Action 表所有 Action 在 API 有对应接口 | PASS（4/4，无虚构；processesTop 为 API 4.1.4 显式保留契约，PageSpec:33 注记同源） |
| CT-005/006 | 列表列/表单字段来源于 API VO/AO | PASS/N-A（无列表页无表单页，诚实声明 admin:59-65；TOP 进程四列均出自 snapshot processes[] 契约 api:185） |
| CT-007 | 字典 Key 与后端枚举对应 | PASS（admin:48-51 ↔ 04:34-36/18） |
| CT-008 | 分步表单 generate-id/save 接口 | N/A（无分步表单场景，PageSpec 未声明分步） |

## INFO 项（观察记录与建议，无需重跑本 Gate）

| # | 项 | 说明 |
|---|-----|------|
| I1 | domain-model.md:75 交叉引用落空 | 该行注记引用「prd-monitoring.md 过程数据分类表」——实跑 `grep 过程数据 prd-monitoring.md` 无命中，PRD 无该命名表；实质内容在 PRD 第六部分辨析表（prd:101-109：窗口/维度=查询枚举、清零=数据边界、滚动覆盖=内存行为，过程数据分类语义齐全）+ domain-model:75 自身归类句。建议下次触碰 domain-model.md 时将引用改为「prd-monitoring.md 第六部分辨析表」（与 panel-core I2 同类文档小修，不影响分类实质） |
| I2 | ACTIVE_STACKS=go-vue3 悬空绑定 | `.claude/settings.json` 实证 `ACTIVE_STACKS: "go-vue3"` 但 `profiles/` 下无 go-vue3 目录（ls 实证仅 dougong-panel）——无栈规则文件存在，本 Gate 栈级规则层为空，与设计产物头部「无激活栈」声明（api:9、admin:10）在效果上一致；两份前置报告（gate1-panel-core 18:09 落盘、gate1-account-auth）均实证其检查时 `ACTIVE_STACKS: ""`，settings.json mtime 18:38 晚于 panel-core 报告，绑定变更发生在设计产物成文之后。**建议运行所有者在进入 B/F 阶段前确认 ER-03 绑定一致性**（补齐 profiles/go-vue3 或回正 settings），不影响本 Gate 判定 |
| I3 | 字典数据源偏差（无 dict/batch，前端 enums.ts 常量） | PageSpec 第 3 节（admin:53）声明为设计决策「待 Gate 1 复核」——本次复核**接受该偏差**：单机面板、监控 3+1 枚举全固定值、无激活栈规则、04-business-enum.md 为单一事实源；API 侧 GL-032 精神已满足（grep 实证无独立字典端点）。与 account-auth/panel-core 两先例 I5 同判；后续接入栈 profile 时按 GL-032 重审 |
| I4 | GL-010（统一 POST）/GL-011（Result\<T\>）口径说明 | 本模块零写接口 → GL-010 无「业务写接口」可统一，全 GET 为只读性质的自然结论且经 3.1 显式声明（api:64）+ CSRF 不适用同节声明（:68）；GL-011 以 `{"code", "message", "data"}` 等价包装声明（:65，自述与 account-auth D-07 同一契约）。非偏差缺陷，按无栈通用契约口径通过（与 panel-core 先例同判） |
| I5 | PRD N1 未锁定项已全部闭环 | PRD:133 列出的 4 项工程缺口在 D3 全部处置：供给形态→D-01（关闭待对齐-1）、单位精度→D-02、空洞/边界字段形态→D-03（关闭待对齐-3 字段部分）、CPU 口径接口形态→D-04（关闭待对齐-4）、缓冲内存核算→D-05（估算 ≤3MB + B 阶段实测护栏，待对齐-2 保留 B 阶段复核）——第九部分待对齐表（:156-159）与 D-01/D-03/D-04 闭环声明两侧一致，无悬挂设计缺口 |
| I6 | FX-001~141 前端质量清单未随 payload 发布 | 本次实跑 find（payload 全目录 *FX*/*frontend*checklist* 模式）无命中——PageSpec 注记（admin:177、readonly:71）属实；FE 系列属 Gate 2 前端规则扫描范畴，代码完成后执行（同 account-auth/panel-core 口径）。本 Gate 已执行结构级+机械项（9 节结构/Action 三方机械 diff/字典三方同值/版本记录） |

## 栈特化条目 N/A 声明

`profiles/go-vue3` 不存在（ls 实证）→ 无栈级规则文件可叠加（validation skill FR-D2 分层组装的栈层为空集）；按 validation skill「异栈激活时以该栈 rules 替换本节清单」与 account-auth/panel-core 先例：CH3-002/003（统一 POST/Result\<T\>）、CH4-005a~g/007/008（审计七件套/主键规约）、CH4-057~070（MQ/事务边界/领域事件）、CH5-004~018（DDL/SQL 规约）、GL-029~031 等栈强绑定条目 N/A——其中多数在本模块**天然无对象**（零持久化→无 DDL/SQL/审计字段；零写接口→无事务/MQ/权限写校验），其余通用意图（契约完整性、路径/错误码/枚举一致性、`{code,message,data}` 契约）已在对应 G1 项按 api 3.1 声明的通用 REST 契约检查通过。

## 总结

**判定：PASS**（S1~S8 全过 → 依判定规则非 FAIL 非 WARN；建议与观察项记录在案后继续）

- G1-01~G1-11：11/11 通过（无 CRITICAL 失败）
- S1 三文档全局：BA/AA/FL 完整且 9 L4 覆盖闭环（6 交互菜单行×2 角色 + 3 机制型经全局行为/附录A 映射），模块编码 S-13~S-16 连续性机械实证；S2 PRD：G 5/5、T 适用 3/3、N 4/4、FR-01~09 ↔ 子模块一一对应；S3 领域模型：5 类对象分类+无聚合根 A1-A4 判定；S4 API：M1~M5 全过（工具类裁剪矩阵实证）+ GL 适用项通过；S5 变更摘要：8 文档 changelog 齐；S6 PageSpec：9 节结构+机械项过
- S7 跨阶段一致性：机械 diff 实证 4 Action ↔ 4 接口三方逐路径一致、领域对象 D2↔API↔存储映射（无表）逐层一致；S8 枚举池：27 标识符无重复、4 枚举三方同值
- INFO 6 项（I1 domain-model 交叉引用修正、I2 ACTIVE_STACKS 悬空绑定待所有者确认、I3 字典偏差复核接受、I4 GL-010/011 口径、I5 N1 缺口闭环确认、I6 FX 清单归 Gate 2），均不阻断
- B/F 阶段入场注意：以 api-monitoring.md 4 接口为验收计数口径（零持久化、无 DDL/SQL）； MetricsBuffer 进程内契约（api:199-209）与轮询纪律（PageSpec 第 2 节）为 B/F 实现验收要点；FE/FX 全量清单在 Gate 2 执行
