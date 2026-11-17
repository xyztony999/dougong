# D0 需求澄清报告：v0.1.4 用户实测反馈混合轮（env-mgmt / database-mgmt）

> 引擎 design 阶段 D0 产物（2026-11-17，增量澄清）。v0.1.4 发布后用户服务器
> （104.129.180.140:16097，Ubuntu noble，apt 通道）实测反馈 3 项：2 项缺陷修复
> （D-lite 直修）+ 1 项功能增量（MongoDB 管理，走 D1-D4 增量设计轮）。

## 1. 反馈项结构化

| # | 类型 | 模块 | 内容 | 处置 |
|---|------|------|------|------|
| F1 | 缺陷（体验） | env-mgmt（前端） | 任务到终态后环境列表不重拉——用户看到旧 installed/版本/服务态；发起 install/upgrade/uninstall 后应刷新（在途标记） | D-lite 直修（bug 类）：envStore 任务详情「在途→终态」跃迁触发 loadList+loadTasks；发起后刷新（refreshAfterWrite）复核确认已存在 |
| F2 | 缺陷（阻断级） | database-mgmt（后端） | 凭据配置弹窗保存/验证返回裸 INTERNAL（实拍：POST /db/instance/credentials，instanceId=3，tcp/localhost/3306，密码含 @ 与数字） | D-lite 直修（bugfix）：根因=网关哨兵映射缺口（ErrUnavailable/ErrQueryFailed/ErrCommandFailed 未入 translateGatewayErr 表 → response 兜底 INTERNAL），非密码字符问题（密码经 MYSQL_PWD/REDISCLI_AUTH env 注入 CLI 子进程，无 DSN 拼接面——「@ 破坏 DSN」假设证伪）；修复=DBM_021 新增+translatePingErr+--protocol=TCP |
| F3 | 功能增量 | database-mgmt | 「数据库管理页面居然不能管理 mongodb？？」——env-mgmt 已可装 MongoDB（v0.1.3 起，mongodb-org apt 源），db-mgmt 仅发现/管理 MySQL/MariaDB/Redis | 正规 D 增量轮：D1-D4 → Gate 1 → B/F → Gate 2 |

## 2. F2 根因实证记录（读码验证，禁止臆测纪律）

- 通道事实：dbmysql/dbredis 适配器全部经 exec.CommandContext 字面量参数数组外呼
  mysql/redis-cli 客户端；密码经 MYSQL_PWD/REDISCLI_AUTH 环境变量注入（不进 argv，
  G-03）；全仓无 go-sql-driver/DSN 拼接（唯一 sql.Open=面板自身 SQLite）。
- 缺陷链：service/dbmgmt/service.go translateGatewayErr switch 无
  ErrUnavailable/ErrQueryFailed/ErrCommandFailed 三哨兵 case → default 原样透传 →
  api/response.Fail 对非 errcode 错误兜底 INTERNAL/内部错误/data:null（与实拍一致）。
- 触发场景（最可能）：宿主机无 mysql 客户端（如 MySQL 跑 Docker 容器仅发布 3306——
  面板 docker-mgmt 支持；实例3=tcp localhost:3306 由端口探测纳管）；次要：CLI 未分类
  stderr（超时 signal: killed 等）→ ErrQueryFailed 同链路裸 INTERNAL。
- 次级缺陷（同轮修复）：tcp 模式 -h localhost 时 mysql 客户端特例走编译默认 Unix
  socket 忽略 -P（tcp 语义被静默改道）→ connArgs 补 --protocol=TCP。

## 3. F3 增量澄清

**业务概念**：MongoDB 实例发现（mongod systemd 单元/端口 27017/默认 socket
/tmp/mongodb-27017.sock，R-15 探测哲学）、凭据连接（admin 库验证，SCRAM）、库列表
（名称/大小/集合数）、用户管理（db.createUser/dropUser + 角色档位）、状态概览
（serverStatus 白名单摘要，D-08 同构）。

**边界输入**：env-mgmt 已装 MongoDB（mongodb-org，mongod.service）；枚举 db_engine
需 +mongodb（enum_value_added，DDL CHECK 迁移表重建，010 同法）；PageSpec 需
mongo-manage 页/Tab（engineGuard 扩展）；集合/文档级浏览建议二期 OUT。

**拍板项（经主会话 AskUserQuestion 问用户，2026-11-17 提出）**：

| # | 问题 | 选项 | 状态 |
|---|------|------|------|
| Q1 | MongoDB 管理面范围 | A 对齐 MySQL/Redis 裁剪版（发现+凭据+库列表+用户管理+状态概览）/ B 最小面（发现+凭据+库列表只读）/ C=A+集合级只读列表 | ⚠️ 拍板中 |
| Q2 | 技术通道 | A mongo-driver Go 库（新直接依赖，纯 Go，无外部二进制依赖）/ B mongosh CLI 外呼（同构现有通道；mongosh 不随 server 必装/argv-env 密码通道弱/启动 1~2s/EJSON 解析） | ⚠️ 拍板中 |

**技术注记（任一选项均成立）**：mongo-driver v2 经 goproxy.cn 可拉取（v2.10.0，
2026-11-17 验证）；mongosh 不在 mongodb-org-server 包内（仅 metapackage 依赖链且
6.0+ 已移除 legacy shell）——CLI 方案需宿主机额外装 mongosh 且密码经连接串（URI
需 URL 编码——@ 问题真实存在于此通道）或 argv（ps 可见，违 G-03）。

## 4. 澄清决策回填清单

- F1/F2 无新增 ADR（缺陷修复，规则补齐到 api-database-mgmt.md v1.2 rule_added）。
- F3 待 Q1/Q2 拍板后：枚举池 db_engine +mongodb（enum_value_added）、api 文档
  interface_added + 迁移 011、PageSpec 页面/Tab 增量、INDEX note 追加。

## 5. D1 输入摘要（F3，待拍板项落定后生效）

- IN（按 Q1=A 口径，B/C 裁剪）：MongoDB 实例发现与凭据配置（合一档案表 engine
  扩值）、库列表只读投影、用户管理（三档角色映射）、状态概览白名单。
- OUT：集合/文档级浏览与编辑（二期）、mongod 配置文件治理（conf 文件管理归
  file-manager 通用能力）、MongoDB 备份导出导入（mongodump，二期——mysqldump 先例
  可后继）、分片/副本集拓扑治理。
- 复用：db_instance 合一档案表（D-01 engine 列扩值）、凭据 vault（D-02 同构）、
  I-05 前置链、ADR-018 角色边界、R-03 审计。
