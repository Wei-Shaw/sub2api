<template>
  <div class="card p-4">
    <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">
      {{ t('payment.admin.paymentDistribution') }}
    </h3>
    <div
      v-if="!methods?.length"
      class="flex h-32 items-center justify-center text-sm text-gray-500 dark:text-gray-400"
    >
      {{ t('payment.admin.noData') }}
    </div>
    <div v-else class="space-y-3">
      <div v-for="method in methods" :key="method.type" class="space-y-1">
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <span :class="['inline-block h-3 w-3 rounded-full', colorMap[method.type] || 'bg-gray-400']"></span>
            <span class="text-sm text-gray-700 dark:text-gray-300">
              {{ t('payment.methods.' + method.type, method.type) }}
            </span>
          </div>
          <div class="text-right">
            <span class="text-sm font-medium text-gray-900 dark:text-white">
              ${{ formatMoney(method.amount) }}
            </span>
            <span class="ml-2 text-xs text-gray-500 dark:text-gray-400">
              ({{ method.count }})
            </span>
          </div>
        </div>
        <div class="h-2 w-full overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
          <div
            :class="['h-full rounded-full transition-all', barColorMap[method.type] || 'bg-gray-400']"
            :style="{ width: barWidth(method.amount) + '%' }"
          ></div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { money, formatMoney } from '../../../utils/decimal'

const { t } = useI18n()

const props = defineProps<{
  methods: { type: string; amount: string; count: number }[]
}>()

const colorMap: Record<string, string> = {
  alipay: 'bg-blue-500',
  wxpay: 'bg-green-500',
  alipay_direct: 'bg-blue-400',
  wxpay_direct: 'bg-green-400',
  stripe: 'bg-purple-500',
}

const barColorMap: Record<string, string> = {
  alipay: 'bg-blue-500',
  wxpay: 'bg-green-500',
  alipay_direct: 'bg-blue-400',
  wxpay_direct: 'bg-green-400',
  stripe: 'bg-purple-500',
}

// Bar width is a UI-layout calculation (0..100%), not money — converting
// to JS number is fine here as the precision lost rounding to a percent is
// invisible. money() guards against null/undefined.
const maxAmount = computed<number>(() => {
  if (!props.methods?.length) return 1
  const numbers = props.methods.map(m => money(m.amount).toNumber())
  return Math.max(...numbers, 1)
})

function barWidth(amount: string): number {
  return Math.min((money(amount).toNumber() / maxAmount.value) * 100, 100)
}
</script>
