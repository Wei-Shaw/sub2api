<template>
  <div>
    <label class="mb-2 block text-sm font-semibold text-primary-900/80 dark:text-primary-100">
      {{ t('payment.paymentMethod') }}
    </label>
    <div class="grid grid-cols-2 gap-3 sm:flex">
      <button
        v-for="method in sortedMethods"
        :key="method.type"
        type="button"
        :disabled="!method.available"
        :class="[
          'relative flex h-[64px] flex-col items-center justify-center rounded-xl border px-3 transition-all sm:flex-1',
          !method.available
            ? 'cursor-not-allowed border-amber-100/60 bg-primary-50/40 opacity-50 dark:border-dark-700 dark:bg-dark-800/50'
            : selected === method.type
              ? methodSelectedClass(method.type)
              : 'border-amber-100/80 bg-white/85 text-gray-700 shadow-sm shadow-primary-500/5 hover:border-primary-200 hover:bg-primary-50/60 hover:text-primary-800 dark:border-primary-900/25 dark:bg-dark-900/65 dark:text-gray-200 dark:hover:border-primary-800/60 dark:hover:bg-primary-950/25',
        ]"
        @click="method.available && emit('select', method.type)"
      >
        <span class="flex items-center gap-2">
          <img :src="methodIcon(method.type)" :alt="t(`payment.methods.${method.type}`)" class="h-7 w-7 object-contain" />
          <span class="flex flex-col items-start leading-none">
            <span class="text-base font-semibold">{{ t(`payment.methods.${method.type}`) }}</span>
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
import { METHOD_ORDER } from './providerConfig'
import alipayIcon from '@/assets/icons/alipay.svg'
import wxpayIcon from '@/assets/icons/wxpay.svg'
import stripeIcon from '@/assets/icons/stripe.svg'
import airwallexIcon from '@/assets/icons/airwallex.svg'

export interface PaymentMethodOption {
  type: string
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

const METHOD_ICONS: Record<string, string> = {
  alipay: alipayIcon,
  wxpay: wxpayIcon,
  stripe: stripeIcon,
  airwallex: airwallexIcon,
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
  if (type.includes('alipay')) return METHOD_ICONS.alipay
  if (type.includes('wxpay')) return METHOD_ICONS.wxpay
  if (type === 'airwallex') return METHOD_ICONS.airwallex
  return METHOD_ICONS[type] || alipayIcon
}

function methodSelectedClass(type: string): string {
  const brandRing = type.includes('alipay')
    ? 'after:bg-[#02A9F1]'
    : type.includes('wxpay')
      ? 'after:bg-[#09BB07]'
      : type === 'stripe'
        ? 'after:bg-[#676BE5]'
        : type === 'airwallex'
          ? 'after:bg-[#FF6B3D]'
          : 'after:bg-primary-500'
  return [
    'border-primary-400 bg-gradient-to-br from-primary-50 to-amber-50 text-gray-900 shadow-md shadow-primary-500/15',
    'dark:border-primary-500/60 dark:from-primary-900/35 dark:to-primary-950/20 dark:text-gray-100',
    'after:absolute after:left-3 after:top-3 after:h-1.5 after:w-1.5 after:rounded-full',
    brandRing,
  ].join(' ')
}
</script>
