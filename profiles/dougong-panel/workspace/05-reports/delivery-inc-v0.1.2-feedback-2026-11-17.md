# 交付报告 — v0.1.2 用户反馈增量轮（7 项 + Gate 1 改判 DNS-01）

**日期**：2026-11-16 ~ 2026-11-17 · **执行模式**：增量 + 断点续跑（session 轨全程；上轮中断点已承接）· **流水线**：D0（承接）→ D1（承接+DNS-01 增量）→ D2-D4 → Gate 1（两轮确认）→ B → F → 编译门控 → Gate 2（已确认）→ 收尾
**范围**：A 批 site-mgmt 增量（反馈 1-3）+ env-mgmt 新模块（反馈 4）+ B 批 panel-core/account-auth 菜单 IA（反馈 5-7）+ DNS-01 改判（P2：DNS API 自动化）

---

## 一、需求覆盖（7+1 项全闭环）

| # | 反馈 | 方案（最终拍板） | 落点 |
|---|------|----------------|------|
| 1 | 站点列表 80/443 两行合并 | ⚠️A18 同源文件+同域名全集聚合为单条目（listen 合并「80 / 443 ssl」、行展开 blocks 块清单、组级启停逐块；**仅展示层，siteId 寻址与数据模型零变化**） | api D-17 / site/list 聚合投影 / 前端聚合渲染 |
| 2 | 证书缺「更新证书」 | D-18 双通道：acme=cert/renew 立即续期（valid/expiring_soon/renew_failed；失败分流——valid 保持 valid+last_error）；imported=cert/replace 粘贴 PEM 备份原子写回（保留 5 份）+统一 reload。自动续期调度确认既有（每日） | api v2.0 / operList「更新证书」「替换证书文件」 |
| 3 | 申请证书缺「文件验证」 | ⚠️A19：HTTP-01 更名「文件验证」+三级自动落点（root 直写→复用 location→临时注入兜底）；**Gate 1 改判追加 ⚠️A20 DNS-01（DNS API 自动化）**：凭证密文存储、自动 TXT、泛域名（domains 手输 `*.domain`）、自动续期复用凭证；手动 TXT OUT | api D-19/D-20 / 弹窗三挑战+凭证选择 / DNS 凭证管理弹窗 |
| 4 | 环境安装与管理（新模块） | ⚠️A1~A7：六环境（nginx/Apache/Docker/MySQL/Redis/PHP-FPM）+apt/dnf·yum 双系+异步任务（互斥/不可取消/重启恢复）+升级=重装+卸载保守 remove+安装不强制启动；四层边界（包/实例/容器/单元） | env-mgmt 全新四层 + 迁移 008 + 10 接口 + /env 页面 |
| 5 | 顶栏「账户与安全基座」术语泄漏 | 根因 MainLayout.vue:182 硬编码业务域显示名；修复=顶栏动态标题（路由 meta.title 优先→菜单树兜底）；附带修复列表页 loadError 无反馈（「正文空白」候选根因） | MainLayout headerTitle / 三列表页 NAlert+重试 |
| 6 | 设置里找不到用户管理 | 根因 main.go ModuleMenuRegistry 未登记 account-auth 条目（设计侧 PageSpec 本有——实现回归设计）；侧栏新增「用户与安全」分组四子项（两角色） | api-panel-core v2.2 D-P18 / main.go Register |
| 7 | 设置页卡片顺序跳跃 | 方案一四分组重排：常规（语言）/网络与访问（监听·入口·域名）/安全与证书（HTTPS·证书·会话）/关于（更新检查）；Card 语义编号不变+分组标题两级视觉 | settings/index.vue / PageSpec panel-core v1.2 |

## 二、代码交付清单

### 后端（backend/，不含 web/dist 产物）

**新增文件（12）**：
- `internal/store/migrations/008_env_mgmt.sql`（env_install_task 表+两索引）
- `internal/store/migrations/009_site_mgmt_dns01.sql`（certificate 表重建：challenge CHECK +dns_01、+dns_credential_id 列；dns_credential 表）
- `internal/adapter/envpack/gateway.go`（通道探测/版本查询/索引刷新/装卸——R-08 参数数组+OpTimeout 10min+Windows 退化）
- `internal/adapter/acme/dns.go`（IssueDNS：cloudflare/alidns/dnspod→tencentcloud API 密钥通道）
- `internal/store/sitemgmt/dns_cred_repo.go`（凭证 CRUD，List 零回显形态）
- `internal/store/envmgmt/{models.go,task_repo.go}`（任务台账九方法+TxStore）
- `internal/service/sitemgmt/{dns_vault.go,dnscred_svc.go,dns_inc_test.go}`（AES-256-GCM 保险库/renew·replace·凭证三接口/增量测试）
- `internal/service/envmgmt/{service.go,worker.go,dto.go}`（目录映射/探测/互斥/worker/服务操作）
- `internal/api/envmgmt/{module.go,helpers.go,module_test.go}`（10 接口+全栈测试）
- `internal/errcode/envmgmt.go`（ENV_001~006）
- `internal/store/envmgmt/task_repo_test.go`

**修改文件（核心 14）**：
- `internal/adapter/acme/gateway.go`（共用尾部提取：ctxClient/ensureRegistered/obtain）
- `internal/store/sitemgmt/{models.go,cert_repo.go,store.go}`（Certificate +DnsCredentialID/challenge 扩值/DnsCred()）
- `internal/service/sitemgmt/{projector.go,certsvc.go,dto.go,ports.go,service.go,consts.go,sitemgmt_test.go}`（聚合投影/request 扩展/renewCertWith 分流/IssueDNS 端口/dnsVault 注入）
- `internal/api/sitemgmt/{handler.go,routes.go,module_test.go}`（+5 接口）
- `internal/errcode/sitemgmt.go`（CERT_008~010）
- `cmd/dougong/main.go`（dnsVault 装配+account-auth 分组+env 菜单+env 模块注册）
- `go.mod/go.sum`（lego DNS 子包传递依赖）

### 前端（frontend/src/）

**新增（7）**：`types/env.ts`、`api/env.ts`、`stores/env.ts`、`stores/__tests__/env.test.ts`、`views/env-mgmt/index.vue`、`views/env-mgmt/components/EnvTaskDrawer.vue`、`views/site-mgmt/certificates/components/{CertReplaceModal,DnsCredentialsModal}.vue`

**修改（核心 14）**：`types/enums.ts`（acmeChallenge +dns_01、DNS_PROVIDER、env 四枚举）、`types/site.ts`（SiteListItem 聚合化+DNS/replace 类型+CERT_ERROR_TEXT 扩）、`api/site.ts`（23 Action）、`stores/site.ts`（findBySiteId 块级）、`views/site-mgmt/index.vue`（聚合行+展开+组级启停）、`views/site-mgmt/components/SiteDetailDrawer.vue`、`views/site-mgmt/certificates/index.vue`（operList 更新/替换+DNS 凭证入口）、`certificates/components/{CertRequestModal,CertDeployModal}.vue`、`views/cron-backup/components/BackupJobDrawer.vue`、`layouts/MainLayout.vue`（动态顶栏+env 图标）、`router/index.ts`（/env）、`views/panel-core/settings/index.vue`（四分组）、`views/account-auth/{user-list,session-list,audit-log-list}/index.vue`（loadError）、`vite.config.ts`（/env proxy）

### 设计文档（workspace/）

- 增量：BA/AA/FL-site-mgmt v1.2、function-list.md v2.5、prd-site-mgmt v1.2、api-site-mgmt **v2.0**、PageSpec site-mgmt v1.2×2、prd-panel-core v1.5、api-panel-core v2.2、PageSpec panel-core v1.2×2、PageSpec account-auth v1.3/v1.1、prd-account-auth v1.1、domain-model **v1.20**（新增聚合根 dns_credential）、枚举池 +5 行（dns_01 扩值+dns_provider+env×4）、doc-center.json 再生成×3
- 新建：prd-env-mgmt v1.0、api-env-mgmt v1.0、PageSpec env-mgmt v1.0×2、BA/AA/FL-env-mgmt v1.0（承接上轮）
- 报告：gate1-combined-inc-2026-11-16.md（含 P2 改判节）、gate2-combined-inc-2026-11-17.md、本报告

## 三、验证结果（全绿）

| 门控 | 结果 |
|------|------|
| vue-tsc --noEmit | exit 0 |
| npm run build | exit 0（30.0s） |
| vitest | **174/174**（15 文件；+env store 4） |
| go build / vet | 0 / 0 |
| go test -race -count=1 ./... | 全 ok 0 FAIL（增量 13 用例） |
| GOOS=linux go build | exit 0 |
| gofmt -l | 0 文件 |
| dist→backend/web/dist 同步 | 完成，同步后 go build=0 |
| Gate 1 | WARN→**用户确认通过**（P1~P4+W1~W8；P2 改判 DNS-01 增量复检全过） |
| Gate 2 | WARN→**用户确认通过**（W1~W4 接受） |

## 四、WARN 登记与实机冒烟清单（Linux 目标环境）

| # | 项 | 说明 |
|---|----|------|
| W1 | **DNS provider 实联** | Cloudflare/阿里云 DNS/腾讯云 DNSPod 三家真实外呼、TXT 记录添加/传播/清理（lego provider 构造已编译验证；fake 网关覆盖编排分支；Windows 开发机无法真实外呼） |
| W2 | **envpack 实机操作** | apt update/install/remove、dnf·yum makecache/install/remove、dpkg-query/rpm -q 版本查询真实执行（含第三方仓库代配场景：RHEL 系 docker-ce） |
| W3 | **systemd 实机** | env 服务操作六动作（start/stop/restart/enable/disable+GetUnit 状态）经真实 dbus（Windows 无 systemd） |
| W4 | **DNS-01 端到端** | 凭证创建→cert/request(dns_01)→TXT→签发→自动续期全链（含 TXT 清理失败残留场景的人工核查） |
| W5 | 依赖登记 | lego DNS 子包传递依赖（cloudflare-go/alibaba-cloud-sdk-go/tencentcloud-sdk-go）——已 go mod tidy 锁定，License 均为 Apache-2.0/MIT 兼容 |
| W6 | 测试深度 | env 页面/凭证弹窗无组件挂载用例（devDeps 无 @vue/test-utils，沿全仓先例；store 级 4 用例在位） |
| — | 既有延续 | 各模块历史 WARN（挂载级测试/环境绑定）不因本轮变化；npm 主 chunk 4.1MB 为 echarts 既有形态 |

## 五、发布建议

**建议发布 v0.1.3**（MINOR）：

1. **内容**：新模块 env-mgmt（环境管理）+ site-mgmt 证书手动更新/DNS-01 + 侧栏「用户与安全」分组 + 动态顶栏 + 设置页四分组 + 三个列表页加载失败态修复。
2. **API 变更说明（需在 Release Notes 明示）**：
   - 新增 13 接口（site-mgmt 5 + env-mgmt 10，其中 dns-credential 3 个）；
   - **BREAKING**：GET /sites/site/list 响应 items 结构变更（聚合条目：顶层 siteId 移除、寻址经 blocks[].siteId）——面板前端同版本发布，无外部消费者；
   - certificate 表迁移 009（自动执行，表重建保留数据）；env_install_task 新表（008）。
3. **升级兼容**：单二进制原地替换即可（SQLite 迁移自动）；首次启动自动创建 dns-cred.key/env 表；无配置文件新增键。
4. **验证建议**：发布前在 Linux 服务器跑一轮 §四 冒烟清单（重点 DNS-01 全链与 RHEL 系 docker-ce 仓库代配）。
5. git 操作（commit/push/tag/Release）由主会话统一执行，本报告不附带。

## 六、收尾状态

- INDEX.md：env-mgmt 新模块条目终态（六 stage 全 done，双 gate warn 已确认）；site-mgmt/panel-core/account-auth 四 stage note 与 gate2 note 均追加增量轮记录；C-10 花括号双引号纪律核查通过（27 处 note 内花括号均在双引号包裹内）。
- doc-center.json 已随 D2/DNS-01 增量再生成（12 业务域，site-mgmt 双聚合根）。
- 流水线状态：全链 done，无 todo/pending 残留（env-mgmt 段断言通过）。
