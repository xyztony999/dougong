# PageSpec · site-mgmt · 管理员（admin）

| 属性 | 值 |
|------|-----|
| 版本 | v1.0 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-12 |
| 用户类型 | admin |
| 唯一数据源 | 02-detailed/api/api-site-mgmt.md v1.0 |
| 组件体系 | Vue 3 + Naive UI（knowledge/architecture-choices 栈版本表；无激活栈 profile，不使用 vue3-engine 栈 Scm* 组件）；Monaco Editor（nginx 语法高亮，idea.md §5.3 选型） |
| 确认方式 | headless run：7 项交互确认依据 D3 契约（api-site-mgmt.md D-01~D-16）+ ADR-018/024 + FL v1.0 + PRD FR-01~27 推导，标注于各节 |

## 第 1 节 模块元信息

| 项 | 值 | 确认依据 |
|----|-----|---------|
| 模块编码 | site-mgmt（域前缀 /sites） | api 3.1 |
| 业务类型 | master_data 子形态 + 工具动作混合（certificate：数据生命周期六状态机 ADR-024；site：配置投影 + 运维动作集合） | 分册 2.1 判据；api 1.4 功能类型判定 |
| 页面类型 | site-list = **hybrid**（列表 + 详情 Drawer 多区块组合 + 日志 Tab）；cert-list = config；site-source = **code**（Monaco 第三方组件深度集成） | 分册 2.2；service-mgmt Drawer 先例、file-manager edit 页先例 |
| UI 框架 | Naive UI（NDataTable/NForm/NModal/NPopconfirm/NTabs/NDrawer/NAlert/NTag）+ Monaco Editor | 栈版本表 |
| 状态管理 | Pinia：sitesStore（列表投影缓存/generatedAt/刷新）、certsStore、枚举常量 store | 栈版本表 |
| 错误处理 | 统一拦截器按 api 3.4 code 分流：AUTH_001/002→跳登录、AUTH_003→提示刷新、AUTH_008→边界原因提示（ADR-018）；SITE_002→提示刷新列表、SITE_003→回滚 diff 弹窗（配置已回滚）、SITE_004→「配置已保存但重载失败」+output、SITE_010→基线冲突弹窗、SITE_011→日志空态、SITE_012→空态引导、SITE_013→「快照失败未做修改」、CERT_002→状态提示、CERT_003→引导复用部署、CERT_004→失败原因展示、CERT_005→引用站点清单引导解除 | api 3.4 错误码表 |

**7 项交互确认**（headless 推导，逐项落点）：① 模块元信息见本节；② 查询条件见 4.1/4.2（仅 API 真实查询参数，不虚构关键字搜索）；③ 表格列见 4.1/4.2；④ operList 见 4.1/4.2；⑤ 表单分组见 4.1 新建弹窗与 4.4 Drawer Card；⑥ 详情 Card 见 4.4（证书详情为行内 Modal，api 无证书详情接口——不虚构）；⑦ 页面类型覆盖：site-source=code（Monaco）、site-list=hybrid（Drawer 多区块+日志 Tab），cert-list 维持 config。

## 第 2 节 Action 定义（与 API 一一对应）

| Action | 方法 | 路径 | 页面 | 权限 | CSRF |
|--------|------|------|------|------|------|
| siteList | GET | /sites/site/list | site-list | admin+readonly | — |
| siteDetail | GET | /sites/site/detail | site-list（详情 Drawer） | admin+readonly | — |
| siteCreate | POST | /sites/site/create | site-list（新建弹窗） | admin | ✓ |
| siteUpdate | POST | /sites/site/update | site-list（Drawer 基本信息 Card 编辑态） | admin | ✓ |
| proxyUpdate | POST | /sites/proxy/update | site-list（Drawer 反代 Card 编辑态） | admin | ✓ |
| siteSource | GET | /sites/site/source | site-source | admin | — |
| siteSourceSave | POST | /sites/site/source-save | site-source | admin | ✓ |
| siteEnable | POST | /sites/site/enable | site-list（operList + Drawer 顶部开关） | admin | ✓ |
| siteDisable | POST | /sites/site/disable | site-list（operList + Drawer 顶部开关） | admin | ✓ |
| logTail | GET | /sites/log/tail | site-list（Drawer 日志 Card Tab） | admin+readonly | — |
| nginxReload | POST | /sites/nginx/reload | site-list 工具栏 | admin | ✓ |
| phpPoolsList | GET | /sites/php-pools/list | site-list（新建弹窗数据源） | admin | — |
| certList | GET | /sites/cert/list | cert-list | admin+readonly | — |
| certRequest | POST | /sites/cert/request | cert-list（申请弹窗） | admin | ✓ |
| certDeploy | POST | /sites/cert/deploy | cert-list（部署弹窗） | admin | ✓ |
| certUndeploy | POST | /sites/cert/undeploy | cert-list（解除部署弹窗） | admin | ✓ |
| certRetryRenewal | POST | /sites/cert/retry-renewal | cert-list operList | admin | ✓ |
| certDelete | POST | /sites/cert/delete | cert-list operList | admin | ✓ |

> S7 一致性：18 Action 与 api-site-mgmt.md 18 接口一一对应（无未覆盖接口、无虚构接口；cert/request 一接口覆盖 HTTP-01/TLS-ALPN 两菜单——challenge 参数区分，api C-08-01/02 合并口径）。

## 第 3 节 字典 Key 清单

| Key | 取值（=01-solution/04-business-enum.md 定稿） | 用途 |
|-----|--------------------------------------------|------|
| site_type | static / php_fpm / reverse_proxy | 新建弹窗 type 选择（条件显隐驱动） |
| site_enabled | enabled / disabled | 站点启停列 + 筛选（colorTag） |
| cert_source | acme / imported | 证书来源列 + 筛选 |
| acme_challenge | http_01 / tls_alpn | 申请弹窗挑战方式选择 + 证书 challenge 列 |
| cert_status | pending / valid / expiring_soon / expired / renew_failed | 证书状态列 + 筛选（colorTag）+ 站点列表证书状态列 |
| site_log_type | access / error | Drawer 日志 Card Tab |
| panel_role | admin / readonly | 角色判断（按钮可见性） |

**数据源规范偏差声明**：不实现 `POST /{domain}/dict/batch`（分册 3.6）——面板单机、7 枚举全部固定值，字典由前端 TS 常量提供（从 04-business-enum.md 定稿值生成 `enums.ts`，单一事实源；account-auth/monitoring/service-mgmt/file-manager 同偏差先例）。此偏差为设计决策，待 Gate 1 复核。

**状态颜色方案**（分册 3.1 语义映射）：

| Key::值 | 颜色 |
|---------|------|
| site_enabled::enabled | green |
| site_enabled::disabled | gray |
| cert_status::pending | blue（进行中） |
| cert_status::valid | green |
| cert_status::expiring_soon | orange（临近异常） |
| cert_status::expired | red |
| cert_status::renew_failed | red |
| cert_source::acme | blue |
| cert_source::imported | gray |
| 布尔 nginxAvailable / truncated | false=orange / true=gray |

## 第 4 节 列表页

### 4.1 站点管理（site-list，hybrid）— 路由 /sites

**权限**：admin+readonly 可见（ADR-018 全可看 + D0 口径）；全部变更按钮 admin 专属。

**空态与全局提示**（api D-01/D-15/D-03）：

- `nginxAvailable=false`：页面渲染空态引导卡（SITE_012 语义）——「未检出 nginx，无可接管站点」+ 说明文案；隐藏工具栏与筛选区（R-15 发现哲学：无 nginx 即无对象，不报错不虚构列表）。
- `duplicates[]` 非空：列表上方 NAlert warning——「检测到同域名多文件重复定义：{serverNames}（{files[]}）；面板已按消重优先级（sites-enabled > conf.d > vhost.d）登记写回落点，重复定义未被修改」（api D-03，接管不破坏口径）。
- 列表数据为当次扫描投影（generatedAt 展示于工具栏右侧「扫描于 HH:mm:ss」+ 手动刷新按钮——刷新即重新 siteList，D-01 无缓存口径）。

**查询条件**（对应 api site/list query 唯一参数，诚实不虚构）：

| 条件 | 字段 | 组件 | 查询方式 |
|------|------|------|---------|
| 启停状态 | enabled | NSelect（site_enabled，含「全部」清空项） | eq |

**表格列**（NDataTable，列顺序按分册 3.1）：

| 列 | 字段 | 组件 | 宽度 | 配置 |
|----|------|------|------|------|
| 序号 | — | index | 60 | — |
| 主域名 | serverNames[0] | link | 200 fixed:left | 点击打开详情 Drawer（分册：详情不入 operList） |
| 全部域名 | serverNames | text | 200 | 逗号拼接，NTooltip 全集（多域名时） |
| 监听 | listen | text | 120 | 逗号拼接 |
| 根目录 | root | text | 180 | NTooltip 长路径 |
| 证书 | certStatus（+certRef tooltip） | dict cert_status | 120 | colorTag；null=「无引用」gray tag；有引用未纳管=「未纳管」orange tag（certId 缺失时，D-11 推导） |
| 反代上游 | proxy.upstream | text | 180 | 非反代站点「—」；仅首个反代段（api D-03 口径） |
| 启停 | enabled | dict site_enabled | 90 | colorTag |
| 来源文件 | sourceFile | text | 200 | NTooltip 全路径（⚠️A13 消重落点呈现） |
| 操作 | — | action | 200 fixed:right | operList |

**工具栏**（admin only；readonly 整体隐藏）：`新建站点`（NButton primary）· `重载 nginx`（NPopconfirm：「将平滑重载 nginx 使配置生效」→ nginxReload；SITE_004 → 错误弹窗展示 data.output 并提示检查环境后重试，D-05 配置已保留语义）。

**operList**（按钮可见性 = role===admin，readonly 整列隐藏；启用/停用按行当前 enabled 二选一渲染）：

| 按钮 | Action | 交互 |
|------|--------|------|
| 启用 / 停用 | siteEnable / siteDisable | NPopconfirm（停用文案含「保留配置与证书引用，停用≠删除」OUT-6）→ 成功后刷新列表 + toast；SITE_003 回滚 diff 弹窗（见下） |
| 源码编辑 | —（路由跳转） | router.push `/sites/{siteId}/source`（Monaco 页，4.3） |
| 日志 | logTail | 打开详情 Drawer 并定位日志 Card Tab（便捷入口；数据加载同 Drawer 日志 Card） |

**SITE_003 回滚 diff 弹窗**（全部配置写接口共用，D-04 语义）：NModal 展示「保存失败，nginx -t 校验未通过，**配置已自动回滚到修改前**」+ errors[] 错误行列表 + diff 差异展示（等宽字体对比块）+ 按钮「知道了」（修正内容后重发；回滚非幂等重放，api 4.2.4 幂等语义）。

**新建站点弹窗**（NModal 表单；字段按 type 条件显隐，主字段 ≤4 + 「高级选项」折叠 ≤2——分册 3.2「<5 字段 Dialog」按展开可见字段数口径执行，标注为设计决策）：

| 字段 | 组件 | 校验 | 显隐条件 |
|------|------|------|---------|
| 站点类型 | NSelect（site_type） | 必填；默认 static | 恒显 |
| 域名 | NSelect multiple tag 形态（输入回车添加） | 必填 ≥1；形态校验 SITE_007（失焦前端预校验） | 恒显 |
| 监听端口 | NInput（默认 `80`；多值逗号分隔） | 必填；1~65535 SITE_006 | 恒显 |
| 根目录 | NInput | type=static 必填 | type=static |
| PHP-FPM Pool | NSelect（phpPoolsList 数据源，name+socket 呈现） | type=php_fpm 必填；池清单为空时禁用提交并提示「未检出 php-fpm pool」（api FR-15/D-14 如实口径） | type=php_fpm |
| 上游地址 | NInput | type=reverse_proxy 必填；http(s)://host[:port] 形态 SITE_008 | type=reverse_proxy |
| └ 高级选项（折叠） | — | — | type=reverse_proxy 展开 |
|   Host 头 | NInput | 可空 | └ |
|   WebSocket 透传 | NSwitch | 默认关 | └ |

提交 siteCreate → 成功 toast「站点已创建并生效」+ 刷新列表；SITE_005 → 提示「同域名+端口站点已存在」（data.existingSiteId 提供查看引导）。

**详情 Drawer**（4.4）与**只读视角差异**见 readonly 文档。

### 4.2 证书管理（cert-list，config）— 路由 /certificates

**权限**：admin+readonly 可见（ADR-018 全可看）；全部变更按钮 admin 专属。

**查询条件**（对应 api cert/list query 参数）：

| 条件 | 字段 | 组件 | 查询方式 |
|------|------|------|---------|
| 来源 | source | NSelect（cert_source，含「全部」） | eq |
| 状态 | status | NSelect（cert_status，含「全部」） | eq |

**表格列**：

| 列 | 字段 | 组件 | 宽度 | 配置 |
|----|------|------|------|------|
| 序号 | — | index | 60 | — |
| 主域名 | primaryDomain | link | 180 fixed:left | 点击打开只读详情 NModal（数据源=行数据，api 无证书详情接口——不虚构请求） |
| 域名 | domains | tag 组 | 160 | 首域名 + `+N`；NTooltip 全集 |
| 签发者 | issuer | text | 140 | — |
| 来源 | source | dict cert_source | 90 | — |
| 挑战方式 | challenge | dict acme_challenge | 110 | imported 行「—」 |
| 状态 | status | dict cert_status | 120 | colorTag；renew_failed 行附 lastError NTooltip（FR-24 排障呈现） |
| 有效期至 | notAfter | text | 180 | expiring_soon 行整行橙色浅底提示 |
| 引用站点 | referencedSites | text | 120 | 数量 + NTooltip（siteId+serverNames 清单，D-11 推导） |
| 操作 | — | action | 220 fixed:right | operList |

**工具栏**：`申请证书`（admin only）。

**申请证书弹窗**（NModal 表单，3 字段 → Dialog 形态）：

| 字段 | 组件 | 校验 | 说明 |
|------|------|------|------|
| 站点 | NSelect（siteList 数据源，serverNames 呈现） | 必填 | 域名集合即 SAN（api cert/request 语义） |
| 挑战方式 | NSelect（acme_challenge） | 必填；默认 http_01 | 选项描述：「HTTP-01（需域名可经 80 端口 HTTP 访问）」/「TLS-ALPN（需 443 端口 HTTPS，无需 80）」 |
| — | — | — | 提交按钮 loading 态 + 文案「正在向 ACME 服务申请，最长约 2 分钟」（ACME_TIMEOUT=120s 同步终态，api D-09）；CERT_003 → 提示「同域名证书已存在」+「查看已有证书」引导（data.existingCertId）；CERT_004 → 失败原因展示并提示可重试 |

**operList**（可见性 = role===admin；续期重试仅 status=renew_failed 行渲染）：

| 按钮 | Action | 交互 |
|------|--------|------|
| 部署 | certDeploy | NModal：选择目标站点（siteList 数据源，仅 enabled 站点）→ 确认；CERT_002（非 valid/expiring_soon）提示 data.currentStatus；成功 toast「已部署并重载生效」 |
| 解除部署 | certUndeploy | NModal：选择引用站点（referencedSites 数据源）→ 确认（文案「将恢复该站点为部署前原证书引用」——api D-11 恢复原值语义）；CERT_006 → 如实提示「该站点未部署此证书」 |
| 续期重试 | certRetryRenewal | NPopconfirm（展示 lastError 摘要）→ 同步等待终态（loading 文案同申请）；成功 toast「续期成功，已替换并重新部署」；失败 CERT_004 刷新 lastError |
| 删除 | certDelete | NPopconfirm 强确认（文案区分来源：acme=「将删除证书与托管文件及其续期任务」/ imported=「仅解除面板管理关系，不删除源站证书文件」——api I-04/⚠️A16）；CERT_005 → NModal 列出引用站点清单 + 「去解除部署」引导（不可强制删除） |

### 4.3 源码编辑（site-source，code）— 路由 /sites/:siteId/source

**权限**：admin only（路由 meta role 守卫；readonly 直接路由访问 → 403 边界页 ADR-018）。⚠️A17：仅站点配置文件可编辑，无全局主配置入口。

**页面结构**（全屏编辑器布局）：

- 顶栏：返回列表按钮 + 站点标识（serverNames + sourceFile 只读呈现）+ `保存` 按钮（primary）+ 基线状态 tag（「基线一致」green / 「基线过期」orange）。
- 编辑器：Monaco Editor（nginx 语法高亮），加载 siteSource 响应 content；只读角色不可达（守卫保证）。

**交互流**（api D-06 乐观锁 + SITE_003/SITE_010 语义）：

1. 进入：siteSource 加载 → content 渲染 + baseMtime 记忆；SITE_014（超 1MiB）→ 提示改用下载后本地处理（编辑器只读降级）。
2. 保存 siteSourceSave `{content, baseMtime}`：
   - code=0 → toast「已保存并重载生效」+ 更新 baseMtime；
   - SITE_010 → 冲突弹窗：「配置已被外部修改（面板外/file-manager 通道），当前基线 {baseMtime} ≠ 最新 {currentMtime}」+ 按钮「重新加载」（强确认：丢弃本地编辑）/「取消」；
   - SITE_003 → 回滚 diff 弹窗（同 4.1 共用组件；**保存已回滚，文件为原内容**——编辑器内容需同步刷新为服务端原样，提示「请修正后重试」）；
   - SITE_004 → 提示「内容已保存，nginx 重载失败」+ data.output（编辑器内容保持已保存态）；
   - SITE_014 → 超限提示（提交前前端预校验长度优先）。
3. 离开守卫：dirty 状态路由离开确认（NPopconfirm 语义，「未保存的修改将丢失」）。

### 4.4 详情 Drawer（site-detail，宿主 site-list）

**打开方式**：主域名列 link / operList「日志」；NDrawer width 640。

**顶部**：serverNames 标题 + enabled 开关（NSwitch + NPopconfirm，admin only；readonly 只读 tag）+ 关闭按钮。

**Card 划分**（分册 3.3：主表→detail Card；api 投影字段分组）：

| Card | 内容 | 操作 |
|------|------|------|
| 基本信息 | serverNames/listen/root/sourceFile 只读呈现 | 「编辑」（admin）→ 编辑态表单（字段同 create 静态部分：域名多选/监听/根目录）→ siteUpdate → 成功刷新 Drawer + 列表；SITE_003 回滚弹窗共用 |
| 反向代理 | proxy.upstream/hostHeader/websocket；非反代站点整 Card「—」 | 「编辑」（admin，仅反代站点）→ 表单（上游/Host 头/WS 透传开关）→ proxyUpdate（M3 验收主场景入口） |
| 证书 | certRef/certStatus/certId；未纳管引用 orange 提示「该引用未纳入面板管理，可在证书页识别后部署」 | 「去证书页部署/管理」路由链接（部署动作以 certId 为主键，api cert/deploy 语义——Drawer 不重复承载） |
| location 清单 | 只读表格：path/proxyPass/fastcgiPass | 无（结构化单 location 编辑不做——全量经源码编辑，api 4.2.4 proxy 口径） |
| Raw 块 | 只读等宽文本块（原文本；R-07 保全承诺呈现「写回时原样保留」提示） | 「源码编辑」路由链接 |
| 日志 | NTabs（access/error = site_log_type）+ 行数选择（100/500/1000，默认 100 上限 1000）+ 刷新按钮；行列表（ts 有值显示时间，无值原样文本）；truncated=true 提示「仅展示最近 N 行」 | logTail 拉取；SITE_011 → 空态「日志文件不存在或不可读」（⚠️A15 回退链穷尽，如实呈现）；**两角色可看**（D0 口径，readonly Drawer 唯一可操作区=刷新按钮） |

## 第 5 节 表单页

新建站点弹窗（4.1）、申请证书弹窗（4.2）、部署/解除弹窗（4.2）、Drawer 编辑态（4.4）均已按 Dialog 形态定义（字段数 <5 或条件显隐后 ≤5，api 无独立表单保存接口——全部动作接口即时生效，无草稿态无保存/提交两步）。无独立路由表单页。

## 第 6 节 详情页

无独立详情路由页——站点详情=Drawer（4.4，hybrid 宿主 site-list）、证书详情=行内只读 NModal（4.2，数据源=行数据）。无审批记录 Card（本模块无审批）、无子表 Card。

## 第 7 节 路由定义

| 路由 | 页面 | meta |
|------|------|------|
| /sites | site-list | `{requiresAuth:true}`（readonly 只读视图） |
| /certificates | cert-list | `{requiresAuth:true}`（readonly 只读视图） |
| /sites/:siteId/source | site-source | `{requiresAuth:true, roles:['admin']}`（⚠️A17 + ADR-018；越权 → 403 边界页） |

**守卫顺序**（与全局框架一致，account-auth 第 7 节同构）：sessionCurrent 探测 → roles 校验（unauthorized → 403 页提示边界原因 ADR-018）。

**侧边菜单（admin 视角，本模块贡献）**：网站管理（/sites）· SSL 证书（/certificates）；readonly 同两项（只读视图）。

## 第 8 节 全局一致性

| # | 检查 | 结果 |
|---|------|------|
| 1 | Action ↔ api-site-mgmt.md 接口一一对应（S7） | ✅ 18/18 全映射，无虚构（cert/request 单接口覆盖两挑战菜单——api 合并口径） |
| 2 | 字典 Key ↔ 04-business-enum.md 定稿枚举一一对应（S8） | ✅ 7/7（site_type/site_enabled/cert_source/acme_challenge/cert_status/site_log_type/panel_role，取值逐字对齐定稿） |
| 3 | 所有 POST Action 均携带 X-CSRF-Token（拦截器统一注入） | ✅ |
| 4 | ADR-018 边界：readonly 按钮隐藏由前端 role 判断 + 后端 AUTH_008 双保险；site-source 路由级 roles 守卫 | ✅ |
| 5 | 错误文案与 api 3.4 错误码一一对应（SITE_001~014/CERT_001~007/AUTH 族） | ✅ 第 1 节错误处理 + 各节交互流 |
| 6 | 查询条件仅含 API 真实参数（site/list 仅 enabled；cert/list 仅 source/status）——不虚构关键字搜索/时间范围 | ✅ 4.1/4.2 |
| 7 | 字典数据源偏差（无 dict/batch，前端常量）已声明 | ✅ 第 3 节 |
| 8 | 无 📎 Scm* 组件（无 vue3-engine 栈 profile，Naive UI 等价映射） | ✅ |
| 9 | 状态颜色方案按分册 3.1 语义映射；R-06「改不坏」承诺在 SITE_003 弹窗文案显性呈现（配置已自动回滚） | ✅ 第 3/4 节 |
| 10 | 时间展示本地时区、提交 ISO UTC（公共约定）；generatedAt 投影时刻呈现（D-01） | ✅ 4.1 |
| 11 | M3 验收主场景可达：Drawer 反代 Card 编辑（proxyUpdate）→ 保存 → 成功反馈含「站点已生效、其余站点无变化」语义 | ✅ 4.4 |
| 12 | 停用≠删除、接管不破坏、imported 不动源站文件等业务承诺在确认文案显性呈现（OUT-6/I-04/⚠️A16） | ✅ 4.1/4.2 operList |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-12 | initial | 首次生成：3 页面（site-list hybrid + cert-list config + site-source code）+ 1 详情 Drawer（6 Card）+ 18 Action、7 字典 Key、2 列表页/5 弹窗表单/0 独立表单路由/0 独立详情路由 |

> FX 质量清单说明：FX-001~FX-141 逐条清单文件未随引擎 0.5.2 payload 发布（find 验证仅命中无关的 java-microservice 后端清单；分册仅列编号范围），本节按分册 9 节结构逐节落实，并以第 8 节 12 项全局一致性自查替代可机械执行部分（account-auth/monitoring/service-mgmt/file-manager 四模块同先例）；全量 141 项留待 Gate 1 validation 执行。
