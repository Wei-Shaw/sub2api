<template>
  <div>
    <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
      {{ t('payment.paymentMethod') }}
    </label>
    <div class="grid grid-cols-2 gap-3 sm:flex">
      <button
        v-for="method in sortedMethods"
        :key="method.type"
        type="button"
        :disabled="!method.available"
        :class="[
          'relative flex h-[60px] flex-col items-center justify-center rounded-lg border px-3 transition-all sm:flex-1',
          !method.available
            ? 'cursor-not-allowed border-gray-200 bg-gray-50 opacity-50 dark:border-dark-700 dark:bg-dark-800/50'
            : selected === method.type
              ? methodSelectedClass(method.type)
              : 'border-gray-300 bg-white text-gray-700 hover:border-gray-400 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200 dark:hover:border-dark-500',
        ]"
        @click="method.available && emit('select', method.type)"
      >
        <span class="flex items-center gap-2">
          <!-- Alipay official logo -->
          <span v-if="method.type.includes('alipay')" class="flex h-8 w-8 items-center justify-center">
            <svg viewBox="0 0 1024 1024" class="h-8 w-8">
              <path d="M230.4 0h563.2A230.4 230.4 0 0 1 1024 230.4v563.2A230.4 230.4 0 0 1 793.6 1024H230.4A230.4 230.4 0 0 1 0 793.6V230.4A230.4 230.4 0 0 1 230.4 0z" fill="#1677FF"/>
              <path d="M716.8 604.16c-60.42-24.58-128.26-51.2-128.26-51.2s35.84-91.65 49.66-156.67c13.82-65.02 5.12-102.4-40.96-117.76-46.08-15.36-81.92 7.17-97.28 56.32-15.36 49.15-15.36 122.88 5.12 194.56-30.72 56.32-71.68 122.88-112.64 179.2-40.96 56.32-71.68 92.16-71.68 92.16s-97.28 35.84-143.36 76.8c-46.08 40.96-56.32 76.8-40.96 102.4 15.36 25.6 56.32 30.72 112.64-5.12 56.32-35.84 107.52-107.52 148.48-168.96 0 0 81.92-30.72 128-43.01 46.08-12.29 81.92-20.48 81.92-20.48s71.68 66.56 148.48 92.16c76.8 25.6 133.12 15.36 138.24-20.48 5.12-35.84-30.72-61.44-46.08-71.68-41.98-27.65-81.92-46.08-131.32-66.05zM293.63 855.04c-35.84 25.6-66.56 15.36-56.32-10.24 10.24-25.6 46.08-61.44 92.16-86.02 0 0 10.24-5.12 30.72-15.36-30.72 51.2-66.56 112.64-66.56 111.62zm225.28-491.52c-5.12-56.32 10.24-86.02 35.84-86.02s35.84 25.6 25.6 76.8c-10.24 51.2-30.72 107.52-30.72 107.52s-25.6-42.0-30.72-98.3zm-71.68 296.96c25.6-46.08 51.2-97.28 71.68-143.36 0 0 61.44 25.6 97.28 35.84-30.72 12.8-102.4 71.68-168.96 107.52z" fill="#FFFFFF"/>
            </svg>
          </span>
          <!-- WeChat Pay official logo -->
          <span v-else-if="method.type.includes('wxpay')" class="flex h-8 w-8 items-center justify-center">
            <svg viewBox="0 0 1024 1024" class="h-8 w-8">
              <path d="M230.4 0h563.2A230.4 230.4 0 0 1 1024 230.4v563.2A230.4 230.4 0 0 1 793.6 1024H230.4A230.4 230.4 0 0 1 0 793.6V230.4A230.4 230.4 0 0 1 230.4 0z" fill="#07C160"/>
              <path d="M408.06 429.06a25.6 25.6 0 1 0 0-51.2 25.6 25.6 0 0 0 0 51.2zm-152.06 0a25.6 25.6 0 1 0 0-51.2 25.6 25.6 0 0 0 0 51.2z" fill="#FFFFFF"/>
              <path d="M332 271c-119.3 0-216 79.7-216 178 0 55.1 28.9 104.9 74.2 138.7l-18.5 55.7 64.4-33.2c26.6 8.9 55.6 13.9 86 13.9 7.3 0 14.5-0.3 21.6-0.9-9-27.6-14.2-57-14.2-87.5C329.5 408.9 422.2 332.7 535 332.7c7.3 0 14.5 0.4 21.6 1-23.6-36.5-67.8-62.7-118.6-62.7H332z" fill="#FFFFFF"/>
              <path d="M710.4 593.9a20.5 20.5 0 1 0 0-41 20.5 20.5 0 0 0 0 41zm-121.6 0a20.5 20.5 0 1 0 0-41 20.5 20.5 0 0 0 0 41z" fill="#FFFFFF"/>
              <path d="M649.6 365.7c-99.1 0-179.5 69.3-179.5 154.8s80.4 154.8 179.5 154.8c21.3 0 41.8-3.2 60.8-9.2l53.5 27.5-15.4-46.2c37.6-28.1 61.1-69.4 61.1-115.0-0.1-85.4-80.5-154.7-160-166.7z" fill="#FFFFFF"/>
            </svg>
          </span>
          <!-- Stripe icon -->
          <span v-else-if="isStripe(method.type)" :class="['flex h-8 w-8 items-center justify-center rounded-md text-white', iconBgClass(method.type)]">
            <Icon name="creditCard" size="sm" />
          </span>
          <!-- EasyPay icon -->
          <span v-else :class="['flex h-8 w-8 items-center justify-center rounded-md text-white', iconBgClass(method.type)]">
            <span class="text-sm font-bold">{{ iconLabel(method.type) }}</span>
          </span>
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
import Icon from '@/components/icons/Icon.vue'

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

/** Fixed display order for payment methods */
const METHOD_ORDER = ['easypay', 'alipay', 'wxpay', 'stripe']

const sortedMethods = computed(() => {
  return [...props.methods].sort((a, b) => {
    const ai = METHOD_ORDER.indexOf(a.type)
    const bi = METHOD_ORDER.indexOf(b.type)
    return (ai === -1 ? 999 : ai) - (bi === -1 ? 999 : bi)
  })
})

function isStripe(type: string) {
  return type === 'stripe'
}

function iconBgClass(type: string): string {
  if (type === 'easypay') return 'bg-[#FF6B35]'
  if (type === 'stripe') return 'bg-[#635bff]'
  return 'bg-gray-500'
}

function iconLabel(type: string): string {
  if (type === 'easypay') return '易'
  return type[0]?.toUpperCase() || '?'
}

function methodSelectedClass(type: string): string {
  if (type === 'easypay') return 'border-[#FF6B35] bg-orange-50 text-gray-900 shadow-sm dark:bg-orange-950 dark:text-gray-100'
  if (type.includes('alipay')) return 'border-[#1677FF] bg-blue-50 text-gray-900 shadow-sm dark:bg-blue-950 dark:text-gray-100'
  if (type.includes('wxpay')) return 'border-[#07C160] bg-green-50 text-gray-900 shadow-sm dark:bg-green-950 dark:text-gray-100'
  if (type === 'stripe') return 'border-[#635bff] bg-indigo-50 text-gray-900 shadow-sm dark:bg-indigo-950 dark:text-gray-100'
  return 'border-primary-500 bg-primary-50 text-gray-900 shadow-sm dark:bg-primary-950 dark:text-gray-100'
}
</script>
