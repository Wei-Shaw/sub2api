/**
 * Shared constants and types for payment provider management.
 */

// --- Types ---

export interface ConfigFieldDef {
  key: string
  label: string
  sensitive: boolean
  optional?: boolean
  clearable?: boolean
  defaultValue?: string
  hintKey?: string
  options?: TypeOption[]
}

export interface TypeOption {
  value: string
  label: string
  [key: string]: unknown
}

/** Callback URL paths for a provider. */
export interface CallbackPaths {
  notifyUrl?: string
  returnUrl?: string
}

// --- Constants ---

/** Maps provider key → available payment types. */
export const PROVIDER_SUPPORTED_TYPES: Record<string, string[]> = {
  easypay: ['alipay', 'wxpay'],
  alipay: ['alipay'],
  wxpay: ['wxpay'],
  stripe: ['card', 'alipay', 'wxpay', 'link'],
  airwallex: ['airwallex'],
}

/** Available payment modes for EasyPay providers. */
export const EASYPAY_PAYMENT_MODES = ['qrcode', 'popup'] as const

/** Fixed display order for user-facing payment methods */
export const METHOD_ORDER = ['alipay', 'alipay_direct', 'wxpay', 'wxpay_direct', 'stripe', 'airwallex'] as const

/** Payment mode constants */
export const PAYMENT_MODE_QRCODE = 'qrcode'
export const PAYMENT_MODE_POPUP = 'popup'

export const PAYMENT_CURRENCY_OPTIONS: TypeOption[] = [
  { value: 'CNY', label: 'CNY' },
  { value: 'HKD', label: 'HKD' },
  { value: 'USD', label: 'USD' },
  { value: 'EUR', label: 'EUR' },
  { value: 'GBP', label: 'GBP' },
  { value: 'AUD', label: 'AUD' },
  { value: 'CAD', label: 'CAD' },
  { value: 'SGD', label: 'SGD' },
  { value: 'JPY', label: 'JPY' },
  { value: 'KRW', label: 'KRW' },
  { value: 'NZD', label: 'NZD' },
]

// 与后端当前集成的 stripe-go v85.0.0 的 stripe.APIVersion 保持一致。
export const STRIPE_SDK_API_VERSION = '2026-03-25.dahlia'

/** Preferred popup size for payment gateways. Alipay's standard checkout
 * (QR + account login panel) needs ~1200×900 to render without any scrolling. */
const PAYMENT_POPUP_PREFERRED_WIDTH = 1250
const PAYMENT_POPUP_PREFERRED_HEIGHT = 900

/** Build a window.open features string sized to fit within the current screen
 * while preferring the above dimensions. Centers the popup on the available
 * work area so nothing is clipped on smaller laptop displays. */
export function getPaymentPopupFeatures(): string {
  const screen = typeof window !== 'undefined' ? window.screen : null
  const availW = screen?.availWidth ?? PAYMENT_POPUP_PREFERRED_WIDTH
  const availH = screen?.availHeight ?? PAYMENT_POPUP_PREFERRED_HEIGHT
  const width = Math.min(PAYMENT_POPUP_PREFERRED_WIDTH, availW - 40)
  const height = Math.min(PAYMENT_POPUP_PREFERRED_HEIGHT, availH - 40)
  const left = Math.max(0, Math.floor((availW - width) / 2))
  const top = Math.max(0, Math.floor((availH - height) / 2))
  return `width=${width},height=${height},left=${left},top=${top},scrollbars=yes,resizable=yes`
}

/** Webhook paths for each provider (relative to origin). */
export const WEBHOOK_PATHS: Record<string, string> = {
  easypay: '/api/v1/payment/webhook/easypay',
  alipay: '/api/v1/payment/webhook/alipay',
  wxpay: '/api/v1/payment/webhook/wxpay',
  stripe: '/api/v1/payment/webhook/stripe',
  airwallex: '/api/v1/payment/webhook/airwallex',
}

export const RETURN_PATH = '/payment/result'

/** Fixed callback paths per provider — displayed as read-only after base URL. */
export const PROVIDER_CALLBACK_PATHS: Record<string, CallbackPaths> = {
  easypay: { notifyUrl: WEBHOOK_PATHS.easypay, returnUrl: RETURN_PATH },
  alipay: { notifyUrl: WEBHOOK_PATHS.alipay, returnUrl: RETURN_PATH },
  wxpay: { notifyUrl: WEBHOOK_PATHS.wxpay },
  // stripe: 不需要回调 URL 配置，Webhook 单独配置。
  // airwallex: 不需要回调 URL 配置，Webhook 在空中云汇后台配置。
}

/** Per-provider config fields (excludes notifyUrl/returnUrl which are handled separately). */
export const PROVIDER_CONFIG_FIELDS: Record<string, ConfigFieldDef[]> = {
  easypay: [
    { key: 'pid', label: 'PID', sensitive: false },
    { key: 'pkey', label: 'PKey', sensitive: true },
    { key: 'apiBase', label: '', sensitive: false },
    { key: 'cidAlipay', label: '', sensitive: false, optional: true },
    { key: 'cidWxpay', label: '', sensitive: false, optional: true },
  ],
  alipay: [
    { key: 'appId', label: 'App ID', sensitive: false },
    { key: 'privateKey', label: '', sensitive: true },
    { key: 'publicKey', label: '', sensitive: true },
  ],
  wxpay: [
    { key: 'appId', label: 'App ID', sensitive: false },
    { key: 'mchId', label: '', sensitive: false },
    { key: 'privateKey', label: '', sensitive: true },
    { key: 'apiV3Key', label: '', sensitive: true },
    { key: 'certSerial', label: '', sensitive: false },
    { key: 'publicKey', label: '', sensitive: true },
    { key: 'publicKeyId', label: '', sensitive: false },
  ],
  stripe: [
    { key: 'secretKey', label: '', sensitive: true },
    { key: 'publishableKey', label: '', sensitive: false },
    { key: 'webhookSecret', label: '', sensitive: true },
    { key: 'currency', label: '', sensitive: false, defaultValue: 'CNY', hintKey: 'admin.settings.payment.field_paymentCurrencyHint', options: PAYMENT_CURRENCY_OPTIONS },
  ],
  airwallex: [
    { key: 'clientId', label: '', sensitive: false },
    { key: 'apiKey', label: '', sensitive: true },
    { key: 'webhookSecret', label: '', sensitive: true },
    { key: 'apiBase', label: '', sensitive: false, defaultValue: 'https://api.airwallex.com/api/v1', hintKey: 'admin.settings.payment.field_airwallexApiBaseHint' },
    { key: 'countryCode', label: '', sensitive: false, defaultValue: 'CN' },
    { key: 'currency', label: '', sensitive: false, defaultValue: 'CNY', hintKey: 'admin.settings.payment.field_paymentCurrencyHint', options: PAYMENT_CURRENCY_OPTIONS },
    { key: 'accountId', label: '', sensitive: false, optional: true, clearable: true, hintKey: 'admin.settings.payment.field_accountIdHint' },
  ],
}

// --- Helpers ---

/** Resolve type label for display. */
export function resolveTypeLabel(
  typeVal: string,
  _providerKey: string,
  allTypes: TypeOption[],
  _redirectLabel: string,
): TypeOption {
  return allTypes.find(pt => pt.value === typeVal) || { value: typeVal, label: typeVal }
}

/** Get available type options for a provider key. */
export function getAvailableTypes(
  providerKey: string,
  allTypes: TypeOption[],
  redirectLabel: string,
): TypeOption[] {
  const types = PROVIDER_SUPPORTED_TYPES[providerKey] || []
  return types.map(t => resolveTypeLabel(t, providerKey, allTypes, redirectLabel))
}

/** Extract base URL from a full callback URL by removing the known path suffix. */
export function extractBaseUrl(fullUrl: string, path: string): string {
  if (!fullUrl) return ''
  if (fullUrl.endsWith(path)) return fullUrl.slice(0, -path.length)
  // Fallback: try to extract origin
  try { return new URL(fullUrl).origin } catch { return fullUrl }
}
