<template>
  <AppLayout>
    <div class="mx-auto flex max-w-[1500px] flex-col gap-4">
      <div class="flex min-w-0 flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <div class="flex min-w-0 flex-wrap items-center gap-2">
          <button
            v-for="tab in tabs"
            :key="tab.id"
            type="button"
            class="tools-tab"
            :class="{ 'tools-tab-active': activeTab === tab.id }"
            @click="activeTab = tab.id"
          >
            <Icon :name="tab.icon" size="sm" />
            <span>{{ tab.label }}</span>
          </button>
        </div>
        <div class="text-xs text-gray-500 dark:text-dark-400">
          写入使用 sub2api 现有 Admin API；不做自动代理分配。
        </div>
      </div>

      <section v-if="activeTab === 'proxy'" class="tools-surface">
        <div class="tools-section-header">
          <div>
            <h2 class="tools-title">代理导入</h2>
            <p class="tools-description">粘贴 Clash YAML 或输入订阅链接，预览后再导入到现有 IP 管理。</p>
          </div>
          <div v-if="proxyPreview" class="tools-badges">
            <span class="tools-badge">总数 {{ proxyPreview.summary.total }}</span>
            <span class="tools-badge text-emerald-700 dark:text-emerald-300">有效 {{ proxyPreview.summary.valid }}</span>
            <span class="tools-badge text-amber-700 dark:text-amber-300">重复 {{ proxyPreview.summary.duplicates }}</span>
            <span class="tools-badge text-red-700 dark:text-red-300">错误 {{ proxyPreview.summary.invalid }}</span>
          </div>
        </div>

        <div class="grid grid-cols-1 gap-4 2xl:grid-cols-[minmax(0,0.92fr)_minmax(0,1.08fr)]">
          <div class="min-w-0 space-y-4">
            <div>
              <label class="input-label">Clash 订阅链接</label>
              <div class="flex min-w-0 flex-col gap-2 sm:flex-row">
                <input
                  v-model="proxySubscriptionUrl"
                  class="input"
                  type="url"
                  placeholder="https://example.com/subs/clash/..."
                />
                <button
                  type="button"
                  class="btn btn-secondary shrink-0"
                  :disabled="proxyPreviewLoading || !proxySubscriptionUrl.trim()"
                  @click="previewProxyFromUrl"
                >
                  <Icon name="cloud" size="sm" />
                  抓取预览
                </button>
              </div>
              <p class="input-hint">订阅由后端代抓，避免浏览器 CORS；内容大小限制 2 MiB。</p>
            </div>

            <div>
              <label class="input-label">Clash YAML</label>
              <textarea
                v-model="proxyYaml"
                class="input min-h-[230px] resize-y font-mono text-xs leading-relaxed"
                spellcheck="false"
                placeholder="proxies:&#10;  - name: proxy-a&#10;    type: http&#10;    server: 127.0.0.1&#10;    port: 8080"
              ></textarea>
              <div class="mt-2 flex flex-wrap items-center gap-2">
                <button
                  type="button"
                  class="btn btn-primary"
                  :disabled="proxyPreviewLoading || !proxyYaml.trim()"
                  @click="previewProxyFromYaml"
                >
                  <Icon name="search" size="sm" />
                  解析 YAML
                </button>
                <button
                  type="button"
                  class="btn btn-secondary"
                  :disabled="!proxyYaml && !proxySubscriptionUrl && !proxyPreview"
                  @click="resetProxyImport"
                >
                  <Icon name="x" size="sm" />
                  清空
                </button>
              </div>
            </div>
          </div>

          <div class="min-w-0">
            <div class="mb-3 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
              <div>
                <h3 class="text-sm font-semibold text-gray-900 dark:text-white">预览结果</h3>
                <p class="text-xs text-gray-500 dark:text-dark-400">
                  支持 http、https、socks5、socks5h；enabled:false 会保留为 inactive。
                </p>
              </div>
              <div class="flex flex-wrap gap-2">
                <button
                  type="button"
                  class="btn btn-primary btn-sm"
                  :disabled="!canImportProxies || proxyImporting"
                  @click="importProxyData"
                >
                  <Icon name="upload" size="sm" />
                  保留名称/状态导入
                </button>
                <button
                  type="button"
                  class="btn btn-secondary btn-sm"
                  :disabled="!canImportProxies || proxyImporting"
                  @click="importProxyBatch"
                >
                  批量接口导入
                </button>
              </div>
            </div>

            <div v-if="proxyImportResult" class="mb-3 rounded-lg border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700 dark:border-emerald-900/60 dark:bg-emerald-950/30 dark:text-emerald-300">
              创建 {{ proxyImportResult.proxy_created }}，复用 {{ proxyImportResult.proxy_reused }}，失败 {{ proxyImportResult.proxy_failed }}
            </div>
            <div v-if="proxyBatchResult" class="mb-3 rounded-lg border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700 dark:border-emerald-900/60 dark:bg-emerald-950/30 dark:text-emerald-300">
              创建 {{ proxyBatchResult.created }}，跳过 {{ proxyBatchResult.skipped }}
            </div>

            <div class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-700">
              <div v-if="proxyPreviewLoading" class="tools-empty">
                正在解析订阅...
              </div>
              <div v-else-if="!proxyPreview" class="tools-empty">
                先抓取订阅或解析 YAML，这里会显示可导入代理。
              </div>
              <div v-else class="overflow-x-auto">
                <table class="min-w-[920px] divide-y divide-gray-200 text-sm dark:divide-dark-700">
                  <thead class="bg-gray-50 text-xs uppercase text-gray-500 dark:bg-dark-900/60 dark:text-dark-400">
                    <tr>
                      <th class="tools-th">#</th>
                      <th class="tools-th">名称</th>
                      <th class="tools-th">协议</th>
                      <th class="tools-th">地址</th>
                      <th class="tools-th">账号</th>
                      <th class="tools-th">状态</th>
                      <th class="tools-th">结果</th>
                    </tr>
                  </thead>
                  <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-800 dark:bg-dark-900/30">
                    <tr v-for="row in displayedProxyRows" :key="row.index">
                      <td class="tools-td text-gray-500">{{ row.index }}</td>
                      <td class="tools-td max-w-[220px] truncate font-medium text-gray-900 dark:text-white">
                        {{ row.proxy.name || row.name || '-' }}
                      </td>
                      <td class="tools-td">
                        <span class="tools-code">{{ row.proxy.protocol || '-' }}</span>
                      </td>
                      <td class="tools-td">
                        <span class="font-mono text-xs">{{ row.proxy.host || '-' }}:{{ row.proxy.port || '-' }}</span>
                      </td>
                      <td class="tools-td">{{ row.proxy.username ? '已设置' : '-' }}</td>
                      <td class="tools-td">
                        <span :class="row.proxy.status === 'active' ? 'status-ok' : 'status-muted'">
                          {{ row.proxy.status || '-' }}
                        </span>
                      </td>
                      <td class="tools-td">
                        <span v-if="row.valid" class="status-ok">可导入</span>
                        <span v-else class="status-error">{{ row.errors.join('；') }}</span>
                      </td>
                    </tr>
                  </tbody>
                </table>
                <div v-if="proxyPreview.rows.length > displayedProxyRows.length" class="border-t border-gray-200 px-3 py-2 text-xs text-gray-500 dark:border-dark-700 dark:text-dark-400">
                  仅显示前 {{ displayedProxyRows.length }} 条，导入会使用全部有效代理。
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      <section v-else-if="activeTab === 'rt'" class="tools-surface">
        <div class="tools-section-header">
          <div>
            <h2 class="tools-title">OpenAI RT 导入</h2>
            <p class="tools-description">从混合文本中提取 email 和 rt_ / rt- token，逐条校验后创建 OpenAI OAuth 账号。</p>
          </div>
          <div class="tools-badges">
            <span class="tools-badge">并发 {{ normalizedRtConcurrency }}</span>
            <span class="tools-badge">{{ selectedRtProxyId ? `指定代理 #${selectedRtProxyId}` : '不指定代理' }}</span>
          </div>
        </div>

        <div class="grid grid-cols-1 gap-4 2xl:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]">
          <div class="min-w-0 space-y-4">
            <div>
              <label class="input-label">RT 文本</label>
              <textarea
                v-model="rtInput"
                class="input min-h-[260px] resize-y font-mono text-xs leading-relaxed"
                spellcheck="false"
                placeholder="user@example.com rt_...&#10;another@example.com rt-..."
              ></textarea>
              <div class="mt-2 flex flex-wrap items-center gap-2">
                <button type="button" class="btn btn-secondary" :disabled="!rtInput.trim()" @click="parseRtInput">
                  <Icon name="search" size="sm" />
                  预览提取
                </button>
                <button type="button" class="btn btn-primary" :disabled="importingRt || !rtInput.trim()" @click="importRtRows">
                  <Icon name="upload" size="sm" />
                  开始导入
                </button>
              </div>
            </div>

            <div class="grid grid-cols-1 gap-3 xl:grid-cols-3">
              <div>
                <label class="input-label">统一指定 proxy_id</label>
                <select v-model="rtProxyIdRaw" class="input">
                  <option value="">不指定</option>
                  <option v-for="proxy in activeProxies" :key="proxy.id" :value="String(proxy.id)">
                    {{ proxyLabel(proxy) }}
                  </option>
                </select>
              </div>
              <div>
                <label class="input-label">client_id</label>
                <input v-model="rtClientId" class="input font-mono text-xs" placeholder="留空使用后端默认" />
              </div>
              <div>
                <label class="input-label">并发</label>
                <input v-model.number="rtConcurrency" class="input" type="number" min="1" max="5" />
              </div>
            </div>
            <div class="flex flex-wrap gap-2">
              <button type="button" class="btn btn-secondary btn-sm" @click="rtClientId = openAIMobileClientId">
                填入 Mobile client_id
              </button>
              <button type="button" class="btn btn-secondary btn-sm" @click="rtClientId = ''">
                使用默认 client_id
              </button>
            </div>
          </div>

          <div class="min-w-0">
            <div class="mb-3 flex flex-wrap items-center gap-2">
              <span class="tools-badge">提取 {{ rtRows.length }}</span>
              <span class="tools-badge text-emerald-700 dark:text-emerald-300">成功 {{ rtSuccessCount }}</span>
              <span class="tools-badge text-red-700 dark:text-red-300">失败 {{ rtFailedCount }}</span>
            </div>

            <div class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-700">
              <div v-if="rtRows.length === 0" class="tools-empty">
                先预览或直接开始导入，这里会显示提取到的 RT。
              </div>
              <div v-else class="overflow-x-auto">
                <table class="min-w-[900px] divide-y divide-gray-200 text-sm dark:divide-dark-700">
                  <thead class="bg-gray-50 text-xs uppercase text-gray-500 dark:bg-dark-900/60 dark:text-dark-400">
                    <tr>
                      <th class="tools-th">#</th>
                      <th class="tools-th">Email</th>
                      <th class="tools-th">RT</th>
                      <th class="tools-th">状态</th>
                      <th class="tools-th">信息</th>
                    </tr>
                  </thead>
                  <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-800 dark:bg-dark-900/30">
                    <tr v-for="row in rtRows" :key="row.id">
                      <td class="tools-td text-gray-500">{{ row.index }}</td>
                      <td class="tools-td">{{ row.email || '-' }}</td>
                      <td class="tools-td font-mono text-xs">{{ maskToken(row.refreshToken) }}</td>
                      <td class="tools-td">
                        <span :class="rtStatusClass(row.status)">{{ rtStatusLabel(row.status) }}</span>
                      </td>
                      <td class="tools-td max-w-[360px] truncate">{{ row.message || '-' }}</td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>
          </div>
        </div>
      </section>

      <section v-else class="tools-surface">
        <div class="tools-section-header">
          <div>
            <h2 class="tools-title">Codex 配置脚本</h2>
            <p class="tools-description">生成本机 Codex 配置，只写 sub2api 用户 API Key，不导出上游 OAuth token。</p>
          </div>
          <button type="button" class="btn btn-primary btn-sm" @click="copyCodexScript">
            <Icon name="copy" size="sm" />
            复制脚本
          </button>
        </div>

        <div class="grid grid-cols-1 gap-4 2xl:grid-cols-[minmax(0,0.75fr)_minmax(0,1.25fr)]">
          <div class="space-y-4">
            <div>
              <label class="input-label">API Base URL</label>
              <input v-model="codexBaseUrl" class="input font-mono text-xs" @input="codexBaseTouched = true" />
            </div>
            <div>
              <label class="input-label">sub2api 用户 API Key</label>
              <input v-model="codexApiKey" class="input font-mono text-xs" type="password" placeholder="sk-..." />
            </div>
            <div class="grid grid-cols-1 gap-3 xl:grid-cols-2">
              <div>
                <label class="input-label">默认模型</label>
                <input v-model="codexModel" class="input" />
              </div>
              <label class="mt-7 flex items-center gap-2 text-sm text-gray-700 dark:text-dark-200">
                <input v-model="codexWebSocketV2" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
                启用 WebSocket v2
              </label>
            </div>
          </div>

          <pre class="max-h-[560px] overflow-auto rounded-lg border border-gray-200 bg-gray-950 p-4 text-xs leading-relaxed text-gray-100 dark:border-dark-700"><code>{{ codexScript }}</code></pre>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import type { ClashProxyPreviewResponse } from '@/api/admin/proxies'
import type { AdminDataImportResult, CreateAccountRequest, Proxy } from '@/types'

type ToolTab = 'proxy' | 'rt' | 'codex'
type IconName = InstanceType<typeof Icon>['$props']['name']
type RtStatus = 'pending' | 'running' | 'success' | 'failed'

interface RtRow {
  id: string
  index: number
  email: string
  refreshToken: string
  status: RtStatus
  message: string
}

const appStore = useAppStore()

const tabs: Array<{ id: ToolTab; label: string; icon: IconName }> = [
  { id: 'proxy', label: '代理导入', icon: 'server' },
  { id: 'rt', label: 'OpenAI RT 导入', icon: 'key' },
  { id: 'codex', label: 'Codex 配置脚本', icon: 'terminal' }
]

const activeTab = ref<ToolTab>('proxy')

const proxySubscriptionUrl = ref('')
const proxyYaml = ref('')
const proxyPreview = ref<ClashProxyPreviewResponse | null>(null)
const proxyPreviewLoading = ref(false)
const proxyImporting = ref(false)
const proxyImportResult = ref<AdminDataImportResult | null>(null)
const proxyBatchResult = ref<{ created: number; skipped: number } | null>(null)

const canImportProxies = computed(() => (proxyPreview.value?.data_payload.proxies.length ?? 0) > 0)
const displayedProxyRows = computed(() => proxyPreview.value?.rows.slice(0, 300) ?? [])

const activeProxies = ref<Proxy[]>([])
const rtInput = ref('')
const rtRows = ref<RtRow[]>([])
const importingRt = ref(false)
const rtProxyIdRaw = ref('')
const rtClientId = ref('')
const rtConcurrency = ref(3)
const openAIMobileClientId = 'app_LlGpXReQgckcGGUo2JrYvtJK'

const selectedRtProxyId = computed(() => {
  const id = Number.parseInt(rtProxyIdRaw.value, 10)
  return Number.isFinite(id) && id > 0 ? id : null
})
const normalizedRtConcurrency = computed(() => Math.min(5, Math.max(1, Number(rtConcurrency.value) || 3)))
const rtSuccessCount = computed(() => rtRows.value.filter((row) => row.status === 'success').length)
const rtFailedCount = computed(() => rtRows.value.filter((row) => row.status === 'failed').length)

const codexBaseTouched = ref(false)
const codexBaseUrl = ref('')
const codexApiKey = ref('')
const codexModel = ref('gpt-5.4')
const codexWebSocketV2 = ref(false)

const previewProxyFromUrl = async () => {
  await previewProxy({ url: proxySubscriptionUrl.value.trim() })
}

const previewProxyFromYaml = async () => {
  await previewProxy({ content: proxyYaml.value })
}

const previewProxy = async (payload: { url?: string; content?: string }) => {
  proxyPreviewLoading.value = true
  proxyImportResult.value = null
  proxyBatchResult.value = null
  try {
    proxyPreview.value = await adminAPI.proxies.previewClashImport(payload)
    appStore.showSuccess(`解析完成：有效 ${proxyPreview.value.summary.valid} 条`)
  } catch (error) {
    appStore.showError(errorMessage(error, '解析失败'))
  } finally {
    proxyPreviewLoading.value = false
  }
}

const importProxyData = async () => {
  if (!proxyPreview.value || !canImportProxies.value) return
  proxyImporting.value = true
  proxyBatchResult.value = null
  try {
    proxyImportResult.value = await adminAPI.proxies.importData({ data: proxyPreview.value.data_payload })
    appStore.showSuccess(`导入完成：创建 ${proxyImportResult.value.proxy_created}，复用 ${proxyImportResult.value.proxy_reused}`)
  } catch (error) {
    appStore.showError(errorMessage(error, '导入失败'))
  } finally {
    proxyImporting.value = false
  }
}

const importProxyBatch = async () => {
  if (!proxyPreview.value || !canImportProxies.value) return
  proxyImporting.value = true
  proxyImportResult.value = null
  try {
    proxyBatchResult.value = await adminAPI.proxies.batchCreate(proxyPreview.value.batch_payload.proxies)
    appStore.showSuccess(`批量导入完成：创建 ${proxyBatchResult.value.created}，跳过 ${proxyBatchResult.value.skipped}`)
  } catch (error) {
    appStore.showError(errorMessage(error, '批量导入失败'))
  } finally {
    proxyImporting.value = false
  }
}

const resetProxyImport = () => {
  proxySubscriptionUrl.value = ''
  proxyYaml.value = ''
  proxyPreview.value = null
  proxyImportResult.value = null
  proxyBatchResult.value = null
}

const parseRtInput = () => {
  const rows = extractRtRows(rtInput.value)
  rtRows.value = rows
  if (rows.length === 0) {
    appStore.showWarning('没有提取到 rt_ 或 rt- token')
    return
  }
  appStore.showSuccess(`提取到 ${rows.length} 条 RT`)
}

const importRtRows = async () => {
  if (rtRows.value.length === 0) {
    parseRtInput()
  }
  const rows = rtRows.value
  if (rows.length === 0) return

  importingRt.value = true
  rows.forEach((row) => {
    row.status = 'pending'
    row.message = ''
  })
  rtRows.value = [...rows]

  try {
    await runWithConcurrency(rows, normalizedRtConcurrency.value, importOneRtRow)
    const success = rtSuccessCount.value
    const failed = rtFailedCount.value
    if (success > 0 && failed === 0) {
      appStore.showSuccess(`RT 导入完成：成功 ${success} 条`)
    } else if (success > 0) {
      appStore.showWarning(`RT 部分导入完成：成功 ${success}，失败 ${failed}`)
    } else {
      appStore.showError('RT 导入失败')
    }
  } finally {
    importingRt.value = false
  }
}

const importOneRtRow = async (row: RtRow) => {
  updateRtRow(row, { status: 'running', message: '刷新 token 中...' })
  try {
    const clientId = rtClientId.value.trim() || undefined
    const tokenInfo = await adminAPI.accounts.refreshOpenAIToken(
      row.refreshToken,
      selectedRtProxyId.value,
      '/admin/openai/refresh-token',
      clientId
    )
    const credentials = buildOpenAICredentials(tokenInfo, row.refreshToken, clientId)
    const extra = buildOpenAIExtra(tokenInfo)
    const email = readString(tokenInfo, 'email') || row.email
    const accountName = email || `OpenAI OAuth Account #${row.index}`
    const payload: CreateAccountRequest = {
      name: accountName,
      notes: null,
      platform: 'openai',
      type: 'oauth',
      credentials,
      extra,
      proxy_id: selectedRtProxyId.value,
      concurrency: 10,
      priority: 1,
      rate_multiplier: 1,
      group_ids: [],
      expires_at: null,
      auto_pause_on_expired: true
    }
    await adminAPI.accounts.create(payload)
    updateRtRow(row, { status: 'success', message: `已创建 ${accountName}` })
  } catch (error) {
    updateRtRow(row, { status: 'failed', message: errorMessage(error, '导入失败') })
  }
}

const loadActiveProxies = async () => {
  try {
    activeProxies.value = await adminAPI.proxies.getAllWithCount()
  } catch {
    activeProxies.value = []
  }
}

const extractRtRows = (input: string): RtRow[] => {
  const emailRE = /[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}/gi
  const rtRE = /rt[_-][A-Za-z0-9._-]+/g
  const rows: RtRow[] = []
  const seen = new Set<string>()

  input.split(/\r?\n/).forEach((line) => {
    const emails = line.match(emailRE) || []
    const tokens = line.match(rtRE) || []
    for (const token of tokens) {
      if (seen.has(token)) continue
      seen.add(token)
      const email = emails[0] || ''
      rows.push({
        id: `${rows.length + 1}-${token.slice(0, 10)}`,
        index: rows.length + 1,
        email,
        refreshToken: token,
        status: 'pending',
        message: ''
      })
    }
  })

  if (rows.length > 0) return rows

  const allEmails = input.match(emailRE) || []
  const allTokens = input.match(rtRE) || []
  allTokens.forEach((token, index) => {
    if (seen.has(token)) return
    seen.add(token)
    rows.push({
      id: `${rows.length + 1}-${token.slice(0, 10)}`,
      index: rows.length + 1,
      email: allEmails[index] || '',
      refreshToken: token,
      status: 'pending',
      message: ''
    })
  })
  return rows
}

const runWithConcurrency = async <T,>(
  items: T[],
  concurrency: number,
  worker: (item: T, index: number) => Promise<void>
) => {
  let next = 0
  const runners = Array.from({ length: Math.min(concurrency, items.length) }, async () => {
    while (next < items.length) {
      const index = next++
      await worker(items[index], index)
    }
  })
  await Promise.all(runners)
}

const updateRtRow = (row: RtRow, patch: Partial<RtRow>) => {
  Object.assign(row, patch)
  rtRows.value = [...rtRows.value]
}

const buildOpenAICredentials = (
  tokenInfo: Record<string, unknown>,
  fallbackRefreshToken: string,
  clientId?: string
): Record<string, unknown> => {
  const accessToken = readString(tokenInfo, 'access_token')
  if (!accessToken) {
    throw new Error('refresh-token response missing access_token')
  }

  const credentials: Record<string, unknown> = {
    access_token: accessToken
  }
  const expiresAt = normalizeExpiresAt(tokenInfo.expires_at, tokenInfo.expires_in)
  if (expiresAt) {
    credentials.expires_at = expiresAt
  }

  const refreshToken = readString(tokenInfo, 'refresh_token') || fallbackRefreshToken
  if (refreshToken) credentials.refresh_token = refreshToken

  for (const key of [
    'id_token',
    'email',
    'chatgpt_account_id',
    'chatgpt_user_id',
    'organization_id',
    'plan_type',
    'subscription_expires_at'
  ]) {
    const value = readString(tokenInfo, key)
    if (value) credentials[key] = value
  }
  if (clientId) credentials.client_id = clientId
  return credentials
}

const buildOpenAIExtra = (tokenInfo: Record<string, unknown>): Record<string, unknown> | undefined => {
  const extra: Record<string, unknown> = {}
  for (const key of ['email', 'name', 'privacy_mode']) {
    const value = readString(tokenInfo, key)
    if (value) extra[key] = value
  }
  return Object.keys(extra).length > 0 ? extra : undefined
}

const readString = (source: Record<string, unknown>, key: string): string => {
  const value = source[key]
  return typeof value === 'string' ? value.trim() : ''
}

const normalizeExpiresAt = (expiresAt: unknown, expiresIn: unknown): string => {
  if (typeof expiresAt === 'number' && expiresAt > 0) {
    return new Date(expiresAt * 1000).toISOString()
  }
  if (typeof expiresAt === 'string' && expiresAt.trim()) {
    const numeric = Number(expiresAt)
    if (Number.isFinite(numeric) && numeric > 0) {
      return new Date(numeric * 1000).toISOString()
    }
    return expiresAt.trim()
  }
  if (typeof expiresIn === 'number' && expiresIn > 0) {
    return new Date(Date.now() + expiresIn * 1000).toISOString()
  }
  return new Date(Date.now() + 3600 * 1000).toISOString()
}

const proxyLabel = (proxy: Proxy) => {
  const count = typeof proxy.account_count === 'number' ? ` · ${proxy.account_count} 账号` : ''
  return `#${proxy.id} ${proxy.name} · ${proxy.protocol}://${proxy.host}:${proxy.port}${count}`
}

const maskToken = (token: string) => {
  if (token.length <= 14) return token
  return `${token.slice(0, 8)}...${token.slice(-6)}`
}

const rtStatusLabel = (status: RtStatus) => {
  switch (status) {
    case 'running':
      return '处理中'
    case 'success':
      return '成功'
    case 'failed':
      return '失败'
    default:
      return '待处理'
  }
}

const rtStatusClass = (status: RtStatus) => {
  switch (status) {
    case 'running':
      return 'status-running'
    case 'success':
      return 'status-ok'
    case 'failed':
      return 'status-error'
    default:
      return 'status-muted'
  }
}

function ensureV1(value: string): string {
  const raw = (value || window.location.origin).trim()
  const absolute = raw.startsWith('/')
    ? `${window.location.origin}${raw}`
    : raw
  const trimmed = absolute.replace(/\/+$/, '')
  if (trimmed.endsWith('/v1')) return trimmed
  if (trimmed.endsWith('/api')) return `${trimmed}/v1`
  if (trimmed.endsWith('/api/v1')) return trimmed
  return `${trimmed}/v1`
}

const defaultApiBaseUrl = computed(() => {
  const configured = appStore.cachedPublicSettings?.api_base_url || window.location.origin
  return ensureV1(configured)
})

watch(
  defaultApiBaseUrl,
  (value) => {
    if (!codexBaseTouched.value) {
      codexBaseUrl.value = value
    }
  },
  { immediate: true }
)

const codexScript = computed(() => {
  const baseUrl = ensureV1(codexBaseUrl.value || defaultApiBaseUrl.value)
  const apiKey = codexApiKey.value.trim() || 'PASTE_SUB2API_USER_API_KEY_HERE'
  const model = codexModel.value.trim() || 'gpt-5.4'
  const websocketConfig = codexWebSocketV2.value
    ? `
supports_websockets = true

[features]
responses_websockets_v2 = true`
    : ''

  return `#!/usr/bin/env bash
set -euo pipefail

CONFIG_DIR="\${CODEX_HOME:-$HOME/.codex}"
STAMP="$(date +%Y%m%d-%H%M%S)"
mkdir -p "$CONFIG_DIR"

if [ -f "$CONFIG_DIR/config.toml" ]; then
  cp "$CONFIG_DIR/config.toml" "$CONFIG_DIR/config.toml.bak-$STAMP"
fi
if [ -f "$CONFIG_DIR/auth.json" ]; then
  cp "$CONFIG_DIR/auth.json" "$CONFIG_DIR/auth.json.bak-$STAMP"
fi

cat > "$CONFIG_DIR/config.toml" <<'SUB2API_CODEX_CONFIG'
model_provider = "sub2api"
model = ${tomlString(model)}
review_model = ${tomlString(model)}
model_reasoning_effort = "xhigh"
disable_response_storage = true
network_access = "enabled"
windows_wsl_setup_acknowledged = true
model_context_window = 1000000
model_auto_compact_token_limit = 900000

[model_providers.sub2api]
name = "sub2api"
base_url = ${tomlString(baseUrl)}
wire_api = "responses"
requires_openai_auth = true${websocketConfig}
SUB2API_CODEX_CONFIG

cat > "$CONFIG_DIR/auth.json" <<'SUB2API_CODEX_AUTH'
${JSON.stringify({ OPENAI_API_KEY: apiKey }, null, 2)}
SUB2API_CODEX_AUTH

chmod 600 "$CONFIG_DIR/auth.json"
echo "Codex config written to $CONFIG_DIR"
`
})

const copyCodexScript = async () => {
  await copyText(codexScript.value, '脚本已复制')
}

const copyText = async (text: string, successMessage: string) => {
  try {
    await navigator.clipboard.writeText(text)
    appStore.showSuccess(successMessage)
  } catch (error) {
    appStore.showError(errorMessage(error, '复制失败'))
  }
}

const tomlString = (value: string) => JSON.stringify(value)

const errorMessage = (error: unknown, fallback: string) => {
  if (error && typeof error === 'object') {
    const maybe = error as { message?: unknown; response?: { data?: { detail?: unknown; message?: unknown } } }
    const detail = maybe.response?.data?.detail || maybe.response?.data?.message || maybe.message
    if (typeof detail === 'string' && detail.trim()) return detail
  }
  return fallback
}

onMounted(() => {
  void loadActiveProxies()
})
</script>

<style scoped>
.tools-surface {
  @apply min-w-0 rounded-lg border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-900/70;
}

.tools-section-header {
  @apply mb-4 flex min-w-0 flex-col gap-3 border-b border-gray-100 pb-4 dark:border-dark-800 lg:flex-row lg:items-start lg:justify-between;
}

.tools-title {
  @apply text-base font-semibold text-gray-900 dark:text-white;
}

.tools-description {
  @apply mt-1 text-sm text-gray-500 dark:text-dark-400;
}

.tools-tab {
  @apply inline-flex items-center gap-2 rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm font-medium text-gray-600 shadow-sm transition-colors hover:border-gray-300 hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-900 dark:text-dark-300 dark:hover:border-dark-600 dark:hover:bg-dark-800;
}

.tools-tab-active {
  @apply border-primary-300 bg-primary-50 text-primary-700 dark:border-primary-800 dark:bg-primary-950/40 dark:text-primary-300;
}

.tools-badges {
  @apply flex flex-wrap items-center gap-2;
}

.tools-badge {
  @apply inline-flex items-center rounded-full border border-gray-200 bg-gray-50 px-2.5 py-1 text-xs font-medium text-gray-600 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-300;
}

.tools-empty {
  @apply flex min-h-[220px] items-center justify-center px-4 py-10 text-center text-sm text-gray-500 dark:text-dark-400;
}

.tools-th {
  @apply whitespace-nowrap px-3 py-2 text-left font-semibold;
}

.tools-td {
  @apply whitespace-nowrap px-3 py-2 align-middle text-gray-700 dark:text-dark-200;
}

.tools-code {
  @apply rounded bg-gray-100 px-1.5 py-0.5 font-mono text-xs text-gray-700 dark:bg-dark-800 dark:text-dark-200;
}

.status-ok {
  @apply rounded-full bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300;
}

.status-error {
  @apply rounded-full bg-red-50 px-2 py-0.5 text-xs font-medium text-red-700 dark:bg-red-950/40 dark:text-red-300;
}

.status-muted {
  @apply rounded-full bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600 dark:bg-dark-800 dark:text-dark-300;
}

.status-running {
  @apply rounded-full bg-sky-50 px-2 py-0.5 text-xs font-medium text-sky-700 dark:bg-sky-950/40 dark:text-sky-300;
}
</style>
