<template>
  <div
    class="min-w-[7rem] text-xs"
    :title="details"
    :aria-label="details"
    :aria-busy="loading"
    data-testid="account-performance"
  >
    <div v-if="loading && !stats" class="space-y-1">
      <div v-for="row in 3" :key="row" class="h-3 w-20 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
    </div>
    <template v-else>
      <div v-if="stats?.request_count === 0" class="text-gray-400">
        {{ t('admin.accounts.performance.noUsage') }}
      </div>
      <div v-else-if="stats" class="space-y-0.5">
        <div v-for="metric in metrics" :key="metric.key" class="flex items-center justify-between gap-3">
          <span class="text-gray-500 dark:text-gray-400">{{ metric.label }}</span>
          <span class="whitespace-nowrap font-medium tabular-nums text-gray-700 dark:text-gray-300">
            {{ metric.value }}
          </span>
        </div>
        <div v-if="hasFewSamples" class="text-[10px] text-amber-600 dark:text-amber-400">
          {{ t('admin.accounts.performance.fewSamples') }}
        </div>
      </div>
      <div v-else-if="!error" class="text-gray-400">-</div>
      <div v-if="error" class="mt-0.5 text-amber-600 dark:text-amber-400" role="status">
        {{ t(stats ? 'admin.accounts.performance.stale' : 'admin.accounts.performance.failed') }}
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AccountPerformanceStats } from '@/api/admin/accounts'
import { formatDateTime } from '@/utils/format'

const props = withDefaults(defineProps<{
  stats?: AccountPerformanceStats | null
  windowStart?: string
  windowEnd?: string
  loading?: boolean
  error?: boolean
}>(), {
  stats: null,
  windowStart: '',
  windowEnd: '',
  loading: false,
  error: false
})

const { t, locale } = useI18n()
const number = (value: number, digits: number) =>
  new Intl.NumberFormat(locale.value, { maximumFractionDigits: digits }).format(value)

const metrics = computed(() => [
  {
    key: 'ttft',
    label: t('admin.accounts.performance.ttft'),
    value: props.stats?.ttft_ms == null ? '-' : `${number(props.stats.ttft_ms / 1000, 2)} s`,
    samples: props.stats?.ttft_samples ?? 0
  },
  {
    key: 'tps',
    label: t('admin.accounts.performance.tps'),
    value: props.stats?.tps == null ? '-' : `${number(props.stats.tps, 1)} tok/s`,
    samples: props.stats?.tps_samples ?? 0
  },
  {
    key: 'cache',
    label: t('admin.accounts.performance.cache'),
    value: props.stats?.cache_rate == null ? '-' : `${number(props.stats.cache_rate * 100, 1)}%`,
    samples: props.stats?.cache_samples ?? 0
  }
])

const hasFewSamples = computed(() => metrics.value.some(metric => metric.samples > 0 && metric.samples < 5))
const details = computed(() => {
  const lines = [t('admin.accounts.performance.hint')]
  if (props.windowStart && props.windowEnd) {
    lines.push(t('admin.accounts.performance.window', {
      start: formatDateTime(props.windowStart),
      end: formatDateTime(props.windowEnd)
    }))
  }
  if (props.stats) {
    lines.push(t('admin.accounts.performance.requests', { count: props.stats.request_count }))
    for (const metric of metrics.value) {
      lines.push(t('admin.accounts.performance.samples', { metric: metric.label, count: metric.samples }))
    }
    lines.push(t('admin.accounts.performance.lastRequest', {
      time: props.stats.last_request_at ? formatDateTime(props.stats.last_request_at) : '-'
    }))
  }
  lines.push(t('admin.accounts.performance.timingHint'), t('admin.accounts.performance.cacheHint'))
  if (props.error) lines.push(t(props.stats ? 'admin.accounts.performance.stale' : 'admin.accounts.performance.failed'))
  return lines.join('\n')
})
</script>
