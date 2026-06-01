<!-- RechargeTab: top-up form with amount input, method selection, fee summary, and submit button. -->
<template>
  <div class="card p-5">
    <p class="text-xs font-medium text-gray-400 dark:text-gray-500">{{ t('payment.rechargeAccount') }}</p>
    <p class="mt-1 text-base font-semibold text-gray-900 dark:text-white">{{ username }}</p>
    <p class="text-semantic-success mt-0.5 text-sm font-medium">{{ t('payment.currentBalance') }}: {{ formatMoney(balance) }}</p>
  </div>
  <div v-if="enabledMethods.length === 0" class="card py-16 text-center">
    <p class="text-gray-500 dark:text-gray-400">{{ t('payment.notAvailable') }}</p>
  </div>
  <template v-else>
    <div class="card p-6">
      <AmountInput
        :model-value="amount"
        :amounts="[10, 20, 50, 100, 200, 500, 1000, 2000, 5000]"
        :min="minAmount"
        :max="maxAmount"
        @update:model-value="$emit('update:amount', $event)"
      />
      <p v-if="amountError" class="text-semantic-warning mt-2 text-xs">{{ amountError }}</p>
    </div>
    <div v-if="enabledMethods.length >= 1" class="card p-6">
      <PaymentMethodSelector
        :methods="methodOptions"
        :selected="selectedMethod"
        @select="$emit('update:selectedMethod', $event)"
      />
    </div>
    <div v-if="showSummary" class="card p-6">
      <div class="space-y-2 text-sm">
        <div class="flex justify-between">
          <span class="text-gray-500 dark:text-gray-400">{{ t('payment.paymentAmount') }}</span>
          <span class="text-gray-900 dark:text-white">{{ formatAmount(validAmount) }}</span>
        </div>
        <div v-if="hasFeeRate" class="flex justify-between">
          <span class="text-gray-500 dark:text-gray-400">{{ t('payment.fee') }} ({{ feeRate }}%)</span>
          <span class="text-gray-900 dark:text-white">{{ formatAmount(feeAmount) }}</span>
        </div>
        <div v-if="hasFeeRate" class="flex justify-between border-t border-gray-200 pt-2 dark:border-dark-600">
          <span class="font-medium text-gray-700 dark:text-gray-300">{{ t('payment.actualPay') }}</span>
          <span class="text-lg font-bold text-primary-600 dark:text-primary-400">{{ formatAmount(totalAmount) }}</span>
        </div>
        <div v-if="hasMultiplier" class="flex justify-between" :class="{ 'border-t border-gray-200 pt-2 dark:border-dark-600': !hasFeeRate }">
          <span class="text-gray-500 dark:text-gray-400">{{ t('payment.creditedBalance') }}</span>
          <span class="text-gray-900 dark:text-white">${{ formatMoney(creditedAmount) }}</span>
        </div>
        <p v-if="hasMultiplier" class="border-t border-gray-200 pt-2 text-xs text-gray-500 dark:border-dark-600 dark:text-gray-400">
          {{ t('payment.rechargeRatePreview', { usd: formatMoney(balanceRechargeMultiplier) }) }}
        </p>
      </div>
    </div>
    <button :class="['btn w-full py-3 text-base font-medium', buttonClass]" :disabled="!canSubmit || submitting" @click="$emit('submit')">
      <span v-if="submitting" class="flex items-center justify-center gap-2">
        <span class="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
        {{ t('common.processing') }}
      </span>
      <span v-else>{{ t('payment.createOrder') }} {{ formatAmount(totalAmount) }}</span>
    </button>
  </template>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatMoney, type Decimal } from '../../utils/decimal'
import AmountInput from './AmountInput.vue'
import PaymentMethodSelector from './PaymentMethodSelector.vue'
import type { PaymentMethodOption } from './PaymentMethodSelector.vue'

const { t } = useI18n()

const props = defineProps<{
  username: string
  balance: string | number | null | undefined
  amount: number | null
  enabledMethods: string[]
  methodOptions: PaymentMethodOption[]
  selectedMethod: string
  minAmount: number
  maxAmount: number
  amountError: string
  validAmount: Decimal
  hasFeeRate: boolean
  feeRate: Decimal
  feeAmount: Decimal
  totalAmount: Decimal
  hasMultiplier: boolean
  creditedAmount: Decimal
  balanceRechargeMultiplier: Decimal
  canSubmit: boolean
  submitting: boolean
  buttonClass: string
  formatAmount: (v: number | Decimal | string) => string
}>()

defineEmits<{
  'update:amount': [value: number | null]
  'update:selectedMethod': [value: string]
  submit: []
}>()

const showSummary = computed(() => props.validAmount.gt(0))
</script>