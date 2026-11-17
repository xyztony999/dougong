# Gate 1 增量复检报告 — 安装后配置就绪（env-mgmt 主导 + database-mgmt OHS 联动）

| 属性 | 值 |
|------|-----|
| 模块 | env-mgmt（主）+ database-mgmt（OHS 提供方） |
| 检查时间 | 2026-11-17 |
| 轮次 | /pipeline 增量轮（安装后配置就绪，d0-clarification-2026-11-17-bootstrap.md） |
| 基线 | api-env-mgmt.md v1.2 / api-database-mgmt.md v2.2 / prd-env-mgmt.md v1.2 / domain-model.md v1.22 / BA/AA/FL env-mgmt v1.1 / PageSpec admin·readonly v1.3 / 04-business-enum.md（+bootstrap_status） |
| 执行轨 | session（本会话无 CreateWorkflow/Agent 工具） |
| 判定 | **WARN**（CRITICAL 0 / 契约断裂 0；五项硬检查全过；W1~W5 建议项呈请用户确认） |

## 1. 增量五项硬检查（全过）

### 1.1 编号接力（全序唯一，无冲突）

| 序列 | 本轮新增 | 接力前序 | 实证 |
|------|---------|---------|------|
| 设计决策 env-mgmt | D-15~D-19 | D-14（v1.1） | grep `\| D-1[5-9] \|` 5/5 命中 |
| 设计决策 db-mgmt | D-20 | D-19（v2.0） | grep `\| D-20 \|` 命中 |
| PRD 功能需求 | FR-07 | FR-06 | grep `\| FR-07 \|` 命中 |
| BA 假设 | ⚠️A11~A15（+补录 A8~A10） | ⚠️A7（v1.0）/A10（prd v1.1） | grep `⚠️A1[1-5]` 5/5 命中 |
| 不变量 | I-09 | I-08 | grep `\| I-09 \|` 命中 |
| AA 功能子模块 | C-14-04（机制型） | C-14-03 | grep C-14-04 命中 |
| BA L4 | 2.2.1.4 安装后配置就绪 | 2.2.1.3 | 业务架构树同步 |
| 迁移 | 012_env_bootstrap.sql | 011_database_mongo.sql | 011 实存（db-mgmt MongoDB），012 ADD COLUMN |
| 枚举 | bootstrap_status | env_task_status 等 | 池新增行 |
| 审计动作码 | env_bootstrap（env）/ instance_bootstrapped（db） | env_repo_configured / mongo_user_* | 常量节登记 |

### 1.2 枚举同步（S8 四方同值）

bootstrap_status{succeeded,failed,skipped}（NULL=无引导步骤非枚举值）：

| 方 | 取值 | 实证 |
|----|------|------|
| 枚举池 04-business-enum.md | succeeded \| failed \| skipped | grep 命中 |
| DDL（api-env-mgmt 5.3） | CHECK (bootstrap_status IN ('succeeded','failed','skipped')) | grep 命中 |
| admin PageSpec 第 3 节 | succeeded / failed / skipped | grep 命中 |
| readonly PageSpec 第 3 节 | 引用 admin 6 Key（含 bootstrap_status） | grep 命中 |

env_type 七值（v1.1 已扩，本轮 domain-model 环境条目行滞后补正七值）；db_engine/grant_level/mongo_role 等既有枚举零改动。

### 1.3 DDL-文档一致

env_install_task +bootstrap_status TEXT NULL CHECK(三值) + bootstrap_note TEXT NULL：字段清单（4.1.1）↔ DDL（5.3）↔ 迁移 012（ADD COLUMN，SQLite 支持带 CHECK 可空列追加无需表重建）↔ SQL #7（Finish 扩展含两列）逐处一致。db_instance 零 DDL 变更（复用既有 cred_cipher 密文列）。

### 1.4 Action-API 对齐（S7 机械 diff）

| 视角 | Action 数 | 实证 |
|------|----------|------|
| admin PageSpec | 10（GET 3：env/list·task/list·task/detail；POST 7：env/install·env/uninstall·service/{start,stop,restart,enable,disable}） | grep 路径清单 10/10 |
| api-env-mgmt 接口 | 10（同上） | 4.1.4 接口表 |
| readonly | 3 只读 + 7 负空间 = 10 全集 | 第 2 节 |

**本轮无新 HTTP 接口**——引导为 worker 后置机制 + db-mgmt 进程内 OHS（BootstrapAfterInstall，非 HTTP）；task/list·task/detail 仅响应字段扩展（+bootstrapStatus/bootstrapNote，interface_changed）。S7 双向零差，readonly 负空间恰为 7 写接口。

### 1.5 变更记录齐备

9 份产物 changelog 均有本轮行：BA/AA/FL v1.1、prd-env-mgmt v1.2（+补录 v1.1 W1 行）、domain-model v1.22、api-env-mgmt v1.2、api-database-mgmt v2.2、PageSpec admin/readonly v1.3。枚举池 bootstrap_status 行含来源依据。占位符扫描：新增内容无 TODO/待定/{占位}/TBD（D-01「待定项」为历史已落定措辞，非残留）。

## 2. PRD 就绪检查（增量复核）

| 层级 | 通过/总数 | 状态 |
|------|----------|------|
| 第一层 G1-G5 | 5/5 | PASS（G1 +产品定调与引导边界；G2 IN 7/OUT 12；G4 domain-model v1.22） |
| 第二层 T 适用 | 6/6 | PASS（T1/T1a/T2/T4/T9；T3/T5/T6 N/A 有据；T2 +bootstrap_status 辨析非新状态机；T1a +db-mgmt OHS 消费） |
| 第三层 N1-N4 | 4/4 | PASS（N1 引导全注入化；N2 +5 引导异常分支=16；N3 引导结果同事务；N4 台账即历史） |

## 3. API 文档完整性（增量面，按本项目 SQLite/go-vue3 裁剪先例）

通用 CH4 标准字段项（VARCHAR32 主键/审计八组/MQ/乐观锁等）对本项目 SQLite INTEGER PK + 进程内单体 + 无 MQ 架构 N/A（全仓既有裁剪先例，account-auth 基座）。增量面检查：

| 检查 | 结果 |
|------|------|
| CH1 设计决策含决策点/选择/理由（D-15~D-20） | PASS |
| CH3 权限矩阵/错误码（无新 HTTP 接口，ENV_001~006 不变；引导失败为异步结果非 HTTP 错误码） | PASS |
| CH4 字段表/不变量/领域服务/状态辨析（+bootstrap 字段/I-09/BootstrapTrigger/结果维度辨析） | PASS |
| CH4 仓储契约（Finish 扩展）+ OHS 消费契约（BootstrapGateway） | PASS |
| CH5 SQL 清单（#7 扩展）/DDL（+两列+迁移 012）/对外服务（+OHS 消费/提供） | PASS |
| GL-009 路径三段式（10/10，无新路径） | PASS |
| GL-021/026 枚举来源池（bootstrap_status 入池） | PASS |
| db-mgmt §4.6 OHS 契约（签名/编排/窄接口/约束）完整 | PASS |

## 4. 前后端契约一致性（设计侧）

| 编号 | 检查 | 状态 |
|------|------|------|
| CT-001 | Action 路径 ↔ api（10/10） | PASS |
| CT-002 | task/list·detail 响应 +bootstrapStatus/bootstrapNote ↔ PageSpec 引导列/横幅字段 | PASS |
| CT-003 | bootstrap_status 枚举池↔DDL↔双 PageSpec 四方同值（前端 enums 待 F 阶段，Gate 2 复验五方） | PASS |
| CT-007 | 字典 Key 6 项 ↔ 池定稿 | PASS |

## 5. WARN 建议项（呈请用户确认，不阻断）

| # | 项 | 说明 | 处置建议 |
|---|----|------|---------|
| W1 | MySQL host 名字解析依赖 | dougong@localhost 依赖 MySQL 默认 skip-name-resolve=OFF（fresh install 态）；用户日后开启 skip-name-resolve 则 TCP 127.0.0.1 不匹配 localhost → 引导验证失败如实呈现（D-16 已登记风险）。Q1 拍板已接受此权衡 | 接受（文档已登记；实机冒烟验证默认态可治理） |
| W2 | Redis bind 非运行时可改 | bind 经 CONFIG GET 校验本机；fresh install 默认 127.0.0.1 ::1 无需写；若极端非 loopback（fresh install 不可能）bind 不可 CONFIG SET，仅告警不强制（requirepass 为主防护） | 接受（MVP 边界，文档登记） |
| W3 | 引擎实联归 Linux 冒烟 | Windows 开发机无 MySQL/Redis/MongoDB 引擎与 systemd；引导类测试全注入化（零 runner 实况依赖，CI 第 7 起预防）；socket root auth_socket/localhost exception/CONFIG REWRITE 实联归 Linux 目标环境冒烟 | 接受（沿全仓环境绑定先例；发布后实机验证） |
| W4 | 工作区本轮外未提交改动 | 含 DBM_005 UX 小修（frontend/types/database.ts 等）+ ACME v2.4 后端改动（acme_test.go:488 旧断言 FAIL 归属 ACME 轮）；本轮专注自身文件不触碰，git 操作主会话统一处理 | 知悉（Gate 2 全仓 test 时该 FAIL 归属 ACME 轮，非本轮） |
| W5 | BA/AA/FL 历史滞后已补正 | v1.1 W1 轮 BA/AA/FL 未随 api/prd 升级（仍 v1.0 六环境），本轮 D1 同步至 v1.1（补录 A8~A10 + 七环境 + 引导 L4） | 已闭环（本轮补正） |

## 6. 总结

**判定：WARN**（CRITICAL 0 / 契约断裂 0）

- 增量五项硬检查全过（编号接力/枚举四方同值/DDL-文档一致/S7 10/10 双向零差/changelog 9 份齐）
- PRD 完备性 G 5/5 · T 适用 6/6 · N 4/4
- 无新 HTTP 接口（引导=worker 机制+OHS），S7 Action 数不变；db_instance 零 DDL 变更
- W1~W5 建议项呈请用户经主会话确认；确认后进入 B/F 代码阶段

**待用户确认**：W1~W5 全过 → 进入 B1-B7 / F1-F4。

## 7. 用户确认记录

2026-11-17 用户经主会话转达确认：**全部确认**——D0 设计默认（dougong@localhost 单账户 / ALL PRIVILEGES *.* WITH GRANT OPTION / bootstrap_status 独立维度 / Redis CONFIG SET+REWRITE 仅未设置时写）+ W1~W5 全部接受。Gate 1 由 WARN 收敛为 **PASS（用户确认）**，进入 B/F 代码阶段。