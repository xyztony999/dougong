# 斗拱 Dougong

**嵌入你的服务器，而不是重装它。** / *Embed into your server, not reinstall it.*

轻量级、开源、可接管存量环境的服务器运维面板——装上就能管理已有的 nginx、MySQL/MariaDB、Redis、systemd 服务与 Docker 容器，不需要重装任何东西。

A lightweight, open-source server panel built to **adopt your existing environment**: point it at the nginx, MySQL/MariaDB, Redis, systemd services and Docker daemon already running on your box — nothing gets reinstalled.

- 单二进制：Go 后端 + Vue 3 前端 go:embed 内嵌，SQLite 单文件存储，零运行时依赖 / Single static binary (Go + embedded Vue 3 SPA, SQLite, no runtime deps)
- 存量接管：nginx 配置扫描导入、round-trip 保真写回（raw 块保留）、已有实例自动发现 / Adopt-in-place: scan & import existing nginx configs with faithful round-trip writes, auto-discover running instances
- 可信：GPL-3.0、零遥测零回传、全变更审计、HttpOnly 会话 + CSRF + RBAC / Trustworthy: GPL-3.0, zero telemetry, full audit trail, HttpOnly sessions + CSRF + RBAC

## 功能 / Features

| 模块 | 说明 |
|------|------|
| 账户与安全 / Auth | 初始化向导、密码+TOTP、登录锁定、会话治理、审计日志 |
| 面板基座 / Core | 面板配置、安全入口路径、菜单、证书、版本信息 |
| 监控 / Monitoring | CPU·内存·磁盘·网络·负载实时与 24h 趋势、TOP 进程 |
| systemd 服务 / Services | 服务列表、启停重启、开机自启（dbus） |
| 文件管理 / Files | 浏览、上传下载、Monaco 在线编辑、回收站、压缩解压、权限属主 |
| 网站 / Sites | **存量 nginx 站点导入与可视化编辑**、ACME 证书、反向代理、配置编辑器 |
| 数据库 / Databases | MySQL/MariaDB 库·用户·授权·备份，Redis 状态·慢日志·key 浏览 |
| 终端 / Terminal | 浏览器 xterm.js ↔ WebSocket ↔ PTY |
| 防火墙 / Firewall | ufw / firewalld 自动识别、端口规则、监听对照 |
| 容器 / Containers | 接管已有 Docker：容器·镜像·卷·网络·Compose 项目 |
| 计划任务与备份 / Cron & Backup | cron 管理、站点+数据库备份、保留轮转 |

## 快速开始 / Quick Start

```bash
# 一键安装（systemd 发行版: Debian 11+ / Ubuntu 20.04+ / RHEL 9；从 GitHub Releases 下载并校验）
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash

# 升级 = 重复执行同一命令（二进制替换 + 重启；配置与数据不动）
```

### 国内服务器安装 / Installing from mainland China

国内直连 GitHub 常被 RST，脚本内置三条通道（安装时会自动提示）：

```bash
# ① 镜像前缀（对 github/raw/api 三源统一生效）
curl -fsSL https://ghproxy.com/https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash
DOUGONG_MIRROR=https://ghproxy.com sudo -E bash install.sh

# ② 镜像不代理 api.github.com 时：指定版本跳过版本查询
DOUGONG_VERSION=v0.1.2 DOUGONG_MIRROR=https://ghproxy.com sudo -E bash install.sh

# ③ 本地模式（任意机器下载 Release 后上传，零外网依赖）
bash install.sh ./dougong-linux-amd64
```

镜像仅是传输通道——下载包仍强制 **sha256 校验**，完整性不受镜像方影响。

从源码构建（需要 Go 1.22+ 与 Node 18+）/ Build from source：

```bash
make build-linux          # 前端构建 + 内嵌 + linux/amd64 静态二进制 → release/dougong-linux-amd64
sudo bash install.sh release/dougong-linux-amd64   # 本地二进制安装
```

安装后查看监听端口（默认随机高位端口，防扫描）：`journalctl -u dougong -n 5 | grep listening`，浏览器访问完成初始化向导。CLI 初始化（无浏览器场景）：`dougong init -username admin`（密码经 `DOUGONG_INIT_PASSWORD` 注入）。

开发模式：后端 `cd backend && go run ./cmd/dougong`，前端 `make dev`（Vite 代理）。

## 安全设计 / Security

- 命令执行一律参数数组（`exec.CommandContext`），全仓禁 shell 拼接 / arg-array command execution only, no shell concat
- 配置写入固定流程：快照备份 → 修改 → `nginx -t` 校验 → 失败自动回滚 / backup → modify → `nginx -t` → auto-rollback
- 所有变更类操作写审计日志；零外部回传（除用户主动 ACME 证书申请）/ every mutation audited; zero phone-home
- 随机高位端口 + 可选随机安全入口路径 + 建议启用 HTTPS / random high port, optional secret entry path, HTTPS recommended

## 文档 / Docs

设计全档案（业务架构、PRD、API 契约、交互规格、门控报告）见 [profiles/dougong-panel/workspace](profiles/dougong-panel/workspace/)，项目计划书见 [idea.md](profiles/dougong-panel/workspace/00-architecture/idea.md)。

## 许可 / License

[GPL-3.0](LICENSE)
