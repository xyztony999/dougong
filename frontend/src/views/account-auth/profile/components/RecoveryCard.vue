<script setup lang="ts">
/**
 * 恢复码管理卡（v0.7.0 登录安全增量）：生成 10 个一次性恢复码（明文仅展示一次）、
 * 查看剩余数量。passkey/TOTP 丢失时用恢复码登录，防锁死。
 */
import { onMounted, ref } from 'vue'
import { useMessage } from 'naive-ui'
import { recoveryCount, recoveryGenerate } from '@/api/auth'
import { copyText } from '@/utils/clipboard'

const message = useMessage()
const remaining = ref<number | null>(null)
const generating = ref(false)
/** 刚生成的明文码（仅本次展示，关闭即不可再看） */
const shownCodes = ref<string[]>([])
const showCodes = ref(false)

async function load(): Promise<void> {
  try {
    const data = await recoveryCount()
    remaining.value = data.remaining
  } catch {
    remaining.value = null
  }
}

async function onGenerate(): Promise<void> {
  generating.value = true
  try {
    const data = await recoveryGenerate()
    shownCodes.value = data.codes
    showCodes.value = true
    await load()
  } catch {
    // 拦截器已提示
  } finally {
    generating.value = false
  }
}

async function onCopy(): Promise<void> {
  const ok = await copyText(shownCodes.value.join('\n'))
  message[ok ? 'success' : 'error'](ok ? '已复制' : '复制失败，请手动保存')
}

onMounted(() => {
  void load()
})
</script>

<template>
  <n-card title="恢复码">
    <div class="recovery-row">
      <template v-if="remaining === null">
        <n-tag size="small" :bordered="false">检测中</n-tag>
      </template>
      <template v-else>
        <n-tag :type="remaining > 0 ? 'success' : 'warning'" size="small" :bordered="false">
          剩余 {{ remaining }} 个
        </n-tag>
      </template>
      <n-popconfirm @positive-click="onGenerate">
        重新生成会使现有未用恢复码全部作废，确认？
        <template #trigger>
          <n-button size="small" :loading="generating">{{ remaining && remaining > 0 ? '重新生成' : '生成恢复码' }}</n-button>
        </template>
      </n-popconfirm>
    </div>
    <p class="hint">passkey 或动态码丢失时，可用一个恢复码登录。每个码只能用一次；请妥善保存，关闭后无法再次查看。</p>

    <n-modal v-model:show="showCodes" preset="card" title="请保存恢复码" style="width: 420px" :mask-closable="false">
      <div class="codes">
        <div v-for="c in shownCodes" :key="c" class="code mono">{{ c }}</div>
      </div>
      <template #footer>
        <div class="actions">
          <n-button @click="onCopy">复制全部</n-button>
          <n-button type="primary" @click="showCodes = false">我已保存</n-button>
        </div>
      </template>
    </n-modal>
  </n-card>
</template>

<style scoped>
.recovery-row {
  display: flex;
  align-items: center;
  gap: 12px;
}
.hint {
  margin: 8px 0 0;
  font-size: 12px;
  opacity: 0.65;
}
.codes {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 6px 16px;
}
.code {
  font-size: 14px;
  letter-spacing: 0.02em;
}
.mono {
  font-family: ui-monospace, SFMono-Regular, Consolas, 'Courier New', monospace;
}
.actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
</style>
