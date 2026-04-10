<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import EmptyState from '@/components/common/EmptyState.vue'
import { opsAPI, type OpsOpenAIRoutingStatsResponse } from '@/api/admin/ops'
import { formatNumber } from '@/utils/format'

interface Props {
  platformFilter?: string
  groupIdFilter?: number | null
  timeRange: '5m' | '30m' | '1h' | '6h' | '24h' | 'custom'
  startTime?: string | null
  endTime?: string | null
  refreshToken: number
}

const props = withDefaults(defineProps<Props>(), {
  platformFilter: '',
  groupIdFilter: null,
  startTime: null,
  endTime: null,
})

const emit = defineEmits<{
  (e: 'open-details', targetGroup: string): void
}>()

const { t } = useI18n()

const loading = ref(false)
const errorMessage = ref('')
const response = ref<OpsOpenAIRoutingStatsResponse | null>(null)

const activeLabel = computed(() => t('admin.usage.routingTargetGroupActive'))
const exhaustedLabel = computed(() => t('admin.usage.routingTargetGroupExhausted'))

const metrics = computed(() => {
  const data = response.value
  const retriedRequestCountByGroup = data?.retried_request_count_by_group ?? {}
  const retryCountByGroup = data?.retry_count_by_group ?? {}

  return [
    {
      key: 'retried_request_count',
      label: t('admin.ops.openaiRetry.retriedRequestCount'),
      active: retriedRequestCountByGroup.active ?? 0,
      exhausted: retriedRequestCountByGroup.exhausted ?? 0,
    },
    {
      key: 'retry_count',
      label: t('admin.ops.openaiRetry.retryCount'),
      active: retryCountByGroup.active ?? 0,
      exhausted: retryCountByGroup.exhausted ?? 0,
    },
  ]
})

const hasData = computed(() => metrics.value.some((metric) => metric.active > 0 || metric.exhausted > 0))

function buildParams() {
  const params: Record<string, any> = {
    time_range: props.timeRange,
    platform: props.platformFilter || undefined,
    group_id: typeof props.groupIdFilter === 'number' && props.groupIdFilter > 0 ? props.groupIdFilter : undefined,
  }

  if (props.timeRange === 'custom') {
    params.start_time = props.startTime || undefined
    params.end_time = props.endTime || undefined
  }

  return params
}

function formatMetricValue(value: number): string {
  return formatNumber(Math.round(value))
}

function formatShare(value: number, total: number): string {
  if (total <= 0) return '0%'
  return `${((value / total) * 100).toFixed(1)}%`
}

function total(metric: { active: number; exhausted: number }): number {
  return (metric.active ?? 0) + (metric.exhausted ?? 0)
}

async function loadData() {
  loading.value = true
  errorMessage.value = ''
  try {
    response.value = await opsAPI.getOpenAIRoutingStats(buildParams())
  } catch (err: any) {
    console.error('[OpsOpenAIRetryCard] Failed to load data', err)
    response.value = null
    errorMessage.value = err?.message || t('admin.ops.openaiRetry.failedToLoad')
  } finally {
    loading.value = false
  }
}

watch(
  () => ({
    platform: props.platformFilter,
    groupId: props.groupIdFilter,
    timeRange: props.timeRange,
    startTime: props.startTime,
    endTime: props.endTime,
    refreshToken: props.refreshToken,
  }),
  () => {
    void loadData()
  },
  { immediate: true }
)
</script>

<template>
  <section class="card p-4 md:p-5">
    <div class="mb-4">
      <h3 class="text-sm font-bold text-gray-900 dark:text-white">
        {{ t('admin.ops.openaiRetry.title') }}
      </h3>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.ops.openaiRetry.subtitle') }}
      </p>
    </div>

    <div v-if="errorMessage" class="mb-4 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-600 dark:bg-red-900/20 dark:text-red-400">
      {{ errorMessage }}
    </div>

    <div v-if="loading" class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('admin.ops.loadingText') }}
    </div>

    <EmptyState
      v-else-if="!hasData"
      :title="t('common.noData')"
      :description="t('admin.ops.openaiRetry.empty')"
    />

    <div v-else class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-2">
      <article
        v-for="metric in metrics"
        :key="metric.key"
        class="rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800"
      >
        <div class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ metric.label }}
        </div>

        <div class="mt-4 space-y-3 text-sm">
          <button type="button" class="w-full rounded-xl bg-emerald-50 p-3 text-left dark:bg-emerald-900/20" @click="emit('open-details', 'active')">
            <div class="flex items-center justify-between gap-3">
              <span class="font-medium text-emerald-700 dark:text-emerald-300">{{ activeLabel }}</span>
              <span class="text-xs text-emerald-600 dark:text-emerald-400">{{ formatShare(metric.active, total(metric)) }}</span>
            </div>
            <div class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">
              {{ formatMetricValue(metric.active) }}
            </div>
          </button>

          <button type="button" class="w-full rounded-xl bg-amber-50 p-3 text-left dark:bg-amber-900/20" @click="emit('open-details', 'exhausted')">
            <div class="flex items-center justify-between gap-3">
              <span class="font-medium text-amber-700 dark:text-amber-300">{{ exhaustedLabel }}</span>
              <span class="text-xs text-amber-600 dark:text-amber-400">{{ formatShare(metric.exhausted, total(metric)) }}</span>
            </div>
            <div class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">
              {{ formatMetricValue(metric.exhausted) }}
            </div>
          </button>

          <button type="button" class="w-full border-t border-gray-200 pt-3 text-left text-sm dark:border-dark-700" @click="emit('open-details', '')">
            <div class="flex items-center justify-between gap-3">
              <span class="font-medium text-gray-600 dark:text-gray-300">{{ t('common.total') }}</span>
              <span class="text-base font-semibold text-gray-900 dark:text-white">{{ formatMetricValue(total(metric)) }}</span>
            </div>
          </button>
        </div>
      </article>
    </div>
  </section>
</template>
