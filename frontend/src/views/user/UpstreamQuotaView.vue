<template>
  <AppLayout>
    <div class="space-y-5">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('upstreamQuota.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('upstreamQuota.description') }}</p>
        </div>
        <button v-if="enabled" type="button" class="btn btn-secondary" data-action="refresh" :disabled="loading" @click="load()">
          {{ loading ? t('upstreamQuota.loading') : t('upstreamQuota.refresh') }}
        </button>
      </div>
      <p v-if="!checking && !enabled && !error" class="text-sm text-gray-500">{{ t('upstreamQuota.disabledPage') }}</p>
      <template v-if="enabled">
        <div class="flex gap-1 border-b border-gray-200 dark:border-dark-700" role="tablist" :aria-label="t('upstreamQuota.platforms')">
          <button v-for="tab in platforms" :id="`quota-tab-${tab.key}`" :key="tab.key" type="button" role="tab" :aria-selected="platform === tab.key" aria-controls="quota-panel" :data-platform="tab.key" class="border-b-2 px-4 py-2 text-sm font-medium transition-colors" :class="platform === tab.key ? 'border-primary-500 text-primary-600 dark:text-primary-400' : 'border-transparent text-gray-500 hover:text-gray-800 dark:text-gray-400'" @click="select(tab.key)">
            {{ tab.label }}
          </button>
        </div>
        <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('upstreamQuota.cacheNote') }}</p>
        <p v-if="platform === 'antigravity'" class="text-xs text-gray-500 dark:text-gray-400">{{ t('upstreamQuota.modelNote') }}</p>
        <div id="quota-panel" role="tabpanel" :aria-labelledby="`quota-tab-${platform}`" :aria-busy="loading">
          <p v-if="loading" role="status" class="py-8 text-center text-gray-500">{{ t('upstreamQuota.loading') }}</p>
          <p v-else-if="error" role="alert" class="rounded-lg bg-red-50 p-4 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">{{ t('upstreamQuota.loadError') }}</p>
          <p v-else-if="!accounts.length" class="py-8 text-center text-gray-500">{{ t('upstreamQuota.empty') }}</p>
          <div v-else class="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <article v-for="account in accounts" :key="account.id" class="card space-y-3 p-4">
              <div class="flex items-center justify-between gap-2">
                <h2 class="font-medium text-gray-900 dark:text-white">{{ account.alias }}</h2>
                <span class="rounded-full px-2 py-0.5 text-xs" :class="statusClass(account.status)">{{ t(`upstreamQuota.status.${account.status}`) }}</span>
              </div>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('upstreamQuota.concurrency') }}: {{ account.current_concurrency ?? '—' }} / {{ account.max_concurrency }}
              </p>
              <template v-if="account.quota_state === 'known' && account.windows.length">
                <div v-for="window in account.windows" :key="`${window.key}-${window.variant ?? 0}`" class="space-y-1">
                  <div class="flex flex-wrap justify-between gap-x-2 text-xs text-gray-600 dark:text-gray-300">
                    <span>{{ t(`upstreamQuota.windows.${window.key}`) }}<span v-if="window.variant"> #{{ window.variant }}</span><span v-if="window.resets_at" class="text-gray-400"> ({{ t('upstreamQuota.resets') }} {{ formatTime(window.resets_at) }})</span></span>
                    <span>{{ t('upstreamQuota.remaining') }} {{ Math.max(0, 100 - window.used_percent).toFixed(1) }}%</span>
                  </div>
                  <div class="h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700" role="progressbar" :aria-label="t(`upstreamQuota.windows.${window.key}`)" :aria-valuenow="window.used_percent" :aria-valuemin="0" :aria-valuemax="100">
                    <div class="h-full rounded-full" :class="barClass(window.used_percent)" :style="{ width: `${window.used_percent}%` }" />
                  </div>
                </div>
              </template>
              <div v-else class="space-y-2">
                <p class="text-xs text-gray-400">{{ t(account.quota_state === 'unavailable' ? 'upstreamQuota.unavailable' : 'upstreamQuota.unknown') }}</p>
                <div class="h-2 rounded-full bg-gray-200 dark:bg-dark-700" />
              </div>
              <p v-if="account.updated_at" class="text-xs text-gray-400">{{ t('upstreamQuota.sampleTime') }} {{ formatTime(account.updated_at) }}</p>
            </article>
          </div>
        </div>
      </template>
      <p v-else-if="checking" role="status" class="py-8 text-center text-gray-500">{{ t('upstreamQuota.loading') }}</p>
      <p v-else-if="error" role="alert" class="text-sm text-red-600">{{ t('upstreamQuota.loadError') }}</p>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import { upstreamQuotaAPI, type QuotaPlatform, type SharedQuotaAccount } from '@/api/upstreamQuota'

const { t, locale } = useI18n()
const platforms: { key: QuotaPlatform; label: string }[] = [
  { key: 'openai', label: 'GPT' }, { key: 'antigravity', label: 'Antigravity' }, { key: 'gemini', label: 'Gemini' }
]
const enabled = ref(false)
const checking = ref(true)
const platform = ref<QuotaPlatform>('openai')
const loading = ref(false)
const error = ref(false)
const accounts = ref<SharedQuotaAccount[]>([])
let requestVersion = 0
let disposed = false
async function load() {
  const version = ++requestVersion
  loading.value = true
  error.value = false
  accounts.value = []
  try {
    const data = await upstreamQuotaAPI.snapshot(platform.value)
    if (!disposed && version === requestVersion) accounts.value = data.accounts
  } catch {
    if (!disposed && version === requestVersion) error.value = true
  } finally {
    if (!disposed && version === requestVersion) loading.value = false
  }
}
function select(value: QuotaPlatform) {
  if (platform.value === value) return
  platform.value = value
  void load()
}
function formatTime(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString(locale?.value)
}
function barClass(used: number) {
  return used >= 90 ? 'bg-red-500' : used >= 75 ? 'bg-amber-500' : 'bg-green-500'
}
function statusClass(status: SharedQuotaAccount['status']) {
  if (status === 'available') return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300'
  if (status === 'rate_limited') return 'bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300'
  return 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400'
}
onMounted(async () => {
  try {
    const config = await upstreamQuotaAPI.config()
    if (disposed) return
    enabled.value = config.enabled
    if (enabled.value) await load()
  } catch { if (!disposed) error.value = true }
  finally { if (!disposed) checking.value = false }
})
onBeforeUnmount(() => { disposed = true; requestVersion++ })
</script>
