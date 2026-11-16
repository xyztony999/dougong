<script setup lang="ts">
/**
 * 授权管理弹窗（PageSpec 5.4；dbuserGrant/dbuserRevoke，一弹窗两区）。
 * 新增授权区：库（options=databaseList 实况）+ 档位（grant_level 三档，D-10 说明文案）；
 * 既有授权区：该用户 grants 列表（库名 + 档位 tag），逐条回收（NPopconfirm，DBM_011 如实提示）。
 * 每次授权/回收后重拉 dbuserList 提取该用户行（I-04 实况直更，不乐观更新）。
 */
import { computed, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import type { SelectOption } from 'naive-ui'
import { databaseList, dbuserGrant, dbuserList, dbuserRevoke } from '@/api/database'
import { ApiError } from '@/api/http'
import { GRANT_LEVEL, dictItem } from '@/types/enums'
import type { DictItem, GrantLevel } from '@/types/enums'
import type { DbGrantItem } from '@/types/database'
import { dbmErrorText } from '@/types/database'

const show = defineModel<boolean>('show', { default: false })

const props = defineProps<{
  instanceId: number
  /** 目标用户（null=未选定） */
  user: { username: string; host: string; grants: DbGrantItem[] } | null
}>()

const emit = defineEmits<{
  /** 授权/回收成功（页面重拉用户清单） */
  changed: []
}>()

const message = useMessage()

/** ---- 数据（I-04 每次打开/动作后实况重拉） ---- */

const grants = ref<DbGrantItem[]>([])
const databases = ref<string[]>([])
const loading = ref(false)
const granting = ref(false)
const revokingDb = ref<string | null>(null)
/** 新增授权区内联错误（DBM_009/007 目标不存在） */
const inlineError = ref<string | null>(null)

async function reload(): Promise<void> {
  if (props.user === null) return
  loading.value = true
  try {
    const [users, dbs] = await Promise.all([
      dbuserList(props.instanceId),
      databaseList(props.instanceId)
    ])
    const row = users.items.find((u) => u.username === props.user?.username && u.host === props.user?.host)
    grants.value = row?.grants ?? []
    databases.value = dbs.items.map((d) => d.name)
    emit('changed') // 页面同步刷新用户清单（授权概况列）
  } finally {
    loading.value = false
  }
}

watch(show, (v) => {
  if (!v || props.user === null) return
  grants.value = props.user.grants
  form.value = { database: null, level: 'readonly' }
  inlineError.value = null
  void reload()
})

/** ---- 新增授权区 ---- */

interface GrantForm {
  database: string | null
  level: GrantLevel
}

const form = ref<GrantForm>({ database: null, level: 'readonly' })

const dbOptions = computed<SelectOption[]>(() =>
  databases.value.map((d) => ({ label: d, value: d }))
)

/** 三档下拉（D-10 权限映射说明随选项呈现） */
const levelOptions: SelectOption[] = (Object.keys(GRANT_LEVEL) as GrantLevel[]).map((v) => {
  const hint: Record<GrantLevel, string> = {
    readonly: '仅 SELECT',
    write: 'SELECT, INSERT, UPDATE, DELETE',
    all: 'ALL PRIVILEGES（高危）'
  }
  return { label: `${GRANT_LEVEL[v].label} — ${hint[v]}`, value: v }
})

/** 档位 Tag 展示项（grant_level 三色：readonly=gray / write=blue / all=red，PageSpec 第 3 节） */
function levelDict(level: string): DictItem {
  return dictItem(GRANT_LEVEL, level)
}

async function onGrant(): Promise<void> {
  if (props.user === null || form.value.database === null) {
    message.warning('请选择库与授权档位')
    return
  }
  inlineError.value = null
  granting.value = true
  try {
    await dbuserGrant({
      instanceId: props.instanceId,
      username: props.user.username,
      host: props.user.host,
      database: form.value.database,
      level: form.value.level
    })
    message.success(`已授权 ${form.value.database}（${GRANT_LEVEL[form.value.level].label}）`)
    form.value = { database: null, level: 'readonly' }
    await reload()
  } catch (e) {
    if (e instanceof ApiError) {
      if (e.is('DBM_009') || e.is('DBM_007')) {
        inlineError.value = dbmErrorText(e.code, e.data) // 目标不存在内联提示（PageSpec 5.4）
        return
      }
      const text = dbmErrorText(e.code, e.data)
      if (text !== null) message.error(text)
    }
  } finally {
    granting.value = false
  }
}

/** ---- 既有授权区：回收（NPopconfirm；DBM_011 如实提示） ---- */

async function onRevoke(database: string): Promise<void> {
  if (props.user === null) return
  revokingDb.value = database
  try {
    await dbuserRevoke(props.instanceId, props.user.username, props.user.host, database)
    message.success(`已回收 ${database} 的授权`)
    await reload()
  } catch (e) {
    if (e instanceof ApiError) {
      const text = dbmErrorText(e.code, e.data) // DBM_011 授权关系不存在等
      if (text !== null) message.warning(text)
    }
  } finally {
    revokingDb.value = null
  }
}

function grantRowKey(g: DbGrantItem): string {
  return g.database
}
</script>

<template>
  <n-modal
    v-model:show="show"
    preset="card"
    :title="`授权管理${user !== null ? ` — ${user.username}@${user.host}` : ''}`"
    style="width: 560px"
    :mask-closable="false"
  >
    <n-spin :show="loading">
      <!-- 新增授权区 -->
      <div class="section-title">新增授权</div>
      <div class="grant-row">
        <n-select
          v-model:value="form.database"
          :options="dbOptions"
          filterable
          placeholder="选择库（实例实况）"
          class="grant-db"
        />
        <n-select v-model:value="form.level" :options="levelOptions" class="grant-level" />
        <n-button type="primary" :loading="granting" @click="onGrant">授权</n-button>
      </div>
      <n-alert v-if="inlineError !== null" type="error" class="inline-error" :show-icon="false">
        {{ inlineError }}
      </n-alert>

      <!-- 既有授权区 -->
      <div class="section-title">既有授权</div>
      <n-empty
        v-if="grants.length === 0"
        description="该用户暂无库级授权（未识别的库外权限不呈现）"
        class="empty"
      />
      <div v-else class="grants-list">
        <div v-for="g in grants" :key="grantRowKey(g)" class="grant-item">
          <span class="grant-dbname">{{ g.database }}</span>
          <n-tag :type="levelDict(g.level).tag" size="small" :bordered="false">
            {{ levelDict(g.level).label }}
          </n-tag>
          <n-popconfirm @positive-click="onRevoke(g.database)">
            <template #trigger>
              <n-button
                size="tiny"
                type="error"
                ghost
                :loading="revokingDb === g.database"
                :disabled="revokingDb !== null"
              >
                回收
              </n-button>
            </template>
            确认回收 {{ user?.username }}@{{ user?.host }} 在 {{ g.database }} 上的全部授权？
          </n-popconfirm>
        </div>
      </div>
    </n-spin>
    <template #footer>
      <div class="modal-footer">
        <n-button @click="show = false">关闭</n-button>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.section-title {
  font-weight: 500;
  font-size: 13px;
  margin: 4px 0 10px;
}
.grant-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.grant-db {
  width: 200px;
}
.grant-level {
  width: 230px;
}
.inline-error {
  margin-top: 10px;
}
.grants-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 240px;
  overflow-y: auto;
}
.grant-item {
  display: flex;
  align-items: center;
  gap: 10px;
}
.grant-dbname {
  min-width: 160px;
  font-size: 13px;
}
.empty {
  padding: 12px 0;
}
.modal-footer {
  display: flex;
  justify-content: flex-end;
}
</style>
