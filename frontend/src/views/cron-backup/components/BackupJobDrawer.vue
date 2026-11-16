<script setup lang="ts">
/**
 * 新建/编辑备份任务 Drawer（PageSpec 5.3；admin 专属，readonly 无入口不渲染宿主）。
 * 宽 720px 右侧 Drawer：名称 / 备份源动态行组（站点目录 NSelect multiple +
 * 数据库 实例+库 NSelect，联动数据源=site-mgmt /sites/site/list 与
 * database-mgmt /db/{instance,database}/list 既有接口，api 1.2⑦）/ 目标类型 radio
 * 条件显隐字段组（local 绝对路径 / rsync host·user·port·path，MVP ssh 免密无凭据 D-06）/
 * 保留份数 1-100 / 调度表达式（复用 jobValidate 校验预览，D2 口径-2 双消费）。
 *
 * 已声明的实现偏差（诚实对应 API）：backup-job/list 不下发 sourceItems/targetConfig
 * 结构化定义（仅 sourceSummary/targetBrief 概要）且无 detail 接口——编辑态仅回填
 * name/targetType/retentionCount/scheduleCron，源与目标须重选（update 为整体替换
 * 语义，api 4.1.4）；Drawer 顶部 NAlert 呈现当前定义概要与替换提示。
 */
import { computed, reactive, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import type { FormInst, FormRules, SelectOption } from 'naive-ui'
import { backupJobCreate, backupJobUpdate } from '@/api/cron'
import { databaseList, instanceList } from '@/api/database'
import { ApiError } from '@/api/http'
import { siteList } from '@/api/site'
import { useCronStore } from '@/stores/cron'
import { isFieldErrorList } from '@/types/auth'
import type { DbInstanceItem } from '@/types/database'
import { BACKUP_SOURCE_TYPE, dictItem } from '@/types/enums'
import type { SiteListItem } from '@/types/site'
import { CB_ERROR_TEXT, cronValidateErrors } from '@/types/cron'
import type { BackupSourceItem, BackupTargetConfig } from '@/types/cron'
import ExprPreview from './ExprPreview.vue'
import { useExprValidate } from '../expr-validate'

/** 备份源动态行（表单视图模型；提交时映射为 BackupSourceItem[]） */
type SourceRow =
  | { kind: 'site_dir'; paths: string[] }
  | { kind: 'database'; instanceId: number | null; databases: string[] }

/** 保留份数 UI 初始值（服务端无默认值必填；水位语义 I-05） */
const RETENTION_DEFAULT = 7

const show = defineModel<boolean>('show', { default: false })
const emit = defineEmits<{ changed: [] }>()

const store = useCronStore()
const message = useMessage()

const editing = computed(() => store.editingBackupJob)

const formRef = ref<FormInst | null>(null)
const submitting = ref(false)
const form = reactive({
  name: '',
  targetType: 'local' as 'local' | 'rsync',
  localPath: '',
  rsyncHost: '',
  rsyncUser: '',
  rsyncPort: null as number | null,
  rsyncPath: '',
  retentionCount: RETENTION_DEFAULT,
  scheduleCron: ''
})
const sourceRows = ref<SourceRow[]>([{ kind: 'site_dir', paths: [] }])
/** 服务端字段级错误（CB_001/CB_002/CB_005 回填） */
const serverErrors = ref<{ name: string | null; scheduleCron: string | null }>({
  name: null,
  scheduleCron: null
})

const scheduleRef = computed(() => form.scheduleCron)
const preview = useExprValidate(scheduleRef)

/** ---- 联动数据源（开 Drawer 即拉取，不缓存——实况投影口径） ---- */
const siteOptions = ref<SelectOption[]>([])
const instanceOptions = ref<SelectOption[]>([])
const instances = ref<DbInstanceItem[]>([])
/** 每实例库清单缓存（Drawer 会话内；实例选定后拉取，PageSpec 5.3） */
const dbOptionsByInstance = ref<Record<number, SelectOption[]>>({})
const dbLoadingByInstance = ref<Record<number, boolean>>({})
const linkageLoading = ref(false)

async function loadLinkage(): Promise<void> {
  linkageLoading.value = true
  try {
    const [sites, insts] = await Promise.all([siteList(), instanceList()])
    siteOptions.value = sites.items.map((s: SiteListItem) => ({
      label: `${s.serverNames[0] ?? s.sourceFile}·${s.root}`,
      value: s.root
    }))
    instances.value = insts.items
    instanceOptions.value = insts.items.map((it) => {
      const addr = it.connMode === 'tcp' ? `${it.host ?? ''}:${it.port ?? ''}` : (it.socketPath ?? '')
      return { label: `${it.engine} ${addr}`, value: it.id }
    })
  } catch {
    // 拦截器统一提示；下拉空选项自然引导刷新重开
  } finally {
    linkageLoading.value = false
  }
}

/** 实例选定后拉取库清单（api 1.2⑦：/db/database/list?instanceId=；未选库=整实例全库） */
async function onInstanceChange(row: Extract<SourceRow, { kind: 'database' }>): Promise<void> {
  row.databases = []
  const id = row.instanceId
  if (id === null) return
  if (dbOptionsByInstance.value[id] !== undefined) return
  dbLoadingByInstance.value = { ...dbLoadingByInstance.value, [id]: true }
  try {
    const data = await databaseList(id)
    dbOptionsByInstance.value = { ...dbOptionsByInstance.value, [id]: data.items.map((d) => ({ label: d.name, value: d.name })) }
  } catch {
    // 拦截器统一提示；可重选实例重试
  } finally {
    const next = { ...dbLoadingByInstance.value }
    delete next[id]
    dbLoadingByInstance.value = next
  }
}

/** ---- 行组操作（I-09：至少 1 行；两类行混合） ---- */
function addSiteRow(): void {
  sourceRows.value.push({ kind: 'site_dir', paths: [] })
}
function addDatabaseRow(): void {
  sourceRows.value.push({ kind: 'database', instanceId: null, databases: [] })
}
function removeRow(index: number): void {
  sourceRows.value.splice(index, 1)
}

/** 视图行 → 契约 sourceItems（site_dir 多选路径逐项展开；database 未选库=整实例全库） */
const mappedSourceItems = computed<BackupSourceItem[]>(() => {
  const items: BackupSourceItem[] = []
  for (const row of sourceRows.value) {
    if (row.kind === 'site_dir') {
      for (const p of row.paths) items.push({ type: 'site_dir', path: p })
    } else if (row.instanceId !== null) {
      if (row.databases.length === 0) items.push({ type: 'database', instanceId: row.instanceId })
      else
        for (const d of row.databases)
          items.push({ type: 'database', instanceId: row.instanceId, database: d })
    }
  }
  return items
})

function buildTargetConfig(): BackupTargetConfig | null {
  if (form.targetType === 'local') {
    return form.localPath.trim() !== '' ? { path: form.localPath.trim() } : null
  }
  if (form.rsyncHost.trim() === '' || form.rsyncUser.trim() === '' || form.rsyncPath.trim() === '') {
    return null
  }
  return form.rsyncPort === null
    ? { host: form.rsyncHost.trim(), user: form.rsyncUser.trim(), path: form.rsyncPath.trim() }
    : {
        host: form.rsyncHost.trim(),
        user: form.rsyncUser.trim(),
        port: form.rsyncPort,
        path: form.rsyncPath.trim()
      }
}

/** 目标字段组结构性预校验（服务端 CB_001 终审，R-14） */
const targetValid = computed(() => buildTargetConfig() !== null)

const rules: FormRules = {
  name: [
    { required: true, message: '请输入任务名称', trigger: 'blur' },
    { validator: (_r: unknown, v: string) => v.trim().length <= 64, message: '名称不超过 64 字符（R-14）', trigger: 'blur' }
  ],
  retentionCount: {
    required: true,
    type: 'number',
    message: '保留份数 1-100（I-05 水位）',
    trigger: 'blur'
  }
}

/** 保存门控（预览为提交前置，PageSpec 5.3；源≥1 行 I-09；目标字段组齐备 I-10） */
const canSave = computed(
  () =>
    form.name.trim() !== '' &&
    form.name.trim().length <= 64 &&
    mappedSourceItems.value.length > 0 &&
    targetValid.value &&
    preview.state.value === 'valid' &&
    !submitting.value
)

watch(show, (v) => {
  if (!v) return
  serverErrors.value = { name: null, scheduleCron: null }
  dbOptionsByInstance.value = {}
  const job = store.editingBackupJob
  // 编辑态回填：list 仅提供概要字段（已声明偏差，见组件头注释）；源与目标重选
  form.name = job !== null ? job.name : ''
  form.targetType = job !== null ? job.targetType : 'local'
  form.localPath = ''
  form.rsyncHost = ''
  form.rsyncUser = ''
  form.rsyncPort = null
  form.rsyncPath = ''
  form.retentionCount = job !== null ? job.retentionCount : RETENTION_DEFAULT
  form.scheduleCron = job !== null ? job.scheduleCron : ''
  sourceRows.value = [{ kind: 'site_dir', paths: [] }]
  preview.reset()
  formRef.value?.restoreValidation()
  void loadLinkage()
})

/** 编辑态当前定义概要（sourceSummary/targetBrief 概要直呈，不虚构结构化回填；
 *  type 未知值 dictItem 兜底、ref 已经 cronback 归一恒 string，'' 不带引用段） */
const editingSummaryTags = computed(() =>
  editing.value === null
    ? []
    : editing.value.sourceSummary.map((s) => {
        const dict = dictItem(BACKUP_SOURCE_TYPE, s.type)
        return { tag: dict.tag, label: s.ref === '' ? dict.label : `${dict.label}·${s.ref}` }
      })
)

async function onSave(): Promise<void> {
  if (!canSave.value) return
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  const targetConfig = buildTargetConfig()
  if (targetConfig === null) return
  const payload = {
    name: form.name.trim(),
    sourceItems: mappedSourceItems.value,
    targetType: form.targetType,
    targetConfig,
    retentionCount: form.retentionCount,
    scheduleCron: form.scheduleCron.trim()
  }
  submitting.value = true
  try {
    if (editing.value === null) {
      await backupJobCreate(payload)
      message.success('备份任务已创建')
    } else {
      await backupJobUpdate({ jobId: editing.value.id, ...payload })
      message.success('备份任务已保存')
    }
    show.value = false
    emit('changed') // 重拉 backupJobList（含 nextFireAt/lastRun 派生刷新，第 2 节纪律）
  } catch (e) {
    handleError(e)
  } finally {
    submitting.value = false
  }
}

/** CB_001（字段级）/CB_002（段位级）/CB_005（名称）/CB_004（实况已删）分流（PageSpec 第 1 节） */
function handleError(e: unknown): void {
  if (!(e instanceof ApiError)) return
  if (e.is('CB_005')) {
    serverErrors.value.name = CB_ERROR_TEXT['CB_005'] // I-06 名称字段错误（I-06/CB_005 → Drawer 任务名字段错误）
    return
  }
  if (e.is('CB_002')) {
    const errors = cronValidateErrors(e.data)
    serverErrors.value.scheduleCron =
      errors.length > 0 ? errors.join('；') : CB_ERROR_TEXT['CB_002']
    return
  }
  if (e.is('CB_001')) {
    const fields = isFieldErrorList(e.data) ? e.data : []
    serverErrors.value = {
      name: fields.find((f) => f.field === 'name')?.message ?? null,
      scheduleCron: fields.find((f) => f.field === 'scheduleCron')?.message ?? null
    }
    const rest = fields.filter((f) => f.field !== 'name' && f.field !== 'scheduleCron')
    if (rest.length > 0) {
      message.error(rest.map((f) => `${f.field}: ${f.message}`).join('；'))
    } else if (fields.length === 0) {
      message.error(CB_ERROR_TEXT['CB_001'])
    }
    return
  }
  if (e.is('CB_004')) {
    message.warning(CB_ERROR_TEXT['CB_004'])
    show.value = false
    emit('changed') // 实况投影：可能已在面板外删除（api D-01 同判）
    return
  }
  const text = CB_ERROR_TEXT[e.code]
  if (text !== undefined) message.error(text)
}
</script>

<template>
  <n-drawer v-model:show="show" :width="720" placement="right">
    <n-drawer-content
      :title="editing === null ? '新建备份任务' : '编辑备份任务'"
      closable
    >
      <!-- 编辑态整体替换提示（已声明偏差：list 不回传源/目标结构化定义） -->
      <n-alert v-if="editing !== null" type="info" class="replace-alert">
        整体替换语义：列表接口不回传备份源与目标配置的结构化定义，保存时将以本次填写内容整体替换原任务——请重新选择备份源与目标。当前定义概要：目标
        {{ editing.targetBrief }}，调度 {{ editing.scheduleCron }}；源：
        <n-tag
          v-for="(s, i) in editingSummaryTags"
          :key="i"
          :type="s.tag"
          size="small"
          :bordered="false"
          class="summary-tag"
        >
          {{ s.label }}
        </n-tag>
      </n-alert>

      <n-form ref="formRef" :model="form" :rules="rules" label-placement="top">
        <n-form-item label="名称" path="name" :feedback="serverErrors.name ?? undefined">
          <n-input
            v-model:value="form.name"
            placeholder="≤64 字符；唯一性由服务端校验（编辑态排除自身）"
            clearable
          />
        </n-form-item>

        <!-- 备份源动态行组（I-09 至少 1 行；两类行混合） -->
        <n-form-item label="备份源" :show-feedback="false">
          <div class="source-block">
            <template v-for="(row, i) in sourceRows" :key="i">
              <div v-if="row.kind === 'site_dir'" class="source-row">
                <n-select
                  v-model:value="row.paths"
                  multiple
                  clearable
                  filterable
                  :options="siteOptions"
                  :loading="linkageLoading"
                  placeholder="站点目录（可多选，取站点根路径）"
                  class="source-select"
                />
                <n-button size="small" quaternary type="error" @click="removeRow(i)">
                  移除
                </n-button>
              </div>
              <div v-else class="source-row db-row">
                <n-select
                  v-model:value="row.instanceId"
                  clearable
                  filterable
                  :options="instanceOptions"
                  :loading="linkageLoading"
                  placeholder="数据库实例"
                  class="source-select db-instance"
                  @update:value="() => { void onInstanceChange(row) }"
                />
                <n-select
                  v-model:value="row.databases"
                  multiple
                  clearable
                  filterable
                  :options="row.instanceId !== null ? (dbOptionsByInstance[row.instanceId] ?? []) : []"
                  :loading="row.instanceId !== null && dbLoadingByInstance[row.instanceId] === true"
                  :disabled="row.instanceId === null"
                  placeholder="库（未选=整实例全库）"
                  class="source-select"
                />
                <n-button size="small" quaternary type="error" @click="removeRow(i)">移除</n-button>
              </div>
            </template>
            <div class="source-add-row">
              <n-button size="small" dashed @click="addSiteRow()">+ 站点目录</n-button>
              <n-button size="small" dashed @click="addDatabaseRow()">+ 数据库</n-button>
            </div>
            <div class="hint">
              站点目录保存后为路径快照，站点变更不自动跟随；数据库项引用实例（未选库=整实例全库）。
            </div>
          </div>
        </n-form-item>

        <!-- 目标类型条件显隐字段组（I-10；rsync 无凭据字段 D-06） -->
        <n-form-item label="目标类型" path="targetType">
          <n-radio-group v-model:value="form.targetType">
            <n-radio value="local">本地目录</n-radio>
            <n-radio value="rsync">rsync 远程</n-radio>
          </n-radio-group>
        </n-form-item>
        <template v-if="form.targetType === 'local'">
          <n-form-item label="本地目录（绝对路径）" path="localPath">
            <n-input
              v-model:value="form.localPath"
              class="mono-input"
              placeholder="如 /var/backups/dugong（须为面板数据目录之外的绝对路径）"
              clearable
            />
          </n-form-item>
        </template>
        <template v-else>
          <n-form-item label="主机 host" path="rsyncHost">
            <n-input v-model:value="form.rsyncHost" placeholder="rsync 目标主机" clearable />
          </n-form-item>
          <n-form-item label="用户 user" path="rsyncUser">
            <n-input v-model:value="form.rsyncUser" placeholder="ssh 登录用户" clearable />
          </n-form-item>
          <n-form-item label="端口 port（可空默认 22）" path="rsyncPort">
            <n-input-number
              v-model:value="form.rsyncPort"
              :min="1"
              :max="65535"
              placeholder="22"
              class="port-input"
              clearable
            />
          </n-form-item>
          <n-form-item label="远程路径 path" path="rsyncPath">
            <n-input v-model:value="form.rsyncPath" class="mono-input" placeholder="如 /backups/dugong" clearable />
          </n-form-item>
        </template>

        <n-form-item label="保留份数（1-100）" path="retentionCount">
          <n-input-number v-model:value="form.retentionCount" :min="1" :max="100" class="port-input" />
        </n-form-item>

        <n-form-item
          label="调度表达式"
          path="scheduleCron"
          :feedback="serverErrors.scheduleCron ?? undefined"
        >
          <n-input
            v-model:value="form.scheduleCron"
            class="mono-input"
            placeholder="五段式（分 时 日 月 周）"
            clearable
          />
        </n-form-item>
        <expr-preview
          :state="preview.state.value"
          :next-fires="preview.nextFires.value"
          :error-text="preview.errorText.value"
        />
      </n-form>
      <div v-if="form.targetType === 'rsync'" class="hint footer-hint">
        远程目标经 ssh 免密连接，面板不存储任何凭据。
      </div>

      <template #footer>
        <div class="drawer-footer">
          <n-button :disabled="submitting" @click="show = false">取消</n-button>
          <n-button type="primary" :disabled="!canSave" :loading="submitting" @click="onSave()">
            保存
          </n-button>
        </div>
      </template>
    </n-drawer-content>
  </n-drawer>
</template>

<style scoped>
.replace-alert {
  margin-bottom: 12px;
}
.summary-tag {
  margin-right: 4px;
}
.source-block {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.source-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.source-select {
  flex: 1;
  min-width: 0;
}
.db-row .db-instance {
  flex: 0 0 40%;
}
.source-add-row {
  display: flex;
  gap: 8px;
}
.hint {
  font-size: 12px;
  opacity: 0.6;
}
.footer-hint {
  margin-top: 8px;
}
.port-input {
  width: 180px;
}
.drawer-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
.mono-input :deep(input) {
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', monospace;
}
</style>
