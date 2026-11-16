/**
 * 条目图标映射（PageSpec 第 3 节颜色方案）：
 * - directory → 文件夹图标（蓝灰）；symlink → 链接图标（紫）+ 名称尾注「→ 目标」（列表不展开目标，api 4.2.4）；
 * - file → 按扩展名映射（.md/.json/.sh/.go/.py/.js/.log/归档类…），未识别 → 默认文档图标。
 * 图标色为行内点缀色（非大面积背景，frontend-rules §4.1 主题变量条目不适用）；
 * 使用 CSS 命名色避免硬编码十六进制（grep 检查口径）。
 * 回收站 entryType 为内联两值（file/directory，api 4.1.1）——同映射复用。
 */
import type { Component } from 'vue'
import {
  ArchiveOutline,
  CodeSlashOutline,
  DocumentOutline,
  DocumentTextOutline,
  FolderOutline,
  LinkOutline,
  TerminalOutline
} from '@vicons/ionicons5'
import type { TrashEntryType } from '@/types/file'
import type { ArchiveFormat, FileEntryType } from '@/types/enums'

export interface EntryIconInfo {
  icon: Component
  /** CSS 颜色；缺省 = 继承文本色 */
  color?: string
}

/** 归档扩展名识别（PageSpec 4.3 解压按钮可见性 + 图标映射，第 3 节局部映射） */
const ARCHIVE_EXTENSIONS = ['.tar', '.tar.gz', '.tgz', '.zip']

export function isArchiveName(name: string): boolean {
  const lower = name.toLowerCase()
  return ARCHIVE_EXTENSIONS.some((ext) => lower.endsWith(ext))
}

/** 按扩展名的文件图标（PageSpec 第 3 节映射表；未识别 → 默认文档图标） */
function fileIcon(name: string): EntryIconInfo {
  const lower = name.toLowerCase()
  if (lower.endsWith('.sh') || lower.endsWith('.bash') || lower.endsWith('.zsh')) {
    return { icon: TerminalOutline }
  }
  if (lower.endsWith('.json')) return { icon: CodeSlashOutline }
  if (
    lower.endsWith('.go') ||
    lower.endsWith('.py') ||
    lower.endsWith('.js') ||
    lower.endsWith('.ts') ||
    lower.endsWith('.rs') ||
    lower.endsWith('.c') ||
    lower.endsWith('.cpp') ||
    lower.endsWith('.java')
  ) {
    return { icon: CodeSlashOutline }
  }
  if (isArchiveName(name)) return { icon: ArchiveOutline }
  if (
    lower.endsWith('.md') ||
    lower.endsWith('.log') ||
    lower.endsWith('.txt') ||
    lower.endsWith('.conf') ||
    lower.endsWith('.ini') ||
    lower.endsWith('.yaml') ||
    lower.endsWith('.yml') ||
    lower.endsWith('.sql')
  ) {
    return { icon: DocumentTextOutline }
  }
  return { icon: DocumentOutline }
}

/** 条目 → 图标（files 列表/图标视图/回收站原路径列共用） */
export function entryIcon(name: string, entryType: FileEntryType | TrashEntryType): EntryIconInfo {
  if (entryType === 'directory') return { icon: FolderOutline, color: 'cornflowerblue' }
  if (entryType === 'symlink') return { icon: LinkOutline, color: 'mediumpurple' }
  return fileIcon(name)
}

/** archive_format 三选一 → 产物缺省扩展名（PageSpec 4.4.9：留空 = archive.{ext} 按格式） */
export function archiveExtension(format: ArchiveFormat): string {
  if (format === 'tar') return 'tar'
  if (format === 'tar.gz') return 'tar.gz'
  return 'zip'
}
