<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import EmptyState from '@/components/common/EmptyState.vue'
import { opsAPI, type OpsOpenAIStickyStatsResponse } from '@/api/admin/ops'
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

const { t } = useI18n()

const loading = ref(false)
const errorMessage = ref('')
const response = ref<OpsOpenAIStickyStatsResponse | null>(null)

const summaryMetrics = computed(() => {
  const data = response.value
  return [
    {
      key: 'evaluated_request_count',
      label: t('admin.ops.openaiSticky.evaluatedRequestCount'),
      value: data?.evaluated_request_count ?? 0,
      isPercent: false,
    },
    {
      key: 'sticky_hit_count',
      label: t('admin.ops.openaiSticky.stickyHitCount'),
      value: data?.sticky_hit_count ?? 0,
      isPercent: false,
    },
    {
      key: 'sticky_hit_rate',
      label: t('admin.ops.openaiSticky.stickyHitRate'),
      value: data?.sticky_hit_rate ?? 0,
      isPercent: true,
    },
    {
      key: 'sticky_account_switch_rate',
      label: t('admin.ops.openaiSticky.stickyAccountSwitchRate'),
      value: data?.sticky_account_switch_rate ?? 0,
      isPercent: true,
    },
  ]
})

const evalResultItems = computed(() => {
  const entries = Object.entries(response.value?.eval_result_count ?? {})
  return entries.sort((a, b) => b[1] - a[1])
})

const sessionSourceItems = computed(() => {
  const entries = Object.entries(response.value?.session_source_count ?? {})
  return entries.sort((a, b) => b[1] - a[1])
})

const hasData = computed(() => {
  const data = response.value
  return !!data && (
    data.evaluated_request_count > 0 ||
    Object.keys(data.eval_result_count || {}).length > 0 ||
    Object.keys(data.session_source_count || {}).length > 0
  )
})

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

function formatMetricValue(value: number, isPercent: boolean): string {
  if (isPercent) return `${(value * 100).toFixed(1)}%`
  return formatNumber(Math.round(value))
}

function formatCount(value: number): string {
  return formatNumber(Math.round(value || 0))
}

function translateEvalResult(key: string): string {
  const map: Record<string, string> = {
    hit: t('admin.ops.openaiSticky.eval.hit'),
    miss_no_binding: t('admin.ops.openaiSticky.eval.missNoBinding'),
    miss_binding_invalid: t('admin.ops.openaiSticky.eval.missBindingInvalid'),
    miss_binding_restricted: t('admin.ops.openaiSticky.eval.missBindingRestricted'),
    miss_binding_excluded: t('admin.ops.openaiSticky.eval.missBindingExcluded'),
    bypassed_previous_response_id: t('admin.ops.openaiSticky.eval.bypassedPreviousResponseId'),
    no_session_signal: t('admin.ops.openaiSticky.eval.noSessionSignal'),
  }
  return map[key] || key
}

function translateSessionSource(key: string): string {
  const map: Record<string, string> = {
    header_session_id: t('admin.ops.openaiSticky.source.headerSessionId'),
    header_conversation_id: t('admin.ops.openaiSticky.source.headerConversationId'),
    header_x_session_affinity: t('admin.ops.openaiSticky.source.headerXSessionAffinity'),
    prompt_cache_key: t('admin.ops.openaiSticky.source.promptCacheKey'),
    content_fallback: t('admin.ops.openaiSticky.source.contentFallback'),
    fallback_seed: t('admin.ops.openaiSticky.source.fallbackSeed'),
    none: t('admin.ops.openaiSticky.source.none'),
  }
  return map[key] || key
}

async function loadData() {
  loading.value = true
  errorMessage.value = ''
  try {
    response.value = await opsAPI.getOpenAIStickyStats(buildParams())
  } catch (err: any) {
    console.error('[OpsOpenAIStickyCard] Failed to load data', err)
    response.value = null
    errorMessage.value = err?.message || t('admin.ops.openaiSticky.failedToLoad')
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
        {{ t('admin.ops.openaiSticky.title') }}
      </h3>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.ops.openaiSticky.subtitle') }}
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
      :description="t('admin.ops.openaiSticky.empty')"
    />

    <div v-else class="space-y-4">
      <div class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
        <article
          v-for="metric in summaryMetrics"
          :key="metric.key"
          class="rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800"
        >
          <div class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
            {{ metric.label }}
          </div>
          <div class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">
            {{ formatMetricValue(metric.value, metric.isPercent) }}
          </div>
        </article>
      </div>

      <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <article class="rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('admin.ops.openaiSticky.evalBreakdown') }}
          </div>
          <div class="mt-3 space-y-2 text-sm">
            <div v-for="item in evalResultItems" :key="item[0]" class="flex items-center justify-between gap-3">
              <span class="text-gray-600 dark:text-gray-300">{{ translateEvalResult(item[0]) }}</span>
              <span class="font-semibold text-gray-900 dark:text-white">{{ formatCount(item[1]) }}</span>
            </div>
          </div>
        </article>

        <article class="rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('admin.ops.openaiSticky.sessionSourceBreakdown') }}
          </div>
          <div class="mt-3 space-y-2 text-sm">
            <div v-for="item in sessionSourceItems" :key="item[0]" class="flex items-center justify-between gap-3">
              <span class="text-gray-600 dark:text-gray-300">{{ translateSessionSource(item[0]) }}</span>
              <span class="font-semibold text-gray-900 dark:text-white">{{ formatCount(item[1]) }}</span>
            </div>
          </div>
        </article>
      </div>
    </div>
  </section>
</template>
