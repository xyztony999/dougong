# account-auth 前端生成任务清单（F01~F07）

> PageSpec：02-detailed/interaction/{admin,readonly}/account-auth/（admin v1.1 + readonly v1.0）
> 类型对齐：02-detailed/api/api-account-auth.md v1.0/v1.1（唯一事实源）
> 本模块为 frontend/ 首模块，含全栈脚手架（Vite+Vue3+TS+Naive UI+Pinia+vue-router）。

## 状态

| 任务 | 状态 | 产物 | 备注 |
|------|------|------|------|
| F01 模块分析 | [x] | 本文件 | 页面类型全 config：3 列表页（user-list/session-list 无查询区——API 无筛选参数；audit-log-list 条件分页）+ 3 表单页（init-wizard/login 二步式/profile 双 Card）+ 框架布局 + 403 边界页；readonly 按 ADR-018 负空间（5 Action 前端隐藏） |
| F00 脚手架 | [x] | package.json / vite.config.ts / tsconfig.json / index.html / .gitignore | npm install 真实执行；dev proxy /auth → DG_BACKEND_ADDR 默认 http://127.0.0.1:8080（api-panel-core.md listen_port=安装时随机高位端口，无固定开发端口 → 用默认值并在交付 summary 注明） |
| F02 类型定义 | [x] | src/types/api.ts / enums.ts / auth.ts + src/utils/{totp,format}.ts | 包络 `{"code","message","data"}`（api 3.1，与通用模板 `msg` 不同——以 API 文档为准）；分页 `{items,total}` + query `page/pageSize`（api 3.1 公共约定，覆盖 frontend-rules §3 records/pageNum 模板——设计文档优先）；8 字典 Key 颜色方案照 PageSpec 第 3 节 |
| F03 api 层 | [x] | src/api/http.ts / src/api/auth.ts | 18 Action 一一对应（S7）；POST 统一 X-CSRF-Token（csrfToken 内存态，R-02 不落 localStorage）；拦截器分流 AUTH_001~014 |
| F04 页面组件 | [x] | src/views/account-auth/**（7 页面 + 5 局部组件）+ src/layouts/MainLayout.vue + src/views/common/403.vue | 见下方页面清单 |
| F05 状态 | [x] | src/stores/auth.ts | authStore（currentUser/csrfToken/initialized）跨页面共享；页面内状态不进 Pinia |
| F06 路由 | [x] | src/router/index.ts | 7 路由懒加载 + meta（guestOnly/requiresAuth/roles/title）；守卫顺序 initCheck（缓存）→ sessionCurrent 探测 → roles 校验 → 403 |
| F07 合规检查 | [x] | — | frontend-rules §6 F-01~F-06 扫描全过；`npm run build`（vue-tsc --noEmit && vite build，cwd=frontend/）全绿 |

## 页面清单（7 页面 = admin PageSpec v1.1 第 9 节口径）

| 页面 | 路由 | 视图文件 | readonly 差异（负空间落地） |
|------|------|---------|---------------------------|
| init-wizard | /init-wizard | views/account-auth/init-wizard/index.vue | 公共页复用（readonly 文档第 5 节） |
| login | /login | views/account-auth/login/index.vue | 公共页复用；AUTH_007 二步式/AUTH_004 倒计时 |
| 框架布局 | / | layouts/MainLayout.vue | 菜单同四项，无管理动作入口 |
| user-list | /users | views/account-auth/user-list/index.vue | 无「新增用户」工具栏按钮；无操作列（重置密码/删除整体隐藏） |
| session-list | /sessions | views/account-auth/session-list/index.vue | 无「强制下线」操作列；isCurrent 标记列保留 |
| audit-log-list | /audit-logs | views/account-auth/audit-log-list/index.vue | 无「导出 CSV」按钮；详情 JSON 弹窗保留 |
| profile-self | /profile | views/account-auth/profile/index.vue | 公共页复用（自助凭证操作不受变更禁令限制） |

局部组件：user-list/components/{UserCreateModal,ResetPasswordModal}.vue、audit-log-list/components/AuditDetailModal.vue、profile/components/{TotpBindModal,TotpDisableModal}.vue。

## 已声明的实现偏差（诚实对应 API，不虚构接口）

1. **用户名失焦查重（AUTH_011）**：PageSpec 4.1 新增用户「失焦查重提示 AUTH_011」——API 无 check 端点（S7 18 接口无查重），AUTH_011 仅在 userCreate 提交时返回 → 落地为提交失败后回填字段级错误（不虚构查询接口，同 PageSpec 4.1「诚实对应接口」先例）。
2. **会话来源列 UA 悬浮（4.2）**：PageSpec 要求 sourceIp 列 UA NTooltip，但 api 4.2.4 list items 无 userAgent 字段（DDL 有列、接口不下发）→ 仅展示 sourceIp，不加悬浮。
3. **profile 绑定时间（5.3 已启用态）**：任何接口均不下发 confirmedAt → 已启用态仅展示 NTag，不展示绑定时间；totpEnabled 状态经 userList（admin+readonly 可见）匹配自身 id 获取。
4. **分页字段名**：frontend-rules §3 模板为 records/pageNum，api-account-auth.md 3.1 公共约定为 items/total + page/pageSize → 以 API 文档为准（types 单一来源，无两套字段名）。
5. **审计动作码中文释义**：api 3.4 常量表未给中文文案，enums.ts 的 AUDIT_ACTION_TEXT 为前端展示层生成（动作码值本身逐字对齐 15 常量）。

## 门禁记录

- `npm run build`（= vue-tsc --noEmit && vite build）：✅ 全绿（本文件 F07 行更新时点）
- frontend-rules §6 扫描：F-01~F-06 全过（命令与输出见交付 summary）

## Gate 2 FAIL 第 1 轮修复记录（2026-11-14，报告 05-reports/gate2-account-auth-2026-11-14.md）

| 项 | 修复 | 决策登记 |
|----|------|---------|
| E1（AL-005 错误码通道断裂） | 新增 `src/utils/errcode.ts` `normalizeCode()`：包络边界把后端数值 401NN（errcode/auth.go「AUTH_0NN→401NN」）归一为契约符号 `AUTH_0NN`，符号串原样透传；接入 `api/http.ts:147`（request）与 `api/auth.ts:154`（exportAuditLog）；全站 16 处符号比对**不动**（契约 §3.4 口径） | 前端采「双通道归一」而非改 16 处比对：① 契约文档为唯一事实源、禁改（§3.4 code 列=符号）；② 后端若按修复方向回归符号通道，前端零改动；③ 跨模块码 40400→NOT_FOUND、50000→INTERNAL 一并归一。⚠️ 后端侧「A-2 裁定」仍未在 workspace 登记——需后端 owner 处置（改发符号或正式修订契约），前端已两通道兼容 |
| E2（AL-003 审计清单键 PascalCase） | 新增 `src/utils/audit.ts`：`normalizeAuditItem/normalizeAuditListResult` 在 api 层把后端 AuditRecord PascalCase 键（含 ID/ObjectID 连续大写特例）归一为契约 camelCase；`api/auth.ts auditList` 接入；类型/页面字段保持契约单一来源 | 根因修复=后端补 camelCase json tag（超出前端范围）；前端容错器 camelCase 分支直通，后端修复后无感。首版 naive Pascal 转换漏 ID/ObjectID 两特例，被新增单测捕获后修正（PASCAL_ALIASES） |
| W2（profile 模板笔误） | `profile/index.vue:134` `bordered: false` → `:bordered="false"`（:131 同类已在首轮修复）；复核剩余 `bordered: false` 5 处均为 h() render 属性对象合法用法 | — |
| W1（前端 0 测试） | 补 vitest ^2.1.8 + `npm test` 脚本 + 4 个纯函数测试：`__tests__/{errcode,audit,format,totp}.test.ts` 共 19 用例全绿（`npx vitest run`：4 files / 19 tests passed） | E1/E2 适配器本身纳入断言（Gate 2 报告要求「按前端契约口径补断言」的前端份额）；后端侧断言盲区修复归 B7 |

自验（2026-11-14，cwd=frontend/）：`npx vitest run` 19/19 PASS；`npm run build` exit 0；grep 复核——`<n-tag[^>]*bordered:` 0 命中、`String(env.code)` 0 残留、`normalizeCode` 双边界接线、符号比对计数 19 处（含拦截器码集，全部 §3.4 符号口径）。

---

# monitoring 前端生成任务清单（F01~F07）

> PageSpec：02-detailed/interaction/{admin,readonly}/monitoring/monitoring.md（双视角 v1.0；readonly 与 admin 完全一致——零写接口使 ADR-018 差异面为空，无负空间、无禁用分支）
> 类型对齐：02-detailed/api/api-monitoring.md v1.0（唯一事实源）
> 复用 account-auth/panel-core 既有结构追加（api 层/包络/守卫/菜单下发不另起炉灶）。

## 状态

| 任务 | 状态 | 产物 | 备注 |
|------|------|------|------|
| F01 模块分析 | [x] | 本文件 | 页面类型 hybrid×2：监控概览（Card A 系统信息 + Card B 实时快照 + Card C TOP 进程，路由 /monitoring）+ 历史趋势（控件条 + echarts 折线，路由 /monitoring/history）；无列表页/无表单页/无 operList（PageSpec 第 4/5 节诚实声明）；4 Action 全 GET 只读 |
| F02 类型定义 | [x] | src/types/monitoring.ts + enums.ts 追加 metric_* 三字典 + utils/format.ts 追加 formatBytes/formatBps/formatUptime | D-08：采集失败字段 null（组与叶子均可空）；processes 单项可空性初版误判为非空 → Gate 2 E1 修正为 cpuPercent/memBytes number|null（见下方修复记录）；D-03 series 统一形状；D-02 单位格式化前端落法（bps/bytes 1024 进制 1 位小数、uptime ≥2 级粒度） |
| F03 api 层 | [x] | src/api/monitoring.ts | 4 Action 一一对应（S7）：metricsSnapshot/metricsHistory/processesTop/systemInfo；scope 仅 dimension=cpu 时携带（D-04） |
| F04 页面组件 | [x] | src/views/monitoring/{index.vue, history/index.vue} + components/{SystemInfoCard,SnapshotCard,ProcessesCard,TrendChart}.vue | 概览横幅（degraded NAlert/滞后连接中断 orange NTag/MON_004 初始化中空态）；快照 3s 整卡直更（echarts 不用于快照区）；TOP 表 ≤10 行无操作列；趋势 connectNulls=false 断线表缺失、partial orange NTag、startTime 起点灰字、空态 NEmpty；echarts 6 为 PageSpec §1 给定选型（idea.md §5.3） |
| F05 状态 | [x] | src/stores/monitoring.ts | PageSpec §1 声明 monitorStore（latestSnapshot/trendData/trendParams/systemInfo/pollTimers）；轮询纪律 D-01：快照 3s（滞后阈值 6s=2×采样）/系统信息 60s/趋势按需无自动轮询；document.hidden 暂停 + 恢复可见立即拉取；页面 onUnmounted 调 stopRealtimePolling 清全部定时器与监听 |
| F06 路由 | [x] | src/router/index.ts 追加 2 路由 + vite.config.ts /monitoring proxy + MainLayout MENU_ICONS 追加 pulse | 懒加载 + meta.title；requiresAuth 继承布局路由（两角色无 roles 限制）；菜单项由后端 ModuleMenuRegistry 下发（框架边界归 panel-core，本模块不硬编码菜单） |
| F07 合规检查 | [x] | — | frontend-rules §6 F-01~F-05 扫描 0 命中 + 定时器/图表实例配对清理核验；`npm run build`（vue-tsc --noEmit && vite build，cwd=frontend/）exit 0（7.94s）；`npx vitest run` 33/33（新增 format 用例 14 条） |

## 已声明的实现决策（依据 PageSpec/API，不虚构）

1. **processesTop 不入默认轮询**（PageSpec 第 2 节）：概览页 TOP 表随快照同源同响应刷新；api 层保留 `processesTop()` 独立契约函数供仪表盘/后续页面单取，概览页不调用。
2. **echarts 依赖新增**：package.json 无 echarts → `npm install echarts`（^6.1.0）真实执行；为 PageSpec §1 组件体系给定选型（idea.md §5.3），非虚构。
3. **MON_001~003 兜底提示落 store 错误态 + 页面 watch 提示**：store 无 Message 上下文（useMessage 须在 Provider 内 setup 调用），故 trendError 由 history 页 watch 后 useMessage 提示，避免双提示（http 拦截器不含 MON 码集）。
4. **图表主题联动**：TrendChart 取 `useThemeVars()`（n-config-provider 主题对象，明暗随 Naive 全局主题）；echarts.init 第二参注入 `{ backgroundColor: 'transparent', textStyle: { color: textColorBase } }` 主题对象——echarts v6 ESM 入口不内置 'dark' 主题名，故弃初版 matchMedia+'dark' 方案改主题对象注入，背景透明叠加卡片；resize 监听与实例 dispose 在 onUnmounted 配对清理。（初版 matchMedia 方案已被替换，本行为终态登记 2026-11-14 重派发复验轮）
5. **菜单图标映射**：MainLayout MENU_ICONS 追加 `pulse`（PulseOutline）；monitoring 菜单项由后端 ModuleMenuRegistry 登记（后端模块 todo），未登记前图标映射就绪、未知名兜底 AppsOutline 逻辑不变。

## 门禁记录

- `npm run build`（= vue-tsc --noEmit && vite build，cwd=frontend/）：✅ exit 0（2026-11-14，7.94s；TrendChart 改 echarts/core 按需注册后 history 懒加载块 1130.70 kB→517.99 kB、gzip 380.98→177.51）
- `npx vitest run`：✅ 4 files / 33 tests passed（含新增 formatBytes/formatBps/formatPercent/formatLoad/formatUptime 14 用例；初跑 1 失败——formatUptime 不足 1 分钟退化秒级后同步修正断言）
- frontend-rules §6 扫描：F-01~F-05 全部 0 命中；monitoring views 硬编码 hex 0 命中、定时器 0 命中（集中 stores/monitoring.ts，clearInterval/removeEventListener/dispose 配对，页面 onUnmounted 调 stopRealtimePolling）

## Gate 2 FAIL 第 1 轮修复记录（2026-11-14，报告 05-reports/gate2-monitoring-2026-11-14.md）

| 项 | 修复 | 决策登记 |
|----|------|---------|
| E1（AL-003 processes 可空性断裂，FAIL 主因） | types/monitoring.ts `ProcessItem.cpuPercent/memBytes` → `number \| null`（对齐 backend internal/service/monitoring/types.go:17-19 `*float64/*uint64`——null=读取失败或无基线，sampler 新进程首轮必 null 常态可达；文件头与接口注释同步纠正）；ProcessesCard 两列 render 改 `formatPercent(row.cpuPercent)`/`formatBytes(row.memBytes)`（null → '—'，守卫内建于格式化器且有既有单测覆盖）——null 触发 TypeError 致 TOP 表整卡渲染崩溃的路径结构性消除 | 采用报告修复建议 a（前端补可空 + render 守卫）；formatBytes 原本 null 安全，崩溃点仅 cpuPercent.toFixed(1)，统一改走 formatPercent 保持口径一致（1 位小数 + %） |
| E2（磁盘进度条恒满格） | SnapshotCard 磁盘行重构为视图模型 `DiskRowView{mount, percent, usageText}`：percent=`usagePercent(used, total)`（utils/format.ts 新增纯函数：used/total×100 1 位小数；任一缺失或 total≤0 → null 不渲染进度条），usageText 保持原「used / total」字节数文本不变 | 修复前 diskPercent 直接返回 usedBytes 字节数作 n-progress percentage（期望 0-100）→ 恒满格；新增 4 组 usagePercent 单测拦截该类回归（含「GiB 级 used 不再恒 100」回归用例） |
| 随附（非报告项） | MainLayout MENU_ICONS 补 `trend: TrendingUpOutline`——backend main.go:137 monitoring-history 菜单登记 `Icon: "trend"`，原前端无此映射走 AppsOutline 兜底；`pulse` 映射与 backend main.go:134 登记名一致无需改 | 图标名为 ModuleMenuRegistry 登记字符串、前端维护映射（MainLayout 既有约定）；未登记名兜底逻辑不变 |

自验（2026-11-14，cwd=frontend/）：`npm run build`（vue-tsc --noEmit && vite build）exit 0（7.78s）；`npx vitest run` 4 files / 37 tests 全过（+usagePercent 4 用例）；grep——`cpuPercent.toFixed` 无守卫残留 0、SnapshotCard `diskPercent` 残留 0、F-01~F-05 全 0 命中、pulse+trend 图标映射 2/2。

W1/W2 维持报告原判（WARN 非本轮 FAIL 修复项）：views/monitoring 组件与 monitorStore 仍无组件级挂载测试（@vue/test-utils 未引入——E1 类回归改以类型收窄 + 纯函数单测拦截，组件挂载测试基础设施待后续裁定）；src/components 目录缺席（account-auth Gate 2 W4 同判）。

## 重派发复验轮（2026-11-14）

对 PageSpec（admin/readonly v1.0）+ api-monitoring.md v1.0 逐文件重核 F01~F06 全符：4 Action 一一对应（scope 仅 cpu 携带 D-04）、2 页面/4 组件（横幅三态/控件条条件显隐/partial+startTime 清零边界/connectNulls=false/空态 NEmpty）、monitorStore 轮询纪律（3s/60s/按需 + hidden 暂停 + onUnmounted 清理）、2 路由懒加载 + meta.title、enums 三字典取值与 04-business-enum 定稿逐字一致、readonly 全等无负空间（无 role 分支代码）。门禁实跑：F-01~F-05 grep 全 0 命中、monitoring views 定时器 0 命中（store 内 clearInterval/removeEventListener + TrendChart dispose 配对）、`npm run build`（vue-tsc --noEmit && vite build）exit 0（26.75s）、`npx vitest run` 6 files/51 tests 全过无回归。随附修正：本节决策 #4 表述更新为终态（useThemeVars 主题对象注入，弃 matchMedia+'dark' 初版方案——与 TrendChart.vue 现实现一致）。


# service-mgmt 前端生成任务清单（F01~F07）

> PageSpec：02-detailed/interaction/{admin,readonly}/service-mgmt/service-mgmt.md（admin v1.0 + readonly 子集 v1.0；readonly 同路由同组件按角色渲染子集——操作列整列不渲染、自启恒 NTag，ADR-018）
> 类型对齐：02-detailed/api/api-service-mgmt.md v1.0（唯一事实源）
> 复用既有结构追加（http 拦截器/enums 字典源/守卫/菜单下发不另起炉灶）。

## 状态

| 任务 | 状态 | 产物 | 备注 |
|------|------|------|------|
| F01 模块分析 | [x] | 本文件 | 页面类型 config×1（/services 列表页 + 属性 Drawer 480px）；7 Action（2 GET admin+readonly + 5 POST admin+CSRF）；无表单页（PageSpec 第 5 节 N/A：零持久化无新增/编辑语义）；状态过滤/名称搜索为查询条件非表单 |
| F02 类型定义 | [x] | src/types/service.ts + enums.ts 追加 service_active_state/service_enabled_state/unit_type | enabledState/subState/loadState 透传 string（D-04/⚠️A14，非两态灰字原值降级走 dictItem 兜底）；activeState 为三态枚举；SVC_ERROR_TEXT 8 码集中映射（001/002/004/005=「参数非法」兜底，PageSpec 第 1 节）；isServiceActiveState 三态收窄（SVC_007 data.activeState 透传入行前判定） |
| F03 api 层 | [x] | src/api/service.ts | 7 Action 一一对应（S7）；函数名=Action 名；state 多值逗号拼接 query（api list 契约）；5 POST 经 http.ts 统一注入 X-CSRF-Token |
| F04 页面组件 | [x] | src/views/service-mgmt/index.vue + components/UnitDetailDrawer.vue | 工具条（NSelect multiple + NInput 防抖 300ms + 刷新 loading 防抖）；表格 7 列（名称 link fixed:left/类型 dict/运行状态 colorTag/状态细分透传/自启 NSwitch·NTag 复合/操作列 admin only fixed:right）；启停按钮显隐=canOperate 权威布尔 + activeState 呈现层优化；停止/重启 NPopconfirm（PageSpec 原文文案）；SVC_007 结果 Modal（reason+实际终态 Tag）；SVC_006 warning；SVC_003 warning+自动重拉；SVC_008 页面横幅+重试、表格不渲染（I-01）；空态 NEmpty「未匹配到服务」；共 N 个 unit + generatedAt 本地时区灰字；Drawer 480px 9 字段单区块 NDescriptions、头部刷新图标、SVC_003 NEmpty/SVC_008 NAlert、description/loadState null→「—」、无操作按钮 |
| F05 状态 | [x] | src/stores/service.ts | PageSpec §1 serviceStore：units/total/generatedAt/listParams/unitDetail/inFlightSet（含 Drawer 态）；无轮询定时器（D-03 手动+操作驱动，本 store 不持定时器）；动作成功 applySnapshot 行级直更 + generatedAt=响应时刻（不整表重拉）；NSwitch 回弹到响应值（不信任乐观更新）；页面卸载 resetView 清 Drawer/inFlight；迟到 detail 响应不写入已关闭 Drawer |
| F06 路由 | [x] | src/router/index.ts 追加 /services + vite.config.ts /services proxy + MainLayout MENU_ICONS 追加 server | requiresAuth 继承布局路由（admin+readonly 同路由，PageSpec 第 7 节）；菜单项由后端 ModuleMenuRegistry 下发（后端 todo，暂按 server 预置 ServerOutline 映射，未登记名兜底 AppsOutline 零影响） |
| F07 合规检查 | [x] | — | frontend-rules §6 F-01~F-05 扫描 0 命中（src/components 目录不存在警告=account-auth Gate 2 W4 既有状态，src/views 0 命中）；唯一 setTimeout（搜索防抖）onUnmounted 配对清理；`npm run build`（vue-tsc --noEmit && vite build，cwd=frontend/）exit 0（7.98s，service-mgmt chunk 29.50 kB）；`npx vitest run` 4 files/37 tests 全过无回归 |

## 已声明的实现决策（依据 PageSpec/API，不虚构）

1. **函数名=Action 名**（unitList/unitDetail/unitStart/…）：与 PageSpec 第 2 节 S7 映射逐字对应（monitoring 先例），不用 frontend-rules §3 通用 list/get 动词模板——Action 名即契约标识。
2. **SVC_008 动作路径归一页面横幅**：PageSpec 第 1 节将 SVC_008 定义为「页面级错误横幅+重试（不显示过期数据 I-01）」且 api 3.4 声明「列表/属性/动作共前置依赖」→ 动作返回 SVC_008 时同样 markListUnavailable（横幅+清空表格），不另行 toast。
3. **AUTH_008 全局文案**：readonly 越界（正常 UI 不可达）走全局拦截器既有文案（types/auth.ts AUTH_008「权限不足：只读角色禁止变更操作」）；PageSpec 模块级文案「…禁止服务操作」为同一码族的全局分流口径，不为本模块单开分支。
4. **readonly 子集渲染策略**：同路由同组件（readonly PageSpec 第 7 节），操作列整列不渲染（`if (authStore.isAdmin)` 才 push，非禁用置灰）、自启列恒 NTag（`isAdmin && canToggleAutostart` 才渲染 NSwitch）、scrollX 随角色收缩（970/750）。
5. **store 无 Message 上下文**（monitoring 先例）：SVC_002 兜底文案经 listBizError 状态由页面 watch 后 useMessage 提示；SVC_003/006/007/008 在页面 handleActionError 分流（store 不 toast，避免双提示——http 拦截器不含 SVC 码集）。
6. **菜单图标预置映射**：后端 service-mgmt 模块 todo，ModuleMenuRegistry 登记名未定 → MainLayout 预置 `server: ServerOutline`（backend main.go 落定登记名后对齐或自然兜底 AppsOutline，零影响）。
7. **loadList 重入守卫**：listLoading 期间的新触发被跳过（刷新按钮 loading 防抖同源；monitoring trendLoading 同先例）；参数变更引发的漏拉由下一次触发补偿，不影响最终一致性。

## 门禁记录

- `npm run build`（= vue-tsc --noEmit && vite build，cwd=frontend/）：✅ exit 0（2026-11-14，7.98s；500kB chunk 警告为既有 monitoring history echarts 块，非本模块产物）
- `npx vitest run`：✅ 4 files / 37 tests passed（无回归；本模块无新增纯函数——dictItem 兜底逻辑为既有已测函数复用）
- frontend-rules §6 扫描：F-01~F-05 全 0 命中；service-mgmt views 定时器 1 处（搜索防抖 setTimeout）onUnmounted 配对清理核验（index.vue:316-318）

## 门控第 1 轮 FAIL 修复记录（2026-11-14，编译门控 npm run build exit=2）

| 项 | 修复 | 根因登记 |
|----|------|---------|
| TS2339（UnitDetailDrawer.vue:44 `header-extra`） | n-drawer-content 改用 `#header` 插槽自绘「标题 + 刷新按钮」行（closable 关闭图标由组件独立渲染；naive-ui DrawerContent 插槽面仅 header/default/footer，DrawerContent.mjs:65-81） | 首轮 build 假绿根因：`vue-tsc --noEmit && vite build` 中 vue-tsc 先行、components.d.ts（unplugin-vue-components 生成）尚未登记 NDrawerContent 真实插槽类型 → 按宽松类型通过；门控复跑时 dts 已刷新暴露 TS2339。防复发：修复验证跑两轮（build exit 0 后再 `npm run typecheck` exit 0，覆盖 dts 再生成后的第二遍） |

自验（2026-11-14，cwd=frontend/）：`npm run build` exit 0（8.10s）；构建后二次 `npm run typecheck` exit 0；`npx vitest run` 37/37 无回归。


# file-manager 前端生成任务清单（F01~F07）

> PageSpec：02-detailed/interaction/{admin,readonly}/file-manager/file-manager.md（admin v1.1 + readonly v1.1；readonly 为 admin 子集——同路由同组件按角色渲染，20 个 ✗ 接口负空间逐项对应，ADR-018）
> 类型对齐：02-detailed/api/api-file-manager.md v1.0（唯一事实源，22 接口）
> 复用既有结构追加（http 拦截器/enums 字典源/守卫/菜单下发不另起炉灶）。

## 状态

| 任务 | 状态 | 产物 | 备注 |
|------|------|------|------|
| F01 模块分析 | [x] | 本文件 | 页面类型 config×2（files 列表 /files + trash 回收站 /files/trash）+ code×1（edit 在线编辑 /files/edit，Monaco）；22 Action（6 GET + 16 POST admin+CSRF）；readonly 子集 2 读 Action + 20 负空间 |
| F02 类型定义 | [x] | src/types/file.ts + enums.ts 追加 file_type/archive_format | 投影条目 8 字段（4.2.1）/trash 六键（4.1.1，id 不渲染）/22 接口 data 形状逐一对齐；局部常量（sort/order/onConflict/回收站两值/CreateEntryType/5MiB 阈值）按 PageSpec 第 3 节归 types/file.ts 非池枚举；FILE_ERROR_TEXT 15 码集中映射（第 1 节）+ isConflictList/scopeReason unknown 收窄器 |
| F03 api 层 | [x] | src/api/file.ts + http.ts 追加 postForm/downloadFile | 22 Action 一一对应（S7，函数名=Action 名）；upload/chunk 走 multipart（postForm，不设 Content-Type 由浏览器带 boundary）、download 走流式（downloadFile：JSON 错误体=开流前校验失败走包络通道 D-16，成功 Blob+Content-Disposition 文件名）；http.ts 重构出 unwrapEnvelope 共用解包 |
| F04 页面组件 | [x] | src/views/file-manager/**（3 页面 + 9 局部组件 + icons.ts/errors.ts） | files：工具条（面包屑/上级/排序/视图切换/admin 上传·新建·压缩·路径边界/刷新）+ 表格 8 列（选择/名称图标链接/类型/大小/权限/属主/mtime/操作 320 fixed:right）+ 图标视图 NGrid 卡片 + 9 Dialog（新建/重命名/移动·解压内嵌 DirBrowser/权限九宫格/属主/压缩/路径边界/冲突决策共用）+ 上传管线双分支（直传≤5MiB/分块三段式 NProgress）+ 拖拽放置（admin 才注册监听）+ 批量删除逐行循环单一确认；trash：原路径只读/两值类型/快照大小/删除人/删除时间/还原（冲突无 skip）+ 清空 NModal 强警告空站禁用 + 无 generatedAt 不虚构；edit：Monaco 动态 import + 扩展名语言局部映射 + Ctrl/Cmd+S 双通道 + dirty 路由/beforeunload 双拦截 + savedMeta 直更 + readonly 组件内兜底跳回 |
| F05 状态 | [x] | src/stores/{file,trash,editor}.ts | PageSpec §1 三 store：fileStore（currentPath/resolvedPath/entries/generatedAt/viewMode/sortParams/inFlightSet/selection/目录选择器态）+ trashStore（items/total/inFlightSet）+ editorStore（path/content/dirty/savedMeta/loadError）；刷新纪律 D-03 无轮询（内容类 loadList 重拉、chmod/chown applyMetaPatch 行直更）；FILE_002 scopeError 横幅 + FILE_012 回受管根防死循环 |
| F06 路由 | [x] | src/router/index.ts 追加 3 路由 + vite.config.ts /files proxy + MainLayout 菜单图标 | /files、/files/trash、/files/edit（requiresAuth 继承布局路由，两角色同路由组件内子集渲染）；edit query path 缺失组件内返回列表 + readonly 兜底跳回（readonly PageSpec 第 7 节）；菜单 folder/trash 预置图标映射（后端 ModuleMenuRegistry todo，未登记名兜底 AppsOutline） |
| F07 合规检查 | [x] | — | frontend-rules §6 F-01~F-05 扫描全 0 命中 + hex 0 + 本模块定时器 0（无轮询 D-03；edit 页监听器 onUnmounted 配对）；`npm run build`（vue-tsc --noEmit && vite build，cwd=frontend/）exit 0（29.15s）+ 构建后二次 `npm run typecheck` exit 0 + `npx vitest run` 37/37 无回归 |

## 已声明的实现决策（依据 PageSpec/API，不虚构）

1. **初始浏览省略 path 参数（''=受管根）——A-F20 已裁定**：readonly 无 path-scope/get 权（api 3.3 ✗）、无从预知受管根，缺省浏览只能由服务端解析。Gate 2 第 1 轮 E1 后端落定 A-F20（projector.go 缺省 path=guard RootPath，宽容缺省不升版 api 契约，后端已补 TestListDirDefaultPathIsRoot）；前端侧补 store 回归测试（root-default 请求 + data.path 首屏渲染 + rootPath 捕获，src/stores/__tests__/file.test.ts）。**同族修复**：后端仅 entry/list 有缺省语义——写操作（upload/create/move/unarchive 的 dir/targetDir）空串即 FILE_001，原实现在受管根下恰发空串；落法=root-default 响应 data.path 捕获为 rootPath 并将 currentPath/pickerPath 规范化为具体绝对路径，写寻址一律走 store.writeDir()（rootPath ?? '/' 兜底，越界由服务端 FILE_002 拦截）；pathScope 更改后 invalidateRoot 由下次 root-default 重解。
2. **monaco-editor ^0.57.0 真实安装**（npm install 实跑；PageSpec §1 给定选型）。0.57 根入口（esm/vs/index.js）为 worker-free 形态——语言定义为懒加载 monarch 词法（languages/definitions/*），编辑器全程主线程运行，无需 MonacoEnvironment/worker 通道（首轮按旧版 ?worker 五件套装配被 Rollup exports 映射拒绝，改根入口直入后消除）；Monaco 经动态 import 懒加载（独立 chunk ~4.1MB gzip 1.07MB，仅 /files/edit 触达，readonly 永不加载）。
3. **图标视图行操作承载**：PageSpec 4.2「操作经选中后工具栏，与列表视图同一 Action 集」——落法=卡片「操作」NDropdown 菜单（同一 rowActions 单一来源）+ 复选框承载压缩/批量删除；图标视图删除确认走 useDialog().warning（NDropdown 项无法内嵌 NPopconfirm），确认文案与列表视图逐字一致（R-17 轻确认口径不变）。
4. **面包屑根段标签「受管根」**：root-default 响应后 rootPath 已知（首次加载捕获），根段渲染为「受管根」标签（点击=root-default 重回根）而非裸路径。
5. **FILE_005 conflicts 载荷形状**：api 逐场景声明 upload=[{name}]（4.2.4）、restore=[{path}]（4.1.4），move/unarchive 仅声明 data.conflicts[] 未逐字段——统一 ConflictEntry `{name?, path?}` + conflictLabel 展示器（path ?? name），不虚构必填键。
6. **entryDownload 在 api 层触发浏览器保存**（blob→a[download]→revoke）：与 22 Action 函数形统一为页面单一调用点（PageSpec 4.3「window 触发流下载」）。
7. **AUTH_008 全局文案**：readonly 越界（正常 UI 不可达——本模块 20 负空间入口全不渲染）走既有全局拦截器文案，不为本模块单开分支（service-mgmt 决策 3 同口径）。
8. **trash 还原后原目录列表刷新**：files 页每次挂载即重拉（D-03 进入页面行），restore 成功后无需跨页通知——/files 重新进入时自然取新（无 keep-alive）。
9. **trash/list data.warning 消费（W1 修复）**：后端 A-F11 增补孤儿对账标注（完整中文文案，omitempty）——types/file.ts TrashListData 声明 `warning?: string`，trashStore.listWarning 捕获，trash 页 NAlert warning 直接呈现服务端文案（不二次改写）。

## 门禁记录

- `npm run build`（= vue-tsc --noEmit && vite build，cwd=frontend/）：✅ exit 0（2026-11-14，29.15s；monaco 懒加载 chunk 4119.32 kB/gzip 1066.30 kB 仅 edit 路由触达；500kB 警告另含既有 monitoring history echarts 块）
- 构建后二次 `npm run typecheck`：✅ exit 0（components.d.ts 再生成后复验——service-mgmt TS2339 教训防复发）
- `npx vitest run`：✅ 4 files / 37 tests passed（无回归；本模块无新增纯函数——冲突收窄/文案映射为内联逻辑，G-04 类 grep 全过）
- frontend-rules §6 扫描：F-01~F-05 全 0 命中；file-manager views hex 0 命中（图标色用 CSS 命名色 cornflowerblue/mediumpurple/crimson）；定时器 0 命中（无轮询；edit 页 keydown/beforeunload/media 监听 onUnmounted 配对清理）
- 修复过程两轮编译门控：①首轮 vue-tsc 10 错（.vue 类型导入缺扩展名 ×4、BracesOutline 不存在、FileEntryType 导错源、级联 implicit any ×4）→ 全修复；②vite build monaco ?worker 子路径被包 exports 映射拒绝 → 改根入口动态 import（决策 2）

## Gate 2 FAIL 第 1 轮修复记录（2026-11-14，报告 05-reports/gate2-file-manager-2026-11-14.md）

| 项 | 修复 | 决策登记 |
|----|------|---------|
| E1（阻断：初始浏览空 path → 后端 FILE_001） | 修复主体在后端 A-F20（projector.go 缺省 path=受管根，宽容缺省不升版 api 契约，backend/tasks.md:337/391 + TestListDirDefaultPathIsRoot 回归）。前端侧按报告「补双侧用例」落三件：① 回归测试 src/stores/__tests__/file.test.ts——root-default 请求形状（path: undefined + name/asc）、data.path 首屏渲染（resolvedPath）、currentPath/rootPath 规范化断言；② 同族修复（报告未及、复测发现）：后端仅 entry/list 有缺省语义，写操作 dir/targetDir 空串即 FILE_001——原实现受管根下 upload/create/move/unarchive 恰发空串；落法=root-default 响应 data.path 捕获 rootPath + currentPath/pickerPath 规范化 + store.writeDir() 统一写寻址 + Move/Unarchive 确认钮在 picker 规范化前禁用 + pathScope 更改 invalidateRoot；③ 决策 1 与 api/file.ts EntryListQuery 注释改引 A-F20 裁定 | 后端选「缺省=受管根」而非 api 升版（A-F20 登记原文：readonly 无从预知受管根，缺省浏览只能由服务端解析）；前端写寻址不虚构「前端可知根」——rootPath 恒来自服务端响应 |
| W1（trash warning 未消费） | types/file.ts TrashListData + `warning?: string`（A-F11 omitempty）；trashStore.listWarning 捕获；trash 页工具栏下 NAlert warning 呈现服务端完整文案；trash.test.ts 2 用例（非空捕获/omitempty 归位 null） | 服务端文案为完整句子（「N 条记录因回收站内容缺失已剔除（可能被面板外改动）」）——前端直呈不改写 |
| W2（views+stores 0 vitest，部分处置） | 新增 2 个 store 测试文件 11 用例：fileStore 7（E1 回归 6 + picker 规范化 1）+ trashStore 4（W1 2 + 在途互斥/错误静默 2）；view 组件挂载测试基础设施（@vue/test-utils）仍缺席——沿用 monitoring/service-mgmt W1 同判（WARN 维持，组件级回归以类型收窄+store 单测拦截） | store 层为 E1/W1 修复的落点，测试随修复走；组件挂载测试待后续裁定 |
| 随附（测试捕获的既有 bug） | fileStore FILE_012 回根递归重拉入口复位 listBizError 致 warning 被吞（页面 watch 见 null）——递归完成后回填消息（stores/file.ts loadList FILE_012 分支） | 由 file.test.ts「FILE_012 → 回受管根重拉」用例捕获后修复（初跑 1 失败→修复→48/48） |
| W3（src/components 缺席） | 维持（account-auth Gate 2 W4 / monitoring W2 / service-mgmt INFO 先例——本模块无跨页面复用组件诉求，局部组件归 views/file-manager/components/） | — |

自验（2026-11-14，cwd=frontend/）：`npm run build`（vue-tsc --noEmit && vite build）exit 0（24.33s）；构建后二次 `npm run typecheck` exit 0；`npx vitest run` 6 files / 48 tests 全过（37 既有 + 11 新增）；frontend-rules §6 F-01~F-05 + hex 复扫全 0 命中。


# site-mgmt 前端生成任务清单（F01~F07）

> PageSpec：02-detailed/interaction/{admin,readonly}/site-mgmt/site-mgmt.md（admin v1.0 + readonly v1.0；readonly 同路由同组件按角色渲染子集——14 个写/编辑前置 Action 负空间，ADR-018/D0 口径）
> 类型对齐：02-detailed/api/api-site-mgmt.md v1.0（唯一事实源，18 接口）
> 复用既有结构追加（http 拦截器/enums 字典源/守卫/菜单下发不另起炉灶）。

## 状态

| 任务 | 状态 | 产物 | 备注 |
|------|------|------|------|
| F01 模块分析 | [x] | 本文件 | 页面类型 hybrid×1（/sites site-list 列表 + 详情 Drawer 6 Card 含日志 Tab）+ config×1（/certificates cert-list）+ code×1（/sites/:siteId/source Monaco 源码编辑）；18 Action（4 GET 两角色 + 12 POST + 2 GET admin 专属：siteSource/phpPoolsList）；readonly 负空间 14（工具栏/操作列整列不渲染、Drawer 全只读 + 日志刷新、source 路由 roles 守卫 → 403） |
| F02 类型定义 | [x] | src/types/site.ts + enums.ts 追加 6 字典 + 13 审计动作码 | 投影条目 12 字段/list envelopes（nginxAvailable+duplicates[]+generatedAt）/detail（locations+rawBlocks）/create·update·proxy·source·source-save·toggle·logTail·nginxReload·phpPools/cert 全形状逐一对齐 api 4.1.4/4.2.4；create req listen=string、update req listen=string[]（api 契约原样不归一）；SITE_ERROR_TEXT 14 码 + CERT_ERROR_TEXT 7 码集中映射（PageSpec 第 1 节）+ isSiteRollbackData/isSiteStaleData/isCertSitesData/fieldOfString unknown 收窄器；certId?/certStatus? 可选（D-11 未纳管引用缺失=「未纳管」orange 推导依据） |
| F03 api 层 | [x] | src/api/site.ts | 18 Action 一一对应（S7，函数名=Action 名）；4 GET 两角色 + 2 GET admin + 12 POST（http.ts 统一注入 X-CSRF-Token） |
| F04 页面组件 | [x] | src/views/site-mgmt/**（3 页面 + 8 局部组件 + errors.ts） | 见下方页面/组件清单与实现决策 |
| F05 状态 | [x] | src/stores/site.ts | PageSpec §1 声明 sitesStore + certsStore（枚举常量 store 按第 3 节偏差声明落 types/enums.ts 常量源不建 dict store）；sitesStore 跨页面共享根因=cert-list 申请/部署弹窗站点数据源；刷新纪律 D-01 实时投影：手动+动作驱动无轮询，写成功 loadList() 重取实况（不信任乐观更新） |
| F06 路由 | [x] | src/router/index.ts 追加 3 路由 + vite.config.ts /sites proxy + MainLayout globe/lock 图标 | /sites、/certificates 两角色同路由（requiresAuth 继承布局路由）；/sites/:siteId/source meta.roles:['admin']（既有守卫第 4 步 → 403 边界页，⚠️A17/ADR-018）；siteId 含 `/`与`#`（D-02 路径#序号）——跳转方 encodeURIComponent、vue-router 解码 params 契约成立；菜单项由后端 ModuleMenuRegistry 下发（后端 todo，globe/lock 预置映射未登记名兜底 AppsOutline） |
| F07 合规检查 | [x] | — | frontend-rules §6 F-01~F-05 全 0 命中 + site-mgmt 定时器 0（无轮询；source 页 keydown/beforeunload/media 监听 onUnmounted 配对）+ hex 0（rgba 语义色）；`npm run build`（vue-tsc --noEmit && vite build，cwd=frontend/）exit 0（24.56s；site-mgmt chunk 66.92 kB gzip 20.82、certificates 17.57 kB gzip 6.69；500kB 警告为既有 monitoring history echarts 块与 monaco 块——source 页共享既有 monaco 懒加载 chunk，非新增独立副本）+ 构建后二次 `npm run typecheck` exit 0 + `npx vitest run` 6 files/51 tests 全过（48 既有 + formatTime 3 新增） |

## 页面/组件清单（admin PageSpec 第 9 节口径）

| 页面 | 路由 | 视图文件 | readonly 差异（负空间落地） |
|------|------|---------|---------------------------|
| site-list | /sites | views/site-mgmt/index.vue | 无工具栏管理按钮（新建站点/重载 nginx）；无操作列（启用停用/源码编辑/日志入口整列不渲染）；筛选/刷新/扫描时刻/Drawer 只读视图保留 |
| cert-list | /certificates | views/site-mgmt/certificates/index.vue | 无「申请证书」工具栏；无操作列（部署/解除/续期重试/删除整列不渲染）；renew_failed lastError 提示保留（排障观测口径） |
| site-source | /sites/:siteId/source | views/site-mgmt/source/index.vue | 路由 roles:['admin'] 守卫 → readonly 直达 403 边界页（不可达 Monaco） |

局部组件：components/{SiteCreateModal, SiteDetailDrawer, RollbackDiffModal}.vue、certificates/components/{CertRequestModal, CertDeployModal, CertUndeployModal, CertDetailModal}.vue、errors.ts（siteActionError 兜底分流）。RollbackDiffModal 为 SITE_003 全部配置写接口共用（列表启停/Drawer 编辑/新建/证书部署解除/source 保存 7 处消费）。

## 已声明的实现决策（依据 PageSpec/API，不虚构）

1. **工具栏/查询区拆分**：admin「工具栏」严格=admin only（新建站点/重载 nginx，readonly 整体隐藏）；generatedAt「扫描于 HH:mm:ss」+ 刷新按钮落查询区两角色可见（PageSpec 4.1 空态与全局提示节为全局列表行为非 admin 工具栏行；service-mgmt 刷新先例同构）。
2. **Monaco nginx 高亮落 ini 词法**：Monaco 无内置 nginx 语言，站点配置文件（.conf 指令/键值风格）用 ini 词法（file-manager edit .conf 同映射先例）；worker-free 根入口动态 import 复用既有 monaco chunk（仅 source 路由触达）。
3. **SITE_014 装载分支**：siteSource 装载即拒（API 不下发超限 content）→ warning「请下载后本地处理」+ 返回列表（file-manager edit 拒入先例）；保存路径前端预校验优先（TextEncoder UTF-8 字节数 >1MiB 不发请求）。
4. **SITE_004 后静默重取基线**（source 页）：内容已落盘（mtime 已变）但响应无新 baseMtime——不重取则下次保存必误报 SITE_010；静默 siteSource 对齐 baseMtime（内容一致时才覆盖基线，失败静默、由下次保存如实暴露）。
5. **证书列「未纳管」推导**：certRef 非空且 certId 缺失 → orange「未纳管」（D-11 实时推导值缺失语义）；certRef=null → gray「无引用」；纳管命中 → cert_status colorTag + certRef NTooltip。
6. **CERT_003「查看已有证书」引导**：existingCertId 在当前筛选清单命中才打开行内详情 Modal（数据源=行数据，不虚构详情请求），未命中提示清除筛选（不构造虚假行）。
7. **deploy/undeploy/retry/delete 错误兜底**：CERT_002 附 data.currentStatus dict 文案；CERT_005 → NModal 引用站点清单 +「去解除部署」直接打开解除弹窗（预选目标证书）；retry loading 用 message.loading(duration 0) + destroy 配对（终态同步 ≤120s，D-09 文案同申请弹窗）。
8. **续期重试按钮仅 renew_failed 行渲染**（PageSpec 4.2 operList 条件）；删除 NPopconfirm 文案按来源区分（acme 连带托管文件 / imported 不动源站文件，I-04/⚠️A16）。
9. **Drawer 日志 Card**：打开 Drawer 即拉 logTail（spec 4.4 日志 Card logTail 拉取口径）；Tab/行数切换即重拉；truncated → 「仅展示最近 N 行」；SITE_011 → NEmpty 如实空态（⚠️A15 回退链穷尽不虚构）；readonly 唯一可操作区=刷新按钮。
10. **sitesStore.loadList 竞态防抖**：listLoading 期间新触发跳过（service-mgmt 先例）；迟到 detail/log 响应不写入已切换/关闭的 Drawer（siteId/show 双守卫）。
11. **AUTH_008 全局文案**：readonly 越界（正常 UI 不可达——负空间入口全不渲染）走既有全局拦截器文案（service-mgmt 决策 3 同口径）。
12. **expiring_soon 整行橙色浅底**：rowClassName + scoped :deep 覆盖（n-data-table 行级样式通道）。

## 门禁记录

- `npm run build`（= vue-tsc --noEmit && vite build，cwd=frontend/）：✅ exit 0（2026-11-14，24.56s）
- 构建后二次 `npm run typecheck`：✅ exit 0（components.d.ts 再生成后复验——service-mgmt TS2339 教训防复发）
- `npx vitest run`：✅ 6 files / 51 tests passed（48 既有无回归 + formatTime 3 新增：null/'—'、非法串透传、本地 HH:mm:ss 形态）
- frontend-rules §6 扫描（2026-11-14 实跑）：F-01 0 命中（src 全部 .vue 含 script setup）、F-02 0 命中（全 src 无 : any/as any）、F-03 0 命中、F-04 0 命中、F-05 0 命中；site-mgmt views 定时器 0 命中（source 页监听器 onUnmounted 配对清理）；site-mgmt views 硬编码 hex 0 命中（rgba 语义色）
- 修复过程一轮编译门控：首轮 vue-tsc 2 错（site-mgmt/index.vue certCell 闭包内 row.certStatus/certRef 收窄丢失 → 局部 const 先行收窄）→ 修复后全绿


# panel-core 前端生成任务清单（F01~F07）

# panel-core 前端生成任务清单（F01~F07）

> PageSpec：02-detailed/interaction/{admin,readonly}/panel-core/panel-core.md（admin v1.0 + readonly v1.0；readonly 负空间 4 变更 Action + 更新检查触发从紧，ADR-018）
> 类型对齐：02-detailed/api/api-panel-core.md v1.0（唯一事实源，7 接口）
> 产物于前序轮生成（INDEX.md 2026-11-14 复验轮记录）；本节为 2026-11-14 重派发轮 F01~F07 全量复验登记（逐文件对 PageSpec 重核 + 门禁本轮实跑）。

## 状态

| 任务 | 状态 | 产物 | 备注 |
|------|------|------|------|
| F01 模块分析 | [x] | 本文件 | 页面类型全 config×2（settings 路由 /settings 6 Card + about 路由 /about）；无列表页（PageSpec 第 4 节诚实声明：单例配置无台账）、无详情路由（第 6 节）；框架级职责（第 7 节）：侧边菜单 menuList 服务端下发（MainLayout）+ 首访 HTTPS 提示（settings 页横幅派生）+ 入口前缀透明（http.ts 相对路径 + 404 归 NOT_FOUND 不进业务分流） |
| F02 类型定义 | [x] | src/types/panel.ts + enums.ts（https_mode/listen_scope/panel_cert_source/config_effect_mode 4 字典 + 颜色方案） | detail 六分组（listen/entryPath/https/updateCheck/lang/session + pendingRestart）与 api 4.1.4 逐字段对齐；update 为部分字段集（listenAddress?: string \| null——scope≠custom 显式传 null 清残留，I-P02）；PANEL_001~006 文案集中 PANEL_ERROR_TEXT（第 1 节错误处理行逐字）；panel_cert 仅摘要（I-P06 key_pem 零下发）；session 透传字符串原样展示（格式契约未定义，W1 已知项） |
| F03 api 层 | [x] | src/api/panel.ts | 7 Action 一一对应（S7，函数名=Action 名）：configDetail/configUpdate/entryPathRegenerate/certUpload/menuList/versionGet/updateCheckRun；4 POST 走 http.ts 统一 X-CSRF-Token 注入；拦截器 PANEL_001~006 统一 toast + AUTH_008 边界文案 + AUTH_012 fields 逐字段提示（http.ts handleGlobalError） |
| F04 页面组件 | [x] | src/views/panel-core/**（settings/index.vue + 6 Card 组件 + about/index.vue） | 页面级横幅：pendingRestart warning（不可手动消除，D-P02）+ HTTP 明文 info（FR-07）；Card1 监听：scope 下拉/条件地址（I-P02 正则+PANEL_001 后端终审）/端口 1024..65535、角标「需重启」；Card2 安全入口：0→1 NModal 确认（风险文案）→成功路径大字 NModal+复制、1→0 NPopconfirm、regenerate NPopconfirm 警告、NCode 当前路径 readonly 全可见（ADR-018/FL 附录B）、角标「即时生效」；Card3 HTTPS：三态下拉+模式 hint、imported 才展开 PEM 导入区（私钥 password 输入承载遮罩态）、PANEL_003 切 imported 前置校验、self_signed 成功 toast、证书摘要（来源 colorTag/颁发者/有效期+30 天橙色临近）、keyPem 成功双清零回显（I-P06）；Card4 语言 zh-CN 单选项（I-P08）；Card5 会话只读透传无按钮（D-P08）；Card6 更新检查：开关 NPopconfirm 双文案、检查按钮 admin+开关开启双门控+loading（I-P05）、结果 tag（有新版本 blue/已是最新 green）、无自动重试（D-P09）；about：version 大字/gitCommit 截短 8 位/buildTime 本地时区/GPL-3.0+slogan，无检查更新入口（避免双入口）；readonly 负空间：4 Card 控件禁用、无保存/导入/重新生成/检查更新按钮、证书导入区不渲染（readonly 文档 5.1 六 Card 差异逐项落地） |
| F05 状态 | [x] | src/stores/panel.ts | PageSpec 第 1 节声明 panelStore：configDetail/configLoading/configError + menuItems/menuLoading/menuError + pendingRestart computed；每次变更成功统一 re-fetch（5.1 提交粒度决策）；布局挂载重拉菜单（换角色登录不残留，D-P07） |
| F06 路由 | [x] | src/router/index.ts（/settings、/about 挂 MainLayout children）+ vite.config.ts /panel proxy + MainLayout 菜单改服务端下发 | requiresAuth 继承布局路由（两角色无 roles 限制，PageSpec 第 7 节）；懒加载 + meta.title；守卫复用 account-auth 全局链（initCheck→sessionCurrent→roles），本模块无新增守卫（D-P11 全接口需会话） |
| F07 合规检查 | [x] | — | 本轮实跑（2026-11-14，cwd=frontend/）：frontend-rules §6 F-01~F-05 全 0 命中 + panel-core views 定时器 0（无轮询，R-09 无自动外呼）+ hex 0；`npm run build`（vue-tsc --noEmit && vite build）exit 0（32.56s；500kB 警告为既有 monaco/echarts 块非本模块产物）+ 构建后二次 `npm run typecheck` exit 0 + `npx vitest run` 6 files/51 tests 无回归 |

## 已声明的实现决策（依据 PageSpec/API，不虚构）

1. **私钥粘贴态遮罩**：Naive UI textarea 无遮罩能力 → 私钥 PEM 用 NInput type=password（show-password-on=click）承载「粘贴态遮罩」语义（HttpsCard 内注释登记）。
2. **session 透传值原样展示**：api D-P08 未定义 absoluteTimeout/idleTimeout 值格式（后端 A-P2 裁定=秒数整数）→ 前端 string 原样展示不换算（Gate 2 W1 已知项，类型两侧不一致待后续裁定，前端不单方面虚构格式）。
3. **菜单图标前端映射**：icon 为 ModuleMenuRegistry 登记字符串，MainLayout MENU_ICONS 维护映射、未知名兜底 AppsOutline（D-P07 前端不硬编码菜单结构，仅图标字形映射）。
4. **EntryPathGuard 404 语义**：未携带/错携带入口前缀 → 标准 404 无业务码（D-P03）→ http.ts 归 NOT_FOUND 通道提示，不进 PANEL_*/AUTH_* 分流（书签丢失前缀看到 404 属预期安全行为）。
5. **AUTH_008 全局文案**：readonly 越界走既有全局拦截器文案（types/auth.ts AUTH_008 含「只读角色禁止变更操作」ADR-018 口径），不为本模块单开分支。

## 门禁记录（2026-11-14 重派发复验轮实跑）

- frontend-rules §6 扫描：F-01 0 / F-02 0 / F-03 0 / F-04 0 / F-05 0 命中；panel-core views `setInterval|setTimeout` 0 命中（无轮询——R-09 外呼仅双门控手动触发）；panel-core views 硬编码 hex 0 命中
- `npm run build`（vue-tsc --noEmit && vite build）：✅ exit 0（32.56s；settings chunk 26.51 kB gzip 9.78、about 随 MainLayout 共 chunk；500kB 警告=既有 monaco 4119.32 kB 与 monitoring history 517.99 kB 块，非本模块产物）
- 构建后二次 `npm run typecheck`：✅ exit 0（components.d.ts 再生成后复验）
- `npx vitest run`：✅ 6 files / 51 tests passed（无回归；本模块无专属纯函数新增——PANEL_ERROR_TEXT/enums 常量为声明式映射）


# database-mgmt 前端生成任务清单（F01~F07）

> PageSpec：02-detailed/interaction/{admin,readonly}/database-mgmt/database-mgmt.md（admin v1.1 + readonly v1.1；readonly 同路由同组件按角色渲染子集——7 可见 Action + 16 负空间含 key Tab 不渲染，ADR-018/D0）
> 类型对齐：02-detailed/api/api-database-mgmt.md v1.1（唯一事实源，23 接口）
> 复用既有结构追加（http 拦截器/enums 字典源/守卫/菜单下发不另起炉灶）。

## 状态

| 任务 | 状态 | 产物 | 备注 |
|------|------|------|------|
| F01 模块分析 | [x] | 本文件 | 页面类型 config×1（/databases 实例清单）+ hybrid×2（mysql-manage /databases/mysql/:instanceId 三 Tab[库/用户/观测]、redis-manage /databases/redis/:instanceId 三 Tab[info/慢日志/key]——key Tab admin 专属 readonly 不渲染）；23 Action（GET 10：readonly 可用 7 + admin 专属 3[databaseCharsets/redisKeyList/redisKeyTtl]；POST 13=admin+CSRF）；readonly 负空间 16 |
| F02 类型定义 | [x] | src/types/database.ts + enums.ts 追加 4 字典（db_engine/db_conn_mode/instance_conn_state/grant_level）+ 13 审计动作码 | 实例档案 11 字段（cred_cipher 零下发 I-02；version 可空如实呈现）/库·用户·授权投影/key·慢日志·进程列表/23 接口 req·data 形状逐一对齐 api v1.1；DBM_ERROR_TEXT 20 码 + dbmErrorText/reasonOf（DBM_003/012/013 附 data.reason）；MYSQL_STATUS_FIELD_TEXT（D-08 白名单 10 项）/REDIS_INFO_FIELD_TEXT 标签映射；DUMP_SIZE_LIMIT_BYTES=512MiB；redis 引擎紫色无 NTag 语义型 → render.ts rgba 语义色（site-mgmt 先例） |
| F03 api 层 | [x] | src/api/database.ts + http.ts 追加 postDownload（downloadFile 重构出共用 unwrapStream） | 23 Action 一一对应（S7，函数名=Action 名）：10 GET + 13 POST（统一 X-CSRF-Token）；backupExport=POST 文件流（双形态分流 D-06 + 浏览器保存 fallback `{db}.sql`）；backupImport=multipart（postForm：instanceId/database/file）；redisKeyList cursor/pattern query（COUNT=200 服务端常量不传） |
| F04 页面组件 | [x] | src/views/database-mgmt/**（3 页面 + 7 局部组件 + render.ts） | 见下方页面/组件清单与实现决策 |
| F05 状态 | [x] | src/stores/database.ts | PageSpec §1 dbStore（实例档案缓存）：items/generatedAt/engineFilter + ensureLoaded（engineGuard 直达治理页先拉清单，D-07 进程内首探由 list 触发）+ findById（摘要条/守卫）+ applyCredentials（凭据成功行级直更 reachable/credentialsSet/version，D-03）；刷新纪律 D-04 无轮询（本 store 不持定时器）；库/用户/状态等过程数据留页面内不进 Pinia |
| F06 路由 | [x] | src/router/index.ts 追加 3 路由（engineGuard meta + 守卫第 5 步）+ vite.config.ts /db proxy + MainLayout MENU_ICONS 追加 database | /databases 两角色同路由；治理页 engineGuard 以 dbStore 档案校验：非数字 id/档案不存在→DBM_002 语义 toast+回清单、引擎不匹配→DBM_018 语义 toast+回清单（mariadb 走 mysql 路由）；菜单项由后端 ModuleMenuRegistry 下发（后端 todo，database 预置 DiscOutline 映射未登记名兜底） |
| F07 合规检查 | [x] | — | frontend-rules §6 F-01~F-05 扫描 0 命中（唯一 hex 命中=KeyBrowse CSS 注释内色值引用，非应用色）；database-mgmt views 定时器 0（无轮询）；`npm run build`（vue-tsc --noEmit && vite build，cwd=frontend/）exit 0（25.89s；database-mgmt chunk 4.48kB/mysql 82.95kB/redis 9.60kB；500kB 警告为既有 monaco/echarts 块非本模块产物）+ 构建后二次 `npm run typecheck` exit 0 + `npx vitest run` 7 files/61 tests 全过无回归 |

## 页面/组件清单（admin PageSpec 第 9 节口径）

| 页面 | 路由 | 视图文件 | readonly 差异（负空间落地） |
|------|------|---------|---------------------------|
| db-list | /databases | views/database-mgmt/index.vue | 无工具栏（无重新探测/刷新——进入页面即拉取）；操作列仅「查看」；行点击/查看入口保留（治理页内再按角色渲染） |
| mysql-manage | /databases/mysql/:instanceId | views/database-mgmt/mysql/index.vue | 无「修改 root 密码」；库/用户 Tab 无工具栏与操作列；观测 Tab 两视角全等；DBM_004 横幅文案改「待管理员配置凭据」 |
| redis-manage | /databases/redis/:instanceId | views/database-mgmt/redis/index.vue | 无「设置密码」；状态/慢日志两 Tab 全等（条数选择保留）；key 浏览 Tab 不渲染（非禁用置灰，D0 口径） |

局部组件：components/{InstanceCredentialsModal, PasswordSetModal}.vue（5.5/5.7 共用——字段同构，DBM_019 处置同）、mysql/components/{CreateDatabaseModal, CreateUserModal, GrantManageModal, ImportBackupModal}.vue、redis/components/KeyBrowse.vue、render.ts（engineTag/connStateTag/credentialsTag/identityText 共享渲染 + EngineTag/ConnStateTag 函数式组件形态）。

## 已声明的实现决策（依据 PageSpec/API，不虚构）

1. **identityKey 列呈现为组装形态**：instance/list 不下发 identity_key 原串（api 4.1.4 响应字段无此项）→ 实例标识列按 D-01 组装规则呈现 `host:port` 或 socket 路径（render.ts identityText），不虚构原串字段。
2. **DBM_019 恢复直达 5.1 弹窗**：mysql/redis 治理页各挂载一份 InstanceCredentialsModal（credentials 接口引擎无关，redis 实例同样适用）——dialog.error 强提示按钮直达，PageSpec 5.5「按钮直达 5.1 弹窗（recoverHint）」。
3. **观测 Tab 数据门控**：connState=unreachable 或 !credentialsSet 时 Tab 数据不自动拉取（横幅前置提示 DBM_003/004 语义，PageSpec 6.1「前置提示」）；凭据配置成功后 watch(governable) 触发当前 Tab 首拉。
4. **slowlog durationUs→ms 前端换算**（PageSpec 6.2 明示「durationUs→ms 呈现」2 位小数右对齐）；mysql innodb_buffer_pool_size 纯数值串经 formatBytes 人类可读（其余 variables 原样透传，D-08 白名单只读展示）。
5. **游标分页栈**（D-09）：pageCursors[i]=第 i 页请求 cursor，上一页出栈回退、下一页压栈 nextCursor；complete=true 禁用下一页+标注「已遍历完」；新查询复位栈以 cursor="0" 起始；TTL 行内查询态/结果按 key 收敛（换页清空）。
6. **readonly 无工具栏采字面负空间**（readonly 文档 4.1「无工具栏」）：重新探测与刷新均 admin only——readonly 进入页面即拉取（列表页无手动刷新入口；治理页观测 Tab 刷新按钮对 readonly 保留=观测 Tab 两视角全等，readonly 文档 6.1 Tab3「与 admin 版完全一致」）。
7. **移除/导出/导入在途互斥**（D-12）：busy 为前端在途标志（导出/导入执行期间该行与同实例操作禁用），DBM_016 由服务端终审——前端无法预知内存任务表，失败 toast「操作在途稍后重试」（第 1 节口径）。
8. **AUTH_008 全局文案**：readonly 越界（正常 UI 不可达——负空间入口全不渲染）走既有全局拦截器文案（service-mgmt 决策 3 / site-mgmt 决策 11 同口径）。
9. **EngineTag/ConnStateTag 函数式组件**（render.ts）：摘要条模板内需要 VNode 形态的字典 Tag（redis 紫色 rgba 覆盖 NTag type 不支持的场景）——Vue3 函数组件=返回 VNode 的函数，避免在 .vue 内写 tsx。

## 门禁记录

- `npm run build`（= vue-tsc --noEmit && vite build，cwd=frontend/）：✅ exit 0（25.89s）
- 构建后二次 `npm run typecheck`：✅ exit 0（components.d.ts 再生成后复验——service-mgmt TS2339 教训防复发；本轮实际命中同型问题一次：NUpload @change 载荷为 options 对象非数组（dts 刷新后 TS2322 暴露），已按 UploadOnChange 签名修复后双绿）
- `npx vitest run`：✅ 7 files / 61 tests passed（无回归；本模块无新增纯函数——dbmErrorText/reasonOf 为声明式映射+两行收窄，逻辑由 typecheck 与消费方路径覆盖）
- frontend-rules §6 扫描：F-01/F-02/F-03/F-04/F-05 全 0 命中；database-mgmt views 定时器 0 命中（无轮询 D-04）；hex 仅 KeyBrowse.vue:213 CSS 注释内色值引用 1 命中（非应用色，应用值为 rgba）


# terminal 前端生成任务清单（F01~F07）

> PageSpec：02-detailed/interaction/{admin,readonly}/terminal/terminal.md（admin v1.0 + readonly v1.0 零可达整域禁用——0 页面/0 路由/0 菜单/0 Action，ADR-018）
> 类型对齐：02-detailed/api/api-terminal.md v1.1（唯一事实源，2 端点：1 HTTP GET + 1 WS）
> 复用既有结构追加（http 拦截器/enums 字典源/守卫/菜单下发不另起炉灶）。

## 状态

| 任务 | 状态 | 产物 | 备注 |
|------|------|------|------|
| F01 模块分析 | [x] | 本文件 | 页面类型 code×1（终端页 /terminal，xterm.js 深度集成承载全部交互）；2 Action（userOptions GET + sessionConnect WS，S7 一一对应；帧协议 ready/input/resize/output/closed 为消息层不单列）；无列表页/无查询/无表格/无详情页（N/A 判定有据：帧反馈等价承载，PageSpec 第 4/6 节）；readonly 零可达（菜单不下发+路由守卫+服务端 AUTH_008 三层，无任何 readonly 代码分支） |
| F02 类型定义 | [x] | src/types/terminal.ts + enums.ts 追加 terminal_end_reason/terminal_identity_source + 2 审计动作码 + utils/terminal.ts（Base64 编解码分块防栈溢出） | userOptions data（items name 字典序）/ready 帧（sessionId/identity/limits 权威值）/closed 帧（reason/closedAt/durationSeconds）/客户端帧（input data·resize cols·rows）逐字对齐 api 4.1.4；parseServerFrame 逐字段收窄（I-06 前端镜像：残缺/未知→null=协议违规）；parseHandshakeFailure 容忍解析（纯符号串/JSON 包络两形态）；terminalErrorText 逐字落 PageSpec 第 1/5 节文案 |
| F03 api 层 | [x] | src/api/terminal.ts | 2 Action 一一对应（S7，函数名=Action 名）：userOptions 走 request()；sessionConnect=connectTerminalSession（WS 传输层封装：同源 wss/ws 地址 + buildQuery 复用 + readyReceived 追踪 + OPEN 态守卫发送；组件不裸连，frontend-rules §3）；无自动重连（api 4.1.2 辨析③） |
| F04 页面组件 | [x] | src/views/terminal/{index.vue, components/ConnectModal.vue, components/SessionPane.vue} + src/components/AppTerminal.vue | 三区布局：NTabs(card closable display-directive=show 保活) + 尾部「+」（达上限禁用+tooltip，活跃标签数=并发阈值）+ 状态条（ready 帧 limits 权威值 30min/8h + 连接 Tag）；AppTerminal=xterm.js 宿主（FitAddon 尺寸自适应+ResizeObserver，scrollback 5000 防内存膨胀，onUnmounted 配对清理 dispose/disconnect）；SessionPane=output 直写（Base64 解码字节流）+ truncated 黄色提示行（D-04）+ closed 帧 6 原因分级反馈（重连/重登/关闭，第 6 节表）；ConnectModal=2 字段 Dialog（身份 radio+系统用户 NSelect filterable，TERM_007 NAlert+重试+指定身份不可用默认身份仍可连，开框即拉不缓存 D-02） |
| F05 状态 | [x] | src/stores/terminal.ts | PageSpec §1 terminalStore：tabs[]（sessionId/runUser/limits/status/closeInfo）+ userOptions（loading/error）+ connecting（computed）+ dialogVisible + concurrentLimit（预置 5，ready 帧权威值覆盖）；WS 通道与 output writer 为模块级 Map（非响应式运行时资源）；无轮询（WS 即数据流）；disposeAll 页面卸载全部尽力 close（D-03 前端不承担一致性责任） |
| F06 路由 | [x] | src/router/index.ts 追加 /terminal + vite.config.ts /terminal proxy（ws:true）+ MainLayout MENU_ICONS 追加 terminal | meta roles:['admin']（本路由表对 PageSpec requiresAdmin 的既有承载形态，site-source 先例；守卫第 4 步→403 边界页）；菜单项由后端 ModuleMenuRegistry 下发（后端 todo，terminal→TerminalOutline 预置映射未登记名兜底；仅 admin 下发，ADR-018） |
| F07 合规检查 | [x] | — | frontend-rules §6 扫描（2026-11-15 实跑）：F-01~F-05 全 0 命中 + terminal views/AppTerminal 定时器 0（无轮询）+ hex 0（rgb/rgba 语义色）+ `new WebSocket` 全仓唯一命中=api/terminal.ts:68（api 层封装，组件不裸连）；`npm run build`（vue-tsc --noEmit && vite build）exit 0（30.29s；terminal chunk 306.01 kB gzip 79.26 仅 /terminal 路由触达；500kB 警告为既有 monaco/echarts 块）+ 构建后二次 `npm run typecheck` exit 0 + `npx vitest run` 8 files/77 tests 全过（61 既有 + terminal 16 新增：Base64 roundtrip/帧解析收窄/握手容忍解析/文案） |

## 已声明的实现决策（依据 PageSpec/API，不虚构）

1. **WS 握手拒绝详情为尽力而为通道（诚实声明）**：api 3.1 规定握手拒绝=升级前 HTTP 4xx/5xx + 错误码 JSON——浏览器 WebSocket API 不透出失败升级的 HTTP 错误体（平台限制，非实现缺失），仅 CloseEvent.reason 为服务端可选通道。落法：parseHandshakeFailure 容忍解析 reason（纯 TERM_*/AUTH_* 符号串或 JSON 包络 {code,data} 两形态），可识别则按 PageSpec 第 5 节流程表逐码分流（TERM_001 warning/002/003 重开对话框+自动重拉/004/006 error+移除连接中标签/AUTH_001/002 跳登录），不可识别走通用文案「终端连接失败：连接被拒绝或网络不可达」——不虚构错误码。后端（todo）若在 close reason 或升级后错误帧中携带符号码，前端零改动兼容。
2. **ready 后无 closed 帧的异常断开本地按 network_error 收口**：服务端 closed 帧（含 network_error 判定，D-03）到达前连接即断（如代理中断）→ 前端以本地时间合成 closeInfo{reason:'network_error'} 呈现同款红色反馈——与 api 4.1.2 六原因语义一致，仅 durationSeconds 为本地推算值。
3. **WS 封装位置**：frontend-rules §3 建议 composable；本模块 WS 为「一标签一通道」强状态对象（非通用 socket 复用场景），封装于 api 层 connectTerminalSession（传输层）+ terminalStore（会话状态机），SessionPane 只呈现——组件不裸连的规则意图达成。
4. **上限判定双源**：「+」禁用阈值取 concurrentLimit（预置 ADR-023 标准档 5，任一 ready 帧到达后以 limits.maxConcurrent 权威值覆盖）；服务端 TERM_001（data={limit,current}）为终审（I-05 跨浏览器计数前端不可知，不前端拦截代替）。
5. **重连=重走新建流程**（api 4.1.2 辨析③）：closed 反馈区「重新连接」打开新建会话对话框且复位默认身份（不继承任何状态——ConnectModal watch(show) 复位）。
6. **粘贴确认未实现**：stack-specs frontend-03 §5.2 建议「粘贴大文本提示确认」，但 PageSpec（交互唯一事实源）未定义该交互——不虚构未定义交互面；input 帧 16KB 上限（I-06）由服务端终审。
7. **AUTH_008 全局文案沿用**：readonly 越界（正常 UI 不可达）走全局拦截器既有码族分流；WS 侧若可识别 AUTH_008 reason 走 terminalErrorText 模块文案（PageSpec 第 1 节逐字）。
8. **xterm.js 依赖新增**：`npm install @xterm/xterm@^5.5.0 @xterm/addon-fit@^0.10.0` 真实执行（peer 匹配；PageSpec §1 组件体系给定选型 xterm.js+尺寸自适应 addon）；WebLinksAddon 未采用（PageSpec 仅声明尺寸自适应 addon）。

## 门禁记录

- `npm run build`（= vue-tsc --noEmit && vite build，cwd=frontend/）：✅ exit 0（2026-11-15，30.29s；修复 1 轮：初版用了 NAlert #action 插槽——本版 naive-ui dts 无该插槽（TS2339），按钮改独立操作行）
- 构建后二次 `npm run typecheck`：✅ exit 0（components.d.ts 再生成后复验——service-mgmt TS2339 教训防复发）
- `npx vitest run`：✅ 8 files / 77 tests passed（61 既有无回归 + terminal 16 新增）
- frontend-rules §6 扫描：F-01/F-02/F-03/F-04/F-05 全 0 命中；terminal views + AppTerminal 定时器 0 命中（无轮询；ResizeObserver/WS 于 onUnmounted 配对清理）；hex 0 命中（rgb()/rgba() 语义色承载终端底色）

# firewall 前端生成任务清单（F01~F07）

> PageSpec：02-detailed/interaction/{admin,readonly}/firewall/（admin v1.0 + readonly v1.0，readonly 为 admin 子集）
> 类型对齐：02-detailed/api/api-firewall.md v1.0（唯一事实源）

## 状态

| 任务 | 状态 | 产物 | 备注 |
|------|------|------|------|
| F01 模块分析 | [x] | 本文件 | 页面类型 hybrid×1（/firewall：页头后端状态 Card + backend=none 整页引导态 + 端口规则/监听端口对照双 Tab + 添加规则 Dialog + FW_005 危险确认 Modal + FW_007 部分生效 Modal）；5 Action（3 GET admin+readonly + 2 POST admin，S7 一一对应）；readonly 负空间 2 项（工具栏无添加按钮 + 操作列整列不渲染，ADR-018）；无详情页（api 无 detail 接口，rawSpec 列内承载后端互认）；刷新纪律 = status 先行 / Tab 懒加载 / 写后对照标记过期 / 无轮询无防抖定时器 |
| F02 类型定义 | [x] | src/types/firewall.ts + enums.ts 追加 | status/rule/port 响应、create/delete req、FW_005 data（{panelPort,rulePort,action,hint}）逐字对齐 api 4.1.4；4 字典入 enums.ts（firewall_backend/firewall_rule_action/firewall_rule_protocol/listen_port_state，PageSpec 第 3 节颜色方案：allow=green/deny=red/reject=warning橙/limit=info蓝/allowed_by_rule=green/no_allow_rule=warning橙）；FW_001~010 文案逐条落 PageSpec 第 1 节；2 审计动作码 firewall_rule_created/deleted 入 AUDIT_ACTION_TEXT |
| F03 api 层 | [x] | src/api/firewall.ts | 5 Action 一一对应（S7，函数名=Action 名）：backendStatus/ruleList（query action 逗号多值+protocol）/portList/ruleCreate/ruleDelete；POST 经 http.ts 统一 X-CSRF-Token |
| F04 页面组件 | [x] | src/views/firewall/index.vue + components/AddRuleModal.vue | 页头状态 Card（backend/active Tag、version null→「—」、supportedActions 权威面中文逗号串、刷新=重拉 status+当前 Tab）；引导态 NEmpty 整页降级（不渲染双 Tab）；FW_010 页面级 NAlert+重试不显示过期数据；Tab1 动作多选/协议筛选（变更即触发）+添加按钮（admin，在途禁用）+规则表（8 列+操作列，isPanelPort 预警 NTag+Popconfirm 加强文案，rawSpec 等宽原值透传）；Tab2 对照表（7 列，external Tag、openState dictTag、risk-row 橙底高亮+图例+readonly 处置线索文案）；AddRuleModal 4 字段（port 正则+1-65535+N≤M、source 可空 IPv4/IPv6 CIDR 结构性预校验、action options=supportedActions 权威面）；FW_005 危险确认 Modal（hint 优先+命中详情 panelPort×rulePort×action Tag，确认=原 req 追加 confirmDanger 重提交，不记忆）；FW_007 部分生效 Modal（固定文案承载两层语义，关闭后重拉 ruleList）；FW_008 error+data.reason（Dialog 保持打开）；FW_003 warning+自动重拉；FW_006 warning+关 Dialog 不重拉（未发生变更） |
| F05 状态 | [x] | src/stores/firewall.ts | PageSpec 第 1 节声明面全落：backend/status 态（含 statusFailed 重试）、ruleItems/ruleTotal/ruleGeneratedAt/ruleLoaded、portItems/portLoaded/portStale（写后过期标记，D-04）、listParams、inFlightKeys+hasInFlight、addDialogVisible；刷新纪律：loadRules/loadPorts 内置 status 先行守卫（backend=null/none 直接 return）、FW_009 兜底 markNoBackend 降级引导态、FW_010 markUnavailable 清空数据；无定时器（无轮询无防抖——筛选为离散控件变更即时触发） |
| F06 路由 | [x] | src/router/index.ts 追加 /firewall | requiresAuth 继承布局路由（admin+readonly 同路由组件内子集渲染——无添加按钮+操作列整列不渲染）；vite proxy /firewall 追加；MainLayout MENU_ICONS 预置 flame=FlameOutline（后端 ModuleMenuRegistry todo，未登记名兜底口径同先例） |
| F07 合规检查 | [x] | — | frontend-rules §6 扫描（2026-11-15 实跑）：F-01~F-05 全 0 命中 + firewall views 定时器 0 命中（无轮询无防抖）+ hex 0 命中（风险行橙底用 rgba()）；`npm run build`（vue-tsc --noEmit && vite build）exit 0（30.91s；firewall chunk 18.86 kB gzip 7.38 仅 /firewall 路由触达；500kB 警告为既有 monaco/echarts 块）+ 构建后二次 `npm run typecheck` exit 0 + `npx vitest run` 8 files/77 tests 全过（既有无回归，firewall 无新增测试——校验器为组件内私有纯函数，无 composable/工具新增） |

## 已声明的实现决策（依据 PageSpec/API，不虚构）

1. **FW_007 data 不渲染字段值（诚实声明）**：api 3.4 FW_007 仅声明「data 两层状态」未定字段名（permanent/runtime 等），PageSpec 5.4 也只给固定文案「永久配置已写入，运行态应用失败——规则将在防火墙服务重启后生效。」→ 落地为 NModal 固定文案承载两层语义（D-08 不回滚不虚构），data 不做猜测性字段渲染；后端落定字段名后可零文案风险补挂。
2. **direction 列呈现「入站」**：api 4.1.1 direction 恒 "ingress"（常量、响应呈现用）；一致性 #12 原值透传清单（rawSpec/proto/addr/version）不含 direction → 中文界面呈现「入站」，非翻译失真点。
3. **source 的 IPv6 校验为结构性预校验**：前端预校验（'::' 至多一次+十六进制组 1-4 位+前缀 ≤128）+ 服务端 FW_002 终审（R-14）；内嵌 IPv4 形态（::ffff:1.2.3.4）等完整压缩校验不在前端重复实现——PageSpec 5.1 定位即「前端预校验+服务端兜底」。
4. **FW_006 后不重拉清单**：等价规则已存在=未发生变更（D-07 不静默），PageSpec 5.4 仅要求 warning+关闭 Dialog → 不虚构「重拉规则/对照过期」动作（对照无变化）。
5. **页头 backend 摘要双源同步**：rule/list 响应随行 backend{backend,active} 摘要（api D-02「页头呈现」）→ store.syncBackendSummary 同步页头 Tag（version/supportedActions 仍以 status 响应为权威源，写操作后不重拉 status——PageSpec 第 2 节纪律）。
6. **statusFailed 首拉网络级失败页头重试**：api 对 status 端点无专属错误码（FW_010 归规则面调用）→ 首拉网络失败时页头 NAlert+重试（后端状态 Card 内），不虚构错误码分流。

## 门禁记录

- `npm run build`（= vue-tsc --noEmit && vite build，cwd=frontend/）：✅ exit 0（2026-11-15，30.91s，一次通过）
- 构建后二次 `npm run typecheck`：✅ exit 0（components.d.ts 再生成后复验——terminal 先例防复发）
- `npx vitest run`：✅ 8 files / 77 tests passed（既有无回归）
- frontend-rules §6 扫描：F-01/F-02/F-03/F-04/F-05 全 0 命中；firewall views 定时器 0 命中（无轮询无防抖）；hex 0 命中（rgba() 语义色承载风险行橙底）


# docker-mgmt 前端生成任务清单（F01~F07）

> PageSpec：02-detailed/interaction/{admin,readonly}/docker-mgmt/（admin v1.0 + readonly v1.0，readonly 为 admin 子集——公共页定义以 admin 文档为单一事实源）
> 类型对齐：02-detailed/api/api-docker-mgmt.md v1.0（唯一事实源，24 端点）

## 状态

| 任务 | 状态 | 产物 | 备注 |
|------|------|------|------|
| F01 模块分析 | [x] | 本文件 | 页面类型 hybrid×1（/docker：页头 daemon 状态 Card + unavailable 引导态 NEmpty + 容器/Compose/镜像/卷/网络五 Tab + 容器详情 Drawer（stats+日志双 Card）+ Compose 日志 Drawer + 拉取镜像 Dialog）；24 Action（9 GET admin+readonly + 2 WS admin+readonly + 13 POST admin，S7 一一对应）；readonly 负空间 13 项（容器六写→操作列整列不渲染 + pull→无拉取按钮 + 镜像/卷/网络删除→操作列整列不渲染 + compose 三写→操作列整列不渲染（项目名 link→日志 Drawer 保留），ADR-018）；无详情路由页（观测经 Drawer 按需拉取）；刷新纪律 = status 先行 / Tab 懒加载（切回不重拉除非过期）/ 容器六动作→重拉容器+compose 标记过期 / 资源删除→重拉本清单+容器标记过期 / compose 动作→重拉 compose+容器 / Drawer 打开按需 stats+logs(tail=500) / 无轮询 / WS end 帧停止不重连 / 卸载清理全部 WS 订阅 |
| F02 类型定义 | [x] | src/types/docker.ts + enums.ts 追加 | daemon status（state/version/detail 可空）/container item（state 三态∪过渡态透传 string、composeProject null、supportedActions: ContainerAction[]）/logs/stats/五动作+delete req/result/image/volume/network/compose 投影与 req/result 逐字对齐 api 4.1-4.3；DK_004 data{currentState,action,allowedActions}/DK_015 data{workingDir,configFile,missing} 收窄器；WS 帧协议 DockerLogFrame=log/end + parseDockerLogFrame 逐字段收窄（未知 reason/残缺→null=协议违规）；3 字典入 enums.ts（container_status[running=green/paused=orange/stopped=gray]、docker_daemon_state[available=green/unavailable=orange]、compose_action[操作集口径非 dict 组件用途]）+ ContainerAction/DockerLogEndReason 类型 + DOCKER_LOG_END_REASON 五 reason 分级（daemon_lost=red 余 info）+ isDockerLogEndReason 收窄；DK_001~018 文案逐条落 PageSpec 第 1 节（DK_ERROR_TEXT）+ daemonDetailText（not_installed/socket_unreachable→固定文案，其余原样透传）+ CONTAINER_ACTION_TEXT/COMPOSE_ACTION_TEXT 操作列渲染文案（第 3 节字典清单未列此 Key，入 types/docker.ts 不入 enums 枚举族）；13 审计动作码入 AUDIT_ACTION_TEXT |
| F03 api 层 | [x] | src/api/docker.ts | 24 Action 一一对应（S7，函数名=Action 名）：9 GET（daemonStatus/containerList[state 逗号多值+composeProject 等值]/containerLogs[tail]/containerStats/imageList/volumeList/networkList/composeList/composeLogs）+ 13 POST（容器六动作/imagePull/imageDelete/volumeDelete/networkDelete/composeUp/Down/Restart，CSRF 经 http.ts 统一注入）+ 2 WS（connectContainerLogsFollow/connectComposeLogsFollow——共用 connectLogFollow 传输层：同源 wss/ws+buildQuery 复用+end 帧追踪+OPEN 守卫 close；组件不裸连，frontend-rules §3 terminal 先例；无自动重连） |
| F04 页面组件 | [x] | src/views/docker/index.vue + components/{ContainerDetailDrawer,ComposeLogsDrawer,LogFollowCard,PullImageModal}.vue | 页头 daemon 状态 Card（接管状态 Tag/version「Docker {v}」null→「—」/unavailable 灰字 detail/刷新=重拉 status+当前 Tab）；引导态 NEmpty 整页降级（detail 文案拼入，不渲染五 Tab）；五 Tab 懒加载（切回不重拉除非过期）；Tab1 状态多选+Compose 项目筛选（300ms 去抖，onUnmounted 配对清理）+容器表（名称 link→详情 Drawer/状态 dict colorTag 过渡态透传 blue+Tooltip/Compose 项目 null→「—」独立容器 Tooltip/操作列 supportedActions 权威渲染六按钮[启停/重启/暂停恢复/删除 Popconfirm 分级文案，过渡态行灰字「执行中」]，行级 inFlight 全按钮 loading/禁用）；Tab2 Compose 表（项目名 link→日志 Drawer/工作目录·配置文件等宽透传/容器数+Tooltip 明细/定位 Tag[locatable=green 可操作/red 定位失败+Tooltip]/操作列 up/restart/down[加强文案]+日志[locatable=false 全禁用+兜底 Tooltip]，同项目 inFlight 互斥）；Tab3 拉取按钮（admin）+镜像表（ref 等宽/大小右对齐 formatBytes/删除 Popconfirm）；Tab4 卷表（删除加强文案数据永久销毁）；Tab5 网络表（子网 null→「—」/预定义网络如实呈现）；PullImageModal 1 字段（ref 结构性预校验+DK_012 服务端回填，submitting 期「拉取中…」+底部灰字 600s 可知会）；ContainerDetailDrawer 720px（stats 快照 Card[CPU/内存 formatBytes·formatPercent/刷新快照/无轮询]+日志 Card）；ComposeLogsDrawer 720px（label 透传灰字+locatable=false NAlert 降级+容器页兜底引导）；LogFollowCard 共用（tail 档位 NSelect[跟随中禁用]/实时跟随 NSwitch/等宽滚动区自动滚底/end 帧 info·error 分级 toast+断流行/数据生成于）；DK_003/004[权威动作面中文串]/005/007/009[+重拉清单]/006/008/010[+data.reason]/011/012[字段错误]/013[Dialog 保持打开]/014[+重拉 compose]/015[label 路径+兜底]/016/017[+reason]/018/002[兜底引导态] 全分流 |
| F05 状态 | [x] | src/stores/docker.ts | PageSpec 第 1 节声明面全落：daemon/status 态（含 statusFailed 重试）+五清单（items/total/generatedAt/loading/loaded + compose/container stale 过期标记）+listParams+inFlightKeys（container:/compose:/image:/volume:/network: 前缀键行级互斥）+pullDialog/pullInFlight+logDrawer 态（logTarget/logLines/logTail/logLoading/logGeneratedAt/followActive/followEnded/followBroken/stats/statsLoading）；WS 句柄为模块级非响应式变量（terminal 先例）；刷新纪律：loadX 内置 status 先行守卫（daemon=null/unavailable 直接 return）+DK_002 兜底 markDaemonUnavailable（合成 unavailable+清空五清单——未装≠无容器 I-01）；Tab 懒加载 ensureTabLoaded/写后标记 markComposeStale/markContainersStale；观测 Drawer：open 复位态（logs/stats 由 Drawer 组件拉取分流错误）、closeLogDrawer 即断 WS 无残留、startFollow 清空尾部缓冲（D-03 握手携 tail 服务端先回放）、end 帧/异常断流收口、MAX_LOG_LINES=5000 防膨胀；无定时器（无轮询——stats 为单次快照 D-07） |
| F06 路由 | [x] | src/router/index.ts 追加 /docker + vite.config.ts /docker proxy（ws:true）+ MainLayout MENU_ICONS 追加 docker=CubeOutline | requiresAuth 继承布局路由（admin+readonly 同路由组件内子集渲染——五 Tab 操作列整列不渲染+无拉取按钮，13 写负空间 ADR-018）；WS 升级端点同源代理需 ws:true（D-03）；菜单项由后端 ModuleMenuRegistry 按角色下发（对两角色均可见，未登记名兜底口径同先例） |
| F07 合规检查 | [x] | — | frontend-rules §6 扫描（2026-11-15 实跑）：F-01~F-05 全 0 命中 + docker views 定时器仅 composeProjectTimer 去抖 1 处（onUnmounted clearTimeout 配对清理）+ hex 0 命中（rgba() 语义色）+ `new WebSocket` 全仓 2 命中均为 api 层（terminal.ts:68/docker.ts:198——组件不裸连）；`npm run build`（vue-tsc --noEmit && vite build）exit 0（27.31s；docker chunk 32.36 kB gzip 11.04 仅 /docker 路由触达；500kB 警告为既有 monaco/echarts/history 块）+ 构建后二次 `npm run typecheck` exit 0 + `npx vitest run` 9 files/87 tests 全过（77 既有无回归 + docker 10 新增：帧收窄/DK_004·015 data 收窄/reason 透传/detail 文案/DK 文案 18 码覆盖） |

## 已声明的实现决策（依据 PageSpec/API，不虚构）

1. **开启跟随时清空尾部缓冲（D-03 握手携 tail 的推论）**：WS 握手 query 携 tail 且「握手成功先回放尾部 tail 行→流式推送」——若保留 GET 尾部行再叠加回放帧，同段日志会双份。落法 startFollow 先清 logLines/logGeneratedAt，由 WS 回放重建尾部（档位语义在跟随通道内自洽）。
2. **异常断流本地收口（terminal network_error 同判）**：浏览器 WS API 不区分握手拒绝与网络断开（end 帧未达即 onClose）→ 前端以 followBroken 布尔收口，呈现红色「日志流连接异常中断，可重新开启跟随重试」；end 帧五 reason 按 PageSpec 5.1/5.2 分级（container_removed/container_stopped/project_gone/locate_failed→info、daemon_lost→error）且不虚构第六种 reason；协议违规帧（收窄失败）同异常断流收口。无自动重连（重开跟随开关=重试入口）。
3. **跟随中行数档位禁用**：跟随握手自带 tail 回放，档位变更的「重拉尾部」语义与流式通道冲突（PageSpec 5.1 未定义组合行为）——跟随期间禁用 NSelect，停止跟随后可变更并重拉。
4. **Compose 项目筛选 300ms 去抖**：PageSpec 4.1「参数变更即触发 containerList」对文本输入逐键触发会与 store loading 防抖（在途丢弃最新参数）冲突——以 300ms 窗口合并（意图=无独立「查询」按钮即时生效），setTimeout 于 onUnmounted clearTimeout 配对清理；状态下拉（离散控件）即时触发。
5. **readonly 过渡态可见性承载**：readonly 文档第 4 节「过渡态行『执行中』灰字提示保留」——「执行中」灰字位于操作列（4.2），而操作列对 readonly 整列不渲染（负空间第 2 节）；readonly 的过渡态可见性由状态列透传 blue Tag + Tooltip「操作进行中，完成后自动回落」（4.1）承载，两文档合并后的诚实落法。
6. **成功反馈文案口径**：容器六动作「{动作}指令已执行」逐字 PageSpec 4.2；镜像/卷/网络删除与 Compose 三动词 PageSpec 未给逐字成功文案——沿用全站句式（「镜像已删除/卷已删除/网络已删除」firewall「规则已删除」先例；Compose「{verb} 指令已执行」与容器句式同构）。
7. **DK_015 双落点**：操作列 locatable=false 时按钮 disabled+Tooltip（含容器页兜底口径，4.3 前置呈现）；伪造直调（正常 UI 不可达）回落 message.warning 携 label 记录路径（第 1 节「NAlert warning 呈现 label 路径」的列表上下文落法）；Drawer 内（5.2）独立 NAlert 降级提示 + 兜底引导。
8. **WS 封装位置**：frontend-rules §3 建议 composable；本模块 WS 为「一 Drawer 一通道」强状态对象，封装于 api 层 connectLogFollow（传输层）+ dockerStore（订阅状态机与句柄），组件不裸连——规则意图达成（terminal 先例同判）。
9. **观测面错误在 Drawer 组件分流**：store 的 loadLogs/loadStats 将非 DK_002 错误向调用方抛出（Drawer 组件有 Message 上下文按码分流：DK_003→warning+自动重拉容器清单、DK_014→warning+重拉 compose）；清单类加载错误由 store 消化（DK_002→降级引导态，其余拦截器统一提示）——与 firewall「store 无 Message 上下文」先例一致。

## 门禁记录

- `npm run build`（= vue-tsc --noEmit && vite build，cwd=frontend/）：✅ exit 0（2026-11-15，27.31s，一次通过；docker chunk 32.36 kB gzip 11.04）；新增 docker.test.ts 后复跑 exit 0（29.41s——vue-tsc 覆盖测试文件终验）
- 构建后二次 `npm run typecheck`：✅ exit 0（components.d.ts 再生成后复验——terminal 先例防复发）
- `npx vitest run`：✅ 9 files / 87 tests passed（77 既有无回归 + docker 10 新增：src/utils/__tests__/docker.test.ts——帧收窄/收窄器/reason 透传/detail 文案/DK_001~018 文案覆盖）
- frontend-rules §6 扫描：F-01/F-02/F-03/F-04/F-05 全 0 命中；docker views 定时器仅去抖 1 处（onUnmounted 配对清理）；hex 0 命中（rgba() 语义色）；`new WebSocket` 全仓仅 api 层 2 处（组件不裸连）


# cron-backup 前端生成任务清单（F01~F07）

> PageSpec：02-detailed/interaction/{admin,readonly}/cron-backup/（admin v1.0 + readonly v1.0；readonly 为 admin 子集——公共页定义以 admin 文档为单一事实源，10 个 ✗ 接口负空间逐项对应，ADR-018）
> 类型对齐：02-detailed/api/api-cron-backup.md v1.0（唯一事实源，15 接口）
> 复用既有结构追加（http 拦截器/enums 字典源/守卫/菜单下发不另起炉灶）。

## 状态

| 任务 | 状态 | 产物 | 备注 |
|------|------|------|------|
| F01 模块分析 | [x] | 本文件 | 页面类型 hybrid×2（/cron 计划任务页双 Tab[计划任务+执行日志]+预置位只读分区+任务 Dialog+触发结果 Modal；/cron/backup 备份页双 Tab[备份任务+备份历史]+任务 Drawer 720px+执行详情 Drawer 640px）；15 Action（GET 6=readonly 可用 5+admin 专属 download 1；POST 9=admin+CSRF）；readonly 负空间 10 项（两表操作列整列不渲染+新建按钮×2+任务 Dialog/任务 Drawer/触发 Modal 宿主不渲染+详情 Drawer 下载按钮不渲染）；刷新纪律=Tab 懒加载/写后重拉/trigger 不重拉/立即备份 runId+历史 Tab 高亮 10s/详情 Drawer 5s 轮询（转终态即停/关闭即停/hidden 暂停）/历史列表不自动轮询 |
| F02 类型定义 | [x] | src/types/cron.ts + enums.ts 追加 5 字典 + 10 审计动作码 + utils/format.ts 追加 formatDurationMs/formatRelative | 三线类型逐一对齐 api §4：cron_job 投影（jobKey 不透明引用 D-01/remark null/systemEntries 文件粒度）/backup_job（sourceItems 判别联合+targetConfig 判别联合 I-09/I-10+lastRun 读时派生 D-11）/backup_run（快照 D-12+files manifest 清单）；CB_001~009 文案集中 CB_ERROR_TEXT（PageSpec 第 1 节逐字）+isStringArray/cronValidateErrors 收窄器（CB_002 data.errors[]）+复用 types/auth isFieldErrorList（CB_001 data.fields[] 同 {field,message} 形状）；5 字典颜色照 PageSpec 第 3 节（running=info/success=success/failure=error；panel=info/existing=default；local=success/rsync=info；cron=default/manual=info） |
| F03 api 层 | [x] | src/api/cron.ts + http.ts 追加 requestText | 15 Action 一一对应（S7，函数名=Action 名）：logTail 走 requestText（200+text/plain 纯文本通道 D-04——JSON 形态回落包络错误分流）；backupRunDownload 走 downloadFile+api 层触发浏览器原生保存（file-manager entryDownload 先例，Content-Disposition 原名兜底 manifest 文件名）；delete/update 三接口成功 data 形状契约未声明 → Promise<unknown> 不消费不虚构 |
| F04 页面组件 | [x] | src/views/cron-backup/**（2 页面 + 5 局部文件） | 见下方页面/组件清单与实现决策 |
| F05 状态 | [x] | src/stores/cron.ts | PageSpec §1 声明面全落（jobItems/systemEntries/logTailText/backupJobItems/runItems/runTotal/listParams/inFlight×9/triggerModal/runDrawer/backupFormDrawer）；在途互斥前缀键（job-form/job-del:/job-trigger:/backup-form/backup-del:/backup-run:/backup-download:，validate 在组件 checking 态承载）；详情 Drawer 轮询定时器在 RunDetailDrawer 组件持有（onUnmounted 配对）store 供数据通道+转终态刷新一次列表+CB_007 关 Drawer+迟到响应守卫；高亮 10s 定时器 store 持有 resetBackupView 配对清理（页面 onUnmounted 调用）；resetCronView/resetBackupView 清 Dialog/Drawer/inFlight |
| F06 路由 | [x] | src/router/index.ts 追加 2 路由 + vite.config.ts /cron proxy + MainLayout MENU_ICONS 追加 cron/backup | /cron、/cron/backup 两角色同路由（requiresAuth 继承布局路由，PageSpec 第 7 节）；菜单项由后端 ModuleMenuRegistry 按角色下发（后端 todo，cron=TimerOutline/backup=CloudUploadOutline 预置映射未登记名兜底 AppsOutline） |
| F07 合规检查 | [x] | — | frontend-rules §6 扫描（2026-11-15 实跑）：F-01~F-05 全 0 命中 + cron-backup 定时器 3 处（校验防抖/详情轮询/高亮 10s）均 onUnmounted/reset 配对清理 + visibilitychange 监听 onMounted/onUnmounted 配对 + hex 0 命中（rgba 语义色）；`npm run build`（vue-tsc --noEmit && vite build）exit 0（30.26s；backup chunk 22.78 kB gzip 8.63 仅 /cron/backup 路由触达；500kB 警告为既有 monaco/echarts/history 块非本模块产物）+ 构建后二次 `npm run typecheck` exit 0 + `npx vitest run` 10 files/105 tests 全过（87 既有无回归 + 新增 18：formatDurationMs 5/formatRelative 8/cron.test.ts 5〔CB 文案 9 码覆盖+收窄器〕） |

## 页面/组件清单（admin PageSpec 第 9 节口径）

| 页面 | 路由 | 视图文件 | readonly 差异（负空间落地） |
|------|------|---------|---------------------------|
| cron 计划任务页 | /cron | views/cron-backup/index.vue | 工具栏仅刷新（新建按钮不渲染）；任务表操作列整列不渲染（编辑/手动触发/删除）；预置位分区/执行日志/行数档位完全一致 |
| backup 备份页 | /cron/backup | views/cron-backup/backup/index.vue | 工具栏仅刷新；任务表操作列整列不渲染（立即备份/编辑/删除）；历史筛选+分页+详情入口保留；详情 Drawer 产物清单可见、下载按钮不渲染+表尾指引文案 |

局部组件：components/{JobEditModal, TriggerResultModal, BackupJobDrawer, RunDetailDrawer, ExprPreview}.vue + expr-validate.ts（校验预览共用 composable）+ backup/index.vue 消费 Drawer 两枚。

## 已声明的实现决策与偏差（依据 PageSpec/API，不虚构）

1. **编辑备份任务无法回填源清单与目标配置（契约缺口如实声明，非实现缺失）**：api backup-job/list 仅回传 sourceSummary[{type,ref}]/targetBrief 概要（4.1.4），不下发 sourceItems/targetConfig 结构化定义且无 detail 接口——database 项 ref 为「实例名·库名」展示串无法还原 instanceId。落法：编辑 Drawer 回填 name/targetType/retentionCount/scheduleCron，源与目标重选（update 为整体替换语义）；顶部 NAlert info 呈现当前定义概要（sourceSummary Tag+targetBrief+scheduleCron）与替换提示。后端若增补结构化回显可零 UI 语义变更补挂回填。
2. **logTail 纯文本通道**：api 3.1/4.3.4 规定 /cron/log/tail 为 200+text/plain（日志行流非 JSON 数组）→ http.ts 追加 requestText（Accept: text/plain；JSON 形态回落 unwrapEnvelope 错误分流；404 归 NOT_FOUND 通道）。
3. **表达式校验预览双消费共用**：JobEditModal（expr 字段）与 BackupJobDrawer（scheduleCron 字段）复用同一 useExprValidate composable + 同一 jobValidate Action（api D2 口径-2 单一表达式语义）；500ms 防抖定时器 composable 内 onUnmounted 配对清理；valid 前保存按钮禁用（预览为提交前置，PageSpec 5.1）；非 CB_002 错误占位「校验服务暂不可用」灰红提示（拦截器已 toast，避免「校验中」悬停）。
4. **手动触发同步语义**：行级 inFlight（触发期间三按钮禁用+触发按钮 loading+tooltip「执行中，最长等待 120 秒」）；结果 Modal 四态 Tag（超时/退出码 0/退出码 n/异常终止 null）+耗时灰字+输出等宽滚动区；输出截断标注以 output.length ≥ 64KB 为唯一可观测信号（api D-05 尾部截断常量）前置「…（输出过长，已截断保留尾部）」行；无重试按钮+「如需重试请再次手动触发」路径文案（PageSpec 5.2 逐字）。
5. **详情 Drawer 轮询落组件**：定时器由 RunDetailDrawer 持有（watch show=false 即停 / watch status 转终态即停 / document.hidden 暂停恢复 / onUnmounted clearInterval+removeEventListener 四路配对）；store 供数据通道——loadDetail 内转终态自动刷新一次 backupRunList（第 2 节纪律）、CB_007 关 Drawer+重拉列表（文案经 detailError 由组件 message.warning 提示，monitoring「store 无 Message 上下文」先例）。
6. **失败源 red 标注为文本命中**：api errorMessage 为「含失败源标识与错误摘要」的文本（无结构化失败源映射）→ sources[] 行以 errorMessage.includes(ref) 判定 red 标注（PageSpec 第 6 节「error_message 中对应源 red 标注」的唯一可得落法，不虚构字段）。
7. **已删任务两处弱引用呈现**（api D-12）：历史筛选下拉当前值不在任务清单时以「#{id}（任务已删）」label 呈现（仍可筛选）；历史表任务列 jobId 不在清单时灰字小标「任务已删」（jobName 快照仍可读）。
8. **AUTH_008 全局文案沿用**：readonly 越界（正常 UI 不可达——负空间入口全不渲染）走全局拦截器既有码族文案（service-mgmt 决策 3/site-mgmt 决策 11 同口径），不为本模块单开「计划任务与备份」变体分支。
9. **retentionCount UI 初始值 7**：api 契约无默认值（必填 1-100），Drawer 新建态取 7 为前端表单初始值（服务端按提交值落库，无契约影响）。
10. **rsync port 可空默认 22**：NInputNumber 空值提交时省略 port 字段（api target_config port? 可选，服务端默认 22，D-06）。

## 门禁记录

- `npm run build`（= vue-tsc --noEmit && vite build）：✅ exit 0（2026-11-15，30.26s，首轮 3 错——store return 漏导出 backupFormVisible/editingBackupJob ×3 连锁 + sysEntryColumns 未用参数 ×1——修复后通过）
- 构建后二次 `npm run typecheck`：✅ exit 0（components.d.ts 再生成后复验——service-mgmt TS2339 教训防复发）
- `npx vitest run`：✅ 10 files / 105 tests passed（87 既有无回归 + 新增 18）
- frontend-rules §6 扫描：F-01/F-02/F-03/F-04/F-05 全 0 命中；cron-backup 定时器 3 处配对清理核验；hex 0 命中（rgba 语义色）

## Gate 2 FAIL 第 1 轮修复记录（2026-11-15，报告 05-reports/gate2-cron-backup-2026-11-14.md）

| 项 | 修复 | 决策登记 |
|----|------|---------|
| E1（阻断：backup-job/list sourceSummary[] 与 backup-run/detail sources[] 后端未产出契约 ref 字段） | 新增 `src/utils/cronback.ts` 归一适配器并接线 `src/api/cron.ts`（backupJobList/backupRunDetail 响应逐项过 `normalizeSourceRef`）：①契约分支 `{type,ref}` 直通（后端投影层补 ref 后零改动接管）；②后端现状分支（dto.go SourceItemDTO `{type,path?,instanceId?,database?}`）推导——site_dir→ref=path / database→`实例 {id}·{库}`（缺库=整实例；instanceId 为快照事实，实例名跨域不可得，已删实例如实以 id 呈现，D-12 同判）；无可推导→ref='' 渲染层降级不崩。渲染层三处防御：backup/index.vue sourceCell 与 RunDetailDrawer 源明细/BackupJobDrawer 编辑概要的 `BACKUP_SOURCE_TYPE[s.type]` 直接索引全部改 `dictItem` 兜底（⚠️A14 口径，未知 type 原值灰字不崩）；RunDetailDrawer `sourceFailed` 加 ref!=='' 守卫（includes('') 恒真会误标全部源失败）；ref==='' 时不渲染引用文本。`BackupSourceRef.type` 放宽为 `BackupSourceType \| string`（docker ContainerStatus|string 透传先例），`ref` 保持契约 `string`（归一器保证输出形状） | 采用报告修复方向 a 的前端份额（后端投影层补 ref 为主路径）；修复口径=account-auth Gate 2 E1/E2 同判（api 层归一、契约类型单一来源、后端修复后无感直通）而非改契约或弱化类型。「双侧补形状断言」前端侧落 cron.test.ts 新增 10 用例：契约直通/现状推导（site_dir·database·缺库）/空串降级/非对象剔除/输出恒契约形状/两个管线（list items[].sourceSummary、detail sources）逐项归一+契约 ref 直通不改写/坏形状不吞错。根因修复=后端 dto.go SourceItemDTO 补 ref 投影（超出前端范围，报告已列后端侧待办与 module_test.go 断言缺口） |

自验（2026-11-15，cwd=frontend/）：`npm run build`（vue-tsc --noEmit && vite build）exit 0（27.30s）；构建后二次 `npm run typecheck` exit 0；`npx vitest run` 10 files / 115 tests 全过（105 既有无回归 + E1 归一器 10 新增）；grep 复扫——F-01~F-05 全 0 命中、`BACKUP_SOURCE_TYPE[` 直接索引全仓 0 残留（全部走 dictItem）。W1/W2/W3 维持报告原判（后端覆盖率与组件挂载测试基础设施非本轮 FAIL 修复项；views/store 挂载测试沿 monitoring/service-mgmt W 先例待后续裁定）。

# 增量轮 · v0.1.2 用户反馈 + Gate 1 改判 DNS-01（2026-11-16/17，/pipeline session 轨）

> 设计输入：PageSpec site-mgmt v1.2×2（⚠️A18/A19/A20）/ env-mgmt v1.0×2 / panel-core v1.2×2 /
> account-auth v1.3+v1.1；数据源 api-site-mgmt.md v2.0 · api-env-mgmt.md v1.0 · api-panel-core.md v2.2（D-P18）。

- [x] T-SM1 · site-mgmt 前端增量（v1.1+v1.2 合并落码）
  - types/site.ts：SiteListItem 聚合化（去顶层 siteId、+blocks: SiteBlock[]，D-17）+ CertRequestRequest 扩展（siteId/domains 二选一+dnsCredentialId）+ DnsCredential/Replace/Renew 全套类型 + CERT_ERROR_TEXT 扩 CERT_008~010；enums.ts：AcmeChallenge +dns_01、DNS_PROVIDER 新增
  - api/site.ts：23 Action（+certRenew/certReplace/dnsCred×3；certRequest 签名对象化）
  - 站点列表（views/site-mgmt/index.vue）：聚合行（监听列「80 / 443 ssl」合并）+ 行展开 blocks 块清单（siteId/listen/enabled/证书引用级明细）+ 组级启停（blocks 顺序逐块、已处目标态跳过、失败停止并刷新）+ rowKey 聚合键；SiteDetailDrawer/CertDeployModal/BackupJobDrawer 寻址改 blocks[0].siteId；stores/site.ts findBySiteId 块级查找
  - 证书页：申请弹窗重写（三挑战方式文案〔文件验证 HTTP-01/TLS-ALPN/DNS-01〕+域名来源二选一切换〔手动模式含 *.domain 泛域名〕+DNS 凭证选择〔空清单引导配置、CERT_009 弹窗引导〕）；operList「更新证书」（acme 行 valid/expiring_soon/renew_failed，取代续期重试渲染；D-18 失败分流文案「原证书仍有效」）+「替换证书文件」（imported 行，CertReplaceModal：PEM 粘贴+备份/私钥范围声明+新元数据呈现）；工具栏「DNS 凭证」（DnsCredentialsModal：列表零回显+provider 驱动新建表单〔cloudflare/aliyun/dnspod〕+删除引用警示 I-C01）
- [x] T-ENV2 · env-mgmt 前端新模块
  - types/env.ts（通道/条目/任务/服务操作类型；children:null 契约注释）+ api/env.ts（10 Action）+ stores/env.ts（全景成败流〔listError 如实〕+任务清单筛选+详情 5s 轮询终态即停 onUnmounted 由页面清理）
  - views/env-mgmt/index.vue：页头通道状态条+六环境卡片网格（实况 tag〔installed=null「探测不可用」〕+在途标记查看引导+操作区 admin 专属：安装/升级 NPopconfirm、卸载强确认 NModal 两段警示+输入环境名锁定、服务启停自启组）+ EnvTaskDrawer（历史筛选列表+详情输出尾部等宽块）
  - 路由 /env（meta.title）+ vite dev proxy /env + MainLayout MENU_ICONS env 映射
- [x] T-IA2 · B 批 IA（v0.1.2 反馈 5-7 项，D-P18/方案一）
  - MainLayout：顶栏标题=当前页面标题（headerTitle computed：路由 meta.title 优先→菜单树含分组遍历兜底→「斗拱面板」；替换硬编码「账户与安全基座」——反馈第 5 项根因 MainLayout.vue:182）；children 分组渲染既有支持（toMenuOptions 递归+children:null 双防护）直接生效
  - settings/index.vue：四分组重排（常规[语言]/网络与访问[监听·入口·域名]/安全与证书[HTTPS·证书·会话]/关于[更新检查]，Card 语义编号不变+分组标题视觉层级——PageSpec panel-core v1.2）
  - account-auth 三列表页 loadError 错误态（user-list/session-list/audit-log-list：NAlert+重试，不呈现裸空表格——反馈第 5 项「正文空白」修复，PageSpec account-auth v1.3 规范）
- [x] 门禁自验（2026-11-17 实跑 cwd=frontend/）：vue-tsc --noEmit=0、npm run build exit 0（30.0s）、vitest 15 文件 174 用例全过（+env store 4 用例：loadList 成败流/taskFilter/clearTaskDetail 幂等）、产物同步 backend/web/dist（make web）+ 同步后 go build ./...=0；F-01~F-05 禁项 grep 0 命中（any/原生 alert·confirm/views 层 fetch/定时器不配对清理/Options API——增量代码未引入）

# 增量轮 · W1 实机冒烟缺陷修复（2026-11-17，/pipeline session 轨）

> 设计输入：PageSpec env-mgmt admin/readonly v1.1；数据源 api-env-mgmt.md v1.1（⚠️A8/A9/A10，用户确认 W1~W5）。

- [x] T-ENV4 · env-mgmt 前端增量（三态+MongoDB）
  - enums.ts：EnvType +mongodb（七值，S8 四方同值）+ ENV_TYPE 字典项
  - views/env-mgmt/index.vue：installedText 三态精确化（null 且通道可用=「探测不可用」橙/通道不可用=「未探测」灰——页头横幅承载通道语义不重复/false=「未安装」）；安装·升级 Popconfirm 文案补第三方源代配说明（MongoDB/RHEL 系 Docker CE）；卸载警示二补 MongoDB、提交文案补源文件保留说明；七条目网格注释与页头注释 v1.1
  - stores/__tests__/env.test.ts：载荷升七条目（+mongodb 卡片）与全名 unitName（nginx.service/mongod.service——D-13 回归）
- [x] 门禁自验（2026-11-17 实跑 cwd=frontend/）：npm run build exit 0（vue-tsc --noEmit + vite build 24.8s）、npx vitest run 16 文件 174 用例全过（env store 载荷升七条目含 mongodb 卡片与全名 unitName）、产物同步 backend/web/dist（make web）+ 同步后 go build ./...=0；F-01~F-05 禁项 grep 0 命中（改动面未引入 any/原生 alert·confirm/views 层 fetch/Options API）
