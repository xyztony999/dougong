<script setup lang="ts">
/**
 * 资源计数条（2026-11-23 监控概览重排，frontend-design）：面板首页第一屏=
 * 「这台服务器上有什么」——网站/数据库/容器/应用/环境 五个计数格，单条横排
 * （非五张同款卡片），格间细分隔线；点击直达对应模块。计数经既有 list 接口
 * 并行拉取（allSettled：单模块不可用〔如未装 Docker〕不阻断其余，置 — 不虚构）。
 * readonly 同可看（ADR-018 列表全可看）。
 */
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { NIcon } from 'naive-ui'
import {
  GlobeOutline,
  ServerOutline,
  CubeOutline,
  AppsOutline,
  CloudDownloadOutline
} from '@vicons/ionicons5'
import { siteList } from '@/api/site'
import { instanceList } from '@/api/database'
import { containerList } from '@/api/docker'
import { appList } from '@/api/app'
import { envList } from '@/api/env'

const router = useRouter()

interface Cell {
  key: string
  label: string
  path: string
  icon: typeof GlobeOutline
  count: number | null
}

const cells = ref<Cell[]>([
  { key: 'sites', label: '网站', path: '/sites?tab=site', icon: GlobeOutline, count: null },
  { key: 'databases', label: '数据库', path: '/databases', icon: ServerOutline, count: null },
  { key: 'containers', label: '容器', path: '/docker', icon: CubeOutline, count: null },
  // 「应用管理」独立页面已并入网站管理页的项目分栏（ADR-033）：计数口径不变（Python + Node 项目总数）。
  { key: 'apps', label: '项目', path: '/sites?tab=python', icon: AppsOutline, count: null },
  { key: 'envs', label: '环境', path: '/env', icon: CloudDownloadOutline, count: null }
])

function setCount(key: string, n: number | null): void {
  const c = cells.value.find((x) => x.key === key)
  if (c !== undefined) c.count = n
}

onMounted(async () => {
  const [sites, dbs, containers, apps, envs] = await Promise.allSettled([
    siteList(),
    instanceList(),
    containerList(),
    appList(),
    envList()
  ])
  setCount('sites', sites.status === 'fulfilled' ? sites.value.items.length : null)
  setCount('databases', dbs.status === 'fulfilled' ? dbs.value.items.length : null)
  setCount('containers', containers.status === 'fulfilled' ? containers.value.items.length : null)
  setCount('apps', apps.status === 'fulfilled' ? apps.value.items.length : null)
  setCount(
    'envs',
    envs.status === 'fulfilled' ? envs.value.items.filter((e) => e.installed === true).length : null
  )
})

function display(n: number | null): string {
  return n === null ? '—' : String(n)
}

function go(path: string): void {
  void router.push(path)
}
</script>

<template>
  <n-card size="small" :bordered="true" class="resource-bar">
    <div class="row">
      <button
        v-for="c in cells"
        :key="c.key"
        type="button"
        class="cell"
        :title="`查看${c.label}`"
        @click="go(c.path)"
      >
        <span class="num">{{ display(c.count) }}</span>
        <span class="lab">
          <n-icon :component="c.icon" :size="14" class="lab-icon" />
          {{ c.label }}
        </span>
      </button>
    </div>
  </n-card>
</template>

<style scoped>
.resource-bar :deep(.n-card__content) {
  padding: 8px 4px;
}
.row {
  display: flex;
  align-items: stretch;
}
.cell {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  padding: 10px 8px;
  background: transparent;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  font: inherit;
  color: inherit;
  transition: background-color 0.15s ease;
}
.cell + .cell {
  border-left: 1px solid rgba(128, 128, 128, 0.22);
  border-radius: 0;
}
.cell:hover {
  background: rgba(128, 128, 128, 0.12);
}
.cell:focus-visible {
  outline: 2px solid rgba(128, 128, 128, 0.5);
  outline-offset: -2px;
}
.num {
  font-size: 26px;
  font-weight: 650;
  line-height: 1.1;
  font-variant-numeric: tabular-nums;
}
.lab {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  opacity: 0.7;
}
.lab-icon {
  opacity: 0.8;
}
@media (max-width: 640px) {
  .row {
    flex-wrap: wrap;
  }
  .cell {
    flex: 1 1 33%;
  }
  .cell + .cell {
    border-left: none;
  }
}
</style>