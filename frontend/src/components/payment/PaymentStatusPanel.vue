<template>
  <div class="space-y-4">
    <!-- ═══ Terminal States: receipt with a stamp, user clicks to return ═══ -->

    <!-- Success -->
    <template v-if="outcome === 'success'">
      <div class="stub">
        <div class="flex flex-wrap items-center justify-between gap-3 px-5 py-4">
          <p class="text-h3 font-bold text-accent-strong">{{ props.orderType === 'subscription' ? t('payment.result.subscriptionSuccess') : t('payment.result.success') }}</p>
          <span class="badge badge-success px-2.5 py-1 text-label">
            <Icon name="check" size="sm" />
            {{ t('payment.status.paid') }}
          </span>
        </div>
        <dl v-if="paidOrder" class="divide-y divide-border border-t border-border px-5 text-body">
          <div class="flex justify-between gap-4 py-2">
            <dt class="text-fg-muted">{{ t('payment.orders.orderId') }}</dt>
            <dd class="font-mono text-fg">#{{ paidOrder.id }}</dd>
          </div>
          <div v-if="paidOrder.out_trade_no" class="flex justify-between gap-4 py-2">
            <dt class="text-fg-muted">{{ t('payment.orders.orderNo') }}</dt>
            <dd class="min-w-0 break-all text-right font-mono text-fg">{{ paidOrder.out_trade_no }}</dd>
          </div>
          <div class="flex justify-between gap-4 py-2">
            <dt class="text-fg-muted">{{ t('payment.orders.amount') }}</dt>
            <dd class="tabular-nums text-fg">{{ formatCreditedAmount(paidOrder.amount) }}</dd>
          </div>
          <div class="flex justify-between gap-4 py-2">
            <dt class="text-fg-muted">{{ t('payment.orders.payAmount') }}</dt>
            <dd class="font-bold tabular-nums text-fg">{{ formatGatewayAmount(paidOrder.pay_amount, paidOrder.currency) }}</dd>
          </div>
        </dl>
        <div class="stub-perforation" />
        <div class="px-5 py-4">
          <button class="btn btn-primary w-full" @click="handleDone">{{ t('common.confirm') }}</button>
        </div>
      </div>
    </template>

    <!-- Cancelled -->
    <template v-else-if="outcome === 'cancelled'">
      <div class="stub">
        <div class="space-y-2 px-5 py-5">
          <span class="badge badge-gray px-2.5 py-1 text-label">{{ t('payment.qr.cancelled') }}</span>
          <p class="text-body text-fg-muted">{{ t('payment.qr.cancelledDesc') }}</p>
        </div>
        <div class="stub-perforation" />
        <div class="px-5 py-4">
          <button class="btn btn-primary w-full" @click="handleDone">{{ t('common.confirm') }}</button>
        </div>
      </div>
    </template>

    <!-- Expired / Failed -->
    <template v-else-if="outcome === 'expired'">
      <div class="stub">
        <div class="space-y-2 px-5 py-5">
          <span class="badge badge-warning px-2.5 py-1 text-label">{{ t('payment.qr.expired') }}</span>
          <p class="text-body text-fg-muted">{{ t('payment.qr.expiredDesc') }}</p>
        </div>
        <div class="stub-perforation" />
        <div class="px-5 py-4">
          <button class="btn btn-primary w-full" @click="handleDone">{{ t('common.confirm') }}</button>
        </div>
      </div>
    </template>

    <!-- ═══ Active States: stub = order details / perforation / scan part ═══ -->
    <template v-else>
      <div class="stub">
        <!-- Details part -->
        <div class="px-5 pt-4">
          <p class="text-h3 font-bold text-accent-strong">{{ showQRCode ? scanTitle : t('payment.qr.payInNewWindowHint') }}</p>
          <p v-if="payAmount != null" class="mt-2 text-display font-bold tabular-nums text-fg">
            {{ formatGatewayAmount(payAmount) }}
          </p>
        </div>
        <dl class="mt-3 divide-y divide-border border-t border-border px-5 pb-1 text-body">
          <div v-if="outTradeNo" class="flex justify-between gap-4 py-2">
            <dt class="shrink-0 text-fg-muted">{{ t('payment.orders.orderNo') }}</dt>
            <dd class="min-w-0 break-all text-right font-mono text-fg">{{ outTradeNo }}</dd>
          </div>
          <!-- GPM Pay matches the transfer to this order only through the memo. -->
          <div v-if="transferMemo" class="flex justify-between gap-4 py-2">
            <dt class="shrink-0 text-fg-muted">{{ t('payment.qr.transferMemo') }}</dt>
            <dd class="min-w-0 break-all text-right font-mono font-bold text-fg">{{ transferMemo }}</dd>
          </div>
          <div v-else-if="orderId" class="flex justify-between gap-4 py-2">
            <dt class="text-fg-muted">{{ t('payment.orders.orderId') }}</dt>
            <dd class="font-mono text-fg">#{{ orderId }}</dd>
          </div>
          <div v-if="amount != null" class="flex justify-between gap-4 py-2">
            <dt class="text-fg-muted">{{ t('payment.orders.amount') }}</dt>
            <dd class="tabular-nums text-fg">{{ formatCreditedAmount(amount) }}</dd>
          </div>
        </dl>

        <div class="stub-perforation" />

        <!-- Scan / hand-over part -->
        <div class="flex flex-col items-center gap-4 px-5 py-5">
          <div v-if="showQRCode" class="relative border border-border-strong bg-white p-3">
            <canvas ref="qrCanvas" class="mx-auto"></canvas>
            <!-- Brand logo overlay -->
            <div class="pointer-events-none absolute inset-0 flex items-center justify-center">
              <span class="rounded-sm bg-accent p-1.5 ring-2 ring-white">
                <img :src="qrLogoIcon" alt="" class="h-5 w-5 brightness-0 invert" />
              </span>
            </div>
          </div>
          <div v-else class="spinner h-8 w-8 text-accent"></div>

          <div
            class="flex w-full items-baseline justify-between gap-3 border border-border px-3 py-2"
            :class="remainingSeconds > 0 && remainingSeconds <= 60 ? 'bg-meter-weak text-meter-ink' : ''"
          >
            <span class="text-meta font-medium" :class="remainingSeconds > 0 && remainingSeconds <= 60 ? '' : 'text-fg-muted'">
              {{ showQRCode ? t('payment.qr.expiresIn') : t('payment.qr.waitingPayment') }}
            </span>
            <span class="text-h2 font-bold tabular-nums">{{ countdownDisplay }}</span>
          </div>
          <p v-if="showQRCode" class="text-meta text-fg-subtle">{{ t('payment.qr.waitingPayment') }}</p>

          <div class="flex w-full flex-wrap items-center justify-center gap-2">
            <button v-if="payUrl" class="btn btn-secondary" @click="reopenPopup">
              {{ t('payment.qr.openPayWindow') }}
            </button>
            <button
              data-test="save-payment-qr"
              class="btn btn-secondary"
              @click="saveQRCode"
            >
              <Icon name="download" size="sm" />
              {{ t('payment.qr.saveQRCode') }}
            </button>
          </div>
        </div>
      </div>
      <button class="btn btn-ghost w-full" :disabled="cancelling" @click="handleCancel">
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
import { extractI18nErrorMessage } from '@/utils/apiError'
import { getPaymentPopupFeatures } from '@/components/payment/providerConfig'
import { formatPaymentAmount, normalizePaymentCurrency } from '@/components/payment/currency'
import type { PaymentOrder } from '@/types/payment'
import Icon from '@/components/icons/Icon.vue'
import QRCode from 'qrcode'
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
  /**
   * What the gateway said when it sent the customer back, lowercased.
   * Only a negative verdict is honoured — see settleFromGatewayReturn.
   */
  gatewayReturnStatus?: string
}>()

type PaymentOutcome = 'success' | 'cancelled' | 'expired'

const emit = defineEmits<{ done: []; success: []; settled: [outcome: PaymentOutcome] }>()

const i18n = useI18n()
const { t } = i18n
const paymentStore = usePaymentStore()
const appStore = useAppStore()

const qrCanvas = ref<HTMLCanvasElement | null>(null)
const qrUrl = ref('')
const remainingSeconds = ref(0)
const cancelling = ref(false)
const paidOrder = ref<PaymentOrder | null>(null)
const paymentCurrency = computed(() => normalizePaymentCurrency(props.currency))
// 入账余额始终是 USD，与网关币种无关。
function formatCreditedAmount(value: number): string {
  return formatPaymentAmount(value, 'USD', localeCode.value)
}
const localeCode = computed(() => {
  const raw = i18n.locale as unknown
  if (typeof raw === 'string') return raw
  if (raw && typeof raw === 'object' && 'value' in raw) {
    return String((raw as { value?: string }).value || '')
  }
  return undefined
})

// Terminal outcome: null = still active, 'success' | 'cancelled' | 'expired'
const outcome = ref<PaymentOutcome | null>(null)

let pollTimer: ReturnType<typeof setInterval> | null = null
let countdownTimer: ReturnType<typeof setInterval> | null = null
let verifyAttempts = 0
let lastVerifyAt = 0

const VERIFY_RETRY_INTERVAL_MS = 15000
const VERIFY_RETRY_MAX_ATTEMPTS = 6

const showQRCode = computed(() => !!qrUrl.value)

const qrLogoIcon = computed(() => paymentIcon)

const scanTitle = computed(() => t('payment.qr.scanToPay'))
// Mirrors backend payment.TransferCodeForOutTradeNo.
const transferMemo = computed(() =>
  props.paymentType.startsWith('gpmpay') && props.outTradeNo
    ? props.outTradeNo.replace(/_/g, '').toUpperCase()
    : '',
)

const countdownDisplay = computed(() => {
  const m = Math.floor(remainingSeconds.value / 60)
  const s = remainingSeconds.value % 60
  return m.toString().padStart(2, '0') + ':' + s.toString().padStart(2, '0')
})


function formatGatewayAmount(value: number, currency?: string | null): string {
  return formatPaymentAmount(value, currency || paymentCurrency.value, localeCode.value)
}

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

// 网关回跳只用来提前结束「等待中」，从不用来宣布成功：那必须由后端订单
// 状态说了算，否则用户改一下 URL 就能看到一个假的成功页。
//
// 即便如此也先再查一次订单：网关说失败、而 IPN 已经把订单标成已支付的
// 情况虽然少见，但一旦发生，直接收摊会让用户以为钱丢了。
async function settleFromGatewayReturn(status: string) {
  if (outcome.value) return
  const normalized = status.trim().toLowerCase()
  if (normalized !== 'cancelled' && normalized !== 'failed') return

  await pollStatus()
  if (outcome.value) return

  setOutcome(normalized === 'cancelled' ? 'cancelled' : 'expired')
  cleanup()
}

async function renderQR() {
  await nextTick()
  if (!showQRCode.value || !qrCanvas.value || !qrUrl.value) return
  await QRCode.toCanvas(qrCanvas.value, qrUrl.value, {
    width: 220, margin: 2,
    errorCorrectionLevel: 'M',
  })
}

function saveQRCode() {
  const canvas = qrCanvas.value
  if (!canvas) return
  const link = document.createElement('a')
  link.href = canvas.toDataURL('image/png')
  link.download = `payment-${props.outTradeNo || props.orderId}.png`
  document.body.appendChild(link)
  link.click()
  link.remove()
}

// The gateway can miss a callback, so a still-PENDING order is re-verified
// against the upstream a few times before we give up on this poll cycle.
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
watch(() => props.gatewayReturnStatus, (status) => {
  if (status) void settleFromGatewayReturn(status)
}, { immediate: true })

pollTimer = setInterval(pollStatus, 3000)
renderQR()

watch([() => qrUrl.value, showQRCode], () => renderQR())
onUnmounted(() => cleanup())
</script>
