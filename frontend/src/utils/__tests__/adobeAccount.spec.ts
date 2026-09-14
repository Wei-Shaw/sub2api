import { describe, expect, it } from 'vitest'

import { isAdobeRelayAccount } from '@/utils/adobeAccount'

describe('isAdobeRelayAccount', () => {
  it('requires adobe apikey with a non-empty base_url', () => {
    expect(isAdobeRelayAccount(null)).toBe(false)
    expect(isAdobeRelayAccount({ platform: 'adobe', type: 'oauth', credentials: { cookie: 'x' } })).toBe(false)
    expect(isAdobeRelayAccount({ platform: 'adobe', type: 'apikey', credentials: { api_key: 'sk' } })).toBe(false)
    expect(isAdobeRelayAccount({
      platform: 'openai',
      type: 'apikey',
      credentials: { api_key: 'sk', base_url: 'https://relay.example' }
    })).toBe(false)
    expect(isAdobeRelayAccount({
      platform: 'adobe',
      type: 'apikey',
      credentials: { api_key: 'sk', base_url: ' https://relay.example ' }
    })).toBe(true)
  })
})
