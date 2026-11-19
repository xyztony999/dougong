# D0 澄清：db-tools 自动 nginx 落地（IP 直连可用，2026-11-19）

> 触发：用户实机 v0.1.15 反馈——phpMyAdmin 与 mongo-express 安装均 DBT_012。
> 取证根因：用户经 **IP+端口直连面板、未绑域名**，nginx 无面板 server 块，
> selectLanding（inject.go）0 匹配 → DBT_012「未发现面板域 nginx server 块」。
> 次生 UX 缺陷：worker finish 存 task.Error=err.Error() 仅「DBT_012: 部署操作失败」，
 **data.reason 被吞**，任务详情看不到具体原因（用户截图实证）。
> 模式：db-tools 增量（v1.3 设计 / api v1.3）。

## 一、根因与现状取证

- selectLanding 要求 nginx 存在「面板域」server 块：匹配 pa.BindDomain，或 proxy_pass
  指向面板监听 host:port（proxyTargetsPanel）。IP 直连 Go 服务（install.sh 形态）无此块 → 0 匹配。
- injectToolLocations 仅注入**已存在**块（ref.File+ref.Index），经 site-mgmt ConfigApply（写+nginx -t+reload）。
- toolBaseHost（deploy.go:689）：host=ref.ServerName||pa.BindDomain||requestHost；base=scheme://host（**无端口**，
  假定面板域经 nginx 80/443 承载）。IP:port 直连形态下工具路径无处承载。
- **cookie 跨端口共享**：会话 cookie host=IP（host-only，端口无关）→ 浏览器访问 IP:<工具端口> 同样携带 dg_session，
  gating auth_request 可校验。这是自动落地方案可行的关键。

## 二、拍板方案（主会话 D0，用户「continue」授权推进）

### Q1 自动落地块（核心，独立端口 18090 + 域名联动；用户 2026-11-19 二次拍板）
- selectLanding 0 匹配时**自动创建专用 nginx 工具 server 块**（不再直接 DBT_012 拒绝）：
  - 文件 {nginx conf.d}/dougong-tools.conf（编译期常量路径，沿 site-mgmt 扫描目录）；
  - `server { listen <ToolLandingPort>; server_name <跟随绑定状态>; ... }`；
  - **ToolLandingPort=18090**（编译期常量，避开常用端口；被占用则 DBT_012 如实报「工具端口被占用」引导释放）；
  - 绑定 0.0.0.0（IP 直连可达；**安全边界=gating auth_request**，工具本体 php-fpm/mongo-express 仍仅 localhost）。
- **域名联动（对齐宝塔「绑域名后仅域名访问」语义，用户前瞻性问题拍板）**：
  - 未绑域名：server_name _（IP/任意 Host 可达）；
  - 已绑域名：server_name=<BindDomain> + 非匹配 Host 返回 404（nginx `if ($host != ...) { return 404; }`
    或 default_server 兜底拒——镜像 HostGuard 语义）→ **绑域名后工具也只认域名，与面板一致**；
  - **域名变更同步**：panel 绑定/解绑域名时刷新自动块 server_name（panel 配置变更新增轻量钩子
    通知 db-tools 重注入；或惰性同步——工具 install/open 时按当前 BindDomain 校正块）。D1 裁定钩子 vs 惰性。
- 既有匹配块（绑域名/反代面板）优先沿用（不新建）；仅 0 匹配时自动建。多匹配仍 DBT_012 引导明确（不变）。
- siteRef +Port 字段（落地监听端口；既有 80/443 块 Port=0 表默认）+AutoCreated 标志。
- toolBaseHost：ref.Port!=0 → base=scheme://hostname:ref.Port（hostname 取 BindDomain 优先，否则 requestHost 去端口）；
  Port=0 沿旧形态（scheme://host）。Open URL 据此生成 http://IP:18090/dbtool/...（绑域名后 http://域名:18090/...）。
- 幂等：自动块已存在则复用（hasMarker/文件存在判定）；两工具共用一块（gate location 去重已有）。
- 卸载：移除工具 location；**两工具均卸载后移除自动块**（AutoCreated 标志判定；非自动块不动）。

### Q1b HostGuard 内部端点豁免（必修 bug，取证 main.go:488）
- HostGuard 是全局 net/http 包装器（包裹整个 gin 引擎），**误拦内部 gate/verify·signon/verify 端点**：
  绑域名后 auth_request 子请求 Host≠域名 → HostGuard 404 → gating 失效工具全挂。
- 修复：内部端点（loopback+秘密头防护链）**豁免 HostGuard**（HostGuard 包装器对内部路径前缀
  /dbtool/gate/、/dbtool/signon/ 直通放行；其安全由 loopback 源+秘密头+会话/令牌承载，Host 校验误伤）。
  EntryPathGuard 同核（内部端点不应受入口路径限制）。豁免范围最小化（仅内部 2 端点路径）。

### Q2 reason 可见（UX 修复）
- worker finish 错误文本须含 errcode data.reason（errcode.Error 取 Data["reason"] 拼接），
  任务详情可见具体失败原因（DBT_012 不再吞 reason）。沿 env-mgmt worker 错误形态先例核对。

### Q3 安全底线（不可妥协，沿 v1.4）
- 工具本体仅 localhost（php-fpm unix socket / mongo-express 127.0.0.1:8081）不变。
- 自动块 listen 18090 公网可达但**每请求 auth_request gating**（面板会话+admin，无会话/readonly→401）；
  gate 内部端点 loopback+秘密头双防护不变；signon HMAC+60s+一次性+双绑定不变；凭据 vault 零回显不变。
- 自动块写入经 site-mgmt ConfigApply（nginx -t 自检+reload，失败回滚字节复原 R-06）。
- 端口放行引导：安装成功输出/文档提示用户放行 ToolLandingPort（防火墙/云安全组）。

## 三、范围边界（本轮不做）
- 自动配置防火墙放行（firewall 模块联动归后续；本轮仅提示）。
- 工具端口用户可配（编译期常量 18090；可配归后续）。
- HTTPS 工具落地（自动块 http；TLS 归绑域名形态既有能力）。
- 面板自身经 nginx 承载的改造（不动面板 Go 直连形态）。

## 四、验收口径（供 Gate 1/Gate 2）
- selectLanding 0 匹配→自动建块（fake nginx 替身回归：listen 18090/server_name 跟随绑定/gate+tool location）；
  端口占用→DBT_012 reason；既有块优先；多匹配引导；幂等复用；卸载清块。
- **域名联动回归**：未绑 server_name _；绑定后 server_name=域名+非匹配 Host 404；绑定/解绑同步刷新块。
- **HostGuard 豁免回归**：绑域名后内部 gate/signon 端点（loopback+秘密头）仍可达（不被 HostGuard 404）；
  非内部路径 HostGuard 强校验不变（IP 直连面板仍 404）；豁免范围最小化。
- toolBaseHost Port 分支回归（IP:18090 / 域名:18090 URL）；Open URL 含端口。
- worker reason 可见回归（task.Error 含 reason 文本）。
- 安全专项无回归（gating 401/signon 防伪/vault 零回显/localhost 绑定/卸载不删数据）。
- 编译门控：go build/vet/test -race + vue-tsc/vite build/vitest + linux 交叉 + gofmt + dist 同步。
- Linux 实机冒烟：IP 直连面板装 phpMyAdmin/mongo-express 自动建块成功，经 IP:18090 打开（放行端口后），
  无会话访问 18090 工具路径→401。归目标环境。