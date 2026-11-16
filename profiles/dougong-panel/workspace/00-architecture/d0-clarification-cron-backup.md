# D0 需求澄清报告：cron-backup（计划任务与备份）

> 引擎 design 阶段 D0 产物（2026-11-12，批量澄清）。作为 D1 架构设计的直接输入。
> 需求来源：idea.md §4.1 M8 + knowledge/business-concepts（cron_job/backup_job/backup_run、R-11、ADR-013/016）。

## 1. 业务概念解析

| 维度 | 内容 |
|------|------|
| 业务术语 | cron 表达式、手动触发、执行日志尾部、备份任务、备份源（站点目录+数据库）、目标（本地/rsync 远程）、保留份数轮转、backup_run |
| 业务流程 | cron 管理：接管系统 crontab（读写）→ CRUD + 表达式校验 → 执行日志尾部（系统 cron 日志）→ 手动触发（面板代执行，记审计）；备份：backup_job（站点目录 + 数据库 → 本地目录 / rsync 远程）→ 调度执行 → backup_run 记录（状态/耗时/产物/大小）→ 按保留份数轮转清理（R-11） |
| 业务角色 | admin 全操作；readonly 查看 cron 列表/备份任务/执行历史（ADR-018） |
| 业务规则 | R-11 保留份数轮转；ADR-016 backup_run 独立执行记录实体（已确认）；R-08 手动触发/备份命令一律参数数组；R-03 全部变更与手动触发记审计 |
| 业务数据 | cron_job（表达式/命令/备注/来源）、backup_job（源清单/目标/保留份数/调度）、backup_run（开始/结束/状态 running→success·failure/耗时/产物路径与大小/触发方式 cron·manual） |

## 2. 架构匹配与类型判定

- 匹配结果：cron_job/backup_job 已在 kb 登记；backup_run 已由 ADR-016（上轮 D0 Q2）确认新增；workspace 无本模块设计文档 → **无匹配，新增**
- 需求类型：**新增** → D1 初始生成
- 模块定位：cronx/backupx 适配器消费模块；「接管存量」同样适用（系统已有 cron 任务自动纳入管理）

## 3. 澄清问答记录

| # | 问题 | 结论 | 落点 |
|---|------|------|------|
| — | 本轮无新增开放点 | backup_run 实体与状态机 ADR-016 已确认（2026-11-12 上轮 Q2）；备份目标 MVP 范围（本地 + 可选 rsync）材料已明示 | — |

## 4. 澄清决策回填清单

- 无新增 ADR
- workspace/INDEX.md：cron-backup.requirement → partial（D0 完成，D1-D4 工作流进行中）

## 5. D1 输入摘要（结构化需求）

**模块边界**：
- IN（cron 管理）：系统 crontab 接管（列表/新建/编辑/删除）、cron 表达式校验（解析+下次执行时间预览）、执行日志尾部、手动触发（面板代执行 + 超时控制 + 审计）
- IN（备份）：backup_job CRUD（备份源：站点目录〔联动 site-mgmt 站点清单〕+ 数据库〔联动 database-mgmt 实例/库清单〕；目标：本地目录 / rsync 远程；保留份数）、立即执行、备份历史（backup_run 列表/详情/产物下载）、轮转清理（R-11）
- OUT：备份自动还原流程（MVP 不做，产物手动下载恢复——口径 D1 确认）、云存储目标 S3/OSS（二期）、增量/去重备份（二期）、跨服务器 rsync 管理界面（MVP 仅目标地址配置）

**核心实体**：cron_job（系统 crontab 投影 + 面板标注）、backup_job（落 SQLite）、backup_run（落 SQLite，ADR-016 状态机）

**状态机**（已确认）：backup_run：running → success / failure；记录按保留份数轮转清理

**待 D1 细化（次要，不阻断）**：
- ⚠️ cron 接管边界：root crontab 读写为主；`/etc/cron.d/`、系统级 crontab 建议只读展示（写操作仅限 root crontab）；执行日志来源（journalctl -u cron / /var/log/cron 按发行版适配器取）
- ⚠️ backup_job 调度器归属（建议：面板内置调度器独立于系统 cron，避免写 crontab 的双写一致性问题；cron 管理与备份调度解耦）
- 备份产物命名与清单格式（建议：{job名}/{时间戳}/ 目录 + manifest.json）
- 数据库备份凭据使用链路（复用 database-mgmt 已存凭据，D1 联动）
