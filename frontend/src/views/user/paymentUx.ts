import { normalizeVisibleMethod REDACTED from '@/components/payment/paymentFlow'
import { extractApiErrorCode REDACTED from '@/utils/apiError'

const DISPLAY_METHOD_ALIASES: Record<string, string> = {
  wechat: 'wxpay',
  wechat_pay: 'wxpay',
REDACTED

export interface PaymentScenarioContext {
  paymentMethod: string
  isMobile: boolean
  isWechatBrowser: boolean
REDACTED

export interface PaymentScenarioErrorDescriptor {
  messageKey: string
  hintKey?: string
REDACTED

export function normalizePaymentMethodForDisplay(paymentType: string): string {
  const trimmed = paymentType.trim().toLowerCase()
  const visibleMethod = normalizeVisibleMethod(trimmed)
  if (visibleMethod) return visibleMethod
  return DISPLAY_METHOD_ALIASES[trimmed] ?? trimmed
REDACTED

export function paymentMethodI18nKey(paymentType: string): string {
  return `payment.methods.${normalizePaymentMethodForDisplay(paymentType)REDACTED`
REDACTED

export function buildPaymentErrorToastMessage(message: string, hint?: string): string {
  if (!hint) return message
  return `${messageREDACTED ${hintREDACTED`.trim()
REDACTED

function defaultWechatHint(context: PaymentScenarioContext): string {
  if (!context.isMobile) return 'payment.errors.wechatScanOnDesktopHint'
  return 'payment.errors.wechatOpenInWeChatHint'
REDACTED

function defaultAlipayHint(context: PaymentScenarioContext): string {
  if (context.isMobile) return 'payment.errors.alipayMobileOpenHint'
  return 'payment.errors.alipayDesktopQrHint'
REDACTED

export function describePaymentScenarioError(
  error: unknown,
  context: PaymentScenarioContext,
): PaymentScenarioErrorDescriptor | null {
  const method = normalizePaymentMethodForDisplay(context.paymentMethod)
  const code = extractApiErrorCode(error)
  const message = error instanceof Error
    ? error.message
    : (typeof error === 'object' && error && 'message' in error && typeof error.message === 'string'
      ? error.message
      : String(error || ''))
  const normalizedMessage = message.toLowerCase()

  if (method === 'wxpay') {
    if (code === 'WECHAT_H5_NOT_AUTHORIZED') {
      return {
        messageKey: 'payment.errors.wechatH5NotAuthorized',
        hintKey: defaultWechatHint(context),
      REDACTED
    REDACTED
    if (code === 'WECHAT_PAYMENT_MP_NOT_CONFIGURED') {
      return {
        messageKey: 'payment.errors.wechatPaymentMpNotConfigured',
        hintKey: context.isWechatBrowser
          ? 'payment.errors.wechatSwitchBrowserHint'
          : defaultWechatHint(context),
      REDACTED
    REDACTED
    if (code === 'NO_AVAILABLE_INSTANCE') {
      return {
        messageKey: 'payment.errors.wechatUnavailable',
        hintKey: defaultWechatHint(context),
      REDACTED
    REDACTED
    if (code === 'WECHAT_JSAPI_FAILED' || normalizedMessage.includes('get_brand_wcpay_request:fail')) {
      return {
        messageKey: 'payment.errors.wechatJsapiFailed',
        hintKey: defaultWechatHint(context),
      REDACTED
    REDACTED
    if (
      normalizedMessage.includes('weixinjsbridge is unavailable') ||
      normalizedMessage.includes('wechat_jsapi_unavailable')
    ) {
      return {
        messageKey: 'payment.errors.wechatJsapiUnavailable',
        hintKey: 'payment.errors.wechatOpenInWeChatHint',
      REDACTED
    REDACTED
    if (code === 'PAYMENT_GATEWAY_ERROR' || code === 'UNHANDLED_PAYMENT_SCENARIO') {
      return {
        messageKey: 'payment.errors.wechatUnavailable',
        hintKey: defaultWechatHint(context),
      REDACTED
    REDACTED
  REDACTED

  if (method === 'alipay' && (code === 'PAYMENT_GATEWAY_ERROR' || code === 'UNHANDLED_PAYMENT_SCENARIO')) {
    return {
      messageKey: context.isMobile
        ? 'payment.errors.alipayMobileUnavailable'
        : 'payment.errors.alipayDesktopUnavailable',
      hintKey: defaultAlipayHint(context),
    REDACTED
  REDACTED

  return null
REDACTED
