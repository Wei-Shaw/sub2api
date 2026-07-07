<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <div class="flex-1 sm:max-w-64">
            <input
              v-model="searchQuery"
              type="text"
              :placeholder="t('admin.modelPricing.search', 'Search model...')"
              class="input"
              @input="handleSearch"
            />
          </div>

          <Select
            v-model="sourceFilter"
            :options="sourceFilterOptions"
            class="w-36"
            @change="loadData"
          />

          <div class="flex flex-1 flex-wrap items-center justify-end gap-2">
            <button
              @click="loadData"
              :disabled="loading"
              class="btn btn-secondary"
              :title="t('common.refresh')"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button
              @click="handleSyncFromRemote"
              :disabled="syncing"
              class="btn btn-secondary"
            >
              <Icon name="download" size="md" class="mr-1" />
              {{ syncing ? t('admin.modelPricing.syncing', 'Syncing...') : t('admin.modelPricing.syncNow', 'Sync from Remote') }}
            </button>
            <button
              @click="toggleAutoSync"
              :class="['btn', syncStatus.auto_sync_enabled ? 'btn-primary' : 'btn-secondary']"
            >
              <svg class="mr-1 h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M4.031 9.865a8.25 8.25 0 0113.803-3.7l3.181 3.182" />
              </svg>
              {{ t('admin.modelPricing.autoSync', 'Auto Sync') }}
            </button>
            <button @click="openCreateDialog" class="btn btn-primary">
              <Icon name="plus" size="md" class="mr-1" />
              {{ t('admin.modelPricing.add', 'Add') }}
            </button>
          </div>
        </div>

        <div class="mt-2 flex flex-wrap items-center gap-4 text-sm text-gray-500 dark:text-dark-400">
          <span v-if="syncStatus.last_synced_at">
            {{ t('admin.modelPricing.lastSynced', 'Last synced') }}: {{ formatDateTime(syncStatus.last_synced_at) }}
          </span>
          <span v-if="syncStatus.model_count > 0">
            {{ t('admin.modelPricing.modelCount', 'Models') }}: {{ syncStatus.model_count }}
          </span>
        </div>
      </template>

      <template #table>
        <div v-if="selectedIds.length > 0" class="flex items-center gap-3 rounded-lg border border-blue-200 bg-blue-50 px-4 py-2 dark:border-blue-800 dark:bg-blue-900/20">
          <span class="text-sm text-blue-700 dark:text-blue-300">
            {{ selectedIds.length }} selected
          </span>
          <button
            @click="handleBulkDelete"
            class="btn btn-sm btn-danger"
          >
            {{ t('admin.modelPricing.bulkDelete', 'Bulk Delete') }}
          </button>
          <button
            @click="clearSelection"
            class="btn btn-sm btn-secondary"
          >
            {{ t('common.cancel') }}
          </button>
        </div>

        <DataTable
          :columns="columns"
          :data="items"
          :loading="loading"
          default-sort-key="model"
          default-sort-order="asc"
        >
          <template #header-select>
            <input
              type="checkbox"
              class="h-4 w-4 cursor-pointer rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              :checked="allSelected"
              @change="toggleSelectAll"
            />
          </template>
          <template #cell-select="{ row }">
            <input
              type="checkbox"
              class="h-4 w-4 cursor-pointer rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              :checked="isSelected(row.id)"
              @change="toggleSelect(row.id)"
            />
          </template>
          <template #cell-model="{ value }">
            <span class="font-mono text-sm font-medium text-gray-900 dark:text-white">{{ value }}</span>
          </template>
          <template #cell-litellm_provider="{ value }">
            <span class="text-sm text-gray-600 dark:text-gray-400">{{ value }}</span>
          </template>
          <template #cell-input_cost_per_token="{ row }">
            <span class="font-mono text-sm text-gray-700 dark:text-gray-300">
              {{ formatCostPerMTok(row.input_cost_per_token) }}
            </span>
          </template>
          <template #cell-output_cost_per_token="{ row }">
            <span class="font-mono text-sm text-gray-700 dark:text-gray-300">
              {{ formatCostPerMTok(row.output_cost_per_token) }}
            </span>
          </template>
          <template #cell-cache_creation_input_token_cost="{ row }">
            <span class="font-mono text-sm text-gray-700 dark:text-gray-300">
              {{ formatCostPerMTok(row.cache_creation_input_token_cost) }}
            </span>
          </template>
          <template #cell-cache_read_input_token_cost="{ row }">
            <span class="font-mono text-sm text-gray-700 dark:text-gray-300">
              {{ formatCostPerMTok(row.cache_read_input_token_cost) }}
            </span>
          </template>
          <template #cell-source="{ value }">
            <span
              :class="[
                'badge',
                value === 'remote' ? 'badge-primary' : 'badge-gray'
              ]"
            >
              {{ value === 'remote' ? t('admin.modelPricing.remote', 'Remote') : t('admin.modelPricing.manual', 'Manual') }}
            </span>
          </template>
          <template #cell-locked="{ value }">
            <svg
              v-if="value"
              class="h-4 w-4 text-amber-500"
              fill="currentColor"
              viewBox="0 0 20 20"
            >
              <path fill-rule="evenodd" d="M5 9V7a5 5 0 0110 0v2a2 2 0 012 2v5a2 2 0 01-2 2H5a2 2 0 01-2-2v-5a2 2 0 012-2zm8-2v2H7V7a3 3 0 016 0z" clip-rule="evenodd" />
            </svg>
            <svg
              v-else
              class="h-4 w-4 text-gray-400 dark:text-dark-500"
              fill="currentColor"
              viewBox="0 0 20 20"
            >
              <path d="M10 2a5 5 0 00-5 5v2a2 2 0 00-2 2v5a2 2 0 002 2h10a2 2 0 002-2v-5a2 2 0 00-2-2H7V7a3 3 0 015.905-.75 1 1 0 001.937-.5A5.002 5.002 0 0010 2z" />
            </svg>
          </template>
          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <button
                @click="openEditDialog(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
              >
                <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M16.862 4.487l1.687-1.688a1.875 1.875 0 112.652 2.652L10.582 16.07a4.5 4.5 0 01-1.897 1.13L6 18l.8-2.685a4.5 4.5 0 011.13-1.897l8.932-8.931zm0 0L19.5 7.125M18 14v4.75A2.25 2.25 0 0115.75 21H5.25A2.25 2.25 0 013 18.75V8.25A2.25 2.25 0 015.25 6H10" />
                </svg>
                <span class="text-xs">{{ t('common.edit') }}</span>
              </button>
              <button
                @click="handleDelete(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
              >
                <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M14.74 9l-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 01-2.244 2.077H8.084a2.25 2.25 0 01-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 00-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 013.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.964 51.964 0 00-3.32 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 00-7.5 0" />
                </svg>
                <span class="text-xs">{{ t('common.delete') }}</span>
              </button>
            </div>
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>

    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('admin.modelPricing.delete', 'Delete')"
      :message="t('admin.modelPricing.confirmDelete', 'Are you sure you want to delete this entry?')"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />

    <ConfirmDialog
      :show="showBulkDeleteDialog"
      :title="t('admin.modelPricing.bulkDelete', 'Bulk Delete')"
      :message="t('admin.modelPricing.confirmBulkDelete', { count: selectedIds.length })"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="confirmBulkDelete"
      @cancel="showBulkDeleteDialog = false"
    />

    <BaseDialog
      :show="showFormDialog"
      :title="isEditing ? t('admin.modelPricing.edit', 'Edit Pricing') : t('admin.modelPricing.add', 'Add Pricing')"
      width="wide"
      @close="showFormDialog = false"
    >
      <form @submit.prevent="handleSave" class="space-y-4">
        <div>
          <label class="input-label">{{ t('admin.modelPricing.model', 'Model') }}</label>
          <input
            v-model="form.model"
            type="text"
            required
            class="input"
            :placeholder="t('admin.modelPricing.model', 'Model')"
          />
        </div>

        <div>
          <label class="input-label">{{ t('admin.modelPricing.provider', 'Provider') }}</label>
          <input
            v-model="form.litellm_provider"
            type="text"
            required
            class="input"
            :placeholder="t('admin.modelPricing.provider', 'Provider')"
          />
        </div>

        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="input-label">{{ t('admin.modelPricing.inputCost', 'Input ($/MTok)') }}</label>
            <input
              v-model.number="form.input_cost_per_mtok"
              type="number"
              step="0.01"
              min="0"
              class="input"
              placeholder="0.00"
            />
          </div>
          <div>
            <label class="input-label">{{ t('admin.modelPricing.outputCost', 'Output ($/MTok)') }}</label>
            <input
              v-model.number="form.output_cost_per_mtok"
              type="number"
              step="0.01"
              min="0"
              class="input"
              placeholder="0.00"
            />
          </div>
        </div>

        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="input-label">{{ t('admin.modelPricing.cacheWriteCost', 'Cache Write ($/MTok)') }}</label>
            <input
              v-model.number="form.cache_write_cost_per_mtok"
              type="number"
              step="0.01"
              min="0"
              class="input"
              placeholder="0.00"
            />
          </div>
          <div>
            <label class="input-label">{{ t('admin.modelPricing.cacheReadCost', 'Cache Read ($/MTok)') }}</label>
            <input
              v-model.number="form.cache_read_cost_per_mtok"
              type="number"
              step="0.01"
              min="0"
              class="input"
              placeholder="0.00"
            />
          </div>
        </div>

        <div class="flex items-center gap-6">
          <label class="flex items-center gap-2">
            <input
              v-model="form.locked"
              type="checkbox"
              class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
            />
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('admin.modelPricing.locked', 'Locked') }}</span>
          </label>
          <label class="flex items-center gap-2">
            <input
              v-model="form.supports_prompt_caching"
              type="checkbox"
              class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
            />
            <span class="text-sm text-gray-700 dark:text-gray-300">Supports Prompt Caching</span>
          </label>
        </div>
      </form>

      <template #footer>
        <div class="flex justify-end space-x-3">
          <button
            type="button"
            @click="showFormDialog = false"
            class="rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-200 dark:hover:bg-dark-600 dark:focus:ring-offset-dark-800"
          >
            {{ t('admin.modelPricing.cancel', 'Cancel') }}
          </button>
          <button
            type="button"
            :disabled="saving"
            @click="handleSave"
            class="rounded-md bg-primary-600 px-4 py-2 text-sm font-medium text-white hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 dark:focus:ring-offset-dark-800 disabled:opacity-50"
          >
            {{ saving ? '...' : t('admin.modelPricing.save', 'Save') }}
          </button>
        </div>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { modelPricingAPI } from '@/api/admin/model-pricing'
import type { ModelPricingEntry, SyncStatus } from '@/api/admin/model-pricing'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import { formatDateTime } from '@/utils/format'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()

const items = ref<ModelPricingEntry[]>([])
const loading = ref(false)
const searchQuery = ref('')
const sourceFilter = ref('')
const syncing = ref(false)
const saving = ref(false)
const showFormDialog = ref(false)
const showDeleteDialog = ref(false)
const showBulkDeleteDialog = ref(false)
const isEditing = ref(false)
const editingId = ref<number | null>(null)
const deletingEntry = ref<ModelPricingEntry | null>(null)
const selectedIds = ref<number[]>([])
const syncStatus = reactive<SyncStatus>({
  auto_sync_enabled: false,
  last_synced_at: null,
  model_count: 0
})

const pagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0,
  pages: 0
})

const form = reactive({
  model: '',
  litellm_provider: '',
  input_cost_per_mtok: 0,
  output_cost_per_mtok: 0,
  cache_write_cost_per_mtok: 0,
  cache_read_cost_per_mtok: 0,
  locked: false,
  supports_prompt_caching: false
})

const columns = computed<Column[]>(() => [
  { key: 'select', label: '', sortable: false },
  { key: 'model', label: t('admin.modelPricing.model', 'Model'), sortable: true },
  { key: 'litellm_provider', label: t('admin.modelPricing.provider', 'Provider'), sortable: true },
  { key: 'input_cost_per_token', label: t('admin.modelPricing.inputCost', 'Input ($/MTok)'), sortable: false },
  { key: 'output_cost_per_token', label: t('admin.modelPricing.outputCost', 'Output ($/MTok)'), sortable: false },
  { key: 'cache_creation_input_token_cost', label: t('admin.modelPricing.cacheWriteCost', 'Cache Write ($/MTok)'), sortable: false },
  { key: 'cache_read_input_token_cost', label: t('admin.modelPricing.cacheReadCost', 'Cache Read ($/MTok)'), sortable: false },
  { key: 'source', label: t('admin.modelPricing.source', 'Source'), sortable: true },
  { key: 'locked', label: t('admin.modelPricing.locked', 'Locked'), sortable: true },
  { key: 'actions', label: t('admin.modelPricing.actions', 'Actions'), sortable: false }
])

const sourceFilterOptions = computed(() => [
  { value: '', label: t('admin.modelPricing.source', 'All Sources') },
  { value: 'remote', label: t('admin.modelPricing.remote', 'Remote') },
  { value: 'manual', label: t('admin.modelPricing.manual', 'Manual') }
])

const allSelected = computed(() => {
  if (items.value.length === 0) return false
  return items.value.every(item => selectedIds.value.includes(item.id))
})

let abortController: AbortController | null = null
let searchTimeout: ReturnType<typeof setTimeout>

const formatCostPerMTok = (costPerToken: number | null | undefined): string => {
  if (costPerToken == null || costPerToken === 0) return '-'
  const perMTok = costPerToken * 1_000_000
  if (perMTok < 0.01) return `$${perMTok.toFixed(4)}`
  if (perMTok < 1) return `$${perMTok.toFixed(3)}`
  return `$${perMTok.toFixed(2)}`
}

const isSelected = (id: number) => selectedIds.value.includes(id)

const toggleSelect = (id: number) => {
  const idx = selectedIds.value.indexOf(id)
  if (idx >= 0) {
    selectedIds.value.splice(idx, 1)
  } else {
    selectedIds.value.push(id)
  }
}

const toggleSelectAll = (event: Event) => {
  const target = event.target as HTMLInputElement
  if (target.checked) {
    selectedIds.value = items.value.map(item => item.id)
  } else {
    selectedIds.value = []
  }
}

const clearSelection = () => {
  selectedIds.value = []
}

const loadData = async () => {
  if (abortController) {
    abortController.abort()
  }
  const currentController = new AbortController()
  abortController = currentController
  loading.value = true

  try {
    const response = await modelPricingAPI.list({
      page: pagination.page,
      page_size: pagination.page_size,
      search: searchQuery.value || undefined,
      source: sourceFilter.value || undefined
    })
    if (currentController.signal.aborted) return
    items.value = response.items || []
    pagination.total = response.total || 0
  } catch (error: any) {
    if (
      currentController.signal.aborted ||
      error?.name === 'AbortError' ||
      error?.code === 'ERR_CANCELED'
    ) {
      return
    }
    appStore.showError(t('common.error'))
    console.error('Error loading model pricing:', error)
  } finally {
    if (abortController === currentController && !currentController.signal.aborted) {
      loading.value = false
      abortController = null
    }
  }
}

const loadSyncStatus = async () => {
  try {
    const status = await modelPricingAPI.getSyncStatus()
    syncStatus.auto_sync_enabled = status.auto_sync_enabled
    syncStatus.last_synced_at = status.last_synced_at
    syncStatus.model_count = status.model_count
  } catch (error) {
    console.error('Error loading sync status:', error)
  }
}

const handleSearch = () => {
  clearTimeout(searchTimeout)
  searchTimeout = setTimeout(() => {
    pagination.page = 1
    loadData()
  }, 300)
}

const handlePageChange = (page: number) => {
  pagination.page = page
  loadData()
}

const handlePageSizeChange = (size: number) => {
  pagination.page_size = size
  pagination.page = 1
  loadData()
}

const resetForm = () => {
  form.model = ''
  form.litellm_provider = ''
  form.input_cost_per_mtok = 0
  form.output_cost_per_mtok = 0
  form.cache_write_cost_per_mtok = 0
  form.cache_read_cost_per_mtok = 0
  form.locked = false
  form.supports_prompt_caching = false
}

const openCreateDialog = () => {
  isEditing.value = false
  editingId.value = null
  resetForm()
  showFormDialog.value = true
}

const openEditDialog = (entry: ModelPricingEntry) => {
  isEditing.value = true
  editingId.value = entry.id
  form.model = entry.model
  form.litellm_provider = entry.litellm_provider
  form.input_cost_per_mtok = (entry.input_cost_per_token ?? 0) * 1_000_000
  form.output_cost_per_mtok = (entry.output_cost_per_token ?? 0) * 1_000_000
  form.cache_write_cost_per_mtok = (entry.cache_creation_input_token_cost ?? 0) * 1_000_000
  form.cache_read_cost_per_mtok = (entry.cache_read_input_token_cost ?? 0) * 1_000_000
  form.locked = entry.locked
  form.supports_prompt_caching = entry.supports_prompt_caching
  showFormDialog.value = true
}

const handleSave = async () => {
  saving.value = true
  try {
    const payload: Partial<ModelPricingEntry> = {
      model: form.model,
      litellm_provider: form.litellm_provider,
      input_cost_per_token: form.input_cost_per_mtok / 1_000_000,
      output_cost_per_token: form.output_cost_per_mtok / 1_000_000,
      cache_creation_input_token_cost: form.cache_write_cost_per_mtok / 1_000_000 || null,
      cache_read_input_token_cost: form.cache_read_cost_per_mtok / 1_000_000 || null,
      locked: form.locked,
      supports_prompt_caching: form.supports_prompt_caching
    }

    if (isEditing.value && editingId.value) {
      await modelPricingAPI.update(editingId.value, payload)
      appStore.showSuccess(t('admin.modelPricing.updateSuccess', 'Pricing updated successfully'))
    } else {
      await modelPricingAPI.create(payload)
      appStore.showSuccess(t('admin.modelPricing.createSuccess', 'Pricing created successfully'))
    }
    showFormDialog.value = false
    loadData()
  } catch (error: any) {
    appStore.showError(error?.response?.data?.detail || t('common.error'))
    console.error('Error saving pricing:', error)
  } finally {
    saving.value = false
  }
}

const handleDelete = (entry: ModelPricingEntry) => {
  deletingEntry.value = entry
  showDeleteDialog.value = true
}

const confirmDelete = async () => {
  if (!deletingEntry.value) return
  try {
    await modelPricingAPI.delete(deletingEntry.value.id)
    appStore.showSuccess(t('admin.modelPricing.deleteSuccess', 'Pricing deleted successfully'))
    showDeleteDialog.value = false
    deletingEntry.value = null
    loadData()
  } catch (error: any) {
    appStore.showError(error?.response?.data?.detail || t('common.error'))
    console.error('Error deleting pricing:', error)
  }
}

const handleBulkDelete = () => {
  showBulkDeleteDialog.value = true
}

const confirmBulkDelete = async () => {
  try {
    await modelPricingAPI.bulkDelete([...selectedIds.value])
    appStore.showSuccess(t('admin.modelPricing.deleteSuccess', 'Pricing deleted successfully'))
    showBulkDeleteDialog.value = false
    clearSelection()
    loadData()
  } catch (error: any) {
    appStore.showError(error?.response?.data?.detail || t('common.error'))
    console.error('Error bulk deleting pricing:', error)
  }
}

const handleSyncFromRemote = async () => {
  syncing.value = true
  try {
    const status = await modelPricingAPI.syncFromRemote()
    syncStatus.auto_sync_enabled = status.auto_sync_enabled
    syncStatus.last_synced_at = status.last_synced_at
    syncStatus.model_count = status.model_count
    appStore.showSuccess(t('admin.modelPricing.syncSuccess', 'Synced from remote successfully'))
    loadData()
  } catch (error: any) {
    appStore.showError(error?.response?.data?.detail || t('common.error'))
    console.error('Error syncing pricing:', error)
  } finally {
    syncing.value = false
  }
}

const toggleAutoSync = async () => {
  try {
    const result = await modelPricingAPI.setAutoSync(!syncStatus.auto_sync_enabled)
    syncStatus.auto_sync_enabled = result.auto_sync_enabled
    appStore.showSuccess(
      syncStatus.auto_sync_enabled
        ? t('admin.modelPricing.autoSync', 'Auto Sync') + ' enabled'
        : t('admin.modelPricing.autoSync', 'Auto Sync') + ' disabled'
    )
  } catch (error: any) {
    appStore.showError(error?.response?.data?.detail || t('common.error'))
    console.error('Error toggling auto sync:', error)
  }
}

onMounted(() => {
  loadData()
  loadSyncStatus()
})

onUnmounted(() => {
  clearTimeout(searchTimeout)
  abortController?.abort()
})
</script>
