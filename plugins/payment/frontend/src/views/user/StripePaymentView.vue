<template>
  <component :is="isPopup ? 'div' : AppLayout" :class="isPopup ? 'min-h-screen bg-gray-50 dark:bg-dark-900' : ''">
    <div class="mx-auto max-w-lg space-y-6 py-8" :class="isPopup ? 'px-4' : ''">
      <div v-if="loading" class="flex items-center justify-center py-20">
        <div class="h-8 w-8 animate-spin rounded-full border-4 border-primary-500 border-t-transparent"></div>
      </div>
      <div v-else-if="initError" class="card p-8 text-center">
        <div class="stat-icon-danger mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full">
          <Icon name="exclamationCircle" size="xl" />
        </div>
        <h3 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('payment.stripeLoadFailed') }}</h3>
        <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ initError }}</p>
        <button class="btn btn-primary mt-6" @click="router.push('/purchase')">{{ t('payment.result.backToRecharge') }}</button>
      </div>
      <template v-else>
        <!-- Amount header -->
        <div v-if="order" class="card overflow-hidden">
          <div class="bg-gradient-to-br from-[#635bff] to-[#4f46e5] px-6 py-6 text-center">
            <p class="text-sm font-medium text-indigo-200">{{ t('payment.actualPay') }}</p>
            <p class="mt-1 text-3xl font-bold text-white">{{ formatGatewayAmount(order.pay_amount) }}</p>
          </div>
        </div>

        <!-- WeChat QR -->
        <template v-if="wechatQrUrl">
          <div class="card p-6">
            <div class="flex flex-col items-center space-y-4">
              <p class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('payment.qr.scanWxpay') }}</p>
              <div class="wxpay-qr-frame relative rounded-lg border-2 border-[#2BB741] p-4 dark:border-[#2BB741]/70">
                <img :src="wechatQrUrl" alt="WeChat Pay QR" class="h-56 w-56 rounded" />
                <div class="pointer-events-none absolute inset-0 flex items-center justify-center">
                  <span class="rounded-full bg-[#2BB741] p-2 shadow ring-2 ring-white">
                    <svg class="h-5 w-5 text-white" viewBox="0 0 24 24" fill="currentColor"><path d="M8.691 2.188C3.891 2.188 0 5.476 0 9.53c0 2.212 1.17 4.203 3.002 5.55a.59.59 0 0 1 .213.665l-.39 1.48c-.019.07-.048.141-.048.213 0 .163.13.295.29.295a.326.326 0 0 0 .167-.054l1.903-1.114a.864.864 0 0 1 .717-.098 10.16 10.16 0 0 0 2.837.403c.276 0 .543-.027.811-.05-.857-2.578.157-4.972 1.932-6.446 1.703-1.415 3.882-1.98 5.853-1.838-.576-3.583-4.196-6.348-8.596-6.348zM5.785 5.991c.642 0 1.162.529 1.162 1.18a1.17 1.17 0 0 1-1.162 1.178A1.17 1.17 0 0 1 4.623 7.17c0-.651.52-1.18 1.162-1.18zm5.813 0c.642 0 1.162.529 1.162 1.18a1.17 1.17 0 0 1-1.162 1.178 1.17 1.17 0 0 1-1.162-1.178c0-.651.52-1.18 1.162-1.18zm3.636 4.35c-2.084 0-3.993.672-5.363 1.844-1.188.982-2.004 2.308-2.004 3.862 0 1.207.546 2.355 1.483 3.285.114.113.238.213.358.321l-.105.42c-.021.084-.042.17-.042.253 0 .168.126.258.282.258.065 0 .126-.025.18-.058l1.27-.765a.69.69 0 0 1 .58-.086c.96.282 1.99.437 3.043.437 2.633 0 5.03-.972 6.4-2.5.782-.87 1.258-1.901 1.258-3.006 0-3.328-3.325-6.006-7.34-6.006zm-3.21 3.09c.52 0 .94.429.94.957a.949.949 0 0 1-.94.955.949.949 0 0 1-.94-.955c0-.528.42-.957.94-.957zm4.739 0c.52 0 .94.429.94.957a.949.949 0 0 1-.94.955.949.949 0 0 1-.94-.955c0-.528.42-.957.94-.957z"/></svg>
                  </span>
                </div>
              </div>
              <p class="text-center text-sm text-gray-500 dark:text-gray-400">{{ t('payment.qr.scanWxpayHint') }}</p>
            </div>
          </div>
          <div class="card p-4 text-center">
            <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('payment.qr.waitingPayment') }}</p>
          </div>
        </template>

        <!-- Alipay redirect state -->
        <template v-else-if="redirecting">
          <div class="card p-6">
            <div class="flex flex-col items-center space-y-4 py-4">
              <div class="h-10 w-10 animate-spin rounded-full border-4 border-[#00AEEF] border-t-transparent"></div>
              <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('payment.qr.payInNewWindowHint') }}</p>
            </div>
          </div>
        </template>

        <!-- Success state -->
        <template v-else-if="stripeSuccess">
          <div class="card p-6 text-center">
            <div class="flex flex-col items-center gap-3 py-4">
              <div class="stat-icon-success flex h-16 w-16 items-center justify-center rounded-full">
                <Icon name="check" size="lg" />
              </div>
              <p class="text-lg font-bold text-gray-900 dark:text-white">{{ t('payment.result.success') }}</p>
              <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('payment.stripeSuccessProcessing') }}</p>
            </div>
          </div>
        </template>

        <!-- Full Payment Element -->
        <template v-else-if="showPaymentElement">
          <div class="card p-6">
            <div ref="stripeMountEl" class="min-h-[200px]"></div>
            <p v-if="stripeError" class="text-semantic-danger mt-4 text-sm">{{ stripeError }}</p>
            <button class="btn btn-stripe mt-6 w-full py-3 text-base" :disabled="stripeSubmitting || !stripeReady" @click="handleGenericPay">
              <span v-if="stripeSubmitting" class="flex items-center justify-center gap-2">
                <span class="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
                {{ t('common.processing') }}
              </span>
              <span v-else>{{ t('payment.stripePay') }}</span>
            </button>
          </div>
          <div class="text-center">
            <button class="btn btn-secondary" @click="router.push('/purchase')">{{ t('payment.result.backToRecharge') }}</button>
          </div>
        </template>

        <!-- Error state -->
        <div v-if="stripeError && !showPaymentElement" class="card p-4">
          <p class="text-semantic-danger text-sm">{{ stripeError }}</p>
          <button class="btn btn-secondary mt-3 w-full" @click="router.push('/purchase')">{{ t('payment.result.backToRecharge') }}</button>
        </div>
      </template>
    </div>
  </component>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { formatPaymentAmount } from '../../components/payment/currency'
import AppLayout from '../../components/common/AppLayout.vue'
import { Icon } from '@sub2api/plugin-sdk'
import { useStripePayment } from './useStripePayment'

const i18n = useI18n()
const { t } = i18n
const route = useRoute()
const router = useRouter()

const isPopup = computed(() => !!route.query.method || route.query.popup === '1')

const {
  loading,
  initError,
  stripeError,
  stripeSubmitting,
  stripeSuccess,
  stripeReady,
  order,
  currency,
  wechatQrUrl,
  redirecting,
  showPaymentElement,
  stripeMountEl,
  initStripe,
  mountPaymentElement,
  handleGenericPay,
} = useStripePayment()

const localeCode = computed(() => {
  const raw = i18n.locale as unknown
  if (typeof raw === 'string') return raw
  if (raw && typeof raw === 'object' && 'value' in raw) {
    return String((raw as { value?: string }).value || '')
  }
  return undefined
})

function formatGatewayAmount(value: number | string): string {
  const num = typeof value === 'number' ? value : Number(value)
  return formatPaymentAmount(num, currency.value, localeCode.value)
}

onMounted(async () => {
  await initStripe()
  if (showPaymentElement.value) {
    await nextTick()
    mountPaymentElement()
  }
})
</script>

<style scoped>
/* WeChat Pay 品牌色二维码框底 (微信绿)。品牌专属浅底, 非通用语义色,
   故保留在组件内, 不进 SDK 通用语义类。 */
.wxpay-qr-frame {
  @apply bg-green-50 dark:bg-green-950/20;
}
</style>
