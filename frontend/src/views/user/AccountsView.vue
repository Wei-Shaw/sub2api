<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="mb-4 rounded-md border border-gray-200 bg-white p-1 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="flex gap-1 overflow-x-auto pb-px" role="tablist" :aria-label="t('admin.accounts.groupTabsLabel')">
            <button type="button" role="tab" :aria-selected="filters.group === allGroupsTab.value" :class="groupTabClass(allGroupsTab.value)" @click="selectGroup(allGroupsTab.value)">
              {{ allGroupsTab.label }}
            </button>

            <VueDraggable
              v-model="orderedGroupTabs"
              tag="div"
              class="flex shrink-0 gap-1"
              :animation="180"
              :delay="250"
              :delay-on-touch-only="false"
              :touch-start-threshold="3"
              :fallback-tolerance="4"
              :force-fallback="true"
              direction="horizontal"
              handle=".visible-account-group-drag-handle"
              ghost-class="opacity-40"
              chosen-class="cursor-grabbing"
              @start="handleGroupDragStart"
              @end="handleGroupDragEnd"
            >
              <button
                v-for="tab in orderedGroupTabs"
                :key="tab.value"
                type="button"
                role="tab"
                :aria-selected="filters.group === tab.value"
                :class="[groupTabClass(tab.value), 'visible-account-group-drag-handle cursor-grab select-none touch-none active:cursor-grabbing']"
                @click="handleGroupClick(tab.value)"
              >
                <Icon name="menu" size="xs" class="mr-1 text-current/50" aria-hidden="true" />
                {{ tab.label }}
              </button>
            </VueDraggable>

            <button type="button" role="tab" :aria-selected="filters.group === ungroupedTab.value" :class="groupTabClass(ungroupedTab.value)" @click="selectGroup(ungroupedTab.value)">
              {{ ungroupedTab.label }}
            </button>
          </div>
        </div>

        <div class="flex flex-wrap-reverse items-start justify-between gap-3">
          <div class="flex flex-1 flex-wrap items-center gap-3">
            <SearchInput v-model="filters.search" class="w-full sm:w-64" :placeholder="t('visibleAccounts.searchPlaceholder')" @search="reloadFromFirstPage" />
            <Select v-model="filters.platform" class="w-40" :options="platformOptions" @change="reloadFromFirstPage" />
            <Select v-model="filters.type" class="w-40" :options="typeOptions" @change="reloadFromFirstPage" />
            <Select v-model="filters.status" class="w-44" :options="statusOptions" @change="reloadFromFirstPage" />
          </div>

          <div class="flex shrink-0 items-center gap-2">
            <button type="button" class="btn btn-secondary px-2 md:px-3" :disabled="loading" :title="t('visibleAccounts.refresh')" :aria-label="t('visibleAccounts.refresh')" @click="handleManualRefresh">
              <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
            </button>

            <div ref="autoRefreshDropdownRef" class="relative">
              <button type="button" class="btn btn-secondary px-2 md:px-3" :title="t('visibleAccounts.autoRefresh')" :aria-expanded="showAutoRefreshDropdown" @click="toggleAutoRefreshDropdown">
                <Icon name="refresh" size="sm" :class="{ 'animate-spin': autoRefreshEnabled }" />
                <span class="ml-1.5 hidden md:inline">{{ autoRefreshEnabled ? t('visibleAccounts.autoRefreshCountdown', { seconds: autoRefreshCountdown }) : t('visibleAccounts.autoRefresh') }}</span>
              </button>
              <div v-if="showAutoRefreshDropdown" class="absolute right-0 z-50 mt-2 w-56 rounded-md border border-gray-200 bg-white p-2 shadow-lg dark:border-dark-700 dark:bg-dark-800">
                <button type="button" class="visible-account-menu-item" @click="setAutoRefreshEnabled(!autoRefreshEnabled)">
                  <span>{{ t('visibleAccounts.enableAutoRefresh') }}</span>
                  <Icon v-if="autoRefreshEnabled" name="check" size="sm" class="text-primary-500" />
                </button>
                <div class="my-1 border-t border-gray-100 dark:border-dark-700" />
                <button v-for="seconds in autoRefreshIntervals" :key="seconds" type="button" class="visible-account-menu-item" @click="setAutoRefreshInterval(seconds)">
                  <span>{{ t(`visibleAccounts.refreshInterval${seconds}s`) }}</span>
                  <Icon v-if="autoRefreshIntervalSeconds === seconds" name="check" size="sm" class="text-primary-500" />
                </button>
              </div>
            </div>

            <div ref="columnDropdownRef" class="relative">
              <button type="button" class="btn btn-secondary px-2 md:px-3" :title="t('visibleAccounts.moreActions')" :aria-label="t('visibleAccounts.moreActions')" :aria-expanded="showColumnDropdown" @click="toggleColumnDropdown">
                <Icon name="more" size="sm" />
                <span class="ml-1.5 hidden md:inline">{{ t('visibleAccounts.moreActions') }}</span>
                <Icon name="chevronDown" size="xs" class="ml-1 hidden md:inline" />
              </button>
              <div v-if="showColumnDropdown" class="absolute right-0 z-50 mt-2 w-64 rounded-md border border-gray-200 bg-white p-2 shadow-lg dark:border-dark-700 dark:bg-dark-800">
                <div class="flex items-center justify-between px-3 py-2">
                  <span class="text-xs font-semibold text-gray-400 dark:text-gray-500">{{ t('visibleAccounts.viewColumns') }}</span>
                  <Icon name="grid" size="sm" class="text-gray-400" />
                </div>
                <button v-for="column in toggleableColumns" :key="column.key" type="button" class="visible-account-menu-item" @click="toggleColumn(column.key)">
                  <span class="truncate">{{ column.label }}</span>
                  <Icon v-if="isColumnVisible(column.key)" name="check" size="sm" class="text-primary-500" />
                </button>
              </div>
            </div>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="accounts"
          :loading="loading"
          row-key="id"
          :server-side-sort="true"
          :default-sort-key="filters.sort_by"
          :default-sort-order="filters.sort_order"
          :sort-storage-key="SORT_STORAGE_KEY"
          :estimate-row-height="156"
          :overscan="5"
          :virtualize-threshold="50"
          @sort="handleSort"
        >
          <template #empty>
            <div class="flex flex-col items-center py-4 text-center">
              <Icon name="inbox" size="xl" class="mb-3 text-gray-300 dark:text-dark-500" />
              <p class="font-medium text-gray-900 dark:text-gray-100">{{ t('visibleAccounts.emptyTitle') }}</p>
              <p class="mt-1 max-w-md text-sm text-gray-500 dark:text-gray-400">{{ t('visibleAccounts.emptyDescription') }}</p>
            </div>
          </template>
          <template #cell-name="{ value }"><span class="font-medium text-gray-900 dark:text-gray-100">{{ value }}</span></template>
          <template #cell-id="{ value }"><span class="font-mono text-xs text-gray-500 dark:text-gray-400">#{{ value }}</span></template>
          <template #cell-platform_type="{ row }"><PlatformTypeBadge :platform="row.platform" :type="row.type" /></template>
          <template #cell-status="{ row }"><span :class="['badge text-xs', statusMeta(row).className]">{{ statusMeta(row).label }}</span></template>
          <template #cell-schedulable="{ row }">
            <span :class="row.schedulable ? 'text-emerald-600 dark:text-emerald-400' : 'text-gray-500 dark:text-gray-400'">{{ row.schedulable ? t('visibleAccounts.schedulableYes') : t('visibleAccounts.schedulableNo') }}</span>
          </template>
          <template #cell-concurrency="{ value }"><span class="tabular-nums">{{ value }}</span></template>
          <template #cell-usage="{ row }"><AccountUsageCell :account="toUsageAccount(row)" api-base="/accounts" read-only /></template>
          <template #cell-priority="{ value }"><span class="tabular-nums">{{ value }}</span></template>
          <template #cell-groups="{ row }">
            <div v-if="row.groups.length" class="flex max-w-64 flex-wrap gap-1">
              <span v-for="group in row.groups" :key="group.id" class="max-w-36 truncate rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-600 dark:bg-dark-600 dark:text-gray-300" :title="group.name">{{ group.name }}</span>
            </div>
            <span v-else class="text-gray-400 dark:text-dark-500">-</span>
          </template>
          <template #cell-last_used_at="{ value }"><span v-if="value" :title="formatDateTime(value)">{{ formatRelativeTime(value) }}</span><span v-else class="text-gray-400 dark:text-dark-500">{{ t('visibleAccounts.never') }}</span></template>
          <template #cell-created_at="{ value }"><span :title="formatDateTime(value)">{{ formatDateTime(value) }}</span></template>
          <template #cell-expires_at="{ value }"><span v-if="value" :title="formatDateTime(new Date(value * 1000))">{{ formatRelativeTime(new Date(value * 1000)) }}</span><span v-else class="text-gray-400 dark:text-dark-500">{{ t('visibleAccounts.noExpiry') }}</span></template>
          <template #cell-actions="{ row }">
            <button type="button" class="flex flex-col items-center gap-0.5 rounded-md p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:hover:bg-dark-700 dark:hover:text-white" :aria-label="t('common.more')" @click="openActionMenu(row, $event)">
              <Icon name="more" size="sm" />
              <span class="text-xs">{{ t('common.more') }}</span>
            </button>
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination v-if="pagination.total > 0" :page="pagination.page" :total="pagination.total" :page-size="pagination.pageSize" @update:page="handlePageChange" @update:pageSize="handlePageSizeChange" />
      </template>
    </TablePageLayout>
    <AccountActionMenu
      :show="actionMenu.show"
      :account="actionMenu.account"
      :anchor-rect="actionMenu.anchorRect"
      read-only
      @close="closeActionMenu"
      @test="openTestModal"
      @stats="openStatsModal"
    />
    <AccountTestModal :show="showTest" :account="selectedAccount" api-base="/accounts" @close="closeTestModal" />
    <AccountStatsModal :show="showStats" :account="selectedAccount" api-base="/accounts" @close="closeStatsModal" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useDebounceFn, useIntervalFn } from '@vueuse/core'
import { VueDraggable } from 'vue-draggable-plus'
import { useI18n } from 'vue-i18n'
import { accountsAPI, type VisibleAccount, type VisibleAccountGroup, type VisibleAccountFilters } from '@/api/accounts'
import { useAppStore } from '@/stores/app'
import { CONCRETE_PLATFORM_OPTIONS } from '@/constants/platforms'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime, formatRelativeTime } from '@/utils/format'
import type { Account } from '@/types'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import SearchInput from '@/components/common/SearchInput.vue'
import Select from '@/components/common/Select.vue'
import PlatformTypeBadge from '@/components/common/PlatformTypeBadge.vue'
import AccountUsageCell from '@/components/account/AccountUsageCell.vue'
import Icon from '@/components/icons/Icon.vue'
import AccountActionMenu from '@/components/admin/account/AccountActionMenu.vue'
import AccountTestModal from '@/components/admin/account/AccountTestModal.vue'
import AccountStatsModal from '@/components/admin/account/AccountStatsModal.vue'

const { t } = useI18n()
const appStore = useAppStore()

const COLUMN_STORAGE_KEY = 'visible-account-hidden-columns'
const SORT_STORAGE_KEY = 'visible-account-table-sort'
const AUTO_REFRESH_STORAGE_KEY = 'visible-account-auto-refresh'
const GROUP_ORDER_STORAGE_KEY = 'visible-account-group-tab-order'
const DEFAULT_HIDDEN_COLUMNS = ['priority', 'created_at']
const SORTABLE_KEYS = new Set(['name', 'id', 'status', 'schedulable', 'priority', 'last_used_at', 'created_at', 'expires_at'])

const loadInitialSort = () => {
  const fallback = { sort_by: 'name', sort_order: 'asc' as const }
  try {
    const parsed = JSON.parse(localStorage.getItem(SORT_STORAGE_KEY) ?? 'null') as { key?: string; order?: string } | null
    if (!parsed?.key || !SORTABLE_KEYS.has(parsed.key)) return fallback
    return { sort_by: parsed.key, sort_order: parsed.order === 'desc' ? 'desc' as const : 'asc' as const }
  } catch {
    return fallback
  }
}

const initialSort = loadInitialSort()
const accounts = ref<VisibleAccount[]>([])
const groups = ref<VisibleAccountGroup[]>([])
const loading = ref(false)
const filters = reactive<Required<Pick<VisibleAccountFilters, 'platform' | 'type' | 'status' | 'group' | 'search'>> & { sort_by: string; sort_order: 'asc' | 'desc' }>({
  platform: '', type: '', status: '', group: '', search: '', sort_by: initialSort.sort_by, sort_order: initialSort.sort_order
})
const pagination = reactive({ page: 1, pageSize: 20, total: 0 })
let abortController: AbortController | null = null

type GroupTab = { value: string; label: string }
const allGroupsTab = computed<GroupTab>(() => ({ value: '', label: t('visibleAccounts.allGroups') }))
const ungroupedTab = computed<GroupTab>(() => ({ value: 'ungrouped', label: t('visibleAccounts.ungrouped') }))
const groupTabOrder = ref<number[]>(loadStoredGroupOrder())
const orderedGroupTabs = ref<GroupTab[]>([])
const suppressNextGroupClick = ref(false)

function loadStoredGroupOrder(): number[] {
  try {
    const parsed = JSON.parse(localStorage.getItem(GROUP_ORDER_STORAGE_KEY) ?? 'null')
    return Array.isArray(parsed) ? parsed.filter((id): id is number => Number.isInteger(id) && id > 0) : []
  } catch { return [] }
}
function syncOrderedGroupTabs() {
  const groupsById = new Map(groups.value.map((group) => [group.id, group]))
  const orderedIds = [...groupTabOrder.value.filter((id) => groupsById.has(id)), ...groups.value.map((group) => group.id).filter((id) => !groupTabOrder.value.includes(id))]
  orderedGroupTabs.value = orderedIds.map((id) => ({ value: String(id), label: groupsById.get(id)!.name }))
  if (groups.value.length > 0) groupTabOrder.value = orderedIds
}
function handleGroupDragStart() { suppressNextGroupClick.value = true }
function handleGroupDragEnd() {
  groupTabOrder.value = orderedGroupTabs.value.map((tab) => Number(tab.value)).filter((id) => Number.isInteger(id) && id > 0)
  try { localStorage.setItem(GROUP_ORDER_STORAGE_KEY, JSON.stringify(groupTabOrder.value)) } catch { /* Browser storage may be unavailable. */ }
  window.setTimeout(() => { suppressNextGroupClick.value = false }, 0)
}
function handleGroupClick(value: string) { if (!suppressNextGroupClick.value) selectGroup(value) }
watch(groups, syncOrderedGroupTabs, { immediate: true })

const platformOptions = computed(() => [{ value: '', label: t('visibleAccounts.allPlatforms') }, ...CONCRETE_PLATFORM_OPTIONS])
const typeOptions = computed(() => [
  { value: '', label: t('visibleAccounts.allTypes') },
  { value: 'oauth', label: t('visibleAccounts.oauth') },
  { value: 'setup-token', label: t('visibleAccounts.setupToken') },
  { value: 'apikey', label: t('visibleAccounts.apiKey') },
  { value: 'bedrock', label: 'AWS Bedrock' }
])
const statusOptions = computed(() => [
  { value: '', label: t('visibleAccounts.allStatuses') },
  { value: 'active', label: t('visibleAccounts.status.active') },
  { value: 'inactive', label: t('visibleAccounts.status.inactive') },
  { value: 'error', label: t('visibleAccounts.status.error') },
  { value: 'rate_limited', label: t('visibleAccounts.status.rateLimited') },
  { value: 'temp_unschedulable', label: t('visibleAccounts.status.tempUnschedulable') },
  { value: 'unschedulable', label: t('visibleAccounts.status.unschedulable') }
])

const hiddenColumns = reactive(new Set<string>(loadHiddenColumns()))
function loadHiddenColumns(): string[] {
  try {
    const parsed = JSON.parse(localStorage.getItem(COLUMN_STORAGE_KEY) ?? 'null')
    return Array.isArray(parsed) ? parsed.filter((key): key is string => typeof key === 'string') : DEFAULT_HIDDEN_COLUMNS
  } catch { return DEFAULT_HIDDEN_COLUMNS }
}
const allColumns = computed<Column[]>(() => [
  { key: 'name', label: t('visibleAccounts.columns.name'), sortable: true },
  { key: 'id', label: t('visibleAccounts.columns.id'), sortable: true },
  { key: 'platform_type', label: t('visibleAccounts.columns.platformType') },
  { key: 'status', label: t('visibleAccounts.columns.status'), sortable: true },
  { key: 'schedulable', label: t('visibleAccounts.columns.schedulable'), sortable: true },
  { key: 'concurrency', label: t('visibleAccounts.columns.concurrency') },
  { key: 'groups', label: t('visibleAccounts.columns.groups') },
  { key: 'usage', label: t('visibleAccounts.columns.usageWindows') },
  { key: 'priority', label: t('visibleAccounts.columns.priority'), sortable: true },
  { key: 'last_used_at', label: t('visibleAccounts.columns.lastUsed'), sortable: true },
  { key: 'created_at', label: t('visibleAccounts.columns.createdAt'), sortable: true },
  { key: 'expires_at', label: t('visibleAccounts.columns.expiresAt'), sortable: true },
  { key: 'actions', label: t('admin.accounts.columns.actions') }
])
const toggleableColumns = computed(() => allColumns.value.filter((column) => column.key !== 'name' && column.key !== 'actions'))
const columns = computed(() => allColumns.value.filter((column) => column.key === 'name' || column.key === 'actions' || !hiddenColumns.has(column.key)))
const isColumnVisible = (key: string) => !hiddenColumns.has(key)
const toggleColumn = (key: string) => {
  const willHide = !hiddenColumns.has(key)
  if (willHide) hiddenColumns.add(key)
  else hiddenColumns.delete(key)
  try { localStorage.setItem(COLUMN_STORAGE_KEY, JSON.stringify([...hiddenColumns])) } catch { /* Browser storage may be unavailable. */ }
  if (willHide && filters.sort_by === key) {
    filters.sort_by = 'name'
    filters.sort_order = 'asc'
    try { localStorage.setItem(SORT_STORAGE_KEY, JSON.stringify({ key: 'name', order: 'asc' })) } catch { /* Browser storage may be unavailable. */ }
    reloadFromFirstPage()
  }
}

const groupTabClass = (value: string) => [
  'inline-flex min-h-10 shrink-0 items-center rounded-md px-3 py-2 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500',
  filters.group === value ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300' : 'text-gray-500 hover:bg-gray-50 hover:text-gray-700 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-gray-200'
]

const statusMeta = (account: VisibleAccount) => {
  const now = Date.now()
  if (account.overload_until && new Date(account.overload_until).getTime() > now) return { label: t('visibleAccounts.status.overloaded'), className: 'badge-danger' }
  if (account.temp_unschedulable_until && new Date(account.temp_unschedulable_until).getTime() > now) return { label: t('visibleAccounts.status.tempUnschedulable'), className: 'badge-warning' }
  if (account.rate_limit_reset_at && new Date(account.rate_limit_reset_at).getTime() > now) return { label: t('visibleAccounts.status.rateLimited'), className: 'badge-warning' }
  if (account.status === 'error') return { label: t('visibleAccounts.status.error'), className: 'badge-danger' }
  if (account.status !== 'active') return { label: t('visibleAccounts.status.inactive'), className: 'badge-secondary' }
  if (!account.schedulable) return { label: t('visibleAccounts.status.unschedulable'), className: 'badge-secondary' }
  return { label: t('visibleAccounts.status.active'), className: 'badge-success' }
}
const toUsageAccount = (account: VisibleAccount) => account as unknown as Account

const autoRefreshIntervals = [5, 10, 15, 30] as const
const showAutoRefreshDropdown = ref(false)
const autoRefreshDropdownRef = ref<HTMLElement | null>(null)
const autoRefreshEnabled = ref(false)
const autoRefreshIntervalSeconds = ref<(typeof autoRefreshIntervals)[number]>(30)
const autoRefreshCountdown = ref(0)
const showColumnDropdown = ref(false)
const columnDropdownRef = ref<HTMLElement | null>(null)
const actionMenu = reactive<{ show: boolean; account: Account | null; anchorRect: DOMRect | null }>({ show: false, account: null, anchorRect: null })
const selectedAccount = ref<Account | null>(null)
const showTest = ref(false)
const showStats = ref(false)

const loadAutoRefreshSettings = () => {
  try {
    const parsed = JSON.parse(localStorage.getItem(AUTO_REFRESH_STORAGE_KEY) ?? 'null') as { enabled?: boolean; interval_seconds?: number } | null
    autoRefreshEnabled.value = parsed?.enabled === true
    const seconds = Number(parsed?.interval_seconds)
    if (autoRefreshIntervals.includes(seconds as (typeof autoRefreshIntervals)[number])) autoRefreshIntervalSeconds.value = seconds as (typeof autoRefreshIntervals)[number]
  } catch { autoRefreshEnabled.value = false }
  autoRefreshCountdown.value = autoRefreshEnabled.value ? autoRefreshIntervalSeconds.value : 0
}
const saveAutoRefreshSettings = () => {
  try { localStorage.setItem(AUTO_REFRESH_STORAGE_KEY, JSON.stringify({ enabled: autoRefreshEnabled.value, interval_seconds: autoRefreshIntervalSeconds.value })) } catch { /* Browser storage may be unavailable. */ }
}
const setAutoRefreshEnabled = (enabled: boolean) => {
  autoRefreshEnabled.value = enabled
  autoRefreshCountdown.value = enabled ? autoRefreshIntervalSeconds.value : 0
  saveAutoRefreshSettings()
  if (enabled) resumeAutoRefresh()
  else pauseAutoRefresh()
}
const setAutoRefreshInterval = (seconds: (typeof autoRefreshIntervals)[number]) => {
  autoRefreshIntervalSeconds.value = seconds
  autoRefreshCountdown.value = autoRefreshEnabled.value ? seconds : 0
  saveAutoRefreshSettings()
}

const loadAccounts = async () => {
  abortController?.abort()
  abortController = new AbortController()
  loading.value = true
  try {
    const result = await accountsAPI.list(pagination.page, pagination.pageSize, { ...filters }, { signal: abortController.signal })
    accounts.value = result.items
    pagination.total = result.total
  } catch (error: any) {
    if (error?.name !== 'CanceledError' && error?.name !== 'AbortError') appStore.showError(extractApiErrorMessage(error, t('common.error')))
  } finally { loading.value = false }
}

const reloadFromFirstPage = () => { pagination.page = 1; void loadAccounts() }
const selectGroup = (value: string) => { if (filters.group !== value) { filters.group = value; reloadFromFirstPage() } }
const handleSort = (key: string, order: 'asc' | 'desc') => { filters.sort_by = key; filters.sort_order = order; reloadFromFirstPage() }
const handlePageChange = (page: number) => { pagination.page = page; void loadAccounts() }
const handlePageSizeChange = (pageSize: number) => { pagination.pageSize = pageSize; reloadFromFirstPage() }
const handleManualRefresh = () => { autoRefreshCountdown.value = autoRefreshEnabled.value ? autoRefreshIntervalSeconds.value : 0; void loadAccounts() }
const toggleAutoRefreshDropdown = () => { showAutoRefreshDropdown.value = !showAutoRefreshDropdown.value; showColumnDropdown.value = false }
const toggleColumnDropdown = () => { showColumnDropdown.value = !showColumnDropdown.value; showAutoRefreshDropdown.value = false }
const openActionMenu = (account: VisibleAccount, event: MouseEvent) => {
  actionMenu.account = account as Account
  actionMenu.anchorRect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  actionMenu.show = true
}
const closeActionMenu = () => { actionMenu.show = false }
const openTestModal = (account: Account) => { selectedAccount.value = account; showTest.value = true }
const openStatsModal = (account: Account) => { selectedAccount.value = account; showStats.value = true }
const closeTestModal = () => { showTest.value = false; selectedAccount.value = null }
const closeStatsModal = () => { showStats.value = false; selectedAccount.value = null }
const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as Node
  if (autoRefreshDropdownRef.value && !autoRefreshDropdownRef.value.contains(target)) showAutoRefreshDropdown.value = false
  if (columnDropdownRef.value && !columnDropdownRef.value.contains(target)) showColumnDropdown.value = false
}

const { pause: pauseAutoRefresh, resume: resumeAutoRefresh } = useIntervalFn(async () => {
  if (!autoRefreshEnabled.value || document.hidden || loading.value) return
  if (autoRefreshCountdown.value <= 1) { autoRefreshCountdown.value = autoRefreshIntervalSeconds.value; await loadAccounts(); return }
  autoRefreshCountdown.value -= 1
}, 1000, { immediate: false })

const debouncedSearch = useDebounceFn(reloadFromFirstPage, 300)
watch(() => filters.search, debouncedSearch)

onMounted(async () => {
  loadAutoRefreshSettings()
  document.addEventListener('click', handleClickOutside)
  const [groupsResult] = await Promise.allSettled([accountsAPI.listGroups(), loadAccounts()])
  if (groupsResult.status === 'fulfilled') groups.value = groupsResult.value
  else appStore.showError(extractApiErrorMessage(groupsResult.reason, t('common.error')))
  if (autoRefreshEnabled.value) resumeAutoRefresh()
})

onUnmounted(() => {
  abortController?.abort()
  pauseAutoRefresh()
  document.removeEventListener('click', handleClickOutside)
})
</script>

<style scoped>
.visible-account-menu-item {
  @apply flex w-full items-center justify-between gap-3 rounded-md px-3 py-2 text-sm text-gray-700 transition-colors hover:bg-gray-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-gray-200 dark:hover:bg-dark-700;
}
</style>
