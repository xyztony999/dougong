# PageSpec · 防火墙（firewall） · 管理员（admin）

| 属性 | 值 |
|------|-----|
| 版本 | v1.0 |
| 状态 | 稳定 |
| 最后更新 | 2026-11-14 |
| 用户类型 | admin（readonly 视角为本文档子集，差异见 ../readonly/firewall/firewall.md） |
| 唯一数据源 | 02-detailed/api/api-firewall.md v1.0 |
| 组件体系 | Vue 3 + Naive UI（NDataTable/NSelect/NInput/NButton/NTag/NPopconfirm/NModal/NAlert/NEmpty/NTabs/NForm/NFormItem；无激活栈 profile，不使用 vue3-engine 栈 Scm* 组件） |
| 确认方式 | headless run：7 项交互确认依据 D3 契约（api-firewall.md D-01~D-10 + I-01~I-08）+ ADR-018 + FL v1.0 推导，标注于各节 |

## 第 1 节 模块元信息

| 项 | 值 | 确认依据 |
|----|-----|---------|
| 模块编码 | firewall（域前缀 /firewall） | api 3.1 |
| 业务类型 | **工具类**（分册 2.1 三型之外的诚实标注：无聚合根、无 MQ——D2/D3 判定；投影列表 + 2 写接口，无状态机无审批） | api 1.4/4.1.2；domain-model.md firewall 域 |
| 页面类型 | **hybrid**（列表页 Tab 切换：端口规则 / 监听端口对照两 Tab → 分册 2.2「列表页有 Tab 切换」触发 hybrid，layout: 'tab'；无左树右表/图表/拖拽，不触发 code） | 分册 2.2；FL 2.1.1.2/2.1.1.3 两菜单同页承载（idea.md §5.2 单一 views/firewall 目录） |
| UI 框架 | Naive UI（NTabs/NDataTable/NSelect/NInput/NForm/NTag/NPopconfirm/NModal） | 栈版本表 |
| 状态管理 | Pinia：firewallStore（backend/status/ruleItems/portItems/generatedAt/listParams/inFlight/addDialog 态）；枚举常量 store（与 account-auth/service-mgmt 共用 enums.ts） | 栈版本表 |
| 错误处理 | 统一拦截器按 api 3.4 code 分流：AUTH_001/002→跳登录（全局拦截器）、AUTH_003→CSRF 失败提示（全局）、AUTH_008→「权限不足：只读角色禁止防火墙规则变更」（全局边界拒绝，正常 UI 无入口不触发）、FW_001/FW_002→添加 Dialog 内对应字段错误提示（正常 UI 由控件与校验约束不可达，api D-05/R-14）、FW_003→warning「该规则已不存在（可能已在面板外删除）」+ 自动重拉规则清单（api D-07）、FW_004→正常 UI 不可达（动作下拉仅列 supportedActions 权威面，api D-01/I-04）兜底「参数非法」提示、FW_005→危险确认 Modal（第 5 节两段式，api D-06/I-05）、FW_006→warning「等价规则已存在，未重复添加」（api D-07）、FW_007→NModal 部分生效说明（api D-08）、FW_008→error「后端命令执行失败」+ data.reason、FW_009→正常流程不可达（status 先行判定引导态，第 4.0 节）兜底跳引导态、FW_010→页面级 NAlert error「防火墙后端不可用（如 firewalld 服务停止）」+「重试」按钮，数据区不渲染（api I-01 不显示过期数据） | api 3.4 + D-01/D-05~D-08/I-01 |

## 第 2 节 Action 定义（与 API 一一对应）

| Action | 方法 | 路径 | 页面 | 权限 | CSRF |
|--------|------|------|------|------|------|
| backendStatus | GET | /firewall/backend/status | firewall·页头状态 Card（含引导态判定） | admin+readonly | —（GET） |
| ruleList | GET | /firewall/rule/list | firewall·Tab 端口规则 | admin+readonly | —（GET） |
| portList | GET | /firewall/port/list | firewall·Tab 端口对照 | admin+readonly | —（GET） |
| ruleCreate | POST | /firewall/rule/create | firewall·添加规则 Dialog | admin | ✓ |
| ruleDelete | POST | /firewall/rule/delete | firewall·规则表操作列 | admin | ✓ |

> S7 一致性：5 Action 与 api-firewall.md 5 接口一一对应（一接口一 Action，无未覆盖接口、无虚构接口）。CSRF Token 由全局拦截器统一注入 `X-CSRF-Token`（account-auth R-02 机制）。

**刷新纪律**（确认项 2/7 交叉，api D-01「按需拉取」契约的前端落法）：

| 触发 | 行为 |
|------|------|
| 进入页面 | backendStatus 先行 → backend≠none 时激活 Tab 懒加载对应清单（首次切 Tab 亦然） |
| 点击「刷新」按钮（页头/Tab 内） | 重拉 backendStatus / 当前 Tab 清单（loading 态） |
| ruleCreate/ruleDelete 成功 | 重拉 ruleList（规则集合已变）；portList 标记**过期**——Tab 切回端口对照时重拉（openState 为规则×监听读取时派生，api D-04）；backendStatus 不重拉（规则操作不改变后端识别结论） |
| 无自动轮询 | 不设定时轮询（monitoring D-01 / service-mgmt D-03 同口径；页面刷新节奏=手动+操作驱动，PRD 无自动刷新需求不虚构） |
| `document.hidden` | 无定时器需要暂停；恢复可见不自动重拉（保持按需纪律） |
| 页面卸载 | 清理 Dialog/inFlight/Tab 过期标记 |

## 第 3 节 字典 Key 清单

| Key | 取值（=01-solution/04-business-enum.md 定稿） | 用途 |
|-----|--------------------------------------------|------|
| firewall_backend | ufw / firewalld / none | 页头后端类型 Tag + 引导态判定（none） |
| firewall_rule_action | allow / deny / reject / limit | 动作筛选下拉（多选）+ 动作列 dict + colorTag + 添加 Dialog 动作下拉（options=响应 supportedActions 权威面，api D-01/I-04） |
| firewall_rule_protocol | tcp / udp | 协议筛选下拉 + 协议列 dict + 添加 Dialog 协议下拉 |
| listen_port_state | allowed_by_rule / no_allow_rule | 对照表开放状态列 dict + colorTag |
| panel_role | admin / readonly | 菜单可见性判定（authStore 复用；本模块 readonly 为子集视角，差异见 readonly 文档） |

**数据源规范偏差声明**（同 account-auth/panel-core/monitoring/service-mgmt 先例）：不实现 `POST /{domain}/dict/batch`（分册 3.6）——枚举全部固定值，字典由前端 TS 常量提供（enums.ts 单一事实源，共用同一文件）。此偏差为设计决策，待 Gate 1 复核。

**状态颜色方案**（分册状态颜色语义映射）：

| 值 | 颜色 | 语义依据 |
|----|------|---------|
| backend=ufw / firewalld | blue | 后端识别为中性信息（分册「进行中」蓝=激活态）；与 active Tag 并列呈现 |
| action=allow | green | 放行（分册「已通过/成功」语义） |
| action=deny | red | 阻断丢弃（分册「已拒绝」语义） |
| action=reject | orange | 明确拒绝（分册「异常」语义，与 deny 区分为可感知拒绝） |
| action=limit | blue | 限速放行（分册「进行中」蓝） |
| openState=allowed_by_rule | green | 已被规则放行 |
| openState=no_allow_rule | orange | 无放行规则（分册「异常」橙——风险提示态；external=true 行加浅橙底高亮，第 4.4 节） |
| active=true/false（后端运行状态） | green / gray | 运行中 / 未启用（透传属性，api 1.2 OUT-⑤ 无开关） |

## 第 4 节 列表页（核心页，/firewall，hybrid · Tab 布局）

**权限**：admin 全功能；readonly 为子集（无添加按钮、无操作列，见 readonly 文档）。

### 4.0 页头：后端状态 Card 与引导态（backendStatus，确认项 1）

页头 Card 恒为页面第一区块（两 Tab 之上）：

| 元素 | 呈现 |
|------|------|
| 后端类型 | NTag（firewall_backend dict 颜色） |
| 运行状态 | NTag（active=true→green「运行中」/ false→gray「未启用」，透传属性） |
| 版本 | text（version=null →「—」，不虚构） |
| 动作支持面 | 灰字说明「当前后端支持动作：{supportedActions 中文逗号串}」（api D-01 权威面；firewalld 仅 allow 时明示） |
| 刷新 | NButton（loading 防抖，重拉 backendStatus + 当前 Tab 清单） |
| 数据生成于 | 灰字 {generatedAt 本地时区}（api generatedAt 契约） |

**引导态**（backend=none，api FW_009/I-01「无后端≠无规则」契约的前端落法）：页头下方整页降级为 NEmpty 引导视图——「未检测到受支持的防火墙后端（ufw / firewalld）。请在服务器上安装并启用其一后刷新重试。」**不渲染两 Tab 与任何规则/对照内容、不呈现空规则列表**（FL S-42-02；提示页不提供代为安装入口，PRD FR-02）；保留页头刷新按钮（安装后重试入口）。FW_010（后端已识别但不可用）→ 页面级 NAlert error + 重试（4.2 同款落法），同样不渲染过期数据。

### 4.1 Tab 1 工具条（查询条件，确认项 2；参数变更即触发 ruleList）

| 控件 | 组件 | 取值与校验 | 查询方式 |
|------|------|-----------|---------|
| 动作筛选 | NSelect multiple（clearable） | firewall_rule_action 四值，空=全部 | 多选值逗号拼接 query `action`（api：逗号分隔多值；任一值非法→FW_001，正常 UI 下拉不可达） |
| 协议筛选 | NSelect（clearable，单选） | firewall_rule_protocol 两值，空=全部 | query `protocol` |
| 添加规则 | NButton（primary） | 打开添加 Dialog（第 5 节） | — |
| 刷新 | NButton（loading 防抖） | 以当前参数重拉 | — |

表格上方灰字「数据生成于 {generatedAt}」+「共 N 条规则」。

### 4.2 Tab 1 表格列（端口规则，NDataTable，确认项 3）

| 列 | 类型 | 宽度 | 说明 |
|----|------|------|------|
| 序号 | index | 60 | — |
| 端口 | text | 140 | items[].port（"80" 或 "8080-8090" 显示形，api D-05） |
| 协议 | dict(firewall_rule_protocol) | 90 | items[].protocol |
| 方向 | text | 90 | items[].direction（恒 "ingress" 常量列，api 4.1.1 单值维度呈现） |
| 来源限制 | text | 170 | items[].source（null → 「任意」灰字；CIDR 原值） |
| 动作 | dict(firewall_rule_action) + colorTag | 110 | items[].action（第 3 节颜色方案） |
| 面板端口 | Tag | 100 | items[].isPanelPort=true → NTag orange「面板端口」；false → 空白（api D-06/I-05 标注，删除涉险预警线索） |
| 后端原始语义 | text | 260 | items[].rawSpec 原值透传（与后端自身工具输出互认，api 4.1.1；等宽字体呈现） |
| 操作 | action | 120 fixed:right | 见 4.3（readonly 视角整列不渲染） |

无分页（全量返回 api D-02）、无排序控件（服务端端口数值升序）；空结果 → NEmpty「后端无端口规则」（后端可用且无规则为常态，与引导态语义区分，api N2）；加载中 → 表格骨架。

### 4.3 Tab 1 操作列（operList，确认项 4）

| 按钮 | 可见性条件 | 二次确认 | 依据 |
|------|-----------|---------|------|
| 删除 | 恒显（全部规则行） | NPopconfirm「确认删除该规则？删除后即时生效。」；**若 isPanelPort=true，Popconfirm 文案加强**「该规则直接命中面板自身端口（{port}），删除后可能导致面板失去访问！」（预警线索前置）；提交后服务端 FW_005 权威兜底（第 5.3 节两段式） | api D-06/I-05；FL S-44-02 |

> 添加入口在工具栏（4.1）不在操作列（逐行添加无意义）。删除为唯一行级操作。

**在途互斥与结果反馈**（确认项 5 交叉，api I-05/I-07 前端落法）：

| 环节 | 行为 |
|------|------|
| 点击删除 | 行级 inFlight 登记——该行按钮置 loading/禁用 + 添加按钮禁用（防重复提交） |
| 成功（code=0） | message.success「规则已删除」+ 重拉 ruleList + portList 标记过期（第 2 节纪律） |
| FW_003 | warning「该规则已不存在（可能已在面板外删除）」+ 自动重拉 ruleList（实况同步，api D-07） |
| FW_005 | 危险确认 Modal（第 5.3 节两段式——服务端权威判定触发） |
| FW_007/FW_008 | 第 5.4 节结果反馈（同 create） |

### 4.4 Tab 2 表格列（监听端口对照，NDataTable）

工具条仅「刷新」按钮（无筛选参数，api port/list 无 query；对照为全量视图）。表格上方灰字「数据生成于 {generatedAt}」+「共 N 个监听端口」。

| 列 | 类型 | 宽度 | 说明 |
|----|------|------|------|
| 序号 | index | 60 | — |
| 协议 | text | 100 | items[].proto 原值透传（tcp/tcp6/udp/udp6，开放集合不枚举化——api D-04 同族匹配口径，不翻译） |
| 监听地址 | text | 170 | items[].addr 原样 |
| 端口 | text | 100 | items[].port |
| 监听进程 | text | 200 | process=null → 「—」；否则 `{name} ({pid})`（取不到不虚构，api 4.1.1） |
| 对外 | Tag | 90 | external=true → NTag blue「对外」；false → NTag gray「仅本机」（loopback 维度，api D-04） |
| 开放状态 | dict(listen_port_state) + colorTag | 140 | items[].openState（第 3 节颜色方案）；**openState=no_allow_rule ∧ external=true 的行加浅橙底高亮**——风险敞口重点提示对象（api D-04/PRD FR-04），表头上方灰字图例「橙底行=对外监听且无放行规则」 |

无分页、无排序控件；空结果 → NEmpty「当前无监听端口」（罕见态，如实呈现）；加载中 → 骨架；监听读取部分失败条目 process=null 呈现「—」（不虚构，api 4.1.1）。

## 第 5 节 表单页（添加规则 Dialog，确认项 5）

**形态依据**：添加规则 4 字段（port/protocol/source/action，< 5 字段 → Dialog，分册 3.2 表单类型判定）；无独立表单路由页。入口：Tab 1 工具栏「添加规则」按钮 → NModal Dialog（宽 520px，标题「添加端口规则」）。

### 5.1 表单字段（NForm，api create req 推断）

| 字段 | 组件 | 取值与校验（前端预校验 + 服务端 R-14 兜底 FW_001/002） |
|------|------|------------------------------------------------------|
| 端口 port | NInput | 占位「80 或 8080-8090」；正则 `^([1-9]\d{0,4})(-([1-9]\d{0,4}))?$` + 1-65535 + N≤M（api D-05/FW_001） |
| 协议 protocol | NSelect | firewall_rule_protocol 两值（tcp 默认） |
| 来源限制 source | NInput（可空） | 空=任意来源（占位「留空=任意来源」）；非空须为合法 IPv4/IPv6 CIDR（api D-05/FW_002） |
| 动作 action | NSelect | options=backendStatus.supportedActions 权威面（api D-01/I-04：firewalld 环境仅列 allow——正常 UI 不可达 FW_004）；下拉项附中文释义（allow 放行 / deny 丢弃 / reject 拒绝 / limit 限速放行） |

direction 不入表单（固定入站单值维度，api D-05）；表单底部灰字说明「规则经当前后端即时生效（firewalld 环境同时写入永久配置与运行态）」（api D-08 双写口径的用户可知会项）。按钮：**添加 / 取消**（工具类无审批语义，分册按钮组合参照 master_data 的 保存/取消 形态）。

### 5.2 提交行为

校验通过 → ruleCreate（POST，CSRF）→ 成功 message.success「规则已添加」+ 关闭 Dialog + 重拉 ruleList + portList 标记过期（第 2 节纪律）。

### 5.3 FW_005 两段式危险确认（api D-06/I-05 的前端落法，面板端口保护硬项）

无论前端是否预判（isPanelPort 标注为预警线索，4.2/4.3），**权威判定以服务端为准**：create（action∈deny/reject 命中面板端口）或 delete（allow 族规则命中面板端口）未携 confirmDanger → 服务端返回 FW_005 → 前端弹出危险确认 Modal：

| 元素 | 呈现 |
|------|------|
| 标题/图标 | 「危险操作：面板端口保护」（NModal 警示态） |
| 警示文案 | 「此操作可能导致面板失去访问！确认执行前请确保保有服务器本地/SSH 救援通道。」（api FW_005 data.hint；M7 明示项） |
| 命中详情 | data.panelPort（面板端口）× data.rulePort（规则端口）× data.action（Tag 渲染） |
| 按钮 | 「确认执行」（error 态，点击 → 原 req 追加 confirmDanger:true 重新提交）/「取消」 |

confirmDanger 每次涉险操作独立要求（不记忆，api 幂等语义）；确认后执行结果按 5.4 反馈。FW_005 拒绝未发生变更不记审计（api I-07），前端停留 Modal 等待用户决策。

### 5.4 结果反馈（FW_006/007/008，api D-07/D-08）

| code | 反馈 |
|------|------|
| FW_006 | warning「等价规则已存在，未重复添加」+ 关闭 Dialog（api D-07 不静默） |
| FW_007 | NModal 说明框：「永久配置已写入，运行态应用失败——规则将在防火墙服务重启后生效。」+ data 两层状态（api D-08：不回滚不虚构）；关闭后重拉 ruleList |
| FW_008 | error「后端命令执行失败：{data.reason}」（Dialog 保持打开供修正重试） |

## 第 6 节 详情页

**无详情页**（确认项 6 判定 N/A）：api-firewall.md 无 detail 类接口（全册 3 GET + 2 POST，规则全字段已在列表行内呈现——含 rawSpec 原始语义列即「与后端互认」的详情承载，api 4.1.1）；无 Drawer/独立详情路由、无子表 Card、无审批记录 Card、无操作日志 Card（操作历史经 account-auth 审计日志查询，PRD N4）。

## 第 7 节 路由定义与框架行为

| 路由 | 页面 | meta |
|------|------|------|
| /firewall | firewall（防火墙页：页头状态 Card + 引导态 + 双 Tab + 添加 Dialog） | `{requiresAuth:true}` |

**守卫**：复用全局守卫（initCheck → sessionCurrent → roles）；本模块无新增守卫。

**框架级边界**（复用 panel-core 框架行为，见 ../admin/panel-core/panel-core.md 第 7 节）：菜单渲染/安全入口前缀透明/顶栏均为 panel-core 宿主职责；本模块 1 个路由经模块页面挂载点接入（panel-core S-10），菜单项由 ModuleMenuRegistry 登记、服务端按角色下发（防火墙菜单对 admin/readonly 均可见——readonly 可进入查看，操作面为其不可见，FL/ADR-018）。

## 第 8 节 全局一致性

| # | 检查 | 结果 |
|---|------|------|
| 1 | Action ↔ api-firewall.md 接口一一对应（S7） | ✅ 5/5 接口全映射，无虚构、无遗漏 |
| 2 | 字典 Key ↔ 04-business-enum.md 定稿枚举一一对应（S8） | ✅ 4/4（firewall_backend/firewall_rule_action/firewall_rule_protocol/listen_port_state）+ panel_role 复用；取值逐字一致 |
| 3 | 写 Action CSRF 注入完整（2 POST 全部 ✓） | ✅ 第 2 节 + 全局拦截器 |
| 4 | 动作支持面权威来源=服务端 supportedActions（动作下拉照此渲染），前端不自行推断 | ✅ 5.1 节（api D-01/I-04；FW_004 正常 UI 不可达已声明） |
| 5 | 面板端口保护两段式：服务端 FW_005 权威兜底 + 前端危险 Modal（confirmDanger 重提交）；前端 isPanelPort 标注仅为预警线索 | ✅ 4.2/4.3/5.3 节（api D-06/I-05，M7 硬项） |
| 6 | 刷新纪律符合 D-01 按需拉取：status 先行/Tab 懒加载/写后重拉规则+对照标记过期/无轮询 | ✅ 第 2 节纪律表 |
| 7 | FW_001~010 + AUTH_001/002/003/008 错误文案与 api 3.4 一一对应 | ✅ 第 1 节错误处理 + 4.0/4.3/5.4 节 |
| 8 | 引导态落法：backend=none 整页降级引导、不呈现空规则列表（无后端≠无规则）；FW_009 正常流程不可达兜底 | ✅ 4.0 节（api FW_009/I-01，PRD FR-02） |
| 9 | ADR-018：readonly 为子集视角（无添加按钮、操作列整列不渲染），2 个 ✗ 项逐项负空间 | ✅ readonly 文档对应 |
| 10 | 无 📎 Scm* 组件；Naive UI 为栈版本表选型 | ✅ 头部组件体系 |
| 11 | 无详情页诚实性声明（无 detail 接口，rawSpec 列内承载后端互认语义） | ✅ 第 6 节 |
| 12 | 时间本地时区展示（generatedAt）、原值透传不翻译（rawSpec/proto/addr；version=null 呈「—」） | ✅ 4.0/4.2/4.4 节 |
| 13 | FW_007 部分生效如实反馈（不虚构全成功、不隐藏 Dialog 错误） | ✅ 5.4 节（api D-08/I-06） |

## 第 9 节 文档版本与变更记录

| 版本 | 日期 | 变更类型 | 说明 |
|------|------|---------|------|
| v1.0 | 2026-11-14 | initial | 首次生成：1 hybrid 页面（页头后端状态 Card + 引导态 + 端口规则/端口对照双 Tab + 添加规则 Dialog）、5 Action（3 读 + 2 写）、4 字典 Key + panel_role 复用、0 独立表单页（4 字段 Dialog）/0 详情页；面板端口保护两段式（FW_005 服务端权威 + 危险 Modal）、动作支持面权威渲染（supportedActions）、刷新纪律（status 先行/Tab 懒加载/写后对照标记过期）为本阶段核心交互定义 |

> FX 质量清单说明：FX-001~FX-141 逐条清单文件未随引擎 0.5.2 payload 发布（monitoring/panel-core/account-auth/service-mgmt 先例同口径复核），本节按分册 9 节结构逐节落实，并以第 8 节 13 项全局一致性自查替代可机械执行部分；全量 141 项留待 Gate 1 validation 执行。
