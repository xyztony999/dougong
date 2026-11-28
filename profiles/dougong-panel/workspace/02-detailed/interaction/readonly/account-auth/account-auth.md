# PageSpec · account-auth · 只读用户（readonly）

| 属性 | 值 |
|------|-----|
| 版本 | v1.4 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-28（v1.4 随 admin v1.6 恢复码降级） |
| 用户类型 | readonly |
| 唯一数据源 | 02-detailed/api/api-account-auth.md v1.4 |
| 公共页定义 | 复用 admin 文档（../admin/account-auth/account-auth.md 第 5.1/5.2/5.3 节）：初始化向导、登录页、个人安全设置 |
| 边界依据 | ADR-018：全可看（含审计日志）+ 禁一切变更 + 禁 Web 终端 + 禁上传下载 |
| 组件体系 | Vue 3 + Naive UI（同 admin 文档） |

## 第 1 节 模块元信息

| 项 | 值 |
|----|-----|
| 模块编码 | account-auth |
| 业务类型 | master_data（只读视角） |
| 页面类型 | 全部 config |
| 可见页面 | login（公共）、框架（精简菜单）、session-list（只读视图）、audit-log-list（只读视图）、profile-self（公共复用） |
| 错误处理 | 同 admin 拦截器；AUTH_008 边界提示文案明确"只读角色禁止变更操作"（ADR-018） |

## 第 2 节 Action 定义

| Action | 方法 | 路径 | 页面 | 说明 |
|--------|------|------|------|------|
| login / totpVerify / sessionCurrent / webauthnLoginBegin / webauthnLoginFinish / passkeyVerify / recoveryLogin | — | 同 admin 第 2 节 | 公共页复用 | 登录安全三入口两角色同等（无密码 passkey / 二因素 / 恢复码登录） |
| logout | POST | /auth/session/logout | 顶栏 | ✓CSRF |
| userList | GET | /auth/user/list | user-list | 只读视图 |
| sessionList | GET | /auth/session/list | session-list | 只读视图 |
| auditList | GET | /auth/audit-log/list | audit-log-list | 只读视图（ADR-018 含审计） |
| pwdChange / totpSetup / totpEnable / totpDisable / webauthnRegisterBegin / webauthnRegisterFinish / webauthnList / webauthnDelete / recoveryGenerate / recoveryCount | POST/GET | 同 admin 第 2 节 | profile-self | 自助凭证操作不受变更禁令限制（FL 附录B 行业惯例）；passkey/恢复码管理为本人自助 |

**负空间（readonly 不可见 Action，ADR-018 矩阵）**：userCreate / userDelete / userResetPwd / sessionRevoke / auditExport——前端按 role===admin 隐藏入口 + 后端 AUTH_008 双保险；越界直达请求返回 403 语义页提示边界原因。

## 第 3 节 字典 Key 清单

与 admin 文档第 3 节完全一致（同一 enums.ts 常量源）：panel_role / panel_user_status / session_status / session_end_reason / audit_action / audit_result / audit_source / init_status；颜色方案同源。

## 第 4 节 列表页

### 4.1 用户管理（user-list，只读视图）— 路由 /users

- 查询条件/表格列：与 admin 版 4.1 完全一致（ADR-018 全可看）。
- **差异**：无工具栏（无「新增用户」）；无操作列（operList 为空 → 不渲染 action 列）。

### 4.2 在线会话（session-list，只读视图）— 路由 /sessions

- 查询条件/表格列：与 admin 版 4.2 一致。
- **差异**：无操作列（无「强制下线」按钮；isCurrent 标记列保留）。

### 4.3 审计日志（audit-log-list，只读视图）— 路由 /audit-logs

- 查询条件/表格列/分页/详情 JSON 弹窗：与 admin 版 4.3 一致（含审计查看，ADR-018）。
- **差异**：无工具栏（无「导出 CSV」——解释性决策：导出视为数据取出，从紧把握，FL 附录B）。

## 第 5 节 表单页

- **登录页 / 初始化向导 / 个人安全设置**：与 admin 版第 5.1/5.2/5.3/5.4 节定义完全一致（公共页，readonly 文档不重复定义；修改密码/TOTP/passkey/恢复码为自助凭证操作，不受变更禁令限制；登录页 passkey 按钮按环境支持度显示，v1.2 随 admin v1.4）。

## 第 6 节 详情页

无独立详情路由（同 admin，行内弹窗承载）。

## 第 7 节 路由定义

| 路由 | meta | 与 admin 差异 |
|------|------|--------------|
| /login、/init-wizard、/profile | 同 admin | 无 |
| /users、/sessions、/audit-logs | `{requiresAuth:true}` | 同路由只读渲染（按 authStore.user.role 分支） |
| 菜单 | 同四项 | 无管理动作入口 |

## 第 8 节 全局一致性

| # | 检查 | 结果 |
|---|------|------|
| 1 | Action 与 admin 文档/API 文档一致，无 readonly 专属虚构接口 | ✅ |
| 2 | 负空间 5 个 Action 与 ADR-018 边界矩阵逐项对应 | ✅ 第 2 节 |
| 3 | 只读视图与 admin 视图共用列定义（无列差异、仅按钮差异） | ✅ |
| 4 | CSRF/token 纪律同 admin（第 8 节 3/4 项） | ✅ |
| 5 | 自助凭证操作纳入范围且标注依据 | ✅ 第 2 节 |
| 6 | 字典同源 enums.ts | ✅ |
| 7 | 登录安全自助 Action（passkey/恢复码）两角色同等，未误入负空间 | ✅ 第 2 节 |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-12 | initial | 首次生成：只读视角 3 列表页（去操作列/导出）+ 公共页引用；负空间 5 Action 标注 |
| v1.1 | 2026-11-16 | copy_revision | 增量 D4 随 admin v1.3：侧栏「用户与安全」分组只读可达（用户管理只读视图/在线会话/审计日志/个人安全设置，两角色同组）；顶栏标题=当前页面标题（D-P18）；列表加载失败态 NAlert+重试同 admin 规范；负空间与 Action 零变化 |
| v1.2 | 2026-11-28 | interface_added | 登录安全增量随 admin v1.4/api v1.3~v1.4：公共页引 admin §5.4（登录页 passkey 无密码/二因素/恢复码三入口；个人安全 Passkey 卡与恢复码卡）；第 2 节 Action 补齐 10 个登录安全动作（登录侧 3 + profile-self 侧 7，均两角色同等自助）；负空间仍 5 个（登录安全不含 admin 专属动作）；字典/路由零变化 |
| v1.4 | 2026-11-28 | structural | 随 admin v1.6 恢复码降级：公共页引用自动承载——恢复码移出分段条，经「登录遇到问题？用恢复码登录」进入独立恢复态（两角色同等自助）；Action/负空间零变化 | — |
| v1.3 | 2026-11-28 | structural | 随 admin v1.5 登录页分段切换改版：公共页引用（admin 第 5.1/5.2/5.3/5.4 节）自动承载——三方式分段切换、passkey 指纹主按钮、二因素步骤独占区与「返回重新登录」对 readonly 同样生效（登录方式与二因素属公共认证流，无角色差异）；Action/负空间零变化 | — |
> FX 清单执行方式同 admin 文档第 9 节注记（清单文件未随 payload 发布，按 9 节结构落实 + 第 8 节自查；全量留待 Gate 1）。
