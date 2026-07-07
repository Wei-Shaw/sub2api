import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  clearAffiliateReferralCode,
  clearOAuthAffiliateCode,
  loadAffiliateReferralCode,
  loadOAuthAffiliateCode,
  resolveAffiliateReferralCode,
  storeAffiliateReferralCode,
  storeOAuthAffiliateCode
} from '@/utils/oauthAffiliate'

describe('oauthAffiliate', () => {
  beforeEach(() => {
    localStorage.clear()
    sessionStorage.clear()
    vi.useRealTimers()
  })

  it('persists affiliate referral code across pages', () => {
    expect(resolveAffiliateReferralCode(' 5579J7CFG9PF ')).toBe('5579J7CFG9PF')
    expect(loadAffiliateReferralCode()).toBe('5579J7CFG9PF')
    expect(resolveAffiliateReferralCode()).toBe('5579J7CFG9PF')
  })

  it('expires stale affiliate referral code', () => {
    const now = Date.UTC(2026, 0, 1)
    storeAffiliateReferralCode('AFF123', now)

    expect(loadAffiliateReferralCode(now + 30 * 24 * 60 * 60 * 1000 - 1)).toBe('AFF123')
    expect(loadAffiliateReferralCode(now + 30 * 24 * 60 * 60 * 1000 + 1)).toBe('')
    expect(localStorage.getItem('affiliate_referral_code')).toBeNull()
  })

  it('keeps oauth transient code separate from persistent referral code', () => {
    storeAffiliateReferralCode('PERSISTED')
    storeOAuthAffiliateCode('OAUTH')

    expect(loadAffiliateReferralCode()).toBe('PERSISTED')
    expect(loadOAuthAffiliateCode()).toBe('OAUTH')

    clearOAuthAffiliateCode()
    expect(loadOAuthAffiliateCode()).toBe('')
    expect(loadAffiliateReferralCode()).toBe('PERSISTED')

    clearAffiliateReferralCode()
    expect(loadAffiliateReferralCode()).toBe('')
  })
})
