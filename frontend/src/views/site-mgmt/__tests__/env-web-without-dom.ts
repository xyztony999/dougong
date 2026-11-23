import type { Environment } from 'vitest/environments'

/**
 * vitest 自定义测试环境：模块按「web」方式编译，但不提供任何 DOM。
 *
 * 为什么需要：vitest 默认的 node 环境一律按 SSR 方式编译，.vue 文件只会得到
 * ssrRender、没有客户端 render，无法挂载比对。而「真实 Vue 运行时 + 极简宿主」的
 * 挂载测试并不需要 DOM（见 sites-hub-mount.test.ts），也就不必为此引入 jsdom /
 * happy-dom。
 *
 * 接入：vite.config.ts 的 test.environmentMatchGlobs（按测试文件匹配）。
 */
const webWithoutDom: Environment = {
  name: 'web-without-dom',
  transformMode: 'web',
  setup() {
    return { teardown() {} }
  }
}

export default webWithoutDom
