<template>
  <div class="space-y-4">
    <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.accounts.quotaHistory.description') }}</p>
    <div class="flex items-center justify-between gap-3">
      <div class="flex gap-2" role="group" :aria-label="t('admin.accounts.quotaHistory.window')">
        <button
          v-for="window in windows"
          :key="window"
          type="button"
          :aria-pressed="selectedWindow === window"
          :data-test="`window-${window}`"
          class="rounded-lg px-4 py-2 text-sm font-medium"
          :class="selectedWindow === window ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300' : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'"
          @click="selectedWindow = window"
        >{{ window }}</button>
      </div>
      <button type="button" class="btn btn-secondary text-sm" :disabled="loading" @click="loadHistory">
        {{ t('common.refresh') }}
      </button>
    </div>

    <div v-if="loading" class="flex justify-center py-8" role="status">
      <LoadingSpinner />
      <span class="sr-only">{{ t('common.loading') }}</span>
    </div>
    <div v-else-if="failed" role="alert" class="rounded-lg bg-red-50 p-4 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">
      <p>{{ t('admin.accounts.quotaHistory.loadFailed') }}</p>
      <button type="button" class="mt-2 font-medium underline" data-test="retry" @click="loadHistory">{{ t('admin.accounts.quotaHistory.retry') }}</button>
    </div>
    <div v-else class="space-y-5">
      <section v-for="section in sections" :key="section.key" class="space-y-3" :data-test="section.key">
        <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ section.title }}</h4>
        <p v-if="!section.entries.length" class="rounded-lg border border-dashed border-gray-200 p-4 text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
          {{ t(section.key === 'current' ? 'admin.accounts.quotaHistory.noCurrent' : 'admin.accounts.quotaHistory.noHistory') }}
        </p>
        <article
          v-for="entry in section.entries"
          :key="`${entry.window_start}-${entry.window_end}`"
          class="rounded-xl border border-gray-200 p-4 dark:border-dark-600"
          data-test="history-entry"
        >
          <div class="flex flex-wrap items-center justify-between gap-2 text-sm">
            <p class="font-medium text-gray-900 dark:text-gray-100">
              {{ formatTime(entry.window_start) }} – {{ formatTime(entry.window_end) }}
            </p>
            <span class="rounded bg-gray-100 px-2 py-1 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300">
              {{ endReason(entry) }}
            </span>
          </div>
          <dl class="mt-4 grid grid-cols-2 gap-4 lg:grid-cols-4">
            <div>
              <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.quotaHistory.referenceCost') }}</dt>
              <dd class="mt-1 font-semibold text-gray-900 dark:text-white" data-test="reference-cost">{{ money(entry.api_reference_cost) }}</dd>
            </div>
            <div>
              <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.quotaHistory.lastPercent') }}</dt>
              <dd class="mt-1 font-semibold text-gray-900 dark:text-white">
                {{ percent(entry.last_used_percent) }}
                <span v-if="entry.last_used_percent >= 100" class="block text-xs font-normal text-amber-600 dark:text-amber-400">{{ t('admin.accounts.quotaHistory.observedExhausted') }}</span>
              </dd>
            </div>
            <div>
              <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.quotaHistory.estimatedLimit') }}</dt>
              <dd class="mt-1 font-semibold text-gray-900 dark:text-white" data-test="estimated-limit">
                {{ entry.estimated_reference_limit != null ? t('admin.accounts.quotaHistory.approximate', { amount: money(entry.estimated_reference_limit) }) : t('admin.accounts.quotaHistory.unavailable') }}
              </dd>
            </div>
            <div>
              <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.quotaHistory.requests') }}</dt>
              <dd class="mt-1 font-semibold text-gray-900 dark:text-white">{{ entry.requests.toLocaleString() }}</dd>
            </div>
          </dl>
          <p class="mt-3 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.quotaHistory.observation', { first: formatTime(entry.first_observed_at), last: formatTime(entry.last_sample_at), count: entry.sample_count }) }}
          </p>
          <p v-if="entry.estimated_reference_limit != null" class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.quotaHistory.estimateBasis', { amount: money(entry.estimate_reference_cost), percent: percent(entry.estimate_used_percent), time: formatTime(entry.estimate_observed_at) }) }}
          </p>
          <ul v-if="entry.quality_flags.length" class="mt-2 space-y-1 text-xs text-amber-700 dark:text-amber-400">
            <li v-for="flag in entry.quality_flags" :key="flag">{{ qualityLabel(flag) }}</li>
          </ul>
        </article>
        <button
          v-if="section.key === 'history' && completedEntries.length > visibleHistoryCount"
          type="button"
          class="btn btn-secondary w-full text-sm"
          data-test="show-more"
          @click="visibleHistoryCount += 10"
        >{{ t('admin.accounts.quotaHistory.showMore') }}</button>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { formatDateTimeToMinute } from '@/utils/format'
import type { AccountQuotaWindow, AccountWindowHistoryResponse, AccountWindowUsageEntry } from '@/types'

const props = defineProps<{ accountId: number }>()
const { t } = useI18n()
const windows: AccountQuotaWindow[] = ['5h', '7d']
const selectedWindow = ref<AccountQuotaWindow>('5h')
const visibleHistoryCount = ref(10)
const history = ref<AccountWindowHistoryResponse | null>(null)
const loading = ref(false)
const failed = ref(false)
let requestID = 0
let controller: AbortController | undefined

const entries = computed(() => [...(history.value?.windows[selectedWindow.value] ?? [])]
  .sort((left, right) => Date.parse(right.window_end) - Date.parse(left.window_end)))
const completedEntries = computed(() => entries.value.filter(entry => entry.finalized))
const sections = computed(() => [
  { key: 'current', title: t('admin.accounts.quotaHistory.current'), entries: entries.value.filter(entry => !entry.finalized).slice(0, 1) },
  { key: 'history', title: t('admin.accounts.quotaHistory.recent'), entries: completedEntries.value.slice(0, visibleHistoryCount.value) }
])

async function loadHistory() {
  visibleHistoryCount.value = 10
  const request = ++requestID
  controller?.abort()
  controller = new AbortController()
  loading.value = true
  failed.value = false
  history.value = null
  try {
    const result = await adminAPI.accounts.getWindowHistory(props.accountId, 90, controller.signal)
    if (request === requestID) history.value = result
  } catch {
    if (request === requestID) failed.value = true
  } finally {
    if (request === requestID) loading.value = false
  }
}

watch(selectedWindow, () => { visibleHistoryCount.value = 10 })

watch(() => props.accountId, () => {
  selectedWindow.value = '5h'
  void loadHistory()
}, { immediate: true })

onBeforeUnmount(() => {
  ++requestID
  controller?.abort()
})

const formatTime = (value: string | null) => value ? formatDateTimeToMinute(value) : '—'
const money = (value: number | null) => value != null && Number.isFinite(value) ? `$${value.toFixed(2)}` : '—'
const percent = (value: number | null) => value != null && Number.isFinite(value) ? `${Number(value.toFixed(1))}%` : '—'

function endReason(entry: AccountWindowUsageEntry): string {
  if (!entry.finalized) return t('admin.accounts.quotaHistory.inProgress')
  if (entry.end_reason === 'early_reset') return t('admin.accounts.quotaHistory.earlyReset')
  if (entry.end_reason === 'window_changed') return t('admin.accounts.quotaHistory.windowChanged')
  if (entry.end_reason === 'expired') return t('admin.accounts.quotaHistory.expired')
  if (entry.end_reason === 'reset_observed') return t('admin.accounts.quotaHistory.resetObserved')
  return t('admin.accounts.quotaHistory.ended')
}

function qualityLabel(flag: string): string {
  switch (flag) {
    case 'partial_start': return t('admin.accounts.quotaHistory.partialStart')
    case 'missing_pricing': return t('admin.accounts.quotaHistory.missingPricing')
    case 'low_utilization': return t('admin.accounts.quotaHistory.lowUtilization')
    case 'observation_gap': return t('admin.accounts.quotaHistory.observationGap')
    case 'pending_usage': return t('admin.accounts.quotaHistory.pendingUsage')
    case 'external_usage_unknown': return t('admin.accounts.quotaHistory.externalUsage')
    case 'ambiguous_reset': return t('admin.accounts.quotaHistory.ambiguousReset')
    default: return t('admin.accounts.quotaHistory.incompleteData')
  }
}
</script>
