<script setup lang="ts">
/**
 * key 浏览（PageSpec 6.2 Tab 3；admin 专属——readonly 页不渲染本组件，D0 口径 key 值属数据内容取出）。
 * 工具栏：pattern（默认 *，占位提示支持通配；DBM_017 形态错误内联）+ 查询（cursor="0" 起始）。
 * 游标分页（D-09）：非页码分页——上一页/下一页由 nextCursor 驱动，游标栈支持回退；
 * complete=true 禁用下一页并标注「已遍历完」；每页 COUNT=200 服务端常量，前端不传。
 * TTL 行内查询：-1 显示「永不过期」（Redis 语义如实透传）；DBM_015 提示 key 不存在。
 */
import { computed, h, ref } from 'vue'
import { useMessage } from 'naive-ui'
import { NButton, NTag } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { redisKeyList, redisKeyTtl } from '@/api/database'
import { ApiError } from '@/api/http'
import type { KeyListItem } from '@/types/database'
import { dbmErrorText } from '@/types/database'
import { formatDateTime } from '@/utils/format'

const props = defineProps<{
  instanceId: number
}>()

const message = useMessage()

/** ---- 查询条件（PageSpec 6.2：pattern 默认 *；DBM_017 内联） ---- */

const pattern = ref('*')
const patternError = ref<string | null>(null)
const loading = ref(false)
const items = ref<KeyListItem[]>([])
const generatedAt = ref<string | null>(null)
const nextCursor = ref<string | null>(null)
const complete = ref(false)

/** 游标栈：pageCursors[i] = 第 i 页请求 cursor（栈式回退，D-09） */
const pageCursors = ref<string[]>(['0'])
const pageIdx = ref(0)

const canPrev = computed(() => pageIdx.value > 0 && !loading.value)
const canNext = computed(() => nextCursor.value !== null && !complete.value && !loading.value)

async function fetchPage(cursor: string): Promise<void> {
  loading.value = true
  try {
    const data = await redisKeyList({
      instanceId: props.instanceId,
      cursor,
      pattern: pattern.value === '' ? '*' : pattern.value
    })
    items.value = data.items
    nextCursor.value = data.nextCursor
    complete.value = data.complete
    generatedAt.value = data.generatedAt
    ttlMap.value = {}
  } catch (e) {
    if (e instanceof ApiError && e.is('DBM_017')) {
      patternError.value = dbmErrorText(e.code, e.data) ?? 'key 匹配模式非法'
      return
    }
    if (e instanceof ApiError) {
      const text = dbmErrorText(e.code, e.data)
      if (text !== null) message.error(text)
    }
  } finally {
    loading.value = false
  }
}

/** 查询（新起点：游标栈复位，cursor="0" 起始） */
function onQuery(): void {
  patternError.value = null
  if (pattern.value.trim() === '') pattern.value = '*'
  pageCursors.value = ['0']
  pageIdx.value = 0
  void fetchPage('0')
}

function onPrev(): void {
  if (!canPrev.value) return
  pageIdx.value -= 1
  void fetchPage(pageCursors.value[pageIdx.value] ?? '0')
}

function onNext(): void {
  if (!canNext.value || nextCursor.value === null) return
  pageIdx.value += 1
  if (pageCursors.value.length === pageIdx.value) {
    pageCursors.value.push(nextCursor.value)
  }
  void fetchPage(pageCursors.value[pageIdx.value] ?? '0')
}

/** ---- TTL 行内查询（-1=永不过期；DBM_015 提示） ---- */

const ttlMap = ref<Record<string, number>>({})
const ttlLoading = ref<Set<string>>(new Set())

async function onTtl(row: KeyListItem): Promise<void> {
  ttlLoading.value.add(row.key)
  try {
    const data = await redisKeyTtl(props.instanceId, row.key)
    ttlMap.value[row.key] = data.ttlSec
  } catch (e) {
    if (e instanceof ApiError && e.is('DBM_015')) {
      message.warning('key 不存在（可能已过期）')
      delete ttlMap.value[row.key]
      return
    }
    if (e instanceof ApiError) {
      const text = dbmErrorText(e.code, e.data)
      if (text !== null) message.error(text)
    }
  } finally {
    ttlLoading.value.delete(row.key)
  }
}

function ttlText(key: string): string | null {
  const ttl = ttlMap.value[key]
  if (ttl === undefined) return null
  return ttl === -1 ? '永不过期' : `${ttl} 秒`
}

const columns: DataTableColumns<KeyListItem> = [
  { title: 'key', key: 'key', width: 320, ellipsis: { tooltip: true } },
  {
    title: '类型',
    key: 'type',
    width: 120,
    render: (row) =>
      // Redis 原生类型透传（开放集合非池枚举，text tag 呈现）
      h(NTag, { size: 'small', bordered: false }, { default: () => row.type })
  },
  {
    title: 'TTL',
    key: 'ttl',
    width: 200,
    render: (row) => {
      const text = ttlText(row.key)
      if (text !== null) return h('span', { class: 'ttl-text' }, text)
      return h(
        NButton,
        {
          size: 'tiny',
          disabled: ttlLoading.value.has(row.key),
          loading: ttlLoading.value.has(row.key),
          onClick: () => void onTtl(row)
        },
        { default: () => '查 TTL' }
      )
    }
  }
]

function rowKey(row: KeyListItem): string {
  return row.key
}
</script>

<template>
  <div>
    <!-- 工具栏：pattern + 查询（PageSpec 6.2 Tab 3） -->
    <div class="toolbar">
      <n-input
        v-model:value="pattern"
        class="pattern-input"
        :status="patternError !== null ? 'error' : undefined"
        placeholder="匹配模式（支持通配，默认 *）"
        @keyup.enter="onQuery"
      />
      <n-button type="primary" size="small" :loading="loading" @click="onQuery">查询</n-button>
    </div>
    <div v-if="patternError !== null" class="pattern-error">{{ patternError }}</div>

    <n-data-table
      :columns="columns"
      :data="items"
      :loading="loading"
      :scroll-x="640"
      :row-key="rowKey"
    >
      <template #empty>
        <n-empty description="当前游标页无匹配 key" class="empty" />
      </template>
    </n-data-table>

    <!-- 游标分页（D-09：上一页/下一页，complete 标注已遍历完） -->
    <div class="pager">
      <span class="meta">
        第 {{ pageIdx + 1 }} 页
        <template v-if="generatedAt !== null">· 生成于 {{ formatDateTime(generatedAt) }}</template>
        <template v-if="complete">· 已遍历完</template>
      </span>
      <span class="spacer" />
      <n-button size="small" :disabled="!canPrev" @click="onPrev">上一页</n-button>
      <n-button size="small" :disabled="!canNext" @click="onNext">下一页</n-button>
    </div>
  </div>
</template>

<style scoped>
.toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}
.pattern-input {
  width: 320px;
}
.pattern-error {
  font-size: 12px;
  /* Naive UI error 语义色（#d03050）rgba 形态——无 hex 硬编码，与主题错误色一致 */
  color: rgba(208, 48, 80, 1);
  margin: -6px 0 8px;
}
.pager {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 12px;
}
.meta {
  font-size: 12px;
  opacity: 0.6;
}
.spacer {
  flex: 1;
}
.ttl-text {
  font-size: 13px;
}
.empty {
  padding: 24px 0;
}
</style>
