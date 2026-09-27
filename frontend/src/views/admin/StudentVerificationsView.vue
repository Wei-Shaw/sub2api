<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <div class="relative w-full md:w-80">
            <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
            <input
              v-model="filters.keyword"
              type="text"
              class="input pl-10"
              :placeholder="t('admin.studentVerifications.searchPlaceholder')"
              @input="debounceLoad"
            />
          </div>
          <Select
            v-model="filters.status"
            class="w-40"
            :options="statusOptions"
            :placeholder="t('admin.studentVerifications.allStatus')"
            @update:model-value="reloadFromFirstPage"
          />
          <button class="btn btn-secondary px-2 md:px-3" :disabled="loading" :title="t('common.refresh')" @click="loadRecords">
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="records" :loading="loading">
          <template #cell-user_id="{ row }">
            <span class="font-mono text-sm text-gray-900 dark:text-white">
              {{ row.user_id > 0 ? `#${row.user_id}` : t('admin.studentVerifications.deletedUser') }}
            </span>
          </template>
          <template #cell-email="{ row }">
            <span class="font-mono text-sm text-gray-700 dark:text-gray-300">{{ row.email }}</span>
          </template>
          <template #cell-status="{ row }">
            <span :class="['badge whitespace-nowrap', statusBadgeClass(row.status)]">
              {{ t(`admin.studentVerifications.status.${row.status}`) }}
            </span>
          </template>
          <template #cell-granted_group_ids="{ row }">
            <span class="text-sm text-gray-700 dark:text-gray-300">
              {{ row.granted_group_ids?.length ? row.granted_group_ids.join(', ') : '-' }}
            </span>
          </template>
          <template #cell-rebate_rate_applied="{ row }">
            <span class="text-sm text-gray-700 dark:text-gray-300">
              {{ row.rebate_rate_applied != null ? `${row.rebate_rate_applied}%` : '-' }}
            </span>
          </template>
          <template #cell-expires_at="{ row }">
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ formatDateTime(row.expires_at) }}</span>
          </template>
          <template #cell-created_at="{ row }">
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ formatDateTime(row.created_at) }}</span>
          </template>
          <template #cell-actions="{ row }">
            <div class="flex items-center gap-2">
              <button
                v-if="row.status === 'active'"
                type="button"
                class="text-xs text-amber-600 hover:text-amber-700 dark:text-amber-400"
                @click="openRevoke(row)"
              >
                {{ t('admin.studentVerifications.revoke') }}
              </button>
              <button
                type="button"
                class="text-xs text-red-600 hover:text-red-700 dark:text-red-400"
                @click="openDelete(row)"
              >
                {{ t('admin.studentVerifications.delete') }}
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

    <!-- Revoke dialog -->
    <BaseDialog
      :show="revokeDialog"
      :title="t('admin.studentVerifications.revokeTitle')"
      width="normal"
      @close="revokeDialog = false"
    >
      <div class="space-y-4">
        <p class="text-sm text-gray-600 dark:text-gray-300">
          {{ t('admin.studentVerifications.revokeConfirm', { email: selected?.email }) }}
        </p>
        <div>
          <label class="input-label">{{ t('admin.studentVerifications.revokeReason') }}</label>
          <input
            v-model="revokeReason"
            type="text"
            class="input"
            :placeholder="t('admin.studentVerifications.revokeReasonPlaceholder')"
          />
        </div>
        <div class="flex justify-end gap-2">
          <button type="button" class="btn btn-secondary" @click="revokeDialog = false">
            {{ t('common.cancel') }}
          </button>
          <button type="button" class="btn btn-danger" :disabled="acting" @click="confirmRevoke">
            {{ t('admin.studentVerifications.revoke') }}
          </button>
        </div>
      </div>
    </BaseDialog>

    <ConfirmDialog
      :show="deleteDialog"
      :title="t('admin.studentVerifications.deleteTitle')"
      :message="t('admin.studentVerifications.deleteConfirm', { email: selected?.email })"
      :confirm-text="t('admin.studentVerifications.delete')"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="deleteDialog = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import type { Column } from '@/components/common/types'
import { useAppStore } from '@/stores/app'
import {
  adminStudentVerificationAPI,
  type AdminStudentVerificationRecord,
} from '@/api/admin/studentVerification'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime as formatDisplayDateTime } from '@/utils/format'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const records = ref<AdminStudentVerificationRecord[]>([])
const pagination = reactive({ page: 1, page_size: 20, total: 0 })
const filters = reactive({ keyword: '', status: '' as '' | 'active' | 'expired' | 'revoked' })

const revokeDialog = ref(false)
const deleteDialog = ref(false)
const revokeReason = ref('')
const acting = ref(false)
const selected = ref<AdminStudentVerificationRecord | null>(null)
let debounceTimer: ReturnType<typeof setTimeout> | null = null

const statusOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('admin.studentVerifications.allStatus') },
  { value: 'active', label: t('admin.studentVerifications.status.active') },
  { value: 'expired', label: t('admin.studentVerifications.status.expired') },
  { value: 'revoked', label: t('admin.studentVerifications.status.revoked') },
])

const columns = computed<Column[]>(() => [
  { key: 'user_id', label: t('admin.studentVerifications.columns.user') },
  { key: 'email', label: t('admin.studentVerifications.columns.email') },
  { key: 'status', label: t('admin.studentVerifications.columns.status') },
  { key: 'granted_group_ids', label: t('admin.studentVerifications.columns.groups') },
  { key: 'rebate_rate_applied', label: t('admin.studentVerifications.columns.rebate') },
  { key: 'expires_at', label: t('admin.studentVerifications.columns.expiresAt') },
  { key: 'created_at', label: t('admin.studentVerifications.columns.verifiedAt') },
  { key: 'actions', label: t('admin.studentVerifications.columns.actions') },
])

function statusBadgeClass(status: string): string {
  switch (status) {
    case 'active':
      return 'badge-success'
    case 'expired':
      return 'badge-warning'
    default:
      return 'badge-danger'
  }
}

function formatDateTime(value: string | null | undefined): string {
  return value ? formatDisplayDateTime(value) : '-'
}

async function loadRecords() {
  loading.value = true
  try {
    const res = await adminStudentVerificationAPI.listStudentVerifications({
      page: pagination.page,
      page_size: pagination.page_size,
      status: filters.status,
      keyword: filters.keyword,
    })
    records.value = res.items || []
    pagination.total = res.total || 0
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('common.error')))
  } finally {
    loading.value = false
  }
}

function debounceLoad() {
  if (debounceTimer) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(() => reloadFromFirstPage(), 300)
}

function reloadFromFirstPage() {
  pagination.page = 1
  void loadRecords()
}

function handlePageChange(page: number) {
  pagination.page = page
  void loadRecords()
}

function handlePageSizeChange(size: number) {
  pagination.page_size = size
  pagination.page = 1
  void loadRecords()
}

function openRevoke(row: AdminStudentVerificationRecord) {
  selected.value = row
  revokeReason.value = ''
  revokeDialog.value = true
}

function openDelete(row: AdminStudentVerificationRecord) {
  selected.value = row
  deleteDialog.value = true
}

async function confirmRevoke() {
  if (!selected.value) return
  acting.value = true
  try {
    await adminStudentVerificationAPI.revokeStudentVerification(selected.value.id, revokeReason.value)
    appStore.showSuccess(t('admin.studentVerifications.revokeSuccess'))
    revokeDialog.value = false
    await loadRecords()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('common.error')))
  } finally {
    acting.value = false
  }
}

async function confirmDelete() {
  if (!selected.value) return
  acting.value = true
  try {
    await adminStudentVerificationAPI.deleteStudentVerification(selected.value.id)
    appStore.showSuccess(t('admin.studentVerifications.deleteSuccess'))
    deleteDialog.value = false
    await loadRecords()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('common.error')))
  } finally {
    acting.value = false
  }
}

onMounted(loadRecords)
</script>
