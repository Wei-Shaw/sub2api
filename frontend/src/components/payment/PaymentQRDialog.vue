<template>
  <BaseDialog :show="show" :title="dialogTitle" width="narrow" @close="handleClose">
    <!-- QR Code + Polling State -->
    <div v-if="!success" class="space-y-4">
      <!-- QR Code mode -->
      <template v-if="qrUrl">
        <div class="flex justify-center">
          <QrFrame :tone="qrTone" :logo="qrLogoIcon">
            <canvas ref="qrCanvas" class="mx-auto block"></canvas>
          </QrFrame>
        </div>
        <p v-if="scanHint" class="text-center text-xs text-ink-tertiary">
          {{ scanHint }}
        </p>
      </template>

      <!-- Popup window waiting mode (no QR code) -->
      <template v-else>
        <!--
          Status is text in the semantic tone, with the spinner as the redundant
          channel. The live region sits here rather than around the clock below,
          which ticks once a second and would otherwise be read out forever.
        -->
        <div aria-live="polite">
          <p class="flex items-center gap-2 text-2xs font-medium uppercase tracking-[0.08em] text-ink-tertiary">
            <span class="spinner h-3 w-3 shrink-0" aria-hidden="true" />
            {{ t('common.processing') }}
          </p>
          <p class="mt-2 text-sm text-ink-secondary">{{ t('payment.qr.payInNewWindowHint') }}</p>
        </div>
        <Button
          v-if="payUrl"
          variant="outline"
          size="md"
          data-testid="reopen-payment-window"
          @click="reopenPopup"
        >
          {{ t('payment.qr.openPayWindow') }}
        </Button>
      </template>

      <!-- Countdown -->
      <div v-if="expired" aria-live="polite">
        <p class="text-2xs font-medium uppercase tracking-[0.08em] text-warn">
          {{ t('payment.status.expired') }}
        </p>
        <p class="mt-2 text-sm font-medium text-ink">{{ t('payment.qr.expired') }}</p>
      </div>
      <CountdownPanel
        v-else
        :label="qrUrl ? t('payment.qr.expiresIn') : ''"
        :value="countdownDisplay"
        :caption="t('payment.qr.waitingPayment')"
      />
    </div>

    <!-- Success State -->
    <div v-else class="space-y-4">
      <div aria-live="polite">
        <p class="text-2xs font-medium uppercase tracking-[0.08em] text-success">
          {{ t('payment.status.completed') }}
        </p>
        <p class="mt-2 text-lg font-semibold text-ink">{{ t('payment.result.success') }}</p>
      </div>
      <dl
        v-if="paidOrder"
        class="divide-y divide-line-subtle border-y border-line-subtle text-xs"
      >
        <div class="flex items-baseline justify-between gap-4 py-1.5">
          <dt class="shrink-0 text-ink-tertiary">{{ t('payment.orders.orderId') }}</dt>
          <!-- An id, not a quantity: no `Intl.NumberFormat`, or `#1234` → `#1,234`. -->
          <dd class="font-mono tabular-nums slashed-zero text-ink">#{{ paidOrder.id }}</dd>
        </div>
        <!-- Credited balance is USD credit; paid amount is the gateway currency. -->
        <div class="flex items-baseline justify-between gap-4 py-1.5">
          <dt class="shrink-0 text-ink-tertiary">{{ t('payment.orders.amount') }}</dt>
          <dd class="inline-flex items-baseline justify-end gap-0.5">
            <span class="text-2xs text-ink-tertiary">{{ creditedAmountSymbol }}</span>
            <NumCell :value="paidOrder.amount" :precision="2" />
          </dd>
        </div>
        <div class="flex items-baseline justify-between gap-4 py-1.5">
          <dt class="shrink-0 text-ink-tertiary">{{ t('payment.orders.payAmount') }}</dt>
          <dd class="inline-flex items-baseline justify-end gap-0.5">
            <span class="text-2xs text-ink-tertiary">{{ paymentAmountSymbol(paidOrder) }}</span>
            <NumCell :value="paidOrder.pay_amount" :precision="paidOrderPrecision" />
          </dd>
        </div>
      </dl>
    </div>

    <template #footer>
      <div class="flex justify-end gap-2">
        <!--
          `loading` keeps the label box and overlays a spinner instead of
          swapping the text for "processing…", which changed the button's width
          at the exact moment the user was clicking it.
        -->
        <Button
          v-if="!success && !expired"
          variant="outline"
          size="md"
          :loading="cancelling"
          @click="handleCancel"
        >
          {{ t('payment.qr.cancelOrder') }}
        </Button>
        <Button v-if="success" tone="accent" variant="solid" size="md" @click="handleDone">
          {{ t('common.confirm') }}
        </Button>
        <Button v-if="expired" tone="accent" variant="solid" size="md" @click="handleClose">
          {{ t('payment.result.backToRecharge') }}
        </Button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, computed, watch, onUnmounted, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
// Direct paths, never the `components/common` barrel: it drags `createI18n`
// into the graph and breaks specs that mock `vue-i18n` with a partial factory.
import Button from '@/components/common/Button.vue'
import NumCell from '@/components/common/NumCell.vue'
import CountdownPanel from '@/components/payment/CountdownPanel.vue'
import QrFrame from '@/components/payment/QrFrame.vue'
import { usePaymentStore } from '@/stores/payment'
import { useAppStore } from '@/stores'
import { paymentAPI } from '@/api/payment'
import { extractI18nErrorMessage } from '@/utils/apiError'
import type { PaymentOrder } from '@/types/payment'
import { currencySymbol, paymentCurrencyFractionDigits } from '@/components/payment/currency'
import QRCode from 'qrcode'
import alipayIcon from '@/assets/icons/alipay.svg'
import wxpayIcon from '@/assets/icons/wxpay.svg'

const props = defineProps<{
  show: boolean
  orderId: number
  qrCode: string
  expiresAt: string
  paymentType: string
  /** URL for reopening the payment popup window */
  payUrl?: string
}>()

const emit = defineEmits<{
  close: []
  success: []
}>()

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
const creditedAmountSymbol = currencySymbol('USD')

let pollTimer: ReturnType<typeof setInterval> | null = null
let countdownTimer: ReturnType<typeof setInterval> | null = null
let verifyAttempts = 0
let lastVerifyAt = 0

const VERIFY_RETRY_INTERVAL_MS = 15000
const VERIFY_RETRY_MAX_ATTEMPTS = 6

const isAlipay = computed(() => false)
const isWxpay = computed(() => false)

const dialogTitle = computed(() => {
  if (success.value) return t('payment.result.success')
  if (!qrUrl.value) return t('payment.qr.payInNewWindow')
  if (isAlipay.value) return t('payment.qr.scanAlipay')
  if (isWxpay.value) return t('payment.qr.scanWxpay')
  return t('payment.qr.scanToPay')
})

const scanHint = computed(() => {
  if (isAlipay.value) return t('payment.qr.scanAlipayHint')
  if (isWxpay.value) return t('payment.qr.scanWxpayHint')
  return ''
})

const qrTone = computed<'alipay' | 'wxpay' | ''>(() => {
  if (isAlipay.value) return 'alipay'
  if (isWxpay.value) return 'wxpay'
  return ''
})

/** No mark for an unknown provider — an invented logo is worse than none. */
const qrLogoIcon = computed(() => {
  if (isAlipay.value) return alipayIcon
  if (isWxpay.value) return wxpayIcon
  return ''
})

function paymentAmountSymbol(order: PaymentOrder): string {
  return currencySymbol(order.currency)
}

const paidOrderPrecision = computed(() =>
  paymentCurrencyFractionDigits(paidOrder.value?.currency)
)

const countdownDisplay = computed(() => {
  const m = Math.floor(remainingSeconds.value / 60)
  const s = remainingSeconds.value % 60
  return m.toString().padStart(2, '0') + ':' + s.toString().padStart(2, '0')
})

function reopenPopup() {
  if (props.payUrl) {
    window.open(props.payUrl, '_blank', 'noopener,noreferrer')
  }
}

async function renderQR() {
  await nextTick()
  if (!qrCanvas.value || !qrUrl.value) return
  /*
   * `M` whenever a mark is overlaid: the centre modules it covers have to be
   * recoverable from the error-correction data, or the code stops scanning.
   * Without a mark, `L` keeps the modules larger and easier for a camera.
   */
  await QRCode.toCanvas(qrCanvas.value, qrUrl.value, {
    width: 220,
    margin: 2,
    errorCorrectionLevel: qrLogoIcon.value ? 'M' : 'L',
  })
}

async function pollStatus() {
  if (!props.orderId) return
  let order = await paymentStore.pollOrderStatus(props.orderId)
  if (!order) return
  order = await tryRecoverPendingOrder(order)
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

async function tryRecoverPendingOrder(order: PaymentOrder): Promise<PaymentOrder> {
  if (!isWxpay.value) return order
  const outTradeNo = String(order.out_trade_no || '').trim()
  if (!outTradeNo) return order
  const normalizedStatus = String(order.status || '').trim().toUpperCase()
  if (normalizedStatus !== 'PENDING') return order
  const now = Date.now()
  if (verifyAttempts >= VERIFY_RETRY_MAX_ATTEMPTS || now - lastVerifyAt < VERIFY_RETRY_INTERVAL_MS) {
    return order
  }

  lastVerifyAt = now
  verifyAttempts += 1
  try {
    const result = await paymentAPI.verifyOrder(outTradeNo)
    return result.data ?? order
  } catch {
    return order
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
  if (!props.orderId || cancelling.value) return
  cancelling.value = true
  try {
    await paymentAPI.cancelOrder(props.orderId)
    cleanup()
    emit('close')
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  } finally {
    cancelling.value = false
  }
}

function handleClose() {
  cleanup()
  emit('close')
}

function handleDone() {
  cleanup()
  emit('close')
}

function cleanup() {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
  if (countdownTimer) { clearInterval(countdownTimer); countdownTimer = null }
}

function init() {
  // Reset state
  success.value = false
  paidOrder.value = null
  expired.value = false
  cancelling.value = false
  qrUrl.value = props.qrCode
  verifyAttempts = 0
  lastVerifyAt = 0

  let seconds = 30 * 60
  if (props.expiresAt) {
    const expiresAt = new Date(props.expiresAt)
    seconds = Math.floor((expiresAt.getTime() - Date.now()) / 1000)
  }
  startCountdown(seconds)
  pollTimer = setInterval(pollStatus, 3000)
  renderQR()
}

// Watch for dialog open/close
watch(() => props.show, (isOpen) => {
  if (isOpen) {
    init()
  } else {
    cleanup()
  }
})

watch(qrUrl, () => renderQR())

onUnmounted(() => cleanup())
</script>
