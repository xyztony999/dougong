# Gate 1 验证报告 — 设计→代码门控（v0.1.4 用户反馈混合轮）

**模块**：env-mgmt（F1 缺陷修复）+ database-mgmt（F2 缺陷修复 / F3 MongoDB 增量）
**检查时间**：2026-11-17
**API 文档版本**：api-database-mgmt.md v2.0（含 v1.2 rule_added 修复）/ api-env-mgmt.md v1.1（未变更）
**执行方式**：session 轨（validation skill Gate 1 清单，go-vue3 栈适配口径——沿本仓 Gate 1 先例，Java/Escm 专属条目按栈适配豁免）

## 0. 本轮范围与拍板记录

| 项 | 内容 | 拍板 |
|----|------|------|
| F1 | env-mgmt 任务终态后环境列表不重拉（体验缺陷，D-lite 直修） | 无需拍板（bug 类） |
| F2 | dbmgmt 凭据保存/验证返回裸 INTERNAL（阻断级，D-lite 直修） | 无需拍板（bug 类）；根因实证见 §3 |
| F3 | database-mgmt 支持 MongoDB（功能增量，D1-D4 正规增量轮） | 用户拍板（2026-11-17 经主会话）：Q1=A 对齐 MySQL/Redis 面（发现+凭据+库列表+用户管理三档角色+状态概览；集合/文档级浏览 OUT）；Q2=A mongo-driver Go 库通道 |

## 1. PRD 就绪检查（F3 增量）

| 层级 | 通过/总数 | 状态 |
|------|----------|------|
| 第一层（G1-G5） | 5/5（prd-database-mgmt.md v1.1 4.8 增量保持基线完备） | PASS |
| 第二层（T1-T9） | 适用 4/4（T1 字段=库清单三字段/用户角色字段；T1a 被引用=cron-backup 备份源二期 OUT 已声明；T4 筛选=引擎筛选含 mongodb；T8 幂等=grant 幂等/删除非幂等如实报错沿基线） | PASS |
| 第三层（N1-N4） | 4/4（N2 异常=未开鉴权 localhost exception 语境/重名/角色不存在；N3=三档角色+标识符白名单；N4=审计 4 动作码） | INFO |

## 2. API 文档完整性（api-database-mgmt.md v2.0）

| 章节 | 通过/总数 | 状态 |
|------|----------|------|
| 第1章 模块设计概述 | D-15~D-19 决策含决策点/理由；领域对象总览 engine 扩值口径入 §4.5 | PASS |
| 第2章 跨聚合根设计 | N/A（无新聚合根——db_instance 扩值） | N/A |
| 第3章 应用/基础设施层 | §3.1 资源清单 +mongo/mdb/muser；§3.2 +dbmongo 依赖行（mongo-driver v2 登记）；§3.3 矩阵 30 接口；§3.4 复用 DBM_001~021 | PASS |
| 第4章 聚合根详细设计 | §4.5 MongoDB 治理三小节（网关契约/7 接口详情/集成） | PASS |
| 第5章 模块清单汇总 | §5.3 迁移 011（engine CHECK 扩值表重建，010 先例）；§5.2 SQL 零新增（复用 1~6） | PASS |
| 全局一致性 | GL-009 三段式全过（复查见 §4 F-1）；GL-021/026 mongo_role+db_engine 池内定义；GL-001a 头日期=changelog 最新 2026-11-17 | PASS |

## 3. F2 根因实证与修复设计复核（阻断级缺陷）

- **根因（读码实证，非臆测）**：`translateGatewayErr`（service/dbmgmt/service.go）对 `dbmysql.ErrUnavailable`/`ErrQueryFailed`、`dbredis.ErrCommandFailed` 三哨兵无映射 → default 原样透传 → `response.Fail` 对非 errcode 错误兜底 `INTERNAL/内部错误/data:null`（与用户实拍一致）。用户「@ 破坏 DSN」假设证伪：全仓无 go-sql-driver/DSN 拼接（唯一 sql.Open=面板自身 SQLite），密码经 `MYSQL_PWD`/`REDISCLI_AUTH` env 注入 CLI 子进程。
- **修复设计（api v1.2 rule_added）**：DBM_021（通道不可用）新错误码；translatePingErr（凭据验证语境未分类失败→DBM_006，契约 D-03「连接类失败一律业务码」落实）；tcp 模式 `--protocol=TCP`（-h localhost socket 特例改道次级缺陷）。
- **判定**：PASS——错误表/接口行/changelog/前端内联提示四方一致。

## 4. 前后端契约一致性

| 编号 | 检查项 | 状态 | 说明 |
|------|--------|------|------|
| CT-001 | API 路径与 PageSpec Action 对齐 | PASS | 30/30（mongo/status、mdb/list、muser/{list,create,delete,grant,revoke}）；**F-1 自查修正**：初稿 /db/mongo/database/list 等四段式违反 G1-08/GL-009（v1.1 F-1 同类），检查中即改三段式（资源 mongo/mdb/muser），双侧同步 |
| CT-002 | 请求/响应字段与页面字段对应 | PASS | 5.9 建用户（username/password/db/role ↔ muser/create req）；6.3 三 Tab 列定义 ↔ status/mdb/list/muser/list 响应字段（sizeBytes/collections/mechanisms/roles） |
| CT-003 | 枚举值前后端一致 | PASS | db_engine 四值/mongo_role 三值——池↔api↔PageSpec 三方同值（S8 机械校验过）；前端 enums.ts 实装归 F 阶段（B/F 对齐项） |
| CT-004 | Action 表无虚构接口 | PASS | 30 Action 全部有 api 端点（grep 机械校验） |
| S7 | 接口↔Action 一一对应 | PASS | api 30 唯一端点 = admin 30 Action；readonly 可见 10+负空间 20=30 |
| S8 | 字典 Key↔枚举池 | PASS | 5/5（db_engine/db_conn_mode/instance_conn_state/grant_level/mongo_role） |

## 5. 历史契约修订核查（v1.2 修复轮）

- DBM_021 入 §3.4 错误表（21 码）+ credentials 接口行 + 前端 InstanceCredentialsModal 内联——四方一致 ✓
- PageSpec 第 1 节错误分流行 +DBM_021 ✓；第 8 节检查 6 计数 001~021 ✓
- env-mgmt PageSpec v1.2（admin+readonly）：终态跃迁重拉语义入 4.3 刷新行 + changelog ✓

## 6. WARN 项（供用户确认）

| # | 项 | 说明 | 建议 |
|---|----|------|------|
| W-1 | F3 探测集 socket 路径单值 | mongodb 默认 socket 仅探 /tmp/mongodb-27017.sock（官方默认 unixSocketPath；自定义路径不追——探测不到=未按默认装，如实口径沿 D-07/⚠️A19 哲学） | 接受（与 mysql 四路径惯例的差异=mongod 默认值唯一且官方文档固定） |
| W-2 | F3 无建/删库接口 | MongoDB 隐式建库语义，与 MySQL 面差异如实裁剪（D-18）；用户如建库→写数据自动产生，或经用户授权建 | 接受（Q1=A 拍板面已含） |
| W-3 | F3 新直接依赖 mongo-driver v2 | go.mod 新增（纯 Go；go.sum 锁定；goproxy.cn 可达已验证）。R-08/R-09 纪律：进程内库无外呼、无 shell 面 | 接受（Q2=A 拍板即含） |
| W-4 | F3 状态概览白名单具体条目 | D-08 同构白名单 ≤10 项（version/uptime/connections×2/mem/opcounters×4）——条目口径实现期如遇 driver 字段形态差异可等价替换 | 授权实现期微调（不动接口形态） |

## 7. 总结

**判定：WARN（4 项建议项待用户确认；CRITICAL 0——四段式违规已当场修正）**

- 通过：PRD 5/5+4/4、API 六章节、CT/S7/S8 机械校验全过
- CRITICAL 违规：0（F-1 检查中自查修正：mongo 路径三段式化）
- WARN：4（W-1~W-4，均为拍板面已含或实现期授权项）

**用户确认 W-1~W-4 后进入 B/F 阶段**（F1/F2 代码已先行完成并全绿——bug 类 D-lite 直修先例；F3 代码待本轮确认后实施）。
