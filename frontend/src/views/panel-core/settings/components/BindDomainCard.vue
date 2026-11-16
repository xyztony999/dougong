<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import type { FormInst, FormItemRule, FormRules } from 'naive-ui'
import { configUpdate } from '@/api/panel'
import { useAuthStore } from '@/stores/auth'
import { usePanelStore } from '@/stores/panel'
import { CONFIG_EFFECT_MODE, dictItem } from '@/types/enums'
import type { PanelConfigDetail } from '@/types/panel'

/**
 * Card 3 域名绑定（PageSpec 5.1 v1.1，ADR-028/D-P12）：
 * 绑定/解绑均经 configUpdate.bindDomain（null/空串=解绑）；即时生效（HostGuard 下一请求即用，D-P16）。
 * readonly：输入框 disabled、无绑定/解绑按钮（负空间），绑定状态可见（ADR-018 全可看）。
 */
const props = defineProps<{ detail: PanelConfigDetail }>()
const message = useMessage()
const authStore = useAuthStore()
const panelStore = usePanelStore()

/** 锚点 id=panel-bind-domain-card：面板证书 Card「请先绑定域名」提示跳转目标（PageSpec 5.1 Card 5） */
const bound = computed(() => props.detail.bindDomain.value)
const effectTag = computed(() => dictItem(CONFIG_EFFECT_MODE, props.detail.bindDomain.effectMode))

const formRef = ref<FormInst | null>(null)
const form = ref<{ bindDomain: string }>({ bindDomain: '' })

watch(
  () => props.detail,
  () => {
    // 输入框不回填当前绑定值（绑定动作语义=填新值；当前值在只读状态区展示）
    form.value.bindDomain = ''
    formRef.value?.restoreValidation()
  },
  { immediate: true }
)

/** RFC 1123 主机名（D-P12）：label 集 [A-Za-z0-9-] 首尾非连字符、单 label ≤63、总长 ≤253；不含 scheme/path/端口（正则天然排除 : 与 /） */
const hostNamePattern = /^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$/

const rules: FormRules = {
  bindDomain: [
    {
      trigger: ['blur', 'input'],
      validator: (_rule: FormItemRule, value: string): true | Error => {
        if (value === '') return true // 空串不触发绑定（按钮置不可用）
        if (value.length > 253) return new Error('域名长度超过 253 字符')
        return hostNamePattern.test(value)
          ? true
          : new Error('域名格式不合法（应为主机名，不含 http:// 或路径）')
      }
    }
  ]
}

/** 绑定按钮可用 = 输入非空（格式校验在确认前 validate；PageSpec「输入非空域名后可用」） */
const canBind = computed(() => form.value.bindDomain.trim() !== '')

/** 绑定 NModal 强确认（ADR-028 风险对策；文案逐字 = PageSpec 5.1 Card 3） */
const bindConfirmShow = ref(false)
const binding = ref(false)

async function onBindRequest(): Promise<void> {
  try {
    await formRef.value?.validate()
  } catch {
    return // 字段级提示已展示（与 AUTH_012 fields.bindDomain 同文案口径）
  }
  bindConfirmShow.value = true
}

/** 返回 false = 保持 Modal 打开（Naive UI dialog 约定）：失败后可改值重试 */
async function onBindConfirm(): Promise<void | false> {
  binding.value = true
  try {
    await configUpdate({ bindDomain: form.value.bindDomain.trim() })
    message.success('域名已绑定，即时生效（变更已记入审计）')
    bindConfirmShow.value = false
    await panelStore.loadConfig()
  } catch {
    // AUTH_012 fields / 服务端校验已由拦截器统一提示
    return false
  } finally {
    binding.value = false
  }
}

/** 解绑：NPopconfirm 确认（文案 = PageSpec 5.1 Card 3） */
const unbinding = ref(false)

async function onUnbind(): Promise<void> {
  unbinding.value = true
  try {
    await configUpdate({ bindDomain: null })
    message.success('已解绑')
    await panelStore.loadConfig()
  } catch {
    // 拦截器已提示
  } finally {
    unbinding.value = false
  }
}
</script>

<template>
  <n-card id="panel-bind-domain-card" title="域名绑定">
    <template #header-extra>
      <n-tag :type="effectTag.tag" size="small" :bordered="false">{{ effectTag.label }}</n-tag>
    </template>

    <!-- 当前绑定状态（readonly 可见，ADR-018 全可看） -->
    <n-descriptions label-placement="left" :column="1" bordered size="small" class="bound-desc">
      <n-descriptions-item label="当前绑定">
        <template v-if="bound !== null">
          <n-code :code="bound" class="domain-code" />
          <n-tag type="info" size="small" :bordered="false" class="bound-tag">已绑定</n-tag>
        </template>
        <n-tag v-else size="small" :bordered="false">未绑定</n-tag>
      </n-descriptions-item>
    </n-descriptions>

    <!-- 绑定表单：readonly disabled、无按钮（负空间） -->
    <n-form
      ref="formRef"
      :model="form"
      :rules="rules"
      label-placement="top"
      :disabled="!authStore.isAdmin"
      style="max-width: 420px"
    >
      <n-form-item label="绑定域名" path="bindDomain">
        <n-input
          v-model:value="form.bindDomain"
          placeholder="未绑定——填写域名后仅该域名可访问面板"
        />
      </n-form-item>
    </n-form>
    <p class="hint">
      绑定后仅该域名可访问面板（Host 全等比较，大小写不敏感），IP 直连与其他域名将返回 404；修改即时生效。
    </p>

    <div v-if="authStore.isAdmin" class="actions">
      <n-popconfirm v-if="bound !== null" @positive-click="onUnbind">
        解绑后任意 Host 与 IP 均可访问面板，确认解绑？
        <template #trigger>
          <n-button :loading="unbinding">解绑</n-button>
        </template>
      </n-popconfirm>
      <n-button type="primary" :disabled="!canBind" :loading="binding" @click="onBindRequest">
        绑定
      </n-button>
    </div>

    <!-- 绑定强确认 Modal（ADR-028：404 风险 + 解析前置 + 其他访问方式提醒，文案逐字） -->
    <n-modal
      v-model:show="bindConfirmShow"
      preset="dialog"
      type="warning"
      title="绑定域名"
      positive-text="确认绑定"
      negative-text="取消"
      :mask-closable="false"
      @positive-click="onBindConfirm"
    >
      绑定后仅该域名可访问面板，IP 直连与其他域名将返回 404 无法访问。请确认域名已解析到本服务器，且已妥善保存其他访问方式（如安全入口路径）。
    </n-modal>
  </n-card>
</template>

<style scoped>
.bound-desc {
  max-width: 560px;
  margin-bottom: 12px;
}
.domain-code {
  padding: 2px 8px;
  border-radius: 3px;
  font-size: 13px;
}
.bound-tag {
  margin-left: 8px;
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
</style>
