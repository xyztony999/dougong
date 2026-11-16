# D0 需求澄清报告：docker-mgmt（容器管理）

> 引擎 design 阶段 D0 产物（2026-11-12，批量澄清）。作为 D1 架构设计的直接输入。
> 需求来源：idea.md §4.1 M9 + §5.3（Docker SDK/Compose CLI）+ knowledge/business-concepts（container/image/volume/network/compose_project、R-12/R-13、A7）。

## 1. 业务概念解析

| 维度 | 内容 |
|------|------|
| 业务术语 | Docker daemon 接管、`/var/run/docker.sock`、容器生命周期、日志跟踪、Compose 项目、label 识别、安装引导 |
| 业务流程 | 连接本机已有 daemon（官方 Go SDK）→ 容器/镜像/卷/网络自动发现（接管而非重装）→ 容器操作（启停/重启/删除/日志/资源占用）；Compose：容器 label 自动识别项目 → up/down/restart/logs（`docker compose` CLI 参数数组，经 label 定位工作目录）；未装 Docker → 安装引导页而非报错 |
| 业务角色 | admin 全操作；readonly 查看容器/镜像/卷/网络/Compose 列表与日志（ADR-018） |
| 业务规则 | R-12 只接管已有 daemon，**不做应用商店/一键部署**；R-13 Compose 经 label 识别 + CLI 参数数组；ADR-013 Compose 走 CLI 不走 SDK 编排；A7 已确认容器状态枚举含 paused |
| 业务数据 | container（名称/镜像/状态/资源占用/所属 Compose label）、image、volume、network、compose_project（名称/工作目录/配置文件/容器清单） |

## 2. 架构匹配与类型判定

- 匹配结果：五个对象均已在 kb 登记；ADR-013 已定 Compose 实现方式；workspace 无本模块设计文档 → **无匹配，新增**
- 需求类型：**新增** → D1 初始生成
- 模块定位：dockerx 适配器消费模块；与 DPanel 同哲学（接管既有容器），与 1Panel 应用商店路线明确切割（R-12）

## 3. 澄清问答记录

| # | 问题 | 结论 | 落点 |
|---|------|------|------|
| Q1（批量 D0 附带） | 容器状态枚举是否含 paused（A7：Docker 标准状态，材料未列全） | **含 paused**（running/stopped/paused 为面板操作映射核心态；created/exited/dead 等 API 全态由 D1 列全并映射到面板动作） | ADR-025，A7 关闭（按建议确认） |

## 4. 澄清决策回填清单

- knowledge/design-decisions：ADR-025 追加（容器状态枚举含 paused）
- knowledge/business-concepts：状态机表容器行去 ⚠️A7 标记
- knowledge/source-index：A7 → 已确认
- workspace/INDEX.md：docker-mgmt.requirement → partial（D0 完成，D1-D4 工作流进行中）

## 5. D1 输入摘要（结构化需求）

**模块边界**：
- IN：daemon 连接管理（sock 探测；未装 → 安装引导页文案与检测）、容器列表/状态/启停/重启/暂停恢复/删除、容器日志（尾部查看 + 实时跟踪）、容器资源占用（CPU/内存）、镜像列表/拉取/删除、卷列表与删除、网络列表与删除、Compose 项目自动识别（label）与 up/down/restart/logs
- OUT：应用商店/一键部署（**明确不做**，R-12）、容器内终端（二期可选，idea.md 标注）、镜像构建/Dockerfile 管理（非目标）、Registry/私有仓库管理（非目标）、Swarm/Kubernetes（非目标）、跨主机管理（非目标 §4.2）

**核心实体**：container、image、volume、network（Docker API 实态投影不落库）、compose_project（label 聚合投影）

**状态机**（已确认）：running ⇄ stopped ⇄ paused（start/stop/restart/pause/unpause 面板动作映射；created/exited/dead 为 API 态由 D1 列全）

**待 D1 细化（次要，不阻断）**：
- ⚠️ 容器状态枚举全集与面板动作映射表（Docker API 全态 → 面板可用动作矩阵）
- Compose 项目操作的工作目录定位失败兜底（label 缺失/目录已移除时的报错口径）
- 日志跟踪的行数上限与轮转策略（建议默认尾部 500 行，跟踪复用 Docker log stream API）
