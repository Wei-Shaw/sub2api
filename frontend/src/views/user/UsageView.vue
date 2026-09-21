<template>
  <AppLayout>
    <div class="space-y-6">
      <UsageStatsCards
        :stats="usageStats"
        :show-account-cost="false"
        :show-cost-breakdown="false"
        currency-symbol="¥"
      />

      <div class="card p-4">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div class="flex items-center gap-2">
            <span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.dashboard.timeRange') }}:</span>
            <DateRangePicker v-model:start-date="startDate" v-model:end-date="endDate" @change="onDateRangeChange" />
          </div>
          <button type="button" @click="refreshData" :disabled="loading" class="btn btn-secondary">
            {{ t('common.refresh') }}
          </button>
        </div>
      </div>

      <UsageTable
        :data="usageLogs"
        :loading="loading"
        :columns="usageColumns"
        :server-side-sort="true"
        :show-account-billing="false"
        :show-upstream-endpoint="false"
        currency-symbol="¥"
        :show-cost-details="false"
        latency-display="preferred"
        default-sort-key="created_at"
        default-sort-order="desc"
        @sort="handleSort"
        @ipGeoBatchFailed="handleIpGeoBatchFailed"
      />

      <Pagination
        v-if="pagination.total > 0"
        :page="pagination.page"
        :total="pagination.total"
        :page-size="10"
        :show-page-size-selector="false"
        @update:page="handlePageChange"
      />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { usageAPI } from '@/api'
import AppLayout from '@/components/layout/AppLayout.vue'
import Pagination from '@/components/common/Pagination.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import UsageStatsCards from '@/components/admin/usage/UsageStatsCards.vue'
import UsageTable from '@/components/admin/usage/UsageTable.vue'
import type { Column } from '@/components/common/types'
import type { UsageLog, UsageQueryParams, UsageStatsResponse } from '@/types'

const { t } = useI18n()
const appStore = useAppStore()
const usageStats = ref<UsageStatsResponse | null>(null)
const usageLogs = ref<UsageLog[]>([])
const loading = ref(false)
let abortController: AbortController | null = null

const formatLocalDate = (date: Date): string =>
  `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`

const getLast24HoursRangeDates = () => {
  const end = new Date()
  const start = new Date(end.getTime() - 24 * 60 * 60 * 1000)
  return { start: formatLocalDate(start), end: formatLocalDate(end) }
}

const defaultRange = getLast24HoursRangeDates()
const startDate = ref(defaultRange.start)
const endDate = ref(defaultRange.end)
const pagination = reactive({ page: 1, page_size: 10, total: 0 })
const sortState = reactive({ sort_by: 'created_at', sort_order: 'desc' as 'asc' | 'desc' })

const usageColumns = computed<Column[]>(() => [
  { key: 'created_at', label: t('usage.time'), sortable: true },
  { key: 'api_key', label: t('usage.apiKeyFilter'), sortable: false },
  { key: 'group', label: t('admin.usage.group'), sortable: false },
  { key: 'model', label: t('usage.model'), sortable: true },
  { key: 'latency', label: t('usage.elapsed'), sortable: false },
  { key: 'input_tokens', label: t('usage.in'), sortable: false },
  { key: 'output_tokens', label: t('usage.out'), sortable: false },
  { key: 'cost', label: t('usage.cost'), sortable: false },
  { key: 'ip_address', label: 'IP', sortable: false },
])

const buildParams = (page = pagination.page): UsageQueryParams => ({
  page,
  page_size: 10,
  start_date: startDate.value,
  end_date: endDate.value,
  sort_by: sortState.sort_by,
  sort_order: sortState.sort_order,
})

const loadLogs = async () => {
  abortController?.abort()
  const controller = new AbortController()
  abortController = controller
  loading.value = true
  try {
    const response = await usageAPI.query(buildParams(), { signal: controller.signal })
    if (!controller.signal.aborted) {
      usageLogs.value = response.items
      pagination.total = response.total
    }
  } catch (error: any) {
    if (error?.name !== 'AbortError' && error?.code !== 'ERR_CANCELED') {
      appStore.showError(t('usage.failedToLoad'))
    }
  } finally {
    if (abortController === controller) loading.value = false
  }
}

const loadStats = async () => {
  try {
    usageStats.value = await usageAPI.getStats({
      start_date: startDate.value,
      end_date: endDate.value,
    })
  } catch (error) {
    console.error('Failed to load usage stats:', error)
    usageStats.value = null
  }
}

const refreshData = () => {
  void loadLogs()
  void loadStats()
}

const onDateRangeChange = (range: { startDate: string; endDate: string; preset: string | null }) => {
  startDate.value = range.startDate
  endDate.value = range.endDate
  pagination.page = 1
  refreshData()
}

const handlePageChange = (page: number) => {
  pagination.page = page
  void loadLogs()
}

const handleSort = (key: string, order: 'asc' | 'desc') => {
  sortState.sort_by = key
  sortState.sort_order = order
  pagination.page = 1
  void loadLogs()
}

const handleIpGeoBatchFailed = () => {
  appStore.showError(t('usage.ipGeo.batchFailed'))
}

onMounted(refreshData)
onUnmounted(() => abortController?.abort())
</script>
