<script setup lang="ts">
/**
 * 网站管理页外层（PageSpec site-mgmt v1.3，ADR-033）：三个分栏——「网站」（既有
 * nginx 站点：静态 / PHP / 反向代理，内容与交互原样）、「Python 项目」、「Node 项目」
 * （后两者为运行时项目面板，按运行时各挂一份）。
 *
 * 当前分栏写入地址栏参数 tab（/sites?tab=node 可直达；刷新与分享后保持）。同一时刻
 * 只挂载当前分栏（display-directive="if"）：切换分栏即卸载，面板内的轮询定时器随之
 * 清理；也避免三个分栏同时探测。
 * UI 文案人话化。
 */
import { ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NTabPane, NTabs } from 'naive-ui'
import SiteListPane from './index.vue'
import AppProjectsPane from '@/views/app-mgmt/AppProjectsPane.vue'

type SitesTab = 'site' | 'python' | 'node'

/** 当前地址栏参数对应的分栏（未知或缺省=「网站」） */
function tabFromQuery(v: unknown): SitesTab {
  return v === 'python' || v === 'node' ? v : 'site'
}

const route = useRoute()
const router = useRouter()
const active = ref<SitesTab>(tabFromQuery(route.query.tab))

// 浏览器前进/后退、页内链接改变 ?tab= 时同步分栏。
watch(
  () => route.query.tab,
  (v) => {
    const t = tabFromQuery(v)
    if (t !== active.value) active.value = t
  }
)

function onTabChange(name: string) {
  const t = tabFromQuery(name)
  if (t === tabFromQuery(route.query.tab)) return
  void router.replace({ query: { ...route.query, tab: t === 'site' ? undefined : t } })
}
</script>

<template>
  <div class="sites-hub">
    <NTabs v-model:value="active" type="line" animated @update:value="onTabChange">
      <NTabPane name="site" tab="网站" display-directive="if">
        <SiteListPane />
      </NTabPane>
      <NTabPane name="python" tab="Python 项目" display-directive="if">
        <AppProjectsPane runtime="python" />
      </NTabPane>
      <NTabPane name="node" tab="Node 项目" display-directive="if">
        <AppProjectsPane runtime="node" />
      </NTabPane>
    </NTabs>
  </div>
</template>

<style scoped>
.sites-hub {
  padding: 0 0 4px;
}
</style>
