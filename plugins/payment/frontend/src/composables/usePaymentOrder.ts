/**
 * Composable: order creation, wechat JSBridge, payment recovery, error handling.
 *
 * Extracted from PaymentView.vue -- encapsulates the complex createOrder flow,
 * wechat OAuth/JSAPI bridging, mobile QR fallback, and localStorage recovery.
 */
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { usePaymentStore } from '../stores/payment'
import { useAppStore } from '../stores/host'
import { extractApiErrorMessage, extractI18nErrorMessage } from '@sub2api/plugin-sdk'
import { isMobileDevice } from '../utils/device'
import {
  PAYMENT_RECOVERY_STORAGE_KEY,
  buildCreateOrderPayload,
  clearPaymentRecoverySnapshot,
  decidePaymentLaunch,
  normalizeVisibleMethod,
  readPaymentRecoverySnapshot,
  writePaymentRecoverySnapshot,
  type PaymentRecoverySnapshot,
} from '../components/payment/paymentFlow'
import { getPaymentPopupFeatures } from '../components/payment/providerConfig'
import { buildPaymentErrorToastMessage, describePaymentScenarioError } from '../views/user/paymentUx'
import type { CreateOrderResult, OrderType, SubscriptionPlan } from '../types/payment'
import { invokeWechatJsapiPayment, buildWechatOAuthAuthorizeUrl } from './paymentWechatBridge'
import { shouldFallbackToDesktopQr } from './paymentMobileFallback'

export interface CreateOrderOptions {
  openid?: string
  wechatResumeToken?: string
  paymentType?: string
  isResume?: boolean
  mobileQrFallbackAttempted?: boolean
}

interface MobileQrFallbackContext {
  orderAmount: string
  orderType: OrderType
  planId?: number
  paymentType: string
  attempted: boolean
}

function emptyPaymentState(): PaymentRecoverySnapshot {
  return {
    orderId: 0, amount: '0', qrCode: '', expiresAt: '', paymentType: '',
    payUrl: '', outTradeNo: '', clientSecret: '', intentId: '', currency: '',
    countryCode: '', paymentEnv: '', payAmount: '0', orderType: '',
    paymentMode: '', resumeToken: '', createdAt: 0,
  }
}
// -- Composable --

export function usePaymentOrder(deps: {
  selectedMethod: () => string
  selectedPlan: () => SubscriptionPlan | null
  selectedCurrency: () => string
}) {
  const { t } = useI18n()
  const router = useRouter()
  const paymentStore = usePaymentStore()
  const appStore = useAppStore()

  const submitting = ref(false)
  const paymentPhase = ref<'select' | 'paying'>('select')
  const paymentState = ref<PaymentRecoverySnapshot>(emptyPaymentState())

  function persistRecoverySnapshot(snapshot: PaymentRecoverySnapshot) {
    if (typeof window === 'undefined' || !snapshot.orderId) return
    writePaymentRecoverySnapshot(window.localStorage, snapshot, PAYMENT_RECOVERY_STORAGE_KEY)
  }
  function removeRecoverySnapshot() {
    if (typeof window === 'undefined') return
    clearPaymentRecoverySnapshot(window.localStorage, PAYMENT_RECOVERY_STORAGE_KEY)
  }
  function resetPayment() {
    paymentPhase.value = 'select'
    paymentState.value = emptyPaymentState()
    removeRecoverySnapshot()
  }

  async function redirectToPaymentResult(state: PaymentRecoverySnapshot): Promise<void> {
    const query: Record<string, string | undefined> = {}
    if (state.orderId > 0) query.order_id = String(state.orderId)
    if (state.outTradeNo) query.out_trade_no = state.outTradeNo
    if (state.resumeToken) query.resume_token = state.resumeToken
    await router.push({ path: '/payment/result', query })
  }

  function applyScenarioError(err: unknown, paymentMethod: string): boolean {
    const descriptor = describePaymentScenarioError(err, {
      paymentMethod,
      isMobile: isMobileDevice(),
      isWechatBrowser: typeof window !== 'undefined' && /MicroMessenger/i.test(window.navigator.userAgent),
    })
    if (!descriptor) return false
    const msg = t(descriptor.messageKey)
    const hint = descriptor.hintKey ? t(descriptor.hintKey) : ''
    appStore.showError(buildPaymentErrorToastMessage(msg, hint))
    return true
  }

  async function attemptMobileQrFallback(err: unknown, context: MobileQrFallbackContext): Promise<boolean> {
    if (!shouldFallbackToDesktopQr(err, context.paymentType, context.attempted)) return false
    try {
      const visibleMethod = normalizeVisibleMethod(context.paymentType) || context.paymentType
      const payload = buildCreateOrderPayload({
        amount: context.orderAmount, paymentType: visibleMethod, orderType: context.orderType,
        planId: context.planId, origin: typeof window !== 'undefined' ? window.location.origin : '',
        isMobile: false, isWechatBrowser: false,
      })
      const result = await paymentStore.createOrder(payload) as CreateOrderResult & { resume_token?: string }
      const stripeMethod = visibleMethod === 'wxpay' ? 'wechat_pay' : 'alipay'
      const stripeRouteUrl = result.client_secret
        ? router.resolve({ path: '/payment/stripe', query: { order_id: String(result.order_id), client_secret: result.client_secret, method: stripeMethod, resume_token: result.resume_token || undefined } }).href
        : ''
      const decision = decidePaymentLaunch(result, {
        visibleMethod, orderType: context.orderType, isMobile: false, isWechatBrowser: false,
        stripePopupUrl: stripeRouteUrl, stripeRouteUrl,
      })
      if (decision.kind !== 'qr_waiting' || !decision.paymentState.qrCode) return false
      paymentState.value = decision.paymentState
      paymentPhase.value = 'paying'
      persistRecoverySnapshot(decision.recovery)
      appStore.showWarning(t('payment.errors.mobilePaymentFallbackToQr'))
      return true
    } catch {
      return false
    }
  }

  async function createOrder(
    orderAmount: string, orderType: OrderType,
    planId?: number, options: CreateOrderOptions = {},
  ) {
    submitting.value = true
    const requestType = normalizeVisibleMethod(options.paymentType || deps.selectedMethod())
      || options.paymentType || deps.selectedMethod()
    try {
      const payload = buildCreateOrderPayload({
        amount: orderAmount, paymentType: requestType, orderType, planId,
        origin: typeof window !== 'undefined' ? window.location.origin : '',
        isMobile: isMobileDevice(),
        isWechatBrowser: typeof window !== 'undefined' && /MicroMessenger/i.test(window.navigator.userAgent),
      })
      if (options.openid) payload.openid = options.openid
      if (options.wechatResumeToken) payload.wechat_resume_token = options.wechatResumeToken

      const result = await paymentStore.createOrder(payload) as CreateOrderResult & { resume_token?: string }
      const visibleMethod = normalizeVisibleMethod(requestType) || requestType
      await processPaymentDecision(result, visibleMethod, orderAmount, orderType, planId, options)
    } catch (err: unknown) {
      await handleCreateOrderError(err, orderAmount, orderType, planId, requestType, options)
    } finally {
      submitting.value = false
    }
  }
  async function processPaymentDecision(
    result: CreateOrderResult & { resume_token?: string },
    visibleMethod: string, orderAmount: string,
    orderType: OrderType, planId: number | undefined,
    options: CreateOrderOptions,
  ) {
    const stripeMethod = visibleMethod === 'stripe' ? '' : visibleMethod === 'wxpay' ? 'wechat_pay' : 'alipay'
    const stripeRouteUrl = result.client_secret && visibleMethod !== 'airwallex'
      ? router.resolve({ path: '/payment/stripe', query: { order_id: String(result.order_id), client_secret: result.client_secret, method: stripeMethod || undefined, resume_token: result.resume_token || undefined } }).href
      : ''
    const stripePopupUrl = result.client_secret
      ? router.resolve({ path: '/payment/stripe', query: { order_id: String(result.order_id), client_secret: result.client_secret, method: stripeMethod || undefined, resume_token: result.resume_token || undefined, popup: '1' } }).href
      : ''
    const airwallexRouteUrl = result.client_secret && result.intent_id
      ? router.resolve({ path: '/payment/airwallex', query: { order_id: String(result.order_id), out_trade_no: result.out_trade_no || undefined, resume_token: result.resume_token || undefined } }).href
      : ''
    const decision = decidePaymentLaunch(result, {
      visibleMethod, orderType, isMobile: isMobileDevice(),
      isWechatBrowser: typeof window !== 'undefined' && /MicroMessenger/i.test(window.navigator.userAgent),
      stripePopupUrl, stripeRouteUrl, airwallexRouteUrl,
    })

    if (decision.kind === 'wechat_oauth' && decision.oauth?.authorize_url) {
      window.location.href = buildWechatOAuthAuthorizeUrl(decision.oauth.authorize_url, { paymentType: visibleMethod, orderType, planId, orderAmount })
      return
    }
    if (decision.kind === 'unhandled') {
      applyScenarioError({ reason: 'UNHANDLED_PAYMENT_SCENARIO' }, visibleMethod)
      return
    }

    paymentState.value = decision.paymentState
    paymentPhase.value = 'paying'
    persistRecoverySnapshot(decision.recovery)

    const openWindow = (url: string) => {
      const win = window.open(url, 'paymentPopup', getPaymentPopupFeatures())
      if (!win || win.closed) window.location.href = url
    }
    if (decision.kind === 'stripe_popup') { openWindow(decision.paymentState.payUrl); return }
    if (decision.kind === 'stripe_route') { window.location.href = decision.paymentState.payUrl; return }
    if (decision.kind === 'airwallex_route') { window.location.href = decision.paymentState.payUrl; return }
    if (decision.kind === 'wechat_jsapi' && decision.jsapi) {
      await handleWechatJsapi(decision, visibleMethod, orderAmount, orderType, planId, options)
      return
    }
    if (decision.kind === 'redirect_waiting' && decision.paymentState.payUrl) {
      if (isMobileDevice()) { window.location.href = decision.paymentState.payUrl; return }
      openWindow(decision.paymentState.payUrl)
    }
  }

  async function handleWechatJsapi(
    decision: ReturnType<typeof decidePaymentLaunch>,
    visibleMethod: string, orderAmount: string,
    orderType: OrderType, planId: number | undefined,
    options: CreateOrderOptions,
  ) {
    try {
      const jsapiResult = await invokeWechatJsapiPayment(decision.jsapi as Record<string, unknown>)
      const errMsg = String(jsapiResult.err_msg || '').toLowerCase()
      if (errMsg.includes('cancel')) {
        appStore.showInfo(t('payment.qr.cancelled'))
        resetPayment()
      } else if (errMsg && !errMsg.includes('ok')) {
        resetPayment()
        const ctx: MobileQrFallbackContext = { orderAmount, orderType, planId, paymentType: visibleMethod, attempted: options.mobileQrFallbackAttempted === true }
        const fallbackApplied = await attemptMobileQrFallback({ reason: 'WECHAT_JSAPI_FAILED', message: errMsg }, ctx)
        if (!fallbackApplied) applyScenarioError({ reason: 'WECHAT_JSAPI_FAILED', message: errMsg }, visibleMethod)
      } else {
        const resultState = { ...decision.paymentState }
        resetPayment()
        await redirectToPaymentResult(resultState)
      }
    } catch (err: unknown) {
      resetPayment()
      const ctx: MobileQrFallbackContext = { orderAmount, orderType, planId, paymentType: visibleMethod, attempted: options.mobileQrFallbackAttempted === true }
      const fallbackApplied = await attemptMobileQrFallback(err, ctx)
      if (!fallbackApplied) throw err
    }
  }

  async function handleCreateOrderError(
    err: unknown, orderAmount: string, orderType: OrderType,
    planId: number | undefined, requestType: string, options: CreateOrderOptions,
  ) {
    const apiErr = err as Record<string, unknown>
    if (apiErr.reason === 'TOO_MANY_PENDING') {
      const metadata = apiErr.metadata as Record<string, unknown> | undefined
      appStore.showError(buildPaymentErrorToastMessage(t('payment.errors.tooManyPending', { max: metadata?.max || '' }), ''))
    } else if (apiErr.reason === 'CANCEL_RATE_LIMITED') {
      appStore.showError(buildPaymentErrorToastMessage(t('payment.errors.cancelRateLimited'), ''))
    } else if (await attemptMobileQrFallback(err, {
      orderAmount, orderType, planId, paymentType: requestType,
      attempted: options.mobileQrFallbackAttempted === true,
    })) {
      return
    } else {
      const method = normalizeVisibleMethod(options.paymentType || deps.selectedMethod()) || deps.selectedMethod()
      const handled = applyScenarioError(err, method)
      if (!handled) {
        const msg = extractI18nErrorMessage(err, t, 'payment.errors', extractApiErrorMessage(err, t('payment.result.failed')))
        appStore.showError(buildPaymentErrorToastMessage(msg, ''))
      }
    }
  }

  function tryRestorePayment(routeResumeToken?: string): boolean {
    if (typeof window === 'undefined') return false
    const restored = readPaymentRecoverySnapshot(
      window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY),
      { resumeToken: routeResumeToken },
    )
    if (restored) {
      paymentState.value = restored
      paymentPhase.value = 'paying'
      return true
    }
    removeRecoverySnapshot()
    return false
  }

  function getRestoredMethod(): string {
    return normalizeVisibleMethod(paymentState.value.paymentType) || ''
  }

  return {
    submitting,
    paymentPhase,
    paymentState,
    createOrder,
    resetPayment,
    removeRecoverySnapshot,
    tryRestorePayment,
    getRestoredMethod,
  }
}