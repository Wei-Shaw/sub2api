import { describe, expect, it } from 'vitest'
import {
  applyExplicitCacheUpstreamCredentials,
  isOfficialGeminiBaseUrl,
  normalizeExplicitCacheUpstreamMaxTTL
} from '../geminiExplicitCache'

describe('isOfficialGeminiBaseUrl', () => {
  it('treats empty and the official host as official', () => {
    expect(isOfficialGeminiBaseUrl('')).toBe(true)
    expect(isOfficialGeminiBaseUrl(undefined)).toBe(true)
    expect(isOfficialGeminiBaseUrl('https://generativelanguage.googleapis.com')).toBe(true)
    expect(isOfficialGeminiBaseUrl('https://generativelanguage.googleapis.com/')).toBe(true)
  })

  it('treats relays, path prefixes and invalid urls as custom', () => {
    expect(isOfficialGeminiBaseUrl('https://relay.example.com')).toBe(false)
    expect(isOfficialGeminiBaseUrl('https://generativelanguage.googleapis.com/gw')).toBe(false)
    expect(isOfficialGeminiBaseUrl('http://generativelanguage.googleapis.com')).toBe(false)
    expect(isOfficialGeminiBaseUrl('not a url')).toBe(false)
  })
})

describe('applyExplicitCacheUpstreamCredentials', () => {
  it('writes the switch and an optional ttl cap', () => {
    const credentials: Record<string, unknown> = {}
    applyExplicitCacheUpstreamCredentials(credentials, true, '1800')
    expect(credentials).toEqual({ explicit_cache_upstream: true, explicit_cache_upstream_max_ttl_seconds: 1800 })

    applyExplicitCacheUpstreamCredentials(credentials, true, '')
    expect(credentials).toEqual({ explicit_cache_upstream: true })
  })

  it('removes both fields when disabled', () => {
    const credentials: Record<string, unknown> = {
      api_key: 'k',
      explicit_cache_upstream: true,
      explicit_cache_upstream_max_ttl_seconds: 600
    }
    applyExplicitCacheUpstreamCredentials(credentials, false, 600)
    expect(credentials).toEqual({ api_key: 'k' })
  })

  it('ignores non-positive caps', () => {
    expect(normalizeExplicitCacheUpstreamMaxTTL(0)).toBeNull()
    expect(normalizeExplicitCacheUpstreamMaxTTL(-5)).toBeNull()
    expect(normalizeExplicitCacheUpstreamMaxTTL('abc')).toBeNull()
    expect(normalizeExplicitCacheUpstreamMaxTTL(90.7)).toBe(90)
  })
})
