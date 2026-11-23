# 斗拱面板 应用管理（app-mgmt）API 设计

| 属性 | 值 |
|------|-----|
| 版本 | v1.2 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-23（v1.2 宝塔式重组 D3：create 增 runtime 与 Node 启动方式、容器形态下线（APP_014）；list 增运行时与 Node 就绪度；迁移 018） |
| 输入 | prd-app-mgmt.md v1.0（FR-01~12；完备性检查通过：G 5/5、T 适用 8/8、N 4/4）、domain-model.md v1.27（app+app_task 双聚合根+启动方式值对象）、D1 三文档（BA/AA/FL v1.0，⚠️A1~A12+安全四道防线 BA 2.4/AA 附录C）、api-env-mgmt.md v1.4（Python 环境供给 FR-09+就绪度读契约）、api-site-mgmt.md v2.8（ConfigWritePipeline OHS D-21）、prd-env-mgmt.md 待对齐承接（能力位字段/候选包名） |
| 接口规约 | 栈规约 backend-02-api-conventions.md（go-vue3）；认证=HttpOnly Cookie 会话 + CSRF token（R-02，复用 account-auth）；鉴权=RBAC 两角色边界矩阵（ADR-018；日志 admin 专属 ⚠️A8）；外部命令一律字面量参数数组（R-08） |

## 1. 模块设计概述

### 1.1 业务背景及目标

应用管理是面板的 Python 应用托管模块（2026-11-20 用户拍板，d0-clarification-app-mgmt-2026-11-20.md）：把「装运行时 → 建 venv → 装依赖 → 写 systemd 服务 → 建反代站点 → 看日志」六步手工链收拢为应用生命周期管理——创建应用（目录校验 → venv → 依赖安装 → 生成服务单元，异步任务 ⚠️A3）、运行管理（启停/自启/参数/日志）、对外暴露（建站联动 ⚠️A7）、删除（保守保全 ⚠️A5）。Python 运行环境本体由 env-mgmt 供给（v1.4 FR-09 第九环境 +python，⚠️A18），本模块经环境实况读契约消费依赖就绪度（python3/venv/pip 能力位），缺失分层如实拒绝并软引导（⚠️A2，不代装）。与既有模块分界：环境软件包层归 env-mgmt、通用单元层归 service-mgmt（双入口并存 ⚠️A12）、站点层归 site-mgmt、文件层归 file-manager。服务对象：admin 执行全部操作与应用日志查看（⚠️A8）；readonly 查看应用清单与任务状态输出（ADR-018）。

### 1.2 边界说明

| 项 | 说明 |
|----|------|
| 本模块做 | 应用全景（档案+实况+端口+站点提示）、创建/依赖安装/删除（异步任务+安全写入）、任务台账观测、服务操作（应用语义入口，双入口并存）、参数编辑（.env/启动参数）、应用日志（admin 专属）、建站联动 |
| 本模块不做（接口边界） | ① 容器路线应用部署——docker-mgmt（本轮容器侧仅排查零开发，D0 Q1）；② Python 多版本——OUT（⚠️A11）；③ 应用市场/代码上传——OUT（⚠️A1）；④ 任务取消——OUT（⚠️A3）；⑤ 运行环境代装——软引导 env-mgmt（⚠️A2）；⑥ 数据库代建——database-mgmt 既有能力；⑦ 域名/HTTPS/证书本体——site-mgmt/panel-core（本模块仅消费建站能力）；⑧ .env 内容密文托管——OUT-14（用户自有配置，文件为唯一事实源）；⑨ 认证/鉴权/审计本体——复用 account-auth；⑩ 页面外壳与导航——归 panel-core |
| 与存储的关系 | SQLite 单文件（与全模块同库）；app + app_task 双表（5.3）；实况（服务态/就绪度/端口监听）零台账实时探测投影；.env/unit 文件为外部事实源不入库 |

### 1.3 设计决策

| # | 决策 | 理由 |
|---|------|------|
| D-01 | **实时探测投影**：app/list 每请求即时探测逐应用实况（服务运行态/自启态经 SysdGateway 单元查询、venv 存在性文件判定、Python 就绪度经 env-mgmt 读契约、依赖安装态 venv 内包查询、端口监听本机查询）与在途任务标记，无缓存层，响应带 generatedAt；探测失败字段 null 如实（不虚构运行态）；探测失败仅归条目级 null（非全局拒绝——与 env-mgmt D-01 通道级/条目级三态同哲学，本模块无通道级概念） | site-mgmt/env-mgmt D-01「投影真实性」同口径；systemd/文件系统/env-mgmt 实况/本机网络栈为事实源 |
| D-02 | **档案+任务双聚合落 SQLite**（domain-model v1.27）：app 单表（应用名唯一 UNIQUE 约束承载 I-02）+ app_task 单表（四态任务台账，与 env_install_task 同构）；实况投影零台账；.env/unit 文件为外部事实源（面板编辑器直读写不入库 OUT-14） | prd FR-01/FR-03/FR-06；创建/删除/依赖安装的执行事实与档案无法从外部完整推导必须面板自持 |
| D-03 | **创建前置校验链**（⚠️A1/A2/A4）：create 同步校验逐项如实——应用名格式（APP_001，`^[a-z0-9][a-z0-9-]{0,31}$`）→ 唯一性（APP_002，UNIQUE+预查）→ 目录存在（APP_003，EvalSymlinks 规范化+绝对路径约束，防逃逸沿 file-manager R-04 同构）→ 入口文件存在（APP_003）→ Python 就绪度（APP_004，data.missing 分层=[python\|venv\|pip]）→ 端口冲突（APP_005，本机监听查询）→ 同应用在途互斥（APP_006，新应用无历史任务故仅档案名冲突域）；校验全过 → INSERT app_task(pending) 同事务发起审计 → 返回 taskId | prd FR-03①/N2-1~4；服务端权威校验（R-14）不信任前端；软链解析防目录逃逸 |
| D-04 | **异步任务与同应用互斥**（⚠️A3）：create/deps/delete 同步校验 → INSERT app_task(status=pending) 与发起审计同事务 → 立即返回 taskId；worker goroutine（wg 编排，创建即触发）消费：running 落库 → 执行链 → 终态落库+终态审计；互斥=创建事务内判定同 app_id 存在 pending/running 即拒（APP_006，data.taskId；SQLite 单连接写串行保证原子）；服务操作（start/stop/restart/enable/disable）为瞬时命令不占任务通道（D-10）；**重启恢复**：启动扫描 running 任务标 failed（error=面板重启中断），不续跑（venv/pip 中断态不安全，沿 env-mgmt 同判）；任务取消不做（OUT-4） | prd FR-03②/FR-06/N2-9；与 env_install_task 互斥粒度差异：本模块按 app_id（多应用并存各自独立互斥） |
| D-05 | **创建执行链**（worker）：CreateVenv（`python3 -m venv .venv` 于应用目录，R-08 参数数组）→ 依赖安装（存在 requirements.txt 则 `.venv/bin/pip install -r requirements.txt`；无清单跳过该步如实记输出）→ 服务单元生成（D-06）→ 可选立即启动（create 请求 startNow 布尔，经 SysdGateway start）；任一步失败=任务 failed（error 含 data.reason，输出尾部定格）；**创建失败不残留半档案**——档案登记仅在任务 succeeded 终态同事务执行（I-C02；失败时已建 venv 保留〔重试/重建可复用，删除任务或勾选删目录时清理〕）；pip 安装超时 PY_OP_TIMEOUT=600s（沿 PKG_OP_TIMEOUT 同量级）单命令超时 | prd FR-03③；无 requirements.txt 为合法形态（⚠️A11 纯入口应用）；半档案不残留保证列表真实性 |
| D-06 | **服务单元生成与安全写入**（⚠️A4/A6）：单元名 `dougong-app-{name}.service`（命名前缀区分用户自建单元，service-mgmt 清单如实可见 ⚠️A12）；单元内容编译期模板渲染（User=运行用户候选默认面板运行用户/WorkingDirectory=应用目录/ExecStart=启动命令〔preset_fastapi=`.venv/bin/uvicorn {module}:{app} --host 127.0.0.1 --port {port}` 参数化模板；custom=用户自定义命令经 venv 内相对路径白名单校验 APP_012〕/EnvironmentFile={dir}/.env/Restart=on-failure）；写入经安全管线：备份原文件 → 原子写 → `systemd-analyze verify` 校验 → 失败回滚原字节（沿 site-mgmt R-06 四步同构）；单元落位 /etc/systemd/system/（编译期常量）；写入后 `systemctl daemon-reload`（参数数组） | prd FR-03③/FR-08③；⚠️A6 拍板；白名单校验=仅接受 `.venv/bin/` 前缀可执行相对路径+参数无 shell 元字符（`;|&$`` 等，APP_012 拒绝） |
| D-07 | **删除执行链**（⚠️A5）：delete 请求携带 purgeDir 布尔（默认 false=保留代码目录）；强确认交互归前端（NModal 明示删除范围知情提示）；执行：stop（幂等，已停止无副作用）→ disable（自启撤除）→ 单元文件移除（备份保留回滚能力，移除后 daemon-reload）→ **purgeDir=true 时删应用目录**（含 .venv；rm 参数数组，目录路径过 D-03 同款规范化校验防逃逸；默认 false 绝不删代码）→ 档案删除；**关联站点不自动删除**（呈现提示归前端；本模块不持站点引用，无站点撤除动作——FR-10 说明）；执行链对「服务/单元已不存在」保守继续（跳过该步如实记输出，N2-10）；档案删除与终态同事务 | prd FR-05；绝不静默删代码（⚠️A5 底线）；purgeDir 为显式勾选载体 |
| D-08 | **实况探测与就绪度读契约**（⚠️A2）：Python 就绪度经 env-mgmt 环境实况读契约（进程内，单向）——`EnvReadySource.PythonReadiness() → {Installed bool, Version string, VenvSupport bool, PipAvailable bool}`（env-mgmt v1.4 扩展：venv/pip 能力位，探测口径 python3 --version + venv 模块判定 + pip 可执行判定；契约扩展字段登记归 api-env-mgmt v1.4）；缺失分层呈现（missing 集合：python/venv/pip）；venv 存在性=应用目录 .venv/ 判定；依赖安装态=`.venv/bin/pip list` 计数（只读查询）；端口监听=本机监听表查询（复用 netlisten 类适配器资产口径） | prd FR-02/FR-11；pr d-app-mgmt 待对齐-3 落定：扩展字段=venvSupport/pipAvailable 布尔（本节为权威定义）；就绪度失败如实 null |
| D-09 | **参数编辑安全写回**（⚠️A6/A10）：GET /app/app/env 读 .env 现文件内容（行数组原样呈现——用户自有配置 admin 可见，OUT-14 不入库不加密；文件不存在=空数组如实）；POST /app/app/params 保存 .env 整文件替换（先读取现文件比对呈现差异提示归前端；服务端保存前重读做基线比对防覆盖外部修改——基线不一致返回 APP_012 data.reason=文件已变更请刷新，乐观锁语义）+ 启动参数变更触发单元文件更新（D-06 安全管线）；保存后提示重启生效（不自动重启，FR-07 执行）；行解析支持 KEY=VALUE/注释/#/export 前缀（解析呈现用，保存整文件替换）；审计 detail 不含参数值（⚠️A9） | prd FR-08/待对齐-7 落定：整文件替换+基线比对；.env 为用户自有配置零密钥语义（domain-model 启动方式值对象注记） |
| D-10 | **服务操作瞬时通道**（⚠️A12）：start/stop/restart/enable/disable 经 SysdGateway（复用 service-mgmt 适配器，单元名=档案 unit_name，R-14 请求仅 appId 不开放任意单元名）；等待终态语义与 service-mgmt 一致；与 service-mgmt 通用入口双入口并存同结果（同一 systemctl 通道）；操作对象限定本模块档案单元（APP_007 前置）；瞬时命令不占任务通道（与在途变更任务并发由 systemctl 语义兜底，prd 待对齐-6 落定） | prd FR-07；档案寻址防任意单元操作越界面 |
| D-11 | **应用日志**（⚠️A8）：GET /app/log 经日志网关查询本应用单元日志尾部（`journalctl -u {unit} -n {lines} --no-pager` 参数数组）；档位 lines ∈ {200,500,1000}（沿容器日志先例）；**admin 专属**（⚠️A8：readonly 请求全局 RoleRequired 拒绝 + 前端入口不渲染——沿 database-mgmt key 数据内容先例）；只读呈现不提供清理（日志轮转归系统 journald 机制）；LOG_QUERY_TIMEOUT=10s | prd FR-09/待对齐-4 部分落定：档位/超时定稿 |
| D-12 | **建站联动 OHS 消费**（⚠️A7）：POST /app/site/link 消费 site-mgmt 建站能力（进程内单向 OHS）——`SiteLinkGateway.CreateReverseProxySite(ctx, req) → {siteRef, err}`（reverse_proxy 类型站点，上游 127.0.0.1:{应用端口}，可选域名；经 site-mgmt 安全写入管线 R-06 落盘）；req `{appId, domain?}`；域名/HTTPS/证书本体归 site-mgmt/panel-core 既有能力；**本模块不持久化站点引用**（站点存在性经 site-mgmt 清单联动查询呈现——值快照口径，domain-model v1.27 关联领域对象零项）；失败透传 site-mgmt 错误语义（APP_011 data.reason）；重复建站（同名站点已存在）返回 site-mgmt 语义错误如实引导站点管理；契约签名与 api-site-mgmt.md §5.1 提供方登记为跨模块待同步项（归 site-mgmt 联动轮，本轮消费方定义） | prd FR-10；⚠️A7 拍板；待对齐-2 落定：进程内 OHS 沿 db-tools ConfigApply 消费先例 |
| D-13 | **失败重试语义**（⚠️A3）：POST /app/task/retry 仅对 failed 且 action ∈ {deps_install, create} 的任务可重试——create 重试=重新执行依赖安装类步骤（venv 已存在则跳过创建步骤，幂等）；deps_install 重试=重新执行 pip 安装；delete 失败不可重试（保守——删除链中断态需人工核实，重新发起删除=新任务）；重试=复用原任务行（状态回 running，沿「重新发起=新任务」的例外显式声明：依赖安装类步骤天然幂等，原任务重试不产生台账膨胀） | prd FR-06/FR-04；与 env-mgmt「重新发起=新任务」的差异显式登记（依赖安装幂等性承载重试语义） |
| D-14 | 混合模式（MIXED）声明：全部对象实际为自管（无外部数据同步源；应用为用户自有代码面板不分发），不设 manage_mode 字段，权限管控按角色区分（ADR-018） | 分册 Step 2 默认混合模式；本模块无托管侧 |
| D-15 | **任务台账全量保留**：不建清理任务（低频操作量级小）；任务列表分页（默认 50 上限 100，沿 env_install_task 公共分页约定） | prd N1；沿 env-mgmt D-11 |

### 1.4 领域对象总览

| 聚合根 | 域内对象/值对象 | 子流程 | 关联领域对象 |
|--------|---------------|--------|-------------|
| app（应用档案）、app_task（应用变更任务，四态状态机） | 启动方式（值对象，内嵌 app：start_mode+启动命令）；应用条目呈现（=档案+实况投影合并，无独立实体） | 无（任务状态机为聚合根本体，与 backup_run/env_install_task 同构） | 无（不持久化站点引用——建站联动值快照口径；消费均进程内契约或软引导，domain-model v1.27） |

无领域事件与 MQ（单机单体；审计为进程内 OHS 出站调用）。

**功能类型判定**（分册 Step 1）：聚合根 app/app_task 存在、由本模块管理且有写操作、状态为**数据生命周期**（档案存在性+四态任务状态机 ⚠️A3，非审批流转）→ **主数据类子形态**（与 env-mgmt「单据型聚合根+投影实体+工具类服务动作集合」混合形态同构，差异在双聚合根）；据此裁剪：第 2 章必填（双聚合根跨聚合编排）、4.2 按 app_task_status 状态机填写、第 5 章必填（有 DDL）。

### 1.5 上下文映射

| 上下文 | 关系 | 机制 |
|--------|------|------|
| account-auth | 遵奉者 + OHS 消费 | 全部接口位于 Cookie 会话后（R-02，AuthRequired）；应用列表/任务两角色同等、全部变更与日志 admin 专属（RoleRequired + AUTH_008 ⚠️A8）；写接口 CSRF（CsrfGuard）；审计写入 AuditService.Record（R-03，发起/终态/服务操作/参数/建站联动） |
| panel-core | 消费其 HTTP 骨架 | `/app` 路由组挂载；外壳与菜单归 panel-core（B 批增量登记菜单「应用管理」） |
| env-mgmt | OHS 消费（进程内读契约，单向） | Python 就绪度读契约 `EnvReadySource.PythonReadiness()`（D-08；venv/pip 能力位 v1.4 扩展，api-env-mgmt v1.4 登记）；Python 环境缺失软引导到环境管理安装页（无跨模块 API 硬依赖） |
| site-mgmt | OHS 消费（进程内，单向） | 建站联动 `SiteLinkGateway.CreateReverseProxySite`（D-12；reverse_proxy 站点，经安全写入管线 R-06）；契约提供方登记为跨模块待同步项（api-site-mgmt.md §5.1 归 site-mgmt 联动轮） |
| service-mgmt | 共享适配层 | SysdGateway systemd 网关实现共享（进程内适配器，非模块间调用）；本模块服务操作限定档案单元；应用单元在其服务清单如实可见（双入口并存 ⚠️A12） |
| file-manager | 被消费（软引导） | 创建校验目录/入口缺失时呈现「经文件管理器放置代码」指引（PageSpec 层，无跨模块 API 硬依赖） |
| python3/pip、systemd、journald（本机系统组件） | 数据源与动作通道 | AppPyGateway/AppUnitWriter/LogGateway 承载（R-08 参数数组）；本机资源非外部业务系统对接，无 ACL 必要；pip 索引为第三方依赖源（用户主动 R-09 例外，与 ACME 同判） |

## 2. 跨聚合根设计

### 2.1 聚合根关系图

```
app（应用档案）
  │ 1:N（按 app_id 归组；同域聚合引用非父子隶属——沿 backup_job/backup_run、db_tool/tool_deploy_task 先例）
  └── app_task（应用变更任务，四态状态机）
        └── 任务终态驱动 app 档案登记/删除（跨聚合同事务协作）
```

### 2.2 跨聚合根不变量

| 编号 | 不变量 | 违反处理 |
|------|--------|---------|
| I-C01 | **任务终态与档案同事务**：create 任务 succeeded ↔ app 档案 INSERT 同事务（两半不可分——无档案的 succeeded 任务/无任务的档案均不出现）；delete 任务 succeeded ↔ app 档案 DELETE 同事务；deps_install 任务终态不改档案（依赖态为实况投影） | worker Finish 扩展（事务内档案操作，SQL #8/#9） |
| I-C02 | **创建失败不残留半档案**：create 任务 failed 时 app 表无该应用行（venv 等已建产物保留供重试/重建，目录清理归 delete 或用户手动） | D-05；档案登记仅在 succeeded 终态 |
| I-C03 | **互斥约束跨聚合生效**：同 app_id 至多一个 pending/running app_task（创建事务内判定 APP_006）；档案存在的应用才可发起 deps/delete（APP_007） | 创建事务内判定（SQLite 单写者串行） |
| I-C04 | **删除范围显式性**：delete 任务执行链中目录删除仅当请求 purgeDir=true（D-07）；档案删除与终态同事务（I-C01）——purge 失败不阻断档案撤除（目录残留如实记输出，用户可手动清理） | D-07 执行链 |

### 2.3 跨聚合根编排

创建/依赖安装/删除均经 TaskWorker 统一编排：同步校验（app 侧前置）→ INSERT app_task（pending，同事务发起审计）→ worker 异步执行链（running 落库 → 步骤执行 → 终态+档案协作+审计同事务）；服务操作/参数编辑/日志/建站联动不经任务通道（瞬时动作或同步动作，D-10/D-09/D-12）。

## 3. 应用/基础设施层设计

### 3.1 接口规范

| 项 | 规范 |
|----|------|
| 路径格式 | `/app/{resource}/{action}` 三段式；资源单数（app/task/service/log/site） |
| HTTP 方法 | 读=GET，写=POST（面板内部契约） |
| 响应格式 | `{"code": 0, "message": "ok", "data": {...}}`（全项目同一契约） |
| 时间格式 | ISO-8601 UTC（公共约定） |
| 认证 | 复用 account-auth：HttpOnly Cookie `dg_session`（AuthRequired） |
| CSRF | 全部 POST 动作须带 `X-CSRF-Token` 头（R-02）；GET 豁免 |
| 中间件链 | AuthRequired → CsrfGuard（POST）→ RoleRequired（按接口，3.3 矩阵）；就绪度/互斥/白名单校验为服务层编排（D-03/D-04），非中间件 |
| 用户类型标注 | 查看接口（app/list、task/list、task/detail）：admin+readonly；全部写接口 + **env 读取与 log 读取**（admin 专属 ⚠️A8/.env 为参数面）：admin |

### 3.2 依赖与约束

| 依赖 | 类型 | 约束 |
|------|------|------|
| account-auth 中间件 | 框架 | AuthRequired/CsrfGuard/RoleRequired；不得自建会话解析 |
| account-auth AuditService.Record | OHS | 操作审计（R-03）；任务创建/终态与审计同事务（SQLite 侧）；服务操作/参数/建站联动同事务 |
| env-mgmt EnvReadySource | OHS（进程内读契约） | Python 就绪度（D-08）；venv/pip 能力位 v1.4 扩展；不代装环境（软引导） |
| site-mgmt SiteLinkGateway | OHS（进程内，消费方定义本轮） | 建站联动（D-12）；经安全写入管线 R-06；契约提供方登记跨模块待同步项 |
| service-mgmt SysdGateway | 共享适配层 | 服务启停自启与状态查询（D-10）；不复制 systemd 通道实现 |
| AppPyGateway/AppUnitWriter/LogGateway 适配器 | 动作通道 | venv/pip/单元写入/日志查询（R-08 参数数组，D-05/D-06/D-11 超时）；命令失败归一任务 failed 语义/APP_010 |
| SQLite | 存储 | app+app_task 双表（5.3）；单连接写串行承载互斥原子性（D-04） |
| panel-core | 框架 | 路由组挂载 `/app`；菜单登记「应用管理」 |

### 3.3 权限模型

角色：admin / readonly（panel_role）；无访客可达面。

**接口权限矩阵**：

| 接口 | admin | readonly | CSRF |
|------|-------|----------|------|
| GET /app/app/list | ✓ | ✓（ADR-018 全可看） | —（GET） |
| POST /app/app/create | ✓ | ✗（ADR-018 禁变更） | ✓ |
| POST /app/app/delete | ✓ | ✗（同上） | ✓ |
| GET /app/app/env | ✓ | ✗（参数面 admin 专属——参数编辑只读视角无入口） | —（GET） |
| POST /app/app/params | ✓ | ✗ | ✓ |
| POST /app/app/deps | ✓ | ✗ | ✓ |
| GET /app/task/list | ✓ | ✓（ADR-018） | —（GET） |
| GET /app/task/detail | ✓ | ✓（ADR-018） | —（GET） |
| POST /app/task/retry | ✓ | ✗ | ✓ |
| POST /app/service/start / stop / restart | ✓ | ✗（ADR-018 禁变更） | ✓ |
| POST /app/service/enable / disable | ✓ | ✗（同上） | ✓ |
| GET /app/log | ✓ | ✗（**日志 admin 专属 ⚠️A8**，沿 key 数据内容先例） | —（GET） |
| POST /app/site/link | ✓ | ✗ | ✓ |

数据权限规则：无行级隔离——应用全景与任务台账两角色可见集完全一致；角色差异体现在写接口与 env/log 两个 admin 专属读取面。readonly 越界由 RoleRequired 拦截返回 AUTH_008（附边界原因）。

### 3.4 公共规范

**错误码**（APP_ 前缀本模块新增 12 个；鉴权/CSRF/越界复用 account-auth 码族）：

| code | 标识 | HTTP 语义 | 说明 |
|------|------|----------|------|
| 0 | OK | 成功 | — |
| APP_001 | INVALID_APP_NAME | 参数错误 | 应用名非法（D-03 格式约束 `^[a-z0-9][a-z0-9-]{0,31}$`；R-14） |
| APP_002 | APP_NAME_TAKEN | 参数冲突 | 应用名已存在（I-02 唯一性；data.appId 指向既有档案） |
| APP_003 | APP_DIR_INVALID | 参数错误 | 代码目录或入口文件不存在（D-03；data.reason 区分 dir/entry；含目录逃逸拒绝语义） |
| APP_004 | ENV_MISSING | 拒绝 | Python 环境就绪度不足（D-08；data.missing=["python"\|"venv"\|"pip"] 分层；引导到环境管理） |
| APP_005 | PORT_OCCUPIED | 参数冲突 | 监听端口被占用（D-03 端口冲突检测；data.port；不静默换端口） |
| APP_006 | TASK_IN_FLIGHT | 参数冲突 | 同应用在途任务互斥（D-04，data.taskId 指向在途任务） |
| APP_007 | APP_NOT_FOUND | 拒绝 | 应用档案不存在（deps/delete/params/service/log/site/link 前置） |
| APP_008 | TASK_NOT_FOUND | 拒绝 | 任务不存在（台账实况） |
| APP_009 | VENV_MISSING | 拒绝 | venv 不存在（deps 前置 D-05；data.reason=运行环境缺失请重建应用或修复目录） |
| APP_010 | UNIT_OP_FAILED | 操作失败 | systemd 服务操作失败（D-10）；data.reason（网关错误详情） |
| APP_011 | SITE_LINK_FAILED | 操作失败 | 建站联动失败（D-12）；data.reason（site-mgmt OHS 错误透传，如端口/命名冲突语义） |
| APP_012 | PARAMS_INVALID | 参数错误 | 参数校验失败（.env 基线不一致/行格式非法/自定义命令白名单不通过 D-06/D-09；data.reason 细分） |
| APP_013 | IMAGE_INVALID | 参数错误 | 容器化应用镜像引用格式非法（`名称[:tag]` 服务端格式校验，复用 DK_012 同规则；R-14） |
| AUTH_001/002/003/008 | （复用） | — | 未登录/过期/CSRF 失败/readonly 越界（account-auth 归口） |

**枚举**（本模块 D2 定稿入池 3 项 + env_type 引用，本次引用）：

| 枚举标识 | 取值 | 用途 |
|---------|------|------|
| app_task_action | create \| deps_install \| delete | 任务动作（task/list·detail 响应） |
| app_task_status | pending \| running \| succeeded \| failed | 任务状态（task/list·detail 响应与筛选；D-04 四态机） |
| app_start_mode | preset_fastapi \| custom | 启动方式（app/list 响应+create 请求+params 编辑；D-06） |
| env_type | （引用，九值） | 就绪度呈现与 Python 环境引导（env-mgmt 定稿；本模块仅消费 python 项就绪度） |
| panel_role / audit_action / audit_result | （引用） | 权限与审计（account-auth 定稿） |

**业务常量**（审计细粒度动作码，非枚举池条目）：`app_task_started（创建任务，detail 含 appName/action）/ app_task_finished（终态，detail 含 action/status/error 摘要）/ app_service_started / app_service_stopped / app_service_restarted / app_service_enabled / app_service_disabled / app_params_updated（detail 不含参数值）/ app_site_linked（detail 含 appName/siteRef）`（category 均为 change；object_type=`app_task`/`app`，object_id=taskId 或 appId）。

**业务常量**（操作参数与阈值，非枚举池条目）：

| 常量 | 值 | 说明 |
|------|-----|------|
| PY_OP_TIMEOUT | 600s | venv 创建/单次 pip 安装命令超时上限（D-05；超时任务终态 failed error=timeout） |
| TASK_OUTPUT_TAIL_LINES | 200 | 任务输出尾部采集行数（沿 env-mgmt D-08 同值；终态定格 output_tail） |
| TASK_LIST_PAGE_SIZE | 50 | 任务列表默认分页（上限 100，公共分页约定内取值） |
| APP_UNIT_PREFIX | dougong-app- | 服务单元命名前缀（D-06；单元名=dougong-app-{name}.service） |
| APP_UNIT_DIR | /etc/systemd/system | 服务单元落位目录（D-06 编译期常量） |
| LOG_TAIL_OPTIONS | 200,500,1000 | 日志尾部档位（D-11） |
| LOG_QUERY_TIMEOUT | 10s | 日志查询超时（D-11） |
| APP_NAME_PATTERN | ^[a-z0-9][a-z0-9-]{0,31}$ | 应用名格式约束（D-03/APP_001） |

## 4. 聚合根与领域对象详细设计

### 4.1 app（聚合根，应用档案）

#### 4.1.1 领域模型

**表结构组合**：app 单表（5.3；实况投影零台账不落库 D-01；.env/unit 文件为外部事实源不入库）。

**字段清单**：

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | 聚合根标识（appId） |
| name | TEXT | NOT NULL UNIQUE | 应用名（全局唯一 I-02；D-03 格式约束） |
| dir | TEXT | NOT NULL | 代码目录（绝对路径规范化，D-03 防逃逸） |
| entry | TEXT | NOT NULL | 入口文件（相对 dir） |
| start_mode | TEXT | NOT NULL CHECK(in preset_fastapi,custom) | app_start_mode（D-06） |
| start_spec | TEXT | NOT NULL DEFAULT '{}' | 启动方式值对象 JSON 序列化（D-06：preset 参数段/自定义命令；整列替换式更新） |
| listen_port | INTEGER | NOT NULL CHECK(1024..65535) | 监听端口（默认 127.0.0.1 绑定语义 ⚠️A4） |
| unit_name | TEXT | NOT NULL | 服务单元名（dougong-app-{name}.service，D-06） |
| created_by | TEXT | NOT NULL | 创建者用户名（admin） |
| created_at / updated_at | DATETIME | NOT NULL | 创建 / 更新时间 |

**不变量**（聚合内）：

| 编号 | 不变量 | 违反处理 |
|------|--------|---------|
| I-01 | 档案-单元对应：unit_name 恒等于 APP_UNIT_PREFIX+name+.service（派生列一致性，命名前缀承载面板归属标识 ⚠️A12） | 写侧保证（D-06 渲染） |
| I-02 | 应用名全局唯一（UNIQUE 约束 + 创建预查 APP_002） | D-03 |
| I-03 | 档案仅随 create 任务 succeeded 登记、delete 任务 succeeded 撤除（I-C01/C02——无手工 INSERT/DELETE 通道） | 写侧编排 |
| I-04 | 参数引用外部事实源：.env/unit 文件内容不入库（OUT-14）；档案仅存引用（dir/entry/unit_name/start_spec） | 结构保证 |

**领域服务**：AppCatalog（呈现编排：档案×实况投影合并）、AppProber（实况探测编排 D-01：SysdGateway 单元态+venv 判定+EnvReadySource+端口监听）、ParamEditor（D-09 安全写回编排）、ServiceOps（D-10 服务操作编排+审计）、SiteLinker（D-12 建站联动编排）。

#### 4.1.2 状态模型

面板侧无自有状态机（档案存在性+服务实况投影；prd-app-mgmt.md 第六部分辨析五项：服务运行态/档案存在性/依赖就绪度/建站状态均非状态机）；服务运行/自启状态为 systemd 实况投影；「任务 succeeded 但服务未启动」合法（创建时未勾选立即启动），如实分开呈现。

#### 4.1.3 接口设计（app 侧接口）

见 4.3 接口设计（跨聚合统一）；app 侧接口：app/list（读）、app/create·delete·deps（异步任务发起）、app/env·params（参数面）、service/*（服务操作）、log（日志）、site/link（建站联动）。

#### 4.1.4 领域事件与集成

无 MQ、无领域事件。app 侧仓储契约：`AppRepository { Create(tx)(I-C01), GetByID, GetByName, List, Delete(tx)(I-C01), UpdateStartSpec(tx) }`。网关契约见 4.4。

### 4.2 app_task（聚合根，应用变更任务）

#### 4.2.1 领域模型

**表结构组合**：app_task 单表（5.3；与 env_install_task 台账同构）。

**字段清单**：

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INTEGER | PK AUTOINCREMENT | 聚合根标识（taskId） |
| app_id | INTEGER | NOT NULL | 目标应用（app 弱引用；create 场景档案未登记时=预分配 id 语义，见备注） |
| app_name | TEXT | NOT NULL | 应用名快照（列表/详情呈现；档案撤除后任务仍可读——沿 backup_job job_name 快照先例） |
| action | TEXT | NOT NULL CHECK(in create,deps_install,delete) | app_task_action |
| status | TEXT | NOT NULL DEFAULT 'pending' CHECK(in pending,running,succeeded,failed) | app_task_status（四态机 D-04） |
| purge_dir | INTEGER | NOT NULL DEFAULT 0 | 删除范围（D-07；0=保留代码目录/1=同时删除；仅 delete 任务语义） |
| output_tail | TEXT | NOT NULL DEFAULT '' | 输出尾部（终态定格，≤200 行） |
| error | TEXT | NULL | 失败原因（failed 终态；含 data.reason/timeout/面板重启中断） |
| requested_by | TEXT | NOT NULL | 发起者用户名（admin） |
| created_at / updated_at | DATETIME | NOT NULL | 发起时间 / 状态更新时间 |
| started_at / finished_at | DATETIME | NULL | 执行开始 / 终态时间 |

> 备注（app_id 语义）：create 任务的 app_id 在档案登记前不存在——取「任务先行、档案后置」形态：app_id 对 create 任务存 0（未登记），档案登记（I-C01 同事务）后按 name 关联呈现；deps/delete 任务的 app_id 为既有档案 ID。呈现层以 app_name 快照归组（app/list 在途标记按 name 匹配）。

**不变量**（聚合内）：

| 编号 | 不变量 | 违反处理 |
|------|--------|---------|
| I-05 | 同应用在途唯一：同 app_name（create 场景）/app_id（deps/delete 场景）至多一个 pending/running 任务 | 创建事务内判定（APP_006，data.taskId） |
| I-06 | 终态不可逆：succeeded/failed 不再迁移；**例外：failed 且 action ∈ {create,deps_install} 可经 retry 回 running（D-13 显式声明，重试复用原任务行）** | 写侧编排 |
| I-07 | 任务创建与发起审计同事务；终态落库+档案协作+终态审计同事务（I-C01） | D-04/D-05 |
| I-08 | 重启恢复：启动扫描 running 任务标 failed（error=面板重启中断），不续跑 | main 装配启动环节（D-04） |
| I-09 | 输出尾部上限：output_tail 恒为最近 ≤TASK_OUTPUT_TAIL_LINES 行 | 采集侧保证 |
| I-10 | 删除范围显式：purge_dir 仅 delete 任务携带且默认 0（D-07/I-C04） | 写侧保证 |

#### 4.2.2 状态模型

app_task_status 四态状态机（⚠️A3）：

| 当前状态 | 目标状态 | 触发 | 接口 |
|---------|---------|------|------|
| pending | running | worker 取出任务开始执行 | create/deps/delete 创建后异步 |
| running | succeeded | 执行链全部成功 | worker 终态回写（+档案协作 I-C01） |
| running | failed | 步骤失败/超时（PY_OP_TIMEOUT） | worker 终态回写（error+输出定格） |
| running | failed | 面板重启恢复 | 启动扫描（I-08，error=面板重启中断） |
| failed | running | retry（仅 create/deps_install，D-13） | POST /app/task/retry |

无审批/驳回环节；状态变迁历史经任务台账本体+审计（started/finished 两段）。

#### 4.2.3 接口设计（task 侧接口）

见 4.3（task/list、task/detail、task/retry）。

#### 4.2.4 领域事件与集成

无 MQ。app_task 侧仓储契约：`AppTaskRepository { Create(tx), GetByID, List(筛选+分页), UpdateStatus(tx), AppendOutputTail, Finish(tx)(status/error+档案协作 I-C01 同事务), FailRunningAll(tx)(重启恢复), Retry(tx)(failed→running，D-13), InFlightByApp(name/id) }`。

### 4.3 接口设计（跨聚合统一）

**接口清单**（16 接口：GET 5 / POST 11）：

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /app/app/list | GET | admin+readonly | 应用全景（档案+实况+在途标记+站点提示，D-01） |
| /app/app/create | POST | admin | 创建应用（前置校验链 D-03 → 异步任务 D-04/D-05） |
| /app/app/delete | POST | admin | 删除应用（purgeDir 范围 D-07 → 异步任务） |
| /app/app/env | GET | admin | .env 内容读取（D-09；admin 专属） |
| /app/app/params | POST | admin | 参数保存（.env+启动参数，安全写回 D-09） |
| /app/app/deps | POST | admin | 依赖重装（venv 前置 → 异步任务） |
| /app/task/list | GET | admin+readonly | 任务台账（action/status 筛选+按应用筛选，分页） |
| /app/task/detail | GET | admin+readonly | 单任务详情（输出尾部排障） |
| /app/task/retry | POST | admin | 失败任务重试（D-13：仅 create/deps_install） |
| /app/service/{start,stop,restart,enable,disable} | POST | admin | 服务操作（瞬时通道 D-10，5 接口） |
| /app/log | GET | admin | 应用日志尾部（D-11；admin 专属 ⚠️A8） |
| /app/site/link | POST | admin | 建站联动（消费 site-mgmt OHS D-12） |

**接口详情**（全部写接口隐含 CSRF 与 AUTH_008 角色边界；create/delete/deps 隐含互斥 APP_006、档案前置 APP_007，不再逐条重复）：

- **GET /app/app/list**：无参。处理：AppRepository.List 全量 → AppProber 逐应用探测（服务运行/自启经 SysdGateway 单元态〔unit_name=档案映射〕；venv 存在性文件判定；Python 就绪度经 EnvReadySource〔D-08，模块级一次探测全应用共享——就绪度为环境事实非应用事实〕；依赖安装态 venv 内包查询；端口监听本机查询）→ 在途任务标记（I-05 按 name/id 归组查询 pending/running）→ 站点提示（经 site-mgmt 站点清单联动查询按端口/命名匹配呈现，只读值快照）。响应 data=`{pythonReadiness: {installed: boolean, version: string|null, venvSupport: boolean, pipAvailable: boolean}, items:[{id, name, dir, entry, startMode: app_start_mode, listenPort, serviceRunning: boolean|null, serviceEnabled: boolean|null, venvExists: boolean, depsInstalled: boolean|null, portListening: boolean|null, runningTaskId: number|null, hasSite: boolean|null}], generatedAt}`（探测失败字段 null 如实不虚构）。审计：—（查看类）。
- **POST /app/app/create**：req `{name, startMode: app_start_mode, listenPort, startNow?: boolean, ...形态分支字段}`——**v1.1 双形态（⚠️A13）**：venv 形态（startMode=preset_fastapi|custom）字段不变 `{dir, entry, startSpec}`；**container 形态（startMode=container）**：`{image, ports?: [{host, container, protocol?}], env?: [{key, value}], mounts?: [{source, target}], restartPolicy?: "no"|"always"|"unless-stopped"|"on-failure"}`——镜像必填（格式校验 APP_013，复用 DK_012 规则）；无 dir/entry/Python 就绪度校验；服务端生成单应用 compose.yml（含 dougong-app 归属标签 ⚠️A13）→ 经 docker-mgmt Compose 项目创建通道（api-docker-mgmt.md v1.1 D-11，消费方定义）落盘并自动 up -d → app 档案持 compose 项目名引用（unit_name 为空；venv 探测/端口监听实况口径分支）。校验链（D-03 顺序）：APP_001 → APP_002 → APP_003（dir 规范化+entry 存在）→ APP_004（data.missing 分层）→ APP_005 → 白名单校验（custom 时 APP_012）。执行：INSERT app_task（action=create，app_name=name，app_id=0，pending）+发起审计同事务 → worker（D-05：CreateVenv→pip install〔有 requirements.txt〕→单元生成〔D-06 安全管线〕→startNow 时 start→succeeded+档案 INSERT 同事务〔I-C01/C02〕）。响应 data=`{taskId}`。审计：app_task_started + app_task_finished。
- **POST /app/app/delete**：req `{appId, purgeDir?: boolean=false}`。校验：APP_007。执行：INSERT app_task（action=delete，purge_dir=请求值）+审计 → worker（D-07：stop→disable→单元移除+daemon-reload→〔purgeDir〕目录删除→succeeded+档案 DELETE 同事务〔I-C01/C04〕）。响应 data=`{taskId}`。审计：app_task_started + app_task_finished（detail 含删除范围）。
- **GET /app/app/env**：query `{appId}`。校验：APP_007。处理：读 {dir}/.env（不存在=空数组如实；行原样呈现，KEY=VALUE 解析仅供前端表单渲染）。响应 data=`{appId, lines: string[], updatedAt: string|null}`。审计：—（查看类，参数面 admin 专属）。
- **POST /app/app/params**：req `{appId, envLines?: string[], startSpec?: 同 create（可选，仅变更段提交）, baseMtime?: string}`。校验：APP_007；基线比对（D-09：.env 现 mtime 与 baseMtime 不一致 → APP_012 data.reason=「文件已被外部修改，请刷新后重试」）；行格式/白名单（APP_012）。执行：.env 整文件原子写（备份原文件失败还原）+ startSpec 变更时单元更新（D-06 安全管线）→ 同事务审计。响应 data=`{appId, needRestart: true}`（前端呈现「需重启应用生效」提示，不自动重启）。审计：app_params_updated（change，detail 不含参数值）。
- **POST /app/app/deps**：req `{appId}`。校验：APP_007；venv 存在（APP_009）；requirements.txt 存在（缺失如实拒绝 data.reason=「应用无 requirements.txt，无需安装依赖」——不虚构任务）。执行：INSERT app_task（action=deps_install）+审计 → worker（pip install → succeeded；不重启服务提示归前端 D-09）。响应 data=`{taskId}`。审计：app_task_started + app_task_finished。
- **GET /app/task/list**：query `{appId?(按应用筛选), action?(app_task_action), status?(app_task_status), page?, pageSize?(默认 50 上限 100)}`。响应 data=`{items:[{id, appId, appName, action, status, purgeDir, requestedBy, createdAt, startedAt?, finishedAt?, error?, retryable: boolean}], total}`（created_at 倒序；retryable=failed 且 action∈{create,deps_install}——D-13 前端按钮态数据源）。审计：—（查看类）。
- **GET /app/task/detail**：query `{taskId}`。校验：APP_008。响应 data=`{...list 项全字段, outputTail}`。审计：—（查看类）。
- **POST /app/task/retry**：req `{taskId}`。校验：APP_008；retryable 判定（D-13：failed 且 action∈{create,deps_install}，否则 APP_008 data.reason=该任务不可重试）；互斥复查（I-05——重试时若同应用已有新在途任务如实拒绝 APP_006）。执行：状态回 running（Retry tx）→ worker 重新执行（create 重试=venv 已存在跳过创建步骤，幂等）。响应 data=`{taskId}`。审计：app_task_started（detail 含 retry 语义）+ app_task_finished。
- **POST /app/service/{start|stop|restart|enable|disable}**：req `{appId}`。校验：APP_007。执行：SysdGateway 按档案 unit_name 操作（D-10，等待终态语义同 service-mgmt）→ 同事务审计。响应 data=`{appId, unit: unitName, result: "ok"|失败 APP_010 data.reason}`。审计：app_service_started/stopped/restarted/enabled/disabled（change，detail={appId, unit}）。
- **GET /app/log**：query `{appId, lines?(200|500|1000, 默认 200)}`。校验：APP_007；lines ∈ LOG_TAIL_OPTIONS（R-14）。处理：LogGateway journalctl 查询（D-11，LOG_QUERY_TIMEOUT）。响应 data=`{appId, unit: unitName, lines: 档位, content: string}`。审计：—（查看类；admin 专属 ⚠️A8）。
- **POST /app/site/link**：req `{appId, domain?: string}`。校验：APP_007；域名格式（携带时 RFC 1123 校验，沿 panel-core bindDomain 规则语义）。执行：SiteLinkGateway.CreateReverseProxySite（D-12：reverse_proxy 站点，上游 127.0.0.1:{listen_port}；经 site-mgmt R-06 管线）→ 同事务审计。响应 data=`{appId, siteRef: {file, index, serverName, scheme, port}}`（值快照呈现，不落库）。失败：APP_011 data.reason（site-mgmt 错误透传）。审计：app_site_linked（change，detail 含 appName/siteRef）。

**跨接口复用规则**：实况探测统一 AppProber（list/create 前置/呈现共用；Python 就绪度模块级一次探测共享）；任务执行统一 TaskWorker（create/deps/delete 共用链骨架，步骤编排差异内聚）；服务操作统一 ServiceOps；参数写回统一 ParamEditor（D-06 安全管线复用）；审计统一 AuditService.Record（同事务）。

**幂等语义**：create 重复提交经 I-02/I-05 拒绝（APP_002/APP_006 如实）；delete/deps 在途互斥拒绝（APP_006）；服务操作对已处目标态幂等（systemd 语义）；retry 幂等由互斥复查承载（I-05）；params 基线比对防覆盖（D-09 乐观锁语义）；task/detail 重复读无副作用。防重复提交为前端在途态（P3 过程数据）。

### 4.4 领域事件与集成

无 MQ、无领域事件。本机资源侧网关契约（进程内适配层，非 SQL）：

```
AppPyGateway {
  CreateVenv(ctx, dir, timeout) (outputTail string, err)   // python3 -m venv .venv（D-05，R-08 参数数组）
  PipInstall(ctx, dir, timeout) (outputTail string, err)   // .venv/bin/pip install -r requirements.txt（D-05）
  VenvExists(dir) bool                                      // .venv/ 目录判定（D-08）
  DepsProbe(dir) (count int, err)                           // .venv/bin/pip list 只读计数（D-01；探测失败=错误如实）
  RemoveDir(ctx, dir) (outputTail string, err)              // 目录删除（D-07 purgeDir=true；路径过规范化校验）
}
AppUnitWriter {
  WriteUnit(unitName, content) (rollback func(), err)       // 安全写入管线（D-06：备份→原子写→systemd-analyze verify→失败回滚）
  RemoveUnit(unitName) (rollback func(), err)               // 单元移除（D-07；备份保留）
  DaemonReload(ctx) err                                     // systemctl daemon-reload（参数数组）
  VerifyUnit(unitPath) err                                  // systemd-analyze verify（参数数组）
}
LogGateway {
  Tail(unitName string, lines int, timeout) (content string, err)  // journalctl -u {unit} -n {lines} --no-pager（D-11）
}
SiteLinkGateway（site-mgmt OHS 消费契约，本轮消费方定义，D-12）{
  CreateReverseProxySite(ctx, req {name, port, domain?}) (siteRef SiteRef, err)
}
EnvReadySource（env-mgmt 读契约消费，D-08；权威定义见 api-env-mgmt.md v1.4 §5.1）{
  PythonReadiness() (PythonReadiness {Installed bool, Version string|null, VenvSupport bool, PipAvailable bool}, err)
}
SysdGateway（复用 service-mgmt 适配器，D-10）{ UnitState(unit) / Start / Stop / Restart / Enable / Disable }
约束：全部命令字面量参数数组（R-08）；白名单校验（D-03/D-06）；TaskWorker 组合 CreateVenv→PipInstall→WriteUnit→[Start] 完成创建链（D-05）
```

## 5. 模块清单汇总

### 5.1 对外服务清单（HTTP 接口）

| 服务（接口） | 消费方 | 契约要点 |
|-------------|--------|---------|
| GET /app/app/list | 前端应用管理页（两角色） | 档案+实况实时投影（D-01）；在途标记（I-05）；Python 就绪度分层呈现 |
| POST /app/app/create | 前端应用管理页（admin） | 前置校验链（D-03）；异步任务（D-04/D-05）；档案终态登记（I-C01/C02）；APP_001~006/012 |
| POST /app/app/delete | 前端应用管理页（admin） | 删除范围显式（D-07/I-C04）；绝不静默删代码；站点不自动删 |
| GET /app/app/env · POST /app/app/params | 前端应用管理页（admin） | .env 外部事实源直读写（D-09）；基线比对乐观锁；安全写回 |
| POST /app/app/deps | 前端应用管理页（admin） | venv 前置 APP_009；失败可重试（D-13） |
| GET /app/task/list · GET /app/task/detail · POST /app/task/retry | 前端应用管理页（list/detail 两角色，retry admin） | 任务台账与输出尾部；retryable 按钮态（D-13）；ADR-018 |
| POST /app/service/{start,stop,restart,enable,disable} | 前端应用管理页（admin） | SysdGateway 复用（D-10）；档案单元限定；双入口并存 ⚠️A12 |
| GET /app/log | 前端应用管理页（admin 专属 ⚠️A8） | journalctl 尾部档位（D-11） |
| POST /app/site/link | 前端应用管理页（admin） | site-mgmt OHS 消费（D-12）；站点引用值快照不落库 |
| Python 就绪度（软引导，非 HTTP） | 前端创建拒绝提示 | 「到环境管理安装 Python」指引链接 |
| 中间件 AuthRequired / CsrfGuard / RoleRequired | （复用） | account-auth 提供（3.1） |
| AuditService.Record | （出站调用） | 操作审计（R-03，同事务） |
| **env-mgmt EnvReadySource（消费）** | **（出站调用，进程内 OHS）** | **Python 就绪度（D-08）；api-env-mgmt.md v1.4 §5.1 提供方登记** |
| **site-mgmt SiteLinkGateway（消费）** | **（出站调用，进程内 OHS）** | **建站联动（D-12）；api-site-mgmt.md §5.1 提供方登记归其联动轮（跨模块待同步项）** |

本模块无对外提供的 OHS 服务契约（HTTP 消费方仅前端应用管理页）；service-mgmt 服务清单可见应用单元为通用单元层如实呈现行为（⚠️A12），非契约消费。

### 5.2 SQL 清单

| # | SQL | 场景 |
|---|-----|------|
| 1 | INSERT INTO app_task(app_id, app_name, action, status, purge_dir, requested_by, created_at, updated_at) VALUES(...) | 任务创建（pending，同事务发起审计，D-04/I-07） |
| 2 | SELECT * FROM app_task WHERE (app_id=? OR app_name=?) AND status IN ('pending','running') | 互斥判定（I-05，创建事务内） |
| 3 | SELECT * FROM app_task WHERE id=? | 任务详情 |
| 4 | SELECT * FROM app_task WHERE [app_id=?] [AND action=?] [AND status=?] ORDER BY created_at DESC LIMIT ? OFFSET ? | 任务列表（筛选+分页） |
| 5 | UPDATE app_task SET status='running', started_at=?, updated_at=? WHERE id=? | 执行开始（worker） |
| 6 | UPDATE app_task SET output_tail=?, updated_at=? WHERE id=? | 输出尾部追加/定格 |
| 7 | UPDATE app_task SET status='running', started_at=?, updated_at=? WHERE id=? AND status='failed' | 重试（D-13/I-06 例外；worker 重新消费） |
| 8 | INSERT INTO app(name, dir, entry, start_mode, start_spec, listen_port, unit_name, created_by, created_at, updated_at) VALUES(...) | 档案登记（create 任务 succeeded 终态同事务，I-C01/C02） |
| 9 | DELETE FROM app WHERE id=? | 档案撤除（delete 任务 succeeded 终态同事务，I-C01） |
| 10 | UPDATE app_task SET status=?, error=?, finished_at=?, updated_at=? WHERE id=? | 终态回写（与 SQL#8/#9 同事务，I-C01） |
| 11 | UPDATE app_task SET status='failed', error='面板重启中断', finished_at=?, updated_at=? WHERE status='running' | 重启恢复（I-08，启动扫描） |
| 12 | SELECT id, name FROM app_task WHERE status IN ('pending','running') | 应用列表在途标记（I-05 查询） |
| 13 | UPDATE app SET start_spec=?, updated_at=? WHERE id=? | 启动参数变更（params，与审计同事务） |

### 5.3 DDL

```sql
CREATE TABLE IF NOT EXISTS app (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  name         TEXT    NOT NULL UNIQUE,                -- 应用名（全局唯一，I-02；APP_UNIT_PREFIX 派生单元名）
  dir          TEXT    NOT NULL,                       -- 代码目录（绝对路径规范化，D-03）
  entry        TEXT    NOT NULL,                       -- 入口文件（相对 dir）
  start_mode   TEXT    NOT NULL CHECK (start_mode IN ('preset_fastapi','custom')),
  start_spec   TEXT    NOT NULL DEFAULT '{}',          -- 启动方式值对象 JSON（D-06；整列替换式更新）
  listen_port  INTEGER NOT NULL CHECK (listen_port BETWEEN 1024 AND 65535),
  unit_name    TEXT    NOT NULL,                       -- 服务单元名（dougong-app-{name}.service，D-06）
  created_by   TEXT    NOT NULL,
  created_at   DATETIME NOT NULL,
  updated_at   DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_app_unit ON app(unit_name);

CREATE TABLE IF NOT EXISTS app_task (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  app_id       INTEGER NOT NULL DEFAULT 0,             -- 目标应用（create 任务=0 未登记语义，4.2.1 备注）
  app_name     TEXT    NOT NULL,                       -- 应用名快照（沿 backup_job job_name 先例）
  action       TEXT    NOT NULL CHECK (action IN ('create','deps_install','delete')),
  status       TEXT    NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','succeeded','failed')),
  purge_dir    INTEGER NOT NULL DEFAULT 0,             -- 删除范围（D-07；0=保留代码目录）
  output_tail  TEXT    NOT NULL DEFAULT '',            -- 输出尾部（≤200 行定格）
  error        TEXT,                                   -- 失败原因（failed；含 data.reason/timeout/面板重启中断）
  requested_by TEXT    NOT NULL,
  created_at   DATETIME NOT NULL,
  updated_at   DATETIME NOT NULL,
  started_at   DATETIME,
  finished_at  DATETIME
);
CREATE INDEX IF NOT EXISTS idx_app_task_app_status ON app_task(app_id, status);
CREATE INDEX IF NOT EXISTS idx_app_task_created ON app_task(created_at);
```

> 迁移：**015_app_mgmt.sql**（初建两表；014 已被 env-mgmt Node CHECK 重建占用，015 为下一空号）。app/app_task 均为新建表零数据迁移风险；索引见上（idx_app_unit 唯一索引承载单元名一致性 I-01；idx_app_task_app_status 承载互斥查询 I-05）。

## 6. 更新记录

### 6.1 变更记录

| 版本 | 日期 | 变更类型 | 变更摘要 | 目标节号 |
|------|------|---------|---------|---------|
| v1.0 | 2026-11-20 | initial | 首次生成：16 个 HTTP 接口（/app/{app,task,service,log,site}/*——GET 5〔app/list、app/env、task/list、task/detail、log〕/ POST 11〔app/create·delete·params·deps、task/retry、service/{start,stop,restart,enable,disable}×5、site/link〕）+ 2 表 DDL（app 档案〔UNIQUE name+unit 唯一索引〕+ app_task 四态任务台账〔app_id/app_name 快照+purge_dir 删除范围〕，迁移 015_app_mgmt.sql）+ 5 网关契约（AppPyGateway/AppUnitWriter/LogGateway/SiteLinkGateway〔site-mgmt OHS 消费，跨模块待同步项〕/EnvReadySource〔env-mgmt 读契约 v1.4〕）+ SysdGateway 复用 + 12 错误码（APP_001~012 + 复用 AUTH 族）+ 3 枚举引用（app_task_action/app_task_status/app_start_mode 本域 D2 定稿入池）+ 9 审计动作常量 + 15 项设计决策（D-01 实时探测/D-02 双聚合落库/D-03 创建校验链/D-04 异步任务互斥+重启恢复/D-05 创建执行链+半档案不残留/D-06 单元生成安全写入/D-07 删除保守执行链/D-08 就绪度读契约/D-09 参数安全写回+基线比对/D-10 服务操作瞬时通道/D-11 日志 admin 专属/D-12 建站联动 OHS/D-13 失败重试语义/D-14 混合模式/D-15 台账全量保留）；第 2 章跨聚合设计（I-C01~C04：终态档案同事务/半档案不残留/互斥跨聚合/删除范围显式）；关闭 prd-app-mgmt.md 待对齐-3（就绪度能力位字段=venvSupport/pipAvailable）/待对齐-4 部分（日志档位 200/500/1000、超时 10s、PY_OP_TIMEOUT=600s、APP_UNIT_PREFIX）/待对齐-5（应用名 Pattern）/待对齐-6（服务操作瞬时通道不占任务互斥）/待对齐-7（.env 整文件替换+基线比对）；待对齐-1（迁移编号=015 落定关闭）；待对齐-2（建站 OHS 消费方定义落定，提供方登记归 site-mgmt 联动轮）；无旧版对比 | — |
| v1.2 | 2026-11-23 | 增量 | 宝塔式重组 D3（ADR-033）：POST /app/app/create 增 runtime 与 Node 启动方式（node_file / npm_script），startMode=container 拒绝 APP_014；GET /app/app/list 条目增 runtime、响应增 nodeReady、可选 runtime 过滤；新增 APP_014~016；迁移 018_app_runtime.sql；D-16 / D-17 | 文末「增量 v1.2」节 A~E |

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

---

## 机械自检记录（M1~M5，生成时实跑口径）

| # | 自检项 | 结果 |
|---|--------|------|
| M1 | 章节结构完整（1.x/2.x/3.x/4.x/5.x/6.x 🔒 全在） | ✅ |
| M2 | 接口路径三段式（16/16 路径 `/app/{resource}/{action}` 形态；service/{verb} 为路径段承载动作沿 env-mgmt 先例） | ✅ |
| M3 | 枚举引用有定义（app_task_action/app_task_status/app_start_mode/env_type/panel_role/audit_action/audit_result 均在 04-business-enum.md 有定义；本域 3 项 D2 已入池） | ✅ |
| M4 | 占位符残留（🔒 章节无 {按需填写}/TODO） | ✅ |
| M5 | 变更记录（6.1 含 v1.0 initial 行；6.2/6.3 在位） | ✅ |
| v1.1 | 2026-11-22 | 容器化应用增量 D1（BA/AA/FL v1.1 级联，d0-clarification-docker-mgmt-create-2026-11-21.md Q5「本轮一起做」）：①enum_value_added：app_start_mode +container（DDL CHECK 迁移 017_app_container.sql 表重建沿 015 先例）；②rule_changed：POST /app/app/create 请求扩 container 形态分支（image 必填 APP_013/端口映射/环境变量/挂载/重启策略；无 dir/entry/Python 就绪度校验）；③跨模块消费 +docker-mgmt Compose 项目创建通道（api-docker-mgmt.md v1.1 S-50-04/D-11 提供方在位——归属标签 dougong.app 注入；消费方契约本侧登记）；④field_changed：app 档案 unit_name 对容器化形态为空、改持 compose 项目名引用（start_spec JSON 承载容器化参数）；⑤错误码 +APP_013 IMAGE_INVALID（镜像格式校验，复用 DK_012 规则）；⑥建站联动（FR-10/S-61-04）对容器化形态同等适用（反代到容器映射端口）；⑦app_start_mode 枚举池 +container 值（D2 确认，四方同值归 D4 前端 enums 同步） |

## 增量 v1.2（2026-11-23）：宝塔式重组 D3——Node 运行时与容器形态下线

> 依据 baota-restructure-design-2026-11-23.md 第三、四、八章。本节是权威增量；上文 v1.1 中「容器运行」形态相关字段与校验以本节为准（下线）。

### A. 接口变更

| 接口 | 变更 |
|------|------|
| POST /app/app/create | 请求体增 runtime?: string（python \| node，缺省 python）。startMode 取值域按运行时：python → preset_fastapi \| custom；node → node_file \| npm_script。startSpec 增 file?: string（node_file：项目内入口文件相对路径）、script?: string（npm_script：npm 脚本名，缺省 start）。startMode=container → APP_014 |
| GET /app/app/list | 条目增 runtime: string；响应增 nodeReady: NodeReady；query 增 runtime?: string（可选过滤，缺省返回全部） |
| 其余 14 个接口 | 不变——删除 / 依赖重装 / 参数 / 服务操作 / 日志 / 任务 / 建站对 Node 项目同等适用（依赖重装对 Node 执行 npm install） |

### B. 新增结构

| 结构 | 字段 | 说明 |
|------|------|------|
| NodeReady | nodeAvailable: boolean；npmAvailable: boolean；nodeVersion: string \| null | Node 就绪度（来源 env-mgmt NodeReadiness 探测；版本不可知时为 null 不虚构）；与 pythonReady 并列 |

### C. 新增错误码

| code | 标识 | HTTP 语义 | 说明 |
|------|------|---------|------|
| APP_014 | START_MODE_RETIRED | 参数错误 | 该启动方式已下线：容器化部署请到「容器管理 › 应用」创建 |
| APP_015 | NODE_ENV_INCOMPLETE | 前置条件不满足 | Node 环境不完整（缺 node 或 npm），请到环境管理安装或修复 Node |
| APP_016 | NODE_START_SPEC_INVALID | 参数错误 | Node 启动参数非法：入口文件须为项目内的相对路径；脚本名只允许字母、数字与 . _ : - |

### D. 设计决策（增补）

| 编号 | 决策 | 理由 |
|------|------|------|
| D-16 | Node 进程托管用 systemd 原生（不引入 PM2）；服务单元的 ExecStart 使用创建时解析到的 node / npm 绝对路径，并记入应用档案 | systemd 不做 PATH 查找；与 Python 项目先例一致（R-08 参数数组、单元安全写入与回滚） |
| D-17 | Node 依赖安装为 npm install（存在 package.json 时执行，不存在则跳过），复用 deps_install 任务动作与状态机；不建隔离环境，依赖落在项目目录 | Node 依赖天然属于项目目录；任务观测面复用，不新增动作 |

### E. DDL（迁移 018_app_runtime.sql）

ALTER TABLE app ADD COLUMN runtime TEXT NOT NULL DEFAULT 'python';
——仅加列，不重建表；既有行缺省即 Python 项目。start_mode 列的 CHECK 约束保留历史值 container（该形态已下线，由服务层拒绝，APP_014）。
