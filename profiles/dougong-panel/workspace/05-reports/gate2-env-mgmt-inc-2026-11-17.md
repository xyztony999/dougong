# Gate 2 验证报告 — 代码→交付门控（env-mgmt W1 实机冒烟缺陷修复轮 · 增量 lite）

**模块**：env-mgmt（增量；上轮 v1.0 交付 Gate 2 WARN 已确认）
**检查时间**：2026-11-17（/pipeline 增量轮，session 轨——B/F 按 go-vue3 编排 agent 规约降级会话执行）
**输入**：api-env-mgmt.md v1.1 / PageSpec admin·readonly v1.1（Gate 1-lite WARN W1~W5 用户经主会话确认 2026-11-17：MongoDB 8.0 轨道 / 代配补真接受出网 / MySQL 不代配 / docker.io 首选 / 迁移 010 表重建）
**范围**：三项实机缺陷修复（bug1 enable InvalidArgs / bug2 四卡片误显探测不可用 / 需求 3 MongoDB 七环境）+ D-06 代配补真 + 编译门控全量

## 一、根因修复明细（先读码定位，非臆测）

| # | 缺陷 | 根因 | 修复 |
|---|------|------|------|
| 1 | enable nginx 报 `org.freedesktop.DBus.Error.InvalidArgs`（systemd 可达但参数被拒） | 目录表 UnitByFamily 无 .service 后缀；systemd EnableUnitFiles/DisableUnitFiles 经 unit_name_to_type 校验、**无后缀补全**（dbus-manager.c），无后缀名直接 InvalidArgs；StartUnit/LoadUnit 有服务端补全故 start/探测正常、仅 enable/disable 报错——与实测逐字吻合。sysd 网关编组（as+bb）本身正确 | Catalog 七条目 UNIT 名规范全名（D-13/⚠️A8）；php-fpm apt 版本化单元（phpX.Y-fpm.service）经 ListUnits 正则解析（unitFor）；网关零改动 |
| 2 | Docker/MySQL/Redis/PHP-FPM 卡片「探测不可用」+「服务态未探测」 | dpkg-query 对不在数据库的包输出 "no packages found matching xxx"（退出 1）未被未装分支覆盖（仅匹配 not installed/未安装软件包）→ 逐环境 query error → installed=null 被 UI 映射「探测不可用」（通道级语义被逐环境复用）；次要：包名单候选（docker-ce 自装查 docker.io 落空）+ dpkg 输出本地化 | QueryInstalled 重写（D-14）：dpkg `-f='${Status} ${Version}'` 状态字段判定 + LC_ALL=C 注入 + no-packages-found=未装；rpm 退出码语义；多候选包名数组（installedPkg：任一命中=已装/全未装=未装/真错误=条目级 null）；前端 installedText 三态精确化（通道不可用=「未探测」灰、条目真错误=「探测不可用」橙、未装=「未安装」） |
| 3 | 缺 MongoDB；D-06「仓库代配」v1.0 声明未实现（worker 空转日志，无文件写入无审计） | 设计-实现偏差 | 七环境目录（+mongodb：mongodb-org/mongod.service/VendorRepo 三系 true）；EnsureVendorRepo 补真（os-release 判定→ubuntu·debian·rhel 系支持矩阵→apt 写 mongodb-org-8.0.list+HTTPS 下载公钥 .asc 校验 armored 头→dnf/yum 写 mongodb-org-8.0.repo·docker-ce.repo；幂等；矩阵外零写入任务 failed）；worker 次序前移 代配→刷新索引→装卸；env_repo_configured 审计落地；卸载移除实际命中候选 |

## 二、编译门控八项（实跑 2026-11-17）

| 门控 | 结果 |
|------|------|
| vue-tsc --noEmit（npm run build 内含） | ✅ 0 错误 |
| npm run build（vite） | ✅ exit 0（24.8s） |
| npx vitest run | ✅ 16 文件 174 用例全过（env store 载荷升七条目+全名 unitName） |
| go build ./... | ✅ 0 |
| go vet ./... | ✅ 0 |
| go test -race -count=1 ./... | ✅ 46 包 ok / 0 FAIL（新增 24 用例，见下） |
| GOOS=linux GOARCH=amd64 交叉编译 | ✅ 0 |
| 产物同步 backend/web/dist（make web）+ 同步后 go build | ✅ 0；gofmt 0 文件 |

## 三、规则扫描（增量面）

- T07（backend-rules）：G-01 exec 唯一站点 `exec.CommandContext` 参数数组（vendorrepo 为 net/http+os.WriteFile，R-08「文件写入非命令拼接」语义保持；探测 LookPath 注入既有）；G-02/G-03 SQL 拼接/硬编码凭据 0 命中；G-06 无新增 goroutine（dispatchWorker 既有有限时）；gofmt 0。
- F07（frontend-rules）：改动面 any/原生 alert·confirm/views 层 fetch·axios/Options API 0 命中；枚举文案走 ENV_TYPE 字典映射；文案变更与 PageSpec v1.1 逐字对齐。

## 四、前后端契约对齐（AL 机械实跑）

| 检查 | 结果 |
|------|------|
| AL-001/002 十接口三方（api ↔ PageSpec admin 10 Action/readonly 3+7 ↔ api/env.ts 6 函数=10 路径） | ✅ 零变化（本轮无接口增删） |
| AL-003 DTO 逐形（EnvItem 等响应结构） | ✅ 结构零变化；unitName 取值域升规范全名（类型不变，前端原样呈现） |
| AL-004 env_type 七值五方同值 | ✅ DDL CHECK（010）/api 3.4/enums.ts/池/PageSpec 逐字一致 |
| 迁移链 | ✅ 001~010 全量重放（store.Migrate 动态计数含 010；TestEnvTypeSevenValues 断言七值写入+非法拒绝+台账行保留） |

## 五、测试增量（24 用例，全注入·CI 环境无关——第 5 起耦合预防）

- envpack gateway_test（14 用例）：apt 六分支（installed/hold/config-files/not-installed/**no-packages-found 回归**/真错误）、rpm 三分支、通道不可用、argv 形状+LC_ALL=C 注入断言。
- envpack vendorrepo_test（7 用例）：ubuntu/debian 内容断言、支持矩阵外三形态零写入、yum 双 repo、幂等（公钥复用不重下）、公钥 armored 校验防错页、noop。
- service catalog_env_test（3 用例）：七条目全名机械断言（**缺陷 1 回归**）、EnvList 三态（docker 未装 false/**缺陷 2 回归**/mysql 第二候选命中/redis config-files 未装/mongodb 七环境）、php-fpm 探测真错误条目级 null。
- store TestEnvTypeSevenValues（1 用例）：迁移 010 回归。
- 注入面：envpack 新增 runCmd/readFile/writeFile/mkdirAll/httpGet 五注入点；DetectChannel 去 GOOS 短路改由探测承载（生产语义不变：非 Linux LookPath 三连落空即 ENV_001）；CI runner 的 dpkg 数据库/systemd/网络零参与。

## 六、WARN 项（呈请确认，不阻断）

| # | 事项 | 说明/建议 |
|---|------|----------|
| W1 | 实联冒烟归 Linux 目标机（环境绑定如实声明，沿上轮口径） | 建议在 cheerful-bliss-1 验证清单：①nginx enable/disable+start（全名修复）②四卡片三态呈现（未装环境显「未安装」灰）③dpkg-query 三态实机复核 ④MongoDB 安装（代配→索引→mongodb-org 装毕 mongod.service 实况）⑤任务输出尾部含 [vendor-repo] 行+审计 env_repo_configured 落账 |
| W2 | 工作区存在本轮之外未提交改动 | main.go 菜单图标、MainLayout.vue/403.vue/firewall/index.vue/site-mgmt/index.vue 小改——非本轮引入未触碰；发布打包前需确认归属并一并处置 |
| W3 | 支持矩阵边界如实失败（设计口径 W1 确认） | RHEL10/fedora、Ubuntu bionic、Debian trixie 等矩阵外组合=任务 failed 明示原因零写入；MySQL apt 系在 Debian12 无 mysql-server（官方源不代配拍板）失败如实呈现于输出尾部 |

## 七、判定

**CRITICAL 0 / 契约断裂 0 / 编译门控八项全过 → WARN（W1~W3，均为部署环境与工作区管理建议项，无代码缺陷项）。**
按流水线纪律：Gate 2 需用户确认方可终态 PASS——本报告经主会话转呈，确认后 INDEX gate2 记 pass 并可进入发布（建议 v0.1.3，见交付报告）。
