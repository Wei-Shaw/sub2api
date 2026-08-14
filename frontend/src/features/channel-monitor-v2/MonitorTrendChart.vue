<template>
  <section class="flex min-h-[320px] min-w-0 flex-col rounded border border-line bg-surface">
    <header class="flex flex-wrap items-start justify-between gap-x-4 gap-y-2 border-b border-line px-4 py-3">
      <div class="min-w-0">
        <h2 class="truncate text-sm font-semibold text-ink">
          {{ t('channelMonitorV2.chart.title') }}
        </h2>
        <p class="mt-0.5 text-xs text-ink-tertiary">
          {{ t('channelMonitorV2.chart.description') }}
        </p>
      </div>
      <div class="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1.5">
        <!--
          Series swatches are 8px squares, matching the chart.js legend box that
          `applyChartDefaults()` sets. Each carries its own text label, so the
          colour is never the only channel.
        -->
        <span
          v-for="entry in legend"
          :key="entry.label"
          class="inline-flex shrink-0 items-center gap-1.5 text-2xs text-ink-tertiary"
        >
          <span
            class="h-2 w-2 shrink-0"
            :style="{ backgroundColor: entry.color }"
            aria-hidden="true"
          ></span>
          {{ entry.label }}
        </span>
        <Badge class="shrink-0">{{ bucketLabel }}</Badge>
        <Button size="xs" class="shrink-0" :disabled="!zoomed" @click="resetChartZoom">
          {{ t('channelMonitorV2.chart.resetZoom') }}
        </Button>
      </div>
    </header>

    <div class="min-h-0 flex-1 p-4">
      <!-- Flat hairline placeholders. No shimmer sweep, no spinner. -->
      <div v-if="loading" class="flex h-[280px] flex-col justify-end gap-3 sm:h-[300px]">
        <div v-for="i in 5" :key="i" class="skeleton h-3" :style="{ width: `${34 + i * 13}%` }"></div>
      </div>
      <div
        v-else-if="chartData"
        ref="chartRef"
        class="h-[280px] sm:h-[300px]"
        @wheel="onChartWheel"
      >
        <Line ref="lineRef" :data="chartData" :options="chartOptions" />
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
import { useI18n } from 'vue-i18n'
import { computed, ref, watch } from 'vue'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
} from 'chart.js'
import { Line } from 'vue-chartjs'

import { useThemedChart } from '@/components/charts/chartTheme'
import Badge from '@/components/common/Badge.vue'
import Button from '@/components/common/Button.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import type { MonitorCoverage, MonitorMetric, MonitorHealth } from '@/api/channelMonitorV2'
import { formatMonitorMs, formatMonitorPercent } from '@/features/channel-monitor-v2/monitorFormat'
import {
  applyWheelZoom,
  clientXRatio,
  isZoomed,
  resetZoom,
  sliceByZoom,
  type ZoomState,
} from '@/features/channel-monitor-v2/monitorZoom'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Title, Tooltip, Legend)
const { t, locale } = useI18n()

const props = defineProps<{
  trend: Array<{ bucket_start: string; metrics: MonitorMetric; health: MonitorHealth }>
  coverage: MonitorCoverage | null
  loading?: boolean
}>()

const chartRef = ref<HTMLElement | null>(null)
const zoom = ref<ZoomState>(resetZoom())
const zoomed = computed(() => isZoomed(zoom.value))

/**
 * The chart palette used to be three hardcoded hexes plus five theme-branched
 * `isDark ? … : …` pairs, so it had no relationship to the design tokens and
 * the axis/tooltip colours were maintained twice.
 *
 * `useThemedChart` is reactive on the theme AND pushes `update('none')` on a
 * flip — `'none'` because animating a theme change reads as a glitch. Axis,
 * grid, tooltip, font, point radius and legend box are all set globally by
 * `applyChartDefaults()`; nothing below re-declares them.
 */
type ChartInstance = { chart?: { update: (mode?: string) => void } } | null
const lineRef = ref<ChartInstance>(null)
const chartTheme = useThemedChart(lineRef)

/** Error · cache · TTFT, in the categorical series order. Never the status hues. */
const seriesColors = computed(() => {
  const palette = chartTheme.value.series
  return { error: palette[0], cache: palette[1], ttft: palette[2] }
})

const legend = computed(() => [
  { color: seriesColors.value.error, label: t('channelMonitorV2.chart.errorLegend') },
  { color: seriesColors.value.cache, label: t('channelMonitorV2.chart.cacheLegend') },
  { color: seriesColors.value.ttft, label: t('channelMonitorV2.chart.ttftLegend') },
])

const bucketLabel = computed(() => {
  const seconds = props.coverage?.bucket_seconds || 60
  const minutes = seconds / 60
  if (minutes < 60) return t('channelMonitorV2.bucket.minutes', { count: minutes })
  const hours = minutes / 60
  if (hours < 24) return t('channelMonitorV2.bucket.hours', { count: hours })
  return t('channelMonitorV2.bucket.days', { count: hours / 24 })
})

const chartData = computed(() => {
  const points = visibleTrend.value
  if (!points.length) return null
  const labels = points.map((p) =>
    new Intl.DateTimeFormat(locale.value || undefined, {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    }).format(new Date(p.bucket_start))
  )
  /*
   * Plotted values are the measured values.
   *
   * These three series used to be pushed through a 3-point moving average and
   * then drawn with `tension: 0.4` + monotone interpolation. Both invent data:
   * the average halves a single-bucket latency spike or error burst — exactly
   * the event this page exists to surface — and the bezier then invents further
   * values between the samples it was given. The tooltip reported the smoothed
   * number as if it had been measured. `applyChartDefaults()` sets
   * `elements.line.tension = 0` for this reason; nothing here overrides it.
   */
  return {
    labels,
    datasets: [
      {
        label: t('channelMonitorV2.chart.errorDataset'),
        data: points.map((p) => (p.metrics.error_rate || 0) * 100),
        borderColor: seriesColors.value.error,
        backgroundColor: seriesColors.value.error,
        yAxisID: 'yPct',
        fill: false,
      },
      {
        label: t('channelMonitorV2.chart.cacheDataset'),
        data: points.map((p) => (p.metrics.cache_rate || 0) * 100),
        borderColor: seriesColors.value.cache,
        backgroundColor: seriesColors.value.cache,
        yAxisID: 'yPct',
        fill: false,
      },
      {
        label: t('channelMonitorV2.chart.ttftDataset'),
        // `?? null` keeps "not measured" distinct from "measured 0ms".
        data: points.map((p) => p.metrics.ttft?.p50_ms ?? null),
        borderColor: seriesColors.value.ttft,
        backgroundColor: seriesColors.value.ttft,
        yAxisID: 'yTtft',
        fill: false,
        spanGaps: true,
      },
    ],
  }
})

/** Window the series by zoom state around the cursor — not always the last N points. */
const visibleTrend = computed(() => sliceByZoom(props.trend || [], zoom.value))

function onChartWheel(event: WheelEvent) {
  // Plain vertical wheel zooms X (narrower time range); shift/horizontal pans.
  event.preventDefault()
  const ratio = clientXRatio(event.clientX, chartRef.value)
  zoom.value = applyWheelZoom(zoom.value, event, ratio)
}

function resetChartZoom() {
  zoom.value = resetZoom()
}

watch(() => props.trend, () => {
  zoom.value = resetZoom()
})

const chartOptions = computed(() => {
  const theme = chartTheme.value
  return {
    responsive: true,
    maintainAspectRatio: false,
    interaction: { mode: 'index' as const, intersect: false },
    plugins: {
      legend: { display: false },
      tooltip: {
        callbacks: {
          label(ctx: { dataset: { label?: string }; parsed: { y: number | null } }) {
            const label = ctx.dataset.label || ''
            const y = ctx.parsed.y
            if (y == null) return `${label}: –`
            if (
              label === t('channelMonitorV2.chart.errorDataset') ||
              label === t('channelMonitorV2.chart.cacheDataset')
            ) {
              return `${label}: ${formatMonitorPercent(y / 100)}`
            }
            return `${label}: ${formatMonitorMs(y)}`
          },
        },
      },
    },
    scales: {
      x: {
        ticks: { maxRotation: 0, autoSkip: true, maxTicksLimit: 8, autoSkipPadding: 10 },
        grid: { display: false },
      },
      yPct: {
        type: 'linear' as const,
        position: 'left' as const,
        min: 0,
        suggestedMax: 100,
        ticks: { callback: (v: string | number) => `${v}%` },
        grid: { color: theme.grid },
        title: {
          display: true,
          text: t('channelMonitorV2.chart.percentAxis'),
          color: theme.axisTitle,
        },
      },
      yTtft: {
        type: 'linear' as const,
        position: 'right' as const,
        min: 0,
        ticks: {
          color: seriesColors.value.ttft,
          callback: (v: string | number) => formatMonitorMs(Number(v)),
        },
        grid: { display: false },
        title: {
          display: true,
          text: t('channelMonitorV2.metrics.ttftP50'),
          color: seriesColors.value.ttft,
        },
      },
    },
  }
})
</script>
