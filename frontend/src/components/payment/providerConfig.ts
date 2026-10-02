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

/** Provider keys of the supported gateways. */
export const PROVIDER_SEPAY = 'sepay'
export const PROVIDER_NOWPAYMENTS = 'nowpayments'
export const PROVIDER_GPMPAY = 'gpmpay'

/**
 * User-facing SePay payment methods.
 *
 * Napas and card were retired — SePay now sells VietQR bank transfer only.
 * Their identifiers survive in `payment.methods.*` i18n keys and in the admin
 * order filters, because orders placed before the change still carry them.
 */
export const SEPAY_BANK_TRANSFER = 'sepay_bank_transfer'

/** NOWPayments offers a single method: its hosted crypto checkout. */
export const NOWPAYMENTS_CRYPTO = 'nowpayments_crypto'

/** GPM Pay offers a single method: a VietQR transfer we render ourselves. */
export const GPMPAY_BANK_TRANSFER = 'gpmpay_bank_transfer'

/** Maps provider key -> available payment types. */
export const PROVIDER_SUPPORTED_TYPES: Record<string, string[]> = {
  [PROVIDER_SEPAY]: [SEPAY_BANK_TRANSFER],
  [PROVIDER_NOWPAYMENTS]: [NOWPAYMENTS_CRYPTO],
  [PROVIDER_GPMPAY]: [GPMPAY_BANK_TRANSFER],
}

/** Fixed display order for user-facing payment methods. */
export const METHOD_ORDER = [SEPAY_BANK_TRANSFER, GPMPAY_BANK_TRANSFER, NOWPAYMENTS_CRYPTO] as const

/** Payment mode constants. */
export const PAYMENT_MODE_QRCODE = 'qrcode'
export const PAYMENT_MODE_POPUP = 'popup'
export const PAYMENT_MODE_REDIRECT = 'redirect'

/** Gateway environments, mirroring the backend `env` config field. Both
 * gateways use the same two values. */
export const PROVIDER_ENV_OPTIONS: TypeOption[] = [
  { value: 'production', label: 'Production' },
  { value: 'sandbox', label: 'Sandbox' },
]

export const SEPAY_ENV_OPTIONS = PROVIDER_ENV_OPTIONS

export const PAYMENT_CURRENCY_OPTIONS: TypeOption[] = [
  { value: 'VND', label: 'VND' },
  { value: 'USD', label: 'USD' },
]

/** Webhook paths for each provider (relative to origin). */
export const WEBHOOK_PATHS: Record<string, string> = {
  [PROVIDER_SEPAY]: '/api/v1/payment/webhook/sepay',
  [PROVIDER_NOWPAYMENTS]: '/api/v1/payment/webhook/nowpayments',
  [PROVIDER_GPMPAY]: '/api/v1/payment/webhook/gpmpay',
}

export const RETURN_PATH = '/payment/result'

/** Fixed callback paths per provider - displayed as read-only after base URL. */
export const PROVIDER_CALLBACK_PATHS: Record<string, CallbackPaths> = {
  [PROVIDER_SEPAY]: { notifyUrl: WEBHOOK_PATHS[PROVIDER_SEPAY], returnUrl: RETURN_PATH },
  [PROVIDER_NOWPAYMENTS]: { notifyUrl: WEBHOOK_PATHS[PROVIDER_NOWPAYMENTS], returnUrl: RETURN_PATH },
  // The payer never leaves our QR page, so there is nothing to return to.
  [PROVIDER_GPMPAY]: { notifyUrl: WEBHOOK_PATHS[PROVIDER_GPMPAY] },
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
      key: 'ipnSecretKey',
      label: 'IPN Secret Key',
      sensitive: true,
      optional: true,
      clearable: true,
      hintKey: 'admin.settings.payment.field_ipnSecretKeyHint',
    },
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
  [PROVIDER_NOWPAYMENTS]: [
    { key: 'apiKey', label: 'API Key', sensitive: true },
    {
      // Required, unlike SePay's: the IPN signature is the only proof a
      // NOWPayments callback is genuine — the gateway offers no order lookup
      // to double-check it against.
      key: 'ipnSecretKey',
      label: 'IPN Secret Key',
      sensitive: true,
      hintKey: 'admin.settings.payment.field_nowPaymentsIpnSecretKeyHint',
    },
    {
      key: 'env',
      label: 'Environment',
      sensitive: false,
      defaultValue: 'production',
      options: PROVIDER_ENV_OPTIONS,
    },
    {
      key: 'currency',
      label: 'Currency',
      sensitive: false,
      defaultValue: 'USD',
      hintKey: 'admin.settings.payment.field_nowPaymentsCurrencyHint',
      options: PAYMENT_CURRENCY_OPTIONS,
    },
    {
      key: 'payCurrency',
      label: 'Pay Currency',
      sensitive: false,
      optional: true,
      clearable: true,
      hintKey: 'admin.settings.payment.field_payCurrencyHint',
    },
  ],
  [PROVIDER_GPMPAY]: [
    { key: 'apiToken', label: 'API Token', sensitive: true },
    {
      // The HMAC signature is the only proof a GPM Pay webhook is genuine.
      key: 'webhookSecret',
      label: 'Webhook Secret',
      sensitive: true,
      hintKey: 'admin.settings.payment.field_gpmPayWebhookSecretHint',
    },
    {
      key: 'bankBin',
      label: 'Bank BIN',
      sensitive: false,
      hintKey: 'admin.settings.payment.field_gpmPayBankBinHint',
    },
    { key: 'accountNumber', label: 'Account Number', sensitive: false },
    {
      key: 'allowSimulated',
      label: 'Allow Simulated',
      sensitive: false,
      defaultValue: 'false',
      hintKey: 'admin.settings.payment.field_gpmPayAllowSimulatedHint',
      options: [
        { value: 'false', label: 'No' },
        { value: 'true', label: 'Yes' },
      ],
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

/** Resolve a payment type to its display option, falling back to the raw value. */
export function resolveTypeLabel(typeVal: string, allTypes: TypeOption[]): TypeOption {
  return allTypes.find(pt => pt.value === typeVal) || { value: typeVal, label: typeVal }
}

/** Get available type options for a provider key. */
export function getAvailableTypes(providerKey: string, allTypes: TypeOption[]): TypeOption[] {
  const types = PROVIDER_SUPPORTED_TYPES[providerKey] || []
  return types.map(t => resolveTypeLabel(t, allTypes))
}

/**
 * Report whether a gateway has at least one of its payment methods enabled.
 *
 * The gateway key (`sepay`) and its methods (`sepay_bank_transfer`, ...) are
 * separate identifiers, so a provider key must never be looked up directly in
 * the enabled-types list — that check silently yields nothing.
 */
export function isProviderKeyEnabled(providerKey: string, enabledTypes: string[]): boolean {
  const types = PROVIDER_SUPPORTED_TYPES[providerKey] || []
  return types.some(type => enabledTypes.includes(type))
}

/** Extract base URL from a full callback URL by removing the known path suffix. */
export function extractBaseUrl(fullUrl: string, path: string): string {
  if (!fullUrl) return ''
  if (fullUrl.endsWith(path)) return fullUrl.slice(0, -path.length)
  // Fallback: try to extract origin
  try { return new URL(fullUrl).origin } catch { return fullUrl }
}
