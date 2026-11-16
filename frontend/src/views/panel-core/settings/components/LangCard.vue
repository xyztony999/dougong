<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import type { SelectOption } from 'naive-ui'
import { configUpdate } from '@/api/panel'
import { useAuthStore } from '@/stores/auth'
import { usePanelStore } from '@/stores/panel'
import { CONFIG_EFFECT_MODE, dictItem } from '@/types/enums'
import type { PanelConfigDetail } from '@/types/panel'

const props = defineProps<{ detail: PanelConfigDetail }>()
const message = useMessage()
const authStore = useAuthStore()
const panelStore = usePanelStore()

/** 语言展示名（api I-P08：MVP 仅 zh-CN；PANEL_006 为后端兜底） */
const LANG_LABELS: Readonly<Record<string, string>> = {
  'zh-CN': '简体中文'
}

interface LangFormData {
  lang: string | null
}

const langForm = ref<LangFormData>({ lang: null })

watch(
  () => props.detail,
  (d) => {
    langForm.value = { lang: d.lang.value }
  },
  { immediate: true }
)

const langOptions = computed<SelectOption[]>(() =>
  props.detail.lang.supported.map((value) => ({ label: LANG_LABELS[value] ?? value, value }))
)

const effectTag = computed(() => dictItem(CONFIG_EFFECT_MODE, props.detail.lang.effectMode))

const saving = ref(false)

async function onSave(): Promise<void> {
  saving.value = true
  try {
    await configUpdate({ lang: langForm.value.lang ?? props.detail.lang.value })
    message.success('已保存')
    await panelStore.loadConfig()
  } catch {
    // PANEL_006 已由拦截器统一提示
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <n-card title="面板参数">
    <template #header-extra>
      <n-tag :type="effectTag.tag" size="small" :bordered="false">{{ effectTag.label }}</n-tag>
    </template>
    <n-form
      :model="langForm"
      label-placement="top"
      :disabled="!authStore.isAdmin"
      style="max-width: 420px"
    >
      <n-form-item label="界面语言" path="lang">
        <n-select v-model:value="langForm.lang" :options="langOptions" />
      </n-form-item>
    </n-form>
    <div v-if="authStore.isAdmin" class="actions">
      <n-button type="primary" :loading="saving" @click="onSave">保存</n-button>
    </div>
  </n-card>
</template>

<style scoped>
.actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
</style>
