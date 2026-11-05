import { describe, expect, it } from 'vitest'
import { buildProxyConsoleURL } from '../proxyConsole'

describe('buildProxyConsoleURL', () => {
  it('builds a MetaCubeXD auto-connect link for the standard UI path', () => {
    expect(buildProxyConsoleURL('http://proxy.example.com:9090/ui/')).toBe(
      'http://proxy.example.com:9090/ui/?hostname=proxy.example.com&port=9090&http=1#/proxies'
    )
  })

  it('opens a non-MetaCubeXD console without adding parameters or changing its route', () => {
    expect(buildProxyConsoleURL('https://proxy.example.com/dashboard?view=nodes#active')).toBe(
      'https://proxy.example.com/dashboard?view=nodes#active'
    )
  })

  it('rejects credentials and secrets', () => {
    const authenticatedURL = new URL('http://proxy.example.com/ui/')
    authenticatedURL.username = 'example-user'
    authenticatedURL.password = 'example-password'
    expect(() => buildProxyConsoleURL(authenticatedURL.toString())).toThrow()
    expect(() => buildProxyConsoleURL('http://proxy.example.com/ui/?secret=value')).toThrow()
  })
})
