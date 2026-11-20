<script setup lang="ts">
/**
 * 备份导入弹窗（PageSpec 5.6；backupImport，行内入口预选目标库）。
 * 提交前 NModal 强确认（useDialog.warning 渲染为模态）：导入为数据变更且非幂等
 * （重复导入数据叠加）、失败不自动回滚（PRD FR-16/N2）。
 * 文件 ≤512MiB 前端预检（DBM_014 服务端为准，D-06）；执行中按钮在途禁用（DBM_016 互斥 D-12）。
 * 失败 DBM_013 展示 data.reason（弹窗内联，保持打开可重试）。
 */
import { ref, watch } from 'vue'
import { useDialog, useMessage } from 'naive-ui'
import type { UploadFileInfo } from 'naive-ui'
import { backupImport, databaseList } from '@/api/database'
import { ApiError } from '@/api/http'
import type { BackupImportResult, DbDatabaseItem } from '@/types/database'
import { DUMP_SIZE_LIMIT_BYTES, dbmErrorText } from '@/types/database'
import { formatBytes } from '@/utils/format'

const show = defineModel<boolean>('show', { default: false })

const props = defineProps<{
  instanceId: number
  /** 行内入口预选目标库（锁定不可改，PageSpec 5.6） */
  database: string | null
}>()

const emit = defineEmits<{
  /** 导入成功（页面重拉库清单） */
  imported: [result: BackupImportResult]
}>()

const message = useMessage()
const dialog = useDialog()
const importing = ref(false)
const fileList = ref<UploadFileInfo[]>([])
/** 弹窗内联错误（DBM_013 reason / 预检失败） */
const inlineError = ref<string | null>(null)

/** 库候选（弹窗可脱离库 Tab 直接打开——实况读取 I-04；预选锁定时仅作展示校验） */
const databases = ref<DbDatabaseItem[]>([])

watch(show, async (v) => {
  if (!v) return
  fileList.value = []
  inlineError.value = null
  if (props.database === null) {
    try {
      const data = await databaseList(props.instanceId)
      databases.value = data.items
    } catch {
      // 错误已提示；候选为空由选择器如实呈现
    }
  }
})

function currentFile(): File | null {
  const info = fileList.value[0]
  if (info === undefined || !(info.file instanceof File)) return null
  return info.file
}

/** NUpload @change 载荷（UploadOnChange：{file, fileList, event?}——取 fileList 单文件收敛） */
function onFileChange(options: { fileList: UploadFileInfo[] }): void {
  fileList.value = options.fileList.slice(-1) // 单文件
  inlineError.value = null
}

/** 提交前强确认（PageSpec 5.6：非幂等与不回滚语义必须明示） */
function confirmDialog(file: File): void {
  dialog.warning({
    title: '确认导入备份？',
    content:
      `目标库 ${props.database ?? ''} 将执行 SQL 导入（${formatBytes(file.size)}）。` +
      '导入会修改数据，重复导入会叠加；失败不会自动回滚。',
    positiveText: '确认导入',
    negativeText: '取消',
    onPositiveClick: () => void doImport(file)
  })
}

async function doImport(file: File): Promise<void> {
  if (props.database === null) return
  importing.value = true
  inlineError.value = null
  try {
    const result = await backupImport(props.instanceId, props.database, file)
    message.success(`导入完成（${formatBytes(result.sizeBytes)}），动作已记入审计`)
    emit('imported', result)
    show.value = false
  } catch (e) {
    if (e instanceof ApiError) {
      if (e.is('DBM_013')) {
        const reason = dbmErrorText(e.code, e.data)
        inlineError.value = reason ?? '导入执行失败（不自动回滚）'
        return
      }
      const text = dbmErrorText(e.code, e.data) // DBM_016 在途 / DBM_014 超限（服务端为准）
      if (text !== null) message.error(text)
    }
  } finally {
    importing.value = false
  }
}

function onSubmit(): void {
  const file = currentFile()
  if (props.database === null) {
    message.warning('未指定目标库')
    return
  }
  if (file === null) {
    inlineError.value = '请选择 SQL 文件（.sql）'
    return
  }
  if (file.size > DUMP_SIZE_LIMIT_BYTES) {
    inlineError.value = `文件超过 512MiB 上限（当前 ${formatBytes(file.size)}）`
    return
  }
  confirmDialog(file)
}
</script>

<template>
  <n-modal
    v-model:show="show"
    preset="card"
    title="导入备份"
    style="width: 480px"
    :mask-closable="false"
  >
    <n-form label-placement="top">
      <n-form-item label="目标库（行内入口预选锁定）">
        <n-select
          :value="database"
          :options="databases.map((d) => ({ label: d.name, value: d.name }))"
          disabled
          placeholder="—"
        />
      </n-form-item>
      <n-form-item label="SQL 文件（≤512MiB）">
        <n-upload
          :default-upload="false"
          accept=".sql"
          :max="1"
          :file-list="fileList"
          :disabled="importing"
          @change="onFileChange"
        >
          <n-button :disabled="importing">选择 .sql 文件</n-button>
        </n-upload>
      </n-form-item>
      <n-alert v-if="inlineError !== null" type="error" :show-icon="false">
        {{ inlineError }}
      </n-alert>
    </n-form>
    <template #footer>
      <div class="modal-footer">
        <n-button :disabled="importing" @click="show = false">取消</n-button>
        <n-button type="primary" :loading="importing" @click="onSubmit">导入</n-button>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
</style>
