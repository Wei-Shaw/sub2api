import { describe, expect, it } from 'vitest'

import {
  ceilMoney,
  currencyMinorUnits,
  formatMoney,
  ledgerAmountFromPayment,
  paymentAmountFromLedger,
  roundMoney,
} from '@/utils/money'

describe('money utils', () => {
  it('uses zero-decimal formatting for VND and KRW', () => {
    expect(currencyMinorUnits('VND')).toBe(0)
    expect(currencyMinorUnits('KRW')).toBe(0)
    expect(formatMoney(200000, 'VND')).toBe('₫200,000')
    expect(formatMoney(12000, 'KRW')).toBe('₩12,000')
  })

  it('rounds and ceils using currency minor units', () => {
    expect(roundMoney(7.836, 'USD')).toBe(7.84)
    expect(ceilMoney(7.831, 'USD')).toBe(7.84)
    expect(roundMoney(200000.4, 'VND')).toBe(200000)
    expect(ceilMoney(200000.1, 'VND')).toBe(200001)
  })

  it('converts payment currency to USD ledger preview by FX snapshot rate', () => {
    expect(ledgerAmountFromPayment(200000, 'VND', 'USD', { VND: 1 / 25500 })).toBe(7.84)
    expect(paymentAmountFromLedger(7.84, 'VND', 'USD', { VND: 1 / 25500 })).toBe(199920)
  })
})
