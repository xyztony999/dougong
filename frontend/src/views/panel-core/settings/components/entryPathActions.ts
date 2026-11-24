/**
 * 安全入口卡片里三个会改变面板地址的动作（panel-core PageSpec 5.1 Card 2）：开启、重新生成、关闭。
 *
 * 这三个动作成功后面板的前缀立刻变了，而当前页面是按旧地址加载的——页面里的 <base> 还是旧值，
 * 之后发出的每一个请求（包括重新读取配置）都带旧前缀，被入口网关拦掉。所以成功后不在当前页面上
 * 继续请求，而是整页跳到新地址下的设置页，由新页面重新加载。
 *
 * 提示和「请保存入口路径」的展示跨不过整页跳转：跳转前记一笔，新页面的卡片挂载时读出来
 * （takeEntryPathChange）再展示一次。
 */
import { configUpdate, entryPathRegenerate } from '@/api/panel'
import type { EntryPathRegeneratedData, PanelConfigDetail, PanelConfigUpdateReq } from '@/types/panel'
import { joinBase } from '@/utils/base'

export type EntryPathChange = 'enabled' | 'regenerated' | 'disabled'

/** 动作完成后的提示（新页面挂载时展示） */
export const ENTRY_PATH_DONE_TEXT: Readonly<Record<EntryPathChange, string>> = {
  enabled: '安全入口已开启，变更已记入审计',
  regenerated: '已重新生成，变更已记入审计',
  disabled: '安全入口已关闭，变更已记入审计'
}

const CHANGE_KEY = 'panel.entryPath.change'
/** 记录的有效期：新页面没有紧接着打开（比如加载失败，过了很久才再进设置页），就不再当作刚发生 */
export const CHANGE_TTL_MS = 60_000

export interface EntryPathActionsDeps {
  configUpdate(req: PanelConfigUpdateReq): Promise<PanelConfigDetail>
  regenerate(): Promise<EntryPathRegeneratedData>
  /** 整页跳转到面板内的地址 */
  navigate(url: string): void
  /** 跨整页跳转记一笔（生产用 sessionStorage） */
  remember(change: EntryPathChange): void
}

/** 设置页在某个入口路径下的地址。入口路径为空（安全入口没开启）时是根地址下的 /settings。 */
export function entrySettingsUrl(entryPath: string | null): string {
  const base = entryPath === null || entryPath === '' ? '/' : `/${encodeURIComponent(entryPath)}/`
  return joinBase(base, '/settings')
}

export function createEntryPathActions(deps: EntryPathActionsDeps) {
  /** 成功：记一笔，然后整页跳走。失败时什么都没变（拦截器已提示），留在当前页。 */
  const done = (change: EntryPathChange, url: string): boolean => {
    deps.remember(change)
    deps.navigate(url)
    return true
  }
  return {
    /** 0→1。返回的就是更新后的配置，入口路径以它为准（没有既有路径时服务端会自动生成，api 4.1.4） */
    async enable(): Promise<boolean> {
      try {
        const view = await deps.configUpdate({ entryPathEnabled: true })
        return done('enabled', entrySettingsUrl(view.entryPath.path))
      } catch {
        return false
      }
    },
    /** 重新生成：旧路径立即失效 */
    async regenerate(): Promise<boolean> {
      try {
        const data = await deps.regenerate()
        return done('regenerated', entrySettingsUrl(data.entryPath))
      } catch {
        return false
      }
    },
    /** 1→0：之后从根地址访问 */
    async disable(): Promise<boolean> {
      try {
        await deps.configUpdate({ entryPathEnabled: false })
        return done('disabled', entrySettingsUrl(null))
      } catch {
        return false
      }
    }
  }
}

/** 记下刚发生的变更。存储不可用时忽略——新页面照常跳转，只是不再补提示和路径。 */
export function rememberEntryPathChange(change: EntryPathChange, now: number = Date.now()): void {
  try {
    window.sessionStorage.setItem(CHANGE_KEY, `${change}@${now}`)
  } catch {
    // 存不下就算了
  }
}

/** 取出刚发生的变更并清除。没有记录、记录不认识、或者太久之前的，都返回 null。 */
export function takeEntryPathChange(now: number = Date.now()): EntryPathChange | null {
  let raw: string | null
  try {
    raw = window.sessionStorage.getItem(CHANGE_KEY)
    window.sessionStorage.removeItem(CHANGE_KEY)
  } catch {
    return null
  }
  if (raw === null) return null
  const at = raw.lastIndexOf('@')
  const change = raw.slice(0, at)
  const atTime = Number(raw.slice(at + 1))
  if (!(change in ENTRY_PATH_DONE_TEXT) || !Number.isFinite(atTime) || now - atTime > CHANGE_TTL_MS) {
    return null
  }
  return change as EntryPathChange
}

/** 浏览器环境下的依赖（生产用；测试注入替身）。 */
export function createBrowserEntryPathDeps(): EntryPathActionsDeps {
  return {
    configUpdate,
    regenerate: entryPathRegenerate,
    navigate: (url) => window.location.assign(url),
    remember: (change) => rememberEntryPathChange(change)
  }
}
