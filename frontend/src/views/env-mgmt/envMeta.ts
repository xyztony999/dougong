import type { EnvType } from '@/types/enums'

/** 品牌色：logo 底与卡片色条 */
export const ENV_ACCENT: Record<EnvType, string> = {
  nginx: '#009639',
  apache: '#d22128',
  docker: '#2496ed',
  mysql: '#4479a1',
  redis: '#dc382d',
  php_fpm: '#777bb4',
  mongodb: '#47a248',
  node: '#5fa04e',
  python: '#3776ab'
}

export function isRuntimeEnv(envType: EnvType): boolean {
  return envType === 'node' || envType === 'python'
}

/**
 * 卡片上的短版本：只留开头的数字段，丢掉发行版后缀。
 * 29.1.3-0ubuntu3-24.04.2 → 29.1.3；1.30.6-1~noble → 1.30.6
 */
export function shortEnvVersion(raw: string | null | undefined): string {
  if (raw == null) return ''
  const t = raw.trim()
  if (t === '') return ''
  const m = t.match(/^v?(\d+(?:\.\d+){1,3})/)
  return m ? m[1] : t
}

export function installedLabel(version: string | null | undefined): string {
  const short = shortEnvVersion(version)
  return short === '' ? '已安装' : `已安装 · ${short}`
}
