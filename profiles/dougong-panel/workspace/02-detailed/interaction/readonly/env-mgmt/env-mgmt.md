# PageSpec · env-mgmt · 只读用户（readonly）

| 属性 | 值 |
|------|-----|
| 版本 | v1.5 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-20（v1.5 Python 环境增量——app-mgmt D0 Q2 同步） |
| 用户类型 | readonly |
| 唯一数据源 | 02-detailed/api/api-env-mgmt.md v1.4（本文件为 admin 视角 v1.5 的只读负空间标注文档） |

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

同 admin 第 3 节 6 Key（env_type/env_task_action/env_task_status/package_manager_family/**bootstrap_status〔v1.3〕**/panel_role）；状态颜色方案同（含 bootstrap_status succeeded green/failed orange/skipped gray）。

## 第 4 节 列表页

### 4.1 环境管理（env-list，只读视图）— 路由 /env

- 页头通道状态条与刷新同等可用（观测能力）；generatedAt 呈现。
- 环境卡片（v1.1 七条目 +MongoDB；v1.4 八条目 +Node.js；v1.5 九条目 +Python）：实况字段（installed/version/服务状态/自启/在途标记）全部只读呈现，三态语义同 admin（null=条目级探测失败角标/false=未安装/true=已装版本，⚠️A9）；**Node 特化同 admin v1.4（⚠️A17）**：版本经 node --version（缺席 null 不虚构）；serviceRunning/serviceEnabled=null 为运行时型无服务单元——服务状态/自启区块不渲染（非探测失败语义）；**Python 特化同 admin v1.5（⚠️A18，⚠️A17 同构）**：版本经 python3 --version（缺席 null 不虚构）；服务状态/自启区块不渲染；venv/pip 能力位只读呈现（绿字「venv ✓ pip ✓」/orange「缺 venv」/「缺 pip」，与 admin 同数据同配色）；**操作区整区不渲染**（安装/升级/卸载/服务操作/自启开关，ADR-018 负空间——Node/Python 的安装/升级/卸载入口对 readonly 同样不渲染，服务操作入口对两角色均不渲染〔Node/Python 无服务单元〕）；未装条目无安装入口（纯「未安装」状态呈现）。

### 4.2 任务列表与详情（只读视图）

- 任务 Drawer 同 admin 视角（筛选/列表列/详情区块一致，**含「引导」列与引导结果横幅，v1.3**）；行点击查看详情同等可用；
- **引导结果只读呈现（v1.3）**：bootstrapStatus 角标（列表）与详情横幅（succeeded=已就绪/failed=已安装·引导失败+原因/skipped=保守跳过）同等可看（ADR-018 全可看）；「前往数据库管理」链接 readonly 可达（数据库清单两角色可看）；**无「重新安装/升级」操作入口**（变更类负空间，恢复指引文案呈现但按钮不渲染）；
- 详情刷新按钮可用（含 running 态 5s 轮询——观测轮询非变更操作；终态跃迁触发全景/清单重拉，v1.2 同 admin 4.3）。

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
| 3 | 字典 Key 与池定稿同值（6 项，v1.3 +bootstrap_status；v1.4 env_type 八值 +node；v1.5 env_type 九值 +python 五方同值） | ✅ 第 3 节 |
| 4 | 实况与任务观测能力与 admin 同等（ADR-018 全可看，含引导结果只读呈现） | ✅ 4.1/4.2 |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-16 | initial | 首次生成：只读视角 1 页面（环境卡片只读 + 任务 Drawer）+ 3 只读 Action + 7 负空间标注 |
| v1.1 | 2026-11-17 | content_changed | 数据源升 api v1.1（admin v1.1 同步）：卡片七条目（+MongoDB）+ 实况三态只读语义同 admin（⚠️A9）；字典 env_type 七值（第 3 节引用 admin 第 3 节自动同步） |
| v1.2 | 2026-11-17 | content_changed | v0.1.4 反馈第 1 项同步：任务详情终态跃迁触发环境全景与任务清单重拉（readonly 观测语义同 admin v1.2 4.3，无新增 Action/路由） |
| v1.3 | 2026-11-17 | dict_changed + content_changed | 安装后配置就绪增量同步（数据源升 api v1.2，admin v1.3）：字典 +bootstrap_status（6 Key，第 3 节引用 admin 自动同步）；任务列表「引导」列与详情引导结果横幅只读呈现（ADR-018 全可看）；「前往数据库管理」链接可达（清单两角色可看）；「重新安装/升级」恢复按钮负空间不渲染（变更类）；3 只读 Action + 7 负空间不变（引导无新 HTTP Action） |
| v1.4 | 2026-11-19 | dict_changed + content_changed | Node 环境增量同步（数据源升 api v1.3，admin v1.4；db-tools Gate 2 W2 闭环）：字典 env_type 七值→八值（+node，第 3 节引用 admin 自动同步，四方同值）；卡片八条目（+Node.js）；Node 服务状态/自启 null 不渲染（运行时型 ⚠️A17）；Node 安装/升级/卸载入口 readonly 负空间不渲染（admin 专属变更）；3 只读 Action + 7 负空间不变（Node 无新 HTTP Action） |
| v1.5 | 2026-11-20 | dict_changed + content_changed | Python 环境增量同步（数据源升 api v1.4，admin v1.5；app-mgmt D0 Q2）：字典 env_type 八值→九值（+python，第 3 节引用 admin 自动同步，五方同值）；卡片九条目（+Python）；Python 服务状态/自启 null 不渲染（运行时型 ⚠️A17 同构）；venv/pip 能力位只读呈现（与 admin 同数据）；Python 安装/升级/卸载入口 readonly 负空间不渲染（admin 专属变更）；3 只读 Action + 7 负空间不变（Python 无新 HTTP Action） |
