import { describe, expect, it } from 'vitest'

import { extractAdobeAccessTokenFromCookieInput, extractAdobeAccessTokenInput, extractAdobeArpFromCookieInput, extractAdobeArpSessionInput, extractAdobeCookieInput, isAdobeRelayAccount } from '@/utils/adobeAccount'

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

describe('extractAdobeCookieInput', () => {
  it('keeps a raw cookie string and strips a Cookie: prefix', () => {
    expect(extractAdobeCookieInput('  ims_sid=abc; aux_sid=def  ')).toBe('ims_sid=abc; aux_sid=def')
    expect(extractAdobeCookieInput('Cookie: ims_sid=abc; aux_sid=def')).toBe('ims_sid=abc; aux_sid=def')
    expect(extractAdobeCookieInput('COOKIE:  ims_sid=abc')).toBe('ims_sid=abc')
  })

  it('unwraps {cookie} / {cookies} objects and cookie arrays', () => {
    expect(extractAdobeCookieInput(JSON.stringify({ cookie: 'ims_sid=abc' }))).toBe('ims_sid=abc')
    expect(extractAdobeCookieInput(JSON.stringify({ cookies: ['a=1', ' b=2 '] }))).toBe('a=1; b=2')
    expect(
      extractAdobeCookieInput(
        JSON.stringify([
          { name: 'ims_sid', value: 'abc' },
          { name: 'aux_sid', value: 'def' }
        ])
      )
    ).toBe('ims_sid=abc; aux_sid=def')
  })

  it('unwraps a sub2api-data envelope from the cookie exporter', () => {
    expect(
      extractAdobeCookieInput(
        JSON.stringify({
          type: 'sub2api-data',
          version: 1,
          exported_at: '2026-09-20T06:54:00.000Z',
          proxies: [],
          accounts: [
            {
              name: 'adobe-jane@example.com',
              platform: 'adobe',
              type: 'oauth',
              credentials: { cookie: 'ims_sid=abc; aux_sid=def' },
              concurrency: 10,
              priority: 1
            }
          ]
        })
      )
    ).toBe('ims_sid=abc; aux_sid=def')
  })

  it('returns empty for blank or cookie-less JSON', () => {
    expect(extractAdobeCookieInput('')).toBe('')
    expect(extractAdobeCookieInput('   ')).toBe('')
    expect(extractAdobeCookieInput(JSON.stringify({ foo: 'bar' }))).toBe('')
    expect(extractAdobeCookieInput('{"oops"')).toBe('{"oops"')
  })
})

describe('extractAdobeArpSessionInput', () => {
  it('keeps a raw ARP token', () => {
    expect(extractAdobeArpSessionInput('  eyJzaWQiOiIxIiwiZnRyIjoieCJ9  ')).toBe('eyJzaWQiOiIxIiwiZnRyIjoieCJ9')
  })

  it('unwraps {arp_session_id} objects', () => {
    expect(extractAdobeArpSessionInput(JSON.stringify({ arp_session_id: 'arp-token' }))).toBe('arp-token')
  })

  it('unwraps a sub2api-data envelope with arp_session_id', () => {
    expect(
      extractAdobeArpSessionInput(
        JSON.stringify({
          type: 'sub2api-data',
          accounts: [
            {
              credentials: { cookie: 'ims_sid=abc', arp_session_id: 'arp-from-json' }
            }
          ]
        })
      )
    ).toBe('arp-from-json')
  })

  it('returns empty when the envelope has no ARP field', () => {
    expect(
      extractAdobeArpSessionInput(
        JSON.stringify({
          type: 'sub2api-data',
          accounts: [{ credentials: { cookie: 'ims_sid=abc' } }]
        })
      )
    ).toBe('')
    expect(extractAdobeArpSessionInput('')).toBe('')
    expect(extractAdobeArpSessionInput(JSON.stringify({ cookie: 'ims_sid=abc' }))).toBe('')
  })

  it('does not treat a raw cookie string as ARP', () => {
    expect(extractAdobeArpFromCookieInput('ims_sid=abc; aux_sid=def')).toBe('')
    expect(
      extractAdobeArpFromCookieInput(
        JSON.stringify({
          type: 'sub2api-data',
          accounts: [{ credentials: { cookie: 'ims_sid=abc', arp_session_id: 'arp-from-cookie-json' } }]
        })
      )
    ).toBe('arp-from-cookie-json')
  })
})

describe('extractAdobeAccessTokenInput', () => {
  it('keeps a raw IMS token and strips a Bearer prefix', () => {
    expect(extractAdobeAccessTokenInput('  eyJhbGciOiJIUzI1NiJ9.e30.sig  ')).toBe('eyJhbGciOiJIUzI1NiJ9.e30.sig')
    expect(extractAdobeAccessTokenInput('Bearer eyJhbGciOiJIUzI1NiJ9.e30.sig')).toBe('eyJhbGciOiJIUzI1NiJ9.e30.sig')
  })

  it('unwraps {access_token} objects and a sub2api-data envelope', () => {
    expect(extractAdobeAccessTokenInput(JSON.stringify({ access_token: 'ims-token' }))).toBe('ims-token')
    expect(
      extractAdobeAccessTokenInput(
        JSON.stringify({
          type: 'sub2api-data',
          accounts: [
            {
              credentials: { cookie: 'ims_sid=abc', access_token: 'ims-from-json' }
            }
          ]
        })
      )
    ).toBe('ims-from-json')
  })

  it('does not treat a raw cookie string as an IMS token', () => {
    expect(extractAdobeAccessTokenFromCookieInput('ims_sid=abc; aux_sid=def')).toBe('')
    expect(extractAdobeAccessTokenFromCookieInput(JSON.stringify({ cookie: 'ims_sid=abc' }))).toBe('')
    expect(
      extractAdobeAccessTokenFromCookieInput(
        JSON.stringify({
          type: 'sub2api-data',
          accounts: [{ credentials: { cookie: 'ims_sid=abc', access_token: 'ims-from-cookie-json' } }]
        })
      )
    ).toBe('ims-from-cookie-json')
  })
})
