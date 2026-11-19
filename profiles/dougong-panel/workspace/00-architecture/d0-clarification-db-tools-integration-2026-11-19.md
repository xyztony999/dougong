# D0 澄清：数据库现成 Web 工具集成（db-tools，2026-11-19）

> 触发：用户实机反馈后提产品方向之问「MySQL/Mongo 是否用现成工具而非面板自造一套」。
> 用户拍板：**走宝塔式集成现成 Web 工具**。理由：dougong 未来要上 Node/Python 项目管理，
> 环境管理本就要装 PHP/Node 运行时，集成工具复用已管理环境，不违背哲学。
> 模式：新模块 db-tools（跨 env-mgmt/site-mgmt/service-mgmt/database-mgmt 消费）。

## 一、用户三项拍板（2026-11-19 AskUserQuestion）

- **Q1 范围分期 = 一步到位**：本轮 env-mgmt 新增 Node 环境（apt/dnf/yum + NodeSource 源）
  + 集成 phpMyAdmin（MySQL/MariaDB）与 mongo-express（MongoDB）两个工具。
- **Q2 访问安全 = 面板会话 gating 反代**：工具绑 localhost 不暴露独立公网端口；经面板 nginx
  路径反代，访问须面板登录会话（admin 专属）。phpMyAdmin 用 signon 桥接面板会话，
  mongo-express 用面板设定的 basic auth。统一鉴权/审计/角色，攻击面小。
- **Q3 现有浏览去留 = 保留作轻量入口**：v0.1.12/13 面板内表/文档浏览保留（快速查看/基础 CRUD）；
  phpMyAdmin/mongo-express 作高级管理入口，两者并存。实例/凭据/库/用户治理仍留面板。

## 二、工具选型与复用积木（代码取证）

- **phpMyAdmin**（PHP）管 MySQL/MariaDB：复用 env-mgmt **PHP-FPM**（已就绪，service.go:94，
  含版本化 unit 扫描 UnitPatternByFamily）+ site-mgmt **php_fpm 站点**（nginx+php-fpm）。
  需 PHP 扩展（mysqli/mbstring/zip/gd/json/curl 等）——安装/检测须覆盖。
- **mongo-express**（Node）管 MongoDB：复用**新增 Node 环境** + service-mgmt systemd 服务
  + site-mgmt **reverse_proxy 站点**（上游 127.0.0.1:8081）。
- env-mgmt 现七环境（nginx/apache/docker/mysql/redis/php-fpm/mongodb，service.go:67-107），
  **Node/Python 未纳入**——本轮 +Node（Python 归未来）。
- 下载分发沿 install.sh 先例：版本钉死 + sha256 校验 + 国内镜像选项（DOUGONG_MIRROR 同构）。

## 三、范围（本轮做）

1. **env-mgmt +Node 环境**：Catalog +EnvNode（NodeSource 源代配三系，LTS 版本钉死，
   安装/升级/卸载/服务操作/自启沿既有形态；npm 随 Node）。迁移 + 枚举 + 测试。
2. **db-tools 工具管理**（新模块或 database-mgmt 子域，D1 裁定聚合归属）：
   - 工具目录（phpMyAdmin/mongo-express）：状态（未装/已装/版本/运行态）、安装/升级/卸载。
   - phpMyAdmin：下载 tarball（钉版本+sha256+镜像）→ 解压管理目录 → 生成 config.inc.php
     （signon  auth 桥接）→ 经 site-mgmt 建 php_fpm 站点（nginx 路径）→ 扩展依赖检测。
   - mongo-express：npm 安装 → systemd 服务（env 注入 Mongo 凭据〔vault 取出〕+面板 basic auth
     +baseUrl 路径前缀+bind 127.0.0.1）→ 经 site-mgmt 建 reverse_proxy 站点。
   - 工具凭据（basic auth/signon 密钥）AES-GCM vault 永不回显（I-02 同构）。
3. **会话 gating 反代（安全核心）**：
   - 工具仅绑 localhost；nginx 工具路径 `auth_request` → 面板鉴权端点（校验面板会话 cookie
     + admin 角色）→ 200 放行 / 401 拒。工具路径在面板域下（cookie scope 覆盖）。
   - phpMyAdmin signon：面板生成签名令牌 → signon URL 经面板 API 校验 → 注入 MySQL 凭据到
     phpMyAdmin 会话（凭据取自 vault，零回显）。
   - mongo-express：nginx proxy_set_header 注入面板设定的 basic auth（或 signon 同构）。
   - 审计：工具安装/升级/卸载/访问开启记审计（R-03，detail 不含凭据）。
4. **database-mgmt 入口**：实例治理页/库行增「高级管理（phpMyAdmin/mongo-express）」入口
   （admin 专属；未装引导安装；readonly 负空间）。面板内轻量浏览保留并存。
5. **前端**：工具管理页（状态/安装/升级/卸载/打开）+ 入口按钮 + readonly 负空间。

## 四、安全底线（不可妥协）

- 工具**绝不**直接公网暴露（仅 localhost + 面板会话 gating）；无面板 admin 会话不可达。
- 凭据（MySQL/Mongo 连接凭据、basic auth、signon 密钥）vault AES-GCM 永不回显（I-02）。
- R-08 exec 参数数组（npm/systemctl 等）；下载 sha256 校验；R-03 写动作同事务审计脱敏。
- signon 令牌签名防伪 + 短时效 + 一次性（防重放）；auth_request 端点防 CSRF/越权。
- 卸载工具不删数据库数据（仅解除工具部署，沿 instanceRemove 哲学）。
- 零遥测（R-09）。

## 五、范围边界（本轮不做，登记后续）
- Python 环境 / 其他工具（phpRedisAdmin/redis-commander/Adminer/pgAdmin）。
- Redis 集成工具（保留面板 key 浏览；用户本轮仅点 MySQL/Mongo）。
- 工具内细粒度权限映射到面板 readonly（工具访问整体 admin 专属；readonly 不可达工具）。
- 面板内表/文档浏览下线（保留并存，Q3）。

## 六、验收口径（供 Gate 1/Gate 2）
- env-mgmt Node 环境四方同值（枚举池/api/PageSpec/代码）+ 迁移 + 三系安装测试（fake 替身）。
- db-tools 接口三段式 + DTO 可空如实 + 错误码族（新 DBT 码或复用，D3 裁定）+ 审计动作码。
- gating：无会话/非 admin → 401/403 实测；localhost 绑定实证；signon 令牌防伪回归。
- 编译门控：go build/vet/test -race + vue-tsc/vite build/vitest + linux 交叉 + gofmt + dist 同步。
- 实机冒烟（Linux）：PHP-FPM+扩展装 phpMyAdmin 可登录管 MySQL；Node 装 mongo-express 可管 Mongo；
  无会话访问被拒；卸载不删数据。归目标环境。