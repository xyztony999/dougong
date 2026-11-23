/// <reference types="vitest" />
import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'
import { NaiveUiResolver } from 'unplugin-vue-components/resolvers'

// 后端地址：api-panel-core.md 3.4——listen_port 为安装时生成的随机高位端口（无固定开发端口），
// 开发联调默认 127.0.0.1:8080，可用环境变量 DG_BACKEND_ADDR 覆盖。禁止前端代码硬编码后端地址。
const backendAddr = process.env.DG_BACKEND_ADDR ?? 'http://127.0.0.1:8080'

export default defineConfig({
  plugins: [
    vue(),
    AutoImport({
      imports: ['vue'],
      dts: 'src/auto-imports.d.ts'
    }),
    Components({
      resolvers: [NaiveUiResolver()],
      dts: 'src/components.d.ts'
    })
  ],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url))
    }
  },
  server: {
    port: 5173,
    proxy: {
      '/auth': {
        target: backendAddr,
        changeOrigin: true
      },
      '/panel': {
        target: backendAddr,
        changeOrigin: true
      },
      '/monitoring': {
        target: backendAddr,
        changeOrigin: true
      },
      '/services': {
        target: backendAddr,
        changeOrigin: true
      },
      '/files': {
        target: backendAddr,
        changeOrigin: true
      },
      '/sites': {
        target: backendAddr,
        changeOrigin: true
      },
      // env-mgmt（api-env-mgmt.md 3.1：/env/{resource}/{action} 三段式，读 GET + 写 POST 同前缀）
      '/env': {
        target: backendAddr,
        changeOrigin: true
      },
      '/db': {
        target: backendAddr,
        changeOrigin: true
      },
      // db-tools（api-db-tools.md 3.1：/dbtool/{resource}/{action} 三段式，读 GET + 写 POST 同前缀；
      // SPA 路由 /db-tools 带连字符不命中本前缀，无冲突）
      '/dbtool': {
        target: backendAddr,
        changeOrigin: true
      },
      // firewall（api-firewall.md 3.1：/firewall/{resource}/{action} 三段式，读 GET + 写 POST 同前缀）
      '/firewall': {
        target: backendAddr,
        changeOrigin: true
      },
      // terminal（api-terminal.md D-01/D-09：WS 升级端点同源代理需 ws:true；HTTP GET /terminal/user/options 同前缀）
      '/terminal': {
        target: backendAddr,
        changeOrigin: true,
        ws: true
      },
      // docker-mgmt（api-docker-mgmt.md 3.1：/docker/{resource}/{action} 三段式，HTTP 与
      // WS 日志跟随 /docker/{container,compose}/logs-follow 同前缀——WS 升级需 ws:true，D-03）
      '/docker': {
        target: backendAddr,
        changeOrigin: true,
        ws: true
      },
      // cron-backup（api-cron-backup.md 3.1：/cron/{resource}/{action} 三段式，读 GET + 写 POST 同前缀）
      '/cron': {
        target: backendAddr,
        changeOrigin: true
      }
    }
  },
  build: {
    outDir: 'dist'
  },
  // vitest（npm test）：只列与默认不同的项
  test: {
    // 挂载测试要用客户端方式编译 .vue——默认的 node 环境按 SSR 编译，组件只有 ssrRender、
    // 挂不起来。只给它换一个「按 web 方式编译、不提供 DOM」的环境，其余测试不受影响。
    environmentMatchGlobs: [
      ['**/sites-hub-mount.test.ts', './src/views/site-mgmt/__tests__/env-web-without-dom.ts']
    ]
  }
})
