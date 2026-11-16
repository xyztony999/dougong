<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { usePanelStore } from '@/stores/panel'
import BindDomainCard from './components/BindDomainCard.vue'
import EntryPathCard from './components/EntryPathCard.vue'
import HttpsCard from './components/HttpsCard.vue'
import LangCard from './components/LangCard.vue'
import ListenCard from './components/ListenCard.vue'
import PanelCertCard from './components/PanelCertCard.vue'
import SessionCard from './components/SessionCard.vue'
import UpdateCheckCard from './components/UpdateCheckCard.vue'

const panelStore = usePanelStore()

/**
 * 页面级横幅（PageSpec 5.1）：
 * - pendingRestart=true → 待重启警示（不可手动消除，重启后随 false 自动消失，D-P02）
 * - HTTP 明文 + https.mode=off → 建议启用 HTTPS（FR-07 建议但不强制；模式非 off 不展示）
 */
const showHttpHint = computed(
  () => window.location.protocol === 'http:' && panelStore.configDetail?.https.mode === 'off'
)

function retry(): void {
  void panelStore.loadConfig()
  void panelStore.loadCertStatus()
}

onMounted(() => {
  // configDetail 与 certStatus 并行拉取（PageSpec 5.1 Card 5 数据源；两角色均可见）
  void panelStore.loadConfig()
  void panelStore.loadCertStatus()
})
</script>

<template>
  <div class="page">
    <n-alert
      v-if="panelStore.pendingRestart"
      type="warning"
      title="监听配置待重启生效"
      :closable="false"
    >
      监听配置已修改，重启服务后生效（systemctl restart dougong）
    </n-alert>
    <n-alert v-if="showHttpHint" type="info" title="建议启用 HTTPS" :closable="false">
      当前使用 HTTP 明文连接，建议启用 HTTPS（设置页下方可配置）
    </n-alert>

    <n-spin v-if="panelStore.configLoading && panelStore.configDetail === null" class="loading">
      <div class="loading-inner">正在加载面板配置…</div>
    </n-spin>
    <n-result
      v-else-if="panelStore.configError && panelStore.configDetail === null"
      status="error"
      title="配置加载失败"
      description="请检查面板服务是否可达后重试。"
    >
      <template #footer>
        <n-button type="primary" :loading="panelStore.configLoading" @click="retry">重试</n-button>
      </template>
    </n-result>

    <!-- 8 Card 各自独立提交（PageSpec 5.1 v1.2：四分组重排——Card 语义编号不变，
         呈现顺序按分组；v0.1.2 反馈第 7 项「卡片顺序跳跃」收敛） -->
    <template v-else-if="panelStore.configDetail !== null">
      <div class="group-title">常规</div>
      <lang-card :detail="panelStore.configDetail" />
      <div class="group-title">网络与访问</div>
      <listen-card :detail="panelStore.configDetail" />
      <entry-path-card :detail="panelStore.configDetail" />
      <bind-domain-card :detail="panelStore.configDetail" />
      <div class="group-title">安全与证书</div>
      <https-card :detail="panelStore.configDetail" />
      <panel-cert-card :detail="panelStore.configDetail" />
      <session-card :detail="panelStore.configDetail" />
      <div class="group-title">关于</div>
      <update-check-card :detail="panelStore.configDetail" />
    </template>
  </div>
</template>

<style scoped>
/* 分组标题（PageSpec 5.1 v1.2 四分组视觉层级：分组标题—Card 两级） */
.group-title {
  font-size: 13px;
  font-weight: 600;
  opacity: 0.55;
  margin: 8px 0 -4px;
}
.page {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-width: 760px;
}
.loading {
  padding: 48px 0;
}
.loading-inner {
  padding: 24px 48px;
  font-size: 13px;
  opacity: 0.65;
}
</style>
