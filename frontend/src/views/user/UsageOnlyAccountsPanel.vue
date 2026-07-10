<template>
  <section class="usage-only-accounts flex min-h-[260px] flex-col rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
    <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 bg-emerald-50/70 px-4 py-3 dark:border-dark-700 dark:bg-emerald-900/10 sm:px-5">
      <div>
        <h2 class="text-sm font-semibold text-gray-900 dark:text-white">账号</h2>
      </div>
      <div class="flex items-center gap-2">
        <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="reload">
          <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
          <span>刷新</span>
        </button>
      </div>
    </div>

    <div class="flex-1 overflow-hidden">
      <DataTable
        :columns="columns"
        :data="accounts"
        :loading="loading"
        row-key="id"
        :server-side-sort="true"
        default-sort-key="name"
        default-sort-order="asc"
        @sort="handleSort"
      >
        <template #cell-name="{ row, value }">
          <div class="flex min-w-[220px] flex-col gap-1">
            <span class="font-medium text-gray-900 dark:text-white">{{ value }}</span>
            <span
              v-if="accountEmail(row)"
              class="max-w-[260px] truncate text-xs text-gray-500 dark:text-gray-400"
              :title="accountEmail(row)"
            >
              {{ accountEmail(row) }}
            </span>
          </div>
        </template>

        <template #cell-platform_type="{ row }">
          <PlatformTypeBadge
            :platform="row.platform"
            :type="row.type"
            :plan-type="planType(row)"
            :privacy-mode="privacyMode(row)"
            :subscription-expires-at="subscriptionExpiresAt(row)"
          />
        </template>

        <template #cell-status="{ row }">
          <AccountStatusIndicator :account="row" :allow-temp-unsched-details="false" />
        </template>

        <template #cell-today_stats="{ row }">
          <AccountTodayStatsCell
            :stats="todayStatsByAccountId[String(row.id)] ?? null"
            :loading="todayStatsLoading"
            :error="todayStatsError"
          />
        </template>

        <template #cell-usage="{ row }">
          <AccountUsageCell
            :account="row"
            :today-stats="todayStatsByAccountId[String(row.id)] ?? null"
            :today-stats-loading="todayStatsLoading"
            :manual-refresh-token="usageManualRefreshToken"
            :show-active-query="false"
          />
        </template>

        <template #cell-last_used_at="{ value }">
          <span class="text-sm text-gray-500 dark:text-dark-400">{{ formatRelativeTime(value) }}</span>
        </template>

        <template #empty>
          <EmptyState message="暂无账号数据" />
        </template>
      </DataTable>
    </div>

    <Pagination
      v-if="pagination.total > 0"
      :page="pagination.page"
      :total="pagination.total"
      :page-size="pagination.page_size"
      @update:page="handlePageChange"
      @update:pageSize="handlePageSizeChange"
    />
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api'
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformTypeBadge from '@/components/common/PlatformTypeBadge.vue'
import AccountStatusIndicator from '@/components/account/AccountStatusIndicator.vue'
import AccountTodayStatsCell from '@/components/account/AccountTodayStatsCell.vue'
import AccountUsageCell from '@/components/account/AccountUsageCell.vue'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import { formatRelativeTime } from '@/utils/format'
import type { Account, WindowStats } from '@/types'
import type { Column } from '@/components/common/types'

const { t } = useI18n()

const columns = computed<Column[]>(() => [
  { key: 'name', label: t('admin.accounts.columns.name'), sortable: true },
  { key: 'platform_type', label: t('admin.accounts.columns.platformType'), sortable: false },
  { key: 'status', label: t('admin.accounts.columns.status'), sortable: true },
  { key: 'today_stats', label: t('admin.accounts.columns.todayStats'), sortable: false },
  { key: 'usage', label: t('admin.accounts.columns.usageWindows'), sortable: false },
  { key: 'last_used_at', label: t('admin.accounts.columns.lastUsed'), sortable: true }
])

const accounts = ref<Account[]>([])
const loading = ref(false)
const abortController = ref<AbortController | null>(null)
const usageManualRefreshToken = ref(0)
const todayStatsByAccountId = ref<Record<string, WindowStats>>({})
const todayStatsLoading = ref(false)
const todayStatsError = ref<string | null>(null)
const todayStatsReqSeq = ref(0)

const pagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0,
  pages: 0
})

const sortState = reactive({
  sort_by: 'name',
  sort_order: 'asc' as 'asc' | 'desc'
})

const buildDefaultTodayStats = (): WindowStats => ({
  requests: 0,
  tokens: 0,
  cost: 0,
  standard_cost: 0,
  user_cost: 0
})

const loadTodayStats = async () => {
  const accountIDs = accounts.value.map(account => account.id)
  const reqSeq = ++todayStatsReqSeq.value
  if (accountIDs.length === 0) {
    todayStatsByAccountId.value = {}
    todayStatsError.value = null
    return
  }

  todayStatsLoading.value = true
  todayStatsError.value = null
  try {
    const result = await adminAPI.accounts.getBatchTodayStats(accountIDs)
    if (reqSeq !== todayStatsReqSeq.value) return
    const nextStats: Record<string, WindowStats> = {}
    for (const accountID of accountIDs) {
      const key = String(accountID)
      nextStats[key] = result.stats?.[key] ?? buildDefaultTodayStats()
    }
    todayStatsByAccountId.value = nextStats
  } catch (error) {
    if (reqSeq !== todayStatsReqSeq.value) return
    todayStatsError.value = 'Failed'
    console.error('Failed to load account today stats:', error)
  } finally {
    if (reqSeq === todayStatsReqSeq.value) {
      todayStatsLoading.value = false
    }
  }
}

const loadAccounts = async () => {
  abortController.value?.abort()
  const controller = new AbortController()
  abortController.value = controller
  loading.value = true

  try {
    const response = await adminAPI.accounts.list(
      pagination.page,
      pagination.page_size,
      {
        sort_by: sortState.sort_by,
        sort_order: sortState.sort_order
      },
      { signal: controller.signal }
    )
    if (controller.signal.aborted) return
    accounts.value = response.items
    pagination.total = response.total
    pagination.pages = response.pages
    await loadTodayStats()
  } catch (error) {
    const abortError = error as { name?: string; code?: string }
    if (abortError?.name === 'AbortError' || abortError?.code === 'ERR_CANCELED') return
    console.error('Failed to load accounts:', error)
  } finally {
    if (abortController.value === controller) {
      loading.value = false
      abortController.value = null
    }
  }
}

const reload = () => {
  usageManualRefreshToken.value += 1
  loadAccounts()
}

const handlePageChange = (page: number) => {
  pagination.page = page
  loadAccounts()
}

const handlePageSizeChange = (pageSize: number) => {
  pagination.page_size = pageSize
  pagination.page = 1
  loadAccounts()
}

const handleSort = (key: string, order: 'asc' | 'desc') => {
  sortState.sort_by = key
  sortState.sort_order = order
  pagination.page = 1
  loadAccounts()
}

const stringField = (value: unknown): string => {
  return typeof value === 'string' ? value : ''
}

const accountEmail = (account: Account): string => {
  return (
    stringField(account.display_email) ||
    stringField(account.extra?.email_address) ||
    stringField(account.extra?.email) ||
    stringField(account.credentials?.email)
  )
}

const planType = (account: Account): string | undefined => {
  return stringField(account.credentials?.plan_type) || undefined
}

const privacyMode = (account: Account): string | undefined => {
  return stringField(account.extra?.privacy_mode) || undefined
}

const subscriptionExpiresAt = (account: Account): string | undefined => {
  return stringField(account.credentials?.subscription_expires_at) || undefined
}

onMounted(() => {
  loadAccounts()
})
</script>

<style scoped>
.usage-only-accounts :deep(.table-wrapper) {
  height: 100%;
  overflow: auto;
}

.usage-only-accounts :deep(thead) {
  @apply bg-gray-50/80 dark:bg-dark-800/80;
}

.usage-only-accounts :deep(th) {
  @apply px-5 py-4 text-left text-sm font-medium text-gray-600 dark:text-dark-300;
}

.usage-only-accounts :deep(td) {
  @apply px-5 py-4 text-sm text-gray-700 dark:text-gray-300;
}
</style>
