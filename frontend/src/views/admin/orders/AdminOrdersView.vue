<template>
  <AppLayout>
    <div class="space-y-4">
      <!-- Filters -->
      <div class="card p-4">
        <div class="flex flex-wrap items-center gap-3">
          <div class="flex-1 sm:max-w-64">
            <input v-model="orderSearch" type="text" :placeholder="t('payment.admin.searchOrders')" class="input" @input="debounceLoadOrders" />
          </div>
          <Select v-model="orderFilters.status" :options="statusFilterOptions" class="w-36" @change="loadOrders" />
          <Select v-model="orderFilters.payment_type" :options="paymentTypeFilterOptions" class="w-40" @change="loadOrders" />
          <Select v-model="orderFilters.order_type" :options="orderTypeFilterOptions" class="w-36" @change="loadOrders" />
          <div class="flex flex-1 flex-wrap items-center justify-end gap-2">
            <button @click="loadOrders" :disabled="ordersLoading" class="btn btn-secondary" :title="t('common.refresh')">
              <Icon name="refresh" size="md" :class="ordersLoading ? 'animate-spin' : ''" />
            </button>
          </div>
        </div>
      </div>

      <!-- Table -->
      <OrderTable :orders="orders" :loading="ordersLoading" show-user>
        <template #actions="{ row }">
          <div class="flex items-center gap-1">
            <button @click="showOrderDetail(row)" class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-dark-600">
              <Icon name="eye" size="sm" />
              {{ t('common.view') }}
            </button>
            <button v-if="row.status === 'PENDING'" @click="handleCancelOrder(row)" class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-yellow-600 hover:bg-yellow-50 dark:text-yellow-400 dark:hover:bg-yellow-900/20">
              <Icon name="x" size="sm" />
              {{ t('payment.orders.cancel') }}
            </button>
            <button v-if="row.status === 'FAILED'" @click="handleRetryOrder(row)" class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-blue-600 hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20">
              <Icon name="refresh" size="sm" />
              {{ t('payment.admin.retry') }}
            </button>
            <template v-if="row.status === 'REFUND_REQUESTED'">
              <span v-if="row.refund_amount" class="rounded-full bg-purple-100 px-1.5 py-0.5 text-xs font-medium text-purple-700 dark:bg-purple-900/30 dark:text-purple-300">${{ row.refund_amount.toFixed(2) }}</span>
              <button @click="openRefundDialog(row)" class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-purple-600 hover:bg-purple-50 dark:text-purple-400 dark:hover:bg-purple-900/20">
                <Icon name="check" size="sm" />
                {{ t('payment.admin.approveRefund') }}
              </button>
            </template>
            <button v-else-if="row.status === 'REFUND_FAILED'" @click="openRefundDialog(row)" class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-purple-600 hover:bg-purple-50 dark:text-purple-400 dark:hover:bg-purple-900/20">
              <Icon name="refresh" size="sm" />
              {{ t('payment.admin.retryRefund') }}
            </button>
            <button v-else-if="row.status === 'COMPLETED' || row.status === 'PARTIALLY_REFUNDED'" @click="openRefundDialog(row)" class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20">
              <Icon name="dollar" size="sm" />
              {{ t('payment.admin.refund') }}
            </button>
          </div>
        </template>
      </OrderTable>
      <Pagination v-if="orderPagination.total > 0" :page="orderPagination.page" :total="orderPagination.total" :page-size="orderPagination.page_size" @update:page="handleOrderPageChange" @update:pageSize="handleOrderPageSizeChange" />
    </div>

    <!-- Order Detail Dialog -->
    <BaseDialog :show="showDetailDialog" :title="t('payment.admin.orderDetail')" width="wide" @close="showDetailDialog = false">
      <div v-if="selectedOrder" class="space-y-4">
        <div class="grid grid-cols-2 gap-4">
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.orderId') }}</p><p class="font-mono text-sm font-medium text-gray-900 dark:text-white">#{{ selectedOrder.id }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.orderNo') }}</p><p class="text-sm font-medium text-gray-900 dark:text-white">{{ selectedOrder.out_trade_no }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.status') }}</p><OrderStatusBadge :status="selectedOrder.status" /></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.amount') }}</p><p class="text-sm font-medium text-gray-900 dark:text-white">${{ selectedOrder.amount.toFixed(2) }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.payAmount') }}</p><p class="text-sm font-medium text-gray-900 dark:text-white">${{ selectedOrder.pay_amount.toFixed(2) }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.paymentMethod') }}</p><p class="text-sm text-gray-700 dark:text-gray-300">{{ t('payment.methods.' + selectedOrder.payment_type, selectedOrder.payment_type) }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.feeRate') }}</p><p class="text-sm text-gray-700 dark:text-gray-300">{{ (selectedOrder.fee_rate * 100).toFixed(1) }}%</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.createdAt') }}</p><p class="text-sm text-gray-700 dark:text-gray-300">{{ formatDateTime(selectedOrder.created_at) }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.expiresAt') }}</p><p class="text-sm text-gray-700 dark:text-gray-300">{{ formatDateTime(selectedOrder.expires_at) }}</p></div>
          <div v-if="selectedOrder.paid_at"><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.paidAt') }}</p><p class="text-sm text-gray-700 dark:text-gray-300">{{ formatDateTime(selectedOrder.paid_at) }}</p></div>
          <div v-if="selectedOrder.refund_amount"><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.refundAmount') }}</p><p class="text-sm font-medium text-red-600 dark:text-red-400">${{ selectedOrder.refund_amount.toFixed(2) }}</p></div>
          <div v-if="selectedOrder.refund_reason" class="col-span-2"><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.refundReason') }}</p><p class="text-sm text-gray-700 dark:text-gray-300">{{ selectedOrder.refund_reason }}</p></div>
          <!-- Refund request info -->
          <div v-if="selectedOrder.refund_requested_at" class="col-span-2 border-t border-gray-200 pt-3 dark:border-dark-600">
            <p class="mb-2 text-xs font-medium text-purple-600 dark:text-purple-400">{{ t('payment.admin.refundRequestInfo') }}</p>
            <div class="grid grid-cols-2 gap-4">
              <div>
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.refundRequestedAt') }}</p>
                <p class="text-sm text-gray-700 dark:text-gray-300">{{ formatDateTime(selectedOrder.refund_requested_at) }}</p>
              </div>
              <div>
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.refundRequestedBy') }}</p>
                <p class="text-sm text-gray-700 dark:text-gray-300">#{{ selectedOrder.refund_requested_by }}</p>
              </div>
              <div class="col-span-2">
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.refundRequestReason') }}</p>
                <p class="text-sm text-gray-700 dark:text-gray-300">{{ selectedOrder.refund_request_reason }}</p>
              </div>
            </div>
          </div>
        </div>
        <!-- Audit Logs -->
        <div v-if="orderAuditLogs.length > 0" class="border-t border-gray-200 pt-4 dark:border-dark-600">
          <p class="mb-2 text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('payment.admin.auditLogs') }}</p>
          <div class="max-h-48 space-y-2 overflow-y-auto">
            <div v-for="log in orderAuditLogs" :key="log.id" class="rounded-lg border border-gray-100 bg-gray-50 p-2.5 dark:border-dark-600 dark:bg-dark-800">
              <div class="flex items-center justify-between">
                <span class="text-xs font-medium text-gray-700 dark:text-gray-300">{{ log.action }}</span>
                <span class="text-xs text-gray-400">{{ formatDateTime(log.created_at) }}</span>
              </div>
              <div v-if="log.detail" class="mt-1 break-all text-xs text-gray-500 dark:text-gray-400">{{ log.detail }}</div>
              <div v-if="log.operator" class="mt-1 text-xs text-gray-400">{{ t('payment.admin.operator') }}: {{ log.operator }}</div>
            </div>
          </div>
        </div>
      </div>
    </BaseDialog>

    <AdminRefundDialog :show="showRefundDialog" :order="selectedOrder" :submitting="refundSubmitting" @confirm="handleRefund" @cancel="showRefundDialog = false" />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminPaymentAPI } from '@/api/admin/payment'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatOrderDateTime } from '@/components/payment/orderUtils'
import type { PaymentOrder } from '@/types/payment'
import AppLayout from '@/components/layout/AppLayout.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import AdminRefundDialog from '@/components/admin/payment/AdminRefundDialog.vue'
import OrderStatusBadge from '@/components/payment/OrderStatusBadge.vue'
import OrderTable from '@/components/payment/OrderTable.vue'

interface AuditLog {
  id: number
  action: string
  detail: string | null
  operator: string | null
  created_at: string
}

const { t } = useI18n()
const appStore = useAppStore()

const ordersLoading = ref(false)
const orders = ref<PaymentOrder[]>([])
const orderSearch = ref('')
const orderFilters = reactive({ status: '', payment_type: '', order_type: '' })
const orderPagination = reactive({ page: 1, page_size: 20, total: 0 })
const selectedOrder = ref<PaymentOrder | null>(null)
const showDetailDialog = ref(false)
const showRefundDialog = ref(false)
const refundSubmitting = ref(false)
const orderAuditLogs = ref<AuditLog[]>([])

let debounceTimer: ReturnType<typeof setTimeout> | null = null
function debounceLoadOrders() {
  if (debounceTimer) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(() => loadOrders(), 300)
}

async function loadOrders() {
  ordersLoading.value = true
  try {
    const res = await adminPaymentAPI.getOrders({
      page: orderPagination.page, page_size: orderPagination.page_size,
      keyword: orderSearch.value || undefined, status: orderFilters.status || undefined,
      payment_type: orderFilters.payment_type || undefined, order_type: orderFilters.order_type || undefined,
    })
    orders.value = res.data.items || []
    orderPagination.total = res.data.total || 0
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally { ordersLoading.value = false }
}

function handleOrderPageChange(page: number) { orderPagination.page = page; loadOrders() }
function handleOrderPageSizeChange(size: number) { orderPagination.page_size = size; orderPagination.page = 1; loadOrders() }

const statusFilterOptions = computed(() => [
  { value: '', label: t('payment.admin.allStatuses') },
  { value: 'PENDING', label: t('payment.status.pending') },
  { value: 'PAID', label: t('payment.status.paid') },
  { value: 'COMPLETED', label: t('payment.status.completed') },
  { value: 'EXPIRED', label: t('payment.status.expired') },
  { value: 'CANCELLED', label: t('payment.status.cancelled') },
  { value: 'FAILED', label: t('payment.status.failed') },
  { value: 'REFUNDED', label: t('payment.status.refunded') },
  { value: 'REFUND_REQUESTED', label: t('payment.status.refund_requested') },
  { value: 'REFUND_FAILED', label: t('payment.status.refund_failed') },
])

const paymentTypeFilterOptions = computed(() => [
  { value: '', label: t('payment.admin.allPaymentTypes') },
  { value: 'alipay', label: t('payment.methods.alipay') },
  { value: 'wxpay', label: t('payment.methods.wxpay') },
  { value: 'stripe', label: t('payment.methods.stripe') },
])

const orderTypeFilterOptions = computed(() => [
  { value: '', label: t('payment.admin.allOrderTypes') },
  { value: 'balance', label: t('payment.admin.balanceOrder') },
  { value: 'subscription', label: t('payment.admin.subscriptionOrder') },
])

async function showOrderDetail(order: PaymentOrder) {
  selectedOrder.value = order
  orderAuditLogs.value = []
  showDetailDialog.value = true
  try {
    const res = await adminPaymentAPI.getOrder(order.id)
    const data = res.data as unknown as Record<string, unknown>
    if (data.order) selectedOrder.value = data.order as PaymentOrder
    orderAuditLogs.value = ((data.auditLogs || data.audit_logs || []) as unknown) as AuditLog[]
  } catch (_err: unknown) { /* keep cached order data */ }
}

async function handleCancelOrder(order: PaymentOrder) {
  try { await adminPaymentAPI.cancelOrder(order.id); appStore.showSuccess(t('payment.admin.orderCancelled')); loadOrders() }
  catch (err: unknown) { appStore.showError(extractApiErrorMessage(err, t('common.error'))) }
}

async function handleRetryOrder(order: PaymentOrder) {
  try { await adminPaymentAPI.retryRecharge(order.id); appStore.showSuccess(t('payment.admin.retrySuccess')); loadOrders() }
  catch (err: unknown) { appStore.showError(extractApiErrorMessage(err, t('common.error'))) }
}

function openRefundDialog(order: PaymentOrder) { selectedOrder.value = order; showRefundDialog.value = true }

async function handleRefund(data: { amount: number; reason: string; deduct_balance: boolean; force: boolean }) {
  if (!selectedOrder.value) return
  refundSubmitting.value = true
  try {
    await adminPaymentAPI.refundOrder(selectedOrder.value.id, { amount: data.amount, reason: data.reason, deduct_balance: data.deduct_balance, force: data.force })
    appStore.showSuccess(t('payment.admin.refundSuccess')); showRefundDialog.value = false; loadOrders()
  } catch (err: unknown) { appStore.showError(extractApiErrorMessage(err, t('common.error'))) }
  finally { refundSubmitting.value = false }
}

function formatDateTime(dateStr: string): string { return formatOrderDateTime(dateStr) }

onMounted(() => loadOrders())
</script>
