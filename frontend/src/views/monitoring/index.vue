<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { useMonitorStore } from '@/stores/monitoring'
import ProcessesCard from './components/ProcessesCard.vue'
import ResourceBar from './components/ResourceBar.vue'
import SnapshotCard from './components/SnapshotCard.vue'
import SystemInfoCard from './components/SystemInfoCard.vue'

/**
 * 监控概览（PageSpec 6.1，路由 /monitoring，admin+readonly 完全一致——ADR-018 全可看；
 * 2026-11-23 起为登录后默认首页）：重排（frontend-design）——第一屏资源计数条
 * （网站/数据库/容器/应用/环境，点击直达）+ 实时快照/系统信息双栏 + TOP 进程跨栏。
 * 轮询生命周期与页面挂载/卸载配对——快照 3s + 系统信息 60s（store 内集中管理，
 * document.hidden 暂停、恢复可见立即拉取；🔴 onUnmounted 清全部定时器与监听）。
 */
const store = useMonitorStore()

onMounted(() => {
  store.startRealtimePolling()
})

onUnmounted(() => {
  store.stopRealtimePolling()
})
</script>

<template>
  <div class="page">
    <div class="page-head">
      <h3 class="page-title">监控概览</h3>
      <!-- 数据滞后/连接中断顶角角标（orange；保留上次数据，恢复后自动消失——轮询不停） -->
      <div class="badges">
        <n-tag v-if="store.connectionLost" type="warning" size="small" :bordered="false">
          连接中断
        </n-tag>
        <n-tag v-else-if="store.stale" type="warning" size="small" :bordered="false">
          数据滞后
        </n-tag>
      </div>
    </div>

    <!-- degraded 横幅（D-08：部分指标采集失败，缺失项暂不显示，不虚构数据） -->
    <n-alert v-if="store.latestSnapshot?.degraded === true" type="info" class="banner">
      本轮部分指标采集失败，缺失项暂不显示，不影响其他指标
    </n-alert>

    <!-- MON_004 空态：启动后尚无成功采样（内联提示，轮询不停止） -->
    <n-alert v-if="store.initializing" type="default" class="banner">监控数据初始化中</n-alert>

    <!-- 第一屏：资源计数条（这台服务器上有什么；点击直达模块） -->
    <ResourceBar />

    <!-- 实时快照（3s）+ 系统信息（60s）双栏；TOP 进程（快照同源）跨栏 -->
    <div class="monitor-grid">
      <SnapshotCard :snapshot="store.latestSnapshot" />
      <SystemInfoCard :info="store.systemInfoData" />
      <ProcessesCard :processes="store.latestSnapshot?.processes ?? null" class="span-all" />
    </div>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.page-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.page-title {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
}
.banner {
  margin-bottom: 0;
}
.monitor-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
  align-items: start;
}
.span-all {
  grid-column: 1 / -1;
}
@media (max-width: 900px) {
  .monitor-grid {
    grid-template-columns: 1fr;
  }
}
</style>
