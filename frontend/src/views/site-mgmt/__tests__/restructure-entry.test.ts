/**
 * 宝塔式重组（ADR-033）入口可达性测试——每个新增的用户可见动作，逐环断言整条链路
 * 在代码里真实连通，缺任一环即失败：
 *
 *   触发按钮 → 打开函数 / 置真 → 弹窗（v-model）→ 提交函数 → 前端 API 函数 → 接口地址
 *   → 后端路由注册（跨层：直接读后端 Go 源码，前端调用的每个地址后端必须真的注册了）
 *
 * 背景：v0.3.0 出现过「弹窗、处理函数、API、后端端点全在，但没有一个按钮能打开弹窗」
 * 的交付断裂（show*Create 从未被置真），当时所有门控都是绿的。挂载级组件测试沿全仓
 * 先例不引入，以源码断言承载；唯一例外是 sites-hub-mount.test.ts——「切换分栏时组件
 * 实例被原地复用」这类问题只有真实挂载才看得见，它用真实 Vue 运行时 + 极简宿主（不
 * 依赖 DOM）专门守这一处。
 */
import { readFileSync, readdirSync } from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'
import hubSource from '../SitesHub.vue?raw'
import paneSource from '@/views/app-mgmt/AppProjectsPane.vue?raw'
import appsTabSource from '@/views/docker/components/AppsTab.vue?raw'
import dockerViewSource from '@/views/docker/index.vue?raw'
import resourceBarSource from '@/views/monitoring/components/ResourceBar.vue?raw'
import routerSource from '@/router/index.ts?raw'
import appApiSource from '@/api/app.ts?raw'
import dockerApiSource from '@/api/docker.ts?raw'

const BACKEND_API = path.resolve(__dirname, '../../../../../backend/internal/api')

/** 读取后端某模块注册的全部接口地址：Prefix() + 各 .GET/.POST 路径（前缀与路由
 *  可能分布在该模块不同的源文件里，逐文件扫描） */
function backendRoutes(moduleDir: string): Set<string> {
  const dir = path.join(BACKEND_API, moduleDir)
  const sources = readdirSync(dir)
    .filter((file) => file.endsWith('.go') && !file.endsWith('_test.go'))
    .map((file) => readFileSync(path.join(dir, file), 'utf8'))
  const prefix = sources.map((src) => /Prefix\(\) string \{ return "([^"]+)" \}/.exec(src)?.[1]).find((p) => p !== undefined)
  expect(prefix, `后端模块 ${moduleDir} 未定义路由前缀`).toBeTruthy()
  const routes = new Set<string>()
  for (const src of sources) {
    for (const m of src.matchAll(/\.(?:GET|POST)\("([^"]+)"/g)) {
      routes.add(`${prefix}${m[1]}`)
    }
  }
  return routes
}

/** 取源码中某函数体（从 function 定义或 computed 定义起，到下一个顶层 function / 结尾） */
function bodyOf(src: string, name: string): string {
  const start = src.search(new RegExp(`(?:async )?function ${name}\\(|const ${name} = computed\\(`))
  expect(start, `缺少函数或计算属性 ${name}`).toBeGreaterThanOrEqual(0)
  const rest = src.slice(start + 1)
  const next = rest.search(/\n(?:async )?function \w+\(/)
  return next === -1 ? src.slice(start) : src.slice(start, start + 1 + next)
}

/** 断言「触发按钮 → 打开函数 → 弹窗 → 提交 → API 函数」四环连通 */
function expectEntryChain(src: string, opts: {
  action: string
  buttonRe: RegExp
  openFn: string
  showVar: string
  submitFn: string
  apiCall: string
}): void {
  const { action, buttonRe, openFn, showVar, submitFn, apiCall } = opts
  expect(src, `「${action}」缺少触发按钮`).toMatch(buttonRe)
  expect(bodyOf(src, openFn), `「${action}」的 ${openFn}() 没有打开弹窗（${showVar} 未置真）`)
    .toContain(`${showVar}.value = true`)
  expect(src, `「${action}」缺少绑定 ${showVar} 的弹窗`).toContain(`v-model:show="${showVar}"`)
  expect(bodyOf(src, submitFn), `「${action}」的 ${submitFn}() 没有调用 ${apiCall}`).toContain(`${apiCall}(`)
}

/** 断言前端 API 函数请求的地址在后端真实注册 */
function expectApiRoute(apiRoutes: Set<string>, apiSrc: string, fnName: string): void {
  const body = bodyOf(apiSrc, fnName)
  const route = /request(?:<[^>]*>)?\(\s*[`']([^`']+)[`']/.exec(body)?.[1]
  expect(route, `前端 API 函数 ${fnName} 的请求地址未找到`).toBeTruthy()
  expect(apiRoutes.has(route as string), `前端 ${fnName} 调用 ${route}，但后端没有注册这个地址`).toBe(true)
}

describe('网站管理页三分栏（网站 / Python 项目 / Node 项目）', () => {
  it('三个分栏齐全，且两个项目分栏按运行时各挂一份面板', () => {
    for (const [name, label] of [['site', '网站'], ['python', 'Python 项目'], ['node', 'Node 项目']] as const) {
      expect(hubSource, `缺少「${label}」分栏`).toMatch(new RegExp(`<NTabPane key="${name}" name="${name}" tab="${label}"`))
    }
    expect(hubSource).toContain('<AppProjectsPane runtime="python" />')
    expect(hubSource).toContain('<AppProjectsPane runtime="node" />')
    expect(hubSource).toContain('<SiteListPane />') // 既有站点列表原样作为「网站」分栏
    // 分栏必须带 key：不带 key 时切换分栏是原地改属性，面板的 setup 不会重跑
    // （行为层面的验证见 sites-hub-mount.test.ts）
    expect(Array.from(hubSource.matchAll(/<NTabPane\s/g))).toHaveLength(3)
    for (const m of hubSource.matchAll(/<NTabPane key="([^"]+)" name="([^"]+)"/g)) expect(m[1]).toBe(m[2])
  })

  it('分栏由地址栏参数 tab 持久化；路由指向三分栏外层，旧地址 /app 重定向', () => {
    expect(hubSource).toContain('route.query.tab')
    expect(routerSource).toContain("import('@/views/site-mgmt/SitesHub.vue')")
    expect(routerSource).toMatch(/path: 'app',\s*name: 'app-mgmt',\s*redirect: \{ path: '\/sites', query: \{ tab: 'python' \} \}/)
    expect(routerSource).not.toContain("import('@/views/app-mgmt/index.vue')")
  })

  it('监控总览资源条的「项目」入口指向网站管理的项目分栏（不再指向不存在的地址）', () => {
    expect(resourceBarSource).toContain("{ key: 'apps', label: '项目', path: '/sites?tab=python'")
    expect(resourceBarSource).not.toContain("path: '/apps'")
  })
})

describe('项目面板：创建入口与操作链路（Python / Node 同一面板）', () => {
  it('创建项目：表头与空态各一个入口 → 弹窗 → 提交 → appCreate，并带上运行时', () => {
    // 表头入口条件是 isAdmin；空态入口条件是 isAdmin && !loadError（两者都是 admin 专属）
    const buttonRe = /v-if="isAdmin[^"]*"[^>]*@click="createShow = true"[^>]*>\s*创建\{\{ runtimeLabel \}\}项目/g
    expect(paneSource.match(buttonRe)?.length, '创建入口应有两处（表头 + 空态）').toBe(2)
    // 两处入口都必须受 isAdmin 约束（readonly 不渲染创建入口）
    for (const m of paneSource.matchAll(/<NButton v-if="([^"]+)"[^>]*@click="createShow = true"/g)) {
      expect(m[1]).toMatch(/^isAdmin/)
    }
    expect(paneSource).toContain('v-model:show="createShow"')
    const submit = bodyOf(paneSource, 'submitCreate')
    expect(submit).toContain('appCreate(')
    expect(submit).toContain('runtime: props.runtime') // 运行时随所在分栏确定
  })

  it.each([
    ['启停', /onClick: \(\) => doService\(r, 'start'\)/, 'doService', 'appServiceOp('],
    ['重装依赖', /onPositiveClick: \(\) => doDeps\(r\)/, 'doDeps', 'appDepsReinstall('],
    ['参数', /onClick: \(\) => openOps\(r, 'params'\)/, 'openOps', 'loadEnv'],
    ['日志', /onClick: \(\) => openOps\(r, 'logs'\)/, 'openOps', 'loadLog'],
    ['建站', /onClick: \(\) => openSite\(r\)/, 'openSite', 'submitSite'],
    ['删除', /onClick: \(\) => openDelete\(r\)/, 'openDelete', 'submitDelete'],
    ['开机自启', /onPositiveClick: \(\) => toggleAuto\(r, !on\)|onUpdateValue: \(v: boolean\) => toggleAuto\(r, v\)/, 'toggleAuto', 'appServiceOp(']
  ])('%s：行内按钮 → 处理函数 → 接口调用', (_name, buttonRe, handler, callee) => {
    expect(paneSource).toMatch(buttonRe)
    // handler 自身直接调接口，或经它打开的弹窗提交函数调接口
    const direct = bodyOf(paneSource, handler).includes(`${callee}`)
    const viaModal = callee !== 'appServiceOp(' && callee !== 'appDepsReinstall(' && bodyOf(paneSource, callee).includes('app')
    expect(direct || viaModal, `「${_name}」链路未连通到接口调用（${handler} → ${callee}）`).toBe(true)
  })

  it('加载中如实呈现：清单返回前表格转加载态，就绪度与空状态不拿初始值充数', () => {
    // Node 清单要等 PM2 回话；返回前不能说「还没有项目」「环境缺失」
    expect(paneSource).toContain(':data="items" :loading="loading"')
    expect(bodyOf(paneSource, 'readyTag')).toContain('if (!loaded.value)')
    expect(bodyOf(paneSource, 'needEnvGuide')).toContain('loaded.value &&')
    expect(paneSource).toContain(":description=\"loaded ? `还没有${runtimeLabel}项目` : '未能读取项目列表'\"")
  })
})

describe('项目面板：Node 运行时（PM2）与容器运行形态下线', () => {
  it('Node 分栏的启动方式为 启动文件 / npm 脚本；「容器运行」选项已移除', () => {
    expect(paneSource).toContain("value: 'node_file'")
    expect(paneSource).toContain("value: 'npm_script'")
    expect(paneSource).not.toContain('容器运行')
    expect(paneSource).not.toContain("'container'")
  })

  it('Node 分栏呈现 PM2 实况（状态 / 重启次数 / 内存），开机自启开关切换前二次确认', () => {
    expect(paneSource).toContain('PM2_STATUS_TEXT')
    expect(paneSource).toContain('重启 ${r.restarts ?? 0} 次')
    expect(paneSource).toContain('关闭开机自启会同时停止应用')
    expect(paneSource).toContain('开启开机自启会同时启动应用')
  })

  it('Node 环境不完整逐项列出缺失（node / npm / PM2）并给环境管理入口', () => {
    const tag = bodyOf(paneSource, 'nodeMissing')
    for (const item of ['node', 'npm', 'PM2']) expect(tag).toContain(`'${item}'`)
    expect(paneSource).toContain('Node 环境不完整：缺')
    expect(paneSource).toContain('<router-link to="/env">')
  })

  it('参数页签按应用实际保存的启动参数预填（不再拿创建表单的上次输入顶替）', () => {
    const open = bodyOf(paneSource, 'openOps')
    expect(open).toContain('r.startSpec')
    expect(open).toContain('specInput.module = sp?.module')
    expect(open).toContain('specInput.file = sp?.file')
    expect(bodyOf(paneSource, 'saveParams')).toContain('appParamsSave(')
  })
})

describe('前端接口地址 ↔ 后端路由注册（跨层契约）', () => {
  const appRoutes = backendRoutes('appmgmt')
  const dockerRoutes = backendRoutes('dockermgmt')

  it.each(['appList', 'appCreate', 'appDelete', 'appEnvGet', 'appParamsSave', 'appDepsReinstall', 'appTaskRetry'])(
    '应用管理：%s 调用的地址后端已注册', (fn) => expectApiRoute(appRoutes, appApiSource, fn)
  )

  it('应用管理的六个服务操作地址后端均已注册', () => {
    for (const op of ['start', 'stop', 'restart', 'enable', 'disable']) {
      const route = `/app/service/${op}`
      expect(appRoutes.has(route), `后端缺少 ${route}`).toBe(true)
    }
    expect(bodyOf(appApiSource, 'appServiceOp')).toContain('/app/service/${op}')
    // 清单接口带运行时过滤参数
    expect(bodyOf(appApiSource, 'appList')).toContain('query: { runtime }')
  })

  it.each(['appList', 'appCreate', 'appService', 'appLogs', 'appDelete', 'appSite', 'composeDelete'])(
    '容器管理：%s 调用的地址后端已注册', (fn) => expectApiRoute(dockerRoutes, dockerApiSource, fn)
  )

  it('后端不再下发「应用管理」独立菜单', () => {
    const mainGo = readFileSync(path.resolve(BACKEND_API, '../../cmd/dougong/main.go'), 'utf8')
    expect(mainGo).not.toMatch(/Key: "app"/)
    expect(mainGo).toContain('Key: "sites"') // 网站管理菜单仍在
  })
})

describe('容器管理「应用」页签：入口与操作链路', () => {
  it('页签挂载在容器页签之后；创建应用 → 弹窗 → 提交 → appCreate', () => {
    expect(dockerViewSource).toMatch(/<n-tab-pane name="apps" tab="应用">\s*<AppsTab \/>/)
    expect(dockerViewSource.indexOf('name="apps"')).toBeGreaterThan(dockerViewSource.indexOf('name="containers"'))
    expectEntryChain(appsTabSource, {
      action: '创建应用',
      buttonRe: /v-if="isAdmin" type="primary" @click="openCreate"/,
      openFn: 'openCreate',
      showVar: 'createShow',
      submitFn: 'submitCreate',
      apiCall: 'appCreate'
    })
    // 空态里也有创建入口
    expect((appsTabSource.match(/@click="openCreate"/g) ?? []).length).toBe(2)
  })

  it.each([
    ['启动', /execService\(row, 'start'\)/],
    ['停止', /execService\(row, 'stop'\)/],
    ['重启', /execService\(row, 'restart'\)/]
  ])('应用%s：行内按钮 → execService → appService', (_name, re) => {
    expect(appsTabSource).toMatch(re)
    expect(bodyOf(appsTabSource, 'execService')).toContain('appService(')
  })

  it('日志 / 建站 / 删除：按钮 → 处理函数 → 接口调用', () => {
    expect(appsTabSource).toContain('void openLogs(row)')
    expect(bodyOf(appsTabSource, 'loadLogs')).toContain('appLogs(')
    expect(appsTabSource).toMatch(/onClick: \(\) => openSite\(row\)/)
    expect(bodyOf(appsTabSource, 'submitSite')).toContain('appSite(')
    expect(appsTabSource).toContain('onClick: () => openDelete(row)')
    expect(bodyOf(appsTabSource, 'submitDelete')).toContain('appDelete(')
  })

  it('创建表单覆盖拍板字段（名称/镜像/主机端口/容器端口/环境变量多组/挂载多组/重启策略）', () => {
    for (const label of ['应用名', '镜像', '主机端口', '容器端口', '环境变量', '挂载', '重启策略']) {
      expect(appsTabSource, `创建表单缺少「${label}」`).toContain(`label="${label}"`)
    }
    expect(appsTabSource).toContain('添加环境变量')
    expect(appsTabSource).toContain('添加挂载')
    expect(appsTabSource).toContain('应用端口只在本机可访问')
  })

  it('readonly 负空间：创建按钮与操作列均为 admin 专属', () => {
    expect(appsTabSource).toMatch(/v-if="isAdmin" type="primary" @click="openCreate"/)
    expect(appsTabSource).toMatch(/if \(isAdmin\.value\) \{\s*base\.push\(\{\s*title: '操作'/)
  })
})

describe('Compose 页签：零容器项目可见、可删除', () => {
  it('零容器项目呈现「未运行」', () => {
    expect(dockerViewSource).toContain("row.containers.length === 0")
    expect(dockerViewSource).toContain("'未运行'")
  })

  it('删除项目：按钮是确认框的触发器 → 确认后 execComposeDelete → composeDelete；确认文案区分是否删除目录', () => {
    // 接法必须是「按钮弹出确认，确认后才执行」。删除不可恢复；按钮自己绑 onClick 的话点一下就删了
    // （v0.4.0 开发中在真实浏览器里撞到，全仓同类写法由 __tests__/confirm-trigger.test.ts 把关）
    expect(dockerViewSource).toMatch(
      /const deleteBtn = h\(\s*NButton,\s*\{[^}]*\},\s*\{ default: \(\) => '删除项目' \},?\s*\)\s*nodes\.push\(\s*h\(\s*NPopconfirm,\s*\{ onPositiveClick: \(\) => void execComposeDelete\(row\) \},\s*\{\s*trigger: \(\) => deleteBtn,/
    )
    const del = bodyOf(dockerViewSource, 'execComposeDelete')
    expect(del).toContain('composeDelete(')
    expect(dockerViewSource).toContain('项目目录不在面板管理范围内，不会删除')
    expect(dockerViewSource).toContain('不可恢复')
  })
})
