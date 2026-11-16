# Gate 2 验证报告 — 代码→交付门控

**模块**：计划任务与备份（cron-backup）
**扫描路径**：backend/（internal/{api,service,store}/cronback、internal/adapter/{cronx,backupx}、internal/errcode/cronbackup.go、internal/store/migrations/006_cron_backup.sql、internal/service/dbmgmt/ohs.go 增补、internal/config cron 段、cmd/dougong/main.go 装配段）+ frontend/src/（types/cron.ts、api/cron.ts、stores/cron.ts、views/cron-backup/ 7 文件+expr-validate.ts、router/index.ts:200-213 追加段、layouts/MainLayout.vue 菜单映射）
**扫描时间**：2026-11-15（报告日期沿用流水线批次 2026-11-14 命名）
**契约基准**：profiles/dougong-panel/workspace/02-detailed/api/api-cron-backup.md v1.0
**规则链**：引擎 GR-01~GR-06 ⊕ backend-rules §7 G-01~G-07 / frontend-rules §6 F-01~F-06 ⊕ business-concepts R-01~R-18（适用项）
**编译门控（脚本已执行，本报告引用不重跑）**：build=0、vet=0、test=0、web=0

---

## 0. 判定总览

**判定：FAIL**（1 项前后端契约断裂 E1；🔴 CRITICAL 栈规则违例 0；其余检查全过）

| 类别 | 结果 |
|------|------|
| 后端 G-01~G-06 实跑 | 全过（G-01 含 1 处登记性命中，人工核对 PASS，见 §1） |
| 前端 F-01~F-05 实跑 | 全过（全 0 命中） |
| G-07 / F-06 | 引用脚本 build/vet/test/web=0 全绿；本检查员另实跑模块 5 包 go test -cover 全 ok + vitest 105/105 |
| AL-001/002/004/005、包络、DDL | 全对齐 |
| **AL-003 DTO 字段 vs 前端类型** | **FAIL（E1：`ref` 字段后端缺失，见 §3）** |
| 业务规则 R-02/03/08/09/11/14 | 全过 |
| SEC-GATE 探针 | 通过（Mimosa 已禁用）；模块真实实现无合规桩 → DEBT-SEC-GATE 不适用 |
| WARN | W1 service 覆盖率 72.4%（差 80% 目标 7.6pp）、W2 前端组件/store 0 挂载测试、W3 cronx/backupx 实联归 Linux 冒烟（覆盖率余量） |

---

## 1. 后端规则扫描（backend-rules §7，实跑 grep）

| # | 检查项 | 结果 | 证据（命令+输出/文件:行） |
|---|--------|------|--------------------------|
| G-01 | 无 shell 拼接 | PASS（登记性命中） | `grep -rnE 'exec\.Command(Weak)?Context?\(.+"(-c\|/c)"' --include="*.go" internal/ cmd/` **1 命中**——`internal/adapter/cronx/runner.go:145` `exec.CommandContext(cctx, r.shellBin, "-c", command)`。人工核对（独立复核 tasks.md A-C2/T07 登记）：①命令串为**独立 argv 元素**（参数数组形态，零拼接——G-01 禁的是「拼接含用户输入的字符串」）；②命令原文取自 crontab 投影、不经请求参数（I-08，service.go:122-137 按 jobKey 从 s.tab.List 匹配取 command，req 仅 jobKey）；③D-05 设计决策明文「以面板运行用户身份 shell 语义代执行」（api-cron-backup.md §1.3 D-05），与 cron 自身执行同信任级；④端点 admin+CSRF（routes.go:58 write 组）。本模块其余 exec 点逐个核对全为字面量/参数数组：cronx/gateway.go:41 `crontab -l`、:59 `crontab -`（stdin 管道写回，A-C11）、runner.go:51 journalctl 字面量、backupx/rsync.go:37 `exec.CommandContext(ctx, e.rsyncBin, args...)`（append 构造的参数切片整组传入；`-e ssh -p %d` 端口为 validate.go:94 服务端校验后的 1..65535 整数经 fmt %d 格式化，无注入面） |
| G-02 | 无 SQL 拼接 | PASS | `grep -rnE 'Sprintf\(.+(SELECT\|INSERT\|UPDATE\|DELETE)' --include="*.go" internal/ cmd/` 0 命中（exit=1）。store/cronback 动态 WHERE=静态条件串+? 参数数组（run_repo.go:211-238 runWhere）、Rotate IN 列表=事务内 SELECT 收集的纯数字 id+占位符数组参数绑定（run_repo.go:110-121，注释明示 G-02） |
| G-03 | 无硬编码凭据 | PASS | `grep -rnEi '(password\|secret\|token\|apikey)\s*=\s*"[^"]{8,}"' --include="*.go" internal/ cmd/ \| grep -v _test` 0 命中（exit=1）；rsync target_config 无凭据字段（D-06，dto.go:85-90 仅 path/host/user/port）；数据库源凭据经 dbmgmt OHS 域内解密（ohs.go DumpStream→governTarget，⚠A18 精神） |
| G-04 | SQL 只在 store 层 | PASS | `grep -rlE "(SELECT\|INSERT\|UPDATE\|DELETE) " --include="*.go" internal/ cmd/ \| grep -v internal.store \| grep -v _test` 层外命中中本模块 3 处——service/cronback/{consts.go:32, scheduler.go:81, run.go:54,294} 人工核对**全为注释文字**（「INSERT run 同事务」「轮转 DELETE」等，非执行 SQL）；其余层外命中为前序模块已裁定项（service/auth/user.go:114、fileman/sitemgmt/dbmgmt 注释与 A-DB10 引擎侧）；清单中 `internal\store\*` 条目为 Windows 反斜杠路径绕过 `-v internal/store` 过滤的扫描工件（文件确在 store 层，前序报告同判） |
| G-05 | 无 panic 滥用 | PASS | `grep -rn "panic(" --include="*.go" internal/ cmd/ \| grep -v _test` 0 命中（exit=1）；模块内 recover 均为 panic 隔离防御（run.go:281-287 runOneSafely、scheduler.go:44-48、main.go:432-436/449-453/462-466），非 panic 调用 |
| G-06 | goroutine 有退出路径 | PASS | `grep -rn "go func" --include="*.go" internal/ cmd/ \| grep -v _test` 11 处——本模块新增 3 处全在 cmd/dougong/main.go：:442 调度器（RunScheduler scheduler.go:26-38 select ctx.Done→return nil）、:459 执行 worker×2 循环（RunExecutor run.go:269-278 同法），均 wg.Add/Done 配对+ctx 取消退出+双层 panic 隔离（main.go:426-470 实读核对）；其余 8 处为前序模块已裁定（terminal/monitoring/sweeper/cert/docker 读泵等） |
| G-07 | 编译+vet+test 全绿 | PASS（引用+补测） | 脚本实跑 build=0/vet=0/test=0/web=0（ask 提供，未重跑）；本检查员另实跑 `go test -cover -count=1 ./internal/service/cronback/ ./internal/api/cronback/ ./internal/store/cronback/ ./internal/adapter/cronx/ ./internal/adapter/backupx/` → 5 包全 ok（72.4%/70.5%/72.0%/35.1%/58.1%） |

**SEC-GATE 探针（本检查员实跑）**：向 backend/.secgate-probe.go 写入 `package dougong` + `exec.Command("true")` 字面量参数数组 → 写入+读取+删除成功（输出 `PROBE: write+read+delete OK (Mimosa disabled)`）。探针通过 + 本模块外部命令为真实实现（crontab/journalctl/rsync/sh 通道网关直写，无合规桩；Windows 开发机不可达组件以纯函数层+fake 测试覆盖并如实声明，非桩）→ **DEBT-SEC-GATE 不适用**，不构成 WARN/FAIL。

## 2. 前端规则扫描（frontend-rules §6，实跑 grep）

| # | 检查项 | 结果 | 证据 |
|---|--------|------|------|
| F-01 | 无 Options API | PASS | `grep -rLn "script setup" --include="*.vue" src/` 0 命中（exit=1）；本模块 7 个 .vue 逐读全为 `<script setup lang="ts">` |
| F-02 | 无裸 any | PASS | `grep -rnE "(:\s*any\|as any)" --include="*.ts" --include="*.vue" src/` 0 命中（exit=1）；types/cron.ts 服务端 data 用 unknown+收窄（cronValidateErrors/isStringArray :289-302） |
| F-03 | 请求只走 api 层 | PASS | `grep -rnE "(axios\|fetch)\(" --include="*.vue" src/views src/components` 0 命中（exit=1）；15 Action 全在 api/cron.ts，特殊通道 requestText（http.ts:258）/downloadFile（http.ts:198）经 api 层 |
| F-04 | 无混入其他组件库 | PASS | `grep -rnE "from '(antd\|element\|ant-design-vue)'" src/` 0 命中（exit=1）；UI 全 n-* |
| F-05 | 无原生弹窗 | PASS | `grep -rnE "(^|\W)(alert\|confirm)\(" --include="*.vue" --include="*.ts" src/` 0 命中（exit=1）；反馈走 useMessage/useDialog 语境的 NPopconfirm（危险操作删除/触发均确认框，index.vue:132-176、backup/index.vue:168-180） |
| F-06 | typecheck+build 全绿 | PASS（引用+补测） | 脚本 web=0（ask 提供，未重跑）；本检查员另实跑 `npx vitest run` → 10 文件 105/105 全过（本模块归属 cron.test.ts 5/5） |

栈规则 ⚠️ 项抽查：轮询定时器配对清理+页面可见性暂停（RunDetailDrawer.vue:51-96 setInterval/clearInterval/onUnmounted/visibilitychange，§4.5 ✓）；防抖定时器 onUnmounted 配对（expr-validate.ts:1-30 ✓）；枚举列 n-tag 语义色经 types/enums.ts 映射表（CRON_SOURCE/BACKUP_* DictItem，render 无硬编码文案，§4.2 ✓）；分页 remote 受控（backup/index.vue:355-363 PaginationProps+itemCount=runTotal ✓）；表单 n-form rules+canSave 门控（BackupJobDrawer.vue:166-188 ✓）；路由懒加载（router/index.ts:207,213 `() => import(...)` ✓）。

## 3. 前后端接口对齐（以 api-cron-backup.md v1.0 为基准）

| # | 检查项 | 结果 | 证据 |
|---|--------|------|------|
| AL-001 | 路径/方法 vs API 文档 | PASS | routes.go:26 Prefix=/cron；:39-43 readBoth GET 5（job/list、log/tail、backup-job/list、backup-run/list、backup-run/detail）；:49 readAdmin GET backup-run/download（admin 专属，ADR-018）；:54-62 write POST 9（job/{create,update,delete,validate,trigger}+backup-job/{create,update,delete,run}）——与 api §3.3 权限矩阵 15 接口逐行一致；链序 InitGuard→AuthRequired→[GET 角色] / [POST CsrfGuard→RequireRole(admin)] 与 §3.1 一致 |
| AL-002 | 前端调用路径 vs 后端 | PASS | api/cron.ts 15 函数路径逐一对应：jobList:37 /cron/job/list、jobCreate:42、jobUpdate:47、jobDelete:55、jobValidate:60、jobTrigger:65、logTail:70（requestText 文本通道，D-04）、backupJobList:77、backupJobCreate:82、backupJobUpdate:90、backupJobDelete:98、backupJobRun:103、backupRunList:110（query 6 键）、backupRunDetail:125（query runId）、backupRunDownload:134（downloadFile 流式，D-10/D-16） |
| **AL-003** | DTO 字段 vs 前端类型 | **FAIL** | **E1 契约断裂：`ref` 字段后端缺失**（详见证链表下） |
| AL-004 | 枚举值前后端一致 | PASS | 五枚举三方同值：cron_source panel/existing（cronx/crontab.go:19-20 ↔ enums.ts:360 ↔ api §3.4）、backup_source_type site_dir/database（store/cronback/models.go:25-26 ↔ enums.ts:367）、backup_target_type local/rsync（models.go:31-32 ↔ enums.ts:374）、backup_trigger cron/manual（models.go:37-38 ↔ enums.ts:381）、backup_run_status running/success/failure（models.go:43-45 ↔ enums.ts:388，ADR-016） |
| AL-005 | 错误码前端可识别 | PASS | CB_001~009 九码：errcode/cronbackup.go:8-24 符号串逐字 ↔ 前端 CB_ERROR_TEXT（types/cron.ts:276-286）九键全覆盖（cron.test.ts:5 断言键名逐字）；分流消费在位——CB_001 字段级（isFieldErrorList 回填 Drawer/Modal）、CB_002 段位级（cronValidateErrors）、CB_003/004 warning+自动重拉（index.vue:47-67、backup/index.vue:206-232）、CB_006 互斥 warning（:206）、CB_008/009 下载前置 warning（RunDetailDrawer.vue:117-119）、CB_007 轮转消失关 Drawer（stores/cron.ts:263-270）；AUTH 码族复用全局拦截器 |
| 包络 | 响应包络对齐 | PASS | response.go:20-27 `{"code":0,"message":"ok","data":...}`、失败 code=符号串 HTTP 200 ↔ http.ts Envelope ↔ api §3.1（message）逐字；log/tail 走 200+text/plain（handler.go:141-142）、download 走 200+attachment 流（handler.go:268-273），两特殊通道与 §3.1 一致 |
| DDL | 迁移 vs §5.3 | PASS | 006_cron_backup.sql:6-33 与 api §5.3 逐行比对全同（backup_job 8 列+UNIQUE/CHECK、backup_run 11 列+2 CHECK+DEFAULT running、idx_run_job/idx_run_status/idx_run_running 部分唯一索引 I-01/D-08）；复用 migrate.go 版本表机制 |
| 常量 | 档位常量对齐 | PASS | TRIGGER_TIMEOUT=120s（consts.go:57）、PREVIEW_COUNT=5（:59）、LOG_LINES 默认 200 档位 [50,100,200,500,1000]（:61,80 + NormalizeLogLines :83-100 越档归 200）、SCHEDULER_TICK=1min（:63）、OUTPUT_MAX=64KB 尾部截断（cronx/runner.go:120-123,168-177）↔ api §3.4 常量表逐字 |

### E1 证链（AL-003 契约断裂——FAIL 驱动项）

| 侧 | 证据 |
|----|------|
| 契约基准 | api-cron-backup.md §4.1.4（:216）：GET /cron/backup-job/list `data=items[{id, name, **sourceSummary[{type, ref}]**, targetType, targetBrief, ...}]`——sourceSummary 元素形状明文 `{type, ref}` |
| 后端实际产出 | service/cronback/dto.go:76-82 `SourceItemDTO{type, path?, instanceId?, database?}` **无 ref 字段**；run.go:31 `SourceSummary: toSourceDTOs(j.SourceItems)`（:408-417 原样转换）；`grep -rn '"ref"\|Ref' internal/{service,store,api}/cronback/ internal/adapter/backupx/` 0 命中、无自定义 MarshalJSON——list 实发 `{"type":"site_dir","path":"/x"}` / `{"type":"database","instanceId":3,...}` |
| detail sources 同断 | backupx/executor.go:40-46 `SourceSpec{type, path?, instanceId?, instanceName?, database?}` 无 ref；BackupRunDetail.Sources（dto.go:194）直用该形态——api §4.2.4 sources[] 前端消费同为 `{type, ref}` |
| 前端消费 | types/cron.ts:132-136 `BackupSourceRef{type, ref}`（注释明示「list sourceSummary 与 run detail sources 共用」）；backup/index.vue:70-71 渲染 `` `目录 ${s.ref.length > 18 ? ...}` `` ——**运行时 s.ref=undefined，site_dir 行 `s.ref.length` 抛 TypeError**（表格 render 内），database 行显示「DB undefined」；RunDetailDrawer.vue:213 `{{ s.ref }}` 空白、:45-48 sourceFailed(ref) 失败源标注永不命中 |
| 测试缺口 | 后端 module_test.go:483-498 job/list 仅断言 items 数/nextFireAt/lastRun，未断言 sourceSummary 形状；前端 cron.test.ts 不覆盖响应形状——断裂双侧测试均未拦截 |
| 修复方向（建议） | 后端投影层补 ref（site_dir→path；database→「实例名·库名」或 instanceId+database 组合串，D-12 快照语义同构 jobName 先例），detail sources 同法（A-C5 manifest 语义外发前映射）；或经契约升版改形状——二选一后双侧补形状断言 |

## 4. 业务规则适用项（R-01~R-18）

| 规则 | 结果 | 证据 |
|------|------|------|
| R-02 会话 Cookie/CSRF | PASS | POST 9 接口全链 AuthRequired→CsrfGuard→RequireRole(admin)（routes.go:52-53）；GET 只读豁免 CSRF（§3.3 一致）；token 不入前端代码（stores/cron.ts 无 localStorage） |
| R-03 变更写审计 | PASS | 10 契约动作码+1 增补 system 码全用（consts.go:17-36，A-C13 backup_run_interrupted）；SQLite 域内变更与审计**同事务**——CreateBackupJob（service.go:170-187）、RunBackupJob（run.go:65-85，A-C3）、executeRun 终态+轮转+backup_rotated 同事务（run.go:296-338，D-09）；crontab/命令/传输类紧随写入 recordAudit 重试一次降级日志（service.go:270-277）；download 记 **security** 类审计（run.go:259-265，数据取出口径）；trigger 按结果记 success/failure（service.go:139-144） |
| R-08 参数数组禁拼接 | PASS | 见 G-01：crontab -l/-、journalctl、rsync、sh 全参数数组；runner sh -c 为 argv 元素形态（I-08 命令原文不经请求）+ tasks.md A-C2/T07 登记 + 本轮独立复核维持 |
| R-09 零遥测零回传 | PASS | 模块唯一出站流量=rsync 推送/拉回**用户主动配置**的远程目标（api §1.5 明文 R-09 例外语义，与 ACME 同判）；无 phone-home/账号绑定/更新检查面（模块 import 面仅本地 exec+SQLite+OHS） |
| R-11 备份保留份数轮转 | PASS | Rotate（run_repo.go:77-128）：保留最近 N 条 success、删除 id<保留集最小 id 的全部记录（含更早 failure，D2 口径-5）；与终态回写同事务+system 审计 backup_rotated 留痕（run.go:319-332）；store_test 轮转语义用例在位；产物清理事务后 best-effort 失败留存（run.go:342-352，A-C4） |
| R-14 服务端校验不信任前端 | PASS | handler 层 ShouldBindJSON+binding required（dto.go 全 req 结构）；service 层四要素逐字段 CB_001（validate.go:25-121：name≤64/I-09 源类型与绝对路径与库名正则/实例存在性 InstanceRefChecker/I-10 目标匹配/retention 1..100/A-C8 数据目录四向重叠拒绝）；list query 服务端校验（run.go:117-155 status/trigger 枚举+from/to RFC3339）；runId 正整数（handler.go:234-239,252-257）；download file∈run.files 白名单（run.go:220-223，裸名精确匹配——产物名为执行器生成，无路径注入面） |

不适用的规则（本模块无涉及面）：R-01/R-05/R-06/R-07/R-15/R-16/R-17/R-18（登录/nginx/文件/终端域）；R-04 文件路径白名单——模块不做用户任意路径读写（产物目录=服务端生成的 {目标根}/{job 名}/{时间戳}/，下载经 files 白名单），A-C8 数据目录防递归为 R-04 精神在本域的落点。

## 5. 测试覆盖率（本检查员实跑）

| 层级 | 覆盖率 | 达标（SKILL §3 目标） |
|------|--------|----------------------|
| 后端 service/cronback | 72.4% | NO（80% 目标差 7.6pp → **W1**） |
| 后端 api/cronback（Controller） | 70.5% | YES（≥60%） |
| 后端 store/cronback | 72.0% | —（无硬性目标，表驱动 5 用例含轮转/互斥/终态不可逆） |
| 后端 adapter/cronx | 35.1% | 环境绑定（crontab/journalctl 实机通道 Windows 不可达，纯函数层已覆盖 → **W3**） |
| 后端 adapter/backupx | 58.1% | 本地真实产物 5 用例在位；rsync 通道归 Linux 冒烟（→ W3） |
| 前端组件 | 挂载级 0 测试 | NO（vitest 105/105 中本模块归属仅 utils 层 cron.test.ts 5 用例 → **W2**，沿 terminal/dbmgmt 先例） |

命令：`go test -cover -count=1 ./internal/service/cronback/ ./internal/api/cronback/ ./internal/store/cronback/ ./internal/adapter/cronx/ ./internal/adapter/backupx/`（5 包 ok）；`npx vitest run`（10 files 105/105）。

## 6. 其他核验

- **middleware**：无新增（api/middleware/ 无 cronback 文件），复用 InitGuard/AuthRequired/CsrfGuard/RequireRole 四件套（routes.go:34-53）——api §3.1「复用 account-auth 中间件链」明文 ✓
- **main.go 装配**（:308-347,438-470）：cronStore/backupExec/cronSvc 构建注入 panelDataDir（A-C8 基准）、启动恢复 RecoverOrphans（D-08，:321-325）、菜单 cron/backup Order 750/760 两角色（:328-331，A-C12）、模块挂载（:347）、调度 1+worker×2 goroutine（G-06 ✓）
- **dbmgmt OHS 增补**（ohs.go:27,46,71 DumpStream/DatabaseNames/InstanceArchive）——DumpDatabase 消费登记兑现（api §3.2/§5.1），凭据域内解密
- **config cron 段**（config.go:29-30,53-63,89,160-163）：crontab/journalctl/rsync/shell 四二进制可配置+默认值，无硬编码公网地址
- **前端菜单图标**：MainLayout.vue:84-85 cron→TimerOutline、backup→CloudUploadOutline 已映射（docker-mgmt W1 同类问题在本模块不存在）
- **router 追加段**（router/index.ts:200-213）：/cron、/cron/backup 两路由 requiresAuth 继承布局+懒加载 ✓
- **已声明的实现偏差（INFO，非违规）**：编辑备份任务源/目标无法回填——api list 不下发结构化定义且无 detail 接口（契约自身留白），整体替换语义+NAlert 概要如实呈现（BackupJobDrawer.vue:10-13 头注释，frontend INDEX 已声明）
- **INFO**：engine-rules.md 缺席（沿前序各模块 INFO 先例，按 SKILL 分层说明执行 GR 机制项）；包络 msg/message 以 api 契约 message 为准（全仓 D-07 定稿）；cronx/backupx 实机联调（crontab/journalctl/rsync 二进制交互）归属 Linux 目标环境部署冒烟（tasks.md 环境说明如实声明）

## 7. 总结

**判定：FAIL**

- 通过：后端 G-01~G-06 实跑全过（G-01 登记性命中人工核对 PASS）、前端 F-01~F-05 全 0 命中、G-07/F-06 引用脚本全绿+模块 5 包/vitest 105 实跑全过、AL-001/002/004/005+包络+DDL+常量全对齐、业务规则适用项 R-02/03/08/09/11/14 全过、SEC-GATE 探针通过且模块无桩（DEBT-SEC-GATE 不适用）
- **CRITICAL/契约断裂：1 项（E1）**——backup-job/list `sourceSummary[].ref` 与 backup-run/detail `sources[].ref` 后端缺失（前端渲染 undefined 且 site_dir 行抛 TypeError）→ 返回 B/F 修复后复验
- WARN 3 项：W1 service 覆盖率 72.4%（80% 目标差 7.6pp）；W2 前端 views/cron-backup 组件+store 0 挂载测试（沿先例）；W3 cronx 35.1%/backupx 58.1% 环境绑定归 Linux 冒烟
- INFO 4 项：编辑回填偏差已声明、engine-rules.md 缺席、包络 msg/message 全仓口径、实机联调归 Linux 冒烟

---

# 复检第 1 轮（2026-11-15）

**复检范围**：上轮 FAIL 唯一驱动项 E1 闭环核验 + 栈规则/契约/业务规则抽查回归；编译门控复跑（脚本）build=0、vet=0、test=0、web=0（ask 提供，引用不重跑）。
**判定：WARN**（沿全仓先例：CRITICAL 0/契约断裂 0 → WARN 收敛；E1 双侧修复+测试锁定，遗留 WARN 3 项维持）

## 1. E1 闭环核验（FAIL 驱动项）

| 侧 | 修复落点（本轮实读证据） |
|----|--------------------------|
| 后端 DTO | service/cronback/dto.go:77-87 `SourceItemDTO` 增 `Ref string json:"ref"`（注释明示 E1 修复：site_dir→目录路径 / database→「实例名·库名」，库名缺省=\* 整实例语义；path/instanceId/database 保留超集供编辑回填） |
| 后端投影 | run.go:415-434 `toSourceDTOs`（list sourceSummary 投影）：site_dir→`backupx.SourceRef(it.Path)`、database→`backupx.DBRef(instanceLabel, db)`；:436-444 `instanceLabel` 经档案 OHS 解析实例显示名，悬空/异常→`#<id>` 如实可辨（D-12 弱引用合法态） |
| 后端执行链 | backupx/executor.go:43-50 `SourceSpec` 增 `Ref json:"ref"`；:52-61 `SourceRef`/`DBRef` 组装函数；:184-186 manifest 落盘前空 Ref 兜底；:216 整实例展开后逐库重写 Ref（manifest 完备性） |
| detail 通道 | run.go:192-195 detail.sources 经 `ReadManifestPath` 读 manifest 语义外发（A-C5），`detail.Sources = m.Sources`——manifest 含 Ref（executor 落盘）→ §4.2.4 `sources[]{type, ref}` 闭环 |
| 前端归一层 | utils/cronback.ts:1-75（新增）：`normalizeSourceRef` 双分支——①契约分支 ref 直通（后端补 ref 后零改动接管）②现状分支 site_dir→path / database→`实例 {id}·{库}` 推导；无可推导字段→`ref=''` 降级不虚构；非对象/缺 type→null 剔除；`normalizeBackupJobListData`/`normalizeBackupRunDetail` 清单级入口 |
| 前端接入 | api/cron.ts:85 backupJobList→`.then(normalizeBackupJobListData)`、:136-137 backupRunDetail→`.then(normalizeBackupRunDetail)`——归一在 api 层收口（account-auth E1/E2 同判先例） |
| 前端消费降级 | backup/index.vue:63-81 sourceCell `s.ref === ''` 降级（s.ref.length 恒安全不再 TypeError）、RunDetailDrawer.vue:224-225 `v-if="s.ref !== ''"` 条件渲染+`sourceFailed(s.ref)` 失败源标注恢复命中、BackupJobDrawer.vue:216-218 编辑态概要降级 |
| 测试锁定（后端 service 级） | cronback_test.go:617-618 `detail.Sources[0].Ref == "local·shop"`；:667-673 list `SourceSummary[0].Ref == "/var/www/site"`（site_dir）+ `[1].Ref == "local·shop"`（database）——投影函数形状逐值断言 |
| 测试锁定（后端 HTTP 级） | module_test.go:537-565：detail 响应体形状结构含 `Ref string json:"ref"`（:554）+ 断言 `detail.Sources[0].Ref == "/var/www/site"`、`[1].Ref == "local·shop"`（:563，注释明示「E1 回归防线」） |
| 测试锁定（前端） | cron.test.ts:61-131+（5→15 用例）：契约形状直通/现状形状推导/空串降级/非对象剔除/归一输出恒 `{type:string, ref:string}`（s.ref.length 不再对 undefined 崩）/job list items[].sourceSummary 逐项归一 |

**结论：E1 闭环**——契约 §4.1.4 `sourceSummary[{type, ref}]` 与 §4.2.4 `sources[]` 后端全链产出 ref、前端 api 层归一双通道兜底、双侧三处测试防线锁定形状。另注（INFO 级，非缺陷）：HTTP 级 job/list 响应未直接断言 sourceSummary 形状（module_test.go:495 断言 items 数/nextFireAt/lastRun），但 service 级 cronback_test.go:667-673 已锁定投影函数输出且 handler 为薄壳透传，防线等效。

## 2. 抽查回归（复扫结果）

| 检查 | 结果 | 证据（命令+输出） |
|------|------|-------------------|
| G-01 | PASS（登记性命中维持） | 复跑 `grep -rnE 'exec\.Command(Weak)?Context?\(.+"(-c\|/c)"'` 仍唯一命中 cronx/runner.go:145——argv 元素形态（行尾注释现明示「argv 数组：["/bin/sh","-c",<原文>]」）、命令原文取自 crontab 投影（I-08）、admin+CSRF，上轮 A-C2/T07 登记判定维持 |
| G-02/G-03/G-05 | PASS | 复跑三项 grep 均 0 命中（exit=1），与上轮一致 |
| G-04 | PASS | 本模块层外命中仍为 service/cronback/{consts.go:32, scheduler.go:81, run.go:55,294} 4 处，逐行核对全为注释文字（「INSERT run 同事务」「轮转 DELETE」等）；`internal\store\cronback\*` 4 条为 Windows 反斜杠路径绕过 `-v internal/store` 的扫描工件（文件确在 store 层，上轮同判） |
| G-06 | PASS | 复跑 `grep -rn "go func"` 11 处（较上轮 +2：docker/logs.go:165+handler_ws.go:86 归 docker-mgmt 域）；本模块 2 站点 main.go:442 调度器（RunScheduler）+:459 worker×2（RunExecutor）均 wg 配对+ctx 取消退出+双层 panic 隔离（本轮 :420-470 实读）；:498/:504 为 HTTP/HTTPS serve goroutine（serveErr 通道收口，ErrServerClosed 排除），全编排合法 |
| F-01~F-05 | PASS | 复跑五项 grep 全 0 命中（exit=1） |
| AL-001/002 | PASS | routes.go:39-62（GET 6=两角色 5+admin 专属 download / POST 9）↔ api/cron.ts:41-147（15 函数路径逐一对应，本轮清点 15/15 一致）；list/detail 归一 .then 不改路径契约 |
| R-11/R-03 | PASS | Rotate（run_repo.go:74-121）+ run.go:320 同事务调用、:324 victims>0 时 system 审计 backup_rotated 留痕（consts.go:34）；recordAudit :241（download security 类）/:364（interrupted）在位 |
| G-07/F-06 | PASS（引用+补测） | 编译门控复跑（ask 提供）：build=0、vet=0、test=0、web=0；本检查员另实跑模块 5 包 `go test -cover -count=1` 全 ok + `npx vitest run` 10 files 115/115 |

## 3. 测试与覆盖率（本轮实跑）

`go test -cover -count=1 ./internal/service/cronback/ ./internal/api/cronback/ ./internal/store/cronback/ ./internal/adapter/cronx/ ./internal/adapter/backupx/` → 5 包全 ok：service 72.8%（上轮 72.4%，ref 投影新代码入测微升，仍差 80% 目标 7.2pp → **W1 维持**）、api 70.5%、store 72.0%、cronx 35.1%、backupx 58.8%（+0.7pp）。`npx vitest run` → 10 files **115/115**（上轮 105，新增 10 用例即 cron.test.ts E1 归一层系列 5→15）。

## 4. 复检结论

**判定：WARN**

- E1 契约断裂闭环（后端 ref 全链产出+manifest 语义+前端 api 层归一+双侧三处形状断言），CRITICAL 0
- 栈规则/契约/业务规则抽查回归零违例；编译门控复跑全 0+模块 5 包/vitest 115 实跑全绿
- WARN 3 项维持：W1 service 覆盖率 72.8%（差 7.2pp）；W2 前端 views/cron-backup 挂载级 0 测试（cron.test.ts 15 用例均 utils 层，沿 terminal/dbmgmt 先例）；W3 cronx 35.1%/backupx 58.8% 环境绑定归 Linux 冒烟
- INFO 沿用首轮 4 项 + 本轮新增 1 项（HTTP 级 job/list sourceSummary 形状断言缺席，service 级等效锁定）

---

# WARN 修复轮（2026-11-16）

**范围**：上列 WARN 3 项逐项处置（用户指派轮；后端代码+前端代码，无契约文档改动——三项均为测试覆盖类，契约面零变更）。

| 项 | 内容 | 处置 | 证据（file:line）／理由 |
|----|------|------|--------------------------|
| W1 | service/cronback 覆盖率 72.8%（差 80% 目标 7.2pp） | **已修复（闭环）** | 新增 `backend/internal/service/cronback/warnfix_test.go`（15 测试函数）+ `cronback_test.go` fakeExec 增 `manifestErr` 注入点（cronback_test.go:121,146-147）。补齐复检轮 0% 函数全 8 个——ListBackupRuns（含筛选四非法分支/briefPtr 截断与 null/分页兜底钳制/id 倒序，warnfix_test.go TestListBackupRunsQueryAndBrief）、DeleteBackupJob（成功+CB_004+FR-09 run 不级联+事务错误矩阵）、TailCronLog（三分支）、RunExecutor（worker 消费+ctx 取消退出）、runOneSafely（panic 隔离+failRunDirect 补偿）、failRunDirect（成功/n==0/仓储错误三分支）、briefPtr、RunScheduler（取消退出）；并覆盖低覆盖分支——targetBrief rsync 双形态（含 User 空历史行防御分支）、jobPortHint rsync/已删任务、instanceLabel 悬空 `#404·shop` 与档案异常、recordAudit 重试一次+降级、auditEntry actor 装配、executeRun 六错误分支（BeginTx/finishFailure/commit failure/finishSuccess/rotate/commit success）、CreateBackupJob/UpdateBackupJob/DeleteBackupJob/RunBackupJob 事务错误矩阵、ListBackupJobs 清单与 LatestByJob 错误、scheduleTick List 错误与非法表达式防御跳过、fireCronRun 队列满补偿与仓储泛化错误、enqueue 队列满、NextFireStr 非法/无命中、statSize 失败、detail manifest 三分支（ErrRemoteManifest/读错误/空 manifest）、translateCronTabErr/translateJobErr wrap 分支。装置：failStore/failTx/failJobRepo/failRunRepo/stubTx/panicExec/failAudit/failLogs/errArchive。**实测 `go test -cover -count=1` service/cronback 72.8%→93.8%**（超 80% 目标 13.8pp），`-race` 亦 ok。装置设计备注：db.go:39 `SetMaxOpenConns(1)` 下，服务在 BeginTx 前走连接池查询的路径（RunBackupJob→GetByID）不能预占真实连接注入事务错误——以 stubTx 无连接替身承载（warnfix_test.go:192-205 注释明示） |
| W2 | 前端 views/cron-backup 组件+store 0 挂载测试（vitest 115 中模块归属仅 utils 层 15） | **部分修复（store 级闭环，挂载级维持）** | store 级闭环：新增 `frontend/src/stores/__tests__/cron.test.ts` 12 用例——loadJobs 成败流（吞错保持上一投影+loaded 不回退+在途防抖）、loadLogTail/setLogLines 档位联动与失败保留旧文本、loadRuns 筛选 null→undefined 参数投影+setRunFilter page 复位+setRunPage/Size 联动、inFlight 前缀键互斥+resetCronView 清空、openRunDetail 迟到响应守卫、CB_007 关 Drawer+detailError 文案+重拉列表（api D-09）、轮询转终态刷新历史一次、markHighlight 10s 定时器+resetBackupView 配对清理（vi.useFakeTimers）、Dialog/Drawer/触发结果态开关。全量 vitest 155→**167/167** 实跑全绿。**挂载级维持**：`@vue/test-utils` 与 DOM 测试环境（happy-dom/jsdom）均不在 frontend/package.json devDependencies（本轮核验），加装依赖超出本轮修复面归后续迭代——panel-core W4/service-mgmt W1/database-mgmt W2 同判先例 |
| W3 | cronx 35.1%/backupx 58.8% 环境绑定（crontab/journalctl/rsync 二进制 Windows 开发机不可达） | **维持** | 覆盖缺口全部位于外部二进制交互通道（cronx crontab/journalctl 实机、backupx rsync 远端），本机无对应运行环境无法真实执行；纯函数层已有测试覆盖（复检 §3）。按 tasks.md 环境说明与首轮 §5 判定，归 Linux 目标环境部署冒烟；不在本机硬造桩（虚造通过率违反如实声明纪律） |

## 修复轮自验（本轮实跑）

| 检查 | 命令 | 结果 |
|------|------|------|
| 后端编译 | `go build ./...`（cwd=backend/） | exit 0 |
| 后端 vet | `go vet ./...`（cwd=backend/） | exit 0 |
| 后端全量测试 | `go test -count=1 ./...`（cwd=backend/） | 39 包全 ok、0 FAIL |
| 模块 5 包覆盖率 | `go test -cover -count=1 ./internal/service/cronback/ ./internal/api/cronback/ ./internal/store/cronback/ ./internal/adapter/cronx/ ./internal/adapter/backupx/` | 5 包 ok：service **93.8%**（W1 达标）/api 70.5%/store 72.0%/cronx 35.1%/backupx 58.8%（W3 维持项原值） |
| service 包竞态 | `go test -race -count=1 ./internal/service/cronback/` | ok（93.8%，RunExecutor/RunScheduler goroutine 用例无竞态） |
| 前端全量测试 | `npx vitest run`（cwd=frontend/） | 13 文件 167/167（上轮 155+新增 cronStore 12） |
| 前端构建 | `npm run build`（cwd=frontend/，vue-tsc --noEmit + vite build） | exit 0 |

**修复轮结论**：W1 闭环（93.8% 超 80% 目标）、W2 store 级闭环+挂载级沿先例维持、W3 维持（Linux 冒烟归属）；契约面与生产代码零改动（仅测试新增），G-01~G-07/F-01~F-06 既有判定不受影响（测试文件不在扫描生产路径内）。

> **复核行（2026-11-16，WARN 复核员）**：三项逐一核验——W1 闭环成立（warnfix_test.go 实测 15 个测试函数与报告一致；cronback_test.go:121/:146-147 manifestErr 注入在位；本轮复跑 `go test -race -count=1 -cover ./internal/service/cronback/` → **93.8%** 实证超 80%）；W2 store 级闭环成立（stores/__tests__/cron.test.ts 实测 12 用例；全量 vitest 本轮实测 **15 文件/167 用例全过**——修复轮自验表「13 文件 167/167」文件计数为笔误，用例总数一致）；W3 维持理由充分（crontab/journalctl/rsync 二进制 Windows 不可达，不硬造桩）。W2 挂载级与 W3 环境绑定未闭环 → **终局判定：warn**。汇总见 05-reports/gate2-warn-fix-review-2026-11-16.md。
