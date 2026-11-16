# Gate 1 验证报告 — 设计→代码门控（site-mgmt）

**模块**：网站管理（site-mgmt，核心卖点模块，本项目首个核心域）
**检查时间**：2026-11-12
**判定标准**：validation skill（engine 0.5.2）+ design skill S1~S8 预检；产物质量编号唯一来源 = `.claude/rules/engine-rules.md` G1-01~G1-11
**判定规则**：S1~S6 任一未过=FAIL；仅 S7~S8 未过=WARN；全过=PASS
**检查对象**（7 份产物 + 枚举池 + INDEX）：

- `00-architecture/business-arch-site-mgmt.md` v1.0、`application-arch-site-mgmt.md` v1.0、`function-list-site-mgmt.md` v1.0
- `02-detailed/prd/prd-site-mgmt.md` v1.0、`02-detailed/prd/domain-model.md` v1.8（site-mgmt 段落：第一部分 :70-85、清单行 :96/:122-126、子流程 :140/:147、关联 :160）
- `02-detailed/api/api-site-mgmt.md` v1.0
- `02-detailed/interaction/admin/site-mgmt/site-mgmt.md` v1.0、`02-detailed/interaction/readonly/site-mgmt/site-mgmt.md` v1.0
- `01-solution/04-business-enum.md`（本模块 6 枚举行 :10-14、:42）
- `INDEX.md`（site-mgmt 节 :83-94）

**执行环境说明**：本工作区无激活技术栈（ACTIVE_STACKS 为空，api-site-mgmt.md:9 自述）——CH/FE/FX 清单中栈特定条目（Scm* 组件、dict/batch、CRUD+options 等 java/vue3-engine 约定）按前五模块（account-auth/panel-core/monitoring/service-mgmt/file-manager）Gate 1 同口径执行：检查其在本项目通用 REST 契约下的等价物与模块内自洽性。

---

## 逐项检查（S1~S8 → G1-01~G1-11 映射）

### S1 三文档全局检查 → G1-01 ✅ PASS

**BA 内容完整性**（business-arch-site-mgmt.md）：
- L1-L4 层级完整：5 L2（:71/:95/:115/:167/:187）→ 9 L3 → 27 L4，逐层有「所属 L2/L3」标注；机械计数 27 = 更新记录自述「27 业务活动」（:339）✓
- 格式合规：第一部分概述（:9-53，含 1.1 背景/1.2 定位/1.3 层级定义/1.4 范围+OUT 边界声明 :53）→ 第二部分（本架构概览 :59 + 每 L2/L3/L4 业务说明）→ 第三部分架构树（:239-282，与第二部分逐项一致，27 节点核对无出入）→ 附录A L3 依赖表（:290-307，15 行四列格式合规）+ 附录B 术语表（:312-333，三列合规）→ 更新记录表格（:337-339）✓
- BA 不含用户类型（:35 定位声明「不标注用户类型」，正文抽查无用户类型词）✓

**AA 内容完整性**（application-arch-site-mgmt.md）：
- 应用域分层表四行齐（:60-65），基础数据域/平台支撑域为空且逐条注明判定理由（:19-20、:64-65）——DDD 四层归类结论含逐条判定依据（:15-20）✓
- 9 个 BA L3 → 9 功能模块一一对应（C-07 站点运行操作 :71、C-08 :115、C-09 :161、C-10 :194、S-26 :238、S-27 :290、S-28 :335、S-29 :405、S-30 :425）；27 个 L4 → 27 子模块一一对应 ✓
- 每个功能模块/子模块：功能说明 + 用户类型复选框（admin/readonly 两角色逐一核对，机制型子模块两角色留空并注明「系统内部机制」如 :171/:204/:242）+ 最末一级调用关系表或「无调用」声明（:83-89/:101/:220/:252 等）✓
- 第三部分应用架构树（:471-515）与第二部分逐项一致；编码衔接声明（:24，C-07 起编承接 C-01~C-06/S-01~S-25，符合「域内最大序号后递增」规则）✓

**FL 内容完整性**（function-list-site-mgmt.md）：
- 按用户类型分章（2.1 admin :42、2.2 readonly :220）+ 全局行为说明（:40，覆盖 10 个机制型子模块）✓
- 菜单表格四列（菜单名称/功能点描述/所属功能子模块/备注）；admin 19 菜单行机械计数 ✓（启停 2+日志 2+重载 1+证书 5+建站 3+站点 5+源码 1）；readonly 5 菜单行 ✓；每行「所属功能子模块」可追溯 AA 编码 ✓
- 第三部分功能视图（:281-350）与第二部分一致；附录A 用户类型功能对照表（:354-367）与 AA 权限标注逐项一致（查看 4 项两角色 ✓/变更 7 项 admin ✓，依据 ADR-018/D0）✓

**跨文档一致性**：BA 9 L3 → AA 9 模块全覆盖；27 L4 → 27 子模块全覆盖（10 机制型经 AA 附录A 映射表 :519-529 + FL 全局行为 :40 覆盖）；用户类型名称 AA/FL 一致（admin/readonly）；A13~A17 口径三文档同步（BA :22-28 / AA 附录B :542-555 / FL 附录B :369-381）✓

---

### S2 PRD 完备性 → G1-02 / G1-06 ✅ PASS

PRD 附录自带完备性检查报告（prd-site-mgmt.md:312-359），本次逐项复核其证据：

| 层级 | 项 | 复核结果 | 证据 |
|------|----|---------|------|
| 第一层 | G1 业务目标 | ✅ | :11-15 解决什么（存量 nginx 站点接管与治理）/为谁（admin 变更、readonly 查看）/核心卖点与改不坏承诺落法 |
| 第一层 | G2 功能范围 | ✅ | :19-49 IN 27 项逐项标注 D1 AA 子模块依据；:53-65 OUT 11 项逐项标注归属（含二期/panel-core 归属） |
| 第一层 | G3 角色定义 | ✅ | :69-74 两角色职责+获得方式+访客不涉及声明+审计口径（变更留痕/查看不留） |
| 第一层 | G4 领域对象 | ✅ | domain-model.md :122-126（certificate 聚合根 + site 实体 + 3 值对象），判定论证 :82-85 |
| 第一层 | G5 对象关系 | ✅ | domain-model.md「与上级关系」列 + 1 子流程 I/O 契约（:140）+ 关联领域对象=无声明（:160） |
| 第二层 | T1/T1a/T2/T4/T6/T7/T8 | ✅ | T1=领域对象清单字段列；T1a=:231-237（现阶段无对外契约+消费方视角 3 链）；T2=第六部分 2 状态机+8 项辨析（:181-216）；T4=第七部分四查询形态（:220-228）；T6/T7=第九部分 ACME ACL 行（:274）；T8=N3 幂等语义（:264） |
| 第二层 | T3/T5/T9 N/A | ✅ 有据 | T3=无审批（:216 即时执行+pending 非）；T5=无导出（:229）；T9=非工具型（:339） |
| 第三层 | N1-N4 | ✅ | N1 锁定项+10 未锁定项显式移交（:241）；N2=16 异常分支（:245-262）；N3=20 规则+幂等（:152-176/:264）；N4=零台账+审计+conf.backup 历史载体（:266） |

**G1-06 状态机完整性**：certificate 六状态机（ADR-024）8 条流转齐（当前/目标/触发/备注四列，:183-193）+ imported 入册特例（:194）；site enabled ⇄ disabled 2 条流转（:198-201）；辨析表 8 项防伪状态机（:205-214）；「6 状态」vs 枚举命名 5 状态的计数差异显式登记（:207，待 ADR-024 侧澄清，不阻塞）✓
**覆盖性抽查**：FR-01~27 与 AA 27 子模块一一对应（PRD 第四部分各表编号含 AA 编码）；FL admin 19 菜单行均由对应 FR 承载 ✓

---

### S3 领域模型分离 → G1-06 / G1-11 ✅ PASS

5 类对象全分类、聚合根已确认（domain-model.md site-mgmt 段落）：

- **本域判定**：范围明确跳过战略设计 Step 1~3（:72，PRD 标注模块名）；核心域判定+依据（:77，本项目首个核心域）✓
- **聚合根 certificate**（:82）：A1-A4 判定 4/4（一致性边界/全局引用/生命周期入口/状态驱动）+ 落 SQLite 理由（续期任务状态不可从配置文件推导）✓
- **域内对象 site**（:83）：A1-A4 全不通过判定 + 实体（非值对象）理由（标识可寻址+属性可变，与 systemd_service/file_entry 同构）+ 零站点台账 D2 判定 ✓
- **值对象 ×3**：upstream（:84，D0 事项关闭）/Raw 块/写回落点映射（:85）✓
- **子流程**：自动续期任务 I/O 契约五列表（:140，触发/输入/输出/写入对象）✓
- **关联领域对象**：无，含理由（同域引用+本机资源+外部系统经 ACL 非 业务域聚合根，:160）✓
- **过程数据前置过滤**：PRD 第九部分分类表 10 行（:282-293，P1~P7 类别标注）✓
- 业务域清单行（:96）与领域对象清单行（:122-126）齐全，存储映射经 v1.8 D3 回填（:212）✓

---

### S4 API 模板合规（M1~M5+GL）→ G1-03 / G1-08 ✅ PASS

**机械自检表**（本次实际执行）：

| # | 检查项 | 结果 | 证据 |
|---|--------|------|------|
| M1 | 章节结构完整 | ✅ | 1.x（5 小节 :11-66）/3.x（4 小节 :68-185）/4.x（certificate 5 小节 :188-287 + site 5 小节 :289-386）/5.x（3 小节 :388-447）/6.x（3 小节 :449-473）齐；第 2 章裁剪有显式声明（:66「不适用——单聚合根」+ 1.4 裁剪依据 :54，符合 d3 裁剪矩阵「第 2 章仅多聚合根模块」） |
| M2 | 接口路径三段式 | ✅ | 18 接口全部 `/sites/{resource}/{action}`（资源：site/proxy/log/nginx/cert/php-pools）；本次以 for 循环 grep -Fc 逐路径计数核验 18/18 在 API 文档各出现 ≥2 次（3.3 矩阵+4.x 接口表/5.1 清单） |
| M3 | 枚举引用有定义 | ✅ | 8 枚举引用（site_type/site_enabled/cert_source/acme_challenge/cert_status/site_log_type + panel_role/audit_action/audit_result）全部在 04-business-enum.md 有定稿行（:10-14/:18/:22/:28/:42）；API 3.4 自述「引用无新增」与池状态一致 |
| M4 | 占位符残留 | ✅ | grep -c 实测：API 文档「按需填写」=0、TODO=0（7 份本模块产物同测全 0） |
| M5 | 变更记录 | ✅ | 6.1 变更记录表存在，v1.0 initial 行完整（:455，含接口/DDL/错误码/枚举/决策计数与「关闭待对齐-1~-10」） |

**GL 全局一致性（适用项）**：GL-001a 文档头日期 2026-11-12 ≥ changelog 最新行 2026-11-12 ✓；GL-001b 必填章节非空 ✓；GL-002 第 2 章跳过原因注明（:66）✓；GL-003 混合模式声明 D-13（:41）✓；GL-004 领域对象总览（:50）与 Ch4 一一对应（certificate→4.1、site/upstream/Raw/写回落点映射→4.2.1 字段清单内嵌、自动续期→4.1.3）✓；GL-009 路径全三段式 ✓；GL-010/011 HTTP 方法与响应格式模块内统一声明（3.1 :75-76，读 GET/写 POST 与 account-auth D-07 同一约定，无激活栈下按项目级契约判定）✓；GL-012 写接口全部含权限校验（3.3 矩阵 12 写接口 admin+CSRF 逐一标注 :104-117）✓；错误码完整性 G1-03：21 新增（SITE_001~014/CERT_001~007）+ AUTH 族复用（:123-151），每接口校验链含错误码引用 ✓。

**栈特定项等价判定**：GL-031（主数据 options）——certificate 为消费终模块无对外 OHS，options 不适用已在 1.4/:411 显式声明 ✓；GL-032（dict/batch）——API 无独立字典端点（18 路径清单实证无 dict 资源），前端常量偏差见 S6/I2 ✓。

---

### S5 API 变更摘要 → G1-09 ✅ PASS

6.1 变更记录（api-site-mgmt.md:451-455）：v1.0 · 2026-11-12 · `initial` · 结构化摘要（18 接口+1 表 DDL+4 网关契约+1 子流程+21 错误码+8 枚举引用+13 审计动作常量+16 项设计决策逐项点名）+「关闭 prd 待对齐-1~-10」+「无旧版对比」；6.2 变更类型词汇表（:457-469）与 6.3 版本号规则（:471-473）齐 ✓。D1/PRD/PageSpec 侧 changelog 亦均已填写（BA/AA/FL/PRD 更新记录各 v1.0 行；两 PageSpec 第 9 节 initial 行）——G1-09 全产物达标 ✓。

---

### S6 PageSpec 输出校验 → G1-04 / G1-05 ✅ PASS

**机械自检**（admin/readonly 两份，9 节结构齐）：模块元信息含唯一数据源声明（admin:9）与业务/页面类型判定依据（admin:18-19，master_data 子形态+hybrid/config/code 三型含理由）；Action 表 18 行含方法/路径/页面/权限/CSRF 五列（admin:28-47）；字典 Key 7 项同源池（admin:53-61）+ 颜色方案表（admin:65-78）；列表页 2 个（site-list 查询条件/10 列含宽度 fixed 配置 admin:94-111、cert-list admin:149-167）+ operList（admin:115-122/:179-186）+ 5 弹窗表单含字段/组件/校验/显隐（admin:127-139/:171-177）+ 详情 Drawer 6 Card（admin:208-223）；路由 3 条含 meta roles 守卫（admin:235-239）；第 8 节 12 项全局一致性自查含证据指向（admin:245-260）；readonly 负空间 14 Action 逐项对应（readonly:32）；版本变更记录 v1.0 initial（两份第 9 节）✓

**页面类型覆盖（G1-04）**：列表（site-list/cert-list）/详情（Drawer+行内 Modal）/表单（5 弹窗+Drawer 编辑态）全覆盖；hybrid（site-list Drawer+日志 Tab）/config（cert-list）/code（site-source Monaco）三型判定含依据；本模块无审批类页面与 API「无审批」判定一致（admin:231 诚实声明）✓

**字段对齐（G1-05）**：site-list 列字段 ⊆ API site/list items（siteId/serverNames/listen/root/enabled/certRef/certId/certStatus/proxy/sourceFile，API :352）；cert-list 列 ⊆ cert/list items（API :265）；Drawer 6 Card 字段 ⊆ site/detail 响应（API :353）；无臆造字段 ✓

**FX 全量清单执行方式声明**：FX-001~141 逐条清单文件未随 engine 0.5.2 payload 发布——本次 `find` 实证（payload 仅命中 java-microservice backend checklist，无前端清单文件），PageSpec 自述（admin:268）与事实相符；结构级+可机械执行项已按 9 节逐节落实，与前五模块 Gate 1 同口径，全量 FX 留待栈接入后重审（见 I3）。

---

### S7 跨阶段一致性 → G1-11 ✅ PASS（机械核验）

**API 领域对象 ↔ D2 领域模型**：API 1.4 总览（:50）certificate 聚合根/site 实体/upstream/Raw 块/写回落点映射值对象/自动续期子流程/关联领域对象=无——与 domain-model.md :96/:122-126/:140/:160 逐项一致；certificate 字段清单（API :196-211）与领域对象行核心业务字段（domain-model :122）对齐（deploy_prev_refs 为 API 侧机制列 D-11、关联站点引用改为实时推导，均有显式决策编号）；site 投影字段（API :295-309）与 domain-model :123 对齐（certId/certStatus 为 D-11 实时推导呈现增补）✓

**PageSpec Action ↔ API 接口**（for 循环 grep -Fc 逐路径三方计数实测）：18 接口在 API 文档、admin PageSpec 均命中（admin 每路径 ≥1）；readonly 仅命中 4 只读路径（site/list、site/detail、log/tail、cert/list），14 写路径计数=0——与 API 3.3 权限矩阵（查看类 admin+readonly/全部写 admin）**逐行吻合**；admin 18 Action 与 18 接口一一对应无遗漏无虚构（admin:49 自述与本次机械计数一致）✓

**菜单↔接口合并口径**（已文档化的 N:1，非失配）：FL C-08-01/02 两菜单 → cert/request 单接口（challenge 参数区分，API :257 + admin:49 注记）；S-27-01/02/03 三模板菜单 → site/create 单接口（type 参数，API :339）——两侧均有显式合并声明，可追溯 ✓

---

### S8 枚举池无冲突 → G1-07 ✅ PASS（机械核验）

- **池内无重复**：awk 提取 04-business-enum.md 全部枚举标识去重比对——33 个标识 0 重复（uniq -d 输出空）✓
- **本模块 6 枚举三方同值**：site_type/site_enabled/cert_source/acme_challenge/cert_status/site_log_type 在池（:10-14/:42，均「定稿 D2 2026-11-12」）与 API 3.4（:157-162）的取值串 grep -cF 逐字比对全等（如 `pending \| valid \| expiring_soon \| expired \| renew_failed` 池=1 api=1）；PageSpec 第 3 节（admin:55-61）取值同源 ✓
- **近义枚举无冲突**：cert_source{acme,imported}（网站管理）vs panel_cert_source{self_signed,imported}（面板基座，:32）标识不同、语义边界已在池注明 ✓
- **引用枚举指向正确**：panel_role/audit_action/audit_result 均指向 account-auth 定稿行（:18/:22/:28）✓

---

## G1-01~G1-11 汇总

| G1 编号 | 检查项 | 结果 | 承载检查 |
|---------|--------|------|---------|
| G1-01 | 架构文档与模块设计一致 | ✅ | S1（三文档联动 + 跨文档覆盖核验） |
| G1-02 | PRD 覆盖所有业务场景和角色 | ✅ | S2（G/T/N 三层复核 + FR↔子模块/菜单对应） |
| G1-03 | API 设计完整（路径/请求/响应/错误码） | ✅ | S4（18 接口逐接口详情 + 21 错误码 + 请求/响应/校验齐） |
| G1-04 | 交互设计覆盖所有页面类型 | ✅ | S6（列表/表单/详情/源码 code 型覆盖；本模块无审批页与无审批判定一致） |
| G1-05 | API 字段与交互规格字段对齐 | ✅ | S6（列表列/Drawer Card 字段 ⊆ API 响应，无臆造） |
| G1-06 | 状态机定义完整 | ✅ | S2/S3（certificate 8 流转 + site 2 流转 + 8 项辨析防伪） |
| G1-07 | 枚举统一无冲突 | ✅ | S8（33 标识唯一 + 6 枚举三方同值 + 近义枚举边界） |
| G1-08 | 接口路径 /domain/resource/action | ✅ | S4-M2（18/18 三段式逐路径计数） |
| G1-09 | 设计文档 changelog 已填写 | ✅ | S5（API/D1/PRD/PageSpec 全产物 initial 行） |
| G1-10 | 状态索引模块状态已更新 | ✅ | INDEX.md site-mgmt stages requirement/api/interaction=done（:86-88）；本次门控将 gates.gate1 置 pass |
| G1-11 | 跨阶段模型一致性 | ✅ | S7（领域对象↔D2 逐项 + Action↔接口 18/18 + readonly 4+14 负空间机械实证） |

---

## 建议项（INFO，不阻断）

| # | 事项 | 说明与建议 |
|---|------|-----------|
| I1 | api-site-mgmt.md:8 输入行引用「domain-model.md v1.7」而该文件现为 v1.8 | v1.8 为本模块 D3 定稿回填（domain-model.md:212，certificate 存储映射说明更新，不改分类/枚举/业务字段）——API 依赖的 site-mgmt 建模内容未变，仅元数据引用滞后；建议下次 API 版本迭代时同步为 v1.8 |
| I2 | PageSpec 字典数据源偏差（无 dict/batch，前端 enums.ts 常量） | admin:63 声明为设计决策「待 Gate 1 复核」——本次**复核接受**：7 枚举全固定值、04-business-enum.md 单一事实源、API 18 路径实证无 dict 端点；与 account-auth/panel-core/monitoring/service-mgmt/file-manager 五先例同判；接入栈 profile 后按 GL-032 重审 |
| I3 | FX-001~141 全量清单文件未随 engine 0.5.2 payload 发布 | find 实证（payload 仅 java-microservice 后端清单）；本次已执行结构级 9 节 + 机械项核验，与前五模块同口径；全量 FX 待清单发布或栈接入后补跑 |
| I4 | ADR-024 文本「6 状态」与枚举命名 5 状态的计数差异 | 已在 PRD 第六部分（:207）/AA 附录B（:553）/domain-model（:122 备注）/枚举池（:14）四方登记并统一按 5 命名状态执行；待 ADR-024 侧澄清——如确有第 6 中间态需修订 ADR-024 并同步三文档+枚举池（触发 S8 重审） |
| I5 | FL 菜单→接口 N:1 合并口径 | cert/request 覆盖 C-08-01/02、site/create 覆盖 S-27-01/02/03（参数区分模板/挑战方式）——API 与 PageSpec 两侧均有显式合并声明可追溯；二期若两挑战/三模板交互分化需评估拆分接口 |

---

## 总结

**判定：PASS**

- S1~S8 全部通过（S1 三文档 / S2 PRD / S3 领域模型 / S4 API M1~M5+GL / S5 变更摘要 / S6 PageSpec / S7 跨阶段一致性 / S8 枚举池），G1-01~G1-11 全过，CRITICAL 无失败
- 机械实证项：M2 18/18 三段式逐路径计数、M4 7 产物占位符全 0、S7 Action↔接口 18/18 三方计数 + readonly 4 读/14 负空间吻合、S8 池 33 标识 0 重复 + 6 枚举取值串池↔API 逐字全等
- 5 项 INFO 建议记录在案（I1 元数据引用滞后、I2 字典偏差复核接受、I3 FX 全量留栈后、I4 ADR-024 计数差异已登记、I5 菜单合并口径），均不阻断进入代码生成阶段
- 可进入 B1（后端代码生成）；I4 如触发 ADR-024 修订需回流水线同步三文档与枚举池
