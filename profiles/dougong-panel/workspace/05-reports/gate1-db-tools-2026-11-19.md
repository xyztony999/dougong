# Gate 1 验证报告 — 设计→代码门控（db-tools v1.0/v1.1 + env-mgmt Node 增量）

**模块**：db-tools（新模块，ADR-032）+ env-mgmt Node 环境增量（D1/D2 面）
**检查时间**：2026-11-19
**编号源**：engine-rules.md G1-01~G1-11（唯一编号源）；validation SKILL Gate 1 节为执行展开；S7/S8 为 design 侧预检编号（不混用）
**D0 基准**：00-architecture/d0-clarification-db-tools-integration-2026-11-19.md（Q1 一步到位 / Q2 面板会话 gating 反代 / Q3 轻量入口并存 + 安全底线）

**基线文档版本**：api-db-tools.md v1.1 / prd-db-tools.md v1.0 / domain-model.md v1.24 / BA·AA·FL-db-tools v1.0 / PageSpec admin·readonly v1.0 / BA·AA·FL-env-mgmt v1.2 / prd-env-mgmt.md v1.3；提供方登记 api-database-mgmt.md v2.4、api-site-mgmt.md v2.8、api-account-auth.md v1.2；消费兑现 database-mgmt PageSpec admin/readonly v1.5。

**检查范围**（ask 指定）：00-architecture（db-tools 三文档 + env-mgmt 三文档）、02-detailed/prd（prd-db-tools + prd-env-mgmt + domain-model）、02-detailed/api/api-db-tools.md、02-detailed/interaction/{admin,readonly}/db-tools/。跨模块提供方登记与代码取证为支撑抽查。

---

## 一、机械核验（本轮实跑）

脚本：`D:/tmp/gate1-dbtools/check.py` 与 `enum2.py`（python -I 隔离模式执行，UTF-8 输出）。

| # | 检查 | 命令/方法 | 结果 |
|---|------|----------|------|
| M1 | 三段式（G1-08） | check.py CHECK 1：正则提取 api 全部 `/dbtool/...` 路径按段计数 | **PASS 10/10**——tool/{list,install,upgrade,uninstall}、task/{list,detail}、access/{open,rotate}、gate/verify、signon/verify 全三段。2 段命中仅 nginx 工具 location 前缀（/dbtool/phpmyadmin/、/dbtool/mongo-express/，D-13 baseUrl）与 D-01 资源枚举模板截断，非 API 端点 |
| M2 | S7 Action↔API 双向 diff（G1-11） | check.py CHECK 2：api 3.3 矩阵 8 面板接口 ↔ admin PageSpec 第 2 节 8 Action 集合 diff + 方法比对 | **PASS 8/8**——API-only=∅、Action-only=∅、方法不一致=∅；内部端点 2（gate/verify、signon/verify）未误列前端 Action（PageSpec 第 2 节显式声明服务端调用方） |
| M3 | readonly 负空间 | check.py CHECK 3：readonly 3 读 Action ∪ 5 负空间 == admin 8 全集；子集/不相交/全 GET 校验 | **PASS**——3 读（dbtoolList/TaskList/TaskDetail 全 GET）+5 负空间（Install/Upgrade/Uninstall/Rotate/Open）=8/8，负空间∩可见=∅；含 dbtoolOpen 工具访问类（⚠️A10） |
| M4 | 枚举四方同值（G1-07） | enum2.py：池（04-business-enum.md）↔ api 3.4 ↔ admin 第 3 节字典 ↔ domain-model `{...}` 取值集合比对（转义管道 `\|` 正确 split） | **PASS**——db_tool_type{phpmyadmin,mongo_express}/db_tool_task_action{install,upgrade,uninstall}/db_tool_task_status{pending,running,succeeded,failed}/db_engine{mysql,mariadb,redis,mongodb} 四方逐值相等；env_type 八值（含 node）池==api==domain-model（PageSpec 声明透传 env-mgmt 字典非本模块 Key，admin 第 3 节注）；panel_role 池==admin==domain-model（api 3.4 合并引用行）。脚本两处 False 为解析假象（env_type 非字典 Key、panel_role 合并行），人工复核取值一致 |
| M5 | 池唯一性 | `grep -oP` 提取池标识列 `sort \| uniq -d` | **PASS**——空输出（无重复定义）；db_tool_state 不采用行注明理由（04-business-enum.md:66） |
| M6 | DTO 可空显式 | check.py CHECK 5：正则提取 `字段: 类型\|null` | **PASS**——9 字段显式 \|null：installedVersion/boundInstanceId/running/runningTaskId/instanceId/startedAt/finishedAt/error/tokenExpiresAt；api 3.1 声明「JSON null ≠ TS undefined，后端 *T 无 omitempty」纪律（api-db-tools.md:118）；admin 第 8 节检查 11 null 呈现逐项对应（db-tools.md:193） |
| M7 | 错误码族 | check.py CHECK 6：DBT_(\d{3}) 集合 | **PASS**——DBT_001~012 连续唯一无缺号；复用 AUTH_001/002/003/008；内部端点拒绝语义显式（gate 裸 401/signon 包络 DBT_007/008/010/loopback·秘密头失败裸 404，api-db-tools.md:176） |
| M8 | 审计码一致性 | check.py CHECK 7：api vs prd dbt_* 集合 diff | **WARN W1**——prd-only={dbt_opened}（prd-db-tools.md:90,92 FR-08/FR-10），api 权威定义 dbt_tool_accessed（api-db-tools.md:44 D-16/:191）；api-only={dbt_task_finished,dbt_tool_accessed}（prd 以「任务终态审计」泛称承载终态码，可接受；dbt_opened 为命名不一致） |
| M9 | changelog（G1-09） | 12 文件逐份 grep 更新记录/变更记录节 + 表头版本 | **PASS**——12/12 有 changelog 节；表头版本==最新行（api v1.1、prd-env v1.3、domain-model v1.24、env BA/AA/FL v1.2、其余 v1.0） |
| M10 | 占位符扫描 | `grep -nE "TODO\|TBD\|待填\|占位\|FIXME"` 11 份 in-scope 文件 | **PASS**——0 命中 |

## 二、G1-01~G1-11 逐项

| 编号 | 检查项 | 结果 | 依据 |
|------|--------|------|------|
| G1-01 | 架构文档与模块设计一致 | PASS（W2 措辞滞后） | BA 3 阶段/3 L3/10 L4（business-arch-db-tools.md:184-202 树）== AA 10 子模块（C-15-01~04/S-58-01~02/S-59-01~04，application-arch-db-tools.md:241-262）== FL admin 8 菜单行/readonly 2（function-list-db-tools.md:53-148 实数）== PRD FR-01~10 一一对应；编码 C-15/S-58/S-59 承接全局唯一（AA:24 占用核验声明）；安全架构 BA 2.4 四防线 ↔ AA 附录C ↔ PRD 第五部分末映射表三层同构；env-mgmt Node 增量 L2/L3/L4 集合零变化（BA-env:195 changelog ①），八环境 BA/AA/FL/prd/domain-model/池六处同步 |
| G1-02 | PRD 覆盖业务场景与角色 | PASS | prd-db-tools 完备性 G 5/5、T 适用 8/8（T3/T5 N/A 有据：无审批/无导出）、N 4/4（N2 20 异常分支，prd-db-tools.md:297-304）；角色 admin/readonly+访客不涉及（第三部分）；Q1~Q3 拍板全落 FR；prd-env-mgmt v1.3 G 5/5·T 6/6·N 4/4（:277-286），FR-08 Node 供给+⚠️A16/A17 |
| G1-03 | API 设计完整 | PASS | 10 端点路径/方法/权限/CSRF/请求/响应/错误码齐备（api 3.3 矩阵+4.1.4/4.2.4/4.4 详情）；2 表 DDL+迁移 013（5.3，001~012 实存核验见三）；18 设计决策 D-01~D-18；常量表 D-13；配置项 tools.mirror/tools.npm_registry |
| G1-04 | 交互覆盖页面类型 | PASS | 1 hybrid 页面 /db-tools（卡片列表+任务 Drawer 列表/详情，admin 第 4 节）；表单=即时 Dialog 形态无独立路由（第 5 节声明）；详情=Drawer（第 6 节声明）；审批 N/A（prd T3 依据）；readonly 同构只读视图 |
| G1-05 | API 字段与交互规格对齐 | PASS | 卡片字段（displayName/targetEngines/catalogVersion/installed/installedVersion/boundInstanceId/running/depsReady/depsMissing/runningTaskId）逐项源自 tool/list DTO（api:280）；任务列 9 列源自 task/list DTO（api:394）；Drawer 详情+outputTail 源自 task/detail（api:395）；查询条件仅 toolType/status 真实参数（admin 4.2，不虚构）；null 呈现对齐（M6） |
| G1-06 | 状态机完整 | PASS | db_tool_task_status 四态：pending→running→succeeded/failed + 重启恢复 running→failed（api 4.2.2:372-377；prd 第六部分:136-141），触发/接口/终态不可逆（I-07）齐备；db_tool 无状态机辨析 4 项（api 4.1.2）+prd 辨析 6 项防伪状态机；env_task_status 不变（Node 服务态 null 为不适用语义非新状态机，prd-env:142 T2 辨析） |
| G1-07 | 枚举统一无冲突 | PASS | M4 四方同值 + M5 池唯一；3 新枚举 D2 定稿入池（池:63-65）；db_tool_state 不采用理由注明（池:66）；env_type 扩 node 八值 D1 登记 D2 确认（池:57） |
| G1-08 | 接口路径规范 | PASS | M1 10/10 三段式 /dbtool/{resource}/{action}（D-01 资源段裁定，不用裸 tool/不用连字符） |
| G1-09 | changelog 已填写 | PASS | M9 12/12；prd-env-mgmt 双 v1.1 行见 I1 |
| G1-10 | INDEX 模块状态已更新 | PASS | INDEX db-tools 行 stages requirement/api/interaction=done（D0-D4 完成注记详实）、backend/frontend/validation=todo（待 B/F/Gate 2）与本 Gate 时点一致；gates.gate1 由本 Gate 更新（pending→warn）；env-mgmt 行 Node D2 完成/D3-D4 待工作流注记在位（INDEX:26） |
| G1-11 | 跨阶段模型一致性 | PASS（W1/W2） | domain-model v1.24 db_tool/tool_deploy_task 双聚合根字段↔api 4.1.1/4.2.1 字段清单↔5.3 DDL 逐列一致（tool_type UNIQUE/cred_cipher/instance_id NULL/unit_name NULL/output_tail/error/时间字段）；关联领域对象 db_instance（domain-model:339）↔ api D-10 OHS ↔ api-database-mgmt §4.9 签名一致；S7 8/8（M2）；存储映射（domain-model:438）↔ DDL 两表+令牌台账无表一致 |

## 三、D0 三拍板落地核验

### Q1 一步到位（env-mgmt +Node + phpMyAdmin/mongo-express）— PASS

- Node 环境：BA-env v1.2 ⚠️A16（NodeSource 三系代配 ⚠️A10 同构+LTS 轨道钉死+nodejs 三系同+npm 随包+node --version）/⚠️A17（运行时型无服务单元：单元映射空/服务态 null/启停入口禁用/装完即用无引导）→ AA-env S-53/S-54/C-14 同步 → FL-env 八环境菜单行不变（admin 6/readonly 2，计数笔误订正在案）→ prd-env v1.3 FR-08 + FR-01/02/03/06/07 Node 语义同步 → domain-model v1.24 环境条目八值+发行版映射 +node（零新对象）→ 池 env_type 八值。六处同值（M4）。
- 两工具：api D-06 phpMyAdmin（tarball+sha256+config.inc.php signon+PHP-FPM 默认 pool 复用+扩展清单 php -m）/D-07 mongo-express（npm 精确版本+systemd 单元 env 注入+bind 127.0.0.1:8081+baseUrl）；目录两条目编译期（D-13/I-13）。
- **待同步项如实**：env-mgmt api Node 增量（env_type 八值/DDL CHECK 迁移/常量）归其 D3/D4——实查 api-env-mgmt.md:135 仍七值、:173 DDL CHECK 七值、ENV_002「七值」，与 api-db-tools D-18「不硬编码七值/八值，以实况读契约返回为准」自洽。设计面无缺口，代码面待 env-mgmt D3 轮（I4）。

### Q2 面板会话 gating 反代（安全核心）— 四要素显式且自洽（W3/W4 两处显式性待补）

| 要素 | 显式落点 | 自洽性核验 |
|------|---------|-----------|
| localhost 绑定 | api D-07（ME_CONFIG_SITE_BIND_ADDRESS=127.0.0.1，默认 0.0.0.0 须显式覆盖）+D-13 常量 MONGO_EXPRESS_PORT=8081/MONGO_EXPRESS_BIND=127.0.0.1；phpMyAdmin 无独立端口（FastCGI 经 PHP-FPM pool，D-06）；BA 2.4 防线一（无防火墙放行） | ✓ 与 D0「工具绝不公网暴露」一致；PageSpec 安装文案「仅监听 127.0.0.1:8081」（admin:93）同值 |
| auth_request 会话 gating | api D-03 注入模板（工具 location auth_request→内部 gate location internal; proxy_pass 面板后端+秘密头+cookie 透传，4.4.4）+D-11 三重防护（loopback 源校验裸 404/X-Dbtool-Internal 秘密头 0600/不挂会话 CSRF 中间件）+4.4.1（VerifyWs 只读不续期+role==admin 服务端权威，裸 200/401） | ✓ 消费 account-auth 既有 VerifyWs（session.go:59 实存核验），api-account-auth v1.2 登记「零新增 HTTP，端点归 db-tools 自持」；readonly/无会话→401 与 ⚠️A10 一致；吊销级联即时切断（逐请求实时校验）；gating 不落审计（⚠️A11 防写放大）与 VerifyWs 只读不续期同判 |
| signon 防伪 | api D-09+4.4.3 令牌规约：HMAC-SHA256（密钥 crypto/rand 32B vault 密文）+TTL 60s+一次性 jti 内存台账（TTL 清扫 30s，重启全失效安全保守）+instanceId/sessionDigest 双绑定；四重校验→DBT_007/008/010 三路拒绝 | ✓ sessionDigest 概念实存（backend identity.go:13「token_hash 前 8 位，对外会话标识」）；prd N2 令牌四路异常↔错误码一一对应；**W3**：SignonURL 重定向落地载体未定义（access/open 为 POST+CSRF，phpMyAdmin 302 GET 不可达；PageSpec 无 GET 落地路由）——主流程（面板打开→令牌→window.open）闭环，二次流程（工具内 PMA 会话过期→SignonURL 重定向）缺载体；**W4**：4.4.2④ sessionDigest 绑定要求回调携面板会话 cookie，SignonScript 从浏览器请求 $_COOKIE 转发步骤未显式（机制可推导且与「凭据响应不经浏览器」不矛盾，建议补一句） |
| 凭据零回显 | api I-02（cred_cipher 任何响应/日志/审计不含明文/密文/密钥）+D-08（basic auth 口令明文仅生成与写秘密文件内存态，nginx include 0600 服务端注入，浏览器全程无感知；rotate 响应无口令字段）+D-09/4.4.2（MySQL 凭据仅 loopback 响应 SignonScript→$_SESSION，永不进浏览器）+D-10（ToolCreds 明文用毕即弃不落库不入日志不入审计） | ✓ access/open 响应仅 {toolType,url,tokenExpiresAt}（令牌非凭据，url 片段下发 admin 用于 signon 重定向——口径显式声明 admin:159）；三类凭据（连接凭据/basic auth/signon 密钥）载体分清：连接凭据取 db-mgmt vault 本域不持久化（domain-model:293/339）；dbtool-cred.key 0600 沿 db-cred.key 先例（vault.go:14-24 实存） |

**其余安全底线**：sha256 强制不降级（D-05，失败=failed+清理）；R-08 参数数组（npm/systemctl/tar/php -m 全字面量）；R-03 同事务审计脱敏（D-16 六码 detail 不含凭据/口令/令牌）；卸载不删数据（D-15 执行链白名单+I-C03，PageSpec 两段警示+输入锁定+「绝不删除数据库数据」文案 admin:97-102）；R-09 零遥测（下载用户主动例外+phpMyAdmin VersionCheck=false，api:69）。

### Q3 轻量入口保留并存 — PASS

- api 1.2 边界⑩「面板内轻量浏览归 database-mgmt S-56/S-57 保留并存」；PRD ⚠️A13/OUT-4。
- 高级入口消费兑现：database-mgmt admin PageSpec v1.5 §6.8（实例行/库行入口，引擎映射 mysql/mariadb→phpmyadmin、mongodb→mongo_express、redis 无入口 OUT；工具状态懒加载 dbtoolList；已装 window.open 新标签页；未装引导安装；mongo-express boundInstanceId 绑定校验——api v1.1 联动）；readonly v1.5 入口负空间（4.1/6.1/6.3 不渲染+gating 网络层闸门登记+轻量浏览保留可达 Q3/Q6 不变）。跨模块 Action 引用声明「本模块 S7 44/44 不变，api 零新增接口」沿 F5 先例。
- 打开裁定=新标签页 window.open noopener（非 iframe）——理由与 gating cookie scope/sessionDigest 绑定一致性自洽（admin 4.4，关闭 prd 待对齐-6）。

## 四、跨模块提供方登记与代码取证（支撑抽查，实读/实跑）

| 项 | 核验 | 结果 |
|----|------|------|
| api-database-mgmt.md v2.4 | D-27（:55）/§4.9（:641-661）/§5.1（:676）/1.5（:79）/6.1（:761）ToolCredentialGateway 签名与 api-db-tools D-10 逐参一致（ToolCreds{Username,Password,Host,Port,Engine}/ReadyInfo{Ready,Reason,Engine}）；零 HTTP/DDL 变更声明 | ✓ |
| api-site-mgmt.md v2.8 | D-21（:49）/§5.1（:495）/1.5（:69）/6.1（:570）ConfigWritePipeline Apply(ctx,file,newContent,allowNew)/Remove(ctx,file) 与 api-db-tools D-03/4.1.5 一致；注入内容归 db-tools 组合 | ✓ |
| api-account-auth.md v1.2 | 1.5（:55）/3.2（:115）/4.2.5（:326）/5.1（:448）/6.1（:539）VerifyWs gating 消费登记；表头 v1.2 订正在位；零新增 HTTP | ✓ |
| backend VerifyWs | `grep -rn "func.*VerifyWs" backend/` → session.go:59 实存，只读不续期（I-11 同判定） | ✓ |
| backend SessionDigest | identity.go:13「token_hash 前 8 位，对外会话标识」实存——signon 绑定概念有代码承载 | ✓ |
| 迁移空号 | `ls backend/internal/store/migrations/` → 001~012 实存（012=env_bootstrap），013 为下一空号 | ✓ |
| pipeline.go:21 | backend/internal/service/sitemgmt/pipeline.go:21 applyPipeline(ctx,file,newContent,allowNew)——R-06 四步（备份→原子写→nginx -t→回滚）实存，ConfigWritePipeline 泛化基础成立 | ✓ |
| 其余先例 | extract.go:42-50 ServerBlock.Proxy()（落点识别）/vault.go:14-24 db-cred.key 0600/acme.go:360 injectChallengeLocation（span 注入+备份回滚）/phpfpm gateway.go:54 ListPools/nginxx gateway.go:183 WriteSecretFileAtomic/env-mgmt service.go:67 Catalog+:94 PHP-FPM——逐一实读在位 | ✓ |

## 五、WARN 项（非 CRITICAL，待用户确认）

- **W1 审计动作码命名不一致**：prd-db-tools.md:90（FR-08）/:92（FR-10）用 `dbt_opened`，api-db-tools.md:44（D-16）/:191 权威定义 `dbt_tool_accessed`。语义同一（访问开启），api 为下游权威且前端不消费审计码，不阻断；建议 prd 回填统一为 dbt_tool_accessed（或 api 注明别名沿革）。
- **W2 D2→D3 决策演进未回填上游措辞**（契约本身自洽，提供方登记均已兑现）：
  - ① 工具路径形态：prd-db-tools.md:80（FR-03「经 site-mgmt 建 php_fpm/reverse_proxy 站点」）/:177/:224、AA-db-tools 附录B（:286）/:90/:122、domain-model.md:419 仍为「建站」措辞；api D-03 已裁定**面板域 server 块持久 location 注入（不落独立站点）**经 ConfigWritePipeline OHS（site-mgmt v2.8 D-21 已登记）。
  - ② gating 端点归属：prd-db-tools.md:222（「account-auth 新增内部能力」）、AA-db-tools:189/:283/:340、domain-model.md:220/:232/:353/:417 仍为「account-auth 新增内部端点待同步」措辞；api D-11 已裁定 **db-tools 自持 gate/verify + 复用既有 VerifyWs**（account-auth v1.2 已登记零新增 HTTP）。
  - 建议下一轮 prd/AA/domain-model 措辞回填（沿全仓「D3 定稿回填」先例，如 domain-model v1.6/v1.8/v1.10 行）。
- **W3 signon 二次认证落地载体未定义**：D-06/D-09 SignonURL「指向面板打开入口 URL（含 instanceId）」，但 access/open 为 POST+CSRF 接口，phpMyAdmin 未登录 302 GET 重定向不可直接调用；PageSpec 仅 /db-tools 路由，无 signon 重认证 GET 落地页。主打开流程闭环不受影响；工具内 PMA 会话过期后的重定向路径缺载体定义。建议 D3 补充 SignonURL 落地形态（GET 落地页触发重新打开，或指向面板 /db-tools 引导）。
- **W4 SignonScript 会话 cookie 转发步骤未显式**：api 4.4.2④（:457）sessionDigest 绑定校验要求「SignonScript 回调携面板会话 cookie」，但未显式说明 cookie 来源（SignonScript 从浏览器工具路径请求 $_COOKIE['dg_session'] 读取并转发）。机制可推导（工具路径在面板域下 cookie scope 覆盖，SignonScript 运行于浏览器请求上下文）且与「不经浏览器」（指凭据响应通道）不矛盾；因 ask 要求安全设计显式，建议补一句明示。
- **W5 版本/sha256/LTS 常量落码取证待办**：D-13 PHPMYADMIN_VERSION=5.2.2⚠️/MONGO_EXPRESS_VERSION=1.0.0⚠️/NODE_LTS_TRACK=22⚠️——sha256 值设计侧不虚构、后端落码时经官方发布页取证填入（诚实声明，非缺陷）；NODE_LTS_TRACK 需与 env-mgmt 待对齐-5（LTS 版本号其 D3 定稿）同步落定。B 阶段前置提醒项。

## 六、INFO 项

- **I1** prd-env-mgmt.md 更新记录双 v1.1 行（:294 补录简版/:296 详版），顺序 v1.3/v1.2/v1.1/v1.0/v1.1 非严格降序——建议合并去重。
- **I2** prd-env-mgmt.md:264 T9 依据仍写「FR-01~07」（FR-08 已存在；v1.3 增量复核行已覆盖 FR-08，措辞小滞后）。
- **I3** database-mgmt admin PageSpec:9 表头数据源引 api-db-tools.md v1.0，而 §6.8（:449）消费 v1.1 新增 boundInstanceId——建议表头引用升 v1.1。
- **I4** env-mgmt Node 增量四方同值目前成立面=设计侧（池/BA/AA/FL/prd/domain-model/api-db-tools 透传）；api-env-mgmt.md:135/:173 仍七值、PageSpec/前端 enums 同值归其 D3/D4——待同步项如实登记（api-db-tools D-18），非本轮缺口。
- **I5** FX 全量 141 项清单未随引擎 payload 发布，PageSpec 以第 8 节自查替代（admin 12 项/readonly 5 项，全仓先例）；本 Gate 按 G1 编号源+S7/S8 机械核验执行，FX 逐条归 Gate 2/前端栈。
- **I6** dict/batch 偏差（5 字典 Key 前端 TS 常量，单一事实源枚举池）已声明（admin 第 3 节），沿全仓既有偏差先例。

## 七、总结

**判定：WARN**（CRITICAL 0 / 契约断裂 0）

- 机械核验 M1~M10：9 PASS + 1 WARN（M8 审计码命名）。
- G1-01~G1-11：11/11 PASS（G1-01/G1-11 带 W1/W2 措辞滞后注记，不构成契约断裂——api 为权威下游且提供方登记三方兑现）。
- D0 三拍板：Q1 ✓（Node 增量设计面六处同值，api/代码面待同步项如实）/ Q2 ✓ 四要素显式且自洽（W3/W4 两处显式性待补，不削弱安全约束本身）/ Q3 ✓（高级入口 v1.5 兑现+轻量浏览保留）。
- WARN W1~W5 待用户经主会话确认；确认后 Gate 1 记 pass，进入 B1（backend）。
- 未执行项如实声明：env-mgmt Node 的 api/PageSpec/前端 enums/DDL CHECK 同值属其 D3/D4 轮，本轮未执行（I4）；编译门控/实机冒烟属 Gate 2，本轮未执行。

**报告路径**：05-reports/gate1-db-tools-2026-11-19.md