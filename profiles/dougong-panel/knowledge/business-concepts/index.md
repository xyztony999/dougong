<!-- kb-generated: 2026-11-12 | sources: 1 | assumptions: 10 | open: 0 | updated: 2026-11-12 批量 D0 回填 -->

# business-concepts 知识库

> 由 /profile kb 从 workspace/00-architecture/idea.md 蒸馏生成（2026-11-12）。行内 `⚠️假设An` 对应 source-index §假设汇总编号。2026-11-12 批量 D0 后全部假设已关闭（A8 随各模块 D3 定稿），已拍板行以 ADR 编号标注。

## 对象表

| 对象 | 英文 | 定义 | 关键字段（要点） |
|------|------|------|----------------|
| 站点 | site | nginx server 块的结构化管理对象（核心卖点：存量扫描导入 + 可视化编辑 + 写回） | server_name、listen、root、ssl_cert、location 列表、proxy_pass 上游、enabled、raw 块（未识别指令原文） |
| SSL 证书 | certificate | 站点 TLS 证书：ACME 申请（HTTP-01/TLS-ALPN）或存量识别导入，自动续期 | 域名列表、issuer、有效期、来源（acme/imported）、续期任务 |
| 数据库实例 | db_instance | 发现并接管的 MySQL/MariaDB 实例（socket/TCP/凭据配置） | 类型、连接方式、版本、状态概览、进程列表 |
| 数据库 | database | 实例内的库对象 | 名称、字符集、备份导出/导入 |
| 数据库用户 | db_user | 实例内的用户对象 | 用户名、host、授权、密码 |
| Redis 实例 | redis_instance | 发现并接管的 Redis 实例 | 连接方式、info 状态、慢日志、密码 |
| 文件条目 | file_entry | 文件管理器中的文件/目录对象 | 路径、大小、权限、属主、内容（在线编辑） |
| 回收站条目 | trash_entry | 文件删除回收站记录（澄清新增 2026-11-12，ADR-021） | 原路径、删除时间、操作者、类型（文件/目录）、大小、trash 内标识（落 SQLite） |
| 系统用户 | system_user | 操作系统层用户（终端指定用户、文件 chown、以指定身份运行） | 名称、uid、gid、home、shell |
| systemd 服务 | systemd_service | 已装 systemd 服务（发现即接管，dbus 管理） | 名称、active 状态、sub 状态、开机自启 |
| 容器 | container | 已有 Docker daemon 上的容器（接管而非重装） | 名称、镜像、状态、资源占用、日志、所属 Compose 项目（label） |
| 镜像 | image | 容器镜像 | 名称、tag、大小 |
| 卷 | volume | Docker 卷 | 名称、挂载点、大小 |
| 网络 | network | Docker 网络 | 名称、驱动、子网 |
| Compose 项目 | compose_project | 经容器 label 自动识别的 Compose 项目 | 名称、工作目录、配置文件、容器清单 |
| 计划任务 | cron_job | cron 定时任务 | 表达式、命令、执行日志尾部、手动触发 |
| 备份任务 | backup_job | 站点目录 + 数据库的定时备份任务 | 备份源（站点目录/数据库）、目标（本地/rsync 远程）、保留份数、轮转 |
| 备份执行记录 | backup_run | 备份任务的一次执行实例（Q2 澄清新增，2026-11-12） | 开始/结束时间、状态（running/success/failure）、耗时、产物路径与大小、触发方式（cron/手动）、轮转清理 |
| 面板用户 | panel_user | 面板登录用户（区别于系统用户） | 用户名、密码、TOTP 开关、角色（RBAC） |
| 会话 | session | 面板登录会话（HttpOnly Cookie 承载） | 过期时间、CSRF token、强制下线标记 |
| 审计日志 | audit_log | 全部变更类操作的记录（只追加） | 操作者、时间、对象、变更内容、结果 |
| 防火墙规则 | firewall_rule | ufw / firewalld 端口规则 | 端口、协议、方向、来源限制、后端类型 |
| 监控指标 | metric | 实时 + 历史系统指标 | CPU（总/核）、内存、磁盘（容量/IO）、网络（上下行）、负载、TOP 进程、环形缓冲 |
| 终端会话 | terminal_session | Web 终端 PTY 会话（xterm.js ↔ WebSocket ↔ PTY） | 运行用户、连接超时、并发限制（并发 5/空闲 30min/硬上限 8h，ADR-023）、鉴权来源（内存态不落库） |
| 面板配置 | panel_config | 面板自身设置 | 监听地址/端口（默认随机高位）、安全入口路径、HTTPS、更新检查开关 |

## 状态机表

| 对象 | 状态 | 流转 | 触发 |
|------|------|------|------|
| 站点 | enabled | ⇄ disabled | 面板站点开关 → 走安全写入流程（规则 R-06）→ nginx reload |
| 站点 | disabled | ⇄ enabled | 同上 |
| SSL 证书 | pending | → valid / → renew_failed | ACME 申请（HTTP-01 / TLS-ALPN） |
| SSL 证书 | valid | → expiring_soon（到期前 30 天，ADR-024 六状态机已拍板）→ 自动续期成功 → valid；→ expired | 自动续期任务（lego） |
| SSL 证书 | renew_failed | → 重试成功 → valid / → expired；可手动重试 | 续期失败处理 |
| systemd 服务 | active / inactive / failed | 启动 / 停止 / 重启 | 面板操作（dbus，禁 shell 拼接） |
| systemd 服务 | enabled / disabled（开机自启维度） | 自启开关切换 | 面板操作 |
| 容器 | running / stopped / paused（核心三态含 paused，ADR-025） | start / stop / restart / pause / unpause | 面板操作（Docker 官方 SDK；API 全态映射 D1 列全） |
| 回收站条目 | 在站 | → 已还原 / → 已清空 | 文件管理器还原/清空操作（ADR-021） |
| 面板用户 | 正常 | → 锁定 | 连续 5 次登录失败，锁 15 分钟（R-01） |
| 面板用户 | 锁定 | → 正常 | 锁定期满自动解除 |
| 会话 | 有效 | → 过期 / → 强制下线 | 过期时间到 / 管理员操作（R-02） |
| 备份执行记录 | running | → success / → failure；记录按保留份数轮转清理（R-11） | cron 定时触发或手动触发 |

## 角色表

| 角色 | 英文 | 职责 | 可操作对象 |
|------|------|------|-----------|
| 管理员 | admin | 全部变更类操作 + 用户/权限/审计管理 | 全部对象 |
| 只读 | readonly | 查看类操作（监控/日志/各类列表，含审计日志） | 全部对象（边界已拍板 ADR-018：禁一切变更、禁 Web 终端、禁上传下载） |

## 业务规则表

| 编号 | 规则 | 来源（材料） |
|------|------|-------------|
| R-01 | 登录失败锁定：连续 5 次失败锁定 15 分钟 | §4.1 M1 |
| R-02 | API 鉴权用 HttpOnly Cookie 会话，token 不入 localStorage；全站 CSRF token（同源校验 + token） | §4.1 M1 / §5.4 |
| R-03 | 所有变更类操作写审计日志：谁、何时、改了什么、结果 | §4.1 M1 |
| R-04 | 文件操作路径白名单（默认 `/`，可配置排除 `/proc`、`/sys` 等）；所有路径先 EvalSymlinks 解析真实路径再前缀校验，防 `../` 与符号链接逃逸 | §4.1 M3 / §5.4 |
| R-05 | 文件上传：临时文件 + rename；大文件分块 | §5.4 |
| R-06 | nginx 配置写入固定流程：快照备份（conf.backup/时间戳/）→ 修改 → `nginx -t` 校验 → 失败自动恢复备份并报错 diff | §3 / §5.4 |
| R-07 | nginx round-trip：解析不了的指令/块保留为 Raw 节点按原文本写回，保证不破坏现有配置 | §3 / §5.4 |
| R-08 | 命令执行一律 `exec.Command(name, args...)` 参数数组；全项目禁止 `sh -c` 拼接用户输入（代码审查项 + CI grep 检查） | §5.4 / §7 |
| R-09 | 零遥测零回传：无 phone-home、无强制账号绑定、无广告推广位；更新检查默认关闭且可完全禁用；程序自身不发起外部请求（除用户主动 ACME 申请） | §3 / §7 |
| R-10 | 面板默认监听 `0.0.0.0:随机高位端口`，支持仅 localhost / 指定地址；可选随机安全入口路径；建议但强制 HTTPS（自签或导入证书） | §5.4 |
| R-11 | 备份保留份数轮转 | §4.1 M8 |
| R-12 | 容器管理只接管已有 Docker daemon（`/var/run/docker.sock`），不做应用商店/一键部署 | §4.1 M9 |
| R-13 | Compose 经容器 label 自动识别项目，调用 `docker compose` CLI（参数数组），经 label 定位工作目录与配置文件 | §4.1 M9 / §5.3 |
| R-14 | handler 层对全部输入做服务端校验，不信任前端 | §7 |
| R-15 | 存量自动发现：扫描 nginx conf.d/vhost.d/sites-enabled 解析为站点对象；MySQL/MariaDB/Redis/PHP-FPM 实例、systemd 服务、Docker daemon 自动识别纳入管理，一律接管不要求重装 | §3 |
| R-16 | 站点编辑写回不得影响其余站点（round-trip 验收：其余站点配置 byte-level 或语义级无变化） | §6 M3 |
| R-17 | 文件删除进回收站（`.dougong-trash/` + trash_entry 元数据，支持还原/清空；ADR-021 拍板，2026-11-12） | §4.1 M3 / 澄清 |
| R-18 | Web 终端以面板运行用户（通常 root）身份运行，可选指定系统用户；复用面板会话鉴权；连接超时与并发限制 | §4.1 M6 |

## 关键业务流程

| 流程 | 步骤 | 来源 |
|------|------|------|
| nginx 配置安全写入（项目核心） | 1 备份到 conf.backup/时间戳/ → 2 写新配置 → 3 `nginx -t` 校验 → 4 失败恢复备份并报错 diff → 5 reload | §3 / §5.4 |
| nginx 存量导入 | 1 扫描 conf.d/vhost.d/sites-enabled → 2 词法扫描解析为指令树 → 3 提取 server 块为站点对象 → 4 面板可视化编辑 → 5 重新序列化写回（Raw 节点保留原文） | §3 / §5.4 |
| 初始化引导 | 首次访问设置管理员密码（或 CLI 生成） | §4.1 M1 |
