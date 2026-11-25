import type { EnvType } from '@/types/enums'

/** 环境卡片左侧色条与字母标（工业控制台风格，不用通用插画） */
export const ENV_ACCENT: Record<EnvType, string> = {
  nginx: '#3dd68c',
  apache: '#f59e4c',
  docker: '#5ec8f0',
  mysql: '#7c8cff',
  redis: '#ef5b5b',
  php_fpm: '#b57cff',
  mongodb: '#4ade80',
  node: '#a3e635',
  python: '#eab308'
}

export const ENV_MONOGRAM: Record<EnvType, string> = {
  nginx: 'Ng',
  apache: 'Ap',
  docker: 'Dk',
  mysql: 'My',
  redis: 'Rd',
  php_fpm: 'Ph',
  mongodb: 'Mg',
  node: 'Nd',
  python: 'Py'
}

export function isRuntimeEnv(envType: EnvType): boolean {
  return envType === 'node' || envType === 'python'
}
