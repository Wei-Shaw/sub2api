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
          <p class="text-xs text-ink-secondary">{{ t('payment.orders.orderId') }}</p>
          <p class="font-mono text-sm font-medium text-ink">#{{ order.id }}</p>
        </div>
        <div>
          <p class="text-xs text-ink-secondary">{{ t('payment.orders.status') }}</p>
          <span :class="['badge', statusBadgeClass(order.status)]">
            {{ t('payment.status.' + order.status.toLowerCase(), order.status) }}
          </span>
        </div>
        <div>
          <p class="text-xs text-ink-secondary">{{ t('payment.orders.baseAmount') }}</p>
          <p class="text-sm font-medium text-ink">{{ paymentAmountSymbol }}{{ baseAmount.toFixed(2) }}</p>
        </div>
        <div v-if="order.fee_rate > 0">
          <p class="text-xs text-ink-secondary">{{ t('payment.orders.fee') }} ({{ order.fee_rate }}%)</p>
          <p class="text-sm font-medium text-ink">{{ paymentAmountSymbol }}{{ feeAmount.toFixed(2) }}</p>
        </div>
        <div>
          <p class="text-xs text-ink-secondary">{{ t('payment.orders.payAmount') }}</p>
          <p class="text-sm font-medium text-ink">{{ paymentAmountSymbol }}{{ order.pay_amount.toFixed(2) }}</p>
        </div>
        <div v-if="order.amount !== order.pay_amount">
          <p class="text-xs text-ink-secondary">{{ t('payment.orders.creditedAmount') }}</p>
          <p class="text-sm font-medium text-ink">{{ creditedAmountSymbol }}{{ order.amount.toFixed(2) }}</p>
        </div>
        <div>
          <p class="text-xs text-ink-secondary">{{ t('payment.orders.paymentMethod') }}</p>
          <p class="text-sm text-ink-secondary">
            {{ t('payment.methods.' + order.payment_type, order.payment_type) }}
          </p>
        </div>
        <div>
          <p class="text-xs text-ink-secondary">{{ t('payment.admin.orderType') }}</p>
          <p class="text-sm text-ink-secondary">
            {{ t('payment.admin.' + order.order_type + 'Order', order.order_type) }}
          </p>
        </div>
        <div>
          <p class="text-xs text-ink-secondary">{{ t('payment.orders.userId') }}</p>
          <p class="text-sm text-ink-secondary">#{{ order.user_id }}</p>
        </div>
        <div>
          <p class="text-xs text-ink-secondary">{{ t('payment.orders.createdAt') }}</p>
          <p class="text-sm text-ink-secondary">{{ formatDateTime(order.created_at) }}</p>
        </div>
        <div>
          <p class="text-xs text-ink-secondary">{{ t('payment.admin.expiresAt') }}</p>
          <p class="text-sm text-ink-secondary">{{ formatDateTime(order.expires_at) }}</p>
        </div>
        <div v-if="order.paid_at">
          <p class="text-xs text-ink-secondary">{{ t('payment.admin.paidAt') }}</p>
          <p class="text-sm text-ink-secondary">{{ formatDateTime(order.paid_at) }}</p>
        </div>
        <div v-if="order.completed_at">
          <p class="text-xs text-ink-secondary">{{ t('payment.admin.completedAt') }}</p>
          <p class="text-sm text-ink-secondary">{{ formatDateTime(order.completed_at) }}</p>
        </div>
      </div>


      <div class="flex items-center justify-end gap-2 border-t border-line pt-4">
        <button
          v-if="order.status === 'PENDING'"
          @click="emit('cancel', order)"
          class="btn btn-sm rounded-md border border-warn/40 bg-warn-tint px-3 py-1.5 text-sm text-yellow-600 hover:bg-yellow-100 dark:text-yellow-400 dark:hover:bg-yellow-900/30"
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
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import type { PaymentOrder } from '@/types/payment'
import { statusBadgeClass, formatOrderDateTime } from '@/components/payment/orderUtils'
import { currencySymbol } from '@/components/payment/currency'

const { t } = useI18n()

const props = defineProps<{
  show: boolean
  order: PaymentOrder | null
}>()

const creditedAmountSymbol = currencySymbol('USD')

const paymentAmountSymbol = computed(() => currencySymbol(props.order?.currency))

/** 充值金额 (base amount before fee) = pay_amount - fee = pay_amount / (1 + fee_rate/100) */
const baseAmount = computed(() => {
  if (!props.order) return 0
  const feeRate = Number(props.order.fee_rate) || 0
  if (feeRate <= 0) return props.order.pay_amount
  return props.order.pay_amount / (1 + feeRate / 100)
})

/** 手续费 = pay_amount - baseAmount */
const feeAmount = computed(() => {
  if (!props.order) return 0
  const feeRate = Number(props.order.fee_rate) || 0
  if (feeRate <= 0) return 0
  return props.order.pay_amount - baseAmount.value
})

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'cancel', order: PaymentOrder): void
  (e: 'retry', order: PaymentOrder): void
}>()

function formatDateTime(dateStr: string): string {
  return formatOrderDateTime(dateStr)
}
</script>
