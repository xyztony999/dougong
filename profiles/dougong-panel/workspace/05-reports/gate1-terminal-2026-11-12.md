# Gate 1 验证报告 — 设计→代码门控

**模块**：terminal（Web 终端）
**检查时间**：2026-11-14（报告文件名沿用流水线指定期 `gate1-terminal-2026-11-12.md`）
**检查者**：门控检查员·terminal（validation skill · Gate 1；检查项编号唯一来源：engine-rules.md G1-01~G1-11；流程预检 S1~S8 见 design skill）
**产物版本**：BA/AA/FL v1.0、PRD v1.0、domain-model v1.11（terminal 域段落，2026-11-14 追加）、API v1.0、PageSpec admin/readonly v1.0、枚举池（terminal 2 条定稿）
**Profile 绑定**：dougong-panel（`.claude/settings.json:3` 实证 `ACTIVE_PROFILE: dougong-panel`；`ACTIVE_STACKS: "go-vue3"` 但 `profiles/` 目录 ls 实证仅含 `dougong-panel/`——无 go-vue3 栈规则文件可叠加，见 I2；本 Gate 实际可执行层 = 引擎机制 + 业务规则，栈强绑定条目按六模块先例口径 N/A，见「栈特化条目 N/A 声明」）

## 检查对象（均本次实读）

| 阶段 | 文件 |
|------|------|
| D1 | 00-architecture/business-arch-terminal.md、application-arch-terminal.md、function-list-terminal.md |
| D2 | 02-detailed/prd/prd-terminal.md、prd/domain-model.md（terminal 段落：本域判定 :105-119 / 业务域行 :132 / 对象行 :168-170 / 注释 :179 / 子流程注 :195 / 关联注 :210 / 集成行 :245-249 / 存储映射 :251+:259 / v1.11 更新行 :276） |
| D3 | 02-detailed/api/api-terminal.md |
| D4 | 02-detailed/interaction/admin/terminal/terminal.md、interaction/readonly/terminal/terminal.md |
| 参考 | 01-solution/04-business-enum.md（39 条，terminal 定稿 2 条 :47-48）、workspace/INDEX.md、engine-rules.md、validation SKILL.md、gate1-{monitoring,service-mgmt,site-mgmt,database-mgmt}-2026-11-12.md（零持久化/负空间同构先例口径） |

## S1~S8 阶段汇总预检结论（判定主表）

| # | 预检项 | 规则对应 | 结果 | 关键证据（实读/实跑） |
|---|--------|---------|------|------|
| S1 | 三文档全局检查 | G1-01 | **PASS** | BA 层级完整：3 L2 / 5 L3 / 11 L4（business-arch:62-136），第三部分树 11 L4 与第二部分逐条对应（:142-163），附录A L3 依赖 5 行与 AA 调用关系同构（:171-177 ↔ application-arch:79-108/:199-227）；AA 2 应用域（核心 C-11 + 支撑 S-38~S-41）11 子模块均带功能说明+用户类型+调用关系（application-arch:63-228），树=正文（:234-258），附录A 机制型映射恰 6 行（:266-273：S-38-01/S-38-02/C-11-01/02/03/S-41-02），附录B 跨模块契约 4 行+OUT 边界 5 行（:277-294）；FL 4 菜单行覆盖 5 个 UI 型子模块（S-40-02 输入传递经「Web 终端」宿主菜单功能点覆盖，FL:80）+ 6 机制型经全局行为段（FL:40）与 AA 附录A，11/11 闭环（FL:153 自述与实点一致）；readonly 章显式登记整域禁用（FL:94-96），第三部分视图=第二部分（:104-125）；**模块编码连续性机械实证**：`grep -oE '（[CS]-[0-9]+）'` 各 AA 文档——account-auth C-01~03/S-01~05、panel-core S-06~12、monitoring S-13~16、service-mgmt C-04~05/S-17~18、file-manager C-06/S-19~25、site-mgmt C-07~10/S-26~30、database-mgmt S-31~37、terminal C-11/S-38~41（AA 中出现的 C-03/S-03/S-04 为对 account-auth 的跨模块引用非本模块编码）——C 序列 01~11、S 序列 01~41 全连续无冲突，「域内最大序号后递增」成立；ADR-023（并发 5/空闲 30min/硬上限 8h）、R-18、ADR-018、D1 三项落定口径（指定用户范围/空闲从严/多标签归 D4）在 BA:21/:76/:88/:112、AA:47-48/:116/:161、FL:13/:40/:68/:80、PRD:99-108 四层同口径 |
| S2 | PRD 完备性检查 | G1-02 | **PASS** | G 5/5（目标 prd:11-15、IN 9 项 :19-31 / OUT 8 项 :33-44、2 角色+获得方式+访客不涉及+运行身份≠操作者辨析 :48-53、领域对象/关系指向 domain-model :226-227）；T 适用 4/4：T1 七类字段（:233 ↔ domain-model:168）、T1a 终模块无对外契约+消费方视角三条链（:150-154）、T2 单对象状态机 5 流转行+6 项辨析防伪状态机（:115-136）、T9 各 FR I/O（第四部分，帧级协议入待对齐-1 语义已锁定 :242）；T3/T4/T5/T6/T7/T8 六项 N/A 均有据（:236-241：无审批/无台账无导出/本机组件非外部对接/无幂等面）；N 4/4（N2 11 个异常分支 :162-174，含审计失败拒建的从严口径）；FR-01~FR-11 ↔ AA 11 子模块一一映射（:57 注明 + 第四部分五表逐行 S-38-01→FR-01 … S-41-02→FR-11）；PRD 附录自查（:219-262）与本次独立复验一致 |
| S3 | 领域模型分离 | G1-06 / G1-11 | **PASS** | 5 类判定落盘：terminal_session → 聚合根 A1✓/A2△/A3✓/A4✓ 逐条论证（domain-model:117，一致性边界=会话记录↔PTY 进程↔并发配额同生共死；**内存态不落库**=全项目唯一「聚合根+零持久化」组合，聚合边界不由存储形态决定）；运行身份/限流策略 → 值对象（:118-119 ↔ 对象行 :169-170）；无子流程（:195）、无关联领域对象（:210，system_user 为系统实况只读引用、鉴权会话引用为运行时凭据非聚合 ID）均注明理由；过程数据 6 类归类不入清单（prd:192-201 P2~P5 ↔ domain-model:179 注释）；战略判定=支撑域含完整依据（domain-model:112）；存储映射 terminal_session→无表（:251/:259）↔ PRD:109 ↔ API 1.2/5.2/5.3 零持久化三处同口径（api:23/:268/:272）；G1-06 状态机三要素齐：活跃→断开（销毁）×5 业务触发+断开原因枚举（prd:117-123）+ 6 辨析（:125-134）↔ API 4.1.2 六触发表含判定方（api:185-194，protocol_error 为 D3 增补接口层防御，双侧声明非业务触发） |
| S4 | API 全局模板合规（M1~M5 + GL） | G1-03 / G1-08 | **PASS** | M1 章节结构：1.1~1.5 全在（api:11-55）、第 2 章单聚合根裁剪显式声明（:55「不适用——本模块单聚合根（裁剪矩阵第 2 章），对象级约束以不变量 I-01~I-07 列于 4.1.1」）、3.1~3.4（:59-143）、4.1.1~4.1.5 完整子结构（:151-251）、5.1~5.3（:253-272）、6.1~6.3（:274-299）；M2 **实跑** `grep -oE '(GET\|POST\|WS) /terminal/[a-z]+/[a-z]+'` = 仅 `GET /terminal/user/options`（×4 处声明）与 `WS /terminal/session/connect`（×3 处），**2/2 三段式** `/terminal/{resource}/{action}`（资源单数声明 :63），无其他路径形态（`/terminal` 裸前缀仅为路由组挂载声明 :52/:80）；M3 API 引用 4 枚举（terminal_end_reason/terminal_identity_source :117-118 + panel_role :119 + audit_action/audit_result/audit_source :120）均在池且定稿（04:47-48/:18/:22-24）；M4 **实跑**占位符 grep（`{按需填写}\|TODO\|FIXME\|TBD\|{待填写}\|待补充`）API+PRD+domain-model+两 PageSpec+BA/AA/FL 八文件 **exit=1 全 clean**；M5 6.1 存在含 v1.0 `initial` 行 + `enum_value_added` 行（api:280-281）；GL-001a **实跑**：8 文件头部「最后更新 2026-11-14」=各自 changelog 最新条目日期 ✓；GL-001b/002（第 2 章/4.1.3 子流程/5.2 SQL/5.3 DDL/配置项均注明不适用或「无」并给出零持久化依据）；GL-003 D-08 混合模式声明（:36）；GL-004 领域对象总览 1 对象 ↔ 4.1 一节；GL-005/006 无子流程/无关联领域对象三处同口径（1.4 注 :43 ↔ 4.1.3 :200 ↔ domain-model:195/:210）；GL-007 3.3 端点矩阵（admin ✓/readonly ✗）↔ 4.1.4 权限列全 admin 一致（:88-92 ↔ :204-207）；GL-008 无 MQ/领域事件（:222）↔ 3.2 依赖一致；GL-009 通过（M2）；GL-010/011 按项目声明契约口径（3.1 :65：读=GET、WS=GET+Upgrade、`{"code",message,data}` 包装——六模块先例 I4 同判，栈特化统一 POST/Result\<T\> 条目 N/A，见 I4）；GL-012 WS 写通道权限校验=握手守卫链 I-01（:173）；GL-021/022/026/027/028 枚举引用标注来源池（:113「均在 workspace/01-solution/04-business-enum.md 定义」）且池无重复（见 S8）；GL-029/030/031 N/A（无子表懒加载、内存态会话变体无 CRUD/options 语义，4.1 开注 :147-149 已声明）；GL-032 API 侧通过（实跑路径 grep 无 dict 端点；前端 enums.ts 偏差声明见 I3） |
| S5 | API 变更摘要 | G1-09 | **PASS** | api 6.1 两行（v1.0 initial + terminal_end_reason 增补 protocol_error 的 `enum_value_added` 行含目标节号 3.4/4.1.2，:280-281）、6.2 变更类型词汇表、6.3 语义化版本规则齐（:283-299）；**实跑** changelog 存在性 grep：8 份设计文档全部含更新记录/变更记录节（BA:195-199、AA:296-300、FL:149-153、PRD:264-268、domain-model:261-276 累积 v1.0~v1.11 terminal 追加于 v1.11、api 6.1、admin 第 9 节 :163-167、readonly 第 9 节 :71-75）；API 6.1 摘要与正文逐项点数一致（2 端点/帧 5 类型/3 网关契约/7+4 错误码/4 枚举引用/D-01~D-09 九决策，全部与正文实点相符） |
| S6 | PageSpec 输出校验 | G1-04 / G1-05 | **PASS** | 两份 PageSpec 9 节结构齐（admin:13-169、readonly:1-77）；模块元信息含唯一数据源=api-terminal.md v1.0（admin:9、readonly:9）、业务类型「工具类·会话通道」诚实标注三型之外并给出 D2/D3 判定依据（admin:18）、页面类型 code 判定附第三方组件深度集成触发条件（admin:19）；Action 表 2 行含方法/路径/页面/权限/CSRF 列（admin:28-29，**sed 原文提取实证**）+ 刷新纪律表（:35-42：开框即拉不缓存/无轮询/卸载尽力 close）；字典 Key 3 条同源池（:48-50）+ 断开原因颜色文案 6 值全映射（:56-63）+ dict/batch 偏差声明（:52，见 I3）；终端页三区布局/交互通道行为/标签操作（:73-96，多标签上限=并发阈值 5 有 D1 落定依据）；新建会话对话框 2 字段 Dialog 判定（<5 规则，:100-105）+ 逐级反馈表按 api 4.1.4 握手顺序落法（:111-119）；断开反馈 closed 帧 6 原因分级（重连/重登/关闭，:127-133）；路由 1 条 requiresAdmin + 菜单仅 admin 下发（:139-144）；第 8 节 12 项全局一致性自查逐项有证据指向（:148-161）；readonly 零可达整域禁用模式显式声明区别于先例（readonly:10）+ 负空间 2 行与 api 3.3 矩阵 ✗ 项逐项对应（readonly:32-33 ↔ api:90-91，sed 原文提取实证）；列表/表单（无保存语义）/详情/分步的 N/A 判定逐项有据（admin:69/:107/:123）；FX 清单未随 0.5.2 payload 发布经本次 `find` 实证（payload 全目录无 `*FX*`/`*frontend*check*` 命中，admin:169/readonly:77 注记属实，见 I5） |
| S7 | 跨阶段一致性 | G1-11 | **WARN**（1 项字段清单偏差，见 W1） | **通过部分（机械实证）**：领域对象主体一致——D2 terminal_session 聚合根/内存态/枚举字段（domain-model:168）↔ API 1.4 总览（api:43 同分类同承载）↔ 4.1 运行时字段清单（:157-167）↔ 存储映射无表（domain-model:259 ↔ api:268/:272）逐层一致；**Action 机械比对（原文提取）**：API 2 端点（3.3 矩阵 :90-91 = 4.1.4 清单 :206-207 = 5.1 清单 :259-260 三处同数）== admin PageSpec 2 Action（userOptions→GET /terminal/user/options、sessionConnect→WS（GET+Upgrade）/terminal/session/connect，admin:28-29），方法+路径逐行相同，无未覆盖、无虚构；readonly 0 Action + 负空间 2 行 = api 3.3 ✗ 项全集（readonly:32-33），无虚构接口；**待对齐闭环双侧同记**：PRD 待对齐-1~-6（prd:207-212）↔ API D-01~D-06 决策行各含「关闭 prd-terminal.md 待对齐-x」（api:29-34，grep 实证 7 处）；帧契约字段对齐：ready 帧 {sessionId, identity, limits} ↔ 状态条/标签（api:212 ↔ admin:75-77）、closed 帧 {reason, closedAt, durationSeconds} ↔ 断开反馈（api:214 ↔ admin:127-133）、userOptions items[{name,uid,shell}] ↔ Select 选项（api:211 ↔ admin:105）；错误码 TERM_001~007+AUTH_001/002/003/008 ↔ PageSpec 第 1 节逐码分流（api:99-111 ↔ admin:22）；**偏差（W1）**：api 4.1.1 运行时字段清单 9 项缺「鉴权会话引用」——domain-model:168 核心业务字段列与 BA:185 术语表均含该字段（「鉴权会话引用为运行时校验凭据与联动断开触发源」），且 API 自身 D-07（30s 存续期复核，api:35）与 4.1.5 出站契约 `SessionAuthService.VerifyWs(rawToken)`（:251）要求按会话持有面板会话凭据方可复核，但字段清单（sessionId/userId/identity/createdAt/lastActiveAt/sourceIp/userAgent/pty/wsConn/outBuf）无承载字段，api 1.4 注（:43）仅以「非聚合 ID」口径提及未落为运行时字段——判 D2↔D3 字段级漂移（次要差异：domain-model「空闲截止/硬上限截止（推导的运行时字段）」经 API createdAt+8h（:162 hardDeadlineAt）与 lastActiveAt+30min（I-03 :175）语义闭合，sessionId/userId 为 D3 合法技术细化，均判一致） |
| S8 | 枚举池无冲突 | G1-07 | **PASS** | **实跑**池标识符提取 `sort \| uniq -c` 重复检查 = 无重复输出（**39 条**全唯一，较 database-mgmt 轮 37 条 + terminal 2 条吻合）；terminal_end_reason **三方同值**（6 值）：04:47 `self_close\|idle_timeout\|hard_limit\|panel_session_invalid\|network_error\|protocol_error` ↔ api:117 ↔ admin:48，protocol_error 增补在池行（「D3 2026-11-14 增补 protocol_error」+I-06/D-01 依据）与 API 3.4（「D3 增补，非 PRD 第六部分 5 类业务触发」）双侧同记、PageSpec 含该值文案（admin:63）；terminal_identity_source **三方同值**（2 值）：04:48 `panel_run_user\|specified_system_user` ↔ api:118 ↔ admin:49；panel_role 引用与池一致（04:18 ↔ api:119 ↔ admin:50）；audit_action/audit_result/audit_source 引用 account-auth 定稿（04:22-24 ↔ api:120）；与 account-auth session_end_reason 同构不冲突（标识符不同、取值域不重叠）；API 3.4 自述「无新增枚举」（:113，仅 D3 增补 1 取值）与池状态一致；PRD「2 个枚举定稿入池」（prd:268）与池 2 行「定稿（D2 2026-11-14 新增）」一致 |

**判定规则回执**：S1~S6 全过 + S8 过 → 非 FAIL；S7 有未通过项（W1）→ **总体判定 = WARN**（1 项 WARN 级文档修正 + 6 项 INFO 观察记录在案；无 CRITICAL 失败，不阻断，W1 修复后无需重跑全量 Gate）。

## G1-01~G1-11 逐项结论

| 编号 | 检查项 | 结果 | 依据（本次实读/实跑） |
|------|--------|------|----------------------|
| G1-01 | 架构文档与模块设计一致 | **PASS** | AA 1.1 承接 BA v1.0 并按 L3→DDD 归类（application-arch:13-22）；PRD 输入列明 D0+D1 三文档（prd:8）；API 输入列明 PRD v1.0+domain-model v1.11+D1（api:8）；FR 编号按 AA 11 子模块组织（prd:57）；PageSpec 唯一数据源=api v1.0（admin:9/readonly:9）；R-18/R-02/ADR-018/ADR-023/R-08/R-14 口径数值六文档一致（证据见 S1/S2 行） |
| G1-02 | PRD 覆盖所有业务场景和角色 | **PASS** | 2 角色+职责+获得方式+访客不涉及+运行身份辨析（prd:48-53）；11 L4 → FR-01~11 一一映射（prd:59-93）；异常场景 11 条覆盖握手拒绝/角色拒绝/并发上限/身份校验/PTY 失败/会话失效联动/断连/僵死进程/输出洪峰/重连/审计失败从严（prd:162-174）；readonly 整域禁用与 FL 附录A 对照一致（FL:129-137 ↔ prd:51） |
| G1-03 | API 设计完整（路径/请求/响应/错误码） | **PASS** | 接口计数三处同数=2（3.3/:88-92、4.1.4/:204-207、5.1/:259-260）；每端点含握手校验顺序①~⑨/query/响应 data 形状/错误码/审计动作（:209-214）；WS 帧协议 5 类型含方向与载荷（D-01 :29 + 3.4 常量表 :126-128）；错误码 TERM_001~007 七个+复用 AUTH_001/002/003/008（:99-111）覆盖 PRD N2 接口侧可表达项（并发→TERM_001、用户不存在→TERM_002、无登录 shell→TERM_003、PTY 失败→TERM_004、参数→TERM_005、审计不可用→TERM_006、清单不可用→TERM_007、越界→AUTH_008）；零持久化替代契约=3 进程内网关/注册表（TerminalSessionRegistry/PtyGateway/SysUserDirectory，:224-249）+ 5.2/5.3 无 SQL/DDL 声明 |
| G1-04 | 交互设计覆盖所有页面类型 | **PASS** | 列表页 N/A 有据（admin:69：api 无集合查询语义、PRD OUT-5）；表单=新建会话 Dialog 2 字段（admin:98-107，无保存语义声明与 D-08 一致）；详情 N/A=帧反馈等价承载（admin:121-123，ready/closed 帧）；审批 N/A 有据（prd:136）；readonly 零可达整域禁用+负空间（readonly:10/:28-35）；页面类型 code 判定附据（admin:19） |
| G1-05 | API 字段与交互规格字段对齐 | **PASS** | ready 帧 {sessionId, identity{source,runUser}, limits{maxConcurrent:5, idleTimeoutSeconds:1800, hardLimitSeconds:28800}} ↔ 标签标题 runUser/状态条 sessionId 前 8 位+上限文案（api:212 ↔ admin:75-77）；input/resize 帧 ↔ xterm onData/onResize（api:213 ↔ admin:83-84）；output truncated 标记 ↔ 终端内提示行（api D-04 ↔ admin:85）；closed 帧 reason ↔ 6 值反馈分级（api:214 ↔ admin:127-133）；userOptions {name,uid,shell,generatedAt} ↔ Select label=name 辅显 uid/shell（api:211 ↔ admin:105）；WS query {identitySource, username?} ↔ 对话框 radio+select（api:212 ↔ admin:104-105）；错误码文案逐码对应（api:99-111 ↔ admin:22）——无臆造字段 |
| G1-06 | 状态机定义完整（状态/流转/触发条件） | **PASS** | 单对象状态机：活跃→断开（终态=销毁）三要素齐——状态 2、流转 5 行（PRD :119-123）各含当前态/目标态/触发/terminal_end_reason/备注、触发条件与判定方（API :187-194 六行含 protocol_error）；「断开即销毁无 closed 持久态」定位与 6 项辨析防伪状态机（PRD :125-134 ↔ API :196 辨析①~④）；无审批声明（PRD :136） |
| G1-07 | 枚举统一且无冲突 | **PASS** | 实跑：池 39 标识符无重复；2 本域枚举池=API=PageSpec 三方逐字同值（04:47-48 ↔ api:117-118 ↔ admin:48-49）；protocol_error 增补双侧声明闭环；frame_type 5 值与 9 项业务常量明确标注「非枚举池条目」（api:122-141）且 PageSpec 未误登记为字典 |
| G1-08 | 接口路径符合 /domain/resource/action | **PASS** | 2/2 端点三段式（grep 实证：/terminal/user/options、/terminal/session/connect；3.1 :63 声明三段式+资源单数） |
| G1-09 | 设计文档 changelog 已填写 | **PASS** | 8 份文档均有版本属性表+更新记录（grep 实证明细见 S5）；API 6.1 使用词汇表内 `initial`/`enum_value_added` 类型；domain-model 累积 v1.0~v1.11 且 v1.11 追加行自述（1 业务域/1 聚合根/2 值对象/2 枚举/零持久化）与实际产物一致（:276） |
| G1-10 | 状态索引模块状态已更新 | **PASS** | INDEX.md:107-118 terminal requirement/api/interaction 均 done 且 note 含产物路径与计数口径（「2 端点（1 HTTP+1 WS）」「S7 2/2 Action 映射 + S8 枚举成员校验通过」与本次独立复验一致；INDEX:110 note 内「聚合视图 v1.7」与实际 v1.11 不符，元数据滞后记 I1）；gates.gate1 pending 由本次检查落盘为 warn（本次动作） |
| G1-11 | 跨阶段模型一致性 | **WARN**（W1） | 通过部分：D2 聚合根+2 值对象 ↔ API 1.4 ↔ 4.1 分类一致；无子流程/无关联领域对象/无领域事件三文档同口径（domain-model:195/:210/:249、api:43/:200/:222、prd:203）；存储映射「无表」↔ API 5.2/5.3 闭环；PageSpec 2 Action ↔ API 2 端点机械比对一致（admin全等、readonly 0+2 负空间）；待对齐-1~-6 双侧闭环。偏差 W1：api 4.1.1 运行时字段清单缺「鉴权会话引用」（D2 domain-model:168 与 BA:185 均有，且 API D-07/4.1.5 的 VerifyWs 复核机制需要该字段承载）——详见 S7 行与 W1 项；不影响接口契约与页面呈现（握手鉴权经 Cookie 通道完成、复核机制语义已由 D-07 闭合，仅对象字段盘点漏列），修复成本一行 |

## 执行展开（validation skill Gate 1 三大块）

### 1. PRD 就绪检查

| 层级 | 通过/总数 | 状态 |
|------|----------|------|
| 第一层 G1-G5 | 5/5 | PASS（prd 附录自查 :221-227 与实读一致） |
| 第二层 T1-T9 | 适用 4/4（T1/T1a/T2/T9），N/A 6 项有据（T3/T4/T5/T6/T7/T8） | PASS（:229-242） |
| 第三层 N1-N4 | 4/4（N1 部分锁定已声明） | INFO（N1 未锁定 4 项经 D-01/D-03/D-04/D-06 全部关闭，双侧闭环见 S7） |

### 2. API 文档完整性（M1~M5 机械 + GL 全局）

| 章节 | 结果 | 说明 |
|------|------|------|
| M1 章节结构 | PASS | 1.1~1.5/3.1~3.4/4.1（4.1.1~4.1.5）/5.1~5.3/6.1~6.3 全在且非空；第 2 章单聚合根裁剪显式声明（:55）；4.1 开注「主数据类·内存态会话变体」判定与变体理由（:147-149） |
| M2 路径格式 | PASS | 2/2 三段式（grep 实证） |
| M3 枚举引用有定义 | PASS | 4/4 在池定稿，无重复 |
| M4 占位符残留 | PASS | grep exit=1（八文件均无 `{按需填写}`/TODO/FIXME/TBD/{待填写}/待补充） |
| M5 变更记录 | PASS | 6.1 存在含 v1.0 initial + enum_value_added 行（:280-281） |
| GL 全局一致性 | PASS | GL-001a/b、002、003、004~009、012、021~022、026~028 通过（明细见 S4 行）；GL-010/011 按项目声明契约口径（读=GET+WS Upgrade、`{"code",message,data}` 包装，六先例同判，见 I4）；GL-029/030/031 N/A（无子表懒加载、无 CRUD/options 语义）；GL-032 通过（API 无 dict 端点 grep 实证；前端常量偏差见 I3） |

### 3. 前后端契约一致性（CT）

| 编号 | 检查项 | 状态 |
|------|--------|------|
| CT-001 | API 路径与 PageSpec Action 路径对齐 | PASS（2/2 原文提取比对一致；readonly 0 Action+2 负空间=api ✗ 全集） |
| CT-002 | 请求/响应字段与页面字段对应 | PASS（G1-05 明细：ready/closed/output 帧字段、userOptions 选项、WS query 逐键对应） |
| CT-003 | 枚举值前后端一致 | PASS（2 本域枚举池=API=PageSpec 三方同值；panel_role 引用一致） |
| CT-004 | Action 表所有 Action 在 API 有对应接口 | PASS（2/2，无虚构、无遗漏） |
| CT-005 | 列表页列字段来源于 API ListVO | N/A（无列表页，诚实声明 admin:69 有据） |
| CT-006 | 表单页字段来源于 API SaveAO | PASS（等价承载：对话框 2 字段=WS query {identitySource, username?}，无臆造；无持久化表单语义 admin:107） |
| CT-007 | 字典 Key 与后端枚举对应 | PASS（admin:48-50 ↔ 04:47-48/:18） |
| CT-008 | 分步表单 generate-id/save 接口 | N/A（无分步表单，未声明分步） |

## WARN 项

| # | 项 | 说明 | 修复建议 |
|---|-----|------|---------|
| W1 | api-terminal.md 4.1.1 运行时字段清单（:157-167）缺「鉴权会话引用」字段 | G1-11/S7 跨阶段字段漂移：domain-model.md:168 terminal_session 核心业务字段列含「鉴权会话引用」（备注：「运行时校验凭据与联动断开触发源、非跨域聚合 ID 引用」），BA 附录B 术语表（business-arch:185）同将「鉴权会话引用」列为会话实例五要素之一；而 API 4.1.1 运行时字段清单 9 项（sessionId/userId/identity/createdAt/lastActiveAt/sourceIp/userAgent/pty/wsConn/outBuf）无承载字段——api 1.4 注（:43）仅以「非聚合 ID」口径提及，未落为运行时字段。且 API 自身机制依赖该字段：D-07（api:35）「每 30s 对每条会话经 SessionAuthService.VerifyWs 只读复核」与 4.1.5 出站契约 `VerifyWs(rawToken)`（:251）必须按会话持有面板会话凭据（dg_session 原始令牌或其引用）方可执行复核——字段缺失使 D-07 无数据承载。属文档级字段盘点遗漏而非机制缺口（握手鉴权经 Cookie 完成、复核语义已由 D-07 声明），不影响端点契约与页面呈现 | 下次触碰 api-terminal.md 时在 4.1.1 运行时字段清单补一行（如 `panelSessionRef | 运行时凭据 | 面板会话引用（握手 Cookie 会话的复核凭据，D-07 30s 复核与失效联动触发源；非聚合 ID，见 domain-model 第五部分）`），并在 6.1 追加 MINOR 变更行；不阻断进入 B 阶段——B 阶段实现 D-07 时自然需要该字段，届时实现者以 domain-model:168 口径为准即可 |

## INFO 项（观察记录与建议，无需重跑本 Gate）

| # | 项 | 说明 |
|---|-----|------|
| I1 | INDEX.md:110 terminal requirement note「聚合视图 v1.7」元数据滞后 | 该 note 自述 D1 时点聚合视图版本为 v1.7，但 D1 成文于 2026-11-14（此前 database-mgmt 回填后应为 v1.10），且同 note 后文 D2 已写 v1.11（与实际一致）；api:8 输入行已正确引用 v1.11。判 INDEX note 内一处版本笔误/滞后，非产物缺陷（site-mgmt 先例 I1 同类）；本次仅获授权更新 gates.gate1 行，不改此 note，建议下次触碰 INDEX 时顺手订正 |
| I2 | ACTIVE_STACKS=go-vue3 悬空绑定 | `.claude/settings.json:3-4` 实证 `ACTIVE_PROFILE: dougong-panel`、`ACTIVE_STACKS: "go-vue3"`，但 `profiles/` ls 实证仅含 `dougong-panel/`——无栈规则文件存在，栈级规则层为空集。设计产物头部按「无激活技术栈」声明成文（api:9、admin:10），与 monitoring/service-mgmt 等六模块报告 I2 同一事实。**建议运行所有者在进入 B/F 阶段前确认 ER-03 绑定一致性**（补齐 profiles/go-vue3 或回正 settings），不影响本 Gate 判定 |
| I3 | 字典数据源偏差（无 dict/batch，前端 enums.ts 常量） | admin PageSpec:52 声明为设计决策「待 Gate 1 复核」——本次复核**接受该偏差**：本模块枚举全固定值（2 本域+panel_role 复用）、无激活栈规则、04-business-enum.md 为单一事实源；API 侧 GL-032 精神已满足（grep 实证无独立字典端点）。与 account-auth/panel-core/monitoring/service-mgmt/site-mgmt/database-mgmt 六先例同判；后续接入栈 profile 时按 GL-032 重审 |
| I4 | GL-010（统一 POST）/GL-011（Result\<T\>）口径说明 | 本项目契约=读 GET/写 POST + WS 升级端点（api:65 自述 + D-09 :37 论证 WS=GET+Upgrade 的会话绑定安全性）+ `{"code",message,data}` 等价包装（:65）；与 validation skill GL-010 的 scm 栈口径不同，按无栈通用契约口径通过（六先例同判）；非偏差缺陷 |
| I5 | FX-001~141 前端质量清单未随 payload 发布 | 本次实跑 `find`（0.5.2 payload 全目录 `*FX*`/`*frontend*check*` 模式）无命中——PageSpec 注记（admin:169、readonly:77）属实。两份 PageSpec 按 9 节结构落实，可机械执行部分（Action 三方比对、字典三方同值、负空间逐项对应、第 8 节自查证据核验）本次已完成；FE/FX 命名条目属 Gate 2 前端规则扫描范畴，代码完成后执行（七模块同口径） |
| I6 | PRD 第六部分 5 业务触发 vs 池/API 6 值（protocol_error）的文档形态 | PRD 状态流转表（prd:119-123）仅列 5 类业务触发；D3 增补的 protocol_error 在池（04:47 增补标注+依据）、API（:117/:194 含判定方与 I-06 依据）、PageSpec（admin:63 红色反馈+文案）三方齐备且均声明「接口层防御路径，非 PRD 业务触发」——口径自洽、无下游冲突；PRD 未回填第 6 行属可接受的阶段文档时序（该值在 PRD 定稿后由 D3 引入）。建议 PRD 再版时补一行备注以免后来者困惑，不阻断 |

## 栈特化条目 N/A 声明

`profiles/go-vue3` 不存在（ls 实证）→ 无栈级规则文件可叠加（validation skill FR-D2 分层组装的栈层为空集）；按七模块先例：CH3-002/003（统一 POST/Result\<T\>）、CH4-005a~g/007/008（审计七件套/主键规约）、CH4-057~070（MQ/事务边界/领域事件）、CH5-004~018（DDL/SQL 规约）、GL-029~031 等栈强绑定条目 N/A——其中多数在本模块**天然无对象**（零持久化→无 DDL/SQL/审计字段/仓储表；无 MQ 单机单体→无领域事件/事务边界/MQ 消息；无 HTTP 写接口→CSRF 面在 WS 握手层由 D-09 论证），其余通用意图（契约完整性、路径/错误码/枚举一致性、`{code,message,data}` 契约、权限矩阵完备性）已在对应 G1 项按 api 3.1 声明的通用 REST/JSON+WS 契约检查通过。

## 总结

**判定：WARN**（S1~S6 全过、S8 过；S7 一项 WARN 级字段清单偏差 W1 → 依判定规则「仅 S7~S8 未过=WARN」；无 CRITICAL 失败，不阻断，W1 为一行文档修正）

- G1-01~G1-10：10/10 通过；G1-11：WARN（W1 一项）
- S1 三文档全局：BA 3 L2/5 L3/11 L4 树=正文、AA 2 域 11 子模块、FL 4 菜单行+6 机制型 11/11 闭环、readonly 整域禁用显式登记；模块编码 C-11/S-38~S-41 连续性机械实证（全局 C-01~11/S-01~41 无冲突）
- S2 PRD：G 5/5、T 适用 4/4（6 项 N/A 有据）、N 4/4、FR-01~11 ↔ AA 11 子模块一一对应
- S3 领域模型：聚合根 A1-A4 判定（全项目唯一「聚合根+零持久化」）+2 值对象+无子流程/无关联对象均注明理由；零持久化三处同口径
- S4 API：M1~M5 全过（单聚合根裁剪矩阵实证）+ GL 适用项通过；2 端点 2/2 三段式
- S5 变更摘要：8 文档 changelog 齐（机械实证），摘要与正文点数一致
- S6 PageSpec：9 节结构+机械项过，readonly 零可达负空间与 api 权限矩阵逐项对应
- S7 跨阶段一致性：Action 2/2 机械比对一致、待对齐-1~-6 双侧闭环、帧契约字段逐键对齐；唯一偏差 W1=api 4.1.1 字段清单缺「鉴权会话引用」（D2/BA 有、D3 无，且 D-07 复核机制需要其承载）
- S8 枚举池：39 标识符无重复、2 本域枚举池=API=PageSpec 三方逐字同值、protocol_error 增补双侧声明闭环
- WARN 1 项（W1）+ INFO 6 项（I1 INDEX 元数据滞后、I2 ACTIVE_STACKS 悬空绑定待所有者确认、I3 字典偏差复核接受、I4 GL-010/011 口径、I5 FX 清单归 Gate 2、I6 protocol_error 的 PRD 形态）
- B/F 阶段入场注意：以 api-terminal.md 2 端点为验收计数口径（1 HTTP GET + 1 WS，零持久化无 DDL/SQL）；B 阶段实现要点=TerminalHandshakeGuard 九步准入链（I-01）、TerminalSessionRegistry/PtyGateway/SysUserDirectory 三契约（R-08 参数数组 spawn）、D-03 心跳 30s×2、D-04 256KB 环形缓冲、D-07 30s 会话复核（实现时补 W1 字段承载）、D-06 审计两段口径（建立失败拒建 TERM_006）；F 阶段=xterm.js 多标签（上限=并发 5）+ closed 帧 6 原因分级反馈 + userOptions 开框即拉；FE/FX 全量清单在 Gate 2 执行
