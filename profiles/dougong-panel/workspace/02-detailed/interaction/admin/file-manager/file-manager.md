# PageSpec · 文件管理器（file-manager） · 管理员（admin）

| 属性 | 值 |
|------|-----|
| 版本 | v1.1 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-14 |
| 用户类型 | admin（readonly 视角为本文档子集，差异见 ../readonly/file-manager/file-manager.md） |
| 唯一数据源 | 02-detailed/api/api-file-manager.md v1.0 |
| 组件体系 | Vue 3 + Naive UI（NDataTable/NSelect/NInput/NButton/NSwitch/NTag/NPopconfirm/NModal/NDrawer/NAlert/NEmpty/NDescriptions/NProgress/NGrid/NUpload 触发/NCheckbox；在线编辑器=Monaco Editor 深度集成；无激活栈 profile，不使用 vue3-engine 栈 Scm* 组件） |
| 确认方式 | headless run：7 项交互确认依据 D3 契约（api-file-manager.md D-01~D-16）+ ADR-018 + ⚠️A12 + FL v1.0 推导，标注于各节 |

## 第 1 节 模块元信息

| 项 | 值 | 确认依据 |
|----|-----|---------|
| 模块编码 | file-manager（域前缀 /files） | api 文档 3.1 |
| 业务类型 | **混合形态（诚实标注）**：主数据型聚合根 trash_entry（in_trash→restored/purged 生命周期，回收站页承载）+ 工具类文件动作集合（file_entry 实时投影零台账，D2/D3 判定）——分册 2.1 三型之外，与 service-mgmt「诚实标注」同口径 | api 1.4；domain-model.md file-manager 域 |
| 页面类型 | **config ×2 + code ×1**：files（文件管理列表页，标准列表：无 Tab/无左树右表——回收站为独立路由页非 Tab，不触发 hybrid）、trash（回收站列表页，config）、edit（在线编辑页，**code**——Monaco 第三方组件深度集成，分册 2.2 触发条件） | 分册 2.2；api 3.1 |
| UI 框架 | Naive UI + Monaco Editor（edit 页） | 栈版本表 |
| 状态管理 | Pinia：fileStore（currentPath/breadcrumb/viewMode 列表图标/sortParams/entries/generatedAt/inFlightSet/selection 多选/目录选择器态）；trashStore（trashItems/total/inFlightSet）；editorStore（path/content/dirty/savedMeta）；枚举常量 store（与 account-auth/monitoring/service-mgmt 共用 enums.ts） | 栈版本表 |
| 错误处理 | 统一拦截器按 api 3.4 code 分流：AUTH_001/002→跳登录（全局）、AUTH_003→CSRF 提示（全局）、AUTH_008→「权限不足：只读角色禁止该操作」（全局边界拒绝，正常 UI 无入口不触发）、FILE_001/007/008/010/015→「参数非法」兜底提示（正常 UI 由控件约束不可达，api R-14）、FILE_002→error「该路径不受管理（越界或命中排除目录）」+ data.reason（D-01；正常导航不可达，符号链接外指等兜底）、FILE_003→warning「条目不存在（可能已被删除）」+ 自动刷新列表、FILE_004→warning「{目标} 已存在」（D-13 无覆盖语义动作）、FILE_005→**冲突决策 Modal**（4.4.8，D-07 核心交互）、FILE_006→warning「该文件为二进制或超过 1MiB，请下载后本地编辑」（D-05）、FILE_009→error「压缩包包含越界条目（安全拦截），已整体取消解压」（zip slip，D-07）、FILE_011→warning「超过单请求 5MiB 上限」（兜底，前端预判走分块）、FILE_012→warning「目标目录不存在」+ 刷新、FILE_013→error「文件系统错误：{data.reason}」（磁盘满/权限等如实呈现）、FILE_014→warning「回收站条目不存在或已处理」+ 刷新回收站 | api 3.4 + D-01/D-05/D-07/D-13 |

## 第 2 节 Action 定义（与 API 一一对应）

| Action | 方法 | 路径 | 页面 | 权限 | CSRF |
|--------|------|------|------|------|------|
| pathScopeGet | GET | /files/path-scope/get | files·路径边界设置 Dialog | admin | —（GET） |
| pathScopeUpdate | POST | /files/path-scope/update | files·路径边界设置 Dialog | admin | ✓ |
| entryList | GET | /files/entry/list | files（列表主数据）+ 移动/解压目录选择器 | admin+readonly | —（GET） |
| entryUpload | POST | /files/entry/upload | files·工具栏上传（≤5MiB 分支） | admin | ✓ |
| chunkInit | POST | /files/upload/chunk-init | files·工具栏上传（>5MiB 分支第 1 步） | admin | ✓ |
| chunkUpload | POST | /files/upload/chunk | files·工具栏上传（>5MiB 分支第 2 步，逐块循环） | admin | ✓ |
| chunkComplete | POST | /files/upload/chunk-complete | files·工具栏上传（>5MiB 分支第 3 步） | admin | ✓ |
| entryDownload | GET | /files/entry/download | files·行操作「下载」 | admin | —（GET 只读） |
| entryContent | GET | /files/entry/content | edit 页进入加载 | admin | —（GET 只读） |
| entryContentSave | POST | /files/entry/content-save | edit 页「保存」 | admin | ✓ |
| entryCreate | POST | /files/entry/create | files·工具栏「新建」Dialog | admin | ✓ |
| entryRename | POST | /files/entry/rename | files·行操作「重命名」Dialog | admin | ✓ |
| entryMove | POST | /files/entry/move | files·行操作「移动」Dialog | admin | ✓ |
| entryDelete | POST | /files/entry/delete | files·行操作「删除」 | admin | ✓ |
| entryChmod | POST | /files/entry/chmod | files·行操作「权限」Dialog | admin | ✓ |
| entryChown | POST | /files/entry/chown | files·行操作「属主」Dialog | admin | ✓ |
| systemAccountsList | GET | /files/system-accounts/list | files·属主 Dialog 候选数据源 | admin | —（GET） |
| entryArchive | POST | /files/entry/archive | files·工具栏「压缩」Dialog（4.4.9） | admin | ✓ |
| entryUnarchive | POST | /files/entry/unarchive | files·行操作「解压」Dialog（归档行） | admin | ✓ |
| trashList | GET | /files/trash/list | trash（列表主数据） | admin+readonly | —（GET） |
| trashRestore | POST | /files/trash/restore | trash·行操作「还原」 | admin | ✓ |
| trashPurge | POST | /files/trash/purge | trash·工具栏「清空」 | admin | ✓ |

> S7 一致性：22 Action 与 api-file-manager.md 22 接口一一对应（一接口一 Action，无未覆盖接口、无虚构接口）。CSRF Token 由全局拦截器统一注入 `X-CSRF-Token`（16 个 POST 全覆盖，api 3.3 矩阵）。

**刷新纪律**（api D-03「按需拉取」契约的前端落法；写操作响应多不含投影快照，与 service-mgmt「快照直更」的差异点）：

| 触发 | 行为 |
|------|------|
| 进入页面 / 点击「刷新」/ 面包屑与上级导航 | entryList 重拉（loading 态）；trash 页同构 trashList |
| **改变目录内容**的操作成功（upload/chunkComplete/create/rename/move/delete/archive/unarchive/restore） | **重拉当前列表**（响应无投影快照，api 4.2.4；操作驱动刷新，D-03） |
| **仅改元数据**的操作成功（chmod/chown/content-save） | 以响应新值直更对应行（mode/mtime/size，api 响应含刷新值），不整表重拉 |
| pathScopeUpdate 成功 | 关闭 Dialog + message「已生效」；当前目录可能变为越界（排除自身）→ FILE_002 页面提示兜底（api FR-02） |
| 无自动轮询 | 不设定时轮询（D-03：按需拉取；文件系统无推送语义，手动+操作驱动） |
| purge 成功 | message「已清空 N 条」+ 重拉 trashList |
| 页面卸载 | 清理 inFlight/目录选择器/editor 态；edit 页有 dirty 拦截（第 5 节） |

## 第 3 节 字典 Key 清单

| Key | 取值（=01-solution/04-business-enum.md 定稿） | 用途 |
|-----|--------------------------------------------|------|
| file_type | file / directory / symlink | entryType 列 dict + 图标映射（第 4.2 节） |
| archive_format | tar / tar.gz / zip | 压缩 Dialog format NSelect（第 4.4.9 节）+ 解压行识别（第 4.4.6 节） |
| panel_role | admin / readonly | 菜单可见性判定（authStore 复用；readonly 为子集视角，见 readonly 文档） |
| trash_entry_status | in_trash / restored / purged | **不进入 UI 字典**——trash/list 恒为 in_trash 子集（api I-02 服务端过滤），页面无状态列；登记于此仅作枚举池对齐声明，不渲染 |

局部常量（非枚举池条目，前端 TS 常量）：回收站条目类型 `file / directory`（api 4.1.1 内联 CHECK 两值，与 file_type 值域不同——symlink 删除归 file）；排序域 `name / size / mtime` + `asc / desc`；冲突策略 `ask / overwrite / skip`（还原 `ask / overwrite`）。

**数据源规范偏差声明**（同 account-auth/panel-core/monitoring/service-mgmt 先例）：不实现 `POST /{domain}/dict/batch`（分册 3.6）——枚举全部固定值，字典由前端 TS 常量提供（enums.ts 单一事实源，共用同一文件）。此偏差为设计决策，待 Gate 1 复核。

**状态/类型颜色方案**（分册语义映射）：

| 值 | 颜色/图标 | 语义依据 |
|----|----------|---------|
| entryType=directory | 文件夹图标（蓝灰） | 目录 |
| entryType=symlink | 链接图标（紫色）+ 名称尾注「→ 目标」（列表不展开目标，api 4.2.4） | 符号链接 |
| entryType=file | 按扩展名文件图标映射表（.md/.json/.sh/.go/.py/.js/.log/归档类/默认文档图标）；未识别扩展名→默认文件图标 | 文件 |
| 回收站 entryType | file/directory 两值同上映射（api 内联两值） | — |

## 第 4 节 列表页（核心页，/files）

**权限**：admin 全功能；readonly 为子集（无工具栏写按钮、无行写操作、无路径边界设置，见 readonly 文档）。

### 4.1 工具条（确认项 2 的落位）

| 区 | 控件 | 组件 | 行为 |
|----|------|------|------|
| 导航区 | 面包屑 | NBreadcrumb | currentPath 逐段拆分（受管根为根段）；逐级点击=切换目录重拉 entryList；上级按钮（NArrowUp 图标）；**无路径输入框**（api OUT-6 从紧把握） |
| 排序区 | 排序字段 + 方向 | NSelect + NButton（asc/desc 切换） | name/size/mtime（默认 name/asc，api D-03）；变更即重拉（服务端排序） |
| 视图区 | 列表/图标切换 | NButtonGroup | viewMode 切换（P3 前端呈现态，不调接口）；列表=NDataTable，图标=NGrid 卡片（4.2） |
| 操作区（admin） | 上传 | NButton + 隐形 file input multiple + 全表格区拖拽放置（dragover 高亮） | 走上传管线（4.5；>5MiB 自动分块分支） |
| 操作区（admin） | 新建 | NButton → 4.4.1 Dialog | — |
| 操作区（admin） | 压缩 | NButton（selection≥1 时可用）→ 4.4.9 Dialog | 多选行打包 |
| 操作区（admin） | 路径边界设置 | NButton（齿轮图标）→ 4.4.7 Dialog | ⚠️A12 |
| 信息区 | 刷新 | NButton（loading 防抖） | 重拉 entryList |

表格上方灰字「数据生成于 {generatedAt 本地时区}」；selection 状态提示「已选 N 项」（多选框列承载压缩与批量删除——批量删除=逐行 entryDelete 循环 + 单一确认，见 4.3）。

### 4.2 表格列（NDataTable，确认项 3；图标视图=NGrid 卡片：图标+名称，操作经选中后工具栏，与列表视图同一 Action 集）

| 列 | 类型 | 宽度 | 说明 |
|----|------|------|------|
| 选择 | checkbox | 50 | 多选（admin；readonly 不渲染选择列——无批量操作场景） |
| 名称 | link + 图标 | 260 fixed:left | items[].name（前置文件图标，第 3 节映射）；点击行为分流：directory→进入目录（重拉）；symlink→无动作（不展开目标，api 4.2.4）；file→打开编辑页（第 5 节；仅 admin——readonly 侧 file 点击无动作，编辑为其负空间） |
| 类型 | dict(file_type) + 图标 | 100 | items[].entryType |
| 大小 | text（右对齐） | 120 | items[].size 人类可读格式（B/KB/MB，前端格式化；目录恒 0，api 4.2.1） |
| 权限 | text | 110 | items[].mode 八进制原值（如 `0644`；rwx 分组呈现归后续迭代，PageSpec v1.0 从简） |
| 属主 | text | 160 | items[].ownerUser / ownerGroup |
| 修改时间 | text | 180 | items[].mtime 本地时区 |
| 操作 | action | 320 fixed:right | 见 4.3（readonly 视角整列不渲染） |

无分页（全量返回 D-03）、排序控件在工具条（服务端排序）；total 展示（「共 N 项」）；空目录 → NEmpty「此目录为空」（常态非错误）；加载中 → 表格骨架；FILE_002 → 页面级 NAlert error「该路径不受管理（越界或命中排除目录）」+「返回受管根」按钮（重置到受管根重拉）。

### 4.3 操作列（operList，确认项 4）

**按钮集合与可见性**（前端维护完整按钮集；可见性由条目类型权威值 entryType + 归档扩展名识别控制——api 无逐条目 canOperate 布尔，类型判断为前端呈现约束、服务端各自校验兜底，R-14 职责分离同 service-mgmt 先例）：

| 按钮 | 可见性条件 | 二次确认 | 调用 |
|------|-----------|---------|------|
| 下载 | entryType=file（目录打包下载 OUT-3 不提供；symlink 不提供——下载目标为链接本体无意义，从紧） | 无 | entryDownload（window 触发流下载；下载动作经 api 记 security 审计，D-08，前端无感知） |
| 编辑 | entryType=file | 无 | 跳转 edit 页（第 5 节；FILE_006 由 entryContent 加载时返回） |
| 重命名 | 恒显 | 无（同名 FILE_004 直接反馈，D-13） | 4.4.2 Dialog |
| 移动 | 恒显 | 无（冲突走 4.4.8 决策） | 4.4.3 目录选择器 Dialog |
| 解压 | 归档扩展名识别（.tar/.tar.gz/.zip，第 3 节局部映射） | 无（越界 FILE_009 安全拦截、冲突走 4.4.8） | 4.4.6 Dialog |
| 权限 | 恒显 | 无 | 4.4.4 Dialog |
| 属主 | 恒显 | 无 | 4.4.5 Dialog |
| 删除 | 恒显 | NPopconfirm「确认删除？条目将移入回收站，可在回收站还原。」 | entryDelete（R-17 可逆删除，轻确认即可） |

批量删除：selection>1 时工具栏出现「删除所选」（NPopconfirm「确认删除所选 N 项？将移入回收站。」）→ 逐行 entryDelete 循环（inFlight 汇总进度）→ 完成后统一重拉；部分失败（FILE_013/003）汇总呈现失败清单、成功项不回滚（各自已入回收站）。

**在途互斥**（PRD N2/P3）：行级 inFlightSet 登记——该行全部操作按钮置 loading/禁用；动作完成（成功或错误终态）解除；无假进度。

### 4.4 Dialog 集合（确认项 5；全部 <5 字段 → NModal Dialog，分册 3.2 表单类型规则）

#### 4.4.1 新建（entryCreate）

| 字段 | 组件 | 校验 |
|------|------|------|
| 类型 | NRadioGroup（文件/目录） | 必选 |
| 名称 | NInput | 必填；不含路径分隔符（FILE_007 兜底，前端预校验分隔符与空值） |

按钮：创建/取消。成功 → message + 重拉。FILE_004 → warning「{名称} 已存在」（D-13）。

#### 4.4.2 重命名（entryRename）

| 字段 | 组件 | 校验 |
|------|------|------|
| 新名称 | NInput（预填当前名） | 同上 |

成功 → 重拉。FILE_004 → warning「目标名称已存在」。

#### 4.4.3 移动（entryMove，目录选择器）

NModal（内嵌 mini 目录浏览器）：面包屑（起始=当前浏览目录）+ 子目录列表（entryList 结果过滤 entryType=directory，点击进入）+「选择此目录」确认按钮。确认 → entryMove `{sourcePath, targetDir, onConflict:"ask"}`。移动到自身/自身子目录 → 服务端 FILE_013/行为拒绝如实呈现（api 未单独建模，从服务端错误如实反馈；不做前端预判虚设规则）。冲突 → 4.4.8。

#### 4.4.4 权限（entryChmod）

| 字段 | 组件 | 校验 |
|------|------|------|
| 权限九宫格 | 3×3 NCheckbox（所有者/组/其他 × 读/写/执行；预勾选自 mode 解析） | 八进制合成显示随勾选联动 |
| 八进制值 | 只读展示 | 合成值 ∈ 0~0777（FILE_015 兜底） |

不提供递归选项（D-06：chmod 仅作用目标自身）。成功 → 行 mode 直更（响应刷新值）。

#### 4.4.5 属主（entryChown）

| 字段 | 组件 | 校验 |
|------|------|------|
| 属主用户 | NSelect（候选 systemAccountsList.users，可清空） | 至少填一项 |
| 属主组 | NSelect（候选 systemAccountsList.groups，可清空） | 同上 |

候选清单打开 Dialog 时加载一次（D-14 实时读取；不缓存）。成功 → 行 owner 直更。

#### 4.4.6 解压（entryUnarchive，仅归档行）

| 字段 | 组件 | 校验 |
|------|------|------|
| 目标目录 | 目录选择器（同 4.4.3，起始=归档所在目录） | 必选 |
| 冲突策略 | NRadioGroup（逐项询问 ask[默认]/覆盖 overwrite/跳过 skip） | api ON_CONFLICT 取值域 |

确认 → entryUnarchive。FILE_009 → error 安全拦截提示（整体取消，零落盘，D-07）；FILE_005 → 4.4.8（ask 分支）。

#### 4.4.7 路径边界设置（pathScopeGet/pathScopeUpdate，⚠️A12）

| 字段 | 组件 | 校验 |
|------|------|------|
| 受管根 | NInput（预填当前值） | 绝对路径形态（FILE_001 兜底）；前端预校验 |
| 排除目录 | NDynamicTags（逐项添加/删除；预填拆分） | 每项绝对路径；不含受管根本身（前端预校验 + FILE_002 兜底，api 4.2.4） |

打开时 pathScopeGet 加载；保存 → pathScopeUpdate → 成功 message「配置已保存并生效」（D-02 同事务审计+副本刷新，前端无感）+ 关闭。

#### 4.4.8 冲突决策 Modal（FILE_005 统一处理，D-07 前端落法）

| 要素 | 行为 |
|------|------|
| 触发 | 任一写 Action 返回 FILE_005（data.conflicts[]） |
| 呈现 | NModal 列出冲突项（名称/类型；还原场景为原路径占用项）；说明文案「目标位置存在同名条目」 |
| 选项 | 上传/移动/解压：「覆盖全部」→ 重发 onConflict=overwrite；「跳过全部」→ 重发 onConflict=skip（还原无此项）；「取消」→ 关闭不重发（放弃；还原场景语义=放弃，api D-07） |
| 落盘保证 | ask 分支服务端零落盘（api D-07 预检），前端无「部分完成」中间态 |
| 重发后 | 按原 Action 刷新纪律收尾（4.x 各节） |

#### 4.4.9 压缩（entryArchive，Gate 1 W2 修复补全）

| 字段 | 组件 | 校验 |
|------|------|------|
| 打包对象 paths | 只读摘要「已选 N 项」（4.1 多选框 selection 承载，不逐项罗列不可编辑） | 进入 Dialog 前由工具栏按钮 selection≥1 保证；req 以 selection 对应 items[].path[] 提交（api 4.2.4 paths[]） |
| 格式 format | NSelect（archive_format 三选一：tar / tar.gz / zip；必选，默认预选 zip 为 UI 呈现选择——请求恒带 format，api req 必填无缺省） | 取值 ∈ archive_format（FILE_008 兜底，正常 UI 由 NSelect 约束不可达） |
| 产物名 targetName | NInput（选填；placeholder「留空 = archive.{ext}（按格式）」） | 不含路径分隔符（FILE_007 兜底，前端预校验）；留空=服务端缺省名 `archive.{ext}` 按 format 扩展名生成（api 4.2.4） |

确认 → entryArchive `{paths[], format, targetName?}`；产物落**当前浏览目录**（含目录条目递归打包，api 4.2.4）。成功 → message「已压缩 N 项 → {data.path}」+ 重拉当前列表（刷新纪律「改变目录内容」行）；FILE_004 → warning「{产物名} 已存在」（**D-13 同名直接错误、无覆盖决策**——非 D-07 冲突决策流，不触发 4.4.8）；FILE_002 → error 越界提示（paths 任一越界整体不执行，第 1 节兜底）。

### 4.5 上传管线（工具栏/拖拽统一入口；api D-04/D-10 前端落法）

| 分支 | 条件 | 流程 |
|------|------|------|
| 直传 | 所有文件 ≤5MiB | entryUpload（multipart：dir + files + onConflict=ask）→ FILE_005 冲突决策 → 成功 message「已上传 N 个文件」+ 重拉 |
| 分块 | 任一文件 >5MiB | 逐文件：前端按 5MiB 切片 → chunkInit（dir/fileName/totalSize/totalChunks）→ chunk 循环（uploadId/index/chunk；顺序上传，NProgress=received/total）→ chunkComplete（合并落盘）→ 下一文件 |
| 失败 | chunk 任一块 FILE_010/013 | 终止该文件管线 → error 如实呈现（api D-10：中断即整任务作废，重传=重新走 chunkInit；无断点续传 OUT-4，不虚设续传 UI） |
| 进度 | 分块分支 | NProgress 按已完成块数推进（前端计算，非服务端推送；P2 展示数据）；直传为按钮 loading |
| 冲突 | 分块分支 | chunkInit 阶段同名预检（api 4.2.4 chunk-init 同名预检）→ FILE_005 → 4.4.8（ask 重发=重新 chunkInit 携带 overwrite？——**api chunk-init 无 onConflict 参数**，冲突决策仅直传/移动/解压/还原承载；分块分支遇 FILE_005 呈现冲突提示 + 用户改名或取消（不提供覆盖决策——诚实对齐 api 契约，不虚构参数）） |

> 4.5 冲突行修正声明：api chunk-init 同名预检返回 FILE_005 但请求体无 onConflict（api 4.2.4）——前端落法=提示「目标已存在同名文件」+ 用户修改文件名（本地重命名后重传）或取消；此为 api 契约的如实承接，PageSpec 不虚构 overwrite 通道。若后续迭代需分块覆盖决策，属 API 变更（interface_changed），经变更管理联动。

## 第 4A 节 回收站列表页（/files/trash，Gate 1 W1 修复补全）

**权限**：admin 全功能（还原/清空）；readonly 子集——仅浏览列表（无清空按钮、无操作列，见 readonly 文档差异表）。数据源 trashList（GET /files/trash/list，第 2 节 Action 表）。

### 4A.1 工具栏

| 控件 | 组件 | 行为 |
|------|------|------|
| 刷新 | NButton（loading 防抖） | 重拉 trashList（刷新纪律表第 1 行「trash 页同构」） |
| 清空（admin） | NButton（danger 类型）→ **NModal 强警告确认** | 确认文案「清空回收站为**终局销毁**：全部 {total} 条条目将被永久删除，不可还原、不可恢复。确认清空？」；确认 → trashPurge（无 req，清空全部在站条目，api 4.1.4）；取消 → 关闭不请求 |

- 清空成功 → message「已清空 {data.purgedCount} 条」+ 重拉 trashList（刷新纪律表 purge 行）；空站（total=0）时清空按钮禁用（服务端幂等语义仍在——purge 空站幂等成功 purgedCount=0，api N3；前端禁用仅为避免无意义请求）。
- **两级确认口径**（R-17）：purge 为物理删除（记录不留行，销毁事实由审计 trash_purged 留痕，api I-02）→ 强警告 NModal（红字「不可还原」+ 条数明示）；对比 4.3 删除=NPopconfirm 轻确认（可逆——条目入回收站）。

### 4A.2 表格列（NDataTable）

无排序控件（deleted_at 倒序服务端固定，api D-03）；无分页（全量返回 D-03）；无选择列（还原为逐行操作、清空为全站动作，无多选场景）。

| 列 | 类型 | 宽度 | 说明 |
|----|------|------|------|
| 原路径 | text（前置文件图标，第 3 节回收站 entryType 颜色映射） | 360 fixed:left | items[].originalPath（还原目标，api 4.1.1）；**只读展示、点击无动作**——原路径当前存在与否未知，不做导航跳转（与 4.2 名称列的差异，如实承接） |
| 类型 | 局部常量 + 图标 | 100 | items[].entryType（**file/directory 两值内联域**，与 file_type 值域不同，第 3 节局部常量声明；symlink 删除归 file，api 4.1.1） |
| 大小 | text（右对齐） | 120 | items[].size 人类可读格式（删除入站时快照——目录为累计值，与 4.2 实时投影「目录恒 0」的差异，api 4.1.1） |
| 删除人 | text | 140 | items[].operatorName（D-12 操作者快照：面板用户改名/删除后仍可读） |
| 删除时间 | text | 180 | items[].deletedAt 本地时区（倒序首列语义） |
| 操作（admin） | action | 120 fixed:right | 见 4A.3（readonly 整列不渲染） |

- **items[].id 不渲染**——服务端 trashId 寻址键，仅供还原请求 trashRestore req `{trashId}` 使用（api 4.1.4）；前端行键可用 id，不作为可见列。
- total 展示「共 N 条」；空站 → NEmpty「回收站为空」（常态非错误）；加载中 → 表格骨架。
- 孤儿记录（trash 目录内容已被面板外干预删除）由服务端列表对账剔除（api 4.1.4「trash 实况对账」，PRD N2），前端不建模孤儿态。
- trash/list 响应无 generatedAt（api 4.1.4：data={items, total}）——**不渲染**「数据生成于…」灰字（与 files 页差异，不虚构字段）。

### 4A.3 操作列（admin；readonly 整列不渲染）

| 按钮 | 可见性条件 | 二次确认 | 调用 |
|------|-----------|---------|------|
| 还原 | 恒显（在站条目恒可还原，api I-02） | 无（冲突走 4.4.8 决策，非轻/强确认） | trashRestore `{trashId, onConflict:"ask"}` → 成功 message「已还原至 {data.originalPath}」+ 重拉 trashList 与原目录列表（刷新纪律「改变目录内容」行含 restore）；**FILE_005 → 4.4.8 冲突决策 Modal**（还原场景选项=「覆盖全部」重发 onConflict=overwrite /「取消」=放弃——**无「跳过全部」**，api D-07）；FILE_002 → error「原路径已不受管理（白名单可能已收紧）」+ 重拉（第 1 节兜底）；FILE_014 → warning「回收站条目不存在或已处理」+ 重拉（第 1 节） |

## 第 5 节 在线编辑页（edit，code 类型页，/files/edit?path=…）

**页面类型依据**：Monaco Editor 第三方组件深度集成（分册 2.2 code 触发条件）；独立路由而非 Dialog——编辑器需要完整工作区，且运维场景常对照多个文件。

| 环节 | 行为 |
|------|------|
| 进入 | 读取 query path → entryContent（loading 态）→ 成功：Monaco 加载 content（language=扩展名映射表：.js/.ts/.go/.py/.sh/.md/.json/.yaml/.conf/.ini/.sql 等，未识别→plaintext，诚实声明局部映射）；FILE_006 → warning + router.back（二进制/超限拒绝进入，D-05）；FILE_002/003 → 错误提示 + 返回 |
| 保存 | NButton「保存」（dirty 时可用，Ctrl/Cmd+S 同义）→ entryContentSave（全量覆盖，api I-09）→ 成功 message「已保存」+ savedMeta 更新（响应 mtime/size）；FILE_011/013 → error，编辑器内容保留待重试（api N2：不合并不自动解决） |
| dirty 检测 | 编辑内容≠加载内容 → 路由离开/beforeunload 拦截 confirm「有未保存更改，确认离开？」（P3 前端态；无自动保存、无草稿——OUT-2 无版本历史） |
| 头部 | 文件路径只读展示 + savedMeta（大小/修改时间）+「返回列表」按钮 |
| 权限 | admin only——readonly 不进入此页（负空间：readonly 侧 file 点击无动作，第 2 节 readonly 文档） |
| 无版本历史/协同 | 不渲染历史/协作 UI（OUT-2；api D-05 最后写者胜，无冲突合并界面） |

## 第 6 节 详情页

**N/A——无独立详情页**：api 无 detail 类接口（22 接口无单点属性查看；条目属性全量呈现于列表列 4.2，文件内容查看经编辑页第 5 节承载）。分册 3.3 详情页推断不触发（无详情数据源）。确认项 6 落定：Card 划分 N/A。

## 第 7 节 路由定义与框架行为

| 路由 | 页面 | meta |
|------|------|------|
| /files | files（文件管理列表页：工具条/表格/图标视图/Dialog 集合/上传管线） | `{requiresAuth:true}` |
| /files/trash | trash（回收站列表页，第 4A 节） | `{requiresAuth:true}` |
| /files/edit | edit（在线编辑页，code 类型，query path 必带） | `{requiresAuth:true}` |

**守卫**：复用全局守卫（initCheck → sessionCurrent → roles）；edit 页组件内校验 query path 存在性（缺失→返回列表）。

**框架级边界**（复用 panel-core 框架行为，同 service-mgmt 先例）：菜单渲染/安全入口前缀透明/顶栏归 panel-core 宿主；3 个路由经模块页面挂载点接入（panel-core S-10）；菜单项由 ModuleMenuRegistry 登记、服务端按角色下发——「文件管理」菜单对 admin/readonly 均可见（readonly 可浏览），「回收站」子项同（trash/list 两角色 ✓）。

## 第 8 节 全局一致性

| # | 检查 | 结果 |
|---|------|------|
| 1 | Action ↔ api-file-manager.md 接口一一对应（S7） | ✅ 22/22 接口全映射（第 2 节表），无虚构、无遗漏 |
| 2 | 字典 Key ↔ 04-business-enum.md 定稿枚举（S8） | ✅ file_type/archive_format 引用逐字一致 + panel_role 复用；trash_entry_status 声明不进 UI（I-02 服务端过滤）无虚假渲染；回收站两值类型为局部常量（api 内联 CHECK，非池枚举）已声明 |
| 3 | 写 Action CSRF 注入完整（16 POST 全 ✓） | ✅ 第 2 节 + 全局拦截器 |
| 4 | 冲突决策 D-07 落法：FILE_005 统一 Modal、ask 零落盘、还原无 skip、分块分支无覆盖通道（如实承接 api 契约） | ✅ 第 4.4.8/4.5 节 |
| 5 | 上传管线 D-04/D-10：5MiB 阈值、三段式分块、中断作废无续传 UI（OUT-4） | ✅ 第 4.5 节 |
| 6 | 编辑页 D-05/D-16：1MiB+二进制拒绝进入、全量覆盖保存、dirty 拦截、无版本历史 UI（OUT-2） | ✅ 第 5 节 |
| 7 | 刷新纪律符合 D-03：手动+操作驱动、无轮询；目录内容类操作重拉、元数据类直更（响应含新值） | ✅ 第 2 节纪律表 |
| 8 | FILE_001~015 + AUTH_001/002/003/008 错误文案与 api 3.4 一一对应 | ✅ 第 1 节错误处理 + 4.x/第 5 节 |
| 9 | ⚠️A12 落法：路径边界设置 Dialog（pathScopeGet/Update），readonly 无入口 | ✅ 第 4.4.7 节 + readonly 文档 |
| 10 | R-17 落法：删除=轻确认移入回收站（可逆），purge=强警告终局销毁 | ✅ 第 4.3 节（删除 NPopconfirm 轻确认）+ 第 4A.1 节（清空 NModal 强警告，两级确认口径）+ 4.4.8（还原冲突决策） |
| 11 | ADR-018：readonly 为子集视角（工具栏写按钮/行写操作/编辑页/边界设置整组不渲染），20 个 ✗ 项逐项负空间 | ✅ readonly 文档对应 |
| 12 | 无 📎 Scm* 组件；Naive UI+Monaco 为栈版本表选型；时间本地时区；枚举原值透传不翻译 | ✅ 头部组件体系 + 第 3/4.2 节 |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-12 | initial | 首次生成：3 页面（files 列表 config + trash 回收站 config + edit 在线编辑 code/Monaco）、22 Action（6 读 + 16 写，与 api 一一对应）、2 池枚举 + panel_role 复用（trash_entry_status 声明不进 UI）、8 类 Dialog（新建/重命名/移动目录选择器/权限九宫格/属主/解压/路径边界设置/冲突决策统一 Modal）、上传管线直传+分块双分支、刷新纪律（内容类重拉/元数据直更）；冲突决策（D-07）、分块 5MiB（D-04/D-10）、编辑 1MiB+二进制（D-05）、删除入站轻确认 vs 清空强警告（R-17）为本阶段核心交互定义 |
| v1.1 | 2026-11-14 | update | Gate 1 WARN 收尾（05-reports/gate1-file-manager-2026-11-12.md W1~W3，Action 集/接口契约零变更）：**W1** 补第 4A 节回收站列表页（4A.1 工具栏=刷新+清空 NModal 强警告「终局销毁不可还原」文案+空站禁用、4A.2 列定义对照 api trash/list items 六键 id/originalPath/entryType/size/operatorName/deletedAt——id 不渲染、deleted_at 倒序无排序控件、无 generatedAt 不虚构、4A.3 还原行操作触发既有 4.4.8 冲突 Modal[还原无 skip]），第 7 节路由行与第 8 节自查项 10 指向同步修正（原「第 4 节对话框」失准指向已除）；**W2** 补 4.4.9 压缩 Dialog（paths=selection 只读摘要、format=NSelect archive_format 三选一、targetName=选填 NInput 缺省 archive.{ext}、FILE_004 同名直接错误 D-13 无覆盖不触发 4.4.8），修正 4.1 工具条（原错指 4.4.4）与 Action 表 entryArchive 行（补 4.4.9 编号）两处指向；**W3** 第 3 节 archive_format 行解压指向 4.4.5→4.4.6 并补压缩 Dialog 4.4.9 指向。机械复验摘录（grep 实跑 2026-11-14，修复后行号）：`grep -n "4A"`=8 行（:231 节标题/:235 4A.1/:245 4A.2/:256/:263 4A.3/:291 路由行/:311 自查项 10）；`grep -n "第 4 节对话框"` exit=1 无残留；`grep -n "4\.4\.9"`=4 行（:45 Action 表/:70 dict 行/:100 工具条/:209 小节标题）；`grep -c "targetName"`=2（4.4.9 字段行+确认段，原 0 落点已补）；`grep -n "4\.4\.4"`仅剩 :132（4.3 权限行正确指向）+:163（4.4.4 标题自身）；`grep -n "第 4\.4\.5 节"` exit=1、`grep -n "第 4\.4\.6 节"`=1 行（:70） |

> FX 质量清单说明：FX-001~FX-141 逐条清单文件未随引擎 0.5.2 payload 发布（account-auth/panel-core/monitoring/service-mgmt 先例同口径复核），本节按分册 9 节结构逐节落实，并以第 8 节 12 项全局一致性自查替代可机械执行部分；全量 141 项留待 Gate 1 validation 执行。
