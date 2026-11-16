<script setup lang="ts">
import { computed } from 'vue'
import { darkTheme, dateZhCN, zhCN } from 'naive-ui'
import type { GlobalTheme } from 'naive-ui'

// 暗色为一等公民（frontend-03 §1）：默认跟随系统 prefers-color-scheme
const prefersDark =
  typeof window !== 'undefined' && window.matchMedia('(prefers-color-scheme: dark)').matches

const theme = computed<GlobalTheme | null>(() => (prefersDark ? darkTheme : null))
</script>

<template>
  <n-config-provider :theme="theme" :locale="zhCN" :date-locale="dateZhCN">
    <n-message-provider>
      <n-dialog-provider>
        <router-view />
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>
