import type {
  BankTransferInfo,
  CreateOrderRequest,
  CreateOrderResult,
  MethodLimit,
  OrderType,
} from '@/types/payment'

export const PAYMENT_RECOVERY_STORAGE_KEY = 'payment.recovery.current'

/**
 * The two providers left each back exactly one user-facing method, so there is
 * no alias layer any more: a payment type is a provider key.
 */
export const VISIBLE_PAYMENT_METHODS = ['sepay', 'nowpayments'] as const

export type VisiblePaymentMethod = (typeof VISIBLE_PAYMENT_METHODS)[number]

export type PaymentLaunchKind =
  | 'qr_waiting'
  | 'redirect_waiting'
  | 'unhandled'

export interface PaymentRecoverySnapshot {
  orderId: number
  amount: number
  qrCode: string
  expiresAt: string
  paymentType: string
  payUrl: string
  outTradeNo: string
  intentId: string
  currency: string
  payAmount: number
  orderType: OrderType | ''
  paymentMode: string
  resumeToken: string
  transfer?: BankTransferInfo
  createdAt: number
}

export interface PaymentLaunchContext {
  visibleMethod: string
  orderType: OrderType
  isMobile: boolean
  now?: number
}

export interface PaymentLaunchDecision {
  kind: PaymentLaunchKind
  paymentState: PaymentRecoverySnapshot
  recovery: PaymentRecoverySnapshot
}

export interface BuildCreateOrderPayloadInput {
  amount: number
  paymentType: string
  orderType: OrderType
  planId?: number
  origin?: string
  isMobile: boolean
}

type CreateOrderFlowResult = CreateOrderResult & {
  resume_token?: string
}

type StorageWriter = Pick<Storage, 'removeItem' | 'setItem'>

export function isVisiblePaymentMethod(method: string): method is VisiblePaymentMethod {
  return (VISIBLE_PAYMENT_METHODS as readonly string[]).includes(method.trim())
}

export function normalizeVisibleMethod(method: string): VisiblePaymentMethod | '' {
  const trimmed = method.trim()
  return isVisiblePaymentMethod(trimmed) ? trimmed : ''
}

export function getVisibleMethods(methods: Record<string, MethodLimit>): Record<string, MethodLimit> {
  const visible: Record<string, MethodLimit> = {}

  Object.entries(methods).forEach(([type, limit]) => {
    const normalized = normalizeVisibleMethod(type)
    if (!normalized) return
    visible[normalized] = { ...limit }
  })

  return visible
}

export function buildCreateOrderPayload(input: BuildCreateOrderPayloadInput): CreateOrderRequest {
  const paymentType = input.paymentType.trim()
  const normalizedOrigin = (input.origin || '').trim().replace(/\/+$/, '')
  const payload: CreateOrderRequest = {
    amount: input.amount,
    payment_type: paymentType,
    order_type: input.orderType,
    is_mobile: input.isMobile,
    payment_source: 'hosted_redirect',
  }

  if (input.planId) {
    payload.plan_id = input.planId
  }
  if (normalizedOrigin) {
    payload.return_url = `${normalizedOrigin}/payment/result`
  }

  return payload
}

/**
 * Decides how to present a freshly created order.
 *
 * SePay hands back a VietQR payload the payer scans in their banking app;
 * NOWPayments hands back a hosted invoice URL. Which one arrives is the
 * provider's decision, so the shape of the response drives this — not the
 * method name.
 */
export function decidePaymentLaunch(
  result: CreateOrderFlowResult,
  context: PaymentLaunchContext,
): PaymentLaunchDecision {
  const visibleMethod = normalizeVisibleMethod(context.visibleMethod) || context.visibleMethod
  const baseState = createPaymentRecoverySnapshot({
    orderId: result.order_id,
    amount: result.amount,
    qrCode: result.qr_code || '',
    expiresAt: result.expires_at || '',
    paymentType: visibleMethod,
    payUrl: result.pay_url || '',
    outTradeNo: result.out_trade_no || '',
    intentId: result.intent_id || '',
    currency: result.currency || '',
    payAmount: result.pay_amount,
    orderType: context.orderType,
    paymentMode: (result.payment_mode || '').trim(),
    resumeToken: result.resume_token || '',
    transfer: result.transfer,
  }, context.now)

  const normalizedPaymentMode = baseState.paymentMode.trim().toLowerCase()
  const prefersRedirect = normalizedPaymentMode === 'redirect'
  const prefersQr = normalizedPaymentMode === 'qrcode' || (!prefersRedirect && !!baseState.qrCode)

  if (prefersQr && baseState.qrCode) {
    return { kind: 'qr_waiting', paymentState: baseState, recovery: baseState }
  }

  if (baseState.payUrl) {
    return { kind: 'redirect_waiting', paymentState: baseState, recovery: baseState }
  }

  if (baseState.qrCode) {
    return { kind: 'qr_waiting', paymentState: baseState, recovery: baseState }
  }

  return { kind: 'unhandled', paymentState: baseState, recovery: baseState }
}

export function createPaymentRecoverySnapshot(
  state: Omit<PaymentRecoverySnapshot, 'createdAt'>,
  now = Date.now(),
): PaymentRecoverySnapshot {
  return {
    ...state,
    createdAt: now,
  }
}

export function writePaymentRecoverySnapshot(
  storage: StorageWriter,
  snapshot: PaymentRecoverySnapshot,
  key = PAYMENT_RECOVERY_STORAGE_KEY,
): void {
  storage.setItem(key, JSON.stringify(snapshot))
}

export function clearPaymentRecoverySnapshot(
  storage: Pick<Storage, 'removeItem'>,
  key = PAYMENT_RECOVERY_STORAGE_KEY,
): void {
  storage.removeItem(key)
}

export function readPaymentRecoverySnapshot(
  raw: string | null | undefined,
  options: { now?: number; resumeToken?: string } = {},
): PaymentRecoverySnapshot | null {
  if (!raw) return null

  try {
    const parsed = JSON.parse(raw) as Partial<PaymentRecoverySnapshot>
    if (
      typeof parsed.orderId !== 'number'
      || typeof parsed.amount !== 'number'
      || typeof parsed.qrCode !== 'string'
      || typeof parsed.expiresAt !== 'string'
      || typeof parsed.paymentType !== 'string'
      || typeof parsed.payUrl !== 'string'
      || (parsed.outTradeNo != null && typeof parsed.outTradeNo !== 'string')
      || (parsed.intentId != null && typeof parsed.intentId !== 'string')
      || (parsed.currency != null && typeof parsed.currency !== 'string')
      || typeof parsed.payAmount !== 'number'
      || typeof parsed.paymentMode !== 'string'
      || typeof parsed.resumeToken !== 'string'
      || (parsed.transfer != null && typeof parsed.transfer !== 'object')
      || typeof parsed.createdAt !== 'number'
    ) {
      return null
    }

    const now = options.now ?? Date.now()
    const expiresAt = Date.parse(parsed.expiresAt)
    if (Number.isFinite(expiresAt) && expiresAt <= now) {
      return null
    }
    if (options.resumeToken && parsed.resumeToken !== options.resumeToken) {
      return null
    }

    return {
      orderId: parsed.orderId,
      amount: parsed.amount,
      qrCode: parsed.qrCode,
      expiresAt: parsed.expiresAt,
      paymentType: parsed.paymentType,
      payUrl: parsed.payUrl,
      outTradeNo: parsed.outTradeNo || '',
      intentId: parsed.intentId || '',
      currency: parsed.currency || '',
      payAmount: parsed.payAmount,
      orderType: parsed.orderType === 'subscription' ? 'subscription' : 'balance',
      paymentMode: parsed.paymentMode,
      resumeToken: parsed.resumeToken,
      transfer: parsed.transfer,
      createdAt: parsed.createdAt,
    }
  } catch {
    return null
  }
}
