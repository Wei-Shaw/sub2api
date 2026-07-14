<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import Select from '@/components/common/Select.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import {
  opsAPI,
  type OpsUserUsageStatsParams,
  type OpsUserUsageStatsResponse,
  type OpsUserUsageStatsTimeRange
} from '@/api/admin/ops'
import { formatCurrency, formatDateTime } from '@/utils/format'
import { formatCompactNumber, formatExactNumber } from '../utils/opsFormatters'

interface Props {
  platformFilter?: string
  groupIdFilter?: number | null
  refreshToken: number
}

type ViewMode = 'topn' | 'pagination'

const props = withDefaults(defineProps<Props>(), {
  platformFilter: '',
  groupIdFilter: null
})

const { t } = useI18n()
const router = useRouter()
const loading = ref(false)
const errorMessage = ref('')
const response = ref<OpsUserUsageStatsResponse | null>(null)

const timeRange = ref<OpsUserUsageStatsTimeRange>('24h')
const viewMode = ref<ViewMode>('topn')
const topN = ref(20)
const page = ref(1)
const pageSize = ref(20)

const items = computed(() => response.value?.items ?? [])
const total = computed(() => response.value?.total ?? 0)
const totalPages = computed(() => {
  if (viewMode.value !== 'pagination') return 1
  return Math.max(1, Math.ceil(total.value / Math.max(pageSize.value, 1)))
})

const timeRangeOptions = computed(() => [
  { value: '30m', label: t('admin.ops.timeRange.30m') },
  { value: '1h', label: t('admin.ops.timeRange.1h') },
  { value: '24h', label: t('admin.ops.timeRange.24h') },
  { value: '15d', label: t('admin.ops.timeRange.15d') },
  { value: '30d', label: t('admin.ops.timeRange.30d') }
])

const viewModeOptions = computed(() => [
  { value: 'topn', label: t('admin.ops.userUsageStats.viewModeTopN') },
  { value: 'pagination', label: t('admin.ops.userUsageStats.viewModePagination') }
])

const topNOptions = [10, 20, 50, 100].map((value) => ({ value, label: `Top ${value}` }))
const pageSizeOptions = [10, 20, 50, 100].map((value) => ({ value, label: String(value) }))

function buildParams(): OpsUserUsageStatsParams {
  const params: OpsUserUsageStatsParams = {
    time_range: timeRange.value,
    platform: props.platformFilter || undefined,
    group_id: typeof props.groupIdFilter === 'number' && props.groupIdFilter > 0 ? props.groupIdFilter : undefined
  }
  if (viewMode.value === 'topn') {
    params.top_n = topN.value
  } else {
    params.page = page.value
    params.page_size = pageSize.value
  }
  return params
}

async function loadData() {
  loading.value = true
  errorMessage.value = ''
  try {
    response.value = await opsAPI.getUserUsageStats(buildParams())
    if (viewMode.value === 'pagination' && page.value > totalPages.value) {
      page.value = totalPages.value
      response.value = await opsAPI.getUserUsageStats(buildParams())
    }
  } catch (err: any) {
    console.error('[OpsUserUsageStatsCard] Failed to load data', err)
    response.value = null
    errorMessage.value = err?.message || t('admin.ops.userUsageStats.failedToLoad')
  } finally {
    loading.value = false
  }
}

watch(
  () => ({
    timeRange: timeRange.value,
    viewMode: viewMode.value,
    topN: topN.value,
    page: page.value,
    pageSize: pageSize.value,
    platform: props.platformFilter,
    groupId: props.groupIdFilter,
    refreshToken: props.refreshToken
  }),
  (next, prev) => {
    const filtersChanged = !prev ||
      next.timeRange !== prev.timeRange ||
      next.viewMode !== prev.viewMode ||
      next.pageSize !== prev.pageSize ||
      next.platform !== prev.platform ||
      next.groupId !== prev.groupId

    if (next.viewMode === 'pagination' && filtersChanged && next.page !== 1) {
      page.value = 1
      return
    }
    void loadData()
  },
  { immediate: true }
)

function openUserUsage(userID: number) {
  void router.push({ path: '/admin/usage', query: { user_id: String(userID) } })
}

function onPrevPage() {
  if (viewMode.value === 'pagination' && page.value > 1) page.value -= 1
}

function onNextPage() {
  if (viewMode.value === 'pagination' && page.value < totalPages.value) page.value += 1
}
</script>

<template>
  <section class="card p-4 md:p-5">
    <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
      <div>
        <h3 class="text-sm font-bold text-gray-900 dark:text-white">
          {{ t('admin.ops.userUsageStats.title') }}
        </h3>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.ops.userUsageStats.description') }}
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <div class="w-36">
          <Select v-model="timeRange" :options="timeRangeOptions" />
        </div>
        <div class="w-36">
          <Select v-model="viewMode" :options="viewModeOptions" />
        </div>
        <div v-if="viewMode === 'topn'" class="w-28">
          <Select v-model="topN" :options="topNOptions" />
        </div>
        <template v-else>
          <div class="w-24">
            <Select v-model="pageSize" :options="pageSizeOptions" />
          </div>
          <button class="btn btn-secondary btn-sm" :disabled="loading || page <= 1" @click="onPrevPage">
            {{ t('admin.ops.userUsageStats.prevPage') }}
          </button>
          <button class="btn btn-secondary btn-sm" :disabled="loading || page >= totalPages" @click="onNextPage">
            {{ t('admin.ops.userUsageStats.nextPage') }}
          </button>
          <span class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.ops.userUsageStats.pageInfo', { page, total: totalPages }) }}
          </span>
        </template>
      </div>
    </div>

    <div v-if="errorMessage" class="mb-4 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-600 dark:bg-red-900/20 dark:text-red-400">
      {{ errorMessage }}
    </div>

    <div v-if="loading" class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('admin.ops.loadingText') }}
    </div>

    <EmptyState
      v-else-if="items.length === 0"
      :title="t('common.noData')"
      :description="t('admin.ops.userUsageStats.empty')"
    />

    <div v-else class="space-y-3">
      <div class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-700">
        <div class="max-h-[460px] overflow-auto">
          <table class="min-w-[1120px] w-full text-left text-xs md:text-sm">
            <thead class="sticky top-0 z-10 bg-white dark:bg-dark-800">
              <tr class="border-b border-gray-200 text-gray-500 dark:border-dark-700 dark:text-gray-400">
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.userUsageStats.table.user') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.userUsageStats.table.requestCount') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.userUsageStats.table.inputTokens') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.userUsageStats.table.outputTokens') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.userUsageStats.table.cacheTokens') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.userUsageStats.table.totalTokens') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.userUsageStats.table.actualCost') }}</th>
                <th class="px-3 py-2 font-semibold">{{ t('admin.ops.userUsageStats.table.lastRequestAt') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="row in items"
                :key="row.user_id"
                class="border-b border-gray-100 text-gray-700 last:border-b-0 dark:border-dark-800 dark:text-gray-200"
              >
                <td class="px-3 py-2">
                  <button class="text-left font-medium text-primary-600 hover:underline dark:text-primary-400" @click="openUserUsage(row.user_id)">
                    {{ row.username || row.email || `#${row.user_id}` }}
                  </button>
                  <div v-if="row.email && row.email !== row.username" class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                    {{ row.email }} · #{{ row.user_id }}
                  </div>
                </td>
                <td class="px-3 py-2 tabular-nums" :title="formatExactNumber(row.request_count)">{{ formatCompactNumber(row.request_count) }}</td>
                <td class="px-3 py-2 tabular-nums" :title="formatExactNumber(row.input_tokens)">{{ formatCompactNumber(row.input_tokens) }}</td>
                <td class="px-3 py-2 tabular-nums" :title="formatExactNumber(row.output_tokens)">{{ formatCompactNumber(row.output_tokens) }}</td>
                <td class="px-3 py-2 tabular-nums" :title="formatExactNumber(row.cache_tokens)">{{ formatCompactNumber(row.cache_tokens) }}</td>
                <td class="px-3 py-2 tabular-nums font-medium" :title="formatExactNumber(row.total_tokens)">{{ formatCompactNumber(row.total_tokens) }}</td>
                <td class="px-3 py-2 tabular-nums">{{ formatCurrency(row.actual_cost) }}</td>
                <td class="px-3 py-2 whitespace-nowrap">{{ formatDateTime(row.last_request_at) || '-' }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
      <div
        v-if="viewMode === 'topn'"
        class="text-xs tabular-nums text-gray-500 dark:text-gray-400"
        :title="formatExactNumber(total)"
      >
        {{ t('admin.ops.userUsageStats.totalUsers', { total: formatCompactNumber(total) }) }}
      </div>
    </div>
  </section>
</template>
