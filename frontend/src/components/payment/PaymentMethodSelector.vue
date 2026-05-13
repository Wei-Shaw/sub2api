<template>
  <div>
    <label class="mb-3 flex items-center justify-between text-sm font-semibold text-slate-800 dark:text-slate-100">
      <span>{{ t('payment.paymentMethod') }}</span>
      <span class="rounded-full bg-blue-50 px-3 py-1 text-xs font-medium text-blue-700 dark:bg-blue-500/10 dark:text-blue-200">官方通道</span>
    </label>
    <div class="grid gap-3 sm:grid-cols-2">
      <button
        v-for="method in sortedMethods"
        :key="method.type"
        type="button"
        :disabled="!method.available"
        :class="[
          'relative flex min-h-[82px] items-center gap-4 rounded-2xl border p-4 text-left transition-all',
          !method.available
            ? 'cursor-not-allowed border-slate-200 bg-slate-50 opacity-50 dark:border-slate-800 dark:bg-slate-800/50'
            : selected === method.type
              ? methodSelectedClass(method.type)
              : 'border-slate-200 bg-white text-slate-700 hover:border-blue-300 hover:shadow-md dark:border-slate-700 dark:bg-slate-950 dark:text-slate-200 dark:hover:border-blue-400/70',
        ]"
        @click="method.available && emit('select', method.type)"
      >
        <span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-slate-50 ring-1 ring-slate-200 dark:bg-slate-900 dark:ring-slate-700">
          <img :src="methodIcon(method.type)" :alt="t(`payment.methods.${method.type}`)" class="h-7 w-7 object-contain" />
        </span>
        <span class="min-w-0 flex-1">
          <span class="flex items-center gap-2">
            <span class="font-semibold text-slate-950 dark:text-white">{{ t(`payment.methods.${method.type}`) }}</span>
            <span v-if="method.available" class="rounded-full bg-orange-50 px-2 py-0.5 text-[11px] font-semibold text-orange-600 dark:bg-orange-500/10 dark:text-orange-200">推荐</span>
          </span>
          <span class="mt-1 block text-xs text-slate-500 dark:text-slate-400">
            {{ methodDescription(method.type) }}
          </span>
          <span
            v-if="method.fee_rate > 0"
            class="mt-1 block text-xs text-slate-500 dark:text-slate-400"
          >
            {{ t('payment.fee') }} {{ method.fee_rate }}%
          </span>
        </span>
        <span
          :class="[
            'flex h-6 w-6 shrink-0 items-center justify-center rounded-full border transition-colors',
            selected === method.type
              ? 'border-blue-600 bg-blue-600 text-white'
              : 'border-slate-300 text-transparent dark:border-slate-600',
          ]"
        >
          <span v-if="selected === method.type" class="text-xs leading-none">✓</span>
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

function methodDescription(type: string): string {
  if (type.includes('alipay')) return '跳转至支付宝安全收银台完成付款'
  if (type.includes('wxpay')) return '使用微信支付完成订单付款'
  if (type === 'stripe') return '支持国际银行卡与 Stripe 通道'
  if (type === 'airwallex') return '支持国际银行卡与 Airwallex 通道'
  return '订单创建后进入安全支付页面'
}

function methodSelectedClass(type: string): string {
  if (type.includes('alipay')) return 'border-[#02A9F1] bg-sky-50 shadow-md shadow-sky-900/10 ring-2 ring-sky-500/10 dark:bg-sky-950/40'
  if (type.includes('wxpay')) return 'border-[#09BB07] bg-emerald-50 shadow-md shadow-emerald-900/10 ring-2 ring-emerald-500/10 dark:bg-emerald-950/40'
  if (type === 'stripe') return 'border-[#676BE5] bg-indigo-50 shadow-md shadow-indigo-900/10 ring-2 ring-indigo-500/10 dark:bg-indigo-950/40'
  if (type === 'airwallex') return 'border-[#FF6B3D] bg-orange-50 shadow-md shadow-orange-900/10 ring-2 ring-orange-500/10 dark:bg-orange-950/40'
  return 'border-blue-500 bg-blue-50 shadow-md shadow-blue-900/10 ring-2 ring-blue-500/10 dark:bg-blue-950/40'
}
</script>
