# 选择服务池

> /profile kb 于 2026-11-12 从 workspace/00-architecture/idea.md 蒸馏的**草稿**条目（status: draft）。
> 本面板数据源均为本机系统运行时数据；API 路径/返回结构为 D3 设计草案 ⚠️假设A8，D3 定稿后去 draft。

| 标识 | 用途（前端选择器） | 所属域 | 路径（草案） | 返回结构（草案） |
|------|------------------|--------|-------------|----------------|
| syd-services | 服务管理列表、计划任务命令上下文 | 服务管理 | GET /api/systemd/services ⚠️假设A8 | [{name, active_state, sub_state, enabled}] |
| db-instances | 数据库管理页实例选择 | 数据库管理 | GET /api/db/instances ⚠️假设A8 | [{id, type(mysql\|mariadb\|redis), endpoint, version, status}] |
| sites | 备份任务选择站点目录、证书绑定站点 | 网站管理 | GET /api/sites ⚠️假设A8 | [{id, server_names[], root, enabled}] |
| certs | 站点绑定证书选择 | 网站管理 | GET /api/certs ⚠️假设A8 | [{id, domains[], issuer, not_before, not_after, source}] |
| system-users | 终端指定运行用户、文件 chown 属主选择 | 终端/文件管理 | GET /api/system/users ⚠️假设A8 | [{name, uid, gid, home, shell}] |
| firewall-ports | 防火墙监听端口对照 | 防火墙 | GET /api/firewall/ports ⚠️假设A8 | [{port, proto, process, open}] |
| docker-images | 容器重建/镜像拉取选择 | 容器管理 | GET /api/docker/images ⚠️假设A8 | [{name, tag, size}] |
