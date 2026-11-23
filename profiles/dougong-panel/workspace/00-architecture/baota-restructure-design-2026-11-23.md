# 跨模块设计：宝塔式「网站 / 容器」二分重组（ADR-033 · D1~D4 设计依据）

| 属性 | 值 |
|------|-----|
| 版本 | v1.1 |
| 状态 | Gate 1 已确认（2026-11-23）；本版按用户拍板把 Node 托管修订为 PM2 |
| 最后更新 | 2026-11-23（v1.1：Node 项目改用 PM2 托管，第四章与相关小节修订；v1.0 为 systemd 托管方案，已被取代） |
| 决策来源 | d0-clarification-baota-restructure-2026-11-23.md（用户六项拍板） |
| 涉及模块 | app-mgmt（运行时项目托管）/ site-mgmt（网站管理页承载）/ docker-mgmt（容器管理）/ env-mgmt（Node 环境含 PM2 安装与就绪度） |
| 新增编码 | S-62 面板应用、S-62-01~05、S-50-05；错误码 DK_028~032、APP_014~016；迁移 018 |

> 本文是本轮跨模块重组的单一设计依据：各模块 BA/AA/FL/PRD/API/PageSpec 的增量小节均指向本文对应章节。
> 模块文档给出契约细节（接口字段、页面规格），本文给出决策与边界。

## 一、决策来源与设计落点

| D0 | 用户拍板 | 本文落点 |
|----|---------|---------|
| Q1 | Node 项目托管本轮一起做 | 第四章 |
| Q2 | 拆散并删除「应用管理」独立菜单 | 第二、三章 |
| Q3 | 容器管理新增「应用」分区 | 第五章 |
| Q4 | Compose 生命周期缺口三项一起补 | 第六章 |
| Q5 | 先热修 v0.3.1，重组另发 v0.4.0 | v0.3.1 已于 2026-11-23 发布；本轮目标 v0.4.0 |
| Q6 | 本地提交，Gate 2 确认后推送 | 流程约束，不涉及设计 |

## 二、信息架构（菜单与路由）

**原则**：按部署形态二分——非容器化部署的一切入口归「网站管理」，容器化部署的一切入口归「容器管理」（宝塔式）。

| 侧栏菜单 | 路由 | 变更 | 内容 |
|---------|------|------|------|
| 网站管理 | /sites | 页面改三分栏 | 分栏「网站」（既有 nginx 站点：静态 / PHP / 反向代理）/「Python 项目」/「Node 项目」 |
| SSL 证书 | /certificates | 不变 | — |
| 容器管理 | /docker | 新增「应用」页签 | 容器 / **应用** / Compose / 镜像 / 卷 / 网络 |
| 应用管理 | /app | **菜单项删除** | 路由保留为重定向 → /sites?tab=python（旧书签不 404） |

- 分栏由地址栏参数 `tab` 持久化（`site` / `python` / `node`），可分享定位、刷新不丢。
- 监控总览资源条（并行会话提交 bd25386 引入）的「应用」计数入口改为「项目」→ /sites?tab=python，计数口径不变（Python + Node 项目总数）。
- 页面图标、菜单排序不变（网站管理 Order 沿用、容器管理 Order 800 沿用）。

## 三、模块职责修订

### 3.1 app-mgmt —— 运行时项目托管

- **定位收窄**：承载「非容器化的运行时项目」托管，现为 Python 与 Node 两种运行时。
- **OUT**：容器运行形态下线。原因：该形态自 v0.3.0 起不可用（前端不发镜像字段、启停指令不合法、挂载参数丢失），且与 Q2/Q3 拍板方向不一致——容器化部署统一归容器管理。下线内容：`createContainerized`、`container_form.go`、`container_ops.go`、`ComposeCreatorFunc` 装配、前端「容器运行」选项。服务层对 `startMode=container` 返回 APP_014（指引到容器管理创建应用）。
- **IN**：Node 项目托管（PM2 托管进程，第四章）。
- **保留**：建站联动、服务操作、参数编辑、日志、任务跟踪、删除——对两种运行时同等适用。
- **数据**：见第七章。既有 app 行不动，缺省视为 Python 项目。

### 3.2 site-mgmt —— 网站管理页承载

- **无接口变更**（api-site-mgmt.md 不升版）。页面级组合：网站管理页以分栏承载 app-mgmt 的 Python / Node 项目面板（嵌入其列表与创建入口，不在 site-mgmt 侧复制接口）。
- 「应用」分区（docker-mgmt）的建站消费 site-mgmt 既有反向代理建站能力（与 app-mgmt 建站联动同一通道，第十一章）。

### 3.3 docker-mgmt —— 容器管理

- 新增「应用」页签（第五章）：面板生成的单应用编排的集中视图与入口。
- Compose 页签补齐生命周期（第六章）。
- 归属标签语义收窄：`dougong.app=<名称>` 自本轮起**仅**由「应用」创建通道注入；经「新建 Compose 项目」粘贴 yml 创建的项目不再注入该标签（修订 D-11 / ⚠️A22）。

### 3.4 env-mgmt —— Node 环境（含 PM2）

- **Node 环境增补 PM2**：安装 / 升级 Node 环境时一并全局安装 PM2（npm 包），并注册 PM2 开机自启（Linux：生成 pm2 的 systemd 启动单元，开机时恢复已保存的进程列表）。已装 Node 但缺 PM2 的存量主机，经环境管理的「升级」动作补装（与 v0.3.0 的 pip 补装同一机制）。
- **Node 就绪度探测**（沿 Python 就绪度先例）：node 可用、npm 可用、PM2 可用、node 版本。供 app-mgmt 创建 Node 项目前置校验与界面分层提示。
- 卸载 Node 环境时不动 PM2 进程列表（面板无法判断是否仍有项目依赖），仅如实提示。

## 四、Node 项目托管设计（PM2）

### 4.1 定位与依据

用户拍板：Node 项目「像宝塔那样用 PM2 托管」（Gate 1，2026-11-23）。本章依据 PM2 7.1.2 的实测行为编写（隔离数据目录探测，结论见 4.2）。

- 每个_Node 项目是 PM2 中的一个进程_，进程名 `dougong-app-{名称}`；管理员可在服务器上直接用 `pm2 list` 看到并操作。
- PM2 负责：进程守护（崩溃自动拉起）、状态与资源占用、日志落盘、开机恢复。
- 面板负责：创建（装依赖 → 注册 PM2 进程）、启停 / 重启、开机自启开关、日志查看、参数编辑、建站、删除。

### 4.2 PM2 实测行为（设计依据）

| 行为 | 实测结论 |
|------|---------|
| 启动 | `pm2 start <脚本> --name <名称> --cwd <目录>`；npm 脚本模式 `pm2 start npm --name <名称> --cwd <目录> -- run <脚本名>`，均可正常在线 |
| 状态查询 | `pm2 jlist` 返回 JSON 数组：状态（online / stopped / errored 等）、进程号、重启次数、内存、工作目录、日志文件路径 |
| 启停重启删除 | `pm2 stop / start / restart / delete <名称>` 均正常 |
| 保存与开机恢复 | `pm2 save` 记录全部进程及其**当时的状态**（含已停止的）；`pm2 resurrect` 按保存时的状态恢复——**保存前已停止的进程恢复后仍是停止**，不会被拉起 |
| 环境变量 | 编排文件里的 env 生效；`pm2 restart … --update-env` 可更新 |
| 注册开机自启 | `pm2 startup systemd` 仅 Linux 可用（Windows 报 Init system not found）——本机无法验证，归服务器实机验证 |
| 查询的副作用 | 守护进程不在时，任何 pm2 命令（含 jlist）都会自动拉起守护进程 |

### 4.3 与 Python 项目的同构关系

| 环节 | Python（既有） | Node（本轮） |
|------|---------------|-------------|
| 运行环境来源 | 环境管理 Python 条目 | 环境管理 Node 条目（含 PM2） |
| 前置就绪度 | venv 支持 + pip 可用 | node 可用 + npm 可用 + PM2 可用 |
| 项目内环境 | `.venv` 虚拟环境 | 不建隔离环境（依赖装在项目目录 `node_modules`） |
| 依赖安装 | `pip install -r requirements.txt` | 存在 `package.json` 时 `npm install`；不存在则跳过 |
| 进程托管 | systemd 服务单元 | **PM2 进程**（进程名 `dougong-app-{名称}`） |
| 崩溃自拉起 | systemd `Restart=on-failure` | PM2 `autorestart` |
| 启动方式 | FastAPI 预设 / 自定义命令 | 启动文件 / npm 脚本 |
| 开机自启 | `systemctl enable / disable` | PM2 进程列表的保存状态（见 4.6） |
| 日志 | journalctl | PM2 日志文件（out / error 两路） |
| 参数 / 建站 / 删除 / 任务 | 既有 | 同能力面 |

### 4.4 启动器与启动方式

面板提供一个**启动器脚本**（Node 脚本，内嵌在面板二进制里，首次创建 Node 项目时落到面板数据目录）。PM2 进程的执行目标是启动器，而不是项目文件本身：

1. 启动器读取项目目录下的 `.env`，把键值注入进程环境——因此环境变量仍然只在 `.env` 一处维护，面板不需要把 `.env` 的内容（可能含密钥）复制到任何别的文件里；每次进程启动都会重新读取，改完参数重启即生效。
2. 按启动方式拉起应用：

| startMode | 名称 | 启动器的动作 | 校验 |
|-----------|------|-------------|------|
| `node_file` | 启动文件 | 在项目目录内加载入口文件（`require`），应用运行在 PM2 直接管理的进程里 | 入口文件为项目目录内的相对路径；禁 `..`、禁绝对路径 |
| `npm_script` | npm 脚本 | 以解析到的 npm 绝对路径执行 `npm run <脚本名>` | 脚本名匹配 `^[A-Za-z0-9_.:-]{1,64}`，缺省 `start` |

3. node 与 npm 用面板在创建时解析到的**绝对路径**（记入应用档案），不依赖守护进程的 PATH——PM2 守护进程在开机时由 systemd 拉起，其 PATH 未必包含 node 的安装位置。
4. 进程参数：工作目录 = 项目目录；fork 模式（单实例）；`autorestart` 开启。

### 4.5 创建流程

前置校验 → 异步任务：

1. **前置校验**（同步）：名称 / 端口占用 / 目录存在（沿 Python）；Node 就绪度缺失（缺 node、npm 或 PM2）→ APP_015 并给环境管理引导；启动方式参数校验 → APP_016。
2. **任务执行**：`npm install`（有 `package.json` 时；失败则任务失败、可重试）→ 落启动器（若不存在）→ `pm2 start` 注册进程 → 未勾选「立即启动」则 `pm2 stop` → `pm2 save` → 登记档案（`runtime=node`）。

### 4.6 服务操作与开机自启

| 操作 | 实现 | 是否触发 `pm2 save` |
|------|------|---------------------|
| 启动 / 停止 / 重启 | `pm2 start / stop / restart <名称>` | 否 |
| 开机自启 **开** | `pm2 start` + `pm2 save` | 是 |
| 开机自启 **关** | `pm2 stop` + `pm2 save` | 是 |
| 删除 | `pm2 delete` + `pm2 save` | 是 |

- **设计要点**：面板只在创建、删除、切换开机自启时执行 `pm2 save`；手动启停不保存。于是开机恢复用的始终是「上一次显式设置开机自启时的状态」——开关是一个**持久设置**，与 Python 项目的 `systemctl enable / disable` 语义等价（例如：开机自启已开启的项目被手动停掉，下次开机仍会启动；已关闭的项目被手动启动，下次开机仍不会启动）。
- **与 Python 项目的差异（需在界面明示）**：开启开机自启会**同时启动**应用，关闭会**同时停止**应用——PM2 只能通过「保存时的运行状态」表达这一设置。
- 开机自启状态 = 读取 PM2 保存文件（`dump.pm2`）中该进程的状态：保存为运行中 = 已开启；不存在或保存为停止 = 已关闭。
- 崩溃自拉起由 PM2 负责（与开关无关）；手动停止的进程 PM2 不会自行拉起。

### 4.7 日志与参数

- **日志**：取 PM2 日志文件尾部（路径来自 `pm2 jlist`：out 与 error 两个文件），两路合并返回（error 路的行带「错误」标记以便区分）。日志文件由 PM2 持续追加，**面板不做日志轮转**（范围外，见第十三章），界面提示日志会随运行增长。
- **参数编辑**：`.env` 整文件安全写入（与 Python 同一管线）；保存后提示「需重启应用生效」。
- **重装依赖**：执行 `npm install`（任务动作与状态机复用），完成后不自动重启。

### 4.8 与「服务管理」页的关系

Node 项目不是 systemd 服务单元，不出现在「服务管理」页；Python 项目不变。Node 项目的全部操作在「网站管理 › Node 项目」分栏完成；服务器命令行上对应的是 `pm2 list / pm2 logs <名称>`。

## 五、容器管理「应用」分区设计

### 5.1 定位

「应用」= 面板生成的**单服务** compose 编排 + 归属标签 `dougong.app=<名称>`，提供单应用视角的创建、启停、日志、删除、建站。手写 yml 的多服务项目仍在「Compose」页签管理。

### 5.2 数据来源（零持久化，沿 D-10）

不建表。应用清单由两路实况合并：

- **落盘扫描**：Compose 落盘根 `/opt/dougong-compose/{名称}/docker-compose.yml`，解析得到镜像、端口映射、服务定义；带 `dougong.app` 标签者为「应用」。
- **容器实况**：按 compose 项目名聚合容器状态 → 运行中 / 已停止 / 部分运行。

### 5.3 应用编排模板（面板生成）

```yaml
services:
  app:
    image: <镜像>
    ports:
      - "127.0.0.1:<主机端口>:<容器端口>"
    environment:            # 可选
      KEY: "value"
    volumes:                # 可选
      - <来源>:<容器内路径>
    restart: <策略>          # 缺省 unless-stopped
    labels:
      - dougong.app=<名称>
```

- **主机端口只绑 127.0.0.1**：与 Python / Node 项目「服务只监听本机、经建站对外暴露」的口径一致（D-app-03）。需要直接对外暴露端口的场景走「容器」页签的创建容器或手写 Compose。
- 名称规则同 Compose 项目名：`^[a-z0-9][a-z0-9-]{0,63}$`（即目录名与 compose 项目名）。
- 端口冲突前置校验（主机端口已被占用 → DK_031）。

### 5.4 操作语义

| 操作 | 语义 | 实现 |
|------|------|------|
| 启动 | 拉起容器 | `compose up -d` |
| 停止 | 停容器、保留容器与目录（便于快速再启动） | `compose stop`（ComposeRunner 动词白名单新增 `stop`） |
| 重启 | 重启容器 | `compose restart` |
| 日志 | 应用日志 | `compose logs`（经既有 Compose 日志通道） |
| 删除 | 停止并移除容器、删除项目目录 | `compose down` + 删除目录；强确认 |
| 建站 | 反向代理到 `127.0.0.1:<主机端口>` | 消费 site-mgmt 反向代理建站能力 |

## 六、Compose 项目生命周期三项（D0 Q4）

| 缺口 | 修法 |
|------|------|
| 零容器时列表不可见 | Compose 列表 = 容器 label 聚合（既有，含面板外项目）∪ 落盘根目录扫描（面板创建的项目）。合并键为项目名；落盘存在但无容器的项目呈现为「未运行」（containers=[]、locatable=true），可直接启动 |
| 项目名被目录占住无法重建 | 承上：同名项目现在可见、可删除；DK_027 文案补充「可在 Compose 列表查看或删除同名项目后重建」 |
| 无删除入口 | 新增「删除项目」：`compose down` + 删除项目目录（仅当目录位于面板落盘根下；面板外项目只 down 不删目录，响应如实标注 `deletedDir=false`）；强确认 |

## 七、数据与枚举

- **迁移 018**：`app` 表新增列 `runtime TEXT NOT NULL DEFAULT 'python'`（取值 python / node）。仅加列，不重建表；`start_mode` 的 DDL CHECK 保留历史值 `container`（服务层拒绝，APP_014）。
- **枚举池**：新增 `app_runtime`（python / node）；`app_start_mode` 增 `node_file`、`npm_script`，`container` 标注「已下线（DDL CHECK 历史值）」；`docker_app_state`（running / stopped / partial）。
- app 档案扩展：`startSpec` 对 Node 承载 `{file}` 或 `{script}`，另记 `nodeBin` / `npmBin` 解析结果（供启动器使用）。PM2 进程名由应用名派生（`dougong-app-{名称}`），不单独存储；PM2 进程列表与开机恢复状态由 PM2 自持，面板不建表。

## 八、接口增量清单（D3 依据）

### 8.1 app-mgmt（api-app-mgmt.md v1.1 → v1.2）

| 接口 | 变更 |
|------|------|
| POST /app/app/create | 请求体 +`runtime`（python \| node，缺省 python）；`startMode` 取值域按运行时区分：python → preset_fastapi \| custom；node → node_file \| npm_script；`container` → APP_014。`startSpec`：node_file=`{file}`、npm_script=`{script}` |
| GET /app/app/list | 条目 +`runtime`；响应 +`pythonReady` / `nodeReady`（nodeReady 含 nodeAvailable / npmAvailable / pm2Available / nodeVersion）；Node 条目另带重启次数与内存占用；可选 query `runtime` 过滤 |
| GET /app/app/task/list、task/detail、service/*、log、params、env、deps、delete、site/link | 不变（对 Node 项目同等适用） |
| 新增错误码 | APP_014 容器运行形态已下线（指引到容器管理）/ APP_015 Node 环境不完整（缺 node、npm 或 PM2）/ APP_016 Node 启动参数非法 |

### 8.2 docker-mgmt（api-docker-mgmt.md v1.1 → v1.2）

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| /docker/app/list | GET | admin+readonly | 应用清单（名称 / 状态 / 容器 / 镜像 / 主机端口 / 是否已建站） |
| /docker/app/create | POST | admin+CSRF | `{name, image, hostPort, containerPort, env[], mounts[], restartPolicy}` → 生成编排、落盘、`up -d` |
| /docker/app/service | POST | admin+CSRF | `{name, op: start\|stop\|restart}` |
| /docker/app/logs | GET | admin+readonly | `?name=&tail=` |
| /docker/app/delete | POST | admin+CSRF | `{name}`：down + 删除目录 |
| /docker/app/site | POST | admin+CSRF | `{name, domains[]}`：反向代理建站到 127.0.0.1:主机端口 |
| /docker/compose/delete | POST | admin+CSRF | `{project}`：第六章删除入口 |
| /docker/compose/list | GET | admin+readonly | 响应不变，数据源并入落盘扫描（零容器项目） |

新增错误码：DK_028 应用不存在 / DK_029 应用名格式非法 / DK_030 应用操作失败（+data.reason）/ DK_031 主机端口已被占用 / DK_032 Compose 项目删除失败（+data.reason）。应用重名复用 DK_027。
新增审计动作：`app_created` / `app_service` / `app_deleted` / `app_site_linked` / `compose_project_deleted`。

### 8.3 site-mgmt / env-mgmt

- site-mgmt：无接口变更。
- env-mgmt：Node 环境安装 / 升级增补 PM2 安装与开机自启注册；新增进程内契约 `NodeReadiness`（供 app-mgmt 消费），无新增 HTTP 接口。

## 九、交互结构（D4 依据）

### 9.1 网站管理（PageSpec site-mgmt v1.2 → v1.3）

- 页面顶部三级分栏：「网站」（默认）/「Python 项目」/「Node 项目」。
- 「网站」分栏：既有站点列表与全部交互原样迁入，零行为变化。
- 「Python 项目」「Node 项目」分栏：原应用管理页按运行时过滤后的同一面板（列表、创建、任务、运维抽屉），创建表单随运行时呈现对应字段。

### 9.2 应用管理（PageSpec app-mgmt v1.0 → v1.1）

- 不再是独立页面，改为网站管理页内嵌面板（按 `runtime` 属性实例化两次）。Node 项目分栏是 PM2 进程视角：状态列呈现 PM2 状态（运行中 / 已停止 / 异常）并附重启次数与内存占用；开机自启开关带说明「开启会同时启动应用，关闭会同时停止应用」。
- 创建表单：去掉「容器运行」选项；Node 运行时下启动方式为「启动文件 / npm 脚本」；表单内说明「由 PM2 托管进程，可在服务器上用 pm2 list 查看」。
- 就绪度提示随运行时：Node 缺 node / npm 时给环境管理引导。

### 9.3 容器管理（PageSpec docker-mgmt v1.1 → v1.2）

- 新增「应用」页签（位于「容器」之后）：列表 + 「创建应用」+ 行内 启动/停止/重启/日志/建站/删除；创建弹窗字段：名称、镜像、主机端口、容器端口、环境变量（多组）、挂载（多组）、重启策略。
- Compose 页签：零容器项目呈现「未运行」；新增行内「删除项目」（强确认，明示是否删除目录）；DK_027 提示更新。

## 十、readonly 负空间（ADR-018）

| 位置 | 可看 | 不渲染 |
|------|------|--------|
| 网站管理三分栏 | 站点列表、项目列表与任务状态 | 创建、删除、启停、重装依赖、建站等全部变更入口；日志内容（沿 app-mgmt ⚠️A8 管理员专属） |
| 容器管理「应用」页签 | 应用清单、日志（观测类） | 创建应用、启停、删除、建站 |
| Compose 页签 | 项目列表（含未运行项目） | 删除项目 |

## 十一、跨模块消费登记

| 消费方 | 提供方 | 契约 | 状态 |
|--------|--------|------|------|
| app-mgmt | env-mgmt | `NodeReadiness`（新增，沿 `PythonReadiness` 先例；含 PM2 可用性） | 本轮新增 |
| app-mgmt | PM2（外部进程管理器，随 Node 环境安装） | 进程注册 / 启停 / 状态 / 日志路径 / 保存 | 本轮新增（适配层 adapter/pmx） |
| docker-mgmt | site-mgmt | 反向代理建站（应用建站；沿 app-mgmt `SiteLinkGateway` 适配先例） | 本轮新增 |
| site-mgmt（页面） | app-mgmt | 页面级嵌入 Python / Node 项目面板（无服务调用） | 本轮新增 |
| ~~app-mgmt~~ | ~~docker-mgmt~~ | ~~Compose 创建通道（容器运行形态）~~ | **本轮下线** |

## 十二、并行会话同步项

- 监控总览资源条「应用」入口 → 「项目」（/sites?tab=python）。
- 登录后默认路由为监控页（bd25386），本轮不改。

## 十三、范围边界（本轮不做）

- PM2 集群模式（多实例负载均衡）、日志轮转（pm2-logrotate）、Node 版本隔离（nvm / 多版本）
- 「应用」分区的多服务编排、编排文件在线编辑、容器 CPU / 内存限制
- Java / Go 等其它运行时的项目托管
- 面板外 Compose 项目（仅容器 label 发现）的目录删除

## 十四、设计决策清单

| 编号 | 决策 | 理由 |
|------|------|------|
| D-app-01 | 「应用管理」菜单删除，app-mgmt 模块保留为运行时项目托管能力 | 用户拍板 Q2；后端模块边界与数据不动，重组发生在界面与入口层 |
| D-app-02 | Node 项目用 PM2 托管进程（用户 Gate 1 拍板）；面板经启动器脚本统一拉起应用、`.env` 仍是环境变量唯一来源；开机自启开关靠「仅在三处显式保存」实现持久语义 | 用户要求与宝塔一致；PM2 实测支持按保存状态恢复；启动器避免把 `.env` 内容复制到别处 |
| D-app-03 | 容器「应用」主机端口只绑 127.0.0.1 | 与 Python / Node 项目「只监听本机、经建站对外」口径一致；直接对外暴露走创建容器 / 手写 Compose |
| D-app-04 | 容器运行形态从 app-mgmt 下线，由 docker-mgmt「应用」分区原生承载 | 该形态自 v0.3.0 起不可用；按形态二分归属更清晰 |
| D-app-05 | `dougong.app` 归属标签仅由「应用」创建通道注入 | 标签语义收窄为「面板应用」，与手写 Compose 项目可区分 |
| D-app-06 | 应用清单零持久化（落盘扫描 + 容器实况） | 沿 docker-mgmt D-10 无表无 SQL；事实源在文件系统与 daemon |
| D-app-07 | Compose 列表并入落盘根目录扫描 | 修复零容器不可见；面板外项目仍仅经 label 发现，行为不变 |
| D-app-08 | 迁移 018 仅加 `runtime` 列，不重建 app 表 | 最小改动；`container` 历史值由服务层拒绝 |
