# PageSpec · account-auth · 管理员（admin）

| 属性 | 值 |
|------|-----|
| 版本 | v1.6 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-28（v1.6 恢复码降级为恢复手段，§5.2） |
| 用户类型 | admin（另含全用户公共页：初始化向导/登录，readonly 文档引用本文定义） |
| 唯一数据源 | 02-detailed/api/api-account-auth.md v1.4 |
| 组件体系 | Vue 3 + Naive UI（knowledge/architecture-choices 栈版本表；无激活栈 profile，不使用 vue3-engine 栈 Scm* 组件） |
| 确认方式 | headless run：7 项交互确认依据 D3 契约 + ADR-017/018/019 + FL v1.0 推导，标注于各节 |

## 第 1 节 模块元信息

| 项 | 值 | 确认依据 |
|----|-----|---------|
| 模块编码 | account-auth（域前缀 /auth） | api 文档 3.1 |
| 业务类型 | master_data（panel_user/session：无 MQ、生命周期状态；audit_log：只读台账） | 分册 2.1 判据；api 4.x |
| 页面类型 | 全部 config（无 Tab 切换/左树右表/拖拽画布场景） | 分册 2.2 |
| UI 框架 | Naive UI（NDataTable/NForm/NModal/NPopconfirm/NTag） | 栈版本表 |
| 状态管理 | Pinia：authStore（currentUser/csrfToken）、枚举常量 store | 栈版本表 |
| 错误处理 | 统一拦截器按 api 3.4 code 分流：AUTH_002→跳登录、AUTH_003→提示刷新、AUTH_004→倒计时提示、AUTH_008→边界原因提示（ADR-018）、AUTH_009/010/014→业务提示 | api 3.4/1.2 |

## 第 2 节 Action 定义（与 API 一一对应）

| Action | 方法 | 路径 | 页面 | 权限 | CSRF |
|--------|------|------|------|------|------|
| initCheck | GET | /auth/init/check | init-wizard/路由守卫 | 公开 | — |
| initWizard | POST | /auth/init/wizard | init-wizard | 公开（仅未初始化） | 豁免 |
| login | POST | /auth/session/login | login | 公开 | 豁免 |
| totpVerify | POST | /auth/session/totp-verify | login（二步） | 公开（凭 ticket） | 豁免 |
| sessionCurrent | GET | /auth/session/current | 全局守卫 | 会话 | — |
| logout | POST | /auth/session/logout | 全局框架 | 会话 | ✓ |
| userList | GET | /auth/user/list | user-list | admin+readonly | — |
| userCreate | POST | /auth/user/create | user-list | admin | ✓ |
| userDelete | POST | /auth/user/delete | user-list | admin | ✓ |
| userResetPwd | POST | /auth/user/reset-password | user-list | admin | ✓ |
| pwdChange | POST | /auth/user/password-change | profile-self | 会话 | ✓ |
| totpSetup | POST | /auth/user/totp-setup | profile-self | 会话 | ✓ |
| totpEnable | POST | /auth/user/totp-enable | profile-self | 会话 | ✓ |
| totpDisable | POST | /auth/user/totp-disable | profile-self | 会话 | ✓ |
| sessionList | GET | /auth/session/list | session-list | admin+readonly | — |
| sessionRevoke | POST | /auth/session/revoke | session-list | admin | ✓ |
| auditList | GET | /auth/audit-log/list | audit-log-list | admin+readonly | — |
| auditExport | GET | /auth/audit-log/export | audit-log-list | admin | — |
| webauthnRegisterBegin | POST | /auth/webauthn/register-begin | profile-self | 本人会话 | ✓ |
| webauthnRegisterFinish | POST | /auth/webauthn/register-finish | profile-self | 本人会话 | ✓ |
| webauthnList | GET | /auth/webauthn/list | profile-self | 本人会话 | — |
| webauthnDelete | POST | /auth/webauthn/delete | profile-self | 本人会话 | ✓ |
| webauthnLoginBegin | POST | /auth/webauthn/login-begin | login | 公开 | — |
| webauthnLoginFinish | POST | /auth/webauthn/login-finish | login | 公开 | — |
| passkeyVerify | POST | /auth/session/passkey-verify | login（二步） | 公开（凭 ticket） | — |
| recoveryGenerate | POST | /auth/recovery-codes/generate | profile-self | 本人会话 | ✓ |
| recoveryCount | GET | /auth/recovery-codes | profile-self | 本人会话 | — |
| recoveryLogin | POST | /auth/session/recovery-login | login | 公开 | — |

> S7 一致性：28 Action 与 api-account-auth.md 28 接口一一对应（v1.4 +10 登录安全：passkey 注册/管理/登录/二因素 + 恢复码生成/计数/登录；无未覆盖接口、无虚构接口）。

## 第 3 节 字典 Key 清单

| Key | 取值（=01-solution/04-business-enum.md 定稿） | 用途 |
|-----|--------------------------------------------|------|
| panel_role | admin / readonly | 角色列、角色下拉 |
| panel_user_status | active / locked | 用户状态列（colorTag） |
| session_status | active / expired / revoked | 会话状态（派生展示，预留） |
| session_end_reason | self_logout / forced_logout / cascade | 预留（在线清单仅列有效会话，不展示） |
| audit_action | change / security | 审计类别列+筛选（colorTag） |
| audit_result | success / failure | 审计结果列+筛选（colorTag） |
| audit_source | web / cli / system | 审计来源列+筛选 |
| init_status | uninitialized / initialized | 路由守卫判定（不展示） |

**数据源规范偏差声明**：不实现 `POST /{domain}/dict/batch`（分册 3.6）——面板单机、8 枚举全部固定值，字典由前端 TS 常量提供（代码生成时从 04-business-enum.md 定稿值生成 `enums.ts`，单一事实源）。此偏差为设计决策，待 Gate 1 复核。

**状态颜色方案**（分册 3.1 语义映射）：

| Key::值 | 颜色 |
|---------|------|
| panel_user_status::active | green |
| panel_user_status::locked | red |
| session_status::active | green |
| session_status::expired | gray |
| session_status::revoked | red |
| audit_action::change | blue |
| audit_action::security | orange |
| audit_result::success | green |
| audit_result::failure | red |
| 布尔 totpEnabled/enabled | true=green / false=gray |

## 第 4 节 列表页

### 4.1 用户管理（user-list）— 路由 /users

**权限**：admin+readonly 可见（ADR-018 全可看）；变更按钮 admin 专属。

**查询条件**：无（API 无筛选参数——用户量极小全量返回，api 4.1.4；诚实对应接口，不虚构查询区）。

**表格列**（NDataTable，列顺序按分册 3.1）：

| 列 | 字段 | 组件 | 宽度 | 配置 |
|----|------|------|------|------|
| 序号 | — | index | 60 | — |
| 用户名 | username | text | 180 | — |
| 角色 | role | dict panel_role | 100 | — |
| 状态 | status | dict panel_user_status | 100 | colorTag |
| TOTP | totpEnabled | 布尔 tag | 80 | true=已启用(green)/false=未启用(gray) |
| 锁定至 | lockedUntil | text | 180 | ISO 时间；null 显示 — |
| 创建时间 | createdAt | text | 180 | — |
| 操作 | — | action | 160 fixed:right | operList |

**工具栏**：`新增用户`（admin only，NButton primary）——无批量删除（API 为单删接口，无批量端点）。

**operList**（按钮可见性 = 当前用户 role===admin，readonly 角色整列隐藏）：

| 按钮 | Action | 交互 |
|------|--------|------|
| 重置密码 | userResetPwd | NModal 表单弹窗：新密码（≥8 校验 A10）→ 确认 → 成功提示"该用户全部会话已下线" |
| 删除 | userDelete | NPopconfirm 强确认（文案含目标用户名）→ AUTH_010 提示"系统需保留至少一个管理员"/AUTH_014 提示"不能删除当前登录账户" |

**新增用户弹窗**（NModal 表单，字段 3 个 <5 → Dialog 形态）：

| 字段 | 组件 | 校验 | 默认 |
|------|------|------|------|
| 用户名 | NInput | 必填；`[a-zA-Z0-9_.-]{1,32}`（api I-10）；失焦查重提示 AUTH_011 | — |
| 初始密码 | NInput type=password | 必填；≥8 位（A10） | — |
| 角色 | NSelect（panel_role） | 必填 | readonly |

### 4.2 在线会话（session-list）— 路由 /sessions

**权限**：admin+readonly 可见；强制下线 admin 专属。

**查询条件**：无（API 全量返回有效会话，按最后活跃倒序）。

**表格列**：

| 列 | 字段 | 组件 | 宽度 | 配置 |
|----|------|------|------|------|
| 序号 | — | index | 60 | — |
| 用户名 | username | text | 140 | — |
| 会话标识 | sessionId | text | 140 | token 摘要前 8 位（api 4.2.4） |
| 建立时间 | createdAt | text | 180 | — |
| 最后活跃 | lastActiveAt | text | 180 | — |
| 来源 | sourceIp | text | 140 | UA 悬浮 NTooltip |
| 标记 | isCurrent | tag | 100 | true=「当前会话」(blue) |
| 操作 | — | action | 120 fixed:right | admin only |

**operList**：`强制下线`（sessionRevoke）——NPopconfirm 确认（文案"该用户将立即被退出登录"）；isCurrent=true 的行禁用该按钮（登出走框架登出）；AUTH_002 会话已失效 → 刷新列表。

### 4.3 审计日志（audit-log-list）— 路由 /audit-logs

**权限**：admin+readonly 可见（ADR-018 含审计）；导出 admin 专属。

**查询条件**（对应 api 4.3.4 query 精确匹配参数，回车/按钮触发）：

| 条件 | 字段 | 组件 | 查询方式 |
|------|------|------|---------|
| 时间范围 | from,to | NDatePicker type=daterange | between（ISO UTC 转换） |
| 操作者 | actorName | NInput | eq |
| 类别 | category | NSelect（audit_action） | eq |
| 动作 | action | NSelect（15 动作码常量，可搜索） | eq |
| 对象类型 | objectType | NInput | eq |
| 对象标识 | objectId | NInput | eq（可与对象类型独立/组合使用；Gate 1 W3 修正对齐 API query） |

**表格列**：

| 列 | 字段 | 组件 | 宽度 | 配置 |
|----|------|------|------|------|
| 序号 | — | index | 60 | — |
| 时间 | createdAt | text | 180 fixed:left | 本地时区展示（公共约定） |
| 操作者 | actorName | text | 120 | — |
| 类别 | category | dict audit_action | 100 | colorTag |
| 动作 | action | text | 140 | 动作码常量 tooltip 中文释义 |
| 对象 | objectType/objectId | text 合并 | 180 | `type:id`，null 显示 type |
| 结果 | result | dict audit_result | 90 | colorTag |
| 来源 | source | dict audit_source | 90 | — |
| 详情 | detail | action | 80 | 「查看」→ NModal 只读 JSON 树（null 显示禁用） |

**分页**：NDataTable remote 分页（page 从 1 起 + pageSize，`{items,total}` 公共约定）；排序固定 createdAt desc（API 无排序参数）。

**工具栏**：`导出 CSV`（admin only）——按当前筛选条件拼 query 触发 auditExport 下载；行数超限（AUTH_012）提示缩小时间范围；导出成功 toast"导出动作已记入审计"。

## 第 5 节 表单页

### 5.1 初始化向导（init-wizard）— 路由 /init-wizard（全用户公共页，readonly 复用）

**进入条件**：路由守卫调 initCheck：initialized=false 才可进入；其余路由全部重定向至此（api I-01/InitGuard）。已初始化访问 → 重定向 /login。

**表单**（NForm 独立页面，居中卡片）：

| 字段 | 组件 | 校验 | 说明 |
|------|------|------|------|
| 用户名 | NInput | 必填；`[a-zA-Z0-9_.-]{1,32}` | 首个管理员 |
| 密码 | NInput type=password | 必填；≥8 位（A10）；强度弱提示（不阻断，从宽） | — |
| 确认密码 | NInput type=password | 与密码一致（前端比对） | — |
| 启用 TOTP（可选） | NSwitch | — | 默认关；开启展开绑定区 |
| └ 二维码 | otpauthUri → NQRCode | — | ⚠️设计决策：secret 由前端生成（`TotpGenerator.generateSecret()`）并随 initWizard 提交 `totpSecret+totpCode`（字段名与 api 4.4.4 req 对齐，Gate 1 W4 修正）；api 4.4.4 已注明"前端先取得 secret"——备选方案为新增服务端 setup 接口（D3 待修订建议，不阻塞） |
| └ 动态码 | NInput | 6 位数字 | 提交时服务端验证，AUTH_006 提示重输 |

**按钮**：`创建管理员`（提交 initWizard）——成功：Set-Cookie 已生效，toast 欢迎 → 跳转框架首页；AUTH_009（并发后到）→ 提示"面板已初始化"→ 跳 /login。

### 5.2 登录页（login）— 路由 /login（全用户公共页）

**进入条件**：initialized=true（否则重定向 /init-wizard）；已登录访问 → 重定向框架首页。

**结构（v1.6：登录方式两段 + 恢复码降级为恢复手段）**：用户名输入框常驻页首——全部登录形态共用，切换保留已填内容；方式区为分段切换（NTabs type=segment：密码 / passkey——**恢复码不与登录方式并列**，v1.5 的恢复码段移除）；浏览器不支持 passkey 时不渲染分段条，直接呈现密码表单；方式区底部一条安静的恢复入口「登录遇到问题？用恢复码登录」——点击进入独立恢复态：分段条隐藏、用户名保留、恢复码表单 +「返回密码登录」；底部固定说明「本面板不提供注册与找回密码；重置请使用命令行或联系其他管理员。」

**各方式表单与提交**：

| 段 | 字段/动作 | 校验 | 提交 |
|------|------|------|------|
| 密码 | 密码 NInput type=password +「登录」主按钮 | 必填；回车提交 | login → AUTH_007 二步（见下） |
| passkey | 「使用 passkey 登录」主按钮（指纹图标）+ 说明行「验证本机的指纹、面部或安全密钥」 | 未填用户名 → 提示「请先输入用户名」并聚焦用户名框 | webauthnLoginBegin → navigator.credentials.get → webauthnLoginFinish → 会话（无密码） |
| 恢复态（经恢复入口进入，非登录方式） | 恢复码 NInput（占位示例 1A2B-3C4D-5E6F-7G8H）+「登录」主按钮+「返回密码登录」 | 必填（* 标记）；未填提示并聚焦 | recoveryLogin → 会话（一次性消耗） |

**二因素步骤（AUTH_007 后独占内容区，隐藏分段条）**：TOTP 账户——动态码输入框 +「验证动态码」；requiresPasskey 账户（无 TOTP 有 passkey）——「用 passkey 验证」按钮（指纹图标）+ 说明行。两态均提供「返回重新登录」（丢弃内存中的 loginTicket 回到方式选择）；用户名保留并置灰。二步式 ticket 仅内存不落 localStorage（R-02，D-02）。

**错误与锁定**：AUTH_005 通用「用户名或密码错误」（防枚举，api D-06 语境）；AUTH_004 锁定倒计时「账户已锁定，剩余 mm:ss」（remainingSeconds）+ 禁用提交至期满；AUTH_006 动态码错误（计一次失败）；AUTH_013 ticket 失效 → 提示重新登录；AUTH_015 passkey 验证失败（可重试或改用密码）；AUTH_016 恢复码无效或已使用；AUTH_017 账户未注册 passkey（提示改用密码）；AUTH_018 当前地址不支持 passkey（IP 直连，提示绑定域名启用 HTTPS）。

**回车提交**：按当前所处方式/步骤分派——密码段=login、passkey 段=passkey 无密码登录、恢复码段=recoveryLogin、TOTP 步=totpVerify、passkey 二因素步=passkeyVerify。无注册/找回（单机面板，重置经 CLI 或其他管理员）。

### 5.3 个人安全设置（profile-self）— 路由 /profile（admin+readonly 共用，readonly 复用）

**Card 1 修改密码**（3 字段 → Dialog 形态直接内嵌表单）：

| 字段 | 组件 | 校验 |
|------|------|------|
| 旧密码 | NInput type=password | 必填 |
| 新密码 | NInput type=password | ≥8 位（A10） |
| 确认新密码 | NInput type=password | 与新密码一致 |

提交 pwdChange → 成功 toast"密码已修改，其他会话已下线"（api FR-12：保留当前会话）。

**Card 2 TOTP 两步验证**：

| 状态 | 展示 | 操作 |
|------|------|------|
| 未启用 | NTag「未启用」(gray) | 「开始绑定」→ totpSetup → 弹窗显示 NQRCode(otpauthUri) + secret 文本（仅此一次，api I-09）+ 6 位动态码输入 → totpEnable；AUTH_006 重输；成功 NTag→「已启用」(green) |
| 已启用 | NTag「已启用」(green) + 绑定时间 | 「解绑」→ NModal 输入登录密码确认 → totpDisable（api FR-13 密码确认）；成功 NTag→「未启用」 |

**按钮组合**：master_data 惯例 保存/取消（各 Card 独立提交，无跨 Card 草稿态）。

### 5.4 登录安全增量（v1.4，passkey/恢复码；v1.5 登录页交互已并入 5.2）

**登录页（5.2 扩展，v1.5 起以 5.2 分段切换承载，此处保留能力口径）**：
- 「使用 passkey 登录」文字按钮（需先填用户名；浏览器支持 passkey 时显示）：webauthnLoginBegin → navigator.credentials.get → webauthnLoginFinish → 会话（无密码）。
- 密码登录后若返回 `requiresPasskey`（AUTH_007 带标记，无 TOTP 但有 passkey）：显示「用 passkey 验证」按钮 → webauthnLoginBegin → get → passkeyVerify(ticket) → 会话（二因素，与 TOTP 两步同构）。
- 「用恢复码登录」切换：显示恢复码输入框 + 提交 → recoveryLogin → 会话（一次性消耗）。
- 不支持 passkey 的环境（非安全上下文/无域名）隐藏 passkey 按钮；后端对 IP 访问返回 AUTH_018 提示改用 HTTPS 域名。

**个人安全设置（5.3 扩展，Card 3/4）**：
- Card 3 Passkey：列表（名称/添加时间）+「添加 passkey」（register-begin → credentials.create → register-finish）+ 每行删除（NPopconfirm）；不支持时提示。
- Card 4 恢复码：剩余数量 tag +「生成/重新生成」（NPopconfirm 警告旧码作废）→ 弹窗一次性展示 10 个明文码 + 复制；关闭后不可再查看（I-P06 同口径）。

## 第 6 节 详情页

本模块无独立详情路由页（列表行内弹窗承载：新增/重置密码/审计详情 JSON/TOTP 绑定弹窗）。无审批记录 Card、无子表 Card。

## 第 7 节 路由定义

| 路由 | 页面 | meta |
|------|------|------|
| /init-wizard | init-wizard | `{guestOnly:true, initRequired:true}` |
| /login | login | `{guestOnly:true, initializedRequired:true}` |
| / | 框架布局（NLayout：侧边菜单+顶栏） | `{requiresAuth:true}`；子路由挂载点；首页重定向由面板全局路由决定（非本模块范围） |
| /users | user-list | `{requiresAuth:true, title:'用户管理'}`（readonly 可见只读视图） |
| /sessions | session-list | `{requiresAuth:true, title:'在线会话'}` |
| /audit-logs | audit-log-list | `{requiresAuth:true, title:'审计日志'}` |
| /profile | profile-self | `{requiresAuth:true, title:'个人安全设置'}` |

**守卫顺序**（对应 api 3.1 中间件链前端镜像）：路由级 initCheck（首次进入缓存，AUTH_009 语境刷新）→ sessionCurrent 探测（AUTH_001 → /login；AUTH_002 → /login + 提示过期）→ roles 校验（ unauthorized → 403 页提示边界原因 ADR-018）。

**顶栏**（v1.3）：标题=当前页面标题（路由 meta.title 优先、菜单 title 兜底——替换此前硬编码业务域显示名「账户与安全基座」，内部设计术语不泄漏用户 UI，D-P18 框架级职责承载于 PageSpec panel-core v1.2 第 7 节）；右侧当前用户名 + 角色 NTag + `登出`（logout → 成功后清 authStore → 跳 /login；接口 AUTH_002 容错同样跳转）。

**侧边菜单（admin 视角，v1.3 分组化）**：「用户与安全」分组（组内：用户管理 /users、在线会话 /sessions、审计日志 /audit-logs、个人安全设置 /profile；两角色可达——页面内操作差异按 ADR-018 页面级负空间，export 等写入口 admin 专属）——登记经 panel-core ModuleMenuRegistry（api-panel-core.md v2.2 D-P18；v0.1.2 反馈第 6 项：实现侧此前未登记致 /users 不可达，本增量实现回归本节既有设计）。

## 第 8 节 全局一致性
**列表页加载失败态规范（v1.3，v0.1.2 反馈第 5 项「正文空白」修复）**：user-list/session-list/audit-log-list 统一——loadError=true 时渲染 NAlert error「加载失败：{原因}」+「重试」按钮（重新发起本页主查询），**不呈现裸空表格**（此前 user-list loadError 分支无 UI 反馈致正文空白；空态 NEmpty 仅在加载成功且无数据时呈现）。


| # | 检查 | 结果 |
|---|------|------|
| 1 | Action ↔ api-account-auth.md 接口一一对应（S7） | ✅ 18/18 接口全映射，无虚构 |
| 2 | 字典 Key ↔ 04-business-enum.md 定稿枚举一一对应（S8） | ✅ 8/8 |
| 3 | 所有 POST Action 均携带 X-CSRF-Token（拦截器统一注入 authStore.csrfToken） | ✅ |
| 4 | token/secret/ticket 不落 localStorage（R-02） | ✅ csrfToken/ticket 内存态；totp secret 仅展示不持久化 |
| 5 | ADR-018 边界：readonly 角色的按钮隐藏由前端 role 判断 + 后端 AUTH_008 双保险 | ✅ |
| 6 | 锁定/过期/越界错误文案与 api 3.4 错误码一一对应 | ✅ |
| 7 | 字典数据源偏差（无 dict/batch，前端常量）已声明 | ✅ 第 3 节 |
| 8 | 无 📎 Scm* 组件（无 vue3-engine 栈 profile，Naive UI 等价映射） | ✅ |
| 9 | init-wizard TOTP secret 前端生成决策已标注 + D3 待修订建议 | ✅ 第 5.1 节 |
| 10 | 时间展示本地时区、提交 ISO UTC（公共约定） | ✅ |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-12 | initial | 首次生成：7 页面（3 公共/认证页 + 4 功能页）、18 Action、8 字典 Key、3 列表页/4 表单/0 详情路由 |
| v1.1 | 2026-11-12 | field_added | audit-log-list 筛选区补"对象标识"（objectId eq），对齐 API query 修订（Gate 1 W3 修正）；W4 动态码提交字段统一为 totpSecret+totpCode | 4.3 |
| v1.2 | 2026-11-16 | copy_revision | 首启 UI 缺陷修复增量：全局 dark 背景同步/响应式跟随/暗色对比度（App.vue 装配，独立页 5.1/5.2 受益）；5.1/5.2 说明文案净化（去「不阻断/单向/ADR-017/经 CLI」术语表述，功能语义不变）——UI 文案未在本 PageSpec 逐字锁定，Action/契约/校验零变更 | 5.1/5.2 |
| v1.3 | 2026-11-16 | copy_revision / structural | 增量 D4（v0.1.2 用户反馈第 5/6 项）：侧栏菜单分组化登记（「用户与安全」分组四子项两角色，经 panel-core ModuleMenuRegistry 补全——实现回归本节既有设计，修复 /users 不可达）；顶栏标题=当前页面标题（替换硬编码「账户与安全基座」业务域显示名）；路由 meta +title 四项；列表页加载失败态规范（loadError NAlert+重试，修复正文空白缺陷）；Action/字典/接口契约零变化 |
| v1.4 | 2026-11-27 | interface_added / structural | 登录安全增量 D4（数据源 api-account-auth.md v1.3 §4.6，d0-clarification-account-auth-login-security-2026-11-27）：**+10 Action**（passkey 注册 begin/finish、列表、删除、无密码登录 begin/finish、二因素 passkeyVerify、恢复码生成/计数/登录）；5.2 登录页 +passkey 无密码/二因素/恢复码三入口；5.3 个人安全 +Card 3 Passkey 管理 +Card 4 恢复码（一次性展示）；S7 18→28 一一对应；字典/路由零变化 |
| v1.5 | 2026-11-28 | structural | 登录页交互改版（用户拍板「顶部分段切换」）：用户名常驻 + NTabs segment 三段（密码/passkey/恢复码，passkey 不支持时整段隐藏）各配完整表单，同屏单方式；passkey 升级为带指纹图标的主按钮（未填用户名聚焦提示）；恢复码段不再残留密码框；AUTH_007 二因素步骤独占内容区（隐藏分段条）+「返回重新登录」；错误文案页面级补 AUTH_015/016/017/018；回车按方式/步骤分派；Action/字典/路由零变化（S7 28↔28 不变）；readonly v1.3 承载公共页引用 |
| v1.6 | 2026-11-28 | structural | 恢复码降级为恢复手段（用户反馈「恢复码不应该作为一种登录方式，应该是作为恢复手段的」）：方式区改两段（密码/passkey）；恢复码移出分段条——方式区底部安静文字链「登录遇到问题？用恢复码登录」进入独立恢复态（分段条隐藏/用户名保留/恢复码表单+「返回密码登录」）；浏览器不支持 passkey 时整段隐藏直接呈现密码表单（消除单段分段条形态）；回车分派与二因素步骤不变；Action/契约零变化（S7 28↔28）；readonly v1.4 承接 | 5.2 | 5.2 / 5.4 |

> FX 质量清单说明：FX-001~FX-141 逐条清单文件未随引擎 0.5.2 payload 发布（find 验证无 checklist.md），本节按分册 9 节结构逐节落实，并以第 8 节 10 项全局一致性自查替代可机械执行部分；全量 141 项留待 Gate 1 validation 执行。
