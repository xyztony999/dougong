<script setup lang="ts">
/**
 * 环境管理页（PageSpec env-mgmt v1.1，hybrid；路由 /env；admin+readonly 同路由
 * 按角色渲染子集——操作区整区不渲染，ADR-018 负空间）。
 * 页头通道状态条 + 环境卡片网格（七条目 D-02 v1.1 +MongoDB：目录字段+实况+
 * 在途标记；实况三态 ⚠️A9——通道级不可用归页头横幅、条目级真错误才显
 * 「探测不可用」角标、未装含包名不在包数据库形态）+ 任务 Drawer（历史筛选
 * 列表 / 详情输出尾部 running 5s 轮询终态即停）。安装/升级 NPopconfirm（刷新
 * 索引+第三方源代配提示 ⚠️A10）；卸载强确认 NModal 两段警示（⚠️A6：本域服务
 * 实况+关联模块静态影响）+ 输入环境名锁定；服务操作组（D-09，单元全名 ⚠️A8）。
 */
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useMessage } from 'naive-ui'
import { envInstall } from '@/api/env'
import { ApiError } from '@/api/http'
import { useAuthStore } from '@/stores/auth'
import { useEnvStore } from '@/stores/env'
import { PACKAGE_MANAGER_FAMILY, dictItem } from '@/types/enums'
import type { EnvItem } from '@/types/env'
import { formatTime } from '@/utils/format'
import EnvTaskDrawer from './components/EnvTaskDrawer.vue'
import { ENV_ACCENT, ENV_MONOGRAM } from './envMeta'
import { envTaskColumns, envTypeOptions, statusOptions } from './taskColumns'
import { useRouter } from 'vue-router'

const authStore = useAuthStore()
const store = useEnvStore()
const message = useMessage()
const router = useRouter()

/** ---- 任务 Drawer ---- */

const drawerShow = ref(false)

function openTasks(): void {
  drawerShow.value = true
  void store.loadTasks(1)
}

function onDrawerTaskClick(taskId: number): void {
  void store.loadTaskDetail(taskId)
}

/** 在途任务「查看」引导（卡片标记 → Drawer 定位任务） */
function viewRunningTask(taskId: number): void {
  drawerShow.value = true
  void store.loadTasks(1).then(() => store.loadTaskDetail(taskId))
}

/** ---- 安装/升级（NPopconfirm 文案含刷新索引与仓库代配提示，D-05/D-06） ---- */

const installing = ref<string | null>(null)

async function onInstall(item: EnvItem): Promise<void> {
  installing.value = item.envType
  try {
    const res = await envInstall(item.envType)
    message.success(res.action === 'upgrade' ? '升级任务已创建' : '安装任务已创建')
    await store.refreshAfterWrite()
    viewRunningTask(res.taskId)
  } catch (e) {
    handleEnvError(e)
  } finally {
    installing.value = null
  }
}

/** env 域错误分流（ENV_001~006；文本与 api 3.4 一一对应） */
function handleEnvError(e: unknown): void {
  if (!(e instanceof ApiError)) return
  if (e.is('ENV_001')) {
    message.error('包管理器通道不可用（apt/dnf·yum 均未检出）——变更功能不可用')
    return
  }
  if (e.is('ENV_003')) {
    const taskId = (e.data as Record<string, unknown> | null)?.taskId
    message.error('该环境已有任务进行中')
    if (typeof taskId === 'number') viewRunningTask(taskId)
    return
  }
  if (e.is('ENV_004')) {
    message.error('环境未安装')
    void store.loadList()
    return
  }
  message.error(e.message)
}

/** ---- 通道状态条 ---- */

const channelTag = computed(() => {
  if (!store.channel.available) {
    return { text: '包管理通道不可用（变更功能已禁用）', type: 'warning' as const }
  }
  const label = store.channel.family != null ? dictItem(PACKAGE_MANAGER_FAMILY, store.channel.family).label : ''
  return { text: `包管理通道：${label}`, type: 'success' as const }
})

/** 卡片实况渲染辅助（v1.1 三态 ⚠️A9：installed=null 仅条目级真错误显橙——通道级
 * 不可用由页头横幅承载语义，角标降级灰「未探测」不重复渲染；false=未安装含
 * 包名不在包数据库形态） */
function installedText(item: EnvItem): { text: string; type: 'default' | 'success' | 'warning' } {
  if (item.installed === null) {
    return store.channel.available
      ? { text: '探测不可用', type: 'warning' }
      : { text: '未探测', type: 'default' }
  }
  if (!item.installed) return { text: '未安装', type: 'default' }
  return { text: `已安装 · ${item.version ?? ''}`, type: 'success' }
}

const taskColumns = computed(() => envTaskColumns(onDrawerTaskClick))

onMounted(() => {
  void store.loadList()
})

onUnmounted(() => {
  store.stopPoll()
  store.clearTaskDetail()
})
</script>

<template>
  <div class="page">
    <!-- 页头：通道状态条 + 刷新 + 任务入口 -->
    <n-card size="small">
      <div class="head-row">
        <n-tag :type="channelTag.type" size="small">{{ channelTag.text }}</n-tag>
        <span class="meta">探测于 {{ formatTime(store.generatedAt) }}</span>
        <div class="spacer"></div>
        <n-button size="small" :loading="store.listLoading" @click="store.loadList()">刷新</n-button>
        <n-button size="small" @click="openTasks">任务记录</n-button>
      </div>
      <n-alert
        v-if="store.listError"
        type="error"
        closable
        style="margin-top: 8px"
        title="环境状态加载失败"
      >
        环境列表加载失败，请重试。
        <n-button size="tiny" @click="store.loadList()">重试</n-button>
      </n-alert>
    </n-card>

    <div class="grid">
      <article
        v-for="item in store.items"
        :key="item.envType"
        class="env-tile"
        :class="{ clickable: item.installed === true }"
        @click="item.installed === true && router.push(`/env/${item.envType}`)"
      >
        <span class="rail" :style="{ background: ENV_ACCENT[item.envType] }"></span>
        <div class="tile-body">
          <div class="tile-head">
            <span class="mono-mark" :style="{ color: ENV_ACCENT[item.envType] }">
              {{ ENV_MONOGRAM[item.envType] }}
            </span>
            <div class="tile-id">
              <div class="env-title">
                {{ item.displayName }}
                <n-tag v-if="item.runningTaskId !== null" type="info" size="small" :bordered="false">
                  任务进行中
                </n-tag>
              </div>
              <div class="env-desc">{{ item.description }}</div>
            </div>
            <n-tag :type="installedText(item).type" size="small" :bordered="false">
              {{ installedText(item).text }}
            </n-tag>
          </div>
          <div v-if="item.installed !== false && item.unitName !== ''" class="env-service">
            <template v-if="item.serviceRunning !== null">
              <span class="dot" :class="{ on: item.serviceRunning }"></span>
              {{ item.serviceRunning ? '运行中' : '已停止' }}
              <span class="sep">·</span>
              自启{{ item.serviceEnabled ? '开' : '关' }}
            </template>
            <span v-else class="env-unit-none">服务状态未知</span>
          </div>
          <div class="tile-foot" @click.stop>
            <n-button
              v-if="item.runningTaskId !== null"
              size="tiny"
              text
              type="primary"
              @click="viewRunningTask(item.runningTaskId ?? 0)"
            >
              查看任务
            </n-button>
            <template v-if="authStore.isAdmin && store.channel.available">
              <n-button
                v-if="item.installed === true"
                size="small"
                type="primary"
                @click="router.push(`/env/${item.envType}`)"
              >
                设置
              </n-button>
              <n-popconfirm v-else-if="item.installed === false" @positive-click="onInstall(item)">
                <template #trigger>
                  <n-button size="small" type="primary" :loading="installing === item.envType">
                    安装
                  </n-button>
                </template>
                将安装 {{ item.displayName }}，大约几分钟。
              </n-popconfirm>
            </template>
          </div>
        </div>
      </article>
    </div>

    <!-- 任务 Drawer -->
    <env-task-drawer
      v-model:show="drawerShow"
      :columns="taskColumns"
      :env-type-options="envTypeOptions"
      :status-options="statusOptions"
      @refresh="store.loadTasks(1)"
    />

  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.head-row {
  display: flex;
  align-items: center;
  gap: 12px;
}
.spacer {
  flex: 1;
}
.meta {
  font-size: 12px;
  opacity: 0.65;
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 14px;
}
.env-tile {
  display: flex;
  border-radius: 10px;
  overflow: hidden;
  border: 1px solid var(--n-border-color);
  background: var(--n-color, transparent);
  min-height: 148px;
}
.env-tile.clickable {
  cursor: pointer;
}
.env-tile.clickable:hover {
  border-color: rgba(24, 160, 88, 0.45);
}
.rail {
  width: 4px;
  flex-shrink: 0;
}
.tile-body {
  flex: 1;
  padding: 14px 16px 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-width: 0;
}
.tile-head {
  display: flex;
  gap: 12px;
  align-items: flex-start;
}
.mono-mark {
  width: 40px;
  height: 40px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-family: ui-monospace, SFMono-Regular, Consolas, 'Courier New', monospace;
  font-weight: 700;
  font-size: 13px;
  letter-spacing: 0.02em;
  background: rgba(127, 127, 127, 0.12);
  flex-shrink: 0;
}
.tile-id {
  flex: 1;
  min-width: 0;
}
.env-title {
  font-weight: 600;
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.env-desc {
  margin-top: 2px;
  font-size: 12px;
  opacity: 0.7;
  line-height: 1.4;
}
.env-service {
  font-size: 12px;
  opacity: 0.8;
  display: flex;
  align-items: center;
  gap: 6px;
}
.dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #6b7280;
}
.dot.on {
  background: #18a058;
}
.sep {
  opacity: 0.4;
}
.env-unit-none {
  font-size: 12px;
  opacity: 0.55;
}
.tile-foot {
  margin-top: auto;
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  align-items: center;
}
</style>
