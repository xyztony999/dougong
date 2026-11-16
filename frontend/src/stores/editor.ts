/**
 * editorStore —— PageSpec 第 1 节声明：path/content/dirty/savedMeta。
 * edit 页（/files/edit?path=…，admin 专属——readonly 组件内兜底跳回，负空间）：
 * 进入 entryContent 加载（FILE_006/002/003 拒绝进入）；保存 entryContentSave 全量覆盖（I-09，
 * 响应新值直更 savedMeta）；dirty=编辑内容≠加载内容（P3 前端态，路由离开/beforeunload 拦截）。
 */
import { ref } from 'vue'
import { defineStore } from 'pinia'
import { ApiError } from '@/api/http'
import { entryContent } from '@/api/file'
import type { ContentSaveData } from '@/types/file'

/** 加载失败分类（PageSpec 第 5 节：FILE_006 warning+返回 / FILE_002、FILE_003 错误提示+返回） */
export type EditorLoadError = 'not_editable' | 'out_of_scope' | 'not_found' | 'error' | null

export const useEditorStore = defineStore('editor', () => {
  const path = ref('')
  /** 加载内容（dirty 比对基线；保存成功后更新为已保存内容） */
  const content = ref('')
  const dirty = ref(false)
  const savedMeta = ref<{ size: number; mtime: string } | null>(null)
  const loading = ref(false)
  const loadError = ref<EditorLoadError>(null)

  async function load(target: string): Promise<EditorLoadError> {
    loading.value = true
    loadError.value = null
    content.value = ''
    dirty.value = false
    savedMeta.value = null
    try {
      const data = await entryContent(target)
      path.value = data.path
      content.value = data.content
      savedMeta.value = { size: data.size, mtime: data.mtime }
      return null
    } catch (e) {
      if (e instanceof ApiError) {
        if (e.is('FILE_006')) loadError.value = 'not_editable'
        else if (e.is('FILE_002')) loadError.value = 'out_of_scope'
        else if (e.is('FILE_003')) loadError.value = 'not_found'
        else loadError.value = 'error'
      } else {
        loadError.value = 'error'
      }
      return loadError.value
    } finally {
      loading.value = false
    }
  }

  function setDirty(value: boolean): void {
    dirty.value = value
  }

  /** 保存成功：savedMeta 以响应新值直更 + dirty 基线复位（content=已保存内容，api 响应 {path,size,mtime}） */
  function applySaved(data: ContentSaveData, savedContent: string): void {
    savedMeta.value = { size: data.size, mtime: data.mtime }
    content.value = savedContent
    dirty.value = false
  }

  /** 离开页面/换文件：全量复位 */
  function reset(): void {
    path.value = ''
    content.value = ''
    dirty.value = false
    savedMeta.value = null
    loading.value = false
    loadError.value = null
  }

  /** 保存动作归 api 层（entryContentSave），此处仅暴露直更入口——页面持有 loading 态 */
  return { path, content, dirty, savedMeta, loading, loadError, load, setDirty, applySaved, reset }
})
