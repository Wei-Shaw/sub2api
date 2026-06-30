<template>
  <div class="space-y-4">
    <!-- Quick Amount Buttons -->
    <div>
      <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('payment.quickAmounts') }}
      </label>
      <div class="grid grid-cols-3 gap-2">
        <button
          v-for="amt in filteredAmounts"
          :key="amt"
          type="button"
          :class="[
            'relative rounded-lg border-2 px-4 py-3 text-center font-medium transition-colors',
            modelValue === amt
              ? 'border-primary-500 bg-primary-50 text-primary-700 dark:border-primary-400 dark:bg-primary-900/40 dark:text-primary-300'
              : 'border-gray-200 bg-white text-gray-700 hover:border-gray-300 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200 dark:hover:border-dark-500',
          ]"
          @click="selectAmount(amt)"
        >
          <span class="block">{{ amt }}</span>
          <span
            v-if="bonusRateForAmount(amt) > 0"
            class="mt-0.5 block text-[11px] font-semibold leading-none text-amber-600 dark:text-amber-400"
          >
            +{{ formatBonusRate(bonusRateForAmount(amt)) }}%
          </span>
          <span
            v-if="showRedDots && bonusRateForAmount(amt) > 0"
            class="absolute right-1 top-1 inline-block h-3 w-3 rounded-full bg-red-500 ring-2 ring-red-500/30 motion-safe:animate-pulse"
            :aria-label="t('payment.promo.redDotAria')"
          ></span>
        </button>
      </div>
    </div>

    <!-- Custom Amount Input -->
    <div>
      <label class="mb-2 flex items-center gap-2 text-sm font-medium text-gray-700 dark:text-gray-300">
        <span>{{ t('payment.customAmount') }}</span>
        <span
          v-if="customBonusRate > 0"
          class="text-xs font-semibold text-amber-600 dark:text-amber-400"
        >
          {{ t('payment.promo.bonusBadge', { rate: formatBonusRate(customBonusRate) }) }}
        </span>
      </label>
      <div class="relative">
        <span class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-dark-500">
          ¥
        </span>
        <input
          type="text"
          inputmode="decimal"
          :value="customText"
          :placeholder="placeholderText"
          class="input w-full py-3 pl-8 pr-4"
          @input="handleInput"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'

interface BonusTier {
  min_amount: number
  bonus_rate: number
}

const props = withDefaults(defineProps<{
  amounts?: number[]
  modelValue: number | null
  min?: number
  max?: number
  /** 升序 tier 列表；undefined / 空数组都视为没有活动。 */
  bonusTiers?: BonusTier[]
  /** 活动红点开关；为 false 时只渲染 +x% 标签、不渲染圆点。 */
  showRedDots?: boolean
}>(), {
  amounts: () => [10, 20, 50, 100, 200, 500, 1000, 2000, 5000],
  min: 0,
  max: 0,
  bonusTiers: () => [],
  showRedDots: false,
})

const emit = defineEmits<{
  'update:modelValue': [value: number | null]
  /** 用户点击"命中赠送档位"的预置金额时触发，父组件据此 dismiss 红点。 */
  bonusPresetClicked: [amount: number]
}>()

const { t } = useI18n()

const customText = ref('')

// 0 = no limit
const filteredAmounts = computed(() =>
  props.amounts.filter((a) => (props.min <= 0 || a >= props.min) && (props.max <= 0 || a <= props.max))
)

const placeholderText = computed(() => {
  if (props.min > 0 && props.max > 0) return `${props.min} - ${props.max}`
  if (props.min > 0) return `≥ ${props.min}`
  if (props.max > 0) return `≤ ${props.max}`
  return t('payment.enterAmount')
})

const AMOUNT_PATTERN = /^\d*(\.\d{0,2})?$/

/**
 * 找出 `amount` 命中的最高赠送档位的 bonus_rate（升序最末匹配胜出）。
 * tiers 假定已按 min_amount 升序，与后端校验保持一致；若上游传入乱序数组，
 * 这里也只能尽力——展示位无需过度防御。
 */
function bonusRateForAmount(amount: number): number {
  let rate = 0
  for (const tier of props.bonusTiers) {
    if (amount >= tier.min_amount) {
      rate = tier.bonus_rate
    } else {
      break
    }
  }
  return rate
}

function formatBonusRate(rate: number): string {
  // bonus_rate 是 0~1 的小数，按整数百分比显示；25 → "25"
  return String(Math.round(rate * 100))
}

/**
 * 当前自定义输入框值命中的赠送档位倍率。
 * 与 bonusRateForAmount 同源；输入未填 / 非数字 / 非正数时返回 0。
 * 注意：用户点击 quick preset 时也会同步 customText，因此这个 badge
 * 在两种入口下都会反映"当前金额对应的赠送比例"，是预期行为。
 */
const customBonusRate = computed(() => {
  const num = parseFloat(customText.value)
  if (!Number.isFinite(num) || num <= 0) return 0
  return bonusRateForAmount(num)
})

function selectAmount(amt: number) {
  customText.value = String(amt)
  emit('update:modelValue', amt)
  if (bonusRateForAmount(amt) > 0) {
    emit('bonusPresetClicked', amt)
  }
}

function handleInput(e: Event) {
  const val = (e.target as HTMLInputElement).value
  if (!AMOUNT_PATTERN.test(val)) return
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
