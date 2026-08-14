<template>
  <div class="space-y-6">
    <!--
      Filter bar. Labels are field labels, not sentences: 2xs uppercase in
      tertiary ink, so the controls are what the eye lands on.
    -->
    <Surface>
      <div class="flex flex-wrap items-center gap-x-6 gap-y-3">
        <div class="flex min-w-0 items-center gap-2">
          <span class="shrink-0 text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
            {{ t('dashboard.timeRange') }}
          </span>
          <DateRangePicker
            :start-date="startDate"
            :end-date="endDate"
            @update:startDate="$emit('update:startDate', $event)"
            @update:endDate="$emit('update:endDate', $event)"
            @change="$emit('dateRangeChange', $event)"
          />
        </div>

        <div class="flex items-center gap-2">
          <span class="shrink-0 text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
            {{ t('dashboard.granularity') }}
          </span>
          <div class="w-28">
            <Select
              :model-value="granularity"
              :options="granularityOptions"
              @update:model-value="$emit('update:granularity', $event)"
              @change="$emit('granularityChange')"
            />
          </div>
        </div>

        <Button
          class="ml-auto"
          size="md"
          :disabled="loading"
          :loading="loading"
          @click="$emit('refresh')"
        >
          <template #icon>
            <Icon name="refresh" size="xs" />
          </template>
          {{ t('common.refresh') }}
        </Button>
      </div>
    </Surface>

    <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
      <!-- Model distribution: donut + the table that doubles as its legend. -->
      <Surface
        :title="t('dashboard.modelDistribution')"
        flush
        class="relative overflow-hidden"
        data-testid="dashboard-model-distribution"
      >
        <!--
          Opaque veil, deliberately. A translucent + blurred overlay let a
          half-rendered chart show through, which reads as a rendering fault
          rather than as loading. Do not make this transparent again.
        -->
        <div v-if="loading" class="absolute inset-0 z-10 flex items-center justify-center bg-surface">
          <LoadingSpinner size="md" />
        </div>

        <div class="flex flex-col gap-4 p-4 sm:flex-row sm:items-start sm:gap-6">
          <div class="h-40 w-40 shrink-0 self-center">
            <Doughnut v-if="modelData" ref="doughnutRef" :data="modelData" :options="doughnutOptions" />
            <p v-else class="flex h-full items-center justify-center text-xs text-ink-tertiary">
              {{ t('dashboard.noDataAvailable') }}
            </p>
          </div>

          <div class="max-h-48 w-full min-w-0 flex-1 overflow-auto">
            <table v-if="rows.length > 0" class="table" data-testid="dashboard-model-table">
              <thead>
                <tr>
                  <th scope="col">{{ t('dashboard.model') }}</th>
                  <th scope="col" class="is-numeric">{{ t('dashboard.requests') }}</th>
                  <th scope="col" class="is-numeric">{{ t('dashboard.tokens') }}</th>
                  <th scope="col" class="is-numeric">{{ t('dashboard.actual') }}</th>
                  <th scope="col" class="is-numeric">{{ t('dashboard.standard') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="row in rows" :key="row.model">
                  <th scope="row" class="max-w-[9rem] text-left font-normal text-ink">
                    <span class="flex min-w-0 items-center gap-2">
                      <!--
                        8px square keyed to the slice. The donut has no legend
                        of its own — this column IS the legend, and it carries
                        the numbers too.
                      -->
                      <span
                        class="h-2 w-2 shrink-0"
                        :style="{ backgroundColor: row.color }"
                        aria-hidden="true"
                      />
                      <span class="truncate text-xs" :title="row.model">{{ row.model }}</span>
                    </span>
                  </th>
                  <td class="is-numeric"><NumCell :value="row.requests" /></td>
                  <td class="is-numeric"><NumCell :value="row.tokens" compact /></td>
                  <td class="is-numeric"><NumCell :value="row.actualCost" :precision="4" /></td>
                  <td class="is-numeric"><NumCell :value="row.cost" :precision="4" /></td>
                </tr>
              </tbody>
            </table>
            <p v-else class="py-8 text-center text-xs text-ink-tertiary">
              {{ t('dashboard.noDataAvailable') }}
            </p>
          </div>
        </div>
      </Surface>

      <!-- Token usage trend (shared chart component). -->
      <TokenUsageTrend :trend-data="trend" :loading="loading" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import Button from '@/components/common/Button.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import NumCell from '@/components/common/NumCell.vue'
import Select from '@/components/common/Select.vue'
import Surface from '@/components/common/Surface.vue'
import { useThemedChart } from '@/components/charts/chartTheme'
import TokenUsageTrend from '@/components/charts/TokenUsageTrend.vue'
import Icon from '@/components/icons/Icon.vue'
import { Doughnut } from 'vue-chartjs'
import type { TrendDataPoint, ModelStat } from '@/types'
import { formatTokensK as formatTokens } from '@/utils/format'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  ArcElement,
  Title,
  Tooltip,
  Legend,
  Filler,
} from 'chart.js'

ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  ArcElement,
  Title,
  Tooltip,
  Legend,
  Filler
)

const props = defineProps<{
  loading: boolean
  startDate: string
  endDate: string
  granularity: string
  trend: TrendDataPoint[]
  models: ModelStat[]
}>()

defineEmits([
  'update:startDate',
  'update:endDate',
  'update:granularity',
  'dateRangeChange',
  'granularityChange',
  'refresh',
])

const { t } = useI18n()

const granularityOptions = computed(() => [
  { value: 'day', label: t('dashboard.day') },
  { value: 'hour', label: t('dashboard.hour') },
])

/**
 * chart.js reads colors when the options object is built, so a chart whose
 * options are frozen keeps the old palette forever. This file used to do
 * exactly that: `doughnutOptions` was a plain object literal and the slice
 * colors were eight hardcoded Tailwind hexes, so the donut never re-themed and
 * its palette had no relationship to the design tokens.
 *
 * `useThemedChart` gives a genuinely reactive theme AND pushes an
 * `update('none')` on flip — `'none'` because animating a theme change looks
 * like a glitch rather than a transition.
 */
type ChartInstance = { chart?: { update: (mode?: string) => void } } | null
const doughnutRef = ref<ChartInstance>(null)
const chartTheme = useThemedChart(doughnutRef)

interface ModelRow {
  model: string
  color: string
  requests: number | null
  tokens: number | null
  actualCost: number | null
  cost: number | null
}

/**
 * A missing measurement and a measurement of zero are different facts, so no
 * `|| 0` anywhere here — `NumCell` renders an en dash for the former.
 */
function numOrNull(value: unknown): number | null {
  if (value === null || value === undefined || value === '') return null
  const n = Number(value)
  return Number.isFinite(n) ? n : null
}

const rows = computed<ModelRow[]>(() => {
  const palette = chartTheme.value.series
  return (props.models ?? []).map((m, index) => ({
    model: m.model,
    color: palette[index % palette.length],
    requests: numOrNull(m.requests),
    tokens: numOrNull(m.total_tokens),
    actualCost: numOrNull(m.actual_cost),
    cost: numOrNull(m.cost),
  }))
})

const modelData = computed(() => {
  if (!props.models?.length) return null
  return {
    labels: props.models.map((m: ModelStat) => m.model),
    datasets: [
      {
        data: props.models.map((m: ModelStat) => m.total_tokens),
        // Perceptually ordered categorical palette from the tokens, not eight
        // raw hexes that only worked in light mode.
        backgroundColor: rows.value.map((row) => row.color),
        /*
         * Deliberately opts out of the global `arc.borderWidth = 1`.
         *
         * The global hairline separates neighbouring slices of similar hue, and
         * it earns its place on a bounded donut. This one is unbounded: it maps
         * `props.models` straight through, and `usageAPI.getDashboardModels`
         * takes no limit and returns one row per distinct model with no
         * "Others" bucket, so the tail is all sub-1% slices. Worse than the
         * admin donut, in fact — this one is 160px (h-40), ~500px of
         * circumference, so a single hairline is a larger share of every slice
         * it touches. Below ~2% the separator IS the slice.
         */
        borderWidth: 0,
      },
    ],
  }
})

const doughnutOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  plugins: {
    // The table beside the donut is the legend, and it carries the numbers.
    legend: { display: false },
    tooltip: {
      callbacks: {
        label: (context: { label?: string; parsed: number }) =>
          `${context.label}: ${formatTokens(context.parsed)} tokens`,
      },
    },
  },
}))
</script>
