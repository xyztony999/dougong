<script setup lang="ts">
/**
 * Passkey 管理卡（v0.7.0 登录安全增量）：注册/列表/删除本人 passkey。
 * 注册走 navigator.credentials.create（attestation），删除需确认。
 */
import { onMounted, ref } from 'vue'
import { useMessage } from 'naive-ui'
import { webauthnDelete, webauthnList, webauthnRegisterBegin, webauthnRegisterFinish } from '@/api/auth'
import type { WebAuthnCredentialItem } from '@/api/auth'
import { formatDateTime } from '@/utils/format'
import { passkeySupported, prepareCreateOptions, serializeCreation } from '@/utils/webauthn'

const message = useMessage()
const supported = passkeySupported()
const items = ref<WebAuthnCredentialItem[]>([])
const loading = ref(false)
const registering = ref(false)

async function load(): Promise<void> {
  loading.value = true
  try {
    const data = await webauthnList()
    items.value = data.items
  } catch {
    // 拦截器已提示
  } finally {
    loading.value = false
  }
}

async function onRegister(): Promise<void> {
  registering.value = true
  try {
    const begin = await webauthnRegisterBegin()
    const cred = (await navigator.credentials.create({ publicKey: prepareCreateOptions(begin.options) })) as PublicKeyCredential | null
    if (cred === null) {
      message.error('未获取到 passkey')
      return
    }
    await webauthnRegisterFinish({ token: begin.token, credential: serializeCreation(cred), name: '本机 passkey' })
    message.success('passkey 已添加')
    await load()
  } catch (e) {
    console.error('passkey register failed', e)
    message.error('添加 passkey 失败，请确认浏览器支持并已解锁设备后重试')
  } finally {
    registering.value = false
  }
}

async function onDelete(id: number): Promise<void> {
  try {
    await webauthnDelete({ id })
    message.success('passkey 已删除')
    await load()
  } catch {
    // 拦截器已提示
  }
}

onMounted(() => {
  void load()
})
</script>

<template>
  <n-card title="Passkey">
    <p v-if="!supported" class="hint">当前浏览器或访问方式不支持 passkey（需 HTTPS 安全上下文）。</p>
    <div v-else class="passkey-row">
      <n-button type="primary" size="small" :loading="registering" @click="onRegister">添加 passkey</n-button>
      <span class="hint">添加后可用 passkey 免密码登录，或作为密码后的第二因素。</span>
    </div>
    <n-spin :show="loading" size="small">
      <n-empty v-if="items.length === 0 && !loading" description="还没有 passkey" size="small" style="margin-top: 12px" />
      <div v-else class="passkey-list">
        <div v-for="it in items" :key="it.id" class="passkey-item">
          <span class="name">{{ it.name }}</span>
          <span class="time">添加于 {{ formatDateTime(it.createdAt) }}</span>
          <n-popconfirm @positive-click="onDelete(it.id)">
            删除后该设备将无法用此 passkey 登录，确认？
            <template #trigger>
              <n-button size="tiny">删除</n-button>
            </template>
          </n-popconfirm>
        </div>
      </div>
    </n-spin>
  </n-card>
</template>

<style scoped>
.passkey-row {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.passkey-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 12px;
}
.passkey-item {
  display: flex;
  align-items: center;
  gap: 12px;
}
.passkey-item .name {
  font-weight: 600;
}
.passkey-item .time {
  font-size: 12px;
  opacity: 0.65;
}
.hint {
  margin: 0;
  font-size: 12px;
  opacity: 0.65;
}
</style>
