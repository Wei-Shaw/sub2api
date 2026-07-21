<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.upstreamBilling.historyTitle')"
    width="extra-wide"
    @close="handleClose"
  >
    <div class="space-y-4" data-testid="upstream-rate-history-dialog">
      <div class="flex flex-col gap-3 border-b border-gray-200 pb-4 dark:border-gray-700 sm:flex-row sm:items-center sm:justify-between">
        <div class="min-w-0">
          <p class="truncate text-sm font-semibold text-gray-900 dark:text-white">{{ account?.name }}</p>
          <p class="text-xs text-gray-500 dark:text-gray-400">#{{ account?.id }}</p>
        </div>
        <div class="inline-flex w-fit rounded-md bg-gray-100 p-0.5 dark:bg-dark-700" role="group" :aria-label="t('admin.accounts.upstreamBilling.historyRange')">
          <button
            v-for="days in ranges"
            :key="days"
            type="button"
            class="min-w-12 rounded px-2.5 py-1.5 text-xs font-medium transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30"
            :class="selectedDays === days
              ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-600 dark:text-white'
              : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-100'"
            :aria-pressed="selectedDays === days"
            @click="selectedDays = days"
          >
            {{ t('admin.accounts.upstreamBilling.historyRangeDays', { count: days }) }}
          </button>
        </div>
      </div>

      <div
        v-if="warning"
        class="flex items-center justify-between gap-3 border-l-2 border-amber-400 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:bg-amber-900/20 dark:text-amber-200"
        data-testid="upstream-rate-history-cache-warning"
      >
        <span>{{ warning }}</span>
        <button type="button" class="shrink-0 underline underline-offset-2" @click="loadHistory">
          {{ t('admin.accounts.upstreamBilling.historyRetry') }}
        </button>
      </div>

      <div v-if="loading && !history" class="flex h-72 items-center justify-center">
        <LoadingSpinner />
      </div>

      <div
        v-else-if="error && !history"
        class="flex h-72 flex-col items-center justify-center gap-3 text-center text-sm text-gray-500 dark:text-gray-400"
        data-testid="upstream-rate-history-error"
      >
        <p>{{ error }}</p>
        <button type="button" class="btn btn-secondary inline-flex items-center gap-1.5" @click="loadHistory">
          <Icon name="refresh" size="sm" />
          {{ t('admin.accounts.upstreamBilling.historyRetry') }}
        </button>
      </div>

      <template v-else-if="history">
        <div
          v-if="history.truncated"
          class="border-l-2 border-amber-400 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:bg-amber-900/20 dark:text-amber-200"
          data-testid="upstream-rate-history-truncated"
        >
          {{ t('admin.accounts.upstreamBilling.historyTruncated') }}
        </div>

        <div
          v-if="history.events.length === 0"
          class="flex h-72 items-center justify-center text-sm text-gray-500 dark:text-gray-400"
          data-testid="upstream-rate-history-empty"
        >
          {{ t('admin.accounts.upstreamBilling.historyEmpty') }}
        </div>

        <template v-else>
          <div
            class="relative h-72 border-b border-gray-200 pb-4 dark:border-gray-700"
            @mousemove="handleChartMouseMove"
            @mouseleave="clearLineHover"
          >
            <Line ref="rateChart" :data="chartData" :options="chartOptions" />
            <span
              v-if="lineHover"
              class="pointer-events-none absolute z-10 -translate-x-1/2 -translate-y-full whitespace-nowrap rounded bg-gray-900 px-2 py-1 text-xs text-white shadow-lg dark:bg-gray-800"
              :style="{ left: `${lineHover.left}px`, top: `${lineHover.top}px` }"
              data-testid="upstream-rate-history-line-tooltip"
            >
              {{ t('admin.accounts.upstreamBilling.historyEffectiveRate') }}: {{ lineHover.value }}
            </span>
            <span
              v-if="loading"
              class="absolute right-2 top-1 text-xs text-gray-400 dark:text-gray-500"
              data-testid="upstream-rate-history-revalidating"
            >
              {{ t('admin.accounts.upstreamBilling.historyRefreshing') }}
            </span>
          </div>

          <div class="overflow-x-auto rounded-md border border-gray-200 dark:border-gray-700">
            <table class="min-w-full divide-y divide-gray-200 text-left text-xs dark:divide-gray-700">
              <thead class="bg-gray-50 text-gray-500 dark:bg-dark-700/60 dark:text-gray-400">
                <tr>
                  <th class="whitespace-nowrap px-3 py-2 font-medium">{{ t('admin.accounts.upstreamBilling.historyPeriod') }}</th>
                  <th class="whitespace-nowrap px-3 py-2 font-medium">{{ t('admin.accounts.upstreamBilling.historyDuration') }}</th>
                  <th class="whitespace-nowrap px-3 py-2 font-medium">{{ t('admin.accounts.upstreamBilling.historyGroupRate') }}</th>
                  <th class="whitespace-nowrap px-3 py-2 font-medium">{{ t('admin.accounts.upstreamBilling.historyUserRate') }}</th>
                  <th class="whitespace-nowrap px-3 py-2 font-medium">{{ t('admin.accounts.upstreamBilling.historyPeak') }}</th>
                  <th class="whitespace-nowrap px-3 py-2 font-medium">{{ t('admin.accounts.upstreamBilling.historyResolvedRate') }}</th>
                  <th class="whitespace-nowrap px-3 py-2 font-medium">{{ t('admin.accounts.upstreamBilling.historyEffectiveRate') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 bg-white text-gray-700 dark:divide-gray-800 dark:bg-dark-800 dark:text-gray-300">
                <tr v-for="event in history.events" :key="event.id">
                  <td class="whitespace-nowrap px-3 py-2.5">
                    <span>{{ formatTimestamp(event.detected_at) }}</span>
                    <span class="mx-1 text-gray-400">~</span>
                    <span>{{ event.interval_end ? formatTimestamp(event.interval_end) : t('admin.accounts.upstreamBilling.historyCurrent') }}</span>
                    <span v-if="event.carried_in" class="ml-1 text-[10px] text-gray-400">
                      {{ t('admin.accounts.upstreamBilling.historyCarriedIn') }}
                    </span>
                  </td>
                  <td class="whitespace-nowrap px-3 py-2.5">{{ formatEventDuration(event) }}</td>
                  <td class="whitespace-nowrap px-3 py-2.5 font-mono">{{ formatRate(event.group_rate_multiplier) }}</td>
                  <td class="whitespace-nowrap px-3 py-2.5 font-mono">{{ formatOptionalRate(event.user_rate_multiplier) }}</td>
                  <td class="whitespace-nowrap px-3 py-2.5">{{ formatPeak(event) }}</td>
                  <td class="whitespace-nowrap px-3 py-2.5 font-mono">{{ formatRate(event.resolved_rate_multiplier) }}</td>
                  <td class="whitespace-nowrap px-3 py-2.5 font-mono font-semibold text-sky-600 dark:text-sky-300">
                    {{ formatRate(event.effective_rate_multiplier) }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </template>
      </template>
    </div>

    <template #footer>
      <button type="button" class="btn btn-secondary" @click="handleClose">
        {{ t('common.close') }}
      </button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Chart as ChartJS,
  LinearScale,
  PointElement,
  LineElement,
  Tooltip,
  Legend
} from 'chart.js'
import type { TooltipItem } from 'chart.js'
import { Line } from 'vue-chartjs'
import BaseDialog from '@/components/common/BaseDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'
import type {
  Account,
  UpstreamBillingRateHistoryDays,
  UpstreamBillingRateHistoryEvent,
  UpstreamBillingRateHistoryResponse
} from '@/types'
import {
  getUpstreamBillingRateHistoryCache,
  setUpstreamBillingRateHistoryCache
} from './upstreamBillingRateHistoryCache'

ChartJS.register(LinearScale, PointElement, LineElement, Tooltip, Legend)

const props = defineProps<{
  show: boolean
  account: Account | null
}>()

const emit = defineEmits<{
  (event: 'close'): void
}>()

const { t } = useI18n()
const ranges: UpstreamBillingRateHistoryDays[] = [7, 30, 90, 365]
const selectedDays = ref<UpstreamBillingRateHistoryDays>(7)
const history = ref<UpstreamBillingRateHistoryResponse | null>(null)
const loading = ref(false)
const error = ref('')
const warning = ref('')
const rangeEnd = ref(Date.now())
const rateChart = ref<{ chart?: ChartJS<'line'> } | null>(null)
const lineHover = ref<{ left: number; top: number; value: string } | null>(null)
let requestController: AbortController | null = null
let requestSequence = 0
let dialogWasOpen = false

const loadHistory = async () => {
  const accountID = props.account?.id
  if (!props.show || !accountID) return
  requestController?.abort()
  const controller = new AbortController()
  requestController = controller
  const sequence = ++requestSequence
  const cached = getUpstreamBillingRateHistoryCache(accountID, selectedDays.value)
  rangeEnd.value = Date.now()
  history.value = cached?.data ?? null
  loading.value = true
  error.value = ''
  warning.value = ''

  try {
    const result = await adminAPI.accounts.getUpstreamBillingRateHistoryWithEtag(
      accountID,
      selectedDays.value,
      { signal: controller.signal, etag: cached?.etag }
    )
    if (sequence !== requestSequence) return
    if (result.notModified && cached) {
      setUpstreamBillingRateHistoryCache(accountID, selectedDays.value, {
        data: cached.data,
        etag: result.etag ?? cached.etag
      })
      history.value = cached.data
    } else if (result.data) {
      setUpstreamBillingRateHistoryCache(accountID, selectedDays.value, {
        data: result.data,
        etag: result.etag
      })
      history.value = result.data
    } else {
      const message = t('admin.accounts.upstreamBilling.historyLoadFailed')
      if (cached) warning.value = message
      else error.value = message
    }
  } catch (requestError) {
    const value = requestError as { code?: string; name?: string }
    if (value.code === 'ERR_CANCELED' || value.name === 'CanceledError' || value.name === 'AbortError') return
    const message = extractApiErrorMessage(requestError, t('admin.accounts.upstreamBilling.historyLoadFailed'))
    if (cached) warning.value = message
    else error.value = message
  } finally {
    if (sequence === requestSequence) loading.value = false
    if (requestController === controller) requestController = null
  }
}

watch(
  [() => props.show, () => props.account?.id, selectedDays],
  ([show, accountID, days]) => {
    const opening = show && !dialogWasOpen
    dialogWasOpen = show
    if (opening && days !== 7) {
      selectedDays.value = 7
      return
    }
    if (!show || !accountID) {
      if (!show) {
        requestSequence++
        requestController?.abort()
        requestController = null
      }
      return
    }
    void loadHistory()
  },
  { immediate: true }
)

onBeforeUnmount(() => {
  requestSequence++
  requestController?.abort()
})

const rangeStart = computed(() => rangeEnd.value - selectedDays.value * 24 * 60 * 60 * 1000)
const validEvents = computed(() => (history.value?.events ?? []).filter(event => (
  Number.isFinite(Date.parse(event.detected_at)) &&
  Number.isFinite(event.effective_rate_multiplier)
)))
const chartRangeStart = computed(() => {
  if (history.value?.truncated || validEvents.value.some(event => event.carried_in)) {
    return rangeStart.value
  }
  const firstEventAt = validEvents.value.reduce(
    (earliest, event) => Math.min(earliest, Date.parse(event.detected_at)),
    Number.POSITIVE_INFINITY
  )
  if (!Number.isFinite(firstEventAt)) return rangeStart.value
  return Math.max(rangeStart.value, Math.min(firstEventAt, rangeEnd.value))
})

const chartData = computed(() => {
  const events = validEvents.value
  const points = events.map(event => ({
    x: Math.max(chartRangeStart.value, Date.parse(event.detected_at)),
    y: event.effective_rate_multiplier
  }))
  const radii = events.map(event => event.carried_in ? 0 : 3)
  const hitRadii = events.map(event => event.carried_in ? 0 : 8)
  if (points.length > 0) {
    points.push({ x: rangeEnd.value, y: points[points.length - 1].y })
    radii.push(0)
    hitRadii.push(0)
  }
  return {
    datasets: [{
      label: t('admin.accounts.upstreamBilling.historyEffectiveRate'),
      data: points,
      borderColor: '#38bdf8',
      backgroundColor: '#38bdf8',
      borderWidth: 2,
      pointRadius: radii,
      pointHitRadius: hitRadii,
      pointHoverRadius: 5,
      // Chart.js "before" keeps the previous event value until the next event's x coordinate.
      stepped: 'before' as const,
      tension: 0
    }]
  }
})

const chartTextColor = computed(() => document.documentElement.classList.contains('dark') ? '#d1d5db' : '#4b5563')
const chartGridColor = computed(() => document.documentElement.classList.contains('dark') ? '#374151' : '#e5e7eb')
const chartOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  animation: false as const,
  interaction: { intersect: true, mode: 'nearest' as const },
  plugins: {
    legend: { display: false },
    tooltip: {
      callbacks: {
        title: (items: TooltipItem<'line'>[]) => {
          const value = items[0]?.parsed.x
          return typeof value === 'number' ? formatTimestamp(value) : ''
        },
        label: (item: TooltipItem<'line'>) => {
          const value = item.parsed.y
          return typeof value === 'number'
            ? `${t('admin.accounts.upstreamBilling.historyEffectiveRate')}: ${formatRate(value)}`
            : ''
        }
      }
    }
  },
  scales: {
    x: {
      type: 'linear' as const,
      min: chartRangeStart.value,
      max: rangeEnd.value,
      grid: { color: chartGridColor.value },
      ticks: {
        color: chartTextColor.value,
        maxTicksLimit: 7,
        callback: (value: string | number) => formatChartTimestamp(Number(value))
      }
    },
    y: {
      type: 'linear' as const,
      beginAtZero: true,
      grid: { color: chartGridColor.value },
      ticks: {
        color: chartTextColor.value,
        callback: (value: string | number) => formatRate(Number(value))
      }
    }
  }
}))

const formatRate = (value: number) => `${Number(value.toPrecision(12))}x`
const formatOptionalRate = (value?: number | null) => value == null ? '-' : formatRate(value)
const formatTimestamp = (value: string | number) => formatDateTime(typeof value === 'number' ? new Date(value) : value) || '-'
const formatChartTimestamp = (value: number) => formatDateTime(new Date(value), {
  month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit'
})
const clearLineHover = () => {
  lineHover.value = null
}
const handleChartMouseMove = (event: MouseEvent) => {
  const chart = rateChart.value?.chart
  const canvas = chart?.canvas
  if (!chart || !canvas) return clearLineHover()
  if (chart.getElementsAtEventForMode(event, 'nearest', { intersect: true }, false).length > 0) {
    return clearLineHover()
  }

  const rect = canvas.getBoundingClientRect()
  const cssCursorX = event.clientX - rect.left
  const cssCursorY = event.clientY - rect.top
  const cursorX = cssCursorX * chart.width / rect.width
  const cursorY = cssCursorY * chart.height / rect.height
  const timestamp = chart.scales.x.getValueForPixel(cursorX)
  if (timestamp == null || !Number.isFinite(timestamp)) return clearLineHover()

  const activeEvent = validEvents.value.reduce<UpstreamBillingRateHistoryEvent | null>((active, item) => {
    const itemAt = Math.max(chartRangeStart.value, Date.parse(item.detected_at))
    if (itemAt > timestamp) return active
    if (!active) return item
    const activeAt = Math.max(chartRangeStart.value, Date.parse(active.detected_at))
    return itemAt >= activeAt ? item : active
  }, null)
  if (!activeEvent) return clearLineHover()

  const lineY = chart.scales.y.getPixelForValue(activeEvent.effective_rate_multiplier)
  if (Math.abs(cursorY - lineY) > 8) return clearLineHover()
  const wrapperRect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  const cssLineY = lineY * rect.height / chart.height
  const lineTop = rect.top - wrapperRect.top + cssLineY
  lineHover.value = {
    left: rect.left - wrapperRect.left + Math.min(Math.max(cssCursorX, 60), Math.max(60, rect.width - 60)),
    top: lineTop - 8,
    value: formatRate(activeEvent.effective_rate_multiplier)
  }
}
const formatEventDuration = (event: UpstreamBillingRateHistoryEvent) => {
  const start = Math.max(rangeStart.value, Date.parse(event.detected_at))
  const end = event.interval_end ? Math.min(rangeEnd.value, Date.parse(event.interval_end)) : rangeEnd.value
  const minutes = Math.max(0, Math.floor((end - start) / 60_000))
  if (minutes < 1) return t('admin.accounts.upstreamBilling.historyDurationLessMinute')
  if (minutes < 60) return t('admin.accounts.upstreamBilling.historyDurationMinutes', { minutes })
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return t('admin.accounts.upstreamBilling.historyDurationHoursMinutes', { hours, minutes: minutes % 60 })
  return t('admin.accounts.upstreamBilling.historyDurationDaysHours', { days: Math.floor(hours / 24), hours: hours % 24 })
}
const formatPeak = (event: UpstreamBillingRateHistoryEvent) => {
  if (!event.peak_rate_enabled) return t('admin.accounts.upstreamBilling.historyPeakDisabled')
  return `${event.peak_start}-${event.peak_end} · ${formatOptionalRate(event.peak_rate_multiplier)} · ${event.peak_timezone}`
}
const handleClose = () => emit('close')
</script>
