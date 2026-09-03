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

/** Provider key of the only supported gateway. */
export const PROVIDER_SEPAY = 'sepay'

/** User-facing SePay payment methods. */
export const SEPAY_BANK_TRANSFER = 'sepay_bank_transfer'
export const SEPAY_NAPAS = 'sepay_napas'
export const SEPAY_CARD = 'sepay_card'

/** Maps provider key -> available payment types. */
export const PROVIDER_SUPPORTED_TYPES: Record<string, string[]> = {
  [PROVIDER_SEPAY]: [SEPAY_BANK_TRANSFER, SEPAY_NAPAS, SEPAY_CARD],
}

/** Fixed display order for user-facing payment methods. */
export const METHOD_ORDER = [SEPAY_BANK_TRANSFER, SEPAY_NAPAS, SEPAY_CARD] as const

/** Payment mode constants. */
export const PAYMENT_MODE_QRCODE = 'qrcode'
export const PAYMENT_MODE_POPUP = 'popup'
export const PAYMENT_MODE_REDIRECT = 'redirect'

/** SePay environments, mirroring the backend `env` config field. */
export const SEPAY_ENV_OPTIONS: TypeOption[] = [
  { value: 'production', label: 'Production' },
  { value: 'sandbox', label: 'Sandbox' },
]

export const PAYMENT_CURRENCY_OPTIONS: TypeOption[] = [
  { value: 'VND', label: 'VND' },
  { value: 'USD', label: 'USD' },
]

/** Webhook paths for each provider (relative to origin). */
export const WEBHOOK_PATHS: Record<string, string> = {
  [PROVIDER_SEPAY]: '/api/v1/payment/webhook/sepay',
}

export const RETURN_PATH = '/payment/result'

/** Fixed callback paths per provider - displayed as read-only after base URL. */
export const PROVIDER_CALLBACK_PATHS: Record<string, CallbackPaths> = {
  [PROVIDER_SEPAY]: { notifyUrl: WEBHOOK_PATHS[PROVIDER_SEPAY], returnUrl: RETURN_PATH },
}

/**
 * Per-provider config fields (excludes notifyUrl/returnUrl which are handled separately).
 *
 * `sensitive: true` must stay in sync with the backend definition in
 * internal/service/payment_config_providers.go (providerSensitiveConfigFields):
 * those values are never returned by the admin GET API.
 */
export const PROVIDER_CONFIG_FIELDS: Record<string, ConfigFieldDef[]> = {
  [PROVIDER_SEPAY]: [
    { key: 'merchantId', label: 'Merchant ID', sensitive: false },
    { key: 'secretKey', label: 'Secret Key', sensitive: true },
    {
      key: 'env',
      label: 'Environment',
      sensitive: false,
      defaultValue: 'production',
      options: SEPAY_ENV_OPTIONS,
    },
    {
      key: 'currency',
      label: 'Currency',
      sensitive: false,
      defaultValue: 'VND',
      hintKey: 'admin.settings.payment.field_paymentCurrencyHint',
      options: PAYMENT_CURRENCY_OPTIONS,
    },
  ],
}

/** Preferred popup size for payment gateways. */
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
