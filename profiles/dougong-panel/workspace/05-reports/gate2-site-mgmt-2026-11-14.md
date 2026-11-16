# Gate 2 验证报告 — 代码→交付门控（site-mgmt 网站管理·核心卖点）

**模块**：site-mgmt
**扫描路径**：backend/internal/{api,service,store,adapter,errcode} + migrations/004_site_mgmt.sql + cmd/dougong/main.go（装配段）⊕ frontend/src/{types,api,stores,views/site-mgmt,router}
**扫描时间**：2026-11-15（检查员独立轮；文件名按指令落 2026-11-14 批次）
**契约基准**：profiles/dougong-panel/workspace/02-detailed/api/api-site-mgmt.md v1.0（18 接口/21 错误码/8 枚举/certificate DDL）
**规则链**：引擎 GR-01~06 ⊕ backend-rules §7 G-01~G-07 / frontend-rules §6 F-01~F-06 ⊕ business-concepts R-01~R-18 适用项
**编译门控（脚本已跑，本轮引用不重跑）**：build=0 vet=0 test=0 web=0

## 总结

**判定**：**FAIL**（1 项契约断裂 F-1；CRITICAL 栈规则违例 0；WARN 3 项 W-1~W-3）

- 通过：后端规则扫描 G-01~G-07 全过、前端 F-01~F-06 全过、AL-001/002/004/005 全对齐、业务规则适用项全过、SEC-GATE 探针通过（DEBT-SEC-GATE 不适用）
- CRITICAL 违例：0
- 契约断裂：1（F-1，php_fpm 建站链路）
- WARN：3

---

## 0. 阻断项（FAIL）

### F-1 php_fpm 建站链路契约断裂：前端（按契约/交互规格）永不发送 root，后端强制 php_fpm root 必填 → UI 创建 PHP 站点必然被拒（AL-003/CT-006 类）

三方证据链（均本轮实读）：

| 方 | 证据 | 内容 |
|----|------|------|
| 契约基准 | api-site-mgmt.md §4.2.4 create req（第 354 行） | `root?(type=static 必填)` —— root 仅 static 必填，php_fpm 可选 |
| 交互规格 | 02-detailed/interaction/admin/site-mgmt/site-mgmt.md 新建弹窗字段表（第 128~135 行） | 根目录字段「校验=type=static 必填；显隐条件=type=static」——php_fpm 无 root 输入 |
| 后端 | backend/internal/service/sitemgmt/create.go:42-48 | `case SiteTypePHPFPM: if req.PoolName == "" {...}; if req.Root == "" { return nil, fieldError("root", "type=php_fpm 时 root 必填") }` —— 比契约更严 |
| 前端 | frontend/src/views/site-mgmt/components/SiteCreateModal.vue:179 与 :253 | 提交体 `root: f.type === 'static' ? f.root : undefined`；模板 `<n-form-item v-if="form.type === 'static'" label="根目录">` —— php_fpm 永不带 root |
| 测试盲区 | backend/internal/api/sitemgmt/module_test.go:419-423（HTTP 级仅 reverse_proxy）；backend/internal/service/sitemgmt/sitemgmt_test.go:706-710（service 级 php_fpm 用例显式带 `Root: www` 故通过） | 「php_fpm + 无 root」组合零覆盖，断裂未被任何测试暴露 |

后果：从面板 UI 创建 PHP 站点（site_type=php_fpm，PRD FR-15/三模板之一）100% 被后端 AUTH_012 fields[root] 拒绝——功能路径不可用。
修复方向（二选一，需设计侧拍板）：① 契约/交互规格/前端为 php_fpm 增加 root 字段（后端现状不动，php 站点 SCRIPT_FILENAME 依赖 $document_root，工程上更合理）；② 后端放宽 php_fpm 的 root 必填回归契约（root 缺省时模板省略 root 指令，继承 http 级配置）。任一方向落定后需补「php_fpm 无 root」HTTP 级回归用例。

---

## 1. 后端规则扫描（backend-rules §7 G-01~G-07，实跑 grep）

| # | 检查项 | 结果 | 证据（命令+输出/文件:行） |
|---|--------|------|--------------------------|
| G-01 | 无 shell 拼接 | PASS | `grep -rnE "exec\.Command(Weak)?Context?\(.+\"(-c|/c)\"" --include="*.go" .` 0 命中（exit=1）；全仓 `exec.Command` 4 处人工核对：nginxx/parser.go:3（注释）、nginxx/gateway.go:31（注释）为文字；真实调用仅 gateway.go:303（`nginx -t`）与 :327（`nginx -s reload`）——均 `exec.CommandContext(cctx, g.binPath, "-t"/"-s", "reload")` 字面量参数数组，二进制路径构造期 `exec.LookPath` 解析（gateway.go:59），R-08 达成 |
| G-02 | 无 SQL 拼接 | PASS | `grep -rnE "Sprintf\(.+(SELECT\|INSERT\|UPDATE\|DELETE)" --include="*.go" .` 0 命中（exit=1）；store/sitemgmt/cert_repo.go 全部 SQL 静态字面量 + `?` 参数（:39-45/:59/:71-72/:92-93/:112/:122-124/:145/:165/:172/:180） |
| G-03 | 无硬编码凭据 | PASS | `grep -rnEi "(password\|secret\|token)\s*=\s*\"[^\"]{8,}\"" --include="*.go" \| grep -v _test` 0 命中（exit=1）；ACME 账户密钥运行期生成落文件 0600（acme/gateway.go:199-210 `ecdsa.GenerateKey` + `os.WriteFile(keyPath, pemB, 0o600)`），非硬编码 |
| G-04 | SQL 只在 store 层 | PASS | `grep -rlE "(SELECT\|INSERT\|UPDATE\|DELETE) " --include="*.go" \| grep -v internal/store \| grep -v _test` 命中 5 文件逐条核对全为注释：本模块 certsvc.go:80「→ INSERT pending（机制无审计）」/scanner.go:160「→ INSERT imported 行」为流程注释；其余 user.go:114/scope.go:20/trash.go:36,81 为前序模块注释（与 file-manager gate2 同判） |
| G-05 | 无 panic 滥用 | PASS | `grep -rn "panic(" --include="*.go" \| grep -v _test` 0 命中（exit=1） |
| G-06 | goroutine 有退出路径 | PASS | `grep -rn "go func" --include="*.go" \| grep -v _test` 6 处全在 cmd/dougong/main.go（:241/:256/:271/:325/:331 前序已核 + :287 本模块续期调度）；RunRenewalScheduler（service/sitemgmt/renewal.go:19-33）两级 `select ctx.Done() → return nil` 退出，main 侧 wg.Done + panic 隔离兜底（main.go:286-297）；internal/ 自起 goroutine 0 处（ACME 超时以 ctx 注入 RoundTripper 实现，acme/gateway.go:242-253，零 goroutine） |
| G-07 | 编译+vet+test 全绿 | PASS | 脚本已跑 build=0/vet=0/test=0（ask 声明，本轮引用）；本轮补充实跑 `go test -count=1 -cover` 六包全 ok（见 §4 覆盖率） |

附加扫描：`log.Print` 非 slog 0 命中；`fmt.Print` 调试输出 0 命中（均 exit=1）。

## 2. 前端规则扫描（frontend-rules §6 F-01~F-06，实跑 grep）

| # | 检查项 | 结果 | 证据 |
|---|--------|------|------|
| F-01 | 无 Options API | PASS | `grep -rLn "script setup" --include="*.vue" src/` 0 命中（exit=1） |
| F-02 | 无裸 any | PASS | `grep -rnE "(:\s*any\|as any)" --include="*.ts" --include="*.vue" src/` 0 命中（exit=1） |
| F-03 | 请求只走 api 层 | PASS | `grep -rnE "(axios\|fetch)\(" --include="*.vue" src/views` 0 命中（src/components 目录不存在，规则目标面如实注明；exit=1/2）；site-mgmt 全部请求经 src/api/site.ts（18 函数），唯一 fetch 出口 http.ts |
| F-04 | 无混入其他组件库 | PASS | `grep -rnE "from '(antd\|element\|ant-design-vue)'" src/` 0 命中（exit=1） |
| F-05 | 无原生弹窗 | PASS | `grep -rnE "(^\|\W)(alert\|confirm)\(" --include="*.vue" --include="*.ts" src/` 0 命中（exit=1）；site-mgmt 域内亦 0 |
| F-06 | typecheck+build 全绿 | PASS | 脚本已跑 web=0（引用）；frontend/tasks.md F07 记录 `npm run build`（vue-tsc --noEmit && vite build）exit 0 + 独立 typecheck exit 0 |

附加扫描（site-mgmt 域）：`setInterval\|setTimeout` 0 命中（无轮询，D-01 手动刷新纪律，stores/site.ts:5-7 注释明示不持定时器）；`#[0-9a-fA-F]{6}` 0 命中（expiring 行底色用 rgba，certificates/index.vue:480）。事件监听配对：source/index.vue:211-214/275-276 挂载、:280-288 onUnmounted 全量摘除（keydown/beforeunload/matchMedia + editor/model dispose）。

## 3. 前后端接口对齐（AL-001~005，契约基准 api-site-mgmt.md v1.0）

| # | 检查项 | 结果 | 证据 |
|---|--------|------|------|
| AL-001 | Controller 路径 vs API 文档 | PASS | routes.go:41-66 逐条比对 api §3.3 矩阵/§4.1.4/§4.2.4：18 接口全一致——GET 两角色 4（site/list:41、site/detail:42、log/tail:43、cert/list:44）+ GET admin 2（site/source:49、php-pools/list:50）+ POST admin+CSRF 12（create:55、update:56、proxy/update:57、source-save:58、enable:59、disable:60、nginx/reload:61、cert/request:62、deploy:63、undeploy:64、retry-renewal:65、delete:66）；前缀 /sites（:30）；中间件链 AuthRequired→CsrfGuard(POST)→RequireRole 与 §3.1 一致 |
| AL-002 | 前端 Action 路径 vs Controller | PASS | api/site.ts 18 函数路径与 routes.go 18 条全等（siteList:40/siteDetail:45/siteCreate:50/siteUpdate:55/proxyUpdate:60/siteSource:65/siteSourceSave:74/siteEnable:82/siteDisable:86/logTail:95/nginxReload:100/phpPoolsList:105/certList:112/certRequest:120/certDeploy:128/certUndeploy:135/certRetryRenewal:143/certDelete:151）；方法匹配（读 6 GET/写 12 POST） |
| AL-003 | DTO 字段 vs 前端 VO 字段 | **FAIL（F-1）**+WARN（W-1） | F-1 见 §0（create root 三方断裂）；W-1 见下。其余抽查一致：update 请求 listen 双侧 string[]（dto.go:94 / types/site.ts:94）；source/source-save baseMtime 乐观锁字段一致（dto.go:123-136 / types/site.ts:125-136）；错误 data 载荷键一致（SITE_003 errors+diff dto.go:260-264 / types/site.ts:222-230；SITE_010 currentMtime dto.go:268-271 / site.ts:228-230；SITE_005 existingSiteId dto.go:274-276；CERT_003 existingCertId dto.go:279-281；CERT_005 sites[] dto.go:288-291） |
| AL-004 | 枚举值前后端一致 | PASS | types/enums.ts:202-246 六字典取值与 api §3.4 逐字一致：site_type{static,php_fpm,reverse_proxy}/site_enabled{enabled,disabled}/cert_source{acme,imported}/acme_challenge{http_01,tls_alpn}/cert_status{pending,valid,expiring_soon,expired,renew_failed}/site_log_type{access,error}；后端同值常量 consts.go:47-57 + store/sitemgmt/models.go:20-32 + DDL CHECK（004_site_mgmt.sql:9-11） |
| AL-005 | 错误码前端可识别 | PASS | errcode/sitemgmt.go 21 码（SITE_001~014+CERT_001~007）与 types/site.ts SITE_ERROR_TEXT:276-291（14 码全）/CERT_ERROR_TEXT:294-302（7 码全）一一对应；包络：后端 response.go:25-27 成功 code=0 数值、失败上线 symbol 符号串（:34/:44），前端 http.ts:153-160 `env.code === 0` 分流 + normalizeCode 双通道归一（utils/errcode.ts:20-29 字符串原样透传）；复用码族 AUTH_001/002/003/008/012 前端均有全局分流（http.ts:70-99）；13 个审计动作码前端文案全收录（enums.ts:270-282，含 cert_renewed） |

## 4. 测试覆盖率（本轮实跑 `go test -count=1 -cover`，backend/ cwd）

| 层级/包 | 覆盖率 | 达标（SKILL Gate2 §3 目标） |
|---------|--------|------------------------------|
| internal/service/sitemgmt（Service 层） | 78.0% | NO（目标 80%+，差 2pp）→ W-2 |
| internal/api/sitemgmt（Controller 层） | 71.3% | YES（目标 60%+） |
| internal/store/sitemgmt | 78.6% | （SKILL 无 store 单列目标，参考值） |
| internal/adapter/nginxx | 53.1% | parser/extract 有 28 样本 round-trip；gateway.go fs/exec 原语经 service 层 fake 网关注入间接覆盖（包内直测低）→ W-2 |
| internal/adapter/phpfpm | 0.0%（无测试文件） | NO → W-2（Linux 环境绑定，tasks.md 如实声明归目标机冒烟） |
| internal/adapter/acme | 0.0%（无测试文件） | NO → W-2（同上，lego 外呼不可本机实联） |
| 前端组件（views/site-mgmt） | 未实测（无组件级 vitest；既有 51 用例为 stores/utils 层） | 未实测 → W-2（沿 file-manager gate2 W2 同判） |

测试存在性核验：api 权限矩阵/CSRF/admin 全链路（module_test.go:332/371/400）+ service 22 用例（sitemgmt_test.go:308-1360）+ store 生命周期/同事务（store_test.go:31/142）+ parser 13 用例（parser_test.go:10-228）。**round-trip 样本集 28 例 ≥ 20（M3 验收硬项）**：parser_test.go:11-40（含 if/正则 location/引号转义/upstream/map/dormant 注释块/CRLF/中文/游离分号 Raw 保全）。

## 5. 业务规则与设计决策抽查（R-01~R-18 适用项 + api D-01~D-16 关键项）

| 项 | 结果 | 证据 |
|----|------|------|
| R-02 会话+CSRF | PASS | 写组三件套 routes.go:53-54（AuthRequired→CsrfGuard→RequireRole(admin)）；前端 POST 统一注入 X-CSRF-Token（http.ts:113-116），GET 豁免与契约 §3.1 一致 |
| R-03 审计 | PASS | 13 动作码全落地（consts.go:61-74）：文件系统侧 auditAction 重试一次+日志兜底（service.go:67-95）；certificate 终态/删除与审计同事务（certsvc.go:170-210 finalizeRenewal / :443-466 CertDelete tx）；reload 失败亦记 failure（logtail.go:160-168） |
| R-06 安全写入四步 | PASS | pipeline.go:21-71：快照（失败 SITE_013 :36-42 整体不执行）→ 原子写（:47）→ `nginx -t`（:52，失败自动回滚+SITE_003 errors+diff :57-64）→ 尾部清理保留 20 份（:66-69/:99-120）；测试实证 TestPipelineRollbackAndBackup/TestPipelineBackupFailureAborts（sitemgmt_test.go:416/472） |
| R-07 Raw 保全 | PASS | span 替换模型（tasks.md A-SM2）：未编辑字节恒不变；TestRawPreservation（parser_test.go:59-74）+ 28 样本 byte 级 round-trip |
| R-08 命令参数数组 | PASS | 见 G-01（本模块为全仓唯一真实外部命令网关：gateway.go:303/:327） |
| R-09 零遥测 | PASS | 模块内唯一网络出站为 ACME（acme/gateway.go，D-09 除外条款）；无其他 http.Client/外呼（grep 核对：模块包内 `http.Client` 仅 acme 一处 :105-108） |
| R-14 服务端校验 | PASS | validate.go 全套（域名/监听/上游/pool/baseMtime/UTF-8/1MiB）+ handler ShouldBindJSON+binding；前端校验仅为预校验（n-form rules），未豁免服务端 |
| R-15 存量扫描导入 | PASS | scanner.go:52-71（ScanDirs→Parse→ExtractSites）；目录缺失跳过（gateway.go:99-104），nginx 未检出 list 空态+nginxAvailable:false（projector.go:136-145，I-06） |
| R-16 隔离写回 | PASS | span 模型构造保证 + TestUpdateSiteIsolationAndRawPreservation（sitemgmt_test.go:536，其余站点 byte 级不变实证） |
| D-02 siteId | PASS | siteid.go:11-26（路径#序号；序号含 dormant A-SM3） |
| D-03 消重/落点 | PASS | scanner.go:73-155 dedupe（items 留最高优先级目录，duplicates[] 如实）；FirstWritableDir（gateway.go:265-273）+ dougong- 前缀（create.go:78） |
| D-06 乐观锁 | PASS | source.go:68-76（baseMtime RFC3339Nano 比对→SITE_010 currentMtime）；测试 :786 |
| D-08/D-11 读时推算/引用推导 | PASS | certcalc.go:28-68（30 天阈值/CERT_EXPIRING_DAYS consts.go:19）+ :112-138（storage_path/origin_path 实时匹配）；imported 元数据读时刷新不落库（:31-39） |
| D-09 ACME ACL | PASS | 账户密钥 0600 文件不入库（acme/gateway.go:199-233）；ACME_TIMEOUT=120s consts.go:24 + ctxTransport 零 goroutine 注入（acme/gateway.go:242-253）；外部概念止步 IssueResult（:72-79） |
| D-16 启停注释/还原 | PASS | edit.go:442-480（dormant ⇄ active byte 级；幂等 :455-457）；TestToggleDisableEnable（:654） |
| ADR-018 角色边界 | PASS | 路由组级 RequireRole（routes.go:39-54）+ 越界 AUTH_008（middleware/role.go:23-26）+ 前端路由 roles:['admin']→403（router/index.ts:137）+ readonly 负空间（列表/证书页无操作列 index.vue:223/:252） |
| SEC-GATE 债务口径 | 不适用 | 本轮门禁探针 `git --version` rc=0（Mimosa 已禁用）；模块零合规桩——nginx 通道为真实网关直写（tasks.md:459「无桩」）→ DEBT-SEC-GATE 不适用 |

## 6. WARN 项汇总

| # | 项 | 说明 |
|---|----|------|
| W-1 | create 响应 listen 类型级不一致（无运行时影响） | 前端 types/site.ts:87 `SiteCreateResult.listen: string[]` vs 后端 dto.go:84 `CreateSiteData.Listen string`（原样回传请求的 string）。当前 UI 未消费该返回字段（SiteCreateModal.vue:175 仅 await），无运行时断裂；同类：types/site.ts:16 `SiteProxy.hostHeader: string \| null` 比后端 ProxyDTO.HostHeader string（dto.go:41）偏宽。建议类型对齐防未来消费踩坑 |
| W-2 | 覆盖率缺口 | service/sitemgmt 78.0%（差 SKILL Service 80% 目标 2pp）；adapter/phpfpm、adapter/acme 包内 0%（Windows 开发机无法实联 php-fpm/ACME，backend/tasks.md:505 如实声明归 Linux 目标机冒烟——沿 service-mgmt dbus 同判）；前端 site-mgmt 无组件级测试（沿 file-manager gate2 W2 先例） |
| W-3 | 陈旧注释（文档漂移，无功能影响） | http.ts:157「后端包络 code 上线数值 401NN」与 utils/errcode.ts 文件头「后端现状…数值 401NN…符号仅存日志」均与现行实现矛盾——response.go:31-34/errcode.go:7-10 已改为 symbol 符号串直上（Gate 2 E1 修复 2026-11-14）；normalizeCode 双通道兼容故无功能影响，建议订正注释 |

## 7. 复验口径

- 本报告全部 grep/覆盖率命令于本轮实跑（backend/ 与 frontend/ cwd，Git Bash）；编译门控四件套按 ask 引用脚本结果未重跑。
- F-1 修复后需复验点：php_fpm create（带/不带 root 两分支）HTTP 级用例 + 前端表单/类型对齐 + INDEX gate2 复评。

---

# 复检第 1 轮（2026-11-15）— 上轮 FAIL 复验 + 回归抽查

**编译门控复跑（脚本，本轮引用）**：build=0 vet=0 test=0 web=0；本轮另实跑 `go test -count=1 -cover`（backend/ 六包）与 `npx vitest run`（frontend/ 全量）。

## R1.0 判定：**WARN**（F-1 闭环；CRITICAL 0 / 契约断裂 0；遗留 W-2R 环境绑定债务——全仓「FAIL→复检收敛 WARN」先例同判，见 account-auth/panel-core/monitoring/service-mgmt/file-manager 各 gate2 note）

## R1.1 F-1 闭环核验（修复方向=后端放宽回归契约，设计文档零改动）

| 方 | 本轮实读证据 | 结论 |
|----|--------------|------|
| 契约基准 | api-site-mgmt.md:354 原文 `root?(type=static 必填)` 未改；interaction site-mgmt.md 新建弹窗字段表「根目录…type=static 必填 / 显隐 type=static」未改 | 修复未倒改文档（backend/tasks.md:502 记录「设计文档唯一事实源，禁止改文档」） |
| 后端 | create.go:42-47 php_fpm 分支仅强制 poolName（root 必填校验已移除，注释标注 Gate 2 F-1 修复）；create.go:61-63 root 显式提供时仍走存在性校验（A-SM6 保持）；renderTemplate create.go:151-154 root 为空时模板省略 root 指令（继承上层配置） | 已修 |
| 前端 | SiteCreateModal.vue:179 `root: f.type === 'static' ? f.root : undefined`、:253 `v-if="form.type === 'static'"` 仅 static 发送/渲染——本就合契约（上轮已判），无需改 | 一致 |
| service 级回归锁 | sitemgmt_test.go:697-709 TestCreatePHPSiteWithoutRoot（php_fpm 无 root 创建必须成功，失败即 fail）+ :720 static 无 root 仍拒（契约必填项不变）；带 root 分支 :736-737（php.example.com Root: www）在位 | 双分支覆盖 |
| HTTP 级回归锁 | module_test.go:432-438 TestAdminFullFlow 增 php_fpm 无 root POST 完整链路 `wantCode 0` | 复验口径要求的 HTTP 级用例已补 |
| 实跑 | `go test -count=1 -cover`（backend/ cwd，六包）全 ok（见 R1.4） | 用例真实通过，非仅存在 |

## R1.2 W-1/W-2/W-3 处置核验

- **W-1 闭环**：types/site.ts:89 `SiteCreateResult.listen: string`（行注释「backend CreateSiteData.Listen string，dto.go:84——Gate 2 W-1 对齐」）↔ dto.go:80-86 `Listen string` 两侧同型；hostHeader :17/:80 收窄为 string；errors.test.ts:78-98 静态断言（listen=string / hostHeader=string 空串语义）防类型漂移回归。
- **W-2 主体收敛**：service 覆盖率 78.0%→**82.6%**（R1.4 实测，过 SKILL Service 80% 目标）；前端补 site-mgmt 域 vitest 10 用例（views/site-mgmt/__tests__/errors.test.ts：SITE_004 弹窗通道/SITE_012 reason/SITE+CERT 21 码文案全覆盖/载荷类型守卫/W-1 静态断言，本轮实跑 ✓）。残留见 R1.5 W-2R。
- **W-3 闭环**：http.ts:157-158 注释订正为「现行后端 symbol 符号串直上（response.go Fail→ec.Symbol()，Gate 2 E1 双侧修复）…W-3 订正」；utils/errcode.ts:4-10 头注释同步（数值 401NN 降格为历史通道兼容兜底表述）。

## R1.3 回归抽查（栈规则复扫 + AL 抽查，本轮实跑，backend/ 与 frontend/ cwd）

后端：
- G-01 `grep -rnE "exec\.Command(Weak)?Context?\(.+\"(-c|/c)\"" --include="*.go" .` 0 命中（rc=1）；exec.Command 全仓 4 处人工核对=2 注释（parser.go:3/gateway.go:31）+2 真实调用（gateway.go:303/:327 均字面量参数数组）——与首轮一致
- G-02 SQL 拼接 / G-03 硬编码凭据 / G-05 panic：复扫均 0 命中（rc=1）
- G-04 层外命中本模块 2 处均注释（certsvc.go:80「→ INSERT pending（机制无审计，A-SM9）」/scanner.go:160「→ INSERT imported 行」）
- G-06 `go func` 6 处全在 cmd/dougong/main.go（:241/:256/:271/:287/:325/:331，行号与首轮一致无漂移；退出路径首轮已核 renewal.go 两级 select ctx.Done）
- G-07 编译门控引用脚本 build/vet/test=0；本轮实跑六包 go test 全 ok

前端：
- F-01/F-02/F-04/F-05 复扫 0 命中（rc=1）；F-03 src/views 0 命中（src/components 目录不存在，与首轮同况如实注明）
- `npx vitest run` 全量 61/61（7 文件全 ✓，site-mgmt errors.test.ts 10 用例在列）

AL 抽查：
- AL-001 routes.go:41-66 18 条路由（GET 两角色 4+GET admin 2+POST admin+CSRF 12）与首轮逐字一致；中间件链 AuthRequired→CsrfGuard(POST)→RequireRole 不变（R-02 无回归）
- AL-002 api/site.ts 18 函数路径（:40/:45/:50/:55/:60/:65/:74/:82/:86/:95/:100/:105/:112/:120/:128/:135/:143/:151）与 routes.go 一一对应，读 6 GET/写 12 POST 方法匹配
- AL-003 修复点：CreateSiteData.Listen string（dto.go:80-86）↔ types/site.ts:89 同型；create req root 三方可选性一致（契约 php_fpm 可选 ↔ 后端已放宽 ↔ 前端仅 static 发送）——首轮 FAIL 点换三方证据链复核全部一致
- R-06 管线抽查：pipeline.go 四步结构在位（写前快照 :14-16 / 原子写 / nginx -t 失败自动恢复 :31-46 / 尾部快照保留清理 :46）；create.go:79 仍走 applyPipeline+reloadAfterWrite

## R1.4 覆盖率复测（本轮实跑 `go test -count=1 -cover`，backend/ cwd）

| 包 | 首轮 | 复检 | 变化 |
|----|------|------|------|
| internal/service/sitemgmt | 78.0% | **82.6%** | +4.6pp，过 SKILL Service 80% 目标 |
| internal/api/sitemgmt | 71.3% | 71.3% | 持平，过 60% 目标 |
| internal/store/sitemgmt | 78.6% | 78.6% | 持平 |
| internal/adapter/nginxx | 53.1% | 53.1% | 持平（round-trip 28 样本在包内直测，fs/exec 原语经 fake 网关间接覆盖） |
| internal/adapter/phpfpm | 0% | 0% | W-2R（无测试文件，环境绑定） |
| internal/adapter/acme | 0% | 0% | W-2R（同上） |
| 前端 views/site-mgmt | 未实测 | 错误分流/类型守卫逻辑层 10 用例 ✓；.vue 组件挂载级 0 | W-2R |

## R1.5 遗留 WARN

| # | 项 | 说明 |
|---|----|------|
| W-2R | 覆盖率残留（环境绑定 + 组件级） | adapter/phpfpm、adapter/acme 包内 0%（Windows 开发机无法实联 php-fpm/ACME 外呼，backend/tasks.md 如实声明归 Linux 目标机冒烟——沿 service-mgmt dbus 先例）；前端 .vue 组件挂载级测试缺席（本轮补的是 views 层逻辑测试）——沿 file-manager gate2 W2 先例 |

SEC-GATE：模块零合规桩维持、外部命令面无新增（exec.Command 仍 2 处真实调用）；上轮探针已过、DEBT-SEC-GATE 不适用，无变化。

## R1.6 复检结论

**判定：WARN**——F-1 按契约方向修复且双侧回归锁在位（service + HTTP 级，实跑通过）；W-1/W-3 闭环，W-2 主体收敛（service 82.6% 达标 + 前端域测试补齐）；CRITICAL 0 / 契约断裂 0；遗留 W-2R 一项（环境绑定覆盖率债务 + 组件级测试缺席，全仓先例同判 WARN）。复验口径三件套齐：php_fpm create 带/不带 root 双分支用例（service 级 :697-720 与 :736 + HTTP 级 :432-438）+ 前端表单/类型对齐（R1.2）+ INDEX gate2 复评（本轮同步更新）。
