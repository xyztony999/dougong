<script setup lang="ts">
import { computed, ref } from 'vue'
import { useMessage } from 'naive-ui'
import { configUpdate, updateCheckRun } from '@/api/panel'
import { useAuthStore } from '@/stores/auth'
import { usePanelStore } from '@/stores/panel'
import { CONFIG_EFFECT_MODE, dictItem } from '@/types/enums'
import type { PanelConfigDetail, UpdateCheckResultData } from '@/types/panel'
import { formatDateTime } from '@/utils/format'

const props = defineProps<{ detail: PanelConfigDetail }>()
const message = useMessage()
const authStore = useAuthStore()
const panelStore = usePanelStore()

const enabled = computed(() => props.detail.updateCheck.enabled)
const effectTag = computed(() => dictItem(CONFIG_EFFECT_MODE, props.detail.updateCheck.effectMode))

/** Popconfirm 文案随当前开关态切换（开启→关闭确认 / 关闭→开启确认） */
const switchTip = computed(() =>
  enabled.value ? '关闭后完全禁止外呼' : '开启后可手动检查新版本，面板不会自动外呼'
)

/** 开关变更（admin + NPopconfirm 确认；readonly 渲染禁用开关，状态可见） */
async function onToggle(): Promise<void> {
  try {
    await configUpdate({ updateCheck: !enabled.value })
    message.success('开关已保存，变更已记入审计')
    result.value = null
    await panelStore.loadConfig()
  } catch {
    // 拦截器已提示
  }
}

/** 手动检查（I-P05 双重门控：开关开启 + admin 显式点击；单次外呼，无自动重试 D-P09） */
const checking = ref(false)
const result = ref<UpdateCheckResultData | null>(null)

async function onCheck(): Promise<void> {
  checking.value = true
  try {
    result.value = await updateCheckRun()
  } catch {
    // PANEL_004/005 已由拦截器统一提示；不自动重试
    result.value = null
  } finally {
    checking.value = false
  }
}

const checkedAtText = computed(() =>
  result.value === null ? '' : formatDateTime(result.value.checkedAt)
)
</script>

<template>
  <n-card title="更新检查">
    <template #header-extra>
      <n-tag :type="effectTag.tag" size="small" :bordered="false">{{ effectTag.label }}</n-tag>
    </template>

    <div class="switch-row">
      <span>启用更新检查</span>
      <n-popconfirm v-if="authStore.isAdmin" @positive-click="onToggle">
        {{ switchTip }}
        <template #trigger>
          <n-switch :value="enabled" />
        </template>
      </n-popconfirm>
      <n-switch v-else :value="enabled" disabled />
    </div>
    <p class="hint">默认关闭；开启后仅在手动点击「检查更新」时发起一次外呼，面板自身不会自动外呼。</p>

    <!-- 仅 admin 且开关开启时可用（api 权限矩阵 + I-P05 双重门控）；readonly 无此按钮（负空间） -->
    <div v-if="authStore.isAdmin" class="actions">
      <n-button :disabled="!enabled" :loading="checking" @click="onCheck">检查更新</n-button>
    </div>

    <!-- 内联结果区（过程数据不落库/不落本地存储，PageSpec 第 8 节 #4） -->
    <div v-if="result !== null" class="result">
      <n-tag v-if="result.updateAvailable" type="info" :bordered="false">
        有新版本：{{ result.latestVersion }}
      </n-tag>
      <n-tag v-else type="success" :bordered="false">已是最新</n-tag>
      <span class="checked-at">检查于 {{ checkedAtText }}</span>
    </div>
  </n-card>
</template>

<style scoped>
.switch-row {
  display: flex;
  align-items: center;
  gap: 16px;
}
.hint {
  margin: 8px 0 0;
  font-size: 12px;
  opacity: 0.65;
}
.actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  margin-top: 12px;
}
.result {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 12px;
}
.checked-at {
  font-size: 12px;
  opacity: 0.65;
}
</style>
