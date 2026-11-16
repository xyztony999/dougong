<script setup lang="ts">
/**
 * SITE_003 回滚 diff 弹窗（PageSpec 4.1——全部配置写接口共用，D-04 语义）：
 * 「保存失败，nginx -t 校验未通过，配置已自动回滚到修改前」+ errors[] 错误行列表 +
 * diff 差异等宽对比块 + 按钮「知道了」（修正内容后重发；回滚非幂等重放，api 4.2.4）。
 */
import { computed } from 'vue'
import type { SiteRollbackData } from '@/types/site'

const props = defineProps<{
  /** SITE_003 data（null=关闭） */
  data: SiteRollbackData | null
}>()

const emit = defineEmits<{
  close: []
}>()

const show = computed<boolean>({
  get: () => props.data !== null,
  set: (v) => {
    if (!v) emit('close')
  }
})
</script>

<template>
  <n-modal v-model:show="show" preset="card" title="保存失败：nginx -t 校验未通过" style="width: 620px">
    <div v-if="data !== null" class="body">
      <n-alert type="warning" :show-icon="true">
        配置已自动回滚到修改前（R-06 改不坏承诺）。请依据以下错误修正内容后重新提交——回滚不是幂等重放。
      </n-alert>
      <div class="section-title">nginx 校验错误行</div>
      <ul class="error-list">
        <li v-for="(line, i) in data.errors" :key="i" class="error-line">{{ line }}</li>
        <li v-if="data.errors.length === 0" class="error-line muted">（服务端未返回错误行明细）</li>
      </ul>
      <template v-if="data.diff !== undefined && data.diff !== ''">
        <div class="section-title">写回内容与原文件差异</div>
        <pre class="diff-block">{{ data.diff }}</pre>
      </template>
    </div>
    <template #footer>
      <div class="footer">
        <n-button type="primary" @click="emit('close')">知道了</n-button>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.section-title {
  font-size: 13px;
  font-weight: 500;
  margin-top: 4px;
}
.error-list {
  margin: 0;
  padding: 8px 12px 8px 28px;
  border-radius: 4px;
  background-color: rgba(240, 160, 32, 0.08);
  max-height: 160px;
  overflow: auto;
}
.error-line {
  font-family: Consolas, Menlo, monospace;
  font-size: 12px;
  line-height: 1.7;
  word-break: break-all;
}
.muted {
  opacity: 0.6;
}
.diff-block {
  margin: 0;
  padding: 10px 12px;
  border-radius: 4px;
  font-family: Consolas, Menlo, monospace;
  font-size: 12px;
  line-height: 1.6;
  max-height: 240px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
  background-color: rgba(128, 128, 128, 0.1);
}
.footer {
  display: flex;
  justify-content: flex-end;
}
</style>
