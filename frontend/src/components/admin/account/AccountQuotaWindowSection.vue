<template>
  <section class="space-y-3" data-testid="quota-window-section">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <div>
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.accounts.quotaWindows.title') }}
        </h3>
        <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.quotaWindows.localScope') }}
        </p>
      </div>
      <LoadingSpinner v-if="loading" size="sm" />
    </div>

    <div
      v-if="error"
      class="flex items-center justify-between gap-3 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-800/60 dark:bg-red-950/20 dark:text-red-300"
      data-testid="quota-window-error"
    >
      <span>{{ t('admin.accounts.quotaWindows.loadFailed') }}</span>
      <button
        type="button"
        class="inline-flex shrink-0 items-center gap-1 rounded-lg px-2 py-1 text-xs font-medium hover:bg-red-100 dark:hover:bg-red-900/30"
        @click="emit('retry')"
      >
        <Icon name="refresh" size="xs" />
        {{ t('common.retry') }}
      </button>
    </div>

    <div
      v-if="!loading && windows.length === 0"
      class="rounded-lg border border-dashed border-gray-300 px-4 py-8 text-center text-sm text-gray-500 dark:border-dark-500 dark:text-gray-400"
      data-testid="quota-window-empty"
    >
      {{ t('admin.accounts.quotaWindows.noSupportedWindows') }}
    </div>

    <template v-if="loading && windows.length === 0">
      <article
        v-for="skeletonIndex in 2"
        :key="skeletonIndex"
        class="card overflow-hidden p-0 motion-safe:animate-pulse"
        data-testid="quota-window-skeleton"
        aria-hidden="true"
      >
        <header class="px-4 py-3">
          <div class="flex items-center justify-between gap-3">
            <div class="flex min-w-0 items-center gap-2.5">
              <div class="h-8 w-8 shrink-0 rounded-lg bg-gray-200 dark:bg-dark-500" />
              <div class="min-w-0 space-y-2">
                <div class="h-3.5 w-28 max-w-full rounded bg-gray-200 dark:bg-dark-500" />
                <div class="h-2.5 w-40 max-w-full rounded bg-gray-100 dark:bg-dark-600" />
              </div>
            </div>
            <div class="h-5 w-16 shrink-0 rounded bg-gray-200 dark:bg-dark-500" />
          </div>
          <div class="mt-3 h-1.5 rounded-full bg-gray-200 dark:bg-dark-500" />
        </header>
        <div class="grid grid-cols-1 border-t border-gray-200 md:grid-cols-3 dark:border-dark-600">
          <div
            v-for="columnIndex in 3"
            :key="columnIndex"
            class="min-h-44 space-y-4 p-4"
            :class="columnIndex > 1 ? 'border-t border-gray-200 md:border-l md:border-t-0 dark:border-dark-600' : ''"
            data-testid="quota-window-skeleton-column"
          >
            <div class="space-y-2">
              <div class="h-3.5 w-32 max-w-full rounded bg-gray-200 dark:bg-dark-500" />
              <div class="h-2.5 w-40 max-w-full rounded bg-gray-100 dark:bg-dark-600" />
            </div>
            <div v-for="metricIndex in 4" :key="metricIndex" class="flex justify-between gap-4">
              <div class="h-2.5 w-16 rounded bg-gray-100 dark:bg-dark-600" />
              <div class="h-3 w-14 rounded bg-gray-200 dark:bg-dark-500" />
            </div>
          </div>
        </div>
      </article>
    </template>

    <article
      v-for="window in windows"
      :key="window.key"
      class="card overflow-hidden p-0"
      :data-window-key="window.key"
    >
      <header class="px-4 py-3">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div class="flex min-w-0 items-center gap-2.5">
            <div class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-blue-50 text-blue-600 dark:bg-blue-950/40 dark:text-blue-400">
              <Icon :name="window.key === 'thirty_day' ? 'calendar' : 'clock'" size="sm" />
            </div>
            <div class="min-w-0">
              <h4 class="truncate text-sm font-semibold text-gray-900 dark:text-white">
                {{ windowTitle(window.key) }}
              </h4>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                {{ resetText(window) }}
              </p>
            </div>
          </div>
          <div v-if="window.utilization !== null" class="shrink-0 text-right">
            <span class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.quotaWindows.used') }}
            </span>
            <span class="ml-1 text-lg font-semibold text-gray-900 dark:text-white">
              {{ formatPercent(window.utilization) }}
            </span>
          </div>
        </div>
        <div
          v-if="window.utilization !== null"
          class="mt-3 h-1.5 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-500"
          role="progressbar"
          :aria-label="`${windowTitle(window.key)} ${t('admin.accounts.quotaWindows.used')}`"
          aria-valuemin="0"
          aria-valuemax="100"
          :aria-valuenow="Math.min(100, Math.max(0, window.utilization))"
        >
          <div
            class="h-full rounded-full transition-[width] duration-300"
            :class="progressClass(window.utilization)"
            :style="{ width: `${Math.min(100, Math.max(0, window.utilization))}%` }"
          />
        </div>
      </header>

      <div
        v-if="window.boundaryStatus !== 'ready'"
        class="border-t border-gray-200 px-4 py-8 text-center dark:border-dark-600"
        data-testid="quota-window-boundary-error"
      >
        <Icon name="exclamationCircle" size="md" class="mx-auto mb-2 text-amber-500" />
        <p class="text-sm text-gray-600 dark:text-gray-300">
          {{ boundaryStatusText(window.boundaryStatus) }}
        </p>
      </div>

      <div v-else class="grid grid-cols-1 border-t border-gray-200 md:grid-cols-3 dark:border-dark-600">
        <div
          v-for="(column, index) in columnsFor(window)"
          :key="column.kind"
          class="min-w-0 p-4"
          :class="[
            index > 0 ? 'border-t border-gray-200 md:border-l md:border-t-0 dark:border-dark-600' : '',
            column.kind === 'forecast' ? 'bg-blue-50/60 dark:bg-blue-950/20' : ''
          ]"
          :data-column="column.kind"
        >
          <div class="mb-3 min-h-10">
            <h5 class="text-sm font-semibold text-gray-800 dark:text-gray-100">
              {{ columnTitle(column.kind) }}
            </h5>
            <p class="mt-0.5 truncate text-xs text-gray-500 dark:text-gray-400" :title="column.range">
              {{ column.range }}
            </p>
          </div>

          <dl class="space-y-2.5">
            <div v-for="metric in metrics" :key="metric" class="flex min-h-5 items-start justify-between gap-3">
              <dt class="text-xs text-gray-500 dark:text-gray-400">{{ metricTitle(metric) }}</dt>
              <dd class="max-w-[60%] text-right text-sm font-semibold text-gray-900 dark:text-white">
                {{ metricValue(column, metric) }}
              </dd>
            </div>
          </dl>

          <p
            v-if="column.kind === 'forecast'"
            class="mt-3 border-t border-blue-200/70 pt-2 text-xs text-blue-700 dark:border-blue-800/60 dark:text-blue-300"
          >
            {{ forecastBasis(window) }}
          </p>
        </div>
      </div>
    </article>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { formatCurrency, formatDate, formatNumber } from '@/utils/format'
import type {
  AccountQuotaWindowKey,
  AccountWindowBoundaryStatus,
  AccountWindowUsageForecast,
  AccountWindowUsageItem
} from '@/types'
import type { AccountQuotaWindowModel } from '@/features/account-window-usage/accountWindowUsage'

type ColumnKind = 'previous' | 'current' | 'forecast'
type Metric = 'requests' | 'tokens' | 'cost' | 'successRate'
type Column = {
  kind: ColumnKind
  range: string
  item: AccountWindowUsageItem | AccountWindowUsageForecast | null
}

defineProps<{
  windows: AccountQuotaWindowModel[]
  loading: boolean
  error: string | null
}>()

const emit = defineEmits<{
  (e: 'retry'): void
}>()

const { t } = useI18n()
const metrics: Metric[] = ['requests', 'tokens', 'cost', 'successRate']

const windowTitle = (key: AccountQuotaWindowKey) => t(`admin.accounts.quotaWindows.window.${key}`)
const columnTitle = (kind: ColumnKind) => t(`admin.accounts.quotaWindows.column.${kind}`)
const metricTitle = (metric: Metric) => t(`admin.accounts.quotaWindows.metric.${metric}`)

const columnsFor = (window: AccountQuotaWindowModel): Column[] => [
  { kind: 'previous', range: formatRange(window.previousRange), item: window.previous },
  { kind: 'current', range: formatRange(window.currentRange), item: window.current },
  { kind: 'forecast', range: formatRange(window.currentRange), item: window.forecast }
]

const formatRange = (range: AccountQuotaWindowModel['currentRange']) => {
  if (!range) return '-'
  const options: Intl.DateTimeFormatOptions = {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false
  }
  return `${formatDate(range.startTime, options)} - ${formatDate(range.endTime, options)}`
}

const metricValue = (column: Column, metric: Metric) => {
  if (!column.item) return '-'
  if (metric === 'successRate') {
    if (column.kind === 'forecast') return '-'
    const item = column.item as AccountWindowUsageItem
    if (item.success_rate_status !== 'available' || item.success_rate === null) {
      return t(`admin.accounts.quotaWindows.successRateStatus.${item.success_rate_status}`)
    }
    return formatPercent(item.success_rate)
  }

  const item = column.item as AccountWindowUsageItem | AccountWindowUsageForecast
  if (metric === 'requests') return formatNumber(item.total_requests)
  if (metric === 'tokens') return formatNumber(item.total_tokens)
  return formatCurrency(item.account_cost)
}

const resetText = (window: AccountQuotaWindowModel) => {
  if (!window.resetAt) return t('admin.accounts.quotaWindows.resetUnknown')
  return t('admin.accounts.quotaWindows.resetsAt', {
    time: formatDate(window.resetAt, {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      hour12: false
    })
  })
}

const boundaryStatusText = (status: AccountWindowBoundaryStatus) =>
  t(`admin.accounts.quotaWindows.boundaryStatus.${status}`)

const forecastBasis = (window: AccountQuotaWindowModel) => {
  if (!window.forecast) return t('admin.accounts.quotaWindows.forecastUnavailable')
  return window.forecast.basis === 'quota'
    ? t('admin.accounts.quotaWindows.forecastByQuota')
    : t('admin.accounts.quotaWindows.forecastByPrevious')
}

const formatPercent = (value: number) => `${value.toFixed(value < 1 ? 2 : 1)}%`
const progressClass = (value: number) => {
  if (value >= 100) return 'bg-red-500'
  if (value >= 80) return 'bg-amber-500'
  return 'bg-emerald-500'
}
</script>
