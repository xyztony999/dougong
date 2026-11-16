<script setup lang="ts">
/**
 * 日志 Card（PageSpec 5.1 Card B / 5.2 单日志 Card——两 Drawer 元素完全一致，共用组件）。
 * - 日志区：等宽滚动文本，自动滚底（呈现 GET 尾部行 + WS 跟随追加帧，D-03 双通道同缓冲）；
 * - 行数：tail 档位 200/500/1000/2000（缺省 500——1-2000 域档位化）；变更即重拉尾部；
 *   跟随中禁用（跟随握手自带 tail 回放，避免双通道口径冲突）；
 * - 跟随开关：开启建立 WS 订阅（Cookie 握手同会话），关闭即断开；end 帧/异常断流按
 *   reason 分级提示（container_removed/container_stopped/project_gone/locate_failed→info、
 *   daemon_lost→error，PageSpec 5.1/5.3；无自动重连，重开开关=重试入口）；
 * - 状态面与 WS 句柄在 dockerStore（logDrawer 态），本组件仅呈现与触发。
 */
import { computed, nextTick, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import type { SelectOption } from 'naive-ui'
import { useDockerStore } from '@/stores/docker'
import { DOCKER_LOG_END_REASON } from '@/types/enums'
import { formatDateTime } from '@/utils/format'

const props = defineProps<{
  /** true=Compose 定位失败降级（日志端点 DK_015 同判不可用，控件禁用，PageSpec 5.2） */
  disabled?: boolean
}>()

const store = useDockerStore()
const message = useMessage()

/** tail 档位（api D-03 常量：缺省 500、取值域 1-2000 的档位化） */
const TAIL_OPTIONS: readonly number[] = [200, 500, 1000, 2000]
const tailOptions: SelectOption[] = TAIL_OPTIONS.map((n) => ({ label: `${n} 行`, value: n }))

const linesEl = ref<HTMLElement | null>(null)

/** 新行到达 → 滚底（跟随流场景；无定时器，纯 watch + nextTick） */
watch(
  () => store.logLines.length,
  async () => {
    await nextTick()
    if (linesEl.value !== null) linesEl.value.scrollTop = linesEl.value.scrollHeight
  }
)

/** end 帧分级提示（PageSpec 5.1 reason 行：info 两态 / daemon_lost error） */
watch(
  () => store.followEnded,
  (reason) => {
    if (reason === null) return
    const item = DOCKER_LOG_END_REASON[reason]
    if (item.tag === 'error') message.error(item.label)
    else message.info(item.label)
  }
)

/** 异常断流（握手拒绝/网络断开/协议违规——end 帧未达即关闭） */
watch(
  () => store.followBroken,
  (broken) => {
    if (broken) message.error('日志流连接异常中断，可重新开启跟随重试')
  }
)

const endLineText = computed<string | null>(() => {
  if (store.followEnded !== null) return DOCKER_LOG_END_REASON[store.followEnded].label
  if (store.followBroken) return '日志流已中断（可重新开启跟随重试）'
  return null
})
const endLineError = computed(
  () => store.followEnded === 'daemon_lost' || store.followBroken
)

function onToggleFollow(on: boolean): void {
  if (on) store.startFollow()
  else store.stopFollowByUser()
}

function onTailChange(tail: number): void {
  store.setLogTail(tail)
}
</script>

<template>
  <div class="log-card">
    <div class="log-toolbar">
      <span class="toolbar-label">行数</span>
      <n-select
        :value="store.logTail"
        :options="tailOptions"
        :disabled="props.disabled === true || store.followActive"
        size="small"
        class="tail-select"
        @update:value="onTailChange"
      />
      <span class="toolbar-label">实时跟随</span>
      <n-switch
        :value="store.followActive"
        :disabled="props.disabled === true"
        size="small"
        @update:value="onToggleFollow"
      />
      <span v-if="store.followActive" class="follow-on">跟随中</span>
    </div>
    <div v-if="endLineText !== null" class="follow-end" :class="{ error: endLineError }">
      {{ endLineText }}
    </div>
    <div ref="linesEl" class="log-lines">
      <div v-if="store.logLines.length === 0" class="log-empty">暂无日志</div>
      <div v-for="(line, i) in store.logLines" :key="i" class="log-line">{{ line }}</div>
    </div>
    <div class="meta">数据生成于 {{ formatDateTime(store.logGeneratedAt) }}</div>
  </div>
</template>

<style scoped>
.log-card {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.log-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.toolbar-label {
  font-size: 12px;
  opacity: 0.6;
}
.tail-select {
  width: 110px;
}
.follow-on {
  font-size: 12px;
  color: rgba(32, 128, 240, 1);
}
.follow-end {
  font-size: 12px;
  opacity: 0.75;
}
.follow-end.error {
  color: rgba(208, 48, 80, 1);
}
.log-lines {
  max-height: 320px;
  min-height: 120px;
  overflow: auto;
  border: 1px solid rgba(128, 128, 128, 0.25);
  border-radius: 3px;
  padding: 8px;
  background: rgba(128, 128, 128, 0.06);
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', monospace;
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-all;
}
.log-empty {
  opacity: 0.45;
}
.log-line {
  white-space: pre-wrap;
  word-break: break-all;
}
.meta {
  font-size: 12px;
  opacity: 0.6;
}
</style>
