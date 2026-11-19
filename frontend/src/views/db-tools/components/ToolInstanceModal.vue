<script setup lang="ts">
/**
 * 实例选择弹窗（PageSpec db-tools 4.1 mongo-express 安装 / 4.4 phpmyadmin 打开共用）：
 * 数据源=跨模块 instanceList（api/database.ts）按 engines 筛选；选项呈现
 * identityText + 引擎 + credentialsSet 状态；credentialsSet=false 选项禁用 +
 * tooltip「请先到数据库管理配置凭据」+ 链接 /databases（DBT_006 前置）。
 * 单实例时自动选中（4.4 免弹窗口径由父级承载——本弹窗内单选项亦预选）。
 */
import { computed, h, ref, watch } from 'vue'
import { NTag, NTooltip } from 'naive-ui'
import type { SelectOption } from 'naive-ui'
import { instanceList } from '@/api/database'
import type { DbInstanceItem } from '@/types/database'
import { DB_ENGINE, dictItem } from '@/types/enums'
import type { DbEngine } from '@/types/enums'
import { instanceIdentityText } from '@/types/dbtool'

const props = defineProps<{
  /** 目标引擎集（mongo_express 安装=[mongodb]；phpmyadmin 打开=[mysql,mariadb]） */
  engines: DbEngine[]
  title: string
  intro: string
  okText?: string
}>()

const emit = defineEmits<{
  select: [instanceId: number]
}>()

const show = defineModel<boolean>('show', { default: false })

const instances = ref<DbInstanceItem[]>([])
const loading = ref(false)
const loadError = ref(false)
const selected = ref<number | null>(null)

const candidates = computed(() =>
  instances.value.filter((it) => props.engines.includes(it.engine))
)

const anyUnready = computed(() => candidates.value.some((it) => !it.credentialsSet))

async function load(): Promise<void> {
  loading.value = true
  loadError.value = false
  try {
    const data = await instanceList()
    instances.value = data.items
    // 单候选自动选中（4.4 单实例免选择口径）
    const ready = candidates.value.filter((it) => it.credentialsSet)
    selected.value = ready.length === 1 ? ready[0].id : null
  } catch {
    loadError.value = true
  } finally {
    loading.value = false
  }
}

watch(show, (v) => {
  if (v) {
    selected.value = null
    void load()
  }
})

const options = computed<SelectOption[]>(() =>
  candidates.value.map((it) => ({
    label: `${instanceIdentityText(it)}（${dictItem(DB_ENGINE, it.engine).label}）`,
    value: it.id,
    disabled: !it.credentialsSet,
    credentialsSet: it.credentialsSet
  }))
)

/** 禁用选项 tooltip（credentialsSet=false → 引导配置凭据，DBT_006 前置） */
function renderLabel(option: SelectOption): ReturnType<typeof h> {
  const label = String(option.label ?? '')
  if (option.disabled === true) {
    return h(
      NTooltip,
      null,
      {
        trigger: () => h('span', { style: 'opacity: 0.55' }, `${label} · 凭据未配置`),
        default: () => '请先到数据库管理配置凭据'
      }
    )
  }
  return h('span', null, [
    label,
    h(
      NTag,
      { type: 'success', size: 'small', bordered: false, style: 'margin-left: 8px' },
      { default: () => '凭据已配置' }
    )
  ])
}

function onConfirm(): void {
  if (selected.value !== null) emit('select', selected.value)
}
</script>

<template>
  <n-modal
    v-model:show="show"
    preset="card"
    :title="title"
    style="width: 560px"
    :mask-closable="false"
  >
    <n-alert type="info" :show-icon="true" style="margin-bottom: 12px">{{ intro }}</n-alert>
    <n-spin :show="loading">
      <n-select
        v-model:value="selected"
        :options="options"
        :render-label="renderLabel"
        placeholder="选择目标实例"
        :disabled="loadError"
      />
      <div v-if="anyUnready" class="hint">
        凭据未配置的实例不可选——
        <router-link to="/databases">到数据库管理配置凭据</router-link>
      </div>
      <n-alert v-if="loadError" type="error" style="margin-top: 10px" title="实例清单加载失败">
        <n-button size="tiny" @click="load">重试</n-button>
      </n-alert>
      <n-empty
        v-if="!loading && !loadError && candidates.length === 0"
        description="未发现目标引擎实例（可到数据库管理重新探测）"
        size="small"
        style="margin-top: 10px"
      />
    </n-spin>
    <template #footer>
      <div class="footer">
        <n-button @click="show = false">取消</n-button>
        <n-button type="primary" :disabled="selected === null" @click="onConfirm">
          {{ okText ?? '确定' }}
        </n-button>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.hint {
  margin-top: 8px;
  font-size: 12px;
  opacity: 0.8;
}
.footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
</style>
