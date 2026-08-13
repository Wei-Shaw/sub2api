<template>
  <div class="space-y-4">
    <!--
      ═══ Terminal states ═══

      One block for all three outcomes instead of three near-identical cards.
      What every one of them used to lead with: an 64px circle in a pastel tint
      holding a 32px glyph — green tick, gray cross, orange clock. That is the
      most recognisable generated-dashboard cliché there is, it spent the most
      prominent element on the card on decoration, and the tint carried the
      meaning while the words underneath repeated it.

      Now the STATE IS TEXT: a small uppercase word in the semantic tone, then
      the sentence in ink, then the order it refers to. The tone is the redundant
      channel, not the message — which is also what makes this legible in a
      screenshot and to a reader who cannot separate the hues.
    -->
    <template v-if="outcome">
      <section class="rounded border border-line bg-surface p-5">
        <!--
          The live region wraps the header only. A payment settles with no user
          gesture behind it — the poll resolves and the screen changes — so this
          has to be announced; but the countdown below ticks once a second and
          announcing that would drown everything else out.
        -->
        <div aria-live="polite">
          <p
            class="text-2xs font-medium uppercase tracking-[0.08em]"
            :class="OUTCOME_TONE_CLASS[outcome]"
          >
            {{ outcomeEyebrow }}
          </p>
          <h2 class="mt-2 text-lg font-semibold text-ink">{{ outcomeTitle }}</h2>
          <p v-if="outcomeDescription" class="mt-1 text-sm text-ink-tertiary">
            {{ outcomeDescription }}
          </p>
        </div>

        <dl
          v-if="paidOrder"
          class="mt-5 divide-y divide-line-subtle border-y border-line-subtle text-xs"
        >
          <div class="flex items-baseline justify-between gap-4 py-1.5">
            <dt class="shrink-0 text-ink-tertiary">{{ t('payment.orders.orderId') }}</dt>
            <!--
              An order id is an IDENTIFIER, not a quantity. Routing it through
              `NumCell` would run `Intl.NumberFormat` over it and print `#1,234`,
              so it gets the numeric TYPOGRAPHY without the numeric formatting.
            -->
            <dd class="font-mono tabular-nums slashed-zero text-ink">#{{ paidOrder.id }}</dd>
          </div>
          <div v-if="paidOrder.out_trade_no" class="flex items-baseline justify-between gap-4 py-1.5">
            <dt class="shrink-0 text-ink-tertiary">{{ t('payment.orders.orderNo') }}</dt>
            <dd class="min-w-0 break-all text-right font-mono text-2xs slashed-zero text-ink">
              {{ paidOrder.out_trade_no }}
            </dd>
          </div>
          <!--
            Two currencies on one panel, and they must stay distinguishable: the
            credited balance is always USD credit, the paid amount is whatever
            the gateway settled in. Collapsing them onto one symbol is how a
            user comes to believe they were charged 88 dollars for a ¥88 order.
          -->
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
              <span class="text-2xs text-ink-tertiary">{{ paidOrderCurrencySymbol }}</span>
              <NumCell :value="paidOrder.pay_amount" :precision="paidOrderPrecision" />
            </dd>
          </div>
        </dl>

        <Button
          class="mt-5"
          tone="accent"
          variant="solid"
          size="md"
          data-testid="payment-outcome-confirm"
          @click="handleDone"
        >
          {{ t('common.confirm') }}
        </Button>
      </section>
    </template>

    <!-- ═══ QR code ═══ -->
    <template v-else-if="showQRCode">
      <section class="rounded border border-line bg-surface p-5">
        <h2 class="text-sm font-medium text-ink">{{ scanTitle }}</h2>
        <p v-if="scanHint" class="mt-1 text-xs text-ink-tertiary">{{ scanHint }}</p>
        <div class="mt-5 flex justify-center">
          <QrFrame :tone="qrTone" :logo="qrLogoIcon">
            <canvas ref="qrCanvas" class="mx-auto block"></canvas>
          </QrFrame>
        </div>
        <div v-if="payUrl" class="mt-5 flex justify-center">
          <Button
            variant="outline"
            size="md"
            data-testid="reopen-payment-window"
            @click="reopenPopup"
          >
            {{ t('payment.qr.openPayWindow') }}
          </Button>
        </div>
      </section>

      <CountdownPanel
        :label="t('payment.qr.expiresIn')"
        :value="countdownDisplay"
        :caption="t('payment.qr.waitingPayment')"
      />

      <Button variant="outline" size="md" block :loading="cancelling" @click="handleCancel">
        {{ t('payment.qr.cancelOrder') }}
      </Button>
    </template>

    <!-- ═══ Waiting on a popup / redirect ═══ -->
    <template v-else>
      <section class="rounded border border-line bg-surface p-5">
        <div aria-live="polite">
          <p class="flex items-center gap-2 text-2xs font-medium uppercase tracking-[0.08em] text-ink-tertiary">
            <span class="spinner h-3 w-3 shrink-0" aria-hidden="true" />
            {{ t('common.processing') }}
          </p>
          <p class="mt-2 text-sm text-ink-secondary">{{ t('payment.qr.payInNewWindowHint') }}</p>
        </div>
        <div v-if="payUrl" class="mt-5">
          <Button
            variant="outline"
            size="md"
            data-testid="reopen-payment-window"
            @click="reopenPopup"
          >
            {{ t('payment.qr.openPayWindow') }}
          </Button>
        </div>
      </section>

      <CountdownPanel :value="countdownDisplay" :caption="t('payment.qr.waitingPayment')" />

      <Button variant="outline" size="md" block :loading="cancelling" @click="handleCancel">
        {{ t('payment.qr.cancelOrder') }}
      </Button>
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
import {
  currencySymbol,
  normalizePaymentCurrency,
  paymentCurrencyFractionDigits,
} from '@/components/payment/currency'
import type { PaymentOrder } from '@/types/payment'
// Imported by direct path, never through `components/common/index.ts`: the
// barrel pulls `createI18n` into the module graph and breaks every spec that
// mocks `vue-i18n` with a partial factory.
import Button from '@/components/common/Button.vue'
import NumCell from '@/components/common/NumCell.vue'
import CountdownPanel from '@/components/payment/CountdownPanel.vue'
import QrFrame from '@/components/payment/QrFrame.vue'
import QRCode from 'qrcode'
import alipayIcon from '@/assets/icons/alipay.svg'
import wxpayIcon from '@/assets/icons/wxpay.svg'
import paymentIcon from '@/assets/icons/payment.svg'
const props = defineProps<{
  orderId: number
  amount?: number
  payAmount?: number
  qrCode: string
  expiresAt: string
  paymentType: string
  payUrl?: string
  orderType?: string
  currency?: string
  outTradeNo?: string
}>()

type PaymentOutcome = 'success' | 'cancelled' | 'expired'

const emit = defineEmits<{ done: []; success: []; settled: [outcome: PaymentOutcome] }>()

const { t } = useI18n()
const paymentStore = usePaymentStore()
const appStore = useAppStore()

const qrCanvas = ref<HTMLCanvasElement | null>(null)
const qrUrl = ref('')
const remainingSeconds = ref(0)
const cancelling = ref(false)
const paidOrder = ref<PaymentOrder | null>(null)
const paymentCurrency = computed(() => normalizePaymentCurrency(props.currency))
const creditedAmountSymbol = currencySymbol('USD')
const paidOrderCurrencySymbol = computed(() =>
  currencySymbol(paidOrder.value?.currency || paymentCurrency.value)
)
const paidOrderPrecision = computed(() =>
  paymentCurrencyFractionDigits(paidOrder.value?.currency || paymentCurrency.value)
)

// Terminal outcome: null = still active, 'success' | 'cancelled' | 'expired'
const outcome = ref<PaymentOutcome | null>(null)

/*
 * Neither in-flight nor cancelled state claims the accent: the accent means
 * "you can interact with this" and nothing else in this system. `cancelled` is
 * not a failure either — the user asked for it — so it stays neutral, and only
 * an expiry the user did not choose earns a semantic colour.
 */
const OUTCOME_TONE_CLASS: Record<PaymentOutcome, string> = {
  success: 'text-success',
  cancelled: 'text-ink-tertiary',
  expired: 'text-warn',
}

let pollTimer: ReturnType<typeof setInterval> | null = null
let countdownTimer: ReturnType<typeof setInterval> | null = null
let verifyAttempts = 0
let lastVerifyAt = 0

const VERIFY_RETRY_INTERVAL_MS = 15000
const VERIFY_RETRY_MAX_ATTEMPTS = 6

const isAlipay = computed(() => false)
const isWxpay = computed(() => false)
const showQRCode = computed(() => !!qrUrl.value)

/** `''` means "no provider brand" — QrFrame then falls back to a hairline. */
const qrTone = computed<'alipay' | 'wxpay' | ''>(() => {
  if (isAlipay.value) return 'alipay'
  if (isWxpay.value) return 'wxpay'
  return ''
})

const qrLogoIcon = computed(() => {
  if (isAlipay.value) return alipayIcon
  if (isWxpay.value) return wxpayIcon
  return paymentIcon
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

const outcomeEyebrow = computed(() => {
  if (outcome.value === 'success') return t('payment.status.completed')
  if (outcome.value === 'cancelled') return t('payment.status.cancelled')
  return t('payment.status.expired')
})

const outcomeTitle = computed(() => {
  if (outcome.value === 'success') {
    return props.orderType === 'subscription'
      ? t('payment.result.subscriptionSuccess')
      : t('payment.result.success')
  }
  if (outcome.value === 'cancelled') return t('payment.qr.cancelled')
  return t('payment.qr.expired')
})

const outcomeDescription = computed(() => {
  if (outcome.value === 'cancelled') return t('payment.qr.cancelledDesc')
  if (outcome.value === 'expired') return t('payment.qr.expiredDesc')
  return ''
})

const countdownDisplay = computed(() => {
  const m = Math.floor(remainingSeconds.value / 60)
  const s = remainingSeconds.value % 60
  return m.toString().padStart(2, '0') + ':' + s.toString().padStart(2, '0')
})


function isSuccessStatus(status: string | null | undefined): boolean {
  return status === 'COMPLETED' || status === 'PAID' || status === 'RECHARGING'
}

function reopenPopup() {
  if (props.payUrl) {
    const win = window.open(props.payUrl, '_blank', 'noopener,noreferrer')
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
  if (!showQRCode.value || !qrCanvas.value || !qrUrl.value) return
  await QRCode.toCanvas(qrCanvas.value, qrUrl.value, {
    width: 220, margin: 2,
    errorCorrectionLevel: 'M',
  })
}


// A SePay order is only ever confirmed by its webhook, so a browser returning
// from the banking app can legitimately be ahead of our own state. Asking the
// server to re-check upstream is the only way to close that gap.
async function tryRecoverPendingOrder(order: PaymentOrder): Promise<PaymentOrder> {
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

let pollInFlight = false
async function pollStatus() {
  if (!props.orderId || outcome.value) return
  // 防重入：接口（含 verifyOrder 二次确认）响应慢于 3 秒轮询间隔时避免并发重叠请求。
  if (pollInFlight) return
  pollInFlight = true
  try {
    let order = await paymentStore.pollOrderStatus(props.orderId)
    if (!order) return
    // 已进入终态则不再处理迟到的响应。
    if (outcome.value) return
    order = await tryRecoverPendingOrder(order)
    if (outcome.value) return
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
  } finally {
    pollInFlight = false
  }
}

function startCountdown(seconds: number) {
  remainingSeconds.value = Math.max(0, seconds)
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
verifyAttempts = 0
lastVerifyAt = 0
let seconds = 30 * 60
if (props.expiresAt) {
  seconds = Math.floor((new Date(props.expiresAt).getTime() - Date.now()) / 1000)
}
startCountdown(seconds)
pollTimer = setInterval(pollStatus, 3000)
renderQR()

watch([() => qrUrl.value, showQRCode], () => renderQR())
onUnmounted(() => cleanup())
</script>
