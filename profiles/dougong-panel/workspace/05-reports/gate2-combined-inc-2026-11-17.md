# Gate 2 验证报告 — 代码→交付门控（三批合并增量：site-mgmt v1.2/v2.0 + env-mgmt 新模块 + panel-core/account-auth 菜单 IA）

**模块**：site-mgmt（增量：聚合/手动更新/DNS-01）· env-mgmt（新模块）· panel-core + account-auth（菜单 IA）
**检查时间**：2026-11-17（/pipeline 增量轮，session 轨）
**扫描路径**：backend/internal/{adapter,service,api,store,errcode} + cmd/dougong · frontend/src
**基线**：api-site-mgmt.md v2.0 · api-env-mgmt.md v1.0 · api-panel-core.md v2.2（D-P18）· PageSpec site-mgmt v1.2×2 / env-mgmt v1.0×2 / panel-core v1.2×2 / account-auth v1.3+v1.1

## 一、编译与测试门控（本轮实跑）

| 门控 | 命令 | 结果 |
|------|------|------|
| 前端类型 | vue-tsc --noEmit | exit 0 |
| 前端构建 | npm run build | exit 0（30.0s） |
| 前端测试 | npx vitest run | **15 文件 174 用例全过**（+env store 4：loadList 成败流/taskFilter/clearTaskDetail 幂等） |
| 后端构建 | go build ./... | exit 0 |
| 后端静态 | go vet ./... | exit 0 |
| 后端测试 | go test -race -count=1 ./... | 全 ok 0 FAIL（增量测试：sitemgmt dns_inc_test 3 用例〔聚合投影/凭证生命周期含 D-18 分流与 I-C01 断言/replace〕+ store envmgmt 4 用例〔生命周期/互斥/恢复/分页〕+ api envmgmt 3 用例〔两角色矩阵/ENV_001 退化/AUTH_008+ENV_002+ENV_005〕+ sitemgmt module_test 扩 CERT_009/007 双断言） |
| 交叉编译 | GOOS=linux go build ./... | exit 0 |
| 产物同步 | make web（frontend/dist → backend/web/dist，go:embed 源） | 完成；同步后 go build=0 |
| 格式 | gofmt -l backend/ | 0 文件 |

## 二、后端规则扫描（G-01~G-07，增量面全过）

| # | 检查 | 结果 | 证据 |
|---|------|------|------|
| G-01 | 禁 shell 拼接/exec 合规 | PASS | envpack 全 exec.CommandContext 参数数组+exec.LookPath 探测（gateway.go:54/151）；acme IssueDNS 经 lego provider 无自拼命令；存量 cronx T07 登记不变 |
| G-02 | SQL 参数化 | PASS | store/sitemgmt（certCols+dns_credential_id、dns_cred_repo 全参数化）、store/envmgmt（task_repo 九方法静态 SQL+args）零拼接 |
| G-03 | 硬编码凭据 | PASS | 凭据走 DnsCredentialVault（dns-cred.key 0600，AES-256-GCM）；测试 fixture 值带 test-only 标记 |
| G-04 | SQL 层外 | PASS | grep 命中均注释（envmgmt/service.go:212 等） |
| G-05 | panic | PASS | 增量代码 0 panic |
| G-06 | goroutine 退出路径 | PASS | 增量唯一新 goroutine=dispatchWorker→runTask（worker.go:44，有限时：单命令 OpTimeout=10min、链路上界 3×，无长驻循环；注释在案）；main.go 既有编排不变 |
| G-07 | 编译门控 | PASS | 见第一节 |

## 三、前端规则扫描（F-01~F-05，增量面全过）

| # | 检查 | 结果 |
|---|------|------|
| F-01/F-02 | any/原生 alert·confirm | PASS（增量面 grep 0 命中） |
| F-03 | views 层 fetch/axios | PASS（http.ts 统一通道） |
| F-04 | 定时器配对清理 | PASS（env store 轮询 stopPoll/clearTaskDetail + 页面 onUnmounted 调用） |
| F-05 | Options API | PASS（增量全部 script setup） |

## 四、前后端契约对齐（AL-001~005）

| # | 检查 | 结果 |
|---|------|------|
| AL-001 | Controller 路径 vs api 文档 | ✅ site-mgmt 23 路径逐字一致（/sites 前缀下 grep 双侧 diff=0，含 dns-credential/{list,create,delete}、cert/{renew,replace}）；env-mgmt 10 路径一致（env/list·install·uninstall + task/list·detail + service/{start,stop,restart,enable,disable}） |
| AL-002 | 前端 Action vs Controller | ✅ api/site.ts 23 URL 全等；api/env.ts 10 Action（service×5 经 envServiceOp(op) 模板路径）全等 |
| AL-003 | DTO vs 前端类型 | ✅ SiteItem 聚合条目（后端 Blocks []SiteBlock json:"blocks" ↔ 前端 blocks: SiteBlock[]；块级 siteId/listen/enabled/certRef/certId/certStatus 六字段同名同型；顶层 siteId 双侧移除一致）；CertRequestRequest（siteId?/domains?/challenge/dnsCredentialId?）双侧一致；DnsCredentialItem（id/name/provider/createdAt/referencedCertCount）一致；children:null 契约——后端 MenuNode.Children nil→JSON null，前端 PanelMenuItem.children 类型与 toMenuOptions null 判空双防护在位（v0.1.1 教训闭环） |
| AL-004 | 枚举前后端一致 | ✅ acme_challenge +dns_01（迁移 009 CHECK/Go 常量/TS enums/枚举池四方）；dns_provider 三值四方；env 四枚举（env_type/env_task_action/env_task_status/package_manager_family）迁移 008 CHECK/Go/TS/池四方同值 |
| AL-005 | 错误码前端可识别 | ✅ CERT_008/009/010（errcode + CERT_ERROR_TEXT + 弹窗分流：CERT_008 内联/CERT_009 引导配置/CERT_010 reason）；ENV_001~006（errcode + handleEnvError 分流含 ENV_003 taskId 引导）；AUTH 族复用拦截器 |

## 五、业务规则抽查（R 系列）

| # | 规则 | 结果 |
|---|------|------|
| R-02/R-03 | 会话/CSRF/审计 | ✅ env 写接口全 admin+CsrfGuard；任务创建/终态与审计同事务（TxStore）；凭证 create/delete 审计 detail 零凭据材料（I-D01）——测试断言密文形态 |
| R-08 | 参数数组 | ✅ envpack/apt·dnf·yum 全字面量参数数组（DEBIAN_FRONTEND 经 Env 不进 argv）；DNS TXT 经 lego provider |
| R-09 | 外呼纪律 | ✅ DNS 服务商 API 经 lego provider 封装（用户主动申请/续期调度，与 ACME CA 同判）；DNS TXT 清理失败不阻断签发 |
| R-14 | 服务端校验 | ✅ envType/challenge/dnsCredentialId/provider 凭据形态/domains 泛域名形态/PEM 解析（x509）全服务端 |
| ADR-018 | 只读边界 | ✅ env 读 3 两角色/写 7 admin；dns-credential 全家 admin（敏感配置最小暴露）；站点聚合呈现两角色同数据；readonly 负空间测试断言（AUTH_008） |

## 六、设计决策落地核验（增量关键面）

| 决策 | 落地证据 |
|------|---------|
| D-17 聚合投影 | groupEntries/aggregateGroup（同源文件+server_name 全集；listen 合并去重/enabled 任一/证书组内最优 ssl 优先/blocks 子清单）；detail 单块寻址不变——测试 TestListSitesAggregate 断言「同文件 2 块→1 条目+跨文件不聚合」 |
| D-18 手动更新 | cert/renew failStatus 分流（valid 失败保持 valid+last_error——测试断言）；cert/replace 备份 5 份+原子写回+统一 reload+响应元数据（测试断言 origin 文件内容与 CERT_008） |
| D-19/D-20 | HTTP-01 三级落点（prepareHTTP01 既有+A19 文案与 PageSpec 弹窗文案逐字）；IssueDNS 三 provider（cloudflare/dnspod→tencentcloud API 密钥通道 A-ENV1 裁定/aliyun）；证书行 dns_credential_id 落库+续期凭证复用+缺失失败落库（测试断言） |
| ⚠️A3/A4/A6 | env 任务互斥（InFlight 事务内判定+ENV_003 测试）/升级=重装（action 实况判定）/卸载保守（前端两段警示+输入锁定、服务端 ENV_004） |
| I-04 重启恢复 | FailRunningAll running→failed「面板重启中断」——测试断言（pending 不误恢复） |
| D-P18 | main.go account-auth 分组四子项两角色+env 菜单；MenuItem.children 渲染+分组 path 空；顶栏动态标题（meta.title→菜单树→兜底）；children:null 双防护 |

## 七、WARN 项

| # | 项 | 说明 |
|---|----|------|
| W1 | DNS provider 实联归 Linux 冒烟 | IssueDNS 三 provider 真实外呼与 TXT 传播在 Windows 开发机不可验证（fake 网关覆盖编排分支、lego provider 构造已编译验证）；envpack apt/dnf/yum 与 systemd 实机操作同归 Linux 冒烟——沿 ptyx/sysd/firewallx 环境绑定先例如实声明 |
| W2 | acme gateway 共用尾部重构 | Issue/IssueDNS 提取 ctxClient/ensureRegistered/obtain 共用（行为等价重构，既有 28 用例回归全过）；lego DNS 子包引入 go.mod 新传递依赖（cloudflare-go/alibaba-sdk/tencentcloud-sdk，均经 tidy 锁定） |
| W3 | PageSpec v1.2 版本口径 | site-mgmt PageSpec 版本行 v1.2（v1.1 D4 增量+DNS-01 增量合并为 v1.2 一个版本行 + changelog 两行）——INDEX note 与文档 changelog 一致 |
| W4 | 前端挂载级测试 | env-mgmt 页面/DNS 凭证弹窗无组件挂载用例（沿 panel-core W4/全仓 devDependencies 先例：@vue/test-utils 不在 devDeps）；store 级 4 用例在位 |
| I1 | 仓库行宽提示 | npm build chunk 4.1MB（echarts 主 chunk，既有形态非增量引入）；vite chunkSizeWarning 既有告警不阻断 |
| I2 | S7 PageSpec Action 与前端渲染差异 | certRetryRenewal 前端不再渲染独立按钮（保留 Action 兼容，PageSpec v1.2 已声明）——非缺陷，契约口径已三方登记 |

## 总结

**判定**：**WARN**（CRITICAL 0 / 契约断裂 0；4 WARN 均为环境绑定/测试深度/既有形态类，2 INFO）

- 编译门控八项全绿（双侧构建+测试+交叉编译+产物同步+格式）。
- AL-001~005 契约对齐零差（33 新增/变更接口三方一致；聚合 DTO 与 children:null 契约双侧闭环）。
- 增量测试 13 个新用例/断言扩展全过（-race）。
- 待用户确认后收尾（INDEX/交付报告/发布建议）。
