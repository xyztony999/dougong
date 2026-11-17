# 更新日志 / Changelog

发布说明按版本倒序排列；每个版本的完整说明也会作为 GitHub Release 正文发布。

## v0.1.4 - 2026-11-17

环境管理升级链路热修（v0.1.3 服务器实测反馈）。

### 变更内容

- **修复环境升级在 conffile 冲突时失败**：升级中若新包携带的配置文件（如 `/etc/nginx/nginx.conf`）与本地已修改版本冲突，dpkg 会弹出交互式 Y/N 提示——无人值守环境下无 stdin 可读，直接失败（实机案例：Ubuntu 原生 nginx 1.24 经 nginx.org 源升 1.30 时卡死）。现采用无人值守标准三保险：`DEBIAN_FRONTEND=noninteractive` + `--force-confdef` + `--force-confold`——**本地已修改的配置保持现版不被覆盖**，发行版新版落 `.dpkg-dist` 旁置文件供事后对比
- **修复 MongoDB 源代配后签名验证失败（NO_PUBKEY）**：生成的 apt 单行源里 `arch` 与 `signed-by` 两个选项误用逗号分隔——apt 把整串并入 arch 值，`signed-by` 被静默丢弃，退回默认信任库后无法验证 MongoDB 官方源签名。已改为空格分隔并修正组件名（Ubuntu 用 `multiverse`、Debian 用 `main`，不再附加多余 `main`）；密钥本身无误（armored `.asc` 在 Ubuntu 24.04 实测可用）
- **修复刷新页面偶发裸 JSON 404**：九个页面路由与后端模块 API 前缀同名（`/env` `/sites` `/services` `/files` `/monitoring` `/terminal` `/firewall` `/docker` `/cron`），侧栏点击走前端路由无感，但**直接打开/刷新/收藏夹**这些地址会被后端按「模块前缀未知路径」吐 `NOT_FOUND` JSON 而非页面。现按浏览器导航特征（`Accept: text/html`）区分：导航回 SPA 页面，API 客户端维持 404 包络契约
- **修复未安装环境显示「已停止/自启关」**：systemd 的 LoadUnit 对不存在的单元也返回 `not-found` 形态的属性（inactive），被误映射为服务已停止。现该形态服务态如实为空，未安装环境不再渲染服务状态行
- 包操作命令由 `apt` 改为 `apt-get`（脚本场景稳定接口，消除 apt 自身的 WARNING 噪音）

### 安装与升级

- 服务器重跑安装命令即可（同 v0.1.3 方式；数据不动）
- **若你的服务器已卡在半配置状态**（升级报 `end of file on stdin at conffile prompt`），先手动解卡再升级面板：
  `sudo DEBIAN_FRONTEND=noninteractive apt-get -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" -f install`
  然后在面板重新发起升级，即可带新策略完成

## v0.1.3 - 2026-11-17

环境管理实机缺陷修复轮（v0.1.2 上线后的第一批服务器实测反馈）+ 面板 UI 打磨轮。

### 变更内容

**环境管理修复**
- 修复已安装服务「启用」报错 `org.freedesktop.DBus.Error.InvalidArgs`：目录表 systemd 单元名全部改为规范全名（如 `nginx.service`）——`EnableUnitFiles` 不像 `StartUnit` 那样自动补全后缀
- 修复 Docker / MySQL / Redis / PHP-FPM 卡片误显「探测不可用」：`dpkg-query` 对未装包的输出未被「未安装」判定覆盖，现改用包状态字段判定（不受系统语言影响），未装环境将如实显示灰色「未安装」
- 包名多候选检测：自装 `docker-ce`、`mysql-community-server` 等官方源形态可被正确识别为已安装
- 新增 **MongoDB 8.0** 环境（第七个环境）：自动配置 MongoDB 官方软件源（apt/dnf/yum 三系，含公钥下载与审计）后安装 `mongodb-org`

**面板 UI**
- 修复菜单图标重复：服务管理、文件管理及「用户与安全」四个子菜单此前因图标键未映射全部回退为同一兜底图标，现各有专属图标
- 清理提示文案：403 页内部术语（ADR-018）、站点空态、防火墙状态卡重复前缀、环境卡片「安装后可配合「X」模块使用」软引导行等
- 环境卡片描述去除模块括注；`/env/env/list` 响应移除 `relatedModule` 字段

**其他修复（随本版带出）**
- CLI / 服务端按已安装服务解析数据目录，不再依赖启动时的工作目录
- 证书：已有导入证书的域名允许发起 ACME 接管（CERT_003 语义修正）
- 证书页补充说明：导入证书为何不显示「更新证书」按钮
- 安装脚本：中英双语交互选择；大陆服务器下载路径友好化（镜像前缀 + 版本固定）

### 安装与升级

- **新安装**：`curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | bash`
- **从 v0.1.1 / v0.1.2 升级**：服务器上重跑同一条命令即可（自动下载最新版、sha256 校验、原地替换二进制并重启 systemd 服务；数据目录 `/var/lib/dougong` 不动）
- **国内服务器**（github 直连易被重置）：`DOUGONG_MIRROR=https://ghproxy.com sudo -E bash install.sh`；镜像不代理 API 时可固定版本：`DOUGONG_VERSION=v0.1.3`
- 数据库迁移 010（env_type 枚举扩 MongoDB）随启动自动执行

### 升级后建议验证

1. 环境管理页对已装 nginx 执行 启用/停用/启动 不再报错
2. 未安装的环境显示灰「未安装」而非橙「探测不可用」
3. MongoDB 安装全链（源代配 → 安装 → mongod 服务实况）
4. 若此前自装 docker-ce：Docker 卡片应显示已安装

## v0.1.2 - 2026-11-16

（本版 tag 曾按用户指示强制重指向，包含下列全部内容）

- 新增**环境管理**模块：nginx / Apache / Docker / MySQL / Redis / PHP-FPM 六环境安装·升级·卸载（apt 与 dnf/yum 双包管理器，异步任务台账）
- **站点列表聚合**：同配置文件的 80/443 server 块合并为一条目，可展开块清单
- **证书增强**：新增更新/替换证书；DNS-01 自动化验证（Cloudflare / 阿里云 DNS / DNSPod 凭证管理、自动 TXT、泛域名证书）
- **面板 IA 重构**：顶栏显示当前页标题；侧栏新增「用户与安全」分组（用户/会话/审计/个人安全，修复此前不可达）；设置页四分组重排；首启/登录页 dark mode 完整适配与文案净化
- 新增服务端管理 CLI：`dougong status / info / reset-password / set-port / entrypath / service`
- 数据库迁移 008/009；升级方式同上（重跑 install.sh）

## v0.1.1 - 2026-11-16

- 修复登录后侧栏整体消失（后端叶子菜单 `children:null` 挡崩前端渲染）
- CLI 优化：默认静默 gin 调试横幅（`DOUGONG_DEBUG=1` 恢复），新增 `dougong version`

## v0.1.0 - 2026-11-16

首个发布版本：单二进制面板（监控 / 服务 / 文件 / 网站 / 数据库 / 终端 / 防火墙 / 容器 / 计划任务与备份 / 账户与安全 十一模块），Linux amd64/arm64，一键安装脚本。
