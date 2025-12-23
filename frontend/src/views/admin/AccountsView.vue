<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Page Header Actions -->
      <div class="flex justify-end gap-3">
        <button
          @click="loadAccounts"
          :disabled="loading"
          class="btn btn-secondary"
          :title="t('common.refresh')"
        >
          <svg
            :class="['w-5 h-5', loading ? 'animate-spin' : '']"
            fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"
          >
            <path stroke-linecap="round" stroke-linejoin="round" d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M4.031 9.865a8.25 8.25 0 0113.803-3.7l3.181 3.182m0-4.991v4.99" />
          </svg>
        </button>
        <button
          @click="showCreateModal = true"
          class="btn btn-primary"
        >
          <svg class="w-5 h-5 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M12 4.5v15m7.5-7.5h-15" />
          </svg>
          {{ t('admin.accounts.createAccount') }}
        </button>
      </div>

      <!-- Search and Filters -->
      <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
        <div class="relative flex-1 max-w-md">
          <svg class="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M21 21l-5.197-5.197m0 0A7.5 7.5 0 105.196 5.196a7.5 7.5 0 0010.607 10.607z" />
          </svg>
          <input
            v-model="searchQuery"
            type="text"
            :placeholder="t('admin.accounts.searchAccounts')"
            class="input pl-10"
            @input="handleSearch"
          />
        </div>
        <div class="flex flex-wrap gap-3">
          <Select
            v-model="filters.platform"
            :options="platformOptions"
            :placeholder="t('admin.accounts.allPlatforms')"
            class="w-40"
            @change="loadAccounts"
          />
          <Select
            v-model="filters.type"
            :options="typeOptions"
            :placeholder="t('admin.accounts.allTypes')"
            class="w-40"
            @change="loadAccounts"
          />
          <Select
            v-model="filters.status"
            :options="statusOptions"
            :placeholder="t('admin.accounts.allStatus')"
            class="w-36"
            @change="loadAccounts"
          />
        </div>
      </div>

      <!-- Accounts Table -->
      <div class="card overflow-hidden">
        <DataTable :columns="columns" :data="accounts" :loading="loading">
          <template #cell-name="{ value }">
            <span class="font-medium text-gray-900 dark:text-white">{{ value }}</span>
          </template>

          <template #cell-platform="{ value }">
            <div class="flex items-center gap-2">
              <span
                :class="[
                  'w-2 h-2 rounded-full',
                  value === 'anthropic' ? 'bg-orange-500' : value === 'openai' ? 'bg-green-500' : 'bg-gray-400'
                ]"
              />
              <span class="text-sm text-gray-700 dark:text-gray-300 capitalize">{{ value === 'anthropic' ? 'Anthropic' : value === 'openai' ? 'OpenAI' : value }}</span>
            </div>
          </template>

          <template #cell-type="{ value }">
            <span
              :class="[
                'badge',
                value === 'oauth' ? 'badge-primary' : value === 'setup-token' ? 'badge-info' : 'badge-purple'
              ]"
            >
              {{ value === 'oauth' ? 'Oauth' : value === 'setup-token' ? t('admin.accounts.setupToken') : t('admin.accounts.apiKey') }}
            </span>
          </template>

          <template #cell-status="{ row }">
            <AccountStatusIndicator :account="row" />
          </template>

          <template #cell-schedulable="{ row }">
            <button
              @click="handleToggleSchedulable(row)"
              :disabled="togglingSchedulable === row.id"
              class="relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 dark:focus:ring-offset-dark-800 disabled:opacity-50 disabled:cursor-not-allowed"
              :class="[
                row.schedulable
                  ? 'bg-primary-500 hover:bg-primary-600'
                  : 'bg-gray-200 dark:bg-dark-600 hover:bg-gray-300 dark:hover:bg-dark-500'
              ]"
              :title="row.schedulable ? t('admin.accounts.schedulableEnabled') : t('admin.accounts.schedulableDisabled')"
            >
              <span
                class="pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
                :class="[row.schedulable ? 'translate-x-4' : 'translate-x-0']"
              />
            </button>
          </template>

          <template #cell-today_stats="{ row }">
            <AccountTodayStatsCell :account="row" />
          </template>

          <template #cell-usage="{ row }">
            <AccountUsageCell :account="row" />
          </template>

          <template #cell-priority="{ value }">
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ value }}</span>
          </template>

          <template #cell-last_used_at="{ value }">
            <span class="text-sm text-gray-500 dark:text-dark-400">
              {{ formatRelativeTime(value) }}
            </span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <!-- Reset Status button for error accounts -->
              <button
                v-if="row.status === 'error'"
                @click="handleResetStatus(row)"
                class="p-2 rounded-lg hover:bg-red-50 dark:hover:bg-red-900/20 text-red-500 hover:text-red-600 dark:hover:text-red-400 transition-colors"
                :title="t('admin.accounts.resetStatus')"
              >
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M9 15L3 9m0 0l6-6M3 9h12a6 6 0 010 12h-3" />
                </svg>
              </button>
              <!-- Clear Rate Limit button -->
              <button
                v-if="isRateLimited(row) || isOverloaded(row)"
                @click="handleClearRateLimit(row)"
                class="p-2 rounded-lg hover:bg-amber-50 dark:hover:bg-amber-900/20 text-amber-500 hover:text-amber-600 dark:hover:text-amber-400 transition-colors"
                :title="t('admin.accounts.clearRateLimit')"
              >
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M12 6v6h4.5m4.5 0a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
              </button>
              <!-- Test Connection button -->
              <button
                @click="handleTest(row)"
                class="p-2 rounded-lg hover:bg-green-50 dark:hover:bg-green-900/20 text-gray-500 hover:text-green-600 dark:hover:text-green-400 transition-colors"
                :title="t('admin.accounts.testConnection')"
              >
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M5.25 5.653c0-.856.917-1.398 1.667-.986l11.54 6.347a1.125 1.125 0 010 1.972l-11.54 6.347a1.125 1.125 0 01-1.667-.986V5.653z" />
                </svg>
              </button>
              <button
                v-if="row.type === 'oauth' || row.type === 'setup-token'"
                @click="handleReAuth(row)"
                class="p-2 rounded-lg hover:bg-blue-50 dark:hover:bg-blue-900/20 text-gray-500 hover:text-blue-600 dark:hover:text-blue-400 transition-colors"
                :title="t('admin.accounts.reAuthorize')"
              >
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M13.19 8.688a4.5 4.5 0 011.242 7.244l-4.5 4.5a4.5 4.5 0 01-6.364-6.364l1.757-1.757m13.35-.622l1.757-1.757a4.5 4.5 0 00-6.364-6.364l-4.5 4.5a4.5 4.5 0 001.242 7.244" />
                </svg>
              </button>
              <button
                v-if="row.type === 'oauth' || row.type === 'setup-token'"
                @click="handleRefreshToken(row)"
                class="p-2 rounded-lg hover:bg-purple-50 dark:hover:bg-purple-900/20 text-gray-500 hover:text-purple-600 dark:hover:text-purple-400 transition-colors"
                :title="t('admin.accounts.refreshToken')"
              >
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M4.031 9.865a8.25 8.25 0 0113.803-3.7l3.181 3.182m0-4.991v4.99" />
                </svg>
              </button>
              <button
                @click="handleEdit(row)"
                class="p-2 rounded-lg hover:bg-gray-100 dark:hover:bg-dark-700 text-gray-500 hover:text-primary-600 dark:hover:text-primary-400 transition-colors"
                :title="t('common.edit')"
              >
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                  <path stroke-linecap="round" stroke-linejoin="round"
                    d="M16.862 4.487l1.687-1.688a1.875 1.875 0 112.652 2.652L10.582 16.07a4.5 4.5 0 01-1.897 1.13L6 18l.8-2.685a4.5 4.5 0 011.13-1.897l8.932-8.931zm0 0L19.5 7.125M18 14v4.75A2.25 2.25 0 0115.75 21H5.25A2.25 2.25 0 013 18.75V8.25A2.25 2.25 0 015.25 6H10" />
                </svg>
              </button>
              <button
                @click="handleDelete(row)"
                class="p-2 rounded-lg hover:bg-red-50 dark:hover:bg-red-900/20 text-gray-500 hover:text-red-600 dark:hover:text-red-400 transition-colors"
                :title="t('common.delete')"
              >
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                  <path stroke-linecap="round" stroke-linejoin="round"
                    d="M14.74 9l-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 01-2.244 2.077H8.084a2.25 2.25 0 01-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 00-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 013.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.964 51.964 0 00-3.32 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 00-7.5 0" />
                </svg>
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState
              :title="t('admin.accounts.noAccountsYet')"
              :description="t('admin.accounts.createFirstAccount')"
              :action-text="t('admin.accounts.createAccount')"
              @action="showCreateModal = true"
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

    <!-- Create Account Modal -->
    <CreateAccountModal
      :show="showCreateModal"
      :proxies="proxies"
      :groups="groups"
      @close="showCreateModal = false"
      @created="loadAccounts"
    />

    <!-- Edit Account Modal -->
    <EditAccountModal
      :show="showEditModal"
      :account="editingAccount"
      :proxies="proxies"
      :groups="groups"
      @close="closeEditModal"
      @updated="loadAccounts"
    />

    <!-- Re-Auth Modal -->
    <ReAuthAccountModal
      :show="showReAuthModal"
      :account="reAuthAccount"
      @close="closeReAuthModal"
      @reauthorized="loadAccounts"
    />

    <!-- Test Account Modal -->
    <AccountTestModal
      :show="showTestModal"
      :account="testingAccount"
      @close="closeTestModal"
    />

    <!-- Delete Confirmation Dialog -->
    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('admin.accounts.deleteAccount')"
      :message="t('admin.accounts.deleteConfirm', { name: deletingAccount?.name })"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { Account, Proxy, Group } from '@/types'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import { CreateAccountModal, EditAccountModal, ReAuthAccountModal } from '@/components/account'
import AccountStatusIndicator from '@/components/account/AccountStatusIndicator.vue'
import AccountUsageCell from '@/components/account/AccountUsageCell.vue'
import AccountTodayStatsCell from '@/components/account/AccountTodayStatsCell.vue'
import AccountTestModal from '@/components/account/AccountTestModal.vue'
import { formatRelativeTime } from '@/utils/format'

const { t } = useI18n()
const appStore = useAppStore()

// Table columns
const columns = computed<Column[]>(() => [
  { key: 'name', label: t('admin.accounts.columns.name'), sortable: true },
  { key: 'platform', label: t('admin.accounts.columns.platform'), sortable: true },
  { key: 'type', label: t('admin.accounts.columns.type'), sortable: true },
  { key: 'status', label: t('admin.accounts.columns.status'), sortable: true },
  { key: 'schedulable', label: t('admin.accounts.columns.schedulable'), sortable: true },
  { key: 'today_stats', label: t('admin.accounts.columns.todayStats'), sortable: false },
  { key: 'usage', label: t('admin.accounts.columns.usageWindows'), sortable: false },
  { key: 'priority', label: t('admin.accounts.columns.priority'), sortable: true },
  { key: 'last_used_at', label: t('admin.accounts.columns.lastUsed'), sortable: true },
  { key: 'actions', label: t('admin.accounts.columns.actions'), sortable: false }
])

// Filter options
const platformOptions = computed(() => [
  { value: '', label: t('admin.accounts.allPlatforms') },
  { value: 'anthropic', label: t('admin.accounts.platforms.anthropic') },
  { value: 'openai', label: t('admin.accounts.platforms.openai') }
])

const typeOptions = computed(() => [
  { value: '', label: t('admin.accounts.allTypes') },
  { value: 'oauth', label: t('admin.accounts.oauthType') },
  { value: 'setup-token', label: t('admin.accounts.setupToken') },
  { value: 'apikey', label: t('admin.accounts.apiKey') }
])

const statusOptions = computed(() => [
  { value: '', label: t('admin.accounts.allStatus') },
  { value: 'active', label: t('common.active') },
  { value: 'inactive', label: t('common.inactive') },
  { value: 'error', label: t('common.error') }
])

// State
const accounts = ref<Account[]>([])
const proxies = ref<Proxy[]>([])
const groups = ref<Group[]>([])
const loading = ref(false)
const searchQuery = ref('')
const filters = reactive({
  platform: '',
  type: '',
  status: '',
})
const pagination = reactive({
  page: 1,
  page_size: 20,
  total: 0,
  pages: 0
})

// Modal states
const showCreateModal = ref(false)
const showEditModal = ref(false)
const showReAuthModal = ref(false)
const showDeleteDialog = ref(false)
const showTestModal = ref(false)
const editingAccount = ref<Account | null>(null)
const reAuthAccount = ref<Account | null>(null)
const deletingAccount = ref<Account | null>(null)
const testingAccount = ref<Account | null>(null)
const togglingSchedulable = ref<number | null>(null)

// Rate limit / Overload helpers
const isRateLimited = (account: Account): boolean => {
  if (!account.rate_limit_reset_at) return false
  return new Date(account.rate_limit_reset_at) > new Date()
}

const isOverloaded = (account: Account): boolean => {
  if (!account.overload_until) return false
  return new Date(account.overload_until) > new Date()
}

// Data loading
const loadAccounts = async () => {
  loading.value = true
  try {
    const response = await adminAPI.accounts.list(
      pagination.page,
      pagination.page_size,
      {
        platform: filters.platform || undefined,
        type: filters.type || undefined,
        status: filters.status || undefined,
        search: searchQuery.value || undefined
      }
    )
    accounts.value = response.items
    pagination.total = response.total
    pagination.pages = response.pages
  } catch (error) {
    appStore.showError(t('admin.accounts.failedToLoad'))
    console.error('Error loading accounts:', error)
  } finally {
    loading.value = false
  }
}

const loadProxies = async () => {
  try {
    proxies.value = await adminAPI.proxies.getAllWithCount()
  } catch (error) {
    console.error('Error loading proxies:', error)
  }
}

const loadGroups = async () => {
  try {
    // Load groups for all platforms to support both Anthropic and OpenAI accounts
    groups.value = await adminAPI.groups.getAll()
  } catch (error) {
    console.error('Error loading groups:', error)
  }
}

// Search handling
let searchTimeout: ReturnType<typeof setTimeout>
const handleSearch = () => {
  clearTimeout(searchTimeout)
  searchTimeout = setTimeout(() => {
    pagination.page = 1
    loadAccounts()
  }, 300)
}

// Pagination
const handlePageChange = (page: number) => {
  pagination.page = page
  loadAccounts()
}

// Edit modal
const handleEdit = (account: Account) => {
  editingAccount.value = account
  showEditModal.value = true
}

const closeEditModal = () => {
  showEditModal.value = false
  editingAccount.value = null
}

// Re-Auth modal
const handleReAuth = (account: Account) => {
  reAuthAccount.value = account
  showReAuthModal.value = true
}

const closeReAuthModal = () => {
  showReAuthModal.value = false
  reAuthAccount.value = null
}

// Token refresh
const handleRefreshToken = async (account: Account) => {
  try {
    await adminAPI.accounts.refreshCredentials(account.id)
    appStore.showSuccess(t('admin.accounts.tokenRefreshed'))
    loadAccounts()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.accounts.failedToRefresh'))
    console.error('Error refreshing token:', error)
  }
}

// Delete
const handleDelete = (account: Account) => {
  deletingAccount.value = account
  showDeleteDialog.value = true
}

const confirmDelete = async () => {
  if (!deletingAccount.value) return

  try {
    await adminAPI.accounts.delete(deletingAccount.value.id)
    appStore.showSuccess(t('admin.accounts.accountDeleted'))
    showDeleteDialog.value = false
    deletingAccount.value = null
    loadAccounts()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.accounts.failedToDelete'))
    console.error('Error deleting account:', error)
  }
}

// Clear rate limit
const handleClearRateLimit = async (account: Account) => {
  try {
    await adminAPI.accounts.clearRateLimit(account.id)
    appStore.showSuccess(t('admin.accounts.rateLimitCleared'))
    loadAccounts()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.accounts.failedToClearRateLimit'))
    console.error('Error clearing rate limit:', error)
  }
}

// Reset account status (clear error and rate limit)
const handleResetStatus = async (account: Account) => {
  try {
    // Clear error status
    await adminAPI.accounts.clearError(account.id)
    // Also clear rate limit if exists
    if (isRateLimited(account) || isOverloaded(account)) {
      await adminAPI.accounts.clearRateLimit(account.id)
    }
    appStore.showSuccess(t('admin.accounts.statusReset'))
    loadAccounts()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.accounts.failedToResetStatus'))
    console.error('Error resetting account status:', error)
  }
}

// Toggle schedulable
const handleToggleSchedulable = async (account: Account) => {
  togglingSchedulable.value = account.id
  try {
    const updatedAccount = await adminAPI.accounts.setSchedulable(account.id, !account.schedulable)
    const index = accounts.value.findIndex(a => a.id === account.id)
    if (index !== -1) {
      accounts.value[index] = updatedAccount
    }
    appStore.showSuccess(
      updatedAccount.schedulable
        ? t('admin.accounts.schedulableEnabled')
        : t('admin.accounts.schedulableDisabled')
    )
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.accounts.failedToToggleSchedulable'))
    console.error('Error toggling schedulable:', error)
  } finally {
    togglingSchedulable.value = null
  }
}

// Test modal
const handleTest = (account: Account) => {
  testingAccount.value = account
  showTestModal.value = true
}

const closeTestModal = () => {
  showTestModal.value = false
  testingAccount.value = null
}

// Initialize
onMounted(() => {
  loadAccounts()
  loadProxies()
  loadGroups()
})
</script>
