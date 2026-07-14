import { describe, expect, it } from 'vitest'
import type { Account } from '@/types'
import { isAccountRateLimited, isGrokFreeRecoveryPending } from '../accountStatus'

const makeAccount = (overrides: Partial<Account> = {}): Account => ({
  id: 1,
  name: 'account',
  platform: 'grok',
  type: 'oauth',
  proxy_id: null,
  concurrency: 1,
  priority: 1,
  status: 'active',
  error_message: null,
  last_used_at: null,
  expires_at: null,
  auto_pause_on_expired: true,
  created_at: '2026-07-14T00:00:00Z',
  updated_at: '2026-07-14T00:00:00Z',
  schedulable: true,
  rate_limited_at: null,
  rate_limit_reset_at: null,
  overload_until: null,
  temp_unschedulable_until: null,
  temp_unschedulable_reason: null,
  session_window_start: null,
  session_window_end: null,
  session_window_status: null,
  ...overrides
})

describe('accountStatus', () => {
  it('keeps a pending Grok Free account rate limited after its reset timestamp expires', () => {
    const account = makeAccount({
      grok_free_recovery_pending: true,
      rate_limit_reset_at: '2026-07-14T00:01:00Z'
    })

    expect(isGrokFreeRecoveryPending(account)).toBe(true)
    expect(isAccountRateLimited(account, Date.parse('2026-07-14T00:02:00Z'))).toBe(true)
  })

  it('keeps the timestamp behavior for ordinary accounts', () => {
    const account = makeAccount({
      grok_free_recovery_pending: false,
      rate_limit_reset_at: '2026-07-14T00:03:00Z'
    })

    expect(isAccountRateLimited(account, Date.parse('2026-07-14T00:02:00Z'))).toBe(true)
    expect(isAccountRateLimited(account, Date.parse('2026-07-14T00:04:00Z'))).toBe(false)
  })

  it('does not apply a misplaced Grok Free marker to other platforms', () => {
    const account = makeAccount({
      platform: 'openai',
      grok_free_recovery_pending: true
    })

    expect(isGrokFreeRecoveryPending(account)).toBe(false)
    expect(isAccountRateLimited(account)).toBe(false)
  })
})
