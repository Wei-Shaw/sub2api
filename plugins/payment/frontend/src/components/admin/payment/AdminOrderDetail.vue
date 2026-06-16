<template>
  <BaseDialog
    :show="show"
    :title="t('payment.admin.orderDetail')"
    width="wide"
    @close="emit('close')"
  >
    <div v-if="order" class="space-y-4">
      <div class="grid grid-cols-2 gap-4">
        <div>
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.orderId') }}</p>
          <p class="font-mono text-sm font-medium text-gray-900 dark:text-white">#{{ order.id }}</p>
        </div>
        <div>
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.status') }}</p>
          <span :class="['badge', statusBadgeClass(order.status)]">
            {{ t('payment.status.' + order.status.toLowerCase(), order.status) }}
          </span>
        </div>
        <div>
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.baseAmount') }}</p>
          <p class="text-sm font-medium text-gray-900 dark:text-white">¥{{ formatMoney(baseAmount) }}</p>
        </div>
        <div v-if="hasFee">
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.fee') }} ({{ order.fee_rate }}%)</p>
          <p class="text-sm font-medium text-gray-900 dark:text-white">¥{{ formatMoney(feeAmount) }}</p>
        </div>
        <div>
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.payAmount') }}</p>
          <p class="text-sm font-medium text-gray-900 dark:text-white">¥{{ formatMoney(order.pay_amount) }}</p>
        </div>
        <div v-if="order.amount !== order.pay_amount">
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.creditedAmount') }}</p>
          <p class="text-sm font-medium text-gray-900 dark:text-white">{{ order.order_type === 'balance' ? '$' : '¥' }}{{ formatMoney(order.amount) }}</p>
        </div>
        <div>
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.paymentMethod') }}</p>
          <p class="text-sm text-gray-700 dark:text-gray-300">
            {{ t('payment.methods.' + order.payment_type, order.payment_type) }}
          </p>
        </div>
        <div>
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.orderType') }}</p>
          <p class="text-sm text-gray-700 dark:text-gray-300">
            {{ t('payment.admin.' + order.order_type + 'Order', order.order_type) }}
          </p>
        </div>
        <div>
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.userId') }}</p>
          <p class="text-sm text-gray-700 dark:text-gray-300">#{{ order.user_id }}</p>
        </div>
        <div>
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.createdAt') }}</p>
          <p class="text-sm text-gray-700 dark:text-gray-300">{{ formatDateTime(order.created_at) }}</p>
        </div>
        <div>
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.expiresAt') }}</p>
          <p class="text-sm text-gray-700 dark:text-gray-300">{{ formatDateTime(order.expires_at) }}</p>
        </div>
        <div v-if="order.paid_at">
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.paidAt') }}</p>
          <p class="text-sm text-gray-700 dark:text-gray-300">{{ formatDateTime(order.paid_at) }}</p>
        </div>
        <div v-if="order.completed_at">
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.completedAt') }}</p>
          <p class="text-sm text-gray-700 dark:text-gray-300">{{ formatDateTime(order.completed_at) }}</p>
        </div>
      </div>

      <div
        v-if="order.refund_amount"
        class="surface-danger rounded-lg p-3"
      >
        <h4 class="text-semantic-danger mb-2 text-sm font-semibold">
          {{ t('payment.admin.refundInfo') }}
        </h4>
        <div class="grid grid-cols-2 gap-2 text-sm">
          <div>
            <span class="text-semantic-danger">{{ t('payment.admin.refundAmount') }}:</span>
            <span class="text-semantic-danger ml-1 font-medium">{{ order.order_type === 'balance' ? '$' : '¥' }}{{ formatMoney(order.refund_amount) }}</span>
          </div>
          <div v-if="order.refund_reason" class="col-span-2">
            <span class="text-semantic-danger">{{ t('payment.admin.refundReason') }}:</span>
            <span class="text-semantic-danger ml-1">{{ order.refund_reason }}</span>
          </div>
        </div>
      </div>

      <div class="flex items-center justify-end gap-2 border-t border-gray-200 pt-4 dark:border-dark-700">
        <button
          v-if="order.status === 'PENDING'"
          @click="emit('cancel', order)"
          class="btn btn-sm btn-soft-warning rounded-md px-3 py-1.5 text-sm"
        >
          {{ t('payment.orders.cancel') }}
        </button>
        <button
          v-if="order.status === 'FAILED'"
          @click="emit('retry', order)"
          class="btn btn-sm btn-secondary"
        >
          {{ t('payment.admin.retry') }}
        </button>
        <button
          v-if="canRefund(order)"
          @click="emit('refund', order)"
          class="btn btn-sm btn-soft-danger rounded-md px-3 py-1.5 text-sm"
        >
          {{ t('payment.admin.refund') }}
        </button>
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { BaseDialog } from '@sub2api/plugin-sdk'
import type { PaymentOrder } from '../../../types/payment'
import { statusBadgeClass, canRefund as canRefundStatus, formatOrderDateTime } from '../../payment/orderUtils'
import { money, formatMoney, Decimal } from '../../../utils/decimal'

const { t } = useI18n()

const props = defineProps<{
  show: boolean
  order: PaymentOrder | null
}>()

const hasFee = computed(() => {
  if (!props.order) return false
  return money(props.order.fee_rate).gt(0)
})

/** 充值金额 (base amount before fee) = pay_amount - fee = pay_amount / (1 + fee_rate/100) */
const baseAmount = computed<Decimal>(() => {
  if (!props.order) return new Decimal(0)
  const pay = money(props.order.pay_amount)
  const fee = money(props.order.fee_rate)
  if (fee.lte(0)) return pay
  // 1 + fee_rate/100; pay / divisor preserves precision
  const divisor = new Decimal(1).plus(fee.div(100))
  return pay.div(divisor)
})

/** 手续费 = pay_amount - baseAmount */
const feeAmount = computed<Decimal>(() => {
  if (!props.order) return new Decimal(0)
  if (money(props.order.fee_rate).lte(0)) return new Decimal(0)
  return money(props.order.pay_amount).minus(baseAmount.value)
})

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'cancel', order: PaymentOrder): void
  (e: 'retry', order: PaymentOrder): void
  (e: 'refund', order: PaymentOrder): void
}>()

function canRefund(order: PaymentOrder): boolean {
  return canRefundStatus(order.status)
}

function formatDateTime(dateStr: string): string {
  return formatOrderDateTime(dateStr)
}
</script>
