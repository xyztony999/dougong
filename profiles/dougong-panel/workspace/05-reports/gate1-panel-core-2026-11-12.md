# Gate 1 验证报告 — 设计→代码门控

**模块**：panel-core（面板基座）
**检查时间**：2026-11-12
**检查者**：门控检查员·panel-core（validation skill · Gate 1；检查项编号唯一来源：engine-rules.md G1-01~G1-11；流程预检 S1~S8 见 design SKILL.md「Gate 1 阶段汇总预检」）
**产物版本**：BA/AA/FL v1.0、PRD v1.0、domain-model v1.1、API v1.0、PageSpec admin/readonly v1.0
**Profile 绑定**：dougong-panel（`ACTIVE_STACKS: ""` 实证于 .claude/settings.json → 无激活栈；validation skill「未激活 → Gate 1 可对裸工作区执行」适用；栈强绑定执行条目按 account-auth 先例口径检查，见「栈特化条目 N/A 声明」）

## 检查对象

| 阶段 | 文件（均本次实读） |
|------|------|
| D1 | 00-architecture/business-arch-panel-core.md、application-arch-panel-core.md、function-list-panel-core.md |
| D2 | 02-detailed/prd/prd-panel-core.md、prd/domain-model.md（panel-core 段落：本域判定/业务域行/领域对象 3 行/子流程注/关联注/集成 4 行/存储映射） |
| D3 | 02-detailed/api/api-panel-core.md |
| D4 | 02-detailed/interaction/admin/panel-core/panel-core.md、interaction/readonly/panel-core/panel-core.md |
| 参考 | 01-solution/04-business-enum.md（24 条，其中 panel-core 定稿 4 条）、workspace/INDEX.md、engine-rules.md、design/validation SKILL.md 及 d1~d4 分册 |

## S1~S8 阶段汇总预检结论（判定主表）

| # | 预检项 | 规则对应 | 结果 | 关键证据（实读/实跑） |
|---|--------|---------|------|------|
| S1 | 三文档全局检查 | G1-01 | **PASS**（附 W1/W2） | BA 4 L2/7 L3/16 L4 层级完整（business-arch:61-167），第三部分树 16 L4 与第二部分一一对应（:173-202）；AA 应用域分层表 4 行含空层理由（application-arch:62-67），7 模块 S-06~S-12/16 子模块均带功能说明+用户类型复选框+最末级调用关系（:73-304），树=正文（:310-336）；BA 7 L3↔AA 7 模块、16 L4↔16 子模块逐一同名对应；FL 两角色分章覆盖 16/16 子模块（9 子模块有章节行×2 角色、7 个机制型经概览全局行为段/附录覆盖，AA 附录A 映射恰 10 行 :340-356），第三部分视图=第二部分（function-list:242-296）；用户类型名称 AA/FL 一致（admin/readonly）；三文档更新记录均表格格式 v1.0 |
| S2 | PRD 完备性检查 | G1-02 | **PASS** | G 5/5（目标 prd:13、IN 5/OUT 6 :19-38、3 角色 :42-48、领域对象=domain-model:56-58、对象关系列齐全）；T 适用项通过（T1 字段=对象清单+AA 附录C 9 项全集；T1a 被引用 3 链 prd:134-138）；T2/T3/T4/T5/T7/T8/T9 共 8 项 N/A 均有据（无状态机辨析 prd:115-124、单例读无台账 :128-130、R-09 受限 :197）；T6 N/A（受限）+ 待对齐-1 显式跟踪（:166）；N 4/4（N2 十个异常分支 :146-157）；FR-01~16 ↔ 16 子模块一一对应（:56-93） |
| S3 | 领域模型分离 | G1-06 / G1-11 | **PASS**（附 I2） | 5 类对象分类完成：聚合根 panel_config（A1-A4 依据注明 domain-model:56）、域内对象-实体 HTTPS 证书（1:1 :57）、值对象 配置生效分类（:58）、子流程无（:70 注明理由）、关联领域对象无（:79 注明 OHS 消费不持 ID）、过程数据已归类（:61 + api:47）；聚合根已确认；G1-06 状态机：无生命周期状态机为 D0 拍板且经易混淆项逐项辨析（prd:115-124、api 4.1.2），非缺失 |
| S4 | API 全局模板合规（M1~M5 + GL） | G1-03 / G1-08 | **PASS** | M1 章节 1.x~6.x 全部存在非空（api:11-322，2 章以单聚合根声明填充）；M2 实跑 `grep -E '(GET\|POST) /panel/'` = 7/7 三段式 `/panel/{config,menu,system}/{action}`；M3 4 枚举均在池（04:30-33）；M4 实跑占位符 grep（按需填写/TODO/FIXME/TBD）exit=1 无残留；M5 6.1 存在含 v1.0 initial 行（api:300-304）；GL-001a/b、003、004~008、009、012、021~022、026~028 通过，GL-010/011 按 D-07 无栈声明口径（读 GET/写 POST + code/message/data 包装，同 account-auth 先例），GL-029~031 N/A（单例 detail 全量返回、无 options/dict 端点——grep 实证 exit=1，PageSpec 第 3/4 节偏差声明），GL-032 偏差复核接受（同 account-auth I5） |
| S5 | API 变更摘要 | G1-09 | **PASS** | api 6.1 v1.0 initial 行含变更类型/摘要/目标节号列（:300-304）；6.2 变更类型词汇表、6.3 版本号规则齐（:306-322）；全部 8 份设计文档 changelog 齐：BA:246-248、AA:415-417、FL:328-330、PRD:224-226、domain-model:101-104、api:300-304、PageSpec admin:208-212、readonly:88-90 |
| S6 | PageSpec 输出校验 | G1-04 / G1-05 | **PASS**（附 I4） | 9 节结构齐（两份）；模块元信息齐含唯一数据源声明（admin:13-22）；Action 表 7 行含方法/路径/页面/权限/CSRF 列（admin:26-34）；字典 Key 4+1 同源池（:40-46）；无列表页/无详情路由诚实声明（:66-68、:167-169）；6 Card 表单+about（:74-165）；路由 2 条+框架级行为（:171-187）；readonly 负空间 4 Action 与 API 3.3 矩阵 ✗ 项逐项对应（readonly:26-32）；G1-05 字段逐项对齐（见 CT-002）；FX-001~141 清单文件未随 payload 发布经本次 `find` 实证，结构级+机械项已执行 |
| S7 | 跨阶段一致性 | G1-11 | **PASS** | 领域对象：D2（domain-model:56-58）↔ API 1.4（api:43-47）↔ 4.1 表结构（panel_config/panel_cert :167-194）↔ 存储映射（domain-model:95-97）逐项一致；待对齐-3/-4 闭环（PRD:168-169 已定稿 ↔ D-P01/D-P04）；Action：实跑机械 diff——API 7 路径 ↔ admin PageSpec 7 Action **一一对应、无虚构无遗漏**，readonly 3 Action 为其子集、4 负空间=API 3.3 admin-only 4 项；方法/权限/CSRF 三列逐行一致 |
| S8 | 枚举池无冲突 | G1-07 | **PASS** | 实跑池标识符 `uniq -d` = 无重复（24 条全唯一）；API 引用 4 枚举（https_mode/listen_scope/panel_cert_source/config_effect_mode）取值与池 04:30-33 逐字一致；DDL CHECK（api:275/280/290）与池一致；panel_cert_source vs 网站管理 cert_source 命名/取值区分已注明（04:32）；panel_role/audit_action 引用均在池定稿（04:18/22） |

**判定规则回执**：S1~S6 全过 → 非 FAIL；S7~S8 全过 → 非 WARN。**总体判定 = PASS**（2 项建议 W1/W2 与 5 项 INFO 记录在案，均为文档元数据小修，不阻断、无需重跑本 Gate）。

## G1-01~G1-11 逐项结论

| 编号 | 检查项 | 结果 | 依据（本次实读/实跑） |
|------|--------|------|----------------------|
| G1-01 | 架构文档与模块设计一致 | **PASS** | AA 1.1 承接 BA v1.0（application-arch:13）；PRD 输入列明 D1 三文档（prd:8）；API 输入列明 PRD+domain-model+D1（api:8）；FR 编号按 AA S-06~S-12 组织（prd:54-93）；PageSpec 唯一数据源=api v1.0（admin:9） |
| G1-02 | PRD 覆盖所有业务场景和角色 | **PASS** | 3 角色含职责与获得方式（prd:42-48）；16 L4 → FR-01~16 一一映射（S-06-01→FR-01 … S-12-02→FR-16，prd:56-93）；异常场景 10 条（:146-157）；readonly 边界场景（越权拒绝 :154）与 FL 附录A 对照一致 |
| G1-03 | API 设计完整（路径/请求/响应/错误码） | **PASS** | 实跑接口计数 = **7**（api:223-229 接口表 + 4.1.4 详情 bullet 同为 7）；每接口 req/data/校验顺序/审计口径齐备（:235-241）；错误码 PANEL_001~006 + 复用 AUTH_001/002/003/008/012（:125-136）覆盖 PRD N2 全部 10 个异常分支（custom 地址 PANEL_001、证书 PANEL_002/003、更新检查 PANEL_004/005、语言 PANEL_006、越权 AUTH_008、参数 AUTH_012）；2 表 DDL（:272-295）+ 5 条 SQL 清单（:262-268）+ 4 项进程内契约（:251-258） |
| G1-04 | 交互设计覆盖所有页面类型 | **PASS** | 表单页=settings 6 Card（admin:74-151）；关于页 about（:153-165）；列表页/详情页 N/A 有显式诚实声明（单例配置无台账，admin:66-68、:167-169）；审批 N/A 有据（本域无审批语义，prd:124）；readonly 只读渲染差异逐 Card 声明（readonly:44-57）；路由 2 条+框架级行为落位（admin:171-187） |
| G1-05 | API 字段与交互规格字段对齐 | **PASS** | 逐字段核对：detail data 11 组键（listen.scope/address/port、entryPath.enabled/path、https.mode+cert.source/issuer/notBefore/notAfter、updateCheck.enabled、lang.value/supported、session.absoluteTimeout/idleTimeout、pendingRestart，api:235）↔ 各 Card 字段（admin:87-93/98-100/113-117/129/136-139/147-149）；update req 7 字段（api:236）↔ Card 1/2/3/4/6 提交字段；upload req certPem/keyPem（:238）↔ Card 3 导入区；version data 3 字段（:240）↔ about（admin:159-163）；update-check data 4 字段（:241）↔ Card 6 结果区——均一致，无臆造字段 |
| G1-06 | 状态机定义完整 | **PASS**（以「无状态机」完整论证满足） | 本域无生命周期状态机为 D0 拍板；PRD 第六部分对 4 个易混淆项逐一辨析（模式枚举/布尔开关/派生提示态/角色判定结果，prd:115-124）；API 4.1.2 同口径并给出 pendingRestart 派生规则（api:213）；T3 审批 N/A 有依据（:124） |
| G1-07 | 枚举统一且无冲突 | **PASS** | 实跑：池 24 标识符无重复；API 3.4 的 4 枚举取值与池逐条一致（04:30-33 ↔ api:142-145）；DDL CHECK 约束一致（api:275/280/290）；PageSpec 字典 Key 同源（admin:40-46）；cert_source/panel_cert_source 区分已声明（04:32）；lang/布尔项非枚举的口径两文档一致（api:147、admin:48） |
| G1-08 | 接口路径符合 /domain/resource/action | **PASS** | 7 接口全部 `/panel/{resource}/{action}` 三段式（api:83 声明 + grep 明细）；资源 config/menu/system |
| G1-09 | 设计文档 changelog 已填写 | **PASS** | 8 份文档均有版本属性表+更新记录表（明细见 S5）；API 6.1 使用变更类型词汇 `initial`（词汇表 :306-318） |
| G1-10 | 状态索引模块状态已更新 | **PASS** | INDEX.md:38-40 panel-core requirement/api/interaction 均 done 且 note 含产物路径与计数口径；gate1 pending 由本次检查落盘为 pass |
| G1-11 | 跨阶段模型一致性 | **PASS** | D2 1 聚合根+1 实体+1 值对象 ↔ API 1.4 总览 ↔ 4.1 两表；存储映射（domain-model:95）↔ 5.3 DDL 2 表（配置生效分类=域级规则无表，两处一致）；无子流程/无关联领域对象三文档同口径（domain-model:70/79、api:217/45、prd 第六部分）；PageSpec 7 Action ↔ API 7 接口一一对应（机械 diff 实证，见 S7） |

## 执行展开（validation skill Gate 1 三大块）

### 1. PRD 就绪检查

| 层级 | 通过/总数 | 状态 |
|------|----------|------|
| 第一层 G1-G5 | 5/5 | PASS（prd 附录自查 :179-185 与实读一致） |
| 第二层 T1-T9 | 适用 2/2，N/A 8 项有据 | PASS（T1 :191、T1a :192；T6 N/A（受限）依据 R-09+待对齐-1 跟踪 :197；其余 N/A 依据 prd:193-200） |
| 第三层 N1-N4 | 4/4（N1 部分锁定已声明） | INFO（I1） |

### 2. API 文档完整性（M1~M5 机械 + GL 全局）

| 章节 | 结果 | 说明 |
|------|------|------|
| M1 章节结构 | PASS | 1.1~1.5/2.1~2.3/3.1~3.4/4.1.1~4.5/5.1~5.3/6.1~6.3 全部存在且非空；2 章以「单聚合根声明」填充并给出无跨聚合不变量/编排的理由（api:69-75） |
| M2 路径格式 | PASS | 7/7 三段式（grep 实证） |
| M3 枚举引用有定义 | PASS | 4/4 在池（04:30-33），无重复 |
| M4 占位符残留 | PASS | grep exit=1，🔒 章节无 `{按需填写}`/TODO |
| M5 变更记录 | PASS | 6.1 存在含 v1.0 initial 行 |
| GL 全局一致性 | PASS（附 I3） | GL-001a 头部 2026-11-12=更新记录日期 ✓；GL-001b 必填章节非空 ✓；GL-002 主要选填章节均注明跳过原因（2 章/4.1.2/4.1.3/4.1.5），4.1 标签池子项未标注（I3）；GL-003 D-P10 混合模式声明 ✓；GL-004~008 总览/子流程/关联/角色/依赖与 Ch4 一致 ✓；GL-009/010/011 按无栈 D-07 口径 ✓（读 GET/写 POST、code/message/data）；GL-012 写接口均有权限+CSRF ✓；GL-021/022/026/027/028 枚举闭环 ✓（来源标注为文件级 04-business-enum.md，池仅单章节，无歧义）；GL-029/030/031 N/A（单例全量 detail、无子表开关、无 options 语义场景——API 无 options/dict 端点 grep 实证）；GL-032 无独立字典端点 ✓（前端常量偏差见 I5） |

### 3. 前后端契约一致性（CT）

| 编号 | 检查项 | 状态 |
|------|--------|------|
| CT-001 | API 路径与 PageSpec Action 路径对齐 | PASS（7/7 机械 diff 一致） |
| CT-002 | 请求/响应字段与页面字段对应 | PASS（G1-05 明细） |
| CT-003 | 枚举值前后端一致 | PASS（4 Key 池=API=PageSpec 三方同值） |
| CT-004 | Action 表所有 Action 在 API 有对应接口 | PASS（7/7，无虚构；readonly 3 项为子集） |
| CT-005/006 | 列表列/表单字段来源于 API VO/AO | PASS（无列表页；表单字段均有 req/data 依据） |
| CT-007 | 字典 Key 与后端枚举对应 | PASS（admin:40-46 ↔ 04:30-33 + panel_role:18） |
| CT-008 | 分步表单 generate-id/save 接口 | N/A（无分步表单场景，PageSpec 未声明分步） |

## WARN 建议项（记录在案，不阻断，均文档小修）

| # | 项 | 位置 | 说明 |
|---|-----|------|------|
| W1 | R-10 口径笔误「建议但强制」 | business-arch-panel-core.md:13、function-list-panel-core.md:13、prd-panel-core.md:100 | 三处将 R-10 的「建议**但不**强制 HTTPS」误写为「建议但强制」——与同文档其余 18 处正确口径（BA 7 处/FL 3 处/AA 4 处/PRD 3 处/PageSpec 1 处，grep 实证）及实现相矛盾（https_mode 默认 off、PageSpec 仅 HTTP 首访提示横幅）。PRD:100 位于规则汇总表（规范语境），建议三处一并改为「建议但不强制」 |
| W2 | FL/PRD 菜单统计口径不吻合 | function-list-panel-core.md:330、prd-panel-core.md:52 | 两处声称「10 菜单行 + 6 机制型子模块」；正文实际为每章 9 行（7 具名交互 + 2「无独立菜单」全局行为行）、7 个子模块无章节行经全局行为段/附录覆盖、AA 附录A 机制型映射恰 10 行（application-arch:344-356）——疑为「6 交互 + 10 机制型」标签互换。16/16 覆盖本身完整（FR↔子模块↔FL 一一对应成立），仅统计口径建议统一（如「7 具名交互菜单×2 角色 + 9 机制型子模块经全局行为/附录覆盖」） |

## INFO 项（观察记录，无需整改）

| # | 项 | 说明 |
|---|-----|------|
| I1 | PRD N1 未锁定项（更新检查外呼超时/证书大小上限/自签有效期/入口路径字符集） | prd:142 已声明不虚构、B 阶段补齐；API 3.2 同步标注「超时上限为编译期常量（N1 待 B 阶段实测校准）」（api:101）——两文档口径一致 |
| I2 | domain-model.md 两处元数据滞后 | :61 引用「prd-panel-core.md 过程数据分类表」——PRD 无同名表（实质内容在 prd:119-122 辨析行 + api:47 过程数据行，P2/P7 分类齐全）；:57 HTTPS 证书行备注「物理存储 D3 复核」未随 D3 定稿回写（同文件 :97 存储映射说明与 PRD:169 待对齐-4 已记录定稿结论）。建议下次触碰该文件时顺手修正 |
| I3 | API 4.1 未按模板括注补「本聚合根无标签池」一行 | d3 模板 4.1 结构含标签池子项（d3-api.md）；本模块无标签语义且无栈 profile，GL-002 主体已满足，补一句标注即可闭环（CH4-022 语义） |
| I4 | FX-001~141 前端质量清单未随 payload 发布 | 本次实跑 find：payload 下仅 profiles/java-microservice/skills/backend-specs/15-validation-checklist.md，无前端通用 FX/FE 清单——PageSpec（admin:212）声明属实；FE 系列属 Gate 2 前端规则扫描范畴，代码完成后执行（同 account-auth I4 口径）。本 Gate 已执行结构级+机械项（9 节结构/Action 机械 diff/字典同源/版本记录） |
| I5 | 字典数据源偏差（无 dict/batch，前端 enums.ts 常量） | PageSpec 第 3 节（admin:50）声明为设计决策待 Gate 1 复核：面板单机、4+1 枚举全固定值、无激活栈、04-business-enum.md 为单一事实源——**本次复核接受该偏差**（与 account-auth I5 同判）；后续接入栈 profile 时按 GL-032 重审 |
| I6 | 待对齐-1（更新检查版本源）仍开放 | PRD:166 跟踪中；D3 以 ACL 适配器隔离+I-P05 双重门控落地接口（api 3.2/D-P09），源标识未锁定故 T6/T7/T8 暂不可补——开放项归属清晰（B 阶段锁定源后补 PRD T 行），无设计矛盾 |

## 栈特化条目 N/A 声明

`ACTIVE_STACKS: ""`（.claude/settings.json 实证）→ 按 validation skill「异栈激活时以该栈 rules 替换本节清单」与 account-auth 先例：CH3-002/003（统一 POST/Result\<T\>）、CH4-005a~g/007/008（审计七件套/{module}_id VARCHAR(32)）、CH4-057~070（MQ/事务边界/领域事件）、CH5-008~018（MySQL DDL 规约）、GL-029~031 等栈强绑定条目 N/A；其通用意图（契约完整性、路径/错误码/枚举一致性、SQLite 布尔 0/1 公共约定 api:147）已在对应 G1 项按 D-07 声明的通用 REST 契约检查通过。单机单进程无 MQ 与 api-account-auth.md D-05/D-07 先例一致。

## 总结

**判定：PASS**（S1~S8 全过 → 依判定规则非 FAIL 非 WARN；建议项记录在案后继续）

- G1-01~G1-11：11/11 通过（无 CRITICAL 失败）
- S1 三文档全局：BA/AA/FL 完整且 BA→AA→FL 覆盖 16/16 闭环；S2 PRD：G 5/5、T 适用 2/2、N 4/4；S3 领域模型：5 类对象分类+聚合根确认；S4 API：M1~M5 全过 + GL 适用项通过；S5 变更摘要：8 文档 changelog 齐；S6 PageSpec：9 节结构+机械项过
- S7 跨阶段一致性：机械 diff 实证 7 Action ↔ 7 接口一一对应、领域对象 D2↔API↔DDL 逐层一致；S8 枚举池：24 标识符无重复、4 枚举三方同值
- WARN 2 项（W1 R-10 口径笔误×3 处、W2 菜单统计口径）+ INFO 6 项，均为文档元数据小修，可在下一次触碰对应文件时顺手清零，无需重跑本 Gate
- B/F 阶段入场注意：以 api-panel-core.md 7 接口为验收计数口径；FX/FE 全量清单在 Gate 2 执行
