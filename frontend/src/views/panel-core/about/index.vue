<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { versionGet } from '@/api/panel'
import type { PanelVersionData } from '@/types/panel'
import { formatDateTime } from '@/utils/format'

/** 进入时 versionGet 一次（api D-P11：需会话；无操作按钮、无检查更新入口——该动作在设置页 Card 6） */
const version = ref<PanelVersionData | null>(null)
const loading = ref(false)
const failed = ref(false)

async function load(): Promise<void> {
  loading.value = true
  failed.value = false
  try {
    version.value = await versionGet()
  } catch {
    failed.value = true
    // 错误已由拦截器统一提示
  } finally {
    loading.value = false
  }
}

function retry(): void {
  void load()
}

/** gitCommit 截短 8 位（PageSpec 5.2） */
const shortCommit = computed(() => {
  const c = version.value?.gitCommit ?? ''
  return c === '' ? '—' : c.slice(0, 8)
})

onMounted(() => {
  void load()
})
</script>

<template>
  <div class="page">
    <n-card title="版本信息" :loading="loading">
      <template v-if="version !== null">
        <div class="version">{{ version.version }}</div>
        <n-descriptions label-placement="left" :column="1" bordered size="small" class="desc">
          <n-descriptions-item label="Git 提交">{{ shortCommit }}</n-descriptions-item>
          <n-descriptions-item label="构建时间">{{ formatDateTime(version.buildTime) }}</n-descriptions-item>
        </n-descriptions>
      </template>
      <n-result
        v-else-if="failed"
        status="error"
        title="版本信息加载失败"
        description="请检查面板服务是否可达后重试。"
      >
        <template #footer>
          <n-button type="primary" @click="retry">重试</n-button>
        </template>
      </n-result>

      <div class="identity">
        <div class="slogan">斗拱 Dougong · 嵌入你的服务器，而不是重装它</div>
        <n-tag size="small" :bordered="false">GPL-3.0</n-tag>
      </div>
    </n-card>
  </div>
</template>

<style scoped>
.page {
  max-width: 560px;
}
.version {
  font-size: 28px;
  font-weight: 600;
  margin-bottom: 16px;
}
.desc {
  margin-bottom: 20px;
}
.identity {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.slogan {
  font-size: 13px;
  opacity: 0.75;
}
</style>
