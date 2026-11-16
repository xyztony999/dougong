# Gate 1 验证报告 — 设计→代码门控（三批合并增量：site-mgmt 增量 + env-mgmt 首检 + panel-core/account-auth 菜单 IA 增量）

**模块**：site-mgmt（增量）/ env-mgmt（新模块首检）/ panel-core + account-auth（菜单 IA 增量）
**检查时间**：2026-11-16（/pipeline 增量轮，session 轨）
**基线版本**：api-site-mgmt.md v1.2 · prd-site-mgmt.md v1.1 · api-env-mgmt.md v1.0 · prd-env-mgmt.md v1.0 · api-panel-core.md v2.2 · PageSpec：site-mgmt v1.1×2 / env-mgmt v1.0×2 / panel-core v1.2×2 / account-auth v1.3+v1.1
**输入来源**：v0.1.2 用户实测反馈 7 项（A 批 1-3=site-mgmt、A 批 4=env-mgmt、B 批 5-7=panel-core/account-auth）

## 一、待用户复核的既定拍板（本轮 Gate 1 呈现项，用户仍可改判）

### P1. ⚠️A18 站点列表聚合（反馈第 1 项，上轮 D1 拍板 + 本轮 D3 定稿）

- 口径：**同来源文件（sourceFile 相同）且 server_name 全集相同**的多个 server 块聚合为一个列表条目——监听合并显示（如 `80 / 443 ssl`）、enabled=任一块生效、证书状态=组内最优（443/ssl 块优先）；条目携带 blocks[] 子清单保留每块 siteId。
- 边界：跨文件或域名集不同**不聚合**；全部写接口与 detail 仍单 siteId 寻址不变（**仅展示层聚合，数据模型与 siteId 寻址零变化**，domain-model v1.18 判定）；组级启停=前端对 blocks 顺序逐块调用。
- 契约落点：api-site-mgmt v1.2 D-17 + site/list 响应结构变更（items 无顶层 siteId，寻址经 blocks[].siteId）。

### P2. 「文件验证」语义 —— **Gate 1 用户已改判（2026-11-16）：DNS-01 纳入本轮，DNS API 自动化口径**

- **维持部分（⚠️A19）**：「文件验证」= HTTP-01 更名对齐宝塔称谓 + 三级自动落点策略（root 直写→复用 location→临时注入兜底，D-19）——不变。
- **改判部分（⚠️A20，已落盘设计增量 v1.2/v2.0）**：DNS-01 经 **DNS 服务商 API 自动化**——面板存储 API 凭证（AES-256-GCM 密文+独立密钥文件 dns-cred.key 0600，沿 database-mgmt CredentialVault 先例，明文永不回显、审计脱敏、admin 专属），签发/续期自动 TXT 添加→验证→签发→清理；支持泛域名（cert/request 扩展 domains 手动输入 `*.domain`）与自动续期（证书行落 dns_credential_id，凭证缺失=续期失败 last_error 如实）；R-09 例外语义与 ACME CA 同判。
- **服务商清单（呈请复核）**：Cloudflare + 阿里云 DNS + 腾讯云 DNSPod——理由：三家覆盖国内外主流场景，且均为 lego v4 官方内置 provider（适配成本最低、零自研协议实现），适配器注册表可扩展（后续新增服务商=映射表+provider 注册）。
- **手动 TXT：OUT**（实现成本评估结论）：手动 TXT 每次续期需人工在服务商控制台操作，与「自动续期」承诺互斥；DNS 场景一律走 API 自动化，不如不做半自动形态。
- 设计落点：BA/AA/FL v1.2（C-08-04/05、业务活动 28→30）+ prd v1.2（FR-29/FR-30）+ domain-model v1.20（**新增聚合根 dns_credential**，site-mgmt 变双聚合根）+ 枚举 acme_challenge +dns_01 / dns_provider 新增 + api v2.0（接口 20→23、第 2 章跨聚合根设计启用、迁移 009）+ PageSpec v1.2×2（申请弹窗+凭证管理弹窗）。

### P2′. DNS-01 增量机械复检（改判后补跑，2026-11-16）

| 校验 | 结果 |
|------|------|
| S7：api 23 路径 ↔ admin PageSpec 23 Action | ✅ 双侧 sort diff **IDENTICAL**（含 /sites/dns-credential/{list,create,delete}） |
| S7：readonly 4 读 + 19 负空间 = 23 全集 | ✅（dnsCred×3 新增负空间行在位） |
| S8：acme_challenge 扩值 dns_01 / dns_provider 三方同值 | ✅ 池 ↔ api（3.4 表+DDL CHECK）↔ PageSpec（8 Key）逐字一致 |
| 编号接力 | ✅ FR-29/30（FR-28 后）、C-08-04/05、D-20（D-19 后）、CERT_009/010、迁移 009、⚠️A20（A19 后）、动作码 +dns_credential_* |
| DDL↔字段清单 | ✅ dns_credential 表 6 列 ↔ 4.3.1 字段清单；certificate +dns_credential_id 列（I-01 组装约束同步） |
| 跨聚合根一致性 | ✅ 第 2 章 I-C01/C02 与 4.3/凭证 delete 详情/续期输入三处互证 |
| doccenter | ✅ 已再生成（聚合根清单变更：site-mgmt 双聚合根） |
| 占位符 | ✅ 0 残留 |

### 增量 WARN（DNS-01 新增，呈请复核）

| # | 项 | 说明 |
|---|----|------|
| W6 | cert/request 请求结构扩展 | siteId 变可选 + domains/dnsCredentialId 新参（interface_changed 已登记）——泛域名与无站点场景的必要扩展；同轮前后端切换 |
| W7 | 凭证无 update 接口 | 轮换=删除重建（MVP 口径）；如需在位轮换二期扩展 |
| W8 | 凭证有效性不预校验 | 创建时仅形态校验（不外呼验证——避免创建动作外呼 R-09 纪律），有效性在签发时经服务商 API 验证（校验态不缓存） |

### P3. ⚠️A1~A7 环境管理边界（反馈第 4 项，上轮 D0 拍板 + 本轮 D2-D3 定稿）

- 六环境目录：nginx / Apache / Docker / MySQL / Redis / PHP-FPM（编译期静态清单，扩展走面板版本迭代）。
- 双包管理器系：apt（Debian 系）+ dnf·yum（RHEL 系，dnf 无则回退 yum），运行时探测择一；需第三方官方仓库的环境（典型 RHEL 系 Docker CE）由面板写入源配置并审计。
- 异步任务：pending → running → succeeded / failed；同环境互斥；取消不做（包管理器中断态不安全）；任务台账落 SQLite（跨重启留存；重启恢复 running→failed 如实呈现）。
- 升级=重装仓库最新版（幂等，先刷新索引）；可升级检测二期 OUT。
- 卸载=包 remove（保留配置与数据），purge 二期 OUT；强确认=两段警示（本域服务运行实况 + 关联模块静态影响文案）+ 输入环境名锁定——**动态跨模块计数（容器数/实例数/站点数）显式 OUT-9**（BA ⚠️A6「列出警示」按此口径收窄落地，见 W3）。
- 安装后不强制 start/enable（尊重发行版包脚本默认）。
- 包名/单元名/版本查询=编译期映射表（Debian：nginx/apache2/docker.io/mysql-server/redis-server/php-fpm；RHEL 系：nginx/httpd/docker-ce/mysql-server/redis/php-fpm 等）。

### P4. B 批 IA 方案（反馈第 5-7 项，本轮工作假设——**推荐方案一，未经用户逐项选择**）

> 说明：本轮执行计划确认时 AskUserQuestion 因等待超时被打断，按计划确认指令推进推荐方案（方案一）；PageSpec 层变更成本低，用户改判可快速重做。

- **顶栏（第 5 项）**：标题=当前页面标题（路由 meta.title 优先、菜单 title 兜底），替换 MainLayout 硬编码「账户与安全基座」（内部业务域显示名泄漏）；用户名/角色/登出区不变。根因定位：MainLayout.vue:182。
- **用户管理入口（第 6 项）**：侧栏新增「用户与安全」分组——用户管理 /users、在线会话 /sessions、审计日志 /audit-logs、个人资料 /profile（两角色可达；页面内操作差异按 ADR-018 负空间）。根因：设计侧 PageSpec account-auth 第 7 节本有菜单四项，实现侧 ModuleMenuRegistry（main.go）未登记 account-auth 条目致不可达——本增量=实现回归设计 + 分组升格（api-panel-core v2.2 D-P18，MenuItem.children 启用）。
- **设置页重排（第 7 项）**：四分组——常规（语言）/ 网络与访问（监听端口·安全入口·域名绑定）/ 安全与证书（HTTPS·面板证书·会话参数）/ 关于（更新检查）；8 Card 语义编号不变、呈现顺序+分组标题重排（PageSpec panel-core v1.2）。
- **附带修复**：account-auth 列表页 loadError 错误态规范（NAlert+重试——user-list 加载失败时正文空白的候选根因，v0.1.0 /users 前科同域）。

## 二、PRD 就绪检查

| 模块 | G1-G5 | T 适用项 | N1-N4 | 结论 |
|------|-------|---------|-------|------|
| site-mgmt（增量） | 维持 5/5（v1.0 基线） | 维持 7/7（FR-28 台账型变更操作，T2 迁移口径已覆盖） | 4/4（N2 +3 场景） | ✅ 增量完备性复核通过（prd 附录 v1.1 节） |
| env-mgmt（首检） | 5/5 | 6/6（T1/T1a/T2/T4/T9+幂等并入 N3；T3/T5/T6 N/A 有据） | 4/4 | ✅（prd-env-mgmt.md 附录） |

## 三、API 文档完整性（按 go-vue3 栈规约裁剪适用项执行，先例口径）

| 章节 | site-mgmt v1.2（增量重跑） | env-mgmt v1.0（首检） | panel-core v2.2（增量） | 结果 |
|------|--------------------------|----------------------|----------------------|------|
| 第 1 章（CH1 适用项） | ✅ 决策表 +D-17~D-19；领域对象总览/上下文映射零变化 | ✅ 12 决策/对象总览/无子流程与关联对象已声明 | ✅ +D-P18 | PASS |
| 第 2 章 | N/A（单聚合根声明在位） | N/A（单聚合根） | N/A | PASS |
| 第 3 章（CH3 适用项） | ✅ 权限矩阵 +2 行；错误码 CERT_002 扩展+CERT_008；常量 +2 | ✅ 路径/方法/包络/中间件链/权限矩阵/错误码 ENV_001~006/枚举引用 5 项标源/常量 | ✅ 矩阵零变化（rule_added） | PASS |
| 第 4 章 | ✅ 4.1.3 触发扩展；4.1.4 +2 接口详情+幂等；4.2.4 list 聚合结构 | ✅ 4.1 领域模型/不变量/状态机/接口详情 10 条/幂等/网关契约；4.2 投影链路 | ✅ menu/list 详情行补注 | PASS |
| 第 5 章 | ✅ 5.1 +2 行；5.2 注 replace 无 SQL | ✅ 5.1/5.2（9 SQL）/5.3 DDL | 零变化 | PASS |
| 全局（GL 适用项） | ✅ GL-001a 日期、GL-009 三段式、GL-021 枚举引用 | ✅ 同左 | ✅ | PASS |
| M1~M5 机械自检 | 5/5（20 路径三段式、changelog v1.2 在位） | 5/5（10 路径三段式、changelog v1.0 在位、占位符 0） | 5/5（rule_added 行在位） | PASS |

## 四、前后端契约一致性（S7/S8 机械实跑）

| 校验 | 实跑结果 | 结论 |
|------|---------|------|
| S7 site-mgmt | api 20 接口路径 ↔ admin PageSpec 20 Action 路径 **逐字全等**（grep 双侧排序 diff=0；cert/request 单接口覆盖两挑战菜单既有口径）；readonly 4 读 Action + 16 负空间 = 20 全集（负空间行含 certRenew/certReplace） | ✅ |
| S7 env-mgmt | api 10 接口（含 service 合并行展开 5）↔ admin 10 Action 全等；readonly 3 读 + 7 负空间 = 10 全集 | ✅ |
| S7 panel-core / account-auth | Action 10/18 零变化（v1.2/v1.3 copy_revision 层）；菜单 IA 经 D-P18 登记（分组+四子项+env 一项），menu/list 响应结构零变化 | ✅ |
| S8 枚举三方同值 | env_type/env_task_action/env_task_status/package_manager_family：池（04-business-enum.md:56-59）↔ api DDL CHECK/3.4 表 ↔ PageSpec 第 3 节，取值逐字一致；site-mgmt/panel-core 增量零新枚举 | ✅ |
| CT-002 字段对应 | site/list 聚合条目字段（blocks[] 子清单）↔ PageSpec 4.1 聚合呈现与 Drawer 块清单区一致；cert/renew·replace 响应 ↔ operList 按钮交互（loading/失败分流/PEM 弹窗）一致；env/list 字段 ↔ 卡片区块一致 | ✅ |
| 编号接力 | FR-28（FR-27 后）、D-17~D-19（D-16 后）、D-P18（D-P17 后）、CERT_008、ENV_001~006（新前缀族无冲突）、迁移 008（001~007 后）、⚠️A18/A19（A17 后）、C-14/S-53~S-54（S-52 后） | ✅ |
| DDL↔字段清单 | env_install_task：DDL 11 列 ↔ 4.1.1 字段清单逐列一致（含 CHECK 三枚举同值、两索引） | ✅ |
| doccenter | D2 领域划分/模型变更后已再生成（12 业务域，engine-cli doccenter 实跑 ✓） | ✅ |

## 五、WARN 项（建议/复核，不阻断）

| # | 项 | 说明 | 处置建议 |
|---|----|------|---------|
| W1 | site/list 响应结构破坏性变更 | items 移除顶层 siteId（聚合条目化，D-17）——前后端同仓同版本发布无外部消费者，但属契约 breaking（interface_changed 已登记）；v0.1.2 前端不兼容本响应 | 确认接受（B/F 同轮切换） |
| W2 | certRetryRenewal 保留但无前端按钮 | cert/renew 覆盖其全部场景（renew_failed 亦可用），Action 与接口保留兼容、前端不再渲染独立按钮 | 确认保留（或 B 阶段标记 deprecated） |
| W3 | BA ⚠️A6 动态计数收窄 | BA 文本「Docker 有容器/MySQL 有实例/nginx 有站点时列出警示」→ PRD 落地为静态警示文案+本域服务实况（动态跨模块计数 OUT-9，无跨模块硬调用纪律） | 确认收窄口径（二期如需动态计数走关联模块 OHS 联动设计） |
| W4 | P4 IA 方案未经逐项选择 | 方案一（动态顶栏+侧栏分组+四分组设置页）为推荐假设 | 确认方案一或改选（PageSpec 层可快速重做；若改方案二/三需回改 PageSpec panel-core v1.2/account-auth v1.3 第 7 节+D-P18） |
| W5 | replace 后自动 reload | imported 证书替换后自动 nginx reload（ssl_cert 路径未变但证书内容变更需 reload 生效）；站点无引用该文件时不 reload 亦可——实现按「统一 reload」从简 | 确认统一 reload |
| I1 | prd 说明行 | prd-panel-core v1.5 / prd-account-auth v1.1 为纯 IA 说明行（FR/OUT/N 零变化） | INFO |
| I2 | 迁移编号 | 008_env_mgmt.sql 为 B 阶段产物（设计侧已登记编号与 DDL） | INFO |

## 总结

**判定**：**WARN**（CRITICAL 0 / 契约断裂 0；初检 5 项 WARN + DNS-01 改判增量 P2′ 复检全过 + W6~W8 三项增量 WARN，均为不阻断建议项）

**用户确认状态（2026-11-16 主会话转达）**：P1 ✅ 确认 / P3 ✅ 确认 / P4 ✅ 方案一 / W1/W2/W5 ✅ 按建议处置 / **P2 改判为 DNS-01 DNS API 自动化（已落盘增量并复检，见 P2/P2′ 节）——服务商清单与手动 TXT OUT 结论呈请复核（W6~W8 一并）**。

- 机械项：M1~M5 ×3 文档全过；S7 site-mgmt 20/20 + env-mgmt 10/10 双向零差；S8 枚举三方同值；编号接力无冲突；占位符 0。
- 结构项：PRD 完备性 site-mgmt 增量复核通过 + env-mgmt 首检通过；API 文档完整性按栈规约适用项全过；DDL 与字段清单一致。
- 待用户确认：P1~P4（既定拍板与 IA 方案）+ W1~W5。确认后进入 B（后端）阶段。
