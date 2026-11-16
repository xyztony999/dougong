# D0 需求澄清报告：service-mgmt（systemd 服务管理）

> 引擎 design 阶段 D0 产物（2026-11-12，批量澄清）。作为 D1 架构设计的直接输入。
> 需求来源：idea.md §4.1 M2（systemd 部分）+ knowledge/business-concepts（systemd_service、R-08、ADR-008）。

## 1. 业务概念解析

| 维度 | 内容 |
|------|------|
| 业务术语 | systemd 服务、启动/停止/重启、开机自启（enable/disable） |
| 业务流程 | 列出 unit（dbus ListUnits）→ 查看状态 → 面板操作（Start/Stop/Restart/enable/disable）→ 结果反馈；全部变更记审计（R-03） |
| 业务角色 | admin 可操作；readonly 仅查看列表与状态（ADR-018） |
| 业务规则 | R-08 经 dbus（godbus/dbus）而非 shell 拼接；ADR-008 仅 systemd 发行版（Debian 11+/Ubuntu 20.04+/RHEL 9 系） |
| 业务数据 | systemd_service（dbus 实时投影，不落库） |

## 2. 架构匹配与类型判定

- 匹配结果：systemd_service 对象已在 kb 登记；workspace 无本模块设计文档 → **无匹配，新增**
- 需求类型：**新增** → D1 初始生成
- 模块定位：sysd 适配器（adapter 层）的唯一消费模块；「发现即接管」哲学的直接体现（系统上已装服务自动纳入）

## 3. 澄清问答记录

| # | 问题 | 结论 | 落点 |
|---|------|------|------|
| — | 本轮无新增开放点 | 状态机与自启维度 kb 已定义（active/inactive/failed + enabled/disabled）；实现通道材料已定（dbus） | — |

## 4. 澄清决策回填清单

- 无新增 ADR
- workspace/INDEX.md：service-mgmt.requirement → partial（D0 完成，D1-D4 工作流进行中）

## 5. D1 输入摘要（结构化需求）

**模块边界**：
- IN：服务列表（含状态过滤/搜索）、启停/重启、开机自启开关、unit 基本属性查看
- OUT：进程 TOP 列表（monitoring）、journalctl 服务日志查看（材料 M2 未列，MVP 不做，留二期）、unit 文件编辑（材料未列，不做）

**核心实体**：systemd_service（名称、active 状态、sub 状态、开机自启、unit 类型；dbus 实时读取，不落库）

**状态机**（kb 已确认）：active ⇄ inactive / failed；enabled ⇄ disabled（自启维度独立于运行维度）

**待 D1 细化（次要，不阻断）**：
- ⚠️ unit 范围口径：建议仅 system 级管理器；列表含 service/socket/timer 等全部类型但**操作类按钮仅对 .service 开放**
- 操作耗时与超时（Restart 类 dbus 调用 Job 完成等待的 UI 反馈口径）
