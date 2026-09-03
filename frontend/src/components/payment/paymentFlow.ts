import type {
  CreateOrderRequest,
  CreateOrderResult,
  MethodLimit,
  OrderType,
} from '@/types/payment'
import { METHOD_ORDER } from './providerConfig'

export const PAYMENT_RECOVERY_STORAGE_KEY = 'payment.recovery.current'

export type VisiblePaymentMethod = (typeof METHOD_ORDER)[number]

export type PaymentLaunchKind =
  | 'qr_waiting'
  | 'redirect_waiting'
  | 'unhandled'

const VISIBLE_METHODS = new Set<string>(METHOD_ORDER)

export interface PaymentRecoverySnapshot {
  orderId: number
  amount: number
  qrCode: string
  expiresAt: string
  paymentType: string
  payUrl: string
  outTradeNo: string
  currency: string
  paymentEnv: string
  payAmount: number
  orderType: OrderType | ''
  paymentMode: string
  resumeToken: string
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

/**
 * Normalize a payment method identifier.
 *
 * Each SePay method is its own user-facing choice, so unlike the previous
 * multi-gateway aliasing this never folds a method onto the gateway key —
 * doing so would make the order pick a method the user did not press.
 */
export function normalizeVisibleMethod(method: string): VisiblePaymentMethod | '' {
  const trimmed = method.trim()
  return VISIBLE_METHODS.has(trimmed) ? (trimmed as VisiblePaymentMethod) : ''
}

export function getVisibleMethods(methods: Record<string, MethodLimit>): Record<string, MethodLimit> {
  const visible: Record<string, MethodLimit> = {}

  Object.entries(methods).forEach(([type, limit]) => {
    const normalized = normalizeVisibleMethod(type) || type.trim()
    if (!normalized) return
    visible[normalized] = { ...limit }
  })

  return visible
}

export function buildCreateOrderPayload(input: BuildCreateOrderPayloadInput): CreateOrderRequest {
  const paymentType = normalizeVisibleMethod(input.paymentType) || input.paymentType.trim()
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
    currency: result.currency || '',
    paymentEnv: result.payment_env || '',
    payAmount: result.pay_amount,
    orderType: context.orderType,
    paymentMode: (result.payment_mode || '').trim(),
    resumeToken: result.resume_token || '',
  }, context.now)

  const normalizedPaymentMode = baseState.paymentMode.trim().toLowerCase()
  const prefersRedirect = result.result_type === 'form_post'
    || normalizedPaymentMode === 'redirect'
    || normalizedPaymentMode === 'popup'
    || (context.isMobile && !!baseState.payUrl)
  const prefersQr = normalizedPaymentMode === 'qrcode'
    || normalizedPaymentMode === 'native'
    || (!prefersRedirect && !!baseState.qrCode)

  if (prefersRedirect && baseState.payUrl) {
    return { kind: 'redirect_waiting', paymentState: baseState, recovery: baseState }
  }

  if (prefersQr && baseState.qrCode) {
    return { kind: 'qr_waiting', paymentState: baseState, recovery: baseState }
  }

  if (baseState.payUrl) {
    return { kind: 'redirect_waiting', paymentState: baseState, recovery: baseState }
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
      || (parsed.currency != null && typeof parsed.currency !== 'string')
      || (parsed.paymentEnv != null && typeof parsed.paymentEnv !== 'string')
      || typeof parsed.payAmount !== 'number'
      || typeof parsed.paymentMode !== 'string'
      || typeof parsed.resumeToken !== 'string'
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
      currency: parsed.currency || '',
      paymentEnv: parsed.paymentEnv || '',
      payAmount: parsed.payAmount,
      orderType: parsed.orderType === 'subscription' ? 'subscription' : 'balance',
      paymentMode: parsed.paymentMode,
      resumeToken: parsed.resumeToken,
      createdAt: parsed.createdAt,
    }
  } catch {
    return null
  }
}
