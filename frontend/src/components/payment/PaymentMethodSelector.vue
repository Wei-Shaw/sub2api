<template>
  <div>
    <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
      {{ t('payment.paymentMethod') }}
    </label>
    <div class="grid grid-cols-2 gap-3 sm:flex">
      <button
        v-for="method in methods"
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
          <span :class="['flex h-8 w-8 items-center justify-center rounded-md text-white', iconBgClass(method.type)]">
            <Icon v-if="isStripe(method.type)" name="creditCard" size="sm" />
            <span v-else class="text-sm font-bold">
              {{ iconLabel(method.type) }}
            </span>
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
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

export interface PaymentMethodOption {
  type: string
  fee_rate: number
  available: boolean
}

defineProps<{
  methods: PaymentMethodOption[]
  selected: string
}>()

const emit = defineEmits<{
  select: [type: string]
}>()

const { t } = useI18n()

function isStripe(type: string) {
  return type === 'stripe' || type === 'card'
}

function iconBgClass(type: string): string {
  if (type === 'easypay') return 'bg-[#FF6B35]'
  if (type.includes('alipay')) return 'bg-[#00AEEF]'
  if (type.includes('wxpay')) return 'bg-[#07C160]'
  if (type === 'stripe' || type === 'card') return 'bg-[#635bff]'
  return 'bg-gray-500'
}

function iconLabel(type: string): string {
  if (type === 'easypay') return '易'
  if (type.includes('alipay')) return '\u652f'
  if (type.includes('wxpay')) return '\u5fae'
  return type[0]?.toUpperCase() || '?'
}

function methodSelectedClass(type: string): string {
  if (type === 'easypay') return 'border-[#FF6B35] bg-orange-50 text-gray-900 shadow-sm dark:bg-orange-950 dark:text-gray-100'
  if (type.includes('alipay')) return 'border-[#00AEEF] bg-blue-50 text-gray-900 shadow-sm dark:bg-blue-950 dark:text-gray-100'
  if (type.includes('wxpay')) return 'border-[#07C160] bg-green-50 text-gray-900 shadow-sm dark:bg-green-950 dark:text-gray-100'
  if (type === 'stripe' || type === 'card') return 'border-[#635bff] bg-indigo-50 text-gray-900 shadow-sm dark:bg-indigo-950 dark:text-gray-100'
  return 'border-primary-500 bg-primary-50 text-gray-900 shadow-sm dark:bg-primary-950 dark:text-gray-100'
}
</script>
