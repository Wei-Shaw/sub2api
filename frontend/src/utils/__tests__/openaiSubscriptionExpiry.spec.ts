import { describe, expect, it } from 'vitest'
import type { Account } from '@/types'
import {
  getOpenAISubscriptionExpiryDisplay,
  OPENAI_SUBSCRIPTION_EXPIRY_SNAPSHOT_KEY
} from '../openaiSubscriptionExpiry'

const account = (overrides: Partial<Account> = {}): Account => ({
  id: 41,
  name: 'account',
  platform: 'openai',
  type: 'oauth',
  proxy_id: null,
  concurrency: 1,
  priority: 1,
  status: 'active',
  error_message: null,
  last_used_at: null,
  expires_at: 1_786_078_800,
  auto_pause_on_expired: false,
  created_at: '2026-08-01T00:00:00Z',
  updated_at: '2026-08-01T00:00:00Z',
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

describe('getOpenAISubscriptionExpiryDisplay', () => {
  it('prefers the explicitly queried upstream snapshot over manual expiry', () => {
    const result = getOpenAISubscriptionExpiryDisplay(account({
      extra: {
        [OPENAI_SUBSCRIPTION_EXPIRY_SNAPSHOT_KEY]: {
          status: 'available',
          expires_at: '2026-08-08T07:23:45Z',
          checked_at: '2026-08-07T06:17:00Z',
          source: 'subscriptions'
        }
      }
    }))

    expect(result).toMatchObject({
      expiresAt: '2026-08-08T07:23:45Z',
      source: 'upstream',
      checkedAt: '2026-08-07T06:17:00Z'
    })
  })

  it('falls back to the operator-entered expiry after a trusted unavailable result', () => {
    const result = getOpenAISubscriptionExpiryDisplay(account({
      credentials: { subscription_expires_at: '2027-01-01T00:00:00Z' },
      extra: {
        [OPENAI_SUBSCRIPTION_EXPIRY_SNAPSHOT_KEY]: {
          status: 'unavailable',
          checked_at: '2026-08-07T06:17:00Z',
          source: 'accounts_check'
        }
      }
    }))

    expect(result.source).toBe('manual')
    expect(result.expiresAt).toBe(new Date(1_786_078_800 * 1000).toISOString())
  })

  it('keeps showing a legacy real expiry until the first explicit query', () => {
    expect(getOpenAISubscriptionExpiryDisplay(account({
      credentials: { subscription_expires_at: '2026-08-08T07:23:45Z' }
    }))).toEqual({
      expiresAt: '2026-08-08T07:23:45Z',
      source: 'legacy'
    })
  })
})
