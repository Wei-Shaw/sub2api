import { describe, expect, it } from 'vitest'

import {
  getPaymentVisibleMethodSourceOptions,
  normalizePaymentVisibleMethodSource,
} from '@/api/admin/settings'

describe('admin settings payment visible method helpers', () => {
  it('normalizes aliases into canonical source keys per visible method', () => {
    expect(normalizePaymentVisibleMethodSource('alipay', 'official')).toBe('official_alipay')
    expect(normalizePaymentVisibleMethodSource('alipay', 'alipay_direct')).toBe('official_alipay')
    expect(normalizePaymentVisibleMethodSource('alipay', 'easypay')).toBe('easypay_alipay')

    expect(normalizePaymentVisibleMethodSource('wxpay', 'official')).toBe('official_wxpay')
    expect(normalizePaymentVisibleMethodSource('wxpay', 'wechat')).toBe('official_wxpay')
    expect(normalizePaymentVisibleMethodSource('wxpay', 'easypay')).toBe('easypay_wxpay')
  })

  it('rejects unknown or cross-method source values', () => {
    expect(normalizePaymentVisibleMethodSource('alipay', 'official_wxpay')).toBe('')
    expect(normalizePaymentVisibleMethodSource('wxpay', 'official_alipay')).toBe('')
    expect(normalizePaymentVisibleMethodSource('alipay', 'unknown')).toBe('')
    expect(normalizePaymentVisibleMethodSource('wxpay', null)).toBe('')
  })

  it('exposes method-scoped source options instead of arbitrary strings', () => {
    expect(getPaymentVisibleMethodSourceOptions('alipay')).toEqual([
      {
        value: '',
        labelKey: 'admin.settings.paymentVisibleMethods.sources.notConfigured',
        labelEn: 'Not configured',
      },
      {
        value: 'official_alipay',
        labelKey: 'admin.settings.paymentVisibleMethods.sources.officialAlipay',
        labelEn: 'Official Alipay',
      },
      {
        value: 'easypay_alipay',
        labelKey: 'admin.settings.paymentVisibleMethods.sources.easypayAlipay',
        labelEn: 'EasyPay Alipay',
      },
    ])

    expect(getPaymentVisibleMethodSourceOptions('wxpay')).toEqual([
      {
        value: '',
        labelKey: 'admin.settings.paymentVisibleMethods.sources.notConfigured',
        labelEn: 'Not configured',
      },
      {
        value: 'official_wxpay',
        labelKey: 'admin.settings.paymentVisibleMethods.sources.officialWxpay',
        labelEn: 'Official WeChat Pay',
      },
      {
        value: 'easypay_wxpay',
        labelKey: 'admin.settings.paymentVisibleMethods.sources.easypayWxpay',
        labelEn: 'EasyPay WeChat Pay',
      },
    ])
  })
})
