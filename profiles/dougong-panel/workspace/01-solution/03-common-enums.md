# 公共枚举池

> /profile kb 于 2026-11-12 初始化的**草稿**条目（status: draft）。D1/D3 阶段核对定稿。

## 通用约定

| 条目 | 约定 | 状态 | 来源 |
|------|------|------|------|
| 通用启停 | enabled \| disabled（站点/服务自启/更新检查开关等复用） | draft | 材料 §4.1 多处 |
| 操作结果 | success \| failure | draft ⚠️假设A9 | 材料"审计记录结果" |
| 排序方向 | asc \| desc | draft | 惯例生成 |
| 时间存储 | ISO-8601，UTC 存储、前端本地时区展示 | draft | 惯例生成（审计/日志/证书有效期均涉时间） |
| 分页 | page（1 起）+ page_size，返回 {items, total} | draft | 惯例生成 |
| 布尔表示 | API JSON 用 true/false，存储 0/1（SQLite） | draft | 惯例生成（modernc.org/sqlite） |
