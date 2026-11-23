# 发版规定 / Release Policy

> 适用范围：斗拱 Dougong 面板（github.com/xyztony999/dougong）。
> 核心原则：**版本号与 GitHub Release 代表「用户经 install.sh 升级能拿到的二进制变更批次」，不是每次 commit 都发版。**

## 一、什么改动才发版（跳版本号 + 二进制 Release）

只有**会改变发布二进制**的改动才发版：

- 后端 Go 代码改动；
- 前端代码改动（含内嵌 UI 文案——前端经 `go:embed` 打进二进制）。

多个代码改动**攒批**，一次发一个 Release，**不逢 commit 必发**。

## 二、什么改动不发版（只 commit + push）

- **纯文档**：README、docs/、设计产物（profiles/）、记忆等——不影响二进制，只 commit，不跳版本、不发 Release。
- **install.sh**：取自 `raw.githubusercontent.com/.../main/install.sh`，改完**即时生效**，无需发版。
  修 bug 后提醒用户用 `?v=N` 查询参数绕 CDN 缓存（raw 缓存约 5 分钟）。
- **CI 配置**（不影响产物形态时）：只 commit。

> CHANGELOG 的「未发布（草稿，攒批中）」小节用于积累变更点，**草稿积累本身不算发版**；
> 正式 `## vX.Y.Z` 小节是发版动作的一部分（见第五节）。

## 三、版本号方案（语义化 `0.MINOR.PATCH`）

| 位 | 含义 | 例子 |
|----|------|------|
| MINOR | 新模块 / 新功能 | db-tools 工具集成、数据库表级治理 |
| PATCH | bug 修复 / 小改进 / UI 文案净化 | php-fpm 单元解析修复、文案人话化 |

- 多个 PATCH 攒一批再发；一个功能完成可发 MINOR。
- `0.x` 阶段接口可能变动，MINOR 也可能含破坏性变更——**如实写进 CHANGELOG「升级注意」**。
- 版本基线：当前 `v0.1.16`；下一个功能 → `0.2.0`；下一个修复批 → `0.1.17`。
  （历史 `0.1.0~0.1.16` 为旧「逢改必发」策略产物，不追溯重编号。）

## 四、发版时机（按需手动）

- **默认**：代码改动先 commit + push 攒着；**维护者明确说「发版」才打 tag**。不自动定时发版。
- **例外**：实机阻塞性 bug（用户无法正常使用）可立即发 PATCH 热修，仍需维护者确认。
- 纯文档/install.sh 改动随时 commit + push，不触发发版决策。

## 五、发版 checklist（打 tag 前必做）

1. **查并行 WIP**：`git status` + 在途工作流，勿把他人未完成改动卷入本次发版。
2. **查最新版本**：`gh release list` 确认当前最新 tag，避免撞号。
3. **写 CHANGELOG 小节**：正式 `## vX.Y.Z - YYYY-MM-DD`，含「变更内容」（分组列点）+「安装与升级」+「升级注意」（迁移号/破坏性）。
   —— `release.yml` 用 awk 提取 `## <tag>` 到下一个 `## ` 之间正文，**无小节即拒发**；小节标题须与 tag 名逐字一致。
4. **编译门控全绿**：`go build/vet/test -race`（backend）+ 前端 `build`（vue-tsc+vite）+ `vitest`。
5. **推送 main，等 CI 变绿再打 tag**：`release.yml` 不依赖 CI，tag 一推就直接构建发布。
   开发机是 Windows、CI 与服务器是 Linux，路径、权限这类平台差异只有 CI 能发现——本地全绿不代表 Linux 上全绿。
   需要时可在本机验证：`CGO_ENABLED=0 GOOS=linux go test -c -o x.test <包>`，再把 `x.test` 放进一次性容器运行
   （`docker run --rm --network none --read-only --user 65534:65534 --tmpfs /tmp --entrypoint /t -v <x.test>:/t:ro ubuntu:24.04 -test.v`）。
6. **打 tag 推送**：`git tag vX.Y.Z && git push origin vX.Y.Z` → `release.yml` 构建 linux amd64/arm64 + sha256sums + GitHub Release（ldflags 注版本）。
7. **验证**：Release 产物齐全 + CI 绿。
8. **通知升级**：提示用户服务器重跑 `install.sh` 升级（数据目录不动）+ 列出实机验证项。

> 2026-11-23 的教训（v0.4.0）：推送 main 后立即打了 tag，Release 成功，而 main 的 CI 在 Linux 上挂了一个路径判定测试
> （Windows 上恒绿）。修复只能发布后补（fe035d9）。自此打 tag 前必须先看到 main 的 CI 为绿。

### 漏节恢复

若先打 tag 才发现 CHANGELOG 无该小节（release 拒发）：
`git tag -d <v> && git push origin :refs/tags/<v>` 删本地+远端 tag → 补小节 commit+push → 在新 commit 重打同名 tag 推送。

## 六、安装与升级口径（写进每个 Release）

```bash
# 新装 / 升级（升级=重跑，数据目录 /var/lib/dougong 不动）
curl -fsSL https://raw.githubusercontent.com/xyztony999/dougong/main/install.sh | sudo bash
dougong version   # 应为 vX.Y.Z
```

国内服务器 GitHub RST：`DOUGONG_MIRROR=<镜像前缀> DOUGONG_VERSION=vX.Y.Z ...`（见 README「国内服务器安装」）。