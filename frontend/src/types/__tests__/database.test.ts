/**
 * database-mgmt 类型层测试（v0.1.4 反馈轮增量）：
 * ①dbmErrorText DBM_021/022 新码文案与 reason 附加（凭据 INTERNAL 修复轮 + mongo
 * 增量轮补齐的映射缺口——前端 toast 分流）；②字典 S8 锚点（DB_ENGINE 四值含
 * mongodb、MONGO_ROLE 三档——api-database-mgmt.md v2.0 / PageSpec v1.2 同值）。
 * v0.1.7 DBM_005 诊断轮增量：③DBM_005/006 reason 附加与文案订正（不再断言
 * 「用户名或密码错误」——1045 含账户不存在/host 落空）；④dbmAuthRejectedTips
 * 引擎族排查指引（纯函数，CI 无引擎依赖）。
 */
import { describe, expect, it } from 'vitest'
import { DB_ENGINE, MONGO_ROLE } from '../enums'
import { dbmAuthRejectedTips, dbmErrorText, reasonOf } from '../database'

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

describe('dbmErrorText（DBM_005/006 v0.1.7 UX 增强）', () => {
  it('DBM_005 文案不再断言密码错 + reason 附加（实拍 1045 回归锚点）', () => {
    expect(dbmErrorText('DBM_005', null)).toBe('认证被拒：账户、host 或密码未通过服务器验证')
    const reason =
      "authentication rejected: ERROR 1045 (28000): Access denied for user 'tonyxyz'@'localhost' (using password: YES)"
    expect(dbmErrorText('DBM_005', { reason })).toBe(
      `认证被拒：账户、host 或密码未通过服务器验证：${reason}`
    )
  })

  it('DBM_006 基础文案 + reason 附加；无 reason 不附加', () => {
    expect(dbmErrorText('DBM_006', null)).toBe('连接失败：网络或 socket 不可达')
    expect(dbmErrorText('DBM_006', { reason: "Can't connect to MySQL server on 'localhost' (111)" })).toBe(
      "连接失败：网络或 socket 不可达：Can't connect to MySQL server on 'localhost' (111)"
    )
    expect(dbmErrorText('DBM_006', {})).toBe('连接失败：网络或 socket 不可达')
  })

  it('reasonOf 收窄：非字符串/空串/非对象 → null', () => {
    expect(reasonOf({ reason: 123 })).toBeNull()
    expect(reasonOf({ reason: '' })).toBeNull()
    expect(reasonOf(null)).toBeNull()
    expect(reasonOf('raw')).toBeNull()
    expect(reasonOf({ reason: 'ok' })).toBe('ok')
  })
})

describe('dbmAuthRejectedTips（DBM_005 引擎族排查指引，v0.1.7）', () => {
  it('MySQL/MariaDB 三则：账户存在性/host 形态/密码一致（覆盖 1045 全部可能）', () => {
    for (const engine of ['mysql', 'mariadb']) {
      const tips = dbmAuthRejectedTips(engine)
      expect(tips).toHaveLength(3)
      expect(tips[0]).toContain('不存在的账户同样返回 Access denied')
      expect(tips[1]).toContain('localhost / 127.0.0.1 / %')
      expect(tips[1]).toContain('默认 host=localhost')
      expect(tips[2]).toContain('ALTER USER')
      expect(tips[2]).toContain('@ 等特殊字符面板原样传递')
    }
  })

  it('Redis：requirepass/ACL 口径', () => {
    const tips = dbmAuthRejectedTips('redis')
    expect(tips).toHaveLength(1)
    expect(tips[0]).toContain('requirepass')
    expect(tips[0]).toContain('ACL')
  })

  it('MongoDB：admin 库 authSource 口径（D-16）', () => {
    const tips = dbmAuthRejectedTips('mongodb')
    expect(tips).toHaveLength(1)
    expect(tips[0]).toContain('admin 库')
    expect(tips[0]).toContain('authSource=admin')
  })

  it('未知引擎兜底通用指引（不空手）', () => {
    const tips = dbmAuthRejectedTips('unknown')
    expect(tips.length).toBeGreaterThan(0)
    expect(tips[0]).toContain('账户已创建且密码正确')
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
