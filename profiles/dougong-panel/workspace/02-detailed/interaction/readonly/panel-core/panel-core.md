# PageSpec · panel-core · 只读用户（readonly）

| 属性 | 值 |
|------|-----|
| 版本 | v1.4 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-24 |
| 用户类型 | readonly |
| 唯一数据源 | 02-detailed/api/api-panel-core.md v2.4 |
| 公共页定义 | 复用 admin 文档（../admin/panel-core/panel-core.md 第 7 节）：框架级行为（菜单渲染/首访 HTTPS 提示/入口前缀寻址〔服务端注入 `<base>` + 前端相对寻址，v1.4〕/页面切换的加载反馈与失败恢复/弹窗抽屉关闭期间的侧栏点击，v1.4） |
| 边界依据 | ADR-018：全可看（设置项当前值全部可见，含安全入口路径值——FL 附录B 决策）+ 禁一切变更 + 禁 Web 终端 + 禁上传下载 |
| 组件体系 | Vue 3 + Naive UI（同 admin 文档） |

## 第 1 节 模块元信息

| 项 | 值 |
|----|-----|
| 模块编码 | panel-core |
| 业务类型 | master_data（只读视角） |
| 页面类型 | 全部 config |
| 可见页面 | 框架（menuList 过滤后菜单）、settings（只读渲染）、about（无差异） |
| 错误处理 | 同 admin 拦截器；AUTH_008 边界提示文案明确「只读角色禁止变更操作」（ADR-018） |

## 第 2 节 Action 定义

| Action | 方法 | 路径 | 页面 | 说明 |
|--------|------|------|------|------|
| configDetail | GET | /panel/config/detail | settings | 只读渲染数据源（含安全入口路径值与绑定域名，ADR-018 全可看） |
| certStatus | GET | /panel/cert/status | settings·面板证书 Card | 面板证书状态只读查看（v1.1 新增；来源/状态/有效期/失败原因可见，ADR-018 全可看） |
| menuList | GET | /panel/menu/list | 框架侧边菜单 | 服务端已按角色过滤（api D-P07），前端直接渲染 |
| versionGet | GET | /panel/system/version | about | 与 admin 一致 |

**负空间（readonly 不可见 Action，对应 api 3.3 权限矩阵 ✗ 项 + ADR-018）**：configUpdate（含域名绑定/解绑字段）/ entryPathRegenerate / certUpload / certRequest / certRetryRenewal / updateCheckRun——前端按 role===admin 隐藏入口 + 后端 AUTH_008 双保险；越界直达请求返回 403 语义页提示边界原因。特别地：更新检查「手动触发」、面板证书「申请」与「手动重试续期」对 readonly 不可见（产生外呼/变更的动作从变更边界从紧把握，FL 附录B 解释性决策 + api 权限矩阵从紧注）；各开关与状态查看态本身可见（控件禁用而非隐藏）。

## 第 3 节 字典 Key 清单

与 admin 文档第 3 节完全一致（同一 enums.ts 常量源）：https_mode / listen_scope / panel_cert_source / config_effect_mode（+panel_role 复用）；颜色方案同源。

## 第 4 节 列表页

无列表页（同 admin 第 4 节声明：单例配置无台账场景）。

## 第 5 节 表单页

### 5.1 面板设置（settings，只读渲染）— 路由 /settings

页面级横幅（pendingRestart 警示 / HTTP 明文建议）与 admin 一致——两者均为全局状态展示，非变更操作。

与 admin 版 5.1 的差异（8 Card 逐项）：

| Card | 与 admin 差异 |
|------|--------------|
| 监听设置 | 全部控件 disabled；无「保存」按钮；「需重启」角标与 pendingRestart 横幅保留（状态可见） |
| 安全入口 | 开关 disabled（查看开关状态）；路径值完整可见（ADR-018 全可看，FL 附录B 决策：收紧属 ADR-018 修订事项）；无「重新生成路径」按钮 |
| 域名绑定（v1.1） | 当前绑定状态可见（detail.bindDomain.value，null=NTag「未绑定」）；输入框 disabled；无「绑定」「解绑」按钮（负空间：configUpdate） |
| HTTPS 设置 | 模式下拉 disabled（四态含 acme 可见）；证书导入区不渲染（无 PEM 输入框）；证书摘要（来源/颁发者/有效期）可见；无「导入证书」「保存」按钮 |
| 面板证书（v1.1） | certStatus 只读渲染：ACME 域名/挑战方式/自动续期说明/证书摘要（来源+状态 tag/颁发者/有效期）/lastError 可见；无「申请证书」「手动重试续期」按钮（负空间：外呼触发从紧，见第 2 节） |
| 面板参数 | 语言下拉 disabled（当前值 zh-CN 可见）；无「保存」按钮 |
| 会话参数 | 与 admin 完全一致（本就只读透传，无差异） |
| 更新检查 | 开关状态可见（disabled NSwitch）；无「检查更新」按钮（负空间：外呼触发从紧，见第 2 节） |

### 5.2 关于（about）— 路由 /about

与 admin 版 5.2 完全一致（纯展示页，无差异）。

## 第 6 节 详情页

无独立详情路由（同 admin）。

## 第 7 节 路由定义

| 路由 | meta | 与 admin 差异 |
|------|------|--------------|
| /settings、/about | `{requiresAuth:true}` | 同路由只读渲染（按 authStore.user.role==='readonly' 分支禁用控件/隐藏按钮） |
| 框架菜单 | — | menuList 返回即为本角色可见集（服务端过滤），无需前端二次处理 |
| 页面切换的加载反馈与失败恢复、弹窗抽屉关闭期间的侧栏点击 | — | 与 admin 完全一致（框架级机制，两角色同行为；文案与规则见 admin 文档第 7 节 v1.3） |

## 第 8 节 全局一致性

| # | 检查 | 结果 |
|---|------|------|
| 1 | Action 与 admin 文档/API 文档一致，无 readonly 专属虚构接口 | ✅ 4 个只读 Action（configDetail/certStatus/menuList/versionGet）均为 admin 文档子集 |
| 2 | 负空间 6 个 Action 与 api 3.3 权限矩阵 ✗ 项逐项对应 | ✅ 第 2 节 |
| 3 | 只读视图与 admin 视图共用 Card/字段定义（无字段差异、仅控件禁用与按钮差异） | ✅ 第 5 节（8 Card） |
| 4 | 安全入口路径值可见性已标注依据（ADR-018 全可看 + FL 附录B） | ✅ 第 5.1 节 |
| 5 | 更新检查/面板证书申请/手动重试触发负空间已标注依据（FL 附录B 解释性决策 + 外呼从紧） | ✅ 第 2/5.1 节 |
| 6 | CSRF/敏感信息纪律同 admin（第 8 节 3/4 项）；证书导入区在只读视图不渲染 | ✅ |
| 7 | 字典同源 enums.ts（v1.1 含 cert_status 引用与 https_mode/panel_cert_source 扩展 acme） | ✅ |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-12 | initial | 首次生成：只读视角 settings（6 Card 全可见、控件禁用、4 Action 负空间）+ about（无差异）；公共框架行为引用 admin 文档第 7 节 |
| v1.1 | 2026-11-16 | interface_added / field_added | 增量 D4（ADR-028/029，数据源 api-panel-core.md v2.0，同步 admin 文档 v1.1）：只读 Action 3→4（+certStatus 面板证书状态查看）；负空间 4→6（+certRequest/certRetryRenewal，configUpdate 含域名绑定/解绑字段已在负空间内）；差异表 6→8 Card（+域名绑定只读行、+面板证书只读行）；字典同源扩展（cert_status 引用、https_mode/panel_cert_source +acme） |
| v1.2 | 2026-11-16 | copy_revision / structural | 增量 D4 随 admin v1.2（数据源 api v2.2）：设置页四分组重排同 admin 视角（只读控件禁用不变）；顶栏标题=当前页面标题（D-P18 框架级，两角色一致）；Action/负空间/字典零变化 |
| v1.3 | 2026-11-24 | rule_added | 增量 D4 随 admin v1.3（数据源 api v2.3）：第 7 节登记页面切换的加载反馈与失败恢复、弹窗抽屉关闭期间的侧栏点击（两角色一致，规则见 admin 文档第 7 节）；Action/负空间/字典/页面结构零变化 |
| v1.4 | 2026-11-24 | rule_changed | 增量 D4 随 admin v1.4（数据源 api v2.4 D-P20，缺陷「安全入口开启后面板在浏览器里打不开」修复）：第 7 节「安全入口前缀」由「透明」改为实际机制「服务端注入 `<base>` + 前端相对寻址」（两角色一致，规则见 admin 文档第 7 节）；readonly 对安全入口 Card 仍全可见、控件禁用（开关/重新生成在负空间内，整页跳转交互仅 admin 触发）；Action/负空间/字典/页面结构零变化 |

> FX 清单执行方式同 admin 文档第 9 节注记（清单文件未随 payload 发布，按 9 节结构落实 + 第 8 节自查；全量留待 Gate 1）。
