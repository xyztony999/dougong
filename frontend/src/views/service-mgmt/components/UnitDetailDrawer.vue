<script setup lang="ts">
/**
 * unit 属性 Drawer（PageSpec 第 6 节）：右侧 480px、单区块 NDescriptions。
 * - 数据为打开时刻快照（不自动刷新），Drawer 头部「刷新」图标按钮重拉 unitDetail；
 * - Drawer 内无操作按钮（操作集中在列表操作列，避免双入口状态同步复杂度）；
 * - readonly 与 admin 全等（属性查看为只读能力，FL 附录A）；
 * - SVC_003 → NEmpty「该服务已不存在」；SVC_008 → NAlert 错误（不显示过期数据，I-01）；
 * - description/loadState null → 「—」；subState/enabledState 原值透传（D-04，非两态灰字 ⚠️A14）。
 */
import { computed } from 'vue'
import { RefreshOutline } from '@vicons/ionicons5'
import { useServiceStore } from '@/stores/service'
import { SERVICE_ACTIVE_STATE, SERVICE_ENABLED_STATE, UNIT_TYPE, dictItem } from '@/types/enums'
import type { DictItem } from '@/types/enums'

const store = useServiceStore()

const show = computed<boolean>({
  get: () => store.drawerOpen,
  set: (v) => {
    if (!v) store.closeDrawer()
  }
})

const detail = computed(() => store.unitDetail)

const typeDict = computed<DictItem | null>(() =>
  detail.value !== null ? dictItem(UNIT_TYPE, detail.value.unitType) : null
)
const activeDict = computed<DictItem | null>(() =>
  detail.value !== null ? dictItem(SERVICE_ACTIVE_STATE, detail.value.activeState) : null
)
/** 两态走 dict 颜色，非两态（static 等）灰字原值——⚠️A14/D-05 只读展示（Drawer 内不提供开关） */
const enabledDict = computed<DictItem | null>(() =>
  detail.value !== null ? dictItem(SERVICE_ENABLED_STATE, detail.value.enabledState) : null
)
const descriptionText = computed(() => detail.value?.description ?? '—')
const loadStateText = computed(() => detail.value?.loadState ?? '—')
</script>

<template>
  <n-drawer v-model:show="show" :width="480" placement="right">
    <!-- NDrawerContent 无 header-extra 插槽（naive-ui 仅 NCard 系提供；插槽面
         = header/default/footer，DrawerContent.mjs:65-81）——标题行自绘：
         标题 + 刷新按钮，closable 关闭图标由组件独立渲染于行尾。 -->
    <n-drawer-content closable>
      <template #header>
        <div class="drawer-header">
          <span class="drawer-header__title">{{ store.drawerName ?? '服务属性' }}</span>
          <n-button
            quaternary
            circle
            size="small"
            :loading="store.detailLoading"
            title="刷新"
            @click="store.refreshDetail()"
          >
            <template #icon>
              <n-icon><RefreshOutline /></n-icon>
            </template>
          </n-button>
        </div>
      </template>

      <!-- SVC_008：通道不可用（不显示过期数据，I-01） -->
      <n-alert
        v-if="store.detailError === 'unavailable'"
        type="error"
        title="systemd 服务通道不可用，无法获取服务属性"
      />
      <!-- SVC_003：unit 已不存在 -->
      <n-empty v-else-if="store.detailError === 'notfound'" description="该服务已不存在" class="state-block" />
      <!-- 首次加载中 -->
      <n-spin v-else-if="store.detailLoading && detail === null" class="state-block" />
      <n-descriptions
        v-else-if="detail !== null"
        :column="1"
        label-placement="left"
        bordered
        size="small"
      >
        <n-descriptions-item label="名称">{{ detail.name }}</n-descriptions-item>
        <n-descriptions-item label="类型">
          <n-tag v-if="typeDict !== null" :type="typeDict.tag" size="small" :bordered="false">
            {{ typeDict.label }}
          </n-tag>
        </n-descriptions-item>
        <n-descriptions-item label="运行状态">
          <n-tag v-if="activeDict !== null" :type="activeDict.tag" size="small" :bordered="false">
            {{ activeDict.label }}
          </n-tag>
        </n-descriptions-item>
        <n-descriptions-item label="状态细分">{{ detail.subState }}</n-descriptions-item>
        <n-descriptions-item label="开机自启">
          <n-tag v-if="enabledDict !== null" :type="enabledDict.tag" size="small" :bordered="false">
            {{ enabledDict.label }}
          </n-tag>
        </n-descriptions-item>
        <n-descriptions-item label="描述">{{ descriptionText }}</n-descriptions-item>
        <n-descriptions-item label="加载状态">{{ loadStateText }}</n-descriptions-item>
      </n-descriptions>
    </n-drawer-content>
  </n-drawer>
</template>

<style scoped>
.state-block {
  padding: 48px 0;
}
.drawer-header {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  min-width: 0;
}
.drawer-header__title {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
