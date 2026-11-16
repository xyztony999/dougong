# PageSpec · systemd 服务管理（service-mgmt） · 只读用户（readonly）

| 属性 | 值 |
|------|-----|
| 版本 | v1.0 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-12 |
| 用户类型 | readonly |
| 唯一数据源 | 02-detailed/api/api-service-mgmt.md v1.0 |
| 公共页定义 | **以 admin 文档（../admin/service-mgmt/service-mgmt.md）为单一事实源**——列表页结构/查询条件/表格列/Drawer 字段/错误处理/刷新纪律逐字复用；本文档仅声明子集差异与负空间（本模块存在 5 个写接口，readonly 为子集视角，对照 panel-core 模式而非 monitoring 全等模式） |
| 边界依据 | ADR-018：全模块可查看 + 禁一切变更/终端/上传下载——服务操作为变更类动作，readonly 界面无入口、越界请求被全局边界管控拒绝（api 3.3 权限矩阵 5 个 ✗ 项 + AUTH_008） |
| 组件体系 | Vue 3 + Naive UI（同 admin 文档） |

## 第 1 节 模块元信息

| 项 | 值 |
|----|-----|
| 模块编码 | service-mgmt |
| 业务类型 | 工具类（同 admin 判定） |
| 页面类型 | config ×1（服务管理列表页 + 属性 Drawer，同 admin） |
| 可见页面 | /services——与 admin 同路由（readonly 可进入查看服务全集，FL 附录A「服务列表/过滤搜索/属性查看 = 两角色 ✓」） |
| 错误处理 | 与 admin 一致，另含 AUTH_008 全局边界拒绝提示「权限不足：只读角色禁止服务操作」（正常 UI 无入口不触发——api 3.3 readonly 全 ✗ + 全局中间件拦截） |

## 第 2 节 Action 定义

| Action | 方法 | 路径 | 页面 | 说明 |
|--------|------|------|------|------|
| unitList | GET | /services/unit/list | services | 与 admin 一致（同参数/同刷新纪律） |
| unitDetail | GET | /services/unit/detail | services·属性 Drawer | 与 admin 一致 |

**负空间声明**（api 3.3 权限矩阵 5 个 ✗ 项逐项对应）：

| admin Action（readonly 不注册） | api 权限矩阵 ✗ 项 | 负空间落法 |
|--------------------------------|------------------|-----------|
| unitStart | POST /services/unit/start = ✗ | 操作列整列不渲染（启动/停止/重启按钮均不存在，非禁用置灰——不渲染使无入口可探） |
| unitStop | POST /services/unit/stop = ✗ | 同上 |
| unitRestart | POST /services/unit/restart | 同上 |
| unitEnable | POST /services/unit/enable = ✗ | 自启列无 NSwitch——恒渲染为 NTag（两态 dict 颜色 / 非两态灰字原值 ⚠️A14） |
| unitDisable | POST /services/unit/disable = ✗ | 同上 |

> 越界兜底：即使伪造请求，全局中间件 RoleRequired 拦截返回 AUTH_008（api 3.3「接口层不自建权限分支」）；本 PageSpec 层面以不渲染保证无入口（对照 api 4.1.4 幂等语义：无入口 + 边界拒绝双层）。inFlightSet/Drawer 刷新按钮/名称列 link 等 readonly 全部保留（只读能力对齐 ADR-018「全可看」）。

## 第 3 节 字典 Key 清单

与 admin 文档第 3 节完全一致（同一 enums.ts 常量源）：service_active_state / service_enabled_state / unit_type（+panel_role 复用）；颜色方案同源（active=green/inactive=gray/failed=red/enabled=blue/disabled=gray/非两态灰字）。

## 第 4 节 列表页（/services，只读视角）

与 admin 文档第 4 节逐字一致，仅以下差异：

| 项 | admin | readonly |
|----|-------|----------|
| 操作列（4.3） | 4 按钮操作集 + 在途互斥 + 终态反馈 | **整列不渲染**（负空间，第 2 节）；表格列定义其余不变（fixed:right 列消失，名称列仍 fixed:left） |
| 开机自启列（4.2） | canToggleAutostart → NSwitch，否则 NTag | **恒为 NTag**（两态 dict 颜色 / 非两态灰字原值）；canToggleAutostart 字段仍接收但不渲染入口 |
| 工具条（4.1） | 状态过滤/名称搜索/刷新 | **完全一致**（过滤搜索为查看能力，ADR-018 全可看） |

空态/加载态/SVC_008 页面横幅/generatedAt 展示与 admin 一致。

## 第 5 节 表单页

无表单页（同 admin 第 5 节：零持久化无新增/编辑语义）。

## 第 6 节 详情页（属性 Drawer）

与 admin 文档第 6 节逐字一致：字段集/打开方式（名称 link + 属性行内按钮）/刷新图标/SVC 处理全同——属性查看为只读能力（FL 附录A「unit 属性查看 = 两角色 ✓」），Drawer 内本就无操作按钮（admin 亦然），故无差异。

## 第 7 节 路由定义

| 路由 | meta | 与 admin 差异 |
|------|------|--------------|
| /services | `{requiresAuth:true}` | 同路由同页面组件（组件内部按角色渲染子集视图）；无独立 readonly 路由 |
| 框架菜单 | — | menuList 下发的服务管理菜单项对 admin/readonly 均可见（服务端按 ADR-018「全可看」下发） |

## 第 8 节 全局一致性

| # | 检查 | 结果 |
|---|------|------|
| 1 | readonly Action 与 api 只读接口一致、无虚构接口 | ✅ 2 Action（unitList/unitDetail）=api 3.3 readonly ✓ 项全集 |
| 2 | 负空间检查：api 3.3 权限矩阵 ✗ 项逐项对应 | ✅ 5/5（start/stop/restart/enable/disable → 操作列不渲染 + 自启无开关，第 2 节表） |
| 3 | 子集视角与 admin 公共定义单一事实源（无字段差异、无查询差异、Drawer 全等） | ✅ 第 4/6 节引用声明 |
| 4 | S8 字典同源 enums.ts（三枚举取值逐字一致） | ✅ 第 3 节 |
| 5 | AUTH_008 边界拒绝文案与全局拦截器一致（正常 UI 不可达） | ✅ 第 1 节 |
| 6 | ADR-018 三禁落法：禁变更=操作列负空间；禁终端/禁上传下载=本模块无可作用对象 | ✅ 第 2 节 |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-12 | initial | 首次生成：只读视角为 admin 子集（2 Action、同页面、操作列整列不渲染、自启恒 Tag 无开关）；api 3.3 权限矩阵 5 个 ✗ 项逐项负空间对应；公共页定义单一事实源 = admin 文档 |

> FX 清单执行方式同 admin 文档第 9 节注记（清单文件未随 payload 发布，按 9 节结构落实 + 第 8 节自查；全量留待 Gate 1）。
