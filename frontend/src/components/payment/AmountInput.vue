<template>
  <div class="space-y-4">
    <!-- Quick Amount Buttons -->
    <div v-if="filteredAmounts.length > 0">
      <label class="mb-4 block text-base font-semibold text-gray-900 dark:text-white">
        {{ t('payment.quickAmounts') }}
        <span class="ml-1 text-xs font-normal text-gray-400 dark:text-gray-500">· {{ t('payment.paymentAmount') }}</span>
      </label>
      <div class="grid grid-cols-2 gap-3 pt-2 sm:grid-cols-3 sm:gap-4">
        <button
          v-for="amt in filteredAmounts"
          :key="amt"
          type="button"
          :class="[
            'relative rounded-2xl border px-3 py-5 text-center font-semibold tabular-nums transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary-500',
            modelValue === amt
              ? 'border-primary-500 bg-primary-50 text-primary-700 ring-1 ring-primary-500 dark:border-primary-400 dark:bg-primary-900/40 dark:text-primary-300 dark:ring-primary-400'
              : quoteFor(amt).percent > 0
                ? 'border-teal-200 bg-white text-gray-700 hover:border-teal-400 dark:border-teal-500/25 dark:bg-dark-800 dark:text-gray-200 dark:hover:border-teal-400/60'
                : 'border-gray-200 bg-white text-gray-700 hover:border-gray-300 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200 dark:hover:border-dark-500',
          ]"
          :data-testid="`quick-amount-${amt}`"
          :aria-pressed="modelValue === amt"
          @click="selectAmount(amt)"
        >
          <!-- 促销价签仅在命中优惠档位时展示 -->
          <span
            v-if="quoteFor(amt).percent > 0"
            class="pointer-events-none absolute -top-2.5 right-2 z-10"
            data-testid="quick-amount-bonus-badge"
          >
            <span
              class="inline-flex items-center whitespace-nowrap rounded-full border border-teal-200 bg-teal-50 px-2 py-0.5 text-[10px] font-semibold leading-tight tracking-tight text-teal-700 dark:border-teal-700 dark:bg-teal-950 dark:text-teal-200"
            >
              <span>{{ badgeText(amt) }}</span>
            </span>
          </span>
          <span class="block text-xl tracking-tight">{{ formatPaymentAmount(quoteFor(amt).payBase, currency) }}</span>
          <!-- 下方固定显示最终到账金额，避免把支付金额和到账余额混淆。 -->
          <span
            :class="[
              'mt-0.5 block text-[11px] font-normal leading-tight',
              quoteFor(amt).percent > 0 ? 'text-teal-700 dark:text-teal-300' : 'text-gray-500 dark:text-gray-400',
            ]"
            data-testid="quick-amount-credited"
          >{{ secondLine(amt) }}</span>
        </button>
      </div>
    </div>

    <!-- Custom Amount Input -->
    <div>
      <label for="recharge-custom-amount" class="mb-3 mt-6 block text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('payment.customAmount') }}
      </label>
      <div class="relative">
        <span class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-dark-500">
          {{ currencySymbol(currency || 'USD') }}
        </span>
        <input
          id="recharge-custom-amount"
          type="text"
          inputmode="decimal"
          :value="customText"
          :placeholder="placeholderText"
          class="input w-full rounded-xl py-3.5 pl-8 pr-4 tabular-nums"
          @input="handleInput"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { RechargeBonusTier } from '@/types/payment'
import { formatRechargeBonusNumber, quoteRechargeBonus, type RechargeBonusMode } from '@/utils/rechargeBonus'
import { currencySymbol, formatPaymentAmount } from './currency'

const props = withDefaults(defineProps<{
  amounts?: number[]
  modelValue: number | null
  min?: number
  max?: number
  /** 充值优惠阶梯（按 min_amount 升序）；为空时不显示价签与第二行 */
  bonusTiers?: RechargeBonusTier[]
  /** 阶梯模式：bonus 赠金 / discount 折扣 */
  bonusMode?: RechargeBonusMode
  /** 充值倍率（1 支付币种 = multiplier USD），用于计算到账金额 */
  multiplier?: number
  /** 支付币种（顶部实付金额与自定义金额输入使用） */
  currency?: string
  /** 到账余额币种；余额固定以 USD 记账，但保留参数便于明确展示。 */
  creditedCurrency?: string
}>(), {
  amounts: () => [10, 20, 50, 100, 200, 500, 1000, 2000, 5000],
  min: 0,
  max: 0,
  bonusTiers: () => [],
  bonusMode: 'bonus',
  multiplier: 1,
  currency: undefined,
  creditedCurrency: 'USD',
})

const emit = defineEmits<{
  'update:modelValue': [value: number | null]
}>()

const { t } = useI18n()

const customText = ref('')

// 0 = no limit
const filteredAmounts = computed(() =>
  props.amounts.filter((a) => (props.min <= 0 || a >= props.min) && (props.max <= 0 || a <= props.max))
)

function currencyDigits(): number {
  if (!props.currency) return 2
  try {
    return new Intl.NumberFormat(undefined, { style: 'currency', currency: props.currency }).resolvedOptions().maximumFractionDigits ?? 2
  } catch {
    return 2
  }
}

function quoteFor(amt: number) {
  return quoteRechargeBonus(props.bonusTiers, amt, {
    multiplier: props.multiplier,
    mode: props.bonusMode,
    currencyDigits: currencyDigits(),
  })
}

// 价签文案：赠金「+20%」，折扣「20% OFF」
function badgeText(amt: number): string {
  const percent = formatRechargeBonusNumber(quoteFor(amt).percent)
  return props.bonusMode === 'discount' ? `${percent}% OFF` : `+${percent}%`
}

function secondLine(amt: number): string {
  const quote = quoteFor(amt)
  return t('payment.rechargeBonus.creditedShort', { amount: formatPaymentAmount(quote.credited, props.creditedCurrency) })
}

const placeholderText = computed(() => {
  if (props.min > 0 && props.max > 0) return `${props.min} - ${props.max}`
  if (props.min > 0) return `≥ ${props.min}`
  if (props.max > 0) return `≤ ${props.max}`
  return t('payment.enterAmount')
})

const AMOUNT_PATTERN = /^\d*(\.\d{0,2})?$/

function selectAmount(amt: number) {
  customText.value = String(amt)
  emit('update:modelValue', amt)
}

function handleInput(e: Event) {
  const input = e.target as HTMLInputElement
  const val = input.value
  if (!AMOUNT_PATTERN.test(val)) {
    input.value = customText.value
    return
  }
  customText.value = val
  if (val === '') {
    emit('update:modelValue', null)
    return
  }
  const num = parseFloat(val)
  if (!isNaN(num) && num > 0) {
    emit('update:modelValue', num)
  } else {
    emit('update:modelValue', null)
  }
}

watch(() => props.modelValue, (v) => {
  if (v !== null && String(v) !== customText.value) {
    customText.value = String(v)
  }
}, { immediate: true })
</script>
