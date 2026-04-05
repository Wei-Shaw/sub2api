<template>
  <BaseDialog
    :show="show"
    :title="t('payment.admin.refundOrder')"
    width="normal"
    @close="emit('cancel')"
  >
    <form id="refund-form" @submit.prevent="handleSubmit" class="space-y-4">
      <!-- Order Info -->
      <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-700">
        <div class="flex justify-between text-sm">
          <span class="text-gray-500 dark:text-gray-400">{{ t('payment.orders.orderId') }}</span>
          <span class="font-mono text-gray-900 dark:text-white">#{{ order?.id }}</span>
        </div>
        <div class="mt-1 flex justify-between text-sm">
          <span class="text-gray-500 dark:text-gray-400">{{ t('payment.orders.amount') }}</span>
          <span class="font-medium text-gray-900 dark:text-white">¥{{ order?.pay_amount?.toFixed(2) }}</span>
        </div>
        <div v-if="order?.refund_amount" class="mt-1 flex justify-between text-sm">
          <span class="text-gray-500 dark:text-gray-400">{{ t('payment.admin.alreadyRefunded') }}</span>
          <span class="font-medium text-red-600 dark:text-red-400">¥{{ order.refund_amount.toFixed(2) }}</span>
        </div>
      </div>

      <!-- Refund Amount -->
      <div>
        <label class="input-label">{{ t('payment.admin.refundAmount') }}</label>
        <div class="relative">
          <span class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500">¥</span>
          <input
            v-model.number="form.amount"
            type="number"
            step="0.01"
            min="0.01"
            :max="maxRefundable"
            class="input pl-7"
            required
          />
        </div>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('payment.admin.maxRefundable') }}: ¥{{ maxRefundable.toFixed(2) }}
        </p>
      </div>

      <!-- Reason -->
      <div>
        <label class="input-label">{{ t('payment.admin.refundReason') }}</label>
        <textarea
          v-model="form.reason"
          rows="3"
          class="input"
          :placeholder="t('payment.admin.refundReasonPlaceholder')"
          required
        ></textarea>
      </div>

      <!-- Deduct Balance -->
      <div class="flex items-center gap-2">
        <input
          id="deduct-balance"
          v-model="form.deduct_balance"
          type="checkbox"
          class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
        />
        <label for="deduct-balance" class="text-sm text-gray-700 dark:text-gray-300">
          {{ t('payment.admin.deductBalance') }}
        </label>
      </div>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" @click="emit('cancel')" class="btn btn-secondary">
          {{ t('common.cancel') }}
        </button>
        <button
          type="submit"
          form="refund-form"
          :disabled="submitting || form.amount <= 0"
          class="rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-red-500 focus:ring-offset-2 disabled:opacity-50 dark:focus:ring-offset-dark-800"
        >
          {{ submitting ? t('common.processing') : t('payment.admin.confirmRefund') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { reactive, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import type { PaymentOrder } from '@/types/payment'

const { t } = useI18n()

const props = defineProps<{
  show: boolean
  order: PaymentOrder | null
  submitting?: boolean
}>()

const emit = defineEmits<{
  (e: 'confirm', data: { amount: number; reason: string; deduct_balance: boolean }): void
  (e: 'cancel'): void
}>()

const form = reactive({
  amount: 0,
  reason: '',
  deduct_balance: false,
})

const maxRefundable = computed(() => {
  if (!props.order) return 0
  return props.order.pay_amount - (props.order.refund_amount || 0)
})

watch(() => props.show, (val) => {
  if (val && props.order) {
    form.amount = maxRefundable.value
    form.reason = ''
    form.deduct_balance = false
  }
})

function handleSubmit() {
  if (form.amount <= 0 || form.amount > maxRefundable.value) return
  emit('confirm', { ...form })
}
</script>
