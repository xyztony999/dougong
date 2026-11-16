<!-- kb-generated: 2026-11-12 | sources: 1 | assumptions: 6 | open: 0 -->

# design-decisions 知识库

> 由 /profile kb 从 workspace/00-architecture/idea.md §2「已确认的决策记录」及其他章节蒸馏生成（2026-11-12）。来源列 = 材料章节；澄清产生的决策来源 = 澄清日期。

## ADR 表

| 编号 | 决策 | 影响 | 来源 |
|------|------|------|------|
| ADR-001 | 后端用 **Go**（单静态二进制、go:embed 内嵌前端、内存占用低、交叉编译方便、nginx/系统管理生态成熟） | 决定全后端形态与部署产物 | 材料 §2 D1 |
| ADR-002 | 前端用 **Vue 3 + TypeScript + Vite**，UI 库 Naive UI（中文开源社区对 Vue 最熟悉；Naive UI TS 友好、轻量） | 前端形态与组件生态 | 材料 §2 D2 |
| ADR-003 | 许可证 **GPL-3.0**（防云厂商闭源商用 fork；面板类项目惯例） | 仓库许可与合规 | 材料 §2 D3 |
| ADR-004 | 产品定位**开源社区项目**（仓库按开源标准搭建：CI、贡献指南、发布流程；多用户 + RBAC） | 工程化要求 | 材料 §2 D4 |
| ADR-005 | 核心卖点 = **接管已有环境**（直接补宝塔最大痛点；区别于 1Panel 应用商店路线） | nginxx 解析器为项目核心资产；各模块"发现即接管"哲学 | 材料 §2 D5 / §3 / §9.2 |
| ADR-006 | MVP 范围 = 全核心功能集（网站+反代、文件、监控+服务、MySQL/MariaDB+Redis、终端、防火墙、计划任务、备份、容器），按里程碑分期交付 | 里程碑 M1-M7 划分 | 材料 §2 D6 |
| ADR-007 | 数据库模块范围：MVP 只做 MySQL/MariaDB + Redis；**PostgreSQL 二期** | dbmysql/dbredis 范围边界 | 材料 §2 D7 |
| ADR-008 | 目标平台 = 主流 systemd 发行版（Debian 11+、Ubuntu 20.04+、RHEL 9 系）；非 systemd（Alpine 等）不做 | sysd/adapter 层设计；平台支持矩阵 | 材料 §2 D8 |
| ADR-009 | 部署形态 = 单二进制 + 可审计安装脚本（下载二进制 + systemd unit）；无运行时依赖 | go:embed、SQLite 单文件、安装脚本要求 | 材料 §2 D9 |
| ADR-010 | nginx 配置解析**自研**（词法扫描 → 指令树 → Raw 保留），不用第三方库（无维护良好的 Go 库） | nginxx 包为项目核心资产，需 round-trip 样本集（≥20 真实配置）重点测试 | 材料 §5.3 / §5.4 |
| ADR-011 | 存储用 **SQLite**（modernc.org/sqlite 纯 Go 驱动，免 CGO 交叉编译无痛） | store 层实现；单文件部署 | 材料 §5.3 / §3 |
| ADR-012 | **零遥测**承诺（无外呼、更新检查默认关闭、除用户主动 ACME 外不发起任何外部请求） | 全模块网络纪律；安全信任卖点 | 材料 §3 / §7 / §1 |
| ADR-013 | Compose 管理**走 `docker compose` CLI**（参数数组调用，经 label 定位），不用 SDK 编排 | dockerx 实现方式 | 材料 §5.3 / §4.1 M9 |
| ADR-014 | 监控历史趋势 = 进程内环形缓冲**不落库**（重启后历史清零；零写放大；缓冲条数受 <60MB 内存预算约束） | metrics 包实现（轻量优先）；A4 假设关闭（2026-11-12 批量 D0 拍板） | 材料 §4.1 M2 / 澄清 2026-11-12（批量 D0 Q2） |
| ADR-015 | 业务对象英文命名确认：23 个对象英文标识全部按 kb 建议确认（site/db_instance/database/db_user/redis_instance/file_entry/system_user/cron_job/backup_job/panel_user/session/audit_log/firewall_rule/metric/terminal_session/panel_config + Docker/systemd 行业标准五件套/服务/证书等） | DDL/接口/包命名有唯一依据；A1 假设关闭 | 澄清 2026-11-12（Q1） |
| ADR-016 | 备份任务增加独立执行记录实体 **backup_run**（状态 running→success/failure，含耗时/产物路径/大小/触发方式；按保留份数轮转清理记录） | 备份模块 D3/DDL 有明确对象边界；backup_run_status 枚举取值确定 | 澄清 2026-11-12（Q2） |
| ADR-017 | 管理员初始化**双入口**：Web 首次访问向导（未初始化前拦截一切路由）+ CLI 子命令（`dougong set-admin` 类）；两入口互斥生效（先到先得，初始化完成后另一入口拒绝） | account-auth D1 边界（含 CLI 子命令与安装脚本联动）；并发仲裁建议初始化锁单行 INSERT | 澄清 2026-11-12（/design D0 Q1） |
| ADR-018 | 只读角色边界：**全模块可查看（含审计日志）+ 禁一切变更 + 禁 Web 终端 + 禁上传下载** | RBAC 权限矩阵基线（全模块适用）；A5 假设关闭 | 澄清 2026-11-12（/design D0 Q2） |
| ADR-019 | 会话过期策略默认值：**绝对过期 12h + 空闲超时 2h（滑动续期）**，两项均可配置；强制下线 = 管理员立即吊销 | auth 会话实现与"强制下线"语义；A9 会话部分关闭 | 澄清 2026-11-12（/design D0 Q3） |
| ADR-020 | 审计日志**保留 90 天**，后台每日任务清理过期记录；清理动作本身记审计；到期前可手动导出 | audit_log 存储体积控制（<60MB 目标）与后台任务设计；A9 审计部分关闭（终端限流阈值移交 terminal 模块 D0） | 澄清 2026-11-12（/design D0 Q4） |
| ADR-021 | 文件删除**进回收站**：`.dougong-trash/` 目录 + trash_entry 落 SQLite（原路径/删除时间/操作者/类型/大小），支持还原与清空；还原冲突（原路径已存在）要求用户选择 | file-manager 删除/回收站设计与 trash_entry 对象登记；**A6 假设方向修正**（原假设二次确认，用户拍板回收站）；R-17 更新 | 澄清 2026-11-12（批量 D0 Q1） |
| ADR-023 | Web 终端限流阈值**标准档**：每用户并发 5 PTY；空闲 30 分钟断开；单连接硬上限 8 小时 | terminal 会话生命周期与并发控制；A9 余项（终端阈值）关闭，A9 全部关闭 | 澄清 2026-11-12（批量 D0 Q3） |
| ADR-024 | SSL 证书生命周期 **ACME 惯例 6 状态机**：pending → valid → expiring_soon（到期前 30 天）→ expired / renew_failed（重试成功回 valid）；支撑自动续期调度与失败排障 | site-mgmt cert_status 枚举与续期任务调度设计；A2 假设关闭 | 澄清 2026-11-12（批量 D0 Q4） |
| ADR-025 | 容器状态枚举**含 paused**（running/stopped/paused 为面板操作映射核心态；created/exited/dead 等 API 全态由 D1 列全并映射动作矩阵） | docker-mgmt container_status 枚举；A7 假设关闭（按建议确认） | 澄清 2026-11-12（批量 D0 附带） |
| ADR-026 | 密码复杂度**从宽**：最小长度 8，不强制字符类（个人自托管场景） | account-auth D2/D3 校验规则定稿依据；A10 假设关闭（按建议确认） | 澄清 2026-11-12（批量 D0 附带） |
| ADR-027 | **单仓双目录布局**：仓库根下 `backend/`（Go：cmd/dougong + internal/ + go.mod）与 `frontend/`（Vue3 + TS + Vite + Naive UI + Pinia）并列，替代 idea.md §5.2 的"根=Go 项目 + web/ 前端"原生布局；MVP 不为前后端单开仓库 | go-vue3 栈 `requires_env` 落点：BACKEND_ROOT=<仓根>/backend、FRONTEND_ROOT=<仓根>/frontend（settings.json env 已配置）；栈 module_pattern `cmd/{service}` 相对 BACKEND_ROOT 不变；/feature B/F 阶段代码落盘目标 | 用户拍板 2026-11-12（go-vue3 激活时） |
| ADR-028 | **面板域名绑定=强校验**（宝塔同款语义）：panel_config 新增 bind_domain（可空）；绑定后仅该 Host 可访问面板，IP/其他 Host 一律**标准 404**（与 EntryPathGuard 同防探测语义 D-P03/PRD N2，无业务错误码包裹）；读态失败 fail-closed 404（沿 A-P7 先例） | panel-core 增量：HostGuard 全局前置（与 EntryPathHandler 同层 net/http 包装）、配置接口/设置页/审计 | 用户拍板 2026-11-16（/pipeline 增量 D0 Q1） |
| ADR-029 | **面板自身证书 ACME=经已有 nginx HTTP-01**：复用 site-mgmt AcmeGateway（lego，D-09 先例）与 nginxx 安全写回（R-06 备份→临时注入 /.well-known/acme-challenge location→签发→撤除→`nginx -t`→回滚保障）；面板证书落 panel_cert（source 扩展 acme）；不新增独立 80 端口监听（存量服务器 80 被 nginx 占用，符合接管哲学）；DNS-01 二期 | panel-core 增量：跨模块消费契约（site-mgmt 登记被依赖+OHS）、证书申请/续期接口与编排服务、https_mode 联动 | 用户拍板 2026-11-16（/pipeline 增量 D0 Q2） |
