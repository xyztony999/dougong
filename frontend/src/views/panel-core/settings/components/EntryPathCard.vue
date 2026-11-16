<script setup lang="ts">
import { computed, ref } from 'vue'
import { useMessage } from 'naive-ui'
import { configUpdate, entryPathRegenerate } from '@/api/panel'
import { useAuthStore } from '@/stores/auth'
import { usePanelStore } from '@/stores/panel'
import { CONFIG_EFFECT_MODE, dictItem } from '@/types/enums'
import type { PanelConfigDetail } from '@/types/panel'
import { copyText } from '@/utils/clipboard'

const props = defineProps<{ detail: PanelConfigDetail }>()
const message = useMessage()
const authStore = useAuthStore()
const panelStore = usePanelStore()

const enabled = computed(() => props.detail.entryPath.enabled)
/** 当前完整相对路径（URL 前缀形态 /{entryPath}；readonly 可见完整值——ADR-018 全可看） */
const currentFullPath = computed(() => {
  const p = props.detail.entryPath.path
  return p === null || p === '' ? '' : `/${p}`
})
const effectTag = computed(() => dictItem(CONFIG_EFFECT_MODE, props.detail.entryPath.effectMode))

/** 0→1 确认 Modal（PageSpec 5.1 Card 2 交互流 1） */
const enableConfirmShow = ref(false)

function onEnableRequest(): void {
  enableConfirmShow.value = true
}

async function onEnableConfirm(): Promise<void> {
  try {
    await configUpdate({ entryPathEnabled: true })
    // 无既有路径时服务端自动生成（api 4.1.4）；统一 re-fetch 取新值
    await panelStore.loadConfig()
    message.success('安全入口已开启，变更已记入审计')
    enableConfirmShow.value = false
    const path = panelStore.configDetail?.entryPath.path ?? null
    if (path !== null) openPathModal(path)
  } catch {
    // 失败保持开关原状态（拦截器已提示）
  }
}

/** 1→0：NPopconfirm 确认（模板内触发） */
async function onDisable(): Promise<void> {
  try {
    await configUpdate({ entryPathEnabled: false })
    message.success('安全入口已关闭，变更已记入审计')
    await panelStore.loadConfig()
  } catch {
    // 拦截器已提示
  }
}

const regenerating = ref(false)

/** 重新生成：旧路径立即失效；PANEL_* 失败原路径不变（api 4.1.4） */
async function onRegenerate(): Promise<void> {
  regenerating.value = true
  try {
    const data = await entryPathRegenerate()
    message.success('已重新生成，变更已记入审计')
    await panelStore.loadConfig()
    openPathModal(data.entryPath)
  } catch {
    // 拦截器已提示
  } finally {
    regenerating.value = false
  }
}

/** 路径大字展示 Modal（prd N2 遗忘风险对策） */
const pathModalShow = ref(false)
const pathModalValue = ref('')

function openPathModal(path: string): void {
  pathModalValue.value = `/${path}`
  pathModalShow.value = true
}

async function onCopy(text: string): Promise<void> {
  const ok = await copyText(text)
  if (ok) message.success('已复制')
  else message.error('复制失败，请手动选择复制')
}
</script>

<template>
  <n-card title="安全入口">
    <template #header-extra>
      <n-tag :type="effectTag.tag" size="small" :bordered="false">{{ effectTag.label }}</n-tag>
    </template>

    <div class="switch-row">
      <span>启用安全入口</span>
      <!-- 1→0：NPopconfirm 确认「关闭后任意路径均可访问面板」 -->
      <n-popconfirm v-if="enabled && authStore.isAdmin" @positive-click="onDisable">
        关闭后任意路径均可访问面板，确认关闭？
        <template #trigger>
          <n-switch :value="true" />
        </template>
      </n-popconfirm>
      <!-- readonly：开关禁用但状态可见（ADR-018） -->
      <n-switch
        v-else
        :value="enabled"
        :disabled="!authStore.isAdmin"
        @update:value="onEnableRequest"
      />
    </div>
    <p class="hint">
      开启后未携带入口路径的访问将被拒绝（返回标准 404，属预期安全行为）；修改即时生效。
    </p>

    <!-- 当前入口路径：enabled 时展示，readonly 完整可见 -->
    <div v-if="enabled && currentFullPath !== ''" class="path-box">
      <span class="path-label">当前入口路径</span>
      <n-code :code="currentFullPath" class="path-code" />
      <n-button size="small" @click="onCopy(currentFullPath)">复制</n-button>
    </div>

    <!-- 重新生成：仅 admin 且已启用 -->
    <div v-if="authStore.isAdmin && enabled" class="actions">
      <n-popconfirm @positive-click="onRegenerate">
        旧路径立即失效，未保存新路径将无法访问，确认重新生成？
        <template #trigger>
          <n-button type="warning" ghost :loading="regenerating">重新生成路径</n-button>
        </template>
      </n-popconfirm>
    </div>

    <!-- 0→1 确认 Modal（含风险提示） -->
    <n-modal
      v-model:show="enableConfirmShow"
      preset="dialog"
      type="warning"
      title="开启安全入口"
      positive-text="开启"
      negative-text="取消"
      :mask-closable="false"
      @positive-click="onEnableConfirm"
    >
      开启后未携带入口路径的访问将被拒绝，请妥善保存路径。确认开启？
    </n-modal>

    <!-- 路径大字展示 Modal -->
    <n-modal
      v-model:show="pathModalShow"
      preset="card"
      title="安全入口路径"
      :mask-closable="false"
      style="width: 560px"
    >
      <p class="modal-warn">请立即复制并妥善保存以下路径（未携带该路径的访问将被拒绝）：</p>
      <n-code :code="pathModalValue" class="path-modal-code" />
      <template #footer>
        <div class="actions">
          <n-button type="primary" @click="onCopy(pathModalValue)">复制</n-button>
        </div>
      </template>
    </n-modal>
  </n-card>
</template>

<style scoped>
.switch-row {
  display: flex;
  align-items: center;
  gap: 16px;
}
.hint {
  margin: 8px 0 12px;
  font-size: 12px;
  opacity: 0.65;
}
.path-box {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.path-label {
  font-size: 13px;
  opacity: 0.75;
}
.path-code {
  padding: 4px 10px;
  border-radius: 3px;
  font-size: 14px;
}
.actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  margin-top: 12px;
}
.modal-warn {
  margin: 0 0 12px;
  font-size: 13px;
}
.path-modal-code {
  display: block;
  padding: 16px 12px;
  border-radius: 4px;
  font-size: 20px;
  font-weight: 600;
  text-align: center;
  word-break: break-all;
}
</style>
