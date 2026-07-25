<template>
  <div class="card p-4">
    <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">
      {{ t('payment.admin.paymentDistribution') REDACTEDREDACTED
    </h3>
    <div
      v-if="!methods?.length"
      class="flex h-32 items-center justify-center text-sm text-gray-500 dark:text-gray-400"
    >
      {{ t('payment.admin.noData') REDACTEDREDACTED
    </div>
    <div v-else class="space-y-3">
      <div v-for="method in methods" :key="method.type" class="space-y-1">
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <span :class="['inline-block h-3 w-3 rounded-full', colorMap[method.type] || 'bg-gray-400']"></span>
            <span class="text-sm text-gray-700 dark:text-gray-300">
              {{ t('payment.methods.' + method.type, method.type) REDACTEDREDACTED
            </span>
          </div>
          <div class="space-y-1 text-right">
            <span v-for="[currency, amount] in sortedAmounts(method.amount)" :key="currency" class="block text-sm font-medium text-gray-900 dark:text-white">
              {{ formatMoney(currency, amount) REDACTEDREDACTED
            </span>
            <span class="ml-2 text-xs text-gray-500 dark:text-gray-400">
              ({{ method.count REDACTEDREDACTED)
            </span>
          </div>
        </div>
        <div v-for="[currency, amount] in sortedAmounts(method.amount)" :key="currency" class="flex items-center gap-2">
          <span class="w-10 text-xs text-gray-500 dark:text-gray-400">{{ currency REDACTEDREDACTED</span>
          <div class="h-2 flex-1 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
            <div
              :class="['h-full rounded-full transition-all', barColorMap[method.type] || 'bg-gray-400']"
              :style="{ width: barWidth(currency, amount) + '%' REDACTED"
            ></div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import type { CurrencyAmounts, PaymentMethodStats REDACTED from '@/types/payment'

const { t REDACTED = useI18n()

const props = defineProps<{
  methods: PaymentMethodStats[]
REDACTED>()

const colorMap: Record<string, string> = {
  alipay: 'bg-blue-500',
  wxpay: 'bg-green-500',
  alipay_direct: 'bg-blue-400',
  wxpay_direct: 'bg-green-400',
  stripe: 'bg-purple-500',
REDACTED

const barColorMap: Record<string, string> = {
  alipay: 'bg-blue-500',
  wxpay: 'bg-green-500',
  alipay_direct: 'bg-blue-400',
  wxpay_direct: 'bg-green-400',
  stripe: 'bg-purple-500',
REDACTED

const maxAmounts = computed<CurrencyAmounts>(() => {
  return props.methods.reduce<CurrencyAmounts>((maximums, method) => {
    for (const [currency, amount] of Object.entries(method.amount)) {
      maximums[currency] = Math.max(maximums[currency] || 0, amount)
    REDACTED
    return maximums
  REDACTED, {REDACTED)
REDACTED)

function sortedAmounts(amounts: CurrencyAmounts): [string, number][] {
  return Object.entries(amounts).sort(([left], [right]) => left.localeCompare(right))
REDACTED

function barWidth(currency: string, amount: number): number {
  return Math.min((amount / (maxAmounts.value[currency] || 1)) * 100, 100)
REDACTED

function formatMoney(currency: string, amount: number): string {
  return new Intl.NumberFormat(undefined, { style: 'currency', currency REDACTED).format(amount)
REDACTED
</script>
