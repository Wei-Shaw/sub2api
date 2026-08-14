<template>
  <div class="flex min-h-screen items-start justify-center bg-canvas px-4 py-16">
    <div class="w-full max-w-md space-y-6">
      <!-- Loading -->
      <div v-if="loading" class="rounded border border-line bg-surface p-5" aria-live="polite">
        <p class="flex items-center gap-2 text-2xs font-medium uppercase tracking-[0.08em] text-ink-tertiary">
          <span class="spinner h-3 w-3 shrink-0" aria-hidden="true" />
          {{ t('common.processing') }}
        </p>
      </div>

      <template v-else>
        <!--
          THE STATUS IS TEXT.

          This screen used to open with a 80px circle in a pastel tint holding a
          40px tick, spinner or cross — the single most recognisable
          generated-dashboard cliché, and it spent the most prominent element on
          the page on decoration while the tint carried the meaning. A payment
          result is one of the highest-stakes sentences in the product; it should
          read like a statement, not like a sticker.

          The live region matters here specifically: a pending result refreshes
          itself on a timer with no user gesture behind it, so a screen reader
          user would otherwise never learn that the payment went through.
        -->
        <div aria-live="polite" :aria-busy="isPending ? 'true' : undefined">
          <p
            class="flex items-center gap-2 text-2xs font-medium uppercase tracking-[0.08em]"
            :class="statusToneClass"
          >
            <span v-if="isPending" class="spinner h-3 w-3 shrink-0" aria-hidden="true" />
            {{ statusEyebrow }}
          </p>
          <h1 class="mt-2 text-xl font-semibold text-ink">{{ statusTitle }}</h1>
          <p v-if="isPending" class="mt-1 text-sm text-ink-tertiary">
            {{ t('payment.result.processingHint') }}
          </p>
        </div>

        <!-- Order Info -->
        <dl
          v-if="order"
          class="divide-y divide-line-subtle rounded border border-line bg-surface px-4 text-xs"
        >
          <div v-if="hasOrderId(order)" class="flex items-baseline justify-between gap-4 py-2">
            <dt class="shrink-0 text-ink-tertiary">{{ t('payment.orders.orderId') }}</dt>
            <!--
              Order ids and trade numbers get the numeric TYPOGRAPHY — mono,
              tabular, slashed zero — but never `NumCell`, which runs
              `Intl.NumberFormat` and would turn `#1234` into `#1,234` and an
              `out_trade_no` into something with an en dash in it.
            -->
            <dd class="font-mono tabular-nums slashed-zero text-ink">#{{ order.id }}</dd>
          </div>
          <div v-if="order.out_trade_no" class="flex items-baseline justify-between gap-4 py-2">
            <dt class="shrink-0 text-ink-tertiary">{{ t('payment.orders.orderNo') }}</dt>
            <dd class="min-w-0 break-all text-right font-mono text-2xs slashed-zero text-ink">
              {{ order.out_trade_no }}
            </dd>
          </div>
          <div v-if="hasAmountFields(order)" class="flex items-baseline justify-between gap-4 py-2">
            <dt class="shrink-0 text-ink-tertiary">{{ t('payment.orders.baseAmount') }}</dt>
            <dd class="inline-flex items-baseline justify-end gap-0.5">
              <span class="text-2xs text-ink-tertiary">{{ gatewayCurrencySymbol }}</span>
              <NumCell :value="baseAmount" :precision="gatewayPrecision" />
            </dd>
          </div>
          <div
            v-if="hasAmountFields(order) && order.fee_rate > 0"
            class="flex items-baseline justify-between gap-4 py-2"
          >
            <dt class="shrink-0 text-ink-tertiary">
              {{ t('payment.orders.fee') }}
              <span class="font-mono tabular-nums">({{ order.fee_rate }}%)</span>
            </dt>
            <dd class="inline-flex items-baseline justify-end gap-0.5">
              <span class="text-2xs text-ink-tertiary">{{ gatewayCurrencySymbol }}</span>
              <NumCell :value="feeAmount" :precision="gatewayPrecision" />
            </dd>
          </div>
          <div v-if="hasAmountFields(order)" class="flex items-baseline justify-between gap-4 py-2">
            <dt class="shrink-0 font-medium text-ink">{{ t('payment.orders.payAmount') }}</dt>
            <dd class="inline-flex items-baseline justify-end gap-0.5">
              <span class="text-2xs text-ink-tertiary">{{ gatewayCurrencySymbol }}</span>
              <NumCell :value="order.pay_amount" :precision="gatewayPrecision" />
            </dd>
          </div>
          <!--
            The credited amount is USD credit for a balance top-up and the
            gateway currency for a subscription. Those are genuinely different
            units and the symbol has to say which — flattening them is how a
            user comes to believe a ¥88 order credited $88.
          -->
          <div
            v-if="hasAmountFields(order) && order.amount !== order.pay_amount"
            class="flex items-baseline justify-between gap-4 py-2"
          >
            <dt class="shrink-0 text-ink-tertiary">{{ t('payment.orders.creditedAmount') }}</dt>
            <dd class="inline-flex items-baseline justify-end gap-0.5">
              <span class="text-2xs text-ink-tertiary">{{ creditedCurrencySymbol(order) }}</span>
              <NumCell :value="order.amount" :precision="creditedPrecision(order)" />
            </dd>
          </div>
          <div v-if="hasPaymentType(order)" class="flex items-baseline justify-between gap-4 py-2">
            <dt class="shrink-0 text-ink-tertiary">{{ t('payment.orders.paymentMethod') }}</dt>
            <dd class="text-ink">
              {{ t(paymentMethodI18nKey(order.payment_type), normalizedOrderPaymentType(order.payment_type)) }}
            </dd>
          </div>
          <div class="flex items-baseline justify-between gap-4 py-2">
            <dt class="shrink-0 text-ink-tertiary">{{ t('payment.orders.status') }}</dt>
            <dd><OrderStatusBadge :status="displayOrderStatus(order.status)" /></dd>
          </div>
        </dl>

        <!-- EasyPay return info (when no order loaded) -->
        <dl
          v-else-if="returnInfo"
          class="divide-y divide-line-subtle rounded border border-line bg-surface px-4 text-xs"
        >
          <div v-if="returnInfo.outTradeNo" class="flex items-baseline justify-between gap-4 py-2">
            <dt class="shrink-0 text-ink-tertiary">{{ t('payment.orders.orderId') }}</dt>
            <dd class="min-w-0 break-all text-right font-mono text-2xs slashed-zero text-ink">
              {{ returnInfo.outTradeNo }}
            </dd>
          </div>
          <div v-if="returnInfo.money" class="flex items-baseline justify-between gap-4 py-2">
            <dt class="shrink-0 text-ink-tertiary">{{ t('payment.orders.payAmount') }}</dt>
            <dd class="inline-flex items-baseline justify-end gap-0.5">
              <span class="text-2xs text-ink-tertiary">{{ gatewayCurrencySymbol }}</span>
              <!--
                `returnInfo.money` is an untrusted query-string value.
                `returnMoney` resolves to `null` when it is not a finite number,
                and `NumCell` then renders an en dash in disabled ink rather than
                `0.00`: on a payment receipt, "we could not read the amount" and
                "the amount was zero" are not the same claim.
              -->
              <NumCell :value="returnMoney" :precision="gatewayPrecision" />
            </dd>
          </div>
          <div v-if="returnInfo.type" class="flex items-baseline justify-between gap-4 py-2">
            <dt class="shrink-0 text-ink-tertiary">{{ t('payment.orders.paymentMethod') }}</dt>
            <dd class="text-ink">
              {{ t(paymentMethodI18nKey(returnInfo.type), normalizedOrderPaymentType(returnInfo.type)) }}
            </dd>
          </div>
        </dl>

        <!-- Actions -->
        <div class="flex gap-2">
          <Button variant="outline" size="md" class="flex-1" @click="router.push('/purchase')">
            {{ t('payment.result.backToRecharge') }}
          </Button>
          <Button
            tone="accent"
            variant="solid"
            size="md"
            class="flex-1"
            @click="router.push('/orders')"
          >
            {{ t('payment.result.viewOrders') }}
          </Button>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onBeforeUnmount, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import OrderStatusBadge from '@/components/payment/OrderStatusBadge.vue'
// Direct paths, never the `components/common` barrel: it pulls `createI18n`
// into the module graph and breaks the partial `vue-i18n` factory mock this
// view's spec relies on.
import Button from '@/components/common/Button.vue'
import NumCell from '@/components/common/NumCell.vue'
import {
  PAYMENT_RECOVERY_STORAGE_KEY,
  clearPaymentRecoverySnapshot,
  readPaymentRecoverySnapshot,
} from '@/components/payment/paymentFlow'
import { usePaymentStore } from '@/stores/payment'
import { paymentAPI } from '@/api/payment'
import type { PublicOrderVerifyResult } from '@/api/payment'
import type { OrderStatus, PaymentOrder } from '@/types/payment'
import {
  currencySymbol,
  normalizePaymentCurrency,
  paymentCurrencyFractionDigits,
} from '@/components/payment/currency'
import { normalizePaymentMethodForDisplay, paymentMethodI18nKey } from './paymentUx'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const paymentStore = usePaymentStore()

type ResolvedOrder = PaymentOrder | PublicOrderVerifyResult

const order = ref<ResolvedOrder | null>(null)
const loading = ref(true)
const currency = ref('CNY')

interface ReturnInfo {
  outTradeNo: string
  money: string
  type: string
  tradeStatus: string
}
const returnInfo = ref<ReturnInfo | null>(null)

const SUCCESS_STATUSES = new Set(['COMPLETED', 'PAID', 'RECHARGING'])
const PENDING_STATUSES = new Set(['PENDING', 'CREATED', 'WAITING', 'PROCESSING'])

/**
 * The refresh schedule has to outlast what it is waiting for.
 *
 * A card gateway answers in seconds, and this page used to poll for exactly 30
 * of them — but a crypto payment is confirmed by a chain, and a busy network
 * routinely takes half an hour to reach `finished`. Giving up first left the
 * page insisting it would refresh automatically while it had already stopped,
 * which is the one thing a payment screen must never say.
 *
 * So: quick polls while a fast settlement is still plausible, then a slow beat
 * that stays honest for the length of a real confirmation.
 */
const STATUS_REFRESH_FAST_INTERVAL_MS = 2000
const STATUS_REFRESH_FAST_ATTEMPTS = 15
const STATUS_REFRESH_SLOW_INTERVAL_MS = 15000
const STATUS_REFRESH_MAX_ATTEMPTS = 135 // ~30 fast seconds, then ~30 minutes

let statusRefreshTimer: ReturnType<typeof setTimeout> | null = null
const refreshAttempts = ref(0)

function statusRefreshDelayMs(attempts: number): number {
  return attempts < STATUS_REFRESH_FAST_ATTEMPTS
    ? STATUS_REFRESH_FAST_INTERVAL_MS
    : STATUS_REFRESH_SLOW_INTERVAL_MS
}

/** 充值金额 = pay_amount / (1 + fee_rate/100)，fee_rate=0 时等于 pay_amount */
const baseAmount = computed(() => {
  if (!hasAmountFields(order.value)) return 0
  const feeRate = Number(order.value.fee_rate) || 0
  if (feeRate <= 0) return order.value.pay_amount ?? 0
  return Math.round((order.value.pay_amount / (1 + feeRate / 100)) * 100) / 100
})

/** 手续费 = pay_amount - baseAmount */
const feeAmount = computed(() => {
  if (!hasAmountFields(order.value)) return 0
  const feeRate = Number(order.value.fee_rate) || 0
  if (feeRate <= 0) return 0
  return Math.round((order.value.pay_amount - baseAmount.value) * 100) / 100
})

const gatewayCurrencySymbol = computed(() => currencySymbol(currency.value))
const gatewayPrecision = computed(() => paymentCurrencyFractionDigits(currency.value))

/**
 * A balance top-up credits USD, whatever the gateway settled in; a subscription
 * order's `amount` is already in the gateway currency.
 */
function creditedCurrencySymbol(nextOrder: PaymentOrder): string {
  return nextOrder.order_type === 'balance' ? currencySymbol('USD') : gatewayCurrencySymbol.value
}

function creditedPrecision(nextOrder: PaymentOrder): number {
  return nextOrder.order_type === 'balance' ? 2 : gatewayPrecision.value
}

/**
 * `null` when the gateway's `money` query param is absent or unparseable, so a
 * missing measurement renders as an en dash instead of a confident `0.00`.
 */
const returnMoney = computed<number | null>(() => {
  const raw = String(returnInfo.value?.money ?? '').trim()
  if (!raw) return null
  const parsed = Number(raw)
  return Number.isFinite(parsed) ? parsed : null
})

const isSuccess = computed(() => {
  return isSuccessStatus(order.value?.status)
})

const isPending = computed(() => {
  return isPendingStatus(order.value?.status)
})

const statusTitle = computed(() => {
  if (isSuccess.value) {
    return t('payment.result.success')
  }
  if (isPending.value) {
    return t('payment.result.processing')
  }
  return t('payment.result.failed')
})

/**
 * A short state word above the sentence. Pending is `warn`, not the accent:
 * the accent means "you can interact with this" in this system and must never
 * signal state.
 */
const statusEyebrow = computed(() => {
  if (isSuccess.value) return t('payment.status.completed')
  if (isPending.value) return t('payment.status.pending')
  return t('payment.status.failed')
})

const statusToneClass = computed(() => {
  if (isSuccess.value) return 'text-success'
  if (isPending.value) return 'text-warn'
  return 'text-danger'
})

function normalizedOrderPaymentType(paymentType: string): string {
  return normalizePaymentMethodForDisplay(paymentType || '') || paymentType || ''
}

function setResolvedOrder(nextOrder: ResolvedOrder | null): void {
  order.value = nextOrder
  if (nextOrder && 'currency' in nextOrder && nextOrder.currency) {
    currency.value = normalizePaymentCurrency(nextOrder.currency)
  }
}

function hasOrderId(nextOrder: ResolvedOrder | null): nextOrder is PaymentOrder {
  return !!nextOrder && 'id' in nextOrder && typeof nextOrder.id === 'number'
}

function hasAmountFields(nextOrder: ResolvedOrder | null): nextOrder is PaymentOrder {
  return !!nextOrder && 'pay_amount' in nextOrder && typeof nextOrder.pay_amount === 'number' && 'amount' in nextOrder && typeof nextOrder.amount === 'number'
}

function hasPaymentType(nextOrder: ResolvedOrder | null): nextOrder is PaymentOrder {
  return !!nextOrder && 'payment_type' in nextOrder && typeof nextOrder.payment_type === 'string' && nextOrder.payment_type.trim() !== ''
}

function normalizeOrderStatus(status: string | null | undefined): string {
  return String(status || '').trim().toUpperCase()
}

function displayOrderStatus(status: string): OrderStatus {
  return normalizeOrderStatus(status) as OrderStatus
}

function isSuccessStatus(status: string | null | undefined): boolean {
  return SUCCESS_STATUSES.has(normalizeOrderStatus(status))
}

function isPendingStatus(status: string | null | undefined): boolean {
  return PENDING_STATUSES.has(normalizeOrderStatus(status))
}

function readRouteQueryString(key: string): string {
  const value = route.query[key]
  if (Array.isArray(value)) {
    return typeof value[0] === 'string' ? value[0] : ''
  }
  return typeof value === 'string' ? value : ''
}

function restoreRecoverySnapshot(context: {
  resumeToken: string
  routeOrderId: number
  routeOutTradeNo: string
}) {
  if (typeof window === 'undefined') {
    return null
  }

  const rawSnapshot = window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)
  if (!rawSnapshot) {
    return null
  }

  if (context.resumeToken) {
    return readPaymentRecoverySnapshot(rawSnapshot, {
      resumeToken: context.resumeToken,
    })
  }

  if (!context.routeOrderId && !context.routeOutTradeNo) {
    return null
  }

  const restored = readPaymentRecoverySnapshot(rawSnapshot)
  if (!restored) {
    return null
  }

  if (context.routeOrderId > 0 && restored.orderId !== context.routeOrderId) {
    return null
  }

  if (context.routeOutTradeNo && restored.outTradeNo !== context.routeOutTradeNo) {
    return null
  }

  return restored
}

/**
 * The gateway payment id off the return URL.
 *
 * NOWPayments opens a hosted invoice and only creates the payment once the
 * buyer picks a coin, so the identifier we stored at checkout is the invoice's.
 * `NP_id` on the redirect is the first time the payment's own id reaches us,
 * and it is the one the backend can actually poll — see the payment-reference
 * note on `resolveOrderPublicByResumeToken`.
 */
function readPaymentReference(): string {
  return readRouteQueryString('NP_id').trim()
}

async function resolveOrderFromResumeToken(resumeToken: string): Promise<ResolvedOrder | null> {
  try {
    const result = await paymentAPI.resolveOrderPublicByResumeToken(resumeToken, readPaymentReference())
    return result.data
  } catch (_err: unknown) {
    return null
  }
}

async function resolveOrderFromOutTradeNo(outTradeNo: string): Promise<ResolvedOrder | null> {
  try {
    const result = await paymentAPI.verifyOrder(outTradeNo)
    return result.data
  } catch (_err: unknown) {
    try {
      const result = await paymentAPI.verifyOrderPublic(outTradeNo)
      return result.data
    } catch (_innerErr: unknown) {
      return null
    }
  }
}

function clearStatusRefreshTimer(): void {
  if (statusRefreshTimer !== null) {
    clearTimeout(statusRefreshTimer)
    statusRefreshTimer = null
  }
}

function clearRecoverySnapshot(): void {
  if (typeof window === 'undefined') return
  clearPaymentRecoverySnapshot(window.localStorage, PAYMENT_RECOVERY_STORAGE_KEY)
}

function clearRecoverySnapshotForTerminalStatus(status: string | null | undefined): void {
  if (!status) return
  if (!isPendingStatus(status)) {
    clearRecoverySnapshot()
  }
}

function scheduleStatusRefresh(refreshOrder: (() => Promise<ResolvedOrder | null>) | null): void {
  clearStatusRefreshTimer()
  if (!refreshOrder || !isPending.value || refreshAttempts.value >= STATUS_REFRESH_MAX_ATTEMPTS) {
    return
  }

  statusRefreshTimer = setTimeout(async () => {
    refreshAttempts.value += 1
    const refreshedOrder = await refreshOrder()
    if (refreshedOrder) {
      setResolvedOrder(refreshedOrder)
      clearRecoverySnapshotForTerminalStatus(refreshedOrder.status)
    }

    if (isPendingStatus(order.value?.status)) {
      scheduleStatusRefresh(refreshOrder)
    }
  }, statusRefreshDelayMs(refreshAttempts.value))
}

onMounted(async () => {
  const resumeToken = readRouteQueryString('resume_token')
  const routeOrderId = Number(readRouteQueryString('order_id')) || 0
  let outTradeNo = readRouteQueryString('out_trade_no')
  let orderId = 0
  let resumeTokenLookupFailed = false

  const restored = restoreRecoverySnapshot({
    resumeToken,
    routeOrderId,
    routeOutTradeNo: outTradeNo,
  })
  if (restored?.orderId) {
    orderId = restored.orderId
  }
  if (restored?.currency) {
    currency.value = normalizePaymentCurrency(restored.currency)
  }
  if (!outTradeNo && restored?.outTradeNo) {
    outTradeNo = restored.outTradeNo
  }

  if (resumeToken) {
    const resolvedOrder = await resolveOrderFromResumeToken(resumeToken)
    if (resolvedOrder) {
      setResolvedOrder(resolvedOrder)
      if (!orderId) {
        orderId = hasOrderId(resolvedOrder) ? resolvedOrder.id : 0
      }
    } else if (routeOrderId > 0) {
      resumeTokenLookupFailed = true
      orderId = routeOrderId
    } else {
      resumeTokenLookupFailed = true
    }
  } else if (routeOrderId > 0) {
    orderId = routeOrderId
  }

  const hasLegacyFallbackContext = readRouteQueryString('trade_status').trim() !== ''
  const shouldUsePublicOutTradeNo = outTradeNo !== '' && (hasLegacyFallbackContext || routeOrderId > 0 || orderId > 0)

  if (!order.value && orderId && (!resumeToken || routeOrderId > 0)) {
    try {
      setResolvedOrder(await paymentStore.pollOrderStatus(orderId))
    } catch (_err: unknown) {
      // Order lookup failed, will try legacy fallback below when possible.
    }
  }

  if (!order.value && shouldUsePublicOutTradeNo && (!resumeToken || resumeTokenLookupFailed)) {
    const legacyOrder = await resolveOrderFromOutTradeNo(outTradeNo)
    if (legacyOrder) {
      setResolvedOrder(legacyOrder)
      if (!orderId) {
        orderId = hasOrderId(legacyOrder) ? legacyOrder.id : 0
      }
    }
  }

  if (!order.value && !orderId && outTradeNo && hasLegacyFallbackContext) {
    returnInfo.value = {
      outTradeNo,
      money: String(route.query.money || ''),
      type: String(route.query.type || ''),
      tradeStatus: String(route.query.trade_status || ''),
    }
  }

  const refreshOrder = async (): Promise<ResolvedOrder | null> => {
    if (resumeToken) {
      const resolvedOrder = await resolveOrderFromResumeToken(resumeToken)
      if (resolvedOrder) {
        return resolvedOrder
      }
    }

    if (orderId) {
      try {
        return await paymentStore.pollOrderStatus(orderId)
      } catch (_err: unknown) {
        // Fall through to legacy public verification when order polling is unavailable.
      }
    }

    if (shouldUsePublicOutTradeNo) {
      return await resolveOrderFromOutTradeNo(outTradeNo)
    }

    return null
  }

  if (isPendingStatus(order.value?.status)) {
    scheduleStatusRefresh(refreshOrder)
  } else if (order.value) {
    clearRecoverySnapshotForTerminalStatus(order.value.status)
  } else if (returnInfo.value) {
    clearRecoverySnapshot()
  }
  loading.value = false
})

onBeforeUnmount(() => {
  clearStatusRefreshTimer()
})
</script>
