# 斗拱面板 系统监控（monitoring）API 设计

| 属性 | 值 |
|------|-----|
| 版本 | v1.0 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-12 |
| 输入 | prd-monitoring.md v1.0（完备性检查通过：G 5/5、T 适用 3/3、N 4/4）、domain-model.md v1.2（monitoring 域：2 值对象，无聚合根）、d0-clarification-monitoring.md、D1 三文档（BA/AA/FL v1.0） |
| 接口规约 | 无激活技术栈（ACTIVE_STACKS 为空）——按通用 REST/JSON 契约设计；认证=HttpOnly Cookie 会话（R-02）；鉴权=RBAC 两角色边界矩阵（ADR-018）；**零持久化**（ADR-014：无表、无 DDL、无 SQL） |

## 1. 模块设计概述

### 1.1 业务背景及目标

系统监控是斗拱面板的只读观测服务：为登录运维者提供所在服务器的实时快照（CPU 总/分核、内存、磁盘容量与 IO、网络上下行、负载）、TOP 进程清单、近 1h/24h 历史趋势与系统信息。数据由后台采集链路持续生产（3 秒采样 → 1 分钟聚合 → 进程内环形缓冲 1440 条 ≈ 24h，ADR-014 不落库），接口侧全部为只读查询（admin/readonly 同等可用，ADR-018）。它是登录后首页可见性的直接来源（idea.md §6 M1 验收），并被 panel-core 仪表盘页消费（D0 模块定位）。

### 1.2 边界说明

| 项 | 说明 |
|----|------|
| 本模块做 | 周期采样与环形缓冲维护（后台机制）、实时快照/历史趋势/TOP 进程/系统信息四类只读查询接口 |
| 本模块不做（接口边界） | ① systemd 服务管理——归 service-mgmt；② 告警/通知——非目标；③ 进程操作（kill/优先级）——不做；④ 监控页面外壳与导航——归 panel-core；⑤ 历史数据落库与 >24h 回看——ADR-014 拍板不落库；⑥ 认证/鉴权本体——复用 account-auth 中间件（本模块仅声明依赖） |
| 与存储的关系 | **零持久化**：无表、无 DDL、无 SQL（ADR-014；domain-model.md 存储映射说明「monitoring→无表」）；metric 为进程内环形缓冲，system_info 为实时投影 |

### 1.3 设计决策

| # | 决策 | 理由 |
|---|------|------|
| D-01 | 数据供给形态=**REST 拉取（轮询）**，不建 WS 推送通道：快照/TOP 前端轮询间隔建议 3s（与采样周期一致），趋势仅在切换窗口/维度时拉取一次 | 数据源 3s 采样，轮询已达更新上限；避免常驻长连接内存/连接开销（<60MB 轻量哲学）；终端 WS 场景归 terminal，监控无此必要。**关闭 prd-monitoring.md 待对齐-1** |
| D-02 | 度量单位与精度口径：速率一律 bytes/s（B/s）整数、百分比 1 位小数、容量 bytes 整数、负载原值（2 位小数）、uptime 秒整数；人类可读格式化归前端（D4/呈现层） | 接口层统一 SI 基本单位，消除单位歧义；前端按需格式化 |
| D-03 | 历史趋势响应统一形状 `series[]`：`{key, label, unit, points:[{ts, v}]}`，`v=null` 表达该分钟缺失（采样失败/未覆盖）；响应级 `startTime`（数据起点，表达重启清零边界）+ `partial`（窗口内存在缺失） | 单一形状承载全部维度（磁盘分设备/多序列），gap 与清零边界显式表达；**关闭 prd-monitoring.md 待对齐-3 字段形态部分**（呈现归 D4） |
| D-04 | CPU 趋势子口径走查询参数 `scope`（metric_scope{total, per_core}，默认 total，仅 dimension=cpu 时合法）；实时快照单响应两层（totalPercent + perCore[]）无需参数 | 快照一次拿全（页首渲染），趋势按需选择口径避免默认下发分核矩阵（核数×1440 点）；**关闭 prd-monitoring.md 待对齐-4 接口部分** |
| D-05 | 缓冲内存估算与护栏：单条目≈2KB（16 核/≤8 磁盘/≤8 网卡 + TOP 10 进程口径），1440 条 ≈ ≤3MB；不设设备数截断（如实记录），超大规格主机（>64 核或 >16 盘）由 B 阶段实测复核 | 远低于面板 <60MB 预算（idea.md §6）；不虚构截断规则，估算口径显式记录待实测 |
| D-06 | 混合模式（MIXED）声明：全部对象实际为自管只读（无外部数据源，R-09 零外呼），不设 manage_mode 字段，权限管控按角色区分 | 分册 Step 2 默认混合模式；本模块无托管侧、无写侧 |
| D-07 | 查询均为非分页单范围读：快照/TOP/系统信息为单点读；历史趋势以窗口为范围一次返回，窗口内无数据返回空 series + startTime=null（code=0，非错误流） | 监控查询天然幂等且量级有界（1440 点上限）；空数据为常态（重启初期），以正常响应表达 |
| D-08 | 采集失败不虚构数据：单指标失败该值缺失（趋势 v=null/快照字段置空并标注），系统信息部分失败失败字段 null + errors[] | R-14 精神延伸至数据侧：不伪造默认值；快照携带采集时间戳可辨识数据新旧 |

### 1.4 领域对象总览

| 领域对象 | 分类 | 承载 | 服务接口 |
|---------|------|------|---------|
| metric（监控指标条目） | 值对象 | 进程内环形缓冲（1min × 1440，滚动淘汰，ADR-014）；无表 | /monitoring/metrics/snapshot、/monitoring/metrics/history、/monitoring/processes/top |
| system_info（系统信息投影） | 值对象 | 实时获取不持久化；无表 | /monitoring/system/info |

无聚合根（D2 判定：A1-A4 全不通过，只读观测流）；无子流程；无关联领域对象（不持有任何外部域 ID）；无领域事件与 MQ（D-05 先例同理：单机单体，R-09）。

### 1.5 上下文映射

| 上下文 | 关系 | 机制 |
|--------|------|------|
| panel-core（仪表盘页） | 被依赖 | OHS：监控数据读取契约（本模块 4 个 GET 接口，见 5.1）；供给形态 REST 拉取（D-01） |
| account-auth | 遵奉者 | 全部接口位于 Cookie 会话后（R-02，AuthRequired）；readonly 全可看（ADR-018，RoleRequired）；复用其中间件链，不自建鉴权 |
| 操作系统（本机） | 数据源 | 进程内采集（gopsutil 选型，idea.md §5.3，材料给定事实）；非外部系统对接，无 ACL 必要（R-09 零外呼） |

> 跨聚合根设计（第 2 章）：不适用——本模块无聚合根（工具类裁剪矩阵第 2 章「—」），跨对象约束以不变量形式列于 4.1.1。

## 3. 应用/基础设施层设计

### 3.1 接口规范

| 项 | 规范 |
|----|------|
| 路径格式 | `/monitoring/{resource}/{action}` 三段式；资源复数（metrics/processes）或单数（system）按语义 |
| HTTP 方法 | 全部 GET（只读模块，无任何写接口） |
| 响应格式 | `{"code": 0, "message": "ok", "data": {...}}`；code=0 成功，非 0 见 3.4 错误码（与 account-auth D-07 同一契约） |
| 时间格式 | ISO-8601 UTC（公共约定）；秒级采样时间戳、分钟级序列点时间戳 |
| 认证 | 复用 account-auth：HttpOnly Cookie `dg_session`（AuthRequired） |
| CSRF | 不适用——本模块全部为 GET 只读（CSRF 校验针对变更类请求，R-02） |
| 中间件链 | AuthRequired → RoleRequired（全部接口 admin+readonly，无差异化角色分支） |
| 用户类型标注 | 全部接口：admin+readonly（ADR-018 全可看；访客不可达） |

### 3.2 依赖与约束

| 依赖 | 类型 | 约束 |
|------|------|------|
| account-auth 中间件 | 框架 | AuthRequired/RoleRequired 注册顺序见 3.1；不得自建会话解析 |
| 本机系统读取（gopsutil 类） | 数据源 | 采集协程容错：单轮单指标失败不中断采集进程（PRD N2）；采集周期 3s（⚠️A12） |
| panel-core | 下游消费方 | 仅经 4 个 GET 接口消费（OHS 契约，5.1）；不得直读进程内缓冲结构 |

### 3.3 权限模型

角色：admin / readonly（panel_role 固定枚举）；本模块无访客可达面。

**接口权限矩阵**：

| 接口 | admin | readonly | CSRF |
|------|-------|----------|------|
| GET /monitoring/metrics/snapshot | ✓ | ✓（ADR-018 全可看） | —（GET） |
| GET /monitoring/metrics/history | ✓ | ✓（ADR-018 全可看） | —（GET） |
| GET /monitoring/processes/top | ✓ | ✓（ADR-018 全可看） | —（GET） |
| GET /monitoring/system/info | ✓ | ✓（ADR-018 全可看） | —（GET） |

数据权限规则：无行级隔离——监控数据不含任何操作者维度，两角色可见集完全一致；本模块无变更接口，不存在只读越权拒绝面（越权场景=未登录，由 AUTH_001/002 统一处理）。

### 3.4 公共规范

**错误码**（monitoring 前缀，新增 4 个；认证类复用 account-auth 错误码 AUTH_001/002）：

| code | 标识 | HTTP 语义 | 说明 |
|------|------|----------|------|
| 0 | OK | 成功 | — |
| MON_001 | INVALID_WINDOW | 参数错误 | window 非法（不在 metric_window 取值内） |
| MON_002 | INVALID_DIMENSION | 参数错误 | dimension 非法（不在 metric_dimension 取值内） |
| MON_003 | INVALID_SCOPE | 参数错误 | scope 非法，或 dimension≠cpu 时携带 scope（D-04） |
| MON_004 | NO_DATA_YET | 数据未就绪 | 启动后尚无成功采样（快照/TOP/趋势均可能；code=0 空序列优先，见 D-07） |

**枚举**（均在 workspace/01-solution/04-business-enum.md 定义；前两个 D2 定稿、metric_scope 本次 D3 定稿）：

| 枚举标识 | 取值 | 用途 |
|---------|------|------|
| metric_window | last_1h \| last_24h | 趋势时间窗口 |
| metric_dimension | cpu \| memory \| disk \| network \| load | 趋势指标维度 |
| metric_scope | total \| per_core | cpu 维度统计口径（D-04） |
| panel_role | admin \| readonly | 权限模型角色（引用，account-auth 定稿） |

**业务常量**（非枚举池条目）：

| 常量 | 值 | 说明 |
|------|-----|------|
| 采样周期 | 3s | ⚠️A12；采集协程节拍 |
| 缓冲容量 | 1440 条（1min 粒度 ≈ 24h） | ⚠️A12；滚动淘汰 |
| TOP 进程数 | 10 | ⚠️A12；按 CPU 占用降序 |
| series 命名前缀 | cpu_total / cpu_core_{n} / memory_used / disk_capacity:{mount} / disk_io_read / disk_io_write / network_up / network_down / load_1m / load_5m / load_15m | D-03 统一形状的 key 规范 |

**配置项**：无——采样周期/缓冲容量/TOP N 为编译期常量（⚠️A12 定值，PRD OUT-6：无用户可配置面），非配置键、不入库。

## 4. 领域对象与服务详细设计

> 本模块无聚合根（工具类），本章按领域对象组织；各节保持与先例一致的子节结构（领域模型→状态模型→子流程→接口设计→事件与集成）。

### 4.1 metric（监控指标条目，值对象 + 环形缓冲链路）

#### 4.1.1 领域模型

**表结构组合**：无表（ADR-014 零持久化）。metric 条目驻留进程内环形缓冲：1 分钟聚合粒度 × 1440 条（⚠️A12），满后滚动淘汰最旧；面板重启即清空、从零积累。

**条目字段清单**（缓冲条目 = 快照/趋势/TOP 的共同数据源；单位口径 D-02）：

| 字段 | 类型 | 说明 |
|------|------|------|
| ts | datetime | 分钟聚合锚点时间戳（ISO-8601 UTC） |
| cpu_total | float(1 位小数) | CPU 总使用率 % |
| cpu_per_core | float[]（1 位小数） | 分核使用率 %，按核心序 |
| mem_used_bytes / mem_total_bytes / mem_used_percent | int / int / float | 内存占用 |
| disk_capacity | {mount, totalBytes, usedBytes}[] | 按挂载点分设备（⚠️A13） |
| disk_io_read_bps / disk_io_write_bps | int | 全设备合计 IO 速率（⚠️A13） |
| net_up_bps / net_down_bps | int | 全设备合计上下行速率（⚠️A13） |
| load_1m / load_5m / load_15m | float(2 位小数) | 系统负载 |
| top_processes | {pid, name, cpuPercent, memBytes}[]（≤10） | TOP 进程清单（⚠️A12，按 cpuPercent 降序） |
| degraded | bool | 本轮存在采集失败指标（部分字段缺省） |

实时层：最新 3s 采样条目（同结构、未聚合）仅驻留内存单槽位，供快照/TOP 直读；分钟聚合异步落入环形缓冲。双层组织（实时最新 + 历史缓冲）见 D1 BA 术语表。

**不变量**：

| 编号 | 不变量 | 违反处理 |
|------|--------|---------|
| I-01 | 缓冲条目不可变：写入后不更新（淘汰即丢弃，无修改语义） | 结构保证（只追加式写入） |
| I-02 | 缓冲容量恒 ≤1440 条：满后写入必伴随最旧条目淘汰 | 环形结构保证 |
| I-03 | 单写者：分钟聚合写入仅来自采集链路聚合步骤；读取并发不限，互斥策略为 B 阶段实现细节（PRD N2） | 架构约束 |
| I-04 | 重启即空：缓冲不落库，进程生命周期 = 数据生命周期（ADR-014）；history 的 startTime 恒 ≥ 本进程启动时刻 | 结构保证 |
| I-05 | 条目字段不伪造：采集失败指标置缺省并置 degraded=true（D-08）；聚合条目含 null 值时趋势对应点 v=null | 写入侧校验 |
| I-06 | TOP 进程 ≤10、按 cpuPercent 降序（⚠️A12） | 采样侧保证 |

**领域服务**：MetricsSampler（3s 周期采集 + 容错）、MinuteAggregator（3s→1min 聚合入缓冲）、RingBuffer（I-01~04 承载）。

#### 4.1.2 状态模型

无状态机（PRD 第六部分辨析：时间窗口/维度/口径为查询参数枚举，缓冲滚动覆盖为内存结构行为，均非领域状态）。

#### 4.1.3 子流程设计

无（采样与分钟聚合为后台机制链路，无独立生命周期状态；domain-model.md 第四部分已声明）。

#### 4.1.4 接口设计

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /monitoring/metrics/snapshot | GET | admin+readonly | 实时快照（最新采样条目） |
| /monitoring/metrics/history | GET | admin+readonly | 历史趋势序列（窗口 × 维度 × 口径） |
| /monitoring/processes/top | GET | admin+readonly | TOP 进程清单（最新轮） |

**接口详情**：

- **GET /monitoring/metrics/snapshot**：无 query。校验：无参数；数据就绪性见下。响应 data=`{ts, degraded, cpu:{totalPercent, perCore:[float]}, memory:{usedBytes, totalBytes, usedPercent}, disk:{capacity:[{mount, totalBytes, usedBytes}], ioReadBps, ioWriteBps}, net:{upBps, downBps}, load:{l1, l5, l15}, processes:[{pid, name, cpuPercent, memBytes}]}`——processes 为最新 TOP 清单（快照页一次取全，避免快照与 TOP 两次轮询错拍；/processes/top 保留为独立契约供仪表盘单取）。启动后首轮采样未完成返回 MON_004；单指标失败对应字段 null + degraded=true（D-08）。
- **GET /monitoring/metrics/history**：query `{window, dimension, scope?}`。校验（R-14）：window ∈ metric_window 否则 MON_001；dimension ∈ metric_dimension 否则 MON_002；scope 省略时默认 total，非空时须 ∈ metric_scope 且仅 dimension=cpu 合法，否则 MON_003。响应 data=`{window, dimension, scope, startTime, partial, series:[{key, label, unit, points:[{ts, v}]}]}`：
  - cpu + total → 1 条 series（key=cpu_total, unit=percent）；cpu + per_core → 每核 1 条（key=cpu_core_{n}）；
  - memory → 1 条（key=memory_used, unit=percent，v=usedPercent）；
  - disk → 每挂载点 1 条（key=disk_capacity:{mount}, unit=bytes, v=usedBytes）+ 2 条合计 IO（key=disk_io_read / disk_io_write, unit=bps）；
  - network → 2 条（key=network_up / network_down, unit=bps）；
  - load → 3 条（key=load_1m / load_5m / load_15m, unit=value）。
  - `startTime` = 窗口内最早有效点时间（受 I-04 约束 ≥ 进程启动时刻；无数据时 null）；`partial` = 窗口内存在 v=null 点或窗口未被数据完整覆盖；点数上限 = 窗口分钟数（last_1h→≤60、last_24h→≤1440）；窗口内无任何数据 → series=[] + startTime=null + code=0（D-07）。
- **GET /monitoring/processes/top**：无 query。响应 data=`{ts, processes:[{pid, name, cpuPercent, memBytes}]}`（≤10，降序，I-06）；本轮 TOP 采集失败返回 MON_004（区别于指标降级——进程维度无部分成功语义）。

**跨接口复用规则**：快照与 TOP 同源于最新采样槽位（一次内存读）；history 只读环形缓冲（SnapshotReader 不得穿透到缓冲内部结构，3.2 约束）；参数枚举校验统一走 ParamsGuard（MON_001~003 映射）。

#### 4.1.5 领域事件与集成

无 MQ、无领域事件（单机单体，R-09；D-05 先例同理）。「仓储契约」为零持久化替代——**缓冲访问契约（进程内，非 SQL）**：

```
MetricsBuffer {
  Latest() (Snapshot, bool)                        // 实时层最新条目（快照/TOP 共源）
  Series(window metric_window, dim metric_dimension,
         scope metric_scope) ([]Series, startTime, partial)  // 只读缓冲窗口投影（I-04）
  AppendMinute(entry MinuteEntry)                  // 单写者入口（I-03）
}
约束：Series 为只读投影，调用方不得持有内部切片引用；AppendMinute 淘汰语义由 RingBuffer 保证（I-02）
```

关联域调用：无出站；被调用契约 = 监控数据读取契约（4 个 GET 接口，5.1）。

### 4.2 system_info（系统信息投影，值对象）

#### 4.2.1 领域模型

**表结构**：无表——实时获取不持久化（kb system_info 只读投影），每次请求即时读取本机并组装响应。

| 字段 | 类型 | 说明 |
|------|------|------|
| distro | string \| null | 发行版（含版本）；识别失败 null |
| kernel | string \| null | 内核版本 |
| hostname | string \| null | 主机名 |
| uptimeSeconds | int \| null | 自上次启动累计秒数 |
| errors | {field, reason}[] \| null | 部分获取失败清单（D-08：不虚构默认值） |

**不变量**：I-07 每次请求实时读取，响应不含缓存态字段标识（准静态数据无需缓存语义，读取开销可忽略）；I-08 失败字段 null 并入 errors，不返回占位文本。

**领域服务**：SystemInfoReader（本机读取 + 容错组装）。

#### 4.2.2 状态模型

无状态机（静态标识 + 单调递增运行时长，无流转语义）。

#### 4.2.3 子流程设计

无。

#### 4.2.4 接口设计

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /monitoring/system/info | GET | admin+readonly | 系统信息（静态 + 运行时长） |

**接口详情**：

- **GET /monitoring/system/info**：无 query。响应 data=`{distro, kernel, hostname, uptimeSeconds, errors}`——静态与运行信息单响应返回（FL：同一「系统信息」入口承载 S-16-01/02）；全部字段失败仍返回 code=0 + errors 全量清单（读取行为本身不视为服务错误）。

#### 4.2.5 领域事件与集成

无 MQ、无仓储（零持久化）。关联域调用：无。

## 5. 模块清单汇总

### 5.1 对外服务清单（HTTP OHS 读取契约）

| 服务（接口） | 消费方 | 契约要点 |
|-------------|--------|---------|
| GET /monitoring/metrics/snapshot | 前端监控页；panel-core 仪表盘 | 最新采样全量；轮询 3s（D-01） |
| GET /monitoring/metrics/history | 前端监控页；panel-core 仪表盘 | 窗口×维度×口径序列；清零边界经 startTime/partial（D-03） |
| GET /monitoring/processes/top | 前端监控页；panel-core 仪表盘 | TOP 10 进程（⚠️A12） |
| GET /monitoring/system/info | 前端监控页；panel-core 仪表盘 | 静态+运行信息单响应 |
| 中间件 AuthRequired / RoleRequired | （复用） | account-auth 提供，顺序见 3.1 |

### 5.2 SQL 清单

**无**——本模块零持久化（ADR-014）：metric 为进程内环形缓冲、system_info 为实时投影，无任何 SQLite 表读写（domain-model.md 存储映射说明「monitoring→无表」）。

### 5.3 DDL

**无**——零持久化（同 5.2；本模块不新增任何表，面板库 schema 无变更）。

## 6. 更新记录

### 6.1 变更记录

| 版本 | 日期 | 变更类型 | 变更摘要 | 目标节号 |
|------|------|---------|---------|---------|
| v1.0 | 2026-11-12 | initial | 首次生成：4 个只读 HTTP 接口（/monitoring/{metrics/{snapshot,history},processes/top,system/info}，全部 GET、admin+readonly）+ 1 个进程内缓冲访问契约 MetricsBuffer + 零持久化（无表/无 DDL/无 SQL）+ 4 错误码（MON_001~004）+ 3 枚举引用（metric_window/metric_dimension D2 已定稿、metric_scope 本次定稿入池）+ 8 项设计决策（D-01 REST 轮询、D-03 series 统一形状、D-04 scope 参数、D-05 缓冲内存估算等）；关闭 prd-monitoring.md 待对齐-1/-3/-4；无旧版对比 | — |

### 6.2 变更类型词汇表

| 变更类型 | 含义 | 需要目标节号 |
|---------|------|------------|
| `initial` | 首次生成 | 否 |
| `structural` | 章节增删/重组 | 否 |
| `table_added` / `table_changed` | 新增/变更数据表 | 是 |
| `field_added` / `field_changed` / `field_removed` | 字段增删改 | 是 |
| `enum_added` / `enum_value_added` / `enum_value_changed` | 枚举增删改 | 是 |
| `interface_added` / `interface_changed` / `interface_removed` | 接口增删改 | 是 |
| `state_added` / `state_changed` / `flow_added` / `flow_changed` | 状态/流转增删改 | 是 |
| `rule_added` / `rule_changed` | 规则增删改 | 是 |
| `event_added` / `event_changed` | 领域事件增删改 | 是 |

### 6.3 版本号规则

语义化版本：vMAJOR.MINOR——结构/接口增删改升 MAJOR；字段/枚举/规则微调升 MINOR；本文档随模块迭代在 6.1 追加行。
