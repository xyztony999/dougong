<script setup lang="ts">
/**
 * SessionPane —— 单标签会话面板：AppTerminal 宿主 + closed 帧断开反馈（PageSpec 第 6 节）。
 * 数据/通道归 terminalStore；本组件只做呈现与事件转发（output 直写、input/resize 上抛）。
 * closed 标签保留可查看尾部输出（4.3：关闭为纯前端移除、重连=重走新建流程不继承状态）。
 */
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import AppTerminal from '@/components/AppTerminal.vue'
import { useTerminalStore } from '@/stores/terminal'
import { TERMINAL_END_REASON, dictItem } from '@/types/enums'
import type { TerminalEndReason } from '@/types/enums'

type AlertType = 'default' | 'info' | 'success' | 'warning' | 'error'

const props = defineProps<{ tabId: string }>()

const store = useTerminalStore()
const router = useRouter()
const term = ref<InstanceType<typeof AppTerminal> | null>(null)

const tab = computed(() => store.tabs.find((t) => t.id === props.tabId) ?? null)
const endInfo = computed(() => {
  const info = tab.value?.closeInfo
  if (tab.value?.status !== 'closed' || info === null || info === undefined) return null
  return { reason: info.reason as TerminalEndReason, ...dictItem(TERMINAL_END_REASON, info.reason) }
})

/** NTag 语义色 → NAlert 类型（TagType.primary 无 Alert 对应型，降级 info） */
const alertType = computed<AlertType>(() => {
  const tag = endInfo.value?.tag
  if (tag === undefined) return 'default'
  return tag === 'primary' ? 'info' : tag
})

/** 断开反馈按钮分级（第 6 节表）：self_close 仅关闭；idle/hard/network/protocol 重连；panel 失效重登 */
const showReconnect = computed(() => {
  const reason = endInfo.value?.reason
  return (
    reason === 'idle_timeout' ||
    reason === 'hard_limit' ||
    reason === 'network_error' ||
    reason === 'protocol_error'
  )
})
const isPanelSessionInvalid = computed(() => endInfo.value?.reason === 'panel_session_invalid')

/** output 帧 + 背压标记直写（D-04：truncated → 终端内黄色提示行） */
function writeOutput(bytes: Uint8Array, truncated: boolean): void {
  if (truncated) {
    term.value?.write('\r\n\u001b[33m⚠ 输出缓冲溢出，部分历史输出已被丢弃\u001b[0m\r\n')
  }
  term.value?.write(bytes)
}

onMounted(() => {
  store.attachTerminal(props.tabId, writeOutput)
  term.value?.fit()
})

onUnmounted(() => {
  store.detachTerminal(props.tabId)
})

// 标签激活时重适配（隐藏容器 fit 无效，AppTerminal 已守卫 0 宽高）
watch(
  () => store.activeTabId,
  async (id) => {
    if (id === props.tabId) {
      await nextTick()
      term.value?.fit()
    }
  }
)

// ready 定稿后首次尺寸同步（PTY 初始 80x24 → 容器实际 cols/rows）
watch(
  () => tab.value?.status,
  async (status, prev) => {
    if (status === 'connected' && prev !== 'connected') {
      await nextTick()
      term.value?.fit()
      const dims = term.value?.dims()
      if (dims !== undefined) store.sendResize(props.tabId, dims.cols, dims.rows)
    }
  }
)

function onInput(data: string): void {
  store.sendInput(props.tabId, data)
}

function onResize(cols: number, rows: number): void {
  store.sendResize(props.tabId, cols, rows)
}

function reconnect(): void {
  store.openConnectDialog()
}

function relogin(): void {
  void router.replace('/login')
}

function closeTab(): void {
  store.removeTab(props.tabId)
}
</script>

<template>
  <div class="session-pane">
    <div class="term-area">
      <AppTerminal ref="term" @input="onInput" @resize="onResize" />
    </div>
    <n-alert v-if="endInfo !== null" :type="alertType" class="close-alert" :bordered="false">
      {{ endInfo.label }}
    </n-alert>
    <div v-if="endInfo !== null" class="close-actions">
      <n-button
        v-if="showReconnect"
        size="small"
        type="primary"
        @click="reconnect"
      >
        重新连接
      </n-button>
      <n-button
        v-else-if="isPanelSessionInvalid"
        size="small"
        type="primary"
        @click="relogin"
      >
        重新登录
      </n-button>
      <n-button v-if="!isPanelSessionInvalid" size="small" quaternary @click="closeTab">
        关闭标签
      </n-button>
    </div>
  </div>
</template>

<style scoped>
.session-pane {
  height: 100%;
  min-height: 0;
  display: flex;
  flex-direction: column;
  background: rgb(18, 21, 28);
  border-radius: 4px;
  overflow: hidden;
}
.term-area {
  flex: 1;
  min-height: 0;
  padding: 8px;
}
.close-alert {
  flex-shrink: 0;
  margin: 0 8px;
}
.close-actions {
  flex-shrink: 0;
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 8px;
}
</style>
