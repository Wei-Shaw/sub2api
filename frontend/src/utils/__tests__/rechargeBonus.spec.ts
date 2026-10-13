import { describe, expect, it } from 'vitest'

import {
  calculateRechargeBonus,
  describeRechargeBonusIntervals,
  formatRechargeBonusNumber,
  isDuplicateRechargeBonusMinAmount,
  isRechargeBonusPercentValidForMode,
  matchRechargeBonusTier,
  normalizeRechargeBonusMode,
  normalizeRechargeBonusTiers,
  quoteRechargeBonus,
  sanitizeRechargeBonusTiersForSubmit,
} from '../rechargeBonus'

const tiers = [
  { min_amount: 100, bonus_percent: 20 },
  { min_amount: 500, bonus_percent: 30 },
  { min_amount: 1000, bonus_percent: 35 },
]

describe('rechargeBonus helpers', () => {
  it('normalizes settings payload: drops invalid, dedupes, sorts ascending', () => {
    expect(
      normalizeRechargeBonusTiers([
        { min_amount: 500, bonus_percent: 30 },
        { min_amount: -1, bonus_percent: 5 },
        { min_amount: 100, bonus_percent: 20 },
        { min_amount: '100', bonus_percent: 99 },
        { min_amount: 50, bonus_percent: 5000 },
        { min_amount: 10.123, bonus_percent: 1 },
        null,
        'junk',
      ]),
    ).toEqual([
      { min_amount: 100, bonus_percent: 20 },
      { min_amount: 500, bonus_percent: 30 },
    ])
    expect(normalizeRechargeBonusTiers(undefined)).toEqual([])
  })

  it('sanitizes drafts for submit: incomplete rows are dropped', () => {
    expect(
      sanitizeRechargeBonusTiersForSubmit([
        { min_amount: 1000, bonus_percent: 35 },
        { min_amount: null, bonus_percent: 10 },
        { min_amount: 100, bonus_percent: null },
        { min_amount: 100, bonus_percent: 20 },
        { min_amount: 0, bonus_percent: 0 },
      ]),
    ).toEqual([
      { min_amount: 0, bonus_percent: 0 },
      { min_amount: 100, bonus_percent: 20 },
      { min_amount: 1000, bonus_percent: 35 },
    ])
    expect(sanitizeRechargeBonusTiersForSubmit(null)).toEqual([])
  })

  it('detects duplicate thresholds across rows', () => {
    const drafts = [
      { min_amount: 100, bonus_percent: 20 },
      { min_amount: 100, bonus_percent: 30 },
      { min_amount: null, bonus_percent: 30 },
    ]
    expect(isDuplicateRechargeBonusMinAmount(drafts, 1)).toBe(true)
    expect(isDuplicateRechargeBonusMinAmount(drafts, 2)).toBe(false)
  })

  it('matches the highest threshold not above the payment amount', () => {
    expect(matchRechargeBonusTier(tiers, 99.99)).toBeNull()
    expect(matchRechargeBonusTier(tiers, 100)?.bonus_percent).toBe(20)
    expect(matchRechargeBonusTier(tiers, 499.99)?.bonus_percent).toBe(20)
    expect(matchRechargeBonusTier(tiers, 500)?.bonus_percent).toBe(30)
    expect(matchRechargeBonusTier(tiers, 5000)?.bonus_percent).toBe(35)
    expect(matchRechargeBonusTier(tiers, 0)).toBeNull()
    expect(matchRechargeBonusTier([{ min_amount: 0.3, bonus_percent: 1 }], 0.1 + 0.2)).not.toBeNull()
  })

  it('calculates bonus on the credited base with cent rounding', () => {
    expect(calculateRechargeBonus(100, 20)).toBe(20)
    expect(calculateRechargeBonus(33.33, 15)).toBe(5)
    expect(calculateRechargeBonus(100, 0)).toBe(0)
  })

  it('quotes threshold by payment amount and bonus by credited base', () => {
    expect(quoteRechargeBonus(tiers, 100)).toMatchObject({ percent: 20, base: 100, bonus: 20, credited: 120 })
    // 1000 CNY × 0.14 = 140 USD base; threshold matched by the entered 1000 → 35%
    expect(quoteRechargeBonus(tiers, 1000, 0.14)).toMatchObject({ percent: 35, base: 140, bonus: 49, credited: 189 })
    // 700 CNY → 98 USD base; matched by 700 → 30%
    expect(quoteRechargeBonus(tiers, 700, 0.14)).toMatchObject({ percent: 30, base: 98, bonus: 29.4, credited: 127.4 })
    expect(quoteRechargeBonus(tiers, 50)).toMatchObject({ percent: 0, base: 50, bonus: 0, credited: 50, tier: null })
    expect(quoteRechargeBonus([], 100)).toMatchObject({ percent: 0, bonus: 0, credited: 100 })
  })

  it('quotes discount mode: credit stays, pay base shrinks, free part recorded as bonus', () => {
    expect(quoteRechargeBonus(tiers, 500, { mode: 'discount' })).toMatchObject({
      mode: 'discount', percent: 30, payBase: 350, base: 500, bonus: 150, credited: 500,
    })
    // 倍率 0.14：1000 CNY 到账 140 USD，35% off 实付 650 CNY，免费部分 140 − 91 = 49 USD
    expect(quoteRechargeBonus(tiers, 1000, { mode: 'discount', multiplier: 0.14 })).toMatchObject({
      percent: 35, payBase: 650, base: 140, bonus: 49, credited: 140,
    })
    // 币种精度：JPY 无小数
    expect(quoteRechargeBonus([{ min_amount: 1, bonus_percent: 15 }], 101, { mode: 'discount' }).payBase).toBe(85.85)
    expect(quoteRechargeBonus([{ min_amount: 1, bonus_percent: 15 }], 101, { mode: 'discount', currencyDigits: 0 }).payBase).toBe(86)
    // 折扣 ≥ 100% 视为无优惠（fail-safe）
    expect(quoteRechargeBonus([{ min_amount: 1, bonus_percent: 100 }], 100, { mode: 'discount' })).toMatchObject({ percent: 0, payBase: 100, credited: 100 })
    // 未命中
    expect(quoteRechargeBonus(tiers, 50, { mode: 'discount' })).toMatchObject({ percent: 0, payBase: 50, credited: 50, bonus: 0 })
  })

  it('normalizes mode and validates percent per mode', () => {
    expect(normalizeRechargeBonusMode('discount')).toBe('discount')
    expect(normalizeRechargeBonusMode(' Discount ')).toBe('discount')
    expect(normalizeRechargeBonusMode('bonus')).toBe('bonus')
    expect(normalizeRechargeBonusMode(undefined)).toBe('bonus')
    expect(normalizeRechargeBonusMode('junk')).toBe('bonus')
    expect(isRechargeBonusPercentValidForMode(100, 'bonus')).toBe(true)
    expect(isRechargeBonusPercentValidForMode(100, 'discount')).toBe(false)
    expect(isRechargeBonusPercentValidForMode(99.99, 'discount')).toBe(true)
  })

  it('describes intervals with an implicit no-bonus head segment', () => {
    expect(describeRechargeBonusIntervals(tiers)).toEqual([
      { from: 0, to: 100, percent: 0 },
      { from: 100, to: 500, percent: 20 },
      { from: 500, to: 1000, percent: 30 },
      { from: 1000, to: null, percent: 35 },
    ])
    expect(describeRechargeBonusIntervals([{ min_amount: 0, bonus_percent: 5 }])).toEqual([
      { from: 0, to: null, percent: 5 },
    ])
    expect(describeRechargeBonusIntervals([])).toEqual([])
  })

  it('formats numbers without trailing zeros', () => {
    expect(formatRechargeBonusNumber(20)).toBe('20')
    expect(formatRechargeBonusNumber(12.5)).toBe('12.5')
    expect(formatRechargeBonusNumber(100.1)).toBe('100.1')
  })
})
