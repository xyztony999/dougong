/**
 * 组件树外消息通知（路由守卫 / http 拦截器无 useMessage 上下文时使用；
 * 仍为 Naive UI message，禁原生 alert——frontend-rules §4.4）
 */
import { createDiscreteApi } from 'naive-ui'

const { message } = createDiscreteApi(['message'])

export function notifyError(content: string): void {
  message.error(content)
}

export function notifyInfo(content: string): void {
  message.info(content)
}
