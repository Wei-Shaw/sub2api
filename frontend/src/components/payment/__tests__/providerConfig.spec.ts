import { describe, expect, it } from 'vitest'
import {
  GPMPAY_BANK_TRANSFER,
  METHOD_ORDER,
  NOWPAYMENTS_CRYPTO,
  PAYMENT_CURRENCY_OPTIONS,
  PROVIDER_CONFIG_FIELDS,
  PROVIDER_GPMPAY,
  PROVIDER_NOWPAYMENTS,
  PROVIDER_SEPAY,
  PROVIDER_SUPPORTED_TYPES,
  SEPAY_BANK_TRANSFER,
  SEPAY_ENV_OPTIONS,
  WEBHOOK_PATHS,
  extractBaseUrl,
  getAvailableTypes,
  isProviderKeyEnabled,
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

  it('requires the merchant credentials but leaves the IPN key optional', () => {
    // ipnSecretKey only exists when the merchant portal sets IPN auth type to
    // SECRET_KEY; forcing it would block an otherwise valid configuration.
    for (const key of ['merchantId', 'secretKey', 'env', 'currency']) {
      expect(findField(PROVIDER_SEPAY, key)?.optional).toBeFalsy()
    }
    expect(findField(PROVIDER_SEPAY, 'ipnSecretKey')?.optional).toBe(true)
    expect(findField(PROVIDER_SEPAY, 'ipnSecretKey')?.sensitive).toBe(true)
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
    // Unlike SePay's, it is not optional: the signature is the only proof a
    // callback is genuine, and the backend constructor rejects a blank one.
    expect(findField(PROVIDER_NOWPAYMENTS, 'ipnSecretKey')?.optional).toBeFalsy()
    expect(findField(PROVIDER_NOWPAYMENTS, 'apiKey')?.optional).toBeFalsy()
    expect(findField(PROVIDER_NOWPAYMENTS, 'payCurrency')?.optional).toBe(true)
  })

  it('defaults to production and USD', () => {
    expect(findField(PROVIDER_NOWPAYMENTS, 'env')?.defaultValue).toBe('production')
    expect(findField(PROVIDER_NOWPAYMENTS, 'currency')?.defaultValue).toBe('USD')
  })
})

describe('supported payment types', () => {
  it('exposes exactly the three SePay methods', () => {
    expect(PROVIDER_SUPPORTED_TYPES[PROVIDER_SEPAY]).toEqual([
      SEPAY_BANK_TRANSFER,
            ])
    expect(PROVIDER_SUPPORTED_TYPES[PROVIDER_NOWPAYMENTS]).toEqual([NOWPAYMENTS_CRYPTO])
    expect(PROVIDER_SUPPORTED_TYPES[PROVIDER_GPMPAY]).toEqual([GPMPAY_BANK_TRANSFER])
    expect([...METHOD_ORDER]).toEqual([
      SEPAY_BANK_TRANSFER,
      GPMPAY_BANK_TRANSFER,
              NOWPAYMENTS_CRYPTO,
    ])
  })

  it('registers no removed gateway', () => {
    expect(Object.keys(PROVIDER_SUPPORTED_TYPES)).toEqual([PROVIDER_SEPAY, PROVIDER_NOWPAYMENTS, PROVIDER_GPMPAY])
    expect(Object.keys(WEBHOOK_PATHS)).toEqual([PROVIDER_SEPAY, PROVIDER_NOWPAYMENTS, PROVIDER_GPMPAY])
    expect(WEBHOOK_PATHS[PROVIDER_SEPAY]).toBe('/api/v1/payment/webhook/sepay')
    expect(WEBHOOK_PATHS[PROVIDER_NOWPAYMENTS]).toBe('/api/v1/payment/webhook/nowpayments')
    expect(WEBHOOK_PATHS[PROVIDER_GPMPAY]).toBe('/api/v1/payment/webhook/gpmpay')
  })

  it('falls back to the raw value when a type has no supplied label', () => {
    expect(getAvailableTypes(PROVIDER_SEPAY, [])).toEqual([
      { value: SEPAY_BANK_TRANSFER, label: SEPAY_BANK_TRANSFER },
    ])
    expect(getAvailableTypes(PROVIDER_SEPAY, [{ value: SEPAY_BANK_TRANSFER, label: 'VietQR' }])).toEqual([
      { value: SEPAY_BANK_TRANSFER, label: 'VietQR' },
    ])
  })

  it('returns nothing for an unknown provider key', () => {
    expect(getAvailableTypes('stripe', [])).toEqual([])
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

describe('isProviderKeyEnabled', () => {
  it('matches on the gateway methods, not the gateway key', () => {
    // The admin stores enabled *methods*; looking the provider key itself up in
    // that list finds nothing and empties the "Add Provider" dropdown.
    expect(isProviderKeyEnabled(PROVIDER_SEPAY, [SEPAY_BANK_TRANSFER])).toBe(true)
    expect(isProviderKeyEnabled(PROVIDER_SEPAY, ['nowpayments_crypto', SEPAY_BANK_TRANSFER])).toBe(true)
    expect(isProviderKeyEnabled(PROVIDER_SEPAY, [PROVIDER_SEPAY])).toBe(false)
  })

  it('is false when nothing is enabled or the gateway is unknown', () => {
    expect(isProviderKeyEnabled(PROVIDER_SEPAY, [])).toBe(false)
    expect(isProviderKeyEnabled('stripe', [SEPAY_BANK_TRANSFER])).toBe(false)
  })
})
