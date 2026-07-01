import { defineConfig, loadEnv, Plugin REDACTED from 'vite'
import vue from '@vitejs/plugin-vue'
import checker from 'vite-plugin-checker'
import { resolve REDACTED from 'path'
import { Buffer REDACTED from 'node:buffer'
import { lookup REDACTED from 'node:dns/promises'
import type { IncomingMessage, ServerResponse REDACTED from 'node:http'
import { isIP REDACTED from 'node:net'

/**
 * Vite 插件：开发模式下注入公开配置到 index.html
 * 与生产模式的后端注入行为保持一致，消除闪烁
 */
function injectPublicSettings(backendUrl: string): Plugin {
  return {
    name: 'inject-public-settings',
    apply: 'serve',
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

const LLM_TESTER_MAX_BODY_BYTES = 12 * 1024 * 1024
const LLM_TESTER_TIMEOUT_MS = 300000

function llmTesterDevProxy(): Plugin {
  return {
    name: 'llm-tester-dev-proxy',
    apply: 'serve',
    configureServer(server) {
      server.middlewares.use(async (req, res, next) => {
        const pathname = new URL(req.url || '/', 'http://localhost').pathname
        if (req.method !== 'POST' || !pathname.startsWith('/api/v1/llm-tester/')) {
          next()
          return
        REDACTED

        try {
          const body = await readDevProxyJson(req)
          const route = pathname.slice('/api/v1/llm-tester/'.length)
          if (route === 'models') {
            await forwardDevLLMTesterRequest(res, body, 'GET', 'models')
            return
          REDACTED
          if (route === 'chat/completions') {
            await forwardDevLLMTesterRequest(res, body, 'POST', 'chat/completions')
            return
          REDACTED
          if (route === 'images/generations') {
            await forwardDevLLMTesterRequest(res, body, 'POST', 'images/generations')
            return
          REDACTED
          if (route === 'responses') {
            await forwardDevLLMTesterRequest(res, body, 'POST', 'responses')
            return
          REDACTED
          next()
        REDACTED catch (error) {
          writeDevProxyError(res, 502, devProxyErrorMessage(error))
        REDACTED
      REDACTED)
    REDACTED
  REDACTED
REDACTED

async function readDevProxyJson(req: IncomingMessage): Promise<Record<string, any>> {
  const chunks: Buffer[] = []
  let total = 0
  for await (const chunk of req) {
    const buffer = Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk)
    total += buffer.length
    if (total > LLM_TESTER_MAX_BODY_BYTES) {
      throw new Error('request body is too large')
    REDACTED
    chunks.push(buffer)
  REDACTED

  try {
    const parsed = JSON.parse(Buffer.concat(chunks).toString('utf8'))
    return parsed && typeof parsed === 'object' ? parsed : {REDACTED
  REDACTED catch {
    throw new Error('invalid request body')
  REDACTED
REDACTED

async function forwardDevLLMTesterRequest(
  res: ServerResponse,
  body: Record<string, any>,
  method: 'GET' | 'POST',
  resource: 'models' | 'chat/completions' | 'images/generations' | 'responses'
) {
  const baseUrl = String(body.base_url || '').trim()
  const apiKey = String(body.api_key || '').trim()
  if (!baseUrl) {
    writeDevProxyError(res, 400, 'base_url is required')
    return
  REDACTED
  if (!apiKey) {
    writeDevProxyError(res, 400, 'api_key is required')
    return
  REDACTED
  if (apiKey.length > 8192) {
    writeDevProxyError(res, 400, 'api_key is too long')
    return
  REDACTED
  if (method === 'POST' && !body.payload) {
    writeDevProxyError(res, 400, 'payload is required')
    return
  REDACTED

  const endpoint = await buildDevLLMTesterEndpoint(baseUrl, resource)
  const upstream = await fetch(endpoint, {
    method,
    headers: {
      Authorization: `Bearer ${apiKeyREDACTED`,
      Accept: 'application/json',
      'Content-Type': 'application/json',
      'User-Agent': 'Sub2API-LLM-Tester/1.0',
      'X-Title': 'Sub2API LLM Tester'
    REDACTED,
    body: method === 'POST' ? JSON.stringify(body.payload || {REDACTED) : undefined,
    signal: AbortSignal.timeout(LLM_TESTER_TIMEOUT_MS)
  REDACTED)
  const payload = Buffer.from(await upstream.arrayBuffer())
  if (payload.length > LLM_TESTER_MAX_BODY_BYTES) {
    writeDevProxyError(res, 502, 'upstream response is too large')
    return
  REDACTED

  res.statusCode = upstream.status
  res.setHeader('Content-Type', upstream.headers.get('content-type') || 'application/json')
  res.end(payload)
REDACTED

async function buildDevLLMTesterEndpoint(baseUrl: string, resource: 'models' | 'chat/completions' | 'images/generations' | 'responses'): Promise<string> {
  const url = new URL(baseUrl.replace(/\/+$/, ''))
  if (url.protocol !== 'https:') {
    throw new Error('base_url must use https')
  REDACTED
  if (url.username || url.password) {
    throw new Error('base_url must not include user info')
  REDACTED
  await assertDevProxyPublicHost(url.hostname)
  url.search = ''
  url.hash = ''
  if (!/\/v\d+$/i.test(url.pathname)) {
    url.pathname = `${url.pathname.replace(/\/+$/, '')REDACTED/v1`
  REDACTED
  url.pathname = `${url.pathname.replace(/\/+$/, '')REDACTED/${resourceREDACTED`
  return url.toString()
REDACTED

async function assertDevProxyPublicHost(hostname: string) {
  const host = hostname.trim().toLowerCase()
  if (isBlockedDevProxyHost(host)) {
    throw new Error(`host is not allowed: ${hostnameREDACTED`)
  REDACTED
  if (isIP(host)) {
    if (isBlockedDevProxyIP(host)) throw new Error(`host is not allowed: ${hostnameREDACTED`)
    return
  REDACTED
  const addrs = await lookup(host, { all: true, verbatim: false REDACTED)
  if (!addrs.length) {
    throw new Error(`host did not resolve: ${hostnameREDACTED`)
  REDACTED
  for (const addr of addrs) {
    if (isBlockedDevProxyIP(addr.address)) {
      throw new Error(`resolved ip is not allowed: ${addr.addressREDACTED`)
    REDACTED
  REDACTED
REDACTED

function isBlockedDevProxyHost(host: string): boolean {
  return (
    !host ||
    host === 'localhost' ||
    host.endsWith('.localhost') ||
    host === 'metadata' ||
    host === 'metadata.google.internal' ||
    host === 'metadata.goog' ||
    host === 'instance-data' ||
    host === 'instance-data.ec2.internal'
  )
REDACTED

function isBlockedDevProxyIP(address: string): boolean {
  if (address.includes(':')) {
    const lower = address.toLowerCase()
    return lower === '::' || lower === '::1' || lower.startsWith('fc') || lower.startsWith('fd') || lower.startsWith('fe80')
  REDACTED
  const parts = address.split('.').map((part) => Number(part))
  if (parts.length !== 4 || parts.some((part) => Number.isNaN(part))) return true
  const [a, b] = parts
  return (
    a === 0 ||
    a === 10 ||
    a === 127 ||
    (a === 100 && b >= 64 && b <= 127) ||
    (a === 169 && b === 254) ||
    (a === 172 && b >= 16 && b <= 31) ||
    (a === 192 && b === 168)
  )
REDACTED

function writeDevProxyError(res: ServerResponse, status: number, message: string) {
  res.statusCode = status
  res.setHeader('Content-Type', 'application/json')
  res.end(JSON.stringify({ code: status, message REDACTED))
REDACTED

function devProxyErrorMessage(error: unknown): string {
  const message = error instanceof Error ? error.message : 'LLM tester proxy failed'
  const cause = error instanceof Error ? (error as Error & { cause?: unknown REDACTED).cause : undefined
  if (cause instanceof Error && cause.message && cause.message !== message) {
    return `${messageREDACTED: ${cause.messageREDACTED`
  REDACTED
  return message || 'LLM tester proxy failed'
REDACTED

export default defineConfig(({ mode REDACTED) => {
  // 加载环境变量
  const env = loadEnv(mode, process.cwd(), '')
  const backendUrl = env.VITE_DEV_PROXY_TARGET || 'http://localhost:8080'
  const devPort = Number(env.VITE_DEV_PORT || 3000)

  return {
    plugins: [
      vue(),
      checker({
        vueTsc: true
      REDACTED),
      injectPublicSettings(backendUrl),
      llmTesterDevProxy()
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
    emptyOutDir: true,
    rollupOptions: {
      output: {
        /**
         * 手动分包配置
         * 分离第三方库并按功能合并应用代码，避免循环依赖
         */
        manualChunks(id: string) {
          if (id.includes('node_modules')) {
            // Vue 核心库
            if (
              id.includes('/vue/') ||
              id.includes('/vue-router/') ||
              id.includes('/pinia/') ||
              id.includes('/@vue/')
            ) {
              return 'vendor-vue'
            REDACTED

            // UI 工具库（较大，单独分离）
            if (id.includes('/@vueuse/') || id.includes('/xlsx/')) {
              return 'vendor-ui'
            REDACTED

            // 图表库
            if (id.includes('/chart.js/') || id.includes('/vue-chartjs/')) {
              return 'vendor-chart'
            REDACTED

            // 国际化
            if (id.includes('/vue-i18n/') || id.includes('/@intlify/')) {
              return 'vendor-i18n'
            REDACTED

            // 其他小型第三方库合并
            return 'vendor-misc'
          REDACTED

          // 应用代码：按入口点自动分包，不手动干预
          // 这样可以避免循环依赖，同时保持合理的 chunk 数量
        REDACTED
      REDACTED
    REDACTED
  REDACTED,
    server: {
      host: '0.0.0.0',
      port: devPort,
      proxy: {
        '/api': {
          target: backendUrl,
          changeOrigin: true
        REDACTED,
        '/v1': {
          target: backendUrl,
          changeOrigin: true
        REDACTED,
        '/setup': {
          target: backendUrl,
          changeOrigin: true
        REDACTED
      REDACTED
    REDACTED
  REDACTED
REDACTED)
