<template>
  <AppLayout>
    <div class="mx-auto flex max-w-md flex-col items-center space-y-6 py-8">
      <h2 class="text-xl font-semibold text-gray-900 dark:text-white">
        {{ qrUrl ? scanTitle : t('payment.qr.payInNewWindow') }}
      </h2>
      <div v-if="qrUrl" class="relative rounded-2xl bg-white p-6 shadow-lg dark:bg-dark-800">
        <canvas ref="qrCanvas" class="mx-auto"></canvas>
        <div class="pointer-events-none absolute inset-0 flex items-center justify-center">
          <span class="rounded-full bg-gray-400 p-2 shadow ring-2 ring-white">
            <img :src="paymentIcon" alt="" class="h-5 w-5 brightness-0 invert" />
          </span>
        </div>
      </div>
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
        class="btn btn-primary w-full py-3">
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
import { extractI18nErrorMessage } from '@/utils/apiError'
import { useAppStore } from '@/stores'
import QRCode from 'qrcode'
import paymentIcon from '@/assets/icons/payment.svg'

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

const scanTitle = computed(() => t('payment.qr.scanToPay'))

async function renderQR() {
  await nextTick()
  if (!qrCanvas.value || !qrUrl.value) return

  // Medium error correction leaves room for the logo overlay the template
  // draws on top of the canvas.
  await QRCode.toCanvas(qrCanvas.value, qrUrl.value, {
    width: 256,
    margin: 2,
    errorCorrectionLevel: 'M',
  })
}

let pollInFlight = false
async function pollStatus() {
  if (!orderId.value) return
  // 防重入：接口响应慢于 3 秒轮询间隔时避免并发重叠请求与重复跳转。
  if (pollInFlight) return
  pollInFlight = true
  try {
    const order = await paymentStore.pollOrderStatus(orderId.value)
    if (!order) return
    // 定时器已被 cleanup 清除时不再执行终态跳转（响应可能在 cleanup 后才回来）。
    if (!pollTimer) return
    if (order.status === 'COMPLETED' || order.status === 'PAID') {
      cleanup()
      router.push({ path: '/payment/result', query: { order_id: String(orderId.value), status: 'success' } })
    } else if (order.status === 'EXPIRED' || order.status === 'CANCELLED' || order.status === 'FAILED') {
      cleanup()
      expired.value = true
    }
  } finally {
    pollInFlight = false
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
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
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
  qrUrl.value = String(route.query.qr || '')
  payUrl.value = String(route.query.pay_url || '')
  paymentType.value = String(route.query.payment_type || '')

  // Calculate countdown from expiresAt
  const expiresAtStr = String(route.query.expires_at || '')
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
