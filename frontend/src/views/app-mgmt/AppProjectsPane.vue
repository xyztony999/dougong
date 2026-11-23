<script setup lang="ts">
/**
 * 运行时项目面板（PageSpec app-mgmt v1.1，ADR-033）：按 runtime 属性实例化——
 * 网站管理页的「Python 项目」「Node 项目」分栏各挂一份。就绪度状态条 + 项目表格
 * （操作列 admin only）+ 创建表单 + 删除强确认 + 运维 Drawer（概要/参数/日志——
 * 参数与日志 admin 专属 ⚠️A8）+ 任务 Drawer（running 5s 轮询终态即停，retry 按钮
 * retryable 态 D-13）；readonly 3 读+13 负空间（ADR-018）。
 *
 * Python 项目：venv + 系统服务（既有行为）。Node 项目：由 PM2 托管进程——状态、
 * 重启次数、内存来自 PM2；开机自启开关开启会同时启动应用、关闭会同时停止应用
 * （PM2 只能用「保存时的运行状态」表达这个设置，所以切换前二次确认）。
 * UI 文案人话化。
 */
import { computed, h, onMounted, onUnmounted, reactive, ref } from 'vue'
import { NAlert, NButton, NDataTable, NDrawer, NDrawerContent, NEmpty, NForm, NFormItem, NInput, NInputNumber, NModal, NPopconfirm, NSelect, NSwitch, NTabPane, NTabs, NTag, NTooltip, useMessage } from 'naive-ui'
import { storeToRefs } from 'pinia'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { appCreate, appDelete, appDepsReinstall, appEnvGet, appLogGet, appParamsSave, appServiceOp, appSiteLink, appTaskRetry } from '@/api/app'
import { APP_ERROR_TEXT } from '@/types/app'
import { APP_START_MODE, APP_RUNTIME, APP_TASK_ACTION, APP_TASK_STATUS } from '@/types/enums'
import type { AppItem, AppStartSpec, AppTaskDetailData } from '@/types/app'
import type { AppRuntime, AppStartMode } from '@/types/enums'

const props = defineProps<{ runtime: AppRuntime }>()

const appStore = useAppStore(props.runtime)
const auth = useAuthStore()
const message = useMessage()
const { items, ready, nodeReady, generatedAt, loading, loaded, loadError, tasks, currentTask } = storeToRefs(appStore)

const isNode = computed(() => props.runtime === 'node')
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
const logLines = ref(200)
const logContent = ref('')
const tasksShow = ref(false)
const taskTab = ref<'history' | 'detail'>('history')
const siteDomain = ref('')

/** 启动参数的输入值（创建表单与参数页签共用一套形状；参数页签打开时按应用实际保存的值预填） */
const specInput = reactive({ module: 'app', app: 'app', args: '', command: '', file: '', script: 'start' })

const createForm = reactive<{ name: string; dir: string; entry: string; startMode: AppStartMode; listenPort: number; startNow: boolean }>({
  name: '', dir: '', entry: isNode.value ? '' : 'app.py',
  startMode: isNode.value ? 'node_file' : 'preset_fastapi',
  listenPort: isNode.value ? 3000 : 8000, startNow: false
})

const startModeOptions = computed(() =>
  isNode.value
    ? [
        { label: '启动文件（如 server.js）', value: 'node_file' },
        { label: 'npm 脚本（npm run <脚本名>）', value: 'npm_script' }
      ]
    : [
        { label: 'FastAPI 预设（uvicorn）', value: 'preset_fastapi' },
        { label: '自定义命令', value: 'custom' }
      ]
)

/** buildStartSpec 按启动方式取对应字段（其余字段不下发）。必填项为空时返回
 *  undefined——参数页签只改变量时不拿空的启动参数去覆盖已保存的值。 */
function buildStartSpec(mode: AppStartMode, v: typeof specInput, entry: string): AppStartSpec | undefined {
  switch (mode) {
    case 'preset_fastapi':
      return v.module.trim() && v.app.trim() ? { module: v.module.trim(), app: v.app.trim(), args: v.args.trim() } : undefined
    case 'custom':
      return v.command.trim() ? { command: v.command.trim() } : undefined
    case 'node_file':
      return entry.trim() ? { file: entry.trim() } : undefined
    case 'npm_script':
      return v.script.trim() ? { script: v.script.trim() } : undefined
  }
}

/** PM2 状态呈现（原值透传；null=进程未注册） */
const PM2_STATUS_TEXT: Record<string, { text: string; type: 'success' | 'default' | 'error' | 'warning' }> = {
  online: { text: '运行中', type: 'success' },
  stopped: { text: '已停止', type: 'default' },
  errored: { text: '异常', type: 'error' },
  restarting: { text: '重启中', type: 'warning' }
}

function formatBytes(n: number | null): string {
  if (n === null) return '—'
  if (n >= 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`
  return `${Math.max(1, Math.round(n / 1024))} KB`
}

const columns = computed(() => [
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
      title: '服务状态', key: 'svc', width: 110,
      render: (r: AppItem) => {
        if (isNode.value) {
          if (r.pm2Status === null) return h(NTag, { type: 'default' }, { default: () => '未注册' })
          const st = PM2_STATUS_TEXT[r.pm2Status]
          return h(NTag, { type: st?.type ?? 'default' }, { default: () => st?.text ?? r.pm2Status })
        }
        if (r.unitFailed) return h(NTag, { type: 'warning' }, { default: () => '服务异常' })
        if (r.serviceRunning === null) return h(NTag, { type: 'default' }, { default: () => '未探测' })
        return h(NTag, { type: r.serviceRunning ? 'success' : 'default' }, { default: () => (r.serviceRunning ? '运行中' : '已停止') })
      }
    },
    ...(isNode.value
      ? [{
          title: 'PM2', key: 'pm2', width: 150,
          render: (r: AppItem) => (r.pm2Status === null ? '—' : `重启 ${r.restarts ?? 0} 次 · ${formatBytes(r.memoryBytes)}`)
        }]
      : []),
    {
      title: '开机自启', key: 'auto', width: 90,
      render: (r: AppItem) => {
        if (!isAdmin.value) {
          return h(NTag, { type: r.serviceEnabled ? 'success' : 'default' }, { default: () => (r.serviceEnabled ? '已自启' : '未自启') })
        }
        const sw = () => h(NSwitch, { value: r.serviceEnabled === true, disabled: r.runningTaskId !== null })
        if (isNode.value) {
          // Node 项目：开关会同时启动/停止应用——切换前二次确认。
          const on = r.serviceEnabled === true
          return h(NPopconfirm, { onPositiveClick: () => toggleAuto(r, !on) }, {
            trigger: sw,
            default: () => (on ? '关闭开机自启会同时停止应用。确定关闭？' : '开启开机自启会同时启动应用。确定开启？')
          })
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
        if (isNode.value) {
          if (r.depsInstalled === null) return '—' // 项目里没有 package.json
          return h(NTag, { type: r.depsInstalled ? 'success' : 'default' }, { default: () => (r.depsInstalled ? '已安装' : '未安装') })
        }
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
        if (r.hasSite === true) return h('a', { class: 'app-link', href: '/sites?tab=site', target: '_blank' }, '已建站')
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
        const depsDisabled = isNode.value ? r.depsInstalled === null || r.runningTaskId !== null : !r.venvExists || r.runningTaskId !== null
        btns.push(h(NPopconfirm, { onPositiveClick: () => doDeps(r) }, {
          trigger: () => h(NButton, { size: 'small', disabled: depsDisabled }, { default: () => '重装依赖' }),
          default: () => (isNode.value ? '按 package.json 重新安装依赖（npm install）？完成后需重启应用生效。' : '按 requirements.txt 重新安装依赖？完成后需重启应用生效。')
        }))
        btns.push(h(NButton, { size: 'small', onClick: () => openOps(r, 'params') }, { default: () => '参数' }))
        btns.push(h(NButton, { size: 'small', onClick: () => openOps(r, 'logs') }, { default: () => '日志' }))
        if (r.hasSite !== true) {
          btns.push(h(NButton, { size: 'small', onClick: () => openSite(r) }, { default: () => '建站' }))
        }
        btns.push(h(NButton, { size: 'small', type: 'error', disabled: r.runningTaskId !== null, onClick: () => openDelete(r) }, { default: () => '删除' }))
        return h('div', { class: 'ops' }, btns)
      }
    }
  ])

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

/** 启动参数必填项缺失时的提示（创建表单前端预检；后端仍按白名单权威校验） */
function specMissingHint(mode: AppStartMode): string {
  switch (mode) {
    case 'preset_fastapi':
      return '请填写模块与实例名（如 app:app）'
    case 'custom':
      return '请填写启动命令'
    case 'node_file':
      return '请填写启动文件（项目内的相对路径，如 server.js）'
    case 'npm_script':
      return '请填写 npm 脚本名（如 start）'
  }
}

async function submitCreate() {
  const npmScriptMode = createForm.startMode === 'npm_script'
  const spec = buildStartSpec(createForm.startMode, specInput, createForm.entry)
  if (!spec) {
    message.warning(specMissingHint(createForm.startMode))
    return
  }
  try {
    await appCreate({
      name: createForm.name,
      dir: createForm.dir,
      // npm 脚本方式不需要入口文件（启动目标是 package.json 里的脚本）
      entry: npmScriptMode ? '' : createForm.entry,
      runtime: props.runtime,
      startMode: createForm.startMode,
      startSpec: spec,
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
  if (tab === 'params') {
    // 启动参数按应用实际保存的值预填（清单随带 startSpec）——不拿表单里上次
    // 输入的内容顶替，否则保存参数会悄悄改掉应用真正的启动参数。
    const sp = r.startSpec
    specInput.module = sp?.module ?? ''
    specInput.app = sp?.app ?? ''
    specInput.args = sp?.args ?? ''
    specInput.command = sp?.command ?? ''
    specInput.file = sp?.file ?? ''
    specInput.script = sp?.script ?? 'start'
    loadEnv()
  }
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
  if (!opsApp.value) return
  try {
    const spec = buildStartSpec(opsApp.value.startMode, specInput, specInput.file)
    const out = await appParamsSave({
      appId: opsAppId.value,
      envLines: envLines.value,
      startSpec: spec,
      baseMtime: envBase.value ?? undefined
    })
    if (out.needRestart) message.success('已保存，需重启应用生效')
    else message.success('已保存，进程已按新的启动参数重新注册')
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

/** retryCurrent 重试当前查看的失败任务（失败原因如实提示，不静默） */
async function retryCurrent() {
  const task = currentTask.value
  if (!task) return
  try {
    await appTaskRetry(task.id)
    message.success('已重新发起任务')
    await appStore.loadTaskDetail(task.id)
  } catch (e) {
    message.error(errText(e))
  }
}

/** Node 环境缺失项（逐项列出，引导环境管理） */
const nodeMissing = computed(() => {
  const m: string[] = []
  if (!nodeReady.value.nodeAvailable) m.push('node')
  if (!nodeReady.value.npmAvailable) m.push('npm')
  if (!nodeReady.value.pm2Available) m.push('PM2')
  return m
})

const readyTag = computed(() => {
  // 首次加载完成前就绪度只是初始值——显示「检测中」，不说环境缺失。
  if (!loaded.value) return { type: 'default' as const, text: '检测中' }
  if (isNode.value) {
    if (nodeMissing.value.length > 0) return { type: 'warning' as const, text: `Node 环境不完整：缺 ${nodeMissing.value.join('、')}` }
    return { type: 'success' as const, text: `Node 环境就绪${nodeReady.value.nodeVersion ? '（' + nodeReady.value.nodeVersion + '）' : ''}` }
  }
  if (!ready.value.installed) return { type: 'warning' as const, text: 'Python 未安装（到环境管理安装）' }
  if (ready.value.venvSupport === false || ready.value.pipAvailable === false) {
    return { type: 'warning' as const, text: `Python 环境不完整：缺 ${ready.value.venvSupport === false ? 'venv' : ''}${ready.value.pipAvailable === false ? ' pip' : ''}` }
  }
  return { type: 'success' as const, text: `Python 环境就绪${ready.value.version ? '（' + ready.value.version + '）' : ''}` }
})

const needEnvGuide = computed(() =>
  loaded.value &&
  (isNode.value
    ? nodeMissing.value.length > 0
    : !ready.value.installed || ready.value.venvSupport === false || ready.value.pipAvailable === false)
)

const runtimeLabel = computed(() => APP_RUNTIME[props.runtime].label)

onMounted(() => {
  void refresh()
})
onUnmounted(() => {
  appStore.stopPolling()
})
</script>

<template>
  <div class="pane">
    <div class="head">
      <NTag :type="readyTag.type">{{ readyTag.text }}</NTag>
      <NTag v-if="needEnvGuide" type="warning" :bordered="false">
        <router-link to="/env">{{ isNode ? '到环境管理安装或升级' : '到环境管理安装' }}</router-link>
      </NTag>
      <span v-if="generatedAt" class="gen">探测于 {{ generatedAt }}</span>
      <NButton size="small" :loading="loading" @click="refresh">刷新</NButton>
      <NButton v-if="isAdmin" size="small" type="primary" @click="createShow = true">创建{{ runtimeLabel }}项目</NButton>
    </div>

    <NAlert v-if="isNode" type="info" :bordered="false" class="pm2-hint">
      Node 项目由 PM2 托管进程，可在服务器上用 pm2 list 查看。手动启停不影响开机自启设置。
    </NAlert>

    <NAlert v-if="loadError" type="error" class="load-err">{{ loadError }} <NButton size="tiny" @click="refresh">重试</NButton></NAlert>

    <NDataTable :columns="columns" :data="items" :loading="loading" :row-key="(r: AppItem) => r.id">
      <template #empty>
        <div class="tbl-empty">
          <NEmpty :description="loaded ? `还没有${runtimeLabel}项目` : '未能读取项目列表'" />
          <NButton v-if="isAdmin && !loadError" size="small" type="primary" @click="createShow = true">创建{{ runtimeLabel }}项目</NButton>
        </div>
      </template>
    </NDataTable>

    <!-- 创建表单（两分组：基本信息/启动配置；启动方式随运行时） -->
    <NModal v-model:show="createShow" preset="card" :title="`创建${runtimeLabel}项目`" style="width: 560px">
      <NForm label-placement="left" label-width="110">
        <div class="group-title">基本信息</div>
        <NFormItem label="应用名"><NInput v-model:value="createForm.name" placeholder="小写字母、数字、连字符（不超过 32 个字符）" /></NFormItem>
        <NFormItem label="代码目录"><NInput v-model:value="createForm.dir" placeholder="/opt/myapp（目录须已存在）" /></NFormItem>
        <NFormItem v-if="!(isNode && createForm.startMode === 'npm_script')" :label="isNode ? '启动文件' : '入口文件'">
          <NInput v-model:value="createForm.entry" :placeholder="isNode ? 'server.js（相对代码目录，可含子目录如 bin/www）' : 'app.py（相对代码目录）'" />
        </NFormItem>
        <NFormItem label="监听端口"><NInputNumber v-model:value="createForm.listenPort" :min="1024" :max="65535" /><span class="hint">默认仅本机可访问</span></NFormItem>
        <div class="group-title">启动配置</div>
        <NFormItem label="启动方式">
          <NSelect v-model:value="createForm.startMode" :options="startModeOptions" />
        </NFormItem>
        <NFormItem v-if="createForm.startMode === 'preset_fastapi'" label="模块:实例"><NInput v-model:value="specInput.module" placeholder="app" /><NInput v-model:value="specInput.app" placeholder="app" /></NFormItem>
        <NFormItem v-if="createForm.startMode === 'custom'" label="启动命令"><NInput v-model:value="specInput.command" placeholder=".venv/bin/你的启动命令（须以 .venv/bin/ 开头）" /></NFormItem>
        <NFormItem v-if="createForm.startMode === 'npm_script'" label="脚本名"><NInput v-model:value="specInput.script" placeholder="start" /></NFormItem>
        <NAlert v-if="isNode" type="info" :bordered="false">项目里有 package.json 时会自动安装依赖。</NAlert>
        <NFormItem label="立即启动"><NSwitch v-model:value="createForm.startNow" /></NFormItem>
      </NForm>
      <template #footer>
        <NButton @click="createShow = false">取消</NButton>
        <NButton type="primary" @click="submitCreate">发起创建</NButton>
      </template>
    </NModal>

    <!-- 删除强确认（两段警示 + purgeDir 默认关 + 输入锁定 ⚠️A5） -->
    <NModal v-model:show="deleteShow" preset="card" title="删除应用" style="width: 520px">
      <NAlert type="warning">{{ isNode ? '将从 PM2 注销进程并移除应用条目；代码目录默认保留。' : '将停止服务并移除系统服务条目；代码目录默认保留。' }}</NAlert>
      <div class="purge"><NSwitch v-model:value="purgeDir" /> 同时删除代码目录（含运行环境，不可恢复）</div>
      <NAlert type="info">已关联的站点不会自动删除，请到站点管理自行处置。</NAlert>
      <NFormItem label="输入应用名确认"><NInput v-model:value="deleteConfirmInput" :placeholder="deleteTarget?.name" /></NFormItem>
      <template #footer>
        <NButton @click="deleteShow = false">取消</NButton>
        <NButton type="error" :disabled="deleteConfirmInput !== deleteTarget?.name" @click="submitDelete">确认删除</NButton>
      </template>
    </NModal>

    <!-- 运维 Drawer（概要/参数/日志——参数与日志 admin 专属 ⚠️A8） -->
    <NDrawer v-model:show="opsShow" :width="560">
      <NDrawerContent :title="`应用：${opsApp?.name ?? ''}`">
        <NTabs v-model:value="opsTab">
          <NTabPane name="summary" tab="概要">
            <div v-if="opsApp" class="summary">
              <div>代码目录：{{ opsApp.dir }}</div>
              <div>启动方式：{{ APP_START_MODE[opsApp.startMode]?.label ?? opsApp.startMode }}<template v-if="opsApp.startMode === 'npm_script'">（npm run {{ opsApp.startSpec?.script ?? 'start' }}）</template><template v-else>（{{ opsApp.entry || opsApp.startSpec?.file || '—' }}）</template></div>
              <div>监听端口：{{ opsApp.listenPort }}（127.0.0.1）</div>
              <template v-if="isNode">
                <div>进程状态：{{ opsApp.pm2Status === null ? '未注册' : (PM2_STATUS_TEXT[opsApp.pm2Status]?.text ?? opsApp.pm2Status) }}</div>
                <div>重启次数：{{ opsApp.restarts ?? '—' }} · 内存占用：{{ formatBytes(opsApp.memoryBytes) }}</div>
                <div>依赖：{{ opsApp.depsInstalled === null ? '无 package.json' : opsApp.depsInstalled ? '已安装' : '未安装' }}</div>
              </template>
              <template v-else>
                <div>依赖就绪：{{ opsApp.venvExists ? 'venv ✓' : 'venv 缺失' }} / {{ opsApp.depsInstalled ? '依赖 ✓' : '依赖未装' }}</div>
              </template>
              <div>端口监听：{{ opsApp.portListening === null ? '未探测' : opsApp.portListening ? '是' : '否' }}</div>
              <div v-if="isAdmin" class="site-row">
                <NInput v-model:value="siteDomain" placeholder="域名（可选，如 app.example.com）" />
                <NButton size="small" type="primary" @click="submitSite">建站暴露</NButton>
              </div>
              <div v-else class="hint">仅本机可访问；如需对外暴露请联系管理员</div>
            </div>
          </NTabPane>
          <NTabPane v-if="isAdmin" name="params" tab="参数">
            <NAlert type="info" :bordered="false">{{ isNode ? '环境变量保存后需重启应用生效；修改启动方式会立即重新注册进程（运行中的应用会重启）。' : '环境变量与应用参数保存后需重启应用生效。' }}</NAlert>
            <div class="group-title">启动参数</div>
            <NFormItem v-if="opsApp?.startMode === 'preset_fastapi'" label="模块:实例"><NInput v-model:value="specInput.module" placeholder="app" /><NInput v-model:value="specInput.app" placeholder="app" /></NFormItem>
            <NFormItem v-else-if="opsApp?.startMode === 'custom'" label="启动命令"><NInput v-model:value="specInput.command" placeholder=".venv/bin/你的启动命令（须以 .venv/bin/ 开头）" /></NFormItem>
            <NFormItem v-else-if="opsApp?.startMode === 'node_file'" label="启动文件"><NInput v-model:value="specInput.file" placeholder="server.js" /></NFormItem>
            <NFormItem v-else-if="opsApp?.startMode === 'npm_script'" label="脚本名"><NInput v-model:value="specInput.script" placeholder="start" /></NFormItem>
            <div class="group-title">环境变量（.env，每行一个 KEY=VALUE）</div>
            <div class="env-editor">
              <div v-for="(_line, i) in envLines" :key="i" class="env-line"><NInput v-model:value="envLines[i]" size="small" /></div>
              <NButton size="small" dashed @click="envLines.push('')">+ 变量</NButton>
            </div>
            <NButton type="primary" @click="saveParams">保存参数</NButton>
          </NTabPane>
          <NTabPane v-if="isAdmin" name="logs" tab="日志">
            <NAlert v-if="isNode" type="info" :bordered="false">日志由 PM2 落盘，随运行持续增长，面板不做自动清理。</NAlert>
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
              <NButton v-if="isAdmin && currentTask.retryable" size="small" type="warning" @click="retryCurrent">重试</NButton>
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
.pane { padding: 4px 0; }
.pm2-hint { margin-bottom: 12px; }
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
.tbl-empty { padding: 36px 0; display: flex; flex-direction: column; align-items: center; gap: 12px; }
</style>
