<script setup lang="ts">
/**
 * Web 终端页（/terminal，code 型——xterm.js 深度集成，PageSpec 第 4 节）。
 * 三区布局：标签栏（NTabs card closable + 尾部「+」，多标签上限=并发阈值）/
 * 终端主体（每标签一 AppTerminal 实例，display-directive=show 保活、仅激活可见）/
 * 状态条（ready 帧 limits 权威值 + 连接状态 Tag）。
 * admin 专属路由（roles:['admin'] 守卫，readonly 零可达整域禁用——ADR-018，负空间见 readonly PageSpec）。
 * 无轮询（WS 通道即数据流）；页面卸载 disposeAll 全部 WS 尽力 close（PageSpec 第 2 节）。
 */
import { computed, onUnmounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useMessage } from 'naive-ui'
import { useAuthStore } from '@/stores/auth'
import { useTerminalStore } from '@/stores/terminal'
import type { TerminalIdentitySource } from '@/types/enums'
import { terminalErrorText } from '@/types/terminal'
import ConnectModal from './components/ConnectModal.vue'
import SessionPane from './components/SessionPane.vue'

const store = useTerminalStore()
const authStore = useAuthStore()
const message = useMessage()
const router = useRouter()

/** 标签标题：连接中态「连接中…」，定稿后=ready 帧 runUser（PageSpec 4.1） */
function tabLabel(tabId: string): string {
  const tab = store.tabs.find((t) => t.id === tabId)
  if (tab === undefined) return ''
  if (tab.status === 'connecting') return '连接中…'
  return tab.runUser ?? '会话'
}

/** limits 时长展示（1800→30min / 28800→8h；ready 帧权威值，R-14） */
function formatLimit(seconds: number): string {
  if (seconds % 3600 === 0) return `${seconds / 3600}h`
  if (seconds % 60 === 0) return `${seconds / 60}min`
  return `${seconds}s`
}

const activeTab = computed(() => store.tabs.find((t) => t.id === store.activeTabId) ?? null)

const statusText = computed(() => {
  const tab = activeTab.value
  if (tab === null) return ''
  if (tab.status === 'connecting') return '正在建立会话…'
  const parts = [tab.runUser ?? '会话']
  if (tab.sessionId !== null) parts.push(`会话 ${tab.sessionId.slice(0, 8)}`)
  if (tab.limits !== null) {
    parts.push(`空闲上限 ${formatLimit(tab.limits.idleTimeoutSeconds)}`)
    parts.push(`最长 ${formatLimit(tab.limits.hardLimitSeconds)}`)
  }
  return parts.join(' · ')
})

/** 连接状态 Tag：connected=green / closed=gray（PageSpec 4.1；connecting=default 中间态） */
const statusTag = computed<{ label: string; type: 'default' | 'success' }>(() => {
  const status = activeTab.value?.status
  if (status === 'connected') return { label: '已连接', type: 'success' }
  if (status === 'closed') return { label: '已断开', type: 'default' }
  return { label: '连接中', type: 'default' }
})

/** 「+」达上限禁用（活跃标签数=并发阈值；上限取 ready 帧权威值，预置 ADR-023 标准档 5） */
const plusDisabled = computed(() => store.activeCount >= store.concurrentLimit)
const plusTooltip = computed(() => `已达并发上限 ${store.concurrentLimit}，请先关闭其他会话`)

function onTabChange(name: string): void {
  store.activeTabId = name
}

function onTabClose(name: string): void {
  store.removeTab(name)
}

function openNewSession(): void {
  store.openConnectDialog()
}

function onConnect(identitySource: TerminalIdentitySource, username: string): void {
  store.createSession(identitySource, username)
}

/**
 * 握手失败反馈（PageSpec 第 5 节流程表；store 无 Message 上下文 → 页面 watch 分流）：
 * AUTH_001/002 跳登录；TERM_002/003/005 重开对话框+自动重拉候选；TERM_001 warning；
 * TERM_004/006 及未识别 error（连接中标签已由 store 移除，I-02 对称回收）。
 */
watch(
  () => store.handshakeFailure,
  (failure) => {
    if (failure === null) return
    if (failure.code === 'AUTH_001' || failure.code === 'AUTH_002') {
      message.error(failure.code === 'AUTH_002' ? '登录已过期，请重新登录' : '请先登录')
      authStore.reset()
      void router.replace('/login')
      return
    }
    const text = terminalErrorText(failure.code, failure.data)
    if (
      failure.code === 'TERM_002' ||
      failure.code === 'TERM_003' ||
      failure.code === 'TERM_005'
    ) {
      message.warning(text)
      void store.loadUserOptions() // 实况可能变化，自动重拉（第 5 节）
      store.openConnectDialog() // 保留对话框上下文重开
      return
    }
    if (failure.code === 'TERM_001') {
      message.warning(text)
      return
    }
    message.error(text)
  }
)

// 页面卸载/路由离开：全部活跃 WS 尽力 close（PageSpec 第 2 节；孤儿由服务端兜底回收）
onUnmounted(() => {
  store.disposeAll()
})
</script>

<template>
  <div class="terminal-page">
    <n-tabs
      v-if="store.tabs.length > 0"
      class="terminal-tabs"
      type="card"
      size="small"
      :value="store.activeTabId ?? undefined"
      @update:value="onTabChange"
      @close="onTabClose"
    >
      <n-tab-pane
        v-for="tab in store.tabs"
        :key="tab.id"
        :name="tab.id"
        :tab="tabLabel(tab.id)"
        display-directive="show"
        closable
      >
        <SessionPane :tab-id="tab.id" />
      </n-tab-pane>
      <template #suffix>
        <n-tooltip trigger="hover" :disabled="!plusDisabled">
          <template #trigger>
            <n-button
              size="tiny"
              quaternary
              class="new-tab-btn"
              :disabled="plusDisabled"
              @click="openNewSession"
            >
              ＋
            </n-button>
          </template>
          {{ plusTooltip }}
        </n-tooltip>
      </template>
    </n-tabs>
    <div v-else class="empty-area">
      <n-empty description="暂无终端会话">
        <template #extra>
          <n-button size="small" @click="openNewSession">新建会话</n-button>
        </template>
      </n-empty>
    </div>
    <div class="statusbar">
      <span class="status-text">{{ statusText }}</span>
      <n-tag v-if="activeTab !== null" :type="statusTag.type" size="small">
        {{ statusTag.label }}
      </n-tag>
    </div>
    <ConnectModal @connect="onConnect" />
  </div>
</template>

<style scoped>
.terminal-page {
  display: flex;
  flex-direction: column;
  height: calc(100vh - 88px); /* MainLayout 头部 56px + 内容区上下 padding 16px×2 */
  gap: 8px;
}
.terminal-tabs {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
.terminal-tabs :deep(.n-tabs-nav) {
  flex-shrink: 0;
}
.terminal-tabs :deep(.n-tabs-pane-wrapper) {
  flex: 1 1 auto;
  min-height: 0;
}
.terminal-tabs :deep(.n-tabs-pane-scroll-wrapper) {
  height: 100%;
}
.terminal-tabs :deep(.n-tab-pane) {
  height: 100%;
  box-sizing: border-box;
}
.new-tab-btn {
  margin: 0 4px;
}
.empty-area {
  flex: 1;
  min-height: 0;
  display: flex;
  align-items: center;
  justify-content: center;
}
.statusbar {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 24px;
}
.status-text {
  font-size: 12px;
  opacity: 0.65;
}
</style>
