# Gate 2 报告 · app-mgmt 代码→交付门控（2026-11-20）

> 判定：**WARN（CRITICAL 0，沿全仓先例）**——编译门控全绿、栈扫描 0 违例、
> 契约一致；WARN 均为环境绑定与流程性事项，待用户经主会话确认。

## 一、编译门控（全绿）

| # | 门控 | 结果 |
|---|------|------|
| 1 | go build ./... | ✅ exit 0 |
| 2 | go vet ./... | ✅ exit 0 |
| 3 | go test ./... -race -count=1 | ✅ 全仓 55 包 0 FAIL（appmgmt service 8 函数+api 权限矩阵全栈 1；回归含 envmgmt 九条目更新后全绿） |
| 4 | npm run build（vue-tsc --noEmit + vite build） | ✅ exit 0 |
| 5 | npx vitest run | ✅ 22 文件 292 用例全过（+app store 4；env.test ENV_TYPE 九值更新） |
| 6 | gofmt -l | ✅ 0 文件 |
| 7 | GOOS=linux CGO_ENABLED=0 交叉编译 | ✅ exit 0 |
| 8 | dist 同步 backend/web/dist 后 go build 复验 | ✅ exit 0（make web，assets 222 文件） |

## 二、栈规则扫描（实跑 grep）

| 规则 | 结果 |
|------|------|
| G-01 shell 拼接（sh -c/bash -c/cmd /c） | ✅ 0 命中（appx 全部 exec.CommandContext 参数数组） |
| G-02 SQL 拼接 | ✅ SQL 仅 store/appmgmt（静态字面量 WHERE 片段+?绑定，envmgmt List 同形）；层外 0 命中 |
| G-03 硬编码凭据 | ✅ 0 命中（.env 为用户自有配置文件不入库不加密 OUT-14，审计脱敏 ⚠️A9） |
| G-04 层外 SQL | ✅ 0（service/api 无 SQL） |
| G-05 panic | ✅ 0 命中（appmgmt 三包） |
| G-06 goroutine 退出路径 | ✅ dispatch=创建即触发有限时命令链（单命令 ≤PYOpTimeout，链路上界 3×，无长驻循环）；轮询定时器 onUnmounted 清理 |
| G-07 vet/test | ✅ 全绿 |
| F v-html/eval/localStorage | ✅ 0 命中（app-mgmt 视图/store/api） |

## 三、契约一致（AL 抽验）

- S7：API 16 接口 ↔ api/app.ts 16 Action ↔ admin PageSpec 16 Action 机械一致；readonly 3 读+13 负空间=16 全集。
- S8：app_task_action/app_task_status/app_start_mode 池↔api↔enums.ts↔store 逐值同源；env_type 九值池↔api-env-mgmt v1.4↔前端 enums↔env.test 断言五方同值。
- DDL↔字段清单：015 两表列与 api §5.3 逐列核对一致；错误码 APP_001~012 双侧（errcode+前端 APP_ERROR_TEXT）12/12。

## 四、实现亮点（测试锁定）

- **A-APP3（真缺陷修复）**：runTask 原守卫 `!= pending` 会吞掉 Retry（已置 running）的再入——修正为仅终态早退，测试 TestRetrySemantics 锁定（deps 失败→重试→succeeded 全链）。
- I-C01/C02 档案协作同事务：创建成功=任务终态+档案 INSERT 同事务（失败无半档案残留）；删除=终态+档案 DELETE 同事务——TestCreateWorkerChain/TestDeleteChain 锁定。
- ⚠️A8 日志 admin 专属：readonly /app/log、/app/app/env 越界 AUTH_008 双保险（api 全栈测试断言）。

## 五、WARN（待用户确认）

| # | 事项 | 处置建议 |
|---|------|---------|
| W1 | python3/pip/venv/systemd/journalctl/nginx 建站实联归 Linux 目标环境冒烟（Windows 开发机替身全注入如实声明） | 冒烟清单：创建 FastAPI 应用→systemd 拉起→建站反代可达→日志可见→卸载干净；Python 未装引导链路 |
| W2 | 建站 OHS 提供方登记（api-site-mgmt.md §5.1）归 site-mgmt 联动轮（Gate 1 W1 用户已接受） | site-mgmt 增量轮兑现 |
| W3 | api 层 service覆盖率：appmgmt service 包以 8 测试函数覆盖主链（校验链/创建/删除/重试/互斥/恢复/基线/建站），错误分支余量沿先例 | 沿先例待确认；ADR 归后续补齐轮 |
| W4 | 工作区存在本轮外未提交改动（沿交付节奏惯例：改动留工作区攒批，未自动 git commit） | 发版决策：feature→minor（攒批手动 tag），本轮不执行发版 |
| W5 | 前端 .vue 挂载级测试沿全仓先例未引入（@vue/test-utils 不在 devDeps） | store/types 层 4 用例覆盖核心流 |

## 六、交付清单

- 后端：backend/internal/{store/appmgmt,service/appmgmt,api/appmgmt,adapter/appx,errcode/appmgmt.go} + migrations/{015_app_mgmt,016_env_mgmt_python}.sql + envpack 能力位探测 + envsvc PythonReadiness OHS + main 装配（菜单 Order 860）
- 前端：frontend/src/{types/app.ts,api/app.ts,stores/app.ts,views/app-mgmt/index.vue} + enums 三字典 + EnvType 九值 + 路由 /app + MENU_ICONS apps + 测试 2 文件
- 设计产物：D0-D4 全链 + Gate 1（见 05-reports/gate1-app-mgmt-2026-11-20.md）
- tasks.md：backend/frontend 各追加 app-mgmt 增量节（T01~T07/F01~F05 全勾）
