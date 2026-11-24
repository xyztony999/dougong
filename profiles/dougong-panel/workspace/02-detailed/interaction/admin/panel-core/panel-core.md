# PageSpec · panel-core · 管理员（admin）

| 属性 | 值 |
|------|-----|
| 版本 | v1.4 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-24 |
| 用户类型 | admin（框架级行为：菜单渲染/首访 HTTPS 提示/页面切换的加载反馈与失败恢复/弹窗抽屉关闭期间的侧栏点击/安全入口前缀寻址在本节定义，readonly 文档引用本文） |
| 唯一数据源 | 02-detailed/api/api-panel-core.md v2.4 |
| 组件体系 | Vue 3 + Naive UI（knowledge/architecture-choices 栈版本表；无激活栈 profile，不使用 vue3-engine 栈 Scm* 组件） |
| 确认方式 | headless run：7 项交互确认依据 D3 契约 + R-09/R-10 + ADR-018 + FL v1.1 + ADR-028/029 推导，标注于各节 |

## 第 1 节 模块元信息

| 项 | 值 | 确认依据 |
|----|-----|---------|
| 模块编码 | panel-core（域前缀 /panel） | api 文档 3.1 |
| 业务类型 | master_data（panel_config 配置单例：无 MQ、无状态机流转） | 分册 2.1 判据；api 4.1.2 |
| 页面类型 | 全部 config（无 Tab 切换/左树右表/拖拽画布场景；设置页多 Card 内联表单沿用 profile-self 惯例） | 分册 2.2；api-account-auth 5.3 先例 |
| UI 框架 | Naive UI（NForm/NSelect/NSwitch/NInputNumber/NAlert/NTag/NModal/NPopconfirm/NCode） | 栈版本表 |
| 状态管理 | Pinia：panelStore（configDetail/pendingRestart/menuTree）、枚举常量 store（与 account-auth 共用 enums.ts） | 栈版本表 |
| 错误处理 | 统一拦截器按 api 3.4 code 分流：AUTH_001/002→跳登录（复用 account-auth 拦截器）、AUTH_008→边界原因提示（ADR-018）、AUTH_012→fields 逐字段提示、PANEL_001→custom 地址必填、PANEL_002→证书解析/配对失败、PANEL_003→需先导入/申请对应来源证书、PANEL_004→"更新检查未开启"、PANEL_005→"版本源不可达"（不自动重试，api D-P09）、PANEL_006→语言取值不支持、PANEL_007→"请先绑定域名再申请证书"、PANEL_008→"nginx 不可用或 80 端口未监听（HTTP-01 挑战通道不可用）"、PANEL_009→ACME 签发/续期失败原因展示（不自动重试，api D-P13）、PANEL_010→证书状态不允许该操作、PANEL_011→"已有有效 ACME 证书，续期请用重试" | api 3.4 |

## 第 2 节 Action 定义（与 API 一一对应）

| Action | 方法 | 路径 | 页面 | 权限 | CSRF |
|--------|------|------|------|------|------|
| configDetail | GET | /panel/config/detail | settings（+框架 HTTPS 提示派生；含 bindDomain 组） | admin+readonly | — |
| configUpdate | POST | /panel/config/update | settings（各 Card 分域提交，含域名绑定 Card 的绑定/解绑） | admin | ✓ |
| entryPathRegenerate | POST | /panel/config/regenerate-entry-path | settings·安全入口 Card | admin | ✓ |
| certUpload | POST | /panel/config/upload-certificate | settings·HTTPS Card | admin | ✓ |
| certStatus | GET | /panel/cert/status | settings·面板证书 Card | admin+readonly | — |
| certRequest | POST | /panel/cert/request | settings·面板证书 Card | admin | ✓ |
| certRetryRenewal | POST | /panel/cert/retry-renewal | settings·面板证书 Card | admin | ✓ |
| menuList | GET | /panel/menu/list | 框架侧边菜单（全局） | admin+readonly（服务端过滤） | — |
| versionGet | GET | /panel/system/version | about | admin+readonly | — |
| updateCheckRun | POST | /panel/system/update-check | settings·更新检查 Card | admin | ✓ |

> S7 一致性：10 Action 与 api-panel-core.md v2.0 10 接口一一对应（一接口一 Action，无未覆盖接口、无虚构接口；域名绑定/解绑并入 configUpdate.bindDomain——api D-P12，不设独立接口故无独立 Action）。机制型能力（FR-01~03 安装部署、FR-07 首访提示、FR-08 布局骨架、FR-11 认证承载、监听重启 systemctl、HostGuard 拦截）无接口即无 Action，见第 7 节框架行为注记。

## 第 3 节 字典 Key 清单

| Key | 取值（=01-solution/04-business-enum.md 定稿） | 用途 |
|-----|--------------------------------------------|------|
| https_mode | off / self_signed / imported / acme | HTTPS 模式下拉 + 模式 tag（colorTag；v1.1 增量 +acme，api D-P14） |
| listen_scope | all / localhost / custom | 监听范围下拉 |
| panel_cert_source | self_signed / imported / acme | 证书摘要展示（来源 tag；v1.1 增量 +acme，ADR-029） |
| cert_status | valid / expiring_soon / renew_failed / expired | 面板证书 ACME 状态 tag（v1.1 新增引用；池定义 5 值，本模块呈现 4 值子集——同步终态无 pending，api D-P13） |
| config_effect_mode | immediate / restart_required | 生效方式标识（effectMode 渲染） |
| panel_role | admin / readonly | 菜单/按钮可见性判定（authStore 复用） |

lang 为字符串非枚举（MVP 固定 `zh-CN` 单选项，api I-P08；多语言二期再登记 panel_lang）；布尔项 entry_path_enabled/update_check 用 NSwitch（0/1，公共布尔约定）；面板证书挑战方式固定 http_01（api 3.4 常量，非请求参数无字典项）。

**数据源规范偏差声明**（同 account-auth 先例）：不实现 `POST /{domain}/dict/batch`（分册 3.6）——面板单机、枚举全部固定值，字典由前端 TS 常量提供（enums.ts 单一事实源，与 account-auth 共用同一文件）。此偏差为设计决策，待 Gate 1 复核。

**状态颜色方案**（分册 3.1 语义映射）：

| Key::值 | 颜色 |
|---------|------|
| https_mode::off | gray |
| https_mode::self_signed | blue |
| https_mode::imported | green |
| https_mode::acme | blue（「ACME 证书」tag，v1.1） |
| panel_cert_source::self_signed | blue |
| panel_cert_source::imported | green |
| panel_cert_source::acme | blue（v1.1） |
| cert_status::valid | green（v1.1） |
| cert_status::expiring_soon | orange（v1.1，「30 天内到期」） |
| cert_status::renew_failed | red（v1.1，附 lastError 展示） |
| cert_status::expired | gray（v1.1） |
| config_effect_mode::immediate | green（「即时生效」tag） |
| config_effect_mode::restart_required | orange（「需重启」tag） |
| updateAvailable | true=blue「有新版本」/ false=green「已是最新」 |
| pendingRestart | true=orange 警示横幅 |

## 第 4 节 列表页

本模块**无列表页**：panel_config 为单例配置（api 4.1.2 无状态机、detail 单例读无分页），无台账/清单场景——诚实对应接口，不虚构列表。交互确认第 2/3/4 项（查询条件/表格列/operList）经此声明判定为 N/A；操作按钮下沉到第 5 节各 Card（确认项 4 的落位）。

## 第 5 节 表单页

### 5.1 面板设置（settings）— 路由 /settings

**权限**：admin+readonly 可见（ADR-018 全可看）；全部变更控件 admin 专属（readonly 渲染为禁用态，见 readonly 文档）。

**页面级横幅**（进入页面及每次 configDetail 刷新后计算）：

| 条件 | 展示 |
|------|------|
| detail.pendingRestart=true | NAlert type=warning「监听配置已修改，重启服务后生效（systemctl restart dougong）」——关闭横幅不可手动消除，重启后随 pendingRestart=false 自动消失（api D-P02 派生标识） |
| location.protocol==='http:' 且 detail.https.mode==='off' | NAlert type=info「当前使用 HTTP 明文连接，建议启用 HTTPS（设置页下方可配置）」——FR-07「建议但不强制」口径；HTTPS 模式非 off 时不展示 |

**页面分组与顺序（v1.2 重排，v0.1.2 用户反馈第 7 项）**：8 Card 语义编号不变（Card 1~8），**呈现顺序按四分组重排**并加分组标题（NText depth=3 分组小节标题，视觉层级：分组标题—Card 两级）：

| 分组 | 顺序 | Card |
|------|------|------|
| 常规 | ① | Card 6 面板参数（lang） |
| 网络与访问 | ② | Card 1 监听设置（listen）→ Card 2 安全入口（entryPath）→ Card 3 域名绑定（bindDomain） |
| 安全与证书 | ③ | Card 4 HTTPS 设置（https）→ Card 5 面板证书（cert ACME）→ Card 7 会话参数（session） |
| 关于 | ④ | Card 8 更新检查（updateCheck） |

（原 v1.1 顺序 Card 1→2→3→4→5→6→7→8 的「语言被挤至第 6、会话第 7」跳跃问题就此收敛；HTTPS Card 的 to acme 前置引导锚点仍指向 Card 5（同分组相邻）。）

**提交粒度决策**（确认项 5）：8 个 Card 各自独立提交（同 account-auth profile-self 惯例：无跨 Card 草稿态），每 Card 一个「保存」按钮，仅提交本 Card 字段（api configUpdate 为部分字段集语义）；成功后统一 re-fetch configDetail 刷新 effectMode/pendingRestart（域名绑定 Card 生效无需刷新绑定值以外的状态；面板证书 Card 动作后另行 re-fetch certStatus）。

#### Card 1 监听设置（listen）

| 字段 | 组件 | 校验 | 默认/说明 |
|------|------|------|----------|
| 监听范围 | NSelect（listen_scope） | 必填 | all=0.0.0.0；scope≠custom 时隐藏地址框 |
| 监听地址（条件显隐） | NInput | scope=custom 时必填，合法 IP/主机名（api I-P02）；PANEL_001 提示 | — |
| 监听端口 | NInputNumber | 必填；1024..65535（api I-P02/DDL CHECK）；AUTH_012 fields 提示 | 安装时生成值 |

**按钮**：`保存`（configUpdate，仅本 Card 字段）→ 成功 toast「已保存，重启服务后生效」+ pendingRestart 横幅出现；Card 角标 NTag「需重启」(orange)（config_effect_mode）。

#### Card 2 安全入口（entryPath）

| 字段/元素 | 组件 | 说明 |
|----------|------|------|
| 启用开关 | NSwitch | 切换即触发确认流（见下），不直接提交 |
| 当前入口路径 | NCode + `复制` 按钮 | enabled=true 时展示；readonly 可见完整路径（FL 附录B 决策） |

**交互流**（v1.4：三个动作成功后都**整页跳转**到新地址下的设置页——当前页面是按旧地址加载的，`<base>` 还是旧值，成功后再发的任何请求都带旧前缀、会被入口网关拦掉，故不在当前页继续请求；跳转前在会话存储记一笔〔60s 时效〕，新页面挂载时补成功提示并再次展示路径，prd N2 遗忘风险对策不丢）：
1. 开关 0→1：NModal 确认（文案含风险提示「开启后未携带入口路径的访问将被拒绝，请妥善保存路径」）→ configUpdate {entryPathEnabled:true} → 若无既有路径服务端自动生成（api 4.1.4）→ 成功后**整页跳到 `/{entryPath}/settings`**，新页面弹 NModal 大字展示路径 + 复制按钮（prd N2 遗忘风险对策）。
2. 开关 1→0：NPopconfirm 确认「关闭后任意路径均可访问面板」→ configUpdate {entryPathEnabled:false} → 成功后**整页跳到根地址 `/settings`**。
3. `重新生成路径`（NButton，enabled=true 时可用）：NPopconfirm 警告「旧路径立即失效，未保存新路径将无法访问」→ entryPathRegenerate → 成功后**整页跳到新路径下的设置页** + 新路径 NModal + 复制；PANEL_* 失败原路径不变、留在当前页（api 4.1.4）。

修改即时生效（EntryPathGuard 下一请求即用新值，api D-P03）；Card 角标 NTag「即时生效」(green)。审计提示：成功 toast 附「变更已记入审计」（api panel_entry_path_regenerated）。

#### Card 3 域名绑定（bindDomain，v1.1 新增）

| 字段/元素 | 组件 | 校验/说明 |
|----------|------|----------|
| 绑定域名 | NInput（占位「未绑定——填写域名后仅该域名可访问面板」） | RFC 1123 主机名、≤253、不含 scheme/path/端口（api D-P12；AUTH_012 fields.bindDomain→「域名格式不合法（应为主机名，不含 http:// 或路径）」） |
| 当前绑定状态 | 只读描述列表 | detail.bindDomain.value：null→NTag「未绑定」(gray)；非空→NCode 展示域名 + NTag「已绑定」(blue) |

**交互流**（api D-P12：绑定/解绑均经 configUpdate.bindDomain，null/空串=解绑）：
1. `绑定`（输入非空域名后可用）：**NModal 强确认**（ADR-028 风险对策，文案：「绑定后仅该域名可访问面板，IP 直连与其他域名将返回 404 无法访问。请确认域名已解析到本服务器，且已妥善保存其他访问方式（如安全入口路径）」）→ configUpdate {bindDomain:"…"} → 成功 toast「域名已绑定，即时生效（变更已记入审计）」。
2. `解绑`（已绑定时可见）：NPopconfirm「解绑后任意 Host 与 IP 均可访问面板」→ configUpdate {bindDomain:null} → 成功 toast「已解绑」。

即时生效（HostGuard 下一请求即用新值，api D-P16）；Card 角标 NTag「即时生效」(green)。

#### Card 4 HTTPS 设置（https）

| 字段/元素 | 组件 | 说明 |
|----------|------|------|
| HTTPS 模式 | NSelect（https_mode） | off/self_signed/imported/acme 四态（R-10 三态 + v1.1 增量 acme，api D-P14） |
| └ 模式说明 | 内联 hint | self_signed→「保存后系统自动生成自签证书」；imported→展开证书导入区；acme→「ACME 证书在下方『面板证书』Card 申请，申请成功后自动启用（勿在此手选）」 |
| └ 证书 PEM | NInput type=textarea（monospace） | certPem；仅 imported 展开 |
| └ 私钥 PEM | NInput type=textarea（monospace，粘贴态遮罩） | keyPem；仅 imported 展开；保存成功后清空不回显（api I-P06 零下发）。遮罩用 CSS（-webkit-text-security）只改显示不改值，多行粘贴的换行原样保留，附「显示私钥/隐藏私钥」开关临时核对。**必须多行文本域**：v0.5.0 前实现误用单行 password 框，粘贴多行 PEM 时换行被浏览器折成空格致导入失败，v0.5.1 回归本设计 |
| └ 当前证书摘要 | 只读描述列表 | cert 存在时展示：来源 tag（panel_cert_source）/颁发者/有效期（notAfter 过期临近橙色提示）；无「查看私钥」入口（api 3.3 脱敏） |

**按钮**：
- `保存`（configUpdate {httpsMode}）：to imported 前置校验证书已导入（PANEL_003→提示「请先导入证书与私钥」）；to acme 前置校验 panel_cert.source=acme 且未过期（PANEL_003→提示「请先在下方『面板证书』Card 申请 ACME 证书」，api D-P14）；to self_signed 成功 toast「已保存，自签证书已生成」（api D-P06 即时生效）；to off 直接生效。
- `导入证书`（certUpload {certPem, keyPem}，仅 imported 展开区可见）：PANEL_002→「证书解析失败或与私钥不配对，请检查 PEM 内容」（R-14）；后端解析前对 PEM 容错重排——粘贴产生的空白坏格式（标记与正文挤在一行、base64 夹空格）去掉空白按 64 折行后再解析，只修空白不放宽配对校验（v0.5.1）；成功 toast「证书已导入并记入审计」+ 摘要刷新；不改变 https_mode（两接口顺序协作：先导入后切模式，api 2.3）。

即时生效；Card 角标「即时生效」。

#### Card 5 面板证书（cert ACME，v1.1 新增）

**数据源**：certStatus（进入页面与 configDetail 并行拉取；申请/重试成功后 re-fetch）。

| 字段/元素 | 组件 | 说明 |
|----------|------|------|
| ACME 域名 | 只读文本 + 状态 | certStatus.acme.domain（=bind_domain 当前值）；未绑定时 NAlert info「请先在上方『域名绑定』Card 绑定域名」+ 锚点链接 |
| 挑战方式 | 只读文本 | 固定 http_01（经服务器已有 nginx 验证，api 3.4 常量；DNS-01 二期不展示） |
| 自动续期 | 只读 hint | 「ACME 证书到期前 30 天自动续期（每日后台调度，失败不自动轰炸）；失败后可手动重试」（api 4.1.3/D-P13） |
| 证书摘要 | 只读描述列表 | cert=null→NTag「未配置证书」(gray)；存在→来源 tag（panel_cert_source，acme=blue）/状态 tag（cert_status，仅 acme 来源显示）/颁发者/有效期（notAfter 30 天内橙色）；**无「查看私钥」入口**（api I-P06） |
| 上次失败原因 | NAlert error（条件渲染） | cert.lastError（renew_failed 时展示，排障口径 FR-18） |

**按钮**：
- `申请证书`（certRequest，admin；未绑定域名时禁用——PANEL_007 前端预判）：**loading 同步等待态**（api D-P13 同步终态 ≤120s：按钮 loading + 文案「正在签发（经 nginx 验证域名，最长约 2 分钟），请勿关闭页面」）；PANEL_008→「nginx 不可用或 80 端口未监听，无法完成域名验证」；PANEL_009→展示 data.reason（不自动重试）；PANEL_011→「已有有效 ACME 证书（{notAfter} 到期），续期请使用下方重试」；成功→摘要刷新 + toast「证书已签发，HTTPS 已切换为 ACME 证书（记入审计）」（https_mode 联动 acme，api D-P14，同时刷新 configDetail）。
- `手动重试续期`（certRetryRenewal，admin；仅 cert.source=acme 且 status=renew_failed 时可见）：loading 同步等待态同上；成功/失败反馈同申请（PANEL_010 在本可见条件下不会触发，兜底提示「当前状态无需重试」）。

即时生效；Card 角标「即时生效」。

#### Card 6 面板参数（lang）

| 字段 | 组件 | 校验 |
|------|------|------|
| 界面语言 | NSelect | 仅 `zh-CN` 单选项（MVP，api I-P08；PANEL_006 兜底） |

**按钮**：`保存`（configUpdate {lang}）→ 即时生效；Card 角标「即时生效」。

#### Card 7 会话参数（只读透传）

| 元素 | 组件 | 说明 |
|------|------|------|
| 绝对过期时长 | 只读文本 | detail.session.absoluteTimeout（默认 12h，ADR-019） |
| 空闲超时时长 | 只读文本 | detail.session.idleTimeout（默认 2h） |
| 维护方式说明 | 内联 hint | 「该参数由账户与安全基座（account-auth）管理：修改配置文件后重启生效；本页仅展示（api D-P08）」 |

无保存按钮（只读透传，PRD FR-14；POST session 字段被服务端拒绝）。

#### Card 8 更新检查（updateCheck）

| 字段/元素 | 组件 | 说明 |
|----------|------|------|
| 更新检查开关 | NSwitch | 默认关（R-09）；切换 NPopconfirm：开启→「开启后可手动检查新版本，面板不会自动外呼」；关闭→「关闭后完全禁止外呼」→ configUpdate {updateCheck} |
| 检查更新 | NButton | 仅 admin 且开关开启时可用（api 权限矩阵 + I-P05 双重门控）；loading 态防重复点击 |
| 检查结果 | 内联结果区 | updateCheckRun 成功：updateAvailable=true → NTag「有新版本：{latestVersion}」(blue)（仅提示，无下载/升级入口——idea.md §4.2 非目标）；false → NTag「已是最新」(green)；PANEL_004→提示开关未开启；PANEL_005→「版本源不可达，可稍后手动重试」（不自动重试，api D-P09） |

开关变更即时生效；成功 toast「开关已保存，变更已记入审计」（panel_config_changed）；手动触发不落审计（api 3.4 口径）。

### 5.2 关于（about）— 路由 /about

**权限**：admin+readonly 一致（纯展示）。

**Card 版本信息**（进入时 versionGet 一次）：

| 元素 | 组件 | 说明 |
|------|------|------|
| 当前版本 | 大字文本 | data.version |
| 构建信息 | 只读描述列表 | gitCommit（截短 8 位）/ buildTime（本地时区展示） |
| 产品标识 | 静态文案 | 「斗拱 Dougong · 嵌入你的服务器，而不是重装它」+ GPL-3.0 标识（idea.md §10/D3） |

无操作按钮；无「检查更新」入口（该动作在设置页 Card 6，避免双入口）。

## 第 6 节 详情页

本模块无独立详情路由页（配置即"详情"，由 settings 多 Card 承载；无子表/审批记录/操作日志 Card——配置变更历史由 account-auth 审计日志页承载，不复建）。确认项 6 判定为 N/A。

## 第 7 节 路由定义与框架行为

| 路由 | 页面 | meta |
|------|------|------|
| /settings | settings | `{requiresAuth:true, title:'面板设置'}`（readonly 只读渲染） |
| /about | about | `{requiresAuth:true, title:'关于'}` |

**守卫**：复用 account-auth 全局守卫（initCheck → sessionCurrent → roles）；本模块无新增守卫（全部接口要求会话，api D-P11，未登录由全局守卫拦截）。

**框架级行为（本模块作为宿主基座的职责，确认项 7 覆盖声明）**：

| 行为 | 数据源 | 说明 |
|------|--------|------|
| 侧边菜单渲染 | menuList → NMenu 树 | 进入框架后调用一次（D-P07 服务端按角色过滤下发，前端不硬编码菜单、不做二次过滤）；panel-core 自身菜单（面板设置/关于）亦经 ModuleMenuRegistry 构建期登记、由同一接口返回 |
| 首访 HTTPS 建议 | configDetail.https.mode + location.protocol | 见 5.1 页面级横幅（FR-07，无独立接口） |
| 安全入口前缀（v1.4 修订） | 服务端注入 `<base>` + 前端相对寻址 | 开启安全入口后，页面、脚本、图标、接口、WebSocket **全部位于 `/{entryPath}/` 之下**：①构建用相对地址（`base: './'`），入口 HTML 引用 `./assets/…`、`./favicon.svg`，按需分包相对入口脚本自身解析；②服务端下发入口 HTML 时注入 `<base href="/{entryPath}/">`（未开启时 `/`），深链接 `/{entryPath}/sites/12` 下的相对地址也解析到入口路径下（api D-P20）；③前端经统一「应用基址」辅助函数（读 `<base>`）给路由历史基址、接口请求、WebSocket 地址、图标、自动重新加载加前缀。未携带前缀的请求由 EntryPathGuard 返回标准 404（api D-P03，不进前端拦截器分流；书签丢失前缀 = 看到 404，属预期安全行为）。修复前实现从未做到本行意图（入口 HTML 与接口全是根路径写法），开启即白屏——取证与验收见 d0-clarification-panel-core-entry-path-2026-11-24.md |
| 顶栏（v1.2） | 路由 meta.title + 菜单树 | 顶栏标题=当前页面标题（D-P18：路由 meta.title 优先、菜单 title 兜底——替换硬编码业务域显示名，内部术语不泄漏用户 UI）；用户名/角色/登出区不变（account-auth 复用）；非菜单页（403/编辑页等）以 meta.title 呈现 |
| 页面切换的加载反馈（v1.3） | 路由守卫（beforeEach / afterEach / onError） | 点侧栏菜单项或页面内跳转后：①被点击的菜单项**立即高亮**（不等页面文件下载；目标不是菜单页时保持原高亮）；②切换超过 200ms 仍未完成时，顶栏标题右侧出现小转圈 +「正在加载页面…」；③切换完成或失败即消失。200ms 内完成的切换不出现（不闪烁）。快速连点以最后一次点击为准 |
| 页面文件没下载下来——网络原因（v1.3） | router.onError / vite:preloadError | 界面留在原页面，提示「页面没加载出来。请检查网络后再点一次」；不自动重新加载；下一次点击自动重试，不需要刷新。入口脚本探测请求本身失败、或探测拿到 404 以外的其它状态码（反向代理的 5xx、不支持 HEAD 的 405 等）时同样按网络原因处理 |
| 页面文件没下载下来——面板已升级、旧页面还开着（v1.3） | 同上；判定方式=对当前页面入口脚本的地址发 HEAD 请求（不走缓存），只有服务器明确回答该文件已不存在——标准 404，或 2xx 但内容不是 JavaScript（回退的 HTML）——才算面板已升级（api D-P19） | 自动重新加载并打开刚才点的页面，同一目标 30 秒内至多自动一次（防循环）；重新加载后提示「面板已更新，页面已重新打开」；自动重新加载后仍失败、或 30 秒内已自动重新加载过 → 弹出提示框「面板已更新，请刷新页面。」，按钮「刷新页面」（重新加载当前地址）/「取消」 |
| 适用范围（v1.3，2026-11-24 验收时补记） | — | 「面板已升级、旧页面还开着」的自动处理只对**含有本规则的版本**加载出来的页面生效。从不含本规则的旧版本升级上来时，升级前已打开的页面跑的仍是旧代码，需手动刷新一次；此后再升级才由面板自动处理（真实升级实测见 Gate 2 报告） |
| 页面内的按需模块没下载下来（v1.3，如在线编辑器） | vite:preloadError | 与上面两条同规则；这种情况自动重新加载的是当前页面地址。路由跳转引起的加载失败由 router.onError 处理，同一次失败不重复提示 |
| 其它导航错误（v1.3） | router.onError | 提示「页面打开出错。请刷新后重试」；不自动重新加载 |
| 弹窗、抽屉正在关闭时点侧栏（v1.3） | 侧栏框架层（MainLayout，document 捕获阶段） | 弹窗/抽屉关闭后的离场动画约 0.2~0.25 秒，期间全屏层仍盖在侧栏上（主线程忙时更久）。离场期间落在侧栏菜单项上的点击转发给该菜单项——关掉弹窗后立刻点侧栏能点动；落在页面其它位置的点击照旧被拦住（避免连点误触弹窗下面的按钮）；「点遮罩关闭」只关闭一次，不会被转发 |

## 第 8 节 全局一致性

| # | 检查 | 结果 |
|---|------|------|
| 1 | Action ↔ api-panel-core.md 接口一一对应（S7） | ✅ 10/10 接口全映射，无虚构（v1.1：+certStatus/certRequest/certRetryRenewal；域名绑定/解绑并入 configUpdate 无独立接口） |
| 2 | 字典 Key ↔ 04-business-enum.md 定稿枚举一一对应（S8） | ✅ 5/5（https_mode/listen_scope/panel_cert_source/cert_status/config_effect_mode）+ panel_role 复用；https_mode 与 panel_cert_source 取值随池扩展 acme；lang/布尔项非枚举已注明 |
| 3 | 所有 POST Action 均携带 X-CSRF-Token（拦截器统一注入） | ✅ |
| 4 | key_pem/私钥材料不回显不持久化（api I-P06）；检查结果过程数据不落本地存储 | ✅ 5.1 Card 4/5 |
| 5 | ADR-018 边界：readonly 禁用渲染 + 后端 AUTH_008/PANEL_* 双保险 | ✅（负空间明细见 readonly 文档第 2 节） |
| 6 | PANEL_001~011 + AUTH_008/012 错误文案与 api 3.4 一一对应 | ✅ 第 1 节错误处理行 |
| 7 | 字典数据源偏差（无 dict/batch，前端常量）已声明 | ✅ 第 3 节 |
| 8 | 无 📎 Scm* 组件（无 vue3-engine 栈 profile，Naive UI 等价映射） | ✅ |
| 9 | 无列表页/查询区/表格列的诚实性声明（单例配置无台账） | ✅ 第 4 节 |
| 10 | 时间展示本地时区、提交 ISO UTC（公共约定） | ✅ 5.2 构建信息 |
| 11 | 生效语义三态呈现完整：即时生效 toast / 需重启横幅+角标 / 只读透传无按钮 | ✅ 5.1 各 Card |
| 12 | R-09 纪律：外呼仅经「开关开启 + 显式点击」双门控，无自动轮询/预取外呼 | ✅ 5.1 Card 8；面板证书 ACME 为用户显式点击申请/重试（R-09 例外语义，api D-P13），同样无自动触发 |
| 13 | ADR-028 风险对策：域名绑定强确认 NModal + 解绑 NPopconfirm；绑定后 HostGuard 404 行为已向用户明示 | ✅ 5.1 Card 3 |
| 14 | ACME 同步等待（≤120s）交互：loading 态 + 防重复点击 + 明示时长；失败原因如实展示不自动重试 | ✅ 5.1 Card 5 |
| 15 | 页面切换加载反馈与失败恢复的用户可见文案（v1.3，第 7 节）过栈文案规约：无 emoji、无感叹号，说清结果与下一步 | ✅ 「正在加载页面…」+ 4 条提示语 + 1 个提示框逐条核对 |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-12 | initial | 首次生成：2 页面（settings 6 Card + about）、7 Action、4 字典 Key、0 列表页/2 表单页/0 详情路由；框架级职责（菜单渲染/HTTPS 提示/入口前缀透明）并入第 7 节 |
| v1.1 | 2026-11-16 | interface_added / field_added / enum_value_added | 增量 D4（ADR-028/029，数据源 api-panel-core.md v2.0）：**新增 Action 3 个**（certStatus/certRequest/certRetryRenewal，7→10）；settings 新增 Card 3 域名绑定（绑定强确认 NModal/解绑 NPopconfirm，经 configUpdate.bindDomain）与 Card 5 面板证书（申请/状态/手动重试，同步等待 ≤120s loading 态），原 Card 3~6 顺延为 4/6/7/8（6→8）；HTTPS Card 模式下拉扩四态（+acme，to acme 前置引导至面板证书 Card）；字典 +cert_status 引用（4 值子集）、https_mode/panel_cert_source 取值扩 acme；错误处理 +PANEL_007~011 文案；第 8 节自查 12→14 项（ADR-028 风险对策、ACME 同步等待交互）；S7 机械 diff 10/10 通过（见本次自检记录） |
| v1.2 | 2026-11-16 | copy_revision / structural | 增量 D4（v0.1.2 用户反馈第 5/7 项，数据源 api-panel-core.md v2.2 D-P18）：**设置页四分组重排**（常规/网络与访问/安全与证书/关于——8 Card 语义编号不变、呈现顺序与分组标题落地，Card 6 语言回归常规首卡）；**顶栏标题=当前页面标题**（路由 meta.title 优先、菜单 title 兜底，替换硬编码业务域显示名——框架级职责本节登记）；路由 meta +title 两项；Action/字典/契约零变化（菜单 IA 变更见 api v2.2 与 PageSpec account-auth v1.3） |
| v1.3 | 2026-11-24 | rule_added | 增量 D4（缺陷反馈「左侧侧栏有时点不动」，数据源 api-panel-core.md v2.3 D-P19，取证见 00-architecture/d0-clarification-panel-core-sidebar-nav-2026-11-24.md 第 7 节）：第 7 节框架级行为 +6 行——页面切换的加载反馈（菜单项立即高亮 + 超过 200ms 顶栏显示「正在加载页面…」）、页面文件没下载下来的四种处理（网络原因提示 / 面板升级后自动重新加载一次、兜底提示框 / 页面内按需模块 / 其它导航错误只提示）、弹窗抽屉正在关闭时把落在侧栏上的点击转发给菜单项；第 8 节自查 14→15 项；Action/字典/路由表/页面结构零变化（无新页面、无新接口） |
| v1.4 | 2026-11-24 | rule_changed / copy_revision | 增量 D4（缺陷「安全入口开启后面板在浏览器里打不开」，数据源 api-panel-core.md v2.4 D-P20，取证与验收见 00-architecture/d0-clarification-panel-core-entry-path-2026-11-24.md 第 8 节，用户拍板方案 A「完整前缀」）：**第 7 节「安全入口前缀」行修订**——由「前端不感知前缀（透明）」改为实际机制「服务端注入 `<base>` + 前端相对寻址」（构建 `base: './'` + 入口 HTML 注入 `<base href="/{entryPath}/">` + 统一应用基址辅助函数供路由/请求/WebSocket/图标/自动重新加载加前缀；修复前实现从未做到、开启即白屏）；**Card 2 安全入口交互流修订**——开启/重新生成/关闭成功后整页跳转到新地址下的设置页（不在旧基址页面上重读配置），跳转前会话存储记一笔（60s 时效）供新页面补提示与路径展示（prd N2 遗忘风险对策不丢）；Action/字典/路由表/页面结构零变化（无新页面、无新接口） |

> FX 质量清单说明：FX-001~FX-141 逐条清单文件未随引擎 0.5.2 payload 发布（本次 find 复核：payload 下无 design/front-spec checklist，frontend-specs 仅存在于未激活的栈 profile vue3-engine/react-vite 下），本节按分册 9 节结构逐节落实，并以第 8 节 12 项全局一致性自查替代可机械执行部分；全量 141 项留待 Gate 1 validation 执行。
