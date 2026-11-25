<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useDialog, useMessage } from 'naive-ui'
import {
  envConfigFile,
  envInstall,
  envProfile,
  envSaveConfigFile,
  envSaveSettings,
  envServiceOp,
  envUninstall,
  envVersions
} from '@/api/env'
import { ApiError } from '@/api/http'
import { useAuthStore } from '@/stores/auth'
import { useEnvStore } from '@/stores/env'
import { ENV_TYPE } from '@/types/enums'
import type { EnvType } from '@/types/enums'
import type { EnvItem, EnvProfileData, EnvServiceOp, EnvSettingView, EnvVersionItem } from '@/types/env'
import { ENV_ACCENT, ENV_MONOGRAM, isRuntimeEnv } from './envMeta'
import EnvTaskDrawer from './components/EnvTaskDrawer.vue'
import { envTaskColumns, envTypeOptions, statusOptions } from './taskColumns'

const route = useRoute()
const router = useRouter()
const message = useMessage()
const dialog = useDialog()
const authStore = useAuthStore()
const store = useEnvStore()

const envType = computed<EnvType | null>(() => {
  const t = String(route.params.envType ?? '')
  return t in ENV_TYPE ? (t as EnvType) : null
})

const profile = ref<EnvProfileData | null>(null)
const loading = ref(false)
const loadError = ref(false)
const tab = ref<'service' | 'settings' | 'files' | 'versions'>('service')

const item = computed<EnvItem | null>(() => profile.value?.item ?? null)
const hasService = computed(() => profile.value?.hasService === true)

const settingsForm = ref<Record<string, string>>({})
const savingSettings = ref(false)

const fileId = ref('')
const fileContent = ref('')
const filePath = ref('')
const fileLoading = ref(false)
const savingFile = ref(false)

const versions = ref<EnvVersionItem[]>([])
const versionsLoading = ref(false)
const installing = ref(false)

watch(
  () => profile.value?.settings,
  (list) => {
    const next: Record<string, string> = {}
    for (const s of list ?? []) next[s.key] = s.value
    settingsForm.value = next
  }
)

watch(
  () => profile.value?.configFiles,
  (files) => {
    const first = (files ?? []).find((f) => f.exists) ?? files?.[0]
    fileId.value = first?.id ?? ''
  }
)

async function loadProfile(): Promise<void> {
  if (envType.value === null) return
  loading.value = true
  loadError.value = false
  try {
    profile.value = await envProfile(envType.value)
    if (isRuntimeEnv(envType.value) || !hasService.value) {
      if (tab.value === 'service') tab.value = (profile.value.settings.length > 0 ? 'settings' : 'versions')
    }
  } catch {
    loadError.value = true
  } finally {
    loading.value = false
  }
}

async function loadFile(): Promise<void> {
  if (envType.value === null || fileId.value === '') return
  const meta = profile.value?.configFiles.find((f) => f.id === fileId.value)
  if (meta === undefined || !meta.exists) {
    fileContent.value = ''
    filePath.value = meta?.path ?? ''
    return
  }
  fileLoading.value = true
  try {
    const data = await envConfigFile(envType.value, fileId.value)
    fileContent.value = data.content
    filePath.value = data.path
  } catch {
    fileContent.value = ''
  } finally {
    fileLoading.value = false
  }
}

async function loadVersions(): Promise<void> {
  if (envType.value === null) return
  versionsLoading.value = true
  try {
    const data = await envVersions(envType.value)
    versions.value = data.versions
  } catch {
    versions.value = []
  } finally {
    versionsLoading.value = false
  }
}

watch(fileId, () => {
  void loadFile()
})

watch(tab, (t) => {
  if (t === 'versions') void loadVersions()
  if (t === 'files') void loadFile()
})

function handleEnvError(e: unknown): void {
  if (!(e instanceof ApiError)) return
  if (e.is('ENV_001')) {
    message.error('包管理器通道不可用')
    return
  }
  if (e.is('ENV_003')) {
    message.error('该环境已有任务进行中')
    const taskId = (e.data as Record<string, unknown> | null)?.taskId
    if (typeof taskId === 'number') {
      drawerShow.value = true
      void store.loadTasks(1).then(() => store.loadTaskDetail(taskId))
    }
    return
  }
  if (e.is('ENV_004')) {
    message.error('环境未安装')
    void loadProfile()
    return
  }
  if (e.is('ENV_006')) {
    const reason = (e.data as Record<string, unknown> | null)?.reason
    dialog.error({
      title: '服务操作失败',
      content: typeof reason === 'string' ? reason : '服务端未返回原因'
    })
    return
  }
  if (e.is('ENV_007') || e.is('ENV_008')) {
    const reason = (e.data as Record<string, unknown> | null)?.reason
    message.error(typeof reason === 'string' && reason !== '' ? reason : e.message)
    return
  }
  message.error(e.message)
}

const serviceBusy = ref<string | null>(null)

async function onServiceOp(op: EnvServiceOp): Promise<void> {
  if (envType.value === null) return
  serviceBusy.value = op
  try {
    await envServiceOp(op, envType.value)
    message.success('已执行')
    await loadProfile()
  } catch (e) {
    handleEnvError(e)
  } finally {
    serviceBusy.value = null
  }
}

async function onSaveSettings(): Promise<void> {
  if (envType.value === null) return
  savingSettings.value = true
  try {
    await envSaveSettings(envType.value, settingsForm.value)
    message.success('配置已保存')
    await loadProfile()
  } catch (e) {
    handleEnvError(e)
  } finally {
    savingSettings.value = false
  }
}

async function onSaveFile(): Promise<void> {
  if (envType.value === null || fileId.value === '') return
  savingFile.value = true
  try {
    await envSaveConfigFile(envType.value, fileId.value, fileContent.value)
    message.success('文件已保存')
    await loadProfile()
  } catch (e) {
    handleEnvError(e)
  } finally {
    savingFile.value = false
  }
}

const drawerShow = ref(false)
const taskColumns = computed(() => envTaskColumns((id) => void store.loadTaskDetail(id)))

async function onInstall(version?: string): Promise<void> {
  if (envType.value === null) return
  installing.value = true
  try {
    const res = await envInstall(envType.value, version)
    message.success(version ? '切换任务已创建' : '任务已创建')
    drawerShow.value = true
    await store.refreshAfterWrite()
    void store.loadTaskDetail(res.taskId)
    await loadProfile()
    await loadVersions()
  } catch (e) {
    handleEnvError(e)
  } finally {
    installing.value = false
  }
}

const uninstallConfirmName = ref('')
const uninstallShow = ref(false)
const uninstalling = ref(false)
const uninstallOk = computed(
  () => item.value !== null && uninstallConfirmName.value === item.value.displayName
)

async function onUninstall(): Promise<void> {
  if (envType.value === null) return
  uninstalling.value = true
  try {
    const res = await envUninstall(envType.value)
    message.success('卸载任务已创建')
    uninstallShow.value = false
    drawerShow.value = true
    await store.refreshAfterWrite()
    void store.loadTaskDetail(res.taskId)
    await loadProfile()
  } catch (e) {
    handleEnvError(e)
  } finally {
    uninstalling.value = false
  }
}

function settingControl(s: EnvSettingView): 'switch' | 'text' {
  return s.type === 'switch' ? 'switch' : 'text'
}

onMounted(() => {
  if (envType.value === null) {
    void router.replace('/env')
    return
  }
  void loadProfile()
  void store.loadList()
})

onUnmounted(() => {
  store.stopPoll()
  store.clearTaskDetail()
})
</script>

<template>
  <div v-if="envType !== null" class="page">
    <div class="crumb">
      <n-button text @click="router.push('/env')">环境管理</n-button>
      <span class="crumb-sep">/</span>
      <span>{{ ENV_TYPE[envType].label }}</span>
    </div>

    <n-spin :show="loading && profile === null">
      <n-result v-if="loadError && profile === null" status="error" title="加载失败">
        <template #footer>
          <n-button type="primary" @click="loadProfile">重试</n-button>
        </template>
      </n-result>

      <template v-else-if="item !== null">
        <header class="hero">
          <span class="rail" :style="{ background: ENV_ACCENT[envType] }"></span>
          <span class="mono-mark" :style="{ color: ENV_ACCENT[envType] }">{{ ENV_MONOGRAM[envType] }}</span>
          <div class="hero-text">
            <h1>{{ item.displayName }}</h1>
            <p>
              <template v-if="item.installed === true">
                {{ item.version ?? '已安装' }}
                <template v-if="item.unitName !== ''">
                  · {{ item.serviceRunning ? '运行中' : '已停止' }}
                  · 自启{{ item.serviceEnabled ? '开' : '关' }}
                </template>
              </template>
              <template v-else-if="item.installed === false">未安装</template>
              <template v-else>状态未知</template>
            </p>
          </div>
          <div v-if="authStore.isAdmin" class="hero-actions">
            <template v-if="hasService && item.installed === true">
              <n-button size="small" :loading="serviceBusy === 'start'" @click="onServiceOp('start')">
                启动
              </n-button>
              <n-button size="small" :loading="serviceBusy === 'stop'" @click="onServiceOp('stop')">
                停止
              </n-button>
              <n-button size="small" :loading="serviceBusy === 'restart'" @click="onServiceOp('restart')">
                重启
              </n-button>
              <n-switch
                :value="item.serviceEnabled === true"
                :loading="serviceBusy === 'enable' || serviceBusy === 'disable'"
                @update:value="(v: boolean) => onServiceOp(v ? 'enable' : 'disable')"
              />
            </template>
            <n-button v-if="item.installed === true" size="small" type="error" ghost @click="uninstallShow = true">
              卸载
            </n-button>
            <n-popconfirm v-else-if="item.installed === false" @positive-click="onInstall()">
              <template #trigger>
                <n-button size="small" type="primary" :loading="installing">安装</n-button>
              </template>
              将安装 {{ item.displayName }}，大约几分钟。
            </n-popconfirm>
          </div>
        </header>

        <n-tabs v-model:value="tab" type="line" animated>
          <n-tab-pane v-if="hasService" name="service" tab="服务">
            <p class="pane-lead">单元 {{ item.unitName || '—' }}</p>
            <n-alert v-if="item.installed !== true" type="info" :closable="false">
              安装后可在这里启停服务。
            </n-alert>
            <div v-else class="kv">
              <div><span>运行</span>{{ item.serviceRunning ? '运行中' : '已停止' }}</div>
              <div><span>开机自启</span>{{ item.serviceEnabled ? '开' : '关' }}</div>
            </div>
          </n-tab-pane>

          <n-tab-pane v-if="(profile?.settings.length ?? 0) > 0" name="settings" tab="常用配置">
            <n-alert v-if="item.installed !== true" type="info" :closable="false">
              安装后可改这些常用项。
            </n-alert>
            <n-form v-else label-placement="left" label-width="140" class="settings-form">
              <n-form-item v-for="s in profile?.settings ?? []" :key="s.key" :label="s.label">
                <n-switch
                  v-if="settingControl(s) === 'switch'"
                  :value="settingsForm[s.key] === (s.options?.[0] ?? 'on')"
                  @update:value="(v: boolean) => (settingsForm[s.key] = v ? (s.options?.[0] ?? 'on') : (s.options?.[1] ?? 'off'))"
                />
                <n-input v-else v-model:value="settingsForm[s.key]" :placeholder="s.hint || s.key" />
                <template v-if="s.hint" #feedback>{{ s.hint }}</template>
              </n-form-item>
              <n-button
                v-if="authStore.isAdmin"
                type="primary"
                :loading="savingSettings"
                @click="onSaveSettings"
              >
                保存配置
              </n-button>
            </n-form>
          </n-tab-pane>

          <n-tab-pane v-if="(profile?.configFiles.length ?? 0) > 0" name="files" tab="配置文件">
            <div class="file-bar">
              <n-select
                v-model:value="fileId"
                :options="(profile?.configFiles ?? []).map((f) => ({ label: `${f.label}  ${f.path}`, value: f.id }))"
                style="max-width: 520px"
              />
              <n-button
                v-if="authStore.isAdmin"
                type="primary"
                :disabled="fileId === '' || !(profile?.configFiles.find((f) => f.id === fileId)?.exists)"
                :loading="savingFile"
                @click="onSaveFile"
              >
                保存文件
              </n-button>
            </div>
            <p v-if="filePath" class="path">{{ filePath }}</p>
            <n-spin :show="fileLoading">
              <n-input
                v-model:value="fileContent"
                type="textarea"
                :rows="22"
                class="mono"
                :disabled="!authStore.isAdmin || !(profile?.configFiles.find((f) => f.id === fileId)?.exists)"
                placeholder="文件不存在或尚未安装"
              />
            </n-spin>
          </n-tab-pane>

          <n-tab-pane name="versions" tab="版本">
            <div class="ver-head">
              <span>当前 {{ item.version || '—' }}</span>
              <n-button
                v-if="authStore.isAdmin && item.installed === true"
                size="small"
                :loading="installing"
                @click="onInstall()"
              >
                升级到最新
              </n-button>
            </div>
            <n-spin :show="versionsLoading">
              <n-empty v-if="versions.length === 0" description="仓库里没有读到版本，可以直接升级到最新。" />
              <ul v-else class="ver-list">
                <li v-for="row in versions" :key="row.version" class="ver-row">
                  <span class="ver-id">{{ row.version }}</span>
                  <n-tag v-if="row.current" type="success" size="small" :bordered="false">当前</n-tag>
                  <n-popconfirm
                    v-else-if="authStore.isAdmin"
                    @positive-click="onInstall(row.version)"
                  >
                    <template #trigger>
                      <n-button size="tiny" :disabled="installing">切换</n-button>
                    </template>
                    将切换到 {{ row.version }}，大约几分钟。
                  </n-popconfirm>
                </li>
              </ul>
            </n-spin>
          </n-tab-pane>
        </n-tabs>
      </template>
    </n-spin>

    <env-task-drawer
      v-model:show="drawerShow"
      :columns="taskColumns"
      :env-type-options="envTypeOptions"
      :status-options="statusOptions"
    />

    <n-modal
      v-model:show="uninstallShow"
      preset="card"
      title="卸载环境"
      style="width: 480px"
      :mask-closable="false"
    >
      <n-alert type="warning" style="margin-bottom: 12px">
        卸载 {{ item?.displayName }} 会立即停用对应功能，配置和数据目录会保留。
      </n-alert>
      <n-input v-model:value="uninstallConfirmName" :placeholder="`输入 ${item?.displayName ?? ''} 确认`" />
      <template #footer>
        <n-space justify="end">
          <n-button @click="uninstallShow = false">取消</n-button>
          <n-button type="error" :disabled="!uninstallOk" :loading="uninstalling" @click="onUninstall">
            确认卸载
          </n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-width: 960px;
}
.crumb {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  opacity: 0.75;
}
.crumb-sep {
  opacity: 0.4;
}
.hero {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 16px 16px 16px 0;
  border: 1px solid var(--n-border-color);
  border-radius: 10px;
  overflow: hidden;
}
.rail {
  width: 4px;
  align-self: stretch;
}
.mono-mark {
  width: 44px;
  height: 44px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-family: ui-monospace, SFMono-Regular, Consolas, 'Courier New', monospace;
  font-weight: 700;
  background: rgba(127, 127, 127, 0.12);
}
.hero-text {
  flex: 1;
  min-width: 0;
}
.hero-text h1 {
  margin: 0;
  font-size: 18px;
}
.hero-text p {
  margin: 4px 0 0;
  font-size: 12px;
  opacity: 0.7;
}
.hero-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  align-items: center;
  padding-right: 16px;
}
.pane-lead {
  margin: 0 0 12px;
  font-size: 13px;
  opacity: 0.7;
}
.kv {
  display: grid;
  gap: 8px;
  font-size: 13px;
}
.kv span {
  display: inline-block;
  width: 88px;
  opacity: 0.55;
}
.settings-form {
  max-width: 520px;
}
.file-bar {
  display: flex;
  gap: 12px;
  margin-bottom: 8px;
  align-items: center;
}
.path {
  margin: 0 0 8px;
  font-size: 12px;
  font-family: ui-monospace, SFMono-Regular, Consolas, 'Courier New', monospace;
  opacity: 0.65;
}
.mono :deep(textarea) {
  font-family: ui-monospace, SFMono-Regular, Consolas, 'Courier New', monospace;
  font-size: 12px;
}
.ver-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
  font-size: 13px;
}
.ver-list {
  list-style: none;
  margin: 0;
  padding: 0;
  border: 1px solid var(--n-border-color);
  border-radius: 8px;
  overflow: hidden;
}
.ver-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 14px;
  border-bottom: 1px solid var(--n-border-color);
}
.ver-row:last-child {
  border-bottom: 0;
}
.ver-id {
  flex: 1;
  font-family: ui-monospace, SFMono-Regular, Consolas, 'Courier New', monospace;
  font-size: 13px;
}
</style>
