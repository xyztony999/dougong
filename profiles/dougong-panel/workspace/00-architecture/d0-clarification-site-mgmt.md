# D0 需求澄清报告：site-mgmt（网站管理，核心卖点）

> 引擎 design 阶段 D0 产物（2026-11-12，批量澄清）。作为 D1 架构设计的直接输入。
> 需求来源：idea.md §4.1 M4 + §3/§5.4（round-trip/安全写入）+ knowledge/business-concepts（site/certificate、R-06/R-07/R-15/R-16、A2）。

## 1. 业务概念解析

| 维度 | 内容 |
|------|------|
| 业务术语 | 存量导入、server 块、指令树、Raw 节点、round-trip、站点模板（静态/PHP-FPM/反代）、ACME（HTTP-01/TLS-ALPN）、自动续期、反代上游、`nginx -t`、conf.backup 快照 |
| 业务流程 | 存量导入：扫描 conf.d/vhost.d/sites-enabled → 词法扫描解析指令树 → 提取 server 块为站点对象 → 可视化编辑 → 重新序列化写回（Raw 保留原文）；安全写入：备份（conf.backup/时间戳/）→ 修改 → `nginx -t` → 失败自动恢复并报错 diff → reload；证书：ACME 申请（lego）→ 部署 → 自动续期 |
| 业务角色 | admin 全操作；readonly 查看站点列表/详情/证书状态/日志尾部（ADR-018） |
| 业务规则 | R-06 安全写入四步流程；R-07 未识别指令保留 Raw 原文写回；R-15 存量自动发现；R-16 编辑写回不得影响其余站点（byte-level/语义级无变化）；A2 已拍板 **证书 6 状态机** |
| 业务数据 | site（server_name/listen/root/ssl_cert/location 列表/proxy_pass 上游/enabled/raw 块）、certificate（域名列表/issuer/有效期/来源 acme·imported/状态/续期任务） |

## 2. 架构匹配与类型判定

- 匹配结果：site/certificate 已在 kb 登记；nginxx 解析器已在 ADR-010 确认为自研核心资产；workspace 无本模块设计文档 → **无匹配，新增**
- 需求类型：**新增** → D1 初始生成
- 模块定位：**项目核心卖点模块**（里程碑 M3，工作量最大）；nginxx round-trip 样本集 ≥20 为验收硬项

## 3. 澄清问答记录

| # | 问题 | 结论 | 落点 |
|---|------|------|------|
| Q1（批量 D0 Q4） | 证书生命周期状态机：ACME 惯例 6 状态 vs 简化 3 状态 | **ACME 惯例 6 状态**：pending → valid → expiring_soon（到期前 30 天）→ expired / renew_failed；支撑自动续期调度与失败排障 | ADR-024，A2 关闭 |

## 4. 澄清决策回填清单

- knowledge/design-decisions：ADR-024 追加（证书 6 状态机）
- knowledge/business-concepts：状态机表证书行更新为 6 状态（去 ⚠️A2 标记）
- knowledge/source-index：A2 → 已确认
- workspace/INDEX.md：site-mgmt.requirement → partial（D0 完成，D1-D4 工作流进行中）

## 5. D1 输入摘要（结构化需求）

**模块边界**：
- IN：存量扫描导入（扫描→解析→站点列表：域名/监听端口/根目录/证书状态/反代上游）、站点列表/详情/编辑（写回 Raw 保留）、新建站点三模板（静态 / PHP-FPM 对接已有 pool / 反向代理）、SSL 证书管理（ACME HTTP-01/TLS-ALPN 申请、部署、自动续期、存量证书识别导入、删除）、反向代理管理（上游地址、Host 头、WebSocket 透传开关）、Monaco 配置编辑器（保存前自动 `nginx -t`）、站点级启停（enabled 开关 → R-06 流程 → reload）、访问/错误日志尾部查看、nginx reload
- OUT：Apache/Caddy 等其他 web server（不做）、DNS-01 通配符证书（材料限定 HTTP-01/TLS-ALPN，二期）、多 nginx 实例并存管理（见待细化）、HSTS/安全头配置面板（二期）

**核心实体**：site、certificate（含 6 状态机）；upstream 作为 site 内嵌结构（D1 定是否独立对象）

**状态机**（已确认）：
- site：enabled ⇄ disabled（开关走 R-06 安全写入流程 → nginx reload）
- certificate：pending → valid → expiring_soon（≤30 天）→（自动续期成功 → valid｜到期 → expired｜续期失败 → renew_failed→ 重试→valid/expired）

**待 D1 细化（次要，不阻断）**：
- ⚠️ 单 nginx 实例/多配置目录合并策略：conf.d 与 sites-enabled 并存时的扫描去重与写回落点（建议：维护「站点 → 来源文件」映射，写回原文件）
- ⚠️ PHP-FPM pool 对接方式（发现已有 pool → 模板选择时列出；FastCGI 参数模板内置）
- 日志路径发现规则（server 块 access_log/error_log 指令解析 + 默认路径回退）
- 证书存储布局（建议 /etc/dougong/certs/ 或复用 nginx 惯例路径，D1 定）
