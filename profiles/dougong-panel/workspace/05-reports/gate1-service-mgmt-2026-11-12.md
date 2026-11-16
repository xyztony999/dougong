# Gate 1 验证报告 — 设计→代码门控

**模块**：service-mgmt（systemd 服务管理）
**检查时间**：2026-11-12
**检查者**：门控检查员·service-mgmt（validation skill · Gate 1；检查项编号唯一来源：engine-rules.md G1-01~G1-11；流程预检 S1~S8 见 design SKILL.md「Gate 1 阶段汇总预检」）
**产物版本**：BA/AA/FL v1.0、PRD v1.0、domain-model v1.3（service-mgmt 域段落）、API v1.0、PageSpec admin/readonly v1.0
**Profile 绑定**：dougong-panel（`.claude/settings.json` 实证 `ACTIVE_PROFILE: dougong-panel`；`ACTIVE_STACKS: "go-vue3"` 但 `profiles/` 目录 ls 实证仅含 `dougong-panel/`——无 go-vue3 栈规则文件可叠加，详见 I2；本 Gate 实际可执行层 = 引擎机制 + 业务规则，栈强绑定条目按 account-auth/panel-core/monitoring 先例口径 N/A，见「栈特化条目 N/A 声明」）

## 检查对象

| 阶段 | 文件（均本次实读） |
|------|------|
| D1 | 00-architecture/business-arch-service-mgmt.md、application-arch-service-mgmt.md、function-list-service-mgmt.md |
| D2 | 02-detailed/prd/prd-service-mgmt.md、prd/domain-model.md（service-mgmt 段落：本域判定 :44-52/业务域行 :61/对象行 :83/注释 :88/子流程注 :99/关联注 :110/集成 4 行 :129-132/存储映射 :138） |
| D3 | 02-detailed/api/api-service-mgmt.md |
| D4 | 02-detailed/interaction/admin/service-mgmt/service-mgmt.md、interaction/readonly/service-mgmt/service-mgmt.md |
| 参考 | 01-solution/04-business-enum.md（30 条，其中 service-mgmt 定稿 3 条 :37-39）、workspace/INDEX.md、engine-rules.md、validation SKILL.md、design SKILL.md 与 d3-api.md/d4-interaction.md 分册、application-arch-{account-auth,panel-core,monitoring}.md（编码连续性旁证）、05-reports/gate1-monitoring-2026-11-12.md（同构模块先例口径） |

## S1~S8 阶段汇总预检结论（判定主表）

| # | 预检项 | 规则对应 | 结果 | 关键证据（实读/实跑） |
|---|--------|---------|------|------|
| S1 | 三文档全局检查 | G1-01 | **PASS** | BA 层级完整：3 L2/4 L3/9 L4（business-arch:66-128），第三部分树 9 L4 与第二部分一一对应（:134-152），附录A L3 依赖 7 行与 AA 调用关系逐条同构（:160-168 ↔ application-arch:83-99/:123-127/:173-201）；AA 4 功能模块 C-04/C-05/S-17/S-18 + 9 子模块均带功能说明+用户类型复选框+调用关系（application-arch:69-201），树=正文（:207-228）；模块编码连续性**机械实证**：grep 各 AA 文档括号编码集合——account-auth C-01~C-03/S-01~S-05、panel-core S-06~S-12、monitoring S-13~S-16、service-mgmt C-04~C-05/S-17~S-18，无冲突且「域内最大序号后递增」成立（AA:22 声明与实跑一致）；机制型覆盖闭环：3 个机制型子模块（S-17-01/S-17-02/C-04-03）经 AA 附录A 恰 3 行映射（:232-240）+ FL:40 全局行为段覆盖，6 个交互子模块在 FL 两角色章节菜单行一一对应（admin 7 行 :54-113、readonly 3 行 :127-149，合计 10 行与 FL:219 自述一致）；FL 第三部分视图=第二部分（:155-191）；附录A 用户对照 6 行与 ADR-018 逐条对应（:197-204）；BA 落定的两个 D0 口径（system 级管理器 :68、全类型列表+仅 .service 可操作 :104、等待终态反馈 :120）在 AA:103/:119、FL:44、PRD:97-98 四文档同口径 |
| S2 | PRD 完备性检查 | G1-02 | **PASS** | G 5/5（目标 prd-service-mgmt:11-15、IN 8 项 :19-30/OUT 9 项 :32-44、2 角色+获得方式+访客不涉及声明 :48-53、领域对象指向 domain-model v1.3 :222、对象关系 :223）；T 适用项 4/4：T1 字段=systemd_service 六类字段（:229 ↔ domain-model:83）、T1a 被引用关系两条（:150-155：终模块无对外契约+消费方视角）、T2 双维度状态机 4+2 流转行+5 项辨析防伪状态机（:107-136）、T4 状态过滤+名称搜索+名称精确寻址两类查询（:141-146）；T3/T5/T6/T7/T8/T9 共 6 项 N/A 均有据（无审批 :137、零持久化无导出 :148、dbus 非外部对接 R-09 :190、幂等入 N3 :104、I/O 在第四部分）；N 4/4（N2 11 个异常分支 :161-175，含通道不可用/操作失败/并发/static 边界/越界拒绝/面板自身被停）；FR-01~FR-09 ↔ AA 9 子模块一一映射（:59-86，S-17-01→FR-01 … C-05-01→FR-09）；PRD 附录自查结论（:215-258）与本次独立复验一致 |
| S3 | 领域模型分离 | G1-06 / G1-11 | **PASS** | 5 类对象分类完成：聚合根=无且 A1-A4 逐条判定（domain-model:52：无一致性边界/无跨域全局引用/无生命周期入口/无域内子对象——投影流不需要聚合根守门）、域内对象-实体=systemd_service（:83，含「有稳定标识且状态可变故为实体而非值对象」论证，与 monitoring 只读观测流判值对象相区分 :88）、值对象/子流程/关联领域对象=无且均注明理由（:99/:110）；过程数据 5 类已归类不入清单（prd:192-200 P1/P2/P3/P4/P7）；分离落盘：聚合视图 domain-model.md v1.3 service-mgmt 域行齐备（:61/:83/:99/:110/:129-132/:138），PRD:8/:222 与 API:8 引用一致；存储映射 service-mgmt→无表（:138）↔ API 1.2/5.2/5.3 零持久化同口径（api:23/:257/:261）；G1-06 状态机：双维度状态机（运行三态+自启两态，事实源在 systemd 侧）状态/流转/触发三要素齐（prd:110-136 六个流转行 + 4 个辨析项，api 4.1.2 :181-188 同口径），自启维度与运行维度独立性两文档一致（prd:124-125 ↔ api:186） |
| S4 | API 全局模板合规（M1~M5 + GL） | G1-03 / G1-08 | **PASS** | M1 章节结构：**d3 裁剪矩阵实证**——无聚合根→工具类（d3-api.md 判定规则 1），第 2 章=「—」可裁剪且 api:56 显式声明不适用+理由（对象级约束以不变量 I-01~I-07 列于 4.1.1 替代其职能）；第 1 章 1.1~1.5 全在（api:13-56）、第 3 章 3.1~3.4 存在（:60-139，工具类选填不缺）、第 4 章按领域对象组织且保留 4.1.1~4.1.5 完整子结构（:143-238，标题偏离模板在 :143 声明理由）、第 5 章 5.1~5.3 存在（:242-261）、第 6 章 6.1~6.3 存在（:265-287）；M2 实跑 `grep -oE "(GET\|POST) /services/unit/[a-z]+"` = 7/7 三段式 `/services/unit/{action}`，无其他路径形态（花括号组记法与 markdown 粗体尾注为编辑痕迹非路径，逐条目检确认）；M3 API 引用 6 枚举（3 服务枚举 :123-125 + panel_role :126 + audit_action/audit_result :127）均在池（04-business-enum:37-39/:18/:22/:28）；M4 实跑占位符 grep（按需填写/TODO/FIXME/TBD/待补充）API+PRD+domain-model+两 PageSpec+BA/AA/FL 八文件全部 exit=1 无残留；M5 6.1 存在含 v1.0 initial 行（api:269）；GL-001a（头部 2026-11-12=更新记录日期，8 文档同）/GL-001b/GL-002（第 2 章/4.1.3 子流程/5.2 SQL/5.3 DDL/配置项均注明不适用原因）/GL-003（D-08 混合模式声明，:36）/GL-004（1.4 一对象 ↔ 4.1 一节）/GL-005/006（无子流程/无关联领域对象，1.4 注 :45 与 4.1.3/4.1.5 同口径；出站 AuditService.Record 为 OHS 消费非关联领域对象，与 domain-model:110 声明一致）/GL-007（3.3 权限矩阵读=admin+readonly 写=admin ↔ 4.1.2 面板触发列与 4.1.4 接口权限列一致）/GL-008（无领域事件无 MQ，1.4 注 :45 与 4.1.5 :222 声明一致；依赖方 3.2 ↔ 出站清单 4.1.5 仅 AuditService.Record 一条）/GL-009 通过；GL-010/011 按项目声明契约口径（3.1 :65-66 读=GET/写=POST 面板内部契约 + `{"code",message,data}` 包装与 account-auth D-07 同契约，见 I4）；GL-012 通过（5 个写接口每个均含校验顺序含权限边界 D-09 :210-214 + 3.3 矩阵 + 3.1 中间件链 RoleRequired 写=admin）；GL-021/022/026/027/028 枚举闭环通过（来源为文件级 04-business-enum.md 标注 :119，池单文件无歧义）；GL-029/030 N/A（无子表无懒加载场景，detail 返回全量 9 字段 :209）；GL-031 N/A（非主数据类，无 CRUD/options 语义）；GL-032 API 侧通过（实跑路径 grep 无 dict 端点；前端 enums.ts 偏差声明见 I3） |
| S5 | API 变更摘要 | G1-09 | **PASS** | api 6.1 v1.0 initial 行含变更类型/变更摘要/目标节号三列（:269）；6.2 变更类型词汇表、6.3 语义化版本规则齐（:271-287）；全部 8 份设计文档 changelog 齐：BA:189-193、AA:263-267、FL:215-219、PRD:260-264、domain-model:140-147（v1.0~v1.3 四行累积，service-mgmt 追加于 v1.3）、api:263-287、PageSpec admin 第 9 节 :172-176、readonly 第 9 节 :85-89（两 PageSpec 以「文档版本与变更记录」节承载，grep 实证节标题行存在）；API 6.1 摘要与正文相符（7 接口/8 错误码/6 枚举引用/9 设计决策，逐项点数一致） |
| S6 | PageSpec 输出校验 | G1-04 / G1-05 | **PASS** | 两份 PageSpec 9 节结构齐（admin:13-176、readonly:14-89）；模块元信息含唯一数据源=api-service-mgmt.md v1.0（admin:9）、业务类型=工具类诚实判定并注明「分册 2.1 三型之外」依据（admin:18 ↔ d4 分册 2.1 三型 + d3 判定规则）、页面类型=config 判定附不满足 hybrid/code 触发条件的理由（admin:19）；Action 表 7 行含方法/路径/页面/权限/CSRF 列（admin:26-34）+ 刷新纪律表（:40-46）；字典 Key 3+1 同源池（:50-55）+ 颜色方案表（:61-68）+ dict/batch 偏差声明（:57，见 I3）；列表页控件条 3 控件（:76-80）/表格列 7 列含宽度与 fixed（:86-94）/操作列 4 按钮+可见性权威布尔声明（:102-107）+ 在途互斥与终态反馈 7 环节表（:113-121）；无表单页诚实声明（admin:127，与零持久化「发现即接管」口径一致）；详情=属性 Drawer 7 字段 + 刷新 + 错误处理（:129-143）；路由 1 条+框架级边界落位 panel-core（:147-153）；第 8 节 12 项全局一致性自查逐项有证据指向；readonly 子集视角声明（公共页定义=admin 文档单一事实源，readonly:10）+ 5 个写 Action 负空间逐项对应 api 3.3 权限矩阵 ✗ 项（readonly:31-41）；FX-001~141 清单文件未随 payload 发布经本次 `find` 实证（payload 无 *FX*/*frontend*checklist* 命中），结构级+机械项已执行（见 I6） |
| S7 | 跨阶段一致性 | G1-11 | **WARN**（1 项字段清单偏差，见 W1） | **通过部分（机械实证）**：领域对象主体一致——D2 systemd_service（domain-model:83：实体/投影集合条目/3 枚举字段/内存态不落库）↔ API 1.4 总览（api:43 同分类同承载）↔ 4.1.1 字段清单（:153-163）↔ 存储映射 无表（domain-model:138 ↔ api:257/:261）逐层一致；**Action 机械 diff 实跑**：API 7 接口 == admin PageSpec 7 Action（空白归一化后 `diff` 输出 IDENTICAL 7/7，方法 GET×2/POST×5 逐行同）；readonly 2 Action = API 只读接口全集，`comm -23` 实差恰为 5 个 POST（与负空间表 5 行一一对应），readonly 无虚构接口（`comm -13` 空）；D3 关闭 PRD 待对齐-1/-2/-3/-4 的闭环两侧同记（api:29-33 决策行含「关闭 prd 待对齐-x」字样 ↔ prd:204-207 待对齐表）；**偏差（W1）**：domain-model.md:83 systemd_service 核心业务字段列缺「加载状态（loadState）」——BA 2.2.1.3（business-arch:100）、FL S-18-03（:113/:149）、PRD FR-05（prd:72）、API 4.1.1（api:161 loadState）、PageSpec Drawer（admin:141）五方均含该字段，唯 D2 对象行遗漏；同为透传展示属性的 sub 状态已列示，按本工作区「domain-model 行字段清单=权威盘点」惯例（monitoring metric 行 9 组字段与 API 全对齐）判定为真实跨阶段漂移 |
| S8 | 枚举池无冲突 | G1-07 | **PASS** | 实跑池标识符 `sort | uniq -c` 重复检查 = **空**（30 条全唯一，无跨域命名冲突）；API 引用 6 枚举取值与池逐字一致：service_active_state（04:37 active\|inactive\|failed ↔ api:123 ↔ admin:52 **三方同值**）、service_enabled_state（04:38 enabled\|disabled ↔ api:124 ↔ admin:53，⚠️A14 非两态透传口径两侧同记）、unit_type（04:39 11 值 ↔ api:125 ↔ admin:54 逐值一致）、panel_role（04:18 ↔ api:126 ↔ admin:55）；audit_action/audit_result 复用 account-auth 定稿与池一致（04:22/:28 ↔ api:127）；SVC_001 校验正则后缀集（api:137）= unit_type 11 值逐一对应；3 个服务枚举 D2 定稿已入池（04:37-39 均标注「定稿（D2 2026-11-12 新增）」）且 API 3.4 自述「本次为引用无新增」（:119）与池状态一致；domain-model v1.3 更新行自述「3 个服务枚举入池」（:147）与实数一致 |

**判定规则回执**：S1~S6 全过 → 非 FAIL；S7 有未通过项（W1）→ **总体判定 = WARN**（1 项 WARN 级文档修正 + 6 项 INFO 观察记录在案；无 CRITICAL 失败，不阻断，W1 修复后无需重跑全量 Gate）。

## G1-01~G1-11 逐项结论

| 编号 | 检查项 | 结果 | 依据（本次实读/实跑） |
|------|--------|------|----------------------|
| G1-01 | 架构文档与模块设计一致 | **PASS** | AA 1.1 承接 BA v1.0 并按 L3→DDD 归类（application-arch:13-22）；PRD 输入列明 D0+D1 三文档（prd:8）；API 输入列明 PRD+domain-model+D1（api:8）；FR 编号按 AA 9 子模块组织（prd:57 注明对应关系）；PageSpec 唯一数据源=api v1.0（admin:9）；六文档 ADR-008/ADR-018/R-03/R-08/R-15/双状态机口径数值与语义一致（证据见 S1） |
| G1-02 | PRD 覆盖所有业务场景和角色 | **PASS** | 2 角色+职责+获得方式+访客不涉及声明（prd:50-53）；9 L4 → FR-01~09 一一映射（prd:61-86）；异常场景 11 条覆盖通道不可用/操作失败/在途重复/目标消失/并发/static 边界/非 .service/参数非法/空结果/面板自身被停/越界拒绝（prd:163-175）；readonly 边界（ADR-018 无入口+越界拒绝）与 FL 附录A 对照一致（FL:197-204 ↔ prd:101） |
| G1-03 | API 设计完整（路径/请求/响应/错误码） | **PASS** | 实跑接口计数 = **7**（3.3 矩阵 :90-96、4.1.4 清单 :198-204、5.1 清单 :246-249 三处同数）；每接口含 query/req/校验顺序/响应 data 形状/错误码/审计动作码（:208-214）；错误码 SVC_001~008 八个 + 复用 AUTH_001/002/003/008（:107-117）覆盖 PRD N2 全部 11 个异常分支中接口侧可表达的项（名称非法→SVC_001、过滤非法→SVC_002、目标消失→SVC_003、类型边界→SVC_004、static 边界→SVC_005、超时→SVC_006、执行失败→SVC_007、通道不可用→SVC_008、越界→AUTH_008）；dbus 网关契约 SysdGateway 8 方法+JobResult 结构+约束行（:225-235）替代仓储契约 + 零持久化声明（5.2/5.3 :257/:261） |
| G1-04 | 交互设计覆盖所有页面类型 | **PASS** | 列表页完整（admin 第 4 节）；表单页 N/A 有显式诚实声明且给出无表单语义的依据（admin:127：无 save/update 类接口、零持久化、发现即接管无新增入口）；详情（展示）页=属性 Drawer 形态判定附理由（admin:131：9 字段单区块、运维高频不离开列表上下文）；审批 N/A 有据（prd:137 无审批语义）；config 页面类型判定附不满足 hybrid/code 的理由（admin:19）；readonly 子集+负空间声明（readonly:10/:31-41） |
| G1-05 | API 字段与交互规格字段对齐 | **PASS** | 逐字段核对：list data 9 键（items 7 字段+total+generatedAt，api:208）↔ 表格 7 列均标注 items[] 字段来源 + 「共 N 个」+ generatedAt 灰字（admin:86-96/:82）——无臆造字段，列表不含 description 与 API 载荷口径一致（api:208 ↔ admin 未渲染该列）；detail data 9 键（api:209）↔ Drawer 7 展示字段（admin:135-141，canOperate/canToggleAutostart 为行为布尔不展示，readonly:54 注明「仍接收但不渲染」）；动作 req `{name}` ↔ 全部操作以 unit 名称寻址（D-09，admin 操作列/开关均按行寻址）；错误码 SVC_001~008+AUTH_001/002/003/008 文案 ↔ api 3.4 一一对应（admin:22 ↔ api:107-117，含 SVC_006 提示文案与 api data.note 语义一致） |
| G1-06 | 状态机定义完整（状态/流转/触发条件） | **PASS** | 双维度状态机三要素齐：运行维度 4 流转行+自启维度 2 流转行均含当前态/目标态/触发/备注（prd:112-125）；面板侧「无自有状态机写入」的定位声明及 4 项辨析（瞬时执行态不建模/subState 非状态机/两维度正交/投影刷新非写入，api:188 ↔ prd:130-135）；T3 审批 N/A 有依据（prd:137） |
| G1-07 | 枚举统一且无冲突 | **PASS** | 实跑：池 30 标识符无重复；API 3.4 的 6 枚举取值与池逐条一致（04:37-39/18/22/28 ↔ api:123-127）；PageSpec 字典 Key 三方同值（admin:52-55）；业务常量与枚举边界清晰——5 个审计动作码与 3 项操作常量均标注「非枚举池条目」（api:129-137），PageSpec 第 3 节未误登记为字典 |
| G1-08 | 接口路径符合 /domain/resource/action | **PASS** | 7 接口全部 `/services/unit/{action}` 三段式（api:64 声明 + grep 实证 7/7；域=services、资源=unit、动作=list/detail/start/stop/restart/enable/disable） |
| G1-09 | 设计文档 changelog 已填写 | **PASS** | 8 份文档均有版本属性表+变更记录（明细见 S5）；API 6.1 使用词汇表内 `initial` 类型（api:269/:273-283）；domain-model 按「每完成一个模块追加」累积 v1.0~v1.3 且 service-mgmt 追加行自述与实际产物一致（:147） |
| G1-10 | 状态索引模块状态已更新 | **PASS** | INDEX.md:59-64 service-mgmt requirement/api/interaction 均 done 且 note 含产物路径与计数口径（「7 个 HTTP 接口」「S7 7/7 Action 映射 + S8 枚举机械校验通过」与本次独立复验一致）；gates.gate1 pending 由本次检查落盘为 warn（本报告第 9 节动作） |
| G1-11 | 跨阶段模型一致性 | **WARN**（W1） | 通过部分：D2 无聚合根+1 实体 ↔ API 1.4 ↔ 4.1（无 DDL 与存储映射「无表」闭环）；无子流程/无关联领域对象/无领域事件 三文档同口径（domain-model:99/:110/:131-132、api:45/:192/:222、prd 第六部分）；PageSpec 7 Action ↔ API 7 接口机械 diff 一致（admin IDENTICAL 7/7；readonly 2+5 负空间，见 S7）；偏差 W1：domain-model:83 字段清单缺 loadState（详见 S7 行与 W1 项）——同字段在 BA/FL/PRD/API/PageSpec 五方齐备，属 D2 对象行文档级遗漏，不影响接口契约与页面渲染（五方口径已闭合），修复成本一行 |

## 执行展开（validation skill Gate 1 三大块）

### 1. PRD 就绪检查

| 层级 | 通过/总数 | 状态 |
|------|----------|------|
| 第一层 G1-G5 | 5/5 | PASS（prd 附录自查 :217-223 与实读一致） |
| 第二层 T1-T9 | 适用 4/4，N/A 6 项有据 | PASS（T1 :229、T1a :230、T2 :231、T4 :233；T3 :232、T5 :234、T6/T7/T8 :235-237、T9 :238） |
| 第三层 N1-N4 | 4/4（N1 部分锁定已声明） | INFO（4 项未锁定缺口经 D-01~D-05 全部关闭，见 I5） |

### 2. API 文档完整性（M1~M5 机械 + GL 全局）

| 章节 | 结果 | 说明 |
|------|------|------|
| M1 章节结构 | PASS | 1.1~1.5/3.1~3.4/4.1（含 4.1.1~4.1.5 完整子结构）/5.1~5.3/6.1~6.3 全部存在且非空；第 2 章按 d3 裁剪矩阵对工具类=「—」裁剪，且 api:56 显式声明不适用+理由（不变量 I-01~I-07 列于 4.1.1 替代其职能）；第 4 章标题「领域对象与服务详细设计」为无聚合根场景的组织适配，:143 已声明 |
| M2 路径格式 | PASS | 7/7 三段式（grep 实证） |
| M3 枚举引用有定义 | PASS | 6/6 在池（04:37-39/18/22/28），无重复 |
| M4 占位符残留 | PASS | grep exit=1（API+PRD+domain-model+两 PageSpec+BA/AA/FL 八文件均无 `{按需填写}`/TODO/FIXME/TBD/待补充） |
| M5 变更记录 | PASS | 6.1 存在含 v1.0 initial 行（api:269） |
| GL 全局一致性 | PASS | GL-001a/b、002、003、004~009、012、021~022、026~028 通过（明细见 S4 行）；GL-010/011 按项目声明契约口径（读=GET/写=POST 内部契约、code/message/data 包装，见 I4）；GL-029/030/031 N/A（无子表懒加载、非主数据类）；GL-032 通过（API 无 dict 端点 grep 实证；前端常量偏差见 I3） |

### 3. 前后端契约一致性（CT）

| 编号 | 检查项 | 状态 |
|------|--------|------|
| CT-001 | API 路径与 PageSpec Action 路径对齐 | PASS（7/7 机械 diff 一致，API↔admin 全等；readonly 2 读 Action=API 只读全集） |
| CT-002 | 请求/响应字段与页面字段对应 | PASS（G1-05 明细：列表 7 列/控件条 2 参数/Drawer 7 字段逐键对应） |
| CT-003 | 枚举值前后端一致 | PASS（3 服务枚举+panel_role 池=API=PageSpec 三方同值） |
| CT-004 | Action 表所有 Action 在 API 有对应接口 | PASS（7/7，无虚构、无遗漏；readonly 负空间 5 项=api 3.3 ✗ 项逐项对应） |
| CT-005 | 列表页列字段来源于 API ListVO | PASS（7 列全部标注 items[] 字段来源，无臆造） |
| CT-006 | 表单页字段来源于 API SaveAO | N/A（无表单页，诚实声明 admin:127） |
| CT-007 | 字典 Key 与后端枚举对应 | PASS（admin:52-55 ↔ 04:37-39/18） |
| CT-008 | 分步表单 generate-id/save 接口 | N/A（无分步表单场景，PageSpec 未声明分步） |

## WARN 项

| # | 项 | 说明 | 修复建议 |
|---|-----|------|---------|
| W1 | domain-model.md:83 systemd_service 字段清单缺「加载状态（loadState）」 | G1-11/S7 跨阶段字段漂移：BA 2.2.1.3（business-arch:100「描述、加载状态、运行状态与 sub 状态、开机自启状态、unit 类型等」）、FL S-18-03 两角色行（function-list:113/:149）、PRD FR-05（prd:72）、API 4.1.1（api:161 loadState）、PageSpec Drawer（admin:141）五方均将加载状态列为一等属性；唯 domain-model.md:83 核心业务字段列为「名称、unit 类型、运行状态、sub 状态、开机自启状态、描述」6 项无加载状态。同为透传展示属性的 sub 状态已列示，且本工作区惯例为 domain-model 行字段清单=权威盘点（monitoring metric 行 :81 九组字段与 API 全对齐），故判为遗漏而非裁剪 | 下次触碰 domain-model.md 时在该行核心业务字段列补「加载状态」四字（v1.3 行无需重写，随下次模块追加时一并修订）；不影响接口契约/页面/代码生成（下游五方口径已闭合），不阻断进入 B 阶段 |

## INFO 项（观察记录与建议，无需重跑本 Gate）

| # | 项 | 说明 |
|---|-----|------|
| I1 | PRD「台账（状态观测与变更操作）型」与 D3「工具类」标型差异 | prd:213 附录自述功能类型为「台账（状态观测与变更操作）型」，而 API/PageSpec 按 d3 判定规则 1（无聚合根→工具类）标「工具类」（admin:18）。两标型分属 D2 完备性分层（T4/T5 适用性裁剪）与 D3 API 裁剪矩阵两个不同用途，各自内部自洽；操作侧（D3 裁剪与接口形态）实际采用的是工具类结论且全文档一致，无下游冲突。建议后续模块统一在 PRD 附录标注 d3 判定类型以免歧义 |
| I2 | ACTIVE_STACKS=go-vue3 悬空绑定 | `.claude/settings.json` 实证 `ACTIVE_STACKS: "go-vue3"` 但 `profiles/` 下仅 dougong-panel（ls 实证）——无栈规则文件存在，本 Gate 栈级规则层为空。设计产物头部按「无激活栈」声明成文（api:9、admin:10），与 monitoring 报告 I2 同一事实（绑定变更发生在设计产物成文后）。**建议运行所有者在进入 B/F 阶段前确认 ER-03 绑定一致性**（补齐 profiles/go-vue3 或回正 settings），不影响本 Gate 判定 |
| I3 | 字典数据源偏差（无 dict/batch，前端 enums.ts 常量） | PageSpec 第 3 节（admin:57）声明为设计决策「待 Gate 1 复核」——本次复核**接受该偏差**：枚举全固定值（3+1 枚举）、无激活栈规则、04-business-enum.md 为单一事实源；API 侧 GL-032 精神已满足（grep 实证无独立字典端点）。与 account-auth/panel-core/monitoring 三先例同判；后续接入栈 profile 时按 GL-032 重审 |
| I4 | GL-010（统一 POST）/GL-011（Result\<T\>）口径说明 | 本项目契约=读 GET/写 POST（api:65 自述「面板内部契约，account-auth D-07 同一约定」，7 接口 GET×2+POST×5 全遵守）+ `{"code",message,data}` 等价包装（api:66）；与 validation skill GL-010「业务接口统一 POST」的 scm 栈口径不同，按无栈通用契约口径通过（monitoring 先例 I4 同判）；非偏差缺陷 |
| I5 | PRD N1 未锁定项已全部闭环 | prd:204-207 待对齐-1/-2/-3/-4 四项在 D3 全部处置：等待上限/超时→D-01（90s+SVC_006）、分页与排序→D-02、刷新机制→D-03、subState 形态→D-04、自启非两态字段形态→D-05（canToggleAutostart 权威布尔）——各决策行均含「**关闭 prd-service-mgmt.md 待对齐-x**」字样，无悬挂设计缺口；D4 呈现侧（刷新纪律/非两态灰字降级）在 PageSpec :40-46/:68 落地 |
| I6 | FX-001~141 前端质量清单未随 payload 发布 | 本次实跑 find（payload 全目录 *FX*/*frontend*checklist* 模式）无命中——PageSpec 注记（admin:178、readonly:91）属实。两份 PageSpec 注记表述为「全量留待 Gate 1」，但清单文件不存在故本次按可执行部分执行：9 节结构齐、Action 三方机械 diff、字典三方同值、负空间逐项对应、第 8 节 12 项自查证据核验均已完成；FE-001~141 命名条目属 Gate 2 前端规则扫描范畴（validation skill「前端规则扫描」节），代码完成后执行（同三先例口径） |

## 栈特化条目 N/A 声明

`profiles/go-vue3` 不存在（ls 实证）→ 无栈级规则文件可叠加（validation skill FR-D2 分层组装的栈层为空集）；按 validation skill「异栈激活时以该栈 rules 替换本节清单」与 account-auth/panel-core/monitoring 三先例：CH3-002/003（统一 POST/Result\<T\>）、CH4-005a~g/007/008（审计七件套/主键规约）、CH4-057~070（MQ/事务边界/领域事件）、CH5-004~018（DDL/SQL 规约）、GL-029~031 等栈强绑定条目 N/A——其中多数在本模块**天然无对象**（零持久化→无 DDL/SQL/审计字段/仓储；无 MQ 单机单体→无领域事件/事务边界/MQ 消息），其余通用意图（契约完整性、路径/错误码/枚举一致性、`{code,message,data}` 契约、权限矩阵完备性）已在对应 G1 项按 api 3.1 声明的通用 REST 契约检查通过。

## 总结

**判定：WARN**（S1~S6 全过、S8 过；S7 一项 WARN 级字段清单偏差 W1 → 依判定规则「仅 S7~S8 未过=WARN」；无 CRITICAL 失败，不阻断，W1 为一行文档修正）

- G1-01~G1-10：10/10 通过；G1-11：WARN（W1 一项）
- S1 三文档全局：BA/AA/FL 完整且 9 L4 覆盖闭环（6 交互子模块 10 菜单行×两角色 + 3 机制型经全局行为/AA 附录A 恰 3 行映射），模块编码 C-04~C-05/S-17~S-18 连续性机械实证；S2 PRD：G 5/5、T 适用 4/4、N 4/4、FR-01~09 ↔ 9 子模块一一对应；S3 领域模型：5 类对象分类+无聚合根 A1-A4 判定+实体/值对象区分论证；S4 API：M1~M5 全过（工具类裁剪矩阵实证）+ GL 适用项通过；S5 变更摘要：8 文档 changelog 齐；S6 PageSpec：9 节结构+机械项过，readonly 负空间与 api 权限矩阵逐项对应
- S7 跨阶段一致性：机械 diff 实证 7 Action ↔ 7 接口（admin IDENTICAL 7/7、readonly 2+5 负空间）、领域对象主体与存储映射（无表）逐层一致；唯一偏差 W1=domain-model 字段清单缺 loadState（五方有、D2 无）
- S8 枚举池：30 标识符无重复、6 枚举引用三方同值、SVC_001 正则后缀集与 unit_type 逐值对应
- WARN 1 项（W1）+ INFO 6 项（I1 标型口径、I2 ACTIVE_STACKS 悬空绑定待所有者确认、I3 字典偏差复核接受、I4 GL-010/011 口径、I5 N1 缺口闭环确认、I6 FX 清单归 Gate 2）
- B/F 阶段入场注意：以 api-service-mgmt.md 7 接口为验收计数口径（零持久化、无 DDL/SQL）；SysdGateway dbus 契约（api:225-235，R-08 禁 shell）、同步等待终态 90s 语义（D-01/SVC_006）、canOperate/canToggleAutostart 权威布尔（D-05）与刷新纪律（PageSpec 第 2 节：手动+操作驱动、动作成功不整表重拉）为 B/F 实现验收要点；FE/FX 全量清单在 Gate 2 执行

---

# 复检第 1 轮（2026-11-12）

**范围**：按同一检查清单 S1~S8 复核 service-mgmt 全部产物；重点核验首轮 W1，其余项回归抽查。检查对象与首轮相同（9 份产物 + 枚举池 + INDEX）。

## 变更范围识别（mtime 实证）

`ls -l --time-style=+%FT%T` 实跑：9 份产物中**仅 domain-model.md（20:51:42）晚于首轮报告落盘（19:54:16）**——恰为 W1 指向文件；BA/AA/FL（19:12~19:15）、枚举池（19:28）、PRD（19:27）、API（19:36）、两 PageSpec（19:41）mtime 均早于首轮实读，自首轮检查后未变更。首轮对未变更产物的实读结论（S1/S2/S3/S5/S6 及 S4/GL 全部引用行号）经 mtime 回归确认仍有效。

## W1 修复核验（重点项）— **PASS**

| 核验点 | 结果 | 证据（实跑命令与输出） |
|--------|------|------|
| 字段补录 | ✅ | `grep -n 'loadState\|加载状态' domain-model.md` → **:83** systemd_service 行核心业务字段列现含「加载状态（loadState，透传属性）」（位于 sub 状态后、开机自启前）；至此 BA 2.2.1.3（business-arch:100）/FL（:113/:149）/PRD FR-05（prd:72）/API 4.1.1（api:161）/PageSpec Drawer（admin:141）/D2（domain-model:83）**六方一致**。D2 7 字段+枚举 ↔ API 4.1.1 9 字段的剩余差异仅 canOperate/canToggleAutostart 两个计算布尔（D-05/⚠️A14 的 D3 合法新增，首轮已判合理）→ G1-11/S7 字段清单一致性成立 |
| 修改范围受控 | ✅ | `grep -n systemd_service domain-model.md` 输出 :52/:83/:88/:138 四处，:52（A1-A4 判定）/:88（注释）/:138（存储映射）逐字与首轮实读一致——仅 ：83 字段列与 ：148 变更，未触碰分类/枚举/存储映射（与 W1 修复建议「一行文档修正」相符） |
| changelog 与版本 | ✅ | 更新记录追加 v1.4 行（:148，注明「Gate 1 W1 修正（2026-11-12 复检）」+补录依据）；版本头 v1.3→v1.4（:11）；GL-001a 复跑：头部「最后更新 2026-11-12」（:12）>= 最新条目日期 ✓ |

## S1~S8 回归抽查（机械项全部重跑，其余按 mtime 未变回归确认）

| # | 项 | 重跑命令/方法 | 结果 |
|---|-----|--------------|------|
| S1~S3/S5/S6 | 三文档/PRD/领域模型/changelog/PageSpec | mtime 回归（见上节）+ 首轮实读结论 | **PASS**（产物未变更，首轮证据行号仍有效） |
| S4 | M2 路径格式 | `grep -oE "(GET\|POST) /services/unit/[a-z]+" api-service-mgmt.md \| sort -u` | **PASS**（7/7 三段式不变：list/detail/start/stop/restart/enable/disable） |
| S4 | M4 占位符 | `grep -rnE '\{按需填写\}\|TODO\|FIXME\|TBD\|\{待填写\}\|待补充'`（8 文件，**含修改后的 domain-model.md v1.4**） | **PASS**（exit=1 全 clean） |
| S5 | changelog 存在性 | `grep -cE '^## 更新记录\|^### 6.1 变更记录'`×6 + PageSpec 第 9 节 grep | **PASS**（8/8；domain-model 现累计 v1.0~v1.4 五行） |
| S7 | Action 三方机械 diff | 空白归一化 `diff`（API↔admin）+ `comm -23`/`comm -13`（API↔readonly） | **PASS**（admin IDENTICAL 7/7；API minus readonly 恰 5 POST=负空间表；readonly minus API 空） |
| S7 | W1 专项 | 见上节六方一致核验 | **PASS** |
| S8 | 枚举唯一性 | 池标识符 `sort \| uniq -c \| awk '$1>1'`（输出空）+ 总数 `wc -l` = 30 | **PASS**（无重复） |
| S8 | 三方同值抽查 | 逐文件 sed -n 取 service_active_state 行比对（04:37/api:123/admin:52） | **PASS**（三方均为 active\|inactive\|failed，PageSpec 分隔符 `/` 值同）；池/API/PageSpec mtime 未变，其余 5 枚举沿用首轮逐字核对结论 |

## 复检判定

**PASS**（S1~S8 全过：W1 已按建议修复并经六方一致核验；无新增发现；首轮 6 项 INFO 沿用——其中 I2 ACTIVE_STACKS=go-vue3 悬空绑定仍待所有者确认，不属产物缺陷、不阻断）。G1-01~G1-11：11/11 通过（G1-11 由 WARN 转 PASS）。INDEX.md gates.gate1 由 warn 更新为 pass。

**B/F 阶段入场要点沿用首轮总结节**（接口计数 7、SysdGateway 契约、D-01 终态语义、D-05 权威布尔、刷新纪律；FE/FX 归 Gate 2）。
