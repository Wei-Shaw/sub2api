<!-- SubscriptionConfirm: plan detail card, method selection, fee summary, confirm/cancel. -->
<template>
  <div class="card p-5">
    <!-- Header: platform badge + plan name -->
    <div class="mb-3 flex flex-wrap items-center gap-2">
      <span :class="['rounded-md border px-2 py-0.5 text-xs font-medium', badgeClass]">
        {{ platformLabel(plan.group_platform || '') }}
      </span>
      <h3 class="text-lg font-bold text-gray-900 dark:text-white">{{ plan.name }}</h3>
    </div>
    <!-- Price -->
    <div class="flex items-baseline gap-2">
      <span v-if="plan.original_price" class="text-sm text-gray-400 line-through dark:text-gray-500">
        {{ formatAmount(plan.original_price) }}
      </span>
      <span :class="['text-3xl font-bold', textClass]">{{ formatAmount(plan.price) }}</span>
      <span class="text-sm text-gray-500 dark:text-gray-400">/ {{ validitySuffix }}</span>
    </div>
    <!-- Description -->
    <p v-if="plan.description" class="mt-2 text-sm leading-relaxed text-gray-500 dark:text-gray-400">
      {{ plan.description }}
    </p>
    <!-- Rate + Limits grid -->
    <div class="mt-3 grid grid-cols-2 gap-3">
      <div>
        <span class="text-xs text-gray-400 dark:text-gray-500">{{ t('payment.planCard.rate') }}</span>
        <div class="flex items-baseline">
          <span :class="['text-lg font-bold', textClass]">&times;{{ plan.rate_multiplier ?? 1 }}</span>
        </div>
      </div>
      <div v-if="plan.daily_limit_usd != null">
        <span class="text-xs text-gray-400 dark:text-gray-500">{{ t('payment.planCard.dailyLimit') }}</span>
        <div class="text-lg font-semibold text-gray-800 dark:text-gray-200">${{ plan.daily_limit_usd }}</div>
      </div>
      <div v-if="plan.weekly_limit_usd != null">
        <span class="text-xs text-gray-400 dark:text-gray-500">{{ t('payment.planCard.weeklyLimit') }}</span>
        <div class="text-lg font-semibold text-gray-800 dark:text-gray-200">${{ plan.weekly_limit_usd }}</div>
      </div>
      <div v-if="plan.monthly_limit_usd != null">
        <span class="text-xs text-gray-400 dark:text-gray-500">{{ t('payment.planCard.monthlyLimit') }}</span>
        <div class="text-lg font-semibold text-gray-800 dark:text-gray-200">${{ plan.monthly_limit_usd }}</div>
      </div>
      <div v-if="plan.daily_limit_usd == null && plan.weekly_limit_usd == null && plan.monthly_limit_usd == null">
        <span class="text-xs text-gray-400 dark:text-gray-500">{{ t('payment.planCard.quota') }}</span>
        <div class="text-lg font-semibold text-gray-800 dark:text-gray-200">{{ t('payment.planCard.unlimited') }}</div>
      </div>
    </div>
  </div>
  <div v-if="enabledMethods.length >= 1" class="card p-6">
    <PaymentMethodSelector
      :methods="subMethodOptions"
      :selected="selectedMethod"
      @select="$emit('update:selectedMethod', $event)"
    />
  </div>
  <div v-if="hasFeeRate && hasPositivePlanPrice" class="card p-6">
    <div class="space-y-2 text-sm">
      <div class="flex justify-between">
        <span class="text-gray-500 dark:text-gray-400">{{ t('payment.amountLabel') }}</span>
        <span class="text-gray-900 dark:text-white">{{ formatAmount(plan.price) }}</span>
      </div>
      <div class="flex justify-between">
        <span class="text-gray-500 dark:text-gray-400">{{ t('payment.fee') }} ({{ feeRate }}%)</span>
        <span class="text-gray-900 dark:text-white">{{ formatAmount(subFeeAmount) }}</span>
      </div>
      <div class="flex justify-between border-t border-gray-200 pt-2 dark:border-dark-600">
        <span class="font-medium text-gray-700 dark:text-gray-300">{{ t('payment.actualPay') }}</span>
        <span class="text-lg font-bold text-primary-600 dark:text-primary-400">{{ formatAmount(subTotalAmount) }}</span>
      </div>
    </div>
  </div>
  <button :class="['btn w-full py-3 text-base font-medium', buttonClass]" :disabled="!canSubmit || submitting" @click="$emit('confirm')">
    <span v-if="submitting" class="flex items-center justify-center gap-2">
      <span class="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
      {{ t('common.processing') }}
    </span>
    <span v-else>{{ t('payment.createOrder') }} {{ formatAmount(hasFeeRate ? subTotalAmount : plan.price) }}</span>
  </button>
  <button class="btn btn-secondary w-full" @click="$emit('cancel')">{{ t('common.cancel') }}</button>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { platformBadgeClass, platformTextClass, platformLabel } from '@sub2api/plugin-sdk'
import PaymentMethodSelector from './PaymentMethodSelector.vue'
import type { PaymentMethodOption } from './PaymentMethodSelector.vue'
import type { SubscriptionPlan } from '../../types/payment'
import type { Decimal } from '../../utils/decimal'

const { t } = useI18n()

const props = defineProps<{
  plan: SubscriptionPlan
  enabledMethods: string[]
  subMethodOptions: PaymentMethodOption[]
  selectedMethod: string
  hasFeeRate: boolean
  feeRate: Decimal
  hasPositivePlanPrice: boolean
  subFeeAmount: Decimal
  subTotalAmount: Decimal
  canSubmit: boolean
  submitting: boolean
  buttonClass: string
  formatAmount: (v: number | Decimal | string) => string
}>()

defineEmits<{
  'update:selectedMethod': [value: string]
  confirm: []
  cancel: []
}>()

const badgeClass = computed(() => platformBadgeClass(props.plan.group_platform || ''))
const textClass = computed(() => platformTextClass(props.plan.group_platform || ''))

const validitySuffix = computed(() => {
  const u = props.plan.validity_unit || 'day'
  if (u === 'month') return t('payment.perMonth')
  if (u === 'year') return t('payment.perYear')
  return props.plan.validity_days + t('payment.days')
})
</script>