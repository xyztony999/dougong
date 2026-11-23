# 斗拱面板 容器管理（docker-mgmt）API 设计

| 属性 | 值 |
|------|-----|
| 版本 | v1.2 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-23（v1.2 宝塔式重组 D3：+6 个 /docker/app/* 接口与 /docker/compose/delete；DK_028~032；Compose 列表并入落盘扫描；归属标签注入收窄） |
| 输入 | prd-docker-mgmt.md v1.0（完备性检查通过：G 5/5、T 适用 6/6、N 4/4）、domain-model.md v1.13（docker-mgmt 域：5 实体 container/image/volume/network/compose_project，无聚合根）、d0-clarification-docker-mgmt.md、D1 三文档（BA/AA/FL v1.0，BA 附录B 状态映射与动作矩阵） |
| 接口规约 | 无激活技术栈（ACTIVE_STACKS 为空）——按通用 REST/JSON 契约设计；认证=HttpOnly Cookie 会话（R-02）；鉴权=RBAC 两角色边界矩阵（ADR-018）；**零持久化**（kb 拍板：无表、无 DDL、无 SQL；容器/镜像/卷/网络经官方 Go SDK、Compose 经 `docker compose` CLI 参数数组，R-08/ADR-013） |

## 1. 模块设计概述

### 1.1 业务背景及目标

容器管理是斗拱面板 M9 里程碑（idea.md §4.1；§6 验收章节标 M6）的容器运维模块，也是「接管存量环境」哲学（R-12/R-15）在容器维度的承载：连接本机既有 Docker daemon（`/var/run/docker.sock`，官方 Go SDK），已有容器、镜像、卷、网络与 Compose 项目自动纳入管理——接管而非重装，未安装 Docker 时以安装引导页呈现而非报错；容器生命周期操作受 ADR-025 状态映射与动作矩阵约束（BA 附录B 唯一口径），Compose 经容器 label 自动识别项目并以 `docker compose` CLI 参数数组方式操作（R-13/ADR-013，定位失败时容器级操作兜底）；与应用商店路线明确切割（R-12 不做一键部署）。只读用户同等查看全部清单与日志（ADR-018「全可看」，日志属查看类）。服务对象：单机自托管的管理员（admin）与只读用户（readonly）。

### 1.2 边界说明

| 项 | 说明 |
|----|------|
| 本模块做 | daemon 接管结论查询（状态/版本/不可用细节）、容器清单（含 Compose 归属与权威动作面）/日志（尾部+跟随）/资源占用快照、容器六动作（启停/重启/暂停恢复/删除）、镜像清单/拉取/删除、卷与网络的清单/删除、Compose 项目清单（label 聚合）/up/down/restart/项目日志（尾部+跟随） |
| 本模块不做（接口边界） | ① 应用商店/一键部署——不做（R-12）；② 容器内终端——二期（terminal 域被依赖预期，PRD OUT-2）；③ 镜像构建/Dockerfile/Registry 私仓管理——不做（OUT-3/4）；④ Swarm/K8s、跨主机、远程 daemon 接入——不做（OUT-5/6）；⑤ 容器/镜像/卷/网络创建类动作——不做（OUT-7，经 Compose up 或面板外 docker CLI 达成，清单如实呈现）；⑥ 容器级历史指标——归 monitoring（OUT-8）；⑦ Docker 安装执行——不做（引导页仅检测与指引，OUT-9）；⑧ 日志持久化/轮转/历史检索——不做（daemon 实况流，OUT-10）；⑨ 认证/鉴权/审计本体——复用 account-auth 中间件与 AuditService.Record（仅声明依赖）；⑩ 模块页面外壳——归 panel-core |
| 与存储的关系 | **零持久化**：无表、无 DDL、无 SQL（kb 拍板；domain-model.md 存储映射说明「docker-mgmt→无表」）；五类对象均为 daemon/label 实况投影 |

### 1.3 设计决策

| # | 决策 | 理由 |
|---|------|------|
| D-01 | daemon 探测=**每次 GET /docker/daemon/status 即时探测**（无缓存层、按需拉取，monitoring D-01 / service-mgmt D-03 / firewall D-01 同口径）；响应 `{state: docker_daemon_state, version, detail?}`——available 时 version 伴随呈现、无 detail；unavailable 时 detail 为探测结论细节透传（`not_installed` / `socket_unreachable`，透传属性非枚举），支撑「未装引导」与「运行中不可达」的呈现区分；业务面（清单/操作）调用时服务端前置复核，unavailable → DK_002（**不返回空清单**——未装≠无容器，I-01） | 探测为轻量本机判定，按需拉取即最新结论；细节透传避免为二值区分扩枚举（docker_daemon_state 保持 D2 定稿二值）。**关闭 prd 待对齐-8** |
| D-02 | 容器清单**全量返回不分页** + 服务端筛选（`state?` 多值逗号分隔 container_status、`composeProject?` 等值）+ 排序（名称字典序）；每容器携带 **supportedActions 服务端权威动作面**（按当前映射态 × BA 附录B.2 矩阵计算：start/stop/restart/pause/unpause/delete 六动作子集，过渡态为空数组），前端操作入口照此渲染、不自行推断（firewall D-01 supportedActions 同构）；composeProject 字段=容器 label 透传（无项目为 null） | 单机容器数量级为数十~百条（PRD N1 量级锁定），全量返回免分页往返；动作面判定属状态映射事实，收归服务端权威（R-14 精神）。**关闭 prd 待对齐-1（含字段集与分页复核）** |
| D-03 | 日志双通道：**尾部读取=GET /docker/{container,compose}/logs**（JSON 行数组，`tail` 参数缺省 500、取值域 1-2000，D1 口径-3 参数化）；**实时跟随=WS /docker/{container,compose}/logs-follow**（`?containerId`/`?project`+`tail` 握手参数；terminal WS 先例：Cookie 会话握手鉴权 + Origin 同源校验；服务端帧两类 `{"type":"log","line"}` 与 `{"type":"end","reason"}`，end 后服务端关闭；reason ∈ container_removed / container_stopped / project_gone / locate_failed / daemon_lost——业务常量非枚举池条目，不入审计）；客户端无上行帧，断开即取消订阅 | 尾部为一次性低量读取走 REST；跟随为长连接流走 WS（terminal D-09 同判：WS 握手复用面板会话、Origin 防跨站）；两通道同源 daemon 日志流。**关闭 prd 待对齐-2** |
| D-04 | 动作-状态校验=**服务端矩阵守卫**：六个生命周期端点按 BA 附录B.2 唯一口径校验（start←stopped；stop/restart←running\|paused；pause←running；unpause←paused；delete←stopped），不匹配 → DK_004，data=`{currentState, action, allowedActions}`（allowedActions 即该容器权威动作面，与 D-02 同源计算）；**delete 请求无 force 类参数**（MVP 无强制删除，I-04/I-05） | 前端入口禁用可绕过、服务端不可（防直调）；矩阵为 ADR-025/BA 附录B 拍板口径，服务端唯一实现点。**关闭 prd 待对齐-3** |
| D-05 | 镜像拉取=**同步 POST**：daemon pull 阻塞至完成/失败（超时 600s 编译期常量），成功返回 `{imageId, ref}`，失败 DK_013（data.reason 透传 daemon 错误：tag 不存在/网络不通/仓库限流等）；MVP 不做进度流（D4 以前端 pending 态呈现；进度流二期） | 单机面板拉取为低频操作，同步终态最简可靠；进度流引入长连接复杂度不值当（PRD N1 未锁定项落定）。**关闭 prd 待对齐-4** |
| D-06 | Compose 操作=**动词端点** POST /docker/compose/{up,down,restart}（service-mgmt 动作段风格）+ GET /docker/compose/logs；CLI 组装=`docker compose --project-name <p> --project-directory <dir> -f <file> <verb>`（label 取 `com.docker.compose.project` / `.working_dir` / `.config_files` 三键，R-13；全部参数数组执行，R-08/ADR-013）；超时 300s；**同项目并发互斥**=进程内 per-project 串行执行，进行中再提交 → DK_018（data.busy=true）；compose/list 响应每项目携带 `locatable: bool`（服务端定位判定：工作目录存在且配置文件可读），false 时操作/日志端点 → DK_015 | 动词端点与生命周期六动作风格一致；compose_action 枚举为操作集权威口径（审计 detail 与 D4 动作渲染），承载形态为路径动作段与 logs 端点（枚举池行依据已补注）；locatable 前置判定避免操作时才发现定位失败。**关闭 prd 待对齐-5** |
| D-07 | 资源占用快照=**单次 stats 读取**：CPU%（daemon stats 计算值）+ 内存 usage/limit/percent 一次快照返回（无短窗 stream、无历史）；快照为读取时派生值（响应即消亡，domain-model 过程数据 P2） | 排障场景单次快照足够（PRD FR-05）；历史趋势归 monitoring（OUT-8）。**关闭 prd 待对齐-6** |
| D-08 | daemon 拒绝原因映射（R-14 之外的错误归一）：SDK/daemon 错误按语义归类——`not found` → 各资源 DK_00x_NOT_FOUND；`conflict/in use`（镜像被引用/卷被挂载/网络被接入）→ DK_006/008/010；预定义网络删除 → DK_011；其余 → DK_017（data.reason 原 stderr 透传，不虚构）；Compose CLI 失败 → DK_016（data.reason 透传） | daemon 拒绝为删除约束最终权威（D1 口径-4）；语义归类支撑前端精准提示，原文透传保真。**关闭 prd 待对齐-7** |
| D-09 | 容器创建=**同步 POST /docker/container/create**：服务端逐项校验（容器名白名单 ^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$ /端口映射 host:container 格式与范围/环境变量 KEY=VALUE/挂载 source:target[:mode]/重启策略四值/网络名存在于清单）→ 镜像本地缺失自动先拉取（复用 pull 通道 600s）→ SDK ContainerCreate+Start → 创建后即读实况返回 `{containerId, name, state}`；端口冲突/名称冲突/网络不存在等 daemon 拒绝如实透传（DK_019~023 语义归类） | docker run 语义一步到位（D0 Q2）；校验服务端权威 R-14；零台账投影不变——创建后经 list 即时可见 |
| D-10 | 卷创建/网络创建=**同步 POST**（S-48-03/S-49-03）：卷 `{name, driver?=local}`、网络 `{name, driver?=bridge, subnet?, gateway?}`；重名/非法名称/子网冲突 daemon 拒绝如实透传（DK_024/DK_025 data.reason）；创建后即时进入清单实况并可选为容器创建数据源（表单联动，D0 Q4） | daemon 为最终权威（I-05 同构）；创建后投影刷新 |
| D-11 | Compose 项目创建=**同步 POST /docker/compose/create**：req `{projectName, composeYaml, autoUp?=true}`；服务端 YAML 语法校验（失败 DK_026 data.reason 带行号）→ 项目名白名单 ^[a-z0-9][a-z0-9-]{0,63}$（防逃逸+根前缀约束）→ 落盘根 `/opt/dougong-compose/{projectName}/docker-compose.yml`（编译期常量；目录已存在 → DK_027 重名拒绝）→ 原子写 0644 → autoUp 时经 ComposeRunner up -d（复用项目级互斥 DK_018/D-06）→ 响应 `{projectName, workingDir, configFile, started}`；**归属标签预埋（⚠️A22）**：服务端在写入前向 YAML labels 段注入 `dougong.app={projectName}`（app-mgmt 容器化应用消费，全网视图经标签区分归属）；compose 全文不落审计 detail（脱敏 ⚠️A9 同构） | 落盘根常量+白名单双防逃逸；YAML 校验库选型（go-yaml）与互斥复用沿既有；**关闭 prd v1.1 待对齐（校验器/常量）** |
| D-12 | 错误码接力 DK_019~027（容器创建语义 5 + 卷/网络创建 2 + Compose 创建 2），零台账哲学不变（创建对象即时进入实况投影，无新域对象无新状态机）；审计动作常量 +4：container_created/volume_created/network_created/compose_project_created | 编号全序唯一沿既有 DK 段；A-APP 段无交叉 |
| D-09 | **操作与审计非同事务**：业务动作在 daemon/CLI 侧（非 SQLite 事务域），动作完成后经 AuditService.Record 独立写入；写入失败重试一次，仍失败记面板运行日志兜底、不影响操作结果返回 | account-auth I-05「同事务」前提是变更落 SQLite；本模块动作不可回滚，强一致不可达成——如实降级并显式声明（firewall D-09 / service-mgmt D-06 同判） |
| D-10 | 零持久化与混合模式（MIXED）声明：五类对象实际为自管投影（数据源=本机 Docker daemon，无外部同步源，R-09），不设 manage_mode 字段；写侧为 SDK 调用/命令转发非数据 CRUD | kb 拍板不落库；分册 Step 2 默认混合模式，本模块无托管侧（firewall D-10 / service-mgmt D-07 同构） |
| D-11 | 资源寻址标识：`containerId`（daemon 64-hex ID）、`imageId`（daemon ID）、`volumeName`（卷名原生即稳定标识）、`networkId`（daemon ID）、`project`（label 项目名）——均为前端**不透明引用**（来自各 list 响应，原样透传回操作端点，不解析内容仅校验非空） | 与 domain-model 判定一致（标识跨读取稳定可寻址）；不透明引用避免前端拼装/猜测标识形态 |

### 1.4 领域对象总览

| 领域对象 | 分类 | 承载 | 服务接口 |
|---------|------|------|---------|
| container（容器投影，containerId 标识） | 实体 | daemon 实况投影集合（进程内，读取时整集合刷新，D-10 无表） | /docker/container/{list,logs,logs-follow,stats,start,stop,restart,pause,unpause,delete} |
| image / volume / network（资源投影，imageId/volumeName/networkId 标识） | 实体 | daemon 实况投影集合（同上） | /docker/{image/{list,pull,delete}, volume/{list,delete}, network/{list,delete}} |
| compose_project（label 聚合投影，project 标识） | 实体 | 容器 label 聚合派生投影（读取时聚合刷新，D-10 无表） | /docker/compose/{list,up,down,restart,logs,logs-follow} |

无聚合根（D2 判定：A1-A4 均不通过——事实源在 daemon 侧、终模块无跨域引用、容器创建不在面板动作集、无面板侧状态驱动）；无子流程（生命周期/拉取/Compose 操作为单动作即时执行，domain-model.md 第四部分声明）；无关联领域对象（terminal 容器内终端为二期被依赖预期不预先建模）；无领域事件与 MQ（单机单体，R-09；审计为进程内 OHS 出站调用，D-09）。daemon 接管结论（docker_daemon_state）为域级判定值非对象状态字段（D2 注记）；资源占用快照、日志流内容、拉取/操作在途为过程数据（读取响应即消亡/一次性执行，domain-model.md 第三部分注），不入领域对象。

### 1.5 上下文映射

| 上下文 | 关系 | 机制 |
|--------|------|------|
| account-auth | 遵奉者 + OHS 消费 | 全部接口位于 Cookie 会话后（R-02，AuthRequired）；readonly 同等查看（含日志与 WS 跟随）、写操作 admin 专属（ADR-018，RoleRequired + AUTH_008 边界拒绝）；写接口 CSRF（R-02，CsrfGuard）；WS 握手 Cookie 鉴权 + Origin 同源（terminal D-09 同判）；审计写入 AuditService.Record（R-03，D-09 降级语义） |
| 本机 Docker daemon | 投影与动作源 | dockerx 适配器：容器/镜像/卷/网络经官方 Go SDK（idea.md §5.3），Compose 经 `docker compose` CLI 参数数组（R-08/ADR-013）；本机系统组件非外部业务系统，无 ACL 必要（R-09 零外呼——拉取外呼在 daemon 侧且为用户主动发起，面板不直接对接 registry） |
| 前端容器页 | 唯一消费方 | 仅经 24 个端点消费（5.1）；无其他业务模块依赖本模块（terminal 二期为预期方向） |

> 跨聚合根设计（第 2 章）：不适用——本模块无聚合根（工具类裁剪矩阵第 2 章「—」），对象级约束以不变量形式列于 4.1.1/4.2.1/4.3.1。

## 3. 应用/基础设施层设计

### 3.1 接口规范

| 项 | 规范 |
|----|------|
| 路径格式 | `/docker/{resource}/{action}` 三段式（域=docker；资源=daemon/container/image/volume/network/compose；动作段含 9 查询 + 13 写 + 2 WS）；资源单数（与 account-auth/service-mgmt/firewall 资源单数风格一致） |
| HTTP 方法 | 读=GET（status/list×5/logs×2/stats + WS×2 升级），写=POST（生命周期×6 + pull/delete×4 + compose×3，面板内部契约，account-auth D-07 同一约定） |
| 响应格式 | `{"code": 0, "message": "ok", "data": {...}}`；code=0 成功，非 0 见 3.4 错误码（与 account-auth D-07 同一契约）；清单 data=`{items, total}`（公共约定） |
| 时间格式 | ISO-8601 UTC（公共约定）；generatedAt 为投影生成时刻 |
| 认证 | 复用 account-auth：HttpOnly Cookie `dg_session`（AuthRequired）；WS 端点握手同 Cookie 鉴权 + Origin 同源校验（terminal D-09 同判） |
| CSRF | 全部 POST 须带 `X-CSRF-Token` 头（R-02，CsrfGuard）；GET 与 WS 豁免（WS 无变更语义） |
| 中间件链 | AuthRequired → CsrfGuard（POST）→ RoleRequired（读=admin+readonly，写=admin；WS=admin+readonly）→ DaemonGuard（业务面前置复核 daemon 可用性，D-01，unavailable → DK_002） |
| 用户类型标注 | 读与 WS 接口：admin+readonly（ADR-018 全可看，日志属查看类）；写接口：admin（readonly 无界面入口 + 越界 AUTH_008 全局边界拒绝） |

### 3.2 依赖与约束

| 依赖 | 类型 | 约束 |
|--------|------|------|
| account-auth 中间件 | 框架 | AuthRequired/CsrfGuard/RoleRequired 注册顺序见 3.1；不得自建会话解析与角色判定 |
| account-auth AuditService.Record | OHS | 操作审计经其写入（R-03）；失败重试一次后日志兜底（D-09），不得自建审计表 |
| 本机 Docker daemon（dockerx 适配器） | 数据源与动作通道 | SDK 交互经 DockerGateway、CLI 经 ComposeRunner（R-08 参数数组禁 shell 拼接）；SDK 操作超时 60s、pull 600s、Compose CLI 300s（编译期常量）；通道失败归一 DK_016/DK_017 语义 |
| panel-core | 框架 | 路由组挂载 `/docker`（HTTP + WS 升级）；不得越界承载业务逻辑 |

### 3.3 权限模型

角色：admin / readonly（panel_role 固定枚举）；本模块无访客可达面（全部位于登录会话后）。

**接口权限矩阵**：

| 接口 | admin | readonly | CSRF |
|------|-------|----------|------|
| GET /docker/daemon/status | ✓ | ✓（ADR-018 全可看，含未装结论） | —（GET） |
| GET /docker/container/list / image/list / volume/list / network/list / compose/list | ✓ | ✓（ADR-018 全可看） | —（GET） |
| GET /docker/container/logs / compose/logs | ✓ | ✓（日志属查看类，ADR-018） | —（GET） |
| GET /docker/container/stats | ✓ | ✓ | —（GET） |
| WS /docker/container/logs-follow / compose/logs-follow | ✓ | ✓（查看类流；Cookie 握手 + Origin） | —（WS） |
| POST /docker/container/{start,stop,restart,pause,unpause,delete} | ✓ | ✗（无界面入口 + AUTH_008 边界拒绝，ADR-018） | ✓ |
| POST /docker/image/{pull,delete} | ✓ | ✗（同上） | ✓ |
| POST /docker/volume/delete / network/delete | ✓ | ✗（同上） | ✓ |
| POST /docker/compose/{up,down,restart} | ✓ | ✗（同上） | ✓ |

数据权限规则：无行级隔离——五类清单与日志两角色可见集完全一致；角色差异仅体现在写接口（admin 专属）。readonly 越界由全局中间件 RoleRequired 拦截返回 AUTH_008（附边界原因），接口层不自建权限分支。

### 3.4 公共规范

**错误码**（DK_ 前缀为本模块新增 18 个；鉴权/CSRF/越界复用 account-auth 码族）：

| code | 标识 | HTTP 语义 | 说明 |
|------|------|----------|------|
| 0 | OK | 成功 | — |
| DK_001 | INVALID_PARAM | 参数错误 | containerId/imageId/volumeName/networkId/project/tail 非空与取值校验（不透明引用非空 + tail 1-2000；R-14/D-11） |
| DK_002 | DAEMON_UNAVAILABLE | 拒绝 | 接管判定 unavailable 时访问业务面（前端应据 status 呈现引导页；**不返回空清单**——未装≠无容器，I-01/D-01） |
| DK_003 | CONTAINER_NOT_FOUND | 拒绝 | 容器不存在（面板外已删，不静默成功） |
| DK_004 | ACTION_STATE_MISMATCH | 拒绝 | 动作与当前状态不匹配（BA 附录B.2 矩阵；data={currentState, action, allowedActions}，I-04/D-04） |
| DK_005 | IMAGE_NOT_FOUND | 拒绝 | 镜像不存在（删除目标） |
| DK_006 | IMAGE_IN_USE | 拒绝 | 镜像被容器引用（daemon 拒绝透传，D-08） |
| DK_007 | VOLUME_NOT_FOUND | 拒绝 | 卷不存在 |
| DK_008 | VOLUME_IN_USE | 拒绝 | 卷被容器挂载（daemon 拒绝透传） |
| DK_009 | NETWORK_NOT_FOUND | 拒绝 | 网络不存在 |
| DK_010 | NETWORK_IN_USE | 拒绝 | 网络被容器接入（daemon 拒绝透传） |
| DK_011 | NETWORK_PREDEFINED | 拒绝 | 预定义网络不可删除（bridge/host/none 等，daemon 拒绝语义归类，D-08） |
| DK_012 | IMAGE_REF_INVALID | 参数错误 | 镜像引用格式非法（`名称[:tag]` 服务端格式校验，R-14） |
| DK_013 | PULL_FAILED | 操作失败 | 拉取失败（data.reason 透传 daemon 错误：tag 不存在/网络不通/限流等，D-05/D-08） |
| DK_014 | COMPOSE_PROJECT_NOT_FOUND | 拒绝 | 项目不存在（label 聚合无此项目名） |
| DK_015 | COMPOSE_LOCATE_FAILED | 拒绝 | 定位失败（工作目录已移除/配置文件缺失；data={workingDir, configFile, missing}，呈现 label 记录路径，D1 口径-2/D-06） |
| DK_016 | COMPOSE_EXEC_FAILED | 操作失败 | compose CLI 执行失败（data.reason 透传 stderr；含超时，I-03） |
| DK_017 | DAEMON_EXEC_FAILED | 操作失败 | SDK 操作失败（daemon 内部错误，data.reason 原文透传不虚构，D-08） |
| DK_018 | COMPOSE_OPERATION_BUSY | 拒绝 | 同项目 up/down/restart 进行中（进程内 per-project 串行互斥，data.busy=true，D-06/I-08） |
| DK_019 | CONTAINER_NAME_INVALID | 参数错误 | 容器名格式非法（D-09 白名单，R-14） |
| DK_020 | PORT_MAPPING_INVALID | 参数错误 | 端口映射格式非法（host:container 范围/格式，R-14） |
| DK_021 | ENV_FORMAT_INVALID | 参数错误 | 环境变量格式非法（须 KEY=VALUE，R-14） |
| DK_022 | MOUNT_FORMAT_INVALID | 参数错误 | 挂载格式非法（source:target[:mode]，R-14） |
| DK_023 | CONTAINER_CREATE_FAILED | 操作失败 | 容器创建/启动失败（名称冲突/端口占用/网络不存在等 daemon 拒绝透传 data.reason；镜像拉取失败 DK_013 透传） |
| DK_024 | VOLUME_CREATE_FAILED | 操作失败 | 卷创建失败（重名/非法名称 daemon 拒绝透传 data.reason，D-10） |
| DK_025 | NETWORK_CREATE_FAILED | 操作失败 | 网络创建失败（重名/子网冲突等透传 data.reason，D-10） |
| DK_026 | COMPOSE_YAML_INVALID | 参数错误 | compose.yml 语法校验失败（data.reason 带行号，D-11） |
| DK_027 | COMPOSE_PROJECT_EXISTS | 拒绝 | 项目已存在（落盘目录已存在，data.projectName，D-11） |
| AUTH_001/002 | （复用） | 401 | 未登录/会话过期——中间件归 account-auth |
| AUTH_003 | （复用） | 403 | CSRF 校验失败（POST） |
| AUTH_008 | （复用）FORBIDDEN | 拒绝 | readonly 越界写操作，附边界原因（ADR-018） |

**枚举**（均在 workspace/01-solution/04-business-enum.md 定义；三个容器枚举 D2 已定稿，本次为引用无新增）：

| 枚举标识 | 取值 | 用途 |
|---------|------|------|
| docker_daemon_state | available \| unavailable | 接管判定值（status 响应 state 字段；unavailable→未装引导降级，D-01） |
| container_status | running \| stopped \| paused | 容器映射态（container/list 响应 state 字段 + list 筛选参数多值域；created/exited/dead→stopped 归并、restarting/removing 过渡态透传原始值，BA 附录B.1/D-02） |
| compose_action | up \| down \| restart \| logs | Compose 操作集权威口径（up/down/restart 经路径动作段承载、logs 经 GET 端点承载，D-06；审计 detail 与 D4 动作渲染引用） |
| panel_role | admin \| readonly | 权限模型角色（引用，account-auth 定稿） |
| audit_action / audit_result | change \| security / success \| failure | 操作审计类别与结果（引用，account-auth 定稿） |

**业务常量**（操作审计细粒度动作码，非枚举池条目）：`container_started / container_stopped / container_restarted / container_paused / container_unpaused / container_deleted / image_pulled / image_deleted / volume_deleted / network_deleted / compose_up / compose_down / compose_restarted`（action 列存此码，category=audit_action.change，object_type=container/image/volume/network/compose_project，object_id=containerId/imageId/volumeName/networkId/project）。

**业务常量**（操作参数与帧协议，非枚举池条目）：

| 常量 | 值 | 说明 |
|------|-----|------|
| SDK 操作超时 | 60s | DockerGateway 单操作等待上限（编译期常量；超时归一 DK_017） |
| 拉取超时 | 600s | daemon pull 同步等待上限（D-05；超时归一 DK_013） |
| Compose CLI 超时 | 300s | ComposeRunner 单命令等待上限（D-06；超时归一 DK_016） |
| tail 参数 | 缺省 500，取值域 1-2000 | 日志尾部行数（D1 口径-3 参数化；非法 → DK_001） |
| WS end 帧 reason | container_removed / container_stopped / project_gone / locate_failed / daemon_lost | 日志跟随终止原因（帧载荷常量，不入审计非池条目，D-03） |
| Compose label 键 | com.docker.compose.project / com.docker.compose.project.working_dir / com.docker.compose.project.config_files | 项目识别与定位依据（官方 label，R-13/D-06） |

**配置项**：无——超时/tail 缺省/矩阵口径为编译期常量与代码逻辑（无用户可配置面）；无新增配置键。

## 4. 领域对象与服务详细设计

> 本模块无聚合根（工具类），本章按领域对象组织；各节保持与先例一致的子节结构（领域模型→状态模型→子流程→接口设计→事件与集成）。

### 4.1 daemon 接管与 container（容器投影与生命周期）

#### 4.1.1 领域模型

**表结构组合**：无表（D-10 零持久化）。container 驻留进程内，为每次 daemon 读取时重建的投影集合（读取即刷新，无驻留缓存层）；生命周期操作直达 daemon（SDK 契约），不经投影对象中转持久化。

**接管结论条目**（status 响应载荷，域级判定值非对象状态字段）：

| 字段 | 类型 | 说明 |
|------|------|------|
| state | enum(docker_daemon_state) | available / unavailable（D-01 即时探测） |
| version | string \| null | daemon 版本（available 时伴随呈现，能力代际识别用；unavailable 为 null） |
| detail | string \| null | unavailable 时的探测结论细节（not_installed / socket_unreachable，透传属性；available 为 null） |

**容器投影条目字段清单**（list/stats 响应共同数据源）：

| 字段 | 类型 | 说明 |
|------|------|------|
| containerId | string | daemon 64-hex ID（D-11 不透明引用，操作/日志/快照寻址） |
| name | string | 容器名 |
| image | string | 镜像引用（名称:tag） |
| state | enum(container_status) \| string | 核心三态（映射归并，BA 附录B.1）；过渡态时透传原始值（restarting/removing） |
| composeProject | string \| null | 所属 Compose 项目（label 透传；无项目为 null） |
| supportedActions | [string] | 服务端权威动作面：按当前态 × BA 附录B.2 矩阵计算的六动作子集（过渡态为 []，D-02/D-04） |

**资源占用快照条目**（stats 载荷，过程数据——读取响应即消亡，不入领域对象）：

| 字段 | 类型 | 说明 |
|------|------|------|
| cpuPercent | number | CPU 占用率（daemon stats 计算值，D-07） |
| memUsageBytes / memLimitBytes / memPercent | number | 内存用量/上限/百分比（单次快照） |

**不变量**：

| 编号 | 不变量 | 违反处理 |
|------|--------|---------|
| I-01 | 投影真实性：全部清单/状态响应恒为当次 daemon 实况读取快照；无缓存层、无陈旧数据回退；daemon unavailable 访问业务面返回 DK_002（**不返回空清单**——未装≠无容器）；空清单仅出现于 daemon 可用且确无对象时 | 读取侧保证（每次请求即时读取，D-01） |
| I-02 | 零持久化：五类对象不落库，面板重启无本地状态；面板外 docker CLI 变更经下次读取如实反映；容器状态历史不在本模块留存（审计归 account-auth） | 结构保证（无表） |
| I-03 | 通道纪律：SDK 交互经 DockerGateway、Compose 经 ComposeRunner 参数数组执行（R-08 禁 shell 拼接），超时见常量表；Gateway/Runner 为唯一 daemon 交互出口 | 结构保证（适配层唯一出口） |
| I-04 | 动作矩阵强制：六个生命周期端点按 BA 附录B.2 唯一口径服务端校验，不匹配 → DK_004（data 携权威动作面）；delete 仅 stopped 态开放、请求无 force 类参数（MVP 无强制删除）；supportedActions 恒为服务端权威计算 | ContainerActionService 校验（D-02/D-04） |
| I-05 | 删除约束权威：镜像/卷/网络删除可达性以 daemon 拒绝为最终权威（无前置引用推导、不透传 force 参数绕过）；预定义网络不可删如实呈现（DK_011） | 删除端点校验 + 错误归一（D-08） |
| I-06 | 可审计：每个变更端点调用恰好产生一条审计记录（success/failure 如实；DK_001/002/004 拒绝未发生变更不记，R-03 字面口径）；审计写入失败按 D-09 降级并日志兜底 | 变更服务编排（I-07 同构 firewall） |

**领域服务**：DaemonProbeService（sock 探测编排 + 结论组装，D-01）、ContainerProjector（容器投影组装 + 筛选排序 + composeProject 标注 + supportedActions 计算）、ContainerActionService（写编排：矩阵校验→SDK 执行→审计）、LogStreamService（尾部读取与 WS 跟随订阅管理：tail 回放 + 流式推送 + 终止判定）、StatsService（单次快照读取）。

#### 4.1.2 状态模型

面板侧无自有状态机写入（I-02 零持久化）；容器状态机（ADR-025）为 daemon 实况的面板动作映射——state 为映射呈现值（读取时整值刷新），动作下发为状态迁移请求，状态事实源在 daemon。

辨析：① state 三态为映射归并值（created/exited/dead→stopped）、restarting/removing 为透传过渡值（PRD 第六部分辨析）；② supportedActions 为按当前态派生的可用性矩阵面（非状态字段）；③ docker_daemon_state 为每次探测整值刷新的判定值（D2 注记同构 firewall_backend）；④ 生命周期动作在途为一次性执行过程（P7）；⑤ 未装引导呈现为 state=unavailable 的降级视图。无审批/驳回环节（删除确认为交互层危险操作防护，非审批流——PRD 第六部分末注）；无状态历史表（操作追溯经 account-auth audit_log）。

#### 4.1.3 子流程设计

无（daemon 探测为进入模块的只读判定链路、生命周期动作为单动作即时执行、日志跟随为流式读取订阅——均无独立生命周期状态，domain-model.md 第四部分已声明）。

#### 4.1.4 接口设计

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /docker/daemon/status | GET | admin+readonly | 接管结论（状态/版本/不可用细节） |
| /docker/container/list | GET | admin+readonly | 容器投影清单（筛选 + Compose 归属 + 权威动作面） |
| /docker/container/logs | GET | admin+readonly | 容器日志尾部读取 |
| /docker/container/logs-follow | WS | admin+readonly | 容器日志实时跟随（流） |
| /docker/container/stats | GET | admin+readonly | 容器资源占用快照 |
| /docker/container/start | POST | admin | 启动（矩阵守卫） |
| /docker/container/stop | POST | admin | 停止（矩阵守卫） |
| /docker/container/restart | POST | admin | 重启（矩阵守卫） |
| /docker/container/pause | POST | admin | 暂停（矩阵守卫） |
| /docker/container/unpause | POST | admin | 恢复（矩阵守卫） |
| /docker/container/delete | POST | admin | 删除（确认制 + 仅 stopped 态） |
| /docker/container/create | POST | admin | 创建并启动容器（镜像缺失自动拉取；D-09） |
| /docker/volume/create | POST | admin | 创建命名卷（D-10） |
| /docker/network/create | POST | admin | 创建自定义网络（D-10） |
| /docker/compose/create | POST | admin | 新建 Compose 项目（YAML 校验+落盘+自动 up -d；D-11） |

**接口详情**（请求/响应/校验）：

- **GET /docker/daemon/status**：无参数。处理：即时探测（D-01，I-01 无缓存）。响应 data=`{state: docker_daemon_state, version: string|null, detail: string|null, generatedAt}`——unavailable 时前端据此渲染安装引导页（detail 区分未装/不可达），不调用业务面。审计：无（查看类）。
- **GET /docker/container/list**：query `{state?, composeProject?}`——state 多值逗号分隔（container_status 值域 + 过渡态原始值，任一非法 → DK_001），composeProject 等值匹配。处理：daemon 可用性前置（DK_002）→ 即时读取全量投影（I-01/I-02）→ 服务端筛选+排序（名称字典序，D-02）→ composeProject 标注 + supportedActions 计算。响应 data=`{items:[{containerId, name, image, state, composeProject, supportedActions}], total, daemon:{state, version}, generatedAt}`——daemon 摘要随行携带（页头呈现，宿主 S-45-01 结论）；空清单仅当 daemon 可用且无容器。审计：无（查看类）。
- **GET /docker/container/logs**：query `{containerId, tail?}`。校验：containerId 非空（DK_001）→ daemon 可用（DK_002）→ 存在性（DK_003，实况复核）→ tail 取值域（DK_001，缺省 500）。响应 data=`{lines: [string], generatedAt}`。审计：无（查看类）。
- **WS /docker/container/logs-follow**：握手 query `{containerId, tail?}`；握手校验：Cookie 会话（admin+readonly）+ Origin 同源（terminal D-09 同判）→ 存在性（不存在拒绝握手）。流协议：握手成功先回放尾部 tail 行（`{"type":"log","line"}` 逐帧）→ 随后流式推送新日志行 → 终止时发 `{"type":"end","reason"}` 并关闭（reason ∈ 常量表五值：容器删除 container_removed / 容器停止 container_stopped / daemon 失联 daemon_lost）；客户端无上行帧，断开即取消订阅。审计：无（查看类）。
- **GET /docker/container/stats**：query `{containerId}`。校验：DK_001/DK_002/DK_003。响应 data=`{cpuPercent, memUsageBytes, memLimitBytes, memPercent, generatedAt}`（单次快照，D-07）。审计：无（查看类）。
- **POST /docker/container/{start|stop|restart|pause|unpause}**：req `{containerId}`。校验顺序（R-14/I-04）：containerId 非空（DK_001）→ daemon 可用（DK_002）→ 存在性（DK_003，实况复核）→ **矩阵校验**（当前态不允许该动作 → DK_004，data=`{currentState, action, allowedActions}`；过渡态 allowedActions=[] 恒拒）→ SDK 执行（失败 DK_017，data.reason 透传）。成功 data=`{containerId, currentState}`（操作后即读实况）。审计：container_started/stopped/restarted/paused/unpaused（change；detail={containerId, name, action}；结果按执行实况——DK_004/017 为 failure，I-06）。
- **POST /docker/container/delete**：req `{containerId}`（**无 force 字段**，I-04/I-05）。校验顺序：DK_001 → DK_002 → DK_003 → 矩阵校验（仅 stopped 态开放；running/paused/过渡态 → DK_004，提示先停止）→ SDK 执行（失败 DK_017）。成功 data=`{deleted: true, containerId}`。删除确认交互在前端（危险操作防护，非服务端两段式——无面板自锁风险场景，区别于 firewall FW_005）。审计：container_deleted（change；detail={containerId, name, image}）。

- **POST /docker/container/create**：req `{name, image, ports?: [{hostPort, containerPort, protocol?}], env?: [{key, value}], mounts?: [{source, target, mode?}], restartPolicy?: "no"|"always"|"unless-stopped"|"on-failure", network?: string}`。校验顺序：daemon 可用（DK_002）→ 逐项格式校验（DK_019/020/021/022，R-14）→ 网络名存在于清单（不存在 DK_009）→ 容器名/端口占用 daemon 拒绝在创建时透传（DK_023）。执行：镜像本地缺失 → 自动拉取（600s，失败 DK_013 透传不创建）→ SDK ContainerCreate+Start（失败 DK_023，data.reason 透传）。成功 data=`{containerId, name, state}`（即读实况）。审计：container_created（change；detail={name, image}；不含环境变量值）。
- **POST /docker/volume/create**：req `{name, driver?: "local"}`。校验：daemon 可用（DK_002）→ 名称格式（DK_001）。执行 SDK VolumeCreate（重名/非法名称 daemon 拒绝 → DK_024，data.reason 透传）。成功 data=`{name, driver}`。审计：volume_created（change）。
- **POST /docker/network/create**：req `{name, driver?: "bridge", subnet?, gateway?}`。校验：daemon 可用（DK_002）→ 名称格式（DK_001）→ 子网/网关格式可选校验（DK_001）。执行 SDK NetworkCreate（重名/子网冲突 → DK_025，data.reason 透传）。成功 data=`{name, driver, subnet?}`。审计：network_created（change）。
- **POST /docker/compose/create**：req `{projectName, composeYaml, autoUp?: true}`。校验：daemon 可用（DK_002）→ 项目名白名单（DK_001）→ YAML 语法校验（失败 DK_026，data.reason 带行号）→ 落盘目录已存在（DK_027，data.projectName）。执行：归属标签注入（⚠️A22，labels 段 `dougong.app={projectName}`，写入前处理）→ 目录创建+原子写 0644 → autoUp 时 ComposeRunner up -d（项目级互斥 DK_018；up 失败 DK_016 透传，**已落盘的 compose.yml 保留**——项目可在列表重试 up，不回滚删除）。成功 data=`{projectName, workingDir, configFile, started}`。审计：compose_project_created（change；detail={projectName}；不含 compose 全文）。

**跨接口复用规则**：daemon 可用性前置统一由 DaemonGuard 中间件 + DaemonProbeService 承载（DK_002）；权威动作面计算统一走 ContainerProjector.supportedActions（list 响应与 DK_004 data 同源）；写操作审计统一走 `AuditService.Record`（D-09）；SDK 执行统一经 DockerGateway（I-03）；日志尾部/跟随统一走 LogStreamService（D-03）。

**幂等语义**：生命周期动作非幂等（矩阵不匹配 DK_004 拒绝，不静默成功）；delete 非幂等（不存在 DK_003，不静默成功）；全部读取与 WS 订阅无副作用（纯投影/流）；删除确认不记忆（每次独立确认，PRD N3）。

#### 4.1.5 领域事件与集成

无 MQ、无领域事件（单机单体，R-09）。「仓储契约」为零持久化替代——**进程内网关契约（非 SQL；SDK 唯一 daemon 交互出口）**：

```
DockerGateway {
  Ping() (DaemonInfo, error)                       // sock 探测+版本（DaemonInfo={State(docker_daemon_state), Version, Detail}，D-01）
  ListContainers() ([]ContainerFact, error)         // 容器实况（含 compose label 透传；I-01/I-02）
  GetContainerLogs(id string, tail int) ([]string, error)
  FollowContainerLogs(id string, tail int) (LogStream, error)  // 流订阅（LogStream=行 channel + 终止原因）
  GetContainerStats(id string) (StatsFact, error)   // 单次快照（D-07）
  ContainerAction(id string, action string) error   // start/stop/restart/pause/unpause（SDK 直达；矩阵校验在服务层）
  DeleteContainer(id string) error
}
约束：SDK 单操作 ≤60s；通道错误归一 DK_017（原文透传）；等价/不存在等 daemon 原生语义如实上抛（D-08）
```

关联域调用（出站）：`AuditService.Record`（account-auth OHS，操作审计 R-03；D-09 降级语义）；无其他出站。

### 4.2 image / volume / network（资源投影与删除）

#### 4.2.1 领域模型

**表结构组合**：无表（D-10 零持久化）。三类资源驻留进程内为读取时重建的投影集合；删除直达 daemon。

**资源投影条目字段清单**：

| 资源 | 字段 | 说明 |
|------|------|------|
| image | imageId / ref（名称:tag）/ sizeBytes | 镜像投影（imageId 不透明引用；引用情况不做前置推导标注，I-05） |
| volume | volumeName / mountpoint / sizeBytes | 卷投影（volumeName 原生即稳定标识） |
| network | networkId / name / driver / subnet | 网络投影（预定义网络如实呈现，删除时 DK_011）；subnet 恒 string，无 IPAM 子网时为空串 ""（非 null，gate2 I1 修复轮钉死），前端判空渲染占位「—」 |

**不变量**：

| 编号 | 不变量 | 违反处理 |
|------|--------|---------|
| I-05 | （沿用 4.1.1）删除约束权威：被引用（镜像）/被挂载（卷）/被接入（网络）时 daemon 拒绝并如实呈现（DK_006/008/010），无前置推导、无 force 参数；预定义网络 DK_011 | 删除端点校验 + 错误归一（D-08） |
| I-07 | 拉取外呼口径：pull 经 daemon 执行（面板自身零外呼不变，R-09——registry 外呼在 daemon 侧且为用户主动发起）；拉取为 admin 专属变更动作记审计 | PullService 编排（D-05） |
| I-08 | （Compose 并发互斥见 4.3.1）资源删除幂等口径：不存在 → DK_005/007/009（不静默成功）；读取无副作用 | 删除端点校验 |

**领域服务**：ResourceProjector（三类资源投影组装）、PullService（拉取编排：ref 校验→daemon 同步拉取→审计，D-05）、ResourceDeleteService（删除编排：存在性→daemon 删除→错误归一→审计）。

#### 4.2.2 状态模型

面板侧无自有状态机（有无即状态，PRD 第六部分辨析；与 firewall「规则有无即状态」同判）：对象存在性=daemon 侧记录的存在与删除；拉取在途为一次性执行过程（P7）。

#### 4.2.3 子流程设计

无（拉取为「提交→同步执行→终态反馈」单动作过程、删除为单动作即时执行——均无独立生命周期状态，domain-model.md 第四部分已声明）。

#### 4.2.4 接口设计

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /docker/image/list | GET | admin+readonly | 镜像投影清单 |
| /docker/image/pull | POST | admin | 拉取镜像（同步终态，D-05） |
| /docker/image/delete | POST | admin | 删除镜像（引用约束） |
| /docker/volume/list | GET | admin+readonly | 卷投影清单 |
| /docker/volume/delete | POST | admin | 删除卷（数据销毁确认） |
| /docker/network/list | GET | admin+readonly | 网络投影清单 |
| /docker/network/delete | POST | admin | 删除网络（预定义不可删） |

**接口详情**：

- **GET /docker/image/list**：无参数。处理：daemon 可用性前置（DK_002）→ 即时读取全量投影。响应 data=`{items:[{imageId, ref, sizeBytes}], total, generatedAt}`。审计：无。
- **POST /docker/image/pull**：req `{ref}`。校验顺序（R-14/I-07）：ref 镜像引用格式（`名称[:tag]`，DK_012）→ daemon 可用（DK_002）→ daemon 同步拉取（≤600s；失败 DK_013，data.reason 透传——tag 不存在/网络不通/限流）。成功 data=`{imageId, ref}`。审计：image_pulled（change；detail={ref}；DK_012/013 按结果如实）。
- **POST /docker/image/delete**：req `{imageId}`（无 force 字段，I-05）。校验顺序：DK_001 → DK_002 → 存在性（DK_005）→ daemon 删除（被引用 → DK_006；失败 DK_017）。成功 data=`{deleted: true, imageId}`。审计：image_deleted（change；detail={imageId, ref}）。
- **GET /docker/volume/list**：无参数。响应 data=`{items:[{volumeName, mountpoint, sizeBytes}], total, generatedAt}`。审计：无。
- **POST /docker/volume/delete**：req `{volumeName}`。校验顺序：DK_001 → DK_002 → 存在性（DK_007）→ daemon 删除（被挂载 → DK_008；失败 DK_017）。成功 data=`{deleted: true, volumeName}`。删除确认在前端（确认文案明示数据销毁不可恢复，PRD FR-13）。审计：volume_deleted（change；detail={volumeName}）。
- **GET /docker/network/list**：无参数。响应 data=`{items:[{networkId, name, driver, subnet}], total, generatedAt}`。审计：无。
- **POST /docker/network/delete**：req `{networkId}`。校验顺序：DK_001 → DK_002 → 存在性（DK_009）→ daemon 删除（被接入 → DK_010；预定义网络 → DK_011；失败 DK_017）。成功 data=`{deleted: true, networkId}`。审计：network_deleted（change；detail={networkId, name}）。

**幂等语义**：pull 幂等（同 ref 重复拉取按 daemon 层叠/覆盖语义如实反馈成功，PRD N3）；三类 delete 非幂等（不存在 DK_005/007/009 报错，不静默成功）；读取无副作用。

#### 4.2.5 领域事件与集成

无 MQ、无领域事件。网关契约（DockerGateway 扩展，R-08 唯一出口）：

```
DockerGateway {
  ListImages() ([]ImageFact, error) / PullImage(ref string) (ImageFact, error) / DeleteImage(id string) error
  ListVolumes() ([]VolumeFact, error) / DeleteVolume(name string) error
  ListNetworks() ([]NetworkFact, error) / DeleteNetwork(id string) error
}
约束：pull ≤600s、其余 ≤60s；in-use/预定义/不存在等 daemon 语义归类上抛（D-08：DK_006/008/010/011 与 NOT_FOUND 族）
```

关联域调用（出站）：`AuditService.Record`（R-03，D-09）；无其他出站。

### 4.3 compose_project（label 聚合识别与 CLI 编排）

#### 4.3.1 领域模型

**表结构组合**：无表（D-10 零持久化）。compose_project 为每次读取时按容器 label 重新聚合的派生投影（项目名聚合键）；操作直达 `docker compose` CLI。

**项目投影条目字段清单**：

| 字段 | 类型 | 说明 |
|------|------|------|
| project | string | 项目名（label `com.docker.compose.project` 聚合键，D-11 不透明引用） |
| workingDir | string | 工作目录（label `.working_dir` 透传，R-13） |
| configFile | string | 配置文件（label `.config_files` 透传） |
| containers | [{containerId, name, state}] | 项目容器清单及实况（对 container 的同域引用集合） |
| locatable | bool | 服务端定位判定（工作目录存在且配置文件可读；false → 操作/日志端点 DK_015，D-06） |

**不变量**：

| 编号 | 不变量 | 违反处理 |
|------|--------|---------|
| I-06 | 定位失败兜底：项目级操作（up/down/restart/logs/logs-follow）定位失败 → DK_015（data 携 label 记录路径）；**容器级操作不受影响**（项目内容器仍经 4.1 端点逐个操作，D1 口径-2 兜底通道） | 操作端点前置校验（D-06） |
| I-08 | Compose 并发互斥：同项目 up/down/restart 进行中再提交 → DK_018（进程内 per-project 串行队列）；不同项目并行不受影响 | ComposeRunner 互斥（D-06） |
| I-09 | CLI 纪律：全部 compose 调用经 ComposeRunner 参数数组执行（R-08/ADR-013 不走 SDK 编排），单命令 ≤300s；label 定位失败不执行 CLI | 结构保证（I-03 同源） |

**领域服务**：ComposeProjector（label 扫描聚合 + locatable 判定）、ComposeRunner（CLI 编排：定位→组装参数→串行执行→结果归一）、ComposeLogStreamService（项目聚合日志尾部/跟随，复用 LogStreamService 通道形态）。

#### 4.3.2 状态模型

面板侧无自有状态机（项目运行态=其容器组实况的读取时汇总呈现，PRD 第六部分辨析）；locatable 为识别时判定值（整值刷新，非状态机）；up/down/restart 在途为一次性执行过程（P7）；CLI 失败无回滚语义（Compose 原子性由其自身保证，PRD N2）。

#### 4.3.3 子流程设计

无（label 识别为读取时聚合的机制链路、Compose 操作为单动作 CLI 执行——均无独立生命周期状态，domain-model.md 第四部分已声明）。

#### 4.3.4 接口设计

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /docker/compose/list | GET | admin+readonly | 项目清单（label 聚合 + locatable 判定） |
| /docker/compose/up | POST | admin | 项目 up（CLI 参数数组） |
| /docker/compose/down | POST | admin | 项目 down |
| /docker/compose/restart | POST | admin | 项目 restart |
| /docker/compose/logs | GET | admin+readonly | 项目聚合日志尾部读取 |
| /docker/compose/logs-follow | WS | admin+readonly | 项目聚合日志实时跟随（流） |

**接口详情**：

- **GET /docker/compose/list**：无参数。处理：daemon 可用性前置（DK_002）→ 容器清单 label 扫描聚合（无 label 容器不归属任何项目，R-13）→ locatable 判定（D-06）。响应 data=`{items:[{project, workingDir, configFile, containers:[{containerId, name, state}], locatable}], total, generatedAt}`。审计：无。
- **POST /docker/compose/{up|down|restart}**：req `{project}`。校验顺序（R-14/I-06/I-08）：project 非空（DK_001）→ daemon 可用（DK_002）→ 项目存在（DK_014，实况复核）→ 定位（locatable=false → DK_015，data=`{workingDir, configFile, missing}`）→ 并发互斥（进行中 → DK_018）→ CLI 执行（`docker compose --project-name <p> --project-directory <dir> -f <file> <verb>` 参数数组，≤300s；失败 DK_016，data.reason 透传 stderr）。成功 data=`{project, verb, done: true}`。审计：compose_up/compose_down/compose_restarted（change；detail={project, workingDir, verb}；DK_014/015/016/018 按结果如实，I-06 同口径）。**定位失败时兜底通道不变**：项目内容器仍可经 /docker/container/* 逐个操作（I-06）。
- **GET /docker/compose/logs**：query `{project, tail?}`。校验顺序：DK_001 → DK_002 → DK_014 → DK_015 → tail 取值域（DK_001，缺省 500）。响应 data=`{lines: [string], generatedAt}`（项目全部容器聚合日志）。审计：无（查看类）。
- **WS /docker/compose/logs-follow**：握手 query `{project, tail?}`；握手校验同 container/logs-follow（Cookie 会话 admin+readonly + Origin 同源 + 项目存在/可定位，失败拒绝握手）。流协议同 D-03（回放 tail → 流式推送 → end 帧关闭；reason ∈ project_gone / locate_failed / daemon_lost）。审计：无（查看类）。

**幂等语义**：up/down/restart 非幂等（重复提交按 CLI 执行结果如实反馈；同项目进行中 → DK_018）；logs/logs-follow 读取无副作用。

#### 4.3.5 领域事件与集成

无 MQ、无领域事件。网关契约（CLI 唯一出口，R-08/ADR-013）：

```
ComposeRunner {
  ListProjectFacts() ([]ComposeProjectFact, error)   // 经 DockerGateway.ListContainers 的 label 聚合（R-13）
  Run(project ComposeProjectFact, verb string) error // docker compose --project-name/--project-directory/-f/<verb>（参数数组；per-project 串行互斥 I-08；≤300s）
  Logs(project ComposeProjectFact, tail int) ([]string, error)
  FollowLogs(project ComposeProjectFact, tail int) (LogStream, error)
}
约束：verb ∈ {up, down, restart}（logs 走 Logs/FollowLogs 只读通道）；定位失败不执行 CLI（I-06）；
      stderr 如实上抛由服务层归一 DK_016；不使用 --reload 类全局副作用参数（firewall D-08 同哲学）
```

关联域调用（出站）：`AuditService.Record`（R-03，D-09）；`DockerGateway.ListContainers`（同进程内组合，label 数据源）；无其他出站。

## 5. 模块清单汇总

### 5.1 对外服务清单（HTTP/WS 接口）

| 服务（接口） | 消费方 | 契约要点 |
|-------------|--------|---------|
| GET /docker/daemon/status | 前端容器页（唯一消费方） | 即时探测结论 + 未装/不可达区分（D-01） |
| GET /docker/container/list | 前端容器页 | 全量投影 + 筛选 + Compose 归属 + 权威动作面（D-02） |
| GET /docker/container/logs + WS /docker/container/logs-follow | 前端容器页 | 尾部 500 行 + 实时跟随（D-03 双通道） |
| GET /docker/container/stats | 前端容器页 | CPU/内存单次快照（D-07） |
| POST /docker/container/{start,stop,restart,pause,unpause,delete} | 前端容器页（admin） | 矩阵守卫 DK_004 + 权威动作面（D-04）；delete 无 force（I-04/I-05） |
| GET /docker/image/list + POST /docker/image/{pull,delete} | 前端镜像页 | 拉取同步终态（D-05）；引用约束 DK_006（D-08） |
| GET /docker/volume/list + POST /docker/volume/delete | 前端卷页 | 数据销毁确认；被挂载 DK_008 |
| GET /docker/network/list + POST /docker/network/delete | 前端网络页 | 预定义网络 DK_011；被接入 DK_010 |
| GET /docker/compose/list + POST /docker/compose/{up,down,restart} | 前端 Compose 页 | label 聚合 + locatable 判定 + 并发互斥（D-06）；定位失败兜底（I-06） |
| GET /docker/compose/logs + WS /docker/compose/logs-follow | 前端 Compose 页 | 项目聚合日志双通道（D-03 同口径） |
| 中间件 AuthRequired / CsrfGuard / RoleRequired / DaemonGuard | （复用+本模块） | account-auth 提供（DaemonGuard 为本模块业务前置，顺序见 3.1） |
| AuditService.Record | （出站调用） | 操作审计写入（R-03，D-09） |

本模块无对外 OHS 服务契约——服务消费终模块（1.5：无其他业务模块依赖；terminal 二期为预期方向）。

### 5.2 SQL 清单

**无**——本模块零持久化（D-10）：五类对象均为 daemon/label 实况投影，无任何 SQLite 表读写（domain-model.md 存储映射说明「docker-mgmt→无表」）；操作审计落库由 account-auth 的 AuditRepository 承载（非本模块 SQL）。

### 5.3 DDL

**无**——零持久化（同 5.2；本模块不新增任何表，面板库 schema 无变更）。

## 6. 更新记录

### 6.1 变更记录

| 版本 | 日期 | 变更类型 | 变更摘要 | 目标节号 |
|------|------|---------|---------|---------|
| v1.0 | 2026-11-14 | initial | 首次生成：24 个端点（22 HTTP：GET /docker/{daemon/status, container/{list,logs,stats}, image/list, volume/list, network/list, compose/{list,logs}} 9 查询 + POST /docker/{container/{start,stop,restart,pause,unpause,delete}, image/{pull,delete}, volume/delete, network/delete, compose/{up,down,restart}} 13 写；2 WS：/docker/{container,compose}/logs-follow——读=admin+readonly / 写=admin+CSRF）+ 2 个进程内网关契约（DockerGateway SDK 通道 / ComposeRunner CLI 参数数组通道；零持久化：无表/无 DDL/无 SQL）+ 18 错误码（DK_001~018）+ 复用 account-auth 码族（AUTH_001/002/003/008）+ 6 枚举引用（三个容器枚举 D2 已定稿，无新增；compose_action 池行依据补注承载形态）+ 13 个审计动作常量 + 11 项设计决策（D-01 即时探测+细节透传、D-02 全量筛选+权威动作面、D-03 日志双通道+WS 帧协议、D-04 矩阵服务端守卫、D-05 拉取同步终态、D-06 Compose 动词端点+locatable+并发互斥、D-07 单次快照、D-08 daemon 拒绝归一、D-09 审计非同事务、D-10 零持久化、D-11 不透明引用寻址）；关闭 prd-docker-mgmt.md 待对齐-1~-8 全部八项；无旧版对比 | — |
| v1.2 | 2026-11-23 | 增量 | 宝塔式重组 D3（ADR-033）：新增 /docker/app/{list,create,service,logs,delete,site} 与 /docker/compose/delete 共 7 个接口（24→31 端点）；DK_028~032；compose/list 条目增 panelManaged 并纳入零容器项目；dougong.app 标签仅「应用」创建通道注入（修订 D-11）；D-13~D-17；审计动作 +5 | 文末「增量 v1.2」节 A~E |

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
| v1.1 | 2026-11-22 | 创建类能力增量 D1（BA/AA/FL v1.1 级联，d0-clarification-docker-mgmt-create-2026-11-21.md 五项拍板）：①interface_added ×4：POST /docker/container/create（D-09：逐项校验 DK_019~022+镜像缺失自动拉取复用 pull 通道+SDK Create+Start+创建后即读实况）、POST /docker/volume/create（D-10：名称/驱动，daemon 拒绝透传 DK_024）、POST /docker/network/create（D-10：名称/驱动/子网网关可选，透传 DK_025）、POST /docker/compose/create（D-11：YAML 校验 DK_026 带行号+项目名白名单+落盘根 /opt/dougong-compose/ 防逃逸+目录已存在 DK_027+autoUp 复用项目级互斥+up 失败不回滚落盘+⚠️A22 归属标签注入）；端点 22→26（GET 9/POST 15/WS 2）；②enum_value_added/rule_added：DK_019~027 九码接力（D-12）；D-09~D-12 四项设计决策；审计动作常量 +4（container_created/volume_created/network_created/compose_project_created）；③零台账投影哲学不变（新建对象即时进实况清单）；⚠️A22 归属标签预埋供 app-mgmt 容器化应用（跨模块消费方登记见 api-app-mgmt.md v1.1）；④关闭 prd v1.1 待对齐（校验器选型 go-yaml/落盘根常量/容器创建校验细则均 D-09~D-11 落定）；无旧版 DDL 变更（零持久化不变） |

## 增量 v1.2（2026-11-23）：宝塔式重组 D3——面板应用与 Compose 生命周期

> 依据 baota-restructure-design-2026-11-23.md 第五、六、八章。本节是权威增量。

### A. 新增接口（7 个；模块端点 24 → 31）

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /docker/app/list | GET | admin+readonly | 面板应用清单：落盘根扫描（编排文件解析）与容器实况合并；零持久化 |
| /docker/app/create | POST | admin | 创建应用：名称 / 端口前置校验 → 渲染单服务编排 → 落盘 /opt/dougong-compose/{名称}/docker-compose.yml → compose up -d |
| /docker/app/service | POST | admin | 应用启停重启：start=compose up -d、stop=compose stop、restart=compose restart |
| /docker/app/logs | GET | admin+readonly | 应用日志（compose logs 通道）；query name、tail（缺省 500，取值域 1~2000） |
| /docker/app/delete | POST | admin | 删除应用：compose down + 删除项目目录 |
| /docker/app/site | POST | admin | 应用建站：为应用创建反向代理站点，上游 127.0.0.1:{主机端口}（消费 site-mgmt 建站能力） |
| /docker/compose/delete | POST | admin | 删除 Compose 项目：compose down + 删除面板落盘根下的项目目录 |

### B. 请求与响应

| 接口 | 请求 | 响应 data |
|------|------|-----------|
| app/create | name（必填，匹配 ^[a-z0-9][a-z0-9-]{0,63}$）、image（必填）、hostPort（必填，1~65535）、containerPort（必填，1~65535）、env?（[{key, value}]）、mounts?（[{source, target}]）、restartPolicy?（no / always / unless-stopped / on-failure，缺省 unless-stopped） | { name, workingDir, configFile, hostPort, state } |
| app/list | 无 | { items: [{ name, state, containers: [{containerId, name, state}], image, hostPort, containerPort, workingDir, configFile, siteLinked }], total, generatedAt }；state 取 docker_app_state（running / stopped / partial） |
| app/service | name（必填）、op（必填，start / stop / restart） | { name, op } |
| app/logs | query name（必填）、tail? | { project, lines }（沿 compose/logs 形状，project=应用名） |
| app/delete | name（必填） | { name, deletedDir: true } |
| app/site | name（必填）、domains（必填，非空数组） | 沿 app-mgmt 建站联动的响应形状（siteId 等） |
| compose/delete | project（必填） | { project, deletedDir: boolean }——目录位于面板落盘根下为 true；面板外项目只停止、不删目录，为 false |

**compose/list（修订，响应形状向后兼容）**：条目增 panelManaged: boolean（项目目录位于面板落盘根下）；落盘存在但无容器的项目并入清单——containers 为空数组、workingDir 与 configFile 非空、locatable 为 true（可直接启动）。

**应用编排模板（面板生成，单服务）**：服务名 app；ports 绑定 127.0.0.1:{主机端口}:{容器端口}；environment / volumes 按请求；restart 按请求；labels 含 dougong.app={名称}。

### C. 新增错误码

| code | 标识 | HTTP 语义 | 说明 |
|------|------|---------|------|
| DK_028 | APP_NOT_FOUND | 不存在 | 应用不存在（可能已在面板外删除） |
| DK_029 | APP_NAME_INVALID | 参数错误 | 应用名格式非法（小写字母、数字、连字符，以字母或数字开头） |
| DK_030 | APP_OP_FAILED | 操作失败 | 应用操作失败（创建 / 启停 / 删除；data.reason 透传） |
| DK_031 | APP_PORT_IN_USE | 前置条件不满足 | 主机端口已被占用 |
| DK_032 | COMPOSE_DELETE_FAILED | 操作失败 | Compose 项目删除失败（data.reason 透传） |

应用重名复用 DK_027（项目已存在）。

### D. 审计动作（+5）

app_created / app_service_op / app_deleted / app_site_linked / compose_project_deleted（detail 不含环境变量值，脱敏口径沿 app-mgmt ⚠️A9）。

### E. 设计决策（增补）

| 编号 | 决策 | 理由 |
|------|------|------|
| D-13 | 面板应用零持久化：清单由落盘根扫描与容器实况合并而成，不建表 | 沿 D-10；事实源在文件系统与 daemon |
| D-14 | 应用的主机端口只绑 127.0.0.1，经建站对外暴露 | 与 Python / Node 项目「只监听本机、经建站对外」口径一致；直接对外暴露走创建容器或手写 Compose |
| D-15 | dougong.app 归属标签仅由「应用」创建通道注入；「新建 Compose 项目」不再注入（修订 D-11） | 标签语义收窄为「面板应用」，与手写 Compose 项目可区分 |
| D-16 | compose/list 并入落盘根目录扫描；compose/delete 仅删除落盘根下的项目目录 | 修复零容器项目不可见；不触碰面板管理范围外的目录 |
| D-17 | 应用「停止」用 compose stop（保留容器），「删除」用 compose down 并删目录 | 停止后可快速再启动；删除才做清理。ComposeRunner 动词白名单相应新增 stop |
