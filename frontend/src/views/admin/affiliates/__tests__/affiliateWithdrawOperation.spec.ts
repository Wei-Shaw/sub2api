import { beforeEach, describe, expect, it } from 'vitest'
import { completeAffiliateWithdrawOperation, prepareAffiliateWithdrawOperation } from '../affiliateWithdrawOperation'

let adminId = 2000

beforeEach(() => {
  localStorage.setItem('auth_user', JSON.stringify({ id: ++adminId }))
  sessionStorage.clear()
})

describe('affiliate offline withdrawal operation identity', () => {
  it('keeps the key of an unfinished registration for the same user and ledger amount', () => {
    const first = prepareAffiliateWithdrawOperation(42, 10)
    const retry = prepareAffiliateWithdrawOperation(42, 10.000000001)

    expect(first.outcomeUncertain).toBe(false)
    expect(first.key).toMatch(new RegExp(`^affiliate-withdraw-${adminId}-`))
    expect(retry.key).toBe(first.key)
    expect(retry.amount).toBe(10)
    expect(retry.outcomeUncertain).toBe(true)

    completeAffiliateWithdrawOperation(retry)
    const next = prepareAffiliateWithdrawOperation(42, 10)
    expect(next.key).not.toBe(first.key)
    expect(next.outcomeUncertain).toBe(false)
  })

  it('isolates different users, amounts and administrators', () => {
    const first = prepareAffiliateWithdrawOperation(42, 10)
    const differentAmount = prepareAffiliateWithdrawOperation(42, 10.5)
    const differentUser = prepareAffiliateWithdrawOperation(43, 10)
    localStorage.setItem('auth_user', JSON.stringify({ id: ++adminId }))
    const differentAdmin = prepareAffiliateWithdrawOperation(42, 10)

    expect(new Set([first, differentAmount, differentUser, differentAdmin].map(operation => operation.key)).size).toBe(4)
  })

  it('resumes a key stored by an earlier page and clears it after completion', () => {
    const scope = `sub2api:admin:affiliate-withdraw:${adminId}:42:12.5`
    sessionStorage.setItem(scope, 'saved-withdraw-key')

    const resumed = prepareAffiliateWithdrawOperation(42, 12.5)
    expect(resumed.key).toBe('saved-withdraw-key')
    expect(resumed.outcomeUncertain).toBe(true)

    completeAffiliateWithdrawOperation(resumed)
    expect(sessionStorage.getItem(scope)).toBeNull()
    expect(prepareAffiliateWithdrawOperation(42, 12.5).key).not.toBe('saved-withdraw-key')
  })
})
