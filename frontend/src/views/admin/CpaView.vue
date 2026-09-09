<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- ============ 连接配置 ============ -->
      <section class="card">
        <div class="card-header flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">
              {{ t('admin.cpa.connection.title') }}
            </h2>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.cpa.connection.baseUrlHint') }}
            </p>
          </div>
          <div class="flex items-center gap-2">
            <span
              v-if="version"
              class="badge"
              :class="version.reachable ? 'badge-success' : 'badge-warning'"
            >
              {{ version.reachable ? t('admin.cpa.connection.reachable') : t('admin.cpa.connection.unreachable') }}
            </span>
            <span v-if="version?.version" class="badge badge-primary">CPA {{ version.version }}</span>
          </div>
        </div>

        <div class="card-body space-y-4">
          <div class="grid gap-4 md:grid-cols-2">
            <div>
              <label class="input-label" for="cpa-base-url">
                {{ t('admin.cpa.connection.baseUrl') }}
              </label>
              <input
                id="cpa-base-url"
                v-model="form.baseUrl"
                type="text"
                class="input"
                placeholder="http://host.docker.internal:8317"
              />
            </div>
            <div>
              <label class="input-label" for="cpa-management-key">
                {{ t('admin.cpa.connection.managementKey') }}
              </label>
              <input
                id="cpa-management-key"
                v-model="form.managementKey"
                :type="showKey ? 'text' : 'password'"
                class="input"
                autocomplete="new-password"
                :placeholder="t('admin.cpa.connection.managementKeyPlaceholder')"
              />
              <p v-if="config?.management_key_configured" class="input-hint">
                {{ t('admin.cpa.connection.managementKeyKeep', { hint: config.management_key_hint }) }}
              </p>
            </div>
          </div>

          <div class="flex flex-wrap items-center gap-4">
            <label class="flex cursor-pointer items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
              <Toggle v-model="form.enabled" />
              {{ t('admin.cpa.connection.enabled') }}
            </label>

            <div class="ml-auto flex flex-wrap items-center gap-2">
              <button class="btn btn-secondary" :disabled="testing" @click="handleTest">
                <Icon name="play" size="md" class="mr-2" />
                {{ testing ? t('admin.cpa.connection.testing') : t('admin.cpa.connection.test') }}
              </button>
              <button class="btn btn-primary" :disabled="saving" @click="handleSave">
                <Icon name="check" size="md" class="mr-2" />
                {{ t('admin.cpa.connection.save') }}
              </button>
            </div>
          </div>

          <!-- 测试结果 -->
          <div v-if="version" class="rounded-lg bg-gray-50 p-3 text-xs dark:bg-dark-700">
            <div class="flex flex-wrap gap-x-6 gap-y-1">
              <span>
                <span class="text-gray-500 dark:text-gray-400">{{ t('admin.cpa.connection.healthOk') }}:</span>
                <span :class="version.health_ok ? 'text-emerald-600' : 'text-red-500'">
                  {{ version.health_ok ? t('admin.cpa.common.yes') : t('admin.cpa.common.no') }}
                </span>
              </span>
              <span>
                <span class="text-gray-500 dark:text-gray-400">{{ t('admin.cpa.connection.keyAccepted') }}:</span>
                <span :class="version.key_accepted ? 'text-emerald-600' : 'text-red-500'">
                  {{ version.key_accepted ? t('admin.cpa.common.yes') : t('admin.cpa.common.no') }}
                </span>
              </span>
              <span v-if="version.commit">
                <span class="text-gray-500 dark:text-gray-400">{{ t('admin.cpa.connection.commit') }}:</span>
                <span class="font-mono">{{ version.commit }}</span>
              </span>
              <span v-if="version.build_date">
                <span class="text-gray-500 dark:text-gray-400">{{ t('admin.cpa.connection.buildDate') }}:</span>
                {{ version.build_date }}
              </span>
            </div>
            <p v-if="version.message" class="mt-1 text-amber-600 dark:text-amber-400">{{ version.message }}</p>
          </div>
        </div>
      </section>

      <!-- ============ Tabs ============ -->
      <div class="flex flex-wrap items-center gap-2 border-b border-gray-200 dark:border-dark-700">
        <button
          v-for="tab in tabs"
          :key="tab.id"
          class="-mb-px border-b-2 px-4 py-2 text-sm font-medium transition-colors"
          :class="
            activeTab === tab.id
              ? 'border-primary-500 text-primary-600 dark:text-primary-400'
              : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'
          "
          @click="activeTab = tab.id"
        >
          {{ t(tab.label) }}
        </button>
        <div class="ml-auto pb-2">
          <button class="btn btn-secondary btn-sm" :disabled="loadingTab" @click="reloadActiveTab">
            <Icon name="refresh" size="sm" class="mr-1" :class="loadingTab ? 'animate-spin' : ''" />
            {{ t('admin.cpa.common.refresh') }}
          </button>
        </div>
      </div>

      <!-- ============ 凭据与额度 ============ -->
      <section v-if="activeTab === 'authFiles'" class="space-y-4">
        <p v-if="tabError" class="text-sm text-red-500">{{ t('admin.cpa.common.error', { message: tabError }) }}</p>
        <div v-else-if="!authFiles.length" class="card card-body text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.cpa.authFiles.empty') }}
        </div>

        <article
          v-for="file in authFiles"
          :key="file.auth_index || file.name"
          class="card card-hover overflow-hidden"
        >
          <!-- 顶部状态色条 -->
          <div class="h-1 w-full" :class="statusAccent(file)" />

          <div class="p-5">
            <!-- 头部：供应商图标 + 名称 + 状态 + 操作 -->
            <div class="flex items-start gap-3">
              <div class="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl" :class="providerIconClass(file)">
                <Icon :name="providerIcon(file)" size="md" />
              </div>

              <div class="min-w-0 flex-1">
                <div class="flex flex-wrap items-center gap-2">
                  <h3 class="truncate text-sm font-semibold text-gray-900 dark:text-white" :title="file.name">
                    {{ file.name }}
                  </h3>
                  <span class="badge" :class="statusClass(file)">{{ statusLabel(file) }}</span>
                  <span v-if="filePlanType(file)" class="badge badge-success">{{ filePlanType(file) }}</span>
                  <span v-if="file.priority != null" class="badge badge-gray">P{{ file.priority }}</span>
                </div>
                <div class="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-gray-500 dark:text-gray-400">
                  <span class="inline-flex items-center gap-1">
                    <Icon name="cube" size="xs" />{{ file.provider || file.type || '—' }}
                  </span>
                  <span v-if="file.email" class="inline-flex min-w-0 items-center gap-1">
                    <Icon name="mail" size="xs" /><span class="truncate" :title="file.email">{{ file.email }}</span>
                  </span>
                  <span v-if="file.note" class="truncate" :title="file.note">· {{ file.note }}</span>
                </div>
              </div>

              <button
                class="btn btn-secondary btn-sm shrink-0"
                :disabled="quotaLoading[file.auth_index]"
                @click="loadQuota(file)"
              >
                <Icon
                  name="refresh"
                  size="sm"
                  class="mr-1"
                  :class="quotaLoading[file.auth_index] ? 'animate-spin' : ''"
                />
                {{
                  quotaLoading[file.auth_index]
                    ? t('admin.cpa.authFiles.quotaLoading')
                    : t('admin.cpa.authFiles.refreshQuota')
                }}
              </button>
            </div>

            <!-- 统计格子 -->
            <div class="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-4">
              <div class="rounded-xl bg-emerald-50 px-3 py-2 dark:bg-emerald-900/15">
                <p class="text-[11px] font-medium text-emerald-700/80 dark:text-emerald-400/80">
                  {{ t('admin.cpa.authFiles.success') }}
                </p>
                <p class="text-lg font-semibold leading-tight text-emerald-600 dark:text-emerald-400">
                  {{ file.success ?? 0 }}
                </p>
              </div>
              <div class="rounded-xl bg-red-50 px-3 py-2 dark:bg-red-900/15">
                <p class="text-[11px] font-medium text-red-700/80 dark:text-red-400/80">
                  {{ t('admin.cpa.authFiles.failed') }}
                </p>
                <p class="text-lg font-semibold leading-tight text-red-600 dark:text-red-400">
                  {{ file.failed ?? 0 }}
                </p>
              </div>
              <div class="rounded-xl bg-gray-50 px-3 py-2 dark:bg-dark-700/60">
                <p class="text-[11px] font-medium text-gray-500 dark:text-gray-400">
                  {{ t('admin.cpa.authFiles.successRate') }}
                </p>
                <p class="text-lg font-semibold leading-tight text-gray-800 dark:text-gray-100">
                  {{ successRate(file) }}
                </p>
              </div>
              <div class="rounded-xl bg-gray-50 px-3 py-2 dark:bg-dark-700/60">
                <p class="text-[11px] font-medium text-gray-500 dark:text-gray-400">
                  {{ t('admin.cpa.authFiles.lastRefresh') }}
                </p>
                <p
                  class="truncate text-sm font-medium leading-tight text-gray-800 dark:text-gray-100"
                  :title="formatTime(file.last_refresh)"
                >
                  {{ relativeTime(file.last_refresh) }}
                </p>
              </div>
            </div>

            <!-- 额度（api-call → chatgpt wham/usage） -->
            <div class="mt-4">
              <div class="mb-2 flex items-center gap-2">
                <Icon name="chartBar" size="sm" class="text-gray-400 dark:text-gray-500" />
                <span class="text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
                  {{ t('admin.cpa.authFiles.quotaTitle') }}
                </span>
                <span
                  v-if="quotaResetCredits(quota[file.auth_index]) !== null"
                  class="badge badge-gray ml-auto"
                >
                  {{ t('admin.cpa.authFiles.resetCredits') }} {{ quotaResetCredits(quota[file.auth_index]) }}
                </span>
              </div>

              <div v-if="quota[file.auth_index]" class="space-y-2">
                <UsageProgressBar
                  v-for="row in quotaRows(quota[file.auth_index])"
                  :key="row.key"
                  :label="row.label"
                  :utilization="row.usedPercent"
                  :resets-at="row.resetsAt"
                  :color="row.color"
                  label-width="auto"
                />
              </div>
              <p v-else-if="quotaError[file.auth_index]" class="text-xs text-red-500">
                {{ t('admin.cpa.authFiles.quotaError', { message: quotaError[file.auth_index] }) }}
              </p>
              <p v-else class="text-xs text-gray-400 dark:text-gray-500">
                {{ t('admin.cpa.authFiles.noQuotaYet') }}
              </p>
            </div>

            <!-- 最近请求 + 被动信号 -->
            <div class="mt-4 grid gap-4 border-t border-gray-100 pt-4 dark:border-dark-700 lg:grid-cols-2">
              <div>
                <div class="mb-2 flex items-baseline gap-2">
                  <span class="text-xs font-medium text-gray-500 dark:text-gray-400">
                    {{ t('admin.cpa.authFiles.recentRequests') }}
                  </span>
                  <span class="text-[11px] text-gray-400 dark:text-gray-500">{{ requestTotals(file) }}</span>
                </div>
                <div v-if="(file.recent_requests || []).length" class="flex h-8 items-end gap-1">
                  <div
                    v-for="(bucket, index) in file.recent_requests"
                    :key="index"
                    class="relative h-full flex-1 overflow-hidden rounded-sm bg-gray-100 dark:bg-dark-600"
                    :title="`${formatTime(bucket.time)} · ✓${bucket.success} ✗${bucket.failed}`"
                  >
                    <div
                      class="absolute bottom-0 w-full bg-emerald-500"
                      :style="{ height: bucketSegments(bucket, bucketMax(file)).success }"
                    />
                    <div
                      class="absolute w-full bg-red-400"
                      :style="{
                        bottom: bucketSegments(bucket, bucketMax(file)).success,
                        height: bucketSegments(bucket, bucketMax(file)).failed
                      }"
                    />
                  </div>
                </div>
                <p v-else class="text-xs text-gray-400 dark:text-gray-500">
                  {{ t('admin.cpa.authFiles.noSignals') }}
                </p>
              </div>

              <div>
                <div class="mb-2 text-xs font-medium text-gray-500 dark:text-gray-400">
                  {{ t('admin.cpa.authFiles.passiveSignals') }}
                </div>
                <div v-if="signalRows(file).length" class="flex flex-wrap gap-1.5">
                  <span
                    v-for="signal in signalRows(file)"
                    :key="signal.key"
                    class="inline-flex max-w-full items-center gap-1 rounded-lg bg-gray-100 px-2 py-0.5 text-[11px] dark:bg-dark-700"
                  >
                    <span class="truncate text-gray-500 dark:text-gray-400" :title="signal.key">{{ signal.key }}</span>
                    <span class="font-semibold text-gray-800 dark:text-gray-100">{{ signal.value }}</span>
                  </span>
                </div>
                <p v-else class="text-xs text-gray-400 dark:text-gray-500">
                  {{ t('admin.cpa.authFiles.noSignals') }}
                </p>
                <p v-if="file.quota?.observed_at" class="mt-1 text-[11px] text-gray-400 dark:text-gray-500">
                  {{ t('admin.cpa.authFiles.observedAt', { time: formatTime(file.quota.observed_at) }) }}
                </p>
              </div>
            </div>

            <!-- 元信息 -->
            <div
              v-if="file.next_retry_after || file.status_message"
              class="mt-3 flex flex-wrap gap-x-5 gap-y-1 text-[11px] text-gray-500 dark:text-gray-400"
            >
              <span v-if="file.next_retry_after">
                {{ t('admin.cpa.authFiles.nextRetry') }}: {{ formatTime(file.next_retry_after) }}
              </span>
              <span v-if="file.status_message">{{ file.status_message }}</span>
            </div>
          </div>
        </article>
      </section>

      <!-- ============ 使用情况 ============ -->
      <section v-else-if="activeTab === 'usage'" class="space-y-4">
        <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cpa.usage.hint') }}</p>
        <p v-if="tabError" class="text-sm text-red-500">{{ t('admin.cpa.common.error', { message: tabError }) }}</p>
        <div v-else-if="!usageRows.length" class="card card-body text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.cpa.usage.empty') }}
        </div>
        <div v-else class="card overflow-x-auto">
          <table class="table">
            <thead>
              <tr>
                <th>{{ t('admin.cpa.usage.provider') }}</th>
                <th>{{ t('admin.cpa.usage.key') }}</th>
                <th class="text-right">{{ t('admin.cpa.usage.success') }}</th>
                <th class="text-right">{{ t('admin.cpa.usage.failed') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="row in usageRows" :key="row.provider + row.key">
                <td>{{ row.provider }}</td>
                <td class="max-w-xs truncate font-mono text-xs" :title="row.key">{{ row.key }}</td>
                <td class="text-right text-emerald-600 dark:text-emerald-400">{{ row.success }}</td>
                <td class="text-right text-red-500">{{ row.failed }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <!-- ============ 日志 ============ -->
      <section v-else class="space-y-4">
        <div class="card">
          <div class="card-header">
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('admin.cpa.logs.requestLogs') }}
            </h3>
          </div>
          <div class="card-body text-sm">
            <p v-if="requestLogsDisabled" class="text-amber-600 dark:text-amber-400">
              {{ t('admin.cpa.logs.requestLogsDisabled') }}
            </p>
            <p v-else-if="requestLogs?.lines?.length" class="max-h-64 overflow-auto font-mono text-xs">
              <span v-for="(line, index) in requestLogs.lines" :key="index" class="block">{{ line }}</span>
            </p>
            <p v-else class="text-gray-500 dark:text-gray-400">{{ t('admin.cpa.logs.empty') }}</p>
          </div>
        </div>

        <div class="card">
          <div class="card-header">
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('admin.cpa.logs.errorLogs') }}
            </h3>
          </div>
          <div class="card-body">
            <p v-if="tabError" class="text-sm text-red-500">{{ t('admin.cpa.common.error', { message: tabError }) }}</p>
            <p v-else-if="!errorLogs.length" class="text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.cpa.logs.empty') }}
            </p>
            <div v-else class="overflow-x-auto">
              <table class="table">
                <thead>
                  <tr>
                    <th>{{ t('admin.cpa.logs.fileName') }}</th>
                    <th class="text-right">{{ t('admin.cpa.logs.size') }}</th>
                    <th>{{ t('admin.cpa.logs.modified') }}</th>
                    <th class="text-right">{{ t('admin.cpa.logs.view') }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="file in errorLogs" :key="file.name">
                    <td class="font-mono text-xs">{{ file.name }}</td>
                    <td class="text-right">{{ formatBytes(file.size) }}</td>
                    <td>{{ formatTime(file.modified * 1000) }}</td>
                    <td class="text-right">
                      <button class="btn btn-secondary btn-sm" @click="openLog(file.name)">
                        <Icon name="eye" size="sm" />
                      </button>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </section>
    </div>

    <!-- 日志内容弹窗 -->
    <BaseDialog :show="logDialog.open" :title="t('admin.cpa.logs.contentTitle', { name: logDialog.name })" @close="logDialog.open = false">
      <pre class="max-h-[60vh] overflow-auto whitespace-pre-wrap break-all text-xs">{{
        logDialog.content || t('admin.cpa.logs.contentEmpty')
      }}</pre>
      <template #footer>
        <button class="btn btn-secondary" @click="logDialog.open = false">
          {{ t('admin.cpa.logs.close') }}
        </button>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import UsageProgressBar from '@/components/account/UsageProgressBar.vue'
import { useAppStore } from '@/stores'
import { formatRelativeTime } from '@/utils/format'
import { cpaApi } from '@/api/cpa'
import type {
  CpaAuthFile,
  CpaCodexRateLimit,
  CpaCodexUsagePayload,
  CpaCodexWindow,
  CpaConfigView,
  CpaErrorLogFile,
  CpaLogsResponse,
  CpaRecentRequestBucket,
  CpaVersionInfo
} from '@/api/cpa'

type TabId = 'authFiles' | 'usage' | 'logs'

const { t } = useI18n()
const appStore = useAppStore()

const tabs: Array<{ id: TabId; label: string }> = [
  { id: 'authFiles', label: 'admin.cpa.tabs.authFiles' },
  { id: 'usage', label: 'admin.cpa.tabs.usage' },
  { id: 'logs', label: 'admin.cpa.tabs.logs' }
]

const activeTab = ref<TabId>('authFiles')
const config = ref<CpaConfigView | null>(null)
const version = ref<CpaVersionInfo | null>(null)
const showKey = ref(false)
const saving = ref(false)
const testing = ref(false)
const loadingTab = ref(false)
const tabError = ref('')

const form = reactive({
  enabled: false,
  baseUrl: '',
  managementKey: ''
})

const authFiles = ref<CpaAuthFile[]>([])
const quota = reactive<Record<string, CpaCodexUsagePayload>>({})
const quotaError = reactive<Record<string, string>>({})
const quotaLoading = reactive<Record<string, boolean>>({})
const usageRows = ref<Array<{ provider: string; key: string; success: number; failed: number }>>([])
const errorLogs = ref<CpaErrorLogFile[]>([])
const requestLogs = ref<CpaLogsResponse | null>(null)
const requestLogsDisabled = ref(false)
const logDialog = reactive({ open: false, name: '', content: '' })

onMounted(async () => {
  await loadConfig()
  await reloadActiveTab()
})

async function loadConfig() {
  try {
    const cfg = await cpaApi.getConfig()
    config.value = cfg
    form.enabled = cfg.enabled
    form.baseUrl = cfg.base_url
  } catch (error) {
    appStore.showError(errorMessage(error))
  }
}

async function handleSave() {
  saving.value = true
  try {
    const cfg = await cpaApi.updateConfig({
      enabled: form.enabled,
      base_url: form.baseUrl,
      management_key: form.managementKey.trim() === '' ? null : form.managementKey
    })
    config.value = cfg
    form.managementKey = ''
    appStore.showSuccess(t('admin.cpa.connection.saved'))
    await reloadActiveTab()
  } catch (error) {
    appStore.showError(errorMessage(error))
  } finally {
    saving.value = false
  }
}

async function handleTest() {
  testing.value = true
  try {
    version.value = await cpaApi.testConnection()
  } catch (error) {
    appStore.showError(errorMessage(error))
  } finally {
    testing.value = false
  }
}

async function reloadActiveTab() {
  loadingTab.value = true
  tabError.value = ''
  try {
    if (activeTab.value === 'authFiles') {
      const list = await cpaApi.listAuthFiles()
      authFiles.value = list.files || []
      // 自动拉取 codex 凭据的真实额度（一次页面加载只做一轮）
      for (const file of authFiles.value) {
        if (isCodex(file) && !quota[file.auth_index] && !quotaLoading[file.auth_index]) {
          void loadQuota(file)
        }
      }
    } else if (activeTab.value === 'usage') {
      const usage = await cpaApi.getApiKeyUsage()
      usageRows.value = flattenUsage(usage)
    } else {
      requestLogsDisabled.value = false
      requestLogs.value = null
      try {
        requestLogs.value = await cpaApi.getLogs({ limit: 200 })
      } catch (error) {
        const message = errorMessage(error)
        // CPA 未开启 logging-to-file 时返回 400，属于预期状态，其余错误照常上报
        if (/logging/i.test(message)) {
          requestLogsDisabled.value = true
        } else {
          throw error
        }
      }
      const logs = await cpaApi.listErrorLogs()
      errorLogs.value = logs.files || []
    }
  } catch (error) {
    tabError.value = errorMessage(error)
  } finally {
    loadingTab.value = false
  }
}

async function loadQuota(file: CpaAuthFile) {
  if (!file.auth_index) return
  quotaLoading[file.auth_index] = true
  delete quotaError[file.auth_index]
  try {
    quota[file.auth_index] = await cpaApi.getAuthFileQuota(file.auth_index)
  } catch (error) {
    quotaError[file.auth_index] = errorMessage(error)
  } finally {
    quotaLoading[file.auth_index] = false
  }
}

async function openLog(name: string) {
  try {
    const result = await cpaApi.getErrorLogContent(name)
    logDialog.name = result.name
    logDialog.content = result.content
    logDialog.open = true
  } catch (error) {
    appStore.showError(errorMessage(error))
  }
}

function isCodex(file: CpaAuthFile): boolean {
  const provider = (file.provider || file.type || '').toLowerCase()
  return provider === 'codex' || provider === 'openai'
}

/** 套餐类型来自 api-call 的真实额度响应，被动信号里没有。 */
function filePlanType(file: CpaAuthFile): string {
  const payload = quota[file.auth_index]
  if (!payload) return ''
  return payload.plan_type || payload.planType || ''
}

function statusClass(file: CpaAuthFile): string {
  if (file.disabled) return 'badge-warning'
  if (file.unavailable) return 'badge-danger'
  const status = (file.status || '').toLowerCase()
  if (status === 'active' || status === 'ready' || status === 'ok') return 'badge-success'
  return 'badge-warning'
}

/** 卡片顶部 1px 状态色条。 */
function statusAccent(file: CpaAuthFile): string {
  if (file.unavailable) return 'bg-red-400'
  if (file.disabled) return 'bg-amber-400'
  const status = (file.status || '').toLowerCase()
  if (status === 'active' || status === 'ready' || status === 'ok') return 'bg-emerald-400'
  return 'bg-gray-300 dark:bg-dark-600'
}

function statusLabel(file: CpaAuthFile): string {
  if (file.unavailable) return t('admin.cpa.authFiles.unavailable')
  if (file.disabled) return t('admin.cpa.authFiles.disabled')
  return file.status || t('admin.cpa.common.unknown')
}

/** 供应商图标：按 provider 挑一个语义接近的图标。 */
function providerIcon(file: CpaAuthFile): 'sparkles' | 'brain' | 'cloud' | 'key' {
  const provider = (file.provider || file.type || '').toLowerCase()
  if (provider === 'codex' || provider === 'openai') return 'sparkles'
  if (provider === 'claude' || provider === 'anthropic') return 'brain'
  if (provider === 'gemini' || provider === 'google') return 'cloud'
  return 'key'
}

function providerIconClass(file: CpaAuthFile): string {
  if (file.unavailable) return 'bg-red-50 text-red-500 dark:bg-red-900/20 dark:text-red-400'
  if (file.disabled) return 'bg-amber-50 text-amber-500 dark:bg-amber-900/20 dark:text-amber-400'
  const status = (file.status || '').toLowerCase()
  if (status === 'active' || status === 'ready' || status === 'ok') {
    return 'bg-emerald-50 text-emerald-500 dark:bg-emerald-900/20 dark:text-emerald-400'
  }
  return 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400'
}

/** 成功率：没有请求样本时显示占位符而不是 0%。 */
function successRate(file: CpaAuthFile): string {
  const success = file.success ?? 0
  const failed = file.failed ?? 0
  const total = success + failed
  if (total <= 0) return '—'
  return `${((success / total) * 100).toFixed(1)}%`
}

function requestTotals(file: CpaAuthFile): string {
  return `✓ ${file.success ?? 0} · ✗ ${file.failed ?? 0}`
}

function signalRows(file: CpaAuthFile): Array<{ key: string; value: string }> {
  const signals = file.quota?.signals || {}
  return Object.entries(signals)
    .map(([key, value]) => ({ key, value }))
    .slice(0, 12)
}

function bucketMax(file: CpaAuthFile): number {
  const buckets = file.recent_requests || []
  return Math.max(1, ...buckets.map((item) => item.success + item.failed))
}

/** 单桶堆叠高度：绿色=成功、红色=失败，均相对该凭据的最大桶。 */
function bucketSegments(bucket: CpaRecentRequestBucket, max: number): { success: string; failed: string } {
  const success = ((bucket.success || 0) / max) * 100
  const failed = ((bucket.failed || 0) / max) * 100
  return { success: `${success}%`, failed: `${failed}%` }
}

function relativeTime(value?: string | null): string {
  if (!value) return '—'
  const formatted = formatRelativeTime(value)
  return formatted || '—'
}

function pickWindow(rateLimit: CpaCodexRateLimit | null | undefined, primary: boolean): CpaCodexWindow | null {
  if (!rateLimit) return null
  return primary
    ? rateLimit.primary_window || rateLimit.primaryWindow || null
    : rateLimit.secondary_window || rateLimit.secondaryWindow || null
}

function normalizeNumber(value: unknown): number | null {
  if (typeof value === 'number' && Number.isFinite(value)) return value
  if (typeof value === 'string') {
    const parsed = Number(value.trim())
    return Number.isFinite(parsed) ? parsed : null
  }
  return null
}

function windowResetISO(win: CpaCodexWindow | null): string | null {
  if (!win) return null
  const resetAt = normalizeNumber(win.reset_at ?? win.resetAt)
  if (resetAt !== null && resetAt > 0) {
    // reset_at 为秒级 epoch（Codex 返回的是秒）
    return new Date(resetAt * 1000).toISOString()
  }
  const afterSeconds = normalizeNumber(win.reset_after_seconds ?? win.resetAfterSeconds)
  if (afterSeconds !== null) {
    return new Date(Date.now() + afterSeconds * 1000).toISOString()
  }
  return null
}

/**
 * Codex 的 primary/secondary 并不固定对应 5h/周：Pro 账号的 primary 可能就是
 * 7 天窗口（实测 limit_window_seconds=604800）。因此按窗口时长判断标签。
 */
function windowLabel(win: CpaCodexWindow | null, fallback: string): string {
  if (!win) return fallback
  const seconds = normalizeNumber(win.limit_window_seconds ?? win.limitWindowSeconds)
  if (seconds === null || seconds <= 0) return fallback
  const hours = seconds / 3600
  if (hours <= 6) return t('admin.cpa.authFiles.fiveHour')
  if (hours <= 24 * 8) return t('admin.cpa.authFiles.weekly')
  return t('admin.cpa.authFiles.monthly')
}

function quotaRows(payload: CpaCodexUsagePayload | undefined): Array<{
  key: string
  label: string
  usedPercent: number
  resetsAt: string | null
  color: 'indigo' | 'emerald' | 'purple' | 'amber'
}> {
  if (!payload) return []
  const rows: Array<{
    key: string
    label: string
    usedPercent: number
    resetsAt: string | null
    color: 'indigo' | 'emerald' | 'purple' | 'amber'
  }> = []
  const rateLimit = payload.rate_limit || payload.rateLimit || null
  const primary = pickWindow(rateLimit, true)
  const secondary = pickWindow(rateLimit, false)
  if (primary) {
    rows.push({
      key: 'primary',
      label: windowLabel(primary, t('admin.cpa.authFiles.fiveHour')),
      usedPercent: normalizeNumber(primary.used_percent ?? primary.usedPercent) ?? 0,
      resetsAt: windowResetISO(primary),
      color: 'indigo'
    })
  }
  if (secondary) {
    rows.push({
      key: 'secondary',
      label: windowLabel(secondary, t('admin.cpa.authFiles.weekly')),
      usedPercent: normalizeNumber(secondary.used_percent ?? secondary.usedPercent) ?? 0,
      resetsAt: windowResetISO(secondary),
      color: 'emerald'
    })
  }
  const codeReview = payload.code_review_rate_limit || payload.codeReviewRateLimit || null
  const reviewPrimary = pickWindow(codeReview, true)
  if (reviewPrimary) {
    rows.push({
      key: 'code-review',
      label: t('admin.cpa.authFiles.codeReview'),
      usedPercent: normalizeNumber(reviewPrimary.used_percent ?? reviewPrimary.usedPercent) ?? 0,
      resetsAt: windowResetISO(reviewPrimary),
      color: 'purple'
    })
  }
  const additional = payload.additional_rate_limits || payload.additionalRateLimits || []
  additional.forEach((entry, index) => {
    const name = entry.limit_name || entry.limitName || entry.metered_feature || entry.meteredFeature || `#${index + 1}`
    const win = pickWindow(entry.rate_limit || entry.rateLimit || null, true)
    if (!win) return
    rows.push({
      key: `additional-${index}`,
      label: t('admin.cpa.authFiles.additional', { name }),
      usedPercent: normalizeNumber(win.used_percent ?? win.usedPercent) ?? 0,
      resetsAt: windowResetISO(win),
      color: 'amber'
    })
  })
  return rows
}

function quotaResetCredits(payload: CpaCodexUsagePayload | undefined): number | null {
  if (!payload) return null
  const credits = payload.rate_limit_reset_credits || payload.rateLimitResetCredits || null
  if (!credits) return null
  return normalizeNumber(credits.applicable_available_count ?? credits.applicableAvailableCount ?? credits.available_count ?? credits.availableCount)
}

function flattenUsage(usage: Record<string, Record<string, { success: number; failed: number }>>) {
  const rows: Array<{ provider: string; key: string; success: number; failed: number }> = []
  Object.entries(usage || {}).forEach(([provider, entries]) => {
    Object.entries(entries || {}).forEach(([compositeKey, entry]) => {
      rows.push({
        provider,
        key: compositeKey.split('|').pop() || compositeKey,
        success: entry.success || 0,
        failed: entry.failed || 0
      })
    })
  })
  return rows.sort((a, b) => b.success + b.failed - (a.success + a.failed))
}

function formatTime(value?: string | number | null): string {
  if (value === undefined || value === null || value === '') return '—'
  const date = typeof value === 'number' ? new Date(value) : new Date(value)
  if (Number.isNaN(date.getTime())) return String(value)
  return date.toLocaleString()
}

function formatBytes(size: number): string {
  if (!Number.isFinite(size) || size <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let value = size
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }
  return `${value.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`
}

function errorMessage(error: unknown): string {
  if (error instanceof Error && error.message) return error.message
  if (typeof error === 'string') return error
  return String(error)
}
</script>
