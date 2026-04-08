<template>
  <div class="space-y-4">
    <!-- Success State -->
    <template v-if="success">
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
              <div class="flex justify-between">
                <span class="text-gray-500 dark:text-gray-400">{{ t('payment.orders.amount') }}</span>
                <span class="font-medium text-gray-900 dark:text-white">&yen;{{ paidOrder.pay_amount.toFixed(2) }}</span>
              </div>
            </div>
          </div>
          <button class="btn btn-primary" @click="handleDone">{{ t('common.confirm') }}</button>
        </div>
      </div>
    </template>
    <!-- QR Code Mode -->
    <template v-else-if="qrUrl">
      <div class="card p-6">
        <div class="flex flex-col items-center space-y-4">
          <p class="text-lg font-semibold text-gray-900 dark:text-white">{{ scanTitle }}</p>
          <div class="rounded-2xl bg-white p-4 shadow-sm dark:bg-dark-800">
            <canvas ref="qrCanvas" class="mx-auto"></canvas>
          </div>
          <p v-if="scanHint" class="text-center text-sm text-gray-500 dark:text-gray-400">{{ scanHint }}</p>
        </div>
      </div>
      <div class="card p-4">
        <div v-if="expired" class="text-center">
          <p class="text-lg font-medium text-red-500">{{ t('payment.qr.expired') }}</p>
          <button class="btn btn-primary mt-4" @click="handleDone">{{ t('payment.result.backToRecharge') }}</button>
        </div>
        <div v-else class="text-center">
          <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('payment.qr.expiresIn') }}</p>
          <p class="mt-1 text-2xl font-bold tabular-nums text-gray-900 dark:text-white">{{ countdownDisplay }}</p>
          <p class="mt-1 text-xs text-gray-400 dark:text-gray-500">{{ t('payment.qr.waitingPayment') }}</p>
        </div>
      </div>
      <button v-if="!expired" class="btn btn-secondary w-full" :disabled="cancelling" @click="handleCancel">
        {{ cancelling ? t('common.processing') : t('payment.qr.cancelOrder') }}
      </button>
    </template>
    <!-- Waiting for Popup/Redirect Mode -->
    <template v-else>
      <div class="card p-6">
        <div class="flex flex-col items-center space-y-4 py-4">
          <div v-if="!expired" class="h-10 w-10 animate-spin rounded-full border-4 border-primary-500 border-t-transparent"></div>
          <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('payment.qr.payInNewWindowHint') }}</p>
          <button v-if="payUrl" class="btn btn-secondary text-sm" @click="reopenPopup">
            {{ t('payment.qr.openPayWindow') }}
          </button>
        </div>
      </div>
      <div class="card p-4">
        <div v-if="expired" class="text-center">
          <p class="text-lg font-medium text-red-500">{{ t('payment.qr.expired') }}</p>
          <button class="btn btn-primary mt-4" @click="handleDone">{{ t('payment.result.backToRecharge') }}</button>
        </div>
        <div v-else class="text-center">
          <p class="mt-1 text-2xl font-bold tabular-nums text-gray-900 dark:text-white">{{ countdownDisplay }}</p>
          <p class="mt-1 text-xs text-gray-400 dark:text-gray-500">{{ t('payment.qr.waitingPayment') }}</p>
        </div>
      </div>
      <button v-if="!expired" class="btn btn-secondary w-full" :disabled="cancelling" @click="handleCancel">
        {{ cancelling ? t('common.processing') : t('payment.qr.cancelOrder') }}
      </button>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onUnmounted, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { usePaymentStore } from '@/stores/payment'
import { useAppStore } from '@/stores'
import { paymentAPI } from '@/api/payment'
import { extractApiErrorMessage } from '@/utils/apiError'
import { POPUP_WINDOW_FEATURES } from '@/components/payment/providerConfig'
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

const emit = defineEmits<{ done: []; success: [] }>()

const { t } = useI18n()
const paymentStore = usePaymentStore()
const appStore = useAppStore()

const qrCanvas = ref<HTMLCanvasElement | null>(null)
const qrUrl = ref('')
const remainingSeconds = ref(0)
const expired = ref(false)
const cancelling = ref(false)
const success = ref(false)
const paidOrder = ref<PaymentOrder | null>(null)

let pollTimer: ReturnType<typeof setInterval> | null = null
let countdownTimer: ReturnType<typeof setInterval> | null = null

const isAlipay = computed(() => props.paymentType.includes('alipay'))
const isWxpay = computed(() => props.paymentType.includes('wxpay'))

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

function reopenPopup() {
  if (props.payUrl) {
    window.open(props.payUrl, 'paymentPopup', POPUP_WINDOW_FEATURES)
  }
}

async function renderQR() {
  await nextTick()
  if (!qrCanvas.value || !qrUrl.value) return
  const logoSrc = isAlipay.value ? alipayIcon : isWxpay.value ? wxpayIcon : null
  await QRCode.toCanvas(qrCanvas.value, qrUrl.value, {
    width: 220, margin: 2,
    errorCorrectionLevel: logoSrc ? 'H' : 'M',
  })
  if (!logoSrc) return
  const canvas = qrCanvas.value
  const ctx = canvas.getContext('2d')
  if (!ctx) return
  const img = new Image()
  img.src = logoSrc
  img.onload = () => {
    const logoSize = 40
    const x = (canvas.width - logoSize) / 2
    const y = (canvas.height - logoSize) / 2
    const pad = 4
    ctx.fillStyle = '#FFFFFF'
    ctx.beginPath()
    const r = 5
    ctx.moveTo(x - pad + r, y - pad)
    ctx.arcTo(x + logoSize + pad, y - pad, x + logoSize + pad, y + logoSize + pad, r)
    ctx.arcTo(x + logoSize + pad, y + logoSize + pad, x - pad, y + logoSize + pad, r)
    ctx.arcTo(x - pad, y + logoSize + pad, x - pad, y - pad, r)
    ctx.arcTo(x - pad, y - pad, x + logoSize + pad, y - pad, r)
    ctx.fill()
    ctx.drawImage(img, x, y, logoSize, logoSize)
  }
}

async function pollStatus() {
  if (!props.orderId) return
  const order = await paymentStore.pollOrderStatus(props.orderId)
  if (!order) return
  if (order.status === 'COMPLETED' || order.status === 'PAID') {
    cleanup()
    paidOrder.value = order
    success.value = true
    emit('success')
  } else if (order.status === 'EXPIRED' || order.status === 'CANCELLED' || order.status === 'FAILED') {
    cleanup()
    expired.value = true
  }
}

function startCountdown(seconds: number) {
  remainingSeconds.value = Math.max(0, seconds)
  if (remainingSeconds.value <= 0) { expired.value = true; return }
  countdownTimer = setInterval(() => {
    remainingSeconds.value--
    if (remainingSeconds.value <= 0) { expired.value = true; cleanup() }
  }, 1000)
}

async function handleCancel() {
  if (!props.orderId || cancelling.value) return
  cancelling.value = true
  try {
    await paymentAPI.cancelOrder(props.orderId)
    cleanup()
    emit('done')
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
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
