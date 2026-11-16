# 斗拱面板 知识注入

> /profile kb 生成（2026-11-12，材料：workspace/00-architecture/idea.md 项目计划书）。详细条目见 knowledge/ 四节；假设溯源见 knowledge/source-index.md。

## 业务一句话

轻量开源服务器运维面板 `dougong`：**接管存量环境**（nginx/MySQL/Redis/systemd/Docker 发现即接管，不重装），Go 单二进制 + Vue3 内嵌前端，零遥测，GPL-3.0。

## 知识四节

- **business-concepts**：24 业务对象（站点/容器/备份任务/备份执行记录/审计日志…）、状态机（站点启停/用户锁定/证书生命周期/备份执行）、2 角色（admin/readonly）、18 条业务规则（R-01~R-18，核心：nginx 写入"备份→修改→nginx -t→失败回滚"、round-trip raw 保留、禁 sh -c 拼接、零遥测）
- **design-decisions**：ADR-001~016（Go/Vue3/GPL-3.0/存量接管/MVP 范围/自研 nginx 解析器/SQLite/零遥测等；ADR-015 对象英文命名、ADR-016 backup_run 实体，均为 2026-11-12 澄清确认）
- **architecture-choices**：11 领域（网站管理为核心卖点）、栈选型表（gin/modernc-sqlite/gopsutil/lego/Docker SDK…，版本工程化时锁定）
- **service-pool**：7 个选择器数据源索引（本机系统运行时数据：服务/实例/站点/证书/系统用户/端口/镜像）

## 待办

- 假设 6 条（A2/A4/A6~A8/A10）复核：/profile kb 重跑确认，或留 /profile check C-11 WARN 清单（A1/A3/A5/A9 已确认）
