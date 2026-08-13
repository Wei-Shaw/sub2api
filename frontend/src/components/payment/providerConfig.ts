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
  sepay: ['sepay'],
  nowpayments: ['nowpayments'],
}

/** Fixed display order for user-facing payment methods */
export const METHOD_ORDER = ['sepay', 'nowpayments'] as const

/** Payment mode constants */
export const PAYMENT_MODE_QRCODE = 'qrcode'
/** Send the payer to the provider's hosted checkout page instead of rendering a QR. */
export const PAYMENT_MODE_REDIRECT = 'redirect'

/**
 * Currencies a provider may be configured to price in.
 *
 * SePay is not offered a choice: it reads Vietnamese bank transactions, so it
 * always settles in dong. This list is for NOWPayments, which quotes a fiat
 * price and lets the buyer pay in any coin.
 */
export const PAYMENT_CURRENCY_OPTIONS: TypeOption[] = [
  { value: 'USD', label: 'USD' },
  { value: 'EUR', label: 'EUR' },
  { value: 'VND', label: 'VND' },
  { value: 'SGD', label: 'SGD' },
  { value: 'AUD', label: 'AUD' },
  { value: 'GBP', label: 'GBP' },
]

/** Webhook paths for each provider (relative to origin). */
export const WEBHOOK_PATHS: Record<string, string> = {
  sepay: '/api/v1/payment/webhook/sepay',
  nowpayments: '/api/v1/payment/webhook/nowpayments',
}

export const RETURN_PATH = '/payment/result'

/** Fixed callback paths per provider — displayed as read-only after base URL. */
export const PROVIDER_CALLBACK_PATHS: Record<string, CallbackPaths> = {
  // SePay posts to the webhook configured in its own dashboard; there is no
  // return URL because the payer never leaves our page.
  sepay: { notifyUrl: WEBHOOK_PATHS.sepay },
  nowpayments: { notifyUrl: WEBHOOK_PATHS.nowpayments, returnUrl: RETURN_PATH },
}

/** Per-provider config fields (excludes notifyUrl/returnUrl which are handled separately). */
export const PROVIDER_CONFIG_FIELDS: Record<string, ConfigFieldDef[]> = {
  sepay: [
    { key: 'accountNumber', label: '', sensitive: false },
    { key: 'bankCode', label: '', sensitive: false, hintKey: 'admin.settings.payment.field_bankCodeHint' },
    { key: 'bankBin', label: '', sensitive: false, optional: true, clearable: true, hintKey: 'admin.settings.payment.field_bankBinHint' },
    { key: 'accountName', label: '', sensitive: false, optional: true, clearable: true },
    { key: 'apiKey', label: '', sensitive: true, hintKey: 'admin.settings.payment.field_sepayApiKeyHint' },
    { key: 'apiToken', label: '', sensitive: true, optional: true, hintKey: 'admin.settings.payment.field_sepayApiTokenHint' },
  ],
  nowpayments: [
    { key: 'apiKey', label: '', sensitive: true },
    { key: 'ipnSecret', label: '', sensitive: true, hintKey: 'admin.settings.payment.field_ipnSecretHint' },
    { key: 'currency', label: '', sensitive: false, defaultValue: 'USD', hintKey: 'admin.settings.payment.field_paymentCurrencyHint', options: PAYMENT_CURRENCY_OPTIONS },
    { key: 'apiBase', label: '', sensitive: false, optional: true, clearable: true, defaultValue: 'https://api.nowpayments.io/v1' },
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
