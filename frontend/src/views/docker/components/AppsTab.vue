<script setup lang="ts">
/**
 * 容器管理「应用」页签（PageSpec docker-mgmt v1.2，ADR-033 S-62 面板应用）：
 * 应用清单（落盘扫描 + 容器实况，两角色可看）+ 工具条「创建应用」（admin 专属）
 * + 行内 启动/停止/重启/日志/建站/删除（admin；readonly 操作列整列不渲染——
 * ADR-018 负空间）。应用=面板生成的单服务编排，主机端口只绑本机（建站后对外）。
 * 创建失败保留弹窗并显示原因（沿拉取镜像弹窗口径）。UI 文案人话化。
 */
import { computed, h, onMounted, reactive, ref } from 'vue'
import { NAlert, NButton, NDataTable, NForm, NFormItem, NInput, NInputNumber, NModal, NPopconfirm, NSelect, NTag, NTooltip, useMessage } from 'naive-ui'
import type { DataTableColumns, SelectOption } from 'naive-ui'
import { storeToRefs } from 'pinia'
import { ApiError } from '@/api/http'
import { appCreate, appDelete, appLogs, appService, appSite } from '@/api/docker'
import type { DockerAppItem } from '@/types/docker'
import { DK_ERROR_TEXT, dockerErrorReason } from '@/types/docker'
import { DOCKER_APP_STATE } from '@/types/enums'
import { useDockerAppsStore } from '@/stores/docker-apps'
import { useAuthStore } from '@/stores/auth'
import { formatDateTime } from '@/utils/format'

const store = useDockerAppsStore()
const authStore = useAuthStore()
const message = useMessage()
const { items, total, generatedAt, loading, loadError } = storeToRefs(store)

const isAdmin = computed(() => authStore.isAdmin)
const generatedAtText = computed(() => formatDateTime(generatedAt.value))

/** 行级在途互斥（同一应用的按钮 loading/禁用，防重复提交） */
const inFlight = ref<ReadonlySet<string>>(new Set<string>())
function setBusy(name: string) {
  inFlight.value = new Set(inFlight.value).add(name)
}
function releaseBusy(name: string) {
  const next = new Set(inFlight.value)
  next.delete(name)
  inFlight.value = next
}

/** 统一错误文案：错误码文案 + 服务端原因（DK_030/032 等透传 data.reason） */
function errText(e: unknown): string {
  if (!(e instanceof ApiError)) return e instanceof Error ? e.message : String(e)
  const text = DK_ERROR_TEXT[e.code] ?? e.code
  const reason = dockerErrorReason(e.data)
  return reason !== null ? `${text}：${reason}` : text
}

/** daemon 不可用兜底：提示（页头状态由外层 store 的下次拉取呈现） */
function handleWriteError(e: unknown): void {
  if (e instanceof ApiError && e.is('DK_002')) {
    message.warning(DK_ERROR_TEXT['DK_002'])
    return
  }
  message.error(errText(e))
}

// ---- 创建应用弹窗 ----

const createShow = ref(false)
const createError = ref<string | null>(null)
const createSubmitting = ref(false)
/** 创建表单模型：env / mounts 在表单里恒为数组（接口上可选，空数组即不下发内容） */
interface AppCreateForm {
  name: string
  image: string
  hostPort: number
  containerPort: number
  env: { key: string; value: string }[]
  mounts: { source: string; target: string; mode?: string }[]
  restartPolicy: string
}

const createForm = reactive<AppCreateForm>({
  name: '',
  image: '',
  hostPort: 18080,
  containerPort: 80,
  env: [{ key: '', value: '' }],
  mounts: [{ source: '', target: '' }],
  restartPolicy: 'unless-stopped'
})

const restartPolicyOptions: SelectOption[] = [
  { label: '除非手动停止', value: 'unless-stopped' },
  { label: '总是重启', value: 'always' },
  { label: '失败时重启', value: 'on-failure' },
  { label: '不重启', value: 'no' }
]

function openCreate(): void {
  createError.value = null
  createShow.value = true
}

async function submitCreate(): Promise<void> {
  createError.value = null
  if (!createForm.name.trim() || !createForm.image.trim()) {
    createError.value = '请填写应用名和镜像'
    return
  }
  createSubmitting.value = true
  try {
    const data = await appCreate({
      name: createForm.name.trim(),
      image: createForm.image.trim(),
      hostPort: createForm.hostPort,
      containerPort: createForm.containerPort,
      // 空行提交前过滤
      env: createForm.env.filter((v) => v.key.trim() !== ''),
      mounts: createForm.mounts.filter((m) => m.source.trim() !== '' || m.target.trim() !== ''),
      restartPolicy: createForm.restartPolicy
    })
    message.success(`应用已创建并启动：${data.name}`)
    createShow.value = false
    createForm.name = ''
    createForm.image = ''
    await store.load()
  } catch (e) {
    // 字段/端口/重名/启动失败：保留弹窗并显示原因，供修正后重试
    createError.value = errText(e)
  } finally {
    createSubmitting.value = false
  }
}

// ---- 行内操作 ----

type AppOp = 'start' | 'stop' | 'restart'

async function execService(row: DockerAppItem, op: AppOp): Promise<void> {
  setBusy(row.name)
  try {
    await appService(row.name, op)
    message.success(op === 'start' ? '应用已启动' : op === 'stop' ? '应用已停止（容器保留）' : '应用已重启')
    await store.load()
  } catch (e) {
    if (e instanceof ApiError && e.is('DK_028')) {
      message.warning(DK_ERROR_TEXT['DK_028'])
      await store.load()
      return
    }
    handleWriteError(e)
  } finally {
    releaseBusy(row.name)
  }
}

// 日志弹窗（compose logs 通道；两角色可看）
const logsShow = ref(false)
const logsName = ref('')
const logsLoading = ref(false)
const logsLines = ref<string[]>([])

async function openLogs(row: DockerAppItem): Promise<void> {
  logsName.value = row.name
  logsShow.value = true
  await loadLogs()
}

async function loadLogs(): Promise<void> {
  logsLoading.value = true
  try {
    const data = await appLogs(logsName.value, 500)
    logsLines.value = data.lines
  } catch (e) {
    message.error(errText(e))
  } finally {
    logsLoading.value = false
  }
}

// 删除应用（强确认：输入应用名——容器移除 + 目录删除，不可恢复）
const deleteShow = ref(false)
const deleteTarget = ref<DockerAppItem | null>(null)
const deleteConfirmInput = ref('')
const deleteSubmitting = ref(false)

function openDelete(row: DockerAppItem): void {
  deleteTarget.value = row
  deleteConfirmInput.value = ''
  deleteShow.value = true
}

async function submitDelete(): Promise<void> {
  const row = deleteTarget.value
  if (!row) return
  deleteSubmitting.value = true
  try {
    await appDelete(row.name)
    message.success(`应用已删除：${row.name}`)
    deleteShow.value = false
    await store.load()
  } catch (e) {
    if (e instanceof ApiError && e.is('DK_028')) {
      message.warning(DK_ERROR_TEXT['DK_028'])
      deleteShow.value = false
      await store.load()
      return
    }
    handleWriteError(e)
  } finally {
    deleteSubmitting.value = false
  }
}

// 建站（反向代理到应用本机端口）
const siteShow = ref(false)
const siteTarget = ref<DockerAppItem | null>(null)
const siteDomain = ref('')
const siteSubmitting = ref(false)

function openSite(row: DockerAppItem): void {
  siteTarget.value = row
  siteDomain.value = ''
  siteShow.value = true
}

async function submitSite(): Promise<void> {
  const row = siteTarget.value
  if (!row) return
  siteSubmitting.value = true
  try {
    const data = await appSite(row.name, siteDomain.value.trim() !== '' ? [siteDomain.value.trim()] : [])
    message.success(`站点已创建：${data.siteId}；域名与 HTTPS 可到网站管理配置`)
    siteShow.value = false
    await store.load()
  } catch (e) {
    handleWriteError(e)
  } finally {
    siteSubmitting.value = false
  }
}

// ---- 表格列 ----

const columns = computed<DataTableColumns<DockerAppItem>>(() => {
  const base: DataTableColumns<DockerAppItem> = [
    { title: '序号', key: 'index', width: 60, render: (_row, index) => index + 1 },
    {
      title: '应用名',
      key: 'name',
      width: 180,
      // 应用名 link → 日志弹窗（查看类，两角色保留）
      render: (row) =>
        h(NButton, { text: true, type: 'primary', onClick: () => void openLogs(row) }, { default: () => row.name })
    },
    { title: '镜像', key: 'image', width: 200, render: (row) => h('span', { class: 'mono' }, row.image || '—') },
    {
      title: '本机端口',
      key: 'port',
      width: 200,
      render: (row) =>
        row.hostPort > 0 ? `127.0.0.1:${row.hostPort} → ${row.containerPort}` : '—'
    },
    {
      title: '状态',
      key: 'state',
      width: 110,
      render: (row) => {
        const d = DOCKER_APP_STATE[row.state]
        return h(NTag, { type: d?.tag ?? 'default', size: 'small', bordered: false }, { default: () => d?.label ?? row.state })
      }
    },
    {
      title: '站点',
      key: 'site',
      width: 130,
      render: (row) => {
        if (row.siteLinked === true) return h('a', { class: 'app-link', href: '/sites?tab=site', target: '_blank' }, '已建站')
        if (row.siteLinked === false) {
          return h(NTooltip, null, { trigger: () => '仅本机可访问', default: () => '应用端口只绑本机；可经「建站」反向代理对外暴露' })
        }
        return '—'
      }
    }
  ]
  if (isAdmin.value) {
    base.push({
      title: '操作',
      key: 'actions',
      width: 330,
      fixed: 'right',
      render: (row) => {
        const busy = inFlight.value.has(row.name)
        const nodes: ReturnType<typeof h>[] = []
        if (row.state === 'stopped') {
          nodes.push(
            h(NButton, { size: 'small', disabled: busy, onClick: () => void execService(row, 'start') }, { default: () => '启动' })
          )
        } else {
          nodes.push(
            h(NPopconfirm, { onPositiveClick: () => void execService(row, 'stop') }, {
              trigger: () => h(NButton, { size: 'small', disabled: busy }, { default: () => '停止' }),
              default: () => `停止应用 ${row.name}？容器会保留，可随时再启动。`
            }),
            h(NPopconfirm, { onPositiveClick: () => void execService(row, 'restart') }, {
              trigger: () => h(NButton, { size: 'small', disabled: busy }, { default: () => '重启' }),
              default: () => `重启应用 ${row.name}？`
            })
          )
        }
        nodes.push(
          h(NButton, { size: 'small', onClick: () => void openLogs(row) }, { default: () => '日志' })
        )
        if (row.siteLinked !== true) {
          nodes.push(h(NButton, { size: 'small', onClick: () => openSite(row) }, { default: () => '建站' }))
        }
        nodes.push(
          h(NButton, { size: 'small', type: 'error', ghost: true, disabled: busy, onClick: () => openDelete(row) }, { default: () => '删除' })
        )
        return h('div', { class: 'row-actions' }, nodes)
      }
    })
  }
  return base
})

const scrollX = computed(() => (isAdmin.value ? 1310 : 880))

onMounted(() => {
  if (!store.loaded) void store.load()
})
</script>

<template>
  <div class="apps-tab">
    <div class="controls-row">
      <n-button v-if="isAdmin" type="primary" @click="openCreate">创建应用</n-button>
      <n-button :loading="loading" @click="store.load()">刷新</n-button>
    </div>
    <div class="meta">
      数据生成于 {{ generatedAtText }} · 共 {{ total }} 个应用
    </div>
    <n-alert v-if="loadError" type="error" class="load-err">
      {{ loadError }}
      <n-button size="tiny" @click="store.load()">重试</n-button>
    </n-alert>
    <n-data-table
      :columns="columns"
      :data="items"
      :loading="loading"
      :scroll-x="scrollX"
      :row-key="(r: DockerAppItem) => r.name"
    >
      <template #empty>
        <div class="tbl-empty">
          <n-empty description="还没有应用" />
          <n-button v-if="isAdmin && !loadError" size="small" type="primary" @click="openCreate">创建应用</n-button>
        </div>
      </template>
    </n-data-table>

    <!-- 创建应用弹窗（失败保留并显示原因） -->
    <n-modal v-if="isAdmin" v-model:show="createShow" preset="card" title="创建应用" style="width: 640px">
      <n-alert v-if="createError !== null" type="error" :bordered="false" class="create-error">{{ createError }}</n-alert>
      <n-form label-placement="left" label-width="90">
        <n-form-item label="应用名"><n-input v-model:value="createForm.name" placeholder="小写字母、数字、连字符，如 my-app" /></n-form-item>
        <n-form-item label="镜像"><n-input v-model:value="createForm.image" placeholder="nginx:latest" /></n-form-item>
        <n-form-item label="主机端口"><n-input-number v-model:value="createForm.hostPort" :min="1" :max="65535" /></n-form-item>
        <n-form-item label="容器端口"><n-input-number v-model:value="createForm.containerPort" :min="1" :max="65535" /></n-form-item>
        <n-form-item label="环境变量">
          <div class="row-list">
            <div v-for="(v, i) in createForm.env" :key="i" class="kv-row">
              <n-input v-model:value="v.key" placeholder="KEY" />
              <span class="kv-sep">=</span>
              <n-input v-model:value="v.value" placeholder="value" />
              <n-button size="small" quaternary :disabled="createForm.env.length === 1" @click="createForm.env.splice(i, 1)">删除</n-button>
            </div>
            <n-button size="small" dashed class="row-add" @click="createForm.env.push({ key: '', value: '' })">添加环境变量</n-button>
          </div>
        </n-form-item>
        <n-form-item label="挂载">
          <div class="row-list">
            <div v-for="(m, i) in createForm.mounts" :key="i" class="kv-row">
              <n-input v-model:value="m.source" placeholder="宿主机路径或卷名" />
              <span class="kv-sep">:</span>
              <n-input v-model:value="m.target" placeholder="容器内路径（以 / 开头）" />
              <n-button size="small" quaternary :disabled="createForm.mounts.length === 1" @click="createForm.mounts.splice(i, 1)">删除</n-button>
            </div>
            <n-button size="small" dashed class="row-add" @click="createForm.mounts.push({ source: '', target: '' })">添加挂载</n-button>
          </div>
        </n-form-item>
        <n-form-item label="重启策略"><n-select v-model:value="createForm.restartPolicy" :options="restartPolicyOptions" /></n-form-item>
      </n-form>
      <n-alert type="info" :bordered="false">应用端口只在本机可访问，建站后对外提供访问。</n-alert>
      <template #footer>
        <n-button @click="createShow = false">取消</n-button>
        <n-button type="primary" :loading="createSubmitting" @click="submitCreate">创建并启动</n-button>
      </template>
    </n-modal>

    <!-- 日志弹窗（两角色可看） -->
    <n-modal v-model:show="logsShow" preset="card" :title="`应用日志：${logsName}`" style="width: 760px">
      <div class="log-head">
        <n-button size="small" :loading="logsLoading" @click="loadLogs">刷新</n-button>
      </div>
      <pre class="log">{{ logsLines.length > 0 ? logsLines.join('\n') : '暂无日志' }}</pre>
    </n-modal>

    <!-- 建站弹窗 -->
    <n-modal v-if="isAdmin" v-model:show="siteShow" preset="card" :title="`应用建站：${siteTarget?.name ?? ''}`" style="width: 480px">
      <n-alert type="info" :bordered="false">
        将创建反向代理站点，把访问转发到应用的本机端口{{ siteTarget && siteTarget.hostPort > 0 ? ` 127.0.0.1:${siteTarget.hostPort}` : '' }}。
      </n-alert>
      <n-form-item label="域名" label-placement="left" label-width="60">
        <n-input v-model:value="siteDomain" placeholder="app.example.com（可留空，之后到网站管理绑定）" />
      </n-form-item>
      <template #footer>
        <n-button @click="siteShow = false">取消</n-button>
        <n-button type="primary" :loading="siteSubmitting" @click="submitSite">创建站点</n-button>
      </template>
    </n-modal>

    <!-- 删除应用（强确认：输入应用名） -->
    <n-modal v-if="isAdmin" v-model:show="deleteShow" preset="card" title="删除应用" style="width: 520px">
      <n-alert type="warning">
        将停止并移除容器，并删除项目目录 {{ deleteTarget?.workingDir }}，不可恢复。已建的站点不会删除。
      </n-alert>
      <n-form-item label="输入应用名确认" label-placement="left" label-width="120">
        <n-input v-model:value="deleteConfirmInput" :placeholder="deleteTarget?.name" />
      </n-form-item>
      <template #footer>
        <n-button @click="deleteShow = false">取消</n-button>
        <n-button type="error" :loading="deleteSubmitting" :disabled="deleteConfirmInput !== deleteTarget?.name" @click="submitDelete">确认删除</n-button>
      </template>
    </n-modal>
  </div>
</template>

<style scoped>
.apps-tab {
  padding: 4px 0;
}
.controls-row {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 8px;
}
.meta {
  font-size: 12px;
  opacity: 0.6;
  margin-bottom: 8px;
}
.load-err {
  margin-bottom: 12px;
}
.create-error {
  margin-bottom: 12px;
}
.row-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  width: 100%;
}
.kv-row {
  display: flex;
  align-items: center;
  gap: 6px;
}
.kv-row > :deep(.n-input) {
  flex: 1;
}
.kv-sep {
  flex: none;
  opacity: 0.6;
}
.row-add {
  align-self: flex-start;
}
.log-head {
  display: flex;
  gap: 8px;
  margin-bottom: 8px;
}
.log {
  background: rgba(128, 128, 128, 0.08);
  padding: 8px;
  max-height: 420px;
  overflow: auto;
  font-size: 12px;
  white-space: pre-wrap;
}
.mono {
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', monospace;
  font-size: 12px;
  word-break: break-all;
}
.app-link {
  color: var(--primary-color, #18a058);
}
.tbl-empty {
  padding: 24px 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
}
.row-actions {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
</style>
