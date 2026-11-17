/**
 * database-mgmt 类型层测试（v0.1.4 反馈轮增量）：
 * ①dbmErrorText DBM_021/022 新码文案与 reason 附加（凭据 INTERNAL 修复轮 + mongo
 * 增量轮补齐的映射缺口——前端 toast 分流）；②字典 S8 锚点（DB_ENGINE 四值含
 * mongodb、MONGO_ROLE 三档——api-database-mgmt.md v2.0 / PageSpec v1.2 同值）。
 */
import { describe, expect, it } from 'vitest'
import { DB_ENGINE, MONGO_ROLE } from '../enums'
import { dbmErrorText } from '../database'

describe('dbmErrorText（DBM_021/022 增量）', () => {
  it('DBM_021 通道不可用：基础文案 + reason 附加', () => {
    expect(dbmErrorText('DBM_021', null)).toBe(
      '引擎客户端通道不可用（宿主机未安装 mysql/redis-cli）'
    )
    expect(dbmErrorText('DBM_021', { reason: 'dbmysql: "mysql" not found in PATH' })).toBe(
      '引擎客户端通道不可用（宿主机未安装 mysql/redis-cli）：dbmysql: "mysql" not found in PATH'
    )
  })

  it('DBM_022 权限不足：基础文案 + reason 附加（mongo serverStatus Unauthorized）', () => {
    expect(dbmErrorText('DBM_022', null)).toBe('引擎操作被拒（凭据账户权限不足）')
    expect(dbmErrorText('DBM_022', { reason: 'not authorized on admin' })).toBe(
      '引擎操作被拒（凭据账户权限不足）：not authorized on admin'
    )
  })

  it('未知码返回 null（不提示）', () => {
    expect(dbmErrorText('DBM_999', null)).toBeNull()
  })
})

describe('字典 S8 锚点（枚举池同值）', () => {
  it('DB_ENGINE 四值含 mongodb（enum_value_added 同步）', () => {
    expect(Object.keys(DB_ENGINE)).toEqual(['mysql', 'mariadb', 'redis', 'mongodb'])
    expect(DB_ENGINE.mongodb.label).toBe('MongoDB')
    expect(DB_ENGINE.mongodb.tag).toBe('success') // PageSpec 第 3 节 mongodb=green
  })

  it('MONGO_ROLE 三档（D-19：read/readWrite/dbAdmin）', () => {
    expect(Object.keys(MONGO_ROLE)).toEqual(['read', 'readWrite', 'dbAdmin'])
    expect(MONGO_ROLE.dbAdmin.tag).toBe('error') // 高危着色
  })
})
