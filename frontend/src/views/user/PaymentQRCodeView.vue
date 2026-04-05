<template>
  <AppLayout>
    <div class="mx-auto flex max-w-md flex-col items-center space-y-6 py-8">
      <h2 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('payment.qr.scanToPay') }}</h2>
      <div v-if="qrUrl" class="rounded-2xl bg-white p-6 shadow-lg dark:bg-dark-800">
        <canvas ref="qrCanvas" class="mx-auto"></canvas>
      </div>
      <div v-if="expired" class="text-center">
        <p class="text-lg font-medium text-red-500">{{ t('payment.qr.expired') }}</p>
        <button class="btn-primary mt-4" @click="router.push('/purchase')">{{ t('payment.result.backToRecharge') }}</button>
      </div>
      <div v-else class="text-center">
        <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('payment.qr.expiresIn') }}</p>
        <p class="mt-1 text-2xl font-bold tabular-nums text-gray-900 dark:text-white">{{ countdownDisplay }}</p>
        <p class="mt-2 text-sm text-gray-400 dark:text-gray-500">{{ t('payment.qr.waitingPayment') }}</p>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import { usePaymentStore } from '@/stores/payment'
import QRCode from 'qrcode'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const paymentStore = usePaymentStore()

const qrCanvas = ref<HTMLCanvasElement | null>(null)
const qrUrl = ref('')
const orderId = ref(0)
const remainingSeconds = ref(0)
const expired = ref(false)

let pollTimer: ReturnType<typeof setInterval> | null = null
let countdownTimer: ReturnType<typeof setInterval> | null = null

const countdownDisplay = computed(() => {
  const m = Math.floor(remainingSeconds.value / 60)
  const s = remainingSeconds.value % 60
  return m.toString().padStart(2, '0') + ':' + s.toString().padStart(2, '0')
})

async function renderQR() {
  await nextTick()
  if (qrCanvas.value && qrUrl.value) {
    await QRCode.toCanvas(qrCanvas.value, qrUrl.value, { width: 256, margin: 2 })
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
  remainingSeconds.value = seconds
  countdownTimer = setInterval(() => {
    remainingSeconds.value--
    if (remainingSeconds.value <= 0) {
      expired.value = true
      cleanup()
    }
  }, 1000)
}

function cleanup() {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
  if (countdownTimer) { clearInterval(countdownTimer); countdownTimer = null }
}

watch(qrUrl, () => renderQR())

onMounted(() => {
  orderId.value = Number(route.query.order_id) || 0
  qrUrl.value = (route.query.qr as string) || ''
  startCountdown(30 * 60)
  pollTimer = setInterval(pollStatus, 3000)
  renderQR()
})

onUnmounted(() => cleanup())
</script>
