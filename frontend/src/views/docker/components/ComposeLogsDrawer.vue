<script setup lang="ts">
/**
 * Compose 日志 Drawer（PageSpec 5.2；宽 720px，标题「Compose 项目 {project}」；项目名 link 打开）。
 * 单日志 Card=LogFollowCard（行数档位/跟随开关/end 帧处理与 5.1 Card B 完全一致，D-03 同口径）。
 * 页头附灰字工作目录/配置文件（R-13 label 透传原值不翻译）；locatable=false 打开即
 * NAlert warning 呈现 DK_015 语义 + 引导容器页兜底（api 4.3.4 logs 校验含 DK_015，
 * 不触日志端点——PageSpec 5.2），日志控件禁用。
 * DK_014（面板外已删）→ warning + 重拉 Compose 清单（PageSpec 第 1 节）。
 */
import { computed, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { ApiError } from '@/api/http'
import { useDockerStore } from '@/stores/docker'
import { DK_ERROR_TEXT } from '@/types/docker'
import LogFollowCard from './LogFollowCard.vue'

const store = useDockerStore()
const message = useMessage()

const show = computed(() => store.logTarget?.kind === 'compose')
/** 收窄目标（show=true 时恒非空 compose 形态） */
const target = computed(() => (store.logTarget?.kind === 'compose' ? store.logTarget : null))
const locatableFailed = computed(() => target.value !== null && !target.value.locatable)

/** Drawer 打开 → locatable 时按需拉取尾部日志；定位失败降级（不触端点，api 4.3.4） */
watch(show, (visible) => {
  if (!visible || locatableFailed.value) return
  void store.loadLogs().catch((e: unknown) => handleLogsError(e))
})

function handleLogsError(e: unknown): void {
  if (!(e instanceof ApiError)) return
  if (e.is('DK_014')) {
    message.warning(DK_ERROR_TEXT['DK_014'])
    void store.loadCompose()
    return
  }
  const text = DK_ERROR_TEXT[e.code]
  if (text !== undefined) message.error(text)
}

function onUpdateShow(v: boolean): void {
  if (!v) store.closeLogDrawer()
}
</script>

<template>
  <n-drawer :show="show" :width="720" @update:show="onUpdateShow">
    <n-drawer-content
      :title="`Compose 项目 ${target?.project ?? ''}`"
      closable
    >
      <!-- 页头灰字 label 透传（R-13：与 daemon/CLI 互认，原值不翻译） -->
      <div class="meta">
        工作目录：{{ target?.workingDir ?? '—' }} · 配置文件：{{ target?.configFile ?? '—' }}
      </div>
      <!-- locatable=false：DK_015 语义降级提示 + 容器级兜底引导（PageSpec 5.2） -->
      <n-alert v-if="locatableFailed" type="warning" class="locate-alert">
        工作目录或配置文件缺失，无法获取项目日志（可经容器页逐个操作兜底）
      </n-alert>
      <n-card size="small" title="日志" class="drawer-card">
        <log-follow-card :disabled="locatableFailed" />
      </n-card>
    </n-drawer-content>
  </n-drawer>
</template>

<style scoped>
.meta {
  font-size: 12px;
  opacity: 0.6;
  margin-bottom: 12px;
  word-break: break-all;
}
.locate-alert {
  margin-bottom: 16px;
}
.drawer-card {
  margin-bottom: 16px;
}
</style>
