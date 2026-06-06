import { describe, expect, it } from 'vitest'
import { formatUserDisplayName } from '@/utils/format'

describe('formatUserDisplayName', () => {
  it('prefers explicit device_code over username and email', () => {
    expect(formatUserDisplayName({
      user_id: 42,
      email: 'invite-8794805d94e3@example.com',
      username: 'long-generated-username',
      device_code: 'DLG-ABCD-1234'
    })).toBe('DLG-ABCD-1234')
  })

  it('treats dashboard/subscription device identity aliases as device display codes', () => {
    expect(formatUserDisplayName({
      user_id: 43,
      email: 'invite-8794805d94e4@example.com',
      username: 'invite-8794805d94e4',
      device_identity_code: 'DLG-WXYZ-9876'
    })).toBe('DLG-WXYZ-9876')
  })

  it('keeps the old username/email/id fallback order when no device code exists', () => {
    expect(formatUserDisplayName({
      user_id: 44,
      email: 'fallback@example.com',
      username: 'friendly-name'
    })).toBe('friendly-name')

    expect(formatUserDisplayName({
      user_id: 45,
      email: 'fallback-long-address@example.com'
    }, 16)).toBe('fallback…ple.com')

    expect(formatUserDisplayName({ user_id: 46 })).toBe('User #46')
  })
})
