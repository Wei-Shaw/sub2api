<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <div class="min-w-[260px] flex-1">
            <input
              v-model="filters.search"
              type="text"
              class="input"
              :placeholder="t('admin.expandAccounts.searchPlaceholder')"
              @input="debounceLoad"
            />
          </div>
          <Select
            v-model="filters.used"
            :options="usedOptions"
            class="w-40"
            @change="reloadFirstPage"
          />
          <Select
            v-model="filters.login_status"
            :options="loginStatusOptions"
            class="w-40"
            @change="reloadFirstPage"
          />
          <Select
            v-model="filters.account_type"
            :options="accountTypeOptions"
            class="w-40"
            @change="reloadFirstPage"
          />
          <div class="flex flex-1 items-center justify-end gap-2">
            <button class="btn btn-secondary" :disabled="loading" @click="loadItems" :title="t('common.refresh')">
              <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button class="btn btn-primary" @click="openCreate">
              <Icon name="plus" size="sm" class="mr-1" />
              {{ t('admin.expandAccounts.create') }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="items"
          :loading="loading"
          :server-side-sort="false"
        >
          <template #cell-email="{ value }">
            <span class="text-sm text-gray-900 dark:text-white">{{ value }}</span>
          </template>

          <template #cell-session_key="{ value, row }">
            <div class="flex items-center gap-2">
              <code class="max-w-[280px] truncate rounded bg-gray-100 px-2 py-1 font-mono text-xs text-gray-800 dark:bg-dark-700 dark:text-gray-100" :title="value">
                {{ value }}
              </code>
              <button
                class="text-gray-400 transition-colors hover:text-gray-600 dark:hover:text-gray-200"
                :title="t('common.copy')"
                @click="copySessionKey(value, row.id)"
              >
                <Icon :name="copiedId === row.id ? 'check' : 'copy'" size="sm" />
              </button>
            </div>
          </template>

          <template #cell-email_pwd="{ value }">
            <span class="text-sm text-gray-700 dark:text-gray-200">{{ value || '-' }}</span>
          </template>

          <template #cell-help_email="{ value }">
            <span class="text-sm text-gray-700 dark:text-gray-200">{{ value || '-' }}</span>
          </template>

          <template #cell-help_email_url="{ value }">
            <a
              v-if="value"
              :href="value"
              target="_blank"
              rel="noopener noreferrer"
              class="text-sm text-primary-600 hover:underline dark:text-primary-400"
              :title="value"
            >
              <span class="inline-block max-w-[220px] truncate align-middle">{{ value }}</span>
            </a>
            <span v-else class="text-sm text-gray-400 dark:text-dark-500">-</span>
          </template>

          <template #cell-channel="{ value }">
            <span class="text-sm text-gray-700 dark:text-gray-200">{{ value || '-' }}</span>
          </template>

          <template #cell-proxy_name="{ value }">
            <span class="text-sm text-gray-700 dark:text-gray-200" :title="value || ''">{{ value || '-' }}</span>
          </template>

          <template #cell-used="{ value }">
            <span
              :class="[
                'inline-flex rounded-full px-2 py-1 text-xs font-medium',
                value
                  ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
                  : 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
              ]"
            >
              {{ value ? t('admin.expandAccounts.used') : t('admin.expandAccounts.unused') }}
            </span>
          </template>

          <template #cell-login_status="{ value }">
            <span
              :class="[
                'inline-flex rounded-full px-2 py-1 text-xs font-medium',
                loginStatusBadgeClass(value)
              ]"
            >
              {{ loginStatusLabel(value) }}
            </span>
          </template>

          <template #cell-account_type="{ row }">
            <span
              :class="[
                'inline-flex rounded-full px-2 py-1 text-xs font-medium',
                row.account_id != null
                  ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
                  : 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200'
              ]"
            >
              {{ row.account_id != null ? t('admin.expandAccounts.accountTypeOld') : t('admin.expandAccounts.accountTypeNew') }}
            </span>
          </template>

          <template #cell-created_at="{ value }">
            <span class="text-sm text-gray-500 dark:text-dark-400">{{ formatDateTime(value) }}</span>
          </template>

          <template #cell-updated_at="{ value }">
            <span class="text-sm text-gray-500 dark:text-dark-400">{{ formatDateTime(value) }}</span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <button
                class="rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-dark-700 dark:hover:text-gray-200"
                :title="t('common.edit')"
                @click="openEdit(row)"
              >
                <Icon name="edit" size="sm" />
              </button>
              <button
                class="rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                :title="t('common.delete')"
                @click="promptDelete(row)"
              >
                <Icon name="trash" size="sm" />
              </button>
              <button
                class="rounded-lg p-1.5 transition-colors"
                :class="row.used
                  ? 'cursor-not-allowed text-gray-300 dark:text-dark-500'
                  : 'text-gray-500 hover:bg-emerald-50 hover:text-emerald-600 dark:hover:bg-emerald-900/20 dark:hover:text-emerald-400'"
                :title="row.used ? t('admin.expandAccounts.alreadyUsed') : t('admin.expandAccounts.markUsed')"
                :disabled="row.used || markingUsedId === row.id"
                @click="handleMarkUsed(row)"
              >
                <Icon name="check" size="sm" :class="markingUsedId === row.id ? 'animate-pulse' : ''" />
              </button>
            </div>
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :page-size="pagination.page_size"
          :total="pagination.total"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>

    <BaseDialog
      :show="showFormDialog"
      :title="editingItem ? t('admin.expandAccounts.edit') : t('admin.expandAccounts.create')"
      width="normal"
      @close="closeFormDialog"
    >
      <form id="expand-account-form" class="space-y-4" @submit.prevent="submitForm">
        <div>
          <label class="input-label">{{ t('common.email') }}</label>
          <input v-model.trim="form.email" class="input" type="text" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.expandAccounts.platform') }}</label>
          <input v-model.trim="form.platform" class="input" type="text" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.expandAccounts.subscriptionType') }}</label>
          <input v-model.trim="form.subscription_type" class="input" type="text" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.expandAccounts.country') }}</label>
          <input v-model.trim="form.country" class="input" type="text" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.expandAccounts.sessionKey') }}</label>
          <textarea v-model.trim="form.session_key" class="input min-h-[120px] font-mono" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.expandAccounts.emailPwd') }}</label>
          <input v-model.trim="form.email_pwd" class="input" type="text" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.expandAccounts.helpEmail') }}</label>
          <input v-model.trim="form.help_email" class="input" type="text" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.expandAccounts.helpEmailUrl') }}</label>
          <input v-model.trim="form.help_email_url" class="input" type="text" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.expandAccounts.channel') }}</label>
          <input v-model.trim="form.channel" class="input" type="text" />
        </div>
      </form>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button class="btn btn-secondary" @click="closeFormDialog">{{ t('common.cancel') }}</button>
          <button class="btn btn-primary" type="submit" form="expand-account-form" :disabled="saving">
            {{ saving ? t('common.saving') : t('common.save') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('admin.expandAccounts.deleteTitle')"
      :message="t('admin.expandAccounts.deleteConfirm')"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { ExpandAccount, CreateExpandAccountRequest } from '@/api/admin/expandAccounts'
import type { Column } from '@/components/common/types'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'
import { getPersistedPageSize, setPersistedPageSize } from '@/composables/usePersistedPageSize'
import { useClipboard } from '@/composables/useClipboard'
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
const { copyToClipboard } = useClipboard()

const items = ref<ExpandAccount[]>([])
const loading = ref(false)
const saving = ref(false)
const copiedId = ref<number | null>(null)
const showFormDialog = ref(false)
const showDeleteDialog = ref(false)
const editingItem = ref<ExpandAccount | null>(null)
const deletingItem = ref<ExpandAccount | null>(null)
const markingUsedId = ref<number | null>(null)

const filters = reactive({
  search: '',
  used: 'all',
  login_status: 'all' as 'all' | '0' | '1' | '2',
  account_type: 'all' as 'all' | 'old' | 'new'
})

const pagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0
})

const form = reactive<CreateExpandAccountRequest>({
  email: '',
  platform: '',
  subscription_type: '',
  country: '',
  session_key: '',
  email_pwd: '',
  help_email: '',
  help_email_url: '',
  channel: ''
})

const columns: Column[] = [
  { key: 'email', label: t('common.email') },
  { key: 'platform', label: t('admin.expandAccounts.platform') },
  { key: 'subscription_type', label: t('admin.expandAccounts.subscriptionType') },
  { key: 'country', label: t('admin.expandAccounts.country') },
  { key: 'session_key', label: t('admin.expandAccounts.sessionKey') },
  { key: 'email_pwd', label: t('admin.expandAccounts.emailPwd') },
  { key: 'help_email', label: t('admin.expandAccounts.helpEmail') },
  { key: 'help_email_url', label: t('admin.expandAccounts.helpEmailUrl') },
  { key: 'channel', label: t('admin.expandAccounts.channel') },
  { key: 'proxy_name', label: t('admin.expandAccounts.proxyName') },
  { key: 'used', label: t('common.status') },
  { key: 'login_status', label: t('admin.expandAccounts.loginStatus') },
  { key: 'account_type', label: t('admin.expandAccounts.accountType') },
  { key: 'created_at', label: t('admin.expandAccounts.createdAt') },
  { key: 'updated_at', label: t('admin.expandAccounts.updatedAt') },
  { key: 'actions', label: t('common.actions') }
]

const usedOptions = [
  { label: t('common.all'), value: 'all' },
  { label: t('admin.expandAccounts.unused'), value: 'unused' },
  { label: t('admin.expandAccounts.used'), value: 'used' }
]

const loginStatusOptions = [
  { label: t('common.all'), value: 'all' },
  { label: t('admin.expandAccounts.loginStatusPending'), value: '0' },
  { label: t('admin.expandAccounts.loginStatusSuccess'), value: '1' },
  { label: t('admin.expandAccounts.loginStatusFailed'), value: '2' }
]

const accountTypeOptions = [
  { label: t('common.all'), value: 'all' },
  { label: t('admin.expandAccounts.accountTypeOld'), value: 'old' },
  { label: t('admin.expandAccounts.accountTypeNew'), value: 'new' }
]

function loginStatusLabel(value: number | null | undefined): string {
  switch (Number(value ?? 0)) {
    case 1:
      return t('admin.expandAccounts.loginStatusSuccess')
    case 2:
      return t('admin.expandAccounts.loginStatusFailed')
    default:
      return t('admin.expandAccounts.loginStatusPending')
  }
}

function loginStatusBadgeClass(value: number | null | undefined): string {
  switch (Number(value ?? 0)) {
    case 1:
      return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
    case 2:
      return 'bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-300'
    default:
      return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200'
  }
}

let debounceTimer: ReturnType<typeof setTimeout> | null = null

function debounceLoad() {
  if (debounceTimer) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(() => {
    reloadFirstPage()
  }, 300)
}

async function loadItems() {
  loading.value = true
  try {
    const data = await adminAPI.expandAccounts.list(pagination.page, pagination.page_size, {
      search: filters.search.trim() || undefined,
      used: filters.used === 'all' ? undefined : filters.used,
      login_status: filters.login_status === 'all' ? undefined : Number(filters.login_status),
      account_type: filters.account_type === 'all' ? undefined : filters.account_type
    })
    items.value = data.items
    pagination.total = data.total
    pagination.page = data.page
    pagination.page_size = data.page_size
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.expandAccounts.loadFailed')))
  } finally {
    loading.value = false
  }
}

function reloadFirstPage() {
  pagination.page = 1
  void loadItems()
}

function handlePageChange(page: number) {
  pagination.page = page
  void loadItems()
}

function handlePageSizeChange(pageSize: number) {
  pagination.page = 1
  pagination.page_size = pageSize
  setPersistedPageSize(pageSize)
  void loadItems()
}

function resetForm() {
  form.email = ''
  form.platform = ''
  form.subscription_type = ''
  form.country = ''
  form.session_key = ''
  form.email_pwd = ''
  form.help_email = ''
  form.help_email_url = ''
  form.channel = ''
}

function openCreate() {
  editingItem.value = null
  resetForm()
  showFormDialog.value = true
}

function openEdit(item: ExpandAccount) {
  editingItem.value = item
  form.email = item.email
  form.platform = item.platform
  form.subscription_type = item.subscription_type
  form.country = item.country
  form.session_key = item.session_key
  form.email_pwd = item.email_pwd ?? ''
  form.help_email = item.help_email ?? ''
  form.help_email_url = item.help_email_url ?? ''
  form.channel = item.channel ?? ''
  showFormDialog.value = true
}

function closeFormDialog() {
  showFormDialog.value = false
  editingItem.value = null
  resetForm()
}

async function submitForm() {
  if (!form.email || !form.platform || !form.subscription_type || !form.country || !form.session_key) {
    appStore.showError(t('admin.expandAccounts.requiredFields'))
    return
  }

  saving.value = true
  try {
    if (editingItem.value) {
      await adminAPI.expandAccounts.update(editingItem.value.id, { ...form })
      appStore.showSuccess(t('admin.expandAccounts.updated'))
    } else {
      await adminAPI.expandAccounts.create({ ...form })
      appStore.showSuccess(t('admin.expandAccounts.created'))
    }
    closeFormDialog()
    await loadItems()
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.expandAccounts.saveFailed')))
  } finally {
    saving.value = false
  }
}

function promptDelete(item: ExpandAccount) {
  deletingItem.value = item
  showDeleteDialog.value = true
}

async function confirmDelete() {
  if (!deletingItem.value) return
  try {
    await adminAPI.expandAccounts.deleteAccount(deletingItem.value.id)
    appStore.showSuccess(t('admin.expandAccounts.deleted'))
    showDeleteDialog.value = false
    deletingItem.value = null
    await loadItems()
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.expandAccounts.deleteFailed')))
  }
}

async function handleMarkUsed(item: ExpandAccount) {
  if (item.used) return
  markingUsedId.value = item.id
  try {
    await adminAPI.expandAccounts.markUsed(item.id)
    appStore.showSuccess(t('admin.expandAccounts.markUsedSuccess'))
    await loadItems()
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.expandAccounts.markUsedFailed')))
  } finally {
    markingUsedId.value = null
  }
}

async function copySessionKey(value: string, id: number) {
  const ok = await copyToClipboard(value, t('admin.expandAccounts.sessionKeyCopied'))
  if (!ok) return
  copiedId.value = id
  window.setTimeout(() => {
    if (copiedId.value === id) copiedId.value = null
  }, 1200)
}

onMounted(() => {
  void loadItems()
})
</script>

