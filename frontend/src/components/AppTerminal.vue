<script setup lang="ts">
/**
 * AppTerminal —— xterm.js 通用终端宿主（frontend-rules §4.5 / stack-specs frontend-03 §5.2）。
 * 职责：xterm 实例生命周期 + FitAddon 尺寸自适应 addon（PageSpec §1 组件体系）+ ResizeObserver。
 * 不含业务/会话逻辑：连接管理归调用方（stores/terminal），键入/尺寸经 emits 上抛、
 * 输出经 expose 的 write() 直写（PTY 负责回显，前端不本地拼接）。
 * 卸载清理配对（frontend-rules §4.5）：observer.disconnect + term.dispose 于 onUnmounted。
 */
import { FitAddon } from '@xterm/addon-fit'
import { Terminal } from '@xterm/xterm'
import '@xterm/xterm/css/xterm.css'
import { onMounted, onUnmounted, ref } from 'vue'

const emit = defineEmits<{
  /** 键入（UTF-16 字符串；方向键等功能键为转义序列——Base64 编码归上层） */
  input: [data: string]
  /** 尺寸变化（FitAddon 计算后的 cols/rows） */
  resize: [cols: number, rows: number]
}>()

const host = ref<HTMLDivElement | null>(null)
let term: Terminal | null = null
let fitAddon: FitAddon | null = null
let observer: ResizeObserver | null = null

/** 适配容器尺寸（隐藏 pane 宽高为 0 时跳过——n-tabs display-directive=show 隐藏态） */
function fit(): void {
  const el = host.value
  if (el === null || el.clientWidth === 0 || el.clientHeight === 0) return
  try {
    fitAddon?.fit()
  } catch {
    // 尺寸计算异常（如字符测量未就绪）不致命，后续 ResizeObserver 触发重试
  }
}

onMounted(() => {
  const el = host.value
  if (el === null) return
  term = new Terminal({
    // 光标主题/字号：固定默认值（PageSpec 4.1——全局设置面未定义，不虚构）
    fontSize: 14,
    fontFamily: 'Consolas, "Courier New", monospace',
    cursorBlink: true,
    // 缓冲上限防内存膨胀（stack-specs frontend-03 §5.2）
    scrollback: 5000,
    theme: {
      background: 'rgb(18, 21, 28)',
      foreground: 'rgb(214, 218, 226)',
      cursor: 'rgb(150, 190, 255)',
      selectionBackground: 'rgba(96, 132, 196, 0.35)'
    }
  })
  fitAddon = new FitAddon()
  term.loadAddon(fitAddon)
  term.open(el)
  term.onData((data) => {
    emit('input', data)
  })
  term.onResize(({ cols, rows }) => {
    emit('resize', cols, rows)
  })
  observer = new ResizeObserver(() => {
    fit()
  })
  observer.observe(el)
  fit()
})

onUnmounted(() => {
  observer?.disconnect()
  observer = null
  term?.dispose()
  term = null
  fitAddon = null
})

defineExpose({
  /** 输出直写（output 帧 Base64 解码后的字节流；xterm 原生按 UTF-8 解码） */
  write: (data: Uint8Array | string): void => {
    term?.write(data)
  },
  fit,
  dims: (): { cols: number; rows: number } => ({
    cols: term?.cols ?? 80,
    rows: term?.rows ?? 24
  }),
  focus: (): void => {
    term?.focus()
  }
})
</script>

<template>
  <div ref="host" class="app-terminal"></div>
</template>

<style scoped>
.app-terminal {
  height: 100%;
  min-height: 0;
  background: rgb(18, 21, 28);
}
</style>
