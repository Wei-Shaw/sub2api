<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Page Header Actions -->
      <div class="flex justify-end">
        <button
          @click="showAssignModal = true"
          class="btn btn-primary"
        >
          <svg class="w-5 h-5 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M12 4.5v15m7.5-7.5h-15" />
          </svg>
          {{ t('admin.subscriptions.assignSubscription') REDACTEDREDACTED
        </button>
      </div>

      <!-- Filters -->
      <div class="flex flex-wrap gap-3">
        <Select
          v-model="filters.status"
          :options="statusOptions"
          :placeholder="t('admin.subscriptions.allStatus')"
          class="w-40"
          @change="loadSubscriptions"
        />
        <Select
          v-model="filters.group_id"
          :options="groupOptions"
          :placeholder="t('admin.subscriptions.allGroups')"
          class="w-48"
          @change="loadSubscriptions"
        />
      </div>

      <!-- Subscriptions Table -->
      <div class="card overflow-hidden">
        <DataTable :columns="columns" :data="subscriptions" :loading="loading">
          <template #cell-user="{ row REDACTED">
            <div class="flex items-center gap-2">
              <div class="flex h-8 w-8 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/30">
                <span class="text-sm font-medium text-primary-700 dark:text-primary-300">
                  {{ row.user?.email?.charAt(0).toUpperCase() || '?' REDACTEDREDACTED
                </span>
              </div>
              <span class="font-medium text-gray-900 dark:text-white">{{ row.user?.email || `User #${row.user_idREDACTED` REDACTEDREDACTED</span>
            </div>
          </template>

          <template #cell-group="{ row REDACTED">
            <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-400">
              {{ row.group?.name || `Group #${row.group_idREDACTED` REDACTEDREDACTED
            </span>
          </template>

          <template #cell-usage="{ row REDACTED">
            <div class="space-y-1 min-w-[200px]">
              <div v-if="row.group?.daily_limit_usd" class="flex items-center gap-2">
                <span class="text-xs text-gray-500 w-12">{{ t('admin.subscriptions.daily') REDACTEDREDACTED</span>
                <div class="flex-1 bg-gray-200 dark:bg-dark-600 rounded-full h-2">
                  <div
                    class="h-2 rounded-full transition-all"
                    :class="getProgressClass(row.daily_usage_usd, row.group?.daily_limit_usd)"
                    :style="{ width: getProgressWidth(row.daily_usage_usd, row.group?.daily_limit_usd) REDACTED"
                  ></div>
                </div>
                <span class="text-xs text-gray-500 w-20 text-right">
                  ${{ row.daily_usage_usd?.toFixed(2) || '0.00' REDACTEDREDACTED / ${{ row.group?.daily_limit_usd?.toFixed(2) REDACTEDREDACTED
                </span>
              </div>
              <div v-if="row.group?.weekly_limit_usd" class="flex items-center gap-2">
                <span class="text-xs text-gray-500 w-12">{{ t('admin.subscriptions.weekly') REDACTEDREDACTED</span>
                <div class="flex-1 bg-gray-200 dark:bg-dark-600 rounded-full h-2">
                  <div
                    class="h-2 rounded-full transition-all"
                    :class="getProgressClass(row.weekly_usage_usd, row.group?.weekly_limit_usd)"
                    :style="{ width: getProgressWidth(row.weekly_usage_usd, row.group?.weekly_limit_usd) REDACTED"
                  ></div>
                </div>
                <span class="text-xs text-gray-500 w-20 text-right">
                  ${{ row.weekly_usage_usd?.toFixed(2) || '0.00' REDACTEDREDACTED / ${{ row.group?.weekly_limit_usd?.toFixed(2) REDACTEDREDACTED
                </span>
              </div>
              <div v-if="row.group?.monthly_limit_usd" class="flex items-center gap-2">
                <span class="text-xs text-gray-500 w-12">{{ t('admin.subscriptions.monthly') REDACTEDREDACTED</span>
                <div class="flex-1 bg-gray-200 dark:bg-dark-600 rounded-full h-2">
                  <div
                    class="h-2 rounded-full transition-all"
                    :class="getProgressClass(row.monthly_usage_usd, row.group?.monthly_limit_usd)"
                    :style="{ width: getProgressWidth(row.monthly_usage_usd, row.group?.monthly_limit_usd) REDACTED"
                  ></div>
                </div>
                <span class="text-xs text-gray-500 w-20 text-right">
                  ${{ row.monthly_usage_usd?.toFixed(2) || '0.00' REDACTEDREDACTED / ${{ row.group?.monthly_limit_usd?.toFixed(2) REDACTEDREDACTED
                </span>
              </div>
              <div v-if="!row.group?.daily_limit_usd && !row.group?.weekly_limit_usd && !row.group?.monthly_limit_usd" class="text-xs text-gray-500">
                {{ t('admin.subscriptions.noLimits') REDACTEDREDACTED
              </div>
            </div>
          </template>

          <template #cell-expires_at="{ value REDACTED">
            <div v-if="value">
              <span class="text-sm" :class="isExpiringSoon(value) ? 'text-orange-600 dark:text-orange-400' : 'text-gray-700 dark:text-gray-300'">
                {{ formatDate(value) REDACTEDREDACTED
              </span>
              <div v-if="getDaysRemaining(value) !== null" class="text-xs text-gray-500">
                {{ getDaysRemaining(value) REDACTEDREDACTED {{ t('admin.subscriptions.daysRemaining') REDACTEDREDACTED
              </div>
            </div>
            <span v-else class="text-sm text-gray-500">{{ t('admin.subscriptions.noExpiration') REDACTEDREDACTED</span>
          </template>

          <template #cell-status="{ value REDACTED">
            <span
              :class="[
                'badge',
                value === 'active' ? 'badge-success' : value === 'expired' ? 'badge-warning' : 'badge-danger'
              ]"
            >
              {{ t(`admin.subscriptions.status.${valueREDACTED`) REDACTEDREDACTED
            </span>
          </template>

          <template #cell-actions="{ row REDACTED">
            <div class="flex items-center gap-1">
              <button
                v-if="row.status === 'active'"
                @click="handleExtend(row)"
                class="p-2 rounded-lg hover:bg-green-50 dark:hover:bg-green-900/20 text-gray-500 hover:text-green-600 dark:hover:text-green-400 transition-colors"
                :title="t('admin.subscriptions.extend')"
              >
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M12 6v6h4.5m4.5 0a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
              </button>
              <button
                v-if="row.status === 'active'"
                @click="handleRevoke(row)"
                class="p-2 rounded-lg hover:bg-red-50 dark:hover:bg-red-900/20 text-gray-500 hover:text-red-600 dark:hover:text-red-400 transition-colors"
                :title="t('admin.subscriptions.revoke')"
              >
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M18.364 18.364A9 9 0 005.636 5.636m12.728 12.728A9 9 0 015.636 5.636m12.728 12.728L5.636 5.636" />
                </svg>
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState
              :title="t('admin.subscriptions.noSubscriptionsYet')"
              :description="t('admin.subscriptions.assignFirstSubscription')"
              :action-text="t('admin.subscriptions.assignSubscription')"
              @action="showAssignModal = true"
            />
          </template>
        </DataTable>
      </div>

      <!-- Pagination -->
      <Pagination
        v-if="pagination.total > 0"
        :page="pagination.page"
        :total="pagination.total"
        :page-size="pagination.page_size"
        @update:page="handlePageChange"
      />
    </div>

    <!-- Assign Subscription Modal -->
    <Modal
      :show="showAssignModal"
      :title="t('admin.subscriptions.assignSubscription')"
      size="lg"
      @close="closeAssignModal"
    >
      <form @submit.prevent="handleAssignSubscription" class="space-y-5">
        <div>
          <label class="input-label">{{ t('admin.subscriptions.form.user') REDACTEDREDACTED</label>
          <Select
            v-model="assignForm.user_id"
            :options="userOptions"
            :placeholder="t('admin.subscriptions.selectUser')"
            searchable
          />
        </div>
        <div>
          <label class="input-label">{{ t('admin.subscriptions.form.group') REDACTEDREDACTED</label>
          <Select
            v-model="assignForm.group_id"
            :options="subscriptionGroupOptions"
            :placeholder="t('admin.subscriptions.selectGroup')"
          />
          <p class="input-hint">{{ t('admin.subscriptions.groupHint') REDACTEDREDACTED</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.subscriptions.form.validityDays') REDACTEDREDACTED</label>
          <input
            v-model.number="assignForm.validity_days"
            type="number"
            min="1"
            class="input"
          />
          <p class="input-hint">{{ t('admin.subscriptions.validityHint') REDACTEDREDACTED</p>
        </div>

        <div class="flex justify-end gap-3 pt-4">
          <button
            @click="closeAssignModal"
            type="button"
            class="btn btn-secondary"
          >
            {{ t('common.cancel') REDACTEDREDACTED
          </button>
          <button
            type="submit"
            :disabled="submitting"
            class="btn btn-primary"
          >
            <svg
              v-if="submitting"
              class="animate-spin -ml-1 mr-2 h-4 w-4"
              fill="none"
              viewBox="0 0 24 24"
            >
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
            </svg>
            {{ submitting ? t('admin.subscriptions.assigning') : t('admin.subscriptions.assign') REDACTEDREDACTED
          </button>
        </div>
      </form>
    </Modal>

    <!-- Extend Subscription Modal -->
    <Modal
      :show="showExtendModal"
      :title="t('admin.subscriptions.extendSubscription')"
      size="md"
      @close="closeExtendModal"
    >
      <form v-if="extendingSubscription" @submit.prevent="handleExtendSubscription" class="space-y-5">
        <div class="p-4 bg-gray-50 dark:bg-dark-700 rounded-lg">
          <p class="text-sm text-gray-600 dark:text-gray-400">
            {{ t('admin.subscriptions.extendingFor') REDACTEDREDACTED
            <span class="font-medium text-gray-900 dark:text-white">{{ extendingSubscription.user?.email REDACTEDREDACTED</span>
          </p>
          <p class="text-sm text-gray-600 dark:text-gray-400 mt-1">
            {{ t('admin.subscriptions.currentExpiration') REDACTEDREDACTED:
            <span class="font-medium text-gray-900 dark:text-white">
              {{ extendingSubscription.expires_at ? formatDate(extendingSubscription.expires_at) : t('admin.subscriptions.noExpiration') REDACTEDREDACTED
            </span>
          </p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.subscriptions.form.extendDays') REDACTEDREDACTED</label>
          <input
            v-model.number="extendForm.days"
            type="number"
            min="1"
            required
            class="input"
          />
        </div>

        <div class="flex justify-end gap-3 pt-4">
          <button
            @click="closeExtendModal"
            type="button"
            class="btn btn-secondary"
          >
            {{ t('common.cancel') REDACTEDREDACTED
          </button>
          <button
            type="submit"
            :disabled="submitting"
            class="btn btn-primary"
          >
            {{ submitting ? t('admin.subscriptions.extending') : t('admin.subscriptions.extend') REDACTEDREDACTED
          </button>
        </div>
      </form>
    </Modal>

    <!-- Revoke Confirmation Dialog -->
    <ConfirmDialog
      :show="showRevokeDialog"
      :title="t('admin.subscriptions.revokeSubscription')"
      :message="t('admin.subscriptions.revokeConfirm', { user: revokingSubscription?.user?.email REDACTED)"
      :confirm-text="t('admin.subscriptions.revoke')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="confirmRevoke"
      @cancel="showRevokeDialog = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { adminAPI REDACTED from '@/api/admin'
import type { UserSubscription, Group, User REDACTED from '@/types'
import type { Column REDACTED from '@/components/common/DataTable.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Modal from '@/components/common/Modal.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'

const { t REDACTED = useI18n()
const appStore = useAppStore()

const columns = computed<Column[]>(() => [
  { key: 'user', label: t('admin.subscriptions.columns.user'), sortable: true REDACTED,
  { key: 'group', label: t('admin.subscriptions.columns.group'), sortable: true REDACTED,
  { key: 'usage', label: t('admin.subscriptions.columns.usage'), sortable: false REDACTED,
  { key: 'expires_at', label: t('admin.subscriptions.columns.expires'), sortable: true REDACTED,
  { key: 'status', label: t('admin.subscriptions.columns.status'), sortable: true REDACTED,
  { key: 'actions', label: t('admin.subscriptions.columns.actions'), sortable: false REDACTED
])

// Filter options
const statusOptions = computed(() => [
  { value: '', label: t('admin.subscriptions.allStatus') REDACTED,
  { value: 'active', label: t('admin.subscriptions.status.active') REDACTED,
  { value: 'expired', label: t('admin.subscriptions.status.expired') REDACTED,
  { value: 'revoked', label: t('admin.subscriptions.status.revoked') REDACTED
])

const subscriptions = ref<UserSubscription[]>([])
const groups = ref<Group[]>([])
const users = ref<User[]>([])
const loading = ref(false)
const filters = reactive({
  status: '',
  group_id: ''
REDACTED)
const pagination = reactive({
  page: 1,
  page_size: 20,
  total: 0,
  pages: 0
REDACTED)

const showAssignModal = ref(false)
const showExtendModal = ref(false)
const showRevokeDialog = ref(false)
const submitting = ref(false)
const extendingSubscription = ref<UserSubscription | null>(null)
const revokingSubscription = ref<UserSubscription | null>(null)

const assignForm = reactive({
  user_id: null as number | null,
  group_id: null as number | null,
  validity_days: 30
REDACTED)

const extendForm = reactive({
  days: 30
REDACTED)

// Group options for filter (all groups)
const groupOptions = computed(() => [
  { value: '', label: t('admin.subscriptions.allGroups') REDACTED,
  ...groups.value.map(g => ({ value: g.id.toString(), label: g.name REDACTED))
])

// Group options for assign (only subscription type groups)
const subscriptionGroupOptions = computed(() =>
  groups.value
    .filter(g => g.subscription_type === 'subscription' && g.status === 'active')
    .map(g => ({ value: g.id, label: g.name REDACTED))
)

// User options for assign
const userOptions = computed(() =>
  users.value.map(u => ({ value: u.id, label: u.email REDACTED))
)

const loadSubscriptions = async () => {
  loading.value = true
  try {
    const response = await adminAPI.subscriptions.list(
      pagination.page,
      pagination.page_size,
      {
        status: filters.status as any || undefined,
        group_id: filters.group_id ? parseInt(filters.group_id) : undefined
      REDACTED
    )
    subscriptions.value = response.items
    pagination.total = response.total
    pagination.pages = response.pages
  REDACTED catch (error) {
    appStore.showError(t('admin.subscriptions.failedToLoad'))
    console.error('Error loading subscriptions:', error)
  REDACTED finally {
    loading.value = false
  REDACTED
REDACTED

const loadGroups = async () => {
  try {
    groups.value = await adminAPI.groups.getAll()
  REDACTED catch (error) {
    console.error('Error loading groups:', error)
  REDACTED
REDACTED

const loadUsers = async () => {
  try {
    const response = await adminAPI.users.list(1, 1000)
    users.value = response.items
  REDACTED catch (error) {
    console.error('Error loading users:', error)
  REDACTED
REDACTED

const handlePageChange = (page: number) => {
  pagination.page = page
  loadSubscriptions()
REDACTED

const closeAssignModal = () => {
  showAssignModal.value = false
  assignForm.user_id = null
  assignForm.group_id = null
  assignForm.validity_days = 30
REDACTED

const handleAssignSubscription = async () => {
  if (!assignForm.user_id || !assignForm.group_id) return

  submitting.value = true
  try {
    await adminAPI.subscriptions.assign({
      user_id: assignForm.user_id,
      group_id: assignForm.group_id,
      validity_days: assignForm.validity_days
    REDACTED)
    appStore.showSuccess(t('admin.subscriptions.subscriptionAssigned'))
    closeAssignModal()
    loadSubscriptions()
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.subscriptions.failedToAssign'))
    console.error('Error assigning subscription:', error)
  REDACTED finally {
    submitting.value = false
  REDACTED
REDACTED

const handleExtend = (subscription: UserSubscription) => {
  extendingSubscription.value = subscription
  extendForm.days = 30
  showExtendModal.value = true
REDACTED

const closeExtendModal = () => {
  showExtendModal.value = false
  extendingSubscription.value = null
REDACTED

const handleExtendSubscription = async () => {
  if (!extendingSubscription.value) return

  submitting.value = true
  try {
    await adminAPI.subscriptions.extend(extendingSubscription.value.id, {
      days: extendForm.days
    REDACTED)
    appStore.showSuccess(t('admin.subscriptions.subscriptionExtended'))
    closeExtendModal()
    loadSubscriptions()
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.subscriptions.failedToExtend'))
    console.error('Error extending subscription:', error)
  REDACTED finally {
    submitting.value = false
  REDACTED
REDACTED

const handleRevoke = (subscription: UserSubscription) => {
  revokingSubscription.value = subscription
  showRevokeDialog.value = true
REDACTED

const confirmRevoke = async () => {
  if (!revokingSubscription.value) return

  try {
    await adminAPI.subscriptions.revoke(revokingSubscription.value.id)
    appStore.showSuccess(t('admin.subscriptions.subscriptionRevoked'))
    showRevokeDialog.value = false
    revokingSubscription.value = null
    loadSubscriptions()
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.subscriptions.failedToRevoke'))
    console.error('Error revoking subscription:', error)
  REDACTED
REDACTED

// Helper functions
const formatDate = (dateString: string) => {
  return new Date(dateString).toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric'
  REDACTED)
REDACTED

const getDaysRemaining = (expiresAt: string): number | null => {
  const now = new Date()
  const expires = new Date(expiresAt)
  const diff = expires.getTime() - now.getTime()
  if (diff < 0) return null
  return Math.ceil(diff / (1000 * 60 * 60 * 24))
REDACTED

const isExpiringSoon = (expiresAt: string): boolean => {
  const days = getDaysRemaining(expiresAt)
  return days !== null && days <= 7
REDACTED

const getProgressWidth = (used: number, limit: number | null): string => {
  if (!limit || limit === 0) return '0%'
  const percentage = Math.min((used / limit) * 100, 100)
  return `${percentageREDACTED%`
REDACTED

const getProgressClass = (used: number, limit: number | null): string => {
  if (!limit || limit === 0) return 'bg-gray-400'
  const percentage = (used / limit) * 100
  if (percentage >= 90) return 'bg-red-500'
  if (percentage >= 70) return 'bg-orange-500'
  return 'bg-green-500'
REDACTED

onMounted(() => {
  loadSubscriptions()
  loadGroups()
  loadUsers()
REDACTED)
</script>
