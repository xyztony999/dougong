/**
 * 表达式校验预览（PageSpec 5.1/5.3 校验预览区；api D-03/D2 口径-2 单一表达式语义）：
 * 任务 Dialog 与备份任务 Drawer 调度字段共用同一 jobValidate Action（机制型双消费）。
 * 输入防抖 500ms → jobValidate；态机 idle → checking → valid | invalid；
 * valid 前禁止保存（预览为提交前置，PageSpec 5.1）。
 * 防抖定时器 onUnmounted 配对清理（frontend-rules §4.5）。
 */
import { onUnmounted, ref, watch } from 'vue'
import type { Ref } from 'vue'
import { jobValidate } from '@/api/cron'
import { ApiError } from '@/api/http'
import { CB_ERROR_TEXT, cronValidateErrors } from '@/types/cron'

export type ExprValidateState = 'idle' | 'checking' | 'valid' | 'invalid'

const DEBOUNCE_MS = 500

export function useExprValidate(exprRef: Ref<string>) {
  const state = ref<ExprValidateState>('idle')
  /** 合法时的未来 5 次执行时间（ISO-8601，PREVIEW_COUNT=5） */
  const nextFires = ref<string[]>([])
  /** 非法时的段位级错误文案（CB_002 data.errors[] 拼接） */
  const errorText = ref<string | null>(null)
  let timer: ReturnType<typeof setTimeout> | null = null

  function clearTimer(): void {
    if (timer !== null) {
      clearTimeout(timer)
      timer = null
    }
  }

  /** 表单打开/复位：清态与定时器（由消费组件在 watch(show) 中调用） */
  function reset(): void {
    clearTimer()
    state.value = 'idle'
    nextFires.value = []
    errorText.value = null
  }

  async function run(): Promise<void> {
    const expr = exprRef.value.trim()
    if (expr === '') {
      reset()
      return
    }
    state.value = 'checking'
    errorText.value = null
    try {
      const data = await jobValidate({ expr })
      // 迟到守卫：响应到达时输入已再变（下一次 run 覆盖，此处以最新输入判定）
      if (exprRef.value.trim() !== expr) return
      nextFires.value = data.nextFires
      state.value = 'valid'
    } catch (e) {
      if (exprRef.value.trim() !== expr) return
      nextFires.value = []
      if (e instanceof ApiError && e.is('CB_002')) {
        const errors = cronValidateErrors(e.data)
        errorText.value =
          errors.length > 0 ? errors.join('；') : CB_ERROR_TEXT['CB_002']
      } else {
        // 网络/会话级错误已由拦截器 toast；占位文案避免「校验中」悬停（下次输入自动重试）
        errorText.value = '校验服务暂不可用，请稍后重试'
      }
      state.value = 'invalid'
    }
  }

  watch(exprRef, () => {
    clearTimer()
    timer = setTimeout(() => {
      void run()
    }, DEBOUNCE_MS)
  })

  onUnmounted(clearTimer)

  return { state, nextFires, errorText, reset }
}
