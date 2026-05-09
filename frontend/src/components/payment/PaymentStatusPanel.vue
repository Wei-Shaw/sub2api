<template>
  <div class="space-y-4">
    <!-- ═══ Terminal States: show result, user clicks to return ═══ -->

    <!-- Success -->
    <template v-if="outcome === 'success'">
      <div class="card p-6">
        <div class="flex flex-col items-center space-y-4 py-4">
          <div class="flex h-16 w-16 items-center justify-center rounded-full bg-green-100 dark:bg-green-900/30">
            <Icon name="check" size="lg" class="text-green-500" />
          </div>
          <p class="text-lg font-bold text-gray-900 dark:text-white">{{ props.orderType === 'subscription' ? t('payment.result.subscriptionSuccess') : t('payment.result.success') }}</p>
          <div v-if="paidOrder" class="w-full rounded-xl bg-gray-50 p-4 dark:bg-dark-800">
            <div class="space-y-2 text-sm">
              <div class="flex justify-between">
                <span class="text-gray-500 dark:text-gray-400">{{ t('payment.orders.orderId') }}</span>
                <span class="font-medium text-gray-900 dark:text-white">#{{ paidOrder.id }}</span>
              </div>
              <div v-if="paidOrder.out_trade_no" class="flex justify-between">
                <span class="text-gray-500 dark:text-gray-400">{{ t('payment.orders.orderNo') }}</span>
                <span class="font-medium text-gray-900 dark:text-white">{{ paidOrder.out_trade_no }}</span>
              </div>
              <div class="flex justify-between">
                <span class="text-gray-500 dark:text-gray-400">{{ t('payment.orders.amount') }}</span>
                <span class="font-medium text-gray-900 dark:text-white">{{ paidOrder.order_type === 'balance' ? '$' : '¥' }}{{ paidOrder.amount.toFixed(2) }}</span>
              </div>
              <div class="flex justify-between">
                <span class="text-gray-500 dark:text-gray-400">{{ t('payment.orders.payAmount') }}</span>
                <span class="font-medium text-gray-900 dark:text-white">¥{{ paidOrder.pay_amount.toFixed(2) }}</span>
              </div>
            </div>
          </div>
          <button class="btn btn-primary" @click="handleDone">{{ t('common.confirm') }}</button>
        </div>
      </div>
    </template>

    <!-- Cancelled -->
    <template v-else-if="outcome === 'cancelled'">
      <div class="card p-6">
        <div class="flex flex-col items-center space-y-4 py-4">
          <div class="flex h-16 w-16 items-center justify-center rounded-full bg-gray-100 dark:bg-dark-700">
            <svg class="h-8 w-8 text-gray-400 dark:text-gray-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </div>
          <p class="text-lg font-bold text-gray-900 dark:text-white">{{ t('payment.qr.cancelled') }}</p>
          <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('payment.qr.cancelledDesc') }}</p>
          <button class="btn btn-primary" @click="handleDone">{{ t('common.confirm') }}</button>
        </div>
      </div>
    </template>

    <!-- Expired / Failed -->
    <template v-else-if="outcome === 'expired'">
      <div class="card p-6">
        <div class="flex flex-col items-center space-y-4 py-4">
          <div class="flex h-16 w-16 items-center justify-center rounded-full bg-orange-100 dark:bg-orange-900/30">
            <svg class="h-8 w-8 text-orange-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M12 6v6h4.5m4.5 0a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
          </div>
          <p class="text-lg font-bold text-gray-900 dark:text-white">{{ t('payment.qr.expired') }}</p>
          <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('payment.qr.expiredDesc') }}</p>
          <button class="btn btn-primary" @click="handleDone">{{ t('common.confirm') }}</button>
        </div>
      </div>
    </template>

    <!-- ═══ Active States: QR or Popup waiting ═══ -->

    <!-- QR Code Mode -->
    <template v-else-if="qrUrl">
      <div class="card overflow-hidden">
        <div :class="['border-b px-5 py-4 sm:px-6', providerHeaderClass]">
          <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div class="flex min-w-0 items-center gap-3">
              <span :class="['flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-white shadow-sm ring-1 dark:bg-dark-800', providerIconRingClass]">
                <img v-if="hasProviderLogo" :src="paymentProviderIcon" alt="" class="h-7 w-7" />
                <Icon v-else name="creditCard" size="lg" :class="providerTextClass" />
              </span>
              <div class="min-w-0">
                <p class="text-xs font-semibold text-gray-500 dark:text-gray-400">{{ t('payment.qr.cashierTitle') }}</p>
                <h2 class="mt-1 truncate text-lg font-semibold text-gray-900 dark:text-white">{{ scanTitle }}</h2>
              </div>
            </div>
            <div class="flex flex-wrap items-center gap-2">
              <span class="inline-flex items-center gap-1.5 rounded-full border border-green-200 bg-green-50 px-3 py-1 text-xs font-semibold text-green-700 dark:border-green-500/25 dark:bg-green-500/10 dark:text-green-300">
                <span class="h-2 w-2 rounded-full bg-green-500"></span>
                {{ t('payment.qr.pendingStatus') }}
              </span>
              <span class="inline-flex items-center rounded-full border border-gray-200 bg-white px-3 py-1 text-xs font-medium text-gray-600 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300">
                {{ t('payment.qr.orderLabel', { id: props.orderId }) }}
              </span>
            </div>
          </div>
        </div>

        <div class="grid gap-6 p-5 sm:p-6 lg:grid-cols-[minmax(0,1fr)_minmax(260px,0.85fr)]">
          <div class="flex flex-col items-center rounded-2xl border border-gray-100 bg-gray-50/70 p-5 dark:border-dark-700 dark:bg-dark-900/30">
            <p class="text-sm font-semibold text-gray-800 dark:text-gray-100">{{ t('payment.qr.qrCodeLabel') }}</p>
            <div class="mt-4 rounded-2xl bg-white p-4 shadow-sm ring-1 ring-gray-200 dark:ring-dark-700">
              <div :class="['relative rounded-xl border p-3', qrBorderClass]">
                <canvas ref="qrCanvas" class="mx-auto"></canvas>
                <div class="pointer-events-none absolute inset-0 flex items-center justify-center">
                  <span :class="['flex h-11 w-11 items-center justify-center rounded-full bg-white shadow-md ring-2', qrLogoRingClass]">
                    <img v-if="hasProviderLogo" :src="paymentProviderIcon" alt="" class="h-7 w-7" />
                    <Icon v-else name="creditCard" size="md" class="text-gray-500" />
                  </span>
                </div>
              </div>
            </div>
            <p v-if="scanHint" class="mt-4 max-w-xs text-center text-sm leading-6 text-gray-500 dark:text-gray-400">{{ scanHint }}</p>
            <div class="mt-4 flex w-full max-w-xs items-center gap-2 rounded-xl border border-gray-200 bg-white px-3 py-2 text-sm text-gray-600 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300">
              <Icon name="shield" size="sm" class="shrink-0 text-green-500" />
              <span>{{ t('payment.qr.securePayment') }}</span>
            </div>
          </div>

          <div class="flex flex-col gap-4">
            <div class="rounded-2xl border border-gray-100 bg-white p-4 dark:border-dark-700 dark:bg-dark-800/70">
              <div class="flex items-start justify-between gap-4">
                <div>
                  <p class="text-sm font-medium text-gray-500 dark:text-gray-400">{{ t('payment.qr.expiresIn') }}</p>
                  <p class="mt-1 text-3xl font-bold tabular-nums text-gray-900 dark:text-white">{{ countdownDisplay }}</p>
                </div>
                <span class="inline-flex shrink-0 items-center gap-1.5 rounded-full bg-gray-100 px-3 py-1 text-xs font-semibold text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                  <Icon name="clock" size="xs" />
                  {{ t('payment.qr.pendingStatus') }}
                </span>
              </div>
              <div class="mt-4 h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                <div class="h-full rounded-full transition-all duration-500" :class="countdownBarClass" :style="{ width: countdownProgressStyle }"></div>
              </div>
              <p class="mt-3 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('payment.qr.autoConfirmHint') }}</p>
            </div>

            <div class="rounded-2xl border border-gray-100 bg-gray-50/60 p-4 dark:border-dark-700 dark:bg-dark-900/25">
              <div class="flex items-center gap-2 text-sm font-semibold text-gray-800 dark:text-gray-100">
                <Icon name="clipboard" size="sm" class="text-gray-500" />
                <span>{{ t('payment.qr.stepsTitle') }}</span>
              </div>
              <div class="mt-4 space-y-3">
                <div v-for="(step, index) in paymentSteps" :key="step" class="flex items-start gap-3">
                  <span :class="['mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-xs font-bold text-white', providerStepClass]">{{ index + 1 }}</span>
                  <p class="pt-0.5 text-sm leading-5 text-gray-600 dark:text-gray-300">{{ step }}</p>
                </div>
              </div>
            </div>

            <div class="rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm leading-6 text-amber-800 dark:border-amber-500/25 dark:bg-amber-500/10 dark:text-amber-100">
              <div class="flex gap-2">
                <Icon name="infoCircle" size="sm" class="mt-0.5 shrink-0 text-amber-500 dark:text-amber-300" />
                <span>{{ t('payment.qr.expireWarning') }}</span>
              </div>
            </div>

            <div class="flex flex-col-reverse gap-3 pt-1 sm:flex-row sm:justify-end">
              <button class="btn btn-secondary w-full sm:w-auto" :disabled="cancelling" @click="handleCancel">
                {{ cancelling ? t('common.processing') : t('payment.qr.cancelOrder') }}
              </button>
              <button v-if="payUrl" :class="['btn w-full sm:w-auto', providerButtonClass]" @click="reopenPopup">
                <Icon name="externalLink" size="sm" />
                <span>{{ t('payment.qr.openPayWindow') }}</span>
              </button>
            </div>
          </div>
        </div>
      </div>
    </template>

    <!-- Waiting for Popup/Redirect Mode -->
    <template v-else>
      <div class="card overflow-hidden">
        <div :class="['border-b px-5 py-4 sm:px-6', providerHeaderClass]">
          <div class="flex items-center justify-between gap-4">
            <div class="flex min-w-0 items-center gap-3">
              <span :class="['flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-white shadow-sm ring-1 dark:bg-dark-800', providerIconRingClass]">
                <img v-if="hasProviderLogo" :src="paymentProviderIcon" alt="" class="h-7 w-7" />
                <Icon v-else name="creditCard" size="lg" :class="providerTextClass" />
              </span>
              <div class="min-w-0">
                <p class="text-xs font-semibold text-gray-500 dark:text-gray-400">{{ t('payment.qr.cashierTitle') }}</p>
                <h2 class="mt-1 truncate text-lg font-semibold text-gray-900 dark:text-white">{{ t('payment.qr.redirectTitle') }}</h2>
              </div>
            </div>
            <span class="hidden shrink-0 items-center gap-1.5 rounded-full border border-green-200 bg-green-50 px-3 py-1 text-xs font-semibold text-green-700 dark:border-green-500/25 dark:bg-green-500/10 dark:text-green-300 sm:inline-flex">
              <span class="h-2 w-2 rounded-full bg-green-500"></span>
              {{ t('payment.qr.pendingStatus') }}
            </span>
          </div>
        </div>

        <div class="p-5 sm:p-6">
          <div class="flex flex-col items-center rounded-2xl border border-gray-100 bg-gray-50/70 px-5 py-8 text-center dark:border-dark-700 dark:bg-dark-900/30">
            <div class="flex h-16 w-16 items-center justify-center rounded-full bg-white shadow-sm ring-1 ring-gray-200 dark:bg-dark-800 dark:ring-dark-700">
              <div class="h-9 w-9 animate-spin rounded-full border-4 border-primary-500 border-t-transparent"></div>
            </div>
            <p class="mt-5 text-base font-semibold text-gray-900 dark:text-white">{{ t('payment.qr.redirectDesc') }}</p>
            <p class="mt-2 max-w-md text-sm leading-6 text-gray-500 dark:text-gray-400">{{ t('payment.qr.payInNewWindowHint') }}</p>
            <div class="mt-6 w-full max-w-md rounded-2xl border border-gray-100 bg-white p-4 dark:border-dark-700 dark:bg-dark-800/70">
              <div class="flex items-center justify-between gap-4">
                <span class="text-sm font-medium text-gray-500 dark:text-gray-400">{{ t('payment.qr.expiresIn') }}</span>
                <span class="text-2xl font-bold tabular-nums text-gray-900 dark:text-white">{{ countdownDisplay }}</span>
              </div>
              <div class="mt-4 h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                <div class="h-full rounded-full transition-all duration-500" :class="countdownBarClass" :style="{ width: countdownProgressStyle }"></div>
              </div>
            </div>
          </div>

          <div class="mt-4 flex flex-col-reverse gap-3 sm:flex-row sm:justify-end">
            <button class="btn btn-secondary w-full sm:w-auto" :disabled="cancelling" @click="handleCancel">
              {{ cancelling ? t('common.processing') : t('payment.qr.cancelOrder') }}
            </button>
            <button v-if="payUrl" :class="['btn w-full sm:w-auto', providerButtonClass]" @click="reopenPopup">
              <Icon name="externalLink" size="sm" />
              <span>{{ t('payment.qr.openPayWindow') }}</span>
            </button>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onUnmounted, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { usePaymentStore } from '@/stores/payment'
import { useAppStore } from '@/stores'
import { paymentAPI } from '@/api/payment'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { getPaymentPopupFeatures } from '@/components/payment/providerConfig'
import type { PaymentOrder } from '@/types/payment'
import Icon from '@/components/icons/Icon.vue'
import QRCode from 'qrcode'
import alipayIcon from '@/assets/icons/alipay.svg'
import wxpayIcon from '@/assets/icons/wxpay.svg'

const props = defineProps<{
  orderId: number
  qrCode: string
  expiresAt: string
  paymentType: string
  payUrl?: string
  orderType?: string
}>()

type PaymentOutcome = 'success' | 'cancelled' | 'expired'

const emit = defineEmits<{ done: []; success: []; settled: [outcome: PaymentOutcome] }>()

const { t } = useI18n()
const paymentStore = usePaymentStore()
const appStore = useAppStore()

const qrCanvas = ref<HTMLCanvasElement | null>(null)
const qrUrl = ref('')
const remainingSeconds = ref(0)
const initialSeconds = ref(30 * 60)
const cancelling = ref(false)
const paidOrder = ref<PaymentOrder | null>(null)

// Terminal outcome: null = still active, 'success' | 'cancelled' | 'expired'
const outcome = ref<PaymentOutcome | null>(null)

let pollTimer: ReturnType<typeof setInterval> | null = null
let countdownTimer: ReturnType<typeof setInterval> | null = null

const isAlipay = computed(() => props.paymentType.includes('alipay'))
const isWxpay = computed(() => props.paymentType.includes('wxpay'))
const hasProviderLogo = computed(() => isAlipay.value || isWxpay.value)

const paymentProviderIcon = computed(() => (isAlipay.value ? alipayIcon : wxpayIcon))

const providerName = computed(() => {
  if (isAlipay.value) return t('payment.methods.alipay')
  if (isWxpay.value) return t('payment.methods.wxpay')
  return t('payment.orders.paymentMethod')
})

const providerHeaderClass = computed(() => {
  if (isAlipay.value) return 'border-[#00AEEF]/15 bg-[#00AEEF]/5 dark:bg-[#00AEEF]/10'
  if (isWxpay.value) return 'border-[#2BB741]/15 bg-[#2BB741]/5 dark:bg-[#2BB741]/10'
  return 'border-gray-100 bg-gray-50/80 dark:border-dark-700 dark:bg-dark-900/30'
})

const providerIconRingClass = computed(() => {
  if (isAlipay.value) return 'ring-[#00AEEF]/20'
  if (isWxpay.value) return 'ring-[#2BB741]/20'
  return 'ring-gray-200 dark:ring-dark-600'
})

const providerTextClass = computed(() => {
  if (isAlipay.value) return 'text-[#00AEEF]'
  if (isWxpay.value) return 'text-[#2BB741]'
  return 'text-gray-500'
})

const qrBorderClass = computed(() => {
  if (isAlipay.value) return 'border-[#00AEEF]/30 bg-white'
  if (isWxpay.value) return 'border-[#2BB741]/30 bg-white'
  return 'border-gray-200 bg-white'
})

const qrLogoRingClass = computed(() => {
  if (isAlipay.value) return 'ring-[#00AEEF]/20'
  if (isWxpay.value) return 'ring-[#2BB741]/20'
  return 'ring-gray-200'
})

const providerStepClass = computed(() => {
  if (isAlipay.value) return 'bg-[#00AEEF]'
  if (isWxpay.value) return 'bg-[#2BB741]'
  return 'bg-primary-500'
})

const providerButtonClass = computed(() => {
  if (isAlipay.value) return 'btn-alipay'
  if (isWxpay.value) return 'btn-wxpay'
  return 'btn-primary'
})

const scanTitle = computed(() => {
  if (isAlipay.value) return t('payment.qr.scanAlipay')
  if (isWxpay.value) return t('payment.qr.scanWxpay')
  return t('payment.qr.scanToPay')
})

const scanHint = computed(() => {
  if (isAlipay.value) return t('payment.qr.scanAlipayHint')
  if (isWxpay.value) return t('payment.qr.scanWxpayHint')
  return ''
})

const countdownDisplay = computed(() => {
  const m = Math.floor(remainingSeconds.value / 60)
  const s = remainingSeconds.value % 60
  return m.toString().padStart(2, '0') + ':' + s.toString().padStart(2, '0')
})

const countdownProgressStyle = computed(() => {
  if (initialSeconds.value <= 0) return '0%'
  const percent = Math.max(0, Math.min(100, (remainingSeconds.value / initialSeconds.value) * 100))
  return percent + '%'
})

const countdownBarClass = computed(() => {
  if (remainingSeconds.value <= 60) return 'bg-red-500'
  if (remainingSeconds.value <= 5 * 60) return 'bg-amber-500'
  if (isAlipay.value) return 'bg-[#00AEEF]'
  if (isWxpay.value) return 'bg-[#2BB741]'
  return 'bg-primary-500'
})

const paymentSteps = computed(() => [
  t('payment.qr.stepOpenApp', { provider: providerName.value }),
  t('payment.qr.stepScan'),
  t('payment.qr.stepConfirm'),
])

function isSuccessStatus(status: string | null | undefined): boolean {
  return status === 'COMPLETED' || status === 'PAID' || status === 'RECHARGING'
}

function reopenPopup() {
  if (props.payUrl) {
    const win = window.open(props.payUrl, 'paymentPopup', getPaymentPopupFeatures())
    if (!win || win.closed) {
      window.location.href = props.payUrl
    }
  }
}

function setOutcome(next: PaymentOutcome) {
  if (outcome.value === next) return
  outcome.value = next
  emit('settled', next)
}

async function renderQR() {
  await nextTick()
  if (!qrCanvas.value || !qrUrl.value) return
  await QRCode.toCanvas(qrCanvas.value, qrUrl.value, {
    width: 220, margin: 2,
    errorCorrectionLevel: 'M',
  })
}

async function pollStatus() {
  if (!props.orderId || outcome.value) return
  const order = await paymentStore.pollOrderStatus(props.orderId)
  if (!order) return
  if (isSuccessStatus(order.status)) {
    cleanup()
    paidOrder.value = order
    setOutcome('success')
    emit('success')
  } else if (order.status === 'CANCELLED') {
    cleanup()
    setOutcome('cancelled')
  } else if (order.status === 'EXPIRED' || order.status === 'FAILED') {
    cleanup()
    setOutcome('expired')
  }
}

function startCountdown(seconds: number) {
  const safeSeconds = Math.max(0, seconds)
  initialSeconds.value = safeSeconds || 30 * 60
  remainingSeconds.value = safeSeconds
  if (remainingSeconds.value <= 0) { setOutcome('expired'); return }
  countdownTimer = setInterval(() => {
    remainingSeconds.value--
    if (remainingSeconds.value <= 0) { setOutcome('expired'); cleanup() }
  }, 1000)
}

async function handleCancel() {
  if (!props.orderId || cancelling.value) return
  cancelling.value = true
  try {
    await paymentAPI.cancelOrder(props.orderId)
    cleanup()
    setOutcome('cancelled')
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  } finally {
    cancelling.value = false
  }
}

function handleDone() { cleanup(); emit('done') }

function cleanup() {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
  if (countdownTimer) { clearInterval(countdownTimer); countdownTimer = null }
}

// Initialize on mount
qrUrl.value = props.qrCode
let seconds = 30 * 60
if (props.expiresAt) {
  seconds = Math.floor((new Date(props.expiresAt).getTime() - Date.now()) / 1000)
}
startCountdown(seconds)
pollTimer = setInterval(pollStatus, 3000)
renderQR()

watch(() => qrUrl.value, () => renderQR())
onUnmounted(() => cleanup())
</script>
