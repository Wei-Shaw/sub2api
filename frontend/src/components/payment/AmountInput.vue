<template>
  <div class="space-y-4">
    <!--
      Quick amounts. A `radiogroup`, because that is what it is: the old markup
      was a grid of bare buttons with the selection carried by a 2px tinted
      border and nothing else, so a screen reader heard nine unrelated buttons
      and never learned which one was active.

      Selection is the accent — that is the one job the accent has in this
      system. It is not a status and it is not decoration, so a chip that is
      merely available stays on a hairline.
    -->
    <div>
      <p :id="quickAmountsLabelId" class="mb-2 text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
        {{ t('payment.quickAmounts') }}
      </p>
      <div
        role="radiogroup"
        :aria-labelledby="quickAmountsLabelId"
        class="grid grid-cols-3 gap-2 sm:grid-cols-5"
      >
        <button
          v-for="amt in filteredAmounts"
          :key="amt"
          type="button"
          role="radio"
          :aria-checked="modelValue === amt"
          :class="[
            'h-8 rounded border px-2 font-mono text-sm tabular-nums slashed-zero',
            'transition-colors duration-fast ease-out',
            modelValue === amt
              ? 'border-accent bg-accent-tint text-accent'
              : 'border-line bg-surface text-ink hover:border-line-strong hover:bg-surface-hover',
          ]"
          @click="selectAmount(amt)"
        >
          {{ amt }}
        </button>
      </div>
    </div>

    <!--
      The custom amount is the only field on the recharge tab that can fail
      validation, and its error used to be rendered by the PARENT as a loose
      `<p>` below this component — which meant showing it pushed the payment
      method grid and the submit button down by a line, at the exact moment the
      user was reaching for the button. `FormField` reserves the message row, so
      the error appears in place.
    -->
    <FormField :label="t('payment.customAmount')" :error="error">
      <template #default="{ id, describedBy, invalid }">
        <div class="relative">
          <span
            class="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 font-mono text-sm text-ink-tertiary"
            aria-hidden="true"
          >
            {{ creditedCurrencySymbol }}
          </span>
          <input
            :id="id"
            type="text"
            inputmode="decimal"
            :value="customText"
            :placeholder="placeholderText"
            :aria-describedby="describedBy"
            :aria-invalid="invalid || undefined"
            class="input pl-7 text-right font-mono tabular-nums slashed-zero"
            :class="{ 'input-error': error }"
            @input="handleInput"
          />
        </div>
      </template>
    </FormField>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, useId, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import FormField from '@/components/common/FormField.vue'
import { currencySymbol } from '@/components/payment/currency'

const props = withDefaults(defineProps<{
  amounts?: number[]
  modelValue: number | null
  min?: number
  max?: number
  /**
   * Validation text. Owned by the parent because the rule depends on the
   * selected gateway's per-method limits, but RENDERED here so the message sits
   * against the field it describes instead of floating under the card.
   */
  error?: string
}>(), {
  amounts: () => [10, 20, 50, 100, 200, 500, 1000, 2000, 5000],
  min: 0,
  max: 0,
  error: '',
})

const emit = defineEmits<{
  'update:modelValue': [value: number | null]
}>()

const { t } = useI18n()

const quickAmountsLabelId = `quick-amounts-${useId()}`

/**
 * The recharge amount is always denominated in USD credit, whatever currency
 * the chosen gateway settles in — so this symbol is fixed, not derived from the
 * selected method. Conflating the two is how a user ends up believing they are
 * topping up 100 CNY.
 */
const creditedCurrencySymbol = currencySymbol('USD')

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

function selectAmount(amt: number) {
  customText.value = String(amt)
  emit('update:modelValue', amt)
}

function handleInput(e: Event) {
  const input = e.target as HTMLInputElement
  const val = input.value
  if (!AMOUNT_PATTERN.test(val)) {
    /*
     * Rejecting the keystroke used to mean "return and change nothing", but
     * `:value` is bound to `customText`, so with no reactive change Vue never
     * re-rendered and the rejected characters stayed on screen — the field
     * showed `1.234` while the model held `1.23`. On a payment amount, a field
     * that disagrees with what will be charged is not a cosmetic defect.
     */
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
