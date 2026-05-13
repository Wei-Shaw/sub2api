<template>
  <AppLayout>
    <div class="space-y-4">
      <div class="card p-4">
        <div class="grid gap-3 md:grid-cols-[160px_1fr_180px_180px_auto] md:items-end">
          <div>
            <label class="input-label">{{ t('invoice.filters.status') }}</label>
            <Select
              v-model="filters.status"
              :options="statusFilterOptions"
              class="mt-1"
              @change="handleFilterChange"
            />
          </div>
          <div>
            <label class="input-label">{{ t('invoice.admin.keywordPlaceholder') }}</label>
            <input
              v-model="filters.keyword"
              type="text"
              class="input mt-1 w-full"
              :placeholder="t('invoice.admin.keywordPlaceholder')"
              @keyup.enter="handleFilterChange"
            />
          </div>
          <div>
            <label class="input-label">{{ t('invoice.filters.startDate') }}</label>
            <input
              v-model="filters.start_time"
              type="date"
              class="input mt-1 w-full"
              @change="handleFilterChange"
            />
          </div>
          <div>
            <label class="input-label">{{ t('invoice.filters.endDate') }}</label>
            <input
              v-model="filters.end_time"
              type="date"
              class="input mt-1 w-full"
              @change="handleFilterChange"
            />
          </div>
          <div class="flex gap-2">
            <button type="button" class="btn btn-primary" @click="handleFilterChange">
              {{ t('common.search') }}
            </button>
            <button type="button" class="btn btn-secondary" @click="resetFilters">
              {{ t('common.reset') }}
            </button>
          </div>
        </div>
      </div>

      <div class="card overflow-hidden">
        <div v-if="loading" class="flex items-center justify-center py-16">
          <Icon name="refresh" size="lg" class="animate-spin text-gray-400" />
        </div>
        <div v-else-if="items.length === 0" class="flex flex-col items-center justify-center gap-3 py-16 text-gray-500 dark:text-gray-400">
          <Icon name="document" size="xl" />
          <p class="text-sm">{{ t('invoice.empty.requests') }}</p>
        </div>
        <div v-else class="divide-y divide-gray-200 dark:divide-dark-700">
          <article v-for="item in items" :key="item.id" class="p-4">
            <div class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
              <div class="min-w-0 space-y-2">
                <div class="flex flex-wrap items-center gap-2">
                  <span class="font-mono text-sm font-semibold text-gray-900 dark:text-white">{{ item.serial_no }}</span>
                  <span :class="statusBadgeClass(item.status)">
                    {{ t(`invoice.status.${item.status}`) }}
                  </span>
                  <span
                    v-if="item.profile_snapshot?.invoice_type === 'vat_special'"
                    class="rounded-full bg-purple-50 px-2 py-0.5 text-xs font-medium text-purple-700 dark:bg-purple-900/30 dark:text-purple-300"
                  >
                    {{ t('invoice.invoiceTypes.vat_special') }}
                  </span>
                  <span
                    v-if="item.has_refunded_orders"
                    class="rounded-full bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-700 dark:bg-amber-900/30 dark:text-amber-300"
                    :title="t('invoice.refundCoupling.tooltip')"
                  >
                    {{ t('invoice.refundCoupling.badge') }}
                  </span>
                  <span class="text-xs text-gray-500 dark:text-gray-400">
                    {{ item.username || '-' }}
                    <template v-if="item.user_email">
                      ({{ item.user_email }})
                    </template>
                  </span>
                </div>
                <div class="grid gap-x-8 gap-y-1 text-sm text-gray-600 dark:text-gray-300 md:grid-cols-2">
                  <div>
                    <span class="text-gray-500 dark:text-gray-400">{{ t('invoice.fields.title') }}:</span>
                    <span class="ml-1 font-medium text-gray-900 dark:text-white">{{ item.profile_snapshot.title }}</span>
                  </div>
                  <div>
                    <span class="text-gray-500 dark:text-gray-400">{{ t('invoice.fields.taxNumber') }}:</span>
                    <span class="ml-1 break-all font-mono">{{ item.profile_snapshot.tax_number }}</span>
                  </div>
                  <div>
                    <span class="text-gray-500 dark:text-gray-400">{{ t('invoice.fields.email') }}:</span>
                    <span class="ml-1 break-all">{{ item.profile_snapshot.email }}</span>
                  </div>
                  <div>
                    <span class="text-gray-500 dark:text-gray-400">{{ t('invoice.fields.createdAt') }}:</span>
                    <span class="ml-1">{{ formatDateTime(item.created_at) }}</span>
                  </div>
                  <div v-if="item.invoice_no">
                    <span class="text-gray-500 dark:text-gray-400">{{ t('invoice.fields.invoiceNo') }}:</span>
                    <span class="ml-1 font-mono font-medium text-gray-700 dark:text-gray-200">{{ item.invoice_no }}</span>
                  </div>
                  <div v-if="item.processed_at">
                    <span class="text-gray-500 dark:text-gray-400">{{ t('invoice.fields.processedAt') }}:</span>
                    <span class="ml-1">{{ formatDateTime(item.processed_at) }}</span>
                  </div>
                </div>
              </div>
              <div class="shrink-0 text-left lg:text-right">
                <div class="text-lg font-semibold text-gray-900 dark:text-white">{{ formatCurrency(item.total_amount) }}</div>
                <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('invoice.requests.orderCount', { count: item.orders?.length || 0 }) }}</div>
              </div>
            </div>

            <div v-if="item.reject_reason" class="mt-3 rounded-md bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">
              {{ item.reject_reason }}
            </div>

            <div v-if="item.orders?.length" class="mt-3 flex flex-wrap gap-2">
              <span
                v-for="order in item.orders"
                :key="order.id"
                class="rounded-md bg-gray-100 px-2 py-1 font-mono text-xs text-gray-700 dark:bg-dark-700 dark:text-gray-300"
                :title="formatCurrency(order.pay_amount)"
              >
                {{ order.out_trade_no }} ({{ formatCurrency(order.pay_amount) }})
              </span>
            </div>

            <div class="mt-3 flex flex-wrap items-center justify-end gap-2 border-t border-gray-100 pt-3 dark:border-dark-700">
              <button
                v-if="item.status === 'pending'"
                type="button"
                class="btn btn-secondary btn-sm"
                @click="openRejectDialog(item)"
              >
                <Icon name="x" size="sm" class="mr-1" />
                {{ t('invoice.admin.reject') }}
              </button>
              <button
                v-if="item.status === 'pending'"
                type="button"
                class="btn btn-primary btn-sm"
                @click="openCompleteDialog(item)"
              >
                <Icon name="check" size="sm" class="mr-1" />
                {{ t('invoice.admin.complete') }}
              </button>
            </div>
          </article>
        </div>

        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </div>
    </div>

    <BaseDialog
      :show="completeDialogOpen"
      :title="t('invoice.admin.completeTitle')"
      width="normal"
      @close="closeCompleteDialog"
    >
      <form class="space-y-4" @submit.prevent="submitComplete">
        <div>
          <label class="input-label">{{ t('invoice.admin.summaryLabel') }}</label>
          <p class="mt-1 text-sm text-gray-600 dark:text-gray-300">
            {{ activeRequest?.serial_no }} · {{ activeRequest?.profile_snapshot?.title }} · {{ activeRequest ? formatCurrency(activeRequest.total_amount) : '' }}
          </p>
        </div>
        <div>
          <label class="input-label">{{ t('invoice.fields.invoiceNo') }} <span class="text-red-500">*</span></label>
          <input
            v-model="completeForm.invoice_no"
            type="text"
            class="input mt-1 w-full"
            maxlength="64"
            :placeholder="t('invoice.admin.invoiceNoPlaceholder')"
            required
          />
        </div>
        <div>
          <label class="input-label">{{ t('invoice.admin.fileLabel') }} <span class="text-red-500">*</span></label>
          <div class="mt-1 flex items-center gap-3">
            <button
              type="button"
              class="btn btn-secondary"
              @click="triggerFilePicker"
            >
              <Icon name="document" size="sm" class="mr-2" />
              {{ completeForm.file ? t('common.reselect') : t('common.selectFile') }}
            </button>
            <span v-if="completeForm.file" class="truncate text-sm text-gray-700 dark:text-gray-300">
              {{ completeForm.file.name }} ({{ formatFileSize(completeForm.file.size) }})
            </span>
            <span v-else class="text-sm text-gray-400">{{ t('common.noFileSelected') }}</span>
          </div>
          <input
            ref="fileInputRef"
            type="file"
            class="hidden"
            accept=".pdf,.png,.jpg,.jpeg,.zip,.xls,.xlsx"
            @change="onFileChange"
          />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('invoice.admin.fileHint') }}</p>
          <p v-if="activeRequest?.profile_snapshot?.email" class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('invoice.admin.fileHintEmail', { email: activeRequest.profile_snapshot.email }) }}
          </p>
        </div>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="closeCompleteDialog">{{ t('common.cancel') }}</button>
          <button type="button" class="btn btn-primary" :disabled="actionLoading || !canSubmitComplete" @click="submitComplete">
            {{ actionLoading ? t('common.processing') : t('invoice.admin.sendInvoiceEmail') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <BaseDialog
      :show="rejectDialogOpen"
      :title="t('invoice.admin.rejectTitle')"
      width="normal"
      @close="closeRejectDialog"
    >
      <form class="space-y-4" @submit.prevent="submitReject">
        <div>
          <p class="text-sm text-gray-600 dark:text-gray-300">
            {{ activeRequest?.serial_no }} · {{ activeRequest?.profile_snapshot?.title }}
          </p>
        </div>
        <div>
          <label class="input-label">{{ t('invoice.admin.reasonLabel') }} <span class="text-red-500">*</span></label>
          <textarea
            v-model="rejectForm.reason"
            rows="4"
            maxlength="1000"
            required
            class="input mt-1 w-full"
            :placeholder="t('invoice.admin.reasonPlaceholder')"
          />
        </div>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="closeRejectDialog">{{ t('common.cancel') }}</button>
          <button type="button" class="btn btn-primary" :disabled="actionLoading || !rejectForm.reason.trim()" @click="submitReject">
            {{ actionLoading ? t('common.processing') : t('invoice.admin.reject') }}
          </button>
        </div>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminInvoiceAPI } from '@/api/admin/invoices'
import { useAppStore } from '@/stores'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { formatCurrency, formatDateTime } from '@/utils/format'
import type { SelectOption } from '@/types'
import type { AdminInvoiceListParams, AdminInvoiceRequest, InvoiceStatus } from '@/types/invoice'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const actionLoading = ref(false)
const items = ref<AdminInvoiceRequest[]>([])

const pagination = reactive({ page: 1, page_size: 20, total: 0 })
const filters = reactive<{ status: InvoiceStatus | ''; keyword: string; start_time: string; end_time: string }>({
  status: '',
  keyword: '',
  start_time: '',
  end_time: ''
})

const completeDialogOpen = ref(false)
const rejectDialogOpen = ref(false)
const activeRequest = ref<AdminInvoiceRequest | null>(null)
const fileInputRef = ref<HTMLInputElement | null>(null)

const completeForm = reactive<{ invoice_no: string; file: File | null }>({
  invoice_no: '',
  file: null
})
const rejectForm = reactive<{ reason: string }>({ reason: '' })

const statusFilterOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('common.all') },
  { value: 'pending', label: t('invoice.status.pending') },
  { value: 'completed', label: t('invoice.status.completed') },
  { value: 'rejected', label: t('invoice.status.rejected') }
])

const canSubmitComplete = computed(() => completeForm.invoice_no.trim() !== '' && completeForm.file !== null)

async function fetchList() {
  loading.value = true
  try {
    const params: AdminInvoiceListParams = {
      page: pagination.page,
      page_size: pagination.page_size
    }
    if (filters.status) params.status = filters.status
    if (filters.keyword.trim()) params.keyword = filters.keyword.trim()
    if (filters.start_time) params.start_time = filters.start_time
    if (filters.end_time) params.end_time = filters.end_time

    const res = await adminInvoiceAPI.list(params)
    items.value = res.data.items || []
    pagination.total = res.data.total || 0
  } catch (err: unknown) {
    showError(err)
  } finally {
    loading.value = false
  }
}

function handleFilterChange() {
  pagination.page = 1
  fetchList()
}

function resetFilters() {
  filters.status = ''
  filters.keyword = ''
  filters.start_time = ''
  filters.end_time = ''
  handleFilterChange()
}

function handlePageChange(page: number) {
  pagination.page = page
  fetchList()
}

function handlePageSizeChange(size: number) {
  pagination.page_size = size
  pagination.page = 1
  fetchList()
}

function openCompleteDialog(item: AdminInvoiceRequest) {
  activeRequest.value = item
  completeForm.invoice_no = ''
  completeForm.file = null
  if (fileInputRef.value) fileInputRef.value.value = ''
  completeDialogOpen.value = true
}

function closeCompleteDialog() {
  completeDialogOpen.value = false
  activeRequest.value = null
}

function openRejectDialog(item: AdminInvoiceRequest) {
  activeRequest.value = item
  rejectForm.reason = ''
  rejectDialogOpen.value = true
}

function closeRejectDialog() {
  rejectDialogOpen.value = false
  activeRequest.value = null
}

function onFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  completeForm.file = input.files && input.files.length > 0 ? input.files[0] : null
}

function triggerFilePicker() {
  fileInputRef.value?.click()
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`
}

async function submitComplete() {
  if (!activeRequest.value || !completeForm.file) return
  const invoiceNo = completeForm.invoice_no.trim()
  if (!invoiceNo) {
    appStore.showError(t('invoice.admin.invoiceNoRequired'))
    return
  }
  actionLoading.value = true
  try {
    await adminInvoiceAPI.complete(activeRequest.value.id, invoiceNo, completeForm.file)
    appStore.showSuccess(t('invoice.admin.completed'))
    closeCompleteDialog()
    await fetchList()
  } catch (err: unknown) {
    showError(err)
  } finally {
    actionLoading.value = false
  }
}

async function submitReject() {
  if (!activeRequest.value) return
  const reason = rejectForm.reason.trim()
  if (!reason) return
  actionLoading.value = true
  try {
    await adminInvoiceAPI.reject(activeRequest.value.id, reason)
    appStore.showSuccess(t('invoice.admin.rejected'))
    closeRejectDialog()
    await fetchList()
  } catch (err: unknown) {
    showError(err)
  } finally {
    actionLoading.value = false
  }
}

function statusBadgeClass(status: InvoiceStatus): string {
  const base = 'rounded-full px-2 py-0.5 text-xs font-medium'
  switch (status) {
    case 'pending':
      return `${base} bg-yellow-50 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-300`
    case 'completed':
      return `${base} bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300`
    case 'rejected':
      return `${base} bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300`
    default:
      return `${base} bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300`
  }
}

function showError(err: unknown) {
  appStore.showError(extractI18nErrorMessage(err, t, 'invoice.errors', t('common.error')))
}

onMounted(() => {
  fetchList()
})
</script>
