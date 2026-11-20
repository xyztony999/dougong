<script setup lang="ts">
/**
 * 应用管理页（app-list hybrid，PageSpec app-mgmt v1.0）：页头就绪度状态条 +
 * 应用表格（操作列 admin only）+ 创建向导 Modal + 删除强确认 Modal + 运维
 * Drawer（概要/参数/日志——参数与日志 admin 专属 ⚠️A8）+ 任务 Drawer（双区，
 * running 5s 轮询终态即停，retry 按钮 retryable 态 D-13）；readonly 3 读+13
 * 负空间（ADR-018）。UI 文案人话化。
 */
import { computed, h, onMounted, onUnmounted, reactive, ref } from 'vue'
import { NAlert, NButton, NDataTable, NDrawer, NDrawerContent, NEmpty, NForm, NFormItem, NInput, NInputNumber, NModal, NPopconfirm, NSelect, NSwitch, NTabPane, NTabs, NTag, NTooltip, useMessage } from 'naive-ui'
import { storeToRefs } from 'pinia'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { appCreate, appDelete, appDepsReinstall, appEnvGet, appLogGet, appParamsSave, appServiceOp, appSiteLink, appTaskRetry } from '@/api/app'
import { APP_ERROR_TEXT } from '@/types/app'
import { APP_START_MODE, APP_TASK_ACTION, APP_TASK_STATUS } from '@/types/enums'
import type { AppItem, AppTaskDetailData } from '@/types/app'

const appStore = useAppStore()
const auth = useAuthStore()
const message = useMessage()
const { items, ready, generatedAt, loadError, tasks, currentTask } = storeToRefs(appStore)

const isAdmin = computed(() => auth.isAdmin)
const createShow = ref(false)
const deleteShow = ref(false)
const deleteTarget = ref<AppItem | null>(null)
const purgeDir = ref(false)
const deleteConfirmInput = ref('')
const opsShow = ref(false)
const opsTab = ref<'summary' | 'params' | 'logs'>('summary')
const opsApp = ref<AppItem | null>(null)
const opsAppId = ref(0)
const envLines = ref<string[]>([])
const envBase = ref<string | null>(null)
const presetModule = ref('app')
const presetApp = ref('app')
const customCommand = ref('')
const logLines = ref(200)
const logContent = ref('')
const tasksShow = ref(false)
const taskTab = ref<'history' | 'detail'>('history')
const createForm = reactive({ name: '', dir: '', entry: 'app.py', startMode: 'preset_fastapi' as 'preset_fastapi' | 'custom', listenPort: 8000, startNow: false })
const siteDomain = ref('')

const columns = [
  { title: '#', key: 'idx', width: 56, render: (_r: unknown, i: number) => `${i + 1}` },
  {
    title: '应用名', key: 'name', width: 160,
    render: (row: AppItem) =>
      isAdmin.value
        ? h('a', { class: 'app-link', onClick: () => openOps(row) }, row.name)
        : row.name
  },
  { title: '启动方式', key: 'startMode', width: 130, render: (r: AppItem) => APP_START_MODE[r.startMode]?.label ?? r.startMode },
  { title: '端口', key: 'listenPort', width: 80 },
  {
    title: '服务状态', key: 'svc', width: 120,
    render: (r: AppItem) => {
      if (r.unitFailed) return h(NTag, { type: 'warning' }, { default: () => '服务异常' })
      if (r.serviceRunning === null) return h(NTag, { type: 'default' }, { default: () => '未探测' })
      return h(NTag, { type: r.serviceRunning ? 'success' : 'default' }, { default: () => (r.serviceRunning ? '运行中' : '已停止') })
    }
  },
  {
    title: '开机自启', key: 'auto', width: 90,
    render: (r: AppItem) => {
      if (!isAdmin.value) {
        return h(NTag, { type: r.serviceEnabled ? 'success' : 'default' }, { default: () => (r.serviceEnabled ? '已自启' : '未自启') })
      }
      return h(NSwitch, {
        value: r.serviceEnabled === true,
        onUpdateValue: (v: boolean) => toggleAuto(r, v),
        disabled: r.runningTaskId !== null
      })
    }
  },
  {
    title: '依赖', key: 'deps', width: 90,
    render: (r: AppItem) => {
      if (!r.venvExists) return h(NTag, { type: 'warning' }, { default: () => '环境缺失' })
      if (r.depsInstalled === null) return '—'
      return h(NTag, { type: r.depsInstalled ? 'success' : 'default' }, { default: () => (r.depsInstalled ? '已安装' : '未安装') })
    }
  },
  {
    title: '任务', key: 'task', width: 100,
    render: (r: AppItem) =>
      r.runningTaskId !== null
        ? h('a', { class: 'app-link', onClick: () => openTask(r.runningTaskId as number) }, '任务进行中')
        : '—'
  },
  {
    title: '站点', key: 'site', width: 130,
    render: (r: AppItem) => {
      if (r.hasSite === true) return h('a', { class: 'app-link', href: '/sites', target: '_blank' }, '已建站')
      if (r.hasSite === false) return h(NTooltip, null, { trigger: () => '仅本机可访问', default: () => '可经「建站」反向代理对外暴露' })
      return '—'
    }
  },
  {
    title: '操作', key: 'ops', width: 300, fixed: 'right' as const,
    render: (r: AppItem) => {
      if (!isAdmin.value) return null
      const btns: ReturnType<typeof h>[] = []
      if (r.serviceRunning !== false && r.serviceRunning !== null) {
        btns.push(h(NPopconfirm, { onPositiveClick: () => doService(r, 'stop') }, { trigger: () => h(NButton, { size: 'small' }, { default: () => '停止' }), default: () => '停止该应用？' }))
        btns.push(h(NPopconfirm, { onPositiveClick: () => doService(r, 'restart') }, { trigger: () => h(NButton, { size: 'small' }, { default: () => '重启' }), default: () => '重启该应用？' }))
      } else {
        btns.push(h(NButton, { size: 'small', onClick: () => doService(r, 'start') }, { default: () => '启动' }))
      }
      btns.push(h(NPopconfirm, { onPositiveClick: () => doDeps(r) }, { trigger: () => h(NButton, { size: 'small', disabled: !r.venvExists || r.runningTaskId !== null }, { default: () => '重装依赖' }), default: () => '按 requirements.txt 重新安装依赖？完成后需重启应用生效。' }))
      btns.push(h(NButton, { size: 'small', onClick: () => openOps(r, 'params') }, { default: () => '参数' }))
      btns.push(h(NButton, { size: 'small', onClick: () => openOps(r, 'logs') }, { default: () => '日志' }))
      if (r.hasSite !== true) {
        btns.push(h(NButton, { size: 'small', onClick: () => openSite(r) }, { default: () => '建站' }))
      }
      btns.push(h(NButton, { size: 'small', type: 'error', disabled: r.runningTaskId !== null, onClick: () => openDelete(r) }, { default: () => '删除' }))
      return h('div', { class: 'ops' }, btns)
    }
  }
]

function errText(e: unknown): string {
  const raw = e instanceof Error ? e.message : String(e)
  for (const k of Object.keys(APP_ERROR_TEXT)) {
    if (raw.includes(k)) return APP_ERROR_TEXT[k] + (raw.split(k)[1] ?? '')
  }
  return raw
}

async function refresh() {
  await appStore.loadList()
}

async function doService(r: AppItem, op: 'start' | 'stop' | 'restart' | 'enable' | 'disable') {
  try {
    await appServiceOp(r.id, op)
    message.success(op === 'enable' || op === 'disable' ? '自启设置已更新' : '操作完成')
    await refresh()
  } catch (e) {
    message.error(errText(e))
  }
}

async function toggleAuto(r: AppItem, on: boolean) {
  try {
    await appServiceOp(r.id, on ? 'enable' : 'disable')
    message.success(on ? '已开启开机自启' : '已关闭开机自启')
    await refresh()
  } catch (e) {
    message.error(errText(e))
  }
}

async function doDeps(r: AppItem) {
  try {
    await appDepsReinstall(r.id)
    message.success('依赖安装任务已发起，完成后需重启应用生效')
    tasksShow.value = true
    await appStore.loadTasks()
    await refresh()
  } catch (e) {
    message.error(errText(e))
  }
}

async function submitCreate() {
  const startSpec =
    createForm.startMode === 'preset_fastapi'
      ? { module: presetModule.value, app: presetApp.value }
      : { command: customCommand.value }
  try {
    await appCreate({
      name: createForm.name,
      dir: createForm.dir,
      entry: createForm.entry,
      startMode: createForm.startMode,
      startSpec,
      listenPort: createForm.listenPort,
      startNow: createForm.startNow
    })
    message.success('创建任务已发起')
    createShow.value = false
    tasksShow.value = true
    await appStore.loadTasks()
    await refresh()
  } catch (e) {
    message.error(errText(e))
  }
}

function openDelete(r: AppItem) {
  deleteTarget.value = r
  purgeDir.value = false
  deleteConfirmInput.value = ''
  deleteShow.value = true
}

async function submitDelete() {
  if (!deleteTarget.value) return
  try {
    await appDelete({ appId: deleteTarget.value.id, purgeDir: purgeDir.value })
    message.success('删除任务已发起')
    deleteShow.value = false
    tasksShow.value = true
    await appStore.loadTasks()
    await refresh()
  } catch (e) {
    message.error(errText(e))
  }
}

function openOps(r: AppItem, tab: 'summary' | 'params' | 'logs' = 'summary') {
  opsApp.value = r
  opsAppId.value = r.id
  opsTab.value = tab
  opsShow.value = true
  if (tab === 'params') loadEnv()
  if (tab === 'logs') loadLog()
}

async function loadEnv() {
  try {
    const data = await appEnvGet(opsAppId.value)
    envLines.value = data.lines
    envBase.value = data.updatedAt
  } catch (e) {
    message.error(errText(e))
  }
}

async function saveParams() {
  try {
    const spec =
      opsApp.value?.startMode === 'preset_fastapi'
        ? { module: presetModule.value, app: presetApp.value }
        : opsApp.value?.startMode === 'custom'
          ? { command: customCommand.value }
          : undefined
    const out = await appParamsSave({
      appId: opsAppId.value,
      envLines: envLines.value,
      startSpec: spec,
      baseMtime: envBase.value ?? undefined
    })
    if (out.needRestart) message.success('已保存，需重启应用生效')
    await refresh()
  } catch (e) {
    message.error(errText(e))
  }
}

async function loadLog() {
  try {
    const data = await appLogGet(opsAppId.value, logLines.value)
    logContent.value = data.content
  } catch (e) {
    message.error(errText(e))
  }
}

function openSite(r: AppItem) {
  opsApp.value = r
  opsAppId.value = r.id
  siteDomain.value = ''
  opsShow.value = true
  opsTab.value = 'summary'
  message.info('填写域名后点「建站暴露」完成反向代理；域名与 HTTPS 可到站点管理配置')
}

async function submitSite() {
  try {
    const out = await appSiteLink(opsAppId.value, siteDomain.value || undefined)
    message.success(out.note)
    opsShow.value = false
    await refresh()
  } catch (e) {
    message.error(errText(e))
  }
}

function openTask(taskId: number) {
  tasksShow.value = true
  taskTab.value = 'detail'
  void appStore.loadTaskDetail(taskId)
}

const readyTag = computed(() => {
  if (!ready.value.installed) return { type: 'warning' as const, text: 'Python 未安装（到环境管理安装）' }
  if (ready.value.venvSupport === false || ready.value.pipAvailable === false)
    return { type: 'warning' as const, text: `Python 环境不完整：缺 ${ready.value.venvSupport === false ? 'venv' : ''}${ready.value.pipAvailable === false ? ' pip' : ''}` }
  return { type: 'success' as const, text: `Python 环境就绪${ready.value.version ? '（' + ready.value.version + '）' : ''}` }
})

function pollCleanup() {
  appStore.stopPolling()
}
onMounted(() => {
  void refresh()
})
onUnmounted(pollCleanup)
</script>

<template>
  <div class="page">
    <div class="head">
      <NTag :type="readyTag.type">{{ readyTag.text }}</NTag>
      <NTag v-if="!ready.installed || ready.venvSupport === false" type="warning" :bordered="false">
        <router-link to="/env">到环境管理安装</router-link>
      </NTag>
      <span class="gen">探测于 {{ generatedAt }}</span>
      <NButton size="small" @click="refresh">刷新</NButton>
      <NButton v-if="isAdmin" size="small" type="primary" @click="createShow = true">创建应用</NButton>
    </div>

    <NAlert v-if="loadError" type="error" class="load-err">{{ loadError }} <NButton size="tiny" @click="refresh">重试</NButton></NAlert>

    <NDataTable :columns="columns" :data="items" :row-key="(r: AppItem) => r.id" />

    <NEmpty v-if="items.length === 0 && !loadError" description="还没有应用，点击「创建应用」开始" />

    <!-- 创建向导（两分组：基本信息/启动配置；mode 条件显隐） -->
    <NModal v-model:show="createShow" preset="card" title="创建应用" style="width: 560px">
      <NForm label-placement="left" label-width="110">
        <div class="group-title">基本信息</div>
        <NFormItem label="应用名"><NInput v-model:value="createForm.name" placeholder="小写字母、数字、连字符（不超过 32 个字符）" /></NFormItem>
        <NFormItem label="代码目录"><NInput v-model:value="createForm.dir" placeholder="/opt/myapp（目录须已存在）" /></NFormItem>
        <NFormItem label="入口文件"><NInput v-model:value="createForm.entry" placeholder="app.py（相对代码目录）" /></NFormItem>
        <NFormItem label="监听端口"><NInputNumber v-model:value="createForm.listenPort" :min="1024" :max="65535" /><span class="hint">默认仅本机可访问</span></NFormItem>
        <div class="group-title">启动配置</div>
        <NFormItem label="启动方式">
          <NSelect v-model:value="createForm.startMode" :options="[
            { label: 'FastAPI 预设（uvicorn）', value: 'preset_fastapi' },
            { label: '自定义命令', value: 'custom' }
          ]" />
        </NFormItem>
        <NFormItem v-if="createForm.startMode === 'preset_fastapi'" label="模块:实例"><NInput v-model:value="presetModule" placeholder="app" /><NInput v-model:value="presetApp" placeholder="app" /></NFormItem>
        <NFormItem v-if="createForm.startMode === 'custom'" label="启动命令"><NInput v-model:value="customCommand" placeholder=".venv/bin/你的启动命令（须以 .venv/bin/ 开头）" /></NFormItem>
        <NFormItem label="立即启动"><NSwitch v-model:value="createForm.startNow" /></NFormItem>
      </NForm>
      <template #footer>
        <NButton @click="createShow = false">取消</NButton>
        <NButton type="primary" @click="submitCreate">发起创建</NButton>
      </template>
    </NModal>

    <!-- 删除强确认（两段警示 + purgeDir 默认关 + 输入锁定 ⚠️A5） -->
    <NModal v-model:show="deleteShow" preset="card" title="删除应用" style="width: 520px">
      <NAlert type="warning">将停止服务并移除系统服务条目；代码目录默认保留。</NAlert>
      <div class="purge"><NSwitch v-model:value="purgeDir" /> 同时删除代码目录（含运行环境，不可恢复）</div>
      <NAlert type="info">已关联的站点不会自动删除，请到站点管理自行处置。</NAlert>
      <NFormItem label="输入应用名确认"><NInput v-model:value="deleteConfirmInput" :placeholder="deleteTarget?.name" /></NFormItem>
      <template #footer>
        <NButton @click="deleteShow = false">取消</NButton>
        <NButton type="error" :disabled="deleteConfirmInput !== deleteTarget?.name" @click="submitDelete">确认删除</NButton>
      </template>
    </NModal>

    <!-- 应用运维 Drawer（概要/参数/日志——参数与日志 admin 专属 ⚠️A8） -->
    <NDrawer v-model:show="opsShow" :width="560">
      <NDrawerContent :title="`应用：${opsApp?.name ?? ''}`">
        <NTabs v-model:value="opsTab">
          <NTabPane name="summary" tab="概要">
            <div v-if="opsApp" class="summary">
              <div>代码目录：{{ opsApp.dir }}</div>
              <div>入口文件：{{ opsApp.entry }}</div>
              <div>监听端口：{{ opsApp.listenPort }}（127.0.0.1）</div>
              <div>依赖就绪：{{ opsApp.venvExists ? 'venv ✓' : 'venv 缺失' }} / {{ opsApp.depsInstalled ? '依赖 ✓' : '依赖未装' }}</div>
              <div>端口监听：{{ opsApp.portListening === null ? '未探测' : opsApp.portListening ? '是' : '否' }}</div>
              <div v-if="isAdmin" class="site-row">
                <NInput v-model:value="siteDomain" placeholder="域名（可选，如 app.example.com）" />
                <NButton size="small" type="primary" @click="submitSite">建站暴露</NButton>
              </div>
              <div v-else class="hint">仅本机可访问；如需对外暴露请联系管理员</div>
            </div>
          </NTabPane>
          <NTabPane v-if="isAdmin" name="params" tab="参数">
            <NAlert type="info" :bordered="false">环境变量与应用参数保存后需重启应用生效。</NAlert>
            <div class="env-editor">
              <div v-for="(_line, i) in envLines" :key="i" class="env-line"><NInput v-model:value="envLines[i]" size="small" /></div>
              <NButton size="small" dashed @click="envLines.push('')">+ 变量</NButton>
            </div>
            <NButton type="primary" @click="saveParams">保存参数</NButton>
          </NTabPane>
          <NTabPane v-if="isAdmin" name="logs" tab="日志">
            <div class="log-head">
              <NSelect v-model:value="logLines" size="small" style="width: 120px" :options="[
                { label: '200 行', value: 200 },
                { label: '500 行', value: 500 },
                { label: '1000 行', value: 1000 }
              ]" @update:value="loadLog" />
              <NButton size="small" @click="loadLog">刷新</NButton>
            </div>
            <pre class="log">{{ logContent }}</pre>
          </NTabPane>
        </NTabs>
      </NDrawerContent>
    </NDrawer>

    <!-- 任务 Drawer（历史/详情双区；running 5s 轮询终态即停） -->
    <NDrawer v-model:show="tasksShow" :width="620">
      <NDrawerContent title="应用任务">
        <NTabs v-model:value="taskTab">
          <NTabPane name="history" tab="任务">
            <NDataTable
              :columns="[
                { title: 'ID', key: 'id', width: 64 },
                { title: '应用', key: 'appName', width: 120 },
                { title: '动作', key: 'action', width: 100, render: (r: AppTaskDetailData) => APP_TASK_ACTION[r.action]?.label ?? r.action },
                { title: '状态', key: 'status', width: 90, render: (r: AppTaskDetailData) => h(NTag, { type: APP_TASK_STATUS[r.status]?.tag as never }, { default: () => APP_TASK_STATUS[r.status]?.label ?? r.status }) },
                { title: '发起时间', key: 'createdAt', width: 170 }
              ]"
              :data="tasks"
              :row-key="(r: AppTaskDetailData) => r.id"
              @update:current-row="(r: AppTaskDetailData) => appStore.loadTaskDetail(r.id)"
            />
            <NButton size="small" @click="appStore.loadTasks()">刷新</NButton>
          </NTabPane>
          <NTabPane name="detail" tab="详情">
            <div v-if="currentTask">
              <div class="summary">
                <div>动作：{{ APP_TASK_ACTION[currentTask.action]?.label ?? currentTask.action }}</div>
                <div>状态：<NTag :type="APP_TASK_STATUS[currentTask.status]?.tag as never">{{ APP_TASK_STATUS[currentTask.status]?.label ?? currentTask.status }}</NTag></div>
                <div v-if="currentTask.error">失败原因：<NAlert type="error">{{ currentTask.error }}</NAlert></div>
              </div>
              <NButton v-if="isAdmin && currentTask.retryable" size="small" type="warning" @click="() => { appTaskRetry(currentTask!.id); appStore.loadTaskDetail(currentTask!.id) }">重试</NButton>
              <pre class="log">{{ currentTask.outputTail }}</pre>
            </div>
            <NEmpty v-else description="从任务列表选择一条任务" />
          </NTabPane>
        </NTabs>
      </NDrawerContent>
    </NDrawer>
  </div>
</template>

<style scoped>
.head { display: flex; align-items: center; gap: 12px; margin-bottom: 12px; }
.gen { color: var(--text-color-3, #999); font-size: 12px; }
.page { padding: 4px 0; }
.load-err { margin-bottom: 12px; }
.ops { display: flex; gap: 4px; flex-wrap: wrap; }
.app-link { color: var(--primary-color, #18a058); cursor: pointer; }
.group-title { font-weight: 600; margin: 8px 0; }
.hint { color: var(--text-color-3, #999); font-size: 12px; margin-left: 8px; }
.purge { margin: 8px 0; display: flex; align-items: center; gap: 8px; }
.summary { display: flex; flex-direction: column; gap: 6px; }
.env-editor { display: flex; flex-direction: column; gap: 4px; margin: 8px 0; }
.env-line { display: flex; }
.log-head { display: flex; gap: 8px; margin-bottom: 8px; }
.log { background: rgba(128, 128, 128, 0.08); padding: 8px; max-height: 360px; overflow: auto; font-size: 12px; white-space: pre-wrap; }
.site-row { display: flex; gap: 8px; align-items: center; }
</style>
