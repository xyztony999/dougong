# 交付报告 — 环境安装后配置就绪（post-install bootstrap）

| 属性 | 值 |
|------|-----|
| 日期 | 2026-11-17 |
| 轮次 | /pipeline 增量轮（env-mgmt 主导 + database-mgmt OHS 联动） |
| 产品定调 | 「好面板的标准是装完即配置就绪，解决 Linux 运维配置难题」 |
| 实证痛点 | DBM_005 诊断轮终裁：apt 装完 MySQL 只有 root@localhost（auth_socket 无密码），面板 TCP+密码连不上，用户无法自助建第一个账户——装完 ≠ 可用 |
| 本轮目标 | 环境安装后自动引导配置到面板可直接治理（装完即可管） |
| 执行轨 | session（无 CreateWorkflow/Agent 工具，工作流/subagent 轨降级） |
| 发版决策 | **用户拍板「先不发」**（继续攒批，下一批可能 PHP 多版本）；CHANGELOG v0.1.8 仅草稿，不执行 git |

## 1. 各阶段 / Gate 结果

| 阶段 | 产物 | 结果 |
|------|------|------|
| D0 需求澄清 | d0-clarification-2026-11-17-bootstrap.md；拍板 Q1~Q4（经主会话 AskUserQuestion 用户确认，均采纳推荐） | 完成 |
| D1 架构设计 | BA/AA/FL env-mgmt v1.1（L4 2.2.1.4 安装后配置就绪 / C-14-04 机制型 / ⚠️A11~A15 / 补录 A8~A10 / 七环境同步 / env-mgmt→db-mgmt OHS 硬依赖） | 完成 |
| D2 PRD | prd-env-mgmt v1.2（FR-07 + OUT 二期 PHP 多版本/MySQL 升级登记）；domain-model v1.22（env_install_task +bootstrap_status/note，非新聚合根/非新状态机）；枚举池 +bootstrap_status；doccenter 再生成（12 业务域 rich） | 完成（G 5/5·T 6/6·N 4/4） |
| D3 API | api-env-mgmt v1.2（D-15~D-19 + 迁移 012 + task DTO +引导字段 + BootstrapGateway 消费契约）；api-database-mgmt v2.2（§4.6/D-20 BootstrapAfterInstall OHS + 引擎引导窄接口 + instance_bootstrapped） | 完成 |
| D4 交互 | PageSpec admin/readonly v1.3（字典 +bootstrap_status S8 6/6 + 任务列表引导列 + 详情引导横幅；Action/路由零变更 S7 10/10） | 完成 |
| **Gate 1** | gate1-env-bootstrap-inc-2026-11-17.md | **PASS**（WARN→用户确认 W1~W5 全过；五项硬检查全过/契约零差） |
| B1-B7 后端 | 迁移 012 + db-mgmt OHS + 适配器引导窄接口 + env-mgmt worker 消费 + 测试全注入 | 完成 |
| F1-F4 前端 | enums/types + 任务列表引导列 + Drawer 引导横幅 + vitest | 完成 |
| 编译门控 | 九项实跑 | 全绿 |
| **Gate 2** | gate2-env-bootstrap-inc-2026-11-17.md | **PASS**（WARN→用户确认 W1~W4 全过；CRITICAL 0/契约零差/门控全绿） |

## 2. 架构

**跨模块单向 OHS（与 cron-backup 消费 DumpStream 同构）**：

```
env-mgmt worker（安装/升级包操作成功后）
  │ ① 服务就绪：SysdGateway 幂等 start 目录 DB 单元（⚠️A15，enable 不强制）
  │ ② 消费 db-mgmt 进程内 OHS（单向依赖 env-mgmt→db-mgmt，无环）
  ▼
db-mgmt BootstrapAfterInstall(ctx, engine, actorName) → BootstrapOutcome{Status, Note, InstanceID}
  ① 幂等前置：本机默认 identity 已 credentialsSet → skipped（不覆盖用户凭据）
  ② 引导通道建账户/设密码（引擎窄接口，有界重试等待就绪 BOOTSTRAP_READY_TIMEOUT=60s/2s）
  ③ 治理通道 TCP Ping 验证新凭据（失败 → failed，不写未验证凭据，D-03 哲学）
  ④ CredentialVault 加密 UPSERT db_instance + reachable + version + 审计 instance_bootstrapped（同事务脱敏）
  ▼
env-mgmt 引导结果回写任务台账（bootstrap_status/note 随终态同事务定格 I-09）
  · 引导失败不改包安装 succeeded（独立维度 ⚠️A14）
  · env_bootstrap 审计（detail 不含密码）
```

**职责切分（四层分界）**：触发与结果呈现（worker 后置/服务就绪/任务台账）归 env-mgmt；建账户/设密码/注册实例与凭据归 db-mgmt 实例层（持适配器+CredentialVault+InstanceRepository）。db_instance 零 DDL 变更（复用 cred_cipher 密文列）；env_install_task +bootstrap_status/note 两列（迁移 012 ADD COLUMN）。无新 HTTP 接口（引导=worker 机制+OHS）。

## 3. 三引擎引导通道（区别于 TCP+密码治理通道）

| 引擎 | 引导通道 | 账户/配置 | 幂等保守 |
|------|---------|----------|---------|
| MySQL | unix socket 以 root auth_socket 免密建连（面板 root 运行，mysql 客户端子进程亦 root；MYSQL_PWD 空注入 auth_socket 忽略；DISCOVERY_PROBE_SET socket 候选逐一尝试） | CREATE USER **dougong@localhost** IDENTIFIED BY 随机强密码 + **GRANT ALL PRIVILEGES ON *.* WITH GRANT OPTION**（Q1/Q2；支撑 db-mgmt 三档授权 all 档；**不动 root auth_socket**） | 账户已存在 → skipped（不重置密码） |
| Redis | redis-cli 免认证连 localhost:6379（fresh install 无 requirepass；REDISCLI_AUTH env 注入） | CONFIG GET requirepass 判未设 → **CONFIG SET requirepass 随机密码 + CONFIG REWRITE 持久化**（Q4）+ CONFIG GET bind 校验本机 | requirepass 已设 → skipped（不覆盖） |
| MongoDB | mongo-driver 免认证连 localhost:27017（**localhost exception**：无用户时可经 localhost 建首个 admin 用户） | admin 库 createUser **dougong root 角色**随机密码 | 已有用户/已开鉴权（Unauthorized/AuthRejected/UserExists）→ skipped |

**安全**：随机强密码 crypto/rand 24B→base64url 32 字符（不弱默认）；AES-256-GCM 密文落 db_instance.cred_cipher（db-cred.key 0600）；账户 host 限本机、bind 本机不公网暴露；**密码永不回显**（BootstrapOutcome.Note/审计 detail/任务输出/响应/日志均不含密码）；命令字面量参数数组 R-08（引擎侧 SQL quoteString A-DB3）。

**已知权衡（W1/W3）**：dougong@localhost 依赖 MySQL 默认 skip-name-resolve=OFF（fresh install 态 TCP 127.0.0.1/::1 经名字解析匹配 localhost）；用户开启 skip-name-resolve 后 TCP 落空 → 引导验证失败如实呈现（实机冒烟验证默认态）。

## 4. 文件清单（本轮）

#### 后端新增
- `backend/internal/store/migrations/012_env_bootstrap.sql`
- `backend/internal/service/dbmgmt/bootstrap.go` + `bootstrap_test.go`
- `backend/internal/service/envmgmt/bootstrap.go` + `bootstrap_test.go`
- `backend/internal/adapter/dbmysql/bootstrap.go`
- `backend/internal/adapter/dbredis/bootstrap.go` + `bootstrap_test.go`
- `backend/internal/adapter/dbmongo/bootstrap.go`

#### 后端改动
- `backend/internal/store/envmgmt/models.go` / `task_repo.go` / `task_repo_test.go`
- `backend/internal/service/envmgmt/service.go` / `worker.go` / `dto.go`
- `backend/internal/service/dbmgmt/service.go`
- `backend/cmd/dougong/main.go`
- `backend/tasks.md`

#### 前端新增 / 改动
- 新增 `frontend/src/types/__tests__/env.test.ts`
- 改动 `frontend/src/types/enums.ts` / `env.ts`、`frontend/src/views/env-mgmt/index.vue`、`frontend/src/views/env-mgmt/components/EnvTaskDrawer.vue`、`frontend/src/stores/__tests__/env.test.ts`、`frontend/tasks.md`
- `backend/web/dist/*`（build 产物同步）

#### 设计文档
- `00-architecture/d0-clarification-2026-11-17-bootstrap.md`（新）
- `00-architecture/business-arch-env-mgmt.md` / `application-arch-env-mgmt.md` / `function-list-env-mgmt.md`（v1.1）
- `02-detailed/prd/prd-env-mgmt.md`（v1.2）/ `domain-model.md`（v1.22）
- `01-solution/04-business-enum.md`（+bootstrap_status）
- `02-detailed/api/api-env-mgmt.md`（v1.2）/ `api-database-mgmt.md`（v2.2）
- `02-detailed/interaction/{admin,readonly}/env-mgmt/env-mgmt.md`（v1.3）
- `profiles/dougong-panel/doc-center.json`（再生成）
- `05-reports/gate1-env-bootstrap-inc-2026-11-17.md` / `gate2-env-bootstrap-inc-2026-11-17.md` / 本报告
- `workspace/INDEX.md`（状态写回）

## 5. 构建验证（实跑）

| 门控 | 结果 |
|------|------|
| `npx vue-tsc --noEmit` | exit 0 |
| `npx vitest run` | 18 文件 / 195 全过（+7 本轮） |
| `npm run build` | exit 0（25.11s；chunk 体积警告既有 INFO） |
| dist 同步 + `go build ./...` | exit 0（embed 生效） |
| `go vet`（本轮 9 包） | exit 0 |
| `go test -race -count=1`（本轮 9 包） | 9 包全 ok 0 FAIL |
| `GOOS=linux GOARCH=amd64 go build ./...` | exit 0 |
| `gofmt -l`（本轮改动） | 0 文件 |
| **全仓 `go build ./...` + `go test -count=1 ./...`**（ACME 轮经主会话修复后） | **48 包 ok 0 FAIL** |

覆盖率：service/dbmgmt **86.4%**（超 80% 目标；bootstrap.go BootstrapAfterInstall 83.3%/bootstrapEngine 92.9%）；service/envmgmt runBootstrap **100%**；dbredis configGetValue/isLoopbackBind 100%。适配器引导方法（dbmysql/dbmongo BootstrapAdminUser、dbredis BootstrapRequirepass）环境绑定 0% 归 Linux 冒烟（service 层编排全注入覆盖）。

栈规则：G-01~G-07 / F-01~F-05 本轮改动面 0 违例；AL-001~005 契约零差（bootstrap_status 五方同值：池/DDL CHECK/后端 store 常量/前端 BOOTSTRAP_STATUS/双 PageSpec）。CI 第 7 起预防：引导类测试全注入化（fakeMySQLBoot/fakeRedisBoot/fakeMongoBoot/fakeBoot + dbredis 纯函数），零 runner 实况引擎依赖。

## 6. 实机冒烟清单（发布后 Linux 目标环境 104.129.180.140:16097 验证）

1. apt 装 MySQL → 引导经 socket root auth_socket 建 dougong@localhost → 面板数据库管理 TCP 验证可治理（skip-name-resolve 默认 OFF）。
2. apt 装 Redis → CONFIG SET requirepass + REWRITE 持久化 → 重启 redis 密码仍在 → 面板可治理；bind 127.0.0.1 校验。
3. apt 装 MongoDB（mongodb-org 8.0 代配）→ localhost exception 建 admin root 用户 → 面板可治理。
4. 幂等：重复安装/升级 → 已配置跳过（bootstrap_status=skipped）；用户已有账户/requirepass/凭据不覆盖。
5. 引导失败呈现：模拟引擎未就绪 → 任务 succeeded + bootstrap_status=failed + 任务 Drawer 横幅「已安装·引导失败」+ 重试指引。
6. 密码零回显：审计 detail/任务输出/响应/列表均不含密码。

## 7. 待跟进 / 二期登记

- **PHP 多版本并存**（sury.org/ondrej PPA 5.6~8.6 coinstallable，各版本独立 phpX.Y-fpm）——二期另起轮（用户拍板本轮 OUT；下一批候选）。
- **MySQL 版本升级路径**（mysql-apt-config 单版本原地升级，不支持降级/禁跳系列/须备份）——二期另起轮。
- 适配器引导方法实机覆盖率（归 Linux 冒烟，§6）。
- MySQL host 名字解析依赖（skip-name-resolve 开启态 TCP 落空如实呈现；如需免疫可二期评估多 host 或 '%' + bind 强约束）。

## 8. 发布建议

**用户拍板「先不发」**——本轮不执行任何 git 操作，CHANGELOG.md v0.1.8 仅草稿（配置就绪为主 + DBM_005 UX + ACME v2.4 随附）。功能已完成并通过 Gate 1/Gate 2，待实机冒烟（§6）；后续攒批发版时建议 v0.1.8，随附冒烟清单交 Linux 目标环境验证引导链路。发版决策由主会话问用户。