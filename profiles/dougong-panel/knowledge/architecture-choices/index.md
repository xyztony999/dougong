<!-- kb-generated: 2026-11-12 | sources: 1 | assumptions: 8 | open: 0 -->

# architecture-choices 知识库

> 由 /profile kb 从 workspace/00-architecture/idea.md §4.1（模块需求）/ §5（架构设计）蒸馏生成（2026-11-12）。

## 领域划分表

| 领域 | 职责范围 | 对应 internal 包 | 里程碑 | INDEX 模块 |
|------|---------|----------------|--------|-----------|
| 账户与安全基座 | 初始化引导、登录（密码+TOTP）、会话/CSRF/锁定、RBAC、审计日志 | auth、store、api（middleware） | M1 | account-auth |
| 面板基座 | 入口/配置/优雅退出、路由、安装脚本 | cmd/dougong、config、api | M1 | panel-core |
| 系统监控 | 实时指标（CPU/内存/磁盘/网络/负载/TOP 进程）+ 历史趋势（环形缓冲） | metrics | M1 | monitoring |
| 服务管理 | systemd 服务列表/状态/启停/重启/自启开关 | sysd | M1 | service-mgmt |
| 文件管理 | 浏览/上传下载/在线编辑/权限/压缩/安全路径层 | files | M2 | file-manager |
| 网站管理（核心卖点） | 存量扫描导入、站点编辑写回（raw 保留）、三模板、ACME 证书、配置编辑器、日志尾部 | nginxx（核心资产）+ api/sites | M3 | site-mgmt |
| 数据库管理 | MySQL/MariaDB 实例发现与库/用户管理；Redis 状态/慢日志/key 浏览 | dbmysql、dbredis | M4 | database-mgmt |
| Web 终端 | xterm.js ↔ WebSocket ↔ PTY | terminal | M5 | terminal |
| 防火墙 | ufw/firewalld 自动识别、端口规则管理、监听端口对照 | firewallx | M5 | firewall |
| 容器管理 | 接管已有 Docker daemon：容器/镜像/卷/网络 + Compose 项目（label 识别） | dockerx | M6 | docker-mgmt |
| 计划任务与备份 | cron CRUD/校验/日志/手动触发；备份任务与保留轮转 | cronx、backupx | M7 | cron-backup |

> 总体架构：单体 Go 服务 `dougong`（HTTP API + WebSocket + 内嵌前端），分层 api → service → adapter（隔离发行版与外部软件差异）→ store(SQLite)；详见材料 §5.1/§5.2 目录结构。

## 栈版本表

| 层 | 选型 | 版本 | 备注 |
|----|------|------|------|
| 后端语言 | Go | 材料未锁定（工程化时定） | 单静态二进制 |
| Web 框架 | gin | 未锁定 | 中间件生态全 |
| 存储 | modernc.org/sqlite | 未锁定 | 纯 Go 免 CGO |
| 系统指标 | shirou/gopsutil/v3 | v3 | |
| systemd | godbus/dbus | 未锁定 | 避免 shell 拼接 |
| Docker | docker/docker 官方 Go SDK | 未锁定 | Compose 经 docker compose CLI（ADR-013） |
| ACME | go-acme/lego | 未锁定 | 证书申请与续期 |
| 终端 PTY | creack/pty + gorilla/websocket | 未锁定 | |
| 日志 | 标准库 log/slog | 随 Go 版本 | 结构化日志 |
| 前端框架 | Vue 3 + TypeScript + Vite | 未锁定 | |
| 状态管理 | Pinia | 未锁定 | |
| UI 库 | Naive UI | 未锁定 | TS 友好、轻量 |
| 前端关键组件 | xterm.js / monaco-editor / echarts | 未锁定 | 终端 / 配置编辑 / 监控图 |

> 版本号材料均未锁定（属工程化缺口，非假设）—— scaffolding 阶段以 go.mod / package.json 实锁为准。
