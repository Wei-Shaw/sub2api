import { describe, expect, it } from 'vitest'
import {
  mergeIntoBlacklist,
  normalizeBlacklistEmail,
  parseBlacklistInput
} from '../registrationEmailBlacklist'

describe('registrationEmailBlacklist', () => {
  it('normalizes addresses the same way as the backend', () => {
    expect(normalizeBlacklistEmail('  Alice@Gmail.COM ')).toBe('alice@gmail.com')
    expect(normalizeBlacklistEmail('user@example.com.')).toBe('user@example.com')
    expect(normalizeBlacklistEmail('a.b+tag@googlemail.com')).toBe('a.b+tag@googlemail.com')
    expect(normalizeBlacklistEmail('@gmail.com')).toBeNull()
    expect(normalizeBlacklistEmail('user@')).toBeNull()
    expect(normalizeBlacklistEmail('a@b@gmail.com')).toBeNull()
    expect(normalizeBlacklistEmail('user@localhost')).toBeNull()
    expect(normalizeBlacklistEmail('user@bad_domain.com')).toBeNull()
    expect(normalizeBlacklistEmail(`${'x'.repeat(65)}@gmail.com`)).toBeNull()
    expect(normalizeBlacklistEmail('a\u0085b@gmail.com')).toBeNull()
    expect(normalizeBlacklistEmail(`${'用'.repeat(21)}@example.com`)).toBe(`${'用'.repeat(21)}@example.com`)
    expect(normalizeBlacklistEmail(`${'用'.repeat(22)}@example.com`)).toBeNull()
    expect(normalizeBlacklistEmail('İvan@Example.com')).toBe('İvan@example.com')
  })

  it('splits pasted text and separates invalid tokens', () => {
    expect(parseBlacklistInput('A@gmail.com, b@gmail.com；c@gmail.com\n\nbad a@gmail.com')).toEqual({
      valid: ['a@gmail.com', 'b@gmail.com', 'c@gmail.com'],
      invalid: ['bad'],
      duplicates: 1
    })
    expect(parseBlacklistInput('   ')).toEqual({ valid: [], invalid: [], duplicates: 0 })
  })

  it('prepends only new addresses and counts duplicates', () => {
    expect(mergeIntoBlacklist(['x@gmail.com'], ['y@gmail.com', 'x@gmail.com', 'z@gmail.com'])).toEqual({
      list: ['y@gmail.com', 'z@gmail.com', 'x@gmail.com'],
      added: ['y@gmail.com', 'z@gmail.com'],
      duplicates: 1
    })
  })
})
