# PageSpec · panel-core · 管理员（admin）

| 属性 | 值 |
|------|-----|
| 版本 | v1.0 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-12 |
| 用户类型 | admin（框架级行为：菜单渲染/首访 HTTPS 提示在本节定义，readonly 文档引用本文） |
| 唯一数据源 | 02-detailed/api/api-panel-core.md v1.0 |
| 组件体系 | Vue 3 + Naive UI（knowledge/architecture-choices 栈版本表；无激活栈 profile，不使用 vue3-engine 栈 Scm* 组件） |
| 确认方式 | headless run：7 项交互确认依据 D3 契约 + R-09/R-10 + ADR-018 + FL v1.0 推导，标注于各节 |

## 第 1 节 模块元信息

| 项 | 值 | 确认依据 |
|----|-----|---------|
| 模块编码 | panel-core（域前缀 /panel） | api 文档 3.1 |
| 业务类型 | master_data（panel_config 配置单例：无 MQ、无状态机流转） | 分册 2.1 判据；api 4.1.2 |
| 页面类型 | 全部 config（无 Tab 切换/左树右表/拖拽画布场景；设置页多 Card 内联表单沿用 profile-self 惯例） | 分册 2.2；api-account-auth 5.3 先例 |
| UI 框架 | Naive UI（NForm/NSelect/NSwitch/NInputNumber/NAlert/NTag/NModal/NPopconfirm/NCode） | 栈版本表 |
| 状态管理 | Pinia：panelStore（configDetail/pendingRestart/menuTree）、枚举常量 store（与 account-auth 共用 enums.ts） | 栈版本表 |
| 错误处理 | 统一拦截器按 api 3.4 code 分流：AUTH_001/002→跳登录（复用 account-auth 拦截器）、AUTH_008→边界原因提示（ADR-018）、AUTH_012→fields 逐字段提示、PANEL_001→custom 地址必填、PANEL_002→证书解析/配对失败、PANEL_003→需先导入证书、PANEL_004→"更新检查未开启"、PANEL_005→"版本源不可达"（不自动重试，api D-P09）、PANEL_006→语言取值不支持 | api 3.4 |

## 第 2 节 Action 定义（与 API 一一对应）

| Action | 方法 | 路径 | 页面 | 权限 | CSRF |
|--------|------|------|------|------|------|
| configDetail | GET | /panel/config/detail | settings（+框架 HTTPS 提示派生） | admin+readonly | — |
| configUpdate | POST | /panel/config/update | settings（各 Card 分域提交） | admin | ✓ |
| entryPathRegenerate | POST | /panel/config/regenerate-entry-path | settings·安全入口 Card | admin | ✓ |
| certUpload | POST | /panel/config/upload-certificate | settings·HTTPS Card | admin | ✓ |
| menuList | GET | /panel/menu/list | 框架侧边菜单（全局） | admin+readonly（服务端过滤） | — |
| versionGet | GET | /panel/system/version | about | admin+readonly | — |
| updateCheckRun | POST | /panel/system/update-check | settings·更新检查 Card | admin | ✓ |

> S7 一致性：7 Action 与 api-panel-core.md 7 接口一一对应（一接口一 Action，无未覆盖接口、无虚构接口）。机制型能力（FR-01~03 安装部署、FR-07 首访提示、FR-08 布局骨架、FR-11 认证承载、监听重启 systemctl）无接口即无 Action，见第 7 节框架行为注记。

## 第 3 节 字典 Key 清单

| Key | 取值（=01-solution/04-business-enum.md 定稿） | 用途 |
|-----|--------------------------------------------|------|
| https_mode | off / self_signed / imported | HTTPS 模式下拉 + 模式 tag（colorTag） |
| listen_scope | all / localhost / custom | 监听范围下拉 |
| panel_cert_source | self_signed / imported | 证书摘要展示（来源 tag） |
| config_effect_mode | immediate / restart_required | 生效方式标识（effectMode 渲染） |
| panel_role | admin / readonly | 菜单/按钮可见性判定（authStore 复用） |

lang 为字符串非枚举（MVP 固定 `zh-CN` 单选项，api I-P08；多语言二期再登记 panel_lang）；布尔项 entry_path_enabled/update_check 用 NSwitch（0/1，公共布尔约定）。

**数据源规范偏差声明**（同 account-auth 先例）：不实现 `POST /{domain}/dict/batch`（分册 3.6）——面板单机、枚举全部固定值，字典由前端 TS 常量提供（enums.ts 单一事实源，与 account-auth 共用同一文件）。此偏差为设计决策，待 Gate 1 复核。

**状态颜色方案**（分册 3.1 语义映射）：

| Key::值 | 颜色 |
|---------|------|
| https_mode::off | gray |
| https_mode::self_signed | blue |
| https_mode::imported | green |
| panel_cert_source::self_signed | blue |
| panel_cert_source::imported | green |
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

**提交粒度决策**（确认项 5）：6 个 Card 各自独立提交（同 account-auth profile-self 惯例：无跨 Card 草稿态），每 Card 一个「保存」按钮，仅提交本 Card 字段（api configUpdate 为部分字段集语义）；成功后统一 re-fetch configDetail 刷新 effectMode/pendingRestart。

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

**交互流**：
1. 开关 0→1：NModal 确认（文案含风险提示「开启后未携带入口路径的访问将被拒绝，请妥善保存路径」）→ configUpdate {entryPathEnabled:true} → 若无既有路径服务端自动生成（api 4.1.4）→ 成功后弹 NModal 大字展示路径 + 复制按钮（prd N2 遗忘风险对策）。
2. 开关 1→0：NPopconfirm 确认「关闭后任意路径均可访问面板」→ configUpdate {entryPathEnabled:false}。
3. `重新生成路径`（NButton，enabled=true 时可用）：NPopconfirm 警告「旧路径立即失效，未保存新路径将无法访问」→ entryPathRegenerate → 新路径 NModal + 复制；PANEL_* 失败原路径不变（api 4.1.4）。

修改即时生效（EntryPathGuard 下一请求即用新值，api D-P03）；Card 角标 NTag「即时生效」(green)。审计提示：成功 toast 附「变更已记入审计」（api panel_entry_path_regenerated）。

#### Card 3 HTTPS 设置（https）

| 字段/元素 | 组件 | 说明 |
|----------|------|------|
| HTTPS 模式 | NSelect（https_mode） | off/self_signed/imported 三态（R-10） |
| └ 模式说明 | 内联 hint | self_signed→「保存后系统自动生成自签证书」；imported→展开证书导入区 |
| └ 证书 PEM | NInput type=textarea（monospace） | certPem；仅 imported 展开 |
| └ 私钥 PEM | NInput type=textarea（monospace，粘贴态遮罩） | keyPem；仅 imported 展开；保存成功后清空不回显（api I-P06 零下发） |
| └ 当前证书摘要 | 只读描述列表 | cert 存在时展示：来源 tag（panel_cert_source）/颁发者/有效期（notAfter 过期临近橙色提示）；无「查看私钥」入口（api 3.3 脱敏） |

**按钮**：
- `保存`（configUpdate {httpsMode}）：to imported 前置校验证书已导入（PANEL_003→提示「请先导入证书与私钥」）；to self_signed 成功 toast「已保存，自签证书已生成」（api D-P06 即时生效）；to off 直接生效。
- `导入证书`（certUpload {certPem, keyPem}，仅 imported 展开区可见）：PANEL_002→「证书解析失败或与私钥不配对，请检查 PEM 内容」（R-14）；成功 toast「证书已导入并记入审计」+ 摘要刷新；不改变 https_mode（两接口顺序协作：先导入后切模式，api 2.3）。

即时生效；Card 角标「即时生效」。

#### Card 4 面板参数（lang）

| 字段 | 组件 | 校验 |
|------|------|------|
| 界面语言 | NSelect | 仅 `zh-CN` 单选项（MVP，api I-P08；PANEL_006 兜底） |

**按钮**：`保存`（configUpdate {lang}）→ 即时生效；Card 角标「即时生效」。

#### Card 5 会话参数（只读透传）

| 元素 | 组件 | 说明 |
|------|------|------|
| 绝对过期时长 | 只读文本 | detail.session.absoluteTimeout（默认 12h，ADR-019） |
| 空闲超时时长 | 只读文本 | detail.session.idleTimeout（默认 2h） |
| 维护方式说明 | 内联 hint | 「该参数由账户与安全基座（account-auth）管理：修改配置文件后重启生效；本页仅展示（api D-P08）」 |

无保存按钮（只读透传，PRD FR-14；POST session 字段被服务端拒绝）。

#### Card 6 更新检查（updateCheck）

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
| /settings | settings | `{requiresAuth:true}`（readonly 只读渲染） |
| /about | about | `{requiresAuth:true}` |

**守卫**：复用 account-auth 全局守卫（initCheck → sessionCurrent → roles）；本模块无新增守卫（全部接口要求会话，api D-P11，未登录由全局守卫拦截）。

**框架级行为（本模块作为宿主基座的职责，确认项 7 覆盖声明）**：

| 行为 | 数据源 | 说明 |
|------|--------|------|
| 侧边菜单渲染 | menuList → NMenu 树 | 进入框架后调用一次（D-P07 服务端按角色过滤下发，前端不硬编码菜单、不做二次过滤）；panel-core 自身菜单（面板设置/关于）亦经 ModuleMenuRegistry 构建期登记、由同一接口返回 |
| 首访 HTTPS 建议 | configDetail.https.mode + location.protocol | 见 5.1 页面级横幅（FR-07，无独立接口） |
| 安全入口前缀 | 透明 | 前端不感知 `/{entryPath}` 前缀：静态资源与 API 均相对路径；未携带前缀的请求由 EntryPathGuard 返回标准 404（api D-P03，不进前端拦截器分流；书签丢失前缀 = 看到 404，属预期安全行为） |
| 顶栏 | 复用 account-auth | 用户名/角色/登出（非本模块范围） |

## 第 8 节 全局一致性

| # | 检查 | 结果 |
|---|------|------|
| 1 | Action ↔ api-panel-core.md 接口一一对应（S7） | ✅ 7/7 接口全映射，无虚构 |
| 2 | 字典 Key ↔ 04-business-enum.md 定稿枚举一一对应（S8） | ✅ 4/4（https_mode/listen_scope/panel_cert_source/config_effect_mode）+ panel_role 复用；lang/布尔项非枚举已注明 |
| 3 | 所有 POST Action 均携带 X-CSRF-Token（拦截器统一注入） | ✅ |
| 4 | key_pem/私钥材料不回显不持久化（api I-P06）；检查结果过程数据不落本地存储 | ✅ 5.1 Card 3 |
| 5 | ADR-018 边界：readonly 禁用渲染 + 后端 AUTH_008/PANEL_* 双保险 | ✅（负空间明细见 readonly 文档第 2 节） |
| 6 | PANEL_001~006 + AUTH_008/012 错误文案与 api 3.4 一一对应 | ✅ 第 1 节错误处理行 |
| 7 | 字典数据源偏差（无 dict/batch，前端常量）已声明 | ✅ 第 3 节 |
| 8 | 无 📎 Scm* 组件（无 vue3-engine 栈 profile，Naive UI 等价映射） | ✅ |
| 9 | 无列表页/查询区/表格列的诚实性声明（单例配置无台账） | ✅ 第 4 节 |
| 10 | 时间展示本地时区、提交 ISO UTC（公共约定） | ✅ 5.2 构建信息 |
| 11 | 生效语义三态呈现完整：即时生效 toast / 需重启横幅+角标 / 只读透传无按钮 | ✅ 5.1 各 Card |
| 12 | R-09 纪律：外呼仅经「开关开启 + 显式点击」双门控，无自动轮询/预取外呼 | ✅ 5.1 Card 6 |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-12 | initial | 首次生成：2 页面（settings 6 Card + about）、7 Action、4 字典 Key、0 列表页/2 表单页/0 详情路由；框架级职责（菜单渲染/HTTPS 提示/入口前缀透明）并入第 7 节 |

> FX 质量清单说明：FX-001~FX-141 逐条清单文件未随引擎 0.5.2 payload 发布（本次 find 复核：payload 下无 design/front-spec checklist，frontend-specs 仅存在于未激活的栈 profile vue3-engine/react-vite 下），本节按分册 9 节结构逐节落实，并以第 8 节 12 项全局一致性自查替代可机械执行部分；全量 141 项留待 Gate 1 validation 执行。
