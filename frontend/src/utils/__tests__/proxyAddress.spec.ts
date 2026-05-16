import { describe, expect, it } from 'vitest'
import {
  buildProxyUrl,
  detectProxyIPVersion,
  formatProxyAddress,
  formatProxyHost,
  normalizeProxyHost,
  parseProxyUrl
} from '../proxyAddress'

describe('proxyAddress', () => {
  it('normalizes bracketed IPv6 hosts', () => {
    expect(normalizeProxyHost('[2001:db8::1]')).toBe('2001:db8::1')
    expect(normalizeProxyHost(' [::1] ')).toBe('::1')
  })

  it('detects IPv6 correctly', () => {
    expect(detectProxyIPVersion('45.145.57.212')).toBe('ipv4')
    expect(detectProxyIPVersion('[2001:db8::1]')).toBe('ipv6')
  })

  it('formats IPv6 hosts with brackets for display', () => {
    expect(formatProxyHost('2001:db8::1')).toBe('[2001:db8::1]')
    expect(formatProxyAddress('2001:db8::1', 1080)).toBe('[2001:db8::1]:1080')
  })

  it('builds IPv6 proxy urls correctly', () => {
    expect(
      buildProxyUrl({
        protocol: 'socks5',
        host: '2001:db8::1',
        port: 1080,
        username: 'user',
        password: 'p@ss:word'
      })
    ).toBe('socks5://user:p%40ss%3Aword@[2001:db8::1]:1080')
  })

  it('parses IPv6 proxy urls and removes brackets from host storage', () => {
    expect(parseProxyUrl('socks5://user:pass@[2001:db8::1]:1080')).toEqual({
      protocol: 'socks5',
      host: '2001:db8::1',
      port: 1080,
      username: 'user',
      password: 'pass'
    })
  })

  it('rejects unsupported proxy urls', () => {
    expect(parseProxyUrl('ftp://example.com:21')).toBeNull()
    expect(parseProxyUrl('socks5://[2001:db8::1]')).toBeNull()
  })
})
