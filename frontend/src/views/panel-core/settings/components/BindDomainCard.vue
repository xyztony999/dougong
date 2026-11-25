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

/** 锚点 id=panel-bind-domain-card：面板证书「请先绑定域名」提示跳转目标 */
const bound = computed(() => props.detail.bindDomain.value)
const effectTag = computed(() => dictItem(CONFIG_EFFECT_MODE, props.detail.bindDomain.effectMode))

/** 刚绑定的域名（本地态）：通过 IP 绑定时，绑定后的配置刷新本身会被 HostGuard 404，
 *  detail 拿不到新值——警示条须由本地态驱动，不能只靠刷新后的 bound。 */
const noticeDomain = ref<string | null>(null)
const noticeHost = computed(() => noticeDomain.value ?? bound.value)

/** 绑定后的域名访问地址：沿用当前协议/端口/路径，仅把主机换成绑定域名。
 *  绑定后 HostGuard 拒绝 IP 直连（404），给醒目链接由用户自行跳转——不自动跳，
 *  避免域名尚未解析/配好时把用户锁在网页外（v0.6.2 拍板）。 */
const boundAccessUrl = computed(() => {
  if (noticeHost.value === null) return ''
  const port = window.location.port ? `:${window.location.port}` : ''
  return `${window.location.protocol}//${noticeHost.value}${port}${window.location.pathname}`
})

const formRef = ref<FormInst | null>(null)
const form = ref<{ bindDomain: string }>({ bindDomain: '' })

watch(
  () => props.detail,
  () => {
    form.value.bindDomain = ''
    formRef.value?.restoreValidation()
  },
  { immediate: true }
)

/** RFC 1123 主机名（D-P12） */
const hostNamePattern = /^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$/

const rules: FormRules = {
  bindDomain: [
    {
      trigger: ['blur', 'input'],
      validator: (_rule: FormItemRule, value: string): true | Error => {
        if (value === '') return true
        if (value.length > 253) return new Error('域名长度超过 253 字符')
        return hostNamePattern.test(value)
          ? true
          : new Error('填主机名即可，不要带 http:// 或路径')
      }
    }
  ]
}

const canBind = computed(() => form.value.bindDomain.trim() !== '')

const bindConfirmShow = ref(false)
const binding = ref(false)

async function onBindRequest(): Promise<void> {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  bindConfirmShow.value = true
}

async function onBindConfirm(): Promise<void | false> {
  binding.value = true
  const domain = form.value.bindDomain.trim()
  try {
    await configUpdate({ bindDomain: domain })
    // 先记本地态再刷新：IP 访问时刷新会被 HostGuard 404，警示条仍须显示
    noticeDomain.value = domain
    message.success('已绑定，立即生效')
    bindConfirmShow.value = false
    await panelStore.loadConfig()
  } catch {
    return false
  } finally {
    binding.value = false
  }
}

const unbinding = ref(false)

async function onUnbind(): Promise<void> {
  unbinding.value = true
  try {
    await configUpdate({ bindDomain: null })
    noticeDomain.value = null
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
  <n-card id="panel-bind-domain-card" title="访问域名">
    <template #header-extra>
      <n-tag :type="effectTag.tag" size="small" :bordered="false">{{ effectTag.label }}</n-tag>
    </template>

    <div class="bind-hero" :class="{ bound: bound !== null }">
      <div class="bind-kicker">当前</div>
      <div v-if="bound !== null" class="bind-value">
        <span class="mono">{{ bound }}</span>
        <n-tag type="info" size="small" :bordered="false">已绑定</n-tag>
      </div>
      <div v-else class="bind-value muted">未绑定 · IP 和任意域名都可打开面板</div>
    </div>

    <n-alert v-if="noticeHost !== null" type="warning" title="请改用域名访问" class="bound-notice">
      <div>IP 直连和其他域名已被拒绝（404）。请通过下面地址访问面板：</div>
      <a :href="boundAccessUrl" class="bound-link mono">{{ boundAccessUrl }}</a>
    </n-alert>

    <n-form
      ref="formRef"
      :model="form"
      :rules="rules"
      :show-label="false"
      :disabled="!authStore.isAdmin"
    >
      <n-form-item path="bindDomain" :show-feedback="true">
        <div class="bind-row">
          <n-input
            v-model:value="form.bindDomain"
            placeholder="panel.example.com"
            @keyup.enter="canBind && onBindRequest()"
          />
          <n-button
            v-if="authStore.isAdmin"
            type="primary"
            :disabled="!canBind"
            :loading="binding"
            @click="onBindRequest"
          >
            绑定
          </n-button>
          <n-popconfirm v-if="authStore.isAdmin && bound !== null" @positive-click="onUnbind">
            解绑后 IP 和任意域名都能打开面板，确认？
            <template #trigger>
              <n-button :loading="unbinding">解绑</n-button>
            </template>
          </n-popconfirm>
        </div>
      </n-form-item>
    </n-form>
    <p class="hint">绑定后只能用这个域名打开面板，IP 和其他域名返回 404。</p>

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
      绑定后只能用这个域名打开面板。确认域名已经解析到本机。
    </n-modal>
  </n-card>
</template>

<style scoped>
.bind-hero {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 14px 16px;
  margin-bottom: 16px;
  border-radius: 8px;
  border: 1px solid var(--n-border-color);
  background: var(--n-color-embedded, rgba(127, 127, 127, 0.08));
}
.bind-hero.bound {
  border-color: rgba(24, 160, 88, 0.35);
}
.bind-kicker {
  font-size: 12px;
  letter-spacing: 0.04em;
  opacity: 0.55;
}
.bind-value {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  font-size: 16px;
  font-weight: 600;
}
.bind-value.muted {
  font-weight: 400;
  font-size: 13px;
  opacity: 0.75;
}
.mono {
  font-family: ui-monospace, SFMono-Regular, Consolas, 'Courier New', monospace;
  font-size: 15px;
  letter-spacing: 0.01em;
}
.bind-row {
  display: flex;
  gap: 8px;
  width: 100%;
  align-items: center;
}
.bind-row :deep(.n-input) {
  flex: 1;
}
.hint {
  margin: 4px 0 0;
  font-size: 12px;
  opacity: 0.65;
}
.bound-notice {
  margin-bottom: 16px;
}
.bound-link {
  display: inline-block;
  margin-top: 6px;
  word-break: break-all;
  font-size: 14px;
}
</style>
