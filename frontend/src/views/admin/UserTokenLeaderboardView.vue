<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="card p-4">
        <div class="flex flex-wrap items-center gap-4">
          <div class="flex items-center gap-2">
            <span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.dashboard.timeRange') }}:</span>
            <DateRangePicker
              v-model:start-date="startDate"
              v-model:end-date="endDate"
              @change="onDateRangeChange"
            />
          </div>

          <div class="ml-auto flex flex-wrap items-center gap-3">
            <div class="flex items-center gap-2">
              <span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.usage.tokenLeaderboardLimit') }}:</span>
              <div class="w-24">
                <Select v-model="limit" :options="limitOptions" @change="loadRanking" />
              </div>
            </div>
            <button type="button" class="btn btn-secondary" @click="loadRanking">
              {{ t('common.refresh') }}
            </button>
            <button type="button" class="btn btn-secondary" @click="resetFilters">
              {{ t('common.reset') }}
            </button>
          </div>
        </div>
      </div>

      <UsageFilters
        v-model="filters"
        :start-date="startDate"
        :end-date="endDate"
        :exporting="false"
        :show-actions="false"
        @change="loadRanking"
      />

      <UserTokenLeaderboard
        :items="ranking"
        :loading="loading"
        :error="error"
        :max-rows="limit"
        @userClick="goToUserUsage"
      />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { adminAPI } from '@/api/admin'
import type { AdminUsageQueryParams } from '@/api/admin/usage'
import type { UserBreakdownItem } from '@/types'
import { requestTypeToLegacyStream } from '@/utils/usageRequestType'
import AppLayout from '@/components/layout/AppLayout.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import Select from '@/components/common/Select.vue'
import UsageFilters from '@/components/admin/usage/UsageFilters.vue'
import UserTokenLeaderboard from '@/components/admin/usage/UserTokenLeaderboard.vue'

const { t } = useI18n()
const router = useRouter()

const formatLocalDate = (date: Date) => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

const getLast24HoursRangeDates = (): { start: string; end: string } => {
  const end = new Date()
  const start = new Date(end.getTime() - 24 * 60 * 60 * 1000)
  return {
    start: formatLocalDate(start),
    end: formatLocalDate(end)
  }
}

const defaultRange = getLast24HoursRangeDates()
const startDate = ref(defaultRange.start)
const endDate = ref(defaultRange.end)
const filters = ref<AdminUsageQueryParams>({
  start_date: startDate.value,
  end_date: endDate.value,
  request_type: undefined,
  billing_type: null,
  billing_mode: undefined
})
const limit = ref(10)
const ranking = ref<UserBreakdownItem[]>([])
const loading = ref(false)
const error = ref(false)
let requestSeq = 0

const limitOptions = computed(() => [
  { value: 10, label: '10' },
  { value: 20, label: '20' },
  { value: 50, label: '50' },
  { value: 100, label: '100' }
])

const onDateRangeChange = (range: { startDate: string; endDate: string; preset: string | null }) => {
  startDate.value = range.startDate
  endDate.value = range.endDate
  filters.value = {
    ...filters.value,
    start_date: range.startDate,
    end_date: range.endDate
  }
  loadRanking()
}

const loadRanking = async () => {
  const seq = ++requestSeq
  loading.value = true
  error.value = false
  try {
    const requestType = filters.value.request_type
    const legacyStream = requestType ? requestTypeToLegacyStream(requestType) : filters.value.stream
    const response = await adminAPI.dashboard.getUserBreakdown({
      start_date: filters.value.start_date || startDate.value,
      end_date: filters.value.end_date || endDate.value,
      user_id: filters.value.user_id,
      model: filters.value.model,
      api_key_id: filters.value.api_key_id,
      account_id: filters.value.account_id,
      group_id: filters.value.group_id,
      request_type: requestType,
      stream: legacyStream === null ? undefined : legacyStream,
      billing_type: filters.value.billing_type,
      billing_mode: filters.value.billing_mode,
      sort_by: 'tokens',
      limit: limit.value
    })
    if (seq !== requestSeq) return
    ranking.value = response.users || []
  } catch (err) {
    if (seq !== requestSeq) return
    console.error('Failed to load user token ranking:', err)
    ranking.value = []
    error.value = true
  } finally {
    if (seq === requestSeq) loading.value = false
  }
}

const resetFilters = () => {
  const range = getLast24HoursRangeDates()
  startDate.value = range.start
  endDate.value = range.end
  filters.value = {
    start_date: startDate.value,
    end_date: endDate.value,
    request_type: undefined,
    billing_type: null,
    billing_mode: undefined
  }
  limit.value = 10
  loadRanking()
}

const goToUserUsage = (userId: number) => {
  const query: Record<string, string> = {
    user_id: String(userId),
    start_date: startDate.value,
    end_date: endDate.value
  }
  if (filters.value.api_key_id) query.api_key_id = String(filters.value.api_key_id)
  if (filters.value.account_id) query.account_id = String(filters.value.account_id)
  if (filters.value.group_id) query.group_id = String(filters.value.group_id)
  if (filters.value.model) query.model = filters.value.model
  if (filters.value.request_type) query.request_type = filters.value.request_type
  if (filters.value.billing_type != null) query.billing_type = String(filters.value.billing_type)
  if (filters.value.billing_mode) query.billing_mode = filters.value.billing_mode
  router.push({ path: '/admin/usage', query })
}

onMounted(() => {
  loadRanking()
})
</script>
