<template>
  <AppLayout>
    <div class="space-y-4">
      <!-- Filters -->
      <div class="border-b-2 border-accent pb-3">
        <div class="flex flex-wrap items-center gap-3">
          <div class="flex-1 sm:max-w-64">
            <input v-model="orderSearch" type="text" :placeholder="t('payment.admin.searchOrders')" class="input" @input="debounceLoadOrders" />
          </div>
          <Select v-model="orderFilters.status" :options="statusFilterOptions" class="w-36" @change="loadOrders" />
          <Select v-model="orderFilters.payment_type" :options="paymentTypeFilterOptions" class="w-40" @change="loadOrders" />
          <Select v-model="orderFilters.order_type" :options="orderTypeFilterOptions" class="w-36" @change="loadOrders" />
          <div class="flex flex-1 flex-wrap items-center justify-end gap-2">
            <button @click="loadOrders" :disabled="ordersLoading" class="btn btn-secondary" :title="t('common.refresh')" :aria-label="t('common.refresh')">
              <Icon name="refresh" size="md" :class="ordersLoading ? 'animate-spin' : ''" />
            </button>
          </div>
        </div>
      </div>

      <!-- Table -->
      <OrderTable :orders="orders" :loading="ordersLoading" show-user>
        <template #actions="{ row }">
          <div class="flex items-center gap-1">
            <button @click="showOrderDetail(row)" class="btn btn-ghost btn-sm">
              <Icon name="eye" size="sm" />
              {{ t('common.view') }}
            </button>
            <button v-if="row.status === 'PENDING'" @click="handleCancelOrder(row)" class="btn btn-ghost btn-sm text-warning hover:bg-warning-weak hover:text-warning-strong">
              <Icon name="x" size="sm" />
              {{ t('payment.orders.cancel') }}
            </button>
            <button v-if="row.status === 'FAILED'" @click="handleRetryOrder(row)" class="btn btn-ghost btn-sm text-accent">
              <Icon name="refresh" size="sm" />
              {{ t('payment.admin.retry') }}
            </button>
          </div>
        </template>
      </OrderTable>
      <Pagination v-if="orderPagination.total > 0" :page="orderPagination.page" :total="orderPagination.total" :page-size="orderPagination.page_size" @update:page="handleOrderPageChange" @update:pageSize="handleOrderPageSizeChange" />
    </div>

    <!-- Order Detail Dialog -->
    <BaseDialog :show="showDetailDialog" :title="t('payment.admin.orderDetail')" width="wide" @close="showDetailDialog = false">
      <div v-if="selectedOrder" class="space-y-4">
        <dl class="grid grid-cols-1 border-t border-border sm:grid-cols-2 sm:gap-x-6">
          <div class="flex items-baseline justify-between gap-3 border-b border-border py-2"><dt class="text-meta font-medium text-fg-muted">{{ t('payment.orders.orderId') }}</dt><dd class="font-mono text-body font-semibold text-fg">#{{ selectedOrder.id }}</dd></div>
          <div class="flex items-baseline justify-between gap-3 border-b border-border py-2"><dt class="text-meta font-medium text-fg-muted">{{ t('payment.orders.orderNo') }}</dt><dd class="min-w-0 break-all font-mono text-meta text-fg">{{ selectedOrder.out_trade_no }}</dd></div>
          <div class="flex items-baseline justify-between gap-3 border-b border-border py-2"><dt class="text-meta font-medium text-fg-muted">{{ t('payment.orders.status') }}</dt><dd class="text-body"><OrderStatusBadge :status="selectedOrder.status" /></dd></div>
          <div class="flex items-baseline justify-between gap-3 border-b border-border py-2"><dt class="text-meta font-medium text-fg-muted">{{ t('payment.orders.amount') }}</dt><dd class="text-body font-semibold tabular-nums text-fg">{{ creditedAmountSymbol }}{{ selectedOrder.amount.toFixed(2) }}</dd></div>
          <div class="flex items-baseline justify-between gap-3 border-b border-border py-2"><dt class="text-meta font-medium text-fg-muted">{{ t('payment.orders.payAmount') }}</dt><dd class="text-body font-bold tabular-nums text-fg">{{ paymentAmountSymbol(selectedOrder) }}{{ selectedOrder.pay_amount.toFixed(2) }}</dd></div>
          <div class="flex items-baseline justify-between gap-3 border-b border-border py-2"><dt class="text-meta font-medium text-fg-muted">{{ t('payment.orders.paymentMethod') }}</dt><dd class="text-body text-fg">{{ t('payment.methods.' + selectedOrder.payment_type, selectedOrder.payment_type) }}</dd></div>
          <div class="flex items-baseline justify-between gap-3 border-b border-border py-2"><dt class="text-meta font-medium text-fg-muted">{{ t('payment.admin.feeRate') }}</dt><dd class="text-body tabular-nums text-fg">{{ selectedOrder.fee_rate }}%</dd></div>
          <div class="flex items-baseline justify-between gap-3 border-b border-border py-2"><dt class="text-meta font-medium text-fg-muted">{{ t('payment.orders.createdAt') }}</dt><dd class="text-body tabular-nums text-fg">{{ formatDateTime(selectedOrder.created_at) }}</dd></div>
          <div class="flex items-baseline justify-between gap-3 border-b border-border py-2"><dt class="text-meta font-medium text-fg-muted">{{ t('payment.admin.expiresAt') }}</dt><dd class="text-body tabular-nums text-fg">{{ formatDateTime(selectedOrder.expires_at) }}</dd></div>
          <div v-if="selectedOrder.paid_at" class="flex items-baseline justify-between gap-3 border-b border-border py-2"><dt class="text-meta font-medium text-fg-muted">{{ t('payment.admin.paidAt') }}</dt><dd class="text-body tabular-nums text-fg">{{ formatDateTime(selectedOrder.paid_at) }}</dd></div>
          <div v-if="selectedOrder.refund_amount" class="flex items-baseline justify-between gap-3 border-b border-border py-2"><dt class="text-meta font-medium text-fg-muted">{{ t('payment.admin.refundAmount') }}</dt><dd class="text-body font-semibold tabular-nums text-danger">{{ creditedAmountSymbol }}{{ selectedOrder.refund_amount.toFixed(2) }}</dd></div>
          <div v-if="selectedOrder.refund_reason" class="border-b border-border py-2 sm:col-span-2"><dt class="text-meta font-medium text-fg-muted">{{ t('payment.admin.refundReason') }}</dt><dd class="mt-0.5 text-body text-fg">{{ selectedOrder.refund_reason }}</dd></div>
          <!-- Refund request info -->
          <div v-if="selectedOrder.refund_requested_at" class="pt-3 sm:col-span-2">
            <p class="text-label font-bold text-accent-strong">{{ t('payment.admin.refundRequestInfo') }}</p>
            <div class="mt-1 grid grid-cols-1 border-t border-border sm:grid-cols-2 sm:gap-x-6">
              <div class="flex items-baseline justify-between gap-3 border-b border-border py-2"><dt class="text-meta font-medium text-fg-muted">{{ t('payment.admin.refundRequestedAt') }}</dt><dd class="text-body tabular-nums text-fg">{{ formatDateTime(selectedOrder.refund_requested_at) }}</dd></div>
              <div class="flex items-baseline justify-between gap-3 border-b border-border py-2"><dt class="text-meta font-medium text-fg-muted">{{ t('payment.admin.refundRequestedBy') }}</dt><dd class="font-mono text-body text-fg">#{{ selectedOrder.refund_requested_by }}</dd></div>
              <div class="border-b border-border py-2 sm:col-span-2"><dt class="text-meta font-medium text-fg-muted">{{ t('payment.admin.refundRequestReason') }}</dt><dd class="mt-0.5 text-body text-fg">{{ selectedOrder.refund_request_reason }}</dd></div>
            </div>
          </div>
        </dl>
        <!-- Audit Logs -->
        <div v-if="orderAuditLogs.length > 0">
          <p class="text-label font-bold text-accent-strong">{{ t('payment.admin.auditLogs') }}</p>
          <div class="mt-1 max-h-48 divide-y divide-border overflow-y-auto border-y border-border">
            <div v-for="log in orderAuditLogs" :key="log.id" class="py-2">
              <div class="flex items-center justify-between">
                <span class="font-mono text-meta font-medium text-fg">{{ log.action }}</span>
                <span class="text-meta tabular-nums text-fg-subtle">{{ formatDateTime(log.created_at) }}</span>
              </div>
              <div v-if="log.detail" class="mt-1 break-all text-meta text-fg-muted">{{ log.detail }}</div>
              <div v-if="log.operator" class="mt-1 text-meta text-fg-subtle">{{ t('payment.admin.operator') }}: {{ log.operator }}</div>
            </div>
          </div>
        </div>
      </div>
    </BaseDialog>

  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminPaymentAPI } from '@/api/admin/payment'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { formatOrderDateTime } from '@/components/payment/orderUtils'
import type { PaymentOrder } from '@/types/payment'
import AppLayout from '@/components/layout/AppLayout.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import OrderStatusBadge from '@/components/payment/OrderStatusBadge.vue'
import OrderTable from '@/components/payment/OrderTable.vue'
import { currencySymbol } from '@/components/payment/currency'

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
const orderAuditLogs = ref<AuditLog[]>([])
const creditedAmountSymbol = currencySymbol('USD')

function paymentAmountSymbol(order: PaymentOrder | null | undefined): string {
  return currencySymbol(order?.currency)
}

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
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
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
  { value: 'REFUND_PENDING', label: t('payment.status.refund_pending') },
  { value: 'REFUND_FAILED', label: t('payment.status.refund_failed') },
])

const paymentTypeFilterOptions = computed(() => [
  { value: '', label: t('payment.admin.allPaymentTypes') },
  { value: 'gpmpay_bank_transfer', label: t('payment.methods.gpmpay_bank_transfer') },
  { value: 'nowpayments_crypto', label: t('payment.methods.nowpayments_crypto') },
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
  catch (err: unknown) { appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error'))) }
}

async function handleRetryOrder(order: PaymentOrder) {
  try { await adminPaymentAPI.retryRecharge(order.id); appStore.showSuccess(t('payment.admin.retrySuccess')); loadOrders() }
  catch (err: unknown) { appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error'))) }
}

function formatDateTime(dateStr: string): string { return formatOrderDateTime(dateStr) }

onMounted(() => loadOrders())
</script>
