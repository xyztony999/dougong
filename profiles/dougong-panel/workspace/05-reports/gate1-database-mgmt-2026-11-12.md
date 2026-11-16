# Gate 1 验证报告 — 设计→代码门控

**模块**：database-mgmt（数据库管理）
**检查时间**：2026-11-12
**检查者**：门控检查员·database-mgmt（validation skill · Gate 1；检查项编号唯一来源：engine-rules.md G1-01~G1-11；S1~S8 为 design SKILL.md「Gate 1 阶段汇总预检」系列，两套编号不混用）
**产物版本**：BA/AA/FL v1.0、PRD v1.0、domain-model v1.10（database-mgmt 段落，含 v1.9 追加 + v1.10 D3 回填）、API v1.0、PageSpec admin/readonly v1.0
**Profile 绑定**：`.claude/settings.json` 实证 `ACTIVE_PROFILE: dougong-panel`；`ACTIVE_STACKS: "go-vue3"` 但 `profiles/` 目录仅含 dougong-panel（ls 实证）——栈层规则文件不存在，同前六模块报告先例（见 I2），本 Gate 实际执行层 = 引擎机制 + 业务规则；栈强绑定条目按 account-auth~site-mgmt 先例口径 N/A（见「栈特化条目 N/A 声明」）

## 判定：**PASS**（复检第 1 轮收敛，2026-11-12——首轮 FAIL（F-1 路径格式 21/23）已修复并机械复验通过；首轮完整记录保留于下文，复检记录见文末「复检第 1 轮」）

> 首轮判定（已被复检取代）：**FAIL**——单项 CRITICAL：S4 / G1-08 接口路径格式 21/23；当时 S1/S2/S3/S5/S6/S7/S8 全过。

## 检查对象

| 阶段 | 文件（均本次实读全文） |
|------|------|
| D1 | 00-architecture/business-arch-database-mgmt.md（268 行）、application-arch-database-mgmt.md（435 行）、function-list-database-mgmt.md（430 行） |
| D2 | 02-detailed/prd/prd-database-mgmt.md（312 行）、prd/domain-model.md（database-mgmt 段落：本域判定 :87-103 / 业务域行 :115 / 对象 5 行 :146-150 / 过程数据注 :158 / 子流程注 :173 / 关联注 :187 / 集成 4 行 :218-221 / 存储映射 :223+:230） |
| D3 | 02-detailed/api/api-database-mgmt.md（466 行） |
| D4 | 02-detailed/interaction/admin/database-mgmt/database-mgmt.md（264 行）、interaction/readonly/database-mgmt/database-mgmt.md（93 行） |
| 参考 | 01-solution/04-business-enum.md（37 条，database-mgmt 定稿 4 条 :43-46、db_type 不采用 :15）、workspace/INDEX.md、engine-rules.md、validation SKILL.md、design SKILL.md 与 d3-api.md/d4-interaction.md 分册、api-site-mgmt.md（路径先例核验） |

## S1~S8 阶段汇总预检结论（判定主表）

| # | 预检项 | 规则对应 | 结果 | 关键证据（实读/实跑） |
|---|--------|---------|------|------|
| S1 | 三文档全局检查 | G1-01 | **PASS** | BA 层级完整：4 L2/7 L3/20 L4（business-arch:191-224 树与第二部分 2.1~2.4 逐节一一对应），更新记录自述「4 阶段/7 关键业务/20 业务活动」（BA:268）与实数一致；AA 7 功能模块 S-31~S-37 + 20 子模块（1 机制型 S-31-01）均带功能说明+用户类型复选框+调用关系（application-arch:75-343），树=正文（:349-379）；**模块编码连续性机械实证**：实跑 `grep -hoE "\bS-[0-9]{2}\b" 00-architecture/application-arch-*.md \| sort -u` = S-01~S-37 连续无缺口，`grep -lE "\bS-3[1-7]\b"` 仅命中 application-arch-database-mgmt.md（其余六模块 AA 最大码分别为 S-05/S-12/S-16/S-18/S-25/S-30），C-01~C-10 未被触碰——与 AA:24 编码衔接声明一致；机制型覆盖闭环：S-31-01 无页面经 FL:40 全局行为 + AA 附录A 恰 1 行映射（AA:383-390）覆盖；FL admin 19 菜单行（FL:42-229）↔ AA 19 交互子模块一一对应、readonly 7 菜单行（FL:230-313）= readonly 可见子集；FL 第三部分两视图=第二部分（FL:318-394）；BA 附录A 8 条 L3 依赖（BA:230-241）与 AA 调用关系逐条同构（S-31-02/03/04、S-33-04→S-32-01、S-33-05/S-36-03→S-31-02、S-35-01→S-32-01、审计 10 处）；A18/A19/A20、R-03/R-08/R-15、ADR-007/ADR-018 口径在 BA/AA/FL/PRD/API/PageSpec 六文档一致（探测范围、凭据不回显、移除不删数据、审计脱敏） |
| S2 | PRD 完备性检查 | G1-02 | **PASS** | G 5/5（目标 prd:11-15、IN 20 项/OUT 11 项 :19-58、2 角色+访客不涉及+获得方式 :60-67、领域对象指向 domain-model :270-271、对象关系 :271）；T 适用 4/4：T1 字段=domain-model 对象行核心字段列（dm:146-150）、T1a 第七部分 :181-187（现阶段无对外契约+cron-backup 方向不预先建模+消费方视角两链）、T4 五类查询形态 :169-177、T8 幂等语义逐动作 :213；T2/T3/T5/T6/T7/T9 共 6 项 N/A 均有据（第六部分 7 项辨析 :153-163 + 无审批 :165、导出为业务动作非台账导出 :179、本机组件非外部系统 :283-284、非工具型 :286）；N 4/4（N1 量级+未锁定项移交 :191、N2 15 个异常分支 :193-211、N3 16 条规则+幂等补充 :128-147/:213、N4 零台账+审计追溯 :215）；FR-01~FR-20 ↔ AA 20 子模块一一映射（每 FR 行携带 S-xx-yy 码，prd:73-126）；PRD 附录自查（prd:259-306）与本次独立复验一致 |
| S3 | 领域模型分离 | G1-06 / G1-11 | **PASS** | 5 类对象分类完成并落入 domain-model.md 聚合视图：2 聚合根 db_instance/redis_instance（A1 ✓/A3 ✓、A2 △/A4 △，按「2 项及以上通过且 A1 通过」规则判定，dm:99-100）+ 2 域内实体 database/db_user（A1-A4 全不通过+零台账判定，dm:101-102）+ 1 内嵌值对象凭据（dm:103/:150）；过程数据（慢日志/进程列表/key 条目/概览摘要/字符集候选/备份 SQL 产物/探测管线）归类不入清单（dm:158，与 PRD 第九部分过程数据表 :229-240 双侧同记）；无子流程有据（dm:173：导出导入为单动作、探测为后台机制链路）；无关联领域对象有据（dm:187）；分离落盘独立聚合视图，PRD:270-271 与 API 1.4（api:44-53）引用一致；存储映射 db_instance/redis_instance→合一表 db_instance、database/db_user→无表（dm:223/:230）↔ API D-01/DDL（api:29/:421-440）闭环；G1-06 状态机：无业务状态机为 D0 拍板，PRD 第六部分 7 项辨析表（reachable/unreachable=探测态、纳管=记录存在语义、TTL=倒计时属性等，prd:153-163），API 4.1.2 同口径（api:230） |
| S4 | API 全局模板合规（M1~M5 + GL） | G1-03 / G1-08 | **FAIL**（仅 M2/GL-009，其余过） | M1 章节结构：必选 1.x/2.x/4.x/6.x 全在（本模块 2 聚合根 → 第 2 章必选且存在：2.1 关系图/2.2 不变量 I-01~I-06/2.3 编排 5 项，api:63-96）；3.1~3.4/5.1~5.3/6.1~6.3 齐。**M2 路径格式实跑** `grep -hoE "(GET\|POST) /db/[a-zA-Z/_-]+" api` = 23 条唯一路径（GET 10/POST 13，与 3.3 矩阵、4.1.4/4.2.4/4.3.4/4.4.4、5.x、6.1 计数四处同数），**其中 21 条三段式、2 条四段式**（`GET /db/redis/key/list`、`GET /db/redis/key/ttl`）——d3-api.md:280 M2 规则「所有接口路径符合 /domain/resource/action 三段式（对应 G1-08）」无「双级资源」例外条款（d3-api.md/d4-interaction.md grep 双级/嵌套/子资源/四级 均无命中）；api:104 自declared「双级资源 /db/redis/key/{action}（同 site-mgmt /sites/log/tail 先例）」的**先例引用失实**：实跑 site-mgmt api 路径 grep = 20 条全三段，`/sites/log/tail` 即 sites(domain)/log(resource)/tail(action) 普通三段式，非双级资源；前六模块 M2 全量达标（18/18、7/7、4/4、7/7、22/22、18/18）本模块为首个破例者 → M2 未过，GL-009（接口路径全部为三级路径格式）同源失败，INDEX.md api 阶段 note「M1~M5 机械自检通过」（INDEX:99）与机械实证不符（详见 F-1）。M3 枚举引用：4+1 枚举均在池（04:43-46 grant_level/:28 audit_result，池 37 标识符 uniq -c 全 1）。M4 占位符实跑 `grep -nE "按需填写\|TODO\|FIXME\|TBD"` 八产物 exit=1 无残留。M5 变更记录 6.1 含 v1.0 initial 行（api:448）。GL：GL-001a（头部 2026-11-12=更新记录）/GL-001b/GL-004（1.4 总览 ↔ 4.1~4.4 逐对象对应）通过；GL-009 **未过**（同 M2）；GL-010/011 按无栈通用契约口径（读 GET/写 POST 声明 api:105、`{"code","message","data"}` 包装 api:106，account-auth D-07 同款，六模块先例，见 I3）；GL-012 通过（3.3 矩阵全部 POST 行含 AUTH_008+CSRF，api:129-146）；GL-021/022/026/027/028 枚举闭环通过（来源文件级 04-business-enum.md 标注 api:177）；GL-029/030 N/A（无 detail 子表开关场景）；GL-031 按无栈口径 N/A（charsets 接口承载 options 语义，api:333）；GL-032 通过（实跑 grep API 无 dict 端点；前端偏差声明见 I4） |
| S5 | API 变更摘要 | G1-09 | **PASS** | api 6.1 v1.0 initial 行含变更类型/变更摘要/目标节号（api:446-448）；6.2 变更类型词汇表、6.3 语义化版本规则齐（api:450-466）；全部 8 份设计文档 changelog 齐：BA:264-268、AA:431-435、FL:426-430、PRD:308-312、domain-model:232-246（v1.9 追加+v1.10 D3 回填两行）、api:442-448、PageSpec admin:258-262、readonly:87-91 |
| S6 | PageSpec 输出校验 | G1-04 / G1-05 | **PASS**（附 W2 建议项） | 两份 PageSpec 9 节结构齐（admin:13-264、readonly:14-93）；模块元信息含唯一数据源=api-database-mgmt.md v1.0（admin:9、readonly:9）、业务类型/页面类型/UI 框架/状态管理/错误处理齐（admin:15-22）；admin Action 表 23 行含方法/路径/页面/权限/CSRF 列（admin:26-50）；字典 Key 4 项同源池（admin:56-61）；列表页 db-list 查询条件+表格列（序号 60/identityKey fixed:left 240/operList action fixed:right）+空态引导齐（admin:84-114）；表单 8 个弹窗/行内动作字段-校验-默认值齐（admin:120-188，凭据零回显贯穿 5.1/5.3/5.5/5.7）；详情页 mysql-manage 三 Tab/redis-manage 三 Tab（key Tab admin 专属）齐（admin:190-229）；路由 3 条+engineGuard 守卫链（admin:231-241）；readonly 7 Action+16 负空间（readonly:26-36）与 api 3.3 权限矩阵无冲突；G1-04 页面类型覆盖：列表/详情（hybrid 治理页）/表单（Dialog 弹窗）齐，审批 N/A 有据（PRD:165 无审批语义）；G1-05 字段对齐逐项核对通过（明细见 CT-002）；**W2**：readonly 第 1 节页面类型行「mysql-manage=hybrid（两 Tab）」（readonly:20）与其自身可见页面行「库/用户/观测三 Tab 只读」（readonly:21）及第 6.1 节三 Tab 定义（readonly:55-60）不符——readonly 的 mysql-manage 可见 3 个 Tab（库/用户/状态与进程，与 FL 2.2/AA S-32-01/S-33-01/S-34-0x readonly 开放一致），「两 Tab」为计数笔误；FX-001~141 清单未随 payload 发布经本次 `find` 实证（无命中），结构级+机械项已执行（六模块同口径，见 I5） |
| S7 | 跨阶段一致性 | G1-11 | **PASS** | 领域对象：D2（dm:146-150 五对象）↔ API 1.4 总览（api:44-53）↔ 4.1/4.2/4.3 字段与接口 ↔ 存储映射（dm:223/230 ↔ api D-01/DDL 5.3）逐层一致；**Action 机械 diff 实跑**（awk 按表格列提取 method+path，剥离 CR）：API 23 对 == admin PageSpec 23 Action 逐行 IDENTICAL（diff 空输出）；readonly 7 Action 为 API 真子集（comm 反向差=0）、负空间 16 = 23-7 逐项对应 ADR-018/D0 矩阵（readonly:36）；方法/权限/CSRF 三列三方一致（api:129-146 ↔ admin:26-50 ↔ readonly:26-34）；PRD 待对齐-1~-10 的闭环按本工作区先例口径（PRD 表为分派记录、API D-xx 记录关闭——monitoring/site-mgmt 同形态，prd-monitoring:154-159/prd-site-mgmt:299-308 均为分派态）双侧成立：D-02 关闭-1、D-04 关闭-2、D-10 关闭-3、D-08 关闭-4、D-06 关闭-5/-8、D-07 关闭-6、D-09 关闭-7、D-12 关闭-9、D-11 关闭-10（api:30-40 决策表+6.1 自述），无悬挂 |
| S8 | 枚举池无冲突 | G1-07 | **PASS** | 实跑池标识符提取+`sort \| uniq -c`：37 条全唯一（最大计数 1，无跨域命名冲突）；API 引用 4 枚举与池逐字一致三方同值：db_engine{mysql,mariadb,redis}/db_conn_mode{socket,tcp}/instance_conn_state{reachable,unreachable}/grant_level{readonly,write,all}——04:43-46 ↔ api:181-184 ↔ admin:58-61 ↔ readonly:40（grant_level 用于授权概况 tag 只读呈现）；audit_result 复用 account-auth 定稿（04:28）；既有 draft db_type 不采用且理由注明（04:15，被 db_engine 取代）；grant_level 为 D3 新增定稿入池并注明关闭 prd 待对齐-3（04:46）；domain-model v1.9/v1.10 更新行（dm:245-246）与 PRD:225/312 自述一致 |

**判定规则回执**：S4 未过（M2/GL-009）→ **FAIL**（阻断）；S1/S2/S3/S5/S6/S7/S8 全过。

## F-1（CRITICAL，阻断项）

**G1-08 / M2 / GL-009：2 个接口路径为四段式，且「双级资源」例外无规则依据、先例引用失实。**

- 违规路径：`GET /db/redis/key/list`、`GET /db/redis/key/ttl`（api:146/:383-389/:4.4.4；admin:49-50 同路径）。
- 规则依据：engine-rules.md G1-08「接口路径是否符合规范（/domain/resource/action）」；d3-api.md:280 M2「所有接口路径符合 /domain/resource/action 三段式（对应 G1-08）」；validation SKILL.md GL-009「接口路径全部为三级路径格式」。三处均无例外条款（d3-api.md/d4-interaction.md grep「双级/嵌套/子资源/四级」零命中）。
- 先例核验：api:104 声明「双级资源 /db/redis/key/{action}（同 site-mgmt /sites/log/tail 先例）」——实跑 `grep -hoE "(GET|POST) /sites/[a-zA-Z/_-]+" api-site-mgmt.md | sort -u` = 20 条全部三段式；`/sites/log/tail` 的结构是 sites(domain)/log(resource)/tail(action)，并非双级资源。先例不存在。
- 影响面：前六模块 M2 全量三段达标（account-auth 18/18、panel-core 7/7、monitoring 4/4、service-mgmt 7/7、file-manager 22/22、site-mgmt 18/18）；本模块为首例破格，若放行将使 /db 域形成两种路径文法，破坏 GR-05 前后端契约对齐的统一口径。另：INDEX.md:99 api 阶段 note「M1~M5 机械自检通过」与机械实证（21/23）不符，系自检声明失真，恰为本 Gate 应拦截项。
- 修复口径（机械改名，不改语义）：将两接口改为三段式（如 `/db/key/list`、`/db/key/ttl`，resource=key 归 /db 域单数资源命名一致；或合并语义近似方案由设计侧定），同步修改 api 3.1/3.3/4.4.4/6.1 + admin PageSpec 第 2/6.2 节 + readonly 负空间清单 + INDEX api note；改后重跑本 Gate 机械项复验（接口计数应仍为 23、三方 diff 应 IDENTICAL）。

## G1-01~G1-11 逐项结论

| 编号 | 检查项 | 结果 | 依据（本次实读/实跑） |
|------|--------|------|----------------------|
| G1-01 | 架构文档与模块设计一致 | **PASS** | AA 1.1 承接 BA v1.0 并完成 DDD 四层归类（7 L3 全支撑域、三层空且注明理由，application-arch:15-20）；PRD 输入列明 D0+D1 三文档（prd:8）；API 输入列明 PRD v1.0+domain-model+D0+D1（api:8）；PageSpec 唯一数据源=api v1.0（admin:9/readonly:9）；FR 编号按 AA S-31~S-37 组织（prd:73-126）；BA→AA→FL→PRD→API→PageSpec 六文档 A18/A19/A20、R-03/R-08/R-15、ADR-007/ADR-018 口径一致（证据见 S1） |
| G1-02 | PRD 覆盖所有业务场景和角色 | **PASS** | 2 角色+职责+获得方式+访客不涉及（prd:60-67）；20 L4 → FR-01~20 一一映射（含 S 码）；异常场景 15 条覆盖实例不可达/凭据验证失败/改密同步失败两步非原子/requirepass 失效/重名冲突/删不存在/dump 失败/导入中断不回滚/面板外变更/大库分页/空态/越界拒绝/实例卸载不自动移除（prd:193-211）；readonly 边界与 FL 附录A 逐行一致（FL:398-412 ↔ prd:65） |
| G1-03 | API 设计完整（路径/请求/响应/错误码） | **PASS** | 23 接口计数四处同数（3.3 矩阵 api:127-146、4.1.4/4.2.4/4.3.4/4.4.4、5.2 SQL 场景、6.1 changelog）；每接口含 query/req/校验规则/响应 data 形状/错误码/审计动作（api:253-266/:330-335/:361-367/:386-389）；错误码 DBM_001~020 实数 20 行（api:156-175）+复用 AUTH_001/002/003/008/012，覆盖 PRD N2 接口侧可表达分支（不可达 DBM_003、凭据未配 DBM_004、验证失败 DBM_005/006、同步失败 DBM_019、在途 DBM_016、导入超限 DBM_014 等）；DbGateway 网关契约 24 方法（api:276-303）替代引擎侧仓储契约；DDL 5.3 + SQL 清单 5.2（6 条全参数绑定）齐 |
| G1-04 | 交互设计覆盖所有页面类型 | **PASS** | 列表页=db-list（admin:84-114）；详情/hybrid 治理页=mysql-manage 三 Tab+redis-manage 三 Tab（admin:190-229）；表单=8 个 Dialog 弹窗/行内动作（admin:120-188）；审批 N/A 有据（prd:165）；readonly 视角差异完整（readonly:42-74，key Tab 不渲染=负空间呈现） |
| G1-05 | API 字段与交互规格字段对齐 | **PASS** | 逐字段核对：instance/list data 11 键（api:255）↔ db-list 8 列+摘要条（identityKey 为 host:port/socketPath 派生展示字段，已标注 D-01 组装规则，admin:96，非臆造）；databaseList {name,charset}（api:332）↔ Tab1 两列；dbuserList {username,host,grants[{database,level}]}（api:363）↔ Tab2 三列+授权 tag；mysqlStatus（api:259）↔ Card1 NDescriptions；mysqlProcesslist 8 键（api:260）↔ Card2 九列；redisInfo sections（api:262）↔ 6.2 Tab1；redisSlowlog 4 键（api:263）↔ Tab2 四列；key/list {nextCursor,complete,items}（api:388）↔ 游标分页+两列+TTL 行内；credentials/create/grant/revoke/root-password/password/import/export req 字段 ↔ 5.1~5.8 表单一一对应（确认新密码为 UI 一致性字段惯例）；DBM_001~020 文案 ↔ 第 1 节错误分流（admin:22/:252）——无臆造字段 |
| G1-06 | 状态机定义完整 | **PASS**（以「无状态机」完整论证满足） | D0 拍板 reachable/unreachable 为探测态；PRD 第六部分 7 项辨析表（prd:153-163）；API 4.1.2 无状态机声明+conn_state 整值覆盖语义（api:230，I-03）；T3 审批 N/A 依据（prd:165） |
| G1-07 | 枚举统一且无冲突 | **PASS** | 池 37 标识符实跑 uniq 全唯一；4 枚举三方同值（04:43-46 ↔ api:181-184 ↔ PageSpec:58-61/40）；grant_level 与既有枚举无命名/取值冲突；db_type 不采用留痕（04:15）；字符集候选明示为实况开放集合非枚举（api:39 D-11、PageSpec:63），未误入字典清单 |
| G1-08 | 接口路径符合 /domain/resource/action | **FAIL** | 21/23 三段式；`/db/redis/key/list`、`/db/redis/key/ttl` 四段式；「双级资源」例外无 d3-api.md 规则依据、site-mgmt 先例引用失实（实跑证据见 S4/F-1） |
| G1-09 | 设计文档 changelog 已填写 | **PASS** | 8 份文档均有版本属性表+更新记录表（明细见 S5）；api 6.1 使用词汇表内 `initial` 类型 |
| G1-10 | 状态索引模块状态已更新 | **PASS** | INDEX.md:98-100 database-mgmt requirement/api/interaction 均 done 且 note 含产物路径与计数口径（其 api note「M1~M5 机械自检通过」经本次实证失真，随 F-1 一并修正，见 F-1 影响面）；gates.gate1 由本次检查落盘为 fail |
| G1-11 | 跨阶段模型一致性 | **PASS** | D2 五对象 ↔ API 1.4 ↔ 4.1~4.3 ↔ 存储映射闭环；无子流程/无关联领域对象/无领域事件 三文档同口径（dm:173/:187、api:274「无 MQ 无领域事件」、prd 第六/九部分）；PageSpec 23 Action ↔ API 23 接口一一对应（机械 diff IDENTICAL，见 S7）；readonly 7+16 负空间与权限矩阵吻合 |

## 执行展开（validation skill Gate 1 三大块）

### 1. PRD 就绪检查

| 层级 | 通过/总数 | 状态 |
|------|----------|------|
| 第一层 G1-G5 | 5/5 | PASS（prd 附录自查 :263-271 与实读一致） |
| 第二层 T1-T9 | 适用 4/4（T1/T1a/T4/T8），N/A 6 项有据 | PASS（T2/T3 辨析 :149-165、T5 :179、T6/T7 :283-284、T9 :286） |
| 第三层 N1-N4 | 4/4（N1 部分锁定已声明） | INFO（未锁定项 10 项经 D-01~D-14 全部关闭，见 S7） |

### 2. API 文档完整性（M1~M5 机械 + GL 全局）

| 章节 | 结果 | 说明 |
|------|------|------|
| M1 章节结构 | PASS | 1.1~1.5/2.1~2.3（双聚合根必选第 2 章在）/3.1~3.4/4.1~4.4/5.1~5.3/6.1~6.3 全在且非空 |
| M2 路径格式 | **FAIL** | 21/23 三段式；2 条四段式（/db/redis/key/*）——见 F-1 |
| M3 枚举引用有定义 | PASS | 4+1 在池（04:28/43-46），无重复 |
| M4 占位符残留 | PASS | grep exit=1（八产物均无 按需填写/TODO/FIXME/TBD） |
| M5 变更记录 | PASS | 6.1 含 v1.0 initial 行 |
| GL 全局一致性 | **FAIL**（仅 GL-009） | GL-001a/b、004、012、021/022、026~028 通过；GL-009 同 M2 未过；GL-010/011 按无栈通用契约口径（I3）；GL-029/030 N/A；GL-031 按无栈 N/A（charsets 承载 options 语义）；GL-032 通过（无 dict 端点；前端偏差见 I4） |

### 3. 前后端契约一致性（CT）

| 编号 | 检查项 | 状态 |
|------|--------|------|
| CT-001 | API 路径与 PageSpec Action 路径对齐 | PASS（23/23 机械 diff 一致 API↔admin；readonly 7 子集+16 负空间）——注：路径本身含 F-1 四段式问题，两侧一致地偏离，修复时双侧同步 |
| CT-002 | 请求/响应字段与页面字段对应 | PASS（G1-05 明细逐键对应） |
| CT-003 | 枚举值前后端一致 | PASS（4 枚举 池=API=PageSpec(admin/readonly) 四方同值） |
| CT-004 | Action 表所有 Action 在 API 有对应接口 | PASS（23/23，无虚构） |
| CT-005/006 | 列表列/表单字段来源于 API VO/AO | PASS（db-list 列出自 list data 键；identityKey 为标注过的派生展示字段；8 表单字段均出自对应 req） |
| CT-007 | 字典 Key 与后端枚举对应 | PASS（admin:56-61 ↔ 04:43-46） |
| CT-008 | 分步表单 generate-id/save 接口 | N/A（无分步表单，表单全 Dialog 形态） |

## 建议/观察项（不改变 FAIL 判定；F-1 修复时一并处置）

| # | 项 | 说明 |
|-----|-----|------|
| W2 | readonly PageSpec:20「mysql-manage=hybrid（两 Tab）」计数笔误 | readonly 可见 mysql-manage 为三 Tab（库/用户/状态与进程，readonly:21/:55-60、FL 2.2 三菜单行、AA S-32-01/S-33-01/S-34 readonly ✓、API database/dbuser/status/processlist readonly ✓ 四方证据一致）——第 1 节该格应改「三 Tab」；与 F-1 同轮小修即可 |
| I1 | api:8 输入行 domain-model v1.9 vs 现 v1.10 | v1.10 为本模块自身 D3 定稿回填（dm:246，合一表 D-01+AES 方案 D-02），api v1.0 成文时点为 v1.9 属正常时序——与 site-mgmt I1 同类元数据滞后，建议下次触碰 api 时更新输入行版本号 |
| I2 | ACTIVE_STACKS=go-vue3 悬空绑定 | settings.json 实证 `ACTIVE_STACKS: "go-vue3"` 但 profiles/ 下无 go-vue3 目录（ls 实证仅 dougong-panel）——同前六模块报告记录；建议运行所有者在进入 B/F 阶段前确认 ER-03 绑定一致性（补齐 profiles/go-vue3 或回正 settings） |
| I3 | GL-010/011 无栈通用契约口径 | 读 GET/写 POST（api:105）+ `{"code","message","data"}` 包装（api:106）为 account-auth D-07 以来的面板内部契约先例（六模块同判）；栈接入后如需统一 POST/Result\<T\> 再行重审 |
| I4 | 字典数据源偏差（无 dict/batch，前端 enums.ts 常量） | admin:63 显式声明——本次复核**接受**：单机面板、4 枚举全固定值、无激活栈规则、04-business-enum.md 单一事实源；API 侧无独立字典端点（grep 实证）。与 account-auth~site-mgmt 先例同判；接入栈 profile 时按 GL-032 重审 |
| I5 | FX-001~141 前端质量清单未随 payload 发布 | 本次实跑 find（payload 全目录 *FX*/*frontend*check* 模式）无命中——PageSpec 注记（admin:264、readonly:93）属实；FE/FX 全量归 Gate 2 前端规则扫描。本 Gate 已执行结构级+机械项（9 节结构/Action 三方机械 diff/字典四方同值/版本记录） |
| I6 | identityKey 列表列为派生展示字段 | db-list「实例标识」列非 API 响应直出键，由 host:port/socketPath 按 D-01 规则组装呈现（admin:96 已标注）——判定为合规派生展示（非臆造），记录口径供 B/F 实现对齐 |

## 栈特化条目 N/A 声明

`profiles/go-vue3` 不存在（ls 实证）→ 栈级规则层为空集（validation skill FR-D2 分层组装栈层无文件）；按 skill「异栈激活时以该栈 rules 替换本节清单」与六模块先例：CH3-002/003（统一 POST/Result\<T\>）、CH4-005a~g/007/008（审计七件套/主键 VARCHAR(32) 规约——本模块 SQLite INTEGER PK AUTOINCREMENT 为无栈自有 DDL 规约，api 5.3 自洽）、CH4-057~070（MQ/事务边界/领域事件）、CH5-004~018（MySQL 型 DDL 规约）、GL-029~031 等栈强绑定条目 N/A；通用意图（契约完整性/路径与错误码与枚举一致性/写接口权限校验/`{code,message,data}` 契约/DDL 与领域模型一致）已在对应 G1 项按通用契约检查，其中 G1-08 未过（F-1）。

## 总结

**判定：FAIL**（S4 未过 → 按判定规则阻断；修复后重跑本 Gate）

- G1-01~G1-11：10/11 通过；**G1-08 未过**（2/23 路径四段式 + 先例引用失实 + INDEX 自检声明失真）
- 通过面：S1 三文档（层级/树/编码 S-31~S-37 连续性机械实证/机制型覆盖闭环/依赖同构）；S2 PRD（G 5/5、T 适用 4/4、N 4/4、FR↔AA 一一映射）；S3 领域模型（2 聚合根+2 实体+1 值对象分离落盘、零台账判定、存储映射↔DDL 闭环）；S5 changelog 八文档齐；S6 PageSpec 结构+字段对齐（附 W2 计数笔误）；S7 三方机械 diff 23/23 IDENTICAL+readonly 7+16 负空间+待对齐-1~-10 双侧闭环；S8 池 37 唯一+4 枚举四方同值
- 阻断面：F-1（G1-08/M2/GL-009）——修复口径为机械改名（如 /db/key/list、/db/key/ttl）并同步 api 3.1/3.3/4.4.4/6.1 + admin PageSpec 第 2/6.2 节 + readonly 负空间清单 + INDEX api note「M1~M5 机械自检通过」表述；建议同轮处置 W2
- 复验口径：修复后重跑 M2 grep（应 23/23 三段）+ 三方 diff（应仍 23/23 IDENTICAL）+ 接口计数四处同数（23 不变）即可收敛，无需重查其余项

---

# 复检第 1 轮（2026-11-12）

**触发**：首轮 FAIL（F-1：`/db/redis/key/{list,ttl}` 四段式 21/23 + W2 readonly Tab 计数笔误）修复后复验。**复检范围**：按同一 S1~S8 清单复核全部产物；重点核验 F-1/W2 修复，其余项机械回归。

## 变更探测（复检输入面）

实跑 `ls -l --time-style` + `wc -l`：首轮报告落盘（23:42:26）后被修改的产物 = api（466→467 行，23:43:51）、admin PageSpec（264→265，23:44:08）、readonly PageSpec（93→94，23:44:09）、INDEX.md（23:44:34）；BA/AA/FL/PRD/domain-model/enum 的 mtime（23:06~23:29）早于首轮读取时点且本轮锚点核验未变（版本行 v1.0×4 + domain-model v1.10、PRD `grep -c "^| FR-"`=20、domain-model db_instance/redis_instance 命中 13、enum 46 行 37 条）——上轮全文实读结论继续有效，本轮对未变文件执行锚点回归而非重读（如实声明）。

## F-1 / W2 修复核验（重点项，全部实读/实跑）

| # | 核验 | 结果 | 证据 |
|---|------|------|------|
| R1-F1a | API 侧改名 | **过** | api v1.1（api:5）：3.3 矩阵 `GET /db/key/list / ttl`（api:146）、4.4.4 两接口 `/db/key/list`、`/db/key/ttl`（api:383-384/:388-389，I-05 前置 engine=redis 保留）、6.1 新增 v1.1 行（api:449，interface_changed + 目标节号「3.1 / 3.3 / 4.4.4 / 6.1」）；3.1 路径格式改为「`/db/{resource}/{action}` 三段式（G1-08/M2，无双级资源例外）」且资源单数清单含 key（api:104）——失实的「双级资源/site-mgmt 先例」声明已删除 |
| R1-F1b | M2 机械复验 | **过** | 实跑 `grep -hoE "(GET\|POST) /db/[a-zA-Z/_-]+" api \| sort -u` = 23 条唯一路径；awk split 段数校验：23 条全部均匀「3 实段」（前导空元素口径 n=4，无一条出现 4 实段）——**23/23 三段式**（首轮 21/23 → 收敛）；GL-009 随之通过 |
| R1-F1c | 四段式残留扫描 | **过** | 实跑 `grep -rn "redis/key" --include="*.md" workspace/`：命中 7 处全部为合法记载——api:449/admin:263/INDEX:105 为变更记录中的历史路径记载（「由四段式…改为三段式」）、api:85 为 I-05 资源族列举（「redis、key 接口要求 engine=redis」，改名后语义仍准确）、api:104 为资源枚举顺列、其余 2 处为本报告自身的首轮记录——规范性章节零残留 |
| R1-F1d | PageSpec 同步 | **过** | admin v1.1：Action 表 redisKeyList/redisKeyTtl 路径 `/db/key/list`、`/db/key/ttl`（admin:49-50），changelog v1.1 行注明「随 API v1.1 改名、Action 名/页面归属/权限不变、S7 23/23 保持」（admin:263）；readonly v1.1 changelog 注明「本模块 readonly 视角无 key 路径引用，F-1 改名不涉及本文档正文」（readonly:92）——与实读一致（readonly 负空间按 Action 名列举，readonly:36） |
| R1-W2 | readonly Tab 计数笔误 | **过** | readonly:20 已改「mysql-manage=hybrid（三 Tab）」，与 readonly:21 可见页面行、第 6.1 节三 Tab 定义（readonly:58-60）、FL 2.2 三菜单行、AA S-32-01/S-33-01/S-34 readonly 开放口径四方一致；changelog v1.1 field_changed 行在案（readonly:92） |
| R1-IDX | INDEX api note 失真表述 | **过** | INDEX:99 api note 已改为「M1~M5 通过——M2 初检 21/23 三段式，经 Gate 1 F-1 修正 key 路径为 /db/key/{list,ttl} 后复验 23/23」——首轮指出的自检声明失真已如实订正 |

## S1~S8 复检结论（回归项）

| # | 复检结论 | 本轮执行内容 |
|---|---------|-------------|
| S1 | **PASS**（沿用+回归） | 未变锚点（BA/AA/FL v1.0）+ 编码连续性实跑：`grep -hoE "\bS-[0-9]{2}\b" application-arch-*.md \| sort -u` = S-01~S-37 连续，`grep -lE "\bS-3[1-7]\b"` 仍仅命中 database-mgmt AA；FL 结构未变（19+7 菜单，上轮全读有效） |
| S2 | **PASS**（沿用+回归） | PRD v1.0 锚点未变（版本行、`grep -c "^\| FR-"`=20）；上轮 G 5/5、T 适用 4/4、N 4/4 全文实读结论有效，本轮无 PRD 变更触发重查 |
| S3 | **PASS**（沿用+回归） | domain-model v1.10 未变（版本行+13 处 db_instance/redis_instance 命中）；api v1.1 仅改 2 路径名，对象分类/存储映射/DDL（api 5.3 逐字未动）/合一表 D-01 均不受影响 |
| S4 | **PASS**（本轮收敛） | F-1 修复后 M2 23/23（R1-F1b）；M1 章节结构未受改名影响（2.x/4.4 标题层级实读确认）；M3 枚举引用未变（api:181-185）；M4 占位符复跑八产物 exit=1 无残留；M5 changelog v1.1 行含类型/摘要/节号（api:449）；GL-009 过，GL-001a（头部 2026-11-12=v1.1 日期）过，其余 GL 项沿用首轮结论（无涉变更） |
| S5 | **PASS**（本轮收敛） | api 6.1 现 v1.0+v1.1 两行（v1.1 使用词汇表内 `interface_changed` 类型+目标节号）；admin:263/readonly:92 v1.1 行齐——8 文档 changelog 全部在案且格式合规 |
| S6 | **PASS**（本轮收敛） | W2 修复实证（R1-W2）；admin 9 节结构未变（本轮实读第 1~3 节+第 9 节，中段 4~8 节仅 Action 路径列受 F-1 影响、已由 R1-F1d 覆盖）；readonly 9 节全读；G1-05 字段对齐不受改名影响（响应/请求字段未动） |
| S7 | **PASS**（本轮复跑） | 三方机械 diff 复跑（awk 表列提取+剥 CR）：API 23 == admin 23 逐行 **IDENTICAL**（diff 空输出）；readonly 7 Action 为 API 真子集（comm 反向差 0）、负空间 16=23-7 且负空间按 Action 名（redisKeyList/redisKeyTtl）与改名后接口对应无歧义；方法/权限/CSRF 三列一致性随 IDENTICAL diff 一并成立 |
| S8 | **PASS**（本轮复跑） | 池复跑：37 条标识符 `uniq -c` 计数集合={1}（全唯一）；4 枚举取值池行完整打印核对（db_engine=mysql\|mariadb\|redis、db_conn_mode=socket\|tcp、instance_conn_state=reachable\|unreachable、grant_level=readonly\|write\|all，04:43-46）↔ api:181-184 ↔ admin:58-61 ↔ readonly:40 四方逐字同值 |

**判定规则回执**：S1~S8 全过 → **PASS**。

## 复检 INFO 项（不阻断）

| # | 项 | 说明 |
|---|-----|------|
| N1（新） | 两 PageSpec 头部「唯一数据源」版本滞后 | admin:9/readonly:9 仍写「api-database-mgmt.md v1.0」而 api 已 v1.1（两文档 v1.1 changelog 均自述「随 API v1.1」）——元数据滞后与 site-mgmt I1/api:8 domain-model v1.9 同类，内容已实质同步（路径/字段/权限全部对齐 v1.1），建议下次触碰时升写 v1.1 |
| I1~I6 | 沿用首轮 | I1（api:8 输入行 domain-model v1.9 vs 现 v1.10）、I2（ACTIVE_STACKS=go-vue3 悬空绑定）、I3（GL-010/011 无栈口径）、I4（dict/batch 偏差接受）、I5（FX 清单归 Gate 2）、I6（identityKey 派生展示字段）均维持原判，本轮无变化触发 |

## 复检总结

**判定：PASS**（首轮 F-1/W2 全部修复并经机械复验收敛；S1~S8 全过；无 CRITICAL 失败）

- 修复面：F-1（M2 21/23→23/23 三段式，3.1 失实例外声明删除，PageSpec/INDEX 同步，残留扫描零规范性命中）+ W2（readonly 三 Tab 笔误）+ INDEX api note 失真表述订正——三处修复均有 changelog/登记在案（api v1.1、admin v1.1、readonly v1.1）
- 回归面：三方 diff 23/23 IDENTICAL、M4 占位符零残留、池 37 唯一+4 枚举四方同值、模块编码 S-31~S-37 唯一性、未变文件锚点（BA/AA/FL/PRD/domain-model）全部通过
- 遗留：N1 与 I1~I6 均 INFO 级，不阻断、无需重跑本 Gate
- B/F 阶段入场口径：以 api-database-mgmt.md **v1.1** 为验收计数口径（23 接口全三段式、key 路径=/db/key/{list,ttl}）；FE/FX 全量清单在 Gate 2 执行
