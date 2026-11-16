<script setup lang="ts">
/**
 * 属主 Dialog（PageSpec 4.4.5，entryChown）。
 * 候选 systemAccountsList（打开时加载一次，D-14 实时读取不缓存）；属主用户/组 NSelect 可清空，
 * 至少填一项；成功 → 行 owner 直更（响应刷新值）；D-06 仅作用目标自身。
 */
import { computed, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import type { SelectOption } from 'naive-ui'
import { entryChown, systemAccountsList } from '@/api/file'
import { ApiError } from '@/api/http'
import type { FileEntry } from '@/types/file'
import { fileActionError } from '../errors'

const show = defineModel<boolean>('show', { default: false })

const props = defineProps<{
  entry: FileEntry | null
}>()

const emit = defineEmits<{
  done: [path: string, ownerUser: string, ownerGroup: string]
}>()

const message = useMessage()
const saving = ref(false)
const accountsLoading = ref(false)
const userOptions = ref<SelectOption[]>([])
const groupOptions = ref<SelectOption[]>([])
const ownerUser = ref<string | null>(null)
const ownerGroup = ref<string | null>(null)
const feedback = ref<string | null>(null)

const currentText = computed(() =>
  props.entry !== null ? `${props.entry.ownerUser} / ${props.entry.ownerGroup}` : ''
)

/** 打开时加载候选一次（D-14；不缓存）+ 预填当前值 */
watch(show, (v) => {
  if (v && props.entry !== null) {
    ownerUser.value = props.entry.ownerUser
    ownerGroup.value = props.entry.ownerGroup
    feedback.value = null
    accountsLoading.value = true
    void systemAccountsList()
      .then((data) => {
        userOptions.value = data.users.map((u) => ({ label: `${u.name} (${u.uid})`, value: u.name }))
        groupOptions.value = data.groups.map((g) => ({ label: `${g.name} (${g.gid})`, value: g.name }))
      })
      .catch(() => {
        // 候选加载失败已由拦截器提示；Dialog 保持可开（下拉为空，可手输候选不可选时重开重试）
      })
      .finally(() => {
        accountsLoading.value = false
      })
  }
})

async function onSubmit(): Promise<void> {
  if (props.entry === null) return
  if ((ownerUser.value === null || ownerUser.value === '') && (ownerGroup.value === null || ownerGroup.value === '')) {
    feedback.value = '属主用户与属主组至少填一项'
    return
  }
  feedback.value = null
  saving.value = true
  try {
    const data = await entryChown(props.entry.path, ownerUser.value ?? undefined, ownerGroup.value ?? undefined)
    message.success('属主已修改')
    show.value = false
    emit('done', data.path, data.ownerUser, data.ownerGroup)
  } catch (e) {
    if (e instanceof ApiError) {
      const info = fileActionError(e)
      if (info !== null) {
        if (info.level === 'warning') message.warning(info.text)
        else message.error(info.text)
      }
    }
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <n-modal
    :show="show"
    preset="card"
    title="属主"
    style="width: 440px"
    :mask-closable="false"
    @update:show="(v: boolean) => (show = v)"
  >
    <div class="current">当前属主：{{ currentText }}</div>
    <div class="fields">
      <n-select
        v-model:value="ownerUser"
        :options="userOptions"
        :loading="accountsLoading"
        clearable
        filterable
        placeholder="属主用户（可清空）"
      />
      <n-select
        v-model:value="ownerGroup"
        :options="groupOptions"
        :loading="accountsLoading"
        clearable
        filterable
        placeholder="属主组（可清空）"
      />
    </div>
    <div v-if="feedback !== null" class="feedback">{{ feedback }}</div>
    <div class="note">仅作用目标自身，不递归；候选为系统实况实时读取。</div>
    <template #footer>
      <div class="footer">
        <n-button @click="show = false">取消</n-button>
        <n-button type="primary" :loading="saving" @click="onSubmit">保存</n-button>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.current {
  font-size: 13px;
  margin-bottom: 12px;
  opacity: 0.75;
}
.fields {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.feedback {
  margin-top: 8px;
  font-size: 12px;
  color: crimson;
}
.note {
  margin-top: 12px;
  font-size: 12px;
  opacity: 0.6;
}
.footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
</style>
