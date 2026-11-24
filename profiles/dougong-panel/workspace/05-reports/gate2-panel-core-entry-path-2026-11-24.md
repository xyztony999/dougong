# Gate 2 报告 · panel-core 安全入口路径缺陷修复增量（2026-11-24）

> 判定：**WARN（CRITICAL 0，沿全仓先例）**——修复已完成，编译门控全绿，在真实浏览器（无头 Edge + CDP）里逐项验收通过；WARN 为需要用户知情的边界与未覆盖项，待用户确认。
> 输入：D0 `00-architecture/d0-clarification-panel-core-entry-path-2026-11-24.md`（第 3 节根因、第 6 节候选方案、第 7 节用户拍板方案 A「完整前缀」、第 8 节实现与验收记录）。
> 缺陷原话：「当然要修复！」（针对取证时发现的「安全入口路径开启后面板在浏览器里打不开」）。

## 〇、执行说明（如实）

- 本轮由主会话执行（第一轮流水线子代理因账户使用上限中断后，主会话已接手；本轮全程主会话）。
- 真实浏览器核对用**无头 Edge + CDP**，不是应用内浏览器（那个窗格在本机不绘制）。启动条件：全新临时配置目录、调试端口只绑 127.0.0.1、禁用同步/扩展/后台联网、屏蔽一切非本机域名解析、只访问本机隔离实例 `127.0.0.1:18697`；每次运行结束经 `Browser.close` 关闭、按命令行核对无残留进程、删除临时目录。本轮多次启动均正常关闭，无残留。
- 点击走浏览器真实输入管线（`Input.dispatchMouseEvent`）；弹层按钮等入场动画结束、位置稳定后再点（动画中取位置会点偏，第一轮已踩过）。
- 测量工具与原始数据在本机临时目录 `E:\Temp\dg-sidebar-click\`（`scripts\`、`out\`），不入库。
- 验收实例用的是当前代码构建的二进制（前端产物同步进 `backend/web/dist` 后 `go build`）；升级场景另构建了一个入口脚本哈希不同的第二二进制（`vite build --target es2015`）做真实切换。

## 一、编译门控（最终代码，主会话实跑，全绿）

| # | 门控 | 结果 |
|---|------|------|
| 1 | go build ./... | ✅ exit 0 |
| 2 | go vet ./... | ✅ exit 0 |
| 3 | go test ./... -race -count=1 | ✅ 55 包 0 FAIL |
| 4 | vue-tsc --noEmit | ✅ exit 0 |
| 5 | vite build | ✅ exit 0 |
| 6 | npx vitest run | ✅ 35 文件 531 用例（第一轮为 31 文件 458） |
| 7 | gofmt -l | ✅ 0 文件 |
| 8 | GOOS=linux 交叉编译 amd64 / arm64 | ✅ 均 exit 0 |
| — | dist 同步 backend/web/dist 后 go build | ✅（验收二进制即由此构建；验收后已恢复原产物） |
| — | Linux 上的行为 | ✅ Docker Desktop 未运行（不自行启动），改用 WSL2 直接运行交叉编译出的静态测试二进制：`internal/api` 整包 PASS |

## 二、修复内容与落点（方案 A「完整前缀」）

| 决定（D0 第 7 节） | 实现 |
|------|------|
| 构建相对地址 | `frontend/vite.config.ts` `base: './'`——入口 HTML 引用 `./assets/…`、`./favicon.svg`，按需分包相对入口脚本自身（`import.meta.url`）解析 |
| 服务端注入 `<base>` | `backend/internal/api/server.go`：下发 index.html 时在 `<head>` 后注入 `<base href="/{entryPath}/">`（未开启 `/`）；`spaBasePath` 经 `SetEntryPathSource(panelSvc)`（main 装配）每请求读状态，读态失败 fail-closed 404（同 D-P03）；`spaIndexWithBase` 大小写不敏感、HTML 转义、无 `<head>` 原样、不原地改输入；Content-Length 用注入后长度、HEAD 同头 |
| 前端统一应用基址 | `frontend/src/utils/base.ts`（新增）：`appBasePath()` 读 `<base>`、`withBase()`、`joinBase()`；供 `api/http.ts`（5 fetch）、`api/auth.ts`（审计导出）、`api/terminal.ts`·`api/docker.ts`（2 WS）、`router/index.ts`（`createWebHistory(appBasePath())`）、`router/navFailure.ts`（自动重载 `assign(withBase(target))`）、3 处图标使用 |
| 设置页整页跳转 | `entryPathActions.ts`（新增）+ `EntryPathCard.vue`：开启/重新生成/关闭成功后整页跳到新地址下的设置页（不在旧基址页面重读配置）；会话存储记一笔（60s 时效）供新页面补提示与路径展示（prd N2 不丢） |
| 防回归扫描 | `src/__tests__/sitePaths.test.ts` + `support/scanSitePaths.ts`（新增）：全仓扫描根路径字面量 fetch/WS、白名单、同源完整地址未经 withBase、模板根路径、整页跳转根路径——以后新模块漏带前缀会被测试拦下 |
| 安全口径不变 | EntryPathGuard「不带前缀一律 404」语义未动；不依赖浏览器 Cookie；HostGuard 链序（Entry→Host→gin）与数据库工具内部端点豁免未动 |

## 三、真实浏览器与 HTTP 验收（主会话实跑）

| 面 | 结果 |
|----|------|
| HTTP·入口关闭 | 24/24：根地址入口文档带 `<base href="/">`、深链接、页面文件、接口、WS 握手、内部端点豁免照常 |
| HTTP·入口开启 | 32/32：入口文档带 `<base href="/{entryPath}/">`、相对地址解析后逐个 200、深链接、不带前缀/错误前缀一律标准 404、WS 握手到达终端模块、内部端点豁免不受影响 |
| 浏览器·设置页三动作 | 16/16：开启/重新生成/关闭各自整页跳到正确地址（约 1.2~1.3s），`<base>`、提示、路径展示框都对，旧路径立即 404，根地址下页面照常 |
| 浏览器·完整走一遍 | 23/23：入口地址冷打开→登录页（不再空白）→登录→21 个侧栏页面→4 组深链接刷新+按需分包→在线编辑器→终端 WS→退出；全程无失败请求、无落在入口路径之外的请求 |
| 浏览器·升级自动重载 | 5/5：服务换成另一构建后，在旧页面点没打开过的页面→162ms 自动重新加载到入口路径下的目标页，HEAD 探测旧入口脚本在入口路径下得 404，提示「面板已更新，页面已重新打开」 |

## 四、变异检验（主会话实跑）

| 面 | 结果 |
|----|------|
| 服务端入口文档注入 | 15/15 被抓到（基址不带前缀/忽略开关/读态失败 fail-open/不注入/Content-Length 不更新/注入位置错/不转义/原地改输入/无 head 被改/未闭合 head 被改/大小写敏感/缺尾斜杠/缺头斜杠/读错忽略/空路径仍加前缀），0 存活、0 无效，源文件逐字节恢复 |
| 前端基址/构造点/设置页动作 | 17/17 被抓到（withBase 不加前缀/appBasePath 恒 //joinBase 丢基址/接受 //host/接受相对 base/请求·导出·终端 WS·日志 WS·自动重载漏前缀/开启·关闭·重新生成跳错地址/不记一笔/TTL 失效/entrySettingsUrl 恒根/全仓扫描器识别 WS 漏前缀），0 存活、0 无效，源文件逐字节恢复 |

## 五、WARN（需用户知情，待确认）

| # | 项 | 说明 |
|---|----|----|
| W1 | 升级前已打开的旧页面仍需手动刷新一次 | 承第一轮固有边界：旧页面引用的分包文件名已不存在，且旧版本前端不含本修复。CHANGELOG 须注明「升级后请刷新一次已打开的面板页面」 |
| W2 | 终端 WebSocket 在本机无法完成握手 | Windows 开发机终端后端创建不了进程（TERM_004，Unix PTY 平台限制）。对照实验：入口路径下与根路径都返回同样 TERM_004，且入口路径下能通过鉴权、到达终端模块（非网关 404）——与本轮修复无关；浏览器已确认 WS 地址正确走在入口路径之下。真实终端会话归 Linux 目标环境冒烟 |
| W3 | 日志跟随 WebSocket 未在浏览器实测 | 验收实例 Docker Desktop 未运行（不自行启动），无容器可跟随；地址构造与终端 WS 同一封装（经 withBase），由单测覆盖，网关侧路由同口径 |
| W4 | 真实链路效果待用户升级后确认 | 验收在隔离实例；用户服务器升级后的实际效果待确认 |
| W5 | 只读角色未在浏览器单独核对 | 承第一轮 W5；readonly 对安全入口 Card 全可见、控件禁用，整页跳转交互仅 admin 触发 |
| W6 | 只测了 Chromium 内核 | 无头 Edge；其它内核未测 |
| W7 | 自动重新加载会丢当前页未保存的输入 | 承第一轮 W2（升级自动重载的固有代价） |

## 六、结论

CRITICAL 0、契约断裂 0。方案 A「完整前缀」已实现并经真实浏览器验收：开启安全入口后面板不再白屏，页面/脚本/图标/接口/WebSocket 全部位于入口路径之下，不带前缀一律 404 的安全口径不变、不依赖 Cookie。编译门控全绿、变异 32 个全被抓到。W1~W7 为边界与未覆盖项，待用户经主会话确认后记 pass。推送与发版由用户决定（打 tag 前必须先看到 main 的 CI 变绿）。