import { defineConfig, Plugin REDACTED from 'vite'
import vue from '@vitejs/plugin-vue'
import checker from 'vite-plugin-checker'
import { resolve REDACTED from 'path'

/**
 * Vite 插件：开发模式下注入公开配置到 index.html
 * 与生产模式的后端注入行为保持一致，消除闪烁
 */
function injectPublicSettings(): Plugin {
  const backendUrl = process.env.VITE_DEV_PROXY_TARGET || 'http://localhost:8080'

  return {
    name: 'inject-public-settings',
    transformIndexHtml: {
      order: 'pre',
      async handler(html) {
        try {
          const response = await fetch(`${backendUrlREDACTED/api/v1/settings/public`, {
            signal: AbortSignal.timeout(2000)
          REDACTED)
          if (response.ok) {
            const data = await response.json()
            if (data.code === 0 && data.data) {
              const script = `<script>window.__APP_CONFIG__=${JSON.stringify(data.data)REDACTED;</script>`
              return html.replace('</head>', `${scriptREDACTED\n</head>`)
            REDACTED
          REDACTED
        REDACTED catch (e) {
          console.warn('[vite] 无法获取公开配置，将回退到 API 调用:', (e as Error).message)
        REDACTED
        return html
      REDACTED
    REDACTED
  REDACTED
REDACTED

export default defineConfig({
  plugins: [
    vue(),
    checker({
      typescript: true,
      vueTsc: true
    REDACTED),
    injectPublicSettings()
  ],
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src'),
      // 使用 vue-i18n 运行时版本，避免 CSP unsafe-eval 问题
      'vue-i18n': 'vue-i18n/dist/vue-i18n.runtime.esm-bundler.js'
    REDACTED
  REDACTED,
  define: {
    // 启用 vue-i18n JIT 编译，在 CSP 环境下处理消息插值
    // JIT 编译器生成 AST 对象而非 JS 代码，无需 unsafe-eval
    __INTLIFY_JIT_COMPILATION__: true
  REDACTED,
  build: {
    outDir: '../backend/internal/web/dist',
    emptyOutDir: true
  REDACTED,
  server: {
    host: '0.0.0.0',
    port: Number(process.env.VITE_DEV_PORT || 3000),
    proxy: {
      '/api': {
        target: process.env.VITE_DEV_PROXY_TARGET || 'http://localhost:8080',
        changeOrigin: true
      REDACTED,
      '/setup': {
        target: process.env.VITE_DEV_PROXY_TARGET || 'http://localhost:8080',
        changeOrigin: true
      REDACTED
    REDACTED
  REDACTED
REDACTED)
