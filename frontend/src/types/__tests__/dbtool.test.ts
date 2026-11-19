/**
 * db-tools 字典 S8 锚点（枚举池 04-business-enum.md 同值）+ 纯函数呈现助手
 * （PageSpec db-tools v1.1 第 3 节颜色方案 / 4.1 / 4.4 / database-mgmt 6.8 引擎映射）
 * + DBT_001~012 错误码全覆盖防缺码回归锚点（api-db-tools.md v1.2 3.4）。
 */
import { describe, expect, it } from 'vitest'
import { DB_TOOL_TASK_ACTION, DB_TOOL_TASK_STATUS, DB_TOOL_TYPE } from '../enums'
import {
  DBT_ERROR_TEXT,
  dbtErrorText,
  dbtMissingOf,
  dbtReasonOf,
  dbtTaskIdOf,
  depMissingText,
  depsMissingSummary,
  depsReadyTag,
  installedStateTag,
  instanceIdentityText,
  isDbToolTaskInFlight,
  openToastText,
  runningStateTag,
  toolForEngine
} from '../dbtool'

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
    expect(depMissingText({ kind: 'extension', name: 'mysqli' })).toBe('PHP 扩展 mysqli 缺失')
  })
  it('depsMissingSummary 顿号连接', () => {
    expect(
      depsMissingSummary([
        { kind: 'env', name: 'php_fpm' },
        { kind: 'extension', name: 'zip' }
      ])
    ).toBe('PHP-FPM 环境未安装、PHP 扩展 zip 缺失')
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
    expect(runningStateTag(null)).toEqual({ label: '运行态未知', tag: 'warning' })
  })
  it('depsReadyTag：就绪 green / 缺失 orange', () => {
    expect(depsReadyTag(true)).toEqual({ label: '依赖就绪', tag: 'success' })
    expect(depsReadyTag(false)).toEqual({ label: '依赖缺失', tag: 'warning' })
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
    ).toBe('已打开（令牌 60 秒内有效）')
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
    expect(dbtErrorText('DBT_003', null)).toBe('该工具有任务进行中')
    expect(dbtErrorText('DBT_011', null)).toBe('当前已为目录版本，无需升级')
  })
  it('DBT_012 追加 data.reason（失败原因如实呈现）', () => {
    expect(dbtErrorText('DBT_012', { reason: '落点冲突：多候选 server 块' })).toBe(
      '工具部署/轮换操作失败：落点冲突：多候选 server 块'
    )
    expect(dbtErrorText('DBT_012', null)).toBe('工具部署/轮换操作失败')
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
