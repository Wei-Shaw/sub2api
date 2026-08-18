import { describe, expect, it } from 'vitest'
import {
  INFINI_CURRENCIES,
  INFINI_CURRENCY_OPTIONS,
  METHOD_ORDER,
  PAYMENT_CURRENCY_OPTIONS,
  PROVIDER_CALLBACK_PATHS,
  PROVIDER_CONFIG_FIELDS,
  PROVIDER_SUPPORTED_TYPES,
  PROVIDER_TOGGLE_OPTIONS,
  providerSupportsRefund,
  WEBHOOK_PATHS,
  isBuiltInAlipayMethod,
  isBuiltInWxpayMethod,
  parseEasyPayCustomMethods,
  serializeEasyPayCustomMethods,
} from '@/components/payment/providerConfig'

function findField(providerKey: string, key: string) {
  const fields = PROVIDER_CONFIG_FIELDS[providerKey] || []
  return fields.find(field => field.key === key)
}

describe('PROVIDER_CONFIG_FIELDS.wxpay', () => {
  it('keeps admin form validation aligned with backend-required credentials', () => {
    expect(findField('wxpay', 'publicKeyId')?.optional).toBeFalsy()
    expect(findField('wxpay', 'certSerial')?.optional).toBeFalsy()
  })

  it('only keeps the simplified visible credential set in the admin form', () => {
    expect(findField('wxpay', 'mpAppId')).toBeUndefined()
    expect(findField('wxpay', 'h5AppName')).toBeUndefined()
    expect(findField('wxpay', 'h5AppUrl')).toBeUndefined()
  })
})

describe('PROVIDER_CONFIG_FIELDS.airwallex', () => {
  it('adds currency config with CNY as the default', () => {
    const currency = findField('airwallex', 'currency')

    expect(currency?.defaultValue).toBe('CNY')
    expect(currency?.hintKey).toBe('admin.settings.payment.field_paymentCurrencyHint')
    expect(currency?.options).toBe(PAYMENT_CURRENCY_OPTIONS)
  })

  it('marks accountId as optional and explains when it can be left blank', () => {
    const accountId = findField('airwallex', 'accountId')

    expect(accountId?.optional).toBe(true)
    expect(accountId?.clearable).toBe(true)
    expect(accountId?.hintKey).toBe('admin.settings.payment.field_accountIdHint')
  })

  it('explains that apiBase must match the Airwallex key environment', () => {
    expect(findField('airwallex', 'apiBase')?.hintKey).toBe('admin.settings.payment.field_airwallexApiBaseHint')
  })
})

describe('PROVIDER_CONFIG_FIELDS.infini', () => {
  it('offers only the currencies Infini prices orders in, defaulting to USD', () => {
    const currency = findField('infini', 'currency')

    expect(currency?.defaultValue).toBe('USD')
    expect(currency?.options).toBe(INFINI_CURRENCY_OPTIONS)
    // Infini rejects CNY, so the shared default must not leak into this list.
    expect(INFINI_CURRENCY_OPTIONS.map(option => option.value)).toEqual(INFINI_CURRENCIES)
    expect(INFINI_CURRENCY_OPTIONS.map(option => option.value)).not.toContain('CNY')
    expect(INFINI_CURRENCY_OPTIONS.every(option => PAYMENT_CURRENCY_OPTIONS.includes(option))).toBe(true)
  })

  it('marks only the two secrets as sensitive', () => {
    const sensitive = (PROVIDER_CONFIG_FIELDS.infini || [])
      .filter(field => field.sensitive)
      .map(field => field.key)

    // Must stay in sync with providerSensitiveConfigFields in
    // backend/internal/service/payment_config_providers.go.
    expect(sensitive).toEqual(['secretKey', 'webhookSecret'])
  })

  it('requires every credential the backend constructor validates', () => {
    for (const key of ['keyId', 'secretKey', 'webhookSecret', 'apiBase']) {
      expect(findField('infini', key)).toBeDefined()
      expect(findField('infini', key)?.optional).toBeFalsy()
    }
    expect(findField('infini', 'apiBase')?.defaultValue).toBe('https://openapi.infini.money')
  })

  it('defaults payer email forwarding on and renders it as a toggle', () => {
    const forward = findField('infini', 'forwardPayerEmail')

    expect(forward?.defaultValue).toBe('true')
    expect(forward?.options).toBe(PROVIDER_TOGGLE_OPTIONS)
    expect(PROVIDER_TOGGLE_OPTIONS.map(option => option.value)).toEqual(['true', 'false'])
    expect(PROVIDER_TOGGLE_OPTIONS.every(option => typeof option.labelKey === 'string')).toBe(true)
  })

  it('declares that Infini cannot refund', () => {
    // Mirrors payment.ProviderSupportsRefund in the Go backend; Infini exposes
    // no merchant-initiated refund API.
    expect(providerSupportsRefund('infini')).toBe(false)
    expect(providerSupportsRefund('stripe')).toBe(true)
    expect(providerSupportsRefund('airwallex')).toBe(true)
    expect(providerSupportsRefund('easypay')).toBe(true)
  })

  it('wires the webhook path and leaves callback URLs to the Infini console', () => {
    expect(WEBHOOK_PATHS.infini).toBe('/api/v1/payment/webhook/infini')
    expect(PROVIDER_CALLBACK_PATHS.infini).toBeUndefined()
    expect(PROVIDER_SUPPORTED_TYPES.infini).toEqual(['infini'])
    expect(METHOD_ORDER).toContain('infini')
  })
})

describe('PROVIDER_CONFIG_FIELDS.stripe', () => {
  it('adds currency config with CNY as the default', () => {
    const currency = findField('stripe', 'currency')

    expect(currency?.defaultValue).toBe('CNY')
    expect(currency?.hintKey).toBe('admin.settings.payment.field_paymentCurrencyHint')
    expect(currency?.options).toBe(PAYMENT_CURRENCY_OPTIONS)
  })
})

describe('EasyPay custom methods config', () => {
  it('parses customMethods from the JSON string stored in provider config', () => {
    expect(parseEasyPayCustomMethods(
      '[{"type":"ldc","upstreamType":"epay","displayName":"LDC"},{"type":"usdt_trc20","upstreamType":"usdt","displayName":"USDT-TRC20"}]',
    )).toEqual([
      { type: 'ldc', upstreamType: 'epay', displayName: 'LDC' },
      { type: 'usdt_trc20', upstreamType: 'usdt', displayName: 'USDT-TRC20' },
    ])
  })

  it('serializes non-empty custom methods into the config string format', () => {
    expect(serializeEasyPayCustomMethods([
      { type: 'ldc', upstreamType: 'epay', displayName: 'LDC' },
      { type: '  ', upstreamType: 'ignored', displayName: 'Ignored' },
      { type: 'usdt_trc20', upstreamType: 'usdt', displayName: '' },
    ])).toBe('[{"type":"ldc","upstreamType":"epay","displayName":"LDC"},{"type":"usdt_trc20","upstreamType":"usdt","displayName":""}]')
  })

  it('returns an empty string for invalid or empty custom methods', () => {
    expect(parseEasyPayCustomMethods('not-json')).toEqual([])
    expect(serializeEasyPayCustomMethods([{ type: '', upstreamType: 'epay', displayName: 'LDC' }])).toBe('')
  })
})

describe('built-in payment method helpers', () => {
  it('only treats exact built-in aliases as Alipay or WeChat Pay', () => {
    expect(isBuiltInAlipayMethod('alipay')).toBe(true)
    expect(isBuiltInAlipayMethod('alipay_direct')).toBe(true)
    expect(isBuiltInAlipayMethod('card_alipay')).toBe(false)

    expect(isBuiltInWxpayMethod('wxpay')).toBe(true)
    expect(isBuiltInWxpayMethod('wxpay_direct')).toBe(true)
    expect(isBuiltInWxpayMethod('card_wxpay')).toBe(false)
  })
})
