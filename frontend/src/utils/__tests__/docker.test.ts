/**
 * docker 类型层纯函数测试（types/docker.ts——帧收窄/错误 data 收窄/文案映射；
 * terminal 先例：types 层解析器测试落 utils/__tests__）。
 */
import { describe, expect, it } from 'vitest'
import {
  daemonDetailText,
  dockerErrorReason,
  isActionMismatchInfo,
  isComposeLocateFailedInfo,
  parseDockerLogFrame
} from '@/types/docker'
import { DK_ERROR_TEXT } from '@/types/docker'
import { isDockerLogEndReason } from '@/types/enums'

describe('parseDockerLogFrame（D-03 帧协议收窄）', () => {
  it('log 帧合法输入 → 收窄为 log 帧', () => {
    expect(parseDockerLogFrame('{"type":"log","line":"hello"}')).toEqual({
      type: 'log',
      line: 'hello'
    })
  })

  it('五值 reason 的 end 帧 → 收窄为 end 帧', () => {
    for (const reason of [
      'container_removed',
      'container_stopped',
      'project_gone',
      'locate_failed',
      'daemon_lost'
    ]) {
      expect(parseDockerLogFrame(`{"type":"end","reason":"${reason}"}`)).toEqual({
        type: 'end',
        reason
      })
    }
  })

  it('未知 reason / 残缺字段 / 坏 JSON / 非对象 → null（协议违规由调用方收口）', () => {
    expect(parseDockerLogFrame('{"type":"end","reason":"whatever"}')).toBeNull()
    expect(parseDockerLogFrame('{"type":"log"}')).toBeNull()
    expect(parseDockerLogFrame('{"type":"log","line":123}')).toBeNull()
    expect(parseDockerLogFrame('not json')).toBeNull()
    expect(parseDockerLogFrame('[]')).toBeNull()
    expect(parseDockerLogFrame('{"type":"input","line":"x"}')).toBeNull()
  })
})

describe('isDockerLogEndReason', () => {
  it('五值命中、其余不命中', () => {
    expect(isDockerLogEndReason('daemon_lost')).toBe(true)
    expect(isDockerLogEndReason('self_close')).toBe(false)
    expect(isDockerLogEndReason(42)).toBe(false)
  })
})

describe('isActionMismatchInfo（DK_004 data）', () => {
  it('合法 {currentState, action, allowedActions} → true', () => {
    expect(
      isActionMismatchInfo({ currentState: 'running', action: 'start', allowedActions: ['stop'] })
    ).toBe(true)
  })
  it('缺字段/类型不符/非对象 → false', () => {
    expect(isActionMismatchInfo({ currentState: 'running' })).toBe(false)
    expect(isActionMismatchInfo({ currentState: 1, action: 'x', allowedActions: [] })).toBe(false)
    expect(isActionMismatchInfo('running')).toBe(false)
  })
})

describe('isComposeLocateFailedInfo（DK_015 data）', () => {
  it('label 路径可空字段收窄', () => {
    expect(
      isComposeLocateFailedInfo({ workingDir: '/srv/app', configFile: null, missing: 'config' })
    ).toBe(true)
    expect(isComposeLocateFailedInfo({ workingDir: 1, configFile: null, missing: '' })).toBe(false)
  })
})

describe('dockerErrorReason（DK_006/008/010/013/016/017 data.reason 透传）', () => {
  it('字符串 reason 提取；空串/缺失/非对象 → null（不虚构）', () => {
    expect(dockerErrorReason({ reason: 'image is being used' })).toBe('image is being used')
    expect(dockerErrorReason({ reason: '' })).toBeNull()
    expect(dockerErrorReason({})).toBeNull()
    expect(dockerErrorReason('oops')).toBeNull()
  })
})

describe('daemonDetailText（4.0 detail 透传属性→文案）', () => {
  it('not_installed/socket_unreachable → 固定文案；其余/null → 原样/空串', () => {
    expect(daemonDetailText('not_installed')).toBe('未检测到已安装的 Docker')
    expect(daemonDetailText('socket_unreachable')).toBe('daemon socket 不可达')
    expect(daemonDetailText('other_probe_result')).toBe('other_probe_result')
    expect(daemonDetailText(null)).toBe('')
  })
})

describe('DK_ERROR_TEXT（DK_001~032：001~018 对应 api 3.4/PageSpec 第 1 节；019~027 创建类；028~032 面板应用与 Compose 生命周期）', () => {
  it('32 码全覆盖', () => {
    const codes = Array.from({ length: 32 }, (_, i) => `DK_${String(i + 1).padStart(3, '0')}`)
    for (const code of codes) {
      expect(DK_ERROR_TEXT[code], code).toBeTruthy()
    }
    expect(Object.keys(DK_ERROR_TEXT)).toHaveLength(32)
  })
})
