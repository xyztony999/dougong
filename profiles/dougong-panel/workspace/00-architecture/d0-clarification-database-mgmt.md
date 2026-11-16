# D0 需求澄清报告：database-mgmt（数据库管理）

> 引擎 design 阶段 D0 产物（2026-11-12，批量澄清）。作为 D1 架构设计的直接输入。
> 需求来源：idea.md §4.1 M5 + knowledge/business-concepts（db_instance/database/db_user/redis_instance、ADR-007、R-15）。

## 1. 业务概念解析

| 维度 | 内容 |
|------|------|
| 业务术语 | 实例发现（socket/TCP/凭据配置）、授权、root 密码修改、mysqldump 导出导入、进程列表、info 状态、慢日志、key 浏览 |
| 业务流程 | MySQL/MariaDB：发现实例（socket 探测 + 用户配置凭据）→ 库/用户 CRUD → 授权 → 备份导出（mysqldump）/导入；Redis：发现实例 → info 状态/慢日志/密码设置/key 只读浏览；全部变更记审计（R-03） |
| 业务角色 | admin 全操作；readonly 查看实例/库/状态概览/慢日志（ADR-018） |
| 业务规则 | R-15 存量自动发现（发现即接管，不要求重装）；ADR-007 MVP 仅 MySQL/MariaDB + Redis（PostgreSQL 二期）；R-08 mysqldump 等命令一律参数数组 |
| 业务数据 | db_instance（类型/连接方式/版本/状态）、database（名称/字符集）、db_user（用户名/host/授权）、redis_instance（连接方式/info/慢日志） |

## 2. 架构匹配与类型判定

- 匹配结果：四个对象均已在 kb 登记；workspace 无本模块设计文档 → **无匹配，新增**
- 需求类型：**新增** → D1 初始生成
- 模块定位：dbmysql/dbredis 适配器的消费模块；凭据管理涉敏感数据存储，安全设计重点（§7 硬约束）

## 3. 澄清问答记录

| # | 问题 | 结论 | 落点 |
|---|------|------|------|
| — | 本轮无新增开放点 | 范围边界 ADR-007 已拍板（PostgreSQL 二期）；Redis MVP 只读（材料明示「MVP 不做复杂数据编辑」） | — |

## 4. 澄清决策回填清单

- 无新增 ADR
- workspace/INDEX.md：database-mgmt.requirement → partial（D0 完成，D1-D4 工作流进行中）

## 5. D1 输入摘要（结构化需求）

**模块边界**：
- IN（MySQL/MariaDB）：实例发现与凭据配置管理、数据库 CRUD（建/删/字符集）、用户 CRUD 与授权管理、root 密码修改、备份导出（mysqldump）/导入、进程列表、状态概览（version/variables 摘要）
- IN（Redis）：实例发现、info 状态概览、慢日志查看、密码设置、key 浏览（只读 + TTL 查看）
- OUT：PostgreSQL（ADR-007 二期）、SQL 查询工作台/在线执行 SQL（材料未列，不做——安全面大）、Redis 数据编辑/发布订阅（材料明确 MVP 只读）、phpMyAdmin 类内置工具（安全先例不做，§9.3）

**核心实体**：db_instance、database、db_user、redis_instance；凭据（db_instance 内嵌加密字段，D1 定方案）

**状态机**：实例连接状态 reachable/unreachable（探测得出，非流转语义）

**待 D1 细化（次要，不阻断）**：
- ⚠️ 凭据存储加密方案（SQLite 内加密字段；密钥来源与首次初始化绑定关系——D1/安全设计定）
- ⚠️ 实例发现探测范围（建议：默认 socket 常见路径 /tmp/mysql.sock、/var/run/mysqld/ + localhost:3306/6379 探测；不做全网段扫描）
- 多实例同机场景的命名与展示（host:port 标识）
