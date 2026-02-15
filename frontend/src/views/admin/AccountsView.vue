<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap-reverse items-start justify-between gap-3">
          <AccountTableFilters
            v-model:searchQuery="params.search"
            :filters="params"
            :groups="groups"
            @update:filters="(newFilters) => Object.assign(params, newFilters)"
            @change="debouncedReload"
            @update:searchQuery="debouncedReload"
          />
          <AccountTableActions
            :loading="loading"
            @refresh="load"
            @sync="showSync = true"
            @create="showCreate = true"
          >
            <template #after>
              <!-- Auto Refresh Dropdown -->
              <div class="relative" ref="autoRefreshDropdownRef">
                <button
                  @click="
                    showAutoRefreshDropdown = !showAutoRefreshDropdown;
                    showColumnDropdown = false
                  "
                  class="btn btn-secondary px-2 md:px-3"
                  :title="t('admin.accounts.autoRefresh')"
                >
                  <Icon name="refresh" size="sm" :class="[autoRefreshEnabled ? 'animate-spin' : '']" />
                  <span class="hidden md:inline">
                    {{
                      autoRefreshEnabled
                        ? t('admin.accounts.autoRefreshCountdown', { seconds: autoRefreshCountdown REDACTED)
                        : t('admin.accounts.autoRefresh')
                    REDACTEDREDACTED
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
                      <span>{{ t('admin.accounts.enableAutoRefresh') REDACTEDREDACTED</span>
                      <Icon v-if="autoRefreshEnabled" name="check" size="sm" class="text-primary-500" />
                    </button>
                    <div class="my-1 border-t border-gray-100 dark:border-gray-700"></div>
                    <button
                      v-for="sec in autoRefreshIntervals"
                      :key="sec"
                      @click="setAutoRefreshInterval(sec)"
                      class="flex w-full items-center justify-between rounded-md px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-gray-700"
                    >
                      <span>{{ autoRefreshIntervalLabel(sec) REDACTEDREDACTED</span>
                      <Icon v-if="autoRefreshIntervalSeconds === sec" name="check" size="sm" class="text-primary-500" />
                    </button>
                  </div>
                </div>
              </div>

              <!-- Error Passthrough Rules -->
              <button
                @click="showErrorPassthrough = true"
                class="btn btn-secondary"
                :title="t('admin.errorPassthrough.title')"
              >
                <Icon name="shield" size="md" class="mr-1.5" />
                <span class="hidden md:inline">{{ t('admin.errorPassthrough.title') REDACTEDREDACTED</span>
              </button>

              <!-- Column Settings Dropdown -->
              <div class="relative" ref="columnDropdownRef">
                <button
                  @click="
                    showColumnDropdown = !showColumnDropdown;
                    showAutoRefreshDropdown = false
                  "
                  class="btn btn-secondary px-2 md:px-3"
                  :title="t('admin.users.columnSettings')"
                >
                  <svg class="h-4 w-4 md:mr-1.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M9 4.5v15m6-15v15m-10.875 0h15.75c.621 0 1.125-.504 1.125-1.125V5.625c0-.621-.504-1.125-1.125-1.125H4.125C3.504 4.5 3 5.004 3 5.625v12.75c0 .621.504 1.125 1.125 1.125z" />
                  </svg>
                  <span class="hidden md:inline">{{ t('admin.users.columnSettings') REDACTEDREDACTED</span>
                </button>
                <!-- Dropdown menu -->
                <div
                  v-if="showColumnDropdown"
                  class="absolute right-0 z-50 mt-2 w-48 origin-top-right rounded-lg border border-gray-200 bg-white shadow-lg dark:border-gray-700 dark:bg-gray-800"
                >
                  <div class="max-h-80 overflow-y-auto p-2">
                    <button
                      v-for="col in toggleableColumns"
                      :key="col.key"
                      @click="toggleColumn(col.key)"
                      class="flex w-full items-center justify-between rounded-md px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-gray-700"
                    >
                      <span>{{ col.label REDACTEDREDACTED</span>
                      <Icon v-if="isColumnVisible(col.key)" name="check" size="sm" class="text-primary-500" />
                    </button>
                  </div>
                </div>
              </div>
            </template>
            <template #beforeCreate>
              <button @click="showImportData = true" class="btn btn-secondary">
                {{ t('admin.accounts.dataImport') REDACTEDREDACTED
              </button>
              <button @click="openExportDataDialog" class="btn btn-secondary">
                {{ selIds.length ? t('admin.accounts.dataExportSelected') : t('admin.accounts.dataExport') REDACTEDREDACTED
              </button>
            </template>
          </AccountTableActions>
        </div>
      </template>
      <template #table>
        <AccountBulkActionsBar :selected-ids="selIds" @delete="handleBulkDelete" @edit="showBulkEdit = true" @clear="selIds = []" @select-page="selectPage" @toggle-schedulable="handleBulkToggleSchedulable" />
        <DataTable
          :columns="cols"
          :data="accounts"
          :loading="loading"
          row-key="id"
          default-sort-key="name"
          default-sort-order="asc"
          :sort-storage-key="ACCOUNT_SORT_STORAGE_KEY"
        >
          <template #header-select>
            <input
              type="checkbox"
              class="h-4 w-4 cursor-pointer rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              :checked="allVisibleSelected"
              @click.stop
              @change="toggleSelectAllVisible($event)"
            />
          </template>
          <template #cell-select="{ row REDACTED">
            <input type="checkbox" :checked="selIds.includes(row.id)" @change="toggleSel(row.id)" class="rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          </template>
          <template #cell-name="{ row, value REDACTED">
            <div class="flex flex-col">
              <span class="font-medium text-gray-900 dark:text-white">{{ value REDACTEDREDACTED</span>
              <span
                v-if="row.extra?.email_address"
                class="text-xs text-gray-500 dark:text-gray-400 truncate max-w-[200px]"
                :title="row.extra.email_address"
              >
                {{ row.extra.email_address REDACTEDREDACTED
              </span>
            </div>
          </template>
          <template #cell-notes="{ value REDACTED">
            <span v-if="value" :title="value" class="block max-w-xs truncate text-sm text-gray-600 dark:text-gray-300">{{ value REDACTEDREDACTED</span>
            <span v-else class="text-sm text-gray-400 dark:text-dark-500">-</span>
          </template>
          <template #cell-platform_type="{ row REDACTED">
            <PlatformTypeBadge :platform="row.platform" :type="row.type" />
          </template>
          <template #cell-capacity="{ row REDACTED">
            <AccountCapacityCell :account="row" />
          </template>
          <template #cell-status="{ row REDACTED">
            <AccountStatusIndicator :account="row" @show-temp-unsched="handleShowTempUnsched" />
          </template>
          <template #cell-schedulable="{ row REDACTED">
            <button @click="handleToggleSchedulable(row)" :disabled="togglingSchedulable === row.id" class="relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 dark:focus:ring-offset-dark-800" :class="[row.schedulable ? 'bg-primary-500 hover:bg-primary-600' : 'bg-gray-200 hover:bg-gray-300 dark:bg-dark-600 dark:hover:bg-dark-500']" :title="row.schedulable ? t('admin.accounts.schedulableEnabled') : t('admin.accounts.schedulableDisabled')">
              <span class="pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out" :class="[row.schedulable ? 'translate-x-4' : 'translate-x-0']" />
            </button>
          </template>
          <template #cell-today_stats="{ row REDACTED">
            <AccountTodayStatsCell :account="row" />
          </template>
          <template #cell-groups="{ row REDACTED">
            <AccountGroupsCell :groups="row.groups" :max-display="4" />
          </template>
          <template #cell-usage="{ row REDACTED">
            <AccountUsageCell :account="row" />
          </template>
          <template #cell-proxy="{ row REDACTED">
            <div v-if="row.proxy" class="flex items-center gap-2">
              <span class="text-sm text-gray-700 dark:text-gray-300">{{ row.proxy.name REDACTEDREDACTED</span>
              <span v-if="row.proxy.country_code" class="text-xs text-gray-500 dark:text-gray-400">
                ({{ row.proxy.country_code REDACTEDREDACTED)
              </span>
            </div>
            <span v-else class="text-sm text-gray-400 dark:text-dark-500">-</span>
          </template>
          <template #cell-rate_multiplier="{ row REDACTED">
            <span class="text-sm font-mono text-gray-700 dark:text-gray-300">
              {{ (row.rate_multiplier ?? 1).toFixed(2) REDACTEDREDACTEDx
            </span>
          </template>
          <template #cell-priority="{ value REDACTED">
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ value REDACTEDREDACTED</span>
          </template>
          <template #cell-last_used_at="{ value REDACTED">
            <span class="text-sm text-gray-500 dark:text-dark-400">{{ formatRelativeTime(value) REDACTEDREDACTED</span>
          </template>
          <template #cell-expires_at="{ row, value REDACTED">
            <div class="flex flex-col items-start gap-1">
              <span class="text-sm text-gray-500 dark:text-dark-400">{{ formatExpiresAt(value) REDACTEDREDACTED</span>
              <div v-if="isExpired(value) || (row.auto_pause_on_expired && value)" class="flex items-center gap-1">
                <span
                  v-if="isExpired(value)"
                  class="inline-flex items-center rounded-md bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-700 dark:bg-amber-900/30 dark:text-amber-300"
                >
                  {{ t('admin.accounts.expired') REDACTEDREDACTED
                </span>
                <span
                  v-if="row.auto_pause_on_expired && value"
                  class="inline-flex items-center rounded-md bg-emerald-100 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300"
                >
                  {{ t('admin.accounts.autoPauseOnExpired') REDACTEDREDACTED
                </span>
              </div>
            </div>
          </template>
          <template #cell-actions="{ row REDACTED">
            <div class="flex items-center gap-1">
              <button @click="handleEdit(row)" class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400">
                <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M16.862 4.487l1.687-1.688a1.875 1.875 0 112.652 2.652L10.582 16.07a4.5 4.5 0 01-1.897 1.13L6 18l.8-2.685a4.5 4.5 0 011.13-1.897l8.932-8.931zm0 0L19.5 7.125M18 14v4.75A2.25 2.25 0 0115.75 21H5.25A2.25 2.25 0 013 18.75V8.25A2.25 2.25 0 015.25 6H10" /></svg>
                <span class="text-xs">{{ t('common.edit') REDACTEDREDACTED</span>
              </button>
              <button @click="handleDelete(row)" class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400">
                <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M14.74 9l-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 01-2.244 2.077H8.084a2.25 2.25 0 01-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 00-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 013.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.964 51.964 0 00-3.32 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 00-7.5 0" /></svg>
                <span class="text-xs">{{ t('common.delete') REDACTEDREDACTED</span>
              </button>
              <button @click="openMenu(row, $event)" class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900 dark:hover:bg-dark-700 dark:hover:text-white">
                <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M6.75 12a.75.75 0 11-1.5 0 .75.75 0 011.5 0zM12.75 12a.75.75 0 11-1.5 0 .75.75 0 011.5 0zM18.75 12a.75.75 0 11-1.5 0 .75.75 0 011.5 0z" /></svg>
                <span class="text-xs">{{ t('common.more') REDACTEDREDACTED</span>
              </button>
            </div>
          </template>
        </DataTable>
      </template>
      <template #pagination><Pagination v-if="pagination.total > 0" :page="pagination.page" :total="pagination.total" :page-size="pagination.page_size" @update:page="handlePageChange" @update:pageSize="handlePageSizeChange" /></template>
    </TablePageLayout>
    <CreateAccountModal :show="showCreate" :proxies="proxies" :groups="groups" @close="showCreate = false" @created="reload" />
    <EditAccountModal :show="showEdit" :account="edAcc" :proxies="proxies" :groups="groups" @close="showEdit = false" @updated="handleAccountUpdated" />
    <ReAuthAccountModal :show="showReAuth" :account="reAuthAcc" @close="closeReAuthModal" @reauthorized="handleAccountUpdated" />
    <AccountTestModal :show="showTest" :account="testingAcc" @close="closeTestModal" />
    <AccountStatsModal :show="showStats" :account="statsAcc" @close="closeStatsModal" />
    <AccountActionMenu :show="menu.show" :account="menu.acc" :position="menu.pos" @close="menu.show = false" @test="handleTest" @stats="handleViewStats" @reauth="handleReAuth" @refresh-token="handleRefresh" @reset-status="handleResetStatus" @clear-rate-limit="handleClearRateLimit" />
    <SyncFromCrsModal :show="showSync" @close="showSync = false" @synced="reload" />
    <ImportDataModal :show="showImportData" @close="showImportData = false" @imported="handleDataImported" />
    <BulkEditAccountModal :show="showBulkEdit" :account-ids="selIds" :proxies="proxies" :groups="groups" @close="showBulkEdit = false" @updated="handleBulkUpdated" />
    <TempUnschedStatusModal :show="showTempUnsched" :account="tempUnschedAcc" @close="showTempUnsched = false" @reset="handleTempUnschedReset" />
    <ConfirmDialog :show="showDeleteDialog" :title="t('admin.accounts.deleteAccount')" :message="t('admin.accounts.deleteConfirm', { name: deletingAcc?.name REDACTED)" :confirm-text="t('common.delete')" :cancel-text="t('common.cancel')" :danger="true" @confirm="confirmDelete" @cancel="showDeleteDialog = false" />
    <ConfirmDialog :show="showExportDataDialog" :title="t('admin.accounts.dataExport')" :message="t('admin.accounts.dataExportConfirmMessage')" :confirm-text="t('admin.accounts.dataExportConfirm')" :cancel-text="t('common.cancel')" @confirm="handleExportData" @cancel="showExportDataDialog = false">
      <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
        <input type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" v-model="includeProxyOnExport" />
        <span>{{ t('admin.accounts.dataExportIncludeProxies') REDACTEDREDACTED</span>
      </label>
    </ConfirmDialog>
    <ErrorPassthroughRulesModal :show="showErrorPassthrough" @close="showErrorPassthrough = false" />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted REDACTED from 'vue'
import { useIntervalFn REDACTED from '@vueuse/core'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { useAuthStore REDACTED from '@/stores/auth'
import { adminAPI REDACTED from '@/api/admin'
import { useTableLoader REDACTED from '@/composables/useTableLoader'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { CreateAccountModal, EditAccountModal, BulkEditAccountModal, SyncFromCrsModal, TempUnschedStatusModal REDACTED from '@/components/account'
import AccountTableActions from '@/components/admin/account/AccountTableActions.vue'
import AccountTableFilters from '@/components/admin/account/AccountTableFilters.vue'
import AccountBulkActionsBar from '@/components/admin/account/AccountBulkActionsBar.vue'
import AccountActionMenu from '@/components/admin/account/AccountActionMenu.vue'
import ImportDataModal from '@/components/admin/account/ImportDataModal.vue'
import ReAuthAccountModal from '@/components/admin/account/ReAuthAccountModal.vue'
import AccountTestModal from '@/components/admin/account/AccountTestModal.vue'
import AccountStatsModal from '@/components/admin/account/AccountStatsModal.vue'
import AccountStatusIndicator from '@/components/account/AccountStatusIndicator.vue'
import AccountUsageCell from '@/components/account/AccountUsageCell.vue'
import AccountTodayStatsCell from '@/components/account/AccountTodayStatsCell.vue'
import AccountGroupsCell from '@/components/account/AccountGroupsCell.vue'
import AccountCapacityCell from '@/components/account/AccountCapacityCell.vue'
import PlatformTypeBadge from '@/components/common/PlatformTypeBadge.vue'
import Icon from '@/components/icons/Icon.vue'
import ErrorPassthroughRulesModal from '@/components/admin/ErrorPassthroughRulesModal.vue'
import { formatDateTime, formatRelativeTime REDACTED from '@/utils/format'
import type { Account, Proxy, AdminGroup REDACTED from '@/types'

const { t REDACTED = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const proxies = ref<Proxy[]>([])
const groups = ref<AdminGroup[]>([])
const selIds = ref<number[]>([])
const showCreate = ref(false)
const showEdit = ref(false)
const showSync = ref(false)
const showImportData = ref(false)
const showExportDataDialog = ref(false)
const includeProxyOnExport = ref(true)
const showBulkEdit = ref(false)
const showTempUnsched = ref(false)
const showDeleteDialog = ref(false)
const showReAuth = ref(false)
const showTest = ref(false)
const showStats = ref(false)
const showErrorPassthrough = ref(false)
const edAcc = ref<Account | null>(null)
const tempUnschedAcc = ref<Account | null>(null)
const deletingAcc = ref<Account | null>(null)
const reAuthAcc = ref<Account | null>(null)
const testingAcc = ref<Account | null>(null)
const statsAcc = ref<Account | null>(null)
const togglingSchedulable = ref<number | null>(null)
const menu = reactive<{show:boolean, acc:Account|null, pos:{top:number, left:numberREDACTED|nullREDACTED>({ show: false, acc: null, pos: null REDACTED)
const exportingData = ref(false)

// Column settings
const showColumnDropdown = ref(false)
const columnDropdownRef = ref<HTMLElement | null>(null)
const hiddenColumns = reactive<Set<string>>(new Set())
const DEFAULT_HIDDEN_COLUMNS = ['proxy', 'notes', 'priority', 'rate_multiplier']
const HIDDEN_COLUMNS_KEY = 'account-hidden-columns'

// Sorting settings
const ACCOUNT_SORT_STORAGE_KEY = 'account-table-sort'

// Auto refresh settings
const showAutoRefreshDropdown = ref(false)
const autoRefreshDropdownRef = ref<HTMLElement | null>(null)
const AUTO_REFRESH_STORAGE_KEY = 'account-auto-refresh'
const autoRefreshIntervals = [5, 10, 15, 30] as const
const autoRefreshEnabled = ref(false)
const autoRefreshIntervalSeconds = ref<(typeof autoRefreshIntervals)[number]>(30)
const autoRefreshCountdown = ref(0)

const autoRefreshIntervalLabel = (sec: number) => {
  if (sec === 5) return t('admin.accounts.refreshInterval5s')
  if (sec === 10) return t('admin.accounts.refreshInterval10s')
  if (sec === 15) return t('admin.accounts.refreshInterval15s')
  if (sec === 30) return t('admin.accounts.refreshInterval30s')
  return `${secREDACTEDs`
REDACTED

const loadSavedColumns = () => {
  try {
    const saved = localStorage.getItem(HIDDEN_COLUMNS_KEY)
    if (saved) {
      const parsed = JSON.parse(saved) as string[]
      parsed.forEach(key => hiddenColumns.add(key))
    REDACTED else {
      DEFAULT_HIDDEN_COLUMNS.forEach(key => hiddenColumns.add(key))
    REDACTED
  REDACTED catch (e) {
    console.error('Failed to load saved columns:', e)
    DEFAULT_HIDDEN_COLUMNS.forEach(key => hiddenColumns.add(key))
  REDACTED
REDACTED

const saveColumnsToStorage = () => {
  try {
    localStorage.setItem(HIDDEN_COLUMNS_KEY, JSON.stringify([...hiddenColumns]))
  REDACTED catch (e) {
    console.error('Failed to save columns:', e)
  REDACTED
REDACTED

const loadSavedAutoRefresh = () => {
  try {
    const saved = localStorage.getItem(AUTO_REFRESH_STORAGE_KEY)
    if (!saved) return
    const parsed = JSON.parse(saved) as { enabled?: boolean; interval_seconds?: number REDACTED
    autoRefreshEnabled.value = parsed.enabled === true
    const interval = Number(parsed.interval_seconds)
    if (autoRefreshIntervals.includes(interval as any)) {
      autoRefreshIntervalSeconds.value = interval as any
    REDACTED
  REDACTED catch (e) {
    console.error('Failed to load saved auto refresh settings:', e)
  REDACTED
REDACTED

const saveAutoRefreshToStorage = () => {
  try {
    localStorage.setItem(
      AUTO_REFRESH_STORAGE_KEY,
      JSON.stringify({
        enabled: autoRefreshEnabled.value,
        interval_seconds: autoRefreshIntervalSeconds.value
      REDACTED)
    )
  REDACTED catch (e) {
    console.error('Failed to save auto refresh settings:', e)
  REDACTED
REDACTED

if (typeof window !== 'undefined') {
  loadSavedColumns()
  loadSavedAutoRefresh()
REDACTED

const setAutoRefreshEnabled = (enabled: boolean) => {
  autoRefreshEnabled.value = enabled
  saveAutoRefreshToStorage()
  if (enabled) {
    autoRefreshCountdown.value = autoRefreshIntervalSeconds.value
    resumeAutoRefresh()
  REDACTED else {
    pauseAutoRefresh()
    autoRefreshCountdown.value = 0
  REDACTED
REDACTED

const setAutoRefreshInterval = (seconds: (typeof autoRefreshIntervals)[number]) => {
  autoRefreshIntervalSeconds.value = seconds
  saveAutoRefreshToStorage()
  if (autoRefreshEnabled.value) {
    autoRefreshCountdown.value = seconds
  REDACTED
REDACTED

const toggleColumn = (key: string) => {
  if (hiddenColumns.has(key)) {
    hiddenColumns.delete(key)
  REDACTED else {
    hiddenColumns.add(key)
  REDACTED
  saveColumnsToStorage()
REDACTED

const isColumnVisible = (key: string) => !hiddenColumns.has(key)

const { items: accounts, loading, params, pagination, load, reload, debouncedReload, handlePageChange, handlePageSizeChange REDACTED = useTableLoader<Account, any>({
  fetchFn: adminAPI.accounts.list,
  initialParams: { platform: '', type: '', status: '', group: '', search: '' REDACTED
REDACTED)

const isAnyModalOpen = computed(() => {
  return (
    showCreate.value ||
    showEdit.value ||
    showSync.value ||
    showImportData.value ||
    showExportDataDialog.value ||
    showBulkEdit.value ||
    showTempUnsched.value ||
    showDeleteDialog.value ||
    showReAuth.value ||
    showTest.value ||
    showStats.value ||
    showErrorPassthrough.value
  )
REDACTED)

const { pause: pauseAutoRefresh, resume: resumeAutoRefresh REDACTED = useIntervalFn(
  async () => {
    if (!autoRefreshEnabled.value) return
    if (document.hidden) return
    if (loading.value) return
    if (isAnyModalOpen.value) return
    if (menu.show) return

    if (autoRefreshCountdown.value <= 0) {
      autoRefreshCountdown.value = autoRefreshIntervalSeconds.value
      try {
        await load()
      REDACTED catch (e) {
        console.error('Auto refresh failed:', e)
      REDACTED
      return
    REDACTED

    autoRefreshCountdown.value -= 1
  REDACTED,
  1000,
  { immediate: false REDACTED
)

// All available columns
const allColumns = computed(() => {
  const c = [
    { key: 'select', label: '', sortable: false REDACTED,
    { key: 'name', label: t('admin.accounts.columns.name'), sortable: true REDACTED,
    { key: 'platform_type', label: t('admin.accounts.columns.platformType'), sortable: false REDACTED,
    { key: 'capacity', label: t('admin.accounts.columns.capacity'), sortable: false REDACTED,
    { key: 'status', label: t('admin.accounts.columns.status'), sortable: true REDACTED,
    { key: 'schedulable', label: t('admin.accounts.columns.schedulable'), sortable: true REDACTED,
    { key: 'today_stats', label: t('admin.accounts.columns.todayStats'), sortable: false REDACTED
  ]
  if (!authStore.isSimpleMode) {
    c.push({ key: 'groups', label: t('admin.accounts.columns.groups'), sortable: false REDACTED)
  REDACTED
  c.push(
    { key: 'usage', label: t('admin.accounts.columns.usageWindows'), sortable: false REDACTED,
    { key: 'proxy', label: t('admin.accounts.columns.proxy'), sortable: false REDACTED,
    { key: 'priority', label: t('admin.accounts.columns.priority'), sortable: true REDACTED,
    { key: 'rate_multiplier', label: t('admin.accounts.columns.billingRateMultiplier'), sortable: true REDACTED,
    { key: 'last_used_at', label: t('admin.accounts.columns.lastUsed'), sortable: true REDACTED,
    { key: 'expires_at', label: t('admin.accounts.columns.expiresAt'), sortable: true REDACTED,
    { key: 'notes', label: t('admin.accounts.columns.notes'), sortable: false REDACTED,
    { key: 'actions', label: t('admin.accounts.columns.actions'), sortable: false REDACTED
  )
  return c
REDACTED)

// Columns that can be toggled (exclude select, name, and actions)
const toggleableColumns = computed(() =>
  allColumns.value.filter(col => col.key !== 'select' && col.key !== 'name' && col.key !== 'actions')
)

// Filtered columns based on visibility
const cols = computed(() =>
  allColumns.value.filter(col =>
    col.key === 'select' || col.key === 'name' || col.key === 'actions' || !hiddenColumns.has(col.key)
  )
)

const handleEdit = (a: Account) => { edAcc.value = a; showEdit.value = true REDACTED
const openMenu = (a: Account, e: MouseEvent) => {
  menu.acc = a

  const target = e.currentTarget as HTMLElement
  if (target) {
    const rect = target.getBoundingClientRect()
    const menuWidth = 200
    const menuHeight = 240
    const padding = 8
    const viewportWidth = window.innerWidth
    const viewportHeight = window.innerHeight

    let left, top

    if (viewportWidth < 768) {
      // 居中显示,水平位置
      left = Math.max(padding, Math.min(
        rect.left + rect.width / 2 - menuWidth / 2,
        viewportWidth - menuWidth - padding
      ))

      // 优先显示在按钮下方
      top = rect.bottom + 4

      // 如果下方空间不够,显示在上方
      if (top + menuHeight > viewportHeight - padding) {
        top = rect.top - menuHeight - 4
        // 如果上方也不够,就贴在视口顶部
        if (top < padding) {
          top = padding
        REDACTED
      REDACTED
    REDACTED else {
      left = Math.max(padding, Math.min(
        e.clientX - menuWidth,
        viewportWidth - menuWidth - padding
      ))
      top = e.clientY
      if (top + menuHeight > viewportHeight - padding) {
        top = viewportHeight - menuHeight - padding
      REDACTED
    REDACTED

    menu.pos = { top, left REDACTED
  REDACTED else {
    menu.pos = { top: e.clientY, left: e.clientX - 200 REDACTED
  REDACTED

  menu.show = true
REDACTED
const toggleSel = (id: number) => { const i = selIds.value.indexOf(id); if(i === -1) selIds.value.push(id); else selIds.value.splice(i, 1) REDACTED
const allVisibleSelected = computed(() => {
  if (accounts.value.length === 0) return false
  return accounts.value.every(account => selIds.value.includes(account.id))
REDACTED)
const toggleSelectAllVisible = (event: Event) => {
  const target = event.target as HTMLInputElement
  if (target.checked) {
    const next = new Set(selIds.value)
    accounts.value.forEach(account => next.add(account.id))
    selIds.value = Array.from(next)
    return
  REDACTED
  const visibleIds = new Set(accounts.value.map(account => account.id))
  selIds.value = selIds.value.filter(id => !visibleIds.has(id))
REDACTED
const selectPage = () => { selIds.value = [...new Set([...selIds.value, ...accounts.value.map(a => a.id)])] REDACTED
const handleBulkDelete = async () => { if(!confirm(t('common.confirm'))) return; try { await Promise.all(selIds.value.map(id => adminAPI.accounts.delete(id))); selIds.value = []; reload() REDACTED catch (error) { console.error('Failed to bulk delete accounts:', error) REDACTED REDACTED
const updateSchedulableInList = (accountIds: number[], schedulable: boolean) => {
  if (accountIds.length === 0) return
  const idSet = new Set(accountIds)
  accounts.value = accounts.value.map((account) => (idSet.has(account.id) ? { ...account, schedulable REDACTED : account))
REDACTED
const normalizeBulkSchedulableResult = (
  result: {
    success?: number
    failed?: number
    success_ids?: number[]
    failed_ids?: number[]
    results?: Array<{ account_id: number; success: boolean REDACTED>
  REDACTED,
  accountIds: number[]
) => {
  const responseSuccessIds = Array.isArray(result.success_ids) ? result.success_ids : []
  const responseFailedIds = Array.isArray(result.failed_ids) ? result.failed_ids : []
  if (responseSuccessIds.length > 0 || responseFailedIds.length > 0) {
    return {
      successIds: responseSuccessIds,
      failedIds: responseFailedIds,
      successCount: typeof result.success === 'number' ? result.success : responseSuccessIds.length,
      failedCount: typeof result.failed === 'number' ? result.failed : responseFailedIds.length,
      hasIds: true,
      hasCounts: true
    REDACTED
  REDACTED

  const results = Array.isArray(result.results) ? result.results : []
  if (results.length > 0) {
    const successIds = results.filter(item => item.success).map(item => item.account_id)
    const failedIds = results.filter(item => !item.success).map(item => item.account_id)
    return {
      successIds,
      failedIds,
      successCount: typeof result.success === 'number' ? result.success : successIds.length,
      failedCount: typeof result.failed === 'number' ? result.failed : failedIds.length,
      hasIds: true,
      hasCounts: true
    REDACTED
  REDACTED

  const hasExplicitCounts = typeof result.success === 'number' || typeof result.failed === 'number'
  const successCount = typeof result.success === 'number' ? result.success : 0
  const failedCount = typeof result.failed === 'number' ? result.failed : 0
  if (hasExplicitCounts && failedCount === 0 && successCount === accountIds.length && accountIds.length > 0) {
    return {
      successIds: accountIds,
      failedIds: [],
      successCount,
      failedCount,
      hasIds: true,
      hasCounts: true
    REDACTED
  REDACTED

  return {
    successIds: [],
    failedIds: [],
    successCount,
    failedCount,
    hasIds: false,
    hasCounts: hasExplicitCounts
  REDACTED
REDACTED
const handleBulkToggleSchedulable = async (schedulable: boolean) => {
  const accountIds = [...selIds.value]
  try {
    const result = await adminAPI.accounts.bulkUpdate(accountIds, { schedulable REDACTED)
    const { successIds, failedIds, successCount, failedCount, hasIds, hasCounts REDACTED = normalizeBulkSchedulableResult(result, accountIds)
    if (!hasIds && !hasCounts) {
      appStore.showError(t('admin.accounts.bulkSchedulableResultUnknown'))
      selIds.value = accountIds
      load().catch((error) => {
        console.error('Failed to refresh accounts:', error)
      REDACTED)
      return
    REDACTED
    if (successIds.length > 0) {
      updateSchedulableInList(successIds, schedulable)
    REDACTED
    if (successCount > 0 && failedCount === 0) {
      const message = schedulable
        ? t('admin.accounts.bulkSchedulableEnabled', { count: successCount REDACTED)
        : t('admin.accounts.bulkSchedulableDisabled', { count: successCount REDACTED)
      appStore.showSuccess(message)
    REDACTED
    if (failedCount > 0) {
      const message = hasCounts || hasIds
        ? t('admin.accounts.bulkSchedulablePartial', { success: successCount, failed: failedCount REDACTED)
        : t('admin.accounts.bulkSchedulableResultUnknown')
      appStore.showError(message)
      selIds.value = failedIds.length > 0 ? failedIds : accountIds
    REDACTED else {
      selIds.value = hasIds ? [] : accountIds
    REDACTED
  REDACTED catch (error) {
    console.error('Failed to bulk toggle schedulable:', error)
    appStore.showError(t('common.error'))
  REDACTED
REDACTED
const handleBulkUpdated = () => { showBulkEdit.value = false; selIds.value = []; reload() REDACTED
const handleDataImported = () => { showImportData.value = false; reload() REDACTED
const accountMatchesCurrentFilters = (account: Account) => {
  if (params.platform && account.platform !== params.platform) return false
  if (params.type && account.type !== params.type) return false
  if (params.status) {
    if (params.status === 'rate_limited') {
      if (!account.rate_limit_reset_at) return false
      const resetAt = new Date(account.rate_limit_reset_at).getTime()
      if (!Number.isFinite(resetAt) || resetAt <= Date.now()) return false
    REDACTED else if (account.status !== params.status) {
      return false
    REDACTED
  REDACTED
  const search = String(params.search || '').trim().toLowerCase()
  if (search && !account.name.toLowerCase().includes(search)) return false
  return true
REDACTED
const mergeRuntimeFields = (oldAccount: Account, updatedAccount: Account): Account => ({
  ...updatedAccount,
  current_concurrency: updatedAccount.current_concurrency ?? oldAccount.current_concurrency,
  current_window_cost: updatedAccount.current_window_cost ?? oldAccount.current_window_cost,
  active_sessions: updatedAccount.active_sessions ?? oldAccount.active_sessions
REDACTED)

const syncPaginationAfterLocalRemoval = () => {
  const nextTotal = Math.max(0, pagination.total - 1)
  pagination.total = nextTotal
  pagination.pages = nextTotal > 0 ? Math.ceil(nextTotal / pagination.page_size) : 0

  const maxPage = Math.max(1, pagination.pages || 1)
  let shouldReload = false

  if (pagination.page > maxPage) {
    pagination.page = maxPage
    shouldReload = nextTotal > 0
  REDACTED else if (nextTotal > 0) {
    const displayedEnd = (pagination.page - 1) * pagination.page_size + accounts.value.length
    // 当前页条目变少时，若后续还有数据则补齐，避免空页/少一条直到手动刷新。
    shouldReload = displayedEnd < nextTotal
  REDACTED

  if (shouldReload) {
    load().catch((error) => {
      console.error('Failed to refresh accounts after local removal:', error)
    REDACTED)
  REDACTED
REDACTED

const patchAccountInList = (updatedAccount: Account) => {
  const index = accounts.value.findIndex(account => account.id === updatedAccount.id)
  if (index === -1) return
  const mergedAccount = mergeRuntimeFields(accounts.value[index], updatedAccount)
  if (!accountMatchesCurrentFilters(mergedAccount)) {
    accounts.value = accounts.value.filter(account => account.id !== mergedAccount.id)
    syncPaginationAfterLocalRemoval()
    selIds.value = selIds.value.filter(id => id !== mergedAccount.id)
    if (menu.acc?.id === mergedAccount.id) {
      menu.show = false
      menu.acc = null
    REDACTED
    return
  REDACTED
  const nextAccounts = [...accounts.value]
  nextAccounts[index] = mergedAccount
  accounts.value = nextAccounts
  if (edAcc.value?.id === mergedAccount.id) edAcc.value = mergedAccount
  if (reAuthAcc.value?.id === mergedAccount.id) reAuthAcc.value = mergedAccount
  if (tempUnschedAcc.value?.id === mergedAccount.id) tempUnschedAcc.value = mergedAccount
  if (deletingAcc.value?.id === mergedAccount.id) deletingAcc.value = mergedAccount
  if (menu.acc?.id === mergedAccount.id) menu.acc = mergedAccount
REDACTED
const handleAccountUpdated = (updatedAccount: Account) => {
  patchAccountInList(updatedAccount)
REDACTED
const formatExportTimestamp = () => {
  const now = new Date()
  const pad2 = (value: number) => String(value).padStart(2, '0')
  return `${now.getFullYear()REDACTED${pad2(now.getMonth() + 1)REDACTED${pad2(now.getDate())REDACTED${pad2(now.getHours())REDACTED${pad2(now.getMinutes())REDACTED${pad2(now.getSeconds())REDACTED`
REDACTED
const openExportDataDialog = () => {
  includeProxyOnExport.value = true
  showExportDataDialog.value = true
REDACTED
const handleExportData = async () => {
  if (exportingData.value) return
  exportingData.value = true
  try {
    const dataPayload = await adminAPI.accounts.exportData(
      selIds.value.length > 0
        ? { ids: selIds.value, includeProxies: includeProxyOnExport.value REDACTED
        : {
            includeProxies: includeProxyOnExport.value,
            filters: {
              platform: params.platform,
              type: params.type,
              status: params.status,
              search: params.search
            REDACTED
          REDACTED
    )
    const timestamp = formatExportTimestamp()
    const filename = `sub2api-account-${timestampREDACTED.json`
    const blob = new Blob([JSON.stringify(dataPayload, null, 2)], { type: 'application/json' REDACTED)
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = filename
    link.click()
    URL.revokeObjectURL(url)
    appStore.showSuccess(t('admin.accounts.dataExported'))
  REDACTED catch (error: any) {
    appStore.showError(error?.message || t('admin.accounts.dataExportFailed'))
  REDACTED finally {
    exportingData.value = false
    showExportDataDialog.value = false
  REDACTED
REDACTED
const closeTestModal = () => { showTest.value = false; testingAcc.value = null REDACTED
const closeStatsModal = () => { showStats.value = false; statsAcc.value = null REDACTED
const closeReAuthModal = () => { showReAuth.value = false; reAuthAcc.value = null REDACTED
const handleTest = (a: Account) => { testingAcc.value = a; showTest.value = true REDACTED
const handleViewStats = (a: Account) => { statsAcc.value = a; showStats.value = true REDACTED
const handleReAuth = (a: Account) => { reAuthAcc.value = a; showReAuth.value = true REDACTED
const handleRefresh = async (a: Account) => {
  try {
    const updated = await adminAPI.accounts.refreshCredentials(a.id)
    patchAccountInList(updated)
  REDACTED catch (error) {
    console.error('Failed to refresh credentials:', error)
  REDACTED
REDACTED
const handleResetStatus = async (a: Account) => {
  try {
    const updated = await adminAPI.accounts.clearError(a.id)
    patchAccountInList(updated)
    appStore.showSuccess(t('common.success'))
  REDACTED catch (error) {
    console.error('Failed to reset status:', error)
  REDACTED
REDACTED
const handleClearRateLimit = async (a: Account) => {
  try {
    const updated = await adminAPI.accounts.clearRateLimit(a.id)
    patchAccountInList(updated)
    appStore.showSuccess(t('common.success'))
  REDACTED catch (error) {
    console.error('Failed to clear rate limit:', error)
  REDACTED
REDACTED
const handleDelete = (a: Account) => { deletingAcc.value = a; showDeleteDialog.value = true REDACTED
const confirmDelete = async () => { if(!deletingAcc.value) return; try { await adminAPI.accounts.delete(deletingAcc.value.id); showDeleteDialog.value = false; deletingAcc.value = null; reload() REDACTED catch (error) { console.error('Failed to delete account:', error) REDACTED REDACTED
const handleToggleSchedulable = async (a: Account) => {
  const nextSchedulable = !a.schedulable
  togglingSchedulable.value = a.id
  try {
    const updated = await adminAPI.accounts.setSchedulable(a.id, nextSchedulable)
    updateSchedulableInList([a.id], updated?.schedulable ?? nextSchedulable)
  REDACTED catch (error) {
    console.error('Failed to toggle schedulable:', error)
    appStore.showError(t('admin.accounts.failedToToggleSchedulable'))
  REDACTED finally {
    togglingSchedulable.value = null
  REDACTED
REDACTED
const handleShowTempUnsched = (a: Account) => { tempUnschedAcc.value = a; showTempUnsched.value = true REDACTED
const handleTempUnschedReset = async () => {
  if(!tempUnschedAcc.value) return
  try {
    const updated = await adminAPI.accounts.clearError(tempUnschedAcc.value.id)
    showTempUnsched.value = false
    tempUnschedAcc.value = null
    patchAccountInList(updated)
  REDACTED catch (error) {
    console.error('Failed to reset temp unscheduled:', error)
  REDACTED
REDACTED
const formatExpiresAt = (value: number | null) => {
  if (!value) return '-'
  return formatDateTime(
    new Date(value * 1000),
    {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      hour12: false
    REDACTED,
    'sv-SE'
  )
REDACTED
const isExpired = (value: number | null) => {
  if (!value) return false
  return value * 1000 <= Date.now()
REDACTED

// 滚动时关闭操作菜单（不关闭列设置下拉菜单）
const handleScroll = () => {
  menu.show = false
REDACTED

// 点击外部关闭列设置下拉菜单
const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  if (columnDropdownRef.value && !columnDropdownRef.value.contains(target)) {
    showColumnDropdown.value = false
  REDACTED
  if (autoRefreshDropdownRef.value && !autoRefreshDropdownRef.value.contains(target)) {
    showAutoRefreshDropdown.value = false
  REDACTED
REDACTED

onMounted(async () => {
  load()
  try {
    const [p, g] = await Promise.all([adminAPI.proxies.getAll(), adminAPI.groups.getAll()])
    proxies.value = p
    groups.value = g
  REDACTED catch (error) {
    console.error('Failed to load proxies/groups:', error)
  REDACTED
  window.addEventListener('scroll', handleScroll, true)
  document.addEventListener('click', handleClickOutside)

  if (autoRefreshEnabled.value) {
    autoRefreshCountdown.value = autoRefreshIntervalSeconds.value
    resumeAutoRefresh()
  REDACTED else {
    pauseAutoRefresh()
  REDACTED
REDACTED)

onUnmounted(() => {
  window.removeEventListener('scroll', handleScroll, true)
  document.removeEventListener('click', handleClickOutside)
REDACTED)
</script>
