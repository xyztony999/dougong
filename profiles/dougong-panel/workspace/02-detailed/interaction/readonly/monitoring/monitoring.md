# PageSpec · 系统监控（monitoring） · 只读用户（readonly）

| 属性 | 值 |
|------|-----|
| 版本 | v1.0 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-12 |
| 用户类型 | readonly |
| 唯一数据源 | 02-detailed/api/api-monitoring.md v1.0 |
| 公共页定义 | **与 admin 文档（../admin/monitoring/monitoring.md）完全一致**——全部页面/Card/控件/轮询纪律/错误处理逐字复用，无差异表（本模块无任何变更操作可禁用） |
| 边界依据 | ADR-018：全模块可查看 + 禁一切变更/终端/上传下载——本模块全部功能为查看类（api 全册仅 4 个 GET），只读巡检为本模块核心场景（D0/FL：两角色能力完全一致） |
| 组件体系 | Vue 3 + Naive UI + echarts（同 admin 文档） |

## 第 1 节 模块元信息

| 项 | 值 |
|----|-----|
| 模块编码 | monitoring |
| 业务类型 | 工具类（只读观测，同 admin 判定） |
| 页面类型 | hybrid ×2（监控概览 / 历史趋势） |
| 可见页面 | /monitoring、/monitoring/history——与 admin 完全一致（api 3.3 权限矩阵无 ✗ 项，两角色可见集相同） |
| 错误处理 | 与 admin 完全一致（AUTH_001/002 跳登录、MON_001~004 文案同源 api 3.4） |

## 第 2 节 Action 定义

| Action | 方法 | 路径 | 页面 | 说明 |
|--------|------|------|------|------|
| metricsSnapshot | GET | /monitoring/metrics/snapshot | monitoring | 3s 轮询，与 admin 一致 |
| metricsHistory | GET | /monitoring/metrics/history | history | 按需拉取（窗口/维度/口径切换 + 刷新按钮），与 admin 一致 |
| processesTop | GET | /monitoring/processes/top | （独立契约，默认不轮询） | 与 admin 一致 |
| systemInfo | GET | /monitoring/system/info | monitoring·系统信息 Card | 60s 慢轮询，与 admin 一致 |

**负空间声明**：**无负空间**——api-monitoring.md 3.3 权限矩阵中不存在任何「admin ✓ / readonly ✗」分叉项（本模块零写接口，ADR-018「禁变更/终端/上传下载」在本模块无可作用对象：无变更按钮可隐藏、无表单可禁用、无上传下载入口）。两视角 Action 集合、页面集合、控件集合完全相同；对照 panel-core 模式（readonly=admin 子集 + 控件禁用），monitoring 的 readonly 视图为**全等视角**而非子集视角——这是「只读展示型模块」的诚实形态，非权限缺口。

## 第 3 节 字典 Key 清单

与 admin 文档第 3 节完全一致（同一 enums.ts 常量源）：metric_window / metric_dimension / metric_scope（+panel_role 复用）；单位格式化与颜色方案同源（无状态 tag，仅 partial orange 提示）。

## 第 4 节 列表页 / 第 5 节 表单页

无列表页、无表单页（同 admin 第 4/5 节声明：零持久化工具类模块，无台账与表单语义；TOP 进程无操作列——进程操作不在产品范围）。

## 第 6 节 详情页

监控概览（/monitoring）与历史趋势（/monitoring/history）的 Card 划分、控件条、轮询纪律、echarts 渲染、清零边界呈现——**与 admin 文档第 6 节逐字一致**，此处不复写（单一事实源 = admin 文档；两角色渲染同一组件树，无 role 分支代码）。

## 第 7 节 路由定义

| 路由 | meta | 与 admin 差异 |
|------|------|--------------|
| /monitoring、/monitoring/history | `{requiresAuth:true}` | **无差异**——同路由同组件树（无 role 分支禁用逻辑；对比 panel-core readonly 的 disabled 分支，本模块不存在该分支） |
| 框架菜单 | — | menuList 下发的 monitoring 菜单项对 admin/readonly 同样可见（服务端按 ADR-018 全可看下发） |

## 第 8 节 全局一致性

| # | 检查 | 结果 |
|---|------|------|
| 1 | Action 与 admin 文档/API 文档一致，无 readonly 专属虚构接口 | ✅ 4 Action 全等（无子集差异——api 3.3 无 ✗ 项） |
| 2 | 负空间检查：api 3.3 权限矩阵 ✗ 项逐项对应 | ✅ 0 项 ✗（全等视角声明见第 2 节，非权限缺口） |
| 3 | 只读视图与 admin 视图共用页面/字段/控件定义（无字段差异、无禁用差异） | ✅ 第 6 节引用声明 |
| 4 | S8 字典同源 enums.ts（metric_window/metric_dimension/metric_scope 取值逐字一致） | ✅ 第 3 节 |
| 5 | 轮询纪律/清零边界呈现/不虚构数据纪律同 admin 第 8 节 6/7/8 项 | ✅（复用同一组件实现） |
| 6 | ADR-018 边界落法说明：禁变更/终端/上传下载在本模块无可作用对象 | ✅ 第 2 节负空间声明 |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-12 | initial | 首次生成：只读视角与 admin 视角**完全一致**（4 Action 全等、2 页面全等、无负空间、无禁用分支）——本模块零写接口使 ADR-018 差异面为空，公共页定义单一事实源 = admin 文档 |

> FX 清单执行方式同 admin 文档第 9 节注记（清单文件未随 payload 发布，按 9 节结构落实 + 第 8 节自查；全量留待 Gate 1）。
