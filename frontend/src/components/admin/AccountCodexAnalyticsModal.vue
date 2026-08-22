<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.codexAnalytics.title')"
    width="extra-wide"
    @close="handleClose"
  >
    <div :lang="locale" class="min-h-[22rem] tabular-nums">
      <div v-if="loading" class="flex min-h-[22rem] flex-col items-center justify-center gap-3" role="status">
        <LoadingSpinner />
        <p class="text-sm text-gray-500 dark:text-dark-400">
          {{ t('admin.accounts.codexAnalytics.loading') }}
        </p>
      </div>

      <section
        v-else-if="errorKind"
        class="flex min-h-[22rem] flex-col items-center justify-center px-4 text-center"
        role="alert"
      >
        <div
          :class="[
            'mb-4 flex h-11 w-11 items-center justify-center rounded-xl',
            errorKind === 'unauthorized'
              ? 'bg-amber-100 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300'
              : errorKind === 'rate-limited'
                ? 'bg-orange-100 text-orange-700 dark:bg-orange-500/15 dark:text-orange-300'
                : 'bg-red-100 text-red-700 dark:bg-red-500/15 dark:text-red-300'
          ]"
        >
          <Icon :name="errorKind === 'unauthorized' ? 'link' : 'exclamationTriangle'" size="lg" />
        </div>
        <h3 class="text-base font-semibold text-gray-900 dark:text-gray-100">
          {{ t(`admin.accounts.codexAnalytics.errors.${errorKind}.title`) }}
        </h3>
        <p class="mt-2 max-w-lg text-sm leading-6 text-gray-600 dark:text-dark-300">
          {{ t(`admin.accounts.codexAnalytics.errors.${errorKind}.description`) }}
        </p>
        <p v-if="errorMessage" class="mt-2 max-w-lg break-words text-xs text-gray-500 dark:text-dark-400">
          {{ errorMessage }}
        </p>
        <button type="button" class="btn btn-secondary mt-5 min-h-10 active:scale-95" @click="fetchAnalytics">
          <Icon name="refresh" size="sm" />
          {{ t('admin.accounts.codexAnalytics.retry') }}
        </button>
      </section>

      <template v-else-if="analytics">
        <header class="mb-4 flex flex-col gap-3 border-b border-gray-200 pb-4 dark:border-dark-700 sm:flex-row sm:items-start sm:justify-between">
          <div class="min-w-0">
            <div class="flex min-w-0 items-center gap-2">
              <span class="h-2 w-2 flex-none rounded-full bg-emerald-500"></span>
              <h3 class="truncate text-sm font-semibold text-gray-900 dark:text-gray-100">
                {{ account?.name }}
              </h3>
            </div>
            <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-dark-400">
              {{ t('admin.accounts.codexAnalytics.scopeDescription') }}
            </p>
          </div>
          <div class="grid w-full min-w-0 gap-2 sm:w-auto sm:max-w-sm sm:justify-items-end">
            <label for="codex-analytics-period" class="sr-only">
              {{ t('admin.accounts.codexAnalytics.periodSelector.label') }}
            </label>
            <Select
              id="codex-analytics-period"
              v-model="selectedPeriod"
              :options="periodOptions"
              :aria-label="t('admin.accounts.codexAnalytics.periodSelector.label')"
              class="w-full sm:w-56"
              @change="handlePeriodChange"
            />
            <div class="flex min-w-0 max-w-full flex-wrap items-center gap-x-2 gap-y-1 text-[11px] leading-5 text-gray-500 dark:text-dark-400 sm:justify-end">
              <span class="max-w-full break-words rounded-md bg-gray-100 px-2 py-1 dark:bg-dark-800">
                {{ t('admin.accounts.codexAnalytics.periodRange', { start: formatPeriodDate(analytics.period_start), end: formatPeriodDate(analytics.period_end) }) }}
              </span>
              <span class="max-w-full break-words rounded-md bg-gray-100 px-2 py-1 dark:bg-dark-800">
                {{ analytics.cache.hit ? t('admin.accounts.codexAnalytics.cacheExpires', { time: formatDateTime(analytics.cache.expires_at) }) : t('admin.accounts.codexAnalytics.fresh') }}
              </span>
              <span class="max-w-full break-words">{{ formatDateTime(analytics.fetched_at) }}</span>
            </div>
          </div>
        </header>

        <div v-if="warnings.length" class="mb-4 space-y-2" role="status">
          <div
            v-for="warning in warnings"
            :key="`${warning.code}:${warning.message}`"
            class="flex gap-2 rounded-lg bg-amber-50 px-3 py-2.5 text-xs leading-5 text-amber-900 ring-1 ring-inset ring-amber-200 dark:bg-amber-500/10 dark:text-amber-200 dark:ring-amber-500/20"
          >
            <Icon name="exclamationTriangle" size="sm" class="mt-0.5 flex-none" />
            <div>
              <span class="font-semibold">{{ isLocalFallback ? t('admin.accounts.codexAnalytics.localFallback') : t('admin.accounts.codexAnalytics.warning') }}</span>
              <span class="ml-1">{{ warningMessage(warning) }}</span>
            </div>
          </div>
        </div>

        <section v-if="isEmpty" class="flex min-h-[18rem] flex-col items-center justify-center px-4 text-center">
          <Icon name="chartBar" size="xl" class="text-gray-300 dark:text-dark-600" />
          <h3 class="mt-3 text-sm font-semibold text-gray-900 dark:text-gray-100">
            {{ t('admin.accounts.codexAnalytics.empty.title') }}
          </h3>
          <p class="mt-1 max-w-md text-sm leading-6 text-gray-500 dark:text-dark-400">
            {{ t('admin.accounts.codexAnalytics.empty.description') }}
          </p>
          <button type="button" class="btn btn-secondary mt-4 min-h-10 active:scale-95" @click="fetchAnalytics">
            <Icon name="refresh" size="sm" />
            {{ t('admin.accounts.codexAnalytics.retry') }}
          </button>
        </section>

        <div v-else class="space-y-4">
          <section aria-labelledby="codex-kpis-title">
            <div class="mb-2 flex items-end justify-between gap-3">
              <div>
                <h3 id="codex-kpis-title" class="text-sm font-semibold text-gray-900 dark:text-gray-100">
                  {{ t('admin.accounts.codexAnalytics.overview') }}
                </h3>
                <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">
                  {{ t('admin.accounts.codexAnalytics.officialVsManaged') }}
                </p>
              </div>
            </div>
            <div class="grid grid-cols-2 overflow-hidden rounded-xl ring-1 ring-gray-200 dark:ring-dark-700 lg:grid-cols-4">
              <div v-for="kpi in primaryKpis" :key="kpi.label" class="min-w-0 bg-white p-3 odd:bg-gray-50/70 dark:bg-dark-900 dark:odd:bg-dark-800/50 sm:p-3.5">
                <p class="min-h-10 break-words text-[11px] font-medium leading-4 text-gray-500 dark:text-dark-400 lg:min-h-0" :title="kpi.label">{{ kpi.label }}</p>
                <p class="mt-1 whitespace-nowrap text-base font-semibold leading-6 tracking-tight text-gray-900 dark:text-gray-100 sm:text-xl" :title="kpi.fullValue">{{ kpi.value }}</p>
                <p class="mt-1 min-h-8 break-words text-[11px] leading-4 text-gray-400 dark:text-dark-500 lg:min-h-0">{{ kpi.source }}</p>
              </div>
            </div>
            <dl class="mt-2 grid grid-cols-2 gap-x-4 gap-y-2 rounded-lg bg-gray-50 px-3 py-2.5 text-xs dark:bg-dark-800/60 sm:grid-cols-3 lg:grid-cols-7">
              <div v-for="metric in detailMetrics" :key="metric.label" class="min-h-14 min-w-0 lg:min-h-0">
                <dt class="break-words leading-4 text-gray-500 dark:text-dark-400">{{ metric.label }}</dt>
                <dd class="mt-1 whitespace-nowrap text-left font-semibold leading-4 text-gray-800 dark:text-gray-200">{{ metric.value }}</dd>
              </div>
            </dl>
          </section>

          <section aria-labelledby="codex-limits-title">
            <h3 id="codex-limits-title" class="mb-2 text-sm font-semibold text-gray-900 dark:text-gray-100">
              {{ t('admin.accounts.codexAnalytics.rateLimits') }}
            </h3>
            <div class="grid gap-2 sm:grid-cols-2">
              <div v-for="window in rateLimitWindows" :key="window.key" class="rounded-xl bg-gray-50 p-3.5 ring-1 ring-inset ring-gray-200 dark:bg-dark-800/60 dark:ring-dark-700">
                <div class="flex items-center justify-between gap-3">
                  <span class="text-xs font-semibold text-gray-700 dark:text-gray-200">{{ window.label }}</span>
                  <span :class="window.statusClass" class="rounded-full px-2 py-0.5 text-[11px] font-semibold">{{ window.status }}</span>
                </div>
                <div class="mt-3 h-1.5 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
                  <div :class="window.barClass" class="h-full rounded-full" :style="{ width: `${window.percent}%` }"></div>
                </div>
                <div class="mt-2 flex items-center justify-between gap-3 text-xs">
                  <span class="font-semibold text-gray-900 dark:text-gray-100">{{ formatPercent(window.percent) }}</span>
                  <span class="text-right text-gray-500 dark:text-dark-400">
                    {{ window.resetText }}
                  </span>
                </div>
              </div>
            </div>
          </section>

          <section v-if="profileMetrics.length" aria-labelledby="codex-profile-title">
            <h3 id="codex-profile-title" class="mb-2 text-sm font-semibold text-gray-900 dark:text-gray-100">
              {{ t('admin.accounts.codexAnalytics.profile.title') }}
            </h3>
            <dl class="grid grid-cols-2 gap-px overflow-hidden rounded-xl bg-gray-200 ring-1 ring-gray-200 dark:bg-dark-700 dark:ring-dark-700 sm:grid-cols-3 lg:grid-cols-5">
              <div v-for="metric in profileMetrics" :key="metric.label" class="bg-white px-3 py-3 dark:bg-dark-900">
                <dt class="text-[11px] leading-4 text-gray-500 dark:text-dark-400">{{ metric.label }}</dt>
                <dd class="mt-1 text-sm font-semibold text-gray-900 dark:text-gray-100">{{ metric.value }}</dd>
              </div>
            </dl>
          </section>

          <div class="grid gap-4 lg:grid-cols-[minmax(0,1.7fr)_minmax(16rem,0.8fr)]">
            <section class="min-w-0 rounded-xl bg-white p-3.5 ring-1 ring-gray-200 dark:bg-dark-900 dark:ring-dark-700" aria-labelledby="codex-trend-title">
              <div class="mb-3">
                <h3 id="codex-trend-title" class="text-sm font-semibold text-gray-900 dark:text-gray-100">
                  {{ t('admin.accounts.codexAnalytics.trend.title') }}
                </h3>
                <p class="mt-0.5 text-[11px] text-gray-500 dark:text-dark-400">
                  {{ t('admin.accounts.codexAnalytics.trend.description') }}
                </p>
              </div>
              <div class="h-64 sm:h-72">
                <Bar v-if="trendChartData" :data="trendChartData" :options="barOptions" />
                <div v-else class="flex h-full items-center justify-center text-sm text-gray-500 dark:text-dark-400">
                  {{ t('admin.accounts.codexAnalytics.noManagedTraffic') }}
                </div>
              </div>
            </section>

            <section class="min-w-0 rounded-xl bg-white p-3.5 ring-1 ring-gray-200 dark:bg-dark-900 dark:ring-dark-700" aria-labelledby="codex-models-title">
              <div class="mb-3">
                <h3 id="codex-models-title" class="text-sm font-semibold text-gray-900 dark:text-gray-100">
                  {{ t('admin.accounts.codexAnalytics.models.title') }}
                </h3>
                <p class="mt-0.5 text-[11px] text-gray-500 dark:text-dark-400">
                  {{ t('admin.accounts.codexAnalytics.models.description') }}
                </p>
              </div>
              <div v-if="modelChartData" class="h-52">
                <Doughnut :data="modelChartData" :options="doughnutOptions" />
              </div>
              <div v-else class="flex h-52 items-center justify-center text-sm text-gray-500 dark:text-dark-400">
                {{ t('admin.accounts.codexAnalytics.noManagedTraffic') }}
              </div>
              <ol v-if="modelRows.length" class="mt-2 divide-y divide-gray-100 text-xs dark:divide-dark-700">
                <li v-for="model in modelRows" :key="model.label" class="flex items-center justify-between gap-3 py-2">
                  <span class="min-w-0 truncate text-gray-600 dark:text-dark-300" :title="model.label">{{ model.label }}</span>
                  <span class="flex-none font-semibold text-gray-900 dark:text-gray-100">{{ formatTokens(model.value) }}</span>
                </li>
              </ol>
            </section>
          </div>
        </div>
      </template>
    </div>

    <template #footer>
      <div class="flex justify-end">
        <button type="button" class="btn btn-secondary min-h-10 active:scale-95" @click="handleClose">
          {{ t('common.close') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  ArcElement,
  BarElement,
  CategoryScale,
  Chart as ChartJS,
  Legend,
  LinearScale,
  Tooltip
} from 'chart.js'
import { Bar, Doughnut } from 'vue-chartjs'
import BaseDialog from '@/components/common/BaseDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import {
  DEFAULT_CODEX_ANALYTICS_PERIOD_QUERY,
  type CodexAnalyticsPeriodQuery,
  type CodexAnalyticsRateLimitWindow,
  type CodexAnalyticsResponse
} from '@/api/admin/accounts'
import type { Account } from '@/types'
import { extractApiErrorMessage } from '@/utils/apiError'

ChartJS.register(CategoryScale, LinearScale, BarElement, ArcElement, Tooltip, Legend)

const props = defineProps<{
  show: boolean
  account: Account | null
}>()

const emit = defineEmits<{
  (event: 'close'): void
}>()

const { t, locale } = useI18n()
type AnalyticsPeriodSelection = 'current_7d' | 'recent_7' | 'recent_14' | 'recent_30'

const PERIOD_QUERIES: Record<AnalyticsPeriodSelection, CodexAnalyticsPeriodQuery> = {
  current_7d: DEFAULT_CODEX_ANALYTICS_PERIOD_QUERY,
  recent_7: { period: 'recent', days: 7 },
  recent_14: { period: 'recent', days: 14 },
  recent_30: { period: 'recent', days: 30 }
}

const selectedPeriod = ref<AnalyticsPeriodSelection>('current_7d')
const periodOptions = computed(() => [
  { value: 'current_7d', label: t('admin.accounts.codexAnalytics.periodSelector.currentCycle') },
  { value: 'recent_7', label: t('admin.accounts.codexAnalytics.periodSelector.recent7') },
  { value: 'recent_14', label: t('admin.accounts.codexAnalytics.periodSelector.recent14') },
  { value: 'recent_30', label: t('admin.accounts.codexAnalytics.periodSelector.recent30') }
])
const analytics = ref<CodexAnalyticsResponse | null>(null)
const loading = ref(false)
const errorKind = ref<'unauthorized' | 'rate-limited' | 'generic' | null>(null)
const errorMessage = ref('')
const now = ref(Date.now())
const isDark = ref(false)
let countdownTimer: ReturnType<typeof setInterval> | null = null
let themeObserver: MutationObserver | null = null
let requestController: AbortController | null = null
let requestVersion = 0

const warnings = computed(() => analytics.value?.warnings ?? [])
const warningTranslationKeys: Readonly<Record<string, string>> = {
  current_7d_window_unavailable: 'admin.accounts.codexAnalytics.warnings.current_7d_window_unavailable',
  official_daily_buckets_approximate_period: 'admin.accounts.codexAnalytics.warnings.official_daily_buckets_approximate_period',
  rate_limits_unavailable: 'admin.accounts.codexAnalytics.warnings.rate_limits_unavailable',
  official_profile_unavailable: 'admin.accounts.codexAnalytics.warnings.official_profile_unavailable',
  cache_read_failed: 'admin.accounts.codexAnalytics.warnings.cache_read_failed',
  cache_write_failed: 'admin.accounts.codexAnalytics.warnings.cache_write_failed'
}
const isLocalFallback = computed(() => warnings.value.length > 0 && analytics.value?.summary.official_total_tokens == null)
const isEmpty = computed(() => {
  const value = analytics.value
  if (!value) return true

  const summary = value.summary
  const hasSummaryActivity = hasNonzeroValue(
    summary.official_total_tokens,
    summary.managed_total_tokens,
    summary.input_tokens,
    summary.output_tokens,
    summary.cache_tokens,
    summary.cache_read_tokens,
    summary.cache_hit_rate,
    summary.estimated_cost,
    summary.requests
  )
  const hasModelActivity = value.models.some((model) => hasNonzeroValue(
    model.input_tokens,
    model.output_tokens,
    model.cache_tokens,
    model.total_tokens,
    model.requests,
    model.estimated_cost
  ))
  const hasTimeSeriesActivity = value.time_series.some((row) => hasNonzeroValue(
    row.official_total_tokens,
    row.input_tokens,
    row.output_tokens,
    row.cache_tokens,
    row.total_tokens,
    row.requests,
    row.estimated_cost
  ))

  return !hasSummaryActivity && !hasModelActivity && !hasTimeSeriesActivity
})

function hasNonzeroValue(...values: Array<number | null | undefined>): boolean {
  return values.some((value) => value != null && Number.isFinite(value) && value !== 0)
}

function warningMessage(warning: { code: string; message: string }): string {
  const translationKey = warningTranslationKeys[warning.code]
  return translationKey ? t(translationKey) : warning.message
}

const primaryKpis = computed(() => {
  const value = analytics.value
  if (!value) return []
  const summary = value.summary
  const resetCountdown = formatPreferredResetCountdown(value)
  return [
    {
      label: t('admin.accounts.codexAnalytics.kpis.officialTokens'),
      value: formatOptionalTokens(summary.official_total_tokens),
      fullValue: formatOptionalNumber(summary.official_total_tokens),
      source: t('admin.accounts.codexAnalytics.sources.openai')
    },
    {
      label: t('admin.accounts.codexAnalytics.kpis.cacheHitRate'),
      value: formatPercent(summary.cache_hit_rate),
      fullValue: formatPercent(summary.cache_hit_rate),
      source: t('admin.accounts.codexAnalytics.sources.sub2api')
    },
    {
      label: t('admin.accounts.codexAnalytics.kpis.estimatedCost'),
      value: formatCurrency(summary.estimated_cost),
      fullValue: formatCurrency(summary.estimated_cost),
      source: t('admin.accounts.codexAnalytics.sources.sub2api')
    },
    {
      label: t('admin.accounts.codexAnalytics.kpis.resetCountdown'),
      value: resetCountdown,
      fullValue: resetCountdown,
      source: t('admin.accounts.codexAnalytics.sources.openai')
    }
  ]
})

const detailMetrics = computed(() => {
  const summary = analytics.value?.summary
  if (!summary) return []
  return [
    { label: t('admin.accounts.codexAnalytics.kpis.managedTokens'), value: formatTokens(summary.managed_total_tokens) },
    { label: t('admin.accounts.codexAnalytics.kpis.requests'), value: formatNumber(summary.requests) },
    { label: t('admin.accounts.codexAnalytics.kpis.currentLimit'), value: formatPercent(summary.current_limit_used_percent) },
    { label: t('admin.accounts.codexAnalytics.kpis.input'), value: formatTokens(summary.input_tokens) },
    { label: t('admin.accounts.codexAnalytics.kpis.output'), value: formatTokens(summary.output_tokens) },
    { label: t('admin.accounts.codexAnalytics.kpis.cache'), value: formatTokens(summary.cache_tokens) },
    { label: t('admin.accounts.codexAnalytics.kpis.cacheRead'), value: formatTokens(summary.cache_read_tokens) }
  ]
})

const profileMetrics = computed(() => {
  const profile = analytics.value?.profile
  if (!profile) return []
  const rows = [
    [t('admin.accounts.codexAnalytics.profile.lifetimeTokens'), formatOptionalTokens(profile.lifetime_tokens), profile.lifetime_tokens],
    [t('admin.accounts.codexAnalytics.profile.peakDailyTokens'), formatOptionalTokens(profile.peak_daily_tokens), profile.peak_daily_tokens],
    [t('admin.accounts.codexAnalytics.profile.longestTurn'), formatOptionalDuration(profile.longest_running_turn_seconds), profile.longest_running_turn_seconds],
    [t('admin.accounts.codexAnalytics.profile.currentStreak'), formatOptionalDays(profile.current_streak_days), profile.current_streak_days],
    [t('admin.accounts.codexAnalytics.profile.longestStreak'), formatOptionalDays(profile.longest_streak_days), profile.longest_streak_days]
  ] as const
  return rows
    .filter((row) => row[2] != null)
    .map((row) => ({ label: row[0], value: row[1] }))
})

const rateLimitWindows = computed(() => {
  const limits = analytics.value?.rate_limits
  return [
    buildRateWindow('five-hour', t('admin.accounts.codexAnalytics.windows.fiveHour'), limits?.five_hour),
    buildRateWindow('seven-day', t('admin.accounts.codexAnalytics.windows.sevenDay'), limits?.seven_day)
  ]
})

const chartPalette = computed(() => ({
  text: isDark.value ? '#d1d5db' : '#4b5563',
  grid: isDark.value ? 'rgba(107, 114, 128, 0.22)' : 'rgba(107, 114, 128, 0.16)',
  input: '#3b82f6',
  output: '#10b981',
  cache: '#f59e0b',
  official: '#f97316',
  models: ['#2563eb', '#10b981', '#f59e0b', '#94a3b8']
}))

const trendChartData = computed(() => {
  const rows = analytics.value?.time_series ?? []
  if (!rows.length) return null
  return {
    labels: rows.map((row) => row.date),
    datasets: [
      {
        label: t('admin.accounts.codexAnalytics.trend.input'),
        data: rows.map((row) => row.input_tokens),
        backgroundColor: chartPalette.value.input,
        stack: 'managed'
      },
      {
        label: t('admin.accounts.codexAnalytics.trend.output'),
        data: rows.map((row) => row.output_tokens),
        backgroundColor: chartPalette.value.output,
        stack: 'managed'
      },
      {
        label: t('admin.accounts.codexAnalytics.trend.cache'),
        data: rows.map((row) => row.cache_tokens),
        backgroundColor: chartPalette.value.cache,
        stack: 'managed'
      },
      {
        label: t('admin.accounts.codexAnalytics.trend.official'),
        data: rows.map((row) => row.official_total_tokens ?? null),
        backgroundColor: 'transparent',
        borderColor: chartPalette.value.official,
        borderWidth: 2,
        borderRadius: 3,
        borderSkipped: false,
        stack: 'official'
      }
    ]
  }
})

const barOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  interaction: { mode: 'index' as const, intersect: false },
  plugins: {
    legend: {
      position: 'bottom' as const,
      labels: { color: chartPalette.value.text, usePointStyle: true, pointStyle: 'circle', padding: 14, font: { size: 10 } }
    }
  },
  scales: {
    x: {
      stacked: true,
      grid: { display: false },
      ticks: { color: chartPalette.value.text, maxRotation: 0, autoSkip: true, font: { size: 10 } }
    },
    y: {
      stacked: true,
      beginAtZero: true,
      grid: { color: chartPalette.value.grid },
      ticks: { color: chartPalette.value.text, callback: (value: string | number) => formatTokens(Number(value)), font: { size: 10 } }
    },
  }
}))

const modelRows = computed(() => {
  const sorted = [...(analytics.value?.models ?? [])]
    .filter((model) => model.total_tokens > 0)
    .sort((left, right) => right.total_tokens - left.total_tokens)
  const top = sorted.slice(0, 3).map((model) => ({ label: model.model, value: model.total_tokens }))
  const remainder = sorted.slice(3).reduce((total, model) => total + model.total_tokens, 0)
  if (remainder > 0) top.push({ label: t('admin.accounts.codexAnalytics.models.other'), value: remainder })
  return top
})

const modelChartData = computed(() => {
  if (!modelRows.value.length) return null
  return {
    labels: modelRows.value.map((model) => model.label),
    datasets: [{
      data: modelRows.value.map((model) => model.value),
      backgroundColor: chartPalette.value.models.slice(0, modelRows.value.length),
      borderWidth: 0,
      hoverOffset: 3
    }]
  }
})

const doughnutOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  cutout: '68%',
  plugins: {
    legend: {
      display: false
    }
  }
}))

watch(
  () => [props.show, props.account?.id] as const,
  ([visible]) => {
    if (visible && props.account) {
      void fetchAnalytics()
      return
    }
    cleanupRequest()
    stopCountdown()
    analytics.value = null
    errorKind.value = null
    errorMessage.value = ''
    selectedPeriod.value = 'current_7d'
  },
  { immediate: true }
)

onMounted(() => {
  syncTheme()
  themeObserver = new MutationObserver(syncTheme)
  themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
})

onUnmounted(() => {
  cleanupRequest()
  stopCountdown()
  themeObserver?.disconnect()
  themeObserver = null
})

async function fetchAnalytics(): Promise<void> {
  if (!props.account || !props.show) return
  cleanupRequest()
  const version = ++requestVersion
  requestController = new AbortController()
  loading.value = true
  errorKind.value = null
  errorMessage.value = ''
  try {
    const response = await adminAPI.accounts.getCodexAnalytics(
      props.account.id,
      PERIOD_QUERIES[selectedPeriod.value],
      { signal: requestController.signal }
    )
    if (version !== requestVersion) return
    analytics.value = response
    startCountdown()
  } catch (error) {
    if (version !== requestVersion || requestController?.signal.aborted) return
    const status = extractStatus(error)
    errorKind.value = status === 401 ? 'unauthorized' : status === 429 ? 'rate-limited' : 'generic'
    errorMessage.value = extractApiErrorMessage(error, '')
    analytics.value = null
    stopCountdown()
  } finally {
    if (version === requestVersion) {
      loading.value = false
      requestController = null
    }
  }
}

function handlePeriodChange(): void {
  void fetchAnalytics()
}

function extractStatus(error: unknown): number | null {
  if (!error || typeof error !== 'object') return null
  const value = error as { status?: unknown; code?: unknown; response?: { status?: unknown } }
  const raw = value.status ?? value.response?.status ?? value.code
  const status = Number(raw)
  return Number.isFinite(status) ? status : null
}

function cleanupRequest(): void {
  requestVersion += 1
  requestController?.abort()
  requestController = null
  loading.value = false
}

function startCountdown(): void {
  stopCountdown()
  now.value = Date.now()
  countdownTimer = setInterval(() => {
    now.value = Date.now()
  }, 1000)
}

function stopCountdown(): void {
  if (countdownTimer) clearInterval(countdownTimer)
  countdownTimer = null
}

function syncTheme(): void {
  isDark.value = document.documentElement.classList.contains('dark')
}

function handleClose(): void {
  cleanupRequest()
  stopCountdown()
  analytics.value = null
  errorKind.value = null
  errorMessage.value = ''
  selectedPeriod.value = 'current_7d'
  emit('close')
}

function buildRateWindow(key: string, label: string, window?: CodexAnalyticsRateLimitWindow | null) {
  const percent = clampPercent(window?.used_percent)
  const seconds = secondsUntilReset(window)
  const unavailable = window == null
  const limited = window?.limit_reached === true || window?.allowed === false || percent >= 100
  const warning = !limited && percent >= 80
  return {
    key,
    label,
    percent,
    status: unavailable
      ? t('admin.accounts.codexAnalytics.windows.unavailable')
      : limited
        ? t('admin.accounts.codexAnalytics.windows.limited')
        : warning
          ? t('admin.accounts.codexAnalytics.windows.nearLimit')
          : t('admin.accounts.codexAnalytics.windows.available'),
    statusClass: unavailable
      ? 'bg-gray-200 text-gray-600 dark:bg-dark-700 dark:text-dark-300'
      : limited
        ? 'bg-red-100 text-red-700 dark:bg-red-500/15 dark:text-red-300'
        : warning
          ? 'bg-amber-100 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300'
          : 'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300',
    barClass: limited ? 'bg-red-500' : warning ? 'bg-amber-500' : unavailable ? 'bg-gray-300 dark:bg-dark-600' : 'bg-emerald-500',
    resetText: unavailable
      ? t('admin.accounts.codexAnalytics.windows.noReset')
      : seconds == null
        ? t('admin.accounts.codexAnalytics.windows.resetUnknown')
        : seconds <= 0
          ? t('admin.accounts.codexAnalytics.windows.resetting')
          : t('admin.accounts.codexAnalytics.windows.resetsIn', { time: formatCountdown(seconds) })
  }
}

function secondsUntilReset(window?: CodexAnalyticsRateLimitWindow | null): number | null {
  if (!window) return null
  if (window.reset_at != null) {
    const timestamp = parseTimestamp(window.reset_at)
    if (timestamp != null) return Math.max(0, Math.ceil((timestamp - now.value) / 1000))
  }
  if (window.reset_after_seconds != null && analytics.value) {
    const fetchedAt = parseTimestamp(analytics.value.fetched_at)
    if (fetchedAt != null) {
      return Math.max(0, Math.ceil((fetchedAt + window.reset_after_seconds * 1000 - now.value) / 1000))
    }
  }
  return null
}

function formatPreferredResetCountdown(value: CodexAnalyticsResponse): string {
  const sevenDaySeconds = secondsUntilReset(value.rate_limits.seven_day)
  const seconds = sevenDaySeconds ?? secondsUntilReset(value.rate_limits.five_hour)
  if (seconds == null) return t('admin.accounts.codexAnalytics.unavailable')
  if (seconds <= 0) return t('admin.accounts.codexAnalytics.windows.resetting')
  return formatCountdown(seconds)
}

function clampPercent(value: unknown): number {
  const percent = Number(value)
  return Number.isFinite(percent) ? Math.min(100, Math.max(0, percent)) : 0
}

function formatCountdown(totalSeconds: number): string {
  const seconds = Math.max(0, Math.floor(totalSeconds))
  const days = Math.floor(seconds / 86_400)
  const hours = Math.floor((seconds % 86_400) / 3_600)
  const minutes = Math.floor((seconds % 3_600) / 60)
  const rest = seconds % 60
  if (days > 0) return `${days}d ${String(hours).padStart(2, '0')}:${String(minutes).padStart(2, '0')}:${String(rest).padStart(2, '0')}`
  return `${String(hours).padStart(2, '0')}:${String(minutes).padStart(2, '0')}:${String(rest).padStart(2, '0')}`
}

function formatNumber(value: number): string {
  return new Intl.NumberFormat(locale.value).format(Number.isFinite(value) ? value : 0)
}

function formatOptionalNumber(value?: number | null): string {
  return value == null ? t('admin.accounts.codexAnalytics.unavailable') : formatNumber(value)
}

function formatTokens(value: number): string {
  const safe = Number.isFinite(value) ? value : 0
  return new Intl.NumberFormat(locale.value, { notation: 'compact', maximumFractionDigits: 1 }).format(safe)
}

function formatOptionalTokens(value?: number | null): string {
  return value == null ? t('admin.accounts.codexAnalytics.unavailable') : formatTokens(value)
}

function formatPercent(value: number): string {
  const safe = Number.isFinite(value) ? value : 0
  return `${safe.toFixed(1)}%`
}


function formatCurrency(value: number): string {
  return new Intl.NumberFormat(locale.value, { style: 'currency', currency: 'USD', maximumFractionDigits: 4 }).format(Number.isFinite(value) ? value : 0)
}

function formatOptionalDuration(value?: number | null): string {
  if (value == null) return t('admin.accounts.codexAnalytics.unavailable')
  if (value < 60) return t('admin.accounts.codexAnalytics.profile.seconds', { count: Math.round(value) })
  return t('admin.accounts.codexAnalytics.profile.minutes', { count: (value / 60).toFixed(1) })
}

function formatOptionalDays(value?: number | null): string {
  return value == null
    ? t('admin.accounts.codexAnalytics.unavailable')
    : t('admin.accounts.codexAnalytics.profile.days', { count: formatNumber(value) })
}

function parseTimestamp(value: number | string): number | null {
  const numeric = Number(value)
  if (Number.isFinite(numeric)) return numeric < 10_000_000_000 ? numeric * 1000 : numeric
  const parsed = Date.parse(String(value))
  return Number.isFinite(parsed) ? parsed : null
}

function formatPeriodDate(value: number): string {
  const timestamp = parseTimestamp(value)
  if (timestamp == null) return t('admin.accounts.codexAnalytics.unavailable')
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium' }).format(timestamp)
}

function formatDateTime(value: number | string): string {
  const timestamp = parseTimestamp(value)
  if (timestamp == null) return t('admin.accounts.codexAnalytics.unavailable')
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'short' }).format(timestamp)
}
</script>
