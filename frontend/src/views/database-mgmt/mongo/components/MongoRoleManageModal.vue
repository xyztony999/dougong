<script setup lang="ts">
/**
 * MongoDB 角色管理弹窗（PageSpec v1.2 5.10；admin）。一弹窗两区（与 MySQL 5.4 同构）：
 * 新增授权区（库+角色 → mongoUserGrant）+ 既有授权区（逐条回收 mongoUserRevoke，
 * DBM_011 如实提示——service 实况前置校验）。roles 数据由用户清单行传入
 *（复用 muser/list 响应，D-04 写后由父级重拉）。
 */
import { ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import type { SelectOption } from 'naive-ui'
import { mongoUserGrant, mongoUserRevoke } from '@/api/database'
import { ApiError } from '@/api/http'
import { MONGO_ROLE, dictItem } from '@/types/enums'
import type { MongoRole } from '@/types/enums'
import type { MongoDatabaseItem, MongoUserItem } from '@/types/database'

const show = defineModel<boolean>('show', { default: false })

const props = defineProps<{
  instanceId: number
  /** 目标用户（null=未选定，弹窗不渲染主体） */
  user: MongoUserItem | null
  /** 库清单实况（授予候选；目标库不存在时如实放行提示——api 4.5.2 预授权语义） */
  databases: MongoDatabaseItem[]
}>()

const emit = defineEmits<{
  /** 授予或回收成功（用户清单重拉） */
  changed: []
}>()

const message = useMessage()
const inlineError = ref<string | null>(null)
const granting = ref(false)
const revokingKey = ref<string | null>(null)

const form = ref({ db: '', role: 'read' as MongoRole })

watch(show, (v) => {
  if (!v) return
  form.value = { db: '', role: 'read' }
  inlineError.value = null
})

const dbOptions: SelectOption[] = props.databases.map((d) => ({ label: d.name, value: d.name }))
const roleOptions: SelectOption[] = (Object.keys(MONGO_ROLE) as MongoRole[]).map((v) => ({
  label: `${MONGO_ROLE[v].label}（${v}）`,
  value: v
}))

function errorText(e: unknown): string | null {
  if (!(e instanceof ApiError)) return null
  const map: Record<string, string> = {
    DBM_009: '用户不存在（可能已被删除，请刷新用户清单）',
    DBM_011: '授权关系不存在（该用户未持有此库角色）',
    // DBM_029 引擎操作被拒（A-DB13：旧 DBM_022 语义映射已随编号裁定改判）
    DBM_029: '数据库拒绝操作：当前账号权限不足'
  }
  return map[e.code] ?? e.message
}

async function onGrant(): Promise<void> {
  if (props.user === null || form.value.db === '') {
    inlineError.value = '请选择目标库与角色'
    return
  }
  inlineError.value = null
  granting.value = true
  try {
    await mongoUserGrant({
      instanceId: props.instanceId,
      username: props.user.username,
      db: form.value.db,
      role: form.value.role
    })
    message.success(`已授予 ${props.user.username} @ ${form.value.db}：${form.value.role}`)
    emit('changed')
  } catch (e) {
    const text = errorText(e)
    if (text !== null) inlineError.value = text
  } finally {
    granting.value = false
  }
}

async function onRevoke(db: string, role: string): Promise<void> {
  if (props.user === null) return
  inlineError.value = null
  revokingKey.value = `${db}:${role}`
  try {
    await mongoUserRevoke({
      instanceId: props.instanceId,
      username: props.user.username,
      db,
      role: role as MongoRole
    })
    message.success(`已回收 ${props.user.username} @ ${db}：${role}`)
    emit('changed')
  } catch (e) {
    const text = errorText(e)
    if (text !== null) inlineError.value = text
  } finally {
    revokingKey.value = null
  }
}
</script>

<template>
  <n-modal
    v-model:show="show"
    preset="card"
    :title="`角色管理${props.user !== null ? '：' + props.user.username : ''}`"
    style="width: 560px"
    :mask-closable="false"
  >
    <template v-if="props.user !== null">
      <!-- 新增授权区 -->
      <div class="section-title">新增授权</div>
      <div class="grant-row">
        <n-select
          v-model:value="form.db"
          :options="dbOptions"
          filterable
          tag
          placeholder="目标库（可输入候选外库名）"
          class="grant-db"
        />
        <n-select v-model:value="form.role" :options="roleOptions" class="grant-role" />
        <n-button type="primary" :loading="granting" @click="onGrant">授予</n-button>
      </div>
      <n-alert type="info" :show-icon="false" style="margin-top: 8px">
        目标库不存在时 MongoDB 允许预授权（库建立后角色即生效）——候选外库名可手输。
      </n-alert>

      <!-- 既有授权区 -->
      <div class="section-title" style="margin-top: 16px">既有授权</div>
      <div v-if="props.user.roles.length === 0" class="meta">无任何角色</div>
      <div v-else class="roles">
        <div v-for="r in props.user.roles" :key="`${r.db}:${r.role}`" class="role-row">
          <span class="role-text">
            {{ r.db }}：{{ dictItem(MONGO_ROLE, r.role as MongoRole)?.label ?? r.role }}
            <span class="meta">（{{ r.role }}）</span>
          </span>
          <n-popconfirm @positive-click="onRevoke(r.db, r.role)">
            <template #trigger>
              <n-button
                size="tiny"
                type="error"
                ghost
                :loading="revokingKey === `${r.db}:${r.role}`"
                :disabled="revokingKey !== null"
              >
                回收
              </n-button>
            </template>
            确认回收 {{ r.db }}:{{ r.role }}？
          </n-popconfirm>
        </div>
      </div>

      <n-alert v-if="inlineError !== null" type="error" :show-icon="false" style="margin-top: 10px">
        {{ inlineError }}
      </n-alert>
    </template>
  </n-modal>
</template>

<style scoped>
.section-title {
  font-size: 13px;
  font-weight: 600;
  margin-bottom: 8px;
}
.grant-row {
  display: flex;
  gap: 8px;
  align-items: center;
}
.grant-db {
  flex: 1;
}
.grant-role {
  width: 180px;
}
.roles {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.role-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 4px 10px;
  border-radius: 4px;
  background: rgba(128, 128, 128, 0.07);
}
.role-text {
  font-size: 13px;
}
.meta {
  font-size: 12px;
  opacity: 0.65;
}
</style>
