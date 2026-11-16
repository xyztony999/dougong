# D0 需求澄清报告：terminal（Web 终端）

> 引擎 design 阶段 D0 产物（2026-11-12，批量澄清）。作为 D1 架构设计的直接输入。
> 需求来源：idea.md §4.1 M6 + knowledge/business-concepts（terminal_session、system_user、R-18、A9 余项）。

## 1. 业务概念解析

| 维度 | 内容 |
|------|------|
| 业务术语 | PTY、xterm.js、WebSocket 升级、指定系统用户、连接超时、并发限制 |
| 业务流程 | 前端 xterm.js → WebSocket（复用面板会话鉴权）→ 服务端 creack/pty 建立 PTY → 以面板运行用户（通常 root）或指定系统用户身份交互；断开条件：会话过期/空闲超时/硬上限/面板登出 |
| 业务角色 | **admin 专属**（readonly 禁用 Web 终端，ADR-018）；可选指定系统用户（system_user） |
| 业务规则 | R-18 复用面板会话鉴权 + 连接超时 + 并发限制；A9 余项已拍板：**每用户并发 5 PTY、空闲 30 分钟断开、单连接硬上限 8 小时**；R-02 WS 鉴权同源 Cookie 会话 |
| 业务数据 | terminal_session（运行用户、建立时间、最后活跃、连接来源、鉴权会话引用；内存态，不落库，断开即释放） |

## 2. 架构匹配与类型判定

- 匹配结果：terminal_session/system_user 已在 kb 登记；workspace 无本模块设计文档 → **无匹配，新增**
- 需求类型：**新增** → D1 初始生成
- 模块定位：terminal 适配器（ws↔PTY）唯一消费模块；安全敏感（root shell 暴露面），限流与鉴权复用为设计重点

## 3. 澄清问答记录

| # | 问题 | 结论 | 落点 |
|---|------|------|------|
| Q1（批量 D0 Q3） | 限流阈值取值（机制已定：会话鉴权+超时+并发） | **标准档**：每用户并发 5 PTY；空闲 30 分钟断开；单连接硬上限 8 小时 | ADR-023，A9 完全关闭 |

## 4. 澄清决策回填清单

- knowledge/design-decisions：ADR-023 追加（终端阈值标准档）
- knowledge/business-concepts：terminal_session 行补充阈值
- knowledge/source-index：A9 → 已确认（终端阈值部分关闭，A9 全部关闭）
- workspace/INDEX.md：terminal.requirement → partial（D0 完成，D1-D4 工作流进行中）

## 5. D1 输入摘要（结构化需求）

**模块边界**：
- IN：WS→PTY 会话建立（默认面板运行用户；可选指定系统用户——用户提供 system_user 选择列表）、会话生命周期管理（空闲 30min/硬上限 8h 断开）、并发控制（每用户 5）、断连清理与资源释放
- OUT：容器内终端（docker-mgmt 二期可选项）、会话录制/回放（非目标）、SFTP 文件传输（file-manager 已覆盖）、多机终端（非目标 §4.2）

**核心实体**：terminal_session（内存态）；system_user（只读引用 account-auth/系统侧发现）

**状态机**（已确认）：session 活跃 → 空闲 30min 断开 / 满 8h 断开 / 用户主动断开 / 面板会话失效断开（R-18 联动）

**待 D1 细化（次要，不阻断）**：
- ⚠️ 可指定系统用户的清单来源与权限边界（建议：admin 可指定任意系统用户，切换走 su 语义经 PTY login；不做密码代输——以 root 身份 spawn 指定用户 shell）
- 空闲判定口径（无输入 vs 无流量；建议无输入含输出静默双阈值从严取小）
- 多标签/多会话前端形态（建议标签页，上限=并发阈值 5）
