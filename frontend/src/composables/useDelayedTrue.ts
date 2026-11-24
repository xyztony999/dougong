/**
 * 来源变真后，延迟 delayMs 才变真；这段时间里来源若变回假则取消。
 *
 * 用于「页面切换超过 200ms 才显示加载提示」：缓存命中的切换几十毫秒就完成，提示不该
 * 闪一下；真正在等分包的切换才需要让人看见。必须在组件 setup 或 effect scope 内调用
 * （离开作用域时清定时器）。
 */
import { onScopeDispose, ref, watch } from 'vue'
import type { Ref } from 'vue'

/** 可读的布尔来源：ref、computed 都满足。 */
export type ReadableBoolean = { readonly value: boolean }

export function useDelayedTrue(source: ReadableBoolean, delayMs: number): Ref<boolean> {
  const delayed = ref(false)
  let timer: ReturnType<typeof setTimeout> | null = null

  const clear = (): void => {
    if (timer !== null) {
      clearTimeout(timer)
      timer = null
    }
  }

  // 同步触发：来源在同一拍里先假后真（上一次切换刚完成、下一次紧接着开始）也要重新计时
  watch(
    () => source.value,
    (on) => {
      clear()
      delayed.value = false
      if (on) {
        timer = setTimeout(() => {
          timer = null
          delayed.value = true
        }, delayMs)
      }
    },
    { immediate: true, flush: 'sync' }
  )

  onScopeDispose(clear)
  return delayed
}
