<template>
  <AppLayout>
    <div class="mx-auto max-w-md space-y-6 py-8">
      <header>
        <h1 class="text-lg font-semibold text-ink">
          {{ qrUrl ? scanTitle : t('payment.qr.payInNewWindow') }}
        </h1>
        <p v-if="qrUrl && !expired && scanHint" class="mt-1 text-sm text-ink-tertiary">
          {{ scanHint }}
        </p>
      </header>

      <div v-if="qrUrl" class="flex justify-center">
        <QrFrame :tone="qrTone" :logo="qrLogoIcon">
          <canvas ref="qrCanvas" class="mx-auto block"></canvas>
        </QrFrame>
      </div>

      <!--
        The expiry used to be a bare `text-lg text-red-500` line with no word
        for the state and no live region, so a screen reader user watching a
        countdown was simply never told it had run out.
      -->
      <div v-if="expired" class="rounded border border-line bg-surface p-5">
        <div aria-live="polite">
          <p class="text-2xs font-medium uppercase tracking-[0.08em] text-warn">
            {{ t('payment.status.expired') }}
          </p>
          <p class="mt-2 text-sm font-medium text-ink">{{ t('payment.qr.expired') }}</p>
        </div>
        <Button
          class="mt-5"
          tone="accent"
          variant="solid"
          size="md"
          @click="router.push('/purchase')"
        >
          {{ t('payment.result.backToRecharge') }}
        </Button>
      </div>

      <template v-else>
        <CountdownPanel
          :label="qrUrl ? t('payment.qr.expiresIn') : t('payment.qr.payInNewWindowHint')"
          :value="countdownDisplay"
          :caption="t('payment.qr.waitingPayment')"
        />

        <Button
          v-if="payUrl && !qrUrl"
          :href="payUrl"
          variant="outline"
          size="md"
          block
          target="_blank"
          rel="noopener noreferrer"
        >
          {{ t('payment.qr.openPayWindow') }}
        </Button>

        <Button
          v-if="orderId"
          variant="outline"
          size="md"
          block
          :loading="cancelling"
          @click="handleCancel"
        >
          {{ t('payment.qr.cancelOrder') }}
        </Button>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
// Direct path, never the `components/common` barrel — it pulls `createI18n`
// into the graph and breaks partial `vue-i18n` factory mocks.
import Button from '@/components/common/Button.vue'
import CountdownPanel from '@/components/payment/CountdownPanel.vue'
import QrFrame from '@/components/payment/QrFrame.vue'
import { usePaymentStore } from '@/stores/payment'
import { paymentAPI } from '@/api/payment'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { useAppStore } from '@/stores'
import QRCode from 'qrcode'
import alipayIcon from '@/assets/icons/alipay.svg'
import wxpayIcon from '@/assets/icons/wxpay.svg'

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

const isAlipay = computed(() => false)
const isWxpay = computed(() => false)

const qrTone = computed<'alipay' | 'wxpay' | ''>(() => {
  if (isAlipay.value) return 'alipay'
  if (isWxpay.value) return 'wxpay'
  return ''
})

/** No mark for an unrecognised provider — an invented logo is worse than none. */
const qrLogoIcon = computed(() => {
  if (isAlipay.value) return alipayIcon
  if (isWxpay.value) return wxpayIcon
  return ''
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

async function renderQR() {
  await nextTick()
  if (!qrCanvas.value || !qrUrl.value) return

  /*
   * The brand mark is a DOM overlay now (see `QrFrame`) rather than being
   * composited into the canvas by hand — 25 lines of `arcTo` drawing a white
   * rounded rectangle that CSS already does. `M` error correction is still
   * required whenever a mark covers the centre modules, or the code stops
   * scanning; without one, `L` keeps the modules larger for the camera.
   */
  await QRCode.toCanvas(qrCanvas.value, qrUrl.value, {
    width: 256,
    margin: 2,
    errorCorrectionLevel: qrLogoIcon.value ? 'M' : 'L',
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
