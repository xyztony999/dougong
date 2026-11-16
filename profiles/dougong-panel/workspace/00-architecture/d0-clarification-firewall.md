# D0 需求澄清报告：firewall（防火墙）

> 引擎 design 阶段 D0 产物（2026-11-12，批量澄清）。作为 D1 架构设计的直接输入。
> 需求来源：idea.md §4.1 M7 + knowledge/business-concepts（firewall_rule、R-08/R-15）。

## 1. 业务概念解析

| 维度 | 内容 |
|------|------|
| 业务术语 | ufw、firewalld、后端自动识别、端口规则、监听端口对照、面板端口保护 |
| 业务流程 | 探测 ufw / firewalld（自动识别后端；均无 → 提示引导）→ 规则列表 → 添加/删除 → 后端命令封装（参数数组，R-08）→ 变更记审计（R-03）；监听端口（ss 系调用）与规则对照展示开放状态 |
| 业务角色 | admin 可增删规则；readonly 查看规则与开放状态（ADR-018） |
| 业务规则 | R-08 命令参数数组禁拼接；R-15 自动识别既有后端；面板自身端口保护提示（删除面板端口规则时警告+二次确认语义） |
| 业务数据 | firewall_rule（端口、协议 tcp/udp、方向 in、来源限制、动作 allow/deny、后端类型 ufw/firewalld——后端实态投影不落库） |

## 2. 架构匹配与类型判定

- 匹配结果：firewall_rule 已在 kb 登记；firewallx 适配器双后端（ufw/firewalld）已在架构 §5.1 明确 → **无匹配，新增**
- 需求类型：**新增** → D1 初始生成
- 模块定位：firewallx 适配器消费模块；误操作风险高（可锁死面板自身），保护提示为设计硬项

## 3. 澄清问答记录

| # | 问题 | 结论 | 落点 |
|---|------|------|------|
| — | 本轮无新增开放点 | 双后端范围与「均无则提示」材料已明确；面板端口保护为 M7 明示项 | — |

## 4. 澄清决策回填清单

- 无新增 ADR
- workspace/INDEX.md：firewall.requirement → partial（D0 完成，D1-D4 工作流进行中）

## 5. D1 输入摘要（结构化需求）

**模块边界**：
- IN：后端自动识别（ufw → firewalld 优先级探测；均无 → 引导提示页）、端口规则列表/添加/删除（端口、协议、来源限制、动作）、监听端口列表对照展示（开放状态）、面板自身端口规则删除保护（警告 + 确认）
- OUT：firewalld zone/富规则完整管理（MVP 仅映射端口级规则，zone 用默认，D1 确认口径）、IP 黑白名单独立管理（二期）、端口扫描/入侵检测（非目标）

**核心实体**：firewall_rule（后端实态投影，不落库；操作即时生效并审计）

**状态机**：无（规则有无即状态）

**待 D1 细化（次要，不阻断）**：
- ⚠️ firewalld 端口级映射口径（permanent + runtime 双写 or 仅 runtime+提示；建议 permanent 为主保证重启有效）
- ufw deny/reject/limit 三动作的表达与默认值（建议 UI 提供 allow/deny/reject/limit 全动作）
- 监听端口信息来源（ss -tulnp 解析 vs /proc 直读；建议 gopsutil 统一取）
