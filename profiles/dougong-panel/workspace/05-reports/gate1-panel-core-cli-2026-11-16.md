# Gate 1 增量验证报告 — panel-core CLI 增量（设计→代码门控）

**模块**：panel-core（CLI 管理通道增量，ADR-030）
**检查时间**：2026-11-16（增量检查员·cli·G1）
**增量产物版本**（均本次实读 + `git diff` 实看）：BA/AA/FL 分册 v1.1→v1.2、聚合 business-arch.md v2.1→v2.2 / function-list.md v2.1→v2.2（核验零改动行）、prd-panel-core.md v1.2→v1.4（v1.3=CLI 增量 D2 + v1.4=D3 联动行）、domain-model.md v1.16→v1.17（零新对象注记）、api-panel-core.md v2.0→**v2.1**（4.1.4 CLI 入口命令族契约 + D-P17 + PANEL_012）、INDEX.md、knowledge/design-decisions/index.md（+ADR-030）
**检查范围声明**：本轮为 CLI 增量 D1~D3 产物的增量口径核验（增量正确性 + 增量-存量一致性 + 存量抽查回归）；编号源沿 engine-rules.md G1-01~G1-11（本轮触达项映射见 §3），完整复验可跑 /validate。

> **工作区并发提示（如实登记）**：本轮核验窗口内，同一工作区有并行增量落盘——聚合 business-arch.md/function-list.md 在 CLI 增量的 v2.2 行之后追加了 v2.3（site-mgmt v0.1.2 反馈增量）与 v2.4（env-mgmt 新模块子树）行，另有未跟踪的 env-mgmt 三份 D1 文档与 doc-center.json 再生成（generatedAt 2026-11-16T14:20:56Z，实跑 `git diff` 仅时间戳一行变化）。实跑 `git diff | grep -cE "panel-core|ADR-030|CLI 管理通道|reset-password"` 于三份 site-mgmt 分册 = **0 命中**，与 panel-core CLI 增量零重叠；panel-core 相关关键行在本轮窗口内未漂移（PANEL_012 恒 :155、CLI 入口块恒 :292 起、api v2.1 changelog 恒 :386）。聚合 v2.2 行因 env-mgmt 子树插入第二/三部分而顺移（business-arch.md :444 / function-list.md :972），内容逐字未变。

---

## 1. 五项硬检查（增量口径）

| # | 检查 | 结论 | 证据（本轮实读/实跑） |
|---|------|------|----------------------|
| 1 | **编号接力** | **PASS** | D1：S-06-04 新增且全仓唯一（`grep -rn "S-06-04" --include="*.md"` 实跑，仅 panel-core 五文档 + INDEX 命中，无他模块占用）；S-06-01/02/03 存量在位（AA :79/:91/:101），2.1.1.4 L4 编号承接 2.1.1.1~2.1.1.3（BA :69-81 实读连续无跳号）；计数三方同数——BA L4 标题 `grep -c "^###### "` = **19** = BA v1.2 行「19 业务活动」（:271），AA `grep -c "^##### "` = **19** = FL v1.2 行「覆盖功能子模块扩为 19 个」（:376），7 关键业务不变。D2：FR-19 唯一（PRD FR 行实读 FR-01~FR-19 共 19 行无重复，FR-17/18 存量完好）；待对齐-8 接力（-1~-8 连续完整，prd :182-190 实读）、待对齐-2 状态更新（:183）。D3：D-P17 承接 D-P16（`grep -oE "^\| D-P[0-9]+"` 实跑 D-P01~D-P17 全序唯一）；PANEL_012 承接 PANEL_011（`grep "^| PANEL_"` 实跑 001~012 全序唯一）；I-P 零新增（I-P01~I-P10 维持，与零新对象一致）；CLI 入口块落位 4.1.4 接口设计内（:292-305，v2.1 changelog 与 INDEX 所引「4.1.4 CLI 入口」一致）；D-P17 所引「4.1.1 领域服务」（:242）与「4.1.5 仓储契约」（:309）实存且节号准确 |
| 2 | **枚举与错误码接力** | **PASS** | 错误码：PANEL_012 唯一新增（api :155 INIT_PASSWORD_MISSING，全仓 grep 仅 panel-core 文档+INDEX；退出码 2 语义与 D-P17/4.1.4 一致）；PANEL_001~011 存量行 diff 实看零改动。枚举：**零新增枚举/零取值扩展**——audit_source 为既有池值 `web\|cli\|system`（04-business-enum.md:23，定稿 2026-11-12），且枚举池文件在 git 修改清单外（实跑 `git status --porcelain`）与「枚举池既有 audit_source」声明互证。审计动作码**全量复用零新增**：panel_config_changed / panel_entry_path_regenerated 为 api :169 既有常量（v2.1 仅追加复用注），password_reset 为 account-auth 既有常量（api-account-auth.md:178，:257 security 分类）——三者与 4.1.4 命令表审计列逐行对上（set-port/off→panel_config_changed，regen→panel_entry_path_regenerated，reset-password→password_reset，status/info/show/service 无审计，与 prd FR-19 审计列一致） |
| 3 | **变更记录完整**（G1-09/GL-001a） | **PASS** | 10 份触达文件 changelog 逐份核对均有本轮行：BA v1.2（:271）/AA v1.2（:475）/FL v1.2（:376）/聚合 business-arch.md v2.2（:444，核验零改动说明）/聚合 function-list.md v2.2（:972，同）/prd v1.3+v1.4（:279-280）/domain-model v1.17（:377）/api v2.1（:386，interface_added + rule_added 双类型 + 9 个目标节号）；配套：INDEX.md requirement/api 两 note 增量改写（diff 实看）、ADR-030 入 knowledge 索引（:39）。GL-001a 实跑：全部文档头「版本」=changelog 最新行版本、「最后更新 2026-11-16」≥ 最新行日期——**上轮 W1（prd 头版本滞后）已闭环**（头 v1.4=最新行 v1.4，v1.3 行明示「表头版本滞后偏差一并修正」）。未触达文件与声明三方互证：api-account-auth.md（5.1 消费预告明示不改动）、双 PageSpec（D4=N/A 说明行 prd :191）、枚举池（无新增）均不在 git 修改清单 |
| 4 | **与 ADR-030 语义一致** | **PASS** | 以 ADR-030（knowledge :39）为基准逐条比对 BA 2.1.1.4（:81-83）/AA 2.1.1.4（:111-124）/FL 附录A·B（:355/:367）/PRD FR-19（:64）/api D-P17（:45）+4.1.4（:292-305）六层文本：① 六命令族 status/info/reset-password/set-port/entrypath show\|regen\|off/service start\|stop\|restart\|status 六方同一命令集零增减；② 「直接操作本机数据目录/SQLite、复用进程内 service 层、无 HTTP 绕行」逐层在位（D-P17+4.1.4 通用约定+FR-19）；③ 读命令运行时可直读（SQL #2 通道注）/写命令统一提示 `systemctl restart dougong`（且诚实标注实际生效仍按 config_effect_mode）；④ 变更审计 source=cli（枚举池既有值）；⑤ 裸 `dougong`=启动服务；⑥ reset-password 新密码仅经 DOUGONG_INIT_PASSWORD 注入不经 argv（缺失=PANEL_012，与 init 同口径）；⑦ service 子命令 systemctl 参数数组 R-08；⑧ 无新领域对象（domain-model v1.17 注记+各清单零改动）/无前端页面（PageSpec 未动+prd D4=N/A 行）——全部一致。增量细化项（退出码 0/2/1 约定、set-port 端口域 1024..65535 复用 I-P02/R-14、reset-password 会话吊销即时与 restart 提示的关系说明）为 ADR-030 之下的合理具体化，无一处与 ADR 相抵；Web 并发写仲裁显式挂 prd 待对齐-8 归 B 阶段（:190），非隐藏矛盾 |
| 5 | **存量抽查（既有 init 契约未被破坏）** | **PASS** | ① api-account-auth.md **零修改**（`git status --porcelain` 实跑不在清单）；4.4.4「CLI 入口（非 HTTP）——服务方法规格」原样在位（:419-423）：`AuthService.InitViaCLI` 与向导同仲裁/stdout 人类可读/审计 source=cli/非 0 退出 AUTH_009 语义——D-P17 与 v2.1 changelog 所引「沿 init CLI『非 0 退出+stdout 提示』先例，api-account-auth.md 4.4.4」真实成立；② reset-password 消费预告所引 HTTP 同源语义逐点吻合（api-account-auth.md:257：A10 长度≥8 / 成功吊销全部会话 cascade / password_reset security）；③ api-panel-core.md HTTP 侧存量零破坏：10 接口清单与权限矩阵逐行同 v2.0（4.1.4 表 :262-273 实读，diff 无触）、PANEL_001~011 行未动（diff 仅 +PANEL_012）、3.1 中间件链本体未动（仅 +CLI 说明行 :100）、DDL 5.3 与 SQL #1~#7 语句零变更（仅 #2/#3 追加 CLI 场景注）、跨接口复用规则仅句尾追加 CLI 复用子句；④ M2 抽查：10 条 HTTP 路径仍全三段式 /panel/{resource}/{action}；M4 抽查：`grep -nE "TODO\|TBD\|待补充\|待填写\|XXX\|占位"` 于 8 份触达文档仅 2 命中且均为存量语义文本（domain-model:79「无占位者」竞品措辞、:374 changelog「占位行关闭」），CLI 新增内容零占位符 |

## 2. 与 S 先例/跨模块契约的一致性（G1-11 增量面）

- **结构先例**：4.1.4 CLI 入口命令族表「沿 api-account-auth.md 4.4.4 init『服务方法规格』先例」——先例实存（见硬检查 #5①），且扩展形态合理（命令表五列：命令/参数/语义与输出要点/退出码/审计），非 HTTP 通道不进 3.1 中间件链的声明与 account-auth init 先例同构。
- **5.1 消费预告**（api :329）：account-auth「用户密码重置业务」消费预告行声明「OHS 方法签名待其文档登记、本版以消费预告登记不改动 api-account-auth.md」——单向预告、不动提供方文档，沿 v2.0 site-mgmt 跨模块契约的登记纪律（彼为双向、此为单向预告+回填承诺，差异源于提供方文档本轮不动，处理诚实）。
- **D4=N/A**：CLI 无页面 → PageSpec 不动、无新增 Action → S7 Action-API 机械 diff 无增量面（HTTP 10 接口↔10 Action 基线未动）；prd :191 说明行登记 N/A 理由（ADR-030 无前端页面）——口径自洽。

## 3. G1-01~G1-11 映射（本轮触达项）

| 编号 | 检查项 | 状态 | 说明 |
|------|--------|------|------|
| G1-01 | 架构文档与模块设计一致 | PASS | BA/AA/FL/API 四层 CLI 通道表述同源同口径（硬检查 #4 六层比对） |
| G1-02 | PRD 覆盖业务场景与角色 | PASS | FR-19 覆盖六命令族；访客（终端形态）职责更新（prd :49-51）；CLI 增量完备性复核报告在档（prd 附录，G 5/5、T 适用 4/4、N 4/4） |
| G1-03 | API 设计完整（路径/请求/响应/错误码） | PASS（附 W1） | CLI 契约六行参数/退出码/输出要点/审计齐备；PANEL_012 补位；W1 为 5.1 节号引用失准 |
| G1-06 | 状态机定义完整 | PASS（N/A 面） | prd 第六部分 +CLI 非状态机辨析行（:133），与 4.1.2 既有口径一致 |
| G1-07 | 枚举统一无冲突 | PASS | 零新增枚举；audit_source 既有池值；动作码复用零新增（硬检查 #2） |
| G1-08 | 接口路径规范 | PASS | HTTP 存量 10 路径三段式不变；CLI 非 HTTP 通道不适用路径规范（已声明不经 3.1） |
| G1-09 | changelog 已填写 | PASS | 硬检查 #3，10 文件全 |
| G1-10 | 状态索引已更新 | PASS | INDEX requirement/api note 增量改写在位（diff 实看） |
| G1-11 | 跨阶段模型一致 | PASS | 零新对象（domain-model v1.17 注记 ↔ api 4.1.1 ↔ DDL 零变更三方一致）；CLI 无 Action 面；G1-04/G1-05（交互覆盖/字段对齐）本轮无增量面（无页面） |

## 4. WARN 项

- **W1（本轮新增·交叉引用精度）** api-panel-core.md:329（5.1 消费表）将 reset-password 同源 HTTP 接口引为「api-account-auth.md **4.4**」——该接口实际位于 **4.1.4 接口设计（panel_user）**（api-account-auth.md:257 实读）；4.4 为「初始化状态」聚合（init 先例在 4.4.4）。语义无错、节号失准，单行修正建议：改为「api-account-auth.md 4.1.4」。
- **W2（上轮延续，本轮文件零改动）** domain-model.md panel_config 行（:201 区域）行内枚举仍为 `https_mode{off,self_signed,imported}` 未回填 acme；:343-344 存储映射说明仍用「归 D3 DDL 修订，prd 待对齐-6/-7」未关闭措辞（prd 早已关闭）。
- **W3（上轮延续，PageSpec 未动）** admin PageSpec:206 残留「该动作在设置页 Card 6」——更新检查 Card 已顺延为 Card 8。
- **W4（上轮延续，PageSpec 未动）** readonly PageSpec:37 字典清单缺 cert_status。
- **W5（上轮延续，domain-model 未动）** domain-model:253 子流程输入列「panel_cert（绑定域名、…ACME 账户引用）」字段归属措辞与 D3 定稿契约（bind_domain 属 panel_config、无账户引用）不符。
- **W6（上轮 W1 延续）** R-10 口径笔误「建议但强制」×3 仍在：business-arch-panel-core.md:13 / function-list-panel-core.md:13 / prd-panel-core.md:106（行号随 v1.3/v1.4 增行 104→106）。
- **W7（上轮 W2 延续）** 菜单行统计口径仍不可机械复现：prd:55「12 菜单行 + 6 机制型」+ CLI 补语——绝对计数基准未在 FL 附录明示（CLI delta 本身正确：+1 机制型、不新增菜单行，12+7=19 与 AA 19 子模块吻合）。

## 5. INFO 项

- **I1** function-list-panel-core.md:13 依据行仍引「application-arch-panel-core.md **v1.1**」（AA 已 v1.2）——版本引用滞后，单行修正。
- **I2** api v2.1 输入行引「prd-panel-core.md v1.3」而 prd 现为 v1.4——v1.4 为 D3 联动行（内容仅回指 api v2.1），v1.3 是 D3 设计时的真实输入，属准确溯源而非滞后（上轮 W1 的主缺陷=prd 头版本滞后，本轮已闭）。
- **I3** doc-center.json 由并行工作流再生成（仅 generatedAt 变化）；其粒度不含 FR/S 编码（实跑 json 解析：FR-16/17/18 与 S-07-03 均不载）——CLI 增量「未跑 doccenter」的两处声明（DDD 归类无变化/枚举无新增）与产物现状一致，无失真。
- **I4** 工作区并发增量（site-mgmt v2.3 / env-mgmt v2.4）与 panel-core CLI 增量零重叠（见文首提示）；本报告所有结论仅对 panel-core CLI 增量产物负责。
- **I5** CLI 退出码在 service 子命令族允许「非 0 透传 systemctl 失败码」（超出 0/2/1 三值）——已在命令表内显式声明，非隐藏偏差，B 阶段实现时按表执行即可。

## 6. 总结

**判定：WARN**（CRITICAL 0 / 契约断裂 0）

- 五项硬检查（编号接力 / 枚举与错误码接力 / 变更记录完整 / ADR-030 语义一致 / 存量 init 契约抽查）**全部通过**，每项均有本轮实读或实跑证据。
- 7 项 WARN = 本轮新增 1 项（W1 节号引用失准，单行级）+ 上轮增量 Gate 1 遗留延续 6 项（W2~W7，均位于本轮零改动文件，属在案延期）；3+2 项 INFO 如实登记（含工作区并发观测）。
- CLI 增量对存量契约的侵入性为零（HTTP 接口/错误码/DDL/SQL/枚举池/account-auth 文档均未动），上轮 W1（prd 版本元数据）已闭环。建议 W1/I1 随下次触达 api/FL 文档时单行小修；W2~W7 维持上轮处置建议（增量代码阶段并行小修）。
- 下游：CLI 增量 D0-D4 全链已齐（D4=N/A 登记在案），可进入 CLI 增量代码阶段（B/F）；待对齐-8 余量（stdout 版式、Web 并发写仲裁）按登记归 B 阶段实现口径。
