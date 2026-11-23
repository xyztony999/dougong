# Gate 2 报告 · 宝塔式「网站 / 容器」二分重组（ADR-033）（2026-11-23）

> 判定：**WARN（CRITICAL 0，沿全仓先例）**——B/F 落码完成，编译门控全绿；入口可达性、真实 Docker / PM2 端到端、真实浏览器逐个点击、带数据升级测试均通过。核对过程中发现并修复了 5 个问题，其中「二次确认形同虚设」是容器管理上线以来就存在的安全类缺陷，「分栏实例复用」会让本轮的新功能在主路径上不可用。WARN 为只能在目标服务器上验证的事项与流程性事项，待用户经主会话确认。
>
> 依据：baota-restructure-design-2026-11-23.md v1.2、d0-clarification-baota-restructure-2026-11-23.md（六项拍板）、gate1-baota-restructure-2026-11-23.md。

## 一、编译门控（最终代码 46dd540，全绿）

| # | 门控 | 结果 |
|---|------|------|
| 1 | go build ./... | ✅ exit 0 |
| 2 | go vet ./... | ✅ exit 0 |
| 3 | go test ./... -race -count=1 | ✅ 55 包 0 FAIL（本次未出现 appmgmt 的 Windows -race 偶发超时） |
| 4 | vue-tsc --noEmit | ✅ exit 0 |
| 5 | vite build | ✅ exit 0 |
| 6 | npx vitest run | ✅ 26 文件 361 用例全过（v0.3.1 为 24 文件 342） |
| 7 | gofmt -l | ✅ 0 文件 |
| 8 | GOOS=linux 交叉编译 amd64 / arm64 | ✅ 均 exit 0 |
| — | dist 同步 backend/web/dist 后 go build 复验 | ✅ exit 0（内嵌产物与 4/5 同源） |

## 二、交付落点

| 交付 | 落点 |
|------|------|
| 网站管理页三分栏（网站 / Python 项目 / Node 项目），分栏写入 ?tab | views/site-mgmt/SitesHub.vue；router（/sites → SitesHub；/app 重定向 /sites?tab=python）；监控页资源条「项目」格 |
| 项目面板按运行时各挂一份；Node 为 PM2 进程视角 | views/app-mgmt/AppProjectsPane.vue（prop runtime）；stores/app.ts（按运行时分仓库） |
| Node 项目后端：创建 / 启停 / 重启 / 开机自启 / 重装依赖 / 日志 / 参数 / 删除 | service/appmgmt/node.go + adapter/pmx（PM2 网关与内嵌启动器）+ adapter/appx（node / npm 解析） |
| 容器运行形态下线（APP_014） | service/appmgmt（createContainerized、container_form.go、container_ops.go 已移除） |
| env-mgmt：Node 环境连带安装 PM2 并注册开机自启；PM2 就绪度探测 | adapter/envpack（ProbePm2Available / NpmInstallGlobal / Pm2Startup）；service/envmgmt/readiness.go、worker.go |
| 迁移 018（app 表重建：runtime 列 + 启动方式五值 CHECK） | backend/internal/store/migrations/018_app_runtime.sql |
| 容器管理「应用」页签：清单 / 创建 / 启停重启 / 日志 / 建站 / 删除 | views/docker/components/AppsTab.vue + stores/docker-apps.ts；后端 service/dockermgmt/app.go、api/dockermgmt/app.go |
| Compose 生命周期：零容器项目可见、可再启动；新增「删除项目」 | service/dockermgmt/panelcompose.go、create.go；views/docker/index.vue |
| 错误码 APP_014~016、DK_028~032；审计动作 5 项 | errcode/、consts.go |
| 应用管理独立菜单删除 | cmd/dougong/main.go（不再注册 app 菜单；dockerSvc 注入建站适配器） |

## 三、入口可达性硬性检查（缺任一环即 FAIL）

规则沿 gate2-docker-create-2026-11-22.md 第八节：每个交付的可见动作，核验 触发按钮 → 打开函数 / 置真 → 弹窗 → 提交调用前端 Action → 路由 → HTTP 处理器 → 服务方法 全链连通，给出 file:line。行号取自最终代码（46dd540）。**结果：全部连通，PASS。** 与 v0.3.0 那次的区别是：这次每个动作都在真实页面里点过（第四节），不再只有源码证据。

### 3.1 网站管理与项目面板（前端路径均在 frontend/src 下，后端在 backend/internal 下）

| 动作 | 触发 | 弹窗 / 打开 | 提交 → 前端 Action | 路由（写组：admin + CSRF） | 处理器 | 服务方法 |
|------|------|------------|--------------------|---------------------------|--------|----------|
| 创建 Python / Node 项目 | views/app-mgmt/AppProjectsPane.vue:460（表头）、:473（空态） | `createShow = true` → NModal :479 | submitCreate :273 → api/app.ts:31，运行时 :278 | /app/app/create — api/appmgmt/module.go:50 | module.go:75 | service/appmgmt/service.go:232 Create |
| 启动 / 停止 / 重启 | AppProjectsPane.vue:186~189（停止、重启经确认；未运行时显示启动） | — | :221 → api/app.ts:79 | /app/service/{start,stop,restart} — module.go:55~57 | module.go:126 | service.go:573 ServiceOp（Node 分支 node.go:238） |
| 开机自启开关 | AppProjectsPane.vue:141（切换前确认） | — | :231 → api/app.ts:79 | /app/service/{enable,disable} — module.go:58~59 | module.go:126 | service.go:573 |
| 重装依赖 | AppProjectsPane.vue:192（经确认） | — | :241 → api/app.ts:51 | /app/app/deps — module.go:53 | module.go:116 | service.go:410 Deps |
| 参数 | AppProjectsPane.vue:196 → 运维抽屉「参数」页签 :541 | NDrawer :517 | saveParams :349 → api/app.ts:46 | /app/app/params — module.go:52 | module.go:106 | service.go:482 ParamsSave |
| 日志 | AppProjectsPane.vue:197 → 抽屉「日志」页签 :555 | NDrawer :517 | loadLog :365 → api/app.ts:84 | /app/log（读组，admin 专属） | module.go:173 | service.go:684 LogTail（Node 分支 node.go:323） |
| 建站 | AppProjectsPane.vue:199 → 抽屉概要 :536 | NDrawer :517 | submitSite :383 → api/app.ts:89 | /app/site/link — module.go:60 | module.go:185 | service.go（SiteLink） |
| 删除 | AppProjectsPane.vue:201 → 强确认弹窗 :505 | NModal :505（输入应用名才可确认） | submitDelete :304 → api/app.ts:36 | /app/app/delete — module.go:51 | module.go:85 | service.go（Delete） |
| 监控页「项目」格 | views/monitoring/components/ResourceBar.vue:40 | 路由跳转 | — | /sites?tab=python — router/index.ts:144、:146 | — | — |
| 旧地址 /app | 书签 | 重定向 | — | router/index.ts:282~284 → /sites?tab=python | — | — |

### 3.2 容器管理「应用」页签与 Compose 页签

| 动作 | 触发 | 弹窗 / 打开 | 提交 → 前端 Action | 路由 | 处理器 | 服务方法 |
|------|------|------------|--------------------|------|--------|----------|
| 「应用」页签 | views/docker/index.vue:1117（页签）→ :1118 `<AppsTab />` | — | — | — | — | — |
| 创建应用 | views/docker/components/AppsTab.vue:329 | openCreate → n-modal :355 | submitCreate :104 → api/docker.ts:312 | /docker/app/create — api/dockermgmt/routes.go:101 | app.go:36 | service/dockermgmt/app.go:326 AppCreate |
| 启动 | AppsTab.vue:289 | — | execService :134 → api/docker.ts:317 | /docker/app/service — routes.go:102 | app.go:55 | app.go:361 AppService |
| 停止 / 重启 | AppsTab.vue:293、:297（经确认） | — | 同上 | 同上 | 同上 | 同上 |
| 日志 | AppsTab.vue:246（名称）、:304 | 日志弹窗 | openLogs :164 → api/docker.ts:322 | /docker/app/logs — routes.go:70（读组） | app.go:16 | app.go:390 AppLogs |
| 建站 | AppsTab.vue:307 | 建站弹窗 | :224 → api/docker.ts:332 | /docker/app/site — routes.go:104 | app.go:94 | app.go:442 AppSite |
| 删除 | AppsTab.vue:310 | 强确认弹窗（输入应用名） | :190 → api/docker.ts:327 | /docker/app/delete — routes.go:103 | app.go:74 | app.go:413 AppDelete |
| Compose 删除项目 | views/docker/index.vue:584（仅作确认框触发器，不绑 onClick）→ :589 onPositiveClick | NPopconfirm | execComposeDelete :267 → :271 → api/docker.ts:337 | /docker/compose/delete — routes.go:105 | app.go:114 | app.go:481 ComposeDelete |
| Compose 零容器项目可见、可再启动 | Compose 页签列表（状态「未运行」）；启动沿既有 up | — | — | /docker/compose/list、/compose/up（既有） | 既有 | panelcompose.go:164 listProjects |
| 容器行停止 / 重启 / 暂停 / 删除、Compose 行 up / restart / down 经确认后执行 | views/docker/index.vue:457（容器）、:539~558（Compose） | NPopconfirm | onPositiveClick → execContainerAction / execComposeAction | 既有路由 | 既有 | 既有 |

源码把关：`views/site-mgmt/__tests__/restructure-entry.test.ts` 41 用例（逐动作断言上述链路，并直接读后端 Go 源码核对每个前端调用的地址都已注册）；`__tests__/confirm-trigger.test.ts` 全仓扫描确认框触发按钮不得绑定执行。

readonly 负空间（ADR-018）：创建、启停、删除、建站等入口均为 admin 专属，由上述源码断言覆盖；**未在真实浏览器里以只读账号核对**（见第十一节）。

## 四、真实浏览器核对（在真实页面里逐个点击）

**方式**：编译真实二进制（内嵌最终前端产物），隔离数据目录、Compose 落盘根与 PM2_HOME，在本机 Docker Desktop + 真实 PM2 7.1.2 上启动面板（端口 18697），用应用内浏览器打开真实页面：走页面自己的登录流程登录，再逐个点击入口，读取页面实际渲染的内容；涉及 Docker 的结果用 `docker` 命令行在 daemon 侧核对；是否发出了写请求用页面内的请求监听核对。只操作测试自己的对象（dg-e2e 目录下的项目与容器、dg-smoke / dg-confirm 前缀资源），全程未触碰既有容器与 Compose 项目。

**局限（如实）**：应用内浏览器的页面在本环境里**不绘制**（动画帧回调不运行、CSS 过渡不触发结束事件、真实鼠标输入不送达）。因此：
- 点击用页面内派发的 DOM 点击事件完成，走的是组件自己的处理函数，但**不等于真实鼠标点击**；
- 弹窗与气泡的入场 / 离场动画、遮罩效果**无法验证**（节点会停在过渡态，逻辑上的开关状态已另行核对）；
- 这一局限本身曾掩盖过一个缺陷：分栏切换动画在不绘制的页面里卡在第一帧，最初被当成纯环境问题；随后的核对才发现真正的问题是分栏实例复用（第五节第 1 项）。

**结果**：

| 核对项 | 结果 |
|--------|------|
| 登录 → 落到监控页；资源条「0 网站 / 0 数据库 / 8 容器 / **1 项目** / 0 环境」 | ✅ |
| 点「项目」格 → /sites?tab=python，落在 Python 项目分栏 | ✅ |
| 三分栏来回切换：每次只挂载当前分栏、不透明度 1、地址栏 ?tab 同步 | ✅ |
| 从监控页先进 Python 再切 Node：Node 分栏有独立仓库（app-node 注册）、读到真实项目行 n1（启动文件 / 18750 / 运行中 / 重启 0 次·53.3 MB / 仅本机可访问）；每次切换都是新实例，切到「网站」时面板被卸载 | ✅（修复后；修复前 Node 分栏沿用 Python 仓库，显示「还没有Node项目」） |
| 加载中：清单返回前状态条「检测中」、无「到环境管理安装」入口、表格加载态；返回后才显示真实状态 | ✅（修复后） |
| 「创建Node项目」弹窗：标题、字段「启动文件」、端口默认 3000、启动方式 = 启动文件 / npm 脚本（无「容器运行」）；切到 npm 脚本后出现「脚本名」默认 start | ✅ |
| 「创建Python项目」弹窗：入口文件 app.py、端口 8000、FastAPI 预设 / 自定义命令、模块:实例 | ✅ |
| Node 行内确认文案：开机自启「关闭开机自启会同时停止应用。确定关闭？」、停止、重启、按 package.json 重装依赖 | ✅ |
| 容器管理页签顺序：容器 / 应用 / Compose / 镜像 / 卷 / 网络 | ✅ |
| 「应用」页签：「创建应用」按钮；列为 序号 / 应用名 / 镜像 / 本机端口 / 状态 / 站点 / 操作；smokeapp 行 `127.0.0.1:18760 → 8080`（只绑本机）、运行中 | ✅ |
| 「创建应用」弹窗：应用名 / 镜像 / 主机端口 / 容器端口 / 环境变量（多组）/ 挂载（多组）/ 重启策略（四项），说明「应用端口只在本机可访问」 | ✅ |
| Compose 页签：从未启动的 ghostp 可见、状态「未运行」；每行有「删除项目」 | ✅ |
| 「创建容器」「新建 Compose 项目」弹窗可从按钮打开 | ✅ |
| 旧地址 /app → /sites?tab=python（Python 项目分栏） | ✅ |
| 全程无运行时错误、无 console.error / Vue 警告 | ✅ |
| **二次确认真的拦得住**：容器行「停止」「删除」、Compose 行「down」「删除项目」——点一下：无写请求、对象不变、出现确认气泡；点「确认」：恰好一次请求，Docker 侧结果相符 | ✅（修复后；修复前点一下即执行，见第五节第 3 项） |

**过程记录（如实）**：
1. 浏览器核对分几轮进行，中间因修复重建并重启过测试实例；每轮都用新数据目录，登录流程重新走。
2. 核对用的标签页与应用内浏览器是同一个，中途出现过一次非我发起的页面跳转和一次页面监听变量消失（后者两次受控复现均未再现，删除容器后页面并不刷新）；推测是该标签页同时被手动操作。此后自动化核对改在另开的标签页里进行，未再出现。
3. 测试实例停止、临时数据与测试镜像清理后，机器与开始前一致（7 容器 / 0 卷 / 6 网络 / 0 悬空镜像 / 0 测试进程）。应用内浏览器里原先那个标签页仍指向已停止的测试地址，未代为关闭。

## 五、核对中发现并修复的问题（5 项）

前 3 项在真实浏览器核对中发现，第 4 项在带数据升级测试中发现，第 5 项与第 1 项同源。全部先在真实环境复现、修复后复验，并补了会在回归时失败的测试。

| # | 问题 | 影响与来源 | 根因 | 修复 | 把关 |
|---|------|-----------|------|------|------|
| 1 | **分栏不带 key，Python / Node 面板被当成同一个实例** | 本轮引入（0a13e70）。从监控页「项目」入口先进 Python 再切 Node，Node 分栏沿用 Python 的仓库与表单默认值：永远显示「还没有Node项目」，创建表单是 Python 的默认值。直接打开 ?tab=node 一切正常，所以此前的浏览器核对通过是碰巧；切换时面板的轮询也不清理 | NTabs 在 display-directive="if" 下每次只渲染当前分栏这一个子节点；节点不带 key 时切换被当成同一节点改了属性，setup 不重跑 | 三个分栏节点各自带 key（868e3f9） | sites-hub-mount.test.ts：真实 Vue 运行时挂载，去掉 key 即失败 |
| 2 | 清单未返回时显示「还没有项目」「环境不完整」 | 原应用管理页起就存在，Python 探测快看不出；Node 清单要等 PM2，假状态挂一秒左右，且与监控页「1 项目」矛盾 | 仓库无加载态，就绪度与清单的初始值被直接呈现 | 仓库记录加载中与首次成功加载；状态条「检测中」、表格加载态、首次失败写「未能读取项目列表」（868e3f9） | 仓库层 3 用例 + 入口测试断言；3 个变异均被抓到 |
| 3 | **二次确认形同虚设：点一下就直接执行** | 容器行 停止 / 重启 / 暂停 / 删除 与 Compose 行 up / restart / down **自上线起如此**（波及 v0.1.0~v0.3.1）；本轮新增的「删除项目」（删目录、不可恢复）照抄了同一写法。真实页面复现：点一下「停止」容器即停、点一下「删除」容器即删、点一下「down」项目即停、点一下「删除项目」目录即删，均无确认 | 触发按钮自己绑了 onClick 执行，外面又套了确认框（确认后再执行一遍）。入口可达性测试只核对「按钮 → 处理函数」是否连通，错误接法同样满足；替身测试与接口端到端都不经过按钮 | 带确认文案的按钮只作确认框触发器，执行只放在 onPositiveClick（c077fb9） | __tests__/confirm-trigger.test.ts：全仓扫描每个 NPopconfirm 的触发按钮（渲染函数与模板两种写法），扫描器自身有用例；3 个变异均被抓到 |
| 4 | 迁移 018 重建表后自增序号回落 | 升级前删除过的项目 id 会被重新发放（全部删除过则从 1 开始）；任务台账永久保留，新项目会挂上旧项目的历史任务。数据本身不丢。010 / 014 / 016 / 017 四次重建是同样写法（已发布，未回改） | 重建只拷贝行，新表序号只到现存最大 id；此前所有表重建都只有空库测试 | 迁移在弃旧表前把旧序号带到新表（46dd540） | store 包 3 个带数据升级场景；去掉序号语句则 2 个场景失败 |
| 5 | 分栏切换动画 | 环境相关：不绘制的页面里过渡不结束。去掉动画也与面板其它页签页一致 | NTabs animated 依赖过渡结束事件 | 去掉 animated（868e3f9） | — |

**为什么现有门控没拦住**：三道检查各有盲区——源码断言看得见「连不连通」、看不见「实例是否复用」「按钮是不是自己就执行了」；接口端到端绕过了界面；此前的浏览器核对只走了一条进入路径。自此新增三道把关：挂载测试、全仓确认框扫描、带数据升级测试；并把「同一组件按属性挂在多个分栏时必须带 key」「确认框触发按钮不得绑定执行」写入设计文档 9.4。

## 六、真实环境端到端（最终二进制 46dd540，本机 Docker Desktop 29.8.1 / Compose v5.5.1 / PM2 7.1.2）

| 脚本 | 结果 | 覆盖 |
|------|------|------|
| 既有回归（run.sh） | **94 / 94** | 登录与 CSRF、创建容器 / 卷 / 网络、新建 Compose（落盘、up、服务级标签）、容器六动作、Compose up / down / restart / 日志、拒绝与校验各错误码；清理后与基线一致 |
| 本轮新能力（run2.sh） | **121 / 121** | A. Node 项目全链：创建 → .env 生效 → PM2 实况 → 启停 → 开机自启的持久语义（手动启停不保存、开关才保存）→ 日志 → 参数 → 重装依赖（npm install）→ npm 脚本方式 → 删除。B. 容器管理「应用」：创建 → daemon 侧核对（主机端口只绑 127.0.0.1、环境变量）→ 启停 → 日志 → 删除 + 拒绝场景（重名、名称非法、端口被占、应用不存在）。C. Compose 生命周期：零容器可见 → 再启动 → 删除（目录删除）；面板外项目只停止不删目录（deletedDir=false，目录原样） |

两个脚本各跑过两遍（修复确认缺陷前一遍、最终二进制一遍），结果一致。端到端过程中额外查出并已修复的问题（8319c7c 及之前）：网络删除拒绝归类（DK_010 / DK_011）、容器名提示与契约不符、清单未带出已保存的启动参数、Node 路径校验在 Windows 下误拒、Node 项目的依赖安装误查 venv、参数保存缺 Node 分支、TestParamsBaseline 计时抖动。

## 七、带数据升级测试（迁移 018）

对应已部署主机的升级路径（库里已有旧版项目与任务）。`store` 包 `TestMigrate018*`：先按旧结构（到 017）建库并写入旧版形态数据，再执行 018。

| 场景 | 断言 | 结果 |
|------|------|------|
| 有既有项目，且删除过 id 更大的项目 | FastAPI 预设 / 已下线的容器形态 / 自定义命令三条档案逐字段不变、runtime 取 python；任务台账 2 行保留；新建项目 id = 4（不复用已删除的 3）；runtime / start_mode 取值域与 unit_name 唯一生效；integrity_check = ok；runtime 列 NOT NULL DEFAULT 'python' | ✅ |
| 项目全部删除后升级 | 新建项目 id = 6（修复前为 1） | ✅ |
| 从未建过项目 | 新建项目 id = 1 | ✅ |

## 八、测试与变异检验

| 层 | 增量 | 说明 |
|----|------|------|
| 前端挂载测试（新增一类） | sites-hub-mount.test.ts 3 用例 | 真实 Vue 运行时 + 极简宿主挂载 SitesHub（不依赖 DOM、不加依赖）。vitest 的 node 环境把 .vue 按 SSR 编译（只有 ssrRender），为此加了一个「按 web 方式编译、不提供 DOM」的自定义环境（env-web-without-dom.ts，vite.config.ts 的 environmentMatchGlobs 只作用于这一个文件） |
| 全仓确认框扫描（新增一类） | confirm-trigger.test.ts 12 用例 | 10 个是扫描器自测（已知有问题的写法必须识别、正确写法不得误报；若扫描器认不出确认框则直接失败），2 个扫全仓 |
| 入口可达性 | restructure-entry.test.ts 41 用例 | 新增分栏 key、加载态断言；「删除项目」由「标签和函数存在」改为断言接法 |
| 仓库层 | app.test.ts 9 用例 | 加载中 / 首次失败不算已加载 / 并发请求 |
| 后端 | store 包迁移测试 3 场景 | 见第七节 |

变异检验（故意改坏 → 必须有测试变红 → 还原）：

| 改坏的内容 | 结果 |
|-----------|------|
| 表格去掉加载态绑定；就绪度去掉首次加载判断；环境管理入口去掉首次加载条件 | 各 1 个用例失败 ✅ |
| 三个分栏全部去掉 key | 挂载测试 2 用例失败 ✅ |
| 只去掉其中一个分栏的 key | 两分栏 key 不同、不会复用，挂载测试如实通过；源码断言失败 ✅ |
| 「删除项目」按钮重新绑上 onClick | 全仓扫描 + 入口测试断言失败 ✅ |
| 容器行 / Compose 行触发按钮绑上 onClick | 全仓扫描失败 ✅ |
| 迁移去掉序号保持语句 | 2 个升级场景失败 ✅ |

**测试基础设施的一处取舍**：挂载测试需要 .vue 编译出客户端 render。正路是装 jsdom 或 happy-dom，试算会往锁文件带进 53 个包（其中大半是其它平台的 esbuild 可选包），没有擅自安装；改用不加依赖的自定义环境。代价是用了 vitest 的 environmentMatchGlobs——vitest 3 将移除该配置项，届时需迁到 projects（第十节 W6）。

## 九、栈规则扫描（本轮相对 v0.3.1 改动的 44 个非测试代码文件，实跑）

| 规则 | 结果 |
|------|------|
| G-01 shell 拼接（sh -c / bash -c） | ✅ 0 命中 |
| G-02 SQL 拼接 | ✅ 0 命中（迁移 DDL 为静态文件） |
| G-03 硬编码口令 | ✅ 0 命中 |
| G-05 panic（非测试） | ✅ 0 命中 |
| G-06 goroutine | ✅ 0 新增（11 处均为主程序里既有的模块启动协程：v0.3.1 时 11 处、现在 11 处） |
| G-07 外部命令 | ✅ 3 处调用（docker compose、环境包管理、PM2）全部参数数组，不经 shell |
| F v-html / eval / new Function / localStorage / innerHTML 赋值 | ✅ 0 实际使用（2 处命中为注释里的纪律说明） |

## 十、WARN（待用户确认）

| # | 事项 | 处置建议 |
|---|------|---------|
| W1 | **只能在目标服务器上验证**：PM2 开机自启注册（`pm2 startup systemd` 仅 Linux，本机 Windows 报 Init system not found）；Python 项目的 systemd / venv 真实流程；nginx 建站链路（本机无 nginx，「网站」分栏显示未检出 nginx，建站动作未在本机走通） | 升级后按服务器冒烟清单执行（随 CHANGELOG 给出） |
| W2 | **行为变更需知**：容器行与 Compose 行带确认的按钮，由「点一下即执行」改为「确认后执行」。这是设计文档一直写明的行为，但对用过旧版的人是可感知的变化 | 写入 CHANGELOG，明确说明旧版点一下即执行 |
| W3 | **旧版本风险提示**：二次确认失效波及已发布的 v0.1.0~v0.3.1——其中的「删除容器」「down」等点一下即执行 | CHANGELOG 升级说明里提醒；是否需要单独通知由用户定 |
| W4 | **升级会重建 app 表**（迁移 018，单事务，带数据测试已覆盖） | CHANGELOG 升级提示：升级前备份数据库（通用建议） |
| W5 | 真实浏览器核对的局限：点击为页面内派发事件、弹窗与气泡动画未验证；只读角色界面未以只读账号核对 | 可接受则确认；需要的话在服务器上人工点一遍只读账号 |
| W6 | vitest 的 environmentMatchGlobs 将在 vitest 3 移除 | 升级 vitest 时迁到 projects（已在代码注释与本报告登记） |
| W7 | 已知遗留（不影响功能）：Compose 项目停止后在列表里排到末尾（运行中的在前、落盘扫描到的在后）；PM2 日志不轮转（范围外）；010 / 014 / 016 / 017 四次历史重建未保持自增序号（已发布，不回改） | 排序问题如需调整另开小轮 |
| W8 | Windows -race 下 appmgmt 偶发超时（沿先例；本次全仓 0 FAIL） | 如实登记 |

## 十一、未覆盖

- 只读角色界面（仅源码断言）；
- 真实鼠标点击、弹窗 / 气泡的动画与遮罩；
- 真实镜像拉取（本机已有基础镜像）、WebSocket 日志跟随；
- 服务器侧：PM2 开机自启、systemd / venv、nginx 建站（W1）；
- 旧版本库经真实旧二进制生成后再升级（第七节用真实迁移文件与同一驱动在内存库里模拟，未另建 v0.3.1 二进制实跑）。

## 十二、交付清单与发版

- **代码提交（本地，未推送）**：edbbcc7、4c3bc78、5a2849c、8319c7c（重组主体）；0a13e70（界面三分栏）；868e3f9（分栏 key、加载态）；c077fb9（二次确认）；46dd540（迁移 018 序号）。
- **设计产物**：跨模块设计 v1.2（含 9.4 前端实现约束）；ADR-033；BA/AA/FL、prd、api、PageSpec 各模块增量；domain-model v1.29、business-arch、function-list、枚举池；本报告；INDEX 登记。
- **发版**：用户确认 Gate 2 后推送，按 docs/RELEASING.md 发 **v0.4.0**（minor：新功能 Node 项目托管、容器应用、Compose 删除、信息架构重组）。CHANGELOG 需写入：W2 行为变更、W3 旧版本风险、W4 升级提示、W1 服务器冒烟清单。
