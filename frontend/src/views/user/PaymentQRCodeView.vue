<template>
  <AppLayout>
    <div class="mx-auto flex max-w-md flex-col items-center space-y-6 py-8">
      <h2 class="text-xl font-semibold text-gray-900 dark:text-white">
        {{ qrUrl ? scanTitle : t('payment.qr.payInNewWindow') }}
      </h2>
      <div v-if="qrUrl" class="rounded-2xl bg-white p-6 shadow-lg dark:bg-dark-800">
        <canvas ref="qrCanvas" class="mx-auto"></canvas>
      </div>
      <!-- Scan prompt for QR code -->
      <p v-if="qrUrl && !expired && scanHint" class="text-center text-sm text-gray-500 dark:text-gray-400">
        {{ scanHint }}
      </p>
      <div v-if="expired" class="text-center">
        <p class="text-lg font-medium text-red-500">{{ t('payment.qr.expired') }}</p>
        <button class="btn btn-primary mt-4" @click="router.push('/purchase')">{{ t('payment.result.backToRecharge') }}</button>
      </div>
      <div v-else class="text-center">
        <p class="text-sm text-gray-500 dark:text-gray-400">{{ qrUrl ? t('payment.qr.expiresIn') : t('payment.qr.payInNewWindowHint') }}</p>
        <p class="mt-1 text-2xl font-bold tabular-nums text-gray-900 dark:text-white">{{ countdownDisplay }}</p>
        <p class="mt-2 text-sm text-gray-400 dark:text-gray-500">{{ t('payment.qr.waitingPayment') }}</p>
      </div>
      <a v-if="payUrl && !qrUrl && !expired" :href="payUrl" target="_blank" rel="noopener noreferrer"
        class="btn btn-secondary inline-flex w-full items-center justify-center gap-2 py-3">
        {{ t('payment.qr.openPayWindow') }}
      </a>
      <!-- Cancel button -->
      <button v-if="!expired && orderId" class="btn btn-secondary w-full" :disabled="cancelling" @click="handleCancel">
        {{ cancelling ? t('common.processing') : t('payment.qr.cancelOrder') }}
      </button>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import { usePaymentStore } from '@/stores/payment'
import { paymentAPI } from '@/api/payment'
import { extractApiErrorMessage } from '@/utils/apiError'
import { useAppStore } from '@/stores'
import QRCode from 'qrcode'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const paymentStore = usePaymentStore()
const appStore = useAppStore()

const qrCanvas = ref<HTMLCanvasElement | null>(null)
const qrUrl = ref('')
const payUrl = ref('')
const orderId = ref(0)
const remainingSeconds = ref(0)
const expired = ref(false)
const cancelling = ref(false)
const paymentType = ref('')

let pollTimer: ReturnType<typeof setInterval> | null = null
let countdownTimer: ReturnType<typeof setInterval> | null = null

const countdownDisplay = computed(() => {
  const m = Math.floor(remainingSeconds.value / 60)
  const s = remainingSeconds.value % 60
  return m.toString().padStart(2, '0') + ':' + s.toString().padStart(2, '0')
})

const isAlipay = computed(() => paymentType.value.includes('alipay'))
const isWxpay = computed(() => paymentType.value.includes('wxpay'))

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

// Alipay logo SVG as data URI (blue brand mark)
const ALIPAY_LOGO = `data:image/svg+xml,${encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><rect width="64" height="64" rx="12" fill="#1677FF"/><text x="32" y="44" font-family="Arial,sans-serif" font-size="32" font-weight="bold" fill="white" text-anchor="middle">支</text></svg>')}`

// WeChat Pay logo SVG as data URI (green brand mark)
const WXPAY_LOGO = `data:image/svg+xml,${encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><rect width="64" height="64" rx="12" fill="#07C160"/><path d="M20 22c-8 0-14.5 5.4-14.5 12.1 0 3.8 2 7.2 5.1 9.5l-1.3 3.8 4.4-2.3c1.8.6 3.8 1 5.9 1 .5 0 1 0 1.5-.1-.6-1.9-1-3.9-1-6C20.1 33 27 27 35.5 27c.5 0 1 0 1.5.1C35.4 23.4 28.2 22 20 22zm-5 8a2 2 0 1 1 0-4 2 2 0 0 1 0 4zm10 0a2 2 0 1 1 0-4 2 2 0 0 1 0 4z" fill="white"/><path d="M50.5 40c0-5.8-5.5-10.5-12.2-10.5S26 34.2 26 40s5.5 10.5 12.3 10.5c1.5 0 2.8-.2 4.1-.6l3.6 2-1-3.2c2.6-2 4.1-4.8 4.1-7.9zm-16.2-1.5a1.5 1.5 0 1 1 0-3 1.5 1.5 0 0 1 0 3zm8 0a1.5 1.5 0 1 1 0-3 1.5 1.5 0 0 1 0 3z" fill="white"/></svg>')}`

function getLogoForType(): string | null {
  if (isAlipay.value) return ALIPAY_LOGO
  if (isWxpay.value) return WXPAY_LOGO
  return null
}

async function renderQR() {
  await nextTick()
  if (!qrCanvas.value || !qrUrl.value) return

  // Use high error correction to support logo overlay
  const logoSrc = getLogoForType()
  await QRCode.toCanvas(qrCanvas.value, qrUrl.value, {
    width: 256,
    margin: 2,
    errorCorrectionLevel: logoSrc ? 'H' : 'M',
  })

  if (!logoSrc) return

  // Draw logo in center of QR code
  const canvas = qrCanvas.value
  const ctx = canvas.getContext('2d')
  if (!ctx) return

  const img = new Image()
  img.src = logoSrc
  img.onload = () => {
    const logoSize = 48
    const x = (canvas.width - logoSize) / 2
    const y = (canvas.height - logoSize) / 2
    // White background with rounded corners
    const pad = 5
    ctx.fillStyle = '#FFFFFF'
    ctx.beginPath()
    const r = 6
    ctx.moveTo(x - pad + r, y - pad)
    ctx.arcTo(x + logoSize + pad, y - pad, x + logoSize + pad, y + logoSize + pad, r)
    ctx.arcTo(x + logoSize + pad, y + logoSize + pad, x - pad, y + logoSize + pad, r)
    ctx.arcTo(x - pad, y + logoSize + pad, x - pad, y - pad, r)
    ctx.arcTo(x - pad, y - pad, x + logoSize + pad, y - pad, r)
    ctx.fill()
    // Draw logo
    ctx.drawImage(img, x, y, logoSize, logoSize)
  }
}

async function pollStatus() {
  if (!orderId.value) return
  const order = await paymentStore.pollOrderStatus(orderId.value)
  if (!order) return
  if (order.status === 'COMPLETED' || order.status === 'PAID') {
    cleanup()
    router.push({ path: '/payment/result', query: { order_id: String(orderId.value), status: 'success' } })
  } else if (order.status === 'EXPIRED' || order.status === 'CANCELLED' || order.status === 'FAILED') {
    cleanup()
    expired.value = true
  }
}

function startCountdown(seconds: number) {
  remainingSeconds.value = Math.max(0, seconds)
  if (remainingSeconds.value <= 0) {
    expired.value = true
    return
  }
  countdownTimer = setInterval(() => {
    remainingSeconds.value--
    if (remainingSeconds.value <= 0) {
      expired.value = true
      cleanup()
    }
  }, 1000)
}

async function handleCancel() {
  if (!orderId.value || cancelling.value) return
  cancelling.value = true
  try {
    await paymentAPI.cancelOrder(orderId.value)
    cleanup()
    router.push('/purchase')
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    cancelling.value = false
  }
}

function cleanup() {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
  if (countdownTimer) { clearInterval(countdownTimer); countdownTimer = null }
}

watch(qrUrl, () => renderQR())

onMounted(() => {
  orderId.value = Number(route.query.order_id) || 0
  qrUrl.value = (route.query.qr as string) || ''
  payUrl.value = (route.query.pay_url as string) || ''
  paymentType.value = (route.query.payment_type as string) || ''

  // Calculate countdown from expiresAt
  const expiresAtStr = route.query.expires_at as string
  let seconds = 30 * 60 // fallback: 30 minutes
  if (expiresAtStr) {
    const expiresAt = new Date(expiresAtStr)
    const now = new Date()
    seconds = Math.floor((expiresAt.getTime() - now.getTime()) / 1000)
  }
  startCountdown(seconds)
  pollTimer = setInterval(pollStatus, 3000)
  renderQR()
})

onUnmounted(() => cleanup())
</script>
