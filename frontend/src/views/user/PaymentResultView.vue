<template>
  <div class="flex min-h-screen items-center justify-center bg-surface-sunken px-4 py-8">
    <div class="w-full max-w-md space-y-4">
      <!-- Loading -->
      <div v-if="loading" class="flex items-center justify-center py-20">
        <div class="spinner h-8 w-8 text-accent"></div>
      </div>
      <template v-else>
        <div class="stub">
        <!-- Receipt head: title + status stamp -->
        <div class="flex flex-wrap items-start justify-between gap-3 px-5 py-4">
          <div class="min-w-0">
            <h2 class="text-h2 font-bold text-accent-strong">
              {{ statusTitle }}
            </h2>
            <p v-if="isPending" class="mt-1 text-body text-fg-muted">
              {{ statusHint }}
            </p>
          </div>
          <span
            class="badge shrink-0 gap-1.5 px-3 py-1 text-label"
            :class="isSuccess ? 'badge-success' : isPending ? 'badge-warning' : 'badge-danger'"
          >
            <span v-if="isPending" class="spinner h-3 w-3" aria-hidden="true"></span>
            <svg v-else-if="isSuccess" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5" aria-hidden="true">
              <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
            </svg>
            <svg v-else class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5" aria-hidden="true">
              <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
            {{ stampLabel }}
          </span>
        </div>
        <!-- Order Info -->
        <div v-if="order" class="border-t border-border px-5">
          <div class="divide-y divide-border text-body">
            <div v-if="hasOrderId(order)" class="flex items-center justify-between gap-4 py-2">
              <span class="shrink-0 text-fg-muted">{{ t('payment.orders.orderId') }}</span>
              <span class="min-w-0 break-all text-right font-mono text-fg">#{{ order.id }}</span>
            </div>
            <div v-if="order.out_trade_no" class="flex items-center justify-between gap-4 py-2">
              <span class="shrink-0 text-fg-muted">{{ t('payment.orders.orderNo') }}</span>
              <span class="min-w-0 break-all text-right font-mono text-fg">{{ order.out_trade_no }}</span>
            </div>
            <div v-if="hasAmountFields(order)" class="flex items-center justify-between gap-4 py-2">
              <span class="shrink-0 text-fg-muted">{{ t('payment.orders.baseAmount') }}</span>
              <span class="text-right tabular-nums text-fg">{{ formatGatewayAmount(baseAmount) }}</span>
            </div>
            <div v-if="hasAmountFields(order) && order.fee_rate > 0" class="flex items-center justify-between gap-4 py-2">
              <span class="shrink-0 text-fg-muted">{{ t('payment.orders.fee') }} ({{ order.fee_rate }}%)</span>
              <span class="text-right tabular-nums text-fg">{{ formatGatewayAmount(feeAmount) }}</span>
            </div>
            <div v-if="hasAmountFields(order)" class="flex items-center justify-between gap-4 py-2">
              <span class="shrink-0 text-fg-muted">{{ t('payment.orders.payAmount') }}</span>
              <span class="text-h3 font-bold tabular-nums text-fg">{{ formatGatewayAmount(order.pay_amount) }}</span>
            </div>
            <!-- 只有充值单才有"到账"这回事，订阅单买的是订阅本身。amount 一律以
                 USD 记账，用网关币种格式化它会把 60 USD 显示成 60 VND。 -->
            <div v-if="hasAmountFields(order) && order.order_type === 'balance' && order.amount !== order.pay_amount" class="flex items-center justify-between gap-4 py-2">
              <span class="shrink-0 text-fg-muted">{{ t('payment.orders.creditedAmount') }}</span>
              <span class="text-right tabular-nums text-fg">{{ formatPaymentAmount(order.amount, 'USD', localeCode) }}</span>
            </div>
            <div v-if="hasPaymentType(order)" class="flex items-center justify-between gap-4 py-2">
              <span class="shrink-0 text-fg-muted">{{ t('payment.orders.paymentMethod') }}</span>
              <span class="text-right tabular-nums text-fg">{{ t(paymentMethodI18nKey(order.payment_type), normalizedOrderPaymentType(order.payment_type)) }}</span>
            </div>
            <div class="flex items-center justify-between gap-4 py-2">
              <span class="shrink-0 text-fg-muted">{{ t('payment.orders.status') }}</span>
              <OrderStatusBadge :status="displayOrderStatus(order.status)" />
            </div>
          </div>
        </div>
        <!-- EasyPay return info (when no order loaded) -->
        <div v-else-if="returnInfo" class="border-t border-border px-5">
          <div class="divide-y divide-border text-body">
            <div v-if="returnInfo.outTradeNo" class="flex items-center justify-between gap-4 py-2">
              <span class="shrink-0 text-fg-muted">{{ t('payment.orders.orderId') }}</span>
              <span class="min-w-0 break-all text-right font-mono text-fg">{{ returnInfo.outTradeNo }}</span>
            </div>
            <div v-if="returnInfo.money" class="flex items-center justify-between gap-4 py-2">
              <span class="shrink-0 text-fg-muted">{{ t('payment.orders.payAmount') }}</span>
              <span class="text-right tabular-nums text-fg">{{ formatGatewayAmount(Number(returnInfo.money) || 0) }}</span>
            </div>
            <div v-if="returnInfo.type" class="flex items-center justify-between gap-4 py-2">
              <span class="shrink-0 text-fg-muted">{{ t('payment.orders.paymentMethod') }}</span>
              <span class="text-right tabular-nums text-fg">{{ t(paymentMethodI18nKey(returnInfo.type), normalizedOrderPaymentType(returnInfo.type)) }}</span>
            </div>
          </div>
        </div>
        <div class="stub-perforation" />
        <!-- Actions -->
        <!-- Paid: point at the next step (using the service), not back at the top-up page -->
        <div v-if="isSuccess" class="flex gap-3 px-5 py-4">
          <button class="btn btn-secondary flex-1" @click="router.push('/dashboard')">{{ t('home.goToDashboard') }}</button>
          <button class="btn btn-primary flex-1" @click="router.push('/keys')">{{ t('dashboard.createApiKey') }}</button>
        </div>
        <div v-else class="flex gap-3 px-5 py-4">
          <button class="btn btn-secondary flex-1" @click="router.push('/purchase')">{{ t('payment.result.backToRecharge') }}</button>
          <button class="btn btn-primary flex-1" @click="router.push('/orders')">{{ t('payment.result.viewOrders') }}</button>
        </div>
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
import {
  PAYMENT_RECOVERY_STORAGE_KEY,
  clearPaymentRecoverySnapshot,
  readPaymentRecoverySnapshot,
} from '@/components/payment/paymentFlow'
import { usePaymentStore } from '@/stores/payment'
import { useAuthStore } from '@/stores/auth'
import { paymentAPI } from '@/api/payment'
import type { PublicOrderVerifyResult } from '@/api/payment'
import type { OrderStatus, PaymentOrder } from '@/types/payment'
import { formatPaymentAmount, normalizePaymentCurrency } from '@/components/payment/currency'
import { normalizePaymentMethodForDisplay, paymentMethodI18nKey } from './paymentUx'
import { reportResultToOpener } from './paymentPopupBridge'

const i18n = useI18n()
const { t } = i18n
const route = useRoute()
const router = useRouter()
const paymentStore = usePaymentStore()
const authStore = useAuthStore()

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
const STATUS_REFRESH_INTERVAL_MS = 2000
const STATUS_REFRESH_MAX_ATTEMPTS = 15

let statusRefreshTimer: ReturnType<typeof setTimeout> | null = null
let userBalanceRefreshStarted = false
const refreshAttempts = ref(0)

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

const localeCode = computed(() => {
  const raw = i18n.locale as unknown
  if (typeof raw === 'string') return raw
  if (raw && typeof raw === 'object' && 'value' in raw) {
    return String((raw as { value?: string }).value || '')
  }
  return undefined
})

const isSuccess = computed(() => {
  return isSuccessStatus(order.value?.status)
})

const isPending = computed(() => {
  return isPendingStatus(order.value?.status)
})

// 网关回跳时带的 status 只用来挑文案，绝不参与「是否已支付」的判定——
// 那始终以库里的订单状态为准，否则用户手改 URL 就能伪造一个成功页。
const returnedStatus = computed(() =>
  String(route.query.status || '').trim().toLowerCase(),
)

// 网关明说这单没成（用户取消，或支付失败）：订单要到过期或对账后才落终态，
// 这段时间既不该说「处理中」，也不该继续轮询——不会有任何变化，只会白转到上限。
const ABANDONED_RETURN_STATUSES = new Set(['cancelled', 'failed'])
const isAbandonedReturn = computed(
  () => ABANDONED_RETURN_STATUSES.has(returnedStatus.value) && isPending.value,
)

const statusHint = computed(() => {
  if (!isAbandonedReturn.value) return t('payment.result.processingHint')
  return returnedStatus.value === 'cancelled'
    ? t('payment.result.cancelledHint')
    : t('payment.result.failedHint')
})

const statusTitle = computed(() => {
  if (isSuccess.value) {
    return t('payment.result.success')
  }
  if (isPending.value) {
    // 用户在网关点了取消：订单要到过期或对账后才落终态，此时一直显示
    // 「处理中」会让人以为钱还在路上。
    if (isAbandonedReturn.value) {
      return returnedStatus.value === 'cancelled'
        ? t('payment.result.cancelled')
        : t('payment.result.failed')
    }
    return t('payment.result.processing')
  }
  return t('payment.result.failed')
})

// 印章上的状态词（订单行里仍有精确状态）
const stampLabel = computed(() => {
  if (isSuccess.value) return t('payment.status.paid')
  if (isPending.value) {
    if (isAbandonedReturn.value) {
      return t(returnedStatus.value === 'cancelled' ? 'payment.status.cancelled' : 'payment.status.failed')
    }
    return t('payment.status.pending')
  }
  return t('payment.status.failed')
})

function normalizedOrderPaymentType(paymentType: string): string {
  return normalizePaymentMethodForDisplay(paymentType || '') || paymentType || ''
}

function formatGatewayAmount(value: number): string {
  return formatPaymentAmount(value, currency.value, localeCode.value)
}

function setResolvedOrder(nextOrder: ResolvedOrder | null): void {
  order.value = nextOrder
  if (nextOrder && 'currency' in nextOrder && nextOrder.currency) {
    currency.value = normalizePaymentCurrency(nextOrder.currency)
  }
  refreshUserBalanceForSuccessfulOrder(nextOrder)
}

function refreshUserBalanceForSuccessfulOrder(nextOrder: ResolvedOrder | null): void {
  if (!nextOrder || userBalanceRefreshStarted || normalizeOrderStatus(nextOrder.status) !== 'COMPLETED') {
    return
  }
  if ('order_type' in nextOrder && nextOrder.order_type !== 'balance') {
    return
  }

  userBalanceRefreshStarted = true
  void authStore.refreshUser().catch(() => {
    // The order result remains authoritative even if refreshing profile data fails.
  })
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

async function resolveOrderFromResumeToken(resumeToken: string): Promise<ResolvedOrder | null> {
  try {
    const result = await paymentAPI.resolveOrderPublicByResumeToken(resumeToken)
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
  if (!refreshOrder || !isPending.value || isAbandonedReturn.value
    || refreshAttempts.value >= STATUS_REFRESH_MAX_ATTEMPTS) {
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
  }, STATUS_REFRESH_INTERVAL_MS)
}

onMounted(async () => {
  // Opened as the checkout popup: hand the order back to the window the user
  // actually started from and close, instead of stranding the outcome here.
  if (reportResultToOpener(route.query as Record<string, unknown>)) {
    return
  }

  // Not a popup — the checkout ran in this window, so the top-up page it came
  // from is gone. Send the user back there and let the panel that was already
  // tracking this order report the outcome, instead of showing the same result
  // on a page they never chose to open.
  //
  // Only when this browser still holds the snapshot for this order: without it
  // the panel has nothing to restore, and a link opened on another device has
  // to render here.
  // `status` is only ever set by a gateway return URL, so it tells apart "the
  // gateway just sent the user here" from "the user reopened this link", which
  // must keep rendering in place.
  if (readRouteQueryString('status') !== '' && restoreRecoverySnapshot({
    resumeToken: readRouteQueryString('resume_token'),
    routeOrderId: Number(readRouteQueryString('order_id')) || 0,
    routeOutTradeNo: readRouteQueryString('out_trade_no'),
  })) {
    void router.replace({ path: '/purchase', query: route.query })
    return
  }

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
