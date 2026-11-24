<script setup lang="ts">
import { onMounted, watch } from 'vue'
import { useDialog, useMessage } from 'naive-ui'
import { useNavigationStore } from '@/stores/navigation'
import { NAV_TEXT, consumeReloadedFlag, createBrowserStorage, resolveNavFeedback } from '@/router/navFailure'

/**
 * 页面切换失败的提示（框架级，没有界面；PageSpec panel-core 第 7 节 v1.3）。
 *
 * 失败由 navigationStore 记录、种类在 router/navFailure.ts 判定，这里只负责把结论变成
 * 用户看得到的东西：网络原因 / 其它错误 → 消息条；面板升级后自动重新加载没成功 →
 * 带「刷新页面」按钮的提示框；自动重新加载成功 → 一句「已重新打开」。
 * 必须放在 n-message-provider 与 n-dialog-provider 里面（见 App.vue）。
 */
const nav = useNavigationStore()
const message = useMessage()
const dialog = useDialog()

watch(
  () => nav.failureSeq,
  () => {
    const current = nav.failure
    if (current === null) return
    const feedback = resolveNavFeedback(current)
    if (feedback.kind === 'dialog') {
      dialog.error({
        title: feedback.title,
        content: feedback.content,
        positiveText: feedback.positiveText,
        negativeText: feedback.negativeText,
        onPositiveClick: () => {
          window.location.reload()
        }
      })
    } else if (feedback.level === 'info') {
      message.info(feedback.text)
    } else {
      // 多留几秒：慢链路上用户可能正在看网络，默认 3 秒不够
      message.error(feedback.text, { duration: 6000 })
    }
  }
)

onMounted(() => {
  // 自动重新加载后补一句说明。标记是重新加载前写下的，60 秒内才认，避免旧标记误报。
  if (consumeReloadedFlag(createBrowserStorage(), Date.now())) message.info(NAV_TEXT.reloaded)
})
</script>

<template>
  <!-- 没有界面：只把切换失败转成提示 -->
</template>
