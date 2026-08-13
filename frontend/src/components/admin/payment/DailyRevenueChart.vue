<template>
  <div class="card p-4">
    <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">
      {{ t('payment.admin.dailyRevenue') }}
    </h3>
    <div class="h-64">
      <div v-if="loading" class="flex h-full items-center justify-center">
        <LoadingSpinner size="md" />
      </div>
      <Line v-else-if="chartData" ref="lineRef" :data="chartData" :options="chartOptions" />
      <div
        v-else
        class="flex h-full items-center justify-center text-sm text-gray-500 dark:text-gray-400"
      >
        {{ t('payment.admin.noData') }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Tooltip,
  Legend,
  Filler
} from 'chart.js'
import { Line } from 'vue-chartjs'
import { useThemedChart } from '@/components/charts/chartTheme'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import type { DailyPaymentStats } from '@/types/payment'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend, Filler)

const { t } = useI18n()

const props = defineProps<{
  data: DailyPaymentStats[]
  loading?: boolean
}>()

/**
 * The palette used to be four hardcoded `rgb()`/`rgba()` pairs plus a fifth
 * loose `rgb(16, 185, 129)` for the order-count line, so it had no relationship
 * to the design tokens and stayed light-mode blue/purple/amber in dark mode.
 *
 * `useThemedChart` is reactive on the theme AND pushes `update('none')` on a
 * flip — `'none'` because animating a theme change reads as a glitch. Line
 * tension, point radius, hit radius, fonts, grid, tooltip and legend box are
 * all set globally by `applyChartDefaults()`; nothing below re-declares them.
 */
type ChartInstance = { chart?: { update: (mode?: string) => void } } | null
const lineRef = ref<ChartInstance>(null)
const chartTheme = useThemedChart(lineRef)

/** `0.1` alpha as hex, preserving the fill opacity the raw `rgba()` used. */
const FILL_ALPHA = '1A'

const chartData = computed(() => {
  if (!props.data || props.data.length === 0) return null
  const currencies = [...new Set(props.data.flatMap(day => Object.keys(day.amount)))].sort()
  const palette = chartTheme.value.series
  const seriesColor = (index: number) => palette[index % palette.length]
  return {
    labels: props.data.map(d => d.date),
    datasets: [
      ...currencies.map((currency, index) => {
        const color = seriesColor(index)
        return {
          label: `${currency} ${t('payment.admin.revenue')}`,
          data: props.data.map(day => day.amount[currency] || 0),
          borderColor: color,
          backgroundColor: `${color}${FILL_ALPHA}`,
          fill: true,
          // Global point radius is 0. Order counts and revenue are sampled per
          // day and the range selector goes down to 7 points, so hovering is
          // the only way to read an exact value — a dot on hover marks which
          // sample the tooltip belongs to without dotting all 90 days.
          pointHoverRadius: 5,
        }
      }),
      {
        label: t('payment.admin.orderCount'),
        data: props.data.map(d => d.count),
        // Continues the categorical sequence rather than restarting it: the
        // order count is another series on this chart, so it must not reuse
        // whichever colour a currency already took.
        borderColor: seriesColor(currencies.length),
        backgroundColor: `${seriesColor(currencies.length)}${FILL_ALPHA}`,
        fill: false,
        pointHoverRadius: 5,
        yAxisID: 'y1',
      }
    ]
  }
})

/*
 * Was a plain object. Two bugs in one: it froze `t()` at construction, so the
 * axis titles kept the language the component happened to mount in, and it
 * never rebuilt on a theme flip. A `computed` re-reads both.
 */
const chartOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  interaction: { mode: 'index' as const, intersect: false },
  scales: {
    y: {
      type: 'linear' as const,
      display: true,
      position: 'left' as const,
      title: { display: true, text: t('payment.admin.revenue') },
    },
    y1: {
      type: 'linear' as const,
      display: true,
      position: 'right' as const,
      title: { display: true, text: t('payment.admin.orderCount') },
      grid: { drawOnChartArea: false },
    }
  },
  plugins: {
    legend: { position: 'top' as const },
  }
}))
</script>
