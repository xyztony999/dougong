/**
 * env-mgmt 字典 S8 锚点（枚举池 04-business-enum.md 同值）+ 安装后配置就绪
 * bootstrap_status 独立维度（v1.2，api-env-mgmt v1.2 D-18）。
 */
import { describe, expect, it } from 'vitest'
import {
  BOOTSTRAP_STATUS,
  ENV_TASK_ACTION,
  ENV_TASK_STATUS,
  ENV_TYPE,
  PACKAGE_MANAGER_FAMILY
} from '../enums'

describe('env 字典 S8 锚点（枚举池同值）', () => {
  it('ENV_TYPE 八值含 mongodb/node（v1.1 +mongodb；v1.3 +node，db-tools 轮 W2 同步）', () => {
    expect(Object.keys(ENV_TYPE)).toEqual([
      'nginx',
      'apache',
      'docker',
      'mysql',
      'redis',
      'php_fpm',
      'mongodb',
      'node'
    ])
    expect(ENV_TYPE.mongodb.label).toBe('MongoDB')
    expect(ENV_TYPE.node.label).toBe('Node.js')
  })

  it('ENV_TASK_ACTION 三动作', () => {
    expect(Object.keys(ENV_TASK_ACTION)).toEqual(['install', 'upgrade', 'uninstall'])
  })

  it('ENV_TASK_STATUS 四态机', () => {
    expect(Object.keys(ENV_TASK_STATUS)).toEqual(['pending', 'running', 'succeeded', 'failed'])
    expect(ENV_TASK_STATUS.succeeded.tag).toBe('success')
    expect(ENV_TASK_STATUS.failed.tag).toBe('error')
  })

  it('PACKAGE_MANAGER_FAMILY 双系三值', () => {
    expect(Object.keys(PACKAGE_MANAGER_FAMILY)).toEqual(['apt', 'dnf', 'yum'])
  })

  it('BOOTSTRAP_STATUS 三值（v1.2 安装后配置就绪，D-18）', () => {
    expect(Object.keys(BOOTSTRAP_STATUS)).toEqual(['succeeded', 'failed', 'skipped'])
    // PageSpec 第 3 节颜色方案：succeeded green / failed orange（包已装故非红）/ skipped gray。
    expect(BOOTSTRAP_STATUS.succeeded.tag).toBe('success')
    expect(BOOTSTRAP_STATUS.failed.tag).toBe('warning')
    expect(BOOTSTRAP_STATUS.skipped.tag).toBe('default')
    expect(BOOTSTRAP_STATUS.succeeded.label).toBe('已就绪')
    expect(BOOTSTRAP_STATUS.failed.label).toBe('引导失败')
  })

  it('bootstrap_status 与 env_task_status 独立（同名 succeeded/failed 语义不同）', () => {
    // 引导 failed=warning（包已装），任务 failed=error（包操作失败）——独立维度不混淆。
    expect(BOOTSTRAP_STATUS.failed.tag).not.toBe(ENV_TASK_STATUS.failed.tag)
  })
})