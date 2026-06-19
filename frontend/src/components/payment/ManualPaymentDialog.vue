<template>
  <BaseDialog
    :show="show"
    :title="submitted ? t('payment.manual.submittedTitle') : t('payment.manual.title')"
    width="normal"
    @close="handleClose"
  >
    <div class="space-y-5" data-testid="manual-payment-dialog">
      <div class="rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800">
        <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('payment.manual.planLabel') }}</p>
        <div class="mt-1 flex flex-wrap items-end justify-between gap-2">
          <p class="text-lg font-semibold text-gray-900 dark:text-white">{{ plan.name }}</p>
          <p class="text-2xl font-bold text-primary-600 dark:text-primary-400">
            {{ formattedAmount }}
          </p>
        </div>
      </div>

      <template v-if="!submitted">
        <div class="grid grid-cols-2 gap-2 rounded-lg bg-gray-100 p-1 dark:bg-dark-800">
          <button
            type="button"
            :class="tabClass('wxpay')"
            data-testid="manual-payment-tab-wxpay"
            @click="activeMethod = 'wxpay'"
          >
            {{ t('payment.manual.wxpay') }}
          </button>
          <button
            type="button"
            :class="tabClass('alipay')"
            data-testid="manual-payment-tab-alipay"
            @click="activeMethod = 'alipay'"
          >
            {{ t('payment.manual.alipay') }}
          </button>
        </div>

        <div class="rounded-lg border border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-900">
          <img
            v-if="activeMethod === 'wxpay'"
            :src="wxpayQr"
            :alt="t('payment.manual.wxpayQrAlt')"
            class="mx-auto max-h-[52vh] w-full max-w-sm rounded-md object-contain"
            data-testid="manual-payment-wxpay-qr"
          />
          <img
            v-else
            :src="alipayQr"
            :alt="t('payment.manual.alipayQrAlt')"
            class="mx-auto max-h-[52vh] w-full max-w-sm rounded-md object-contain"
            data-testid="manual-payment-alipay-qr"
          />
        </div>

        <p class="text-sm leading-relaxed text-gray-500 dark:text-gray-400">
          {{ t('payment.manual.scanHint') }}
        </p>
      </template>

      <div v-else class="space-y-4 rounded-lg border border-green-200 bg-green-50 p-5 text-center dark:border-green-900/50 dark:bg-green-900/20">
        <div class="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-green-100 text-2xl text-green-600 dark:bg-green-900/50 dark:text-green-300">
          ✓
        </div>
        <div>
          <p class="text-lg font-semibold text-green-700 dark:text-green-200">
            {{ t('payment.manual.submittedTitle') }}
          </p>
          <p class="mt-2 text-sm leading-relaxed text-green-700/80 dark:text-green-200/80">
            {{ t('payment.manual.submittedHint') }}
          </p>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="flex flex-col-reverse gap-3 sm:flex-row sm:justify-end">
        <button type="button" class="btn btn-secondary" @click="handleClose">
          {{ t('common.cancel') }}
        </button>
        <button
          v-if="!submitted"
          type="button"
          class="btn btn-primary"
          data-testid="manual-payment-complete"
          @click="submitted = true"
        >
          {{ t('payment.manual.complete') }}
        </button>
        <button
          v-else
          type="button"
          class="btn btn-primary"
          data-testid="manual-payment-redeem"
          @click="emit('redeem')"
        >
          {{ t('payment.manual.goRedeem') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { formatPaymentAmount } from '@/components/payment/currency'
import type { SubscriptionPlan } from '@/types/payment'
import wxpayQr from '@/assets/payment/manual-wxpay.jpg'
import alipayQr from '@/assets/payment/manual-alipay.jpg'

const props = defineProps<{
  show: boolean
  plan: SubscriptionPlan
  localeCode?: string
}>()

const emit = defineEmits<{
  close: []
  redeem: []
}>()

const { t } = useI18n()
const activeMethod = ref<'wxpay' | 'alipay'>('wxpay')
const submitted = ref(false)

const formattedAmount = computed(() => formatPaymentAmount(props.plan.price, 'CNY', props.localeCode))

function tabClass(method: 'wxpay' | 'alipay') {
  return [
    'rounded-md px-3 py-2 text-sm font-medium transition-colors',
    activeMethod.value === method
      ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
      : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200',
  ]
}

function handleClose() {
  emit('close')
}

watch(() => props.show, (show) => {
  if (show) {
    activeMethod.value = 'wxpay'
    submitted.value = false
  }
})
</script>
