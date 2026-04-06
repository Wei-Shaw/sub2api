/**
 * Shared constants and types for payment provider management.
 */

// --- Types ---

export interface ConfigFieldDef {
  key: string
  label: string
  sensitive: boolean
  optional?: boolean
  defaultValue?: string
}

export interface TypeOption {
  value: string
  label: string
}

// --- Constants ---

/** Maps provider key → available payment types. */
export const PROVIDER_SUPPORTED_TYPES: Record<string, string[]> = {
  easypay: ['easypay', 'alipay', 'wxpay'],
  alipay: ['alipay'],
  wxpay: ['wxpay'],
  stripe: ['stripe'],
}

/** Webhook paths for each provider (relative to origin). */
export const WEBHOOK_PATHS: Record<string, string> = {
  easypay: '/api/v1/payment/webhook/easypay',
  alipay: '/api/v1/payment/webhook/alipay',
  wxpay: '/api/v1/payment/webhook/wxpay',
  stripe: '/api/v1/payment/webhook/stripe',
}

export const RETURN_PATH = '/payment/result'

const baseURL = typeof window !== 'undefined' ? window.location.origin : ''

/** Per-provider config fields with defaults. */
export const PROVIDER_CONFIG_FIELDS: Record<string, ConfigFieldDef[]> = {
  easypay: [
    { key: 'pid', label: 'PID', sensitive: false },
    { key: 'pkey', label: 'PKey', sensitive: true },
    { key: 'apiBase', label: '', sensitive: false },
    { key: 'notifyUrl', label: '', sensitive: false, defaultValue: baseURL + WEBHOOK_PATHS.easypay },
    { key: 'returnUrl', label: '', sensitive: false, defaultValue: baseURL + RETURN_PATH },
    { key: 'cidAlipay', label: '', sensitive: false, optional: true },
    { key: 'cidWxpay', label: '', sensitive: false, optional: true },
  ],
  alipay: [
    { key: 'appId', label: 'App ID', sensitive: false },
    { key: 'privateKey', label: '', sensitive: true },
    { key: 'publicKey', label: '', sensitive: true },
    { key: 'notifyUrl', label: '', sensitive: false, defaultValue: baseURL + WEBHOOK_PATHS.alipay },
    { key: 'returnUrl', label: '', sensitive: false, defaultValue: baseURL + RETURN_PATH },
  ],
  wxpay: [
    { key: 'appId', label: 'App ID', sensitive: false },
    { key: 'mchId', label: '', sensitive: false },
    { key: 'privateKey', label: '', sensitive: true },
    { key: 'apiV3Key', label: '', sensitive: true },
    { key: 'publicKey', label: '', sensitive: true },
    { key: 'publicKeyId', label: '', sensitive: false, optional: true },
    { key: 'certSerial', label: '', sensitive: false, optional: true },
    { key: 'notifyUrl', label: '', sensitive: false, defaultValue: baseURL + WEBHOOK_PATHS.wxpay },
  ],
  stripe: [
    { key: 'secretKey', label: '', sensitive: true },
    { key: 'publishableKey', label: '', sensitive: false },
    { key: 'webhookSecret', label: '', sensitive: true },
  ],
}

// --- Helpers ---

/** Parse comma-separated types string into array. */
export function parseTypes(raw: string): string[] {
  return raw.split(',').map(s => s.trim()).filter(Boolean)
}

/** Resolve type label: for easypay provider, show "跳转" for the easypay type. */
export function resolveTypeLabel(
  typeVal: string,
  providerKey: string,
  allTypes: TypeOption[],
  redirectLabel: string,
): TypeOption {
  if (typeVal === 'easypay' && providerKey === 'easypay') {
    return { value: typeVal, label: redirectLabel }
  }
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
