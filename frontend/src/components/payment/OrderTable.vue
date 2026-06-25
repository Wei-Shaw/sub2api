<template>
  <DataTable :columns="columns" :data="orders" :loading="loading">
    <template #cell-id="{ value REDACTED">
      <span class="font-mono text-sm">#{{ value REDACTEDREDACTED</span>
    </template>
    <template #cell-out_trade_no="{ value REDACTED">
      <span class="text-sm text-gray-900 dark:text-white">{{ value REDACTEDREDACTED</span>
    </template>
    <template v-if="showUser" #cell-user_email="{ value, row REDACTED">
      <div class="text-sm">
        <span class="text-gray-900 dark:text-white">{{ value || row.user_name || '#' + row.user_id REDACTEDREDACTED</span>
        <span v-if="row.user_notes" class="ml-1 text-xs text-gray-400">({{ row.user_notes REDACTEDREDACTED)</span>
      </div>
    </template>
    <template #cell-pay_amount="{ value, row REDACTED">
      <div class="text-sm">
        <span class="font-medium text-gray-900 dark:text-white">{{ paymentAmountSymbol(row) REDACTEDREDACTED{{ value.toFixed(2) REDACTEDREDACTED</span>
        <span v-if="row.fee_rate > 0" class="ml-1 text-xs text-gray-400" :title="t('payment.orders.fee') + ': ' + row.fee_rate + '%'">
          ({{ t('payment.orders.fee') REDACTEDREDACTED {{ row.fee_rate REDACTEDREDACTED%)
        </span>
        <div v-if="row.amount !== row.pay_amount" class="text-xs text-gray-500">
          {{ t('payment.orders.creditedAmount') REDACTEDREDACTED: {{ creditedAmountSymbol REDACTEDREDACTED{{ row.amount.toFixed(2) REDACTEDREDACTED
        </div>
      </div>
    </template>
    <template #cell-payment_type="{ value REDACTED">
      <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('payment.methods.' + value, value) REDACTEDREDACTED</span>
    </template>
    <template #cell-status="{ value REDACTED">
      <OrderStatusBadge :status="value" />
    </template>
    <template #cell-created_at="{ value REDACTED">
      <span class="text-xs text-gray-500 dark:text-gray-400">{{ formatDate(value) REDACTEDREDACTED</span>
    </template>
    <template #cell-actions="{ row REDACTED">
      <slot name="actions" :row="row" />
    </template>
  </DataTable>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import type { PaymentOrder REDACTED from '@/types/payment'
import type { Column REDACTED from '@/components/common/types'
import DataTable from '@/components/common/DataTable.vue'
import OrderStatusBadge from '@/components/payment/OrderStatusBadge.vue'
import { currencySymbol REDACTED from '@/components/payment/currency'

const { t REDACTED = useI18n()

const props = defineProps<{
  orders: PaymentOrder[]
  loading: boolean
  showUser?: boolean
REDACTED>()

function formatDate(dateStr: string) { return new Date(dateStr).toLocaleString() REDACTED

const creditedAmountSymbol = currencySymbol('USD')

function paymentAmountSymbol(order: PaymentOrder): string {
  return currencySymbol(order.currency)
REDACTED

const columns = computed((): Column[] => {
  const cols: Column[] = [
    { key: 'id', label: t('payment.orders.orderId') REDACTED,
    { key: 'out_trade_no', label: t('payment.orders.orderNo') REDACTED,
  ]
  if (props.showUser) {
    cols.push({ key: 'user_email', label: t('payment.admin.colUser') REDACTED)
  REDACTED
  cols.push(
    { key: 'pay_amount', label: t('payment.orders.payAmount') REDACTED,
    { key: 'payment_type', label: t('payment.orders.paymentMethod') REDACTED,
    { key: 'status', label: t('payment.orders.status') REDACTED,
    { key: 'created_at', label: t('payment.orders.createdAt') REDACTED,
    { key: 'actions', label: t('common.actions') REDACTED,
  )
  return cols
REDACTED)
</script>
