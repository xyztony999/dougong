# 更新日志 / Changelog

发布说明按版本倒序排列；每个版本的完整说明也会作为 GitHub Release 正文发布。

## v0.7.0 - 2026-11-27

登录安全适配：支持 passkey（无密码登录 + 密码后第二因素）、一次性恢复码、新设备识别。

### 变更内容

**新增：passkey（WebAuthn）登录**
- 个人安全设置里可以「添加 passkey」（用本机指纹/面部/安全密钥注册）；注册后可在登录页点「使用 passkey 登录」免密码进入
- 也可把 passkey 当作第二因素：输入密码后，若账户没有动态码（TOTP）但有 passkey，会要求再用 passkey 确认一次
- passkey 需要 HTTPS 或 localhost 安全上下文；用 IP 直连时面板会提示改用 HTTPS 域名

**新增：恢复码**
- 个人安全设置里可生成 10 个一次性恢复码（明文只显示一次，请妥善保存）；passkey 或动态码丢失时，登录页可「用恢复码登录」，每个码只能用一次
- 重新生成会使旧码全部作废

**新增：新设备识别**
- 登录成功后若来自新设备（浏览器指纹首次出现），会话带 newDevice 标记并记入审计，便于发现异常登录

**其它**
- 登录页在浏览器不支持 passkey 时自动隐藏相关按钮

### 安装与升级

```bash
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash
dougong version   # 应为 v0.7.0
```

### 升级注意

- 本版包含数据库迁移 019（新增 passkey/恢复码/设备三张表），升级自动执行，不动现有数据；稳妥起见升级前备份 /var/lib/dougong
- passkey 默认不启用，需用户在个人安全设置里主动添加；不添加则登录方式与之前完全一致
- 通过 IP 直连面板时无法使用 passkey（WebAuthn 要求域名或 localhost），请绑定域名并启用 HTTPS 后使用

## v0.6.2 - 2026-11-25

两处修复：检查更新一直报「版本源不可达」；绑定域名后不知道该怎么继续访问。

### 变更内容

**修复：检查更新可用**
- 此前版本源没有接线，点「检查更新」永远报「版本源不可达，可稍后手动重试」。现在接上了 GitHub 发布源，开启更新检查后点「检查更新」能拿到最新版本并提示是否有更新

**修复：绑定域名后给出访问链接**
- 绑定域名后 IP 直连会被拒绝（404），此前界面没有提示该怎么继续。现在绑定成功后显示醒目的「请改用域名访问」提示和可点链接，点一下即跳到域名地址
- 不自动跳转：避免域名还没解析/配好时把你锁在网页外；万一锁住，服务器上 `dougong unbind-domain` 可解绑恢复

### 安装与升级

```bash
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash
dougong version   # 应为 v0.6.2
```

### 升级注意

- 本版无数据库迁移、无配置变更
- 检查更新默认仍关闭（不会自动外呼），需在设置里开启后手动点击

## v0.6.1 - 2026-11-25

修复：导入证书后界面仍显示「自签证书 正在使用」。

### 变更内容

**修复：导入证书即启用**
- 此前导入证书只是把证书存起来，HTTPS 模式还停在原来的「自签」，界面谎报「自签证书 正在使用」（实际已在用你导入的证书），这时点「重新生成自签证书」还会把导入的证书覆盖掉
- 现在导入证书成功后自动切换到「上传证书」模式，界面「上传证书 正在使用」与实际一致，导入了就用上

### 安装与升级

```bash
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash
dougong version   # 应为 v0.6.1
```

### 升级注意

- 本版仅修复导入证书的模式切换，无数据库迁移、无配置变更
- 升级后重新导入一次证书（或导入新证书），即会正确显示「上传证书 正在使用」

## v0.6.0 - 2026-11-25

环境管理按宝塔的方式进到每个环境里改配置、换版本；面板 HTTPS 和证书收成一张卡；安装确认等提示改成直说结果。

### 变更内容

**环境管理：每个环境有独立设置页**
- 已安装的环境点「设置」（或点卡片）进入该环境：服务启停、常用配置、配置文件、版本切换
- nginx / Apache / PHP-FPM / MySQL / Redis / MongoDB 提供常用项表单；改配置文件时 nginx 会先校验，失败则回滚
- 可从仓库版本列表切换或升级到最新

**面板 HTTPS：设置和证书合成一张卡**
- 四种方式（关闭 / 自签 / 上传 / Let's Encrypt）选一个再操作，不再分两块互相绕
- 已导入的证书可以直接启用，不必再贴一遍；Let's Encrypt 已签发时点启用即可
- 导入证书后颁发者空白：CN 为空时改读组织名，旧证书打开时也会从文件补出来

**提示文案**
- 安装确认、域名绑定、安全入口等提示改成直接说会怎样、接下来做什么

**修复：新服务器首次安装 phpMyAdmin / mongo-express 失败**
- 自动落地块尚不存在时，被误判成读取失败导致装不上。现已按「文件不存在」继续自动建块

### 安装与升级

```bash
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash
dougong version   # 应为 v0.6.0
```

### 升级注意

- 无数据库迁移、无配置变更，升级后原有设置不变
- 升级后请刷新一次已打开的面板页面

## v0.5.1 - 2026-11-24

修复：HTTPS 设置里导入证书时，私钥粘贴进去报「证书解析失败或与私钥不配对」。

### 变更内容

**修复：私钥 PEM 导入失败**
- 私钥输入框此前是单行框，粘贴多行 PEM 时换行会被浏览器折成空格，私钥格式被破坏，导致导入失败。现在私钥和证书一样是多行文本域，粘贴多行 PEM 不再被破坏
- 私钥仍默认遮罩显示（旁边有「显示私钥 / 隐藏私钥」开关可临时核对），遮罩不再影响多行粘贴
- 后端解析前会对 PEM 容错整理：即便粘贴进来的格式带了多余空格、挤成一行，也会先整理成标准格式再解析；只整理空白，证书与私钥是否配对的校验照常严格

### 安装与升级

```bash
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash
dougong version   # 应为 v0.5.1
```

### 升级注意

- 本版仅修复证书导入，无数据库迁移、无配置变更，升级后原有设置不变
- 在 v0.5.0 及更早版本里，私钥框是单行的，多行 PEM 粘进去都会被折坏、无法导入；升级到本版后即可正常导入（如 Cloudflare Origin CA 证书）

## v0.5.0 - 2026-11-24

两处修复：一是侧栏切换页面时的体验——点了一个功能要等很久才有反应、有时干脆没反应；二是开启「安全入口路径」后面板在浏览器里打不开（一片空白）。

### 变更内容

**修复：侧栏切换页面有反馈、失败可恢复**
- 点击侧栏菜单项后，被点的项立即高亮；切换超过 200ms 还没完成时，顶栏显示「正在加载页面…」，完成或失败即消失
- 页面文件没下载下来时不再静默：网络原因会提示「页面没加载出来。请检查网络后再点一次」，再点一次自动重试；面板升级后旧页面引用的文件已不存在时，自动重新加载并打开刚才点的页面（同一目标 30 秒内至多自动一次），并提示「面板已更新，页面已重新打开」
- 弹窗、抽屉正在关闭时，落在侧栏上的点击照常生效（此前会被遮罩吃掉）

**修复：开启安全入口路径后面板打不开（白屏）**
- 此前开启安全入口路径后，用浏览器打开入口地址是一片空白：网关要求每个请求都带入口路径前缀，而页面引用的脚本、图标和所有接口地址都没带。现在页面、脚本、图标、接口、WebSocket 全部位于入口路径之下，开启后面板正常可用
- 安全口径不变：未携带入口路径的访问仍返回标准 404；不依赖浏览器 Cookie
- 设置页里开启、重新生成、关闭安全入口后，页面会自动跳到对应的新地址（开启 / 重新生成跳到入口路径下的设置页，关闭跳到根地址），并再次展示入口路径供复制保存

**其它**
- 面板自己压缩页面文件传输（gzip），慢链路下首次打开页面明显更快；已不存在的页面文件返回标准 404（供前端识别「面板已升级」）

### 安装与升级

```bash
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash
dougong version   # 应为 v0.5.0
```

### 升级注意

- **升级后请刷新一次已打开的面板页面**：升级前已打开的旧页面引用的文件已不存在，需手动刷新一次才能用上本版修复（之后打开的页面不受影响）
- **自动重新加载会丢失当前页未保存的输入**：面板升级触发的自动重新加载会刷新页面，正在编辑而未保存的内容会丢
- **安全入口的书签 / 链接必须带入口路径**：开启安全入口后，丢了入口路径前缀的地址会看到 404，这是预期的安全行为；请保存带入口路径的完整地址

### 升级后建议在服务器上验证

下面几项依赖 Linux 服务器环境，在开发机上无法验证：

- 开启安全入口路径后，用浏览器打开入口地址能正常登录、各页面可用；终端能建立会话
- 慢链路下点击侧栏切换页面能看到加载提示；面板升级后旧页面能自动重新加载

### 已知遗留

- 终端与容器日志跟随的真实会话依赖 Linux 环境（开发机 Windows 无法创建终端进程），本版已在浏览器里确认其连接地址正确走在入口路径之下

## v0.4.0 - 2026-11-23

界面重组：按部署方式把入口分成「网站管理」和「容器管理」两块——不用容器部署的项目（nginx 站点、Python、Node）都归到网站管理，用容器部署的都归到容器管理。新增 Node 项目托管和容器管理的「应用」页签；修复一处从早期版本起就存在的问题：带二次确认的按钮，点一下就会执行。

### 变更内容

**调整：网站管理改为三个分栏，「应用管理」菜单移除**
- 网站管理页现在有三个分栏：「网站」（原有的 nginx 站点，功能不变）、「Python 项目」、「Node 项目」。当前分栏会记在地址栏里（例如 `/sites?tab=node`），刷新或分享链接后位置不丢
- 侧栏不再有「应用管理」。旧地址 `/app` 会自动跳到「Python 项目」分栏；监控页资源条里的「应用」改名为「项目」，点击进入「Python 项目」分栏
- 项目列表还没加载完时显示「检测中」，不再先闪一下「还没有项目」「环境不完整」

**新增：Node 项目托管（由 PM2 管理进程）**
- 在「网站管理 › Node 项目」里创建 Node 项目：填写应用名、代码目录和监听端口，启动方式选「启动文件」（如 server.js）或「npm 脚本」（如 start）。项目里有 package.json 时会自动执行 npm install
- 进程交给 PM2 管理（进程名 `dougong-app-<应用名>`，在服务器上可以用 `pm2 list` 看到）。列表显示运行状态、重启次数和内存占用；可以启动 / 停止 / 重启、看日志、修改 .env 和启动参数、建站、删除
- 「开机自启」开关：打开会同时启动项目，关闭会同时停止项目（切换前会先确认）；平时的手动启停不影响这个设置
- 环境管理里安装或升级 Node.js 时，会顺带安装 PM2 并设置开机自启。这两步如果失败，不影响 Node.js 的安装结果，原因会写在任务输出里
- 创建 Node 项目需要环境管理里有 Node.js；缺 node、npm 或 PM2 时分栏顶部会提示到环境管理处理
- 原应用管理里的「容器运行」选项已移除（它在 v0.3.0、v0.3.1 里本来就无法使用）。需要用容器部署，请到「容器管理 › 应用」

**新增：容器管理的「应用」页签**
- 填写名称、镜像、主机端口和容器端口，就能创建并启动一个应用；可以再加环境变量、挂载和重启策略
- 应用的端口只开放给本机（127.0.0.1），点「建站」配置反向代理后才对外提供访问
- 支持启动 / 停止 / 重启、看日志、建站、删除。删除会移除容器和项目目录，需要输入应用名才能确认

**新增：Compose 项目停止后仍可见，可以删除**
- 已经停止、下面没有容器的 Compose 项目，现在仍然出现在列表里（状态显示「未运行」），可以直接再启动。v0.3.1 更新日志里提到的「停止后从列表消失」问题就此解决
- 每个 Compose 项目新增「删除项目」：停止并移除全部容器；由面板创建的项目还会删除项目目录，确认框里会写明是否会删除目录。v0.3.1 更新日志里提到的「项目名被占用」问题就此解决

**修复：带二次确认的按钮，点一下就会执行（自 v0.1.0 起）**
- 容器页的停止、重启、暂停、删除，以及 Compose 页的启动、重启、停止：此前的确认气泡是在操作已经执行之后才出现的，点「确认」还会再执行一遍。也就是说在 v0.1.0 到 v0.3.1 里，点一下「删除」，容器就被删掉了
- 现在这些按钮点一下只弹出确认，点「确认」之后才执行，并且只执行一次

**其它**
- 「参数」页签现在按项目实际保存的启动参数预填（此前会显示创建表单里上次输入的内容）
- 新增错误提示：Node 环境不完整、Node 启动参数不合法、「容器运行」已移除、应用不存在、应用名格式不对、主机端口已被占用、应用操作失败、Compose 项目删除失败

### 安装与升级

```bash
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash
dougong version   # 应为 v0.4.0
```

### 升级注意

- **操作方式有变化**：容器页和 Compose 页里带确认的按钮，现在是点「确认」之后才执行。旧版本是点一下就直接执行，如果已经习惯旧的操作方式，请注意现在多一步确认
- **旧版本也有同样的问题**：v0.1.0 到 v0.3.1 都存在「带确认的按钮点一下就执行」的情况，包括删除容器。建议尽快升级
- **升级会重建应用表**：本版包含数据库迁移 018（应用表增加「运行时」一列）。迁移在一个事务里完成，已经验证过带着现有项目数据升级时数据不变；稳妥起见，升级前建议备份 `/var/lib/dougong`。升级后原有的应用都归为 Python 项目，出现在「网站管理 › Python 项目」
- **界面位置有变化**：侧栏的「应用管理」菜单没有了，Python 项目在「网站管理 › Python 项目」；容器管理多了「应用」页签

### 升级后建议在服务器上验证

下面几项依赖 Linux 服务器环境，在开发机上无法验证：

- 在环境管理里把 Node.js「升级」一次：任务输出里应能看到 PM2 安装成功，`pm2 -v` 能输出版本号
- 创建一个 Node 项目并打开「开机自启」：`pm2 list` 里能看到 `dougong-app-<应用名>`；重启服务器后项目自动恢复运行
- 创建一个 Python（FastAPI）项目：服务被 systemd 启动
- 给一个 Python 项目、Node 项目或容器应用「建站」：用域名可以访问

### 已知遗留

- Compose 项目停止后，在列表里会排到末尾（运行中的项目排在前面），不影响使用
- Node 项目的日志由 PM2 写入文件，面板不会自动清理，长期运行需要自行清理（例如 `pm2 flush`）

## v0.3.1 - 2026-11-23

修复：v0.3.0 新增的容器管理四项创建能力实际上无法使用——界面上没有创建按钮，后端也有多处必然失败。本版补齐入口并修复后端，已在真实 Docker 环境完成端到端验证；同时修复 Compose「启动」和网络删除提示两处历史问题。

### 变更内容

**修复：容器管理四项创建无法使用（v0.3.0 遗留）**
- v0.3.0 里四个创建弹窗已经实现，但容器 / Compose / 卷 / 网络四个页签的工具栏都没有按钮能打开它们，所以界面上无法创建——现已补齐「创建容器 / 新建项目 / 创建卷 / 创建网络」按钮（管理员专属，只读账号不显示）
- 创建容器表单补全：多组端口映射、多组环境变量、多组挂载可动态增删；重启策略下拉；网络下拉选择（留空使用默认 bridge）
- 新建 Compose 项目：修复「创建并启动」必然失败（传给执行器的指令不合法）；归属标签改写到服务级（此前写在文件顶层，被 Docker 拒绝）；启动失败且项目下没有容器时自动清理已写入的文件
- 创建失败时弹窗保持打开并显示具体原因（此前只弹出未格式化的错误）；「创建后自动启动」开关此前不生效（固定为开启），现已生效
- 新增九条错误码的中文提示（容器名非法、端口映射格式、环境变量格式、挂载格式、创建失败等）

**修复：Compose 项目「启动」一直失败（历史问题）**
- 面板生成的命令把 `-d` 放在了 `up` 前面，Docker 直接拒绝——该按钮自 v0.1.0 起点击必然失败，现已修正

**修复：删除被容器占用的网络时提示有误（历史问题）**
- 此前提示「预定义网络不可删除」，现正确提示「网络被容器接入，无法删除」——Docker 对这两种情况返回相同的错误类别，现按具体原因区分

**订正：容器名输入提示**
- 容器名允许大写字母、点和下划线（与 Docker 原生规则一致），输入提示与报错文案此前误写为「小写字母/数字/连字符」，已订正

**同步带上：登录后默认进入监控总览页**（同期的界面调整，随本版二进制一并发布）
- 登录后默认打开「监控」页（原为「用户与安全」）；监控页首屏新增资源数量条（站点 / 数据库 / 容器 / 应用 / 环境，点击可跳转）

### 安装与升级

```bash
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash
dougong version   # 应为 v0.3.1
```

### 升级注意

- 本版没有数据库迁移，升级不改动既有数据
- **已知遗留（后续版本修复）**：Compose 项目执行「停止」后，项目下已无容器时不会出现在列表里，也无法在面板中再次启动。需要继续使用时，请在服务器上进入项目目录手动启动：`cd /opt/dougong-compose/<项目名> && docker compose up -d`
- **已知遗留（后续版本修复）**：应用管理的「容器运行」选项在 v0.3.0 中同样不可用，本版未修复，将随「网站 / 容器」界面调整一并处理——请暂时不要使用该选项（「venv 运行」不受影响）

## v0.3.0 - 2026-11-22

新功能：容器管理新增「创建容器 / 创建卷 / 创建网络 / 新建 Compose 项目」四项创建能力；应用管理新增「容器运行」形态（单应用 compose 编排）；修复环境管理 Python 预装主机 pip 缺位。

### 变更内容

**新增：容器管理四项创建能力**
- 创建容器：表单填写镜像（本地缺失时自动先拉取，拉取失败不创建）/容器名/端口映射/环境变量/挂载/重启策略/网络选择（下拉），创建即启动；服务端逐项校验（端口映射格式、环境变量格式等，非法值如实拒绝）
- 创建卷：名称 + 驱动（默认 local）；重名与 daemon 拒绝如实透传
- 创建网络：名称 + 驱动（默认 bridge）+ 子网与网关可选；重名/子网冲突如实透传
- 新建 Compose 项目：粘贴 compose.yml 文本（服务端 YAML 语法校验，报错带行号）→ 面板自动创建目录并落盘 → 自动 `compose up -d` → 项目进入 Compose 列表接管；项目名小写连字符，重名如实拒绝
- 新增错误码 DK_019~027（容器名非法/端口映射格式/环境变量格式/挂载格式/创建失败/卷创建失败/网络创建失败/YAML 校验失败/项目重名）
- 新增四条审计动作（container_created/volume_created/network_created/compose_project_created）

**新增：应用管理「容器运行」形态**
- 创建应用时可选 venv 运行（既有）或容器运行：填镜像（必填）/端口/环境变量/挂载/重启策略，面板生成单应用 compose 编排（含 dougong-app 归属标签）并自动启动
- 容器化应用的启停/重启/日志走容器通道（与 venv 形态的 systemd 通道并存，互不干扰）
- 建站反向代理对容器化应用同等适用（反代到容器映射端口）
- 含数据库迁移 017（应用表启动方式扩 container 值）

**修复：环境管理 Python 预装主机 pip 缺位**
- 此前 Python 已预装的系统（如 RHEL 自带 python3）会显示「已安装」但实际缺 pip/venv 支撑，应用管理创建应用时被如实拒绝且无补装路径
- 现已修复：环境管理 Python 条目升级动作会自动补装 python3-venv/python3-pip 支撑包；应用管理创建表单的缺失分层提示与到环境管理的引导同步就绪

**修复：应用管理空态双渲染**
- 应用列表为空时，表格默认「无数据」与自定义提示同时出现——现已收敛为单一空态视图

### 安装与升级

```bash
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash
dougong version   # 应为 v0.3.0
```

### 升级注意

- 含数据库迁移 015~017（应用管理两张新表 + 应用表启动方式扩 container 值 + 环境类型扩 python 值），升级时自动执行，既有数据不受影响
- 容器管理四项创建为管理员专属操作，只读账号不受影响
- 环境管理 Python 条目升级后若系统已有 python3 但缺 pip，面板「状态」栏会如实呈现缺失项——到环境管理点「升级」即可补装

## v0.2.0 - 2026-11-21

新模块：**应用管理**——在面板上一站式托管 Python 应用（venv + systemd 路线）；环境管理新增 Python 运行环境。

### 变更内容

**新增：应用管理（Python 应用托管）**
- 把部署一个 Python 应用要手工做的六件事收拢成一个页面：创建应用（自动建 venv → 按 requirements.txt 装依赖 → 生成系统服务）→ 启停/重启/开机自启 → 参数修改 → 看日志 → 一键建站对外暴露 → 删除
- 创建前逐项校验并如实提示：应用名重复 / 代码目录或入口文件不存在 / Python 环境未装（引导到环境管理安装）/ 端口被占用
- 应用默认只监听本机（127.0.0.1），对外暴露走面板建站的反向代理（可一键创建，也可选填域名）；端口被占用时如实报错，不会偷偷换端口
- 服务异常退出自动拉起（systemd Restart=on-failure）；创建/删除/依赖安装为后台任务，失败原因可见、可重试
- 删除应用绝不静默删代码：代码目录默认保留，只有显式勾选才连目录一起删；已建的站点不受影响
- 应用环境变量（.env）与启动参数可视化编辑，保存前备份、写坏自动还原；保存后提示重启生效
- **日志仅管理员可看**；只读账号可查看应用清单与任务状态（与其他模块一致的只读边界）

**新增：环境管理支持 Python 运行环境**
- 环境目录八环境 → 九环境（+Python，发行版仓库包，无需第三方源；Debian 系自动补装 python3-venv/pip 支撑）
- Python 为运行时型环境：无后台服务，装完即用；就绪度细分到 venv/pip 能力位（应用管理创建前置检查消费）

**修复**
- 应用任务重试语义修正：失败任务的「重试」此前不会真正重新执行（任务停在执行中），现已修正并有回归测试锁定（新模块随本轮首发，不影响既有功能）

### 安装与升级

```bash
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash
dougong version   # 应为 v0.2.0
```

### 升级注意

- 含数据库迁移 015/016（应用管理两张新表 + 环境类型扩 Python），升级时自动执行，既有数据不受影响
- 环境管理新增 Python 条目；此前安装过第三方 Python 的服务器上，面板按发行版包检测结果如实呈现（自装形态以实际包数据库为准）

## v0.1.17 - 2026-11-20

CLI 交互式救援菜单（宝塔 `bt` 风格）+ 界面文案全面「说人话」。

### 变更内容

**新增：`dougong` 交互式管理菜单与紧急救援命令**
- 在终端输入 `dougong` 不再直接启动服务（避免与已运行的 systemd 服务撞端口），而是弹出编号菜单：查状态/版本、重置密码、改用户名、解除域名绑定、关安全入口、改端口、启停服务
- 新增救援命令（被锁在面板外时救回访问）：
  - `dougong rename-user -username <旧> -new <新>`：强制修改面板用户名（会话级联吊销 + 审计）
  - `dougong unbind-domain`：解除域名绑定，绑域名后 DNS 未生效/域名失联时一键恢复 IP 直连（即时生效）
- 新增 `dougong start|serve` 显式启动别名；**兼容既有部署**：非交互场景（systemd/管道）裸 `dougong` 仍启动服务，新装 install.sh 改用 `dougong start`
- 新增审计动作 `username_changed`（修改用户名，security）

**改进：全局面文案净化（无功能变更）**
- 数据库工具：安装/卸载/打开提示去掉「nginx 落点」「独立访问块」「signon 令牌」「经面板会话安全访问」等内部说法，改为直白描述；「轮换凭据」→「重置密码」，「目录版本」→「最新版本」
- 环境管理：「实况」→「状态」、「治理」→「管理」、「引导（幂等）」→「自动配置（不会重复配置）」、「保守跳过」→「已跳过自动配置」
- 数据库管理：「纳管」→「移除/托管」、「凭据」→「账号密码」、「治理/观测」→「管理/查看」、「密码不回显」→「密码保存后不再显示」；导航标题与面包屑「MySQL/Redis/MongoDB 治理」→「管理」
- 网站管理：「反代」→「反向代理」、「未纳管」→「未托管」、证书「存量导入」→「手动导入」
- 其它：容器「兜底」、防火墙「放行」、备份导入「幂等」、文件权限「实况」、HTTPS 私钥「回显」、DNS「凭据」等统一改为常用说法；审计日志与错误码文案同步

### 安装与升级

```bash
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash
dougong version   # 应为 v0.1.17
```

升级后验证：①终端输入 `dougong` 弹出管理菜单（选 2 看版本、0 退出）；②`dougong unbind-domain` 可解除域名绑定恢复 IP 访问、`dougong rename-user` 可改用户名；③systemd 服务正常运行（`systemctl status dougong`）；④各界面提示均为通俗表述。

## v0.1.16 - 2026-11-20

数据库工具**自动 nginx 落地**：IP 直连面板也能一键安装/打开 phpMyAdmin、mongo-express，无需手动建反代站点；工具访问与面板域名绑定联动（对齐宝塔「绑域名后仅域名访问」）。

### 变更内容

**新增：工具自动落地（IP 直连可用）**
- 痛点：v0.1.14/15 安装工具报 DBT_012——经 IP+端口直连面板（未绑域名）时 nginx 没有面板 server 块，工具路径无处承载
- 修复：安装时若 nginx 无面板落点，**自动创建专用工具 server 块**（`dougong-tools.conf`，监听端口 **18090**）；工具经 `http://IP:18090/dbtool/...` 访问，面板会话 cookie 跨端口共享、gating 照常校验
- 既有面板域 server 块（绑域名/反代面板）优先沿用；端口被占用如实报错引导

**新增：工具访问与域名绑定联动**
- 未绑域名：工具块认 IP/任意 Host；**绑定域名后工具块只认该域名、其余 Host 返回 404**（镜像面板 HostGuard 强校验）——与面板「绑域名后仅域名访问」一致
- 绑定/解绑域名经面板配置变更钩子同步刷新工具块（打开时惰性兜底自愈）

**修复：绑定域名后工具 gating 失效（HostGuard 误拦内部端点）**
- 根因：HostGuard 全局包装器连同内部 gate/signon 鉴权端点一起拦，绑域名后 nginx 鉴权子请求 Host 不匹配 → 404 → 工具全挂
- 修复：内部 gate/signon 端点**最小化豁免** HostGuard/EntryPathGuard（仅 2 个路径前缀）；其安全仍由 loopback 源 + 秘密头 + 会话/令牌承载，**不放宽鉴权**；非内部路径强校验不变

**改进：部署失败原因可见**
- 任务详情不再只显示「DBT_012 部署操作失败」，带上具体原因（如端口占用/落点冲突），便于排查

### 安装与升级

```bash
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash
dougong version   # 应为 v0.1.16
```

升级后验证（数据库工具）：①IP 直连面板装 phpMyAdmin/mongo-express 不再报 DBT_012，自动建工具块；②**放行工具端口 18090**（防火墙/云安全组）后经 `http://IP:18090/dbtool/...` 打开（未登录/readonly 不可达）；③绑定面板域名后工具仅经 `http://域名:18090/...` 访问，IP 访问 404；④卸载工具清理自动块（不删数据库数据）。工具本体仍仅绑 localhost，18090 经面板会话 gating。

## v0.1.15 - 2026-11-19

修复 v0.1.14 数据库工具集成的三个实机问题（PHP-FPM 启动 / phpMyAdmin 扩展依赖 / mongo-express 服务重载）。

### 变更内容

**修复：PHP-FPM 服务启动报 `unit not found`（Ubuntu/Debian）**
- 根因：单元解析把「字面名 `php-fpm.service`」当命中——systemd `LoadUnit` 对不存在的单元也返回对象路径（`LoadState=not-found`，调用无错误），旧逻辑只判错误为空便用了字面名，**版本化单元扫描分支永远走不到**（真实单元是 `php8.3-fpm.service` 等）
- 修复：单元解析排除 `not-found`/空装载态，字面名不存在时落版本化正则扫描（`phpX.Y-fpm.service`）；RHEL 系 `php-fpm.service` 真实存在则直用不受影响

**修复：phpMyAdmin 安装报 PHP 扩展缺失（mysqli/mbstring/zip/gd/curl）**
- 根因：环境管理安装 PHP-FPM 只装 `php-fpm` 主包，不含 phpMyAdmin 必需扩展，且面板无补装扩展入口（引导成死路）
- 修复：PHP-FPM 安装/升级时**随主包一并安装必需扩展**（apt：php-mysqli/php-mbstring/php-zip/php-gd/php-curl；RHEL 系：php-mysqlnd/php-mbstring/php-gd，zip/curl 多内置）；best-effort（发行版差异致个别扩展包缺失不阻断主包，phpMyAdmin 依赖检测仍如实报告）

**修复：mongo-express 安装报 `daemon-reload … InvalidArgs`**
- 根因：systemd `Manager.Reload()` 是**无参方法**，代码误传了一个布尔实参 → DBus `InvalidArgs`，单元写入后重载失败致安装中止
- 修复：`DaemonReload` 去掉多余实参（Reload 零参数）

**修复：安装脚本选 English 后 `$2: unbound variable` 中断**
- 根因：`msg()` 在 `set -u` 下英文分支取 `$2`，而 4 处提示把中英文塞进单参数（中文走 `$1` 无恙、英文走 `$2` 崩溃）——故仅英文版中断
- 修复：`msg()` 改 `${2:-${1:-}}`/`${1:-}` 防 unbound（英文缺省回退）；4 处单参数提示拆为规范双语两参数

### 安装与升级

```bash
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash
dougong version   # 应为 v0.1.15
```

升级后验证：①环境管理安装 PHP-FPM 后可正常启动（Ubuntu 自动解析版本化单元），且 mysqli/mbstring/zip/gd/curl 扩展随装；②数据库工具页安装 phpMyAdmin 不再报扩展缺失；③安装 mongo-express 不再报 daemon-reload InvalidArgs，服务正常拉起。

## v0.1.14 - 2026-11-19

数据库**现成 Web 工具集成**（宝塔式）：面板一键部署 phpMyAdmin / mongo-express 作高级管理，复用环境管理已管理的 PHP/Node 运行时；访问经面板会话 gating，工具不直接公网暴露（新模块 db-tools，ADR-032）。

### 变更内容

**新增：数据库工具集成（db-tools）**
- **phpMyAdmin**（管 MySQL/MariaDB）：复用 PHP-FPM 环境，下载钉版本 + sha256 校验 → 部署 → 经面板 nginx 以 PHP 站点承载；PHP 扩展依赖检测（缺失引导补装）
- **mongo-express**（管 MongoDB）：npm 安装 → systemd 服务 → 面板 nginx 反代；目标实例选择 + 凭据从面板保险库取出注入
- 工具管理页：状态/版本/运行态/依赖就绪度，安装/升级/卸载（异步任务台账），卸载**不删除数据库数据**（强确认）
- 数据库治理页/库行新增「高级管理」入口（admin）；**面板内轻量表/文档浏览保留**作快速入口，两者并存

**新增：Node.js 环境纳入环境管理（env-mgmt v1.3）**
- 八环境目录 +Node.js（LTS 22.x 轨道，NodeSource 官方源三系代配，npm 随包）；版本经 `node --version` 探测
- Node 为运行时非 systemd 服务，无 start/stop 语义（服务操作如实拒绝）；为 mongo-express 与未来 Node 项目铺路

**安全：面板会话 gating 反代（不可妥协底线）**
- 工具**仅绑 localhost**，不暴露独立公网端口；唯一访问路径=面板域下工具路径 nginx 反代，经 `auth_request` 校验面板登录会话 + admin 角色（无会话/readonly→401）
- phpMyAdmin **signon 桥接**：HMAC-SHA256 令牌 + 60s 时效 + 一次性防重放 + 实例/会话双绑定；MySQL 凭据经 loopback 服务端注入，**永不进浏览器**
- mongo-express basic auth 面板生成经 nginx 服务端注入；凭据 AES-256-GCM 密文零回显
- 内部鉴权端点 loopback 源校验 + 秘密头双防护；下载为用户主动发起（零遥测例外）

**修复（实机反馈）**
- 实例清单工具菜单重复登记致侧栏双项 → 去重
- env_type 前端/文档同步八值（含 node），env-mgmt 页面可见 Node.js 安装入口

### 安装与升级

```bash
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash
dougong version   # 应为 v0.1.14
```

升级后验证（数据库工具）：①环境管理可安装 Node.js（LTS 22.x）与 PHP-FPM；②数据库工具页安装 phpMyAdmin（需要 PHP-FPM + 扩展）/mongo-express（需要 Node + 目标 Mongo 实例已纳管配凭据）；③「打开」经面板会话进入工具（未登录/readonly 不可达）；④卸载工具不删除数据库数据。工具仅绑 localhost 经面板反代，无需额外放行公网端口。

## v0.1.13 - 2026-11-19

修复 MongoDB 浏览系统集合报「参数错误」（v0.1.12 表级/文档治理增量实机热修）。

### 变更内容

**修复：MongoDB 文档浏览系统集合被误拦（AUTH_012）**
- 痛点：集合清单会列出 MongoDB 系统集合（`system.profile` / `system.js` 等），点「浏览文档」报 `AUTH_012 参数错误：集合名须非空且不含 …/system. 保留前缀`
- 根因：浏览/治理已存在集合的共用校验路径误用了**建集合专属**的 `system.` 保留前缀禁令——该禁令本意是「用户不可创建系统集合」，不该限制浏览真实存在的系统集合
- 修复：拆分校验——通用形态校验（非空/禁 `$` 前缀/禁空字符/长度上限，任何操作均禁）与建集合专属 `system.` 禁令分离；访问已存在集合（含存在性核验）只走形态校验，**系统集合可正常浏览**；建集合仍禁 `system.` 前缀（创建约束不变）
- 回归测试：浏览 `system.profile` 放行 + 形态校验仍拒空名/`$`/空字符 + 建集合仍拒 `system.`

### 安装与升级

```bash
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash
dougong version   # 应为 v0.1.13
```

升级后验证：MongoDB 库 → 浏览集合 → 点系统集合（如 `system.profile`）「浏览文档」不再报参数错误，可正常分页查看。

## v0.1.12 - 2026-11-18

数据库管理**体验与表级治理**大增量：实例清单不再被冗余连接困扰，MySQL 能管到表与行，MongoDB 不再只读，浏览有面包屑返回导航（api-database-mgmt v2.3.1，接口 30→44）。

### 变更内容

**改进：实例清单 TCP 优先纳管（实机反馈「为什么有好几个连接」）**
- 痛点：同一台本机 MySQL 的 socket 端点与 TCP 端点各登记一行，平铺呈现令人困惑（宝塔等面板均走 TCP，socket 管理极少见）
- 修复：探测改为 **TCP 优先两阶段**——本机 TCP 可达时 socket 不再登记为独立实例；仅当实例关闭 TCP（`skip-networking`）时 socket 才兜底发现（不丢发现能力）。清单常见情况天然一行 TCP
- 存量清理：重新探测时保守清理**无凭据**的冗余 socket 行（审计留痕）；**已配凭据**的 socket 行保留并标注，引导改配 TCP 后手动移除（不自动迁移凭据，避免丢失）

**改进：连接状态/凭据标签语义可见（实机反馈「绿条是什么」）**
- 「连接状态」绿/橙标签与「凭据」标签加悬停提示与图例：绿=可达（面板能连上）、橙=不可达；仅反映连通性，不代表凭据正确

**新增：MySQL/MariaDB 表级治理（实机反馈「为什么不能管理库里的表」）**
- 数据库行新增「浏览」入口 → **表清单**（表名/引擎/行数/数据大小/排序规则/注释）→ **表详情**（分页数据浏览 + 结构两 Tab）
- 行级治理：**单列主键**表支持行编辑/删除（无单列主键的表禁用并如实提示原因）；删表不可逆强确认
- 数据浏览经 JSON 逐行聚合通道，含换行/制表符的值不再解析脆弱

**新增：MongoDB 库/集合/文档治理（实机反馈「为什么只能只读」，废止 D-18 只读投影，ADR-031）**
- 数据库行「浏览集合」「删除库」（系统库保护）→ **集合清单**（新建集合/浏览文档/删除集合）→ **文档浏览**（JSON 分页 + 插入/编辑/删除）
- MongoDB 库随首集合写入产生（隐式建库语义），界面如实说明

**新增：顶栏面包屑返回导航（实机反馈「点浏览后怎么回去」）**
- 数据库 → 治理·实例标识 → 库 → 表/集合，逐级可点返回；动态段取实例档案与路由参数

**角色口径（沿 Redis key 浏览先例）**
- 表/集合/结构等**元数据**两角色（admin/readonly）均可看；**行数据/文档内容**与全部写操作 **admin 专属**，readonly 见「数据内容仅管理员可见」负空间

**修复：DBM_029 跨层错误码同步（A-DB13）**
- 引擎操作被拒（凭据账户权限不足）错误码后端/前端双侧对齐，Mongo 治理接口此类错误不再被静默吞掉；两个 Mongo 弹窗判定分支同步订正

### 安装与升级

```bash
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash
dougong version   # 应为 v0.1.12
```

升级后验证（数据库管理）：①实例清单同一本机服务仅一行 TCP（重新探测可清理冗余 socket 行）；②MySQL 库行「浏览」可见表清单/表数据/结构，单列主键表可编辑/删除行；③MongoDB 库行可浏览集合/文档并增删改、可删库；④进入治理/浏览页顶栏面包屑可逐级返回。已有实例与凭据不受影响（探测策略仅影响新登记与冗余清理，已配凭据 socket 行保留）。

## v0.1.11 - 2026-11-17

环境安装后**配置就绪**：装完数据库环境即可在面板直接治理，不再「装完 ≠ 可用」。

### 变更内容

**新增：环境安装后自动引导配置（env-mgmt + database-mgmt 联动，api-env-mgmt v1.2 / api-database-mgmt v2.2）**
- 痛点：经环境管理 apt 装完 MySQL 只有 root@localhost（auth_socket 无密码），面板走 TCP+密码连不上，用户无法自助建第一个账户（DBM_005 诊断轮终裁）
- 修复：MySQL/Redis/MongoDB 安装/升级成功后，worker 自动执行引导——建管理账户/设随机强密码并注册实例与凭据到数据库管理（**装完即可管**）：
  - **MySQL**：经 unix socket 以 root auth_socket 免密建 `dougong@localhost`（ALL PRIVILEGES WITH GRANT OPTION，**不动 root**）
  - **Redis**：CONFIG SET requirepass 随机密码 + CONFIG REWRITE 持久化（仅未设置时写）+ bind 本机校验
  - **MongoDB**：localhost exception 建 admin 库 root 角色用户
- 安全：随机强密码（crypto/rand）AES-256-GCM 密文落库、**永不回显**；账户限本机、bind 本机不公网暴露；引导动作审计留痕
- 幂等保守：已配置凭据/账户已存在/requirepass 已设/已有用户 → 跳过不覆盖用户已有配置
- 引导结果独立呈现：任务状态仍为包安装结果，新增引导结果维度（任务列表「引导」列 + 任务详情横幅）；**安装成功但引导失败**如实标注「已安装·引导失败」+ 原因 + 「重新安装/升级以重试引导」指引（不混淆「已装」与「可治理」）
- PHP/nginx/Apache/Docker 装完即用无引导

**随附（本批攒批，详见下方草稿节）**
- DBM_005 凭据弹窗 UX 小修：DBM_005/006 内联服务器原文 reason + 按引擎族排查指引 + 文案订正（database-mgmt v2.1，前端域内）
- ACME v2.4 链路修复：HTTP-01 webroot 迁出 0700 数据目录（v0.1.7）→ 挑战注入/复原后 reload nginx（v0.1.8）→ 存量纳管跳过面板托管路径影子行（v0.1.9）
- **证书弹窗 UI 三项修复（实机反馈）**：①申请/部署/解除部署/替换/DNS 凭证五个弹窗右上角 X 点击无反应——onClose 未回写受控 show，五弹窗统一补回写；②「DNS 凭证管理」标题去掉冗余括注；③新建站点/站点详情/证书申请三处域名输入由 select tag 形态改为**动态标签**（消除下拉箭头与「无数据」空态噪音，匹配「输入域名后回车添加」交互）

### 安装与升级

```bash
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash
dougong version   # 应为 v0.1.11
```

升级后验证（配置就绪）：在环境管理装 MySQL/Redis/MongoDB 后，系统自动创建管理账户并注册实例到数据库管理——打开数据库页即可直接治理（无需手动建第一个账户）；若引导失败，任务详情独立呈现「已安装·引导失败」与原因，重新安装/升级可幂等重试。已有环境/已有配置不受影响（保守跳过不覆盖）。

## v0.1.10 - 2026-11-17

reload 对存量坏 pid 文件鲁棒化：`nginx -s reload` 因 pid 文件空/缺失失败时，回退解析 master PID 直接 SIGHUP。

### 变更内容

**修复：reload 回退通道（nginxx v2.7，D-05 补强）**
- 实机根因：存量 nginx 的 `/run/nginx.pid` 为**空文件**——`nginx -s reload` 报 `invalid PID number ""`，systemd `ExecReload`（`kill -HUP $(cat /run/nginx.pid)`）同败，而 master 实际存活 → 部署/续期写盘的新配置永不生效（表现为「证书还是老的」；用户被迫 `pkill -9` 重启才生效）
- 回退链：`nginx -s reload` 失败且输出为 pid 文件类错误 → `systemctl show nginx.service --property=MainPID` → 否则扫 `/proc` cmdline 的 `nginx: master process` → 经 Go `os.Process.Signal(SIGHUP)`（无 shell，R-08）；回退成功在输出注明 master pid；解析/发信号失败保留原错误（D-05 配置保留语义不变）
- 回归锁：TestIsPidFileErr / TestMasterPidFromProc（procfs 目录注入，跨平台可跑）
- 服务器侧仍建议根治：查明 pid 文件为何为空（`grep -n pid /etc/nginx/nginx.conf`、`cat /etc/systemd/system/nginx.service.d/override.conf`），必要时 `systemctl restart nginx` 一次让 master 重写

### 安装与升级

```bash
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash
dougong version   # 应为 v0.1.10
```

## v0.1.9 - 2026-11-17

修复证书「存量导入」影子行：ACME 证书部署到站点后，存量自动纳管把面板托管证书文件又导入为一行「存量导入」（与 ACME 行同到期日）。

### 变更内容

**修复：存量纳管跳过面板托管路径（api-site-mgmt v2.6，A-SM5/FR-25 判重补全）**
- 根因：`importExistingCerts` 判重集只含 imported 行的 OriginPath；部署使站点 `ssl_certificate` 指向面板托管路径（ACME 行 StoragePath），该路径不在判重集 → 每次扫描新增一行影子「存量导入」
- 修复：判重集并入**全部证书行的 StoragePath**（托管路径一律不纳管）；回归锁 TestImportSkipsManagedStoragePath
- 已产生的影子行：升级后在证书列表直接删除该行即可（imported 删除=仅解除管理关系，不触碰任何文件），不再复发

### 安装与升级

```bash
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash
dougong version   # 应为 v0.1.9
```

## v0.1.8 - 2026-11-17

修复 ACME HTTP-01 持续 404 的最终根因：挑战配置写盘后**未 reload nginx**（v0.1.4~v0.1.7 实机全部命中）。

### 变更内容

**修复：挑战注入/复原后 reload nginx（site-mgmt）**
- 根因：`applyPipeline` 按 D-05 设计不含 reload（由调用方呈现），而 HTTP-01 挑战注入路径从不调用——挑战 location 写进了配置文件，**运行中的 nginx 仍是旧配置**：80 端口裸 `return 301` 把 LE 带到无挑战入口的 443 → 每次尝试均 404（fake 网关测试无法暴露，故连逃四个版本）
- 修复：注入写回成功后 reload（失败=复原+报错，不留半注入态）；cleanup 复原后 reload（best-effort，D-05 日志呈现）；单文件/dormant 与 v2.3 多文件两条路径均覆盖
- 回归锁：E2E 断言 reload ≥2 次（注入+复原）
- 面板证书 ACME 通道本就带 reload（panel/acme.go），仅 site-mgmt 挑战路径缺失

### 安装与升级

```bash
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash
dougong version   # 应为 v0.1.8
```

升级后对 renew_failed 证书点「更新证书」重试。HTTP-01 至此五层全通：同文件多块注入+跳转包裹（v2.2）→ 跨文件+catch-all 兜底（v2.3）→ webroot 公共可读（v2.4）→ **reload 生效（v2.5/v0.1.8）**；存量 HTTPS 域名仍推荐 DNS-01 最稳。

## v0.1.7 - 2026-11-17

HTTP-01 签发链路第二层根因修复：挑战 webroot 迁出 0700 数据目录（v0.1.6 修好注入后仍会 403 的隐藏缺陷）。

### 变更内容

**修复：ACME HTTP-01 webroot 迁移至 /var/www/dougong-acme（api-site-mgmt v2.4，D-19）**
- 根因：webroot 原为 `{数据目录}/acme-webroot`，而数据目录权限 0700 root——nginx worker（www-data/nginx）不可穿越，挑战文件永远读不到（UNIX 权限 403；RHEL SELinux 下 var_t 上下文同样拒绝 httpd 读取）
- 迁移至 `/var/www/dougong-acme`：Debian/RHEL 标准 httpd 内容目录（SELinux httpd_sys_content_t 覆盖），目录 0755、挑战文件 0644；certbot webroot 插件同族路径
- 三处派生同源收敛（acme 网关 / 面板证书通道 / site-mgmt），账户密钥、写前备份、托管证书仍留 0700 数据目录——安全面零变化
- 与 v0.1.6 的跨文件/默认兜底块注入（v2.3）叠加后，HTTP-01 覆盖：单文件多块、跨文件 80/443、全局 catch-all 重定向、worker 可读性四层全通

### 安装与升级

```bash
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash   # 重复执行即升级
```

升级后对 renew_failed 证书点「更新证书」重试。**若仍失败**请附：`dougong version` 输出 + `nginx -T 2>/dev/null | grep -B2 -A18 "server_name <你的域名>"` 片段，用于定位剩余拓扑。

## v0.1.6 - 2026-11-17

修复存量 nginx 拓扑下 ACME HTTP-01 签发 404（v0.1.5 修复未覆盖的两类拓扑，二次实机反馈）。

### 变更内容

**修复：HTTP-01 挑战注入覆盖全部存量拓扑（api-site-mgmt v2.3，D-19）**
- 实机报错：`Invalid response from https://<域名>/.well-known/acme-challenge/…: 404`——v0.1.5 的多块注入只覆盖 siteId 所在**单个文件**；两类常见存量拓扑仍落空：①80 与 443 server 块**分属不同 conf 文件**；②域名仅 443 在服、http 首跳落在**全局 catch-all 重定向块**（`listen 80 default_server; server_name _; return 301 https://…`，server_name 不匹配域名故被 v0.1.5 跳过）
- v2.3：注入范围扩至**全扫描跨文件**的所有匹配 server_name 块；匹配块中无 80 端口时追加注入 **80 默认兜底块**（default_server 优先、否则扫描序首个 80 块——LE 首跳实际落点）并包裹其 server 级跳转；逐文件经 R-06 安全管线写回、cleanup 逆序逐文件字节级复原
- 回归锁：TestCertRequestHTTP01CrossFile 与 TestCertRequestHTTP01DefaultCatchAll（onIssue 钩子快照注入态 + 复原断言）

### 安装与升级

```bash
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash   # 重复执行即升级
```

升级后对 renew_failed 证书点「更新证书」重试，或重新「申请证书」。国内服务器见 README「国内服务器安装」三通道（镜像前缀/指定版本/本地二进制）。

## v0.1.5 - 2026-11-17

数据库管理 MongoDB 支持轮 + 两项 v0.1.4 实测反馈修复（环境页刷新 / 凭据验证 INTERNAL）。

### 变更内容

**修复：数据库实例凭据保存/验证返回「内部错误」（阻断级）**
- 根因（代码实证）：凭据验证失败时引擎侧错误未映射业务码——当宿主机没有 `mysql`/`redis-cli` 客户端（如数据库跑在 Docker 容器、仅映射端口）或客户端输出未被识别的报错时，错误一路透传被兜底为裸 `INTERNAL`。此前版本在实例列表看到的「凭据验证失败：认证被拒/连接失败」提示在这类场景下根本走不到
- 与密码字符无关：经查全部通道为 `MYSQL_PWD`/`REDISCLI_AUTH` 环境变量注入子进程，密码含 `@`/数字/空格均不受影响（此前用户反馈中的猜测已证伪并在测试中固化）
- 修复：新增错误码 **DBM_021（引擎客户端通道不可用）**——明确提示「宿主机未安装 mysql/redis-cli」，与「网络不通」区分；凭据验证语境下未分类失败统一归 DBM_006；凭据弹窗内联呈现两类提示
- 附带修复：TCP 连接模式显式 `--protocol=TCP`——mysql 客户端对 `-h localhost` 会特例改走本机 socket 并忽略端口，容器/远端 TCP 实例此前连错通道

**修复：环境页任务完成后状态不刷新**
- 任务详情轮询到终态（成功/失败）即停止，但环境卡片仍显示旧的「已安装/版本/服务状态/任务进行中」。现任务一到终态自动重拉环境全景与任务列表；轮询纪律不变（执行中 5 秒、无后台长连接）

**新增：数据库管理支持 MongoDB**
- 实例发现：mongod 默认 socket（`/tmp/mongodb-27017.sock`）与 `localhost:27017` 端口探测，随实例清单统一纳管（凭据弹窗默认端口 27017）
- 凭据连接：SCRAM 认证（admin 库）；连接通道为进程内官方 mongo-driver（新增 Go 依赖，纯 Go 静态链接——**不依赖宿主机安装 mongosh**，从根上避开本轮 DBM_021 修复的「客户端缺席」缺陷类）
- 库列表：名称/大小/集合数只读投影（MongoDB 为隐式建库语义，不提供建/删库——与 MySQL 面的差异如实裁剪）
- 用户管理：创建（用户名/密码/首库首角色）/删除（连带回收全部角色）/库级角色三档（只读 read / 读写 readWrite / 库管理 dbAdmin）授予与回收
- 状态概览：serverStatus 白名单摘要（版本/运行时长/连接/内存/增删改查计数）
- 新增错误码 **DBM_022（引擎操作被拒）**：凭据账户权限不足（如查看 serverStatus 需 clusterMonitor 类权限）时如实提示，不再裸「内部错误」
- 集合/文档级浏览、mongodump 备份导出导入为二期（后续版本）

**内部改进（随本版带出）**
- 数据库接口引擎校验由两族重构为三族（MySQL 族/Redis/MongoDB）——重构前 Redis 系接口会误放行 MongoDB 实例
- 面板单二进制体积约 33MB → 41.7MB（mongo-driver 静态链接的预期代价）
- 证书：修复 HTTP-01 验证在服务器级 301 重定向之后的注入失效（多 server 块场景改注入到重定向目标块）
- 品牌标识：斗拱几何 Logo 上线（浏览器页签 / 侧栏 / 登录页 / 关于页），README 增加徽章行（Release / CI / License / Go 版本 / 下载量）

### 安装与升级

- 从 v0.1.x 升级：服务器上重跑安装命令即可（自动下载最新版、sha256 校验、原地替换二进制并重启 systemd 服务；数据目录 `/var/lib/dougong` 不动）
- 新安装：`curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | bash`
- 国内服务器：`DOUGONG_MIRROR=https://ghproxy.com sudo -E bash install.sh`
- 数据库迁移 011（实例档案表 engine 扩 MongoDB 值）随启动自动执行，既有实例档案与凭据密文不受影响

### 升级后建议验证

1. **凭据保存**：此前报「内部错误」的实例重新配置凭据——若宿主机缺客户端应看到明确提示「引擎客户端通道不可用（宿主机未安装 mysql/redis-cli）」，安装 `mysql-client`/`redis-tools` 后即可正常治理；密码含特殊字符不再有任何影响
2. **环境页刷新**：发起安装/升级/卸载后盯任务详情到终态——环境卡片的已安装状态/版本/服务态与「任务进行中」标记应自动落定，无需手动刷新
3. **MongoDB 全链**：环境页安装 MongoDB → 数据库页应自动发现实例（端口/socket 探测）→ 配置凭据（admin 库管理员账户）→ 库列表/建用户/角色授予回收
4. **权限提示**：若配置的凭据账户权限不足，状态概览等操作应提示「引擎操作被拒（凭据账户权限不足）」而非「内部错误」——serverStatus 需 clusterMonitor 类权限，用户/角色管理需 userAdmin 类权限

## v0.1.4 - 2026-11-17

环境管理升级链路热修（v0.1.3 服务器实测反馈）。

### 变更内容

- **修复环境升级在 conffile 冲突时失败**：升级中若新包携带的配置文件（如 `/etc/nginx/nginx.conf`）与本地已修改版本冲突，dpkg 会弹出交互式 Y/N 提示——无人值守环境下无 stdin 可读，直接失败（实机案例：Ubuntu 原生 nginx 1.24 经 nginx.org 源升 1.30 时卡死）。现采用无人值守标准三保险：`DEBIAN_FRONTEND=noninteractive` + `--force-confdef` + `--force-confold`——**本地已修改的配置保持现版不被覆盖**，发行版新版落 `.dpkg-dist` 旁置文件供事后对比
- **修复 MongoDB 源代配后签名验证失败（NO_PUBKEY）**：生成的 apt 单行源里 `arch` 与 `signed-by` 两个选项误用逗号分隔——apt 把整串并入 arch 值，`signed-by` 被静默丢弃，退回默认信任库后无法验证 MongoDB 官方源签名。已改为空格分隔并修正组件名（Ubuntu 用 `multiverse`、Debian 用 `main`，不再附加多余 `main`）；密钥本身无误（armored `.asc` 在 Ubuntu 24.04 实测可用）
- **修复刷新页面偶发裸 JSON 404**：九个页面路由与后端模块 API 前缀同名（`/env` `/sites` `/services` `/files` `/monitoring` `/terminal` `/firewall` `/docker` `/cron`），侧栏点击走前端路由无感，但**直接打开/刷新/收藏夹**这些地址会被后端按「模块前缀未知路径」吐 `NOT_FOUND` JSON 而非页面。现按浏览器导航特征（`Accept: text/html`）区分：导航回 SPA 页面，API 客户端维持 404 包络契约
- **修复未安装环境显示「已停止/自启关」**：systemd 的 LoadUnit 对不存在的单元也返回 `not-found` 形态的属性（inactive），被误映射为服务已停止。现该形态服务态如实为空，未安装环境不再渲染服务状态行
- 包操作命令由 `apt` 改为 `apt-get`（脚本场景稳定接口，消除 apt 自身的 WARNING 噪音）

### 安装与升级

- 服务器重跑安装命令即可（同 v0.1.3 方式；数据不动）
- **若你的服务器已卡在半配置状态**（升级报 `end of file on stdin at conffile prompt`），先手动解卡再升级面板：
  `sudo DEBIAN_FRONTEND=noninteractive apt-get -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" -f install`
  然后在面板重新发起升级，即可带新策略完成

## v0.1.3 - 2026-11-17

环境管理实机缺陷修复轮（v0.1.2 上线后的第一批服务器实测反馈）+ 面板 UI 打磨轮。

### 变更内容

**环境管理修复**
- 修复已安装服务「启用」报错 `org.freedesktop.DBus.Error.InvalidArgs`：目录表 systemd 单元名全部改为规范全名（如 `nginx.service`）——`EnableUnitFiles` 不像 `StartUnit` 那样自动补全后缀
- 修复 Docker / MySQL / Redis / PHP-FPM 卡片误显「探测不可用」：`dpkg-query` 对未装包的输出未被「未安装」判定覆盖，现改用包状态字段判定（不受系统语言影响），未装环境将如实显示灰色「未安装」
- 包名多候选检测：自装 `docker-ce`、`mysql-community-server` 等官方源形态可被正确识别为已安装
- 新增 **MongoDB 8.0** 环境（第七个环境）：自动配置 MongoDB 官方软件源（apt/dnf/yum 三系，含公钥下载与审计）后安装 `mongodb-org`

**面板 UI**
- 修复菜单图标重复：服务管理、文件管理及「用户与安全」四个子菜单此前因图标键未映射全部回退为同一兜底图标，现各有专属图标
- 清理提示文案：403 页内部术语（ADR-018）、站点空态、防火墙状态卡重复前缀、环境卡片「安装后可配合「X」模块使用」软引导行等
- 环境卡片描述去除模块括注；`/env/env/list` 响应移除 `relatedModule` 字段

**其他修复（随本版带出）**
- CLI / 服务端按已安装服务解析数据目录，不再依赖启动时的工作目录
- 证书：已有导入证书的域名允许发起 ACME 接管（CERT_003 语义修正）
- 证书页补充说明：导入证书为何不显示「更新证书」按钮
- 安装脚本：中英双语交互选择；大陆服务器下载路径友好化（镜像前缀 + 版本固定）

### 安装与升级

- **新安装**：`curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | bash`
- **从 v0.1.1 / v0.1.2 升级**：服务器上重跑同一条命令即可（自动下载最新版、sha256 校验、原地替换二进制并重启 systemd 服务；数据目录 `/var/lib/dougong` 不动）
- **国内服务器**（github 直连易被重置）：`DOUGONG_MIRROR=https://ghproxy.com sudo -E bash install.sh`；镜像不代理 API 时可固定版本：`DOUGONG_VERSION=v0.1.3`
- 数据库迁移 010（env_type 枚举扩 MongoDB）随启动自动执行

### 升级后建议验证

1. 环境管理页对已装 nginx 执行 启用/停用/启动 不再报错
2. 未安装的环境显示灰「未安装」而非橙「探测不可用」
3. MongoDB 安装全链（源代配 → 安装 → mongod 服务实况）
4. 若此前自装 docker-ce：Docker 卡片应显示已安装

## v0.1.2 - 2026-11-16

（本版 tag 曾按用户指示强制重指向，包含下列全部内容）

- 新增**环境管理**模块：nginx / Apache / Docker / MySQL / Redis / PHP-FPM 六环境安装·升级·卸载（apt 与 dnf/yum 双包管理器，异步任务台账）
- **站点列表聚合**：同配置文件的 80/443 server 块合并为一条目，可展开块清单
- **证书增强**：新增更新/替换证书；DNS-01 自动化验证（Cloudflare / 阿里云 DNS / DNSPod 凭证管理、自动 TXT、泛域名证书）
- **面板 IA 重构**：顶栏显示当前页标题；侧栏新增「用户与安全」分组（用户/会话/审计/个人安全，修复此前不可达）；设置页四分组重排；首启/登录页 dark mode 完整适配与文案净化
- 新增服务端管理 CLI：`dougong status / info / reset-password / set-port / entrypath / service`
- 数据库迁移 008/009；升级方式同上（重跑 install.sh）

## v0.1.1 - 2026-11-16

- 修复登录后侧栏整体消失（后端叶子菜单 `children:null` 挡崩前端渲染）
- CLI 优化：默认静默 gin 调试横幅（`DOUGONG_DEBUG=1` 恢复），新增 `dougong version`

## v0.1.0 - 2026-11-16

首个发布版本：单二进制面板（监控 / 服务 / 文件 / 网站 / 数据库 / 终端 / 防火墙 / 容器 / 计划任务与备份 / 账户与安全 十一模块），Linux amd64/arm64，一键安装脚本。
