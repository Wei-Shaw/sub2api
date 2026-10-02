<template>
  <section class="card">
    <div class="card-header">
      <h3 class="card-title">{{ t('payment.admin.paymentDistribution') }}</h3>
    </div>
    <div
      v-if="!methods?.length"
      class="flex h-32 items-center justify-center text-body text-fg-muted"
    >
      {{ t('payment.admin.noData') }}
    </div>
    <div v-else class="divide-y divide-border">
      <div v-for="method in methods" :key="method.type" class="space-y-1.5 px-5 py-3">
        <div class="flex items-start justify-between gap-3">
          <div class="flex items-center gap-2">
            <span :class="['inline-block h-2.5 w-2.5 rounded-sm', colorMap[method.type] || 'bg-border-strong']"></span>
            <span class="text-body text-fg">
              {{ t('payment.methods.' + method.type, method.type) }}
            </span>
          </div>
          <div class="space-y-0.5 text-right tabular-nums">
            <span v-for="[currency, amount] in sortedAmounts(method.amount)" :key="currency" class="block text-body font-semibold text-fg">
              {{ formatMoney(currency, amount) }}
            </span>
            <span class="text-meta text-fg-muted">
              ({{ method.count }})
            </span>
          </div>
        </div>
        <div v-for="[currency, amount] in sortedAmounts(method.amount)" :key="currency" class="flex items-center gap-2">
          <span class="w-10 text-meta text-fg-muted">{{ currency }}</span>
          <div class="progress flex-1">
            <div
              :class="['progress-bar', barColorMap[method.type] || 'bg-border-strong']"
              :style="{ width: barWidth(currency, amount) + '%' }"
            ></div>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { CurrencyAmounts, PaymentMethodStats } from '@/types/payment'

const { t } = useI18n()

const props = defineProps<{
  methods: PaymentMethodStats[]
}>()

const colorMap: Record<string, string> = {
  gpmpay_bank_transfer: 'bg-accent',
  nowpayments_crypto: 'bg-success',
}

const barColorMap: Record<string, string> = {
  gpmpay_bank_transfer: 'bg-accent',
  nowpayments_crypto: 'bg-success',
}

const maxAmounts = computed<CurrencyAmounts>(() => {
  return props.methods.reduce<CurrencyAmounts>((maximums, method) => {
    for (const [currency, amount] of Object.entries(method.amount)) {
      maximums[currency] = Math.max(maximums[currency] || 0, amount)
    }
    return maximums
  }, {})
})

function sortedAmounts(amounts: CurrencyAmounts): [string, number][] {
  return Object.entries(amounts).sort(([left], [right]) => left.localeCompare(right))
}

function barWidth(currency: string, amount: number): number {
  return Math.min((amount / (maxAmounts.value[currency] || 1)) * 100, 100)
}

function formatMoney(currency: string, amount: number): string {
  return new Intl.NumberFormat(undefined, { style: 'currency', currency }).format(amount)
}
</script>
