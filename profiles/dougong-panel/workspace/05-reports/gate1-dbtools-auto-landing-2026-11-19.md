# Gate 1 验证报告 — 设计→代码门控（db-tools v1.3 自动 nginx 落地增量）

**模块**：db-tools（v1.3 增量轮，ADR-032 自动 nginx 落地）
**检查时间**：2026-11-19
**编号源**：engine-rules.md G1-01~G1-11（唯一编号源）；validation SKILL Gate 1 节为执行展开；S7/S8 为 design 侧预检编号（不混用）
**D0 基准**：00-architecture/d0-clarification-dbtools-auto-landing-2026-11-19.md（Q1 自动落地+域名联动 / Q1b HostGuard 内部端点豁免最小化 / Q2 reason 可见 / Q3 安全不变）

**基线文档版本**：api-db-tools.md v1.3 / prd-db-tools.md v1.2 / domain-model.md v1.26 / BA-db-tools v1.1 / AA-db-tools v1.2 / FL-db-tools v1.1 / PageSpec admin·readonly v1.2。

**检查范围**（ask 指定）：db-tools 本轮 D1-D4 增量产物——S7 Action↔API、三段式、枚举四方、DTO 可空、readonly 负空间、D0 拍板落地（Q1/Q1b/Q2/Q3）。代码取证为支撑抽查（实读 backend/ 现有 v0.1.15 基线，验证设计根因诊断与修复规约自洽）。

---

## 一、机械核验（本轮实跑）

脚本：`D:/tmp/gate1-dbtools-auto/check.py` 与 `enum.py`（`python -I` 隔离模式执行，UTF-8 输出）。

| # | 检查 | 命令/方法 | 结果 |
|---|------|----------|------|
| M1 | 三段式（G1-08） | check.py CHECK 1：正则提取 api 全部 `/dbtool/{resource}/{action}` 端点按段计数 | **PASS 11/11**——tool/{list,install,upgrade,uninstall}、task/{list,detail}、access/{open,reopen,rotate}、gate/verify、signon/verify 全三段；2 段命中=0（nginx 工具 location 前缀 /dbtool/phpmyadmin/·/dbtool/mongo-express/ 尾斜杠单段，非 API 端点，正则不误捕） |
| M2 | S7 Action↔API 双向 diff（G1-11） | check.py CHECK 2：api 3.3 矩阵面板接口 ↔ admin PageSpec 第 2 节 Action 集合 diff + 方法比对 | **PASS 9/9**——API-only=∅、Action-only=∅、方法不一致=∅；内部端点 2（gate/verify、signon/verify）未误列前端 Action（admin 第 2 节显式声明服务端调用方 nginx/PHP-FPM）；v1.3 零新接口 Action 集合零变更（dbtoolReopen 为 v1.2 W3 GET 重定向落地 Action） |
| M3 | readonly 负空间 | check.py CHECK 3：readonly 3 读 Action ∪ 6 负空间 == admin 9 全集；子集/不相交/全 GET 校验 | **PASS**——3 读（dbtoolList/TaskList/TaskDetail 全 GET）+6 负空间（Install/Upgrade/Uninstall/Rotate/Open/Reopen）=9/9，负空间∩可见=∅，missing=∅/extra=∅；含工具访问类 dbtoolOpen/dbtoolReopen（⚠️A10 admin 专属） |
| M4 | 枚举四方同值（G1-07） | enum.py：池（04-business-enum.md）↔ api 3.4 ↔ admin 第 3 节字典 ↔ domain-model `{...}` 取值集合比对（按未转义管道 split 修正） | **PASS**——db_tool_type{phpmyadmin,mongo_express}/db_tool_task_action{install,upgrade,uninstall}/db_tool_task_status{pending,running,succeeded,failed} 四方逐值相等；db_engine{mysql,mariadb,redis,mongodb} 池==api==admin==domain-model 并集（dm 拆 db_instance{mysql,mariadb,mongodb}+redis_instance{redis} 两行，并集=4 值）；env_type 八值（含 node）池==api==domain-model（admin 第 3 节注 env_type 为 env-mgmt 字典透传非本模块 Key，故 adm=None 非缺口）；panel_role 池==admin==domain-model（api 3.4 合并引用行「panel_role/audit_action/audit_result/audit_source（引用）」故 api=None 非缺口）。**v1.3 零枚举变更**（port 数值/autoCreated 布尔非枚举） |
| M5 | 池唯一性 | check.py CHECK 4b：标识列 Counter 重复检测 | **PASS**——duplicate=∅；db_tool_state 不采用行注明理由（04-business-enum.md:66） |
| M6 | DTO 可空显式 | check.py CHECK 5：正则提取 `字段: 类型\|null` | **PASS 11 字段**——installedVersion/boundInstanceId/running/runningTaskId/instanceId/startedAt/finishedAt/error/tokenExpiresAt（既有 9）+**landingPort/autoCreated（v1.3 additive）**；api 3.1 声明「JSON null ≠ TS undefined，后端 *T 无 omitempty」纪律（api-db-tools.md:129）；v1.3 可空语义=未装 null/null、已装 siteRef 投影（旧档案缺省 port=0/autoCreated=false 不虚构非零端口）；admin 第 8 节检查 11 null 呈现含 landingPort/autoCreated（db-tools.md:202） |
| M7 | 错误码族 | check.py CHECK 6：DBT_(\d{3}) 集合连续性 | **PASS**——DBT_001~012 连续唯一无缺号（1..12 True）；**v1.3 零新错误码**（DBT_012 reason 细分文案 D-19/D-22）；复用 AUTH_001/002/003/008；内部端点拒绝语义显式（gate 裸 401/signon 包络 DBT_007/008/010/loopback·秘密头失败裸 404，api-db-tools.md:189） |
| M8 | changelog（G1-09） | check.py CHECK 7 + arch 三文档 grep：表头版本 vs 最新 changelog 行 | **PASS 8/8**——api v1.3（行 v1.0~v1.3）/admin v1.2/readonly v1.2/prd v1.2/domain-model v1.26（:475 最新行）/BA v1.1（:273）/AA v1.2（:397）/FL v1.1（:228），表头==最新行 |
| M9 | 占位符扫描 | check.py CHECK 8：`grep -nE "TODO\|TBD\|待填\|占位\|FIXME"` 7 份 in-scope 文件 | **PASS**——0 命中 |

## 二、G1-01~G1-11 逐项

| 编号 | 检查项 | 结果 | 依据 |
|------|--------|------|------|
| G1-01 | 架构文档与模块设计一致 | PASS（I1/I2 措辞滞后） | BA v1.1 L4 2.2.1.5 工具落地自动供给（业务活动 10→11，L2/L3 集合不变，business-arch-db-tools.md:129/:273）== AA v1.2 C-15-05（子模块 10→11，application-arch-db-tools.md:142/:397）== FL v1.1 零新增菜单（admin 8/readonly 2，机制型 3→4，function-list-db-tools.md:35/:228）== PRD FR-11/12（C-15-05）/FR-13（panel-core 机制侧）/FR-14（C-15-04 扩展）；安全五道防线 BA 2.4（:157-196）↔ AA 附录C（:377-389）↔ PRD 第五部分末映射表（:147-155）三层同构（v1.2 四→五）；编码 C-15/S-58/S-59 不变 |
| G1-02 | PRD 覆盖业务场景与角色 | PASS | prd-db-tools v1.2 完备性 G 5/5、T 适用 8/8（T3/T5 N/A 有据）、N 4/4（:305-348）；IN 10→14/OUT 13→17（+自动防火墙放行/端口可配/HTTPS 工具落地/面板 nginx 化 4 OUT）；N2 20→27 异常分支（+端口占用/多候选/钩子刷新失败/惰性校正失败/绑域名非匹配 Host/内部端点豁免边界/清块判定边界，:238-244）；角色 admin/readonly 零变化（自动落地机制型无新角色面）；Q1/Q1b/Q2/Q3 全落 FR-11~14 |
| G1-03 | API 设计完整 | PASS | 11 端点路径/方法/权限/CSRF/请求/响应/错误码齐备（api 3.3 矩阵+4.1.4/4.2.4/4.4）；**v1.3 零新 HTTP 接口**（自动落地/域名联动/reason 为 worker/钩子/错误文本内部机制，3.1 接口面变更纪律行:130）；2 表 DDL 迁移 013 形态不变（5.3，零 DDL 零新迁移:646）；23 设计决策 D-01~D-23（+D-19~D-23）；常量 +TOOL_LANDING_PORT=18090/TOOL_LANDING_CONF_NAME/TOOL_LANDING_BIND=0.0.0.0/INTERNAL_EXEMPT_PREFIXES（3.4:230-233） |
| G1-04 | 交互覆盖页面类型 | PASS | 1 hybrid 页面 /db-tools（卡片列表+任务 Drawer，admin 第 4 节）；v1.2 +访问形态区块（landingPort/autoCreated 投影）+安装成功放行 18090 提示+任务 reason 全文呈现+打开 URL 端口口径；表单=即时 Dialog 无独立路由（第 5 节）；详情=Drawer（第 6 节）；审批 N/A；readonly 同构只读视图（访问形态/reason/放行提示只读呈现同等） |
| G1-05 | API 字段与交互规格对齐 | PASS | 卡片访问形态区块 landingPort=18090→地址+放行提示/=0→面板域路径/=null→不渲染（admin 4.1:89）逐项源自 tool/list additive 字段（api:301）；任务详情 error 含 data.reason 全文（admin 4.3:144 ↔ api D-22:433）；打开 URL 后端权威下发前端不拼接（admin 4.4:155 ↔ api D-23:305）；null 呈现对齐（M6）；卡片展示 hostname=window.location.hostname 为引导，实际 URL 以 dbtoolOpen 响应权威（BindDomain 优先）——口径显式声明自洽 |
| G1-06 | 状态机完整 | PASS | db_tool_task_status 四态不变（pending→running→succeeded/failed + 重启恢复，api 4.2.2:410-415）；running→failed 触发 +自动建块失败（端口占用/配置目录不可用/块写入 nginx -t 不通过/清块失败，D-19）；error 文本含 data.reason（D-22）；**仍 1 个状态机**——prd 第六部分辨析 6→9（+自动落地块生命周期/域名联动同步/AutoCreated 标志三项非状态机，:178-180）；db_tool 无状态机辨析不变 |
| G1-07 | 枚举统一无冲突 | PASS | M4 四方同值 + M5 池唯一；v1.3 零枚举变更（AutoCreated 布尔/Port 数值非枚举不入池，domain-model:215/:298 与 totp_enabled 同口径）；db_tool_state 不采用理由注明（池:66） |
| G1-08 | 接口路径规范 | PASS | M1 11/11 三段式 /dbtool/{resource}/{action}（D-01 资源段裁定）；v1.3 路径零变化 |
| G1-09 | changelog 已填写 | PASS | M8 8/8 表头==最新行；api v1.3 变更摘要详实（6.1:657 rule_added+field_added+interface_changed，目标节号齐）；prd v1.2/domain-model v1.26/BA v1.1/AA v1.2/FL v1.1/admin·readonly v1.2 增量行均在位 |
| G1-10 | INDEX 模块状态已更新 | PASS | INDEX db-tools 行 stages requirement/api/interaction=done（v1.3 增量 D0-D4 全链完成注记详实，INDEX:38-40）、backend=done（v0.1.15 集成基线 T-DBT1~8）、frontend/validation=todo 与本 Gate 时点一致；gates.gate1 由本 Gate 更新（warn→warn，v1.3 增量复检注记）；**v1.3 自动落地代码面（D-19~D-23）尚未实现**（现有 backend 仍 v0.1.15 形态，见四取证），设计 D0-D4 完成待 B 阶段——与 Gate 1 时点（D4 后 B1 前）一致 |
| G1-11 | 跨阶段模型一致性 | PASS（W1 跨模块待同步） | domain-model v1.26 落地引用 siteRef 值对象显式建模（:214/:298）+Port/AutoCreated ↔ api 4.1.1 site_ref JSON 载荷 {file,index,serverName,scheme,port,autoCreated}（:253）↔ 5.3 DDL site_ref TEXT 列不变（:621）逐字一致（零 DDL）；db_tool/tool_deploy_task 字段↔api↔DDL 一致；关联领域对象仍 db_instance 一项（panel_config.bind_domain 实况值消费非 ID 引用，domain-model:356）；S7 9/9（M2）；存储映射零 DDL 注（v1.26）；**panel-core 提供方登记待同步（W1）** |

## 三、D0 四拍板落地核验

### Q1 自动落地+域名联动 — PASS

| 要素 | 显式落点 | 自洽性核验 |
|------|---------|-----------|
| 自动建块 | api D-19（:49）：0 匹配自动建 {nginx 惯例目录}/dougong-tools.conf listen TOOL_LANDING_PORT=18090 绑定 0.0.0.0；目录=nginxx PriorityDirs() 首个实存目录（gateway.go:82 实存）；端口探针 ProbePortAvailable net.Listen 即探即放占用→DBT_012 reason 不静默换端口；ConfigApply(allowNew=true) R-06；幂等复用两工具共用 gate 去重 | ✓ D0 Q1「0 匹配自动创建专用块/端口占用如实/既有块优先/多匹配引导/幂等」全落；D-03 修订（:33）无候选→自动建块不再直接 failed；2.3 编排+自动建块行（:111）；I-C04（:101）自动块与档案一致 |
| 域名联动 | api D-20（:50）：未绑 server_name _ /已绑 BindDomain+`if ($host !~* "^{BindDomain}$") { return 404; }` 镜像 HostGuard stripPort+EqualFold（hostguard.go:40 实读确认）；钩子主通道 ConfigChangeHook.OnBindDomainChanged（:344）+install/open 惰性校正兜底；钩子失败不阻断配置变更（panel_config 权威）；一致零写入幂等；BindDomain RFC1123 校验+转义正则元字符防注入（4.4.4:538） | ✓ D0 Q1 二次拍板「对齐宝塔绑域名后仅域名访问」；D1 裁定双通道（AA 附录B 待对齐-3 关闭:373）；nginx $host 不含端口+大小写不敏感全等==HostGuard 语义自洽；2.3 域名联动刷新行（:112） |
| 清块 | api D-19⑥（:49）/D-15（:304）：两工具均卸载且 siteRef.autoCreated=true→ConfigRemove 整块；非自动块绝不动（不误删用户站点）；块已不存在视为成功（幂等） | ✓ D0 Q1「两工具均卸载后移除自动块（AutoCreated 判定，非自动块不动）」；I-C04/I-14（:268）；2.3 卸载清块行（:113） |
| toolBaseHost 端口分支 | api D-23（:53）：port≠0→{scheme}://{hostname}:{port}（hostname=BindDomain 优先否则 requestHost 去端口）；port=0 沿旧形态；open URL/reopen 302 Location 同口径 IP:18090/域名:18090；dg_tool_instance cookie Path 路径级端口无关 | ✓ D0 Q1「Open URL 据此生成 http://IP:18090/...」；取证 deploy.go:689-701 既有 toolBaseHost 无端口分支（实读确认 `return ref.Scheme+"://"+host`）；cookie host-only 端口无关跨端口可达（D0 取证关键） |
| siteRef 扩展 | api 4.1.1（:253）site_ref JSON +port/autoCreated（TEXT 列不变零 DDL，旧档案缺省 port=0/autoCreated=false parseSiteRef 零值向后兼容）；domain-model v1.26 值对象显式建模（:214/:298） | ✓ 取证 inject.go:20-25 现有 siteRef{File,Index,ServerName,Scheme} 无 Port/AutoCreated（实读确认）——设计 additive 扩字段正确，json.Unmarshal 缺字段零值兼容自洽 |
| 范围边界 | api 1.2 ⑪~⑭（:24）/PRD OUT-14~17（:61-64）：自动防火墙放行 OUT（仅提示）/工具端口可配 OUT（18090 编译期）/HTTPS 工具落地 OUT（自动块 http）/面板自身 nginx 化 OUT | ✓ D0 第三部分范围边界四项全落 OUT；端口放行提示 D-19⑦（安装成功输出尾部，仅提示不自动放行） |
| 上游同步 | BA L4 2.2.1.5/⚠️A14/A15、AA C-15-05/附录B 边界表+4 行、FL 零新增菜单、PRD FR-11/12、domain-model siteRef 值对象 | ✓ 六处同值；FL admin 8/readonly 2 不变（机制型无入口） |

### Q1b HostGuard/EntryPathGuard 内部端点豁免最小化 — PASS

| 要素 | 显式落点 | 自洽性核验 |
|------|---------|-----------|
| 取证 | api D-21（:51）/4.4.5（:542-554）：cmd/dougong/main.go:487-488 EntryPathHandler→HostGuard 全局 net/http 包装器包裹整个 gin 引擎作用于全部路由 | ✓ **实读确认** main.go:487-488 `handler := middleware.EntryPathHandler(panelSvc, middleware.HostGuard(panelSvc, server.Handler()))`（Entry→Host→gin 链）；绑域名后子请求/回调 Host≠域名→HostGuard 404→gating 失效诊断准确 |
| 豁免最小化 | INTERNAL_EXEMPT_PREFIXES=/dbtool/gate/,/dbtool/signon/（编译期常量，仅内部 2 端点路径前缀，api 3.4:233/4.4.5:548） | ✓ D0 Q1b「豁免范围最小化（仅内部 2 端点路径）」；前缀匹配含子路径当前各仅 verify 一动作 |
| HostGuard 豁免 | 规范路径命中豁免前缀→直通放行不做 Host 校验；**非内部路径强校验不变**（bind_domain 非空 Host 去端口全等不匹配含 IP 直连标准 404，fail-closed 沿 A-P7） | ✓ hostguard.go:40 EqualFold(stripPort(r.Host),bindDomain)+:26 fail-closed 404 实读确认；豁免不削弱非内部路径强校验 |
| EntryPathHandler 豁免 | 携入口前缀剥前缀放行（既有语义）+未携前缀但路径直接命中豁免前缀亦直通（防御性兜底，内部调用方 URL 含 entryPath 烘焙 A-DBT6） | ✓ D0 Q1b「EntryPathGuard 同核」；入口路径变更窗口/烘焙时效内回调仍可达 |
| 豁免边界（安全不削弱） | **豁免的是 Host/入口路径校验而非鉴权**——LoopbackGuard+InternalSecretGuard+会话/令牌防护链全量不变（D-11/4.4.1/4.4.2）；非 loopback/无秘密头仍裸 404 | ✓ module.go:208 loopbackGuard（RemoteAddr 127.0.0.1/::1 否则裸 404）/:225 internalSecretGuard（X-Dbtool-Internal）实读确认；gating 会话+admin/signon 令牌四重不放宽 |
| 落点与验收 | 落点归 panel-core（middleware/hostguard.go+entrypath.go，S-07-02/S-07-03）跨模块待同步项本轮不代改；4.4.5 验收五条（绑域名后内部端点可达/非内部路径强校验不变/豁免最小化/无 loopback·秘密头仍 404/入口路径携·未携前缀均可达） | ✓ D0 Q1b 拍板；PRD FR-13/⚠️A16、AA S-59-01/S-59-02 豁免注记、BA 防线二扩展三层同构；**panel-core 提供方登记待同步（W1）** |

### Q2 reason 可见 — PASS

| 要素 | 显式落点 | 自洽性核验 |
|------|---------|-----------|
| 取证 | api D-22（:52）：deploy.go:200-204 `msg := opErr.Error()` 仅「DBT_012: 部署操作失败」（errcode.Error()=symbol+message，errcode.go:28）吞 data.reason | ✓ **实读确认** deploy.go:203 `msg := opErr.Error()`；errcode.go:28 `func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.symbol, e.message) }`+Data() any（:38）——诊断准确 |
| 修复规约 | error=opErr.Error()；errors.As 取 *errcode.Error 且 Data()["reason"] 非空 string→追加 ": "+reason（最终「DBT_012: 部署操作失败: 工具端口 18090 被占用，请释放后重试」）；无 reason 沿 symbol+message 不虚构 | ✓ D0 Q2「errcode.Error 取 Data["reason"] 拼接」；DTO 形状不变文本内容扩展（task/list·detail error 字段，api:189/:433） |
| reason 细分 | DBT_012 reason 细分文案（端口占用/多候选清单/扫描目录失败/自动建块写入失败/域名联动刷新失败/清块失败，api 3.4:186） | ✓ 各执行步骤 WithData reason 如实携带；重启恢复 error=「面板重启中断」不变（I-09） |
| 先例核对 | 沿 env-mgmt worker 错误形态先例（worker.go:88/117/128） | ✓ **实读确认** env-mgmt worker.go:88 "第三方源代配失败: "+verr.Error()/:117 "刷新包索引失败: "+err.Error()/:128 err.Error()——错误文本含具体原因先例成立 |
| 上游同步 | PRD FR-14/⚠️A17、AA C-15-04、BA 防线四扩展；PageSpec admin 4.3 error 全文呈现+DBT_012 拦截器 reason 细分引导（:144/:22），readonly 同等可看（:38） | ✓ 三层同构+前端呈现兑现 |

### Q3 安全底线（不可妥协，沿 v1.4）— PASS（安全约束零削弱）

| 要素 | 显式落点 | 自洽性核验 |
|------|---------|-----------|
| 工具本体仅 localhost | api D-07（ME_CONFIG_SITE_BIND_ADDRESS=127.0.0.1:8081）/D-13 常量/⚠️A6；phpMyAdmin 无独立端口（PHP-FPM pool FastCGI） | ✓ D0 Q3「php-fpm unix socket/mongo-express 127.0.0.1:8081 不变」；BA 防线一（工具进程无公网端口；18090 为 nginx 接入层非工具进程端口，PRD OUT-10 修订:57） |
| 自动块每请求 gating | api D-19⑧/D-11/4.4.1：自动块公网可达但每请求 auth_request gating（面板会话+admin，无会话/readonly→401） | ✓ D0 Q3「自动块 listen 18090 公网可达但每请求 auth_request gating」；BA 防线五（:190-196）；readonly 经 IP:18090 直访同样 gating 401（readonly PageSpec 4.3:49） |
| gate 内部端点双防护 | api D-11/4.4.1：loopback 源校验裸 404+X-Dbtool-Internal 秘密头 0600+不挂会话 CSRF 中间件 | ✓ module.go:208/:225 实存；自动块形态防护链全量不变（4.4.5 豁免边界） |
| signon 防伪 | api D-09/4.4.3：HMAC-SHA256+TTL 60s+一次性 jti 内存台账+instanceId/sessionDigest 双绑定；四重校验→DBT_007/008/010 | ✓ D0 Q3「signon HMAC+60s+一次性+双绑定不变」；自动块形态全量适用（BA 防线五:196） |
| 凭据 vault 零回显 | api I-02（cred_cipher 任何响应/日志/审计不含明文/密文/密钥）+D-08/D-09/D-10 | ✓ D0 Q3「凭据 vault 零回显不变」；BA 防线三 |
| ConfigApply R-06 | api D-19⑤：自动块写入经 ConfigWritePipeline.Apply（nginx -t 自检+reload，失败回滚字节复原 R-06） | ✓ D0 Q3「自动块写入经 site-mgmt ConfigApply（nginx -t+reload，失败回滚 R-06）」；configpipeline.go:18 ConfigApply/:29 ConfigRemove 实存（既有 OHS 零新增契约）；BA 防线五不产生半配置 |
| 端口放行引导 | api D-19⑦：安装成功输出尾部含「请放行 TCP 18090 端口（防火墙/云安全组）」（仅提示不自动放行 OUT） | ✓ D0 Q3「端口放行引导」；PageSpec admin 4.1 安装成功提示（:97） |
| 五道防线三层同构 | BA 2.4（:157-196）↔ AA 附录C（:377-389）↔ PRD 第五部分末映射表（:147-155） | ✓ v1.2 四→五（+防线五自动落地块安全）；防线一/二/四扩展；安全验收五专项（自动落地/域名联动/HostGuard 豁免/Port URL/reason）三层一致 |

## 四、代码取证（支撑抽查，实读 backend/ 现有 v0.1.15 基线）

> 目的：验证设计根因诊断与修复规约相对现有代码自洽（Gate 1 设计面检查；v1.3 修复代码实现归 B 阶段）。

| 项 | 核验命令/方法 | 结果 |
|----|--------------|------|
| HostGuard 全局包装（Q1b 取证） | `sed -n '480,495p' backend/cmd/dougong/main.go` | ✓ :487-488 `EntryPathHandler(panelSvc, HostGuard(panelSvc, server.Handler()))` Entry→Host→gin 链作用于全部路由——D-21 诊断准确 |
| HostGuard 语义（Q1 域名联动镜像） | `grep -n EqualFold/stripPort/StatusNotFound backend/internal/api/middleware/hostguard.go` | ✓ :40 `strings.EqualFold(stripPort(r.Host), bindDomain)`+:26 fail-closed 标准 404+:51 stripPort——D-20 nginx if 写法镜像自洽 |
| worker finish 吞 reason（Q2 取证） | `sed -n '195,210p' backend/internal/service/dbtools/deploy.go` | ✓ :203 `msg := opErr.Error()` 仅 symbol+message——D-22 诊断准确 |
| errcode.Error/Data（Q2 修复基础） | `sed -n '25,40p' backend/internal/errcode/errcode.go` | ✓ :28 Error()=fmt.Sprintf("%s: %s",symbol,message)+:38 Data() any——D-22 errors.As 取 Data()["reason"] 可行 |
| toolBaseHost 无端口分支（Q1 取证） | `sed -n '685,705p' backend/internal/service/dbtools/deploy.go` | ✓ :689-701 `return ref.Scheme+"://"+host, false` 无端口分支——D-23 诊断准确 |
| siteRef 现有结构（Q1 additive 基础） | `sed -n '15,30p' backend/internal/service/dbtools/inject.go` | ✓ :20-25 siteRef{File,Index,ServerName,Scheme} 无 Port/AutoCreated；parseSiteRef json.Unmarshal 缺字段零值——D-19 additive 扩字段零 DDL 向后兼容自洽 |
| ConfigApply/ConfigRemove（Q1/Q3 管线） | `grep -n func.*Apply/Remove backend/internal/service/sitemgmt/configpipeline.go` | ✓ :18 ConfigApply(ctx,file,newContent,allowNew bool)/:29 ConfigRemove(ctx,file)——既有 OHS 零新增契约（D-19/D-20） |
| PriorityDirs（Q1 目录选择） | `grep -n func.*PriorityDirs backend/internal/adapter/nginxx/gateway.go` | ✓ :82 PriorityDirs() []string——D-19 自动块目录选择基础实存 |
| 内部端点防护链（Q3 不变） | `grep -n Loopback/InternalSecret/RemoteAddr backend/internal/api/dbtools/module.go` | ✓ :208 loopbackGuard（127.0.0.1/::1 否则裸 404）/:225 internalSecretGuard（X-Dbtool-Internal）——D-11/D-21 豁免后防护链不变实存 |
| env-mgmt worker reason 先例（Q2） | `grep -n Error()/reason backend/internal/service/envmgmt/worker.go` | ✓ :88/:117/:128 错误文本含具体原因——D-22 沿先例成立 |
| panel-core 提供方登记（W1 核验） | `grep -n ConfigChangeHook/OnBindDomainChanged/INTERNAL_EXEMPT/dbtool豁免 workspace/02-detailed/api/api-panel-core.md`（v2.2）+ prd/BA-panel-core | ✗ **无命中**——api-panel-core.md v2.2 仅 D-P03 EntryPathGuard 通用描述（:31），未登记 db-tools 配置变更钩子提供（S-11-01）/HostGuard·EntryPathGuard 内部端点豁免（S-07）；prd-panel-core/BA-panel-core 亦无——**跨模块待同步项尚未兑现（W1，设计显式声明本轮不代改）** |

## 五、WARN 项（非 CRITICAL，待用户确认）

- **W1 panel-core 跨模块待同步项未兑现**：db-tools v1.3 两条 panel-core 机制侧依赖——①配置变更通知钩子提供（ConfigChangeHook.OnBindDomainChanged，S-11-01，域名联动主通道）②HostGuard/EntryPathGuard 内部端点豁免（INTERNAL_EXEMPT_PREFIXES，S-07-02/S-07-03，必修 bug）——db-tools 侧需求与消费契约已显式登记（api D-20/D-21/4.4.5/5.1，PRD FR-12/13，AA 附录B），但**提供方 api-panel-core.md v2.2 尚未登记**（实查 grep 无命中，prd/BA-panel-core 同）。设计显式声明「panel-core 三文档同步登记为跨模块待同步项，本轮不代改」（沿 database-mgmt 高级入口/env-mgmt Node 先例）。**风险**：Q1b 豁免为必修 bug——panel-core 机制侧未实现前，绑域名后 gating/signon 内部端点仍被 HostGuard 404 致工具全挂；域名联动钩子主通道未提供前仅惰性兜底生效。建议 panel-core 增量轮兑现提供方登记+机制实现（db-tools B 阶段前置依赖）。非 db-tools 设计契约断裂，不阻断本 Gate。

## 六、INFO 项

- **I1 AA S-59 节 intro 措辞滞后**：application-arch-db-tools.md:204「安全架构四道防线见附录C」——附录C v1.2 已升五道防线（:377 标题「五道防线〔v1.2 四→五〕」），S-59 节 intro 仍「四道」。措辞小滞后（约束本身五道齐全），建议回填「五道」。
- **I2 PRD FR-03 建站措辞承前轮 W2①未回填**：prd-db-tools.md:90（FR-03）仍「经 site-mgmt 建 php_fpm 站点」/「建 reverse_proxy 站点」——api D-03 已裁定面板域 server 块持久 location 注入（不落独立站点）。承前轮 gate1-db-tools-2026-11-19.md W2①措辞滞后，非 v1.3 新增；契约本身自洽（api 为权威下游），建议下一轮 prd 措辞回填。
- **I3 v1.3 代码面待 B 阶段**：自动落地/域名联动/HostGuard 豁免/reason 可见/toolBaseHost 端口分支（D-19~D-23）设计 D0-D4 完成，**现有 backend 仍 v0.1.15 形态**（四取证实读：toolBaseHost 无端口分支/deploy.go:203 吞 reason/siteRef 无 Port·AutoCreated/HostGuard 无豁免）——修复实现归 B 阶段（INDEX backend=done 为集成基线，v1.3 增量代码待落）。编译门控/实机冒烟属 Gate 2，本轮未执行如实声明。
- **I4 FX 全量清单未随引擎 payload 发布**：PageSpec 以第 8 节自查替代（admin 15 项/readonly 5 项，全仓先例）；本 Gate 按 G1 编号源+S7/S8 机械核验执行，FX 逐条归 Gate 2/前端栈。
- **I5 dict/batch 偏差**：5 字典 Key 前端 TS 常量（单一事实源枚举池）已声明（admin 第 3 节），沿全仓既有偏差先例。

## 七、总结

**判定：WARN**（CRITICAL 0 / 契约断裂 0）

- 机械核验 M1~M9：9/9 PASS（三段式 11/11、S7 9/9 双向零差、readonly 3+6=9 全集、枚举四方同值、池唯一、DTO 可空 11 字段含 v1.3 additive、DBT_001~012 连续、changelog 8/8、占位符 0）。
- G1-01~G1-11：11/11 PASS（G1-01 带 I1/I2 措辞滞后注记、G1-11 带 W1 跨模块待同步注记，不构成 db-tools 设计契约断裂）。
- D0 四拍板：Q1 ✓（自动建块/域名联动/清块/toolBaseHost 端口分支/siteRef 零 DDL 扩展/范围边界全落，六处上游同值）/ Q1b ✓（豁免最小化 2 前缀、豁免 Host/入口路径而非鉴权、非内部路径强校验 fail-closed 不变、4.4.5 验收五条，取证 main.go:487-488 实读确认）/ Q2 ✓（errors.As 取 data.reason 追加、DBT_012 细分、DTO 形状不变、沿 env-mgmt 先例实读确认）/ Q3 ✓（localhost/每请求 gating/signon 防伪/vault 零回显/ConfigApply R-06/端口放行提示全量适用，五道防线三层同构，安全零削弱）。
- 代码取证 10 项实读确认设计根因诊断与修复规约自洽；panel-core 提供方登记实查无命中（W1）。
- WARN W1（panel-core 跨模块待同步项）待用户经主会话确认/panel-core 增量轮兑现；INFO I1~I5 记录在案。确认后 Gate 1 记 pass，进入 B1（backend v1.3 增量）。
- 未执行项如实声明：v1.3 自动落地代码实现（D-19~D-23）归 B 阶段本轮未执行（I3）；编译门控/实机冒烟属 Gate 2 本轮未执行；panel-core 提供方登记归其增量轮（W1）。

**报告路径**：05-reports/gate1-dbtools-auto-landing-2026-11-19.md