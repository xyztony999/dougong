<script setup lang="ts">
import type { PanelConfigDetail } from '@/types/panel'
import { formatUptime } from '@/utils/format'

// 只读透传区（PRD FR-14 / api D-P08）：POST session 字段被服务端拒绝，本 Card 无保存按钮。
// 值=秒数整数（契约 §4.1.4 钉死 int64，2026-11-16 W1）→ formatUptime 格式化为人类可读时长。
defineProps<{ detail: PanelConfigDetail }>()
</script>

<template>
  <n-card title="会话参数">
    <n-descriptions label-placement="left" :column="1" bordered size="small" style="max-width: 560px">
      <n-descriptions-item label="绝对过期时长">{{ formatUptime(detail.session.absoluteTimeout) }}</n-descriptions-item>
      <n-descriptions-item label="空闲超时时长">{{ formatUptime(detail.session.idleTimeout) }}</n-descriptions-item>
    </n-descriptions>
    <p class="hint">
      该参数由账户与安全基座（account-auth）管理：修改配置文件后重启生效；本页仅展示。
    </p>
  </n-card>
</template>

<style scoped>
.hint {
  margin: 8px 0 0;
  font-size: 12px;
  opacity: 0.65;
}
</style>
