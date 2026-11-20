# D0 澄清：Python 应用管理（app-mgmt，2026-11-20）

> 触发：用户准备在部署了斗拱面板的服务器上部署 nim-proxy（自有 Python 项目），
> 疑问「面板功能上是不是缺了些配置？例如管理 Python 项目；容器管理页面只读不能接管」。
> 经第一轮范围拍板：**组合范围 = 开发 Python 应用管理 + 排查容器只读问题**。
> 模式：新模块 app-mgmt（消费 env-mgmt / site-mgmt / service-mgmt 既有积木）。
> 备注：db-tools D0（2026-11-19）已预告「dougong 未来要上 Node/Python 项目管理」，
> 本轮兑现 Python 侧；Node 已于 env-mgmt v1.2/v1.3 增量落地。

## 一、用户拍板（2026-11-20 两轮 AskUserQuestion）

**第一轮（范围拍板）**：
- 范围 = **Python 管理 + 容器 组合**：开发 Python 应用管理模块；容器侧只排查、不开发新增能力。
- 执行计划 = 会话编排轨（环境无 CreateWorkflow/Agent 工具，工作流轨与 subagent 模式降级为
  会话内执行，产物落点与索引状态协议不变）。
- 容器取证 = 登录账号自述为 admin。

**第二轮（D0 四拍板）**：
- **Q1 部署路线 = venv 为主 + 容器排查**：nim-proxy 这类 Python 应用走 venv + systemd 路线，
  由新模块管理应用生命周期；对外暴露复用建站反向代理；容器侧仅排查只读问题。
- **Q2 Python 运行时供给 = 环境管理扩 Python**：env-mgmt 环境目录八环境 → 九环境（+python），
  沿 Node 增量先例（⚠️A16 代配/⚠️A17 运行时型无服务单元同构）。
- **Q3 能力范围 = 四项全选**：应用生命周期基础（列表/创建/删除 + 环境变量与启动参数编辑）、
  venv 与依赖安装（任务化）、进程托管与自启（systemd）、日志查看 + 建站联动（反向代理一键建站）。
- **Q4 容器取证 = 有列表无操作列**（详见第五节排查结论）。

## 二、架构匹配与复用积木（代码取证）

- **env-mgmt**：环境目录现八环境（nginx/apache/docker/mysql/redis/php-fpm/mongodb/node），
  Python 此前明确 OUT（v1.2 边界表）→ 本轮扩第九环境 python。Node 先例完全同构可复用：
  VendorRepo 三系代配（⚠️A16）、运行时型无服务单元（⚠️A17，服务操作组不渲染、服务态 null 如实）、
  BinaryVersion 命令行版本探测、依赖就绪度读契约（db-tools depsMissing 消费模式）。
  venv 支撑差异：Debian 系需 python3-venv 包（venv 单独分包），RHEL 系 python3 自带；
  检测口径（python3 --version + venv 可用性探测）细节归 D3。
- **site-mgmt**：建站类型 static / php_fpm / **reverse_proxy** 已存在（service/sitemgmt/consts.go:53-55）
  → 应用对外暴露零新建，仅联动（消费 site-mgmt OHS 建站，沿 db-tools 消费先例）。
- **service-mgmt**：systemd 服务管理已有（SysdGateway 被 env-mgmt 复用先例）；
  app-mgmt 生成应用 unit 后，服务操作（启停/重启/自启）复用 sysd 通道。
  与 service-mgmt 服务列表的重叠呈现口径（应用 unit 是否双入口可见）归 D1 裁定。
- **docker-mgmt**：24 端点（9 GET 两角色 + 13 POST admin + 2 WS 日志跟随），容器
  启停/重启/暂停/恢复/删除、镜像拉取/删除、Compose 编排、日志实时跟随全部在位——
  **容器接管能力无缺失**，本轮零开发（只读问题排查见第五节）。
- **任务状态机先例**：env_install_task / tool_deploy_task 四态机（pending/running/succeeded/failed）
  落 SQLite + worker 有限时 goroutine + RecoverInFlight + 审计同事务——app-mgmt 依赖安装
  任务照此同构。
- **nim-proxy 本体**（D:\Project\nim-proxy）：FastAPI + uvicorn[standard] + httpx，
  app.py 入口 + requirements.txt，自带手动 deploy.sh 与 nginx conf——venv/systemd 路线的
  典型形态，作为应用创建参数（入口文件/框架预设/监听端口）设计输入。

## 三、范围（本轮做）

1. **env-mgmt +Python 环境**（增量）：Catalog 第九条目（apt python3/python3-venv/python3-pip
   三包候选、dnf/yum python3；代配沿 NodeSource 先例评估是否需要第三方源，细节 D3 定稿）；
   检测/安装/升级/卸载/版本探测沿运行时型形态；迁移 + 枚举 + 测试；依赖就绪度读契约
   扩 venv 能力位（app-mgmt 消费）。
2. **app-mgmt 新模块**（Python 应用管理）：
   - 应用档案：名称、代码目录、入口文件、启动方式（框架预设 fastapi/uvicorn 或自定义命令）、
     监听端口、运行用户候选；档案落 SQLite（聚合根候选：app + deploy/依赖任务，D2 建模定稿）。
   - 创建链路：目录校验 → python3 -m venv .venv → pip install -r requirements.txt（任务化、
     失败原因可见、可重试）→ 生成 systemd unit → 可选立即启动。
   - 进程托管：启停/重启/开机自启开关/状态探测（复用 sysd 通道）；Restart=on-failure
     异常拉起；环境变量与启动参数编辑（.env 文件 / unit 参数，安全写回）。
   - 日志：journalctl 尾部查看（admin）。
   - 删除：停服务 + 删 unit + 撤自启；代码目录与 venv 保留可选（沿卸载不删数据哲学）。
   - 建站联动：一键创建 reverse_proxy 站点反代应用端口（消费 site-mgmt，可选绑域名）。
   - 双角色：admin 全操作；readonly 只读观测（沿 ADR-018 负空间）。
3. **前端**：Python 应用管理页（应用列表/创建向导/详情 Drawer：状态/参数编辑/日志/建站入口）
   + 环境管理页 Python 卡片（沿 Node 条目形态）+ readonly 负空间。

## 四、安全底线（不可妥协）

- R-08 exec 参数数组（python3/pip/systemctl/journalctl 全部）；写 systemd unit 走安全写回
  管线（备份→原子写→校验→失败回滚，与 site-mgmt R-06 同构，D3 定稿细节）。
- R-03 写动作同事务审计脱敏；R-09 零遥测。
- 应用 unit 默认绑定 127.0.0.1（对外一律经面板建站反代）；监听端口创建时冲突检测。
- pip 走第三方索引无 sha256 可言 → 版本钉死经 requirements.txt + 任务失败原因如实呈现；
  不虚构校验能力。
- 删除应用绝不静默删代码目录（保留可选 + 明示文案）。

## 五、容器只读排查（取证结论与自查步骤）

**现象**：admin 账号登录，容器页面有容器清单但无操作列。

**代码链路核验（本轮实查）**：
- 后端 13 个写端点在位（api/dockermgmt/routes.go:79-91：container start/stop/restart/pause/
  unpause/delete、image pull/delete、volume/network delete、compose up/down/restart）；
  读接口两角色开放 → 有清单与角色无关。
- 前端操作列仅 admin 渲染（views/docker/index.vue:463/586/612/665/714，authStore.isAdmin 驱动，
  readonly 整列不渲染是 ADR-018 设计行为）；isAdmin = currentUser.role === 'admin'
  （stores/auth.ts:23）；后端下发 RoleAdmin="admin"（store/auth/models.go:20）——链路无缺陷。
- **「有列表无操作列」恰是只读角色的前端呈现**。

**结论候选与自查步骤**：
1. 最可能：该登录账号实际角色为 readonly——「用户与安全」页用户列表核对角色列；
   用确认是 admin 的账号登录容器页即可看到操作列（启动/停止/重启/日志等）。
2. 次可能：登录会话对应的不是自认为的那个账号（多账号时）——同上核对。
3. 若核对角色列确为 admin 仍无操作列：浏览器 F12 → Network → 刷新 → 找
   `session/current` 响应，把 JSON 里 role 字段的值反馈给我 → 若 role=admin 而无操作列
   则成立前端缺陷，另开修复轮（本轮范围外）。

**处置**：候选 1/2 成立 → 面板能力无缺失，容器侧零开发闭环；候选 3 成立 → 缺陷修复轮
（本轮流水线外或并入下一批）。

## 六、范围边界（本轮不做，登记后续）

- 容器侧新增能力（一键部署 compose 应用、镜像构建等）——只读问题若证实为账号/环境因素，
  容器侧本轮零改动。
- Python 多版本共存（pyenv / 多版本轨道）——python3 单版本起步（沿 PHP 多版本 OUT 先例）。
- 应用市场 / 模板一键部署；Celery/worker 类多进程拓扑（单应用单 unit 起步）。
- nim-proxy 本身的部署实施操作（面板能力就绪后用户自行部署，或另行协助）。

## 七、验收口径（供 Gate 1 / Gate 2）

- env-mgmt Python 环境：env_type 九值四方同值（池/api/PageSpec/前端 enums）+ 迁移 +
  fake 替身测试（代配/检测/装卸/版本探测）。
- app-mgmt：接口三段式 + DTO 可空如实 + 错误码族（新 APP 码族，D3 定稿）+ 审计动作码 +
  任务四态机与 RecoverInFlight；生命周期链路（创建→venv→依赖→unit→启停→日志→删除→建站联动）
  fake 全覆盖注入测试。
- 契约一致性：app-mgmt 消费 env-mgmt 依赖就绪度读契约 + site-mgmt 建站 OHS 消费登记
  双向落档；service-mgmt 重叠呈现口径 D1 裁定落档。
- 编译门控：go build/vet/test -race + vue-tsc/vite build/vitest + linux 交叉 + gofmt + dist 同步。
- 实机冒烟（Linux 目标环境）：venv 部署 FastAPI 应用 → systemd 拉起 → 建站反代可达 →
  日志可见 → 卸载干净（unit 撤除、代码目录按选择保留）；python3 缺失时引导安装链路。
