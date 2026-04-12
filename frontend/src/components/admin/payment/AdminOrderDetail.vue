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
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.amount') }}</p>
          <p class="text-sm font-medium text-gray-900 dark:text-white">${{ order.amount.toFixed(2) }}</p>
        </div>
        <div>
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.payAmount') }}</p>
          <p class="text-sm font-medium text-gray-900 dark:text-white">${{ order.pay_amount.toFixed(2) }}</p>
        </div>
        <div>
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.paymentMethod') }}</p>
          <p class="text-sm text-gray-700 dark:text-gray-300">
            {{ t('payment.methods.' + order.payment_type, order.payment_type) }}
          </p>
        </div>
        <div>
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.feeRate') }}</p>
          <p class="text-sm text-gray-700 dark:text-gray-300">{{ (order.fee_rate * 100).toFixed(1) }}%</p>
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
        class="rounded-lg border border-red-200 bg-red-50 p-3 dark:border-red-800 dark:bg-red-900/20"
      >
        <h4 class="mb-2 text-sm font-semibold text-red-700 dark:text-red-400">
          {{ t('payment.admin.refundInfo') }}
        </h4>
        <div class="grid grid-cols-2 gap-2 text-sm">
          <div>
            <span class="text-red-600 dark:text-red-400">{{ t('payment.admin.refundAmount') }}:</span>
            <span class="ml-1 font-medium text-red-700 dark:text-red-300">${{ order.refund_amount.toFixed(2) }}</span>
          </div>
          <div v-if="order.refund_reason" class="col-span-2">
            <span class="text-red-600 dark:text-red-400">{{ t('payment.admin.refundReason') }}:</span>
            <span class="ml-1 text-red-700 dark:text-red-300">{{ order.refund_reason }}</span>
          </div>
        </div>
      </div>

      <div class="flex items-center justify-end gap-2 border-t border-gray-200 pt-4 dark:border-dark-700">
        <button
          v-if="order.status === 'PENDING'"
          @click="emit('cancel', order)"
          class="btn btn-sm rounded-md bg-yellow-50 px-3 py-1.5 text-sm text-yellow-600 hover:bg-yellow-100 dark:bg-yellow-900/20 dark:text-yellow-400 dark:hover:bg-yellow-900/30"
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
          class="btn btn-sm rounded-md bg-red-50 px-3 py-1.5 text-sm text-red-600 hover:bg-red-100 dark:bg-red-900/20 dark:text-red-400 dark:hover:bg-red-900/30"
        >
          {{ t('payment.admin.refund') }}
        </button>
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import type { PaymentOrder } from '@/types/payment'
import { statusBadgeClass, canRefund as canRefundStatus, formatOrderDateTime } from '@/components/payment/orderUtils'

const { t } = useI18n()

defineProps<{
  show: boolean
  order: PaymentOrder | null
}>()

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
