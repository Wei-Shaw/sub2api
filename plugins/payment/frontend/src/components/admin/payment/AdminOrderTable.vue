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

      <template #cell-pay_amount="{ value }">
        <span class="text-sm font-medium text-gray-900 dark:text-white">¥{{ formatMoney(value) }}</span>
      </template>

      <template #cell-fee_rate="{ row }">
        <span
          v-if="hasFeeRate(row.fee_rate)"
          class="text-sm text-gray-700 dark:text-gray-300"
        >{{ row.fee_rate }}%</span>
        <span v-else class="text-xs text-gray-400">—</span>
      </template>

      <template #cell-credited_amount="{ row }">
        <span class="text-sm text-gray-700 dark:text-gray-300">
          {{ row.order_type === 'balance' ? '$' : '¥' }}{{ formatMoney(row.amount) }}
        </span>
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
            class="hover-tint-warning flex flex-col items-center gap-0.5 rounded-lg p-1.5"
          >
            <Icon name="x" size="sm" />
            <span class="text-xs">{{ t('payment.orders.cancel') }}</span>
          </button>
          <button
            v-if="row.status === 'FAILED'"
            @click="emit('retry', row)"
            class="hover-tint-info flex flex-col items-center gap-0.5 rounded-lg p-1.5"
          >
            <Icon name="refresh" size="sm" />
            <span class="text-xs">{{ t('payment.admin.retry') }}</span>
          </button>
          <button
            v-if="canRefundRow(row)"
            @click="emit('refund', row)"
            class="hover-tint-danger flex flex-col items-center gap-0.5 rounded-lg p-1.5"
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
import type { PaymentOrder } from '../../../types/payment'
import type { Column } from '@sub2api/plugin-sdk'
import { DataTable } from '@sub2api/plugin-sdk'
import { Pagination } from '@sub2api/plugin-sdk'
import { Select } from '@sub2api/plugin-sdk'
import { Icon } from '@sub2api/plugin-sdk'
import { statusBadgeClass, canRefund, formatOrderDateTime } from '../../payment/orderUtils'
import { money, formatMoney } from '../../../utils/decimal'

const { t } = useI18n()

function hasFeeRate(rate: string | number | null | undefined): boolean {
  return money(rate).gt(0)
}

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
  { key: 'pay_amount', label: t('payment.orders.payAmount') },
  { key: 'fee_rate', label: t('payment.orders.fee') },
  { key: 'credited_amount', label: t('payment.orders.creditedAmount') },
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
  { value: 'airwallex', label: t('payment.methods.airwallex') },
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
