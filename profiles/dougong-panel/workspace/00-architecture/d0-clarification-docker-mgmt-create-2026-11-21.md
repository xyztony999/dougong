# D0 澄清：docker-mgmt 新建容器/Compose/网络/卷 + app-mgmt 容器化应用（增量轮，2026-11-21）

> 触发：用户实机自查终结 Gate1 W2——session/current 实证 role=admin、镜像 Tab「拉取」
> 与网络 Tab「删除」按钮正常渲染，权限/渲染无缺陷；主会话核 routes.go 定性：docker-mgmt
> 设计边界本不含创建类端点（container 零 create、volume/network 仅 delete、compose 仅
> up/down/restart）。用户拍板现在补创建类能力。此前「容器侧仅排查」拍板由本决定取代。

## 一、用户拍板（2026-11-21 两轮 AskUserQuestion）

- **Q1 容器创建参数集 = 基础全集**：镜像（可选缺失自动拉取）/名称/端口映射（多组）/
  环境变量（多组）/挂载（卷名或主机路径，多组）/重启策略（no/always/unless-stopped/
  on-failure）/网络选择（既有网络下拉默认 bridge）。不含命令/入口点覆写、labels、
  设备等高级项（后续增量可补）。
- **Q2 创建行为 = 创建并启动 + 镜像缺失自动拉取**：docker run 语义一步到位；镜像本地
  不存在时自动先拉取（复用既有 PullImage 同步通道），拉取失败如实报错不创建。
- **Q3 新建 Compose 项目 = 粘贴 + 自动建目录 + 自动启动**：表单粘贴 compose.yml 文本
  （服务端 YAML 语法校验，报错带行号）→ 工作目录自动建在面板指定根
  （/opt/dougong-compose/{项目名}/docker-compose.yml，项目名=目录名，小写连字符
  校验候选同应用名规则）→ 写入后自动 compose up -d → 项目出现在既有 Compose 列表
  （label/文件识别投影天然兼容）。
- **Q4 可选顺带 = 新建网络 + 新建卷都做**：新建网络（名称/驱动默认 bridge/子网网关
  可选）；新建卷（名称/驱动默认 local）；两者与容器创建表单的网络/挂载下拉直接联动。
- **Q5 容器化应用 = 本轮一起做**：应用管理新增「容器化」应用类型——创建表单二选一
  （venv/systemd 或 容器化）；容器化应用由面板生成单应用 compose.yml（镜像/端口/
  环境变量/挂载/重启策略 + dougong-app 归属标签）→ 经 Compose 创建通道落盘并自动
  up -d；应用视角启停/重启/日志（容器日志）/删除（compose down + 目录显式勾选删）；
  docker-mgmt 容器全网视图照常可见这些容器（带归属标签）——两处看同一底层资源，
  状态天然一致（用户关切「会不会乱」的答案：标签区分归属，容器管理保持全网视角不
  遮挡，应用管理提供单应用语义视角）。

## 二、匹配结论（增量判定）

- docker-mgmt（C-12/S-45~S-50）：容器创建并入容器运维阶段动作集、Compose 项目创建
  为 Compose 治理阶段新 L4、网络/卷创建并入资源治理对应子模块动作集——**增强**，
  零新模块零新聚合根（daemon 实况投影零台账哲学不变：新建对象即时进入实况投影）。
- app-mgmt（C-16）：容器化应用为应用类型扩展（创建链路分支 + 运维通道分支）——
  **增强**；聚合根 app/app_task 集合不变，app 档案扩展承载容器化形态引用。
- 跨模块：app-mgmt 容器化应用消费 docker-mgmt 的 Compose 项目创建/操作能力
  （OHS 或共享适配层，D1 裁定）；docker-mgmt ComposeRunner/dockerx 共享适配层
  沿 SysdGateway 先例。

## 三、同轮缺陷修复（并入本增量，先于主线落码）

- **缺陷 1（env-mgmt 真缺陷，实机 RHEL）**：python3 预装→「已装」态 pip 永远缺位。
  根因双因：Python 条目未配 ExtraPkgsByFamily（worker 补装环节不触发）+ PkgByFamily
  为探测候选数组安装只取首选 python3（dnf/yum 清单原本就没有 pip 包）。修复：extras
  移入 venv/pip 支撑包（apt python3-venv+python3-pip / dnf·yum python3-pip）——
  「已装但不完整」经升级动作补装；回归 python_env_test.go 两条（目录结构+安装链
  次序含 extras）。UI：就绪度能力位如实呈现缺 venv/pip（PageSpec v1.5 已在位）。
- **缺陷 2（app-mgmt 前端排版）**：n-data-table 默认「无数据」与自定义空态双渲染。
  修复：空态收敛为表格 #empty 插槽（自定义文案+创建按钮居中，删除独立 NEmpty）。
  视图层挂载级测试沿全仓先例不引入（@vue/test-utils 不在 devDeps），修复以模板
  改动+构建/vitest 全绿承载。

## 四、范围边界（本轮不做）

- 容器创建高级项（命令/入口点覆写、labels 手工编辑、设备、CPU/内存限制）——二期
- 容器化应用多容器 compose（单应用单 compose 单服务起步）；应用内数据库/依赖编排
- compose.yml 在线编辑器（先经文件管理器或重创建）；Compose 项目删除（down 后目录
  清理归二期，本轮 down 即止）
- 镜像构建（Dockerfile build）——镜像仅拉取既有形态
- Windows/无 docker daemon 环境不支持（既有支持矩阵如实报错）

## 五、验收口径（供 Gate 1/Gate 2）

- docker-mgmt 新增端点三段式 + 错误码族（DK 接力）+ 审计动作码 + 权限矩阵
  （读两角色/写 admin+CSRF）；容器创建参数校验（名称格式/端口映射格式/环境变量
  格式）服务端权威；镜像缺失自动拉取链路 fake 锁定；Compose YAML 校验失败报错
  带行号；落盘目录防逃逸（项目名白名单+根前缀约束）；网络/卷创建 daemon 拒绝
  为最终权威（重名等如实透传）。
- app-mgmt 容器化应用：创建表单二选一、容器化链路（生成 compose+标签→落盘→up）
  fake 全覆盖；应用视角启停/日志/删除走容器通道回归；docker-mgmt 列表可见归属
  容器（投影天然覆盖）；双角色边界维持（⚠️A8 日志 admin 专属等）。
- env-mgmt 缺陷 1 回归：python extras 补装链（已锁定）。
- 编译门控：go build/vet/test -race + vue-tsc/vite build + vitest + linux 交叉 +
  gofmt + dist 同步复验。
