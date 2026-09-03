import { describe, expect, it } from 'vitest'
import {
  METHOD_ORDER,
  PAYMENT_CURRENCY_OPTIONS,
  PROVIDER_CONFIG_FIELDS,
  PROVIDER_SEPAY,
  PROVIDER_SUPPORTED_TYPES,
  SEPAY_BANK_TRANSFER,
  SEPAY_CARD,
  SEPAY_ENV_OPTIONS,
  SEPAY_NAPAS,
  WEBHOOK_PATHS,
  extractBaseUrl,
  getAvailableTypes,
} from '@/components/payment/providerConfig'

function findField(providerKey: string, key: string) {
  const fields = PROVIDER_CONFIG_FIELDS[providerKey] || []
  return fields.find(field => field.key === key)
}

describe('PROVIDER_CONFIG_FIELDS.sepay', () => {
  it('marks only the merchant secret as sensitive', () => {
    // Must stay in sync with providerSensitiveConfigFields in
    // internal/service/payment_config_providers.go — a field marked sensitive
    // here but not there would be echoed back by the admin GET API.
    expect(findField(PROVIDER_SEPAY, 'secretKey')?.sensitive).toBe(true)
    expect(findField(PROVIDER_SEPAY, 'merchantId')?.sensitive).toBe(false)
    expect(findField(PROVIDER_SEPAY, 'env')?.sensitive).toBe(false)
    expect(findField(PROVIDER_SEPAY, 'currency')?.sensitive).toBe(false)
  })

  it('requires every credential field', () => {
    for (const field of PROVIDER_CONFIG_FIELDS[PROVIDER_SEPAY]) {
      expect(field.optional).toBeFalsy()
    }
  })

  it('defaults to the production environment and VND', () => {
    expect(findField(PROVIDER_SEPAY, 'env')?.defaultValue).toBe('production')
    expect(findField(PROVIDER_SEPAY, 'env')?.options).toBe(SEPAY_ENV_OPTIONS)

    const currency = findField(PROVIDER_SEPAY, 'currency')
    expect(currency?.defaultValue).toBe('VND')
    expect(currency?.hintKey).toBe('admin.settings.payment.field_paymentCurrencyHint')
    expect(currency?.options).toBe(PAYMENT_CURRENCY_OPTIONS)
  })

  it('drops every credential field of the removed gateways', () => {
    for (const key of ['pkey', 'pid', 'privateKey', 'apiV3Key', 'publishableKey', 'webhookSecret', 'accountId']) {
      expect(findField(PROVIDER_SEPAY, key)).toBeUndefined()
    }
  })
})

describe('supported payment types', () => {
  it('exposes exactly the three SePay methods', () => {
    expect(PROVIDER_SUPPORTED_TYPES[PROVIDER_SEPAY]).toEqual([
      SEPAY_BANK_TRANSFER,
      SEPAY_NAPAS,
      SEPAY_CARD,
    ])
    expect([...METHOD_ORDER]).toEqual([SEPAY_BANK_TRANSFER, SEPAY_NAPAS, SEPAY_CARD])
  })

  it('registers no removed gateway', () => {
    expect(Object.keys(PROVIDER_SUPPORTED_TYPES)).toEqual([PROVIDER_SEPAY])
    expect(Object.keys(WEBHOOK_PATHS)).toEqual([PROVIDER_SEPAY])
    expect(WEBHOOK_PATHS[PROVIDER_SEPAY]).toBe('/api/v1/payment/webhook/sepay')
  })

  it('falls back to the raw value when a type has no supplied label', () => {
    const types = getAvailableTypes(PROVIDER_SEPAY, [{ value: SEPAY_NAPAS, label: 'Napas' }], 'Redirect')

    expect(types).toEqual([
      { value: SEPAY_BANK_TRANSFER, label: SEPAY_BANK_TRANSFER },
      { value: SEPAY_NAPAS, label: 'Napas' },
      { value: SEPAY_CARD, label: SEPAY_CARD },
    ])
  })

  it('returns nothing for an unknown provider key', () => {
    expect(getAvailableTypes('stripe', [], 'Redirect')).toEqual([])
  })
})

describe('extractBaseUrl', () => {
  it('strips the known callback path', () => {
    expect(extractBaseUrl('https://panel.example.com/api/v1/payment/webhook/sepay', '/api/v1/payment/webhook/sepay'))
      .toBe('https://panel.example.com')
  })

  it('falls back to the origin when the path does not match', () => {
    expect(extractBaseUrl('https://panel.example.com/other', '/api/v1/payment/webhook/sepay'))
      .toBe('https://panel.example.com')
  })

  it('returns an empty string for an empty URL', () => {
    expect(extractBaseUrl('', '/api/v1/payment/webhook/sepay')).toBe('')
  })
})
