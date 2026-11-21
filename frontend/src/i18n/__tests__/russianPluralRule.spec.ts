import { describe, expect, it } from 'vitest'

import { russianPluralRule } from '../index'

describe('russian plural rule', () => {
  it('picks one / few / many for three-form messages', () => {
    const form = (n: number) => russianPluralRule(n, 3)
    expect([1, 21, 101].map(form)).toEqual([0, 0, 0])
    expect([2, 3, 4, 22, 104].map(form)).toEqual([1, 1, 1, 1, 1])
    expect([0, 5, 11, 12, 14, 25, 111].map(form)).toEqual([2, 2, 2, 2, 2, 2, 2])
  })

  it('falls back to English-style choice for two-form messages', () => {
    expect(russianPluralRule(1, 2)).toBe(0)
    expect(russianPluralRule(0, 2)).toBe(1)
    expect(russianPluralRule(5, 2)).toBe(1)
  })
})
