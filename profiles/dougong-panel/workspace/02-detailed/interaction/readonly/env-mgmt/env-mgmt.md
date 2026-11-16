# PageSpec · env-mgmt · 只读用户（readonly）

| 属性 | 值 |
|------|-----|
| 版本 | v1.0 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-16 |
| 用户类型 | readonly |
| 唯一数据源 | 02-detailed/api/api-env-mgmt.md v1.0（本文件为 admin 视角 v1.0 的只读负空间标注文档） |

## 第 1 节 模块元信息

同 admin 视角元信息（页面/路由/数据源一致）；错误处理同 admin 第 1 节（ENV_001 通道横幅同等呈现——观测不受通道限制）。

## 第 2 节 Action 定义

| Action | 方法 | 路径 | 页面 | 说明 |
|--------|------|------|------|------|
| envList | GET | /env/env/list | env-list | 环境全景只读（目录+实况+通道） |
| envTaskList | GET | /env/task/list | env-list（任务 Drawer） | 任务台账只读 |
| envTaskDetail | GET | /env/task/detail | env-list（任务 Drawer） | 任务详情与输出尾部只读 |

**负空间（readonly 不可见 Action，7 个，ADR-018 矩阵逐项对应）**：envInstall / envUninstall（环境变更类）/ envServiceStart / envServiceStop / envServiceRestart / envServiceEnable / envServiceDisable（服务运维类）——前端按 role===admin 隐藏入口 + 后端 AUTH_008 双保险。

## 第 3 节 字典 Key 清单

同 admin 第 3 节 5 Key（env_type/env_task_action/env_task_status/package_manager_family/panel_role）；状态颜色方案同。

## 第 4 节 列表页

### 4.1 环境管理（env-list，只读视图）— 路由 /env

- 页头通道状态条与刷新同等可用（观测能力）；generatedAt 呈现。
- 环境卡片：实况字段（installed/version/服务状态/自启/在途标记）全部只读呈现；**操作区整区不渲染**（安装/升级/卸载/服务操作/自启开关，ADR-018 负空间）；未装条目无安装入口（纯「未安装」状态呈现）。

### 4.2 任务列表与详情（只读视图）

- 任务 Drawer 同 admin 视角（筛选/列表列/详情区块一致）；行点击查看详情同等可用；
- 详情刷新按钮可用（含 running 态 5s 轮询——观测轮询非变更操作）。

## 第 5 节 表单页

无（readonly 无任何表单入口；admin 版安装/卸载/服务确认弹窗对其负空间）。

## 第 6 节 详情页

无独立详情路由页（任务详情=Drawer，同 admin）。

## 第 7 节 路由定义

| 路由 | 页面 | meta |
|------|------|------|
| /env | env-list | `{requiresAuth:true, title:'环境管理'}` |

**侧边菜单**：环境管理（/env）同 admin（两角色同项，D-P18）。

## 第 8 节 全局一致性

| # | 检查 | 结果 |
|---|------|------|
| 1 | Action ↔ api 接口：3 只读 Action 全映射 + 7 负空间逐项对应（10 接口全集） | ✅ 第 2 节 |
| 2 | 负空间 7 个 Action 与 ADR-018 边界矩阵逐项对应 | ✅ 第 2 节 |
| 3 | 字典 Key 与池定稿同值（5 项） | ✅ 第 3 节 |
| 4 | 实况与任务观测能力与 admin 同等（ADR-018 全可看） | ✅ 4.1/4.2 |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-16 | initial | 首次生成：只读视角 1 页面（环境卡片只读 + 任务 Drawer）+ 3 只读 Action + 7 负空间标注 |
