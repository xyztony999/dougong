# PageSpec · db-tools · 只读用户（readonly）

| 属性 | 值 |
|------|-----|
| 版本 | v1.2 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-19（v1.2 自动 nginx 落地增量轮：访问形态/任务 reason 只读呈现同步，负空间 6 不变，数据源 api v1.3/admin v1.2） |
| 用户类型 | readonly |
| 唯一数据源 | 02-detailed/api/api-db-tools.md v1.3（本文件为 admin 视角 v1.2 的只读负空间标注文档） |

## 第 1 节 模块元信息

同 admin 视角元信息（页面/路由/数据源一致）；错误处理同 admin 第 1 节（DBT_001 依赖清单同等呈现——观测不受依赖限制；DBT_012 reason 细分文案同等）。**工具访问整体 admin 专属（⚠️A10）**：readonly 工具本体不可达——工具路径（/dbtool/phpmyadmin/、/dbtool/mongo-express/）经 nginx gating 角色校验拒绝（401/403，api D-11），即使已知 URL 亦不可达（网络层闸门，非仅前端隐藏）；**v1.2：自动落地块独立端口 18090 形态同语义——readonly 经 http://{IP|域名}:18090/dbtool/... 直访同样 gating 401（每请求会话+admin 校验，api D-19）；绑域名后非匹配 Host 404（api D-20，readonly 经 IP:18090 亦不可达）**。

## 第 2 节 Action 定义

| Action | 方法 | 路径 | 页面 | 说明 |
|--------|------|------|------|------|
| dbtoolList | GET | /dbtool/tool/list | tool-list | 工具全景只读（目录+部署状态+运行态+依赖就绪度） |
| dbtoolTaskList | GET | /dbtool/task/list | tool-list（任务 Drawer） | 任务台账只读 |
| dbtoolTaskDetail | GET | /dbtool/task/detail | tool-list（任务 Drawer） | 任务详情与输出尾部只读 |

**负空间（readonly 不可见 Action，6 个，ADR-018 + ⚠️A10 矩阵逐项对应）**：dbtoolInstall / dbtoolUpgrade / dbtoolUninstall（工具变更类）/ dbtoolRotate（凭据轮换变更类）/ **dbtoolOpen（工具访问类——⚠️A10 工具访问整体 admin 专属，沿 Web 终端整域禁用先例但保留目录/任务查看面）** / **dbtoolReopen（工具访问类——SignonURL 重认证落地，admin 会话 gating 专属 ⚠️A10，v1.1 W3；readonly 无入口且 reopen 经 RoleRequired admin 拒绝 302 回 /db-tools）**——前端按 role===admin 隐藏入口 + 后端 AUTH_008 双保险 + **工具路径 gating 网络层闸门三重防护**。内部端点 2（gate/verify、signon/verify）非前端 Action（同 admin 第 2 节声明）。

## 第 3 节 字典 Key 清单

同 admin 第 3 节 5 Key（db_tool_type/db_tool_task_action/db_tool_task_status/db_engine/panel_role）；状态颜色方案同（部署/运行/依赖三态配色）。

## 第 4 节 列表页

### 4.1 工具管理（tool-list，只读视图）— 路由 /db-tools

- 页头说明条与刷新同等可用（观测能力）；generatedAt 呈现。
- 工具卡片（两条目 phpMyAdmin/mongo-express）：部署状态（installed/installedVersion）、运行态（running 三态）、依赖就绪度（depsReady/depsMissing 清单）、在途标记全部只读呈现，语义同 admin（D-02 投影不虚构）；**访问形态区块只读呈现（v1.2，landingPort/autoCreated 投影同 admin 4.1）：18090→「独立访问端口」Tag+访问地址文本+放行提示文案（纯观测信息，readonly 无放行操作能力——提示文案如实呈现不隐藏）；0→「面板域路径访问」；null→不渲染**；**操作区整区不渲染**（安装/升级/卸载/打开/轮换，ADR-018 + ⚠️A10 负空间）；未装条目无安装入口（纯「未安装」状态呈现）；**依赖缺失「到环境管理安装」链接 readonly 可达**（环境管理页两角色可看，观测引导非变更操作）。

### 4.2 任务列表与详情（只读视图）

- 任务 Drawer 同 admin 视角（筛选/列表列/详情区块一致）；行点击查看详情同等可用；**失败原因 error 含 data.reason 具体原因同等可看（v1.2，api D-22——端口占用/多候选/建块失败等全文呈现，ADR-018 观测面）；安装成功放行提示区块同等呈现（纯信息无操作）**；
- 详情刷新按钮可用（含 running 态 5s 轮询——观测轮询非变更操作；终态跃迁触发全景/清单重拉，同 admin 4.3）；
- **无安装/升级/卸载/打开/轮换操作入口**（变更与访问类负空间）。

### 4.3 工具本体不可达登记（⚠️A10 核心负空间）

| 面 | readonly 语义 |
|----|--------------|
| 工具管理页操作区 | 整区不渲染（前端 role 判断） |
| 打开工具（dbtoolOpen） | 无入口 + 后端 AUTH_008 拒绝 |
| SignonURL 重认证（dbtoolReopen） | 无入口 + admin 会话 gating 拒绝（RoleRequired admin，readonly 302 回 /db-tools，api 4.1.4，v1.1 W3） |
| 工具路径直访（已知 URL） | nginx gating 角色校验 401/403（网络层闸门，api D-11——readonly 会话有效但角色不符）；**v1.2：自动落地块 18090 端口同语义（http://{IP\|域名}:18090/dbtool/... 每请求 gating 401）；绑域名后非匹配 Host 404（api D-20）** |
| database-mgmt 高级管理入口 | 不渲染（database-mgmt readonly PageSpec v1.5 负空间登记） |
| 面板内轻量浏览（对照） | **保留可达**（database-mgmt S-56/S-57 元数据两角色可看，Q3 并存——readonly 经轻量入口浏览表/集合元数据，数据内容仍 admin 专属 Q6） |

## 第 5 节 表单页

无（readonly 无任何表单入口；admin 版安装/卸载/升级/轮换/打开弹窗对其负空间）。

## 第 6 节 详情页

无独立详情路由页（任务详情=Drawer，同 admin）。

## 第 7 节 路由定义

| 路由 | 页面 | meta |
|------|------|------|
| /db-tools | tool-list | `{requiresAuth:true, title:'数据库工具'}` |

**侧边菜单**：数据库工具（/db-tools）同 admin（两角色同项，D-P18）。工具本体路径非面板路由（gating 网络层闸门，readonly 不可达，同 admin 第 7 节）。

## 第 8 节 全局一致性

| # | 检查 | 结果 |
|---|------|------|
| 1 | Action ↔ api v1.3 接口：3 只读 Action 全映射 + 6 负空间逐项对应（面板 9 接口全集，v1.1 W3 +dbtoolReopen；v1.3 零新增接口） | ✅ 第 2 节 |
| 2 | 负空间 6 个 Action 与 ADR-018 + ⚠️A10 边界矩阵逐项对应（含工具访问类 dbtoolOpen/dbtoolReopen） | ✅ 第 2 节 |
| 3 | 字典 Key 与池定稿同值（5 项） | ✅ 第 3 节 |
| 4 | 实况与任务观测能力与 admin 同等（ADR-018 全可看；**v1.2：访问形态区块/任务 reason/放行提示只读呈现同等**） | ✅ 4.1/4.2 |
| 5 | 工具本体三重防护登记（前端隐藏 + AUTH_008 + gating 网络层闸门）；轻量入口保留可达对照；**v1.2：18090 端口直访同 gating 拒绝+绑域名非匹配 Host 404** | ✅ 4.3 |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-19 | initial | 首次生成：只读视角 1 页面（工具卡片只读 + 任务 Drawer）+ 3 只读 Action + 5 负空间标注（含 dbtoolOpen 工具访问类 ⚠️A10）；工具本体不可达三重防护登记（4.3）+ 轻量入口保留可达对照（Q3） |
| v1.1 | 2026-11-19 | interface_added | Gate 1 W3 修复轮（数据源 api v1.2，admin 视角 v1.1）：负空间 5→6（+dbtoolReopen 工具访问类——SignonURL 重认证落地 admin 会话 gating 专属 ⚠️A10，readonly 无入口且 RoleRequired admin 拒绝 302 回 /db-tools）；3 只读 Action 不变（3 读+6 负空间=面板 9 接口全集）；4.3 工具本体不可达登记 +dbtoolReopen 行；第 8 节自查 1/2 同步 6 负空间；字典/路由零变更 |
| v1.2 | 2026-11-19 | rule_added | 自动 nginx 落地增量 D4（数据源 api v1.3，admin 视角 v1.2）：**3 只读 Action + 6 负空间零变更**——4.1 +访问形态区块只读呈现（landingPort/autoCreated 投影：18090→Tag+地址+放行提示文案纯观测；0→面板域路径；null→不渲染）；4.2 +任务详情 error 含 data.reason 同等可看（api D-22）+安装成功放行提示区块同等呈现；4.3 工具路径直访行 +18090 端口 gating 401 同语义+绑域名非匹配 Host 404（api D-19/D-20）；第 1 节 +自动块端口不可达声明；第 8 节自查 1/4/5 同步；字典/路由/负空间集合零变更 |
