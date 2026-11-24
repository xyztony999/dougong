<script setup lang="ts">
import { computed, ref } from 'vue'
import { NGlobalStyle, darkTheme, dateZhCN, zhCN } from 'naive-ui'
import type { GlobalTheme, GlobalThemeOverrides } from 'naive-ui'
import AppNavFeedback from '@/components/AppNavFeedback.vue'

// 暗色为一等公民（frontend-03 §1）：跟随系统 prefers-color-scheme，运行期系统切换即时生效
// （首启 UI 缺陷修复轮：原一次性读取 .matches 不响应系统切换）
const darkQuery =
  typeof window !== 'undefined' && typeof window.matchMedia === 'function'
    ? window.matchMedia('(prefers-color-scheme: dark)')
    : null
const prefersDark = ref(darkQuery?.matches === true)
darkQuery?.addEventListener('change', (event) => {
  prefersDark.value = event.matches
})

const theme = computed<GlobalTheme | null>(() => (prefersDark.value ? darkTheme : null))

// 暗色对比度微调（首启 UI 缺陷修复轮）：placeholder/输入框图标默认 overlay 白 0.38 偏暗
const themeOverrides = computed<GlobalThemeOverrides>(() =>
  prefersDark.value
    ? {
        common: {
          placeholderColor: 'rgba(255, 255, 255, 0.55)',
          iconColor: 'rgba(255, 255, 255, 0.6)'
        }
      }
    : {}
)
</script>

<template>
  <n-config-provider
    :theme="theme"
    :theme-overrides="themeOverrides"
    :locale="zhCN"
    :date-locale="dateZhCN"
  >
    <!-- 全局样式同步：body 背景/文字色随主题（否则独立页 body 保持浏览器白色，出现深色卡片嵌白页） -->
    <NGlobalStyle />
    <n-message-provider>
      <n-dialog-provider>
        <!-- 页面切换失败的提示（消息条 / 提示框）：要能用到上面两个 provider，所以放在它们里面 -->
        <AppNavFeedback />
        <router-view />
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>
