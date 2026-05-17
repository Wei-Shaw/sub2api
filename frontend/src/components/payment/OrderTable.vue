<template>
  <DataTable :columns="columns" :data="orders" :loading="loading">
    <template #cell-id="{ value }">
      <span class="font-mono text-sm">#{{ value }}</span>
    </template>
    <template #cell-out_trade_no="{ value }">
      <span class="text-sm text-gray-900 dark:text-white">{{ value }}</span>
    </template>
    <template v-if="showUser" #cell-user_email="{ value, row }">
      <div class="flex min-w-0 items-center gap-2">
        <div class="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/30">
          <span class="text-sm font-medium text-primary-700 dark:text-primary-300">
            {{ (value || row.user_name || '#').charAt(0).toUpperCase() }}
          </span>
        </div>
        <div class="flex min-w-0 flex-col gap-1">
          <span
            class="block max-w-[14rem] truncate font-medium text-gray-900 dark:text-white sm:max-w-[18rem]"
            :title="value || row.user_name || '#' + row.user_id"
          >
            {{ value || row.user_name || '#' + row.user_id }}
          </span>
          <span v-if="row.user_notes" class="block max-w-[14rem] truncate text-xs text-gray-400 sm:max-w-[18rem]">{{ row.user_notes }}</span>
          <div v-if="row.device_code" class="flex min-w-0 items-center gap-1.5">
            <span
              class="inline-flex min-w-0 max-w-[12rem] items-center rounded-md bg-primary-50 px-1.5 py-0.5 text-xs font-medium text-primary-700 ring-1 ring-inset ring-primary-200 dark:bg-primary-900/20 dark:text-primary-300 dark:ring-primary-800"
              :title="row.device_code"
            >
              <span class="truncate font-mono">{{ row.device_code }}</span>
            </span>
          </div>
        </div>
      </div>
    </template>
    <template #cell-pay_amount="{ value, row }">
      <div class="text-sm">
        <span class="font-medium text-gray-900 dark:text-white">{{ formatCurrencyAmount(value, orderPaymentCurrency(row)) }}</span>
        <span v-if="row.fee_rate > 0" class="ml-1 text-xs text-gray-400" :title="t('payment.orders.fee') + ': ' + row.fee_rate + '%'">
          ({{ t('payment.orders.fee') }} {{ row.fee_rate }}%)
        </span>
        <div v-if="row.ledger_amount && row.ledger_amount !== row.pay_amount" class="text-xs text-gray-500">
          {{ t('payment.orders.creditedAmount') }}: {{ formatCurrencyAmount(row.ledger_amount || row.amount, orderLedgerCurrency(row)) }}        </div>
      </div>
    </template>
    <template #cell-payment_type="{ value }">
      <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('payment.methods.' + value, value) }}</span>
    </template>
    <template #cell-status="{ value }">
      <OrderStatusBadge :status="value" />
    </template>
    <template #cell-created_at="{ value }">
      <span class="text-xs text-gray-500 dark:text-gray-400">{{ formatDate(value) }}</span>
    </template>
    <template #cell-actions="{ row }">
      <slot name="actions" :row="row" />
    </template>
  </DataTable>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PaymentOrder } from '@/types/payment'
import type { Column } from '@/components/common/types'
import DataTable from '@/components/common/DataTable.vue'
import OrderStatusBadge from '@/components/payment/OrderStatusBadge.vue'
import { formatCurrencyAmount, orderPaymentCurrency, orderLedgerCurrency } from '@/components/payment/orderUtils'
const { t } = useI18n()

const props = defineProps<{
  orders: PaymentOrder[]
  loading: boolean
  showUser?: boolean
}>()

function formatDate(dateStr: string) { return new Date(dateStr).toLocaleString() }

const columns = computed((): Column[] => {
  const cols: Column[] = [
    { key: 'id', label: t('payment.orders.orderId') },
    { key: 'out_trade_no', label: t('payment.orders.orderNo') },
  ]
  if (props.showUser) {
    cols.push({ key: 'user_email', label: t('payment.admin.colUser') })
  }
  cols.push(
    { key: 'pay_amount', label: t('payment.orders.payAmount') },
    { key: 'payment_type', label: t('payment.orders.paymentMethod') },
    { key: 'status', label: t('payment.orders.status') },
    { key: 'created_at', label: t('payment.orders.createdAt') },
    { key: 'actions', label: t('common.actions') },
  )
  return cols
})
</script>
