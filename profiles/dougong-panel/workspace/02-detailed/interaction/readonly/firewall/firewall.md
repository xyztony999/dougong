# PageSpec · 防火墙（firewall） · 只读用户（readonly）

| 属性 | 值 |
|------|-----|
| 版本 | v1.0 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-14 |
| 用户类型 | readonly |
| 唯一数据源 | 02-detailed/api/api-firewall.md v1.0 |
| 公共页定义 | **以 admin 文档（../admin/firewall/firewall.md）为单一事实源**——页头状态 Card/引导态/双 Tab/表格列/错误处理/刷新纪律逐字复用；本文档仅声明子集差异与负空间（本模块存在 2 个写接口，readonly 为子集视角，对照 panel-core/service-mgmt 模式而非 monitoring 全等模式） |
| 边界依据 | ADR-018：全模块可查看 + 禁一切变更/终端/上传下载——防火墙规则添加/删除为变更类动作，readonly 界面无入口、越界请求被全局边界管控拒绝（api 3.3 权限矩阵 2 个 ✗ 项 + AUTH_008） |
| 组件体系 | Vue 3 + Naive UI（同 admin 文档） |

## 第 1 节 模块元信息

| 项 | 值 |
|----|-----|
| 模块编码 | firewall |
| 业务类型 | 工具类（同 admin 判定） |
| 页面类型 | hybrid ×1（防火墙页：页头状态 Card + 引导态 + 双 Tab，同 admin） |
| 可见页面 | /firewall——与 admin 同路由（readonly 可进入查看规则与对照全集 + 无后端引导视角，FL 2.2「全可看」） |
| 错误处理 | 与 admin 一致，另含 AUTH_008 全局边界拒绝提示「权限不足：只读角色禁止防火墙规则变更」（正常 UI 无入口不触发——api 3.3 readonly 写全 ✗ + 全局中间件拦截） |

## 第 2 节 Action 定义

| Action | 方法 | 路径 | 页面 | 说明 |
|--------|------|------|------|------|
| backendStatus | GET | /firewall/backend/status | firewall·页头状态 Card | 与 admin 一致（同刷新纪律；readonly 亦见引导态与 FW_010 横幅） |
| ruleList | GET | /firewall/rule/list | firewall·Tab 端口规则 | 与 admin 一致（同筛选参数） |
| portList | GET | /firewall/port/list | firewall·Tab 端口对照 | 与 admin 一致 |

**负空间声明**（api 3.3 权限矩阵 2 个 ✗ 项逐项对应）：

| admin Action（readonly 不注册） | api 权限矩阵 ✗ 项 | 负空间落法 |
|--------------------------------|------------------|-----------|
| ruleCreate | POST /firewall/rule/create = ✗ | 工具栏**「添加规则」按钮不渲染**（非禁用置灰——不渲染使无入口可探）；添加 Dialog/FW_005 危险确认 Modal 相应不可达 |
| ruleDelete | POST /firewall/rule/delete = ✗ | 操作列**整列不渲染**（fixed:right 列消失，其余列定义不变）；FW_005 两段式流程整体不存在 |

> 越界兜底：即使伪造请求，全局中间件 RoleRequired 拦截返回 AUTH_008（api 3.3「接口层不自建权限分支」）；本 PageSpec 层面以不渲染保证无入口。页头状态/筛选/刷新/Tab 懒加载/风险敞口高亮（4.4）等 readonly 全部保留（只读能力对齐 ADR-018「全可看」——规则标注 isPanelPort 与对照视图为查看内容，保留呈现）。

## 第 3 节 字典 Key 清单

与 admin 文档第 3 节完全一致（同一 enums.ts 常量源）：firewall_backend / firewall_rule_action / firewall_rule_protocol / listen_port_state（+panel_role 复用）；颜色方案同源（allow=green/deny=red/reject=orange/limit=blue；allowed_by_rule=green/no_allow_rule=orange；backend/active 同 admin）。

## 第 4 节 列表页（/firewall，只读视角）

与 admin 文档第 4 节逐字一致，仅以下差异：

| 项 | admin | readonly |
|----|-------|----------|
| Tab 1 工具条（4.1） | 动作/协议筛选 + 添加规则 + 刷新 | **无「添加规则」按钮**（负空间，第 2 节）；筛选与刷新完全一致（筛选为查看能力，ADR-018 全可看） |
| 操作列（4.3） | 删除按钮 + 在途互斥 + FW_005/结果反馈 | **整列不渲染**（负空间）；表格列定义其余不变 |
| 页头（4.0） | 状态 Card + 刷新 | **完全一致**（含引导态与 FW_010 横幅——readonly 亦需知道「无后端/后端不可用」实况） |
| Tab 2 对照表（4.4） | 全量对照 + 风险敞口高亮 | **完全一致**（对照为查看能力；高亮提示仅呈现无处置入口，处置线索为「联系管理员或经服务器本地操作」文案，不虚构 readonly 操作路径） |

空态/加载态/骨架/引导态与 admin 一致。

## 第 5 节 表单页

无表单页（同 admin 第 5 节：添加规则 Dialog 为 admin 专属——readonly 无添加入口即无表单形态；筛选为查询条件非表单）。

## 第 6 节 详情页

无详情页（同 admin 第 6 节：无 detail 接口；rawSpec 列内呈现对 readonly 同样可见——后端互认语义为查看内容）。

## 第 7 节 路由定义

| 路由 | meta | 与 admin 差异 |
|------|------|--------------|
| /firewall | `{requiresAuth:true}` | 同路由同页面组件（组件内部按角色渲染子集视图）；无独立 readonly 路由 |
| 框架菜单 | — | menuList 下发的防火墙菜单项对 admin/readonly 均可见（服务端按 ADR-018「全可看」下发） |

## 第 8 节 全局一致性

| # | 检查 | 结果 |
|---|------|------|
| 1 | readonly Action 与 api 只读接口一致、无虚构接口 | ✅ 3 Action（backendStatus/ruleList/portList）= api 3.3 readonly ✓ 项全集 |
| 2 | 负空间检查：api 3.3 权限矩阵 ✗ 项逐项对应 | ✅ 2/2（create → 工具栏无添加按钮；delete → 操作列整列不渲染，第 2 节表） |
| 3 | 子集视角与 admin 公共定义单一事实源（无字段差异、无查询差异、对照视图全等） | ✅ 第 4 节引用声明 |
| 4 | S8 字典同源 enums.ts（四枚举取值逐字一致） | ✅ 第 3 节 |
| 5 | AUTH_008 边界拒绝文案与全局拦截器一致（正常 UI 不可达） | ✅ 第 1 节 |
| 6 | ADR-018 三禁落法：禁变更=添加/删除负空间；禁终端/禁上传下载=本模块无可作用对象 | ✅ 第 2 节 |
| 7 | 引导态对 readonly 同呈现（无后端≠无规则，无角色差异） | ✅ 第 4 节（api FW_009/I-01） |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-14 | initial | 首次生成：只读视角为 admin 子集（3 Action、同页面、无添加按钮、操作列整列不渲染）；api 3.3 权限矩阵 2 个 ✗ 项逐项负空间对应；引导态/对照高亮同 admin 全等；公共页定义单一事实源 = admin 文档 |

> FX 清单执行方式同 admin 文档第 9 节注记（清单文件未随 payload 发布，按 9 节结构落实 + 第 8 节自查；全量留待 Gate 1）。
