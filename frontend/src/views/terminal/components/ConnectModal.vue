<script setup lang="ts">
/**
 * ConnectModal —— 新建会话对话框（PageSpec 第 5 节，2 字段 <5 → NModal Dialog）。
 * 身份来源 radio（terminal_identity_source 两值，默认 panel_run_user）+ 系统用户 select
 * （仅 specified_system_user 显示，选项=userOptions items，D-02 实况直读）。
 * 候选拉取失败（TERM_007）：NAlert error + 重试，「指定系统用户」不可用、「默认身份」仍可连接。
 */
import { computed, ref, watch } from 'vue'
import type { SelectOption } from 'naive-ui'
import { useTerminalStore } from '@/stores/terminal'
import { TERMINAL_IDENTITY_SOURCE } from '@/types/enums'
import type { TerminalIdentitySource } from '@/types/enums'
import { TERM_007_TEXT } from '@/types/terminal'

const emit = defineEmits<{
  connect: [identitySource: TerminalIdentitySource, username: string]
}>()

const store = useTerminalStore()

const identitySource = ref<TerminalIdentitySource>('panel_run_user')
const username = ref<string | null>(null)

const show = computed(() => store.dialogVisible)
const optionsAvailable = computed(() => store.userOptionsData !== null)
const optionsFailed = computed(() => store.userOptionsError !== null)

/** 选项 label 辅显 uid/shell（服务端已滤非交互 shell，api D-02） */
const selectOptions = computed<SelectOption[]>(() =>
  (store.userOptionsData?.items ?? []).map((u) => ({
    label: `${u.name}（uid ${u.uid} · ${u.shell}）`,
    value: u.name
  }))
)

/** 打开即复位（重连=重走新建流程不继承任何状态，api 4.1.2 辨析③） */
watch(show, (visible) => {
  if (visible) {
    identitySource.value = 'panel_run_user'
    username.value = null
  }
})

const connectDisabled = computed(
  () => identitySource.value === 'specified_system_user' && (username.value === null || username.value === '')
)

function onConnect(): void {
  if (connectDisabled.value) return
  emit(
    'connect',
    identitySource.value,
    identitySource.value === 'specified_system_user' ? (username.value ?? '') : ''
  )
}

function retryOptions(): void {
  void store.loadUserOptions()
}

function onUpdateVisible(visible: boolean): void {
  if (!visible) store.closeConnectDialog()
}
</script>

<template>
  <n-modal
    :show="show"
    preset="card"
    title="新建终端会话"
    class="connect-modal"
    :mask-closable="false"
    :style="{ width: '520px' }"
    @update:show="onUpdateVisible"
  >
    <n-alert v-if="optionsFailed" type="error" class="options-alert" :bordered="false">
      {{ TERM_007_TEXT }}
    </n-alert>
    <div v-if="optionsFailed" class="options-retry">
      <n-button size="small" @click="retryOptions">重试</n-button>
    </div>
    <n-form label-placement="top">
      <n-form-item label="身份来源">
        <n-radio-group v-model:value="identitySource">
          <n-radio value="panel_run_user">
            {{ TERMINAL_IDENTITY_SOURCE.panel_run_user.label }}
          </n-radio>
          <n-radio value="specified_system_user" :disabled="!optionsAvailable">
            {{ TERMINAL_IDENTITY_SOURCE.specified_system_user.label }}
          </n-radio>
        </n-radio-group>
      </n-form-item>
      <n-form-item v-if="identitySource === 'specified_system_user'" label="系统用户">
        <n-select
          v-model:value="username"
          filterable
          clearable
          :options="selectOptions"
          :loading="store.userOptionsLoading"
          placeholder="选择系统用户"
        />
      </n-form-item>
    </n-form>
    <template #footer>
      <div class="modal-footer">
        <n-button @click="store.closeConnectDialog()">取消</n-button>
        <n-button type="primary" :disabled="connectDisabled" @click="onConnect">连接</n-button>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.options-alert {
  margin-bottom: 8px;
}
.options-retry {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 12px;
}
.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
