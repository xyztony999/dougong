# panel-core 前端生成任务清单（F01~F07）

> PageSpec：02-detailed/interaction/{admin,readonly}/panel-core/（admin v1.0 + readonly v1.0）
> 类型对齐：02-detailed/api/api-panel-core.md v1.0（唯一事实源）
> 前序模块：account-auth（脚手架/拦截器/守卫/布局已就位，本模块复用追加）

## 状态

| 任务 | 状态 | 产物 | 备注 |
|------|------|------|------|
| F01 模块分析 | [x] | 本文件 | 页面类型全 config：settings（6 Card 内联表单 + 2 页面级横幅）+ about（纯展示）；框架级行为归本模块（PageSpec 第 7 节）：侧边菜单改 menuList 服务端下发驱动（D-P07，替换 account-auth 占位硬编码菜单）；无列表页/无详情路由（PageSpec 第 4/6 节诚实声明）；readonly 按 ADR-018 负空间（4 个变更 Action 入口隐藏/禁用，控件禁用态保留可见性） |
| F02 类型定义 | [x] | src/types/panel.ts + src/types/enums.ts 追加 | detail 六分组结构逐字对齐 api 4.1.4；update 为部分字段集（listenAddress 可显式 null 清残留）；4 字典 Key（https_mode/listen_scope/panel_cert_source/config_effect_mode）入 enums.ts 单一事实源（PageSpec 第 3 节：与 account-auth 共用）；3 审计动作码（panel_config_changed/panel_cert_updated/panel_entry_path_regenerated）补入 AUDIT_ACTION_TEXT（审计页展示联动） |
| F03 api 层 | [x] | src/api/panel.ts + src/api/http.ts 追加 PANEL_* 分流 | 7 Action 一一对应（S7：configDetail/configUpdate/entryPathRegenerate/certUpload/menuList/versionGet/updateCheckRun）；拦截器追加 PANEL_001~006 统一文案（PageSpec 第 1 节错误处理行） |
| F04 页面组件 | [x] | src/views/panel-core/settings/**（index + 6 Card 组件）+ src/views/panel-core/about/index.vue + src/layouts/MainLayout.vue 菜单接入 | 见下方页面清单；无定时器/图表（更新检查为显式单次触发，R-09 无轮询） |
| F05 状态 | [x] | src/stores/panel.ts | panelStore（configDetail/menuItems/pendingRestart，PageSpec 第 1 节声明）——菜单树为框架级跨页面状态、configDetail 供 settings 与横幅共享 |
| F06 路由 | [x] | src/router/index.ts 追加 + vite.config.ts proxy 追加 /panel | /settings、/about 懒加载挂 MainLayout 子路由（requiresAuth 继承，admin+readonly 无 roles 限制）；守卫复用 account-auth 全局守卫（PageSpec 第 7 节：无新增守卫） |
| F07 合规检查 | [x] | — | frontend-rules §6 F-01~F-06 扫描全过；`npm run build`（vue-tsc --noEmit && vite build，cwd=frontend/）全绿 |

## 页面清单（2 页面 + 框架接入）

| 页面 | 路由 | 视图文件 | readonly 差异（ADR-018 负空间落地） |
|------|------|---------|----------------------------------|
| settings | /settings | views/panel-core/settings/index.vue + components/{ListenCard,EntryPathCard,HttpsCard,LangCard,SessionCard,UpdateCheckCard}.vue | 监听：控件禁用无保存；安全入口：开关禁用、路径值完整可见（ADR-018 全可看）、无重新生成；HTTPS：下拉禁用、导入区不渲染、摘要可见；语言：禁用无保存；会话：无差异（本只读）；更新检查：开关可见禁用、无检查按钮（外呼从紧） |
| about | /about | views/panel-core/about/index.vue | 无差异（纯展示） |
| 框架布局 | / | layouts/MainLayout.vue（菜单区改造） | menuList 服务端按角色过滤后直接渲染（前端不二次过滤，D-P07）；顶栏复用 account-auth 非本模块范围不动 |

## 已声明的实现偏差（诚实对应约束，不虚构）

1. **私钥 PEM 遮罩**（PageSpec 5.1 Card 3「NInput type=textarea（monospace，粘贴态遮罩）」）：Naive UI textarea 无密码遮罩能力 → keyPem 用 `n-input type="password" show-password-on="click"`（monospace 字体）承载遮罩语义；certPem 仍 textarea。I-P06 纪律不变（成功后双清空、零回显）。
2. **菜单图标名映射**：ModuleMenuRegistry 的 icon 为字符串名，前端在 MainLayout 维护 name→@vicons 组件映射（people/time/document/shield/settings/info + 兜底 Apps），未知名兜底图标不报错——后端未实现期不阻塞。
3. **会话参数展示格式**：api detail.session.{absoluteTimeout,idleTimeout} 未定义值格式（载体为配置文件 12h/2h）→ 前端按字符串原样展示，不做格式假设。
4. **入口前缀透明**（PageSpec 第 7 节）：前端全相对路径请求，不实现 `/{entryPath}` 前缀拼接；未携带前缀 404 已由 http.ts 既有分支承载（D-P03）。

## 门禁记录

- `npm run build`（= vue-tsc --noEmit && vite build，cwd=frontend/）：✅ 全绿（本文件 F07 行更新时点）
- frontend-rules §6 扫描：F-01~F-06 全过（命令与输出见交付 summary）
