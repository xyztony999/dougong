# Gate 1 增量验证报告 — 设计→代码门控（panel-core 增量复检）

**模块**：panel-core（面板基座）
**检查时间**：2026-11-16
**检查者**：增量检查员·panel-core·G1（validation skill · Gate 1；检查项编号唯一来源：engine-rules.md G1-01~G1-11；S7 为 design SKILL.md 跨阶段一致性预检项）
**增量产物版本**（均本次实读 + `git diff` 实看）：BA/AA/FL 分册 v1.0→v1.1、聚合 business-arch.md v2.0→v2.1 / function-list.md v2.0→v2.1、prd-panel-core.md v1.0→v1.1（变更记录另含 v1.2 行，见 W1）、domain-model.md v1.15→v1.16、api-panel-core.md v1.0→**v2.0**（任务口径写「v1.2」，见 I1）、api-site-mgmt.md v1.0→v1.1（被依赖登记）、PageSpec admin/readonly v1.0→v1.1、枚举池 2 行取值扩展、INDEX.md、doc-center.json、knowledge/design-decisions/index.md（+ADR-028/029）
**增量内容**：ADR-028 面板域名绑定强校验（S-07-03/FR-17/bind_domain/HostGuard/D-P12/D-P16/I-P09）+ ADR-029 面板自身证书 ACME 经已有 nginx（S-08-03/FR-18/panel_cert.source+acme/cert 三接口/D-P13~D-P15/I-P10/子流程 4.1.3/迁移 007/site-mgmt 跨模块消费契约）

## 检查范围（增量口径）

变更项正确性：编号接力 / 枚举池同步 / DDL-文档一致 / Action-API 对齐 / 变更记录完整；存量部分抽查回归（S7 机械 diff 复跑 + 存量项核对）。全部结论基于本次实读文件与实跑命令，逐项给出证据。

## 一、增量五项硬检查

| # | 检查项 | 结果 | 证据（本次实读/实跑） |
|---|--------|------|------|
| 1 | **编号接力** | **PASS** | ① L4/小节：BA 2.2.1.3（business-arch-panel-core.md:97）、2.2.2.3（:113）；AA 2.1.2.3/S-07-03（application-arch-panel-core.md:143）、2.1.3.3/S-08-03（:187）——`grep -n "^##### "` 实跑 18 个 L4 小节连续无跳号；S-07-03/S-08-03 全 workspace grep 仅命中 panel-core 相关 5 文件（INDEX/AA/FL 分册/FL 聚合/PRD），无跨模块冲突。② PRD：FR-17/FR-18 接力 FR-01~16（grep 实跑全集无缺口无重复）；待对齐-6/-7 接力 -1~-5（prd:177-183）。③ API：`grep -o "PANEL_0[0-9][0-9]\|D-P[0-9][0-9]\|I-P[0-9][0-9]" \| sort -u` 实跑 = PANEL_001~011、D-P01~D-P16、I-P01~I-P10 全连续；SQL 清单 1~7；错误码语义扩展（PANEL_003 +acme 分支）在错误表与 changelog 双注明。④ 迁移：007_panel_core_acme.sql 接力既有 001~006——`ls backend/internal/store/migrations/` 实跑确认 001_account_auth~006_cron_backup 六文件在位（007 属 B 阶段增量代码，设计先行正确）。⑤ ADR-028/029 接力 ADR-027（design-decisions/index.md:37-38）。⑥ PageSpec Card 1~8 顺延重编（admin:95-184 实读连续）。⑦ 审计动作常量 +3（panel_cert_requested/panel_cert_renewed/panel_cert_renewal_retried）无与存量（panel_config_changed/panel_cert_updated/panel_entry_path_regenerated 及 site-mgmt cert_* 族）冲突 |
| 2 | **枚举池同步** | **PASS**（附 W2） | 池侧：https_mode +acme（04-business-enum.md:31，注「增量 D3 2026-11-16 取值扩展」）、panel_cert_source +acme（:33，注「增量 D2 2026-11-16」且明示废止旧「无 acme 取值」口径）；cert_status 为既有池条目（:14，5 值，网站管理域）。引用侧三方核对：API 3.4 枚举表（https_mode 四态/panel_cert_source 三态/cert_status 标注「引用、4 值子集呈现」）↔ DDL CHECK（5.3：https_mode IN 4 值、source IN 3 值）↔ 双 PageSpec 字典（admin 第 3 节 +acme、+cert_status 4 值并注「池定义 5 值本模块 4 值子集——同步终态无 pending」）逐字一致；readonly:37 清单滞后见 W4。S8 复跑：池标识符 `awk+sort+uniq -d` 实跑无重复。布尔口径（entry_path_enabled/update_check 非 池枚举）三文档一致沿用存量惯例 |
| 3 | **DDL-文档一致** | **PASS** | api 4.1.1 字段表 ↔ 5.3 DDL 逐列核对：panel_config 新增 bind_domain TEXT NULL（无 NOT NULL，语义=可空未绑定，I-P09）✓、https_mode CHECK 扩 4 值 ✓；panel_cert source CHECK 扩 3 值 ✓、新增 last_error TEXT NULL ✓；DDL CHECK 取值与枚举池逐字一致 ✓。domain-model 存储映射说明（:343）与 DDL 方向一致（panel_config 增列/source 扩值），行内枚举滞后见 W2。迁移 007 描述（ALTER 加列 + CHECK 变更走表重建、语义以 5.3 为唯一事实源）与 DDL 自洽；存量 002_panel_core.sql 实读=v1.0 形态未动（增量代码阶段待落，符合设计先行） |
| 4 | **Action-API 对齐**（CT-001/CT-004/G1-11，S7 机械 diff 复跑） | **PASS** | 实跑 python 机械 diff（正则抽取三方路径+方法）：API 4.1.4 接口表 10 条 ↔ admin PageSpec Action 表 10 条——**双向零差**（API→admin 无 MISSING、admin→API 无 FABRICATED、方法逐行一致）；readonly 4 只读 Action（configDetail/certStatus/menuList/versionGet）均 为 admin 真子集且方法一致；负空间 6（configUpdate/entryPathRegenerate/certUpload/certRequest/certRetryRenewal/updateCheckRun）+ 可见 4 = admin 全集 10（集合相等实跑 True）。权限对齐：cert/status=GET admin+readonly（api 3.3 矩阵 :116 ↔ admin Action :32 ↔ readonly :29 三方一致），cert/request·retry-renewal=admin 专属（api :117-118 ↔ readonly 负空间）；域名绑定/解绑无独立接口=无独立 Action（D-P12 单写入口，admin S7 注记明示）。CT-002 字段抽查：detail data +bindDomain 组、cert/status data（source/status/lastError/acme.domain/challenge/autoRenew）与 Card 3/5 字段一一对应，无臆造字段 |
| 5 | **变更记录完整**（G1-09） | **PASS**（附 W1） | 11 份触达文件 changelog 逐份核对均有本轮行：BA v1.1/AA v1.1/FL v1.1（分册）、business-arch.md v2.1（核验零改动说明）/function-list.md v2.1（同步说明）、prd v1.1+v1.2、domain-model v1.16、api v2.0（interface_added 等五类变更类型+目标节号 18 节）、api-site-mgmt v1.1（rule_added，声明 HTTP 接口/表/枚举本体零变更）、PageSpec admin/readonly v1.1。GL-001a 实跑：全部文档头「最后更新 2026-11-16」≥ changelog 最新行日期 ✓。配套登记：INDEX.md 三 stage note 增量重写、doc-center.json 再生成（_generated.version 3）、ADR-028/029 入 knowledge 索引。W1：prd 头版本 v1.1 滞后 changelog v1.2 行 |

## 二、存量抽查回归

| # | 回归项 | 结果 | 证据 |
|---|--------|------|------|
| 1 | 存量 7 接口不动 | PASS | api 4.1.4 表实读：v1.0 7 路径/方法/权限逐行保留（config/detail GET 双角色、config/update·regenerate-entry-path·upload-certificate POST admin、menu/list·system/version GET 双角色、system/update-check POST admin）；「7 个 HTTP 接口」字样仅存于 v1.0 历史 changelog 行（:365） |
| 2 | 存量枚举不动 | PASS | 枚举池 diff 实看：仅 https_mode/panel_cert_source 两行变更，listen_scope/config_effect_mode 及其余 20+ 行零改动；API 3.4 listen_scope/config_effect_mode 行取值未变 |
| 3 | 存量 SQL/错误码语义 | PASS | SQL 1~3 原语义保留（行 2 注 HostGuard 读态、行 3 注可含 bind_domain 均为注释性扩展）；行 4/5 扩 last_error 列有 changelog 依据；PANEL_001~006 文案未动（003 为注明式扩展） |
| 4 | 领域模型分类不变 | PASS | domain-model v1.16 changelog 自述+实读：仍 1 聚合根（panel_config）+1 实体（HTTPS 证书）+1 值对象（配置生效分类），对象分类零变更，仅字段/子流程/集成行增量；DDD 归类复核结论（支撑业务域，panel_cert 单例对象非多实例状态机）在 AA v1.1 归类段落明示且判定自洽 |
| 5 | readonly 视图回归 | PASS | 原 3 只读 Action 保留+certStatus 新增；原 4 负空间保留+2 扩展（6 项与 api 3.3 矩阵 ✗ 项逐一对应实跑）；8 Card 差异表覆盖原 6 Card 全部行（差异语义未变） |
| 6 | 上轮 Gate 1 遗留建议处置 | **未修复（延续）** | 上轮 W1「建议但强制」笔误 3 处仍在：grep 实跑命中 business-arch-panel-core.md:13 / function-list-panel-core.md:13 / prd-panel-core.md:104（→本轮 W6）；上轮 W2 菜单统计口径延续（→本轮 W7） |
| 7 | 上轮 Gate 1 S7 复跑 | PASS | 本轮机械 diff 以 v2.0 全量 10 接口重跑（见上表第 4 项），存量 7 接口映射包含其中 |

## 三、G1-01~G1-11 增量口径结论

| 编号 | 检查项 | 结果 | 增量依据 |
|------|--------|------|---------|
| G1-01 | 架构文档与模块设计一致 | PASS | AA/BA/FL v1.1 相互引用版本号同步（AA:12 引 BA v1.1、FL:12 引 AA v1.1）；PRD/api/PageSpec 输入行均升引 v1.1/v1.16/v2.0；聚合视图 v2.1 两份零改动/纯同步说明与分册 v1.1 实际变更吻合（L4 低于三秩截断，实跑比对 rollup 树已并入 2 新 L4） |
| G1-02 | PRD 覆盖场景与角色 | PASS | FR-17/18 ↔ FL 2 新菜单行 ×2 角色 ↔ S-07-03/S-08-03 三方映射；readonly 边界（ACME 申请/重试不可见）PRD 第三部分/FL 只读行/api 权限矩阵三口径一致；OUT +2（DNS-01 二期、独立 80 端口不做）有 ADR-029 依据；完备性增量复核块在文（G 5/5、T 适用 3/3——T6 转受限适用论证成立：经 site-mgmt ACL 复用不直连 CA） |
| G1-03 | API 设计完整 | PASS | 3 新接口路径/req/data/校验链/审计口径齐备（4.1.4 详情逐条实读）；PANEL_007~011 覆盖 PRD N2 新增 4 异常分支（404 两分支为标准 404 无业务码、nginx 空态→008、ACME 失败→009）+防重 011/状态 010；业务常量表（ACME_TIMEOUT/CERT_EXPIRING_DAYS/ACME_CHALLENGE）与配置项 acme.directory_url 归属（site-mgmt 治理）落位 |
| G1-04 | 交互设计覆盖页面类型 | PASS | Card 3/5 新增覆盖 FL 两新菜单；Card 重排后 8 Card 全集=FL 设置页菜单面；负空间/只读差异同步；机制型（HostGuard 拦截、自动续期调度）经 AA 附录A 机制侧映射行+PageSpec 第 7 节框架行为注记覆盖，无接口即无 Action 口径维持 |
| G1-05 | API 字段与交互规格字段对齐 | PASS | bindDomain（detail 响应组/update req 字段/Card 3 输入与状态展示）、certStatus data（Card 5 全元素）、lastError（NAlert 条件渲染）、https_mode 四态（下拉+tag 色）逐项对应；PANEL_007~011 前端文案在 admin 第 1 节错误处理行全集登记 |
| G1-06 | 状态机定义完整 | PASS | 「无生命周期状态机」结论经增量复核维持：ACME 状态=读时推算简化标注（D-P13，PRD 第六部分新增辨析行），非多实例状态机——与 site-mgmt certificate 聚合根的差异论证在 AA 归类段/domain-model 实体行/api 4.1.2 三处同口径 |
| G1-07 | 枚举统一无冲突 | PASS（附 W2/W4） | 见硬检查 #2；cert_status 跨域引用（GL-028：指向网站管理池条目）标注合规；4 值子集呈现三处注明依据（同步终态无 pending） |
| G1-08 | 接口路径规范 | PASS | 3 新接口均 /panel/{cert}/{action} 三段式；资源集扩为 config/menu/system/cert（3.1 规范行同步） |
| G1-09 | changelog 已填写 | PASS（附 W1） | 见硬检查 #5 |
| G1-10 | 状态索引已更新 | PASS | INDEX.md panel-core requirement/api/interaction 三 note 增量重写实读在位（含各阶段版本与计数）；gate1 note 由本次追加增量复检结果；doc-center.json 再生成 |
| G1-11 | 跨阶段模型一致性 | PASS（附 W5） | 领域对象：domain-model（:201-202 +子流程 :253 +集成行）↔ api 1.4/2.1/4.1.1 ↔ 存储映射（:343）对象集一致（1+1+1+1 子流程）；Action↔API 机械 diff 见硬检查 #4；跨模块消费契约五点闭环：AA 附录B 表 ↔ domain-model 第六部分 OHS 行 ↔ api 1.5/5.1 消费表 ↔ api-site-mgmt 1.5 被依赖行+5.1 OHS 行（双向互引 1.5/5.1 实读一致，AcmeGateway Issue/Renew 通道名两侧同值）；W5 为子流程输入列措辞精度 |

## 四、WARN / INFO 清单

**WARN（供确认，均非 CRITICAL、不阻断增量进入代码阶段）**

- **W1（版本元数据）** prd-panel-core.md:5 头部「版本 v1.1」滞后于更新记录最新行 v1.2（:256，增量 D3 联动行）；连带 api-panel-core.md:8 输入行仍引「prd-panel-core.md v1.1」。建议头部与引用升 v1.2（changelog 内容本身完整自洽）。
- **W2（枚举回填滞后）** domain-model.md:201 panel_config 行内枚举清单仍为 `https_mode{off,self_signed,imported}`，未随 D3 扩展回填 acme（池 :31 与 DDL 均已四态）；:343 存储映射说明仍以「归 D3 DDL 修订，prd 待对齐-6/-7」未关闭措辞表述（prd v1.2 已关闭待对齐-6/-7）。属分阶段写作的显式延期（v1.16 为 D2 增量、https_mode 扩值在 D3 落地），但工作区终态下行内清单与池不一致，建议下次触达时回填。
- **W3（重编号残留引用）** admin PageSpec:206（about 页说明）「该动作在设置页 Card 6」——更新检查 Card 已顺延为 Card 8（:184），:237/:245 等处均已改而此一处漏改。
- **W4（字典清单滞后）** readonly PageSpec:37 第 3 节「与 admin 文档第 3 节完全一致：https_mode/listen_scope/panel_cert_source/config_effect_mode」未列 cert_status，与其自身 :81（第 8 节第 7 项）及 v1.1 changelog「字典同源扩展（cert_status 引用…）」不一致；admin 侧为 5 键（admin §8-2 实读 5/5）。
- **W5（D2 行文精度）** domain-model.md:253 子流程输入列「panel_cert（绑定域名、挑战方式固定 http_01、ACME 账户引用）」沿 site-mgmt 同构行式（:252）复制，但 D3 定稿契约中 bind_domain 属 panel_config（api 4.1.3 输入=panel_cert.not_after + panel_config.bind_domain）、panel_cert 不另存账户引用（D-P13，api:41）。语义方向一致、字段归属措辞不符，建议措辞对齐。
- **W6（上轮 W1 延续）** R-10 口径笔误「建议但强制」×3 处未修复：business-arch-panel-core.md:13、function-list-panel-core.md:13、prd-panel-core.md:104。
- **W7（上轮 W2 延续）** 菜单行统计口径仍不可机械复现：prd v1.1 称「12 菜单行 + 6 机制型」而 FL admin 章实读为 9 菜单行+2 无独立菜单行（11 节）。增量 delta 本身正确（+2 菜单/+4 物理行/+2 功能子模块=18，FL changelog 口径可机械验证），绝对数的计数基准建议在 FL 附录明示。

**INFO（记录在案）**

- **I1** 任务口径「api-panel-core.md v1.2」与实际不符：api-panel-core.md 头部与 changelog 实读为 v1.0→v2.0（无 v1.2 行）；「v1.2」为 prd-panel-core.md 变更记录行。产物内部自洽，不影响判定。
- **I2** doc-center.json fishbone 的 panel_config desc 被生成器截断（「…v1.16 增量 AD…」）——自动生成产物，重生成即复现，不计缺陷。
- **I3** 存量代码（backend 002_panel_core.sql / 7 接口实现 / 前端 6 Card）仍为 v1.0 形态——属「增量代码阶段」待办（INDEX.md requirement note 已声明），Gate 1（设计→代码门控）通过是进入该阶段的前提，方向正确。

## 五、总结

**判定：WARN**

- 增量五项硬检查（编号接力 / 枚举池同步 / DDL-文档一致 / Action-API 对齐 / 变更记录完整）全部通过，无 CRITICAL 失败；
- S7 机械 diff 复跑通过（API 10 ↔ admin 10 双向零差、readonly 4+6=10 全覆盖、方法/权限逐行一致）；
- 7 项 WARN 均为文档元数据/交叉引用/行文精度级（W1~W5 本轮增量引入 4 项 + 上轮遗留延续 2 项 + D2 措辞 1 项），不影响契约与实现语义，建议在增量代码阶段并行小修（W3/W4/W6 为三处单行级修正）；
- 确认后可继续增量 B/F 阶段（WARN 确认记录即本报告第四节）。
