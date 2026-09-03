<template>
  <div>
    <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
      {{ t('payment.paymentMethod') }}
    </label>
    <div
      data-testid="payment-method-grid"
      class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4"
    >
      <button
        v-for="method in sortedMethods"
        :key="method.type"
        type="button"
        :title="methodLabel(method)"
        :disabled="!method.available"
        :class="[
          'relative flex h-[60px] min-w-0 flex-col items-center justify-center rounded-lg border px-3 transition-all',
          !method.available
            ? 'cursor-not-allowed border-gray-200 bg-gray-50 opacity-50 dark:border-dark-700 dark:bg-dark-800/50'
            : selected === method.type
              ? methodSelectedClass(method.type)
              : 'border-gray-300 bg-white text-gray-700 hover:border-gray-400 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200 dark:hover:border-dark-500',
        ]"
        @click="method.available && emit('select', method.type)"
      >
        <span class="flex w-full min-w-0 items-center justify-center gap-2">
          <img :src="methodIcon(method.type)" :alt="methodLabel(method)" class="h-7 w-7 shrink-0 object-contain" />
          <span class="flex min-w-0 flex-col items-start leading-none">
            <span data-testid="payment-method-label" class="block w-full truncate text-base font-semibold">
              {{ methodLabel(method) }}
            </span>
            <span
              v-if="method.fee_rate > 0"
              class="text-[10px] tracking-wide text-gray-500 dark:text-dark-400"
            >
              {{ t('payment.fee') }} {{ method.fee_rate }}%
            </span>
          </span>
        </span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { METHOD_ORDER, SEPAY_BANK_TRANSFER, SEPAY_CARD, SEPAY_NAPAS } from './providerConfig'
import paymentIcon from '@/assets/icons/payment.svg'

export interface PaymentMethodOption {
  type: string
  display_name?: string
  fee_rate: number
  available: boolean
}

const props = defineProps<{
  methods: PaymentMethodOption[]
  selected: string
}>()

const emit = defineEmits<{
  select: [type: string]
}>()

const { t } = useI18n()

// SePay ships no per-method marks, so every method uses the neutral payment
// glyph rather than a borrowed brand icon.
const METHOD_ICONS: Record<string, string> = {
  [SEPAY_BANK_TRANSFER]: paymentIcon,
  [SEPAY_NAPAS]: paymentIcon,
  [SEPAY_CARD]: paymentIcon,
}

const sortedMethods = computed(() => {
  const order: readonly string[] = METHOD_ORDER
  return [...props.methods].sort((a, b) => {
    const ai = order.indexOf(a.type)
    const bi = order.indexOf(b.type)
    return (ai === -1 ? 999 : ai) - (bi === -1 ? 999 : bi)
  })
})

function methodIcon(type: string): string {
  return METHOD_ICONS[type] || paymentIcon
}

function methodLabel(method: PaymentMethodOption): string {
  return method.display_name || t(`payment.methods.${method.type}`, method.type)
}

function methodSelectedClass(type: string): string {
  switch (type) {
    case SEPAY_BANK_TRANSFER:
      return 'border-[#0A66C2] bg-blue-50 text-gray-900 shadow-sm dark:bg-blue-950 dark:text-gray-100'
    case SEPAY_NAPAS:
      return 'border-[#00875A] bg-emerald-50 text-gray-900 shadow-sm dark:bg-emerald-950 dark:text-gray-100'
    case SEPAY_CARD:
      return 'border-[#6D28D9] bg-violet-50 text-gray-900 shadow-sm dark:bg-violet-950 dark:text-gray-100'
    default:
      return 'border-primary-500 bg-primary-50 text-gray-900 shadow-sm dark:bg-primary-950 dark:text-gray-100'
  }
}
</script>
