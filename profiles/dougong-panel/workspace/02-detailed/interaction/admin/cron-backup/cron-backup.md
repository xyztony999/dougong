# PageSpec · 计划任务与备份（cron-backup） · 管理员（admin）

| 属性 | 值 |
|------|-----|
| 版本 | v1.0 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-14 |
| 用户类型 | admin（readonly 视角为本文档子集，差异见 ../readonly/cron-backup/cron-backup.md） |
| 唯一数据源 | 02-detailed/api/api-cron-backup.md v1.0 |
| 组件体系 | Vue 3 + Naive UI（NTabs/NDataTable/NSelect/NInput/NButton/NTag/NPopconfirm/NModal/NAlert/NEmpty/NDrawer/NForm/NFormItem/NInputNumber/NCollapse；无激活栈 profile，不使用 vue3-engine 栈 Scm\* 组件） |
| 确认方式 | headless run：7 项交互确认依据 D3 契约（api-cron-backup.md D-01~D-14 + I-01~I-10）+ ADR-018 + FL v1.0 推导，标注于各节 |

## 第 1 节 模块元信息

| 项 | 值 | 确认依据 |
|----|-----|---------|
| 模块编码 | cron-backup（域前缀 /cron） | api 3.1 |
| 业务类型 | **混合型（诚实标注）**：备份=业务单据（backup_run 状态机 running→success/failure，ADR-016）+ 备份任务=主数据（配置单据无状态机）+ 计划任务=工具类通道（crontab 投影无聚合根）——三线并存一模块 | api 1.4/4.1~4.3；domain-model.md cron-backup 域 |
| 页面类型 | **hybrid ×2**（两页均有 Tab 切换 → 分册 2.2 触发 hybrid，layout: 'tab'；无左树右表/拖拽/实时试算，不触发 code） | 分册 2.2；FL 2.1（cron 线）与 2.1（备份线）两菜单组各自同页承载 |
| UI 框架 | Naive UI（NTabs/NDataTable/NDrawer/NModal/NForm/NInputNumber/NCollapse 等） | 栈版本表 |
| 状态管理 | Pinia：cronStore（jobItems/systemEntries/logTailText/backupJobItems/runItems/runTotal/listParams/inFlight×9/triggerModal/runDrawer/backupFormDrawer 态）；枚举常量 store（与 account-auth/firewall 等共用 enums.ts） | 栈版本表 |
| 错误处理 | 统一拦截器按 api 3.4 code 分流：AUTH_001/002→跳登录（全局拦截器）、AUTH_003→CSRF 失败提示（全局）、AUTH_008→「权限不足：只读角色禁止计划任务与备份变更操作」（全局边界拒绝，正常 UI 无入口不触发）、CB_001→当前 Dialog/Drawer 内对应字段错误提示（R-14；含本地目录路径非法/host 非法/库名非法）、CB_002→表达式字段错误（data.errors[] 段位级，第 5.1 节校验预览承载）、CB_003→warning「该任务已不存在（可能已在面板外修改）」+ 自动重拉 jobList（api D-01 实况投影）、CB_004→warning「备份任务不存在」+ 重拉 backupJobList、CB_005→Drawer 任务名字段错误「名称已存在」（I-06）、CB_006→warning「该任务已有备份正在执行，请稍后再试」（I-01 手动侧；执行互斥）、CB_007→warning「执行记录不存在」+ 重拉 backupRunList（轮转清理后记录消失的自然结果，api D-09）、CB_008→warning「仅成功的执行记录可下载产物」（I-04）、CB_009→warning「产物文件已缺失（可能在目标位置被删除），执行记录保留为历史事实」 | api 3.4 + D-01/D-09/I-01~I-06 |

## 第 2 节 Action 定义（与 API 一一对应）

| Action | 方法 | 路径 | 页面 | 权限 | CSRF |
|--------|------|------|------|------|------|
| jobList | GET | /cron/job/list | cron·Tab 计划任务（含系统预置位分区） | admin+readonly | —（GET） |
| jobCreate | POST | /cron/job/create | cron·新建/编辑任务 Dialog | admin | ✓ |
| jobUpdate | POST | /cron/job/update | cron·新建/编辑任务 Dialog（编辑态） | admin | ✓ |
| jobDelete | POST | /cron/job/delete | cron·Tab 计划任务·操作列 | admin | ✓ |
| jobValidate | POST | /cron/job/validate | cron·任务 Dialog 表达式字段 + backup·任务 Drawer 调度字段（两处内嵌消费，机制型） | admin | ✓ |
| jobTrigger | POST | /cron/job/trigger | cron·Tab 计划任务·操作列（手动触发） | admin | ✓ |
| logTail | GET | /cron/log/tail | cron·Tab 执行日志 | admin+readonly | —（GET） |
| backupJobList | GET | /cron/backup-job/list | backup·Tab 备份任务 | admin+readonly | —（GET） |
| backupJobCreate | POST | /cron/backup-job/create | backup·新建/编辑任务 Drawer | admin | ✓ |
| backupJobUpdate | POST | /cron/backup-job/update | backup·新建/编辑任务 Drawer（编辑态） | admin | ✓ |
| backupJobDelete | POST | /cron/backup-job/delete | backup·Tab 备份任务·操作列 | admin | ✓ |
| backupJobRun | POST | /cron/backup-job/run | backup·Tab 备份任务·操作列（立即备份，异步返回 runId） | admin | ✓ |
| backupRunList | GET | /cron/backup-run/list | backup·Tab 备份历史 | admin+readonly | —（GET） |
| backupRunDetail | GET | /cron/backup-run/detail | backup·执行详情 Drawer | admin+readonly | —（GET） |
| backupRunDownload | GET | /cron/backup-run/download | backup·执行详情 Drawer·files 行（admin） | admin | —（GET 流式） |

> S7 一致性：15 Action 与 api-cron-backup.md 15 接口一一对应（一接口一 Action，无未覆盖接口、无虚构接口）。CSRF Token 由全局拦截器统一注入 `X-CSRF-Token`（account-auth R-02 机制）；download 为 GET 流式（浏览器原生下载，无 CSRF 面，api 3.1）。

**刷新纪律**（确认项 2/7 交叉；api D-11 读时派生与 D-13 调度语义的前端落法）：

| 触发 | 行为 |
|------|------|
| 进入页面 | 当前 Tab 清单按需拉取（首次切 Tab 亦然，Tab 懒加载）；无 status 先行接口（两页均无引导态判定——cron/备份服务恒在，与 firewall daemon 探测不同构） |
| 点击「刷新」 | 重拉当前 Tab 数据（loading 防抖） |
| jobCreate/jobUpdate/jobDelete 成功 | 重拉 jobList（crontab 实况已变） |
| jobTrigger 完成 | 不重拉 jobList（代执行不改 crontab）；结果经触发 Modal 呈现（第 5.2 节） |
| backupJobCreate/Update/Delete 成功 | 重拉 backupJobList（含 nextFireAt/lastRun 派生刷新） |
| backupJobRun 成功 | message「已开始备份（执行记录 #runId）」+ 自动切换到备份历史 Tab 重拉 backupRunList + 新记录行浅蓝高亮 10s |
| backupRunDelete 类操作 | 无（历史记录仅由轮转清理，无用户删除动作，api FR-15 机制型） |
| **执行详情 Drawer 轮询**（api D-07 留 D4 定的节奏） | Drawer 打开且当前 run status=running → **每 5s 轮询 backupRunDetail**，转终态即停并刷新一次 backupRunList；Drawer 关闭立即停（无残留定时器） |
| 备份历史列表 running 态 | 不自动轮询（列表刷新=手动+操作驱动；进行中结论经 Drawer 轮询承载，克制原则与 firewall「无自动轮询」同口径） |
| `document.hidden` | 页面隐藏暂停 Drawer 轮询定时器，恢复可见且条件仍满足则恢复 |
| 页面卸载 | 清理 Dialog/Drawer/inFlight/轮询定时器 |

## 第 3 节 字典 Key 清单

| Key | 取值（=01-solution/04-business-enum.md 定稿） | 用途 |
|-----|--------------------------------------------|------|
| cron_source | panel / existing | 计划任务来源列 dict + colorTag（任务 Dialog 不入表单——来源由标记注释判定，api D-02） |
| backup_source_type | site_dir / database | 备份任务源构成列 Tag + Drawer 源清单动态行类型选择 |
| backup_target_type | local / rsync | 目标类型列 Tag + Drawer 目标类型下拉（条件显隐目标字段组） |
| backup_trigger | cron / manual | 历史列表触发方式列 dict（筛选下拉同源） |
| backup_run_status | running / success / failure | 历史/详情状态列 dict + colorTag（筛选下拉同源）；详情 Drawer 轮询判定依据 |
| panel_role | admin / readonly | 菜单可见性判定（authStore 复用） |

**数据源规范偏差声明**（同 account-auth/firewall/docker-mgmt 先例）：不实现 `POST /{domain}/dict/batch`（分册 3.6）——枚举全部固定值，字典由前端 TS 常量提供（enums.ts 单一事实源，共用同一文件）。此偏差为设计决策，待 Gate 1 复核。

**状态颜色方案**（分册状态颜色语义映射）：

| 值 | 颜色 | 语义依据 |
|----|------|---------|
| run status=running | blue | 进行中（分册「进行中/审批中」蓝；附 loading 图标） |
| run status=success | green | 已完成/成功 |
| run status=failure | red | 已驳回/拒绝语义映射为执行失败 |
| cron_source=panel | blue | 面板创建（活跃管理态） |
| cron_source=existing | gray | 系统既有（接管存量，中性） |
| backup_target_type=local | green | 本地目标（就近稳定） |
| backup_target_type=rsync | blue | 远程目标（传输态语义，中性蓝） |
| trigger=cron | gray | 定时触发（系统常态） |
| trigger=manual | blue | 手动触发（即时动作态） |

## 第 4 节 列表页

### 4.1 计划任务页（/cron，hybrid · Tab：计划任务 / 执行日志）

**权限**：admin 全功能；readonly 为子集（无新建/编辑/删除/触发入口，见 readonly 文档）。

#### 4.1.1 Tab 1 工具条（确认项 2）

| 控件 | 组件 | 说明 |
|------|------|------|
| 新建任务 | NButton（primary） | 打开新建任务 Dialog（第 5.1 节） |
| 刷新 | NButton（loading 防抖） | 重拉 jobList |

无筛选参数（api job/list 无 query，crontab 行序如实呈现，api D-02）；表格上方灰字「共 N 条任务 · 面板外修改自动反映」。

#### 4.1.2 Tab 1 表格列（root crontab 任务，NDataTable，确认项 3）

| 列 | 类型 | 宽度 | 说明 |
|----|------|------|------|
| 序号 | index | 60 | — |
| 表达式 | text（等宽） | 150 | items[].expr 原值 |
| 命令 | text（等宽，ellipsis+tooltip） | 320 | items[].command 原值（完整内容悬停可见） |
| 备注 | text | 160 | items[].remark；null → 「—」灰字（无备注注释行，api D-02） |
| 来源 | dict(cron_source) + colorTag | 100 | 第 3 节颜色方案 |
| 操作 | action | 200 fixed:right | 见 4.1.3（readonly 整列不渲染） |

无分页（全量返回）、无排序控件（文件行序即序，api D-02）；空结果 → NEmpty「root crontab 无任务」；加载中 → 骨架。

#### 4.1.3 Tab 1 操作列（operList，确认项 4）

| 按钮 | 可见性条件 | 二次确认 | 依据 |
|------|-----------|---------|------|
| 编辑 | 恒显（全部任务行，含 existing——面板可编辑系统既有任务，保留来源） | —（Dialog 内校验预览即防误） | FL S-51-02；api D-02 |
| 手动触发 | 恒显 | NPopconfirm「由面板立即代为执行一次？最长等待 120 秒。」→ 触发 Modal（5.2） | FL S-51-06；api D-05 |
| 删除 | 恒显 | NPopconfirm「确认从 root crontab 删除该任务？同步移除面板标记注释。」 | FL S-51-03 |

**在途互斥**（确认项 5 交叉）：行级 inFlight——手动触发期间该行三按钮禁用+触发按钮 loading（代执行最长 120s 同步等待，api D-05）；编辑/删除提交期间按钮 loading 防重复。

#### 4.1.4 Tab 1 系统预置位分区（只读，api 4.3.1 systemEntries）

表格下方 NCollapse「系统预置任务（只读）」默认折叠，展开后两组只读呈现（**不提供任何写操作**，D1 口径-1）：

| 组 | 呈现 |
|----|------|
| /etc/cron.d/ 文件 | 每文件一子块：文件名（等宽标题）+ 条目表（表达式/命令 两列，api D-02 文件粒度） |
| cron.hourly/daily/weekly/monthly 目录 | 每目录一行：目录名 + 脚本文件名清单（等宽灰字串） |

分区标题右侧灰字「系统预置位仅展示，不提供写操作」。

#### 4.1.5 Tab 2 执行日志（logTail）

工具条：行数 NSelect（档位 [50,100,200,500,1000]，默认 200，api D-04）+ 刷新按钮。主体：等宽字体滚动文本区（text/plain 原文呈现，api 4.3.4）；上方灰字「来源：系统 cron 服务日志（按发行版自动适配）」；空结果 → NEmpty「暂无 cron 执行日志」。加载中 → NSpin。

### 4.2 备份页（/cron/backup，hybrid · Tab：备份任务 / 备份历史）

**权限**：admin 全功能；readonly 为子集（无新建/编辑/删除/立即执行入口、详情 Drawer 无下载按钮）。

#### 4.2.1 Tab 1 工具条

| 控件 | 组件 | 说明 |
|------|------|------|
| 新建备份任务 | NButton（primary） | 打开任务 Drawer（第 5.3 节） |
| 刷新 | NButton | 重拉 backupJobList |

#### 4.2.2 Tab 1 表格列（备份任务，NDataTable）

| 列 | 类型 | 宽度 | 说明 |
|----|------|------|------|
| 序号 | index | 60 | — |
| 名称 | text | 160 | items[].name |
| 备份源 | Tag 组 | 220 | sourceSummary 逐项 Tag：site_dir→「目录 {path 截断 tooltip}」（green）/ database→「DB {实例名·库}」（blue）；tooltip 全量 |
| 目标 | Tag + text | 180 | targetType Tag（第 3 节颜色）+ targetBrief 灰字 |
| 保留份数 | text | 90 | retentionCount（「N 份」） |
| 调度 | text（等宽） | 140 | scheduleCron 原值；tooltip「下次执行：{nextFireAt 本地时区}」（读时派生，api D-11） |
| 最近执行 | dict(backup_run_status)+colorTag + 灰字 | 170 | lastRun.status（颜色第 3 节）+「{finishedAt 相对时间}」；null → 「从未执行」灰字 |
| 操作 | action | 220 fixed:right | 见 4.2.3 |

无分页（任务个位数~十余，api D-11）；空结果 → NEmpty「暂无备份任务」+ 引导新建。

#### 4.2.3 Tab 1 操作列（确认项 4）

| 按钮 | 可见性条件 | 二次确认 | 依据 |
|------|-----------|---------|------|
| 立即备份 | 恒显；该任务 lastRun.status=running 时禁用+tooltip「执行中」（前端预警，服务端 I-01/CB_006 权威兜底） | —（异步即时返回 runId，无破坏性） | FL C-13-02；api D-07 |
| 编辑 | 恒显 | —（Drawer 编辑态） | FL S-52-03 |
| 删除 | 恒显 | NPopconfirm「确认删除该备份任务？既有执行历史与产物将保留（按保留份数自然轮转）。」（api FR-09 不级联语义的用户可知会项） | FL S-52-03 |

在途互斥：立即备份提交期间按钮 loading；CB_006 → warning（第 1 节）。

#### 4.2.4 Tab 2 备份历史（backupRunList）

**筛选条件**（确认项 2；api list query 一一对应）：

| 控件 | 组件 | 查询方式 |
|------|------|---------|
| 任务 | NSelect（clearable，options=backupJobList items 动态装配 {label:name, value:id}；含已删任务不可得——已删任务仅以其 id 数值呈现，label=「#{id}」灰字，api D-12 弱引用如实） | query `jobId` eq |
| 状态 | NSelect（clearable，backup_run_status 三值） | query `status` eq |
| 触发方式 | NSelect（clearable，backup_trigger 两值） | query `trigger` eq |
| 时间范围 | NDatePicker range | query `from`/`to` between |
| 刷新 | NButton | 以当前参数重拉 |

分页：公共分页控件（page/pageSize，响应 total，api D-11）。

**表格列**（确认项 3）：

| 列 | 类型 | 宽度 | 说明 |
|----|------|------|------|
| 序号 | index | 60 | — |
| 任务 | text | 150 | jobName（快照，api D-12；任务已删时同列灰字小标「任务已删」） |
| 触发方式 | dict(backup_trigger) | 100 | 第 3 节颜色 |
| 状态 | dict(backup_run_status) + colorTag | 110 | running 行附 loading 图标 |
| 开始时间 | text | 170 | startedAt 本地时区 |
| 耗时 | text | 100 | durationMs 派生人性化（1m23s）；running → 「—」 |
| 产物大小 | text（右对齐） | 110 | artifactSize 人性化（MB/GB）；null → 「—」 |
| 操作 | action | 100 fixed:right | 「详情」→ 打开执行详情 Drawer（第 6 节） |

## 第 5 节 表单页（弹窗形态，确认项 5）

**形态依据**（site-mgmt 第 5 节同判）：全部表单为动作接口即时生效（无草稿态无保存/提交两步，api 4.x），按项目先例取 Dialog/Drawer 弹窗形态，不建独立路由表单页——任务 Dialog 3 字段（<5）；备份任务 Drawer 字段虽多但单一动作提交（create/update 全量提交），取宽 Drawer 承载源清单动态行（site-mgmt 4.4 Drawer 编辑态同构）。

### 5.1 新建/编辑任务 Dialog（cron 页，jobCreate/jobUpdate）

NModal（宽 560px，标题「新建任务」/「编辑任务」），字段（api create req 推断）：

| 字段 | 组件 | 取值与校验（前端预校验 + 服务端 CB_001/002/003 兜底） |
|------|------|------------------------------------------------------|
| 表达式 expr | NInput（等宽） | 失焦/防抖 500ms 触发 jobValidate（第 5.2 节预览区承载结果）；五段式与标准宏（api D-03） |
| 命令 command | NInput（textarea 2 行，等宽） | 非空（R-14）；原样写入 crontab |
| 备注 remark | NInput（可空） | 空=不写备注注释行（api D-02） |

编辑态：按 jobKey 预填（expr/command/remark）；来源不展示为可编辑项（panel/existing 由标记注释判定，api D-02）。按钮：**保存 / 取消**；保存 → jobCreate（新建）或 jobUpdate（编辑，req 含原 jobKey）→ 成功 message + 关闭 + 重拉 jobList。

**校验预览区**（jobValidate 内嵌，api D-03/FR-04 机制型落法）：表达式字段下方恒占位区——合法 → 灰字「未来 5 次：{nextFires 本地时区串}」；非法 → 字段 error 态 + 段位级错误文案（CB_002 data.errors[]）。预览为提交前置（未通过校验禁止保存按钮）。

### 5.2 手动触发结果 Modal（jobTrigger，api D-05 同步语义落法）

触发确认后：行级 loading（4.1.3）→ 完成弹出 NModal（宽 640px，标题「手动触发结果 — {expr 截断}」）：

| 元素 | 呈现 |
|------|------|
| 结果 Tag | timedOut=true → red「超时终止」；exitCode=0 → green「成功（退出码 0）」；exitCode≠0 → red「失败（退出码 {n}）」；exitCode=null（未正常退出）→ red「异常终止」 |
| 耗时 | 灰字 durationMs 人性化 |
| 输出 | 等宽滚动文本区（output 尾部 64KB，api D-05 截断口径头部省略时标注「…（输出过长，已截断保留尾部）」） |
| 按钮 | 关闭（超时/失败不提供重试按钮——重试=再次手动触发，明确操作路径文案「如需重试请再次手动触发」） |

长等待提示：触发中行按钮 tooltip「执行中，最长等待 120 秒」（同步语义用户可知会，api D-05）。

### 5.3 新建/编辑备份任务 Drawer（backup 页，backupJobCreate/backupJobUpdate）

NDrawer（右侧，宽 720px，标题「新建备份任务」/「编辑备份任务」），字段（api create req 推断；联动数据源=site-mgmt `/sites/site/list` 与 database-mgmt `/db/instance/list`、`/db/database/list` 既有接口，api 1.2⑦）：

| 字段 | 组件 | 取值与校验 |
|------|------|-----------|
| 名称 name | NInput | ≤64 字符；唯一性服务端 CB_005（编辑态排除自身） |
| 备份源 sourceItems | 动态行组（4.3.1） | 至少 1 行（I-09）；两类行混合 |
| ├ 站点目录行 | NSelect（multiple，options=站点清单 docRoot 路径 {label:站点名·路径}) | 选取即值快照（api 值快照语义——tooltip「保存后为路径快照，站点变更不自动跟随」） |
| └ 数据库行 | 实例 NSelect（options=/db/instance/list）+ 库 NSelect（multiple，options=/db/database/list?instanceId=，实例选定后拉取；未选库=全库） | instanceId 引用（I-09 InstanceRefChecker 服务端兜底 CB_001） |
| 目标类型 targetType | NRadioGroup（backup_target_type 两值，local 默认） | 条件显隐目标字段组 |
| ├ 本地目录（local） | NInput（绝对路径） | 非面板数据目录（服务端校验 CB_001，api D-06 防递归覆盖） |
| └ rsync 远程 | host NInput + user NInput + port NInputNumber（可空默认 22）+ path NInput | MVP ssh 免密（表单底部灰字「远程目标经 ssh 免密连接，面板不存储任何凭据」，api D-06） |
| 保留份数 retentionCount | NInputNumber（1-100） | I-05 水位 |
| 调度 scheduleCron | NInput（等宽）+ 校验预览区（同 5.1，复用 jobValidate，api D2 口径-2 单一表达式语义） | 合法五段（I-07/CB_002）；预览未通过禁止保存 |

按钮：**保存 / 取消**；保存 → backupJobCreate/Update → 成功 message + 关闭 + 重拉 backupJobList。

## 第 6 节 详情页（执行详情 Drawer，确认项 6）

**备份执行详情 Drawer**（backupRunDetail，NDrawer 右侧宽 640px；入口=历史表「详情」行内打开——详情不占独立路由，分册 operList 详情跳转规则的 Drawer 形态变体，docker-mgmt 容器详情 Drawer 同构）：

| Card | 内容与数据源 |
|------|-------------|
| 执行信息 Card | 状态 Tag（颜色第 3 节，running 附 loading+轮询中提示）· 任务名（jobName 快照）· 触发方式 · 开始/结束时间（本地时区）· 耗时（派生人性化） |
| 备份源明细 Card | sources[] 逐行：类型 Tag（backup_source_type）+ 引用（目录路径 / 实例·库名）；失败时 error_message 中对应源 red 标注 |
| 产物清单 Card（files） | success：表（文件名等宽 / 大小右对齐）+ 每行「下载」按钮（admin）→ backupRunDownload `{runId, file}`（浏览器原生流式下载，api D-10/D-16）；产物总大小与 artifactPath 灰字呈现（local 路径 / rsync 远端路径）。failure：NAlert error「执行失败：{errorMessage}」+ 无产物清单（I-04 不可下载）。running：NSpin「备份执行中，每 5 秒自动刷新」（第 2 节轮询纪律） |

**cron 任务无详情形态**：无 detail 接口，任务全字段（expr/command/remark/source）已在列表行内呈现（第 4.1.2 节）——与 firewall「rawSpec 列内承载」同判，不虚构详情页。

## 第 7 节 路由定义与框架行为

| 路由 | 页面 | meta |
|------|------|------|
| /cron | cron（计划任务页：双 Tab + 预置位分区 + 任务 Dialog + 触发 Modal） | `{requiresAuth:true}` |
| /cron/backup | backup（备份页：双 Tab + 任务 Drawer + 详情 Drawer） | `{requiresAuth:true}` |

**守卫**：复用全局守卫（initCheck → sessionCurrent → roles）；本模块无新增守卫。

**框架级边界**（复用 panel-core 框架行为）：菜单渲染/安全入口前缀/顶栏为 panel-core 宿主职责；2 个路由经模块页面挂载点接入（panel-core S-10），菜单项由 ModuleMenuRegistry 登记、服务端按角色下发（计划任务/备份两菜单对 admin/readonly 均可见——readonly 可进入查看，操作面为其不可见，FL/ADR-018）。

## 第 8 节 全局一致性

| # | 检查 | 结果 |
|---|------|------|
| 1 | Action ↔ api-cron-backup.md 接口一一对应（S7） | ✅ 15/15 接口全映射，无虚构、无遗漏 |
| 2 | 字典 Key ↔ 04-business-enum.md 定稿枚举一一对应（S8） | ✅ 5/5（cron_source/backup_source_type/backup_target_type/backup_trigger/backup_run_status）+ panel_role 复用；取值逐字一致 |
| 3 | 写 Action CSRF 注入完整（9 POST 全 ✓；download 为 GET 流式无 CSRF 面） | ✅ 第 2 节 + 全局拦截器 |
| 4 | 立即备份异步语义：返回 runId 即反馈 + 历史 Tab 高亮 + 详情 Drawer 轮询承接进度（5s/终态即停/关闭即停）——api D-07 留 D4 的轮询节奏已定义 | ✅ 第 2 节纪律表 + 4.2.3 + 第 6 节 |
| 5 | 手动触发同步语义：行级 loading 最长 120s + 结果 Modal（退出码/超时/输出尾部 64KB）——api D-05 | ✅ 4.1.3 + 5.2 |
| 6 | 表达式校验预览双消费：任务 Dialog + 备份 Drawer 调度字段复用同一 jobValidate Action（api D2 口径-2 单一表达式语义） | ✅ 5.1 + 5.3 |
| 7 | jobKey 不透明引用：列表透传回 update/delete/trigger，前端不解析不展示 | ✅ 4.1.2/4.1.3/5.1（api D-01） |
| 8 | 系统预置位只读分区：折叠呈现、无任何写操作入口（D1 口径-1） | ✅ 4.1.4 |
| 9 | CB_001~009 + AUTH_001/002/003/008 错误文案与 api 3.4 一一对应 | ✅ 第 1 节错误处理 |
| 10 | 实况投影如实性：CB_003/CB_004（面板外已删）warning+自动重拉、备注 null 呈「—」、已删任务历史灰字标「任务已删」（D-12 快照语义） | ✅ 第 1 节 + 4.1.2 + 4.2.4 |
| 11 | ADR-018：readonly 子集视角（无新建/编辑/删除/触发/立即执行入口、无下载按钮），负空间逐项对应 readonly 文档 | ✅ readonly 文档 |
| 12 | 无 📎 Scm\* 组件；Naive UI 为栈版本表选型；无独立路由表单页（弹窗形态判定已注明依据） | ✅ 头部 + 第 5 节 |
| 13 | 时间本地时区展示、原值透传不翻译（expr/command/scheduleCron 等宽呈现）；输出/日志尾部截断如实标注 | ✅ 4.1.5/5.2（api D-04/D-05） |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-14 | initial | 首次生成：2 hybrid 页面（/cron 计划任务页双 Tab+预置位分区+任务 Dialog+触发 Modal；/cron/backup 备份页双 Tab+任务 Drawer+执行详情 Drawer）、15 Action（GET 6 + POST 9）、5 字典 Key + panel_role 复用、0 独立路由表单页/0 独立详情页；立即备份异步 runId+Drawer 5s 轮询（api D-07 留 D4 节奏落定）、手动触发同步 120s 结果 Modal（api D-05）、表达式校验双消费预览（D2 口径-2）、系统预置位只读分区（D1 口径-1）为本阶段核心交互定义 |

> FX 质量清单说明：FX-001~FX-141 逐条清单文件未随引擎 0.5.2 payload 发布（firewall/docker-mgmt 等先例同口径复核），本节按分册 9 节结构逐节落实，并以第 8 节 13 项全局一致性自查替代可机械执行部分；全量 141 项留待 Gate 1 validation 执行。
