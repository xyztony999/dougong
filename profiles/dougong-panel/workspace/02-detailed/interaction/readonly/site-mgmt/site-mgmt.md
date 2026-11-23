# PageSpec · site-mgmt · 只读用户（readonly）

| 属性 | 值 |
|------|-----|
| 版本 | v1.3 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-23（v1.3 宝塔式重组 D4：网站管理页三分栏，只读用户可进入全部分栏查看；项目分栏负空间见 app-mgmt 只读文档） |
| 用户类型 | readonly |
| 唯一数据源 | 02-detailed/api/api-site-mgmt.md v1.2 |
| 边界依据 | D0 口径（readonly 查看站点列表/详情/证书状态/日志尾部）+ ADR-018：全可看 + 禁一切变更 |
| 组件体系 | Vue 3 + Naive UI + Monaco（同 admin 文档；readonly 不可达 Monaco 页） |

## 第 1 节 模块元信息

| 项 | 值 |
|----|-----|
| 模块编码 | site-mgmt |
| 业务类型 | master_data 子形态 + 工具动作混合（只读视角） |
| 页面类型 | site-list 只读视图 = hybrid（Drawer 只读 + 日志 Tab）；cert-list 只读视图 = config |
| 可见页面 | 框架（菜单两项）· site-list（只读视图 + 只读 Drawer 含日志）· cert-list（只读视图） |
| 错误处理 | 同 admin 拦截器；AUTH_008 边界提示文案明确「只读角色禁止变更操作」（ADR-018） |

## 第 2 节 Action 定义

| Action | 方法 | 路径 | 页面 | 说明 |
|--------|------|------|------|------|
| siteList | GET | /sites/site/list | site-list | 只读视图（D0 口径：查看站点列表） |
| siteDetail | GET | /sites/site/detail | site-list（详情 Drawer） | 只读视图（D0 口径：查看站点详情） |
| logTail | GET | /sites/log/tail | site-list（Drawer 日志 Card） | D0 口径：查看访问/错误日志尾部 |
| certList | GET | /sites/cert/list | cert-list | 只读视图（D0 口径：查看证书状态） |

**负空间（readonly 不可见 Action，19 个，ADR-018 矩阵 + D0 口径逐项对应；v1.1 +2、v1.2 +3）**：siteCreate / siteUpdate / proxyUpdate / siteSource / siteSourceSave / siteEnable / siteDisable / nginxReload / phpPoolsList（建站与配置变更类）/ certRequest / certDeploy / certUndeploy / certRetryRenewal / certRenew / certReplace（v1.1：证书手动更新与文件替换）/ dnsCredList / dnsCredCreate / dnsCredDelete（v1.2 新增负空间：DNS 凭证管理 admin 专属——敏感配置最小暴露，api D-20 解释性决策）/ certDelete（证书变更类）——前端按 role===admin 隐藏入口 + 后端 AUTH_008 双保险；site-source 路由（roles:['admin']）直达访问返回 403 语义边界页。

## 第 3 节 字典 Key 清单

与 admin 文档第 3 节完全一致（同一 enums.ts 常量源）：site_type（仅 drawer/引导文案语境，无编辑入口）/ site_enabled / cert_source / acme_challenge / cert_status / site_log_type / panel_role；颜色方案同源。

## 第 4 节 列表页

### 4.1 站点管理（site-list，只读视图）— 路由 /sites

**v1.1 聚合条目**：列表 items 为聚合条目（⚠️A18/D-17，与 admin 视角同构——监听列合并、行展开 blocks 块清单只读呈现、证书列组内最优）；只读视角无组级启停等操作，展开仅为块级明细查看。

- 空态（nginxAvailable=false）/ duplicates[] 提醒 / 查询条件（enabled 筛选）/ 表格列：与 admin 版 4.1 完全一致（ADR-018 全可看 + D0 口径；含证书状态列、Raw 与来源文件呈现）。
- **详情 Drawer**：与 admin 版 4.4 同构（6 Card 全部只读呈现，含日志 Card Tab——logTail 为 readonly 四 Action 之一，D0 口径；行数选择与刷新按钮可用）。
- **差异**：无工具栏（无「新建站点」「重载 nginx」）；无操作列（operList 为空 → 不渲染 action 列）；Drawer 顶部 enabled 为只读 tag（非开关）；各 Card 无「编辑」按钮；基本信息/反代/证书 Card 的路由引导链接保留（证书页只读视图）。

### 4.2 证书管理（cert-list，只读视图）— 路由 /certificates

- 查询条件（source/status 筛选）/ 表格列 / 只读详情 NModal（行数据源）：与 admin 版 4.2 完全一致（ADR-018 全可看）。
- **差异**：无工具栏（无「申请证书」）；无操作列（无部署/解除/续期重试/删除）；renew_failed 行 lastError 提示保留（排障观测口径）。

## 第 5 节 表单页

无（readonly 无任何表单入口；admin 版全部弹窗/编辑态对其负空间）。

## 第 6 节 详情页

无独立详情路由（同 admin：Drawer + 行内 Modal 承载，均只读）。

## 第 7 节 路由定义

| 路由 | meta | 与 admin 差异 |
|------|------|--------------|
| /sites | `{requiresAuth:true}` | 同路由只读渲染（按 authStore.user.role 分支） |
| /certificates | `{requiresAuth:true}` | 同路由只读渲染 |
| /sites/:siteId/source | `{requiresAuth:true, roles:['admin']}` | readonly 直达 → 403 边界页 |
| 菜单 | 网站管理 / SSL 证书 两项 | 无管理动作入口 |

## 第 8 节 全局一致性

| # | 检查 | 结果 |
|---|------|------|
| 1 | Action 与 admin 文档/API 文档一致，无 readonly 专属虚构接口 | ✅ 4 只读 Action（siteList/siteDetail/logTail/certList）均为 API 真实接口 |
| 2 | 负空间 19 个 Action 与 ADR-018 边界矩阵 + D0 口径逐项对应（v1.1 +certRenew/certReplace、v1.2 +dnsCred×3） | ✅ 第 2 节 |
| 3 | 只读视图与 admin 视图共用列定义与 Drawer Card（无列差异、仅按钮/编辑态差异） | ✅ |
| 4 | 日志尾部查看（logTail）纳入 readonly 且标注 D0 口径依据 | ✅ 第 2/4 节 |
| 5 | site-source 路由级 roles 守卫 + 403 边界页（⚠️A17 + ADR-018） | ✅ 第 7 节 |
| 6 | CSRF/token 纪律同 admin；字典同源 enums.ts | ✅ |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-12 | initial | 首次生成：只读视角 2 列表页（site-list 含只读 Drawer+日志 Tab / cert-list）+ 4 只读 Action + 14 负空间标注 |
| v1.1 | 2026-11-16 | interface_changed | 增量 D4（数据源 api-site-mgmt.md v1.2）：负空间 14→16（+certRenew/certReplace 证书手动更新，admin 专属）；site-list 聚合条目只读呈现（⚠️A18/D-17 同 admin 视角，展开仅查看）；只读 Action 4 项与字典 Key 零变化 |
| v1.2 | 2026-11-16 | interface_changed | DNS-01 增量 D4（数据源 api v2.0 ⚠️A20）：负空间 16→19（+dnsCredList/dnsCredCreate/dnsCredDelete——DNS 凭证管理 admin 专属，敏感配置最小暴露）；证书 challenge 列 dns_01 值只读呈现；只读 Action 4 项零变化 |
| v1.3 | 2026-11-23 | 增量 | 宝塔式重组 D4（ADR-033）：网站管理页三分栏对只读用户全可进入查看；「Python 项目」「Node 项目」分栏的可看项与负空间沿 app-mgmt 只读文档 v1.1；站点部分零变化 |

> FX 清单执行方式同 admin 文档第 9 节注记（清单文件未随 payload 发布，按 9 节结构落实 + 第 8 节自查；全量留待 Gate 1）。

## 增量 v1.3（2026-11-23）：宝塔式重组 D4

- 网站管理页三分栏（「网站」/「Python 项目」/「Node 项目」）对只读用户均可进入查看。
- 「Python 项目」「Node 项目」分栏：项目列表与任务状态可看；创建、删除、启停、重装依赖、建站等变更入口与日志内容为负空间——规格见 ../app-mgmt/app-mgmt.md v1.1。
- 站点部分：零变化。
