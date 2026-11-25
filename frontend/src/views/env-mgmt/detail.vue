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
import { ENV_ACCENT, isRuntimeEnv, shortEnvVersion } from './envMeta'
import EnvLogo from './EnvLogo.vue'
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
        <header class="hero" :style="{ '--env-accent': ENV_ACCENT[envType] }">
          <env-logo :env-type="envType" :size="52" />
          <div class="hero-text">
            <h1>{{ item.displayName }}</h1>
            <div class="hero-meta">
              <template v-if="item.installed === true">
                <span class="chip mono" :title="item.version ?? ''">
                  {{ shortEnvVersion(item.version) || '已安装' }}
                </span>
                <span v-if="item.unitName !== ''" class="chip" :class="{ live: item.serviceRunning }">
                  {{ item.serviceRunning ? '运行中' : '已停止' }}
                </span>
                <span v-if="item.unitName !== ''" class="chip">
                  自启{{ item.serviceEnabled ? '开' : '关' }}
                </span>
              </template>
              <span v-else-if="item.installed === false" class="chip">未安装</span>
              <span v-else class="chip">状态未知</span>
            </div>
          </div>
          <div v-if="authStore.isAdmin" class="hero-actions">
            <template v-if="hasService && item.installed === true">
              <n-button-group size="small">
                <n-button :loading="serviceBusy === 'start'" @click="onServiceOp('start')">启动</n-button>
                <n-button :loading="serviceBusy === 'stop'" @click="onServiceOp('stop')">停止</n-button>
                <n-button :loading="serviceBusy === 'restart'" @click="onServiceOp('restart')">重启</n-button>
              </n-button-group>
              <label class="autostart">
                <span>自启</span>
                <n-switch
                  :value="item.serviceEnabled === true"
                  :loading="serviceBusy === 'enable' || serviceBusy === 'disable'"
                  @update:value="(v: boolean) => onServiceOp(v ? 'enable' : 'disable')"
                />
              </label>
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

        <n-tabs v-model:value="tab" type="line" animated class="tabs">
          <n-tab-pane v-if="hasService" name="service" tab="服务">
            <n-alert v-if="item.installed !== true" type="info" :closable="false">
              安装后可在这里启停服务。
            </n-alert>
            <div v-else class="stat-grid">
              <div class="stat">
                <div class="stat-k">单元</div>
                <div class="stat-v mono">{{ item.unitName || '—' }}</div>
              </div>
              <div class="stat">
                <div class="stat-k">运行</div>
                <div class="stat-v" :class="{ live: item.serviceRunning }">
                  {{ item.serviceRunning ? '运行中' : '已停止' }}
                </div>
              </div>
              <div class="stat">
                <div class="stat-k">开机自启</div>
                <div class="stat-v">{{ item.serviceEnabled ? '开' : '关' }}</div>
              </div>
            </div>
          </n-tab-pane>

          <n-tab-pane v-if="(profile?.settings.length ?? 0) > 0" name="settings" tab="常用配置">
            <n-alert v-if="item.installed !== true" type="info" :closable="false">
              安装后可改这些常用项。
            </n-alert>
            <div v-else class="sheet">
              <div v-for="s in profile?.settings ?? []" :key="s.key" class="sheet-row">
                <div class="sheet-copy">
                  <div class="sheet-name">{{ s.label }}</div>
                  <div v-if="s.hint" class="sheet-hint">{{ s.hint }}</div>
                </div>
                <div class="sheet-control">
                  <n-switch
                    v-if="settingControl(s) === 'switch'"
                    :value="settingsForm[s.key] === (s.options?.[0] ?? 'on')"
                    @update:value="(v: boolean) => (settingsForm[s.key] = v ? (s.options?.[0] ?? 'on') : (s.options?.[1] ?? 'off'))"
                  />
                  <n-input
                    v-else
                    v-model:value="settingsForm[s.key]"
                    size="small"
                    :placeholder="s.hint || s.key"
                  />
                </div>
              </div>
              <div v-if="authStore.isAdmin" class="sheet-foot">
                <n-button type="primary" :loading="savingSettings" @click="onSaveSettings">
                  保存配置
                </n-button>
              </div>
            </div>
          </n-tab-pane>

          <n-tab-pane v-if="(profile?.configFiles.length ?? 0) > 0" name="files" tab="配置文件">
            <div class="editor">
              <div class="editor-bar">
                <n-select
                  v-model:value="fileId"
                  size="small"
                  :options="(profile?.configFiles ?? []).map((f) => ({ label: f.label, value: f.id }))"
                  style="width: 180px"
                />
                <span class="editor-path">{{ filePath || '—' }}</span>
                <n-button
                  v-if="authStore.isAdmin"
                  size="small"
                  type="primary"
                  :disabled="fileId === '' || !(profile?.configFiles.find((f) => f.id === fileId)?.exists)"
                  :loading="savingFile"
                  @click="onSaveFile"
                >
                  保存文件
                </n-button>
              </div>
              <n-spin :show="fileLoading">
                <n-input
                  v-model:value="fileContent"
                  type="textarea"
                  :rows="22"
                  class="mono editor-body"
                  :disabled="!authStore.isAdmin || !(profile?.configFiles.find((f) => f.id === fileId)?.exists)"
                  placeholder="文件不存在或尚未安装"
                />
              </n-spin>
            </div>
          </n-tab-pane>

          <n-tab-pane name="versions" tab="版本">
            <div class="ver-banner">
              <div>
                <div class="stat-k">当前版本</div>
                <div class="stat-v mono" :title="item.version ?? ''">
                  {{ shortEnvVersion(item.version) || item.version || '—' }}
                </div>
              </div>
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
                <li v-for="row in versions" :key="row.version" class="ver-row" :class="{ current: row.current }">
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
  gap: 20px;
  max-width: 880px;
}
.crumb {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  opacity: 0.7;
}
.crumb-sep {
  opacity: 0.35;
}
.hero {
  --env-accent: #18a058;
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 18px 18px 18px 16px;
  border: 1px solid color-mix(in srgb, var(--env-accent) 28%, var(--n-border-color, #3a3a3a));
  border-radius: 14px;
  background:
    linear-gradient(135deg, color-mix(in srgb, var(--env-accent) 16%, transparent), transparent 58%),
    var(--n-color, transparent);
  overflow: hidden;
}
.hero-text {
  flex: 1;
  min-width: 0;
}
.hero-text h1 {
  margin: 0;
  font-size: 20px;
  font-weight: 650;
  letter-spacing: -0.02em;
}
.hero-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
}
.chip {
  display: inline-flex;
  align-items: center;
  max-width: 100%;
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 12px;
  background: rgba(127, 127, 127, 0.12);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.chip.live {
  color: #18a058;
  background: rgba(24, 160, 88, 0.14);
}
.chip.mono,
.mono,
.stat-v.mono,
.ver-id,
.editor-path {
  font-family: ui-monospace, SFMono-Regular, Consolas, 'Courier New', monospace;
}
.hero-actions {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
  align-items: center;
}
.autostart {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  opacity: 0.8;
}
.tabs {
  margin-top: 4px;
}
.stat-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 10px;
}
.stat {
  padding: 14px 16px;
  border-radius: 12px;
  border: 1px solid var(--n-border-color);
  background: rgba(127, 127, 127, 0.05);
}
.stat-k {
  font-size: 12px;
  opacity: 0.55;
  margin-bottom: 6px;
}
.stat-v {
  font-size: 15px;
  font-weight: 600;
  word-break: break-all;
}
.stat-v.live {
  color: #18a058;
}
.sheet {
  border: 1px solid var(--n-border-color);
  border-radius: 12px;
  overflow: hidden;
}
.sheet-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(140px, 220px);
  gap: 16px;
  align-items: center;
  padding: 14px 16px;
  border-bottom: 1px solid var(--n-border-color);
}
.sheet-row:has(+ .sheet-foot) {
  border-bottom: 0;
}
.sheet-name {
  font-size: 14px;
  font-weight: 560;
}
.sheet-hint {
  margin-top: 4px;
  font-size: 12px;
  line-height: 1.45;
  opacity: 0.58;
}
.sheet-control {
  justify-self: end;
  width: 100%;
  display: flex;
  justify-content: flex-end;
}
.sheet-control :deep(.n-input) {
  width: 100%;
}
.sheet-foot {
  display: flex;
  justify-content: flex-end;
  padding: 12px 16px;
  border-top: 1px solid var(--n-border-color);
  background: rgba(127, 127, 127, 0.04);
}
.editor {
  border: 1px solid var(--n-border-color);
  border-radius: 12px;
  overflow: hidden;
}
.editor-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  background: rgba(127, 127, 127, 0.06);
  border-bottom: 1px solid var(--n-border-color);
}
.editor-path {
  flex: 1;
  min-width: 0;
  font-size: 12px;
  opacity: 0.62;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.editor-body :deep(textarea) {
  font-family: ui-monospace, SFMono-Regular, Consolas, 'Courier New', monospace;
  font-size: 12.5px;
  line-height: 1.55;
  border-radius: 0;
}
.mono :deep(textarea) {
  font-family: ui-monospace, SFMono-Regular, Consolas, 'Courier New', monospace;
  font-size: 12.5px;
}
.ver-banner {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  padding: 14px 16px;
  margin-bottom: 12px;
  border: 1px solid var(--n-border-color);
  border-radius: 12px;
  background: rgba(127, 127, 127, 0.05);
}
.ver-list {
  list-style: none;
  margin: 0;
  padding: 0;
  border: 1px solid var(--n-border-color);
  border-radius: 12px;
  overflow: hidden;
}
.ver-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 11px 16px;
  border-bottom: 1px solid var(--n-border-color);
}
.ver-row:last-child {
  border-bottom: 0;
}
.ver-row.current {
  background: rgba(24, 160, 88, 0.08);
}
.ver-id {
  flex: 1;
  min-width: 0;
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
@media (max-width: 640px) {
  .hero {
    flex-wrap: wrap;
  }
  .sheet-row {
    grid-template-columns: 1fr;
  }
  .sheet-control {
    justify-content: flex-start;
  }
}
</style>
