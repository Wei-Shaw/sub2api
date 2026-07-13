import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

const requiredKeys = [
  'admin.users.columns.extraConcurrency',
  'admin.users.form.extraConcurrencyDefaultPlaceholder',
  'admin.users.extraConcurrencyMin',
  'admin.users.typeAdminExtraConcurrency',
  'admin.redeem.types.admin_extra_concurrency',
  'admin.settings.defaults.defaultExtraConcurrency',
  'admin.settings.defaults.defaultExtraConcurrencyHint',
  'admin.settings.extraConcurrency.title',
  'admin.settings.extraConcurrency.description',
  'admin.settings.extraConcurrency.enabled',
  'admin.settings.extraConcurrency.enabledHint',
  'admin.settings.extraConcurrency.waitTimeout',
  'admin.settings.extraConcurrency.reservePercent',
  'admin.settings.extraConcurrency.minReservedSlots',
  'admin.settings.extraConcurrency.platformOverrides',
  'admin.settings.extraConcurrency.platformOverridesHint',
  'admin.settings.extraConcurrency.platform',
  'admin.settings.extraConcurrency.reservePercentOverride',
  'admin.settings.extraConcurrency.minReservedSlotsOverride',
  'admin.settings.extraConcurrency.inherit',
  'admin.settings.extraConcurrency.defaultExtraConcurrencyRangeError',
  'admin.settings.extraConcurrency.waitTimeoutRangeError',
  'admin.settings.extraConcurrency.reservePercentRangeError',
  'admin.settings.extraConcurrency.minReservedSlotsRangeError',
  'admin.settings.extraConcurrency.platformReservePercentRangeError',
  'admin.settings.extraConcurrency.platformMinReservedSlotsRangeError',
  'profile.standardConcurrencyLimit',
  'profile.extraConcurrencyLimit',
  'redeem.standardConcurrency',
  'redeem.extraConcurrency',
  'redeem.extraConcurrencyAddedAdmin',
  'redeem.extraConcurrencyReducedAdmin',
] as const

function getMessage(messages: Record<string, unknown>, path: string): unknown {
  return path.split('.').reduce<unknown>((value, key) => {
    if (!value || typeof value !== 'object') return undefined
    return (value as Record<string, unknown>)[key]
  }, messages)
}

describe.each([
  ['en', en],
  ['zh', zh],
] as const)('extra concurrency locale completeness (%s)', (_locale, messages) => {
  for (const key of requiredKeys) {
    it(`defines ${key}`, () => {
      expect(getMessage(messages, key)).toEqual(expect.any(String))
      expect(getMessage(messages, key)).not.toBe('')
    })
  }
})

describe('standard concurrency wording', () => {
  it('keeps redeem-code concurrency scoped to the standard allowance', () => {
    expect(en.redeem.concurrencyAddedRedeem).toContain('Standard')
    expect(zh.redeem.concurrencyAddedRedeem).toContain('标准')
    expect(en.admin.users.columns.concurrency).toContain('Standard')
    expect(zh.admin.users.columns.concurrency).toContain('标准')
    expect(en.admin.redeem.types.concurrency).toContain('Standard')
    expect(zh.admin.redeem.types.concurrency).toContain('标准')
    expect(en.admin.redeem.types.admin_concurrency).toContain('Standard')
    expect(zh.admin.redeem.types.admin_concurrency).toContain('标准')
    expect(en.admin.redeem.concurrency).toContain('Standard')
    expect(zh.admin.redeem.concurrency).toContain('标准')
    expect(en.admin.settings.defaults.defaultConcurrency).toContain('Standard')
    expect(zh.admin.settings.defaults.defaultConcurrency).toContain('标准')
    expect(en.admin.settings.defaults.defaultConcurrencyHint).toContain('standard')
    expect(zh.admin.settings.defaults.defaultConcurrencyHint).toContain('标准')
  })
})
