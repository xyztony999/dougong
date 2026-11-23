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
    <!-- 不加分栏切换动画：与面板其它页签页面（如容器管理）保持一致，切换即时；
         动画会依赖过渡结束事件，在未被绘制的页面（后台标签页）里会停在第一帧。 -->
    <!-- 分栏必须带 key：display-directive="if" 下 NTabs 每次只渲染当前分栏这一个子节点，
         不带 key 时切换分栏会被当成同一个组件原地改属性——面板的 setup 不会重跑，后切的
         分栏会沿用先挂载的那个运行时的仓库与表单初始值，轮询也不会随卸载清理。 -->
    <NTabs v-model:value="active" type="line" @update:value="onTabChange">
      <NTabPane key="site" name="site" tab="网站" display-directive="if">
        <SiteListPane />
      </NTabPane>
      <NTabPane key="python" name="python" tab="Python 项目" display-directive="if">
        <AppProjectsPane runtime="python" />
      </NTabPane>
      <NTabPane key="node" name="node" tab="Node 项目" display-directive="if">
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
