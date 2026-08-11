<template>
  <section
    class="card flex min-h-[360px] flex-col overflow-hidden !rounded-3xl !border-0 !p-6 shadow-sm ring-1 ring-gray-900/5 dark:!bg-dark-800 dark:ring-dark-700"
  >
    <div class="card-header mb-4 flex shrink-0 flex-wrap items-start justify-between gap-3 !border-0 !p-0">
      <div class="min-w-0">
        <h2 class="flex items-center gap-2 text-sm font-bold text-gray-900 dark:text-white">
          <span class="inline-flex h-4 w-4 text-sky-500" aria-hidden="true">
            <Icon name="chart" size="sm" />
          </span>
          {{ t('channelMonitorV2.chart.title') REDACTEDREDACTED
        </h2>
        <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">
          {{ t('channelMonitorV2.chart.description') REDACTEDREDACTED
        </p>
      </div>
      <div class="flex w-full min-w-0 flex-wrap items-center justify-end gap-2 text-xs text-gray-500 dark:text-gray-400 sm:w-auto">
        <span class="flex shrink-0 items-center gap-1">
          <span class="h-2 w-2 rounded-full bg-red-500"></span>{{ t('channelMonitorV2.chart.errorLegend') REDACTEDREDACTED
        </span>
        <span class="flex shrink-0 items-center gap-1">
          <span class="h-2 w-2 rounded-full bg-emerald-500"></span>{{ t('channelMonitorV2.chart.cacheLegend') REDACTEDREDACTED
        </span>
        <span class="flex shrink-0 items-center gap-1">
          <span class="h-2 w-2 rounded-full bg-sky-500"></span>{{ t('channelMonitorV2.chart.ttftLegend') REDACTEDREDACTED
        </span>
        <span class="badge badge-gray shrink-0">{{ bucketLabel REDACTEDREDACTED</span>
        <button
          type="button"
          class="inline-flex shrink-0 items-center rounded-lg border border-gray-200 bg-white px-2 py-1 text-[11px] font-semibold text-gray-600 hover:bg-gray-50 disabled:opacity-50 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-300 dark:hover:bg-dark-800"
          :disabled="!zoomed"
          @click="resetChartZoom"
        >
          {{ t('channelMonitorV2.chart.resetZoom') REDACTEDREDACTED
        </button>
      </div>
    </div>
    <div class="card-body min-h-0 flex-1 !p-0">
      <div v-if="loading" class="flex h-[280px] items-center justify-center sm:h-[300px]">
        <div class="animate-pulse text-sm text-gray-400">{{ t('common.loading') REDACTEDREDACTED</div>
      </div>
      <div
        v-else-if="chartData"
        ref="chartRef"
        class="h-[280px] sm:h-[300px]"
        @wheel="onChartWheel"
      >
        <Line :data="chartData" :options="chartOptions" />
      </div>
      <div v-else class="flex h-[280px] items-center justify-center sm:h-[300px]">
        <EmptyState
          :title="t('channelMonitorV2.chart.emptyTitle')"
          :description="t('channelMonitorV2.empty.description')"
        />
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n REDACTED from 'vue-i18n'
import { computed, ref, watch REDACTED from 'vue'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  Filler,
REDACTED from 'chart.js'
import { Line REDACTED from 'vue-chartjs'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import type { MonitorCoverage, MonitorMetric, MonitorHealth REDACTED from '@/api/channelMonitorV2'
import { formatMonitorMs, formatMonitorPercent REDACTED from '@/features/channel-monitor-v2/monitorFormat'
import {
  applyWheelZoom,
  clientXRatio,
  isZoomed,
  resetZoom,
  sliceByZoom,
  type ZoomState,
REDACTED from '@/features/channel-monitor-v2/monitorZoom'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Title, Tooltip, Legend, Filler)
const { t, locale REDACTED = useI18n()

const props = defineProps<{
  trend: Array<{ bucket_start: string; metrics: MonitorMetric; health: MonitorHealth REDACTED>
  coverage: MonitorCoverage | null
  loading?: boolean
REDACTED>()

const chartRef = ref<HTMLElement | null>(null)
const zoom = ref<ZoomState>(resetZoom())
const zoomed = computed(() => isZoomed(zoom.value))

const isDark = computed(() =>
  typeof document !== 'undefined' && document.documentElement.classList.contains('dark')
)

const bucketLabel = computed(() => {
  const seconds = props.coverage?.bucket_seconds || 60
  const minutes = seconds / 60
  if (minutes < 60) return t('channelMonitorV2.bucket.minutes', { count: minutes REDACTED)
  const hours = minutes / 60
  if (hours < 24) return t('channelMonitorV2.bucket.hours', { count: hours REDACTED)
  return t('channelMonitorV2.bucket.days', { count: hours / 24 REDACTED)
REDACTED)

const chartData = computed(() => {
  const points = visibleTrend.value
  if (!points.length) return null
  const labels = points.map((p) =>
    new Intl.DateTimeFormat(locale.value || undefined, {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    REDACTED).format(new Date(p.bucket_start))
  )
  const errorRates = smoothTrend(points.map((p) => (p.metrics.error_rate || 0) * 100))
  const cacheRates = smoothTrend(points.map((p) => (p.metrics.cache_rate || 0) * 100))
  const ttftP50 = smoothTrend(points.map((p) => p.metrics.ttft?.p50_ms ?? null))
  return {
    labels,
    datasets: [
      {
        label: t('channelMonitorV2.chart.errorDataset'),
        data: errorRates,
        borderColor: '#ef4444',
        backgroundColor: 'rgba(239, 67, 67, 0.10)',
        yAxisID: 'yPct',
        tension: 0.4,
        cubicInterpolationMode: 'monotone' as const,
        fill: 'origin' as const,
        pointRadius: 0,
        pointHoverRadius: 4,
        pointHitRadius: 10,
        borderWidth: 2,
      REDACTED,
      {
        label: t('channelMonitorV2.chart.cacheDataset'),
        data: cacheRates,
        borderColor: '#10b981',
        backgroundColor: 'rgba(16, 185, 129, 0.08)',
        yAxisID: 'yPct',
        tension: 0.4,
        cubicInterpolationMode: 'monotone' as const,
        fill: false,
        pointRadius: 0,
        pointHoverRadius: 4,
        pointHitRadius: 10,
        borderWidth: 2,
      REDACTED,
      {
        label: t('channelMonitorV2.chart.ttftDataset'),
        data: ttftP50,
        borderColor: '#0ea5e9',
        backgroundColor: 'rgba(14, 165, 233, 0.08)',
        yAxisID: 'yTtft',
        tension: 0.4,
        cubicInterpolationMode: 'monotone' as const,
        fill: false,
        pointRadius: 0,
        pointHoverRadius: 4,
        pointHitRadius: 10,
        borderWidth: 2,
        spanGaps: true,
      REDACTED,
    ],
  REDACTED
REDACTED)

/** Window the series by zoom state around the cursor — not always the last N points. */
const visibleTrend = computed(() => sliceByZoom(props.trend || [], zoom.value))

function onChartWheel(event: WheelEvent) {
  // Plain vertical wheel zooms X (narrower time range); shift/horizontal pans.
  event.preventDefault()
  const ratio = clientXRatio(event.clientX, chartRef.value)
  zoom.value = applyWheelZoom(zoom.value, event, ratio)
REDACTED

function resetChartZoom() {
  zoom.value = resetZoom()
REDACTED

watch(() => props.trend, () => {
  zoom.value = resetZoom()
REDACTED)

function smoothTrend(values: Array<number | null>): Array<number | null> {
  if (values.length <= 2) return values
  return values.map((value, index) => {
    if (value == null) return null
    const neighbors = values.slice(Math.max(0, index - 1), Math.min(values.length, index + 2))
      .filter((item): item is number => item != null)
    if (!neighbors.length) return value
    return neighbors.reduce((sum, item) => sum + item, 0) / neighbors.length
  REDACTED)
REDACTED

const chartOptions = computed(() => {
  const text = isDark.value ? '#9ca3af' : '#6b7280'
  const grid = isDark.value ? '#374151' : '#f3f4f6'
  const tooltipBg = isDark.value ? '#1f2937' : '#ffffff'
  const tooltipTitle = isDark.value ? '#f3f4f6' : '#111827'
  const tooltipBody = isDark.value ? '#d1d5db' : '#4b5563'
  return {
    responsive: true,
    maintainAspectRatio: false,
    interaction: { mode: 'index' as const, intersect: false REDACTED,
    plugins: {
      legend: { display: false REDACTED,
      tooltip: {
        backgroundColor: tooltipBg,
        titleColor: tooltipTitle,
        bodyColor: tooltipBody,
        borderColor: grid,
        borderWidth: 1,
        padding: 10,
        displayColors: true,
        callbacks: {
          label(ctx: { dataset: { label?: string REDACTED; parsed: { y: number | null REDACTED REDACTED) {
            const label = ctx.dataset.label || ''
            const y = ctx.parsed.y
            if (y == null) return `${labelREDACTED: -`
            if (label === t('channelMonitorV2.chart.errorDataset') || label === t('channelMonitorV2.chart.cacheDataset')) {
              return `${labelREDACTED: ${formatMonitorPercent(y / 100)REDACTED`
            REDACTED
            return `${labelREDACTED: ${formatMonitorMs(y)REDACTED`
          REDACTED,
        REDACTED,
      REDACTED,
    REDACTED,
    scales: {
      x: {
        ticks: { color: text, maxRotation: 0, autoSkip: true, maxTicksLimit: 8, autoSkipPadding: 10, font: { size: 10 REDACTED REDACTED,
        grid: { display: false REDACTED,
      REDACTED,
      yPct: {
        type: 'linear' as const,
        position: 'left' as const,
        min: 0,
        suggestedMax: 100,
        ticks: {
          color: text,
          font: { size: 10 REDACTED,
          callback: (v: string | number) => `${vREDACTED%`,
        REDACTED,
        grid: { color: grid, borderDash: [4, 4] REDACTED,
        title: { display: true, text: t('channelMonitorV2.chart.percentAxis'), color: text, font: { size: 11 REDACTED REDACTED,
      REDACTED,
      yTtft: {
        type: 'linear' as const,
        position: 'right' as const,
        min: 0,
        ticks: {
          color: '#0ea5e9',
          font: { size: 10 REDACTED,
          callback: (v: string | number) => formatMonitorMs(Number(v)),
        REDACTED,
        grid: { display: false REDACTED,
        title: { display: true, text: t('channelMonitorV2.metrics.ttftP50'), color: '#0ea5e9', font: { size: 11 REDACTED REDACTED,
      REDACTED,
    REDACTED,
  REDACTED
REDACTED)
</script>
