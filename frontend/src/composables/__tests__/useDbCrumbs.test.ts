/**
 * buildDbCrumbs 纯函数测试（database-mgmt PageSpec v1.4 7.2，F5 crumb 链规格）：
 * 数据库 → {引擎} 管理·{identity} → {database} → {table|collection}；
 * 末级当前页不可点（path=null）、其余各级可点；动态段 identity 取档案（缺失兜底 instanceId
 * 原值不虚构，I-05）；非数据库域路由返回空数组（MainLayout 单标题兜底）。
 */
import { describe, expect, it } from 'vitest'
import { buildDbCrumbs } from '../useDbCrumbs'
import type { DbCrumbInput } from '../useDbCrumbs'

const mysqlInstance: DbCrumbInput['instance'] = {
  engine: 'mysql',
  connMode: 'tcp',
  host: 'localhost',
  port: 3306
}

function input(overrides: Partial<DbCrumbInput> = {}): DbCrumbInput {
  return {
    routeName: 'db-mysql-manage',
    instanceId: '7',
    instance: mysqlInstance,
    ...overrides
  }
}

describe('buildDbCrumbs crumb 链（7.2 规格）', () => {
  it('db-list：单 crumb「数据库」末级不可点', () => {
    const crumbs = buildDbCrumbs(input({ routeName: 'db-list', instanceId: '' }))
    expect(crumbs).toEqual([{ label: '数据库', path: null }])
  })

  it('治理页：数据库（可点）/ MySQL 管理·localhost:3306（末级不可点）', () => {
    const crumbs = buildDbCrumbs(input())
    expect(crumbs).toEqual([
      { label: '数据库', path: '/databases' },
      { label: 'MySQL 管理·localhost:3306', path: null }
    ])
  })

  it('mariadb 档案走 mysql 路由：引擎显示名取档案（MariaDB 治理）', () => {
    const crumbs = buildDbCrumbs(
      input({ instance: { ...mysqlInstance, engine: 'mariadb' } })
    )
    expect(crumbs[1].label).toBe('MariaDB 管理·localhost:3306')
  })

  it('redis/mongo 治理页同构', () => {
    const redis = buildDbCrumbs(
      input({
        routeName: 'db-redis-manage',
        instance: { engine: 'redis', connMode: 'tcp', host: '127.0.0.1', port: 6379 }
      })
    )
    expect(redis[1]).toEqual({ label: 'Redis 管理·127.0.0.1:6379', path: null })
    const mongo = buildDbCrumbs(
      input({
        routeName: 'db-mongo-manage',
        instance: { engine: 'mongodb', connMode: 'tcp', host: 'localhost', port: 27017 }
      })
    )
    expect(mongo[1].label).toBe('MongoDB 管理·localhost:27017')
  })

  it('socket 档案 identity 呈现 socket 路径（D-01）', () => {
    const crumbs = buildDbCrumbs(
      input({
        instance: {
          engine: 'mysql',
          connMode: 'socket',
          socketPath: '/var/run/mysqld/mysqld.sock'
        }
      })
    )
    expect(crumbs[1].label).toBe('MySQL 管理·/var/run/mysqld/mysqld.sock')
  })

  it('档案缺失兜底 instanceId 原值（I-05 不虚构）；引擎名按路由段兜底', () => {
    const crumbs = buildDbCrumbs(input({ instance: null }))
    expect(crumbs[1].label).toBe('MySQL 管理·7')
  })

  it('表清单页（browse-database）：治理级可点回治理页，库名末级不可点', () => {
    const crumbs = buildDbCrumbs(
      input({
        routeName: 'db-mysql-tables',
        crumb: 'browse-database',
        database: 'app_prod'
      })
    )
    expect(crumbs).toEqual([
      { label: '数据库', path: '/databases' },
      { label: 'MySQL 管理·localhost:3306', path: '/databases/mysql/7' },
      { label: 'app_prod', path: null }
    ])
  })

  it('表详情页（browse-table）：库级可点回表清单，表名末级不可点', () => {
    const crumbs = buildDbCrumbs(
      input({
        routeName: 'db-mysql-table-detail',
        crumb: 'browse-table',
        database: 'app_prod',
        table: 'orders'
      })
    )
    expect(crumbs).toHaveLength(4)
    expect(crumbs[2]).toEqual({
      label: 'app_prod',
      path: '/databases/mysql/7/browse/app_prod'
    })
    expect(crumbs[3]).toEqual({ label: 'orders', path: null })
  })

  it('集合清单/文档页（mongo browse 链）同构', () => {
    const cols = buildDbCrumbs(
      input({
        routeName: 'db-mongo-collections',
        crumb: 'browse-database',
        database: 'shop',
        instance: { engine: 'mongodb', connMode: 'tcp', host: 'localhost', port: 27017 }
      })
    )
    expect(cols[1].path).toBe('/databases/mongo/7')
    expect(cols[2]).toEqual({ label: 'shop', path: null })
    const docs = buildDbCrumbs(
      input({
        routeName: 'db-mongo-documents',
        crumb: 'browse-collection',
        database: 'shop',
        collection: 'orders',
        instance: { engine: 'mongodb', connMode: 'tcp', host: 'localhost', port: 27017 }
      })
    )
    expect(docs[2].path).toBe('/databases/mongo/7/browse/shop')
    expect(docs[3]).toEqual({ label: 'orders', path: null })
  })

  it('库/表名含特殊字符：路径 encodeURIComponent 组装（点击安全）', () => {
    const crumbs = buildDbCrumbs(
      input({
        routeName: 'db-mysql-table-detail',
        crumb: 'browse-table',
        database: 'a%b',
        table: 't/1'
      })
    )
    expect(crumbs[2].path).toBe('/databases/mysql/7/browse/a%25b')
    expect(crumbs[2].label).toBe('a%b') // params 原值呈现（vue-router 已解码）
    expect(crumbs[3].label).toBe('t/1')
  })

  it('非数据库域路由 → 空数组（MainLayout 单标题兜底不强制改）', () => {
    expect(buildDbCrumbs(input({ routeName: 'site-list' }))).toEqual([])
    expect(buildDbCrumbs(input({ routeName: '' }))).toEqual([])
  })
})
