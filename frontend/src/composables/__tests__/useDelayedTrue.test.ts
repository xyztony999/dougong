/**
 * 「延迟变真」：页面切换超过 200ms 才显示加载提示所依赖的规则
 * （PageSpec panel-core 第 7 节 v1.3：缓存命中的即时切换不该闪一下）。
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { computed, effectScope, nextTick, ref } from 'vue'
import type { EffectScope } from 'vue'
import { useDelayedTrue } from '@/composables/useDelayedTrue'

const DELAY = 200

describe('useDelayedTrue', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  function setup(initial: boolean): { source: ReturnType<typeof ref<boolean>>; delayed: ReturnType<typeof ref<boolean>>; scope: EffectScope } {
    const scope = effectScope()
    const source = ref(initial)
    const delayed = scope.run(() => useDelayedTrue(computed(() => source.value), DELAY))
    return { source, delayed: delayed as ReturnType<typeof ref<boolean>>, scope }
  }

  it('一开始就是真：也要等满延迟才变真', async () => {
    const t = setup(true)
    expect(t.delayed.value).toBe(false)
    await vi.advanceTimersByTimeAsync(DELAY - 1)
    expect(t.delayed.value).toBe(false)
    await vi.advanceTimersByTimeAsync(1)
    expect(t.delayed.value).toBe(true)
    t.scope.stop()
  })

  it('200ms 内完成的切换：始终不显示（不闪）', async () => {
    const t = setup(false)
    t.source.value = true
    await nextTick()
    await vi.advanceTimersByTimeAsync(80)
    t.source.value = false
    await nextTick()
    await vi.advanceTimersByTimeAsync(DELAY * 2)
    expect(t.delayed.value).toBe(false)
    t.scope.stop()
  })

  it('超过 200ms 的切换：延迟到了才显示，切换完成后立刻消失', async () => {
    const t = setup(false)
    t.source.value = true
    await nextTick()
    await vi.advanceTimersByTimeAsync(DELAY)
    expect(t.delayed.value).toBe(true)
    t.source.value = false
    await nextTick()
    expect(t.delayed.value).toBe(false) // 不需要再等 200ms
    t.scope.stop()
  })

  it('连点：以最后一次为准，等待时间从最后一次点击重新算', async () => {
    const t = setup(false)
    t.source.value = true
    await nextTick()
    await vi.advanceTimersByTimeAsync(150)
    t.source.value = false
    t.source.value = true
    await nextTick()
    await vi.advanceTimersByTimeAsync(100) // 距第一次 250ms，距最后一次 100ms
    expect(t.delayed.value).toBe(false)
    await vi.advanceTimersByTimeAsync(100)
    expect(t.delayed.value).toBe(true)
    t.scope.stop()
  })

  it('离开作用域时清掉定时器（不残留、不在卸载后触发）', async () => {
    const t = setup(false)
    t.source.value = true
    await nextTick()
    expect(vi.getTimerCount()).toBe(1)
    t.scope.stop()
    expect(vi.getTimerCount()).toBe(0)
    await vi.advanceTimersByTimeAsync(DELAY * 2)
    expect(t.delayed.value).toBe(false)
  })

  it('延迟已到、没有定时器时卸载：不出错', async () => {
    const t = setup(true)
    await vi.advanceTimersByTimeAsync(DELAY)
    expect(vi.getTimerCount()).toBe(0)
    expect(() => t.scope.stop()).not.toThrow()
  })
})
