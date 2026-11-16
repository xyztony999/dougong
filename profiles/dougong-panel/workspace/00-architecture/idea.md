# 想法（孵化种子）

# 斗拱 Dougong 项目计划书

> 轻量级、开源、可接管存量环境的服务器运维面板
> Slogan：**嵌入你的服务器，而不是重装它。**
> 状态：规划完成，待进入开发阶段 · 2026-11-12（项目定名：斗拱 / Dougong）
> 本文档为自包含项目计划，可直接作为项目管理工具的实施输入。

---

## 1. 项目背景与动机

现有服务器大量使用宝塔面板，存在三个核心痛点：

| 痛点 | 说明 |
|------|------|
| 强制全新安装 | 宝塔要求在干净系统上安装，无法接管服务器上已运行的 nginx 站点、已装的 MySQL/PHP 等环境；存量服务器迁移成本极高 |
| 功能臃肿 | 付费推广位、应用商店、营销弹窗等与运维无关的功能捆绑，资源占用高 |
| 闭源与安全信任问题 | 闭源无法审计，历史上有多个已收录 CVE（含 2020 年 phpMyAdmin 未授权访问大规模事件、2026 年 CVSS 9.1 命令注入）与数据上报争议，详见第 9.3 节 |

**项目目标**：全量重写一个轻量化开源面板，保留宝塔的核心实用功能（网站/文件/数据库/监控/终端/防火墙/计划任务/备份/容器），同时做到：

1. **可接管存量环境**——装上就能管理已有的 nginx、MySQL、Redis、systemd 服务，不需要重装任何东西；
2. **轻量**——单二进制、内存占用 < 60MB、无冗余功能；
3. **可信**——GPL-3.0 开源、零遥测零回传、所有变更可审计、配置修改可回滚。

## 2. 已确认的决策记录

| # | 决策项 | 结论 | 理由 |
|---|--------|------|------|
| D1 | 后端技术栈 | **Go** | 单静态二进制（前端 go:embed 内嵌）、内存占用低、交叉编译方便；nginx/系统管理生态成熟 |
| D2 | 前端技术栈 | **Vue 3 + TypeScript + Vite**，UI 库 Naive UI | 中文开源社区贡献者对 Vue 最熟悉；Naive UI TS 友好、轻量 |
| D3 | 许可证 | **GPL-3.0** | 防止云厂商闭源商用 fork；面板类项目惯例（1Panel 同款） |
| D4 | 产品定位 | **开源社区项目** | 仓库按开源标准搭建（CI、贡献指南、发布流程）；支持多用户 + RBAC |
| D5 | 核心卖点 | **接管已有环境** | 直接补上宝塔最大痛点；区别于 1Panel 等自带应用商店路线 |
| D6 | MVP 范围 | 全核心功能：网站管理+反代、文件管理器、监控+服务管理、MySQL/MariaDB + Redis、Web 终端、防火墙、计划任务、备份、容器管理（Docker） | 用户确认为完整核心集，按里程碑分期交付 |
| D7 | 数据库模块范围 | MVP：MySQL/MariaDB + Redis；**PostgreSQL 放二期** | 控制工作量，聚焦最常见组合 |
| D8 | 目标平台 | 主流 systemd 发行版：Debian 11+、Ubuntu 20.04+、RHEL 9 系（Rocky/Alma/CentOS Stream） | 覆盖绝大多数存量服务器；非 systemd（Alpine 等）不做 |
| D9 | 部署形态 | 单二进制 + 可审计安装脚本（下载二进制 + systemd unit） | 无运行时依赖；安装脚本每一行可读 |

## 3. 差异化定位

1. **存量接管（核心壁垒）**
   - 扫描 nginx 配置目录（conf.d / vhost.d / sites-enabled）→ 解析为结构化站点对象 → 面板内可视化编辑 → 重新生成配置；
   - 解析不了的指令保留为 raw 块**原样写回**，保证 round-trip 不破坏现有配置；
   - 所有配置写入走固定安全流程：**快照备份 → 修改 → `nginx -t` 校验 → 失败自动回滚**；
   - 同理识别系统上已装的 MySQL/MariaDB/Redis/PHP-FPM 实例、systemd 服务和 Docker daemon（已有容器、镜像、Compose 项目自动纳入管理），一律接管而非要求重装。
2. **零遥测零回传**：无 phone-home、无强制账号绑定、无广告/推广位；更新检查默认关闭且可完全禁用；程序自身不发起任何外部请求（除用户主动触发的 ACME 证书申请）。
3. **单二进制部署**：前端资源内嵌，SQLite 单文件存储（纯 Go 驱动），目标常驻内存 < 60MB。

## 4. 功能需求清单（MVP）

### 4.1 模块需求

**M1 账户与安全基座**
- 初始化引导：首次访问设置管理员密码（或 CLI 生成）
- 登录：密码 + TOTP 两步验证（可选开启）；登录失败锁定（如 5 次失败锁 15 分钟）
- 会话管理：过期时间、强制下线；全站 CSRF token
- 多用户 + RBAC（管理员/只读两类角色即可起步）
- 审计日志：记录所有变更类操作（谁、何时、改了什么、结果）

**M2 系统监控与服务管理**
- 实时监控：CPU（总/核）、内存、磁盘（容量与 IO）、网络流量（上下行）、负载、TOP 进程
- 历史趋势：最近 1h/24h 曲线（进程内环形缓冲，不落库或轻量落库）
- systemd 服务列表/状态/启停/重启/开机自启开关
- 系统信息概览：发行版、内核、运行时长、主机名

**M3 文件管理器**
- 浏览（列表/图标）、目录导航、面包屑
- 上传（多文件/拖拽/分块大文件）、下载、在线编辑（Monaco 编辑器，语法高亮）
- 新建/重命名/移动/删除（进回收站或二次确认）
- 权限与属主修改（chmod/chown）
- 压缩/解压（tar/tar.gz/zip）
- 安全约束：路径白名单（默认 `/`，可配置排除 `/proc`、`/sys` 等），所有操作经符号链接解析与前缀校验，防目录逃逸

**M4 网站管理（核心卖点）**
- **存量导入**：扫描 nginx 配置 → 解析 server 块 → 站点列表（域名、监听端口、根目录、证书状态、反代上游）；UI 内编辑后写回，raw 块保留
- 新建站点模板：静态站点 / PHP-FPM（对接已有 php-fpm）/ 反向代理
- SSL 证书：ACME（Let's Encrypt 等）HTTP-01 / TLS-ALPN 申请、部署、自动续期；已有证书识别与导入
- 反向代理管理：上游地址、Host 头、WebSocket 透传开关
- 配置编辑器：Monaco 直接编辑 nginx 配置（保存前自动 `nginx -t`）
- 站点级操作：启停（enabled 开关）、查看访问/错误日志尾部、重载 nginx

**M5 数据库管理**
- MySQL/MariaDB：实例发现（socket/TCP/凭据配置）、数据库与用户 CRUD、用户授权、root 密码修改、备份导出（mysqldump）/导入、进程列表、状态概览
- Redis：实例发现、状态概览（info）、慢日志、密码设置、key 浏览（只读 + TTL 查看，MVP 不做复杂数据编辑）

**M6 Web 终端**
- 浏览器 xterm.js ↔ WebSocket ↔ 服务端 PTY
- 以面板运行用户（通常 root）身份，可选指定系统用户
- 复用面板会话鉴权，连接超时与并发限制

**M7 防火墙**
- 自动识别 ufw / firewalld（均无则提示）
- 端口规则列表/添加/删除；面板自身端口保护提示
- 端口开放状态展示（监听端口列表对照）

**M8 计划任务与备份**
- cron 管理：CRUD、表达式校验、执行日志尾部、手动触发
- 备份任务：站点目录 + 数据库 → 本地目录 / 远程（MVP：本地 + 可选 rsync 目标）；保留份数轮转

**M9 容器管理（Docker）**
- 连接本机已有 Docker daemon（`/var/run/docker.sock`），**接管而非重装**——与项目核心哲学一致
- 容器：列表/状态/启停/重启/删除、日志尾部与实时跟踪、资源占用、（可选二期）容器内终端
- 镜像：列表/拉取/删除；卷与网络：列表与基本操作
- Compose：通过容器 label 自动识别已有 Compose 项目，支持 up/down/restart/logs（调用 `docker compose` CLI，参数数组方式，经 label 定位工作目录与配置文件）
- 明确边界：只做已有环境的管理，**不做应用商店/一键部署**（保持与 1Panel 路线的区别）

### 4.2 非目标（明确不做 / 二期）

- PostgreSQL 管理（二期）
- PHP 版本安装与切换、应用商店、一键部署应用（不做，避免重回宝塔臃肿路线；容器管理仅限"接管已有 Docker"，不含应用商店）
- 多服务器集中管理（二期）
- 非 systemd 发行版、Windows/macOS（不做）
- 面板自身在线自动升级（MVP 仅提供版本检查开关，默认关闭）

## 5. 架构设计

### 5.1 总体架构

单体 Go 服务 `dougong`：HTTP API + WebSocket + 内嵌前端静态资源，一条 systemd 服务运行。

```
浏览器 (Vue3 SPA)
   │  REST + WebSocket (TLS)
   dougong (Go 单二进制)
   ├─ api 层：路由 / 中间件（鉴权、CSRF、审计、限流）
   ├─ service 层：业务逻辑
   ├─ adapter 层（隔离发行版与外部软件差异）：
   │    nginxx    → 配置解析/生成/nginx -t/重载
   │    sysd      → systemd（dbus）
   │    dbmysql   → MySQL/MariaDB 客户端
   │    dbredis   → Redis 客户端
   │    firewallx → ufw / firewalld 适配器
   │    dockerx   → Docker daemon（官方 Go SDK）+ docker compose CLI
   │    files     → 文件操作（安全路径层）
   │    pkgmg     → apt / dnf 包管理差异
   └─ store：SQLite（配置、用户、审计、任务）
```

### 5.2 目录结构

```
dougong/
├─ cmd/dougong/main.go         # 入口：配置解析、服务启动、优雅退出
├─ internal/
│  ├─ api/                     # gin 路由、middleware、ws 升级、DTO
│  ├─ auth/                    # 会话、TOTP、RBAC、登录锁定
│  ├─ store/                   # modernc.org/sqlite、迁移
│  ├─ nginxx/                  # ★ 指令树解析器 + 生成器 + 存量导入（项目核心）
│  ├─ sysd/                    # systemd 管理
│  ├─ metrics/                 # gopsutil 采集 + 环形缓冲
│  ├─ dbmysql/  dbredis/       # 数据库管理
│  ├─ firewallx/               # ufw/firewalld
│  ├─ dockerx/                 # Docker 容器/镜像/卷/网络/Compose 管理
│  ├─ files/                   # 文件管理（含安全路径）
│  ├─ terminal/                # ws ↔ PTY
│  ├─ cronx/  backupx/         # 计划任务、备份
│  └─ config/                  # 面板自身配置
├─ web/                        # Vue3 前端
│  └─ src/views/{dashboard,files,sites,db,containers,terminal,firewall,cron,audit,settings}
├─ install.sh                  # 可审计一键安装
├─ Makefile  .goreleaser.yaml  # 构建/交叉编译/发布（内嵌前端、签名、checksums）
├─ LICENSE  README.md  CONTRIBUTING.md
└─ .github/workflows/ci.yml    # lint + test + 前端 build + 集成测试
```

### 5.3 关键库选型

| 用途 | 选型 | 备注 |
|------|------|------|
| Web 框架 | gin | 社区熟悉度高，中间件生态全 |
| SQLite | modernc.org/sqlite | 纯 Go 免 CGO，交叉编译无痛 |
| 系统指标 | shirou/gopsutil/v3 | CPU/内存/磁盘/网络/进程 |
| systemd | godbus/dbus | 避免 shell 拼接 |
| Docker | github.com/docker/docker/client（官方 SDK） | 容器/镜像/卷/网络 API；Compose 经 `docker compose` CLI（参数数组） |
| ACME | go-acme/lego | 证书申请与续期 |
| 终端 PTY | creack/pty + gorilla/websocket | |
| 日志 | 标准库 log/slog | 结构化日志 |
| 前端 | Vue3 + Vite + Pinia + Naive UI | |
| 前端关键组件 | xterm.js（终端）、monaco-editor（配置编辑）、echarts（监控图） | |
| nginx 配置解析 | **自研**（词法扫描 → 指令树） | 无维护良好的 Go 库；这是项目核心资产，需重点测试 |

### 5.4 关键技术设计

**nginx 配置 round-trip（最核心）**
- 解析：词法扫描器（处理引号、变量、注释、正则 location）→ 生成通用指令树（`Directive{Name, Args, Children}`）
- 语义层：从指令树提取 server 块 → 站点对象（server_name、listen、root、ssl_cert、location 列表、proxy_pass 等）
- 写回：站点对象 → 重新序列化指令树；**未识别指令/块保留为 Raw 节点按原文本输出**
- 写入流程：备份到 `conf.backup/时间戳/` → 写新配置 → `nginx -t` → 失败恢复备份并报错 diff
- 测试基准：收集真实世界复杂配置样本（含 if/try_files/正则 location/变量/map 等）验证"解析→生成→再解析"语义等价

**安全路径层（文件管理）**
- 所有用户提供的路径：`filepath.EvalSymlinks` 解析真实路径 → 白名单前缀校验 → 再操作；防止 `../` 与符号链接逃逸
- 上传用临时文件 + rename；大文件分块

**命令执行纪律**
- 一律 `exec.Command(name, args...)` 参数数组；**全项目禁止** `sh -c` 拼接用户输入（代码审查项 + CI grep 检查）

**面板自身安全**
- 默认监听 `0.0.0.0:随机高位端口`，支持配置仅 localhost / 指定地址
- 可选安全入口路径（随机 URL 前缀）；建议但强制 HTTPS（自签或导入证书）
- CSRF：同源校验 + token；API 鉴权用 HttpOnly Cookie 会话（不用 localStorage 存 token）

## 6. 实施里程碑

> 每个里程碑完成即是一个可运行、可演示的增量；顺序执行。

### M1 — 骨架与安全基座
范围：仓库脚手架（目录、LICENSE、CI、README/CONTRIBUTING）、go:embed 前端、登录（密码+TOTP）、会话/CSRF/锁定、审计日志、监控仪表盘、systemd 服务管理、安装脚本。
**验收标准**：一键脚本安装到干净 Ubuntu → 浏览器完成初始化 → 登录（含 TOTP）→ 看到实时监控 → 启停一个 systemd 服务；`go test ./...` 与 CI 全绿；二进制 < 30MB、常驻内存 < 60MB。

### M2 — 文件管理器
范围：4.1 模块 M3 全部功能。
**验收标准**：对真实目录完成浏览/上传 500MB 文件/在线编辑保存/压缩解压/改权限；路径逃逸测试用例（`../`、符号链接指向 `/etc`）全部被拦截并记审计。

### M3 — 网站管理（核心卖点，工作量最大）
范围：存量站点扫描导入、站点对象编辑、raw 块保留、新建站点三模板、ACME 证书、配置编辑器、日志尾部查看。
**验收标准**：在装有 5+ 真实复杂站点（含反代、PHP、SSL、多 location）的 nginx 上安装面板 → 导入全部站点 → 在面板内修改某站点反代地址并保存 → `nginx -t` 通过且站点生效；其余站点配置 byte-level 或语义级无变化；构造一个会令 nginx -t 失败的修改 → 自动回滚成功；round-trip 测试样本集 ≥ 20 个真实配置全部通过。

### M4 — 数据库管理
范围：MySQL/MariaDB 管理 + Redis 管理（见 4.1 模块 M5）。
**验收标准**：发现存量 MySQL 实例 → 建库/建用户/授权/改密码/备份导出再导入成功；Redis 状态与慢日志展示正确；所有操作可审计。

### M5 — 终端与防火墙
范围：Web SSH、ufw/firewalld 管理（见 4.1 模块 M6/M7）。
**验收标准**：浏览器打开终端可交互（vim/top 正常渲染）；面板会话过期后终端连接断开；ufw 与 firewalld 环境各自通过规则增删测试。

### M6 — 容器管理（Docker）
范围：见 4.1 模块 M9——接管已有 Docker daemon，容器/镜像/卷/网络管理与 Compose 项目接管。
**验收标准**：在已运行多个容器与 Compose 项目的服务器上安装面板 → 全部容器与 Compose 项目自动识别（无需手动登记）→ 启停/重启/日志查看/镜像拉取删除全部可用；未安装 Docker 的机器展示安装引导而非报错。

### M7 — 计划任务、备份与发布
范围：cron 管理、备份任务（见 4.1 模块 M8）、文档、goreleaser 发布流水线（多平台二进制 + checksums + 签名）。
**验收标准**：创建 cron 任务并手动触发看到日志；备份任务产出可用的站点+数据库备份并按保留数轮转；GitHub Release 产出 Debian/Ubuntu/RHEL 可用的安装产物。

## 7. 安全设计要求（贯穿所有里程碑的硬约束）

1. 默认最小暴露：随机高位端口、可选安全入口路径、登录锁定、会话过期
2. 禁止 shell 字符串拼接执行（CI 检查）
3. 配置写入必须"备份 → 修改 → 校验 → 失败回滚"
4. 所有变更操作写审计日志
5. 零外部回传（除用户主动 ACME 申请）；更新检查默认关闭
6. 依赖安全：CI 集成 `govulncheck` + 前端 audit
7. 处理器（handler）层对全部输入做服务端校验，不信任前端

## 8. 验证与质量保障

- **单元测试**：nginx round-trip 样本集（≥20 真实配置）、路径安全、权限/会话、cron 表达式
- **集成测试**：GitHub Actions 在 Ubuntu 容器内安装真实 nginx/MySQL/Redis/systemd 跑 API 级测试
- **前端**：vitest 单测 + Playwright 冒烟（登录→文件→站点关键路径）
- **本地开发**：`make dev`（Vite dev server 代理 Go 后端）
- **发布验收**：测试 VPS 上从宝塔存量环境直接安装接管（不改坏任何站点）——作为 M3 后的专项验收场景

## 9. 竞品与安全调研（已完成核实，2026-11-12）

### 9.1 竞品格局

| 项目 | 技术栈 / 许可证 | 状态 | 与本项目的关系 |
|------|----------------|------|----------------|
| 宝塔 / aaPanel | Python，闭源 | 活跃 | 被替代对象，痛点见第 1、9.3 节 |
| [1Panel](https://github.com/1Panel-dev/1Panel) | Go + Vue，GPL-3.0 | 万星级头部，活跃 | 最接近的开源竞品；但建站强依赖应用商店安装的 OpenResty 容器，**不能管理宿主机已有 nginx/MySQL** |
| [CasaOS](https://github.com/IceWhaleTech/CasaOS) | Go + Vue，Apache-2.0 | 约 3.7 万星；**2024 年底后无新稳定版，重心转商业版 ZimaOS，社区视为维护模式** | 家庭/NAS 场景，Docker 中心，服务器运维弱 |
| [DPanel](https://github.com/donknap/dpanel) | Go + React，**自定义协议（非标准 OSI 开源）** | 活跃，官方称内存约 20MB | Docker/容器管理面板，天然接管已有容器，但不管宿主机 LNMP |
| [mdserver-web](https://github.com/midoks/mdserver-web) | Python(Flask)，Apache-2.0 | 活跃（约 4.5k 星） | 类宝塔复刻，环境需经其插件自装；曾曝 [CVE-2026-41315 未授权 RCE](https://github.com/midoks/mdserver-web/security/advisories/GHSA-3h92-g9hr-xc25)（印证本项目第 7 节安全设计的必要性） |
| [耗子面板 AcePanel](https://github.com/tnb-labs/panel) | Go + Vue，BSD-3 | 约 3k 星，极活跃 | 精神上最接近的国产项目（单文件、轻量、免费），但仍是"自带一套 LNMP"模式，不做存量接管 |
| [Webmin / Virtualmin](https://webmin.com) | Perl，BSD-3，1997 年至今 | 维护中 | **唯一能原生读写系统已有配置的传统面板**（见 9.2），但技术栈老旧、UX 过时；2019 年曾发生源码供应链投毒后门（CVE-2019-15107），印证本项目对发布链完整性的要求 |
| Cockpit（Red Hat） | Python/C/JS | 发行版自带 | 系统级管理（systemd/存储/网络/终端），天然作用于已装软件，但无建站能力 |
| CloudPanel / HestiaCP / ISPConfig / CyberPanel | 各异 | 活跃 | 传统虚拟主机派，均要求"用我装的" |

### 9.2 关键结论：存量接管是真实空白

- **主流建站面板（宝塔、1Panel、mdserver-web、耗子、CloudPanel、HestiaCP 等）无一支持导入已有 nginx 配置或接管 apt/yum 安装的服务**。1Panel 社区的通行做法是"停掉系统 nginx → 装面板 OpenResty → 手工重建 server 块"（迁移而非接管）；宝塔官方论坛版主原话"无法管理以前安装的环境"。
- 真正能接管存量的只有两派：**Webmin/Virtualmin**（能，但 Perl 老旧、UX 过时）和**容器派/系统派**（DPanel/Portainer/Cockpit，作用于容器或服务层，无建站能力）。
- 宝塔与 mdserver-web 的"一键迁移"均为面板到面板迁移，不是存量环境导入。
- **因此"存量接管 + 现代技术栈（Go+Vue）+ 零遥测 + 单二进制"的组合目前没有占位者**，本项目核心卖点成立。与 1Panel 的应用商店模式正交，可共存；容器管理模块走"接管已有 Docker"路线（与 DPanel 同哲学），不做应用商店。

### 9.3 宝塔面板安全问题记录（动机佐证）

**确凿（有 CVE 编号或官方/云厂商通告）**：
- 2020-08 phpMyAdmin 未授权访问（影响 7.4.2 等）：管理员近期登录过 pma 后任何人可免鉴权以 root 进库，当晚出现批量扫描脱库（[阿里云预警](https://www.aliyun.com/notice/116490)、[华为云通告](https://www.huaweicloud.com/notice/2018/20200824104940965.html)）
- CVE-2025-12914（SQL 注入）、CVE-2026-101007/8/9（命令注入，其中 101008 CVSS 9.1）、CVE-2026-101010/11（SQL 注入）、CVE-2026-29859（aaPanel 文件上传 RCE）；据 OpenCVE，2026 年这批披露时厂商对联系不回应
- 2024 堡塔云 WAF 未授权访问 + SQL 注入（官方承认旧版漏洞并修复）

**争议（第三方代码分析 vs 官方否认，无权威定性）**：
- 2022-05 社区逆向分析指 `site_task.py` 每小时向宝塔服务器上传用户域名/域名-IP 对应/请求日志，宝塔官方否认收集隐私。本项目"零遥测"承诺正是对此类信任问题的直接回应。

**不确定项**：各面板 star 数与资源占用数字存在时点与量纲差异（官方宣传 vs 第三方实测），上表仅给数量级，发布前对比页需以实测为准。

## 10. 项目元信息

- 项目名：**斗拱 / Dougong**（GitHub 仓库名 `dougong`，二进制名 `dougong`，systemd 服务 `dougong.service`）
- 命名说明：斗拱是中国古建筑的承重连接构件——连接屋顶（流量）与立柱（系统服务），轻巧、层层咬合、嵌入已有结构而不破坏它，与"接管存量环境"的产品哲学一致。2026-11-12 经 GitHub 查重确认命名空间干净（仅小型 3D 建筑 demo 与个别小 crate，无知名软件项目占用）；Slogan："嵌入你的服务器，而不是重装它。"
- 仓库结构：见 5.2；GitHub 开源，GPL-3.0
- 语言：代码英文注释/commit，README 双语（中/英）
- 交流渠道、文档站：M7 阶段建设
