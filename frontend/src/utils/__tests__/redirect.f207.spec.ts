import { describe, it, expect } from 'vitest'
import { sanitizeRedirectPath, DEFAULT_REDIRECT_PATH } from '../redirect'

// F2-07: 重复的 ?redirect= 会让 vue-router 返回数组，sanitizeRedirectPath 不应抛异常
const f207Sanitize = sanitizeRedirectPath as (path: unknown) => string

describe('sanitizeRedirectPath with non-string query values (F2-07)', () => {
  it('does not throw for a repeated redirect query (array) and uses the first string', () => {
    expect(() => f207Sanitize(['/keys', '/usage'])).not.toThrow()
    expect(f207Sanitize(['/keys', '/usage'])).toBe('/keys')
  })

  it('still sanitizes the first array element', () => {
    expect(f207Sanitize(['//evil.com', '/keys'])).toBe(DEFAULT_REDIRECT_PATH)
    expect(f207Sanitize(['https://evil.com'])).toBe(DEFAULT_REDIRECT_PATH)
  })

  it('falls back to the default for arrays without a usable string', () => {
    expect(f207Sanitize([])).toBe(DEFAULT_REDIRECT_PATH)
    expect(f207Sanitize([null])).toBe(DEFAULT_REDIRECT_PATH)
    expect(f207Sanitize([null, '/keys'])).toBe(DEFAULT_REDIRECT_PATH)
  })

  it('falls back to the default for other non-string values', () => {
    expect(f207Sanitize(42)).toBe(DEFAULT_REDIRECT_PATH)
    expect(f207Sanitize({ path: '/keys' })).toBe(DEFAULT_REDIRECT_PATH)
    expect(f207Sanitize(true)).toBe(DEFAULT_REDIRECT_PATH)
  })
})
