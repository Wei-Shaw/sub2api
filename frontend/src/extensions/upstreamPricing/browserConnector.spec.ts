import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { runInNewContext } from 'node:vm'
import { flushPromises } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

const script = readFileSync(resolve(process.cwd(), '../backend/internal/pkg/upstreampricing/browser_connector.user.js'), 'utf8')
  .replace('__CONNECTOR_CONFIG__', JSON.stringify({ site: 'https://station.example', receiver: 'http://localhost:8080' }))
const ticket = 'a'.repeat(48)
function run(options: { origin?: string; hash?: string; provider?: string; stored?: Record<string, string>; failInfo?: boolean } = {}) {
  const top = {}, appended: { textContent?: string }[] = []
  const getItem = vi.fn((key: string) => options.stored?.[key] ?? null)
  const network = vi.fn(details => {
    const info = details.url.endsWith('-info')
    details.onload({ status: options.failInfo && info ? 403 : 200, finalUrl: details.url,
      responseText: JSON.stringify({ code: 0, data: info ? { base_url: 'https://station.example', provider: options.provider || 'sub2api', user_id: '' } : { verified: true } }) })
  })
  const history = { replaceState: vi.fn(), state: null }
  runInNewContext(script, {
    URL, window: { top, self: top }, location: { origin: options.origin || 'https://station.example', hash: options.hash ?? `#sub2api-connect=${ticket}`, pathname: '/', search: '' }, history,
    localStorage: { getItem }, GM_xmlhttpRequest: network,
    document: { getElementById: () => null, createElement: () => ({ style: {} }), body: { appendChild: (box: { textContent?: string }) => appended.push(box) } },
  })
  return { network, getItem, appended, history }
}
describe('Firefox connector script', () => {
  it('reads only the selected station panel storage and sends the token in a request body', async () => {
    const result = run({ stored: { auth_token: 'synthetic-panel-token', auth_user: '{"id":42}' } }); await flushPromises()
    expect(result.network).toHaveBeenCalledTimes(2)
    const complete = result.network.mock.calls[1]![0]
    expect(complete.url).toBe('http://localhost:8080/api/v1/upstream-pricing/browser-import/connector-complete')
    expect(JSON.parse(complete.data)).toEqual({ ticket, token: 'synthetic-panel-token', user_id: '42' })
    expect(complete.redirect).toBe('error')
    expect(result.getItem.mock.calls.flat()).toEqual(['auth_user', 'auth_token'])
    expect(result.history.replaceState).toHaveBeenCalledWith(null, '', '/')
    expect(JSON.stringify(result.appended)).not.toContain('synthetic-panel-token')
    expect(result.appended.at(-1)?.textContent).toContain('已自动导入')
  })
  it('supports the legacy New API panel token and numeric user ID', async () => {
    const result = run({ provider: 'newapi', stored: { user: '{"id":7,"access_token":"synthetic-newapi-token"}' } }); await flushPromises()
    expect(JSON.parse(result.network.mock.calls[1]![0].data)).toMatchObject({ user_id: '7', token: 'synthetic-newapi-token' })
  })
  it.each([{ origin: 'https://other.example' }, { hash: '#arbitrary' }])('ignores unbound locations: %j', async options => {
    const result = run(options); await flushPromises()
    expect(result.network).not.toHaveBeenCalled(); expect(result.getItem).not.toHaveBeenCalled()
  })
  it('does not read credentials for invalid or expired tickets', async () => {
    const result = run({ failInfo: true }); await flushPromises()
    expect(result.getItem).not.toHaveBeenCalled()
    expect(result.appended.at(-1)?.textContent).toContain('连接验证失败')
  })
  it('does not guess a token or read cookies when the supported storage is absent', async () => {
    const result = run(); await flushPromises()
    expect(result.network).toHaveBeenCalledTimes(2)
    expect(result.network.mock.calls[1]![0].url).toContain('connector-fail')
    expect(result.appended.at(-1)?.textContent).toContain('未找到可读取')
  })
})
