<!-- kb-generated: 2026-11-12 | sources: 1 | assumptions: 8 | open: 0 -->

# service-pool 知识库

> 由 /profile kb 从 workspace/00-architecture/idea.md 蒸馏生成（2026-11-12）。本文件只存索引，明细维护在 workspace/01-solution/05-select-service-pool.md（约定：knowledge 存索引，明细在 workspace conventions）。

> 本面板为单机运维面板：选择器数据源主要是**本机系统运行时数据**（服务/实例/站点/端口），无外部系统引用。路径与返回结构均为 D3 设计草案 ⚠️假设A8。

## 数据源索引

| 标识 | 用途（前端选择器） | 所属域 | 明细 |
|------|------------------|--------|------|
| syd-services | 服务管理列表、计划任务命令上下文 | 服务管理 | 05-select-service-pool.md #syd-services |
| db-instances | 数据库管理页实例选择 | 数据库管理 | 05-select-service-pool.md #db-instances |
| sites | 备份任务选择站点目录、证书绑定站点 | 网站管理 | 05-select-service-pool.md #sites |
| certs | 站点绑定证书选择 | 网站管理 | 05-select-service-pool.md #certs |
| system-users | 终端指定运行用户、文件 chown 属主选择 | 终端/文件管理 | 05-select-service-pool.md #system-users |
| firewall-ports | 防火墙监听端口对照 | 防火墙 | 05-select-service-pool.md #firewall-ports |
| docker-images | 容器重建/镜像拉取选择 | 容器管理 | 05-select-service-pool.md #docker-images |
