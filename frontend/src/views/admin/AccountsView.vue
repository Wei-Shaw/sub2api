<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap-reverse items-start justify-between gap-3">
          <AccountTableFilters
            v-model:searchQuery="params.search"
            :filters="params"
            @update:filters="(newFilters) => Object.assign(params, newFilters)"
            @change="debouncedReload"
            @update:searchQuery="debouncedReload"
          />
          <AccountTableActions
            :loading="loading"
            @refresh="load"
            @sync="showSync = true"
            @create="showCreate = true"
          />
        </div>
      </template>
      <template #table>
        <AccountBulkActionsBar :selected-ids="selIds" @delete="handleBulkDelete" @edit="showBulkEdit = true" @clear="selIds = []" @select-page="selectPage" @toggle-schedulable="handleBulkToggleSchedulable" />
        <DataTable :columns="cols" :data="accounts" :loading="loading" row-key="id">
          <template #cell-select="{ row REDACTED">
            <input type="checkbox" :checked="selIds.includes(row.id)" @change="toggleSel(row.id)" class="rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          </template>
          <template #cell-name="{ value REDACTED">
            <span class="font-medium text-gray-900 dark:text-white">{{ value REDACTEDREDACTED</span>
          </template>
          <template #cell-notes="{ value REDACTED">
            <span v-if="value" :title="value" class="block max-w-xs truncate text-sm text-gray-600 dark:text-gray-300">{{ value REDACTEDREDACTED</span>
            <span v-else class="text-sm text-gray-400 dark:text-dark-500">-</span>
          </template>
          <template #cell-platform_type="{ row REDACTED">
            <PlatformTypeBadge :platform="row.platform" :type="row.type" />
          </template>
          <template #cell-concurrency="{ row REDACTED">
            <div class="flex items-center gap-1.5">
              <span :class="['inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-xs font-medium', (row.current_concurrency || 0) >= row.concurrency ? 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400' : (row.current_concurrency || 0) > 0 ? 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400' : 'bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400']">
                <svg class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M3.75 6A2.25 2.25 0 016 3.75h2.25A2.25 2.25 0 0110.5 6v2.25a2.25 2.25 0 01-2.25 2.25H6a2.25 2.25 0 01-2.25-2.25V6zM3.75 15.75A2.25 2.25 0 016 13.5h2.25a2.25 2.25 0 012.25 2.25V18a2.25 2.25 0 01-2.25 2.25H6A2.25 2.25 0 013.75 18v-2.25zM13.5 6a2.25 2.25 0 012.25-2.25H18A2.25 2.25 0 0120.25 6v2.25A2.25 2.25 0 0118 10.5h-2.25a2.25 2.25 0 01-2.25-2.25V6zM13.5 15.75a2.25 2.25 0 012.25-2.25H18a2.25 2.25 0 012.25 2.25V18A2.25 2.25 0 0118 20.25h-2.25A2.25 2.25 0 0113.5 18v-2.25z" /></svg>
                <span class="font-mono">{{ row.current_concurrency || 0 REDACTEDREDACTED</span>
                <span class="text-gray-400 dark:text-gray-500">/</span>
                <span class="font-mono">{{ row.concurrency REDACTEDREDACTED</span>
              </span>
            </div>
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
    <EditAccountModal :show="showEdit" :account="edAcc" :proxies="proxies" :groups="groups" @close="showEdit = false" @updated="load" />
    <ReAuthAccountModal :show="showReAuth" :account="reAuthAcc" @close="closeReAuthModal" @reauthorized="load" />
    <AccountTestModal :show="showTest" :account="testingAcc" @close="closeTestModal" />
    <AccountStatsModal :show="showStats" :account="statsAcc" @close="closeStatsModal" />
    <AccountActionMenu :show="menu.show" :account="menu.acc" :position="menu.pos" @close="menu.show = false" @test="handleTest" @stats="handleViewStats" @reauth="handleReAuth" @refresh-token="handleRefresh" @reset-status="handleResetStatus" @clear-rate-limit="handleClearRateLimit" />
    <SyncFromCrsModal :show="showSync" @close="showSync = false" @synced="reload" />
    <BulkEditAccountModal :show="showBulkEdit" :account-ids="selIds" :proxies="proxies" :groups="groups" @close="showBulkEdit = false" @updated="handleBulkUpdated" />
    <TempUnschedStatusModal :show="showTempUnsched" :account="tempUnschedAcc" @close="showTempUnsched = false" @reset="handleTempUnschedReset" />
    <ConfirmDialog :show="showDeleteDialog" :title="t('admin.accounts.deleteAccount')" :message="t('admin.accounts.deleteConfirm', { name: deletingAcc?.name REDACTED)" :confirm-text="t('common.delete')" :cancel-text="t('common.cancel')" :danger="true" @confirm="confirmDelete" @cancel="showDeleteDialog = false" />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted REDACTED from 'vue'
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
import ReAuthAccountModal from '@/components/admin/account/ReAuthAccountModal.vue'
import AccountTestModal from '@/components/admin/account/AccountTestModal.vue'
import AccountStatsModal from '@/components/admin/account/AccountStatsModal.vue'
import AccountStatusIndicator from '@/components/account/AccountStatusIndicator.vue'
import AccountUsageCell from '@/components/account/AccountUsageCell.vue'
import AccountTodayStatsCell from '@/components/account/AccountTodayStatsCell.vue'
import AccountGroupsCell from '@/components/account/AccountGroupsCell.vue'
import PlatformTypeBadge from '@/components/common/PlatformTypeBadge.vue'
import { formatDateTime, formatRelativeTime REDACTED from '@/utils/format'
import type { Account, Proxy, Group REDACTED from '@/types'

const { t REDACTED = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const proxies = ref<Proxy[]>([])
const groups = ref<Group[]>([])
const selIds = ref<number[]>([])
const showCreate = ref(false)
const showEdit = ref(false)
const showSync = ref(false)
const showBulkEdit = ref(false)
const showTempUnsched = ref(false)
const showDeleteDialog = ref(false)
const showReAuth = ref(false)
const showTest = ref(false)
const showStats = ref(false)
const edAcc = ref<Account | null>(null)
const tempUnschedAcc = ref<Account | null>(null)
const deletingAcc = ref<Account | null>(null)
const reAuthAcc = ref<Account | null>(null)
const testingAcc = ref<Account | null>(null)
const statsAcc = ref<Account | null>(null)
const togglingSchedulable = ref<number | null>(null)
const menu = reactive<{show:boolean, acc:Account|null, pos:{top:number, left:numberREDACTED|nullREDACTED>({ show: false, acc: null, pos: null REDACTED)

const { items: accounts, loading, params, pagination, load, reload, debouncedReload, handlePageChange, handlePageSizeChange REDACTED = useTableLoader<Account, any>({
  fetchFn: adminAPI.accounts.list,
  initialParams: { platform: '', type: '', status: '', search: '' REDACTED
REDACTED)

const cols = computed(() => {
  const c = [
    { key: 'select', label: '', sortable: false REDACTED,
    { key: 'name', label: t('admin.accounts.columns.name'), sortable: true REDACTED,
    { key: 'platform_type', label: t('admin.accounts.columns.platformType'), sortable: false REDACTED,
    { key: 'concurrency', label: t('admin.accounts.columns.concurrencyStatus'), sortable: false REDACTED,
    { key: 'status', label: t('admin.accounts.columns.status'), sortable: true REDACTED,
    { key: 'schedulable', label: t('admin.accounts.columns.schedulable'), sortable: true REDACTED,
    { key: 'today_stats', label: t('admin.accounts.columns.todayStats'), sortable: false REDACTED
  ]
  if (!authStore.isSimpleMode) {
    c.push({ key: 'groups', label: t('admin.accounts.columns.groups'), sortable: false REDACTED)
  REDACTED
    c.push(
      { key: 'usage', label: t('admin.accounts.columns.usageWindows'), sortable: false REDACTED,
      { key: 'priority', label: t('admin.accounts.columns.priority'), sortable: true REDACTED,
      { key: 'rate_multiplier', label: t('admin.accounts.columns.billingRateMultiplier'), sortable: true REDACTED,
      { key: 'last_used_at', label: t('admin.accounts.columns.lastUsed'), sortable: true REDACTED,
    { key: 'expires_at', label: t('admin.accounts.columns.expiresAt'), sortable: true REDACTED,
    { key: 'notes', label: t('admin.accounts.columns.notes'), sortable: false REDACTED,
    { key: 'actions', label: t('admin.accounts.columns.actions'), sortable: false REDACTED
  )
  return c
REDACTED)

const handleEdit = (a: Account) => { edAcc.value = a; showEdit.value = true REDACTED
const openMenu = (a: Account, e: MouseEvent) => { menu.acc = a; menu.pos = { top: e.clientY, left: e.clientX - 200 REDACTED; menu.show = true REDACTED
const toggleSel = (id: number) => { const i = selIds.value.indexOf(id); if(i === -1) selIds.value.push(id); else selIds.value.splice(i, 1) REDACTED
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
    load().catch((error) => {
      console.error('Failed to refresh accounts:', error)
    REDACTED)
  REDACTED catch (error) {
    console.error('Failed to bulk toggle schedulable:', error)
    appStore.showError(t('common.error'))
  REDACTED
REDACTED
const handleBulkUpdated = () => { showBulkEdit.value = false; selIds.value = []; reload() REDACTED
const closeTestModal = () => { showTest.value = false; testingAcc.value = null REDACTED
const closeStatsModal = () => { showStats.value = false; statsAcc.value = null REDACTED
const closeReAuthModal = () => { showReAuth.value = false; reAuthAcc.value = null REDACTED
const handleTest = (a: Account) => { testingAcc.value = a; showTest.value = true REDACTED
const handleViewStats = (a: Account) => { statsAcc.value = a; showStats.value = true REDACTED
const handleReAuth = (a: Account) => { reAuthAcc.value = a; showReAuth.value = true REDACTED
const handleRefresh = async (a: Account) => { try { await adminAPI.accounts.refreshCredentials(a.id); load() REDACTED catch (error) { console.error('Failed to refresh credentials:', error) REDACTED REDACTED
const handleResetStatus = async (a: Account) => { try { await adminAPI.accounts.clearError(a.id); appStore.showSuccess(t('common.success')); load() REDACTED catch (error) { console.error('Failed to reset status:', error) REDACTED REDACTED
const handleClearRateLimit = async (a: Account) => { try { await adminAPI.accounts.clearRateLimit(a.id); appStore.showSuccess(t('common.success')); load() REDACTED catch (error) { console.error('Failed to clear rate limit:', error) REDACTED REDACTED
const handleDelete = (a: Account) => { deletingAcc.value = a; showDeleteDialog.value = true REDACTED
const confirmDelete = async () => { if(!deletingAcc.value) return; try { await adminAPI.accounts.delete(deletingAcc.value.id); showDeleteDialog.value = false; deletingAcc.value = null; reload() REDACTED catch (error) { console.error('Failed to delete account:', error) REDACTED REDACTED
const handleToggleSchedulable = async (a: Account) => {
  const nextSchedulable = !a.schedulable
  togglingSchedulable.value = a.id
  try {
    const updated = await adminAPI.accounts.setSchedulable(a.id, nextSchedulable)
    updateSchedulableInList([a.id], updated?.schedulable ?? nextSchedulable)
    load().catch((error) => {
      console.error('Failed to refresh accounts:', error)
    REDACTED)
  REDACTED catch (error) {
    console.error('Failed to toggle schedulable:', error)
    appStore.showError(t('admin.accounts.failedToToggleSchedulable'))
  REDACTED finally {
    togglingSchedulable.value = null
  REDACTED
REDACTED
const handleShowTempUnsched = (a: Account) => { tempUnschedAcc.value = a; showTempUnsched.value = true REDACTED
const handleTempUnschedReset = async () => { if(!tempUnschedAcc.value) return; try { await adminAPI.accounts.clearError(tempUnschedAcc.value.id); showTempUnsched.value = false; tempUnschedAcc.value = null; load() REDACTED catch (error) { console.error('Failed to reset temp unscheduled:', error) REDACTED REDACTED
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

onMounted(async () => { load(); try { const [p, g] = await Promise.all([adminAPI.proxies.getAll(), adminAPI.groups.getAll()]); proxies.value = p; groups.value = g REDACTED catch (error) { console.error('Failed to load proxies/groups:', error) REDACTED REDACTED)
</script>
