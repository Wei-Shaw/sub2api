import { describe, expect, it } from 'vitest'
import {
  GPMPAY_BANK_TRANSFER,
  METHOD_ORDER,
  NOWPAYMENTS_CRYPTO,
  PAYMENT_CURRENCY_OPTIONS,
  PROVIDER_CONFIG_FIELDS,
  PROVIDER_GPMPAY,
  PROVIDER_ENV_OPTIONS,
  PROVIDER_NOWPAYMENTS,
  PROVIDER_SUPPORTED_TYPES,
  WEBHOOK_PATHS,
  extractBaseUrl,
  getAvailableTypes,
  isProviderKeyEnabled,
} from '@/components/payment/providerConfig'

function findField(providerKey: string, key: string) {
  const fields = PROVIDER_CONFIG_FIELDS[providerKey] || []
  return fields.find(field => field.key === key)
}

describe('PROVIDER_CONFIG_FIELDS.nowpayments', () => {
  it('marks the API key and the IPN secret as sensitive', () => {
    // Must stay in sync with providerSensitiveConfigFields in
    // internal/service/payment_config_providers.go.
    expect(findField(PROVIDER_NOWPAYMENTS, 'apiKey')?.sensitive).toBe(true)
    expect(findField(PROVIDER_NOWPAYMENTS, 'ipnSecretKey')?.sensitive).toBe(true)
    expect(findField(PROVIDER_NOWPAYMENTS, 'env')?.sensitive).toBe(false)
    expect(findField(PROVIDER_NOWPAYMENTS, 'currency')?.sensitive).toBe(false)
  })

  it('requires the IPN secret', () => {
    // The signature is the only proof a callback is genuine, and the backend
    // constructor rejects a blank one.
    expect(findField(PROVIDER_NOWPAYMENTS, 'ipnSecretKey')?.optional).toBeFalsy()
    expect(findField(PROVIDER_NOWPAYMENTS, 'apiKey')?.optional).toBeFalsy()
    expect(findField(PROVIDER_NOWPAYMENTS, 'payCurrency')?.optional).toBe(true)
  })

  it('defaults to production and USD', () => {
    expect(findField(PROVIDER_NOWPAYMENTS, 'env')?.defaultValue).toBe('production')
    expect(findField(PROVIDER_NOWPAYMENTS, 'env')?.options).toBe(PROVIDER_ENV_OPTIONS)
    expect(findField(PROVIDER_NOWPAYMENTS, 'currency')?.defaultValue).toBe('USD')
    expect(findField(PROVIDER_NOWPAYMENTS, 'currency')?.options).toBe(PAYMENT_CURRENCY_OPTIONS)
  })
})

describe('supported payment types', () => {
  it('exposes exactly one method per gateway', () => {
    expect(PROVIDER_SUPPORTED_TYPES[PROVIDER_NOWPAYMENTS]).toEqual([NOWPAYMENTS_CRYPTO])
    expect(PROVIDER_SUPPORTED_TYPES[PROVIDER_GPMPAY]).toEqual([GPMPAY_BANK_TRANSFER])
    expect([...METHOD_ORDER]).toEqual([GPMPAY_BANK_TRANSFER, NOWPAYMENTS_CRYPTO])
  })

  it('registers no removed gateway', () => {
    expect(Object.keys(PROVIDER_SUPPORTED_TYPES)).toEqual([PROVIDER_NOWPAYMENTS, PROVIDER_GPMPAY])
    expect(Object.keys(WEBHOOK_PATHS)).toEqual([PROVIDER_NOWPAYMENTS, PROVIDER_GPMPAY])
    expect(Object.keys(PROVIDER_CONFIG_FIELDS)).toEqual([PROVIDER_NOWPAYMENTS, PROVIDER_GPMPAY])
    expect(WEBHOOK_PATHS[PROVIDER_NOWPAYMENTS]).toBe('/api/v1/payment/webhook/nowpayments')
    expect(WEBHOOK_PATHS[PROVIDER_GPMPAY]).toBe('/api/v1/payment/webhook/gpmpay')
  })

  it('falls back to the raw value when a type has no supplied label', () => {
    expect(getAvailableTypes(PROVIDER_GPMPAY, [])).toEqual([
      { value: GPMPAY_BANK_TRANSFER, label: GPMPAY_BANK_TRANSFER },
    ])
    expect(getAvailableTypes(PROVIDER_GPMPAY, [{ value: GPMPAY_BANK_TRANSFER, label: 'VietQR' }])).toEqual([
      { value: GPMPAY_BANK_TRANSFER, label: 'VietQR' },
    ])
  })

  it('returns nothing for an unknown provider key', () => {
    expect(getAvailableTypes('stripe', [])).toEqual([])
  })
})

describe('extractBaseUrl', () => {
  it('strips the known callback path', () => {
    expect(extractBaseUrl('https://panel.example.com/api/v1/payment/webhook/gpmpay', '/api/v1/payment/webhook/gpmpay'))
      .toBe('https://panel.example.com')
  })

  it('falls back to the origin when the path does not match', () => {
    expect(extractBaseUrl('https://panel.example.com/other', '/api/v1/payment/webhook/gpmpay'))
      .toBe('https://panel.example.com')
  })

  it('returns an empty string for an empty URL', () => {
    expect(extractBaseUrl('', '/api/v1/payment/webhook/gpmpay')).toBe('')
  })
})

describe('isProviderKeyEnabled', () => {
  it('matches on the gateway methods, not the gateway key', () => {
    // The admin stores enabled *methods*; looking the provider key itself up in
    // that list finds nothing and empties the "Add Provider" dropdown.
    expect(isProviderKeyEnabled(PROVIDER_GPMPAY, [GPMPAY_BANK_TRANSFER])).toBe(true)
    expect(isProviderKeyEnabled(PROVIDER_GPMPAY, [NOWPAYMENTS_CRYPTO, GPMPAY_BANK_TRANSFER])).toBe(true)
    expect(isProviderKeyEnabled(PROVIDER_GPMPAY, [PROVIDER_GPMPAY])).toBe(false)
  })

  it('is false when nothing is enabled or the gateway is unknown', () => {
    expect(isProviderKeyEnabled(PROVIDER_GPMPAY, [])).toBe(false)
    expect(isProviderKeyEnabled('stripe', [GPMPAY_BANK_TRANSFER])).toBe(false)
  })
})
