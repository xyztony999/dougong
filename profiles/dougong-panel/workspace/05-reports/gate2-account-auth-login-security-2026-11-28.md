# Gate 2 报告 — account-auth 登录安全增量实现（v0.6.3 ~ v0.6.5）

- **日期**：2026-11-28（对 2026-11-27 已发布的三版做集中验收登记；增量经 /pipeline 多代理推进，后端子代理配额中断后主会话接手完成）
- **覆盖提交**：d37e974（v0.6.3 代码）/ d254183（设计文档）/ f3ebf2f（v0.6.3 CHANGELOG）/ bf34094（v0.6.4 Origin 放宽）/ 41ac4c5（v0.6.5 flags 持久化+自愈，tag=此）
- **发版事实**：三版均 Release+CI 双绿、Latest 指向 v0.6.5、产物（linux amd64/arm64 + sha256sums）与 CHANGELOG 正文核验过

## 1. 编译门控（最终代码 41ac4c5 主会话实跑）

| 项 | 结果 |
|----|------|
| gofmt -l / go vet ./... | ✅ 0 / 0 |
| go test -race ./... -count=1（全仓 55 包） | ✅ 全 ok 0 FAIL |
| 交叉编译 linux amd64 / linux arm64 / windows amd64 | ✅ 全 0（CGO_ENABLED=0） |
| WSL 实跑 Linux 测试二进制（auth / webauthnx / store 三包） | ✅ 全 PASS（store 测试需带包内 migrations 目录——按设计从包 CWD 读） |
| engine-cli check（C-01~C-11） | ✅ 11 pass / 0 warn / 0 fail |
| 前端 vitest / vue-tsc / vite build | v0.6.3 轮实跑 exit 0（540 用例）；v0.6.4/v0.6.5 纯后端零前端改动未重跑（沿绿态，如实声明） |

## 2. 增量测试与变异

- **adapter/webauthnx selfheal_test.go（软认证器，4 例）**：CDP 虚拟认证器无法置 BackupEligible 位，BE 类缺陷只能在协议层复现——测试手工 CBOR 编码 attestation object（「none」格式，注册 authData 必带 AT 位 0x40）并以 ECDSA-P256 真实签名 assertion，跑 go-webauthn 真库校验：
  1. TestFinishRegistrationReturnsAuthenticatorFlags（注册返回 flags 原始字节 0x4D）
  2. TestFinishLoginSelfHealsLegacyZeroFlags（旧行 flags=0 + BE=1 断言 → 自愈成功并回写 0x0D、signCount=2）
  3. TestFinishLoginRejectsBackupEligibleMismatchWhenFlagsStored（已存 flags 而 BE 不一致 → 仍拒，错误串精确匹配）
  4. TestFinishLoginNormalPathWithPersistedFlags（正常路径 + 错钥签名被拒）
- **service/auth TestWebauthnFlagsPersistedAndWrittenBack**：注册落库 flags=0x4D → 密码+二因素登录 → flags=0x0D/signCount=7/last_used 回写（真实 :memory: SQLite 全链）。
- **变异检验 7 处全杀**：删自愈块 / 去 allFlagsZero 守卫（非旧行被误自愈）/ toCreds 不还原 / 注册不落 flags / Create 不落 flags / scan 不还原 / Touch 不更新 flags 列。
- v0.6.3 轮既有：login_security_test 8 函数（恢复码全生命周期/新设备/二因素/无密码/列表删除/Origin 放宽）+ 虚拟认证器浏览器 6/6。

## 3. 安全专项（逐项取证）

- **S1 凭据材料**：私钥永不落库（服务端只存公钥/flags/sign_count，api §4.6 口径）；恢复码 sha256 哈希、明文一次性返回。
- **S2 自愈边界**：自愈仅当「校验失败原因=BE 不一致」且「存量 flags 全零」双条件成立；已存 flags 的 BE 不一致照旧拒绝（换认证器/克隆不误放行）——变异 2（去守卫）被测试杀死即此边界的回归锁。
- **S3 Origin 放宽边界**：仅「同主机」多形态（scheme × 端口），主机匹配不放松；TestWebAuthnProviderTolerantOrigins 断言四形态在列。
- **S4 ticket 一次性**：login_ticket 消费后重放被拒（TestPasskeyReuse 断言）；webauthn session token 5min TTL + 一次性取出。
- **S5 暴力破解面**：passkey/恢复码失败均计 R-01 锁定计数（recordLoginFailure stage=passkey）；恢复码错误尝试同口径。
- **S6 审计**：注册/删除/生成/恢复码登录/新设备均入审计（webauthn_registered/webauthn_deleted + 登录流 detail.new_device）。

## 4. 实现↔设计逐项取证（Gate 1 时序倒置的对冲）

| 设计条目 | 实现取证 |
|---------|---------|
| api §4.6 10 接口 | api/auth/routes.go 路由逐一在位；PageSpec 28↔28（Gate1 M2） |
| requiresPasskey 二因素 | login.go CountByUser>0 → ticket + LoginOutcome.RequiresPasskey；handler 发 AUTH_007 变体 |
| 恢复码 10 个一次性 | recovery.go GenerateRecoveryCodes/RecoveryLogin（ReplaceUnused 同事务审计） |
| 新设备识别 | service.go issueSessionWithLoginAudit → tx.Devices() upsert + NewDevice |
| flags 持久化+自愈 | 迁移 020 + webauthn_repo 全链 + webauthnx FinishLogin 4 返回 + isBackupEligibleInconsistency/allFlagsZero 双守卫 |
| 迁移 019/020 | 逐字对照 api §4.6 DDL；020 ALTER ADD COLUMN DEFAULT 0 |

未发现实现偏离设计（此前 v0.5.1 曾出现的「实现偏离 PageSpec」类问题本轮无）。

## 5. 实机验收（无头 Edge 158 + CDP）

- **passkey 全流程 6/6**：注册并列表 / 生成 10 恢复码（抓取明文）/ 无密码登录 / 密码后进二因素 / 二因素登录 / 恢复码登录。
- **旧行自愈 7/7（phase-legacy.cjs）**：注册落库 flags=69(0x45) → sqlite3 清零+清 sign_count 模拟 v0.6.4 旧行 → **不重新注册**无密码登录成功 → flags=1/sign_count=2/last_used_at 回写。
- **升级重放**：删 flags 列 + 删 schema_migrations 行（Windows sqlite3 服务运行中写入）→ 新二进制启动自动补 020、旧行 sign_count=3 保留 flags=0。
- **用户服务器实机**：v0.6.4 修 AUTH_015 Origin 面（用户 journalctl 取得真实错误后定位）、v0.6.5 修 BackupEligible flags 面（用户提供日志 `Backup Eligible flag inconsistency detected during login validation` 即本缺陷直接证据）。

## 6. 遗留 WARN（如实）

- **W1** readonly 角色未在浏览器单独走 passkey 流——公共页/自助实现两角色同代码路径，admin 视角已验；负空间判定经 Gate1 M7 机械核对。待有 readonly 账号实机时补验。
- **W2** 真实认证器（手机/密码管理器同步 passkey）路径待用户服务器升级 v0.6.5 后验证——软认证器协议测试 + 虚拟认证器已覆盖协议面，真实设备 BE=true 场景即用户复验。
- **W3** 覆盖率余量沿项目先例（service/auth 增量面已由 8+5 测试函数与 7 变异覆盖核心分支）。
- **W4** globalSessions 为进程内存（重启丢进行中 begin/finish，5min TTL 自清扫）——设计如此（api §4.6 已声明），多实例部署不适用（当前单机部署形态）。
- **W5** Windows 本机 -race 连跑偶发计时敏感超时（appmgmt 已知观察项，非本增量；单跑/CI 全绿）。

## 7. 判定

**WARN**（CRITICAL 0 / 契约断裂 0 / 安全专项全实证；W1~W5 待用户确认或自然闭环——W2 随用户升级复验）
