/**
 * Composable: order resolution + polling for PaymentResultView.
 *
 * Extracted from PaymentResultView.vue to keep the view under 300 lines.
 * Contains order lookup strategies (resume token, order ID, public out_trade_no),
 * recovery snapshot restoration, and periodic status refresh.
 */
import { ref, computed, onBeforeUnmount } from 'vue'
import { useRoute } from 'vue-router'
import {
  PAYMENT_RECOVERY_STORAGE_KEY,
  clearPaymentRecoverySnapshot,
  readPaymentRecoverySnapshot,
} from '../../components/payment/paymentFlow'
import { usePaymentStore } from '../../stores/payment'
import { paymentAPI } from '../../api/payment'
import type { PaymentOrder } from '../../types/payment'
import { normalizePaymentCurrency } from '../../components/payment/currency'

const SUCCESS_STATUSES = new Set(['COMPLETED', 'PAID', 'RECHARGING'])
const PENDING_STATUSES = new Set(['PENDING', 'CREATED', 'WAITING', 'PROCESSING'])
const STATUS_REFRESH_INTERVAL_MS = 2000
const STATUS_REFRESH_MAX_ATTEMPTS = 15

function normalizeOrderStatus(status: string | null | undefined): string {
  return String(status || '').trim().toUpperCase()
}

export function isSuccessStatus(status: string | null | undefined): boolean {
  return SUCCESS_STATUSES.has(normalizeOrderStatus(status))
}

export function isPendingStatus(status: string | null | undefined): boolean {
  return PENDING_STATUSES.has(normalizeOrderStatus(status))
}

export interface ReturnInfo {
  outTradeNo: string
  money: string
  type: string
  tradeStatus: string
}

export function usePaymentResultResolver() {
  const route = useRoute()
  const paymentStore = usePaymentStore()

  const order = ref<PaymentOrder | null>(null)
  const loading = ref(true)
  const currency = ref('CNY')
  const returnInfo = ref<ReturnInfo | null>(null)

  let statusRefreshTimer: ReturnType<typeof setTimeout> | null = null
  const refreshAttempts = ref(0)

  const isSuccess = computed(() => isSuccessStatus(order.value?.status))
  const isPending = computed(() => isPendingStatus(order.value?.status))

  function setResolvedOrder(nextOrder: PaymentOrder | null): void {
    order.value = nextOrder
    if (nextOrder?.currency) {
      currency.value = normalizePaymentCurrency(nextOrder.currency)
    }
  }

  function readRouteQueryString(key: string): string {
    const value = route.query[key]
    if (Array.isArray(value)) return typeof value[0] === 'string' ? value[0] : ''
    return typeof value === 'string' ? value : ''
  }

  function restoreRecoverySnapshot(context: {
    resumeToken: string
    routeOrderId: number
    routeOutTradeNo: string
  }) {
    if (typeof window === 'undefined') return null
    const rawSnapshot = window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)
    if (!rawSnapshot) return null

    if (context.resumeToken) {
      return readPaymentRecoverySnapshot(rawSnapshot, { resumeToken: context.resumeToken })
    }
    if (!context.routeOrderId && !context.routeOutTradeNo) return null

    const restored = readPaymentRecoverySnapshot(rawSnapshot)
    if (!restored) return null
    if (context.routeOrderId > 0 && restored.orderId !== context.routeOrderId) return null
    if (context.routeOutTradeNo && restored.outTradeNo !== context.routeOutTradeNo) return null
    return restored
  }

  async function resolveOrderFromResumeToken(resumeToken: string): Promise<PaymentOrder | null> {
    try {
      const result = await paymentAPI.resolveOrderPublicByResumeToken(resumeToken)
      return result.data
    } catch { return null }
  }

  async function resolveOrderFromOutTradeNo(outTradeNo: string): Promise<PaymentOrder | null> {
    try {
      const result = await paymentAPI.verifyOrderPublic(outTradeNo)
      return result.data
    } catch { return null }
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
    if (!isPendingStatus(status)) clearRecoverySnapshot()
  }

  function scheduleStatusRefresh(refreshOrder: (() => Promise<PaymentOrder | null>) | null): void {
    clearStatusRefreshTimer()
    if (!refreshOrder || !isPending.value || refreshAttempts.value >= STATUS_REFRESH_MAX_ATTEMPTS) return

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

  async function resolveOrder(): Promise<void> {
    const resumeToken = readRouteQueryString('resume_token')
    const routeOrderId = Number(readRouteQueryString('order_id')) || 0
    let outTradeNo = readRouteQueryString('out_trade_no')
    let orderId = 0
    let resumeTokenLookupFailed = false

    const restored = restoreRecoverySnapshot({
      resumeToken, routeOrderId, routeOutTradeNo: outTradeNo,
    })
    if (restored?.orderId) orderId = restored.orderId
    if (restored?.currency) currency.value = normalizePaymentCurrency(restored.currency)
    if (!outTradeNo && restored?.outTradeNo) outTradeNo = restored.outTradeNo

    if (resumeToken) {
      const resolvedOrder = await resolveOrderFromResumeToken(resumeToken)
      if (resolvedOrder) {
        setResolvedOrder(resolvedOrder)
        if (!orderId) orderId = resolvedOrder.id
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
      } catch { /* will try legacy fallback below */ }
    }

    if (!order.value && shouldUsePublicOutTradeNo && (!resumeToken || resumeTokenLookupFailed)) {
      const legacyOrder = await resolveOrderFromOutTradeNo(outTradeNo)
      if (legacyOrder) {
        setResolvedOrder(legacyOrder)
        if (!orderId) orderId = legacyOrder.id
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

    const refreshOrder = async (): Promise<PaymentOrder | null> => {
      if (resumeToken) {
        const resolved = await resolveOrderFromResumeToken(resumeToken)
        if (resolved) return resolved
      }
      if (orderId) {
        try { return await paymentStore.pollOrderStatus(orderId) }
        catch { /* fall through */ }
      }
      if (shouldUsePublicOutTradeNo) return await resolveOrderFromOutTradeNo(outTradeNo)
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
  }

  onBeforeUnmount(() => { clearStatusRefreshTimer() })

  return {
    order,
    loading,
    currency,
    returnInfo,
    isSuccess,
    isPending,
    resolveOrder,
  }
}
