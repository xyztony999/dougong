<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import type { FormInst, FormItemRule, FormRules, SelectOption } from 'naive-ui'
import { configUpdate } from '@/api/panel'
import { useAuthStore } from '@/stores/auth'
import { usePanelStore } from '@/stores/panel'
import { CONFIG_EFFECT_MODE, LISTEN_SCOPE, dictItem } from '@/types/enums'
import type { ListenScope } from '@/types/enums'
import type { PanelConfigDetail } from '@/types/panel'

const props = defineProps<{ detail: PanelConfigDetail }>()
const message = useMessage()
const authStore = useAuthStore()
const panelStore = usePanelStore()

interface ListenFormData {
  scope: ListenScope | null
  address: string
  port: number | null
}

const formRef = ref<FormInst | null>(null)
const form = ref<ListenFormData>({ scope: null, address: '', port: null })
const saving = ref(false)

const scopeOptions: SelectOption[] = (Object.keys(LISTEN_SCOPE) as ListenScope[]).map((value) => ({
  label: LISTEN_SCOPE[value].label,
  value
}))

/** Card 角标（config_effect_mode：listen.* 为 restart_required） */
const effectTag = computed(() => dictItem(CONFIG_EFFECT_MODE, props.detail.listen.effectMode))

/** I-P02 基础格式（IP/主机名，含 IPv6 冒号）；合法性终审在后端（R-14/AUTH_012/PANEL_001） */
const addressPattern = /^[A-Za-z0-9]([A-Za-z0-9.:-]*[A-Za-z0-9])?$/

const rules = computed<FormRules>(() => ({
  scope: { required: true, message: '请选择监听范围', trigger: ['blur', 'change'] },
  address:
    form.value.scope === 'custom'
      ? [
          { required: true, message: '请输入监听地址', trigger: ['blur', 'change'] },
          {
            trigger: ['blur', 'change'],
            validator: (_rule: FormItemRule, value: string): true | Error =>
              addressPattern.test(value) ? true : new Error('请输入合法的 IP 地址或主机名')
          }
        ]
      : [],
  port: [
    { required: true, type: 'number', message: '请输入监听端口', trigger: ['blur', 'change'] },
    {
      trigger: ['blur', 'change'],
      validator: (_rule: FormItemRule, value: number | null): true | Error =>
        value !== null && value >= 1024 && value <= 65535
          ? true
          : new Error('端口范围为 1024~65535')
    }
  ]
}))

watch(
  () => props.detail,
  (d) => {
    form.value = { scope: d.listen.scope, address: d.listen.address ?? '', port: d.listen.port }
  },
  { immediate: true }
)

/** 仅提交本 Card 字段（PageSpec 5.1 提交粒度决策）；scope≠custom 显式清空残留地址（I-P02） */
async function onSave(): Promise<void> {
  try {
    await formRef.value?.validate()
  } catch {
    return // 字段级提示已展示
  }
  saving.value = true
  try {
    await configUpdate({
      listenScope: form.value.scope ?? 'all',
      listenAddress: form.value.scope === 'custom' ? form.value.address : null,
      listenPort: form.value.port ?? 0
    })
    message.success('已保存，重启服务后生效')
    await panelStore.loadConfig()
  } catch {
    // 错误已由拦截器统一提示（AUTH_012 fields / PANEL_001）
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <n-card title="监听设置">
    <template #header-extra>
      <n-tag :type="effectTag.tag" size="small" :bordered="false">{{ effectTag.label }}</n-tag>
    </template>
    <n-form
      ref="formRef"
      :model="form"
      :rules="rules"
      label-placement="top"
      :disabled="!authStore.isAdmin"
      style="max-width: 420px"
    >
      <n-form-item label="监听范围" path="scope">
        <n-select v-model:value="form.scope" :options="scopeOptions" />
      </n-form-item>
      <n-form-item v-if="form.scope === 'custom'" label="监听地址" path="address">
        <n-input v-model:value="form.address" placeholder="IP 地址或主机名" />
      </n-form-item>
      <n-form-item label="监听端口" path="port">
        <n-input-number
          v-model:value="form.port"
          :min="1024"
          :max="65535"
          placeholder="1024~65535"
          style="width: 100%"
        />
      </n-form-item>
    </n-form>
    <div v-if="authStore.isAdmin" class="actions">
      <n-button type="primary" :loading="saving" @click="onSave">保存</n-button>
    </div>
    <p class="hint">监听配置保存后不热生效，重启面板服务后应用（安装时生成的端口为默认值）。</p>
  </n-card>
</template>

<style scoped>
.actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
.hint {
  margin: 8px 0 0;
  font-size: 12px;
  opacity: 0.65;
}
</style>
