<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap-reverse items-start justify-between gap-3">
          <!-- Left: Filters -->
          <div class="flex flex-wrap items-center gap-3">
            <div class="flex items-center gap-2">
              <span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.accountStats.timeRange') }}:</span>
              <DateRangePicker
                v-model:start-date="startDate"
                v-model:end-date="endDate"
                @change="onDateRangeChange"
              />
            </div>
            <Select :model-value="platformFilter" class="w-40" :options="platformOptions" @update:model-value="updatePlatform" @change="handleFilterChange" />
            <Select :model-value="groupFilter" class="w-40" :options="groupOptions" @update:model-value="updateGroup" @change="handleFilterChange" />
          </div>
          <!-- Right: Actions -->
          <div class="flex items-center gap-2">
            <!-- Auto Refresh Dropdown -->
            <div class="relative" ref="autoRefreshDropdownRef">
              <button
                @click="showAutoRefreshDropdown = !showAutoRefreshDropdown"
                class="btn btn-secondary px-2 md:px-3"
                :title="t('admin.accountStats.autoRefresh')"
              >
                <Icon name="refresh" size="sm" :class="[autoRefreshEnabled ? 'animate-spin' : '']" />
                <span class="hidden md:inline">
                  {{
                    autoRefreshEnabled
                      ? t('admin.accountStats.autoRefreshCountdown', { seconds: autoRefreshCountdown })
                      : t('admin.accountStats.autoRefresh')
                  }}
                </span>
              </button>
              <div
                v-if="showAutoRefreshDropdown"
                class="absolute right-0 z-50 mt-2 w-56 origin-top-right rounded-lg border border-gray-200 bg-white shadow-lg dark:border-gray-700 dark:bg-gray-800"
              >
                <div class="p-2">
                  <button
                    @click="setAutoRefreshEnabled(!autoRefreshEnabled)"
                    class="flex w-full items-center justify-between rounded-md px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-gray-700"
                  >
                    <span>{{ t('admin.accountStats.enableAutoRefresh') }}</span>
                    <Icon v-if="autoRefreshEnabled" name="check" size="sm" class="text-primary-500" />
                  </button>
                  <div class="my-1 border-t border-gray-100 dark:border-gray-700"></div>
                  <button
                    v-for="sec in autoRefreshIntervals"
                    :key="sec"
                    @click="setAutoRefreshInterval(sec)"
                    class="flex w-full items-center justify-between rounded-md px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-gray-700"
                  >
                    <span>{{ autoRefreshIntervalLabel(sec) }}</span>
                    <Icon v-if="autoRefreshIntervalSeconds === sec" name="check" size="sm" class="text-primary-500" />
                  </button>
                </div>
              </div>
            </div>
            <!-- Manual Refresh -->
            <button @click="handleManualRefresh" class="btn btn-secondary px-2 md:px-3" :disabled="loading">
              <Icon name="refresh" size="sm" />
              <span class="hidden md:inline">{{ t('common.refresh') }}</span>
            </button>
          </div>
        </div>
      </template>
      <template #table>
        <DataTable
          :columns="columns"
          :data="accounts"
          :loading="loading"
          row-key="id"
          default-sort-key="name"
          default-sort-order="asc"
          sort-storage-key="account-stats-table-sort"
          :estimate-row-height="56"
        >
          <template #cell-name="{ row }">
            <div class="flex flex-col">
              <span class="font-medium text-gray-900 dark:text-white">{{ row.name }}</span>
              <span
                v-if="row.extra?.email_address"
                class="text-xs text-gray-500 dark:text-gray-400 truncate max-w-[200px]"
                :title="row.extra.email_address"
              >{{ row.extra.email_address }}</span>
            </div>
          </template>
          <template #cell-platform="{ row }">
            <PlatformTypeBadge :platform="row.platform" :type="row.type" />
          </template>
          <template #cell-capacity="{ row }">
            <AccountCapacityCell :account="row" />
          </template>
          <template #cell-stats_requests="{ row }">
            <span class="tabular-nums">{{ row.stats_requests ?? '-' }}</span>
          </template>
          <template #cell-stats_tokens="{ row }">
            <span class="tabular-nums">{{ formatTokens(row.stats_tokens) }}</span>
          </template>
          <template #cell-stats_account_cost="{ row }">
            <span class="tabular-nums">{{ formatCost(row.stats_account_cost) }}</span>
          </template>
          <template #cell-actions="{ row }">
            <button
              @click="openDetail(row)"
              class="text-primary-600 hover:text-primary-800 dark:text-primary-400 dark:hover:text-primary-300 text-sm font-medium"
            >
              {{ t('admin.accountStats.viewDetail') }}
            </button>
          </template>
        </DataTable>
        <Pagination
          v-if="totalAccounts > 0"
          :page="page"
          :total="totalAccounts"
          :page-size="pageSize"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>

    <!-- Detail Modal -->
    <div v-if="showDetail && selectedAccount" class="fixed inset-0 z-50 flex items-center justify-center bg-black/50" @click.self="showDetail = false">
      <div class="mx-4 max-h-[90vh] w-full max-w-5xl overflow-y-auto rounded-xl bg-white p-6 shadow-xl dark:bg-dark-800">
        <div class="mb-4 flex items-center justify-between">
          <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
            {{ t('admin.accountStats.accountDetail') }} - {{ selectedAccount.name }}
          </h3>
          <button @click="showDetail = false" class="rounded-md p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-dark-700 dark:hover:text-gray-300">
            <Icon name="x" size="md" />
          </button>
        </div>

        <!-- Usage Summary Cards -->
        <div class="mb-6 grid grid-cols-2 gap-3 md:grid-cols-4">
          <div class="rounded-lg border border-gray-100 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-700/50">
            <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.accountStats.totalRequests') }}</div>
            <div class="mt-1 text-lg font-bold text-gray-900 dark:text-white tabular-nums">{{ detailStats?.total_requests ?? 0 }}</div>
          </div>
          <div class="rounded-lg border border-gray-100 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-700/50">
            <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.accountStats.inputTokens') }}</div>
            <div class="mt-1 text-lg font-bold text-gray-900 dark:text-white tabular-nums">{{ formatTokens(detailStats?.total_input_tokens) }}</div>
          </div>
          <div class="rounded-lg border border-gray-100 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-700/50">
            <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.accountStats.outputTokens') }}</div>
            <div class="mt-1 text-lg font-bold text-gray-900 dark:text-white tabular-nums">{{ formatTokens(detailStats?.total_output_tokens) }}</div>
          </div>
          <div class="rounded-lg border border-gray-100 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-700/50">
            <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.accountStats.accountBilling') }}</div>
            <div class="mt-1 text-lg font-bold text-gray-900 dark:text-white tabular-nums">{{ formatCost(detailStats?.total_account_cost ?? detailStats?.total_cost) }}</div>
          </div>
        </div>

        <!-- Recent Users -->
        <div class="mb-6">
          <h4 class="mb-3 text-sm font-semibold text-gray-700 dark:text-gray-300">{{ t('admin.accountStats.recentUsers') }}</h4>
          <div v-if="detailUsersLoading" class="flex items-center justify-center py-6 text-sm text-gray-500">
            <Icon name="refresh" size="sm" class="mr-2 animate-spin" /> {{ t('common.loading') }}
          </div>
          <div v-else-if="recentUsers.length === 0" class="rounded-lg border border-dashed border-gray-200 py-6 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
            {{ t('admin.accountStats.noRecentUsers') }}
          </div>
          <div v-else class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-700">
            <table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
              <thead class="bg-gray-50 dark:bg-dark-800">
                <tr>
                  <th class="px-3 py-2 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.accountStats.user') }}</th>
                  <th class="px-3 py-2 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.accountStats.email') }}</th>
                  <th class="px-3 py-2 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.accountStats.currentRequests') }}</th>
                  <th class="px-3 py-2 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.accountStats.accountBilling') }}</th>
                  <th class="px-3 py-2 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.accountStats.userCharge') }}</th>
                  <th class="px-3 py-2 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.accountStats.lastUsedAt') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-200 bg-white dark:divide-dark-700 dark:bg-dark-900">
                <tr
                  v-for="user in recentUsers"
                  :key="user.user_id"
                  :class="isUserActive(user) ? 'bg-green-50 dark:bg-green-900/10' : 'hover:bg-gray-50 dark:hover:bg-dark-800/50'"
                  class="transition-colors"
                >
                  <td class="whitespace-nowrap px-3 py-2.5 text-sm text-gray-900 dark:text-white">
                    <div class="flex items-center gap-1.5">
                      <span v-if="isUserActive(user)" class="inline-block h-2 w-2 rounded-full bg-green-500 animate-pulse"></span>
                      <span class="font-medium">#{{ user.user_id }}</span>
                    </div>
                  </td>
                  <td class="whitespace-nowrap px-3 py-2.5 text-sm text-gray-600 dark:text-gray-300">{{ user.email || '-' }}</td>
                  <td class="whitespace-nowrap px-3 py-2.5 text-right text-sm tabular-nums text-gray-700 dark:text-gray-300">{{ currentRequestsForUser(user.user_id) }}</td>
                  <td class="whitespace-nowrap px-3 py-2.5 text-right text-sm tabular-nums text-gray-700 dark:text-gray-300">{{ formatCost(user.account_cost) }}</td>
                  <td class="whitespace-nowrap px-3 py-2.5 text-right text-sm tabular-nums text-gray-700 dark:text-gray-300">{{ formatCost(user.user_cost) }}</td>
                  <td class="whitespace-nowrap px-3 py-2.5 text-right text-sm text-gray-500 dark:text-gray-400">{{ formatRelativeTime(user.last_used_at) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <!-- Range Users -->
        <div>
          <h4 class="mb-3 text-sm font-semibold text-gray-700 dark:text-gray-300">{{ t('admin.accountStats.rangeUsers') }}</h4>
          <div v-if="detailUsersLoading" class="flex items-center justify-center py-6 text-sm text-gray-500">
            <Icon name="refresh" size="sm" class="mr-2 animate-spin" /> {{ t('common.loading') }}
          </div>
          <div v-else-if="rangeUsers.length === 0" class="rounded-lg border border-dashed border-gray-200 py-6 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
            {{ t('admin.accountStats.noRangeUsers') }}
          </div>
          <div v-else class="overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-700">
            <table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
              <thead class="bg-gray-50 dark:bg-dark-800">
                <tr>
                  <th class="px-3 py-2 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.accountStats.user') }}</th>
                  <th class="px-3 py-2 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.accountStats.email') }}</th>
                  <th class="px-3 py-2 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.accountStats.requestCount') }}</th>
                  <th class="px-3 py-2 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.accountStats.accountBilling') }}</th>
                  <th class="px-3 py-2 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.accountStats.userCharge') }}</th>
                  <th class="px-3 py-2 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.accountStats.lastUsedAt') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-200 bg-white dark:divide-dark-700 dark:bg-dark-900">
                <tr v-for="user in rangeUsers" :key="user.user_id" class="transition-colors hover:bg-gray-50 dark:hover:bg-dark-800/50">
                  <td class="whitespace-nowrap px-3 py-2.5 text-sm text-gray-900 dark:text-white"><span class="font-medium">#{{ user.user_id }}</span></td>
                  <td class="whitespace-nowrap px-3 py-2.5 text-sm text-gray-600 dark:text-gray-300">{{ user.email || '-' }}</td>
                  <td class="whitespace-nowrap px-3 py-2.5 text-right text-sm tabular-nums text-gray-700 dark:text-gray-300">{{ user.requests }}</td>
                  <td class="whitespace-nowrap px-3 py-2.5 text-right text-sm tabular-nums text-gray-700 dark:text-gray-300">{{ formatCost(user.account_cost) }}</td>
                  <td class="whitespace-nowrap px-3 py-2.5 text-right text-sm tabular-nums text-gray-700 dark:text-gray-300">{{ formatCost(user.user_cost) }}</td>
                  <td class="whitespace-nowrap px-3 py-2.5 text-right text-sm text-gray-500 dark:text-gray-400">{{ formatRelativeTime(user.last_used_at) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useIntervalFn } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { RecentAccountUser } from '@/api/admin/accounts'
import type { AdminGroup } from '@/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import PlatformTypeBadge from '@/components/common/PlatformTypeBadge.vue'
import AccountCapacityCell from '@/components/account/AccountCapacityCell.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatRelativeTime } from '@/utils/format'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'

const { t } = useI18n()

// State
const loading = ref(false)
const accounts = ref<any[]>([])
const totalAccounts = ref(0)
const page = ref(1)
const pageSize = ref(getPersistedPageSize())
const platformFilter = ref<string | number | boolean | null>('')
const groupFilter = ref<string | number | boolean | null>('')
const groups = ref<AdminGroup[]>([])
const accountStats = ref<Record<number, any>>({})

// Time range (default: last 24 hours via DateRangePicker)
const formatDateToString = (date: Date): string => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}
const now = new Date()
const yesterday = new Date(now.getTime() - 24 * 60 * 60 * 1000)
const startDate = ref(formatDateToString(yesterday))
const endDate = ref(formatDateToString(now))

// Detail modal
const showDetail = ref(false)
const selectedAccount = ref<any>(null)
const detailStats = ref<any>(null)
const rangeUsers = ref<RecentAccountUser[]>([])
const recentUsers = ref<RecentAccountUser[]>([])
const detailUsersLoading = ref(false)
const userConcurrencyByID = ref<Record<number, number>>({})

// Auto refresh
const autoRefreshDropdownRef = ref<HTMLElement | null>(null)
const showAutoRefreshDropdown = ref(false)
const AUTO_REFRESH_STORAGE_KEY = 'account-stats-auto-refresh'
const autoRefreshIntervals = [5, 10, 15, 30] as const
const autoRefreshEnabled = ref(false)
const autoRefreshIntervalSeconds = ref<(typeof autoRefreshIntervals)[number]>(30)
const autoRefreshCountdown = ref(0)
const autoRefreshFetching = ref(false)

// Filter options
const platformOptions = computed(() => [
  { value: '', label: t('admin.accountStats.allPlatforms') },
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'openai', label: 'OpenAI' },
  { value: 'gemini', label: 'Gemini' },
  { value: 'antigravity', label: 'Antigravity' }
])

const groupOptions = computed(() => [
  { value: '', label: t('admin.accountStats.allGroups') },
  { value: 'ungrouped', label: t('admin.accountStats.ungroupedGroup') },
  ...groups.value.map(g => ({ value: String(g.id), label: g.name }))
])

// Table columns
const columns = computed(() => [
  { key: 'name', label: t('admin.accountStats.account'), width: '200px', sortable: true },
  { key: 'platform', label: t('admin.accountStats.platform'), width: '120px' },
  { key: 'capacity', label: t('admin.accountStats.capacity'), width: '120px', align: 'center' as const },
  { key: 'stats_requests', label: t('admin.accountStats.requests'), width: '100px', align: 'right' as const, sortable: true },
  { key: 'stats_tokens', label: t('admin.accountStats.tokens'), width: '100px', align: 'right' as const, sortable: true },
  { key: 'stats_account_cost', label: t('admin.accountStats.accountBilling'), width: '120px', align: 'right' as const, sortable: true },
  { key: 'actions', label: t('admin.accountStats.actions'), width: '100px', align: 'center' as const }
])

const updatePlatform = (value: string | number | boolean | null) => { platformFilter.value = value }
const updateGroup = (value: string | number | boolean | null) => { groupFilter.value = value }

const autoRefreshIntervalLabel = (sec: number) => {
  if (sec === 5) return t('admin.accountStats.refreshInterval5s')
  if (sec === 10) return t('admin.accountStats.refreshInterval10s')
  if (sec === 15) return t('admin.accountStats.refreshInterval15s')
  if (sec === 30) return t('admin.accountStats.refreshInterval30s')
  return `${sec}s`
}

const setAutoRefreshEnabled = (enabled: boolean) => {
  autoRefreshEnabled.value = enabled
  saveAutoRefreshToStorage()
  if (enabled) {
    autoRefreshCountdown.value = autoRefreshIntervalSeconds.value
    resumeAutoRefresh()
  } else {
    pauseAutoRefresh()
    autoRefreshCountdown.value = 0
  }
}

const setAutoRefreshInterval = (seconds: (typeof autoRefreshIntervals)[number]) => {
  autoRefreshIntervalSeconds.value = seconds
  saveAutoRefreshToStorage()
  if (autoRefreshEnabled.value) {
    autoRefreshCountdown.value = seconds
  }
}

const { pause: pauseAutoRefresh, resume: resumeAutoRefresh } = useIntervalFn(
  async () => {
    if (!autoRefreshEnabled.value) return
    if (document.hidden) return
    if (loading.value || autoRefreshFetching.value) return

    if (autoRefreshCountdown.value <= 0) {
      autoRefreshCountdown.value = autoRefreshIntervalSeconds.value
      await refreshSilently()
      return
    }

    autoRefreshCountdown.value -= 1
  },
  1000,
  { immediate: false }
)

const loadSavedAutoRefresh = () => {
  try {
    const saved = localStorage.getItem(AUTO_REFRESH_STORAGE_KEY)
    if (!saved) return
    const parsed = JSON.parse(saved) as { enabled?: boolean; interval_seconds?: number }
    autoRefreshEnabled.value = parsed.enabled === true
    const interval = Number(parsed.interval_seconds)
    if (autoRefreshIntervals.includes(interval as any)) {
      autoRefreshIntervalSeconds.value = interval as any
    }
    if (autoRefreshEnabled.value) {
      autoRefreshCountdown.value = autoRefreshIntervalSeconds.value
    }
  } catch (e) {
    console.error('Failed to load saved auto refresh settings:', e)
  }
}

const saveAutoRefreshToStorage = () => {
  try {
    localStorage.setItem(AUTO_REFRESH_STORAGE_KEY, JSON.stringify({
      enabled: autoRefreshEnabled.value,
      interval_seconds: autoRefreshIntervalSeconds.value
    }))
  } catch (e) {
    console.error('Failed to save auto refresh settings:', e)
  }
}

// Close dropdown on outside click
const handleClickOutside = (e: MouseEvent) => {
  if (autoRefreshDropdownRef.value && !autoRefreshDropdownRef.value.contains(e.target as Node)) {
    showAutoRefreshDropdown.value = false
  }
}

function onDateRangeChange() {
  page.value = 1
  loadData()
}

function handleFilterChange() {
  page.value = 1
  loadData()
}

function handleManualRefresh() {
  loadData()
}

onMounted(async () => {
  document.addEventListener('click', handleClickOutside)
  loadSavedAutoRefresh()
  // Load groups
  try {
    groups.value = await adminAPI.groups.getAll()
  } catch (err) {
    console.error('Failed to load groups:', err)
  }
  loadData()
  if (autoRefreshEnabled.value) resumeAutoRefresh()
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
  pauseAutoRefresh()
})

// Load data
async function loadData(options?: { silent?: boolean }) {
  if (!options?.silent) loading.value = true
  try {
    // Build filters
    const filters: Record<string, string> = {}
    if (platformFilter.value) filters.platform = String(platformFilter.value)
    if (groupFilter.value) filters.group = String(groupFilter.value)

    const result = await adminAPI.accounts.list(page.value, pageSize.value, filters)
    accounts.value = result.items || []
    totalAccounts.value = result.total || 0

    // Load concurrency data
    await loadConcurrency()

    // Load stats for each account
    await loadAccountStats()

    applyStatsToAccounts()
  } catch (err) {
    console.error('Failed to load account stats:', err)
  } finally {
    if (!options?.silent) loading.value = false
  }
}

async function refreshSilently() {
  if (autoRefreshFetching.value) return
  autoRefreshFetching.value = true
  try {
    await loadData({ silent: true })
    if (showDetail.value && selectedAccount.value) {
      const refreshedAccount = accounts.value.find(a => a.id === selectedAccount.value.id)
      if (refreshedAccount) selectedAccount.value = refreshedAccount
      detailStats.value = accountStats.value[selectedAccount.value.id] || detailStats.value
      await loadDetailUsers(selectedAccount.value.id, { silent: true })
    }
  } finally {
    autoRefreshFetching.value = false
  }
}

async function loadConcurrency() {
  try {
    const data = await adminAPI.ops.getConcurrencyStats()
    const concurrencyMap = data.account || {}
    for (const account of accounts.value) {
      const key = String(account.id)
      if (concurrencyMap[key]) {
        account.current_concurrency = concurrencyMap[key].current_in_use || 0
      } else {
        account.current_concurrency = account.current_concurrency || 0
      }
    }
  } catch (err) {
    console.error('Failed to load concurrency:', err)
  }
}

async function loadAccountStats() {
  const stats: Record<number, any> = {}
  const promises = accounts.value.map(async (account) => {
    try {
      const result = await adminAPI.usage.getStats({
        account_id: account.id,
        start_date: startDate.value,
        end_date: endDate.value
      })
      stats[account.id] = result
    } catch {
      // Ignore individual failures
    }
  })
  await Promise.all(promises)
  accountStats.value = stats
}

function applyStatsToAccounts() {
  accounts.value = accounts.value.map((account) => {
    const stats = accountStats.value[account.id]
    return {
      ...account,
      stats_requests: stats?.total_requests ?? 0,
      stats_tokens: stats?.total_tokens ?? 0,
      stats_account_cost: stats?.total_account_cost ?? stats?.total_cost ?? 0,
      stats_user_cost: stats?.total_actual_cost ?? 0,
    }
  })
}

function handlePageChange(newPage: number) {
  page.value = newPage
  loadData()
}

function handlePageSizeChange(newSize: number) {
  pageSize.value = newSize
  page.value = 1
  loadData()
}

// Detail modal
async function openDetail(account: any) {
  selectedAccount.value = account
  showDetail.value = true
  detailStats.value = accountStats.value[account.id] || null
  rangeUsers.value = []
  recentUsers.value = []
  await loadDetailUsers(account.id)
}

async function loadDetailUsers(accountId: number, options?: { silent?: boolean }) {
  if (!options?.silent) detailUsersLoading.value = true
  try {
    const [rangeResult, recentResult, userConcurrencyResult] = await Promise.all([
      adminAPI.accounts.getRecentUsers(accountId, {
        start_date: startDate.value,
        end_date: endDate.value,
      }),
      adminAPI.accounts.getRecentUsers(accountId),
      adminAPI.ops.getUserConcurrencyStats()
    ])
    rangeUsers.value = rangeResult.users || []
    recentUsers.value = recentResult.users || []
    const nextConcurrency: Record<number, number> = {}
    for (const [userID, info] of Object.entries(userConcurrencyResult.user || {})) {
      nextConcurrency[Number(userID)] = info.current_in_use || 0
    }
    userConcurrencyByID.value = nextConcurrency
  } catch (err) {
    console.error('Failed to load account users:', err)
  } finally {
    if (!options?.silent) detailUsersLoading.value = false
  }
}

function currentRequestsForUser(userID: number): number {
  return userConcurrencyByID.value[userID] || 0
}

function isUserActive(user: RecentAccountUser): boolean {
  const lastUsed = new Date(user.last_used_at)
  const oneMinuteAgo = new Date(Date.now() - 60 * 1000)
  return lastUsed > oneMinuteAgo
}

// Formatting helpers
function formatTokens(tokens: number | undefined | null): string {
  if (tokens == null) return '-'
  if (tokens >= 1_000_000) return (tokens / 1_000_000).toFixed(1) + 'M'
  if (tokens >= 1_000) return (tokens / 1_000).toFixed(1) + 'K'
  return String(tokens)
}

function formatCost(cost: number | undefined | null): string {
  if (cost == null) return '-'
  return '$' + cost.toFixed(4)
}
</script>
