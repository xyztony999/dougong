<script setup lang="ts">
/**
 * 计划任务页（PageSpec 4.1，路由 /cron；hybrid·Tab：计划任务 / 执行日志）。
 * Tab1：工具条（新建 admin / 刷新）+ 表格（序号/表达式/命令/备注/来源/操作列 admin）
 * + 系统预置位只读分区（NCollapse 默认折叠，无任何写操作，D1 口径-1）。
 * Tab2：执行日志（行数档位 + 刷新 + 等宽滚动文本区，text/plain 原文 D-04）。
 * readonly 子集（ADR-018 负空间）：无新建按钮、操作列整列不渲染；预置位/日志全可看。
 * 刷新纪律：进入页面拉 jobList；Tab 懒加载；写成功重拉 jobList；
 * jobTrigger 不重拉（代执行不改 crontab），结果经 TriggerResultModal 呈现（D-05）。
 */
import { computed, h, onMounted, onUnmounted, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { NButton, NPopconfirm, NTag, NTooltip } from 'naive-ui'
import type { DataTableColumns, SelectOption } from 'naive-ui'
import { jobDelete, jobTrigger } from '@/api/cron'
import { ApiError } from '@/api/http'
import { useAuthStore } from '@/stores/auth'
import { useCronStore } from '@/stores/cron'
import { CRON_SOURCE, dictItem } from '@/types/enums'
import type { DictItem } from '@/types/enums'
import { CB_ERROR_TEXT } from '@/types/cron'
import type { CronJobItem } from '@/types/cron'
import JobEditModal from './components/JobEditModal.vue'
import TriggerResultModal from './components/TriggerResultModal.vue'

const store = useCronStore()
const authStore = useAuthStore()
const message = useMessage()

/** ---- Tab 框架（懒加载：首次切执行日志 Tab 拉取；计划任务 Tab 由进入页面先行拉取） ---- */
const activeTab = ref<'jobs' | 'log'>('jobs')

watch(activeTab, (tab) => {
  if (tab === 'log' && !store.logLoaded) void store.loadLogTail()
})

/** ---- 写动作（4.1.3 在途互斥：行级 inFlight——触发期间三按钮禁用+触发 loading） ---- */

async function onTrigger(row: CronJobItem): Promise<void> {
  const key = `job-trigger:${row.jobKey}`
  store.setInFlight(key)
  try {
    // 同步等待最长 120s（D-05）；执行失败/超时为业务结果经 Modal 呈现，非请求错误
    const result = await jobTrigger({ jobKey: row.jobKey })
    store.showTriggerResult(row.expr, result)
  } catch (e) {
    if (e instanceof ApiError && e.is('CB_003')) {
      message.warning(CB_ERROR_TEXT['CB_003'])
      void store.loadJobs() // 实况投影：可能已在面板外删除（api D-01）
    }
    // 其余错误拦截器统一提示
  } finally {
    store.releaseInFlight(key)
  }
}

async function onDelete(row: CronJobItem): Promise<void> {
  const key = `job-del:${row.jobKey}`
  store.setInFlight(key)
  try {
    await jobDelete({ jobKey: row.jobKey })
    message.success('任务已删除')
    await store.loadJobs() // crontab 实况已变（第 2 节纪律）
  } catch (e) {
    if (e instanceof ApiError && e.is('CB_003')) {
      message.warning(CB_ERROR_TEXT['CB_003'])
      void store.loadJobs()
    }
  } finally {
    store.releaseInFlight(key)
  }
}

/** ---- Tab 1 表格列（4.1.2；readonly 操作列整列不渲染——负空间） ---- */

function dictTag(map: Readonly<Record<string, DictItem>>, value: string): ReturnType<typeof h> {
  const item = dictItem(map, value)
  return h(NTag, { type: item.tag, size: 'small', bordered: false }, { default: () => item.label })
}

const jobColumns = computed<DataTableColumns<CronJobItem>>(() => {
  const base: DataTableColumns<CronJobItem> = [
    { title: '序号', key: 'index', width: 60, render: (_row, index) => index + 1 },
    { title: '表达式', key: 'expr', width: 150, render: (row) => h('span', { class: 'mono' }, row.expr) },
    {
      title: '命令',
      key: 'command',
      width: 320,
      ellipsis: { tooltip: true },
      render: (row) => h('span', { class: 'mono' }, row.command) // 完整内容悬停可见
    },
    {
      title: '备注',
      key: 'remark',
      width: 160,
      render: (row) =>
        row.remark === null
          ? h('span', { class: 'muted' }, '—') // 无备注注释行（api D-02）
          : row.remark
    },
    {
      title: '来源',
      key: 'source',
      width: 100,
      render: (row) => dictTag(CRON_SOURCE, row.source)
    }
  ]
  if (authStore.isAdmin) {
    base.push({
      title: '操作',
      key: 'actions',
      width: 200,
      fixed: 'right',
      render: (row) => {
        const triggerKey = `job-trigger:${row.jobKey}`
        const delKey = `job-del:${row.jobKey}`
        const triggerInFlight = store.isInFlight(triggerKey)
        const delInFlight = store.isInFlight(delKey)
        // 行级在途互斥：手动触发期间三按钮禁用+触发按钮 loading（最长 120s 同步等待）
        const rowBusy = triggerInFlight || delInFlight
        return h('div', { class: 'row-actions' }, [
          h(
            NButton,
            {
              size: 'small',
              disabled: rowBusy,
              onClick: () => store.openJobForm(row)
            },
            { default: () => '编辑' }
          ),
          h(
            NPopconfirm,
            { onPositiveClick: () => void onTrigger(row) },
            {
              trigger: () =>
                h(
                  NTooltip,
                  { disabled: !triggerInFlight },
                  {
                    trigger: () =>
                      h(
                        NButton,
                        {
                          size: 'small',
                          type: 'primary',
                          ghost: true,
                          loading: triggerInFlight,
                          disabled: rowBusy && !triggerInFlight
                        },
                        { default: () => '手动触发' }
                      ),
                    default: () => '执行中，最长等待 120 秒'
                  }
                ),
              default: () => '由面板立即代为执行一次？最长等待 120 秒。'
            }
          ),
          h(
            NPopconfirm,
            { onPositiveClick: () => void onDelete(row) },
            {
              trigger: () =>
                h(
                  NButton,
                  {
                    size: 'small',
                    type: 'error',
                    ghost: true,
                    loading: delInFlight,
                    disabled: rowBusy && !delInFlight
                  },
                  { default: () => '删除' }
                ),
              default: () => '确认从 root crontab 删除该任务？同步移除面板标记注释。'
            }
          )
        ])
      }
    })
  }
  return base
})

/** readonly 视角无 fixed:right 操作列（负空间：fixed:right 列消失） */
const jobScrollX = computed(() => (authStore.isAdmin ? 990 : 790))

function jobRowKey(row: CronJobItem): string {
  return row.jobKey
}

/** ---- Tab 1 系统预置位分区（4.1.4 只读；api D-02 文件粒度） ---- */

/** 预置位文件条目表（表达式/命令两列，全部文件共用） */
const sysEntryColumns: DataTableColumns<{ expr: string; command: string }> = [
  { title: '表达式', key: 'expr', width: 150, render: (row) => h('span', { class: 'mono' }, row.expr) },
  { title: '命令', key: 'command', render: (row) => h('span', { class: 'mono' }, row.command) }
]

/** 表格上方灰字（4.1.1：面板外修改自动反映） */
const jobCountText = computed(() => `共 ${store.jobItems.length} 条任务 · 面板外修改自动反映`)

/** ---- Tab 2 执行日志（4.1.5） ---- */

const lineOptions: SelectOption[] = [50, 100, 200, 500, 1000].map((n) => ({
  label: `${n} 行`,
  value: n
}))

const logEmpty = computed(() => store.logTailText !== null && store.logTailText.trim() === '')

/** ---- 生命周期（进入页面 jobList；卸载清 Dialog/触发 Modal/inFlight） ---- */

onMounted(() => {
  void store.loadJobs()
})

onUnmounted(() => {
  store.resetCronView()
})
</script>

<template>
  <div class="page">
    <n-card size="small">
      <n-tabs v-model:value="activeTab" type="line">
        <!-- Tab 1 计划任务（4.1.1 工具条 + 4.1.2 表格 + 4.1.4 预置位分区） -->
        <n-tab-pane name="jobs" tab="计划任务">
          <div class="controls-row">
            <!-- 新建入口 admin 专属（readonly 负空间：按钮不渲染）；在途互斥期间禁用 -->
            <n-button
              v-if="authStore.isAdmin"
              type="primary"
              :disabled="store.hasInFlight"
              @click="store.openJobForm(null)"
            >
              新建任务
            </n-button>
            <n-button :loading="store.jobLoading" @click="store.loadJobs()">刷新</n-button>
          </div>
          <div class="meta">{{ jobCountText }}</div>
          <n-data-table
            :columns="jobColumns"
            :data="store.jobItems"
            :loading="store.jobLoading"
            :scroll-x="jobScrollX"
            :row-key="jobRowKey"
          >
            <template #empty>
              <n-empty description="root crontab 无任务" class="empty" />
            </template>
          </n-data-table>

          <!-- 系统预置位只读分区（默认折叠；不提供任何写操作，D1 口径-1） -->
          <n-collapse class="sys-collapse">
            <n-collapse-item name="system">
              <template #header>
                系统预置任务（只读）
                <span class="meta header-meta">系统预置位仅展示，不提供写操作</span>
              </template>
              <template v-if="store.systemEntries !== null">
                <div
                  v-for="file in store.systemEntries.cronDFiles"
                  :key="file.file"
                  class="sys-file-block"
                >
                  <div class="mono sys-file-name">/etc/cron.d/{{ file.file }}</div>
                  <n-data-table
                    size="small"
                    :columns="sysEntryColumns"
                    :data="file.entries"
                    :bordered="false"
                  />
                </div>
                <div
                  v-for="dir in store.systemEntries.cronDirs"
                  :key="dir.dir"
                  class="sys-dir-row"
                >
                  <span class="mono">{{ dir.dir }}</span>
                  <span class="mono muted scripts">{{ dir.scripts.join('　') }}</span>
                </div>
                <n-empty
                  v-if="
                    store.systemEntries.cronDFiles.length === 0 &&
                    store.systemEntries.cronDirs.length === 0
                  "
                  description="无系统预置任务"
                  class="empty"
                />
              </template>
            </n-collapse-item>
          </n-collapse>
        </n-tab-pane>

        <!-- Tab 2 执行日志（4.1.5：text/plain 原文等宽呈现） -->
        <n-tab-pane name="log" tab="执行日志">
          <div class="controls-row">
            <n-select
              :value="store.logLines"
              :options="lineOptions"
              class="ctrl-lines"
              @update:value="(v: number) => store.setLogLines(v)"
            />
            <n-button :loading="store.logLoading" @click="store.loadLogTail()">刷新</n-button>
          </div>
          <div class="meta">来源：系统 cron 服务日志（按发行版自动适配）</div>
          <n-spin v-if="store.logLoading && store.logTailText === null" size="small" class="log-spin" />
          <n-empty v-else-if="logEmpty" description="暂无 cron 执行日志" class="empty" />
          <pre v-else-if="store.logTailText !== null" class="log-output mono">{{ store.logTailText }}</pre>
        </n-tab-pane>
      </n-tabs>
    </n-card>

    <!-- 新建/编辑任务 Dialog（admin 专属；readonly 不渲染——无入口即无表单形态，负空间第 2 节） -->
    <job-edit-modal v-if="authStore.isAdmin" v-model:show="store.jobFormVisible" @changed="store.loadJobs()" />

    <!-- 手动触发结果 Modal（5.2；数据为 admin 专属触发动作产物，readonly 不可达） -->
    <trigger-result-modal />
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.controls-row {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 8px;
}
.ctrl-lines {
  width: 140px;
}
.meta {
  font-size: 12px;
  opacity: 0.6;
  margin-bottom: 8px;
}
.header-meta {
  margin-left: 12px;
  margin-bottom: 0;
}
.muted {
  opacity: 0.55;
}
.mono {
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', monospace;
  font-size: 12px;
}
.row-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.empty {
  padding: 24px 0;
}
.sys-collapse {
  margin-top: 16px;
}
.sys-file-block {
  margin-bottom: 12px;
}
.sys-file-name {
  font-weight: 600;
  margin-bottom: 4px;
}
.sys-dir-row {
  display: flex;
  align-items: baseline;
  gap: 16px;
  padding: 2px 0;
}
.sys-dir-row .scripts {
  word-break: break-all;
}
.log-spin {
  display: inline-flex;
  padding: 24px 0;
}
.log-output {
  max-height: 480px;
  overflow: auto;
  margin: 0;
  padding: 8px;
  border-radius: 4px;
  background: rgba(128, 128, 128, 0.08);
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
