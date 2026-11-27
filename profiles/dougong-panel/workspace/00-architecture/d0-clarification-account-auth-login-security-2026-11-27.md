# D0 需求澄清报告：登录安全适配（passkey / 恢复码 / 登录保护）· account-auth

> 引擎 design 阶段 D0 产物（2026-11-27，增量澄清）。
> 来源：用户「接下来，需要给登录界面做下安全适配，目前只能是用户名和密码，可以使用 passkey、其它安全选项也可以讨论下」。
> 执行方式：多代理模式（本会话无工作流工具）；WARN 策略：继续并记录。

## 1. 现状（取证）

- 登录 = 用户名 + 密码（失败计数锁定 R-01）；**可选 TOTP 两步式**（D-02：启用 TOTP 的用户密码后签发一次性 login_ticket，AUTH_007 → totp-verify）。
- 凭据存储：`panel_user`（password_hash）+ `panel_user_totp`（1:1，enabled 待确认态）。
- **无 passkey/WebAuthn、无恢复码**。会话 Cookie+CSRF+RBAC 不变（R-02/ADR-018）。
- 初始化双入口（向导/CLI）可选 TOTP 激活。

## 2. 反馈项结构化

| # | 类型 | 模块 | 内容 | 处置 |
|---|------|------|------|------|
| F1 | 功能增强 | account-auth | 登录仅密码（+可选 TOTP），缺 passkey 等现代安全选项 | 增量：D0→D1-D4→Gate1→B/F→Gate2 |

## 3. 用户拍板（2026-11-27，AskUserQuestion）

| # | 决定 | 选择 |
|---|------|------|
| Q1 | passkey 形态 | **两者都支持**：无密码登录（注册后免密码）+ 作为第二因素（密码后 passkey 确认，同 TOTP 两步同构） |
| Q2 | 附加能力 | **恢复码**（passkey/TOTP 丢失时一次性登录）+ **登录保护强化**（新设备告警/失败锁定调优） |
| Q3 | 执行方式 | 多代理模式 |
| Q4 | WARN 策略 | 继续并记录 |

## 4. 范围与边界

**做**：
1. WebAuthn/passkey：注册（attestation）+ 断言（assertion）；凭据表 `panel_user_webauthn`（1:N，credential_id/public_key/sign_count/aaguid/name/created）；无密码登录流 + 第二因素流（复用 login_ticket 两步同构）。
2. 恢复码：`panel_user_recovery_codes`（哈希存储、一次性、用尽作废）；生成（注册 passkey/启用 TOTP 时可选展示一次）+ 恢复码登录流（消耗一码）。
3. 登录保护强化：新设备识别（设备指纹 cookie/UA 哈希）+ 新设备登录审计/告警标记；失败锁定参数调优（沿用 R-01 计数，阈值/窗口可配或常量）。
4. 前端：登录页 passkey 按钮（navigator.credentials.get）+ 第二因素 passkey 步；个人安全设置页 passkey 管理（注册/列表/删除）+ 恢复码展示/重生成；初始化向导可选 passkey 暂不做（管理页注册）。

**不做**（本期）：
- 邮箱 OTP（无邮件基础设施）；安全问题（不安全）；passkey 跨设备同步管理（平台负责）；初始化向导 passkey（管理页注册即可）。
- 不改动会话 Cookie/CSRF/RBAC 架构（R-02/ADR-018）。

## 5. 约束

| # | 约束 |
|---|------|
| C1 | WebAuthn 依赖 HTTPS（或 localhost）；面板已支持 HTTPS（imported/acme/self_signed），CF 橙云回源 TLS 满足；RP ID = 绑定域名或访问 Host |
| C2 | 凭据/恢复码**哈希或公钥存储**，私钥永不落库；恢复码只展示一次（I-P06 同口径） |
| C3 | 恢复码一次性、用尽作废；与 TOTP/passkey 互备，防锁死 |
| C4 | 无密码登录仍受失败计数/锁定（R-01）与审计（R-03）约束 |
| C5 | 后端 WebAuthn 库选型：go-webauthn/webauthn（纯 Go，无 CGO）；前端原生 navigator.credentials，无新依赖 |
| C6 | 迁移新增表（webauthn/recovery/设备指纹），表重建须带数据升级测试（[[sqlite-rebuild-upgrade-test]]） |
| C7 | 浏览器级验收：注册 passkey→无密码登录→第二因素→恢复码登录→新设备告警，真实浏览器走一遍 |

## 6. 候选与推荐

- WebAuthn 库：go-webauthn/webauthn（成熟、纯 Go）vs 自研协议（风险高）→ 推荐前者。
- 无密码流：username-less（discoverable credential, resident key）vs 先输用户名再 passkey → 推荐**先输用户名再 passkey**（与现有用户名入口同构、避免枚举差异），同时支持 resident key 免用户名（可选）。简化：本期先做「用户名+passkey」无密码与「密码+passkey」二因素；resident-key 免用户名列为可选增强。
- 恢复码：明文一次性展示+哈希落库（业界标准）。

## 7. 实施要点（D1-D4 依据）

1. 后端：webauthn 适配器（adapter/webauthnx）+ service/auth 扩展（注册/断言/恢复码/设备指纹）+ 新端点（/auth/webauthn/register-begin·finish、/auth/webauthn/login-begin·finish、/auth/recovery-codes/*、/auth/session/passkey-*）+ 迁移。
2. 前端：api/auth 扩展 + 登录页 passkey 流 + 个人安全设置 passkey/恢复码管理。
3. 设计：account-auth BA/AA/FL、PRD、API、PageSpec 增量。
4. 验收：真实浏览器（无头 Edge 支持 WebAuthn 虚拟认证器 CDP WebAuthn.enable/addVirtualAuthenticator）走注册/登录/恢复码全流程。