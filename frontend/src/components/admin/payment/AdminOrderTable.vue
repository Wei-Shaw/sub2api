<template>
  <div class="space-y-4">
    <div class="card p-4">
      <div class="flex flex-wrap items-center gap-3">
        <div class="flex-1 sm:max-w-64">
          <input
            v-model="searchQuery"
            type="text"
            :placeholder="t('payment.admin.searchOrders')"
            class="input"
            @input="handleSearch"
          />
        </div>
        <Select
          v-model="filters.status"
          :options="statusFilterOptions"
          class="w-36"
          @change="emitFiltersChanged"
        />
        <Select
          v-model="filters.payment_type"
          :options="paymentTypeFilterOptions"
          class="w-40"
          @change="emitFiltersChanged"
        />
        <Select
          v-model="filters.order_type"
          :options="orderTypeFilterOptions"
          class="w-36"
          @change="emitFiltersChanged"
        />
        <div class="flex flex-1 flex-wrap items-center justify-end gap-2">
          <button
            @click="emit('refresh')"
            :disabled="loading"
            class="btn btn-secondary"
            :title="t('common.refresh')"
          >
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
        </div>
      </div>
    </div>

    <DataTable :columns="columns" :data="orders" :loading="loading">
      <template #cell-id="{ value }">
        <span class="font-mono text-sm">#{{ value }}</span>
      </template>

      <template #cell-user_id="{ value }">
        <span class="text-sm text-gray-600 dark:text-gray-400">#{{ value }}</span>
      </template>

      <template #cell-amount="{ value, row }">
        <div class="text-sm">
          <span class="font-medium text-gray-900 dark:text-white">${{ value.toFixed(2) }}</span>
          <span v-if="row.pay_amount !== value" class="ml-1 text-xs text-gray-500">
            ({{ t('payment.orders.payAmount') }}: ${{ row.pay_amount.toFixed(2) }})
          </span>
        </div>
      </template>

      <template #cell-payment_type="{ value }">
        <span class="text-sm text-gray-700 dark:text-gray-300">
          {{ t('payment.methods.' + value, value) }}
        </span>
      </template>

      <template #cell-status="{ value }">
        <span :class="['badge', statusBadgeClass(value)]">
          {{ t('payment.status.' + value.toLowerCase(), value) }}
        </span>
      </template>

      <template #cell-order_type="{ value }">
        <span class="text-sm text-gray-700 dark:text-gray-300">
          {{ t('payment.admin.' + value + 'Order', value) }}
        </span>
      </template>

      <template #cell-created_at="{ value }">
        <span class="text-xs text-gray-500 dark:text-gray-400">{{ formatDateTime(value) }}</span>
      </template>

      <template #cell-actions="{ row }">
        <div class="flex items-center gap-2">
          <button
            @click="emit('detail', row)"
            class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-50 hover:text-gray-700 dark:hover:bg-gray-800/50 dark:hover:text-gray-300"
          >
            <Icon name="eye" size="sm" />
            <span class="text-xs">{{ t('common.view') }}</span>
          </button>
          <button
            v-if="row.status === 'PENDING'"
            @click="emit('cancel', row)"
            class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-yellow-50 hover:text-yellow-600 dark:hover:bg-yellow-900/20 dark:hover:text-yellow-400"
          >
            <Icon name="x" size="sm" />
            <span class="text-xs">{{ t('payment.orders.cancel') }}</span>
          </button>
          <button
            v-if="row.status === 'FAILED'"
            @click="emit('retry', row)"
            class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-blue-50 hover:text-blue-600 dark:hover:bg-blue-900/20 dark:hover:text-blue-400"
          >
            <Icon name="refresh" size="sm" />
            <span class="text-xs">{{ t('payment.admin.retry') }}</span>
          </button>
          <button
            v-if="canRefundRow(row)"
            @click="emit('refund', row)"
            class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
          >
            <Icon name="dollar" size="sm" />
            <span class="text-xs">{{ t('payment.admin.refund') }}</span>
          </button>
        </div>
      </template>
    </DataTable>

    <Pagination
      v-if="total > 0"
      :page="page"
      :total="total"
      :page-size="pageSize"
      @update:page="emit('update:page', $event)"
      @update:pageSize="emit('update:pageSize', $event)"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PaymentOrder } from '@/types/payment'
import type { Column } from '@/components/common/types'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { statusBadgeClass, canRefund, formatOrderDateTime } from '@/components/payment/orderUtils'

const { t } = useI18n()

defineProps<{
  orders: PaymentOrder[]
  loading: boolean
  page: number
  pageSize: number
  total: number
}>()

const emit = defineEmits<{
  (e: 'detail', order: PaymentOrder): void
  (e: 'cancel', order: PaymentOrder): void
  (e: 'retry', order: PaymentOrder): void
  (e: 'refund', order: PaymentOrder): void
  (e: 'refresh'): void
  (e: 'update:page', page: number): void
  (e: 'update:pageSize', size: number): void
  (e: 'filter', filters: { keyword?: string; status?: string; payment_type?: string; order_type?: string }): void
}>()

const searchQuery = ref('')
const filters = reactive({ status: '', payment_type: '', order_type: '' })

let debounceTimer: ReturnType<typeof setTimeout> | null = null
function handleSearch() {
  if (debounceTimer) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(() => emitFiltersChanged(), 300)
}

function emitFiltersChanged() {
  emit('filter', {
    keyword: searchQuery.value || undefined,
    status: filters.status || undefined,
    payment_type: filters.payment_type || undefined,
    order_type: filters.order_type || undefined,
  })
}

const columns = computed<Column[]>(() => [
  { key: 'id', label: t('payment.orders.orderId') },
  { key: 'user_id', label: t('payment.orders.userId') },
  { key: 'amount', label: t('payment.orders.amount') },
  { key: 'payment_type', label: t('payment.orders.paymentMethod') },
  { key: 'status', label: t('payment.orders.status') },
  { key: 'order_type', label: t('payment.orders.orderType') },
  { key: 'created_at', label: t('payment.orders.createdAt') },
  { key: 'actions', label: t('payment.orders.actions') },
])

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

function canRefundRow(order: PaymentOrder): boolean {
  return canRefund(order.status)
}

function formatDateTime(dateStr: string): string {
  return formatOrderDateTime(dateStr)
}
</script>
