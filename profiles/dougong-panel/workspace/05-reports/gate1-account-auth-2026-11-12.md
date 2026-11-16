# Gate 1 验证报告 — 设计→代码门控

**模块**：account-auth（账户与安全基座）
**检查时间**：2026-11-12
**检查者**：门控检查员（validation skill · Gate 1；编号源：engine-rules.md G1-01~G1-11）
**产物版本**：BA/AA/FL v1.0、PRD v1.0、domain-model v1.0、API v1.0、PageSpec admin/readonly v1.0
**Profile 绑定**：dougong-panel（profile.yaml `depends: []`，ACTIVE_STACKS 为空 → 无激活栈，栈特化执行清单按通用意图检查、栈强绑定条目标 N/A）

## 检查对象

| 阶段 | 文件 |
|------|------|
| D1 | 00-architecture/business-arch-account-auth.md、application-arch-account-auth.md、function-list-account-auth.md |
| D2 | 02-detailed/prd/prd-account-auth.md、prd/domain-model.md |
| D3 | 02-detailed/api/api-account-auth.md |
| D4 | 02-detailed/interaction/admin/account-auth/account-auth.md、interaction/readonly/account-auth/account-auth.md |
| 参考 | 01-solution/03-common-enums.md（draft）、01-solution/04-business-enum.md（8 条定稿）、workspace/INDEX.md |

## G1-01~G1-11 逐项结论

| 编号 | 检查项 | 结果 | 依据（本次实读/实跑） |
|------|--------|------|----------------------|
| G1-01 | 架构文档与模块设计一致 | **PASS** | AA 树（application-arch:416-457）= BA 树（business-arch:220-260），均 8 L3/25 L4；FL 更新记录 25 子模块=19 交互菜单+12 机制型（function-list:347），AA 附录A 机制型映射表恰 12 行（application-arch:465-478）；PRD IN 6 项（prd:19-28）逐一对应 8 功能模块；FR-01~21 按 C-01~S-05 组织（prd:55-109） |
| G1-02 | PRD 覆盖所有业务场景和角色 | **PASS** | 3 角色定义含职责与获得方式（prd:41-49）；25 个 L4 逐一对到 FR-01~21（C-01-01→FR-01、C-01-02→FR-02、C-01-03→FR-01/02、C-01-04→FR-03、C-02-01~04→FR-04、C-03-01~03→FR-05/06/07+08、S-01-01~03→FR-09/10/11、S-02-01~02→FR-12/13、S-03-01~03→FR-14/15/16、S-04-01~03→FR-17/18/17、S-05-01~03→FR-19/20/21）；异常场景 10 条（prd:158-171）；FL 19 菜单计数经 grep 核对与 PRD:53 口径一致 |
| G1-03 | API 设计完整（路径/请求/响应/错误码） | **PASS**（附 W1/W2） | 实跑 `grep -cE '^\| /auth/' api-account-auth.md` = **18**（8 user + 6 session + 2 audit-log + 2 init，行 244-251/305-310/371-372/420-421），接口详情 bullet 同为 18；每个接口有 req/data/校验/审计说明；错误码表 14 行 AUTH_001~014 + OK = **15**（api:149-163），覆盖 PRD N2 全部 10 个异常分支（锁定 AUTH_004、并发初始化 AUTH_009、TOTP AUTH_006、CSRF AUTH_003、过期 AUTH_002、最后管理员 AUTH_010、越界 AUTH_008、自删 AUTH_014、审计同事务失败=D-05、WS=VerifyWs）；4 个 OHS 服务契约（api:444-449）+ 5 表 DDL（api:468-527）齐备 |
| G1-04 | 交互设计覆盖所有页面类型 | **PASS** | 列表页 3（user-list/session-list/audit-log-list，admin:81-168）；表单页 3（init-wizard/login/profile-self，admin:172-227）；详情页=行内弹窗承载、无独立路由有显式声明（admin:231）；审批页 N/A 有据（本域无审批语义，prd:142）；readonly 负空间 5 Action 与 ADR-018 矩阵对应（readonly:35）；路由 7 条（admin:237-243） |
| G1-05 | API 字段与交互规格字段对齐 | **PASS**（附 W3/W4） | 逐字段核对：userList items 7 字段（api:262）↔ 列表列（admin:92-97）；sessionList items 6 字段（api:317）↔ 列（admin:128-133）；audit query 6 筛选+分页（api:376）↔ 查询条件（admin:146-150）；audit items 10 字段（api:376）↔ 列（admin:156-164）；userCreate/reset-password/password-change/totp-setup/totp-enable/totp-disable req ↔ 表单字段（admin:111-115/106/212-216/224）；AUTH_004 remainingSeconds、AUTH_007 loginTicket（api:153/156）↔ 登录交互流（admin:203）；CSV 列（api:377）↔ 导出（admin:168）——均一致 |
| G1-06 | 状态机定义完整 | **PASS** | 3 组状态机状态/流转/触发齐全（prd:133-141）：panel_user active⇄locked（5 次失败/期满自动）、session active→expired/revoked（12h/2h 到期、登出/强下线/级联，含 end_reason）、init uninitialized→initialized 单向；与 D3 状态模型一致（api:234/293-295/408-410）；T3 审批 N/A 有依据 |
| G1-07 | 枚举统一且无冲突 | **PASS**（附 I1/I2） | 实跑 `grep -cE '\*\*定稿\*\*' 04-business-enum.md` = **8**；API 3.4 的 8 枚举（api:165-176）取值与池逐条一致（04:18-28）；DDL CHECK 约束与枚举一致（api:473/474/497/508/513/514/524）；PageSpec 字典 Key 8 个同源（admin:51-60）；无重复定义 |
| G1-08 | 接口路径符合 /domain/resource/action | **PASS** | 18 接口全部 `/auth/{resource}/{action}` 三段式（api:99 声明；grep 明细见 G1-03）；资源单数 user/session/audit-log/init |
| G1-09 | 设计文档 changelog 已填写 | **PASS** | 7 份文档均有版本属性表+更新记录 v1.0：business-arch:303-307、application-arch:498-502、prd:233-237、domain-model:76-80、api:531-553、PageSpec admin:266-272、readonly:85-91 |
| G1-10 | 状态索引模块状态已更新 | **PASS** | INDEX.md:23-34 account-auth requirement/api/interaction 均 done 且 note 详实（含产物路径与口径）；gate1 pending 由本次检查落盘为 warn |
| G1-11 | 跨阶段模型一致性 | **PASS**（附 W1） | D2 4 聚合根（domain-model:36-44）↔ API 1.4 总览 4 聚合根（api:42-47）；5 域内对象/值对象 ↔ API（TOTP 凭证→panel_user_totp、锁定状态→内嵌字段组 D-04、有效期策略→3.4 配置、变更快照→detail JSON、保留策略→audit.retention_days）；子流程 I/O 契约 ↔ 4.3.3 审计清理（domain-model:52 ↔ api:359-365）；存储映射 5 表（domain-model:74）↔ 5.3 DDL 5 表；D2 OHS（domain-model:66-72）↔ API 5.1 四服务；PageSpec 18 Action ↔ API 18 接口**一一对应无虚构无遗漏**（逐条比对 Action 表 admin:28-45 与 4 张接口表） |

## 执行展开清单（validation skill Gate 1 三大块）

### 1. PRD 就绪检查

| 层级 | 通过/总数 | 状态 |
|------|----------|------|
| 第一层 G1-G5 | 5/5 | PASS（prd:188-196 自查与实读一致：目标/范围/角色/领域对象/对象关系均落盘） |
| 第二层 T1-T9 | 适用 5/5，N/A 5 项有据 | PASS（T2 状态流转 prd:131-141；T4 prd:146-148；T5 prd:150；T3/T6/T7/T8/T9 N/A 依据见 prd:205-211） |
| 第三层 N1-N4 | 4/4（N1 部分锁定已声明） | INFO（见 I3） |

### 2. API 文档完整性

| 章节 | 通过/总数 | 状态 | 说明 |
|------|----------|------|------|
| 第1章 模块设计概述 | 12/12 | PASS | CH1-001~012 对应 1.1-1.5（背景三要素 api:15、边界做/不做 api:21-22、决策表 10 条 api:28-39、领域对象总览 api:42-47、上下文映射无技术实现词汇混入、被依赖方向归属正确 api:51-55） |
| 第2章 跨聚合根设计 | 4/4 | PASS | 关系图 api:61-71、不变量 I-01~I-06 api:77-82、编排 api:87-91；CH2-004（MQ 保证）N/A——无激活栈且 D-05 声明单体进程内同事务，一致性由 I-05 保证，非 MQ 场景适用 |
| 第3章 应用/基础设施层 | 适用通过 | PASS | 路径/方法/响应/认证/CSRF/中间件链/权限矩阵/错误码/枚举/配置均有（api:97-187）；CH3-002 统一 POST、CH3-003 Result\<T\>、CH3-013/014 枚举池来源池标注为 java 栈强绑定条目——无激活栈下按 D-07 声明的通用 REST 契约检查通过（读 GET/写 POST 已声明）；枚举来源池标注实际存在（api:165 "均在 04-business-enum.md 定义"） |
| 第4章 聚合根详细设计 | 适用通过 | PASS | 4 聚合根均有表结构/不变量/状态模型/接口设计/仓储契约（api:191-438）；CH4-005a~g、CH4-007/008（{module}_id VARCHAR(32)）、CH4-057~070（MQ/事务边界）、CH4-066/067（领域事件/MQ）为 java 栈强绑定条目 N/A；CH4-033~041 接口三层模型按本模块实际（管理接口为主、服务接口为 OHS 4.3.4/4.2.5/4.4.4）覆盖 |
| 第5章 模块清单汇总 | 适用通过 | PASS | 对外服务 4 项（api:444-449）、SQL 清单 10 条（api:453-464）、DDL 5 表（api:468-527）；CH5-008~018 为 MySQL 栈强绑定（utf8mb4/InnoDB/DECIMAL(20,6)/_ext/_tag 等表）N/A（SQLite，公共布尔约定 0/1 已遵守 api:484） |
| 全局一致性 GL | 适用通过 | PASS（附 W2） | GL-001a 文档头 2026-11-12=更新记录日期 ✓（7 份同）；GL-009/010/011 按无栈声明口径 ✓；GL-021/022/026/027 API 引用枚举均在池且有来源标注 ✓；GL-029~031（withXxx 开关/options 接口）N/A——本模块无主数据 options 语义场景、用户列表全量返回已声明（api:262）；GL-032 dict/batch 独立端点禁令——PageSpec 已声明偏差（见 I5，本次复核接受） |

### 3. 前后端契约一致性（CT）

| 编号 | 检查项 | 状态 |
|------|--------|------|
| CT-001 | API 路径与 PageSpec Action 路径对齐 | PASS（18 Action 路径逐一比对一致） |
| CT-002 | 请求/响应字段与页面字段对应 | PASS（见 G1-05 明细） |
| CT-003 | 枚举值前后端一致 | PASS（8 字典 Key=池=API 三方同值） |
| CT-004 | Action 表所有 Action 在 API 有对应接口 | PASS（18/18，无虚构） |
| CT-005 | 列表页列字段来源于 API ListVO | PASS（3 列表页无臆造字段） |
| CT-006 | 表单页字段来源于 API SaveAO | PASS（表单字段均有 req 依据） |
| CT-007 | 字典 Key 与后端枚举对应 | PASS（admin:51-60 ↔ 04-business-enum.md:18-28） |
| CT-008 | 分步表单 generate-id/save 接口 | N/A（本模块无分步表单，PageSpec 未声明分步场景） |

## WARN 项（建议修复，不阻断）

| # | 项 | 位置 | 说明 |
|---|-----|------|------|
| W1 | 接口/错误码计数口径不一致 | api-account-auth.md:535、interaction/admin/account-auth/account-auth.md:47 与 :255、INDEX.md:27 | 变更记录与自查看板声称"**16** 个 HTTP 接口/16 错误码/18 Action 对 16 接口"；本次实跑 `grep -cE '^\| /auth/'` = **18** 接口、`grep -cE '^\| AUTH_[0-9]+'` = **14**（+OK=15）。映射本体 18 Action↔18 接口一一对应、无虚构无遗漏（G1-11 实证），仅计数元数据错误，会误导 B/F 阶段验收口径。建议：三处"16"改为 18 接口/15 错误码 |
| W2 | D-10 会话有效期策略表未落 DDL | api-account-auth.md:38、:180-187 | D-10 与 3.4 配置项声明"会话有效期配置由本域自持（**会话有效期策略表单行配置**）"，但 5.3 DDL（api:468-527）无该表、5.2 SQL 清单（api:453-464）无对应 SQL——配置载体（表单行 vs 进程内常量/配置文件）未落地。建议：补 1 张单行配置表 DDL+SQL 或改声明为非表载体 |
| W3 | 审计"对象标识"筛选维度缺失 | prd-account-auth.md:146 ↔ api-account-auth.md:376 | PRD 第七部分 T4 筛选维度含"对象类型/**对象标识**"，API list query 仅有 objectType 无 objectId（响应含 objectId；PageSpec 查询区同样只到 objectType，前端与 API 自洽）。建议：API 补 objectId 筛选参数或 PRD 收窄该维度 |
| W4 | initWizard 动态码字段命名不一致 | interaction/admin/account-auth/account-auth.md:184 ↔ api-account-auth.md:426 | PageSpec 写提交 `totpSecret+code`，API req 定义为 `totpSecret?, totpCode?`。建议：统一为 `totpCode`（以 API 为唯一数据源，符合 PageSpec 自身"唯一数据源"声明） |

## INFO 项（观察记录，无需整改）

| # | 项 | 说明 |
|---|-----|------|
| I1 | domain-model.md:37/38 的 totp_enabled{on,off}/lock_status{none,locked} 未进枚举池 | D2 已备注"枚举以 04-business-enum.md 定稿为准（D3 对齐）"（domain-model:36）；实际实现=布尔 0/1（公共约定，api:484）+ panel_user_status + fail_count/locked_until 内嵌（D-04），演化已声明、无冲突 |
| I2 | 03-common-enums.md 7 条仍为 draft | D3 定稿了业务池 8 条但未复核通用池：分页约定 page_size（03:13）vs API pageSize（api:376）命名差异；"操作结果 success\|failure"（03:10）与 audit_result 值域重叠。draft 状态本身声明待核对，属待办非冲突；建议 D3 补一轮通用池定稿 |
| I3 | PRD N1 未锁定项（QPS/容量/查询响应时间） | prd:156 已声明沿 knowledge 惯例不在设计阶段虚构、D3/B 阶段实测补齐；N1 为加分项，不阻塞 |
| I4 | FX-001~141 前端质量清单未随 payload 发布 | 本次实跑 find 验证：payload 下仅有 profiles/java-microservice/skills/backend-specs/15-validation-checklist.md，无前端 FX/FE 通用清单文件——PageSpec（admin:272）声明属实。FE-001~141 属 validation skill Gate 2 前端规则扫描范畴，代码完成后执行，不在本 Gate 范围 |
| I5 | 字典数据源偏差（无 dict/batch，前端 enums.ts 常量） | PageSpec 第 3 节（admin:62）声明为设计决策待 Gate 1 复核：面板单机、8 枚举全固定值、无激活栈 profile、04-business-enum.md 定稿为单一事实源——**本次复核接受该偏差**；若后续接入 vue3 栈 profile 需按 GL-032 重审 |

## 栈特化条目 N/A 声明

CH3-002/CH3-003、CH4-005a~g/CH4-007/008/CH4-057~070、CH5-008~018、GL-032 等 java/vue 栈强绑定执行条目，因 profile.yaml `depends: []`（ACTIVE_STACKS 为空）按 validation skill"异栈激活时以该栈 rules 替换本节清单"原则标 N/A；其通用意图（契约完整性、路径/错误码/枚举一致性）已在对应 G1 项按 D-07 声明的通用 REST 契约与 SQLite 公共约定检查通过。

## 总结

**判定：WARN**（CRITICAL 全部通过，4 项 WARN 建议待用户确认后继续；确认记录见上表）

- G1-01~G1-11：11/11 通过（其中 G1-03/G1-05/G1-07/G1-11 带非阻断附注）
- PRD 就绪：G 5/5、T 适用 5/5、N 4/4
- API 完整性：各章适用项通过；无 CRITICAL 失败
- 契约一致性：CT-001~007 PASS，CT-008 N/A
- WARN 4 项（W1 计数口径、W2 策略表未落 DDL、W3 objectId 筛选缺失、W4 totpCode 命名）、INFO 5 项
- 建议：W1~W4 可在 D3/PageSpec 一次小修订中清零，修订后无需重跑全量 Gate 1（属元数据/单字段修订，不改变接口面与模型面）
