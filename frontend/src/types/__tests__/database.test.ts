/**
 * database-mgmt 类型层测试（v0.1.4 反馈轮增量）：
 * ①dbmErrorText DBM_021 新码文案与 reason 附加（凭据 INTERNAL 修复轮补齐的映射缺口）；
 * ②字典 S8 锚点（DB_ENGINE 四值含 mongodb、MONGO_ROLE 三档）。
 * v0.1.7 DBM_005 诊断轮增量：③DBM_005/006 reason 附加与文案订正；④dbmAuthRejectedTips。
 * v1.4 增量（api v2.3）：⑤DBM_022~028 七码文案（DBM_022 按 v2.3 裁定为 TABLE_NOT_FOUND——
 * 此前实现轮曾用作引擎权限不足，编号冲突归后端增量轮重编号，前端以 api v2.3 为准）；
 * ⑥表级/集合文档纯函数（分页收窄/集合名校验/系统库判定/行值呈现/JSON 双层校验前端层/
 * _id 呈现与置顶/变更列收集/rescan toast merged 统计）。
 * A-DB13 闭环增量（2026-11-18 Gate 2 E1 修复）：⑦DBM_029 引擎操作被拒文案+reason 附加
 * （后端 ErrDBMEngineUnauthorized 改编号 DBM_029，前端映射同步）+DBM_001~029 全覆盖锚点
 * （防缺码→dbmErrorText null→toastDbm 静默吞错回归）。
 */
import { describe, expect, it } from 'vitest'
import { DB_ENGINE, MONGO_ROLE } from '../enums'
import {
  buildChangedRowValues,
  clampBrowsePageSize,
  DBM_ERROR_TEXT,
  dbmAuthRejectedTips,
  dbmErrorText,
  docWithIdFirst,
  formatDocId,
  isMongoSystemDatabase,
  parseDocumentJson,
  reasonOf,
  rescanToastText,
  rowCellText,
  validateCollectionName
} from '../database'
import type { TableRow } from '../database'

describe('dbmErrorText（DBM_021 增量）', () => {
  it('DBM_021 通道不可用：基础文案 + reason 附加', () => {
    expect(dbmErrorText('DBM_021', null)).toBe(
      '引擎客户端通道不可用（宿主机未安装 mysql/redis-cli）'
    )
    expect(dbmErrorText('DBM_021', { reason: 'dbmysql: "mysql" not found in PATH' })).toBe(
      '引擎客户端通道不可用（宿主机未安装 mysql/redis-cli）：dbmysql: "mysql" not found in PATH'
    )
  })

  it('未知码返回 null（不提示）', () => {
    expect(dbmErrorText('DBM_999', null)).toBeNull()
  })
})

describe('dbmErrorText（v1.4 DBM_022~028 七码，api v2.3 §3.4）', () => {
  it('DBM_022 表不存在（v2.3 裁定 TABLE_NOT_FOUND）', () => {
    expect(dbmErrorText('DBM_022', null)).toBe('目标表不存在')
  })

  it('DBM_023 无单列主键：基础文案 + reason 附加（无主键/复合主键原因如实）', () => {
    expect(dbmErrorText('DBM_023', null)).toBe('该表无单列主键，不支持行级编辑/删除')
    expect(dbmErrorText('DBM_023', { reason: '复合主键（2 列）' })).toBe(
      '该表无单列主键，不支持行级编辑/删除：复合主键（2 列）'
    )
  })

  it('DBM_024/025/026/027/028 基础文案（行/集合/JSON/文档/系统库保护）', () => {
    expect(dbmErrorText('DBM_024', null)).toBe('行已不存在或已被修改')
    expect(dbmErrorText('DBM_025', null)).toBe('目标集合不存在')
    expect(dbmErrorText('DBM_026', null)).toBe('文档 JSON 非法')
    expect(dbmErrorText('DBM_027', null)).toBe('文档已不存在')
    expect(dbmErrorText('DBM_028', null)).toBe('系统库（admin/config/local）不可删除')
  })
})

describe('dbmErrorText（DBM_029 A-DB13 闭环，Gate 2 E1 修复）', () => {
  it('DBM_029 引擎操作被拒：基础文案（不再返回 null 静默吞错）', () => {
    expect(dbmErrorText('DBM_029', null)).toBe('数据库拒绝操作：当前账号权限不足')
  })

  it('DBM_029 data.reason 附加（引擎原文如实，如 Unauthorized code 13）', () => {
    const reason = 'dbmongo: Unauthorized (code 13): not authorized on admin to execute createUser'
    expect(dbmErrorText('DBM_029', { reason })).toBe(
      `数据库拒绝操作：当前账号权限不足：${reason}`
    )
    expect(dbmErrorText('DBM_029', {})).toBe('数据库拒绝操作：当前账号权限不足')
  })

  it('DBM_022 语义锚点：仍为 TABLE_NOT_FOUND（旧引擎权限语义已迁 DBM_029，不复用）', () => {
    expect(dbmErrorText('DBM_022', null)).toBe('目标表不存在')
    expect(dbmErrorText('DBM_022', null)).not.toContain('权限不足')
  })

  it('DBM_001~029 全覆盖锚点：逐码 dbmErrorText 非 null（防缺码静默吞错回归）', () => {
    for (let i = 1; i <= 29; i++) {
      const code = `DBM_${String(i).padStart(3, '0')}`
      expect(DBM_ERROR_TEXT[code], code).toBeDefined()
      expect(dbmErrorText(code, null), code).not.toBeNull()
    }
    expect(Object.keys(DBM_ERROR_TEXT)).toHaveLength(29)
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

describe('clampBrowsePageSize（v1.4 分页收窄，TABLE/DOC_PAGE_LIMIT=100）', () => {
  it('缺省/非法 → 默认 20', () => {
    expect(clampBrowsePageSize(undefined)).toBe(20)
    expect(clampBrowsePageSize(null)).toBe(20)
    expect(clampBrowsePageSize(0)).toBe(20)
    expect(clampBrowsePageSize(-5)).toBe(20)
    expect(clampBrowsePageSize(Number.NaN)).toBe(20)
  })

  it('合法值原样；超限截断至 100；小数取整', () => {
    expect(clampBrowsePageSize(50)).toBe(50)
    expect(clampBrowsePageSize(100)).toBe(100)
    expect(clampBrowsePageSize(500)).toBe(100)
    expect(clampBrowsePageSize(20.9)).toBe(20)
  })
})

describe('validateCollectionName（v1.4 集合名白名单，R-14/api 4.8.4）', () => {
  it('合法名通过', () => {
    expect(validateCollectionName('orders')).toBeNull()
    expect(validateCollectionName('order_items-2')).toBeNull()
  })

  it('空/$ 前缀/空白与空字符拒绝', () => {
    expect(validateCollectionName('')).not.toBeNull()
    expect(validateCollectionName('   ')).not.toBeNull()
    expect(validateCollectionName('$reserved')).not.toBeNull()
    expect(validateCollectionName('has space')).not.toBeNull()
    expect(validateCollectionName('has\0null')).not.toBeNull()
  })
})

describe('isMongoSystemDatabase（v1.4 I-12/D-25 系统库保护前端镜像）', () => {
  it('admin/config/local 命中；业务库不命中', () => {
    expect(isMongoSystemDatabase('admin')).toBe(true)
    expect(isMongoSystemDatabase('config')).toBe(true)
    expect(isMongoSystemDatabase('local')).toBe(true)
    expect(isMongoSystemDatabase('app_prod')).toBe(false)
    expect(isMongoSystemDatabase('Admin')).toBe(false) // 大小写敏感（引擎库名语义）
  })
})

describe('rowCellText（v1.4 行值呈现，JSON null≠TS undefined）', () => {
  it('null → NULL 文本；数值/布尔/字符串原样 String', () => {
    expect(rowCellText(null)).toBe('NULL')
    expect(rowCellText(42)).toBe('42')
    expect(rowCellText(true)).toBe('true')
    expect(rowCellText('')).toBe('')
    expect(rowCellText('含\n换行')).toBe('含\n换行')
  })
})

describe('parseDocumentJson（v1.4 JSON 双层校验前端拦截层，D-23）', () => {
  it('合法 JSON 对象 → ok + 解析结果（嵌套/扩展 JSON 形态透传）', () => {
    const r = parseDocumentJson('{"name":"a","meta":{"$oid":"64f000000000000000000001"},"n":null}')
    expect(r.ok).toBe(true)
    if (r.ok) {
      expect(r.doc.name).toBe('a')
      expect(r.doc.n).toBeNull()
    }
  })

  it('非法 JSON → 内联错误文案（不提交）', () => {
    const r = parseDocumentJson('{bad json}')
    expect(r.ok).toBe(false)
    if (!r.ok) expect(r.error).toContain('JSON 解析失败')
  })

  it('数组/标量/null 非文档形态 → 拒绝', () => {
    for (const text of ['[1,2]', '"str"', '42', 'null']) {
      const r = parseDocumentJson(text)
      expect(r.ok).toBe(false)
      if (!r.ok) expect(r.error).toContain('JSON 对象')
    }
  })
})

describe('formatDocId / docWithIdFirst（v1.4 _id 呈现与置顶）', () => {
  it('ObjectId 扩展 JSON 缩写；字符串/数值 _id JSON 序列化', () => {
    expect(formatDocId({ $oid: '64f000000000000000000001' })).toBe(
      'ObjectId(64f000000000000000000001)'
    )
    expect(formatDocId('abc')).toBe('"abc"')
    expect(formatDocId(7)).toBe('7')
    expect(formatDocId(null)).toBe('null')
  })

  it('_id 置顶其余键序保持；无 _id 原样返回', () => {
    const doc = { b: 2, _id: 'x', a: 1 }
    expect(Object.keys(docWithIdFirst(doc))).toEqual(['_id', 'b', 'a'])
    const noId = { a: 1 }
    expect(docWithIdFirst(noId)).toBe(noId)
  })
})

describe('buildChangedRowValues（v1.4 行编辑变更列收集，5.12）', () => {
  const original: TableRow = { id: 1, name: 'old', note: null, age: 30 }

  it('仅变更列入 values；未改列不提交；主键列排除', () => {
    const values = buildChangedRowValues(
      original,
      {
        id: { text: '2', isNull: false }, // 主键列——排除（定位键不可编辑）
        name: { text: 'new', isNull: false },
        note: { text: '', isNull: true }, // 原 null + 勾选 NULL = 未变更
        age: { text: '30', isNull: false } // 未变更
      },
      'id'
    )
    expect(values).toEqual({ name: 'new' })
  })

  it('NULL 勾选 → null 提交（SET col=NULL）；取消勾选按文本提交', () => {
    const values = buildChangedRowValues(
      original,
      {
        name: { text: '', isNull: true }, // 原非 null → 改 NULL
        note: { text: 'filled', isNull: false } // 原 null → 填值
      },
      'id'
    )
    expect(values).toEqual({ name: null, note: 'filled' })
  })

  it('全部未变更 → 空对象（页面提示不提交）', () => {
    const values = buildChangedRowValues(
      original,
      {
        name: { text: 'old', isNull: false },
        note: { text: '', isNull: true },
        age: { text: '30', isNull: false }
      },
      'id'
    )
    expect(values).toEqual({})
  })
})

describe('rescanToastText（v1.4 F1/D-21 merged 统计）', () => {
  it('merged=0 → 仅新增/更新计数', () => {
    expect(rescanToastText({ discovered: 2, updated: 3, merged: 0 })).toBe(
      '重新探测完成：新增 2 个实例，更新 3 个状态'
    )
  })

  it('merged>0 → 追加清理冗余 Socket 档案统计', () => {
    expect(rescanToastText({ discovered: 0, updated: 1, merged: 2 })).toBe(
      '重新探测完成：新增 0 个实例，更新 1 个状态，清理 2 个冗余 Socket 档案'
    )
  })
})
