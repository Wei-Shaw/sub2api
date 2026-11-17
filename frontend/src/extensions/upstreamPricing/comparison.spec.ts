import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ComparisonView from './ComparisonView.vue'
import BrowserImportPanel from './BrowserImportPanel.vue'
import BrowserConnectView from './BrowserConnectView.vue'
import { stationConnectBookmark } from './browserConnect'
import type { PricingConfig, PricingComparison } from '.'

const api = vi.hoisted(() => ({ comparison: vi.fn(), startImport: vi.fn(), importStatus: vi.fn() }))
vi.mock('./api', () => ({ upstreamPricingAPI: api }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot /></div>' } }))
const config: PricingConfig = { provider: 'newapi', base_url: 'https://example.com', group: '', automatic: true, interval_minutes: 60 }
const price = { model: 'm', mode: 'token' as const, input: '2', output: '8', cache_read: '0', cache_write: null, request: null }
function fixture(): PricingComparison {
  return { accounts: [{ id: 7, name: 'Fixture upstream', provider: 'newapi', base_url: config.base_url, group: 'paid', automatic: true, version_id: 3, last_success: null, next_sync: null, stale: false, account_changed: false, last_error: '' }],
    items: [{ account_id: 7, version_id: 3, model: 'm', multiplier: '0.5', base: price, effective: { ...price, input: '1', output: '4' } }], total: 1, page: 1, page_size: 100 }
}
beforeEach(() => { vi.resetAllMocks(); vi.spyOn(window, 'open').mockReturnValue(null); api.comparison.mockResolvedValue(fixture()) })
afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); window.history.replaceState({}, '', '/') })

describe('saved upstream comparison', () => {
  it('shows saved effective prices without multiplying twice, preserving free and unknown values', async () => {
    const wrapper = mount(ComparisonView); await flushPromises()
    const cells = wrapper.findAll('tbody tr').at(0)!.findAll('td').map(v => v.text())
    expect(cells.slice(2, 8)).toEqual(['0.5x', '1', '4', '0', '—', '—'])
    expect(api.comparison).toHaveBeenCalledWith({ model: '', account_id: 0, page: 1, page_size: 100 }, expect.any(AbortSignal))
    await wrapper.find('input').setValue('astra'); await wrapper.find('form').trigger('submit'); await flushPromises()
    expect(api.comparison.mock.lastCall?.[0].model).toBe('astra'); wrapper.unmount()
  })
  it('suppresses effective prices when the upstream identity changed', async () => {
    const value = fixture(); value.accounts[0]!.account_changed = true; api.comparison.mockResolvedValue(value)
    const wrapper = mount(ComparisonView); await flushPromises()
    expect(wrapper.findAll('tbody td').slice(3, 8).map(v => v.text())).toEqual(['—', '—', '—', '—', '—'])
    expect(wrapper.text()).toContain('快照仅供历史参考'); wrapper.unmount()
  })
})

describe('explicit station connection', () => {
  it('only opens the local receiver after confirming on the expected station', () => {
    const open = vi.fn(), alert = vi.fn(), confirm = vi.fn(() => true)
    const code = decodeURIComponent(stationConnectBookmark(config.base_url, 'http://localhost:8080/pricing-browser-connect#synthetic-ticket').slice('javascript:'.length))
    const run = new Function('location', 'window', 'alert', 'confirm', code)
    run({ origin: 'https://other.example' }, { open }, alert, confirm); expect(open).not.toHaveBeenCalled()
    run({ origin: config.base_url }, { open }, alert, confirm); expect(open).toHaveBeenCalledWith('http://localhost:8080/pricing-browser-connect#synthetic-ticket', '_blank', 'noopener,noreferrer')
    expect(confirm).toHaveBeenCalledTimes(1)
  })
  it('imports verified groups once and stops polling', async () => {
    vi.useFakeTimers()
    api.startImport.mockResolvedValue({ id: 'import-id', ticket: 'ticket', expires_at: new Date(Date.now() + 300000).toISOString() })
    api.importStatus.mockResolvedValue({ id: 'import-id', status: 'ready', user_id: '42', groups: [{ name: 'paid', multiplier: '0.5' }] })
    const wrapper = mount(BrowserImportPanel, { props: { accountId: 7, revision: 1, config } })
    await wrapper.find('button').trigger('click'); await flushPromises()
    expect(wrapper.text()).toContain('Firefox')
    await vi.advanceTimersByTimeAsync(2000); await flushPromises()
    expect(wrapper.emitted('imported')?.[0]?.[0]).toMatchObject({ id: 'import-id', user_id: '42' })
    await vi.advanceTimersByTimeAsync(10000); expect(api.importStatus).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })
  it('reserves a Firefox tab during the click and navigates it with a ticket only', async () => {
    const replace = vi.fn(), close = vi.fn()
    const popup = { opener: {}, location: { replace }, close }
    vi.mocked(window.open).mockReturnValue(popup as unknown as Window)
    api.startImport.mockResolvedValue({ id: 'import-id', ticket: 'a'.repeat(48), expires_at: new Date(Date.now() + 300000).toISOString() })
    const wrapper = mount(BrowserImportPanel, { props: { accountId: 7, revision: 1, config: { ...config, token: 'synthetic-do-not-copy' } } })
    const installer = wrapper.find('a').attributes('href')!
    expect(installer).toContain('browser-connector.user.js')
    expect(installer).not.toContain('synthetic-do-not-copy')
    await wrapper.find('button').trigger('click'); await flushPromises()
    expect(window.open).toHaveBeenCalledWith('about:blank', '_blank')
    expect(popup.opener).toBeNull()
    expect(replace).toHaveBeenCalledWith(`https://example.com/#sub2api-connect=${'a'.repeat(48)}`)
    expect(close).not.toHaveBeenCalled()
    wrapper.unmount()
  })
  it('clears the token after validation and keeps it out of the URL', async () => {
    const ticket = 'a'.repeat(48); window.history.replaceState({}, '', `/pricing-browser-connect#${ticket}`)
    const request = vi.fn().mockResolvedValueOnce({ ok: true, json: async () => ({ code: 0, data: { base_url: config.base_url } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ code: 0, data: { verified: true } }) })
    vi.stubGlobal('fetch', request)
    const wrapper = mount(BrowserConnectView); await flushPromises()
    expect(window.location.hash).toBe('')
    await wrapper.find('input[type=password]').setValue('synthetic-panel-token')
    await wrapper.find('form').trigger('submit'); await flushPromises()
    expect(request.mock.calls[1]?.[0]).toBe('/api/v1/upstream-pricing/browser-import/complete')
    expect(JSON.parse(request.mock.calls[1]?.[1].body).token).toBe('synthetic-panel-token')
    expect(wrapper.find('input[type=password]').exists()).toBe(false)
    expect(wrapper.text()).toContain('已验证'); expect(window.location.href).not.toContain('synthetic-panel-token')
    wrapper.unmount()
  })
})
