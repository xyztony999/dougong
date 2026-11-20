/**
 * db-tools 字典 S8 锚点（枚举池 04-business-enum.md 同值）+ 纯函数呈现助手
 * （PageSpec db-tools v1.2 第 3 节颜色方案 / 4.1 / 4.3 / 4.4 / database-mgmt 6.8 引擎映射）
 * + DBT_001~012 错误码全覆盖防缺码回归锚点（api-db-tools.md v1.3 3.4）
 * + v1.2 自动 nginx 落地呈现（landingPort/autoCreated 可空纪律/访问形态/安装成功放行
 * 提示/reason 细分引导，api D-19/D-22/D-23）。
 */
import { describe, expect, it } from 'vitest'
import { DB_TOOL_TASK_ACTION, DB_TOOL_TASK_STATUS, DB_TOOL_TYPE } from '../enums'
import type { DbToolItem } from '../dbtool'
import {
  DBT_ERROR_TEXT,
  DBTOOL_LANDING_PORT,
  DBTOOL_PAGE_INTRO,
  DBTOOL_PATH_PREFIX,
  dbtErrorText,
  dbtFailureGuide,
  dbtMissingOf,
  dbtReasonOf,
  dbtTaskIdOf,
  depMissingText,
  depsMissingSummary,
  depsReadyTag,
  installReadyAlertText,
  installedStateTag,
  instanceIdentityText,
  isDbToolTaskInFlight,
  landingAddress,
  landingHint,
  landingTag,
  openToastText,
  runningStateTag,
  toolForEngine
} from '../dbtool'

/** DbToolItem 测试夹具（api v1.3 全字段形状；默认=已装自动落地块形态，over 覆盖可空分支） */
function toolItem(over: Partial<DbToolItem> = {}): DbToolItem {
  return {
    toolType: 'phpmyadmin',
    displayName: 'phpMyAdmin',
    description: 'MySQL/MariaDB 高级管理',
    targetEngines: ['mysql', 'mariadb'],
    catalogVersion: '5.2.2',
    installed: true,
    installedVersion: '5.2.2',
    boundInstanceId: null,
    running: true,
    depsReady: true,
    depsMissing: [],
    runningTaskId: null,
    landingPort: 18090,
    autoCreated: true,
    ...over
  }
}

describe('db-tools 字典 S8 锚点（枚举池同值）', () => {
  it('DB_TOOL_TYPE 两值（D2 定稿，与池/api 三方同值）', () => {
    expect(Object.keys(DB_TOOL_TYPE)).toEqual(['phpmyadmin', 'mongo_express'])
    expect(DB_TOOL_TYPE.phpmyadmin.label).toBe('phpMyAdmin')
    expect(DB_TOOL_TYPE.mongo_express.label).toBe('mongo-express')
  })

  it('DB_TOOL_TASK_ACTION 三动作', () => {
    expect(Object.keys(DB_TOOL_TASK_ACTION)).toEqual(['install', 'upgrade', 'uninstall'])
    expect(DB_TOOL_TASK_ACTION.install.label).toBe('安装')
    expect(DB_TOOL_TASK_ACTION.upgrade.label).toBe('升级')
    expect(DB_TOOL_TASK_ACTION.uninstall.label).toBe('卸载')
  })

  it('DB_TOOL_TASK_STATUS 四态机 + PageSpec 第 3 节配色', () => {
    expect(Object.keys(DB_TOOL_TASK_STATUS)).toEqual(['pending', 'running', 'succeeded', 'failed'])
    expect(DB_TOOL_TASK_STATUS.pending.tag).toBe('default') // gray
    expect(DB_TOOL_TASK_STATUS.running.tag).toBe('info') // blue
    expect(DB_TOOL_TASK_STATUS.succeeded.tag).toBe('success') // green
    expect(DB_TOOL_TASK_STATUS.failed.tag).toBe('error') // red
  })
})

describe('toolForEngine 引擎映射（database-mgmt PageSpec 6.8）', () => {
  it('mysql/mariadb → phpmyadmin', () => {
    expect(toolForEngine('mysql')).toBe('phpmyadmin')
    expect(toolForEngine('mariadb')).toBe('phpmyadmin')
  })
  it('mongodb → mongo_express', () => {
    expect(toolForEngine('mongodb')).toBe('mongo_express')
  })
  it('redis 无入口（Redis 集成工具 OUT，D0 范围边界）', () => {
    expect(toolForEngine('redis')).toBeNull()
  })
})

describe('依赖缺失呈现（DBT_001 数据源，⚠️A2）', () => {
  it('kind=env → env_type displayName 映射（php_fpm → PHP-FPM）', () => {
    expect(depMissingText({ kind: 'env', name: 'php_fpm' })).toBe('PHP-FPM 环境未安装')
  })
  it('kind=env node → Node.js 标签（node 已入 ENV_TYPE 八值，W2 同步闭环，不再原样透传）', () => {
    expect(depMissingText({ kind: 'env', name: 'node' })).toBe('Node.js 环境未安装')
  })
  it('kind=env 未知值透传原样（dictItem 兜底，D-18 透传设计不变）', () => {
    expect(depMissingText({ kind: 'env', name: 'unknown_env' })).toBe('unknown_env 环境未安装')
  })
  it('kind=extension → PHP 扩展名原样（开放集合非枚举）', () => {
    expect(depMissingText({ kind: 'extension', name: 'mysqli' })).toBe('PHP 扩展 mysqli 未安装')
  })
  it('depsMissingSummary 顿号连接', () => {
    expect(
      depsMissingSummary([
        { kind: 'env', name: 'php_fpm' },
        { kind: 'extension', name: 'zip' }
      ])
    ).toBe('PHP-FPM 环境未安装、PHP 扩展 zip 未安装')
    expect(depsMissingSummary([])).toBe('')
  })
})

describe('三态呈现助手（D-02 实况不虚构）', () => {
  it('installedStateTag：已装 green+版本 / 未装 gray', () => {
    expect(installedStateTag(true, '5.2.2')).toEqual({ label: '已安装 · 5.2.2', tag: 'success' })
    expect(installedStateTag(false, null)).toEqual({ label: '未安装', tag: 'default' })
  })
  it('runningStateTag 三态：true green / false gray / null orange（探测失败不虚构）', () => {
    expect(runningStateTag(true)).toEqual({ label: '运行中', tag: 'success' })
    expect(runningStateTag(false)).toEqual({ label: '已停止', tag: 'default' })
    expect(runningStateTag(null)).toEqual({ label: '运行状态未知', tag: 'warning' })
  })
  it('depsReadyTag：就绪 green / 缺失 orange', () => {
    expect(depsReadyTag(true)).toEqual({ label: '环境就绪', tag: 'success' })
    expect(depsReadyTag(false)).toEqual({ label: '缺少依赖', tag: 'warning' })
  })
  it('isDbToolTaskInFlight：pending/running 在途，终态不在途', () => {
    expect(isDbToolTaskInFlight('pending')).toBe(true)
    expect(isDbToolTaskInFlight('running')).toBe(true)
    expect(isDbToolTaskInFlight('succeeded')).toBe(false)
    expect(isDbToolTaskInFlight('failed')).toBe(false)
  })
})

describe('打开分发文案（PageSpec 4.4，凭据零回显 I-02）', () => {
  it('phpmyadmin 有 tokenExpiresAt → 令牌 60 秒时效提示', () => {
    expect(
      openToastText({
        toolType: 'phpmyadmin',
        url: '/dbtool/phpmyadmin/?dg_signon=x',
        tokenExpiresAt: '2026-11-19T00:01:00Z'
      })
    ).toBe('已打开（登录凭证 60 秒内有效）')
  })
  it('mongo_express tokenExpiresAt=null → 普通提示（basic auth 服务端注入无感知）', () => {
    expect(
      openToastText({ toolType: 'mongo_express', url: '/dbtool/mongo-express/', tokenExpiresAt: null })
    ).toBe('已打开')
  })
})

describe('instanceIdentityText（实例选择器/绑定反查呈现）', () => {
  it('tcp → host:port', () => {
    expect(instanceIdentityText({ connMode: 'tcp', host: '127.0.0.1', port: 3306 })).toBe(
      '127.0.0.1:3306'
    )
  })
  it('socket → socketPath', () => {
    expect(instanceIdentityText({ connMode: 'socket', socketPath: '/var/run/mysqld/mysqld.sock' })).toBe(
      '/var/run/mysqld/mysqld.sock'
    )
  })
  it('缺失字段不虚构（— 占位）', () => {
    expect(instanceIdentityText({ connMode: 'socket' })).toBe('—')
    expect(instanceIdentityText({ connMode: 'tcp' })).toBe(':—')
  })
})

describe('DBT 错误码分流（api 3.4；dbmErrorText 同构先例）', () => {
  it('DBT_001~012 全覆盖防缺码（12 码连续唯一）', () => {
    for (let i = 1; i <= 12; i++) {
      const code = `DBT_${String(i).padStart(3, '0')}`
      expect(DBT_ERROR_TEXT[code], code).toBeDefined()
    }
    expect(Object.keys(DBT_ERROR_TEXT)).toHaveLength(12)
  })
  it('dbtErrorText 基础文案', () => {
    expect(dbtErrorText('DBT_003', null)).toBe('该工具有任务正在进行')
    expect(dbtErrorText('DBT_011', null)).toBe('已是最新版本，无需升级')
  })
  it('DBT_012 追加 data.reason（失败原因如实呈现）', () => {
    expect(dbtErrorText('DBT_012', { reason: '自动落地块写入失败: nginx -t 不通过' })).toBe(
      '安装或重置密码失败：自动落地块写入失败: nginx -t 不通过'
    )
    expect(dbtErrorText('DBT_012', null)).toBe('安装或重置密码失败')
  })
  it('DBT_012 reason 细分引导（v1.2，PageSpec 第 1 节）：多候选附站点管理引导', () => {
    expect(dbtErrorText('DBT_012', { reason: '落点冲突：多候选 server 块' })).toBe(
      '安装或重置密码失败：落点冲突：多候选 server 块——请到站点管理调整域名配置后重试'
    )
  })
  it('DBT_012 端口占用 reason 全文即引导（后端文案已含释放指引，不重复叠加）', () => {
    expect(dbtErrorText('DBT_012', { reason: '工具端口 18090 被占用，请释放后重试' })).toBe(
      '安装或重置密码失败：工具端口 18090 被占用，请释放后重试'
    )
  })
  it('未知码返回 null（AUTH 族/网络错误归拦截器，不双提示）', () => {
    expect(dbtErrorText('AUTH_008', null)).toBeNull()
    expect(dbtErrorText('DBT_999', null)).toBeNull()
  })
  it('dbtReasonOf 收窄（非对象/空串/非字符串 → null）', () => {
    expect(dbtReasonOf({ reason: 'x' })).toBe('x')
    expect(dbtReasonOf({ reason: '' })).toBeNull()
    expect(dbtReasonOf({ reason: 42 })).toBeNull()
    expect(dbtReasonOf(null)).toBeNull()
    expect(dbtReasonOf('str')).toBeNull()
  })
  it('dbtMissingOf 收窄（DBT_001 data.missing 内联清单；非法项过滤）', () => {
    expect(
      dbtMissingOf({
        missing: [
          { kind: 'env', name: 'php_fpm' },
          { kind: 'extension', name: 'gd' },
          { kind: 'other', name: 'x' },
          { kind: 'env' },
          null
        ]
      })
    ).toEqual([
      { kind: 'env', name: 'php_fpm' },
      { kind: 'extension', name: 'gd' }
    ])
    expect(dbtMissingOf(null)).toEqual([])
    expect(dbtMissingOf({ missing: 'not-array' })).toEqual([])
  })
  it('dbtTaskIdOf 收窄（DBT_003 在途任务查看引导）', () => {
    expect(dbtTaskIdOf({ taskId: 7 })).toBe(7)
    expect(dbtTaskIdOf({ taskId: '7' })).toBeNull()
    expect(dbtTaskIdOf(null)).toBeNull()
  })
})

describe('v1.2 常量锚点（api v1.3 D-13/D-19 同值）', () => {
  it('DBTOOL_LANDING_PORT=18090（TOOL_LANDING_PORT 编译期常量同值）', () => {
    expect(DBTOOL_LANDING_PORT).toBe(18090)
  })
  it('DBTOOL_PATH_PREFIX 两工具路径前缀（TOOL_PATH_PREFIX_* 同值）', () => {
    expect(DBTOOL_PATH_PREFIX.phpmyadmin).toBe('/dbtool/phpmyadmin/')
    expect(DBTOOL_PATH_PREFIX.mongo_express).toBe('/dbtool/mongo-express/')
  })
  it('页头说明条含自动端口 18090 每请求会话校验+绑域名仅域名语义（PageSpec 4.1 v1.2）', () => {
    expect(DBTOOL_PAGE_INTRO).toContain('18090')
    expect(DBTOOL_PAGE_INTRO).toContain('登录面板后才能访问')
    expect(DBTOOL_PAGE_INTRO).toContain('绑定域名后则只能用域名访问')
    expect(DBTOOL_PAGE_INTRO).toContain('表/文档浏览仍然可用')
  })
})

describe('访问形态区块（v1.2，landingPort/autoCreated 投影，PageSpec 4.1，api D-19/I-14）', () => {
  it('landingTag：已装+非零端口 → info「独立访问端口 {port}」', () => {
    expect(landingTag(toolItem())).toEqual({ label: '独立访问端口 18090', tag: 'info' })
  })
  it('landingTag：已装+landingPort=0 → default「面板域路径访问」（既有面板域块形态）', () => {
    expect(landingTag(toolItem({ landingPort: 0, autoCreated: false }))).toEqual({
      label: '经面板域名访问',
      tag: 'default'
    })
  })
  it('landingTag：landingPort=null → null 整块不渲染（JSON null≠undefined 显式判空，不虚构端口）', () => {
    expect(landingTag(toolItem({ landingPort: null, autoCreated: null }))).toBeNull()
  })
  it('landingTag：未装 → null（即使载荷异常携带端口亦不渲染——installed 档案存在性优先）', () => {
    expect(landingTag(toolItem({ installed: false, installedVersion: null }))).toBeNull()
  })
  it('landingTag：端口取投影值不硬编码 18090（非 18090 非零端口如实呈现）', () => {
    expect(landingTag(toolItem({ landingPort: 18091 }))).toEqual({
      label: '独立访问端口 18091',
      tag: 'info'
    })
  })
  it('landingAddress：hostname=当前面板访问主机（IP 直连呈现 IP，D-23 BindDomain 优先口径）', () => {
    expect(landingAddress(toolItem(), '192.168.1.10')).toBe(
      'http://192.168.1.10:18090/dbtool/phpmyadmin/'
    )
    expect(landingAddress(toolItem({ toolType: 'mongo_express' }), '192.168.1.10')).toBe(
      'http://192.168.1.10:18090/dbtool/mongo-express/'
    )
  })
  it('landingAddress：绑域名后经域名访问面板呈现域名', () => {
    expect(landingAddress(toolItem(), 'panel.example.com')).toBe(
      'http://panel.example.com:18090/dbtool/phpmyadmin/'
    )
  })
  it('landingAddress：port=0/null/未装 → null 不渲染地址行', () => {
    expect(landingAddress(toolItem({ landingPort: 0 }), '192.168.1.10')).toBeNull()
    expect(landingAddress(toolItem({ landingPort: null }), '192.168.1.10')).toBeNull()
    expect(landingAddress(toolItem({ installed: false }), '192.168.1.10')).toBeNull()
  })
  it('landingHint：放行 TCP 端口+绑域名仅域名访问语义（readonly 同等呈现纯观测）', () => {
    expect(landingHint(toolItem())).toBe(
      '请在防火墙/云安全组开放 18090 端口；绑定域名后只能用域名访问'
    )
    expect(landingHint(toolItem({ landingPort: 0 }))).toBeNull()
    expect(landingHint(toolItem({ landingPort: null }))).toBeNull()
  })
})

describe('安装成功放行提示（v1.2，PageSpec 4.1/4.3，api D-19⑦/D-23）', () => {
  const task = { action: 'install', status: 'succeeded', toolType: 'phpmyadmin' } as const

  it('install succeeded + autoCreated=true + 非零端口 → 放行提示含端口/地址/绑域名注记', () => {
    expect(installReadyAlertText(task, toolItem(), '192.168.1.10')).toBe(
      '工具已安装好：请在防火墙/云安全组开放端口 18090，然后访问 http://192.168.1.10:18090/dbtool/phpmyadmin/（绑定域名后用 http://域名:18090/...）'
    )
  })
  it('mongo_express 路径前缀同口径', () => {
    expect(
      installReadyAlertText(
        { action: 'install', status: 'succeeded', toolType: 'mongo_express' },
        toolItem({ toolType: 'mongo_express' }),
        '192.168.1.10'
      )
    ).toContain('http://192.168.1.10:18090/dbtool/mongo-express/')
  })
  it('非 install 动作/非 succeeded 终态 → null（upgrade/uninstall/running/failed 不渲染）', () => {
    expect(installReadyAlertText({ ...task, action: 'upgrade' }, toolItem(), 'h')).toBeNull()
    expect(installReadyAlertText({ ...task, action: 'uninstall' }, toolItem(), 'h')).toBeNull()
    expect(installReadyAlertText({ ...task, status: 'running' }, toolItem(), 'h')).toBeNull()
    expect(installReadyAlertText({ ...task, status: 'failed' }, toolItem(), 'h')).toBeNull()
  })
  it('autoCreated=false/null → null（既有面板域块/旧档案缺省不渲染放行提示）', () => {
    expect(installReadyAlertText(task, toolItem({ autoCreated: false }), 'h')).toBeNull()
    expect(installReadyAlertText(task, toolItem({ autoCreated: null }), 'h')).toBeNull()
  })
  it('landingPort=0/null → null（无独立端口不虚构；JSON null 显式判空）', () => {
    expect(installReadyAlertText(task, toolItem({ landingPort: 0 }), 'h')).toBeNull()
    expect(installReadyAlertText(task, toolItem({ landingPort: null }), 'h')).toBeNull()
  })
  it('工具档案缺失（findByType 未命中 null）→ null 不虚构', () => {
    expect(installReadyAlertText(task, null, 'h')).toBeNull()
  })
})

describe('失败原因引导 dbtFailureGuide（v1.2，api D-22 reason 细分，PageSpec 4.3）', () => {
  it('task.error 全文含端口占用 reason → release-port（释放端口后重试引导）', () => {
    expect(
      dbtFailureGuide('DBT_012: 部署操作失败: 工具端口 18090 被占用，请释放后重试')
    ).toBe('release-port')
  })
  it('task.error 全文含多候选 reason → site-landing（到站点管理明确落点 /sites）', () => {
    expect(
      dbtFailureGuide('DBT_012: 部署操作失败: 落点冲突：多候选 server 块（a.conf#0、b.conf#1）')
    ).toBe('site-landing')
  })
  it('其余失败分类/null/空串 → null 不虚构引导', () => {
    expect(dbtFailureGuide('sha256_mismatch')).toBeNull()
    expect(dbtFailureGuide('面板重启中断')).toBeNull()
    expect(dbtFailureGuide(null)).toBeNull()
    expect(dbtFailureGuide('')).toBeNull()
  })
})
