<template>
  <BaseDialog
    :show="show"
    :title="t('payment.admin.orderDetail')"
    width="wide"
    @close="emit('close')"
  >
    <div v-if="order" class="space-y-4">
      <dl class="grid grid-cols-1 border-t border-border sm:grid-cols-2 sm:gap-x-6">
        <div class="flex items-baseline justify-between gap-3 border-b border-border py-2">
          <dt class="text-meta font-medium text-fg-muted">{{ t('payment.orders.orderId') }}</dt>
          <dd class="font-mono text-body font-semibold text-fg">#{{ order.id }}</dd>
        </div>
        <div class="flex items-baseline justify-between gap-3 border-b border-border py-2">
          <dt class="text-meta font-medium text-fg-muted">{{ t('payment.orders.status') }}</dt>
          <dd class="text-body">
            <span :class="['badge', statusBadgeClass(order.status)]">
              {{ t('payment.status.' + order.status.toLowerCase(), order.status) }}
            </span>
          </dd>
        </div>
        <div class="flex items-baseline justify-between gap-3 border-b border-border py-2">
          <dt class="text-meta font-medium text-fg-muted">{{ t('payment.orders.baseAmount') }}</dt>
          <dd class="text-body font-semibold tabular-nums text-fg">{{ paymentAmountSymbol }}{{ baseAmount.toFixed(2) }}</dd>
        </div>
        <div v-if="order.fee_rate > 0" class="flex items-baseline justify-between gap-3 border-b border-border py-2">
          <dt class="text-meta font-medium text-fg-muted">{{ t('payment.orders.fee') }} ({{ order.fee_rate }}%)</dt>
          <dd class="text-body font-semibold tabular-nums text-fg">{{ paymentAmountSymbol }}{{ feeAmount.toFixed(2) }}</dd>
        </div>
        <div class="flex items-baseline justify-between gap-3 border-b border-border py-2">
          <dt class="text-meta font-medium text-fg-muted">{{ t('payment.orders.payAmount') }}</dt>
          <dd class="text-body font-bold tabular-nums text-fg">{{ paymentAmountSymbol }}{{ order.pay_amount.toFixed(2) }}</dd>
        </div>
        <div v-if="(order.bonus_amount ?? 0) > 0" class="flex items-baseline justify-between gap-3 border-b border-border py-2">
          <dt class="text-meta font-medium text-fg-muted">{{ t('payment.orders.bonusAmount') }}</dt>
          <dd class="text-body font-semibold tabular-nums text-accent-strong">+{{ creditedAmountSymbol }}{{ (order.bonus_amount ?? 0).toFixed(2) }}</dd>
        </div>
        <div v-if="order.amount !== order.pay_amount || (order.bonus_amount ?? 0) > 0" class="flex items-baseline justify-between gap-3 border-b border-border py-2">
          <dt class="text-meta font-medium text-fg-muted">{{ t('payment.orders.creditedAmount') }}</dt>
          <dd class="text-body font-semibold tabular-nums text-fg">{{ creditedAmountSymbol }}{{ order.amount.toFixed(2) }}</dd>
        </div>
        <div class="flex items-baseline justify-between gap-3 border-b border-border py-2">
          <dt class="text-meta font-medium text-fg-muted">{{ t('payment.orders.paymentMethod') }}</dt>
          <dd class="text-body text-fg">{{ t('payment.methods.' + order.payment_type, order.payment_type) }}</dd>
        </div>
        <div class="flex items-baseline justify-between gap-3 border-b border-border py-2">
          <dt class="text-meta font-medium text-fg-muted">{{ t('payment.admin.orderType') }}</dt>
          <dd class="text-body text-fg">{{ t('payment.admin.' + order.order_type + 'Order', order.order_type) }}</dd>
        </div>
        <div class="flex items-baseline justify-between gap-3 border-b border-border py-2">
          <dt class="text-meta font-medium text-fg-muted">{{ t('payment.orders.userId') }}</dt>
          <dd class="font-mono text-body text-fg">#{{ order.user_id }}</dd>
        </div>
        <div class="flex items-baseline justify-between gap-3 border-b border-border py-2">
          <dt class="text-meta font-medium text-fg-muted">{{ t('payment.orders.createdAt') }}</dt>
          <dd class="text-body tabular-nums text-fg">{{ formatDateTime(order.created_at) }}</dd>
        </div>
        <div class="flex items-baseline justify-between gap-3 border-b border-border py-2">
          <dt class="text-meta font-medium text-fg-muted">{{ t('payment.admin.expiresAt') }}</dt>
          <dd class="text-body tabular-nums text-fg">{{ formatDateTime(order.expires_at) }}</dd>
        </div>
        <div v-if="order.paid_at" class="flex items-baseline justify-between gap-3 border-b border-border py-2">
          <dt class="text-meta font-medium text-fg-muted">{{ t('payment.admin.paidAt') }}</dt>
          <dd class="text-body tabular-nums text-fg">{{ formatDateTime(order.paid_at) }}</dd>
        </div>
        <div v-if="order.completed_at" class="flex items-baseline justify-between gap-3 border-b border-border py-2">
          <dt class="text-meta font-medium text-fg-muted">{{ t('payment.admin.completedAt') }}</dt>
          <dd class="text-body tabular-nums text-fg">{{ formatDateTime(order.completed_at) }}</dd>
        </div>
      </dl>

      <div v-if="order.refund_amount" class="border border-danger/40 pl-3">
        <h4 class="mb-1 text-label font-bold text-danger">
          {{ t('payment.admin.refundInfo') }}
        </h4>
        <div class="space-y-1 text-body">
          <div>
            <span class="text-fg-muted">{{ t('payment.admin.refundAmount') }}:</span>
            <span class="ml-1 font-semibold tabular-nums text-danger">{{ creditedAmountSymbol }}{{ order.refund_amount.toFixed(2) }}</span>
          </div>
          <div v-if="order.refund_reason">
            <span class="text-fg-muted">{{ t('payment.admin.refundReason') }}:</span>
            <span class="ml-1 text-fg">{{ order.refund_reason }}</span>
          </div>
        </div>
      </div>

      <div class="flex items-center justify-end gap-2 border-t border-border pt-4">
        <button
          v-if="order.status === 'PENDING'"
          @click="emit('cancel', order)"
          class="btn btn-sm btn-secondary text-warning hover:border-warning hover:text-warning-strong"
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
