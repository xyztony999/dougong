# Gate 2 验证报告 — 文件管理器（file-manager）代码→交付门控

| 属性 | 值 |
|------|-----|
| 模块 | file-manager |
| 轮次 | 独立重验轮（2026-11-14；覆盖同名报告，前序轮次见文末注记） |
| 扫描路径 | backend/internal/{api/fileman,service/fileman,store/fileman,adapter/fslocal,errcode/filemanager.go,api/middleware} + backend/internal/store/migrations/003_file_manager.sql + frontend/src/{api/file.ts,types/file.ts,types/enums.ts,stores/{file,trash}.ts,views/file-manager/,router/index.ts 追加段} |
| 契约基准 | profiles/dougong-panel/workspace/02-detailed/api/api-file-manager.md v1.0（22 接口 / FILE_001~015 / §3.3 权限矩阵 / §5.3 DDL） |
| 规则链 | 引擎 GR（.claude/rules/engine-rules.md 缺席——GR 语义按 validation SKILL.md L293 分层说明执行：扫描必执行/编译必过/E2E 必补/契约对齐，见 INFO-1）⊕ backend-rules §7 G-01~G-07 ⊕ frontend-rules §6 F-01~F-06 ⊕ business-concepts R-01~R-18 适用项 |
| 检查时间 | 2026-11-14 |
| 编译门控（脚本已执行，本报告引用未重跑） | go build=0 / go vet=0 / go test=0 / web（vue-tsc+vite build）=0 |
| 本轮实跑补充 | go test -count=1 -race 模块两包 / go test -cover 模块三包 / vitest file+trash 两套 / 全部 grep 扫描（各节附命令与输出） |

## 判定：WARN

- 🔴 CRITICAL 违例：**0**；契约断裂：**0**（前轮 E1 修复双侧在位并有回归用例锁定，本轮实跑全绿）。
- 栈规则扫描 G-01~G-06 / F-01~F-05 实跑全过。
- 业务规则适用项 R-02/03/04/05/08/09/14/17 全过（§4）。
- SEC-GATE：探针通过（`git --version` → `git version 2.56.1.windows.1`，rc=0 未被拦）且模块零桩 → DEBT-SEC-GATE 不适用（§5）。
- WARN 3 项（W1 覆盖率余量 / W2 组件挂载测试缺席 / W3 src/components 缺席）+ INFO 2 项（§7）——按本次判定规则「仅 ⚠️ 建议项=WARN」。

---

## 1. 后端规则扫描（backend-rules §7，本轮实跑，cwd=backend/）

| # | 检查项 | 结果 | 证据（命令 + 输出/人工核对） |
|---|--------|------|------------------------------|
| G-01 | 无 shell 拼接 | PASS | `grep -rnE 'exec\.Command(Weak)?Context?\(.+"(-c\|/c)"' --include="*.go" .` → 0 命中（rc=1）；全量 `grep -rn "exec\.Command" --include="*.go" .` → 唯一命中 `internal/adapter/nginxx/parser.go:3` 为**注释文本**（site-mgmt 模块注释，非调用）——全仓零真实外部命令调用，R-08 by-absence 双证 |
| G-02 | 无 SQL 字符串拼接 | PASS | `grep -rnE 'Sprintf\(.+(SELECT\|INSERT\|UPDATE\|DELETE)' --include="*.go" .` → 0 命中（rc=1） |
| G-03 | 无硬编码凭据 | PASS | `grep -rnEi '(password\|secret\|token)\s*=\s*"[^"]{8,}"' --include="*.go" . \| grep -v _test` → 0 命中（rc=1） |
| G-04 | SQL 只在 store 层 | PASS | 非 store 层命中 4 行逐条人工核对**均为注释**：service/auth/user.go:114（"FK ON DELETE CASCADE 清理"注释）、service/fileman/trash.go:36/81（"INSERT 与审计同事务"注释）、service/fileman/scope.go:20（"UPDATE fs_path_scope 同事务"注释）；store/fileman 两仓储全部 `?` 参数化（trash_repo.go:29/48/55/76/91/104、scope_repo.go:20/30/47，本轮全文实读） |
| G-05 | 无 panic 滥用 | PASS | `grep -rn "panic(" --include="*.go" . \| grep -v _test` → 0 命中（rc=1） |
| G-06 | goroutine 有退出路径 | PASS | `grep -rn "go func" --include="*.go" . \| grep -v _test` → 5 处全部位于 cmd/dougong/main.go:213/228/243/281/287。人工核对 main.go:209-300：3 个后台任务（audit worker :213 / monitoring collector :228 / **本模块 chunk sweeper :243**）均 `wg.Add(1)`+`defer wg.Done()`+panic recover 隔离+以 ctx 取消退出；本模块 ChunkSweeper → transfer.go:265-271 `select { case <-ctx.Done(): …; case <-ticker.C: … }`；2 个 HTTP serve goroutine（:281/:287）经 serveErr 通道 + `errors.Is(err, http.ErrServerClosed)` 归宿——全部由 main 统一编排，无库/service 层裸起 |
| G-07 | 编译+vet+test 全绿 | PASS（引用） | 脚本已执行 build=0 / vet=0 / test=0（本报告引用未重跑）；本轮另实跑模块级 `go test -count=1 -race ./internal/service/fileman/ ./internal/api/fileman/` → `ok …/service/fileman 1.805s` + `ok …/api/fileman 5.444s`（§6） |

## 2. 前端规则扫描（frontend-rules §6，本轮实跑，cwd=frontend/）

| # | 检查项 | 结果 | 证据（命令 + 输出） |
|---|--------|------|---------------------|
| F-01 | 无 Options API | PASS | `grep -rLn "script setup" --include="*.vue" src/` → 0 命中（rc=1；含本模块 3 页面 + 9 组件） |
| F-02 | 无裸 any | PASS | `grep -rnE "(:\s*any\|as any)" --include="*.ts" --include="*.vue" src/` → 0 命中（rc=1） |
| F-03 | 请求只走 api 层 | PASS | `grep -rnE "(axios\|fetch)\(" --include="*.vue" src/views src/components` → 0 命中（rc=2：src/components 目录不存在——W3 口径，扫描面如实限定 src/views） |
| F-04 | 无混入其他组件库 | PASS | `grep -rnE "from '(antd\|element\|ant-design-vue)'" src/` → 0 命中（rc=1） |
| F-05 | 无原生弹窗 | PASS | `grep -rnE "(^\|\W)(alert\|confirm)\(" --include="*.vue" src/` → 0 命中（rc=1） |
| F-06 | typecheck+build 全绿 | PASS（引用） | 脚本已执行 web=0（vue-tsc --noEmit + vite build），本报告引用未重跑 |

## 3. 前后端接口对齐（以 api-file-manager.md v1.0 为基准）

### AL-001 Controller 路径/方法/权限矩阵 vs API 文档：PASS

- **机械 diff 实跑**：`grep -oE '\.(GET\|POST)\("[^"]+"' backend/internal/api/fileman/routes.go`（22 行）与 `grep -oE "'/files/[a-z-]+/[a-z-]+'" frontend/src/api/file.ts`（22 行）提取集合，`diff --strip-trailing-cr` → **DIFF-CLEAN，22/22 路径完全一致**（后端组前缀 `/files` 由 routes.go:29 `Prefix()` 提供）。
- 权限矩阵 vs §3.3 逐组核对（routes.go:34-69 实读）：读组两角色 2（entry/list、trash/list，:38-41）；读组 admin 专属 4（path-scope/get、entry/download、entry/content、system-accounts/list，:44-49）；写组 admin+CsrfGuard 16（:52-69）。GET 豁免 CSRF、POST 全部 CsrfGuard，与 §3.1 链序一致。
- DDL 实读：003_file_manager.sql 与 api §5.3 逐字一致（trash_entry 10 列 + idx_trash_status + fs_path_scope 单行表 CHECK(id=1)）。
- PathScopeGuard 为 service 层强制前置而非 gin middleware——guard 须回传真实路径（guard.go:24-25 A-F2 裁定注释），语义不因载体改变，非违例。

### AL-002 前端 Action 路径 vs Controller 路径：PASS

api/file.ts 22 个函数（本轮全文实读）与 controller 一一对应；特殊通道对齐：upload/chunk 走 multipart `postForm`（file.ts:81/103），download 走流式 `downloadFile`（file.ts:118，D-16 校验先行开流语义）。

### AL-003 DTO 字段 vs 前端类型字段：PASS

dto.go（197 行全文实读）与 types/file.ts（289 行全文实读）逐形状核对一致：EntryItem/FileEntry 8 键（name/path/entryType/size/mode/ownerUser/ownerGroup/mtime）、ListData 4 键（path/items/total/generatedAt）、UploadData{results[{name,path,status}],uploadedCount}、ChunkInit/Progress/CompleteData、ContentData/SaveResultData、CreateData{path,entryType}、RenameData、MoveData{moved[{from,to}],skippedCount}、DeleteData{trashId,-trashName}（int64↔number）、ChmodData/ChownData、SystemAccountsData{users[{name,uid}],groups[{name,gid}]}、ArchiveData{path,size,format}、UnarchiveData{extractedCount,targetDir}、TrashItem 6 键、TrashListData{items,total,warning?}（W1 修复在位：types/file.ts:174 `warning?: string`）、RestoreData{id,originalPath,status:'restored'}、PurgeData{purgedCount}。请求体 16 个形状同查一致（{rootPath,excludedDirs}/{path,content}/{dir,name,type}/{path,newName}/{sourcePath,targetDir,onConflict}/{path,mode}/{path,ownerUser?,ownerGroup?}/{paths[],format,targetName?}/{archivePath,targetDir,onConflict}/{trashId,onConflict?}/{dir,fileName,totalSize,totalChunks}/multipart×2/{uploadId}）。常量同值：DIRECT_UPLOAD_LIMIT=UPLOAD_CHUNK_SIZE=5MiB（consts.go:11-13 `5<<20` vs types/file.ts:264-265 `5*1024*1024`）、EDIT_SIZE_LIMIT 1MiB、EDIT_BINARY_PROBE 8KB、TRASH TTL 1h。冲突/越界载荷收窄诚实（ConflictEntry{name?,path?}+conflictLabel+isConflictList、scopeReason/fsReason，types/file.ts:190-224）不虚构必填键。

### AL-004 枚举值前后端一致：PASS

file_type file\|directory\|symlink（consts.go:41-45 ↔ enums.ts:134）；archive_format tar\|tar.gz\|zip（archive.go:23-25 ↔ enums.ts:142）；trash entry_type 两值 file\|directory（store/fileman/models.go:27-28 ↔ types/file.ts:253 TrashEntryType，api §4.1.1 CHECK 同）；create type=file\|directory（types/file.ts:261）；onConflict ask\|overwrite\|skip、还原收窄 ask\|overwrite（types/file.ts:244 与 file.ts:243 `Exclude<ConflictStrategy,'skip'>` ↔ trash.go:194-198）；sort/order name\|size\|mtime / asc\|desc（consts.go:26-30 ↔ types/file.ts:229-230）——与 api §3.4 三方同值。

### AL-005 错误码前端可识别：PASS

FILE_001~015 双侧 15/15（errcode/filemanager.go:8-36 本轮全文实读 ↔ types/file.ts:272-288 FILE_ERROR_TEXT 15 键）；符号通道经包络上线（response.go:16-24 `Code any` 成功=0/失败=symbol 串），与 api §3.1 `{"code":0,"message":"ok","data":...}` 一致；module_test.go 内 `wantCode` 断言 31 处（`grep -c` 实数）；复用码族 AUTH_001/002/003/008 属 account-auth 全局通道（api §3.4 声明），本轮实读 module_test.go:205-225 见 readonly 越界 AUTH_008 与缺 CSRF 用例在位。

## 4. 业务规则适用项（business-concepts R-01~R-18）

| 规则 | 结果 | 证据（本轮实读） |
|------|------|------|
| R-02 Cookie 会话+CSRF | PASS | routes.go:35-69：全接口 AuthRequired（InitGuard→AuthRequired→写组 CsrfGuard→RequireRole）；缺 CSRF→AUTH_003、readonly 越界→AUTH_008 用例在位（module_test.go:222-225 区域） |
| R-03 变更类操作审计 | PASS | 每写动作恰一条 auditAction（grep 实数：entry 8/meta 4/edit 2/archive 4/transfer 4/trash 6/scope 1 处调用点）；下载 category=security（transfer.go:398/417 ActionFileDownloaded+CategorySecurity，D-08）；失败重试一次+运行日志兜底（service.go:66-75）；同事务例外按 D-02/D-12/D-15 落地（scope.go:46-56、trash.go:98-108、trash.go:297-311）；越界拒绝零审计 isScopeReject（service.go:107-111，D-01/A-F10） |
| R-04 路径白名单+EvalSymlinks 防逃逸 | PASS | guard.go:52-68 Resolve：词汇层域校验先行（零探测面）→ EvalSymlinks → 二次核域拦 symlink 逃逸；ResolveParent/ResolveForWrite/TrashDir/CheckStatic 全套（guard.go:73-165）；ValidateScopeUpdate 排除项含根拒绝（guard.go:135-154）；越界 FILE_002+data.reason 两类（guard.go:168-179） |
| R-05 上传临时文件+rename | PASS | WriteFileAtomic/WriteFileAtomicStreaming 同目录临时文件+rename、失败清理（fslocal/gateway.go:117-128）；edit.go:89 保存走 WriteFileAtomic（I-09） |
| R-08 禁 shell 拼接 | PASS | G-01 全仓零 exec.Command 调用（唯一命中为注释，§1） |
| R-09 零外呼 | PASS | 本模块出站仅 AuditService.Record（进程内 OHS，service.go:19-21）；fslocal 纯标准库无网络调用（G-01b 同证） |
| R-14 服务端校验不信任前端 | PASS | ValidateName（guard.go:204-210，FILE_007）、validModeString 0~0777 八进制（meta.go:111-128，FILE_015）、sort/order 取值域（projector.go:28-37）、onConflict 取值域（trash.go:194-198）、chunk index∈[0,totalChunks)/重复拒绝/累计大小校验（transfer.go:210/235）、ValidateScopeUpdate（guard.go:135-154） |
| R-17 删除入回收站 | PASS | delete→generateTrashName `{unix秒}-{16hex}`（trash.go:21-27，D-15）→ rename 至 .dougong-trash + INSERT + 审计**同事务**（trash.go:76-108），失败补偿回原位（trash.go:109-119，A-F13）；还原 I-02 终态防护（trash_repo.go:74-86 条件带 in_trash + trash.go:241-244）；清空逐条同事务+空站幂等（trash.go:284-311）；孤儿对账剔除+warning（trash.go:148-183）——前端消费链三件齐（stores/trash.ts:16-28 + trash/index.vue:215-217 n-alert） |

（R-01/06/07/10/11/12/13/15/16/18 归属其他模块，本模块不适用。）

## 5. SEC-GATE 债务口径核验（2026-11-14 裁定）

- 门禁探针实跑：`git --version` → `git version 2.56.1.windows.1`（rc=0）——**未被拦，Mimosa 已禁用**。
- 模块桩扫描实跑：`grep -rniE "stub|合规桩|placeholder.*(exec|command)|not.?implemented" internal/{service,api,store}/fileman internal/adapter/fslocal --include="*.go" | grep -v _test` → 0 命中（rc=1）——模块无外部命令合规桩（fslocal 纯标准库实现）。
- 结论：探针通过且模块无桩 → **DEBT-SEC-GATE 不适用**，亦不触发「探针通过仍留桩」的从严条款。

## 6. 测试与覆盖率（GR-03 口径，本轮实跑）

| 命令（实跑） | 输出 |
|---|---|
| `cd backend && go test -count=1 -race ./internal/service/fileman/ ./internal/api/fileman/` | `ok github.com/dougong/dougong/internal/service/fileman 1.805s` / `ok github.com/dougong/dougong/internal/api/fileman 5.444s` |
| `cd backend && go test -count=1 -cover ./internal/service/fileman/ ./internal/api/fileman/ ./internal/store/fileman/` | coverage: **73.4% / 45.9% / 66.4%** |
| `cd frontend && npx vitest run src/stores/__tests__/file.test.ts src/stores/__tests__/trash.test.ts` | `Test Files 2 passed (2)` / `Tests 11 passed (11)` |

- **前轮 FAIL 主项 E1（初始浏览契约断裂）修复双侧在位并有回归锁定**：后端 projector.go:38-40（A-F20：`strings.TrimSpace(path)=="" → path = s.guard.Snapshot().RootPath`，显式 path 语义不变、不升版 api 契约）+ 前端 stores/file.ts:27-37/72-87（root-default 响应 `data.path` 捕获 rootPath + currentPath 规范化；写操作空串即 FILE_001 故 writeDir 具体寻址）。回归用例：fileman_test.go:258-291 `TestListDirDefaultPathIsRoot`（缺省=显式受管根投影全等）+ module_test.go:214-218（「首屏不带 path 两角色均可用」，readonly+admin 双角色 httptest wantCode 0）+ file.test.ts 7 用例（root-default 请求形状/首屏渲染/FILE_012 回根重拉防死循环）——随上述实跑全绿。
- E2E/集成：module_test.go 全栈 httptest（无会话拒绝/角色边界 AUTH_008/CSRF AUTH_003/文件生命周期/直传/分块全链/E1 双角色），service 层 22+ 用例（路径逃逸、zip slip 整体不落盘、同事务审计、D-02 即时生效等），随脚本 test=0 与本轮 -race 复跑全绿。

## 7. WARN / INFO（非阻断）

- **W1（覆盖率余量）**：模块实测 service 73.4%（目标 80%+，差 6.6pp）、api 45.9%（目标 60%+，差 14.1pp）、store 66.4%——非 🔴 违例（SKILL Gate 2 §3 覆盖率为清单项非栈规则 CRITICAL），沿用 account-auth W1「覆盖率余量」同判；建议补 handler 分支与 EXDEV/降级路径用例。
- **W2（组件挂载测试缺席）**：views/file-manager（3 页面 + 9 组件）0 vitest 挂载测试（现有 11 用例均为 store 层）；沿用 monitoring/service-mgmt/panel-core W 同判，组件级回归以类型收窄 + store 单测拦截。
- **W3（src/components 缺席）**：frontend 无 src/components 目录，F-03 扫描面如实限定 src/views（先例口径）。
- **INFO-1**：.claude/rules/engine-rules.md 缺席——GR-01~06 语义按 validation SKILL.md L293 分层说明执行（扫描必执行/编译必过/E2E 必补/契约对齐四要素本报告 §1/§6/§3 全部落实）。
- **INFO-2**：bindError 通道 AUTH_012 未列入 api-file-manager.md §3.4 复用码族清单（属 account-auth 全局参数校验码；grep api 文档 0 命中，建议文档补注）。

## 8. 总结

| 维度 | 结果 |
|------|------|
| 栈规则扫描（G-01~06 / F-01~05 实跑） | 全过，🔴 CRITICAL 0 |
| 编译门控（引用脚本） | build/vet/test/web 全 0；模块级 -race 与 vitest 本轮实跑亦全绿 |
| 契约对齐 AL-001~005 | 5/5 PASS（22 路径机械 diff 全等；DTO/枚举/错误码/包络逐项核对一致；E1 修复在位） |
| 业务规则 R-02/03/04/05/08/09/14/17 | 8/8 PASS |
| SEC-GATE | 探针通过 + 模块零桩 → DEBT-SEC-GATE 不适用 |
| **判定** | **WARN**（W1 覆盖率余量 / W2 组件挂载测试 / W3 src/components；INFO-1/2） |

- 通过：栈规则 11 项 + 编译门控 4 项（引用）+ 模块测试 3 组（实跑）+ 契约抽查 5/5 + 业务规则 8/8
- CRITICAL 违例：0；契约断裂：0
- WARN：3（W1~W3）；INFO：2

---

## 前序轮次注记（历史留痕）

- 第 1 轮（2026-11-14，同日早前）：**FAIL**——E1 初始浏览契约断裂（空 path 请求返回 FILE_001，双角色首屏不可用；实跑 overlay 验证 empty-path err=FILE_001）。
- 复检第 1 轮（2026-11-14）：**PASS**——E1 经后端 A-F20（projector.go 缺省=受管根，backend/tasks.md:337/387-395 登记）+ 前端 root-default 消费修复，双侧回归用例补齐实跑全绿；W1（trash warning）已修复。
- 本轮（独立重验，2026-11-14）：全量重跑栈规则/契约/业务规则/SEC-GATE/模块测试——上述结论全部独立复现，E1 修复保持锁定；新增覆盖率实测引出 W1（覆盖率余量，前轮未测）。报告以本文件为权威版本。
