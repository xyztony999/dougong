# Gate 1 验证报告 — 设计→代码门控（env-mgmt W1 实机冒烟缺陷修复轮 · 增量 lite）

**模块**：env-mgmt（增量，上轮 v1.0 全链 done / Gate 1 WARN 已确认）
**检查时间**：2026-11-17（/pipeline 增量轮，session 轨）
**基线版本**：api-env-mgmt.md v1.0→v1.1 · prd-env-mgmt.md v1.0→v1.1 · PageSpec admin/readonly v1.0→v1.1 · 枚举池 04-business-enum.md env_type 行（六值→七值）
**输入来源**：v0.1.2 用户服务器（cheerful-bliss-1）实机冒烟反馈 3 项（缺陷 1 enable nginx DBus InvalidArgs；缺陷 2 四卡片误显「探测不可用」；需求 3 补 MongoDB）

## 一、增量范围与根因登记

| # | 反馈项 | 根因（代码级定位，非臆测） | 设计落点 |
|---|--------|--------------------------|---------|
| 1 | enable nginx 报 `org.freedesktop.DBus.Error.InvalidArgs` | service/envmgmt Catalog UnitByFamily 全部无 .service 后缀；systemd EnableUnitFiles/DisableUnitFiles 的 DBus 契约对单元名做 unit_name_to_type 校验且**无后缀补全**（systemd dbus-manager.c），无后缀名直接 InvalidArgs；StartUnit/StopUnit/RestartUnit/LoadUnit 有服务端 .service 补全故 start 正常、仅 enable/disable 报错——与实测现象逐字吻合；sysd 网关编组（as+bb）本身正确 | ⚠️A8 + D-13：目录单元名一律规范全名（七环境同步），网关零改动 |
| 2 | Docker/MySQL/Redis/PHP-FPM 卡片全部「探测不可用」+「服务态未探测」 | envpack.QueryInstalled dpkg 分支对完全不在 dpkg 数据库的包（dpkg-query 输出 "no packages found matching xxx"、退出码 1）未被「未安装」分支覆盖（只匹配 "not installed"/「未安装软件包」）→ 逐环境 query error → service 层 installed 保持 null → UI 落「探测不可用」（通道级语义被逐环境复用）；次要：包名单候选（docker-ce 自装形态查 docker.io 落空）+ dpkg 输出受 locale 影响。nginx 已装命中故正常——与截图证据吻合 | ⚠️A9 + D-14 + D-02 修订：dpkg 改 -f='${Status} ${Version}' 状态字段判定 + LC_ALL=C 注入 + no-packages-found=未装；多候选包名数组；三态如实（通道级/条目级/未装） |
| 3 | 缺 MongoDB | 上轮 ⚠️A1 六环境目录；且 D-06「官方仓库代配」设计声明了 needsVendorRepo+env_repo_configured 审计，实现为空转日志（worker 仅 ring.append 提示行，无文件写入无审计）——MongoDB（mongodb-org 三系均不在发行版官方源）入目必须补真代配 | ⚠️A1（六→七）+ ⚠️A10 + D-06 补真：os-release 判定 + apt keyring/下载公钥 + yum repo 文件 + env_repo_configured 审计落地 + 任务链次序前移（代配→刷新索引→安装） |

## 二、检查结果（增量 lite 范围）

### 1. PRD 就绪（增量复核）

| 项 | 结果 |
|----|------|
| v1.1 变更与根因一致性 | ✅ FR-01/02/03/04 与 ⚠️A8/A9/A10 对应；N2 +2 异常分支（三态区分/支持矩阵外组合）；辨析表 +1（探测不可用两义）；N1 补可注入测试纪律 |
| 编号接力 | ✅ ⚠️A8/A9/A10（A7 后）；更新记录 v1.1 行在位 |

### 2. API 文档完整性（增量章节）

| 章节 | 结果 |
|------|------|
| 1.3 决策表 | ✅ D-01/D-02/D-05/D-06/D-07 rule_changed + D-13/D-14 rule_added（编号 D-12 后接力）；决策含根因与先例引用 |
| 3.4 公共规范 | ✅ 枚举表 env_type 七值；ENV_002 取值域七值；业务常量 +4（MONGODB_ORG_TRACK=8.0 ⚠️/VENDOR_*×3/QUERY_TIMEOUT=30s） |
| 4.1.1/4.1.4/4.1.5 | ✅ 字段 CHECK 注记七值；env/list 三态响应语义；install/uninstall 多候选编排与代配次序；EnvPackGateway 契约 EnsureVendorRepo 签名落定（summary 进输出尾部） |
| 5.3 DDL + 迁移 | ✅ CHECK 七值；010_env_mgmt_mongodb.sql 表重建方案（SQLite CHECK 不可 ALTER；009 已被 site-mgmt DNS-01 占用；索引重建/序列保持声明） |
| 6.1 changelog | ✅ v1.1 行（enum_value_added + rule_changed×4 + rule_added×2 + table_changed，目标节号齐全，词汇表合规） |

### 3. 前后端契约一致性（S7/S8 机械实跑）

| 检查 | 结果 |
|------|------|
| S7 接口↔Action | ✅ 零变化（本轮无接口增删；10 接口 ↔ admin 10 Action/readonly 3+7 全集不变） |
| S8 枚举同值 | ✅ env_type 七值四方逐字一致：池（04-business-enum.md:56）↔ api（3.4 表 + DDL CHECK 5.3 + ENV_002）↔ PageSpec admin 第 3 节 ↔（B/F 待落地：前端 enums.ts + DDL 迁移 010，Gate 2 复验） |
| M1~M5 机械 | ✅ 路径三段式零变化；changelog v1.1 ×4 文档在位；最后更新日期 2026-11-17；占位符 0；变更类型词汇表合规 |
| 内部引用 | ✅ ⚠️A8/A9/A10（PRD×11 处）↔ D-13/D-14（api×8 处）交叉引用闭合；I-08 七条目同步 |

## 三、判定

**PASS（机械项）**：S7 零变化、S8 四方同值、M1~M5 全过、编号接力无冲突、DDL↔字段清单一致。

**WARN（呈请确认，不阻断；对应主会话已转达的拍板项 A-D）**：

| # | 事项 | 建议 | 风险 |
|---|------|------|------|
| W1 | MongoDB 版本轨道 8.0（MONGODB_ORG_TRACK 假设行 ⚠️） | 8.0（现行稳定主轨；Debian12/Ubuntu22.04+/RHEL8/9 支持） | 若用户期望 7.0：改 1 常量 + 路径模板，影响极小 |
| W2 | 代配需面板进程出网（repo.mongodb.org 公钥 .asc 下载 + install 时仓库访问） | 接受；无出网环境任务 failed 如实呈现 | 无出网服务器上 MongoDB 安装不可用（如实） |
| W3 | MySQL 官方源不代配（Debian12 无 mysql-server 属已知限制） | 多候选检测已覆盖 mysql-community-server 自装形态；安装失败如实呈现 | Debian12 用户装 MySQL 失败（输出尾部有明确原因） |
| W4 | apt 系 Docker 安装首选发行版原生 docker.io（不代配官方源） | 检测多候选覆盖 docker-ce；安装 docker.io（发行版维护） | 用户偏好 docker-ce 官方源时用 apt 系装到的是 docker.io |
| W5 | D-06 v1.0 声明与实现偏差（空转日志无审计）在本轮修复中补真 | 按 v1.1 设计实现 + env_repo_configured 审计 | 无（回归设计） |

**结论**：CRITICAL 0 / 契约断裂 0 → **WARN（W1~W5，均为参数级建议项）**。用户经主会话确认后进入 B/F。
