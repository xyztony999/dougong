<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { useMonitorStore } from '@/stores/monitoring'
import ProcessesCard from './components/ProcessesCard.vue'
import SnapshotCard from './components/SnapshotCard.vue'
import SystemInfoCard from './components/SystemInfoCard.vue'

/**
 * 监控概览（PageSpec 6.1，路由 /monitoring，admin+readonly 完全一致——ADR-018 全可看）：
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

    <!-- Card A 系统信息（60s 慢轮询） / Card B 实时快照（3s） / Card C TOP 进程（快照同源） -->
    <div class="cards">
      <SystemInfoCard :info="store.systemInfoData" />
      <SnapshotCard :snapshot="store.latestSnapshot" />
      <ProcessesCard :processes="store.latestSnapshot?.processes ?? null" />
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
.cards {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
</style>
