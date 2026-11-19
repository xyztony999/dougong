<script setup lang="ts">
/**
 * 高级管理入口（database-mgmt PageSpec v1.5 6.8，db-tools 集成消费，admin 专属）：
 * 实例行/库行「高级管理」按钮 → 工具状态懒加载判定（dbtoolStore.ensureLoaded）→
 * 已装+运行中 → dbtoolOpen → window.open 新标签页（db-tools 4.4 裁定 noopener）；
 * 未装 → 引导安装弹窗（依赖就绪度+环境管理链接+到工具管理安装）；
 * 运行态 false/null → 「工具服务未运行」提示+到工具管理查看+可选重试打开（DBT_012 兜底）；
 * mongo-express boundInstanceId 绑定校验（不一致明示绑定实例+仍要打开/重新部署二选）。
 * 引擎映射：mysql/mariadb→phpmyadmin、mongodb→mongo_express、redis 无入口（OUT，
 * 父级不渲染本组件）；readonly 负空间——父级不渲染 + 本组件角色兜底不渲染。
 * 错误分流：DBT_004→未装引导；DBT_006→配置凭据引导；AUTH_008 拦截器双保险。
 */
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useMessage } from 'naive-ui'
import { dbtoolOpen } from '@/api/dbtool'
import { ApiError } from '@/api/http'
import { useAuthStore } from '@/stores/auth'
import { useDbStore } from '@/stores/database'
import { useDbtoolStore } from '@/stores/dbtool'
import type { DbEngine } from '@/types/enums'
import type { DbToolItem } from '@/types/dbtool'
import {
  DBTOOL_ACCESS_TOOLTIP,
  dbtErrorText,
  depsMissingSummary,
  instanceIdentityText,
  openToastText,
  toolForEngine
} from '@/types/dbtool'

const props = defineProps<{
  instanceId: number
  engine: DbEngine
}>()

const router = useRouter()
const message = useMessage()
const authStore = useAuthStore()
const dbStore = useDbStore()
const dbtoolStore = useDbtoolStore()

const busy = ref(false)

/** 引导弹窗态（未装/未运行/绑定不一致三形态） */
type GuideState =
  | { kind: 'not-installed'; tool: DbToolItem }
  | { kind: 'not-running'; tool: DbToolItem }
  | { kind: 'bound-mismatch'; tool: DbToolItem; boundIdentity: string }
  | null

const guide = ref<GuideState>(null)

const guideTitle = computed(() => {
  const g = guide.value
  if (g === null) return ''
  if (g.kind === 'not-installed') return `${g.tool.displayName} 未安装`
  if (g.kind === 'not-running') return '工具服务未运行'
  return '绑定实例不一致'
})

/** 绑定实例反查（dbStore 档案缓存 → identityKey；缺失兜底 #id 不虚构） */
function boundIdentityOf(boundInstanceId: number): string {
  const inst = dbStore.findById(boundInstanceId)
  if (inst === null) return `实例 #${boundInstanceId}`
  return `${instanceIdentityText(inst)}（#${boundInstanceId}）`
}

async function doOpen(tool: DbToolItem): Promise<void> {
  busy.value = true
  try {
    const res = await dbtoolOpen(
      tool.toolType,
      tool.toolType === 'phpmyadmin' ? props.instanceId : undefined
    )
    window.open(res.url, '_blank', 'noopener')
    message.success(openToastText(res))
    guide.value = null
  } catch (e) {
    handleDbtError(e, tool)
  } finally {
    busy.value = false
  }
}

function handleDbtError(e: unknown, tool: DbToolItem): void {
  if (!(e instanceof ApiError)) return
  if (e.is('DBT_004')) {
    guide.value = { kind: 'not-installed', tool }
    void dbtoolStore.loadList()
    return
  }
  if (e.is('DBT_006')) {
    message.error('实例未就绪——请到数据库管理配置凭据（实例清单「配置凭据」）')
    return
  }
  const text = dbtErrorText(e.code, e.data)
  if (text !== null) message.error(text)
}

/** 入口点击（6.8 交互流程 1~4） */
async function onClick(): Promise<void> {
  const toolType = toolForEngine(props.engine)
  if (toolType === null) return // redis 无入口（OUT）——父级本不渲染，防御兜底
  busy.value = true
  try {
    await dbtoolStore.ensureLoaded()
    const tool = dbtoolStore.findByType(toolType)
    if (tool === null) {
      message.error('工具目录缺失该条目——请到数据库工具页刷新')
      return
    }
    if (!tool.installed) {
      guide.value = { kind: 'not-installed', tool }
      return
    }
    // mongo-express 绑定校验（6.8 步骤 4：boundInstanceId 与当前治理实例比对）
    if (
      tool.toolType === 'mongo_express' &&
      tool.boundInstanceId !== null &&
      tool.boundInstanceId !== props.instanceId
    ) {
      guide.value = {
        kind: 'bound-mismatch',
        tool,
        boundIdentity: boundIdentityOf(tool.boundInstanceId)
      }
      return
    }
    if (tool.running !== true) {
      guide.value = { kind: 'not-running', tool }
      return
    }
    await doOpen(tool)
  } finally {
    busy.value = false
  }
}

function gotoTools(): void {
  guide.value = null
  void router.push('/db-tools')
}
</script>

<template>
  <!-- readonly 角色兜底不渲染（父级已按 isAdmin 负空间；双保险） -->
  <n-tooltip v-if="authStore.isAdmin">
    <template #trigger>
      <!-- .stop：db-list 行点击进治理页，入口按钮不串扰（stopAnd 同语义） -->
      <n-button size="small" :loading="busy" @click.stop="onClick">高级管理</n-button>
    </template>
    {{ DBTOOL_ACCESS_TOOLTIP }}
  </n-tooltip>

  <!-- 引导弹窗（未装/未运行/绑定不一致） -->
  <n-modal
    :show="guide !== null"
    preset="card"
    :title="guideTitle"
    style="width: 540px"
    :mask-closable="false"
    @update:show="(v: boolean) => { if (!v) guide = null }"
  >
    <template v-if="guide?.kind === 'not-installed'">
      <n-alert type="info" :show-icon="true" style="margin-bottom: 10px">
        {{ guide.tool.displayName }} 未安装——安装后可经面板安全访问进行高级数据库管理。
      </n-alert>
      <template v-if="!guide.tool.depsReady">
        <n-alert type="warning" title="依赖缺失" style="margin-bottom: 10px">
          {{ depsMissingSummary(guide.tool.depsMissing) }}
          <router-link to="/env" class="guide-link">到环境管理安装</router-link>
        </n-alert>
      </template>
    </template>
    <template v-else-if="guide?.kind === 'not-running'">
      <n-alert type="warning" :show-icon="true">
        {{ guide.tool.displayName }} 服务未运行（运行态：{{ guide.tool.running === null ? '未知' : '已停止' }}）——请到数据库工具页查看安装/升级任务，或重试打开（服务端校验兜底）。
      </n-alert>
    </template>
    <template v-else-if="guide?.kind === 'bound-mismatch'">
      <n-alert type="warning" :show-icon="true">
        mongo-express 部署时绑定实例 {{ guide.boundIdentity }}，打开将进入该实例（非当前治理实例）。
        如需切换绑定，请到数据库工具页卸载后重装选择新实例。
      </n-alert>
    </template>
    <template #footer>
      <div class="footer">
        <n-button @click="guide = null">取消</n-button>
        <template v-if="guide?.kind === 'not-installed'">
          <n-button type="primary" @click="gotoTools">到工具管理安装</n-button>
        </template>
        <template v-else-if="guide?.kind === 'not-running'">
          <n-button @click="gotoTools">到工具管理查看</n-button>
          <n-button type="primary" :loading="busy" @click="guide && doOpen(guide.tool)">
            重试打开
          </n-button>
        </template>
        <template v-else-if="guide?.kind === 'bound-mismatch'">
          <n-button @click="gotoTools">到工具管理重新部署</n-button>
          <n-button type="primary" :loading="busy" @click="guide && doOpen(guide.tool)">
            仍要打开
          </n-button>
        </template>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.guide-link {
  margin-left: 8px;
  font-weight: 500;
}
.footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
</style>
