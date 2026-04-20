import type { CreateOrderRequest, CreateOrderResult, MethodLimit, OrderType REDACTED from '@/types/payment'

export const PAYMENT_RECOVERY_STORAGE_KEY = 'payment.recovery.current'

const VISIBLE_METHOD_ALIASES = {
  alipay: 'alipay',
  alipay_direct: 'alipay',
  wxpay: 'wxpay',
  wxpay_direct: 'wxpay',
REDACTED as const

export type VisiblePaymentMethod = 'alipay' | 'wxpay'
export type StripeVisibleMethod = 'alipay' | 'wechat_pay'
export type PaymentLaunchKind =
  | 'qr_waiting'
  | 'redirect_waiting'
  | 'stripe_popup'
  | 'stripe_route'
  | 'unhandled'

export interface PaymentRecoverySnapshot {
  orderId: number
  amount: number
  qrCode: string
  expiresAt: string
  paymentType: string
  payUrl: string
  clientSecret: string
  payAmount: number
  orderType: OrderType | ''
  paymentMode: string
  resumeToken: string
  createdAt: number
REDACTED

export interface PaymentLaunchContext {
  visibleMethod: string
  orderType: OrderType
  isMobile: boolean
  now?: number
  stripePopupUrl?: string
  stripeRouteUrl?: string
REDACTED

export interface PaymentLaunchDecision {
  kind: PaymentLaunchKind
  paymentState: PaymentRecoverySnapshot
  recovery: PaymentRecoverySnapshot
  stripeMethod?: StripeVisibleMethod
REDACTED

export interface BuildCreateOrderPayloadInput {
  amount: number
  paymentType: string
  orderType: OrderType
  planId?: number
  origin?: string
  isWechatBrowser: boolean
REDACTED

type CreateOrderFlowResult = CreateOrderResult & {
  resume_token?: string
REDACTED

type StorageWriter = Pick<Storage, 'removeItem' | 'setItem'>

export function normalizeVisibleMethod(method: string): VisiblePaymentMethod | '' {
  const normalized = VISIBLE_METHOD_ALIASES[method.trim() as keyof typeof VISIBLE_METHOD_ALIASES]
  return normalized ?? ''
REDACTED

export function getVisibleMethods(methods: Record<string, MethodLimit>): Record<string, MethodLimit> {
  const visible: Record<string, MethodLimit> = {REDACTED

  Object.entries(methods).forEach(([type, limit]) => {
    const normalized = normalizeVisibleMethod(type)
    if (!normalized) return

    const isCanonical = type === normalized
    const existing = visible[normalized]
    if (!existing || isCanonical) {
      visible[normalized] = { ...limit REDACTED
    REDACTED
  REDACTED)

  return visible
REDACTED

export function buildCreateOrderPayload(input: BuildCreateOrderPayloadInput): CreateOrderRequest {
  const visibleMethod = normalizeVisibleMethod(input.paymentType) || input.paymentType.trim()
  const normalizedOrigin = (input.origin || '').trim().replace(/\/+$/, '')
  const payload: CreateOrderRequest = {
    amount: input.amount,
    payment_type: visibleMethod,
    order_type: input.orderType,
    payment_source: visibleMethod === 'wxpay' && input.isWechatBrowser
      ? 'wechat_in_app_resume'
      : 'hosted_redirect',
  REDACTED

  if (input.planId) {
    payload.plan_id = input.planId
  REDACTED
  if (normalizedOrigin) {
    payload.return_url = `${normalizedOriginREDACTED/payment/result`
  REDACTED

  return payload
REDACTED

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
    clientSecret: result.client_secret || '',
    payAmount: result.pay_amount,
    orderType: context.orderType,
    paymentMode: (result.payment_mode || '').trim(),
    resumeToken: result.resume_token || '',
  REDACTED, context.now)

  if (baseState.clientSecret) {
    const stripeMethod: StripeVisibleMethod = visibleMethod === 'wxpay' ? 'wechat_pay' : 'alipay'
    const kind: PaymentLaunchKind = stripeMethod === 'alipay' && !context.isMobile
      ? 'stripe_popup'
      : 'stripe_route'
    const payUrl = kind === 'stripe_popup'
      ? context.stripePopupUrl || context.stripeRouteUrl || ''
      : context.stripeRouteUrl || context.stripePopupUrl || ''
    const paymentState = { ...baseState, payUrl REDACTED
    return { kind, paymentState, recovery: paymentState, stripeMethod REDACTED
  REDACTED

  if (baseState.qrCode) {
    return { kind: 'qr_waiting', paymentState: baseState, recovery: baseState REDACTED
  REDACTED

  if (baseState.payUrl) {
    return { kind: 'redirect_waiting', paymentState: baseState, recovery: baseState REDACTED
  REDACTED

  return { kind: 'unhandled', paymentState: baseState, recovery: baseState REDACTED
REDACTED

export function createPaymentRecoverySnapshot(
  state: Omit<PaymentRecoverySnapshot, 'createdAt'>,
  now = Date.now(),
): PaymentRecoverySnapshot {
  return {
    ...state,
    createdAt: now,
  REDACTED
REDACTED

export function writePaymentRecoverySnapshot(
  storage: StorageWriter,
  snapshot: PaymentRecoverySnapshot,
  key = PAYMENT_RECOVERY_STORAGE_KEY,
): void {
  storage.setItem(key, JSON.stringify(snapshot))
REDACTED

export function clearPaymentRecoverySnapshot(
  storage: Pick<Storage, 'removeItem'>,
  key = PAYMENT_RECOVERY_STORAGE_KEY,
): void {
  storage.removeItem(key)
REDACTED

export function readPaymentRecoverySnapshot(
  raw: string | null | undefined,
  options: { now?: number; resumeToken?: string REDACTED = {REDACTED,
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
      || typeof parsed.clientSecret !== 'string'
      || typeof parsed.payAmount !== 'number'
      || typeof parsed.paymentMode !== 'string'
      || typeof parsed.resumeToken !== 'string'
      || typeof parsed.createdAt !== 'number'
    ) {
      return null
    REDACTED

    const now = options.now ?? Date.now()
    const expiresAt = Date.parse(parsed.expiresAt)
    if (Number.isFinite(expiresAt) && expiresAt <= now) {
      return null
    REDACTED
    if (options.resumeToken && parsed.resumeToken && parsed.resumeToken !== options.resumeToken) {
      return null
    REDACTED

    return {
      orderId: parsed.orderId,
      amount: parsed.amount,
      qrCode: parsed.qrCode,
      expiresAt: parsed.expiresAt,
      paymentType: parsed.paymentType,
      payUrl: parsed.payUrl,
      clientSecret: parsed.clientSecret,
      payAmount: parsed.payAmount,
      orderType: parsed.orderType === 'subscription' ? 'subscription' : 'balance',
      paymentMode: parsed.paymentMode,
      resumeToken: parsed.resumeToken,
      createdAt: parsed.createdAt,
    REDACTED
  REDACTED catch {
    return null
  REDACTED
REDACTED
