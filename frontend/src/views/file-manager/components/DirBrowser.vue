<script setup lang="ts">
/**
 * mini 目录浏览器（PageSpec 4.4.3/4.4.6 内嵌目录选择器；态在 fileStore「目录选择器态」）。
 * 面包屑（root=受管根）+ 子目录列表（entryList 过滤 entryType=directory，点击进入）；
 * 供移动（MoveModal）与解压目标目录（UnarchiveModal）共用；readonly 无入口不触发。
 */
import { computed } from 'vue'
import { FolderOutline } from '@vicons/ionicons5'
import { useFileStore } from '@/stores/file'

const store = useFileStore()

interface Crumb {
  label: string
  path: string
}

/** 面包屑：pickerPath 逐段拆分（受管根为根段，PageSpec 4.4.3） */
const crumbs = computed<Crumb[]>(() => {
  const segs = store.pickerPath.split('/').filter(Boolean)
  const list: Crumb[] = [{ label: '受管根', path: '' }]
  segs.forEach((seg, i) => {
    list.push({ label: seg, path: `/${segs.slice(0, i + 1).join('/')}` })
  })
  return list
})

function enter(c: Crumb): void {
  store.enterPickerDir(c.path)
}
</script>

<template>
  <div class="browser">
    <div class="crumbs">
      <template v-for="(c, i) in crumbs" :key="c.path">
        <n-button text size="tiny" type="primary" @click="enter(c)">{{ c.label }}</n-button>
        <span v-if="i < crumbs.length - 1" class="sep">/</span>
      </template>
    </div>
    <n-spin :show="store.pickerLoading" size="small">
      <div class="dirs">
        <n-alert v-if="store.pickerScopeError !== null" type="error" :bordered="false">
          {{ store.pickerScopeError }}
        </n-alert>
        <n-empty v-else-if="store.pickerDirs.length === 0" description="此目录无子目录" size="small" class="empty" />
        <template v-else>
          <div v-for="d in store.pickerDirs" :key="d.path" class="dir-row" @click="store.enterPickerDir(d.path)">
            <n-icon :component="FolderOutline" class="icon" />
            <span class="name">{{ d.name }}</span>
          </div>
        </template>
      </div>
    </n-spin>
  </div>
</template>

<style scoped>
.browser {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.crumbs {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 2px;
}
.sep {
  opacity: 0.5;
  font-size: 12px;
}
.dirs {
  min-height: 160px;
  max-height: 260px;
  overflow: auto;
  border: 1px solid rgba(128, 128, 128, 0.2);
  border-radius: 4px;
  padding: 4px;
}
.dir-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 5px 8px;
  border-radius: 4px;
  cursor: pointer;
  font-size: 13px;
}
.dir-row:hover {
  background: rgba(128, 128, 128, 0.1);
}
.icon {
  color: cornflowerblue;
  flex-shrink: 0;
}
.name {
  word-break: break-all;
}
.empty {
  padding: 24px 0;
}
</style>
