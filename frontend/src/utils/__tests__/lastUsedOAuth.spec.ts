import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  clearLastUsedOAuthProvider,
  getLastUsedOAuthProvider,
  promotePendingOAuthProvider,
  setPendingOAuthProvider
} from '../lastUsedOAuth'

const PENDING_KEY = 'sub2api_pending_oauth_provider'
const LAST_USED_KEY = 'sub2api_last_used_oauth'
const NINETY_DAYS_MS = 90 * 24 * 60 * 60 * 1000

describe('lastUsedOAuth', () => {
  beforeEach(() => {
    localStorage.clear()
    sessionStorage.clear()
    vi.useRealTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('promotes a pending provider to the last-used slot on success', () => {
    setPendingOAuthProvider('github')
    expect(sessionStorage.getItem(PENDING_KEY)).toBe('github')

    promotePendingOAuthProvider()

    expect(getLastUsedOAuthProvider()).toBe('github')
    // pending is consumed after promotion
    expect(sessionStorage.getItem(PENDING_KEY)).toBeNull()
  })

  it('is a no-op when there is no pending provider (does not clobber existing value)', () => {
    setPendingOAuthProvider('linuxdo')
    promotePendingOAuthProvider()
    expect(getLastUsedOAuthProvider()).toBe('linuxdo')

    // e.g. an email-verification link hits setToken with nothing pending
    promotePendingOAuthProvider()
    expect(getLastUsedOAuthProvider()).toBe('linuxdo')
  })

  it('clears the last-used hint (password / 2FA / register path)', () => {
    setPendingOAuthProvider('google')
    promotePendingOAuthProvider()
    expect(getLastUsedOAuthProvider()).toBe('google')

    clearLastUsedOAuthProvider()

    expect(getLastUsedOAuthProvider()).toBeNull()
    expect(localStorage.getItem(LAST_USED_KEY)).toBeNull()
    expect(sessionStorage.getItem(PENDING_KEY)).toBeNull()
  })

  it('expires and prunes a record older than 90 days', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-01T00:00:00Z'))

    setPendingOAuthProvider('oidc')
    promotePendingOAuthProvider()
    expect(getLastUsedOAuthProvider()).toBe('oidc')

    // just under 90 days: still valid
    vi.setSystemTime(new Date(Date.now() + NINETY_DAYS_MS - 1000))
    expect(getLastUsedOAuthProvider()).toBe('oidc')

    // just over 90 days: expired and pruned
    vi.setSystemTime(new Date(Date.now() + 2000))
    expect(getLastUsedOAuthProvider()).toBeNull()
    expect(localStorage.getItem(LAST_USED_KEY)).toBeNull()
  })

  it('ignores an unknown pending provider value', () => {
    sessionStorage.setItem(PENDING_KEY, 'myspace')
    promotePendingOAuthProvider()
    expect(getLastUsedOAuthProvider()).toBeNull()
  })

  it('returns null for a malformed stored record', () => {
    localStorage.setItem(LAST_USED_KEY, 'not-json')
    expect(getLastUsedOAuthProvider()).toBeNull()
    expect(localStorage.getItem(LAST_USED_KEY)).toBeNull()
  })
})
