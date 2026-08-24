import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

// parseProxyUrl is not exported; assert on the source to lock in the
// bracketed-IPv6 host alternative and exercise the regex directly.
const source = readFileSync(
  resolve(process.cwd(), 'src/views/admin/ProxiesView.vue'),
  'utf8'
)

function extractRegex(): RegExp {
  const match = source.match(/const regex =\s*\n?\s*(\/\^\(https\?[^;\n]+\/i)\n/)
  expect(match, 'parseProxyUrl regex not found in ProxiesView.vue').toBeTruthy()
  return new RegExp((match as RegExpMatchArray)[1].slice(1, -2), 'i')
}

describe('proxy batch URL parsing (IPv6 support)', () => {
  it('keeps bracketed-IPv6 host alternative in the regex', () => {
    expect(source).toContain('\\[[0-9a-f:.]+\\]')
  })

  const regex = extractRegex()

  it.each([
    ['socks5://[2001:db8::1]:1080', true],
    ['socks5h://[2001:db8::1]:1080', true],
    ['http://[::1]:8080', true],
    ['socks5://user:pass@[2001:db8::1]:1080', true],
    ['socks5://proxy.example.com:1080', true],
    ['http://192.168.1.1:8080', true],
    ['socks5://user:pass@proxy.example.com:1080', true],
    // bare IPv6 without brackets is ambiguous with host:port — rejected
    ['socks5://2001:db8::1:1080', false],
    // unsupported schemes / malformed ports stay invalid
    ['ftp://example.com:21', false],
    ['socks5://example.com:port', false]
  ])('%s => %s', (line, expected) => {
    expect(regex.test(line)).toBe(expected)
  })

  it('extracts bare IPv6 host without brackets', () => {
    const m = 'socks5://user:pass@[2001:db8::1]:1080'.match(regex)
    expect(m).toBeTruthy()
    const [, , username, password, rawHost, port] = m as RegExpMatchArray
    expect(username).toBe('user')
    expect(password).toBe('pass')
    expect(rawHost.replace(/^\[|\]$/g, '')).toBe('2001:db8::1')
    expect(port).toBe('1080')
  })
})
