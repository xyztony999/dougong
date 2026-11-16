# PageSpec · 文件管理器（file-manager） · 只读用户（readonly）

| 属性 | 值 |
|------|-----|
| 版本 | v1.1 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-14 |
| 用户类型 | readonly |
| 唯一数据源 | 02-detailed/api/api-file-manager.md v1.0 |
| 公共页定义 | **以 admin 文档（../admin/file-manager/file-manager.md）为单一事实源**——files/trash 两页结构/面包屑导航/排序/视图切换/表格列/错误处理/刷新纪律逐字复用；本文档仅声明子集差异与负空间（本模块 20 个接口 readonly=✗，为子集视角，对照 service-mgmt 模式） |
| 边界依据 | ADR-018：全模块可查看 + 禁一切变更 + **禁上传下载**——readonly 界面无入口、越界请求被全局边界管控拒绝（api 3.3 权限矩阵 20 个 ✗ 项 + AUTH_008） |
| 组件体系 | Vue 3 + Naive UI（同 admin 文档；Monaco 不加载——edit 页为 admin 专属） |

## 第 1 节 模块元信息

| 项 | 值 |
|----|-----|
| 模块编码 | file-manager |
| 业务类型 | 混合形态（同 admin 判定） |
| 页面类型 | config ×2（files / trash，同 admin）；**edit 页（code）不可达**（编辑为变更语义前置，负空间） |
| 可见页面 | /files、/files/trash——与 admin 同路由（readonly 可浏览目录内容与回收站列表，FL 附录A「文件浏览/目录导航/回收站列表查看 = 两角色 ✓」） |
| 错误处理 | 与 admin 一致，另含 AUTH_008 全局边界拒绝提示「权限不足：只读角色禁止该操作」（正常 UI 无入口不触发——api 3.3 readonly 全 ✗ + 全局中间件拦截） |

## 第 2 节 Action 定义

| Action | 方法 | 路径 | 页面 | 说明 |
|--------|------|------|------|------|
| entryList | GET | /files/entry/list | files | 与 admin 一致（同参数/同刷新纪律） |
| trashList | GET | /files/trash/list | trash | 与 admin 一致 |

**负空间声明**（api 3.3 权限矩阵 20 个 ✗ 项逐项对应）：

| admin Action（readonly 不注册） | api 权限矩阵 ✗ 项 | 负空间落法 |
|--------------------------------|------------------|-----------|
| pathScopeGet / pathScopeUpdate | GET/POST /files/path-scope/* = ✗ | 工具条「路径边界设置」按钮不渲染（⚠️A12 安全配置 admin 专属） |
| entryUpload / chunkInit / chunkUpload / chunkComplete | POST /files/entry/upload、/files/upload/* = ✗（ADR-018 禁上传） | 工具条「上传」按钮不渲染 + 表格区**不启用拖拽放置**（dragover/drop 监听不注册——无入口可探） |
| entryDownload | GET /files/entry/download = ✗（ADR-018 禁下载） | 行操作「下载」不渲染 |
| entryContent / entryContentSave | GET/POST /files/entry/content* = ✗ | 行操作「编辑」不渲染；名称列 file 点击**无动作**（不跳转 edit 页，edit 路由组件内角色兜底 + Monaco 不加载） |
| entryCreate | POST /files/entry/create = ✗ | 工具条「新建」按钮不渲染 |
| entryRename | POST /files/entry/rename = ✗ | 行操作「重命名」不渲染 |
| entryMove | POST /files/entry/move = ✗ | 行操作「移动」不渲染 |
| entryDelete | POST /files/entry/delete = ✗ | 行操作「删除」不渲染 + 选择列不渲染（无批量场景） |
| entryChmod | POST /files/entry/chmod = ✗ | 行操作「权限」不渲染（mode 列仍展示——元数据查看属全可看） |
| entryChown | POST /files/entry/chown = ✗ | 行操作「属主」不渲染（owner 列仍展示） |
| systemAccountsList | GET /files/system-accounts/list = ✗ | 随属主 Dialog 负空间（候选数据源无消费场景） |
| entryArchive | POST /files/entry/archive = ✗ | 工具条「压缩」按钮不渲染（选择列已无，无多选场景） |
| entryUnarchive | POST /files/entry/unarchive = ✗ | 行操作「解压」不渲染（归档行仅浏览） |
| trashRestore | POST /files/trash/restore = ✗ | trash 页操作列整列不渲染 |
| trashPurge | POST /files/trash/purge = ✗ | trash 页工具栏「清空」按钮不渲染 |

> 越界兜底：即使伪造请求，全局中间件 RoleRequired 拦截返回 AUTH_008（api 3.3「接口层不自建权限分支」）；本 PageSpec 层面以不渲染保证无入口（api D-07/D-13 的服务端校验为第二层）。面包屑/上级导航/排序/视图切换/刷新/generatedAt 展示等 readonly 全部保留（查看能力对齐 ADR-018「全可看」）；FILE_003 自动刷新/FILE_002 页面横幅等只读错误流保留。

## 第 3 节 字典 Key 清单

与 admin 文档第 3 节完全一致（同一 enums.ts 常量源）：file_type / archive_format（UI 渲染用途相同）+ panel_role 复用；trash_entry_status 不进 UI（同 admin 声明）；颜色/图标映射同源。

## 第 4 节 列表页（/files 与 /files/trash，只读视角）

与 admin 文档第 4 节（files 页）与第 4A 节（trash 页，v1.1 起为 trash 公共定义单一事实源落位）逐字一致，仅以下差异：

| 项 | admin | readonly |
|----|-------|----------|
| files 工具条（4.1） | 导航/排序/视图区 + 上传/新建/压缩/路径边界/刷新 | **仅导航/排序/视图/刷新区**——操作区 4 按钮整组不渲染（负空间，第 2 节） |
| files 表格列（4.2） | 选择列 + 8 列 + 操作列 | **无选择列、无操作列**（fixed:right 列消失，名称列仍 fixed:left）；名称列 file 点击无动作（第 2 节） |
| files Dialog（4.4） | 9 类 Dialog（4.4.1~4.4.9） | **全部不渲染**（触发入口均已负空间；冲突决策 Modal 随写 Action 不存在） |
| 上传管线（4.5） | 直传+分块双分支 | **整节 N/A**（无入口；拖拽监听不注册） |
| trash 工具栏（admin 第 4A.1 节） | 清空（NModal 强警告）+ 刷新 | **仅刷新**（清空按钮不渲染） |
| trash 操作列（admin 第 4A.3 节） | 还原（冲突决策 Modal 4.4.8） | **整列不渲染** |

空态/加载态/FILE_002 页面横幅/generatedAt 展示与 admin 一致。

## 第 5 节 在线编辑页（edit）

**整页不可达**：编辑为变更语义（content 读取为编辑前置、content-save 为变更），readonly 无入口——名称列 file 点击无动作（第 2 节）、edit 路由组件内角色兜底跳回列表、Monaco 不加载（组件体系声明）。

## 第 6 节 详情页

N/A（同 admin 第 6 节：无详情类接口，属性在列表列呈现）。

## 第 7 节 路由定义

| 路由 | meta | 与 admin 差异 |
|------|------|--------------|
| /files | `{requiresAuth:true}` | 同路由同页面组件（组件内部按角色渲染子集视图）；无独立 readonly 路由 |
| /files/trash | `{requiresAuth:true}` | 同上 |
| /files/edit | `{requiresAuth:true}` | 组件内角色兜底（readonly 跳回 /files）——入口已在第 2 节负空间 |
| 框架菜单 | — | 「文件管理」「回收站」菜单项对 admin/readonly 均可见（服务端按 ADR-018「全可看」下发） |

## 第 8 节 全局一致性

| # | 检查 | 结果 |
|---|------|------|
| 1 | readonly Action 与 api 只读接口一致、无虚构接口 | ✅ 2 Action（entryList/trashList）=api 3.3 readonly ✓ 项全集 |
| 2 | 负空间检查：api 3.3 权限矩阵 ✗ 项逐项对应 | ✅ 20/20（第 2 节表：上传下载 5 项/编辑 2 项/整理 4 项/元数据 3 项/压缩解压 2 项/回收站治理 2 项/路径边界 2 项/候选清单 1 项——含 ADR-018「禁上传下载」专项 5 项） |
| 3 | 子集视角与 admin 公共定义单一事实源（无字段差异、无查询差异、排序视图全等） | ✅ 第 4 节引用声明 |
| 4 | S8 字典同源 enums.ts（取值逐字一致；trash_entry_status 不进 UI 同声明） | ✅ 第 3 节 |
| 5 | AUTH_008 边界拒绝文案与全局拦截器一致（正常 UI 不可达） | ✅ 第 1 节 |
| 6 | ADR-018 三禁落法：禁变更=写操作/编辑页负空间；禁上传下载=上传按钮/拖拽监听/下载按钮不渲染；禁终端=本模块无终端作用对象 | ✅ 第 2/4/5 节 |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-12 | initial | 首次生成：只读视角为 admin 子集（2 Action、/files 与 /files/trash 同页面、edit 页不可达）；api 3.3 权限矩阵 20 个 ✗ 项逐项负空间对应（含 ADR-018 禁上传下载专项：上传按钮+拖拽监听+下载按钮不渲染）；公共页定义单一事实源 = admin 文档 |
| v1.1 | 2026-11-14 | update | Gate 1 WARN 收尾随 admin v1.1 联动（W1~W3 修复主体在 admin 文档，本文档维持子集视角）：第 4 节引用行扩为「第 4 节（files）+ 第 4A 节（trash）」，trash 工具栏/操作列差异行标注 admin 第 4A.1/4A.3 节引用（trash 公共定义回归 admin 单一事实源，消除「子集文档反客为主」），files Dialog 差异行 8 类→9 类（4.4.1~4.4.9，压缩 Dialog 补全随负空间不变）。机械复验摘录（grep 实跑 2026-11-14）：`grep -c "targetName" readonly` = 0（exit=1——readonly 无写表单，targetName 无落点为正确负空间）；admin 侧三处修正复验输出见 admin 文档 v1.1 行 |

> FX 清单执行方式同 admin 文档第 9 节注记（清单文件未随 payload 发布，按 9 节结构落实 + 第 8 节自查；全量留待 Gate 1）。
